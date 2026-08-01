package prompt

import (
	"fmt"
	"strings"
)

const (
	SepOriginal = "---ORIGINAL---"
	SepModified = "---MODIFIED---"
	SepBody     = "---BODY---"
)

func Compose(targetURL string, titles []string) string {
	context := "No recent titles available; write in English on a general topic."
	if len(titles) > 0 {
		context = "Recent post titles on this blog (match their LANGUAGE and topical niche):\n- " + strings.Join(titles, "\n- ")
	}
	return fmt.Sprintf(`Write a short, original WordPress blog post (max ~1800 characters) for an existing blog.

%s

Write in the SAME LANGUAGE as those titles and pick a topic close to that blog's niche. Weave in exactly ONE natural contextual link to the URL below using a single <a href="..." rel="noopener"> tag with a 2-4 word anchor that fits the sentence. It must read naturally and not look like an ad.

URL to link to: %s

Reply with EXACTLY this format and nothing else (no markdown fences):
TITLE: <post title in that language, no quotes>
ANCHOR: <the 2-4 word anchor you used>
%s
<the post body as clean HTML paragraphs (max ~1800 characters), containing the single <a> tag>`, context, targetURL, SepBody)
}

func Placer(html, targetURL string) string {
	return fmt.Sprintf(`You are editing an existing WordPress blog post. Find ONE paragraph in the post that fits a natural backlink to the URL below, then insert a single <a> tag with a 2-4 word anchor drawn from that paragraph's existing wording. Use rel="noopener". Do not paraphrase or change the paragraph beyond adding that single <a> tag.

URL to link to: %s

Existing post HTML:
%s

Reply with EXACTLY this three-part format and nothing else (no prose, no markdown fences):
ANCHOR: <chosen 2-4 word anchor>
%s
<the exact paragraph copied verbatim from the post HTML above>
%s
<the same paragraph with one new <a> tag inserted>`, targetURL, html, SepOriginal, SepModified)
}
