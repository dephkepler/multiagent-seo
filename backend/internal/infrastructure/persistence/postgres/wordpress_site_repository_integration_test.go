//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"multiagent-seo/internal/domain/wordpress"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

const testEncKey = "test-encryption-key"

func TestWordpressSiteRepository_CreateEncryptsAppPassword(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewWordpressSiteRepository(pool, testEncKey)

	const appPassword = "super secret app password"
	site, err := repo.Create(ctx, wordpress.CreateSite{
		Alias:       "blog",
		URL:         "https://blog.example.com",
		Username:    "admin",
		AppPassword: appPassword,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if site.Alias != "blog" || site.URL != "https://blog.example.com" || site.Username != "admin" {
		t.Errorf("returned site fields mismatch: %+v", site)
	}

	var stored []byte
	if err := pool.QueryRow(ctx,
		`SELECT app_password FROM wordpress_sites WHERE id = $1`, site.ID).Scan(&stored); err != nil {
		t.Fatalf("read raw app_password: %v", err)
	}
	if len(stored) == 0 {
		t.Fatalf("stored app_password is empty")
	}
	if string(stored) == appPassword {
		t.Fatalf("app_password stored as plaintext")
	}
}

func TestWordpressSiteRepository_CredentialsRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewWordpressSiteRepository(testsupport.NewTestDB(t, baseConnStr), testEncKey)

	const appPassword = "abcd efgh ijkl mnop"
	site, err := repo.Create(ctx, wordpress.CreateSite{
		Alias:       "creds",
		URL:         "https://creds.example.com",
		Username:    "editor",
		AppPassword: appPassword,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	creds, err := repo.Credentials(ctx, site.ID)
	if err != nil {
		t.Fatalf("Credentials: %v", err)
	}
	if creds.AppPassword != appPassword {
		t.Errorf("decrypted AppPassword = %q, want %q", creds.AppPassword, appPassword)
	}
	if creds.URL != "https://creds.example.com" {
		t.Errorf("URL = %q", creds.URL)
	}
	if creds.Username != "editor" {
		t.Errorf("Username = %q", creds.Username)
	}
}

func TestWordpressSiteRepository_GetAndList(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewWordpressSiteRepository(testsupport.NewTestDB(t, baseConnStr), testEncKey)

	site, err := repo.Create(ctx, wordpress.CreateSite{
		Alias: "one", URL: "https://one.example.com", Username: "u", AppPassword: "p",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, site.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != site.ID || got.Alias != "one" {
		t.Errorf("Get mismatch: %+v", got)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != site.ID {
		t.Fatalf("List = %+v, want single site %v", list, site.ID)
	}
}

func TestWordpressSiteRepository_GetNotFound(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewWordpressSiteRepository(testsupport.NewTestDB(t, baseConnStr), testEncKey)

	_, err := repo.Get(ctx, uuid.New())
	if !errors.Is(err, wordpress.ErrNotFound) {
		t.Fatalf("Get missing id err = %v, want ErrNotFound", err)
	}
}

func TestWordpressSiteRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewWordpressSiteRepository(testsupport.NewTestDB(t, baseConnStr), testEncKey)

	site, err := repo.Create(ctx, wordpress.CreateSite{
		Alias: "before", URL: "https://before.example.com", Username: "u", AppPassword: "p",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newAlias := "after"
	updated, err := repo.Update(ctx, site.ID, wordpress.UpdateSite{Alias: &newAlias})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Alias != "after" {
		t.Errorf("Alias = %q, want %q", updated.Alias, "after")
	}
	if updated.URL != "https://before.example.com" || updated.Username != "u" {
		t.Errorf("Update clobbered untouched fields: %+v", updated)
	}
}

func TestWordpressSiteRepository_DeleteSoftDeletes(t *testing.T) {
	ctx := context.Background()
	repo := postgres.NewWordpressSiteRepository(testsupport.NewTestDB(t, baseConnStr), testEncKey)

	site, err := repo.Create(ctx, wordpress.CreateSite{
		Alias: "doomed", URL: "https://doomed.example.com", Username: "u", AppPassword: "p",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, site.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.Get(ctx, site.ID); !errors.Is(err, wordpress.ErrNotFound) {
		t.Errorf("Get after delete err = %v, want ErrNotFound", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List after delete = %+v, want empty", list)
	}

	if err := repo.Delete(ctx, site.ID); !errors.Is(err, wordpress.ErrNotFound) {
		t.Errorf("second Delete err = %v, want ErrNotFound", err)
	}
}
