// Package audit grava a trilha de auditoria da aplicação.
//
// Regra do projeto: nunca entram aqui senha, hash, token de sessão, cookie,
// header de autorização, API key, secret nem dado identificável de paciente.
// Detail é validado por allowlist de evento, tanto na escrita quanto na leitura.
package audit

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event identifica o tipo de evento auditado.
type Event string

// Eventos implementados nesta etapa.
const (
	EventUserCreated         Event = "USER_CREATED"
	EventUserUpdated         Event = "USER_UPDATED"
	EventUserActivated       Event = "USER_ACTIVATED"
	EventUserDeactivated     Event = "USER_DEACTIVATED"
	EventUserAccessRenewed   Event = "USER_ACCESS_RENEWED"
	EventUserPasswordReset   Event = "USER_PASSWORD_RESET"
	EventUserPasswordChanged Event = "USER_PASSWORD_CHANGED"
	EventUserUnitsChanged    Event = "USER_UNITS_CHANGED"

	EventUnitCreated     Event = "UNIT_CREATED"
	EventUnitUpdated     Event = "UNIT_UPDATED"
	EventUnitActivated   Event = "UNIT_ACTIVATED"
	EventUnitDeactivated Event = "UNIT_DEACTIVATED"

	EventLoginSuccess Event = "LOGIN_SUCCESS"
	EventLoginFailure Event = "LOGIN_FAILURE"
	EventLogout       Event = "LOGOUT"

	// EventOrthancSettingsChanged registra alteração na configuração do PACS.
	EventOrthancSettingsChanged  Event = "ORTHANC_SETTINGS_CHANGED"
	EventOrthancConnectionTested Event = "ORTHANC_CONNECTION_TESTED"
)

// Entry é um evento a ser registrado.
type Entry struct {
	Event Event
	// ActorUserID fica nil quando não há usuário identificado — por exemplo em
	// LOGIN_FAILURE com username inexistente.
	ActorUserID *uuid.UUID
	// ActorUsername é aceito apenas para ator identificado; tentativa anônima é omitida.
	ActorUsername string
	UnitID        *uuid.UUID
	// Detail é texto curto e sanitizado sobre o evento.
	Detail string
	// Origin identifica a origem da requisição (IP, e estação quando houver).
	Origin string
}

// Recorder grava eventos no PostgreSQL.
type Recorder struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewRecorder devolve um Recorder sobre o pool informado.
func NewRecorder(pool *pgxpool.Pool, log *slog.Logger) *Recorder {
	return &Recorder{pool: pool, log: log}
}

// Record é best-effort para eventos externos à transação administrativa.
// Não registra o erro SQL: ele pode conter valores rejeitados pelo banco.
func (r *Recorder) Record(ctx context.Context, entry Entry) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := insert(ctx, r.pool, entry); err != nil {
		r.log.ErrorContext(ctx, "falha ao gravar auditoria")
		return ErrUnavailable
	}
	return nil
}

var ErrUnavailable = errors.New("auditoria indisponível")
var ErrInvalid = errors.New("parâmetros de auditoria inválidos")

// RecordTx não faz commit: a alteração e todos os eventos pertencem ao chamador.
func RecordTx(ctx context.Context, tx pgx.Tx, entry Entry) error { return insert(ctx, tx, entry) }

func textoOuNil(texto string) *string {
	if texto == "" {
		return nil
	}
	return &texto
}
