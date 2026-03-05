package handler

import (
	"net/http"

	"github.com/itsalfredakku/walink/internal/model"
)

// ListAccounts — GET /api/v1/accounts
func (a *API) ListAccounts(w http.ResponseWriter, r *http.Request) {
	resp := a.mgr.ListAccounts()
	writeJSON(w, http.StatusOK, resp)
}

// CreateAccount — POST /api/v1/accounts
func (a *API) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req model.CreateAccountRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	resp, err := a.mgr.CreateAccount(req)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

// GetAccount — GET /api/v1/accounts/{account_id}
func (a *API) GetAccount(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, acct.Info())
}

// DeleteAccount — DELETE /api/v1/accounts/{account_id}
func (a *API) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	deleteData := r.URL.Query().Get("delete_data") == "true"
	resp, err := a.mgr.DeleteAccount(r.PathValue("account_id"), deleteData)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetConfig — GET /api/v1/accounts/{account_id}/config
func (a *API) GetConfig(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	acct.TouchActivity()
	writeJSON(w, http.StatusOK, model.AccountConfig{
		AccountID:   acct.ID,
		AccountName: acct.AccountName,
		IdleTimeout: acct.IdleTimeout,
	})
}

// UpdateConfig — PUT /api/v1/accounts/{account_id}/config
func (a *API) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	acct := a.mgr.GetAccount(r.PathValue("account_id"))
	if acct == nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	var req model.UpdateAccountConfigRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	acct.TouchActivity()
	if req.AccountName != nil {
		acct.AccountName = *req.AccountName
	}
	if req.IdleTimeout != nil {
		acct.IdleTimeout = *req.IdleTimeout
	}

	writeJSON(w, http.StatusOK, model.AccountConfig{
		AccountID:   acct.ID,
		AccountName: acct.AccountName,
		IdleTimeout: acct.IdleTimeout,
	})
}
