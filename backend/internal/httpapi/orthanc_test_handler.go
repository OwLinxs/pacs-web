package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
)

type orthancTestResponse struct {
	Success   bool                   `json:"success"`
	Status    settings.StatusConexao `json:"status"`
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	CheckedAt time.Time              `json:"checkedAt"`
}

// Não aceita URL/credencial na requisição: testa exclusivamente o registro salvo.
func (s *Server) handleTestOrthanc(w http.ResponseWriter, r *http.Request) {
	user, _ := UsuarioDoContexto(r.Context())
	outcome := "configuration_error"
	defer func() {
		// A categoria é local e fechada; nenhum erro upstream vai para log/auditoria.
		s.log.InfoContext(r.Context(), "teste de conexão Orthanc", "resultado", outcome)
		if s.auditoria != nil {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()
			_ = s.auditoria.Record(ctx, audit.Entry{
				Event: audit.EventOrthancConnectionTested, ActorUserID: &user.ID,
				ActorUsername: user.Username, UnitID: user.UnitID,
				Detail: "PACS/Orthanc: " + outcome, Origin: s.ipDoPedido(r),
			})
		}
	}()
	// Nenhum parâmetro de destino é aceito, nem no corpo nem na query string.
	if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "O teste utiliza somente a configuração salva; envie a requisição sem parâmetros.")
		return
	}
	if s.orthanc == nil {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Teste de conexão indisponível.")
		return
	}
	readCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	snapshot, err := s.settings.OrthancConnection(readCtx)
	cancel()
	if errors.Is(err, settings.ErrNaoConfigurado) {
		writeError(w, s.log, http.StatusConflict, CodeConflict, "Salve a configuração do PACS antes de testar a conexão.")
		return
	}
	if err != nil {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Não foi possível preparar a configuração e a credencial do PACS. Contate o suporte de TI.")
		return
	}
	cfg := snapshot.Config
	if cfg.TimeoutSeconds < 1 || cfg.TimeoutSeconds > 120 {
		writeError(w, s.log, http.StatusConflict, CodeInvalidRequest, "Revise e salve o timeout da conexão antes de testar.")
		return
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	// Mantém os demais endpoints com seus timeouts atuais.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout + 8*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Não foi possível iniciar o teste de conexão.")
		return
	}
	err = s.orthanc.TestSystem(r.Context(), orthanc.Config{
		BaseURL: cfg.BaseURL, Username: cfg.Username, Credential: snapshot.Credential,
		Timeout: timeout, VerifyTLS: cfg.VerifyTLS,
	})
	result := orthancTestResponse{
		Success: err == nil, Status: settings.StatusConectado, Code: "connected",
		Message: "Conexão com o PACS estabelecida com sucesso.", CheckedAt: s.now().UTC(),
	}
	if err != nil {
		result.Status = settings.StatusFalha
		result.Code, result.Message = orthancFailureMessage(err)
	}
	outcome = result.Code
	writeCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	err = s.settings.RecordOrthancTest(writeCtx, snapshot.Revision, result.Status, result.CheckedAt)
	cancel()
	if errors.Is(err, settings.ErrConfiguracaoAlterada) {
		writeError(w, s.log, http.StatusConflict, CodeConflict, "A configuração mudou durante o teste. Recarregue e teste novamente.")
		return
	}
	if err != nil {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "O teste terminou, mas não foi possível registrar o resultado. Tente novamente.")
		return
	}
	// HTTP 200 significa teste concluído; success/status informam conectividade.
	writeJSON(w, s.log, http.StatusOK, result)
}

func orthancFailureMessage(err error) (string, string) {
	var failure orthanc.Failure
	_ = errors.As(err, &failure)
	switch failure {
	case orthanc.Timeout:
		return string(failure), "O PACS não respondeu dentro do tempo configurado."
	case orthanc.Canceled:
		return string(failure), "O teste de conexão foi interrompido."
	case orthanc.DNS:
		return string(failure), "Não foi possível localizar o servidor PACS. Verifique o endereço cadastrado."
	case orthanc.Refused:
		return string(failure), "O servidor PACS recusou a conexão. Verifique a disponibilidade do serviço."
	case orthanc.TLS:
		return string(failure), "Não foi possível validar a conexão TLS do PACS. Verifique o certificado e o endereço cadastrado."
	case orthanc.Unauthorized:
		return string(failure), "O PACS recusou a autenticação. Revise o usuário e a credencial cadastrados."
	case orthanc.Forbidden:
		return string(failure), "O PACS não autorizou a consulta de verificação. Revise as permissões do acesso."
	case orthanc.Redirect:
		return string(failure), "O PACS respondeu com redirecionamento, que não é permitido. Cadastre o endereço direto do serviço."
	case orthanc.InvalidTarget, orthanc.BlockedTarget:
		return string(failure), "O endereço cadastrado não é permitido para o teste de conexão. Revise a configuração."
	case orthanc.InvalidResponse, orthanc.NotOrthanc:
		return string(failure), "O destino não apresentou uma resposta válida de um servidor Orthanc."
	case orthanc.Upstream:
		return string(failure), "O PACS retornou um erro HTTP ou uma resposta inesperada."
	default:
		return string(orthanc.Unavailable), "Não foi possível estabelecer comunicação com o PACS."
	}
}
