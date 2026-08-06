// Package notify reports changes to a set of files, using the best mechanism
// the platform offers: inotify on Linux, kqueue on BSD and macOS,
// ReadDirectoryChangesW on Windows, and modification-time polling elsewhere.
//
// It exists so both oneenv.WithWatch and the oneenv/watch package can share one
// implementation without an import cycle. Everything here is standard library
// only, preserving oneenv's zero-dependency guarantee.
package notify

import "time"

// PollInterval is the modification-time polling cadence used on platforms
// without a native notifier. It is ignored where one is available.
var PollInterval = time.Second
