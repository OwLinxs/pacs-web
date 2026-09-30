package units_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/database"
	"github.com/pmfb-saude/pacs-web/backend/internal/units"
)

// Nunca lê DATABASE_URL/.env ou conecta a um serviço existente. Se os binários
// não estiverem presentes, informa skip. Quando disponíveis, inicia cluster
// descartável em diretório temporário, somente socket Unix, sem porta TCP.
func TestStoreAndMigrationPostgres(t *testing.T) {
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
	socket, err := os.MkdirTemp("", "pacs-units-pg-")
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
	defer cancel()
	uri := "postgresql://unit_test@/postgres?host=" + url.QueryEscape(socket) + "&sslmode=disable"
	pool, err := pgxpool.New(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for pool.Ping(ctx) != nil {
		select {
		case <-ctx.Done():
			t.Fatal("cluster sintético não iniciou")
		case <-time.After(50 * time.Millisecond):
		}
	}
	migrations, err := database.LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 4 {
		t.Fatal("atualize fixture para as migrations atuais")
	}
	if _, err := pool.Exec(ctx, migrations[0].SQL); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `CREATE TABLE schema_migrations(version integer PRIMARY KEY,name text NOT NULL,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO schema_migrations(version,name,checksum) VALUES(1,$1,$2)`, migrations[0].Name, migrations[0].Checksum)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	duplicate := uuid.New()
	userID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO units(id,slug,name,kind) VALUES($1,'legacy-one','Unidade Legada','UBS'),($2,'legacy-two','unidade legada ','OUTRO')`, id, duplicate)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO users(id,name,username,password_hash,role,unit_id) VALUES($1,'Usuário Sintético','synthetic-user','hash-ficticio-nao-utilizavel','MEDICO',$2)`, userID, id)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if database.Migrate(ctx, pool, log) == nil {
		t.Fatal("migration aceitou duplicidade legada")
	}
	_, err = pool.Exec(ctx, `UPDATE units SET name=' ' WHERE id=$1`, duplicate)
	if err != nil {
		t.Fatal(err)
	}
	if database.Migrate(ctx, pool, log) == nil {
		t.Fatal("migration aceitou nome vazio legado")
	}
	_, err = pool.Exec(ctx, `UPDATE units SET name='Outra Legada' WHERE id=$1`, duplicate)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, log); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, log); err != nil {
		t.Fatal("segunda aplicação", err)
	}
	var linked uuid.UUID
	var slug, kind, hash string
	err = pool.QueryRow(ctx, `SELECT u.unit_id,un.slug,un.kind,u.password_hash FROM users u JOIN units un ON un.id=u.unit_id WHERE u.id=$1`, userID).Scan(&linked, &slug, &kind, &hash)
	if err != nil || linked != id || slug != "legacy-one" || kind != "UBS" || hash != "hash-ficticio-nao-utilizavel" {
		t.Fatal("migration alterou unidade/usuário legado")
	}
	ctx = audit.WithActor(ctx, audit.Actor{ID: userID, Username: "synthetic-user"})
	store := units.NewStore(pool)
	made, err := store.Create(ctx, "  Unidade Nova  ")
	if err != nil || made.Name != "Unidade Nova" || !made.Active {
		t.Fatal("create", err)
	}
	if _, err = store.Create(ctx, "UNIDADE NOVA"); !errors.Is(err, units.ErrDuplicate) {
		t.Fatal("unicidade", err)
	}
	name := "Unidade Editada"
	off := false
	before, after, err := store.Update(ctx, made.ID, units.Patch{Name: &name, Active: &off})
	if err != nil || before.Name != made.Name || after.Active || after.Name != name || !after.CreatedAt.Equal(made.CreatedAt) || after.UpdatedAt.Before(made.UpdatedAt) {
		t.Fatal("update", err)
	}
	active, err := store.List(ctx, false, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range active {
		if u.ID == made.ID {
			t.Fatal("inativa listada por padrão")
		}
	}
	all, err := store.List(ctx, true, 50, 0)
	if err != nil || len(all) != 3 {
		t.Fatal("listagem administrativa", err)
	}
	on := true
	_, after, err = store.Update(ctx, made.ID, units.Patch{Active: &on})
	if err != nil || !after.Active {
		t.Fatal("reativar", err)
	}
	// A unicidade pertence ao banco, inclusive sob concorrência.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() { _, err := store.Create(ctx, "Concorrente Fictícia"); results <- err })
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, units.ErrDuplicate) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal(fmt.Sprintf("sucessos=%d conflitos=%d", successes, conflicts))
	}
}
