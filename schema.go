package oneenv

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// fieldPlan is the precomputed decoding instruction for one struct field.
type fieldPlan struct {
	index       []int  // reflect field index path
	key         string // env key (already tag-resolved)
	envPrefix   string // prefix for a nested struct
	defval      string // default tag value
	hasDefant   bool   // whether a default tag was present
	separator   string // element separator for slices/maps
	desc        string // human description for Usage output
	required    bool
	notEmpty    bool         // value must be non-empty when present
	fromFile    bool         // value names a file whose contents are the real value
	initField   bool         // initialize nil pointer/slice/map even when unset
	unset       bool         // remove the variable from the process env after reading
	secret      bool         // value is sensitive: masked by Redacted output
	nested      bool         // field is a struct to recurse into
	nestedSlice bool         // field is a []struct decoded from indexed keys (KEY_0_*, KEY_1_*)
	structType  reflect.Type // struct type to recurse into (nested) or element type (nestedSlice)
	set         setter
	format      formatter // reverse of set, for Marshal
}

// structSchema is the full decoding plan for a struct type. It is built once
// per type and cached, so repeated Load calls skip all reflection analysis.
type structSchema struct {
	fields []fieldPlan
}

var schemaCache sync.Map // reflect.Type -> *structSchema

// schemaFor returns the cached schema for t, building it on first use. When
// the config carries custom type parsers the cache is bypassed, because the
// same type may decode differently between calls.
func schemaFor(t reflect.Type, cfg config) (*structSchema, error) {
	if len(cfg.typeParsers) == 0 {
		if cached, ok := schemaCache.Load(t); ok {
			return cached.(*structSchema), nil
		}
	}
	s, err := buildSchema(t, cfg)
	if err != nil {
		return nil, err
	}
	if len(cfg.typeParsers) == 0 {
		actual, _ := schemaCache.LoadOrStore(t, s)
		return actual.(*structSchema), nil
	}
	return s, nil
}

func buildSchema(t reflect.Type, cfg config) (*structSchema, error) {
	tagKey := cfg.tagKey
	s := &structSchema{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}

		ft := f.Type
		isNested := ft.Kind() == reflect.Struct && !isDecodableStruct(ft)

		plan := fieldPlan{index: f.Index}

		if isNested {
			plan.nested = true
			plan.structType = ft
			// env-prefix takes priority; envPrefix is the fallback.
			plan.envPrefix = firstNonEmpty(f.Tag.Get("env-prefix"), f.Tag.Get("envPrefix"))
			s.fields = append(s.fields, plan)
			continue
		}

		name, opts := parseTag(f.Tag.Get(tagKey))
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		plan.key = name

		// A slice of (non-decodable) structs is decoded from indexed keys, e.g.
		// SERVERS_0_HOST, SERVERS_1_HOST. It is handled specially by the decoder,
		// not by a setter/formatter.
		if ft.Kind() == reflect.Slice {
			elem := ft.Elem()
			if elem.Kind() == reflect.Struct && !isDecodableStruct(elem) {
				plan.nestedSlice = true
				plan.structType = elem
				plan.envPrefix = firstNonEmpty(f.Tag.Get("env-prefix"), f.Tag.Get("envPrefix"))
				s.fields = append(s.fields, plan)
				continue
			}
		}

		// Every inline env option also has a standalone env-* boolean tag; when
		// present it takes priority over the ",option" form.
		plan.required = boolTag(f, "env-required", opts.required)
		plan.notEmpty = boolTag(f, "env-notempty", opts.notEmpty)
		plan.fromFile = boolTag(f, "env-file", opts.fromFile)
		plan.initField = boolTag(f, "env-init", opts.initField)
		plan.unset = boolTag(f, "env-unset", opts.unset)
		plan.secret = boolTag(f, "env-secret", opts.secret)
		// The env-* form always takes priority; the native tag is the fallback,
		// and for the separator so is envSeparator.
		plan.desc = firstNonEmpty(f.Tag.Get("env-description"), f.Tag.Get("desc"))
		plan.separator = firstNonEmpty(
			f.Tag.Get("env-separator"),
			f.Tag.Get("separator"),
			f.Tag.Get("envSeparator"),
			",",
		)
		if dv, ok := f.Tag.Lookup("env-default"); ok {
			plan.defval = dv
			plan.hasDefant = true
		} else if dv, ok := f.Tag.Lookup("default"); ok {
			plan.defval = dv
			plan.hasDefant = true
		}

		if ft == timeType {
			// env-layout takes priority; layout is the fallback.
			layout := firstNonEmpty(f.Tag.Get("env-layout"), f.Tag.Get("layout"))
			if layout == "" {
				layout = time.RFC3339
			}
			plan.set = timeSetter(layout)
			plan.format = timeFormatter(layout)
			s.fields = append(s.fields, plan)
			continue
		}

		if custom, ok := cfg.typeParsers[ft]; ok {
			plan.set = custom
			plan.format = formatterFor(ft)
			s.fields = append(s.fields, plan)
			continue
		}

		set, err := setterFor(ft, cfg.typeParsers)
		if err != nil {
			return nil, err
		}
		plan.set = set
		plan.format = formatterFor(ft)
		s.fields = append(s.fields, plan)
	}
	return s, nil
}

// boolTag returns the parsed boolean value of the env-* tag named key when it
// is present (env-* form takes priority), otherwise the given fallback.
func boolTag(f reflect.StructField, key string, fallback bool) bool {
	if v, ok := f.Tag.Lookup(key); ok {
		b, _ := strconv.ParseBool(v)
		return b
	}
	return fallback
}

// firstNonEmpty returns the first non-empty string among its arguments, or ""
// if all are empty. Used to resolve a tag from several accepted spellings in
// priority order.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

type tagOpts struct {
	required  bool
	notEmpty  bool
	fromFile  bool
	initField bool
	unset     bool
	secret    bool
}

// parseTag splits a struct tag value like "NAME,required,notEmpty" into name
// and opts.
func parseTag(tag string) (name string, opts tagOpts) {
	if tag == "" {
		return "", opts
	}
	parts := strings.Split(tag, ",")
	name = strings.TrimSpace(parts[0])
	for _, o := range parts[1:] {
		switch strings.TrimSpace(o) {
		case "required":
			opts.required = true
		case "notEmpty":
			opts.notEmpty = true
		case "file":
			opts.fromFile = true
		case "init":
			opts.initField = true
		case "unset":
			opts.unset = true
		case "secret":
			opts.secret = true
		}
	}
	return name, opts
}
