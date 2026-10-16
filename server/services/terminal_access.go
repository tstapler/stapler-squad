package services

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// ErrReadOnly is returned when a terminal write is requested for a hidden
// (background) session. Handlers map it to connect.CodeFailedPrecondition.
var ErrReadOnly = errors.New("session is read-only")

// TerminalAccess is parsed once, at the stream boundary, from the session's
// hidden flag (ADR-005). The zero value is ReadOnly so an unset access fails closed.
type TerminalAccess uint8

const (
	// TerminalReadOnly: no input, no resize, no scroll forwarding.
	TerminalReadOnly TerminalAccess = iota
	// TerminalReadWrite: a visible session.
	TerminalReadWrite
)

// AccessFor decides access from the lock-free snapshot only
// (.claude/rules/instance-lock-free-reads.md). It is the constructor every
// stream site uses; the unary guards (PR 5u) add their own.
func AccessFor(inst *session.Instance) TerminalAccess {
	if inst == nil || inst.Snapshot().Hidden {
		return TerminalReadOnly
	}
	return TerminalReadWrite
}

// GuardsFlag is the live hidden_session_readonly_guards value, injected into
// the unary handlers (never a bool a caller could pass as true). A nil
// GuardsFlag means the guards are on: only an explicit "off" ever loosens them.
type GuardsFlag interface {
	GuardsEnabled() bool
}

// UnaryGuardsFlag is the atomic behind GuardsFlag. The zero value (and a nil
// pointer) is "on", so an unreadable config keeps the guards on.
type UnaryGuardsFlag struct{ off atomic.Bool }

// GuardsEnabled reports whether hidden targets are refused by the unary handlers.
func (f *UnaryGuardsFlag) GuardsEnabled() bool { return f == nil || !f.off.Load() }

// SetEnabled flips the flag; false lets the unary handlers accept hidden targets.
func (f *UnaryGuardsFlag) SetEnabled(on bool) { f.off.Store(!on) }

// AccessForUnary is the constructor of the Story 5.2 unary handlers
// (WriteToSession, UpdateSession, RestartSession, RestartShell, SpawnShell, SwitchWorkspace).
// It differs from AccessFor only in the live flag: a hidden target is
// ReadWrite while the flag is off. The pinned-caller test fails if any other
// function, a stream site included, calls it.
func AccessForUnary(inst *session.Instance, flag GuardsFlag) TerminalAccess {
	if inst == nil {
		return TerminalReadOnly
	}
	if !inst.Snapshot().Hidden {
		return TerminalReadWrite
	}
	if flag != nil && !flag.GuardsEnabled() {
		return TerminalReadWrite
	}
	return TerminalReadOnly
}

// Writer returns the capability that every UI-stream pane write goes through,
// or ErrReadOnly. A nil TerminalWriter means "this attach cannot write": stream
// handlers hold the writer, never the TerminalAccess or a bool, so an ignored
// check cannot compile into a write.
func (a TerminalAccess) Writer(sender tmuxInputSender) (TerminalWriter, error) {
	if a != TerminalReadWrite {
		return nil, ErrReadOnly
	}
	if sender == nil {
		sender = realTmuxInputSender{}
	}
	return &paneWriter{sender: sender}, nil
}

// controlModeInput is what a control-mode attach exposes for keystrokes;
// *session.Instance and a shell's tmux session both satisfy it.
type controlModeInput interface {
	SendInputViaControlMode(ctx context.Context, data []byte) error
}

// PaneInput describes one keystroke delivery: control mode first, then the
// tmux subprocess seam when control mode fails.
type PaneInput struct {
	ControlMode controlModeInput
	TmuxSocket  string
	TmuxSession string
	// LogPrefix identifies the stream path in log lines, e.g. "[streamViaHub]".
	LogPrefix string
	Data      []byte
}

// TerminalWriter is the sealed capability for UI-stream pane writes.
type TerminalWriter interface {
	// SendInput types data into the pane. Errors are logged, not returned to the
	// stream: keystrokes may be lost under load but the stream stays alive.
	SendInput(ctx context.Context, in PaneInput) error
	// Sender is the tmux subprocess seam for the capture-pane and fallback routes.
	Sender() tmuxInputSender
}

// tmuxInputSender is the test seam in front of the tmux send-keys and
// resize-window/resize-pane subprocesses. Production wires realTmuxInputSender;
// a test injects a fake per handler, so a read-only attach is observable as
// zero calls instead of an exec the test cannot see.
type tmuxInputSender interface {
	SendInput(serverSocket, tmuxSession string, data []byte) error
	Resize(p capturePaneStreamParams, cols, rows int)
}

// realTmuxInputSender wraps the subprocess helpers unchanged.
type realTmuxInputSender struct{}

func (realTmuxInputSender) SendInput(serverSocket, tmuxSession string, data []byte) error {
	return sendInputToTmuxWithRetry(serverSocket, tmuxSession, data)
}

func (realTmuxInputSender) Resize(p capturePaneStreamParams, cols, rows int) {
	resizeExternalCapturePaneSession(p, cols, rows)
}

// paneWriter is the only TerminalWriter implementation. It is constructed only
// by TerminalAccess.Writer (guard check (a)).
type paneWriter struct {
	sender tmuxInputSender
}

func (w *paneWriter) Sender() tmuxInputSender { return w.sender }

func (w *paneWriter) SendInput(ctx context.Context, in PaneInput) error {
	err := in.ControlMode.SendInputViaControlMode(ctx, in.Data)
	if err == nil {
		return nil
	}
	log.Warn(in.LogPrefix+" CM input failed, retrying via subprocess", "session", in.TmuxSession, "err", err)
	if fbErr := w.sender.SendInput(in.TmuxSocket, in.TmuxSession, in.Data); fbErr != nil {
		log.Error(in.LogPrefix+" subprocess fallback also failed", "session", in.TmuxSession, "err", fbErr)
		return fbErr
	}
	return nil
}
