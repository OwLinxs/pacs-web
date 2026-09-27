package orthanc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/studies"
)

func fakeID(n int) string { return fmt.Sprintf("%08x-00000000-00000000-00000000-00000000", n) }
func studiesConfig() Config {
	return Config{BaseURL: "http://orthanc.test/pacs", Timeout: time.Second, VerifyTLS: true}
}

func TestFindStudiesPageAndModalities(t *testing.T) {
	var finds, children atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if name, secret, ok := r.BasicAuth(); !ok || name != "ficticio" || secret != "credencial-ficticia" {
			t.Error("credencial não utilizada")
		}
		switch r.URL.Path {
		case "/pacs/tools/find":
			finds.Add(1)
			if r.Method != "POST" {
				t.Error("método inesperado")
			}
			var request struct {
				Level                 string
				Expand, CaseSensitive bool
				Limit, Since          int
				Query                 map[string]string
			}
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Error("consulta inválida")
			}
			if request.Level != "Study" || !request.Expand || request.CaseSensitive || request.Limit != 3 || request.Since != 2 {
				t.Errorf("paginação incorreta: %+v", request)
			}
			expected := map[string]string{"StudyDate": "20260901-20260926", "PatientName": "FICTICIO*", "PatientID": "SYNTH-1", "AccessionNumber": "ACC-SYNTH", "StudyDescription": "Exame*", "InstitutionName": "Instituicao Ficticia"}
			if !reflect.DeepEqual(request.Query, expected) {
				t.Error("filtros não repassados")
			}
			_ = json.NewEncoder(w).Encode([]expandedStudy{
				{ID: fakeID(1), Type: "Study", Series: []string{fakeID(10), fakeID(11), fakeID(12)}, MainDicomTags: map[string]string{"StudyInstanceUID": "2.25.123456789", "StudyDate": "20260920", "StudyTime": "101112", "AccessionNumber": "ACC-SYNTH", "StudyDescription": "Exame ficticio", "InstitutionName": "Instituicao Ficticia"}, PatientMainDicomTags: map[string]string{"PatientName": "FICTICIO^UM", "PatientID": "SYNTH-1"}},
				{ID: fakeID(2), Type: "Study", Series: []string{}},           // tags ausentes
				{ID: fakeID(3), Type: "Study", Series: []string{fakeID(30)}}, // sentinela: não hidratar
			})
		case "/pacs/studies/" + fakeID(1) + "/series":
			children.Add(1)
			if r.Method != "GET" || r.URL.RawQuery != "expand=true" {
				t.Error("consulta de séries incorreta")
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"ID": fakeID(10), "Type": "Series", "ParentStudy": fakeID(1), "MainDicomTags": map[string]string{"Modality": "CT"}},
				{"ID": fakeID(11), "Type": "Series", "ParentStudy": fakeID(1), "MainDicomTags": map[string]string{"Modality": "SR"}},
				{"ID": fakeID(12), "Type": "Series", "ParentStudy": fakeID(1), "MainDicomTags": map[string]string{"Modality": "CT"}},
			})
		default:
			t.Errorf("operação não permitida: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cfg := studiesConfig()
	cfg.Username, cfg.Credential = "ficticio", "credencial-ficticia"
	page, err := simulatedClient(t, server).FindStudies(context.Background(), cfg, studies.Query{
		Limit: 2, Offset: 2, DateFrom: "2026-09-01", DateTo: "2026-09-26", PatientName: "FICTICIO*", PatientID: "SYNTH-1", AccessionNumber: "ACC-SYNTH", StudyDescription: "Exame*", InstitutionName: "Instituicao Ficticia",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.HasMore || page.NextOffset == nil || *page.NextOffset != 4 || page.Offset != 2 || page.Limit != 2 {
		t.Fatal("página incorreta")
	}
	first := page.Items[0]
	if first.OrthancStudyID != fakeID(1) || first.StudyInstanceUID != "2.25.123456789" || first.PatientName != "FICTICIO^UM" || first.PatientID != "SYNTH-1" || first.StudyDate != "20260920" || first.StudyTime != "101112" || first.AccessionNumber != "ACC-SYNTH" || first.StudyDescription != "Exame ficticio" || first.InstitutionName != "Instituicao Ficticia" {
		t.Fatal("DTO incompleto")
	}
	if !reflect.DeepEqual(first.Modalities, []string{"CT", "SR"}) || first.SeriesCount != 3 {
		t.Fatal("modalidades/contagem incorretas")
	}
	missing := page.Items[1]
	if missing.PatientName != "" || missing.StudyDate != "" || missing.SeriesCount != 0 || missing.Modalities == nil {
		t.Fatal("tags ausentes não tratadas")
	}
	if finds.Load() != 1 || children.Load() != 1 {
		t.Fatal("consultas excedentes")
	}
	serialized, _ := json.Marshal(page)
	for _, unwanted := range []string{"ParentPatient", "MainDicomTags", "credencial-ficticia", "Authorization", "baseUrl"} {
		if strings.Contains(string(serialized), unwanted) {
			t.Fatal("DTO expôs informação desnecessária")
		}
	}
}

func TestFindStudiesFailuresAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"vazio", `[]`, 200, nil}, {"401", `secret-ficticio`, 401, Unauthorized}, {"403", `secret-ficticio`, 403, Forbidden},
		{"indisponível", `paciente-ficticio`, 503, Upstream}, {"json", `[{paciente-ficticio`, 200, InvalidResponse},
		{"objeto", `{}`, 200, InvalidResponse}, {"null", `null`, 200, InvalidResponse},
		{"tipo incorreto", `[{"ID":"` + fakeID(1) + `","Type":"Patient"}]`, 200, InvalidResponse},
		{"id inseguro", `[{"ID":"../system","Type":"Study"}]`, 200, InvalidResponse},
		{"limite ignorado", `[{},{},{}]`, 200, InvalidResponse},
		{"corpo excessivo", strings.Repeat("x", maxStudyBytes+1), 200, InvalidResponse},
		{"redirect", ``, 302, Redirect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != "/pacs/tools/find" || r.Header.Get("Authorization") != "" {
					t.Error("requisição inesperada")
				}
				w.Header().Set("Location", "/proibido")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			page, err := simulatedClient(t, server).FindStudies(context.Background(), studiesConfig(), studies.Query{Limit: 1})
			if !errors.Is(err, tc.want) {
				t.Fatalf("erro=%v esperado=%v", err, tc.want)
			}
			if calls.Load() != 1 {
				t.Fatal("redirect seguido ou consulta excedente")
			}
			if tc.want == nil && (page.Items == nil || len(page.Items) != 0 || page.HasMore || page.NextOffset != nil) {
				t.Fatal("página vazia incorreta")
			}
			if err != nil && strings.Contains(err.Error(), "ficticio") {
				t.Fatal("erro expõe conteúdo upstream")
			}
		})
	}
}

func TestFindStudiesTimeoutAndCancellation(t *testing.T) {
	for _, stage := range []string{"find", "series"} {
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stage == "series" && r.URL.Path == "/pacs/tools/find" {
					_ = json.NewEncoder(w).Encode([]expandedStudy{{ID: fakeID(1), Type: "Study", Series: []string{fakeID(2)}}})
					return
				}
				_, _ = io.Copy(io.Discard, r.Body)
				<-r.Context().Done()
			}))
			defer server.Close()
			cfg := studiesConfig()
			cfg.Timeout = 40 * time.Millisecond
			client := simulatedClient(t, server)
			_, err := client.FindStudies(context.Background(), cfg, studies.Query{Limit: 1})
			if !errors.Is(err, Timeout) {
				t.Fatalf("erro=%v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err = client.FindStudies(ctx, cfg, studies.Query{Limit: 1})
			if !errors.Is(err, Canceled) {
				t.Fatalf("cancelamento=%v", err)
			}
		})
	}
}

func TestFindStudiesSeriesValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       error
	}{
		{"sem modalidade", `[{"ID":"` + fakeID(2) + `","Type":"Series","ParentStudy":"` + fakeID(1) + `"}]`, nil},
		{"outra relação", `[{"ID":"` + fakeID(2) + `","Type":"Series","ParentStudy":"` + fakeID(3) + `"}]`, InvalidResponse},
		{"série diferente", `[{"ID":"` + fakeID(3) + `","Type":"Series","ParentStudy":"` + fakeID(1) + `"}]`, InvalidResponse},
		{"remoção concorrente", `[]`, InvalidResponse}, {"json inválido", `{`, InvalidResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/pacs/tools/find" {
					_ = json.NewEncoder(w).Encode([]expandedStudy{{ID: fakeID(1), Type: "Study", Series: []string{fakeID(2)}}})
					return
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			page, err := simulatedClient(t, server).FindStudies(context.Background(), studiesConfig(), studies.Query{Limit: 1})
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if tc.want == nil && (len(page.Items[0].Modalities) != 0 || page.Items[0].Modalities == nil || page.Items[0].SeriesCount != 1) {
				t.Fatal("modalidade ausente incorreta")
			}
		})
	}
}

func TestFindStudiesLimitsBeforeNetwork(t *testing.T) {
	client := NewClient()
	for _, q := range []studies.Query{{Limit: 0}, {Limit: 51}, {Limit: 1, Offset: -1}, {Limit: 1, Offset: 10001}} {
		if _, err := client.FindStudies(context.Background(), studiesConfig(), q); !errors.Is(err, InvalidTarget) {
			t.Fatal(err)
		}
	}
	for _, base := range []string{"http://127.0.0.1", "http://169.254.169.254", "http://[::1]"} {
		cfg := studiesConfig()
		cfg.BaseURL = base
		if _, err := client.FindStudies(context.Background(), cfg, studies.Query{Limit: 1}); !errors.Is(err, BlockedTarget) {
			t.Fatal(err)
		}
	}
}

func TestFindStudiesBoundedConcurrency(t *testing.T) {
	var active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pacs/tools/find" {
			rows := []expandedStudy{}
			for i := 1; i <= 9; i++ {
				rows = append(rows, expandedStudy{ID: fakeID(i), Type: "Study", Series: []string{fakeID(i + 100)}})
			}
			_ = json.NewEncoder(w).Encode(rows)
			return
		}
		n := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); n > old; old = peak.Load() {
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		id := strings.Split(r.URL.Path, "/")[3]
		var number int
		_, _ = fmt.Sscanf(id[:8], "%x", &number)
		_ = json.NewEncoder(w).Encode([]map[string]any{{"ID": fakeID(number + 100), "Type": "Series", "ParentStudy": id}})
	}))
	defer server.Close()
	page, err := simulatedClient(t, server).FindStudies(context.Background(), studiesConfig(), studies.Query{Limit: 8, Offset: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 4 || active.Load() != 0 || len(page.Items) != 8 || !page.HasMore || page.NextOffset != nil {
		t.Fatal("concorrência/paginação incorreta")
	}
}
