package backlogsvc

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

type fakeStaleInspector struct{ result services.StaleWorkDisposition }

func (f fakeStaleInspector) InspectStaleWork(context.Context, string, *backlogdto.BacklogItemData) services.StaleWorkDisposition {
	return f.result
}

func TestRemediateStaleWorkSession_should_leaveSessionOpen_When_InspectorNudgesOrSkips(t *testing.T) {
	for name, disp := range map[string]services.StaleWorkDisposition{"nudged": services.StaleWorkNudged, "skipped": services.StaleWorkSkipped, "respawn": services.StaleWorkRespawn} {
		t.Run(name, func(t *testing.T) {
			storage := svctest.CreateTestStorage(t)
			creator := &mockSessionCreator{}
			svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
			stopper := &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{"stale-uuid": true}}
			svc.SetSessionStopper(stopper)
			svc.SetStaleWorkInspector(fakeStaleInspector{result: disp})
			repoPath := t.TempDir()
			svctest.InitGitRepoWithCommit(t, repoPath)
			itemID := createReadyItemForSpawn(t, svc, repoPath, "stale item "+name)
			_, err := storage.CreateItemSession(t.Context(), backlogdto.ItemSessionData{ItemID: itemID, SessionUUID: "stale-uuid", SessionRole: session.SessionRoleWork})
			require.NoError(t, err)
			_, err = storage.RawTransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusInProgress, nil, session.TriggeredBySystem)
			require.NoError(t, err)

			require.NoError(t, svc.RemediateStaleWorkSession(t.Context(), itemID))

			sessions, err := storage.ListItemSessions(t.Context(), itemID)
			require.NoError(t, err)
			restarted := disp == services.StaleWorkRespawn
			assert.Equal(t, restarted, slices.Contains(stopper.KilledPaneUUIDs, "stale-uuid"))
			assert.Equal(t, restarted, len(creator.calls) == 1)
			for _, is := range sessions {
				if is.SessionUUID == "stale-uuid" {
					assert.Equal(t, restarted, is.EndedAt != nil)
				}
			}
		})
	}
}
