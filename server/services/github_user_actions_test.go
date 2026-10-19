package services

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	gh "github.com/tstapler/stapler-squad/github"
)

type actionRecorder struct {
	merged, closed, refreshed int
	method                    string
	draft                     *bool
	err                       error
}

type fixedTokens struct{ ok bool }

func (f fixedTokens) TokenForPR(gh.PRKey) (string, string, bool) { return "tok", "alice", f.ok }

type fixedSnapshot []gh.UserPR

func (f fixedSnapshot) GetAll() []gh.UserPR { return f }

var cachedPR = fixedSnapshot{{Owner: "o", Repo: "r", Number: 7, Host: "github.com", NodeID: "PR_1"}}

func newActionService(r *actionRecorder, tokens bool) *GitHubUserService {
	return &GitHubUserService{actions: prActionDeps{
		merge: func(_ context.Context, _ gh.RepoRef, _ int, method string) error {
			r.merged++
			r.method = method
			return r.err
		},
		close: func(context.Context, gh.RepoRef, int) error { r.closed++; return r.err },
		setDraft: func(_ context.Context, _, _, _ string, draft bool) error {
			r.draft = &draft
			return r.err
		},
		refresh:   func(context.Context) error { r.refreshed++; return nil },
		tokenFrom: fixedTokens{ok: tokens},
		snapshot:  cachedPR,
	}}
}

func TestMergePRDirect_should_DelegateAndRefresh_When_Succeeds(t *testing.T) {
	r := &actionRecorder{}
	method := "squash"
	resp, err := newActionService(r, true).MergePRDirect(context.Background(), connect.NewRequest(
		&sessionv1.MergePRDirectRequest{Owner: "o", Repo: "r", Number: 7, Method: &method}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, 1, r.merged)
	assert.Equal(t, "squash", r.method)
	assert.Equal(t, 1, r.refreshed)
}

func TestMergePRDirect_should_NotRefresh_When_MergeFails(t *testing.T) {
	r := &actionRecorder{err: errors.New("boom")}
	_, err := newActionService(r, true).MergePRDirect(context.Background(), connect.NewRequest(
		&sessionv1.MergePRDirectRequest{Owner: "o", Repo: "r", Number: 7}))
	require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Zero(t, r.refreshed)
}

func TestMergePRDirect_should_RejectInvalidTarget(t *testing.T) {
	_, err := newActionService(&actionRecorder{}, true).MergePRDirect(context.Background(), connect.NewRequest(
		&sessionv1.MergePRDirectRequest{Owner: "o", Repo: "r"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestClosePRDirect_should_DelegateAndRefresh_When_Succeeds(t *testing.T) {
	r := &actionRecorder{}
	resp, err := newActionService(r, true).ClosePRDirect(context.Background(), connect.NewRequest(
		&sessionv1.ClosePRDirectRequest{Owner: "o", Repo: "r", Number: 7}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, 1, r.closed)
	assert.Equal(t, 1, r.refreshed)
}

func TestDraftToggle_should_CallMutationAndRefresh_When_NodeIDPresent(t *testing.T) {
	req := &sessionv1.PRDraftToggleRequest{Owner: "o", Repo: "r", Number: 7}

	r := &actionRecorder{}
	_, err := newActionService(r, true).ConvertPRToDraft(context.Background(), connect.NewRequest(req))
	require.NoError(t, err)
	require.NotNil(t, r.draft)
	assert.True(t, *r.draft)
	assert.Equal(t, 1, r.refreshed)

	r = &actionRecorder{}
	_, err = newActionService(r, true).MarkPRReady(context.Background(), connect.NewRequest(req))
	require.NoError(t, err)
	require.NotNil(t, r.draft)
	assert.False(t, *r.draft)
	assert.Equal(t, 1, r.refreshed)
}

func TestDraftToggle_should_ReturnFailedPrecondition_When_NodeIDEmptyOrNoAccount(t *testing.T) {
	r := &actionRecorder{}
	svc := newActionService(r, true)
	noNode := &sessionv1.PRDraftToggleRequest{Owner: "o", Repo: "r", Number: 8}
	_, err := svc.ConvertPRToDraft(context.Background(), connect.NewRequest(noNode))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	_, err = svc.MarkPRReady(context.Background(), connect.NewRequest(noNode))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	noAcct := &sessionv1.PRDraftToggleRequest{Owner: "o", Repo: "r", Number: 7}
	_, err = newActionService(r, false).MarkPRReady(context.Background(), connect.NewRequest(noAcct))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Nil(t, r.draft)
	assert.Zero(t, r.refreshed)
}

func TestUserPRsToProto_should_MapExtendedFields(t *testing.T) {
	out := userPRsToProto([]gh.UserPR{{Number: 1, NodeID: "PR_1", Body: "b", Labels: []string{"x"},
		Additions: 5, Deletions: 2, ChangedFiles: 3, RequestedReviewerLogins: []string{"bob"}}})[0]
	assert.Equal(t, "PR_1", out.NodeId)
	assert.Equal(t, "b", out.Body)
	assert.Equal(t, []string{"x"}, out.Labels)
	assert.EqualValues(t, 5, out.Additions)
	assert.EqualValues(t, 2, out.Deletions)
	assert.EqualValues(t, 3, out.ChangedFiles)
	assert.Equal(t, []string{"bob"}, out.RequestedReviewerLogins)
}

func TestPRActions_should_ReturnFailedPrecondition_When_AccountNotConnected(t *testing.T) {
	r := &actionRecorder{}
	svc := newActionService(r, false)
	ctx := context.Background()
	_, mergeErr := svc.MergePRDirect(ctx, connect.NewRequest(&sessionv1.MergePRDirectRequest{Owner: "o", Repo: "r", Number: 7}))
	_, closeErr := svc.ClosePRDirect(ctx, connect.NewRequest(&sessionv1.ClosePRDirectRequest{Owner: "o", Repo: "r", Number: 7}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(mergeErr))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(closeErr))
	assert.Zero(t, r.merged+r.closed+r.refreshed)
}

func TestMergePRDirect_should_RejectUnknownMethod(t *testing.T) {
	r := &actionRecorder{}
	bad := "sqush"
	_, err := newActionService(r, true).MergePRDirect(context.Background(), connect.NewRequest(
		&sessionv1.MergePRDirectRequest{Owner: "o", Repo: "r", Number: 7, Method: &bad}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Zero(t, r.merged)
}

func TestDraftToggle_should_ReturnFailedPreconditionWithoutMutating_When_CachedNodeIDEmpty(t *testing.T) {
	r := &actionRecorder{}
	svc := newActionService(r, true)
	svc.actions.snapshot = fixedSnapshot{{Owner: "o", Repo: "r", Number: 7, Host: "github.com"}}
	req := &sessionv1.PRDraftToggleRequest{Owner: "o", Repo: "r", Number: 7}

	_, err := svc.ConvertPRToDraft(context.Background(), connect.NewRequest(req))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	_, err = svc.MarkPRReady(context.Background(), connect.NewRequest(req))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Nil(t, r.draft, "no GraphQL mutation may run without a node_id")
	assert.Zero(t, r.refreshed)
}
