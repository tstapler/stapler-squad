package services

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// recordingPRActions records which PR each GitHub action reaches.
type recordingPRActions struct{ calls []string }

func (f *recordingPRActions) rec(op string, n int, extra string) {
	f.calls = append(f.calls, fmt.Sprintf("%s#%d%s", op, n, extra))
}
func (f *recordingPRActions) MergePR(_ context.Context, _ github.RepoRef, n int, m string) error {
	f.rec("merge", n, ":"+m)
	return nil
}
func (f *recordingPRActions) ClosePR(_ context.Context, _ github.RepoRef, n int) error {
	f.rec("close", n, "")
	return nil
}
func (f *recordingPRActions) PostPRComment(_ context.Context, _ github.RepoRef, n int, _ string) error {
	f.rec("comment", n, "")
	return nil
}
func (f *recordingPRActions) GetPRComments(_ context.Context, _ github.RepoRef, n int) ([]github.PRComment, error) {
	f.rec("comments", n, "")
	return nil, nil
}
func (f *recordingPRActions) GetPRDiff(_ context.Context, _ github.RepoRef, n int) (string, error) {
	f.rec("diff", n, "")
	return "", nil
}

// prTargetService serves one instance holding the given PRs through a GitHubService.
func prTargetService(t *testing.T, prs ...prSpec) (*GitHubService, *recordingPRActions) {
	t.Helper()
	inst := &session.Instance{Title: "multi", UUID: "uuid-multi", GitHubOwner: "acme", GitHubRepo: "app"}
	for i, p := range prs {
		ref, err := domain.NewPRRef("", "acme", "app", p.number)
		require.NoError(t, err)
		require.NoError(t, inst.AttachPR(ref, domain.AttachSourceBranchDiscovery,
			session.PRInitial{State: p.state, Terminal: p.terminal}), "pr %d", i)
	}
	rec := &recordingPRActions{}
	inst.SetPRActionClient(rec)
	svc := NewGitHubService(nil)
	svc.find = func(string) (*session.Instance, error) { return inst, nil }
	return svc, rec
}

var mergedAndOpen = []prSpec{{11, domain.PRStateMerged, true}, {12, domain.PRStateOpen, false}}

func TestMergePR_ShouldTargetActivePR_WhenNoPRSelector(t *testing.T) {
	t.Parallel()
	svc, rec := prTargetService(t, mergedAndOpen...)
	_, err := svc.MergePR(context.Background(), connect.NewRequest(&sessionv1.MergePRRequest{Id: "multi"}))
	require.NoError(t, err)
	assert.Equal(t, []string{"merge#12:merge"}, rec.calls)
}

func TestResolveTargetPR_ShouldFailPrecondition_WhenPRNotAttached(t *testing.T) {
	t.Parallel()
	svc, rec := prTargetService(t, mergedAndOpen...)
	_, err := svc.MergePR(context.Background(), connect.NewRequest(&sessionv1.MergePRRequest{Id: "multi", Pr: protoRef(99)}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Empty(t, rec.calls, "GitHub is not called")
}

func TestResolveTargetPR_ShouldFailPrecondition_WhenUnsetSelectorAndNoPRs(t *testing.T) {
	t.Parallel()
	svc, rec := prTargetService(t)
	_, err := svc.MergePR(context.Background(), connect.NewRequest(&sessionv1.MergePRRequest{Id: "multi"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "no pull request")
	assert.Empty(t, rec.calls, "never a call for number 0")
}

func TestResolveTargetPR_ShouldFallBackToLegacyPR_WhenSessionHasNoLinks(t *testing.T) {
	t.Parallel()
	inst := &session.Instance{Title: "legacy", UUID: "uuid-legacy", GitHubOwner: "acme", GitHubRepo: "app", GitHubPRNumber: 7, GitHubPRURL: "https://github.com/acme/app/pull/7"}
	rec := &recordingPRActions{}
	inst.SetPRActionClient(rec)
	svc := NewGitHubService(nil)
	svc.find = func(string) (*session.Instance, error) { return inst, nil }

	_, err := svc.ClosePR(context.Background(), connect.NewRequest(&sessionv1.ClosePRRequest{Id: "legacy"}))
	require.NoError(t, err)
	assert.Equal(t, []string{"close#7"}, rec.calls)
}

func TestCommentsDiffAskToFix_ShouldUseResolvedTarget_WhenSelectorGiven(t *testing.T) {
	t.Parallel()
	svc, rec := prTargetService(t, mergedAndOpen...)
	ctx := context.Background()

	_, err := svc.GetPRComments(ctx, connect.NewRequest(&sessionv1.GetPRCommentsRequest{Id: "multi", Pr: protoRef(11)}))
	require.NoError(t, err)
	_, err = svc.PostPRComment(ctx, connect.NewRequest(&sessionv1.PostPRCommentRequest{Id: "multi", Body: "x"}))
	require.NoError(t, err)
	_, err = svc.ClosePR(ctx, connect.NewRequest(&sessionv1.ClosePRRequest{Id: "multi", Pr: protoRef(11)}))
	require.NoError(t, err)

	assert.Equal(t, []string{"comments#11", "comment#12", "close#11"}, rec.calls)
}
