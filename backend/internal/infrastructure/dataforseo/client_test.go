package dataforseo

import "testing"

func TestParseSERPResponse_ToleratesMixedNestedItems(t *testing.T) {
	body := []byte(`{"tasks":[{"result":[{"items":[
		{"type":"organic","rank_absolute":1,"url":"https://a.com","title":"A","description":"da"},
		{"type":"organic","rank_absolute":2,"url":"https://b.com","title":"B","description":"db"},
		{"type":"people_also_ask","items":[{"type":"paa_element","title":"Q1?","description":"a1"},{"title":"Q2?"},"Q3?"]},
		{"type":"related_searches","items":["foo","bar"]},
		{"type":"featured_snippet","items":[{"title":"Snip","description":"snip desc"}]}
	]}]}]}`)

	data, err := parseSERPResponse(body, "kw", 5)
	if err != nil {
		t.Fatalf("parseSERPResponse: %v", err)
	}
	if len(data.Results) != 2 {
		t.Errorf("organic results = %d, want 2", len(data.Results))
	}
	if len(data.PAA) != 3 {
		t.Errorf("PAA = %v, want 3 entries", data.PAA)
	}
	if data.FeaturedSnippet == nil || data.FeaturedSnippet.Title != "Snip" {
		t.Errorf("featured snippet = %+v, want title \"Snip\"", data.FeaturedSnippet)
	}
}

func TestParseSERPResponse_RespectsLimit(t *testing.T) {
	body := []byte(`{"tasks":[{"result":[{"items":[
		{"type":"organic","rank_absolute":1,"url":"u1","title":"t1"},
		{"type":"organic","rank_absolute":2,"url":"u2","title":"t2"},
		{"type":"organic","rank_absolute":3,"url":"u3","title":"t3"}
	]}]}]}`)

	data, err := parseSERPResponse(body, "kw", 2)
	if err != nil {
		t.Fatalf("parseSERPResponse: %v", err)
	}
	if len(data.Results) != 2 {
		t.Errorf("organic results = %d, want 2 (limit)", len(data.Results))
	}
}

func TestParseSERPResponse_EmptyTasks(t *testing.T) {
	data, err := parseSERPResponse([]byte(`{"tasks":[]}`), "kw", 5)
	if err != nil {
		t.Fatalf("parseSERPResponse: %v", err)
	}
	if len(data.Results) != 0 || data.Keyword != "kw" {
		t.Errorf("unexpected data: %+v", data)
	}
}
