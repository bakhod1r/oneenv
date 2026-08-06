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

	// ErrUnknownVariable is returned (wrapped in a ParseError) when strict
	// expansion is enabled via WithExpandStrict and a value references a
	// variable that is defined neither earlier in the file nor in the process
	// environment.
	ErrUnknownVariable = errors.New("oneenv: reference to undefined variable")

	// ErrUnknownKey is returned (wrapped in an UnknownKeyError) when
	// WithStrictKeys is enabled and a .env file defines a key no field consumes.
	ErrUnknownKey = errors.New("oneenv: unknown environment variable")

	// ErrPattern is returned (wrapped in a FieldError) when a value does not
	// match the field's `pattern` tag.
	ErrPattern = errors.New("oneenv: value does not match pattern")

	// ErrNotAllowed is returned (wrapped in a FieldError) when a value is not one
	// of the field's `enum` tag values.
	ErrNotAllowed = errors.New("oneenv: value is not one of the allowed values")

	// ErrBadPattern is returned when a field's `pattern` tag is not a valid
	// regular expression. It is a programming error, reported at schema build.
	ErrBadPattern = errors.New("oneenv: invalid pattern tag")
)

// UnknownKeyError reports a key defined in a .env file that no struct field
// consumes — almost always a typo. Returned only under WithStrictKeys.
type UnknownKeyError struct {
	Key  string // the key as written in the file
	File string // file it was found in, empty when unknown
}

// Error names the unrecognized key and, when known, the file it came from.
func (e *UnknownKeyError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("oneenv: unknown environment variable: %s", e.Key)
	}
	return fmt.Sprintf("oneenv: unknown environment variable: %s (in %s)", e.Key, e.File)
}

// Unwrap returns ErrUnknownKey, so errors.Is matches the sentinel.
func (e *UnknownKeyError) Unwrap() error { return ErrUnknownKey }

// ConstraintError reports a value rejected by a `pattern` or `enum` tag,
// carrying what the field expected so the message can name it.
type ConstraintError struct {
	Rule string // "pattern" or "enum"
	Want string // the pattern source, or the allowed values
	Err  error  // ErrPattern or ErrNotAllowed
}

// Error reports which rule rejected the value and what it expected.
func (e *ConstraintError) Error() string {
	return fmt.Sprintf("%v (%s: %s)", e.Err, e.Rule, e.Want)
}

// Unwrap returns the sentinel, so errors.Is matches ErrPattern or ErrNotAllowed.
func (e *ConstraintError) Unwrap() error { return e.Err }

// ParseError describes a syntax error in a .env source, with position.
type ParseError struct {
	File string // file name, empty for in-memory sources
	Line int    // 1-based line number
	Msg  string // human readable message
	Err  error  // underlying cause, when there is one
}

// Error reports the source location and the reason the line could not be
// parsed.
func (e *ParseError) Error() string {
	loc := e.File
	if loc == "" {
		loc = "<source>"
	}
	return fmt.Sprintf("oneenv: %s:%d: %s", loc, e.Line, e.Msg)
}

// Unwrap returns the underlying cause, or nil when there is none.
func (e *ParseError) Unwrap() error { return e.Err }

// FieldError associates a decoding failure with the struct field and env key
// that produced it. Retrieve it from a Load error with errors.As.
type FieldError struct {
	Field string // Go struct field path, e.g. "DB.Port"
	Key   string // env key, e.g. "DB_PORT"
	Err   error  // underlying cause
}

// Error reports the struct field, the env key, and the reason decoding failed.
func (e *FieldError) Error() string {
	return fmt.Sprintf("oneenv: field %s (env %q): %v", e.Field, e.Key, e.Err)
}

// Unwrap returns the underlying decoding error.
func (e *FieldError) Unwrap() error { return e.Err }
