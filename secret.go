package oneenv

import "reflect"

// Secret wraps a sensitive configuration value of type T. It decodes exactly
// like a bare T (reusing oneenv's setters), but its String, GoString and
// MarshalJSON representations are masked, so a Secret never leaks through
// fmt, log, %v/%+v/%#v or encoding/json. Retrieve the real value with Value.
//
//	type Config struct {
//	    APIKey oneenv.Secret[string] `env:"API_KEY"`
//	}
//	fmt.Println(cfg.APIKey)        // ****
//	client.Use(cfg.APIKey.Value()) // the real key
type Secret[T any] struct{ v T }

// NewSecret wraps v in a Secret.
func NewSecret[T any](v T) Secret[T] { return Secret[T]{v: v} }

// Value returns the unmasked underlying value.
func (s Secret[T]) Value() T { return s.v }

// String returns the mask, satisfying fmt.Stringer.
func (s Secret[T]) String() string { return redactedMask }

// GoString returns the mask for %#v formatting.
func (s Secret[T]) GoString() string { return redactedMask }

// MarshalJSON masks the value so it never leaks through encoding/json.
func (s Secret[T]) MarshalJSON() ([]byte, error) { return []byte(`"` + redactedMask + `"`), nil }

// UnmarshalText decodes raw into the underlying T using oneenv's built-in
// setter machinery, so Secret[T] supports every type oneenv can decode.
func (s *Secret[T]) UnmarshalText(text []byte) error {
	t := reflect.TypeFor[T]()
	set, err := setterFor(t, nil)
	if err != nil {
		return err
	}
	return set(reflect.ValueOf(&s.v).Elem(), string(text), ",")
}

// MarshalText renders the real underlying value, so Marshal round-trips a
// Secret back to its plaintext form in a .env file. Use Redacted to mask it.
func (s Secret[T]) MarshalText() ([]byte, error) {
	t := reflect.TypeFor[T]()
	return []byte(formatterFor(t)(reflect.ValueOf(s.v), ",")), nil
}
