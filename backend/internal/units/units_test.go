package units

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestNormalizeName(t *testing.T) {
	for _, value := range []string{"", "   ", "\u00a0\u2003", "Unidade\nFictícia", strings.Repeat("a", 121), string([]byte{0xff})} {
		if _, err := NormalizeName(value); !errors.Is(err, ErrInvalidName) {
			t.Fatal("nome inválido aceito")
		}
	}
	name, err := NormalizeName("  Unidade Fictícia  ")
	if err != nil || name != "Unidade Fictícia" {
		t.Fatal("normalização incorreta")
	}
	if _, err := NormalizeName(strings.Repeat("á", 120)); err != nil {
		t.Fatal(err)
	}
}
func TestPatchValidationAndErrors(t *testing.T) {
	active := false
	name := "   "
	if (Patch{}).Validate() == nil || (Patch{Name: &name}).Validate() == nil {
		t.Fatal("patch inválido aceito")
	}
	if err := (Patch{Active: &active}).Validate(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(storeError(pgx.ErrNoRows), ErrNotFound) {
		t.Fatal("not found")
	}
	if !errors.Is(storeError(&pgconn.PgError{Code: "23505", ConstraintName: "units_name_ci_unique", Detail: "valor não deve sair"}), ErrDuplicate) {
		t.Fatal("duplicata")
	}
	if !errors.Is(storeError(&pgconn.PgError{Code: "23505", ConstraintName: "units_slug_key"}), ErrUnavailable) {
		t.Fatal("erro SQL indevido")
	}
	if strings.Contains(storeError(errors.New("secret-sintetico")).Error(), "secret") {
		t.Fatal("erro não sanitizado")
	}
}
