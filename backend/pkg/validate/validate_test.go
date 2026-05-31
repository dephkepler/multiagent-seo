package validate

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestFieldError_Error(t *testing.T) {
	t.Run("with param", func(t *testing.T) {
		fe := FieldError{Field: "age", Tag: "gte", Param: "18"}
		got := fe.Error()
		want := "field 'age' failed 'gte=18'"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("without param", func(t *testing.T) {
		fe := FieldError{Field: "name", Tag: "required"}
		got := fe.Error()
		want := "field 'name' failed 'required'"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

type passingStruct struct {
	Name string `json:"name" validate:"required"`
	Age  int    `json:"age" validate:"gte=0"`
}

type failingStruct struct {
	Name string `json:"name" validate:"required"`
	Age  int    `json:"age" validate:"gte=18"`
}

func TestValidate_PassingStruct(t *testing.T) {
	s := passingStruct{Name: "Ada", Age: 30}
	if err := Validate(s); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidate_FailingStruct(t *testing.T) {
	s := failingStruct{Name: "", Age: 5}
	err := Validate(s)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(ve.Fields) != 2 {
		t.Fatalf("expected 2 field errors, got %d: %v", len(ve.Fields), ve.Fields)
	}

	byField := map[string]FieldError{}
	for _, fe := range ve.Fields {
		byField[fe.Field] = fe
	}
	nameErr, ok := byField["name"]
	if !ok || nameErr.Tag != "required" {
		t.Errorf("expected name=required, got %+v", nameErr)
	}
	ageErr, ok := byField["age"]
	if !ok || ageErr.Tag != "gte" || ageErr.Param != "18" {
		t.Errorf("expected age=gte/18, got %+v", ageErr)
	}
}

type uuidStruct struct {
	ID uuid.UUID `json:"id" validate:"nonzero_uuid"`
}

func TestValidate_NonzeroUUID(t *testing.T) {
	t.Run("zero fails", func(t *testing.T) {
		s := uuidStruct{ID: uuid.Nil}
		err := Validate(s)
		if err == nil {
			t.Fatal("expected error for zero UUID, got nil")
		}
		var ve *ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected *ValidationError, got %T", err)
		}
		if len(ve.Fields) != 1 || ve.Fields[0].Tag != "nonzero_uuid" {
			t.Fatalf("expected nonzero_uuid failure, got %+v", ve.Fields)
		}
	})

	t.Run("non-zero passes", func(t *testing.T) {
		s := uuidStruct{ID: uuid.New()}
		if err := Validate(s); err != nil {
			t.Fatalf("expected nil for non-zero UUID, got %v", err)
		}
	})
}

func TestMissingFields(t *testing.T) {
	t.Run("from ValidationError", func(t *testing.T) {
		s := failingStruct{Name: "", Age: 5}
		err := Validate(s)
		got := MissingFields(err)
		if len(got) != 2 {
			t.Fatalf("expected 2 fields, got %v", got)
		}
		seen := map[string]bool{}
		for _, f := range got {
			seen[f] = true
		}
		if !seen["name"] || !seen["age"] {
			t.Errorf("expected name and age, got %v", got)
		}
	})

	t.Run("from nil", func(t *testing.T) {
		if got := MissingFields(nil); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("from random error", func(t *testing.T) {
		if got := MissingFields(errors.New("boom")); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}
