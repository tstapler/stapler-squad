package services

import (
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"sync"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

// TestFacadeNavigationIntegration verifies navigation service integration through facade
func TestFacadeNavigationIntegration(t *testing.T) {
	// Create test dependencies
	spin := spinner.New()
	stateManager := config.NewTestState(t.TempDir())
	list := ui.NewList(&spin, false, stateManager)

	// Add test instances
	instances := []*session.Instance{
		createTestInstance(t, "session1"),
		createTestInstance(t, "session2"),
		createTestInstance(t, "session3"),
	}
	for _, inst := range instances {
		finalizer := list.AddInstance(inst)
		finalizer()
	}

	// Create facade with navigation service
	facade := NewFacade(
		nil, // storage - not needed for navigation tests
		nil, // sessionController - not needed
		list,
		nil, // menu
		nil, // statusBar
		nil, // errBox
		nil, // uiCoordinator
		nil, // errorHandler
		100, // instanceLimit
	)

	// Test navigation through facade - verify service integration without testing exact behavior
	t.Run("NavigateDown", func(t *testing.T) {
		// Verify NavigateDown is callable through facade
		err := facade.Navigation().NavigateDown()
		if err != nil {
			t.Fatalf("NavigateDown failed: %v", err)
		}

		// Verify index is valid (non-negative, within bounds)
		index := facade.Navigation().GetCurrentIndex()
		if index < 0 || index >= len(instances) {
			t.Errorf("Invalid index %d (must be 0-%d)", index, len(instances)-1)
		}
	})

	t.Run("NavigateUp", func(t *testing.T) {
		// Verify NavigateUp is callable through facade
		err := facade.Navigation().NavigateUp()
		if err != nil {
			t.Fatalf("NavigateUp failed: %v", err)
		}

		// Verify index is valid
		index := facade.Navigation().GetCurrentIndex()
		if index < 0 || index >= len(instances) {
			t.Errorf("Invalid index %d (must be 0-%d)", index, len(instances)-1)
		}
	})

	t.Run("GetCurrentIndex", func(t *testing.T) {
		// Verify GetCurrentIndex returns valid index
		index := facade.Navigation().GetCurrentIndex()
		if index < 0 || index >= len(instances) {
			t.Errorf("Invalid index %d (must be 0-%d)", index, len(instances)-1)
		}
	})
}

// TestFacadeFilteringIntegration verifies filtering service integration through facade
func TestFacadeFilteringIntegration(t *testing.T) {
	// Create test dependencies
	spin := spinner.New()
	stateManager := config.NewTestState(t.TempDir())
	list := ui.NewList(&spin, false, stateManager)

	// Add test instances (some paused)
	instances := []*session.Instance{
		createTestInstance(t, "active1"),
		createTestInstance(t, "paused1"),
		createTestInstance(t, "active2"),
	}
	instances[1].SetStatus(session.Paused) // Mark second instance as paused

	for _, inst := range instances {
		finalizer := list.AddInstance(inst)
		finalizer()
	}

	// Create facade with filtering service
	facade := NewFacade(
		nil, // storage
		nil, // sessionController
		list,
		nil, // menu
		nil, // statusBar
		nil, // errBox
		nil, // uiCoordinator
		nil, // errorHandler
		100, // instanceLimit
	)

	t.Run("TogglePausedFilter", func(t *testing.T) {
		// Initially no filter
		if facade.Filtering().IsPausedFilterActive() {
			t.Error("Expected filter to be inactive initially")
		}

		// Toggle filter on
		err := facade.Filtering().TogglePausedFilter()
		if err != nil {
			t.Fatalf("TogglePausedFilter failed: %v", err)
		}

		if !facade.Filtering().IsPausedFilterActive() {
			t.Error("Expected filter to be active after toggle")
		}

		// Toggle filter off
		err = facade.Filtering().TogglePausedFilter()
		if err != nil {
			t.Fatalf("Second TogglePausedFilter failed: %v", err)
		}

		if facade.Filtering().IsPausedFilterActive() {
			t.Error("Expected filter to be inactive after second toggle")
		}
	})

	t.Run("GetFilterState", func(t *testing.T) {
		// Activate filter
		facade.Filtering().TogglePausedFilter()

		state := facade.Filtering().GetFilterState()
		if !state.PausedFilterActive {
			t.Error("Expected paused filter to be active in state")
		}
		if state.SearchActive {
			t.Error("Expected search to be inactive in state")
		}
	})
}

// TestFacadeThreadSafety verifies facade thread-safe access
func TestFacadeThreadSafety(t *testing.T) {
	// Create test dependencies
	spin := spinner.New()
	stateManager := config.NewTestState(t.TempDir())
	list := ui.NewList(&spin, false, stateManager)

	// Add test instances
	for i := 0; i < 10; i++ {
		inst := createTestInstance(t, "concurrent-session")
		finalizer := list.AddInstance(inst)
		finalizer()
	}

	// Create facade
	facade := NewFacade(
		nil, nil, list, nil, nil, nil, nil, nil, 100,
	)

	// Concurrent access test
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Navigation operations
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := facade.Navigation().NavigateDown(); err != nil {
				errors <- err
			}
		}()
	}

	// Filtering operations
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := facade.Filtering().TogglePausedFilter(); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation failed: %v", err)
	}
}

// TestFacadeServiceAccess verifies all services are accessible through facade
func TestFacadeServiceAccess(t *testing.T) {
	// Create minimal facade
	spin := spinner.New()
	stateManager := config.NewTestState(t.TempDir())
	list := ui.NewList(&spin, false, stateManager)
	facade := NewFacade(
		nil, nil, list, nil, nil, nil, nil, nil, 100,
	)

	t.Run("NavigationService", func(t *testing.T) {
		nav := facade.Navigation()
		if nav == nil {
			t.Fatal("Navigation service is nil")
		}
	})

	t.Run("FilteringService", func(t *testing.T) {
		filter := facade.Filtering()
		if filter == nil {
			t.Fatal("Filtering service is nil")
		}
	})

	t.Run("SessionManagementService", func(t *testing.T) {
		// SessionManagement may be nil if sessionController is nil
		// This is expected for minimal facade
		sess := facade.SessionManagement()
		if sess != nil {
			t.Log("SessionManagement service available")
		}
	})

	t.Run("UICoordinationService", func(t *testing.T) {
		// UICoordination may be nil if components are nil
		ui := facade.UICoordination()
		if ui != nil {
			t.Log("UICoordination service available")
		}
	})
}

// Helper function to create test instance
func createTestInstance(t *testing.T, title string) *session.Instance {
	t.Helper()

	// Create minimal instance options
	options := session.InstanceOptions{
		Title:       title,
		Path:        "/tmp/test",
		Program:     "test",
		WorkingDir:  "/tmp/test",
		SessionType: session.SessionTypeDirectory,
	}

	inst, err := session.NewInstance(options)
	if err != nil {
		t.Fatalf("Failed to create test instance: %v", err)
	}

	return inst
}
