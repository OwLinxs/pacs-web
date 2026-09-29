package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store lê e grava usuários no PostgreSQL. Todas as queries são parametrizadas.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore devolve um Store sobre o pool informado.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const camposSelecionados = `
    u.id, u.name, u.username, COALESCE(u.email, ''), u.password_hash, u.role,
    u.unit_id, COALESCE(un.name, ''), u.active, u.access_valid_until,
    u.last_login_at, u.created_at, u.updated_at, u.must_change_password, ` + UnitsJSON

// ByUsername busca por username já normalizado.
func (s *Store) ByUsername(ctx context.Context, username string) (User, error) {
	consulta := `SELECT` + camposSelecionados + `
		FROM users u LEFT JOIN units un ON un.id = u.unit_id
		WHERE u.username = $1`
	return s.buscarUm(ctx, consulta, NormalizeUsername(username))
}

// ByID busca por identificador.
func (s *Store) ByID(ctx context.Context, id uuid.UUID) (User, error) {
	consulta := `SELECT` + camposSelecionados + `
		FROM users u LEFT JOIN units un ON un.id = u.unit_id
		WHERE u.id = $1`
	return s.buscarUm(ctx, consulta, id)
}

func (s *Store) buscarUm(ctx context.Context, consulta string, args ...any) (User, error) {
	var u User
	var unitsJSON []byte
	err := s.pool.QueryRow(ctx, consulta, args...).Scan(
		&u.ID, &u.Name, &u.Username, &u.Email, &u.PasswordHash, &u.Role,
		&u.UnitID, &u.UnitName, &u.Active, &u.AccessValidUntil,
		&u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt, &u.MustChangePassword, &unitsJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("buscar usuário: %w", err)
	}
	if json.Unmarshal(unitsJSON, &u.Units) != nil {
		return User{}, fmt.Errorf("unidades inválidas")
	}
	return u, nil
}

// Create insere um usuário. Espera o hash já calculado e os campos validados.
func (s *Store) Create(ctx context.Context, novo NewUser) (User, error) {
	if err := novo.Validate(); err != nil {
		return User{}, err
	}

	var email *string
	if novo.Email != "" {
		email = &novo.Email
	}

	const inserir = `
		INSERT INTO users (name, username, email, password_hash, role, unit_id, access_valid_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	var id uuid.UUID
	var criadoEm, atualizadoEm time.Time
	err := s.pool.QueryRow(ctx, inserir,
		novo.Name, novo.Username, email, novo.PasswordHash, novo.Role, novo.UnitID, novo.AccessValidUntil,
	).Scan(&id, &criadoEm, &atualizadoEm)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return User{}, ErrUsernameTaken
	}
	if err != nil {
		return User{}, fmt.Errorf("criar usuário: %w", err)
	}

	return User{
		ID:               id,
		Name:             novo.Name,
		Username:         novo.Username,
		Email:            novo.Email,
		PasswordHash:     novo.PasswordHash,
		Role:             novo.Role,
		UnitID:           novo.UnitID,
		Active:           true,
		AccessValidUntil: novo.AccessValidUntil,
		CreatedAt:        criadoEm,
		UpdatedAt:        atualizadoEm,
	}, nil
}

// TouchLastLogin registra o último acesso bem-sucedido.
func (s *Store) TouchLastLogin(ctx context.Context, id uuid.UUID, quando time.Time) error {
	const atualizar = `UPDATE users SET last_login_at = $2, updated_at = now() WHERE id = $1`
	if _, err := s.pool.Exec(ctx, atualizar, id, quando); err != nil {
		return fmt.Errorf("registrar último acesso: %w", err)
	}
	return nil
}

// CountByRole conta usuários de um perfil. Usado pelo bootstrap do primeiro ADMIN.
func (s *Store) CountByRole(ctx context.Context, role Role) (int, error) {
	const contar = `SELECT count(*) FROM users WHERE role = $1`
	var total int
	if err := s.pool.QueryRow(ctx, contar, role).Scan(&total); err != nil {
		return 0, fmt.Errorf("contar usuários por perfil: %w", err)
	}
	return total, nil
}

// UnitIDBySlug resolve o identificador de uma unidade pelo slug.
func (s *Store) UnitIDBySlug(ctx context.Context, slug string) (uuid.UUID, error) {
	const consulta = `SELECT id FROM units WHERE slug = $1`
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, consulta, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("unidade %q não cadastrada", slug)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("buscar unidade: %w", err)
	}
	return id, nil
}

// UpdatePasswordHash troca o hash de senha do usuário. Usado na redefinição de
// acesso e na atualização automática de parâmetros do Argon2id.
func (s *Store) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	const atualizar = `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`
	etiqueta, err := s.pool.Exec(ctx, atualizar, id, hash)
	if err != nil {
		return fmt.Errorf("atualizar hash de senha: %w", err)
	}
	if etiqueta.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UnitsJSON requer alias u para users. A coluna legada não é fonte dos vínculos.
const UnitsJSON = `COALESCE((SELECT jsonb_agg(jsonb_build_object('id', un2.id, 'name', un2.name, 'active', un2.active) ORDER BY lower(un2.name), un2.id) FROM user_units uu JOIN units un2 ON un2.id=uu.unit_id WHERE uu.user_id=u.id), '[]'::jsonb)`

// CompareAndSwapPassword impede que rehash de login sobrescreva reset concorrente.
func (s *Store) CompareAndSwapPassword(ctx context.Context, id uuid.UUID, old, next string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET password_hash=$3,updated_at=now() WHERE id=$1 AND password_hash=$2`, id, old, next)
	return tag.RowsAffected() == 1, err
}
