// Package units define as unidades administrativas, sem associação com usuários.
package units

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidName = errors.New("Informe um nome de 1 a 120 caracteres, sem caracteres de controle.")
	ErrEmptyPatch  = errors.New("Informe nome ou estado ativo para alterar a unidade.")
	ErrDuplicate   = errors.New("Já existe uma unidade com esse nome, inclusive entre as inativas.")
	ErrNotFound    = errors.New("Unidade não encontrada.")
	ErrUnavailable = errors.New("unidades indisponíveis")
)

const DefaultLimit = 50
const MaxLimit = 100
const MaxOffset = 10000

type Unit struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Patch struct {
	Name   *string `json:"name"`
	Active *bool   `json:"active"`
}

func NormalizeName(name string) (string, error) {
	// Rejeita controles antes de TrimSpace para não aceitar quebras de linha ocultas.
	if !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl) {
		return "", ErrInvalidName
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 120 {
		return "", ErrInvalidName
	}
	return name, nil
}

func (p Patch) Validate() error {
	if p.Name == nil && p.Active == nil {
		return ErrEmptyPatch
	}
	if p.Name != nil {
		_, err := NormalizeName(*p.Name)
		return err
	}
	return nil
}
