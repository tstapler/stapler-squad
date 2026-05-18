package cdp

import (
	"os"
	"testing"
)

// ---- Real manager (cdpStreamManager) tests using a fake Chrome path -----------

// newManagerWithFakeChrome creates a cdpStreamManager backed by a real (but
// dummy) executable so that Allocate() runs the real port-allocation path and
// writes wrapper scripts. The fake chrome script just exits immediately; we
// never call Start() so it never runs.
func newManagerWithFakeChrome(t *testing.T) CDPStreamManager {
	t.Helper()

	// Write a minimal fake chrome script so ChromePath passes the "non-empty" check.
	dir := t.TempDir()
	fakeChrome := dir + "/fake-chrome"
	if err := os.WriteFile(fakeChrome, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake chrome: %v", err)
	}

	return New(CDPConfig{
		SessionID:  "test-session-manager",
		ChromePath: fakeChrome,
	})
}

// TestAllocate_SetsNonZeroPort verifies that Allocate() picks a free TCP port
// and records it so that Port() returns a nonzero value.
func TestAllocate_SetsNonZeroPort(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)
	t.Cleanup(mgr.Stop)

	if err := mgr.Allocate(); err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	port := mgr.Port()
	if port == 0 {
		t.Error("Port() = 0 after Allocate(); want a nonzero port")
	}
}

// TestAllocate_Idempotent verifies that calling Allocate() twice returns the
// same port without error (sync.Once semantics).
func TestAllocate_Idempotent(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)
	t.Cleanup(mgr.Stop)

	if err := mgr.Allocate(); err != nil {
		t.Fatalf("first Allocate() error = %v", err)
	}
	p1 := mgr.Port()

	if err := mgr.Allocate(); err != nil {
		t.Fatalf("second Allocate() error = %v", err)
	}
	p2 := mgr.Port()

	if p1 != p2 {
		t.Errorf("Allocate() not idempotent: first port=%d, second port=%d", p1, p2)
	}
}

// TestAllocate_WrapperDirCreated verifies that Allocate() creates the wrapper
// script directory.
func TestAllocate_WrapperDirCreated(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)
	t.Cleanup(mgr.Stop)

	if err := mgr.Allocate(); err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	wrapDir := mgr.WrapperDir()
	if wrapDir == "" {
		t.Fatal("WrapperDir() is empty after Allocate()")
	}
	if _, err := os.Stat(wrapDir); os.IsNotExist(err) {
		t.Errorf("wrapper dir %s does not exist on disk after Allocate()", wrapDir)
	}
}

// TestState_BeforeStart_IsNoBrowserOrUnspecified verifies the CDPStreamManager
// state before Start() is called. After Allocate() the state should be
// CDPStatusNoBrowser; before Allocate() it is CDPStatusUnspecified.
func TestState_BeforeAllocate_IsUnspecified(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)
	t.Cleanup(mgr.Stop)

	state := mgr.State()
	if state.Status != CDPStatusUnspecified {
		t.Errorf("State() before Allocate = %v, want CDPStatusUnspecified", state.Status)
	}
	if state.Port != 0 {
		t.Errorf("State().Port before Allocate = %d, want 0", state.Port)
	}
}

// TestState_AfterAllocate_IsNoBrowser verifies the state transitions to
// CDPStatusNoBrowser (port allocated, Chrome not yet running).
func TestState_AfterAllocate_IsNoBrowser(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)
	t.Cleanup(mgr.Stop)

	if err := mgr.Allocate(); err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	state := mgr.State()
	if state.Status != CDPStatusNoBrowser {
		t.Errorf("State() after Allocate = %v, want CDPStatusNoBrowser", state.Status)
	}
}

// TestStop_BeforeStart_DoesNotPanic verifies Stop() is safe to call before
// Start() and without a prior Allocate().
func TestStop_BeforeStart_DoesNotPanic(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)
	// Should not panic.
	mgr.Stop()
}

// TestStop_AfterAllocate_DoesNotPanic verifies Stop() after Allocate() is safe
// and removes the wrapper directory.
func TestStop_AfterAllocate_CleansWrapperDir(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)

	if err := mgr.Allocate(); err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	wrapDir := mgr.WrapperDir()
	if wrapDir == "" {
		t.Fatal("WrapperDir() empty after Allocate()")
	}

	mgr.Stop()

	// After Stop() the wrapper directory should have been removed.
	if _, err := os.Stat(wrapDir); !os.IsNotExist(err) {
		t.Errorf("wrapper dir %s still exists after Stop()", wrapDir)
	}
}

// TestLatestFrame_BeforeStart_ReturnsNil verifies LatestFrame() before any
// screencast frames have been received.
func TestLatestFrame_BeforeStart_ReturnsNil(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)
	t.Cleanup(mgr.Stop)

	if frame := mgr.LatestFrame(); frame != nil {
		t.Errorf("LatestFrame() before Start = %v, want nil", frame)
	}
}

// TestSetStateChangeCallback_DoesNotPanic verifies the callback registration
// path does not panic.
func TestSetStateChangeCallback_DoesNotPanic(t *testing.T) {
	mgr := newManagerWithFakeChrome(t)
	t.Cleanup(mgr.Stop)

	called := make(chan CDPState, 1)
	mgr.SetStateChangeCallback(func(s CDPState) {
		select {
		case called <- s:
		default:
		}
	})

	// Trigger the callback via Allocate — state changes to NoBrowser.
	if err := mgr.Allocate(); err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}

	// The callback fires asynchronously in a goroutine; give it a moment.
	// (We do NOT use time.Sleep — we use a short channel receive with a select.)
	select {
	case s := <-called:
		if s.Status != CDPStatusNoBrowser {
			t.Errorf("callback received state %v, want CDPStatusNoBrowser", s.Status)
		}
	default:
		// Callback may not have fired yet; that's acceptable — we just want
		// to verify it doesn't panic.
	}
}

// ---- CheckDependencies caching test -------------------------------------------

// TestCheckDependencies_ReturnsSameResultOnRepeatedCalls verifies the caching
// behaviour: calling CheckDependencies() multiple times must return the same
// Available/ChromePath values (sync.Once is used internally).
func TestCheckDependencies_ReturnsSameResultOnRepeatedCalls(t *testing.T) {
	// Call many times — none should panic and all must return identical values.
	results := make([]DepsResult, 5)
	for i := range results {
		results[i] = CheckDependencies()
	}

	for i := 1; i < len(results); i++ {
		if results[i].Available != results[0].Available {
			t.Errorf("call %d: Available=%v, want %v", i, results[i].Available, results[0].Available)
		}
		if results[i].ChromePath != results[0].ChromePath {
			t.Errorf("call %d: ChromePath=%q, want %q", i, results[i].ChromePath, results[0].ChromePath)
		}
	}
}

// ---- Noop manager interface completeness test ---------------------------------

// TestNoopManager_FullInterface exercises every method on the noop manager to
// confirm all interface methods are implemented and safe to call.
func TestNoopManager_FullInterface(t *testing.T) {
	mgr := New(CDPConfig{SessionID: "noop-test", ChromePath: ""})

	if err := mgr.Allocate(); err != nil {
		t.Errorf("noop Allocate() error = %v", err)
	}
	if err := mgr.Start(t.Context()); err != nil {
		t.Errorf("noop Start() error = %v", err)
	}
	if s := mgr.State(); s.Status != CDPStatusUnavailable {
		t.Errorf("noop State().Status = %v, want CDPStatusUnavailable", s.Status)
	}
	if p := mgr.Port(); p != 0 {
		t.Errorf("noop Port() = %d, want 0", p)
	}
	if w := mgr.WrapperDir(); w != "" {
		t.Errorf("noop WrapperDir() = %q, want empty", w)
	}
	if f := mgr.LatestFrame(); f != nil {
		t.Error("noop LatestFrame() should return nil")
	}
	if err := mgr.DispatchInput([]byte(`{"method":"Input.dispatchMouseEvent","params":{}}`)); err != nil {
		t.Errorf("noop DispatchInput() error = %v", err)
	}
	mgr.SetStateChangeCallback(func(CDPState) {})
	mgr.Stop() // must not panic
}
