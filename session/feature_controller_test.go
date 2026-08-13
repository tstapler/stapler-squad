package session

import (
	"context"
	"testing"
	"time"
)

// newTestBacklogController creates a BacklogController with a real (but minimal) storage
// and a real BacklogLifecycleListener, suitable for unit-testing Enable/Disable semantics.
// The caller is responsible for calling cleanup().
func newTestBacklogController(t *testing.T) (*BacklogController, func()) {
	t.Helper()
	storage, cleanup := createTestStorage(t)

	listener := NewBacklogLifecycleListener(storage)
	registry := NewPluginRegistry()

	ctrl := NewBacklogController(listener, storage, registry, nil)
	return ctrl, cleanup
}

// TestBacklogController_EnableIdempotent verifies that calling Enable twice does not
// start a second goroutine — the controller stays enabled and has exactly one syncLoop.
func TestBacklogController_EnableIdempotent(t *testing.T) {
	ctrl, cleanup := newTestBacklogController(t)
	defer cleanup()

	ctx := context.Background()

	if err := ctrl.Enable(ctx); err != nil {
		t.Fatalf("first Enable: %v", err)
	}
	if !ctrl.IsEnabled() {
		t.Error("IsEnabled should be true after Enable")
	}

	firstLoop := ctrl.syncLoop

	if err := ctrl.Enable(ctx); err != nil {
		t.Fatalf("second Enable: %v", err)
	}
	if !ctrl.IsEnabled() {
		t.Error("IsEnabled should still be true after second Enable")
	}
	if ctrl.syncLoop != firstLoop {
		t.Error("Enable called twice should not replace the running syncLoop")
	}

	// Cleanup: disable before the test ends so the goroutine is stopped.
	if err := ctrl.Disable(); err != nil {
		t.Errorf("Disable after test: %v", err)
	}
}

// TestBacklogController_DisableIdempotent verifies that calling Disable on an already-disabled
// controller does not panic or return an error.
func TestBacklogController_DisableIdempotent(t *testing.T) {
	ctrl, cleanup := newTestBacklogController(t)
	defer cleanup()

	// Controller starts disabled; Disable should be a no-op.
	if err := ctrl.Disable(); err != nil {
		t.Fatalf("Disable on disabled controller: %v", err)
	}
	if ctrl.IsEnabled() {
		t.Error("IsEnabled should be false after Disable")
	}

	// Second Disable is also a no-op.
	if err := ctrl.Disable(); err != nil {
		t.Fatalf("second Disable on disabled controller: %v", err)
	}
}

// TestBacklogController_EnableThenDisable verifies a full round-trip:
// Enable sets state to active, Disable brings it back to inactive.
func TestBacklogController_EnableThenDisable(t *testing.T) {
	ctrl, cleanup := newTestBacklogController(t)
	defer cleanup()

	ctx := context.Background()

	if err := ctrl.Enable(ctx); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !ctrl.IsEnabled() {
		t.Error("IsEnabled should be true after Enable")
	}
	if ctrl.syncLoop == nil {
		t.Error("syncLoop should be non-nil after Enable")
	}

	if err := ctrl.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if ctrl.IsEnabled() {
		t.Error("IsEnabled should be false after Disable")
	}
	if ctrl.syncLoop != nil {
		t.Error("syncLoop should be nil after Disable")
	}
}

// TestBacklogController_EnableAfterDisable verifies that re-enabling after a disable
// creates a fresh syncLoop and correctly reports IsEnabled as true.
func TestBacklogController_EnableAfterDisable(t *testing.T) {
	ctrl, cleanup := newTestBacklogController(t)
	defer cleanup()

	ctx := context.Background()

	// Enable → Disable → Enable.
	if err := ctrl.Enable(ctx); err != nil {
		t.Fatalf("first Enable: %v", err)
	}
	if err := ctrl.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if err := ctrl.Enable(ctx); err != nil {
		t.Fatalf("second Enable: %v", err)
	}

	if !ctrl.IsEnabled() {
		t.Error("IsEnabled should be true after re-Enable")
	}
	if ctrl.syncLoop == nil {
		t.Error("syncLoop should be non-nil after re-Enable")
	}

	// Cleanup.
	if err := ctrl.Disable(); err != nil {
		t.Errorf("final Disable: %v", err)
	}
}

// TestBacklogController_IsEnabled_ReflectsListenerState verifies that IsEnabled reads
// directly from the listener's atomic bool, so Enable/Disable are immediately visible.
func TestBacklogController_IsEnabled_ReflectsListenerState(t *testing.T) {
	ctrl, cleanup := newTestBacklogController(t)
	defer cleanup()

	ctx := context.Background()

	// Initially disabled.
	if ctrl.IsEnabled() {
		t.Error("controller should start disabled")
	}

	if err := ctrl.Enable(ctx); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	// IsEnabled should reflect the change immediately (atomic).
	deadline := time.Now().Add(100 * time.Millisecond)
	for !ctrl.IsEnabled() {
		if time.Now().After(deadline) {
			t.Fatal("IsEnabled did not become true within 100ms of Enable")
		}
	}

	if err := ctrl.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if ctrl.IsEnabled() {
		t.Error("IsEnabled should be false immediately after Disable")
	}
}
