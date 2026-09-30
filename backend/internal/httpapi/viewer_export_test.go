package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

func TestViewerExportAudit(t *testing.T) {
	for _, role := range []user.Role{"", user.RoleAdmin, user.RoleGestor, user.RoleMedico, "INVALID"} {
		c := montarCenario(t, opcoesCenario{})
		c.aquecerCSRF(t)
		if role != "" {
			loginStudies(t, c, role)
		}
		code := 204
		if role == "" {
			code = 401
		}
		if role == "INVALID" {
			code = 403
		}
		for _, format := range []string{"PNG", "JPEG", "PDF"} {
			for _, identified := range []bool{true, false} {
				expectStatus(t, c.requisitar(t, "POST", "/api/viewer/exports", map[string]any{"format": format, "identified": identified}), code)
			}
		}
		n := 0
		for _, e := range c.auditoria.Entradas() {
			if e.Event == audit.EventViewerImageExported {
				n++
				if e.ActorUserID == nil || e.ActorUsername != "consulta.teste" || e.UnitID != nil || !strings.HasPrefix(e.Detail, "format=") {
					t.Fatal("unsafe attribution")
				}
			}
		}
		if code == 204 && n != 6 || code != 204 && n != 0 {
			t.Fatal("event count")
		}
	}
}
func TestViewerExportRejectsArbitraryMetadata(t *testing.T) {
	c := montarCenario(t, opcoesCenario{})
	loginStudies(t, c, user.RoleMedico)
	for _, body := range []map[string]any{
		{"format": "DICOM", "identified": false}, {"format": "PNG", "identified": "false"}, {"format": "PNG"}, {"format": "PNG", "identified": nil},
		{"format": "PNG", "identified": false, "actorId": "synthetic"}, {"format": "PNG", "identified": false, "event": "USER_CREATED"},
		{"format": "PNG", "identified": true, "PatientName": "PACIENTE TESTE"}, {"format": "PDF", "identified": false, "metadata": map[string]string{"token": "synthetic"}},
	} {
		expectStatus(t, c.requisitar(t, "POST", "/api/viewer/exports", body), 400)
	}
	expectStatus(t, c.requisitar(t, "POST", "/api/viewer/exports?patientId=synthetic", map[string]any{"format": "PNG", "identified": false}), 400)
	for _, e := range c.auditoria.Entradas() {
		if e.Event == audit.EventViewerImageExported {
			t.Fatal("invalid request audited")
		}
	}
}

func TestViewerExportRequiresCSRF(t *testing.T) {
	c := montarCenario(t, opcoesCenario{})
	loginStudies(t, c, user.RoleMedico)
	req, err := http.NewRequest(http.MethodPost, c.servidor.URL+"/api/viewer/exports", strings.NewReader(`{"format":"PNG","identified":false}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.cliente.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	for _, e := range c.auditoria.Entradas() {
		if e.Event == audit.EventViewerImageExported {
			t.Fatal("invalid CSRF audited")
		}
	}
}
