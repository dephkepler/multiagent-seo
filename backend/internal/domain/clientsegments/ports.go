package clientsegments

import "context"

// Repository reads facts only — same read-only contract as leadstats.
type Repository interface {
	ListActivity(ctx context.Context) ([]Activity, error)
}
