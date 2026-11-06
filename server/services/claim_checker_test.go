package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

const claimTestURL = "https://github.com/o/r/issues/42"

type fakePeerLister struct{ entries []session.RegistryEntry }

func (f fakePeerLister) LiveSnapshot() []session.RegistryEntry { return f.entries }

func newClaimTestIdentity(t *testing.T) session.HostIdentity {
	t.Helper()
	identity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	return identity
}

// newClaimTestIndex builds an index whose registry has enrolled the given hosts:
// RecordClaim only accepts claims from enrolled hosts.
func newClaimTestIndex(t *testing.T, enrolled ...session.HostIdentity) *session.ClaimIndex {
	t.Helper()
	registry, err := session.NewHostRegistry(t.TempDir(), session.DefaultHostRegistryTTL)
	require.NoError(t, err)
	for _, identity := range enrolled {
		_, accepted, err := registry.Advertise(session.BuildAdvertisement(identity, []string{"peer.example:8444"}, time.Now()))
		require.NoError(t, err)
		require.True(t, accepted)
	}
	index, err := session.NewClaimIndex(t.TempDir(), registry)
	require.NoError(t, err)
	return index
}

// claimPeer starts a TLS test server standing in for a peer's /internal/claim-lookup
// endpoint and returns its registry entry plus a request counter.
func claimPeer(t *testing.T, identity session.HostIdentity, handler http.HandlerFunc) (session.RegistryEntry, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return session.RegistryEntry{HostID: identity.ID, AdvertisedAddress: []string{srv.Listener.Addr().String()}}, &hits
}

func claimingHandler(record session.ClaimRecord) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(record)
	}
}

func unclaimedHandler(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }

func hangingHandler(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }

func newTestLocalChecker(t *testing.T, self session.HostIdentity, timeout time.Duration, peers ...session.RegistryEntry) (*LocalClaimChecker, *session.ClaimIndex) {
	t.Helper()
	return newTestLocalCheckerEnrolled(t, self, timeout, nil, peers...)
}

func newTestLocalCheckerEnrolled(t *testing.T, self session.HostIdentity, timeout time.Duration, enrolled []session.HostIdentity, peers ...session.RegistryEntry) (*LocalClaimChecker, *session.ClaimIndex) {
	t.Helper()
	index := newClaimTestIndex(t, enrolled...)
	c := NewLocalClaimChecker(index, self.ID, fakePeerLister{entries: peers})
	c.timeout = timeout
	return c, index
}

func TestUnimplementedClaimChecker_CheckClaim_should_ReturnUnclaimedVerdict_When_NoCheckerWired(t *testing.T) {
	var c CrossHostClaimChecker = unimplementedClaimChecker{}
	for name, check := range map[string]func(context.Context, string) (ClaimVerdict, error){
		"live": c.CheckClaim, "local": c.CheckClaimLocalOnly,
	} {
		verdict, err := check(context.Background(), claimTestURL)
		require.NoError(t, err, name)
		assert.Equal(t, NewUnclaimedVerdict(), verdict, name)
	}
}

func TestLocalClaimChecker_CheckClaim_should_ReturnHeldByOtherVerdict_When_ClaimIndexHasEntryForURL(t *testing.T) {
	self, other := newClaimTestIdentity(t), newClaimTestIdentity(t)
	c, index := newTestLocalCheckerEnrolled(t, self, time.Second, []session.HostIdentity{other})
	record := session.NewSignedClaimRecord(other, claimTestURL, "ssq://hostA/backlog/v1/bl_1", time.Now())
	_, err := index.RecordClaim(record)
	require.NoError(t, err)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimHeldByOther, verdict.Kind)
	assert.Equal(t, other.ID.String(), verdict.Record.ClaimingHostID.String())
	assert.Equal(t, "ssq://hostA/backlog/v1/bl_1", verdict.Record.ItemDeepLink)
}

func TestLocalClaimChecker_CheckClaim_should_ReadFromRealClaimIndexOnDisk_When_BackedByTempStateDir(t *testing.T) {
	self, other := newClaimTestIdentity(t), newClaimTestIdentity(t)
	c, index := newTestLocalCheckerEnrolled(t, self, time.Second, []session.HostIdentity{other})

	verdict, err := c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimUnclaimed, verdict.Kind, "empty index")

	_, err = index.RecordClaim(session.NewSignedClaimRecord(self, claimTestURL, "ssq://me/x", time.Now()))
	require.NoError(t, err)
	verdict, _ = c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	assert.Equal(t, ClaimUnclaimed, verdict.Kind, "a claim this host holds itself never blocks it")

	const otherURL = "https://github.com/o/r/issues/43"
	_, err = index.RecordClaim(session.NewSignedClaimRecord(other, otherURL, "ssq://other/x", time.Now()))
	require.NoError(t, err)
	verdict, _ = c.CheckClaimLocalOnly(context.Background(), otherURL)
	assert.Equal(t, ClaimHeldByOther, verdict.Kind)
}

func TestLocalClaimChecker_CheckClaim_should_FindRecentClaimViaLivePeerQuery_When_LocalIndexUnclaimedButPeerHasRecord(t *testing.T) {
	self, hostA := newClaimTestIdentity(t), newClaimTestIdentity(t)
	record := session.NewSignedClaimRecord(hostA, claimTestURL, "ssq://hostA/backlog/v1/bl_1", time.Now())
	peer, _ := claimPeer(t, hostA, claimingHandler(record))
	c, index := newTestLocalCheckerEnrolled(t, self, 2*time.Second, []session.HostIdentity{hostA}, peer)

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
	self, quiet, hung := newClaimTestIdentity(t), newClaimTestIdentity(t), newClaimTestIdentity(t)
	quietPeer, _ := claimPeer(t, quiet, unclaimedHandler)
	hungPeer, _ := claimPeer(t, hung, hangingHandler)
	c, _ := newTestLocalChecker(t, self, 300*time.Millisecond, quietPeer, hungPeer)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimCheckIndeterminate, verdict.Kind, "an unreachable peer must never collapse to Unclaimed")
}

func TestLocalClaimChecker_CheckClaim_should_ReturnUnclaimed_When_AllPeersAnswerNotFound(t *testing.T) {
	self, a, b := newClaimTestIdentity(t), newClaimTestIdentity(t), newClaimTestIdentity(t)
	peerA, _ := claimPeer(t, a, unclaimedHandler)
	peerB, _ := claimPeer(t, b, unclaimedHandler)
	c, _ := newTestLocalChecker(t, self, time.Second, peerA, peerB)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimUnclaimed, verdict.Kind)
}

func TestLocalClaimChecker_CheckClaim_should_CompleteWithinOneTimeoutWindow_When_FanningOutToThreePeersConcurrently(t *testing.T) {
	const timeout = 400 * time.Millisecond
	self, quiet, hung1, hung2 := newClaimTestIdentity(t), newClaimTestIdentity(t), newClaimTestIdentity(t), newClaimTestIdentity(t)
	quietPeer, _ := claimPeer(t, quiet, unclaimedHandler)
	hungPeer1, _ := claimPeer(t, hung1, hangingHandler)
	hungPeer2, _ := claimPeer(t, hung2, hangingHandler)
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
	self, claimant, hung := newClaimTestIdentity(t), newClaimTestIdentity(t), newClaimTestIdentity(t)
	record := session.NewSignedClaimRecord(claimant, claimTestURL, "ssq://a/x", time.Now())
	claimPeerEntry, _ := claimPeer(t, claimant, claimingHandler(record))
	hungPeer, _ := claimPeer(t, hung, hangingHandler)
	c, _ := newTestLocalCheckerEnrolled(t, self, timeout, []session.HostIdentity{claimant}, hungPeer, claimPeerEntry)

	start := time.Now()
	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimHeldByOther, verdict.Kind)
	assert.Less(t, time.Since(start), timeout/2)
}

func TestLocalClaimChecker_CheckClaim_should_ReturnIndeterminate_When_PeerReturnsForgedRecord(t *testing.T) {
	self, peerID := newClaimTestIdentity(t), newClaimTestIdentity(t)
	forged := session.NewSignedClaimRecord(peerID, claimTestURL, "ssq://a/x", time.Now())
	forged.Signature[0] ^= 0xFF
	peer, _ := claimPeer(t, peerID, claimingHandler(forged))
	c, index := newTestLocalChecker(t, self, time.Second, peer)

	verdict, err := c.CheckClaim(context.Background(), claimTestURL)
	require.NoError(t, err)
	assert.Equal(t, ClaimCheckIndeterminate, verdict.Kind)
	_, stored := index.CheckClaim(claimTestURL)
	assert.False(t, stored, "an unverifiable record is never stored")
}

func TestLocalClaimChecker_CheckClaim_should_SkipSelfAndNeverQueryLocalOnlyPath(t *testing.T) {
	self, other := newClaimTestIdentity(t), newClaimTestIdentity(t)
	selfPeer, selfHits := claimPeer(t, self, unclaimedHandler)
	otherPeer, otherHits := claimPeer(t, other, unclaimedHandler)
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
	self, a, b := newClaimTestIdentity(t), newClaimTestIdentity(t), newClaimTestIdentity(t)
	c, index := newTestLocalCheckerEnrolled(t, self, time.Second, []session.HostIdentity{a, b})
	now := time.Now()
	_, err := index.RecordClaim(session.NewSignedClaimRecord(a, claimTestURL, "ssq://a/x", now))
	require.NoError(t, err)
	_, err = index.RecordClaim(session.NewSignedClaimRecord(b, claimTestURL, "ssq://b/x", now.Add(time.Minute)))
	require.NoError(t, err)

	verdict, _ := c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	require.Equal(t, ClaimHeldByOther, verdict.Kind)
	require.True(t, verdict.Record.Disputed)

	require.NoError(t, c.ResolveDispute(context.Background(), claimTestURL))
	verdict, _ = c.CheckClaimLocalOnly(context.Background(), claimTestURL)
	assert.False(t, verdict.Record.Disputed)
}
