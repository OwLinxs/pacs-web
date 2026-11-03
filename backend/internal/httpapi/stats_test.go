package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
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
	mu      sync.Mutex
	ranges  []string
}

func (f *fakeStats) StudyMoments(_ context.Context, _ orthanc.Config, from, to time.Time) ([]stats.Moment, bool, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.ranges = append(f.ranges, from.Format("2006-01-02")+".."+to.Format("2006-01-02"))
	f.mu.Unlock()
	return f.moments, false, f.fail
}
func (f *fakeStats) ArchiveStatistics(context.Context, orthanc.Config) (stats.Archive, error) {
	return stats.Archive{Studies: 7, Instances: 21, DiskMB: 3}, f.fail
}

type usageFunc func(stats.Period, stats.Period) (stats.Usage, error)

func (f usageFunc) Usage(_ context.Context, cur, prev stats.Period) (stats.Usage, error) {
	return f(cur, prev)
}
func (f usageFunc) UnitAccess(context.Context, stats.Period) ([]stats.UnitAccess, error) {
	return []stats.UnitAccess{{Name: "UPA Sintética", ActiveUsers: 2, Logins: 5}}, nil
}

type fakeStore struct {
	days []stats.Daily
	fail error
}

func (f *fakeStore) Days(_ context.Context, from, to time.Time) ([]stats.Daily, error) {
	out := []stats.Daily{}
	for _, d := range f.days {
		if !d.Day.Before(from) && !d.Day.After(to) {
			out = append(out, d)
		}
	}
	return out, f.fail
}
func (f *fakeStore) ArchiveHistory(context.Context, time.Time, time.Time) ([]stats.ArchivePoint, error) {
	return []stats.ArchivePoint{{Month: "2026-09", Studies: 5, DiskMB: 100}, {Month: "2026-10", Studies: 7, DiskMB: 150}}, f.fail
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
	for _, query := range []string{"period=quarter", "period=day&date=2026-13-01", "period=day&date=2999-01-01", "period=day&period=week", "periodo=day"} {
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

func TestStatsUsesStoredDaysAndOnlyFetchesGaps(t *testing.T) {
	// 30/10/2026 12:00 em São Paulo.
	now := func() time.Time { return time.Date(2026, 10, 30, 15, 0, 0, 0, time.UTC) }
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, time.UTC) }
	store := &fakeStore{}
	for day := d(10, 1); day.Before(d(10, 30)); day = day.AddDate(0, 0, 1) {
		daily := stats.Daily{Day: day, Exams: 2}
		daily.ByHour[9] = 2
		store.days = append(store.days, daily)
	}
	source := &fakeStats{moments: []stats.Moment{{Date: "20261030", Time: "1000"}}}
	usage := usageFunc(func(stats.Period, stats.Period) (stats.Usage, error) { return stats.Usage{}, nil })
	c := montarCenario(t, opcoesCenario{stats: source, usage: usage, statsStore: store, now: now})
	seedStudiesSettings(t, c)
	loginStudies(t, c, user.RoleAdmin)

	get := func(query string) (map[string]json.RawMessage, *stats.Exams) {
		t.Helper()
		r := c.requisitar(t, "GET", "/api/stats?"+query, nil)
		raw, _ := io.ReadAll(r.Body)
		r.Body.Close()
		var body map[string]json.RawMessage
		var exams *stats.Exams
		if r.StatusCode != 200 || json.Unmarshal(raw, &body) != nil || json.Unmarshal(body["exams"], &exams) != nil {
			t.Fatalf("%s: %d %s", query, r.StatusCode, raw)
		}
		return body, exams
	}

	// Mês corrente: 1–29/10 gravados (+ setembro ausente), só hoje e o mês anterior vêm do PACS.
	body, exams := get("period=month")
	if exams == nil || exams.Total != 29*2+1 || exams.Buckets[29].Count != 1 || exams.ByHour[9] != 58 || exams.PreviousTotal != 0 {
		t.Fatalf("mês: %+v", exams)
	}
	source.mu.Lock()
	ranges := append([]string(nil), source.ranges...)
	source.ranges = nil
	source.mu.Unlock()
	if len(ranges) != 2 || !contains(ranges, "2026-10-30..2026-10-30") || !contains(ranges, "2026-09-01..2026-09-30") {
		t.Fatalf("intervalos ao vivo: %v", ranges)
	}
	if !strings.Contains(string(body["units"]), "UPA Sintética") || !strings.Contains(string(body["archiveHistory"]), "2026-10") {
		t.Fatalf("unidades/acervo ausentes: %s %s", body["units"], body["archiveHistory"])
	}

	// Semana passada inteira gravada: PACS fora do ar não impede os exames.
	source.fail = orthanc.Unavailable
	_, exams = get("period=week&date=2026-10-20")
	if exams == nil || exams.Total != 14 || exams.PreviousTotal != 14 {
		t.Fatalf("semana gravada: %+v", exams)
	}
	source.fail = nil

	// Ano: 12 meses; banco indisponível cai para o PACS ao vivo no ano inteiro.
	_, exams = get("period=year")
	source.mu.Lock()
	ranges = source.ranges
	source.mu.Unlock()
	if exams == nil || len(exams.Buckets) != 12 || exams.Buckets[9].Count != 59 || !contains(ranges, "2026-01-01..2026-09-30") || !contains(ranges, "2026-10-30..2026-10-30") {
		t.Fatalf("ano: %+v", exams)
	}
	store.fail = stats.ErrStore
	source.ranges = nil // sem concorrência: a requisição anterior já terminou
	body, exams = get("period=year")
	if exams == nil || len(source.ranges) != 2 || !contains(source.ranges, "2026-01-01..2026-10-30") || string(body["archiveHistory"]) != "null" {
		t.Fatalf("fallback: %v %s", source.ranges, body["archiveHistory"])
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
