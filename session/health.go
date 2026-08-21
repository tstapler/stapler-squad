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
// .claude/rules/service-restart-orphan-process.md and
// .claude/rules/tmux-keep-server-on-restart.md), which can surface as a spurious
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

		result := h.checkSingleSession(instance)
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

// checkSingleSession performs a health check on a single session
func (h *SessionHealthChecker) checkSingleSession(instance *Instance) HealthCheckResult {
	result := HealthCheckResult{
		InstanceTitle: instance.Title,
		IsHealthy:     true,
		Issues:        []string{},
		Actions:       []string{},
	}

	// Skip paused instances - they're expected to not have active tmux sessions
	if instance.Paused() {
		result.Actions = append(result.Actions, "Skipped (session is paused)")
		return result
	}

	// Skip hibernated instances - they have no tmux by design
	if instance.Hibernated() {
		result.Actions = append(result.Actions, "Skipped (session is hibernated)")
		return result
	}

	// Skip stopped instances. Stopped is a terminal state that must not be
	// silently resurrected by the poller: Started() is not cleared on a normal
	// Active->Stopped transition (e.g. via instanceOnExitCallback or this
	// checker's own normal-completion path below), and TmuxAlive() already
	// treats Stopped as "not alive" -- so without this skip, every naturally-
	// completed session would hit the "!TmuxAlive()" branch below and get
	// auto-restarted on the very next tick.
	//
	// Skip crashed instances too - they require an explicit resume (see
	// Instance.ResumeFromCrash / the ResumeCrashedSession RPC) and must not be
	// silently respawned by the health checker.
	switch instance.Snapshot().Status {
	case Stopped:
		result.Actions = append(result.Actions, "Skipped (session is stopped)")
		return result
	case Crashed:
		result.Actions = append(result.Actions, "Skipped (session has crashed, awaiting resume)")
		return result
	}

	// LoadInstances() (session/storage.go) always deserializes with
	// deferStart=true so a bulk load at server startup doesn't block HTTP bind
	// on cold-restoring every session -- but that leaves Started() false on
	// every freshly-deserialized Active instance, even though its tmux backend
	// is already wired (fromInstanceData's SetSession call runs regardless of
	// deferStart). Every CheckAllSessions() tick goes through LoadInstances(),
	// so without this, the switch below -- and therefore all dead-pane
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
		switch {
		case !instance.TmuxAlive():
			result.IsHealthy = false
			result.Issues = append(result.Issues, "Instance marked as started but tmux session doesn't exist")

			// Debounce: only recover after failureThreshold consecutive failures.
			// This prevents spurious recovery attempts caused by transient check glitches.
			attempt, count := h.recoveryDebounced(instance.Title)
			if !attempt {
				log.Debug("health check: deferring recovery", "session", instance.Title, "count", count, "threshold", failureThreshold)
				result.Actions = append(result.Actions, fmt.Sprintf("Failure %d/%d: deferring recovery", count, failureThreshold))
			} else {
				result.RecoveryAttempted = true
				if err := instance.Start(false); err != nil {
					result.Issues = append(result.Issues, fmt.Sprintf("Recovery failed: %v", err))
					result.RecoverySuccess = false
					result.Actions = append(result.Actions, "Failed to recreate tmux session")
					if errors.Is(err, tmux.ErrWorkDirMissing) {
						// Its working directory is gone (e.g. a pruned worktree) and won't
						// come back on its own — stop retrying every health-check cycle and
						// fail the session with a status the user can actually see.
						markStartFailed(instance, err)
						result.Actions = append(result.Actions, "Session failed: working directory missing")
					}
				} else {
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
			}

		case instance.PaneProcessDead():
			// tmux session object is alive, but remain-on-exit has left a dead
			// "Pane is dead (signal N, ...)" placeholder because the wrapped
			// program exited (crashed or completed normally). TmuxAlive() alone
			// cannot see this -- it only checks session existence -- so without
			// this branch the session is reported healthy forever. See
			// session/instance_tmux.go PaneProcessDead() doc.
			result.IsHealthy = false
			result.Issues = append(result.Issues, "tmux session alive but pane process has exited (remain-on-exit placeholder)")

			attempt, count := h.recoveryDebounced(instance.Title)
			if !attempt {
				log.Debug("health check: deferring dead-pane recovery", "session", instance.Title, "count", count, "threshold", failureThreshold)
				result.Actions = append(result.Actions, fmt.Sprintf("Failure %d/%d: deferring recovery", count, failureThreshold))
				break
			}

			result.RecoveryAttempted = true
			_, exitCode, exitSignal := instance.PaneExitInfo()

			// Only a session that already existed before this checker started could
			// have been affected by a restart race; one created afterward was never
			// "previously alive" from this process's perspective and gets no grace
			// window. See restartGracePeriod's doc comment.
			sessionPredatesRestart := instance.CreatedAt.Before(h.startedAt)

			if sessionPredatesRestart && time.Since(h.startedAt) < restartGracePeriod {
				// Within the post-startup grace window: fall back to the old
				// silent kill+respawn behavior instead of surfacing a status
				// change, since a dead-pane detection here is more likely a
				// startup-race artifact than a genuine crash (see
				// restartGracePeriod's doc comment). The stale session must be
				// torn down first: Start(false) treats an existing (even
				// dead-paned) tmux session as "already running" and just
				// reattaches via RestoreWithWorkDir, which does NOT relaunch the
				// wrapped program. Killing it first forces Start(false) down the
				// cold-restore path that actually relaunches it (with --resume
				// when a conversation UUID is known).
				if err := instance.KillSession(); err != nil {
					result.Issues = append(result.Issues, fmt.Sprintf("failed to kill stale dead-pane session: %v", err))
				}
				if err := instance.Start(false); err != nil {
					result.Issues = append(result.Issues, fmt.Sprintf("Recovery failed: %v", err))
					result.RecoverySuccess = false
					result.Actions = append(result.Actions, "Failed to respawn dead pane")
					if errors.Is(err, tmux.ErrWorkDirMissing) {
						markStartFailed(instance, err)
						result.Actions = append(result.Actions, "Session failed: working directory missing")
					}
				} else {
					result.RecoverySuccess = true
					result.Actions = append(result.Actions, "Respawned dead pane by recreating tmux session (startup grace window)")
					instance.SetCreationProgress("") // clear any stale "Session failed: ..." message
					if instance.TmuxAlive() && !instance.PaneProcessDead() {
						result.IsHealthy = true
					} else {
						result.Issues = append(result.Issues, "Session still unhealthy after recovery attempt")
					}
				}
				break
			}

			if exitCode == 0 && exitSignal == "" {
				// Normal completion (e.g. the wrapped program exited cleanly),
				// not a crash -- must not be mislabeled as Crashed. Transition to
				// Stopped, matching the status a control-mode-detected exit
				// already produces (instanceOnExitCallback).
				if err := instance.MarkExitedNormally(); err != nil {
					result.Issues = append(result.Issues, fmt.Sprintf("failed to mark session stopped: %v", err))
				} else {
					result.Actions = append(result.Actions, "Pane exited normally (exit code 0); session marked Stopped")
				}
				break
			}

			exitReason := fmt.Sprintf("exit code %d", exitCode)
			if exitSignal != "" {
				exitReason = fmt.Sprintf("signal %s (exit code %d)", exitSignal, exitCode)
			}
			if err := instance.MarkCrashed(exitReason); err != nil {
				result.Issues = append(result.Issues, fmt.Sprintf("failed to mark session crashed: %v", err))
			} else {
				result.Actions = append(result.Actions, fmt.Sprintf("Pane crashed (%s); session marked Crashed, awaiting resume", exitReason))
			}

		default:
			// Session is healthy - reset any accumulated failure count
			h.resetFailureCount(instance.Title)
			result.Actions = append(result.Actions, "Tmux session is healthy")
		}
	}

	// Check worktree existence for non-paused instances
	if !instance.Paused() && instance.gitManager.HasWorktree() {
		worktreePath := instance.gitManager.GetWorktreePath()
		if worktreePath != "" {
			if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
				result.IsHealthy = false
				result.Issues = append(result.Issues, fmt.Sprintf("Worktree path doesn't exist: %s", worktreePath))
				result.Actions = append(result.Actions, "Consider pausing this session or recreating worktree")
			} else {
				result.Actions = append(result.Actions, "Worktree path exists")
			}
		}
	}

	return result
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
