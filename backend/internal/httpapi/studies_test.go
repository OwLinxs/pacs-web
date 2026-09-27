package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
	"github.com/pmfb-saude/pacs-web/backend/internal/studies"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

type finderFunc func(context.Context, orthanc.Config, studies.Query) (studies.Page, error)

func (f finderFunc) FindStudies(ctx context.Context, cfg orthanc.Config, q studies.Query) (studies.Page, error) {
	return f(ctx, cfg, q)
}

func seedStudiesSettings(t *testing.T, c *cenario) {
	t.Helper()
	secret := "credencial-apenas-ficticia"
	_, err := c.settings.SalvarOrthanc(context.Background(), settings.Orthanc{Name: "PACS ficticio", BaseURL: "https://orthanc.test/pacs", Username: "usuario-ficticio", TimeoutSeconds: 42, VerifyTLS: true}, &secret, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
}

func loginStudies(t *testing.T, c *cenario, role user.Role) {
	t.Helper()
	c.adicionarUsuario(t, "consulta.teste", role, nil)
	c.aquecerCSRF(t)
	if response := c.login(t, "consulta.teste", senhaTeste); response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
}

func TestStudiesAuthorization(t *testing.T) {
	for _, tc := range []struct {
		role user.Role
		want int
	}{{"", 401}, {user.RoleAdmin, 200}, {user.RoleMedico, 200}, {user.RoleGestor, 200}, {"NAO_AUTORIZADO", 403}} {
		t.Run(string(tc.role), func(t *testing.T) {
			var calls atomic.Int32
			c := montarCenario(t, opcoesCenario{studies: finderFunc(func(_ context.Context, _ orthanc.Config, q studies.Query) (studies.Page, error) {
				calls.Add(1)
				return studies.Page{Items: []studies.Study{}, Limit: q.Limit}, nil
			})})
			seedStudiesSettings(t, c)
			if tc.role != "" {
				loginStudies(t, c, tc.role)
			}
			r := c.requisitar(t, "GET", "/api/studies", nil)
			defer r.Body.Close()
			if r.StatusCode != tc.want {
				t.Fatalf("status=%d esperado=%d", r.StatusCode, tc.want)
			}
			if (calls.Load() == 1) != (tc.want == 200) {
				t.Fatal("autorização não bloqueou chamada")
			}
			if r.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("resposta pode ficar em cache")
			}
		})
	}
}

func TestStudiesContractFiltersAndPrivacy(t *testing.T) {
	var logs bytes.Buffer
	var called studies.Query
	c := montarCenario(t, opcoesCenario{log: slog.New(slog.NewTextHandler(&logs, nil)), studies: finderFunc(func(_ context.Context, cfg orthanc.Config, q studies.Query) (studies.Page, error) {
		called = q
		if cfg.BaseURL != "https://orthanc.test/pacs" || cfg.Username != "usuario-ficticio" || cfg.Credential != "credencial-apenas-ficticia" || cfg.Timeout != 42*time.Second || !cfg.VerifyTLS {
			t.Error("configuração segura não reutilizada")
		}
		next := q.Offset + q.Limit
		return studies.Page{Items: []studies.Study{{OrthancStudyID: "00000001-00000000-00000000-00000000-00000000", PatientName: "FICTICIO^PRIVACIDADE", PatientID: "ID-FICTICIO-PRIVACIDADE", StudyInstanceUID: "2.25.987654321", Modalities: []string{"CT", "SR"}, SeriesCount: 2}}, Limit: q.Limit, Offset: q.Offset, HasMore: true, NextOffset: &next}, nil
	})})
	seedStudiesSettings(t, c)
	loginStudies(t, c, user.RoleMedico)
	before := len(c.auditoria.Entradas())
	params := url.Values{"limit": {"2"}, "offset": {"4"}, "dateFrom": {"2026-09-01"}, "dateTo": {"2026-09-26"}, "patientName": {"FICTICIO^PRIVACIDADE"}, "patientId": {"ID-FICTICIO-PRIVACIDADE"}, "accessionNumber": {"ACC-FICTICIO"}, "studyDescription": {"DESCRICAO-FICTICIA"}, "institutionName": {"INSTITUICAO-FICTICIA"}}
	r := c.requisitar(t, "GET", "/api/studies?"+params.Encode(), nil)
	if r.StatusCode != 200 {
		t.Fatal(r.StatusCode)
	}
	body := lerCorpo(t, r)
	var page studies.Page
	if json.Unmarshal([]byte(body), &page) != nil || len(page.Items) != 1 || page.Offset != 4 || page.Limit != 2 || page.NextOffset == nil || *page.NextOffset != 6 || !page.HasMore {
		t.Fatal("contrato inválido")
	}
	if called.Limit != 2 || called.Offset != 4 || called.DateFrom != "2026-09-01" || called.DateTo != "2026-09-26" || called.PatientName != "FICTICIO^PRIVACIDADE" || called.PatientID != "ID-FICTICIO-PRIVACIDADE" || called.AccessionNumber != "ACC-FICTICIO" || called.StudyDescription != "DESCRICAO-FICTICIA" || called.InstitutionName != "INSTITUICAO-FICTICIA" {
		t.Fatal("filtros incorretos")
	}
	for _, value := range []string{"FICTICIO^PRIVACIDADE", "ID-FICTICIO-PRIVACIDADE", "2.25.987654321", "ACC-FICTICIO", "DESCRICAO-FICTICIA", "INSTITUICAO-FICTICIA", "credencial-apenas-ficticia", "orthanc.test"} {
		if strings.Contains(logs.String(), value) {
			t.Fatal("dados clínicos/secret em log")
		}
	}
	for _, secret := range []string{"credencial-apenas-ficticia", "usuario-ficticio", "orthanc.test", "MainDicomTags"} {
		if strings.Contains(body, secret) {
			t.Fatal("configuração/resposta bruta exposta")
		}
	}
	if len(c.auditoria.Entradas()) != before {
		t.Fatal("consulta gerou auditoria desnecessária")
	}
}

func TestStudiesQueryValidation(t *testing.T) {
	var calls atomic.Int32
	c := montarCenario(t, opcoesCenario{studies: finderFunc(func(_ context.Context, _ orthanc.Config, q studies.Query) (studies.Page, error) {
		calls.Add(1)
		return studies.Page{Items: []studies.Study{}, Limit: q.Limit, Offset: q.Offset}, nil
	})})
	seedStudiesSettings(t, c)
	loginStudies(t, c, user.RoleMedico)
	for _, query := range []string{"limit=0", "limit=-1", "limit=51", "limit=abc", "limit=1&limit=2", "offset=-1", "offset=10001", "offset=99999999999999999999999", "dateFrom=2026-02-30", "dateFrom=2026-10-01&dateTo=2026-01-01", "dateTo=2026-9-1", "url=http://outro.invalid", "patientName=%zz", "patientName=" + strings.Repeat("a", 129), "patientId=a%5Cb", "patientName=a%00b", "patientId=%FF", "patientName=" + strings.Repeat("a", 4097)} {
		t.Run(query[:min(len(query), 50)], func(t *testing.T) {
			r := c.requisitar(t, "GET", "/api/studies?"+query, nil)
			defer r.Body.Close()
			if r.StatusCode != 400 {
				t.Fatalf("status=%d", r.StatusCode)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("entrada inválida consultou Orthanc")
	}
	for _, query := range []string{"", "limit=50&offset=10000", "dateFrom=2026-09-01", "dateTo=2026-09-26", "patientName=FICTICIO*"} {
		r := c.requisitar(t, "GET", "/api/studies?"+query, nil)
		if r.StatusCode != 200 {
			t.Fatal(r.StatusCode)
		}
		var page studies.Page
		if json.Unmarshal([]byte(lerCorpo(t, r)), &page) != nil {
			t.Fatal("JSON inválido")
		}
		if query == "" && page.Limit != 25 {
			t.Fatal("limite padrão incorreto")
		}
	}
}

func TestStudiesFailuresSanitized(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
		code string
	}{
		{"timeout", orthanc.Timeout, 504, "PACS_TIMEOUT"}, {"indisponível", orthanc.Refused, 502, "PACS_UNAVAILABLE"},
		{"credencial recusada", orthanc.Unauthorized, 502, "PACS_UNAVAILABLE"}, {"resposta inválida", orthanc.InvalidResponse, 502, "PACS_INVALID_RESPONSE"},
		{"erro com dado clínico", errors.New("FICTICIO^PRIVACIDADE ID-FICTICIO 2.25.987654321 credencial-apenas-ficticia"), 502, "PACS_UNAVAILABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			c := montarCenario(t, opcoesCenario{log: slog.New(slog.NewTextHandler(&logs, nil)), studies: finderFunc(func(context.Context, orthanc.Config, studies.Query) (studies.Page, error) {
				return studies.Page{}, tc.err
			})})
			seedStudiesSettings(t, c)
			loginStudies(t, c, user.RoleMedico)
			r := c.requisitar(t, "GET", "/api/studies?patientName=FICTICIO*", nil)
			body := lerCorpo(t, r)
			if r.StatusCode != tc.want || !strings.Contains(body, tc.code) {
				t.Fatalf("status=%d body=%s", r.StatusCode, body)
			}
			for _, secret := range []string{"FICTICIO", "2.25.987654321", "credencial-apenas-ficticia"} {
				if strings.Contains(body+logs.String(), secret) {
					t.Fatal("informação sensível exposta")
				}
			}
		})
	}
}

func TestStudiesMissingConfigurationAndExpiredSession(t *testing.T) {
	var calls atomic.Int32
	c := montarCenario(t, opcoesCenario{studies: finderFunc(func(context.Context, orthanc.Config, studies.Query) (studies.Page, error) {
		calls.Add(1)
		return studies.Page{}, nil
	})})
	loginStudies(t, c, user.RoleMedico)
	r := c.requisitar(t, "GET", "/api/studies", nil)
	r.Body.Close()
	if r.StatusCode != 503 || calls.Load() != 0 {
		t.Fatal("configuração ausente não bloqueou consulta")
	}
	seedStudiesSettings(t, c)
	c.settings.connectionErr = errors.New("credencial-apenas-ficticia")
	r = c.requisitar(t, "GET", "/api/studies", nil)
	if r.StatusCode != 503 || calls.Load() != 0 || strings.Contains(lerCorpo(t, r), "credencial-apenas-ficticia") {
		t.Fatal("erro de configuração exposto")
	}
	c.requisitar(t, "POST", "/api/auth/logout", nil).Body.Close()
	r = c.requisitar(t, "GET", "/api/studies", nil)
	defer r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized || calls.Load() != 0 {
		t.Fatal("sessão revogada aceitou consulta")
	}
}
