package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	domain "multiagent-seo/internal/domain/advocateview"
	"multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
)

type advocateViewService interface {
	Cases(ctx context.Context, advocateID string) ([]domain.Case, error)
	Clients(ctx context.Context, advocateID string) ([]domain.Client, error)
	Client(ctx context.Context, advocateID, clientID string) (domain.Card, error)
	AddNote(ctx context.Context, advocateID, clientID, text, createdBy string) (domain.Note, error)
	SetCaseStatus(ctx context.Context, advocateID, caseID, status string) error
	Settlement(ctx context.Context, advocateID string) (domain.Settlement, error)
	Stats(ctx context.Context, advocateID string) (domain.Stats, error)
}

// MyHandler serves the advocate's own section. The advocate is never a request
// parameter: it comes from the principal the auth middleware resolved, so there
// is nothing here to point at somebody else's rows.
type MyHandler struct {
	svc advocateViewService
}

func NewMyHandler(svc advocateViewService) *MyHandler {
	return &MyHandler{svc: svc}
}

func (h *MyHandler) ListMyCases(w http.ResponseWriter, r *http.Request) {
	advocateID, ok := h.advocate(w, r)
	if !ok {
		return
	}
	list, err := h.svc.Cases(r.Context(), advocateID)
	if err != nil {
		h.writeError(r.Context(), w, "my_cases", err)
		return
	}

	out := oapigen.MyCaseList{Items: make([]oapigen.MyCase, len(list))}
	for i, c := range list {
		out.Items[i] = toAPIMyCase(c)
		out.TotalFee += c.Fee
		out.TotalPaid += c.Paid
		out.TotalOwed += c.Owed()
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *MyHandler) SetMyCaseStatus(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	advocateID, ok := h.advocate(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.SetMyCaseStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.my")
		log.Debug().Err(err).Msg("decode case status body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.SetCaseStatus(r.Context(), advocateID, id.String(), string(body.Status)); err != nil {
		h.writeError(r.Context(), w, "my_case_status", err)
		return
	}
	response.NoContent(w)
}

func (h *MyHandler) ListMyClients(w http.ResponseWriter, r *http.Request) {
	advocateID, ok := h.advocate(w, r)
	if !ok {
		return
	}
	list, err := h.svc.Clients(r.Context(), advocateID)
	if err != nil {
		h.writeError(r.Context(), w, "my_clients", err)
		return
	}

	out := oapigen.MyClientList{Items: make([]oapigen.MyClient, len(list))}
	for i, c := range list {
		out.Items[i] = toAPIMyClient(c)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *MyHandler) GetMyClient(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	advocateID, ok := h.advocate(w, r)
	if !ok {
		return
	}
	card, err := h.svc.Client(r.Context(), advocateID, id.String())
	if err != nil {
		h.writeError(r.Context(), w, "my_client", err)
		return
	}

	out := oapigen.MyClientCard{
		Client:        toAPIMyClient(card.Client),
		Cases:         make([]oapigen.MyCase, len(card.Cases)),
		Consultations: make([]oapigen.MyConsultation, len(card.Consultations)),
		Notes:         make([]oapigen.ClientNote, len(card.Notes)),
	}
	for i, c := range card.Cases {
		out.Cases[i] = toAPIMyCase(c)
	}
	for i, c := range card.Consultations {
		out.Consultations[i] = oapigen.MyConsultation{
			Id:          c.ID,
			ScheduledAt: c.ScheduledAt,
			Price:       c.Price,
			Status:      c.Status,
			CaseNote:    c.CaseNote,
		}
	}
	for i, n := range card.Notes {
		out.Notes[i] = oapigen.ClientNote{
			Id:        n.ID,
			Text:      n.Text,
			CreatedBy: n.CreatedBy,
			CreatedAt: n.CreatedAt,
		}
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *MyHandler) AddMyClientNote(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	advocateID, ok := h.advocate(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.AddClientNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.my")
		log.Debug().Err(err).Msg("decode add note body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// The note is stamped with the login, same as an admin's — so the card
	// shows who wrote it, and the advocate cannot write as somebody else.
	createdBy, _ := middleware.UserIDFromContext(r.Context())

	n, err := h.svc.AddNote(r.Context(), advocateID, id.String(), body.Text, createdBy)
	if err != nil {
		h.writeError(r.Context(), w, "my_client_note", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, oapigen.ClientNote{
		Id:        n.ID,
		Text:      n.Text,
		CreatedBy: n.CreatedBy,
		CreatedAt: n.CreatedAt,
	})
}

func (h *MyHandler) GetMySettlement(w http.ResponseWriter, r *http.Request) {
	advocateID, ok := h.advocate(w, r)
	if !ok {
		return
	}
	s, err := h.svc.Settlement(r.Context(), advocateID)
	if err != nil {
		h.writeError(r.Context(), w, "my_settlement", err)
		return
	}

	out := oapigen.MySettlement{
		AdvocateId:        s.AdvocateID,
		FullName:          s.FullName,
		CommissionPercent: s.CommissionPercent,
		Collected:         s.Collected,
		Accrued:           s.Accrued,
		Paid:              s.Paid,
		Outstanding:       s.Outstanding,
		PaidIsPartial:     s.PaidIsPartial,
		Months:            toAPIMyMonths(s.Months),
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *MyHandler) GetMyStats(w http.ResponseWriter, r *http.Request) {
	advocateID, ok := h.advocate(w, r)
	if !ok {
		return
	}
	stats, err := h.svc.Stats(r.Context(), advocateID)
	if err != nil {
		h.writeError(r.Context(), w, "my_stats", err)
		return
	}

	out := oapigen.MyStats{
		Cases:      stats.Cases,
		Clients:    stats.Clients,
		FeeTotal:   stats.FeeTotal,
		PaidTotal:  stats.PaidTotal,
		ClientDebt: stats.ClientDebt,
		AvgFee:     stats.AvgFee,
		ByStatus:   make([]oapigen.MyStatusCount, len(stats.ByStatus)),
		Months:     toAPIMyMonths(stats.Months),
	}
	for i, s := range stats.ByStatus {
		out.ByStatus[i] = oapigen.MyStatusCount{Status: s.Status, Count: s.Count}
	}
	if !stats.FirstCaseAt.IsZero() {
		out.FirstCaseAt = &stats.FirstCaseAt
	}
	if !stats.LastPaymentAt.IsZero() {
		out.LastPaymentAt = &stats.LastPaymentAt
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

// advocate is the identity every endpoint in this file is scoped by. An admin
// reaches these routes too (they are in the operation's scopes so the section
// can be inspected without a second account) but an admin login has no roster
// row, and answering with "everything" would quietly turn the advocate section
// into the admin one.
func (h *MyHandler) advocate(w http.ResponseWriter, r *http.Request) (string, bool) {
	if isNil(h.svc) {
		problem.Write(w, http.StatusServiceUnavailable, "advocate view unavailable")
		return "", false
	}
	p, ok := middleware.PrincipalFromContext(r.Context())
	if !ok || p.AdvocateID == "" {
		problem.Write(w, http.StatusForbidden, "this login is not linked to an advocate")
		return "", false
	}
	return p.AdvocateID, true
}

var myErrMap = newErrMap(
	"handlers.my",
	E(domain.ErrNotFound, http.StatusNotFound, "not found"),
	EMsg(domain.ErrStatusNotAllowed, http.StatusBadRequest),
	EMsg(domain.ErrEmptyNote, http.StatusBadRequest),
	EMsg(domain.ErrNoteTooLong, http.StatusBadRequest),
	E(domain.ErrNoAdvocate, http.StatusForbidden, "this login is not linked to an advocate"),
)

func (h *MyHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	myErrMap.Handle(ctx, w, op, err)
}

func toAPIMyCase(c domain.Case) oapigen.MyCase {
	out := oapigen.MyCase{
		Id:          c.ID,
		ClientId:    c.ClientID,
		ClientName:  c.ClientName,
		ClientPhone: c.ClientPhone,
		Category:    c.Category,
		Status:      c.Status,
		Description: c.Description,
		Fee:         c.Fee,
		Paid:        c.Paid,
		Owed:        c.Owed(),
		CreatedAt:   c.CreatedAt,
		Payments:    make([]oapigen.MyCasePayment, len(c.Payments)),
	}
	for i, p := range c.Payments {
		out.Payments[i] = oapigen.MyCasePayment{
			Id:     p.ID,
			Amount: p.Amount,
			PaidAt: openapi_types.Date{Time: p.PaidAt},
		}
	}
	return out
}

func toAPIMyClient(c domain.Client) oapigen.MyClient {
	owed := c.Fee - c.Paid
	if owed < 0 {
		owed = 0
	}
	return oapigen.MyClient{
		Id:         c.ID,
		Name:       c.Name,
		Phone:      c.Phone,
		Cases:      c.Cases,
		Fee:        c.Fee,
		Paid:       c.Paid,
		Owed:       owed,
		LastCaseAt: c.LastCaseAt,
	}
}

func toAPIMyMonths(months []domain.MonthMoney) []oapigen.MyMonthMoney {
	out := make([]oapigen.MyMonthMoney, len(months))
	for i, m := range months {
		out[i] = oapigen.MyMonthMoney{Month: m.Month, Collected: m.Collected, Accrued: m.Accrued}
	}
	return out
}
