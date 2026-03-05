package handler

import (
	"net/http"

	"github.com/itsalfredakku/walink/internal/service"
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

	// Account CRUD
	mux.HandleFunc("GET "+base, a.ListAccounts)
	mux.HandleFunc("POST "+base, a.CreateAccount)
	mux.HandleFunc("GET "+base+"/{account_id}", a.GetAccount)
	mux.HandleFunc("DELETE "+base+"/{account_id}", a.DeleteAccount)

	// Account config
	mux.HandleFunc("GET "+base+"/{account_id}/config", a.GetConfig)
	mux.HandleFunc("PUT "+base+"/{account_id}/config", a.UpdateConfig)

	// WhatsApp auth & linking
	mux.HandleFunc("GET "+base+"/{account_id}/status", a.GetStatus)
	mux.HandleFunc("GET "+base+"/{account_id}/link/qr", a.GetQR)
	mux.HandleFunc("POST "+base+"/{account_id}/link/phone", a.LinkPhone)
	mux.HandleFunc("DELETE "+base+"/{account_id}/unlink", a.Unlink)

	// Chats & messages
	mux.HandleFunc("GET "+base+"/{account_id}/chats", a.ListChats)
	mux.HandleFunc("GET "+base+"/{account_id}/chats/{chat_id}/messages", a.GetMessages)
	mux.HandleFunc("POST "+base+"/{account_id}/chats/{chat_id}/messages", a.SendMessage)

	// Chat actions
	mux.HandleFunc("POST "+base+"/{account_id}/chats/{chat_id}/typing", a.SendTyping)
	mux.HandleFunc("POST "+base+"/{account_id}/chats/{chat_id}/read", a.MarkRead)
	mux.HandleFunc("POST "+base+"/{account_id}/chats/{chat_id}/messages/{message_id}/react", a.ReactMessage)
	mux.HandleFunc("POST "+base+"/{account_id}/chats/{chat_id}/messages/{message_id}/reply", a.ReplyMessage)

	// Contacts & groups
	mux.HandleFunc("GET "+base+"/{account_id}/contacts/{contact_id}", a.GetContact)
	mux.HandleFunc("GET "+base+"/{account_id}/groups/{group_id}", a.GetGroup)
}
