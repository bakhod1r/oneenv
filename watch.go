package oneenv

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/bakhod1r/oneenv/internal/notify"
)

// PollInterval is the modification-time polling cadence used by WithWatch on
// platforms without a native notifier (inotify, kqueue, ReadDirectoryChangesW).
// It is ignored where one is available.
func PollInterval() time.Duration { return notify.PollInterval }

// SetPollInterval sets the polling cadence used where no native notifier
// exists. Call it before the first WithWatch load.
func SetPollInterval(d time.Duration) {
	if d > 0 {
		notify.PollInterval = d
	}
}

// WithWatch keeps the configuration up to date: after the initial load, oneenv
// watches the .env files this call reads and re-decodes into the same target
// whenever one of them changes on disk. A failed reload leaves the last good
// values in place.
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	var cfg Config
//	err := oneenv.Load(&cfg, oneenv.WithContext(ctx), oneenv.WithWatch())
//
// Load returns as soon as the first decode is done; watching continues in the
// background until the WithContext context is cancelled. Without WithContext it
// runs for the life of the process.
//
// Every reload is reported through the logger installed with WithLogger — a
// debug record on success, a warning on failure — so nothing more is needed to
// see what happened. Pass a callback when the program has to act on a reload,
// for instance to hand the new values to its readers:
//
//	oneenv.WithWatch(func(err error) {
//	    if err != nil {
//	        return // previous values still stand
//	    }
//	    mu.Lock()
//	    current = live
//	    mu.Unlock()
//	})
//
// Reloads write to the target concurrently with your readers. Hand oneenv the
// lock that guards it with WithMutex, or copy the struct inside the callback
// and let your readers use that copy.
//
// The whole file set is watched, including the full cascade under
// WithEnvFiles. For a blocking form that owns the goroutine itself, use the
// oneenv/watch package.
func WithWatch(onReload ...func(error)) Option {
	return func(c *config) {
		c.watch = true
		if len(onReload) > 0 {
			c.onReload = onReload[0]
		}
	}
}

// WithMutex hands oneenv the lock that guards the target, so a WithWatch reload
// and your readers no longer race: oneenv holds the lock for exactly the moment
// it swaps the new configuration in.
//
//	var (
//	    mu  sync.RWMutex
//	    cfg Config
//	)
//	oneenv.Load(&cfg, oneenv.WithContext(ctx), oneenv.WithMutex(&mu), oneenv.WithWatch())
//
//	// readers
//	mu.RLock()
//	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
//	mu.RUnlock()
//
// A *sync.Mutex works too; with a *sync.RWMutex readers take RLock and oneenv
// takes the write lock. Hold it only while reading fields — a reader that keeps
// it across slow work blocks every reload.
//
// The option is only meaningful together with WithWatch, since nothing else
// writes to the target after Load returns.
func WithMutex(mu sync.Locker) Option {
	return func(c *config) { c.mu = mu }
}

// reload decodes into a fresh value of the target's type and only overwrites
// the target once that has fully succeeded.
//
// Decoding straight into the target would leave it half-updated when a reload
// fails partway — the promise that a failed reload keeps the previous values
// would hold for the fields that were never reached and break for the ones
// already written. The swap at the end is a single assignment, so a reader
// never sees a mix of the old and new configuration for a given field.
//
// The swap itself is guarded by the WithMutex lock when one was given;
// otherwise readers need their own synchronization, since the swap races with
// concurrent reads of the target.
func reload(v any, mu sync.Locker, opts []Option) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return ErrNotAStruct
	}
	fresh := reflect.New(rv.Elem().Type())
	// Decoding happens outside the lock: it reads files and may take a while,
	// and until it succeeds it has nothing to publish.
	if err := Load(fresh.Interface(), opts...); err != nil {
		return err
	}
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	rv.Elem().Set(fresh.Elem())
	return nil
}

// startWatch launches the background reload loop, once the initial decode has
// succeeded. It returns immediately; the loop ends with the context.
func (c config) startWatch(v any, opts []Option) {
	if !c.watch {
		return
	}
	files, _ := c.resolveFiles()
	ctx := c.context()
	// Each reload runs the same options with watching switched off, so a reload
	// never starts a second watcher.
	reloadOpts := append(append([]Option{}, opts...), func(rc *config) { rc.watch = false })
	go func() {
		// A notifier that cannot start (an unreadable directory, an exhausted
		// watch descriptor table) is reported through the same callback as a
		// failed reload, rather than taking the process down.
		if err := notify.Notify(ctx, files, func() {
			err := reload(v, c.mu, reloadOpts)
			c.logReload(err)
			if c.onReload != nil {
				c.onReload(err)
			}
		}); err != nil {
			c.logWatchFailed(err)
			if c.onReload != nil {
				c.onReload(err)
			}
		}
	}()
}

// Live holds a configuration that oneenv keeps up to date, with the lock it
// needs on the inside. Readers call Get, which hands back a snapshot, so no
// goroutine ever touches the value a reload is writing:
//
//	live, err := oneenv.NewLive[Config](ctx, oneenv.WithEnvFiles())
//	if err != nil {
//	    return err
//	}
//
//	cfg := live.Get()
//	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
//
// This is the form to reach for when the configuration is read from several
// goroutines. WithMutex is the equivalent for a value you own yourself, and
// WithWatch the low-level form.
type Live[T any] struct {
	mu      sync.RWMutex
	value   T
	lastErr error

	// scratch is what the watcher decodes into. Only the watcher's goroutine
	// touches it, and its contents are published to value under the write lock,
	// so Get never observes a half-written configuration.
	scratch T
}

// NewLive loads the configuration once and then keeps it current until ctx is
// cancelled, returning the holder. The initial load's error is returned
// directly; a later reload failure is kept in Err and leaves the last good
// value in place.
func NewLive[T any](ctx context.Context, opts ...Option) (*Live[T], error) {
	l := &Live[T]{}
	watchOpts := append(append([]Option{}, opts...), WithContext(ctx), WithWatch(l.publish))
	if err := Load(&l.scratch, watchOpts...); err != nil {
		return nil, err
	}
	l.value = l.scratch
	return l, nil
}

// publish moves a freshly reloaded configuration into place, or records why it
// could not be. It runs on the watcher's goroutine, right after the reload.
func (l *Live[T]) publish(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lastErr = err
	if err == nil {
		l.value = l.scratch
	}
}

// Get returns a snapshot of the current configuration. It is safe to call from
// any goroutine, and the value it returns is yours: a later reload cannot
// change it underneath you.
func (l *Live[T]) Get() T {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.value
}

// Err returns the error from the most recent reload, or nil when the last one
// succeeded. The value Get returns is always the last one that loaded cleanly.
func (l *Live[T]) Err() error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.lastErr
}
