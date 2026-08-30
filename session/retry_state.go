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
	"math/rand"
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// RetryAttemptRecord is one entry in RetryState.RetryHistory.
type RetryAttemptRecord struct {
	Attempt   int       `json:"attempt"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// maxRetryHistoryEntries caps RetryState.RetryHistory so a long-lived,
// repeatedly crash-looping session doesn't grow this slice unboundedly.
const maxRetryHistoryEntries = 10

// RetryState holds the automated crash/stall retry lifecycle for one session:
// attempt count, resolved max, last failure reason, pending-retry timestamp,
// and history. Embedded (by value) into Instance, mirroring ReviewState.
// Protected by inst.mu.
type RetryState struct {
	// RetryAttempt is the number of automated retry attempts consumed so far in
	// the current failure episode. Reset to 0 by RetryNow().
	RetryAttempt int `json:"retry_attempt,omitempty"`
	// RetryMaxAttempts is the policy-resolved cap, snapshotted at driver start
	// (or by RetryNow) so a live config edit mid-cycle doesn't change the cap a
	// session is already partway through.
	RetryMaxAttempts int `json:"retry_max_attempts,omitempty"`
	// LastFailureReason is the most recent classifyFailureReason result:
	// "crashed", "stalled", or "tmux_exited".
	LastFailureReason string `json:"last_failure_reason,omitempty"`
	// NextRetryAt is when the next backed-off restart is due. Zero means no
	// retry is pending.
	NextRetryAt time.Time `json:"next_retry_at,omitempty"`
	// RetryHistory records reason + timestamp per attempt, newest-last, capped
	// at maxRetryHistoryEntries. Preserved across RetryNow() episodes (AC5 wants
	// history to survive a manual retry, not just the automated ones).
	RetryHistory []RetryAttemptRecord `json:"retry_history,omitempty"`
}

// recordAttempt appends a RetryAttemptRecord to RetryHistory, dropping the
// oldest entry once maxRetryHistoryEntries is exceeded. Caller must hold
// inst.mu.
func (rs *RetryState) recordAttempt(attempt int, reason string, now time.Time) {
	rs.RetryHistory = append(rs.RetryHistory, RetryAttemptRecord{Attempt: attempt, Reason: reason, Timestamp: now})
	if len(rs.RetryHistory) > maxRetryHistoryEntries {
		rs.RetryHistory = rs.RetryHistory[len(rs.RetryHistory)-maxRetryHistoryEntries:]
	}
}

// reset clears the attempt/backoff bookkeeping back to a fresh episode, used
// by RetryNow(). RetryHistory is deliberately preserved. Caller must hold
// inst.mu.
func (rs *RetryState) reset() {
	rs.RetryAttempt = 0
	rs.LastFailureReason = ""
	rs.NextRetryAt = time.Time{}
}

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
		resolved.RetryOn = override.RetryOnOrDefault()
	}
	if override.InitialDelaySeconds > 0 {
		resolved.InitialDelay = time.Duration(override.InitialDelaySeconds) * time.Second
	}
	if override.MaxDelaySeconds > 0 {
		resolved.MaxDelay = time.Duration(override.MaxDelaySeconds) * time.Second
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
func evaluateSessionRetry(rs RetryState, policy RetryPolicy, reason string, now, bootTime time.Time) retryDecision {
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

// RetrySnapshot returns a race-free copy of RetryState for wire-boundary
// conversion (server/adapters.InstanceToProto) and read-only UI consumers,
// mirroring how GitHubPRState and its siblings are read directly rather than
// through the InstanceSnapshot atomic pointer.
func (i *Instance) RetrySnapshot() RetryState {
	i.mu.RLock()
	defer i.mu.RUnlock()
	rs := i.RetryState
	rs.RetryHistory = append([]RetryAttemptRecord(nil), i.RetryHistory...)
	return rs
}

// retryPendingElapsed reports whether a retry is currently pending
// (NextRetryAt non-zero) and, if so, whether it has already elapsed as of
// now. Locked separately from any caller-side re-evaluation so the driver
// poll loop's gate can check this without racing a concurrent RetryNow()
// call mutating RetryState from another goroutine.
func (i *Instance) retryPendingElapsed(now time.Time) (pending, elapsed bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.NextRetryAt.IsZero() {
		return false, false
	}
	return true, !now.Before(i.NextRetryAt)
}

// clearNextRetryAt clears a pending retry timestamp, called after a
// successful automated restart consumes it.
func (i *Instance) clearNextRetryAt() {
	i.mu.Lock()
	i.NextRetryAt = time.Time{}
	i.mu.Unlock()
}

// lastRetryFailureReason returns the most recently recorded failure reason,
// used to build the continuation prompt at the moment a scheduled retry's
// backoff delay elapses.
func (i *Instance) lastRetryFailureReason() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.LastFailureReason
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
func restartForRetry(inst *Instance, allowedPath, continuationPrompt string, policy RetryPolicy, stop <-chan struct{}) error {
	if !inst.retryInFlight.CompareAndSwap(false, true) {
		return ErrRetryInFlight
	}
	defer inst.retryInFlight.Store(false)

	st := inst.GetEffectiveStatus()
	var restartErr error
	if st == Stopped || st == PermanentlyFailed {
		inst.RecoverFromStopped()
		// Clear the old (possibly dead) controller so StartController below creates a fresh one.
		inst.StopController()
		restartErr = inst.Start(false)
		if restartErr == nil {
			if ctrlErr := inst.StartController(); ctrlErr != nil {
				log.Warn("SessionDriver: failed to restart controller after retry restart",
					"session", inst.Title, "err", ctrlErr)
			}
		}
	} else {
		restartErr = inst.Restart(false)
	}

	if restartErr != nil {
		log.Error("SessionDriver: retry restart failed; marking for attention",
			"session", inst.Title, "err", restartErr,
		)
		markSessionNeedsAttention(inst, "restart error: "+restartErr.Error())
		return restartErr
	}

	// Set driverRunning = true before spawning to close the race window
	// between an exiting goroutine's own driverRunning bookkeeping and the new
	// one starting (mirrors handleDriverFailure's existing D3 mitigation).
	inst.driverRunning.Store(true)

	driverStop := stop
	if driverStop == nil {
		if existing := inst.driverStopper.Load(); existing != nil {
			driverStop = existing.stop
		} else {
			stopper := &sessionDriverStopper{stop: make(chan struct{})}
			inst.driverStopper.Store(stopper)
			driverStop = stopper.stop
		}
	}

	inst.driverWG.Add(1)
	go func() {
		defer inst.driverWG.Done()
		runSessionDriverWithPrompt(inst, allowedPath, continuationPrompt, policy, driverStop)
	}()
	return nil
}

// SetRetryInFlightForTest sets retryInFlight directly, for tests outside this
// package that need to simulate a concurrent restart already in progress
// (e.g. asserting RetrySession's RPC error mapping) without racing two real
// goroutines.
func (i *Instance) SetRetryInFlightForTest(v bool) {
	i.retryInFlight.Store(v)
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
func (i *Instance) RetryNow(allowedPath string) error {
	policy := resolveRetryPolicy(config.LoadConfig().RetryPolicy, i.RetryPolicyOverride)

	i.mu.Lock()
	i.reset()
	i.RetryMaxAttempts = policy.MaxAttempts
	if i.Status == PermanentlyFailed || i.Status == Stopped {
		i.Status = Active
	}
	i.mu.Unlock()

	prompt := buildRetryContinuationPrompt(i, "manual retry")
	return restartForRetry(i, allowedPath, prompt, policy, nil)
}
