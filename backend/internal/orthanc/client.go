// Package orthanc implementa operações REST de consulta, sem escrita no PACS.
package orthanc

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Failure é uma categoria segura: nunca contém URL, corpo ou credencial.
type Failure string

func (f Failure) Error() string { return string(f) }

const (
	InvalidTarget   Failure = "invalid_target"
	BlockedTarget   Failure = "blocked_target"
	Timeout         Failure = "timeout"
	Canceled        Failure = "canceled"
	DNS             Failure = "dns"
	Refused         Failure = "connection_refused"
	TLS             Failure = "tls"
	Unauthorized    Failure = "unauthorized"
	Forbidden       Failure = "forbidden"
	Upstream        Failure = "upstream_http"
	Redirect        Failure = "redirect"
	InvalidResponse Failure = "invalid_response"
	NotOrthanc      Failure = "not_orthanc"
	Unavailable     Failure = "unavailable"
	NotFound        Failure = "not_found"
	TooLarge        Failure = "too_large"
)

// Config é construída exclusivamente da configuração persistida pelo ADMIN.
type Config struct {
	BaseURL    string
	Username   string
	Credential string
	Timeout    time.Duration
	VerifyTLS  bool
}

// Client não guarda secrets, não faz polling e não usa o transport global.
type Client struct {
	lookup func(context.Context, string, string) ([]netip.Addr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func NewClient() *Client {
	dialer := &net.Dialer{}
	return &Client{lookup: net.DefaultResolver.LookupNetIP, dial: dialer.DialContext}
}

const maxSystemBytes = 64 << 10

// TestSystem confirma a forma esperada da REST API, sem devolver seu conteúdo.
// A identificação é estrutural, não uma prova criptográfica de identidade.
func (c *Client) TestSystem(ctx context.Context, cfg Config) error {
	session, err := c.newSession(cfg)
	if err != nil {
		return err
	}
	defer session.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	body, err := session.read(ctx, http.MethodGet, "/system", nil, maxSystemBytes)
	if err != nil {
		return err
	}
	// Não serializar esta estrutura para a API nem registrar o corpo em log.
	var system struct {
		Version   string `json:"Version"`
		DicomAet  string `json:"DicomAet"`
		DicomPort *int   `json:"DicomPort"`
		HttpPort  *int   `json:"HttpPort"`
	}
	if err := json.Unmarshal(body, &system); err != nil {
		return InvalidResponse
	}
	if strings.TrimSpace(system.Version) == "" || strings.TrimSpace(system.DicomAet) == "" ||
		!validPort(system.DicomPort) || !validPort(system.HttpPort) {
		return NotOrthanc
	}
	return nil
}

// session é privada: os caminhos e métodos vêm somente das operações tipadas.
type session struct {
	client               *http.Client
	base                 *url.URL
	username, credential string
}

func (c *Client) newSession(cfg Config) (*session, error) {
	u, err := systemURL(cfg.BaseURL)
	if err != nil || cfg.Timeout <= 0 || cfg.Timeout > 120*time.Second || strings.Contains(cfg.Username, ":") {
		return nil, InvalidTarget
	}
	u.Path = strings.TrimSuffix(u.Path, "/system")
	transport := &http.Transport{
		Proxy:                  nil,                                                                           // nunca desviar a credencial por HTTP_PROXY/HTTPS_PROXY
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !cfg.VerifyTLS}, // escolha explícita do ADMIN, só neste cliente
		TLSHandshakeTimeout:    cfg.Timeout,
		ResponseHeaderTimeout:  cfg.Timeout,
		MaxResponseHeaderBytes: 16 << 10,
		MaxIdleConnsPerHost:    4,
		DisableCompression:     true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || host != u.Hostname() {
				return nil, InvalidTarget
			}
			var addresses []netip.Addr
			if ip, err := netip.ParseAddr(host); err == nil {
				addresses = []netip.Addr{ip}
			} else {
				addresses, err = c.lookup(ctx, "ip", host)
				if err != nil {
					return nil, classify(err)
				}
			}
			if len(addresses) == 0 {
				return nil, DNS
			}
			// Rejeita também respostas DNS mistas contendo destinos proibidos.
			for _, ip := range addresses {
				if !allowedIP(ip) {
					return nil, BlockedTarget
				}
			}
			var last error
			for _, ip := range addresses {
				// Conecta ao IP validado, sem uma segunda resolução (DNS rebinding).
				conn, err := c.dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			return nil, classify(last)
		},
	}
	client := &http.Client{
		Transport: transport, Timeout: cfg.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &session{client: client, base: u, username: cfg.Username, credential: cfg.Credential}, nil
}

// open é privado e só recebe caminhos construídos pelas operações tipadas.
func (s *session) open(ctx context.Context, method, path string, payload []byte, accept string) (*http.Response, error) {
	u := *s.base
	// Só chamadas internas fornecem path; nunca aceita URL do navegador.
	parts := strings.SplitN(path, "?", 2)
	u.Path += parts[0]
	if len(parts) == 2 {
		u.RawQuery = parts[1]
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, InvalidTarget
	}
	req.Header.Set("Accept", accept)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.username != "" || s.credential != "" {
		req.SetBasicAuth(s.username, s.credential)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, classify(err)
	}
	if response.StatusCode == http.StatusOK {
		return response, nil
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound:
		return nil, NotFound
	case response.StatusCode == http.StatusUnauthorized:
		return nil, Unauthorized
	case response.StatusCode == http.StatusForbidden:
		return nil, Forbidden
	case response.StatusCode >= 300 && response.StatusCode < 400:
		return nil, Redirect
	case response.StatusCode != http.StatusOK:
		return nil, Upstream
	}
	return nil, Upstream
}

func (s *session) read(ctx context.Context, method, path string, payload []byte, maxBytes int64) ([]byte, error) {
	response, err := s.open(ctx, method, path, payload, "application/json")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, classify(err)
	}
	if int64(len(body)) > maxBytes {
		return nil, InvalidResponse
	}
	return body, nil
}

func validPort(port *int) bool { return port != nil && *port > 0 && *port <= 65535 }

func systemURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, InvalidTarget
	}
	if len(raw) > 500 || u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" ||
		u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") ||
		strings.ContainsAny(u.Host, "%\\") || u.RawPath != "" {
		return nil, InvalidTarget
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, InvalidTarget
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, InvalidTarget
	}
	// Permite prefixo de reverse proxy, mas nunca caminhos ambíguos/escapados.
	for _, part := range strings.Split(strings.Trim(u.Path, "/"), "/") {
		if part == "." || part == ".." {
			return nil, InvalidTarget
		}
		for _, ch := range part {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("-._~", ch)) {
				return nil, InvalidTarget
			}
		}
	}
	if strings.Contains(u.Path, "//") {
		return nil, InvalidTarget
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/system"
	return u, nil
}

func allowedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// RFC1918 e ULA são permitidos. Bloqueia redes especiais, tradução IPv6 e
	// endpoints de infraestrutura que não são destinos Orthanc comuns.
	for _, prefix := range blockedNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

var blockedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("168.63.129.16/32"),
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("fd00:ec2::254/128"),
}

func classify(err error) Failure {
	var failure Failure
	if errors.As(err, &failure) {
		return failure
	}
	if errors.Is(err, context.Canceled) {
		return Canceled
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return Timeout
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return DNS
	}
	var verification *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var record tls.RecordHeaderError
	if errors.As(err, &verification) || errors.As(err, &unknown) || errors.As(err, &hostname) || errors.As(err, &invalid) || errors.As(err, &record) {
		return TLS
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return Refused
	}
	return Unavailable
}
