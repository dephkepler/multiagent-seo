package clientsegments

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	domain "multiagent-seo/internal/domain/clientsegments"
)

// matches the old client-side page size so /clients looks unchanged to staff
const defaultLimit = 25

type Service struct {
	repo domain.Repository
}

func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// Segment/Tag come from Derive, not DB columns — filtering happens here in Go, not SQL.
func (s *Service) List(ctx context.Context, filter domain.ListFilter) (domain.ClientList, error) {
	if filter.Segment != "" && !domain.IsSegment(filter.Segment) {
		return domain.ClientList{}, fmt.Errorf("clientsegments: list: %w: %q", domain.ErrInvalidSegment, filter.Segment)
	}
	// filter.Tag isn't validated — a renamed/deleted tag should just match nothing, not error.
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
		if filter.Tag != "" && !slices.Contains(cs.Tags, filter.Tag) && !slices.Contains(cs.ManualTags, filter.Tag) {
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

// nil segment clears the override and falls back to Derive.
func (s *Service) SetOverride(ctx context.Context, clientID string, segment *string) error {
	if segment != nil && !domain.IsSegment(*segment) {
		return fmt.Errorf("clientsegments: set override: %w: %q", domain.ErrInvalidSegment, *segment)
	}
	if err := s.repo.SetSegmentOverride(ctx, clientID, segment); err != nil {
		return fmt.Errorf("clientsegments: set override: %w", err)
	}
	return nil
}

// the vocabulary is enforced by a DB FK (ErrUnknownTag); trim/empty here is just defense in depth.
func (s *Service) AddTag(ctx context.Context, clientID, tag, createdBy string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("clientsegments: add tag: %w: %q", domain.ErrInvalidManualTag, tag)
	}
	if err := s.repo.AddTag(ctx, clientID, tag, createdBy); err != nil {
		return fmt.Errorf("clientsegments: add tag: %w", err)
	}
	return nil
}

func (s *Service) RemoveTag(ctx context.Context, clientID, tag string) error {
	if err := s.repo.RemoveTag(ctx, clientID, tag); err != nil {
		return fmt.Errorf("clientsegments: remove tag: %w", err)
	}
	return nil
}

func (s *Service) Tags(ctx context.Context, clientID string) ([]string, error) {
	list, err := s.List(ctx, domain.ListFilter{ClientID: clientID, Limit: 1})
	if err != nil {
		return nil, fmt.Errorf("clientsegments: tags: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("clientsegments: tags: %w", domain.ErrNotFound)
	}
	return list.Items[0].ManualTags, nil
}

func (s *Service) ListTagDefs(ctx context.Context) ([]domain.TagDef, error) {
	defs, err := s.repo.ListTagDefs(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientsegments: list tag defs: %w", err)
	}
	return defs, nil
}

func (s *Service) CreateTagDef(ctx context.Context, label, category, createdBy string) error {
	label = strings.TrimSpace(label)
	if label == "" || utf8.RuneCountInString(label) > domain.ManualTagMaxLen {
		return fmt.Errorf("clientsegments: create tag def: %w: %q", domain.ErrInvalidManualTag, label)
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = domain.DefaultTagCategory
	}
	if utf8.RuneCountInString(category) > domain.ManualTagMaxLen {
		return fmt.Errorf("clientsegments: create tag def: %w: %q", domain.ErrInvalidManualTag, category)
	}
	if err := s.repo.CreateTagDef(ctx, label, category, createdBy); err != nil {
		return fmt.Errorf("clientsegments: create tag def: %w", err)
	}
	return nil
}

// UpdateTagDef changes label and/or category — nil leaves that field as-is
// (see Repository.UpdateTagDef); an empty *string after trimming is
// rejected rather than silently treated as "leave it", since that would be
// surprising for a caller that explicitly asked to set it to something.
func (s *Service) UpdateTagDef(ctx context.Context, label string, newLabel, newCategory *string) error {
	if newLabel != nil {
		trimmed := strings.TrimSpace(*newLabel)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > domain.ManualTagMaxLen {
			return fmt.Errorf("clientsegments: update tag def: %w: %q", domain.ErrInvalidManualTag, trimmed)
		}
		newLabel = &trimmed
	}
	if newCategory != nil {
		trimmed := strings.TrimSpace(*newCategory)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > domain.ManualTagMaxLen {
			return fmt.Errorf("clientsegments: update tag def: %w: %q", domain.ErrInvalidManualTag, trimmed)
		}
		newCategory = &trimmed
	}
	if err := s.repo.UpdateTagDef(ctx, label, newLabel, newCategory); err != nil {
		return fmt.Errorf("clientsegments: update tag def: %w", err)
	}
	return nil
}

func (s *Service) DeleteTagDef(ctx context.Context, label string) error {
	if err := s.repo.DeleteTagDef(ctx, label); err != nil {
		return fmt.Errorf("clientsegments: delete tag def: %w", err)
	}
	return nil
}
