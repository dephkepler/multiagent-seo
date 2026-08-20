package clientsegments

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	domain "multiagent-seo/internal/domain/clientsegments"
)

// defaultLimit matches the page size the /clients UI used client-side
// before pagination moved to the backend — keeping it means the page looks
// unchanged to staff even though the list is now server-paginated.
const defaultLimit = 25

type Service struct {
	repo domain.Repository
}

func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// List returns a filtered, sorted, paginated page of clients plus all-time
// segment counts — no date range on the underlying data, unlike leadstats:
// a client's funnel position is a snapshot of everything that's ever
// happened to them, not something that makes sense sliced by period.
//
// Segment/Tag live only on the derived ClientSegment, not as DB columns
// (see Derive), so filtering happens here in Go, after deriving every
// client, rather than as SQL WHERE clauses the repository could push down.
// Duplicating Derive's classification rules in SQL would be the same logic
// maintained in two places and drifting apart — at today's data volume
// (~700 clients, one query, no N+1) doing it here is simpler and still
// cheap; if the table grows enough for this to matter, that's the moment
// to reconsider, not before.
func (s *Service) List(ctx context.Context, filter domain.ListFilter) (domain.ClientList, error) {
	if filter.Segment != "" && !domain.IsSegment(filter.Segment) {
		return domain.ClientList{}, fmt.Errorf("clientsegments: list: %w: %q", domain.ErrInvalidSegment, filter.Segment)
	}
	if filter.Tag != "" && !domain.IsTag(filter.Tag) {
		return domain.ClientList{}, fmt.Errorf("clientsegments: list: %w: %q", domain.ErrInvalidTag, filter.Tag)
	}
	if filter.Sort != "" && filter.Sort != domain.SortActivity && filter.Sort != domain.SortLTV {
		return domain.ClientList{}, fmt.Errorf("clientsegments: list: %w: %q", domain.ErrInvalidSort, filter.Sort)
	}

	activity, err := s.repo.ListActivity(ctx)
	if err != nil {
		return domain.ClientList{}, fmt.Errorf("clientsegments: list activity: %w", err)
	}

	now := time.Now()
	all := make([]domain.ClientSegment, len(activity))
	for i, a := range activity {
		all[i] = domain.Derive(a, now)
	}

	counts := make(map[string]int, 6)
	for _, cs := range all {
		counts[cs.Segment]++
	}

	search := strings.ToLower(strings.TrimSpace(filter.Search))
	matched := make([]domain.ClientSegment, 0, len(all))
	for _, cs := range all {
		if filter.ClientID != "" && cs.ClientID != filter.ClientID {
			continue
		}
		if filter.Segment != "" && cs.Segment != filter.Segment {
			continue
		}
		if filter.Tag != "" && !slices.Contains(cs.Tags, filter.Tag) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(cs.Name), search) && !strings.Contains(cs.Phone, search) {
			continue
		}
		matched = append(matched, cs)
	}

	if filter.Sort == domain.SortLTV {
		sort.SliceStable(matched, func(i, j int) bool { return matched[i].LTV > matched[j].LTV })
	} else {
		sort.SliceStable(matched, func(i, j int) bool { return matched[i].LastActivity.After(matched[j].LastActivity) })
	}

	total := len(matched)
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	offset := min(max(filter.Offset, 0), total)
	end := min(offset+limit, total)

	return domain.ClientList{
		Items:         matched[offset:end],
		Total:         total,
		SegmentCounts: counts,
	}, nil
}

// SetOverride pins clientID's segment by hand, or clears the override (and
// falls back to Derive) when segment is nil. Validated here, not just left
// to the DB check constraint, so a bad value fails with a clear domain
// error instead of a raw Postgres constraint message reaching the client.
func (s *Service) SetOverride(ctx context.Context, clientID string, segment *string) error {
	if segment != nil && !domain.IsSegment(*segment) {
		return fmt.Errorf("clientsegments: set override: %w: %q", domain.ErrInvalidSegment, *segment)
	}
	if err := s.repo.SetSegmentOverride(ctx, clientID, segment); err != nil {
		return fmt.Errorf("clientsegments: set override: %w", err)
	}
	return nil
}
