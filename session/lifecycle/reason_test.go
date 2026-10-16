package lifecycle_test

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/lifecycle"
)

func TestReason_UnhandledValue_FailsSafe(t *testing.T) {
	for _, r := range []lifecycle.Reason{lifecycle.Reason(999), lifecycle.Reason(-1)} {
		if got := r.ShouldContinue(); got != false {
			t.Errorf("Reason(%d).ShouldContinue() = %v, want false", int(r), got)
		}
		if got := r.ShouldFireExitCallback(); got != true {
			t.Errorf("Reason(%d).ShouldFireExitCallback() = %v, want true", int(r), got)
		}
	}
}

func TestReason_ZeroValue_IsSafeDefault(t *testing.T) {
	var r lifecycle.Reason
	if got := r.ShouldContinue(); got != false {
		t.Errorf("zero-value Reason.ShouldContinue() = %v, want false", got)
	}
	if got := r.ShouldFireExitCallback(); got != true {
		t.Errorf("zero-value Reason.ShouldFireExitCallback() = %v, want true", got)
	}
}

func TestReason_String_AllNamedConstants(t *testing.T) {
	tests := []struct {
		reason lifecycle.Reason
		want   string
	}{
		{lifecycle.ReasonUnknown, "unknown"},
		{lifecycle.ReasonDeliberateClose, "deliberate_close"},
		{lifecycle.ReasonDeliberateSupersede, "deliberate_supersede"},
		{lifecycle.ReasonCleanExit, "clean_exit"},
		{lifecycle.ReasonTransportDrop, "transport_drop"},
		{lifecycle.ReasonReconnectExhausted, "reconnect_exhausted"},
	}
	for _, tt := range tests {
		t.Run(tt.reason.String(), func(t *testing.T) {
			if got := tt.reason.String(); got != tt.want {
				t.Errorf("Reason(%d).String() = %q, want %q", int(tt.reason), got, tt.want)
			}
		})
	}
}

func TestReason_KnownConstants_ShouldContinueAndShouldFireExitCallback_ReturnExpectedValues(t *testing.T) {
	tests := []struct {
		reason             lifecycle.Reason
		wantShouldContinue bool
		wantFireExit       bool
	}{
		{lifecycle.ReasonUnknown, false, true},
		{lifecycle.ReasonDeliberateClose, false, false},
		{lifecycle.ReasonDeliberateSupersede, false, true},
		{lifecycle.ReasonCleanExit, false, true},
		{lifecycle.ReasonTransportDrop, true, true},
		{lifecycle.ReasonReconnectExhausted, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.reason.String(), func(t *testing.T) {
			if got := tt.reason.ShouldContinue(); got != tt.wantShouldContinue {
				t.Errorf("%v.ShouldContinue() = %v, want %v", tt.reason, got, tt.wantShouldContinue)
			}
			if got := tt.reason.ShouldFireExitCallback(); got != tt.wantFireExit {
				t.Errorf("%v.ShouldFireExitCallback() = %v, want %v", tt.reason, got, tt.wantFireExit)
			}
		})
	}
}
