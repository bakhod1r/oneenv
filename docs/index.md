---
title: Home
layout: default
nav_order: 1
---

<p align="center">
  <img src="assets/logo.svg" alt="oneenv" width="420">
</p>

# oneenv
{: .no_toc }

**Parse `.env` files straight into your Go structs — zero dependencies, pure stdlib.**
{: .fs-6 .fw-300 }

[Get started](getting-started){: .btn .btn-primary .mr-2 }
[GitHub](https://github.com/bakhod1r/oneenv){: .btn }

---

## Why oneenv?

Loading configuration in Go usually takes two separate steps: reading the `.env`
file, and decoding environment variables into a struct. That often means two
dependencies, two APIs, and glue code between them.

`oneenv` does both in **one zero-dependency package** — parsing and decoding in a
single pass over a struct schema that is compiled once and cached — so it stays
[fast and lightweight](benchmarks).

```go
type Config struct {
    Port    int           `env:"PORT" default:"8080"`
    Host    string        `env:"HOST,required"`
    Timeout time.Duration `env:"TIMEOUT" default:"5s"`
    Tags    []string      `env:"TAGS" separator:","`
    DB      DBConfig      `envPrefix:"DB_"`
}

cfg, err := oneenv.Parse[Config]()
if err != nil {
    log.Fatal(err)
}
fmt.Println(cfg.Port) // 8080
```

## Features

- 🪶 **Zero dependencies** — stdlib only. The whole library is a handful of small files.
- 🎯 **Straight to struct** — no `os.Getenv` boilerplate, no glue between two libraries.
- ⚡ **Fast** — byte-level, allocation-light parser; the struct schema is compiled once and cached, so repeated `Load`s are nearly free.
- 🧩 **Rich types** — ints, floats, bool, `time.Duration`, `time.Time`, slices, maps, pointers, nested structs, and any `encoding.TextUnmarshaler`.
- 🔐 **Secrets** — `env:"PASSWORD,file"` reads a value from a path (Docker/K8s `/run/secrets`); `,secret` + `Redacted` and `Secret[T]` keep sensitive values out of logs.
- 🌱 **Env-aware cascade** — `WithEnvFiles()` layers `.env`, `.env.local`, `.env.<env>`, `.env.<env>.local` like Rails/Next.js.
- 🧱 **Slices of structs** — repeated config from indexed keys (`SERVER_0_HOST`, `SERVER_1_HOST`, …).
- 🔄 **Hot reload** — `oneenv/watch` re-decodes on file change via native OS events (inotify / kqueue / Windows), still zero-dependency.
- 🧰 **Extensible** — custom per-type parsers (`WithTypeParser`), value mutators (`WithMutator`), and a pluggable `WithValidator` — all dependency-free.
- ↩️ **Round-trips** — `Marshal` renders a struct back to `.env`, and `Usage` prints a `--help` table of the variables a struct consumes.
- 🧪 **Hermetic tests** — a `Lookuper` interface means no global state and no `t.Setenv`; parallel-safe by design.
- 🧯 **Great errors** — positioned parse errors (`file:line`) and *all* field failures collected at once via `errors.Join`.
- 🔁 **Familiar API** — low-level `Read` / `LoadEnv` / `Overload` and a rich, conventional struct-tag vocabulary.

## Install

```bash
go get github.com/bakhod1r/oneenv
```

Requires **Go 1.26+**.

## Non-goals

To stay fast and dependency-free, `oneenv` deliberately does **not** ship:

- **Multiple config formats** (yaml/json/toml) — `oneenv` is `.env`-only by design.
- **A bundled validation library** — use [`WithValidator`](advanced#validation) to attach your own.

## License

[MIT](https://github.com/bakhod1r/oneenv/blob/main/LICENSE) © [bakhod1r](https://github.com/bakhod1r)
