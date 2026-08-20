package articles

import (
	"context"
	"sync"
	"testing"
)

// Generate() runs each article's pipeline in its own goroutine
// (application/articles/service.go), and RenderHTML shares the package-level
// md across all of them — a regression here (e.g. a future extension that
// keeps mutable state) would only show up under -race, never in a normal
// single-goroutine test run.
func TestRenderHTML_ConcurrentCalls(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			content := "# Heading\n\nSome **bold** text with a [link](https://example.com) and a table.\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
			if _, _, err := RenderHTML(context.Background(), content, RenderOptions{}); err != nil {
				t.Errorf("render %d: %v", n, err)
			}
		}(i)
	}
	wg.Wait()
}
