package handlers

import (
	"context"
	"encoding/json"
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
	GenerateBatch(ctx context.Context, count int, shared apparticles.GenerateRequest) ([]int64, error)
	Publish(ctx context.Context, id int64) (articles.Article, error)
	List(ctx context.Context, limit, offset int) ([]articles.Article, int, error)
	Get(ctx context.Context, id int64) (articles.Article, error)
	RateArticle(ctx context.Context, id int64, rating *bool) error
}

type ArticlesHandler struct {
	svc generateService
}

func NewArticlesHandler(svc generateService) *ArticlesHandler {
	return &ArticlesHandler{svc: svc}
}

func (h *ArticlesHandler) GenerateArticle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.articles")
		log.Debug().Err(err).Msg("decode generate body")
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
		h.writeError(r.Context(), w, "generate", err)
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

func (h *ArticlesHandler) GenerateBatch(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.GenerateBatchRequest
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

	shared := apparticles.GenerateRequest{
		SiteID:                  body.SiteId,
		AutoPublish:             deref(body.AutoPublish),
		IncludeImages:           body.IncludeImages,
		IncludeImageAttribution: ptr(false), // Batch generation doesn't support image attribution currently.
		MinWords:                deref(body.MinWords),
		MaxWords:                deref(body.MaxWords),
		MaxTokens:               deref(body.MaxTokens),
		MaxCycles:               deref(body.MaxCycles),
		Language:                deref(body.Language),
		Provider:                deref(body.Provider),
		Model:                   deref(body.Model),
	}
	if body.AiThreshold != nil {
		shared.AIThreshold = float64(*body.AiThreshold)
	}

	ids, err := h.svc.GenerateBatch(r.Context(), body.Count, shared)
	if err != nil {
		h.writeError(r.Context(), w, "generate_batch", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusAccepted, oapigen.GenerateBatchAccepted{
		Count:      len(ids),
		ArticleIds: ids,
	})
}

func (h *ArticlesHandler) ListArticles(w http.ResponseWriter, r *http.Request, params oapigen.ListArticlesParams) {
	limit, offset := 25, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	arts, total, err := h.svc.List(r.Context(), limit, offset)
	if err != nil {
		h.writeError(r.Context(), w, "list_articles", err)
		return
	}
	items := make([]oapigen.Article, len(arts))
	for i, a := range arts {
		items[i] = toArticle(a)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.ArticleList{Items: items, Total: int64(total)})
}

func (h *ArticlesHandler) GetArticle(w http.ResponseWriter, r *http.Request, id int64) {
	article, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.writeError(r.Context(), w, "get_article", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toArticle(article))
}

func (h *ArticlesHandler) PublishArticle(w http.ResponseWriter, r *http.Request, id int64) {
	article, err := h.svc.Publish(r.Context(), id)
	if err != nil {
		h.writeError(r.Context(), w, "publish_article", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toArticle(article))
}

func (h *ArticlesHandler) RateArticle(w http.ResponseWriter, r *http.Request, id int64) {
	var body oapigen.RateArticleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var rating *bool
	switch body.Rating {
	case oapigen.Like:
		v := true
		rating = &v
	case oapigen.Dislike:
		v := false
		rating = &v
	}
	if err := h.svc.RateArticle(r.Context(), id, rating); err != nil {
		h.writeError(r.Context(), w, "rate_article", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var articlesErrMap = newErrMap("handlers.articles",
	E(apparticles.ErrNoCluster, http.StatusNotFound, "no keyword cluster for topic"),
	E(apparticles.ErrArticleNotFound, http.StatusNotFound, "article not found"),
	E(apparticles.ErrAlreadyPublished, http.StatusConflict, "article already published"),
	E(apparticles.ErrNoDraftToPublish, http.StatusConflict, "article has no draft to publish"),
)

func (h *ArticlesHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	articlesErrMap.Handle(ctx, w, op, err)
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
		Id:          a.ID,
		Keyword:     a.Keyword,
		Status:      string(a.Status),
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
		HumanRating: a.HumanRating,
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
	if len(a.RequestParams) > 0 {
		rp := a.RequestParams
		out.RequestParams = &rp
	}
	out.PublishedAt = a.PublishedAt
	if a.AIScore != nil {
		v := float32(*a.AIScore)
		out.AiScore = &v
	}
	if a.Reward != nil {
		v := float32(*a.Reward)
		out.Reward = &v
	}
	out.QualityOk = a.QualityOK
	out.HumanizeCycles = a.HumanizeCycles
	out.Tokens = a.Tokens
	return out
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func ptr[T any](v T) *T { // Helper to get a pointer to a literal, e.g. ptr(false)
	return &v
}
