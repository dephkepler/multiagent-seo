package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	appapitoken "multiagent-seo/internal/application/apitoken"
	domainapitoken "multiagent-seo/internal/domain/apitoken"
	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

type apiTokenService interface {
	Create(ctx context.Context, userID uuid.UUID, name string) (domainapitoken.Token, string, error)
	List(ctx context.Context, userID uuid.UUID) ([]domainapitoken.Token, error)
	Revoke(ctx context.Context, userID, id uuid.UUID) error
}

type ApiTokensHandler struct {
	svc apiTokenService
}

func NewApiTokensHandler(svc apiTokenService) *ApiTokensHandler {
	return &ApiTokensHandler{svc: svc}
}

func (h *ApiTokensHandler) CreateApiToken(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.CreateApiTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	tok, secret, err := h.svc.Create(r.Context(), user, body.Name)
	if err != nil {
		if errors.Is(err, appapitoken.ErrNoName) {
			problem.Write(w, http.StatusBadRequest, "name")
			return
		}
		log := logger.New(r.Context(), "handlers.apitokens")
		log.Error().Err(err).Msg("create token")
		problem.Write(w, http.StatusInternalServerError, "internal error")
		return
	}

	response.WriteJSON(r.Context(), w, http.StatusCreated, oapigen.ApiTokenCreated{
		Id:        tok.ID,
		Name:      tok.Name,
		Prefix:    tok.Prefix,
		Token:     secret,
		CreatedAt: tok.CreatedAt,
	})
}

func (h *ApiTokensHandler) ListApiTokens(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	tokens, err := h.svc.List(r.Context(), user)
	if err != nil {
		log := logger.New(r.Context(), "handlers.apitokens")
		log.Error().Err(err).Msg("list tokens")
		problem.Write(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]oapigen.ApiToken, len(tokens))
	for i, t := range tokens {
		out[i] = oapigen.ApiToken{Id: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedAt: t.CreatedAt}
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *ApiTokensHandler) DeleteApiToken(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	if err := h.svc.Revoke(r.Context(), user, id); err != nil {
		if errors.Is(err, domainapitoken.ErrNotFound) {
			problem.Write(w, http.StatusNotFound, "token not found")
			return
		}
		log := logger.New(r.Context(), "handlers.apitokens")
		log.Error().Err(err).Msg("revoke token")
		problem.Write(w, http.StatusInternalServerError, "internal error")
		return
	}
	response.NoContent(w)
}

// currentUser pulls the authenticated user id from the request context (set by
// the BearerAuth middleware) and writes 401/503 on failure.
func (h *ApiTokensHandler) currentUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if h.svc == nil {
		problem.Write(w, http.StatusServiceUnavailable, "database unavailable")
		return uuid.Nil, false
	}
	raw, ok := httpMiddleware.UserIDFromContext(r.Context())
	if !ok {
		problem.Write(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		problem.Write(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}
	return id, true
}
