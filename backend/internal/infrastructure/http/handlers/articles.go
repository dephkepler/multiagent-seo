package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	appgen "multiagent-seo/internal/application/generate"
	"multiagent-seo/internal/domain/generate"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/validate"
)

type generateService interface {
	Generate(ctx context.Context, req appgen.GenerateRequest) (generate.Article, error)
	Publish(ctx context.Context, id int64) (generate.Article, error)
	List(ctx context.Context) ([]generate.Article, error)
	Get(ctx context.Context, id int64) (generate.Article, error)
}

type ArticlesHandler struct {
	svc generateService
}

func NewArticlesHandler(svc generateService) *ArticlesHandler {
	return &ArticlesHandler{svc: svc}
}

func (h *ArticlesHandler) GenerateArticle(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}
	// A zero uuid satisfies "required" (its bytes are non-empty), so reject it
	// explicitly: an unset site_id must not silently target the nil site.
	if body.SiteId == uuid.Nil {
		problem.Write(w, http.StatusBadRequest, "site_id")
		return
	}

	article, err := h.svc.Generate(r.Context(), toGenerateRequest(body))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusAccepted, oapigen.GenerateAccepted{
		Id:        article.ID,
		Keyword:   article.Keyword,
		Status:    string(article.Status),
		CreatedAt: article.CreatedAt,
	})
}

func (h *ArticlesHandler) ListArticles(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	arts, err := h.svc.List(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	out := make([]oapigen.Article, len(arts))
	for i, a := range arts {
		out[i] = toArticle(a)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *ArticlesHandler) GetArticle(w http.ResponseWriter, r *http.Request, id int64) {
	if h.unavailable(w) {
		return
	}
	article, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toArticle(article))
}

func (h *ArticlesHandler) PublishArticle(w http.ResponseWriter, r *http.Request, id int64) {
	if h.unavailable(w) {
		return
	}
	article, err := h.svc.Publish(r.Context(), id)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toArticle(article))
}

func (h *ArticlesHandler) unavailable(w http.ResponseWriter) bool {
	if h.svc == nil {
		problem.Write(w, http.StatusServiceUnavailable, "database unavailable")
		return true
	}
	return false
}

func (h *ArticlesHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appgen.ErrNoCluster):
		problem.Write(w, http.StatusNotFound, "no keyword cluster for topic")
	case errors.Is(err, appgen.ErrArticleNotFound):
		problem.Write(w, http.StatusNotFound, "article not found")
	case errors.Is(err, appgen.ErrAlreadyPublished):
		problem.Write(w, http.StatusConflict, "article already published")
	case errors.Is(err, appgen.ErrNoDraftToPublish):
		problem.Write(w, http.StatusConflict, "article has no draft to publish")
	default:
		problem.Write(w, http.StatusInternalServerError, "internal error")
	}
}

func toGenerateRequest(body oapigen.GenerateRequest) appgen.GenerateRequest {
	req := appgen.GenerateRequest{
		Keyword:                 body.Keyword,
		SiteID:                  body.SiteId,
		AutoPublish:             deref(body.AutoPublish),
		IncludeImages:           body.IncludeImages,
		IncludeImageAttribution: body.IncludeImageAttribution,
		MinWords:                deref(body.MinWords),
		MaxWords:                deref(body.MaxWords),
		MaxTokens:               deref(body.MaxTokens),
		MaxCycles:               deref(body.MaxCycles),
		Language:                deref(body.Language),
		SiteTopic:               deref(body.SiteTopic),
		ExtraRules:              deref(body.ExtraRules),
		Provider:                deref(body.Provider),
		Model:                   deref(body.Model),
	}
	if body.AiThreshold != nil {
		req.AIThreshold = float64(*body.AiThreshold)
	}
	return req
}

func toArticle(a generate.Article) oapigen.Article {
	out := oapigen.Article{
		Id:        a.ID,
		Keyword:   a.Keyword,
		Status:    string(a.Status),
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
	if a.SiteID != uuid.Nil {
		site := a.SiteID
		out.SiteId = &site
	}
	if a.Site != "" {
		out.Site = &a.Site
	}
	if a.WPPostID != 0 {
		id := a.WPPostID
		out.WpPostId = &id
	}
	if a.WPEditURL != "" {
		out.WpEditUrl = &a.WPEditURL
	}
	if a.WPPostURL != "" {
		out.WpPostUrl = &a.WPPostURL
	}
	out.ImagesRequested = &a.ImagesRequested
	out.ImagesResolved = &a.ImagesResolved
	out.ImagesSkipped = &a.ImagesSkipped
	return out
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
