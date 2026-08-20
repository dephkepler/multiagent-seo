package articles_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	apparticles "multiagent-seo/internal/application/articles"
	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/jobrunner"
)

func batchDefaults() apparticles.Defaults {
	return apparticles.Defaults{
		MinWords:    500,
		MaxWords:    1000,
		Language:    "en",
		Provider:    "groq",
		Model:       "m",
		AIThreshold: 0.8,
		MaxCycles:   2,
		SERPLimit:   5,
	}
}

func newBatchService(repo *fakeRepo, topics articles.TopicSourceProvider) *apparticles.Service {
	return apparticles.NewService(
		repo,
		fakeLLMFactory{},
		fakeSERP{},
		topics,
		fakeChecker{},
		fakeImages{},
		fakePubProvider{},
		nil,
		jobrunner.NewSyncRunner(),
		batchDefaults(),
		nil,
	)
}

// configTopics is a TopicSource whose topic list and cluster map are set per
// test, exercising normalization and missing-cluster fallback. Lookup yields a
// cluster only for keys present in the lookup map; unknown keys return empty.
type configTopics struct {
	topics   []string
	clusters map[string]articles.Cluster
	lookup   map[string]articles.Cluster
}

func (c configTopics) Topics(context.Context) ([]string, error) { return c.topics, nil }
func (c configTopics) Clusters(context.Context) (map[string]articles.Cluster, error) {
	return c.clusters, nil
}
func (c configTopics) Lookup(_ context.Context, topic string) (articles.Cluster, error) {
	if cl, ok := c.lookup[strings.ToLower(strings.TrimSpace(topic))]; ok {
		return cl, nil
	}
	return articles.Cluster{}, nil
}
func (c configTopics) ForSite(context.Context, uuid.UUID, string) (articles.TopicSource, error) {
	return c, nil
}

// spyTopics records whether Lookup was invoked. Lookup returns a deliberately
// distinct cluster so a test can prove the pre-resolved cluster (not Lookup's)
// was used.
type spyTopics struct {
	topics       []string
	clusters     map[string]articles.Cluster
	lookupResult articles.Cluster
	lookupCalled bool
}

func (s *spyTopics) Topics(context.Context) ([]string, error) { return s.topics, nil }
func (s *spyTopics) Clusters(context.Context) (map[string]articles.Cluster, error) {
	return s.clusters, nil
}
func (s *spyTopics) Lookup(context.Context, string) (articles.Cluster, error) {
	s.lookupCalled = true
	return s.lookupResult, nil
}
func (s *spyTopics) ForSite(context.Context, uuid.UUID, string) (articles.TopicSource, error) {
	return s, nil
}

// errTopics injects errors from each TopicSource method independently.
type errTopics struct {
	topics      []string
	clusters    map[string]articles.Cluster
	topicsErr   error
	clustersErr error
}

func (e errTopics) Topics(context.Context) ([]string, error) { return e.topics, e.topicsErr }
func (e errTopics) Clusters(context.Context) (map[string]articles.Cluster, error) {
	return e.clusters, e.clustersErr
}
func (e errTopics) Lookup(_ context.Context, topic string) (articles.Cluster, error) {
	return articles.Cluster{Keywords: []string{topic, "secondary"}, Title: "Title"}, nil
}
func (e errTopics) ForSite(context.Context, uuid.UUID, string) (articles.TopicSource, error) {
	return e, nil
}

// createErrRepo fails Create for one keyword and delegates the rest to fakeRepo.
type createErrRepo struct {
	*fakeRepo
	failOn string
	err    error
}

func (r *createErrRepo) Create(ctx context.Context, in articles.CreateArticle) (articles.Article, error) {
	if in.Keyword == r.failOn {
		return articles.Article{}, r.err
	}
	return r.fakeRepo.Create(ctx, in)
}

// genKwErrRepo fails GeneratedKeywords and delegates the rest to fakeRepo.
type genKwErrRepo struct {
	*fakeRepo
	err error
}

func (r *genKwErrRepo) GeneratedKeywords(context.Context, uuid.UUID) ([]string, error) {
	return nil, r.err
}

func keywordsOf(repo *fakeRepo, ids []int64) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if a, ok := repo.arts[id]; ok {
			out = append(out, a.Keyword)
		}
	}
	return out
}

func sortedKeywords(repo *fakeRepo, ids []int64) []string {
	out := keywordsOf(repo, ids)
	sort.Strings(out)
	return out
}

func TestGenerateBatch_QueuesNAndReturnsCreatedIds(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	svc := newBatchService(repo, fakeTopics{})
	siteID := uuid.New()

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: siteID})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2", len(ids))
	}
	if ids[0] == ids[1] || ids[0] == 0 || ids[1] == 0 {
		t.Fatalf("ids not unique/non-zero: %v", ids)
	}
	for _, id := range ids {
		stored, err := repo.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("Get(%d): %v", id, err)
		}
		if stored.Status != articles.StatusDraft {
			t.Errorf("id %d status = %s, want draft", id, stored.Status)
		}
		if stored.WPPostID != 123 {
			t.Errorf("id %d WPPostID = %d, want 123", id, stored.WPPostID)
		}
		if stored.SiteID != siteID {
			t.Errorf("id %d SiteID = %s, want %s", id, stored.SiteID, siteID)
		}
	}
	if got, want := keywordsOf(repo, ids), []string{"topic-a", "topic-b"}; !equalStrings(got, want) {
		t.Errorf("keywords = %v, want %v", got, want)
	}
}

func TestGenerateBatch_ReusesClusterAndSkipsLookup(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	spy := &spyTopics{
		topics: []string{"topic-a", "topic-b"},
		clusters: map[string]articles.Cluster{
			"topic-a": {Title: "CLUSTER-TITLE", Keywords: []string{"topic-a", "cluster-kw"}},
			"topic-b": {Title: "CLUSTER-TITLE", Keywords: []string{"topic-b", "cluster-kw"}},
		},
		lookupResult: articles.Cluster{Title: "LOOKUP-TITLE", Keywords: []string{"lookup-kw"}},
	}
	svc := newBatchService(repo, spy)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2", len(ids))
	}
	if spy.lookupCalled {
		t.Error("Lookup was called; pre-resolved cluster should bypass it")
	}
	for _, id := range ids {
		if repo.arts[id].Status != articles.StatusDraft {
			t.Errorf("id %d status = %s, want draft", id, repo.arts[id].Status)
		}
	}
}

func TestGenerateBatch_UsesPreResolvedClusterNotLookup(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	spy := &spyTopics{
		topics: []string{"topic-a"},
		clusters: map[string]articles.Cluster{
			"topic-a": {Title: "CLUSTER-TITLE", Keywords: []string{"topic-a", "cluster-kw"}},
		},
		lookupResult: articles.Cluster{Title: "LOOKUP-TITLE", Keywords: []string{"lookup-kw"}},
	}
	svc := newBatchService(repo, spy)

	ids, err := svc.GenerateBatch(context.Background(), 1, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("len(ids) = %d, want 1", len(ids))
	}
	if spy.lookupCalled {
		t.Error("Lookup was called; resolve must take the req.cluster!=nil branch")
	}
}

func TestGenerateBatch_SkipsAlreadyDoneTopics(t *testing.T) {
	siteID := uuid.New()
	repo := &fakeRepo{
		arts: map[int64]*articles.Article{
			1: {ID: 1, Keyword: "  TOPIC-A ", SiteID: siteID, Status: articles.StatusDraft},
		},
		next: 1,
	}
	svc := newBatchService(repo, fakeTopics{})

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: siteID})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("len(ids) = %d, want 1", len(ids))
	}
	if repo.arts[ids[0]].Keyword != "topic-b" {
		t.Errorf("new keyword = %q, want topic-b", repo.arts[ids[0]].Keyword)
	}
	if ids[0] == 1 {
		t.Error("returned id includes the pre-existing article")
	}
}

func TestGenerateBatch_NormalizesTopicAndStoredKeywordCaseAndWhitespace(t *testing.T) {
	siteID := uuid.New()
	repo := &fakeRepo{
		arts: map[int64]*articles.Article{
			1: {ID: 1, Keyword: "  Topic-A  ", SiteID: siteID, Status: articles.StatusDraft},
		},
		next: 1,
	}
	topics := configTopics{
		topics: []string{"TOPIC-a", "topic-b"},
		clusters: map[string]articles.Cluster{
			"topic-a": {Keywords: []string{"topic-a", "secondary"}, Title: "Title"},
			"topic-b": {Keywords: []string{"topic-b", "secondary"}, Title: "Title"},
		},
	}
	svc := newBatchService(repo, topics)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: siteID})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("len(ids) = %d, want 1", len(ids))
	}
	if repo.arts[ids[0]].Keyword != "topic-b" {
		t.Errorf("new keyword = %q, want topic-b", repo.arts[ids[0]].Keyword)
	}
}

func TestGenerateBatch_DoneSetScopedToSite(t *testing.T) {
	otherSite := uuid.New()
	targetSite := uuid.New()
	repo := &fakeRepo{
		arts: map[int64]*articles.Article{
			1: {ID: 1, Keyword: "topic-a", SiteID: otherSite, Status: articles.StatusDraft},
		},
		next: 1,
	}
	svc := newBatchService(repo, fakeTopics{})

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: targetSite})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2 (otherSite topic-a must not suppress)", len(ids))
	}
	if got, want := sortedKeywords(repo, ids), []string{"topic-a", "topic-b"}; !equalStrings(got, want) {
		t.Errorf("keywords = %v, want %v", got, want)
	}
	for _, id := range ids {
		if repo.arts[id].SiteID != targetSite {
			t.Errorf("id %d SiteID = %s, want targetSite", id, repo.arts[id].SiteID)
		}
	}
}

func TestGenerateBatch_FailedArticlesNotTreatedAsDone(t *testing.T) {
	siteID := uuid.New()
	repo := &fakeRepo{
		arts: map[int64]*articles.Article{
			1: {ID: 1, Keyword: "topic-a", SiteID: siteID, Status: articles.StatusFailed},
		},
		next: 1,
	}
	svc := newBatchService(repo, fakeTopics{})

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: siteID})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2 (failed topic-a must be retried)", len(ids))
	}
	var freshTopicA bool
	for _, id := range ids {
		a := repo.arts[id]
		if a.Keyword == "topic-a" && a.Status != articles.StatusFailed {
			freshTopicA = true
		}
	}
	if !freshTopicA {
		t.Error("expected a fresh non-failed topic-a article")
	}
}

func TestGenerateBatch_NonPositiveCountReturnsNil(t *testing.T) {
	for _, count := range []int{0, -1, -100} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			repo := &fakeRepo{arts: map[int64]*articles.Article{}}
			svc := newBatchService(repo, fakeTopics{})

			ids, err := svc.GenerateBatch(context.Background(), count, apparticles.GenerateRequest{SiteID: uuid.New()})
			if err != nil {
				t.Fatalf("GenerateBatch: %v", err)
			}
			if ids != nil {
				t.Errorf("ids = %v, want nil", ids)
			}
			if repo.next != 0 {
				t.Errorf("repo.next = %d, want 0 (no Create)", repo.next)
			}
		})
	}
}

func manyTopics(n int) configTopics {
	ts := make([]string, n)
	cls := make(map[string]articles.Cluster, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("topic-%03d", i)
		ts[i] = name
		cls[name] = articles.Cluster{Keywords: []string{name, "secondary"}, Title: "Title"}
	}
	return configTopics{topics: ts, clusters: cls}
}

func TestGenerateBatch_ClampsToMaxBatchCount(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	svc := newBatchService(repo, manyTopics(150))

	ids, err := svc.GenerateBatch(context.Background(), 1000, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 100 {
		t.Fatalf("len(ids) = %d, want 100 (clamped)", len(ids))
	}
	if repo.next != 100 {
		t.Errorf("repo.next = %d, want 100", repo.next)
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %d", id)
		}
		seen[id] = true
	}
}

func TestGenerateBatch_ExactlyMaxBatchCountNotClamped(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	svc := newBatchService(repo, manyTopics(100))

	ids, err := svc.GenerateBatch(context.Background(), 100, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 100 {
		t.Fatalf("len(ids) = %d, want 100", len(ids))
	}
}

func TestGenerateBatch_StopsAtCountWhenTopicsExceed(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	topics := configTopics{
		topics: []string{"topic-a", "topic-b", "topic-c"},
		clusters: map[string]articles.Cluster{
			"topic-a": {Keywords: []string{"topic-a", "x"}, Title: "Title"},
			"topic-b": {Keywords: []string{"topic-b", "x"}, Title: "Title"},
			"topic-c": {Keywords: []string{"topic-c", "x"}, Title: "Title"},
		},
	}
	svc := newBatchService(repo, topics)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2", len(ids))
	}
	if repo.next != 2 {
		t.Errorf("repo.next = %d, want 2", repo.next)
	}
	if got, want := keywordsOf(repo, ids), []string{"topic-a", "topic-b"}; !equalStrings(got, want) {
		t.Errorf("keywords = %v, want %v", got, want)
	}
}

func TestGenerateBatch_SkippedKeywordDoesNotConsumeCountBudget(t *testing.T) {
	siteID := uuid.New()
	repo := &fakeRepo{
		arts: map[int64]*articles.Article{
			1: {ID: 1, Keyword: "topic-a", SiteID: siteID, Status: articles.StatusDraft},
		},
		next: 1,
	}
	topics := configTopics{
		topics: []string{"topic-a", "topic-b", "topic-c"},
		clusters: map[string]articles.Cluster{
			"topic-a": {Keywords: []string{"topic-a", "x"}, Title: "Title"},
			"topic-b": {Keywords: []string{"topic-b", "x"}, Title: "Title"},
			"topic-c": {Keywords: []string{"topic-c", "x"}, Title: "Title"},
		},
	}
	svc := newBatchService(repo, topics)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: siteID})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2", len(ids))
	}
	if got, want := sortedKeywords(repo, ids), []string{"topic-b", "topic-c"}; !equalStrings(got, want) {
		t.Errorf("keywords = %v, want %v", got, want)
	}
}

func TestGenerateBatch_ShortfallReturnsFewerWithoutError(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	svc := newBatchService(repo, fakeTopics{})

	ids, err := svc.GenerateBatch(context.Background(), 10, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2 (only 2 topics available)", len(ids))
	}
	if got, want := keywordsOf(repo, ids), []string{"topic-a", "topic-b"}; !equalStrings(got, want) {
		t.Errorf("keywords = %v, want %v", got, want)
	}
}

func TestGenerateBatch_SkipsTopicWithoutClusterAndContinues(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	topics := configTopics{
		topics: []string{"topic-a", "orphan", "topic-b"},
		clusters: map[string]articles.Cluster{
			"topic-a": {Keywords: []string{"topic-a", "x"}, Title: "Title"},
			"topic-b": {Keywords: []string{"topic-b", "x"}, Title: "Title"},
		},
		lookup: map[string]articles.Cluster{}, // orphan -> empty -> ErrNoCluster
	}
	svc := newBatchService(repo, topics)

	ids, err := svc.GenerateBatch(context.Background(), 3, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2 (orphan skipped)", len(ids))
	}
	if got, want := sortedKeywords(repo, ids), []string{"topic-a", "topic-b"}; !equalStrings(got, want) {
		t.Errorf("keywords = %v, want %v", got, want)
	}
	for _, a := range repo.arts {
		if a.Keyword == "orphan" {
			t.Error("orphan should not have produced an article")
		}
	}
}

func TestGenerateBatch_TopicMissingFromClustersFallsBackToLookupAndSkips(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	spy := &spyTopics{
		topics: []string{"topic-a", "orphan"},
		clusters: map[string]articles.Cluster{
			"topic-a": {Title: "T", Keywords: []string{"topic-a", "kw"}},
		},
		lookupResult: articles.Cluster{}, // empty -> ErrNoCluster
	}
	svc := newBatchService(repo, spy)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("len(ids) = %d, want 1", len(ids))
	}
	if !spy.lookupCalled {
		t.Error("expected Lookup fallback for orphan missing from Clusters()")
	}
	if repo.arts[ids[0]].Keyword != "topic-a" {
		t.Errorf("keyword = %q, want topic-a", repo.arts[ids[0]].Keyword)
	}
}

func TestGenerateBatch_AllTopicsMissingFromClusters_ZeroQueuedNoError(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	spy := &spyTopics{
		topics:       []string{"a", "b"},
		clusters:     map[string]articles.Cluster{},
		lookupResult: articles.Cluster{},
	}
	svc := newBatchService(repo, spy)

	ids, err := svc.GenerateBatch(context.Background(), 5, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("len(ids) = %d, want 0", len(ids))
	}
	if !spy.lookupCalled {
		t.Error("expected Lookup fallback")
	}
}

func TestGenerateBatch_SkipsTopicWithEmptyCluster(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	topics := configTopics{
		topics: []string{"topic-a", "topic-b"},
		clusters: map[string]articles.Cluster{
			"topic-a": {}, // present but empty -> ErrNoCluster without Lookup
			"topic-b": {Keywords: []string{"topic-b", "secondary"}, Title: "Title"},
		},
	}
	svc := newBatchService(repo, topics)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("len(ids) = %d, want 1", len(ids))
	}
	if repo.arts[ids[0]].Keyword != "topic-b" {
		t.Errorf("keyword = %q, want topic-b", repo.arts[ids[0]].Keyword)
	}
	for _, a := range repo.arts {
		if a.Keyword == "topic-a" {
			t.Error("topic-a with empty cluster should not produce an article")
		}
	}
}

func TestGenerateBatch_SkipsTopicOnCreateError(t *testing.T) {
	repo := &createErrRepo{
		fakeRepo: &fakeRepo{arts: map[int64]*articles.Article{}},
		failOn:   "topic-a",
		err:      errors.New("boom"),
	}
	svc := apparticles.NewService(
		repo,
		fakeLLMFactory{},
		fakeSERP{},
		fakeTopics{},
		fakeChecker{},
		fakeImages{},
		fakePubProvider{},
		nil,
		jobrunner.NewSyncRunner(),
		batchDefaults(),
		nil,
	)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("len(ids) = %d, want 1 (topic-a Create failed)", len(ids))
	}
	if repo.arts[ids[0]].Keyword != "topic-b" {
		t.Errorf("keyword = %q, want topic-b", repo.arts[ids[0]].Keyword)
	}
}

func TestGenerateBatch_ContinuesPastFailureToFillCount(t *testing.T) {
	inner := &fakeRepo{arts: map[int64]*articles.Article{}}
	repo := &createErrRepo{fakeRepo: inner, failOn: "topic-b", err: errors.New("boom")}
	topics := configTopics{
		topics: []string{"topic-a", "topic-b", "topic-c"},
		clusters: map[string]articles.Cluster{
			"topic-a": {Keywords: []string{"topic-a", "x"}, Title: "Title"},
			"topic-b": {Keywords: []string{"topic-b", "x"}, Title: "Title"},
			"topic-c": {Keywords: []string{"topic-c", "x"}, Title: "Title"},
		},
	}
	svc := apparticles.NewService(
		repo,
		fakeLLMFactory{},
		fakeSERP{},
		topics,
		fakeChecker{},
		fakeImages{},
		fakePubProvider{},
		nil,
		jobrunner.NewSyncRunner(),
		batchDefaults(),
		nil,
	)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("len(ids) = %d, want 2", len(ids))
	}
	if got, want := keywordsOf(inner, ids), []string{"topic-a", "topic-c"}; !equalStrings(got, want) {
		t.Errorf("keywords = %v, want %v", got, want)
	}
}

func TestGenerateBatch_TopicsErrorPropagates(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	sentinel := errors.New("sheet down")
	svc := newBatchService(repo, errTopics{topicsErr: sentinel})

	ids, err := svc.GenerateBatch(context.Background(), 5, apparticles.GenerateRequest{SiteID: uuid.New()})
	if ids != nil {
		t.Errorf("ids = %v, want nil", ids)
	}
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "list topics") {
		t.Fatalf("err = %v, want wrapping %q with 'list topics'", err, sentinel)
	}
	if repo.next != 0 {
		t.Errorf("repo.next = %d, want 0", repo.next)
	}
}

func TestGenerateBatch_ClustersErrorPropagates(t *testing.T) {
	repo := &fakeRepo{arts: map[int64]*articles.Article{}}
	sentinel := errors.New("clusters fetch failed")
	svc := newBatchService(repo, errTopics{topics: []string{"topic-a"}, clustersErr: sentinel})

	ids, err := svc.GenerateBatch(context.Background(), 5, apparticles.GenerateRequest{SiteID: uuid.New()})
	if ids != nil {
		t.Errorf("ids = %v, want nil", ids)
	}
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "list clusters") {
		t.Fatalf("err = %v, want wrapping %q with 'list clusters'", err, sentinel)
	}
	if repo.next != 0 {
		t.Errorf("repo.next = %d, want 0", repo.next)
	}
}

func TestGenerateBatch_GeneratedKeywordsErrorAborts(t *testing.T) {
	inner := &fakeRepo{arts: map[int64]*articles.Article{}}
	sentinel := errors.New("db down")
	repo := &genKwErrRepo{fakeRepo: inner, err: sentinel}
	svc := apparticles.NewService(
		repo,
		fakeLLMFactory{},
		fakeSERP{},
		fakeTopics{},
		fakeChecker{},
		fakeImages{},
		fakePubProvider{},
		nil,
		jobrunner.NewSyncRunner(),
		batchDefaults(),
		nil,
	)

	ids, err := svc.GenerateBatch(context.Background(), 2, apparticles.GenerateRequest{SiteID: uuid.New()})
	if ids != nil {
		t.Errorf("ids = %v, want nil", ids)
	}
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "existing keywords") {
		t.Fatalf("err = %v, want wrapping %q with 'existing keywords'", err, sentinel)
	}
	if inner.next != 0 {
		t.Errorf("inner.next = %d, want 0", inner.next)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
