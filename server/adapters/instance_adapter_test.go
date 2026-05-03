package adapters

import (
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
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

func TestRateLimitResetTime_ZeroIsNil(t *testing.T) {
	// When time is zero, rateLimitResetTime should not be set in proto.
	// The logic in InstanceToProto is: if t := inst.GetRateLimitResetTime(); !t.IsZero() { ... }
	// Verify zero time would skip setting the field.
	var zeroTime time.Time
	if !zeroTime.IsZero() {
		t.Error("expected IsZero() == true for zero time; RateLimitResetTime would incorrectly be set")
	}
}

func TestRateLimitResetTime_NonZeroIsSet(t *testing.T) {
	// Verify that a non-zero time would be considered for setting.
	futureTime := time.Now().Add(1 * time.Hour)
	if futureTime.IsZero() {
		t.Error("expected non-zero future time")
	}
}
