package oneenv

import (
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
// whenever one of them changes on disk. onReload is called with the result of
// each reload — nil on success, the error otherwise — and a failed reload
// leaves the last good values in place.
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	var cfg Config
//	err := oneenv.Load(&cfg,
//	    oneenv.WithContext(ctx),
//	    oneenv.WithWatch(func(err error) {
//	        if err != nil {
//	            log.Printf("config reload failed, keeping previous values: %v", err)
//	        }
//	    }),
//	)
//
// Load returns as soon as the first decode is done; watching continues in the
// background until the WithContext context is cancelled. Without WithContext it
// runs for the life of the process.
//
// Reloads write to the target concurrently with your readers, so guard it with
// a mutex, or copy the struct inside onReload and swap a pointer your readers
// hold. A nil onReload is allowed when only the target matters.
//
// The whole file set is watched, including the full cascade under
// WithEnvFiles. For a blocking form that owns the goroutine itself, use the
// oneenv/watch package.
func WithWatch(onReload func(error)) Option {
	return func(c *config) {
		c.watch = true
		c.onReload = onReload
	}
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
			err := Load(v, reloadOpts...)
			if c.onReload != nil {
				c.onReload(err)
			}
		}); err != nil && c.onReload != nil {
			c.onReload(err)
		}
	}()
}
