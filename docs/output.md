---
title: Output & generation
layout: default
nav_order: 10
---

# Output & generation
{: .no_toc }

1. TOC
{:toc}

---

## Marshal — struct back to `.env`

`Marshal` renders a struct into `.env` bytes (sorted `KEY=value` lines, values
quoted and escaped when needed). `MarshalMap` returns the flat `map[string]string`
instead. Prefixes from nested structs are applied, so the output round-trips
through `Unmarshal`.

```go
data, _ := oneenv.Marshal(cfg)
os.Stdout.Write(data)
// DB_HOST=localhost
// DB_PORT=5432
// NAME="app one"
// TAGS=a,b

m, _ := oneenv.MarshalMap(cfg)   // map[string]string
```

## Usage — generate `--help`

`Usage[T]` writes a table of the variables a struct consumes — key, type, whether
it's required, its default, and the `desc` tag — ideal for a `--help` flag.

```go
oneenv.Usage[Config](os.Stdout)
```

```text
KEY   TYPE           REQUIRED  DEFAULT  DESCRIPTION
PORT  int            no        8080     listen port
HOST  string         yes                bind host
```

## Example — generate `.env.example`

`Example[T]` writes a ready-to-fill `.env.example` for the variables a struct
consumes: each key with its default (empty when none), preceded by the `desc`
tag, the type, and whether it is required. Secret defaults are never written.

```go
oneenv.Example[Config](os.Stdout)
```

```text
# listen port
# type: int
PORT=8080

# bind host
# type: string, required
HOST=
```

The CLI can also produce one from your existing `.env` files — keys are kept,
values stripped:

```sh
oneenv -example            # writes .env.example next to you
oneenv -f .env -example -o -   # print to stdout
```
