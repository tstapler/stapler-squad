package backlogsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

func nudgeRows(t *testing.T, storage *session.Storage, itemID string) []session.FlowMarkerData {
	t.Helper()
	rows, err := storage.ListFlowMarkersForItem(t.Context(), itemID, session.FlowMarkerManualNudge,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	return rows
}

func TestTransitionBacklogItemStatus_should_RecordManualNudge_When_ActorIsUser(t *testing.T) {
	svc, storage, _, _ := enfService(t)
	item := enfItemAt(t, storage, session.BacklogStatusIdea)

	require.NoError(t, enfTransition(t.Context(), svc, item.ID, session.BacklogStatusRefining, ""))

	rows := nudgeRows(t, storage, item.ID)
	require.Len(t, rows, 1)
	assert.Equal(t, session.ManualNudgeStatusTransition, rows[0].Value)
}

func TestTransitionBacklogItemStatus_should_NotRecordManualNudge_When_ActorIsAgentOrSystem(t *testing.T) {
	for _, actor := range []string{session.TriggeredByAgent, session.TriggeredBySystem} {
		t.Run(actor, func(t *testing.T) {
			svc, storage, _, _ := enfService(t)
			item := enfItemAt(t, storage, session.BacklogStatusIdea)

			require.NoError(t, enfTransition(services.WithTransitionActor(t.Context(), actor), svc, item.ID, session.BacklogStatusRefining, ""))

			assert.Empty(t, nudgeRows(t, storage, item.ID))
		})
	}
}

// Only human-driven RPC handlers may record a manual nudge; PR-fix, wake and
// every other automated steer must stay off this list.
func TestManualNudgeRecorders_should_BeCalledOnlyFromHumanDrivenHandlers(t *testing.T) {
	t.Parallel()
	allowed := map[string]bool{
		"terminal_service.go":          true, // WriteToSession RPC
		"session_service_lifecycle.go": true, // RestartSession RPC
		"github_user_nudge.go":         true, // "Ask to fix" NudgeSessionForPR RPC
		"backlog_service_lifecycle.go": true, // TransitionBacklogItemStatus, user actor only
	}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		if strings.Contains(string(b), ".RecordManualNudgeFor") {
			assert.Truef(t, allowed[f], "%s records manual nudges but is not a human-driven handler", f)
		}
	}
}
