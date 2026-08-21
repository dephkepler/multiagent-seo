package handlers

// Server is the one oapigen.ServerInterface implementation: each feature's
// handler is embedded, so the generated router finds every method on one value.
type Server struct {
	*HealthHandler
	*WordpressSitesHandler
	*LoginHandler
	*ArticlesHandler
	*LinkbuildingHandler
	*ApiTokensHandler
	*EmailScrapeHandler
	*LeadStatsHandler
	*ClientSegmentsHandler
	*ClientDetailHandler
	*VaultHandler
	*FinanceHandler
	*MyHandler
	*ClientHandler
}

// Deps names the handlers instead of ordering them. Thirteen positional
// arguments were one silent mistake away from a router that answers the wrong
// feature's requests, and every call site had to be edited each time a
// handler was added.
type Deps struct {
	Health         *HealthHandler
	Wordpress      *WordpressSitesHandler
	Login          *LoginHandler
	Articles       *ArticlesHandler
	Linkbuilding   *LinkbuildingHandler
	ApiTokens      *ApiTokensHandler
	EmailScrape    *EmailScrapeHandler
	LeadStats      *LeadStatsHandler
	ClientSegments *ClientSegmentsHandler
	ClientDetail   *ClientDetailHandler
	Vault          *VaultHandler
	Finance        *FinanceHandler
	My             *MyHandler
	Client         *ClientHandler
}

// NewServer replaces anything Deps left out with a handler wired to no
// service, which answers 503 "unavailable". Leaving the embedded pointer nil
// would instead panic on the first request to that route — a feature switched
// off in config must not take the process down with it.
func NewServer(d Deps) *Server {
	if d.Health == nil {
		d.Health = NewHealthHandler(nil)
	}
	if d.Wordpress == nil {
		d.Wordpress = NewWordpressSitesHandler(nil)
	}
	if d.Login == nil {
		d.Login = NewLoginHandler(nil)
	}
	if d.Articles == nil {
		d.Articles = NewArticlesHandler(nil)
	}
	if d.Linkbuilding == nil {
		d.Linkbuilding = NewLinkbuildingHandler(nil)
	}
	if d.ApiTokens == nil {
		d.ApiTokens = NewApiTokensHandler(nil)
	}
	if d.EmailScrape == nil {
		d.EmailScrape = NewEmailScrapeHandler(nil)
	}
	if d.LeadStats == nil {
		d.LeadStats = NewLeadStatsHandler(nil)
	}
	if d.ClientSegments == nil {
		d.ClientSegments = NewClientSegmentsHandler(nil)
	}
	if d.ClientDetail == nil {
		d.ClientDetail = NewClientDetailHandler(nil)
	}
	if d.Vault == nil {
		d.Vault = NewVaultHandler(nil)
	}
	if d.Finance == nil {
		d.Finance = NewFinanceHandler(nil)
	}
	if d.My == nil {
		d.My = NewMyHandler(nil)
	}
	if d.Client == nil {
		d.Client = NewClientHandler(nil)
	}

	return &Server{
		HealthHandler:         d.Health,
		WordpressSitesHandler: d.Wordpress,
		LoginHandler:          d.Login,
		ArticlesHandler:       d.Articles,
		LinkbuildingHandler:   d.Linkbuilding,
		ApiTokensHandler:      d.ApiTokens,
		EmailScrapeHandler:    d.EmailScrape,
		LeadStatsHandler:      d.LeadStats,
		ClientSegmentsHandler: d.ClientSegments,
		ClientDetailHandler:   d.ClientDetail,
		VaultHandler:          d.Vault,
		FinanceHandler:        d.Finance,
		MyHandler:             d.My,
		ClientHandler:         d.Client,
	}
}
