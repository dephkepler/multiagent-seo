package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	applb "multiagent-seo/internal/application/linkbuilding"
	domainlb "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

type linkbuildingLoginService interface {
	LoginToSites(ctx context.Context, req applb.LoginRequest) (applb.LoginQueued, error)
}

type linkbuildingBacklinkService interface {
	PlaceBacklinks(ctx context.Context, req applb.PlaceBacklinksRequest) (applb.PlaceBacklinksQueued, error)
	ListPlacements(ctx context.Context, runID string) ([]domainlb.Placement, error)
	ListPlaced(ctx context.Context, limit, offset int) ([]domainlb.Placement, int, error)
	Cancel(ctx context.Context, runID string)
}

type LinkbuildingHandler struct {
	loginSvc    linkbuildingLoginService
	backlinkSvc linkbuildingBacklinkService
}

func NewLinkbuildingHandler(loginSvc linkbuildingLoginService, backlinkSvc linkbuildingBacklinkService) *LinkbuildingHandler {
	return &LinkbuildingHandler{loginSvc: loginSvc, backlinkSvc: backlinkSvc}
}

var linkbuildingErrMap = newErrMap("handlers.linkbuilding",
	EMsg(applb.ErrNoSheet, http.StatusBadRequest),
	EMsg(applb.ErrNoTargetSite, http.StatusBadRequest),
)

func (h *LinkbuildingHandler) LoginToSites(w http.ResponseWriter, r *http.Request) {
	if isNil(h.loginSvc) {
		problem.Write(w, http.StatusServiceUnavailable, "link building unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.SiteLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.linkbuilding")
		log.Debug().Err(err).Msg("decode login body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	res, err := h.loginSvc.LoginToSites(r.Context(), applb.LoginRequest{Sheet: body.Sheet})
	if err != nil {
		linkbuildingErrMap.Handle(r.Context(), w, "login_to_sites", err)
		return
	}

	response.WriteJSON(r.Context(), w, http.StatusAccepted, oapigen.SiteLoginAccepted{
		Sheet:       res.Sheet,
		SitesQueued: res.SitesQueued,
	})
}

func (h *LinkbuildingHandler) PlaceBacklinks(w http.ResponseWriter, r *http.Request) {
	if isNil(h.backlinkSvc) {
		problem.Write(w, http.StatusServiceUnavailable, "link building unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.PlaceBacklinksRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.linkbuilding")
		log.Debug().Err(err).Msg("decode place backlinks body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	res, err := h.backlinkSvc.PlaceBacklinks(r.Context(), applb.PlaceBacklinksRequest{
		Sheet:         body.Sheet,
		TargetSiteURL: body.TargetSiteUrl,
		Count:         deref(body.Count),
		Provider:      derefStr(body.Provider),
		Model:         derefStr(body.Model),
	})
	if err != nil {
		linkbuildingErrMap.Handle(r.Context(), w, "place_backlinks", err)
		return
	}

	response.WriteJSON(r.Context(), w, http.StatusAccepted, oapigen.PlaceBacklinksAccepted{
		Sheet:       res.Sheet,
		SitesQueued: res.SitesQueued,
		RunId:       res.RunID,
	})
}

func (h *LinkbuildingHandler) ListPlacements(w http.ResponseWriter, r *http.Request, params oapigen.ListPlacementsParams) {
	if isNil(h.backlinkSvc) {
		problem.Write(w, http.StatusServiceUnavailable, "link building unavailable")
		return
	}
	items, err := h.backlinkSvc.ListPlacements(r.Context(), params.RunId)
	if err != nil {
		linkbuildingErrMap.Handle(r.Context(), w, "list_placements", err)
		return
	}
	out := make([]oapigen.Placement, len(items))
	for i, p := range items {
		out[i] = toAPIPlacement(p)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *LinkbuildingHandler) ListPlacementHistory(w http.ResponseWriter, r *http.Request, params oapigen.ListPlacementHistoryParams) {
	if isNil(h.backlinkSvc) {
		problem.Write(w, http.StatusServiceUnavailable, "link building unavailable")
		return
	}
	limit, offset := pageBounds(params.Limit, params.Offset, 20, 100)
	items, total, err := h.backlinkSvc.ListPlaced(r.Context(), limit, offset)
	if err != nil {
		linkbuildingErrMap.Handle(r.Context(), w, "list_placement_history", err)
		return
	}
	out := make([]oapigen.Placement, len(items))
	for i, p := range items {
		out[i] = toAPIPlacement(p)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.PlacementList{Items: out, Total: total})
}

func (h *LinkbuildingHandler) CancelPlacement(w http.ResponseWriter, r *http.Request, runId string) {
	if isNil(h.backlinkSvc) {
		problem.Write(w, http.StatusServiceUnavailable, "link building unavailable")
		return
	}
	h.backlinkSvc.Cancel(r.Context(), runId)
	w.WriteHeader(http.StatusAccepted)
}

func toAPIPlacement(p domainlb.Placement) oapigen.Placement {
	return oapigen.Placement{
		Id:        p.ID,
		DonorUrl:  p.DonorURL,
		TargetUrl: p.TargetURL,
		Ok:        p.OK,
		Outcome:   ptr(p.Outcome),
		Status:    p.Status,
		PostUrl:   ptr(p.PostURL),
		EditUrl:   ptr(p.EditURL),
		Anchor:    ptr(p.Anchor),
		CreatedAt: p.CreatedAt,
	}
}

func pageBounds(limit, offset *int, defLimit, maxLimit int) (int, int) {
	l := defLimit
	if limit != nil && *limit > 0 {
		l = *limit
	}
	if l > maxLimit {
		l = maxLimit
	}
	o := 0
	if offset != nil && *offset > 0 {
		o = *offset
	}
	return l, o
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
