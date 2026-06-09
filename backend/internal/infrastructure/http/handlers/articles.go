package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	apparticles "multiagent-seo/internal/application/articles"
	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

type generateService interface {
	Generate(ctx context.Context, req apparticles.GenerateRequest) (apparticles.GenerateResult, error)
	Publish(ctx context.Context, id int64) (articles.Article, error)
	List(ctx context.Context) ([]articles.Article, error)
	Get(ctx context.Context, id int64) (articles.Article, error)
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
	if body.SiteId == uuid.Nil {
		problem.Write(w, http.StatusBadRequest, "site_id")
		return
	}

	res, err := h.svc.Generate(r.Context(), toGenerateRequest(body))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	a := res.Article
	accepted := oapigen.GenerateAccepted{
		Id:             a.ID,
		Keyword:        a.Keyword,
		Status:         string(a.Status),
		CreatedAt:      a.CreatedAt,
		TargetKeywords: res.TargetKeywords,
		StatusUrl:      ptr(fmt.Sprintf("/articles/%d", a.ID)),
	}
	if res.SuggestedTitle != "" {
		accepted.SuggestedTitle = &res.SuggestedTitle
	}
	if res.Provider != "" {
		accepted.Provider = &res.Provider
	}
	if res.Model != "" {
		accepted.Model = &res.Model
	}
	if a.Site != "" {
		accepted.Site = &a.Site
	}
	if a.SiteID != uuid.Nil {
		site := a.SiteID
		accepted.SiteId = &site
	}
	response.WriteJSON(r.Context(), w, http.StatusAccepted, accepted)
}

func (h *ArticlesHandler) ListArticles(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	arts, err := h.svc.List(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, err)
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
		h.writeError(r.Context(), w, err)
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
		h.writeError(r.Context(), w, err)
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

func (h *ArticlesHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, apparticles.ErrNoCluster):
		problem.Write(w, http.StatusNotFound, "no keyword cluster for topic")
	case errors.Is(err, apparticles.ErrArticleNotFound):
		problem.Write(w, http.StatusNotFound, "article not found")
	case errors.Is(err, apparticles.ErrAlreadyPublished):
		problem.Write(w, http.StatusConflict, "article already published")
	case errors.Is(err, apparticles.ErrNoDraftToPublish):
		problem.Write(w, http.StatusConflict, "article has no draft to publish")
	default:
		log := logger.New(ctx, "handlers.articles")
		log.Error().Err(err).Msg("internal error")
		problem.Write(w, http.StatusInternalServerError, "internal error")
	}
}

func toGenerateRequest(body oapigen.GenerateRequest) apparticles.GenerateRequest {
	req := apparticles.GenerateRequest{
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

func toArticle(a articles.Article) oapigen.Article {
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
	if len(a.CompetitorData) > 0 {
		cd := a.CompetitorData
		out.CompetitorData = &cd
	}
	if len(a.CheckResult) > 0 {
		cr := a.CheckResult
		out.CheckResult = &cr
	}
	return out
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func ptr[T any](v T) *T { return &v }
