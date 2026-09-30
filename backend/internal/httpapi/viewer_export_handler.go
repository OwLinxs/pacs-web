package httpapi

import (
	"net/http"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
)

// Specific browser-generated derivative notification; never accepts DICOM/actor/target.
func (s *Server) handleViewerExport(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeError(w, s.log, 400, CodeInvalidRequest, "Exportação inválida.")
		return
	}
	fields, err := decodeUserBody(w, r, "format", "identified")
	var format string
	var identified bool
	if err != nil || len(fields) != 2 || !readUserField(fields, "format", &format) || !readUserField(fields, "identified", &identified) || (format != "PNG" && format != "JPEG" && format != "PDF") {
		writeError(w, s.log, 400, CodeInvalidRequest, "Formato ou identificação inválidos.")
		return
	}
	e := audit.FromContext(r.Context(), audit.EventViewerImageExported)
	e.Detail = audit.ExportDetail(format, identified)
	if s.auditoria == nil || s.auditoria.Record(r.Context(), e) != nil {
		writeError(w, s.log, 503, CodeUnavailable, "Não foi possível registrar a exportação na auditoria.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
