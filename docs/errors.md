---
title: Error handling
layout: default
nav_order: 11
---

# Error handling

A single `Load` reports **every** missing or malformed variable at once (joined
via `errors.Join`), not one at a time — so you fix your config in one pass.

```go
if err := oneenv.Load(&cfg); err != nil {
    var pe *oneenv.ParseError
    if errors.As(err, &pe) {
        fmt.Printf("syntax error at %s:%d — %s\n", pe.File, pe.Line, pe.Msg)
    }

    var fe *oneenv.FieldError
    if errors.As(err, &fe) {
        fmt.Printf("field %s (env %q) failed: %v\n", fe.Field, fe.Key, fe.Err)
    }
}
```

## Sentinel errors

Match the cause with `errors.Is`:

| Sentinel | Returned when |
|---|---|
| `ErrNotAStruct` | The target isn't a non-nil pointer to a struct. |
| `ErrRequired` | A `required` field has no value from any source. |
| `ErrEmpty` | A `notEmpty` field is present but empty. |
| `ErrSecretFile` | A `file` field names a path that can't be read. |
| `ErrUnsupportedType` | A field has a type `oneenv` can't decode. |

## Error types

- **`*ParseError`** — a syntax error in a source, with `File`, `Line` and `Msg`.
- **`*FieldError`** — a decode failure, with the struct `Field` path (e.g. `DB.Port`), the env `Key`, and the underlying `Err` (`Unwrap`-able).
