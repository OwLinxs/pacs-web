package settings

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrConfiguracaoAlterada impede atribuir um teste à configuração substituída.
var ErrConfiguracaoAlterada = errors.New("configuração alterada durante o teste")

// ConnectionSnapshot mantém configuração e credencial da mesma leitura.
// Nunca serializar ou registrar em log este objeto.
type ConnectionSnapshot struct {
	Config     Orthanc   `json:"-"`
	Credential string    `json:"-"`
	Revision   time.Time `json:"-"`
}

func (s *Store) OrthancConnection(ctx context.Context) (ConnectionSnapshot, error) {
	var raw []byte
	var snapshot ConnectionSnapshot
	err := s.pool.QueryRow(ctx, `SELECT value, updated_at FROM app_settings WHERE key = $1`, ChaveOrthanc).Scan(&raw, &snapshot.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return snapshot, ErrNaoConfigurado
	}
	if err != nil {
		return snapshot, errors.New("não foi possível ler a conexão")
	}
	var record registroOrthanc
	if json.Unmarshal(raw, &record) != nil {
		return snapshot, errors.New("configuração inválida")
	}
	snapshot.Config = Orthanc{
		Name: record.Name, BaseURL: record.BaseURL, Username: record.Username,
		DICOMWebPath: record.DICOMWebPath, TimeoutSeconds: record.TimeoutSeconds,
		VerifyTLS: record.VerifyTLS, HasCredential: record.CredentialSealed != "",
	}
	if record.CredentialSealed != "" {
		if s.cifra == nil {
			return ConnectionSnapshot{}, ErrSemChaveMestra
		}
		snapshot.Credential, err = s.cifra.Open(record.CredentialSealed, contextoCredencial)
		if err != nil {
			return ConnectionSnapshot{}, err
		}
	}
	return snapshot, nil
}

// RecordOrthancTest altera apenas metadados no JSONB existente, sem migration,
// sem regravar credencial e sem alterar updated_at (revisão da configuração).
func (s *Store) RecordOrthancTest(ctx context.Context, revision time.Time, status StatusConexao, checkedAt time.Time) error {
	if status != StatusConectado && status != StatusFalha {
		return errors.New("status de teste inválido")
	}
	patch, err := json.Marshal(struct {
		Status    StatusConexao `json:"status"`
		CheckedAt time.Time     `json:"lastCheckedAt"`
	}{status, checkedAt.UTC()})
	if err != nil {
		return errors.New("metadados de teste inválidos")
	}
	result, err := s.pool.Exec(ctx, `UPDATE app_settings SET value = value || $1::jsonb WHERE key = $2 AND updated_at = $3`, patch, ChaveOrthanc, revision)
	if err != nil {
		return errors.New("não foi possível registrar a verificação")
	}
	if result.RowsAffected() == 0 {
		return ErrConfiguracaoAlterada
	}
	return nil
}
