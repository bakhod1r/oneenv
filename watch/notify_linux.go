//go:build linux

package watch

import (
	"context"
	"path/filepath"
	"syscall"
	"unsafe"
)

// inotifyEvents are the conditions we react to. We watch the parent directories
// so that atomic saves (write to a temp file, then rename over the target) are
// caught: CREATE/MOVED_TO fire on the directory when the new file appears, while
// MODIFY covers in-place writes.
const inotifyEvents = syscall.IN_MODIFY | syscall.IN_CREATE | syscall.IN_MOVED_TO |
	syscall.IN_MOVED_FROM | syscall.IN_DELETE | syscall.IN_ATTRIB

// notify watches the parent directory of each file with inotify and calls
// onChange whenever a watched file changes. It returns when ctx is cancelled;
// cancellation closes the inotify fd, which unblocks the blocking Read.
func notify(ctx context.Context, files []string, onChange func()) error {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		return err
	}

	// Map each watched directory's watch descriptor to the set of file base
	// names we care about in it.
	watched := make(map[int32]map[string]struct{})
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			abs = f
		}
		dir, name := filepath.Dir(abs), filepath.Base(abs)
		wd, err := syscall.InotifyAddWatch(fd, dir, inotifyEvents)
		if err != nil {
			continue
		}
		if watched[int32(wd)] == nil {
			watched[int32(wd)] = make(map[string]struct{})
		}
		watched[int32(wd)][name] = struct{}{}
	}

	// Close the fd on cancellation to unblock the Read below.
	go func() {
		<-ctx.Done()
		syscall.Close(fd)
	}()

	var buf [4096]byte
	for {
		n, err := syscall.Read(fd, buf[:])
		if err != nil {
			return nil // fd closed on ctx cancellation, or a fatal read error
		}
		if changedWatchedFile(buf[:n], watched) {
			onChange()
		}
	}
}

// changedWatchedFile reports whether any inotify_event in buf targets one of the
// watched file names under its watch descriptor.
func changedWatchedFile(buf []byte, watched map[int32]map[string]struct{}) bool {
	const hdr = syscall.SizeofInotifyEvent
	for offset := 0; offset+hdr <= len(buf); {
		ev := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[offset]))
		names := watched[ev.Wd]
		if ev.Len > 0 && names != nil {
			nameBytes := buf[offset+hdr : offset+hdr+int(ev.Len)]
			// The name is NUL-padded; trim at the first NUL.
			if i := indexByte(nameBytes, 0); i >= 0 {
				nameBytes = nameBytes[:i]
			}
			if _, ok := names[string(nameBytes)]; ok {
				return true
			}
		}
		offset += hdr + int(ev.Len)
	}
	return false
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}
