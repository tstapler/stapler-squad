package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

type prSpec struct {
	number   int
	state    session.PRState
	terminal bool
}

// addLivePRSession persists a session with the given PR links and registers the
// reloaded (hydrated, persister-wired) instance with the live poller.
func addLivePRSession(t *testing.T, fix *forkTestFixture, title, uuid string, prs ...prSpec) *session.Instance {
	t.Helper()
	inst := &session.Instance{Title: title, UUID: uuid, Path: "/tmp/test", Status: domain.Stopped, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(), GitHubOwner: "acme", GitHubRepo: "app"}
	require.NoError(t, fix.storage.AddInstance(inst))
	for _, p := range prs {
		ref, err := session.NewPRRef("", "acme", "app", p.number)
		require.NoError(t, err)
		_, err = fix.storage.AttachSessionPR(context.Background(), uuid, ref,
			session.PRLink{Role: session.PRRolePrimary, Source: session.AttachSourceBranchDiscovery,
				Initial: session.PRInitial{State: p.state, Terminal: p.terminal}}, session.PRMirror{})
		require.NoError(t, err)
	}
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, li := range loaded {
		if li.UUID == uuid {
			svctest.AddInstanceToPoller(fix.poller, li)
			return li
		}
	}
	t.Fatalf("session %s not found after reload", title)
	return nil
}

func protoRef(n int32) *sessionv1.PullRequestRef {
	return &sessionv1.PullRequestRef{Host: "github.com", Owner: "acme", Repo: "app", Number: n}
}

func numbersOf(prs []*sessionv1.SessionPullRequest) []int32 {
	out := make([]int32, 0, len(prs))
	for _, p := range prs {
		out = append(out, p.Number)
	}
	return out
}

func TestDetachSessionPullRequest_ShouldDeleteOneLinkAndTombstoneInOneTxn(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	inst := addLivePRSession(t, fix, "u1", "uuid-u1", prSpec{11, session.PRStateMerged, true}, prSpec{12, session.PRStateOpen, false})

	resp, err := fix.svc.DetachSessionPullRequest(context.Background(), connect.NewRequest(
		&sessionv1.DetachSessionPullRequestRequest{Id: "uuid-u1", Pr: protoRef(12)}))
	require.NoError(t, err)
	assert.Equal(t, []int32{11}, numbersOf(resp.Msg.PullRequests))

	views, err := fix.storage.ListSessionPRViewsByUUIDs(context.Background(), []string{"uuid-u1"})
	require.NoError(t, err)
	require.Len(t, views["uuid-u1"], 1, "only the detached link is removed")

	// Branch discovery attaching #12 again is blocked by the tombstone.
	ref12, _ := session.NewPRRef("", "acme", "app", 12)
	require.NoError(t, inst.AttachPR(ref12, session.AttachSourceBranchDiscovery, session.PRInitial{State: session.PRStateOpen}))
	views, err = fix.storage.ListSessionPRViewsByUUIDs(context.Background(), []string{"uuid-u1"})
	require.NoError(t, err)
	assert.Len(t, views["uuid-u1"], 1, "no recreated link")
	assert.Equal(t, []int{11}, prNumbers(inst))
}

func prNumbers(inst *session.Instance) []int {
	prs := inst.Snapshot().GitHub.PRs
	out := make([]int, 0, len(prs))
	for _, v := range prs {
		out = append(out, v.Ref.Number())
	}
	return out
}

func TestDetachSessionPullRequest_ShouldFailPrecondition_WhenPRNotAttached(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addLivePRSession(t, fix, "u1", "uuid-u1", prSpec{11, session.PRStateOpen, false})

	_, err := fix.svc.DetachSessionPullRequest(context.Background(), connect.NewRequest(
		&sessionv1.DetachSessionPullRequestRequest{Id: "uuid-u1", Pr: protoRef(99)}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	_, err = fix.svc.DetachSessionPullRequest(context.Background(), connect.NewRequest(
		&sessionv1.DetachSessionPullRequestRequest{Id: "uuid-u1"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "an unset selector is never PR 0")
}

func TestPinSessionPullRequest_ShouldMakePinnedActive_UntilUnpinned(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	inst := addLivePRSession(t, fix, "u1", "uuid-u1", prSpec{11, session.PRStateOpen, false}, prSpec{12, session.PRStateOpen, false})
	ctx := context.Background()

	resp, err := fix.svc.PinSessionPullRequest(ctx, connect.NewRequest(&sessionv1.PinSessionPullRequestRequest{Id: "uuid-u1", Pr: protoRef(11)}))
	require.NoError(t, err)
	active := activeOf(resp.Msg.PullRequests)
	require.NotNil(t, active)
	assert.EqualValues(t, 11, active.Number)
	assert.True(t, active.Pinned)

	resp, err = fix.svc.PinSessionPullRequest(ctx, connect.NewRequest(&sessionv1.PinSessionPullRequestRequest{Id: "uuid-u1"}))
	require.NoError(t, err)
	assert.EqualValues(t, 12, activeOf(resp.Msg.PullRequests).Number, "unset clears the pin; the newest open PR is active again")
	for _, v := range inst.Snapshot().GitHub.PRs {
		assert.False(t, v.Pinned)
	}

	_, err = fix.svc.PinSessionPullRequest(ctx, connect.NewRequest(&sessionv1.PinSessionPullRequestRequest{Id: "uuid-u1", Pr: protoRef(99)}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func activeOf(prs []*sessionv1.SessionPullRequest) *sessionv1.SessionPullRequest {
	for _, p := range prs {
		if p.IsActive {
			return p
		}
	}
	return nil
}

func TestAttachSessionPullRequest_ShouldVerifyViaConditionalRequestAndClearTombstone_WhenURLGiven(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addLivePRSession(t, fix, "u1", "uuid-u1", prSpec{11, session.PRStateOpen, false}, prSpec{30, session.PRStateOpen, false})
	ctx := context.Background()

	// Detach #30 so a tombstone exists, then attach it by URL.
	_, err := fix.svc.DetachSessionPullRequest(ctx, connect.NewRequest(&sessionv1.DetachSessionPullRequestRequest{Id: "uuid-u1", Pr: protoRef(30)}))
	require.NoError(t, err)

	var looked []int
	fix.svc.prRefs.lookup = func(_ context.Context, ref github.RepoRef, n int) (*github.PRInfo, error) {
		looked = append(looked, n)
		if n == 404 {
			return nil, errors.New("not found")
		}
		return &github.PRInfo{Number: n, State: "OPEN", HTMLURL: "https://github.com/acme/app/pull/30", HeadRef: "feat"}, nil
	}

	resp, err := fix.svc.AttachSessionPullRequest(ctx, connect.NewRequest(&sessionv1.AttachSessionPullRequestRequest{
		Id: "uuid-u1", Url: "https://github.com/acme/app/pull/30"}))
	require.NoError(t, err)
	assert.ElementsMatch(t, []int32{11, 30}, numbersOf(resp.Msg.PullRequests))
	assert.Equal(t, []int{30}, looked)
	for _, p := range resp.Msg.PullRequests {
		if p.Number == 30 {
			assert.Equal(t, "manual", p.AttachSource)
		}
	}
	views, err := fix.storage.ListSessionPRViewsByUUIDs(ctx, []string{"uuid-u1"})
	require.NoError(t, err)
	assert.Len(t, views["uuid-u1"], 2, "the manual attach cleared the tombstone and wrote the link")

	_, err = fix.svc.AttachSessionPullRequest(ctx, connect.NewRequest(&sessionv1.AttachSessionPullRequestRequest{
		Id: "uuid-u1", Url: "https://github.com/acme/app/pull/404"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "a PR GitHub cannot find is rejected")
	_, err = fix.svc.AttachSessionPullRequest(ctx, connect.NewRequest(&sessionv1.AttachSessionPullRequestRequest{
		Id: "uuid-u1", Url: "https://example.com/not-a-pr"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestRefreshSessionPullRequests_ShouldRefreshOpenAndUnverifiedRowsOnly_WhenCalled(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addLivePRSession(t, fix, "u1", "uuid-u1",
		prSpec{9, session.PRStateUnknown, true}, prSpec{11, session.PRStateMerged, true}, prSpec{12, session.PRStateOpen, false})

	_, err := fix.svc.RefreshSessionPullRequests(context.Background(), connect.NewRequest(&sessionv1.RefreshSessionPullRequestsRequest{Id: "uuid-u1"}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "no refresher wired")

	var polled []int
	fix.svc.SetPRRefreshFunc(func(_ context.Context, _ *session.Instance, ref session.PRRef) error {
		polled = append(polled, ref.Number())
		return nil
	})
	resp, err := fix.svc.RefreshSessionPullRequests(context.Background(), connect.NewRequest(&sessionv1.RefreshSessionPullRequestsRequest{Id: "uuid-u1"}))
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{9, 12}, polled, "unverified-terminal #9 and open #12; verified merged #11 is not polled")
	assert.Len(t, resp.Msg.PullRequests, 3)
}

func TestListSessions_ShouldFilterByPR_WhenPRSetAndNotZero(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addLivePRSession(t, fix, "u1", "uuid-u1", prSpec{12, session.PRStateOpen, false})
	addLivePRSession(t, fix, "u2", "uuid-u2", prSpec{13, session.PRStateOpen, false})
	addLivePRSession(t, fix, "u3", "uuid-u3")
	ctx := context.Background()
	titles := func(req *sessionv1.ListSessionsRequest) []string {
		resp, err := fix.svc.ListSessions(ctx, connect.NewRequest(req))
		require.NoError(t, err)
		out := make([]string, 0, len(resp.Msg.Sessions))
		for _, s := range resp.Msg.Sessions {
			out = append(out, s.Title)
		}
		return out
	}

	assert.Equal(t, []string{"u1"}, titles(&sessionv1.ListSessionsRequest{Pr: protoRef(12)}))
	upper := &sessionv1.PullRequestRef{Host: "GitHub.com", Owner: "Acme", Repo: "App", Number: 12}
	assert.Equal(t, []string{"u1"}, titles(&sessionv1.ListSessionsRequest{Pr: upper}), "identity is canonical, not case sensitive")
	assert.ElementsMatch(t, []string{"u1", "u2", "u3"}, titles(&sessionv1.ListSessionsRequest{}), "unset means no filter")
	assert.Empty(t, titles(&sessionv1.ListSessionsRequest{Pr: protoRef(99)}))

	_, err := fix.svc.ListSessions(ctx, connect.NewRequest(&sessionv1.ListSessionsRequest{Pr: &sessionv1.PullRequestRef{Owner: "acme", Repo: "app"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "a selector without a number is never PR 0")
}
