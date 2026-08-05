---
title: Secrets
layout: default
nav_order: 8
---

# Secrets
{: .no_toc }

1. TOC
{:toc}

---

## Secrets from files

Containers and orchestrators mount secrets as files (`/run/secrets/...`,
Kubernetes secret volumes). Add `,file` and `oneenv` reads the file's contents as
the value — the environment variable holds the **path**, not the secret itself.

```go
type Config struct {
    DBPassword string `env:"DB_PASSWORD,file"`
}
```

```dotenv
DB_PASSWORD=/run/secrets/db_password
```

A trailing newline in the file is trimmed. If the file can't be read, the field
error wraps `ErrSecretFile`. Combine with `default` to provide a fallback path, or
with `required` to insist the secret exists.

## Masking secrets in output

Two ways to keep sensitive values out of logs, dumps and `--help` output.

### `,secret` tag + `Redacted`

Mark a field secret and render the struct with `Redacted` (or `RedactedMap`); the
mask replaces only the marked values, while plain `Marshal` still emits the real
value.

```go
type Config struct {
    Host     string `env:"HOST"`
    Password string `env:"PASSWORD,secret"`
}

out, _ := oneenv.Redacted(cfg)
// HOST=db
// PASSWORD=****
```

### `Secret[T]` wrapper

Wrap any decodable type; its `String`, `%v`/`%#v` and JSON forms are always
masked, so it can't leak through logging by accident. The real value is available
via `.Value()`.

```go
type Config struct {
    APIKey oneenv.Secret[string] `env:"API_KEY"`
}

fmt.Println(cfg.APIKey)         // ****
client.Auth(cfg.APIKey.Value()) // the real key
```

## Secrets are never expanded

`WithExpand()` rewrites `${VAR}` and `$VAR` inside values. That is exactly wrong
for a password, which routinely contains `$`:

```dotenv
DB_PASSWORD=pa$$word
```

Expanded as a normal value this yields `pa$word`; `pa$word` yields `pa`, and
`$ecret123` yields the empty string — a connection then fails with an
authentication error that points nowhere near the real cause.

So expansion skips every field that is a secret:

- a `Secret[T]` field,
- a field tagged `,secret` or `env-secret:"true"`,
- any field tagged `,noexpand` or `env-noexpand:"true"`.

Those fields decode the literal text from the file, byte for byte.

```go
type Config struct {
    DSN      string                `env:"DSN"`                 // expanded
    Password oneenv.Secret[string] `env:"DB_PASSWORD"`         // literal
    Token    string                `env:"TOKEN,noexpand"`      // literal
}
```

Two further guards:

- **Single quotes.** `DB_PASSWORD='pa$$word'` is never expanded, for any field,
  matching POSIX shell.
- **`WithExpandStrict()`.** A reference to a variable defined neither earlier in
  the file nor in the process environment becomes a `*ParseError` wrapping
  `ErrUnknownVariable`, naming the file and line, instead of quietly expanding to
  `""`.

```go
cfg, err := oneenv.Parse[Config](oneenv.WithExpandStrict())
// oneenv: .env:4: oneenv: reference to undefined variable: "ecret123"
```
