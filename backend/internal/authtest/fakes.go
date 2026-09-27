// Package authtest traz implementações em memória das dependências de
// autenticação, usadas pelos testes de auth e da API HTTP.
//
// Só existe para teste; nenhum binário de produção importa este pacote.
package authtest

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

// Users é um repositório de usuários em memória.
type Users struct {
	mu          sync.Mutex
	porUsername map[string]user.User
	porID       map[uuid.UUID]user.User
	// ErroBusca, quando definido, é devolvido por ByUsername.
	ErroBusca error
}

// NewUsers devolve um repositório vazio.
func NewUsers() *Users {
	return &Users{
		porUsername: map[string]user.User{},
		porID:       map[uuid.UUID]user.User{},
	}
}

// Add insere ou substitui um usuário.
func (u *Users) Add(usuario user.User) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if usuario.ID == uuid.Nil {
		usuario.ID = uuid.New()
	}
	usuario.Username = user.NormalizeUsername(usuario.Username)
	u.porUsername[usuario.Username] = usuario
	u.porID[usuario.ID] = usuario
}

// ByUsername implementa auth.UserReader.
func (u *Users) ByUsername(_ context.Context, username string) (user.User, error) {
	if u.ErroBusca != nil {
		return user.User{}, u.ErroBusca
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	usuario, ok := u.porUsername[user.NormalizeUsername(username)]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return usuario, nil
}

// ByID implementa auth.UserReader.
func (u *Users) ByID(_ context.Context, id uuid.UUID) (user.User, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	usuario, ok := u.porID[id]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return usuario, nil
}

// TouchLastLogin implementa auth.UserReader.
func (u *Users) TouchLastLogin(_ context.Context, id uuid.UUID, quando time.Time) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	usuario, ok := u.porID[id]
	if !ok {
		return user.ErrNotFound
	}
	usuario.LastLoginAt = &quando
	u.porID[id] = usuario
	u.porUsername[usuario.Username] = usuario
	return nil
}

// UpdatePasswordHash implementa auth.UserReader.
func (u *Users) UpdatePasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	usuario, ok := u.porID[id]
	if !ok {
		return user.ErrNotFound
	}
	usuario.PasswordHash = hash
	u.porID[id] = usuario
	u.porUsername[usuario.Username] = usuario
	return nil
}

// Sessions é um armazenamento de sessões em memória.
type Sessions struct {
	mu       sync.Mutex
	porToken map[string]auth.Session
	usuarios *Users
	// Agora permite fixar o tempo; o padrão é time.Now.
	Agora func() time.Time
}

// NewSessions devolve um armazenamento ligado ao repositório de usuários.
func NewSessions(usuarios *Users) *Sessions {
	return &Sessions{porToken: map[string]auth.Session{}, usuarios: usuarios}
}

func (s *Sessions) agora() time.Time {
	if s.Agora != nil {
		return s.Agora()
	}
	return time.Now()
}

// Create implementa auth.SessionKeeper.
func (s *Sessions) Create(_ context.Context, userID uuid.UUID, tokenHash []byte, expiraEm time.Time, _ auth.SessionOrigin) (auth.Session, error) {
	agora := s.agora()
	sessao := auth.Session{
		ID:                uuid.New(),
		UserID:            userID,
		CreatedAt:         agora,
		LastSeenAt:        agora,
		AbsoluteExpiresAt: expiraEm,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.porToken[hex.EncodeToString(tokenHash)] = sessao
	return sessao, nil
}

// Resolve implementa auth.SessionKeeper, aplicando as mesmas regras do
// armazenamento real: revogação, prazo absoluto, inatividade e situação da conta.
func (s *Sessions) Resolve(ctx context.Context, tokenHash []byte, idleTTL time.Duration, agora time.Time) (auth.Session, user.User, error) {
	chave := hex.EncodeToString(tokenHash)

	s.mu.Lock()
	sessao, ok := s.porToken[chave]
	s.mu.Unlock()
	if !ok {
		return auth.Session{}, user.User{}, auth.ErrSessaoInvalida
	}
	if !auth.SessionAlive(sessao, idleTTL, agora) {
		return auth.Session{}, user.User{}, auth.ErrSessaoInvalida
	}

	usuario, err := s.usuarios.ByID(ctx, sessao.UserID)
	if err != nil {
		return auth.Session{}, user.User{}, auth.ErrSessaoInvalida
	}
	if podeAutenticar, _ := usuario.CanAuthenticate(agora); !podeAutenticar {
		return auth.Session{}, user.User{}, auth.ErrSessaoInvalida
	}

	sessao.LastSeenAt = agora
	s.mu.Lock()
	s.porToken[chave] = sessao
	s.mu.Unlock()
	return sessao, usuario, nil
}

// Revoke implementa auth.SessionKeeper.
func (s *Sessions) Revoke(_ context.Context, tokenHash []byte, agora time.Time) error {
	chave := hex.EncodeToString(tokenHash)
	s.mu.Lock()
	defer s.mu.Unlock()
	sessao, ok := s.porToken[chave]
	if !ok {
		return nil
	}
	if sessao.RevokedAt == nil {
		sessao.RevokedAt = &agora
		s.porToken[chave] = sessao
	}
	return nil
}

// Quantidade devolve quantas sessões existem no armazenamento.
func (s *Sessions) Quantidade() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.porToken)
}

// Auditoria coleta os eventos registrados.
type Auditoria struct {
	mu       sync.Mutex
	entradas []audit.Entry
}

// NewAuditoria devolve um coletor vazio.
func NewAuditoria() *Auditoria { return &Auditoria{} }

// Record implementa auth.AuditRecorder.
func (a *Auditoria) Record(_ context.Context, entrada audit.Entry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entradas = append(a.entradas, entrada)
	return nil
}

// Entradas devolve uma cópia dos eventos registrados.
func (a *Auditoria) Entradas() []audit.Entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	copia := make([]audit.Entry, len(a.entradas))
	copy(copia, a.entradas)
	return copia
}

// Eventos devolve apenas os tipos de evento, na ordem em que ocorreram.
func (a *Auditoria) Eventos() []audit.Event {
	entradas := a.Entradas()
	eventos := make([]audit.Event, 0, len(entradas))
	for _, entrada := range entradas {
		eventos = append(eventos, entrada.Event)
	}
	return eventos
}
