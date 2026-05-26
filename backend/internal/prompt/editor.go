package prompt

import (
	"fmt"

	"contentflow/internal/prompt/rules"
)

func Editor(article, keyword string, minWords, maxWords int, cluster Cluster, competitors Competitors) string {
	return fmt.Sprintf(`You are an SEO editor. Review and improve the article below without changing its meaning or structure.

PRIMARY KEYWORD: "%s"
REQUIRED WORD COUNT: %d to %d words

%s
ARTICLE:
%s

Fix everything that does not comply with these rules:

%s

%s
Return the fully corrected article only. No explanations.`,
		keyword,
		minWords,
		maxWords,
		competitors.RenderSlim(),
		article,
		rules.EditorChecklist().Render(),
		cluster.Render(),
	)
}
