package app

import (
	"claude-squad/testutil"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestKeyRegistrationWorks - verify that key handling works and doesn't get blocked by PTY errors
func TestKeyRegistrationWorks(t *testing.T) {
	// Create app model
	appModel := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Minimal setup for testing key handling
	})

	// Add a test session (but don't worry if PTY fails - that's what we're testing)
	session := CreateTestSession(t, "test-key-handling")
	_ = appModel.list.AddInstance(session)
	appModel.list.SetSelectedInstance(0)

	// CRITICAL FIX: Ensure the Uncategorized category is expanded in tests
	appModel.list.ExpandCategory("Uncategorized")

	config := testutil.DefaultTUIConfig()
	tm := testutil.CreateTUITest(t, appModel, config)

	// Wait for initial render
	time.Sleep(200 * time.Millisecond)

	// Try some basic key operations that should work regardless of PTY state

	// Test 1: Try a navigation key (should not hang or crash)
	t.Logf("Testing navigation key...")
	tm.Send(tea.KeyMsg{Type: tea.KeyDown})
	time.Sleep(100 * time.Millisecond)

	// Test 2: Try help key (should not hang or crash)
	t.Logf("Testing help key...")
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	time.Sleep(100 * time.Millisecond)

	// Test 3: Try escape key (should not hang or crash)
	t.Logf("Testing escape key...")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(100 * time.Millisecond)

	// Test 4: Verify the app can handle window resize without crashing
	t.Logf("Testing window resize...")
	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	time.Sleep(100 * time.Millisecond)

	// Test 5: Try quit key sequence
	t.Logf("Testing quit key...")
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// The key test is that the test completes without hanging
	// If PTY errors were blocking the event loop, this test would timeout
	// Increased timeout accounts for state isolation setup/teardown during quit
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))

	t.Logf("✅ Key registration test completed - BubbleTea event loop working correctly")
}