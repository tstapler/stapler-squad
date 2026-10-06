package session

import (
	"fmt"
	"sync"

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

	// piSources holds registered PiStatusSources, keyed by instance title —
	// a parallel map to controllers, not a shared interface. See plan.md's
	// Pattern Decisions table: pi status is a purpose-built event-to-status
	// mapping (session/pi_status_source.go) with no PTY-scraping, queued
	// commands, or subagent concepts, so unifying it behind
	// ClaudeController's interface would force those concepts onto pi for
	// no benefit.
	piSources *xsync.Map[string, *PiStatusSource]

	// approvalProviderMu guards approvalProvider, set at most once during
	// startup wiring (see server/dependencies.go) and read on every
	// GetStatus() call for a pi-fallback instance — mirrors
	// ReviewQueuePoller's approvalProvider field/mutex pattern
	// (session/review_queue_poller.go).
	approvalProviderMu sync.RWMutex
	approvalProvider   ApprovalMetadataProvider
}

// NewInstanceStatusManager creates a new status manager.
func NewInstanceStatusManager() *InstanceStatusManager {
	return &InstanceStatusManager{
		controllers: xsync.NewMap[string, *ClaudeController](),
		piSources:   xsync.NewMap[string, *PiStatusSource](),
	}
}

// RegisterPiStatusSource registers a PiStatusSource for an instance. Mirrors
// RegisterController.
func (ism *InstanceStatusManager) RegisterPiStatusSource(instanceTitle string, source *PiStatusSource) {
	ism.piSources.Store(instanceTitle, source)
	log.Debug("registered pi status source", "session", instanceTitle, "count", ism.piSources.Size())
}

// UnregisterPiStatusSource removes a PiStatusSource for an instance. Mirrors
// UnregisterController.
func (ism *InstanceStatusManager) UnregisterPiStatusSource(instanceTitle string) {
	ism.piSources.Delete(instanceTitle)
}

// GetPiStatusSource retrieves a PiStatusSource for an instance.
func (ism *InstanceStatusManager) GetPiStatusSource(instanceTitle string) (*PiStatusSource, bool) {
	return ism.piSources.Load(instanceTitle)
}

// SetApprovalProvider wires the shared pending-approval lookup (backed by
// *services.ApprovalStore in production — see server/dependencies.go) used
// by GetStatus()'s pi-fallback branch to override an inferred Idle status
// with NeedsApproval (Story 5.3.1). Optional: GetStatus() skips the override
// entirely when no provider has been set (e.g. in tests that don't need it).
func (ism *InstanceStatusManager) SetApprovalProvider(provider ApprovalMetadataProvider) {
	ism.approvalProviderMu.Lock()
	defer ism.approvalProviderMu.Unlock()
	ism.approvalProvider = provider
}

func (ism *InstanceStatusManager) getApprovalProvider() ApprovalMetadataProvider {
	ism.approvalProviderMu.RLock()
	defer ism.approvalProviderMu.RUnlock()
	return ism.approvalProvider
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

		return info
	}

	// No Claude controller registered — fall back to a PiStatusSource if one
	// is registered for this instance (Story 5.2.2). This is a parallel
	// lookup, not a shared-interface dispatch — see the Pattern Decision on
	// the piSources field.
	if src, ok := ism.piSources.Load(instance.Title); ok && src != nil {
		info.ClaudeStatus = src.CurrentStatus()
		info.StatusContext = src.StatusContext()
		info.IsControllerActive = true
		// QueuedCommands, SubagentCount, and LastCommandStatus intentionally
		// stay at their zero value: pi has no queued-command or subagent
		// concepts (Pattern Decision, plan.md Epic 5.2).

		// Story 5.3.1: don't show "idle" while pi is actually blocked on a
		// pending human approval decision. PendingApproval.SessionID is
		// stored as the session's stable ID — UUID when present, Title only
		// as a fallback (see ApprovalHandler.resolveSessionID) — so look up
		// by UUID first, then Title, mirroring
		// ReviewQueuePoller.pollOnce's identical fallback.
		if info.ClaudeStatus == detection.StatusIdle {
			if provider := ism.getApprovalProvider(); provider != nil {
				snap := instance.Snapshot()
				approvals := provider.GetApprovalMetadataBySession(snap.UUID)
				if len(approvals) == 0 && snap.UUID != snap.Title {
					approvals = provider.GetApprovalMetadataBySession(snap.Title)
				}
				if len(approvals) > 0 {
					info.ClaudeStatus = detection.StatusNeedsApproval
					info.PendingApprovals = len(approvals)
				}
			}
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
	case detection.StatusCompacting:
		return "⟳" // Compacting context
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
		case Crashed:
			return "Crashed"
		case PermanentlyFailed:
			return "Failed"
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
	case detection.StatusCompacting:
		desc = "Compacting"
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
