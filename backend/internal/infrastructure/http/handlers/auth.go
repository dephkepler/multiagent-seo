package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	appauth "multiagent-seo/internal/application/auth"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

type AuthService interface {
	Login(ctx context.Context, email, password string) (string, time.Time, error)
}

type LoginHandler struct {
	auth AuthService
}

func NewLoginHandler(auth AuthService) *LoginHandler {
	return &LoginHandler{auth: auth}
}

func (h *LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		problem.Write(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	token, expiresAt, err := h.auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		log := logger.New(r.Context(), "handlers.auth")
		if errors.Is(err, appauth.ErrInvalidCredentials) {
			// Audit trail for failed logins; never log password or token.
			log.Warn().Str("email", body.Email).Msg("login failed")
			problem.Write(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		log.Error().Err(err).Msg("internal error")
		problem.Write(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.LoginResponse{Token: token, ExpiresAt: expiresAt})
}
