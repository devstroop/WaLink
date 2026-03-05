package handler

import (
	"net/http"

	"github.com/devstroop/walink/internal/service"
)

// API groups all route handlers.
type API struct {
	mgr *service.AccountManager
}

// NewAPI creates a new API handler group.
func NewAPI(mgr *service.AccountManager) *API {
	return &API{mgr: mgr}
}

// RegisterRoutes wires every endpoint into the mux.
// All paths are under /api/v1/accounts.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	base := "/api/v1/accounts"
	acct := base + "/{account_id}"

	// ── Account CRUD ────────────────────────────────
	mux.HandleFunc("GET "+base, a.ListAccounts)
	mux.HandleFunc("POST "+base, a.CreateAccount)
	mux.HandleFunc("GET "+acct, a.GetAccount)
	mux.HandleFunc("PATCH "+acct, a.UpdateAccount)
	mux.HandleFunc("DELETE "+acct, a.DeleteAccount)

	// ── Session (auth/linking lifecycle) ────────────
	mux.HandleFunc("GET "+acct+"/session", a.GetSession)
	mux.HandleFunc("GET "+acct+"/session/qr", a.GetQR)
	mux.HandleFunc("POST "+acct+"/session/pair", a.PairPhone)
	mux.HandleFunc("DELETE "+acct+"/session", a.DeleteSession)

	// ── Proxy ───────────────────────────────────────
	mux.HandleFunc("GET "+acct+"/proxy", a.GetProxy)
	mux.HandleFunc("PUT "+acct+"/proxy", a.SetProxy)
	mux.HandleFunc("DELETE "+acct+"/proxy", a.DeleteProxy)

	// ── Messaging ───────────────────────────────────
	mux.HandleFunc("GET "+acct+"/messages", a.GetMessages)
	mux.HandleFunc("POST "+acct+"/messages", a.SendMessage)
	mux.HandleFunc("POST "+acct+"/messages/react", a.ReactMessage)
	mux.HandleFunc("POST "+acct+"/messages/read", a.MarkRead)
	mux.HandleFunc("DELETE "+acct+"/messages/{message_id}", a.RevokeMessage)

	// ── Webhook ─────────────────────────────────────
	mux.HandleFunc("GET "+acct+"/webhook", a.GetWebhook)
	mux.HandleFunc("PUT "+acct+"/webhook", a.SetWebhook)
	mux.HandleFunc("DELETE "+acct+"/webhook", a.DeleteWebhook)

	// ── Chats ───────────────────────────────────────
	mux.HandleFunc("GET "+acct+"/chats", a.ListChats)


	// ── Contacts ────────────────────────────────────
	mux.HandleFunc("GET "+acct+"/contacts", a.ListContacts)
	mux.HandleFunc("POST "+acct+"/contacts/check", a.CheckContacts)
	mux.HandleFunc("GET "+acct+"/contacts/{jid}", a.GetContact)

	// ── Groups ──────────────────────────────────────
	mux.HandleFunc("GET "+acct+"/groups", a.ListGroups)
	mux.HandleFunc("POST "+acct+"/groups", a.CreateGroup)
	mux.HandleFunc("GET "+acct+"/groups/{jid}", a.GetGroup)
	mux.HandleFunc("PATCH "+acct+"/groups/{jid}", a.UpdateGroup)
	mux.HandleFunc("DELETE "+acct+"/groups/{jid}", a.LeaveGroup)
	mux.HandleFunc("GET "+acct+"/groups/{jid}/invite", a.GetGroupInvite)
	mux.HandleFunc("POST "+acct+"/groups/{jid}/participants", a.UpdateGroupParticipants)

	// ── Presence ────────────────────────────────────
	mux.HandleFunc("POST "+acct+"/presence", a.SendPresence)

	// ── Profile ─────────────────────────────────────
	mux.HandleFunc("GET "+acct+"/profile", a.GetProfile)
	mux.HandleFunc("PATCH "+acct+"/profile", a.UpdateProfile)
}
