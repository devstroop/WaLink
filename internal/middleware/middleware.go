package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/devstroop/walink/internal/config"
)

// Auth returns middleware that validates the Authorization: Bearer <key> header.
func Auth(secretKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || subtle.ConstantTimeCompare([]byte(parts[1]), []byte(secretKey)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
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
