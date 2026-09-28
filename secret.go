package oneenv

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"reflect"
)

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

// secretMarker is implemented by Secret[T] only. It lets the schema builder
// recognize a secret field by type, so Secret values are automatically excluded
// from ${VAR} expansion without needing a ",secret" tag.
type secretMarker interface{ isOneenvSecret() }

func (Secret[T]) isOneenvSecret() {}

// isSecretType reports whether t is a Secret[T].
func isSecretType(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && reflect.PointerTo(t).Implements(secretMarkerType)
}

var secretMarkerType = reflect.TypeFor[secretMarker]()

// NewSecret wraps v in a Secret.
func NewSecret[T any](v T) Secret[T] { return Secret[T]{v: v} }

// Value returns the unmasked underlying value.
func (s Secret[T]) Value() T { return s.v }

// String returns the mask, satisfying fmt.Stringer.
func (s Secret[T]) String() string { return redactedMask }

// GoString returns the mask for %#v formatting.
func (s Secret[T]) GoString() string { return redactedMask }

// Format masks every fmt verb. Without it, verbs other than v/s/x/X/q (for
// example %d on a Secret[int]) bypass String and print the wrapped value.
func (s Secret[T]) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redactedMask)) }

// LogValue masks the value for log/slog. slog resolves a LogValuer before a
// handler reaches for MarshalText, which renders the real value.
func (s Secret[T]) LogValue() slog.Value { return slog.StringValue(redactedMask) }

// MarshalYAML masks the value for YAML encoders (gopkg.in/yaml.v3 and
// compatible), which would otherwise fall back to MarshalText.
func (s Secret[T]) MarshalYAML() (any, error) { return redactedMask, nil }

// MarshalXML masks the value for encoding/xml, which would otherwise fall back
// to MarshalText.
func (s Secret[T]) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(redactedMask, start)
}

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
// Encoders that fall back to TextMarshaler (slog, YAML, XML) are masked by
// LogValue, MarshalYAML and MarshalXML above; any other encoder that calls
// MarshalText will see the plaintext.
func (s Secret[T]) MarshalText() ([]byte, error) {
	t := reflect.TypeFor[T]()
	return []byte(formatterFor(t)(reflect.ValueOf(s.v), ",")), nil
}
