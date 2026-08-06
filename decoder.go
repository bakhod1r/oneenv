package oneenv

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// decode populates the struct pointed to by v using the given config, file
// values and lookuper. All field errors are collected and returned joined, so
// a caller sees every problem at once rather than one at a time.
func decode(v any, cfg config, fileVals, rawVals map[string]string) error {
	return decodeFiles(v, cfg, fileVals, rawVals, nil)
}

// decodeFiles is decode that also knows which file each key came from, so a
// report can name it.
func decodeFiles(v any, cfg config, fileVals, rawVals, origin map[string]string) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return ErrNotAStruct
	}

	// Without expansion the raw and expanded values coincide, and the raw map is
	// left empty; dropping it keeps the no-expand path on a single map.
	if !cfg.expand {
		rawVals = nil
	}

	// Environment source has priority over file values unless override is set.
	var src Lookuper = layeredSource{
		env:      cfg.lookuper,
		file:     fileVals,
		raw:      rawVals,
		origin:   origin,
		override: cfg.override,
		prefix:   cfg.prefix,
	}

	// Per-field reporting is opt-in; without it the recorder stays nil and the
	// decoder skips the extra source lookup entirely.
	if cfg.tracing() {
		cfg.rec = &recorder{}
	}

	// Strict-key checking needs the set of keys the struct consumes, which is
	// only known once every field has been walked.
	if cfg.strictKeys {
		cfg.known = make(map[string]bool)
	}

	var errs []error
	decodeStruct(rv.Elem(), "", cfg.prefix, src, cfg, &errs)
	if cfg.strictKeys {
		errs = append(errs, unknownKeyErrors(fileVals, origin, cfg)...)
	}
	if err := errors.Join(errs...); err != nil {
		cfg.finish()
		return err
	}
	if cfg.validator != nil {
		if err := cfg.validator(v); err != nil {
			return err
		}
	}
	return cfg.finish()
}

// unknownKeyErrors reports every key present in the .env files that no field of
// the struct consumes — almost always a typo, and silent without this check.
func unknownKeyErrors(fileVals, origin map[string]string, cfg config) []error {
	keys := make([]string, 0, len(fileVals))
	for k := range fileVals {
		if !cfg.known[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	errs := make([]error, 0, len(keys))
	for _, k := range keys {
		errs = append(errs, &UnknownKeyError{Key: k, File: origin[k]})
	}
	return errs
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

		// Record every key this struct consumes, including aliases, so the
		// strict-key check knows what is legitimate.
		if cfg.known != nil {
			cfg.known[keyPrefix+fp.key] = true
			cfg.known[fp.key] = true
			for _, a := range fp.aliases {
				cfg.known[keyPrefix+a] = true
				cfg.known[a] = true
			}
		}

		// The entry this field contributes to the report and the table.
		entry := Entry{
			Key:      keyPrefix + fp.key,
			Field:    fieldPath,
			Source:   SourceUnset,
			Default:  fp.defval,
			Type:     fp.typeName,
			Required: fp.required || cfg.requiredAll,
			Secret:   fp.secret,
		}

		// Secrets and ",noexpand" fields read the literal text, so a '$' in the
		// value is never mistaken for a variable reference.
		lookup := func(key string) (string, bool) {
			if fp.noExpand {
				return lookupRaw(src, key)
			}
			return src.Lookup(key)
		}
		raw, ok := lookup(fp.key)
		usedKey := fp.key
		// An alias is the previous spelling of a key. It is consulted only when
		// the current one is absent, and using it earns a warning.
		if !ok {
			for _, a := range fp.aliases {
				if v, found := lookup(a); found {
					raw, ok, usedKey = v, true, a
					cfg.logDeprecated(a, "use "+fp.key+" instead")
					break
				}
			}
		}
		if ok && fp.deprecated != "" {
			cfg.logDeprecated(keyPrefix+fp.key, fp.deprecated)
		}
		if ok && cfg.tracing() {
			_, entry.Source, entry.File, _ = lookupSource(src, usedKey)
		}

		if !ok {
			if fp.hasDefant {
				raw, entry.Source = fp.defval, SourceDefault
			} else if fp.initField {
				initValue(field)
				cfg.logField(entry)
				continue
			} else if fp.required || cfg.requiredAll {
				*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: ErrRequired})
				entry.Null = true
				cfg.logField(entry)
				continue
			} else {
				cfg.logField(entry)
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

		// A required field must resolve to a non-empty value. A key that exists
		// but carries an empty string is indistinguishable from "not set" for
		// most applications, so both the per-field `,required` tag and the
		// global WithRequired() option treat it as a missing value.
		if (fp.required || cfg.requiredAll) && raw == "" {
			*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: ErrRequired})
			entry.Null = true
			cfg.logField(entry)
			continue
		}

		// `pattern` and `enum` constrain the text before it is decoded, so the
		// error names the rule that was broken rather than a type failure.
		if err := fp.validate(raw); err != nil {
			*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: err})
			continue
		}

		if err := fp.set(field, raw, fp.separator); err != nil {
			*errs = append(*errs, &FieldError{Field: fieldPath, Key: fp.key, Err: err})
			continue
		}
		entry.Value = raw
		cfg.logField(entry)
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
	raw      map[string]string // literal, pre-expansion file values
	origin   map[string]string // key -> file it came from, nil when untracked
	override bool
	prefix   string
}

func (l layeredSource) Lookup(key string) (string, bool) {
	return l.lookup(key, l.file)
}

// LookupRaw resolves key like Lookup, except that a hit in the file values
// yields the literal text before expansion. Process-environment values are
// never expanded to begin with, so they are returned unchanged.
func (l layeredSource) LookupRaw(key string) (string, bool) {
	if l.raw == nil {
		return l.Lookup(key)
	}
	return l.lookup(key, l.raw)
}

func (l layeredSource) lookup(key string, file map[string]string) (string, bool) {
	envKey := l.prefix + key
	if l.override {
		if v, ok := file[envKey]; ok {
			return v, true
		}
		if v, ok := file[key]; ok {
			return v, true
		}
	}
	if v, ok := l.env.Lookup(envKey); ok {
		return v, true
	}
	if v, ok := file[envKey]; ok {
		return v, true
	}
	if v, ok := file[key]; ok {
		return v, true
	}
	return "", false
}
