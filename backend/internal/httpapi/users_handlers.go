package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
	"github.com/pmfb-saude/pacs-web/backend/internal/useradmin"
)

type UsersService interface {
	List(context.Context, user.User, useradmin.Query, time.Time) ([]useradmin.Record, error)
	Apply(context.Context, user.User, useradmin.Command, string, time.Time) (useradmin.Result, error)
	ChangePassword(context.Context, user.User, []byte, string, string, time.Duration, time.Time) error
}

func decodeUserBody(w http.ResponseWriter, r *http.Request, allowed ...string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	// Decode keys individually to reject duplicate fields as well as unknown ones.
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, useradmin.ErrInvalid
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		key, e := d.Token()
		if e != nil {
			return nil, useradmin.ErrInvalid
		}
		k, ok := key.(string)
		if !ok {
			return nil, useradmin.ErrInvalid
		}
		if _, exists := fields[k]; exists {
			return nil, useradmin.ErrInvalid
		}
		valid := false
		for _, a := range allowed {
			if a == k {
				valid = true
			}
		}
		if !valid {
			return nil, useradmin.ErrInvalid
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || string(raw) == "null" {
			return nil, useradmin.ErrInvalid
		}
		fields[k] = raw
	}
	if _, err = d.Token(); err != nil {
		return nil, useradmin.ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, useradmin.ErrInvalid
	}
	return fields, nil
}
func readUserField(fields map[string]json.RawMessage, key string, out any) bool {
	raw, ok := fields[key]
	return ok && json.Unmarshal(raw, out) == nil
}
func (s *Server) usersError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, useradmin.ErrForbidden):
		writeError(w, s.log, 403, CodeForbidden, useradmin.ErrForbidden.Error())
	case errors.Is(err, useradmin.ErrConflict):
		writeError(w, s.log, 409, CodeConflict, useradmin.ErrConflict.Error())
	case errors.Is(err, useradmin.ErrNotFound):
		writeError(w, s.log, 404, CodeNotFound, useradmin.ErrNotFound.Error())
	case errors.Is(err, useradmin.ErrInvalid), errors.Is(err, useradmin.ErrUnits), errors.Is(err, useradmin.ErrPassword):
		writeError(w, s.log, 400, CodeInvalidRequest, err.Error())
	default:
		writeError(w, s.log, 503, CodeUnavailable, useradmin.ErrUnavailable.Error())
	}
}
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		s.usersError(w, useradmin.ErrUnavailable)
		return
	}
	q := useradmin.Query{Limit: 25}
	v, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil || len(r.URL.RawQuery) > 1024 {
		s.usersError(w, useradmin.ErrInvalid)
		return
	}
	for key, vals := range v {
		if len(vals) != 1 {
			s.usersError(w, useradmin.ErrInvalid)
			return
		}
		value := vals[0]
		switch key {
		case "search":
			q.Search = value
		case "role":
			q.Role = user.Role(value)
		case "status":
			q.Status = value
		case "unitId":
			id, err := uuid.Parse(value)
			if err != nil || id == uuid.Nil || id.String() != value {
				s.usersError(w, useradmin.ErrInvalid)
				return
			}
			q.UnitID = &id
		case "limit", "offset":
			n, err := strconv.Atoi(value)
			if err != nil {
				s.usersError(w, useradmin.ErrInvalid)
				return
			}
			if key == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		default:
			s.usersError(w, useradmin.ErrInvalid)
			return
		}
	}
	actor, _ := UsuarioDoContexto(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	items, err := s.users.List(ctx, actor, q, s.now())
	if err != nil {
		s.usersError(w, err)
		return
	}
	if items == nil {
		items = []useradmin.Record{}
	}
	more := len(items) > q.Limit
	var next *int
	if more {
		items = items[:q.Limit]
		n := q.Offset + q.Limit
		if n <= 10000 {
			next = &n
		}
	}
	writeJSON(w, s.log, 200, struct {
		Items      []useradmin.Record `json:"items"`
		Limit      int                `json:"limit"`
		Offset     int                `json:"offset"`
		HasMore    bool               `json:"hasMore"`
		NextOffset *int               `json:"nextOffset"`
	}{items, q.Limit, q.Offset, more, next})
}
func (s *Server) handleUserAction(kind string, role user.Role) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.users == nil {
			s.usersError(w, useradmin.ErrUnavailable)
			return
		}
		c := useradmin.Command{Kind: kind, Role: role}
		password := ""
		if kind != "create" {
			id, e := uuid.Parse(r.PathValue("id"))
			if e != nil || id == uuid.Nil || id.String() != r.PathValue("id") {
				s.usersError(w, useradmin.ErrInvalid)
				return
			}
			c.Target = id
		}
		allowed := map[string][]string{"create": {"name", "username", "email", "unitIds", "validity", "initialPassword"}, "edit": {"name", "email", "unitIds"}, "active": {"active"}, "renew": {"validity"}, "reset": {"initialPassword"}}[kind]
		f, err := decodeUserBody(w, r, allowed...)
		valid := err == nil
		switch kind {
		case "create", "edit":
			valid = valid && readUserField(f, "name", &c.Name) && readUserField(f, "unitIds", &c.UnitIDs)
			if _, ok := f["email"]; ok {
				valid = valid && readUserField(f, "email", &c.Email)
			}
			if kind == "create" {
				valid = valid && readUserField(f, "username", &c.Username) && readUserField(f, "validity", &c.Period) && readUserField(f, "initialPassword", &password)
			}
		case "active":
			valid = valid && readUserField(f, "active", &c.Active)
		case "renew":
			valid = valid && readUserField(f, "validity", &c.Period)
		case "reset":
			valid = valid && readUserField(f, "initialPassword", &password)
		}
		if !valid {
			s.usersError(w, useradmin.ErrInvalid)
			return
		}
		actor, _ := UsuarioDoContexto(r.Context())
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		result, err := s.users.Apply(ctx, actor, c, password, s.now())
		if err != nil {
			s.usersError(w, err)
			return
		}
		for _, event := range result.Events {
			s.auditUser(r, result.User.ID, audit.Event(event))
		}
		status := 200
		if kind == "create" {
			status = 201
		}
		writeJSON(w, s.log, status, result.User)
	}
}
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if s.users == nil {
		s.usersError(w, useradmin.ErrUnavailable)
		return
	}
	fields, err := decodeUserBody(w, r, "currentPassword", "newPassword")
	var current, next string
	if err != nil || !readUserField(fields, "currentPassword", &current) || !readUserField(fields, "newPassword", &next) {
		s.usersError(w, useradmin.ErrInvalid)
		return
	}
	actor, _ := UsuarioDoContexto(r.Context())
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err = s.users.ChangePassword(ctx, actor, auth.HashSessionToken(tokenDoContexto(r.Context())), current, next, s.auth.IdleTTL(), s.now())
	if err != nil {
		s.usersError(w, err)
		return
	}
	s.auditUser(r, actor.ID, audit.EventUserPasswordChanged)
	s.clearSessionCookie(w)
	if csrf, e := novoTokenCSRF(); e == nil {
		s.setCSRFCookie(w, csrf)
	}
	w.WriteHeader(204)
}
func (s *Server) auditUser(r *http.Request, target uuid.UUID, event audit.Event) {
	if s.auditoria == nil {
		return
	}
	actor, _ := UsuarioDoContexto(r.Context())
	_ = s.auditoria.Record(r.Context(), audit.Entry{Event: event, ActorUserID: &actor.ID, ActorUsername: actor.Username, Detail: "target_user_id=" + target.String(), Origin: ipDoPedido(r)})
}

// Only the minimal authenticated flow may be used before changing the password.
func passwordChangeAllowed(r *http.Request) bool {
	return (r.Method == "GET" && r.URL.Path == "/api/auth/me") || (r.Method == "POST" && (r.URL.Path == "/api/auth/logout" || r.URL.Path == "/api/auth/change-password"))
}
