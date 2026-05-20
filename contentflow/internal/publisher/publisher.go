// Package publisher defines a transport-agnostic interface for posting a
// finished article to a CMS.
package publisher

import "context"

// Post is the rendered article handed to a Publisher. Content is opaque
// payload already rendered to the destination's native body format
// (HTML for WordPress).
type Post struct {
	Title   string
	Content string
	// Status is the CMS-native status string; empty lets the implementation choose.
	Status string
}

type Publisher interface {
	CreateDraft(ctx context.Context, p Post) (postID int64, editURL string, err error)
	Publish(ctx context.Context, postID int64) (postURL string, err error)
}
