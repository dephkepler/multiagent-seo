package sheets

import (
	"context"
	"strings"
)

// NewMock returns an in-memory Client used when Google credentials are missing.
func NewMock() Client {
	return &mockClient{
		data: map[string]Result{
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
	data map[string]Result
}

func (m *mockClient) Lookup(_ context.Context, topic string) (Result, error) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	r, ok := m.data[topic]
	if !ok {
		return Result{}, nil
	}
	// Defensive copy so callers can't mutate the mock's state.
	cp := Result{Title: r.Title, Keywords: make([]string, len(r.Keywords))}
	copy(cp.Keywords, r.Keywords)
	return cp, nil
}
