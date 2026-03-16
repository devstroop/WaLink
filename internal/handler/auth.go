package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/devstroop/walink/internal/database"
	"github.com/devstroop/walink/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles login.
type AuthHandler struct {
	db        *database.DB
	secretKey string
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(db *database.DB, secretKey string) *AuthHandler {
	return &AuthHandler{db: db, secretKey: secretKey}
}

// Login authenticates a user and returns a JWT.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, model.ErrorResponse{Error: "username and password are required"})
		return
	}

	user, err := h.db.GetUserByUsername(req.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "internal error"})
		return
	}
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, model.ErrorResponse{Error: "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, model.ErrorResponse{Error: "invalid credentials"})
		return
	}

	if !user.Enabled {
		writeJSON(w, http.StatusForbidden, model.ErrorResponse{Error: "account disabled"})
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	claims := &jwt.RegisteredClaims{
		Subject:   user.ID,
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "walink",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.secretKey))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusOK, model.LoginResponse{
		Token:     signed,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		User: model.UserInfo{
			ID:        user.ID,
			Username:  user.Username,
			RoleID:    user.RoleID,
			RoleName:  user.RoleName,
			Enabled:   user.Enabled,
			CreatedAt: user.CreatedAt,
		},
	})
}
