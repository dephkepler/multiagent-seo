package sheets

import (
	"context"
	"strings"
)

// NewMock returns an in-memory Client with a few canned topic→Result entries.
// Used when Google credentials are not configured (dev / local runs).
func NewMock() Client {
	return &mockClient{
		data: map[string]Result{
			"как выбрать кофемашину для дома": {
				Keywords: []string{"рожковая кофемашина", "капсульная кофемашина", "лучшая кофемашина 2026"},
				Title:    "Как выбрать кофемашину для дома: подробный гид",
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
	// Return a defensive copy so callers can't mutate the mock's state.
	cp := Result{Title: r.Title, Keywords: make([]string, len(r.Keywords))}
	copy(cp.Keywords, r.Keywords)
	return cp, nil
}
