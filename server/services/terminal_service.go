package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// TerminalService handles GetTerminalSnapshot and WriteToSession RPCs.
// Both methods share the same instance-lookup fallback chain (poller →
// external discovery → not-found error), which is factored into the private
// findInstance helper below. Extracted from SessionService per ADR-001 (both
// methods individually exceed the 30-line threshold and are cohesive as a pair).
type TerminalService struct {
	// poller and extDiscovery are wired after construction via SetPoller /
	// SetExternalDiscovery, forwarded from SessionService's own setters.
	poller       *session.ReviewQueuePoller
	extDiscovery *session.ExternalSessionDiscovery
}

// NewTerminalService creates a TerminalService. Wire poller and externalDiscovery
// after construction via SetPoller and SetExternalDiscovery.
func NewTerminalService() *TerminalService {
	return &TerminalService{}
}

// SetPoller wires the live-instance poller for instance lookup.
func (ts *TerminalService) SetPoller(p *session.ReviewQueuePoller) {
	ts.poller = p
}

// SetExternalDiscovery wires the external session discovery (mux-enabled sessions).
func (ts *TerminalService) SetExternalDiscovery(d *session.ExternalSessionDiscovery) {
	ts.extDiscovery = d
}

// findInstance implements the shared poller → external-discovery fallback chain
// used by both GetTerminalSnapshot and WriteToSession.
func (ts *TerminalService) findInstance(id string) *session.Instance {
	if ts.poller != nil {
		if inst := ts.poller.FindInstance(id); inst != nil {
			return inst
		}
	}
	if ts.extDiscovery != nil {
		if inst := ts.extDiscovery.GetSession(id); inst != nil {
			return inst
		}
	}
	return nil
}

// GetTerminalSnapshot returns the last N lines of terminal output for a session.
// Uses inst.Preview() for a read-only snapshot without requiring an active stream.
func (ts *TerminalService) GetTerminalSnapshot(
	ctx context.Context,
	req *connect.Request[sessionv1.GetTerminalSnapshotRequest],
) (*connect.Response[sessionv1.GetTerminalSnapshotResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	// Find the live instance via the poller (avoids loadInstancesWithWiring side effects)
	inst := ts.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	content, err := inst.Preview()
	if err != nil {
		// Non-fatal: return empty snapshot rather than error
		log.Warn("[GetTerminalSnapshot] preview failed", "session", req.Msg.SessionId, "err", err)
		content = ""
	}

	// Trim to last N lines
	lastN := int(req.Msg.LastNLines)
	if lastN <= 0 {
		lastN = 20
	}
	lines := strings.Split(content, "\n")
	if len(lines) > lastN {
		lines = lines[len(lines)-lastN:]
	}
	content = strings.Join(lines, "\n")

	return connect.NewResponse(&sessionv1.GetTerminalSnapshotResponse{
		Content: content,
		IsEmpty: strings.TrimSpace(content) == "",
	}), nil
}

// +api: session:log-client-events
// +api: session:write-to-session
// WriteToSession sends raw text input to a running session's PTY.
func (ts *TerminalService) WriteToSession(
	ctx context.Context,
	req *connect.Request[sessionv1.WriteToSessionRequest],
) (*connect.Response[sessionv1.WriteToSessionResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.Input == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("input is required"))
	}

	inst := ts.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	// BUG-047: must use session.EnterKeySequence ('\r'), not a bare '\n' —
	// the Claude Code CLI's raw-mode TUI only recognizes '\r' as submit, so a
	// trailing '\n' leaves the text sitting unsubmitted in the input buffer.
	text := session.BuildSubmittableInput(req.Msg.Input, req.Msg.PressEnter)

	errCh := make(chan error, 1)
	go func() { errCh <- inst.SendKeys(text) }()

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	select {
	case err := <-errCh:
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("send keys failed: %w", err))
		}
	case <-timeoutCtx.Done():
		return nil, connect.NewError(connect.CodeDeadlineExceeded, fmt.Errorf("timed out writing to session PTY"))
	}

	return connect.NewResponse(&sessionv1.WriteToSessionResponse{Success: true}), nil
}
