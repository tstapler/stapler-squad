package services

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/instance"
)

// prWiringRunner answers the push and gh subprocesses of CreatePullRequest.
type prWiringRunner struct{}

func (prWiringRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	if filepath.Base(name) != "gh" || len(args) < 2 || args[0] != "pr" {
		return nil, nil
	}
	if args[1] == "create" {
		return []byte("https://github.com/acme/app/pull/77\n"), nil
	}
	return nil, exec.ErrNotFound
}

func (prWiringRunner) Start(context.Context, string, string, ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	return nil, nil, nil, exec.ErrNotFound
}

func (prWiringRunner) IsRemote() bool { return false }

type prWiringClient string

func (c prWiringClient) CallBlocking(context.Context, headless.FeatureKey, string, string, headless.CallOptions, headless.CostSink) (string, error) {
	return string(c), nil
}

func TestSessionService_should_WirePRCreationService_When_Constructed(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	ctx := context.Background()

	dir := svctest.NewDraftPRTestRepo(t)
	svctest.RunGit(t, dir, "checkout", "-q", "-b", "feature/wiring")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "change.txt"), []byte("data"), 0o600))
	svctest.RunGit(t, dir, "add", ".")
	svctest.RunGit(t, dir, "commit", "-q", "-m", "a change")
	inst := &instance.Instance{Title: "pr-wiring", UUID: uuid.NewString()}
	inst.SetGitWorktree(git.NewGitWorktreeFromStorage(dir, dir, "pr-wiring", "feature/wiring", ""))
	svctest.AddInstanceToPoller(fix.poller, inst)

	draftBody := func() string {
		resp, err := fix.svc.DraftPullRequest(ctx, connect.NewRequest(&sessionv1.DraftPullRequestRequest{SessionId: inst.UUID}))
		require.NoError(t, err)
		return resp.Msg.Body
	}
	assert.Contains(t, draftBody(), "No diff description available", "no pool yet")

	runner := headless.NewFakeRunner(`{"type":"result","session_id":"s","result":"from the pool"}`)
	fix.svc.SetHeadlessPool(headless.NewPoolWithRunner(headless.PoolConfig{}, runner))
	assert.Equal(t, "from the pool", draftBody())

	fix.svc.SetHeadlessClient(prWiringClient("from the client"))
	assert.Equal(t, "from the client", draftBody())

	item, err := fix.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "linked", Status: string(domain.BacklogStatusReview)})
	require.NoError(t, err)
	_, err = fix.storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: inst.UUID, SessionRole: instance.SessionRoleWork})
	require.NoError(t, err)
	listener := backlog.NewBacklogLifecycleListener(fix.storage)
	listener.SetEnabled(true)
	fix.svc.SetBacklogLifecycleListener(listener)

	subCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	eventCh, subID := fix.bus.Subscribe(subCtx)
	t.Cleanup(func() { fix.bus.Unsubscribe(subID) })

	// The diff above needs real git; push and gh below must not run for real.
	inst.SetGitWorktree(git.NewGitWorktreeFromStorageWithExecutor(dir, dir, "pr-wiring", "feature/wiring", "", git.WithCommandRunner(prWiringRunner{})))
	resp, err := fix.svc.CreatePullRequest(ctx, connect.NewRequest(&sessionv1.CreatePullRequestRequest{SessionId: inst.UUID, Title: "wired"}))
	require.NoError(t, err)
	assert.EqualValues(t, 77, resp.Msg.PrNumber)
	assert.True(t, resp.Msg.Persisted)

	updated, err := fix.storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusPRPending), updated.Status)

	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-eventCh:
			if ev.Type == events.EventSessionUpdated && assert.ObjectsAreEqual([]string{"github_pr_url", "github_pr_number"}, ev.UpdatedFields) {
				return
			}
		case <-deadline:
			t.Fatal("no github_pr_url SessionUpdated event")
		}
	}
}

// Not parallel: WithIsolatedStateDir sets an env var the flag reader consults.
func TestGetSession_should_FillPRHolds_When_BacklogScopeOn(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	flags := flagsvc.NewFeatureFlagService()
	smprSet(t, flags, config.SessionMultiPRScopePoller, true)
	smprSet(t, flags, config.SessionMultiPRScopeBacklog, true)

	fix := setupForkTestFixture(t)
	ref, err := domain.NewPRRef("", "acme", "app", 12)
	require.NoError(t, err)
	inst := &instance.Instance{
		Title: "holds-wiring", UUID: uuid.NewString(), Path: t.TempDir(), Status: domain.Paused, Program: "claude",
		GitHubOwner: "acme", GitHubRepo: "app", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, inst.AttachPR(ref, domain.AttachSourceBranchDiscovery, domain.PRInitial{State: domain.PRStateOpen}))
	require.NoError(t, fix.storage.AddInstance(inst))
	svctest.AddInstanceToPoller(fix.poller, inst)

	holds := func(svc *SessionService) []*sessionv1.PrHold {
		resp, err := svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: inst.UUID}))
		require.NoError(t, err)
		return resp.Msg.Session.PrHolds
	}
	require.Len(t, holds(fix.svc), 1, "poller path")

	noPoller := NewSessionService(fix.storage, fix.bus)
	t.Cleanup(noPoller.Shutdown)
	require.Len(t, holds(noPoller), 1, "storage fallback path")
}

func TestAskSessionToFixPullRequest_should_SteerTheNudgePrompt_When_Delivered(t *testing.T) {
	t.Parallel()
	f := newAskFixFixture(t, domain.Active)
	var steered string
	f.svc.guardedSteer.write = func(_ context.Context, _ *instance.Instance, text string) error {
		steered = text
		return nil
	}
	_, err := f.ask(protoRef(12))
	require.NoError(t, err)
	assert.Contains(t, steered, "Please fix what is currently blocking pull request https://github.com/acme/app/pull/12")
}
