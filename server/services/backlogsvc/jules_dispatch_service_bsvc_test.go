package backlogsvc

import (
	"context"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// createTestJulesReadyItemForRPC is createTestJulesReadyItem plus
// SkipPlanning: true — the RPC-level tests below exercise the real
// *BacklogService.transitionWithGuard (not fakeJulesTransitionGuard), which
// additionally enforces "plan approved or skip_planning" before allowing the
// in_progress transition; that gate is orthogonal to what Epic 2.4.3 tests,
// so it's satisfied here rather than exercised.
func createTestJulesReadyItemForRPC(t *testing.T, ctx context.Context, storage *session.Storage, title string) *backlogdto.BacklogItemData {
	t.Helper()
	repoPath := svctest.NewTestJulesRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:        title,
		Status:       string(domain.BacklogStatusReady),
		RepoPath:     repoPath,
		SkipPlanning: true,
	})
	require.NoError(t, err)
	return item
}

// newTestBacklogServiceWithJules builds a *BacklogService wired exactly like
// production (server/dependencies.go, Task 2.4.4a): the service itself is
// passed as JulesDispatchService's transitionGuard, since *BacklogService
// satisfies julesTransitionGuard structurally.
func newTestBacklogServiceWithJules(storage *session.Storage, cfg *config.Config, client services.JulesSessionCreator, sources *jules.JulesSourceRegistry) *BacklogService {
	svc := NewBacklogService(storage, nil, cfg, nil, nil, nil)
	dispatchSvc := services.NewJulesDispatchService(storage, svc, client, sources, func() *config.Config { return cfg })
	svc.SetJulesDispatcher(dispatchSvc)
	return svc
}

func TestBacklogService_DispatchToJules_should_ReturnItemSessionWithJulesWorkRole_When_RequestValid(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItemForRPC(t, ctx, storage, "rpc dispatch success item")
	cfg := svctest.NewTestJulesConfig(item.RepoPath)
	svc := newTestBacklogServiceWithJules(storage, cfg, &svctest.FakeJulesSessionCreator{}, svctest.NewTestJulesSourceRegistry())

	resp, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item.ID,
		Branch: "backlog/item-1",
		Prompt: "Fix the flaky poller test",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)
	assert.Equal(t, string(session.SessionRoleJulesWork), resp.Msg.ItemSession.SessionRole)
	assert.True(t, strings.HasPrefix(resp.Msg.ItemSession.SessionUuid, "jules-sessions/"),
		"session_uuid must start with jules-sessions/, got %q", resp.Msg.ItemSession.SessionUuid)

	refreshed, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusInProgress), refreshed.Status)
}

func TestBacklogService_DispatchToJules_should_ReturnInvalidArgument_When_BranchEmpty(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	ctx := t.Context()

	item := svctest.CreateTestJulesReadyItem(t, ctx, storage, "rpc missing branch item")
	cfg := svctest.NewTestJulesConfig(item.RepoPath)
	svc := newTestBacklogServiceWithJules(storage, cfg, &svctest.FakeJulesSessionCreator{}, svctest.NewTestJulesSourceRegistry())

	_, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item.ID,
		Branch: "",
		Prompt: "Fix the flaky poller test",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "pushed to GitHub")
}

func TestBacklogService_DispatchToJules_should_ReturnFailedPrecondition_When_ConcurrencyCeilingReached(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	ctx := t.Context()

	item1 := createTestJulesReadyItemForRPC(t, ctx, storage, "rpc concurrency cap item 1")
	item2 := createTestJulesReadyItemForRPC(t, ctx, storage, "rpc concurrency cap item 2")
	cfg := svctest.NewTestJulesConfig(item1.RepoPath)
	cfg.Jules.EgressAcknowledgedRepos = append(cfg.Jules.EgressAcknowledgedRepos, item2.RepoPath)
	cfg.Jules.MaxConcurrentJulesSessions = 1
	svc := newTestBacklogServiceWithJules(storage, cfg, &svctest.FakeJulesSessionCreator{}, svctest.NewTestJulesSourceRegistry())

	_, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item1.ID,
		Branch: "backlog/item-1",
		Prompt: "First dispatch fills the one slot",
	}))
	require.NoError(t, err, "first dispatch must succeed and consume the only slot")

	_, err = svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item2.ID,
		Branch: "backlog/item-2",
		Prompt: "Second dispatch must be rejected by the cap",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestBacklogService_DispatchToJules_should_ReturnFailedPreconditionPointingAtSettings_When_JulesDisabled(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	ctx := t.Context()

	item := svctest.CreateTestJulesReadyItem(t, ctx, storage, "rpc disabled item")
	cfg := svctest.NewTestJulesConfig(item.RepoPath)
	cfg.Jules.Enabled = false
	svc := newTestBacklogServiceWithJules(storage, cfg, &svctest.FakeJulesSessionCreator{}, svctest.NewTestJulesSourceRegistry())

	_, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item.ID,
		Branch: "backlog/item-1",
		Prompt: "Should be rejected because Jules is disabled",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "Settings")
	assert.Contains(t, err.Error(), "Jules")
}

func TestBacklogService_DispatchToJules_should_ReturnFailedPreconditionDirectingToDispatchDialog_When_RepoNotAcknowledged(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	ctx := t.Context()

	item := svctest.CreateTestJulesReadyItem(t, ctx, storage, "rpc unacknowledged repo item")
	cfg := &config.Config{Jules: config.JulesConfig{
		Enabled:                    true,
		MaxConcurrentJulesSessions: 2,
		MaxJulesSessionsPerDay:     15,
		// EgressAcknowledgedRepos deliberately left empty — item.RepoPath was
		// never confirmed via ConfirmEgressConsent.
	}}
	svc := newTestBacklogServiceWithJules(storage, cfg, &svctest.FakeJulesSessionCreator{}, svctest.NewTestJulesSourceRegistry())

	_, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item.ID,
		Branch: "backlog/item-1",
		Prompt: "Should be rejected — repo not acknowledged for cloud egress",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "dispatch dialog")
}
