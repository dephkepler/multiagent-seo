package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	applb "multiagent-seo/internal/application/linkbuilding"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

type linkbuildingService interface {
	QualifyWebsites(ctx context.Context, req applb.QualifyRequest) (applb.QualifyResult, error)
}

type LinkbuildingHandler struct {
	svc linkbuildingService
}

func NewLinkbuildingHandler(svc linkbuildingService) *LinkbuildingHandler {
	return &LinkbuildingHandler{svc: svc}
}

func (h *LinkbuildingHandler) QualifyWebsites(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
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
