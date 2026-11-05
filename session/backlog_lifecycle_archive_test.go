package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// sessDisableGates stands in for an operator satisfying (or removing) every
// configured gate: the next decision finds none.
func sessDisableGates(t *testing.T, h *gateHarness) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, h.repo.client.TransitionGate.Update().SetEnabled(false).Exec(ctx))
	require.NoError(t, h.cfg.InvalidateCache(ctx))
}
