package orthanc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/stats"
)

func TestStudyMomentsQueryAndTruncation(t *testing.T) {
	count := 2
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/pacs/tools/find" {
			t.Errorf("operação não permitida: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		want := map[string]any{"Level": "Study", "Expand": true, "Limit": float64(stats.MaxStudies + 1), "Query": map[string]any{"StudyDate": "20261026-20261101"}}
		if !reflect.DeepEqual(request, want) {
			t.Errorf("consulta incorreta: %v", request)
		}
		found := make([]map[string]any, count)
		for i := range found {
			found[i] = map[string]any{"ID": fakeID(i), "Type": "Study", "MainDicomTags": map[string]string{"StudyDate": "20261027", "StudyTime": "0815"}, "PatientMainDicomTags": map[string]string{"PatientName": "FICTICIO^PACIENTE"}}
		}
		_ = json.NewEncoder(w).Encode(found)
	}))
	defer server.Close()
	from, to := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	moments, truncated, err := simulatedClient(t, server).StudyMoments(context.Background(), studiesConfig(), from, to)
	if err != nil || truncated || !reflect.DeepEqual(moments, []stats.Moment{{Date: "20261027", Time: "0815"}, {Date: "20261027", Time: "0815"}}) {
		t.Fatalf("resultado incorreto: %v %v %v", moments, truncated, err)
	}
	count = stats.MaxStudies + 1
	moments, truncated, err = simulatedClient(t, server).StudyMoments(context.Background(), studiesConfig(), from, to)
	if err != nil || !truncated || len(moments) != stats.MaxStudies {
		t.Fatalf("truncamento incorreto: %d %v %v", len(moments), truncated, err)
	}
	if _, _, err := simulatedClient(t, server).StudyMoments(context.Background(), studiesConfig(), to, from); !errors.Is(err, InvalidTarget) {
		t.Fatal("intervalo invertido aceito")
	}
}

func TestArchiveStatistics(t *testing.T) {
	body := `{"CountPatients":3,"CountStudies":4,"CountSeries":5,"CountInstances":6,"TotalDiskSizeMB":7,"TotalDiskSize":"7340032"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/pacs/statistics" {
			t.Errorf("operação não permitida: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	archive, err := simulatedClient(t, server).ArchiveStatistics(context.Background(), studiesConfig())
	if err != nil || archive != (stats.Archive{Studies: 4, Series: 5, Instances: 6, DiskMB: 7}) {
		t.Fatalf("estatísticas incorretas: %+v %v", archive, err)
	}
	for _, invalid := range []string{`{}`, `{"CountStudies":-1,"CountSeries":1,"CountInstances":1,"TotalDiskSizeMB":1}`, `[]`} {
		body = invalid
		if _, err := simulatedClient(t, server).ArchiveStatistics(context.Background(), studiesConfig()); !errors.Is(err, InvalidResponse) {
			t.Fatalf("resposta inválida aceita: %s", invalid)
		}
	}
}
