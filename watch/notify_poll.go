//go:build !(darwin || dragonfly || freebsd || netbsd || openbsd || windows || linux)

package watch

import (
	"context"
	"os"
	"strconv"
	"time"
)

// notify detects changes by polling the watched files' modification times and
// sizes at PollInterval, calling onChange when any of them changes (including
// creation or deletion). It returns when ctx is cancelled. This is the
// portable fallback used where kqueue is unavailable.
func notify(ctx context.Context, files []string, onChange func()) error {
	interval := PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	last := snapshot(files)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			cur := snapshot(files)
			if cur != last {
				last = cur
				onChange()
			}
		}
	}
}

// snapshot fingerprints the watched files by modification time and size.
func snapshot(files []string) string {
	var b []byte
	for _, f := range files {
		b = append(b, f...)
		b = append(b, 0)
		if fi, err := os.Stat(f); err == nil {
			b = fi.ModTime().AppendFormat(b, time.RFC3339Nano)
			b = append(b, '|')
			b = strconv.AppendInt(b, fi.Size(), 10)
		} else {
			b = append(b, '-')
		}
		b = append(b, '\n')
	}
	return string(b)
}
