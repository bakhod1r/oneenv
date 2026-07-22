---
title: Advanced
layout: default
nav_order: 9
---

# Advanced
{: .no_toc }

1. TOC
{:toc}

---

## Hot reload

The `oneenv/watch` subpackage re-decodes your struct whenever a watched `.env`
file changes. It uses native OS notifications — **inotify** on Linux, **kqueue**
on BSD/macOS and **ReadDirectoryChangesW** on Windows — with modification-time
**polling** as a fallback on any other platform. All standard library, so the
zero-dependency guarantee still holds.

```go
import "github.com/bakhod1r/oneenv/watch"

var cfg Config
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Blocks until ctx is cancelled. Read cfg inside onReload (or guard it with a
// mutex): Watch writes cfg concurrently with your readers.
watch.Watch(ctx, &cfg, func(err error) {
    if err != nil {
        log.Printf("reload failed: %v", err)
        return
    }
    log.Printf("config reloaded")
}, oneenv.WithEnvFiles())
```

## Custom type parsers

Register a parser for any specific type without implementing `TextUnmarshaler`.
It also applies to that type inside slices, maps and pointers.

```go
import "net"

cfg, err := oneenv.Parse[Config](
    oneenv.WithTypeParser(func(s string) (net.IP, error) {
        return net.ParseIP(s), nil
    }),
)
```

{: .note }
Registering any type parser bypasses the shared schema cache for that call, since
the same type could decode differently between calls. Register your parsers once
and reuse the option set to keep things fast.

## Mutators

A `Mutator` transforms every raw value after lookup and before decoding. Mutators
run in registration order, each receiving the previous one's output and a
`context.Context`. Perfect for resolving indirections (secret managers, templating)
or normalising values.

```go
cfg, err := oneenv.ParseContext[Config](ctx,
    oneenv.WithMutator(func(ctx context.Context, key, val string) (string, error) {
        if ref, ok := strings.CutPrefix(val, "sm://"); ok {
            return secretmanager.Resolve(ctx, ref)   // your code
        }
        return val, nil
    }),
)
```

Returning an error from a mutator fails that field and is reported like any other
field error.

## Validation

`oneenv` stays dependency-free, so it ships no validator — but `WithValidator`
lets you attach any one you like. It runs once, on the fully decoded struct, after
a successful decode.

```go
import "github.com/go-playground/validator/v10"

v := validator.New()
cfg, err := oneenv.Parse[Config](
    oneenv.WithValidator(func(c any) error { return v.Struct(c) }),
)
```

## Testing without global state

The decoder never touches `os.Getenv` directly — everything flows through a
`Lookuper`. In tests, pass a `MapLookuper` and skip `os.Setenv` / `t.Setenv`
entirely, so tests stay hermetic and `t.Parallel()`-safe.

```go
func TestConfig(t *testing.T) {
    t.Parallel()

    lookuper := oneenv.MapLookuper{"PORT": "3000", "HOST": "test"}

    var cfg Config
    if err := oneenv.Load(&cfg, oneenv.WithLookuper(lookuper)); err != nil {
        t.Fatal(err)
    }
    // assert on cfg…
}
```
