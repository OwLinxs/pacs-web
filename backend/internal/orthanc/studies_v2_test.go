package orthanc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/pmfb-saude/pacs-web/backend/internal/studies"
)

func TestStudiesV2GlobalOrderAndModality(t *testing.T) {
	for _, direction := range []string{"", "dateDesc", "dateAsc", "native"} {
		t.Run(direction, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/pacs/system" {
					if r.Method != "GET" {
						t.Error("system não é leitura")
					}
					_, _ = w.Write([]byte(`{"Version":"1.12.6","Capabilities":{"HasExtendedFind":true}}`))
					return
				}
				if r.URL.Path != "/pacs/tools/find" || r.Method != "POST" {
					t.Error("rota inesperada")
				}
				var payload struct {
					Query        map[string]string
					OrderBy      []map[string]string
					Limit, Since int
				}
				if json.NewDecoder(r.Body).Decode(&payload) != nil {
					t.Error("JSON inválido")
				}
				if payload.Query["ModalitiesInStudy"] != "CT" || payload.Query["StudyDate"] != "20260901-20260928" || payload.Limit != 26 || payload.Since != 50 {
					t.Error("consulta não preservou filtros e paginação")
				}
				if direction == "native" {
					if len(payload.OrderBy) != 0 {
						t.Error("ordem nativa adulterada")
					}
				} else {
					order := "DESC"
					if direction == "dateAsc" {
						order = "ASC"
					}
					want := []map[string]string{{"Type": "DicomTag", "Key": "StudyDate", "Direction": order}, {"Type": "DicomTag", "Key": "StudyTime", "Direction": order}, {"Type": "DicomTag", "Key": "StudyInstanceUID", "Direction": "ASC"}}
					if !reflect.DeepEqual(payload.OrderBy, want) {
						t.Error("ordenação global incorreta")
					}
				}
				// Ordem proposital: o cliente deve preservar a sequência global do servidor.
				_ = json.NewEncoder(w).Encode([]expandedStudy{{ID: fakeID(9), Type: "Study", Series: []string{}}, {ID: fakeID(1), Type: "Study", Series: []string{}}})
			}))
			defer server.Close()
			page, err := simulatedClient(t, server).FindStudies(context.Background(), studiesConfig(), studies.Query{Limit: 25, Offset: 50, Sort: direction, Modality: "CT", DateFrom: "2026-09-01", DateTo: "2026-09-28"})
			if err != nil || len(page.Items) != 2 || page.Items[0].OrthancStudyID != fakeID(9) || calls != 2 {
				t.Fatalf("resultado/quantidade de chamadas incorreto: %v", err)
			}
		})
	}
}

func TestStudiesV2CapabilitiesFailClosed(t *testing.T) {
	for _, tc := range []struct {
		body     string
		modality string
		want     error
	}{
		{`{"Version":"1.12.6","Capabilities":{"HasExtendedFind":false}}`, "", UnsupportedQuery},
		{`{"Version":"1.12.4"}`, "", UnsupportedQuery},
		{`{"Version":"1.12.5","Capabilities":{"HasExtendedFind":true}}`, "CT", UnsupportedQuery},
		{`{"Version":"mainline","Capabilities":{"HasExtendedFind":true}}`, "CT", UnsupportedQuery},
		{`{"Version":"1.12.6","Capabilities":[]}`, "", InvalidResponse},
		{`{`, "", InvalidResponse},
	} {
		t.Run(tc.body, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/pacs/system" {
					t.Error("find não deveria ser executado")
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := simulatedClient(t, server).FindStudies(context.Background(), studiesConfig(), studies.Query{Limit: 25, Sort: "dateDesc", Modality: tc.modality})
			if !errors.Is(err, tc.want) || calls != 1 {
				t.Fatalf("erro inesperado: %v", err)
			}
		})
	}
}
