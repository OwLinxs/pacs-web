package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pmfb-saude/pacs-web/backend/migrations"
)

// Migration é um arquivo SQL versionado.
type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

// LoadMigrations lê e ordena as migrations embutidas.
//
// Nome esperado: NNNN_descricao.sql, com NNNN único e crescente.
func LoadMigrations() ([]Migration, error) {
	return loadFrom(migrations.FS)
}

func loadFrom(sistema fs.FS) ([]Migration, error) {
	arquivos, err := fs.Glob(sistema, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("ler migrations: %w", err)
	}

	lista := make([]Migration, 0, len(arquivos))
	for _, nome := range arquivos {
		partes := strings.SplitN(strings.TrimSuffix(nome, ".sql"), "_", 2)
		if len(partes) != 2 {
			return nil, fmt.Errorf("migration %q: nome deve ser NNNN_descricao.sql", nome)
		}
		versao, err := strconv.Atoi(partes[0])
		if err != nil || versao <= 0 {
			return nil, fmt.Errorf("migration %q: versão inválida", nome)
		}
		conteudo, err := fs.ReadFile(sistema, nome)
		if err != nil {
			return nil, fmt.Errorf("ler migration %q: %w", nome, err)
		}
		soma := sha256.Sum256(conteudo)
		lista = append(lista, Migration{
			Version:  versao,
			Name:     partes[1],
			SQL:      string(conteudo),
			Checksum: hex.EncodeToString(soma[:]),
		})
	}

	sort.Slice(lista, func(i, j int) bool { return lista[i].Version < lista[j].Version })
	for i := 1; i < len(lista); i++ {
		if lista[i].Version == lista[i-1].Version {
			return nil, fmt.Errorf("migration %d duplicada", lista[i].Version)
		}
	}
	return lista, nil
}

const criarTabelaControle = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    integer     PRIMARY KEY,
    name       text        NOT NULL,
    checksum   text        NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

// Migrate aplica as migrations pendentes, cada uma em sua própria transação.
//
// Só avança: não há migrations de reversão e nada é apagado automaticamente.
// Uma migration já aplicada cujo arquivo tenha mudado é tratada como erro, para
// que uma alteração retroativa não passe silenciosa.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	lista, err := LoadMigrations()
	if err != nil {
		return err
	}

	if _, err := pool.Exec(ctx, criarTabelaControle); err != nil {
		return fmt.Errorf("criar schema_migrations: %w", err)
	}

	aplicadas, err := aplicadasPorVersao(ctx, pool)
	if err != nil {
		return err
	}

	novas := 0
	for _, migration := range lista {
		if checksum, ok := aplicadas[migration.Version]; ok {
			if checksum != migration.Checksum {
				return fmt.Errorf(
					"migration %04d_%s já aplicada com conteúdo diferente: crie uma nova migration em vez de editar esta",
					migration.Version, migration.Name)
			}
			continue
		}
		if err := aplicar(ctx, pool, migration); err != nil {
			return err
		}
		novas++
		log.Info("migration aplicada", "version", migration.Version, "name", migration.Name)
	}

	log.Info("migrations em dia", "aplicadas_agora", novas, "versao_atual", versaoMaxima(lista))
	return nil
}

func aplicadasPorVersao(ctx context.Context, pool *pgxpool.Pool) (map[int]string, error) {
	linhas, err := pool.Query(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("ler schema_migrations: %w", err)
	}
	defer linhas.Close()

	aplicadas := map[int]string{}
	for linhas.Next() {
		var versao int
		var checksum string
		if err := linhas.Scan(&versao, &checksum); err != nil {
			return nil, fmt.Errorf("ler schema_migrations: %w", err)
		}
		aplicadas[versao] = checksum
	}
	if err := linhas.Err(); err != nil {
		return nil, fmt.Errorf("ler schema_migrations: %w", err)
	}
	return aplicadas, nil
}

func aplicar(ctx context.Context, pool *pgxpool.Pool, migration Migration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciar transação da migration %04d: %w", migration.Version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, migration.SQL); err != nil {
		return fmt.Errorf("aplicar migration %04d_%s: %w", migration.Version, migration.Name, err)
	}
	const registrar = `INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`
	if _, err := tx.Exec(ctx, registrar, migration.Version, migration.Name, migration.Checksum); err != nil {
		return fmt.Errorf("registrar migration %04d: %w", migration.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmar migration %04d: %w", migration.Version, err)
	}
	return nil
}

func versaoMaxima(lista []Migration) int {
	if len(lista) == 0 {
		return 0
	}
	return lista[len(lista)-1].Version
}
