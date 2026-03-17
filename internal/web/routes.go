package web

import (
	"io/fs"
	"net/http"

	"github.com/devstroop/walink/internal/database"
	"github.com/devstroop/walink/internal/service"
	smtpclient "github.com/devstroop/walink/internal/smtp"
)

// Handler serves the web UI.
type Handler struct {
	mgr        *service.AccountManager
	db         *database.DB
	render     *Renderer
	secret     string
	version    string
	regEnabled bool
	smtp       *smtpclient.Client
}

// New creates a new web UI handler.
func New(mgr *service.AccountManager, db *database.DB, secret, version string, regEnabled bool, mailer *smtpclient.Client) *Handler {
	return &Handler{
		mgr:        mgr,
		db:         db,
		render:     NewRenderer(),
		secret:     secret,
		version:    version,
		regEnabled: regEnabled,
		smtp:       mailer,
	}
}

// RegisterRoutes mounts all web UI routes on the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// ── Static assets (served directly, no auth) ────
	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	// Favicon shortcut
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/favicon.svg", http.StatusMovedPermanently)
	})

	// ── All web pages go through a single inner mux ─
	inner := http.NewServeMux()

	// Public pages
	inner.HandleFunc("GET /login", h.LoginPage)
	inner.HandleFunc("POST /login", h.LoginSubmit)
	inner.HandleFunc("POST /logout", h.Logout)
	inner.HandleFunc("GET /forgot-password", h.ForgotPasswordPage)
	inner.HandleFunc("POST /forgot-password", h.ForgotPasswordSubmit)
	inner.HandleFunc("GET /reset-password", h.ResetPasswordPage)
	inner.HandleFunc("POST /reset-password", h.ResetPasswordSubmit)
	if h.regEnabled {
		inner.HandleFunc("GET /register", h.RegisterPage)
		inner.HandleFunc("POST /register", h.RegisterSubmit)
	}

	// Dashboard
	inner.HandleFunc("GET /dashboard", h.Dashboard)

	// Accounts
	inner.HandleFunc("GET /accounts", h.AccountsList)
	inner.HandleFunc("POST /accounts", h.AccountsCreate)
	inner.HandleFunc("GET /accounts/{id}", h.AccountDetail)
	inner.HandleFunc("DELETE /accounts/{id}", h.AccountDelete)

	// Account partials (htmx)
	inner.HandleFunc("GET /accounts/{id}/session-status", h.AccountSessionStatus)
	inner.HandleFunc("GET /accounts/{id}/qr", h.AccountQR)
	inner.HandleFunc("POST /accounts/{id}/pair", h.AccountPair)
	inner.HandleFunc("POST /accounts/{id}/disconnect", h.AccountDisconnect)

	// Admin
	inner.HandleFunc("GET /admin/users", h.UsersList)
	inner.HandleFunc("GET /admin/roles", h.RolesList)
	inner.HandleFunc("GET /admin/api-keys", h.APIKeysList)

	// Settings
	inner.HandleFunc("GET /settings", h.Settings)
	inner.HandleFunc("POST /settings/password", h.ChangePassword)

	// Root redirect
	inner.HandleFunc("GET /{$}", h.Root)

	// Catch-all 404 for unknown web paths
	inner.HandleFunc("/", h.NotFound)

	// Wrap with cookie auth (skips public paths)
	mux.Handle("/", WebAuth(h.secret, h.db, h.regEnabled, inner))
}

// page builds a PageData with common fields filled in.
func (h *Handler) page(w http.ResponseWriter, r *http.Request, title, activePage string, data any) PageData {
	return PageData{
		Title:    title + " — WaLink",
		Heading:  title,
		Page:     activePage,
		Version:  h.version,
		Identity: getIdentity(r),
		Flash:    getFlash(w, r),
		Data:     data,
	}
}
