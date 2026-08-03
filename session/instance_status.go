package session

import (
	"fmt"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
)

// InstanceStatusInfo provides extended status information for an instance.
type InstanceStatusInfo struct {
	BasicStatus        Status                   // Creating, Active, Paused, Stopped, Hibernated
	ClaudeStatus       detection.DetectedStatus // If ClaudeController is active
	StatusContext      string                   // Context/details about current status (e.g., error message)
	PendingApprovals   int                      // Number of pending approvals
	QueuedCommands     int                      // Number of queued commands
	LastCommandStatus  string                   // Status of last command
	IsControllerActive bool                     // Whether ClaudeController is running
	IdleState          detection.IdleStateInfo  // NEW: Idle state information
	// SubagentCount is the count of background agents/shells/monitors from the
	// WaitingForAgent detector; 0 unless ClaudeStatus == detection.StatusWaitingForAgent.
	SubagentCount int
}

// InstanceStatusManager manages status information for instances.
type InstanceStatusManager struct {
	// ponytail: xsync.Map replaces map+RWMutex — lock-free reads on the hot GetStatus path
	controllers *xsync.Map[string, *ClaudeController]
}

// NewInstanceStatusManager creates a new status manager.
func NewInstanceStatusManager() *InstanceStatusManager {
	return &InstanceStatusManager{
		controllers: xsync.NewMap[string, *ClaudeController](),
	}
}

// RegisterController registers a controller for an instance.
func (ism *InstanceStatusManager) RegisterController(instanceTitle string, controller *ClaudeController) {
	ism.controllers.Store(instanceTitle, controller)
	log.Debug("registered controller", "session", instanceTitle, "count", ism.controllers.Size())
}

// UnregisterController removes a controller for an instance.
func (ism *InstanceStatusManager) UnregisterController(instanceTitle string) {
	ism.controllers.Delete(instanceTitle)
}

// GetController retrieves a controller for an instance.
func (ism *InstanceStatusManager) GetController(instanceTitle string) (*ClaudeController, bool) {
	return ism.controllers.Load(instanceTitle)
}

// GetAllControllers returns all registered controllers.
func (ism *InstanceStatusManager) GetAllControllers() map[string]*ClaudeController {
	out := make(map[string]*ClaudeController, ism.controllers.Size())
	ism.controllers.Range(func(k string, v *ClaudeController) bool {
		out[k] = v
		return true
	})
	return out
}

// GetStatus retrieves comprehensive status for an instance.
func (ism *InstanceStatusManager) GetStatus(instance *Instance) InstanceStatusInfo {
	controller, exists := ism.controllers.Load(instance.Title)

	info := InstanceStatusInfo{
		// Snapshot(), not instance.Status directly — an unguarded read of instance.Status
		// doesn't synchronize with actor commands' direct field writes (see
		// Instance.GetStatus's doc comment).
		BasicStatus:        instance.Snapshot().Status,
		IsControllerActive: exists && controller != nil && controller.IsStarted(),
	}

	if info.IsControllerActive {
		// Combined call: one hash + one cache read covers both status and idle state.
		claudeStatus, statusContext, idleInfo, subagentCount := controller.GetStatusAndIdleInfo()
		info.ClaudeStatus = claudeStatus
		info.StatusContext = statusContext
		info.IdleState = idleInfo
		info.SubagentCount = subagentCount

		info.QueuedCommands = controller.GetQueuedCommandsCount()

		currentCmd := controller.GetCurrentCommand()
		if currentCmd != nil {
			info.LastCommandStatus = "Executing: " + currentCmd.Text
		}
	}

	return info
}

// GetStatusIcon returns an icon representing the instance status.
func (info InstanceStatusInfo) GetStatusIcon() string {
	if !info.IsControllerActive {
		switch info.BasicStatus {
		case Active:
			return "●" // Active but no controller
		case Paused:
			return "⏸"
		case Hibernated:
			return "❄"
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
	case detection.StatusInputRequired:
		return "⌨" // Waiting for input
	case detection.StatusWaitingForAgent:
		return "⏳" // Waiting for background agent
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
		case Active:
			return "Active"
		case Creating:
			return "Creating"
		case Paused:
			return "Paused"
		case Stopped:
			return "Stopped"
		case Hibernated:
			return "Hibernated"
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
	case detection.StatusExecuting:
		desc = "Executing"
	case detection.StatusIdle:
		desc = "Idle"
	case detection.StatusNeedsApproval:
		desc = "Needs Approval"
	case detection.StatusInputRequired:
		desc = "Needs Input"
	case detection.StatusSuccess:
		desc = "Completed"
	case detection.StatusWaitingForAgent:
		desc = "Waiting for Agent"
	case detection.StatusTestsFailing:
		desc = "Tests Failing"
	case detection.StatusError:
		desc = "Error"
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
		info.ClaudeStatus == detection.StatusInputRequired ||
		info.PendingApprovals > 0
}

// NeedsAttention returns true if the instance requires user attention.
func (info InstanceStatusInfo) NeedsAttention() bool {
	return info.IsWaitingForUser() ||
		info.ClaudeStatus == detection.StatusError ||
		info.ClaudeStatus == detection.StatusTestsFailing
}

// GetColorCode returns a color code for the status (for lipgloss styling).
func (info InstanceStatusInfo) GetColorCode() string {
	if info.ClaudeStatus == detection.StatusError {
		return "196" // Red
	}

	if info.NeedsAttention() {
		return "214" // Orange
	}

	if info.ClaudeStatus == detection.StatusProcessing || info.ClaudeStatus == detection.StatusWaitingForAgent {
		return "39" // Blue
	}

	if info.BasicStatus == Active && info.IsControllerActive {
		return "82" // Green
	}

	if info.BasicStatus == Paused || info.BasicStatus == Hibernated {
		return "240" // Gray
	}

	return "250" // Default gray
}
