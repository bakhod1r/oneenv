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
