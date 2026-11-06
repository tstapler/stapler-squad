//go:build windows

package clihelp

import (
	"context"
	"os/exec"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

func newProbeCmd(ctx context.Context, path ResolvedPath, args []string) *exec.Cmd {
	// G204: path is a Resolve/locate-validated absolute regular file; argv is the constant --help.
	return safeexec.CommandContextPG(ctx, string(path), args...) //nolint:gosec
}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
