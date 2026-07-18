package oneenv

import (
	"encoding"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// formatter is the reverse of a setter: it renders a reflect.Value back to the
// string form used in a .env file. It is chosen once when the schema is built.
type formatter func(src reflect.Value, separator string) string

// Marshal renders v as .env-formatted bytes (KEY=value lines, sorted by key).
// It is the inverse of Unmarshal for the fields oneenv understands. Values that
// contain whitespace, quotes or newlines are double-quoted and escaped.
func Marshal(v any, opts ...Option) ([]byte, error) {
	m, err := MarshalMap(v, opts...)
	if err != nil {
		return nil, err
	}
	return renderEnv(m), nil
}

// redactedMask is the placeholder written in place of a secret field's value
// by Redacted and RedactedMap.
const redactedMask = "****"

// Redacted renders v as .env-formatted bytes like Marshal, but replaces the
// value of every field marked ",secret" (or env-secret:"true") with a mask.
// Use it to log or print a configuration without leaking sensitive values.
func Redacted(v any, opts ...Option) ([]byte, error) {
	m, err := marshalTo(v, true, opts...)
	if err != nil {
		return nil, err
	}
	return renderEnv(m), nil
}

// MarshalMap renders v into a flat key/value map, applying any configured
// prefix so the keys match what Load would look up.
func MarshalMap(v any, opts ...Option) (map[string]string, error) {
	return marshalTo(v, false, opts...)
}

// RedactedMap is MarshalMap with secret field values masked.
func RedactedMap(v any, opts ...Option) (map[string]string, error) {
	return marshalTo(v, true, opts...)
}

func marshalTo(v any, redact bool, opts ...Option) (map[string]string, error) {
	cfg := newConfig(opts)
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, ErrNotAStruct
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, ErrNotAStruct
	}
	out := make(map[string]string)
	if err := marshalStruct(rv, cfg.prefix, cfg, redact, out); err != nil {
		return nil, err
	}
	return out, nil
}

// renderEnv formats a flat key/value map as sorted KEY=value lines.
func renderEnv(m map[string]string) []byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(quoteValue(m[k]))
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func marshalStruct(rv reflect.Value, prefix string, cfg config, redact bool, out map[string]string) error {
	schema, err := schemaFor(rv.Type(), cfg)
	if err != nil {
		return err
	}
	for i := range schema.fields {
		fp := &schema.fields[i]
		field := rv.FieldByIndex(fp.index)
		if fp.nested {
			if err := marshalStruct(field, prefix+fp.envPrefix, cfg, redact, out); err != nil {
				return err
			}
			continue
		}
		if fp.nestedSlice {
			base := fp.envPrefix
			if base == "" {
				base = fp.key + "_"
			}
			for j := 0; j < field.Len(); j++ {
				ep := prefix + base + strconv.Itoa(j) + "_"
				if err := marshalStruct(field.Index(j), ep, cfg, redact, out); err != nil {
					return err
				}
			}
			continue
		}
		if redact && fp.secret {
			out[prefix+fp.key] = redactedMask
			continue
		}
		out[prefix+fp.key] = fp.format(field, fp.separator)
	}
	return nil
}

// quoteValue wraps a value in double quotes and escapes it when it contains
// characters that would not round-trip as a bare unquoted value.
func quoteValue(s string) string {
	if s == "" {
		return ""
	}
	if !strings.ContainsAny(s, " \t\r\n\"'#$") {
		return s
	}
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r", "\t", "\\t")
	return `"` + r.Replace(s) + `"`
}

// timeFormatter renders a time.Time with the given layout.
func timeFormatter(layout string) formatter {
	return func(src reflect.Value, _ string) string {
		t, ok := src.Interface().(time.Time)
		if !ok {
			return ""
		}
		return t.Format(layout)
	}
}

// formatterFor returns the formatter for a type, mirroring setterFor.
func formatterFor(t reflect.Type) formatter {
	switch t.Kind() {
	case reflect.Pointer:
		elem := formatterFor(t.Elem())
		return func(src reflect.Value, sep string) string {
			if src.IsNil() {
				return ""
			}
			return elem(src.Elem(), sep)
		}
	case reflect.Slice:
		elem := formatterFor(t.Elem())
		return func(src reflect.Value, sep string) string {
			parts := make([]string, src.Len())
			for i := range parts {
				parts[i] = elem(src.Index(i), sep)
			}
			return strings.Join(parts, sep)
		}
	case reflect.Map:
		elem := formatterFor(t.Elem())
		return func(src reflect.Value, sep string) string {
			pairs := make([]string, 0, src.Len())
			iter := src.MapRange()
			for iter.Next() {
				pairs = append(pairs, fmt.Sprintf("%v:%s", iter.Key().Interface(), elem(iter.Value(), sep)))
			}
			sort.Strings(pairs)
			return strings.Join(pairs, sep)
		}
	default:
		// A TextMarshaler (e.g. Secret[T]) controls its own .env representation,
		// which may differ from its String() form.
		if t.Implements(textMarshalerType) {
			return func(src reflect.Value, _ string) string {
				b, err := src.Interface().(encoding.TextMarshaler).MarshalText()
				if err != nil {
					return ""
				}
				return string(b)
			}
		}
		return func(src reflect.Value, _ string) string {
			if src.Type() == durationType {
				return time.Duration(src.Int()).String()
			}
			return fmt.Sprintf("%v", src.Interface())
		}
	}
}
