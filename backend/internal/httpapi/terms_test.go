package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/terms"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

type fakeTerms struct {
	mu       sync.Mutex
	accepted map[uuid.UUID]string
	entries  []audit.Entry
	checks   int
	fail     error
}

func (f *fakeTerms) Accepted(_ context.Context, id uuid.UUID, version string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
	return f.accepted[id] == version, f.fail
}
func (f *fakeTerms) Accept(_ context.Context, id uuid.UUID, version string, e audit.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accepted[id] = version
	f.entries = append(f.entries, e)
	return nil
}

func TestTermsGateAcceptAndAudit(t *testing.T) {
	store := &fakeTerms{accepted: map[uuid.UUID]string{}}
	c := montarCenario(t, opcoesCenario{terms: store, stats: &fakeStats{}})
	seedStudiesSettings(t, c)
	loginStudies(t, c, user.RoleMedico)

	me := func() bool {
		t.Helper()
		r := c.requisitar(t, "GET", "/api/auth/me", nil)
		defer r.Body.Close()
		var body struct{ User struct{ TermsRequired bool } }
		if r.StatusCode != 200 || json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatalf("me: %d", r.StatusCode)
		}
		return body.User.TermsRequired
	}
	if !me() {
		t.Fatal("termo deveria ser exigido")
	}
	r := c.requisitar(t, "GET", "/api/units", nil)
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 403 || !strings.Contains(string(raw), "TERMS_REQUIRED") {
		t.Fatalf("sem aceite deveria bloquear: %d %s", r.StatusCode, raw)
	}

	r = c.requisitar(t, "GET", "/api/terms", nil)
	var doc terms.Document
	if r.StatusCode != 200 || json.NewDecoder(r.Body).Decode(&doc) != nil || doc.Version != terms.Version || len(doc.Sections) == 0 {
		t.Fatalf("termo: %d %+v", r.StatusCode, doc)
	}
	r.Body.Close()

	for _, body := range []any{map[string]string{"version": "antiga"}, map[string]any{"version": terms.Version, "extra": true}} {
		r = c.requisitar(t, "POST", "/api/terms/accept", body)
		r.Body.Close()
		if r.StatusCode != 409 && r.StatusCode != 400 {
			t.Fatalf("aceite inválido: %d", r.StatusCode)
		}
	}
	if len(store.entries) != 0 {
		t.Fatal("aceite inválido gravado")
	}
	r = c.requisitar(t, "POST", "/api/terms/accept", map[string]string{"version": terms.Version})
	r.Body.Close()
	if r.StatusCode != 204 {
		t.Fatalf("aceite: %d", r.StatusCode)
	}
	e := store.entries[0]
	if e.Event != audit.EventTermsAccepted || e.Detail != audit.TermsDetail(terms.Version) || e.ActorUserID == nil || e.ActorUsername != "consulta.teste" {
		t.Fatalf("auditoria: %+v", e)
	}
	if me() {
		t.Fatal("termo já aceito")
	}
	checks := store.checks
	r = c.requisitar(t, "GET", "/api/units", nil)
	r.Body.Close()
	if r.StatusCode == 403 {
		t.Fatal("aceite não liberou acesso")
	}
	if store.checks != checks {
		t.Fatal("aceite deveria ficar em cache na sessão")
	}
}

func TestTermsCheckFailureFailsClosed(t *testing.T) {
	store := &fakeTerms{accepted: map[uuid.UUID]string{}}
	c := montarCenario(t, opcoesCenario{terms: store})
	loginStudies(t, c, user.RoleAdmin)
	store.fail = errors.New("banco indisponível")
	r := c.requisitar(t, "GET", "/api/units", nil)
	r.Body.Close()
	if r.StatusCode != 503 {
		t.Fatalf("falha na consulta deveria bloquear: %d", r.StatusCode)
	}
}
