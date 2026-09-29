package units

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = `id, name, active, created_at, updated_at`

// List lê somente a página e uma sentinela. Ordem estável, inclusive para inativas.
func (s *Store) List(ctx context.Context, includeInactive bool, limit, offset int) ([]Unit, error) {
	if limit < 1 || limit > MaxLimit || offset < 0 || offset > MaxOffset {
		return nil, ErrUnavailable
	}
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM units WHERE ($1 OR active) ORDER BY lower(btrim(name)), id LIMIT $2 OFFSET $3`, includeInactive, limit+1, offset)
	if err != nil {
		return nil, storeError(err)
	}
	defer rows.Close()
	items := []Unit{}
	for rows.Next() {
		u, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		return nil, storeError(err)
	}
	return items, nil
}

func (s *Store) Create(ctx context.Context, name string) (Unit, error) {
	name, err := NormalizeName(name)
	if err != nil {
		return Unit{}, err
	}
	id := uuid.New()
	// slug/kind são colunas legadas obrigatórias. Não viram campos novos da API/UI.
	// Slug técnico estável, independente do nome, mantém contratos legados intactos.
	return scan(s.pool.QueryRow(ctx, `INSERT INTO units (id,slug,name,kind) VALUES ($1,$2,$3,'OUTRO') RETURNING `+columns, id, "unit-"+id.String(), name))
}

// Update bloqueia a linha para aplicar apenas campos enviados e informar à
// auditoria a transição efetivamente gravada, mesmo com edições concorrentes.
func (s *Store) Update(ctx context.Context, id uuid.UUID, patch Patch) (Unit, Unit, error) {
	if err := patch.Validate(); err != nil {
		return Unit{}, Unit{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Unit{}, Unit{}, storeError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	before, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM units WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return Unit{}, Unit{}, err
	}
	name, active := before.Name, before.Active
	if patch.Name != nil {
		name, err = NormalizeName(*patch.Name)
		if err != nil {
			return Unit{}, Unit{}, err
		}
	}
	if patch.Active != nil {
		active = *patch.Active
	}
	after := before
	if name != before.Name || active != before.Active {
		after, err = scan(tx.QueryRow(ctx, `UPDATE units SET name=$2,active=$3,updated_at=clock_timestamp() WHERE id=$1 RETURNING `+columns, id, name, active))
		if err != nil {
			return Unit{}, Unit{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Unit{}, Unit{}, storeError(err)
	}
	return before, after, nil
}

func scan(row pgx.Row) (Unit, error) {
	var u Unit
	err := row.Scan(&u.ID, &u.Name, &u.Active, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return Unit{}, storeError(err)
	}
	return u, nil
}

// Não propaga mensagens SQL que possam conter valores de entrada.
func storeError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "units_name_ci_unique" {
		return ErrDuplicate
	}
	return ErrUnavailable
}
