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
	CallBlockingWithOptions(ctx context.Context, key headless.FeatureKey, systemPrompt string, userPrompt string, opts headless.CallOptions) (string, error)
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

// AutonomousDriver monitors a session and injects orchestrator prompts when idle.
type AutonomousDriver struct {
	inst          *Instance
	controller    *ClaudeController
	headlessPool  HeadlessPoolClient
	goal          string
	maxTurns      int
	completionCb  CompletionCallback
	turnCb        TurnCallback
	driverRunning atomic.Bool
	cancel        context.CancelFunc
	mu            sync.Mutex
}

// NewAutonomousDriver creates an AutonomousDriver for inst.
// pool must not be nil; maxTurns ≤ 0 defaults to 20.
func NewAutonomousDriver(inst *Instance, pool HeadlessPoolClient, goal string, maxTurns int) *AutonomousDriver {
	if maxTurns <= 0 {
		maxTurns = 20
	}
	return &AutonomousDriver{
		inst:         inst,
		controller:   inst.GetController(),
		headlessPool: pool,
		goal:         goal,
		maxTurns:     maxTurns,
	}
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
// Context cancellation propagates into CallBlockingWithOptions: the headless pool passes ctx
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

	// Wait for the first idle signal (up to 60s) before beginning the loop.
	startupCtx, startupCancel := context.WithTimeout(ctx, 60*time.Second)
	if !waitForIdle(startupCtx, statusCh, d.controller) {
		startupCancel()
		log.Warn("AutonomousDriver: timed out waiting for initial idle state", "session", sessionName)
		d.fireCompletion(sessionName, AutonomousDriverOutcome{Stuck: true, Reason: "startup timeout"})
		return
	}
	startupCancel()

	outcome := AutonomousDriverOutcome{}

	for turnCount := 0; turnCount < d.maxTurns; turnCount++ {
		if ctx.Err() != nil {
			break
		}

		// Respect rate limit: wait until cleared.
		if err := d.waitForRateLimitClear(ctx); err != nil {
			break
		}

		tail, _ := d.inst.Preview()
		userPrompt := buildOrchestrationPrompt(d.goal, tail, turnCount+1, d.maxTurns)

		keyLen := 8
		if len(sessionID) < keyLen {
			keyLen = len(sessionID)
		}
		featureKey := headless.FeatureKey("autonomous_fix-" + sessionID[:keyLen])
		resp, err := d.headlessPool.CallBlockingWithOptions(ctx, featureKey, autonomousSystemPrompt, userPrompt, headless.CallOptions{})
		if err != nil {
			log.Warn("AutonomousDriver: LLM call failed", "session", sessionName, "turn", turnCount+1, "err", err)
			break
		}

		nextMsg, done, reason, parseErr := parseOrchestrationResponse(resp)
		if parseErr != nil {
			log.Warn("AutonomousDriver: malformed LLM response, retrying", "session", sessionName, "turn", turnCount+1, "resp", resp)
			continue
		}

		if done {
			sessionOutput, _ := d.inst.Preview()
			outcome = AutonomousDriverOutcome{
				Done:   true,
				Reason: reason,
				PRUrl:  ExtractPRURL(sessionOutput),
				Turns:  turnCount + 1,
			}
			log.Info("AutonomousDriver: DONE signal received", "session", sessionName, "turn", turnCount+1, "reason", reason)
			d.fireCompletion(sessionName, outcome)
			return
		}

		_, sendErr := d.controller.SendCommandImmediate(nextMsg + "\r")
		if sendErr != nil {
			log.Warn("AutonomousDriver: SendCommandImmediate failed", "session", sessionName, "turn", turnCount+1, "err", sendErr)
			break
		}
		log.Info("AutonomousDriver: injected turn", "session", sessionName, "turn", turnCount+1)
		d.fireTurnCallback(turnCount+1, d.maxTurns, nextMsg)

		// Wait for idle before the next turn.
		turnCtx, turnCancel := context.WithTimeout(ctx, 5*time.Minute)
		idleReached := waitForIdle(turnCtx, statusCh, d.controller)
		turnCancel()
		if !idleReached {
			log.Warn("AutonomousDriver: session did not become idle after turn, proceeding anyway", "session", sessionName, "turn", turnCount+1)
		}
	}

	if !outcome.Done {
		outcome = AutonomousDriverOutcome{Stuck: true, Reason: "max turns reached", Turns: d.maxTurns}
	}
	d.fireCompletion(sessionName, outcome)
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

// waitForIdle blocks until the controller is idle or ctx is cancelled.
// Returns true if idle was detected, false if ctx expired first.
func waitForIdle(ctx context.Context, statusCh <-chan detection.DetectedStatus, cc *ClaudeController) bool {
	// Fast path: already idle.
	if cc.IsIdle() {
		return true
	}
	for {
		select {
		case <-ctx.Done():
			return false
		case status := <-statusCh:
			if isIdleStatus(status) {
				return true
			}
		}
	}
}

func isIdleStatus(s detection.DetectedStatus) bool {
	return s == detection.StatusIdle || s == detection.StatusReady || s == detection.StatusSuccess
}

// autonomousSystemPrompt is the stable system prompt for orchestrator LLM calls.
const autonomousSystemPrompt = `You are an orchestrator directing a Claude Code session toward a goal.
Reply with exactly one of:
  NEXT_MESSAGE: <message to inject into the session>
  DONE: <reason the goal is complete>
No other text.`

// buildOrchestrationPrompt constructs the user prompt for the orchestrator LLM call.
// Goal and session output are wrapped in XML-style delimiters so that user-controlled
// content (e.g., GitHub PR body embedded in the goal) cannot escape its section and
// spoof a NEXT_MESSAGE or DONE directive in the outer prompt text.
func buildOrchestrationPrompt(goal, tail string, turnCount, maxTurns int) string {
	const maxTailBytes = 80 * 120 // ~80 lines × 120 chars
	if len(tail) > maxTailBytes {
		tail = tail[len(tail)-maxTailBytes:]
	}
	return fmt.Sprintf(
		"<goal>\n%s\n</goal>\n\n<session_output>\n%s\n</session_output>\n\nTurn %d/%d. Reply with NEXT_MESSAGE: <text> or DONE: <reason>.",
		goal, tail, turnCount, maxTurns)
}

// parseOrchestrationResponse parses the LLM's reply into a next message or done signal.
func parseOrchestrationResponse(resp string) (nextMsg string, done bool, reason string, err error) {
	resp = strings.TrimSpace(resp)
	if after, ok := strings.CutPrefix(resp, "DONE:"); ok {
		return "", true, strings.TrimSpace(after), nil
	}
	if after, ok := strings.CutPrefix(resp, "NEXT_MESSAGE:"); ok {
		return strings.TrimSpace(after), false, "", nil
	}
	return "", false, "", fmt.Errorf("unrecognized orchestration response: %q", resp)
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
