package tymux

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"connectrpc.com/connect"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/session/launchcmd"
)

// capStderrRedirect is the GetServerInfo capability for CreateSessionRequest.stderr_path.
const capStderrRedirect = "create_session.stderr_redirect"

// capStdinFile is the GetServerInfo capability for CreateSessionRequest.stdin_path.
const capStdinFile = "create_session.stdin_file"

const (
	shellPath  = "/bin/sh"
	shellFlagC = "-c"
)

// applyLaunch fills req's Command/Args/Env/StderrPath/StdinPath from cmd. A nil cmd leaves
// them empty so tymuxd runs $SHELL. Only commands that need a shell run as
// `/bin/sh -c`; everything else is exec'd directly with no quoting involved.
func (s *tymuxGRPCSession) applyLaunch(ctx context.Context, req *v1.CreateSessionRequest, cmd launchcmd.LaunchCommand) error {
	switch c := cmd.(type) {
	case nil:
		return nil
	case launchcmd.ShellScript:
		viaShell(req, c.ShellString())
		return nil
	case launchcmd.ExecCommand:
		return s.applyExec(ctx, req, c)
	default:
		return fmt.Errorf("tymux: unsupported launch command %T", cmd)
	}
}

func (s *tymuxGRPCSession) applyExec(ctx context.Context, req *v1.CreateSessionRequest, c launchcmd.ExecCommand) error {
	program, plain := c.Program()
	if !plain || c.RequiresShell() {
		viaShell(req, c.ShellString())
		return nil
	}
	// An old daemon silently drops stderr_path/stdin_path, so each redirect needs its capability.
	supports := s.capabilityProbe(ctx)
	if c.StderrAppend != "" && !supports(capStderrRedirect) {
		viaShell(req, c.ShellString())
		return nil
	}
	// The daemon rejects a relative stdin_path, so only absolute paths go to it.
	daemonStdin := c.StdinFile != "" && filepath.IsAbs(c.StdinFile) && supports(capStdinFile)
	if c.StdinFile != "" && !daemonStdin {
		viaShell(req, c.ShellString())
		return nil
	}
	args, err := c.Args(os.ReadFile)
	if err != nil {
		return fmt.Errorf("tymux: resolve launch arguments: %w", err)
	}
	req.Command, req.Args, req.StderrPath, req.StdinPath = program, args, c.StderrAppend, c.StdinFile
	for _, e := range c.Env {
		req.Env[e.Key()] = e.Value
	}
	return nil
}

func viaShell(req *v1.CreateSessionRequest, script string) {
	req.Command, req.Args = shellPath, []string{shellFlagC, script}
}

// capabilityProbe returns a predicate backed by at most one GetServerInfo call;
// a daemon too old to answer advertises nothing.
func (s *tymuxGRPCSession) capabilityProbe(ctx context.Context) func(capability string) bool {
	var caps []string
	var fetched bool
	return func(capability string) bool {
		if !fetched {
			fetched = true
			if resp, err := s.transport.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{})); err == nil {
				caps = resp.Msg.GetCapabilities()
			}
		}
		return slices.Contains(caps, capability)
	}
}
