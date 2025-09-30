package app

import (
	"claude-squad/testutil"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
)

// TestDebugAppRendering - temporary debug function to see what's being rendered
func TestDebugAppRendering(t *testing.T) {
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

	// Let it render for a moment
	time.Sleep(200 * time.Millisecond)

	// Get and print the raw output
	output := tm.Output()
	outputStr := testutil.ReadOutput(t, output)

	fmt.Printf("=== DEBUG: Raw output length: %d ===\n", len(outputStr))
	fmt.Printf("=== DEBUG: Raw output ===\n%q\n", outputStr)
	fmt.Printf("=== DEBUG: Visible output ===\n%s\n", outputStr)
	fmt.Printf("=== DEBUG: Contains 'test-session': %v ===\n", strings.Contains(outputStr, "test-session"))

	// Also check how many instances are in the list
	fmt.Printf("=== DEBUG: List instance count: %d ===\n", appModel.getInstanceCount())
	fmt.Printf("=== DEBUG: Selected index: %d ===\n", appModel.getLastSelectedIndex())

	// Check if the session was actually added
	allInstances := appModel.getAllInstances()
	fmt.Printf("=== DEBUG: All instances: %d ===\n", len(allInstances))
	for i, inst := range allInstances {
		fmt.Printf("=== DEBUG: Instance %d: %s ===\n", i, inst.Title)
	}

	// Clean exit
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(1*time.Second))
}