package prompt

import (
	"fmt"

	"contentflow/internal/prompt/rules"
)

func Writer(brief, keyword, language string, minWords, maxWords int, cluster Cluster) string {
	return fmt.Sprintf(`You are an SEO copywriter. Write a full article based on the brief below.

PRIMARY KEYWORD: "%s"
LANGUAGE: %s
WORD COUNT: %d to %d words (strictly follow this range)

BRIEF:
%s

Follow these rules:

%s

%s
Write the complete article now.`,
		keyword,
		language,
		minWords,
		maxWords,
		brief,
		rules.DefaultSEO().Render(),
		cluster.Render(),
	)
}
