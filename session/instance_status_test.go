package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/detection"
)

// fakeApprovalMetadataProvider is a minimal ApprovalMetadataProvider stand-in
// for tests, keyed by session ID (UUID or Title, matching how
// GetStatus/ReviewQueuePoller look approvals up).
type fakeApprovalMetadataProvider struct {
	bySessionID map[string][]ApprovalMetadata
}

func (f *fakeApprovalMetadataProvider) GetApprovalMetadataBySession(sessionID string) []ApprovalMetadata {
	return f.bySessionID[sessionID]
}

// TestInstanceStatusManager_GetStatus_PiFallback covers Story 5.2.2's AC: an
// instance with program "pi", a registered PiStatusSource reporting
// StatusExecuting, and no registered ClaudeController falls back to the
// PiStatusSource's status.
func TestInstanceStatusManager_GetStatus_PiFallback(t *testing.T) {
	ism := NewInstanceStatusManager()
	inst := &Instance{Title: "pi-session", Program: "pi"}

	src := NewPiStatusSource("pi-session", nil)
	src.handleEvent(PiToolExecutionStartEvent{Type: "tool_execution_start", ToolCallID: "call-1", ToolName: "bash"})
	ism.RegisterPiStatusSource("pi-session", src)

	info := ism.GetStatus(inst)

	if !info.IsControllerActive {
		t.Errorf("IsControllerActive = false, want true (pi fallback)")
	}
	if info.ClaudeStatus != detection.StatusExecuting {
		t.Errorf("ClaudeStatus = %v, want StatusExecuting", info.ClaudeStatus)
	}
	if info.QueuedCommands != 0 || info.SubagentCount != 0 || info.LastCommandStatus != "" {
		t.Errorf("expected zero-value QueuedCommands/SubagentCount/LastCommandStatus for a pi fallback, got %+v", info)
	}
}

// TestInstanceStatusManager_GetStatus_NoPiSourceNoController verifies that
// with neither a Claude controller nor a PiStatusSource registered,
// IsControllerActive stays false and ClaudeStatus stays at its zero value.
func TestInstanceStatusManager_GetStatus_NoPiSourceNoController(t *testing.T) {
	ism := NewInstanceStatusManager()
	inst := &Instance{Title: "plain-session"}

	info := ism.GetStatus(inst)

	if info.IsControllerActive {
		t.Errorf("IsControllerActive = true, want false")
	}
	if info.ClaudeStatus != detection.StatusUnknown {
		t.Errorf("ClaudeStatus = %v, want StatusUnknown (zero value)", info.ClaudeStatus)
	}
}

// TestInstanceStatusManager_GetStatus_PendingApprovalOverridesIdle covers
// Story 5.3.1's first AC bullet: a pi session whose PiStatusSource would
// report StatusIdle, but which has a live pending approval, reports
// StatusNeedsApproval instead.
func TestInstanceStatusManager_GetStatus_PendingApprovalOverridesIdle(t *testing.T) {
	ism := NewInstanceStatusManager()
	inst := &Instance{Title: "pi-session", UUID: "pi-session-uuid", Program: "pi"}

	src := NewPiStatusSource("pi-session", nil)
	src.status.Store(int32(detection.StatusIdle)) // force idle without waiting on the real timer
	ism.RegisterPiStatusSource("pi-session", src)

	provider := &fakeApprovalMetadataProvider{
		bySessionID: map[string][]ApprovalMetadata{
			"pi-session-uuid": {{ApprovalID: "appr-1", ToolName: "bash"}},
		},
	}
	ism.SetApprovalProvider(provider)

	info := ism.GetStatus(inst)

	if info.ClaudeStatus != detection.StatusNeedsApproval {
		t.Errorf("ClaudeStatus = %v, want StatusNeedsApproval", info.ClaudeStatus)
	}
	if info.PendingApprovals != 1 {
		t.Errorf("PendingApprovals = %d, want 1", info.PendingApprovals)
	}
}

// TestInstanceStatusManager_GetStatus_NoPendingApprovalLeavesIdle covers
// Story 5.3.1's second AC bullet: with zero pending approvals, idle
// inference is unaffected.
func TestInstanceStatusManager_GetStatus_NoPendingApprovalLeavesIdle(t *testing.T) {
	ism := NewInstanceStatusManager()
	inst := &Instance{Title: "pi-session", UUID: "pi-session-uuid", Program: "pi"}

	src := NewPiStatusSource("pi-session", nil)
	src.status.Store(int32(detection.StatusIdle))
	ism.RegisterPiStatusSource("pi-session", src)

	provider := &fakeApprovalMetadataProvider{bySessionID: map[string][]ApprovalMetadata{}}
	ism.SetApprovalProvider(provider)

	info := ism.GetStatus(inst)

	if info.ClaudeStatus != detection.StatusIdle {
		t.Errorf("ClaudeStatus = %v, want StatusIdle unaffected", info.ClaudeStatus)
	}
	if info.PendingApprovals != 0 {
		t.Errorf("PendingApprovals = %d, want 0", info.PendingApprovals)
	}
}

// TestInstanceStatusManager_GetStatus_NoApprovalProviderLeavesIdle verifies
// that when no ApprovalProvider has been wired at all (the zero-value
// InstanceStatusManager, matching most existing callers/tests), the idle
// override is skipped rather than panicking on a nil provider.
func TestInstanceStatusManager_GetStatus_NoApprovalProviderLeavesIdle(t *testing.T) {
	ism := NewInstanceStatusManager()
	inst := &Instance{Title: "pi-session", Program: "pi"}

	src := NewPiStatusSource("pi-session", nil)
	src.status.Store(int32(detection.StatusIdle))
	ism.RegisterPiStatusSource("pi-session", src)

	info := ism.GetStatus(inst)

	if info.ClaudeStatus != detection.StatusIdle {
		t.Errorf("ClaudeStatus = %v, want StatusIdle (no provider wired)", info.ClaudeStatus)
	}
}

// TestInstanceStatusManager_RegisterUnregisterPiStatusSource exercises the
// register/unregister/get round trip.
func TestInstanceStatusManager_RegisterUnregisterPiStatusSource(t *testing.T) {
	ism := NewInstanceStatusManager()
	src := NewPiStatusSource("s1", nil)

	ism.RegisterPiStatusSource("s1", src)
	if got, ok := ism.GetPiStatusSource("s1"); !ok || got != src {
		t.Errorf("GetPiStatusSource after Register = (%v, %v), want (src, true)", got, ok)
	}

	ism.UnregisterPiStatusSource("s1")
	if _, ok := ism.GetPiStatusSource("s1"); ok {
		t.Errorf("GetPiStatusSource after Unregister: found, want not found")
	}
}
