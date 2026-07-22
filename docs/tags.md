---
title: Struct tags
layout: default
nav_order: 5
---

# Struct tags
{: .no_toc }

1. TOC
{:toc}

---

```go
type Config struct {
    Port     int           `env:"PORT" default:"8080" desc:"listen port"`
    Host     string        `env:"HOST,required"`
    Tags     []string      `env:"TAGS" separator:","`
    Labels   map[string]int `env:"LABELS"`               // KEY:VALUE pairs, comma-separated
    Started  time.Time     `env:"STARTED" layout:"2006-01-02"`
    Password string        `env:"PASSWORD,file"`          // value read from the file at this path
    Token    string        `env:"TOKEN,notEmpty"`         // present but empty is an error
    Ignored  string        `env:"-"`                      // never populated
    DB       DBConfig      `envPrefix:"DB_"`              // nested struct
}
```

## Tag reference

| Tag | Applies to | Meaning |
|---|---|---|
| `env:"NAME"` | any field | Environment key. Defaults to the Go field name if omitted. `env:"-"` skips the field. |
| `env:"NAME,required"` | any field | Fail if the value is absent from every source. |
| `env:"NAME,notEmpty"` | any field | Fail if the value is present but empty. |
| `env:"NAME,file"` | string-ish | Treat the resolved value as a **path** and read the file's contents as the real value. |
| `env:"NAME,init"` | pointer / slice / map | Allocate a non-nil zero value even when no value is supplied. |
| `env:"NAME,unset"` | any field | Remove the variable from the process environment after reading it. |
| `env:"NAME,secret"` | any field | Mask the value in `Redacted` / `RedactedMap` output (plain `Marshal` keeps it). |
| `default:"..."` | any field | Fallback value when nothing else provides one. |
| `separator:","` | slice / map | Element separator. `envSeparator` is accepted as an alias. |
| `layout:"..."` | `time.Time` | `time.Parse` layout (default `time.RFC3339`). |
| `envPrefix:"DB_"` | nested struct | Prefix applied to every key inside the nested struct. |
| `desc:"..."` | any field | Human description, surfaced by [`Usage`](output#usage--generate---help). |

Multiple options combine: `env:"TOKEN,required,file"` reads a required secret file.

## `env-*` tag aliases

Every configuration tag also has an `env-*` spelling, so an `env`-prefixed
convention can be used throughout:

| Native | `env-*` alias |
|---|---|
| `default:"8080"` | `env-default:"8080"` |
| `separator:","` (or `envSeparator`) | `env-separator:","` |
| `desc:"..."` | `env-description:"..."` |
| `layout:"..."` | `env-layout:"..."` |
| `envPrefix:"DB_"` | `env-prefix:"DB_"` |
| `env:"NAME,required"` | `env-required:"true"` |
| `env:"NAME,notEmpty"` | `env-notempty:"true"` |
| `env:"NAME,file"` | `env-file:"true"` |
| `env:"NAME,init"` | `env-init:"true"` |
| `env:"NAME,unset"` | `env-unset:"true"` |

```go
type Config struct {
    Port int      `env:"PORT" env-default:"8080" env-description:"listen port"`
    Tags []string `env:"TAGS" env-separator:";"`
    Host string   `env:"HOST" env-required:"true"`
    DB   DBConfig `env-prefix:"DB_"`
}
```

**Priority when both spellings are present:** the `env-*` form always wins; the
native tag is the fallback. A boolean `env-*` tag like `env-required:"false"`
explicitly turns the option off.
