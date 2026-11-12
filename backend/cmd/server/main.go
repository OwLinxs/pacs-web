// Command server é o backend do PACS Web Municipal.
//
// Subcomandos:
//
//	serve                      sobe a API (padrão quando nenhum é informado)
//	migrate                    aplica as migrations pendentes e sai
//	admin create               cria o primeiro administrador
//	keygen                     sorteia uma PACS_MASTER_KEY nova
//
// Toda a configuração necessária para subir vem do ambiente. Nada de secret em
// código, em log ou em argumento de linha de comando.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgconn"
)

func main() {
	log := novoLogger()

	comando := "serve"
	argumentos := os.Args[1:]
	if len(argumentos) > 0 && !temPrefixoDeFlag(argumentos[0]) {
		comando = argumentos[0]
		argumentos = argumentos[1:]
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()

	var err error
	switch comando {
	case "serve":
		err = executarServe(ctx, log)
	case "migrate":
		err = executarMigrate(ctx, log)
	case "admin":
		err = executarAdmin(ctx, log, argumentos)
	case "keygen":
		err = executarKeygen()
	case "help", "-h", "--help":
		imprimirAjuda()
		return
	default:
		err = fmt.Errorf("comando desconhecido: %q (use help)", comando)
	}

	if err != nil {
		// Erros de driver/infraestrutura podem conter URLs ou valores sensíveis:
		// só sai uma categoria fixa e o SQLSTATE, nunca a mensagem.
		log.Error("encerrando com erro", "comando", comando, "tipo", categoriaErro(err))
		os.Exit(1)
	}
}

func temPrefixoDeFlag(argumento string) bool {
	return len(argumento) > 0 && argumento[0] == '-'
}

func novoLogger() *slog.Logger {
	nivel := slog.LevelInfo
	if err := nivel.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
		nivel = slog.LevelInfo
	}
	manipulador := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: nivel})
	return slog.New(manipulador)
}

func imprimirAjuda() {
	fmt.Fprint(os.Stdout, `PACS Web Municipal — backend

Uso:
  pacs-server [serve]            sobe a API HTTP
  pacs-server migrate            aplica as migrations pendentes
  pacs-server admin create ...   cria o primeiro administrador
  pacs-server keygen             sorteia uma PACS_MASTER_KEY nova

Variáveis de ambiente: veja .env.example.
`)
}

var errFaltaSubcomando = errors.New("informe um subcomando (ex.: admin create)")

// categoriaErro resume o erro sem texto do driver (que pode citar host, usuário
// ou trechos da DSN). SQLSTATE é um código fixo do PostgreSQL.
func categoriaErro(err error) string {
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &pg):
		switch pg.Code {
		case "28P01", "28000":
			return "autenticação recusada pelo banco (usuário/senha da DSN) [" + pg.Code + "]"
		case "42501":
			return "sem permissão (a conta não pode criar/alterar tabelas; não use APP_DATABASE_URL) [" + pg.Code + "]"
		case "3D000":
			return "banco inexistente na DSN [" + pg.Code + "]"
		}
		return "erro SQL [" + pg.Code + "]"
	case strings.Contains(err.Error(), "DATABASE_URL inválida"):
		return "DSN inválida (formato ou caracteres especiais sem codificação)"
	case errors.As(err, new(*pgconn.ConnectError)):
		return "falha de conexão (host/porta da DSN; dentro do Compose use postgres:5432)"
	case strings.Contains(err.Error(), "já aplicada com conteúdo diferente"):
		return "migration aplicada foi alterada"
	case strings.Contains(err.Error(), "DSN de migration ausente"):
		return "DSN de migration ausente"
	}
	return "outro"
}
