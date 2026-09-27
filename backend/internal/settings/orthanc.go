// Package settings guarda a configuração operacional administrável pela
// interface. Nesta etapa, apenas a conexão com o Orthanc.
//
// IMPORTANTE: guardar a configuração não estabelece conexão. Nenhum código deste
// pacote fala com o Orthanc; o cliente HTTP fica em internal/orthanc e só é
// acionado pelo teste explícito do ADMIN.
package settings

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// ChaveOrthanc é a chave da configuração do Orthanc em app_settings.
const ChaveOrthanc = "orthanc"

// contextoCredencial identifica o propósito do valor cifrado, para que uma
// credencial não possa ser reaproveitada em outro contexto.
const contextoCredencial = "settings:orthanc:credential"

// Limites de validação.
const (
	timeoutMinimo = 1
	timeoutMaximo = 120
	maxNome       = 120
	maxURL        = 500
	maxUsuario    = 120
	maxCaminho    = 200
)

// ErrNaoConfigurado indica que ainda não há configuração salva.
var ErrNaoConfigurado = errors.New("configuração do Orthanc não definida")

// StatusConexao é o resultado da última verificação de conectividade.
type StatusConexao string

const (
	// StatusNaoVerificado indica configuração ainda não testada.
	StatusNaoVerificado StatusConexao = "unverified"
	// StatusConectado indica sucesso no último teste explícito.
	StatusConectado StatusConexao = "connected"
	// StatusFalha indica falha no último teste explícito.
	StatusFalha StatusConexao = "failed"
)

// Orthanc é a configuração da conexão com o servidor PACS.
//
// A credencial NÃO está aqui: ela é cifrada e guardada à parte, e nunca sai do
// backend. Este struct só sabe se existe uma credencial configurada.
type Orthanc struct {
	// Name é um rótulo para a conexão, ex.: "Orthanc Principal".
	Name string
	// BaseURL é a URL base do Orthanc, acessível pelo backend.
	BaseURL string
	// Username fica vazio quando o Orthanc não exige autenticação.
	Username string
	// DICOMWebPath é o caminho do endpoint DICOMweb, ex.: "/dicom-web".
	DICOMWebPath string
	// TimeoutSeconds é o tempo máximo das chamadas ao Orthanc.
	TimeoutSeconds int
	// VerifyTLS controla a verificação do certificado quando a URL é HTTPS.
	VerifyTLS bool
	// HasCredential indica que existe credencial cifrada guardada.
	HasCredential bool
	// Status é o resultado da última verificação de conectividade.
	Status        StatusConexao
	LastCheckedAt *time.Time
}

// Normalize limpa espaços e aplica os padrões.
func (o *Orthanc) Normalize() {
	o.Name = strings.TrimSpace(o.Name)
	o.BaseURL = strings.TrimRight(strings.TrimSpace(o.BaseURL), "/")
	o.Username = strings.TrimSpace(o.Username)
	o.DICOMWebPath = strings.TrimRight(strings.TrimSpace(o.DICOMWebPath), "/")
	if o.TimeoutSeconds == 0 {
		o.TimeoutSeconds = 10
	}
	if o.Status == "" {
		o.Status = StatusNaoVerificado
	}
}

// Validate confere a configuração informada pelo administrador.
//
// A URL é validada com rigor de propósito: é um endereço que o backend vai
// acessar, então recebê-la de um formulário é risco de SSRF. Aqui rejeitamos o
// que é claramente inválido ou perigoso; a política de destino permitido é
// aplicada no cliente HTTP imediatamente antes da conexão.
func (o *Orthanc) Validate() error {
	o.Normalize()

	if o.Name == "" {
		return errors.New("informe um nome para a conexão")
	}
	if len([]rune(o.Name)) > maxNome {
		return fmt.Errorf("nome deve ter no máximo %d caracteres", maxNome)
	}

	if o.BaseURL == "" {
		return errors.New("informe a URL do Orthanc")
	}
	if len(o.BaseURL) > maxURL {
		return fmt.Errorf("URL deve ter no máximo %d caracteres", maxURL)
	}
	endereco, err := url.Parse(o.BaseURL)
	if err != nil {
		return errors.New("URL do Orthanc inválida")
	}
	if endereco.Scheme != "http" && endereco.Scheme != "https" {
		return errors.New("a URL deve começar com http:// ou https://")
	}
	if endereco.Host == "" {
		return errors.New("a URL deve incluir o endereço do servidor")
	}
	if endereco.User != nil {
		return errors.New("não coloque usuário ou senha dentro da URL; use os campos de credencial")
	}
	if endereco.RawQuery != "" || endereco.Fragment != "" {
		return errors.New("a URL não deve ter parâmetros de consulta nem fragmento")
	}
	if porta := endereco.Port(); porta != "" {
		if _, err := net.LookupPort("tcp", porta); err != nil {
			return errors.New("porta inválida na URL")
		}
	}

	if len([]rune(o.Username)) > maxUsuario {
		return fmt.Errorf("usuário deve ter no máximo %d caracteres", maxUsuario)
	}

	if o.DICOMWebPath != "" {
		if !strings.HasPrefix(o.DICOMWebPath, "/") {
			return errors.New("o endpoint DICOMweb deve começar com /")
		}
		if strings.Contains(o.DICOMWebPath, "..") {
			return errors.New("endpoint DICOMweb inválido")
		}
		if len(o.DICOMWebPath) > maxCaminho {
			return fmt.Errorf("endpoint DICOMweb deve ter no máximo %d caracteres", maxCaminho)
		}
	}

	if o.TimeoutSeconds < timeoutMinimo || o.TimeoutSeconds > timeoutMaximo {
		return fmt.Errorf("timeout deve estar entre %d e %d segundos", timeoutMinimo, timeoutMaximo)
	}

	return nil
}
