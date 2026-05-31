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
// login endpoint is derived from BaseURL by the adapter.
type SiteCredential struct {
	Row      int
	BaseURL  string
	Login    string
	Password string
}

// LoginResult is the outcome written back to the sheet. OK is true only on a
// confirmed authenticated session.
type LoginResult struct {
	Row     int
	BaseURL string
	OK      bool
	Status  string
}
