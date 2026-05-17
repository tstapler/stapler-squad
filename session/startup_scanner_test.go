package session

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// fakeStatusProvider is a test double for StatusProvider that returns predetermined statuses.
type fakeStatusProvider struct {
	statuses map[string]InstanceStatusInfo
}

func (f *fakeStatusProvider) GetStatus(inst *Instance) InstanceStatusInfo {
	if s, ok := f.statuses[inst.Title]; ok {
		return s
	}
	return InstanceStatusInfo{}
}

func (f *fakeStatusProvider) GetController(instanceTitle string) (*ClaudeController, bool) {
	return nil, false
}

// fakeContentProvider is a test double for ContentProvider that returns predetermined content.
type fakeContentProvider struct {
	content map[string]string
}

func (f *fakeContentProvider) GetContent(inst *Instance, _ InstanceStatusInfo, _ map[string]time.Time) string {
	return f.content[inst.Title]
}

func (f *fakeContentProvider) EvictInstance(title string) {
	delete(f.content, title)
}

func newFakeContentProvider(content map[string]string) *fakeContentProvider {
	return &fakeContentProvider{content: content}
}

func newFakeStatusProvider(statuses map[string]InstanceStatusInfo) *fakeStatusProvider {
	return &fakeStatusProvider{statuses: statuses}
}

// makeStartedInstance creates a minimal started Instance for StartupScanner tests.
func makeStartedInstance(title string) *Instance {
	inst := &Instance{
		Title:  title,
		UUID:   "uuid-" + title,
		Status: Running,
	}
	inst.started = true
	return inst
}

// TestStartupScanner_Scan_AddsSessionWithApprovalPrompt verifies that Scan() adds
// a session to the review queue when terminal content contains an approval prompt.
// This is the end-to-end regression for AC-1: startup scan detects approval without
// user navigation.
func TestStartupScanner_Scan_AddsSessionWithApprovalPrompt(t *testing.T) {
	inst := makeStartedInstance("session-with-approval")
	approvalContent := "Do you want to allow reading /etc/hosts?\nYes, allow once\nNo"

	statusProvider := newFakeStatusProvider(map[string]InstanceStatusInfo{
		inst.Title: {IsControllerActive: false},
	})
	contentProvider := newFakeContentProvider(map[string]string{
		inst.Title: approvalContent,
	})

	queue := NewReviewQueue()
	scanner := NewStartupScanner(statusProvider, contentProvider)
	added := scanner.Scan([]*Instance{inst}, queue)

	if added == 0 {
		t.Fatal("expected session with approval prompt to be added to review queue")
	}
	item, exists := queue.Get(inst.Title)
	if !exists {
		t.Fatal("expected session in review queue")
	}
	if item.Reason != ReasonApprovalPending {
		t.Errorf("expected reason %s, got %s", ReasonApprovalPending, item.Reason)
	}
}

// TestStartupScanner_Scan_SkipsSessionWithNoApproval verifies AC-2: a recently-active
// session (no approval prompt, no idle/stale threshold) is NOT added to the review queue.
// UpdatedAt and LastMeaningfulOutput are set to now to prevent idle/staleness triggers.
func TestStartupScanner_Scan_SkipsSessionWithNoApproval(t *testing.T) {
	inst := makeStartedInstance("active-session-no-approval")
	now := time.Now()
	inst.UpdatedAt = now            // prevents idle threshold (5s)
	inst.LastMeaningfulOutput = now // prevents staleness threshold (2min)

	statusProvider := newFakeStatusProvider(map[string]InstanceStatusInfo{
		inst.Title: {IsControllerActive: false},
	})
	contentProvider := newFakeContentProvider(map[string]string{
		inst.Title: "$ go test ./...\nok  github.com/example/project 0.042s\n",
	})

	queue := NewReviewQueue()
	scanner := NewStartupScanner(statusProvider, contentProvider)
	added := scanner.Scan([]*Instance{inst}, queue)

	if added != 0 {
		t.Errorf("expected 0 sessions added for recently-active session, got %d (false positive)", added)
	}
	if _, exists := queue.Get(inst.Title); exists {
		t.Error("recently-active session with no prompt must not appear in review queue")
	}
}

// TestStartupScanner_Scan_SkipsPausedInstances verifies that paused sessions
// are not evaluated during startup scan.
func TestStartupScanner_Scan_SkipsPausedInstances(t *testing.T) {
	inst := &Instance{
		Title:  "paused-session",
		Status: Paused,
	}
	inst.started = true

	approvalContent := "Yes, allow once"
	statusProvider := newFakeStatusProvider(map[string]InstanceStatusInfo{})
	contentProvider := newFakeContentProvider(map[string]string{
		inst.Title: approvalContent,
	})

	queue := NewReviewQueue()
	scanner := NewStartupScanner(statusProvider, contentProvider)
	added := scanner.Scan([]*Instance{inst}, queue)

	if added != 0 {
		t.Errorf("paused session must not be scanned, got %d additions", added)
	}
}

// TestStartupScanner_Scan_ControllerActiveWithApproval verifies that a session
// whose active controller reports StatusNeedsApproval is added to the queue.
func TestStartupScanner_Scan_ControllerActiveWithApproval(t *testing.T) {
	inst := makeStartedInstance("controller-approval-session")

	statusProvider := newFakeStatusProvider(map[string]InstanceStatusInfo{
		inst.Title: {
			IsControllerActive: true,
			ClaudeStatus:       detection.StatusNeedsApproval,
		},
	})
	contentProvider := newFakeContentProvider(map[string]string{
		inst.Title: "",
	})

	queue := NewReviewQueue()
	scanner := NewStartupScanner(statusProvider, contentProvider)
	added := scanner.Scan([]*Instance{inst}, queue)

	if added == 0 {
		t.Fatal("session with active controller reporting NeedsApproval must be added to queue")
	}
	item, exists := queue.Get(inst.Title)
	if !exists {
		t.Fatal("session must be in queue")
	}
	if item.Reason != ReasonApprovalPending {
		t.Errorf("expected reason %s, got %s", ReasonApprovalPending, item.Reason)
	}
}

// TestStartupScanner_Scan_MultipleSessionsMixedState verifies that Scan() correctly
// partitions a mixed set of instances: sessions with approval prompts are added,
// recently-active sessions with no prompts are not.
func TestStartupScanner_Scan_MultipleSessionsMixedState(t *testing.T) {
	needsApproval := makeStartedInstance("needs-approval")
	// activeSession has fresh timestamps so idle/staleness thresholds won't trigger.
	activeSession := makeStartedInstance("active-no-prompt")
	now := time.Now()
	activeSession.UpdatedAt = now
	activeSession.LastMeaningfulOutput = now
	inputRequired := makeStartedInstance("input-required")

	statusProvider := newFakeStatusProvider(map[string]InstanceStatusInfo{
		needsApproval.Title: {IsControllerActive: false},
		activeSession.Title: {IsControllerActive: false},
		inputRequired.Title: {IsControllerActive: false},
	})
	contentProvider := newFakeContentProvider(map[string]string{
		needsApproval.Title: "Do you want to allow this action?\nYes, allow once",
		activeSession.Title: "$ go build ./...\nok",
		inputRequired.Title: "Please enter your name: ",
	})

	queue := NewReviewQueue()
	scanner := NewStartupScanner(statusProvider, contentProvider)
	added := scanner.Scan([]*Instance{needsApproval, activeSession, inputRequired}, queue)

	if added < 1 {
		t.Errorf("expected at least 1 session added, got %d", added)
	}
	if _, ok := queue.Get(activeSession.Title); ok {
		t.Error("recently-active session with no prompt must not appear in queue (false positive)")
	}
	if _, ok := queue.Get(needsApproval.Title); !ok {
		t.Error("approval session must appear in queue")
	}
}

// TestStartupScanner_Scan_EmptyInstanceList verifies graceful handling of an empty list.
func TestStartupScanner_Scan_EmptyInstanceList(t *testing.T) {
	statusProvider := newFakeStatusProvider(map[string]InstanceStatusInfo{})
	contentProvider := newFakeContentProvider(map[string]string{})

	queue := NewReviewQueue()
	scanner := NewStartupScanner(statusProvider, contentProvider)
	added := scanner.Scan([]*Instance{}, queue)

	if added != 0 {
		t.Errorf("expected 0 added for empty list, got %d", added)
	}
}
