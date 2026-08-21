package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	domainvault "multiagent-seo/internal/domain/vault"
	"multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/infrastructure/http/response"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/logger"
	"multiagent-seo/pkg/validate"
)

type VaultService interface {
	Create(ctx context.Context, in domainvault.CreateEntry) (domainvault.Entry, error)
	List(ctx context.Context) ([]domainvault.Entry, error)
	Update(ctx context.Context, id uuid.UUID, in domainvault.UpdateEntry) (domainvault.Entry, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type VaultHandler struct {
	entries VaultService
}

func NewVaultHandler(entries VaultService) *VaultHandler {
	return &VaultHandler{entries: entries}
}

func (h *VaultHandler) ListVaultEntries(w http.ResponseWriter, r *http.Request) {
	if isNil(h.entries) {
		problem.Write(w, http.StatusServiceUnavailable, "vault unavailable")
		return
	}

	entries, err := h.entries.List(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, "list_vault_entries", err)
		return
	}
	out := make([]oapigen.VaultEntry, len(entries))
	for i, e := range entries {
		out[i] = toAPIVaultEntry(e)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *VaultHandler) CreateVaultEntry(w http.ResponseWriter, r *http.Request) {
	if isNil(h.entries) {
		problem.Write(w, http.StatusServiceUnavailable, "vault unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.CreateVaultEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.vault")
		log.Debug().Err(err).Msg("decode create vault entry body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	// Whoever is logged in to the admin panel — the only identity a
	// web-typed entry has, same as AddClientNote.
	createdBy, _ := middleware.UserIDFromContext(r.Context())

	entry, err := h.entries.Create(r.Context(), domainvault.CreateEntry{
		Title:     body.Title,
		URL:       strPtrValue(body.Url),
		Username:  strPtrValue(body.Username),
		Password:  strPtrValue(body.Password),
		Notes:     strPtrValue(body.Notes),
		CreatedBy: createdBy,
	})
	if err != nil {
		h.writeError(r.Context(), w, "create_vault_entry", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, toAPIVaultEntry(entry))
}

func (h *VaultHandler) UpdateVaultEntry(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.entries) {
		problem.Write(w, http.StatusServiceUnavailable, "vault unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.UpdateVaultEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.vault")
		log.Debug().Err(err).Msg("decode update vault entry body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	entry, err := h.entries.Update(r.Context(), id, domainvault.UpdateEntry{
		Title:    body.Title,
		URL:      body.Url,
		Username: body.Username,
		Password: body.Password,
		Notes:    body.Notes,
	})
	if err != nil {
		h.writeError(r.Context(), w, "update_vault_entry", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, toAPIVaultEntry(entry))
}

func (h *VaultHandler) DeleteVaultEntry(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.entries) {
		problem.Write(w, http.StatusServiceUnavailable, "vault unavailable")
		return
	}

	if err := h.entries.Delete(r.Context(), id); err != nil {
		h.writeError(r.Context(), w, "delete_vault_entry", err)
		return
	}
	response.NoContent(w)
}

var vaultErrMap = newErrMap("handlers.vault",
	E(domainvault.ErrNotFound, http.StatusNotFound, "vault entry not found"),
)

func (h *VaultHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	vaultErrMap.Handle(ctx, w, op, err)
}

func toAPIVaultEntry(e domainvault.Entry) oapigen.VaultEntry {
	return oapigen.VaultEntry{
		Id:        e.ID,
		Title:     e.Title,
		Url:       e.URL,
		Username:  e.Username,
		Password:  e.Password,
		Notes:     e.Notes,
		CreatedBy: e.CreatedBy,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}
