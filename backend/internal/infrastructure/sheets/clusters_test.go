package sheets

import (
	"context"
	"strings"
	"testing"
)

func TestMockClustersDeepCopyIsolatesState(t *testing.T) {
	src := NewMock()
	ctx := context.Background()

	m1, err := src.Clusters(ctx)
	if err != nil {
		t.Fatalf("Clusters() error = %v", err)
	}

	const androidKey = "android game development services"
	c := m1[androidKey]
	c.Keywords = append(c.Keywords, "SENTINEL")
	m1[androidKey] = c

	m2, err := src.Clusters(ctx)
	if err != nil {
		t.Fatalf("Clusters() error = %v", err)
	}

	if got := len(m2[androidKey].Keywords); got != 5 {
		t.Errorf("m2[%q] has %d keywords, want 5 (mock state corrupted)", androidKey, got)
	}
	for _, kw := range m2[androidKey].Keywords {
		if kw == "SENTINEL" {
			t.Errorf("m2[%q] leaked SENTINEL keyword from m1 mutation", androidKey)
		}
	}
	if got := len(m2["как выбрать ноутбук"].Keywords); got != 3 {
		t.Errorf("m2[ноутбук] has %d keywords, want 3", got)
	}
}

func TestMockClustersKeysMatchLookupNormalization(t *testing.T) {
	src := NewMock()
	ctx := context.Background()

	m, err := src.Clusters(ctx)
	if err != nil {
		t.Fatalf("Clusters() error = %v", err)
	}

	for key, want := range m {
		if norm := strings.ToLower(strings.TrimSpace(key)); key != norm {
			t.Errorf("Clusters key %q is not normalized, want %q", key, norm)
		}

		variants := []string{key, strings.ToUpper(key), "  " + key + "  "}
		for _, v := range variants {
			got, err := src.Lookup(ctx, v)
			if err != nil {
				t.Fatalf("Lookup(%q) error = %v", v, err)
			}
			if got.Title != want.Title {
				t.Errorf("Lookup(%q).Title = %q, want %q", v, got.Title, want.Title)
			}
			if !equalStrings(got.Keywords, want.Keywords) {
				t.Errorf("Lookup(%q).Keywords = %v, want %v", v, got.Keywords, want.Keywords)
			}
		}
	}
}

func TestMockClustersReturnsAllTopics(t *testing.T) {
	src := NewMock()
	ctx := context.Background()

	m, err := src.Clusters(ctx)
	if err != nil {
		t.Fatalf("Clusters() error = %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("len(Clusters) = %d, want 2", len(m))
	}

	for _, key := range []string{"android game development services", "как выбрать ноутбук"} {
		c, ok := m[key]
		if !ok {
			t.Errorf("Clusters missing key %q", key)
			continue
		}
		if c.Title == "" {
			t.Errorf("Clusters[%q].Title is empty", key)
		}
	}

	topics, err := src.Topics(ctx)
	if err != nil {
		t.Fatalf("Topics() error = %v", err)
	}
	if len(topics) != len(m) {
		t.Errorf("len(Topics) = %d, len(Clusters) = %d, want equal", len(topics), len(m))
	}
	for _, topic := range topics {
		if _, ok := m[topic]; !ok {
			t.Errorf("topic %q from Topics() has no cluster in Clusters()", topic)
		}
	}
}

func TestMockClustersValuesAreDistinctSlices(t *testing.T) {
	src := NewMock()
	ctx := context.Background()

	m1, err := src.Clusters(ctx)
	if err != nil {
		t.Fatalf("Clusters() error = %v", err)
	}
	m2, err := src.Clusters(ctx)
	if err != nil {
		t.Fatalf("Clusters() error = %v", err)
	}

	const k = "android game development services"
	orig := m2[k].Keywords[0]

	c := m1[k]
	c.Keywords[0] = "MUTATED"
	m1[k] = c

	if got := m2[k].Keywords[0]; got != orig {
		t.Errorf("m2[%q].Keywords[0] = %q, want %q (calls share a backing array)", k, got, orig)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  Foo Bar  ", "foo bar"},
		{"UPPER", "upper"},
		{"", ""},
		{"   ", ""},
		{"Как Выбрать", "как выбрать"},
		{"already", "already"},
		{"\tTab\n", "tab"},
	}
	for _, c := range cases {
		if got := normalize(c.in); got != c.want {
			t.Errorf("normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestColumnOffset(t *testing.T) {
	cases := []struct {
		base string
		col  string
		want int
	}{
		{"A", "C", 2},
		{"A", "A", 0},
		{"B", "D", 2},
		{"a", "c", 2},
		{"A", "", -1},
		{"", "C", -1},
		{"AA", "C", -1},
		{"A", "CD", -1},
		{"C", "A", -2},
	}
	for _, c := range cases {
		if got := columnOffset(c.base, c.col); got != c.want {
			t.Errorf("columnOffset(%q, %q) = %d, want %d", c.base, c.col, got, c.want)
		}
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
