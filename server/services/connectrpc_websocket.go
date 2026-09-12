package services

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/gorilla/websocket"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/pkg/ansi"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/resize"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
)

// terminalDataPool reuses TerminalData proto objects in the stream hot path to avoid
// per-frame heap allocations. Reset via proto.Reset before putting back.
var terminalDataPool = sync.Pool{
	New: func() any { return &sessionv1.TerminalData{} },
}

// envelopeBufPool reuses wire-send buffers: [5-byte ConnectRPC header][serialized proto].
// gorilla/websocket.WriteMessage copies to the network before returning, so the buffer
// is safe to return to the pool immediately after the call.
var envelopeBufPool = sync.Pool{New: func() any { b := make([]byte, 0, 4096); return &b }}

// coalesceBufPool reuses coalesce buffers in the control-mode streaming loop.
// data from updateChan shares a broadcast backing array; we must copy before appending.
// marshalProtoEnvelope copies out of the coalesce buf before returning, so the buf
// is safe to return to the pool immediately after sendData returns.
var coalesceBufPool = sync.Pool{New: func() any { b := make([]byte, 0, 4096); return &b }}

// marshalProtoEnvelope serializes msg into a pooled buffer pre-padded with a 5-byte
// ConnectRPC envelope header, then writes it to stream in one call.
// Eliminates the separate proto.Marshal alloc and protocol.CreateEnvelope alloc on each frame.
func marshalProtoEnvelope(stream *connectWebSocketStream, flags byte, msg proto.Message) error {
	bp := envelopeBufPool.Get().(*[]byte)
	buf := append((*bp)[:0], 0, 0, 0, 0, 0) // reserve 5-byte header
	var err error
	buf, err = (proto.MarshalOptions{}).MarshalAppend(buf, msg)
	if err != nil {
		*bp = buf[:0]
		envelopeBufPool.Put(bp)
		return err
	}
	buf[0] = flags
	frameLen := len(buf) - 5
	if frameLen > math.MaxUint32 {
		// Mirrors server/protocol/envelope.go's CreateEnvelope: in practice
		// unreachable (no marshaled TerminalData/session-event frame this
		// server produces approaches 4 GiB), but truncating the length
		// header here would desync the receiver's framing, so fail loudly
		// via this function's existing error return rather than silently
		// corrupt the wire protocol.
		*bp = buf[:0]
		envelopeBufPool.Put(bp)
		return fmt.Errorf("marshalProtoEnvelope: frame too large to encode: %d bytes", frameLen)
	}
	// #nosec G115 -- bounds-checked above: the frameLen > math.MaxUint32 guard guarantees this fits in uint32
	binary.BigEndian.PutUint32(buf[1:5], uint32(frameLen))
	wsErr := stream.WriteMessage(websocket.BinaryMessage, buf)
	*bp = buf[:0]
	envelopeBufPool.Put(bp)
	return wsErr
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     isAllowedOrigin,
}

// isAllowedOrigin allows WebSocket upgrades from localhost and any HTTPS origin.
// Requests without an Origin header (e.g., non-browser clients, CLI tools) are allowed.
// Remote HTTPS access is secured by the auth middleware; the origin check here only
// blocks plaintext HTTP origins from non-localhost hosts.
func isAllowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	// Always allow localhost origins
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	// Allow any HTTPS origin — auth is enforced by the middleware layer
	return parsed.Scheme == "https"
}

// ConnectRPCWebSocketHandler handles ConnectRPC streaming calls over WebSocket
// Supports both managed sessions (with direct PTY access) and external sessions
// (discovered via mux socket monitoring, using tmux capture-pane for output)
// rePositionCodes matches ANSI escape sequences that are context-dependent and cause
// garbled rendering when tmux capture-pane output is replayed in a fresh xterm.js terminal.
// These sequences (absolute cursor positioning, screen clears, alternate-screen switches)
// assume a specific prior terminal state that doesn't exist on initial load.
// SGR color sequences (ESC[nm) are intentionally NOT matched and are preserved.
var rePositionCodes = regexp.MustCompile(
	`\x1b\[\d*;?\d*[Hf]` + // Absolute cursor: ESC[H, ESC[n;mH, ESC[n;mf
		`|\x1b\[\d*J` + // Screen clear: ESC[J, ESC[1J, ESC[2J, ESC[3J
		`|\x1b\[\?\d+[hl]` + // Private mode: ESC[?1049h (alt screen), ESC[?25l, etc.
		`|\x1b[78]` + // DEC save/restore cursor: ESC7, ESC8
		`|\x1b\[[su]`, // CSI save/restore cursor: ESC[s, ESC[u
)

// Terminal escape sequence building blocks used when prefixing snapshot content.
const (
	// ansiDECSTR issues a Soft Terminal Reset (DECSTR). Resets scroll region,
	// origin mode (DECOM), line-feed/newline mode (LNM), and other modal state
	// that TUI applications may have set via the live PTY stream.
	ansiDECSTR = "\x1b[!p"
	// ansiEraseScreen erases the visible screen (ED2). Does not touch scrollback.
	ansiEraseScreen = "\x1b[2J"
	// ansiCursorHome moves the cursor to the top-left (CUP 1;1). With DECOM off
	// (guaranteed by a preceding DECSTR) this is always the absolute screen origin.
	ansiCursorHome = "\x1b[H"
	// ansiSnapshotPrefix is prepended to every full-screen snapshot before it is
	// sent to the client. The sequence order matters:
	//   1. DECSTR — reset terminal modes so subsequent sequences are interpreted
	//               in a known default state (scroll region = full screen, etc.)
	//   2. ED2    — erase the now-full screen
	//   3. CUP    — position cursor at the absolute origin before writing content
	ansiSnapshotPrefix = ansiDECSTR + ansiEraseScreen + ansiCursorHome
)

// sanitizeInitialContent removes cursor-positioning and screen-control escape sequences
// from tmux capture-pane output before it is sent as the initial terminal snapshot.
// Without this, the captured content's absolute cursor positions conflict with the
// clear+home prefix we send, producing overlapping/garbled lines on first load.
// New output (streaming after initial load) is unaffected and renders correctly.
func sanitizeInitialContent(content string) string {
	return rePositionCodes.ReplaceAllString(content, "")
}

// prepareSnapshotContent normalizes bare \n to \r\n (xterm.js only moves the
// cursor down, not to column 0, on LF once DECSTR resets LNM off) and requires
// the non-"-J" RawPaneContent variant, since "-J" strips cursor codes and
// merges wrapped rows in a way that breaks redraw.
func prepareSnapshotContent(content streamhub.RawPaneContent) string {
	sanitized := sanitizeInitialContent(string(content))
	// Avoid creating \r\r\n from any pre-existing \r\n pairs.
	sanitized = strings.ReplaceAll(sanitized, "\r\n", "\n")
	return strings.ReplaceAll(sanitized, "\n", "\r\n")
}

// withCursorSync appends a CUP escape to content so xterm.js cursor lands at the
// same position as the tmux pane cursor after the snapshot is displayed. Without
// this, the xterm.js cursor is left wherever the last byte of snapshot content
// placed it, while tmux's cursor is at the running process's working position
// (e.g. inside an Ink TUI animation). The mismatch causes subsequent cursor-up
// sequences emitted by the process to rewind to the wrong lines — producing the
// "billowing" effect where each animation frame stacks below the previous one
// instead of overwriting it.
// cursorPositioner is satisfied by both *session.Instance and shellPanePTY, letting
// withCursorSync target either the main session's pane or a shell's sibling tmux pane.
type cursorPositioner interface {
	GetPaneCursorPosition() (x, y int, err error)
}

// withCursorSyncTimeout bounds how long withCursorSync waits for
// GetPaneCursorPosition before giving up on the cursor-sync suffix and
// returning content unchanged. GetPaneCursorPosition has no timeout of its
// own on its control-mode path (up to 3s, cmCtx) or its subprocess fallback
// (up to 5s just to acquire an exec-gate slot, execGateAcquireTimeout, then
// an unbounded subprocess call on top) — under a degraded control-mode
// connection this can comfortably exceed the frontend's 4s resync stall
// watchdog (useVisibilityResync.ts), which then force-disconnects and
// reconnects, which (StartControlMode/StopControlMode refcounting) tears
// down and restarts control mode — actively feeding the same degradation
// that made this call slow in the first place. Cursor-sync is cosmetic
// (a stale cursor position self-corrects on the next successful update);
// the resync it's attached to is not, so this is a strict trade in favor of
// the resync always meeting the client's deadline.
const withCursorSyncTimeout = 300 * time.Millisecond

// newTerminalOutputData wraps content in the TerminalData_Output frame sent for
// sessionID. Shared by the initial-snapshot send paths (hub, control mode, and
// tmux capture-pane), which all build this identical envelope around whatever
// content each has just prepared.
func newTerminalOutputData(sessionID string, content string) *sessionv1.TerminalData {
	return &sessionv1.TerminalData{
		SessionId: sessionID,
		Data: &sessionv1.TerminalData_Output{
			Output: &sessionv1.TerminalOutput{
				Data: []byte(content),
			},
		},
	}
}

// startupWaitTimeout bounds streamViaHub's wait for a concurrently-starting
// Instance to finish before attaching. Observed real-world gap was ~1.26s.
const startupWaitTimeout = 2 * time.Second

// waitForEvent blocks until bus delivers an event matching match, or timeout
// elapses. Generic on purpose — reuse for other "watch for a session
// transition" needs instead of adding another poll loop.
func waitForEvent(bus *events.EventBus, timeout time.Duration, match func(*events.Event) bool) bool {
	if bus == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ch, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID) // redundant with Subscribe's own ctx.Done() cleanup goroutine, but drops the wait for that goroutine to run
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			if match(ev) {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
}

// waitForInstanceStartedEvent waits for an EventSessionUpdated confirming
// instance.Started(), rather than polling it directly. Relies on every
// Start()-completion path publishing that event (server/dependencies.go's
// boot-time restart and reconcile loops were missing it — now fixed). Falls
// back to a direct Started() check on timeout or a nil bus.
func waitForInstanceStartedEvent(bus *events.EventBus, instance *session.Instance, timeout time.Duration) bool {
	if waitForEvent(bus, timeout, func(ev *events.Event) bool {
		return ev.Type == events.EventSessionUpdated && ev.Session != nil &&
			ev.Session.UUID == instance.UUID && ev.Session.Started()
	}) {
		return true
	}
	return instance.Started()
}

func withCursorSync(content string, target cursorPositioner) string {
	if target == nil {
		return content
	}
	type cursorResult struct {
		x, y int
		err  error
	}
	resultCh := make(chan cursorResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		x, y, err := target.GetPaneCursorPosition()
		resultCh <- cursorResult{x, y, err}
	}()
	select {
	case res := <-resultCh:
		// See streamhub's identically-shaped withCursorSync for why this wait
		// matters: it guarantees the goroutine has fully exited before this
		// function returns, not just handed off its result, so a test's
		// goleak.VerifyNone() immediately afterward never catches it
		// mid-teardown.
		<-done
		if res.err != nil {
			return content
		}
		// CUP is 1-based; tmux cursor coords are 0-based.
		return content + fmt.Sprintf("\x1b[%d;%dH", res.y+1, res.x+1)
	case <-time.After(withCursorSyncTimeout):
		log.Warn("withCursorSync: GetPaneCursorPosition exceeded timeout, skipping cursor sync", "timeout", withCursorSyncTimeout)
		return content
	}
}

// sessionSnapshot caches terminal capture-pane output per session.
// dirty is set true when new output arrives so the next connect gets a fresh capture.
type sessionSnapshot struct {
	content    string
	capturedAt time.Time
	dirty      bool // true when output has arrived since last capture
}

type ConnectRPCWebSocketHandler struct {
	sessionService    *SessionService
	scrollbackManager *scrollback.ScrollbackManager

	// External session support (for unified WebSocket streaming)
	externalDiscovery   *session.ExternalSessionDiscovery
	tmuxStreamerManager *session.ExternalTmuxStreamerManager

	// ponytail: xsync.Map replaces map+RWMutex — markSnapshotDirty called per terminal frame
	snapshotCache *xsync.Map[string, sessionSnapshot]

	// Observability only (no behavior change): tracks which streamViaControlMode
	// generation currently "owns" a given tmux session name. Nothing here
	// prevents two invocations from running concurrently against the same
	// session (e.g. a browser reconnect racing the old connection's deferred
	// StopControlMode, which a full server restart triggers for every open
	// terminal at once) — each independently resizes tmux and captures its
	// pane, and a client that receives a snapshot captured mid-resize by a
	// *different* generation sees garbled/overlapping content (see the ±1
	// nudge comment in streamViaControlMode for the single-generation version
	// of this same failure mode). See recordControlModeStreamStart's doc
	// comment for what gets logged and how to correlate a recurrence.
	activeControlModeStreams *xsync.Map[string, controlModeStreamGeneration]
	controlModeStreamCounter atomic.Int64
}

// controlModeStreamGeneration identifies one streamViaControlMode invocation
// for a tmux session, purely so overlapping invocations can be spotted in
// logs (see ConnectRPCWebSocketHandler.activeControlModeStreams).
type controlModeStreamGeneration struct {
	generation int64
	startedAt  time.Time
}

// NewConnectRPCWebSocketHandler creates a new ConnectRPC WebSocket handler
// tmuxStreamerManager is required for ALL sessions (managed and external) since they all use tmux capture-pane polling
func NewConnectRPCWebSocketHandler(sessionService *SessionService, scrollbackManager *scrollback.ScrollbackManager, tmuxStreamerManager *session.ExternalTmuxStreamerManager) *ConnectRPCWebSocketHandler {
	return &ConnectRPCWebSocketHandler{
		sessionService:           sessionService,
		scrollbackManager:        scrollbackManager,
		tmuxStreamerManager:      tmuxStreamerManager,
		snapshotCache:            xsync.NewMap[string, sessionSnapshot](),
		activeControlModeStreams: xsync.NewMap[string, controlModeStreamGeneration](),
	}
}

// recordControlModeStreamStart registers a new streamViaControlMode
// invocation for tmuxSessionName and returns its generation number plus a
// cleanup func the caller must defer. If a prior generation is still
// registered (its cleanup hasn't run yet), this logs a WARN naming both
// generations and how long the prior one has been running — that's the
// signal to grep for ("overlapping control-mode stream") when terminal
// output looks garbled/overlapping after the fact, since the exact moment of
// visual corruption is rarely caught live.
func (h *ConnectRPCWebSocketHandler) recordControlModeStreamStart(sessionID, tmuxSessionName string) (generation int64, done func()) {
	generation = h.controlModeStreamCounter.Add(1)
	if prior, loaded := h.activeControlModeStreams.Load(tmuxSessionName); loaded {
		log.Warn("[streamViaControlMode] overlapping control-mode stream detected for tmux session",
			"session", sessionID, "tmux", tmuxSessionName,
			"new_generation", generation, "prior_generation", prior.generation,
			"prior_running_for", time.Since(prior.startedAt).String())
	}
	h.activeControlModeStreams.Store(tmuxSessionName, controlModeStreamGeneration{generation: generation, startedAt: time.Now()})
	return generation, func() {
		// Only clear the entry if it's still ours — a newer overlapping
		// generation's entry must survive this (older) generation's cleanup.
		h.activeControlModeStreams.Compute(tmuxSessionName, func(cur controlModeStreamGeneration, loaded bool) (controlModeStreamGeneration, xsync.ComputeOp) {
			if loaded && cur.generation == generation {
				return controlModeStreamGeneration{}, xsync.DeleteOp
			}
			return cur, xsync.CancelOp
		})
	}
}

// hubRegistry is the process-wide registry handing out the one
// *streamhub.StreamHub per tmux session name (Task 2.2.2b), generalizing
// activeControlModeStreams's existing xsync.Map shape per plan.md's
// HubRegistry glossary entry. Story 3.1.1 will widen this into the full
// sticky per-session StreamPath resolver (StreamOwnershipLock-guarded); today
// it is a plain get-or-create, safe under concurrent callers because
// xsync.Map.LoadOrCompute runs valueFn at most once per key.
type hubRegistry struct {
	hubs *xsync.Map[string, *streamhub.StreamHub]
}

// HubRegistry is the single process-wide hub registry for the PathHubOwned
// branch (env-var-gated, default off — see useStreamHub). Not yet wired to
// StreamOwnershipLock (Epic 3.1) or hub-registry-consolidation observability
// (Epic 3.2); those land in later epics of this plan.
var HubRegistry = &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}

// GetOrCreate returns the StreamHub for sessionName, creating it — and
// starting its raw-output pump exactly once — if this is the first caller
// for that name. controller is only consulted on that first (winning) call;
// later callers for the same sessionName get the existing hub regardless of
// what controller they pass.
//
// Story 3.1.2: before ever creating a hub, GetOrCreate itself acquires
// sessionName's StreamOwnershipLock and asserts PathHubOwned via
// ResolveExpecting. This makes GetOrCreate safe to call independently of
// streamTerminal's own top-level Resolve()-gated routing decision — if a
// concurrent legacy StartControlMode call already won the resolution for
// this session, GetOrCreate refuses to create a competing hub and returns
// ErrOwnershipResolvedToOtherPath instead, so the caller (streamViaHub) can
// fall back to joining the legacy path explicitly rather than silently
// operating as a second, independent owner.
func (r *hubRegistry) GetOrCreate(sessionName string, controller streamhub.SessionController) (*streamhub.StreamHub, error) {
	// Holds the ownership lock for the full LoadOrCompute below (not just the
	// resolve step), so this genuinely blocks on — rather than races — a
	// concurrent Instance.StartControlMode call for the same session (Story 3.1.2).
	hub, err := r.loadOrCreateHubLocked(sessionName, controller)
	if err != nil {
		return nil, fmt.Errorf("hubRegistry.GetOrCreate: %w", err)
	}

	// Defense in depth: LoadOrCompute guarantees the constructor above runs at
	// most once per key, so ownerCount must always be 1 here (Epic 3.2).
	streamhub.OverlapInvariant(sessionName, 1)

	return hub, nil
}

func (r *hubRegistry) loadOrCreateHubLocked(sessionName string, controller streamhub.SessionController) (*streamhub.StreamHub, error) {
	var hub *streamhub.StreamHub
	err := streamhub.AcquireOwnershipLock(sessionName).AcquireAndResolveExpecting(true, streamhub.PathHubOwned, func() error {
		h, loaded := r.hubs.LoadOrCompute(sessionName, func() (*streamhub.StreamHub, bool) {
			return streamhub.NewStreamHub(sessionName, controller), false
		})
		if loaded {
			log.Debug("[hubRegistry] reusing existing StreamHub", "session", sessionName)
		}
		hub = h
		// TryStartPump's CAS is a no-op if a pump is already running, so this is
		// safe to call unconditionally — it also restarts the pump for a
		// reactivated hub whose previous pump already exited on full teardown.
		if hub.TryStartPump() {
			go pumpControlModeOutputIntoHub(hub, controller, sessionName)
		}
		return nil
	})
	return hub, err
}

// pumpControlModeResubscribeDelay bounds how fast pumpControlModeOutputIntoHub
// retries SubscribeControlModeUpdates after its subscription channel closes.
// Control mode's process can crash and restart mid-session (StartControlMode
// is refcounted and shared across connections; a crash closes every
// subscriber, per control_mode.go's exit handler) — this is the retry
// interval, not a one-shot timeout, since a genuinely abandoned session's
// hub tears itself down independently (0 subscribers, grace period) and
// takes this loop down with it via hub.State() below.
const pumpControlModeResubscribeDelay = 500 * time.Millisecond

// pumpControlModeOutputIntoHub is the single production feed from a
// session's raw control-mode output into its StreamHub (Task 2.2.2b's
// minimal wiring for a working PathHubOwned path): it runs exactly once per
// hub, since GetOrCreate only invokes it from the winning LoadOrCompute call,
// so N attached subscribers never each run their own duplicate subscription
// (the exact per-connection duplication this project's hub replaces).
//
// Resubscribes when the subscription channel closes, instead of exiting for
// good, as long as the hub itself hasn't torn down. Control mode's
// subscription channel closes both when a session genuinely ends AND when
// its control-mode process merely crashes/restarts (StartControlMode is
// refcounted; a mid-session crash-restart is a normal recoverable event, not
// a session end) — the one-shot version of this function treated both
// identically, so a single control-mode restart anywhere in a hub's
// lifetime permanently starved it of live output with no way to recover
// short of every subscriber detaching and a fresh reattach recreating the
// hub (2026-08-25 regression: reported as "no real-time feedback while
// typing, only updates after a resize" — resize still worked because
// handleCurrentPaneRequest captures fresh via a direct capture-pane
// subprocess call, entirely bypassing this pump). This was flagged as a
// known gap not acceptable to ship once the streamhub default flipped on;
// it shipped anyway — this closes it.
func pumpControlModeOutputIntoHub(hub *streamhub.StreamHub, controller streamhub.SessionController, sessionName string) {
	// Unconditional on every exit path (including a future panic) — this is
	// the other half of TryStartPump's CAS (session/streamhub/hub.go): a
	// caller must be able to tell "the pump that used to feed this hub is
	// truly gone" so a later reconnect can restart one instead of silently
	// reactivating a hub with nothing feeding it live output.
	defer hub.MarkPumpExited()
	// first skips the torn-down check on this goroutine's very first iteration:
	// GetOrCreate's caller reactivates the hub (HubTornDown -> HubActive) after
	// spawning this goroutine, so a State() read racing ahead of that would see
	// the stale prior-teardown state and exit immediately (2026-08-26 regression).
	for first := true; ; first = false {
		if !first && hub.State() == streamhub.HubTornDown {
			log.Info("streamhub raw-output pump exiting: hub torn down", "session", sessionName)
			return
		}

		// StartControlMode is a no-op if already running, and restarts a
		// crashed process — SubscribeControlModeUpdates alone can never recover
		// from a crash, it only listens on whatever already exists (2026-09-01
		// regression: a crash otherwise starved the hub of output forever).
		if err := controller.StartControlMode(); err != nil {
			log.Warn("streamhub raw-output pump: StartControlMode failed, will retry", "session", sessionName, "err", err)
		}

		_, updates := controller.SubscribeControlModeUpdates()
		drainControlModeUpdatesIntoHub(hub, updates)

		if hub.State() == streamhub.HubTornDown {
			log.Info("streamhub raw-output pump exiting: hub torn down", "session", sessionName)
			return
		}
		log.Warn("streamhub raw-output pump: control mode subscription closed, resubscribing", "session", sessionName)
		time.Sleep(pumpControlModeResubscribeDelay)
	}
}

// drainControlModeUpdatesIntoHub feeds updates into hub, coalescing every
// frame already available on the channel into the same batch window before
// flushing — mirrors the legacy per-connection coalesce loop's `select
// {...; default: break coalesce}` pattern so a burst doesn't always pay
// BatchWindow's full MaxBatchWindow ceiling latency.
func drainControlModeUpdatesIntoHub(hub *streamhub.StreamHub, updates <-chan []byte) {
	for data := range updates {
		hub.OnRawOutput(data)
		drainAlreadyAvailable(hub, updates)
		hub.TryFlush()
	}
}

// drainAlreadyAvailable consumes every frame immediately ready on updates
// (non-blocking) into hub, without waiting for more to arrive.
func drainAlreadyAvailable(hub *streamhub.StreamHub, updates <-chan []byte) {
	for {
		select {
		case more, ok := <-updates:
			if !ok {
				return
			}
			hub.OnRawOutput(more)
		default:
			return
		}
	}
}

// useStreamHub resolves the global stream-hub default (config.
// EffectiveStreamHubEnabled), re-read per connection — safe because
// StreamOwnershipLock.Resolve caches the first resolution per tmux session.
func useStreamHub() bool {
	return config.EffectiveStreamHubEnabled(config.LoadConfig())
}

// init wires streamhub's per-session canary override (Story 3.3.1) to this
// package's config access, so session/streamhub itself never needs to
// import package config — the same one-way-dependency shape ADR-003
// establishes for AcquireOwnershipLock. Re-reads config.LoadConfig() on
// every call rather than caching, matching GetFeatureFlag's existing
// re-read-every-time convention (a session's own override can be changed
// without restarting the process).
func init() {
	streamhub.SetSessionOverrideLookup(func(sessionName string) (bool, bool) {
		return config.LoadConfig().GetStreamHubSessionOverride(sessionName)
	})
}

// streamHubSessionKey computes the StreamOwnershipLock/HubRegistry key from a
// session's title and TmuxPrefix (empty meaning "staplersquad_").
// tmuxSessionNameForStreamPath and SetStreamHubSessionOverride must derive it
// identically — they used to duplicate this independently and diverged,
// silently no-opping every canary override (2026-09-01).
func streamHubSessionKey(title, tmuxPrefix string) string {
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_"
	}
	return tmux.NewSessionName(title, tmuxPrefix).String()
}

// tmuxSessionNameForStreamPath computes the tmux session name StreamPath
// resolution keys on for instance, mirroring streamViaHub's own derivation
// (session title + tmux prefix, default "staplersquad_"). Resolution must
// use this same name so a session's StreamOwnershipLock lookup here and its
// hub lookup inside streamViaHub (HubRegistry.GetOrCreate) agree.
func tmuxSessionNameForStreamPath(instance *session.Instance) string {
	snap := instance.Snapshot()
	return streamHubSessionKey(snap.Title, snap.TmuxPrefix)
}

// waitForQuiescence waits until no updates arrive for quietFor duration, or timeout elapses.
// Used after resize nudges to detect when the TUI has finished redrawing.
func waitForQuiescence(updates <-chan struct{}, timeout, quietFor time.Duration) {
	deadline := time.After(timeout)
	quiet := time.NewTimer(quietFor)
	defer quiet.Stop()
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return
			}
			drainAndResetTimer(quiet, quietFor)
		case <-quiet.C:
			return
		case <-deadline:
			return
		}
	}
}

// drainAndResetTimer stops t, draining its channel if it had already fired,
// then resets it to d — the standard safe-reset sequence for a live timer
// (see time.Timer.Reset's own doc comment).
func drainAndResetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// waitForPaneContent polls CapturePaneContentRaw while the instance is still in a
// transient state (i.e. a concurrent Instance.Resume() may be mid-restore), giving up
// immediately once the status settles into one that means capture will never succeed.
func waitForPaneContent(instance *session.Instance) (string, error) {
	const (
		pollInterval = 150 * time.Millisecond
		maxWait      = 5 * time.Second
	)
	deadline := time.Now().Add(maxWait)
	for {
		content, err := instance.CapturePaneContentRaw()
		if err == nil {
			return string(content), nil
		}
		switch instance.Snapshot().Status {
		case session.Paused, session.Stopped, session.Hibernated, session.Crashed:
			return "", err
		}
		if time.Now().After(deadline) {
			return "", err
		}
		time.Sleep(pollInterval)
	}
}

// markSnapshotDirty marks a session's snapshot as dirty so the next connect captures fresh content.
// Called on every terminal frame; xsync.Map.Compute is lock-free on the read path.
func (h *ConnectRPCWebSocketHandler) markSnapshotDirty(sessionID string) {
	h.snapshotCache.Compute(sessionID, func(snap sessionSnapshot, loaded bool) (sessionSnapshot, xsync.ComputeOp) {
		if !loaded {
			return snap, xsync.CancelOp
		}
		snap.dirty = true
		return snap, xsync.UpdateOp
	})
}

// invalidateSnapshot drops a session's cached snapshot so the next getOrRefreshSnapshot
// re-captures from tmux.
//
// This exists for callers that have just done something making the cached content stale
// by construction, rather than merely suspecting new output — specifically the ±1 resize
// nudge in streamViaControlMode, which repaints the TUI at the client's dimensions.
// markSnapshotDirty cannot cover that case: its only call sites live in the
// output-forwarding goroutine, which does not start until after the initial snapshot has
// already been captured and sent, so a nudge-triggered repaint would otherwise leave the
// cache "clean" and serve content captured at the pre-nudge dimensions.
func (h *ConnectRPCWebSocketHandler) invalidateSnapshot(sessionID string) {
	h.snapshotCache.Delete(sessionID)
}

// getOrRefreshSnapshot returns a cached snapshot if clean, otherwise calls captureFn to refresh.
func (h *ConnectRPCWebSocketHandler) getOrRefreshSnapshot(
	sessionID string,
	captureFn func() (string, error),
) (string, error) {
	if snap, ok := h.snapshotCache.Load(sessionID); ok && !snap.dirty {
		// Debug, not Info: this fires on every WebSocket connect (production) and
		// every benchmark iteration (BenchmarkSnapshotCacheHit/Miss run into the
		// hundreds of millions of iterations). At Info level this previously
		// produced tens of millions of log lines during `go test -bench`, which
		// blew past CI log size limits and failed the benchmark job.
		log.Debug("[SnapshotCache] serving cached snapshot", "session", sessionID, "bytes", len(snap.content), "age", time.Since(snap.capturedAt).Round(time.Millisecond))
		return snap.content, nil
	}

	content, err := captureFn()
	if err != nil {
		return "", fmt.Errorf("getOrRefreshSnapshot: %w", err)
	}

	h.snapshotCache.Store(sessionID, sessionSnapshot{
		content:    content,
		capturedAt: time.Now(),
		dirty:      false,
	})

	log.Debug("[SnapshotCache] refreshed snapshot", "session", sessionID, "bytes", len(content))
	return content, nil
}

// SetExternalSessionSupport configures external session discovery support
// This enables the handler to discover and stream external sessions (via mux socket monitoring)
// Note: tmuxStreamerManager is already set in constructor since ALL sessions use it
func (h *ConnectRPCWebSocketHandler) SetExternalSessionSupport(
	discovery *session.ExternalSessionDiscovery,
) {
	h.externalDiscovery = discovery
	log.Info("external session discovery enabled for ConnectRPC WebSocket handler")
}

// resolveSession looks up a session by ID, checking multiple sources in priority order:
// 1. ReviewQueuePoller (for managed sessions with fresh in-memory state)
// 2. Storage (for managed sessions persisted to disk)
// 3. ExternalDiscovery (for external sessions discovered via mux socket monitoring)
//
// Returns the instance and a boolean indicating if it's an external session.
// Returns nil, false if the session is not found in any source.
func (h *ConnectRPCWebSocketHandler) resolveSession(sessionID string) (*session.Instance, bool) {
	// Priority 1: Check ReviewQueuePoller for managed sessions (fresh in-memory state)
	// CRITICAL: Always check poller first - it has the live in-memory instances with active PTYs
	// Fallback to storage would call LoadInstances() which RESTARTS all sessions!
	if h.sessionService.reviewQueuePoller != nil {
		if instance := h.sessionService.reviewQueuePoller.FindInstance(sessionID); instance != nil {
			log.Info("[resolveSession] found managed session in ReviewQueuePoller", "session", sessionID)
			return instance, false // Not external
		}
	}

	// Priority 2: Check ExternalDiscovery for external sessions
	// Check external sessions BEFORE falling back to storage, because storage.LoadInstances()
	// would restart ALL managed sessions (expensive and breaks PTY connections)
	if h.externalDiscovery != nil {
		// Try to find by session title/ID first
		sessions := h.externalDiscovery.GetSessions()
		for _, inst := range sessions {
			if inst.MatchesID(sessionID) {
				log.Info("[resolveSession] found external session via ExternalDiscovery", "session", sessionID)
				return inst, true // External session
			}
		}

		// Also try by tmux session name (for direct tmux connections)
		if inst := h.externalDiscovery.GetSessionByTmux(sessionID); inst != nil {
			log.Info("[resolveSession] found external session by tmux name", "session", sessionID)
			return inst, true // External session
		}
	}

	// Session not found. Do NOT fall back to storage.LoadInstances() — that call restarts
	// every managed session as a side effect and must never be used for a lookup.
	// If the session isn't in the poller or external discovery, it doesn't exist from
	// this handler's perspective. The caller returns a proper not-found response.
	log.Warn("[resolveSession] session not found in poller or external discovery", "session", sessionID)
	return nil, false
}

// HandleWebSocket upgrades HTTP connection to WebSocket and handles ConnectRPC protocol
func (h *ConnectRPCWebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("failed to upgrade connection", "err", err)
		return
	}
	defer conn.Close()

	log.Info("ConnectRPC WebSocket connection established")

	stream, ok := negotiateConnectStream(conn, r)
	if !ok {
		return
	}

	// Call StreamTerminal, then send EndStream while the WebSocket is still open.
	// HandleWebSocket is the single place responsible for sending EndStream, ensuring
	// it is always sent regardless of which code path streamTerminal takes.
	streamDone := TrackOpenStream("StreamTerminal")
	err = h.streamTerminal(stream)
	streamDone()
	if err != nil {
		log.Error("StreamTerminal error", "err", err)
		sendEndStreamError(stream, err)
		return
	}
	sendEndStreamSuccess(stream)
}

// negotiateConnectStream performs the ConnectRPC-over-WebSocket handshake:
// reads the header and enveloped-body messages, validates the RPC method,
// and sends the response headers + initial empty response body. Returns
// ok=false on any failure — already logged, and reported to the client as
// an error frame wherever the protocol got far enough to send one.
func negotiateConnectStream(conn *websocket.Conn, r *http.Request) (*connectWebSocketStream, bool) {
	// 30s deadline: a client that never sends headers should not hold the connection open.
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, headersBytes, err := conn.ReadMessage()
	_ = conn.SetReadDeadline(time.Time{}) // clear deadline for subsequent reads
	if err != nil {
		log.Error("failed to read headers", "err", err)
		return nil, false
	}
	log.Info("received headers", "headers", parseConnectHeaders(string(headersBytes)))

	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, bodyBytes, err := conn.ReadMessage()
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		log.Error("failed to read request body", "err", err)
		return nil, false
	}

	envelope, _, err := protocol.ParseEnvelope(bodyBytes)
	if err != nil {
		log.Error("failed to parse envelope", "err", err)
		sendErrorResponse(conn, fmt.Sprintf("Invalid envelope: %v", err))
		return nil, false
	}

	// Only StreamTerminal is supported today.
	if methodPath := r.URL.Path; !strings.HasSuffix(methodPath, sessionv1connect.SessionServiceStreamTerminalProcedure) {
		log.Error("unsupported RPC method", "method", methodPath)
		sendErrorResponse(conn, fmt.Sprintf("Unsupported method: %s", methodPath))
		return nil, false
	}

	if !sendConnectHandshakeResponse(conn) {
		return nil, false
	}

	log.Info("sent initial response body, starting terminal stream")
	return &connectWebSocketStream{conn: conn, requestMsg: envelope.Data}, true
}

// sendConnectHandshakeResponse sends the response headers and the initial
// empty response body ConnectRPC's streaming protocol requires to acknowledge
// the connection before the real stream begins (no EndStream flag yet).
func sendConnectHandshakeResponse(conn *websocket.Conn) bool {
	responseHeaders := "Status-Code: 200\r\nContent-Type: application/proto\r\n\r\n"
	if err := conn.WriteMessage(websocket.TextMessage, []byte(responseHeaders)); err != nil {
		log.Error("failed to send response headers", "err", err)
		return false
	}

	emptyResponse := &sessionv1.TerminalData{SessionId: "", Data: nil}
	responseBytes, err := proto.Marshal(emptyResponse)
	if err != nil {
		log.Error("failed to marshal initial response", "err", err)
		return false
	}

	responseEnvelope := protocol.CreateEnvelope(0, responseBytes)
	if err := conn.WriteMessage(websocket.BinaryMessage, responseEnvelope); err != nil {
		log.Error("failed to send initial response body", "err", err)
		return false
	}
	return true
}

// connectWebSocketStream wraps a WebSocket connection for ConnectRPC streaming
type connectWebSocketStream struct {
	conn       *websocket.Conn
	requestMsg []byte
	writeMutex sync.Mutex // Protects concurrent writes to WebSocket
}

// WriteMessage safely writes a message to the WebSocket with mutex protection
func (s *connectWebSocketStream) WriteMessage(messageType int, data []byte) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()
	return s.conn.WriteMessage(messageType, data)
}

// streamTerminal handles the StreamTerminal RPC method
func (h *ConnectRPCWebSocketHandler) streamTerminal(stream *connectWebSocketStream) error {
	// Parse the request message to get TerminalData
	var terminalData sessionv1.TerminalData
	if err := proto.Unmarshal(stream.requestMsg, &terminalData); err != nil {
		return fmt.Errorf("failed to unmarshal TerminalData: %w", err)
	}

	sessionID := terminalData.SessionId
	shellID := terminalData.ShellId
	log.Info("StreamTerminal called", "session", sessionID, "shell", shellID)

	// Resolve session using unified resolution strategy
	// This checks ReviewQueuePoller, Storage, and ExternalDiscovery in priority order
	instance, isExternal := h.resolveSession(sessionID)
	if instance == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	log.Debug("[streamTerminal] resolved session", "session", sessionID, "external", isExternal)

	// Shell tabs are independent sibling tmux sessions (see instance_shells.go), not the
	// main session's PTY, so they route separately from the main-terminal logic below.
	if shellID != "" {
		return h.routeShellStream(stream, instance, sessionID, shellID)
	}

	if instance.IsHibernated() {
		if err := resumeHibernatedInstance(instance, sessionID); err != nil {
			return err
		}
	}

	return h.routeMainTerminalStream(stream, instance, sessionID)
}

// routeShellStream routes a shell-tab stream to control mode (default) or
// capture-pane polling (STAPLER_SQUAD_USE_CONTROL_MODE=false), keeping shells
// on the same low-latency event-driven pipeline as the main terminal by
// default instead of falling back to capture-pane polling.
func (h *ConnectRPCWebSocketHandler) routeShellStream(stream *connectWebSocketStream, instance *session.Instance, sessionID, shellID string) error {
	shellTmuxSessionName, ok := instance.GetShellTmuxSessionName(shellID)
	if !ok {
		return fmt.Errorf("shell not found: %s (session %s)", shellID, sessionID)
	}
	if useControlMode := os.Getenv("STAPLER_SQUAD_USE_CONTROL_MODE"); useControlMode == "" || useControlMode == "true" {
		log.Info("[WebSocket] routing shell stream to control mode", "session", sessionID, "shell", shellID, "tmux", shellTmuxSessionName)
		return h.streamShellViaControlMode(stream, instance, shellID, shellTmuxSessionName)
	}
	log.Info("[WebSocket] routing shell stream to capture-pane polling", "session", sessionID, "shell", shellID, "tmux", shellTmuxSessionName)
	return h.streamViaTmuxCapturePane(stream, instance, shellTmuxSessionName)
}

// resumeHibernatedInstance restarts a hibernated instance's controller and
// session driver before streaming. Hibernate() kills the tmux session
// entirely, so without this, the streaming paths below would find no tmux
// session and silently create a bare replacement, leaving Status stuck at
// Hibernated even though a live tmux session now exists underneath it. The
// brief race between this returning and the resumed tmux session actually
// existing is absorbed by RestoreWithWorkDir's own retry/backoff (~1.5s
// total) inside the streaming paths that follow.
func resumeHibernatedInstance(instance *session.Instance, sessionID string) error {
	log.Info("[WebSocket] resuming hibernated session before streaming", "session", sessionID)
	if err := instance.ResumeFromHibernation(context.Background()); err != nil {
		return fmt.Errorf("failed to resume hibernated session %q: %w", sessionID, err)
	}
	return nil
}

// routeMainTerminalStream picks the streaming path for a main-session (non-shell)
// terminal: hub-owned or control-mode for a managed session with control mode
// enabled (STAPLER_SQUAD_USE_CONTROL_MODE, default on), else capture-pane
// polling — the only method that works for external/unmanaged tmux sessions,
// since tmux's PTY belongs to `tmux attach-session`, not the actual process,
// and only updates its buffer on pane-content change rather than streaming
// continuously.
func (h *ConnectRPCWebSocketHandler) routeMainTerminalStream(stream *connectWebSocketStream, instance *session.Instance, sessionID string) error {
	useControlMode := os.Getenv("STAPLER_SQUAD_USE_CONTROL_MODE")
	if (useControlMode == "" || useControlMode == "true") && instance.Snapshot().IsManaged {
		// Resolved once per tmux session via StreamOwnershipLock.Resolve and cached
		// sticky for that session's lifetime (Epic 3.1) — see ADR-003 for why a
		// per-connection re-read would let a flag flip mid-rollout split one
		// session across two owners.
		if streamhub.AcquireOwnershipLock(tmuxSessionNameForStreamPath(instance)).Resolve(useStreamHub()) == streamhub.PathHubOwned {
			log.Info("[WebSocket] routing managed session to hub-owned streaming", "session", sessionID)
			return h.streamViaHub(stream, instance)
		}
		log.Info("[WebSocket] routing managed session to control mode streaming", "session", sessionID)
		return h.streamViaControlMode(stream, instance)
	}

	log.Info("[WebSocket] routing session to capture-pane polling", "session", sessionID)
	return h.streamViaTmuxCapturePane(stream, instance, "")
}

// handleTmuxRestoreFailure centralizes RestoreWithWorkDir failure handling for
// streamViaControlMode and streamViaHub. A missing working directory (a pruned
// git worktree, tmux.ErrWorkDirMissing) moves the session to PermanentlyFailed
// rather than Stopped, since only PermanentlyFailed has a "Retry now" UI action
// (restartForRetry re-creates the worktree), and the wrapped error lets
// sendEndStreamError give the browser a non-retriable code instead of burning
// the reconnect budget against a directory that isn't coming back.
func handleTmuxRestoreFailure(instance *session.Instance, restoreErr error) error {
	if errors.Is(restoreErr, tmux.ErrWorkDirMissing) {
		instance.SetCreationProgress(fmt.Sprintf("Session failed: %s", restoreErr.Error()))
		instance.ForceStatus(session.PermanentlyFailed)
	}
	return fmt.Errorf("tmux session missing and restore failed: %w", restoreErr)
}

// resolveControlModeOwnershipOrJoinHub derives instance's tmux session name
// and asserts the legacy per-connection ownership path — mirroring
// GetOrCreate's own defensive check (Story 3.1.2) so streamViaControlMode is
// safe to call from any future entry point, not just today's single gated
// call site. If a concurrent hub-bound connection already won the ownership
// resolution for this session, joinedHub is true and err is the result of
// joining streamViaHub instead of proceeding as an independent legacy owner
// (which would let this connection resize/capture the pane outside the
// hub's single-owner pipeline).
func (h *ConnectRPCWebSocketHandler) resolveControlModeOwnershipOrJoinHub(stream *connectWebSocketStream, instance *session.Instance) (sessionID, tmuxSessionName string, joinedHub bool, err error) {
	snap := instance.Snapshot()
	sessionID = snap.Title
	tmuxPrefix := snap.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_"
	}
	// Always derive via the canonical sanitizer, never by hand-concatenating prefix+title —
	// a raw title containing spaces would target a session name that was never created (#162).
	tmuxSessionName = tmux.NewSessionName(snap.Title, tmuxPrefix).String()

	if _, lockErr := streamhub.AcquireOwnershipLock(tmuxSessionName).ResolveExpecting(false, streamhub.PathLegacyPerConnection); lockErr != nil {
		log.Info("[streamViaControlMode] ownership resolved to hub-owned path concurrently, joining it instead of proceeding as an independent legacy owner",
			"session", sessionID, "tmux", tmuxSessionName, "err", lockErr)
		return sessionID, tmuxSessionName, true, h.streamViaHub(stream, instance)
	}
	return sessionID, tmuxSessionName, false, nil
}

// parseControlModeHandshake parses the client's first WebSocket message and
// extracts its CurrentPaneRequest — the client is required to send its
// terminal dimensions in this first message rather than an empty handshake.
func parseControlModeHandshake(stream *connectWebSocketStream) (*sessionv1.CurrentPaneRequest, error) {
	var handshakeData sessionv1.TerminalData
	if err := proto.Unmarshal(stream.requestMsg, &handshakeData); err != nil {
		return nil, fmt.Errorf("failed to parse handshake: %w", err)
	}
	currentPaneReq := handshakeData.GetCurrentPaneRequest()
	if currentPaneReq == nil {
		return nil, fmt.Errorf("handshake missing CurrentPaneRequest - client may need update")
	}
	return currentPaneReq, nil
}

// ensureControlModeStarted makes sure a live backend process backs streamer
// before starting control mode, restoring it first if needed, then starts
// control mode itself.
//
// The liveness check must happen before StartControlMode because
// StartControlMode only returns an error if the process fails to launch —
// not if the session can't be found, since that error arrives asynchronously
// via the output reader goroutine instead. The check uses the no-cache
// variant: a stale cached positive (still true after the session died) would
// otherwise let control mode attach to a dead session and immediately
// receive %exit.
func (h *ConnectRPCWebSocketHandler) ensureControlModeStarted(instance *session.Instance, sessionID string, streamer SessionStreamer) error {
	if !instance.IsBackendProcessAlive() {
		log.Info("[streamViaControlMode] session not alive, restoring before control mode", "session", sessionID)
		workDir := instance.GetWorkingDirectory()
		if restoreErr := instance.RestoreProcess(workDir); restoreErr != nil {
			return handleTmuxRestoreFailure(instance, restoreErr)
		}
	}
	if err := streamer.StartControlMode(); err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	return nil
}

// performInitialResizeNudge resizes tmux to the handshake's dimensions before
// the initial capture, using a ±1 nudge (resize.WithForcedRedraw) to
// guarantee SIGWINCH even if tmux is already at the target size — otherwise
// tmux resize-window is a no-op when dimensions already match, and the TUI
// never redraws, leaving stale-dimension content that renders garbled in a
// fresh xterm.js terminal. Runs unconditionally on every reconnect,
// regardless of whether the browser's reported dimensions actually changed.
func (h *ConnectRPCWebSocketHandler) performInitialResizeNudge(instance *session.Instance, sessionID string, streamGeneration int64, currentPaneReq *sessionv1.CurrentPaneRequest, quiescenceCh chan struct{}) {
	if currentPaneReq.TargetCols == nil || currentPaneReq.TargetRows == nil {
		log.Warn("[streamViaControlMode] handshake missing dimensions, layout may be incorrect")
		return
	}
	targetCols := int(*currentPaneReq.TargetCols)
	targetRows := int(*currentPaneReq.TargetRows)

	log.Info("[streamViaControlMode] handshake dimensions, forcing redraw via nudge", "cols", targetCols, "rows", targetRows)

	// The nudge repaints the TUI at targetCols x targetRows, so any snapshot cached
	// from an earlier connection is stale by construction. Drop it here so the
	// capture that follows re-reads the pane instead of serving old-dimension
	// content that xterm.js would then paint live frames over.
	h.invalidateSnapshot(sessionID)

	if err := resize.WithForcedRedraw(instance.ResizePTY, targetCols, targetRows); err != nil {
		log.Error("[streamViaControlMode] failed to resize", "err", err)
		return
	}

	// Wait for the TUI to complete its full redraw before capturing. The
	// output-forwarding goroutine is already subscribed and running, so
	// quiescenceCh receives real signals from redraw frames here — this is
	// genuine quiescence detection, not a fixed settle timer.
	quiescenceStart := time.Now()
	waitForQuiescence(quiescenceCh, 500*time.Millisecond, 200*time.Millisecond)
	if elapsed := time.Since(quiescenceStart); elapsed >= 500*time.Millisecond-5*time.Millisecond {
		log.Warn("[streamViaControlMode] initial quiescence timed out; session may be stalled", "elapsed", elapsed.Round(time.Millisecond), "session", sessionID)
	}
	log.Info("[streamViaControlMode] tmux resized, redraw complete", "cols", targetCols, "rows", targetRows, "generation", streamGeneration)
}

// captureAndSendInitialSnapshot captures the pane at its (already resized)
// current dimensions and sends it as the canonical initial snapshot. If
// capture fails — e.g. a concurrent Instance.Resume() is still mid-restore —
// getOrRefreshSnapshot's caller (waitForPaneContent) polls briefly rather
// than failing immediately: the frontend's loading spinner stays up until the
// first WS message arrives, so withholding that message turns a misleading
// "stopped" flash into a visible "still resuming" wait.
func (h *ConnectRPCWebSocketHandler) captureAndSendInitialSnapshot(stream *connectWebSocketStream, instance *session.Instance, sessionID string) error {
	initialContent, err := h.getOrRefreshSnapshot(sessionID, func() (string, error) {
		return waitForPaneContent(instance)
	})
	if err != nil {
		log.Info("[streamViaControlMode] capture-pane failed, sending stopped notice", "session", sessionID, "err", err)
		// Send a visible notice instead of leaving the terminal blank so the user
		// knows why there is no output (session stopped, not a connection failure).
		initialContent = "\r\n\x1b[33m[session stopped — no terminal content available]\x1b[0m\r\n"
	}
	if initialContent == "" {
		return nil
	}
	return sendInitialSnapshotContent(stream, instance, sessionID, initialContent)
}

// sendInitialSnapshotContent marshals and sends initialContent as the
// canonical initial snapshot, stripping cursor-positioning codes first.
// capture-pane -e preserves absolute cursor positions (ESC[n;mH) from the
// live session, and replaying these in a fresh xterm.js terminal causes
// garbled output because the positions assume a prior terminal state that no
// longer exists — colors (SGR) are preserved, only positioning is removed.
func sendInitialSnapshotContent(stream *connectWebSocketStream, instance *session.Instance, sessionID, initialContent string) error {
	fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(streamhub.RawPaneContent(initialContent)), instance)
	terminalData := newTerminalOutputData(sessionID, fullContent)

	dataBytes, err := proto.Marshal(terminalData)
	if err != nil {
		return fmt.Errorf("failed to marshal initial content: %w", err)
	}
	if err := stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, dataBytes)); err != nil {
		return fmt.Errorf("failed to send initial content: %w", err)
	}

	log.Info("[streamViaControlMode] sent initial snapshot", "bytes", len(initialContent), "session", sessionID)
	log.Info("[streamViaControlMode] scrollback lines sent", "lines", strings.Count(initialContent, "\n")+1, "session", sessionID)
	instance.UpdateTerminalTimestamps(initialContent, true)
	return nil
}

// sendInitialScrollback sends the most recent scrollback history so the
// client can populate its scrollback buffer immediately on connect (R2.2).
// Failures are logged, not returned — a missing initial scrollback batch
// degrades to the client fetching it lazily, it doesn't need to fail the stream.
func (h *ConnectRPCWebSocketHandler) sendInitialScrollback(stream *connectWebSocketStream, sessionID string) {
	if h.scrollbackManager == nil {
		return
	}
	const initialScrollbackLines = 500
	sbData, sbErr := h.scrollbackManager.GetRecentLines(sessionID, initialScrollbackLines)
	if sbErr != nil {
		log.Warn("[streamViaControlMode] failed to fetch initial scrollback", "session", sessionID, "err", sbErr)
		return
	}
	if len(sbData) == 0 {
		return
	}

	sbStats, statsErr := h.scrollbackManager.GetStats(sessionID)
	if statsErr != nil {
		sbStats = scrollback.ScrollbackStats{}
	}
	sbResp := buildInitialScrollbackResponse(sessionID, sbData, sbStats, initialScrollbackLines)
	if sbBytes, merr := proto.Marshal(sbResp); merr != nil {
		log.Error("[streamViaControlMode] failed to marshal initial scrollback", "session", sessionID, "err", merr)
	} else if wsErr := stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, sbBytes)); wsErr == nil {
		log.Info("[streamViaControlMode] sent initial scrollback", "bytes", len(sbData), "session", sessionID)
	}
}

// buildInitialScrollbackResponse wraps sbData (raw bytes from GetRecentLines)
// as a single-chunk ScrollbackResponse. hasMore is true when the session has
// more history than the initial window.
func buildInitialScrollbackResponse(sessionID string, sbData []byte, sbStats scrollback.ScrollbackStats, initialScrollbackLines int) *sessionv1.TerminalData {
	hasMore := sbStats.MemoryLines > initialScrollbackLines || sbStats.StorageBytes > 0
	return &sessionv1.TerminalData{
		SessionId: sessionID,
		Data: &sessionv1.TerminalData_ScrollbackResponse{
			ScrollbackResponse: &sessionv1.ScrollbackResponse{
				Chunks:  []*sessionv1.ScrollbackChunk{{Data: sbData, Sequence: sbStats.NewestSequence}},
				HasMore: hasMore,
				// #nosec G115 -- MemoryLines is a non-negative in-memory scrollback
				// line count (bounded by the scrollback buffer's configured capacity),
				// never negative; widening int -> uint64 cannot overflow.
				TotalLines:     uint64(sbStats.MemoryLines),
				OldestSequence: sbStats.OldestSequence,
				NewestSequence: sbStats.NewestSequence,
			},
		},
	}
}

// streamViaControlMode handles WebSocket streaming using tmux control mode (-C flag).
// This is the proper way to get real-time terminal output from tmux sessions.
// Control mode provides structured notifications (%output, %session-changed, etc.) via the tmux protocol.
//
// Benefits over pipe-pane + FIFO:
// - No FIFO complexity or EOF issues
// - Direct protocol communication with tmux
// - Structured, parseable output format
// - Real-time notifications (no polling needed)
// - Native tmux feature (not a hack)
//
// See: https://github.com/tmux/tmux/wiki/Control-Mode
func (h *ConnectRPCWebSocketHandler) streamViaControlMode(stream *connectWebSocketStream, instance *session.Instance) error {
	sessionID, tmuxSessionName, joinedHub, err := h.resolveControlModeOwnershipOrJoinHub(stream, instance)
	if joinedHub {
		return err
	}

	streamGeneration, doneStreaming := h.recordControlModeStreamStart(sessionID, tmuxSessionName)
	defer doneStreaming()

	log.Info("[streamViaControlMode] starting", "session", sessionID, "tmux", tmuxSessionName, "generation", streamGeneration)

	// Update LastViewed timestamp - user is viewing this session
	instance.MarkViewed()

	// Client sends dimensions in the handshake's first message (no empty
	// handshake), so tmux can be resized and captured immediately.
	currentPaneReq, err := parseControlModeHandshake(stream)
	if err != nil {
		return err
	}

	// Start control mode early so we can subscribe to output events for
	// quiescence detection BEFORE the resize nudge below (see its own doc
	// comment for why the nudge itself needs no separate remote-specific path).
	// Use the SessionStreamer interface to decouple this handler from the
	// concrete *tmux.TmuxSession type — *Instance satisfies it via delegation.
	var streamer SessionStreamer = instance
	if err := h.ensureControlModeStarted(instance, sessionID, streamer); err != nil {
		return err
	}
	defer func() {
		if err := streamer.StopControlMode(); err != nil {
			log.Warn("[streamViaControlMode] StopControlMode error", "err", err)
		}
	}()

	// Subscribe and start the output-forwarding goroutine BEFORE the resize nudge below,
	// so quiescenceCh (signaled inline by that goroutine on every received frame) has a
	// real producer during the initial handshake wait instead of degenerating into a fixed
	// timer. Frames that arrive before the initial snapshot has been captured and sent are
	// used only to drive quiescence detection — they are not forwarded to the client
	// (forwardingReady gates that), since the canonical initial snapshot captured after the
	// resize settles supersedes any partial pre-resize content.
	subscriberID, updateChan := streamer.SubscribeControlModeUpdates()
	defer streamer.UnsubscribeControlModeUpdates(subscriberID)

	log.Info("[streamViaControlMode] subscribed to control mode", "subscriber_id", subscriberID, "session", sessionID)

	quiescenceCh := make(chan struct{}, 16)
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})
	var forwardingReady atomic.Bool
	// resizeSettling mirrors forwardingReady but for the live (post-connect) resize path
	// below: while a window-drag/panel-resize reflow is in flight, the TUI emits partial
	// redraw frames at intermediate/old dimensions. Forwarding those live races the
	// authoritative post-resize snapshot the resize handler sends once quiescence is
	// reached, so xterm.js can end up compositing an in-progress reflow frame on top of
	// (or interleaved with) that snapshot — the same "garbled overlapping-column
	// rendering" the initial-connect forwardingReady gate above was added to prevent,
	// just triggered by resizing instead of reconnecting.
	var resizeSettling atomic.Bool

	// Goroutine 1: Forward control mode updates to WebSocket.
	go h.forwardControlModeOutput(controlModeOutputForwarderParams{
		stream:          stream,
		instance:        instance,
		sessionID:       sessionID,
		updateChan:      updateChan,
		doneChan:        doneChan,
		errChan:         errChan,
		quiescenceCh:    quiescenceCh,
		forwardingReady: &forwardingReady,
		resizeSettling:  &resizeSettling,
	})

	h.performInitialResizeNudge(instance, sessionID, streamGeneration, currentPaneReq, quiescenceCh)

	if err := h.captureAndSendInitialSnapshot(stream, instance, sessionID); err != nil {
		return err
	}

	// The canonical initial snapshot has been sent; frames from here on are live
	// updates the output-forwarding goroutine should actually forward to the client.
	forwardingReady.Store(true)

	h.sendInitialScrollback(stream, sessionID)

	// resizeCh coalesces rapid resize events (e.g. window drags) so only the
	// latest dimensions reach SetWindowSize. The channel holds at most one
	// pending resize; the goroutine is tied to doneChan so it exits with the stream.
	resizeCh := make(chan resizeReq, 1)
	go h.runControlModeResizeCoalescer(controlModeResizeCoalescerParams{
		stream:         stream,
		instance:       instance,
		sessionID:      sessionID,
		doneChan:       doneChan,
		resizeCh:       resizeCh,
		quiescenceCh:   quiescenceCh,
		resizeSettling: &resizeSettling,
	})

	// Goroutine 2: Read from WebSocket and handle input/commands
	go runInputReadLoop(inputReadLoopParams{
		stream:    stream,
		doneChan:  doneChan,
		errChan:   errChan,
		sessionID: sessionID,
		onInput: func(data []byte) {
			// Check send permission
			if !instance.Permissions.CanSendCommand {
				log.Warn("[streamViaControlMode] send permission denied", "session", sessionID)
				return
			}

			// Update timestamps for user interaction
			instance.UpdateTerminalTimestamps(string(data), true)
			instance.MarkUserResponded()

			// Try CM path first (low-latency, no subprocess). Falls back to
			// subprocess send-keys if CM queue is backed up or not running.
			// Errors are non-fatal — keystrokes may be lost under load but
			// the stream stays alive (sending TerminalError kills the stream).
			sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
			sendErr := instance.SendInputViaControlMode(sendCtx, data)
			sendCancel()
			if sendErr != nil {
				log.Warn("[streamViaControlMode] CM input failed, retrying via subprocess", "session", tmuxSessionName, "err", sendErr)
				if fbErr := sendInputToTmux(instance.Snapshot().TmuxServerSocket, tmuxSessionName, data); fbErr != nil {
					log.Error("[streamViaControlMode] subprocess fallback also failed", "session", tmuxSessionName, "err", fbErr)
				}
			}
		},
		onResize: func(cols, rows int) {
			dispatchResizeRequest(resizeCh, resizeReq{cols, rows})
		},
		onScrollbackRequest: func(startLine, endLine string) (string, error) {
			// Delegate the tmux capture (the only piece of this handling that depends
			// on `instance`, which runInputReadLoop does not have access to) back to
			// streamViaControlMode; response building, marshaling, and writing stay
			// inside runInputReadLoop as part of the pure-moved loop body.
			return instance.GetScrollbackHistory(startLine, endLine)
		},
		onCurrentPaneRequest: func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
			// Handle a mid-stream CurrentPaneRequest (e.g. a client-initiated resync) via the
			// same shared helper the initial handshake and streamViaTmuxCapturePane use.
			return handleCurrentPaneRequest(ctx, sessionID, instance, req, currentResyncOptions())
		},
		resizeSettling: &resizeSettling,
	})

	// Wait for either goroutine to error or complete.
	// EndStream is sent by the caller (HandleWebSocket) after this function returns.
	select {
	case err := <-errChan:
		return err
	case <-doneChan:
		return nil
	}
}

// resizeReq is one coalesced client resize request (cols/rows) for
// runControlModeResizeCoalescer.
type resizeReq struct{ cols, rows int }

// controlModeResizeCoalescerParams bundles the per-connection state
// runControlModeResizeCoalescer needs — extracted from an anonymous
// goroutine closure that captured too many locals to pass as bare parameters.
type controlModeResizeCoalescerParams struct {
	stream         *connectWebSocketStream
	instance       *session.Instance
	sessionID      string
	doneChan       chan struct{}
	resizeCh       chan resizeReq
	quiescenceCh   chan struct{}
	resizeSettling *atomic.Bool
}

// runControlModeResizeCoalescer coalesces rapid resize events (e.g. window
// drags) so only the latest dimensions reach SetWindowSize, applying at most
// one resize per event and suppressing duplicates within 50ms (R1.5 — avoid
// redundant PTY ioctls when rapid window-drag events produce identical
// dimensions). Exits when doneChan closes.
func (h *ConnectRPCWebSocketHandler) runControlModeResizeCoalescer(p controlModeResizeCoalescerParams) {
	var last resizeReq
	var lastAppliedAt time.Time
	for {
		select {
		case <-p.doneChan:
			return
		case r := <-p.resizeCh:
			if r == last && time.Since(lastAppliedAt) < 50*time.Millisecond {
				continue
			}
			if h.applyOneControlModeResize(p, r) {
				last, lastAppliedAt = r, time.Now()
			}
		}
	}
}

// applyOneControlModeResize resizes the tmux pane to r, waits for the reflow
// to settle, and sends the client the pre/post ResizeQuiescence signals plus
// a fresh post-resize snapshot. Returns true if the resize was actually
// applied (false on a transient SetWindowSize failure, in which case last
// should not be updated so the next differing resize isn't treated as a
// duplicate).
func (h *ConnectRPCWebSocketHandler) applyOneControlModeResize(p controlModeResizeCoalescerParams, r resizeReq) bool {
	// Suppress live forwarding for the duration of the reflow: the TUI's
	// partial redraw frames at intermediate dimensions would otherwise race
	// the authoritative post-resize snapshot sent below. Cleared once that
	// snapshot has been sent, on every exit path (including early failure).
	p.resizeSettling.Store(true)
	resizeDone := func() { p.resizeSettling.Store(false) }

	if err := p.instance.SetWindowSize(r.cols, r.rows); err != nil {
		if errors.Is(err, streamhub.ErrSessionNotStarted) {
			// Same transient cold-start window StreamHub's applyNegotiatedSize
			// skips (session/streamhub/hub.go) -- the session actor hasn't
			// finished installing its PTY yet. Not an error worth logging;
			// the next resize event (or a client-side retry) will land once
			// it has.
			log.Info("[streamViaControlMode] session not started yet, skipping this resize", "session", p.sessionID)
		} else {
			log.Error("[streamViaControlMode] failed to resize", "err", err)
		}
		resizeDone()
		return false
	}

	sendControlModeResizeQuiescence(p.stream, p.sessionID, r, true)
	waitForResizeQuiescence(p, r) // R1.1: avoid capturing partially-reflowed content
	h.sendPostResizeSnapshot(p, r)

	// Re-enable live forwarding now that the authoritative post-resize
	// snapshot has been sent — must happen before the client-facing
	// Resizing:false signal below, not after, or a live frame arriving in
	// between would be forwarded while the client still thinks it's mid-reflow.
	resizeDone()
	sendControlModeResizeQuiescence(p.stream, p.sessionID, r, false)
	return true
}

// waitForResizeQuiescence waits for tmux to finish reflowing after resize r
// before the next capture-pane (R1.1 — avoid partially-reflowed content).
func waitForResizeQuiescence(p controlModeResizeCoalescerParams, r resizeReq) {
	const deadline = 300 * time.Millisecond
	start := time.Now()
	waitForQuiescence(p.quiescenceCh, deadline, 100*time.Millisecond)
	if elapsed := time.Since(start); elapsed >= deadline-5*time.Millisecond {
		log.Error("[streamViaControlMode] quiescence timed out, sending snapshot anyway", "elapsed", elapsed.Round(time.Millisecond), "session", p.sessionID, "cols", r.cols, "rows", r.rows)
	}
}

// sendControlModeResizeQuiescence emits a ResizeQuiescence signal (R1.4) so
// the client knows whether a tmux reflow triggered by r is starting or has
// completed with a stable snapshot sent.
func sendControlModeResizeQuiescence(stream *connectWebSocketStream, sessionID string, r resizeReq, resizing bool) {
	rqMsg := &sessionv1.TerminalData{
		SessionId: sessionID,
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
		log.Error("[streamViaControlMode] failed to marshal ResizeQuiescence", "session", sessionID, "err", merr)
	} else {
		_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, rqBytes))
	}
}

// sendPostResizeSnapshot captures and sends a fresh pane snapshot at the new
// dimensions so the client display is immediately correct without waiting
// for the next PTY event (R1.3 — post-resize snapshot).
func (h *ConnectRPCWebSocketHandler) sendPostResizeSnapshot(p controlModeResizeCoalescerParams, r resizeReq) {
	snapContent, snapErr := p.instance.CapturePaneContentRaw()
	if snapErr != nil || snapContent == "" {
		return
	}
	h.markSnapshotDirty(p.sessionID)
	// withCursorSync is required here for the same reason as every other
	// snapshot send (see its doc comment): without a trailing CUP the
	// xterm.js cursor desyncs from tmux's, and relative-cursor-up redraws
	// from an Ink TUI then stack below the last repaint instead of
	// overwriting it — this was the one snapshot send of five that omitted
	// it, so resizing (the action users take to clear a garbled pane) left
	// the cursor desynced and made interactive menus billow.
	fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(snapContent), p.instance)
	// ResyncId is intentionally left unset: this snapshot is triggered by a
	// plain TerminalResize frame, which carries no resync_id (only
	// CurrentPaneRequest does, proto events.proto) — a client-initiated
	// resync is instead handled entirely by handleCurrentPaneRequest, which
	// already echoes resync_id (Task 3.2.1.1).
	snapMsg := newTerminalOutputData(p.sessionID, fullContent)
	if snapBytes, merr := proto.Marshal(snapMsg); merr != nil {
		log.Error("[streamViaControlMode] failed to marshal post-resize snapshot", "session", p.sessionID, "err", merr)
	} else {
		_ = p.stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, snapBytes))
	}
}

// controlModeOutputForwarderParams bundles the per-connection state
// forwardControlModeOutput needs — extracted from an anonymous goroutine
// closure that captured too many locals to pass as bare parameters.
type controlModeOutputForwarderParams struct {
	stream          *connectWebSocketStream
	instance        *session.Instance
	sessionID       string
	updateChan      <-chan []byte
	doneChan        chan struct{}
	errChan         chan error
	quiescenceCh    chan struct{}
	forwardingReady *atomic.Bool
	resizeSettling  *atomic.Bool
}

// forwardControlModeOutput is streamViaControlMode's Goroutine 1: forwards
// control-mode updates to the WebSocket, coalescing back-to-back frames into
// a single proto message per write so rapid terminal bursts don't cost one
// syscall each.
func (h *ConnectRPCWebSocketHandler) forwardControlModeOutput(p controlModeOutputForwarderParams) {
	defer close(p.doneChan)

	log.Info("[streamViaControlMode] output goroutine started", "session", p.sessionID)

	// escapeParser is fetched once; may be nil if no controller is running.
	escapeParser := p.instance.GetEscapeParser()

	for {
		select {
		case <-p.doneChan:
			return
		case data, ok := <-p.updateChan:
			if !ok {
				sendExitContentOnClose(p.stream, p.instance, p.sessionID)
				return
			}
			if stop := h.forwardOneControlModeFrame(p, &escapeParser, data); stop {
				return
			}
		}
	}
}

// forwardOneControlModeFrame handles a single frame received by
// forwardControlModeOutput: applies the forwarding/resize-settling gate,
// coalesces further immediately-available frames into one write, taps escape
// analytics, and sends the batch. escapeParser is re-fetched lazily through
// the pointer if it was nil at goroutine start (the controller may start
// after the WebSocket connection was established). Returns true if the
// caller should stop (the send failed).
func (h *ConnectRPCWebSocketHandler) forwardOneControlModeFrame(p controlModeOutputForwarderParams, escapeParser **analytics.EscapeCodeParser, data []byte) bool {
	if !p.forwardingReady.Load() || p.resizeSettling.Load() {
		// Either still settling from the initial resize nudge (frame is redraw
		// noise from before the canonical initial snapshot has been captured),
		// or a live resize reflow is in flight. Drop it, but still count it
		// toward quiescence below.
		signalQuiescence(p.quiescenceCh)
		return false
	}

	// Mark snapshot dirty so the next client connect captures fresh content.
	h.markSnapshotDirty(p.sessionID)

	// data shares a broadcast backing array; copy into a pooled buf before
	// appending. marshalProtoEnvelope copies the payload before returning, so
	// buf is safe to return to coalesceBufPool after sendData.
	cbp := coalesceBufPool.Get().(*[]byte)
	buf := coalesceAvailableFrames(append((*cbp)[:0], data...), p.updateChan)
	tapEscapeAnalytics(p.instance, escapeParser, buf)

	sendErr := sendControlModeOutput(p.stream, p.sessionID, buf)
	*cbp = buf[:0]
	coalesceBufPool.Put(cbp)
	if sendErr != nil {
		log.Error("[streamViaControlMode] failed to send output", "err", sendErr)
		p.errChan <- fmt.Errorf("failed to send output: %w", sendErr)
		return true
	}
	// Signal quiescence detector: output is still flowing (resets the quiescence timer).
	// Inline here eliminates the separate subscription + fan-out goroutine.
	signalQuiescence(p.quiescenceCh)
	return false
}

// tapEscapeAnalytics feeds a coalesced output batch to instance's Stage 2
// escape-code parser, re-fetching escapeParser lazily through the pointer if
// it was nil at goroutine start (the controller may start after the
// WebSocket connection was established).
func tapEscapeAnalytics(instance *session.Instance, escapeParser **analytics.EscapeCodeParser, buf []byte) {
	if *escapeParser == nil {
		*escapeParser = instance.GetEscapeParser()
	}
	ep := *escapeParser
	if ep == nil || !ep.IsEnabled() {
		return
	}
	// Use the monotonic PTY byte offset from the circular buffer so
	// session_seq is stable across WebSocket reconnections (mirrors the
	// Stage 1 counter in ResponseStream.streamLoop). GetTotalBytesWritten
	// reflects the buffer's total *after* every coalesced chunk in buf has
	// already been written, so subtract len(buf) to get buf's start offset —
	// without this, every sessionSeq here is off by len(buf) and mangle
	// correlation against Stage 1 records never matches.
	ep.ParseStage2(buf, instance.GetTotalBytesWritten()-int64(len(buf)))
}

// sendControlModeOutput marshals and writes a terminal output message using
// pooled proto + pooled envelope buffers for 0 allocs per frame on the hot path.
func sendControlModeOutput(stream *connectWebSocketStream, sessionID string, data []byte) error {
	msg := terminalDataPool.Get().(*sessionv1.TerminalData)
	msg.SessionId = sessionID
	msg.Data = &sessionv1.TerminalData_Output{
		Output: &sessionv1.TerminalOutput{Data: data},
	}
	err := marshalProtoEnvelope(stream, 0, msg)
	proto.Reset(msg)
	terminalDataPool.Put(msg)
	return err
}

// coalesceAvailableFrames drains every frame immediately available on
// updates into buf (without blocking for more), up to maxBatchFrames total.
// The cap bounds worst-case latency: at 10K fps that is ~3 ms.
func coalesceAvailableFrames(buf []byte, updates <-chan []byte) []byte {
	const maxBatchFrames = 32
	for framesInBatch := 1; framesInBatch < maxBatchFrames; framesInBatch++ {
		select {
		case more, ok := <-updates:
			if !ok {
				return buf
			}
			buf = append(buf, more...)
		default:
			return buf
		}
	}
	return buf
}

// signalQuiescence performs a non-blocking send on ch, dropping the signal if
// one is already pending — the channel only needs to convey "output arrived
// since last checked", not an exact count.
func signalQuiescence(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// sendExitContentOnClose sends the session's captured exit content, if any,
// so the user sees the error instead of a blank terminal when the
// control-mode subscription channel closes because the session itself exited.
func sendExitContentOnClose(stream *connectWebSocketStream, instance *session.Instance, sessionID string) {
	exitContent := instance.GetExitContent()
	if len(exitContent) == 0 {
		return
	}
	exitData := &sessionv1.TerminalData{
		SessionId: sessionID,
		Data: &sessionv1.TerminalData_Output{
			Output: &sessionv1.TerminalOutput{Data: exitContent},
		},
	}
	if exitBytes, merr := proto.Marshal(exitData); merr == nil {
		_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, exitBytes))
	}
}

// connectionCountPollInterval bounds how quickly a subscriber-count change
// (Epic 4.2, Story 4.2.1) reaches an already-connected browser tab. 1s is
// fast enough for a human-perceptible "another tab just attached" signal
// without adding meaningful load — hub.SubscriberCount() is an O(1) map-len
// read under a mutex already held for microseconds elsewhere in the hub.
const connectionCountPollInterval = 1 * time.Second

// sendConnectionCountUpdates polls hub.SubscriberCount() and pushes a
// side-channel TerminalData (an otherwise-empty TerminalOutput carrying only
// ConnectionCount) to this one connection whenever the count changes. See
// streamViaHub's call site for why this is a poll rather than being stamped
// onto the hub's own broadcast frames. Returns once stop is closed.
func sendConnectionCountUpdates(stream *connectWebSocketStream, hub *streamhub.StreamHub, sessionID string, stop <-chan struct{}) {
	ticker := time.NewTicker(connectionCountPollInterval)
	defer ticker.Stop()

	lastSent := -1
	send := func() {
		count := hub.SubscriberCount()
		if count == lastSent {
			return
		}
		lastSent = count
		// #nosec G115 -- count is a websocket subscriber count for one session
		// (StreamHub.SubscriberCount, an O(1) map length), bounded by the number
		// of concurrent client connections, nowhere near int32 range.
		countCopy := int32(count)
		msg := &sessionv1.TerminalData{
			SessionId: sessionID,
			Data: &sessionv1.TerminalData_Output{
				Output: &sessionv1.TerminalOutput{ConnectionCount: &countCopy},
			},
		}
		if err := marshalProtoEnvelope(stream, 0, msg); err != nil {
			log.Warn("[streamViaHub] failed to send connection_count update", "session", sessionID, "err", err)
		}
	}

	send() // report the count this connection sees immediately, don't wait a full tick
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			send()
		}
	}
}

// HubStartFailedErrorCode is the TerminalError.Code sent to the client when
// the hub-owned streaming path fails to start AND its legacy fallback also
// fails, leaving the connection with no working stream at all (design/ux.md
// Surface 2). useTerminalStream.ts checks for this exact code to skip its
// normal backoff-exhaustion path and surface TerminalOutput.tsx's existing
// hardFailedBanner immediately, since retrying will hit the same failure.
const HubStartFailedErrorCode = "HUB_START_FAILED"

// ensureHubBackendAlive confirms a live backend process backs instance,
// restoring it first if needed, and returns the resulting liveness.
// RestoreProcess always returns nil even on PTY attach failure (see the
// identical comment on session/instance.go's own RestoreWithWorkDir call
// site), so a nil error alone does not prove the session is genuinely ready
// to serve capture/resize traffic — this confirms a real PTY attached before
// treating it (and the self-heal decision in ensureHubInstanceStarted that
// depends on it) as alive.
func (h *ConnectRPCWebSocketHandler) ensureHubBackendAlive(instance *session.Instance, sessionID string) (processAlive bool, err error) {
	if instance.IsBackendProcessAlive() {
		return true, nil
	}
	log.Info("[streamViaHub] session not alive, restoring before control mode", "session", sessionID)
	workDir := instance.GetWorkingDirectory()
	if restoreErr := instance.RestoreProcess(workDir); restoreErr != nil {
		return false, handleTmuxRestoreFailure(instance, restoreErr)
	}
	if _, ptyErr := instance.GetPTYReader(); ptyErr != nil {
		log.Warn("[streamViaHub] restored session but PTY attach failed, not treating as alive", "session", sessionID, "err", ptyErr)
		return false, nil
	}
	return true, nil
}

// ensureHubInstanceStarted waits briefly for a concurrent Instance.Start()
// (e.g. server/dependencies.go's boot-time restart) to finish on a reused
// session, since the tmux-alive check above doesn't cover that case —
// without this wait, attaching a subscriber below would race a doomed
// resize. If the wait times out and tmux is confirmed alive, self-heals
// Started() instead of leaving it wedged at false: see
// MarkStartedIfTmuxAlive's doc comment for why "proceeding anyway" here
// previously meant an endless ErrSessionNotStarted retry loop on every
// capture/resize.
func (h *ConnectRPCWebSocketHandler) ensureHubInstanceStarted(instance *session.Instance, sessionID string, processAlive bool) {
	if instance.Started() {
		return
	}
	log.Info("[streamViaHub] instance not started yet, waiting briefly before attaching", "session", sessionID)
	if waitForInstanceStartedEvent(h.sessionService.GetEventBus(), instance, startupWaitTimeout) {
		return
	}
	if !processAlive {
		log.Warn("[streamViaHub] instance still not started after waiting, proceeding anyway", "session", sessionID, "waited", startupWaitTimeout)
		return
	}
	log.Warn("[streamViaHub] instance not started but tmux is alive, marking started", "session", sessionID, "waited", startupWaitTimeout)
	instance.MarkStartedIfTmuxAlive()
}

// resolveHubOrJoinLegacy calls HubRegistry.GetOrCreate, and on a lost
// ownership race (Story 3.1.2 — a concurrent legacy StartControlMode call
// already won StreamOwnershipLock resolution), joins the legacy path via
// streamViaControlMode instead of creating a competing hub. StartControlMode's
// refcounting is shared by both paths, so it having already succeeded in the
// caller says nothing about which ownership model won.
//
// done=true means the caller should return immediately with (nil hub, err):
// err is the result of the legacy fallback (nil on success, non-nil if it
// too failed). done=false means ownership resolved hub-owned as expected —
// hub is the created/retrieved *StreamHub and err is always nil.
func (h *ConnectRPCWebSocketHandler) resolveHubOrJoinLegacy(stream *connectWebSocketStream, instance *session.Instance, sessionID, tmuxSessionName string) (hub *streamhub.StreamHub, done bool, err error) {
	hub, hubErr := HubRegistry.GetOrCreate(tmuxSessionName, instance)
	if hubErr == nil {
		return hub, false, nil
	}
	log.Info("[streamViaHub] ownership resolved to legacy path concurrently, joining it instead of creating a competing hub",
		"session", sessionID, "tmux", tmuxSessionName, "err", hubErr)
	fallbackErr := h.streamViaControlMode(stream, instance)
	if fallbackErr != nil {
		// design/ux.md Surface 2: only signal a hub-start failure to the client
		// when the hub-owned path AND its legacy fallback both failed — i.e.
		// this connection has no working stream at all. The far more common
		// case (GetOrCreate loses the ownership race, falls back, and the
		// fallback succeeds) must stay silent: an error frame there would be a
		// false alarm on a connection the user experiences as working fine
		// (research/ux.md §4b: don't announce a non-event).
		sendHubStartFailedError(stream, sessionID, hubErr, fallbackErr)
	}
	return nil, true, fallbackErr
}

// sendHubStartFailedError best-effort notifies the client that both the
// hub-owned path and its legacy fallback failed, mirroring
// session_service.go's existing "send error back to client" TerminalData_Error
// convention (its WRITE_ERROR/RESIZE_ERROR call sites). The caller's returned
// fallbackErr still closes the stream via sendEndStreamError regardless.
func sendHubStartFailedError(stream *connectWebSocketStream, sessionID string, hubErr, fallbackErr error) {
	errMsg := &sessionv1.TerminalData{
		SessionId: sessionID,
		Data: &sessionv1.TerminalData_Error{
			Error: &sessionv1.TerminalError{
				Message: fmt.Sprintf("hub start failed (%v) and legacy fallback also failed: %v", hubErr, fallbackErr),
				Code:    HubStartFailedErrorCode,
			},
		},
	}
	if b, marshalErr := proto.Marshal(errMsg); marshalErr != nil {
		log.Error("[streamViaHub] failed to marshal hub-start-failed error", "session", sessionID, "err", marshalErr)
	} else if wsErr := stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, b)); wsErr != nil {
		log.Warn("[streamViaHub] failed to send hub-start-failed error", "session", sessionID, "err", wsErr)
	}
}

// attachHubSubscriber attaches a WebSocketTransport subscriber to hub and
// requests the handshake's initial size. StreamHub.AttachSubscriber sends a
// CatchUpSnapshot to every Transport within the same call (Story 1.2.1's AC);
// that send is suppressed for this transport specifically because it's a raw,
// unprepared []byte(content) write that would forward verbatim to the
// WebSocket, whereas the browser client needs the ANSI-sanitized,
// cursor-synced, proto-enveloped version sendHubInitialSnapshot sends
// explicitly instead — StreamHub has no notion of that framing. See
// WebSocketTransport.SuppressNextSend's doc comment for why suppressing
// exactly one Send call here is safe.
type hubSubscriberAttachment struct {
	stream          *connectWebSocketStream
	instance        *session.Instance
	sessionID       string
	tmuxSessionName string
	currentPaneReq  *sessionv1.CurrentPaneRequest
}

func (h *ConnectRPCWebSocketHandler) attachHubSubscriber(connCtx context.Context, hub *streamhub.StreamHub, a hubSubscriberAttachment) streamhub.SubscriberID {
	transport := NewWebSocketTransport(a.stream, a.sessionID)
	transport.SuppressNextSend()
	subscriberID := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{
		CanResize: true,
		CanWrite:  a.instance.Permissions.CanSendCommand,
	})
	transport.BindSubscriber(hub, subscriberID)

	log.Info("[streamViaHub] attached subscriber", "subscriber_id", string(subscriberID), "session", a.sessionID, "tmux", a.tmuxSessionName)

	if a.currentPaneReq.TargetCols != nil && a.currentPaneReq.TargetRows != nil {
		if size, sizeErr := streamhub.NewTerminalSize(int(*a.currentPaneReq.TargetCols), int(*a.currentPaneReq.TargetRows)); sizeErr != nil {
			log.Warn("[streamViaHub] invalid handshake dimensions", "err", sizeErr)
		} else {
			hub.RequestResize(connCtx, subscriberID, size)
		}
	} else {
		log.Warn("[streamViaHub] handshake missing dimensions, layout may be incorrect")
	}
	return subscriberID
}

// sendHubInitialSnapshot sends an immediate initial snapshot so the client
// isn't left blank while attached — every other Transport (e.g. MuxTransport)
// relies on the hub's own CatchUpSnapshot send instead, but that send is
// suppressed for the browser path (see attachHubSubscriber) since it can't
// replicate the ANSI-sanitization, cursor-sync, and proto-envelope framing
// this handler applies here, which the browser client depends on.
//
// Sent even when content == "" (a genuinely empty pane): the client only
// clears its loading spinner on a received message with non-empty output
// bytes, and ansiSnapshotPrefix alone is non-empty, so an idle/empty session
// still reaches "connected, nothing to show" instead of spinning forever with
// no future event to ever clear it. A capture failure falls through to send
// an empty snapshot rather than skipping the send entirely, for the same
// reason (2026-09-01: skipping it left the client spinning forever with
// nothing to signal a way out).
func (h *ConnectRPCWebSocketHandler) sendHubInitialSnapshot(stream *connectWebSocketStream, instance *session.Instance, sessionID string) error {
	content, captureErr := instance.CapturePaneContentRaw()
	if captureErr != nil {
		log.Warn("[streamViaHub] failed to capture initial content, sending empty snapshot instead", "session", sessionID, "err", captureErr)
		content = ""
	}
	fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(content), instance)
	initMsg := &sessionv1.TerminalData{
		SessionId: sessionID,
		Data: &sessionv1.TerminalData_Output{
			Output: &sessionv1.TerminalOutput{Data: []byte(fullContent)},
		},
	}
	b, merr := proto.Marshal(initMsg)
	if merr != nil {
		log.Error("[streamViaHub] failed to marshal initial content", "err", merr)
		return nil
	}
	if wsErr := stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, b)); wsErr != nil {
		return fmt.Errorf("failed to send initial content: %w", wsErr)
	}
	return nil
}

// streamViaHub is streamViaControlMode's PathHubOwned counterpart (Epic 2.2,
// Story 2.2.2): instead of this connection running its own
// resize/quiescence/capture pipeline, it attaches a WebSocketTransport to the
// tmux session's single *streamhub.StreamHub (via HubRegistry's get-or-create)
// and lets the hub own resize negotiation and output fan-out.
//
// Scope note: this wires the hub path to real production traffic for the
// first time, but does not yet reproduce every refinement
// streamViaControlMode has accumulated (e.g. the ±1 resize nudge, the
// snapshot dirty-tracking cache, escape-sequence analytics). Those remain
// exclusive to the legacy path pending later phases of this plan; this
// function's job is a correct, working PathHubOwned path behind a
// default-off flag, not full feature parity yet.
func (h *ConnectRPCWebSocketHandler) streamViaHub(stream *connectWebSocketStream, instance *session.Instance) error {
	// connCtx is scoped to this WebSocket connection's lifetime, not a fixed
	// background timeout — it's threaded into every hub.RequestResize call
	// below so an upstream disconnect cancels an in-flight resize's
	// SetWindowSize/CapturePaneContentRaw pipeline immediately instead of
	// always waiting out a fixed ceiling. http.Request.Context() is not used
	// here because its post-Hijack() behavior is not well-defined for a
	// long-lived WebSocket connection.
	connCtx, cancelConn := context.WithCancel(context.Background())
	defer cancelConn()

	snap := instance.Snapshot()
	sessionID := snap.Title
	tmuxSessionName := streamHubSessionKey(snap.Title, snap.TmuxPrefix)

	log.Info("[streamViaHub] starting", "session", sessionID, "tmux", tmuxSessionName)

	instance.MarkViewed()

	currentPaneReq, err := parseControlModeHandshake(stream)
	if err != nil {
		return err
	}

	processAlive, err := h.ensureHubBackendAlive(instance, sessionID)
	if err != nil {
		return err
	}
	h.ensureHubInstanceStarted(instance, sessionID, processAlive)

	// StartControlMode is refcounted (session/tmux/control_mode.go), so each
	// concurrent connection to the same session incrementing/decrementing it
	// is safe and mirrors streamViaControlMode's own per-connection call.
	if err := instance.StartControlMode(); err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer func() {
		if err := instance.StopControlMode(); err != nil {
			log.Warn("[streamViaHub] StopControlMode error", "err", err)
		}
	}()

	hub, done, err := h.resolveHubOrJoinLegacy(stream, instance, sessionID, tmuxSessionName)
	if done {
		return err
	}

	subscriberID := h.attachHubSubscriber(connCtx, hub, hubSubscriberAttachment{
		stream:          stream,
		instance:        instance,
		sessionID:       sessionID,
		tmuxSessionName: tmuxSessionName,
		currentPaneReq:  currentPaneReq,
	})
	defer hub.DetachSubscriber(subscriberID)

	if err := h.sendHubInitialSnapshot(stream, instance, sessionID); err != nil {
		return err
	}

	doneChan := make(chan struct{})
	errChan := make(chan error, 1)
	var resizeSettling atomic.Bool

	// Epic 4.2, Story 4.2.1: push hub.SubscriberCount() to this connection
	// whenever it changes, so the frontend's ConnectionCountIndicator (Story
	// 4.2.2) can mount/unmount without depending on real terminal output
	// arriving (an idle session would otherwise never learn its peer count
	// changed). This is deliberately a side-channel poll rather than
	// stamping connection_count onto every hub-broadcast frame: the hub's
	// raw-output fan-out (StreamHub.Broadcast -> Transport.Send) passes
	// unmarshaled tmux bytes shared verbatim across all subscribers
	// (session/streamhub/hub.go's onBatchFlush / WebSocketTransport.Send),
	// so per-subscriber proto framing lives outside streamhub's and
	// WebSocketTransport's current scope — out of bounds for this UI-only
	// epic. Stops the moment this connection's read loop below returns.
	stopConnCountUpdates := make(chan struct{})
	defer close(stopConnCountUpdates)
	go sendConnectionCountUpdates(stream, hub, sessionID, stopConnCountUpdates)

	// runInputReadLoop blocks this goroutine until the WebSocket read errors
	// or an EndStream flag arrives — there is no separate output-forwarding
	// goroutine to race here (unlike streamViaControlMode) because the hub's
	// own per-subscriber writer goroutine (session/streamhub/subscriber.go)
	// already owns delivering output to this connection via transport.Send.
	runInputReadLoop(inputReadLoopParams{
		stream:    stream,
		doneChan:  doneChan,
		errChan:   errChan,
		sessionID: sessionID,
		onInput: func(data []byte) {
			if !instance.Permissions.CanSendCommand {
				log.Warn("[streamViaHub] send permission denied", "session", sessionID)
				return
			}
			instance.UpdateTerminalTimestamps(string(data), true)
			instance.MarkUserResponded()

			sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
			sendErr := instance.SendInputViaControlMode(sendCtx, data)
			sendCancel()
			if sendErr != nil {
				log.Warn("[streamViaHub] CM input failed, retrying via subprocess", "session", tmuxSessionName, "err", sendErr)
				if fbErr := sendInputToTmux(snap.TmuxServerSocket, tmuxSessionName, data); fbErr != nil {
					log.Error("[streamViaHub] subprocess fallback also failed", "session", tmuxSessionName, "err", fbErr)
				}
			}
		},
		onResize: func(cols, rows int) {
			size, sizeErr := streamhub.NewTerminalSize(cols, rows)
			if sizeErr != nil {
				log.Warn("[streamViaHub] invalid resize request", "cols", cols, "rows", rows, "err", sizeErr)
				return
			}
			hub.RequestResize(connCtx, subscriberID, size)
		},
		onScrollbackRequest: func(startLine, endLine string) (string, error) {
			return instance.GetScrollbackHistory(startLine, endLine)
		},
		onCurrentPaneRequest: func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
			return handleCurrentPaneRequest(ctx, sessionID, instance, req, currentResyncOptions())
		},
		resizeSettling: &resizeSettling,
	})

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

// parseInputFrameOrStop parses one raw WebSocket message into a TerminalData
// frame — shared by the three read loops (runShellInputReadLoop,
// runInputReadLoop, runCapturePaneInputReadLoop): envelope parsing, EndStream
// detection, and TerminalData unmarshaling are identical across all three.
// skip=true means the caller should ignore this message and keep reading;
// stop=true means the caller should stop the read loop (an error, if any, has
// already been pushed to errChan).
func parseInputFrameOrStop(errChan chan error, logPrefix string, message []byte) (incoming *sessionv1.TerminalData, skip, stop bool) {
	envelope, _, err := protocol.ParseEnvelope(message)
	if err != nil {
		log.Error(logPrefix+" failed to parse envelope", "err", err)
		return nil, true, false
	}
	if envelope.Flags&protocol.EndStreamFlag != 0 {
		errChan <- nil
		return nil, false, true
	}
	if len(envelope.Data) == 0 {
		return nil, true, false
	}

	var incomingData sessionv1.TerminalData
	if err := proto.Unmarshal(envelope.Data, &incomingData); err != nil {
		log.Error(logPrefix+" failed to unmarshal TerminalData", "err", err)
		return nil, true, false
	}
	return &incomingData, false, false
}

// dispatchResizeRequest pushes req onto a capacity-1 coalescing channel,
// draining a stale pending value and replacing it with the latest if the
// consumer hasn't caught up yet — shared by the main-terminal and shell-tab
// resize-coalescing paths (runControlModeResizeCoalescer /
// runShellControlModeResizeCoalescer).
func dispatchResizeRequest[T any](ch chan T, req T) {
	select {
	case ch <- req:
	default:
		select {
		case <-ch:
		default:
		}
		ch <- req
	}
}

// buildScrollbackResponse builds a ScrollbackResponse from raw captured
// content, shared by the main-terminal (shellID == "") and shell-tab
// scrollback paths.
func buildScrollbackResponse(sessionID, shellID, content string, offset uint64, limit int) *sessionv1.TerminalData {
	trimmed := strings.TrimRight(content, "\n")
	linesReturned := 0
	if trimmed != "" {
		linesReturned = strings.Count(trimmed, "\n") + 1
	}
	var chunks []*sessionv1.ScrollbackChunk
	if linesReturned > 0 {
		chunks = []*sessionv1.ScrollbackChunk{{Data: []byte(content)}}
	}
	return &sessionv1.TerminalData{
		SessionId: sessionID,
		ShellId:   shellID,
		Data: &sessionv1.TerminalData_ScrollbackResponse{
			ScrollbackResponse: &sessionv1.ScrollbackResponse{
				Chunks:         chunks,
				HasMore:        linesReturned >= limit,
				TotalLines:     uint64(linesReturned),
				OldestSequence: offset + uint64(linesReturned),
				NewestSequence: offset,
			},
		},
	}
}

// panePTY abstracts pane capture/resize/dimension operations so streamViaTmuxCapturePane
// can target either the main session's instance-managed PTY or a shell's sibling tmux
// session. *session.Instance already satisfies this interface natively.
type panePTY interface {
	// CapturePaneContentRaw/CapturePaneContentRawPriority — not the -J
	// "joined" CapturePaneContent/CapturePaneContentPriority — deliberately:
	// every use of this interface feeds a live xterm.js render
	// (handleCurrentPaneRequest), and -J both collapses tmux's wrap
	// structure and strips the cursor-positioning codes that render depends
	// on (see streamhub.RawPaneContent's doc comment). Not declaring the
	// joined variants here is what makes reaching for the wrong one a
	// compile error instead of a silent scrambled-reflow bug.
	CapturePaneContentRaw() (streamhub.RawPaneContent, error)
	// CapturePaneContentRawPriority/RefreshTmuxClientPriority/
	// GetPaneDimensionsPriority all take a caller-supplied ctx rather than
	// managing their own — handleCurrentPaneRequest calls several of these in
	// sequence for one resync and threads the same bounded ctx through all of
	// them, so the group shares one real, decreasing deadline instead of each
	// getting an independent fresh timeout (see
	// session/tmux/exec_gate.go's runFastLaneSubprocess doc comment for the
	// 2026-08-25 incident this closes off).
	CapturePaneContentRawPriority(ctx context.Context) (streamhub.RawPaneContent, error)
	GetPaneDimensions() (cols, rows int, err error)
	GetPaneDimensionsPriority(ctx context.Context) (cols, rows int, err error)
	ResizePTY(cols, rows int) error
	// ResizePTYContext mirrors ResizePTY but is bounded by ctx — see
	// Instance.ResizePTYContext's doc comment for why handleCurrentPaneRequest
	// must use this instead of the unbounded ResizePTY above: without it, a
	// contended resize-window call added unbounded extra latency on top of the
	// shared fast-lane budget every other call in that function respects.
	ResizePTYContext(ctx context.Context, cols, rows int) error
	RefreshTmuxClient() error
	RefreshTmuxClientPriority(ctx context.Context) error
	GetPaneCursorPosition() (x, y int, err error)
}

// fastLaneStep wraps a panePTY target so handleCurrentPaneRequest's body always reaches
// dims/resize/refresh/capture through one path instead of hand-writing an
// opts.UseFastLane if/else at each call site. That per-call-site pattern already needed
// fixing once (28a70a7a8, "share one deadline across all fast-lane calls in a resync")
// and still let the resize step slip through unbounded — see
// Instance.ResizePTYContext's doc comment. Routing every step through this one type
// means a future step added to handleCurrentPaneRequest gets ctx-boundedness for free;
// there's no second if/else to remember to write correctly.
type fastLaneStep struct {
	target   panePTY
	ctx      context.Context
	fastLane bool
}

func (s fastLaneStep) dims() (cols, rows int, err error) {
	if s.fastLane {
		return s.target.GetPaneDimensionsPriority(s.ctx)
	}
	return s.target.GetPaneDimensions()
}

func (s fastLaneStep) resize(cols, rows int) error {
	if s.fastLane {
		return s.target.ResizePTYContext(s.ctx, cols, rows)
	}
	return s.target.ResizePTY(cols, rows)
}

func (s fastLaneStep) refresh() error {
	if s.fastLane {
		return s.target.RefreshTmuxClientPriority(s.ctx)
	}
	return s.target.RefreshTmuxClient()
}

func (s fastLaneStep) capture() (streamhub.RawPaneContent, error) {
	if s.fastLane {
		return s.target.CapturePaneContentRawPriority(s.ctx)
	}
	return s.target.CapturePaneContentRaw()
}

// shellPanePTY adapts a shell's sibling *tmux.TmuxSession to the panePTY interface so
// shell tab streams target their own PTY instead of the parent session's.
type shellPanePTY struct {
	session *tmux.TmuxSession
}

func (p shellPanePTY) CapturePaneContentRaw() (streamhub.RawPaneContent, error) {
	content, err := p.session.CapturePaneContentRaw()
	return streamhub.RawPaneContent(content), err
}
func (p shellPanePTY) CapturePaneContentRawPriority(ctx context.Context) (streamhub.RawPaneContent, error) {
	content, err := p.session.CapturePaneContentRawPriority(ctx)
	return streamhub.RawPaneContent(content), err
}
func (p shellPanePTY) GetPaneDimensions() (int, int, error) { return p.session.GetPaneDimensions() }
func (p shellPanePTY) GetPaneDimensionsPriority(ctx context.Context) (int, int, error) {
	return p.session.GetPaneDimensionsPriority(ctx)
}
func (p shellPanePTY) ResizePTY(cols, rows int) error { return p.session.SetWindowSize(cols, rows) }

// ResizePTYContext mirrors Instance.ResizePTYContext's goroutine-race pattern —
// *tmux.TmuxSession.SetWindowSize has no caller-overridable context either.
func (p shellPanePTY) ResizePTYContext(ctx context.Context, cols, rows int) error {
	done := make(chan error, 1)
	go func() { done <- p.session.SetWindowSize(cols, rows) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p shellPanePTY) RefreshTmuxClient() error { return p.session.RefreshClient() }
func (p shellPanePTY) RefreshTmuxClientPriority(ctx context.Context) error {
	return p.session.RefreshClientPriority(ctx)
}
func (p shellPanePTY) GetPaneCursorPosition() (x, y int, err error) {
	return p.session.GetCursorPosition()
}

// ResyncOptions bundles per-request behavior flags for handleCurrentPaneRequest.
// It exists so Epic 4.1 (stale-dimension slow-path skip) and Epic 4.2 (exec-gate
// fast lane) can each add their flag as a named field here instead of accreting
// another positional bool parameter onto handleCurrentPaneRequest's signature —
// see the `primitive-obsession-checklist` skill. Callers resolve the
// corresponding feature flag (config.LoadConfig().GetFeatureFlag(...)) and pass
// the result in; handleCurrentPaneRequest itself stays free of feature-flag
// lookups, so unit tests can exercise both branches by constructing ResyncOptions
// directly instead of mutating global config state.
type ResyncOptions struct {
	// SkipStaleDimensionSlowPath, when true, skips the resize+SIGWINCH+verify
	// block below entirely whenever the request also has StaleDimensions set
	// (Epic 4.1, terminal:resync-skip-stale-dimension-slowpath).
	SkipStaleDimensionSlowPath bool
	// UseFastLane, when true, routes capture/refresh calls through the
	// exec-gate fast lane (Epic 4.2, terminal:resync-exec-gate-fast-lane) via
	// target.CapturePaneContentRawPriority()/RefreshTmuxClientPriority()
	// instead of the plain CapturePaneContentRaw()/RefreshTmuxClient().
	UseFastLane bool
	// EchoResyncID, when true, echoes the incoming request's ResyncId back on the
	// TerminalOutput reply (Task 3.2.1.1, terminal:resync-correlation-id). When
	// false, a request that set a resync_id gets the pre-project empty ResyncId
	// back instead.
	EchoResyncID bool
}

// currentResyncOptions resolves the feature flags handleCurrentPaneRequest's callers
// need to build a ResyncOptions, keeping the config.LoadConfig() lookups in one place
// instead of duplicated across each of handleCurrentPaneRequest's call sites.
func currentResyncOptions() ResyncOptions {
	return ResyncOptions{
		SkipStaleDimensionSlowPath: config.LoadConfig().GetFeatureFlag(terminalResyncSkipStaleDimensionSlowpathFlagName),
		UseFastLane:                config.LoadConfig().GetFeatureFlagWithDefault(terminalResyncExecGateFastLaneFlagName, featureFlagDefault(terminalResyncExecGateFastLaneFlagName)),
		EchoResyncID:               config.LoadConfig().GetFeatureFlag(terminalResyncCorrelationIDFlagName),
	}
}

// derefOr returns *p, or fallback when p is nil — used to log a *int32 request field's
// actual value instead of its pointer address.
func derefOr(p *int32, fallback int32) int32 {
	if p == nil {
		return fallback
	}
	return *p
}

// handleCurrentPaneRequest answers a CurrentPaneRequest against target: resizing the
// pane to the request's target dimensions if they differ from its current ones (with
// the existing SIGWINCH-refresh workaround and post-resize verification), then
// capturing fresh pane content. It is the single shared entry point for producing a
// TerminalOutput reply to a CurrentPaneRequest — extracted out of
// streamViaTmuxCapturePane's inline mid-stream handling so Story 3.2.2's control-mode
// dispatch (via runInputReadLoop's onCurrentPaneRequest callback) runs the identical
// dimension-check/capture logic instead of a hand-copied, divergent version.
//
// sessionID is used for logging only. opts controls per-request behavior — see
// ResyncOptions' doc comment.
//
// The returned TerminalOutput's Data is the raw captured pane content prefixed with
// ansiSnapshotPrefix (clear screen + cursor home), matching streamViaTmuxCapturePane's
// pre-existing behavior. ResyncId echoes req.GetResyncId() verbatim — empty when the
// incoming request didn't set one, never invented server-side — so the client can
// correlate this reply to the specific request that triggered it.
//
// Callers that have a cache/streamer fallback for a capture failure (as
// streamViaTmuxCapturePane does) should apply it themselves on a non-nil error; this
// helper has no such fallback since control-mode callers have no streamer to fall back to.
// resizeBeforeCaptureIfNeeded resizes the pane to req's target dimensions
// before capture, if they differ from its current ones — skipping the whole
// block when the client flags its dimensions as stale and
// opts.SkipStaleDimensionSlowPath is on (Epic 4.1, Task 4.1.1.1): resizing
// the server-side pane to match a dimension the client itself doesn't trust
// would be wrong, so capture proceeds at the pane's current dimensions instead.
func resizeBeforeCaptureIfNeeded(sessionID string, step fastLaneStep, req *sessionv1.CurrentPaneRequest, opts ResyncOptions) {
	if req.GetStaleDimensions() && opts.SkipStaleDimensionSlowPath {
		// Estimate based on the skipped block's fixed sleeps: 2x100ms inter-signal
		// delays + a 250ms post-resize settle = 450ms of gate-wait time avoided,
		// not counting the ResizePTY/RefreshTmuxClient subprocess calls themselves.
		log.ForSession(sessionID).Debug("skipping stale-dimension resize slow path",
			"sessionID", sessionID, "targetCols", derefOr(req.TargetCols, 0), "targetRows", derefOr(req.TargetRows, 0),
			"estimatedTimeSavedMs", 450)
		return
	}
	if req.TargetCols == nil || req.TargetRows == nil || *req.TargetCols <= 0 || *req.TargetRows <= 0 {
		return
	}
	targetCols := int(*req.TargetCols)
	targetRows := int(*req.TargetRows)
	if !paneDimensionsDiffer(sessionID, step, targetCols, targetRows) {
		return
	}

	if resizeErr := step.resize(targetCols, targetRows); resizeErr != nil {
		log.Error("[handleCurrentPaneRequest] failed to resize tmux before capture", "err", resizeErr)
		// Continue anyway - better to send content with wrong dimensions than no content.
		return
	}
	refreshAndVerifyResize(sessionID, step, targetCols, targetRows)
}

// paneDimensionsDiffer reports whether step's current pane dimensions differ
// from targetCols/targetRows (treating a dims() error as "differs", so the
// caller still attempts the resize), logging either way.
func paneDimensionsDiffer(sessionID string, step fastLaneStep, targetCols, targetRows int) bool {
	currentCols, currentRows, dimensionErr := step.dims()
	if dimensionErr != nil {
		log.Warn("[handleCurrentPaneRequest] failed to get current pane dimensions", "err", dimensionErr)
		return true
	}
	if currentCols == targetCols && currentRows == targetRows {
		return false
	}
	log.ForSession(sessionID).Debug("resizing tmux before capture",
		"from", fmt.Sprintf("%dx%d", currentCols, currentRows),
		"to", fmt.Sprintf("%dx%d", targetCols, targetRows))
	return true
}

// refreshAndVerifyResize sends the SIGWINCH-refresh workaround and verifies
// the resize took effect, after resizeBeforeCaptureIfNeeded's step.resize
// call has already succeeded.
func refreshAndVerifyResize(sessionID string, step fastLaneStep, targetCols, targetRows int) {
	// WORKAROUND: Send multiple SIGWINCH signals to help Claude Code detect new dimensions.
	// Claude Code has a bug where it sometimes renders wider than terminal dimensions.
	// Sending multiple refresh signals gives it multiple chances to correct itself.
	// See: https://github.com/anthropics/claude-code/issues (pending bug report)
	for i := 0; i < 3; i++ {
		if refreshErr := step.refresh(); refreshErr != nil {
			log.Warn("[handleCurrentPaneRequest] failed to send refresh signal", "signal", i+1, "err", refreshErr)
		}
		// Small delay between signals to allow processing.
		if i < 2 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// PHASE 1: INCREASED WAIT TIME - Complex UIs (Claude choice menus) need more time
	// The process needs time to receive SIGWINCH, recalculate layout, and regenerate
	// cursor positions. Increased from 150ms to 250ms to ensure even complex
	// interactive UIs have time to complete redraw.
	time.Sleep(250 * time.Millisecond)

	// PHASE 1: Verify resize succeeded before capture.
	verifiedCols, verifiedRows, verifyErr := step.dims()
	switch {
	case verifyErr != nil:
		log.Warn("[handleCurrentPaneRequest] failed to verify resize before capture", "err", verifyErr)
	case verifiedCols != targetCols || verifiedRows != targetRows:
		// Log this as critical since we're about to capture with wrong dimensions.
		log.Warn("[handleCurrentPaneRequest] CRITICAL: dimensions still mismatched after resize", "target_cols", targetCols, "target_rows", targetRows, "actual_cols", verifiedCols, "actual_rows", verifiedRows)
	default:
		log.ForSession(sessionID).Debug("resize before capture verified", "cols", verifiedCols, "rows", verifiedRows)
	}
}

func handleCurrentPaneRequest(ctx context.Context, sessionID string, target panePTY, req *sessionv1.CurrentPaneRequest, opts ResyncOptions) (*sessionv1.TerminalOutput, error) {
	log.ForSession(sessionID).Debug("current pane request",
		"targetCols", derefOr(req.TargetCols, 0), "targetRows", derefOr(req.TargetRows, 0))

	// One shared deadline for every fast-lane call this resync makes (dimension
	// check, resize, refreshes, verify, capture), not a fresh allowance per
	// call — a prior version let bounded-per-call latency still add up to far
	// more real wall-clock time than the client's stall watchdog allows
	// (2026-08-25 incident). Unused when !opts.UseFastLane.
	ctx, cancel := context.WithTimeout(ctx, tmux.ResyncFastLaneTimeout)
	defer cancel()
	step := fastLaneStep{target: target, ctx: ctx, fastLane: opts.UseFastLane}

	resizeBeforeCaptureIfNeeded(sessionID, step, req, opts)

	// Raw (unjoined) capture, not the -J "joined" variant: this content is
	// replayed straight into the client's xterm.js terminal below, which needs
	// tmux's own wrap points and cursor-positioning codes intact (see
	// prepareSnapshotContent's doc comment) — the joined variant silently
	// reflowed every resync into scrambled output.
	content, captureErr := step.capture()
	if captureErr != nil {
		return nil, fmt.Errorf("failed to capture fresh pane content: %w", captureErr)
	}
	fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(content), target)

	logFinalCaptureDiagnostics(sessionID, target, req, content)

	return &sessionv1.TerminalOutput{
		Data:     []byte(fullContent),
		ResyncId: resolveResyncID(sessionID, req, opts),
	}, nil
}

// logFinalCaptureDiagnostics logs the pane's actual dimensions after capture
// (PHASE 1 diagnostics) and flags two known-bug conditions: a resize that
// didn't take effect, and Claude Code's known width-overrun bug (UI elements
// rendering 1-2 columns wider than the terminal reports).
func logFinalCaptureDiagnostics(sessionID string, target panePTY, req *sessionv1.CurrentPaneRequest, content streamhub.RawPaneContent) {
	finalCols, finalRows, finalErr := target.GetPaneDimensions()
	if finalErr != nil {
		log.Warn("[handleCurrentPaneRequest] failed to get final dimensions after capture", "err", finalErr)
		return
	}
	log.ForSession(sessionID).Debug("captured pane content", "cols", finalCols, "rows", finalRows)
	if req.TargetCols != nil && req.TargetRows != nil {
		expectedCols := int(*req.TargetCols)
		expectedRows := int(*req.TargetRows)
		if finalCols != expectedCols || finalRows != expectedRows {
			log.Warn("[handleCurrentPaneRequest] final dimension mismatch", "captured_cols", finalCols, "captured_rows", finalRows, "expected_cols", expectedCols, "expected_rows", expectedRows)
		}
	}

	// WORKAROUND: Detect if Claude Code is rendering wider than terminal dimensions —
	// a known Claude Code bug (UI elements render 1-2 columns wider than reported).
	// Detecting this helps diagnose the issue and can inform future bug reports.
	actualWidth := detectContentWidth(string(content))
	if actualWidth > finalCols {
		log.Warn("[handleCurrentPaneRequest] CLAUDE CODE WIDTH BUG DETECTED: content rendered wider than terminal",
			"actual_width", actualWidth, "terminal_cols", finalCols, "overage", actualWidth-finalCols)
	}
}

// resolveResyncID returns req's resync_id to echo back, or "" if
// terminal:resync-correlation-id is off (opts.EchoResyncID) — a client that
// never sent one, or a deployment with the flag off, gets the pre-project
// empty ResyncId.
func resolveResyncID(sessionID string, req *sessionv1.CurrentPaneRequest, opts ResyncOptions) string {
	if opts.EchoResyncID {
		return req.GetResyncId()
	}
	if req.GetResyncId() != "" {
		// Task 7.1.1.3 (Epic 7.1 observability) — server-side equivalent of the
		// client's correlation-ID-mismatch log: the client tagged this request
		// with a resync_id, but the flag is off here so it will never be echoed
		// back. The client's own pendingResyncIdRef comparison in
		// notifyResyncOutputReceived can't detect this case (it never receives
		// an ID to compare against at all), so this is the only place the
		// dropped correlation is observable.
		log.ForSession(sessionID).Debug("resync_id not echoed: terminal:resync-correlation-id is off",
			"requestedResyncId", req.GetResyncId())
	}
	return ""
}

// inputReadLoopParams bundles runInputReadLoop's parameters — originally 9
// positional arguments (flagged as a long parameter list), grouped here into
// one config struct per Fowler's "Introduce Parameter Object".
type inputReadLoopParams struct {
	stream    *connectWebSocketStream
	doneChan  chan struct{}
	errChan   chan error
	sessionID string
	// onInput forwards one input frame's raw bytes to tmux (CM path + subprocess fallback).
	onInput func(data []byte)
	// onResize pushes a mid-stream resize request to the coalescing worker.
	onResize func(cols, rows int)
	// onScrollbackRequest performs the tmux capture for a ScrollbackRequest;
	// request validation, response construction, and writing stay in handleScrollbackRequest.
	onScrollbackRequest func(startLine, endLine string) (string, error)
	// onCurrentPaneRequest answers a mid-stream CurrentPaneRequest (as opposed to
	// the initial handshake one, which the caller parses and answers before this
	// loop starts) — see handleCurrentPaneRequestFrame.
	onCurrentPaneRequest func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error)
	resizeSettling       *atomic.Bool
}

// runInputReadLoop is the WebSocket input-read loop for streamViaControlMode
// (Goroutine 2), extracted into a standalone function so its bounded-exit
// behavior can be tested against a real WebSocket connection without a live
// tmux session (see TestRunInputReadLoopExitsPromptlyOnConnectionClose).
//
// This is a pure move of the original inline goroutine body: envelope
// parsing, EndStream detection, and TerminalData unmarshaling are unchanged.
// The two actions that depended on the enclosing closure's `instance` —
// forwarding input to tmux and pushing resize requests to the coalescing
// worker — become the onInput/onResize callback invocations.
//
// p.sessionID is required (not derivable from *connectWebSocketStream) purely
// for the WebSocket-read-error log line, which is not covered by either callback.
func runInputReadLoop(p inputReadLoopParams) {
	for {
		select {
		case <-p.doneChan:
			return
		default:
			if stop := readOneInputFrame(p); stop {
				return
			}
		}
	}
}

// readOneInputFrame reads and dispatches one WebSocket frame for
// runInputReadLoop. Returns true if the read loop should stop.
func readOneInputFrame(p inputReadLoopParams) bool {
	_, message, err := p.stream.conn.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			p.errChan <- nil
		} else {
			log.Error("[streamViaControlMode] WebSocket read error", "session", p.sessionID, "err", err)
			p.errChan <- err
		}
		return true
	}

	incomingData, skip, stop := parseInputFrameOrStop(p.errChan, "[streamViaControlMode]", message)
	if stop {
		return true
	}
	if skip {
		return false
	}
	dispatchInputReadLoopFrame(p, incomingData)
	return false
}

// dispatchInputReadLoopFrame routes one parsed TerminalData frame to its
// handler. Handling for ScrollbackRequest/CurrentPaneRequest/
// BatchedCurrentPaneRequest lives in their own handleXFrame functions, split
// out purely to keep this loop's cognitive complexity under the lint gate;
// behavior is unchanged from when they lived inline.
func dispatchInputReadLoopFrame(p inputReadLoopParams, incomingData *sessionv1.TerminalData) {
	if input := incomingData.GetInput(); input != nil {
		p.onInput(input.Data)
	}
	if resize := incomingData.GetResize(); resize != nil {
		log.Debug("[runInputReadLoop] received mid-stream resize frame", "session", p.sessionID, "cols", resize.Cols, "rows", resize.Rows)
		p.onResize(int(resize.Cols), int(resize.Rows))
	}
	if scrollbackReq := incomingData.GetScrollbackRequest(); scrollbackReq != nil {
		handleScrollbackRequest(p.stream, p.sessionID, scrollbackReq, p.onScrollbackRequest)
	}
	// CurrentPaneRequest arrives mid-stream (as opposed to the initial
	// handshake, which streamViaControlMode parses and answers separately
	// before this loop starts) — how a client-initiated resync request
	// (carrying a resync_id to correlate the reply) is answered without a full
	// reconnect.
	if paneReq := incomingData.GetCurrentPaneRequest(); paneReq != nil {
		handleCurrentPaneRequestFrame(p.stream, p.sessionID, paneReq, p.onCurrentPaneRequest, p.resizeSettling)
	}
	// BatchedCurrentPaneRequest (Epic 5.2, terminal:resync-batching): the
	// client's stagger coordinator coalesced several sibling terminals'
	// resyncs into one wire message — answer each coalesced CurrentPaneRequest
	// exactly as if it had arrived individually (see
	// handleBatchedCurrentPaneRequestFrame's doc comment for why each reply is
	// still written as its own individually-resync_id-tagged frame).
	if batchReq := incomingData.GetBatchedCurrentPaneRequest(); batchReq != nil {
		handleBatchedCurrentPaneRequestFrame(p.stream, p.sessionID, batchReq, p.onCurrentPaneRequest, p.resizeSettling)
	}
}

// handleScrollbackRequest answers a client's request for historical terminal
// scrollback, extracted out of runInputReadLoop to keep that loop's cognitive
// complexity under the lint gate (a pure move — behavior is unchanged from
// what previously lived inline in the ScrollbackRequest branch).
//
// FromSequence is treated as a line offset from the end of tmux's history:
//
//	offset=0   → capture-pane -S -(limit)   -E -1     (most recent history)
//	offset=500 → capture-pane -S -(500+limit) -E -501 (next page back)
//
// Uses -J to join tmux soft-wrapped lines, making content width-agnostic so
// it re-wraps correctly in xterm.js after a terminal resize.
func handleScrollbackRequest(
	stream *connectWebSocketStream,
	sessionID string,
	scrollbackReq *sessionv1.ScrollbackRequest,
	onScrollbackRequest func(startLine, endLine string) (string, error),
) {
	const maxScrollbackLimit = 1000
	limit := int(scrollbackReq.Limit)
	if limit <= 0 || limit > maxScrollbackLimit {
		limit = maxScrollbackLimit
	}
	offset := scrollbackReq.FromSequence

	startLine := fmt.Sprintf("-%d", offset+uint64(limit))
	endLine := fmt.Sprintf("-%d", offset+1)
	content, sbErr := onScrollbackRequest(startLine, endLine)
	if sbErr != nil {
		log.Warn("[streamViaControlMode] ScrollbackRequest tmux capture failed", "session", sessionID, "err", sbErr)
		return
	}

	sbResp := buildScrollbackResponse(sessionID, "", content, offset, limit)
	respBytes, merr := proto.Marshal(sbResp)
	if merr != nil {
		log.Error("[streamViaControlMode] failed to marshal scrollback response", "session", sessionID, "err", merr)
		return
	}
	_ = stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, respBytes))
}

// handleCurrentPaneRequestFrame answers a CurrentPaneRequest that arrives mid-stream on
// runInputReadLoop (as opposed to the initial handshake one), delegating the actual
// resize/capture work to onCurrentPaneRequest (backed by handleCurrentPaneRequest) and
// handling only response marshaling/writing here — mirroring handleScrollbackRequest's
// split between "generic frame plumbing" and "actual capture logic."
func handleCurrentPaneRequestFrame(
	stream *connectWebSocketStream,
	sessionID string,
	paneReq *sessionv1.CurrentPaneRequest,
	onCurrentPaneRequest func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error),
	resizeSettling *atomic.Bool,
) {
	ctx, span := telemetry.StartSpan(context.Background(), "terminal.mid_stream_current_pane_request")
	span.SetAttributes(
		attribute.String("session_id", sessionID),
		attribute.String("resync_id", paneReq.GetResyncId()),
	)
	defer span.End()

	log.Debug("[runInputReadLoop] received mid-stream currentPaneRequest frame", "session", sessionID, "resync_id", paneReq.GetResyncId())

	resizeSettling.Store(true)
	defer resizeSettling.Store(false)
	// Threading ctx (not context.Background()) is what makes the exec-gate
	// spans/metrics below nest under this request's own span in Tempo instead
	// of appearing as unrelated root spans — see exec_gate_observability.go's
	// doc comment for the incident this closes.
	output, err := onCurrentPaneRequest(ctx, paneReq)
	if err != nil {
		span.RecordError(err)
		log.Error("[streamViaControlMode] failed to handle mid-stream current pane request", "session", sessionID, "resync_id", paneReq.GetResyncId(), "err", err)
		return
	}
	span.SetAttributes(attribute.Int("output.bytes", len(output.GetData())))
	log.Debug("[runInputReadLoop] answered mid-stream currentPaneRequest", "session", sessionID, "resync_id", paneReq.GetResyncId(), "output_bytes", len(output.GetData()))
	writeCurrentPaneResponse(stream, sessionID, "", output)
}

// terminalResyncCompressionThresholdBytes is the marshaled-payload size above which
// writeCurrentPaneResponse gzip-compresses a resync reply when terminal:resync-compression
// is on (Task 5.1.1.1) — below this size the gzip container overhead outweighs any wire
// savings, matching protocol.CompressEnvelopeIfLarge's own threshold contract.
const terminalResyncCompressionThresholdBytes = 1024

// writeCurrentPaneResponse marshals a single CurrentPaneRequest's TerminalOutput reply
// (already resync_id-tagged by handleCurrentPaneRequest) into its own TerminalData
// envelope and writes it to the stream. Split out of handleCurrentPaneRequestFrame so
// handleBatchedCurrentPaneRequestFrame can reuse the identical per-response wire
// encoding for each coalesced request's reply, without duplicating the marshal/write
// logic.
//
// When terminal:resync-compression is on and the marshaled payload exceeds
// terminalResyncCompressionThresholdBytes, the payload is gzip-compressed via
// protocol.CompressEnvelopeIfLarge and the envelope's CompressedFlag bit is set so the
// client's websocket-transport.ts decompresses it before proto-unmarshaling (Epic 5.1). A
// compression failure falls back to sending the original, uncompressed payload rather than
// dropping the reply — a resync reply is worth more than the wire-size savings.
//
// shellID is set on the outgoing TerminalData when the reply is for a shell tab's own
// stream (streamShellViaControlMode); pass "" for the main session's stream, which never
// tags ShellId (matching handleCurrentPaneRequestFrame/handleBatchedCurrentPaneRequestFrame
// and streamViaTmuxCapturePane's pre-existing behavior).
func writeCurrentPaneResponse(stream *connectWebSocketStream, sessionID string, shellID string, output *sessionv1.TerminalOutput) {
	terminalData := &sessionv1.TerminalData{
		SessionId: sessionID,
		ShellId:   shellID,
		Data: &sessionv1.TerminalData_Output{
			Output: output,
		},
	}
	respBytes, merr := proto.Marshal(terminalData)
	if merr != nil {
		log.Error("[streamViaControlMode] failed to marshal current pane response", "session", sessionID, "err", merr)
		return
	}

	envelopeFlags := byte(0)
	payload := respBytes
	if config.LoadConfig().GetFeatureFlag(terminalResyncCompressionFlagName) {
		compressed, wasCompressed, compressErr := protocol.CompressEnvelopeIfLarge(respBytes, terminalResyncCompressionThresholdBytes)
		if compressErr != nil {
			log.ForSession(sessionID).Warn("failed to compress current pane response, sending uncompressed", "err", compressErr)
		} else if wasCompressed {
			payload = compressed
			envelopeFlags |= protocol.CompressedFlag
		}
	}

	if wsErr := stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(envelopeFlags, payload)); wsErr != nil {
		log.Error("[streamViaControlMode] failed to write current pane response", "session", sessionID, "err", wsErr)
	}
}

// handleBatchedCurrentPaneRequest answers each CurrentPaneRequest coalesced inside a
// BatchedCurrentPaneRequest (Epic 5.2, terminal:resync-batching) by calling
// onCurrentPaneRequest for each one in order — the same callback a lone, unbatched
// CurrentPaneRequest would use (backed by handleCurrentPaneRequest) — and collecting
// the individually-resync_id-tagged TerminalOutput replies.
//
// Batching is purely a client-side coalescing decision (the stagger coordinator groups
// same-tick sibling resyncs when terminal:resync-batching is on — see ADR-006); the
// server does not re-check the flag here and answers whatever BatchedCurrentPaneRequest
// arrives on the wire. A single coalesced request's failure is logged and skipped
// (matching handleCurrentPaneRequestFrame's per-request error handling) rather than
// aborting the rest of the batch, so one bad sibling can't drop replies the others are
// waiting on.
//
// Go/no-go (Task 5.2.1.4): terminal:resync-batching defaults to off, and the decision
// to ever recommend flipping it on by default is deliberately NOT made by this story —
// it's deferred until Epic 5.1's compression benchmark (Task 5.1.1.2) produces real
// wire-size numbers to compare batching's round-trip savings against, per
// requirements.md's Unresolved Question #1 and ADR-006's Consequences section. This
// handler exists so the flag is exercisable and testable now, not so it can be judged
// ready for default-on.
//
// Per-request resync_id correlation is preserved by construction: onCurrentPaneRequest
// echoes req.GetResyncId() verbatim (see handleCurrentPaneRequest's doc comment), and
// this function never merges or reorders outputs across requests — output[i] always
// answers requests[i].
func handleBatchedCurrentPaneRequest(
	sessionID string,
	batchReq *sessionv1.BatchedCurrentPaneRequest,
	onCurrentPaneRequest func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error),
) []*sessionv1.TerminalOutput {
	requests := batchReq.GetRequests()
	// One span for the whole coalesced batch (not one per request): the point of
	// batching is that these requests were coalesced into a single wire message,
	// so their exec-gate spans/metrics nest under one parent here — see
	// handleCurrentPaneRequestFrame's identical treatment for the unbatched case.
	ctx, span := telemetry.StartSpan(context.Background(), "terminal.batched_mid_stream_current_pane_request")
	span.SetAttributes(attribute.String("session_id", sessionID), attribute.Int("request_count", len(requests)))
	defer span.End()

	outputs := make([]*sessionv1.TerminalOutput, 0, len(requests))
	for _, paneReq := range requests {
		output, err := onCurrentPaneRequest(ctx, paneReq)
		if err != nil {
			span.RecordError(err)
			log.Error("[streamViaControlMode] failed to handle coalesced current pane request", "session", sessionID, "resyncId", paneReq.GetResyncId(), "err", err)
			continue
		}
		outputs = append(outputs, output)
	}
	return outputs
}

// handleBatchedCurrentPaneRequestFrame answers a BatchedCurrentPaneRequest that arrives
// mid-stream: it dispatches every coalesced request via handleBatchedCurrentPaneRequest,
// then writes each reply to the stream as its own separate TerminalData/TerminalOutput
// frame — never combined into one response — via writeCurrentPaneResponse, so each
// reply still carries only its own request's resync_id for the client to correlate.
func handleBatchedCurrentPaneRequestFrame(
	stream *connectWebSocketStream,
	sessionID string,
	batchReq *sessionv1.BatchedCurrentPaneRequest,
	onCurrentPaneRequest func(ctx context.Context, req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error),
	resizeSettling *atomic.Bool,
) {
	resizeSettling.Store(true)
	defer resizeSettling.Store(false)
	for _, output := range handleBatchedCurrentPaneRequest(sessionID, batchReq, onCurrentPaneRequest) {
		writeCurrentPaneResponse(stream, sessionID, "", output)
	}
}

// capturePaneTarget bundles streamViaTmuxCapturePane's resolved routing
// state: which tmux session and panePTY to target, and whether this
// connection should be treated as a managed (has-its-own-live-PTY) stream.
type capturePaneTarget struct {
	sessionID        string
	tmuxSessionName  string
	target           panePTY
	effectiveManaged bool
	isShellStream    bool
}

// resolveCapturePaneTarget derives streamViaTmuxCapturePane's tmux session
// name and capture/resize target from instance and shellTmuxSessionName ("" for
// the main terminal). A shell tab targets its own sibling tmux session
// directly (never the parent's) and, since it has its own live PTY, is
// treated like a managed session for capture/resize/redraw purposes even when
// the parent Instance isn't managed.
func resolveCapturePaneTarget(instance *session.Instance, snap *session.InstanceSnapshot, shellTmuxSessionName string) capturePaneTarget {
	isShellStream := shellTmuxSessionName != ""

	var tmuxSessionName string
	switch {
	case isShellStream:
		tmuxSessionName = shellTmuxSessionName
	case snap.ExternalMetadata != nil && snap.ExternalMetadata.TmuxSessionName != "":
		tmuxSessionName = snap.ExternalMetadata.TmuxSessionName
	default:
		// Always via the canonical sanitizer (see #162 — raw concatenation targets
		// a session name that was never actually created whenever the title has spaces).
		tmuxSessionName = streamHubSessionKey(snap.Title, snap.TmuxPrefix)
	}

	// target is where pane capture/resize/dimension calls are actually sent: the
	// parent Instance's own PTY for the main terminal, or the shell's sibling
	// tmux session for a shell tab stream — without this, every call would stay
	// bound to the parent Instance and shell tabs would duplicate its content.
	var target panePTY = instance
	if isShellStream {
		target = shellPanePTY{session: tmux.NewTmuxSessionFromExisting(shellTmuxSessionName)}
	}

	return capturePaneTarget{
		sessionID:        snap.Title,
		tmuxSessionName:  tmuxSessionName,
		target:           target,
		effectiveManaged: isShellStream || snap.IsManaged,
		isShellStream:    isShellStream,
	}
}

// forceCapturePaneRedrawNudge parses the handshake's target dimensions (if
// present) and forces a TUI redraw via a ±1 resize nudge, so the initial
// capture-pane snapshot below reflects a freshly-drawn terminal state. Only
// called for managed/shell targets, which have a live PTY to nudge.
func forceCapturePaneRedrawNudge(stream *connectWebSocketStream, target panePTY) {
	var handshakeCaptureData sessionv1.TerminalData
	if err := proto.Unmarshal(stream.requestMsg, &handshakeCaptureData); err != nil {
		return
	}
	paneReq := handshakeCaptureData.GetCurrentPaneRequest()
	if paneReq == nil || paneReq.TargetCols == nil || paneReq.TargetRows == nil {
		return
	}
	targetCols := int(*paneReq.TargetCols)
	targetRows := int(*paneReq.TargetRows)

	// Skip the nudge when the pane is already at the requested size — this is the
	// common case for a reconnect (e.g. tab regains focus after a dropped idle
	// websocket) against a pane whose viewport never changed. Nudging
	// unconditionally sends a real SIGWINCH on every reconnect, which makes
	// readline-based shells (zsh/bash) redraw and re-echo the in-progress input
	// line into the pane's scrollback — visible as a duplicated command line
	// even though nothing was actually retyped.
	actualCols, actualRows, dimErr := target.GetPaneDimensions()
	if dimErr == nil && actualCols == targetCols && actualRows == targetRows {
		log.Info("[streamViaTmuxCapture] skipping redraw nudge, pane already at target size", "cols", targetCols, "rows", targetRows)
		return
	}

	log.Info("[streamViaTmuxCapture] forcing redraw via nudge", "cols", targetCols, "rows", targetRows)
	if targetCols > 1 {
		if resizeErr := target.ResizePTY(targetCols-1, targetRows); resizeErr == nil {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if resizeErr := target.ResizePTY(targetCols, targetRows); resizeErr == nil {
		time.Sleep(200 * time.Millisecond)
		log.Info("[streamViaTmuxCapture] redraw complete", "cols", targetCols, "rows", targetRows)
	}
}

// sendCapturePaneInitialContent captures (for a managed/shell target, falling
// back to the streamer's cached snapshot on failure) or fetches from the
// streamer (for an external session) the initial pane content, and sends it
// to the client as the canonical initial snapshot.
func sendCapturePaneInitialContent(stream *connectWebSocketStream, instance *session.Instance, cpt capturePaneTarget, streamer *session.ExternalTmuxStreamer) error {
	var initialContent string
	switch {
	case cpt.effectiveManaged:
		if freshContent, captureErr := cpt.target.CapturePaneContentRaw(); captureErr == nil {
			initialContent = string(freshContent)
		} else {
			log.Info("[streamViaTmuxCapture] fresh capture failed, falling back to cached", "err", captureErr)
			initialContent = streamer.GetContent()
		}
	default:
		initialContent = streamer.GetContent()
	}
	if initialContent == "" {
		return nil
	}

	fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(streamhub.RawPaneContent(initialContent)), cpt.target)
	terminalData := newTerminalOutputData(cpt.sessionID, fullContent)

	dataBytes, err := proto.Marshal(terminalData)
	if err != nil {
		return fmt.Errorf("failed to marshal initial content: %w", err)
	}
	if err := stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, dataBytes)); err != nil {
		return fmt.Errorf("failed to send initial content: %w", err)
	}

	log.Info("[streamViaTmuxCapture] sent initial content", "bytes", len(initialContent), "session", cpt.sessionID)
	instance.UpdateTerminalTimestamps(initialContent, true)
	return nil
}

// getOrCreateTmuxStreamer looks up or creates the *ExternalTmuxStreamer for
// tmuxSessionName.
func (h *ConnectRPCWebSocketHandler) getOrCreateTmuxStreamer(tmuxSessionName string) (*session.ExternalTmuxStreamer, error) {
	if h.tmuxStreamerManager == nil {
		return nil, fmt.Errorf("tmux streamer manager not configured (required for capture-pane polling)")
	}
	streamer, err := h.tmuxStreamerManager.GetOrCreate(tmuxSessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to create tmux streamer for '%s': %w", tmuxSessionName, err)
	}
	return streamer, nil
}

// registerCapturePaneConsumer registers a consumer with streamer that
// forwards each full-content update onto a buffered channel (dropping
// content if the channel is full, to avoid blocking the streamer), and
// returns that channel plus the consumer key for later RemoveConsumer.
func registerCapturePaneConsumer(instance *session.Instance, streamer *session.ExternalTmuxStreamer, sessionID string) (outputChan chan string, consumerKey string) {
	outputChan = make(chan string, 100)
	consumer := func(content string) {
		instance.UpdateTerminalTimestamps(content, true)
		select {
		case outputChan <- content:
		default:
			log.Warn("[streamViaTmuxCapture] output channel full, dropping content", "session", sessionID)
		}
	}
	return outputChan, streamer.AddConsumer(consumer)
}

// capturePaneStreamParams bundles the per-connection state
// streamViaTmuxCapturePane's extracted goroutines need.
type capturePaneStreamParams struct {
	stream              *connectWebSocketStream
	instance            *session.Instance
	snap                *session.InstanceSnapshot
	cpt                 capturePaneTarget
	streamer            *session.ExternalTmuxStreamer
	doneChan            chan struct{}
	errChan             chan error
	outputChan          chan string
	paneCaptureSettling *atomic.Bool
}

// forwardCapturePaneOutput is streamViaTmuxCapturePane's Goroutine 1: forwards
// full-snapshot tmux capture-pane polls to the WebSocket, dropping ticks while
// a mid-stream CurrentPaneRequest is writing its own authoritative snapshot
// (paneCaptureSettling) so the two writers can never interleave on the stream.
func forwardCapturePaneOutput(p capturePaneStreamParams) {
	defer close(p.doneChan)

	log.Info("[streamViaTmuxCapture] output goroutine started", "session", p.cpt.sessionID)

	for {
		select {
		case <-p.doneChan:
			return
		case content := <-p.outputChan:
			if p.paneCaptureSettling.Load() {
				continue
			}
			// Since tmux capture-pane returns full snapshots, prepend a clear-screen.
			// Must go through the same sanitize+CRLF-normalize treatment as the initial
			// snapshot (prepareSnapshotContent) — capture-pane's bare LFs and any
			// leftover absolute-positioning/clear codes are otherwise replayed raw into
			// xterm.js on every poll tick, producing "messed up" staircased/garbled
			// rendering for shell tabs.
			fullContent := withCursorSync(ansiSnapshotPrefix+prepareSnapshotContent(streamhub.RawPaneContent(content)), p.cpt.target)

			terminalData := terminalDataPool.Get().(*sessionv1.TerminalData)
			terminalData.SessionId = p.cpt.sessionID
			terminalData.Data = &sessionv1.TerminalData_Output{
				Output: &sessionv1.TerminalOutput{Data: []byte(fullContent)},
			}
			sendErr := marshalProtoEnvelope(p.stream, 0, terminalData)
			proto.Reset(terminalData)
			terminalDataPool.Put(terminalData)
			if sendErr != nil {
				log.Error("[streamViaTmuxCapture] failed to send output", "err", sendErr)
				p.errChan <- sendErr
				return
			}
		}
	}
}

// runCapturePaneInputReadLoop is streamViaTmuxCapturePane's Goroutine 2: reads
// and dispatches WebSocket input/resize/CurrentPaneRequest frames.
func runCapturePaneInputReadLoop(p capturePaneStreamParams) {
	for {
		select {
		case <-p.doneChan:
			return
		default:
			if stop := readOneCapturePaneFrame(p); stop {
				return
			}
		}
	}
}

// readOneCapturePaneFrame reads and dispatches one WebSocket frame for
// runCapturePaneInputReadLoop. Returns true if the read loop should stop.
//
// Blocking read with no deadline: gorilla/websocket poisons the whole Conn on
// the first read error of any kind (including a deadline timeout) — every
// later ReadMessage call returns that same stale error without doing I/O, and
// after 1000 such calls it panics with "repeated read on failed websocket
// connection". A rolling SetReadDeadline + "continue on timeout" loop
// therefore busy-loops into that panic within moments of the first idle
// timeout. The client only sends input/resize messages on demand, so blocking
// indefinitely here is correct; the outer caller closes stream.conn once this
// function returns (see streamViaControlMode for the same pattern), which
// unblocks this call if it's still pending.
func readOneCapturePaneFrame(p capturePaneStreamParams) bool {
	_, message, err := p.stream.conn.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			p.errChan <- nil
		} else {
			p.errChan <- fmt.Errorf("failed to read from WebSocket: %w", err)
		}
		return true
	}

	incomingData, skip, stop := parseInputFrameOrStop(p.errChan, "[streamViaTmuxCapture]", message)
	if stop {
		return true
	}
	if skip {
		return false
	}

	if input := incomingData.GetInput(); input != nil {
		handleCapturePaneInput(p, input.Data)
	}
	if resize := incomingData.GetResize(); resize != nil {
		handleCapturePaneResize(p, int(resize.Cols), int(resize.Rows))
	}
	if currentPaneReq := incomingData.GetCurrentPaneRequest(); currentPaneReq != nil {
		handleCapturePaneCurrentPaneRequest(p, currentPaneReq)
	}
	return false
}

// handleCapturePaneInput sends one input frame to tmux via send-keys.
func handleCapturePaneInput(p capturePaneStreamParams, data []byte) {
	// Check send permission (snap captured at stream start; Permissions is immutable).
	if !p.snap.Permissions.CanSendCommand {
		log.Warn("[streamViaTmuxCapture] send permission denied", "session", p.cpt.sessionID)
		return
	}
	p.instance.UpdateTerminalTimestamps(string(data), true)

	// Errors are non-fatal (stream stays alive). Retry on failure (exec-gate
	// contention or a transient tmux error can otherwise silently drop
	// keystrokes with no client-visible signal).
	if err := sendInputToTmuxWithRetry(p.snap.TmuxServerSocket, p.cpt.tmuxSessionName, data); err != nil {
		log.Warn("[streamViaTmuxCapture] error sending input to tmux after retries", "tmux_session", p.cpt.tmuxSessionName, "err", err)
	}
}

// handleCapturePaneResize resizes the pane using the method appropriate to
// the session type: PTY resize for a managed/shell target, or best-effort
// tmux resize-window/resize-pane commands for an external session (which may
// be attached to other terminals that control the actual size).
func handleCapturePaneResize(p capturePaneStreamParams, targetCols, targetRows int) {
	log.ForSession(p.cpt.sessionID).Debug("resize request", "cols", targetCols, "rows", targetRows)
	if p.cpt.effectiveManaged {
		resizeManagedCapturePaneTarget(p, targetCols, targetRows)
	} else {
		resizeExternalCapturePaneSession(p, targetCols, targetRows)
	}
}

// resizeManagedCapturePaneTarget uses the proper PTY resize method (ioctl,
// signal propagation, tmux window resizing) and verifies it took effect.
func resizeManagedCapturePaneTarget(p capturePaneStreamParams, targetCols, targetRows int) {
	if err := p.cpt.target.ResizePTY(targetCols, targetRows); err != nil {
		log.Warn("[streamViaTmuxCapture] failed to resize managed session", "session", p.cpt.sessionID, "err", err)
		return
	}
	actualCols, actualRows, verifyErr := p.cpt.target.GetPaneDimensions()
	if verifyErr != nil {
		log.Warn("[streamViaTmuxCapture] failed to verify resize", "session", p.cpt.sessionID, "err", verifyErr)
	} else if actualCols != targetCols || actualRows != targetRows {
		log.Warn("[streamViaTmuxCapture] dimension mismatch after resize", "session", p.cpt.sessionID, "target_cols", targetCols, "target_rows", targetRows, "actual_cols", actualCols, "actual_rows", actualRows)
	} else {
		log.ForSession(p.cpt.sessionID).Debug("resize verified", "cols", actualCols, "rows", actualRows)
	}
}

// resizeExternalCapturePaneSession best-effort resizes an external session's
// tmux window and pane via tmux commands, then verifies the result.
func resizeExternalCapturePaneSession(p capturePaneStreamParams, targetCols, targetRows int) {
	socket := p.snap.TmuxServerSocket
	runResize := func(cmd string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		args := tmux.ResolveSocket(socket).Args(cmd, "-t", p.cpt.tmuxSessionName,
			"-x", fmt.Sprintf("%d", targetCols), "-y", fmt.Sprintf("%d", targetRows))
		return runTmuxGatedErr(ctx, socket, func() error {
			return safeexec.CommandContext(ctx, tmux.Binary(), args...).Run()
		})
	}
	if err := runResize("resize-window"); err != nil {
		log.Warn("[streamViaTmuxCapture] failed to resize tmux window for external session", "tmux_session", p.cpt.tmuxSessionName, "err", err)
	}
	if err := runResize("resize-pane"); err != nil {
		log.Warn("[streamViaTmuxCapture] failed to resize tmux pane for external session", "tmux_session", p.cpt.tmuxSessionName, "err", err)
	}

	actualCols, actualRows, verifyErr := p.instance.GetPaneDimensions()
	if verifyErr != nil {
		log.Warn("[streamViaTmuxCapture] failed to verify external resize", "session", p.cpt.sessionID, "err", verifyErr)
	} else if actualCols != targetCols || actualRows != targetRows {
		log.Warn("[streamViaTmuxCapture] external dimension mismatch", "session", p.cpt.sessionID, "target_cols", targetCols, "target_rows", targetRows, "actual_cols", actualCols, "actual_rows", actualRows)
	} else {
		log.ForSession(p.cpt.sessionID).Debug("external resize verified", "cols", actualCols, "rows", actualRows)
	}
}

// handleCapturePaneCurrentPaneRequest answers a mid-stream CurrentPaneRequest
// via the shared handleCurrentPaneRequest helper, falling back to the
// streamer's cached content on failure (handleCurrentPaneRequest itself has
// no streamer to fall back to).
func handleCapturePaneCurrentPaneRequest(p capturePaneStreamParams, req *sessionv1.CurrentPaneRequest) {
	p.paneCaptureSettling.Store(true)
	defer p.paneCaptureSettling.Store(false)

	paneCtx, paneSpan := telemetry.StartSpan(context.Background(), "terminal.mid_stream_current_pane_request")
	defer paneSpan.End()
	paneSpan.SetAttributes(
		attribute.String("session_id", p.cpt.sessionID),
		attribute.String("resync_id", req.GetResyncId()),
	)
	output, handleErr := handleCurrentPaneRequest(paneCtx, p.cpt.sessionID, p.cpt.target, req, currentResyncOptions())
	if handleErr != nil {
		paneSpan.RecordError(handleErr)
		log.Error("[streamViaTmuxCapture] failed to capture fresh pane content", "err", handleErr)
		output = &sessionv1.TerminalOutput{
			Data:     []byte(ansiSnapshotPrefix + p.streamer.GetContent()),
			ResyncId: req.GetResyncId(),
		}
	}

	writeCurrentPaneResponse(p.stream, p.cpt.sessionID, "", output)
	log.ForSession(p.cpt.sessionID).Debug("sent pane content", "bytes", len(output.Data))
}

// streamViaTmuxCapturePane handles WebSocket streaming using tmux capture-pane polling.
// This is the correct method for ALL tmux sessions (both managed and external) because:
// 1. PTY-based streaming doesn't work for tmux (reads from "tmux attach" PTY, not the actual process)
// 2. Tmux capture-pane provides reliable access to the terminal buffer
// 3. Works identically for managed sessions (prefix "staplersquad_<name>") and external sessions
//
// This function polls tmux's pane buffer at regular intervals and sends content deltas to clients.
func (h *ConnectRPCWebSocketHandler) streamViaTmuxCapturePane(stream *connectWebSocketStream, instance *session.Instance, shellTmuxSessionName string) error {
	// Lock-free snapshot for all direct Instance field reads in this handler.
	// Method calls (MarkViewed, ResizePTY, etc.) and write paths are left as-is.
	snap := instance.Snapshot()
	cpt := resolveCapturePaneTarget(instance, snap, shellTmuxSessionName)
	sessionID := cpt.sessionID
	tmuxSessionName := cpt.tmuxSessionName
	target := cpt.target
	effectiveManaged := cpt.effectiveManaged

	log.Info("[streamViaTmuxCapture] starting", "session", sessionID, "tmux", tmuxSessionName, "managed", snap.IsManaged, "shell", cpt.isShellStream)

	streamer, err := h.getOrCreateTmuxStreamer(tmuxSessionName)
	if err != nil {
		return err
	}

	// Update LastViewed timestamp - user is viewing this session
	instance.MarkViewed()
	log.Info("updated LastViewed timestamp for external session", "session", sessionID)

	if effectiveManaged {
		forceCapturePaneRedrawNudge(stream, target)
	}

	if err := sendCapturePaneInitialContent(stream, instance, cpt, streamer); err != nil {
		return err
	}

	// Create channels for goroutine coordination
	errChan := make(chan error, 2)
	doneChan := make(chan struct{})

	// paneCaptureSettling mirrors resizeSettling in streamViaControlMode/
	// streamShellViaControlMode: it suppresses Goroutine 1's poll-forwarded frames while
	// a mid-stream CurrentPaneRequest's authoritative handleCurrentPaneRequest snapshot is
	// being captured and written, so the two writers can never interleave on the stream.
	var paneCaptureSettling atomic.Bool

	outputChan, consumerKey := registerCapturePaneConsumer(instance, streamer, sessionID)
	defer streamer.RemoveConsumer(consumerKey)

	cps := capturePaneStreamParams{
		stream:              stream,
		instance:            instance,
		snap:                snap,
		cpt:                 cpt,
		streamer:            streamer,
		doneChan:            doneChan,
		errChan:             errChan,
		outputChan:          outputChan,
		paneCaptureSettling: &paneCaptureSettling,
	}
	go forwardCapturePaneOutput(cps)
	go runCapturePaneInputReadLoop(cps)

	// Wait for either goroutine to complete or error.
	// EndStream is sent by the caller (HandleWebSocket) after this function returns.
	err = <-errChan

	log.Info("[streamViaTmuxCapture] connection closed", "session", sessionID)
	return err
}

// sendInputToTmux sends input bytes to a tmux session using tmux send-keys.
// Each byte is sent individually using -H (hex) format to handle special characters properly.
// serverSocket must be the same socket the target session's tmux server is bound to
// (e.g. instance.TmuxServerSocket) — an empty string targets the default socket.
// Without routing through ResolveSocket/Args here, send-keys unconditionally hits the
// default tmux server, silently missing any session running on an isolated socket.
func sendInputToTmux(serverSocket, tmuxSessionName string, data []byte) error {
	// Build send-keys command with hex-encoded bytes
	// Using -H flag to send hex bytes, which handles all special characters correctly
	baseArgs := make([]string, 0, 4+len(data))
	baseArgs = append(baseArgs, "send-keys", "-t", tmuxSessionName, "-H")
	for _, b := range data {
		baseArgs = append(baseArgs, fmt.Sprintf("%02x", b))
	}
	args := tmux.ResolveSocket(serverSocket).Args(baseArgs...)

	gateCtx, gateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer gateCancel()
	err := runTmuxInputGatedErr(gateCtx, serverSocket, func() error {
		// Use a fresh context for the command itself, not gateCtx. Gate
		// acquisition can consume most of gateCtx's budget under contention,
		// leaving the exec below racing an already-expiring deadline: if
		// tmux send-keys reaches the server just before cancellation kills
		// the client process, the caller sees an error and retries
		// (sendInputToTmuxWithRetry), re-sending keystrokes that already
		// landed — producing duplicated input in the pane. A fresh timeout
		// here guarantees the command always gets its full run budget.
		cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cmdCancel()
		return safeexec.CommandContext(cmdCtx, tmux.Binary(), args...).Run()
	})
	if err != nil {
		return fmt.Errorf("tmux send-keys failed: %w", err)
	}
	return nil
}

// sendInputToTmuxInputRetries bounds how many times sendInputToTmux is retried
// on failure before the input is given up on. Failures here are almost always
// transient exec-gate contention (the 5s acquire timeout in sendInputToTmux
// expiring under concurrent tmux subprocess load) rather than a real tmux
// error, so a short bounded retry recovers keystrokes that would otherwise be
// silently dropped with no client-visible signal.
const sendInputToTmuxInputRetries = 2

// sendInputToTmuxRetryBackoff is the delay between retries of sendInputToTmux.
const sendInputToTmuxRetryBackoff = 100 * time.Millisecond

// sendInputToTmuxWithRetry calls sendInputToTmux, retrying a bounded number of
// times on failure. See sendInputToTmuxInputRetries for why this exists.
func sendInputToTmuxWithRetry(serverSocket, tmuxSessionName string, data []byte) error {
	var err error
	for attempt := 0; attempt <= sendInputToTmuxInputRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(sendInputToTmuxRetryBackoff)
		}
		if err = sendInputToTmux(serverSocket, tmuxSessionName, data); err == nil {
			return nil
		}
	}
	return err
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

// runTmuxInputGatedErr acquires a tmux input-fast-lane exec-gate slot for
// serverSocket (bounded by ctx), runs fn, then releases the slot. Used
// specifically for keystroke input traffic (the legacy per-keystroke
// send-keys path, sendInputToTmux): it draws from tmux.AcquireInputExecSlot's
// pool, separate from the shared default pool runTmuxGatedErr uses, so user
// input never queues behind a background poller's capture-pane calls on the
// same tmux server.
func runTmuxInputGatedErr(ctx context.Context, serverSocket string, fn func() error) error {
	release, err := tmux.AcquireInputExecSlot(ctx, serverSocket)
	if err != nil {
		return fmt.Errorf("input exec gate: %w", err)
	}
	defer release()
	return fn()
}

// parseConnectHeaders parses HTTP headers from ConnectRPC format (key: value\r\n)
func parseConnectHeaders(headersText string) map[string]string {
	headers := make(map[string]string)
	lines := strings.Split(strings.TrimSpace(headersText), "\r\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			headers[parts[0]] = parts[1]
		}
	}

	return headers
}

// sendErrorResponse sends an error response over WebSocket
func sendErrorResponse(conn *websocket.Conn, errorMsg string) {
	responseHeaders := fmt.Sprintf("Status-Code: 500\r\nContent-Type: text/plain\r\n\r\n%s", errorMsg)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(responseHeaders)); err != nil {
		log.Error("failed to send error response headers", "err", err)
	}
}

// sendEndStreamSuccess sends a successful EndStream message
func sendEndStreamSuccess(stream *connectWebSocketStream) {
	// ConnectRPC protocol requires JSON-encoded EndStream payload (not protobuf)
	// Success EndStream is an empty JSON object
	dataBytes := []byte(`{}`)

	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, dataBytes)
	if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
		// "close sent" means the WebSocket was already closing — benign race on disconnect.
		if strings.Contains(err.Error(), "close sent") {
			log.Info("EndStreamSuccess skipped — websocket already closing")
		} else {
			log.Error("failed to send EndStreamSuccess", "err", err)
		}
	}
}

// endStreamErrorCode picks the ConnectRPC error code string for an
// EndStream error frame. FailedPrecondition for a missing working directory
// (see handleTmuxRestoreFailure) is deliberately distinct from the default
// Internal every other stream error gets: the frontend
// (useTerminalStream.ts's isWorktreeMissingError) matches on this code to
// stop retrying immediately instead of burning its whole reconnect budget
// against a directory that won't reappear on its own. Extracted from
// sendEndStreamError as a pure function so this selection is unit-testable
// without a real WebSocket stream.
func endStreamErrorCode(err error) string {
	if errors.Is(err, tmux.ErrWorkDirMissing) {
		return connect.CodeFailedPrecondition.String()
	}
	return connect.CodeInternal.String()
}

// endStreamErrorMessage returns the text to send to the browser for an
// EndStream error frame. A missing-working-directory error's full text
// (via tmux's ValidateWorkDir/RestoreWithWorkDir) embeds the session's local
// filesystem path — useful server-side (the caller already logs the full
// error before calling sendEndStreamError), but not something to hand a
// browser tab.
func endStreamErrorMessage(err error) string {
	if errors.Is(err, tmux.ErrWorkDirMissing) {
		return "session working directory no longer exists"
	}
	return err.Error()
}

// sendEndStreamError sends an error EndStream message
func sendEndStreamError(stream *connectWebSocketStream, err error) {
	// ConnectRPC protocol requires JSON-encoded EndStream payload (not protobuf)
	// Error EndStream uses the ConnectRPC error JSON format.
	code := endStreamErrorCode(err)
	errMsg, marshalErr := json.Marshal(endStreamErrorMessage(err))
	if marshalErr != nil {
		// json.Marshal of a plain string can't actually fail today, but fall back
		// defensively rather than send truncated/invalid JSON if that ever changes.
		log.Error("[sendEndStreamError] failed to marshal error message", "err", marshalErr)
		errMsg = []byte(`"internal error"`)
	}
	dataBytes := fmt.Appendf(nil, `{"error":{"code":%q,"message":%s}}`, code, errMsg)

	envelope := protocol.CreateEnvelope(protocol.EndStreamFlag, dataBytes)
	if err := stream.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
		log.Error("failed to send EndStreamError", "err", err)
	}
}

// detectContentWidth analyzes captured terminal content to determine the actual
// rendered width by examining visible characters per line. This is used to detect
// if applications like Claude Code are rendering wider than the terminal dimensions.
//
// Returns the maximum visible width found across all lines.
func detectContentWidth(content string) int {
	maxWidth := 0
	for _, line := range strings.Split(content, "\n") {
		// Strip ANSI codes and count visible characters
		stripped := stripAnsiCodes(line)
		width := utf8.RuneCountInString(stripped)
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// stripAnsiCodes removes ANSI escape sequences from a string to count visible characters.
func stripAnsiCodes(s string) string {
	return ansi.StripCSI(s)
}
