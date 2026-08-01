package prompt

import (
	"strconv"
	"strings"

	"multiagent-seo/internal/domain/articles"
	"multiagent-seo/internal/domain/articles/prompt/rules"
)

const WriterTemplate = `You are an experienced SEO copywriter.
You write articles that rank in Google's top 3,
read as genuine expert content written by a human,
and pass AI detectors. Every article you write is built
around a clear unique angle — a perspective or depth
that goes beyond surface-level coverage of the topic.

` + styleMarker + `

PRIMARY KEYWORD: "{{keyword}}"
LANGUAGE: {{language}}
WORD COUNT: {{min_words}} to {{max_words}}
words (strictly follow this range; never write less than needed)

{{competitors}}
BRIEF:
{{brief}}

Follow these rules:

{{rules}}

{{cluster}}
Write the complete article now.`

func RenderWriter(tmpl, brief, keyword, language string, minWords, maxWords int, cluster articles.Cluster, competitors Competitors) string {
	return strings.NewReplacer(
		styleMarker+"\n\n", "",
		styleMarker, "",
		"{{keyword}}", keyword,
		"{{language}}", language,
		"{{min_words}}", strconv.Itoa(minWords),
		"{{max_words}}", strconv.Itoa(maxWords),
		"{{competitors}}", competitors.RenderSlim(),
		"{{brief}}", brief,
		"{{rules}}", rules.DefaultSEO().Render(),
		"{{cluster}}", renderCluster(cluster),
	).Replace(tmpl)
}

func Writer(
	brief,
	keyword,
	language string,
	minWords,
	maxWords int,
	cluster articles.Cluster,
	competitors Competitors,
) string {
	return RenderWriter(
		WriterTemplate,
		brief,
		keyword,
		language, minWords,
		maxWords,
		cluster,
		competitors,
	)
}
