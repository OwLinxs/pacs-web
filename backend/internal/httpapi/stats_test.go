package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/stats"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

type fakeStats struct {
	calls   atomic.Int32
	fail    error
	moments []stats.Moment
}

func (f *fakeStats) StudyMoments(_ context.Context, _ orthanc.Config, from, to time.Time) ([]stats.Moment, bool, error) {
	f.calls.Add(1)
	return f.moments, false, f.fail
}
func (f *fakeStats) ArchiveStatistics(context.Context, orthanc.Config) (stats.Archive, error) {
	return stats.Archive{Studies: 7, Instances: 21, DiskMB: 3}, f.fail
}

type usageFunc func(stats.Period, stats.Period) (stats.Usage, error)

func (f usageFunc) Usage(_ context.Context, cur, prev stats.Period) (stats.Usage, error) {
	return f(cur, prev)
}

func TestStatsAuthorizationAndPeriods(t *testing.T) {
	for _, tc := range []struct {
		role user.Role
		want int
	}{{"", 401}, {user.RoleAdmin, 200}, {user.RoleGestor, 200}, {user.RoleMedico, 403}} {
		t.Run(string(tc.role), func(t *testing.T) {
			source := &fakeStats{}
			c := montarCenario(t, opcoesCenario{stats: source})
			seedStudiesSettings(t, c)
			if tc.role != "" {
				loginStudies(t, c, tc.role)
			}
			r := c.requisitar(t, "GET", "/api/stats?period=week&date=2026-10-29", nil)
			defer r.Body.Close()
			if r.StatusCode != tc.want {
				t.Fatalf("status=%d esperado=%d", r.StatusCode, tc.want)
			}
			if (source.calls.Load() > 0) != (tc.want == 200) {
				t.Fatal("autorização não bloqueou a consulta ao PACS")
			}
		})
	}
	c := montarCenario(t, opcoesCenario{stats: &fakeStats{}})
	loginStudies(t, c, user.RoleAdmin)
	for _, query := range []string{"period=year", "period=day&date=2026-13-01", "period=day&date=2999-01-01", "period=day&period=week", "periodo=day"} {
		r := c.requisitar(t, "GET", "/api/stats?"+query, nil)
		r.Body.Close()
		if r.StatusCode != 400 {
			t.Fatalf("%s aceito: %d", query, r.StatusCode)
		}
	}
}

func TestStatsAggregatesAndPartialFailure(t *testing.T) {
	source := &fakeStats{moments: []stats.Moment{{Date: "20261027", Time: "081500"}, {Date: "20261027", Time: "0930"}, {Date: "20261029", Time: ""}}}
	usage := usageFunc(func(cur, prev stats.Period) (stats.Usage, error) {
		if cur.Kind == "week" && (cur.From.Format("2006-01-02") != "2026-10-26" || prev.From.Format("2006-01-02") != "2026-10-19") {
			t.Errorf("períodos incorretos: %v %v", cur, prev)
		}
		return stats.Usage{Logins: 5, ActiveUsers: 2}, nil
	})
	c := montarCenario(t, opcoesCenario{stats: source, usage: usage})
	seedStudiesSettings(t, c)
	loginStudies(t, c, user.RoleGestor)
	r := c.requisitar(t, "GET", "/api/stats?period=week&date=2026-10-29", nil)
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close()
	var body struct {
		From, To, PreviousFrom string
		Exams                  *stats.Exams
		Archive                *stats.Archive
		Usage                  *stats.Usage
	}
	if r.StatusCode != 200 || json.Unmarshal(raw, &body) != nil {
		t.Fatalf("resposta inválida: %d", r.StatusCode)
	}
	if body.From != "2026-10-26" || body.To != "2026-11-01" || body.PreviousFrom != "2026-10-19" {
		t.Fatalf("semana incorreta: %+v", body)
	}
	if body.Exams == nil || body.Exams.Total != 3 || body.Exams.PreviousTotal != 0 || len(body.Exams.Buckets) != 7 || body.Exams.Buckets[1].Count != 2 || body.Exams.ByHour[8] != 1 || body.Exams.WithoutTime != 1 {
		t.Fatalf("agregação incorreta: %s", raw)
	}
	if body.Archive == nil || body.Archive.Studies != 7 || body.Usage == nil || body.Usage.Logins != 5 {
		t.Fatalf("seções ausentes: %s", raw)
	}
	for _, unwanted := range []string{"credencial", "orthanc.test", "Patient"} {
		if strings.Contains(string(raw), unwanted) {
			t.Fatalf("resposta expôs %q", unwanted)
		}
	}

	// PACS indisponível: uso continua; exames ficam nulos com mensagem genérica.
	source.fail = orthanc.Unavailable
	r = c.requisitar(t, "GET", "/api/stats?period=day", nil)
	raw, _ = io.ReadAll(r.Body)
	r.Body.Close()
	var partial struct {
		Exams      *stats.Exams
		ExamsError string
		Archive    *stats.Archive
		Usage      *stats.Usage
	}
	if r.StatusCode != 200 || json.Unmarshal(raw, &partial) != nil || partial.Exams != nil || partial.Archive != nil || partial.Usage == nil || partial.ExamsError == "" {
		t.Fatalf("falha parcial incorreta: %d %s", r.StatusCode, raw)
	}
}
