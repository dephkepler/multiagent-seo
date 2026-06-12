package prompt

import (
	"strings"
	"testing"

	"multiagent-seo/internal/domain/articles"
)

func TestWriterTemplateSplit(t *testing.T) {
	frozen, evolving := SplitWriter(WriterTemplate)

	if !strings.Contains(evolving, "experienced SEO copywriter") {
		t.Errorf("evolving block should hold the style framing, got: %q", evolving)
	}
	if !strings.Contains(frozen, "{{rules}}") {
		t.Error("frozen block should hold the structural placeholders incl. {{rules}}")
	}
	if strings.Contains(evolving, styleMarker) || strings.Contains(frozen, styleMarker) {
		t.Error("marker must not appear inside either block")
	}
}

func TestRenderStripsMarker(t *testing.T) {
	out := Writer("brief text", "running shoes", "en", 1000, 1500, articles.Cluster{}, Competitors{})

	if strings.Contains(out, styleMarker) {
		t.Error("rendered prompt must not contain the style marker")
	}
	if !strings.Contains(out, "running shoes") {
		t.Error("rendered prompt should contain the keyword")
	}
}
