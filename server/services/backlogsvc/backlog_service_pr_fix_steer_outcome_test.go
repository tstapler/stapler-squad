package backlogsvc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
)

// withPRNumber persists PrNumber, which CreateBacklogItem ignores.
func withPRNumber(t *testing.T, storage *session.Storage, itemID string, n int) {
	t.Helper()
	_, err := storage.UpdateBacklogItem(context.Background(), itemID, session.BacklogItemUpdate{PrNumber: &n}, nil)
	require.NoError(t, err)
}

// outcomeValues returns the pr_fix_outcome marker values (and PR numbers) stored for itemID.
func outcomeValues(t *testing.T, storage *session.Storage, itemID string) (values []string, prs []int) {
	t.Helper()
	rows, err := storage.ListFlowMarkersForItem(context.Background(), itemID, session.FlowMarkerPRFixOutcome,
		time.Unix(0, 0), time.Now().Add(time.Hour))
	require.NoError(t, err)
	for _, m := range rows {
		values = append(values, m.Value)
		prs = append(prs, m.PRNumber)
	}
	return values, prs
}

func TestSteerOutcome_should_RecordSteered_When_SteerDelivered(t *testing.T) {
	t.Parallel()
	svc, _, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
	withPRNumber(t, svc.storage, itemID, 42)

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	values, prs := outcomeValues(t, svc.storage, itemID)
	assert.Equal(t, []string{session.PRFixOutcomeSteered}, values)
	assert.Equal(t, []int{42}, prs)
}

func TestSteerOutcome_should_RecordSkippedActiveOnce_When_DegradedToRespawnBlocked(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	itemID := createPRPendingItemWithActiveSession(t, storage, activeSessionUUIDPrimary)
	withPRNumber(t, storage, itemID, 42)
	// No SessionSteerer wired -> degradeToRespawnBlocked.

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	values, prs := outcomeValues(t, storage, itemID)
	assert.Equal(t, []string{session.PRFixOutcomeSkippedActive}, values)
	assert.Equal(t, []int{42}, prs)
}

func TestSteerOutcome_should_RecordNothing_When_SteerDeliveryFails(t *testing.T) {
	t.Parallel()
	svc, steerer, _ := newTestBacklogServiceForSteerIntegration(t, activeSessionUUIDPrimary, "claude")
	itemID := createPRPendingItemWithActiveSession(t, svc.storage, activeSessionUUIDPrimary)
	steerer.steerErr[activeSessionUUIDPrimary] = errors.New("pty closed")

	require.NoError(t, svc.AutoReopenForPRFix(context.Background(), itemID, ciOnlyFixContext))

	values, _ := outcomeValues(t, svc.storage, itemID)
	assert.Empty(t, values)
}

func TestSteerOutcome_should_NotPanic_When_StorageNil(t *testing.T) {
	t.Parallel()
	svc := &BacklogService{}
	svc.recordPRFixOutcome(context.Background(), "item", session.PRFixOutcomeSteered)
}
