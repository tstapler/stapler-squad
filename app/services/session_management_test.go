package services

import (
	appsession "claude-squad/app/session"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"errors"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
)

// mockSessionController implements a mock session controller for testing
type mockSessionController struct {
	validateErr       error
	operationResult   appsession.SessionOperation
	selectedInstance  *session.Instance
	canPerformOp      bool
	newSessionCalled  bool
	killSessionCalled bool
	// For concurrent testing - callback to run during NewSession
	newSessionCallback func()
	mu                 sync.Mutex
}

func (m *mockSessionController) NewSession() appsession.SessionOperation {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.newSessionCalled = true
	if m.newSessionCallback != nil {
		m.newSessionCallback()
	}
	return m.operationResult
}

func (m *mockSessionController) KillSession() appsession.SessionOperation {
	m.killSessionCalled = true
	return m.operationResult
}

func (m *mockSessionController) AttachSession() appsession.SessionOperation {
	return m.operationResult
}

func (m *mockSessionController) CheckoutSession() appsession.SessionOperation {
	return m.operationResult
}

func (m *mockSessionController) ResumeSession() appsession.SessionOperation {
	return m.operationResult
}

func (m *mockSessionController) ValidateNewSession() error {
	return m.validateErr
}

func (m *mockSessionController) GetSelectedSession() *session.Instance {
	return m.selectedInstance
}

func (m *mockSessionController) CanPerformOperation(op appsession.SessionOperationType) bool {
	return m.canPerformOp
}

func (m *mockSessionController) SetDependencies(deps appsession.Dependencies) {}

func (m *mockSessionController) GetDependencies() appsession.Dependencies {
	return appsession.Dependencies{}
}

// setupTestService creates a test service with mocked dependencies
func setupTestService(t *testing.T, instanceLimit int) (*sessionManagementService, *mockSessionController, *ui.List) {
	// Create storage
	appState := config.LoadState()
	storage, err := session.NewStorage(appState)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	// Create list component
	sp := spinner.New()
	list := ui.NewList(&sp, false, appState)

	// Create mock controller
	mockCtrl := &mockSessionController{
		operationResult: appsession.SessionOperation{
			Success: true,
		},
	}

	// Create service
	service := &sessionManagementService{
		storage:           storage,
		sessionController: mockCtrl,
		list:              list,
		errorHandler:      func(err error) tea.Cmd { return nil },
		instanceLimit:     instanceLimit,
	}

	return service, mockCtrl, list
}

func TestSessionManagementService_CreateSession_Success(t *testing.T) {
	service, mockCtrl, _ := setupTestService(t, 10)

	// Act
	result := service.CreateSession()

	// Assert
	if !result.Success {
		t.Errorf("Expected success, got failure: %v", result.Error)
	}
	if !mockCtrl.newSessionCalled {
		t.Error("Expected NewSession to be called on controller")
	}
}

func TestSessionManagementService_CreateSession_InstanceLimitReached(t *testing.T) {
	service, mockCtrl, list := setupTestService(t, 2)

	// Add instances to reach limit
	inst1, err := session.NewInstance(session.InstanceOptions{
		Title:   "test1",
		Path:    "/tmp",
		Program: "claude",
	})
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	inst2, err := session.NewInstance(session.InstanceOptions{
		Title:   "test2",
		Path:    "/tmp",
		Program: "claude",
	})
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	list.AddInstance(inst1)
	list.AddInstance(inst2)

	// Act
	result := service.CreateSession()

	// Assert
	if result.Success {
		t.Error("Expected failure due to instance limit, got success")
	}
	if result.Error == nil {
		t.Error("Expected error to be set")
	}
	if result.Error.Error() != "instance limit reached" {
		t.Errorf("Expected 'instance limit reached' error, got: %v", result.Error)
	}
	if mockCtrl.newSessionCalled {
		t.Error("NewSession should not be called when limit is reached")
	}
}

func TestSessionManagementService_CreateSession_ValidationError(t *testing.T) {
	service, mockCtrl, _ := setupTestService(t, 10)

	// Setup mock to return validation error
	mockCtrl.validateErr = errors.New("validation failed")

	// Act
	result := service.CreateSession()

	// Assert
	if result.Success {
		t.Error("Expected failure due to validation error, got success")
	}
	if result.Error == nil {
		t.Error("Expected error to be set")
	}
	if mockCtrl.newSessionCalled {
		t.Error("NewSession should not be called when validation fails")
	}
}

func TestSessionManagementService_CreateSession_ControllerError(t *testing.T) {
	service, mockCtrl, _ := setupTestService(t, 10)

	// Setup mock to return operation error
	mockCtrl.operationResult = appsession.SessionOperation{
		Success: false,
		Error:   errors.New("controller error"),
	}

	// Act
	result := service.CreateSession()

	// Assert
	if result.Success {
		t.Error("Expected failure due to controller error, got success")
	}
	if result.Error == nil {
		t.Error("Expected error to be set")
	}
	if result.Error.Error() != "controller error" {
		t.Errorf("Expected 'controller error', got: %v", result.Error)
	}
	if !mockCtrl.newSessionCalled {
		t.Error("NewSession should be called even if it fails")
	}
}

func TestSessionManagementService_ConcurrentCreation(t *testing.T) {
	service, mockCtrl, list := setupTestService(t, 5)

	// Add instances near the limit
	for i := 0; i < 4; i++ {
		inst, err := session.NewInstance(session.InstanceOptions{
			Title:   "test" + string(rune('0'+i)),
			Path:    "/tmp",
			Program: "claude",
		})
		if err != nil {
			t.Fatalf("Failed to create instance: %v", err)
		}
		list.AddInstance(inst)
	}

	// Setup mock to simulate adding instance in controller
	mockCtrl.operationResult = appsession.SessionOperation{Success: true}

	// Add callback to simulate controller adding instance during NewSession
	mockCtrl.newSessionCallback = func() {
		inst, _ := session.NewInstance(session.InstanceOptions{
			Title:   "concurrent",
			Path:    "/tmp",
			Program: "claude",
		})
		if inst != nil {
			list.AddInstance(inst)
		}
	}

	// Attempt concurrent creation from 10 goroutines
	var wg sync.WaitGroup
	results := make([]SessionResult, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = service.CreateSession()
		}(i)
	}

	wg.Wait()

	// Count successes and failures
	successes := 0
	failures := 0
	for _, result := range results {
		if result.Success {
			successes++
		} else {
			failures++
		}
	}

	// Assert: Only 1 should succeed (we had 4, limit is 5)
	// All others should fail with "instance limit reached"
	// The mutex protection should ensure exactly 1 success
	if successes != 1 {
		t.Errorf("Expected exactly 1 success (mutex protection), got %d successes", successes)
		t.Logf("Successes: %d, Failures: %d, Final count: %d", successes, failures, list.NumInstances())
	}
	if failures != 9 {
		t.Errorf("Expected 9 failures, got %d", failures)
	}

	// Final count should be exactly 5 (4 initial + 1 successful)
	if list.NumInstances() != 5 {
		t.Errorf("Expected 5 total instances, got %d", list.NumInstances())
	}
}

func TestSessionManagementService_KillSession(t *testing.T) {
	service, mockCtrl, _ := setupTestService(t, 10)

	// Act
	result := service.KillSession()

	// Assert
	if !result.Success {
		t.Errorf("Expected success, got failure: %v", result.Error)
	}
	if !mockCtrl.killSessionCalled {
		t.Error("Expected KillSession to be called on controller")
	}
}

func TestSessionManagementService_AttachSession(t *testing.T) {
	service, mockCtrl, _ := setupTestService(t, 10)

	// Setup mock to return success
	mockCtrl.operationResult = appsession.SessionOperation{
		Success: true,
	}

	// Act
	result := service.AttachSession()

	// Assert
	if !result.Success {
		t.Errorf("Expected success, got failure: %v", result.Error)
	}
}

func TestSessionManagementService_ResumeSession(t *testing.T) {
	service, mockCtrl, _ := setupTestService(t, 10)

	// Setup mock to return success
	mockCtrl.operationResult = appsession.SessionOperation{
		Success: true,
	}

	// Act
	result := service.ResumeSession()

	// Assert
	if !result.Success {
		t.Errorf("Expected success, got failure: %v", result.Error)
	}
}

func TestSessionManagementService_CheckoutSession(t *testing.T) {
	service, mockCtrl, _ := setupTestService(t, 10)

	// Setup mock to return success
	mockCtrl.operationResult = appsession.SessionOperation{
		Success: true,
	}

	// Act
	result := service.CheckoutSession()

	// Assert
	if !result.Success {
		t.Errorf("Expected success, got failure: %v", result.Error)
	}
}

func TestSessionManagementService_GetActiveSessionsCount(t *testing.T) {
	service, _, list := setupTestService(t, 10)

	// Initially should be 0
	if count := service.GetActiveSessionsCount(); count != 0 {
		t.Errorf("Expected 0 instances, got %d", count)
	}

	// Add instances
	inst1, err := session.NewInstance(session.InstanceOptions{
		Title:   "test1",
		Path:    "/tmp",
		Program: "claude",
	})
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	inst2, err := session.NewInstance(session.InstanceOptions{
		Title:   "test2",
		Path:    "/tmp",
		Program: "claude",
	})
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	list.AddInstance(inst1)
	list.AddInstance(inst2)

	// Should now be 2
	if count := service.GetActiveSessionsCount(); count != 2 {
		t.Errorf("Expected 2 instances, got %d", count)
	}
}

func TestSessionManagementService_GetSelectedSession(t *testing.T) {
	service, mockCtrl, _ := setupTestService(t, 10)

	// Setup mock to return a specific instance
	expectedInst, err := session.NewInstance(session.InstanceOptions{
		Title:   "test",
		Path:    "/tmp",
		Program: "claude",
	})
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	mockCtrl.selectedInstance = expectedInst

	// Act
	inst := service.GetSelectedSession()

	// Assert
	if inst != expectedInst {
		t.Error("Expected to get the mock instance")
	}
}

func TestSessionManagementService_CanPerformOperation(t *testing.T) {
	tests := []struct {
		name     string
		canDo    bool
		op       appsession.SessionOperationType
		expected bool
	}{
		{
			name:     "can perform operation",
			canDo:    true,
			op:       appsession.OpNewSession,
			expected: true,
		},
		{
			name:     "cannot perform operation",
			canDo:    false,
			op:       appsession.OpKillSession,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockCtrl, _ := setupTestService(t, 10)
			mockCtrl.canPerformOp = tt.canDo

			// Act
			result := service.CanPerformOperation(tt.op)

			// Assert
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSessionManagementService_ValidateNewSession(t *testing.T) {
	tests := []struct {
		name          string
		instanceCount int
		limit         int
		validateErr   error
		expectErr     bool
	}{
		{
			name:          "under limit, no validation error",
			instanceCount: 3,
			limit:         10,
			validateErr:   nil,
			expectErr:     false,
		},
		{
			name:          "at limit",
			instanceCount: 10,
			limit:         10,
			validateErr:   nil,
			expectErr:     true,
		},
		{
			name:          "over limit",
			instanceCount: 11,
			limit:         10,
			validateErr:   nil,
			expectErr:     true,
		},
		{
			name:          "under limit, but validation error",
			instanceCount: 3,
			limit:         10,
			validateErr:   errors.New("validation error"),
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, mockCtrl, list := setupTestService(t, tt.limit)
			mockCtrl.validateErr = tt.validateErr

			// Add instances
			for i := 0; i < tt.instanceCount; i++ {
				inst, err := session.NewInstance(session.InstanceOptions{
					Title:   "test" + string(rune('0'+i)),
					Path:    "/tmp",
					Program: "claude",
				})
				if err != nil {
					t.Fatalf("Failed to create instance: %v", err)
				}
				list.AddInstance(inst)
			}

			// Act
			err := service.ValidateNewSession()

			// Assert
			if tt.expectErr && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}
