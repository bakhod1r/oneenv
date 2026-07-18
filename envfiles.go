package oneenv

import (
	"os"
	"strings"
)

// defaultEnvVars is the ordered list of environment variables consulted to
// determine the active runtime environment (e.g. "production") when the
// env-file cascade is enabled via WithEnvFiles.
var defaultEnvVars = []string{"APP_ENV", "GO_ENV"}

// FilesFor returns the ordered list of .env file paths that Load would read
// for the given options, including the full environment-aware cascade when
// WithEnvFiles is set. It lets tooling (such as oneenv/watch) discover which
// files to observe without reimplementing the resolution rules.
func FilesFor(opts ...Option) []string {
	cfg := newConfig(opts)
	if cfg.autoEnvFiles {
		return envFileCascade(cfg.files, resolveEnvName(cfg))
	}
	return cfg.files
}

// resolveEnvName returns the active environment name by consulting, in order,
// the configured env variables (default APP_ENV then GO_ENV). It returns the
// first non-empty value, or "" when none is set.
func resolveEnvName(cfg config) string {
	names := cfg.envVarNames
	if len(names) == 0 {
		names = defaultEnvVars
	}
	for _, n := range names {
		if v, ok := cfg.lookuper.Lookup(n); ok && v != "" {
			return v
		}
		// Fall back to the process environment even when a custom lookuper is
		// set, so the environment name is discoverable in tests and real runs.
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
}

// envFileCascade expands each base file into the dotenv-style cascade, in
// increasing priority order (later files override earlier keys):
//
//	<base>            e.g. .env
//	<base>.local      e.g. .env.local
//	<base>.<env>      e.g. .env.production
//	<base>.<env>.local
//
// When envName is empty only <base> and <base>.local are produced. All files
// in the cascade are optional; missing ones are skipped by the reader.
func envFileCascade(base []string, envName string) []string {
	out := make([]string, 0, len(base)*4)
	for _, b := range base {
		out = append(out, b, localVariant(b))
		if envName != "" {
			envFile := b + "." + envName
			out = append(out, envFile, localVariant(envFile))
		}
	}
	return out
}

// localVariant returns the ".local" sibling of a file name, inserting the
// suffix before a trailing dot-extension only for the bare ".env" convention;
// for any name it simply appends ".local".
func localVariant(name string) string {
	if strings.HasSuffix(name, ".local") {
		return name
	}
	return name + ".local"
}
