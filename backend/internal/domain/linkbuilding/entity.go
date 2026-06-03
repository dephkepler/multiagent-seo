// Package linkbuilding is the donor-site qualification domain: given a list of
// candidate websites (from a sheet), classify each one's topic, count its
// outbound domains, and decide whether it's a suitable backlink target. It owns
// no infrastructure — every dependency is a port implemented in infrastructure.
package linkbuilding

// Website is one donor candidate read from the source sheet. Row is the sheet
// row index so the qualification result can be written back to the same line.
type Website struct {
	Row int
	URL string
}

// Page is a fetched homepage reduced to the signals qualification needs.
// Links holds the raw href values found on the page (resolved/filtered later).
type Page struct {
	Title           string
	MetaDescription string
	Headings        []string
	TextSample      string
	Links           []string
}

// Result is the qualification outcome written back to the sheet row.
type Result struct {
	Row             int
	URL             string
	Topic           string
	OutboundDomains int
	Suitable        bool
}

// SiteCredential is one site we have login access to, read from the credential
// columns. Row lets the login status be written back to the same line; the
// login endpoint is derived from BaseURL by the adapter. Topic is the Flow 1
// classification from column B, exposed here so Flow 2/3 can optionally narrow
// to a specific campaign segment without relying on a re-qualify having
// rewritten the boolean D flag. LoginStatus carries the prior Flow 2 verdict
// from column H so Flow 3 can skip rows that already failed authentication.
type SiteCredential struct {
	Row         int
	BaseURL     string
	Login       string
	Password    string
	Topic       string
	LoginStatus string
}

// LoginResult is the outcome written back to the sheet. OK is true only on a
// confirmed authenticated session.
type LoginResult struct {
	Row     int
	BaseURL string
	OK      bool
	Status  string
}

// DonorCredential is the long-lived app-password we keep per donor site so
// Flow 3 doesn't have to replay the legacy form login every run. AppPassword
// is plaintext only in memory; the store encrypts at rest.
type DonorCredential struct {
	DonorURL    string
	Login       string
	AppPassword string
}

// DonorPost is the slice of a donor's WordPress post we need for backlink
// placement. Content is the post body as HTML (the WP "content.rendered"
// field). PublicURL is the front-end permalink (so the user can verify the
// inserted backlink without logging in); EditURL is the wp-admin editor link
// for revisiting/reverting.
type DonorPost struct {
	ID        int64
	Title     string
	Content   string
	PublicURL string
	EditURL   string
}

// BacklinkInsertion is what the LLM gives back when asked to weave a link into
// an existing post. Anchor is the text the LLM chose (so we can log it);
// ModifiedHTML is the post content with the <a> tag inserted in-place.
type BacklinkInsertion struct {
	Anchor       string
	ModifiedHTML string
}

// PlacementResult is the per-donor outcome written to the sheet's I column.
// OK is true only when the modified post was actually saved back to WordPress.
type PlacementResult struct {
	Row      int
	DonorURL string
	OK       bool
	Status   string
}
