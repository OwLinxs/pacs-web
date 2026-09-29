package useradmin

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const fields = `u.id,u.name,u.username,COALESCE(u.email,''),u.role,u.active,u.access_valid_until,u.last_login_at,u.must_change_password,` + user.UnitsJSON

func scan(row pgx.Row, now time.Time) (Record, error) {
	var r Record
	var until *time.Time
	var raw []byte
	err := row.Scan(&r.ID, &r.Name, &r.Username, &r.Email, &r.Role, &r.Active, &until, &r.LastLoginAt, &r.MustChangePassword, &raw)
	if err != nil {
		return r, storeError(err)
	}
	if json.Unmarshal(raw, &r.Units) != nil {
		return r, ErrUnavailable
	}
	r.Status = "active"
	if !r.Active {
		r.Status = "inactive"
	} else if user.AccessExpired(until, now) {
		r.Status = "expired"
	}
	if until != nil {
		s := until.Format("2006-01-02")
		r.AccessValidUntil = &s
	}
	return r, nil
}
func storeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		return ErrConflict
	}
	return ErrUnavailable
}
func (s *Store) List(ctx context.Context, actor user.User, q Query, now time.Time) ([]Record, error) {
	// Literal substring search, not SQL wildcards; filters never appear in logs.
	rows, err := s.pool.Query(ctx, `SELECT `+fields+` FROM users u WHERE
 ($1='ADMIN' OR u.role='MEDICO') AND ($2='' OR u.role=$2)
 AND ($3='' OR strpos(lower(u.name),lower($3))>0 OR strpos(u.username,lower($3))>0)
 AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM user_units uu WHERE uu.user_id=u.id AND uu.unit_id=$4))
 AND ($5='' OR ($5='inactive' AND NOT u.active) OR ($5='expired' AND u.active AND u.access_valid_until<$6) OR ($5='active' AND u.active AND (u.access_valid_until IS NULL OR u.access_valid_until >=$6)))
 ORDER BY lower(u.name),u.id LIMIT $7 OFFSET $8`, actor.Role, q.Role, q.Search, q.UnitID, q.Status, user.AccessDate(now), q.Limit+1, q.Offset)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		r, e := scan(rows, now)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, storeError(rows.Err())
}
func (s *Store) Apply(ctx context.Context, actor user.User, c Command, now time.Time) (Result, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	defer tx.Rollback(ctx)
	// Re-check the actor inside the write transaction. The actor row serializes
	// administrative changes with deactivation/reset of this operator.
	var a user.User
	err = tx.QueryRow(ctx, `SELECT role,active,access_valid_until,must_change_password,password_hash FROM users WHERE id=$1 FOR UPDATE`, actor.ID).Scan(&a.Role, &a.Active, &a.AccessValidUntil, &a.MustChangePassword, &a.PasswordHash)
	if err != nil {
		return Result{}, ErrForbidden
	}
	ok, _ := a.CanAuthenticate(now)
	if !ok || a.MustChangePassword || a.Role != actor.Role || a.PasswordHash != actor.PasswordHash {
		return Result{}, ErrForbidden
	}
	events := []string{}
	id := c.Target
	if c.Kind == "create" {
		if !CanManage(a.Role, c.Role) {
			return Result{}, ErrForbidden
		}
		id = uuid.New()
		until, _ := Validity(c.Period, nil, now)
		_, err = tx.Exec(ctx, `INSERT INTO users(id,name,username,email,role,password_hash,access_valid_until,must_change_password) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,true)`, id, c.Name, c.Username, c.Email, c.Role, c.PasswordHash, until)
		if err != nil {
			return Result{}, storeError(err)
		}
		events = append(events, "USER_CREATED")
	} else {
		// IDs supplied by the caller never determine authorization: actual target role does.
		var role user.Role
		var active bool
		var until *time.Time
		var name, email string
		err = tx.QueryRow(ctx, `SELECT role,active,access_valid_until,name,COALESCE(email,'') FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&role, &active, &until, &name, &email)
		if err != nil {
			return Result{}, storeError(err)
		}
		if !CanManage(a.Role, role) {
			return Result{}, ErrForbidden
		}
		switch c.Kind {
		case "edit":
			if name != c.Name || email != c.Email {
				_, err = tx.Exec(ctx, `UPDATE users SET name=$2,email=NULLIF($3,''),updated_at=clock_timestamp() WHERE id=$1`, id, c.Name, c.Email)
				events = append(events, "USER_UPDATED")
			}
		case "active":
			if active != c.Active {
				_, err = tx.Exec(ctx, `UPDATE users SET active=$2,updated_at=clock_timestamp() WHERE id=$1`, id, c.Active)
				event := "USER_DEACTIVATED"
				if c.Active {
					event = "USER_ACTIVATED"
				}
				events = append(events, event)
			}
		case "renew":
			next, e := Validity(c.Period, until, now)
			if e != nil {
				return Result{}, e
			}
			_, err = tx.Exec(ctx, `UPDATE users SET access_valid_until=$2,updated_at=clock_timestamp() WHERE id=$1`, id, next)
			events = append(events, "USER_ACCESS_RENEWED")
		case "reset":
			_, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2,must_change_password=true,updated_at=clock_timestamp() WHERE id=$1`, id, c.PasswordHash)
			events = append(events, "USER_PASSWORD_RESET")
		default:
			return Result{}, ErrInvalid
		}
		if err != nil {
			return Result{}, storeError(err)
		}
	}
	if c.Kind == "create" || c.Kind == "edit" {
		changed, e := replaceUnits(ctx, tx, id, c.UnitIDs)
		if e != nil {
			return Result{}, e
		}
		if changed {
			events = append(events, "USER_UNITS_CHANGED")
			_, err = tx.Exec(ctx, `UPDATE users SET updated_at=clock_timestamp() WHERE id=$1`, id)
			if err != nil {
				return Result{}, ErrUnavailable
			}
		}
	}
	if c.Kind == "reset" || (c.Kind == "active" && !c.Active) {
		if err = revoke(ctx, tx, id, now); err != nil {
			return Result{}, err
		}
	}
	result, err := scan(tx.QueryRow(ctx, `SELECT `+fields+` FROM users u WHERE u.id=$1`, id), now)
	if err != nil {
		return Result{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Result{}, ErrUnavailable
	}
	return Result{User: result, Events: events}, nil
}
func replaceUnits(ctx context.Context, tx pgx.Tx, id uuid.UUID, ids []uuid.UUID) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT unit_id FROM user_units WHERE user_id=$1`, id)
	if err != nil {
		return false, ErrUnavailable
	}
	old := []uuid.UUID{}
	for rows.Next() {
		var u uuid.UUID
		if rows.Scan(&u) != nil {
			rows.Close()
			return false, ErrUnavailable
		}
		old = append(old, u)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, ErrUnavailable
	}
	// Stable lock order; FOR SHARE also conflicts with a concurrent unit deactivation.
	ordered := append([]uuid.UUID(nil), ids...)
	slices.SortFunc(ordered, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	for _, u := range ordered {
		var active bool
		if tx.QueryRow(ctx, `SELECT active FROM units WHERE id=$1 FOR SHARE`, u).Scan(&active) != nil {
			return false, ErrUnits
		}
		if !active && !slices.Contains(old, u) {
			return false, ErrUnits
		}
	}
	changed := len(old) != len(ids)
	for _, u := range old {
		if !slices.Contains(ids, u) {
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if _, err = tx.Exec(ctx, `DELETE FROM user_units WHERE user_id=$1 AND NOT(unit_id=ANY($2::uuid[]))`, id, ids); err != nil {
		return false, ErrUnavailable
	}
	for _, u := range ids {
		if _, err = tx.Exec(ctx, `INSERT INTO user_units(user_id,unit_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, u); err != nil {
			return false, ErrUnavailable
		}
	}
	return true, nil
}
func revoke(ctx context.Context, tx pgx.Tx, id uuid.UUID, now time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=$2 WHERE user_id=$1 AND revoked_at IS NULL`, id, now)
	return storeError(err)
}
func (s *Store) ChangePassword(ctx context.Context, u user.User, tokenHash []byte, expected, next string, idle time.Duration, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ErrUnavailable
	}
	defer tx.Rollback(ctx)
	var current user.User
	err = tx.QueryRow(ctx, `SELECT password_hash,active,access_valid_until FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&current.PasswordHash, &current.Active, &current.AccessValidUntil)
	if err != nil {
		return ErrForbidden
	}
	ok, _ := current.CanAuthenticate(now)
	if !ok || current.PasswordHash != expected {
		return ErrForbidden
	}
	var session auth.Session
	err = tx.QueryRow(ctx, `SELECT created_at,last_seen_at,absolute_expires_at,revoked_at FROM sessions WHERE user_id=$1 AND token_hash=$2 FOR UPDATE`, u.ID, tokenHash).Scan(&session.CreatedAt, &session.LastSeenAt, &session.AbsoluteExpiresAt, &session.RevokedAt)
	if err != nil || !auth.SessionAlive(session, idle, now) {
		return ErrForbidden
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2,must_change_password=false,updated_at=clock_timestamp() WHERE id=$1`, u.ID, next); err != nil {
		return ErrUnavailable
	}
	if err = revoke(ctx, tx, u.ID, now); err != nil {
		return err
	}
	return storeError(tx.Commit(ctx))
}

func (s *Store) TargetRole(ctx context.Context, id uuid.UUID) (user.Role, error) {
	var role user.Role
	err := s.pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, id).Scan(&role)
	return role, storeError(err)
}
