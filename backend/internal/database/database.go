// Package database abre o pool PostgreSQL e aplica as migrations versionadas.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config são os parâmetros do pool. Os valores padrão servem a um backend
// municipal de porte pequeno; ajuste por ambiente se a carga exigir.
type Config struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	ConnectTimeout  time.Duration
}

// DefaultConfig devolve a configuração de pool padrão para a URL informada.
func DefaultConfig(url string) Config {
	return Config{
		URL:             url,
		MaxConns:        10,
		MinConns:        1,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 15 * time.Minute,
		ConnectTimeout:  5 * time.Second,
	}
}

// Open cria o pool e confirma a conectividade antes de devolvê-lo.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		// A URL pode conter credenciais: não repassamos a mensagem original.
		return nil, fmt.Errorf("DATABASE_URL inválida")
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("abrir pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectar ao PostgreSQL: %w", err)
	}
	return pool, nil
}
