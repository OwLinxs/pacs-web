// Package httpapi expõe a API HTTP do backend: roteamento, middlewares e
// handlers. Respostas de erro são sempre sanitizadas — nada de stack trace,
// SQL, caminho de arquivo ou secret chega ao navegador.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Código de erro devolvido no corpo. O frontend decide a mensagem pelo código.
const (
	CodeInvalidRequest     = "INVALID_REQUEST"
	CodeInvalidCredentials = "INVALID_CREDENTIALS"
	CodeAccountInactive    = "ACCOUNT_INACTIVE"
	CodeAccountExpired     = "ACCOUNT_EXPIRED"
	CodeUnauthenticated    = "UNAUTHENTICATED"
	CodeForbidden          = "FORBIDDEN"
	CodeNotFound           = "NOT_FOUND"
	CodeConflict           = "CONFLICT"
	CodeRateLimited        = "RATE_LIMITED"
	CodeCSRFInvalid        = "CSRF_INVALID"
	CodeInternal           = "INTERNAL_ERROR"
	CodeUnavailable        = "SERVICE_UNAVAILABLE"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// writeJSON serializa a resposta. Falha de escrita só vira log.
func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if corpo == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(corpo); err != nil {
		log.Warn("falha ao escrever resposta JSON", "erro", err)
	}
}

// writeError devolve o envelope de erro padrão.
func writeError(w http.ResponseWriter, log *slog.Logger, status int, codigo, mensagem string) {
	writeJSON(w, log, status, errorEnvelope{Error: errorBody{Code: codigo, Message: mensagem}})
}

// writeInternalError registra o erro real e devolve uma resposta genérica.
func writeInternalError(w http.ResponseWriter, log *slog.Logger, r *http.Request, err error) {
	log.ErrorContext(r.Context(), "erro interno",
		"metodo", r.Method, "rota", requestLogRoute(r), "erro", err)
	writeError(w, log, http.StatusInternalServerError, CodeInternal,
		"Erro interno. Tente novamente; se persistir, contate o suporte de TI.")
}
