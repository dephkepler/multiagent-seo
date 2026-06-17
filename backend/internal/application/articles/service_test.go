package articles_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	apparticles "multiagent-seo/internal/application/articles"
	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/jobrunner"
)

type fakeRepo struct {
	arts map[int64]*articles.Article
	next int64
}

func (r *fakeRepo) Create(_ context.Context, in articles.CreateArticle) (int64, error) {
	r.next++
	r.arts[r.next] = &articles.Article{ID: r.next, Keyword: in.Keyword, SiteID: in.SiteID, Status: articles.StatusGenerating}
	return r.next, nil
}
func (r *fakeRepo) Get(_ context.Context, id int64) (*articles.Article, error) {
	a, ok := r.arts[id]
	if !ok {
		return nil, articles.ErrNotFound
	}
	cp := *a
	return &cp, nil
}
func (r *fakeRepo) List(context.Context, int, int) ([]articles.Article, int, error) {
	return nil, 0, nil
}
func (r *fakeRepo) UpdateDraft(_ context.Context, id, wpPostID int64, editURL string) error {
	a := r.arts[id]
	a.Status, a.WPPostID, a.WPEditURL = articles.StatusDraft, wpPostID, editURL
	return nil
}
func (r *fakeRepo) MarkFailed(_ context.Context, id int64) error {
	r.arts[id].Status = articles.StatusFailed
	return nil
}
func (r *fakeRepo) MarkPublished(_ context.Context, id int64, postURL string) error {
	r.arts[id].Status, r.arts[id].WPPostURL = articles.StatusPublished, postURL
	return nil
}
func (r *fakeRepo) GeneratedKeywords(_ context.Context, siteID uuid.UUID) ([]string, error) {
	var out []string
	for _, a := range r.arts {
		if a.Status != articles.StatusFailed && a.SiteID == siteID {
			out = append(out, a.Keyword)
		}
	}
	return out, nil
}
func (r *fakeRepo) SetHumanRating(_ context.Context, id int64, rating *bool) error {
	if a := r.arts[id]; a != nil {
		a.HumanRating = rating
	}
	return nil
}
func (r *fakeRepo) SaveImageStats(context.Context, int64, int, int, int) error   { return nil }
func (r *fakeRepo) SaveCompetitorData(context.Context, int64, any) error         { return nil }
func (r *fakeRepo) SaveCheckResult(context.Context, int64, any) error            { return nil }
func (r *fakeRepo) SaveRevision(context.Context, articles.Revision) (int, error) { return 1, nil }
func (r *fakeRepo) SaveEvent(context.Context, articles.GenerationEvent) error    { return nil }

type fakeLLM struct{}

func (fakeLLM) Complete(context.Context, string, int) (string, articles.Usage, error) {
	return "generated body", articles.Usage{}, nil
}

type fakeLLMFactory struct{}

func (fakeLLMFactory) ForModel(string, string) (articles.LLMClient, error) { return fakeLLM{}, nil }

type fakeSERP struct{}

func (fakeSERP) GetSERP(_ context.Context, kw, _ string, _ int) (*articles.CompetitorData, error) {
	return &articles.CompetitorData{Keyword: kw}, nil
}

type fakeTopics struct{}

func (fakeTopics) Lookup(_ context.Context, topic string) (articles.Cluster, error) {
	return articles.Cluster{Keywords: []string{topic, "secondary"}, Title: "Title"}, nil
}
func (fakeTopics) Topics(context.Context) ([]string, error) {
	return []string{"topic-a", "topic-b"}, nil
}
func (fakeTopics) Clusters(context.Context) (map[string]articles.Cluster, error) {
	return map[string]articles.Cluster{
		"topic-a": {Keywords: []string{"topic-a", "secondary"}, Title: "Title"},
		"topic-b": {Keywords: []string{"topic-b", "secondary"}, Title: "Title"},
	}, nil
}

type fakeChecker struct{}

func (fakeChecker) Check(context.Context, string) (*articles.CheckResult, error) {
	return &articles.CheckResult{AIScore: 0.1}, nil
}

type fakeImages struct{}

func (fakeImages) Resolve(context.Context, string, string, string) (articles.ResolvedImage, error) {
	return articles.ResolvedImage{}, nil
}

type fakePub struct{}

func (fakePub) CreateDraft(context.Context, articles.Post) (int64, string, error) {
	return 123, "http://wp/edit", nil
}
func (fakePub) Publish(context.Context, int64) (string, error) { return "http://wp/post", nil }

type fakePubProvider struct{}

func (fakePubProvider) ForSite(context.Context, uuid.UUID) (articles.Publisher, error) {
	return fakePub{}, nil
}

func TestGenerate_HappyPath(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	svc := apparticles.NewService(
		repo, fakeLLMFactory{}, fakeSERP{}, fakeTopics{}, fakeChecker{}, fakeImages{}, fakePubProvider{},
		nil,
		jobrunner.NewSyncRunner(),
		apparticles.Defaults{MinWords: 500, MaxWords: 1000, Language: "en", Provider: "groq", Model: "m", AIThreshold: 0.8, MaxCycles: 2, SERPLimit: 5},
		nil,
	)

	res, err := svc.Generate(context.Background(), apparticles.GenerateRequest{Keyword: "test keyword", SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	stored, err := repo.Get(context.Background(), res.Article.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Status != articles.StatusDraft {
		t.Errorf("status = %s, want draft", stored.Status)
	}
	if stored.WPPostID != 123 {
		t.Errorf("WPPostID = %d, want 123", stored.WPPostID)
	}
}

func TestGenerate_NoClusterAborts(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	svc := apparticles.NewService(
		repo, fakeLLMFactory{}, fakeSERP{}, emptyTopics{}, fakeChecker{}, fakeImages{}, fakePubProvider{},
		nil, jobrunner.NewSyncRunner(), apparticles.Defaults{Language: "en", Provider: "groq", Model: "m"}, nil,
	)
	if _, err := svc.Generate(context.Background(), apparticles.GenerateRequest{Keyword: "unknown", SiteID: uuid.New()}); err == nil {
		t.Fatal("expected error when no cluster found")
	}
}

type emptyTopics struct{}

func (emptyTopics) Lookup(context.Context, string) (articles.Cluster, error) {
	return articles.Cluster{}, nil
}
func (emptyTopics) Topics(context.Context) ([]string, error) { return nil, nil }
func (emptyTopics) Clusters(context.Context) (map[string]articles.Cluster, error) {
	return nil, nil
}
