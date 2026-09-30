package httpapi

import (
	"context"
	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"net/http"
	"time"
)

type AuditReader interface {
	List(context.Context, audit.Query) (audit.Page, error)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q, err := audit.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, s.log, 400, CodeInvalidRequest, "Filtros de auditoria inválidos.")
		return
	}
	if s.auditReader == nil {
		writeError(w, s.log, 503, CodeUnavailable, "Não foi possível consultar a auditoria.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	page, err := s.auditReader.List(ctx, q)
	if err != nil {
		writeError(w, s.log, 503, CodeUnavailable, "Não foi possível consultar a auditoria. Tente novamente.")
		return
	}
	writeJSON(w, s.log, 200, page)
}
