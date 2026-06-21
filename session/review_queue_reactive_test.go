package session

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// reactiveTestStatusProvider implements StatusProvider for reactive queue tests.
type reactiveTestStatusProvider struct {
	statusByTitle map[string]InstanceStatusInfo
}

func (m *reactiveTestStatusProvider) GetStatus(inst *Instance) InstanceStatusInfo {
	if info, ok := m.statusByTitle[inst.Title]; ok {
		return info
	}
	return InstanceStatusInfo{}
}

func (m *reactiveTestStatusProvider) GetController(title string) (*ClaudeController, bool) {
	return nil, false
}

// TestReactiveQueueManager_StatusChange_AddsToQueue verifies that CheckSession
// adds an idle session to the review queue immediately (AC-4).
func TestReactiveQueueManager_StatusChange_AddsToQueue(t *testing.T) {
	t.Parallel()

	queue := NewReviewQueue()
	statusProvider := &reactiveTestStatusProvider{
		statusByTitle: make(map[string]InstanceStatusInfo),
	}

	cfg := DefaultReviewQueuePollerConfig()
	cfg.PollInterval = 50 * time.Millisecond
	cfg.SlowPollInterval = 100 * time.Millisecond
	cfg.IdleThreshold = 10 * time.Millisecond

	poller := NewReviewQueuePollerWithConfig(queue, statusProvider, nil, cfg)

	inst := &Instance{
		Title:  "reactive-test",
		Status: Active,
	}
	inst.started = true
	inst.LastMeaningfulOutput = time.Now().Add(-10 * time.Second)

	poller.AddInstance(inst)

	idleContent := "✻ Perambulated for 1h 5m\n\n> ▌\n? for shortcuts"
	poller.injectCachedContent(inst.Title, idleContent)

	statusProvider.statusByTitle[inst.Title] = InstanceStatusInfo{
		BasicStatus:        Active,
		IsControllerActive: false,
		ClaudeStatus:       detection.StatusIdle,
	}

	poller.CheckSession(inst)

	err := wait.WaitForCondition(func() bool {
		_, exists := queue.Get(inst.Title)
		return exists
	}, wait.WaitConfig{
		Timeout:      200 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Errorf("session not added to review queue within 200ms after CheckSession call: %v", err)
	}
}

// TestReactiveQueueManager_ActiveSession_RemovedFromQueue verifies that a session
// with active content and recent user response is removed from the queue (AC-4 / UR-5).
func TestReactiveQueueManager_ActiveSession_RemovedFromQueue(t *testing.T) {
	t.Parallel()

	queue := NewReviewQueue()
	statusProvider := &reactiveTestStatusProvider{
		statusByTitle: make(map[string]InstanceStatusInfo),
	}

	cfg := DefaultReviewQueuePollerConfig()
	cfg.PollInterval = 50 * time.Millisecond
	poller := NewReviewQueuePollerWithConfig(queue, statusProvider, nil, cfg)

	inst := &Instance{
		Title:  "active-test",
		Status: Active,
	}
	inst.started = true
	inst.LastMeaningfulOutput = time.Now()

	queue.Add(&ReviewItem{
		SessionID:    inst.Title,
		SessionName:  inst.Title,
		Reason:       ReasonIdleTimeout,
		Priority:     PriorityLow,
		DetectedAt:   time.Now().Add(-1 * time.Minute),
		LastActivity: time.Now().Add(-1 * time.Minute),
	})

	poller.AddInstance(inst)

	activeContent := "✻ Perambulating... (30s · ↑ 1.2k tokens)\n\n> ▌\nesc to interrupt"
	poller.injectCachedContent(inst.Title, activeContent)

	inst.LastUserResponse = time.Now().Add(-100 * time.Millisecond)

	statusProvider.statusByTitle[inst.Title] = InstanceStatusInfo{
		BasicStatus:        Active,
		IsControllerActive: true,
		ClaudeStatus:       detection.StatusExecuting,
		IdleState: detection.IdleStateInfo{
			State: detection.IdleStateActive,
		},
	}

	poller.CheckSession(inst)

	err := wait.WaitForCondition(func() bool {
		_, exists := queue.Get(inst.Title)
		return !exists
	}, wait.WaitConfig{
		Timeout:      200 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Errorf("active session not removed from review queue within 200ms after CheckSession call: %v", err)
	}
}
