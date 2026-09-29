// Package useradmin implements administrative operations without granting role changes.
package useradmin

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

var (
	ErrInvalid     = errors.New("Dados de usuário inválidos.")
	ErrForbidden   = errors.New("Seu perfil não pode administrar este usuário.")
	ErrConflict    = errors.New("Nome de usuário já cadastrado.")
	ErrNotFound    = errors.New("Usuário não encontrado.")
	ErrUnits       = errors.New("Selecione unidades válidas; novas associações exigem unidades ativas.")
	ErrUnavailable = errors.New("Não foi possível concluir a operação. Tente novamente.")
	ErrPassword    = errors.New("Senha atual incorreta ou nova senha inválida. Use ao menos 12 caracteres e uma senha diferente.")
)

type Query struct {
	Search        string
	Role          user.Role
	UnitID        *uuid.UUID
	Status        string
	Limit, Offset int
}
type Record struct {
	ID                 uuid.UUID      `json:"id"`
	Name               string         `json:"name"`
	Username           string         `json:"username"`
	Email              string         `json:"email"`
	Role               user.Role      `json:"role"`
	Units              []user.UnitRef `json:"units"`
	Active             bool           `json:"active"`
	Status             string         `json:"status"`
	AccessValidUntil   *string        `json:"accessValidUntil"`
	LastLoginAt        *time.Time     `json:"lastLoginAt"`
	MustChangePassword bool           `json:"mustChangePassword"`
}
type Command struct {
	Kind                  string
	Target                uuid.UUID
	Role                  user.Role
	Name, Username, Email string
	UnitIDs               []uuid.UUID
	Period                string
	Active                bool
	PasswordHash          string
}
type Result struct {
	User   Record
	Events []string
}
type Repository interface {
	TargetRole(context.Context, uuid.UUID) (user.Role, error)
	List(context.Context, user.User, Query, time.Time) ([]Record, error)
	Apply(context.Context, user.User, Command, time.Time) (Result, error)
	ChangePassword(context.Context, user.User, []byte, string, string, time.Duration, time.Time) error
}
type Service struct {
	repo   Repository
	hasher *auth.Hasher
}

func New(repo Repository, hasher *auth.Hasher) *Service { return &Service{repo: repo, hasher: hasher} }
func CanManage(actor, target user.Role) bool {
	return (actor == user.RoleAdmin && (target == user.RoleMedico || target == user.RoleGestor)) || (actor == user.RoleGestor && target == user.RoleMedico)
}
func (s *Service) List(ctx context.Context, actor user.User, q Query, now time.Time) ([]Record, error) {
	if actor.Role != user.RoleAdmin && actor.Role != user.RoleGestor {
		return nil, ErrForbidden
	}
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 10000 || !safeText(q.Search, 120) {
		return nil, ErrInvalid
	}
	if q.Role != "" && q.Role != user.RoleAdmin && q.Role != user.RoleGestor && q.Role != user.RoleMedico {
		return nil, ErrInvalid
	}
	if actor.Role == user.RoleGestor && q.Role != "" && q.Role != user.RoleMedico {
		return nil, ErrForbidden
	}
	if q.Status != "" && q.Status != "active" && q.Status != "inactive" && q.Status != "expired" {
		return nil, ErrInvalid
	}
	return s.repo.List(ctx, actor, q, now)
}
func safeText(s string, max int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (s *Service) Apply(ctx context.Context, actor user.User, c Command, password string, now time.Time) (Result, error) {
	if actor.Role != user.RoleAdmin && actor.Role != user.RoleGestor {
		return Result{}, ErrForbidden
	}
	if c.Kind != "create" {
		role, err := s.repo.TargetRole(ctx, c.Target)
		if err != nil {
			return Result{}, err
		}
		if !CanManage(actor.Role, role) {
			return Result{}, ErrForbidden
		}
	}
	switch c.Kind {
	case "create":
		if !CanManage(actor.Role, c.Role) {
			return Result{}, ErrForbidden
		}
		// Reject spaces rather than silently changing the chosen identifier.
		if c.Username != strings.TrimSpace(c.Username) {
			return Result{}, ErrInvalid
		}
		c.Username = user.NormalizeUsername(c.Username)
		u := user.NewUser{Name: c.Name, Username: c.Username, Email: c.Email, Role: c.Role, PasswordHash: "validation-only"}
		if u.Validate() != nil {
			return Result{}, ErrInvalid
		}
		fallthrough
	case "edit":
		c.Name = strings.TrimSpace(c.Name)
		c.Email = strings.TrimSpace(c.Email)
		if c.Name == "" || !safeText(c.Name, 120) || !safeText(c.Email, 254) {
			return Result{}, ErrInvalid
		}
		if c.Email != "" {
			address, e := mail.ParseAddress(c.Email)
			if e != nil || address.Address != c.Email {
				return Result{}, ErrInvalid
			}
		}
		if len(c.UnitIDs) == 0 || len(c.UnitIDs) > 100 {
			return Result{}, ErrUnits
		}
		seen := map[uuid.UUID]bool{}
		for _, id := range c.UnitIDs {
			if id == uuid.Nil || seen[id] {
				return Result{}, ErrUnits
			}
			seen[id] = true
		}
	case "renew":
	case "active":
	case "reset":
	default:
		return Result{}, ErrInvalid
	}
	if c.Kind == "create" || c.Kind == "renew" {
		if _, err := Validity(c.Period, nil, now); err != nil {
			return Result{}, err
		}
	}
	if c.Kind == "create" || c.Kind == "reset" {
		if auth.ValidatePassword(password) != nil {
			return Result{}, ErrPassword
		}
		hash, err := s.hasher.Hash(password)
		if err != nil {
			return Result{}, ErrUnavailable
		}
		c.PasswordHash = hash
	}
	return s.repo.Apply(ctx, actor, c, now)
}
func (s *Service) ChangePassword(ctx context.Context, u user.User, tokenHash []byte, current, next string, idle time.Duration, now time.Time) error {
	if len(current) > 1024 || auth.ValidatePassword(next) != nil || current == next {
		return ErrPassword
	}
	ok, _, err := s.hasher.Verify(u.PasswordHash, current)
	if err != nil || !ok {
		return ErrPassword
	}
	hash, err := s.hasher.Hash(next)
	if err != nil {
		return ErrUnavailable
	}
	return s.repo.ChangePassword(ctx, u, tokenHash, u.PasswordHash, hash, idle, now)
}

// Validity clamps the day to the target month's last day. Existing future
// validity is the renewal base; expired/unlimited accounts start from today.
func Validity(period string, current *time.Time, now time.Time) (*time.Time, error) {
	months := 0
	switch period {
	case "1m":
		months = 1
	case "3m":
		months = 3
	case "6m":
		months = 6
	case "1y":
		months = 12
	case "unlimited":
		return nil, nil
	default:
		return nil, ErrInvalid
	}
	base := user.AccessDate(now)
	if current != nil && !user.AccessExpired(current, now) {
		y, m, d := current.Date()
		base = time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	first := time.Date(base.Year(), base.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	day := base.Day()
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	result := time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
	return &result, nil
}
