//go:build !windows

// Package ipc provides the local inter-process transport used by the runed
// daemon. On macOS/Linux this is a UNIX domain socket; on Windows (Plan B)
// it will be a named pipe.
package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// Listen binds a unix domain socket at path with 0700 permissions.
//
// The parent directory is created (0700) if missing. Stale socket files
// (leftover from a crashed daemon) are unlinked and re-bound. If another
// process is actively listening on the same path, Listen returns an error.
func Listen(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir parent: %w", err)
	}

	if info, err := os.Stat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			// Not a socket — be conservative, remove so Listen can bind.
			if err := os.Remove(path); err != nil {
				return nil, fmt.Errorf("remove non-socket at %s: %w", path, err)
			}
		} else {
			// Existing socket — probe for live listener.
			if conn, derr := net.Dial("unix", path); derr == nil {
				conn.Close()
				return nil, fmt.Errorf("socket %s already in use", path)
			}
			// Stale: no listener. Remove and rebind below.
			_ = os.Remove(path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	lis, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		lis.Close()
		return nil, fmt.Errorf("chmod: %w", err)
	}
	return lis, nil
}
