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
	List(ctx context.Context, groupID uuid.UUID) ([]domainvault.Entry, error)
	Update(ctx context.Context, id uuid.UUID, in domainvault.UpdateEntry) (domainvault.Entry, error)
	Delete(ctx context.Context, id uuid.UUID) error

	CreateGroup(ctx context.Context, name string) (domainvault.Group, error)
	ListGroups(ctx context.Context) ([]domainvault.GroupWithCount, error)
	DeleteGroup(ctx context.Context, id uuid.UUID) error
}

type VaultHandler struct {
	entries VaultService
}

func NewVaultHandler(entries VaultService) *VaultHandler {
	return &VaultHandler{entries: entries}
}

func (h *VaultHandler) ListVaultEntries(w http.ResponseWriter, r *http.Request, params oapigen.ListVaultEntriesParams) {
	if isNil(h.entries) {
		problem.Write(w, http.StatusServiceUnavailable, "vault unavailable")
		return
	}

	// groupId is optional at the OpenAPI/binding layer on purpose — a missing
	// required query param would 400 before the auth middleware chain even
	// runs (oapi-codegen validates params ahead of middleware), which would
	// let an unauthenticated request distinguish "no token" from "no token +
	// missing param" instead of just 401ing like every other route. Enforced
	// here instead, after auth has already run.
	if params.GroupId == nil {
		problem.Write(w, http.StatusBadRequest, "groupId is required")
		return
	}

	entries, err := h.entries.List(r.Context(), *params.GroupId)
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
		GroupID:   body.GroupId,
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

func (h *VaultHandler) ListVaultGroups(w http.ResponseWriter, r *http.Request) {
	if isNil(h.entries) {
		problem.Write(w, http.StatusServiceUnavailable, "vault unavailable")
		return
	}

	groups, err := h.entries.ListGroups(r.Context())
	if err != nil {
		h.writeError(r.Context(), w, "list_vault_groups", err)
		return
	}
	out := make([]oapigen.VaultGroup, len(groups))
	for i, g := range groups {
		out[i] = toAPIVaultGroup(g)
	}
	response.WriteJSON(r.Context(), w, http.StatusOK, out)
}

func (h *VaultHandler) CreateVaultGroup(w http.ResponseWriter, r *http.Request) {
	if isNil(h.entries) {
		problem.Write(w, http.StatusServiceUnavailable, "vault unavailable")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var body oapigen.CreateVaultGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log := logger.New(r.Context(), "handlers.vault")
		log.Debug().Err(err).Msg("decode create vault group body")
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validate.Validate(body); err != nil {
		problem.Write(w, http.StatusBadRequest, strings.Join(validate.MissingFields(err), ", "))
		return
	}

	group, err := h.entries.CreateGroup(r.Context(), body.Name)
	if err != nil {
		h.writeError(r.Context(), w, "create_vault_group", err)
		return
	}
	response.WriteJSON(r.Context(), w, http.StatusCreated, toAPIVaultGroup(domainvault.GroupWithCount{Group: group}))
}

func (h *VaultHandler) DeleteVaultGroup(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if isNil(h.entries) {
		problem.Write(w, http.StatusServiceUnavailable, "vault unavailable")
		return
	}

	if err := h.entries.DeleteGroup(r.Context(), id); err != nil {
		h.writeError(r.Context(), w, "delete_vault_group", err)
		return
	}
	response.NoContent(w)
}

var vaultErrMap = newErrMap("handlers.vault",
	E(domainvault.ErrNotFound, http.StatusNotFound, "vault entry not found"),
	E(domainvault.ErrGroupNotFound, http.StatusNotFound, "группа не найдена"),
	E(domainvault.ErrGroupHasEntries, http.StatusConflict, "в группе есть пароли — удалите их, затем удалите группу"),
)

func (h *VaultHandler) writeError(ctx context.Context, w http.ResponseWriter, op string, err error) {
	vaultErrMap.Handle(ctx, w, op, err)
}

func toAPIVaultEntry(e domainvault.Entry) oapigen.VaultEntry {
	return oapigen.VaultEntry{
		Id:        e.ID,
		GroupId:   e.GroupID,
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

func toAPIVaultGroup(g domainvault.GroupWithCount) oapigen.VaultGroup {
	return oapigen.VaultGroup{
		Id:         g.ID,
		Name:       g.Name,
		EntryCount: g.EntryCount,
		CreatedAt:  g.CreatedAt,
	}
}
