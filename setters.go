package oneenv

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// setter assigns a parsed string to a reflect.Value of a fixed type. Setters
// are chosen once when the schema is built, so the hot path avoids any
// per-call type switch.
type setter func(dst reflect.Value, raw, separator string) error

var (
	durationType        = reflect.TypeOf(time.Duration(0))
	timeType            = reflect.TypeOf(time.Time{})
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	textMarshalerType   = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// isDecodableStruct reports whether a struct type is decoded directly from a
// string (time.Time or a TextUnmarshaler) rather than recursed into.
func isDecodableStruct(t reflect.Type) bool {
	if t == timeType {
		return true
	}
	return reflect.PointerTo(t).Implements(textUnmarshalerType) ||
		t.Implements(textUnmarshalerType)
}

// setterFor returns the setter for a field type, resolving pointers, custom
// unmarshalers and container types. Custom type parsers, when supplied, take
// precedence over the built-in setters for a matching type.
func setterFor(t reflect.Type, custom map[reflect.Type]setter) (setter, error) {
	if s, ok := custom[t]; ok {
		return s, nil
	}
	if reflect.PointerTo(t).Implements(textUnmarshalerType) {
		return textSetter, nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		elem, err := setterFor(t.Elem(), custom)
		if err != nil {
			return nil, err
		}
		return func(dst reflect.Value, raw, sep string) error {
			ptr := reflect.New(t.Elem())
			if err := elem(ptr.Elem(), raw, sep); err != nil {
				return err
			}
			dst.Set(ptr)
			return nil
		}, nil

	case reflect.String:
		return func(dst reflect.Value, raw, _ string) error {
			dst.SetString(raw)
			return nil
		}, nil

	case reflect.Bool:
		return func(dst reflect.Value, raw, _ string) error {
			b, err := strconv.ParseBool(raw)
			if err != nil {
				return err
			}
			dst.SetBool(b)
			return nil
		}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if t == durationType {
			return func(dst reflect.Value, raw, _ string) error {
				d, err := time.ParseDuration(raw)
				if err != nil {
					return err
				}
				dst.SetInt(int64(d))
				return nil
			}, nil
		}
		return func(dst reflect.Value, raw, _ string) error {
			n, err := strconv.ParseInt(raw, 10, t.Bits())
			if err != nil {
				return err
			}
			dst.SetInt(n)
			return nil
		}, nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(dst reflect.Value, raw, _ string) error {
			n, err := strconv.ParseUint(raw, 10, t.Bits())
			if err != nil {
				return err
			}
			dst.SetUint(n)
			return nil
		}, nil

	case reflect.Float32, reflect.Float64:
		return func(dst reflect.Value, raw, _ string) error {
			f, err := strconv.ParseFloat(raw, t.Bits())
			if err != nil {
				return err
			}
			dst.SetFloat(f)
			return nil
		}, nil

	case reflect.Slice:
		elem, err := setterFor(t.Elem(), custom)
		if err != nil {
			return nil, err
		}
		return func(dst reflect.Value, raw, sep string) error {
			if raw == "" {
				dst.Set(reflect.MakeSlice(t, 0, 0))
				return nil
			}
			parts := strings.Split(raw, sep)
			out := reflect.MakeSlice(t, len(parts), len(parts))
			for i, p := range parts {
				if err := elem(out.Index(i), strings.TrimSpace(p), sep); err != nil {
					return err
				}
			}
			dst.Set(out)
			return nil
		}, nil

	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("%w: map key must be string", ErrUnsupportedType)
		}
		elem, err := setterFor(t.Elem(), custom)
		if err != nil {
			return nil, err
		}
		return func(dst reflect.Value, raw, sep string) error {
			m := reflect.MakeMap(t)
			if raw == "" {
				dst.Set(m)
				return nil
			}
			for _, pair := range strings.Split(raw, sep) {
				k, v, ok := strings.Cut(pair, ":")
				if !ok {
					return fmt.Errorf("invalid map entry %q (want key:value)", pair)
				}
				ev := reflect.New(t.Elem()).Elem()
				if err := elem(ev, strings.TrimSpace(v), sep); err != nil {
					return err
				}
				m.SetMapIndex(reflect.ValueOf(strings.TrimSpace(k)).Convert(t.Key()), ev)
			}
			dst.Set(m)
			return nil
		}, nil
	}

	return nil, fmt.Errorf("%w: %s", ErrUnsupportedType, t)
}

// timeSetter returns a setter that parses time.Time using the given layout.
func timeSetter(layout string) setter {
	return func(dst reflect.Value, raw, _ string) error {
		tm, err := time.Parse(layout, raw)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(tm))
		return nil
	}
}

// textSetter drives encoding.TextUnmarshaler. It is only selected for types
// whose pointer implements the interface, and dst is always an addressable,
// non-pointer value (pointers are unwrapped by the Pointer case), so taking its
// address always yields a valid TextUnmarshaler.
func textSetter(dst reflect.Value, raw, _ string) error {
	return dst.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(raw))
}
