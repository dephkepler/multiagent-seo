package emailscrape

type Site struct {
	Row int
	URL string
}

type Result struct {
	Row    int
	URL    string
	Emails []string
	Status string
}

type EmailRecord struct {
	ListName   string
	SiteURL    string
	Email      string
	SourcePage string
}

type Link struct {
	Href string
	Text string
}

type Page struct {
	URL   string
	HTML  string
	Links []Link
}
