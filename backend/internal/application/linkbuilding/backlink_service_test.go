package linkbuilding_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

type fakeProfileStore struct{}

func (fakeProfileStore) Save(context.Context, domain.DonorProfile) error { return nil }
func (fakeProfileStore) RecentlyFailed(context.Context, time.Time, time.Time) (map[string]bool, error) {
	return nil, nil
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
func (f *fakePlacementStore) ListPlaced(_ context.Context, limit, offset int) ([]domain.Placement, int, error) {
	var ok []domain.Placement
	for _, p := range f.saved {
		if p.OK {
			ok = append(ok, p)
		}
	}
	return ok, len(ok), nil
}
func (f *fakePlacementStore) PlacedDonors(_ context.Context, targetURL string) (map[string]bool, error) {
	out := make(map[string]bool)
	for _, p := range f.saved {
		if p.OK && p.TargetURL == targetURL {
			out[domain.NormalizeDonorURL(p.DonorURL)] = true
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
	caps       func(domain.DonorCredential) (domain.DonorCapabilities, error)
	front      func(domain.DonorCredential) (domain.DonorPost, bool, error)
	latest     func(domain.DonorCredential) (domain.DonorPost, bool, error)
	update     func(domain.DonorCredential, int64, string) error
	updatePage func(domain.DonorCredential, int64, string) error
	create     func(domain.DonorCredential, string, string) (domain.DonorPost, error)
	posted     []string
}

func (s *scriptedEditor) Capabilities(_ context.Context, c domain.DonorCredential) (domain.DonorCapabilities, error) {
	if s.caps == nil {
		return domain.DonorCapabilities{UserID: 1, CanEditPages: true, CanEditOthers: true, CanPublish: true, CanCreate: true}, nil
	}
	return s.caps(c)
}
func (s *scriptedEditor) FrontPage(_ context.Context, c domain.DonorCredential) (domain.DonorPost, bool, error) {
	if s.front == nil {
		return domain.DonorPost{}, false, nil
	}
	return s.front(c)
}
func (s *scriptedEditor) UpdatePageContent(_ context.Context, c domain.DonorCredential, id int64, html string) error {
	if s.updatePage == nil {
		return nil
	}
	return s.updatePage(c, id, html)
}
func (s *scriptedEditor) LatestEditablePost(_ context.Context, c domain.DonorCredential, _ domain.DonorCapabilities) (domain.DonorPost, bool, error) {
	if s.latest == nil {
		return domain.DonorPost{}, false, nil
	}
	return s.latest(c)
}
func (s *scriptedEditor) UpdatePostContent(_ context.Context, c domain.DonorCredential, id int64, html string) error {
	s.posted = append(s.posted, html)
	if s.update == nil {
		return nil
	}
	return s.update(c, id, html)
}
func (s *scriptedEditor) CreatePost(_ context.Context, c domain.DonorCredential, title, html string) (domain.DonorPost, error) {
	if s.create == nil {
		return domain.DonorPost{}, errors.New("wppost create status 403: cannot_create")
	}
	return s.create(c, title, html)
}
func (s *scriptedEditor) LatestTitles(_ context.Context, _ domain.DonorCredential, _ int) ([]string, error) {
	return nil, nil
}
func (s *scriptedEditor) VerifyLink(_ context.Context, _, _ string) (string, error) {
	return domain.LinkDofollow, nil
}

type scriptedPlacer struct {
	place   func(html, target string) (domain.BacklinkInsertion, error)
	compose func(target string) (domain.ComposedPost, error)
}

func (s scriptedPlacer) Place(_ context.Context, html, target string) (domain.BacklinkInsertion, error) {
	if s.place == nil {
		return domain.BacklinkInsertion{}, errors.New("no place script")
	}
	return s.place(html, target)
}
func (s scriptedPlacer) Compose(_ context.Context, target string, _ []string) (domain.ComposedPost, error) {
	if s.compose == nil {
		return domain.ComposedPost{}, errors.New("no compose script")
	}
	return s.compose(target)
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
	return applb.NewBacklinkService(creds, pl, &fakePlacementStore{}, fakeProfileStore{}, ds, is, ed, placerBuilder, applb.LLMDefaults{}, tg, jobrunner.NewSyncRunner(), nil, applb.WithBacklinkDelay(0, 0))
}

func TestPlaceBacklinks_HappyPathIssuesAndCachesCreds(t *testing.T) {
	creds := &fakeCredSource{creds: []domain.SiteCredential{
		{Row: 2, BaseURL: "https://donor.example", Login: "admin", Password: "raw"},
	}}
	pl := &fakePlacements{}
	store := &fakeDonorStore{preload: map[string]domain.DonorCredential{}}
	issuer := scriptedIssuer{fn: func(d, l, p string) (string, error) { return "AAAA BBBB CCCC", nil }}
	editor := &scriptedEditor{latest: func(domain.DonorCredential) (domain.DonorPost, bool, error) {
		return domain.DonorPost{ID: 42, Content: "<p>hello world</p>", PublicURL: "https://donor.example/hello-post/", EditURL: "https://donor.example/wp-admin/post.php?post=42&action=edit"}, true, nil
	}}
	placer := scriptedPlacer{place: func(html, target string) (domain.BacklinkInsertion, error) {
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
	if !strings.Contains(status, "placed (post): https://donor.example/hello-post/") {
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
	editor := &scriptedEditor{latest: func(c domain.DonorCredential) (domain.DonorPost, bool, error) {
		if c.AppPassword != "XXXX YYYY ZZZZ" {
			t.Errorf("editor used wrong app-password: %q", c.AppPassword)
		}
		return domain.DonorPost{ID: 1, Content: "<p>x</p>", EditURL: "https://donor.example/wp-admin/post.php?post=1&action=edit"}, true, nil
	}}
	placer := scriptedPlacer{place: func(_, t string) (domain.BacklinkInsertion, error) {
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
		{Row: 3, BaseURL: "https://transient.example", Login: "u", Password: "p"},
		{Row: 4, BaseURL: "https://lowpriv.example", Login: "u", Password: "p"},
	}}
	store := &fakeDonorStore{preload: map[string]domain.DonorCredential{
		"https://transient.example": {DonorURL: "https://transient.example", AppPassword: "x"},
		"https://lowpriv.example":   {DonorURL: "https://lowpriv.example", AppPassword: "x"},
	}}
	issuer := scriptedIssuer{fn: func(d, _, _ string) (string, error) {
		if d == "https://issuerfail.example" {
			return "", errors.New("CAPTCHA on login")
		}
		return "app", nil
	}}
	editor := &scriptedEditor{
		latest: func(c domain.DonorCredential) (domain.DonorPost, bool, error) {
			if c.DonorURL == "https://lowpriv.example" {
				return domain.DonorPost{}, false, nil // nothing this account may edit
			}
			return domain.DonorPost{ID: 1, Content: "<p>x</p>"}, true, nil
		},
		update: func(c domain.DonorCredential, _ int64, _ string) error {
			if c.DonorURL == "https://transient.example" {
				return errors.New("wppost update status 500: server error")
			}
			return nil
		},
		create: func(c domain.DonorCredential, _, _ string) (domain.DonorPost, error) {
			if c.DonorURL == "https://transient.example" {
				return domain.DonorPost{}, errors.New("wppost create status 500: server error")
			}
			return domain.DonorPost{}, errors.New("wppost create status 403: cannot_create")
		},
	}
	placer := scriptedPlacer{
		place: func(_, target string) (domain.BacklinkInsertion, error) {
			return domain.BacklinkInsertion{Anchor: "a", ModifiedHTML: "<a href=\"" + target + "\">a</a>"}, nil
		},
		compose: func(target string) (domain.ComposedPost, error) {
			return domain.ComposedPost{Title: "T", Anchor: "a", HTML: "<p><a href=\"" + target + "\">a</a></p>"}, nil
		},
	}

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
	if byRow[2].OK || !strings.HasPrefix(byRow[2].Status, "failed: issue app password") || byRow[2].Outcome != domain.OutcomeCaptcha {
		t.Errorf("row 2 should fail at issuer with captcha, got %+v", byRow[2])
	}
	if byRow[3].OK || byRow[3].Outcome != domain.OutcomePostFailed {
		t.Errorf("row 3 (transient on every tier) should be post_failed, got %+v", byRow[3])
	}
	if byRow[4].OK || byRow[4].Outcome != domain.OutcomeNoTarget {
		t.Errorf("row 4 (no editable target, cannot create) should be no_target, got %+v", byRow[4])
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
