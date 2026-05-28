package uuids_test

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"contentflow/pkg/uuids"
)

func TestDedupe_PreservesFirstOccurrence(t *testing.T) {
	t.Parallel()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	got := uuids.Dedupe([]uuid.UUID{a, b, a, c, b})
	want := []uuid.UUID{a, b, c}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDedupe_KeepsNilUUID(t *testing.T) {
	t.Parallel()
	a := uuid.New()
	got := uuids.Dedupe([]uuid.UUID{a, uuid.Nil, a, uuid.Nil})
	want := []uuid.UUID{a, uuid.Nil}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDedupe_EmptyInput(t *testing.T) {
	t.Parallel()
	if got := uuids.Dedupe(nil); got != nil {
		t.Fatalf("nil input: got %v, want nil", got)
	}
	if got := uuids.Dedupe([]uuid.UUID{}); got != nil {
		t.Fatalf("empty input: got %v, want nil", got)
	}
}

func TestDedupeNonZero_DropsNil(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	got := uuids.DedupeNonZero([]uuid.UUID{a, uuid.Nil, b, uuid.Nil, a})
	want := []uuid.UUID{a, b}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDedupeNonZero_AllNilReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := uuids.DedupeNonZero([]uuid.UUID{uuid.Nil, uuid.Nil}); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestDedupeNonZero_EmptyInput(t *testing.T) {
	t.Parallel()
	if got := uuids.DedupeNonZero(nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
