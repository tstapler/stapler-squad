package adapters

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
)

func TestRateLimitStateToProto_AllStates(t *testing.T) {
	tests := []struct {
		name     string
		input    ratelimit.RateLimitState
		expected sessionv1.RateLimitState
	}{
		{"None", ratelimit.StateNone, sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE},
		{"Waiting", ratelimit.StateWaiting, sessionv1.RateLimitState_RATE_LIMIT_STATE_WAITING},
		{"Recovering", ratelimit.StateRecovering, sessionv1.RateLimitState_RATE_LIMIT_STATE_RECOVERING},
		{"Recovered", ratelimit.StateRecovered, sessionv1.RateLimitState_RATE_LIMIT_STATE_RECOVERED},
		{"Failed", ratelimit.StateFailed, sessionv1.RateLimitState_RATE_LIMIT_STATE_FAILED},
		{"Unknown state defaults to None", ratelimit.RateLimitState(99), sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rateLimitStateToProto(tc.input)
			if got != tc.expected {
				t.Errorf("rateLimitStateToProto(%v) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestInstanceToProto_NilReturnsNil(t *testing.T) {
	result := InstanceToProto(nil)
	if result != nil {
		t.Error("expected nil for nil input, got non-nil")
	}
}

// TestInstanceToProto_RateLimitEnabled verifies that the RateLimitEnabled field
// is populated correctly from the Instance struct field.
func TestInstanceToProto_RateLimitEnabled_DefaultTrue(t *testing.T) {
	inst := &session.Instance{} // nil RateLimitAutoResume → defaults to true
	proto := InstanceToProto(inst)
	if proto == nil {
		t.Fatal("expected non-nil proto for non-nil instance")
	}
	if !proto.RateLimitEnabled {
		t.Errorf("expected RateLimitEnabled=true (default), got false")
	}
}

func TestInstanceToProto_RateLimitEnabled_ExplicitFalse(t *testing.T) {
	disabled := false
	inst := &session.Instance{
		RateLimitAutoResume: &disabled,
	}
	proto := InstanceToProto(inst)
	if proto == nil {
		t.Fatal("expected non-nil proto for non-nil instance")
	}
	if proto.RateLimitEnabled {
		t.Errorf("expected RateLimitEnabled=false when explicitly disabled, got true")
	}
}

func TestInstanceToProto_RateLimitState_DefaultNone(t *testing.T) {
	inst := &session.Instance{} // no controller → state is None
	proto := InstanceToProto(inst)
	if proto == nil {
		t.Fatal("expected non-nil proto for non-nil instance")
	}
	if proto.RateLimitState != sessionv1.RateLimitState_RATE_LIMIT_STATE_NONE {
		t.Errorf("expected RATE_LIMIT_STATE_NONE for fresh instance, got %v", proto.RateLimitState)
	}
	if proto.RateLimitResetTime != nil {
		t.Errorf("expected nil RateLimitResetTime for fresh instance, got %v", proto.RateLimitResetTime)
	}
}
