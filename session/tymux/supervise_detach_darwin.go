package tymux

import (
	"os/exec"
	"syscall"
)

// detachFromSession starts tymuxd in its own session so a signal to squad's process group or
// controlling terminal (e.g. a service restart) cannot reach it; macOS has no Pdeathsig to rely on.
func detachFromSession(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}
