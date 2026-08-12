package handlers

import (
	"context"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	domainlead "multiagent-seo/internal/domain/leadstats"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

func openapiDate(t time.Time) openapi_types.Date {
	return openapi_types.Date{Time: t}
}

type leadStatsService interface {
	GetStats(ctx context.Context, from, to time.Time, groupBy string) (domainlead.Stats, error)
}

type LeadStatsHandler struct {
	svc leadStatsService
}

func NewLeadStatsHandler(svc leadStatsService) *LeadStatsHandler {
	return &LeadStatsHandler{svc: svc}
}

func (h *LeadStatsHandler) GetLeadStats(w http.ResponseWriter, r *http.Request, params oapigen.GetLeadStatsParams) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "lead stats unavailable")
		return
	}

	from, to := params.From.Time, params.To.Time
	if to.Before(from) {
		problem.Write(w, http.StatusBadRequest, "to must not be before from")
		return
	}

	groupBy := "day"
	if params.GroupBy != nil {
		groupBy = string(*params.GroupBy)
	}

	stats, err := h.svc.GetStats(r.Context(), from, to, groupBy)
	if err != nil {
		log := logger.New(r.Context(), "handlers.leadstats")
		log.Error().Err(err).Msg("get lead stats failed")
		problem.Write(w, http.StatusInternalServerError, "failed to load lead stats")
		return
	}

	response.WriteJSON(r.Context(), w, http.StatusOK, toOapiLeadStats(stats))
}

func toOapiLeadStats(s domainlead.Stats) oapigen.LeadStats {
	out := oapigen.LeadStats{}
	out.Range.From = openapiDate(s.From)
	out.Range.To = openapiDate(s.To)
	out.Range.GroupBy = s.GroupBy

	out.Totals.Leads = s.Totals.Leads
	out.Totals.Clients = s.Totals.Clients
	out.Totals.Consultations = s.Totals.Consultations
	out.Totals.Revenue = float32(s.Totals.Revenue)
	out.Totals.AvgTicket = float32(s.Totals.AvgTicket)

	out.Trend = make([]oapigen.LeadStatsBucket, len(s.Trend))
	for i, b := range s.Trend {
		out.Trend[i] = oapigen.LeadStatsBucket{Bucket: b.Bucket, Leads: b.Leads, Consultations: b.Consultations}
	}
	out.ByPage = toOapiCounts(s.ByPage)
	out.ByCreator = toOapiCounts(s.ByCreator)
	out.ByStatus = toOapiCounts(s.ByStatus)
	return out
}

func toOapiCounts(cs []domainlead.Count) []oapigen.LeadStatsCount {
	out := make([]oapigen.LeadStatsCount, len(cs))
	for i, c := range cs {
		out[i] = oapigen.LeadStatsCount{Key: c.Key, Count: c.Count}
	}
	return out
}
