package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	appauth "multiagent-seo/internal/application/auth"
	"multiagent-seo/internal/domain/user"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

type AuthService interface {
	Login(ctx context.Context, email, password string) (appauth.Session, error)
	ListUsers(ctx context.Context) ([]user.User, error)
}

type LoginHandler struct {
	auth AuthService
}

func NewLoginHandler(auth AuthService) *LoginHandler {
	return &LoginHandler{auth: auth}
}

func (h *LoginHandler) Login(w http.ResponseWriter, r *http.Request) {
	if isNil(h.auth) {
		problem.Write(w, http.StatusServiceUnavailable, "auth unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.auth")
		log.Debug().Err(err).Msg("decode login body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	session, err := h.auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		log := logger.New(r.Context(), "handlers.auth")
		if errors.Is(err, appauth.ErrInvalidCredentials) {
			log.Warn().Msg("login failed")
			problem.Write(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		log.Error().Err(err).Str("op", "login").Msg("internal error")
		problem.Write(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.LoginResponse{
		Token:     session.Token,
		ExpiresAt: session.ExpiresAt,
		Role:      string(session.Role),
	})
}

func (h *LoginHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if isNil(h.auth) {
		problem.Write(w, http.StatusServiceUnavailable, "auth unavailable")
		return
	}

	users, err := h.auth.ListUsers(r.Context())
	if err != nil {
		log := logger.New(r.Context(), "handlers.auth")
		log.Error().Err(err).Str("op", "list_users").Msg("internal error")
		problem.Write(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]oapigen.User, len(users))
	for i, u := range users {
		out[i] = oapigen.User{Id: u.ID, Email: u.Email, Role: string(u.Role), CreatedAt: u.CreatedAt}
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}
