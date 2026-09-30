package config

import (
	"strings"
	"testing"
	"time"
)

// prepararAmbiente limpa as variáveis relevantes e define o mínimo necessário.
func prepararAmbiente(t *testing.T) {
	t.Helper()
	for _, chave := range []string{
		"APP_ENV", "HTTP_ADDR", "DATABASE_URL", "SESSION_ABSOLUTE_TTL", "SESSION_IDLE_TTL",
		"SHUTDOWN_TIMEOUT", "LOGIN_RATE_WINDOW", "LOGIN_RATE_ATTEMPTS", "ALLOWED_ORIGINS",
		"COOKIE_SECURE", "STATIC_DIR", "PACS_MASTER_KEY", "PUBLIC_ORIGIN", "TRUSTED_PROXY_CIDRS",
	} {
		t.Setenv(chave, "")
	}
	t.Setenv("DATABASE_URL", "postgres://pacs:<SECRET>@localhost:5432/pacsweb")
}

func TestLoadPadroes(t *testing.T) {
	prepararAmbiente(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q, esperado development", cfg.Env)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.SessionIdleTTL != 30*time.Minute {
		t.Errorf("SessionIdleTTL = %v, esperado 30m", cfg.SessionIdleTTL)
	}
	if cfg.CookieSecure {
		t.Error("em desenvolvimento CookieSecure deve começar desligado")
	}
	if len(cfg.AllowedOrigins) != 0 {
		t.Errorf("AllowedOrigins = %v, esperado vazio", cfg.AllowedOrigins)
	}
}

func TestLoadExigeDatabaseURL(t *testing.T) {
	prepararAmbiente(t)
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("esperava erro sem DATABASE_URL")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("erro = %v, deveria citar DATABASE_URL", err)
	}
}

// chaveMestraDeTeste é uma chave de 32 bytes em base64, só para os testes.
const chaveMestraDeTeste = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestLoadProducaoExigeCookieSecure(t *testing.T) {
	prepararAmbiente(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PACS_MASTER_KEY", chaveMestraDeTeste)
	t.Setenv("PUBLIC_ORIGIN", "https://pacs.example.invalid")
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.20.0.0/16")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.CookieSecure {
		t.Error("em produção CookieSecure deve vir ligado")
	}

	t.Setenv("COOKIE_SECURE", "false")
	if _, err := Load(); err == nil {
		t.Error("desligar COOKIE_SECURE em produção deveria ser rejeitado")
	}
}

func TestLoadPublicOriginEProxy(t *testing.T) {
	prepararAmbiente(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PACS_MASTER_KEY", chaveMestraDeTeste)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.20.0.0/16")
	for _, origin := range []string{"", "http://pacs.example.invalid", "https://pacs.example.invalid/path", "https://user@pacs.example.invalid"} {
		t.Setenv("PUBLIC_ORIGIN", origin)
		if _, err := Load(); err == nil {
			t.Errorf("origem %q deveria ser recusada", origin)
		}
	}
	t.Setenv("PUBLIC_ORIGIN", "https://pacs.example.invalid")
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.20.0.0/16")
	cfg, err := Load()
	if err != nil || len(cfg.TrustedProxyCIDRs) != 1 {
		t.Fatalf("configuração válida: %v", err)
	}
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	if _, err := Load(); err == nil {
		t.Fatal("proxy não configurado aceito em produção")
	}
	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr")
	if _, err := Load(); err == nil {
		t.Fatal("CIDR inválido aceito")
	}
}

func TestLoadValidaValores(t *testing.T) {
	casos := []struct {
		nome  string
		chave string
		valor string
	}{
		{"ambiente desconhecido", "APP_ENV", "homologacao"},
		{"duração inválida", "SESSION_IDLE_TTL", "trinta-minutos"},
		{"duração negativa", "SESSION_IDLE_TTL", "-5m"},
		{"tentativas não numéricas", "LOGIN_RATE_ATTEMPTS", "muitas"},
		{"tentativas zero", "LOGIN_RATE_ATTEMPTS", "0"},
		{"origem relativa", "ALLOWED_ORIGINS", "localhost:5173"},
		{"cookie secure inválido", "COOKIE_SECURE", "talvez"},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			prepararAmbiente(t)
			t.Setenv(caso.chave, caso.valor)
			if _, err := Load(); err == nil {
				t.Errorf("%s=%q deveria ser rejeitado", caso.chave, caso.valor)
			}
		})
	}
}

func TestLoadOrigensPermitidas(t *testing.T) {
	prepararAmbiente(t)
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:5173, https://pacs.exemplo.invalid ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("AllowedOrigins = %v", cfg.AllowedOrigins)
	}
	if cfg.AllowedOrigins[0] != "http://localhost:5173" || cfg.AllowedOrigins[1] != "https://pacs.exemplo.invalid" {
		t.Errorf("origens = %v", cfg.AllowedOrigins)
	}
}

func TestLoadChaveMestra(t *testing.T) {
	t.Run("opcional em desenvolvimento", func(t *testing.T) {
		prepararAmbiente(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.MasterKey != "" {
			t.Error("MasterKey deveria ficar vazia quando não informada")
		}
	})

	t.Run("obrigatória em produção", func(t *testing.T) {
		prepararAmbiente(t)
		t.Setenv("APP_ENV", "production")
		if _, erro := Load(); erro == nil {
			t.Error("produção sem PACS_MASTER_KEY deveria ser rejeitada")
		}
	})

	t.Run("rejeita chave de tamanho errado", func(t *testing.T) {
		prepararAmbiente(t)
		t.Setenv("PACS_MASTER_KEY", "Y2hhdmUtY3VydGE=")
		if _, err := Load(); err == nil {
			t.Error("chave fora de 32 bytes deveria ser rejeitada")
		}
	})

	t.Run("aceita chave válida", func(t *testing.T) {
		prepararAmbiente(t)
		t.Setenv("PACS_MASTER_KEY", chaveMestraDeTeste)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.MasterKey != chaveMestraDeTeste {
			t.Error("MasterKey não foi carregada")
		}
	})
}
