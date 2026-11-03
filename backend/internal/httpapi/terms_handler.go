package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/terms"
)

// TermsStore registra a ciência do termo de responsabilidade e sigilo.
type TermsStore interface {
	Accepted(ctx context.Context, userID uuid.UUID, version string) (bool, error)
	Accept(ctx context.Context, userID uuid.UUID, version string, entry audit.Entry) error
}

const codeTermsRequired = "TERMS_REQUIRED"

// termsCacheTTL cobre o prazo absoluto de sessão; entradas antigas são podadas.
const termsCacheTTL = 24 * time.Hour

// termsAllowed lista o que a sessão pode fazer antes de aceitar o termo.
func termsAllowed(r *http.Request) bool {
	switch r.Method + " " + r.URL.Path {
	case "GET /api/auth/me", "POST /api/auth/logout", "POST /api/auth/change-password", "GET /api/terms", "POST /api/terms/accept":
		return true
	}
	return false
}

// termsAccepted consulta o banco até a sessão aceitar; depois usa o cache em
// memória (o aceite não é revogável dentro da mesma versão do binário).
func (s *Server) termsAccepted(ctx context.Context, sessionID, userID uuid.UUID) (bool, error) {
	if _, ok := s.termsOK.Load(sessionID); ok {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ok, err := s.terms.Accepted(ctx, userID, terms.Version)
	if ok {
		s.termsOK.Store(sessionID, time.Now())
	}
	return ok, err
}

func (s *Server) vacuumTerms(now time.Time) {
	s.termsOK.Range(func(key, value any) bool {
		if at, ok := value.(time.Time); !ok || now.Sub(at) > termsCacheTTL {
			s.termsOK.Delete(key)
		}
		return true
	})
}

func termsRequiredFromContext(ctx context.Context) bool {
	required, _ := ctx.Value(chaveTermos).(bool)
	return required
}

func (s *Server) handleGetTerms(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.log, http.StatusOK, terms.Current)
}

func (s *Server) handleAcceptTerms(w http.ResponseWriter, r *http.Request) {
	usuario, ok := UsuarioDoContexto(r.Context())
	if !ok {
		writeError(w, s.log, http.StatusUnauthorized, CodeUnauthenticated, "Sessão inválida ou expirada.")
		return
	}
	var body struct {
		Version string `json:"version"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || decoder.More() {
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Requisição inválida.")
		return
	}
	if body.Version != terms.Version {
		writeError(w, s.log, http.StatusConflict, CodeConflict, "O termo foi atualizado. Leia a versão atual antes de aceitar.")
		return
	}
	entry := audit.FromContext(r.Context(), audit.EventTermsAccepted)
	entry.Detail = audit.TermsDetail(terms.Version)
	if err := s.terms.Accept(r.Context(), usuario.ID, terms.Version, entry); err != nil {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Não foi possível registrar o aceite. Tente novamente.")
		return
	}
	if sessao, ok := sessaoDoContexto(r.Context()); ok {
		s.termsOK.Store(sessao.ID, time.Now())
	}
	w.WriteHeader(http.StatusNoContent)
}
