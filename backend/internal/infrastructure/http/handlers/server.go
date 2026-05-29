package handlers

import "multiagent-seo/internal/oapigen"

// Server embeds the per-feature handlers to satisfy the generated oapigen.ServerInterface.
type Server struct {
	*HealthHandler
	*WordpressSitesHandler
	*LoginHandler
	*ArticlesHandler
	*LinkbuildingHandler
}

var _ oapigen.ServerInterface = (*Server)(nil)

func NewServer(health *HealthHandler, wordpress *WordpressSitesHandler, login *LoginHandler, articles *ArticlesHandler, linkbuilding *LinkbuildingHandler) *Server {
	return &Server{HealthHandler: health, WordpressSitesHandler: wordpress, LoginHandler: login, ArticlesHandler: articles, LinkbuildingHandler: linkbuilding}
}
