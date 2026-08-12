package articles

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/pkg/jobrunner"
)

type internalLLM struct{}

func (internalLLM) Complete(context.Context, string, int) (string, articles.Usage, error) {
	return "", articles.Usage{}, nil
}

type internalLLMFactory struct{}

func (internalLLMFactory) ForModel(string, string) (articles.LLMClient, error) {
	return internalLLM{}, nil
}

type lookupPanicTopics struct{}

func (lookupPanicTopics) Lookup(context.Context, string) (articles.Cluster, error) {
	panic("Lookup must not be called when req.cluster is pre-resolved")
}
func (lookupPanicTopics) Topics(context.Context) ([]string, error) { return nil, nil }
func (lookupPanicTopics) ForSite(context.Context, uuid.UUID, string) (articles.TopicSource, error) {
	return lookupPanicTopics{}, nil
}
func (lookupPanicTopics) Clusters(context.Context) (map[string]articles.Cluster, error) {
	return nil, nil
}

func TestResolve_PreResolvedClusterUsedVerbatim(t *testing.T) {
	svc := NewService(
		nil,
		internalLLMFactory{},
		nil,
		lookupPanicTopics{},
		nil,
		nil,
		nil,
		nil,
		jobrunner.NewSyncRunner(),
		Defaults{Language: "en", Provider: "groq", Model: "m"},
		nil,
	)

	cl := &articles.Cluster{Title: "CLUSTER-TITLE", Keywords: []string{"topic-a", "cluster-kw", "third"}}
	settings, err := svc.resolve(context.Background(), GenerateRequest{
		Keyword: "topic-a",
		SiteID:  uuid.New(),
		cluster: cl,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if settings.cluster.Title != "CLUSTER-TITLE" {
		t.Errorf("cluster.Title = %q, want CLUSTER-TITLE", settings.cluster.Title)
	}
	if len(settings.cluster.Keywords) != 3 {
		t.Errorf("len(cluster.Keywords) = %d, want 3", len(settings.cluster.Keywords))
	}
}
