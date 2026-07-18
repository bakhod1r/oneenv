package oneenv

import (
	"context"
	"os"
)

// Load reads the configured .env files (default ".env"), merges them with the
// process environment, and decodes the result into the struct pointed to by v.
//
// v must be a non-nil pointer to a struct. Values are resolved per field with
// the priority: process environment > .env file > `default` tag. Every field
// error is collected and returned joined via errors.Join.
//
// Load is safe for concurrent use.
func Load(v any, opts ...Option) error {
	cfg := newConfig(opts)
	files, missingOK := cfg.files, false
	if cfg.autoEnvFiles {
		files = envFileCascade(cfg.files, resolveEnvName(cfg))
		missingOK = true // every file in the cascade is optional
	}
	vals, err := readFiles(files, cfg.expand, missingOK)
	if err != nil {
		return err
	}
	return decode(v, cfg, vals)
}

// LoadContext behaves like Load but threads ctx through to any mutators
// registered with WithMutator.
func LoadContext(ctx context.Context, v any, opts ...Option) error {
	return Load(v, append(opts, WithContext(ctx))...)
}

// ParseContext is the generic convenience form of LoadContext.
func ParseContext[T any](ctx context.Context, opts ...Option) (*T, error) {
	return Parse[T](append(opts, WithContext(ctx))...)
}

// Parse is the generic convenience form of Load: it allocates a T, decodes
// into it, and returns the pointer.
//
//	cfg, err := oneenv.Parse[Config](oneenv.WithPrefix("APP_"))
func Parse[T any](opts ...Option) (*T, error) {
	v := new(T)
	if err := Load(v, opts...); err != nil {
		return nil, err
	}
	return v, nil
}

// Unmarshal decodes .env-formatted bytes directly into v, without touching any
// file or the process environment. Options such as WithExpand and WithPrefix
// still apply.
func Unmarshal(data []byte, v any, opts ...Option) error {
	cfg := newConfig(opts)
	vals := make(map[string]string)
	if err := parse("", data, cfg.expand, vals); err != nil {
		return err
	}
	// Decode against the file values only.
	cfg.lookuper = MapLookuper{}
	return decode(v, cfg, vals)
}

// Read parses one or more .env files and returns the merged key/value map,
// without writing anything to the process environment. Later files override
// earlier keys.
func Read(filenames ...string) (map[string]string, error) {
	if len(filenames) == 0 {
		filenames = []string{".env"}
	}
	return readFiles(filenames, false, false)
}

// LoadEnv parses the given .env files and sets each variable into the process
// environment via os.Setenv. Existing variables are preserved (call with
// WithOverride semantics is not available here; use Overload for that).
func LoadEnv(filenames ...string) error {
	return loadEnv(filenames, false)
}

// Overload behaves like LoadEnv but overwrites variables that already exist in
// the process environment.
func Overload(filenames ...string) error {
	return loadEnv(filenames, true)
}

// setenv is a seam over os.Setenv so tests can exercise the error path.
var setenv = os.Setenv

func loadEnv(filenames []string, override bool) error {
	if len(filenames) == 0 {
		filenames = []string{".env"}
	}
	vals, err := readFiles(filenames, false, false)
	if err != nil {
		return err
	}
	for k, v := range vals {
		if _, exists := os.LookupEnv(k); exists && !override {
			continue
		}
		if err := setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}

// readFiles reads and merges the given files in order. When missingOK is
// false, a missing file is an error; the default ".env" is treated as
// optional so zero-config Load works without a file present.
func readFiles(filenames []string, expand, missingOK bool) (map[string]string, error) {
	out := make(map[string]string)
	for _, name := range filenames {
		data, err := os.ReadFile(name)
		if err != nil {
			if os.IsNotExist(err) && (missingOK || isDefaultFile(filenames, name)) {
				continue
			}
			return nil, err
		}
		if err := parse(name, data, expand, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// isDefaultFile reports whether name is the implicit ".env" default, which is
// allowed to be absent.
func isDefaultFile(filenames []string, name string) bool {
	return len(filenames) == 1 && name == ".env"
}
