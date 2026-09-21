package server

import (
	"log/slog"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsappconverty/internal/auth"
	"whatsappconverty/internal/config"
	"whatsappconverty/internal/converty"
	"whatsappconverty/internal/dashboard"
	"whatsappconverty/internal/shops"
)

// App wires the dependencies for the web/API process and owns the HTTP router.
type App struct {
	Cfg       config.Config
	Log       *slog.Logger
	Pool      *pgxpool.Pool
	Auth      *auth.Service
	Shops     *shops.Repository
	Dashboard *dashboard.Repository
	Converty  *converty.Service
}

func New(cfg config.Config, log *slog.Logger, pool *pgxpool.Pool) *App {
	shopsRepo := shops.NewRepository(pool)
	return &App{
		Cfg:       cfg,
		Log:       log,
		Pool:      pool,
		Auth:      auth.NewService(pool, *shopsRepo),
		Shops:     shopsRepo,
		Dashboard: dashboard.NewRepository(pool),
		Converty:  converty.NewService(cfg, pool, log),
	}
}

func (a *App) InitializeMiddleware(r *chi.Mux) {
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(a.loggingMiddleware)

	if a.Cfg.IsProduction() {
		r.Use(middleware.Compress(5))
	}
}

func (a *App) InitializeRoutes(r *chi.Mux) {
	authConfig := kit.AuthenticationConfig{
		AuthFunc:    a.Auth.AuthenticateUser,
		RedirectURL: "/login",
	}

	// Public health and readiness.
	r.Get("/healthz", kit.Handler(a.handleLiveness))
	r.Get("/readyz", kit.Handler(a.handleReadiness))

	// Optional authentication: unauthenticated visitors can reach these.
	r.Group(func(pr chi.Router) {
		pr.Use(kit.WithAuthentication(authConfig, false))
		pr.Get("/", kit.Handler(a.handleIndex))
		pr.Get("/login", kit.Handler(a.handleLoginGet))
		pr.Post("/login", kit.Handler(a.handleLoginPost))
		pr.Get("/register", kit.Handler(a.handleRegisterGet))
		pr.Post("/register", kit.Handler(a.handleRegisterPost))
		pr.Post("/logout", kit.Handler(a.handleLogout))
		pr.Post("/webhooks/converty", kit.Handler(a.handleConvertyWebhook))
	})

	// Required authentication for the merchant dashboard.
	r.Group(func(pr chi.Router) {
		pr.Use(kit.WithAuthentication(authConfig, true))
		pr.Get("/dashboard", kit.Handler(a.handleOverview))
		pr.Get("/auth/converty/connect", kit.Handler(a.handleConvertyConnect))
		pr.Get("/auth/converty/callback", kit.Handler(a.handleConvertyCallback))
		pr.Get("/automations", kit.Handler(a.handlePlaceholder("automations")))
		pr.Get("/templates", kit.Handler(a.handlePlaceholder("templates")))
		pr.Get("/messages", kit.Handler(a.handlePlaceholder("messages")))
		pr.Get("/settings", kit.Handler(a.handlePlaceholder("settings")))
	})

	r.NotFound(kit.Handler(a.handleNotFound))
}
