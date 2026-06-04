package sheets

import (
	"context"
	"strings"

	"multiagent-seo/internal/domain/articles"
)

func NewMock() articles.TopicSource {
	return &mockClient{
		data: map[string]articles.Cluster{
			"android game development services": {
				Keywords: []string{
					"Android Game Development",
					"Android Game Development Services",
					"Android Game Development Company",
					"Android Game App Development",
					"Android Game Development Studio",
				},
				Title: "Android Game Development Services",
			},
			"как выбрать ноутбук": {
				Keywords: []string{"игровой ноутбук", "лёгкий ультрабук", "лучший ноутбук 2026"},
				Title:    "Как выбрать ноутбук: полное руководство",
			},
		},
	}
}

type mockClient struct {
	data map[string]articles.Cluster
}

func (m *mockClient) Lookup(_ context.Context, topic string) (articles.Cluster, error) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	r, ok := m.data[topic]
	if !ok {
		return articles.Cluster{}, nil
	}
	cp := articles.Cluster{Title: r.Title, Keywords: make([]string, len(r.Keywords))}
	copy(cp.Keywords, r.Keywords)
	return cp, nil
}
