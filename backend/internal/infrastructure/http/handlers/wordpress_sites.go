package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	domainwp "multiagent-seo/internal/domain/wordpress"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
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
	sites, err := h.sites.List(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, "list_sites", err)
		return
	}
	out := make([]oapigen.WordpressSite, len(sites))
	for i, s := range sites {
		out[i] = toAPISite(s)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *WordpressSitesHandler) CreateWordpressSite(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.CreateWordpressSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.wordpress_sites")
		log.Debug().Err(err).Msg("decode create site body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	site, err := h.sites.Create(r.Context(), domainwp.CreateSite{
		Alias:       body.Alias,
		Provider:    domainwp.Provider(body.Provider),
		URL:         body.Url,
		Username:    strPtrValue(body.Username),
		AppPassword: strPtrValue(body.AppPassword),
		Languages:   fromAPILanguages(body.Languages),
	})
	if err != nil {
		h.writeError(r.Context(), w, "create_site", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, toAPISite(site))
}

func (h *WordpressSitesHandler) GetWordpressSite(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	site, err := h.sites.Get(r.Context(), id)
	if err != nil {
		h.writeError(r.Context(), w, "get_site", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toAPISite(site))
}

func (h *WordpressSitesHandler) UpdateWordpressSite(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.UpdateWordpressSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.wordpress_sites")
		log.Debug().Err(err).Msg("decode update site body")
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
		Languages:   fromAPILanguages(body.Languages),
	})
	if err != nil {
		h.writeError(r.Context(), w, "update_site", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toAPISite(site))
}

func (h *WordpressSitesHandler) DeleteWordpressSite(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if err := h.sites.Delete(r.Context(), id); err != nil {
		h.writeError(r.Context(), w, "delete_site", err)
		return
	}
	response.NoContent(w)
}

var wordpressErrMap = newErrMap("handlers.wordpress_sites",
	E(domainwp.ErrNotFound, http.StatusNotFound, "wordpress site not found"),
	E(domainwp.ErrAliasExists, http.StatusConflict, "alias already in use"),
)

func (h *WordpressSitesHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	wordpressErrMap.Handle(ctx, w, op, err)
}

func toAPISite(s domainwp.Site) oapigen.WordpressSite {
	return oapigen.WordpressSite{
		Id:        s.ID,
		Alias:     s.Alias,
		Provider:  oapigen.SiteProvider(s.Provider),
		Url:       s.URL,
		Username:  s.Username,
		Languages: toAPILanguages(s.Languages),
		Enabled:   s.Enabled,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func fromAPILanguages(in map[string]oapigen.LanguageConfig) map[string]domainwp.LanguageConfig {
	if in == nil {
		return nil
	}
	out := make(map[string]domainwp.LanguageConfig, len(in))
	for lang, cfg := range in {
		out[lang] = domainwp.LanguageConfig{
			ContextKey:            strPtrValue(cfg.ContextKey),
			KeywordsSpreadsheetID: strPtrValue(cfg.KeywordsSpreadsheetId),
			KeywordsSheet:         strPtrValue(cfg.KeywordsSheet),
		}
	}
	return out
}

func toAPILanguages(in map[string]domainwp.LanguageConfig) map[string]oapigen.LanguageConfig {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]oapigen.LanguageConfig, len(in))
	for lang, cfg := range in {
		out[lang] = oapigen.LanguageConfig{
			ContextKey:            ptrOrNil(cfg.ContextKey),
			KeywordsSpreadsheetId: ptrOrNil(cfg.KeywordsSpreadsheetID),
			KeywordsSheet:         ptrOrNil(cfg.KeywordsSheet),
		}
	}
	return out
}

func strPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
