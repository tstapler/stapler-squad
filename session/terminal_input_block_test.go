package session

import (
	"testing"
	"time"
)

func TestTerminalInputBlock(t *testing.T) {
	t.Parallel()
	archived := time.Now()
	cases := []struct {
		name                   string
		snap                   InstanceSnapshot
		wantBlocked, wantRecov bool
	}{
		{"tymux active", InstanceSnapshot{Backend: BackendTymux, Status: Active}, false, false},
		{"tymux permanently failed", InstanceSnapshot{Backend: BackendTymux, Status: PermanentlyFailed}, true, true},
		{"tymux paused", InstanceSnapshot{Backend: BackendTymux, Status: Paused}, true, true},
		{"tmux stopped still forwards input", InstanceSnapshot{Backend: BackendTmux, Status: Stopped}, false, false},
		{"archived never recovers", InstanceSnapshot{Backend: BackendTymux, Status: Active, ArchivedAt: &archived}, true, false},
		{"hidden never recovers", InstanceSnapshot{Backend: BackendTymux, Status: Stopped, Hidden: true}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocked, recoverable := TerminalInputBlock(&tc.snap)
			if blocked != tc.wantBlocked || recoverable != tc.wantRecov {
				t.Fatalf("got blocked=%v recoverable=%v, want %v %v", blocked, recoverable, tc.wantBlocked, tc.wantRecov)
			}
		})
	}
}
