package session

import (
	"claude-squad/log"
	"fmt"
	"os"
	"time"
)

// SessionHealthChecker manages session health validation and recovery
type SessionHealthChecker struct {
	storage *Storage
}

// NewSessionHealthChecker creates a new session health checker
func NewSessionHealthChecker(storage *Storage) *SessionHealthChecker {
	return &SessionHealthChecker{
		storage: storage,
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

	results := make([]HealthCheckResult, 0, len(instances))

	for _, instance := range instances {
		result := h.checkSingleSession(instance)
		results = append(results, result)

		// Log any issues found
		if !result.IsHealthy {
			log.WarningLog.Printf("Health check found issues for session '%s': %v",
				result.InstanceTitle, result.Issues)
			if result.RecoveryAttempted {
				if result.RecoverySuccess {
					log.DebugLog.Printf("Successfully recovered session '%s'", result.InstanceTitle)
				} else {
					log.ErrorLog.Printf("Failed to recover session '%s'", result.InstanceTitle)
				}
			}
		}
	}

	return results, nil
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

	// Check if instance thinks it's started but tmux session doesn't exist
	if instance.Started() {
		if !instance.TmuxAlive() {
			result.IsHealthy = false
			result.Issues = append(result.Issues, "Instance marked as started but tmux session doesn't exist")

			// Attempt recovery by recreating the tmux session
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
		} else {
			result.Actions = append(result.Actions, "Tmux session is healthy")
		}
	}

	// Check worktree existence for non-paused instances
	if !instance.Paused() && instance.gitWorktree != nil {
		worktreePath := instance.gitWorktree.GetWorktreePath()
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

	log.DebugLog.Printf("Session recovery completed: %d recovered, %d failed", recoveredCount, failedCount)

	// Save the updated state if any recoveries were attempted
	if recoveredCount > 0 || failedCount > 0 {
		instances, err := h.storage.LoadInstances()
		if err != nil {
			return fmt.Errorf("failed to reload instances after recovery: %w", err)
		}

		if err := h.storage.SaveInstances(instances); err != nil {
			log.WarningLog.Printf("Failed to save instances after recovery: %v", err)
		}
	}

	return nil
}

// ScheduledHealthCheck runs health checks at regular intervals
func (h *SessionHealthChecker) ScheduledHealthCheck(interval time.Duration, stopChan <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.DebugLog.Printf("Starting scheduled health checks every %v", interval)

	for {
		select {
		case <-ticker.C:
			if err := h.RecoverUnhealthySessions(); err != nil {
				log.ErrorLog.Printf("Scheduled health check failed: %v", err)
			}
		case <-stopChan:
			log.DebugLog.Printf("Stopping scheduled health checks")
			return
		}
	}
}
