package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/authtest"
	"github.com/pmfb-saude/pacs-web/backend/internal/httpapi"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
	"github.com/pmfb-saude/pacs-web/backend/internal/useradmin"
)

// Memory persistence exercises the real service and HTTP layer. SQL atomicity,
// constraints and migration are tested separately with disposable PostgreSQL.
type usersRepoFake struct {
	mu       sync.Mutex
	users    *authtest.Users
	sessions *authtest.Sessions
	records  map[uuid.UUID]user.User
	unitIDs  []uuid.UUID
}

func (r *usersRepoFake) TargetRole(ctx context.Context, id uuid.UUID) (user.Role, error) {
	u, e := r.users.ByID(ctx, id)
	if e != nil {
		return "", useradmin.ErrNotFound
	}
	return u.Role, nil
}
func recordUser(u user.User, now time.Time) useradmin.Record {
	state := "active"
	if !u.Active {
		state = "inactive"
	} else if user.AccessExpired(u.AccessValidUntil, now) {
		state = "expired"
	}
	var until *string
	if u.AccessValidUntil != nil {
		v := u.AccessValidUntil.Format("2006-01-02")
		until = &v
	}
	return useradmin.Record{ID: u.ID, Name: u.Name, Username: u.Username, Email: u.Email, Role: u.Role, Units: u.Units, Active: u.Active, Status: state, AccessValidUntil: until, MustChangePassword: u.MustChangePassword}
}
func (r *usersRepoFake) List(_ context.Context, a user.User, _ useradmin.Query, now time.Time) ([]useradmin.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []useradmin.Record{}
	for _, u := range r.records {
		if a.Role == user.RoleAdmin || u.Role == user.RoleMedico {
			out = append(out, recordUser(u, now))
		}
	}
	return out, nil
}
func (r *usersRepoFake) Apply(ctx context.Context, _ user.User, c useradmin.Command, now time.Time) (useradmin.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, e := r.users.ByID(ctx, c.Target)
	events := []string{}
	if c.Kind == "create" {
		if _, e := r.users.ByUsername(ctx, c.Username); e == nil {
			return useradmin.Result{}, useradmin.ErrConflict
		}
		u = user.User{ID: uuid.New(), Name: c.Name, Username: c.Username, Email: c.Email, Role: c.Role, Active: true, PasswordHash: c.PasswordHash, MustChangePassword: true}
		u.AccessValidUntil, _ = useradmin.Validity(c.Period, nil, now)
		events = append(events, "USER_CREATED")
	} else if e != nil {
		return useradmin.Result{}, useradmin.ErrNotFound
	}
	switch c.Kind {
	case "edit":
		u.Name = c.Name
		u.Email = c.Email
		events = append(events, "USER_UPDATED")
	case "active":
		u.Active = c.Active
		event := "USER_DEACTIVATED"
		if c.Active {
			event = "USER_ACTIVATED"
		}
		events = append(events, event)
	case "renew":
		u.AccessValidUntil, _ = useradmin.Validity(c.Period, u.AccessValidUntil, now)
		events = append(events, "USER_ACCESS_RENEWED")
	case "reset":
		u.PasswordHash = c.PasswordHash
		u.MustChangePassword = true
		events = append(events, "USER_PASSWORD_RESET")
	}
	if c.Kind == "create" || c.Kind == "edit" {
		u.Units = []user.UnitRef{}
		for _, id := range c.UnitIDs {
			if !slices.Contains(r.unitIDs, id) {
				return useradmin.Result{}, useradmin.ErrUnits
			}
			u.Units = append(u.Units, user.UnitRef{ID: id, Name: "Unidade Sintética", Active: true})
		}
		events = append(events, "USER_UNITS_CHANGED")
	}
	if c.Kind == "reset" || (c.Kind == "active" && !c.Active) {
		_ = r.sessions.RevokeAllForUser(ctx, u.ID, now)
	}
	r.users.Add(u)
	r.records[u.ID] = u
	return useradmin.Result{User: recordUser(u, now), Events: events}, nil
}
func (r *usersRepoFake) ChangePassword(ctx context.Context, u user.User, token []byte, expected, next string, idle time.Duration, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, v, e := r.sessions.Resolve(ctx, token, idle, now)
	if e != nil || v.PasswordHash != expected {
		return useradmin.ErrForbidden
	}
	v.PasswordHash = next
	v.MustChangePassword = false
	r.users.Add(v)
	r.records[v.ID] = v
	return r.sessions.RevokeAllForUser(ctx, v.ID, now)
}
func userScenario(t *testing.T, log *slog.Logger) (*cenario, *usersRepoFake) {
	r := &usersRepoFake{records: map[uuid.UUID]user.User{}, unitIDs: []uuid.UUID{uuid.New(), uuid.New()}}
	c := montarCenario(t, opcoesCenario{log: log, tentativasLogin: 100, users: func(u *authtest.Users, s *authtest.Sessions) httpapi.UsersService {
		r.users = u
		r.sessions = s
		return useradmin.New(r, hasherRapido())
	}})
	return c, r
}
func userBody(r *usersRepoFake) map[string]any {
	return map[string]any{"name": "Usuário Sintético", "username": "SYNTHETIC_USER", "unitIds": r.unitIDs, "validity": "3m", "initialPassword": senhaTeste}
}
func expectStatus(t *testing.T, res *http.Response, status int) string {
	t.Helper()
	body := lerCorpo(t, res)
	if res.StatusCode != status {
		t.Fatalf("HTTP %d expected %d: %s", res.StatusCode, status, body)
	}
	return body
}
func TestUsersAuthorizationMatrix(t *testing.T) {
	for _, actorRole := range []user.Role{user.RoleAdmin, user.RoleGestor, user.RoleMedico} {
		t.Run(string(actorRole), func(t *testing.T) {
			c, r := userScenario(t, nil)
			a := c.adicionarUsuario(t, "actor", actorRole, nil)
			c.aquecerCSRF(t)
			expectStatus(t, c.login(t, a.Username, senhaTeste), 200)
			for _, target := range []struct {
				path string
				role user.Role
			}{{"medicos", user.RoleMedico}, {"gestores", user.RoleGestor}} {
				want := 403
				if useradmin.CanManage(actorRole, target.role) {
					want = 201
				}
				body := userBody(r)
				body["username"] = "create_" + strings.ToLower(string(target.role))
				expectStatus(t, c.requisitar(t, "POST", "/api/admin/users/"+target.path, body), want)
			}
			for _, targetRole := range []user.Role{user.RoleAdmin, user.RoleGestor, user.RoleMedico} {
				u := c.adicionarUsuario(t, "target_"+strings.ToLower(string(targetRole)), targetRole, nil)
				for _, action := range []struct {
					method, path string
					body         any
				}{{"PATCH", "", map[string]any{"name": "Editado Sintético", "email": "", "unitIds": r.unitIDs}}, {"POST", "/active", map[string]bool{"active": false}}, {"POST", "/active", map[string]bool{"active": true}}, {"POST", "/renew", map[string]string{"validity": "1m"}}, {"POST", "/reset-password", map[string]string{"initialPassword": "new-synthetic-password"}}} {
					want := 403
					if useradmin.CanManage(actorRole, targetRole) {
						want = 200
					}
					expectStatus(t, c.requisitar(t, action.method, "/api/admin/users/"+u.ID.String()+action.path, action.body), want)
				}
			}
			want := 200
			if actorRole == user.RoleMedico {
				want = 403
			}
			expectStatus(t, c.requisitar(t, "GET", "/api/admin/users", nil), want)
			if actorRole == user.RoleGestor {
				expectStatus(t, c.requisitar(t, "GET", "/api/admin/users?role=ADMIN", nil), 403)
			}
			// No endpoint can create ADMIN, and role/username/must-change overrides are rejected.
			expectStatus(t, c.requisitar(t, "POST", "/api/admin/users/admins", userBody(r)), 404)
			if actorRole != user.RoleMedico {
				for _, field := range []string{"role", "mustChangePassword", "accessValidUntil", "password_hash"} {
					body := userBody(r)
					body[field] = "ADMIN"
					expectStatus(t, c.requisitar(t, "POST", "/api/admin/users/medicos", body), 400)
				}
			}
		})
	}
}
func TestUsersPasswordLifecycleAndAudit(t *testing.T) {
	var logs bytes.Buffer
	c, r := userScenario(t, slog.New(slog.NewTextHandler(&logs, nil)))
	admin := c.adicionarUsuario(t, "administrator", user.RoleAdmin, nil)
	c.aquecerCSRF(t)
	expectStatus(t, c.login(t, admin.Username, senhaTeste), 200)
	body := expectStatus(t, c.requisitar(t, "POST", "/api/admin/users/medicos", userBody(r)), 201)
	var created useradmin.Record
	if json.Unmarshal([]byte(body), &created) != nil {
		t.Fatal("DTO")
	}
	if !created.MustChangePassword || len(created.Units) != 2 {
		t.Fatal("initial flag/multiple units")
	}
	expectStatus(t, c.requisitar(t, "POST", "/api/admin/users/medicos", userBody(r)), 409)
	stored, _ := c.usuarios.ByID(context.Background(), created.ID)
	if ok, _, e := hasherRapido().Verify(stored.PasswordHash, senhaTeste); !ok || e != nil {
		t.Fatal("not hashed")
	}
	expectStatus(t, c.login(t, created.Username, senhaTeste), 200)
	oldToken := c.cookieSessao()
	for _, path := range []string{"/api/studies", "/api/units", "/api/admin/users", "/api/admin/settings/orthanc", "/api/studies/invalid/series"} {
		result := expectStatus(t, c.requisitar(t, "GET", path, nil), 403)
		if !strings.Contains(result, "PASSWORD_CHANGE_REQUIRED") {
			t.Fatal("missing mandatory gate")
		}
	}
	expectStatus(t, c.requisitar(t, "GET", "/api/auth/me", nil), 200)
	expectStatus(t, c.requisitar(t, "POST", "/api/auth/change-password", map[string]string{"currentPassword": "incorrect", "newPassword": "new-synthetic-password"}), 400)
	expectStatus(t, c.requisitar(t, "POST", "/api/auth/change-password", map[string]string{"currentPassword": senhaTeste, "newPassword": "new-synthetic-password"}), 204)
	if c.cookieSessao() != "" {
		t.Fatal("cookie not cleared")
	}
	if _, _, e := r.sessions.Resolve(context.Background(), auth.HashSessionToken(oldToken), time.Hour, time.Now()); !errors.Is(e, auth.ErrSessaoInvalida) {
		t.Fatal("old session alive")
	}
	expectStatus(t, c.login(t, created.Username, "new-synthetic-password"), 200)
	expectStatus(t, c.requisitar(t, "GET", "/api/units", nil), 503) // passed gate; no units store injected
	token := c.cookieSessao()
	expectStatus(t, c.login(t, admin.Username, senhaTeste), 200)
	expectStatus(t, c.requisitar(t, "POST", "/api/admin/users/"+created.ID.String()+"/reset-password", map[string]string{"initialPassword": "reset-synthetic-password"}), 200)
	if _, _, e := r.sessions.Resolve(context.Background(), auth.HashSessionToken(token), time.Hour, time.Now()); e == nil {
		t.Fatal("reset did not revoke")
	}
	expectStatus(t, c.login(t, created.Username, "reset-synthetic-password"), 200)
	stored, _ = c.usuarios.ByID(context.Background(), created.ID)
	if !stored.MustChangePassword {
		t.Fatal("reset flag")
	}
	expectStatus(t, c.requisitar(t, "POST", "/api/auth/logout", nil), 204)
	expectStatus(t, c.login(t, admin.Username, senhaTeste), 200)
	expectStatus(t, c.requisitar(t, "POST", "/api/admin/users/"+created.ID.String()+"/active", map[string]bool{"active": false}), 200)
	expectStatus(t, c.login(t, created.Username, "reset-synthetic-password"), 403)
	for _, entry := range c.auditoria.Entradas() {
		if strings.HasPrefix(string(entry.Event), "USER_") {
			if entry.ActorUserID == nil || entry.Detail != "target_user_id="+created.ID.String() {
				t.Fatal("audit context")
			}
		}
	}
	for _, secret := range []string{senhaTeste, "new-synthetic-password", "reset-synthetic-password", "$argon2id$"} {
		if strings.Contains(body, secret) || strings.Contains(logs.String(), secret) {
			t.Fatal("sensitive output")
		}
	}
}
func TestUsersMalformedAndUnauthenticated(t *testing.T) {
	c, r := userScenario(t, nil)
	c.aquecerCSRF(t)
	expectStatus(t, c.requisitar(t, "GET", "/api/admin/users", nil), 401)
	expectStatus(t, c.requisitar(t, "POST", "/api/admin/users/medicos", userBody(r)), 401)
	a := c.adicionarUsuario(t, "admin", user.RoleAdmin, nil)
	expectStatus(t, c.login(t, a.Username, senhaTeste), 200)
	for _, query := range []string{"limit=0", "limit=101", "offset=-1", "offset=10001", "role=ROOT", "status=invalid", "unitId=bad", "unknown=1", "limit=1&limit=2"} {
		expectStatus(t, c.requisitar(t, "GET", "/api/admin/users?"+query, nil), 400)
	}
	for _, raw := range []string{`{"name":"first","name":"second"}`, `{"name":null}`, `{} {}`, `{"role":"ADMIN"}`} {
		req, _ := http.NewRequest("POST", c.servidor.URL+"/api/admin/users/medicos", strings.NewReader(raw))
		req.Header.Set("X-CSRF-Token", c.tokenCSRF())
		res, e := c.cliente.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		expectStatus(t, res, 400)
		res.Body.Close()
	}
	req, _ := http.NewRequest("POST", c.servidor.URL+"/api/admin/users/medicos", strings.NewReader(`{}`))
	res, e := c.cliente.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	expectStatus(t, res, 403)
	res.Body.Close()
	expectStatus(t, c.requisitar(t, "PATCH", "/api/admin/users/invalid", map[string]string{}), 400)
	expectStatus(t, c.requisitar(t, "DELETE", "/api/admin/users/"+a.ID.String(), nil), 404)
}
