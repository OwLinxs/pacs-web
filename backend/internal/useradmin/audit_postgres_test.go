package useradmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/database"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
	"github.com/pmfb-saude/pacs-web/backend/internal/units"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
	"github.com/pmfb-saude/pacs-web/backend/internal/useradmin"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func auditSetup(t *testing.T) (context.Context, *pgxpool.Pool, user.User, *auth.Hasher) {
	t.Helper()
	ctx, pool := disposable(t)
	var logs bytes.Buffer
	if err := database.Migrate(ctx, pool, slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
		t.Fatal(err)
	}
	// Runner verifies checksums and does not reapply 0004.
	if err := database.Migrate(ctx, pool, slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
		t.Fatal(err)
	}
	h := auth.NewHasher(auth.Argon2Params{Memory: 1024, Time: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	hash, e := h.Hash("synthetic-initial-password")
	if e != nil {
		t.Fatal(e)
	}
	id := uuid.New()
	if _, e = pool.Exec(ctx, `INSERT INTO users(id,name,username,role,password_hash) VALUES($1,'Operador Sintético','synthetic_admin','ADMIN',$2)`, id, hash); e != nil {
		t.Fatal(e)
	}
	actor, e := user.NewStore(pool).ByID(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	return audit.WithActor(ctx, audit.Actor{ID: id, Username: actor.Username, Origin: "127.0.0.1"}), pool, actor, h
}
func TestAuditAdministrativeAtomicityPostgres(t *testing.T) {
	ctx, pool, actor, h := auditSetup(t)
	store := units.NewStore(pool)
	unit, e := store.Create(ctx, "Unidade Sintética A")
	if e != nil {
		t.Fatal(e)
	}
	unitB, e := store.Create(ctx, "Unidade Sintética B")
	if e != nil {
		t.Fatal(e)
	}
	svc := useradmin.New(useradmin.NewStore(pool), h)
	now := time.Now()
	made, e := svc.Apply(ctx, actor, useradmin.Command{Kind: "create", Role: user.RoleMedico, Name: "Pessoa Sintética", Username: "synthetic_medico", UnitIDs: []uuid.UUID{unit.ID}, Period: "3m"}, "synthetic-initial-password", now)
	if e != nil {
		t.Fatal(e)
	}
	u, e := user.NewStore(pool).ByID(ctx, made.User.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, tokenHash, e := auth.NewSessionToken()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.NewSessionStore(pool).CreateVerified(ctx, u, tokenHash, now.Add(time.Hour), auth.SessionOrigin{}, now); e != nil {
		t.Fatal(e)
	}
	settingsStore := settings.NewStore(pool, nil)
	cfg := settings.Orthanc{Name: "PACS Sintético", BaseURL: "http://fixture.invalid", TimeoutSeconds: 10, VerifyTLS: true}
	if _, e = settingsStore.SalvarOrthanc(ctx, cfg, nil, actor.ID); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event IN ('USER_CREATED','USER_UNITS_CHANGED','UNIT_CREATED','ORTHANC_SETTINGS_CHANGED')`).Scan(&count); e != nil || count != 5 {
		t.Fatal("successful writes missing events", count, e)
	}
	// Force audit INSERT failure only in this disposable database.
	if _, e = pool.Exec(ctx, `CREATE FUNCTION reject_synthetic_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic-sensitive-error'; END $$; CREATE TRIGGER reject_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_synthetic_audit()`); e != nil {
		t.Fatal(e)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		err := pool.QueryRow(ctx, `SELECT md5(concat((SELECT jsonb_agg(to_jsonb(u) ORDER BY id) FROM users u),(SELECT jsonb_agg(to_jsonb(u) ORDER BY id) FROM units u),(SELECT jsonb_agg(to_jsonb(u) ORDER BY user_id,unit_id) FROM user_units u),(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM sessions s),(SELECT jsonb_agg(to_jsonb(s) ORDER BY key) FROM app_settings s),(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_events a)))`).Scan(&value)
		if err != nil {
			t.Fatal("snapshot failed")
		}
		return value
	}
	before := snapshot()
	checks := map[string]func() error{
		"unit create": func() error { _, e := store.Create(ctx, "Unidade Rejeitada"); return e },
		"unit edit": func() error {
			n := "Nome Sintético Alterado"
			_, _, e := store.Update(ctx, unit.ID, units.Patch{Name: &n})
			return e
		},
		"unit deactivate": func() error { a := false; _, _, e := store.Update(ctx, unit.ID, units.Patch{Active: &a}); return e },
		"user create": func() error {
			_, e := svc.Apply(ctx, actor, useradmin.Command{Kind: "create", Role: user.RoleGestor, Name: "Novo Sintético", Username: "synthetic_new", UnitIDs: []uuid.UUID{unit.ID}, Period: "1m"}, "synthetic-initial-password", now)
			return e
		},
		"user edit and links": func() error {
			_, e := svc.Apply(ctx, actor, useradmin.Command{Kind: "edit", Target: u.ID, Name: "Alterado Sintético", UnitIDs: []uuid.UUID{unitB.ID}}, "", now)
			return e
		},
		"user deactivate and revoke": func() error {
			_, e := svc.Apply(ctx, actor, useradmin.Command{Kind: "active", Target: u.ID, Active: false}, "", now)
			return e
		},
		"user renew": func() error {
			_, e := svc.Apply(ctx, actor, useradmin.Command{Kind: "renew", Target: u.ID, Period: "1y"}, "", now)
			return e
		},
		"password reset and revoke": func() error {
			_, e := svc.Apply(ctx, actor, useradmin.Command{Kind: "reset", Target: u.ID}, "synthetic-reset-password", now)
			return e
		},
		"password change and revoke": func() error {
			return svc.ChangePassword(ctx, u, tokenHash, "synthetic-initial-password", "synthetic-personal-password", time.Hour, now)
		},
		"settings": func() error {
			next := cfg
			next.TimeoutSeconds = 20
			_, e := settingsStore.SalvarOrthanc(ctx, next, nil, actor.ID)
			return e
		},
	}
	for name, fn := range checks {
		t.Run(name, func(t *testing.T) {
			e := fn()
			if e == nil {
				t.Fatal("committed without required audit")
			}
			if strings.Contains(e.Error(), "synthetic-sensitive") {
				t.Fatal("SQL detail leaked")
			}
			if snapshot() != before {
				t.Fatal("partial mutation committed")
			}
		})
	}
	if _, e = pool.Exec(ctx, `DROP TRIGGER reject_audit ON audit_events`); e != nil {
		t.Fatal(e)
	}
	// A later event failing rolls back preceding events in the same operation too.
	if _, e = pool.Exec(ctx, `CREATE FUNCTION reject_second_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event IN ('USER_UNITS_CHANGED','UNIT_DEACTIVATED') THEN RAISE EXCEPTION 'synthetic'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_second BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_second_audit()`); e != nil {
		t.Fatal(e)
	}
	if e = checks["user create"](); e == nil {
		t.Fatal("missing second-event failure")
	}
	active := false
	name := "Alterada Sintética"
	if _, _, e = store.Update(ctx, unit.ID, units.Patch{Name: &name, Active: &active}); e == nil {
		t.Fatal("missing second-event unit failure")
	}
	if snapshot() != before {
		t.Fatal("first audit event escaped rollback")
	}
	if _, e = pool.Exec(ctx, `DROP TRIGGER reject_second ON audit_events`); e != nil {
		t.Fatal(e)
	}

	// No-op edits produce no duplicate event.
	if _, _, e = store.Update(ctx, unit.ID, units.Patch{Name: &unit.Name}); e != nil {
		t.Fatal(e)
	}
	if snapshot() != before {
		t.Fatal("noop changed history")
	}
}

func TestAuditQueryAndSafeHistoryPostgres(t *testing.T) {
	ctx, pool, actor, _ := auditSetup(t)
	var logs bytes.Buffer
	recorder := audit.NewRecorder(pool, slog.New(slog.NewTextHandler(&logs, nil)))
	target := uuid.New()
	if _, e := pool.Exec(ctx, `INSERT INTO users(id,name,username,role,password_hash,active) VALUES($1,'Alvo Sintético','synthetic_target','MEDICO','synthetic-unusable',false)`, target); e != nil {
		t.Fatal(e)
	}
	for _, e := range []audit.Event{audit.EventUserCreated, audit.EventUserUpdated, audit.EventUserPasswordReset} {
		if err := recorder.Record(ctx, audit.Entry{Event: e, ActorUserID: &actor.ID, ActorUsername: actor.Username, Detail: "target_user_id=" + target.String(), Origin: "127.0.0.1"}); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE audit_events SET occurred_at=$2 WHERE event=$1`, e, time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	q := audit.Query{Limit: 2}
	p, e := recorder.List(ctx, q)
	if e != nil || len(p.Items) != 2 || !p.HasMore || *p.NextOffset != 2 || p.Items[0].Event != audit.EventUserPasswordReset {
		t.Fatal("pagination/order", e)
	}
	q.Offset = 2
	p, e = recorder.List(ctx, q)
	if e != nil || len(p.Items) != 1 || p.HasMore || p.Items[0].Event != audit.EventUserCreated {
		t.Fatal("next page", e)
	}
	q, e = audit.ParseQuery("dateFrom=2026-09-29&dateTo=2026-09-29&actorId=" + actor.ID.String() + "&targetUserId=" + target.String() + "&event=USER_CREATED&category=users")
	if e != nil {
		t.Fatal(e)
	}
	p, e = recorder.List(ctx, q)
	if e != nil || len(p.Items) != 1 || p.Items[0].Actor.Username != actor.Username || p.Items[0].Target.Username != "synthetic_target" {
		t.Fatal("AND filters/references", e)
	}
	q.Until = q.From
	q.From = nil
	p, e = recorder.List(ctx, q)
	if e != nil || len(p.Items) != 0 {
		t.Fatal("inclusive Sao Paulo day boundary", e)
	}
	// Historical free text is never returned, and events survive renamed/inactive/missing references.
	marker := "synthetic-sensitive-marker"
	if _, e = pool.Exec(ctx, `INSERT INTO audit_events(event,detail,actor_username,origin) VALUES('LOGIN_FAILURE',$1,$1,$1),('UNKNOWN_SYNTHETIC',$1,$1,$1),('USER_UPDATED',$1,$1,$1)`, marker); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `UPDATE users SET name='Operador Renomeado Sintético',active=false WHERE id=$1`, actor.ID); e != nil {
		t.Fatal(e)
	}
	p, e = recorder.List(ctx, audit.Query{Limit: 100})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(p)
	if bytes.Contains(raw, []byte(marker)) || bytes.Contains(raw, []byte("password_hash")) || bytes.Contains(raw, []byte("session_id")) {
		t.Fatal("sensitive legacy metadata exposed")
	}
	if len(p.Items) != 6 {
		t.Fatal("history lost")
	}
	// Only test data deleted here: no deletion capability is exposed by the application.
	if _, e = pool.Exec(ctx, `DELETE FROM users WHERE id=$1 OR id=$2`, actor.ID, target); e != nil {
		t.Fatal(e)
	}
	p, e = recorder.List(ctx, audit.Query{Limit: 100})
	if e != nil || len(p.Items) != 6 {
		t.Fatal("history missing after reference removal", e)
	}
	for _, v := range p.Items {
		if v.Event == audit.EventUserCreated && (v.Target == nil || v.Target.ID != target.String()) {
			t.Fatal("target historical ID lost")
		}
	}
	// Best-effort records do not leak PostgreSQL exception details into application logs.
	if _, e = pool.Exec(ctx, `CREATE FUNCTION reject_synthetic_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic-sensitive-marker'; END $$; CREATE TRIGGER reject_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_synthetic_audit()`); e != nil {
		t.Fatal(e)
	}
	if e = recorder.Record(ctx, audit.Entry{Event: audit.EventLoginFailure, Detail: "usuário inexistente"}); !errors.Is(e, audit.ErrUnavailable) {
		t.Fatal("failure missing")
	}
	if strings.Contains(logs.String(), marker) {
		t.Fatal("secret in log")
	}
}
