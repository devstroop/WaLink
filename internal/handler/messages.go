package handler

import (
	"net/http"
	"strconv"
)

// GetMessages — GET /api/v1/accounts/{account_id}/messages?chat=...&limit=...&before=...
func (a *API) GetMessages(w http.ResponseWriter, r *http.Request) {
	acct := a.requireAccount(w, r)
	if acct == nil {
		return
	}

	chatJID := r.URL.Query().Get("chat")
	if chatJID == "" {
		writeError(w, http.StatusBadRequest, "chat query parameter required")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	before := r.URL.Query().Get("before") // RFC3339 cursor

	resp, err := acct.ListMessages(chatJID, limit, before)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
