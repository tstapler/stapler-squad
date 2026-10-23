package tymux

import (
	"context"
	"fmt"
	"os"
	"slices"

	"connectrpc.com/connect"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/session/launchcmd"
)

// capStderrRedirect is the GetServerInfo capability for CreateSessionRequest.stderr_path.
const capStderrRedirect = "create_session.stderr_redirect"

const (
	shellPath  = "/bin/sh"
	shellFlagC = "-c"
)

// applyLaunch fills req's Command/Args/Env/StderrPath from cmd. A nil cmd leaves
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
	// stdin has no daemon-side redirect yet, and an old daemon would drop stderr_path silently.
	if !plain || c.RequiresShell() || c.StdinFile != "" || (c.StderrAppend != "" && !s.serverSupports(ctx, capStderrRedirect)) {
		viaShell(req, c.ShellString())
		return nil
	}
	args, err := c.Args(os.ReadFile)
	if err != nil {
		return fmt.Errorf("tymux: resolve launch arguments: %w", err)
	}
	req.Command, req.Args, req.StderrPath = program, args, c.StderrAppend
	for _, e := range c.Env {
		req.Env[e.Key()] = e.Value
	}
	return nil
}

func viaShell(req *v1.CreateSessionRequest, script string) {
	req.Command, req.Args = shellPath, []string{shellFlagC, script}
}

// serverSupports reports whether the running daemon advertises capability; a
// daemon too old to answer GetServerInfo advertises none.
func (s *tymuxGRPCSession) serverSupports(ctx context.Context, capability string) bool {
	resp, err := s.transport.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{}))
	return err == nil && slices.Contains(resp.Msg.GetCapabilities(), capability)
}
