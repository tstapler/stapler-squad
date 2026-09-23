//go:build !windows

package clihelp

import (
	"context"
	"os/exec"
	"syscall"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// newProbeCmd builds the command in its own session (Setsid implies a new
// process group with pgid==pid, and no controlling terminal). It replaces
// CommandContextPG's Setpgid to avoid the SIGTTIN case noted there.
func newProbeCmd(ctx context.Context, path ResolvedPath, args []string) *exec.Cmd {
	// G204: path is a Resolve/locate-validated absolute regular file; argv is the constant --help.
	cmd := safeexec.CommandContextPG(ctx, string(path), args...) //nolint:gosec
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

// killGroup SIGKILLs the child's whole process group; ESRCH is ignored.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
