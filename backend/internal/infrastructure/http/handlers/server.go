package handlers

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
}

func NewServer(
	health *HealthHandler,
	wordpress *WordpressSitesHandler,
	login *LoginHandler,
	articles *ArticlesHandler,
	linkbuilding *LinkbuildingHandler,
	apiTokens *ApiTokensHandler,
	emailScrape *EmailScrapeHandler,
	leadStats *LeadStatsHandler,
	clientSegments *ClientSegmentsHandler,
	clientDetail *ClientDetailHandler,
) *Server {
	return &Server{
		HealthHandler:         health,
		WordpressSitesHandler: wordpress,
		LoginHandler:          login,
		ArticlesHandler:       articles,
		LinkbuildingHandler:   linkbuilding,
		ApiTokensHandler:      apiTokens,
		EmailScrapeHandler:    emailScrape,
		LeadStatsHandler:      leadStats,
		ClientSegmentsHandler: clientSegments,
		ClientDetailHandler:   clientDetail,
	}
}
