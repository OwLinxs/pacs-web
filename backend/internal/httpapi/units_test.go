package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pmfb-saude/pacs-web/backend/internal/units"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

type unitsFake struct {
	mu    sync.Mutex
	items map[uuid.UUID]units.Unit
	calls int
}

func newUnitsFake() *unitsFake      { return &unitsFake{items: map[uuid.UUID]units.Unit{}} }
func (f *unitsFake) callCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

func (f *unitsFake) List(_ context.Context, include bool, limit, offset int) ([]units.Unit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	rows := []units.Unit{}
	for _, u := range f.items {
		if include || u.Active {
			rows = append(rows, u)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name) })
	if offset >= len(rows) {
		return []units.Unit{}, nil
	}
	return rows[offset:min(len(rows), offset+limit+1)], nil
}
func (f *unitsFake) Create(_ context.Context, name string) (units.Unit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	for _, u := range f.items {
		if strings.EqualFold(u.Name, name) {
			return units.Unit{}, units.ErrDuplicate
		}
	}
	u := units.Unit{ID: uuid.New(), Name: name, Active: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.items[u.ID] = u
	return u, nil
}
func (f *unitsFake) Update(_ context.Context, id uuid.UUID, p units.Patch) (units.Unit, units.Unit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	before, ok := f.items[id]
	if !ok {
		return units.Unit{}, units.Unit{}, units.ErrNotFound
	}
	after := before
	if p.Name != nil {
		for _, u := range f.items {
			if u.ID != id && strings.EqualFold(u.Name, *p.Name) {
				return before, before, units.ErrDuplicate
			}
		}
		after.Name = *p.Name
	}
	if p.Active != nil {
		after.Active = *p.Active
	}
	after.UpdatedAt = time.Now()
	f.items[id] = after
	return before, after, nil
}
func unitResponse(t *testing.T, r *http.Response, status int) units.Unit {
	t.Helper()
	if r.StatusCode != status {
		t.Fatalf("status=%d esperado=%d corpo=%s", r.StatusCode, status, lerCorpo(t, r))
	}
	var u units.Unit
	if json.Unmarshal([]byte(lerCorpo(t, r)), &u) != nil {
		t.Fatal("JSON inválido")
	}
	return u
}
func TestUnitsAdminLifecycle(t *testing.T) {
	f := newUnitsFake()
	c := montarCenario(t, opcoesCenario{units: f})
	loginStudies(t, c, user.RoleAdmin)
	created := unitResponse(t, c.requisitar(t, "POST", "/api/admin/units", map[string]any{"name": "  Unidade Sintética  "}), 201)
	if created.Name != "Unidade Sintética" || !created.Active || created.ID == uuid.Nil {
		t.Fatal("criação inválida")
	}
	path := "/api/admin/units/" + created.ID.String()
	edited := unitResponse(t, c.requisitar(t, "PATCH", path, map[string]any{"name": "Unidade Renomeada"}), 200)
	if edited.Name != "Unidade Renomeada" {
		t.Fatal("nome não alterado")
	}
	deactivated := unitResponse(t, c.requisitar(t, "PATCH", path, map[string]any{"active": false}), 200)
	if deactivated.Active {
		t.Fatal("desativação falhou")
	}
	for _, tc := range []struct {
		query string
		count int
	}{{"", 0}, {"?includeInactive=true", 1}} {
		r := c.requisitar(t, "GET", "/api/units"+tc.query, nil)
		var page struct{ Items []units.Unit }
		if r.StatusCode != 200 || json.Unmarshal([]byte(lerCorpo(t, r)), &page) != nil || len(page.Items) != tc.count {
			t.Fatal("filtro de ativas inválido")
		}
	}
	if r := c.requisitar(t, "POST", "/api/admin/units", map[string]any{"name": "unidade renomeada"}); r.StatusCode != 409 {
		t.Fatal("duplicata inativa aceita")
	}
	active := unitResponse(t, c.requisitar(t, "PATCH", path, map[string]any{"active": true}), 200)
	if !active.Active {
		t.Fatal("reativação falhou")
	}
	_ = unitResponse(t, c.requisitar(t, "PATCH", path, map[string]any{"active": true}), 200)
	// Persistence/audit atomicity belongs to the store; this double tests HTTP behavior.

	if r := c.requisitar(t, "DELETE", path, nil); r.StatusCode < 400 {
		t.Fatal("exclusão física exposta")
	}
}
func TestUnitsAuthorization(t *testing.T) {
	for _, role := range []user.Role{"", user.RoleGestor, user.RoleMedico, "NAO_AUTORIZADO"} {
		t.Run(string(role), func(t *testing.T) {
			f := newUnitsFake()
			c := montarCenario(t, opcoesCenario{units: f})
			c.aquecerCSRF(t)
			if role != "" {
				c.adicionarUsuario(t, "unidades.teste", role, nil)
				if r := c.login(t, "unidades.teste", senhaTeste); r.StatusCode != 200 {
					t.Fatal(r.StatusCode)
				}
			}
			expected := 403
			if role == "" {
				expected = 401
			}
			for _, req := range []struct {
				method, path string
				body         any
			}{
				{"POST", "/api/admin/units", map[string]any{"name": "Fictícia"}},
				{"PATCH", "/api/admin/units/" + uuid.NewString(), map[string]any{"name": "Fictícia"}},
				{"PATCH", "/api/admin/units/" + uuid.NewString(), map[string]any{"active": false}},
				{"PATCH", "/api/admin/units/" + uuid.NewString(), map[string]any{"active": true}},
				{"GET", "/api/units?includeInactive=true", nil},
			} {
				if r := c.requisitar(t, req.method, req.path, req.body); r.StatusCode != expected {
					t.Fatalf("%s: status=%d", req.method, r.StatusCode)
				}
			}
			if f.callCount() != 0 {
				t.Fatal("não autorizado acessou store")
			}
			want := expected
			if role == user.RoleGestor || role == user.RoleMedico {
				want = 200
			}
			r := c.requisitar(t, "GET", "/api/units", nil)
			if r.StatusCode != want {
				t.Fatal(r.StatusCode)
			}
		})
	}
}
func TestUnitsInputAndCSRF(t *testing.T) {
	f := newUnitsFake()
	c := montarCenario(t, opcoesCenario{units: f})
	loginStudies(t, c, user.RoleAdmin)
	for _, body := range []any{nil, map[string]any{}, map[string]any{"name": ""}, map[string]any{"name": "   "}, map[string]any{"name": "\nX"}, map[string]any{"name": strings.Repeat("a", 121)}, map[string]any{"name": nil}, map[string]any{"name": "Fictícia", "active": false}, map[string]any{"name": "Fictícia", "extra": true}} {
		if r := c.requisitar(t, "POST", "/api/admin/units", body); r.StatusCode != 400 {
			t.Fatal("corpo inválido aceito", r.StatusCode)
		}
	}
	if f.callCount() != 0 {
		t.Fatal("entrada inválida chegou ao store")
	}
	unit := unitResponse(t, c.requisitar(t, "POST", "/api/admin/units", map[string]any{"name": "Unidade Fictícia"}), 201)
	other := unitResponse(t, c.requisitar(t, "POST", "/api/admin/units", map[string]any{"name": "Outra Fictícia"}), 201)
	if r := c.requisitar(t, "PATCH", "/api/admin/units/"+other.ID.String(), map[string]any{"name": "UNIDADE FICTÍCIA"}); r.StatusCode != 409 {
		t.Fatal("edição duplicada aceita")
	}
	for _, body := range []any{map[string]any{}, map[string]any{"active": nil}, map[string]any{"active": "false"}, map[string]any{"name": nil, "active": false}, map[string]any{"slug": "test"}} {
		if r := c.requisitar(t, "PATCH", "/api/admin/units/"+unit.ID.String(), body); r.StatusCode != 400 {
			t.Fatal("patch inválido aceito")
		}
	}
	if r := c.requisitar(t, "PATCH", "/api/admin/units/invalid", map[string]any{"active": false}); r.StatusCode != 400 {
		t.Fatal("id inválido")
	}
	if r := c.requisitar(t, "PATCH", "/api/admin/units/"+uuid.NewString(), map[string]any{"active": false}); r.StatusCode != 404 {
		t.Fatal("ausente")
	}
	for _, query := range []string{"includeInactive=1", "includeInactive=true&includeInactive=false", "unknown=yes", "limit=0", "limit=101", "offset=-1", "offset=10001"} {
		if r := c.requisitar(t, "GET", "/api/units?"+query, nil); r.StatusCode != 400 {
			t.Fatal("query inválida")
		}
	}
	request, _ := http.NewRequest("PATCH", c.servidor.URL+"/api/admin/units/"+unit.ID.String(), strings.NewReader(`{"active":false}`))
	r, err := c.cliente.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != 403 {
		t.Fatal("CSRF ausente aceito")
	}
}
