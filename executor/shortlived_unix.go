//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
)

// applyProcessGroup sets Setpgid: true on cmd.SysProcAttr, placing the child
// in a new process group. This ensures that when a signal is sent to the
// process group (via kill(-pgid, sig)), grandchildren spawned by the child
// are also signalled, preventing orphaned processes.
//
// If cmd.SysProcAttr is already non-nil (e.g. from applyRlimits), the existing
// SysProcAttr is reused and Setpgid is merged in. This avoids overwriting
// Pdeathsig or other fields already set.
func applyProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}
