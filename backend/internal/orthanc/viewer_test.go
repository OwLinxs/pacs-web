package orthanc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/viewer"
)

func syntheticPart10() []byte {
	return append(append(make([]byte, 128), []byte("DICM")...), []byte("CONTEUDO-SINTETICO")...)
}

func TestViewerMetadataAndMembership(t *testing.T) {
	studyID, seriesID, instanceID := fakeID(1), fakeID(2), fakeID(3)
	var files atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("operação de escrita")
		}
		switch r.URL.RequestURI() {
		case "/pacs/studies/" + studyID + "/series?expand=true":
			_ = json.NewEncoder(w).Encode([]viewerSeries{{ID: seriesID, Type: "Series", ParentStudy: studyID, Instances: []string{instanceID}, MainDicomTags: map[string]string{"Modality": "OT", "SeriesNumber": "1", "SeriesDescription": "Serie ficticia"}}})
		case "/pacs/series/" + seriesID:
			_ = json.NewEncoder(w).Encode(viewerSeries{ID: seriesID, Type: "Series", ParentStudy: studyID, Instances: []string{instanceID}})
		case "/pacs/series/" + seriesID + "/instances?expand=true":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"ID": instanceID, "Type": "Instance", "ParentSeries": seriesID, "IndexInSeries": 1}})
		case "/pacs/instances/" + instanceID + "/file":
			files.Add(1)
			w.Header().Set("Content-Type", "application/dicom")
			_, _ = w.Write(syntheticPart10())
		default:
			t.Error("caminho inesperado")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, cfg := simulatedClient(t, server), studiesConfig()
	series, err := client.ViewerSeries(context.Background(), cfg, studyID)
	if err != nil || len(series) != 1 || series[0].InstanceCount != 1 || series[0].Modality != "OT" {
		t.Fatal("séries inválidas", err)
	}
	instances, err := client.ViewerInstances(context.Background(), cfg, studyID, seriesID)
	if err != nil || len(instances) != 1 || instances[0].OrthancInstanceID != instanceID {
		t.Fatal("instâncias inválidas", err)
	}
	file, err := client.OpenDICOM(context.Background(), cfg, studyID, seriesID, instanceID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(file.Body)
	file.Body.Close()
	if err != nil || !bytes.Equal(body, syntheticPart10()) {
		t.Fatal("stream divergente", err)
	}
	if _, err := client.ViewerInstances(context.Background(), cfg, fakeID(9), seriesID); !errors.Is(err, NotFound) {
		t.Fatal("vínculo de estudo não validado")
	}
	if _, err := client.OpenDICOM(context.Background(), cfg, studyID, seriesID, fakeID(9)); !errors.Is(err, NotFound) {
		t.Fatal("vínculo de instância não validado")
	}
	if files.Load() != 1 {
		t.Fatal("arquivo consultado fora do vínculo")
	}
}

func TestViewerEmptyAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"sem séries", "[]", 200, nil}, {"indisponível", "secret-ficticio", 503, Upstream},
		{"não encontrado", "secret-ficticio", 404, NotFound}, {"inválido", "{", 200, InvalidResponse},
		{"null", "null", 200, InvalidResponse}, {"redirect", "", 302, Redirect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "/fora")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			items, err := simulatedClient(t, server).ViewerSeries(context.Background(), studiesConfig(), fakeID(1))
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if tc.want == nil && (items == nil || len(items) != 0) {
				t.Fatal("vazio incorreto")
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("erro não sanitizado")
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(viewerSeries{ID: fakeID(2), Type: "Series", ParentStudy: fakeID(1), Instances: []string{}})
	}))
	defer server.Close()
	items, err := simulatedClient(t, server).ViewerInstances(context.Background(), studiesConfig(), fakeID(1), fakeID(2))
	if err != nil || items == nil || len(items) != 0 {
		t.Fatal("série vazia incorreta", err)
	}
}

func TestDICOMValidationAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name, contentType string
		body              []byte
		size              int64
		want              error
	}{
		{"dicom", "application/dicom", syntheticPart10(), 0, nil},
		{"octet-stream", "application/octet-stream", syntheticPart10(), 0, nil},
		{"html", "text/html", syntheticPart10(), 0, InvalidResponse},
		{"prefixo inválido", "application/dicom", make([]byte, 150), 0, InvalidResponse},
		{"curto", "application/dicom", []byte("FICTICIO"), 0, InvalidResponse},
		{"muito grande", "application/dicom", nil, viewer.MaxDICOMBytes + 1, TooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/pacs/series/"+fakeID(2) {
					_ = json.NewEncoder(w).Encode(viewerSeries{ID: fakeID(2), Type: "Series", ParentStudy: fakeID(1), Instances: []string{fakeID(3)}})
					return
				}
				w.Header().Set("Content-Type", tc.contentType)
				if tc.size > 0 {
					w.Header().Set("Content-Length", strconv.FormatInt(tc.size, 10))
				}
				_, _ = w.Write(tc.body)
			}))
			defer server.Close()
			file, err := simulatedClient(t, server).OpenDICOM(context.Background(), studiesConfig(), fakeID(1), fakeID(2), fakeID(3))
			if !errors.Is(err, tc.want) {
				t.Fatalf("erro=%v esperado=%v", err, tc.want)
			}
			if err == nil {
				file.Body.Close()
			}
		})
	}
	// Limite também vale sem Content-Length: nunca entrega o byte excedente.
	stream := &dicomStream{reader: strings.NewReader("123456789"), closer: io.NopCloser(strings.NewReader("")), cleanup: func() {}, remaining: 8}
	body, err := io.ReadAll(stream)
	stream.Close()
	if string(body) != "12345678" || !errors.Is(err, TooLarge) {
		t.Fatal("limite de stream ausente")
	}
}

func TestDICOMStreamsBeforeCompletionAndCancels(t *testing.T) {
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pacs/series/"+fakeID(2) {
			_ = json.NewEncoder(w).Encode(viewerSeries{ID: fakeID(2), Type: "Series", ParentStudy: fakeID(1), Instances: []string{fakeID(3)}})
			return
		}
		w.Header().Set("Content-Type", "application/dicom")
		_, _ = w.Write(syntheticPart10()[:132])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	file, err := simulatedClient(t, server).OpenDICOM(ctx, studiesConfig(), fakeID(1), fakeID(2), fakeID(3))
	if err != nil {
		t.Fatal("esperou o arquivo inteiro", err)
	}
	if file.Size != -1 {
		t.Fatal("fixture deve usar tamanho desconhecido")
	}
	prefix := make([]byte, 132)
	if _, err := io.ReadFull(file.Body, prefix); err != nil {
		t.Fatal(err)
	}
	cancel()
	file.Body.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("cancelamento não chegou ao Orthanc fictício")
	}
}

func TestViewerValidationBeforeNetworkAndTimeout(t *testing.T) {
	client := NewClient()
	cfg := studiesConfig()
	if _, err := client.ViewerSeries(context.Background(), cfg, "../system"); !errors.Is(err, InvalidTarget) {
		t.Fatal(err)
	}
	if _, err := client.ViewerInstances(context.Background(), cfg, fakeID(1), "invalid"); !errors.Is(err, InvalidTarget) {
		t.Fatal(err)
	}
	if _, err := client.OpenDICOM(context.Background(), cfg, fakeID(1), fakeID(2), "invalid"); !errors.Is(err, InvalidTarget) {
		t.Fatal(err)
	}
	cfg.BaseURL = "http://169.254.169.254"
	if _, err := client.ViewerSeries(context.Background(), cfg, fakeID(1)); !errors.Is(err, BlockedTarget) {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	cfg = studiesConfig()
	cfg.Timeout = 40 * time.Millisecond
	if _, err := simulatedClient(t, server).ViewerSeries(context.Background(), cfg, fakeID(1)); !errors.Is(err, Timeout) {
		t.Fatal(err)
	}
}
