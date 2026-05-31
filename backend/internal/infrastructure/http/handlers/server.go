package handlers

// Server embeds the per-feature handlers to satisfy the generated oapigen.ServerInterface.
type Server struct {
	*HealthHandler
	*WordpressSitesHandler
	*LoginHandler
	*ArticlesHandler
	*LinkbuildingHandler
	*ApiTokensHandler
}

func NewServer(health *HealthHandler, wordpress *WordpressSitesHandler, login *LoginHandler, articles *ArticlesHandler, linkbuilding *LinkbuildingHandler, apiTokens *ApiTokensHandler) *Server {
	return &Server{HealthHandler: health, WordpressSitesHandler: wordpress, LoginHandler: login, ArticlesHandler: articles, LinkbuildingHandler: linkbuilding, ApiTokensHandler: apiTokens}
}
