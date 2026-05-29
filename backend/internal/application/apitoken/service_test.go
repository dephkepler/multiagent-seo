package apitoken_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	appapitoken "multiagent-seo/internal/application/apitoken"
	domain "multiagent-seo/internal/domain/apitoken"
)

type fakeRepo struct {
	byHash map[string]uuid.UUID
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byHash: map[string]uuid.UUID{}} }

func (r *fakeRepo) Create(_ context.Context, t domain.NewToken) (domain.Token, error) {
	r.byHash[t.Hash] = t.UserID
	return domain.Token{ID: uuid.New(), UserID: t.UserID, Name: t.Name, Prefix: t.Prefix}, nil
}
func (r *fakeRepo) ListByUser(context.Context, uuid.UUID) ([]domain.Token, error) { return nil, nil }
func (r *fakeRepo) UserByHash(_ context.Context, hash string) (uuid.UUID, error) {
	uid, ok := r.byHash[hash]
	if !ok {
		return uuid.Nil, domain.ErrNotFound
	}
	return uid, nil
}
func (r *fakeRepo) Revoke(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func TestCreateThenAuthenticate(t *testing.T) {
	svc := appapitoken.NewService(newFakeRepo())
	user := uuid.New()

	tok, secret, err := svc.Create(context.Background(), user, "ci")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tok.Name != "ci" || secret == "" {
		t.Fatalf("bad create result: %+v secret=%q", tok, secret)
	}
	if !appapitoken.HasKeyPrefix(secret) {
		t.Errorf("secret %q lacks key prefix", secret)
	}

	got, err := svc.Authenticate(context.Background(), secret)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got != user {
		t.Errorf("authenticated user = %v, want %v", got, user)
	}
}

func TestAuthenticateRejectsUnknownAndNonKeys(t *testing.T) {
	svc := appapitoken.NewService(newFakeRepo())
	for _, tok := range []string{"mas_unknownsecret", "not-an-api-key", "eyJhbGci.jwt.looking"} {
		if _, err := svc.Authenticate(context.Background(), tok); err != appapitoken.ErrInvalidKey {
			t.Errorf("Authenticate(%q) err = %v, want ErrInvalidKey", tok, err)
		}
	}
}

func TestCreateRequiresName(t *testing.T) {
	svc := appapitoken.NewService(newFakeRepo())
	if _, _, err := svc.Create(context.Background(), uuid.New(), "  "); err != appapitoken.ErrNoName {
		t.Errorf("Create with blank name err = %v, want ErrNoName", err)
	}
}
