package adapters

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func prView(t *testing.T, n int, state session.PRState, terminal bool) session.PRView {
	t.Helper()
	ref, err := session.NewPRRef("", "acme", "app", n)
	require.NoError(t, err)
	return session.PRView{Ref: ref, State: state, StoredTerminal: terminal, AttachedAt: time.Date(2026, 11, 1, 0, 0, n, 0, time.UTC)}
}

func TestSessionPullRequests_ShouldFlagOnlyTheActivePR_WhenMergedAndOpen(t *testing.T) {
	got := SessionPullRequests([]session.PRView{
		prView(t, 11, session.PRStateMerged, true), prView(t, 12, session.PRStateOpen, false),
	}, "")
	require.Len(t, got, 2)
	assert.False(t, got[0].IsActive)
	assert.True(t, got[1].IsActive)
	assert.EqualValues(t, 12, got[1].Number)
}

func TestSessionPullRequests_ShouldMapPollHealthAndUnverifiedOutcome(t *testing.T) {
	limited := prView(t, 12, session.PRStateOpen, false)
	limited.PollError, limited.RateLimitResetAt = "rate_limited", time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	inaccessible := prView(t, 13, session.PRStateOpen, false)
	inaccessible.PollError = "repo_inaccessible"
	seeded := prView(t, 9, session.PRStateUnknown, true)

	got := SessionPullRequests([]session.PRView{limited, inaccessible, seeded, prView(t, 14, session.PRStateOpen, false)}, "")
	assert.Equal(t, sessionv1.PollError_POLL_ERROR_RATE_LIMITED, got[0].PollError)
	assert.NotNil(t, got[0].RateLimitResetAt)
	assert.Equal(t, sessionv1.PollError_POLL_ERROR_REPO_INACCESSIBLE, got[1].PollError)
	assert.True(t, got[2].OutcomeUnverified)
	assert.Equal(t, sessionv1.PollError_POLL_ERROR_NONE, got[3].PollError)
	assert.False(t, got[3].OutcomeUnverified)
}

func TestInstanceToProto_ShouldMapPullRequestsAndKeepScalarsFromMirror(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	inst := newInstanceWithPRs(t, "adapter-prs", 11, 12)
	inst.GitHubPRNumber, inst.GitHubPRURL = 12, "https://github.com/acme/app/pull/12"

	got := InstanceToProto(inst, nil)

	require.Len(t, got.PullRequests, 2)
	assert.EqualValues(t, 12, got.GithubPrNumber)
	active := 0
	for _, pr := range got.PullRequests {
		if pr.IsActive {
			active++
		}
	}
	assert.Equal(t, 1, active)
}

func newInstanceWithPRs(t *testing.T, title string, numbers ...int) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{Title: title, Path: t.TempDir()})
	require.NoError(t, err)
	inst.UUID = "uuid-" + title
	for i, n := range numbers {
		ref, rerr := session.NewPRRef("github.com", "acme", "app", n)
		require.NoError(t, rerr)
		state := session.PRStateOpen
		if i < len(numbers)-1 {
			state = session.PRStateMerged
		}
		require.NoError(t, inst.AttachPR(ref, session.AttachSourceManual, session.PRInitial{
			State: state, Terminal: state.Terminal(), URL: "https://github.com/acme/app/pull/" + fmt.Sprint(n), HeadBranch: "backlog/feature-" + fmt.Sprint(n),
		}))
	}
	return inst
}

// K11: the added list-response size per session and in total stays under the plan's caps.
func TestListSessions_ShouldAddAtMostKnownBytesPerSession_WhenEachHasThreePRs(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	const sessions = 200
	var withPRs, withoutPRs int
	perSessionMax := 0
	for i := 0; i < sessions; i++ {
		inst := newInstanceWithPRs(t, fmt.Sprintf("k11-%d", i), 100+i*3, 101+i*3, 102+i*3)
		s := InstanceToProto(inst, nil)
		with := proto.Size(s)
		s.PullRequests = nil
		without := proto.Size(s)
		withPRs += with
		withoutPRs += without
		if added := with - without; added > perSessionMax {
			perSessionMax = added
		}
	}
	added := withPRs - withoutPRs
	t.Logf("K11 measured: max %d B/session, %d B total for %d sessions", perSessionMax, added, sessions)
	assert.LessOrEqual(t, perSessionMax, 1536, "1.5 KiB per session")
	assert.LessOrEqual(t, added, 40*1024, "40 KiB for 200 sessions")
}

func TestSessionPullRequests_ShouldCarryTitleVerifyAttemptsAndItemDirect(t *testing.T) {
	t.Parallel()
	unverified := prView(t, 9, session.PRStateUnknown, true)
	unverified.Title, unverified.PollFailCount = "Old change", 3
	settled := prView(t, 10, session.PRStateOpen, false)
	settled.PollFailCount = 4 // a live row's failures are not verify attempts
	direct := prView(t, 11, session.PRStateOpen, false)
	direct.BacklogItemID = "item-1"

	got := SessionPullRequests([]session.PRView{unverified, settled, direct}, "")

	assert.Equal(t, "Old change", got[0].Title)
	assert.EqualValues(t, 3, got[0].VerifyAttempts)
	assert.EqualValues(t, session.VerifyAttemptLimit, got[0].VerifyAttemptLimit)
	assert.False(t, got[0].VerifyExhausted)
	assert.False(t, got[0].ItemDirect)
	assert.Zero(t, got[1].VerifyAttempts)
	assert.Zero(t, got[1].VerifyAttemptLimit)
	assert.True(t, got[2].ItemDirect)
}

func TestSessionPullRequests_ShouldFlagVerifyExhausted_WhenFiveFailuresSpanThirtyMinutes(t *testing.T) {
	t.Parallel()
	v := prView(t, 9, session.PRStateUnknown, true)
	v.PollFailCount = session.VerifyAttemptLimit
	v.FirstFailAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	v.LastPolledAt = v.FirstFailAt.Add(31 * time.Minute)

	got := SessionPullRequests([]session.PRView{v}, "")

	assert.True(t, got[0].VerifyExhausted)
	assert.EqualValues(t, session.VerifyAttemptLimit, got[0].VerifyAttempts)
}
