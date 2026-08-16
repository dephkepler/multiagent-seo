package clientsegments_test

import (
	"slices"
	"testing"

	"multiagent-seo/internal/domain/clientsegments"
)

// Cases mirror real clients pulled from the Abalis dev database while
// designing this — not invented numbers, see chat history for the query.
func TestDerive_Segment(t *testing.T) {
	cases := []struct {
		name string
		in   clientsegments.Activity
		want string
	}{
		{"lead only, no consultation", clientsegments.Activity{}, clientsegments.SegmentLead},
		{
			"booked, not completed yet",
			clientsegments.Activity{ScheduledCount: 1, ConsultCount: 1},
			clientsegments.SegmentBooked,
		},
		{
			"consulted, no case yet",
			clientsegments.Activity{CompletedCount: 1, ConsultCount: 1},
			clientsegments.SegmentConsulted,
		},
		{
			"one case",
			clientsegments.Activity{CompletedCount: 1, ConsultCount: 1, CaseCount: 1},
			clientsegments.SegmentClient,
		},
		{
			"two cases",
			clientsegments.Activity{CompletedCount: 1, ConsultCount: 1, CaseCount: 2},
			clientsegments.SegmentRepeat,
		},
		{
			"only cancelled/no_show, no case",
			clientsegments.Activity{LostCount: 2, ConsultCount: 2},
			clientsegments.SegmentLost,
		},
		{
			"a case wins over a lost consultation history",
			clientsegments.Activity{LostCount: 1, ConsultCount: 1, CaseCount: 1},
			clientsegments.SegmentClient,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := clientsegments.Derive(tc.in).Segment
			if got != tc.want {
				t.Errorf("Derive(%+v).Segment = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestDerive_Tags(t *testing.T) {
	t.Run("debtor when fee exceeds paid", func(t *testing.T) {
		got := clientsegments.Derive(clientsegments.Activity{CaseCount: 1, CaseFee: 6000, CasePaid: 0}).Tags
		if !slices.Contains(got, clientsegments.TagDebtor) {
			t.Errorf("Tags = %v, want to contain %q", got, clientsegments.TagDebtor)
		}
	})

	t.Run("not a debtor when paid in full", func(t *testing.T) {
		got := clientsegments.Derive(clientsegments.Activity{CaseCount: 1, CaseFee: 3000, CasePaid: 3000}).Tags
		if slices.Contains(got, clientsegments.TagDebtor) {
			t.Errorf("Tags = %v, want no %q", got, clientsegments.TagDebtor)
		}
	})

	t.Run("no-show risk needs the threshold, not just one", func(t *testing.T) {
		one := clientsegments.Derive(clientsegments.Activity{LostCount: 1, ConsultCount: 1}).Tags
		if slices.Contains(one, clientsegments.TagNoShowRisk) {
			t.Errorf("Tags = %v, want no %q after a single no-show", one, clientsegments.TagNoShowRisk)
		}

		two := clientsegments.Derive(clientsegments.Activity{LostCount: 2, ConsultCount: 2}).Tags
		if !slices.Contains(two, clientsegments.TagNoShowRisk) {
			t.Errorf("Tags = %v, want %q at the threshold", two, clientsegments.TagNoShowRisk)
		}
	})
}

func TestDerive_SegmentOverride(t *testing.T) {
	lost := clientsegments.SegmentLost

	t.Run("override wins over the calculated segment", func(t *testing.T) {
		got := clientsegments.Derive(clientsegments.Activity{
			CaseCount:       1, // would calculate to "client" on its own
			SegmentOverride: &lost,
		})
		if got.Segment != clientsegments.SegmentLost {
			t.Errorf("Segment = %q, want override %q", got.Segment, clientsegments.SegmentLost)
		}
		if !got.Overridden {
			t.Error("Overridden = false, want true")
		}
	})

	t.Run("tags stay tied to the real numbers even when segment is overridden", func(t *testing.T) {
		got := clientsegments.Derive(clientsegments.Activity{
			CaseCount: 1, CaseFee: 6000, CasePaid: 0,
			SegmentOverride: &lost,
		})
		if !slices.Contains(got.Tags, clientsegments.TagDebtor) {
			t.Errorf("Tags = %v, want to still contain %q despite the override", got.Tags, clientsegments.TagDebtor)
		}
	})

	t.Run("no override, nothing changes", func(t *testing.T) {
		got := clientsegments.Derive(clientsegments.Activity{CaseCount: 1})
		if got.Overridden {
			t.Error("Overridden = true, want false when SegmentOverride is nil")
		}
		if got.Segment != clientsegments.SegmentClient {
			t.Errorf("Segment = %q, want calculated %q", got.Segment, clientsegments.SegmentClient)
		}
	})
}

func TestIsSegment(t *testing.T) {
	for _, s := range []string{
		clientsegments.SegmentLead, clientsegments.SegmentBooked, clientsegments.SegmentConsulted,
		clientsegments.SegmentClient, clientsegments.SegmentRepeat, clientsegments.SegmentLost,
	} {
		if !clientsegments.IsSegment(s) {
			t.Errorf("IsSegment(%q) = false, want true", s)
		}
	}
	if clientsegments.IsSegment("bogus") {
		t.Error(`IsSegment("bogus") = true, want false`)
	}
	if clientsegments.IsSegment("") {
		t.Error(`IsSegment("") = true, want false`)
	}
}
