package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session/domain"
)

// newTwoPRInstance returns a session with #11 merged (terminal) and #12 open,
// the shared fixture of the session-multi-pr-tracking headline tests (T1.3.4).
func newTwoPRInstance(t *testing.T) *Instance {
	t.Helper()
	inst := newPRTestInstance(t)
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 11), domain.AttachSourceCreatedFromURL,
		domain.PRInitial{State: domain.PRStateMerged, Terminal: true}))
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery,
		domain.PRInitial{State: domain.PRStateOpen}))
	return inst
}

// fakePRActions records the PR number every action reaches.
type fakePRActions struct{ calls []string }

func (f *fakePRActions) rec(op string, n int) { f.calls = append(f.calls, fmt.Sprintf("%s#%d", op, n)) }

func (f *fakePRActions) MergePR(_ context.Context, _ github.RepoRef, n int, _ string) error {
	f.rec("merge", n)
	return nil
}
func (f *fakePRActions) ClosePR(_ context.Context, _ github.RepoRef, n int) error {
	f.rec("close", n)
	return nil
}
func (f *fakePRActions) PostPRComment(_ context.Context, _ github.RepoRef, n int, _ string) error {
	f.rec("comment", n)
	return nil
}
func (f *fakePRActions) GetPRComments(_ context.Context, _ github.RepoRef, n int) ([]github.PRComment, error) {
	f.rec("comments", n)
	return nil, nil
}
func (f *fakePRActions) GetPRDiff(_ context.Context, _ github.RepoRef, n int) (string, error) {
	f.rec("diff", n)
	return "", nil
}

// TestPRActions_DefaultTargetsActivePR is the headline test for PR actions
// (validation I-36): merge, close and comment with no explicit PR reach #12, the
// active PR, never the merged #11.
func TestPRActions_DefaultTargetsActivePR(t *testing.T) {
	inst := newTwoPRInstance(t)
	fake := &fakePRActions{}
	inst.SetPRActionClient(fake)
	ctx := context.Background()

	require.NoError(t, inst.MergePR(ctx, nil, "squash"))
	require.NoError(t, inst.ClosePR(ctx, nil))
	require.NoError(t, inst.PostComment(ctx, nil, "hi"))
	_, err := inst.GetPRComments(ctx, nil)
	require.NoError(t, err)
	_, err = inst.GetPRDiff(ctx, nil)
	require.NoError(t, err)

	require.Equal(t, []string{"merge#12", "close#12", "comment#12", "comments#12", "diff#12"}, fake.calls)
}

func TestPRActions_ShouldHonorExplicitPRAndRefuseUnattached(t *testing.T) {
	inst := newTwoPRInstance(t)
	fake := &fakePRActions{}
	inst.SetPRActionClient(fake)
	ctx := context.Background()
	r11, r99 := mustPRRef(t, "", "acme", "app", 11), mustPRRef(t, "", "acme", "app", 99)

	require.NoError(t, inst.MergePR(ctx, &r11, "merge"))
	require.Equal(t, []string{"merge#11"}, fake.calls)

	err := inst.MergePR(ctx, &r99, "merge")
	require.ErrorIs(t, err, ErrPRNotAttached)
	require.Len(t, fake.calls, 1, "GitHub is not called for a PR the session does not hold")
}

func TestPRActions_ShouldNeverTargetPRZero_WhenSessionHasNoPR(t *testing.T) {
	inst := newPRTestInstance(t)
	fake := &fakePRActions{}
	inst.SetPRActionClient(fake)
	err := inst.MergePR(context.Background(), nil, "merge")
	require.ErrorIs(t, err, ErrNoPullRequest)
	require.Empty(t, fake.calls)
}

func TestResolvePR_ShouldReturnCurrentPRAndRejectNonPRSession(t *testing.T) {
	t.Parallel()
	got, err := newTwoPRInstance(t).resolvePR(nil)
	require.NoError(t, err)
	require.Equal(t, 12, got.Number())

	explicit := mustPRRef(t, "", "acme", "app", 11)
	got, err = newTwoPRInstance(t).resolvePR(&explicit)
	require.NoError(t, err)
	require.Equal(t, explicit, got)

	_, err = (&Instance{Title: "plain"}).resolvePR(nil)
	require.ErrorContains(t, err, "is not a PR session")
}
