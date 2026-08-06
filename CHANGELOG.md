# Changelog

All notable changes to **oneenv** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.7.0] - 2026-08-06

### Added

- **A report of what is not set** — the keys a struct declares that no source
  supplied. `WithTable` prints a summary line under the table
  (`2 not set: SENTRY_DSN, SMTP_HOST`), `WithLogger` logs one warning per load,
  and `Report.Missing()` / `Report.MissingKeys()` expose the list. A key that
  fell back to its `default` counts as supplied. This is the mirror image of
  `WithStrictKeys`, which reports keys in a file that the struct does not know.

## [1.6.0] - 2026-08-06

### Added

- **`WithWatch(onReload)`** — hot reload as an option, without importing a
  second package:

  ```go
  oneenv.Load(&cfg,
      oneenv.WithContext(ctx),
      oneenv.WithWatch(func(err error) { ... }),
  )
  ```

  `Load` returns after the first decode and keeps re-decoding into the same
  target in the background until the `WithContext` context is cancelled. A
  failed reload leaves the last good values in place, and watching never starts
  if the initial load fails.
- **`SetPollInterval` / `PollInterval`** — the modification-time cadence used
  where no native notifier exists.

### Changed

- The notifier implementations moved to `internal/notify`, shared by
  `WithWatch` and the `oneenv/watch` package. `watch.Watch` and
  `watch.PollInterval` are unchanged.

## [1.5.1] - 2026-08-06

### Added

- **`AsParseError`, `AsFieldError`, `AsUnknownKeyError`, `AsConstraintError`** —
  the generic form of `errors.As`: each returns the typed error and a `bool`, so
  it can be used in the same expression that finds it.

  ```go
  if pe, ok := oneenv.AsParseError(err); ok {
      return fmt.Errorf("config %s:%d: %s", pe.File, pe.Line, pe.Msg)
  }
  ```

  Plain `errors.As` with a declared variable keeps working unchanged.

## [1.5.0] - 2026-08-06

### Changed

- **The generated example file has a new shape.** Every key is written without a
  value — the file is a form to fill in — and what used to be three comment
  lines above each key is now one inline comment after it:

  ```dotenv
  APP_ENV=  # type: string, required, allowed: local|staging|prod
  APP_PORT= # type: int, default: 8080, http listen port
  ```

  A `default` or `example` tag is described in the comment instead of being
  assigned, and each struct gets its own section, separated by two blank lines
  and headed with the field path, so variables that belong together stay
  together. Applies to both `Example[T]` and `WithWriteExample`.

## [1.4.3] - 2026-08-06

### Added

- **`TYPE` and `NULL` columns** in the `WithTable` output, and a `Null` field on
  `Report` entries (also shown by `Explain` and logged by `WithLogger`). `NULL`
  marks a key that resolved to nothing, whether unset or set to an empty value.

### Changed

- **An empty secret prints as `""`, not `****`.** Masking a blank password
  claimed a value that was not there; a set password and an unset one now look
  different in the table.

## [1.4.2] - 2026-08-06

### Changed

- **`WithSecretReveal(n)` narrows its window instead of giving up.** A value too
  short to give both ends `n` characters previously fell back to a full mask;
  now at most half of it is shown, split evenly between the two ends — an
  8-character secret with `n=4` shows two characters at each end. Values shorter
  than eight characters are still masked in full, as is everything under
  `WithRedacted()`.

## [1.4.1] - 2026-08-06

### Changed

- **`WithWriteExample()`** takes no argument: the generated example file lands
  beside the first `.env` the call reads (`.env` → `.env.example`, honoring
  `WithBaseDir`). Passing a path still works.

## [1.4.0] - 2026-08-06

### Added

- **Typo detection** — `WithStrictKeys()` turns a `.env` key that no field
  consumes into an `*UnknownKeyError` (`errors.Is` matches `ErrUnknownKey`), so
  `PORRT=9999` fails at startup instead of silently doing nothing.
- **Source tracing** — `WithReport(&rep)` captures the full resolution detail of
  a load. `rep.Source("PORT")` names the **file** a value came from
  (`.env.production`) or the layer (`env`, `default`, `unset`), `rep.Explain`
  renders everything known about one key, and `rep.Entries()` exposes the rest.
- **`Diff` / `DiffString`** — report every key whose value differs between two
  configurations, with secrets masked on both sides but still reported as
  changed.
- **`Hash` / `HashFull`** — a short, stable fingerprint of the effective
  configuration, for printing on deploy.
- **`WithBaseDir(dir)`** — resolve relative `.env` paths against a directory.
- **`WithWriteExample([path])`** — regenerate an example file from the struct on
  every successful load, rewriting it only when its contents change. With no
  argument it sits next to the `.env` it documents (`.env` → `.env.example`,
  honoring `WithBaseDir`).
- **`WithOutput(w)`** — the single place a writer is named; everything oneenv
  prints goes to `os.Stdout` otherwise.
- **`WithRedacted()`** — mask every secret in full, overriding
  `WithSecretReveal` whatever the option order.
- **New struct tags** — `example` (sample value for `.env.example`), `alias`
  (former key spellings, with a deprecation warning when used), `deprecated`
  (warn whenever the key is used), `enum` (allowed values, `ErrNotAllowed`) and
  `pattern` (regexp, `ErrPattern`). Each has an `env-*` spelling too.
- **CLI subcommands** — `oneenv doctor`, `lint`, `format [-w]`, `explain KEY`,
  `init` and `migrate [-w] OLD=NEW`.

### Changed

- **Secrets are now masked in full by default** in the table, logger and `Print`
  output. `WithSecretReveal(n)` is the only way to reveal any part of one; it
  previously defaulted to revealing four characters at each end.
- **The `SOURCE` column names the file** a value came from, e.g. `.env.local`,
  instead of the generic `file`.
- `WithTable()` and `WithLogger()` no longer require an argument — the table
  goes to stdout and the logger defaults to `slog.Default()`. `WithLogger(l)`
  still accepts a specific logger.
- `Print` takes the value first and no writer: `oneenv.Print(cfg)`. Pair it with
  `WithOutput(w)` to capture the output. **This is a breaking change** to the
  `Print` and `WithTable` signatures introduced hours earlier in 1.3.0.

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

[1.7.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.7.0
[1.6.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.6.0
[1.5.1]: https://github.com/bakhod1r/oneenv/releases/tag/v1.5.1
[1.5.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.5.0
[1.4.3]: https://github.com/bakhod1r/oneenv/releases/tag/v1.4.3
[1.4.2]: https://github.com/bakhod1r/oneenv/releases/tag/v1.4.2
[1.4.1]: https://github.com/bakhod1r/oneenv/releases/tag/v1.4.1
[1.4.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.4.0
[1.3.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.3.0
[1.2.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.2.0
[1.1.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.1.0
[1.0.0]: https://github.com/bakhod1r/oneenv/releases/tag/v1.0.0
