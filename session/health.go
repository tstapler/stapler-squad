package session

import (
	"fmt"
	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/log"
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
}

// failureThreshold is the number of consecutive failed health checks before
// a recovery attempt is triggered. Set to 2 to require two consecutive misses.
const failureThreshold = 2

// NewSessionHealthChecker creates a new session health checker
func NewSessionHealthChecker(storage *Storage) *SessionHealthChecker {
	return &SessionHealthChecker{
		storage:       storage,
		tmuxSocket:    realTmuxSocketQuerier{},
		failureCounts: make(map[string]int),
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

	// Check if instance thinks it's started but tmux session doesn't exist
	if instance.Started() {
		if !instance.TmuxAlive() {
			result.IsHealthy = false
			result.Issues = append(result.Issues, "Instance marked as started but tmux session doesn't exist")

			// Debounce: only recover after failureThreshold consecutive failures.
			// This prevents spurious recovery attempts caused by transient check glitches.
			h.failureCountsMu.Lock()
			h.failureCounts[instance.Title]++
			count := h.failureCounts[instance.Title]
			h.failureCountsMu.Unlock()

			if count < failureThreshold {
				log.Debug("health check: deferring recovery", "session", instance.Title, "count", count, "threshold", failureThreshold)
				result.Actions = append(result.Actions, fmt.Sprintf("Failure %d/%d: deferring recovery", count, failureThreshold))
			} else {
				// Threshold reached - attempt recovery
				h.failureCountsMu.Lock()
				h.failureCounts[instance.Title] = 0 // Reset counter after attempt
				h.failureCountsMu.Unlock()

				result.RecoveryAttempted = true
				if err := instance.Start(false); err != nil {
					result.Issues = append(result.Issues, fmt.Sprintf("Recovery failed: %v", err))
					result.RecoverySuccess = false
					result.Actions = append(result.Actions, "Failed to recreate tmux session")
				} else {
					result.RecoverySuccess = true
					result.Actions = append(result.Actions, "Successfully recreated tmux session")
					// Re-check health after recovery
					if instance.TmuxAlive() {
						result.IsHealthy = true
					} else {
						result.Issues = append(result.Issues, "Session still unhealthy after recovery attempt")
					}
				}
			}
		} else {
			// Session is healthy - reset any accumulated failure count
			h.failureCountsMu.Lock()
			delete(h.failureCounts, instance.Title)
			h.failureCountsMu.Unlock()
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
