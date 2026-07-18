// Package oneenv reads .env files and decodes them straight into Go structs,
// with zero external dependencies.
//
// It combines what most projects reach for two libraries to do — a dotenv file
// parser and an environment-to-struct decoder — behind a single, fast, cached
// API.
//
// # Quick start
//
//	type Config struct {
//	    Port    int           `env:"PORT" default:"8080"`
//	    Host    string        `env:"HOST,required"`
//	    Timeout time.Duration `env:"TIMEOUT" default:"5s"`
//	    Tags    []string      `env:"TAGS" separator:","`
//	    DB      DBConfig      `envPrefix:"DB_"`
//	}
//
//	cfg, err := oneenv.Parse[Config](oneenv.WithFiles(".env", ".env.local"))
//
// # Resolution order
//
// Each field is resolved with the priority: process environment, then .env
// file value, then the `default` tag. WithOverride flips file values above the
// environment.
//
// # Options
//
// Behavior is tuned with functional options: WithFiles, WithEnvFiles,
// WithEnvVar, WithPrefix, WithOverride, WithExpand, WithRequired, WithTagKey,
// WithLookuper, WithTypeParser, WithMutator, WithValidator and WithContext. The
// zero configuration is valid, and later options win over earlier ones.
//
// # Environment-aware file cascade
//
// WithEnvFiles layers files by the active environment, like Rails and Next.js:
// on top of each base file it also reads <base>.local, <base>.<env> and
// <base>.<env>.local (all optional, later files winning). The environment name
// comes from APP_ENV then GO_ENV, configurable with WithEnvVar.
//
// # Secrets
//
// The ",secret" tag option (or env-secret:"true") marks a field sensitive:
// Redacted and RedactedMap render its value as a mask while Marshal keeps the
// real value. Alternatively wrap a field in Secret[T], whose String, GoString
// and JSON forms are always masked, so it never leaks through logging.
//
// # Slices of structs
//
// A []Struct field is decoded from indexed keys. A field tagged env:"SERVER"
// reads SERVER_0_HOST, SERVER_1_HOST, ... one element per index, stopping at
// the first index with no keys present.
//
// # Hot reload
//
// The subpackage oneenv/watch re-decodes a struct whenever a watched .env file
// changes, using native OS notifications — inotify (Linux), kqueue (BSD/macOS)
// and ReadDirectoryChangesW (Windows), with modification-time polling as a
// fallback on other platforms — still with zero external dependencies.
//
// # Tags
//
//	env:"NAME"           bind to env key NAME
//	env:"NAME,required"  error if unset from every source
//	env:"NAME,notEmpty"  error if present but empty
//	env:"NAME,file"      read the value from the file at the resolved path
//	env:"NAME,init"      allocate a nil pointer/slice/map even when unset
//	env:"NAME,unset"     drop the variable from the environment after reading
//	env:"NAME,secret"    mask the value in Redacted output
//	env:"-"              skip the field
//	default:"value"      fallback when unset
//	separator:";"        element delimiter for slices and maps (default ",")
//	envSeparator:";"     alias for separator
//	layout:"2006-01-02"  time.Time parse layout (default RFC3339)
//	envPrefix:"DB_"      prefix applied to a nested struct's keys
//	desc:"..."           description surfaced by Usage
//
// Every configuration tag also has an env-* spelling (env-default,
// env-separator, env-description, env-layout, env-prefix, env-required,
// env-notempty, env-file, env-init, env-unset and env-secret). When both spellings are
// present the env-* form takes priority and the native tag is the fallback.
//
// # Marshal and Usage
//
// Marshal renders a struct back into .env bytes, and Usage writes a table of
// the variables a struct consumes for --help output.
//
// # Performance
//
// The parser scans source bytes once without regexp, and struct schemas are
// analyzed once per type and cached, so repeated Load calls avoid reflection
// overhead. All exported functions are safe for concurrent use.
package oneenv
