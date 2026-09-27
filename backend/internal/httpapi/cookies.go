package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"
)

// Nomes dos cookies. O de sessão é HttpOnly e nunca é lido por JavaScript; o de
// CSRF precisa ser legível pelo frontend, que o reenvia no header.
const (
	cookieSession = "pacs_session"
	cookieCSRF    = "pacs_csrf"
	headerCSRF    = "X-CSRF-Token"
)

// csrfTokenBytes é o tamanho do token de CSRF sorteado.
const csrfTokenBytes = 32

func novoTokenCSRF() (string, error) {
	bruto := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(bruto); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bruto), nil
}

// setSessionCookie grava o cookie de sessão.
//
// HttpOnly impede leitura por script; SameSite=Lax evita envio em requisição
// cross-site iniciada por terceiro mantendo a navegação normal; Secure é
// obrigatório em produção. A expiração do cookie acompanha o prazo absoluto da
// sessão, mas a validade que vale é a do servidor.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expiraEm time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieSession,
		Value:    token,
		Path:     "/",
		Expires:  expiraEm,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieSession,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// setCSRFCookie grava o cookie de CSRF. Não é HttpOnly de propósito: o
// frontend precisa copiar o valor para o header X-CSRF-Token.
func (s *Server) setCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieCSRF,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func lerCookie(r *http.Request, nome string) string {
	cookie, err := r.Cookie(nome)
	if err != nil {
		return ""
	}
	return cookie.Value
}
