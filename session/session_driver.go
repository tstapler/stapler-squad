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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
)

// spinnerTimeRe matches the time/token suffix of an active Claude Code spinner,
// e.g. "(4m 18s ·" or "(1h 5m 37s ·". Only present while Claude is processing.
var spinnerTimeRe = regexp.MustCompile(`\(\d+[hms]`)

// completionVerbRe matches past-tense spinner verbs printed when Claude finishes
// a work session, e.g. "✻ Perambulated for 1h 5m", "✽ Roosted for 9m", or
// "* Moonwalked for 30s". Matches any leading symbol variant.
var completionVerbRe = regexp.MustCompile(`[✻✽\*] \w+ed for \d+`)

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
	inst.driverWG.Add(1)
	go func() {
		defer inst.driverWG.Done()
		defer inst.driverRunning.Store(false)
		runSessionDriver(inst, allowedPath)
	}()
}

// JoinSessionDriver waits for any in-flight SessionDriver goroutine (including
// a handleDriverFailure-spawned restart) to exit, up to stopJoinTimeout. Tests
// should call this before relying on t.TempDir() cleanup, since a driver
// goroutine can otherwise outlive the temp dir it was launched against.
func JoinSessionDriver(inst *Instance) {
	if !waitGroupWithTimeout(&inst.driverWG, stopJoinTimeout) {
		log.Warn("JoinSessionDriver: driver goroutine did not exit within timeout; it may still be running",
			"session", inst.Title, "timeout", stopJoinTimeout)
	}
}

// sanitizeInitialPromptForTmux strips characters that would corrupt the tmux
// send-keys call: null bytes (which tmux silently drops in unpredictable ways),
// newlines and carriage returns (collapsed to spaces so the prompt stays on one
// line), and hard-limits the length to 4096 characters. Returns the trimmed
// result; an empty return value means the caller should fall back to the static
// driverInitialPrompt.
func sanitizeInitialPromptForTmux(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > 4096 {
		s = s[:4096]
		// Step back from the truncation point to avoid splitting a multi-byte UTF-8 rune.
		for !utf8.ValidString(s) && len(s) > 0 {
			s = s[:len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}

// runSessionDriver is the thin wrapper that creates the retried flag and
// delegates to runSessionDriverWithPrompt.
func runSessionDriver(inst *Instance, allowedPath string) {
	var retried atomic.Bool
	var initialPrompt string
	if inst.InitialPrompt != "" {
		sanitized := sanitizeInitialPromptForTmux(inst.InitialPrompt)
		if sanitized != "" {
			initialPrompt = sanitized
		}
	}
	runSessionDriverWithPrompt(inst, allowedPath, initialPrompt, &retried)
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

	// Once a PR URL is found in terminal output we stop scanning.
	// Pre-seed from the current in-memory state so we don't re-scan for sessions
	// that already have a linked PR (e.g. created from a PR URL in the omnibar).
	prURLLinked := inst.GitHubPRNumber > 0

	// nudgeSentAt records when the last backlog-work nudge was sent.
	// Zero means no nudge has been sent yet.
	var nudgeSentAt time.Time

	// No initial prompt configured — skip the send step; driver still handles
	// startup dialogs, auto-approval, and monitoring.
	sentInitial := initialPrompt == ""
	var initialPromptSentAt time.Time
	if sentInitial {
		initialPromptSentAt = time.Now()
	} else {
		// Check if the prompt was already delivered in a previous service run.
		// Use live terminal output first (no disk latency), then fall back to JSONL file.
		if startOutput, err := inst.Preview(); err == nil && outputShowsConversationStarted(startOutput) {
			sentInitial = true
			initialPromptSentAt = time.Now()
		} else if _, err := FindConversationFilePath(inst.GetStableID()); err == nil {
			sentInitial = true
			initialPromptSentAt = time.Now()
		}
	}
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
	sendAnswerKey := func() error { return inst.SendKeys("1\n") }

	for range ticker.C {
		if time.Now().After(totalDeadline) {
			return
		}

		// GetEffectiveStatus for lifecycle decisions (Paused, Stopped).
		// GetDetectedStatus for fine-grained terminal-content signals (Idle = readline prompt).
		st := inst.GetEffectiveStatus()
		detectedSt := inst.GetDetectedStatus()

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
			if inst.OneShot {
				tryExtractClaudeSessionID(inst)
			}
			if isOneShot(inst) || retried.Load() {
				// One-shot sessions: BacklogLifecycleListener handles this; driver exits cleanly.
				return
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
				if inst.OneShot {
					tryExtractClaudeSessionID(inst)
				}
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
			status := answerDialogOnce(&startupLatch, tailed, sendAnswerKey, inst.Title, "startup dialog")

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
			//
			// Exhaustive switch (not a plain if) so a future 4th
			// dialogLatchStatus value can't silently fall through the
			// implicit "unanswered" path and reintroduce the original
			// unbounded-resend bug.
			switch status {
			case dialogUnanswered:
				continue
			case dialogAwaitingDismissal, dialogGaveUp:
				// Fall through to the rest of the loop body — see comment above.
			default:
				panic(fmt.Sprintf("SessionDriver: unhandled dialogLatchStatus %d from answerDialogOnce (startup dialog)", status))
			}
		}

		if !sentInitial {
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

			if claudeAtPrompt || (timedOut && !claudeIsKnownBusy) {
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
					sentInitial = true
					initialPromptSentAt = time.Now()
					continue
				}

				if _, convErr := FindConversationFilePath(inst.GetStableID()); convErr == nil {
					log.Info("SessionDriver: conversation file exists, skipping initial prompt injection",
						"session", inst.Title,
					)
					sentInitial = true
					initialPromptSentAt = time.Now()
					continue
				}

				sendAttempts++

				if timedOut && !claudeAtPrompt {
					log.Warn("SessionDriver: timed out waiting for idle prompt, sending anyway",
						"session", inst.Title,
						"attempt", sendAttempts,
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
				contentBefore, _ := inst.Preview()

				if err := inst.SendKeys(initialPrompt + EnterKeySequence); err != nil {
					log.Warn("SessionDriver: failed to send initial prompt",
						"session", inst.Title,
						"claudeAtPrompt", claudeAtPrompt,
						"timedOut", timedOut,
						"attempt", sendAttempts,
						"err", err,
					)
					if sendAttempts >= 3 {
						log.Error("SessionDriver: giving up on initial prompt after 3 failed attempts",
							"session", inst.Title,
						)
						sentInitial = true
						initialPromptSentAt = time.Now()
					}
					// sentInitial stays false → retry next tick
				} else {
					log.Info("SessionDriver: sent initial prompt",
						"session", inst.Title,
						"claudeAtPrompt", claudeAtPrompt,
						"timedOut", timedOut,
						"attempt", sendAttempts,
						"promptLen", len(initialPrompt),
					)

					// Read-back verification: only when we had a confirmed idle prompt
					// and still have retries left.  After a timeout-triggered send we
					// cannot reliably verify (Claude may not be at a prompt).
					if claudeAtPrompt && sendAttempts < 3 {
						// Wait for PTY echo + pane-capture latency before reading back.
						time.Sleep(500 * time.Millisecond)
						contentAfter, verifyErr := inst.Preview()
						if verifyErr == nil && contentBefore != "" && contentAfter == contentBefore {
							// Terminal content identical to before the send — the
							// keystrokes were likely swallowed before readline consumed
							// them.  Retry on the next tick.
							log.Warn("SessionDriver: terminal content unchanged after send — keystrokes may have been swallowed, retrying",
								"session", inst.Title,
								"attempt", sendAttempts,
							)
							// sentInitial stays false
						} else {
							// Content changed (or verification read failed) — treat as success.
							log.Info("SessionDriver: read-back confirmed initial prompt received",
								"session", inst.Title,
								"attempt", sendAttempts,
							)
							sentInitial = true
							initialPromptSentAt = time.Now()
						}
					} else {
						// Timeout-triggered send or max retries: accept without verification.
						sentInitial = true
						initialPromptSentAt = time.Now()
					}
				}
			}
			continue
		}

		// Inactivity detection: only after initial prompt sent.
		// Use GetEffectiveStatus() (acquires stateMutex.RLock) to avoid data race on Status field.
		if st == Ready {
			last := inst.LastMeaningfulOutputTime()
			// Use the later of initialPromptSentAt or LastMeaningfulOutput as the activity
			// reference. After a service restart, LastMeaningfulOutput may be stale (loaded
			// from DB before the ReviewQueue poller had a chance to refresh it from live
			// terminal content). Using initialPromptSentAt as a floor prevents false inactivity
			// fires immediately after startup.
			activityRef := initialPromptSentAt
			if !last.IsZero() && last.After(initialPromptSentAt) {
				activityRef = last
			}
			if !nudgeSentAt.IsZero() && nudgeSentAt.After(activityRef) {
				activityRef = nudgeSentAt
			}

			idle := time.Since(activityRef)

			// For backlog work sessions: send a task-reminder nudge before restarting.
			// This preserves conversational context when Claude finishes but forgets to
			// call /backlog/review or report_progress.
			if inst.HasTag(TagBacklogWork) && nudgeSentAt.IsZero() && idle > driverBacklogNudgeDelay {
				nudgeSentAt = attemptBacklogNudge(inst, idle)
				continue
			}

			graceTimeout := driverInactivityTimeout
			if inst.HasTag(TagBacklogWork) && !nudgeSentAt.IsZero() {
				graceTimeout = driverBacklogNudgeGrace
			}
			if idle > graceTimeout {
				log.Warn("SessionDriver: session stuck — no output for inactivity timeout",
					"session", inst.Title,
					"inactivity", idle.Round(time.Second),
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
					answerDialogOnce(&approvalLatch, tailed, sendAnswerKey, inst.Title, "approval prompt")
				}
			}
		}

		// Scan terminal output for a GitHub PR URL printed by git push.
		// This auto-links the PR to the session so PRStatusPoller can track it
		// without relying on branch-name discovery (which fails when the omnibar
		// branch name differs from the PR head branch).
		if sentInitial && !prURLLinked && inst.GitHubOwner != "" && previewErr == nil && output != "" {
			if prURL, prNum := scanTerminalForPRURL(output); prURL != "" {
				inst.mu.Lock()
				inst.GitHubPRURL = prURL
				inst.GitHubPRNumber = prNum
				inst.mu.Unlock()
				log.Info("SessionDriver: auto-linked PR from terminal push output",
					"session", inst.Title, "pr", prNum, "url", prURL)
				prURLLinked = true
			}
		}
	}
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
func attemptBacklogNudge(inst *Instance, idle time.Duration) time.Time {
	nudge := "You appear to have paused. Run `/backlog/status` to see remaining " +
		"acceptance criteria. Mark each complete criterion with `/backlog/done-N`, " +
		"then submit with `/backlog/review` once all are done."
	if sendErr := inst.SendKeys(nudge + EnterKeySequence); sendErr != nil {
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
	// If no conversation history exists yet (early PTY exit before Claude started), use the
	// original InitialPrompt so the workflow task is not lost on the first retry.
	continuationPrompt := buildContinuationPrompt(inst)
	if continuationPrompt == "Your previous session exited unexpectedly. Please continue from where you left off." &&
		inst.InitialPrompt != "" {
		if sanitized := sanitizeInitialPromptForTmux(inst.InitialPrompt); sanitized != "" {
			continuationPrompt = sanitized
		}
	}

	// Restart the session.
	var restartErr error
	st := inst.GetEffectiveStatus()
	if st == Stopped {
		inst.RecoverFromStopped()
		// Clear the old (possibly dead) controller so StartController below creates a fresh one.
		inst.StopController()
		restartErr = inst.Start(false)
		if restartErr == nil {
			// Start(false) skips controller setup (reserved for the server wiring path).
			// Restart it explicitly so the session driver can detect the Claude prompt.
			if ctrlErr := inst.StartController(); ctrlErr != nil {
				log.Warn("SessionDriver: failed to restart controller after session restart",
					"session", inst.Title, "err", ctrlErr)
			}
		}
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
	inst.driverWG.Add(1)
	go func() {
		defer inst.driverWG.Done()
		runSessionDriverWithPrompt(inst, allowedPath, continuationPrompt, retried)
	}()
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

// scanTerminalForPRURL searches terminal scrollback output for a GitHub PR URL
// of the form https://github.com/owner/repo/pull/NNN (as printed by git push).
// Returns ("", 0) if not found.
func scanTerminalForPRURL(output string) (string, int) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-30; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, "github.com/") || !strings.Contains(line, "/pull/") {
			continue
		}
		for _, word := range strings.Fields(line) {
			word = strings.Trim(word, ".,;:\"'()")
			if strings.Contains(word, "github.com/") && strings.Contains(word, "/pull/") {
				if ref, err := ParseGitHubURL(word); err == nil && ref.PRNumber > 0 {
					return word, ref.PRNumber
				}
			}
		}
	}
	return "", 0
}

// parseJSONField extracts a string field value from a JSON blob.
// Works with both --output-format json (single object) and stream-json (one
// JSON object per line). Searches recursively through nested objects, so it
// handles both top-level fields (e.g. "result") and nested fields (e.g.
// "session_id" inside a "data" sub-object). Returns empty string if the field
// is not found or its value is not a string.
func parseJSONField(output, field string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, field) {
			continue
		}
		var tree interface{}
		if err := json.Unmarshal([]byte(line), &tree); err != nil {
			continue
		}
		if s := searchJSONString(tree, field); s != "" {
			return s
		}
	}
	return ""
}

// searchJSONString recursively searches a parsed JSON tree for the first
// occurrence of a string-valued field with the given key.
func searchJSONString(v interface{}, field string) string {
	switch val := v.(type) {
	case map[string]interface{}:
		if s, ok := val[field]; ok {
			if str, ok := s.(string); ok {
				return str
			}
		}
		for _, child := range val {
			if s := searchJSONString(child, field); s != "" {
				return s
			}
		}
	case []interface{}:
		for _, item := range val {
			if s := searchJSONString(item, field); s != "" {
				return s
			}
		}
	}
	return ""
}

// parseClaudeSessionID extracts the "session_id" value from a JSON blob
// (both --output-format json and --output-format stream-json).
// Returns empty string if not found.
func parseClaudeSessionID(output string) string {
	return parseJSONField(output, "session_id")
}

// tryExtractClaudeSessionID reads the terminal output for a completed OneShot
// session and stores the extracted Claude session_id on the instance so that
// future restarts use --resume.
func tryExtractClaudeSessionID(inst *Instance) {
	if !inst.OneShot {
		return
	}
	output, err := inst.Preview()
	if err != nil || output == "" {
		return
	}
	uuid := parseClaudeSessionID(output)
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
