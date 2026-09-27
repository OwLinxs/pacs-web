package user

import "fmt"

// Role é o perfil de acesso. As regras abaixo são a autorização real do
// sistema; esconder um botão no frontend não é mecanismo de autorização.
type Role string

const (
	// RoleAdmin administra a aplicação por completo.
	RoleAdmin Role = "ADMIN"
	// RoleGestor administra médicos dentro das funções autorizadas.
	RoleGestor Role = "GESTOR"
	// RoleMedico usa as funções clínicas autorizadas.
	RoleMedico Role = "MEDICO"
)

// ParseRole valida um perfil recebido de fora do processo.
func ParseRole(bruto string) (Role, error) {
	switch Role(bruto) {
	case RoleAdmin:
		return RoleAdmin, nil
	case RoleGestor:
		return RoleGestor, nil
	case RoleMedico:
		return RoleMedico, nil
	default:
		return "", fmt.Errorf("perfil inválido: deve ser ADMIN, GESTOR ou MEDICO")
	}
}

// CanCreateRole diz se quem tem o perfil r pode criar um usuário com o perfil
// alvo. GESTOR cria apenas MEDICO: nunca outro GESTOR, nunca ADMIN, e nunca
// promove alguém a esses perfis.
func (r Role) CanCreateRole(alvo Role) bool {
	switch r {
	case RoleAdmin:
		return alvo == RoleAdmin || alvo == RoleGestor || alvo == RoleMedico
	case RoleGestor:
		return alvo == RoleMedico
	default:
		return false
	}
}

// CanManageUser diz se quem tem o perfil r pode editar, desativar ou redefinir
// o acesso de um usuário com o perfil alvo.
func (r Role) CanManageUser(alvo Role) bool {
	switch r {
	case RoleAdmin:
		return true
	case RoleGestor:
		return alvo == RoleMedico
	default:
		return false
	}
}

// CanChangeCriticalSettings diz se o perfil pode alterar configuração crítica
// da aplicação — integração com o PACS, por exemplo. Só ADMIN.
func (r Role) CanChangeCriticalSettings() bool { return r == RoleAdmin }
