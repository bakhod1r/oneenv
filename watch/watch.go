// Package watch adds hot-reloading to oneenv: it re-decodes your configuration
// whenever a watched .env file changes on disk.
//
// It is implemented with the standard library only (no external dependencies),
// so it inherits oneenv's zero-dependency guarantee. On BSD-family systems
// (including macOS) it uses kqueue for real, event-driven notifications; on
// other platforms it falls back to modification-time polling.
package watch

import (
	"context"
	"time"

	"github.com/bakhod1r/oneenv"
)

// PollInterval is the modification-time polling cadence used on platforms
// without a native notifier. It is ignored where kqueue is available.
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
	files := oneenv.FilesFor(opts...)
	return notify(ctx, files, func() {
		onReload(oneenv.Load(v, opts...))
	})
}
