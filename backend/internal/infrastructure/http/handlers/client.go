package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"multiagent-seo/internal/application/clientportal"
	"multiagent-seo/internal/domain/consultations"
	"multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

type clientPortalService interface {
	FreeSlots(ctx context.Context) ([]time.Time, error)
	Submit(ctx context.Context, req clientportal.Request) (clientportal.Submission, error)
	Me(ctx context.Context, clientID string) (clientportal.Profile, error)
}

// ClientHandler serves what a client can do for themselves.
//
// The client is never a request parameter. Their id, their Telegram id and
// their display name all come from the principal the auth middleware resolved
// out of the launch signature, so a request cannot speak for anyone else.
type ClientHandler struct {
	svc clientPortalService
}

func NewClientHandler(svc clientPortalService) *ClientHandler {
	return &ClientHandler{svc: svc}
}

func (h *ClientHandler) ListClientSlots(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	slots, err := h.svc.FreeSlots(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, "client_slots", err)
		return
	}
	// Never nil: the picker renders an empty list as "no free hours", and a
	// null would make it render an error instead.
	out := oapigen.ClientSlotList{Slots: slots}
	if out.Slots == nil {
		out.Slots = []time.Time{}
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *ClientHandler) SubmitClientRequest(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	caller, ok := middleware.PrincipalFromContext(r.Context())
	if !ok {
		problem.Write(w, http.StatusForbidden, "forbidden")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.ClientRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.client")
		log.Debug().Err(err).Msg("decode client request body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req := clientportal.Request{
		Name:         body.Name,
		Phone:        body.Phone,
		Email:        deref(body.Email),
		Category:     deref(body.Category),
		Question:     deref(body.Question),
		TelegramID:   caller.TelegramID,
		TelegramName: caller.TelegramName,
	}
	if body.Slot != nil {
		req.Slot = *body.Slot
	}

	submission, err := h.svc.Submit(r.Context(), req)
	if err != nil {
		h.writeError(r.Context(), w, "client_request", err)
		return
	}

	clientID, err := uuid.Parse(submission.ClientID)
	if err != nil {
		h.writeError(r.Context(), w, "client_request", err)
		return
	}
	out := oapigen.ClientRequestResult{ClientId: clientID}
	if submission.Consultation != nil {
		consultation := toAPIPortalConsultation(*submission.Consultation)
		out.Consultation = &consultation
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, out)
}

func (h *ClientHandler) GetClientProfile(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	caller, ok := middleware.PrincipalFromContext(r.Context())
	if !ok || caller.ClientID == "" {
		// A guest reaching this is the scope gate working: they have no record
		// to read yet, and the intake is what creates one.
		problem.Write(w, http.StatusForbidden, "this launch is not linked to a client")
		return
	}

	profile, err := h.svc.Me(r.Context(), caller.ClientID)
	if err != nil {
		h.writeError(r.Context(), w, "client_profile", err)
		return
	}

	out := oapigen.ClientProfile{
		Name:            profile.Name,
		NotificationsOn: profile.NotificationsOn,
		Consultations:   make([]oapigen.PortalConsultation, len(profile.Consultations)),
	}
	if profile.Phone != "" {
		out.Phone = &profile.Phone
	}
	for i, c := range profile.Consultations {
		out.Consultations[i] = toAPIPortalConsultation(c)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func toAPIPortalConsultation(c consultations.Consultation) oapigen.PortalConsultation {
	price := c.Price
	out := oapigen.PortalConsultation{
		ScheduledAt: c.ScheduledAt,
		Status:      c.Status,
		Price:       &price,
	}
	if id, err := uuid.Parse(c.ID); err == nil {
		out.Id = openapi_types.UUID(id)
	}
	return out
}

func (h *ClientHandler) available(w http.ResponseWriter) bool {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "client portal unavailable")
		return false
	}
	return true
}

var clientErrMap = newErrMap(
	"handlers.client",
	EMsg(clientportal.ErrNoName, http.StatusBadRequest),
	EMsg(clientportal.ErrNoPhone, http.StatusBadRequest),
	// 409 rather than 400: the request was well formed, the hour just stopped
	// being available, and the app's answer is to redraw the grid.
	E(clientportal.ErrSlotNotOffered, http.StatusConflict, "this hour is no longer available"),
	E(clientportal.ErrNoClient, http.StatusForbidden, "this launch is not linked to a client"),
)

func (h *ClientHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	clientErrMap.Handle(ctx, w, op, err)
}
