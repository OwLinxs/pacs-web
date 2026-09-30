// Package config carrega a configuração do backend a partir do ambiente.
//
// Só entra aqui o que o servidor precisa para subir. Configuração operacional
// (por exemplo a integração com o Orthanc) fica no banco e será administrada
// pela interface numa etapa futura.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/secrets"
)

// Env identifica o ambiente de execução.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvProduction  Env = "production"
)

// Config é a configuração efetiva do processo. Nenhum campo é logado.
type Config struct {
	Env      Env
	HTTPAddr string

	DatabaseURL string

	// SessionAbsoluteTTL encerra a sessão mesmo com uso contínuo.
	SessionAbsoluteTTL time.Duration
	// SessionIdleTTL encerra a sessão por inatividade.
	SessionIdleTTL time.Duration

	// AllowedOrigins habilita CORS apenas para origens explícitas. Em
	// desenvolvimento o caminho recomendado é o proxy do Vite (mesma origem),
	// então normalmente fica vazio.
	AllowedOrigins    []string
	PublicOrigin      string
	TrustedProxyCIDRs []netip.Prefix

	// CookieSecure marca os cookies como Secure. Obrigatório em produção.
	CookieSecure bool

	// LoginRateLimit define a janela de proteção de POST /api/auth/login.
	LoginRateAttempts int
	LoginRateWindow   time.Duration

	ShutdownTimeout time.Duration

	// MasterKey é a chave mestra que cifra secrets em repouso (PACS_MASTER_KEY).
	// Vem só do ambiente: não fica no banco, não vai para o frontend, não entra
	// no Git e não aparece em log. Vazia em desenvolvimento desabilita o
	// armazenamento de credenciais.
	MasterKey string

	// StaticDir, quando preenchido, faz o backend servir o build do frontend na
	// mesma origem da API — o arranjo preferido em produção, que dispensa CORS.
	StaticDir string
}

// Load monta a configuração a partir das variáveis de ambiente e valida o
// conjunto. Retorna erro descrevendo todas as variáveis com problema.
func Load() (Config, error) {
	cfg := Config{
		Env:                Env(getenv("APP_ENV", string(EnvDevelopment))),
		HTTPAddr:           getenv("HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		SessionAbsoluteTTL: 12 * time.Hour,
		SessionIdleTTL:     30 * time.Minute,
		LoginRateAttempts:  10,
		LoginRateWindow:    5 * time.Minute,
		ShutdownTimeout:    15 * time.Second,
		StaticDir:          os.Getenv("STATIC_DIR"),
		MasterKey:          os.Getenv("PACS_MASTER_KEY"),
		PublicOrigin:       strings.TrimSpace(os.Getenv("PUBLIC_ORIGIN")),
	}

	var problemas []string

	switch cfg.Env {
	case EnvDevelopment, EnvProduction:
	default:
		problemas = append(problemas, `APP_ENV deve ser "development" ou "production"`)
	}

	if cfg.DatabaseURL == "" {
		problemas = append(problemas, "DATABASE_URL é obrigatória")
	}

	if err := durationEnv("SESSION_ABSOLUTE_TTL", &cfg.SessionAbsoluteTTL); err != nil {
		problemas = append(problemas, err.Error())
	}
	if err := durationEnv("SESSION_IDLE_TTL", &cfg.SessionIdleTTL); err != nil {
		problemas = append(problemas, err.Error())
	}
	if err := durationEnv("SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout); err != nil {
		problemas = append(problemas, err.Error())
	}
	if err := durationEnv("LOGIN_RATE_WINDOW", &cfg.LoginRateWindow); err != nil {
		problemas = append(problemas, err.Error())
	}
	if err := intEnv("LOGIN_RATE_ATTEMPTS", &cfg.LoginRateAttempts); err != nil {
		problemas = append(problemas, err.Error())
	}
	if cfg.LoginRateAttempts > 20 || cfg.LoginRateWindow < time.Minute {
		problemas = append(problemas, "LOGIN_RATE_ATTEMPTS deve ser <= 20 e LOGIN_RATE_WINDOW >= 1m")
	}

	if origens := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS")); origens != "" {
		for _, origem := range strings.Split(origens, ",") {
			origem = strings.TrimSpace(origem)
			if origem == "" {
				continue
			}
			if !validOrigin(origem) {
				problemas = append(problemas, fmt.Sprintf("ALLOWED_ORIGINS: %q não é uma origem absoluta", origem))
				continue
			}
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, origem)
		}
	}
	if cfg.PublicOrigin != "" && !validOrigin(cfg.PublicOrigin) {
		problemas = append(problemas, "PUBLIC_ORIGIN deve conter somente scheme e host de uma origem absoluta")
	}
	if cfg.Env == EnvProduction && !strings.HasPrefix(cfg.PublicOrigin, "https://") {
		problemas = append(problemas, "PUBLIC_ORIGIN HTTPS é obrigatória em produção")
	}
	if cfg.Env == EnvProduction && len(cfg.AllowedOrigins) > 0 {
		problemas = append(problemas, "ALLOWED_ORIGINS deve ficar vazio em produção")
	}
	if bruto := strings.TrimSpace(os.Getenv("TRUSTED_PROXY_CIDRS")); bruto != "" {
		for _, item := range strings.Split(bruto, ",") {
			prefixo, err := netip.ParsePrefix(strings.TrimSpace(item))
			if err != nil {
				problemas = append(problemas, "TRUSTED_PROXY_CIDRS contém CIDR inválido")
				continue
			}
			cfg.TrustedProxyCIDRs = append(cfg.TrustedProxyCIDRs, prefixo.Masked())
		}
	}
	if cfg.Env == EnvProduction && len(cfg.TrustedProxyCIDRs) == 0 {
		problemas = append(problemas, "TRUSTED_PROXY_CIDRS é obrigatório em produção")
	}

	// Em produção o cookie é sempre Secure; em desenvolvimento permite HTTP local.
	cfg.CookieSecure = cfg.Env == EnvProduction
	if bruto := os.Getenv("COOKIE_SECURE"); bruto != "" {
		valor, err := strconv.ParseBool(bruto)
		switch {
		case err != nil:
			problemas = append(problemas, "COOKIE_SECURE deve ser booleano")
		case !valor && cfg.Env == EnvProduction:
			problemas = append(problemas, "COOKIE_SECURE não pode ser desligado em produção")
		default:
			cfg.CookieSecure = valor
		}
	}

	if cfg.MasterKey != "" {
		if _, err := secrets.ParseKey(cfg.MasterKey); err != nil {
			problemas = append(problemas, "PACS_MASTER_KEY: "+err.Error())
		}
	} else if cfg.Env == EnvProduction {
		problemas = append(problemas, "PACS_MASTER_KEY é obrigatória em produção")
	}

	if len(problemas) > 0 {
		return Config{}, errors.New("configuração inválida: " + strings.Join(problemas, "; "))
	}
	return cfg, nil
}

// IsProduction indica se o processo roda em produção.
func (c Config) IsProduction() bool { return c.Env == EnvProduction }

func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

func getenv(chave, padrao string) string {
	if valor := os.Getenv(chave); valor != "" {
		return valor
	}
	return padrao
}

func durationEnv(chave string, destino *time.Duration) error {
	bruto := os.Getenv(chave)
	if bruto == "" {
		return nil
	}
	valor, err := time.ParseDuration(bruto)
	if err != nil || valor <= 0 {
		return fmt.Errorf("%s deve ser uma duração positiva (ex.: 30m)", chave)
	}
	*destino = valor
	return nil
}

func intEnv(chave string, destino *int) error {
	bruto := os.Getenv(chave)
	if bruto == "" {
		return nil
	}
	valor, err := strconv.Atoi(bruto)
	if err != nil || valor <= 0 {
		return fmt.Errorf("%s deve ser um inteiro positivo", chave)
	}
	*destino = valor
	return nil
}
