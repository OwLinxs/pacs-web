package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/term"

	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/config"
	"github.com/pmfb-saude/pacs-web/backend/internal/database"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

func executarAdmin(ctx context.Context, log *slog.Logger, argumentos []string) error {
	if len(argumentos) == 0 {
		return errFaltaSubcomando
	}
	switch argumentos[0] {
	case "create":
		return executarAdminCreate(ctx, log, argumentos[1:])
	default:
		return fmt.Errorf("subcomando admin desconhecido: %q", argumentos[0])
	}
}

// executarAdminCreate cria o primeiro administrador.
//
// A senha NUNCA vem por argumento de linha de comando: argumentos aparecem em
// `ps` e no histórico do shell. Ou o terminal pergunta (sem eco), ou a senha
// chega pela entrada padrão.
func executarAdminCreate(ctx context.Context, log *slog.Logger, argumentos []string) error {
	conjunto := flag.NewFlagSet("admin create", flag.ContinueOnError)
	nome := conjunto.String("name", "", "nome completo do administrador")
	username := conjunto.String("username", "", "username de acesso (minúsculas)")
	email := conjunto.String("email", "", "e-mail institucional (opcional)")
	unidade := conjunto.String("unit", "", "slug da unidade já cadastrada (opcional)")
	validade := conjunto.String("valid-until", "", "último dia de acesso, AAAA-MM-DD (opcional)")
	conjunto.Usage = func() {
		fmt.Fprint(os.Stderr, `Cria o primeiro administrador do sistema.

Uso:
  pacs-server admin create -name "Administrador Teste" -username admin_teste

A senha é solicitada no terminal, sem eco. Em automação, envie-a pela entrada
padrão:

  pacs-server admin create -name "..." -username ... < arquivo-com-a-senha

Nunca existe flag de senha: argumentos ficam visíveis em ps e no histórico.

Flags:
`)
		conjunto.PrintDefaults()
	}
	if err := conjunto.Parse(argumentos); err != nil {
		return err
	}

	if strings.TrimSpace(*nome) == "" || strings.TrimSpace(*username) == "" {
		conjunto.Usage()
		return errors.New("-name e -username são obrigatórios")
	}

	var validoAte *time.Time
	if *validade != "" {
		data, err := time.Parse("2006-01-02", *validade)
		if err != nil {
			return errors.New("-valid-until deve estar no formato AAAA-MM-DD")
		}
		validoAte = &data
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := database.Open(ctx, database.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool, log); err != nil {
		return err
	}

	usuarios := user.NewStore(pool)

	// O bootstrap existe uma vez. Havendo administrador, os demais usuários
	// passam pelos fluxos autorizados da aplicação.
	existentes, err := usuarios.CountByRole(ctx, user.RoleAdmin)
	if err != nil {
		return err
	}
	if existentes > 0 {
		return errors.New("já existe administrador cadastrado: crie novos usuários pelos fluxos da aplicação")
	}

	var unitID *uuid.UUID
	if *unidade != "" {
		id, err := usuarios.UnitIDBySlug(ctx, *unidade)
		if err != nil {
			return err
		}
		unitID = &id
	}

	senha, err := lerSenha()
	if err != nil {
		return err
	}
	if err := auth.ValidatePassword(senha); err != nil {
		return err
	}

	hash, err := auth.NewDefaultHasher().Hash(senha)
	if err != nil {
		return err
	}

	criado, err := usuarios.Create(ctx, user.NewUser{
		Name:             *nome,
		Username:         *username,
		Email:            *email,
		PasswordHash:     hash,
		Role:             user.RoleAdmin,
		UnitID:           unitID,
		AccessValidUntil: validoAte,
	})
	if errors.Is(err, user.ErrUsernameTaken) {
		return errors.New("username já cadastrado")
	}
	if err != nil {
		return err
	}

	// Só identificadores. Nada de senha ou hash, nem em log nem na saída.
	log.Info("administrador criado", "user_id", criado.ID, "username", criado.Username)
	fmt.Fprintf(os.Stdout, "Administrador criado: %s (%s)\n", criado.Name, criado.Username)
	return nil
}

// lerSenha obtém a senha sem deixar rastro. No terminal, pergunta duas vezes
// com eco desligado; fora dele, lê a primeira linha da entrada padrão.
func lerSenha() (string, error) {
	descritor := int(os.Stdin.Fd())
	if !term.IsTerminal(descritor) {
		leitor := bufio.NewReader(os.Stdin)
		linha, err := leitor.ReadString('\n')
		if err != nil && linha == "" {
			return "", errors.New("não foi possível ler a senha da entrada padrão")
		}
		return strings.TrimRight(linha, "\r\n"), nil
	}

	fmt.Fprint(os.Stderr, "Senha do administrador: ")
	primeira, err := term.ReadPassword(descritor)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", errors.New("não foi possível ler a senha")
	}

	fmt.Fprint(os.Stderr, "Repita a senha: ")
	segunda, err := term.ReadPassword(descritor)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", errors.New("não foi possível ler a senha")
	}

	if string(primeira) != string(segunda) {
		return "", errors.New("as senhas não coincidem")
	}
	return string(primeira), nil
}
