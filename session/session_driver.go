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
	"strings"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
)

const (
	driverPollInterval  = 2 * time.Second
	driverReadyTimeout  = 30 * time.Second
	driverTotalTimeout  = 25 * time.Minute // safety-net; inactivity detection is the real mechanism
	driverInitialPrompt = "Please proceed with the task described in your instructions."

	// driverInactivityTimeout is how long a session can be in the Ready state with no output
	// before it is considered stuck and restarted. Only fires after the initial prompt is sent.
	driverInactivityTimeout = 10 * time.Minute

	// driverMinRuntimeBeforeRetry is the minimum time after sending the initial prompt before
	// an unexpected exit is treated as a crash worth retrying. Sessions that ran longer than
	// this are assumed to have completed their task normally.
	driverMinRuntimeBeforeRetry = 5 * time.Minute

	// Continuation prompt constants.
	driverContinuationMaxMessages = 10
	driverContinuationMaxChars    = 500

	// maxDialogAnswerAttempts bounds retry-on-failure for a DialogAnswerLatch
	// (ADR-001): a SendKeys failure against the *same* dialog content hash is
	// retried up to this many times before the latch gives up (dialogGaveUp).
	// A hash change (a genuinely new/different dialog) resets the counter, so
	// this does not bound legitimate re-answering of a later, different dialog.
	maxDialogAnswerAttempts = 3
)

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

// StartSessionDriver launches a background goroutine that drives the session
// through its startup dialogs, fires the initial task prompt, and monitors
// for approval dialogs throughout the session lifetime.
//
// allowedPath is the session's repo/workspace path — directory-access approval
// dialogs that mention this path are auto-approved.
//
// Calling StartSessionDriver twice on the same instance is safe: the second call
// is a no-op (the idempotency guard uses atomic.Bool.CompareAndSwap).
func StartSessionDriver(inst *Instance, allowedPath string) {
	if !inst.driverRunning.CompareAndSwap(false, true) {
		log.Debug("SessionDriver: driver already running for session, skipping duplicate start",
			"session", inst.Title,
		)
		return
	}
	go func() {
		defer inst.driverRunning.Store(false)
		runSessionDriver(inst, allowedPath)
	}()
}

// runSessionDriver is the thin wrapper that creates the retried flag and
// delegates to runSessionDriverWithPrompt.
func runSessionDriver(inst *Instance, allowedPath string) {
	var retried atomic.Bool
	runSessionDriverWithPrompt(inst, allowedPath, driverInitialPrompt, &retried)
}

// runSessionDriverWithPrompt is the core driver loop. It accepts a custom initial
// prompt (used by the retry path to inject a JSONL-derived continuation) and a
// pointer to the retried flag so the retry goroutine does not spawn a third generation.
func runSessionDriverWithPrompt(inst *Instance, allowedPath string, initialPrompt string, retried *atomic.Bool) {
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

	readyDeadline := time.Now().Add(driverReadyTimeout)
	totalDeadline := time.Now().Add(driverTotalTimeout)

	ticker := time.NewTicker(driverPollInterval)
	defer ticker.Stop()

	sentInitial := false
	var initialPromptSentAt time.Time

	// DialogAnswerLatch state (ADR-001), one per SendKeys("1\n") call site.
	// Independently scoped — the approval-prompt latch is not shared with the
	// startup-dialog latch (a buffer classified true by both detectors
	// simultaneously, though very unlikely given their disjoint phrase sets,
	// could fire both in one tick; see ADR-001/plan.md Risk Control).
	var startupLatch dialogAnswerState
	var approvalLatch dialogAnswerState

	for range ticker.C {
		if time.Now().After(totalDeadline) {
			return
		}

		// Use GetEffectiveStatus (acquires stateMutex.RLock) to avoid data races on Status.
		st := inst.GetEffectiveStatus()

		// Paused is always a clean stop for the driver.
		if st == Paused {
			return
		}

		// Stopped after sentInitial = potential unexpected exit.
		if st == Stopped {
			if !sentInitial {
				// Exited before we even sent the first prompt — likely a startup crash.
				// For one-shot sessions or if we've already retried, just exit.
				if isOneShot(inst) || retried.Load() {
					return
				}
				log.Warn("SessionDriver: session exited before initial prompt sent",
					"session", inst.Title,
				)
				handleDriverFailure(inst, allowedPath, retried, "exit before initial prompt")
				return
			}
			// Stopped after initial prompt was sent.
			if isOneShot(inst) || retried.Load() {
				// One-shot sessions: BacklogLifecycleListener handles this; driver exits cleanly.
				return
			}
			// CONCERN-2 guard: only restart if the session crashed quickly (within 5 minutes
			// of sending the initial prompt). A session that ran for > 5 minutes and then stopped
			// has likely completed its task normally. BacklogLifecycleListener handles that transition.
			if time.Since(initialPromptSentAt) > driverMinRuntimeBeforeRetry {
				log.Info("SessionDriver: session exited after minimum runtime, treating as completion",
					"session", inst.Title,
					"runtime", time.Since(initialPromptSentAt).Round(time.Second),
				)
				return
			}
			log.Warn("SessionDriver: unexpected session exit after initial prompt",
				"session", inst.Title,
			)
			handleDriverFailure(inst, allowedPath, retried, "unexpected exit")
			return
		}

		// Always check Preview for startup dialogs that appear before the status
		// machine reaches NeedsApproval — e.g. the trust-folder safety check.
		output, previewErr := inst.Preview()
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

		if hasOutput && isStartupDialog(tailed) {
			status := answerDialogOnce(&startupLatch, tailed, func() error {
				return inst.SendKeys("1\n")
			}, inst.Title, "startup dialog")

			// Control-flow requirement (ADR-001): only dialogUnanswered (a send
			// was just attempted this tick, whether it succeeded or is still
			// under the retry cap) keeps the single-tick continue. Once the
			// latch reaches dialogAwaitingDismissal or dialogGaveUp, fall
			// through to the rest of the loop body exactly as if
			// isStartupDialog had been false this tick — restoring
			// Ready-detection, the inactivity-timeout escalation, and the
			// NeedsApproval check for ticks after the dialog has been
			// answered or abandoned. Without this, a dialogGaveUp session
			// would silently wedge here until driverTotalTimeout (25 min)
			// with zero operator escalation.
			if status == dialogUnanswered {
				continue
			}
		}

		if !sentInitial {
			ready := st == Ready
			timedOut := time.Now().After(readyDeadline)

			if ready || timedOut {
				if err := inst.SendKeys(initialPrompt + "\n"); err != nil {
					log.Warn("SessionDriver: failed to send initial prompt",
						"session", inst.Title,
						"ready", ready,
						"timedOut", timedOut,
						"err", err,
					)
				} else {
					log.Info("SessionDriver: sent initial prompt",
						"session", inst.Title,
						"ready", ready,
						"timedOut", timedOut,
					)
				}
				sentInitial = true
				initialPromptSentAt = time.Now()
			}
			continue
		}

		// Inactivity detection: only after initial prompt sent.
		// Use GetEffectiveStatus() (acquires stateMutex.RLock) to avoid data race on Status field.
		if st == Ready {
			last := inst.LastMeaningfulOutputTime()
			if !last.IsZero() && time.Since(last) > driverInactivityTimeout {
				log.Warn("SessionDriver: session stuck — no output for inactivity timeout",
					"session", inst.Title,
					"inactivity", time.Since(last).Round(time.Second),
				)
				handleDriverFailure(inst, allowedPath, retried, "inactivity timeout")
				return
			}
		}

		// After the initial prompt is sent, watch for NeedsApproval to handle
		// directory-access dialogs that AutoYes (-y) doesn't cover.
		//
		// Same DialogAnswerLatch defect class as the startup-dialog branch
		// above (ADR-001), applied via an independently-scoped approvalLatch.
		// Asymmetry note: unlike the startup-dialog branch, this call site has
		// no `continue` today (it already falls through naturally to the end
		// of the loop body regardless of outcome), so there is no analogous
		// control-flow starvation risk to fix here — the returned status is
		// only used for the latch's own bookkeeping, not for branching.
		if mgr := inst.GetStatusManager(); mgr != nil {
			if si := mgr.GetStatus(inst); si.ClaudeStatus == detection.StatusNeedsApproval {
				if hasOutput && shouldApprovePrompt(tailed, allowedPath) {
					answerDialogOnce(&approvalLatch, tailed, func() error {
						return inst.SendKeys("1\n")
					}, inst.Title, "approval prompt")
				}
			}
		}
	}
}

// handleDriverFailure is called when the driver detects a stuck or crashed session.
// On first call (retried == false): restarts the session and starts a new driver
//
//	goroutine with the continuation prompt.
//
// On second call (retried == true): adds the session to the ReviewQueue and exits.
//
// The caller must return immediately after calling handleDriverFailure.
func handleDriverFailure(inst *Instance, allowedPath string, retried *atomic.Bool, reason string) {
	if !retried.CompareAndSwap(false, true) {
		// Already retried once. Mark for human attention.
		log.Warn("SessionDriver: session failed twice; marking for attention",
			"session", inst.Title, "reason", reason,
		)
		markSessionNeedsAttention(inst, reason)
		return
	}

	log.Info("SessionDriver: restarting session after failure",
		"session", inst.Title, "reason", reason,
	)

	// Build continuation prompt BEFORE restart (HistoryFilePath may clear after restart).
	continuationPrompt := buildContinuationPrompt(inst)

	// Restart the session.
	var restartErr error
	st := inst.GetEffectiveStatus()
	if st == Stopped {
		inst.RecoverFromStopped()
		restartErr = inst.Start(false)
	} else {
		restartErr = inst.Restart(false)
	}

	if restartErr != nil {
		log.Error("SessionDriver: restart failed; marking for attention",
			"session", inst.Title, "err", restartErr,
		)
		markSessionNeedsAttention(inst, "restart error: "+restartErr.Error())
		return
	}

	// Set driverRunning = true before spawning to close the race window between
	// the old goroutine's defer Store(false) and the new goroutine starting.
	// (Design Decision D3 mitigation.)
	inst.driverRunning.Store(true)

	// Start a new driver goroutine for the restarted session.
	// The new goroutine inherits the retried flag so it will not retry a second time.
	go runSessionDriverWithPrompt(inst, allowedPath, continuationPrompt, retried)
}

// markSessionNeedsAttention adds the instance to its ReviewQueue (if any)
// so the UI surfaces it for operator intervention.
//
// BLOCK-2 fix: ReviewQueue.Add takes *ReviewItem, not (string, string).
func markSessionNeedsAttention(inst *Instance, reason string) {
	rq := inst.GetReviewQueue()
	if rq == nil {
		log.Warn("SessionDriver: ReviewQueue not set on instance, cannot mark NeedsAttention",
			"session", inst.Title,
		)
		return
	}
	rq.Add(&ReviewItem{
		SessionID:    inst.UUID,
		SessionName:  inst.Title,
		Reason:       ReasonStale, // closest existing reason: "no output for extended period"
		Priority:     PriorityMedium,
		DetectedAt:   time.Now(),
		Context:      reason, // "inactivity timeout" or "unexpected exit" or "restart error: ..."
		Tags:         inst.GetTags(),
		Path:         inst.Path,
		Status:       inst.GetEffectiveStatus().String(),
		LastActivity: inst.LastMeaningfulOutputTime(),
	})
}

// buildContinuationPrompt reads the last N messages from the session's JSONL
// conversation log and produces a brief prompt summarizing the last assistant
// turn. Falls back to a generic prompt if the log is unavailable.
func buildContinuationPrompt(inst *Instance) string {
	histPath := inst.HistoryFilePath
	if histPath == "" {
		return "Your previous session exited unexpectedly. Please continue from where you left off."
	}

	msgs, err := readLastNMessagesFromFile(histPath, driverContinuationMaxMessages)
	if err != nil || len(msgs) == 0 {
		log.Warn("SessionDriver: could not read conversation history for continuation prompt",
			"session", inst.Title, "path", histPath, "err", err,
		)
		return "Your previous session exited unexpectedly. Please continue from where you left off."
	}

	// Find the last assistant message.
	// BLOCK-3 fix: ClaudeConversationMessage has a Role field (not Type).
	// Content is already a plain string after extractMsgContent processing — no
	// extractMsgText helper needed or available.
	var lastAssistant string
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			lastAssistant = msgs[i].Content
			break
		}
	}

	if lastAssistant == "" {
		return "Your previous session exited unexpectedly. Please continue from where you left off."
	}

	if len(lastAssistant) > driverContinuationMaxChars {
		lastAssistant = lastAssistant[:driverContinuationMaxChars] + "..."
	}

	return "Your session restarted after an unexpected exit. Your last message was:\n---\n" +
		lastAssistant + "\n---\nPlease continue from where you left off. " +
		"Do not re-introduce yourself or repeat completed work."
}

// isOneShot returns true for sessions that should NOT be auto-retried.
// Triage and review sessions run exactly once; retrying them could corrupt
// backlog state by re-triggering lifecycle transitions.
func isOneShot(inst *Instance) bool {
	return inst.HasTag("backlog:triage") || inst.HasTag("backlog:review")
}

// answerDialogOnce implements the DialogAnswerLatch state machine (ADR-001):
// send at most once per unique dialog-content hash, with bounded
// retry-on-failure only (never retried once a send succeeds).
//
// output is tail-sliced to statusDetectionTailBytes and whitespace-normalized
// before hashing — mirroring GetCurrentStatus's existing tail-then-hash
// precedent (claude_controller.go:528) — so the comparison is scoped to what
// is actually still near-current on screen (not the session's entire
// scrollback history) and is immune to incidental line-wrap/whitespace jitter
// between polling ticks.
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

	if state.status == dialogAwaitingDismissal || state.status == dialogGaveUp {
		return state.status
	}

	// dialogUnanswered: this hash has not yet been successfully answered
	// (or is still within its retry budget) — attempt the send.
	if err := send(); err != nil {
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
