package useradmin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

func TestValidityCalendar(t *testing.T) {
	for _, tc := range []struct{ now, period, current, want string }{
		{"2028-01-31", "1m", "", "2028-02-29"}, {"2027-01-31", "1m", "", "2027-02-28"}, {"2028-02-29", "1y", "", "2029-02-28"},
		{"2026-11-30", "3m", "", "2027-02-28"}, {"2026-08-31", "6m", "", "2027-02-28"}, {"2026-09-29", "1m", "2026-12-31", "2027-01-31"},
		{"2026-09-29", "1m", "2026-01-01", "2026-10-29"}, {"2026-09-29", "unlimited", "2027-01-01", ""},
	} {
		t.Run(tc.now+tc.period+tc.current, func(t *testing.T) {
			now, _ := time.Parse("2006-01-02", tc.now)
			now = now.Add(12 * time.Hour)
			var current *time.Time
			if tc.current != "" {
				d, _ := time.Parse("2006-01-02", tc.current)
				current = &d
			}
			got, e := Validity(tc.period, current, now)
			if e != nil {
				t.Fatal(e)
			}
			if tc.want == "" {
				if got != nil {
					t.Fatal("expected unlimited")
				}
			} else if got == nil || got.Format("2006-01-02") != tc.want {
				t.Fatalf("unexpected calendar date: %v", got)
			}
		})
	}
	if _, e := Validity("365d", nil, time.Now()); !errors.Is(e, ErrInvalid) {
		t.Fatal("arbitrary duration accepted")
	}
	until, _ := time.Parse("2006-01-02", "2026-09-29")
	before, _ := time.Parse(time.RFC3339, "2026-09-30T02:59:59Z")
	after := before.Add(time.Second)
	if user.AccessExpired(&until, before) || !user.AccessExpired(&until, after) {
		t.Fatal("local inclusive boundary")
	}
}

func (r *spyRepo) TargetRole(context.Context, uuid.UUID) (user.Role, error) {
	return user.RoleMedico, nil
}

type spyRepo struct {
	command Command
	calls   int
}

func (r *spyRepo) List(context.Context, user.User, Query, time.Time) ([]Record, error) {
	return nil, nil
}
func (r *spyRepo) Apply(_ context.Context, _ user.User, c Command, _ time.Time) (Result, error) {
	r.command = c
	r.calls++
	return Result{}, nil
}
func (r *spyRepo) ChangePassword(context.Context, user.User, []byte, string, string, time.Duration, time.Time) error {
	return nil
}
func TestInputAndHash(t *testing.T) {
	repo := &spyRepo{}
	hasher := auth.NewHasher(auth.Argon2Params{Memory: 1024, Time: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	s := New(repo, hasher)
	c := Command{Kind: "create", Role: user.RoleMedico, Name: "Usuário Sintético", Username: "SYNTHETIC", UnitIDs: []uuid.UUID{uuid.New(), uuid.New()}, Period: "3m"}
	actor := user.User{Role: user.RoleAdmin}
	password := "synthetic-password-only"
	if _, e := s.Apply(context.Background(), actor, c, password, time.Now()); e != nil {
		t.Fatal(e)
	}
	if repo.command.Username != "synthetic" || !strings.HasPrefix(repo.command.PasswordHash, "$argon2id$") {
		t.Fatal("hash/normalization missing")
	}
	if ok, _, e := hasher.Verify(repo.command.PasswordHash, password); !ok || e != nil {
		t.Fatal("invalid Argon2id")
	}
	for _, name := range []string{"with.dot", "with space", "ácento", " abc", "ab", strings.Repeat("a", 65)} {
		bad := c
		bad.Username = name
		if _, e := s.Apply(context.Background(), actor, bad, password, time.Now()); e == nil {
			t.Fatal("invalid username accepted")
		}
	}
	for _, ids := range [][]uuid.UUID{nil, {uuid.Nil}, {c.UnitIDs[0], c.UnitIDs[0]}} {
		bad := c
		bad.UnitIDs = ids
		if _, e := s.Apply(context.Background(), actor, bad, password, time.Now()); !errors.Is(e, ErrUnits) {
			t.Fatal("invalid links")
		}
	}
	for _, role := range []user.Role{user.RoleAdmin, user.RoleGestor, user.RoleMedico} {
		for _, target := range []user.Role{user.RoleAdmin, user.RoleGestor, user.RoleMedico} {
			bad := c
			bad.Role = target
			_, e := s.Apply(context.Background(), user.User{Role: role}, bad, password, time.Now())
			if (e == nil) != CanManage(role, target) {
				t.Fatalf("authorization %s -> %s", role, target)
			}
		}
	}
	u := user.User{PasswordHash: repo.command.PasswordHash}
	if s.ChangePassword(context.Background(), u, nil, "wrong", password, time.Minute, time.Now()) == nil {
		t.Fatal("wrong password accepted")
	}
	if s.ChangePassword(context.Background(), u, nil, password, password, time.Minute, time.Now()) == nil {
		t.Fatal("same password accepted")
	}
}
