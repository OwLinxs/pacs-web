package user

import "testing"

func TestParseRole(t *testing.T) {
	casos := []struct {
		entrada    string
		esperada   Role
		esperaErro bool
	}{
		{"ADMIN", RoleAdmin, false},
		{"GESTOR", RoleGestor, false},
		{"MEDICO", RoleMedico, false},
		{"admin", "", true},
		{"ROOT", "", true},
		{"", "", true},
	}
	for _, caso := range casos {
		t.Run(caso.entrada, func(t *testing.T) {
			role, err := ParseRole(caso.entrada)
			if caso.esperaErro {
				if err == nil {
					t.Errorf("esperava erro para %q", caso.entrada)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRole(%q): %v", caso.entrada, err)
			}
			if role != caso.esperada {
				t.Errorf("ParseRole(%q) = %q, esperado %q", caso.entrada, role, caso.esperada)
			}
		})
	}
}

func TestCanCreateRole(t *testing.T) {
	casos := []struct {
		nome      string
		autor     Role
		alvo      Role
		permitido bool
	}{
		{"admin cria admin", RoleAdmin, RoleAdmin, true},
		{"admin cria gestor", RoleAdmin, RoleGestor, true},
		{"admin cria medico", RoleAdmin, RoleMedico, true},
		{"gestor cria medico", RoleGestor, RoleMedico, true},
		{"gestor não cria gestor", RoleGestor, RoleGestor, false},
		{"gestor não cria admin", RoleGestor, RoleAdmin, false},
		{"medico não cria medico", RoleMedico, RoleMedico, false},
		{"medico não cria admin", RoleMedico, RoleAdmin, false},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := caso.autor.CanCreateRole(caso.alvo); got != caso.permitido {
				t.Errorf("%s.CanCreateRole(%s) = %v, esperado %v", caso.autor, caso.alvo, got, caso.permitido)
			}
		})
	}
}

func TestCanManageUser(t *testing.T) {
	casos := []struct {
		nome      string
		autor     Role
		alvo      Role
		permitido bool
	}{
		{"admin gerencia admin", RoleAdmin, RoleAdmin, true},
		{"admin gerencia gestor", RoleAdmin, RoleGestor, true},
		{"gestor gerencia medico", RoleGestor, RoleMedico, true},
		{"gestor não gerencia gestor", RoleGestor, RoleGestor, false},
		{"gestor não gerencia admin", RoleGestor, RoleAdmin, false},
		{"medico não gerencia ninguém", RoleMedico, RoleMedico, false},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := caso.autor.CanManageUser(caso.alvo); got != caso.permitido {
				t.Errorf("%s.CanManageUser(%s) = %v, esperado %v", caso.autor, caso.alvo, got, caso.permitido)
			}
		})
	}
}

func TestCanChangeCriticalSettings(t *testing.T) {
	if !RoleAdmin.CanChangeCriticalSettings() {
		t.Error("ADMIN deve poder alterar configuração crítica")
	}
	for _, role := range []Role{RoleGestor, RoleMedico} {
		if role.CanChangeCriticalSettings() {
			t.Errorf("%s não deve poder alterar configuração crítica", role)
		}
	}
}
