package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	appemail "multiagent-seo/internal/application/emailscrape"
	domainemail "multiagent-seo/internal/domain/emailscrape"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

type emailScrapeService interface {
	ScrapeEmails(ctx context.Context, req appemail.ScrapeRequest) (appemail.ScrapeResult, error)
	JobStatus(ctx context.Context, id string) (domainemail.Job, error)
	Cancel(ctx context.Context, id string) error
}

type EmailScrapeHandler struct {
	svc emailScrapeService
}

func NewEmailScrapeHandler(svc emailScrapeService) *EmailScrapeHandler {
	return &EmailScrapeHandler{svc: svc}
}

var emailScrapeErrMap = newErrMap("handlers.emailscrape",
	EMsg(appemail.ErrNoSheet, http.StatusBadRequest),
	E(domainemail.ErrJobNotFound, http.StatusNotFound, "scrape job not found"),
)

func (h *EmailScrapeHandler) ScrapeEmails(w http.ResponseWriter, r *http.Request) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "email scraping unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.ScrapeEmailsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.emailscrape")
		log.Debug().Err(err).Msg("decode scrape body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	res, err := h.svc.ScrapeEmails(r.Context(), appemail.ScrapeRequest{Sheet: body.Sheet})
	if err != nil {
		emailScrapeErrMap.Handle(r.Context(), w, "scrape_emails", err)
		return
	}

	response.WriteJSON(r.Context(), w, http.StatusAccepted, oapigen.ScrapeEmailsAccepted{
		Sheet:       res.Sheet,
		SitesQueued: res.SitesQueued,
		JobId:       res.JobID,
	})
}

func (h *EmailScrapeHandler) GetScrapeJob(w http.ResponseWriter, r *http.Request, id string) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "email scraping unavailable")
		return
	}
	job, err := h.svc.JobStatus(r.Context(), id)
	if err != nil {
		emailScrapeErrMap.Handle(r.Context(), w, "get_scrape_job", err)
		return
	}

	response.WriteJSON(r.Context(), w, http.StatusOK, oapigen.ScrapeJob{
		Id:        job.ID,
		Sheet:     job.Sheet,
		State:     job.State,
		Total:     job.Total,
		Processed: job.Processed,
	})
}

func (h *EmailScrapeHandler) CancelScrapeJob(w http.ResponseWriter, r *http.Request, id string) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "email scraping unavailable")
		return
	}
	if err := h.svc.Cancel(r.Context(), id); err != nil {
		emailScrapeErrMap.Handle(r.Context(), w, "cancel_scrape_job", err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
