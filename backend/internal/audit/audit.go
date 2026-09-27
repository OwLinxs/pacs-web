// Package audit grava a trilha de auditoria da aplicação.
//
// Regra do projeto: nunca entram aqui senha, hash, token de sessão, cookie,
// header de autorização, API key, secret nem dado identificável de paciente.
// Quem chama é responsável por respeitar isso no campo Detail.
package audit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event identifica o tipo de evento auditado.
type Event string

// Eventos implementados nesta etapa.
const (
	EventLoginSuccess Event = "LOGIN_SUCCESS"
	EventLoginFailure Event = "LOGIN_FAILURE"
	EventLogout       Event = "LOGOUT"

	// EventOrthancSettingsChanged registra alteração na configuração do PACS.
	EventOrthancSettingsChanged  Event = "ORTHANC_SETTINGS_CHANGED"
	EventOrthancConnectionTested Event = "ORTHANC_CONNECTION_TESTED"
)

// maxDetail limita o texto livre gravado.
const maxDetail = 500

// Entry é um evento a ser registrado.
type Entry struct {
	Event Event
	// ActorUserID fica nil quando não há usuário identificado — por exemplo em
	// LOGIN_FAILURE com username inexistente.
	ActorUserID *uuid.UUID
	// ActorUsername é o username tentado, preservado mesmo sem usuário.
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

// Record grava o evento. Falha de auditoria não derruba a operação em curso:
// é registrada no log da aplicação e o erro é devolvido para quem quiser tratar.
func (r *Recorder) Record(ctx context.Context, entrada Entry) error {
	const inserir = `
		INSERT INTO audit_events (event, actor_user_id, actor_username, unit_id, detail, origin)
		VALUES ($1, $2, $3, $4, $5, $6)`

	detalhe := truncar(entrada.Detail, maxDetail)
	_, err := r.pool.Exec(ctx, inserir,
		string(entrada.Event), entrada.ActorUserID, textoOuNil(entrada.ActorUsername),
		entrada.UnitID, textoOuNil(detalhe), textoOuNil(entrada.Origin),
	)
	if err != nil {
		r.log.ErrorContext(ctx, "falha ao gravar auditoria", "event", entrada.Event, "erro", err)
		return fmt.Errorf("gravar auditoria: %w", err)
	}
	return nil
}

// Purge remove eventos anteriores ao corte informado. Não é chamado
// automaticamente: retenção é decisão administrativa.
func (r *Recorder) Purge(ctx context.Context, anteriorA time.Time) (int64, error) {
	const apagar = `DELETE FROM audit_events WHERE occurred_at < $1`
	etiqueta, err := r.pool.Exec(ctx, apagar, anteriorA)
	if err != nil {
		return 0, fmt.Errorf("expurgar auditoria: %w", err)
	}
	return etiqueta.RowsAffected(), nil
}

func truncar(texto string, limite int) string {
	if len(texto) <= limite {
		return texto
	}
	return texto[:limite]
}

func textoOuNil(texto string) *string {
	if texto == "" {
		return nil
	}
	return &texto
}
