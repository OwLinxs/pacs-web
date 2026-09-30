package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/secrets"
)

// ErrSemChaveMestra indica tentativa de gravar credencial sem PACS_MASTER_KEY.
var ErrSemChaveMestra = errors.New("PACS_MASTER_KEY não configurada: não é possível guardar credencial")

// registroOrthanc é o formato gravado em app_settings.value.
//
// A credencial vai cifrada neste JSON. Mesmo com acesso de leitura ao banco,
// sem a chave mestra — que vive só no ambiente do processo — ela não se abre.
type registroOrthanc struct {
	Name             string        `json:"name"`
	BaseURL          string        `json:"baseUrl"`
	Username         string        `json:"username,omitempty"`
	DICOMWebPath     string        `json:"dicomWebPath,omitempty"`
	TimeoutSeconds   int           `json:"timeoutSeconds"`
	VerifyTLS        bool          `json:"verifyTls"`
	CredentialSealed string        `json:"credentialSealed,omitempty"`
	Status           StatusConexao `json:"status"`
	LastCheckedAt    *time.Time    `json:"lastCheckedAt,omitempty"`
}

// Store lê e grava a configuração em app_settings.
type Store struct {
	pool *pgxpool.Pool
	// cifra é nil quando PACS_MASTER_KEY não está configurada; nesse caso a
	// configuração ainda pode ser salva, mas sem credencial.
	cifra *secrets.Cipher
}

// NewStore devolve um Store. `cifra` pode ser nil.
func NewStore(pool *pgxpool.Pool, cifra *secrets.Cipher) *Store {
	return &Store{pool: pool, cifra: cifra}
}

// SuportaCredencial indica se há chave mestra para cifrar credenciais.
func (s *Store) SuportaCredencial() bool { return s.cifra != nil }

// Orthanc devolve a configuração salva. Retorna ErrNaoConfigurado quando ainda
// não existe nenhuma.
func (s *Store) Orthanc(ctx context.Context) (Orthanc, error) {
	registro, err := s.lerRegistro(ctx)
	if err != nil {
		return Orthanc{}, err
	}
	return Orthanc{
		Name:           registro.Name,
		BaseURL:        registro.BaseURL,
		Username:       registro.Username,
		DICOMWebPath:   registro.DICOMWebPath,
		TimeoutSeconds: registro.TimeoutSeconds,
		VerifyTLS:      registro.VerifyTLS,
		HasCredential:  registro.CredentialSealed != "",
		Status:         registro.Status,
		LastCheckedAt:  registro.LastCheckedAt,
	}, nil
}

// SalvarOrthanc grava a configuração.
//
// `credencial` nil preserva a credencial existente — salvar as outras
// configurações nunca apaga a credencial. Ponteiro para string vazia remove a
// credencial guardada.
func (s *Store) SalvarOrthanc(ctx context.Context, config Orthanc, credencial *string, autor uuid.UUID) (Orthanc, error) {
	if err := config.Validate(); err != nil {
		return Orthanc{}, err
	}

	anterior, err := s.lerRegistro(ctx)
	if err != nil && !errors.Is(err, ErrNaoConfigurado) {
		return Orthanc{}, err
	}

	novo := registroOrthanc{
		Name:             config.Name,
		BaseURL:          config.BaseURL,
		Username:         config.Username,
		DICOMWebPath:     config.DICOMWebPath,
		TimeoutSeconds:   config.TimeoutSeconds,
		VerifyTLS:        config.VerifyTLS,
		CredentialSealed: anterior.CredentialSealed,
		// Alterar a configuração invalida a verificação anterior.
		Status: StatusNaoVerificado,
	}

	switch {
	case credencial == nil:
		// Mantém o que já estava guardado.
	case *credencial == "":
		novo.CredentialSealed = ""
	default:
		if s.cifra == nil {
			return Orthanc{}, ErrSemChaveMestra
		}
		cifrada, err := s.cifra.Seal(*credencial, contextoCredencial)
		if err != nil {
			return Orthanc{}, fmt.Errorf("cifrar credencial: %w", err)
		}
		novo.CredentialSealed = cifrada
	}

	bruto, err := json.Marshal(novo)
	if err != nil {
		return Orthanc{}, fmt.Errorf("serializar configuração: %w", err)
	}

	const gravar = `
		INSERT INTO app_settings (key, value, updated_at, updated_by)
		VALUES ($1, $2, now(), $3)
		ON CONFLICT (key) DO UPDATE
		   SET value = excluded.value, updated_at = now(), updated_by = excluded.updated_by`
	var autorOuNil *uuid.UUID
	if autor != uuid.Nil {
		autorOuNil = &autor
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Orthanc{}, audit.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, gravar, ChaveOrthanc, bruto, autorOuNil); err != nil {
		return Orthanc{}, audit.ErrUnavailable
	}
	entry := audit.FromContext(ctx, audit.EventOrthancSettingsChanged)
	entry.ActorUserID = autorOuNil
	entry.Detail = "PACS/Orthanc: configuração atualizada"
	if audit.RecordTx(ctx, tx, entry) != nil {
		return Orthanc{}, audit.ErrUnavailable
	}
	if tx.Commit(ctx) != nil {
		return Orthanc{}, audit.ErrUnavailable
	}

	config.HasCredential = novo.CredentialSealed != ""
	config.Status = novo.Status
	config.LastCheckedAt = nil
	return config, nil
}

// CredencialOrthanc devolve a credencial em claro.
//
// Só o backend chama isto, e somente quando for efetivamente falar com o
// Orthanc. O valor nunca é serializado para a API nem registrado em log.
func (s *Store) CredencialOrthanc(ctx context.Context) (string, error) {
	registro, err := s.lerRegistro(ctx)
	if err != nil {
		return "", err
	}
	if registro.CredentialSealed == "" {
		return "", nil
	}
	if s.cifra == nil {
		return "", ErrSemChaveMestra
	}
	return s.cifra.Open(registro.CredentialSealed, contextoCredencial)
}

// AtualizadoEm devolve quando a configuração foi gravada por último.
func (s *Store) AtualizadoEm(ctx context.Context) (time.Time, error) {
	const consulta = `SELECT updated_at FROM app_settings WHERE key = $1`
	var quando time.Time
	err := s.pool.QueryRow(ctx, consulta, ChaveOrthanc).Scan(&quando)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNaoConfigurado
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("ler data da configuração: %w", err)
	}
	return quando, nil
}

func (s *Store) lerRegistro(ctx context.Context) (registroOrthanc, error) {
	const consulta = `SELECT value FROM app_settings WHERE key = $1`

	var bruto []byte
	err := s.pool.QueryRow(ctx, consulta, ChaveOrthanc).Scan(&bruto)
	if errors.Is(err, pgx.ErrNoRows) {
		return registroOrthanc{}, ErrNaoConfigurado
	}
	if err != nil {
		return registroOrthanc{}, fmt.Errorf("ler configuração: %w", err)
	}

	var registro registroOrthanc
	if err := json.Unmarshal(bruto, &registro); err != nil {
		return registroOrthanc{}, fmt.Errorf("configuração gravada em formato inválido")
	}
	if registro.Status == "" {
		registro.Status = StatusNaoVerificado
	}
	return registro, nil
}
