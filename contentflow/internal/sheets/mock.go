package sheets

import "context"

type mockClient struct {
	keywords []string
}

func NewMock() Client {
	return &mockClient{
		keywords: []string{
			"как выбрать кофемашину для дома",
			"лучшие кофемашины 2024",
			"рожковая vs капсульная кофемашина",
		},
	}
}

func (m *mockClient) FetchKeywords(_ context.Context) ([]string, error) {
	return m.keywords, nil
}
