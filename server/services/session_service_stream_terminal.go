package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"

	"connectrpc.com/connect"
)

// fallbackPTYCols/Rows seed a remote raw-PTY session's initial size in
// StreamTerminal's IsRemote() branch (Task 4.4.1d): unlike
// streamViaControlMode's CurrentPaneRequest handshake, no client dimensions
// flow through StreamTerminal's first message, so this is a reasonable
// starting size until the client's first TerminalData_Resize arrives.
const (
	fallbackPTYCols = 80
	fallbackPTYRows = 24
)

// StreamTerminal provides bidirectional streaming for terminal I/O.
// Implements bidirectional streaming where:
// - Client sends: terminal input and resize events
// - Server sends: raw terminal output
//
// NOTE: browser clients never reach this method directly — the WebSocket
// handler (connectrpc_websocket.go) intercepts StreamTerminal calls made
// over its custom websocket transport before they reach here. This handler
// exists to satisfy the ConnectRPC service interface and could be used by
// non-browser gRPC/Connect clients.
//
//nolint:gocognit,gocyclo,funlen // pre-existing complexity relocated verbatim by the session_service.go split (sdd:fix-hotspot, 2026-09-12); reducing it is a separate follow-up, not a file move
func (s *SessionService) StreamTerminal(
	ctx context.Context,
	stream *connect.BidiStream[sessionv1.TerminalData, sessionv1.TerminalData],
) error {
	// Get the first message to determine which session to attach to
	initialMsg, err := stream.Receive()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to receive initial message: %w", err))
	}

	if initialMsg == nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("no initial message received"))
	}

	if initialMsg.SessionId == "" {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	// Get the session instance - CRITICAL: Use the poller's instance to ensure
	// timestamp updates are visible to the review queue. Loading fresh from storage
	// creates a separate object that the poller never sees.
	var instance *session.Instance
	if s.reviewQueuePoller != nil {
		instance = s.reviewQueuePoller.FindInstance(initialMsg.SessionId)
	}

	// Fallback to storage if poller doesn't have it (shouldn't happen normally)
	if instance == nil {
		log.Warn("[StreamTerminal] instance not found in poller, loading from storage (timestamps may desync)", "session", initialMsg.SessionId)
		instances, err := s.loadInstancesWithWiring()
		if err != nil {
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
		}
		for _, inst := range instances {
			if inst.MatchesID(initialMsg.SessionId) {
				instance = inst
				break
			}
		}
	}

	if instance == nil {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", initialMsg.SessionId))
	}

	// Verify session is started and not paused
	if !instance.Started() {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session not started"))
	}

	if instance.Paused() {
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("session is paused"))
	}

	// Create context for managing goroutines. Created here (rather than just
	// above goroutine 1, as before Task 4.4.1d) so the remote branch below
	// can pass it to GetPTYSession.
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Acquire this handler's terminal data source. IsRemote() is the single
	// mechanism this branch is gated on (architecture-review.md Blocker 1)
	// -- never a type switch on ExecutionTarget/Instance fields.
	//
	// Local (unchanged from pre-4.4.1d): GetPTYReader() + dupPTYFile.
	//
	// Remote (ssh-remote-workspaces Phase 4, Task 4.4.1d): a fresh SSH-backed
	// PTY attached to the same remote tmux session via GetPTYSession, so this
	// fallback path streams remote terminal bytes in the same TerminalData
	// shape a local session produces -- differing only in transport
	// underneath, per Story 4.4.1's acceptance criteria. fallbackPTYCols/Rows
	// seed its initial size: unlike streamViaControlMode's handshake, no
	// client dimensions flow through StreamTerminal's first message, and a
	// resize arrives moments later via the TerminalData_Resize case below.
	var readFile *os.File         // set only for a local session; exact pre-4.4.1d behavior
	var remotePTY tmux.PtySession // set only for a remote session
	if instance.IsRemote() {
		remotePTY, err = instance.GetPTYSession(streamCtx, fallbackPTYCols, fallbackPTYRows)
		if err != nil {
			log.Error("[StreamSession] failed to get remote PTY session", "session", instance.Title, "err", err)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get remote PTY session: %w", err))
		}
	} else {
		// Get PTY for reading terminal output
		ptyFile, ptyErr := instance.GetPTYReader()
		if ptyErr != nil {
			log.Error("[StreamSession] failed to get PTY reader", "session", instance.Title, "err", ptyErr)
			return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get PTY reader: %w", ptyErr))
		}

		// Duplicate the PTY fd for this goroutine's exclusive use. ptyFile is
		// shared with the instance's own internal consumers (response stream,
		// command executor), so calling SetReadDeadline directly on it would
		// mutate poll.FD state those other readers depend on. A dup'd fd gets
		// its own independent *os.File/poll.FD — closing or setting a deadline
		// on readFile has no effect on ptyFile or its other readers, since the
		// underlying open file description is only released once every fd
		// referencing it is closed. dupPTYFile is platform-specific
		// (dup_fd_unix.go / dup_fd_windows.go) since syscall.Dup isn't available
		// on Windows.
		readFile, err = dupPTYFile(ptyFile)
		if err != nil {
			return connect.NewError(connect.CodeInternal, err)
		}
	}

	// Channel for errors from goroutines
	errCh := make(chan error, 2)

	// wg tracks both goroutines below so the handler never returns (letting
	// Connect close the underlying stream) while either might still be
	// calling stream.Send/stream.Receive — doing so races with Connect's own
	// end-of-stream write. See BUG-025 follow-up: caught by -race under a
	// real PTY-backed StreamTerminal test.
	var wg sync.WaitGroup

	// sendMu serializes every stream.Send() call across the two goroutines
	// below. connect-go's BidiStream.Send() is documented as unsafe for
	// concurrent use from multiple goroutines: goroutine 1 continuously sends
	// PTY output while goroutine 2 can, on error, send an error message back
	// to the client (WRITE_ERROR / RESIZE_ERROR) — those two goroutines are
	// otherwise independent (one pumps PTY->client, the other pumps
	// client->PTY), so without a shared lock a PTY-output Send() and an
	// input-goroutine error-reply Send() can execute at the same instant on
	// the same stream. Caught by -race under a real PTY-backed StreamTerminal
	// test. Single-writer-via-channel was considered but would require
	// funneling ALL sends (including the hot PTY-output path) through an
	// extra hop; a mutex is the minimal change here since sends are already
	// synchronous, best-effort calls with no ordering requirements beyond
	// mutual exclusion.
	var sendMu sync.Mutex
	sendLocked := func(msg *sessionv1.TerminalData) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(msg)
	}

	// Flow control state for backpressure management
	// Reference: https://xtermjs.org/docs/guides/flowcontrol/
	pauseCh := make(chan bool, 1) // Buffered channel for pause/resume signals
	var ptyPaused bool            // Current PTY pause state

	// Goroutine 1: Read from PTY and send deltas to client (terminal output)
	wg.Add(1)
	if instance.IsRemote() {
		// Remote variant (Task 4.4.1d): reads from remotePTY (tmux.PtySession,
		// an SSH channel) instead of a dup'd local fd. ssh.Session's stdout has
		// no SetReadDeadline -- an io.Reader over an SSH channel doesn't
		// implement net.Conn's deadline interface -- so this cannot reuse the
		// local branch's poll-with-timeout structure below. Instead, a small
		// watcher goroutine closes remotePTY on streamCtx cancellation, which
		// unblocks the in-flight Read with an error (the same "no half-close,
		// Close tears down the whole channel" mechanism sshPtySession.Close()
		// documents) -- the SSH-idiomatic analog of the local deadline.
		go func() {
			defer wg.Done()
			defer remotePTY.Close()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic in output goroutine: %v", r)
				}
			}()

			go func() {
				<-streamCtx.Done()
				_ = remotePTY.Close()
			}()

			buf := make([]byte, 32*1024)
			for {
				// Block until unpaused rather than spinning.
				if ptyPaused {
					select {
					case <-streamCtx.Done():
						return
					case ptyPaused = <-pauseCh:
						if !ptyPaused {
							log.Info("[FlowControl] PTY reading RESUMED", "session", initialMsg.SessionId)
						}
					}
					continue
				}

				select {
				case <-streamCtx.Done():
					return
				case paused := <-pauseCh:
					ptyPaused = paused
					if paused {
						log.Info("[FlowControl] PTY reading PAUSED", "session", initialMsg.SessionId)
					}
					continue
				default:
				}

				n, readErr := remotePTY.Read(buf)
				if n > 0 {
					instance.UpdateTerminalTimestamps(string(buf[:n]), true)

					select {
					case <-streamCtx.Done():
						return
					default:
					}

					outputMsg := &sessionv1.TerminalData{
						SessionId: initialMsg.SessionId,
						Data: &sessionv1.TerminalData_Output{
							Output: &sessionv1.TerminalOutput{
								Data: buf[:n],
							},
						},
					}
					if sendErr := sendLocked(outputMsg); sendErr != nil {
						errCh <- fmt.Errorf("failed to send output: %w", sendErr)
						return
					}
				}

				if readErr != nil {
					select {
					case <-streamCtx.Done():
						// Expected: streamCtx cancellation closed remotePTY to unblock Read.
						return
					default:
					}
					if readErr.Error() != "EOF" {
						errCh <- fmt.Errorf("remote PTY read error: %w", readErr)
					}
					return
				}
			}
		}()
	} else {
		go func() {
			defer wg.Done()
			defer readFile.Close() // our own dup'd fd; does not affect ptyFile or its other readers
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic in output goroutine: %v", r)
				}
			}()

			buf := make([]byte, 32*1024)
			for {
				// Block until unpaused rather than spinning.
				if ptyPaused {
					select {
					case <-streamCtx.Done():
						return
					case ptyPaused = <-pauseCh:
						if !ptyPaused {
							log.Info("[FlowControl] PTY reading RESUMED", "session", initialMsg.SessionId)
						}
					}
					continue
				}

				select {
				case <-streamCtx.Done():
					return
				case paused := <-pauseCh:
					ptyPaused = paused
					if paused {
						log.Info("[FlowControl] PTY reading PAUSED", "session", initialMsg.SessionId)
					}
				default:
					// A short deadline on our own dup'd fd (see readFile above)
					// bounds how long Read can block, so this goroutine notices
					// streamCtx cancellation promptly instead of potentially
					// blocking until the next real PTY output — which could
					// arrive well after the handler has returned and Connect has
					// closed the stream. Safe to set here because readFile is
					// exclusively ours; it does not touch ptyFile's poll.FD.
					_ = readFile.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
					n, readErr := readFile.Read(buf)
					if n > 0 {
						// Update terminal activity timestamps with the output content
						// This ensures LastMeaningfulOutput reflects web UI viewing activity
						instance.UpdateTerminalTimestamps(string(buf[:n]), true)

						select {
						case <-streamCtx.Done():
							return
						default:
						}

						outputMsg := &sessionv1.TerminalData{
							SessionId: initialMsg.SessionId,
							Data: &sessionv1.TerminalData_Output{
								Output: &sessionv1.TerminalOutput{
									Data: buf[:n],
								},
							},
						}
						if sendErr := sendLocked(outputMsg); sendErr != nil {
							errCh <- fmt.Errorf("failed to send output: %w", sendErr)
							return
						}
					}

					if readErr != nil {
						if netErr, ok := readErr.(interface{ Timeout() bool }); ok && netErr.Timeout() {
							// Expected: the deadline above elapsed with no data.
							// Loop back around to re-check streamCtx/pauseCh.
							continue
						}
						// EOF or other read error
						if readErr.Error() != "EOF" {
							errCh <- fmt.Errorf("PTY read error: %w", readErr)
						}
						return
					}
				}
			}
		}()
	}

	// Goroutine 2: Receive from client and forward to PTY (terminal input + resize)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic in input goroutine: %v", r)
			}
		}()

		for {
			select {
			case <-streamCtx.Done():
				return
			default:
				msg, receiveErr := stream.Receive()
				if receiveErr != nil {
					// Check if this is a normal EOF (client closed connection)
					// ConnectRPC returns io.EOF or various "stream ended" errors
					errStr := receiveErr.Error()
					if receiveErr == context.Canceled ||
						receiveErr == context.DeadlineExceeded ||
						errStr == "EOF" ||
						errStr == "stream ended" ||
						strings.Contains(errStr, "stream closed") ||
						strings.Contains(errStr, "connection closed") {
						// Client closed gracefully, exit without error
						return
					}
					// Other errors should be reported
					errCh <- fmt.Errorf("stream receive error: %w", receiveErr)
					return
				}

				if msg == nil {
					// Stream ended cleanly
					return
				}

				switch data := msg.Data.(type) {
				case *sessionv1.TerminalData_Input:
					// Update terminal activity timestamps with user input
					// This ensures LastMeaningfulOutput reflects user interaction via web UI
					instance.UpdateTerminalTimestamps(string(data.Input.Data), true)

					// Forward input to the terminal data source. remotePTY is
					// the only live write target for a remote session --
					// instance.WriteToPTY() routes to TmuxSession.SendKeys(),
					// which writes to t.lockedPTMX() (the LOCAL raw-attach
					// PTY, never populated for a remote session), so it must
					// never be called on this branch (Task 4.4.1d fix: the
					// pre-fix code called it unconditionally, which returned
					// "PTY not initialized" for a remote session's very first
					// keystroke and tore the stream down).
					var writeErr error
					if remotePTY != nil {
						_, writeErr = remotePTY.Write(data.Input.Data)
					} else {
						_, writeErr = instance.WriteToPTY(data.Input.Data)
					}
					if writeErr != nil {
						// Send error back to client
						errorMsg := &sessionv1.TerminalData{
							SessionId: msg.SessionId,
							Data: &sessionv1.TerminalData_Error{
								Error: &sessionv1.TerminalError{
									Message: fmt.Sprintf("Failed to write to PTY: %v", writeErr),
									Code:    "WRITE_ERROR",
								},
							},
						}
						_ = sendLocked(errorMsg) // Best effort
						errCh <- writeErr
						return
					}

					// Publish user interaction event for immediate review queue reactivity
					s.eventBus.Publish(events.NewUserInteractionEvent(
						msg.SessionId,
						"terminal_input",
						"", // No additional context needed
					))

				case *sessionv1.TerminalData_Resize:
					// Handle terminal resize
					cols := int(data.Resize.Cols)
					rows := int(data.Resize.Rows)

					if resizeErr := instance.ResizePTY(cols, rows); resizeErr != nil {
						// Send error back to client
						errorMsg := &sessionv1.TerminalData{
							SessionId: msg.SessionId,
							Data: &sessionv1.TerminalData_Error{
								Error: &sessionv1.TerminalError{
									Message: fmt.Sprintf("Failed to resize terminal: %v", resizeErr),
									Code:    "RESIZE_ERROR",
								},
							},
						}
						_ = sendLocked(errorMsg) // Best effort
						// Don't return on resize errors, they're not fatal
					} else {
						// instance.ResizePTY resizes the tmux window/pane (remote-transparent
						// via the tmux CM/subprocess resize-window command); remotePTY.Resize
						// additionally issues this raw-PTY session's own SSH window-change
						// request (Task 4.4.1e), since this session is a separate SSH channel
						// from the one control-mode/tmux commands travel over and has no other
						// way to learn its PTY dimensions changed.
						if remotePTY != nil {
							if err := remotePTY.Resize(cols, rows); err != nil {
								log.Warn("failed to resize remote PTY session", "cols", cols, "rows", rows, "session", msg.SessionId, "err", err)
							}
						}
						log.Info("resized terminal", "cols", cols, "rows", rows, "session", msg.SessionId)
					}

				case *sessionv1.TerminalData_FlowControl:
					// Handle flow control signals from client
					// Reference: https://xtermjs.org/docs/guides/flowcontrol/
					if data.FlowControl.Paused {
						log.Info("[FlowControl] client requested PAUSE", "watermark_bytes", data.FlowControl.Watermark, "session", msg.SessionId)
						// Signal PTY reading goroutine to pause
						select {
						case pauseCh <- true:
						default:
							// Channel already has pause signal, skip
						}
					} else {
						log.Info("[FlowControl] client requested RESUME", "watermark_bytes", data.FlowControl.Watermark, "session", msg.SessionId)
						// Signal PTY reading goroutine to resume
						select {
						case pauseCh <- false:
						default:
							// Channel already has resume signal, skip
						}
					}

				case *sessionv1.TerminalData_CurrentPaneRequest:
					// NOTE: This handler is currently unused - browser clients use the WebSocket handler
					// (connectrpc_websocket.go) which intercepts streaming calls before they reach here.
					// This handler exists to satisfy the protobuf interface contract and could be used
					// by non-browser gRPC clients in the future.
					//
					// If this handler becomes active, the CurrentPaneRequest resize logic is implemented
					// in connectrpc_websocket.go:524-550 and should be synchronized here.
					log.Warn("[StreamTerminal] CurrentPaneRequest received (unexpected - WebSocket handler should intercept this)")

				case *sessionv1.TerminalData_Error:
					// Client sent an error, log it
					log.Error("client error", "message", data.Error.Message, "code", data.Error.Code)
				}
			}
		}
	}()

	// Wait for either context cancellation or error, then wait for both
	// goroutines to actually stop before returning. Returning early lets
	// Connect close the underlying HTTP/2 stream (write trailers/end-stream);
	// if either goroutine is still mid-Send/Receive when that happens, the
	// concurrent writes to the same connection race — caught by -race even
	// with sendLocked in place, because sendLocked only serializes OUR two
	// goroutines against each other, not against Connect's own teardown write
	// once this function returns. Goroutine 1 is reliably bounded (its dup'd
	// fd's 250ms read deadline means it notices streamCtx.Done() promptly
	// regardless of PTY activity). Goroutine 2's stream.Receive() has no
	// equivalent deadline (connect-go's BidiStream doesn't expose one), so it
	// can genuinely still be blocked here — an earlier version of this code
	// gave up waiting after a short timeout and returned anyway, which is
	// exactly what let the race happen. logSlowShutdown never gives up: it
	// blocks until wg is actually done (so the race is structurally
	// impossible), merely logging if that's taking unusually long so a client
	// that never disconnects is still visible in logs rather than silently
	// leaking the goroutine.
	const shutdownWarnAfter = 2 * time.Second
	select {
	case <-streamCtx.Done():
		log.Info("StreamTerminal: context done", "session", initialMsg.SessionId)
		logSlowShutdown(&wg, shutdownWarnAfter, initialMsg.SessionId, "context done")
		return nil // Clean shutdown
	case err := <-errCh:
		log.Error("StreamTerminal error", "session", initialMsg.SessionId, "err", err)
		cancel() // streamCtx.Done() wasn't otherwise closed on this path; signal both goroutines to stop.
		logSlowShutdown(&wg, shutdownWarnAfter, initialMsg.SessionId, "error")
		return connect.NewError(connect.CodeInternal, err)
	}
}

// waitWithTimeout waits for wg to complete, returning true if it did so
// within timeout and false if the timeout elapsed first. On timeout, this
// bookkeeping goroutine itself is harmlessly leaked (it will eventually
// complete and close the now-unread done channel) — but the caller's own
// tracked goroutines may still be running and may still touch shared state
// (e.g. a stream) after this function returns false. Callers on the false
// path must treat that as a real, logged condition, not a no-op.
//
// StreamTerminal itself does NOT use this — see logSlowShutdown below for why
// "give up and return anyway" is unsafe there. Kept for its own direct test
// coverage (TestWaitWithTimeout) and as a building block other bounded-wait
// callers can use where returning on timeout doesn't race a shared resource.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// logSlowShutdown blocks until wg completes — unconditionally, with no
// give-up. StreamTerminal's two goroutines both call stream.Send()/Receive();
// once this function's caller returns, Connect writes the stream's
// end-of-stream trailers on the same connection, which races with either
// goroutine if it's still mid-Send/Receive. The only way to make that
// structurally impossible is to never return while wg is incomplete — unlike
// waitWithTimeout, this cannot give up and let the caller proceed anyway.
//
// warnAfter only controls a one-time log line so a client that never
// disconnects (holding goroutine 2's stream.Receive() open indefinitely,
// since connect-go's BidiStream has no per-call read deadline to bound it)
// is visible in logs as a real leak, rather than either silently hanging
// forever unnoticed or racing the stream teardown.
func logSlowShutdown(wg *sync.WaitGroup, warnAfter time.Duration, sessionID, reason string) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(warnAfter):
		log.Warn("StreamTerminal: goroutines still running past warn threshold, waiting for them before returning (not giving up, to avoid racing Connect's stream teardown)",
			"session", sessionID, "reason", reason)
		<-done
	}
}
