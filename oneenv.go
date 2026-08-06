package oneenv

import (
	"context"
	"os"
	"path/filepath"
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
	files, missingOK := cfg.resolveFiles()
	cfg.logFiles(files)

	// The per-key origin map is only built when something will report it.
	var origin map[string]string
	if cfg.tracing() {
		origin = make(map[string]string)
	}
	vals, raw, err := readFilesOrigin(files, cfg.expandOptions(), missingOK, origin)
	if err != nil {
		return err
	}
	if err := decodeFiles(v, cfg, vals, raw, origin); err != nil {
		return err
	}
	return cfg.writeExample(v, opts)
}

// resolveFiles returns the .env files this call reads, expanded through the
// environment-aware cascade and resolved against any WithBaseDir, plus whether
// a missing file is acceptable.
func (c config) resolveFiles() (files []string, missingOK bool) {
	files = c.files
	if c.autoEnvFiles {
		files = envFileCascade(c.files, resolveEnvName(c))
		missingOK = true // every file in the cascade is optional
	}
	return c.resolvePaths(files), missingOK
}

// resolvePaths joins each relative name onto the configured base directory.
// Absolute paths and an empty base directory are left alone.
func (c config) resolvePaths(names []string) []string {
	if c.baseDir == "" {
		return names
	}
	out := make([]string, len(names))
	for i, n := range names {
		if filepath.IsAbs(n) {
			out[i] = n
			continue
		}
		out[i] = filepath.Join(c.baseDir, n)
	}
	return out
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
	vals, raw := make(map[string]string), make(map[string]string)
	if err := parseInto("", data, cfg.expandOptions(), vals, raw); err != nil {
		return err
	}
	// Decode against the file values only.
	cfg.lookuper = MapLookuper{}
	return decode(v, cfg, vals, raw)
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
	out, _, err := readFilesRaw(filenames, expandOptions{enabled: expand}, missingOK)
	return out, err
}

// readFilesRaw is readFiles that also returns the literal, pre-expansion values,
// used to decode fields that opt out of expansion.
func readFilesRaw(filenames []string, exp expandOptions, missingOK bool) (out, raw map[string]string, err error) {
	return readFilesOrigin(filenames, exp, missingOK, nil)
}

// readFilesOrigin is readFilesRaw that also records, in origin, which file
// supplied each key. A nil origin map skips the bookkeeping.
func readFilesOrigin(filenames []string, exp expandOptions, missingOK bool, origin map[string]string) (out, raw map[string]string, err error) {
	out, raw = make(map[string]string), make(map[string]string)
	for _, name := range filenames {
		data, err := os.ReadFile(name)
		if err != nil {
			if os.IsNotExist(err) && (missingOK || isDefaultFile(filenames, name)) {
				continue
			}
			return nil, nil, err
		}
		if err := parseOrigin(name, data, exp, out, raw, origin); err != nil {
			return nil, nil, err
		}
	}
	return out, raw, nil
}

// isDefaultFile reports whether name is the implicit ".env" default, which is
// allowed to be absent.
func isDefaultFile(filenames []string, name string) bool {
	// WithBaseDir may have turned ".env" into "/etc/myapp/.env"; the convention
	// is about the file name, not the directory it lives in.
	return len(filenames) == 1 && filepath.Base(name) == ".env"
}
