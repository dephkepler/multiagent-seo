package validate

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var v = validator.New(validator.WithRequiredStructEnabled())

func init() {
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name, _, _ := strings.Cut(fld.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})

	_ = v.RegisterValidation("nonzero_uuid", func(fl validator.FieldLevel) bool {
		field := fl.Field()
		if field.Kind() != reflect.Array || field.Len() != 16 {
			return false
		}
		for i := range field.Len() {
			if field.Index(i).Uint() != 0 {
				return true
			}
		}
		return false
	})
}

type FieldError struct {
	Field string
	Tag   string
	Param string
}

func (fe FieldError) Error() string {
	if fe.Param != "" {
		return fmt.Sprintf("field '%s' failed '%s=%s'", fe.Field, fe.Tag, fe.Param)
	}
	return fmt.Sprintf("field '%s' failed '%s'", fe.Field, fe.Tag)
}

type ValidationError struct {
	Fields []FieldError
}

func (ve *ValidationError) Error() string {
	msgs := make([]string, len(ve.Fields))
	for i, fe := range ve.Fields {
		msgs[i] = fe.Error()
	}
	return strings.Join(msgs, "; ")
}

func Validate(data any) error {
	errs := v.Struct(data)
	if errs == nil {
		return nil
	}
	var validationErrs validator.ValidationErrors
	if !errors.As(errs, &validationErrs) {
		return errs
	}
	var fieldErrors []FieldError
	for _, err := range validationErrs {
		fieldErrors = append(fieldErrors, FieldError{
			Field: err.Field(),
			Tag:   err.Tag(),
			Param: err.Param(),
		})
	}
	return &ValidationError{Fields: fieldErrors}
}

func MissingFields(err error) []string {
	if err == nil {
		return nil
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		return nil
	}
	fields := make([]string, len(validationErr.Fields))
	for i, fe := range validationErr.Fields {
		fields[i] = fe.Field
	}
	return fields
}
