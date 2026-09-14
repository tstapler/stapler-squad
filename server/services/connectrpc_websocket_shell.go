package services

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/resize"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
)

// shellResizeReq is one coalesced client resize request for a shell tab's
// streamShellViaControlMode connection.
type shellResizeReq struct{ cols, rows int }

// shellStreamParams bundles the per-connection state
// streamShellViaControlMode's extracted goroutines need — a shell-tab analog
// of controlModeOutputForwarderParams/controlModeResizeCoalescerParams. Kept
// separate from the main-terminal params (rather than adding an optional
// ShellId to those) because a shell stream targets its own sibling tmux
// session (shellSess) directly, not the parent Instance's PTY.
type shellStreamParams struct {
	stream               *connectWebSocketStream
	instance             *session.Instance
	shellSess            *tmux.TmuxSession
	cursorTarget         shellPanePTY
	sessionID            string
	shellID              string
	shellTmuxSessionName string
	doneChan             chan struct{}
	errChan              chan error
	quiescenceCh         chan struct{}
	resizeCh             chan shellResizeReq
	resizeSettling       *atomic.Bool
}

// shellOutputForwarderParams bundles the state forwardShellControlModeOutput
// needs — the shell-tab analog of controlModeOutputForwarderParams.
type shellOutputForwarderParams struct {
	stream          *connectWebSocketStream
	sessionID       string
	shellID         string
	updateChan      <-chan []byte
	doneChan        chan struct{}
	errChan         chan error
	quiescenceCh    chan struct{}
	forwardingReady *atomic.Bool
	resizeSettling  *atomic.Bool
}

// forwardShellControlModeOutput is streamShellViaControlMode's output-forwarding
// goroutine — the shell-tab analog of forwardControlModeOutput. It omits the
// escape-analytics tap and exit-content send that the main-terminal version
// has, since neither applies to a shell tab's control-mode output.
func forwardShellControlModeOutput(p shellOutputForwarderParams) {
	defer close(p.doneChan)

	for {
		select {
		case <-p.doneChan:
			return
		case data, ok := <-p.updateChan:
			if !ok {
				return
			}

			if !p.forwardingReady.Load() || p.resizeSettling.Load() {
				// Either still settling from the initial resize nudge, or a live
				// resize reflow is in flight — see streamViaControlMode's identical
				// check for the full rationale. Drop it, but still count it toward
				// quiescence below.
				signalQuiescence(p.quiescenceCh)
				continue
			}

			cbp := coalesceBufPool.Get().(*[]byte)
			buf := coalesceAvailableFrames(append((*cbp)[:0], data...), p.updateChan)

			sendErr := sendShellControlModeOutput(p.stream, p.sessionID, p.shellID, buf)
			*cbp = buf[:0]
			coalesceBufPool.Put(cbp)
			if sendErr != nil {
				log.Error("[streamShellViaControlMode] failed to send output", "err", sendErr)
				p.errChan <- fmt.Errorf("failed to send output: %w", sendErr)
				return
			}
			signalQuiescence(p.quiescenceCh)
		}
	}
}

// sendShellControlModeOutput is sendControlModeOutput's shell-tab analog —
// see its doc comment.
func sendShellControlModeOutput(stream *connectWebSocketStream, sessionID, shellID string, data []byte) error {
	msg := terminalDataPool.Get().(*sessionv1.TerminalData)
	msg.SessionId = sessionID
	msg.ShellId = shellID
	msg.Data = &sessionv1.TerminalData_Output{
		Output: &sessionv1.TerminalOutput{Data: data},
	}
	err := marshalProtoEnvelope(stream, 0, msg)
	proto.Reset(msg)
	terminalDataPool.Put(msg)
	return err
}

// runShellControlModeResizeCoalescer is streamShellViaControlMode's resize
// coalescer — the shell-tab analog of runControlModeResizeCoalescer, using
// p.shellSess (the shell's own sibling tmux session) in place of an Instance.
func (h *ConnectRPCWebSocketHandler) runShellControlModeResizeCoalescer(p shellStreamParams) {
	var last shellResizeReq
	var lastAppliedAt time.Time
	for {
		select {
		case <-p.doneChan:
			return
		case r := <-p.resizeCh:
			if r == last && time.Since(lastAppliedAt) < 50*time.Millisecond {
				continue
			}
			if h.applyOneShellResize(p, r) {
				last, lastAppliedAt = r, time.Now()
			}
		}
	}
}

// applyOneShellResize is applyOneControlModeResize's shell-tab analog: resizes
// shellSess to r, waits for the reflow to settle, and sends the client the
// pre/post ResizeQuiescence signals plus a fresh post-resize snapshot.
func (h *ConnectRPCWebSocketHandler) applyOneShellResize(p shellStreamParams, r shellResizeReq) bool {
	// Suppress live forwarding for the duration of the reflow — see
	// streamViaControlMode's identical gate for the full rationale. Cleared on
	// every exit path, including early failure below.
	p.resizeSettling.Store(true)
	resizeDone := func() { p.resizeSettling.Store(false) }

	if err := p.shellSess.SetWindowSize(r.cols, r.rows); err != nil {
		log.Error("[streamShellViaControlMode] failed to resize", "err", err)
		resizeDone()
		return false
	}

	sendShellResizeQuiescence(p.stream, p.sessionID, p.shellID, r, true)

	const quiescenceDeadline = 300 * time.Millisecond
	quiescenceStart := time.Now()
	waitForQuiescence(p.quiescenceCh, quiescenceDeadline, 100*time.Millisecond)
	if elapsed := time.Since(quiescenceStart); elapsed >= quiescenceDeadline-5*time.Millisecond {
		log.Error("[streamShellViaControlMode] quiescence timed out, sending snapshot anyway", "elapsed", elapsed.Round(time.Millisecond), "session", p.sessionID, "shell", p.shellID, "cols", r.cols, "rows", r.rows)
	}

	sendShellPostResizeSnapshot(p)

	// Re-enable live forwarding now that the authoritative post-resize snapshot
	// has been sent — must happen before the client-facing Resizing:false
	// signal below, not after, or a live frame arriving in between would be
	// forwarded while the client still thinks it's mid-reflow.
	resizeDone()
	sendShellResizeQuiescence(p.stream, p.sessionID, p.shellID, r, false)
	return true
}

// sendShellPostResizeSnapshot captures and sends a fresh pane snapshot at the
// shell's new dimensions, mirroring sendPostResizeSnapshot for the main
// terminal — see its doc comment for the cursor-sync rationale.
func sendShellPostResizeSnapshot(p shellStreamParams) {
	snapContent, snapErr := p.shellSess.CapturePaneContentRaw()
	if snapErr != nil || snapContent == "" {
		return
	}
	fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(streamhub.RawPaneContent(snapContent)), p.cursorTarget)
	snapMsg := &sessionv1.TerminalData{
		SessionId: p.sessionID,
		ShellId:   p.shellID,
		Data: &sessionv1.TerminalData_Output{
			Output: &sessionv1.TerminalOutput{Data: []byte(fullContent)},
		},
	}
	if snapBytes, merr := proto.Marshal(snapMsg); merr != nil {
		log.Error("[streamShellViaControlMode] failed to marshal post-resize snapshot", "session", p.sessionID, "shell", p.shellID, "err", merr)
	} else {
		_ = p.stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, snapBytes))
	}
}

// sendShellResizeQuiescence is sendControlModeResizeQuiescence's shell-tab
// analog — see its doc comment.
func sendShellResizeQuiescence(stream *connectWebSocketStream, sessionID, shellID string, r shellResizeReq, resizing bool) {
	rqMsg := &sessionv1.TerminalData{
		SessionId: sessionID,
		ShellId:   shellID,
		Data: &sessionv1.TerminalData_ResizeQuiescence{
			ResizeQuiescence: &sessionv1.ResizeQuiescence{
				Resizing: resizing,
				// #nosec G115 -- r.cols originated as a proto int32 field
				// (TerminalResize.Cols) narrowed to int only for local
				// arithmetic; converting back to int32 cannot overflow.
				Cols: int32(r.cols),
				// #nosec G115 -- see Cols above; same reasoning for Rows.
				Rows: int32(r.rows),
			},
		},
	}
	if rqBytes, merr := proto.Marshal(rqMsg); merr != nil {
		log.Error("[streamShellViaControlMode] failed to marshal ResizeQuiescence", "session", sessionID, "shell", shellID, "err", merr)
	} else {
		_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, rqBytes))
	}
}

// runShellInputReadLoop is streamShellViaControlMode's WebSocket input-read
// loop — a shell-tab analog of runInputReadLoop, kept as a hand-duplicated
// copy (not a call to runInputReadLoop) because that shared loop's
// ScrollbackRequest/CurrentPaneRequest response builders don't carry a
// ShellId field; unifying them would need that plumbing threaded through the
// main-terminal path too, which is out of scope for this pass.
func runShellInputReadLoop(p shellStreamParams) {
	for {
		select {
		case <-p.doneChan:
			return
		default:
			if done := readOneShellFrame(p); done {
				return
			}
		}
	}
}

// readOneShellFrame reads and dispatches one WebSocket frame for
// runShellInputReadLoop. Returns true if the read loop should stop.
func readOneShellFrame(p shellStreamParams) bool {
	message, stop := readShellWebSocketMessage(p)
	if stop {
		return true
	}

	incomingData, skip, stop := parseInputFrameOrStop(p.errChan, "[streamShellViaControlMode]", message)
	if stop {
		return true
	}
	if skip {
		return false
	}

	if input := incomingData.GetInput(); input != nil {
		handleShellInput(p, input.Data)
	}
	if resize := incomingData.GetResize(); resize != nil {
		dispatchShellResize(p, int(resize.Cols), int(resize.Rows))
	}
	if scrollbackReq := incomingData.GetScrollbackRequest(); scrollbackReq != nil {
		handleShellScrollbackRequest(p, scrollbackReq)
	}
	if paneReq := incomingData.GetCurrentPaneRequest(); paneReq != nil {
		handleShellMidStreamCurrentPaneRequest(p, paneReq)
	}
	return false
}

// readShellWebSocketMessage reads one raw WebSocket message for
// readOneShellFrame. stop=true means the caller should stop the read loop —
// the error (nil for a normal client-initiated close) has already been
// pushed to p.errChan.
func readShellWebSocketMessage(p shellStreamParams) (message []byte, stop bool) {
	_, message, err := p.stream.conn.ReadMessage()
	if err == nil {
		return message, false
	}
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		p.errChan <- nil
	} else {
		log.Error("[streamShellViaControlMode] WebSocket read error", "session", p.sessionID, "shell", p.shellID, "err", err)
		p.errChan <- err
	}
	return nil, true
}

// handleShellInput forwards one input frame to the shell's tmux session via
// control mode, falling back to a subprocess send-keys on failure.
func handleShellInput(p shellStreamParams, data []byte) {
	if !p.instance.Permissions.CanSendCommand {
		log.Warn("[streamShellViaControlMode] send permission denied", "session", p.sessionID, "shell", p.shellID)
		return
	}
	p.instance.UpdateTerminalTimestamps(string(data), true)

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
	sendErr := p.shellSess.SendInputViaControlMode(sendCtx, data)
	sendCancel()
	if sendErr != nil {
		log.Warn("[streamShellViaControlMode] CM input failed, retrying via subprocess", "session", p.shellTmuxSessionName, "err", sendErr)
		if fbErr := sendInputToTmux(p.instance.Snapshot().TmuxServerSocket, p.shellTmuxSessionName, data); fbErr != nil {
			log.Error("[streamShellViaControlMode] subprocess fallback also failed", "session", p.shellTmuxSessionName, "err", fbErr)
		}
	}
}

// dispatchShellResize pushes a resize request to the coalescing worker, so
// rapid window-drag events never stall input reading and don't pile up
// unbounded goroutines.
func dispatchShellResize(p shellStreamParams, cols, rows int) {
	dispatchResizeRequest(p.resizeCh, shellResizeReq{cols, rows})
}

// handleShellScrollbackRequest is handleScrollbackRequest's shell-tab analog
// (see runShellInputReadLoop's doc comment for why the surrounding read loop
// isn't unified with runInputReadLoop) — response building itself is shared
// via buildScrollbackResponse.
func handleShellScrollbackRequest(p shellStreamParams, req *sessionv1.ScrollbackRequest) {
	const maxScrollbackLimit = 1000
	limit := int(req.Limit)
	if limit <= 0 || limit > maxScrollbackLimit {
		limit = maxScrollbackLimit
	}
	offset := req.FromSequence

	startLine := fmt.Sprintf("-%d", offset+uint64(limit))
	endLine := fmt.Sprintf("-%d", offset+1)
	content, sbErr := p.shellSess.CapturePaneContentWithOptions(startLine, endLine)
	if sbErr != nil {
		log.Warn("[streamShellViaControlMode] ScrollbackRequest tmux capture failed", "session", p.sessionID, "shell", p.shellID, "err", sbErr)
		return
	}

	sbResp := buildScrollbackResponse(p.sessionID, p.shellID, content, offset, limit)
	if respBytes, merr := proto.Marshal(sbResp); merr != nil {
		log.Error("[streamShellViaControlMode] failed to marshal scrollback response", "session", p.sessionID, "shell", p.shellID, "err", merr)
	} else {
		_ = p.stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, respBytes))
	}
}

// handleShellMidStreamCurrentPaneRequest answers a CurrentPaneRequest arriving
// mid-stream (e.g. a client-initiated resync), via the same shared
// handleCurrentPaneRequest helper streamViaControlMode and
// streamViaTmuxCapturePane use.
func handleShellMidStreamCurrentPaneRequest(p shellStreamParams, paneReq *sessionv1.CurrentPaneRequest) {
	p.resizeSettling.Store(true)
	defer p.resizeSettling.Store(false)

	paneCtx, paneSpan := telemetry.StartSpan(context.Background(), "terminal.mid_stream_current_pane_request")
	defer paneSpan.End()
	paneSpan.SetAttributes(
		attribute.String("session_id", p.sessionID),
		attribute.String("shell_id", p.shellID),
		attribute.String("resync_id", paneReq.GetResyncId()),
	)
	output, handleErr := handleCurrentPaneRequest(paneCtx, p.sessionID, p.cursorTarget, paneReq, currentResyncOptions())
	if handleErr != nil {
		paneSpan.RecordError(handleErr)
		log.Error("[streamShellViaControlMode] failed to handle mid-stream current pane request", "session", p.sessionID, "shell", p.shellID, "err", handleErr)
		return
	}
	writeCurrentPaneResponse(p.stream, p.sessionID, p.shellID, output)
}

// streamShellViaControlMode streams a shell tab through the same low-latency,
// event-driven tmux control-mode pipeline as streamViaControlMode, instead of
// the capture-pane polling path. It targets the shell's own sibling tmux
// session directly (shellSess) rather than the parent Instance's PTY, since
// the shell runs in its own tmux session sharing only the server socket.
func (h *ConnectRPCWebSocketHandler) streamShellViaControlMode(stream *connectWebSocketStream, instance *session.Instance, shellID, shellTmuxSessionName string) error {
	snap := instance.Snapshot()
	sessionID := snap.Title

	shellSess := tmux.NewTmuxSessionFromExistingWithServerSocket(shellTmuxSessionName, snap.TmuxServerSocket)
	cursorTarget := shellPanePTY{session: shellSess}

	log.Info("[streamShellViaControlMode] starting", "session", sessionID, "shell", shellID, "tmux", shellTmuxSessionName)

	instance.MarkViewed()

	var handshakeData sessionv1.TerminalData
	if err := proto.Unmarshal(stream.requestMsg, &handshakeData); err != nil {
		return fmt.Errorf("failed to parse handshake: %w", err)
	}
	currentPaneReq := handshakeData.GetCurrentPaneRequest()
	if currentPaneReq == nil {
		return fmt.Errorf("handshake missing CurrentPaneRequest - client may need update")
	}

	if !shellSess.DoesSessionExistNoCache() {
		return fmt.Errorf("shell tmux session missing: %s", shellTmuxSessionName)
	}

	if err := shellSess.StartControlMode(); err != nil {
		return fmt.Errorf("failed to start control mode for shell: %w", err)
	}
	defer func() {
		if err := shellSess.StopControlMode(); err != nil {
			log.Warn("[streamShellViaControlMode] StopControlMode error", "err", err)
		}
	}()

	// Subscribe and start the output-forwarding goroutine BEFORE the resize nudge below,
	// mirroring the fix in streamViaControlMode: quiescenceCh needs a real producer during
	// the initial handshake wait, not a fixed settle timer. Frames received before the
	// canonical initial snapshot has been sent are used only to drive quiescence detection
	// (forwardingReady gates actual delivery to the client).
	subscriberID, updateChan := shellSess.SubscribeToControlModeUpdates()
	defer shellSess.UnsubscribeFromControlModeUpdates(subscriberID)

	log.Info("[streamShellViaControlMode] subscribed to control mode", "subscriber_id", subscriberID, "session", sessionID, "shell", shellID)

	quiescenceCh := make(chan struct{}, 16)
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})
	var forwardingReady atomic.Bool
	// resizeSettling mirrors forwardingReady but for the live (post-connect) resize path
	// below — see the identical field in streamViaControlMode for the full rationale.
	var resizeSettling atomic.Bool

	go forwardShellControlModeOutput(shellOutputForwarderParams{
		stream:          stream,
		sessionID:       sessionID,
		shellID:         shellID,
		updateChan:      updateChan,
		doneChan:        doneChan,
		errChan:         errChan,
		quiescenceCh:    quiescenceCh,
		forwardingReady: &forwardingReady,
		resizeSettling:  &resizeSettling,
	})

	if currentPaneReq.TargetCols != nil && currentPaneReq.TargetRows != nil {
		targetCols := int(*currentPaneReq.TargetCols)
		targetRows := int(*currentPaneReq.TargetRows)

		log.Info("[streamShellViaControlMode] handshake dimensions, forcing redraw via nudge", "cols", targetCols, "rows", targetRows)

		if err := resize.WithForcedRedraw(shellSess.SetWindowSize, targetCols, targetRows); err != nil {
			log.Error("[streamShellViaControlMode] failed to resize", "err", err)
		} else {
			// Output-forwarding goroutine above is already subscribed, so quiescenceCh
			// receives real signals from redraw frames here.
			quiescenceStart := time.Now()
			waitForQuiescence(quiescenceCh, 500*time.Millisecond, 50*time.Millisecond)
			if elapsed := time.Since(quiescenceStart); elapsed >= 500*time.Millisecond-5*time.Millisecond {
				log.Warn("[streamShellViaControlMode] initial quiescence timed out; shell may be stalled", "elapsed", elapsed.Round(time.Millisecond), "session", sessionID, "shell", shellID)
			}
		}
	} else {
		log.Warn("[streamShellViaControlMode] handshake missing dimensions, layout may be incorrect")
	}

	initialContent, err := shellSess.CapturePaneContentRaw()
	if err != nil {
		log.Info("[streamShellViaControlMode] capture-pane failed, sending stopped notice", "session", sessionID, "shell", shellID, "err", err)
		initialContent = "\r\n\x1b[33m[shell stopped — no terminal content available]\x1b[0m\r\n"
	}

	if initialContent != "" {
		fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(streamhub.RawPaneContent(initialContent)), cursorTarget)

		terminalData := &sessionv1.TerminalData{
			SessionId: sessionID,
			ShellId:   shellID,
			Data: &sessionv1.TerminalData_Output{
				Output: &sessionv1.TerminalOutput{
					Data: []byte(fullContent),
				},
			},
		}
		dataBytes, err := proto.Marshal(terminalData)
		if err != nil {
			return fmt.Errorf("failed to marshal initial content: %w", err)
		}
		envelope := protocol.CreateEnvelope(0, dataBytes)
		if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
			return fmt.Errorf("failed to send initial content: %w", err)
		}
		log.Info("[streamShellViaControlMode] sent initial snapshot", "bytes", len(initialContent), "session", sessionID, "shell", shellID)
	}

	// The canonical initial snapshot has been sent; forward live updates from here on.
	forwardingReady.Store(true)

	resizeCh := make(chan shellResizeReq, 1)
	shellParams := shellStreamParams{
		stream:               stream,
		instance:             instance,
		shellSess:            shellSess,
		cursorTarget:         cursorTarget,
		sessionID:            sessionID,
		shellID:              shellID,
		shellTmuxSessionName: shellTmuxSessionName,
		doneChan:             doneChan,
		errChan:              errChan,
		quiescenceCh:         quiescenceCh,
		resizeCh:             resizeCh,
		resizeSettling:       &resizeSettling,
	}
	go h.runShellControlModeResizeCoalescer(shellParams)
	go runShellInputReadLoop(shellParams)

	select {
	case err := <-errChan:
		return err
	case <-doneChan:
		return nil
	}
}
