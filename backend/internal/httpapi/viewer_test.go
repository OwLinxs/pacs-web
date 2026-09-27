package httpapi_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
	"github.com/pmfb-saude/pacs-web/backend/internal/viewer"
)

const viewerStudy = "00000001-00000000-00000000-00000000-00000000"
const viewerSeries = "00000002-00000000-00000000-00000000-00000000"
const viewerInstance = "00000003-00000000-00000000-00000000-00000000"
const seriesRoute = "/api/studies/" + viewerStudy + "/series"
const instancesRoute = seriesRoute + "/" + viewerSeries + "/instances"
const dicomRoute = instancesRoute + "/" + viewerInstance + "/dicom"

type viewerLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *viewerLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *viewerLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type fakeViewer struct {
	calls   atomic.Int32
	failure error
	empty   bool
	broken  bool
}

func (v *fakeViewer) check(cfg orthanc.Config) error {
	v.calls.Add(1)
	if cfg.Credential != "credencial-apenas-ficticia" || cfg.BaseURL != "https://orthanc.test/pacs" {
		return errors.New("configuração fictícia incorreta")
	}
	return v.failure
}
func (v *fakeViewer) ViewerSeries(_ context.Context, cfg orthanc.Config, _ string) ([]viewer.Series, error) {
	if err := v.check(cfg); err != nil {
		return nil, err
	}
	if v.empty {
		return []viewer.Series{}, nil
	}
	return []viewer.Series{{OrthancSeriesID: viewerSeries, InstanceCount: 1, Modality: "OT"}}, nil
}
func (v *fakeViewer) ViewerInstances(_ context.Context, cfg orthanc.Config, _, _ string) ([]viewer.Instance, error) {
	if err := v.check(cfg); err != nil {
		return nil, err
	}
	if v.empty {
		return []viewer.Instance{}, nil
	}
	return []viewer.Instance{{OrthancInstanceID: viewerInstance}}, nil
}
func (v *fakeViewer) OpenDICOM(_ context.Context, cfg orthanc.Config, _, _, _ string) (viewer.DICOM, error) {
	if err := v.check(cfg); err != nil {
		return viewer.DICOM{}, err
	}
	if v.broken {
		return viewer.DICOM{Body: io.NopCloser(io.MultiReader(strings.NewReader("PREFIXO-FICTICIO"), brokenReader{})), Size: -1}, nil
	}
	return viewer.DICOM{Body: io.NopCloser(strings.NewReader("DICOM-FICTICIO")), Size: int64(len("DICOM-FICTICIO"))}, nil
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("PATIENT-FICTICIO-NAO-LOGAR") }

func TestViewerEndpointsAuthorization(t *testing.T) {
	for _, role := range []user.Role{"", user.RoleAdmin, user.RoleGestor, user.RoleMedico, "OUTRO"} {
		t.Run(string(role), func(t *testing.T) {
			provider := &fakeViewer{}
			c := montarCenario(t, opcoesCenario{viewer: provider})
			seedStudiesSettings(t, c)
			if role != "" {
				loginStudies(t, c, role)
			}
			want := 200
			if role == "" {
				want = 401
			}
			if role == "OUTRO" {
				want = 403
			}
			for _, path := range []string{seriesRoute, instancesRoute, dicomRoute} {
				r := c.requisitar(t, http.MethodGet, path, nil)
				body := lerCorpo(t, r)
				if r.StatusCode != want {
					t.Fatalf("status=%d esperado=%d", r.StatusCode, want)
				}
				if r.Header.Get("Cache-Control") != "no-store" {
					t.Fatal("cache permitido")
				}
				if strings.Contains(body, "credencial") || strings.Contains(body, "orthanc.test") {
					t.Fatal("secret/configuração exposta")
				}
				if path == dicomRoute && want == 200 && (r.Header.Get("Content-Type") != "application/dicom" || r.Header.Get("Content-Disposition") != "inline" || body != "DICOM-FICTICIO") {
					t.Fatal("stream inválido")
				}
			}
			if (provider.calls.Load() == 3) != (want == 200) {
				t.Fatal("autorização falhou")
			}
		})
	}
}

func TestViewerInvalidIDsAndNoProxy(t *testing.T) {
	provider := &fakeViewer{}
	c := montarCenario(t, opcoesCenario{viewer: provider})
	seedStudiesSettings(t, c)
	loginStudies(t, c, user.RoleMedico)
	for _, path := range []string{strings.Replace(seriesRoute, viewerStudy, "invalid", 1), strings.Replace(instancesRoute, viewerSeries, "invalid", 1), strings.Replace(dicomRoute, viewerInstance, "invalid", 1), seriesRoute + "?url=http://outro.invalid", dicomRoute + "?transcode=qualquer"} {
		r := c.requisitar(t, "GET", path, nil)
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Fatal(r.StatusCode)
		}
	}
	request, _ := http.NewRequest("GET", c.servidor.URL+dicomRoute, nil)
	request.Header.Set("Range", "bytes=0-100")
	r, err := c.cliente.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 400 || provider.calls.Load() != 0 {
		t.Fatal("override/ID inválido chegou ao cliente")
	}
}

func TestViewerEmptyAndSanitizedErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		empty   bool
		want    int
	}{
		{"vazio", nil, true, 200}, {"indisponível", orthanc.Refused, false, 502}, {"timeout", orthanc.Timeout, false, 504},
		{"inexistente", orthanc.NotFound, false, 404}, {"limite", orthanc.TooLarge, false, 502},
		{"erro bruto", errors.New("PATIENT-FICTICIO ID-FICTICIO ACC-FICTICIO credencial-apenas-ficticia"), false, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs viewerLogBuffer
			provider := &fakeViewer{failure: tc.failure, empty: tc.empty}
			c := montarCenario(t, opcoesCenario{viewer: provider, log: slog.New(slog.NewTextHandler(&logs, nil))})
			seedStudiesSettings(t, c)
			loginStudies(t, c, user.RoleMedico)
			for _, path := range []string{seriesRoute, instancesRoute} {
				r := c.requisitar(t, "GET", path, nil)
				body := lerCorpo(t, r)
				if r.StatusCode != tc.want {
					t.Fatal(r.StatusCode)
				}
				if tc.empty && !strings.Contains(body, `"items":[]`) {
					t.Fatal("vazio não controlado")
				}
				for _, value := range []string{"PATIENT-FICTICIO", "ID-FICTICIO", "ACC-FICTICIO", "credencial-apenas-ficticia", viewerStudy, viewerSeries, viewerInstance} {
					if strings.Contains(body+logs.String(), value) {
						t.Fatal("dado identificável exposto em erro/log")
					}
				}
			}
		})
	}
}

func TestDICOMInterruptedStreamDoesNotAppendJSON(t *testing.T) {
	var logs viewerLogBuffer
	c := montarCenario(t, opcoesCenario{viewer: &fakeViewer{broken: true}, log: slog.New(slog.NewTextHandler(&logs, nil))})
	seedStudiesSettings(t, c)
	loginStudies(t, c, user.RoleMedico)
	r, err := c.cliente.Get(c.servidor.URL + dicomRoute)
	if err == nil {
		defer r.Body.Close()
		body, readErr := io.ReadAll(r.Body)
		if readErr == nil || strings.Contains(string(body), `"error"`) {
			t.Fatal("stream parcial tratado como sucesso")
		}
	}
	if strings.Contains(logs.String(), "PATIENT-FICTICIO") || strings.Contains(logs.String(), viewerInstance) {
		t.Fatal("dado sensível em log de streaming")
	}
}
