package httpapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

type chaveContexto int

const (
	chaveUsuario chaveContexto = iota
	chaveSessao
	chaveTokenSessao
)

// UsuarioDoContexto devolve o usuário autenticado da requisição.
func UsuarioDoContexto(ctx context.Context) (user.User, bool) {
	usuario, ok := ctx.Value(chaveUsuario).(user.User)
	return usuario, ok
}

func sessaoDoContexto(ctx context.Context) (auth.Session, bool) {
	sessao, ok := ctx.Value(chaveSessao).(auth.Session)
	return sessao, ok
}

func tokenDoContexto(ctx context.Context) string {
	token, _ := ctx.Value(chaveTokenSessao).(string)
	return token
}

// securityHeaders aplica cabeçalhos de segurança às respostas da API.
func securityHeaders(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cabecalho := w.Header()
		cabecalho.Set("X-Content-Type-Options", "nosniff")
		cabecalho.Set("Referrer-Policy", "same-origin")
		cabecalho.Set("X-Frame-Options", "DENY")
		// Respostas da API não devem ficar em cache compartilhado.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			cabecalho.Set("Cache-Control", "no-store")
		}
		proximo.ServeHTTP(w, r)
	})
}

// requestLogger registra método, rota, status e duração. Não registra corpo,
// cookie, header de autorização nem query string.
func (s *Server) requestLogger(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := s.now()
		gravador := &gravadorStatus{ResponseWriter: w, status: http.StatusOK}
		proximo.ServeHTTP(gravador, r)
		s.log.InfoContext(r.Context(), "requisição",
			"metodo", r.Method,
			"rota", requestLogRoute(r),
			"status", gravador.status,
			"duracao_ms", s.now().Sub(inicio).Milliseconds(),
			"ip", ipDoPedido(r),
		)
	})
}

type gravadorStatus struct {
	http.ResponseWriter
	status   int
	escreveu bool
}

// Unwrap permite ajustar o prazo de escrita nas consultas ao Orthanc,
// cujo timeout configurável pode exceder o padrão HTTP de 30 segundos.
func (g *gravadorStatus) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gravadorStatus) WriteHeader(status int) {
	if g.escreveu {
		return
	}
	g.status = status
	g.escreveu = true
	g.ResponseWriter.WriteHeader(status)
}

func (g *gravadorStatus) Write(dados []byte) (int, error) {
	g.escreveu = true
	return g.ResponseWriter.Write(dados)
}

// recoverPanic transforma pânico em 500 sanitizado, sem derrubar o servidor.
func (s *Server) recoverPanic(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recuperado := recover(); recuperado != nil {
				if recuperado == http.ErrAbortHandler {
					panic(recuperado)
				}
				s.log.ErrorContext(r.Context(), "pânico no handler",
					"metodo", r.Method, "rota", requestLogRoute(r), "panico", recuperado)
				writeError(w, s.log, http.StatusInternalServerError, CodeInternal,
					"Erro interno. Tente novamente; se persistir, contate o suporte de TI.")
			}
		}()
		proximo.ServeHTTP(w, r)
	})
}

// cors libera apenas as origens configuradas explicitamente. Sem configuração,
// nada é liberado: em produção o arranjo previsto é mesma origem, e em
// desenvolvimento o proxy do Vite mantém tudo em localhost:5173.
func (s *Server) cors(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origem := r.Header.Get("Origin")
		if origem != "" && slices.Contains(s.cfg.AllowedOrigins, origem) {
			cabecalho := w.Header()
			cabecalho.Set("Access-Control-Allow-Origin", origem)
			cabecalho.Set("Access-Control-Allow-Credentials", "true")
			cabecalho.Set("Access-Control-Allow-Headers", "Content-Type, "+headerCSRF)
			cabecalho.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			cabecalho.Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		proximo.ServeHTTP(w, r)
	})
}

// csrfIssue garante que toda resposta da API leve um cookie de CSRF, para que o
// frontend tenha o que reenviar no header.
func (s *Server) csrfIssue(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if lerCookie(r, cookieCSRF) == "" {
			token, err := novoTokenCSRF()
			if err != nil {
				writeInternalError(w, s.log, r, err)
				return
			}
			s.setCSRFCookie(w, token)
		}
		proximo.ServeHTTP(w, r)
	})
}

// métodos seguros não alteram estado e não exigem CSRF.
var metodosSeguros = []string{http.MethodGet, http.MethodHead, http.MethodOptions}

// csrfProtect exige, em todo método que altera estado, que o header
// X-CSRF-Token seja igual ao cookie pacs_csrf (double submit), e que a origem
// da requisição seja a própria aplicação.
//
// Camadas somadas: cookie SameSite=Lax, conferência de Origin/Referer e o par
// cookie+header. Um site terceiro não consegue ler o cookie para montar o
// header, então não consegue forjar a requisição.
func (s *Server) csrfProtect(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slices.Contains(metodosSeguros, r.Method) {
			proximo.ServeHTTP(w, r)
			return
		}

		if !s.origemPermitida(r) {
			writeError(w, s.log, http.StatusForbidden, CodeCSRFInvalid, "Origem da requisição não permitida.")
			return
		}

		doCookie := lerCookie(r, cookieCSRF)
		doHeader := r.Header.Get(headerCSRF)
		if doCookie == "" || doHeader == "" || subtle.ConstantTimeCompare([]byte(doCookie), []byte(doHeader)) != 1 {
			writeError(w, s.log, http.StatusForbidden, CodeCSRFInvalid,
				"Token de segurança ausente ou inválido. Recarregue a página e tente novamente.")
			return
		}

		proximo.ServeHTTP(w, r)
	})
}

// origemPermitida confere Origin (ou Referer, quando Origin falta) contra o
// próprio host e contra as origens configuradas.
func (s *Server) origemPermitida(r *http.Request) bool {
	bruta := r.Header.Get("Origin")
	if bruta == "" {
		bruta = r.Header.Get("Referer")
	}
	if bruta == "" {
		// Sem Origin nem Referer não há requisição cross-site iniciada por
		// navegador; as demais camadas seguem valendo.
		return true
	}
	endereco, err := url.Parse(bruta)
	if err != nil || endereco.Host == "" {
		return false
	}
	if endereco.Host == r.Host {
		return true
	}
	return slices.ContainsFunc(s.cfg.AllowedOrigins, func(permitida string) bool {
		outra, err := url.Parse(permitida)
		return err == nil && outra.Host == endereco.Host && outra.Scheme == endereco.Scheme
	})
}

// requireSession exige sessão válida. Sessão ausente, expirada ou revogada
// resulta em 401 e limpeza do cookie.
func (s *Server) requireSession(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := lerCookie(r, cookieSession)
		sessao, usuario, err := s.auth.Resolve(r.Context(), token)
		if errors.Is(err, auth.ErrSessaoInvalida) {
			s.clearSessionCookie(w)
			writeError(w, s.log, http.StatusUnauthorized, CodeUnauthenticated, "Sessão inválida ou expirada.")
			return
		}
		if err != nil {
			writeInternalError(w, s.log, r, err)
			return
		}

		ctx := context.WithValue(r.Context(), chaveUsuario, usuario)
		ctx = context.WithValue(ctx, chaveSessao, sessao)
		ctx = context.WithValue(ctx, chaveTokenSessao, token)
		proximo.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireRole exige um dos perfis informados. Aplica-se depois de
// requireSession: a autorização é sempre do servidor.
func (s *Server) requireRole(perfis ...user.Role) func(http.Handler) http.Handler {
	return func(proximo http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			usuario, ok := UsuarioDoContexto(r.Context())
			if !ok {
				writeError(w, s.log, http.StatusUnauthorized, CodeUnauthenticated, "Sessão inválida ou expirada.")
				return
			}
			if !slices.Contains(perfis, usuario.Role) {
				s.log.WarnContext(r.Context(), "acesso negado por perfil",
					"rota", requestLogRoute(r), "perfil", usuario.Role)
				writeError(w, s.log, http.StatusForbidden, CodeForbidden,
					"Seu perfil não tem permissão para esta operação.")
				return
			}
			proximo.ServeHTTP(w, r)
		})
	}
}

// encadear aplica os middlewares na ordem informada, o primeiro por fora.
func encadear(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// ipDoPedido devolve o IP do cliente. Não confia em X-Forwarded-For por
// padrão: atrás de proxy, o endereço real precisa ser repassado pelo próprio
// proxy na conexão (ou esta função precisará de configuração explícita).
func ipDoPedido(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// prazoDeSessao devolve o instante de expiração absoluta para o cookie.
func (s *Server) prazoDeSessao(agora time.Time) time.Time {
	return agora.Add(s.auth.AbsoluteTTL())
}

// Identificadores de estudo/série/instância não entram nos logs de requisição.
func requestLogRoute(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/api/studies/") {
		return "/api/studies/{viewer-resource}"
	}
	if strings.HasPrefix(r.URL.Path, "/viewer/") {
		return "/viewer/{studyID}"
	}
	return r.URL.Path
}
