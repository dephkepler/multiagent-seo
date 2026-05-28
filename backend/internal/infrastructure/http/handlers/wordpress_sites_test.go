package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"

	apphealth "multiagent-seo/internal/application/health"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainhealth "multiagent-seo/internal/domain/health"
	domainwp "multiagent-seo/internal/domain/wordpress"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/config"
)

type fakeWordpressRepo struct {
	mu     sync.Mutex
	sites  map[uuid.UUID]domainwp.Site
	passes map[uuid.UUID]string
}

func newFakeRepo() *fakeWordpressRepo {
	return &fakeWordpressRepo{
		sites:  make(map[uuid.UUID]domainwp.Site),
		passes: make(map[uuid.UUID]string),
	}
}

func (r *fakeWordpressRepo) aliasTaken(alias string, except uuid.UUID) bool {
	for id, s := range r.sites {
		if id != except && s.Alias == alias {
			return true
		}
	}
	return false
}

func (r *fakeWordpressRepo) Create(_ context.Context, in domainwp.CreateSite) (domainwp.Site, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.aliasTaken(in.Alias, uuid.Nil) {
		return domainwp.Site{}, domainwp.ErrAliasExists
	}
	site := domainwp.Site{
		ID:       uuid.New(),
		Alias:    in.Alias,
		URL:      in.URL,
		Username: in.Username,
		Enabled:  true,
	}
	r.sites[site.ID] = site
	r.passes[site.ID] = in.AppPassword
	return site, nil
}

func (r *fakeWordpressRepo) List(context.Context) ([]domainwp.Site, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domainwp.Site, 0, len(r.sites))
	for _, s := range r.sites {
		out = append(out, s)
	}
	return out, nil
}

func (r *fakeWordpressRepo) Get(_ context.Context, id uuid.UUID) (domainwp.Site, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sites[id]
	if !ok {
		return domainwp.Site{}, domainwp.ErrNotFound
	}
	return s, nil
}

func (r *fakeWordpressRepo) Update(_ context.Context, id uuid.UUID, in domainwp.UpdateSite) (domainwp.Site, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sites[id]
	if !ok {
		return domainwp.Site{}, domainwp.ErrNotFound
	}
	if in.Alias != nil {
		if r.aliasTaken(*in.Alias, id) {
			return domainwp.Site{}, domainwp.ErrAliasExists
		}
		s.Alias = *in.Alias
	}
	if in.URL != nil {
		s.URL = *in.URL
	}
	if in.Username != nil {
		s.Username = *in.Username
	}
	if in.Enabled != nil {
		s.Enabled = *in.Enabled
	}
	if in.AppPassword != nil {
		r.passes[id] = *in.AppPassword
	}
	r.sites[id] = s
	return s, nil
}

func (r *fakeWordpressRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sites[id]; !ok {
		return domainwp.ErrNotFound
	}
	delete(r.sites, id)
	delete(r.passes, id)
	return nil
}

func newWordpressRouter(repo domainwp.Repository) http.Handler {
	healthHandler := handlers.NewHealthHandler(apphealth.NewService(domainhealth.NewService(stubRepo{})))
	wpSvc := appwordpress.NewService(repo)
	server := handlers.NewServer(healthHandler, handlers.NewWordpressSitesHandler(wpSvc), handlers.NewLoginHandler(nil))
	return apihttp.NewRouter(config.ServerConfig{
		Host:               "localhost",
		Port:               "0",
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)
}

func doJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
	return rec
}

func TestWordpress_CreateAndList(t *testing.T) {
	router := newWordpressRouter(newFakeRepo())

	rec := doJSON(t, router, http.MethodPost, "/wordpress-sites", oapigen.CreateWordpressSiteRequest{
		Alias:       "blog",
		Url:         "https://blog.example.com",
		Username:    "admin",
		AppPassword: "secret pass",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	var created oapigen.WordpressSite
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Alias != "blog" || created.Url != "https://blog.example.com" {
		t.Errorf("unexpected created site: %+v", created)
	}
	if created.Id == (uuid.UUID{}) {
		t.Errorf("expected non-zero id")
	}

	listRec := doJSON(t, router, http.MethodGet, "/wordpress-sites", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listRec.Code)
	}
	var list []oapigen.WordpressSite
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
}

func TestWordpress_CreateDuplicate(t *testing.T) {
	router := newWordpressRouter(newFakeRepo())
	req := oapigen.CreateWordpressSiteRequest{
		Alias:       "dup",
		Url:         "https://a.example.com",
		Username:    "admin",
		AppPassword: "p",
	}
	if rec := doJSON(t, router, http.MethodPost, "/wordpress-sites", req); rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201", rec.Code)
	}
	rec := doJSON(t, router, http.MethodPost, "/wordpress-sites", req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", rec.Code)
	}
}

func TestWordpress_CreateInvalid(t *testing.T) {
	router := newWordpressRouter(newFakeRepo())
	rec := doJSON(t, router, http.MethodPost, "/wordpress-sites", oapigen.CreateWordpressSiteRequest{
		Alias: "only-alias",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid create status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestWordpress_GetMissing(t *testing.T) {
	router := newWordpressRouter(newFakeRepo())
	rec := doJSON(t, router, http.MethodGet, "/wordpress-sites/"+uuid.NewString(), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, want 404", rec.Code)
	}
}

func TestWordpress_Update(t *testing.T) {
	repo := newFakeRepo()
	router := newWordpressRouter(repo)

	rec := doJSON(t, router, http.MethodPost, "/wordpress-sites", oapigen.CreateWordpressSiteRequest{
		Alias: "orig", Url: "https://x.example.com", Username: "u", AppPassword: "p",
	})
	var created oapigen.WordpressSite
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	newAlias := "renamed"
	enabled := false
	updRec := doJSON(t, router, http.MethodPatch, "/wordpress-sites/"+created.Id.String(), oapigen.UpdateWordpressSiteRequest{
		Alias:   &newAlias,
		Enabled: &enabled,
	})
	if updRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200 (body=%s)", updRec.Code, updRec.Body.String())
	}
	var updated oapigen.WordpressSite
	if err := json.NewDecoder(updRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated.Alias != "renamed" || updated.Enabled {
		t.Errorf("update not applied: %+v", updated)
	}
	if updated.Url != "https://x.example.com" {
		t.Errorf("absent field changed: url=%q", updated.Url)
	}
}

func TestWordpress_Delete(t *testing.T) {
	repo := newFakeRepo()
	router := newWordpressRouter(repo)

	rec := doJSON(t, router, http.MethodPost, "/wordpress-sites", oapigen.CreateWordpressSiteRequest{
		Alias: "todelete", Url: "https://d.example.com", Username: "u", AppPassword: "p",
	})
	var created oapigen.WordpressSite
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	delRec := doJSON(t, router, http.MethodDelete, "/wordpress-sites/"+created.Id.String(), nil)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", delRec.Code)
	}

	missRec := doJSON(t, router, http.MethodDelete, "/wordpress-sites/"+created.Id.String(), nil)
	if missRec.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d, want 404", missRec.Code)
	}
}
