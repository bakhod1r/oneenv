//go:build windows

package watch

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// Win32 constants for directory-change notifications.
const (
	fileListDirectory       = 0x0001
	fileShareAll            = 0x0001 | 0x0002 | 0x0004 // read | write | delete
	openExisting            = 3
	fileFlagBackupSemantics = 0x02000000

	notifyChangeFileName   = 0x00000001
	notifyChangeDirName    = 0x00000002
	notifyChangeAttributes = 0x00000004
	notifyChangeSize       = 0x00000008
	notifyChangeLastWrite  = 0x00000010

	notifyFilter = notifyChangeFileName | notifyChangeDirName |
		notifyChangeAttributes | notifyChangeSize | notifyChangeLastWrite
)

var (
	modkernel32               = syscall.NewLazyDLL("kernel32.dll")
	procReadDirectoryChangesW = modkernel32.NewProc("ReadDirectoryChangesW")
)

// fileNotifyInformation mirrors the Win32 FILE_NOTIFY_INFORMATION header; the
// variable-length file name follows it in the buffer.
type fileNotifyInformation struct {
	NextEntryOffset uint32
	Action          uint32
	FileNameLength  uint32
	// FileName [1]uint16 follows here in the raw buffer.
}

// notify watches the parent directory of each file with ReadDirectoryChangesW
// and calls onChange whenever a watched file changes. Each directory is read in
// its own goroutine with a blocking call; ctx cancellation closes the handles,
// which unblocks the reads and ends the goroutines. It returns when ctx is done.
func notify(ctx context.Context, files []string, onChange func()) error {
	// Group watched file basenames by their parent directory.
	byDir := make(map[string]map[string]struct{})
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			abs = f
		}
		dir, name := filepath.Dir(abs), filepath.Base(abs)
		if byDir[dir] == nil {
			byDir[dir] = make(map[string]struct{})
		}
		byDir[dir][strings.ToLower(name)] = struct{}{}
	}

	var (
		wg      sync.WaitGroup
		handles []syscall.Handle
	)
	for dir, names := range byDir {
		p, err := syscall.UTF16PtrFromString(dir)
		if err != nil {
			continue
		}
		h, err := syscall.CreateFile(p, fileListDirectory, fileShareAll, nil,
			openExisting, fileFlagBackupSemantics, 0)
		if err != nil {
			continue
		}
		handles = append(handles, h)
		wg.Add(1)
		go func(h syscall.Handle, names map[string]struct{}) {
			defer wg.Done()
			watchDir(h, names, onChange)
		}(h, names)
	}

	<-ctx.Done()
	// Closing the handles makes the pending ReadDirectoryChangesW calls fail,
	// unblocking and ending the watcher goroutines.
	for _, h := range handles {
		syscall.CloseHandle(h)
	}
	wg.Wait()
	return nil
}

// watchDir blocks reading change notifications for one directory, invoking
// onChange whenever a change touches one of the watched file names. It returns
// when the handle is closed.
func watchDir(h syscall.Handle, names map[string]struct{}, onChange func()) {
	var buf [4096]byte
	for {
		var bytesReturned uint32
		r, _, _ := procReadDirectoryChangesW.Call(
			uintptr(h),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			0, // do not watch subtree
			uintptr(notifyFilter),
			uintptr(unsafe.Pointer(&bytesReturned)),
			0, // no overlapped
			0, // no completion routine
		)
		if r == 0 {
			return // handle closed or error: stop watching
		}
		if changedWatchedFile(buf[:], names) {
			onChange()
		}
	}
}

// changedWatchedFile reports whether any FILE_NOTIFY_INFORMATION entry in buf
// names one of the watched files (case-insensitively).
func changedWatchedFile(buf []byte, names map[string]struct{}) bool {
	for offset := 0; offset < len(buf); {
		info := (*fileNotifyInformation)(unsafe.Pointer(&buf[offset]))
		nameStart := offset + int(unsafe.Sizeof(*info))
		nameLen := int(info.FileNameLength) / 2 // bytes -> UTF-16 units
		if nameStart+nameLen*2 <= len(buf) {
			name := syscall.UTF16ToString(
				unsafe.Slice((*uint16)(unsafe.Pointer(&buf[nameStart])), nameLen),
			)
			if _, ok := names[strings.ToLower(name)]; ok {
				return true
			}
		}
		if info.NextEntryOffset == 0 {
			break
		}
		offset += int(info.NextEntryOffset)
	}
	return false
}
