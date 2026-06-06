package prompt

import "fmt"

const (
	SepOriginal = "---ORIGINAL---"
	SepModified = "---MODIFIED---"
)

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
