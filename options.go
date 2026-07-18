package oneenv

import (
	"context"
	"reflect"
)

// Option configures a Load, Parse or Read call. Options follow the functional
// options pattern: each Option is a function that mutates an internal config.
// The zero config is valid and sensible, options are applied in order, and a
// later option overrides an earlier one (last-wins).
type Option func(*config)

// config is the resolved, private configuration for a call. Callers never see
// it; they compose Options instead.
type config struct {
	files        []string
	autoEnvFiles bool
	envVarNames  []string
	prefix       string
	override     bool
	expand       bool
	requiredAll  bool
	tagKey       string
	lookuper     Lookuper
	typeParsers  map[reflect.Type]setter
	mutators     []Mutator
	validator    func(any) error
	ctx          context.Context
}

// Mutator transforms a raw value after lookup and before it is decoded into a
// field. Mutators are applied in registration order; each receives the output
// of the previous one. A non-nil error aborts that field's decoding.
type Mutator func(ctx context.Context, key, value string) (string, error)

// defaultConfig returns a config that works with no options supplied.
func defaultConfig() config {
	return config{
		files:    []string{".env"},
		tagKey:   "env",
		lookuper: OSLookuper{},
	}
}

// newConfig builds a config from the default plus the given options.
func newConfig(opts []Option) config {
	c := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	return c
}

// WithFiles sets the .env files to read, replacing the default ".env".
// Files are read in order; later files override earlier keys.
func WithFiles(names ...string) Option {
	return func(c *config) { c.files = names }
}

// WithEnvFiles enables the dotenv-style, environment-aware file cascade. On
// top of each configured base file (default ".env"), oneenv also reads, in
// increasing priority: "<base>.local", "<base>.<env>" and "<base>.<env>.local",
// where <env> is the active environment name (see WithEnvVar). Every file in
// the cascade is optional. This mirrors the layered .env convention used by
// Rails, Next.js and dotenv-cli.
func WithEnvFiles() Option {
	return func(c *config) { c.autoEnvFiles = true }
}

// WithEnvVar sets which environment variables name the active environment for
// WithEnvFiles, consulted in order (first non-empty wins). Defaults to
// APP_ENV then GO_ENV.
func WithEnvVar(names ...string) Option {
	return func(c *config) {
		if len(names) > 0 {
			c.envVarNames = names
		}
	}
}

// WithPrefix restricts environment lookups to keys carrying the given prefix.
// For example WithPrefix("APP_") maps a field tagged env:"PORT" to APP_PORT.
func WithPrefix(prefix string) Option {
	return func(c *config) { c.prefix = prefix }
}

// WithOverride lets values from .env files overwrite variables that already
// exist in the process environment. By default existing variables win.
func WithOverride() Option {
	return func(c *config) { c.override = true }
}

// WithExpand enables ${VAR} and $VAR expansion inside values.
func WithExpand() Option {
	return func(c *config) { c.expand = true }
}

// WithRequired treats every field as required, as if each carried the
// ",required" tag option.
func WithRequired() Option {
	return func(c *config) { c.requiredAll = true }
}

// WithTagKey overrides the struct tag key used for field names (default "env").
func WithTagKey(key string) Option {
	return func(c *config) {
		if key != "" {
			c.tagKey = key
		}
	}
}

// WithLookuper replaces the environment source used during decoding. Defaults
// to OSLookuper. Pass a MapLookuper for hermetic tests.
func WithLookuper(l Lookuper) Option {
	return func(c *config) {
		if l != nil {
			c.lookuper = l
		}
	}
}

// WithTypeParser registers a custom parser for a specific type T. Whenever a
// field of type T is decoded, fn is used instead of the built-in setter. This
// works for named types, structs, or any type not otherwise supported.
//
//	oneenv.WithTypeParser(func(s string) (net.IP, error) {
//	    return net.ParseIP(s), nil
//	})
//
// Registering any type parser disables the shared schema cache for that call,
// so prefer registering parsers once and reusing the option set.
func WithTypeParser[T any](fn func(string) (T, error)) Option {
	t := reflect.TypeOf((*T)(nil)).Elem()
	set := func(dst reflect.Value, raw, _ string) error {
		v, err := fn(raw)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(v))
		return nil
	}
	return func(c *config) {
		if c.typeParsers == nil {
			c.typeParsers = make(map[reflect.Type]setter)
		}
		c.typeParsers[t] = set
	}
}

// WithMutator registers a Mutator applied to every field's raw value before it
// is decoded. Multiple mutators run in registration order.
func WithMutator(m Mutator) Option {
	return func(c *config) {
		if m != nil {
			c.mutators = append(c.mutators, m)
		}
	}
}

// WithContext sets the context passed to mutators. Defaults to
// context.Background.
func WithContext(ctx context.Context) Option {
	return func(c *config) { c.ctx = ctx }
}

// WithValidator registers a function called with the fully decoded target once
// decoding succeeds. Returning an error fails the Load. This keeps oneenv
// dependency-free while letting callers plug in any validation library.
func WithValidator(fn func(v any) error) Option {
	return func(c *config) { c.validator = fn }
}
