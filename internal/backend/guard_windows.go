//go:build windows

package backend

import "os/exec"

// Plan A stub. Real Windows support via Job Objects will land in Plan B.
func attachChildGuards(cmd *exec.Cmd) {
	_ = cmd
}
