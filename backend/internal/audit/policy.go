package audit

import (
	"context"
	"net"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Closed vocabulary; no map[string]any or free-form JSON enters the audit log.
var Events = []Event{
	EventUserCreated, EventUserUpdated, EventUserActivated, EventUserDeactivated,
	EventUserAccessRenewed, EventUserPasswordReset, EventUserPasswordChanged, EventUserUnitsChanged,
	EventUnitCreated, EventUnitUpdated, EventUnitActivated, EventUnitDeactivated,
	EventLoginSuccess, EventLoginFailure, EventLogout, EventOrthancSettingsChanged, EventOrthancConnectionTested,
}

func Known(e Event) bool {
	for _, v := range Events {
		if e == v {
			return true
		}
	}
	return false
}
func Category(e Event) string {
	switch {
	case strings.HasPrefix(string(e), "USER_"):
		return "users"
	case strings.HasPrefix(string(e), "UNIT_"):
		return "units"
	case e == EventOrthancSettingsChanged || e == EventOrthancConnectionTested:
		return "settings"
	case e == EventLoginSuccess || e == EventLoginFailure || e == EventLogout:
		return "auth"
	default:
		return "unknown"
	}
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

func SafeDetail(e Event, detail string) string {
	switch Category(e) {
	case "users":
		raw := strings.TrimPrefix(detail, "target_user_id=")
		if id, err := uuid.Parse(raw); err == nil && id != uuid.Nil && id.String() == raw && detail == "target_user_id="+raw {
			return detail
		}
	case "units":
		for _, value := range []string{"unidade criada", "nome alterado", "estado ativo alterado"} {
			if detail == value {
				return value
			}
		}
	case "auth":
		allowed := map[Event][]string{EventLoginSuccess: {"sessão iniciada"}, EventLogout: {"encerrada pelo usuário"}, EventLoginFailure: {"usuário inexistente", "senha incorreta", "conta desativada", "validade de acesso expirada"}}
		for _, value := range allowed[e] {
			if detail == value {
				return value
			}
		}
	case "settings":
		if e == EventOrthancSettingsChanged {
			if !strings.HasPrefix(detail, "PACS/Orthanc: ") {
				return ""
			}
			for _, part := range strings.Split(strings.TrimPrefix(detail, "PACS/Orthanc: "), ", ") {
				switch part {
				case "configuração criada", "configuração atualizada", "nome", "URL", "usuário", "endpoint DICOMweb", "timeout", "verificação TLS", "credencial removida", "credencial alterada", "sem alteração efetiva":
				default:
					return ""
				}
			}
			if len(detail) <= 300 {
				return detail
			}
		} else {
			for _, value := range []string{"connected", "configuration_error", "timeout", "canceled", "dns", "connection_refused", "tls", "unauthorized", "forbidden", "redirect", "invalid_response", "not_orthanc", "unavailable", "invalid_target", "blocked_target", "upstream_http", "not_found", "too_large"} {
				if detail == "PACS/Orthanc: "+value {
					return detail
				}
			}
		}
	}
	return ""
}

type Actor struct {
	ID               uuid.UUID
	Username, Origin string
}
type actorKey struct{}

// WithActor is set only from the authenticated server context, never request JSON.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}
func FromContext(ctx context.Context, event Event) Entry {
	a, _ := ctx.Value(actorKey{}).(Actor)
	e := Entry{Event: event, ActorUsername: a.Username, Origin: a.Origin}
	if a.ID != uuid.Nil {
		e.ActorUserID = &a.ID
	}
	return e
}

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insert(ctx context.Context, db executor, e Entry) error {
	if !Known(e.Event) {
		return ErrInvalid
	}
	if (Category(e.Event) == "users" || Category(e.Event) == "units" || e.Event == EventOrthancSettingsChanged) && (e.ActorUserID == nil || *e.ActorUserID == uuid.Nil) {
		return ErrInvalid
	}
	if Category(e.Event) == "units" && (e.UnitID == nil || *e.UnitID == uuid.Nil) {
		return ErrInvalid
	}
	if Category(e.Event) == "users" && e.Detail == "" {
		return ErrInvalid
	}
	detail := SafeDetail(e.Event, e.Detail)
	if e.Detail != "" && detail == "" {
		return ErrInvalid
	}
	// Never store an untrusted login attempt as a username; it may be a pasted secret.
	username := ""
	if e.ActorUserID != nil && *e.ActorUserID != uuid.Nil && usernamePattern.MatchString(e.ActorUsername) {
		username = e.ActorUsername
	}
	origin := ""
	if ip := net.ParseIP(e.Origin); ip != nil {
		origin = ip.String()
	}
	_, err := db.Exec(ctx, `INSERT INTO audit_events(event,actor_user_id,actor_username,unit_id,detail,origin) VALUES($1,$2,$3,$4,$5,$6)`, string(e.Event), e.ActorUserID, textoOuNil(username), e.UnitID, textoOuNil(detail), textoOuNil(origin))
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
