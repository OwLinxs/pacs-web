package useradmin_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/database"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
	"github.com/pmfb-saude/pacs-web/backend/internal/useradmin"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
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
func TestPostgresMigrationAndUsers(t *testing.T) {
	ctx, pool := disposable(t)
	migrations, err := database.LoadMigrations()
	if err != nil || len(migrations) != 4 {
		t.Fatal("migrations")
	}
	for _, m := range migrations[:2] {
		if _, err = pool.Exec(ctx, m.SQL); err != nil {
			t.Fatal(err)
		}
	}
	adminID, legacyID, unitA, unitB, inactive := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	hasher := auth.NewHasher(auth.Argon2Params{Memory: 1024, Time: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	initial := "synthetic-initial-password"
	hash, err := hasher.Hash(initial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO units(id,slug,name,kind,active) VALUES($1,'unit-a','Unidade Sintética A','OUTRO',true),($2,'unit-b','Unidade Sintética B','OUTRO',true),($3,'unit-inactive','Unidade Sintética Inativa','OUTRO',false)`, unitA, unitB, inactive); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO users(id,name,username,role,password_hash,unit_id,access_valid_until) VALUES($1,'Administrador Sintético','administrator','ADMIN',$3,NULL,NULL),($2,'Legado Sintético','legacy.incompatible','MEDICO',$3,$4,'2027-01-31')`, adminID, legacyID, hash, unitA); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, migrations[2].SQL)
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatal("accepted incompatible legacy username")
	}
	if _, err = pool.Exec(ctx, `UPDATE users SET username='legacy_user' WHERE id=$1`, legacyID); err != nil {
		t.Fatal(err)
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, migrations[2].SQL); err != nil {
		t.Fatal(err)
	}
	if tx.Commit(ctx) != nil {
		t.Fatal("commit")
	}
	var count int
	var actualHash string
	var legacyUnit uuid.UUID
	var flag bool
	var until time.Time
	if err = pool.QueryRow(ctx, `SELECT unit_id,password_hash,must_change_password,access_valid_until FROM users WHERE id=$1`, legacyID).Scan(&legacyUnit, &actualHash, &flag, &until); err != nil {
		t.Fatal(err)
	}
	if legacyUnit != unitA || actualHash != hash || flag || until.Format("2006-01-02") != "2027-01-31" {
		t.Fatal("legacy changed")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM user_units WHERE user_id=$1 AND unit_id=$2`, legacyID, unitA).Scan(&count); err != nil || count != 1 {
		t.Fatal("missing legacy link")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_units VALUES($1,$2)`, legacyID, unitA); err == nil {
		t.Fatal("duplicate association accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE users SET username='with.dot' WHERE id=$1`, legacyID); err == nil {
		t.Fatal("constraint accepted dot")
	}
	users := user.NewStore(pool)
	admin, err := users.ByID(ctx, adminID)
	if err != nil || admin.MustChangePassword {
		t.Fatal("admin affected")
	}
	store := useradmin.NewStore(pool)
	service := useradmin.New(store, hasher)
	now := time.Now()
	makeUser := func(role user.Role, name string, ids []uuid.UUID) (useradmin.Result, error) {
		return service.Apply(ctx, admin, useradmin.Command{Kind: "create", Role: role, Name: "Pessoa Sintética", Username: name, UnitIDs: ids, Period: "3m"}, initial, now)
	}
	medico, err := makeUser(user.RoleMedico, "synthetic_medico", []uuid.UUID{unitA, unitB})
	if err != nil {
		t.Fatal(err)
	}
	gestor, err := makeUser(user.RoleGestor, "synthetic_gestor", []uuid.UUID{unitA, unitB})
	if err != nil {
		t.Fatal(err)
	}
	if !medico.User.MustChangePassword || !gestor.User.MustChangePassword || len(medico.User.Units) != 2 {
		t.Fatal("new users state")
	}
	if _, err = makeUser(user.RoleMedico, "inactive_link", []uuid.UUID{inactive}); !errors.Is(err, useradmin.ErrUnits) {
		t.Fatal("inactive accepted")
	}
	// Failure rolled the user insert back as well.
	if _, err = users.ByUsername(ctx, "inactive_link"); !errors.Is(err, user.ErrNotFound) {
		t.Fatal("partial create")
	}
	if _, err = makeUser(user.RoleMedico, "synthetic_medico", []uuid.UUID{unitA}); !errors.Is(err, useradmin.ErrConflict) {
		t.Fatal("duplicate username")
	}
	if _, err = pool.Exec(ctx, `UPDATE units SET active=false WHERE id=$1`, unitA); err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(ctx, admin, useradmin.Command{Kind: "edit", Target: medico.User.ID, Name: "Atualizado Sintético", UnitIDs: []uuid.UUID{unitA, unitB}}, "", now)
	if err != nil || len(result.User.Units) != 2 {
		t.Fatal("historical inactive link not retained", err)
	}
	sessions := auth.NewSessionStore(pool)
	u, err := users.ByID(ctx, medico.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	_ = token
	if _, err = sessions.CreateVerified(ctx, u, tokenHash, now.Add(time.Hour), auth.SessionOrigin{}, now); err != nil {
		t.Fatal(err)
	}
	if err = service.ChangePassword(ctx, u, tokenHash, initial, "synthetic-personal-password", time.Hour, now); err != nil {
		t.Fatal(err)
	}
	changed, err := users.ByID(ctx, u.ID)
	if err != nil || changed.MustChangePassword {
		t.Fatal("change flag")
	}
	if _, _, err = sessions.Resolve(ctx, tokenHash, time.Hour, now); !errors.Is(err, auth.ErrSessaoInvalida) {
		t.Fatal("change session alive")
	}
	if _, err = sessions.CreateVerified(ctx, u, tokenHash, now.Add(time.Hour), auth.SessionOrigin{}, now); err == nil {
		t.Fatal("stale password session creation")
	}
	_, secondHash, _ := auth.NewSessionToken()
	if _, err = sessions.CreateVerified(ctx, changed, secondHash, now.Add(time.Hour), auth.SessionOrigin{}, now); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Apply(ctx, admin, useradmin.Command{Kind: "reset", Target: u.ID}, "synthetic-reset-password", now); err != nil {
		t.Fatal(err)
	}
	if _, _, err = sessions.Resolve(ctx, secondHash, time.Hour, now); err == nil {
		t.Fatal("reset session alive")
	}
	if swapped, err := users.CompareAndSwapPassword(ctx, u.ID, changed.PasswordHash, hash); err != nil || swapped {
		t.Fatal("rehash overwrote reset")
	}
	updated, err := users.ByID(ctx, u.ID)
	if err != nil || !updated.MustChangePassword {
		t.Fatal("reset flag")
	}
	_, thirdHash, _ := auth.NewSessionToken()
	if _, err = sessions.CreateVerified(ctx, updated, thirdHash, now.Add(time.Hour), auth.SessionOrigin{}, now); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Apply(ctx, admin, useradmin.Command{Kind: "active", Target: u.ID, Active: false}, "", now); err != nil {
		t.Fatal(err)
	}
	if _, _, err = sessions.Resolve(ctx, thirdHash, time.Hour, now); err == nil {
		t.Fatal("deactivated session alive")
	}
	if _, err = service.Apply(ctx, admin, useradmin.Command{Kind: "active", Target: u.ID, Active: true}, "", now); err != nil {
		t.Fatal(err)
	}
	for _, period := range []string{"1m", "3m", "6m", "1y", "unlimited"} {
		if _, err = service.Apply(ctx, admin, useradmin.Command{Kind: "renew", Target: u.ID, Period: period}, "", now); err != nil {
			t.Fatal(err)
		}
	}
	// Real repository also enforces target roles independently of HTTP/service.
	g, err := users.ByID(ctx, gestor.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE users SET must_change_password=false WHERE id=$1`, g.ID); err != nil {
		t.Fatal(err)
	}
	g.MustChangePassword = false
	for _, target := range []uuid.UUID{admin.ID, g.ID} {
		if _, err = store.Apply(ctx, g, useradmin.Command{Kind: "active", Target: target, Active: false}, now); !errors.Is(err, useradmin.ErrForbidden) {
			t.Fatal("IDOR", err)
		}
	}
	if _, err = store.Apply(ctx, admin, useradmin.Command{Kind: "active", Target: admin.ID, Active: false}, now); !errors.Is(err, useradmin.ErrForbidden) {
		t.Fatal("admin unprotected")
	}
	list, err := service.List(ctx, g, useradmin.Query{Limit: 25}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range list {
		if v.Role != user.RoleMedico {
			t.Fatal("gestor listing escalation")
		}
	}
	for _, q := range []useradmin.Query{{Limit: 1}, {Limit: 25, Search: "Atualizado"}, {Limit: 25, Role: user.RoleGestor}, {Limit: 25, UnitID: &unitA}, {Limit: 25, Status: "expired"}, {Limit: 25, Status: "active"}, {Limit: 25, Status: "inactive"}} {
		if _, err = service.List(ctx, admin, q, now); err != nil {
			t.Fatal("filter query", err)
		}
	}
	// Database uniqueness under competing creates; exactly one transaction wins.
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := makeUser(user.RoleMedico, "concurrent_user", []uuid.UUID{unitB})
			out <- e
		}()
	}
	wg.Wait()
	close(out)
	success, conflict := 0, 0
	for e := range out {
		if e == nil {
			success++
		} else if errors.Is(e, useradmin.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("uniqueness race")
	}
}
