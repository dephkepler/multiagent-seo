package handlers

import (
	"context"
	"encoding/json"
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

var apiTokensErrMap = newErrMap("handlers.apitokens",
	E(appapitoken.ErrNoName, http.StatusBadRequest, "name"),
	E(domainapitoken.ErrNotFound, http.StatusNotFound, "token not found"),
)

func (h *ApiTokensHandler) CreateApiToken(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.CreateApiTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.apitokens")
		log.Debug().Err(err).Msg("decode create token body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	tok, secret, err := h.svc.Create(r.Context(), user, body.Name)
	if err != nil {
		apiTokensErrMap.Handle(r.Context(), w, "create_token", err)
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
		apiTokensErrMap.Handle(r.Context(), w, "list_tokens", err)
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
		apiTokensErrMap.Handle(r.Context(), w, "revoke_token", err)
		return
	}
	response.NoContent(w)
}

func (h *ApiTokensHandler) currentUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
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
