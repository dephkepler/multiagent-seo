package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"

	applb "multiagent-seo/internal/application/linkbuilding"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

// isNil treats a typed-nil interface (e.g. an interface holding a
// *applinkbuilding.Service value that happens to be nil) as nil so callers
// don't have to spell out the untyped-nil dance at the call site.
func isNil(i any) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

type linkbuildingService interface {
	QualifyWebsites(ctx context.Context, req applb.QualifyRequest) (applb.QualifyResult, error)
}

type linkbuildingLoginService interface {
	LoginToSites(ctx context.Context, req applb.LoginRequest) (applb.LoginQueued, error)
}

type linkbuildingBacklinkService interface {
	PlaceBacklinks(ctx context.Context, req applb.PlaceBacklinksRequest) (applb.PlaceBacklinksQueued, error)
}

type LinkbuildingHandler struct {
	svc         linkbuildingService
	loginSvc    linkbuildingLoginService
	backlinkSvc linkbuildingBacklinkService
}

func NewLinkbuildingHandler(svc linkbuildingService, loginSvc linkbuildingLoginService, backlinkSvc linkbuildingBacklinkService) *LinkbuildingHandler {
	return &LinkbuildingHandler{svc: svc, loginSvc: loginSvc, backlinkSvc: backlinkSvc}
}

func (h *LinkbuildingHandler) QualifyWebsites(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "link building unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.QualifyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	res, err := h.svc.QualifyWebsites(r.Context(), applb.QualifyRequest{
		Sheet:           body.Sheet,
		AcceptedTopics:  body.AcceptedTopics,
		CandidateTopics: body.CandidateTopics,
		Provider:        derefStr(body.Provider),
		Model:           derefStr(body.Model),
	})
	if err != nil {
		switch {
		case errors.Is(err, applb.ErrNoSheet), errors.Is(err, applb.ErrNoTopics):
			problem.Write(w, http.StatusBadRequest, err.Error())
		default:
			log := logger.New(r.Context(), "handlers.linkbuilding")
			log.Error().Err(err).Msg("internal error")
			problem.Write(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	response.WriteJSON(r.Context(), w, http.StatusAccepted, oapigen.QualifyAccepted{
		Sheet:          res.Sheet,
		WebsitesQueued: res.WebsitesQueued,
	})
}

func (h *LinkbuildingHandler) LoginToSites(w http.ResponseWriter, r *http.Request) {
	if isNil(h.loginSvc) {
		problem.Write(w, http.StatusServiceUnavailable, "link building unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.SiteLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	res, err := h.loginSvc.LoginToSites(r.Context(), applb.LoginRequest{Sheet: body.Sheet, Topics: body.Topics})
	if err != nil {
		switch {
		case errors.Is(err, applb.ErrNoSheet):
			problem.Write(w, http.StatusBadRequest, err.Error())
		default:
			log := logger.New(r.Context(), "handlers.linkbuilding")
			log.Error().Err(err).Msg("internal error")
			problem.Write(w, http.StatusInternalServerError, "internal error")
		}
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
		Topics:        body.Topics,
		Provider:      derefStr(body.Provider),
		Model:         derefStr(body.Model),
	})
	if err != nil {
		switch {
		case errors.Is(err, applb.ErrNoSheet), errors.Is(err, applb.ErrNoTargetSite):
			problem.Write(w, http.StatusBadRequest, err.Error())
		default:
			log := logger.New(r.Context(), "handlers.linkbuilding")
			log.Error().Err(err).Msg("internal error")
			problem.Write(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	response.WriteJSON(r.Context(), w, http.StatusAccepted, oapigen.PlaceBacklinksAccepted{
		Sheet:       res.Sheet,
		SitesQueued: res.SitesQueued,
	})
}

// derefStr turns an oapigen optional *string into a plain string ("" if nil),
// so the application layer can stick with a value-typed request struct.
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

