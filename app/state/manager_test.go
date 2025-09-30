package state

import (
	"fmt"
	"testing"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	if manager == nil {
		t.Fatal("NewManager should not return nil")
	}

	if manager.Current() != Default {
		t.Errorf("NewManager should initialize to Default state, got %v", manager.Current())
	}
}

func TestManagerCurrentState(t *testing.T) {
	manager := NewManager()

	// Test initial state
	if manager.Current() != Default {
		t.Errorf("Initial state should be Default, got %v", manager.Current())
	}

	// Test state after transition
	err := manager.Transition(New)
	if err != nil {
		t.Fatalf("Transition to New should succeed: %v", err)
	}

	if manager.Current() != New {
		t.Errorf("Current state should be New after transition, got %v", manager.Current())
	}
}

func TestManagerTransition(t *testing.T) {

	tests := []struct {
		name        string
		from        State
		to          State
		expectError bool
		setup       func(Manager)
	}{
		{
			name:        "Default to New (valid)",
			from:        Default,
			to:          New,
			expectError: false,
			setup:       func(m Manager) {},
		},
		{
			name:        "Default to Help (valid)",
			from:        Default,
			to:          Help,
			expectError: false,
			setup:       func(m Manager) {},
		},
		{
			name:        "New to Default (valid)",
			from:        New,
			to:          Default,
			expectError: false,
			setup:       func(m Manager) { m.Transition(New) },
		},
		{
			name:        "Any state to Default (valid)",
			from:        CreatingSession,
			to:          Default,
			expectError: false,
			setup:       func(m Manager) { m.Transition(New); m.Transition(CreatingSession) },
		},
		{
			name:        "Invalid state transition",
			from:        Default,
			to:          State(999), // Invalid state
			expectError: true,
			setup:       func(m Manager) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager()
			tt.setup(manager)

			err := manager.Transition(tt.to)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for transition from %v to %v, but got none", tt.from, tt.to)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for transition from %v to %v: %v", tt.from, tt.to, err)
			}

			if !tt.expectError && manager.Current() != tt.to {
				t.Errorf("Expected state %v after transition, got %v", tt.to, manager.Current())
			}
		})
	}
}

func TestManagerCanTransition(t *testing.T) {

	tests := []struct {
		name     string
		from     State
		to       State
		expected bool
		setup    func(Manager)
	}{
		{
			name:     "Default to New",
			from:     Default,
			to:       New,
			expected: true,
			setup:    func(m Manager) {},
		},
		{
			name:     "Default to Help",
			from:     Default,
			to:       Help,
			expected: true,
			setup:    func(m Manager) {},
		},
		{
			name:     "Any state to Default",
			from:     New,
			to:       Default,
			expected: true,
			setup:    func(m Manager) { m.Transition(New) },
		},
		{
			name:     "Overlay to CreatingSession",
			from:     New,
			to:       CreatingSession,
			expected: true,
			setup:    func(m Manager) { m.Transition(New) },
		},
		{
			name:     "Invalid target state",
			from:     Default,
			to:       State(999),
			expected: false,
			setup:    func(m Manager) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager()
			tt.setup(manager)

			result := manager.CanTransition(tt.to)

			if result != tt.expected {
				t.Errorf("CanTransition(%v) from %v: expected %v, got %v", tt.to, tt.from, tt.expected, result)
			}
		})
	}
}

func TestManagerReset(t *testing.T) {
	manager := NewManager()

	// Transition to a different state
	err := manager.Transition(Help)
	if err != nil {
		t.Fatalf("Transition to Help should succeed: %v", err)
	}

	if manager.Current() != Help {
		t.Fatalf("State should be Help before reset, got %v", manager.Current())
	}

	// Reset to default
	manager.Reset()

	if manager.Current() != Default {
		t.Errorf("State should be Default after reset, got %v", manager.Current())
	}
}

func TestManagerIsOverlay(t *testing.T) {
	manager := NewManager()

	// Test default state (not overlay)
	if manager.IsOverlay() {
		t.Errorf("Default state should not be overlay")
	}

	// Test overlay states
	overlayStates := []State{New, Prompt, Help, Confirm, AdvancedNew, Git, ClaudeSettings, ZFSearch}

	for _, state := range overlayStates {
		err := manager.Transition(state)
		if err != nil {
			t.Fatalf("Transition to %v should succeed: %v", state, err)
		}

		if !manager.IsOverlay() {
			t.Errorf("State %v should be overlay", state)
		}

		manager.Reset()
	}
}

func TestManagerIsAsync(t *testing.T) {
	manager := NewManager()

	// Test default state (not async)
	if manager.IsAsync() {
		t.Errorf("Default state should not be async")
	}

	// Test async state
	err := manager.Transition(New)
	if err != nil {
		t.Fatalf("Transition to New should succeed: %v", err)
	}

	err = manager.Transition(CreatingSession)
	if err != nil {
		t.Fatalf("Transition to CreatingSession should succeed: %v", err)
	}

	if !manager.IsAsync() {
		t.Errorf("CreatingSession state should be async")
	}
}

func TestStateValidation(t *testing.T) {
	manager := NewManager()

	// Test valid states
	validStates := []State{Default, New, Prompt, Help, Confirm, CreatingSession, AdvancedNew, Git, ClaudeSettings, ZFSearch}

	for _, state := range validStates {
		if !manager.IsValid(state) {
			t.Errorf("State %v should be valid", state)
		}

		if !state.IsValid() {
			t.Errorf("State %v should report itself as valid", state)
		}
	}

	// Test invalid state
	invalidState := State(999)
	if manager.IsValid(invalidState) {
		t.Errorf("State %v should be invalid", invalidState)
	}

	if invalidState.IsValid() {
		t.Errorf("State %v should report itself as invalid", invalidState)
	}
}

func TestStateStringRepresentation(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{Default, "Default"},
		{New, "New"},
		{Prompt, "Prompt"},
		{Help, "Help"},
		{Confirm, "Confirm"},
		{CreatingSession, "CreatingSession"},
		{AdvancedNew, "AdvancedNew"},
		{Git, "Git"},
		{ClaudeSettings, "ClaudeSettings"},
		{ZFSearch, "ZFSearch"},
		{State(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			if result != tt.expected {
				t.Errorf("State %v String(): expected %s, got %s", tt.state, tt.expected, result)
			}
		})
	}
}

func TestStateClassification(t *testing.T) {
	tests := []struct {
		state     State
		isOverlay bool
		isAsync   bool
	}{
		{Default, false, false},
		{New, true, false},
		{Prompt, true, false},
		{Help, true, false},
		{Confirm, true, false},
		{CreatingSession, false, true},
		{AdvancedNew, true, false},
		{Git, true, false},
		{ClaudeSettings, true, false},
		{ZFSearch, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			if tt.state.IsOverlayState() != tt.isOverlay {
				t.Errorf("State %v IsOverlayState(): expected %v, got %v", tt.state, tt.isOverlay, tt.state.IsOverlayState())
			}

			if tt.state.IsAsyncState() != tt.isAsync {
				t.Errorf("State %v IsAsyncState(): expected %v, got %v", tt.state, tt.isAsync, tt.state.IsAsyncState())
			}
		})
	}
}

func TestTransitionValidation(t *testing.T) {
	manager := NewManager().(*manager) // Cast to access ValidateTransition method

	// Test valid transition
	validation := manager.ValidateTransition(New)
	if !validation.Valid {
		t.Errorf("Validation should be valid for Default -> New transition")
	}
	if validation.From != Default {
		t.Errorf("Validation From should be Default, got %v", validation.From)
	}
	if validation.To != New {
		t.Errorf("Validation To should be New, got %v", validation.To)
	}

	// Test invalid transition
	validation = manager.ValidateTransition(State(999))
	if validation.Valid {
		t.Errorf("Validation should be invalid for Default -> Invalid State transition")
	}
	if validation.Reason == "" {
		t.Errorf("Validation should provide a reason for invalid transition")
	}
}

// Test StateTransitionHandler methods

func TestTransitionWithContext(t *testing.T) {
	manager := NewManager()

	tests := []struct {
		name            string
		targetState     State
		context         TransitionContext
		expectSuccess   bool
		expectError     bool
		setupState      State
	}{
		{
			name:        "Valid transition with empty context",
			targetState: New,
			context:     TransitionContext{},
			expectSuccess: true,
			expectError: false,
			setupState:  Default,
		},
		{
			name:        "Valid transition with menu state context",
			targetState: Help,
			context: TransitionContext{
				MenuState:   "Help",
				OverlayName: "help",
			},
			expectSuccess: true,
			expectError: false,
			setupState:  Default,
		},
		{
			name:        "Invalid state transition",
			targetState: State(999),
			context:     TransitionContext{},
			expectSuccess: false,
			expectError: true,
			setupState:  Default,
		},
		{
			name:        "Transition with pre-action success",
			targetState: Confirm,
			context: TransitionContext{
				PreTransitionAction: func() error {
					return nil // Success
				},
			},
			expectSuccess: true,
			expectError: false,
			setupState:  Default,
		},
		{
			name:        "Transition with pre-action failure",
			targetState: Confirm,
			context: TransitionContext{
				PreTransitionAction: func() error {
					return fmt.Errorf("pre-action failed")
				},
			},
			expectSuccess: false,
			expectError: true,
			setupState:  Default,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset manager to setup state
			manager.Reset()
			if tt.setupState != Default {
				manager.Transition(tt.setupState)
			}

			result, err := manager.TransitionWithContext(tt.targetState, tt.context)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if result != nil {
				if result.Success != tt.expectSuccess {
					t.Errorf("Expected success %v, got %v", tt.expectSuccess, result.Success)
				}

				if tt.expectSuccess {
					if result.NewState != tt.targetState {
						t.Errorf("Expected new state %v, got %v", tt.targetState, result.NewState)
					}
					if manager.Current() != tt.targetState {
						t.Errorf("Manager current state should be %v, got %v", tt.targetState, manager.Current())
					}
				}
			}
		})
	}
}

func TestTransitionToDefault(t *testing.T) {
	manager := NewManager()

	// Start in a non-default state
	err := manager.Transition(Help)
	if err != nil {
		t.Fatalf("Setup transition failed: %v", err)
	}

	ctx := TransitionContext{
		MenuState:          "Default",
		ShouldCloseOverlay: true,
	}

	result, err := manager.TransitionToDefault(ctx)

	if err != nil {
		t.Errorf("TransitionToDefault should not fail: %v", err)
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if !result.Success {
		t.Errorf("Transition should be successful")
	}

	if result.NewState != Default {
		t.Errorf("New state should be Default, got %v", result.NewState)
	}

	if manager.Current() != Default {
		t.Errorf("Manager should be in Default state, got %v", manager.Current())
	}

	// Verify context is stored
	storedCtx := manager.GetTransitionContext()
	if storedCtx.MenuState != "Default" {
		t.Errorf("Expected menu state 'Default', got %s", storedCtx.MenuState)
	}
}

func TestTransitionToOverlay(t *testing.T) {
	manager := NewManager()

	tests := []struct {
		name        string
		targetState State
		expectError bool
	}{
		{
			name:        "Valid overlay state",
			targetState: New,
			expectError: false,
		},
		{
			name:        "Valid overlay state - Help",
			targetState: Help,
			expectError: false,
		},
		{
			name:        "Invalid non-overlay state",
			targetState: Default,
			expectError: true,
		},
		{
			name:        "Invalid non-overlay state - CreatingSession",
			targetState: CreatingSession,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager.Reset()

			ctx := TransitionContext{
				MenuState:   "Test",
				OverlayName: "test",
			}

			result, err := manager.TransitionToOverlay(tt.targetState, ctx)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for non-overlay state %v", tt.targetState)
			}

			if !tt.expectError {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result == nil || !result.Success {
					t.Errorf("Transition should be successful")
				}
				if manager.Current() != tt.targetState {
					t.Errorf("Manager should be in state %v, got %v", tt.targetState, manager.Current())
				}
			}
		})
	}
}

func TestCommonTransitionPatterns(t *testing.T) {
	manager := NewManager()

	t.Run("TransitionToConfirm", func(t *testing.T) {
		result, err := manager.TransitionToConfirm("Test confirmation")

		if err != nil {
			t.Errorf("TransitionToConfirm should not fail: %v", err)
		}
		if result == nil || !result.Success {
			t.Errorf("Transition should be successful")
		}
		if manager.Current() != Confirm {
			t.Errorf("Manager should be in Confirm state, got %v", manager.Current())
		}

		ctx := manager.GetTransitionContext()
		if ctx.MenuState != "Confirm" {
			t.Errorf("Expected menu state 'Confirm', got %s", ctx.MenuState)
		}
	})

	t.Run("TransitionToHelp", func(t *testing.T) {
		manager.Reset()
		result, err := manager.TransitionToHelp("messages")

		if err != nil {
			t.Errorf("TransitionToHelp should not fail: %v", err)
		}
		if result == nil || !result.Success {
			t.Errorf("Transition should be successful")
		}
		if manager.Current() != Help {
			t.Errorf("Manager should be in Help state, got %v", manager.Current())
		}
	})

	t.Run("TransitionToCreatingSession", func(t *testing.T) {
		manager.Reset()
		result, err := manager.TransitionToCreatingSession()

		if err != nil {
			t.Errorf("TransitionToCreatingSession should not fail: %v", err)
		}
		if result == nil || !result.Success {
			t.Errorf("Transition should be successful")
		}
		if manager.Current() != CreatingSession {
			t.Errorf("Manager should be in CreatingSession state, got %v", manager.Current())
		}

		ctx := manager.GetTransitionContext()
		if ctx.MenuState != "CreatingInstance" {
			t.Errorf("Expected menu state 'CreatingInstance', got %s", ctx.MenuState)
		}
		if !ctx.SessionActive {
			t.Errorf("SessionActive should be true")
		}
	})

	t.Run("TransitionWithCleanup", func(t *testing.T) {
		// Start in overlay state
		manager.Transition(New)
		result, err := manager.TransitionWithCleanup("test-overlay")

		if err != nil {
			t.Errorf("TransitionWithCleanup should not fail: %v", err)
		}
		if result == nil || !result.Success {
			t.Errorf("Transition should be successful")
		}
		if manager.Current() != Default {
			t.Errorf("Manager should be in Default state, got %v", manager.Current())
		}

		ctx := manager.GetTransitionContext()
		if !ctx.ShouldCloseOverlay {
			t.Errorf("ShouldCloseOverlay should be true")
		}
		if ctx.OverlayName != "test-overlay" {
			t.Errorf("Expected overlay name 'test-overlay', got %s", ctx.OverlayName)
		}
	})
}

func TestTransitionContextPersistence(t *testing.T) {
	manager := NewManager()

	ctx := TransitionContext{
		MenuState:       "Test",
		OverlayName:     "test-overlay",
		SessionActive:   true,
		ErrorMessage:    "test error",
	}

	_, err := manager.TransitionWithContext(New, ctx)
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	storedCtx := manager.GetTransitionContext()

	if storedCtx.MenuState != ctx.MenuState {
		t.Errorf("MenuState not persisted: expected %s, got %s", ctx.MenuState, storedCtx.MenuState)
	}
	if storedCtx.OverlayName != ctx.OverlayName {
		t.Errorf("OverlayName not persisted: expected %s, got %s", ctx.OverlayName, storedCtx.OverlayName)
	}
	if storedCtx.SessionActive != ctx.SessionActive {
		t.Errorf("SessionActive not persisted: expected %v, got %v", ctx.SessionActive, storedCtx.SessionActive)
	}
	if storedCtx.ErrorMessage != ctx.ErrorMessage {
		t.Errorf("ErrorMessage not persisted: expected %s, got %s", ctx.ErrorMessage, storedCtx.ErrorMessage)
	}
}

func TestTransitionRollback(t *testing.T) {
	manager := NewManager()

	// Test rollback on post-transition action failure
	ctx := TransitionContext{
		PostTransitionAction: func() error {
			return fmt.Errorf("post-action failed")
		},
	}

	initialState := manager.Current()
	result, err := manager.TransitionWithContext(New, ctx)

	if err == nil {
		t.Errorf("Expected error due to post-action failure")
	}

	if result.Success {
		t.Errorf("Transition should not be successful")
	}

	// Verify rollback - state should remain unchanged
	if manager.Current() != initialState {
		t.Errorf("State should be rolled back to %v, got %v", initialState, manager.Current())
	}
}