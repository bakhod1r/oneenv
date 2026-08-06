package oneenv

import (
	"errors"
	"reflect"
	"regexp"
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
	notEmpty    bool     // value must be non-empty when present
	fromFile    bool     // value names a file whose contents are the real value
	initField   bool     // initialize nil pointer/slice/map even when unset
	unset       bool     // remove the variable from the process env after reading
	secret      bool     // value is sensitive: masked by Redacted output
	aliases     []string // former spellings of the key, consulted when it is absent
	deprecated  string   // hint printed when this key is used at all
	example     string   // sample value written to .env.example
	pattern     *regexp.Regexp
	patternSrc  string       // the pattern as written, for error messages
	enum        []string     // the only accepted values, when non-empty
	typeName    string       // Go type of the field, for reports and Usage
	noExpand    bool         // decode the literal value, never the expanded one
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
		plan.secret = boolTag(f, "env-secret", opts.secret) || isSecretType(ft)
		// Secrets never take part in expansion: a '$' in a password is data, not
		// a variable reference. ",noexpand" opts a non-secret field out too.
		plan.noExpand = boolTag(f, "env-noexpand", opts.noExpand) || plan.secret
		// The env-* form always takes priority; the native tag is the fallback,
		// and for the separator so is envSeparator.
		plan.desc = firstNonEmpty(f.Tag.Get("env-description"), f.Tag.Get("desc"))
		plan.typeName = ft.String()
		plan.example = firstNonEmpty(f.Tag.Get("env-example"), f.Tag.Get("example"))
		plan.deprecated = firstNonEmpty(f.Tag.Get("env-deprecated"), f.Tag.Get("deprecated"))
		if alias := firstNonEmpty(f.Tag.Get("env-alias"), f.Tag.Get("alias")); alias != "" {
			plan.aliases = splitTrim(alias, ",")
		}
		if enum := firstNonEmpty(f.Tag.Get("env-enum"), f.Tag.Get("enum")); enum != "" {
			plan.enum = splitTrim(enum, ",")
		}
		if pat := firstNonEmpty(f.Tag.Get("env-pattern"), f.Tag.Get("pattern")); pat != "" {
			re, err := regexp.Compile(pat)
			if err != nil {
				return nil, &FieldError{Field: f.Name, Key: name, Err: errors.Join(ErrBadPattern, err)}
			}
			plan.pattern, plan.patternSrc = re, pat
		}
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

// splitTrim splits s on sep and trims each part, dropping empty ones. It reads
// the comma-separated tag lists (`alias`, `enum`).
func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validate applies the `pattern` and `enum` tags to a raw value before it is
// decoded, so the error names the rule rather than the type.
func (fp *fieldPlan) validate(raw string) error {
	if fp.pattern != nil && !fp.pattern.MatchString(raw) {
		return &ConstraintError{Rule: "pattern", Want: fp.patternSrc, Err: ErrPattern}
	}
	if len(fp.enum) > 0 {
		for _, allowed := range fp.enum {
			if raw == allowed {
				return nil
			}
		}
		return &ConstraintError{Rule: "enum", Want: strings.Join(fp.enum, ", "), Err: ErrNotAllowed}
	}
	return nil
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
	noExpand  bool
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
		case "noexpand", "noExpand":
			opts.noExpand = true
		}
	}
	return name, opts
}
