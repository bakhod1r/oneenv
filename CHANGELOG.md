# Changelog

All notable changes to **oneenv** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0] - 2026-08-06

### Added

- **`WithTable(w)`** — prints an aligned `KEY / VALUE / SOURCE` table to `w`
  once a `Load` succeeds, one row per resolved field. `SOURCE` names the layer
  that won: `env`, `file`, `default` or `unset`. Nothing is printed when the
  load fails.
- **`WithLogger(l *slog.Logger)`** — logs every file read and every field
  resolved at debug level, with the env key, struct field path, source and
  value.
- **`WithSecretReveal(n)`** — how many leading and trailing characters of a
  secret stay visible in the `WithTable` and `WithLogger` output (default `4`,
  `0` masks everything). A value too short to reveal both ends without
  overlapping is masked completely, so a short secret never leaks in full.
- **`Print(w, v, opts...)`** — renders an already-decoded config as a
  `KEY / VALUE` table with secrets masked, for use outside a `Load`.
- **`Source`** type with the `SourceEnv`, `SourceFile`, `SourceDefault` and
  `SourceUnset` constants.

`Load` remains silent unless one of these options is passed, and the extra
lookup the source column needs is skipped entirely when it is not. `Redacted`
and `RedactedMap` are unchanged: they still mask a secret in full.

## [1.2.0] - 2026-08-05

### Fixed

- **Secrets are no longer corrupted by `WithExpand()`.** Expansion treated every
  `$` in a value as a variable reference, so a password from a password manager
  was silently rewritten: `pa$$word` decoded as `pa$word`, `pa$word` as `pa`, and
  `$ecret123` as `""`. `${PATH}` inside a DSN expanded to the real `PATH`. Fields
  holding secrets now decode the literal text from the file: `Secret[T]` fields,
  fields tagged `,secret` / `env-secret:"true"`, and fields tagged `,noexpand`.
  Non-secret fields expand exactly as before.

### Added

- **`,noexpand` tag** (and `env-noexpand:"true"`) — opt a non-secret field out of
  `${VAR}` / `$VAR` expansion.
- **`WithExpandStrict()`** — expansion where a reference to a variable defined
  neither earlier in the file nor in the process environment fails with a
  `*ParseError` wrapping the new `ErrUnknownVariable`, instead of expanding to
  the empty string.
- **`ErrUnknownVariable`** sentinel, and `ParseError.Err` with an `Unwrap`
  method, so parse failures can be matched with `errors.Is`.

## [1.1.0] - 2026-07-19

### Added

- **`Example[T]`** — generates a ready-to-fill `.env.example` from a struct:
  each key with its default, preceded by comments carrying the `desc` tag, the
  Go type, and whether it is required. Defaults of `secret` fields are never
  written.
- **CLI `-example` flag** — `oneenv -example` writes a `.env.example` from the
  merged `.env` files: keys kept, values stripped, original comments above each
  key preserved, plus inferred-type and required comments. `-o` sets the output
  path (`-` for stdout, default `.env.example`).

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

[1.3.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.3.0
[1.2.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.2.0
[1.1.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.1.0
[1.0.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.0.0
