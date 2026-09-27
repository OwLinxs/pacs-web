package auth_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/authtest"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

const (
	senhaValida = "senha-de-teste-longa"
	senhaErrada = "senha-de-teste-outra"
)

func hasherRapido() *auth.Hasher {
	return auth.NewHasher(auth.Argon2Params{Memory: 8 * 1024, Time: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
}

func logSilencioso() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type ambiente struct {
	servico   *auth.Service
	usuarios  *authtest.Users
	sessoes   *authtest.Sessions
	auditoria *authtest.Auditoria
	agora     time.Time
}

func montarAmbiente(t *testing.T) *ambiente {
	t.Helper()

	amb := &ambiente{agora: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)}
	amb.usuarios = authtest.NewUsers()
	amb.sessoes = authtest.NewSessions(amb.usuarios)
	amb.sessoes.Agora = func() time.Time { return amb.agora }
	amb.auditoria = authtest.NewAuditoria()

	servico, err := auth.NewService(amb.usuarios, amb.sessoes, hasherRapido(), amb.auditoria, logSilencioso(), auth.Options{
		AbsoluteTTL: 12 * time.Hour,
		IdleTTL:     30 * time.Minute,
		Now:         func() time.Time { return amb.agora },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	amb.servico = servico
	return amb
}

// adicionarUsuario cria um usuário com a senha informada já hasheada.
func (a *ambiente) adicionarUsuario(t *testing.T, username string, role user.Role, ajustar func(*user.User)) user.User {
	t.Helper()

	hash, err := hasherRapido().Hash(senhaValida)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	usuario := user.User{
		ID:           uuid.New(),
		Name:         "Usuario Teste",
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Active:       true,
	}
	if ajustar != nil {
		ajustar(&usuario)
	}
	a.usuarios.Add(usuario)
	return usuario
}

func TestLoginSucesso(t *testing.T) {
	amb := montarAmbiente(t)
	criado := amb.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)

	resultado, err := amb.servico.Login(context.Background(), auth.LoginInput{
		Username: "ADMIN.TESTE  ",
		Password: senhaValida,
		Origin:   auth.SessionOrigin{IP: "198.51.100.10"},
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if resultado.Token == "" {
		t.Error("login deveria devolver token de sessão")
	}
	if resultado.User.ID != criado.ID {
		t.Errorf("usuário = %v, esperado %v", resultado.User.ID, criado.ID)
	}
	if !resultado.Session.AbsoluteExpiresAt.Equal(amb.agora.Add(12 * time.Hour)) {
		t.Errorf("prazo absoluto = %v, esperado %v", resultado.Session.AbsoluteExpiresAt, amb.agora.Add(12*time.Hour))
	}
	if resultado.User.LastLoginAt == nil {
		t.Error("último acesso deveria ter sido registrado")
	}

	eventos := amb.auditoria.Eventos()
	if len(eventos) != 1 || eventos[0] != audit.EventLoginSuccess {
		t.Errorf("eventos auditados = %v, esperado [LOGIN_SUCCESS]", eventos)
	}
}

func TestLoginFalhas(t *testing.T) {
	ontem := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)

	casos := []struct {
		nome         string
		username     string
		senha        string
		ajustar      func(*user.User)
		erroEsperado error
		detalhe      string
	}{
		{
			nome:         "usuário inexistente",
			username:     "ninguem.aqui",
			senha:        senhaValida,
			erroEsperado: auth.ErrCredenciaisInvalidas,
			detalhe:      "usuário inexistente",
		},
		{
			nome:         "senha incorreta",
			username:     "medico.teste",
			senha:        senhaErrada,
			erroEsperado: auth.ErrCredenciaisInvalidas,
			detalhe:      "senha incorreta",
		},
		{
			nome:         "conta desativada",
			username:     "medico.teste",
			senha:        senhaValida,
			ajustar:      func(u *user.User) { u.Active = false },
			erroEsperado: auth.ErrContaInativa,
			detalhe:      "conta desativada",
		},
		{
			nome:         "validade expirada",
			username:     "medico.teste",
			senha:        senhaValida,
			ajustar:      func(u *user.User) { u.AccessValidUntil = &ontem },
			erroEsperado: auth.ErrContaExpirada,
			detalhe:      "validade de acesso expirada",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			amb := montarAmbiente(t)
			amb.adicionarUsuario(t, "medico.teste", user.RoleMedico, caso.ajustar)

			_, err := amb.servico.Login(context.Background(), auth.LoginInput{
				Username: caso.username,
				Password: caso.senha,
			})
			if !errors.Is(err, caso.erroEsperado) {
				t.Fatalf("erro = %v, esperado %v", err, caso.erroEsperado)
			}
			if amb.sessoes.Quantidade() != 0 {
				t.Error("login falho não pode criar sessão")
			}

			entradas := amb.auditoria.Entradas()
			if len(entradas) != 1 {
				t.Fatalf("eventos auditados = %d, esperado 1", len(entradas))
			}
			if entradas[0].Event != audit.EventLoginFailure {
				t.Errorf("evento = %q, esperado LOGIN_FAILURE", entradas[0].Event)
			}
			if entradas[0].Detail != caso.detalhe {
				t.Errorf("detalhe = %q, esperado %q", entradas[0].Detail, caso.detalhe)
			}
			if entradas[0].ActorUsername == "" {
				t.Error("auditoria deve guardar o username tentado")
			}
		})
	}
}

func TestLoginNaoRevelaSeUsuarioExiste(t *testing.T) {
	amb := montarAmbiente(t)
	amb.adicionarUsuario(t, "medico.teste", user.RoleMedico, nil)

	_, errInexistente := amb.servico.Login(context.Background(), auth.LoginInput{
		Username: "ninguem.aqui", Password: senhaValida,
	})
	_, errSenhaErrada := amb.servico.Login(context.Background(), auth.LoginInput{
		Username: "medico.teste", Password: senhaErrada,
	})

	if errInexistente == nil || errSenhaErrada == nil {
		t.Fatal("ambas as tentativas deveriam falhar")
	}
	if errInexistente.Error() != errSenhaErrada.Error() {
		t.Errorf("erros diferentes permitem enumerar usuários: %q vs %q", errInexistente, errSenhaErrada)
	}
}

func TestLoginAtualizaHashDesatualizado(t *testing.T) {
	amb := montarAmbiente(t)

	// Hash gravado com parâmetros mais fracos que os do serviço.
	fraco := auth.NewHasher(auth.Argon2Params{Memory: 4 * 1024, Time: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	hashAntigo, err := fraco.Hash(senhaValida)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	amb.usuarios.Add(user.User{
		ID: uuid.New(), Name: "Medico Teste", Username: "medico.teste",
		PasswordHash: hashAntigo, Role: user.RoleMedico, Active: true,
	})

	if _, err := amb.servico.Login(context.Background(), auth.LoginInput{
		Username: "medico.teste", Password: senhaValida,
	}); err != nil {
		t.Fatalf("Login: %v", err)
	}

	atualizado, err := amb.usuarios.ByUsername(context.Background(), "medico.teste")
	if err != nil {
		t.Fatalf("ByUsername: %v", err)
	}
	if atualizado.PasswordHash == hashAntigo {
		t.Error("hash com parâmetros fracos deveria ter sido recalculado no login")
	}
	if ok, _, err := hasherRapido().Verify(atualizado.PasswordHash, senhaValida); err != nil || !ok {
		t.Error("o hash recalculado deve continuar validando a mesma senha")
	}
}

func TestResolveSessao(t *testing.T) {
	amb := montarAmbiente(t)
	criado := amb.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)

	resultado, err := amb.servico.Login(context.Background(), auth.LoginInput{
		Username: "admin.teste", Password: senhaValida,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	t.Run("token válido", func(t *testing.T) {
		_, usuario, err := amb.servico.Resolve(context.Background(), resultado.Token)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if usuario.ID != criado.ID {
			t.Errorf("usuário = %v, esperado %v", usuario.ID, criado.ID)
		}
	})

	t.Run("token vazio", func(t *testing.T) {
		if _, _, err := amb.servico.Resolve(context.Background(), ""); !errors.Is(err, auth.ErrSessaoInvalida) {
			t.Errorf("erro = %v, esperado ErrSessaoInvalida", err)
		}
	})

	t.Run("token desconhecido", func(t *testing.T) {
		if _, _, err := amb.servico.Resolve(context.Background(), "token-que-nunca-existiu"); !errors.Is(err, auth.ErrSessaoInvalida) {
			t.Errorf("erro = %v, esperado ErrSessaoInvalida", err)
		}
	})
}

func TestSessaoExpiraPorInatividadeEPorPrazo(t *testing.T) {
	casos := []struct {
		nome   string
		avanco time.Duration
		valida bool
	}{
		{"dentro da janela", 20 * time.Minute, true},
		{"inatividade estourada", 31 * time.Minute, false},
		{"prazo absoluto estourado", 13 * time.Hour, false},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			amb := montarAmbiente(t)
			amb.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)

			resultado, err := amb.servico.Login(context.Background(), auth.LoginInput{
				Username: "admin.teste", Password: senhaValida,
			})
			if err != nil {
				t.Fatalf("Login: %v", err)
			}

			amb.agora = amb.agora.Add(caso.avanco)

			_, _, err = amb.servico.Resolve(context.Background(), resultado.Token)
			if caso.valida && err != nil {
				t.Fatalf("sessão deveria continuar válida: %v", err)
			}
			if !caso.valida && !errors.Is(err, auth.ErrSessaoInvalida) {
				t.Fatalf("erro = %v, esperado ErrSessaoInvalida", err)
			}
		})
	}
}

func TestSessaoDeContaDesativadaPerdeAcesso(t *testing.T) {
	amb := montarAmbiente(t)
	criado := amb.adicionarUsuario(t, "medico.teste", user.RoleMedico, nil)

	resultado, err := amb.servico.Login(context.Background(), auth.LoginInput{
		Username: "medico.teste", Password: senhaValida,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	criado.Active = false
	amb.usuarios.Add(criado)

	if _, _, err := amb.servico.Resolve(context.Background(), resultado.Token); !errors.Is(err, auth.ErrSessaoInvalida) {
		t.Errorf("erro = %v, esperado ErrSessaoInvalida após desativar a conta", err)
	}
}

func TestLogoutInvalidaSessao(t *testing.T) {
	amb := montarAmbiente(t)
	amb.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)

	resultado, err := amb.servico.Login(context.Background(), auth.LoginInput{
		Username: "admin.teste", Password: senhaValida,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := amb.servico.Logout(context.Background(), resultado.Token, resultado.User); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if _, _, err := amb.servico.Resolve(context.Background(), resultado.Token); !errors.Is(err, auth.ErrSessaoInvalida) {
		t.Errorf("erro = %v, esperado ErrSessaoInvalida depois do logout", err)
	}

	eventos := amb.auditoria.Eventos()
	if len(eventos) != 2 || eventos[1] != audit.EventLogout {
		t.Errorf("eventos = %v, esperado terminar em LOGOUT", eventos)
	}

	// Segundo logout com o mesmo token não é erro.
	if err := amb.servico.Logout(context.Background(), resultado.Token, resultado.User); err != nil {
		t.Errorf("logout repetido deveria ser idempotente: %v", err)
	}
}

func TestNewServiceValidaTTL(t *testing.T) {
	usuarios := authtest.NewUsers()
	casos := []auth.Options{
		{AbsoluteTTL: 0, IdleTTL: time.Minute},
		{AbsoluteTTL: time.Hour, IdleTTL: 0},
		{AbsoluteTTL: -time.Hour, IdleTTL: -time.Minute},
	}
	for _, opcoes := range casos {
		if _, err := auth.NewService(usuarios, authtest.NewSessions(usuarios), hasherRapido(), nil, logSilencioso(), opcoes); err == nil {
			t.Errorf("TTL inválido deveria ser rejeitado: %+v", opcoes)
		}
	}
}
