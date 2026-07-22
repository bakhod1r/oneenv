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
// onChange whenever a watched file changes. It returns when ctx is cancelled.
//
// The inotify fd is non-blocking and multiplexed with an eventfd through epoll:
// cancellation writes to the eventfd, which wakes the epoll wait. Closing the
// inotify fd would NOT unblock a thread already blocked in read(2), so the
// wakeup has to be an explicit readiness event.
func notify(ctx context.Context, files []string, onChange func()) error {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

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

	// eventfd used purely as a cancellation wakeup for the epoll wait.
	efd, err := eventfd()
	if err != nil {
		return err
	}
	defer syscall.Close(efd)

	epfd, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return err
	}
	defer syscall.Close(epfd)

	for _, watchFd := range []int{fd, efd} {
		ev := syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(watchFd)}
		if err := syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, watchFd, &ev); err != nil {
			return err
		}
	}

	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			var one [8]byte
			one[7] = 1
			_, _ = syscall.Write(efd, one[:])
		case <-stopped:
		}
	}()

	var buf [4096]byte
	var events [2]syscall.EpollEvent
	for {
		n, err := syscall.EpollWait(epfd, events[:], -1)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			return err
		}
		for _, ev := range events[:n] {
			if ev.Fd == int32(efd) {
				return nil // cancelled
			}
		}
		for {
			n, err := syscall.Read(fd, buf[:])
			if err != nil {
				break // EAGAIN: drained
			}
			if changedWatchedFile(buf[:n], watched) {
				onChange()
			}
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

// eventfd creates a non-blocking eventfd counter.
func eventfd() (int, error) {
	r0, _, errno := syscall.Syscall(syscall.SYS_EVENTFD2, 0,
		uintptr(syscall.O_CLOEXEC|syscall.O_NONBLOCK), 0)
	if errno != 0 {
		return 0, errno
	}
	return int(r0), nil
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
