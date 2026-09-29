package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

// Erros de autenticação devolvidos ao chamador.
var (
	// ErrCredenciaisInvalidas cobre usuário inexistente e senha errada — o
	// mesmo erro para os dois casos, para não permitir enumeração de usuários.
	ErrCredenciaisInvalidas = errors.New("credenciais inválidas")
	// ErrContaInativa indica senha correta em conta desativada.
	ErrContaInativa = errors.New("conta desativada")
	// ErrContaExpirada indica senha correta com validade de acesso vencida.
	ErrContaExpirada = errors.New("validade de acesso expirada")
)

// UserReader é o que o serviço precisa ler e atualizar em usuários.
type UserReader interface {
	ByUsername(ctx context.Context, username string) (user.User, error)
	ByID(ctx context.Context, id uuid.UUID) (user.User, error)
	TouchLastLogin(ctx context.Context, id uuid.UUID, quando time.Time) error
	CompareAndSwapPassword(ctx context.Context, id uuid.UUID, old, next string) (bool, error)
}

// SessionKeeper é o que o serviço precisa do armazenamento de sessões.
type SessionKeeper interface {
	CreateVerified(context.Context, user.User, []byte, time.Time, SessionOrigin, time.Time) (Session, error)
	Resolve(ctx context.Context, tokenHash []byte, idleTTL time.Duration, agora time.Time) (Session, user.User, error)
	Revoke(ctx context.Context, tokenHash []byte, agora time.Time) error
}

// AuditRecorder grava a trilha de auditoria.
type AuditRecorder interface {
	Record(ctx context.Context, entrada audit.Entry) error
}

// Service autentica usuários e administra sessões.
type Service struct {
	users     UserReader
	sessions  SessionKeeper
	hasher    *Hasher
	auditoria AuditRecorder
	log       *slog.Logger

	absoluteTTL time.Duration
	idleTTL     time.Duration

	// now permite fixar o tempo nos testes.
	now func() time.Time

	// hashInexistente equaliza o custo de um login com username inexistente,
	// para que o tempo de resposta não revele se a conta existe.
	hashInexistente string
}

// Options configura o Service.
type Options struct {
	AbsoluteTTL time.Duration
	IdleTTL     time.Duration
	// Now é opcional; o padrão é time.Now.
	Now func() time.Time
}

// NewService monta o serviço de autenticação.
func NewService(users UserReader, sessions SessionKeeper, hasher *Hasher, auditoria AuditRecorder, log *slog.Logger, opts Options) (*Service, error) {
	if opts.AbsoluteTTL <= 0 || opts.IdleTTL <= 0 {
		return nil, errors.New("TTL de sessão deve ser positivo")
	}
	agora := opts.Now
	if agora == nil {
		agora = time.Now
	}

	// Hash descartável, calculado uma vez, só para gastar o mesmo tempo de CPU
	// quando o username não existe.
	token, _, err := NewSessionToken()
	if err != nil {
		return nil, err
	}
	hashInexistente, err := hasher.Hash(token)
	if err != nil {
		return nil, err
	}

	return &Service{
		users:           users,
		sessions:        sessions,
		hasher:          hasher,
		auditoria:       auditoria,
		log:             log,
		absoluteTTL:     opts.AbsoluteTTL,
		idleTTL:         opts.IdleTTL,
		now:             agora,
		hashInexistente: hashInexistente,
	}, nil
}

// LoginInput são os dados de uma tentativa de login.
type LoginInput struct {
	Username string
	Password string
	Origin   SessionOrigin
}

// LoginResult traz o token de sessão — que vai só para o cookie — e o usuário.
type LoginResult struct {
	Token   string
	Session Session
	User    user.User
}

// Login valida as credenciais, abre a sessão e audita o resultado.
//
// O motivo real só é revelado depois de a senha ser comprovada: conta
// desativada e validade vencida viram erro específico apenas nesse caso.
func (s *Service) Login(ctx context.Context, entrada LoginInput) (LoginResult, error) {
	agora := s.now()
	username := user.NormalizeUsername(entrada.Username)

	usuario, err := s.users.ByUsername(ctx, username)
	if errors.Is(err, user.ErrNotFound) {
		// Gasta o mesmo tempo de um hash real antes de responder.
		_, _, _ = s.hasher.Verify(s.hashInexistente, entrada.Password)
		s.auditar(ctx, audit.Entry{
			Event:         audit.EventLoginFailure,
			ActorUsername: username,
			Detail:        "usuário inexistente",
			Origin:        entrada.Origin.IP,
		})
		return LoginResult{}, ErrCredenciaisInvalidas
	}
	if err != nil {
		return LoginResult{}, err
	}

	senhaOk, precisaRehash, err := s.hasher.Verify(usuario.PasswordHash, entrada.Password)
	if err != nil {
		// Hash corrompido é problema de dados, não do usuário.
		s.log.ErrorContext(ctx, "hash de senha inválido no banco", "user_id", usuario.ID)
		return LoginResult{}, ErrCredenciaisInvalidas
	}
	if !senhaOk {
		s.auditar(ctx, audit.Entry{
			Event:         audit.EventLoginFailure,
			ActorUserID:   &usuario.ID,
			ActorUsername: usuario.Username,
			UnitID:        usuario.UnitID,
			Detail:        "senha incorreta",
			Origin:        entrada.Origin.IP,
		})
		return LoginResult{}, ErrCredenciaisInvalidas
	}

	if ok, motivo := usuario.CanAuthenticate(agora); !ok {
		detalhe := "conta desativada"
		erro := ErrContaInativa
		if motivo == user.BloqueioExpirado {
			detalhe = "validade de acesso expirada"
			erro = ErrContaExpirada
		}
		s.auditar(ctx, audit.Entry{
			Event:         audit.EventLoginFailure,
			ActorUserID:   &usuario.ID,
			ActorUsername: usuario.Username,
			UnitID:        usuario.UnitID,
			Detail:        detalhe,
			Origin:        entrada.Origin.IP,
		})
		return LoginResult{}, erro
	}

	if precisaRehash {
		if next, e := s.hasher.Hash(entrada.Password); e == nil {
			if changed, e := s.users.CompareAndSwapPassword(ctx, usuario.ID, usuario.PasswordHash, next); e == nil && changed {
				usuario.PasswordHash = next
			}
		}
	}

	token, tokenHash, err := NewSessionToken()
	if err != nil {
		return LoginResult{}, err
	}
	sessao, err := s.sessions.CreateVerified(ctx, usuario, tokenHash, agora.Add(s.absoluteTTL), entrada.Origin, agora)
	if err != nil {
		return LoginResult{}, err
	}

	if err := s.users.TouchLastLogin(ctx, usuario.ID, agora); err != nil {
		s.log.WarnContext(ctx, "não foi possível registrar último acesso", "user_id", usuario.ID)
	}

	s.auditar(ctx, audit.Entry{
		Event:         audit.EventLoginSuccess,
		ActorUserID:   &usuario.ID,
		ActorUsername: usuario.Username,
		UnitID:        usuario.UnitID,
		Detail:        "sessão iniciada",
		Origin:        entrada.Origin.IP,
	})

	usuario.LastLoginAt = &agora
	return LoginResult{Token: token, Session: sessao, User: usuario}, nil
}

// Resolve valida o token de sessão vindo do cookie.
func (s *Service) Resolve(ctx context.Context, token string) (Session, user.User, error) {
	if token == "" {
		return Session{}, user.User{}, ErrSessaoInvalida
	}
	return s.sessions.Resolve(ctx, HashSessionToken(token), s.idleTTL, s.now())
}

// Logout invalida a sessão no servidor e audita o encerramento. É idempotente:
// token desconhecido não é erro.
func (s *Service) Logout(ctx context.Context, token string, usuario user.User) error {
	if token == "" {
		return nil
	}
	if err := s.sessions.Revoke(ctx, HashSessionToken(token), s.now()); err != nil {
		return err
	}
	if usuario.ID != uuid.Nil {
		s.auditar(ctx, audit.Entry{
			Event:         audit.EventLogout,
			ActorUserID:   &usuario.ID,
			ActorUsername: usuario.Username,
			UnitID:        usuario.UnitID,
			Detail:        "encerrada pelo usuário",
			Origin:        "",
		})
	}
	return nil
}

// IdleTTL expõe a expiração por inatividade em uso.
func (s *Service) IdleTTL() time.Duration { return s.idleTTL }

// AbsoluteTTL expõe a expiração absoluta em uso.
func (s *Service) AbsoluteTTL() time.Duration { return s.absoluteTTL }

func (s *Service) auditar(ctx context.Context, entrada audit.Entry) {
	if s.auditoria == nil {
		return
	}
	// Auditoria não pode derrubar a operação; o Recorder já registra a falha.
	_ = s.auditoria.Record(ctx, entrada)
}
