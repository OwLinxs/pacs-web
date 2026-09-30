package httpapi

import (
	"context"
	"net/http"
	"time"
)

type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth diz apenas se o processo responde. Sem versão, sem host, sem
// configuração — nada que ajude a mapear o ambiente.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.log, http.StatusOK, healthResponse{Status: "ok"})
}

// handleReady indica se o backend tem o que precisa para atender: hoje, o
// PostgreSQL da aplicação. Nada aqui consulta o Orthanc.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, s.log, http.StatusOK, healthResponse{Status: "ok"})
		return
	}

	ctx, cancelar := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancelar()

	if err := s.db.Ping(ctx); err != nil {
		s.log.ErrorContext(ctx, "banco indisponível na checagem de prontidão")
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Serviço indisponível.")
		return
	}
	writeJSON(w, s.log, http.StatusOK, healthResponse{Status: "ok"})
}
