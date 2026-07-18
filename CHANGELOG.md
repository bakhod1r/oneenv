# Changelog

All notable changes to **oneenv** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-07-18

Initial release: a zero-dependency `.env` parser and struct decoder in one
package.

### Added

- **Core API** — `Parse`, `Load`, `Unmarshal`, `LoadContext`, `ParseContext`,
  and godotenv-style `Read`, `LoadEnv`, `Overload`.
- **Types** — ints/uints/floats, `bool`, `time.Duration`, `time.Time` (with
  `layout`), slices, maps, pointers, nested structs, and any
  `encoding.TextUnmarshaler`.
- **Options** — functional options: `WithFiles`, `WithEnvFiles`, `WithEnvVar`,
  `WithPrefix`, `WithOverride`, `WithExpand`, `WithRequired`, `WithTagKey`,
  `WithLookuper`, `WithTypeParser`, `WithMutator`, `WithValidator`,
  `WithContext`.
- **Tags** — `env` (with `required`, `notEmpty`, `file`, `init`, `unset`,
  `secret` options), `default`, `separator`/`envSeparator`, `layout`,
  `envPrefix`, `desc`, plus an `env-*` alias for every configuration tag
  (`env-default`, `env-separator`, `env-description`, `env-layout`,
  `env-prefix`, `env-required`, `env-notempty`, `env-file`, `env-init`,
  `env-unset`, `env-secret`). When both spellings are present the `env-*` form
  takes priority.
- **Environment-aware file cascade** — `WithEnvFiles()` layers `.env`,
  `.env.local`, `.env.<env>` and `.env.<env>.local` by the active environment
  (`APP_ENV`, then `GO_ENV`; configurable with `WithEnvVar`). `FilesFor`
  exposes the resolved list.
- **Secrets** — `env:"X,file"` reads a value from the file at the resolved
  path; `env:"X,secret"` with `Redacted`/`RedactedMap` masks values in output,
  and the `Secret[T]` wrapper keeps sensitive values out of logs and JSON while
  exposing the real value via `Value()`.
- **Slices of structs** — a `[]Struct` field is decoded from indexed keys
  (`SERVER_0_HOST`, `SERVER_1_HOST`, …).
- **Hot reload** — the `oneenv/watch` subpackage re-decodes on file change via
  native OS notifications (inotify on Linux, kqueue on BSD/macOS,
  ReadDirectoryChangesW on Windows) with modification-time polling as a
  fallback — all standard library, zero dependencies.
- **Marshal & Usage** — `Marshal`/`MarshalMap` render a struct back to `.env`;
  `Usage` prints a `--help` table of the variables a struct consumes.
- **Errors** — positioned `*ParseError` (`file:line`) and every field failure
  collected at once via `errors.Join`; sentinels `ErrNotAStruct`, `ErrRequired`,
  `ErrEmpty`, `ErrSecretFile`, `ErrUnsupportedType`.
- **Hermetic testing** — a `Lookuper` interface (`MapLookuper`, `OSLookuper`,
  `PrefixLookuper`) with no global state, parallel-safe by design.
- **Runnable examples** for the full API surface, so pkg.go.dev renders
  interactive examples.

[1.0.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.0.0
