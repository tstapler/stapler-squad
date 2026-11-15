package backlogsvc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// admissionScenario seeds an item and the sessions around it, then calls
// spawnSessionAfterGates directly so each case pins which admission check wins
// when several would refuse the same spawn.
type admissionScenario struct {
	name           string
	unapprovedPlan bool
	autonomous     bool
	isReopen       bool
	noRepoPath     bool
	noCreator      bool
	noopOpen       bool
	activeWork     bool
	endedWork      int
	endedWorkLive  bool // newest ended round is confirmed live via the stopper, kill ineffective
	activeJules    bool

	wantCode           connect.Code
	wantContains       string
	wantNotKilledPanes bool
}

func TestSpawnSessionAfterGates_should_ApplyAdmissionChecksInOrder(t *testing.T) {
	t.Parallel()
	tests := []admissionScenario{
		{name: "planning gate beats missing repo and creator", unapprovedPlan: true, noRepoPath: true, noCreator: true,
			wantCode: connect.CodeFailedPrecondition, wantContains: "approve the plan"},
		{name: "autonomous skips planning; missing repo beats missing creator", unapprovedPlan: true, autonomous: true, noRepoPath: true, noCreator: true,
			wantCode: connect.CodeFailedPrecondition, wantContains: "repo_path"},
		{name: "missing creator beats session-dependent gates", noCreator: true, noopOpen: true, activeWork: true,
			wantCode: connect.CodeUnimplemented, wantContains: "SessionCreator"},
		{name: "no-op loop gate beats active session and runs before pane kills", noopOpen: true, activeWork: true, endedWork: 1,
			wantCode: connect.CodeFailedPrecondition, wantContains: "repeated_noop_dispatch", wantNotKilledPanes: true},
		{name: "active work session beats confirmed-live, jules and cap", activeWork: true, endedWork: 5, endedWorkLive: true, activeJules: true, isReopen: true,
			wantCode: connect.CodeAlreadyExists, wantContains: "work"},
		{name: "confirmed-live round beats jules and cap", endedWork: 5, endedWorkLive: true, activeJules: true, isReopen: true,
			wantCode: connect.CodeAlreadyExists, wantContains: "confirmed live"},
		{name: "jules beats rework cap", endedWork: 5, activeJules: true, isReopen: true,
			wantCode: connect.CodeAlreadyExists, wantContains: "Jules"},
		{name: "rework cap refuses a reopen at the cap", endedWork: 5, isReopen: true,
			wantCode: connect.CodeFailedPrecondition, wantContains: "rework cap"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runAdmissionScenario(t, tc)
		})
	}
}

func runAdmissionScenario(t *testing.T, tc admissionScenario) {
	t.Helper()
	storage := svctest.CreateTestStorage(t)
	ctx := t.Context()

	var creator SessionCreator
	if !tc.noCreator {
		creator = &mockSessionCreator{}
	}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	if tc.noRepoPath {
		repoPath = ""
	}
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title: "admission order", Status: string(domain.BacklogStatusReady), RepoPath: repoPath,
		PlanApproved: !tc.unapprovedPlan,
	})
	require.NoError(t, err)

	stopper := seedAdmissionSessions(t, ctx, storage, item.ID, tc)
	svc.SetSessionStopper(stopper)
	if tc.noopOpen {
		_, err = storage.MarkStuck(ctx, item.ID, domain.StuckReasonRepeatedNoopDispatch, domain.BacklogStatus(item.Status), "noop loop")
		require.NoError(t, err)
	}

	_, err = svc.spawnSessionAfterGates(ctx, item, tc.isReopen, tc.autonomous)
	require.Error(t, err)
	assert.Equal(t, tc.wantCode, connect.CodeOf(err), err.Error())
	assert.Contains(t, err.Error(), tc.wantContains)
	if tc.wantNotKilledPanes {
		assert.Empty(t, stopper.KilledPaneUUIDs, "a refusal by the no-op gate must precede the ended-round pane kills")
	}
}

func seedAdmissionSessions(t *testing.T, ctx context.Context, storage *session.Storage, itemID string, tc admissionScenario) *svctest.MockSessionStopper {
	stopper := &svctest.MockSessionStopper{LiveUUIDs: map[string]bool{}}
	t.Helper()
	for i := 1; i <= tc.endedWork; i++ {
		uuid := fmt.Sprintf("ended-round-%d", i)
		is, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: itemID, SessionUUID: uuid, SessionRole: instance.SessionRoleWork})
		require.NoError(t, err)
		require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))
		if tc.endedWorkLive && i == tc.endedWork {
			stopper.LiveUUIDs[uuid] = true
			stopper.KillIneffectiveUUIDs = map[string]bool{uuid: true}
		}
	}
	if tc.activeWork {
		_, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: itemID, SessionUUID: "active-work", SessionRole: instance.SessionRoleWork})
		require.NoError(t, err)
		stopper.LiveUUIDs["active-work"] = true
	}
	if tc.activeJules {
		_, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: itemID, SessionUUID: "jules-sessions/x", SessionRole: instance.SessionRoleJulesWork})
		require.NoError(t, err)
	}
	return stopper
}
