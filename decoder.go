package oneenv

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// decode populates the struct pointed to by v using the given config, file
// values and lookuper. All field errors are collected and returned joined, so
// a caller sees every problem at once rather than one at a time.
func decode(v any, cfg config, fileVals map[string]string) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return ErrNotAStruct
	}

	// Environment source has priority over file values unless override is set.
	var src Lookuper = layeredSource{
		env:      cfg.lookuper,
		file:     fileVals,
		override: cfg.override,
		prefix:   cfg.prefix,
	}

	var errs []error
	decodeStruct(rv.Elem(), "", cfg.prefix, src, cfg, &errs)
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if cfg.validator != nil {
		return cfg.validator(v)
	}
	return nil
}

func decodeStruct(rv reflect.Value, path, keyPrefix string, src Lookuper, cfg config, errs *[]error) {
	schema, err := schemaFor(rv.Type(), cfg)
	if err != nil {
		*errs = append(*errs, err)
		return
	}

	for i := range schema.fields {
		fp := &schema.fields[i]
		field := rv.FieldByIndex(fp.index)

		if fp.nested {
			nsrc := src
			if fp.envPrefix != "" {
				nsrc = PrefixLookuper{Prefix: fp.envPrefix, Next: src}
			}
			decodeStruct(field, joinPath(path, rv.Type().Field(fp.index[0]).Name), keyPrefix+fp.envPrefix, nsrc, cfg, errs)
			continue
		}

		if fp.nestedSlice {
			decodeNestedSlice(field, joinPath(path, fieldName(rv, fp.index)), keyPrefix, fp, src, cfg, errs)
			continue
		}

		fieldPath := joinPath(path, fieldName(rv, fp.index))

		// ",unset": drop the variable from the process env once we're done.
		if fp.unset {
			key := keyPrefix + fp.key
			defer func() { _ = os.Unsetenv(key) }()
		}

		raw, ok := src.Lookup(fp.key)
		if !ok {
			if fp.hasDefant {
				raw = fp.defval
			} else if fp.initField {
				initValue(field)
				continue
			} else if fp.required || cfg.requiredAll {
				*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: ErrRequired})
				continue
			} else {
				continue // leave zero value
			}
		}

		// ",file": treat the value as a path and read the secret from it.
		if fp.fromFile {
			data, ferr := os.ReadFile(raw)
			if ferr != nil {
				*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: errors.Join(ErrSecretFile, ferr)})
				continue
			}
			raw = strings.TrimRight(string(data), "\r\n")
		}

		// Apply mutators in registration order.
		if len(cfg.mutators) > 0 {
			mutErr := false
			for _, m := range cfg.mutators {
				raw, err = m(cfg.context(), fp.key, raw)
				if err != nil {
					*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: err})
					mutErr = true
					break
				}
			}
			if mutErr {
				continue
			}
		}

		if fp.notEmpty && raw == "" {
			*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: ErrEmpty})
			continue
		}

		if err := fp.set(field, raw, fp.separator); err != nil {
			*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: err})
		}
	}
}

// maxSliceElements caps how many indexed elements decodeNestedSlice will probe,
// guarding against a runaway loop if a source ever reports every key present.
const maxSliceElements = 4096

// decodeNestedSlice decodes a []struct from indexed keys. For a field tagged
// env:"SERVERS" whose element has env:"HOST", it reads SERVERS_0_HOST,
// SERVERS_1_HOST, ... stopping at the first index that has no keys present.
func decodeNestedSlice(field reflect.Value, path, keyPrefix string, fp *fieldPlan, src Lookuper, cfg config, errs *[]error) {
	elemType := field.Type().Elem()
	elemSchema, err := schemaFor(elemType, cfg)
	if err != nil {
		*errs = append(*errs, err)
		return
	}
	base := fp.envPrefix
	if base == "" {
		base = fp.key + "_"
	}
	slice := reflect.MakeSlice(field.Type(), 0, 0)
	for i := 0; i < maxSliceElements; i++ {
		prefix := base + strconv.Itoa(i) + "_"
		esrc := PrefixLookuper{Prefix: prefix, Next: src}
		if !anyKeyPresent(elemSchema, esrc, cfg) {
			break
		}
		elem := reflect.New(elemType).Elem()
		decodeStruct(elem, path+"["+strconv.Itoa(i)+"]", keyPrefix+prefix, esrc, cfg, errs)
		slice = reflect.Append(slice, elem)
	}
	field.Set(slice)
}

// anyKeyPresent reports whether any leaf env key described by schema is present
// in src, recursing into nested structs. It is used to decide when an indexed
// slice element exists.
func anyKeyPresent(schema *structSchema, src Lookuper, cfg config) bool {
	for i := range schema.fields {
		fp := &schema.fields[i]
		switch {
		case fp.nested:
			nsrc := src
			if fp.envPrefix != "" {
				nsrc = PrefixLookuper{Prefix: fp.envPrefix, Next: src}
			}
			nested, err := schemaFor(fp.structType, cfg)
			if err == nil && anyKeyPresent(nested, nsrc, cfg) {
				return true
			}
		case fp.nestedSlice:
			base := fp.envPrefix
			if base == "" {
				base = fp.key + "_"
			}
			nested, err := schemaFor(fp.structType, cfg)
			if err == nil {
				if anyKeyPresent(nested, PrefixLookuper{Prefix: base + "0_", Next: src}, cfg) {
					return true
				}
			}
		default:
			if _, ok := src.Lookup(fp.key); ok {
				return true
			}
		}
	}
	return false
}

// initValue gives a nil pointer, slice or map a non-nil zero value, so an
// ",init" field is usable even when no value was supplied.
func initValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
	case reflect.Slice:
		if v.IsNil() {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
		}
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
	}
}

// context returns the configured context, defaulting to context.Background.
func (c config) context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func fieldName(rv reflect.Value, index []int) string {
	return rv.Type().FieldByIndex(index).Name
}

func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

// layeredSource resolves a key against the process environment and the parsed
// file values, honoring priority and an optional prefix.
type layeredSource struct {
	env      Lookuper
	file     map[string]string
	override bool
	prefix   string
}

func (l layeredSource) Lookup(key string) (string, bool) {
	envKey := l.prefix + key
	if l.override {
		if v, ok := l.file[key]; ok {
			return v, true
		}
	}
	if v, ok := l.env.Lookup(envKey); ok {
		return v, true
	}
	if v, ok := l.file[key]; ok {
		return v, true
	}
	return "", false
}
