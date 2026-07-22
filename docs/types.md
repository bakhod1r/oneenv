---
title: Supported types
layout: default
nav_order: 6
---

# Supported types

| Category | Types |
|---|---|
| Strings | `string` |
| Booleans | `bool` (`strconv.ParseBool`: `1`, `t`, `true`, `TRUE`, …) |
| Integers | `int`, `int8`…`int64`, `uint`, `uint8`…`uint64` |
| Floats | `float32`, `float64` |
| Time | `time.Duration` (`"5s"`, `"1h30m"`), `time.Time` (RFC3339 or `layout` tag) |
| Collections | `[]T` (any supported `T`), `map[string]T` (`key:value` pairs) |
| Pointers | `*T` for any supported `T` (allocated only when a value is present) |
| Nested | structs (recursed into, with optional `envPrefix`) |
| Custom | any `encoding.TextUnmarshaler`, or any type via [`WithTypeParser`](advanced#custom-type-parsers) |

Slices and maps use the field's separator (default `,`); map entries are
`key:value`. For example `LABELS=a:1,b:2` decodes into `map[string]int{"a":1,"b":2}`.

## Slices of structs

A `[]Struct` field is decoded from indexed keys: the field's env name, then
`_<index>_`, then the element's key. Decoding starts at index `0` and stops at
the first index with no keys present.

```go
type Server struct {
    Host string `env:"HOST"`
    Port int    `env:"PORT"`
}
type Config struct {
    Servers []Server `env:"SERVER"`
}
```

```dotenv
SERVER_0_HOST=a
SERVER_0_PORT=1
SERVER_1_HOST=b
SERVER_1_PORT=2
```
