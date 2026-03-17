package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/devstroop/walink/internal/database"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// UsersList renders the users admin page.
func (h *Handler) UsersList(w http.ResponseWriter, r *http.Request) {
	users, _ := h.db.ListUsers()
	roles, _ := h.db.ListRoles()

	pd := h.page(w, r, "Users", "users", map[string]any{
		"Users": users,
		"Roles": roles,
	})
	h.render.Page(w, http.StatusOK, "users", pd)
}

// UsersCreate handles POST /admin/users.
func (h *Handler) UsersCreate(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	roleID := r.FormValue("role_id")

	if username == "" || password == "" || roleID == "" {
		setFlash(w, "error", "Username, password, and role are required.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	if len(password) < 8 {
		setFlash(w, "error", "Password must be at least 8 characters.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		setFlash(w, "error", "Failed to hash password.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	rec := &database.UserRecord{
		ID:           uuid.NewString(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		RoleID:       roleID,
		Enabled:      true,
	}
	if err := h.db.CreateUser(rec); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to create user: %s", err.Error()))
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "User created.")
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// UsersUpdate handles POST /admin/users/{id}/update.
func (h *Handler) UsersUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	roleID := r.FormValue("role_id")
	enabled := r.FormValue("enabled") == "on"

	if username == "" {
		setFlash(w, "error", "Username cannot be empty.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	// Update role + enabled
	if err := h.db.UpdateUser(id, roleID, enabled); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to update user: %s", err.Error()))
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	// Update username
	if err := h.db.UpdateUserUsername(id, username); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to update username: %s", err.Error()))
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	// Update email
	if err := h.db.UpdateUserEmail(id, email); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to update email: %s", err.Error()))
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	setFlash(w, "success", "User updated.")
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// UsersDelete handles POST /admin/users/{id}/delete.
func (h *Handler) UsersDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	identity := getIdentity(r)
	if identity != nil && identity.UserID == id {
		setFlash(w, "error", "You cannot delete your own account.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	if err := h.db.DeleteUser(id); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to delete user: %s", err.Error()))
	} else {
		setFlash(w, "success", "User deleted.")
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// UsersResetPassword handles POST /admin/users/{id}/reset-password.
func (h *Handler) UsersResetPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	newPw := r.FormValue("new_password")

	if len(newPw) < 8 {
		setFlash(w, "error", "Password must be at least 8 characters.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPw), bcrypt.DefaultCost)
	if err != nil {
		setFlash(w, "error", "Failed to hash password.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	if err := h.db.UpdateUserPassword(id, string(hash)); err != nil {
		setFlash(w, "error", "Failed to reset password.")
	} else {
		setFlash(w, "success", "Password reset successfully.")
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// RolesList renders the roles admin page.
func (h *Handler) RolesList(w http.ResponseWriter, r *http.Request) {
	roles, _ := h.db.ListRoles()

	// Attach permissions to each role for display
	type roleWithPerms struct {
		database.RoleRecord
		Permissions []string
		UserCount   int
	}
	enriched := make([]roleWithPerms, 0, len(roles))
	for _, role := range roles {
		perms, _ := h.db.GetRolePermissions(role.ID)
		count, _ := h.db.CountUsersByRole(role.ID)
		enriched = append(enriched, roleWithPerms{
			RoleRecord:  *role,
			Permissions: perms,
			UserCount:   count,
		})
	}

	pd := h.page(w, r, "Roles", "roles", map[string]any{
		"Roles": enriched,
	})
	h.render.Page(w, http.StatusOK, "roles", pd)
}

// RolesCreate handles POST /admin/roles.
func (h *Handler) RolesCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	permsRaw := strings.TrimSpace(r.FormValue("permissions"))

	if name == "" {
		setFlash(w, "error", "Role name is required.")
		http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
		return
	}

	rec := &database.RoleRecord{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
	}
	if err := h.db.CreateRole(rec); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to create role: %s", err.Error()))
		http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
		return
	}

	if permsRaw != "" {
		perms := splitPerms(permsRaw)
		_ = h.db.SetRolePermissions(rec.ID, perms)
	}

	setFlash(w, "success", "Role created.")
	http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
}

// RolesUpdate handles POST /admin/roles/{id}/update.
func (h *Handler) RolesUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	permsRaw := strings.TrimSpace(r.FormValue("permissions"))

	if err := h.db.UpdateRole(id, name, description); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to update role: %s", err.Error()))
		http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
		return
	}

	perms := splitPerms(permsRaw)
	if err := h.db.SetRolePermissions(id, perms); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to update permissions: %s", err.Error()))
	} else {
		setFlash(w, "success", "Role updated.")
	}
	http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
}

// RolesDelete handles POST /admin/roles/{id}/delete.
func (h *Handler) RolesDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	role, err := h.db.GetRole(id)
	if err != nil || role == nil {
		setFlash(w, "error", "Role not found.")
		http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
		return
	}
	if role.IsBuiltin {
		setFlash(w, "error", "Built-in roles cannot be deleted.")
		http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
		return
	}

	count, _ := h.db.CountUsersByRole(id)
	if count > 0 {
		setFlash(w, "error", fmt.Sprintf("Cannot delete: %d user(s) still assigned to this role.", count))
		http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
		return
	}

	if err := h.db.DeleteRole(id); err != nil {
		setFlash(w, "error", fmt.Sprintf("Failed to delete role: %s", err.Error()))
	} else {
		setFlash(w, "success", "Role deleted.")
	}
	http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
}

// splitPerms splits a comma/newline-separated permission string into a clean slice.
func splitPerms(raw string) []string {
	raw = strings.ReplaceAll(raw, "\n", ",")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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

	// Also load accounts for the create dropdown
	list := h.mgr.ListAccounts()
	rows := make([]AccountRow, 0, len(list.Accounts))
	for _, a := range list.Accounts {
		if identity != nil && !identity.HasPermission("*") && a.UserID != identity.UserID {
			continue
		}
		rows = append(rows, h.infoToRow(a))
	}

	pd := h.page(w, r, "API Keys", "api-keys", map[string]any{
		"Keys":     keys,
		"Accounts": rows,
	})
	h.render.Page(w, http.StatusOK, "api-keys", pd)
}

// APIKeysCreate handles POST /admin/api-keys — creates a key and returns JSON with the plain key.
func (h *Handler) APIKeysCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	name := strings.TrimSpace(r.FormValue("name"))
	accountID := strings.TrimSpace(r.FormValue("account_id"))
	expiresAt := strings.TrimSpace(r.FormValue("expires_at"))

	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	identity := getIdentity(r)
	if identity == nil || identity.UserID == "system" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot create keys as system"})
		return
	}

	var accPtr *string
	if accountID != "" {
		accPtr = &accountID
	}
	var expPtr *string
	if expiresAt != "" {
		// Parse HTML datetime-local format → RFC3339
		t, err := time.Parse("2006-01-02T15:04", expiresAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expiry date"})
			return
		}
		if t.Before(time.Now()) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expiry must be in the future"})
			return
		}
		s := t.UTC().Format(time.RFC3339)
		expPtr = &s
	}

	// Generate key
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key generation failed"})
		return
	}
	plainKey := "walink_" + hex.EncodeToString(rawBytes)
	prefix := plainKey[:15]
	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])

	rec := &database.APIKeyRecord{
		ID:        uuid.NewString(),
		UserID:    identity.UserID,
		AccountID: accPtr,
		Name:      name,
		Prefix:    prefix,
		KeyHash:   keyHash,
		ExpiresAt: expPtr,
		Enabled:   true,
	}
	if err := h.db.CreateAPIKey(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create key"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"key": plainKey, "id": rec.ID, "name": name, "prefix": prefix})
}

// APIKeysDelete handles POST /admin/api-keys/{id}/delete.
func (h *Handler) APIKeysDelete(w http.ResponseWriter, r *http.Request) {
	keyID := r.PathValue("id")
	if keyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing key id"})
		return
	}

	rec, err := h.db.GetAPIKey(keyID)
	if err != nil || rec == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
		return
	}

	// Ownership check
	identity := getIdentity(r)
	if identity != nil && !identity.HasPermission("*") && rec.UserID != identity.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.db.DeleteAPIKey(keyID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── MCP Settings ────────────────────────────────────────────

// MCPSettings renders the MCP configuration page.
func (h *Handler) MCPSettings(w http.ResponseWriter, r *http.Request) {
	enabled := h.db.GetSettingBool("mcp.enabled", true)
	path := h.db.GetSetting("mcp.path", "/mcp")

	// Count API keys for display
	allKeys, _ := h.db.ListAllAPIKeys()

	pd := h.page(w, r, "MCP Server", "mcp", map[string]any{
		"Enabled":     enabled,
		"Path":        path,
		"APIKeyCount": len(allKeys),
	})
	h.render.Page(w, http.StatusOK, "mcp", pd)
}

// MCPSettingsUpdate handles POST /admin/mcp.
func (h *Handler) MCPSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	enabled := r.FormValue("enabled") == "on" || r.FormValue("enabled") == "true"

	val := "false"
	if enabled {
		val = "true"
	}
	if err := h.db.SetSetting("mcp.enabled", val); err != nil {
		setFlash(w, "error", "Failed to update MCP settings.")
		http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
		return
	}

	if enabled {
		setFlash(w, "success", "MCP server enabled.")
	} else {
		setFlash(w, "success", "MCP server disabled.")
	}
	http.Redirect(w, r, "/admin/mcp", http.StatusSeeOther)
}

// Messaging renders the messaging page with sender account selection.
func (h *Handler) Messaging(w http.ResponseWriter, r *http.Request) {
	list := h.mgr.ListAccounts()

	identity := getIdentity(r)
	rows := make([]AccountRow, 0, len(list.Accounts))
	for _, a := range list.Accounts {
		if identity != nil && !identity.HasPermission("*") && a.UserID != identity.UserID {
			continue
		}
		rows = append(rows, h.infoToRow(a))
	}

	pd := h.page(w, r, "Messaging", "messaging", map[string]any{
		"Accounts": rows,
	})
	h.render.Page(w, http.StatusOK, "messaging", pd)
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
