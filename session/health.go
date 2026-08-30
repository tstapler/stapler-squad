package session

import (
	"errors"
	"fmt"
	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
	"os"
	"time"
)

// SessionHealthChecker manages session health validation and recovery
type SessionHealthChecker struct {
	storage    *Storage
	tmuxSocket TmuxSocketQuerier // Tmux server-socket queries for CheckAllSessions; fakeable in tests

	// failureCounts tracks consecutive health-check failures per session title.
	// Recovery is only attempted after failureThreshold consecutive failures,
	// preventing false-positive recoveries from transient check glitches.
	failureCounts   map[string]int
	failureCountsMu deadlock.Mutex

	// startedAt is when this checker was constructed. Dead-pane detections within
	// restartGracePeriod of startedAt fall back to the old silent kill+respawn
	// behavior instead of surfacing a Crashed status, to avoid false-positive
	// crash banners caused by a service restart (see restartGracePeriod's doc
	// comment). Directly overridable in tests (e.g. a zero time.Time is always
	// outside the grace period).
	startedAt time.Time
}

// failureThreshold is the number of consecutive failed health checks before
// a recovery attempt is triggered. Set to 2 to require two consecutive misses.
const failureThreshold = 2

// restartGracePeriod suppresses Crashed/Stopped-status transitions for this long
// after the health checker starts, but ONLY for sessions that already existed
// before the checker started (see the sessionPredatesRestart check at its use
// site below). A service restart can leave an orphaned pre-restart process
// racing the new process over the same tmux server (see
// docs/explanation/service-restart-orphan-process.md and
// docs/explanation/tmux-keep-server-on-restart.md), which can surface as a spurious
// dead-pane detection right after startup for one of those pre-existing
// sessions. During the grace period, such dead panes are still self-healed via
// the old kill+respawn behavior; only after it elapses does a dead pane surface
// as a user-visible Crashed status.
//
// A session created after the checker started has no such ambiguity to protect
// against -- it was spawned entirely within this process's lifetime, so there is
// no orphaned pre-restart process that could be racing it. A dead pane on a
// never-previously-alive session is therefore always a genuine, immediately
// reportable exit, regardless of how soon after startup it happens.
const restartGracePeriod = 60 * time.Second

// markStartFailed fails an instance with a human-readable reason instead of
// leaving it stuck retrying against a directory that will never come back.
// Reuses the CreationProgress + Stopped pattern CreateSession already uses for
// startup failures (see server/services/session_service.go).
func markStartFailed(instance *Instance, err error) {
	instance.SetCreationProgress(fmt.Sprintf("Session failed: %s", err.Error()))
	instance.ForceStatus(Stopped)
	log.ForSession(instance.Title).Error("session failed: working directory missing, giving up", "err", err)
}

// NewSessionHealthChecker creates a new session health checker
func NewSessionHealthChecker(storage *Storage) *SessionHealthChecker {
	return &SessionHealthChecker{
		storage:       storage,
		tmuxSocket:    realTmuxSocketQuerier{},
		failureCounts: make(map[string]int),
		startedAt:     time.Now(),
	}
}

// HealthCheckResult represents the result of a session health check
type HealthCheckResult struct {
	InstanceTitle     string
	IsHealthy         bool
	Issues            []string
	Actions           []string
	RecoveryAttempted bool
	RecoverySuccess   bool
}

// CheckAllSessions performs a health check on all active sessions
func (h *SessionHealthChecker) CheckAllSessions() ([]HealthCheckResult, error) {
	instances, err := h.storage.LoadInstances()
	if err != nil {
		return nil, fmt.Errorf("failed to load instances for health check: %w", err)
	}
	return h.checkInstances(instances), nil
}

// checkInstances runs a health check on the given instances, split out from
// CheckAllSessions so the socket-scoping logic below is unit-testable with a plain
// []*Instance and a fake TmuxSocketQuerier, without real Storage/DB wiring.
//
// Instances can be spread across multiple tmux server sockets (the default socket
// for ordinary sessions, isolated sockets for some worktree/test scenarios).
// Deriving a single socket from the first instance and applying its down/up state
// to every instance would either skip healthy instances on other sockets (false
// "server is down") or run destructive recovery against instances whose own socket
// is genuinely down (false "server is up"). So each instance's down-check is scoped
// to its own socket, memoized per socket to avoid redundant queries.
func (h *SessionHealthChecker) checkInstances(instances []*Instance) []HealthCheckResult {
	downSockets := make(map[string]bool)

	// One tmux.BatchPaneDeadStatus call per distinct socket per tick, instead of
	// one `display-message` subprocess per session per tick -- this is the fix
	// for the reported ~15k display-message calls/20min: that volume came from
	// the per-instance PaneProcessDead()/PaneExitInfo() calls below being
	// evaluated unconditionally for every active session on every tick,
	// independent of whether the session has a viewer attached (control mode is
	// viewer-gated and can't help background sessions -- see
	// session/tmux/control_mode.go and streamViaHub's StartControlMode/
	// StopControlMode refcounting).
	paneStatuses := make(map[string]map[string]tmux.PaneDeadStatus)

	results := make([]HealthCheckResult, 0, len(instances))

	for _, instance := range instances {
		socket := instance.TmuxServerSocket
		down, checked := downSockets[socket]
		if !checked {
			down = h.tmuxSocket.IsServerDown(socket)
			downSockets[socket] = down
		}
		// Guard: if this instance's tmux server is completely down, its session
		// will look dead. Attempting to recover it would be destructive and
		// incorrect. Skip until that socket's server is back up.
		if down {
			log.Warn("health check: tmux server is down, skipping session check", "session", instance.Title, "socket", socket)
			continue
		}

		if _, fetched := paneStatuses[socket]; !fetched {
			statuses, err := tmux.BatchPaneDeadStatus(socket)
			if err != nil {
				// Leave this socket's entry as nil -- checkSingleSession falls back
				// to the per-instance PaneProcessDead()/PaneExitInfo() calls when a
				// session isn't found in paneStatuses.
				log.Debug("health check: BatchPaneDeadStatus failed, falling back to per-instance checks", "socket", socket, "err", err)
			}
			paneStatuses[socket] = statuses
		}

		result := h.checkSingleSession(instance, paneStatuses[socket])
		results = append(results, result)

		// Log any issues found
		if !result.IsHealthy {
			log.Warn("health check found issues for session", "session", result.InstanceTitle, "issues", result.Issues)
			if result.RecoveryAttempted {
				if result.RecoverySuccess {
					log.Debug("successfully recovered session", "session", result.InstanceTitle)
				} else {
					log.Error("failed to recover session", "session", result.InstanceTitle)
				}
			}
		}
	}

	return results
}

// recoveryDebounced increments the consecutive-failure counter for title and
// reports whether failureThreshold has been reached (in which case the
// counter is reset to 0 so the next failure starts a fresh debounce window).
// Shared by every failure mode checkSingleSession detects (missing tmux
// session, dead pane process, ...) so that N consecutive failures of any kind
// -- not just the same kind -- trigger a recovery attempt.
func (h *SessionHealthChecker) recoveryDebounced(title string) (attempt bool, count int) {
	h.failureCountsMu.Lock()
	defer h.failureCountsMu.Unlock()
	h.failureCounts[title]++
	count = h.failureCounts[title]
	if count >= failureThreshold {
		h.failureCounts[title] = 0
		return true, count
	}
	return false, count
}

// resetFailureCount clears the consecutive-failure counter for title, called
// whenever a session is observed healthy.
func (h *SessionHealthChecker) resetFailureCount(title string) {
	h.failureCountsMu.Lock()
	delete(h.failureCounts, title)
	h.failureCountsMu.Unlock()
}

// paneDeadStatus reports whether instance's pane has exited and its exit
// code/signal, preferring a pre-fetched batch result (see checkInstances'
// tmux.BatchPaneDeadStatus call) over the equivalent per-instance
// PaneProcessDead()/PaneExitInfo() calls, which each cost one `display-message`
// tmux subprocess. Falls back to those per-instance calls when batch is nil
// (the batch fetch failed for this socket) or doesn't contain this session's
// name (e.g. GetTmuxSessionName() is empty for an external/uninitialized
// session).
func paneDeadStatus(instance *Instance, batch map[string]tmux.PaneDeadStatus) (dead bool, code int, signal string) {
	if name := instance.GetTmuxSessionName(); name != "" {
		if status, ok := batch[name]; ok {
			return status.Dead, status.Code, status.Signal
		}
	}
	dead, code, signal = instance.PaneExitInfo()
	return dead, code, signal
}

// healthCheckSkipReason reports whether instance is in a state the health
// checker must leave alone, and the human-readable action to record for it.
//
// Paused and hibernated sessions are expected to have no tmux session at all.
// Stopped is a terminal state that must not be silently resurrected by the
// poller: Started() is not cleared on a normal Active->Stopped transition (e.g.
// via instanceOnExitCallback or checkTmuxHealth's own normal-completion path),
// and TmuxAlive() already treats Stopped as "not alive" -- so without this skip,
// every naturally-completed session would hit recoverMissingSession and get
// auto-restarted on the very next tick. Crashed sessions require an explicit
// resume (see Instance.ResumeFromCrash / the ResumeCrashedSession RPC) and must
// not be silently respawned either.
func healthCheckSkipReason(instance *Instance) (string, bool) {
	if instance.Paused() {
		return "Skipped (session is paused)", true
	}
	if instance.Hibernated() {
		return "Skipped (session is hibernated)", true
	}
	switch instance.Snapshot().Status {
	case Stopped:
		return "Skipped (session is stopped)", true
	case Crashed:
		return "Skipped (session has crashed, awaiting resume)", true
	}
	return "", false
}

// checkSingleSession performs a health check on a single session. paneStatus
// is the batch-fetched pane-dead status for instance's tmux socket (nil if the
// batch fetch failed), keyed by session name -- see paneDeadStatus.
func (h *SessionHealthChecker) checkSingleSession(instance *Instance, paneStatus map[string]tmux.PaneDeadStatus) HealthCheckResult {
	result := HealthCheckResult{
		InstanceTitle: instance.Title,
		IsHealthy:     true,
		Issues:        []string{},
		Actions:       []string{},
	}

	if reason, skip := healthCheckSkipReason(instance); skip {
		result.Actions = append(result.Actions, reason)
		return result
	}

	// LoadInstances() (session/storage.go) always deserializes with
	// deferStart=true so a bulk load at server startup doesn't block HTTP bind
	// on cold-restoring every session -- but that leaves Started() false on
	// every freshly-deserialized Active instance, even though its tmux backend
	// is already wired (fromInstanceData's SetSession call runs regardless of
	// deferStart). Every CheckAllSessions() tick goes through LoadInstances(),
	// so without this, checkTmuxHealth -- and therefore all dead-pane
	// detection -- was skipped entirely for every Active session, on every
	// tick, forever: TmuxAlive()/PaneProcessDead() both short-circuit false on
	// !Started(), matching the exact remain-on-exit dead pane this checker
	// exists to catch. instance here is a throwaway copy LoadInstances()
	// constructs fresh on every call (not the live, actor-managed instance),
	// so marking it started has no effect beyond this health check.
	if instance.Status == Active && !instance.Started() {
		instance.started.Store(true)
	}

	// Check if instance thinks it's started but tmux session doesn't exist
	if instance.Started() {
		h.checkTmuxHealth(instance, paneStatus, &result)
	}

	checkWorktreeHealth(instance, &result)

	return result
}

// checkTmuxHealth classifies instance's tmux backend as missing, dead-paned, or
// healthy, and drives the matching recovery. Only called for started sessions.
func (h *SessionHealthChecker) checkTmuxHealth(instance *Instance, paneStatus map[string]tmux.PaneDeadStatus, result *HealthCheckResult) {
	if !instance.TmuxAlive() {
		h.recoverMissingSession(instance, result)
		return
	}

	// paneDeadStatus is only evaluated here, not before the TmuxAlive() check
	// above -- a not-alive session missing from the batch map would otherwise
	// trigger a second, redundant TmuxAlive()/IsAlive() subprocess call on top
	// of the check above.
	paneDead, paneExitCode, paneExitSignal := paneDeadStatus(instance, paneStatus)
	if !paneDead {
		// Session is healthy - reset any accumulated failure count
		h.resetFailureCount(instance.Title)
		result.Actions = append(result.Actions, "Tmux session is healthy")
		return
	}
	h.handleDeadPane(instance, paneExitCode, paneExitSignal, result)
}

// recordStartFailure records a failed recovery Start() on result, and gives up
// permanently when the session's working directory is gone (e.g. a pruned
// worktree) rather than retrying every health-check cycle forever.
func recordStartFailure(instance *Instance, err error, action string, result *HealthCheckResult) {
	result.Issues = append(result.Issues, fmt.Sprintf("Recovery failed: %v", err))
	result.RecoverySuccess = false
	result.Actions = append(result.Actions, action)
	if errors.Is(err, tmux.ErrWorkDirMissing) {
		markStartFailed(instance, err)
		result.Actions = append(result.Actions, "Session failed: working directory missing")
	}
}

// recoverMissingSession handles a session marked started whose tmux session no
// longer exists, by recreating it once the failure debounce has been satisfied.
func (h *SessionHealthChecker) recoverMissingSession(instance *Instance, result *HealthCheckResult) {
	result.IsHealthy = false
	result.Issues = append(result.Issues, "Instance marked as started but tmux session doesn't exist")

	// Debounce: only recover after failureThreshold consecutive failures.
	// This prevents spurious recovery attempts caused by transient check glitches.
	attempt, count := h.recoveryDebounced(instance.Title)
	if !attempt {
		log.Debug("health check: deferring recovery", "session", instance.Title, "count", count, "threshold", failureThreshold)
		result.Actions = append(result.Actions, fmt.Sprintf("Failure %d/%d: deferring recovery", count, failureThreshold))
		return
	}

	result.RecoveryAttempted = true
	if err := instance.Start(false); err != nil {
		recordStartFailure(instance, err, "Failed to recreate tmux session", result)
		return
	}

	result.RecoverySuccess = true
	result.Actions = append(result.Actions, "Successfully recreated tmux session")
	instance.SetCreationProgress("") // clear any stale "Session failed: ..." message
	// Re-check health after recovery
	if instance.TmuxAlive() {
		result.IsHealthy = true
	} else {
		result.Issues = append(result.Issues, "Session still unhealthy after recovery attempt")
	}
}

// handleDeadPane handles a live tmux session whose pane process has exited:
// remain-on-exit has left a dead "Pane is dead (signal N, ...)" placeholder
// because the wrapped program exited (crashed or completed normally).
// TmuxAlive() alone cannot see this -- it only checks session existence -- so
// without this the session is reported healthy forever. See
// session/instance_tmux.go PaneProcessDead() doc.
func (h *SessionHealthChecker) handleDeadPane(instance *Instance, exitCode int, exitSignal string, result *HealthCheckResult) {
	result.IsHealthy = false
	result.Issues = append(result.Issues, "tmux session alive but pane process has exited (remain-on-exit placeholder)")

	attempt, count := h.recoveryDebounced(instance.Title)
	if !attempt {
		log.Debug("health check: deferring dead-pane recovery", "session", instance.Title, "count", count, "threshold", failureThreshold)
		result.Actions = append(result.Actions, fmt.Sprintf("Failure %d/%d: deferring recovery", count, failureThreshold))
		return
	}

	result.RecoveryAttempted = true

	// Only a session that already existed before this checker started could
	// have been affected by a restart race; one created afterward was never
	// "previously alive" from this process's perspective and gets no grace
	// window. See restartGracePeriod's doc comment.
	sessionPredatesRestart := instance.CreatedAt.Before(h.startedAt)
	if sessionPredatesRestart && time.Since(h.startedAt) < restartGracePeriod {
		respawnWithinGraceWindow(instance, result)
		return
	}

	if exitCode == 0 && exitSignal == "" {
		// Normal completion (e.g. the wrapped program exited cleanly),
		// not a crash -- must not be mislabeled as Crashed. Transition to
		// Stopped, matching the status a control-mode-detected exit
		// already produces (instanceOnExitCallback).
		if err := instance.MarkExitedNormally(); err != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("failed to mark session stopped: %v", err))
		} else {
			// RecoverySuccess is checked by checkInstances' caller to decide
			// between logging "successfully recovered" vs. "failed to
			// recover" -- without this, a normal Stopped transition (which
			// is the successful outcome here, not a failure) was logged as
			// an ERROR-level "failed to recover session" on every occurrence.
			result.RecoverySuccess = true
			result.Actions = append(result.Actions, "Pane exited normally (exit code 0); session marked Stopped")
		}
		return
	}

	exitReason := fmt.Sprintf("exit code %d", exitCode)
	if exitSignal != "" {
		exitReason = fmt.Sprintf("signal %s (exit code %d)", exitSignal, exitCode)
	}
	if err := instance.MarkCrashed(exitReason); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("failed to mark session crashed: %v", err))
	} else {
		// See RecoverySuccess comment above -- a successful Crashed transition
		// is likewise not a recovery failure.
		result.RecoverySuccess = true
		result.Actions = append(result.Actions, fmt.Sprintf("Pane crashed (%s); session marked Crashed, awaiting resume", exitReason))
	}
}

// respawnWithinGraceWindow handles a dead pane detected inside the post-startup
// grace window: fall back to the old silent kill+respawn behavior instead of
// surfacing a status change, since a dead-pane detection here is more likely a
// startup-race artifact than a genuine crash (see restartGracePeriod's doc
// comment). The stale session must be torn down first: Start(false) treats an
// existing (even dead-paned) tmux session as "already running" and just
// reattaches via RestoreWithWorkDir, which does NOT relaunch the wrapped
// program. Killing it first forces Start(false) down the cold-restore path that
// actually relaunches it (with --resume when a conversation UUID is known).
func respawnWithinGraceWindow(instance *Instance, result *HealthCheckResult) {
	if err := instance.KillSession(); err != nil {
		result.Issues = append(result.Issues, fmt.Sprintf("failed to kill stale dead-pane session: %v", err))
	}
	if err := instance.Start(false); err != nil {
		recordStartFailure(instance, err, "Failed to respawn dead pane", result)
		return
	}

	result.RecoverySuccess = true
	result.Actions = append(result.Actions, "Respawned dead pane by recreating tmux session (startup grace window)")
	instance.SetCreationProgress("") // clear any stale "Session failed: ..." message
	if instance.TmuxAlive() && !instance.PaneProcessDead() {
		result.IsHealthy = true
	} else {
		result.Issues = append(result.Issues, "Session still unhealthy after recovery attempt")
	}
}

// checkWorktreeHealth flags a session whose git worktree directory has been
// removed out from under it.
func checkWorktreeHealth(instance *Instance, result *HealthCheckResult) {
	if instance.Paused() || !instance.gitManager.HasWorktree() {
		return
	}
	worktreePath := instance.gitManager.GetWorktreePath()
	if worktreePath == "" {
		return
	}
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		result.IsHealthy = false
		result.Issues = append(result.Issues, fmt.Sprintf("Worktree path doesn't exist: %s", worktreePath))
		result.Actions = append(result.Actions, "Consider pausing this session or recreating worktree")
	} else {
		result.Actions = append(result.Actions, "Worktree path exists")
	}
}

// RecoverUnhealthySessions attempts to recover all unhealthy sessions
func (h *SessionHealthChecker) RecoverUnhealthySessions() error {
	results, err := h.CheckAllSessions()
	if err != nil {
		return fmt.Errorf("failed to check sessions for recovery: %w", err)
	}

	recoveredCount := 0
	failedCount := 0

	for _, result := range results {
		if !result.IsHealthy && result.RecoveryAttempted {
			if result.RecoverySuccess {
				recoveredCount++
			} else {
				failedCount++
			}
		}
	}

	log.Debug("session recovery completed", "recovered", recoveredCount, "failed", failedCount)

	// Save the updated state if any recoveries were attempted
	if recoveredCount > 0 || failedCount > 0 {
		instances, err := h.storage.LoadInstances()
		if err != nil {
			return fmt.Errorf("failed to reload instances after recovery: %w", err)
		}

		if err := h.storage.SaveInstances(instances); err != nil {
			log.Warn("failed to save instances after recovery", "err", err)
		}
	}

	return nil
}

// ScheduledHealthCheck runs health checks at regular intervals
func (h *SessionHealthChecker) ScheduledHealthCheck(interval time.Duration, stopChan <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Debug("starting scheduled health checks", "interval", interval)

	for {
		select {
		case <-ticker.C:
			if err := h.RecoverUnhealthySessions(); err != nil {
				log.Error("scheduled health check failed", "err", err)
			}
		case <-stopChan:
			return
		}
	}
}
