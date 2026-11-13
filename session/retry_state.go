package session

// retry_state.go implements the configurable multi-attempt crash/stall retry
// policy (session-retry-backoff): RetryState tracks the automated retry
// lifecycle on an Instance, backoffDelay computes exponential-backoff-with-jitter
// delays, and evaluateSessionRetry is the pure decision function mirroring
// backlog_remediation.go's evaluateRemediation shape. restartForRetry is the
// single retryInFlight-guarded choke point every restart path (automated
// backoff-expiry, restart-grace, manual RetryNow) goes through.

import (
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/queue"
)

// RetryPolicy is the resolved, already-defaulted retry policy — the output of
// resolveRetryPolicy. Distinct from config.RetryPolicyConfig (the raw,
// nilable-field config-package shape): this is the one conversion point from
// config's nil-means-unset representation to the domain layer, so
// evaluateSessionRetry depends only on this plain type.
type RetryPolicy struct {
	Enabled      bool
	MaxAttempts  int
	RetryOn      []string
	InitialDelay time.Duration
	MaxDelay     time.Duration
	// InteractiveStallTimeout is the quiet period after which an interactive
	// session is flagged for attention (never restarted). Zero means the driver
	// default (see interactiveStallBudget).
	InteractiveStallTimeout time.Duration
}

// resolveRetryPolicy merges a per-session override onto the global policy,
// field by field: a field set on override wins; an unset override field
// inherits the resolved global value (post-...OrDefault()), never a bare
// per-field zero-value default. This is what stops a per-session override
// that only sets MaxAttempts from silently widening a narrowed global RetryOn
// back to the all-three fallback.
func resolveRetryPolicy(global config.RetryPolicyConfig, override *config.RetryPolicyConfig) RetryPolicy {
	resolved := RetryPolicy{
		Enabled:      global.EnabledOrDefault(),
		MaxAttempts:  global.MaxAttemptsOrDefault(),
		RetryOn:      global.RetryOnOrDefault(),
		InitialDelay: time.Duration(global.InitialDelaySeconds) * time.Second,
		MaxDelay:     time.Duration(global.MaxDelaySecondsOrDefault()) * time.Second,

		InteractiveStallTimeout: global.InteractiveStallTimeoutOrDefault(),
	}
	if override == nil {
		return resolved
	}
	if override.Enabled != nil {
		resolved.Enabled = *override.Enabled
	}
	if override.MaxAttempts > 0 {
		resolved.MaxAttempts = override.MaxAttempts
	}
	if len(override.RetryOn) > 0 {
		// FilteredRetryOn (not RetryOnOrDefault) so an override where every
		// entry is an unknown/typo'd reason falls back to the already-resolved
		// global value above, rather than RetryOnOrDefault's own empty-result
		// fallback silently widening a deliberately narrow override to all
		// three reasons.
		if filtered := override.FilteredRetryOn(); len(filtered) > 0 {
			resolved.RetryOn = filtered
		}
	}
	if override.InitialDelaySeconds > 0 {
		resolved.InitialDelay = time.Duration(override.InitialDelaySeconds) * time.Second
	}
	if override.MaxDelaySeconds > 0 {
		resolved.MaxDelay = time.Duration(override.MaxDelaySeconds) * time.Second
	}
	if override.InteractiveStallMinutes > 0 {
		resolved.InteractiveStallTimeout = override.InteractiveStallTimeoutOrDefault()
	}
	return resolved
}

// defaultJitterFraction is the +/- bound applied to every computed backoff
// delay in production, preventing a shared-cause failure (network blip,
// service restart) from producing a synchronized restart storm across
// multiple sessions failing at the same instant.
const defaultJitterFraction = 0.1

// minAttemptZeroFloor is a small fixed floor applied whenever the computed
// base delay would otherwise be exactly zero — the default policy keeps
// InitialDelaySeconds at 0 for AC7 backward-compat, and 0*2^0 is still 0, so
// without this floor a shared-cause failure hitting several sessions at the
// same poll tick would restart them all in perfect lockstep even with jitter
// enabled (jitter scales the base delay; jitter of zero is still zero).
const minAttemptZeroFloor = 2 * time.Second

// backoffDelay returns min(initial*2^attempt, max) +/- jitterFraction, never
// negative, never exceeding max*(1+jitterFraction). Guards against
// 1<<attempt overflow for large attempt counts by capping directly at max.
func backoffDelay(attempt int, initial, max time.Duration, jitterFraction float64) time.Duration {
	if max <= 0 {
		max = 300 * time.Second
	}
	var base time.Duration
	switch {
	case attempt < 0:
		base = initial
	case attempt >= 63:
		// 1<<attempt would overflow int64 (or land on an ambiguous shift-by-width
		// result) long before this — any policy reaching attempt 63 wants the cap.
		base = max
	default:
		multiplier := int64(1) << uint(attempt)
		if initial > 0 && multiplier > 0 && initial > max/time.Duration(multiplier) {
			// initial*multiplier would exceed (or overflow toward) max — cap
			// directly instead of computing the intermediate product.
			base = max
		} else {
			base = initial * time.Duration(multiplier)
		}
	}
	if base <= 0 {
		base = minAttemptZeroFloor
	}
	if base > max {
		base = max
	}
	if jitterFraction <= 0 {
		return base
	}
	jitterRange := float64(base) * jitterFraction
	jittered := float64(base) + (rand.Float64()*2-1)*jitterRange
	if jittered < 0 {
		jittered = 0
	}
	upperBound := float64(max) * (1 + jitterFraction)
	if jittered > upperBound {
		jittered = upperBound
	}
	return time.Duration(jittered)
}

// classifyFailureReason returns "tmux_exited" when the tmux session itself is
// gone (pane loss, OOM-killed tmux server, laptop sleep) or "crashed" for a
// process-exit path with the tmux session still alive. The inactivity-timeout
// call site passes "stalled" directly rather than calling this — that path is
// never ambiguous about the reason.
func classifyFailureReason(inst *Instance) string {
	if !inst.pm().IsAlive() {
		return "tmux_exited"
	}
	return "crashed"
}

// restartGraceWindow bounds how long after this process's boot a tmux_exited
// failure is treated as restart-grace (a routine `make install-service`
// deploy killing every tmux session at once) rather than a genuine crash that
// consumes an attempt.
const restartGraceWindow = 60 * time.Second

// retryDecision is evaluateSessionRetry's outcome for a single failure
// occurrence.
type retryDecision int

const (
	// retryDecisionScheduled: eligible; the caller should set NextRetryAt and
	// append a RetryAttemptRecord without restarting yet.
	retryDecisionScheduled retryDecision = iota
	// retryDecisionNotEligible: reason is not in policy.RetryOn (or the policy
	// is disabled) — skip straight to PermanentlyFailed, no wait.
	retryDecisionNotEligible
	// retryDecisionExhausted: attempt count already at (or beyond) the cap —
	// transition to PermanentlyFailed.
	retryDecisionExhausted
	// retryDecisionRestartGrace: eligible, reason is tmux_exited, and this
	// process booted within restartGraceWindow — restart immediately without
	// consuming an attempt.
	retryDecisionRestartGrace
)

// evaluateSessionRetry decides whether a failure with the given reason should
// be scheduled for automated retry, given the current RetryState and resolved
// RetryPolicy. Pure and side-effect-free — no I/O, no mutation — mirroring
// backlog_remediation.go's evaluateRemediation shape.
//
// bootTime is passed explicitly (rather than read from the serverStartTime
// package var directly) so tests can simulate "server booted recently"
// without process-level tricks — matches evaluateRemediation's own bootTime
// param.
func evaluateSessionRetry(rs domain.RetryState, policy RetryPolicy, reason string, now, bootTime time.Time) retryDecision {
	if !policy.Enabled {
		return retryDecisionNotEligible
	}
	if !slices.Contains(policy.RetryOn, reason) {
		return retryDecisionNotEligible
	}
	if reason == "tmux_exited" && now.Sub(bootTime) < restartGraceWindow {
		return retryDecisionRestartGrace
	}
	if rs.RetryAttempt >= rs.RetryMaxAttempts {
		return retryDecisionExhausted
	}
	return retryDecisionScheduled
}

// ErrRetryInFlight is returned by restartForRetry (and propagated by
// RetryNow) when another restart is already in progress for this instance —
// the automated backoff-expiry path and a manual "Retry now" racing each
// other, or two manual calls racing themselves.
var ErrRetryInFlight = errors.New("a retry is already in progress for this session")

// retryExhausted reports whether RetryState's attempt count has reached its
// resolved cap. The RLock is fully released on return — not deferred past
// it — so this is safe to call immediately before a handleDriverFailure call
// on the same goroutine (handleDriverFailure takes inst.mu.Lock() internally;
// a deferred RUnlock held past that call would self-deadlock).
func (i *Instance) retryExhausted() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.RetryMaxAttempts > 0 && i.RetryAttempt >= i.RetryMaxAttempts
}

// SetRetryProgress applies update to the retry state under the instance lock and
// republishes the snapshot, so Snapshot().Retry never lags a driver write.
func (i *Instance) SetRetryProgress(update func(rs *domain.RetryState)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	update(&i.RetryState)
	i.snapshot.Store(buildSnapshot(i))
}

// RetrySnapshot returns a race-free copy of RetryState for wire-boundary
// conversion (server/adapters.InstanceToProto) and read-only UI consumers,
// mirroring how GitHubPRState and its siblings are read directly rather than
// through the InstanceSnapshot atomic pointer.
func (i *Instance) RetrySnapshot() domain.RetryState {
	i.mu.RLock()
	defer i.mu.RUnlock()
	rs := i.RetryState
	rs.RetryHistory = append([]domain.RetryAttemptRecord(nil), i.RetryHistory...)
	return rs
}

// RetryPendingElapsed reports whether a retry is currently pending
// (NextRetryAt non-zero) and, if so, whether it has already elapsed as of
// now. Locked separately from any caller-side re-evaluation so the driver
// poll loop's gate can check this without racing a concurrent RetryNow()
// call mutating RetryState from another goroutine.
func (i *Instance) RetryPendingElapsed(now time.Time) (pending, elapsed bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.NextRetryAt.IsZero() {
		return false, false
	}
	return true, !now.Before(i.NextRetryAt)
}

// ClearNextRetryAt clears a pending retry timestamp, called after a
// successful automated restart consumes it.
func (i *Instance) ClearNextRetryAt() {
	i.SetRetryProgress(func(rs *domain.RetryState) { rs.NextRetryAt = time.Time{} })
}

// LastRetryFailureReason returns the most recently recorded failure reason,
// used to build the continuation prompt at the moment a scheduled retry's
// backoff delay elapses.
func (i *Instance) LastRetryFailureReason() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.LastFailureReason
}

// TryBeginRetry claims the single retry-in-flight slot for restartForRetry
// (session_driver.go) and returns the release func; ok is false while another
// restart holds it.
func (i *Instance) TryBeginRetry() (release func(), ok bool) {
	if !i.retryInFlight.CompareAndSwap(false, true) {
		return nil, false
	}
	return func() { i.retryInFlight.Store(false) }, true
}

// SetRetryInFlightForTest sets retryInFlight directly, for tests outside this
// package that need to simulate a concurrent restart already in progress
// (e.g. asserting RetrySession's RPC error mapping) without racing two real
// goroutines.
func (i *Instance) SetRetryInFlightForTest(v bool) {
	i.retryInFlight.Store(v)
}

// MarkPermanentlyFailedForTest forces a live instance into the terminal
// PermanentlyFailed status, for tests outside this package that need to
// simulate a session which has exhausted its automated retries (e.g.
// asserting RetrySession's manual-recovery RPC). Routes through
// markSessionPermanentlyFailed's existing locked + snapshot-republish path
// rather than a bare `inst.Status = PermanentlyFailed` field write — a plain
// field write races with CreateSession's async start pipeline, whose
// transitionToLocked call publishes Status outside inst.mu entirely (see
// GetStatus's doc comment). Callers must still wait for the instance to
// reach Active (via GetStatus/Snapshot, not this field) before calling this,
// so the pipeline's own transition has already landed.
func (i *Instance) MarkPermanentlyFailedForTest() {
	markSessionPermanentlyFailed(i, "test")
}

// IsRetryPending reports whether an automated restart is currently claimed
// (retryInFlight) or scheduled (NextRetryAt set) for this instance. Used by
// BacklogLifecycleListener's stale-work remediation gate (via
// BacklogService.SessionStopper) to defer to the retry policy's own recovery
// instead of racing it with an independent kill-pane-and-respawn action —
// the two mechanisms serialize through this check rather than each acting on
// the same session's process-level state unaware of the other (AC8).
func (i *Instance) IsRetryPending() bool {
	if i.retryInFlight.Load() {
		return true
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	return !i.NextRetryAt.IsZero()
}

// RetryNow resets RetryState and restarts the session immediately, bypassing
// any pending backoff delay — including from PermanentlyFailed. Backs the
// manual "Retry now" UI action and the RetrySession RPC (AC6). Re-resolves
// the retry policy fresh (rather than reusing a snapshot from a prior driver
// run) since this always starts a brand-new failure episode.
//
// Deliberately does NOT set Status = Active here: restartForRetry branches on
// GetEffectiveStatus() to decide between the RecoverFromStopped()+Start()
// cold-recovery path (Stopped/PermanentlyFailed) and the in-place Restart()
// path (everything else). Setting Status = Active before calling
// restartForRetry made GetEffectiveStatus() always read Active, so the
// cold-recovery branch was unreachable from a manual retry — even starting
// from PermanentlyFailed — while the automated backoff-expiry and
// restart-grace callers (which never pre-mutate Status) correctly took it.
// Status is set to Active only as a side effect of whichever branch
// restartForRetry actually takes (Start()'s Creating→Active transition, or
// Restart()'s own handling), mirroring the automated entry points exactly.
func (i *Instance) RetryNow(allowedPath string) error {
	policy := resolveRetryPolicy(config.LoadConfig().RetryPolicy, i.retryPolicyOverride)

	i.SetRetryProgress(func(rs *domain.RetryState) {
		rs.ResetEpisode()
		rs.RetryMaxAttempts = policy.MaxAttempts
	})

	prompt := buildRetryContinuationPrompt(i, "manual retry")
	return driverControl().RestartForRetry(i, allowedPath, prompt, policy, nil)
}

// RetryPolicyOverride returns the per-session retry policy override, nil when
// the session inherits the global default. It is an accessor, not a snapshot
// field, so the snapshot does not carry a config type.
func (i *Instance) RetryPolicyOverride() *config.RetryPolicyConfig {
	return i.retryPolicyOverride
}

// markSessionPermanentlyFailed transitions the session to the terminal
// PermanentlyFailed status (ADR-001), adds the existing ReviewQueue entry
// (unchanged ReasonStale), and fires a proactive notification — edge-
// triggered on the actual transition, so a later observation of an
// already-PermanentlyFailed session never re-fires either. Only RetryNow()
// re-arms the one-shot by transitioning back to Active.
func markSessionPermanentlyFailed(inst *Instance, reason string) {
	inst.mu.Lock()
	alreadyFailed := inst.Status == domain.PermanentlyFailed
	var snap *domain.InstanceSnapshot
	if !alreadyFailed {
		inst.Status = domain.PermanentlyFailed
		// Publish the transition to inst.snapshot (the atomic pointer
		// GetEffectiveStatus()/Snapshot() actually read) — this write, unlike
		// transitionToLocked's, doesn't route through the state machine (there
		// is no PermanentlyFailed entry in transitionIndex), so nothing else
		// republishes it. Without this, GetEffectiveStatus() kept returning
		// the instance's pre-failure status (e.g. Active) until some unrelated
		// mutator happened to rebuild the snapshot, which broke both the UI's
		// status display and restartForRetry's branch decision for a manual
		// RetryNow() on this instance.
		snap = buildSnapshot(inst)
	}
	attempt := inst.RetryAttempt
	maxAttempts := inst.RetryMaxAttempts
	inst.mu.Unlock()

	if alreadyFailed {
		return
	}
	inst.snapshot.Store(snap)

	log.Warn("SessionDriver: session exhausted retry budget; marking permanently failed",
		"session", inst.Title, "reason", reason, "attempts", attempt, "max_attempts", maxAttempts,
	)
	markSessionNeedsAttention(inst, reason)

	if n := inst.GetNotifier(); n != nil {
		n.Notify(inst.UUID,
			"Session gave up after repeated failures",
			fmt.Sprintf("%s failed to recover after %d attempt(s) (last reason: %s) and will not be retried automatically. Use \"Retry now\" to try again.", inst.Title, attempt, reason),
			7,          // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
			true, true, // urgent, important — automatic recovery gave up; a genuine dead end
		)
	}
}

// buildRetryContinuationPrompt builds the continuation prompt for an
// automated or manual retry: the existing JSONL-derived (or
// InitialPrompt-fallback) continuation text, prefixed with "Previous attempt
// failed due to {reason}." (AC3) plus a one-line worktree-integrity hint —
// not a detection mechanism, just surfacing the possibility to whoever reads
// the resumed session (research/pitfalls.md §4).
func buildRetryContinuationPrompt(inst *Instance, reason string) string {
	base := buildContinuationPrompt(inst)
	if base == "Your previous session exited unexpectedly. Please continue from where you left off." &&
		inst.InitialPrompt != "" {
		if sanitized := cleanDriverPromptText(inst.InitialPrompt); sanitized != "" {
			base = sanitized
		}
	}
	if reason == "" {
		return base
	}
	attempt := inst.Snapshot().Retry.RetryAttempt
	prefix := fmt.Sprintf(
		"Previous attempt failed due to %s. (This is retry attempt %d; if this failure looks identical to the last one, the worktree itself may be the problem.) ",
		reason, attempt,
	)
	return prefix + base
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
	rq.Add(&queue.ReviewItem{
		SessionID:    inst.UUID,
		SessionName:  inst.Title,
		Reason:       queue.ReasonStale, // closest existing reason: "no output for extended period"
		Priority:     queue.PriorityMedium,
		DetectedAt:   time.Now(),
		Context:      reason, // "inactivity timeout" or "unexpected exit" or "restart error: ..."
		Tags:         inst.GetTags(),
		Path:         inst.GetPath(),
		Status:       inst.GetEffectiveStatus().String(),
		LastActivity: inst.LastMeaningfulOutputTime(),
	})
}

// cleanDriverPromptText drops NUL bytes (tmux drops them unpredictably) and
// surrounding whitespace. Empty means "fall back to driverInitialPrompt".
func cleanDriverPromptText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
}

// buildContinuationPrompt reads the last N messages from the session's JSONL
// conversation log and produces a brief prompt summarizing the last assistant
// turn. Falls back to a generic prompt if the log is unavailable.
func buildContinuationPrompt(inst *Instance) string {
	histPath := inst.Snapshot().HistoryFilePath
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

// Continuation prompt constants.
const driverContinuationMaxMessages = 10

const driverContinuationMaxChars = 500
