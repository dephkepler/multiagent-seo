package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	domainwp "multiagent-seo/internal/domain/wordpress"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/validate"
)

const maxBodyBytes = 64 << 10

type WordpressService interface {
	Create(ctx context.Context, in domainwp.CreateSite) (domainwp.Site, error)
	List(ctx context.Context) ([]domainwp.Site, error)
	Get(ctx context.Context, id uuid.UUID) (domainwp.Site, error)
	Update(ctx context.Context, id uuid.UUID, in domainwp.UpdateSite) (domainwp.Site, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type WordpressSitesHandler struct {
	sites WordpressService
}

func NewWordpressSitesHandler(sites WordpressService) *WordpressSitesHandler {
	return &WordpressSitesHandler{sites: sites}
}

func (h *WordpressSitesHandler) ListWordpressSites(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	sites, err := h.sites.List(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	out := make([]oapigen.WordpressSite, len(sites))
	for i, s := range sites {
		out[i] = toAPI(s)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *WordpressSitesHandler) CreateWordpressSite(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.CreateWordpressSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	site, err := h.sites.Create(r.Context(), domainwp.CreateSite{
		Alias:       body.Alias,
		URL:         body.Url,
		Username:    body.Username,
		AppPassword: body.AppPassword,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, toAPI(site))
}

func (h *WordpressSitesHandler) GetWordpressSite(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if h.unavailable(w) {
		return
	}
	site, err := h.sites.Get(r.Context(), id)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toAPI(site))
}

func (h *WordpressSitesHandler) UpdateWordpressSite(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if h.unavailable(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.UpdateWordpressSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	site, err := h.sites.Update(r.Context(), id, domainwp.UpdateSite{
		Alias:       body.Alias,
		URL:         body.Url,
		Username:    body.Username,
		AppPassword: body.AppPassword,
		Enabled:     body.Enabled,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toAPI(site))
}

func (h *WordpressSitesHandler) DeleteWordpressSite(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if h.unavailable(w) {
		return
	}
	if err := h.sites.Delete(r.Context(), id); err != nil {
		h.writeError(w, err)
		return
	}
	response.NoContent(w)
}

// unavailable guards the DB-backed handler when the pool was absent at boot
// (see cmd/server wiring); a nil service means the database is unreachable.
func (h *WordpressSitesHandler) unavailable(w http.ResponseWriter) bool {
	if h.sites == nil {
		problem.Write(w, http.StatusServiceUnavailable, "database unavailable")
		return true
	}
	return false
}

func (h *WordpressSitesHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainwp.ErrNotFound):
		problem.Write(w, http.StatusNotFound, "wordpress site not found")
	case errors.Is(err, domainwp.ErrAliasExists):
		problem.Write(w, http.StatusConflict, "alias already in use")
	default:
		problem.Write(w, http.StatusInternalServerError, "internal error")
	}
}

func toAPI(s domainwp.Site) oapigen.WordpressSite {
	return oapigen.WordpressSite{
		Id:        s.ID,
		Alias:     s.Alias,
		Url:       s.URL,
		Username:  s.Username,
		Enabled:   s.Enabled,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}
