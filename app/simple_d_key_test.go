package app

import (
	"claude-squad/testutil"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
)

// TestSimpleDKeyHandling - test if 'D' key is handled properly
func TestSimpleDKeyHandling(t *testing.T) {
	// Create the same app as the failing test
	appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Minimal setup
	})

	// Add test session
	session := CreateTestSession(t, "test-session")
	_ = appModel.list.AddInstance(session)
	appModel.list.SetSelectedInstance(0)

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, appModel, config)

	// Wait for initial render (don't consume output)
	time.Sleep(100 * time.Millisecond)

	// Press 'D' to trigger kill confirmation
	tm.Type("D")

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Get output AFTER the D keypress to see what happened
	output := tm.Output()
	outputStr := testutil.ReadOutput(t, output)

	t.Logf("Output after 'D' keypress (length: %d):\n%q", len(outputStr), outputStr)
	t.Logf("Contains 'Kill session': %v", strings.Contains(outputStr, "Kill session"))
	t.Logf("Contains confirmation: %v", strings.Contains(outputStr, "Kill session 'test-session'?"))

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(1*time.Second))
}