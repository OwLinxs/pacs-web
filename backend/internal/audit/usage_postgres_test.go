package audit_test

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
	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
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
	socket, err := os.MkdirTemp("", "pacs-usage-pg-")
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

func TestPostgresUsageCivilDaysAndPreviousPeriod(t *testing.T) {
	ctx, pool := disposable(t)
	if err := database.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users(name,username,password_hash,role) VALUES('Sintetico A','sint_a','x','MEDICO'),('Sintetico B','sint_b','x','MEDICO')`); err != nil {
		t.Fatal(err)
	}
	rows, _ := pool.Query(ctx, `SELECT id::text FROM users ORDER BY username`)
	ids := []string{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 2 {
		t.Fatalf("usuários sintéticos: %v", ids)
	}
	// 2026-10-29 civil em São Paulo = [03:00Z de 29, 03:00Z de 30).
	insert := func(at, event, actor string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO audit_events(occurred_at,event,actor_user_id) VALUES($1,$2,NULLIF($3,'')::uuid)`, at, event, actor); err != nil {
			t.Fatal(err)
		}
	}
	insert("2026-10-29T03:00:00Z", "LOGIN_SUCCESS", ids[0])
	insert("2026-10-29T12:00:00Z", "LOGIN_SUCCESS", ids[0])
	insert("2026-10-30T02:59:59Z", "LOGIN_SUCCESS", ids[1])
	insert("2026-10-29T13:00:00Z", "LOGIN_FAILURE", "")
	insert("2026-10-29T14:00:00Z", "VIEWER_IMAGE_EXPORTED", ids[1])
	insert("2026-10-29T15:00:00Z", "USER_CREATED", ids[0])          // fora das métricas
	insert("2026-10-29T02:59:59Z", "LOGIN_SUCCESS", ids[1])         // dia anterior
	insert("2026-10-30T03:00:00Z", "LOGIN_SUCCESS", ids[0])         // dia seguinte
	insert("2026-10-28T10:00:00Z", "VIEWER_IMAGE_EXPORTED", ids[0]) // dia anterior
	day := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	current := stats.Period{Kind: "day", From: day, To: day}
	u, err := audit.NewRecorder(pool, slog.New(slog.NewTextHandler(io.Discard, nil))).Usage(ctx, current, current.Previous())
	if err != nil {
		t.Fatal(err)
	}
	want := stats.Usage{Logins: 3, LoginFailures: 1, ActiveUsers: 2, Exports: 1, PreviousLogins: 1, PreviousActiveUsers: 1, PreviousExports: 1}
	if u != want {
		t.Fatalf("uso incorreto: %+v", u)
	}
}
