package orthanc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const validSystem = `{"Version":"1.12.0","DicomAet":"SYNTHETIC","DicomPort":4242,"HttpPort":8042,"Name":"não devolver"}`

// DNS e dial sintéticos: exercita HTTP/TLS reais apenas em httptest.Server.
// Produção continua rejeitando loopback; não há opção pública para contornar isso.
func simulatedClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	c := NewClient()
	c.lookup = func(_ context.Context, network, host string) ([]netip.Addr, error) {
		if network != "ip" || host != "orthanc.test" {
			t.Errorf("resolução inesperada: %s %s", network, host)
		}
		return []netip.Addr{netip.MustParseAddr("10.20.30.40")}, nil
	}
	c.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(address)
		if host != "10.20.30.40" {
			t.Errorf("dial não usou IP validado: %s", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	return c
}

func TestSystemResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"orthanc", 200, validSystem, nil},
		{"401", 401, "secret sintético não deve escapar", Unauthorized},
		{"403", 403, "secret sintético não deve escapar", Forbidden},
		{"500", 500, "secret sintético não deve escapar", Upstream},
		{"204", 204, "", Upstream},
		{"json inválido", 200, "{segredo-sintetico", InvalidResponse},
		{"outro serviço", 200, `{"status":"ok"}`, NotOrthanc},
		{"null", 200, `null`, NotOrthanc},
		{"tipos errados", 200, `{"Version":42}`, InvalidResponse},
		{"json adicional", 200, validSystem + `{}`, InvalidResponse},
		{"resposta grande", 200, strings.Repeat("x", maxSystemBytes+1), InvalidResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != "GET" || r.URL.RequestURI() != "/pacs/system" {
					t.Errorf("operação inesperada: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("sem credencial não deve enviar Authorization")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			err := simulatedClient(t, server).TestSystem(context.Background(), Config{BaseURL: "http://orthanc.test/pacs/", Timeout: time.Second, VerifyTLS: true})
			if !errors.Is(err, tc.want) {
				t.Fatalf("erro=%v esperado=%v", err, tc.want)
			}
			if requests.Load() != 1 {
				t.Fatal("teste deve executar somente uma requisição")
			}
		})
	}
}

func TestSystemCredentialsAndNoEnvironmentProxy(t *testing.T) {
	var proxyCalls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyCalls.Add(1); w.WriteHeader(500) }))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "usuario-ficticio" || password != " senha-ficticia " {
			t.Error("Basic Auth divergente")
		}
		_, _ = io.WriteString(w, validSystem)
	}))
	defer server.Close()
	err := simulatedClient(t, server).TestSystem(context.Background(), Config{BaseURL: "http://orthanc.test", Username: "usuario-ficticio", Credential: " senha-ficticia ", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if proxyCalls.Load() != 0 {
		t.Fatal("proxy de ambiente foi utilizado")
	}
}

func TestSystemNeverFollowsRedirects(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls.Add(1) }))
	defer destination.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, status) }))
			defer server.Close()
			err := simulatedClient(t, server).TestSystem(context.Background(), Config{BaseURL: "http://orthanc.test", Timeout: time.Second, Credential: "ficticia"})
			if !errors.Is(err, Redirect) {
				t.Fatal(err)
			}
		})
	}
	if destinationCalls.Load() != 0 {
		t.Fatal("redirect foi seguido")
	}
}

func TestSystemTimeoutAndCancellation(t *testing.T) {
	for _, bodyTimeout := range []bool{false, true} {
		t.Run(fmt.Sprint(bodyTimeout), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if bodyTimeout {
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			c := simulatedClient(t, server)
			cfg := Config{BaseURL: "http://orthanc.test", Timeout: 40 * time.Millisecond}
			if err := c.TestSystem(context.Background(), cfg); !errors.Is(err, Timeout) {
				t.Fatalf("timeout: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := c.TestSystem(ctx, cfg); !errors.Is(err, Canceled) {
				t.Fatalf("cancelamento: %v", err)
			}
		})
	}
}

func TestSystemTLSIsScoped(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, validSystem) }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	for _, verify := range []bool{true, false, true} {
		err := simulatedClient(t, server).TestSystem(context.Background(), Config{BaseURL: "https://orthanc.test", Timeout: time.Second, VerifyTLS: verify})
		if verify && !errors.Is(err, TLS) {
			t.Fatalf("deveria validar certificado: %v", err)
		}
		if !verify && err != nil {
			t.Fatalf("desativação explícita: %v", err)
		}
	}
}

func TestSystemNetworkFailuresAreSanitized(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"dns", &net.DNSError{Err: "detalhe-interno-ficticio", Name: "secreto.invalid"}, DNS},
		{"recusada", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, Refused},
		{"genérica", errors.New("credencial-ficticia"), Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient()
			c.lookup = func(context.Context, string, string) ([]netip.Addr, error) { return nil, tc.err }
			c.dial = func(context.Context, string, string) (net.Conn, error) {
				t.Error("dial inesperado")
				return nil, tc.err
			}
			err := c.TestSystem(context.Background(), Config{BaseURL: "http://orthanc.test", Timeout: time.Second})
			if !errors.Is(err, tc.want) {
				t.Fatalf("erro=%v esperado=%v", err, tc.want)
			}
		})
	}
}

func TestSystemRejectsUnsafeTargetsBeforeDial(t *testing.T) {
	for _, target := range []string{
		"file:///system", "ftp://orthanc.test", "http://u:p@orthanc.test", "http://orthanc.test?x=1", "http://orthanc.test?", "http://orthanc.test#",
		"http://orthanc.test:0", "http://orthanc.test:65536", "http://orthanc.test:/", "http://orthanc.test/../admin", "http://orthanc.test/%2e%2e/admin", "http://orthanc.test/a%2fb", "http://orthanc.test//evil",
		"http://127.0.0.1", "http://[::1]", "http://[::ffff:127.0.0.1]", "http://0.0.0.0", "http://169.254.169.254", "http://[fe80::1]", "http://224.0.0.1", "http://100.100.100.200", "http://168.63.129.16", "http://[fd00:ec2::254]", "http://[64:ff9b::7f00:1]",
	} {
		t.Run(target, func(t *testing.T) {
			c := NewClient()
			c.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
				t.Error("DNS inesperado")
				return nil, DNS
			}
			c.dial = func(context.Context, string, string) (net.Conn, error) {
				t.Error("dial proibido")
				return nil, Unavailable
			}
			err := c.TestSystem(context.Background(), Config{BaseURL: target, Timeout: time.Second})
			if !errors.Is(err, InvalidTarget) && !errors.Is(err, BlockedTarget) {
				t.Fatalf("destino não bloqueado: %v", err)
			}
		})
	}
}

func TestSystemRejectsDNSRebindingAndMixedAddresses(t *testing.T) {
	for _, addresses := range [][]netip.Addr{
		{netip.MustParseAddr("169.254.169.254")},
		{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("127.0.0.1")},
	} {
		c := NewClient()
		c.lookup = func(context.Context, string, string) ([]netip.Addr, error) { return addresses, nil }
		c.dial = func(context.Context, string, string) (net.Conn, error) {
			t.Error("dial proibido")
			return nil, Unavailable
		}
		if err := c.TestSystem(context.Background(), Config{BaseURL: "http://orthanc.test", Timeout: time.Second}); !errors.Is(err, BlockedTarget) {
			t.Fatal(err)
		}
	}
}

func TestPrivateNetworksAreAllowed(t *testing.T) {
	for _, address := range []string{"10.1.2.3", "172.16.4.5", "192.168.1.10", "fd12:3456::1"} {
		if !allowedIP(netip.MustParseAddr(address)) {
			t.Errorf("rede privada recusada: %s", address)
		}
	}
}
