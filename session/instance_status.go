package session

import (
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/log"
	"fmt"
	"sync"
)

// InstanceStatusInfo provides extended status information for an instance.
type InstanceStatusInfo struct {
	BasicStatus        Status         // Running, Paused, Ready
	ClaudeStatus       detection.DetectedStatus // If ClaudeController is active
	StatusContext      string         // Context/details about current status (e.g., error message)
	PendingApprovals   int            // Number of pending approvals
	QueuedCommands     int            // Number of queued commands
	LastCommandStatus  string         // Status of last command
	IsControllerActive bool           // Whether ClaudeController is running
	IdleState          detection.IdleStateInfo  // NEW: Idle state information
}

// InstanceStatusManager manages status information for instances.
type InstanceStatusManager struct {
	controllers map[string]*ClaudeController // Map of instance title to controller
	mu          sync.RWMutex
}

// NewInstanceStatusManager creates a new status manager.
func NewInstanceStatusManager() *InstanceStatusManager {
	return &InstanceStatusManager{
		controllers: make(map[string]*ClaudeController),
	}
}

// RegisterController registers a controller for an instance.
func (ism *InstanceStatusManager) RegisterController(instanceTitle string, controller *ClaudeController) {
	ism.mu.Lock()
	defer ism.mu.Unlock()
	ism.controllers[instanceTitle] = controller
	log.DebugLog.Printf("[RegisterController] Registered controller for '%s' (total registered: %d)", instanceTitle, len(ism.controllers))
}

// UnregisterController removes a controller for an instance.
func (ism *InstanceStatusManager) UnregisterController(instanceTitle string) {
	ism.mu.Lock()
	defer ism.mu.Unlock()
	delete(ism.controllers, instanceTitle)
}

// GetController retrieves a controller for an instance.
func (ism *InstanceStatusManager) GetController(instanceTitle string) (*ClaudeController, bool) {
	ism.mu.RLock()
	defer ism.mu.RUnlock()
	controller, exists := ism.controllers[instanceTitle]
	return controller, exists
}

// GetAllControllers returns all registered controllers.
func (ism *InstanceStatusManager) GetAllControllers() map[string]*ClaudeController {
	ism.mu.RLock()
	defer ism.mu.RUnlock()

	// Return a copy to prevent concurrent modification
	controllers := make(map[string]*ClaudeController, len(ism.controllers))
	for k, v := range ism.controllers {
		controllers[k] = v
	}
	return controllers
}

// GetStatus retrieves comprehensive status for an instance.
func (ism *InstanceStatusManager) GetStatus(instance *Instance) InstanceStatusInfo {
	ism.mu.RLock()
	controller, exists := ism.controllers[instance.Title]
	ism.mu.RUnlock()

	// Debug logging to diagnose controller detection issue
	log.DebugLog.Printf("[GetStatus] Session '%s': exists=%v, controller!=nil=%v, IsStarted=%v",
		instance.Title, exists, controller != nil, exists && controller != nil && controller.IsStarted())

	info := InstanceStatusInfo{
		BasicStatus:        instance.Status,
		IsControllerActive: exists && controller != nil && controller.IsStarted(),
	}

	if info.IsControllerActive {
		// Get Claude status with context (includes error details, matched patterns, etc.)
		claudeStatus, statusContext := controller.GetCurrentStatus()
		info.ClaudeStatus = claudeStatus
		info.StatusContext = statusContext

		// Get queued commands count
		commands := controller.GetQueuedCommands()
		info.QueuedCommands = len(commands)

		// Get current command if any
		currentCmd := controller.GetCurrentCommand()
		if currentCmd != nil {
			info.LastCommandStatus = fmt.Sprintf("Executing: %s", currentCmd.Text)
		}

		// Get idle state information
		info.IdleState = controller.GetIdleStateInfo()
	}

	return info
}

// GetStatusIcon returns an icon representing the instance status.
func (info InstanceStatusInfo) GetStatusIcon() string {
	if !info.IsControllerActive {
		switch info.BasicStatus {
		case Running:
			return "●" // Running but no controller
		case Paused:
			return "⏸"
		case Ready:
			return "●"
		default:
			return "?"
		}
	}

	// Controller active - use Claude status
	switch info.ClaudeStatus {
	case detection.StatusReady:
		return "●" // Ready
	case detection.StatusProcessing:
		return "◐" // Working
	case detection.StatusNeedsApproval:
		return "❗" // Needs attention
	case detection.StatusError:
		return "✖" // Error
	default:
		return "●"
	}
}

// GetStatusDescription returns a human-readable status description.
func (info InstanceStatusInfo) GetStatusDescription() string {
	if !info.IsControllerActive {
		switch info.BasicStatus {
		case Running:
			return "Running"
		case Ready:
			return "Ready"
		case Paused:
			return "Paused"
		case Loading:
			return "Loading"
		case NeedsApproval:
			return "Needs Approval"
		default:
			return "Unknown"
		}
	}

	var desc string
	switch info.ClaudeStatus {
	case detection.StatusReady:
		desc = "Ready"
	case detection.StatusProcessing:
		desc = "Processing"
	case detection.StatusNeedsApproval:
		desc = "Needs Approval"
	case detection.StatusError:
		desc = "Error"
	case detection.StatusUnknown:
		desc = "Unknown"
	default:
		desc = "Unknown"
	}

	if info.QueuedCommands > 0 {
		desc += fmt.Sprintf(" (%d queued)", info.QueuedCommands)
	}

	if info.PendingApprovals > 0 {
		desc += fmt.Sprintf(" [%d approvals]", info.PendingApprovals)
	}

	return desc
}

// HasPendingWork returns true if the instance has pending commands or approvals.
func (info InstanceStatusInfo) HasPendingWork() bool {
	return info.QueuedCommands > 0 || info.PendingApprovals > 0
}

// IsWaitingForUser returns true if the instance is waiting for user input.
func (info InstanceStatusInfo) IsWaitingForUser() bool {
	return info.ClaudeStatus == detection.StatusNeedsApproval ||
		info.PendingApprovals > 0
}

// NeedsAttention returns true if the instance requires user attention.
func (info InstanceStatusInfo) NeedsAttention() bool {
	return info.IsWaitingForUser() || info.ClaudeStatus == detection.StatusError
}

// GetColorCode returns a color code for the status (for lipgloss styling).
func (info InstanceStatusInfo) GetColorCode() string {
	if info.ClaudeStatus == detection.StatusError {
		return "196" // Red
	}

	if info.NeedsAttention() {
		return "214" // Orange
	}

	if info.ClaudeStatus == detection.StatusProcessing {
		return "39" // Blue
	}

	if info.BasicStatus == Running && info.IsControllerActive {
		return "82" // Green
	}

	if info.BasicStatus == Paused {
		return "240" // Gray
	}

	return "250" // Default gray
}
