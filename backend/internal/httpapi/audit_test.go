package httpapi_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
	"net/http"
	"strings"
	"testing"
)

type auditFake struct {
	queries chan audit.Query
	fail    bool
}

func (f *auditFake) List(_ context.Context, q audit.Query) (audit.Page, error) {
	f.queries <- q
	if f.fail {
		return audit.Page{}, errors.New("synthetic-sensitive-internal-error")
	}
	return audit.Page{Items: []audit.Record{}, Limit: q.Limit, Offset: q.Offset}, nil
}
func TestAuditAuthorizationAndFilters(t *testing.T) {
	for _, role := range []user.Role{"", user.RoleAdmin, user.RoleGestor, user.RoleMedico} {
		f := &auditFake{queries: make(chan audit.Query, 10)}
		c := montarCenario(t, opcoesCenario{auditReader: f})
		if role != "" {
			loginStudies(t, c, role)
		}
		status := 403
		if role == "" {
			status = 401
		}
		if role == user.RoleAdmin {
			status = 200
		}
		expectStatus(t, c.requisitar(t, "GET", "/api/admin/audit", nil), status)
		if role != user.RoleAdmin {
			if len(f.queries) != 0 {
				t.Fatal("unauthorized read")
			}
			continue
		}
		id := uuid.New().String()
		expectStatus(t, c.requisitar(t, "GET", "/api/admin/audit?limit=25&offset=50&dateFrom=2026-09-01&dateTo=2026-09-29&actorId="+id+"&targetUserId="+id+"&event=USER_CREATED&category=users", nil), 200)
		<-f.queries
		q := <-f.queries
		if q.Offset != 50 || q.ActorID.String() != id || q.TargetID.String() != id || q.Event != audit.EventUserCreated || q.Category != "users" {
			t.Fatal("filters lost")
		}
		for _, raw := range []string{"limit=0", "limit=101", "offset=10001", "actorId=arbitrary", "targetUserId=arbitrary", "event=USER_CREATED%27%20OR%201=1", "dateFrom=wrong", "dateFrom=2026-10-01&dateTo=2026-09-01", "category=wrong", "limit=1&limit=2", "detail=secret"} {
			expectStatus(t, c.requisitar(t, "GET", "/api/admin/audit?"+raw, nil), 400)
		}
		for _, method := range []string{"POST", "PATCH", "PUT", "DELETE"} {
			r := c.requisitar(t, method, "/api/admin/audit", nil)
			if r.StatusCode < 400 {
				t.Fatal("audit is writable")
			}
			r.Body.Close()
		}
	}
}
func TestAuditUnavailableSanitized(t *testing.T) {
	f := &auditFake{queries: make(chan audit.Query, 1), fail: true}
	c := montarCenario(t, opcoesCenario{auditReader: f})
	loginStudies(t, c, user.RoleAdmin)
	r := c.requisitar(t, "GET", "/api/admin/audit", nil)
	if r.StatusCode != http.StatusServiceUnavailable {
		t.Fatal(r.StatusCode)
	}
	body := lerCorpo(t, r)
	if body == "" || containsSensitiveAudit(body) {
		t.Fatal("unsafe response")
	}
}
func containsSensitiveAudit(s string) bool {
	return strings.Contains(s, "synthetic-sensitive") || strings.Contains(s, "password_hash") || strings.Contains(s, "session_id")
}
