package linkbuilding

import "time"

type SiteCredential struct {
	Row             int
	BaseURL         string
	Login           string
	Password        string
	LoginStatus     string
	PlacementStatus string
}

type LoginResult struct {
	Row     int
	BaseURL string
	OK      bool
	Status  string
}

type DonorCredential struct {
	DonorURL    string
	Login       string
	AppPassword string
}

type DonorPost struct {
	ID        int64
	Title     string
	Content   string
	PublicURL string
	EditURL   string
}

type BacklinkInsertion struct {
	Anchor       string
	ModifiedHTML string
}

type ComposedPost struct {
	Title  string
	HTML   string
	Anchor string
}

type DonorCapabilities struct {
	UserID        int64
	Roles         []string
	CanEditPages  bool
	CanEditOthers bool
	CanPublish    bool
	CanCreate     bool
}

type DonorProfile struct {
	DonorURL      string
	Role          string
	CanEditPages  bool
	CanEditOthers bool
	CanPublish    bool
	CanCreate     bool
	LastOutcome   string
}

type PlacementResult struct {
	Row       int
	DonorURL  string
	OK        bool
	Outcome   string
	Status    string
	PostURL   string
	EditURL   string
	Anchor    string
	LinkCheck string
}

type Placement struct {
	ID        int64
	RunID     string
	Sheet     string
	DonorURL  string
	TargetURL string
	OK        bool
	Outcome   string
	Status    string
	PostURL   string
	EditURL   string
	Anchor    string
	LinkCheck string
	CreatedAt time.Time
}
