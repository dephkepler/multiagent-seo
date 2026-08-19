package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	domain "multiagent-seo/internal/domain/clientdetail"
	"multiagent-seo/internal/domain/consultations"
	"multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

type clientDetailService interface {
	Get(ctx context.Context, clientID string) (domain.Detail, error)
	UpdateClient(ctx context.Context, clientID string, edit consultations.ClientEdit) error
	AddNote(ctx context.Context, clientID, text, createdBy string) (domain.Note, error)
	Delete(ctx context.Context, clientID string) error
}

type ClientDetailHandler struct {
	svc clientDetailService
}

func NewClientDetailHandler(svc clientDetailService) *ClientDetailHandler {
	return &ClientDetailHandler{svc: svc}
}

func (h *ClientDetailHandler) GetClientDetail(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "client detail unavailable")
		return
	}

	d, err := h.svc.Get(r.Context(), id.String())
	if err != nil {
		h.writeError(r.Context(), w, "get_client_detail", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toAPIClientDetail(d))
}

func (h *ClientDetailHandler) UpdateClient(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "client detail unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.UpdateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.clientdetail")
		log.Debug().Err(err).Msg("decode update client body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}

	edit := consultations.ClientEdit{
		LastName:    body.LastName,
		FirstName:   body.FirstName,
		Patronymic:  body.Patronymic,
		Phone:       body.Phone,
		Gender:      string(body.Gender),
		Email:       body.Email,
		ClientType:  string(body.ClientType),
		CompanyName: body.CompanyName,
		CompanyCode: body.CompanyCode,
		Address:     body.Address,
		Birthdate:   body.Birthdate,
		TaxID:       body.TaxId,
	}
	if err := h.svc.UpdateClient(r.Context(), id.String(), edit); err != nil {
		h.writeError(r.Context(), w, "update_client", err)
		return
	}
	response.NoContent(w)
}

func (h *ClientDetailHandler) AddClientNote(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "client detail unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.AddClientNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.clientdetail")
		log.Debug().Err(err).Msg("decode add note body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Whoever is logged in to the admin panel — the only identity a
	// web-typed note has, unlike bot-side records which carry a Telegram id.
	createdBy, _ := middleware.UserIDFromContext(r.Context())

	n, err := h.svc.AddNote(r.Context(), id.String(), body.Text, createdBy)
	if err != nil {
		h.writeError(r.Context(), w, "add_client_note", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, oapigen.ClientNote{
		Id:        n.ID,
		Text:      n.Text,
		CreatedBy: n.CreatedBy,
		CreatedAt: n.CreatedAt,
	})
}

func (h *ClientDetailHandler) DeleteClient(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "client detail unavailable")
		return
	}

	if err := h.svc.Delete(r.Context(), id.String()); err != nil {
		h.writeError(r.Context(), w, "delete_client", err)
		return
	}
	response.NoContent(w)
}

var clientDetailErrMap = newErrMap("handlers.clientdetail",
	E(domain.ErrNotFound, http.StatusNotFound, "client not found"),
	EMsg(domain.ErrEmptyNote, http.StatusBadRequest),
	EMsg(domain.ErrInvalidGender, http.StatusBadRequest),
	EMsg(domain.ErrInvalidClientType, http.StatusBadRequest),
	EMsg(domain.ErrHasHistory, http.StatusConflict),
)

func (h *ClientDetailHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	clientDetailErrMap.Handle(ctx, w, op, err)
}

func toAPIClientDetail(d domain.Detail) oapigen.ClientDetail {
	leads := make([]oapigen.ClientLead, len(d.Leads))
	for i, l := range d.Leads {
		leads[i] = oapigen.ClientLead{Id: l.ID, ReceivedAt: l.ReceivedAt, Message: l.Message, Page: l.Page}
	}
	consultations := make([]oapigen.ClientConsultation, len(d.Consultations))
	for i, c := range d.Consultations {
		consultations[i] = oapigen.ClientConsultation{
			Id:          c.ID,
			ScheduledAt: c.ScheduledAt,
			Price:       float32(c.Price),
			Status:      oapigen.ClientConsultationStatus(c.Status),
			CaseNote:    c.CaseNote,
		}
	}
	cases := make([]oapigen.ClientCase, len(d.Cases))
	for i, c := range d.Cases {
		cases[i] = oapigen.ClientCase{
			Id:          c.ID,
			Description: c.Description,
			Category:    c.Category,
			Status:      oapigen.ClientCaseStatus(c.Status),
			Fee:         float32(c.Fee),
			Paid:        float32(c.Paid),
			CreatedAt:   c.CreatedAt,
		}
	}
	notes := make([]oapigen.ClientNote, len(d.Notes))
	for i, n := range d.Notes {
		notes[i] = oapigen.ClientNote{Id: n.ID, Text: n.Text, CreatedBy: n.CreatedBy, CreatedAt: n.CreatedAt}
	}

	return oapigen.ClientDetail{
		Client: oapigen.ClientDetailInfo{
			Id:          d.Client.ID,
			Name:        d.Client.Name,
			LastName:    d.Client.LastName,
			FirstName:   d.Client.FirstName,
			Patronymic:  d.Client.Patronymic,
			Gender:      oapigen.ClientDetailInfoGender(d.Client.Gender),
			Phone:       d.Client.Phone,
			Email:       d.Client.Email,
			ClientType:  oapigen.ClientDetailInfoClientType(d.Client.ClientType),
			CompanyName: d.Client.CompanyName,
			CompanyCode: d.Client.CompanyCode,
			Address:     d.Client.Address,
			Birthdate:   d.Client.Birthdate,
			TaxId:       d.Client.TaxID,
			FirstSeenAt: d.Client.FirstSeenAt,
			LastSeenAt:  d.Client.LastSeenAt,
		},
		RevenueTotal:  float32(d.RevenueTotal),
		Leads:         leads,
		Consultations: consultations,
		Cases:         cases,
		Notes:         notes,
	}
}
