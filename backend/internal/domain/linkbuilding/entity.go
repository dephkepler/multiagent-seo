package linkbuilding

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

type PlacementResult struct {
	Row      int
	DonorURL string
	OK       bool
	Status   string
}
