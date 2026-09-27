package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

// settingsFake guarda a configuração em memória, preservando a credencial como
// o store real faz.
type settingsFake struct {
	mu                sync.Mutex
	config            settings.Orthanc
	credencial        string
	configurado       bool
	suportaCredencial bool
	revision          time.Time
	connectionErr     error
	recordErr         error
}

func novoSettingsFake() *settingsFake {
	return &settingsFake{suportaCredencial: true}
}

func (s *settingsFake) Orthanc(context.Context) (settings.Orthanc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.configurado {
		return settings.Orthanc{}, settings.ErrNaoConfigurado
	}
	config := s.config
	config.HasCredential = s.credencial != ""
	return config, nil
}

func (s *settingsFake) SalvarOrthanc(_ context.Context, config settings.Orthanc, credencial *string, _ uuid.UUID) (settings.Orthanc, error) {
	if err := config.Validate(); err != nil {
		return settings.Orthanc{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case credencial == nil:
		// preserva
	case *credencial == "":
		s.credencial = ""
	default:
		if !s.suportaCredencial {
			return settings.Orthanc{}, settings.ErrSemChaveMestra
		}
		s.credencial = *credencial
	}

	config.Status = settings.StatusNaoVerificado
	config.LastCheckedAt = nil
	s.revision = time.Now()
	s.config = config
	s.configurado = true
	config.HasCredential = s.credencial != ""
	config.Status = settings.StatusNaoVerificado
	return config, nil
}

func (s *settingsFake) SuportaCredencial() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.suportaCredencial
}

func (s *settingsFake) credencialGuardada() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.credencial
}

type corpoOrthanc struct {
	Configured              bool   `json:"configured"`
	Name                    string `json:"name"`
	BaseURL                 string `json:"baseUrl"`
	Username                string `json:"username"`
	DICOMWebPath            string `json:"dicomWebPath"`
	TimeoutSeconds          int    `json:"timeoutSeconds"`
	VerifyTLS               bool   `json:"verifyTls"`
	CredentialConfigured    bool   `json:"credentialConfigured"`
	CredentialSupported     bool   `json:"credentialSupported"`
	Status                  string `json:"status"`
	ConnectionTestAvailable bool   `json:"connectionTestAvailable"`
}

func decodificarOrthanc(t *testing.T, corpo string) corpoOrthanc {
	t.Helper()
	var resposta corpoOrthanc
	if err := json.Unmarshal([]byte(corpo), &resposta); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	return resposta
}

// cenarioAdmin devolve um cenário já autenticado como ADMIN.
func cenarioAdmin(t *testing.T) *cenario {
	t.Helper()
	c := montarCenario(t, opcoesCenario{})
	c.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	c.aquecerCSRF(t)
	if resposta := c.login(t, "admin.teste", senhaTeste); resposta.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", resposta.StatusCode)
	}
	return c
}

func TestGetOrthancSettingsSemConfiguracao(t *testing.T) {
	c := cenarioAdmin(t)

	resposta := c.requisitar(t, http.MethodGet, "/api/admin/settings/orthanc", nil)
	if resposta.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", resposta.StatusCode)
	}
	corpo := decodificarOrthanc(t, lerCorpo(t, resposta))
	if corpo.Configured {
		t.Error("sem configuração salva, configured deveria ser falso")
	}
	if corpo.TimeoutSeconds != 10 || !corpo.VerifyTLS {
		t.Errorf("padrões inesperados: timeout=%d verifyTls=%v", corpo.TimeoutSeconds, corpo.VerifyTLS)
	}
	if corpo.ConnectionTestAvailable {
		t.Error("o teste de conexão ainda não existe: deveria vir falso")
	}
}

func TestPutOrthancSettingsSalvaEPreservaCredencial(t *testing.T) {
	c := cenarioAdmin(t)

	primeira := map[string]any{
		"name":           "Orthanc Principal",
		"baseUrl":        "http://orthanc.interno.invalid:8042",
		"username":       "pacs-web",
		"dicomWebPath":   "/dicom-web",
		"timeoutSeconds": 15,
		"verifyTls":      true,
		"credential":     "credencial-de-teste",
	}
	resposta := c.requisitar(t, http.MethodPut, "/api/admin/settings/orthanc", primeira)
	if resposta.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", resposta.StatusCode)
	}
	corpo := lerCorpo(t, resposta)
	if strings.Contains(corpo, "credencial-de-teste") {
		t.Fatalf("a resposta não pode conter a credencial: %s", corpo)
	}
	salva := decodificarOrthanc(t, corpo)
	if !salva.Configured || !salva.CredentialConfigured {
		t.Errorf("configuração deveria constar salva e com credencial: %+v", salva)
	}
	if c.settings.credencialGuardada() != "credencial-de-teste" {
		t.Error("a credencial deveria ter sido guardada")
	}

	// Salvar sem o campo credential não pode apagar a credencial existente.
	segunda := map[string]any{
		"name":           "Orthanc Principal",
		"baseUrl":        "http://orthanc.interno.invalid:8042",
		"username":       "pacs-web",
		"dicomWebPath":   "/dicom-web",
		"timeoutSeconds": 20,
		"verifyTls":      false,
	}
	resposta = c.requisitar(t, http.MethodPut, "/api/admin/settings/orthanc", segunda)
	if resposta.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", resposta.StatusCode)
	}
	depois := decodificarOrthanc(t, lerCorpo(t, resposta))
	if !depois.CredentialConfigured {
		t.Error("salvar outras configurações não pode remover a credencial")
	}
	if depois.TimeoutSeconds != 20 || depois.VerifyTLS {
		t.Errorf("alterações não aplicadas: %+v", depois)
	}
	if c.settings.credencialGuardada() != "credencial-de-teste" {
		t.Error("a credencial guardada mudou sem ser pedido")
	}

	// String vazia remove.
	terceira := map[string]any{
		"name":           "Orthanc Principal",
		"baseUrl":        "http://orthanc.interno.invalid:8042",
		"timeoutSeconds": 20,
		"verifyTls":      false,
		"credential":     "",
	}
	resposta = c.requisitar(t, http.MethodPut, "/api/admin/settings/orthanc", terceira)
	if resposta.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", resposta.StatusCode)
	}
	if decodificarOrthanc(t, lerCorpo(t, resposta)).CredentialConfigured {
		t.Error("credencial vazia deveria remover a credencial")
	}
	if c.settings.credencialGuardada() != "" {
		t.Error("a credencial deveria ter sido removida")
	}
}

func TestPutOrthancSettingsValidacao(t *testing.T) {
	casos := map[string]map[string]any{
		"sem nome": {
			"name": "", "baseUrl": "http://orthanc.interno.invalid", "timeoutSeconds": 10,
		},
		"url sem esquema": {
			"name": "Orthanc", "baseUrl": "orthanc.interno.invalid", "timeoutSeconds": 10,
		},
		"credencial na url": {
			"name": "Orthanc", "baseUrl": "http://usuario:senha@orthanc.interno.invalid", "timeoutSeconds": 10,
		},
		"timeout absurdo": {
			"name": "Orthanc", "baseUrl": "http://orthanc.interno.invalid", "timeoutSeconds": 5000,
		},
		"campo desconhecido": {
			"name": "Orthanc", "baseUrl": "http://orthanc.interno.invalid", "timeoutSeconds": 10, "password": "x",
		},
	}

	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			c := cenarioAdmin(t)
			resposta := c.requisitar(t, http.MethodPut, "/api/admin/settings/orthanc", corpo)
			if resposta.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, esperado 400", resposta.StatusCode)
			}
			if codigo := codigoDeErro(t, resposta); codigo != httpapiCodeInvalidRequest {
				t.Errorf("código = %q, esperado %q", codigo, httpapiCodeInvalidRequest)
			}
		})
	}
}

func TestOrthancSettingsExigemAdmin(t *testing.T) {
	for _, role := range []user.Role{user.RoleGestor, user.RoleMedico} {
		t.Run(string(role), func(t *testing.T) {
			c := montarCenario(t, opcoesCenario{})
			c.adicionarUsuario(t, "usuario.teste", role, nil)
			c.aquecerCSRF(t)
			if resposta := c.login(t, "usuario.teste", senhaTeste); resposta.StatusCode != http.StatusOK {
				t.Fatalf("login = %d", resposta.StatusCode)
			}

			leitura := c.requisitar(t, http.MethodGet, "/api/admin/settings/orthanc", nil)
			if leitura.StatusCode != http.StatusForbidden {
				t.Errorf("GET = %d, esperado 403", leitura.StatusCode)
			}

			escrita := c.requisitar(t, http.MethodPut, "/api/admin/settings/orthanc", map[string]any{
				"name": "Orthanc", "baseUrl": "http://orthanc.interno.invalid", "timeoutSeconds": 10,
			})
			if escrita.StatusCode != http.StatusForbidden {
				t.Errorf("PUT = %d, esperado 403", escrita.StatusCode)
			}
		})
	}
}

func TestOrthancSettingsSemSessao(t *testing.T) {
	c := montarCenario(t, opcoesCenario{})
	resposta := c.requisitar(t, http.MethodGet, "/api/admin/settings/orthanc", nil)
	if resposta.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", resposta.StatusCode)
	}
}

func TestPutOrthancSettingsSemChaveMestra(t *testing.T) {
	c := cenarioAdmin(t)
	c.settings.suportaCredencial = false

	resposta := c.requisitar(t, http.MethodPut, "/api/admin/settings/orthanc", map[string]any{
		"name": "Orthanc", "baseUrl": "http://orthanc.interno.invalid",
		"timeoutSeconds": 10, "credential": "credencial-de-teste",
	})
	if resposta.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, esperado 503", resposta.StatusCode)
	}
	corpo := lerCorpo(t, resposta)
	if strings.Contains(corpo, "credencial-de-teste") || strings.Contains(strings.ToUpper(corpo), "PACS_MASTER_KEY") {
		t.Errorf("a resposta não pode expor credencial nem o nome da variável: %s", corpo)
	}
}

func TestAlteracaoDeConfiguracaoEAuditada(t *testing.T) {
	c := cenarioAdmin(t)

	resposta := c.requisitar(t, http.MethodPut, "/api/admin/settings/orthanc", map[string]any{
		"name": "Orthanc Principal", "baseUrl": "http://orthanc.interno.invalid",
		"timeoutSeconds": 10, "verifyTls": true, "credential": "credencial-de-teste",
	})
	if resposta.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resposta.StatusCode)
	}

	var encontrado *audit.Entry
	for _, entrada := range c.auditoria.Entradas() {
		if entrada.Event == audit.EventOrthancSettingsChanged {
			copia := entrada
			encontrado = &copia
		}
	}
	if encontrado == nil {
		t.Fatalf("evento ORTHANC_SETTINGS_CHANGED não foi registrado; eventos: %v", c.auditoria.Eventos())
	}
	if strings.Contains(encontrado.Detail, "credencial-de-teste") {
		t.Error("a auditoria não pode conter o valor da credencial")
	}
	if !strings.Contains(encontrado.Detail, "credencial alterada") {
		t.Errorf("detalhe = %q, deveria indicar a alteração da credencial", encontrado.Detail)
	}
}

func (s *settingsFake) OrthancConnection(context.Context) (settings.ConnectionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connectionErr != nil {
		return settings.ConnectionSnapshot{}, s.connectionErr
	}
	if !s.configurado {
		return settings.ConnectionSnapshot{}, settings.ErrNaoConfigurado
	}
	return settings.ConnectionSnapshot{Config: s.config, Credential: s.credencial, Revision: s.revision}, nil
}
func (s *settingsFake) RecordOrthancTest(_ context.Context, revision time.Time, status settings.StatusConexao, checkedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recordErr != nil {
		return s.recordErr
	}
	if !revision.Equal(s.revision) {
		return settings.ErrConfiguracaoAlterada
	}
	s.config.Status = status
	s.config.LastCheckedAt = &checkedAt
	return nil
}
