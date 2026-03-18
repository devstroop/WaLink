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
	inner.HandleFunc("GET /about", h.AboutPage)
	inner.HandleFunc("GET /pricing", h.PricingPage)
	inner.HandleFunc("GET /terms", h.TermsPage)
	inner.HandleFunc("GET /privacy", h.PrivacyPage)
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
	inner.HandleFunc("POST /accounts/{id}/update", h.AccountUpdate)
	inner.HandleFunc("POST /accounts/{id}/delete", h.AccountDeletePost)
	inner.HandleFunc("DELETE /accounts/{id}", h.AccountDelete)

	// Account webhook + proxy (JSON)
	inner.HandleFunc("PUT /accounts/{id}/webhook", h.AccountWebhookSet)
	inner.HandleFunc("DELETE /accounts/{id}/webhook", h.AccountWebhookDelete)
	inner.HandleFunc("PUT /accounts/{id}/proxy", h.AccountProxySet)
	inner.HandleFunc("DELETE /accounts/{id}/proxy", h.AccountProxyDelete)

	// Account partials (htmx)
	inner.HandleFunc("GET /accounts/{id}/session-status", h.AccountSessionStatus)
	inner.HandleFunc("GET /accounts/{id}/session-tab", h.AccountSessionTab)
	inner.HandleFunc("GET /accounts/{id}/qr", h.AccountQR)
	inner.HandleFunc("POST /accounts/{id}/pair", h.AccountPair)
	inner.HandleFunc("POST /accounts/{id}/disconnect", h.AccountDisconnect)

	// Messaging
	inner.HandleFunc("GET /messaging", h.Messaging)
	inner.HandleFunc("POST /messaging/{id}/send", h.MessageSend)
	inner.HandleFunc("GET /messaging/{id}/chats", h.MessagingChats)
	inner.HandleFunc("GET /messaging/{id}/messages", h.MessagingMessages)
	inner.HandleFunc("GET /messaging/{id}/contacts", h.MessagingContacts)
	inner.HandleFunc("GET /messaging/{id}/groups", h.MessagingGroups)
	inner.HandleFunc("GET /messaging/{id}/newsletters", h.MessagingNewsletters)
	inner.HandleFunc("GET /messaging/{id}/newsletter-messages", h.MessagingNewsletterMessages)
	inner.HandleFunc("POST /messaging/{id}/newsletter-follow", h.MessagingNewsletterFollow)
	inner.HandleFunc("POST /messaging/{id}/newsletter-unfollow", h.MessagingNewsletterUnfollow)
	inner.HandleFunc("POST /messaging/{id}/react", h.MessagingReact)
	inner.HandleFunc("POST /messaging/{id}/mark-read", h.MessagingMarkRead)
	inner.HandleFunc("POST /messaging/{id}/revoke", h.MessagingRevoke)

	// Admin
	inner.HandleFunc("GET /admin/users", h.UsersList)
	inner.HandleFunc("POST /admin/users", h.UsersCreate)
	inner.HandleFunc("POST /admin/users/{id}/update", h.UsersUpdate)
	inner.HandleFunc("POST /admin/users/{id}/delete", h.UsersDelete)
	inner.HandleFunc("POST /admin/users/{id}/reset-password", h.UsersResetPassword)
	inner.HandleFunc("GET /admin/roles", h.RolesList)
	inner.HandleFunc("POST /admin/roles", h.RolesCreate)
	inner.HandleFunc("POST /admin/roles/{id}/update", h.RolesUpdate)
	inner.HandleFunc("POST /admin/roles/{id}/delete", h.RolesDelete)
	// API Keys & MCP (all authenticated users)
	inner.HandleFunc("GET /api-keys", h.APIKeysList)
	inner.HandleFunc("POST /api-keys", h.APIKeysCreate)
	inner.HandleFunc("POST /api-keys/{id}/delete", h.APIKeysDelete)
	inner.HandleFunc("GET /mcp-server", h.MCPSettings)
	inner.HandleFunc("POST /mcp-server", h.MCPSettingsUpdate)

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
