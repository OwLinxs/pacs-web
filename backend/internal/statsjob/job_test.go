package statsjob

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
	"github.com/pmfb-saude/pacs-web/backend/internal/stats"
)

type call struct{ from, to string }

type fakeSource struct {
	calls     []call
	truncWide bool
	fail      bool
}

func (f *fakeSource) StudyMoments(_ context.Context, _ orthanc.Config, from, to time.Time) ([]stats.Moment, bool, error) {
	f.calls = append(f.calls, call{from.Format("2006-01-02"), to.Format("2006-01-02")})
	if f.fail {
		return nil, false, orthanc.Unavailable
	}
	return []stats.Moment{{Date: from.Format("20060102"), Time: "0815"}}, f.truncWide && from.Before(to), nil
}
func (f *fakeSource) ArchiveStatistics(context.Context, orthanc.Config) (stats.Archive, error) {
	return stats.Archive{Studies: 3, DiskMB: 10}, nil
}

type fakeStore struct {
	days    map[string]stats.Daily
	archive map[string]stats.Archive
}

func (f *fakeStore) SaveDays(_ context.Context, days []stats.Daily) error {
	for _, d := range days {
		f.days[d.Day.Format("2006-01-02")] = d
	}
	return nil
}
func (f *fakeStore) MissingDays(_ context.Context, from, to time.Time, limit int) ([]time.Time, error) {
	out := []time.Time{}
	for d := to; !d.Before(from) && len(out) < limit; d = d.AddDate(0, 0, -1) {
		if _, ok := f.days[d.Format("2006-01-02")]; !ok {
			out = append(out, d)
		}
	}
	return out, nil
}
func (f *fakeStore) SaveArchive(_ context.Context, day time.Time, a stats.Archive) error {
	f.archive[day.Format("2006-01-02")] = a
	return nil
}

type fakeSettings struct{ err error }

func (f fakeSettings) OrthancConnection(context.Context) (settings.ConnectionSnapshot, error) {
	return settings.ConnectionSnapshot{Config: settings.Orthanc{BaseURL: "http://orthanc.test:8042", TimeoutSeconds: 10}}, f.err
}

func newJob(src *fakeSource, store *fakeStore, cfg Settings) *Job {
	j := New(src, store, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	j.now = func() time.Time { return time.Date(2026, 10, 30, 2, 0, 0, 0, time.UTC) } // 29/10 em São Paulo
	return j
}

func TestRunOnceFillsRecentAndBackfill(t *testing.T) {
	src, store := &fakeSource{}, &fakeStore{days: map[string]stats.Daily{}, archive: map[string]stats.Archive{}}
	j := newJob(src, store, fakeSettings{})
	j.RunOnce(context.Background())
	if store.archive["2026-10-29"].Studies != 3 {
		t.Fatalf("acervo não registrado: %v", store.archive)
	}
	if src.calls[0] != (call{"2026-10-22", "2026-10-28"}) {
		t.Fatalf("recalculo recente: %v", src.calls[0])
	}
	if _, ok := store.days["2026-10-29"]; ok {
		t.Fatal("dia corrente não deve ser gravado")
	}
	if len(store.days) != RecentDays+perRun || store.days["2026-10-22"].Exams != 1 || store.days["2026-10-22"].ByHour[8] != 1 {
		t.Fatalf("dias gravados: %d", len(store.days))
	}
	for _, c := range src.calls[1:] {
		from, _ := time.Parse("2006-01-02", c.from)
		to, _ := time.Parse("2006-01-02", c.to)
		if to.Sub(from) >= chunk*24*time.Hour || !to.Before(time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("bloco inválido: %v", c)
		}
	}

	// Segunda execução no mesmo dia: não recalcula os recentes, só continua o preenchimento.
	src.calls = nil
	j.RunOnce(context.Background())
	if len(src.calls) == 0 || src.calls[0].to == "2026-10-28" {
		t.Fatalf("recalculou recentes de novo: %v", src.calls)
	}
	keys := make([]string, 0, len(store.days))
	for k := range store.days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(store.days) != RecentDays+2*perRun || keys[len(keys)-1] != "2026-10-28" {
		t.Fatalf("preenchimento: %d até %s", len(store.days), keys[len(keys)-1])
	}
}

func TestRunOnceSplitsTruncatedAndStopsOnFailure(t *testing.T) {
	src, store := &fakeSource{truncWide: true}, &fakeStore{days: map[string]stats.Daily{}, archive: map[string]stats.Archive{}}
	j := newJob(src, store, fakeSettings{})
	if !j.collect(context.Background(), orthanc.Config{}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("coleta falhou")
	}
	if len(src.calls) != 4 || len(store.days) != 3 || store.days["2026-10-02"].Exams != 1 || store.days["2026-10-02"].Truncated {
		t.Fatalf("divisão por dia: %v %v", src.calls, store.days)
	}

	failing, empty := &fakeSource{fail: true}, &fakeStore{days: map[string]stats.Daily{}, archive: map[string]stats.Archive{}}
	newJob(failing, empty, fakeSettings{}).RunOnce(context.Background())
	if len(failing.calls) != 2 || len(empty.days) != 0 {
		t.Fatalf("deveria parar após falha: %v", failing.calls)
	}

	unconfigured := &fakeSource{}
	newJob(unconfigured, empty, fakeSettings{err: errors.New("x")}).RunOnce(context.Background())
	if len(unconfigured.calls) != 0 {
		t.Fatal("consultou PACS sem configuração")
	}
}
