package app

import (
	"claude-squad/testutil"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
)

// TestFixedConfirmationModalFlow - test confirmation modal with fixed output consumption pattern
func TestFixedConfirmationModalFlow(t *testing.T) {
	// Create app model with session that can be killed
	appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Minimal setup
	})

	// Add test session
	session := CreateTestSession(t, "test-session")
	_ = appModel.list.AddInstance(session)
	appModel.list.SetSelectedInstance(0)

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, appModel, config)

	// Wait for initial render and verify session is visible
	time.Sleep(100 * time.Millisecond)
	output1 := tm.Output()
	outputStr1 := testutil.ReadOutput(t, output1)
	if !strings.Contains(outputStr1, "test-session") {
		t.Errorf("Expected to see 'test-session' in initial render, got:\n%s", outputStr1)
		return
	}
	t.Logf("✅ Initial render OK - session visible")

	// Press 'D' to trigger kill confirmation
	tm.Type("D")

	// Wait for confirmation modal to appear using single-read approach
	testutil.WaitForOutputContainsAfterAction(t, tm, "Kill session 'test-session'?", 200*time.Millisecond)
	t.Logf("✅ Confirmation modal appeared")

	// Press 'n' to cancel
	tm.Type("n")

	// Wait for modal to close
	time.Sleep(100 * time.Millisecond)
	output2 := tm.Output()
	outputStr2 := testutil.ReadOutput(t, output2)

	// Check that modal is closed (no longer contains confirmation text)
	if strings.Contains(outputStr2, "Kill session 'test-session'?") {
		t.Errorf("Expected confirmation modal to be closed, but it's still visible:\n%s", outputStr2)
		return
	}

	// Check that session still exists (should be back to normal view)
	if !strings.Contains(outputStr2, "test-session") {
		t.Errorf("Expected session to still exist after canceling, got:\n%s", outputStr2)
		return
	}

	t.Logf("✅ Modal canceled successfully - session still exists")

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(1*time.Second))
}