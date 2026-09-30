package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

// maxCorpoLogin limita o corpo aceito no login.
const maxCorpoLogin = 4 << 10

// loginRequest é o corpo de POST /api/auth/login.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// unitResponse é a unidade como o frontend a recebe.
type unitResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// userResponse é o usuário como o frontend a recebe.
//
// Este struct existe para que nada do modelo interno escape por acidente:
// password_hash não tem representação aqui.
type userResponse struct {
	MustChangePassword bool           `json:"mustChangePassword"`
	Units              []user.UnitRef `json:"units"`
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Username           string         `json:"username"`
	Email              string         `json:"email,omitempty"`
	Role               string         `json:"role"`
	Unit               *unitResponse  `json:"unit"`
	Active             bool           `json:"active"`
	AccessValidUntil   *string        `json:"accessValidUntil"`
	LastLoginAt        *string        `json:"lastLoginAt"`
}

type sessionResponse struct {
	User userResponse `json:"user"`
}

func paraUserResponse(u user.User) userResponse {
	resposta := userResponse{
		MustChangePassword: u.MustChangePassword, Units: u.Units,
		ID:       u.ID.String(),
		Name:     u.Name,
		Username: u.Username,
		Email:    u.Email,
		Role:     string(u.Role),
		Active:   u.Active,
	}
	if resposta.Units == nil {
		resposta.Units = []user.UnitRef{}
	}
	if u.UnitID != nil {
		resposta.Unit = &unitResponse{ID: u.UnitID.String(), Name: u.UnitName}
	}
	if u.AccessValidUntil != nil {
		data := u.AccessValidUntil.Format("2006-01-02")
		resposta.AccessValidUntil = &data
	}
	if u.LastLoginAt != nil {
		instante := u.LastLoginAt.UTC().Format(time.RFC3339)
		resposta.LastLoginAt = &instante
	}
	return resposta
}

// handleLogin autentica e abre a sessão.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	chave := s.ipDoPedido(r)
	if permitido, faltando := s.loginLimiter.Allow(chave); !permitido {
		segundos := int(faltando.Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(segundos))
		s.log.WarnContext(r.Context(), "login bloqueado por rate limit", "ip", chave)
		writeError(w, s.log, http.StatusTooManyRequests, CodeRateLimited,
			"Muitas tentativas. Aguarde alguns minutos e tente novamente.")
		return
	}

	var pedido loginRequest
	decodificador := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCorpoLogin))
	decodificador.DisallowUnknownFields()
	if err := decodificador.Decode(&pedido); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Informe usuário e senha.")
			return
		}
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Requisição inválida.")
		return
	}

	pedido.Username = strings.TrimSpace(pedido.Username)
	if pedido.Username == "" || pedido.Password == "" {
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Informe usuário e senha.")
		return
	}
	if len(pedido.Username) > 64 || len(pedido.Password) > 1024 {
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Requisição inválida.")
		return
	}
	select {
	case s.loginSlots <- struct{}{}:
		defer func() { <-s.loginSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		writeError(w, s.log, http.StatusTooManyRequests, CodeRateLimited, "Muitas tentativas. Aguarde e tente novamente.")
		return
	}

	resultado, err := s.auth.Login(r.Context(), auth.LoginInput{
		Username: pedido.Username,
		Password: pedido.Password,
		Origin: auth.SessionOrigin{
			IP:        chave,
			UserAgent: r.UserAgent(),
		},
	})
	switch {
	case errors.Is(err, auth.ErrCredenciaisInvalidas):
		writeError(w, s.log, http.StatusUnauthorized, CodeInvalidCredentials, "Usuário ou senha inválidos.")
		return
	case errors.Is(err, auth.ErrContaInativa):
		writeError(w, s.log, http.StatusForbidden, CodeAccountInactive,
			"Este acesso está desativado. Procure a administração do sistema.")
		return
	case errors.Is(err, auth.ErrContaExpirada):
		writeError(w, s.log, http.StatusForbidden, CodeAccountExpired,
			"A validade deste acesso expirou. Procure a administração do sistema.")
		return
	case err != nil:
		writeInternalError(w, s.log, r, err)
		return
	}

	// Acerto legítimo libera a contagem daquele endereço.
	s.loginLimiter.Reset(chave)

	s.setSessionCookie(w, resultado.Token, resultado.Session.AbsoluteExpiresAt)
	// Troca o token de CSRF junto com a sessão.
	if token, err := novoTokenCSRF(); err == nil {
		s.setCSRFCookie(w, token)
	}

	writeJSON(w, s.log, http.StatusOK, sessionResponse{User: paraUserResponse(resultado.User)})
}

// handleMe devolve o usuário da sessão atual.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	usuario, ok := UsuarioDoContexto(r.Context())
	if !ok {
		writeError(w, s.log, http.StatusUnauthorized, CodeUnauthenticated, "Sessão inválida ou expirada.")
		return
	}
	writeJSON(w, s.log, http.StatusOK, sessionResponse{User: paraUserResponse(usuario)})
}

// handleLogout encerra a sessão no servidor e limpa o cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	usuario, _ := UsuarioDoContexto(r.Context())
	token := tokenDoContexto(r.Context())

	if err := s.auth.Logout(r.Context(), token, usuario); err != nil {
		writeInternalError(w, s.log, r, err)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminPing confirma que a rota exige perfil ADMIN.
func (s *Server) handleAdminPing(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
