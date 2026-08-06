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
    if pe, ok := oneenv.AsParseError(err); ok {
        return fmt.Errorf("config %s:%d: %s", pe.File, pe.Line, pe.Msg)
    }
    if fe, ok := oneenv.AsFieldError(err); ok {
        return fmt.Errorf("config field %s (env %s): %v", fe.Field, fe.Key, fe.Err)
    }
    return err
}
```

`AsParseError`, `AsFieldError`, `AsUnknownKeyError` and `AsConstraintError` are
the generic form of `errors.As`: they return the typed error and a `bool`, so it
can be used in the same expression that finds it. Plain `errors.As` with a
declared variable works exactly as before.

## Sentinel errors

Match the cause with `errors.Is`:

| Sentinel | Returned when |
|---|---|
| `ErrNotAStruct` | The target isn't a non-nil pointer to a struct. |
| `ErrRequired` | A `required` field has no value from any source. |
| `ErrEmpty` | A `notEmpty` field is present but empty. |
| `ErrSecretFile` | A `file` field names a path that can't be read. |
| `ErrUnknownVariable` | Strict expansion (`WithExpandStrict`) hit a reference to a variable defined nowhere. Wrapped in a `*ParseError`. |
| `ErrUnsupportedType` | A field has a type `oneenv` can't decode. |
| `ErrUnknownKey` | `WithStrictKeys` found a `.env` key no field consumes. Wrapped in an `*UnknownKeyError`. |
| `ErrPattern` | A value did not match the field's `pattern` tag. Wrapped in a `*ConstraintError`. |
| `ErrNotAllowed` | A value is not one of the field's `enum` values. Wrapped in a `*ConstraintError`. |
| `ErrBadPattern` | A `pattern` tag is not a valid regular expression. |

## Error types

- **`*ParseError`** — a syntax error in a source, with `File`, `Line` and `Msg`.
- **`*FieldError`** — a decode failure, with the struct `Field` path (e.g. `DB.Port`), the env `Key`, and the underlying `Err` (`Unwrap`-able).
