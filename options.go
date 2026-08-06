package oneenv

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"sync"
)

// Option configures a Load, Parse or Read call. Options follow the functional
// options pattern: each Option is a function that mutates an internal config.
// The zero config is valid and sensible, options are applied in order, and a
// later option overrides an earlier one (last-wins).
type Option func(*config)

// config is the resolved, private configuration for a call. Callers never see
// it; they compose Options instead.
type config struct {
	files          []string
	autoEnvFiles   bool
	envVarNames    []string
	prefix         string
	override       bool
	expand         bool
	expandStrict   bool
	requiredAll    bool
	tagKey         string
	lookuper       Lookuper
	typeParsers    map[reflect.Type]setter
	mutators       []Mutator
	validator      func(any) error
	logger         *slog.Logger
	table          bool
	out            io.Writer
	baseDir        string
	strictKeys     bool
	examplePath    string
	writeExampleOn bool
	watch          bool
	once           bool // read this type once per process, see WithOnce
	onReload       func(error)
	mu             sync.Locker // guards the target across a WithWatch reload
	report         *Report
	// secretReveal is the number of leading/trailing characters of a secret left
	// visible in log and table output; secretRevealSet distinguishes an explicit
	// 0 (mask everything) from "unset, mask in full". redacted forces a full mask
	// whatever the reveal says.
	secretReveal    int
	secretRevealSet bool
	redacted        bool
	rec             *recorder       // collects per-field records for the table output
	known           map[string]bool // keys the struct consumes, for WithStrictKeys
	ctx             context.Context
}

// expandOptions returns the parser-facing view of the expansion settings.
func (c config) expandOptions() expandOptions {
	return expandOptions{enabled: c.expand, strict: c.expandStrict}
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
//
// Expansion never applies to fields that opt out: a field marked ",secret"
// (or env-secret:"true"), or any Secret[T] field, or a field marked
// ",noexpand". Those decode from the literal text in the file, so a password
// containing '$' survives intact. Single-quoted values are also never expanded,
// matching POSIX shell.
//
// A reference to a variable that is defined nowhere expands to the empty
// string. Use WithExpandStrict to make that an error instead.
func WithExpand() Option {
	return func(c *config) { c.expand = true }
}

// WithExpandStrict enables expansion like WithExpand, but a reference to a
// variable that resolves neither against an earlier key in the file nor against
// the process environment fails with ErrUnknownVariable instead of silently
// expanding to "". This turns a value such as "$ecret123" from a value that
// quietly vanishes into a parse error naming the line.
func WithExpandStrict() Option {
	return func(c *config) { c.expand, c.expandStrict = true, true }
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

// WithLogger installs a *slog.Logger that records, at debug level, every file
// oneenv reads and every field it resolves: the env key, the struct field path,
// which layer won (env, file, default, unset) and the value.
//
// A field marked ",secret" (or env-secret:"true"), and any Secret[T] field, is
// masked before it reaches the handler: only the first and last few characters
// survive (see WithSecretReveal), so the plaintext is never logged. Without this
// option Load stays completely silent.
//
//	oneenv.Load(&cfg, oneenv.WithLogger())
//	oneenv.Load(&cfg, oneenv.WithLogger(myLogger)) // a specific logger
func WithLogger(l ...*slog.Logger) Option {
	return func(c *config) {
		if len(l) > 0 && l[0] != nil {
			c.logger = l[0]
			return
		}
		c.logger = slog.Default()
	}
}

// WithTable prints an aligned KEY / VALUE / SOURCE table once a Load succeeds,
// one row per resolved field:
//
//	KEY       VALUE       SOURCE
//	API_KEY   ****        .env.local
//	HOST      localhost   default
//	PORT      8080        env
//
// SOURCE names where the value came from: the .env file that supplied it, "env"
// for the process environment, "default" for the `default` tag, or "unset".
//
// Secret fields are masked in full; WithSecretReveal opens a window on them.
// Nothing is printed when the Load fails. Output goes to os.Stdout unless
// WithOutput redirects it.
func WithTable() Option {
	return func(c *config) { c.table = true }
}

// WithOutput redirects everything oneenv prints — the WithTable table and the
// Print helper — to w instead of os.Stdout. It is the only place a writer is
// needed, so ordinary calls never mention one.
func WithOutput(w io.Writer) Option {
	return func(c *config) { c.out = w }
}

// WithSecretReveal opens a window on secret values in the WithTable, WithLogger
// and Print output: the first and last n characters stay visible and the middle
// becomes "****", so "sk-live-abcdef9f2a" with n=4 shows as "sk-l****9f2a".
//
// Secrets are fully masked by default; this option is the only way to reveal
// any part of one, and WithRedacted overrides it back to a full mask. A value
// too short to reveal both ends without overlapping is masked completely, so a
// short secret never leaks in full.
func WithSecretReveal(n int) Option {
	return func(c *config) {
		if n < 0 {
			n = 0
		}
		c.secretReveal, c.secretRevealSet = n, true
	}
}

// WithRedacted forces every secret to be masked in full, overriding any
// WithSecretReveal in the same option set regardless of order. Use it to make
// the "nothing leaks" policy explicit in code that also sets a reveal window
// elsewhere, e.g. behind a production flag.
func WithRedacted() Option {
	return func(c *config) { c.redacted = true }
}

// WithBaseDir resolves every relative .env path against dir, so a program can
// read its configuration from a fixed location without rewriting each file
// name:
//
//	oneenv.Load(&cfg, oneenv.WithBaseDir("/etc/myapp"))  // /etc/myapp/.env
//
// Absolute paths are left untouched. An empty dir disables the option.
func WithBaseDir(dir string) Option {
	return func(c *config) { c.baseDir = dir }
}

// WithStrictKeys turns an unrecognized key in a .env file into an error, so a
// typo is caught at startup instead of silently taking no effect:
//
//	unknown environment variable: PORRT (in .env)
//
// Every unknown key is reported, joined with any other field error. Keys the
// struct does consume, including those reached through a prefix or an alias,
// are never reported.
func WithStrictKeys() Option {
	return func(c *config) { c.strictKeys = true }
}

// WithReport captures the full resolution detail of a Load into r: every key,
// which file or layer supplied it, its default, and whether it is required.
// Use it to answer "where did this value come from?" long after the Load:
//
//	var rep oneenv.Report
//	oneenv.Load(&cfg, oneenv.WithReport(&rep))
//	fmt.Println(rep.Explain("PORT"))
//
// Secret values inside the report are stored already masked, following the same
// WithSecretReveal and WithRedacted policy as the printed table.
func WithReport(r *Report) Option {
	return func(c *config) { c.report = r }
}

// WithWriteExample writes a ready-to-fill example file every time the Load
// succeeds, generated from the struct: each key with its `example` or `default`
// value, preceded by comments carrying the description, the Go type and whether
// it is required. Secret values are never written.
//
// With no argument the file is created next to the .env it documents — ".env"
// yields ".env.example", and WithBaseDir is honored, so /etc/myapp/.env yields
// /etc/myapp/.env.example. Pass a path to put it somewhere else.
//
//	oneenv.Load(&cfg, oneenv.WithWriteExample())
//	oneenv.Load(&cfg, oneenv.WithWriteExample("docs/env.example"))
//
// The file is rewritten only when its contents would change, so a watcher on
// the directory is not woken on every start.
func WithWriteExample(path ...string) Option {
	return func(c *config) {
		c.writeExampleOn = true
		if len(path) > 0 {
			c.examplePath = path[0]
		}
	}
}

// WithValidator registers a function called with the fully decoded target once
// decoding succeeds. Returning an error fails the Load. This keeps oneenv
// dependency-free while letting callers plug in any validation library.
func WithValidator(fn func(v any) error) Option {
	return func(c *config) { c.validator = fn }
}
