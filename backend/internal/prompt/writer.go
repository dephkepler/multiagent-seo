package prompt

import (
	"fmt"

	"multiagent-seo/internal/prompt/rules"
)

func Writer(brief, keyword, language string, minWords, maxWords int, cluster Cluster, competitors Competitors) string {
	return fmt.Sprintf(`You are an experienced SEO copywriter. You write articles that rank in Google's top 3, read as genuine expert content written by a human, and pass AI detectors. Every article you write is built around a clear unique angle — a perspective or depth that goes beyond surface-level coverage of the topic.

PRIMARY KEYWORD: "%s"
LANGUAGE: %s
WORD COUNT: %d to %d words (strictly follow this range; never write less than needed)

%s
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
		competitors.RenderSlim(),
		brief,
		rules.DefaultSEO().Render(),
		cluster.Render(),
	)
}
