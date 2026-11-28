package backlogsvc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/markerstore"
)

func nudgeRows(t *testing.T, storage *session.Storage, itemID string) []markerstore.FlowMarkerData {
	t.Helper()
	rows, err := storage.ListFlowMarkersForItem(t.Context(), itemID, markerstore.FlowMarkerManualNudge,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	return rows
}

func TestTransitionBacklogItemStatus_should_RecordManualNudge_When_ActorIsUser(t *testing.T) {
	svc, storage, _, _ := enfService(t)
	item := enfItemAt(t, storage, domain.BacklogStatusIdea)

	require.NoError(t, enfTransition(t.Context(), svc, item.ID, domain.BacklogStatusRefining, ""))

	rows := nudgeRows(t, storage, item.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, markerstore.ManualNudgeStatusTransition, rows[0].Value)
}

func TestTransitionBacklogItemStatus_should_NotRecordManualNudge_When_ActorIsAgentOrSystem(t *testing.T) {
	for _, actor := range []string{instance.TriggeredByAgent, instance.TriggeredBySystem} {
		t.Run(actor, func(t *testing.T) {
			svc, storage, _, _ := enfService(t)
			item := enfItemAt(t, storage, domain.BacklogStatusIdea)

			require.NoError(t, enfTransition(domain.WithTransitionActor(t.Context(), actor), svc, item.ID, domain.BacklogStatusRefining, ""))

			assert.Empty(t, nudgeRows(t, storage, item.ID))
		})
	}
}
