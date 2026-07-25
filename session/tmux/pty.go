package tmux

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type PtyFactory interface {
	Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error)
	// StartWithSize starts cmd in a new PTY with the given terminal dimensions set before the
	// child process is forked. This prevents tmux from seeing a 0×0 terminal (which causes it
	// to immediately disconnect) when the parent process has no controlling terminal.
	StartWithSize(cmd *exec.Cmd, ws *pty.Winsize) (*os.File, *exec.Cmd, error)
	Close()
}

// Pty starts a "real" pseudo-terminal (PTY) using the creack/pty package.
type Pty struct{}

func (pt Pty) Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, err
	}
	return ptmx, cmd, nil
}

func (pt Pty) StartWithSize(cmd *exec.Cmd, ws *pty.Winsize) (*os.File, *exec.Cmd, error) {
	ptmx, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		return nil, nil, err
	}
	return ptmx, cmd, nil
}

func (pt Pty) Close() {}

func MakePtyFactory() PtyFactory {
	return Pty{}
}
