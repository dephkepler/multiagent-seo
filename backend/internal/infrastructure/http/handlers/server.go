package handlers

import "contentflow/internal/oapigen"

// Server composes feature handlers into the generated oapigen.ServerInterface.
// Each new feature adds its handler here and embeds it.
type Server struct {
	*HealthHandler
}

var _ oapigen.ServerInterface = (*Server)(nil)

func NewServer(health *HealthHandler) *Server {
	return &Server{HealthHandler: health}
}
