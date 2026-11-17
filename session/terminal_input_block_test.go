package session

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"testing"
	"time"
)

func TestTerminalInputBlock(t *testing.T) {
	t.Parallel()
	archived := time.Now()
	cases := []struct {
		name                   string
		snap                   domain.InstanceSnapshot
		wantBlocked, wantRecov bool
	}{
		{"tymux active", domain.InstanceSnapshot{Backend: domain.BackendTymux, Status: domain.Active}, false, false},
		{"tymux permanently failed", domain.InstanceSnapshot{Backend: domain.BackendTymux, Status: domain.PermanentlyFailed}, true, true},
		{"tymux paused", domain.InstanceSnapshot{Backend: domain.BackendTymux, Status: domain.Paused}, true, true},
		{"tmux stopped still forwards input", domain.InstanceSnapshot{Backend: domain.BackendTmux, Status: domain.Stopped}, false, false},
		{"archived recovers on unarchive", domain.InstanceSnapshot{Backend: domain.BackendTymux, Status: domain.Active, ArchivedAt: &archived}, true, true},
		{"hidden never recovers", domain.InstanceSnapshot{Backend: domain.BackendTymux, Status: domain.Stopped, Hidden: true}, true, false},
		{"hidden and archived never recovers", domain.InstanceSnapshot{Backend: domain.BackendTymux, Status: domain.Active, Hidden: true, ArchivedAt: &archived}, true, false},
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
