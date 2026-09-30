package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/authtest"
	"github.com/pmfb-saude/pacs-web/backend/internal/config"
	"github.com/pmfb-saude/pacs-web/backend/internal/httpapi"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

const senhaTeste = "senha-de-teste-longa"

func hasherRapido() *auth.Hasher {
	return auth.NewHasher(auth.Argon2Params{Memory: 8 * 1024, Time: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
}

// cenario é uma API completa sobre dependências em memória.
// httpapiCodeInvalidRequest evita repetir o nome longo nos testes.
const httpapiCodeInvalidRequest = httpapi.CodeInvalidRequest

type cenario struct {
	servidor  *httptest.Server
	cliente   *http.Client
	usuarios  *authtest.Users
	auditoria *authtest.Auditoria
	settings  *settingsFake
	base      *url.URL
}

type opcoesCenario struct {
	auditReader     httpapi.AuditReader
	tentativasLogin int
	orthanc         httpapi.OrthancTester
	studies         httpapi.StudyFinder
	viewer          httpapi.ViewerClient
	units           httpapi.UnitsStore
	users           func(*authtest.Users, *authtest.Sessions) httpapi.UsersService
	log             *slog.Logger
}

func montarCenario(t *testing.T, opcoes opcoesCenario) *cenario {
	t.Helper()

	if opcoes.tentativasLogin == 0 {
		opcoes.tentativasLogin = 10
	}

	usuarios := authtest.NewUsers()
	sessoes := authtest.NewSessions(usuarios)
	auditoria := authtest.NewAuditoria()
	log := opcoes.log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	servicoAuth, err := auth.NewService(usuarios, sessoes, hasherRapido(), auditoria, log, auth.Options{
		AbsoluteTTL: 12 * time.Hour,
		IdleTTL:     30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("auth.NewService: %v", err)
	}

	configuracoes := novoSettingsFake()
	var usersService httpapi.UsersService
	if opcoes.users != nil {
		usersService = opcoes.users(usuarios, sessoes)
	}

	api, err := httpapi.New(httpapi.Deps{
		Config: config.Config{
			Env:               config.EnvDevelopment,
			CookieSecure:      false,
			LoginRateAttempts: opcoes.tentativasLogin,
			LoginRateWindow:   5 * time.Minute,
		},
		Log:         log,
		Auth:        servicoAuth,
		Settings:    configuracoes,
		Orthanc:     opcoes.orthanc,
		Studies:     opcoes.studies,
		Viewer:      opcoes.viewer,
		Units:       opcoes.units,
		Users:       usersService,
		Auditoria:   auditoria,
		AuditReader: opcoes.auditReader,
	})
	if err != nil {
		t.Fatalf("httpapi.New: %v", err)
	}

	servidor := httptest.NewServer(api.Handler())
	t.Cleanup(servidor.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	base, err := url.Parse(servidor.URL)
	if err != nil {
		t.Fatalf("url: %v", err)
	}

	return &cenario{
		servidor:  servidor,
		cliente:   &http.Client{Jar: jar},
		usuarios:  usuarios,
		auditoria: auditoria,
		settings:  configuracoes,
		base:      base,
	}
}

// adicionarUsuario registra um usuário com a senha de teste.
func (c *cenario) adicionarUsuario(t *testing.T, username string, role user.Role, ajustar func(*user.User)) user.User {
	t.Helper()
	hash, err := hasherRapido().Hash(senhaTeste)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	usuario := user.User{
		ID:           uuid.New(),
		Name:         "Usuario Teste",
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Active:       true,
	}
	if ajustar != nil {
		ajustar(&usuario)
	}
	c.usuarios.Add(usuario)
	return usuario
}

// tokenCSRF devolve o valor do cookie de CSRF guardado no jar.
func (c *cenario) tokenCSRF() string {
	for _, cookie := range c.cliente.Jar.Cookies(c.base) {
		if cookie.Name == "pacs_csrf" {
			return cookie.Value
		}
	}
	return ""
}

func (c *cenario) cookieSessao() string {
	for _, cookie := range c.cliente.Jar.Cookies(c.base) {
		if cookie.Name == "pacs_session" {
			return cookie.Value
		}
	}
	return ""
}

// requisitar envia uma requisição já com o header de CSRF quando há cookie.
func (c *cenario) requisitar(t *testing.T, metodo, caminho string, corpo any) *http.Response {
	t.Helper()

	var leitor io.Reader
	if corpo != nil {
		bruto, err := json.Marshal(corpo)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		leitor = bytes.NewReader(bruto)
	}

	pedido, err := http.NewRequest(metodo, c.servidor.URL+caminho, leitor)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if corpo != nil {
		pedido.Header.Set("Content-Type", "application/json")
	}
	if token := c.tokenCSRF(); token != "" {
		pedido.Header.Set("X-CSRF-Token", token)
	}

	resposta, err := c.cliente.Do(pedido)
	if err != nil {
		t.Fatalf("requisição %s %s: %v", metodo, caminho, err)
	}
	t.Cleanup(func() { _ = resposta.Body.Close() })
	return resposta
}

// aquecerCSRF faz um GET para receber o cookie de CSRF, como o frontend faz na
// carga inicial.
func (c *cenario) aquecerCSRF(t *testing.T) {
	t.Helper()
	resposta := c.requisitar(t, http.MethodGet, "/api/auth/me", nil)
	if resposta.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /api/auth/me sem sessão = %d, esperado 401", resposta.StatusCode)
	}
	if c.tokenCSRF() == "" {
		t.Fatal("a resposta deveria ter emitido cookie de CSRF")
	}
}

func (c *cenario) login(t *testing.T, username, senha string) *http.Response {
	t.Helper()
	return c.requisitar(t, http.MethodPost, "/api/auth/login", map[string]string{
		"username": username,
		"password": senha,
	})
}

func lerCorpo(t *testing.T, resposta *http.Response) string {
	t.Helper()
	bruto, err := io.ReadAll(resposta.Body)
	if err != nil {
		t.Fatalf("ler corpo: %v", err)
	}
	return string(bruto)
}

func codigoDeErro(t *testing.T, resposta *http.Response) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(lerCorpo(t, resposta)), &envelope); err != nil {
		t.Fatalf("decodificar erro: %v", err)
	}
	return envelope.Error.Code
}

func TestLoginFluxoCompleto(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})
	criado := cenario.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	cenario.aquecerCSRF(t)

	resposta := cenario.login(t, "admin.teste", senhaTeste)
	if resposta.StatusCode != http.StatusOK {
		t.Fatalf("login = %d, esperado 200", resposta.StatusCode)
	}
	if cenario.cookieSessao() == "" {
		t.Fatal("login deveria gravar cookie de sessão")
	}

	var sessao struct {
		User struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	corpo := lerCorpo(t, resposta)
	if err := json.Unmarshal([]byte(corpo), &sessao); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	if sessao.User.ID != criado.ID.String() || sessao.User.Role != "ADMIN" {
		t.Errorf("usuário devolvido = %+v", sessao.User)
	}

	// Recarregar a aplicação precisa recuperar a sessão pelo cookie.
	respostaMe := cenario.requisitar(t, http.MethodGet, "/api/auth/me", nil)
	if respostaMe.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/auth/me = %d, esperado 200", respostaMe.StatusCode)
	}

	// Logout invalida no servidor.
	respostaLogout := cenario.requisitar(t, http.MethodPost, "/api/auth/logout", nil)
	if respostaLogout.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d, esperado 204", respostaLogout.StatusCode)
	}

	respostaDepois := cenario.requisitar(t, http.MethodGet, "/api/auth/me", nil)
	if respostaDepois.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/auth/me depois do logout = %d, esperado 401", respostaDepois.StatusCode)
	}
}

func TestLoginCookieDeSessaoEHttpOnly(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})
	cenario.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	cenario.aquecerCSRF(t)

	resposta := cenario.login(t, "admin.teste", senhaTeste)
	var sessao *http.Cookie
	for _, cookie := range resposta.Cookies() {
		if cookie.Name == "pacs_session" {
			sessao = cookie
		}
	}
	if sessao == nil {
		t.Fatal("cookie de sessão ausente")
	}
	if !sessao.HttpOnly {
		t.Error("cookie de sessão deve ser HttpOnly")
	}
	if sessao.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, esperado Lax", sessao.SameSite)
	}
	if sessao.Path != "/" {
		t.Errorf("Path = %q, esperado /", sessao.Path)
	}
}

func TestRespostaNaoExpoeDadosSensiveis(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})
	cenario.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	cenario.aquecerCSRF(t)

	for _, rota := range []string{"login", "me"} {
		var corpo string
		if rota == "login" {
			corpo = lerCorpo(t, cenario.login(t, "admin.teste", senhaTeste))
		} else {
			corpo = lerCorpo(t, cenario.requisitar(t, http.MethodGet, "/api/auth/me", nil))
		}

		for _, proibido := range []string{"passwordHash", "password_hash", "PasswordHash", "argon2id", senhaTeste} {
			if strings.Contains(corpo, proibido) {
				t.Errorf("resposta de %s contém %q: %s", rota, proibido, corpo)
			}
		}
	}
}

func TestLoginErros(t *testing.T) {
	ontem := time.Now().AddDate(0, 0, -1)

	casos := []struct {
		nome           string
		username       string
		senha          string
		ajustar        func(*user.User)
		statusEsperado int
		codigoEsperado string
	}{
		{"senha errada", "admin.teste", "senha-de-teste-outra", nil, http.StatusUnauthorized, httpapi.CodeInvalidCredentials},
		{"usuário inexistente", "ninguem.aqui", senhaTeste, nil, http.StatusUnauthorized, httpapi.CodeInvalidCredentials},
		{"conta desativada", "admin.teste", senhaTeste, func(u *user.User) { u.Active = false }, http.StatusForbidden, httpapi.CodeAccountInactive},
		{"validade expirada", "admin.teste", senhaTeste, func(u *user.User) { u.AccessValidUntil = &ontem }, http.StatusForbidden, httpapi.CodeAccountExpired},
		{"sem usuário", "", senhaTeste, nil, http.StatusBadRequest, httpapi.CodeInvalidRequest},
		{"sem senha", "admin.teste", "", nil, http.StatusBadRequest, httpapi.CodeInvalidRequest},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			cenario := montarCenario(t, opcoesCenario{})
			cenario.adicionarUsuario(t, "admin.teste", user.RoleAdmin, caso.ajustar)
			cenario.aquecerCSRF(t)

			resposta := cenario.login(t, caso.username, caso.senha)
			if resposta.StatusCode != caso.statusEsperado {
				t.Fatalf("status = %d, esperado %d", resposta.StatusCode, caso.statusEsperado)
			}
			if codigo := codigoDeErro(t, resposta); codigo != caso.codigoEsperado {
				t.Errorf("código = %q, esperado %q", codigo, caso.codigoEsperado)
			}
			if cenario.cookieSessao() != "" {
				t.Error("login falho não pode gravar cookie de sessão")
			}
		})
	}
}

func TestLoginCorpoInvalido(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})
	cenario.aquecerCSRF(t)

	casos := map[string]string{
		"json quebrado":    `{"username":`,
		"corpo vazio":      ``,
		"campo inesperado": `{"username":"admin.teste","password":"x","role":"ADMIN"}`,
	}

	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			pedido, err := http.NewRequest(http.MethodPost, cenario.servidor.URL+"/api/auth/login", strings.NewReader(corpo))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			pedido.Header.Set("Content-Type", "application/json")
			pedido.Header.Set("X-CSRF-Token", cenario.tokenCSRF())

			resposta, err := cenario.cliente.Do(pedido)
			if err != nil {
				t.Fatalf("requisição: %v", err)
			}
			defer func() { _ = resposta.Body.Close() }()

			if resposta.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, esperado 400", resposta.StatusCode)
			}
		})
	}
}

func TestRotaAdministrativaExigePerfil(t *testing.T) {
	casos := []struct {
		nome           string
		role           user.Role
		statusEsperado int
		codigoEsperado string
	}{
		{"admin entra", user.RoleAdmin, http.StatusNoContent, ""},
		{"gestor é barrado", user.RoleGestor, http.StatusForbidden, httpapi.CodeForbidden},
		{"medico é barrado", user.RoleMedico, http.StatusForbidden, httpapi.CodeForbidden},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			cenario := montarCenario(t, opcoesCenario{})
			cenario.adicionarUsuario(t, "usuario.teste", caso.role, nil)
			cenario.aquecerCSRF(t)

			if resposta := cenario.login(t, "usuario.teste", senhaTeste); resposta.StatusCode != http.StatusOK {
				t.Fatalf("login = %d", resposta.StatusCode)
			}

			resposta := cenario.requisitar(t, http.MethodGet, "/api/admin/ping", nil)
			if resposta.StatusCode != caso.statusEsperado {
				t.Fatalf("status = %d, esperado %d", resposta.StatusCode, caso.statusEsperado)
			}
			if caso.codigoEsperado != "" {
				if codigo := codigoDeErro(t, resposta); codigo != caso.codigoEsperado {
					t.Errorf("código = %q, esperado %q", codigo, caso.codigoEsperado)
				}
			}
		})
	}
}

func TestRotaProtegidaSemSessao(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})

	for _, rota := range []string{"/api/auth/me", "/api/admin/ping"} {
		t.Run(rota, func(t *testing.T) {
			resposta := cenario.requisitar(t, http.MethodGet, rota, nil)
			if resposta.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, esperado 401", resposta.StatusCode)
			}
			if codigo := codigoDeErro(t, resposta); codigo != httpapi.CodeUnauthenticated {
				t.Errorf("código = %q, esperado %q", codigo, httpapi.CodeUnauthenticated)
			}
		})
	}
}

func TestCSRFObrigatorioEmMetodoQueAlteraEstado(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})
	cenario.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	cenario.aquecerCSRF(t)
	if resposta := cenario.login(t, "admin.teste", senhaTeste); resposta.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", resposta.StatusCode)
	}

	t.Run("sem header", func(t *testing.T) {
		pedido, err := http.NewRequest(http.MethodPost, cenario.servidor.URL+"/api/auth/logout", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resposta, err := cenario.cliente.Do(pedido)
		if err != nil {
			t.Fatalf("requisição: %v", err)
		}
		defer func() { _ = resposta.Body.Close() }()

		if resposta.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, esperado 403", resposta.StatusCode)
		}
		if codigo := codigoDeErro(t, resposta); codigo != httpapi.CodeCSRFInvalid {
			t.Errorf("código = %q, esperado %q", codigo, httpapi.CodeCSRFInvalid)
		}
	})

	t.Run("header divergente", func(t *testing.T) {
		pedido, err := http.NewRequest(http.MethodPost, cenario.servidor.URL+"/api/auth/logout", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		pedido.Header.Set("X-CSRF-Token", "valor-que-nao-corresponde")
		resposta, err := cenario.cliente.Do(pedido)
		if err != nil {
			t.Fatalf("requisição: %v", err)
		}
		defer func() { _ = resposta.Body.Close() }()

		if resposta.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, esperado 403", resposta.StatusCode)
		}
	})

	t.Run("origem de outro site", func(t *testing.T) {
		pedido, err := http.NewRequest(http.MethodPost, cenario.servidor.URL+"/api/auth/logout", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		pedido.Header.Set("X-CSRF-Token", cenario.tokenCSRF())
		pedido.Header.Set("Origin", "https://site-malicioso.invalid")
		resposta, err := cenario.cliente.Do(pedido)
		if err != nil {
			t.Fatalf("requisição: %v", err)
		}
		defer func() { _ = resposta.Body.Close() }()

		if resposta.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, esperado 403", resposta.StatusCode)
		}
	})
}

func TestRateLimitNoLogin(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{tentativasLogin: 3})
	cenario.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	cenario.aquecerCSRF(t)

	for i := 1; i <= 3; i++ {
		resposta := cenario.login(t, "admin.teste", "senha-de-teste-outra")
		if resposta.StatusCode != http.StatusUnauthorized {
			t.Fatalf("tentativa %d = %d, esperado 401", i, resposta.StatusCode)
		}
	}

	resposta := cenario.login(t, "admin.teste", senhaTeste)
	if resposta.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("quarta tentativa = %d, esperado 429", resposta.StatusCode)
	}
	if codigo := codigoDeErro(t, resposta); codigo != httpapi.CodeRateLimited {
		t.Errorf("código = %q, esperado %q", codigo, httpapi.CodeRateLimited)
	}
	if resposta.Header.Get("Retry-After") == "" {
		t.Error("resposta 429 deveria trazer Retry-After")
	}
}

func TestLoginBemSucedidoLiberaContagem(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{tentativasLogin: 3})
	cenario.adicionarUsuario(t, "admin.teste", user.RoleAdmin, nil)
	cenario.aquecerCSRF(t)

	for i := 0; i < 2; i++ {
		if resposta := cenario.login(t, "admin.teste", "senha-de-teste-outra"); resposta.StatusCode != http.StatusUnauthorized {
			t.Fatalf("tentativa %d = %d", i, resposta.StatusCode)
		}
	}
	if resposta := cenario.login(t, "admin.teste", senhaTeste); resposta.StatusCode != http.StatusOK {
		t.Fatalf("acerto = %d, esperado 200", resposta.StatusCode)
	}

	// Depois do acerto a contagem zera e ainda há tentativas disponíveis.
	for i := 0; i < 3; i++ {
		if resposta := cenario.login(t, "admin.teste", "senha-de-teste-outra"); resposta.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("tentativa %d bloqueada: a contagem deveria ter sido liberada", i)
		}
	}
}

func TestHealthEReady(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})

	for _, rota := range []string{"/health", "/health/ready"} {
		resposta := cenario.requisitar(t, http.MethodGet, rota, nil)
		if resposta.StatusCode != http.StatusOK {
			t.Errorf("%s = %d, esperado 200", rota, resposta.StatusCode)
		}
		corpo := lerCorpo(t, resposta)
		if !strings.Contains(corpo, `"status":"ok"`) {
			t.Errorf("%s corpo = %s", rota, corpo)
		}
		for _, proibido := range []string{"postgres", "DATABASE_URL", "version", "host"} {
			if strings.Contains(strings.ToLower(corpo), strings.ToLower(proibido)) {
				t.Errorf("%s expõe %q: %s", rota, proibido, corpo)
			}
		}
	}
}

func TestRotaDesconhecidaDaAPI(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})
	resposta := cenario.requisitar(t, http.MethodGet, "/api/nao/existe", nil)
	if resposta.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", resposta.StatusCode)
	}
	if codigo := codigoDeErro(t, resposta); codigo != httpapi.CodeNotFound {
		t.Errorf("código = %q, esperado %q", codigo, httpapi.CodeNotFound)
	}
}

func TestCabecalhosDeSeguranca(t *testing.T) {
	cenario := montarCenario(t, opcoesCenario{})
	resposta := cenario.requisitar(t, http.MethodGet, "/api/auth/me", nil)

	esperados := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Cache-Control":          "no-store",
	}
	for nome, valor := range esperados {
		if got := resposta.Header.Get(nome); got != valor {
			t.Errorf("%s = %q, esperado %q", nome, got, valor)
		}
	}
}
