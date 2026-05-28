package handlers

import (
	"context"
	"net/http"
	"time"

	domainhealth "multiagent-seo/internal/domain/health"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
)

type healthService interface {
	Check(ctx context.Context) domainhealth.Health
}

type HealthHandler struct {
	health healthService
}

func NewHealthHandler(health healthService) *HealthHandler {
	return &HealthHandler{health: health}
}

func (h *HealthHandler) GetHealthz(w http.ResponseWriter, r *http.Request) {
	result := h.health.Check(r.Context())

	status := http.StatusOK
	if !result.IsHealthy() {
		status = http.StatusServiceUnavailable
	}

	response.WriteJSON(r.Context(), w, status, oapigen.HealthResponse{
		Status: oapigen.HealthResponseStatus(result.Status()),
		Time:   time.Now().UTC(),
	})
}
