package stats_test

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pmfb-saude/pacs-web/backend/internal/database"
	"github.com/pmfb-saude/pacs-web/backend/internal/stats"
)

// Never reads .env/DATABASE_URL; cluster belongs only to this test.
func disposable(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	binary, err := exec.LookPath("postgres")
	if err != nil {
		t.Skip("servidor PostgreSQL local não instalado; teste de integração exige postgres e initdb no PATH")
	}
	initdb := filepath.Join(filepath.Dir(binary), "initdb")
	if _, err := os.Stat(initdb); err != nil {
		t.Skip("initdb não disponível ao lado de postgres")
	}
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	command := exec.Command(initdb, "-D", data, "-U", "unit_test", "-A", "trust", "--no-locale", "-E", "UTF8")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initdb sintético: %v: %s", err, out)
	}
	// Socket curto para respeitar sockaddr_un no macOS.
	socket, err := os.MkdirTemp("", "pacs-stats-pg-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socket) })
	server := exec.Command(binary, "-D", data, "-k", socket, "-h", "", "-F")
	server.Stdout = io.Discard
	server.Stderr = io.Discard
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Process.Signal(syscall.SIGTERM); _ = server.Wait() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	uri := "postgresql://unit_test@/postgres?host=" + url.QueryEscape(socket) + "&sslmode=disable"
	pool, err := pgxpool.New(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for pool.Ping(ctx) != nil {
		select {
		case <-ctx.Done():
			t.Fatal("cluster sintético não iniciou")
		case <-time.After(50 * time.Millisecond):
		}
	}

	return ctx, pool
}

func TestPostgresStoreDaysMissingAndArchive(t *testing.T) {
	ctx, pool := disposable(t)
	if err := database.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	s := stats.NewStore(pool)
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, time.UTC) }
	first := stats.Daily{Day: d(10, 27), Exams: 3, WithoutTime: 1}
	first.ByHour[8] = 2
	if err := s.SaveDays(ctx, []stats.Daily{first, {Day: d(10, 29)}}); err != nil {
		t.Fatal(err)
	}
	first.Exams, first.Truncated = 4, true
	first.ByHour[9] = 1
	if err := s.SaveDays(ctx, []stats.Daily{first}); err != nil { // recalcula
		t.Fatal(err)
	}
	days, err := s.Days(ctx, d(10, 26), d(10, 29))
	if err != nil || len(days) != 2 || days[0] != first || !days[1].Day.Equal(d(10, 29)) {
		t.Fatalf("dias: %+v %v", days, err)
	}
	missing, err := s.MissingDays(ctx, d(10, 25), d(10, 30), 3)
	if err != nil || len(missing) != 3 || !missing[0].Equal(d(10, 30)) || !missing[1].Equal(d(10, 28)) || !missing[2].Equal(d(10, 26)) {
		t.Fatalf("pendentes: %v %v", missing, err)
	}
	if err := s.SaveDays(ctx, []stats.Daily{{Day: d(10, 1), Exams: 1, WithoutTime: 2}}); err == nil {
		t.Fatal("aceitou without_time maior que exams")
	}
	for _, snap := range []struct {
		day  time.Time
		disk int64
	}{{d(9, 10), 90}, {d(9, 30), 100}, {d(10, 5), 120}, {d(10, 29), 150}, {d(10, 29), 155}} {
		if err := s.SaveArchive(ctx, snap.day, stats.Archive{Studies: snap.disk / 10, DiskMB: snap.disk}); err != nil {
			t.Fatal(err)
		}
	}
	history, err := s.ArchiveHistory(ctx, d(9, 1), d(10, 30))
	want := []stats.ArchivePoint{{Month: "2026-09", Studies: 10, DiskMB: 100}, {Month: "2026-10", Studies: 15, DiskMB: 155}}
	if err != nil || len(history) != 2 || history[0] != want[0] || history[1] != want[1] {
		t.Fatalf("histórico: %+v %v", history, err)
	}
}
