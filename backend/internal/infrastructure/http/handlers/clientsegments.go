package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	domain "multiagent-seo/internal/domain/clientsegments"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

type clientSegmentsService interface {
	List(ctx context.Context, filter domain.ListFilter) (domain.ClientList, error)
	SetOverride(ctx context.Context, clientID string, segment *string) error
}

type ClientSegmentsHandler struct {
	svc clientSegmentsService
}

func NewClientSegmentsHandler(svc clientSegmentsService) *ClientSegmentsHandler {
	return &ClientSegmentsHandler{svc: svc}
}

func (h *ClientSegmentsHandler) ListClientSegments(w http.ResponseWriter, r *http.Request, params oapigen.ListClientSegmentsParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "client segments unavailable")
		return
	}

	filter := domain.ListFilter{}
	if params.Id != nil {
		filter.ClientID = *params.Id
	}
	if params.Segment != nil {
		filter.Segment = string(*params.Segment)
	}
	if params.Tag != nil {
		filter.Tag = *params.Tag
	}
	if params.Search != nil {
		filter.Search = *params.Search
	}
	if params.Sort != nil {
		filter.Sort = string(*params.Sort)
	}
	if params.Limit != nil {
		filter.Limit = *params.Limit
	}
	if params.Offset != nil {
		filter.Offset = *params.Offset
	}

	list, err := h.svc.List(r.Context(), filter)
	if err != nil {
		h.writeError(r.Context(), w, "list_client_segments", err)
		return
	}

	items := make([]oapigen.ClientSegment, len(list.Items))
	for i, s := range list.Items {
		items[i] = toAPIClientSegment(s)
	}
	counts := make(map[string]int64, len(list.SegmentCounts))
	for segment, n := range list.SegmentCounts {
		counts[segment] = int64(n)
	}

	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.ClientList{
		Items:         items,
		Total:         int64(list.Total),
		SegmentCounts: counts,
	})
}

func (h *ClientSegmentsHandler) SetClientSegmentOverride(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "client segments unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.SetClientSegmentOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.clientsegments")
		log.Debug().Err(err).Msg("decode segment override body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var segment *string
	if body.SegmentOverride != nil {
		s := string(*body.SegmentOverride)
		segment = &s
	}

	if err := h.svc.SetOverride(r.Context(), id.String(), segment); err != nil {
		h.writeError(r.Context(), w, "set_segment_override", err)
		return
	}
	response.NoContent(w)
}

var clientSegmentsErrMap = newErrMap("handlers.clientsegments",
	E(domain.ErrNotFound, http.StatusNotFound, "client not found"),
	EMsg(domain.ErrInvalidSegment, http.StatusBadRequest),
	EMsg(domain.ErrInvalidTag, http.StatusBadRequest),
	EMsg(domain.ErrInvalidSort, http.StatusBadRequest),
)

func (h *ClientSegmentsHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	clientSegmentsErrMap.Handle(ctx, w, op, err)
}

func toAPIClientSegment(s domain.ClientSegment) oapigen.ClientSegment {
	tags := s.Tags
	if tags == nil {
		tags = []string{} // encode as [], not null — Tags is a required field
	}
	return oapigen.ClientSegment{
		ClientId:     s.ClientID,
		Name:         s.Name,
		Phone:        s.Phone,
		Segment:      oapigen.Segment(s.Segment),
		Overridden:   s.Overridden,
		Tags:         tags,
		LastActivity: s.LastActivity,
		CaseCount:    int64(s.CaseCount),
		CaseFee:      float32(s.CaseFee),
		CasePaid:     float32(s.CasePaid),
		Ltv:          float32(s.LTV),
	}
}
