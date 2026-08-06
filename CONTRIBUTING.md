# Contributing to oneenv

Thanks for taking the time to contribute. This is a small, focused library, so
the bar is simple: keep it **zero-dependency**, keep it **fast**, and keep the
public API **stable**.

## Ground rules

1. **Stdlib only.** `oneenv` must never require a third-party module. `go.mod`
   has no `require` block, and CI fails if one appears. Test helpers are no
   exception.
2. **No global state.** Environment access goes through the `Lookuper`
   interface so tests stay hermetic and parallel-safe. Don't reach for
   `os.Getenv` or `t.Setenv` in new code.
3. **Backwards compatibility.** The module is at v1. Anything exported is a
   promise. New behaviour goes behind a new `Option`, not a changed default.
4. **Secrets stay literal.** Values that flow into `Secret[T]`, `,secret`, or
   `,noexpand` fields must never be rewritten by expansion or logged in an
   error message. If you touch `parser.go`, `decoder.go`, or `secret.go`, add a
   test proving a `$`-heavy password survives untouched.

## Getting started

```bash
git clone https://github.com/bakhod1r/oneenv
cd oneenv
go build ./...
go test -race ./...
```

There is nothing to install — no linters config, no code generation, no make
targets.

## Before you open a pull request

Run what CI runs:

```bash
gofmt -l .              # must print nothing
go vet ./...
go test -race -cover ./...
go build ./examples/...
go mod tidy && git diff --exit-code -- go.mod go.sum
```

Optional but appreciated for parser/decoder changes:

```bash
cd internal/bench && go test -run '^$' -bench . -benchmem ./...
```

Include the before/after benchmark output in the pull request description if
your change touches a hot path.

## Tests

Tests live next to the code they cover, in the same package where they need
access to internals and in `oneenv_test` where they exercise the public API.
Table-driven tests are the norm. A bug fix needs a test that fails before the
fix and passes after it.

## Commit messages

Conventional Commits, lowercase scope in parentheses:

```
fix(expand): never expand secret values
feat(watch): add poll fallback for unsupported filesystems
docs: document the .env cascade order
```

Types in use: `feat`, `fix`, `perf`, `docs`, `test`, `refactor`, `chore`.

## Reporting bugs

Open an issue with:

- the Go version and OS,
- a minimal struct plus the `.env` content that reproduces it,
- what you expected and what you got.

For anything security-sensitive, follow [SECURITY.md](SECURITY.md) instead of
opening a public issue.

## Proposing features

Open an issue first and describe the configuration problem you hit. Features
that can be expressed as a struct tag or an `Option` are the easiest to land;
features that change default decoding behaviour usually aren't accepted.

By contributing you agree that your work is licensed under the
[MIT License](LICENSE).
