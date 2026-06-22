package linkbuilding_test

import (
	"context"
	"testing"

	applb "multiagent-seo/internal/application/linkbuilding"
	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

type fakeCredSource struct {
	creds   []domain.SiteCredential
	written []domain.LoginResult
}

func (f *fakeCredSource) ListCredentials(context.Context, string) ([]domain.SiteCredential, error) {
	return f.creds, nil
}

func (f *fakeCredSource) WriteLoginStatus(_ context.Context, _ string, r []domain.LoginResult) error {
	f.written = append(f.written, r...)
	return nil
}

type scriptedAuth struct {
	fn func(domain.SiteCredential) (domain.LoginResult, error)
}

func (a scriptedAuth) Login(_ context.Context, c domain.SiteCredential) (domain.LoginResult, error) {
	return a.fn(c)
}

func newLoginSvc(creds *fakeCredSource, auth scriptedAuth) *applb.LoginService {
	return applb.NewLoginService(creds, auth, jobrunner.NewSyncRunner(), nil, applb.WithLoginDelay(0, 0))
}

func loginByRow(results []domain.LoginResult) map[int]domain.LoginResult {
	m := make(map[int]domain.LoginResult, len(results))
	for _, r := range results {
		m[r.Row] = r
	}
	return m
}

func TestLoginToSites_WritesEachStatus(t *testing.T) {
	creds := &fakeCredSource{creds: []domain.SiteCredential{
		{Row: 2, BaseURL: "https://a.com", Login: "u", Password: "p"},
		{Row: 3, BaseURL: "https://b.com", Login: "u", Password: "p"},
	}}
	auth := scriptedAuth{fn: func(c domain.SiteCredential) (domain.LoginResult, error) {
		return domain.LoginResult{Row: c.Row, BaseURL: c.BaseURL, OK: true, Status: "login ok"}, nil
	}}

	res, err := newLoginSvc(creds, auth).LoginToSites(context.Background(), applb.LoginRequest{Sheet: "WEBSITES"})
	if err != nil {
		t.Fatalf("LoginToSites: %v", err)
	}
	if res.SitesQueued != 2 {
		t.Fatalf("queued = %d, want 2", res.SitesQueued)
	}
	if len(creds.written) != 2 {
		t.Fatalf("written = %d, want 2", len(creds.written))
	}
	if got := loginByRow(creds.written)[2]; !got.OK || got.Status != "login ok" {
		t.Errorf("row 2 = %+v, want ok", got)
	}
}

func TestLoginToSites_MixedOutcomes(t *testing.T) {
	creds := &fakeCredSource{creds: []domain.SiteCredential{
		{Row: 2, BaseURL: "https://ok.com", Login: "u", Password: "p"},
		{Row: 3, BaseURL: "https://bad.com", Login: "u", Password: "p"},
	}}
	auth := scriptedAuth{fn: func(c domain.SiteCredential) (domain.LoginResult, error) {
		if c.BaseURL == "https://bad.com" {
			return domain.LoginResult{Row: c.Row, BaseURL: c.BaseURL, OK: false, Status: "login failed"}, nil
		}
		return domain.LoginResult{Row: c.Row, BaseURL: c.BaseURL, OK: true, Status: "login ok"}, nil
	}}

	if _, err := newLoginSvc(creds, auth).LoginToSites(context.Background(), applb.LoginRequest{Sheet: "WEBSITES"}); err != nil {
		t.Fatalf("LoginToSites: %v", err)
	}
	rows := loginByRow(creds.written)
	if !rows[2].OK {
		t.Errorf("row 2 should be ok: %+v", rows[2])
	}
	if rows[3].OK || rows[3].Status != "login failed" {
		t.Errorf("row 3 should be failed: %+v", rows[3])
	}
}

func TestLoginToSites_CancellationAbortsAndKeepsPartial(t *testing.T) {
	creds := &fakeCredSource{creds: []domain.SiteCredential{
		{Row: 2, BaseURL: "https://a.com", Login: "u", Password: "p"},
		{Row: 3, BaseURL: "https://b.com", Login: "u", Password: "p"},
		{Row: 4, BaseURL: "https://c.com", Login: "u", Password: "p"},
	}}
	auth := scriptedAuth{fn: func(c domain.SiteCredential) (domain.LoginResult, error) {
		if c.BaseURL == "https://b.com" {
			return domain.LoginResult{Row: c.Row, BaseURL: c.BaseURL}, context.Canceled
		}
		return domain.LoginResult{Row: c.Row, BaseURL: c.BaseURL, OK: true, Status: "login ok"}, nil
	}}

	if _, err := newLoginSvc(creds, auth).LoginToSites(context.Background(), applb.LoginRequest{Sheet: "WEBSITES"}); err != nil {
		t.Fatalf("LoginToSites: %v", err)
	}
	if len(creds.written) != 1 {
		t.Fatalf("written = %d, want 1 (only the pre-cancel success)", len(creds.written))
	}
	if creds.written[0].Row != 2 || !creds.written[0].OK {
		t.Errorf("persisted partial = %+v, want row 2 ok", creds.written[0])
	}
}

func TestLoginToSites_EmptyInventory(t *testing.T) {
	creds := &fakeCredSource{}
	auth := scriptedAuth{fn: func(domain.SiteCredential) (domain.LoginResult, error) {
		t.Fatal("auth must not be called for an empty inventory")
		return domain.LoginResult{}, nil
	}}

	res, err := newLoginSvc(creds, auth).LoginToSites(context.Background(), applb.LoginRequest{Sheet: "WEBSITES"})
	if err != nil {
		t.Fatalf("LoginToSites: %v", err)
	}
	if res.SitesQueued != 0 || len(creds.written) != 0 {
		t.Errorf("empty inventory: queued=%d written=%d, want 0/0", res.SitesQueued, len(creds.written))
	}
}

func TestLoginToSites_RequiresSheet(t *testing.T) {
	auth := scriptedAuth{fn: func(domain.SiteCredential) (domain.LoginResult, error) {
		return domain.LoginResult{}, nil
	}}
	if _, err := newLoginSvc(&fakeCredSource{}, auth).LoginToSites(context.Background(), applb.LoginRequest{}); err != applb.ErrNoSheet {
		t.Errorf("missing sheet err = %v, want ErrNoSheet", err)
	}
}
