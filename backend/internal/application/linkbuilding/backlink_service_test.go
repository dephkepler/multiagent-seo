package linkbuilding_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	applb "multiagent-seo/internal/application/linkbuilding"
	domain "multiagent-seo/internal/domain/linkbuilding"
	"multiagent-seo/pkg/jobrunner"
)

type fakePlacements struct {
	written []domain.PlacementResult
}

func (f *fakePlacements) WritePlacementStatus(_ context.Context, _ string, r []domain.PlacementResult) error {
	f.written = append(f.written, r...)
	return nil
}

type fakePlacementStore struct {
	saved []domain.Placement
}

func (f *fakePlacementStore) Save(_ context.Context, p domain.Placement) error {
	f.saved = append(f.saved, p)
	return nil
}
func (f *fakePlacementStore) ListByRun(_ context.Context, runID string) ([]domain.Placement, error) {
	var out []domain.Placement
	for _, p := range f.saved {
		if p.RunID == runID {
			out = append(out, p)
		}
	}
	return out, nil
}

type fakeDonorStore struct {
	saved   []domain.DonorCredential
	preload map[string]domain.DonorCredential
}

func (f *fakeDonorStore) Get(_ context.Context, donorURL string) (domain.DonorCredential, bool, error) {
	c, ok := f.preload[donorURL]
	return c, ok, nil
}
func (f *fakeDonorStore) Save(_ context.Context, c domain.DonorCredential) error {
	f.saved = append(f.saved, c)
	return nil
}

type scriptedIssuer struct {
	fn func(donorURL, login, password string) (string, error)
}

func (s scriptedIssuer) IssueAppPassword(_ context.Context, donorURL, login, password string) (string, error) {
	return s.fn(donorURL, login, password)
}

type scriptedEditor struct {
	latest func(domain.DonorCredential) (domain.DonorPost, error)
	update func(domain.DonorCredential, int64, string) error
	posted []string
}

func (s *scriptedEditor) LatestPost(_ context.Context, c domain.DonorCredential) (domain.DonorPost, error) {
	return s.latest(c)
}
func (s *scriptedEditor) UpdatePostContent(_ context.Context, c domain.DonorCredential, id int64, html string) error {
	s.posted = append(s.posted, html)
	if s.update == nil {
		return nil
	}
	return s.update(c, id, html)
}

type scriptedPlacer struct {
	fn func(html, target string) (domain.BacklinkInsertion, error)
}

func (s scriptedPlacer) Place(_ context.Context, html, target string) (domain.BacklinkInsertion, error) {
	return s.fn(html, target)
}

type fakeTargets struct {
	url string
	err error
}

func (f fakeTargets) ResolveSiteURL(_ context.Context, _ string) (string, error) { return f.url, f.err }

func newBacklinkSvc(
	creds *fakeCredSource,
	pl *fakePlacements,
	ds *fakeDonorStore,
	is scriptedIssuer,
	ed *scriptedEditor,
	pr scriptedPlacer,
	tg fakeTargets,
) *applb.BacklinkService {
	placerBuilder := func(string, string) (domain.BacklinkPlacer, error) { return pr, nil }
	return applb.NewBacklinkService(creds, pl, &fakePlacementStore{}, ds, is, ed, placerBuilder, applb.LLMDefaults{}, tg, jobrunner.NewSyncRunner(), nil, applb.WithBacklinkDelay(0, 0))
}

func TestPlaceBacklinks_HappyPathIssuesAndCachesCreds(t *testing.T) {
	creds := &fakeCredSource{creds: []domain.SiteCredential{
		{Row: 2, BaseURL: "https://donor.example", Login: "admin", Password: "raw"},
	}}
	pl := &fakePlacements{}
	store := &fakeDonorStore{preload: map[string]domain.DonorCredential{}}
	issuer := scriptedIssuer{fn: func(d, l, p string) (string, error) { return "AAAA BBBB CCCC", nil }}
	editor := &scriptedEditor{latest: func(domain.DonorCredential) (domain.DonorPost, error) {
		return domain.DonorPost{ID: 42, Content: "<p>hello world</p>", PublicURL: "https://donor.example/hello-post/", EditURL: "https://donor.example/wp-admin/post.php?post=42&action=edit"}, nil
	}}
	placer := scriptedPlacer{fn: func(html, target string) (domain.BacklinkInsertion, error) {
		return domain.BacklinkInsertion{Anchor: "hello", ModifiedHTML: "<p><a href=\"" + target + "\">hello</a> world</p>"}, nil
	}}

	res, err := newBacklinkSvc(creds, pl, store, issuer, editor, placer, fakeTargets{url: "https://client.example"}).
		PlaceBacklinks(context.Background(), applb.PlaceBacklinksRequest{Sheet: "WEBSITES", TargetSiteURL: "any"})
	if err != nil {
		t.Fatalf("PlaceBacklinks: %v", err)
	}
	if res.SitesQueued != 1 {
		t.Fatalf("queued = %d, want 1", res.SitesQueued)
	}
	if len(store.saved) != 1 || store.saved[0].AppPassword != "AAAA BBBB CCCC" {
		t.Errorf("issued credential must be saved, got %+v", store.saved)
	}
	if len(pl.written) != 1 || !pl.written[0].OK {
		t.Fatalf("placement not written or not OK: %+v", pl.written)
	}
	status := pl.written[0].Status
	if !strings.Contains(status, "placed: https://donor.example/hello-post/") {
		t.Errorf("status missing public URL: %q", status)
	}
	if !strings.Contains(status, "edit: https://donor.example/wp-admin/post.php?post=42") {
		t.Errorf("status missing edit URL: %q", status)
	}
	if len(editor.posted) != 1 || !strings.Contains(editor.posted[0], "https://client.example") {
		t.Errorf("post content must contain target URL, got %q", editor.posted)
	}
}

func TestPlaceBacklinks_UsesCachedCredentialWithoutIssuing(t *testing.T) {
	creds := &fakeCredSource{creds: []domain.SiteCredential{
		{Row: 2, BaseURL: "https://donor.example", Login: "admin", Password: "raw"},
	}}
	store := &fakeDonorStore{preload: map[string]domain.DonorCredential{
		"https://donor.example": {DonorURL: "https://donor.example", Login: "admin", AppPassword: "XXXX YYYY ZZZZ"},
	}}
	issuer := scriptedIssuer{fn: func(string, string, string) (string, error) {
		t.Fatal("issuer must NOT be called when cached credential is present")
		return "", nil
	}}
	editor := &scriptedEditor{latest: func(c domain.DonorCredential) (domain.DonorPost, error) {
		if c.AppPassword != "XXXX YYYY ZZZZ" {
			t.Errorf("editor used wrong app-password: %q", c.AppPassword)
		}
		return domain.DonorPost{ID: 1, Content: "<p>x</p>", EditURL: "https://donor.example/wp-admin/post.php?post=1&action=edit"}, nil
	}}
	placer := scriptedPlacer{fn: func(_, t string) (domain.BacklinkInsertion, error) {
		return domain.BacklinkInsertion{Anchor: "x", ModifiedHTML: "<a href=\"" + t + "\">x</a>"}, nil
	}}

	_, err := newBacklinkSvc(creds, &fakePlacements{}, store, issuer, editor, placer, fakeTargets{url: "https://client.example"}).
		PlaceBacklinks(context.Background(), applb.PlaceBacklinksRequest{Sheet: "WEBSITES", TargetSiteURL: "any"})
	if err != nil {
		t.Fatalf("PlaceBacklinks: %v", err)
	}
	if len(store.saved) != 0 {
		t.Errorf("cached credential must NOT trigger a Save, got %+v", store.saved)
	}
}

func TestPlaceBacklinks_FailureStagesAreRecorded(t *testing.T) {
	creds := &fakeCredSource{creds: []domain.SiteCredential{
		{Row: 2, BaseURL: "https://issuerfail.example", Login: "u", Password: "p"},
		{Row: 3, BaseURL: "https://llmfail.example", Login: "u", Password: "p"},
		{Row: 4, BaseURL: "https://updatefail.example", Login: "u", Password: "p"},
	}}
	store := &fakeDonorStore{preload: map[string]domain.DonorCredential{
		"https://llmfail.example":    {DonorURL: "https://llmfail.example", AppPassword: "x"},
		"https://updatefail.example": {DonorURL: "https://updatefail.example", AppPassword: "x"},
	}}
	issuer := scriptedIssuer{fn: func(d, _, _ string) (string, error) {
		if d == "https://issuerfail.example" {
			return "", errors.New("CAPTCHA on login")
		}
		return "app", nil
	}}
	editor := &scriptedEditor{
		latest: func(c domain.DonorCredential) (domain.DonorPost, error) {
			return domain.DonorPost{ID: 1, Content: "<p>x</p>", EditURL: c.DonorURL + "/wp-admin/post.php?post=1&action=edit"}, nil
		},
		update: func(c domain.DonorCredential, _ int64, _ string) error {
			if c.DonorURL == "https://updatefail.example" {
				return errors.New("status 403")
			}
			return nil
		},
	}
	var lastDonor string
	editor.latest = func(c domain.DonorCredential) (domain.DonorPost, error) {
		lastDonor = c.DonorURL
		return domain.DonorPost{ID: 1, Content: "<p>x</p>", EditURL: c.DonorURL + "/wp-admin/post.php?post=1&action=edit"}, nil
	}
	placer := scriptedPlacer{fn: func(_, target string) (domain.BacklinkInsertion, error) {
		if lastDonor == "https://llmfail.example" {
			return domain.BacklinkInsertion{}, errors.New("rate limited")
		}
		return domain.BacklinkInsertion{Anchor: "a", ModifiedHTML: "<a href=\"" + target + "\">a</a>"}, nil
	}}

	pl := &fakePlacements{}
	_, err := newBacklinkSvc(creds, pl, store, issuer, editor, placer, fakeTargets{url: "https://client.example"}).
		PlaceBacklinks(context.Background(), applb.PlaceBacklinksRequest{Sheet: "WEBSITES", TargetSiteURL: "any"})
	if err != nil {
		t.Fatalf("PlaceBacklinks: %v", err)
	}

	byRow := map[int]domain.PlacementResult{}
	for _, r := range pl.written {
		byRow[r.Row] = r
	}
	if byRow[2].OK || !strings.HasPrefix(byRow[2].Status, "failed: issue app password") {
		t.Errorf("row 2 should fail at issuer, got %+v", byRow[2])
	}
	if byRow[3].OK || !strings.HasPrefix(byRow[3].Status, "failed: llm") {
		t.Errorf("row 3 should fail at llm, got %+v", byRow[3])
	}
	if byRow[4].OK || !strings.HasPrefix(byRow[4].Status, "failed: update post") {
		t.Errorf("row 4 should fail at update, got %+v", byRow[4])
	}
}

func TestPlaceBacklinks_Validation(t *testing.T) {
	svc := newBacklinkSvc(&fakeCredSource{}, &fakePlacements{}, &fakeDonorStore{}, scriptedIssuer{}, &scriptedEditor{}, scriptedPlacer{}, fakeTargets{url: "https://client.example"})

	if _, err := svc.PlaceBacklinks(context.Background(), applb.PlaceBacklinksRequest{TargetSiteURL: "x"}); err != applb.ErrNoSheet {
		t.Errorf("missing sheet err = %v, want ErrNoSheet", err)
	}
	if _, err := svc.PlaceBacklinks(context.Background(), applb.PlaceBacklinksRequest{Sheet: "WEBSITES"}); err != applb.ErrNoTargetSite {
		t.Errorf("missing target id err = %v, want ErrNoTargetSite", err)
	}
}

func TestPlaceBacklinks_EmptyInventory(t *testing.T) {
	creds := &fakeCredSource{}
	pl := &fakePlacements{}
	issuer := scriptedIssuer{fn: func(string, string, string) (string, error) {
		t.Fatal("issuer must not run for empty inventory")
		return "", nil
	}}
	res, err := newBacklinkSvc(creds, pl, &fakeDonorStore{}, issuer, &scriptedEditor{}, scriptedPlacer{}, fakeTargets{url: "https://client.example"}).
		PlaceBacklinks(context.Background(), applb.PlaceBacklinksRequest{Sheet: "WEBSITES", TargetSiteURL: "x"})
	if err != nil {
		t.Fatalf("PlaceBacklinks: %v", err)
	}
	if res.SitesQueued != 0 || len(pl.written) != 0 {
		t.Errorf("empty inventory: queued=%d written=%d, want 0/0", res.SitesQueued, len(pl.written))
	}
}
