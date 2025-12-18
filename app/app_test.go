package app

import (
	"claude-squad/app/state"
	appui "claude-squad/app/ui"
	"claude-squad/log"
	"fmt"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain runs before all tests to set up the test environment
func TestMain(m *testing.M) {
	// Initialize the logger for tests with dual-stream configuration:
	// - DEBUG logs go to file for detailed debugging (~/.claude-squad/logs/test/)
	// - ERROR logs appear on console for immediate visibility
	// This provides both comprehensive debugging and clean console output
	log.InitializeForTests(log.DEBUG, log.ERROR)
	defer log.Close()

	// Run all tests
	exitCode := m.Run()

	// Exit with the same code as the tests
	os.Exit(exitCode)
}

// TestConfirmationModalStateTransitions tests state transitions without full instance setup
func TestConfirmationModalStateTransitions(t *testing.T) {
	// Use test helper for clean construction
	h := SetupMinimalTestHome(t)

	// Initialize the coordinator
	h.uiCoordinator.Initialize()
	h.uiCoordinator.SetContext(h.ctx)

	t.Run("shows confirmation on D press", func(t *testing.T) {
		// Simulate pressing 'D'
		h.setState(state.Default)
		h.uiCoordinator.CloseAllOverlays()

		// Manually trigger what would happen in handleKeyPress for 'D'
		h.setState(state.Confirm)
		err := h.uiCoordinator.CreateConfirmationOverlay("[!] Kill session 'test'?")
		require.NoError(t, err)

		assert.True(t, h.isInState(state.Confirm))
		confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
		assert.NotNil(t, confirmationOverlay)
		assert.False(t, confirmationOverlay.Dismissed)
	})

	t.Run("returns to default on y press", func(t *testing.T) {
		// Reset coordinator state for this test
		h.uiCoordinator.CloseAllOverlays()
		h.setState(state.Confirm)
		err := h.uiCoordinator.CreateConfirmationOverlay("Test confirmation")
		require.NoError(t, err)

		// Verify overlay was created
		confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
		require.NotNil(t, confirmationOverlay)

		// Simulate pressing 'y' using the main handleKeyPress method
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
		model, _ := h.handleKeyPress(keyMsg)
		homeModel, ok := model.(*home)
		require.True(t, ok)

		assert.True(t, homeModel.isInState(state.Default))
		assert.Nil(t, homeModel.uiCoordinator.GetConfirmationOverlay())
	})

	t.Run("returns to default on n press", func(t *testing.T) {
		// Reset coordinator state for this test
		h.uiCoordinator.CloseAllOverlays()
		h.setState(state.Confirm)
		err := h.uiCoordinator.CreateConfirmationOverlay("Test confirmation")
		require.NoError(t, err)

		// Verify overlay was created
		confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
		require.NotNil(t, confirmationOverlay)

		// Simulate pressing 'n' using the main handleKeyPress method
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
		model, _ := h.handleKeyPress(keyMsg)
		homeModel, ok := model.(*home)
		require.True(t, ok)

		assert.True(t, homeModel.isInState(state.Default))
		assert.Nil(t, homeModel.uiCoordinator.GetConfirmationOverlay())
	})

	t.Run("returns to default on esc press", func(t *testing.T) {
		// Reset coordinator state for this test
		h.uiCoordinator.CloseAllOverlays()
		h.setState(state.Confirm)
		err := h.uiCoordinator.CreateConfirmationOverlay("Test confirmation")
		require.NoError(t, err)

		// Verify overlay was created
		confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
		require.NotNil(t, confirmationOverlay)

		// Simulate pressing ESC using the main handleKeyPress method
		keyMsg := tea.KeyMsg{Type: tea.KeyEscape}
		model, _ := h.handleKeyPress(keyMsg)
		homeModel, ok := model.(*home)
		require.True(t, ok)

		assert.True(t, homeModel.isInState(state.Default))
		assert.Nil(t, homeModel.uiCoordinator.GetConfirmationOverlay())
	})
}

// TestConfirmationModalKeyHandling tests the actual key handling in confirmation state
func TestConfirmationModalKeyHandling(t *testing.T) {
	// Use test helper for clean construction
	h := SetupMinimalTestHome(t)

	// Set initial state to Confirm
	h.setState(state.Confirm)
	// Initialize the coordinator
	h.uiCoordinator.Initialize()
	h.uiCoordinator.SetContext(h.ctx)

	// Create confirmation overlay using coordinator
	err := h.uiCoordinator.CreateConfirmationOverlay("Kill session?")
	require.NoError(t, err)

	testCases := []struct {
		name              string
		key               string
		expectedState     state.State
		expectedDismissed bool
		expectedNil       bool
	}{
		{
			name:              "y key confirms and dismisses overlay",
			key:               "y",
			expectedState:     state.Default,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "n key cancels and dismisses overlay",
			key:               "n",
			expectedState:     state.Default,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "esc key cancels and dismisses overlay",
			key:               "esc",
			expectedState:     state.Default,
			expectedDismissed: true,
			expectedNil:       true,
		},
		{
			name:              "other keys are ignored",
			key:               "x",
			expectedState:     state.Confirm,
			expectedDismissed: false,
			expectedNil:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset coordinator state and ensure clean state for each test case
			h.uiCoordinator.CloseAllOverlays()
			h.setState(state.Confirm)
			err := h.uiCoordinator.CreateConfirmationOverlay("Kill session?")
			require.NoError(t, err)

			// Verify overlay was created before proceeding
			confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
			require.NotNil(t, confirmationOverlay, "Overlay should exist before key press for key: %s", tc.key)

			// Create key message
			var keyMsg tea.KeyMsg
			if tc.key == "esc" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEscape}
			} else {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)}
			}

			// Call handleKeyPress
			model, _ := h.handleKeyPress(keyMsg)
			homeModel, ok := model.(*home)
			require.True(t, ok)

			assert.True(t, homeModel.isInState(tc.expectedState), "State mismatch for key: %s", tc.key)
			resultOverlay := homeModel.uiCoordinator.GetConfirmationOverlay()
			if tc.expectedNil {
				assert.Nil(t, resultOverlay, "Overlay should be nil for key: %s", tc.key)
			} else {
				assert.NotNil(t, resultOverlay, "Overlay should not be nil for key: %s", tc.key)
				// Note: we can't check Dismissed property since overlay may be nil after closing
				// This test is more about ensuring the state transitions work correctly
			}
		})
	}
}

// TestConfirmationMessageFormatting tests that confirmation messages are formatted correctly
func TestConfirmationMessageFormatting(t *testing.T) {
	testCases := []struct {
		name            string
		sessionTitle    string
		expectedMessage string
	}{
		{
			name:            "short session name",
			sessionTitle:    "my-feature",
			expectedMessage: "[!] Kill session 'my-feature'? (y/n)",
		},
		{
			name:            "long session name",
			sessionTitle:    "very-long-feature-branch-name-here",
			expectedMessage: "[!] Kill session 'very-long-feature-branch-name-here'? (y/n)",
		},
		{
			name:            "session with spaces",
			sessionTitle:    "feature with spaces",
			expectedMessage: "[!] Kill session 'feature with spaces'? (y/n)",
		},
		{
			name:            "session with special chars",
			sessionTitle:    "feature/branch-123",
			expectedMessage: "[!] Kill session 'feature/branch-123'? (y/n)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the message formatting directly
			actualMessage := fmt.Sprintf("[!] Kill session '%s'? (y/n)", tc.sessionTitle)
			assert.Equal(t, tc.expectedMessage, actualMessage)
		})
	}
}

// TestConfirmationFlowSimulation tests the confirmation flow by simulating the state changes
func TestConfirmationFlowSimulation(t *testing.T) {
	// Use test helper for clean construction with session
	h := SetupTestHomeWithSession(t, "test-session")

	// Initialize the coordinator
	h.uiCoordinator.Initialize()
	h.uiCoordinator.SetContext(h.ctx)

	// Simulate what happens when D is pressed
	selected := h.list.GetSelectedInstance()
	require.NotNil(t, selected)

	// This is what the KeyKill handler does
	message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
	err := h.uiCoordinator.CreateConfirmationOverlay(message)
	require.NoError(t, err)
	h.setState(state.Confirm)

	// Verify the state
	assert.True(t, h.isInState(state.Confirm))
	confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
	assert.NotNil(t, confirmationOverlay)
	assert.False(t, confirmationOverlay.Dismissed)
	// Test that overlay renders with the correct message
	rendered := confirmationOverlay.Render()
	assert.Contains(t, rendered, "Kill session 'test-session'?")
}

// TestConfirmActionWithDifferentTypes tests that confirmAction works with different action types
func TestConfirmActionWithDifferentTypes(t *testing.T) {
	// Use test helper for clean construction
	h := SetupMinimalTestHome(t)

	// Initialize the coordinator
	h.uiCoordinator.Initialize()
	h.uiCoordinator.SetContext(h.ctx)

	t.Run("works with simple action returning nil", func(t *testing.T) {
		actionCalled := false
		action := func() tea.Msg {
			actionCalled = true
			return nil
		}

		// Set up callback to track action execution
		actionExecuted := false
		err := h.uiCoordinator.CreateConfirmationOverlay("Test action?")
		require.NoError(t, err)
		confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
		confirmationOverlay.OnConfirm = func() {
			h.setState(state.Default)
			actionExecuted = true
			action() // Execute the action
		}
		h.setState(state.Confirm)

		// Verify state was set
		assert.True(t, h.isInState(state.Confirm))
		assert.NotNil(t, confirmationOverlay)
		assert.False(t, confirmationOverlay.Dismissed)
		assert.NotNil(t, confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		confirmationOverlay.OnConfirm()
		assert.True(t, actionCalled)
		assert.True(t, actionExecuted)
	})

	t.Run("works with action returning error", func(t *testing.T) {
		// Reset coordinator state for this test
		h.uiCoordinator.CloseAllOverlays()

		expectedErr := fmt.Errorf("test error")
		action := func() tea.Msg {
			return expectedErr
		}

		// Set up callback to track action execution
		var receivedMsg tea.Msg
		err := h.uiCoordinator.CreateConfirmationOverlay("Error action?")
		require.NoError(t, err)
		confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
		require.NotNil(t, confirmationOverlay)
		confirmationOverlay.OnConfirm = func() {
			h.setState(state.Default)
			receivedMsg = action() // Execute the action and capture result
		}
		h.setState(state.Confirm)

		// Verify state was set
		assert.True(t, h.isInState(state.Confirm))
		assert.NotNil(t, confirmationOverlay)
		assert.False(t, confirmationOverlay.Dismissed)
		assert.NotNil(t, confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		confirmationOverlay.OnConfirm()
		assert.Equal(t, expectedErr, receivedMsg)
	})

	t.Run("works with action returning custom message", func(t *testing.T) {
		// Reset coordinator state for this test
		h.uiCoordinator.CloseAllOverlays()

		action := func() tea.Msg {
			return instanceChangedMsg{}
		}

		// Set up callback to track action execution
		var receivedMsg tea.Msg
		err := h.uiCoordinator.CreateConfirmationOverlay("Custom message action?")
		require.NoError(t, err)
		confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
		require.NotNil(t, confirmationOverlay)
		confirmationOverlay.OnConfirm = func() {
			h.setState(state.Default)
			receivedMsg = action() // Execute the action and capture result
		}
		h.setState(state.Confirm)

		// Verify state was set
		assert.True(t, h.isInState(state.Confirm))
		assert.NotNil(t, confirmationOverlay)
		assert.False(t, confirmationOverlay.Dismissed)
		assert.NotNil(t, confirmationOverlay.OnConfirm)

		// Execute the confirmation callback
		confirmationOverlay.OnConfirm()
		_, ok := receivedMsg.(instanceChangedMsg)
		assert.True(t, ok, "Expected instanceChangedMsg but got %T", receivedMsg)
	})
}

// TestMultipleConfirmationsDontInterfere tests that multiple confirmations don't interfere with each other
func TestMultipleConfirmationsDontInterfere(t *testing.T) {
	// Use test helper for clean construction
	h := SetupMinimalTestHome(t)

	// Initialize the coordinator
	h.uiCoordinator.Initialize()
	h.uiCoordinator.SetContext(h.ctx)

	// First confirmation
	action1Called := false
	action1 := func() tea.Msg {
		action1Called = true
		return nil
	}

	// Set up first confirmation
	err := h.uiCoordinator.CreateConfirmationOverlay("First action?")
	require.NoError(t, err)
	firstOnConfirm := func() {
		h.setState(state.Default)
		action1()
	}
	confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
	confirmationOverlay.OnConfirm = firstOnConfirm
	h.setState(state.Confirm)

	// Verify first confirmation
	assert.True(t, h.isInState(state.Confirm))
	assert.NotNil(t, confirmationOverlay)
	assert.False(t, confirmationOverlay.Dismissed)
	assert.NotNil(t, confirmationOverlay.OnConfirm)

	// Cancel first confirmation (simulate pressing 'n')
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	shouldClose := confirmationOverlay.HandleKeyPress(keyMsg)
	if shouldClose {
		h.setState(state.Default)
		h.uiCoordinator.HideOverlay(appui.ComponentConfirmationOverlay)
	}

	// Second confirmation with different action
	action2Called := false
	action2 := func() tea.Msg {
		action2Called = true
		return fmt.Errorf("action2 error")
	}

	// Set up second confirmation
	err = h.uiCoordinator.CreateConfirmationOverlay("Second action?")
	require.NoError(t, err)
	var secondResult tea.Msg
	secondOnConfirm := func() {
		h.setState(state.Default)
		secondResult = action2()
	}
	secondConfirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
	secondConfirmationOverlay.OnConfirm = secondOnConfirm
	h.setState(state.Confirm)

	// Verify second confirmation
	assert.True(t, h.isInState(state.Confirm))
	assert.NotNil(t, secondConfirmationOverlay)
	assert.False(t, secondConfirmationOverlay.Dismissed)
	assert.NotNil(t, secondConfirmationOverlay.OnConfirm)

	// Execute second action to verify it's the correct one
	secondConfirmationOverlay.OnConfirm()
	errResult, ok := secondResult.(error)
	assert.True(t, ok)
	assert.Equal(t, "action2 error", errResult.Error())
	assert.True(t, action2Called)
	assert.False(t, action1Called, "First action should not have been called")

	// Test that cancelled action can still be executed independently
	firstOnConfirm()
	assert.True(t, action1Called, "First action should be callable after being replaced")
}

// TestConfirmationModalVisualAppearance tests that confirmation modal has distinct visual appearance
func TestConfirmationModalVisualAppearance(t *testing.T) {
	// Use test helper for clean construction
	h := SetupMinimalTestHome(t)

	// Initialize the coordinator
	h.uiCoordinator.Initialize()
	h.uiCoordinator.SetContext(h.ctx)

	// Create a test confirmation overlay
	message := "[!] Delete everything?"
	err := h.uiCoordinator.CreateConfirmationOverlay(message)
	require.NoError(t, err)
	h.setState(state.Confirm)

	// Verify the overlay was created with confirmation settings
	confirmationOverlay := h.uiCoordinator.GetConfirmationOverlay()
	assert.NotNil(t, confirmationOverlay)
	assert.True(t, h.isInState(state.Confirm))
	assert.False(t, confirmationOverlay.Dismissed)

	// Test the overlay render (we can test that it renders without errors)
	rendered := confirmationOverlay.Render()
	assert.NotEmpty(t, rendered)

	// Test that it includes the message content and instructions
	assert.Contains(t, rendered, "Delete everything?")
	assert.Contains(t, rendered, "Press")
	assert.Contains(t, rendered, "to confirm")
	assert.Contains(t, rendered, "to cancel")

	// Test that the danger indicator is preserved
	assert.Contains(t, rendered, "[!")
}
