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
	"syscall"
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
		// Erros de driver/infraestrutura podem conter URLs ou valores sensíveis.
		log.Error("encerrando com erro", "comando", comando)
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
