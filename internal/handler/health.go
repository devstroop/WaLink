package handler

import (
	"encoding/json"
	"net/http"

	"github.com/devstroop/walink/internal/service"
)

// Health returns a simple health check response.
func Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// requireAccount resolves the account from the path. Returns nil and writes a
// 404 if the account doesn't exist.
func (a *API) requireAccount(w http.ResponseWriter, r *http.Request) *service.Account {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return nil
	}
	return acct
}

// requireConnectedAccount resolves the account from the path and ensures it has
// an active WhatsApp connection. Returns nil and writes an error response if
// the account doesn't exist or can't connect.
func (a *API) requireConnectedAccount(w http.ResponseWriter, r *http.Request) *service.Account {
	acct := a.requireAccount(w, r)
	if acct == nil {
		return nil
	}
	if err := acct.EnsureConnected(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "connect: "+err.Error())
		return nil
	}
	return acct
}
