package session

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
	"github.com/tstapler/stapler-squad/session/headless"
)

// HeadlessPoolClient is the narrow interface AutonomousDriver needs from the headless pool.
// *headless.Pool satisfies this interface directly.
type HeadlessPoolClient interface {
	CallBlocking(ctx context.Context, key headless.FeatureKey, systemPrompt string, userPrompt string, opts headless.CallOptions) (string, float64, error)
}

// AutonomousDriverOutcome describes how an autonomous driver run concluded.
type AutonomousDriverOutcome struct {
	Done   bool
	Reason string
	PRUrl  string
	Turns  int
	Stuck  bool // true if exited via maxTurns without DONE signal
}

// CompletionCallback is called when the driver exits with a final outcome.
type CompletionCallback func(instanceName string, outcome AutonomousDriverOutcome)

// TurnCallback is called after each successful turn injection.
type TurnCallback func(turn, maxTurns int, prompt string)

// DriverOption is a functional option for configuring an AutonomousDriver.
type DriverOption func(*AutonomousDriver)

// WithStartupTimeout overrides the default 60s startup idle-wait timeout.
// Use a longer timeout for sessions that spawn parallel subagents (e.g. triage).
func WithStartupTimeout(d time.Duration) DriverOption {
	return func(a *AutonomousDriver) { a.startupTimeout = d }
}

// AutonomousDriver monitors a session and injects orchestrator prompts when idle.
type AutonomousDriver struct {
	inst           *Instance
	controller     *ClaudeController
	headlessPool   HeadlessPoolClient
	goal           string
	maxTurns       int
	startupTimeout time.Duration
	completionCb   CompletionCallback
	turnCb         TurnCallback
	driverRunning  atomic.Bool
	cancel         context.CancelFunc
	mu             sync.Mutex
}

// NewAutonomousDriver creates an AutonomousDriver for inst.
// pool must not be nil; maxTurns ≤ 0 defaults to 20.
// Use functional options (e.g. WithStartupTimeout) to override defaults.
func NewAutonomousDriver(inst *Instance, pool HeadlessPoolClient, goal string, maxTurns int, opts ...DriverOption) *AutonomousDriver {
	if maxTurns <= 0 {
		maxTurns = 20
	}
	d := &AutonomousDriver{
		inst:         inst,
		controller:   inst.GetController(),
		headlessPool: pool,
		goal:         goal,
		maxTurns:     maxTurns,
	}
	for _, o := range opts {
		o(d)
	}
	if d.startupTimeout == 0 {
		d.startupTimeout = 60 * time.Second
	}
	return d
}

// RegisterCompletionCallback sets the function called when the driver exits.
func (d *AutonomousDriver) RegisterCompletionCallback(cb CompletionCallback) {
	d.mu.Lock()
	d.completionCb = cb
	d.mu.Unlock()
}

// RegisterTurnCallback sets the function called after each prompt injection.
func (d *AutonomousDriver) RegisterTurnCallback(cb TurnCallback) {
	d.mu.Lock()
	d.turnCb = cb
	d.mu.Unlock()
}

func (d *AutonomousDriver) fireTurnCallback(turn, maxTurns int, prompt string) {
	d.mu.Lock()
	cb := d.turnCb
	d.mu.Unlock()
	if cb != nil {
		cb(turn, maxTurns, prompt)
	}
}

// Start begins the autonomous driver goroutine. The second call is a no-op.
func (d *AutonomousDriver) Start(ctx context.Context) error {
	if d.headlessPool == nil {
		return fmt.Errorf("AutonomousDriver: headlessPool is nil for session %q", d.inst.Title)
	}
	if !d.driverRunning.CompareAndSwap(false, true) {
		log.Debug("AutonomousDriver: already running, skipping duplicate start", "session", d.inst.Title)
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.controller = d.inst.GetController()
	d.mu.Unlock()

	if d.controller == nil {
		d.driverRunning.Store(false)
		cancel()
		return fmt.Errorf("AutonomousDriver: no controller available for session %q", d.inst.Title)
	}

	go func() {
		defer d.driverRunning.Store(false)
		defer cancel()
		d.run(ctx)
	}()
	return nil
}

// Stop cancels the driver goroutine.
// Context cancellation propagates into CallBlocking: the headless pool passes ctx
// to runner.Run (which kills the subprocess) and the stream reader selects on ctx.Done,
// so Stop returns control to the caller nearly immediately — no blocking LLM call delay.
func (d *AutonomousDriver) Stop() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	d.driverRunning.Store(false)
}

// run is the main driver loop. Must only be called from Start's goroutine.
func (d *AutonomousDriver) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("AutonomousDriver: recovered from panic", "session", d.inst.Title, "panic", r)
		}
	}()

	sessionID := d.inst.UUID
	sessionName := d.inst.Title

	// statusCh receives non-blocking signals from the status-change listener.
	// Capacity 1 so the listener never blocks.
	statusCh := make(chan detection.DetectedStatus, 1)

	d.controller.AddStatusChangeListener(func(newStatus detection.DetectedStatus, _ string) {
		select {
		case statusCh <- newStatus:
		default:
		}
	})

	// Wait for the first idle signal before beginning the loop.
	startupTimeout := d.startupTimeout
	if startupTimeout == 0 {
		startupTimeout = 60 * time.Second
	}
	startupCtx, startupCancel := context.WithTimeout(ctx, startupTimeout)
	// No settle window here: startup only needs to observe the session's
	// first idle signal after launch, not debounce against a background
	// fork — nothing has been injected yet, so there's no premature-nudge
	// risk to guard against.
	if !waitForIdle(startupCtx, statusCh, d.controller, 0) {
		startupCancel()
		log.Warn("AutonomousDriver: timed out waiting for initial idle state", "session", sessionName)
		d.fireCompletion(sessionName, AutonomousDriverOutcome{Stuck: true, Reason: "startup timeout"})
		return
	}
	startupCancel()

	outcome := AutonomousDriverOutcome{}
	malformedResponseCount := 0
	// lastSentNudge tracks the most recent nudge actually delivered (both SendKeys
	// writes succeeded). It's a run()-local, not a struct field: every caller
	// constructs a fresh *AutonomousDriver per Start(), so no cross-restart
	// persistence is needed and a local avoids any d.mu entanglement.
	var lastSentNudge lastNudge

	for turnCount := 0; turnCount < d.maxTurns; turnCount++ {
		if ctx.Err() != nil {
			break
		}

		// Respect rate limit: wait until cleared.
		if err := d.waitForRateLimitClear(ctx); err != nil {
			break
		}

		tail, _ := d.inst.Preview()
		userPrompt := buildOrchestrationPrompt(d.goal, tail, turnCount+1, d.maxTurns, lastSentNudge)

		keyLen := 8
		if len(sessionID) < keyLen {
			keyLen = len(sessionID)
		}
		featureKey := headless.FeatureKey("autonomous_fix-" + sessionID[:keyLen])
		resp, _, err := d.headlessPool.CallBlocking(ctx, featureKey, autonomousSystemPrompt, userPrompt, headless.CallOptions{})
		if err != nil {
			log.Warn("AutonomousDriver: LLM call failed", "session", sessionName, "turn", turnCount+1, "err", err)
			break
		}

		directive, payload, parseErr := parseOrchestrationResponse(resp)
		if parseErr != nil {
			malformedResponseCount++
			log.Warn("AutonomousDriver: malformed LLM response, retrying", "session", sessionName, "turn", turnCount+1, "resp", resp)
			continue
		}

		if directive == directiveDone {
			sessionOutput, _ := d.inst.Preview()
			outcome = AutonomousDriverOutcome{
				Done:   true,
				Reason: payload,
				PRUrl:  ExtractPRURL(sessionOutput),
				Turns:  turnCount + 1,
			}
			log.Info("AutonomousDriver: DONE signal received", "session", sessionName, "turn", turnCount+1, "reason", payload)
			d.fireCompletion(sessionName, outcome)
			return
		}

		// A WAIT directive or an exact-repeat of the last delivered nudge both mean
		// "don't inject anything this turn" — the orchestrator (or the deterministic
		// backstop) has recognized the agent already has this guidance. The turn
		// still counts against maxTurns (bounds a runaway/looping orchestrator, same
		// as a malformed response burning a turn) but skips SendKeys/fireTurnCallback
		// entirely, and lastSentNudge is left unchanged since nothing new was sent.
		suppressed := directive == directiveWait || isDuplicateNudge(payload, lastSentNudge, time.Now(), tail)
		if suppressed {
			log.Info("AutonomousDriver: suppressed nudge", "session", sessionName, "turn", turnCount+1, "directive", directive)
			if ctx.Err() != nil {
				break
			}
			// Proceed on the same idle-settle cadence a real send uses, instead of
			// blocking the full nudgeCooldown (3min production default): a
			// suppressed turn means the agent is still working, so waiting for it
			// to go idle again is what re-arms the dedup guard (isDuplicateNudge
			// re-checks the cooldown next turn) and lets the orchestrator re-poll
			// promptly rather than stalling on an arbitrary fixed timer.
			turnCtx, turnCancel := context.WithTimeout(ctx, 5*time.Minute)
			idleReached := waitForIdle(turnCtx, statusCh, d.controller, idleSettleWindow)
			turnCancel()
			if !idleReached {
				log.Warn("AutonomousDriver: session did not become idle after suppressed turn, proceeding anyway", "session", sessionName, "turn", turnCount+1)
			}
			continue
		}

		nextMsg := payload

		// Use SendKeys (raw PTY write) instead of SendCommandImmediate so that only
		// "\r" is sent. SendCommandImmediate goes through the command executor which
		// appends "\n", producing "\r\n". In Claude Code's TUI input, "\r\n" inserts
		// text into the multiline buffer without submitting — identical to steer_session
		// which uses inst.SendKeys(msg + "\r") directly and is known to work.
		//
		// content and "\r" are sent as two SEPARATE writes, not concatenated into one
		// (BUG-031): a single large write lands its trailing "\r" inside the TUI's
		// paste-detection window for sufficiently long prompts, folding it into the
		// pasted block instead of submitting — live-confirmed via a stuck session
		// showing an unsubmitted "[Pasted text #N +1 lines]" block at the input line.
		// waitForPaneSettle gives the TUI's paste detector a chance to close before
		// the submit keystroke arrives as its own write.
		if sendErr := d.inst.SendKeys(nextMsg); sendErr != nil {
			log.Warn("AutonomousDriver: SendKeys failed", "session", sessionName, "turn", turnCount+1, "err", sendErr)
			break
		}
		waitForPaneSettle(ctx, d.inst)
		if sendErr := d.inst.SendKeys(EnterKeySequence); sendErr != nil {
			log.Warn("AutonomousDriver: submit keystroke failed", "session", sessionName, "turn", turnCount+1, "err", sendErr)
			break
		}
		// Only recorded once both writes above have succeeded — a failed send must
		// not be treated as delivered for future dedup checks. Isolated into
		// nextLastNudge (a pure function) so this invariant is directly unit
		// testable without needing a tmux-backed Instance to force a partial
		// SendKeys failure.
		lastSentNudge = nextLastNudge(lastSentNudge, nextMsg, true, tail)
		log.Info("AutonomousDriver: injected turn", "session", sessionName, "turn", turnCount+1)
		d.fireTurnCallback(turnCount+1, d.maxTurns, nextMsg)

		// Wait for idle before the next turn.
		turnCtx, turnCancel := context.WithTimeout(ctx, 5*time.Minute)
		idleReached := waitForIdle(turnCtx, statusCh, d.controller, idleSettleWindow)
		turnCancel()
		if !idleReached {
			log.Warn("AutonomousDriver: session did not become idle after turn, proceeding anyway", "session", sessionName, "turn", turnCount+1)
		}
	}

	if !outcome.Done {
		reason := "max turns reached"
		if malformedResponseCount > 0 {
			reason = fmt.Sprintf("max turns reached (%d malformed orchestrator responses)", malformedResponseCount)
		}
		outcome = AutonomousDriverOutcome{Stuck: true, Reason: reason, Turns: d.maxTurns}
	}
	d.fireCompletion(sessionName, outcome)
}

// paneSettleChecker is the narrow interface waitForPaneSettle needs — satisfied
// by *Instance's existing HasUpdated method.
type paneSettleChecker interface {
	HasUpdated() (updated bool, hasPrompt bool)
}

// paneSettlePollInterval and paneSettleMaxWait are vars (not consts) so tests
// can shrink them instead of waiting out real timers.
var (
	paneSettlePollInterval = 150 * time.Millisecond
	paneSettleMaxWait      = 2 * time.Second
)

// waitForPaneSettle polls inst until its pane content stops changing for two
// consecutive polls, or paneSettleMaxWait elapses — whichever first. Used
// between writing a turn's content and sending the submit keystroke (BUG-031):
// a large paste can still be landing/rendering in the TUI's paste-detection
// window when a follow-up "\r" arrives, so giving the pane a chance to settle
// first shrinks the race that folds the submit keystroke into the paste block
// instead of ending it. Best-effort: a pane that never settles (e.g. a
// permanently busy session) is left alone once the deadline passes — the
// caller still sends "\r" either way.
func waitForPaneSettle(ctx context.Context, inst paneSettleChecker) {
	deadline := time.Now().Add(paneSettleMaxWait)
	stableCount := 0
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(paneSettlePollInterval):
		}
		updated, _ := inst.HasUpdated()
		if updated {
			stableCount = 0
			continue
		}
		stableCount++
		if stableCount >= 2 {
			return
		}
	}
}

// maxRateLimitWait caps how long waitForRateLimitClear will block in total.
const maxRateLimitWait = 4 * time.Hour

// waitForRateLimitClear blocks until the controller's rate limit state is StateNone.
// Returns an error if ctx is cancelled or the total wait exceeds maxRateLimitWait.
func (d *AutonomousDriver) waitForRateLimitClear(ctx context.Context) error {
	deadline := time.Now().Add(maxRateLimitWait)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("rate limit wait exceeded %v", maxRateLimitWait)
		}
		state := d.controller.GetRateLimitState()
		if state == ratelimit.StateNone {
			return nil
		}
		resetTime := d.controller.GetRateLimitResetTime()
		waitUntil := resetTime.Add(5 * time.Second)
		if waitUntil.Before(time.Now()) {
			waitUntil = time.Now().Add(30 * time.Second)
		}
		// Don't wait past the overall deadline.
		if waitUntil.After(deadline) {
			waitUntil = deadline
		}
		log.Info("AutonomousDriver: rate limited, waiting", "session", d.inst.Title, "until", waitUntil)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Until(waitUntil)):
		}
	}
}

// fireCompletion calls the registered completion callback (if any).
func (d *AutonomousDriver) fireCompletion(sessionName string, outcome AutonomousDriverOutcome) {
	d.mu.Lock()
	cb := d.completionCb
	d.mu.Unlock()
	if cb != nil {
		cb(sessionName, outcome)
	}
}

// idleSettlePollInterval and idleSettleWindow are vars (not consts) so tests
// can shrink them instead of waiting out real timers, matching the
// waitForPaneSettle pattern above.
var (
	idleSettlePollInterval = 500 * time.Millisecond
	idleSettleWindow       = 60 * time.Second
)

// waitForIdle blocks until the session needs a new orchestrator turn, or ctx
// is cancelled. Two paths satisfy it:
//   - An explicit status arrives (approval pending, input required, error,
//     tests failing) — these already mean "needs redirection now", no debounce.
//   - A routine Idle/Ready/Success status persists, with no intervening
//     StatusExecuting/StatusProcessing/StatusWaitingForAgent in between, for
//     settleWindow. A settleWindow of 0 restores immediate first-idle-signal
//     behavior (used for the startup wait, where nothing has been injected
//     yet, so there's no premature-nudge risk to guard against).
//
// Without the settle window, a single transient idle blip — e.g. Claude
// finishes printing a short reply while a background subagent/fork it just
// launched is still running — was enough to fire a new orchestrator turn
// immediately, injecting a nudge (and a paired notification, see
// buildTurnCallback in server/services/autonomous_orchestration_service.go)
// every few seconds instead of only when the session genuinely needed
// redirection (confirmed live 2026-08-02 on stapler-squad-backlog-self-resolve).
//
// Returns true if a turn is warranted, false if ctx expired first.
func waitForIdle(ctx context.Context, statusCh <-chan detection.DetectedStatus, cc *ClaudeController, settleWindow time.Duration) bool {
	if settleWindow <= 0 {
		if cc.IsIdle() {
			return true
		}
		for {
			select {
			case <-ctx.Done():
				return false
			case status := <-statusCh:
				if isIdleStatus(status) || isImmediateStatus(status) {
					return true
				}
			}
		}
	}

	var idleSince time.Time
	if cc.IsIdle() {
		idleSince = time.Now()
	}

	for {
		if !idleSince.IsZero() && time.Since(idleSince) >= settleWindow {
			return true
		}

		wait := idleSettlePollInterval
		if !idleSince.IsZero() {
			if remaining := settleWindow - time.Since(idleSince); remaining < wait {
				wait = remaining
			}
		}

		select {
		case <-ctx.Done():
			return false
		case status := <-statusCh:
			switch {
			case isImmediateStatus(status):
				return true
			case isIdleStatus(status):
				if idleSince.IsZero() {
					idleSince = time.Now()
				}
			default:
				idleSince = time.Time{}
			}
		case <-time.After(wait):
			// Loop back around to re-check the settle window above.
		}
	}
}

func isIdleStatus(s detection.DetectedStatus) bool {
	return s == detection.StatusIdle || s == detection.StatusReady || s == detection.StatusSuccess
}

// isImmediateStatus reports whether s already represents an explicit signal
// that the session needs redirection right now, bypassing idleSettleWindow.
func isImmediateStatus(s detection.DetectedStatus) bool {
	switch s {
	case detection.StatusNeedsApproval, detection.StatusInputRequired, detection.StatusError, detection.StatusTestsFailing:
		return true
	default:
		return false
	}
}

// autonomousSystemPrompt is the stable system prompt for orchestrator LLM calls.
const autonomousSystemPrompt = `You are an orchestrator directing a Claude Code session toward a goal.
Reply with exactly one of:
  NEXT_MESSAGE: <message to inject into the session>
  DONE: <reason the goal is complete>
  WAIT: <reason the agent already has a plan and needs no new nudge>
Use WAIT when the <last_nudge> shows the agent already acknowledged the same guidance and
stated a plan to act on it (e.g. "I'll wait for CI") — do not repeat NEXT_MESSAGE with the
same content just because the session still looks idle.
No other text.`

// buildOrchestrationPrompt constructs the user prompt for the orchestrator LLM call.
// Goal and session output are wrapped in XML-style delimiters so that user-controlled
// content (e.g., GitHub PR body embedded in the goal) cannot escape its section and
// spoof a NEXT_MESSAGE or DONE directive in the outer prompt text. lastSent describes
// the most recent nudge actually delivered (both SendKeys calls succeeded); a zero
// lastSent (lastSent.at.IsZero()) means none has been sent yet. Bundled into one
// struct param (rather than two same-shaped text/time.Time args) per
// .claude/rules/primitive-obsession-checklist.md — the two values are always read
// and passed together, so a struct removes the chance of them being supplied out of
// order at a call site. lastSent.text is LLM-generated content from a prior turn, so
// it's wrapped in its own <last_nudge> tag (same anti-spoofing rationale as
// <goal>/<session_output>) rather than interpolated directly into the instruction text.
// lastNudgeTagEscaper neutralizes the two characters that could otherwise let
// lastSent.text close its <last_nudge> block early.
var lastNudgeTagEscaper = strings.NewReplacer("<", "&lt;", ">", "&gt;")

func buildOrchestrationPrompt(goal, tail string, turnCount, maxTurns int, lastSent lastNudge) string {
	const maxTailBytes = 80 * 120 // ~80 lines × 120 chars
	if len(tail) > maxTailBytes {
		tail = tail[len(tail)-maxTailBytes:]
	}
	// The whole <last_nudge> section is omitted (not just given placeholder
	// content) on turn 1 / whenever no nudge has been delivered yet — an
	// absent section is unambiguous to the orchestrator LLM, whereas a
	// present-but-empty tag risks being mistaken for "a nudge with empty
	// text was sent".
	lastNudgeSection := ""
	if lastSent.text != "" && !lastSent.at.IsZero() {
		// Escape "<"/">" before interpolating: lastSent.text is the system's own
		// prior LLM output being round-tripped back into the prompt, so a nudge
		// containing "</last_nudge>" (or another tag) could otherwise close the
		// block early and spoof content into the surrounding instruction text —
		// the same anti-spoofing rationale as the <goal>/<session_output> wrapping.
		escapedNudgeText := lastNudgeTagEscaper.Replace(lastSent.text)
		lastNudgeSection = fmt.Sprintf("\n\n<last_nudge>\n%s\n(sent %s ago)\n</last_nudge>",
			escapedNudgeText, time.Since(lastSent.at).Round(time.Second))
	}
	return fmt.Sprintf(
		"<goal>\n%s\n</goal>\n\n<session_output>\n%s\n</session_output>%s\n\nTurn %d/%d. Reply with NEXT_MESSAGE: <text>, DONE: <reason>, or WAIT: <reason>.",
		goal, tail, lastNudgeSection, turnCount, maxTurns)
}

// orchestrationDirectiveMarker matches "DONE:", "NEXT_MESSAGE:", or "WAIT:"
// case-insensitively, anywhere in the response — not just as an exact prefix of the
// whole string. Confirmed live (2026-08-01, BUG-056): despite the system prompt's "no
// other text" instruction, the orchestrator model routinely writes a full free-text
// explanation and appends the directive directly onto the end of its last sentence with
// no separating newline at all (e.g. "...reflecting real findings rather than
// guesswork.DONE: Reached the 20-turn limit..."), which the old exact-prefix match
// rejected outright, burning the turn (see AutonomousDriver's malformedResponseCount —
// 8 of 20 turns wasted this way on one live item). Matching case-insensitively anywhere
// handles this plus markdown fencing and preamble-before-the-directive without needing
// separate handling for each.
var orchestrationDirectiveMarker = regexp.MustCompile(`(?i)(DONE|NEXT_MESSAGE|WAIT)\s*:`)

// orchestrationDirective is the 3-way outcome of parsing an orchestrator reply. A
// dedicated enum (rather than a second bool alongside nextMsg/reason strings) avoids
// the same same-typed-parameter ambiguity flagged by
// .claude/rules/primitive-obsession-checklist.md for function parameters — here applied
// to a return value that would otherwise need a second bool to distinguish WAIT from
// DONE.
type orchestrationDirective int

const (
	directiveNextMessage orchestrationDirective = iota
	directiveDone
	directiveWait
)

// parseOrchestrationResponse parses the LLM's reply into a directive plus its payload.
// Finds the LAST occurrence of a directive marker in the response (not the first): the
// model's authoritative final answer consistently comes after any preamble/reasoning it
// writes first, so preferring the last occurrence picks the real directive over an
// earlier echo of the instructions or an incidental mention. Everything after that
// marker, to the end of the response, is the payload — preserving a multi-line
// NEXT_MESSAGE body. This only ever inspects the model's own reply text, never the
// prompt it was given, so a <last_nudge> block containing a literal "NEXT_MESSAGE:"-like
// string in a prior nudge cannot be misparsed as a directive here.
func parseOrchestrationResponse(resp string) (directive orchestrationDirective, payload string, err error) {
	trimmed := strings.TrimSpace(resp)
	matches := orchestrationDirectiveMarker.FindAllStringSubmatchIndex(trimmed, -1)
	if len(matches) == 0 {
		return directiveNextMessage, "", fmt.Errorf("unrecognized orchestration response: %q", resp)
	}
	last := matches[len(matches)-1]
	keyword := strings.ToUpper(trimmed[last[2]:last[3]])
	body := strings.TrimSpace(trimmed[last[1]:])
	switch keyword {
	case "DONE":
		return directiveDone, body, nil
	case "WAIT":
		return directiveWait, body, nil
	default:
		return directiveNextMessage, body, nil
	}
}

var prURLRegex = regexp.MustCompile(`https://github\.com/[^/\s]+/[^/\s]+/pull/\d+`)

// ExtractPRURL scans the last 200 lines of sessionOutput for a GitHub PR URL.
func ExtractPRURL(sessionOutput string) string {
	lines := strings.Split(sessionOutput, "\n")
	start := len(lines) - 200
	if start < 0 {
		start = 0
	}
	tail := strings.Join(lines[start:], "\n")
	return prURLRegex.FindString(tail)
}
