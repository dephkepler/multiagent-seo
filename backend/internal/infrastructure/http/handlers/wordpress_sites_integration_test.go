//go:build integration

package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	apphealth "multiagent-seo/internal/application/health"
	appwordpress "multiagent-seo/internal/application/wordpress"
	domainhealth "multiagent-seo/internal/domain/health"
	apihttp "multiagent-seo/internal/infrastructure/http"
	"multiagent-seo/internal/infrastructure/http/handlers"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/internal/testsupport"
	"multiagent-seo/pkg/config"
)

func itWPServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()
	repo := postgres.NewWordpressSiteRepository(pool, "test-enc-key")
	svc := appwordpress.NewService(repo)

	healthHandler := handlers.NewHealthHandler(apphealth.NewService(domainhealth.NewService(stubRepo{})))
	server := handlers.NewServer(handlers.Deps{
		Health:    healthHandler,
		Wordpress: handlers.NewWordpressSitesHandler(svc),
	})
	router := apihttp.NewRouter(config.ServerConfig{
		BasePath:           "/",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
	}, server, nil)

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv
}

func itWPDo(t *testing.T, srv *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
		rdr = &buf
	}
	req, err := http.NewRequest(method, srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func itWPDecode(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func itWPBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestWordpressSitesCRUD_Integration(t *testing.T) {
	pool := testsupport.NewTestDB(t, baseConnStr)
	srv := itWPServer(t, pool)

	adminUsername := "admin"
	appPassword := "super secret app password"
	createResp := itWPDo(t, srv, http.MethodPost, "/wordpress-sites", oapigen.CreateWordpressSiteRequest{
		Alias:       "blog",
		Provider:    oapigen.Wordpress,
		Url:         "https://blog.example.com",
		Username:    &adminUsername,
		AppPassword: &appPassword,
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201 (body=%s)", createResp.StatusCode, itWPBody(t, createResp))
	}
	var created oapigen.WordpressSite
	itWPDecode(t, createResp, &created)
	if created.Id == (oapigen.WordpressSite{}).Id {
		t.Fatal("expected non-zero id from create")
	}
	if created.Alias != "blog" || created.Url != "https://blog.example.com" || !created.Enabled {
		t.Fatalf("unexpected created site: %+v", created)
	}
	id := created.Id.String()

	listResp := itWPDo(t, srv, http.MethodGet, "/wordpress-sites", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (body=%s)", listResp.StatusCode, itWPBody(t, listResp))
	}
	var list []oapigen.WordpressSite
	itWPDecode(t, listResp, &list)
	if len(list) != 1 || list[0].Id != created.Id {
		t.Fatalf("list = %+v, want exactly the created site", list)
	}

	getResp := itWPDo(t, srv, http.MethodGet, "/wordpress-sites/"+id, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200 (body=%s)", getResp.StatusCode, itWPBody(t, getResp))
	}
	var fetched oapigen.WordpressSite
	itWPDecode(t, getResp, &fetched)
	if fetched.Id != created.Id || fetched.Alias != "blog" {
		t.Fatalf("get returned %+v, want created site", fetched)
	}

	newAlias := "renamed-blog"
	patchResp := itWPDo(t, srv, http.MethodPatch, "/wordpress-sites/"+id, oapigen.UpdateWordpressSiteRequest{
		Alias: &newAlias,
	})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want 200 (body=%s)", patchResp.StatusCode, itWPBody(t, patchResp))
	}
	var patched oapigen.WordpressSite
	itWPDecode(t, patchResp, &patched)
	if patched.Alias != newAlias {
		t.Fatalf("patch alias = %q, want %q", patched.Alias, newAlias)
	}
	if patched.Url != "https://blog.example.com" {
		t.Fatalf("patch changed an absent field: url=%q", patched.Url)
	}

	reGetResp := itWPDo(t, srv, http.MethodGet, "/wordpress-sites/"+id, nil)
	if reGetResp.StatusCode != http.StatusOK {
		t.Fatalf("re-get status = %d, want 200", reGetResp.StatusCode)
	}
	var reFetched oapigen.WordpressSite
	itWPDecode(t, reGetResp, &reFetched)
	if reFetched.Alias != newAlias {
		t.Fatalf("re-get alias = %q, want persisted %q", reFetched.Alias, newAlias)
	}

	delResp := itWPDo(t, srv, http.MethodDelete, "/wordpress-sites/"+id, nil)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204 (body=%s)", delResp.StatusCode, itWPBody(t, delResp))
	}
	delResp.Body.Close()

	goneResp := itWPDo(t, srv, http.MethodGet, "/wordpress-sites/"+id, nil)
	defer goneResp.Body.Close()
	if goneResp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", goneResp.StatusCode)
	}
}
