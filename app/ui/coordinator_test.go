package ui

import (
	"context"
	"testing"
)

func TestCoordinatorInitialization(t *testing.T) {
	coordinator := NewCoordinator()

	if coordinator == nil {
		t.Fatal("Expected coordinator to be created")
	}

	// Test context setting
	ctx := context.Background()
	coordinator.SetContext(ctx)

	// Test initialization
	err := coordinator.Initialize()
	if err != nil {
		t.Fatalf("Expected no error during initialization, got: %v", err)
	}

	// Test registry access
	registry := coordinator.GetRegistry()
	if registry == nil {
		t.Fatal("Expected registry to be available")
	}

	// Test component initialization
	err = coordinator.InitializeComponents(false, nil)
	if err != nil {
		t.Fatalf("Expected no error during component initialization, got: %v", err)
	}
}

func TestCoordinatorComponentManagement(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.Initialize()
	coordinator.InitializeComponents(false, nil)

	// Test component retrieval
	menu := coordinator.GetComponentByType(ComponentMenu)
	if menu == nil {
		t.Error("Expected menu component to be available")
	}

	errBox := coordinator.GetComponentByType(ComponentErrBox)
	if errBox == nil {
		t.Error("Expected error box component to be available")
	}
}

func TestCoordinatorOverlayManagement(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.Initialize()

	// Test that no overlay is active initially
	_, active := coordinator.GetActiveOverlay()
	if active {
		t.Error("Expected no active overlay initially")
	}

	// Test showing an overlay
	err := coordinator.ShowOverlay(ComponentTextInputOverlay)
	if err != nil {
		t.Fatalf("Expected no error showing overlay, got: %v", err)
	}

	// Test that overlay is now active
	activeOverlay, active := coordinator.GetActiveOverlay()
	if !active {
		t.Error("Expected overlay to be active after showing")
	}
	if activeOverlay != ComponentTextInputOverlay {
		t.Errorf("Expected active overlay to be TextInputOverlay, got %v", activeOverlay)
	}

	// Test hiding the overlay
	err = coordinator.HideOverlay(ComponentTextInputOverlay)
	if err != nil {
		t.Fatalf("Expected no error hiding overlay, got: %v", err)
	}

	// Test that no overlay is active after hiding
	_, active = coordinator.GetActiveOverlay()
	if active {
		t.Error("Expected no active overlay after hiding")
	}
}

func TestCoordinatorLayoutManagement(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.Initialize()

	// Test layout updates
	coordinator.UpdateLayout(100, 50)
	coordinator.HandleResize(120, 60)

	// These should not cause any errors
	mainView := coordinator.RenderMain()
	if mainView == "" {
		t.Error("Expected main view to render something")
	}

	layoutView := coordinator.RenderWithLayout()
	if layoutView == "" {
		t.Error("Expected layout view to render something")
	}
}