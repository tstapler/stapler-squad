package app

import (
	"claude-squad/testutil"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestTeatestPerformance - investigate why teatest operations are slow
func TestTeatestPerformance(t *testing.T) {
	t.Logf("Testing teatest performance with minimal app model...")

	// Use the minimal test model from testutil
	minimalModel := testutil.CreateMinimalApp(t)

	// Test with fast config
	config := testutil.TUITestConfig{
		Width:   40,
		Height:  10,
		Timeout: 2 * time.Second,
	}

	start := time.Now()
	tm := testutil.CreateTUITest(t, minimalModel, config)
	t.Logf("⏱️  Teatest creation took: %v", time.Since(start))

	// Test simple key press
	start = time.Now()
	tm.Type("test")
	elapsed := time.Since(start)
	t.Logf("⏱️  Simple key press took: %v", elapsed)

	if elapsed > 100*time.Millisecond {
		t.Logf("⚠️  Key press took longer than expected: %v", elapsed)
	} else {
		t.Logf("✅ Key press performance is acceptable: %v", elapsed)
	}

	// Test quit
	start = time.Now()
	tm.Type("q")

	// Use a very short timeout for quit to avoid hanging
	tm.WaitFinished(t, teatest.WithFinalTimeout(1*time.Second))
	elapsed = time.Since(start)
	t.Logf("⏱️  Quit operation took: %v", elapsed)

	t.Logf("✅ Teatest performance test completed")
}

// TestTeatestWithClaudeSquadModel - test teatest with actual app model but minimal operations
func TestTeatestWithClaudeSquadModel(t *testing.T) {
	t.Logf("Testing teatest performance with Claude Squad app model...")

	// Create minimal app model without expensive initialization
	appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Minimal mock setup
	})

	config := testutil.TUITestConfig{
		Width:   40,
		Height:  10,
		Timeout: 2 * time.Second,
	}

	start := time.Now()
	tm := testutil.CreateTUITest(t, appModel, config)
	elapsed := time.Since(start)
	t.Logf("⏱️  Claude Squad teatest creation took: %v", elapsed)

	if elapsed > 1*time.Second {
		t.Logf("⚠️  App model creation took longer than expected: %v", elapsed)
	} else {
		t.Logf("✅ App model teatest creation performance is acceptable: %v", elapsed)
	}

	// Test one simple operation
	start = time.Now()
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	elapsed = time.Since(start)
	t.Logf("⏱️  Navigation key took: %v", elapsed)

	// Don't test quit to avoid timeout issues - this is about creation performance
	t.Logf("✅ Claude Squad teatest performance test completed")
}