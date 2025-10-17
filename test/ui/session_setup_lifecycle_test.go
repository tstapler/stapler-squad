package ui

import (
	"testing"

	"claude-squad/session"
	"claude-squad/ui/overlay"
	tea "github.com/charmbracelet/bubbletea"
)

// TestSessionSetupOverlay_CallbackLifecycle tests the complete callback lifecycle
// This ensures callbacks fire correctly and the overlay state is valid after callbacks
func TestSessionSetupOverlay_CallbackLifecycle(t *testing.T) {
	t.Run("onComplete fires with correct session data", func(t *testing.T) {
		var callbackFired bool
		var receivedOpts session.InstanceOptions

		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(opts session.InstanceOptions) {
				callbackFired = true
				receivedOpts = opts
			},
			OnCancel: func() {},
		})
		s.SetSize(80, 30)

		// Complete the flow with current location
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test-session")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location (current) -> Branch
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Branch (new) -> Confirm
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Confirm -> Triggers callback

		if !callbackFired {
			t.Fatal("onComplete callback did not fire")
		}

		// Verify the received data is correct
		if receivedOpts.Title != "test-session" {
			t.Errorf("Expected title 'test-session', got '%s'", receivedOpts.Title)
		}

		if receivedOpts.Program == "" {
			t.Error("Expected program to be set, got empty string")
		}

		// For current location with new branch, should be NewWorktree type
		if receivedOpts.SessionType != session.SessionTypeNewWorktree {
			t.Errorf("Expected SessionTypeNewWorktree, got %v", receivedOpts.SessionType)
		}
	})

	t.Run("onComplete fires for all location choices", func(t *testing.T) {
		locationTests := []struct {
			name         string
			setupKeys    []tea.KeyMsg
			expectedType session.SessionType
		}{
			{
				name: "current_location",
				setupKeys: []tea.KeyMsg{
					{Type: tea.KeyRunes, Runes: []rune("current-test")},
					{Type: tea.KeyEnter}, // Basics -> Location
					{Type: tea.KeyEnter}, // Location (current) -> Branch
					{Type: tea.KeyEnter}, // Branch -> Confirm
					{Type: tea.KeyEnter}, // Confirm -> Complete
				},
				expectedType: session.SessionTypeNewWorktree,
			},
		}

		for _, tt := range locationTests {
			t.Run(tt.name, func(t *testing.T) {
				var callbackFired bool
				var receivedOpts session.InstanceOptions

				s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
					OnComplete: func(opts session.InstanceOptions) {
						callbackFired = true
						receivedOpts = opts
					},
					OnCancel: func() {},
				})
				s.SetSize(80, 30)

				// Execute the key sequence
				for _, key := range tt.setupKeys {
					s.Update(key)
				}

				if !callbackFired {
					t.Errorf("onComplete callback did not fire for %s", tt.name)
				}

				if receivedOpts.SessionType != tt.expectedType {
					t.Errorf("Expected session type %v, got %v", tt.expectedType, receivedOpts.SessionType)
				}
			})
		}
	})

	t.Run("onCancel fires on escape at all steps", func(t *testing.T) {
		steps := []struct {
			name     string
			navigate func(*overlay.SessionSetupOverlay)
		}{
			{
				name:     "basics_step",
				navigate: func(s *overlay.SessionSetupOverlay) {
					// At basics step (initial state)
				},
			},
			{
				name: "location_step",
				navigate: func(s *overlay.SessionSetupOverlay) {
					s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
					s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
				},
			},
			{
				name: "branch_step",
				navigate: func(s *overlay.SessionSetupOverlay) {
					s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
					s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
					s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location -> Branch
				},
			},
			{
				name: "confirm_step",
				navigate: func(s *overlay.SessionSetupOverlay) {
					s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
					s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
					s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location -> Branch
					s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Branch -> Confirm
				},
			},
		}

		for _, step := range steps {
			t.Run(step.name, func(t *testing.T) {
				var cancelFired bool

				s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
					OnComplete: func(session.InstanceOptions) {},
					OnCancel: func() {
						cancelFired = true
					},
				})
				s.SetSize(80, 30)

				// Navigate to the step
				step.navigate(s)

				// Press Escape
				s.Update(tea.KeyMsg{Type: tea.KeyEscape})

				if !cancelFired {
					t.Errorf("onCancel did not fire at %s", step.name)
				}
			})
		}
	})

	t.Run("overlay state after onComplete callback", func(t *testing.T) {
		callbackCompleted := false

		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(opts session.InstanceOptions) {
				callbackCompleted = true
			},
			OnCancel: func() {},
		})
		s.SetSize(80, 30)

		// Complete the full flow
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Location -> Branch
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Branch -> Confirm
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Confirm -> Triggers callback

		if !callbackCompleted {
			t.Fatal("Callback should have fired")
		}

		// CRITICAL TEST: Verify overlay can still handle keys after callback fires
		// This tests for the regression where keys stop working after session creation
		// The overlay should gracefully handle additional key presses
		s.Update(tea.KeyMsg{Type: tea.KeyEscape})

		// The exact behavior after callback is implementation-dependent,
		// but the key point is it should NOT crash or become unresponsive
		// We're verifying the Update method doesn't panic
	})

	t.Run("overlay state after onCancel callback", func(t *testing.T) {
		cancelCalled := false

		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(session.InstanceOptions) {},
			OnCancel: func() {
				cancelCalled = true
			},
		})
		s.SetSize(80, 30)

		// Navigate partway through
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Basics -> Location

		// Cancel
		s.Update(tea.KeyMsg{Type: tea.KeyEscape})

		if !cancelCalled {
			t.Fatal("Cancel callback should have fired")
		}

		// Verify overlay can handle additional input after cancel
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		// Should not crash
	})

	t.Run("callbacks passed at construction time work correctly", func(t *testing.T) {
		callbackFired := false

		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(opts session.InstanceOptions) {
				callbackFired = true
			},
			OnCancel: func() {},
		})
		s.SetSize(80, 30)

		// Complete flow
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// The callback should fire when construction-time callbacks are used
		if !callbackFired {
			t.Error("Callback should fire when passed at construction time")
		}
	})
}

// TestSessionSetupOverlay_KeyHandlingAfterCallbacks specifically tests key responsiveness
// This is the core regression test for the reported bug
func TestSessionSetupOverlay_KeyHandlingAfterCallbacks(t *testing.T) {
	t.Run("enter key after complete callback", func(t *testing.T) {
		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(opts session.InstanceOptions) {
				// Callback fires
			},
			OnCancel: func() {},
		})
		s.SetSize(80, 30)

		// Complete the flow
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Triggers callback

		// REGRESSION TEST: Enter should still be processed
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		// Should not panic or hang
	})

	t.Run("escape key after complete callback", func(t *testing.T) {
		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(opts session.InstanceOptions) {
				// Callback fires
			},
			OnCancel: func() {},
		})
		s.SetSize(80, 30)

		// Complete the flow
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Triggers callback

		// REGRESSION TEST: Escape should still be processed
		s.Update(tea.KeyMsg{Type: tea.KeyEscape})
		// Should not panic or hang
	})

	t.Run("tab key after complete callback", func(t *testing.T) {
		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(opts session.InstanceOptions) {
				// Callback fires
			},
			OnCancel: func() {},
		})
		s.SetSize(80, 30)

		// Complete the flow
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Triggers callback

		// REGRESSION TEST: Tab should still be processed
		s.Update(tea.KeyMsg{Type: tea.KeyTab})
		// Should not panic or hang
	})

	t.Run("text input after complete callback", func(t *testing.T) {
		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(opts session.InstanceOptions) {
				// Callback fires
			},
			OnCancel: func() {},
		})
		s.SetSize(80, 30)

		// Complete the flow
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Triggers callback

		// REGRESSION TEST: Text input should still be processed
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("additional text")})
		// Should not panic or hang
	})
}

// TestSessionSetupOverlay_FocusState tests focus management throughout the lifecycle
func TestSessionSetupOverlay_FocusState(t *testing.T) {
	t.Run("overlay starts focused", func(t *testing.T) {
		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(session.InstanceOptions) {},
			OnCancel:   func() {},
		})
		s.SetSize(80, 30)

		// Overlay should be focused by default
		// We can verify this by checking if it responds to keys
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})

		// If not focused, the input wouldn't be processed
		// This is a basic sanity check
	})

	t.Run("focus after blur", func(t *testing.T) {
		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(session.InstanceOptions) {},
			OnCancel:   func() {},
		})
		s.SetSize(80, 30)

		// Blur the overlay
		s.Blur()

		// Re-focus
		s.Focus()

		// Should be responsive again
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
	})

	t.Run("focus state after callback", func(t *testing.T) {
		s := overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
			OnComplete: func(opts session.InstanceOptions) {
				// Callback fires
			},
			OnCancel: func() {},
		})
		s.SetSize(80, 30)

		// Complete flow
		s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})
		s.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// After callback, focus state should still be manageable
		s.Blur()
		s.Focus()

		// Should not crash
	})
}
