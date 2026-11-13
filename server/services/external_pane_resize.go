package services

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ExternalPaneTarget is what an external-pane resize needs from a terminal attach.
type ExternalPaneTarget struct {
	Instance    *session.Instance
	Snap        *domain.InstanceSnapshot
	TmuxSession string
	SessionID   string
}

// resizeExternalCapturePaneSession best-effort resizes an external session's
// tmux window and pane via tmux commands, then verifies the result.
func resizeExternalCapturePaneSession(p ExternalPaneTarget, targetCols, targetRows int) {
	socket := p.Snap.TmuxServerSocket
	runResize := func(cmd string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		args := tmux.ResolveSocket(socket).Args(cmd, "-t", p.TmuxSession,
			"-x", fmt.Sprintf("%d", targetCols), "-y", fmt.Sprintf("%d", targetRows))
		return runTmuxGatedErr(ctx, socket, func() error {
			return safeexec.CommandContext(ctx, tmux.Binary(), args...).Run()
		})
	}
	if err := runResize("resize-window"); err != nil {
		log.Warn("[streamViaTmuxCapture] failed to resize tmux window for external session", "tmux_session", p.TmuxSession, "err", err)
	}
	if err := runResize("resize-pane"); err != nil {
		log.Warn("[streamViaTmuxCapture] failed to resize tmux pane for external session", "tmux_session", p.TmuxSession, "err", err)
	}

	actualCols, actualRows, verifyErr := p.Instance.GetPaneDimensions()
	if verifyErr != nil {
		log.Warn("[streamViaTmuxCapture] failed to verify external resize", "session", p.SessionID, "err", verifyErr)
	} else if actualCols != targetCols || actualRows != targetRows {
		log.Warn("[streamViaTmuxCapture] external dimension mismatch", "session", p.SessionID, "target_cols", targetCols, "target_rows", targetRows, "actual_cols", actualCols, "actual_rows", actualRows)
	} else {
		log.ForSession(p.SessionID).Debug("external resize verified", "cols", actualCols, "rows", actualRows)
	}
}

// runTmuxGatedErr acquires a tmux exec-gate slot for serverSocket (bounded by
// ctx), runs fn, then releases the slot. Used for resize-window/resize-pane
// calls against external (unmanaged) sessions — see session/tmux's runGated,
// which this mirrors, for the primary call-site pattern.
func runTmuxGatedErr(ctx context.Context, serverSocket string, fn func() error) error {
	release, err := tmux.AcquireExecSlot(ctx, serverSocket)
	if err != nil {
		return fmt.Errorf("exec gate: %w", err)
	}
	defer release()
	return fn()
}
