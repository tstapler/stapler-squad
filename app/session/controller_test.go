package session

import (
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"errors"
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Mock dependencies for testing
type mockDependencies struct {
	list            *ui.List
	storage         *mockStorage
	autoYes         bool
	instanceLimit   int
	errorHandlerCalls []error
	stateTransitions  []string
	instanceChangedCalls int
	confirmActionCalls []string
	helpScreenCalls    []interface{}
	sessionSetupOverlay *overlay.SessionSetupOverlay
	pendingSessions    []*session.Instance
	pendingAutoYes     []bool
	finalizers         []func()
}

type mockStorage struct {
	instances []string
	deleted   []string
	errors    map[string]error
}

func (m *mockStorage) DeleteInstance(title string) error {
	if err, exists := m.errors[title]; exists {
		return err
	}
	m.deleted = append(m.deleted, title)
	return nil
}

func (m *mockStorage) LoadInstances() ([]*session.Instance, error) {
	return nil, nil
}

func (m *mockStorage) SaveInstances([]*session.Instance) error {
	return nil
}

func (m *mockStorage) GetStateManager() interface{} {
	return nil
}

func newMockDependencies() *mockDependencies {
	spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spinner, false, nil)

	return &mockDependencies{
		list:          list,
		storage:       &mockStorage{errors: make(map[string]error)},
		autoYes:       false,
		instanceLimit: 100,
	}
}

func (m *mockDependencies) toDependencies() Dependencies {
	return Dependencies{
		List:                m.list,
		Storage:             m.storage,
		AutoYes:             m.autoYes,
		GlobalInstanceLimit: m.instanceLimit,
		ErrorHandler: func(err error) tea.Cmd {
			m.errorHandlerCalls = append(m.errorHandlerCalls, err)
			return func() tea.Msg { return err }
		},
		StateTransition: func(to string) error {
			m.stateTransitions = append(m.stateTransitions, to)
			return nil
		},
		InstanceChanged: func() tea.Cmd {
			m.instanceChangedCalls++
			return nil
		},
		ConfirmAction: func(message string, action tea.Cmd) tea.Cmd {
			m.confirmActionCalls = append(m.confirmActionCalls, message)
			return nil
		},
		ShowHelpScreen: func(helpType interface{}, onComplete func()) {
			m.helpScreenCalls = append(m.helpScreenCalls, helpType)
			if onComplete != nil {
				onComplete()
			}
		},
		SetSessionSetupOverlay: func(overlay *overlay.SessionSetupOverlay) {
			m.sessionSetupOverlay = overlay
		},
		GetSessionSetupOverlay: func() *overlay.SessionSetupOverlay {
			return m.sessionSetupOverlay
		},
		SetPendingSession: func(inst *session.Instance, autoYes bool) {
			m.pendingSessions = append(m.pendingSessions, inst)
			m.pendingAutoYes = append(m.pendingAutoYes, autoYes)
		},
		GetNewInstanceFinalizer: func() func() {
			if len(m.finalizers) > 0 {
				return m.finalizers[len(m.finalizers)-1]
			}
			return nil
		},
		SetNewInstanceFinalizer: func(f func()) {
			m.finalizers = append(m.finalizers, f)
		},
	}
}

func TestNewController(t *testing.T) {
	deps := newMockDependencies().toDependencies()
	controller := NewController(deps)

	if controller == nil {
		t.Fatal("NewController should not return nil")
	}

	// Test dependency access
	retrievedDeps := controller.GetDependencies()
	if retrievedDeps.AutoYes != deps.AutoYes {
		t.Errorf("Expected AutoYes %v, got %v", deps.AutoYes, retrievedDeps.AutoYes)
	}
}

func TestValidateNewSession(t *testing.T) {
	tests := []struct {
		name           string
		instanceCount  int
		instanceLimit  int
		expectedError  bool
	}{
		{
			name:          "Under limit",
			instanceCount: 50,
			instanceLimit: 100,
			expectedError: false,
		},
		{
			name:          "At limit",
			instanceCount: 100,
			instanceLimit: 100,
			expectedError: true,
		},
		{
			name:          "Over limit",
			instanceCount: 150,
			instanceLimit: 100,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDeps := newMockDependencies()
			mockDeps.instanceLimit = tt.instanceLimit

			// Add mock instances to reach the desired count
			for i := 0; i < tt.instanceCount; i++ {
				// Create minimal instance for testing
				instance, _ := session.NewInstance(session.InstanceOptions{
					Title:   fmt.Sprintf("test-%d", i),
					Path:    "/tmp",
					Program: "test",
					AutoYes: false,
				})
				mockDeps.list.AddInstance(instance)()
			}

			controller := NewController(mockDeps.toDependencies())
			err := controller.ValidateNewSession()

			if tt.expectedError && err == nil {
				t.Errorf("Expected validation error, but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}

			if err != nil {
				sessionErr, ok := err.(*SessionError)
				if !ok {
					t.Errorf("Expected SessionError, got %T", err)
				} else if sessionErr.Operation != OpNewSession {
					t.Errorf("Expected OpNewSession, got %v", sessionErr.Operation)
				}
			}
		})
	}
}

func TestCanPerformOperation(t *testing.T) {
	tests := []struct {
		name          string
		operation     SessionOperationType
		setupInstance bool
		instanceCount int
		instanceLimit int
		expected      bool
	}{
		{
			name:          "NewSession - under limit",
			operation:     OpNewSession,
			instanceCount: 50,
			instanceLimit: 100,
			expected:      true,
		},
		{
			name:          "NewSession - at limit",
			operation:     OpNewSession,
			instanceCount: 100,
			instanceLimit: 100,
			expected:      false,
		},
		{
			name:          "KillSession - with selection",
			operation:     OpKillSession,
			setupInstance: true,
			expected:      true,
		},
		{
			name:      "KillSession - no selection",
			operation: OpKillSession,
			expected:  false,
		},
		{
			name:          "AttachSession - no instances",
			operation:     OpAttachSession,
			instanceCount: 0,
			expected:      false,
		},
		{
			name:          "AttachSession - with valid instance",
			operation:     OpAttachSession,
			setupInstance: true,
			instanceCount: 1,
			expected:      false, // Note: will be false in test because tmux not actually alive
		},
		{
			name:          "CheckoutSession - with selection",
			operation:     OpCheckoutSession,
			setupInstance: true,
			expected:      true,
		},
		{
			name:      "CheckoutSession - no selection",
			operation: OpCheckoutSession,
			expected:  false,
		},
		{
			name:          "ResumeSession - with selection",
			operation:     OpResumeSession,
			setupInstance: true,
			expected:      true,
		},
		{
			name:      "ResumeSession - no selection",
			operation: OpResumeSession,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDeps := newMockDependencies()
			mockDeps.instanceLimit = tt.instanceLimit

			// Set up instances if needed
			for i := 0; i < tt.instanceCount; i++ {
				instance, _ := session.NewInstance(session.InstanceOptions{
					Title:   fmt.Sprintf("test-%d", i),
					Path:    "/tmp",
					Program: "test",
					AutoYes: false,
				})
				mockDeps.list.AddInstance(instance)()
			}

			if tt.setupInstance && tt.instanceCount == 0 {
				// Add one instance for selection tests
				instance, _ := session.NewInstance(session.InstanceOptions{
					Title:   "test-selection",
					Path:    "/tmp",
					Program: "test",
					AutoYes: false,
				})
				mockDeps.list.AddInstance(instance)()
				mockDeps.list.SetSelectedInstance(0)
			} else if tt.setupInstance && tt.instanceCount > 0 {
				mockDeps.list.SetSelectedInstance(0)
			}

			controller := NewController(mockDeps.toDependencies())
			result := controller.CanPerformOperation(tt.operation)

			if result != tt.expected {
				t.Errorf("Expected CanPerformOperation(%v) = %v, got %v", tt.operation, tt.expected, result)
			}
		})
	}
}

func TestKillSession(t *testing.T) {
	t.Run("No selection", func(t *testing.T) {
		mockDeps := newMockDependencies()
		controller := NewController(mockDeps.toDependencies())

		result := controller.KillSession()

		if !result.Success {
			t.Error("Expected success for no selection case")
		}
		if result.Error != nil {
			t.Errorf("Unexpected error: %v", result.Error)
		}
		if len(mockDeps.confirmActionCalls) > 0 {
			t.Error("Should not call ConfirmAction when no selection")
		}
	})

	t.Run("With selection", func(t *testing.T) {
		mockDeps := newMockDependencies()

		// Add test instance
		instance, _ := session.NewInstance(session.InstanceOptions{
			Title:   "test-session",
			Path:    "/tmp",
			Program: "test",
			AutoYes: false,
		})
		mockDeps.list.AddInstance(instance)()
		mockDeps.list.SetSelectedInstance(0)

		controller := NewController(mockDeps.toDependencies())
		result := controller.KillSession()

		if !result.Success {
			t.Error("Expected success with valid selection")
		}
		if result.Error != nil {
			t.Errorf("Unexpected error: %v", result.Error)
		}
		if len(mockDeps.confirmActionCalls) != 1 {
			t.Errorf("Expected 1 ConfirmAction call, got %d", len(mockDeps.confirmActionCalls))
		}
		if len(mockDeps.confirmActionCalls) > 0 &&
		   mockDeps.confirmActionCalls[0] != "[!] Kill session 'test-session'?" {
			t.Errorf("Unexpected confirmation message: %s", mockDeps.confirmActionCalls[0])
		}
	})
}

func TestResumeSession(t *testing.T) {
	t.Run("No selection", func(t *testing.T) {
		mockDeps := newMockDependencies()
		controller := NewController(mockDeps.toDependencies())

		result := controller.ResumeSession()

		if !result.Success {
			t.Error("Expected success for no selection case")
		}
		if result.Error != nil {
			t.Errorf("Unexpected error: %v", result.Error)
		}
	})

	t.Run("With selection - resume error", func(t *testing.T) {
		mockDeps := newMockDependencies()

		// Add test instance (not started, so resume will fail)
		instance, _ := session.NewInstance(session.InstanceOptions{
			Title:   "test-session",
			Path:    "/tmp",
			Program: "test",
			AutoYes: false,
		})
		mockDeps.list.AddInstance(instance)()
		mockDeps.list.SetSelectedInstance(0)

		controller := NewController(mockDeps.toDependencies())
		result := controller.ResumeSession()

		if result.Success {
			t.Error("Expected failure for unstarted instance")
		}
		if result.Error == nil {
			t.Error("Expected error for unstarted instance")
		}
		if result.Cmd == nil {
			t.Error("Expected error handler command")
		}
	})
}

func TestSessionOperationType(t *testing.T) {
	tests := []struct {
		op       SessionOperationType
		expected string
	}{
		{OpNewSession, "NewSession"},
		{OpKillSession, "KillSession"},
		{OpAttachSession, "AttachSession"},
		{OpCheckoutSession, "CheckoutSession"},
		{OpResumeSession, "ResumeSession"},
		{SessionOperationType(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.op.String()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestSessionError(t *testing.T) {
	t.Run("Error without cause", func(t *testing.T) {
		err := &SessionError{
			Operation: OpNewSession,
			Message:   "test error",
		}

		if err.Error() != "test error" {
			t.Errorf("Expected 'test error', got '%s'", err.Error())
		}

		if err.Unwrap() != nil {
			t.Errorf("Expected nil unwrap, got %v", err.Unwrap())
		}
	})

	t.Run("Error with cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := &SessionError{
			Operation: OpKillSession,
			Message:   "test error",
			Cause:     cause,
		}

		expected := "test error: underlying error"
		if err.Error() != expected {
			t.Errorf("Expected '%s', got '%s'", expected, err.Error())
		}

		if err.Unwrap() != cause {
			t.Errorf("Expected cause unwrap, got %v", err.Unwrap())
		}
	})
}