package tmux

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type PtyFactory interface {
	Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error)
	Close()
}

// Pty starts a "real" pseudo-terminal (PTY) using the creack/pty package.
type Pty struct{}

func (pt Pty) Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, nil, err
	}

	// Start a goroutine to reap the process when it exits
	// This prevents zombie processes from accumulating
	go func() {
		_ = cmd.Wait() // Reap the process
	}()

	return ptmx, cmd, nil
}

func (pt Pty) Close() {}

func MakePtyFactory() PtyFactory {
	return Pty{}
}
