package detection

import "testing"

func TestIsOSCExecutingPromotable(t *testing.T) {
	t.Parallel()
	want := map[DetectedStatus]bool{
		StatusReady:           true,
		StatusUnknown:         true,
		StatusIdle:            true,
		StatusProcessing:      true,
		StatusNeedsApproval:   false,
		StatusInputRequired:   false,
		StatusError:           false,
		StatusTestsFailing:    false,
		StatusExecuting:       false,
		StatusSuccess:         false,
		StatusWaitingForAgent: false,
		StatusCompacting:      false,
	}
	for status, want := range want {
		if got := IsOSCExecutingPromotable(status); got != want {
			t.Errorf("IsOSCExecutingPromotable(%v) = %v, want %v", status, got, want)
		}
	}
}

func TestIsOSCIdlePromotable(t *testing.T) {
	t.Parallel()
	want := map[DetectedStatus]bool{
		StatusReady:           true,
		StatusUnknown:         true,
		StatusProcessing:      false,
		StatusNeedsApproval:   false,
		StatusInputRequired:   false,
		StatusError:           false,
		StatusTestsFailing:    false,
		StatusIdle:            false,
		StatusExecuting:       false,
		StatusSuccess:         false,
		StatusWaitingForAgent: false,
		StatusCompacting:      false,
	}
	for status, want := range want {
		if got := IsOSCIdlePromotable(status); got != want {
			t.Errorf("IsOSCIdlePromotable(%v) = %v, want %v", status, got, want)
		}
	}
}
