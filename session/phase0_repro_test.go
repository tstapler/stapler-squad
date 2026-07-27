package session

// phase0_repro_test.go — Phase 0 runtime-evidence experiment for backlog item
// 04089969-0f19-499c-be34-2e8bcfc4f13e ("phantom repeated '1' keystroke").
//
// This is a diagnostic repro, not a permanent regression test. It exercises the
// REAL, unmodified runSessionDriverWithPrompt goroutine (session_driver.go) against
// a fake ProcessManager whose CapturePaneContent() always returns the same
// trust-folder dialog text — simulating the "session not started or paused"
// flapping condition from the ticket, where the driver's periodic Preview() check
// keeps observing stale/unchanging dialog content. It counts real SendKeys("1\n")
// invocations made by the actual driver code over several ticks.
//
// See project_plans/phantom-keystroke-replay/research/phase0-findings.md for the
// analysis of this test's output.

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// stuckDialogProcessManager implements ProcessManager. CapturePaneContent always
// returns the same trust-folder dialog text (as if the pane is frozen/stale during
// a flapping episode). SendKeys calls are counted.
type stuckDialogProcessManager struct {
	sendKeysCount atomic.Int32
	dialogText    string
}

const trustDialogText = `Quick safety check: Is this a project you created or one you trust?
❯ 1. Yes, I trust this folder
  2. No, exit`

func (m *stuckDialogProcessManager) Start(dir string) error              { return nil }
func (m *stuckDialogProcessManager) RestoreWithWorkDir(dir string) error  { return nil }
func (m *stuckDialogProcessManager) Close() error                        { return nil }
func (m *stuckDialogProcessManager) IsAlive() bool                       { return true }
func (m *stuckDialogProcessManager) GetSessionIdentifier() string        { return "phase0-repro" }
func (m *stuckDialogProcessManager) HasSession() bool                    { return true }
func (m *stuckDialogProcessManager) GetCurrentWorkingDirectory() (string, error) {
	return "/tmp", nil
}
func (m *stuckDialogProcessManager) GetPTY() (*os.File, error) { return nil, nil }
func (m *stuckDialogProcessManager) SendKeys(keys string) (int, error) {
	m.sendKeysCount.Add(1)
	return len(keys), nil
}
func (m *stuckDialogProcessManager) TapEnter() error                     { return nil }
func (m *stuckDialogProcessManager) SendPromptWithEnter(p string) error  { return nil }
func (m *stuckDialogProcessManager) SendInputViaControlMode(ctx context.Context, data []byte) error {
	return nil
}
func (m *stuckDialogProcessManager) CapturePaneContent() (string, error) {
	// Always returns the same content: simulates a stuck/stale pane read during
	// a flapping episode where the underlying tmux session never advances.
	return m.dialogText, nil
}
func (m *stuckDialogProcessManager) CapturePaneContentRaw() (string, error) {
	return m.dialogText, nil
}
func (m *stuckDialogProcessManager) CapturePaneContentWithOptions(startLine, endLine string) (string, error) {
	return m.dialogText, nil
}
func (m *stuckDialogProcessManager) CaptureViewport(lines int) (string, error) {
	return m.dialogText, nil
}
func (m *stuckDialogProcessManager) GetCursorPosition() (int, int, error) { return 0, 0, nil }
func (m *stuckDialogProcessManager) GetPaneDimensions() (int, int, error) { return 80, 24, nil }
func (m *stuckDialogProcessManager) SetWindowSize(cols, rows int) error   { return nil }
func (m *stuckDialogProcessManager) SetDetachedSize(w, h int, title string) error {
	return nil
}
func (m *stuckDialogProcessManager) RefreshClient() error         { return nil }
func (m *stuckDialogProcessManager) GetPanePID() (int32, error)   { return 0, nil }
func (m *stuckDialogProcessManager) HasUpdated() (bool, bool, string) {
	return false, false, m.dialogText
}
func (m *stuckDialogProcessManager) FilterBanners(content string) (string, int) {
	return content, 0
}
func (m *stuckDialogProcessManager) HasMeaningfulContent(content string) bool { return true }
func (m *stuckDialogProcessManager) StartControlMode() error                 { return nil }
func (m *stuckDialogProcessManager) StopControlMode() error                  { return nil }
func (m *stuckDialogProcessManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	return "", nil
}
func (m *stuckDialogProcessManager) UnsubscribeFromControlModeUpdates(id string) {}
func (m *stuckDialogProcessManager) Attach() (chan struct{}, error)             { return nil, nil }
func (m *stuckDialogProcessManager) DetachSafely() error                       { return nil }
func (m *stuckDialogProcessManager) SetOnExitCallback(fn func(string))         {}
func (m *stuckDialogProcessManager) ResetExitOnce()                            {}

// TestPhase0_StuckDialogCausesUnboundedRepeatedSendKeys is the Phase 0 go/no-go
// runtime evidence gate for AC1. It runs the REAL runSessionDriverWithPrompt
// goroutine (unmodified) for several driverPollInterval ticks against a fake
// session whose visible content never changes (simulating the flapping
// condition where the trust-folder dialog's SendKeys("1\n") answer never
// actually lands because the underlying tmux session is not started/paused).
//
// If this observes SendKeys("1\n") firing more than once, it is direct runtime
// proof — not source-level inference — that session_driver.go's dialog-answer
// loop has no de-duplication/backoff guard and will resend the same keystroke
// indefinitely while a session is stuck, exactly matching the reported symptom.
func TestPhase0_StuckDialogCausesUnboundedRepeatedSendKeys(t *testing.T) {
	fakePM := &stuckDialogProcessManager{dialogText: trustDialogText}

	inst := &Instance{
		Title:          "phase0-repro",
		Status:         Ready, // app-level status stays "Ready" throughout — mirrors the
		started:        true,  // ticket: capture-pane/control-mode fails transiently while
		processManager: fakePM, // Instance.Status itself never flips to Stopped/Paused.
	}

	var retried atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSessionDriverWithPrompt(inst, "/tmp", driverInitialPrompt, &retried)
	}()

	// Let several real poll ticks elapse. driverPollInterval is 2s in production;
	// we can't change that constant from the test, so wait long enough to observe
	// at least 3 ticks of real goroutine execution.
	time.Sleep(driverPollInterval*3 + 500*time.Millisecond)

	count := fakePM.sendKeysCount.Load()
	t.Logf("Phase 0 evidence: real runSessionDriverWithPrompt goroutine called SendKeys(%q) %d times over ~%s while Preview() returned unchanging trust-dialog content",
		"1\\n", count, (driverPollInterval*3 + 500*time.Millisecond))

	if count < 2 {
		t.Fatalf("Phase 0 hypothesis NOT confirmed: expected SendKeys(\"1\\n\") to be called repeatedly (>=2) when the dialog never visibly clears; got %d call(s). "+
			"This would mean session_driver.go's isStartupDialog/SendKeys loop is NOT the duplication mechanism.", count)
	}

	// Cleanup: mark instance stopped-equivalent via total-timeout is too slow for a
	// test; the goroutine will keep running against driverTotalTimeout (25m) in the
	// background otherwise. Force it to observe Paused so it exits cleanly.
	inst.Status = Paused
	select {
	case <-done:
	case <-time.After(driverPollInterval + time.Second):
		t.Log("driver goroutine did not exit promptly after Status=Paused (leak, not fatal to this repro)")
	}
}
