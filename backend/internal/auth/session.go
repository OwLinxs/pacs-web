package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

// Erros de sessão.
var (
	// ErrSessaoInvalida cobre sessão inexistente, revogada ou expirada. O
	// motivo exato não é distinguido para quem chama a API.
	ErrSessaoInvalida = errors.New("sessão inválida")
)

// tokenBytes é o tamanho do identificador de sessão sorteado.
const tokenBytes = 32

// Session é uma sessão server-side.
//
// O token bruto nunca é guardado: o banco recebe apenas o SHA-256. Se o banco
// vazar, os hashes não servem como cookie.
type Session struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	CreatedAt         time.Time
	LastSeenAt        time.Time
	AbsoluteExpiresAt time.Time
	RevokedAt         *time.Time
}

// NewSessionToken sorteia um identificador de sessão e devolve o token para o
// cookie junto do hash que vai para o banco.
func NewSessionToken() (token string, hash []byte, err error) {
	bruto := make([]byte, tokenBytes)
	if _, err := rand.Read(bruto); err != nil {
		return "", nil, fmt.Errorf("gerar token de sessão: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(bruto)
	return token, HashSessionToken(token), nil
}

// HashSessionToken devolve a representação guardada no banco.
func HashSessionToken(token string) []byte {
	soma := sha256.Sum256([]byte(token))
	return soma[:]
}

// SessionOrigin descreve de onde a sessão foi aberta. Serve à operação
// (revogar sessão de uma estação) e não guarda dado de paciente.
type SessionOrigin struct {
	IP        string
	UserAgent string
}

// SessionStore guarda sessões no PostgreSQL.
type SessionStore struct {
	pool *pgxpool.Pool
}

// NewSessionStore devolve um SessionStore sobre o pool informado.
func NewSessionStore(pool *pgxpool.Pool) *SessionStore { return &SessionStore{pool: pool} }

// Create abre uma sessão para o usuário.
func (s *SessionStore) Create(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiraEm time.Time, origem SessionOrigin) (Session, error) {
	const inserir = `
		INSERT INTO sessions (token_hash, user_id, absolute_expires_at, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, last_seen_at`

	var sessao Session
	sessao.UserID = userID
	sessao.AbsoluteExpiresAt = expiraEm

	err := s.pool.QueryRow(ctx, inserir, tokenHash, userID, expiraEm, ipOuNil(origem.IP), textoOuNil(origem.UserAgent)).
		Scan(&sessao.ID, &sessao.CreatedAt, &sessao.LastSeenAt)
	if err != nil {
		return Session{}, fmt.Errorf("criar sessão: %w", err)
	}
	return sessao, nil
}

// Resolve valida o hash informado e, quando a sessão está viva, renova a marca
// de atividade. Devolve ErrSessaoInvalida para ausente, revogada ou expirada —
// por prazo absoluto ou por inatividade.
func (s *SessionStore) Resolve(ctx context.Context, tokenHash []byte, idleTTL time.Duration, agora time.Time) (Session, user.User, error) {
	const consulta = `
		SELECT s.id, s.user_id, s.created_at, s.last_seen_at, s.absolute_expires_at, s.revoked_at,
		       u.id, u.name, u.username, COALESCE(u.email, ''), u.password_hash, u.role,
		       u.unit_id, COALESCE(un.name, ''), u.active, u.access_valid_until,
		       u.last_login_at, u.created_at, u.updated_at, u.must_change_password, ` + user.UnitsJSON + `
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN units un ON un.id = u.unit_id
		WHERE s.token_hash = $1`

	var sessao Session
	var usuario user.User
	var unitsJSON []byte
	err := s.pool.QueryRow(ctx, consulta, tokenHash).Scan(
		&sessao.ID, &sessao.UserID, &sessao.CreatedAt, &sessao.LastSeenAt, &sessao.AbsoluteExpiresAt, &sessao.RevokedAt,
		&usuario.ID, &usuario.Name, &usuario.Username, &usuario.Email, &usuario.PasswordHash, &usuario.Role,
		&usuario.UnitID, &usuario.UnitName, &usuario.Active, &usuario.AccessValidUntil,
		&usuario.LastLoginAt, &usuario.CreatedAt, &usuario.UpdatedAt, &usuario.MustChangePassword, &unitsJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, user.User{}, ErrSessaoInvalida
	}
	if err != nil {
		return Session{}, user.User{}, fmt.Errorf("buscar sessão: %w", err)
	}

	if json.Unmarshal(unitsJSON, &usuario.Units) != nil {
		return Session{}, user.User{}, ErrSessaoInvalida
	}
	if !SessionAlive(sessao, idleTTL, agora) {
		return Session{}, user.User{}, ErrSessaoInvalida
	}
	// Uma conta desativada ou vencida perde acesso mesmo com sessão aberta.
	if ok, _ := usuario.CanAuthenticate(agora); !ok {
		return Session{}, user.User{}, ErrSessaoInvalida
	}

	if err := s.touch(ctx, sessao.ID, agora); err != nil {
		return Session{}, user.User{}, err
	}
	sessao.LastSeenAt = agora
	return sessao, usuario, nil
}

// SessionAlive aplica as três regras de validade: revogação, prazo absoluto e
// inatividade.
func SessionAlive(sessao Session, idleTTL time.Duration, agora time.Time) bool {
	if sessao.RevokedAt != nil {
		return false
	}
	if !agora.Before(sessao.AbsoluteExpiresAt) {
		return false
	}
	if idleTTL > 0 && !agora.Before(sessao.LastSeenAt.Add(idleTTL)) {
		return false
	}
	return true
}

func (s *SessionStore) touch(ctx context.Context, id uuid.UUID, agora time.Time) error {
	const atualizar = `UPDATE sessions SET last_seen_at = $2 WHERE id = $1`
	if _, err := s.pool.Exec(ctx, atualizar, id, agora); err != nil {
		return fmt.Errorf("renovar sessão: %w", err)
	}
	return nil
}

// Revoke invalida a sessão correspondente ao hash. É idempotente.
func (s *SessionStore) Revoke(ctx context.Context, tokenHash []byte, agora time.Time) error {
	const revogar = `UPDATE sessions SET revoked_at = $2 WHERE token_hash = $1 AND revoked_at IS NULL`
	if _, err := s.pool.Exec(ctx, revogar, tokenHash, agora); err != nil {
		return fmt.Errorf("revogar sessão: %w", err)
	}
	return nil
}

// RevokeAllForUser encerra todas as sessões de um usuário. Usado quando a conta
// é desativada ou a senha é redefinida.
func (s *SessionStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID, agora time.Time) error {
	const revogar = `UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`
	if _, err := s.pool.Exec(ctx, revogar, userID, agora); err != nil {
		return fmt.Errorf("revogar sessões do usuário: %w", err)
	}
	return nil
}

// DeleteExpired remove sessões que já não servem para nada, mantendo a tabela
// pequena. Chamado periodicamente pelo servidor.
func (s *SessionStore) DeleteExpired(ctx context.Context, agora time.Time, idleTTL time.Duration) (int64, error) {
	const apagar = `
		DELETE FROM sessions
		WHERE absolute_expires_at < $1
		   OR (revoked_at IS NOT NULL AND revoked_at < $1)
		   OR last_seen_at < $2`
	etiqueta, err := s.pool.Exec(ctx, apagar, agora, agora.Add(-idleTTL))
	if err != nil {
		return 0, fmt.Errorf("limpar sessões: %w", err)
	}
	return etiqueta.RowsAffected(), nil
}

func ipOuNil(bruto string) *netip.Addr {
	endereco, err := netip.ParseAddr(bruto)
	if err != nil {
		return nil
	}
	return &endereco
}

func textoOuNil(bruto string) *string {
	if bruto == "" {
		return nil
	}
	if len(bruto) > 400 {
		bruto = bruto[:400]
	}
	return &bruto
}

// CreateVerified serializa login com reset/desativação: nunca cria uma sessão
// com uma senha verificada antes de uma alteração concorrente da credencial.
func (s *SessionStore) CreateVerified(ctx context.Context, u user.User, hash []byte, expires time.Time, origin SessionOrigin, now time.Time) (Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback(ctx)
	var current user.User
	if err = tx.QueryRow(ctx, `SELECT password_hash,active,access_valid_until FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&current.PasswordHash, &current.Active, &current.AccessValidUntil); err != nil {
		return Session{}, ErrSessaoInvalida
	}
	ok, _ := current.CanAuthenticate(now)
	if !ok || current.PasswordHash != u.PasswordHash {
		return Session{}, ErrCredenciaisInvalidas
	}
	result := Session{UserID: u.ID, AbsoluteExpiresAt: expires}
	err = tx.QueryRow(ctx, `INSERT INTO sessions(token_hash,user_id,absolute_expires_at,ip_address,user_agent) VALUES($1,$2,$3,$4,$5) RETURNING id,created_at,last_seen_at`, hash, u.ID, expires, ipOuNil(origin.IP), textoOuNil(origin.UserAgent)).Scan(&result.ID, &result.CreatedAt, &result.LastSeenAt)
	if err != nil {
		return Session{}, err
	}
	return result, tx.Commit(ctx)
}
