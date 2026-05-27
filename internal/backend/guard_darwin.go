//go:build darwin

package backend

import (
	"os/exec"
	"syscall"
)

func attachChildGuards(cmd *exec.Cmd) {
	// macOS: no Pdeathsig. Just set process group so we can kill the tree.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
