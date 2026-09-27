package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
)

// maxCorpoSettings limita o corpo aceito na configuração.
const maxCorpoSettings = 8 << 10

// AuditRecorder grava eventos de auditoria dos fluxos administrativos.
type AuditRecorder interface {
	Record(ctx context.Context, entrada audit.Entry) error
}

// SettingsStore é o que a API precisa da configuração operacional.
type SettingsStore interface {
	Orthanc(ctx context.Context) (settings.Orthanc, error)
	SalvarOrthanc(ctx context.Context, config settings.Orthanc, credencial *string, autor uuid.UUID) (settings.Orthanc, error)
	SuportaCredencial() bool
	OrthancConnection(ctx context.Context) (settings.ConnectionSnapshot, error)
	RecordOrthancTest(ctx context.Context, revision time.Time, status settings.StatusConexao, checkedAt time.Time) error
}

// orthancSettingsResponse é a configuração como o frontend a recebe.
//
// A credencial NUNCA aparece aqui — só a informação de que existe uma guardada.
type orthancSettingsResponse struct {
	Configured           bool   `json:"configured"`
	Name                 string `json:"name"`
	BaseURL              string `json:"baseUrl"`
	Username             string `json:"username"`
	DICOMWebPath         string `json:"dicomWebPath"`
	TimeoutSeconds       int    `json:"timeoutSeconds"`
	VerifyTLS            bool   `json:"verifyTls"`
	CredentialConfigured bool   `json:"credentialConfigured"`
	// CredentialSupported é falso quando PACS_MASTER_KEY não está configurada.
	CredentialSupported bool       `json:"credentialSupported"`
	Status              string     `json:"status"`
	LastCheckedAt       *time.Time `json:"lastCheckedAt"`
	// Só há teste quando existe configuração salva e cliente disponível.
	ConnectionTestAvailable bool `json:"connectionTestAvailable"`
}

// orthancSettingsRequest é o corpo de PUT /api/admin/settings/orthanc.
//
// Credential ausente preserva a credencial guardada; string vazia a remove.
type orthancSettingsRequest struct {
	Name           string  `json:"name"`
	BaseURL        string  `json:"baseUrl"`
	Username       string  `json:"username"`
	DICOMWebPath   string  `json:"dicomWebPath"`
	TimeoutSeconds int     `json:"timeoutSeconds"`
	VerifyTLS      bool    `json:"verifyTls"`
	Credential     *string `json:"credential"`
}

func (s *Server) respostaOrthanc(config settings.Orthanc, configurado bool) orthancSettingsResponse {
	return orthancSettingsResponse{
		Configured:              configurado,
		Name:                    config.Name,
		BaseURL:                 config.BaseURL,
		Username:                config.Username,
		DICOMWebPath:            config.DICOMWebPath,
		TimeoutSeconds:          config.TimeoutSeconds,
		VerifyTLS:               config.VerifyTLS,
		CredentialConfigured:    config.HasCredential,
		CredentialSupported:     s.settings.SuportaCredencial(),
		Status:                  string(config.Status),
		ConnectionTestAvailable: configurado && s.orthanc != nil,
		LastCheckedAt:           config.LastCheckedAt,
	}
}

// handleGetOrthancSettings devolve a configuração atual do PACS.
func (s *Server) handleGetOrthancSettings(w http.ResponseWriter, r *http.Request) {
	config, err := s.settings.Orthanc(r.Context())
	if errors.Is(err, settings.ErrNaoConfigurado) {
		// Sem configuração ainda: devolve os padrões para o formulário.
		writeJSON(w, s.log, http.StatusOK, s.respostaOrthanc(settings.Orthanc{
			TimeoutSeconds: 10,
			VerifyTLS:      true,
			Status:         settings.StatusNaoVerificado,
		}, false))
		return
	}
	if err != nil {
		writeInternalError(w, s.log, r, err)
		return
	}
	writeJSON(w, s.log, http.StatusOK, s.respostaOrthanc(config, true))
}

// handlePutOrthancSettings grava a configuração do PACS.
//
// Guardar a configuração não estabelece conexão alguma: nenhuma requisição é
// feita ao Orthanc nesta etapa.
func (s *Server) handlePutOrthancSettings(w http.ResponseWriter, r *http.Request) {
	usuario, ok := UsuarioDoContexto(r.Context())
	if !ok {
		writeError(w, s.log, http.StatusUnauthorized, CodeUnauthenticated, "Sessão inválida ou expirada.")
		return
	}

	var pedido orthancSettingsRequest
	decodificador := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCorpoSettings))
	decodificador.DisallowUnknownFields()
	if err := decodificador.Decode(&pedido); err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Envie os dados da configuração.")
			return
		}
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Requisição inválida.")
		return
	}

	credencial := pedido.Credential
	if credencial != nil {
		aparada := strings.TrimSpace(*credencial)
		credencial = &aparada
	}

	config := settings.Orthanc{
		Name:           pedido.Name,
		BaseURL:        pedido.BaseURL,
		Username:       pedido.Username,
		DICOMWebPath:   pedido.DICOMWebPath,
		TimeoutSeconds: pedido.TimeoutSeconds,
		VerifyTLS:      pedido.VerifyTLS,
	}

	// Validar aqui deixa claro o que é erro de entrada (400) e o que é falha
	// interna: qualquer erro do store, daqui para baixo, é nosso.
	if err := config.Validate(); err != nil {
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}

	anterior, errAnterior := s.settings.Orthanc(r.Context())
	if errAnterior != nil && !errors.Is(errAnterior, settings.ErrNaoConfigurado) {
		writeInternalError(w, s.log, r, errAnterior)
		return
	}

	salva, err := s.settings.SalvarOrthanc(r.Context(), config, credencial, usuario.ID)
	switch {
	case errors.Is(err, settings.ErrSemChaveMestra):
		s.log.ErrorContext(r.Context(), "tentativa de gravar credencial sem chave mestra")
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable,
			"O servidor não está preparado para guardar credenciais. Contate o suporte de TI.")
		return
	case err != nil:
		writeInternalError(w, s.log, r, err)
		return
	}

	// Auditoria sem valor algum de credencial: só o que mudou.
	s.auditarAlteracaoOrthanc(r, usuario.ID, usuario.Username, anterior, salva, credencial, errors.Is(errAnterior, settings.ErrNaoConfigurado))

	writeJSON(w, s.log, http.StatusOK, s.respostaOrthanc(salva, true))
}

func (s *Server) auditarAlteracaoOrthanc(r *http.Request, autorID uuid.UUID, autorUsername string, anterior, atual settings.Orthanc, credencial *string, primeiraVez bool) {
	if s.auditoria == nil {
		return
	}

	mudancas := make([]string, 0, 6)
	if primeiraVez {
		mudancas = append(mudancas, "configuração criada")
	} else {
		if anterior.Name != atual.Name {
			mudancas = append(mudancas, "nome")
		}
		if anterior.BaseURL != atual.BaseURL {
			mudancas = append(mudancas, "URL")
		}
		if anterior.Username != atual.Username {
			mudancas = append(mudancas, "usuário")
		}
		if anterior.DICOMWebPath != atual.DICOMWebPath {
			mudancas = append(mudancas, "endpoint DICOMweb")
		}
		if anterior.TimeoutSeconds != atual.TimeoutSeconds {
			mudancas = append(mudancas, "timeout")
		}
		if anterior.VerifyTLS != atual.VerifyTLS {
			mudancas = append(mudancas, "verificação TLS")
		}
	}
	switch {
	case credencial == nil:
	case *credencial == "":
		mudancas = append(mudancas, "credencial removida")
	default:
		mudancas = append(mudancas, "credencial alterada")
	}
	if len(mudancas) == 0 {
		mudancas = append(mudancas, "sem alteração efetiva")
	}

	_ = s.auditoria.Record(r.Context(), audit.Entry{
		Event:         audit.EventOrthancSettingsChanged,
		ActorUserID:   &autorID,
		ActorUsername: autorUsername,
		Detail:        "PACS/Orthanc: " + strings.Join(mudancas, ", "),
		Origin:        ipDoPedido(r),
	})
}
