---
title: Options
layout: default
nav_order: 4
---

# Options

`oneenv` uses the functional-options pattern — the zero config is already sensible,
options are applied in order, and a later option wins over an earlier one.

| Option | Description |
|---|---|
| `WithFiles(names...)` | `.env` files to read (default `.env`); later files override earlier keys. A missing default `.env` is not an error. |
| `WithEnvFiles()` | Enable the environment-aware cascade: also read `<base>.local`, `<base>.<env>`, `<base>.<env>.local` (all optional). |
| `WithEnvVar(names...)` | Which env variables name the active environment for `WithEnvFiles` (default `APP_ENV`, then `GO_ENV`). |
| `WithPrefix(p)` | Restrict lookups to keys carrying a prefix, e.g. `APP_` maps `env:"PORT"` to `APP_PORT`. |
| `WithOverride()` | Let `.env` values overwrite variables already in the process env (default: existing wins). |
| `WithExpand()` | Enable `${VAR}` / `$VAR` expansion inside values. |
| `WithRequired()` | Treat every field as required, as if each carried `,required`. |
| `WithTagKey(k)` | Change the struct tag key (default `env`). |
| `WithLookuper(l)` | Swap the env source — pass a `MapLookuper` for hermetic tests. |
| `WithTypeParser[T](fn)` | Register a custom parser for a specific type `T`. |
| `WithMutator(m)` | Transform each raw value before decoding (receives a `context.Context`). |
| `WithValidator(fn)` | Run a validation callback on the decoded struct — plug in any validator, zero-dep. |
| `WithContext(ctx)` | Context passed to mutators (also via `LoadContext` / `ParseContext`). |
