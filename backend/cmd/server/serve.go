package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/config"
	"github.com/pmfb-saude/pacs-web/backend/internal/database"
	"github.com/pmfb-saude/pacs-web/backend/internal/httpapi"
	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/secrets"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
	"github.com/pmfb-saude/pacs-web/backend/internal/units"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
	"github.com/pmfb-saude/pacs-web/backend/internal/useradmin"
)

// intervaloManutencao é a periodicidade da limpeza de sessões vencidas e de
// janelas do rate limiter.
const intervaloManutencao = 10 * time.Minute

func executarServe(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := database.Open(ctx, database.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.IsProduction() {
		if err := database.VerifyRuntimeRole(ctx, pool); err != nil {
			return err
		}
		if err := database.VerifyMigrations(ctx, pool); err != nil {
			return err
		}
	} else {
		if err := database.Migrate(ctx, pool, log); err != nil {
			return err
		}
	}

	usuarios := user.NewStore(pool)
	sessoes := auth.NewSessionStore(pool)
	auditoria := audit.NewRecorder(pool, log)

	// Sem chave mestra a aplicação sobe, mas não guarda credencial do Orthanc.
	// Em produção a configuração já exige a chave.
	var cifra *secrets.Cipher
	if cfg.MasterKey != "" {
		cifra, err = secrets.NewCipherFromString(cfg.MasterKey)
		if err != nil {
			return err
		}
	} else {
		log.Warn("PACS_MASTER_KEY ausente: credenciais do Orthanc não poderão ser guardadas")
	}
	configuracoes := settings.NewStore(pool, cifra)

	servicoAuth, err := auth.NewService(usuarios, sessoes, auth.NewDefaultHasher(), auditoria, log, auth.Options{
		AbsoluteTTL: cfg.SessionAbsoluteTTL,
		IdleTTL:     cfg.SessionIdleTTL,
	})
	if err != nil {
		return err
	}

	orthancClient := orthanc.NewClient()
	api, err := httpapi.New(httpapi.Deps{
		Config:      cfg,
		Log:         log,
		Auth:        servicoAuth,
		DB:          pool,
		Settings:    configuracoes,
		Orthanc:     orthancClient,
		Studies:     orthancClient,
		Viewer:      orthancClient,
		Units:       units.NewStore(pool),
		Users:       useradmin.New(useradmin.NewStore(pool), auth.NewDefaultHasher()),
		Auditoria:   auditoria,
		AuditReader: auditoria,
	})
	if err != nil {
		return err
	}

	servidor := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.Handler(),
		// Timeouts explícitos: um cliente lento ou malicioso não deve prender
		// conexão indefinidamente.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	manutencaoEncerrada := iniciarManutencao(ctx, log, sessoes, api, cfg.SessionIdleTTL)

	erroServidor := make(chan error, 1)
	go func() {
		log.Info("servidor iniciado",
			"endereco", cfg.HTTPAddr,
			"ambiente", string(cfg.Env),
			"cookie_secure", cfg.CookieSecure,
			"origens_permitidas", cfg.AllowedOrigins,
			"frontend_estatico", cfg.StaticDir != "",
		)
		if err := servidor.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			erroServidor <- err
			return
		}
		erroServidor <- nil
	}()

	select {
	case err := <-erroServidor:
		if err != nil {
			return fmt.Errorf("servidor HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		log.Info("sinal de encerramento recebido, drenando requisições")
	}

	desligarCtx, cancelar := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelar()

	if err := servidor.Shutdown(desligarCtx); err != nil {
		log.Error("encerramento não concluiu no prazo")
		_ = servidor.Close()
	}
	<-manutencaoEncerrada
	log.Info("servidor encerrado")
	return nil
}

// iniciarManutencao roda a limpeza periódica e devolve um canal fechado quando
// a rotina termina.
func iniciarManutencao(ctx context.Context, log *slog.Logger, sessoes *auth.SessionStore, api *httpapi.Server, idleTTL time.Duration) <-chan struct{} {
	pronto := make(chan struct{})
	go func() {
		defer close(pronto)
		ticker := time.NewTicker(intervaloManutencao)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				api.VacuumRateLimiter()
				// Contexto próprio: a limpeza não deve segurar o encerramento.
				limparCtx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
				removidas, err := sessoes.DeleteExpired(limparCtx, time.Now(), idleTTL)
				cancelar()
				if err != nil {
					log.Error("falha ao limpar sessões vencidas")
					continue
				}
				if removidas > 0 {
					log.Info("sessões vencidas removidas", "quantidade", removidas)
				}
			}
		}
	}()
	return pronto
}

func executarMigrate(ctx context.Context, log *slog.Logger) error {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = string(config.EnvDevelopment)
	}
	if env != string(config.EnvDevelopment) && env != string(config.EnvProduction) {
		return errors.New("APP_ENV inválido")
	}
	url := os.Getenv("DATABASE_URL")
	if env == string(config.EnvProduction) {
		url = os.Getenv("MIGRATION_DATABASE_URL")
	}
	if url == "" {
		return errors.New("DSN de migration ausente")
	}
	pool, err := database.Open(ctx, database.DefaultConfig(url))
	if err != nil {
		return err
	}
	defer pool.Close()
	return database.Migrate(ctx, pool, log)
}
