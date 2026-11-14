package session

// session_driver.go runs a background goroutine after a session is started to:
//  1. Answer startup dialogs (trust-folder check, directory-access prompts) by
//     watching Preview() output and sending the appropriate key sequence.
//  2. Detect when Claude Code is at the ">" ready prompt (Ready status).
//  3. Send an initial message so the agent begins its task immediately.
//  4. Watch for NeedsApproval prompts after the initial message is sent and
//     auto-approve those that are within the session's configured path.
//  5. Detect inactivity (session stuck at Ready for >10 min) and unexpected exits.
//  6. Auto-restart once with a JSONL-derived continuation prompt.
//  7. Mark the session for human attention after a second failure.
//
// AutoYes (-y) on the session handles tool-use permission prompts automatically;
// this driver covers everything else that requires interactive input.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// spinnerTimeRe matches the time/token suffix of an active Claude Code spinner,
// e.g. "(4m 18s ·" or "(1h 5m 37s ·". Only present while Claude is processing.
var spinnerTimeRe = regexp.MustCompile(`\(\d+[hms]`)

// completionVerbRe matches past-tense spinner verbs printed when Claude finishes
// a work session, e.g. "✻ Perambulated for 1h 5m", "✽ Roosted for 9m", or
// "* Moonwalked for 30s". Matches any leading symbol variant.
var completionVerbRe = regexp.MustCompile(`[✻✽\*] \w+ed for \d+`)

// driverTiming is the pair of driver timings tests need to shrink.
type driverTiming struct {
	pollInterval, readyTimeout time.Duration
	// totalTimeout overrides driverTotalTimeout; zero means the default.
	totalTimeout time.Duration
}

func (t driverTiming) total() time.Duration {
	if t.totalTimeout > 0 {
		return t.totalTimeout
	}
	return driverTotalTimeout
}

// testDriverTiming overrides the production timings; nil means defaults. It is
// an atomic pointer read once per driver start so leaked goroutines from other
// tests cannot race a test that swaps it.
var testDriverTiming atomic.Pointer[driverTiming]

func currentDriverTiming() driverTiming {
	if t := testDriverTiming.Load(); t != nil {
		return *t
	}
	return driverTiming{pollInterval: driverPollInterval, readyTimeout: driverReadyTimeout}
}

const (
	driverPollInterval  = 2 * time.Second
	driverReadyTimeout  = 30 * time.Second
	driverTotalTimeout  = 25 * time.Minute // safety-net; inactivity detection is the real mechanism
	driverInitialPrompt = "Please proceed with the task described in your instructions."

	// driverInactivityTimeout is how long a session can be in the Ready state with no output
	// before it is considered stuck and restarted. Only fires after the initial prompt is sent.
	driverInactivityTimeout = 10 * time.Minute

	// driverBacklogNudgeDelay is how long after the initial prompt a backlog work session can
	// be idle before we send a task-reminder nudge (instead of restarting immediately).
	driverBacklogNudgeDelay = 5 * time.Minute

	// driverBacklogNudgeGrace is how long we wait after sending a nudge before treating
	// the session as stuck and falling through to the normal restart path.
	driverBacklogNudgeGrace = 5 * time.Minute

	// driverMinRuntimeBeforeRetry is the minimum time after sending the initial prompt before
	// an unexpected exit is treated as a crash worth retrying. Sessions that ran longer than
	// this are assumed to have completed their task normally.
	driverMinRuntimeBeforeRetry = 5 * time.Minute

	// maxDialogAnswerAttempts bounds retry-on-failure for a DialogAnswerLatch
	// (ADR-001): a SendKeys failure against the *same* dialog content hash is
	// retried up to this many times before the latch gives up (dialogGaveUp).
	// A hash change (a genuinely new/different dialog) resets the counter, so
	// this does not bound legitimate re-answering of a later, different dialog.
	maxDialogAnswerAttempts = 3
)

// driverAttempt keeps the current run's activity floor separate from durable
// prompt delivery. Every initial and retry driver constructs one in the core loop.
type driverAttempt struct {
	startedAt           time.Time
	sentInitial         bool
	initialPromptSentAt time.Time
	nudgeSentAt         time.Time
	// flaggedOutputAt is the last-output time when the interactive quiet note
	// was raised; non-zero means raised. It re-arms only when output advances
	// past it, so status flapping cannot re-fire it. Per driver run: a driver
	// restart may note the same quiet episode once more (log-only, harmless).
	flaggedOutputAt time.Time
	stallFlags      int // how many quiet notes this run raised
	// deadSince is when the pane was first probed dead in the current unbroken
	// run of dead probes; zero when the last probe found it alive.
	deadSince time.Time
}

func newDriverAttempt(inst *instance.Instance, initialPrompt string, now time.Time) driverAttempt {
	attempt := driverAttempt{startedAt: now}
	if initialPrompt == "" {
		attempt.sentInitial = true
		attempt.initialPromptSentAt = now
	} else if persisted := inst.GetInitialPromptSentAt(); !persisted.IsZero() {
		attempt.sentInitial = true
		attempt.initialPromptSentAt = persisted
	}
	return attempt
}

func (a *driverAttempt) idleDuration(now, outputAt time.Time) time.Duration {
	activityRef := a.startedAt
	for _, candidate := range []time.Time{a.initialPromptSentAt, outputAt, a.nudgeSentAt} {
		if candidate.After(activityRef) {
			activityRef = candidate
		}
	}
	return now.Sub(activityRef)
}

// dialogLatchStatus is the DialogAnswerLatch state machine (ADR-001):
// dialogUnanswered -> dialogAwaitingDismissal -> dialogGaveUp, keyed by a
// content hash of the (tail-sliced, whitespace-normalized) dialog text.
type dialogLatchStatus int

const (
	// dialogUnanswered means either no dialog has been seen yet for the
	// current hash, or the current hash's send attempts are still under
	// the maxDialogAnswerAttempts retry cap.
	dialogUnanswered dialogLatchStatus = iota
	// dialogAwaitingDismissal means a SendKeys for the current hash
	// succeeded; the latch will not resend while the hash is unchanged.
	dialogAwaitingDismissal
	// dialogGaveUp means SendKeys failed maxDialogAnswerAttempts times in a
	// row for the current hash; the latch will not retry further while the
	// hash is unchanged.
	dialogGaveUp
)

// dialogAnswerState holds the per-call-site DialogAnswerLatch state for one
// SendKeys call site within a single runSessionDriverWithPrompt invocation.
// It is a plain local variable, not an Instance field: StartSessionDriver's
// driverRunning.CompareAndSwap already guarantees a single sequential
// goroutine owns this state for the life of one driver run — the same
// reasoning sentInitial/initialPromptSentAt (already local vars in this
// function) rely on. The zero value (hash 0, status dialogUnanswered,
// attempts 0) doubles as "no dialog seen yet" (see ADR-001 Consequences).
type dialogAnswerState struct {
	hash     uint64
	status   dialogLatchStatus
	attempts int
}

// sessionDriverStartHook lets a test binary record who started each driver; see
// SetSessionDriverStartHookForTest.
var sessionDriverStartHook atomic.Pointer[func(*instance.Instance)]

// SetSessionDriverStartHookForTest registers fn to run synchronously on the
// goroutine that calls StartSessionDriver, each time it actually spawns a
// driver. A leak detector uses it to capture the creating test's stack, which
// the driver goroutine's own stack cannot show. Pass nil to clear.
func SetSessionDriverStartHookForTest(fn func(*instance.Instance)) {
	if fn == nil {
		sessionDriverStartHook.Store(nil)
		return
	}
	sessionDriverStartHook.Store(&fn)
}

// SessionDriverRunningForTest reports whether inst still has a live driver goroutine.
func SessionDriverRunningForTest(inst *instance.Instance) bool {
	return inst.DriverRunning()
}

// StartSessionDriverForTest starts inst's driver and registers its stop+join on
// t's cleanup in the same call, so a test cannot forget to stop it. Tests outside
// package session must use this instead of StartSessionDriver (enforced by the
// nosessiondrivertest analyzer). t is satisfied by testing.TB.
func StartSessionDriverForTest(t interface{ Cleanup(func()) }, inst *instance.Instance, allowedPath string) {
	t.Cleanup(func() {
		instance.StopSessionDriver(inst)
		instance.JoinSessionDriver(inst)
	})
	StartSessionDriver(inst, allowedPath)
}

// StartSessionDriver launches a background goroutine that drives the session
// through its startup dialogs, fires the initial task prompt, and monitors
// for approval dialogs throughout the session lifetime.
//
// allowedPath is the session's repo/workspace path — directory-access approval
// dialogs that mention this path are auto-approved.
//
// Calling StartSessionDriver twice on the same instance is safe: the second call
// is a no-op (the idempotency guard uses atomic.Bool.CompareAndSwap). Calling it
// on an instance whose Destroy() has already run (or is running concurrently)
// is also a no-op — see Instance.driverMu's doc comment — so a delayed caller
// (e.g. CreateSession's async init goroutine racing a fast DeleteSession) can
// never spawn a driver goroutine that nothing will ever be told to stop.
func StartSessionDriver(inst *instance.Instance, allowedPath string) {
	stop, err := inst.BeginDriverRun()
	switch {
	case errors.Is(err, instance.ErrDriverDestroyed):
		log.Debug("SessionDriver: instance already destroyed, refusing to start driver",
			"session", inst.Title,
		)
		return
	case err != nil:
		log.Debug("SessionDriver: driver already running for session, skipping duplicate start",
			"session", inst.Title,
		)
		return
	}
	if hook := sessionDriverStartHook.Load(); hook != nil {
		(*hook)(inst)
	}
	go func() {
		defer inst.FinishDriverGoroutine()
		runSessionDriver(inst, allowedPath, stop)
	}()
}

// runSessionDriver is the thin wrapper that resolves the effective retry
// policy once and delegates to runSessionDriverWithPrompt.
func runSessionDriver(inst *instance.Instance, allowedPath string, stop <-chan struct{}) {
	policy := instance.ResolveRetryPolicy(config.LoadConfig().RetryPolicy, inst.RetryPolicyOverride())
	inst.SetRetryProgress(func(rs *domain.RetryState) { rs.RetryMaxAttempts = policy.MaxAttempts })

	var initialPrompt string
	if inst.InitialPrompt != "" {
		sanitized := instance.CleanDriverPromptText(inst.InitialPrompt)
		if sanitized != "" {
			initialPrompt = sanitized
		}
	}
	runSessionDriverWithPrompt(inst, allowedPath, initialPrompt, policy, stop)
}

// runSessionDriverWithPrompt is the core driver loop. It accepts a custom
// initial prompt (used by the retry path to inject a JSONL-derived
// continuation), the resolved RetryPolicy for this run (resolved once at
// driver start — never re-read from config mid-cycle, so a live config edit
// doesn't change the cap/delay a session is already partway through), and
// the stop channel from this run's sessionDriverStopper so StopSessionDriver
// (called by Instance.Destroy()) can end the loop early.
func runSessionDriverWithPrompt(inst *instance.Instance, allowedPath string, initialPrompt string, policy instance.RetryPolicy, stop <-chan struct{}) {
	// CONCERN-3 fix: panic recovery inside this function so the retry goroutine is
	// also protected (the retry goroutine does NOT go through the StartSessionDriver wrapper).
	defer func() {
		if r := recover(); r != nil {
			log.Error("SessionDriver: panic recovered in driver goroutine",
				"session", inst.Title,
				"panic", r,
			)
		}
	}()

	// ctx is cancelled the moment stop closes, so a Preview poll blocked inside
	// an in-flight capture-pane subprocess is interrupted directly instead of
	// StopSessionDriver only waiting out driverStopTimeout for it to finish on
	// its own — see PreviewContext's doc comment (instance_terminal.go).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-stop:
			cancel()
		case <-ctx.Done():
		}
	}()

	timing := currentDriverTiming()
	readyDeadline := time.Now().Add(timing.readyTimeout)
	runStart := time.Now()
	totalDeadline := driverTotalDeadline(inst, runStart)
	dialogDeadline := runStart.Add(timing.total())

	ticker := time.NewTicker(timing.pollInterval)
	defer ticker.Stop()

	// Skips the per-tick capture-pane while the pane is provably unchanged (pane_activity_gate.go).
	// Local tmux only: the snapshot is the local server's list-panes.
	paneGate := newPaneActivityGate(func() map[string]time.Time {
		return driverPaneSnapshot(inst.IsRemote())
	}, time.Now, driverPaneActivityGateEnabled)

	// PR URLs from terminal output that the scan has already handled.
	scannedURLs := scannedPRURLs{}

	attempt := newDriverAttempt(inst, initialPrompt, time.Now())
	markInitialPromptIfAlreadySent(ctx, inst, &attempt)
	var sendAttempts int

	// DialogAnswerLatch state (ADR-001), one per SendKeys("1\n") call site.
	// Independently scoped — the approval-prompt latch is not shared with the
	// startup-dialog latch (a buffer classified true by both detectors
	// simultaneously, though very unlikely given their disjoint phrase sets,
	// could fire both in one tick; see ADR-001/plan.md Risk Control).
	var startupLatch dialogAnswerState
	var approvalLatch dialogAnswerState

	// Both DialogAnswerLatch call sites below send the identical key sequence
	// ("1\n" selects the affirmative numbered option on both the startup-dialog
	// and approval-prompt menus) — hoisted once so there is a single definition
	// shared by both answerDialogOnce calls instead of two independent literal
	// closures.
	sendAnswerKey := func() error { return sendAnswerKeyUnderLease(inst) }

	for {
		st, detectedSt, exit := driverTickGate(inst, stop, ticker, totalDeadline)
		if exit {
			return
		}

		// A pending backoff-scheduled retry takes priority over every other
		// branch below. Checked here — before st == Stopped re-enters
		// handleStoppedStatus's re-evaluation path — rather than inside
		// handleStoppedStatus itself, because GetEffectiveStatus() may already
		// read something other than Stopped by the time a restart is
		// mid-flight (or may still read Stopped on the very tick it just
		// elapsed): this gate must not depend on which. Without it, a pending
		// retry would be re-evaluated on every 2s tick during the backoff
		// wait, re-appending RetryAttemptRecords and re-incrementing
		// RetryAttempt every tick instead of once after the intended delay.
		if cont, ret := handleRetryPendingTick(inst, allowedPath, policy, stop); ret {
			return
		} else if cont {
			continue
		}

		// Stopped after sentInitial = potential unexpected exit.
		if cont, ret := handleStoppedStatus(ctx, inst, st, allowedPath, initialPrompt, policy, stop, attempt.sentInitial, attempt.initialPromptSentAt); ret {
			return
		} else if cont {
			continue
		}

		// Always check Preview for startup dialogs that appear before the status
		// machine reaches NeedsApproval — e.g. the trust-folder safety check.
		output, previewErr := driverPreview(ctx, paneGate, inst)
		hasOutput := previewErr == nil && output != ""
		// Tail-slice once per tick: Preview() returns the entire accumulated PTY
		// buffer (not a tailed "current screen" snapshot, despite its doc comment),
		// so both the isStartupDialog/shouldApprovePrompt match and the latch hash
		// below are computed against a bounded recent window, not the session's
		// entire history (ADR-001 "Tail-slice before matching and hashing").
		var tailed string
		if hasOutput {
			tailed = tailContent(output, statusDetectionTailBytes)
		}

		// Auto-answering dialogs is scoped to the first driverTotalTimeout for
		// interactive sessions: later on an operator may be at the keyboard.
		dialogsOpen := isAutonomousSession(inst) || time.Now().Before(dialogDeadline)
		if dialogsOpen && hasOutput && isStartupDialog(tailed) {
			if handleStartupDialogTick(inst, tailed, &startupLatch, sendAnswerKey) {
				continue
			}
			// Fall through to the rest of the loop body — see
			// handleStartupDialogTick's doc comment.
		}

		if !attempt.sentInitial {
			sendInitialPromptTick(ctx, inst, initialPrompt, output, detectedSt, readyDeadline, &attempt.sentInitial, &attempt.initialPromptSentAt, &sendAttempts)
			continue
		}

		// Inactivity detection: only after initial prompt sent.
		// Use GetEffectiveStatus() (acquires stateMutex.RLock) to avoid data race on Status field.
		if st == domain.Active {
			cont, ret := handleInactivityTick(ctx, inst, allowedPath, policy, stop, &attempt, time.Now())
			if ret {
				return
			}
			if cont {
				continue
			}
		}

		// After the initial prompt is sent, watch for NeedsApproval to handle
		// directory-access dialogs that AutoYes (-y) doesn't cover.
		if dialogsOpen {
			handleApprovalDialogTick(inst, hasOutput, tailed, allowedPath, &approvalLatch, sendAnswerKey)
		}

		// Scan terminal output for a GitHub PR URL printed by git push.
		scanAndLinkPRURL(inst, attempt.sentInitial, scannedURLs, previewErr, output)
	}
}

// markInitialPromptIfAlreadySent checks whether the initial prompt was already
// delivered in a previous service run: live terminal output first (no disk
// latency), then the JSONL file. It persists the result via
// SetInitialPromptSentAt either way, so a session that predates the persisted
// field (or hit an earlier failed persist) does not re-run these heuristics
// next restart.
func markInitialPromptIfAlreadySent(ctx context.Context, inst *instance.Instance, attempt *driverAttempt) {
	if attempt.sentInitial {
		return
	}
	if startOutput, err := inst.PreviewContext(ctx); err == nil && outputShowsConversationStarted(startOutput) {
		attempt.sentInitial = true
		attempt.initialPromptSentAt = time.Now()
		inst.SetInitialPromptSentAt(attempt.initialPromptSentAt)
	} else if _, err := FindConversationFilePath(ctx, inst.GetStableID()); err == nil {
		attempt.sentInitial = true
		attempt.initialPromptSentAt = time.Now()
		inst.SetInitialPromptSentAt(attempt.initialPromptSentAt)
	}
}

// driverTickGate blocks until the next poll tick (or stop fires), then
// evaluates the driver loop's per-tick early-exit conditions in order: the
// stop channel, the total driver deadline, inst.destroyed (see the inline
// comment this replaced for why that check exists), and Paused status
// (always a clean stop for the driver). Also returns the effective/detected
// status the caller needs regardless — GetEffectiveStatus for lifecycle
// decisions (Paused, Stopped), GetDetectedStatus for fine-grained
// terminal-content signals (Idle = readline prompt) — so callers don't
// re-fetch it. exit reports whether runSessionDriverWithPrompt's loop should
// return immediately without processing this tick further.
func driverTickGate(inst *instance.Instance, stop <-chan struct{}, ticker *time.Ticker, totalDeadline time.Time) (st domain.Status, detectedSt detection.DetectedStatus, exit bool) {
	select {
	case <-stop:
		return 0, 0, true
	case <-ticker.C:
	}

	if !totalDeadline.IsZero() && time.Now().After(totalDeadline) {
		return 0, 0, true
	}

	// inst.destroyed is set by Destroy() (session/instance.go). Checking it
	// here bounds how long this goroutine can outlive its own instance's
	// teardown to a single driverPollInterval tick, instead of continuing
	// until totalDeadline and racing subsequent tmux/config calls
	// (AcquireExecSlot's gateDir/LoadConfig, both keyed off the
	// process-global STAPLER_SQUAD_TEST_DIR env var) against whatever test
	// or session happens to be using that env var's value next.
	if inst.IsDestroyed() {
		return 0, 0, true
	}

	if !isAutonomousSession(inst) && interactiveDriverShouldEnd(inst) {
		return 0, 0, true
	}

	st = inst.GetEffectiveStatus()
	detectedSt = inst.GetDetectedStatus()

	if st == domain.Paused {
		return st, detectedSt, true
	}
	return st, detectedSt, false
}

// handleRetryPendingTick checks for, and applies, a pending backoff-scheduled
// retry (see runSessionDriverWithPrompt's call site for why this must be
// checked before st == Stopped re-enters handleStoppedStatus's re-evaluation
// path). Mirrors handleStoppedStatus/handleInactivityTick's
// (shouldContinue, shouldReturn) signal pattern: shouldContinue means no
// retry was pending (or it's still pending) so the loop should proceed to its
// next tick unchanged; shouldReturn means this goroutine's job is done
// (either restartForRetry succeeded and spawned a fresh continuation, or it
// failed with a non-racing error).
func handleRetryPendingTick(inst *instance.Instance, allowedPath string, policy instance.RetryPolicy, stop <-chan struct{}) (shouldContinue, shouldReturn bool) {
	pending, elapsed := inst.RetryPendingElapsed(time.Now())
	if !pending {
		return false, false
	}
	if !elapsed {
		return true, false
	}
	// A retry scheduled before the archive landed must be dropped, not fired
	// (ADR-001). Placed after the pending/elapsed gates so a non-pending
	// session pays no snapshot read on every tick; the only observable
	// difference is that the pending NextRetryAt is cleared at its deadline
	// rather than the moment the archive lands — no restart happens between.
	if inst.IsArchived() {
		inst.ClearNextRetryAt()
		log.Info("SessionDriver: dropping scheduled retry, session is archived", "session", inst.Title)
		return false, true
	}
	reason := inst.LastRetryFailureReason()
	prompt := instance.BuildRetryContinuationPrompt(inst, reason)
	if err := restartForRetry(inst, allowedPath, prompt, policy, stop); err != nil {
		if errors.Is(err, instance.ErrRetryInFlight) {
			// A racing restart (e.g. a concurrent manual RetryNow) is already
			// in flight — leave NextRetryAt set and retry on a later tick
			// rather than dropping this scheduled retry.
			return true, false
		}
		log.Warn("SessionDriver: automated retry restart failed",
			"session", inst.Title, "err", err,
		)
		return false, true
	}
	inst.ClearNextRetryAt()
	// restartForRetry spawned a fresh continuation goroutine on success; this
	// one's job is done.
	return false, true
}

// handleStoppedStatus handles the driver loop's `st == Stopped` branch: an
// unexpected or expected session exit. Takes st itself (rather than the
// caller gating on `st == Stopped` before calling) so the call site is a
// single flat if/else-if on this function's own (shouldContinue,
// shouldReturn) result, matching handleRetryPendingTick's pattern, instead of
// wrapping a second nested if/if pair inside `if st == Stopped { ... }`.
// shouldContinue and shouldReturn tell the caller which loop control-flow to
// apply; at most one is ever true — this mirrors handleInactivityTick's
// existing signal pattern, needed since a scheduled retry (evaluateSessionRetry's
// "scheduled" decision, reached via handleDriverFailure) must let the poll
// loop keep ticking so its own NextRetryAt gate can fire the actual restart
// once the backoff delay elapses, rather than this function always being
// terminal.
func handleStoppedStatus(ctx context.Context, inst *instance.Instance, st domain.Status, allowedPath string, initialPrompt string, policy instance.RetryPolicy, stop <-chan struct{}, sentInitial bool, initialPromptSentAt time.Time) (shouldContinue, shouldReturn bool) {
	if st != domain.Stopped {
		return false, false
	}
	if !sentInitial {
		// Exited before we even sent the first prompt — likely a startup crash.
		// One-shot sessions (backlog:triage/backlog:review) are never retried.
		if instance.IsOneShot(inst) {
			return false, true
		}
		log.Warn("SessionDriver: session exited before initial prompt sent",
			"session", inst.Title,
		)
		return handleDriverFailure(inst, allowedPath, policy, instance.ClassifyFailureReason(inst), stop)
	}
	// Stopped after initial prompt was sent.
	if inst.OneShot {
		tryExtractClaudeSessionID(ctx, inst)
	}
	if instance.IsOneShot(inst) {
		// One-shot sessions: BacklogLifecycleListener handles this; driver exits cleanly.
		return false, true
	}
	if initialPrompt == "" {
		// No initial prompt was ever configured for this session (e.g. a plain
		// terminal/bash session created without a task) -- sentInitial's "no
		// prompt to send" bookkeeping trivially satisfies `st == Stopped` the
		// moment the session ends, at any time, indistinguishable here from a
		// real AI-agent task interrupted mid-run. There is no in-flight task to
		// protect by retrying, so a user-initiated `exit` (or any other exit)
		// must be left as a clean Stopped, not silently respawned underneath
		// them moments later.
		return false, true
	}
	// CONCERN-2 guard: only restart if the session crashed quickly (within 5 minutes
	// of sending the initial prompt). A session that ran for > 5 minutes and then stopped
	// has likely completed its task normally. BacklogLifecycleListener handles that transition.
	if time.Since(initialPromptSentAt) > driverMinRuntimeBeforeRetry {
		log.Info("SessionDriver: session exited after minimum runtime, treating as completion",
			"session", inst.Title,
			"runtime", time.Since(initialPromptSentAt).Round(time.Second),
		)
		if inst.OneShot {
			tryExtractClaudeSessionID(ctx, inst)
		}
		return false, true
	}
	log.Warn("SessionDriver: unexpected session exit after initial prompt",
		"session", inst.Title,
	)
	return handleDriverFailure(inst, allowedPath, policy, instance.ClassifyFailureReason(inst), stop)
}

// handleStartupDialogTick answers a detected startup dialog and reports
// whether the driver loop's current tick should `continue` immediately.
//
// Control-flow requirement (ADR-001): only dialogUnanswered (a send was just
// attempted this tick, whether it succeeded or is still under the retry cap)
// keeps the single-tick continue. Once the latch reaches
// dialogAwaitingDismissal or dialogGaveUp, the caller falls through to the
// rest of the loop body exactly as if isStartupDialog had been false this
// tick — restoring Ready-detection, the inactivity-timeout escalation, and
// the NeedsApproval check for ticks after the dialog has been answered or
// abandoned. Without this, a dialogGaveUp session would silently wedge here
// until the driver ends (or its stall flag fires) with no operator escalation.
//
// Exhaustive switch (not a plain if) so a future 4th dialogLatchStatus value
// can't silently fall through the implicit "unanswered" path and reintroduce
// the original unbounded-resend bug.
func handleStartupDialogTick(inst *instance.Instance, tailed string, startupLatch *dialogAnswerState, sendAnswerKey func() error) bool {
	status := answerDialogOnce(startupLatch, tailed, sendAnswerKey, inst.Title, "startup dialog")

	switch status {
	case dialogUnanswered:
		return true
	case dialogAwaitingDismissal, dialogGaveUp:
		return false
	default:
		panic(fmt.Sprintf("SessionDriver: unhandled dialogLatchStatus %d from answerDialogOnce (startup dialog)", status))
	}
}

// sendInitialPromptTick handles one driver-loop tick's worth of initial-prompt
// injection: waiting for an idle prompt, re-checking whether a conversation
// already started underneath us, sending the prompt, and read-back
// verification. The caller always `continue`s the loop after calling this.
func sendInitialPromptTick(ctx context.Context, inst *instance.Instance, initialPrompt string, output string, detectedSt detection.DetectedStatus, readyDeadline time.Time, sentInitial *bool, initialPromptSentAt *time.Time, sendAttempts *int) {
	// markSent records every "we're done trying to send" transition below
	// through the same path so it's always durably persisted (via
	// Instance.SetInitialPromptSentAt's injected repo) -- not just reflected
	// in this tick's local pointers, which reset on the next service restart.
	markSent := func() {
		now := time.Now()
		*sentInitial = true
		*initialPromptSentAt = now
		inst.SetInitialPromptSentAt(now)
	}

	// Wait for StatusIdle specifically: the `^>\s*▌?\s*$` pattern confirms
	// Claude Code's readline is showing the input prompt and is listening.
	//
	// Do NOT use st == Ready (which equals st == Active — a deprecated alias
	// that fires the moment the session starts, long before readline is ready).
	// StatusReady is a `.*` catch-all; StatusIdle is the precise signal.
	claudeAtPrompt := detectedSt == detection.StatusIdle
	timedOut := time.Now().After(readyDeadline)

	// On timeout, skip if Claude is known to be actively processing — injecting
	// while busy writes text into the PTY buffer without Enter ever submitting it,
	// since readline isn't active. Keep waiting; claudeAtPrompt will fire when Claude
	// finishes and returns to the input prompt.
	claudeIsKnownBusy := detectedSt == detection.StatusProcessing ||
		detectedSt == detection.StatusExecuting ||
		detectedSt == detection.StatusWaitingForAgent

	if !claudeAtPrompt && (!timedOut || claudeIsKnownBusy) {
		return
	}

	// Re-check whether a conversation is already active before injecting.
	// The user may have typed something manually while we were waiting.
	//
	// Check live PTY output first (no disk I/O, no flush latency), then
	// fall back to the JSONL file check for cases not visible in the terminal
	// buffer (e.g. the session just started and the buffer hasn't scrolled yet).
	if outputShowsConversationStarted(output) {
		log.Info("SessionDriver: terminal output shows conversation already active, skipping injection",
			"session", inst.Title,
		)
		markSent()
		return
	}

	if _, convErr := FindConversationFilePath(ctx, inst.GetStableID()); convErr == nil {
		log.Info("SessionDriver: conversation file exists, skipping initial prompt injection",
			"session", inst.Title,
		)
		markSent()
		return
	}

	// The lease is taken BEFORE the attempt counter: a busy lease must not burn
	// one of the three attempts after which the prompt is dropped for good.
	lease, leaseOK := inst.TryTerminalWriteLease(instance.LeaseWriterDriver)
	if !leaseOK {
		log.Debug("SessionDriver: write lease busy, deferring initial prompt", "session", inst.Title)
		return
	}

	*sendAttempts++

	if timedOut && !claudeAtPrompt {
		log.Warn("SessionDriver: timed out waiting for idle prompt, sending anyway",
			"session", inst.Title,
			"attempt", *sendAttempts,
		)
	}

	if claudeAtPrompt {
		// Brief settling pause after the > prompt appears: the status machine
		// detected the line, but readline's internal input handler may need a
		// few hundred ms to be fully listening.
		time.Sleep(300 * time.Millisecond)
	}

	// Snapshot terminal content immediately before sending so we can verify
	// that the keystrokes were actually received (read-back confirmation).
	contentBefore, _ := inst.PreviewContext(ctx)

	if err := submitInitialPrompt(ctx, inst, lease, initialPrompt); err != nil {
		if errors.Is(err, instance.ErrNoLease) || errors.Is(err, instance.ErrLeaseMismatch) {
			// A programming error (the lease was acquired just above), not a
			// failed delivery: it must neither count toward the attempt limit
			// nor drop the prompt.
			*sendAttempts--
			log.Error("SessionDriver: lease API refused the initial prompt write", "session", inst.Title, "err", err)
			return
		}
		if errors.Is(err, ErrPromptUndeliverable) {
			// Deterministic: retrying cannot help, so stop and tell the operator.
			log.Error("SessionDriver: initial prompt cannot be delivered; not sending it",
				"session", inst.Title, "promptLen", len(initialPrompt), "err", err)
			instance.MarkSessionNeedsAttention(inst, "initial prompt not delivered: "+err.Error())
			markSent()
			return
		}
		log.Warn("SessionDriver: failed to send initial prompt",
			"session", inst.Title,
			"claudeAtPrompt", claudeAtPrompt,
			"timedOut", timedOut,
			"attempt", *sendAttempts,
			"err", err,
		)
		if *sendAttempts >= 3 {
			log.Error("SessionDriver: giving up on initial prompt after 3 failed attempts",
				"session", inst.Title,
			)
			markSent()
		}
		// sentInitial stays false → retry next tick
		return
	}

	log.Info("SessionDriver: sent initial prompt",
		"session", inst.Title,
		"claudeAtPrompt", claudeAtPrompt,
		"timedOut", timedOut,
		"attempt", *sendAttempts,
		"promptLen", len(initialPrompt),
	)

	// Read-back verification: only when we had a confirmed idle prompt
	// and still have retries left.  After a timeout-triggered send we
	// cannot reliably verify (Claude may not be at a prompt).
	if !claudeAtPrompt || *sendAttempts >= 3 {
		// Timeout-triggered send or max retries: accept without verification.
		markSent()
		return
	}

	// Wait for PTY echo + pane-capture latency before reading back. Adaptive
	// (poll for an actual pane change, up to 1.5s) rather than a fixed sleep:
	// a fixed 500ms sleep is a race — a slower-than-usual capture-pane round
	// trip under load makes a genuinely successful send look "swallowed,"
	// which resent the identical prompt on the next tick (the user-visible
	// "same message delivered repeatedly" bug this replaces the sleep to fix).
	// Only fall back to the exact-content comparison (the real swallow case)
	// if the pane never reported any change at all within the wait window.
	if !instance.WaitForPaneUpdate(ctx, inst, instance.DefaultPaneSettlePollInterval, 1500*time.Millisecond) {
		contentAfter, verifyErr := inst.PreviewContext(ctx)
		if verifyErr == nil && contentBefore != "" && contentAfter == contentBefore {
			// Terminal content identical to before the send — the
			// keystrokes were likely swallowed before readline consumed
			// them.  Retry on the next tick.
			log.Warn("SessionDriver: terminal content unchanged after send — keystrokes may have been swallowed, retrying",
				"session", inst.Title,
				"attempt", *sendAttempts,
			)
			// sentInitial stays false
			return
		}
	}
	// Content changed (or verification read failed) — treat as success.
	log.Info("SessionDriver: read-back confirmed initial prompt received",
		"session", inst.Title,
		"attempt", *sendAttempts,
	)
	markSent()
}

func driverWaitingForOperator(inst *instance.Instance) bool {
	status := inst.GetDetectedStatus()
	if (instance.InstanceStatusInfo{ClaudeStatus: status}).IsWaitingForUser() {
		return true
	}
	if inst.HasTag(instance.TagBacklogWork) {
		return false
	}
	switch status {
	case detection.StatusIdle, detection.StatusReady, detection.StatusSuccess:
		return true
	default:
		return false
	}
}

func handleInactivityTick(ctx context.Context, inst *instance.Instance, allowedPath string, policy instance.RetryPolicy, stop <-chan struct{}, attempt *driverAttempt, now time.Time) (shouldContinue, shouldReturn bool) {
	if driverWaitingForOperator(inst) {
		// Human response time is not agent inactivity; resumed work gets a fresh budget.
		attempt.startedAt = now
		attempt.nudgeSentAt = time.Time{}
		return false, false
	}
	idle := attempt.idleDuration(now, inst.LastMeaningfulOutputTime())
	if !isAutonomousSession(inst) {
		return handleInteractiveInactivity(inst, allowedPath, policy, stop, attempt, idle, now)
	}
	nudgeSentAt := &attempt.nudgeSentAt

	// For backlog work sessions: send a task-reminder nudge before restarting.
	// This preserves conversational context when Claude finishes but forgets to
	// call /backlog/review or report_progress.
	if inst.HasTag(instance.TagBacklogWork) && nudgeSentAt.IsZero() && idle > driverBacklogNudgeDelay {
		*nudgeSentAt = attemptBacklogNudge(ctx, inst, idle)
		return true, false
	}

	graceTimeout := driverInactivityTimeout
	if inst.HasTag(instance.TagBacklogWork) && !nudgeSentAt.IsZero() {
		graceTimeout = driverBacklogNudgeGrace
	}
	if idle > graceTimeout {
		logStallDecision(inst, gatherStallEvidence(inst, idle, graceTimeout), stallRestart, policy, now)
		log.Warn("SessionDriver: session stuck — no output for inactivity timeout",
			"session", inst.Title,
			"inactivity", idle.Round(time.Second),
		)
		return handleDriverFailure(inst, allowedPath, policy, "stalled", stop)
	}
	return false, false
}

// handleInteractiveInactivity applies the stall policy to a non-backlog
// session. It never restarts a live pane and never marks the session
// PermanentlyFailed for quiet alone (2026-11-05: nine live sessions were
// killed and failed this way). A dead pane still takes the failure path; a
// live one is flagged once per quiet episode.
func handleInteractiveInactivity(inst *instance.Instance, allowedPath string, policy instance.RetryPolicy, stop <-chan struct{}, attempt *driverAttempt, idle time.Duration, now time.Time) (shouldContinue, shouldReturn bool) {
	out := inst.LastMeaningfulOutputTime()
	if out.After(attempt.flaggedOutputAt) {
		attempt.flaggedOutputAt = time.Time{}
	}
	if idle <= driverInactivityTimeout {
		attempt.deadSince = time.Time{}
		return false, false
	}
	e := gatherStallEvidence(inst, idle, interactiveStallBudget(policy))
	v := decideStall(e)
	if v != stallPaneDead {
		attempt.deadSince = time.Time{}
	}
	switch v {
	case stallPaneDead:
		// A single failed probe can be transient (and the probe cannot tell an
		// error from "dead"), so fail only once probes have been dead for a while.
		if attempt.deadSince.IsZero() {
			attempt.deadSince = now
		}
		if now.Sub(attempt.deadSince) < deadPaneConfirmWindow {
			return false, false
		}
		logStallDecision(inst, e, v, policy, now)
		return handleDriverFailure(inst, allowedPath, policy, "tmux_exited", stop)
	case stallNeedsAttention:
		if attempt.flaggedOutputAt.IsZero() {
			attempt.flaggedOutputAt = out
			attempt.stallFlags++
			noteInteractiveStall(inst, e, policy, now)
		}
	}
	return false, false
}

// handleApprovalDialogTick watches for NeedsApproval to handle
// directory-access dialogs that AutoYes (-y) doesn't cover.
//
// Same DialogAnswerLatch defect class as handleStartupDialogTick (ADR-001),
// applied via an independently-scoped approvalLatch. Asymmetry note: unlike
// the startup-dialog branch, this call site has no `continue` today (it
// already falls through naturally to the end of the loop body regardless of
// outcome), so there is no analogous control-flow starvation risk to fix
// here — the returned status is only used for the latch's own bookkeeping,
// not for branching.
func handleApprovalDialogTick(inst *instance.Instance, hasOutput bool, tailed string, allowedPath string, approvalLatch *dialogAnswerState, sendAnswerKey func() error) {
	mgr := inst.GetStatusManager()
	if mgr == nil {
		return
	}
	si := mgr.GetStatus(inst)
	if si.ClaudeStatus != detection.StatusNeedsApproval {
		return
	}
	if hasOutput && shouldApprovePrompt(tailed, allowedPath) {
		answerDialogOnce(approvalLatch, tailed, sendAnswerKey, inst.Title, "approval prompt")
	}
}

// scannedPRURLs holds the PR URLs the terminal scan has already handled, so a URL
// that stays in the scrollback is not processed again on every tick.
type scannedPRURLs map[string]struct{}

// scanAndLinkPRURL scans terminal output for the PR URLs a git push or
// `gh pr create` printed and attaches each one that belongs to the session's own
// repository, so PRStatusPoller can track it without relying on branch-name
// discovery (which fails when the omnibar branch name differs from the PR head
// branch). Several URLs accumulate as separate links; seen remembers the ones
// already handled.
func scanAndLinkPRURL(inst *instance.Instance, sentInitial bool, seen scannedPRURLs, previewErr error, output string) {
	if !sentInitial || previewErr != nil || output == "" {
		return
	}
	for _, prURL := range scanTerminalForPushedPRURLs(output) {
		if _, done := seen[prURL]; done {
			continue
		}
		seen[prURL] = struct{}{}
		attachScannedPRURL(inst, prURL)
	}
}

// attachScannedPRURL attaches prURL only when its host, owner and repo equal the
// session's resolved ones; a session without a resolved repo attaches nothing.
func attachScannedPRURL(inst *instance.Instance, prURL string) {
	parsed, err := instance.ParseGitHubURL(prURL)
	if err != nil {
		return
	}
	gh := inst.Snapshot().GitHub
	want, wantErr := domain.NewPRRef(gh.GitHubHost, gh.GitHubOwner, gh.GitHubRepo, parsed.PRNumber)
	got, gotErr := domain.NewPRRef(parsed.Host, parsed.Owner, parsed.Repo, parsed.PRNumber)
	if wantErr != nil || gotErr != nil || want != got {
		log.Debug("SessionDriver: ignoring PR URL from terminal output that is not in the session's repository",
			"session", inst.Title, "url", prURL, "session_repo", gh.GitHubOwner+"/"+gh.GitHubRepo)
		return
	}
	inst.AttachScannedPR(prURL, parsed.PRNumber)
	log.Info("SessionDriver: auto-linked PR from terminal push output",
		"session", inst.Title, "pr", parsed.PRNumber, "url", prURL)
}

// submitInitialPrompt is a variable only so a test can force a lease-API error
// out of the otherwise unreachable ErrNoLease/ErrLeaseMismatch branch.
var submitInitialPrompt = func(ctx context.Context, inst *instance.Instance, lease *instance.HeldLease, prompt string) error {
	typed, err := ParseDriverPrompt(prompt)
	if err != nil {
		lease.Release()
		return err
	}
	return instance.SubmitDriverContent(ctx, inst, lease, typed.Keys(), instance.DefaultPaneSettlePollInterval, instance.DefaultPaneSettleMaxWait)
}

// sendAnswerKeyUnderLease types the affirmative key for a dialog. It is the
// chain's acquirer; a busy lease returns ErrLeaseBusy, which answerDialogOnce
// does not count as an attempt.
func sendAnswerKeyUnderLease(inst *instance.Instance) error {
	lease, ok := inst.TryTerminalWriteLease(instance.LeaseWriterDriver)
	if !ok {
		return instance.ErrLeaseBusy
	}
	defer lease.Release()
	return inst.SendKeys("1\n")
}

// attemptBacklogNudge sends a backlog work session the "you appear to have paused"
// task-reminder nudge and returns the value the caller should record as nudgeSentAt.
//
// It always returns a non-zero, current timestamp — on a failed send exactly the same
// as on a successful one. BUG-041: previously nudgeSentAt was only set in the success
// case, so an identical failing SendKeys call retried on every subsequent driver tick
// forever (live evidence: 392 consecutive failed sends over ~13 minutes against one
// dead-pane session). A SendKeys failure here (e.g. tmux "invalid argument") is very
// likely a dead/gone pane that retrying cannot fix, so this deliberately does not retry
// the nudge itself. Returning a non-zero time makes the caller's graceTimeout/idle check
// (which already fires once nudgeSentAt is non-zero) take over: after
// driverBacklogNudgeGrace of continued silence it logs "session stuck" and calls
// handleDriverFailure, which restarts the session once and marks it for human attention
// on a second failure — the give-up signal this nudge path previously lacked.
func attemptBacklogNudge(ctx context.Context, inst *instance.Instance, idle time.Duration) time.Time {
	nudge := "You appear to have paused. Run `/backlog/status` to see remaining " +
		"acceptance criteria. Mark each complete criterion with `/backlog/done-N`, " +
		"then submit with `/backlog/review` once all are done."
	lease, ok := inst.TryTerminalWriteLease(instance.LeaseWriterNudge)
	if !ok {
		// Another writer holds the pane: skip this tick and nudge on the next
		// (a zero time leaves the caller's nudgeSentAt unset).
		log.Debug("SessionDriver: write lease busy, skipping backlog nudge", "session", inst.Title)
		return time.Time{}
	}
	if sendErr := instance.SubmitDriverContent(ctx, inst, lease, nudge, instance.DefaultPaneSettlePollInterval, instance.DefaultPaneSettleMaxWait); sendErr != nil {
		log.Warn("SessionDriver: failed to send backlog nudge, will not retry — falling through to inactivity timeout",
			"session", inst.Title, "err", sendErr)
	} else {
		log.Info("SessionDriver: sent backlog nudge",
			"session", inst.Title,
			"idle", idle.Round(time.Second),
		)
	}
	return time.Now()
}

// handleDriverFailure is called when the driver detects a stuck or crashed
// session. It evaluates the resolved RetryPolicy against the instance's
// current RetryState via evaluateSessionRetry and branches:
//   - scheduled: sets NextRetryAt + appends a RetryAttemptRecord under
//     inst.mu; returns (true, false) so the caller's loop keeps ticking — the
//     poll loop's own NextRetryAt gate (above) performs the actual restart
//     once the backoff delay elapses.
//   - restartGrace: restarts immediately via restartForRetry without
//     consuming an attempt (a routine service restart, not a real crash);
//     returns (false, true) — restartForRetry already spawned a fresh
//     continuation goroutine on success.
//   - notEligible / exhausted: transitions to PermanentlyFailed via
//     markSessionPermanentlyFailed; returns (false, true).
//
// The caller must apply the returned signal immediately (continue/return),
// mirroring handleInactivityTick's existing pattern.
func handleDriverFailure(inst *instance.Instance, allowedPath string, policy instance.RetryPolicy, reason string, stop <-chan struct{}) (shouldContinue, shouldReturn bool) {
	// An archived session is deliberately retired — archiveItemWorkSessions
	// kills its pane without stopping this driver, so the kill arrives here
	// looking like a crash. Never answer that with a restart (ADR-001).
	// Returns "this goroutine is done" rather than falling through to
	// markSessionPermanentlyFailed: writing a failure status and firing a
	// notification for a session the system already retired is noise.
	if inst.IsArchived() {
		log.Info("SessionDriver: session is archived; not retrying", "session", inst.Title, "reason", reason)
		return false, true
	}

	now := time.Now()
	rs := inst.Snapshot().Retry

	switch instance.EvaluateSessionRetry(rs, policy, reason, now, ServerStartTime) {
	case instance.RetryDecisionScheduled:
		nextAttempt := rs.RetryAttempt + 1
		delay := instance.BackoffDelay(rs.RetryAttempt, policy.InitialDelay, policy.MaxDelay, instance.DefaultJitterFraction)
		inst.SetRetryProgress(func(st *domain.RetryState) {
			st.RetryAttempt = nextAttempt
			st.LastFailureReason = reason
			st.NextRetryAt = now.Add(delay)
			st.RecordAttempt(nextAttempt, reason, now)
		})
		log.Info("SessionDriver: scheduling retry after failure",
			"session", inst.Title, "reason", reason,
			"attempt", nextAttempt, "max_attempts", rs.RetryMaxAttempts, "delay", delay,
		)
		return true, false

	case instance.RetryDecisionRestartGrace:
		log.Info("SessionDriver: restart-grace — server booted recently, restarting without consuming a retry attempt",
			"session", inst.Title, "reason", reason,
		)
		inst.SetRetryProgress(func(st *domain.RetryState) { st.LastFailureReason = reason })
		prompt := instance.BuildRetryContinuationPrompt(inst, reason)
		if err := restartForRetry(inst, allowedPath, prompt, policy, stop); err != nil {
			log.Warn("SessionDriver: restart-grace restart failed",
				"session", inst.Title, "err", err,
			)
		}
		return false, true

	default: // retryDecisionNotEligible, retryDecisionExhausted
		instance.MarkSessionPermanentlyFailed(inst, reason)
		return false, true
	}
}

// answerDialogOnce implements the DialogAnswerLatch state machine (ADR-001):
// send at most once per unique dialog-content hash, with bounded
// retry-on-failure only (never retried once a send succeeds).
//
// output is tail-sliced to statusDetectionTailBytes and whitespace-normalized
// before hashing — mirroring GetCurrentStatus's existing tail-then-hash
// precedent (claude_controller.go:528) — so the comparison is scoped to what
// is actually still near-current on screen (not the session's entire
// scrollback history) and is immune to incidental line-wrap/whitespace
// jitter between polling ticks. Callers may pass either the raw Preview()
// output or an already-tailed value (e.g. the same `tailed` they computed
// for their own isStartupDialog/shouldApprovePrompt match) — tailContent is
// idempotent on already-short input, so tailing twice is harmless and this
// function's own tailing is what its unit tests (TestAnswerDialogOnce cases
// f/g) exercise directly, independent of any caller-side tailing.
//
// Returns the latch's resulting status after this tick's transition, so the
// call site can decide whether to keep short-circuiting the rest of the poll
// tick or fall through to it (see ADR-001's "Control flow" section).
func answerDialogOnce(state *dialogAnswerState, output string, send func() error, sessionTitle, logContext string) dialogLatchStatus {
	tailed := tailContent(output, statusDetectionTailBytes)
	normalized := strings.Join(strings.Fields(tailed), " ")
	hash := hashString(normalized)

	if hash != state.hash {
		// New dialog, or this dialog was dismissed and a different one
		// appeared with the same call site — re-arm the latch.
		state.hash = hash
		state.status = dialogUnanswered
		state.attempts = 0
	}

	// Exhaustive switch (not a plain if) so a future 4th dialogLatchStatus
	// value can't silently fall through the implicit "unanswered" path below
	// and reintroduce the original unbounded-resend bug.
	switch state.status {
	case dialogAwaitingDismissal, dialogGaveUp:
		return state.status
	case dialogUnanswered:
		// Proceed below: this hash has not yet been successfully answered
		// (or is still within its retry budget) — attempt the send.
	default:
		panic(fmt.Sprintf("answerDialogOnce: unhandled dialogLatchStatus %d for %s", state.status, logContext))
	}

	if err := send(); err != nil {
		if errors.Is(err, instance.ErrLeaseBusy) {
			// Another writer holds the pane: not a failed attempt, or three
			// busy ticks would abandon the dialog for good (sticky gave-up).
			log.Debug("SessionDriver: write lease busy, will retry "+logContext, "session", sessionTitle)
			return state.status
		}
		state.attempts++
		log.Warn("SessionDriver: failed to answer "+logContext,
			"session", sessionTitle,
			"attempt", state.attempts,
			"err", err,
		)
		if state.attempts >= maxDialogAnswerAttempts {
			state.status = dialogGaveUp
			log.Warn("SessionDriver: giving up on "+logContext+" after repeated send failures",
				"session", sessionTitle,
				"attempts", state.attempts,
			)
		}
		return state.status
	}

	log.Info("SessionDriver: answered "+logContext,
		"session", sessionTitle,
	)
	state.status = dialogAwaitingDismissal
	return state.status
}

// isStartupDialog returns true when output contains a Claude Code startup
// interactive prompt that requires a numbered-menu response (e.g. the
// "Do you trust this folder?" safety check shown on first launch in a new
// working directory).
func isStartupDialog(output string) bool {
	lower := strings.ToLower(output)
	return (strings.Contains(lower, "trust this folder") ||
		strings.Contains(lower, "is this a project you created") ||
		strings.Contains(lower, "quick safety check")) &&
		// Must have a numbered option to select — avoids false positives on
		// non-interactive output that merely mentions trust.
		(strings.Contains(output, "1.") || strings.Contains(output, "❯ 1"))
}

// outputShowsConversationStarted returns true when live terminal content
// indicates that a Claude Code conversation has already begun in this session.
// It checks for signals that are exclusive to in-progress or completed exchanges.
//
// This uses live PTY buffer content (via inst.Preview()) with no disk I/O, making
// it more up-to-date than FindConversationFilePath which depends on JSONL flush latency.
func outputShowsConversationStarted(output string) bool {
	// "esc to interrupt" is shown exclusively while Claude is actively processing.
	if strings.Contains(output, "esc to interrupt") {
		return true
	}

	// Spinner time suffix e.g. "(4m 18s ·" — only present in active work spinner.
	if spinnerTimeRe.MatchString(output) {
		return true
	}

	// Cost summary printed after a completed exchange: "⎿  $0.42" or "Cost: $X.XX".
	if strings.Contains(output, "⎿  $") || strings.Contains(output, "Cost: $") {
		return true
	}

	// Baked/resumption markers used by /loop mode.
	if strings.Contains(output, "◉ Baked for") || strings.Contains(output, "◉ Claude resuming") {
		return true
	}

	// Past-tense completion verb e.g. "✻ Perambulated for 1h 5m" — printed after
	// an active work session finishes and Claude returns to the idle readline prompt.
	if completionVerbRe.MatchString(output) {
		return true
	}

	return false
}

// scanTerminalForPushedPRURLs returns, oldest first, the GitHub PR URLs on the
// last 30 lines of terminal output that git push (optionally "remote:"-prefixed)
// or `gh pr create` printed on a line of their own. A URL mentioned inside other
// text is not a push result and is ignored.
func scanTerminalForPushedPRURLs(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var urls []string
	for _, line := range lines[max(0, len(lines)-30):] {
		word := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "remote:"))
		if strings.ContainsAny(word, " \t") || !strings.Contains(word, "github.com/") || !strings.Contains(word, "/pull/") {
			continue
		}
		if ref, err := instance.ParseGitHubURL(word); err == nil && ref.PRNumber > 0 {
			urls = append(urls, word)
		}
	}
	return urls
}

// tryExtractClaudeSessionID reads the terminal output for a completed OneShot
// session and stores the extracted Claude session_id on the instance so that
// future restarts use --resume.
//
// Takes the driver loop's ctx (not inst.Preview()'s implicit
// context.Background()) so this capture-pane subprocess is cancelled the
// moment StopSessionDriver/Destroy() fires, instead of blocking the driver
// goroutine's defer cancel() from ever running — see PreviewContext's doc
// comment (instance_terminal.go).
func tryExtractClaudeSessionID(ctx context.Context, inst *instance.Instance) {
	if !inst.OneShot {
		return
	}
	output, err := inst.PreviewContext(ctx)
	if err != nil || output == "" {
		return
	}
	uuid := instance.ParseClaudeSessionID(output)
	if uuid == "" {
		return
	}
	inst.SetClaudeConversationUUID(uuid)
	log.Info("SessionDriver: captured claude session_id", "session", inst.Title, "session_id", uuid)
}

// shouldApprovePrompt returns true when the terminal output looks like a
// directory-access dialog for a path under allowedPath.
func shouldApprovePrompt(output, allowedPath string) bool {
	lower := strings.ToLower(output)
	hasAccess := strings.Contains(lower, "allow reading") ||
		strings.Contains(lower, "allow writing") ||
		strings.Contains(lower, "allow executing") ||
		strings.Contains(lower, "do you want to proceed")

	if !hasAccess {
		return false
	}

	// Only approve if the dialog is about a path the session is authorised for.
	if allowedPath == "" {
		return true
	}
	return strings.Contains(output, allowedPath)
}

// restartForRetry is the single choke point every restart path (automated
// backoff-expiry, restart-grace, manual RetryNow) goes through. It claims
// retryInFlight via CAS before doing anything else — a caller whose CAS fails
// gets ErrRetryInFlight without touching RetryState or attempting any
// restart, which is the whole guarantee retryInFlight exists for.
//
// stop is the stop channel the new driver goroutine should watch. Pass the
// current goroutine's own stop channel when calling from within a running
// driver loop (the standard "handleDriverFailure spawns a continuation"
// pattern); pass nil when calling from outside any driver loop (RetryNow) —
// restartForRetry then reuses the instance's existing stopper, or creates one
// if none exists yet.
func restartForRetry(inst *instance.Instance, allowedPath, continuationPrompt string, policy instance.RetryPolicy, stop <-chan struct{}) error {
	releaseRetry, claimed := inst.TryBeginRetry()
	if !claimed {
		return instance.ErrRetryInFlight
	}
	defer releaseRetry()

	var restartErr error
	if inst.IsHotRestoreRecoverable() {
		//nolint:noarchivedrevival automated callers check IsArchived first; RetryNow is an explicit user action
		inst.RecoverFromStopped()
		// A prior failure may have been the session's worktree directory vanishing
		// (see RecreateWorktreeIfMissing); a failure there fails the retry now.
		if setupErr := inst.RecreateWorktreeIfMissing(); setupErr != nil {
			restartErr = fmt.Errorf("failed to recreate missing worktree before retry: %w", setupErr)
		} else {
			// Clear the old (possibly dead) controller so StartController below creates a fresh one.
			inst.StopController()
			//nolint:noarchivedrevival same callers as RecoverFromStopped above
			restartErr = inst.Start(false)
		}
		if restartErr == nil {
			if ctrlErr := inst.StartController(); ctrlErr != nil {
				log.Warn("SessionDriver: failed to restart controller after retry restart",
					"session", inst.Title, "err", ctrlErr)
			}
		}
	} else {
		restartErr = inst.Restart(false, "")
	}

	if restartErr != nil {
		log.Error("SessionDriver: retry restart failed; marking for attention",
			"session", inst.Title, "err", restartErr,
		)
		instance.MarkSessionNeedsAttention(inst, "restart error: "+restartErr.Error())
		return restartErr
	}

	// Register before spawning to close the race window between an exiting
	// goroutine's driverRunning bookkeeping and the new one starting
	// (mirrors handleDriverFailure's existing D3 mitigation).
	driverStop, ok := inst.RegisterDriverContinuation(stop)
	if !ok {
		// Destroy already ran. The restart above has already happened; it is cleaned up only when
		// this ran on a driver goroutine that StopSessionDriver waited for (within driverStopTimeout).
		log.Info("SessionDriver: instance destroyed, not spawning retry continuation", "session", inst.Title)
		return nil
	}
	go func() {
		defer inst.FinishDriverGoroutine()
		runSessionDriverWithPrompt(inst, allowedPath, continuationPrompt, policy, driverStop)
	}()
	return nil
}
