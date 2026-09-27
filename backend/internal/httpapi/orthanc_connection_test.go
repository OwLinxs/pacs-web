package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

type testerFunc func(context.Context, orthanc.Config) error

func (f testerFunc) TestSystem(ctx context.Context, cfg orthanc.Config) error { return f(ctx, cfg) }

func configureTest(t *testing.T, c *cenario) {
	t.Helper()
	response := c.requisitar(t, http.MethodPut, "/api/admin/settings/orthanc", map[string]any{
		"name": "Conexão fictícia", "baseUrl": "https://orthanc.test/pacs", "username": "ficticio",
		"credential": "secret-apenas-teste", "timeoutSeconds": 42, "verifyTls": true,
	})
	if response.StatusCode != 200 {
		t.Fatalf("configuração: %d", response.StatusCode)
	}
}

func TestOrthancConnectionAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name     string
		role     user.Role
		expected int
	}{
		{"anônimo", "", 401}, {"medico", user.RoleMedico, 403}, {"gestor", user.RoleGestor, 403}, {"admin", user.RoleAdmin, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			c := montarCenario(t, opcoesCenario{orthanc: testerFunc(func(context.Context, orthanc.Config) error { calls.Add(1); return nil })})
			c.aquecerCSRF(t)
			if tc.role != "" {
				c.adicionarUsuario(t, "usuario.teste", tc.role, nil)
				if r := c.login(t, "usuario.teste", senhaTeste); r.StatusCode != 200 {
					t.Fatal(r.StatusCode)
				}
			}
			if tc.role == user.RoleAdmin {
				configureTest(t, c)
			}
			response := c.requisitar(t, http.MethodPost, "/api/admin/settings/orthanc/test", nil)
			if response.StatusCode != tc.expected {
				t.Fatalf("status=%d esperado=%d", response.StatusCode, tc.expected)
			}
			if tc.role != user.RoleAdmin && calls.Load() != 0 {
				t.Fatal("cliente chamado sem ADMIN")
			}
			if tc.role == user.RoleAdmin && calls.Load() != 1 {
				t.Fatal("cliente não chamado")
			}
		})
	}
}

func TestOrthancConnectionResultsAndSanitization(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"sucesso", nil, "connected"}, {"timeout", orthanc.Timeout, "timeout"},
		{"dns", orthanc.DNS, "dns"}, {"recusada", orthanc.Refused, "connection_refused"},
		{"tls", orthanc.TLS, "tls"}, {"401", orthanc.Unauthorized, "unauthorized"},
		{"403", orthanc.Forbidden, "forbidden"}, {"500", orthanc.Upstream, "upstream_http"},
		{"json", orthanc.InvalidResponse, "invalid_response"}, {"outro", orthanc.NotOrthanc, "not_orthanc"},
		{"redirect", orthanc.Redirect, "redirect"}, {"bloqueado", orthanc.BlockedTarget, "blocked_target"},
		{"erro desconhecido", errors.New("secret-apenas-teste Authorization resposta-interna"), "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			c := montarCenario(t, opcoesCenario{
				log: slog.New(slog.NewTextHandler(&logs, nil)),
				orthanc: testerFunc(func(ctx context.Context, cfg orthanc.Config) error {
					if cfg.BaseURL != "https://orthanc.test/pacs" || cfg.Username != "ficticio" || cfg.Credential != "secret-apenas-teste" || cfg.Timeout != 42*time.Second || !cfg.VerifyTLS {
						t.Error("configuração salva não foi repassada integralmente")
					}
					return tc.err
				}),
			})
			c.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
			c.aquecerCSRF(t)
			if r := c.login(t, "admin.teste", senhaTeste); r.StatusCode != 200 {
				t.Fatal(r.StatusCode)
			}
			configureTest(t, c)
			response := c.requisitar(t, http.MethodPost, "/api/admin/settings/orthanc/test", nil)
			if response.StatusCode != 200 {
				t.Fatal(response.StatusCode)
			}
			body := lerCorpo(t, response)
			var result struct {
				Success   bool
				Status    string
				Code      string
				CheckedAt time.Time
			}
			if err := json.Unmarshal([]byte(body), &result); err != nil {
				t.Fatal(err)
			}
			if result.Success != (tc.err == nil) || result.Code != tc.code || result.CheckedAt.IsZero() {
				t.Fatalf("resultado inesperado: %+v", result)
			}
			wantStatus := "connected"
			if tc.err != nil {
				wantStatus = "failed"
			}
			if result.Status != wantStatus {
				t.Fatal(result.Status)
			}
			stored, _ := c.settings.Orthanc(context.Background())
			if string(stored.Status) != wantStatus || stored.LastCheckedAt == nil {
				t.Fatal("status/data não registrados")
			}
			readBody := lerCorpo(t, c.requisitar(t, http.MethodGet, "/api/admin/settings/orthanc", nil))
			if !strings.Contains(readBody, `"connectionTestAvailable":true`) || !strings.Contains(readBody, `"lastCheckedAt":`) {
				t.Fatal("metadados ausentes no GET")
			}
			entries := c.auditoria.Entradas()
			last := entries[len(entries)-1]
			if last.Event != audit.EventOrthancConnectionTested || last.ActorUserID == nil || last.Origin == "" || last.Detail != "PACS/Orthanc: "+tc.code {
				t.Fatalf("auditoria inesperada: %+v", last)
			}
			for _, secret := range []string{"secret-apenas-teste", "Authorization", "resposta-interna"} {
				if strings.Contains(body+readBody+logs.String()+last.Detail, secret) {
					t.Fatal("dado sensível exposto")
				}
			}
			configureTest(t, c)
			stored, _ = c.settings.Orthanc(context.Background())
			if stored.Status != settings.StatusNaoVerificado || stored.LastCheckedAt != nil {
				t.Fatal("salvar deve invalidar o teste anterior")
			}
		})
	}
}

func TestOrthancConnectionRejectsCSRFAndInputOverrides(t *testing.T) {
	var calls atomic.Int32
	c := montarCenario(t, opcoesCenario{orthanc: testerFunc(func(context.Context, orthanc.Config) error { calls.Add(1); return nil })})
	c.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	c.aquecerCSRF(t)
	c.login(t, "admin.teste", senhaTeste)
	configureTest(t, c)
	for _, tc := range []struct {
		name, csrf, origin, query, body string
		status                          int
	}{
		{"sem csrf", "", "", "", "", 403},
		{"csrf divergente", "divergente", "", "", "", 403},
		{"origem externa", c.tokenCSRF(), "https://externo.invalid", "", "", 403},
		{"override corpo", c.tokenCSRF(), "", "", `{"baseUrl":"http://outro.invalid"}`, 400},
		{"override query", c.tokenCSRF(), "", "?url=http://outro.invalid", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, c.servidor.URL+"/api/admin/settings/orthanc/test"+tc.query, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-CSRF-Token", tc.csrf)
			req.Header.Set("Origin", tc.origin)
			response, err := c.cliente.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("status=%d", response.StatusCode)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("requisição bloqueada chamou Orthanc")
	}
}

func TestOrthancConnectionPrerequisitesAndConcurrentChange(t *testing.T) {
	var calls atomic.Int32
	c := montarCenario(t, opcoesCenario{orthanc: testerFunc(func(context.Context, orthanc.Config) error { calls.Add(1); return nil })})
	c.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	c.aquecerCSRF(t)
	c.login(t, "admin.teste", senhaTeste)
	if r := c.requisitar(t, http.MethodPost, "/api/admin/settings/orthanc/test", nil); r.StatusCode != 409 {
		t.Fatal(r.StatusCode)
	}
	configureTest(t, c)
	c.settings.connectionErr = errors.New("secret-apenas-teste")
	r := c.requisitar(t, http.MethodPost, "/api/admin/settings/orthanc/test", nil)
	if r.StatusCode != 503 || strings.Contains(lerCorpo(t, r), "secret-apenas-teste") {
		t.Fatal("erro de credencial não sanitizado")
	}
	if calls.Load() != 0 {
		t.Fatal("não deveria chamar cliente")
	}
	c.settings.connectionErr = nil
	c.settings.recordErr = settings.ErrConfiguracaoAlterada
	if r := c.requisitar(t, http.MethodPost, "/api/admin/settings/orthanc/test", nil); r.StatusCode != 409 {
		t.Fatal(r.StatusCode)
	}
	c.settings.recordErr = errors.New("secret-apenas-teste")
	if r := c.requisitar(t, http.MethodPost, "/api/admin/settings/orthanc/test", nil); r.StatusCode != 503 {
		t.Fatal(r.StatusCode)
	}
}
