package prompt

import (
	"fmt"

	"contentflow/internal/prompt/rules"
)

func Editor(article, keyword string, minWords, maxWords int, cluster Cluster) string {
	return fmt.Sprintf(`You are an SEO editor. Review and improve the article below without changing its meaning or structure.

PRIMARY KEYWORD: "%s"
REQUIRED WORD COUNT: %d to %d words

ARTICLE:
%s

Fix everything that does not comply with these rules:

%s

%s
Return the fully corrected article only. No explanations.`,
		keyword,
		minWords,
		maxWords,
		article,
		rules.DefaultSEO().Render(),
		cluster.Render(),
	)
}
