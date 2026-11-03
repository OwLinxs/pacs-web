package terms_test

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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/database"
	"github.com/pmfb-saude/pacs-web/backend/internal/terms"
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
	socket, err := os.MkdirTemp("", "pacs-terms-pg-")
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

func TestPostgresAcceptOncePerVersionWithAudit(t *testing.T) {
	ctx, pool := disposable(t)
	if err := database.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users(name,username,password_hash,role) VALUES('Sintetico','sint_termo','x','MEDICO') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	s := terms.NewStore(pool)
	entry := audit.Entry{Event: audit.EventTermsAccepted, ActorUserID: &id, ActorUsername: "sint_termo", Detail: audit.TermsDetail(terms.Version)}
	if ok, err := s.Accepted(ctx, id, terms.Version); err != nil || ok {
		t.Fatal("aceite inexistente", ok, err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Accept(ctx, id, terms.Version, entry); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := s.Accepted(ctx, id, terms.Version); err != nil || !ok {
		t.Fatal("aceite não registrado", err)
	}
	if ok, _ := s.Accepted(ctx, id, "versao-futura"); ok {
		t.Fatal("outra versão não foi aceita")
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event='TERMS_ACCEPTED' AND actor_user_id=$1`, id).Scan(&events); err != nil || events != 1 {
		t.Fatalf("auditoria: %d %v", events, err)
	}
	bad := entry
	bad.Detail = "versão=<x>"
	if err := s.Accept(ctx, id, "outra", bad); err == nil {
		t.Fatal("auditoria inválida deveria desfazer o aceite")
	}
	if ok, _ := s.Accepted(ctx, id, "outra"); ok {
		t.Fatal("aceite gravado sem auditoria")
	}
}
