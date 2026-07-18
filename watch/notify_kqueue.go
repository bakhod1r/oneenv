//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package watch

import (
	"context"
	"path/filepath"
	"syscall"
	"time"
)

// vnodeEvents are the kqueue vnode conditions we react to: content writes,
// deletes, renames, attribute changes and (for directories) additions.
const vnodeEvents = syscall.NOTE_WRITE | syscall.NOTE_DELETE | syscall.NOTE_RENAME | syscall.NOTE_ATTRIB

// notify watches each file and its parent directory via kqueue and calls
// onChange whenever any of them changes. Watching the directory as well as the
// file catches atomic saves, where an editor replaces the file by renaming a
// temp file over it and the file-level watch would otherwise be lost. It
// returns when ctx is cancelled.
func notify(ctx context.Context, files []string, onChange func()) error {
	kq, err := syscall.Kqueue()
	if err != nil {
		return err
	}
	defer syscall.Close(kq)

	// The set of paths to keep watched: every file plus its parent directory.
	paths := make([]string, 0, len(files)*2)
	seen := make(map[string]struct{})
	for _, f := range files {
		for _, p := range []string{f, filepath.Dir(f)} {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			paths = append(paths, p)
		}
	}

	// fds holds the currently open watch descriptors, rebuilt after each event
	// so that watches survive renames and re-created files.
	var fds []int
	rearm := func() {
		for _, fd := range fds {
			syscall.Close(fd)
		}
		fds = fds[:0]
		for _, p := range paths {
			fd, err := syscall.Open(p, syscall.O_RDONLY, 0)
			if err != nil {
				continue // file may not exist yet; picked up on a later rearm
			}
			ev := syscall.Kevent_t{
				Ident:  uint64(fd),
				Filter: syscall.EVFILT_VNODE,
				Flags:  syscall.EV_ADD | syscall.EV_CLEAR,
				Fflags: vnodeEvents,
			}
			if _, err := syscall.Kevent(kq, []syscall.Kevent_t{ev}, nil, nil); err != nil {
				syscall.Close(fd)
				continue
			}
			fds = append(fds, fd)
		}
	}
	rearm()
	defer func() {
		for _, fd := range fds {
			syscall.Close(fd)
		}
	}()

	// A 250ms kevent timeout bounds how quickly we notice ctx cancellation.
	timeout := syscall.Timespec{Nsec: int64(250 * time.Millisecond)}
	out := make([]syscall.Kevent_t, len(paths)+1)
	var pending bool
	for {
		if ctx.Err() != nil {
			return nil
		}
		n, err := syscall.Kevent(kq, nil, out, &timeout)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			return err
		}
		if n > 0 {
			pending = true
			// Re-open watches: an atomic replace invalidates the old file fd.
			rearm()
			continue
		}
		// Timeout with a pending change: debounce window elapsed, fire once.
		if pending {
			pending = false
			onChange()
		}
	}
}
