package clientsegments

import "context"

// Repository reads facts (same read-only contract as leadstats) plus the
// one write it owns: the manual segment override.
type Repository interface {
	ListActivity(ctx context.Context) ([]Activity, error)
	// SetSegmentOverride pins clientID's segment to *segment, or clears the
	// override and falls back to the calculated value when segment is nil.
	SetSegmentOverride(ctx context.Context, clientID string, segment *string) error
}
