package httpapi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/config"
)

func TestTrustedProxyClientIP(t *testing.T) {
	s := &Server{cfg: config.Config{TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("172.20.0.0/16")}}}
	r := httptest.NewRequest("GET", "http://pacs.example.invalid/api/health", nil)
	r.RemoteAddr = "198.51.100.5:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.10")
	if got := s.ipDoPedido(r); got != "198.51.100.5" {
		t.Fatalf("peer não confiável: %s", got)
	}
	r.RemoteAddr = "172.20.0.2:1234"
	r.Header.Set("X-Forwarded-For", "192.0.2.8, 172.20.0.3")
	if got := s.ipDoPedido(r); got != "192.0.2.8" {
		t.Fatalf("cadeia proxy: %s", got)
	}
	r.Header.Set("X-Forwarded-For", "192.0.2.8, invalid")
	if got := s.ipDoPedido(r); got != "172.20.0.2" {
		t.Fatalf("header malformado: %s", got)
	}
}

func TestProductionOrigin(t *testing.T) {
	s := &Server{cfg: config.Config{Env: config.EnvProduction, PublicOrigin: "https://pacs.example.invalid"}}
	r := httptest.NewRequest("POST", "http://pacs.example.invalid/api/auth/login", nil)
	if s.origemPermitida(r) {
		t.Fatal("origem ausente aceita")
	}
	r.Header.Set("Origin", "http://pacs.example.invalid")
	if s.origemPermitida(r) {
		t.Fatal("downgrade HTTP aceito")
	}
	r.Header.Set("Origin", "https://pacs.example.invalid/path")
	if s.origemPermitida(r) {
		t.Fatal("Origin com path aceito")
	}
	r.Header.Set("Origin", "https://pacs.example.invalid")
	if !s.origemPermitida(r) {
		t.Fatal("origem pública rejeitada")
	}
	r.Host = "evil.example.invalid"
	if s.origemPermitida(r) {
		t.Fatal("Host divergente aceito")
	}
}

func TestRateLimiterFailClosed(t *testing.T) {
	rl := newRateLimiter(10, time.Minute, time.Now)
	rl.maximo = 1
	if ok, _ := rl.Allow("first"); !ok {
		t.Fatal("primeira chave bloqueada")
	}
	if ok, _ := rl.Allow("second"); ok {
		t.Fatal("tabela saturada falhou aberta")
	}
}

func TestRequestLogRouteHidesArbitraryPath(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/identificador-sintetico", nil)
	if got := requestLogRoute(r); strings.Contains(got, "identificador-sintetico") {
		t.Fatalf("path vazou: %s", got)
	}
}

func TestInternalLogsDoNotIncludeSyntheticSecret(t *testing.T) {
	const marker = "SYNTHETIC_SECRET_MARKER"
	var buffer bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buffer, nil))
	s := &Server{log: log}
	r := httptest.NewRequest("GET", "/api/"+marker, nil)
	w := httptest.NewRecorder()
	writeInternalError(w, log, r, errors.New(marker))
	s.recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(marker) })).ServeHTTP(httptest.NewRecorder(), r)
	if strings.Contains(buffer.String(), marker) {
		t.Fatal("erro/pânico/path sintético vazou no log")
	}
}
