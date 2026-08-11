package cdp

import (
	"testing"
)

func TestCheckDependencies_ReturnsDepsResult(t *testing.T) {
	result := CheckDependencies()

	// The function should always return a DepsResult (never panic).
	// We cannot assert Available=true/false because Chrome may or may not
	// be installed in CI, but we can assert invariants:
	// - If Available, ChromePath must be non-empty.
	// - If not Available, Reason must be non-empty.
	if result.Available {
		if result.ChromePath == "" {
			t.Error("CheckDependencies: Available=true but ChromePath is empty")
		}
		if result.Reason != "" {
			t.Errorf("CheckDependencies: Available=true but Reason is %q (expected empty)", result.Reason)
		}
	} else {
		if result.ChromePath != "" {
			t.Errorf("CheckDependencies: Available=false but ChromePath is %q (expected empty)", result.ChromePath)
		}
		if result.Reason == "" {
			t.Error("CheckDependencies: Available=false but Reason is empty")
		}
	}
}

func TestCheckDependencies_IsIdempotent(t *testing.T) {
	r1 := CheckDependencies()
	r2 := CheckDependencies()

	if r1.Available != r2.Available {
		t.Errorf("CheckDependencies not idempotent: first=%v second=%v", r1.Available, r2.Available)
	}
	if r1.ChromePath != r2.ChromePath {
		t.Errorf("CheckDependencies ChromePath changed: first=%q second=%q", r1.ChromePath, r2.ChromePath)
	}
}

func TestCDPStatus_String(t *testing.T) {
	cases := []struct {
		status CDPStatus
		want   string
	}{
		{CDPStatusUnspecified, "Unspecified"},
		{CDPStatusWaiting, "Waiting"},
		{CDPStatusStreaming, "Streaming"},
		{CDPStatusNoBrowser, "NoBrowser"},
		{CDPStatusUnavailable, "Unavailable"},
		{CDPStatus(999), "Unknown"},
	}

	for _, tc := range cases {
		got := tc.status.String()
		if got != tc.want {
			t.Errorf("CDPStatus(%d).String() = %q, want %q", int(tc.status), got, tc.want)
		}
	}
}

func TestNew_ReturnsNoopWhenChromePathEmpty(t *testing.T) {
	mgr := New(CDPConfig{SessionID: "test", ChromePath: ""})
	if mgr == nil {
		t.Fatal("New returned nil manager")
	}
	state := mgr.State()
	if state.Status != CDPStatusUnavailable {
		t.Errorf("noop manager State().Status = %v, want CDPStatusUnavailable", state.Status)
	}
	if mgr.Port() != 0 {
		t.Errorf("noop manager Port() = %d, want 0", mgr.Port())
	}
	if mgr.WrapperDir() != "" {
		t.Errorf("noop manager WrapperDir() = %q, want empty", mgr.WrapperDir())
	}
	if mgr.LatestFrame() != nil {
		t.Error("noop manager LatestFrame() should return nil")
	}
	// Allocate and Start should not error on noop.
	if err := mgr.Allocate(); err != nil {
		t.Errorf("noop manager Allocate() returned error: %v", err)
	}
	if err := mgr.Start(t.Context()); err != nil {
		t.Errorf("noop manager Start() returned error: %v", err)
	}
	// DispatchInput should not error on noop.
	if err := mgr.DispatchInput([]byte(`{"method":"Input.dispatchMouseEvent","params":{}}`)); err != nil {
		t.Errorf("noop manager DispatchInput() returned error: %v", err)
	}
	// Stop should not panic.
	mgr.Stop()
}
