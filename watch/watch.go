// Package watch adds hot-reloading to oneenv: it re-decodes your configuration
// whenever a watched .env file changes on disk.
//
// It is implemented with the standard library only (no external dependencies),
// so it inherits oneenv's zero-dependency guarantee. On Linux it uses inotify,
// on BSD-family systems (including macOS) kqueue, on Windows
// ReadDirectoryChangesW; elsewhere it falls back to modification-time polling.
//
// For a single call site, oneenv.WithWatch does the same thing as an option on
// Load. This package remains the explicit, blocking form.
package watch

import (
	"context"
	"time"

	"github.com/bakhod1r/oneenv"
	"github.com/bakhod1r/oneenv/internal/notify"
)

// PollInterval is the modification-time polling cadence used on platforms
// without a native notifier. It is ignored where one is available.
var PollInterval = time.Second

// Watch loads the configuration into v once, then re-decodes it into v every
// time one of the watched .env files changes on disk, invoking onReload with
// the result of each reload (nil on success). It blocks until ctx is cancelled
// and returns the initial load error, if any.
//
// The files watched are those the options select (default ".env"); with
// oneenv.WithEnvFiles the whole cascade is watched.
//
// Because Watch writes to v concurrently with your readers, guard v with a
// mutex or swap a pointer inside onReload rather than reading v's fields
// directly from another goroutine.
func Watch(ctx context.Context, v any, onReload func(error), opts ...oneenv.Option) error {
	if err := oneenv.Load(v, opts...); err != nil {
		return err
	}
	notify.PollInterval = PollInterval
	files := oneenv.FilesFor(opts...)
	return notify.Notify(ctx, files, func() {
		onReload(oneenv.Load(v, opts...))
	})
}
