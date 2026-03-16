package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/devstroop/walink/internal/config"
	"github.com/devstroop/walink/internal/database"
	"github.com/golang-jwt/jwt/v5"
)

// ─── Identity context ───────────────────────────────────────

type contextKey string

const identityKey contextKey = "identity"

// Identity represents the authenticated caller.
type Identity struct {
	UserID      string   // "system" for secret_key auth
	Username    string   // "system" for secret_key auth
	RoleName    string   // "admin" for secret_key, role.name for users
	Permissions []string // ["*"] for admin, specific for others
}

// GetIdentity extracts the caller identity from the request context.
func GetIdentity(r *http.Request) *Identity {
	v, _ := r.Context().Value(identityKey).(*Identity)
	return v
}

// GetIdentityFromContext extracts the caller identity from a raw context.
// Useful for MCP tool handlers where the HTTP request is not directly available.
func GetIdentityFromContext(ctx context.Context) *Identity {
	v, _ := ctx.Value(identityKey).(*Identity)
	return v
}

// HasPermission checks if the identity is allowed a specific permission.
// Supports exact match and wildcard: "*" matches everything,
// "messages:*" matches "messages:read" and "messages:write".
func (id *Identity) HasPermission(required string) bool {
	for _, p := range id.Permissions {
		if p == "*" {
			return true
		}
		if p == required {
			return true
		}
		// wildcard within resource: "messages:*" matches "messages:read"
		if strings.HasSuffix(p, ":*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(required, prefix) {
				return true
			}
		}
	}
	return false
}

// ─── Auth middleware ─────────────────────────────────────────

// Auth returns middleware that authenticates via Bearer token.
// It first checks against the static secret_key (system admin),
// then falls back to JWT-based user auth.
func Auth(secretKey string, db *database.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			jsonError(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		token := parts[1]

		// Path 1: static secret_key → system admin
		if subtle.ConstantTimeCompare([]byte(token), []byte(secretKey)) == 1 {
			id := &Identity{
				UserID:      "system",
				Username:    "system",
				RoleName:    "admin",
				Permissions: []string{"*"},
			}
			ctx := context.WithValue(r.Context(), identityKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Path 2: JWT token → user auth
		claims := &jwt.RegisteredClaims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secretKey), nil
		})
		if err != nil || !parsed.Valid {
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		userID := claims.Subject
		if userID == "" {
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		user, err := db.GetUser(userID)
		if err != nil || user == nil {
			jsonError(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		if !user.Enabled {
			jsonError(w, "account disabled", http.StatusForbidden)
			return
		}

		perms, err := db.GetRolePermissions(user.RoleID)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}

		id := &Identity{
			UserID:      user.ID,
			Username:    user.Username,
			RoleName:    user.RoleName,
			Permissions: perms,
		}
		ctx := context.WithValue(r.Context(), identityKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePermission returns middleware that checks the caller has a specific permission.
func RequirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := GetIdentity(r)
		if id == nil {
			jsonError(w, "missing authorization", http.StatusUnauthorized)
			return
		}
		if !id.HasPermission(permission) {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// CORS returns middleware that sets CORS headers from config.
// Per the HTTP spec, Access-Control-Allow-Origin must be either a single
// origin or "*" — a comma-separated list is invalid. When multiple origins
// are configured we match the request Origin against the allow-list and
// reflect it back (or reject).
func CORS(cfg config.CORSConfig, next http.Handler) http.Handler {
	allowAll := len(cfg.AllowOrigins) == 1 && cfg.AllowOrigins[0] == "*"
	allowed := make(map[string]struct{}, len(cfg.AllowOrigins))
	for _, o := range cfg.AllowOrigins {
		allowed[o] = struct{}{}
	}
	methods := strings.Join(cfg.AllowMethods, ", ")
	headers := strings.Join(cfg.AllowHeaders, ", ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if _, ok := allowed[origin]; ok && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Headers", headers)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RateLimit returns middleware that caps concurrent in-flight requests.
// When the limit is reached, new requests receive 429 Too Many Requests.
func RateLimit(maxConcurrent int, next http.Handler) http.Handler {
	if maxConcurrent <= 0 {
		return next
	}
	sem := make(chan struct{}, maxConcurrent)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
		}
	})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
