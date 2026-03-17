package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/devstroop/walink/internal/model"
)

// DashboardData holds data for the dashboard page.
type DashboardData struct {
	TotalAccounts int
	Connected     int
	Disconnected  int
	TotalUsers    int
	Accounts      []AccountRow
}

// AccountRow is a simplified account for display.
type AccountRow struct {
	ID          string
	AccountName string
	PhoneNumber string
	Connected   bool
	CreatedAt   string
}

// infoToRow converts a model.AccountInfo to an AccountRow.
func (h *Handler) infoToRow(a model.AccountInfo) AccountRow {
	phone := ""
	if a.PhoneNumber != nil {
		phone = *a.PhoneNumber
	}
	acct := h.mgr.GetAccount(a.ID)
	isConn := acct != nil && acct.IsLoggedIn()
	return AccountRow{
		ID:          a.ID,
		AccountName: a.AccountName,
		PhoneNumber: phone,
		Connected:   isConn,
		CreatedAt:   a.CreatedAt.Format(time.RFC3339),
	}
}

// Dashboard renders the dashboard page.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	list := h.mgr.ListAccounts()

	connected := 0
	rows := make([]AccountRow, 0, len(list.Accounts))
	for _, a := range list.Accounts {
		row := h.infoToRow(a)
		if row.Connected {
			connected++
		}
		rows = append(rows, row)
	}

	userCount := 0
	if users, err := h.db.ListUsers(); err == nil {
		userCount = len(users)
	}

	data := DashboardData{
		TotalAccounts: list.Total,
		Connected:     connected,
		Disconnected:  list.Total - connected,
		TotalUsers:    userCount,
		Accounts:      rows,
	}

	// Only show last 5 on dashboard
	if len(data.Accounts) > 5 {
		data.Accounts = data.Accounts[:5]
	}

	pd := h.page(w, r, "Dashboard", "dashboard", data)
	h.render.Page(w, http.StatusOK, "dashboard", pd)
}

// AccountsList renders the accounts list page.
func (h *Handler) AccountsList(w http.ResponseWriter, r *http.Request) {
	list := h.mgr.ListAccounts()

	identity := getIdentity(r)
	rows := make([]AccountRow, 0, len(list.Accounts))
	for _, a := range list.Accounts {
		// Non-admin: filter to own accounts
		if identity != nil && !identity.HasPermission("*") && a.UserID != identity.UserID {
			continue
		}
		rows = append(rows, h.infoToRow(a))
	}

	pd := h.page(w, r, "Accounts", "accounts", map[string]any{
		"Accounts": rows,
	})
	h.render.Page(w, http.StatusOK, "accounts", pd)
}

// AccountsCreate handles POST /accounts (create new account).
func (h *Handler) AccountsCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("account_name")
	phone := r.FormValue("phone_number")

	if name == "" || phone == "" {
		setFlash(w, "error", "Account name and phone number are required.")
		http.Redirect(w, r, "/accounts", http.StatusSeeOther)
		return
	}

	identity := getIdentity(r)
	req := model.CreateAccountRequest{
		PhoneNumber: phone,
		AccountName: name,
	}
	if identity != nil && identity.UserID != "system" {
		req.UserID = identity.UserID
	}

	_, err := h.mgr.CreateAccount(req)
	if err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to create account: %s", err.Error()))
		http.Redirect(w, r, "/accounts", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Account created successfully.")
	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

// AccountDetail renders the account detail page.
func (h *Handler) AccountDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := h.mgr.GetAccount(id)
	if acct == nil {
		setFlash(w, "error", "Account not found.")
		http.Redirect(w, r, "/accounts", http.StatusSeeOther)
		return
	}

	info := acct.Info()
	phone := ""
	if info.PhoneNumber != nil {
		phone = *info.PhoneNumber
	}

	pd := h.page(w, r, info.AccountName, "account-detail", map[string]any{
		"Account": map[string]any{
			"ID":          info.ID,
			"AccountName": info.AccountName,
			"PhoneNumber": phone,
			"Connected":   acct.IsLoggedIn(),
		},
	})
	h.render.Page(w, http.StatusOK, "account-detail", pd)
}

// AccountDelete handles DELETE /accounts/{id}.
func (h *Handler) AccountDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, err := h.mgr.DeleteAccount(id, false)
	if err != nil {
		if isHTMX(r) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		setFlash(w, "error", err.Error())
		http.Redirect(w, r, "/accounts", http.StatusSeeOther)
		return
	}

	if isHTMX(r) {
		hxRedirect(w, "/accounts")
		return
	}
	setFlash(w, "success", "Account deleted.")
	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

// AccountSessionStatus returns a badge partial for htmx polling.
func (h *Handler) AccountSessionStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := h.mgr.GetAccount(id)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if acct != nil && acct.IsLoggedIn() {
		fmt.Fprint(w, `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">Connected</span>`)
	} else {
		fmt.Fprint(w, `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">Disconnected</span>`)
	}
}

// AccountQR returns the QR code image for htmx.
func (h *Handler) AccountQR(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := h.mgr.GetAccount(id)
	if acct == nil {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="mt-4"><img src="/api/v1/accounts/%s/session/qr" alt="QR Code" class="w-64 h-64 rounded-lg border border-gray-200"><p class="text-xs text-gray-500 mt-2">Scan with WhatsApp → Linked Devices → Link a Device</p></div>`, id)
}

// AccountPair triggers phone pairing and returns the code for htmx.
func (h *Handler) AccountPair(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := h.mgr.GetAccount(id)
	if acct == nil {
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}

	code, err := acct.PairPhone(r.Context(), acct.PhoneNumber)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil {
		fmt.Fprintf(w, `<div class="mt-4 p-3 bg-red-50 border border-red-200 rounded-lg text-sm text-red-700">%s</div>`, err.Error())
		return
	}

	fmt.Fprintf(w, `<div class="mt-4 p-4 bg-brand-50 border border-brand-200 rounded-lg text-center"><p class="text-sm text-gray-600 mb-2">Enter this code in WhatsApp:</p><p class="text-3xl font-mono font-bold text-brand-700 tracking-widest">%s</p><p class="text-xs text-gray-500 mt-2">WhatsApp → Linked Devices → Link a Device → Link with Phone Number</p></div>`, code)
}

// AccountDisconnect disconnects the session and returns a status badge.
func (h *Handler) AccountDisconnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acct := h.mgr.GetAccount(id)
	if acct != nil {
		acct.Logout()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">Disconnected</span>`)
}
