package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestWithSessionPRHolds_should_FillHoldsForNonTerminalPRs_OnlyWhenScopeOn(t *testing.T) {
	t.Parallel()
	inst := &session.Instance{Title: "holds", UUID: "uuid-holds", GitHubOwner: "acme", GitHubRepo: "app"}
	for _, p := range []struct {
		n     int
		state domain.PRState
	}{{11, domain.PRStateMerged}, {12, domain.PRStateOpen}, {13, domain.PRStateUnknown}} {
		ref, err := domain.NewPRRef("", "acme", "app", p.n)
		require.NoError(t, err)
		require.NoError(t, inst.AttachPR(ref, domain.AttachSourceBranchDiscovery, domain.PRInitial{State: p.state, Terminal: p.state == domain.PRStateMerged}))
	}

	off := withSessionPRHoldsScoped(&sessionv1.Session{}, inst, false)
	assert.Empty(t, off.PrHolds, "scope off: today's response")

	on := withSessionPRHoldsScoped(&sessionv1.Session{}, inst, true)
	kinds := map[int32]sessionv1.PrHold_Kind{}
	for _, h := range on.PrHolds {
		assert.Equal(t, "uuid-holds", h.OwnerSessionId)
		kinds[h.Pr.Number] = h.Kind
	}
	assert.Equal(t, map[int32]sessionv1.PrHold_Kind{12: sessionv1.PrHold_KIND_OPEN_PR, 13: sessionv1.PrHold_KIND_UNKNOWN}, kinds)
}
