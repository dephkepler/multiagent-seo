package handlers

import (
	"context"
	"net/http"

	domain "multiagent-seo/internal/domain/clientsegments"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

type clientSegmentsService interface {
	List(ctx context.Context) ([]domain.ClientSegment, error)
}

type ClientSegmentsHandler struct {
	svc clientSegmentsService
}

func NewClientSegmentsHandler(svc clientSegmentsService) *ClientSegmentsHandler {
	return &ClientSegmentsHandler{svc: svc}
}

func (h *ClientSegmentsHandler) ListClientSegments(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "client segments unavailable")
		return
	}

	segments, err := h.svc.List(r.Context())
	if err != nil {
		log := logger.New(r.Context(), "handlers.clientsegments")
		log.Error().Err(err).Msg("list client segments failed")
		problem.Write(w, http.StatusInternalServerError, "failed to load client segments")
		return
	}

	out := make([]oapigen.ClientSegment, len(segments))
	for i, s := range segments {
		tags := s.Tags
		if tags == nil {
			tags = []string{} // encode as [] , not null — Tags is a required field
		}
		out[i] = oapigen.ClientSegment{
			ClientId:     s.ClientID,
			Name:         s.Name,
			Phone:        s.Phone,
			Segment:      oapigen.ClientSegmentSegment(s.Segment),
			Tags:         tags,
			LastActivity: s.LastActivity,
			CaseCount:    int64(s.CaseCount),
			CaseFee:      float32(s.CaseFee),
			CasePaid:     float32(s.CasePaid),
		}
	}

	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}
