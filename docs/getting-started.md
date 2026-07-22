---
title: Getting started
layout: default
nav_order: 2
---

# Getting started
{: .no_toc }

1. TOC
{:toc}

---

## Install

```bash
go get github.com/bakhod1r/oneenv
```

Requires **Go 1.26+**.

## Quick start

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/bakhod1r/oneenv"
)

type Config struct {
    Port    int           `env:"PORT" default:"8080"`
    Host    string        `env:"HOST,required"`
    Timeout time.Duration `env:"TIMEOUT" default:"5s"`
}

func main() {
    // Reads ".env" (if present) and merges it with the process environment.
    cfg, err := oneenv.Parse[Config]()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%s:%d (timeout %s)\n", cfg.Host, cfg.Port, cfg.Timeout)
}
```

```dotenv
# .env
HOST=localhost
PORT=9090
TIMEOUT=30s
```

## Resolution priority

For each field the value is resolved in this order:

1. **Explicit environment variable** (via the `Lookuper`, default `os.LookupEnv`)
2. **`.env` file** value
3. **`default` tag**

With `WithOverride()`, `.env` file values take precedence over the process
environment. A field with no value from any source and no `default` is left at its
zero value — unless it is `required`.

## Where to next

- [Loading configuration](loading) — `Parse`, `Load`, `Unmarshal`, and the low-level API.
- [Options](options) — every functional option in one table.
- [Struct tags](tags) — the full tag vocabulary.
- [Supported types](types) — what can be decoded.
