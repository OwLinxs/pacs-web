// Package user modela os usuários da aplicação e o acesso a eles no PostgreSQL.
package user

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Erros de domínio. Os handlers HTTP os traduzem para respostas sanitizadas.
var (
	// ErrNotFound indica usuário inexistente.
	ErrNotFound = errors.New("usuário não encontrado")
	// ErrUsernameTaken indica colisão de username.
	ErrUsernameTaken = errors.New("username já cadastrado")
)

// padraoUsername espelha a restrição users_username_format da migration.
var padraoUsername = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,63}$`)

// User é um usuário da aplicação.
//
// PasswordHash nunca é serializado: a marcação json:"-" é uma trava extra, já
// que as respostas da API usam DTOs próprios.
type User struct {
	ID       uuid.UUID
	Name     string
	Username string
	Email    string

	PasswordHash string `json:"-"`

	Role   Role
	UnitID *uuid.UUID
	// UnitName vem de um join e fica vazio quando o usuário não tem unidade.
	UnitName string

	MustChangePassword bool
	Units              []UnitRef
	Active             bool
	// AccessValidUntil é o último dia de acesso permitido, inclusive.
	// nil = sem prazo.
	AccessValidUntil *time.Time
	LastLoginAt      *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MotivoBloqueio descreve por que um usuário não pode autenticar.
type MotivoBloqueio string

const (
	// BloqueioInativo indica conta desativada.
	BloqueioInativo MotivoBloqueio = "INACTIVE"
	// BloqueioExpirado indica validade de acesso vencida.
	BloqueioExpirado MotivoBloqueio = "EXPIRED"
)

// CanAuthenticate diz se o usuário pode iniciar sessão no instante informado.
// O segundo retorno só tem valor quando o acesso é negado.
func (u User) CanAuthenticate(agora time.Time) (bool, MotivoBloqueio) {
	if !u.Active {
		return false, BloqueioInativo
	}
	if AccessExpired(u.AccessValidUntil, agora) {
		return false, BloqueioExpirado
	}

	return true, ""
}

// NewUser são os dados necessários para criar um usuário.
type NewUser struct {
	Name             string
	Username         string
	Email            string
	PasswordHash     string
	Role             Role
	UnitID           *uuid.UUID
	AccessValidUntil *time.Time
}

// NormalizeUsername aplica a forma canônica: sem espaços nas pontas e em
// minúsculas. A unicidade no banco é sobre esta forma.
func NormalizeUsername(bruto string) string {
	return strings.ToLower(strings.TrimSpace(bruto))
}

// Validate confere os campos e normaliza username e e-mail no lugar.
func (n *NewUser) Validate() error {
	n.Name = strings.TrimSpace(n.Name)
	n.Username = NormalizeUsername(n.Username)
	n.Email = strings.TrimSpace(n.Email)

	if n.Name == "" {
		return errors.New("nome é obrigatório")
	}
	if len([]rune(n.Name)) > 120 {
		return errors.New("nome deve ter no máximo 120 caracteres")
	}
	if !padraoUsername.MatchString(n.Username) {
		return errors.New("username deve ter de 3 a 64 caracteres, em minúsculas, usando apenas letras, números, hífen ou sublinhado")
	}
	if n.Email != "" {
		if _, err := mail.ParseAddress(n.Email); err != nil {
			return errors.New("e-mail inválido")
		}
		if len(n.Email) > 254 {
			return errors.New("e-mail deve ter no máximo 254 caracteres")
		}
	}
	if _, err := ParseRole(string(n.Role)); err != nil {
		return err
	}
	if n.PasswordHash == "" {
		return errors.New("hash de senha ausente")
	}
	if n.AccessValidUntil != nil && n.AccessValidUntil.IsZero() {
		return fmt.Errorf("validade de acesso inválida")
	}
	return nil
}

// UnitRef expõe somente metadados administrativos.
type UnitRef struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Active bool      `json:"active"`
}
