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
	out.Totals.RevenueBooked = float32(s.Totals.RevenueBooked)
	out.Totals.RevenueEarned = float32(s.Totals.RevenueEarned)
	out.Totals.RevenueLost = float32(s.Totals.RevenueLost)
	out.Totals.AvgTicket = float32(s.Totals.AvgTicket)
	out.Totals.CasesInProgress = s.Totals.CasesInProgress
	out.Totals.CasesCompleted = s.Totals.CasesCompleted
	out.Totals.CaseFeeContracted = float32(s.Totals.CaseFeeContracted)
	out.Totals.CasePaid = float32(s.Totals.CasePaid)
	out.Totals.CaseOwed = float32(s.Totals.CaseOwed)
	out.Totals.SiteSessions = s.Totals.SiteSessions
	out.Totals.OrganicSessions = s.Totals.OrganicSessions

	out.Trend = make([]oapigen.LeadStatsBucket, len(s.Trend))
	for i, b := range s.Trend {
		out.Trend[i] = oapigen.LeadStatsBucket{
			Bucket:          b.Bucket,
			Leads:           b.Leads,
			Consultations:   b.Consultations,
			RevenueEarned:   float32(b.RevenueEarned),
			SiteSessions:    b.SiteSessions,
			OrganicSessions: b.OrganicSessions,
		}
	}
	out.BySource = make([]oapigen.LeadStatsSourceValue, len(s.BySource))
	for i, src := range s.BySource {
		out.BySource[i] = oapigen.LeadStatsSourceValue{
			Key:           src.Key,
			Leads:         src.Leads,
			ConsultedEver: src.ConsultedEver,
			CasedEver:     src.CasedEver,
			RevenueEarned: float32(src.RevenueEarned),
			CasePaid:      float32(src.CasePaid),
		}
	}
	out.ByStatus = toOapiCounts(s.ByStatus)

	out.ByCreator = make([]oapigen.LeadStatsCreatorRevenue, len(s.ByCreator))
	for i, c := range s.ByCreator {
		out.ByCreator[i] = oapigen.LeadStatsCreatorRevenue{
			Key:           c.Key,
			Bookings:      c.Bookings,
			RevenueEarned: float32(c.RevenueEarned),
		}
	}

	out.ByCategory = toOapiCategoryRevenue(s.ByCategory)
	out.ByAdvocate = toOapiCategoryRevenue(s.ByAdvocate)

	out.Funnel = oapigen.LeadStatsFunnel{
		CohortLeads:          s.Funnel.CohortLeads,
		ConsultedEver:        s.Funnel.ConsultedEver,
		CasedEver:            s.Funnel.CasedEver,
		AvgDaysToConsult:     float32(s.Funnel.AvgDaysToConsult),
		AvgDaysConsultToCase: float32(s.Funnel.AvgDaysConsultToCase),
		FirstConsultOutcome: oapigen.LeadStatsFunnelStage{
			Completed: s.Funnel.FirstConsultOutcome.Completed,
			Cancelled: s.Funnel.FirstConsultOutcome.Cancelled,
			NoShow:    s.Funnel.FirstConsultOutcome.NoShow,
			Scheduled: s.Funnel.FirstConsultOutcome.Scheduled,
		},
	}
	out.ByWeekday = toOapiCounts(s.ByWeekday)
	out.ByLeadPracticeArea = toOapiCounts(s.ByLeadPracticeArea)
	out.Audience = oapigen.LeadStatsAudience{
		ByAge:    toOapiCounts(s.Audience.ByAge),
		ByGender: toOapiCounts(s.Audience.ByGender),
		ByCity:   toOapiCounts(s.Audience.ByCity),
	}
	return out
}

// toOapiCategoryRevenue serves both ByCategory (practice area) and
// ByAdvocate (who closed the case) — same Key/Cases/Contracted/Paid shape
// on both the domain and oapigen side, just grouped by a different column.
func toOapiCategoryRevenue(cs []domainlead.CategoryRevenue) []oapigen.LeadStatsCategoryRevenue {
	out := make([]oapigen.LeadStatsCategoryRevenue, len(cs))
	for i, c := range cs {
		out[i] = oapigen.LeadStatsCategoryRevenue{
			Key:        c.Key,
			Cases:      c.Cases,
			Contracted: float32(c.Contracted),
			Paid:       float32(c.Paid),
		}
	}
	return out
}

func toOapiCounts(cs []domainlead.Count) []oapigen.LeadStatsCount {
	out := make([]oapigen.LeadStatsCount, len(cs))
	for i, c := range cs {
		out[i] = oapigen.LeadStatsCount{Key: c.Key, Count: c.Count}
	}
	return out
}
