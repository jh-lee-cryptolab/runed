//go:build !windows

package spawn

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// fileLock is an advisory exclusive flock on a path. Use acquireLock to
// obtain, and call Release exactly once.
type fileLock struct {
	f *os.File
}

// acquireLock opens (or creates) the file at path and tries to obtain an
// exclusive flock, polling every 100ms until either successful or the
// timeout expires.
func acquireLock(path string, timeout time.Duration) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &fileLock{f: f}, nil
		}
		if !isWouldBlock(err) {
			f.Close()
			return nil, fmt.Errorf("flock %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("flock timeout after %s", timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// isWouldBlock returns true if err indicates the lock is held by another
// process. Plays nicely with both EAGAIN and EWOULDBLOCK on every Unix.
func isWouldBlock(err error) bool {
	return err == syscall.EWOULDBLOCK || err == syscall.EAGAIN
}

// Release unlocks and closes the underlying file. Safe to call once; any
// errors during unlock are swallowed (already-released or process exit
// invariably releases the OS lock).
func (l *fileLock) Release() {
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
}
