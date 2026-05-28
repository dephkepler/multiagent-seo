package handlers

import "multiagent-seo/internal/oapigen"

// Server embeds the per-feature handlers to satisfy the generated oapigen.ServerInterface.
type Server struct {
	*HealthHandler
	*WordpressSitesHandler
	*LoginHandler
	*ArticlesHandler
}

var _ oapigen.ServerInterface = (*Server)(nil)

func NewServer(health *HealthHandler, wordpress *WordpressSitesHandler, login *LoginHandler, articles *ArticlesHandler) *Server {
	return &Server{HealthHandler: health, WordpressSitesHandler: wordpress, LoginHandler: login, ArticlesHandler: articles}
}
