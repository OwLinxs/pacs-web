package user

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeUsername(t *testing.T) {
	casos := map[string]string{
		"  Helena.Marques ": "helena.marques",
		"ADMIN.TESTE":       "admin.teste",
		"medico_teste":      "medico_teste",
		"":                  "",
	}
	for entrada, esperado := range casos {
		if got := NormalizeUsername(entrada); got != esperado {
			t.Errorf("NormalizeUsername(%q) = %q, esperado %q", entrada, got, esperado)
		}
	}
}

func TestNewUserValidate(t *testing.T) {
	base := func() NewUser {
		return NewUser{
			Name:         "Medico Teste",
			Username:     "medico_teste",
			Email:        "medico.teste@exemplo.invalid",
			PasswordHash: "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
			Role:         RoleMedico,
		}
	}

	casos := []struct {
		nome       string
		ajustar    func(*NewUser)
		esperaErro bool
	}{
		{"válido", func(*NewUser) {}, false},
		{"sem e-mail", func(n *NewUser) { n.Email = "" }, false},
		{"username com maiúsculas é normalizado", func(n *NewUser) { n.Username = "Medico_Teste" }, false},
		{"nome vazio", func(n *NewUser) { n.Name = "   " }, true},
		{"nome gigante", func(n *NewUser) { n.Name = strings.Repeat("a", 121) }, true},
		{"username curto", func(n *NewUser) { n.Username = "ab" }, true},
		{"username com ponto", func(n *NewUser) { n.Username = "medico.teste" }, true},
		{"username com espaço", func(n *NewUser) { n.Username = "medico teste" }, true},
		{"username com acento", func(n *NewUser) { n.Username = "médico.teste" }, true},
		{"username começando com ponto", func(n *NewUser) { n.Username = ".medico" }, true},
		{"e-mail inválido", func(n *NewUser) { n.Email = "nao-e-email" }, true},
		{"perfil inválido", func(n *NewUser) { n.Role = "ROOT" }, true},
		{"sem hash", func(n *NewUser) { n.PasswordHash = "" }, true},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			novo := base()
			caso.ajustar(&novo)
			err := novo.Validate()
			if caso.esperaErro && err == nil {
				t.Error("esperava erro de validação")
			}
			if !caso.esperaErro {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if novo.Username != strings.ToLower(strings.TrimSpace(novo.Username)) {
					t.Errorf("username deveria estar normalizado, veio %q", novo.Username)
				}
			}
		})
	}
}

func TestCanAuthenticate(t *testing.T) {
	agora := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	hoje := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	ontem := hoje.AddDate(0, 0, -1)
	amanha := hoje.AddDate(0, 0, 1)

	casos := []struct {
		nome     string
		usuario  User
		esperaOk bool
		motivo   MotivoBloqueio
	}{
		{"ativo sem prazo", User{Active: true}, true, ""},
		{"ativo com prazo futuro", User{Active: true, AccessValidUntil: &amanha}, true, ""},
		{"ativo no último dia do prazo", User{Active: true, AccessValidUntil: &hoje}, true, ""},
		{"inativo", User{Active: false}, false, BloqueioInativo},
		{"prazo vencido", User{Active: true, AccessValidUntil: &ontem}, false, BloqueioExpirado},
		{"inativo tem precedência", User{Active: false, AccessValidUntil: &ontem}, false, BloqueioInativo},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			ok, motivo := caso.usuario.CanAuthenticate(agora)
			if ok != caso.esperaOk {
				t.Errorf("CanAuthenticate = %v, esperado %v", ok, caso.esperaOk)
			}
			if motivo != caso.motivo {
				t.Errorf("motivo = %q, esperado %q", motivo, caso.motivo)
			}
		})
	}
}
