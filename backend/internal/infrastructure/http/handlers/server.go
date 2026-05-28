package handlers

import "contentflow/internal/oapigen"

// Server composes feature handlers into the generated oapigen.ServerInterface.
// Each new feature adds its handler here and embeds it.
type Server struct {
	*HealthHandler
	*WordpressSitesHandler
	*LoginHandler
}

var _ oapigen.ServerInterface = (*Server)(nil)

func NewServer(health *HealthHandler, wordpress *WordpressSitesHandler, login *LoginHandler) *Server {
	return &Server{HealthHandler: health, WordpressSitesHandler: wordpress, LoginHandler: login}
}
