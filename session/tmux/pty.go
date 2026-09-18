package tmux

import (
	"context"
	"io"
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

// PtySession abstracts a live, resizable, bidirectional pseudo-terminal connection --
// the shared shape both a local raw-PTY attach (*os.File, via session's localPTYSession)
// and a remote SSH-backed attach (session/tmux/ssh_runner.go's sshPtySession, Task
// 4.4.1b) present to callers that only need to read/write terminal bytes and resize.
// It is deliberately NOT what PtyFactory.Start/StartWithSize return (*os.File) --
// see PtyFactory's doc comment below for why that interface itself is untouched.
type PtySession interface {
	io.ReadWriteCloser
	// Resize changes the PTY's terminal dimensions -- pty.Setsize locally,
	// ssh.Session.WindowChange remotely (ssh-remote-workspaces Task 4.4.1e).
	Resize(cols, rows int) error
}

// RemotePtyFactory creates a PtySession on a remote host over SSH -- the
// counterpart to PtyFactory for the raw (non-control-mode) PTY-attach path
// used by server/services/session_service.go's StreamTerminal raw-PTY
// fallback (ssh-remote-workspaces Phase 4, Task 4.4.1a). It is a new,
// additive interface rather than a retrofit of PtyFactory itself:
// PtyFactory.Start/StartWithSize's *os.File return type is baked into
// TmuxSession's ptmx field and dozens of call sites across tmux.go and
// control_mode.go (pty.Setsize, Fd()-based liveness checks, os.File-typed
// struct fields protected by ptmxMu) -- retrofitting that surface to an
// interface is out of scope for this epic's blast radius and would risk the
// local-only streaming paths this project must leave unmodified. See
// session/tmux/ssh_runner.go's SSHPtyFactory for the concrete
// RequestPty+Start-based implementation.
type RemotePtyFactory interface {
	// StartPty opens a new SSH channel on the remote host, requests a PTY of
	// the given initial size, and starts name/args running attached to it
	// (mirroring PtyFactory.StartWithSize's "size set before the child is
	// forked" contract -- including reusing its *pty.Winsize parameter type,
	// rather than adjacent cols/rows ints, for the same reason -- so a
	// remote tmux attach-session never sees a 0x0 terminal either). dir is
	// the remote working directory ("" for the login default), matching
	// CommandRunner's dir semantics.
	StartPty(ctx context.Context, ws *pty.Winsize, dir, name string, args ...string) (PtySession, error)
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
