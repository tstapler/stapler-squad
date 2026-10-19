package wakes

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/domain"
)

func TestDecide_should_FollowTheOwnerStateTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		allowRevive bool
		owner       OwnerFacts
		want        Verdict
	}{
		{"live owner delivers", false, OwnerFacts{Exists: true, Live: true}, Verdict{Kind: VerdictDeliver}},
		{"deleted owner is skipped", true, OwnerFacts{}, Verdict{VerdictSkip, domain.WakeReasonOwnerMissing}},
		{"archived beats live and allow_revive", true, OwnerFacts{Exists: true, Live: true, Archived: true}, Verdict{VerdictSkip, domain.WakeReasonArchived}},
		{"one-shot is skipped", true, OwnerFacts{Exists: true, Live: true, OneShot: true}, Verdict{VerdictSkip, domain.WakeReasonOneShot}},
		{"terminal item is skipped", false, OwnerFacts{Exists: true, Live: true, ItemTerminal: true}, Verdict{VerdictSkip, domain.WakeReasonItemTerminal}},
		{"stopped with allow_revive needs revive", true, OwnerFacts{Exists: true}, Verdict{Kind: VerdictNeedsRevive}},
		{"stopped without allow_revive is owner_stopped, never needs_revive", false, OwnerFacts{Exists: true}, Verdict{VerdictSkip, domain.WakeReasonOwnerStopped}},
		{"archived stopped owner is archived even with allow_revive", true, OwnerFacts{Exists: true, Archived: true}, Verdict{VerdictSkip, domain.WakeReasonArchived}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Decide(tt.allowRevive, tt.owner))
		})
	}
}

func TestVerdict_should_FailClosed_When_ZeroValue(t *testing.T) {
	t.Parallel()
	assert.Equal(t, VerdictWait, Verdict{}.Kind)
}
