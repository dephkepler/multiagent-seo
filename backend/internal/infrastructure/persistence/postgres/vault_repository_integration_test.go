//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"multiagent-seo/internal/domain/vault"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/internal/testsupport"
)

// Groups can only be deleted once emptied — this is the one real business
// rule the feature has, so pin it down end to end: create a group, put an
// entry in it, confirm ListGroups counts it, confirm DeleteGroup refuses
// while it's non-empty, then confirm it succeeds once the entry is gone.
func TestVaultRepository_DeleteGroup_RefusesUntilEmpty(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.NewTestDB(t, baseConnStr)
	repo := postgres.NewVaultRepository(pool)

	group, err := repo.CreateGroup(ctx, "Соцсети")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	entry, err := repo.Create(ctx, vault.CreateEntry{
		GroupID:   group.ID,
		Title:     "Facebook",
		Username:  "staff",
		Password:  "secret",
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("Create entry: %v", err)
	}

	groups, err := repo.ListGroups(ctx)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	var found bool
	for _, g := range groups {
		if g.ID == group.ID {
			found = true
			if g.EntryCount != 1 {
				t.Errorf("EntryCount = %d, want 1", g.EntryCount)
			}
		}
	}
	if !found {
		t.Fatalf("ListGroups did not include the created group")
	}

	if err := repo.DeleteGroup(ctx, group.ID); !errors.Is(err, vault.ErrGroupHasEntries) {
		t.Fatalf("DeleteGroup on non-empty group = %v, want ErrGroupHasEntries", err)
	}

	if err := repo.Delete(ctx, entry.ID); err != nil {
		t.Fatalf("Delete entry: %v", err)
	}

	if err := repo.DeleteGroup(ctx, group.ID); err != nil {
		t.Fatalf("DeleteGroup once empty: %v", err)
	}
}
