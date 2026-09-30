package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/auth"
	"github.com/pmfb-saude/pacs-web/backend/internal/config"
	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/studies"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
	"github.com/pmfb-saude/pacs-web/backend/internal/viewer"
)

// Pinger é o que a checagem de prontidão precisa do banco.
type Pinger interface {
	Ping(ctx context.Context) error
}

type OrthancTester interface {
	TestSystem(context.Context, orthanc.Config) error
}

type StudyFinder interface {
	FindStudies(context.Context, orthanc.Config, studies.Query) (studies.Page, error)
}

type ViewerClient interface {
	ViewerSeries(context.Context, orthanc.Config, string) ([]viewer.Series, error)
	ViewerInstances(context.Context, orthanc.Config, string, string) ([]viewer.Instance, error)
	OpenDICOM(context.Context, orthanc.Config, string, string, string) (viewer.DICOM, error)
}

// Deps são as dependências do servidor HTTP.
type Deps struct {
	Config config.Config
	Log    *slog.Logger
	Auth   *auth.Service
	// DB é opcional; sem ele /health/ready não verifica o banco.
	DB Pinger
	// Settings é a configuração operacional administrável (PACS/Orthanc).
	Settings SettingsStore
	Orthanc  OrthancTester
	Studies  StudyFinder
	Viewer   ViewerClient
	Units    UnitsStore
	Users    UsersService
	// Auditoria registra eventos administrativos. Opcional.
	Auditoria   AuditRecorder
	AuditReader AuditReader
	// Now é opcional; o padrão é time.Now.
	Now func() time.Time
}

// Server monta e serve a API.
type Server struct {
	cfg         config.Config
	log         *slog.Logger
	auth        *auth.Service
	db          Pinger
	settings    SettingsStore
	orthanc     OrthancTester
	studies     StudyFinder
	viewer      ViewerClient
	units       UnitsStore
	users       UsersService
	auditoria   AuditRecorder
	auditReader AuditReader
	now         func() time.Time

	loginLimiter *rateLimiter
	handler      http.Handler
}

// New monta o servidor com as rotas e middlewares.
func New(deps Deps) (*Server, error) {
	if deps.Log == nil {
		return nil, errors.New("logger é obrigatório")
	}
	if deps.Auth == nil {
		return nil, errors.New("serviço de autenticação é obrigatório")
	}
	agora := deps.Now
	if agora == nil {
		agora = time.Now
	}

	if deps.Settings == nil {
		return nil, errors.New("store de configuração é obrigatório")
	}

	s := &Server{
		cfg:         deps.Config,
		log:         deps.Log,
		auth:        deps.Auth,
		db:          deps.DB,
		settings:    deps.Settings,
		orthanc:     deps.Orthanc,
		studies:     deps.Studies,
		viewer:      deps.Viewer,
		units:       deps.Units,
		users:       deps.Users,
		auditoria:   deps.Auditoria,
		auditReader: deps.AuditReader,
		now:         agora,
		loginLimiter: newRateLimiter(
			deps.Config.LoginRateAttempts,
			deps.Config.LoginRateWindow,
			agora,
		),
	}
	s.handler = s.montarRotas()
	return s, nil
}

// Handler devolve o handler HTTP pronto para uso.
func (s *Server) Handler() http.Handler { return s.handler }

// VacuumRateLimiter descarta janelas vencidas do limitador.
func (s *Server) VacuumRateLimiter() { s.loginLimiter.Vacuum() }

func (s *Server) montarRotas() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /api/viewer/exports", encadear(http.HandlerFunc(s.handleViewerExport), s.requireSession, s.requireRole(user.RoleAdmin, user.RoleGestor, user.RoleMedico)))

	// Autenticação.
	mux.Handle("POST /api/auth/login", http.HandlerFunc(s.handleLogin))
	mux.Handle("POST /api/auth/logout", encadear(http.HandlerFunc(s.handleLogout), s.requireSession))
	mux.Handle("GET /api/auth/me", encadear(http.HandlerFunc(s.handleMe), s.requireSession))
	mux.Handle("GET /api/studies", encadear(http.HandlerFunc(s.handleStudies),
		s.requireSession, s.requireRole(user.RoleAdmin, user.RoleGestor, user.RoleMedico)))
	for route, handler := range map[string]http.HandlerFunc{
		"GET /api/studies/{studyID}/series":                                         s.handleViewerSeries,
		"GET /api/studies/{studyID}/series/{seriesID}/instances":                    s.handleViewerInstances,
		"GET /api/studies/{studyID}/series/{seriesID}/instances/{instanceID}/dicom": s.handleViewerDICOM,
	} {
		mux.Handle(route, encadear(handler, s.requireSession, s.requireRole(user.RoleAdmin, user.RoleGestor, user.RoleMedico)))
	}

	mux.Handle("GET /api/units", encadear(http.HandlerFunc(s.handleListUnits), s.requireSession, s.requireRole(user.RoleAdmin, user.RoleGestor, user.RoleMedico)))
	mux.Handle("POST /api/admin/units", encadear(http.HandlerFunc(s.handleCreateUnit), s.requireSession, s.requireRole(user.RoleAdmin)))
	mux.Handle("PATCH /api/admin/units/{id}", encadear(http.HandlerFunc(s.handlePatchUnit), s.requireSession, s.requireRole(user.RoleAdmin)))

	mux.Handle("POST /api/auth/change-password", encadear(http.HandlerFunc(s.handleChangePassword), s.requireSession))
	mux.Handle("GET /api/admin/users", encadear(http.HandlerFunc(s.handleUsers), s.requireSession, s.requireRole(user.RoleAdmin, user.RoleGestor)))
	for route, handler := range map[string]http.HandlerFunc{
		"POST /api/admin/users/medicos":             s.handleUserAction("create", user.RoleMedico),
		"POST /api/admin/users/gestores":            s.handleUserAction("create", user.RoleGestor),
		"PATCH /api/admin/users/{id}":               s.handleUserAction("edit", ""),
		"POST /api/admin/users/{id}/active":         s.handleUserAction("active", ""),
		"POST /api/admin/users/{id}/renew":          s.handleUserAction("renew", ""),
		"POST /api/admin/users/{id}/reset-password": s.handleUserAction("reset", ""),
	} {
		mux.Handle(route, encadear(handler, s.requireSession, s.requireRole(user.RoleAdmin, user.RoleGestor)))
	}

	mux.Handle("GET /api/admin/audit", encadear(http.HandlerFunc(s.handleAudit), s.requireSession, s.requireRole(user.RoleAdmin)))

	// Rota administrativa mínima, só para comprovar a autorização por perfil.
	// Será substituída pelos endpoints reais de administração.
	mux.Handle("GET /api/admin/ping", encadear(
		http.HandlerFunc(s.handleAdminPing),
		s.requireSession,
		s.requireRole(user.RoleAdmin),
	))

	// Configuração do PACS: só ADMIN lê e só ADMIN grava.
	mux.Handle("GET /api/admin/settings/orthanc", encadear(
		http.HandlerFunc(s.handleGetOrthancSettings),
		s.requireSession,
		s.requireRole(user.RoleAdmin),
	))
	mux.Handle("PUT /api/admin/settings/orthanc", encadear(
		http.HandlerFunc(s.handlePutOrthancSettings),
		s.requireSession,
		s.requireRole(user.RoleAdmin),
	))

	mux.Handle("POST /api/admin/settings/orthanc/test", encadear(
		http.HandlerFunc(s.handleTestOrthanc), s.requireSession, s.requireRole(user.RoleAdmin),
	))

	// Saúde da NOSSA aplicação. Nada aqui diz respeito ao Orthanc.
	mux.Handle("GET /health", http.HandlerFunc(s.handleHealth))
	mux.Handle("GET /health/ready", http.HandlerFunc(s.handleReady))

	// Qualquer outra rota sob /api responde 404 em JSON.
	mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, s.log, http.StatusNotFound, CodeNotFound, "Recurso não encontrado.")
	}))

	if s.cfg.StaticDir != "" {
		mux.Handle("/", s.handlerFrontend(s.cfg.StaticDir))
	}

	return encadear(mux,
		securityHeaders,
		s.requestLogger,
		s.recoverPanic,
		s.cors,
		s.csrfIssue,
		s.csrfProtect,
	)
}

// handlerFrontend serve o build do frontend na mesma origem da API. Caminhos
// que não existem em disco caem no index.html, porque a navegação é do SPA.
func (s *Server) handlerFrontend(raiz string) http.Handler {
	arquivos := http.FileServer(http.Dir(raiz))
	index := filepath.Join(raiz, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limpo := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if limpo == "." || limpo == "/" {
			http.ServeFile(w, r, index)
			return
		}
		if info, err := os.Stat(filepath.Join(raiz, limpo)); err == nil && !info.IsDir() {
			arquivos.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, index)
	})
}
