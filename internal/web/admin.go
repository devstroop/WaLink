package web

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// UsersList renders the users admin page.
func (h *Handler) UsersList(w http.ResponseWriter, r *http.Request) {
	users, _ := h.db.ListUsers()

	pd := h.page(w, r, "Users", "users", map[string]any{
		"Users": users,
	})
	h.render.Page(w, http.StatusOK, "users", pd)
}

// RolesList renders the roles admin page.
func (h *Handler) RolesList(w http.ResponseWriter, r *http.Request) {
	roles, _ := h.db.ListRoles()

	pd := h.page(w, r, "Roles", "roles", map[string]any{
		"Roles": roles,
	})
	h.render.Page(w, http.StatusOK, "roles", pd)
}

// APIKeysList renders the API keys admin page.
func (h *Handler) APIKeysList(w http.ResponseWriter, r *http.Request) {
	identity := getIdentity(r)
	var keys interface{}
	if identity != nil && identity.HasPermission("*") {
		keys, _ = h.db.ListAllAPIKeys()
	} else if identity != nil {
		keys, _ = h.db.ListAPIKeysByUser(identity.UserID)
	}

	pd := h.page(w, r, "API Keys", "api-keys", map[string]any{
		"Keys": keys,
	})
	h.render.Page(w, http.StatusOK, "api-keys", pd)
}

// Settings renders the settings page.
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	identity := getIdentity(r)
	var user interface{}
	if identity != nil && identity.UserID != "system" {
		user, _ = h.db.GetUser(identity.UserID)
	}
	pd := h.page(w, r, "Settings", "settings", map[string]any{
		"User": user,
	})
	h.render.Page(w, http.StatusOK, "settings", pd)
}

// ChangePassword handles POST /settings/password.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	identity := getIdentity(r)
	if identity == nil || identity.UserID == "system" {
		setFlash(w, "error", "Password change is not available for this account.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	current := r.FormValue("current_password")
	newPw := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	if newPw != confirm {
		setFlash(w, "error", "New passwords do not match.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	if len(newPw) < 8 {
		setFlash(w, "error", "New password must be at least 8 characters.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	user, err := h.db.GetUser(identity.UserID)
	if err != nil || user == nil {
		setFlash(w, "error", "User not found.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(current)); err != nil {
		setFlash(w, "error", "Current password is incorrect.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPw), bcrypt.DefaultCost)
	if err != nil {
		setFlash(w, "error", "Failed to process new password.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if err := h.db.UpdateUserPassword(identity.UserID, string(hash)); err != nil {
		setFlash(w, "error", "Failed to update password.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "Password updated successfully.")
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// NotFound renders the styled 404 error page.
func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	pd := PageData{
		Title: "Not Found — WaLink",
		Page:  "error",
		Data: map[string]any{
			"Code":    "404",
			"Title":   "Page not found",
			"Message": "The page you're looking for doesn't exist or has been moved.",
		},
	}
	h.render.Page(w, http.StatusNotFound, "error", pd)
}
