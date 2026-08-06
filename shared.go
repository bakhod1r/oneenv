package oneenv

import (
	"context"
	"reflect"
	"sync"
)

// Configuration is read once per process for a given type and then handed to
// everyone who asks. sharedOf keeps one entry per type; the sync.Once inside it
// is what makes the first caller do the work and the rest wait for it rather
// than each reading the files again.
var sharedRegistry sync.Map // reflect.Type -> *sharedEntry

type sharedEntry struct {
	once  sync.Once
	value any // *T for Shared, *Live[T] for SharedLive
	err   error
}

// sharedOf returns the entry for T, creating it on first use.
func sharedOf[T any]() *sharedEntry {
	t := reflect.TypeFor[T]()
	if e, ok := sharedRegistry.Load(t); ok {
		return e.(*sharedEntry)
	}
	e, _ := sharedRegistry.LoadOrStore(t, &sharedEntry{})
	return e.(*sharedEntry)
}

// WithOnce reads the configuration once per process for the target's type and
// serves every later call from what the first one decoded — so a config loaded
// from six packages is parsed once:
//
//	cfg, err := oneenv.Parse[Config](oneenv.WithOnce())
//
//	var cfg Config
//	err := oneenv.Load(&cfg, oneenv.WithOnce())
//
// The first call does the work; concurrent callers wait for it, and later ones
// are handed a copy of the result, its error included, without touching the
// files. Options passed by those later calls are ignored — the first call
// decided. Each caller gets its own copy, so writing to one target cannot
// disturb another.
//
// Shared and MustShared are the same thing in one call. Use ResetShared in
// tests to forget what was remembered.
func WithOnce() Option {
	return func(c *config) { c.once = true }
}

// loadOnce is the WithOnce path: the first call for this type decodes into the
// registry, and every call copies the stored value into its own target.
func loadOnce(v any, opts []Option) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return ErrNotAStruct
	}
	t := rv.Elem().Type()

	e, _ := sharedRegistry.LoadOrStore(t, &sharedEntry{})
	entry := e.(*sharedEntry)
	entry.once.Do(func() {
		stored := reflect.New(t)
		// The stored value is decoded by the plain path: WithOnce is switched
		// off so this cannot recurse.
		if err := Load(stored.Interface(), append(append([]Option{}, opts...), noOnce)...); err != nil {
			entry.err = err
			return
		}
		entry.value = stored.Interface()
	})
	if entry.err != nil {
		return entry.err
	}
	rv.Elem().Set(reflect.ValueOf(entry.value).Elem())
	return nil
}

// noOnce switches WithOnce back off, so the load that fills the registry does
// not re-enter it.
func noOnce(c *config) { c.once = false }

// Shared loads the configuration once per process and returns the same value to
// every caller afterwards. It is the answer to "config is needed in six
// packages and should not be parsed six times":
//
//	cfg, err := oneenv.Shared[Config]()
//
// The first call does the work — later ones return its result, including its
// error, without touching the files again, and the options they pass are
// ignored. Concurrent first calls block until the one that won has finished, so
// the load happens exactly once.
//
// The returned pointer is shared, so treat the value as read-only: nothing in
// oneenv writes to it after this returns. When the configuration also has to
// follow the files, use SharedLive, which keeps its updates behind a lock.
func Shared[T any](opts ...Option) (*T, error) {
	e := sharedOf[T]()
	e.once.Do(func() {
		v := new(T)
		if err := Load(v, append(append([]Option{}, opts...), noOnce)...); err != nil {
			e.err = err
			return
		}
		e.value = v
	})
	if e.err != nil {
		return nil, e.err
	}
	return e.value.(*T), nil
}

// MustShared is Shared for a configuration a program cannot start without: it
// panics rather than returning an error, so a broken configuration fails at
// startup instead of being handled at every call site.
func MustShared[T any](opts ...Option) *T {
	v, err := Shared[T](opts...)
	if err != nil {
		panic(err)
	}
	return v
}

// SharedLive is Shared for a configuration that also follows the files: the
// first call loads it and starts watching, later calls return the same holder.
// Readers call Get for a snapshot, so a reload never races with them.
//
//	live, err := oneenv.SharedLive[Config](ctx, oneenv.WithEnvFiles())
//	cfg := live.Get()
//
// The context of the first call is the one that governs watching; later calls
// ignore both their context and their options.
func SharedLive[T any](ctx context.Context, opts ...Option) (*Live[T], error) {
	e := sharedOf[*Live[T]]()
	e.once.Do(func() {
		l, err := NewLive[T](ctx, opts...)
		if err != nil {
			e.err = err
			return
		}
		e.value = l
	})
	if e.err != nil {
		return nil, e.err
	}
	return e.value.(*Live[T]), nil
}

// ResetShared drops the process-wide value remembered for T, so the next Shared
// or SharedLive call loads it again. It exists for tests, which need each case
// to start from a clean slate; production code has no reason to call it.
//
// A SharedLive holder dropped this way keeps watching until its context is
// cancelled, so cancel it first.
func ResetShared[T any]() {
	sharedRegistry.Delete(reflect.TypeFor[T]())
	sharedRegistry.Delete(reflect.TypeFor[*Live[T]]())
}
