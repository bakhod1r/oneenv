package oneenv

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by the package. Compare against them with errors.Is.
var (
	// ErrNotAStruct is returned when the decode target is not a non-nil pointer
	// to a struct.
	ErrNotAStruct = errors.New("oneenv: target must be a non-nil pointer to struct")

	// ErrRequired is returned (wrapped in a FieldError) when a field marked
	// required has no value from any source.
	ErrRequired = errors.New("oneenv: required variable is not set")

	// ErrUnsupportedType is returned (wrapped in a FieldError) when a struct
	// field has a type that oneenv cannot decode into.
	ErrUnsupportedType = errors.New("oneenv: unsupported field type")

	// ErrEmpty is returned (wrapped in a FieldError) when a field marked
	// ",notEmpty" resolves to an empty value.
	ErrEmpty = errors.New("oneenv: variable is set but empty")

	// ErrSecretFile is returned (wrapped in a FieldError) when a field marked
	// ",file" names a path that cannot be read.
	ErrSecretFile = errors.New("oneenv: cannot read secret file")
)

// ParseError describes a syntax error in a .env source, with position.
type ParseError struct {
	File string // file name, empty for in-memory sources
	Line int    // 1-based line number
	Msg  string // human readable message
}

func (e *ParseError) Error() string {
	loc := e.File
	if loc == "" {
		loc = "<source>"
	}
	return fmt.Sprintf("oneenv: %s:%d: %s", loc, e.Line, e.Msg)
}

// FieldError associates a decoding failure with the struct field and env key
// that produced it. Retrieve it from a Load error with errors.As.
type FieldError struct {
	Field string // Go struct field path, e.g. "DB.Port"
	Key   string // env key, e.g. "DB_PORT"
	Err   error  // underlying cause
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("oneenv: field %s (env %q): %v", e.Field, e.Key, e.Err)
}

func (e *FieldError) Unwrap() error { return e.Err }
