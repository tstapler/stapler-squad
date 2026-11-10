package services

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

const claimTestURL = "https://github.com/o/r/issues/42"

func unclaimedHandler(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }

func hangingHandler(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }

func newTestLocalChecker(t *testing.T, self hostclaim.HostIdentity, timeout time.Duration, peers ...hostclaim.RegistryEntry) (*LocalClaimChecker, *hostclaim.ClaimIndex) {
	t.Helper()
	return newTestLocalCheckerEnrolled(t, self, timeout, nil, peers...)
}

func newTestLocalCheckerEnrolled(t *testing.T, self hostclaim.HostIdentity, timeout time.Duration, enrolled []hostclaim.HostIdentity, peers ...hostclaim.RegistryEntry) (*LocalClaimChecker, *hostclaim.ClaimIndex) {
	t.Helper()
	index := svctest.NewClaimTestIndex(t, enrolled...)
	c := NewLocalClaimChecker(index, self.ID, svctest.FakePeerLister{Entries: peers})
	c.timeout = timeout
	return c, index
}

func TestUnimplementedClaimChecker_CheckClaim_should_ReturnUnclaimedVerdict_When_NoCheckerWired(t *testing.T) {
	var c CrossHostClaimChecker = UnimplementedClaimChecker{}
	for name, check := range map[string]func(context.Context, string) (ClaimVerdict, error){
		"live": c.CheckClaim, "local": c.CheckClaimLocalOnly,
	} {
		verdict, err := check(context.Background(), claimTestURL)
		require.NoError(t, err, name)
		assert.Equal(t, NewUnclaimedVerdict(), verdict, name)
	}
}

func TestLocalClaimChecker_CheckClaim_should_ReturnHeldByOtherVerdict_When_ClaimIndexHasEntryForURL(t *testing.T) {
	self, other := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	c, index := newTestLocalCheckerEnrolled(t, self, time.Second, []hostclaim.HostIdentity{other})
	record := hostclaim.NewSignedClaimRecord(other, claimTestURL, "ssq://hostA/backlog/v1/bl_1", time.Now())
	_, err := index.RecordClaim(record)
	require.NoError(t, err)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimHeldByOther, verdict.Kind)
	assert.Equal(t, other.ID.String(), verdict.Record.ClaimingHostID.String())
	assert.Equal(t, "ssq://hostA/backlog/v1/bl_1", verdict.Record.ItemDeepLink)
}

func TestLocalClaimChecker_CheckClaim_should_ReadFromRealClaimIndexOnDisk_When_BackedByTempStateDir(t *testing.T) {
	self, other := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	c, index := newTestLocalCheckerEnrolled(t, self, time.Second, []hostclaim.HostIdentity{other})

	verdict, err := c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimUnclaimed, verdict.Kind, "empty index")

	_, err = index.RecordClaim(hostclaim.NewSignedClaimRecord(self, claimTestURL, "ssq://me/x", time.Now()))
	require.NoError(t, err)
	verdict, _ = c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	assert.Equal(t, ClaimUnclaimed, verdict.Kind, "a claim this host holds itself never blocks it")

	const otherURL = "https://github.com/o/r/issues/43"
	_, err = index.RecordClaim(hostclaim.NewSignedClaimRecord(other, otherURL, "ssq://other/x", time.Now()))
	require.NoError(t, err)
	verdict, _ = c.CheckClaimLocalOnly(context.Background(), otherURL)
	assert.Equal(t, ClaimHeldByOther, verdict.Kind)
}

func TestLocalClaimChecker_CheckClaim_should_FindRecentClaimViaLivePeerQuery_When_LocalIndexUnclaimedButPeerHasRecord(t *testing.T) {
	self, hostA := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	record := hostclaim.NewSignedClaimRecord(hostA, claimTestURL, "ssq://hostA/backlog/v1/bl_1", time.Now())
	peer, _ := svctest.ClaimPeer(t, hostA, svctest.ClaimingHandler(record))
	c, index := newTestLocalCheckerEnrolled(t, self, 2*time.Second, []hostclaim.HostIdentity{hostA}, peer)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimHeldByOther, verdict.Kind)
	assert.Equal(t, hostA.ID.String(), verdict.Record.ClaimingHostID.String())

	stored, ok := index.CheckClaim(claimTestURL)
	assert.True(t, ok, "a verified peer answer is ingested into the local index")
	assert.Equal(t, hostA.ID.String(), stored.ClaimingHostID.String())

	local, _ := c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	assert.Equal(t, ClaimHeldByOther, local.Kind)
}

func TestLocalClaimChecker_CheckClaim_should_ReturnIndeterminateVerdict_When_KnownPeerTimesOutAndNoneReportClaim(t *testing.T) {
	self, quiet, hung := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	quietPeer, _ := svctest.ClaimPeer(t, quiet, unclaimedHandler)
	hungPeer, _ := svctest.ClaimPeer(t, hung, hangingHandler)
	c, _ := newTestLocalChecker(t, self, 300*time.Millisecond, quietPeer, hungPeer)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimCheckIndeterminate, verdict.Kind, "an unreachable peer must never collapse to Unclaimed")
}

func TestLocalClaimChecker_CheckClaim_should_ReturnUnclaimed_When_AllPeersAnswerNotFound(t *testing.T) {
	self, a, b := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	peerA, _ := svctest.ClaimPeer(t, a, unclaimedHandler)
	peerB, _ := svctest.ClaimPeer(t, b, unclaimedHandler)
	c, _ := newTestLocalChecker(t, self, time.Second, peerA, peerB)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimUnclaimed, verdict.Kind)
}

func TestLocalClaimChecker_CheckClaim_should_CompleteWithinOneTimeoutWindow_When_FanningOutToThreePeersConcurrently(t *testing.T) {
	const timeout = 400 * time.Millisecond
	self, quiet, hung1, hung2 := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	quietPeer, _ := svctest.ClaimPeer(t, quiet, unclaimedHandler)
	hungPeer1, _ := svctest.ClaimPeer(t, hung1, hangingHandler)
	hungPeer2, _ := svctest.ClaimPeer(t, hung2, hangingHandler)
	c, _ := newTestLocalChecker(t, self, timeout, quietPeer, hungPeer1, hungPeer2)

	start := time.Now()
	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, ClaimCheckIndeterminate, verdict.Kind)
	assert.GreaterOrEqual(t, elapsed, timeout-50*time.Millisecond, "waited for the hung peers")
	assert.Less(t, elapsed, timeout+timeout/2, "two hung peers share one deadline; sequential would take 2x")
}

func TestLocalClaimChecker_CheckClaim_should_ReturnHeldByOtherWithoutWaitingForHungPeer_When_OnePeerClaims(t *testing.T) {
	const timeout = 2 * time.Second
	self, claimant, hung := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	record := hostclaim.NewSignedClaimRecord(claimant, claimTestURL, "ssq://a/x", time.Now())
	claimPeerEntry, _ := svctest.ClaimPeer(t, claimant, svctest.ClaimingHandler(record))
	hungPeer, _ := svctest.ClaimPeer(t, hung, hangingHandler)
	c, _ := newTestLocalCheckerEnrolled(t, self, timeout, []hostclaim.HostIdentity{claimant}, hungPeer, claimPeerEntry)

	start := time.Now()
	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimHeldByOther, verdict.Kind)
	assert.Less(t, time.Since(start), timeout/2)
}

func TestLocalClaimChecker_CheckClaim_should_ReturnIndeterminate_When_PeerReturnsForgedRecord(t *testing.T) {
	self, peerID := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	forged := hostclaim.NewSignedClaimRecord(peerID, claimTestURL, "ssq://a/x", time.Now())
	forged.Signature[0] ^= 0xFF
	peer, _ := svctest.ClaimPeer(t, peerID, svctest.ClaimingHandler(forged))
	c, index := newTestLocalChecker(t, self, time.Second, peer)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimCheckIndeterminate, verdict.Kind)
	_, stored := index.CheckClaim(claimTestURL)
	assert.False(t, stored, "an unverifiable record is never stored")
}

func TestLocalClaimChecker_CheckClaim_should_SkipSelfAndNeverQueryLocalOnlyPath(t *testing.T) {
	self, other := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	selfPeer, selfHits := svctest.ClaimPeer(t, self, unclaimedHandler)
	otherPeer, otherHits := svctest.ClaimPeer(t, other, unclaimedHandler)
	c, _ := newTestLocalChecker(t, self, time.Second, selfPeer, otherPeer)

	_, err := c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Zero(t, selfHits.Load()+otherHits.Load(), "local-only check must not touch the network")

	_, err = c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Zero(t, selfHits.Load(), "own registry entry is not queried")
	assert.EqualValues(t, 1, otherHits.Load())
}

func TestLocalClaimChecker_ResolveDispute_should_ClearDisputedFlag(t *testing.T) {
	self, a, b := svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t), svctest.NewClaimTestIdentity(t)
	c, index := newTestLocalCheckerEnrolled(t, self, time.Second, []hostclaim.HostIdentity{a, b})
	now := time.Now()
	_, err := index.RecordClaim(hostclaim.NewSignedClaimRecord(a, claimTestURL, "ssq://a/x", now))
	require.NoError(t, err)
	_, err = index.RecordClaim(hostclaim.NewSignedClaimRecord(b, claimTestURL, "ssq://b/x", now.Add(time.Minute)))
	require.NoError(t, err)

	verdict, _ := c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	require.Equal(t, ClaimHeldByOther, verdict.Kind)
	require.True(t, verdict.Record.Disputed)

	require.NoError(t, c.ResolveDispute(context.Background(), claimTestURL))
	verdict, _ = c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	assert.False(t, verdict.Record.Disputed)
}
