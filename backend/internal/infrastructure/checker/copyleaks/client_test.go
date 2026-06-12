package copyleaks

import "testing"

func mkMatch(start, length int) matchOffsets {
	var m matchOffsets
	m.Text.Chars.Starts = []int{start}
	m.Text.Chars.Lengths = []int{length}
	return m
}

func TestFlaggedSegments(t *testing.T) {
	text := "I love coffee. In conclusion, it is important."

	results := []resultItem{
		{Classification: 1, Matches: []matchOffsets{mkMatch(0, 14)}},
		{Classification: classAI, Matches: []matchOffsets{mkMatch(15, 31)}},
	}

	got := flaggedSegments(text, results)
	if len(got) != 1 || got[0] != "In conclusion, it is important." {
		t.Fatalf("got %#v, want one AI segment", got)
	}
}

func TestFlaggedSegmentsIgnoresOutOfRange(t *testing.T) {
	text := "short text here"
	results := []resultItem{
		{Classification: classAI, Matches: []matchOffsets{mkMatch(5, 9999)}},
	}
	if got := flaggedSegments(text, results); len(got) != 0 {
		t.Fatalf("out-of-range offset must be skipped, got %#v", got)
	}
}
