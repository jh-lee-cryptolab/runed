//go:build linux

package backend

import (
	"os/exec"
	"syscall"
)

func attachChildGuards(cmd *exec.Cmd) {
	// New process group for group-kill + SIGKILL on parent death (Linux only).
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
}
