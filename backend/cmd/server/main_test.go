package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pmfb-saude/pacs-web/backend/internal/database"
)

func TestCategoriaErroNuncaExpoeDSN(t *testing.T) {
	const segredo = "senha-ficticia-123"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, conexao := database.Open(ctx, database.DefaultConfig("postgres://usuario:"+segredo+"@127.0.0.1:1/banco?sslmode=disable&connect_timeout=1"))
	_, formato := database.Open(ctx, database.DefaultConfig("postgres://usuario:"+segredo+"%zz@postgres/banco"))
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("aplicar migration 0005_x: %w", &pgconn.PgError{Code: "42501", Message: segredo}), "sem permissão"},
		{fmt.Errorf("conectar: %w", &pgconn.PgError{Code: "28P01"}), "autenticação recusada"},
		{fmt.Errorf("x: %w", &pgconn.PgError{Code: "42P07"}), "erro SQL [42P07]"},
		{conexao, "falha de conexão"},
		{formato, "DSN inválida"},
		{fmt.Errorf("migration 0001_init já aplicada com conteúdo diferente: crie uma nova"), "alterada"},
		{fmt.Errorf("qualquer " + segredo), "outro"},
	} {
		got := categoriaErro(tc.err)
		if !strings.Contains(got, tc.want) || strings.Contains(got, segredo) {
			t.Fatalf("%v → %q (esperado %q)", tc.err, got, tc.want)
		}
	}
}
