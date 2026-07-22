---
title: Loading configuration
layout: default
nav_order: 3
---

# Loading configuration
{: .no_toc }

1. TOC
{:toc}

---

`oneenv` gives you a small, layered API. Pick the entry point that fits.

## `Parse` — allocate and decode

`Parse[T]` is the generic convenience form: it allocates a `T`, decodes into it,
and returns the pointer.

```go
cfg, err := oneenv.Parse[Config]()                       // reads the default ".env"

// Point it at one or more files — later files override earlier keys:
cfg, err := oneenv.Parse[Config](
    oneenv.WithFiles(".env", ".env.local", ".env.production"),
)
```

## `Load` — decode into an existing value

```go
var cfg Config
err := oneenv.Load(&cfg, oneenv.WithFiles(".env", ".env.local"))
```

`v` must be a non-nil pointer to a struct. Both `Parse` and `Load` are safe for
concurrent use.

## `Unmarshal` — decode raw bytes

Decodes `.env`-formatted bytes directly, without touching any file or the process
environment. Handy for embedded configs or tests.

```go
data := []byte("PORT=9090\nHOST=localhost")
var cfg Config
err := oneenv.Unmarshal(data, &cfg)
```

## `LoadContext` / `ParseContext` — thread a context

Use these when you register [mutators](advanced#mutators) that need a `context.Context`.

```go
cfg, err := oneenv.ParseContext[Config](ctx, oneenv.WithMutator(resolveSecret))
```

## Low-level API

When you only need the raw values, not a struct:

```go
vals, _ := oneenv.Read(".env", ".env.local")   // merge files → map[string]string
_ = oneenv.LoadEnv(".env", ".env.local")        // sets os.Setenv (existing wins)
_ = oneenv.Overload(".env", ".env.local")       // sets os.Setenv (.env wins)
```

`Read`, `LoadEnv` and `Overload` are variadic — pass as many `.env` files as you
like; later files override earlier keys. With no arguments they default to `.env`.
