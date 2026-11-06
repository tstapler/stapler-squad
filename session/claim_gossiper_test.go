package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// claimPeer is an httptest TLS server standing in for a remote host's claim
// endpoint. It records what it receives and stores accepted records in index.
type claimPeer struct {
	identity HostIdentity
	addr     string
	index    *ClaimIndex
	srv      *httptest.Server

	mu       sync.Mutex
	received []ClaimRecord
}

func (p *claimPeer) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.received)
}

func newClaimPeer(t *testing.T, clock Clock) *claimPeer {
	t.Helper()
	p := &claimPeer{identity: newTestIdentity(t)}
	p.index = newTestClaimIndex(t, t.TempDir(), clock)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != ClaimAdvertisementEndpointPath {
			http.NotFound(w, r)
			return
		}
		var record ClaimRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		p.mu.Lock()
		p.received = append(p.received, record)
		p.mu.Unlock()
		outcome, err := p.index.RecordClaim(record)
		if err != nil || !outcome.Accepted {
			http.Error(w, "rejected", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	p.srv = srv
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse %q: %v", srv.URL, err)
	}
	p.addr = u.Host
	return p
}

// enroll makes the peer's registry pin identity, as it would after that host advertised.
func (p *claimPeer) enroll(t *testing.T, identity HostIdentity, at time.Time) {
	t.Helper()
	if _, accepted, err := p.index.hostRegistry.Advertise(newTestAdvertisement(t, identity, []string{"origin.example:8444"}, at)); err != nil || !accepted {
		t.Fatalf("Advertise(origin on peer) accepted=%v err=%v", accepted, err)
	}
}

// gossipFixture is a sending host with a registry that knows each peer.
type gossipFixture struct {
	self     HostIdentity
	clock    *fakeClock
	registry *HostRegistry
	index    *ClaimIndex
	gossiper *ClaimGossiper
}

func newGossipFixture(t *testing.T) *gossipFixture {
	t.Helper()
	prev := allowImplausibleAddressesForTest
	allowImplausibleAddressesForTest = true
	t.Cleanup(func() { allowImplausibleAddressesForTest = prev })

	f := &gossipFixture{self: newTestIdentity(t), clock: &fakeClock{now: time.Now()}}
	registry, err := NewHostRegistryWithClock(t.TempDir(), DefaultHostRegistryTTL, f.clock)
	if err != nil {
		t.Fatalf("NewHostRegistryWithClock: %v", err)
	}
	f.registry = registry
	f.index, err = NewClaimIndex(t.TempDir(), registry)
	if err != nil {
		t.Fatalf("NewClaimIndex: %v", err)
	}
	f.gossiper, err = NewClaimGossiper(f.self, registry, f.index, []string{"self.example:8444"}, time.Minute)
	if err != nil {
		t.Fatalf("NewClaimGossiper: %v", err)
	}
	return f
}

func (f *gossipFixture) addPeer(t *testing.T, p *claimPeer) {
	t.Helper()
	if _, accepted, err := f.registry.Advertise(newTestAdvertisement(t, p.identity, []string{p.addr}, f.clock.Now())); err != nil || !accepted {
		t.Fatalf("Advertise(peer) accepted=%v err=%v", accepted, err)
	}
	// The peer only accepts claims from hosts it has enrolled.
	if _, accepted, err := p.index.hostRegistry.Advertise(newTestAdvertisement(t, f.self, []string{"self.example:8444"}, f.clock.Now())); err != nil || !accepted {
		t.Fatalf("Advertise(self on peer) accepted=%v err=%v", accepted, err)
	}
}

func (f *gossipFixture) claim(url string) ClaimRecord {
	return NewSignedClaimRecord(f.self, url, "ssq://self/backlog/v1/bl_01J", f.clock.Now())
}

func TestClaimGossiper_BroadcastOnce_should_SendRecordToEveryKnownPeer_When_HostRegistrySnapshotNonEmpty(t *testing.T) {
	f := newGossipFixture(t)
	peers := []*claimPeer{newClaimPeer(t, f.clock), newClaimPeer(t, f.clock)}
	for _, p := range peers {
		f.addPeer(t, p)
	}

	record := f.claim(testIssueURL)
	if err := f.gossiper.BroadcastOnce(context.Background(), record); err != nil {
		t.Fatalf("BroadcastOnce() error = %v, want nil", err)
	}
	for i, p := range peers {
		if p.count() != 1 {
			t.Errorf("peer %d received %d posts, want exactly 1", i, p.count())
		}
		got, ok := p.index.CheckClaim(testIssueURL)
		if !ok || got.ClaimingHostID.String() != f.self.ID.String() {
			t.Errorf("peer %d CheckClaim() = (%+v, %v), want the sender's record", i, got, ok)
		}
	}
}

func TestClaimGossiper_BroadcastOnce_should_ReturnErrorButReachOtherPeers_When_OnePeerUnreachable(t *testing.T) {
	f := newGossipFixture(t)
	good := newClaimPeer(t, f.clock)
	f.addPeer(t, good)
	dead := newClaimPeer(t, f.clock)
	f.addPeer(t, dead)
	dead.srv.Close()

	if err := f.gossiper.BroadcastOnce(context.Background(), f.claim(testIssueURL)); err == nil {
		t.Fatalf("BroadcastOnce() error = nil, want an error naming the unreachable peer")
	}
	if _, ok := good.index.CheckClaim(testIssueURL); !ok {
		t.Fatalf("reachable peer did not receive the claim; round must continue past failures")
	}
}

func TestClaimGossiper_ReGossip_should_SkipPeerExcludedByPrune_When_PeerRegistryEntryAgedPastTTL(t *testing.T) {
	f := newGossipFixture(t)
	stale, fresh := newClaimPeer(t, f.clock), newClaimPeer(t, f.clock)
	f.addPeer(t, stale)
	f.clock.Advance(DefaultHostRegistryTTL + time.Second)
	f.addPeer(t, fresh)

	origin := newTestIdentity(t)
	fresh.enroll(t, origin, f.clock.Now())
	record := NewSignedClaimRecord(origin, testIssueURL, "ssq://origin/x", time.Now()) // real time: the index bounds ClaimedAt against it
	if err := f.gossiper.ReGossip(context.Background(), record); err != nil {
		t.Fatalf("ReGossip() error = %v, want nil", err)
	}
	if stale.count() != 0 {
		t.Errorf("stale peer received %d posts before Prune, want 0", stale.count())
	}
	if fresh.count() != 1 {
		t.Errorf("fresh peer received %d posts, want 1", fresh.count())
	}

	if err := f.registry.Prune(); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, ok := f.registry.Lookup(stale.identity.ID); ok {
		t.Fatalf("stale peer still in registry after Prune")
	}
}

func TestClaimGossiper_ReGossip_should_NotSendBackToClaimant(t *testing.T) {
	f := newGossipFixture(t)
	claimant, other := newClaimPeer(t, f.clock), newClaimPeer(t, f.clock)
	f.addPeer(t, claimant)
	f.addPeer(t, other)
	other.enroll(t, claimant.identity, f.clock.Now())

	record := NewSignedClaimRecord(claimant.identity, testIssueURL, "ssq://claimant/x", f.clock.Now())
	if err := f.gossiper.ReGossip(context.Background(), record); err != nil {
		t.Fatalf("ReGossip() error = %v, want nil", err)
	}
	if claimant.count() != 0 || other.count() != 1 {
		t.Fatalf("claimant got %d, other got %d; want 0 and 1", claimant.count(), other.count())
	}
}

func TestClaimGossiper_BackfillOnce_should_SendOnlyUnsentClaims_When_CalledRepeatedlyAsPeersJoin(t *testing.T) {
	f := newGossipFixture(t)
	first := newClaimPeer(t, f.clock)
	f.addPeer(t, first)
	if outcome, err := f.index.recordOwn(f.claim(testIssueURL)); err != nil || !outcome.Accepted {
		t.Fatalf("local recordOwn = %+v, err %v", outcome, err)
	}

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := f.gossiper.BackfillOnce(ctx); err != nil {
			t.Fatalf("BackfillOnce #%d error = %v", i, err)
		}
	}
	if first.count() != 1 {
		t.Fatalf("first peer received %d posts across two ticks, want 1 (no resend)", first.count())
	}

	late := newClaimPeer(t, f.clock)
	f.addPeer(t, late)
	if err := f.gossiper.BackfillOnce(ctx); err != nil {
		t.Fatalf("BackfillOnce (late peer) error = %v", err)
	}
	if late.count() != 1 || first.count() != 1 {
		t.Fatalf("late got %d, first got %d; want 1 and 1", late.count(), first.count())
	}
}

func TestClaimGossiper_SendClaim_should_ReturnError_When_PeerReturnsNon200(t *testing.T) {
	f := newGossipFixture(t)
	peer := newClaimPeer(t, f.clock)
	bad := f.claim(testIssueURL)
	bad.Signature = []byte("not a signature")
	if err := f.gossiper.SendClaim(context.Background(), peer.addr, bad); err == nil {
		t.Fatalf("SendClaim() error = nil, want error for a 400 response")
	}
}

func TestNewClaimGossiper_should_UseDefaultHostAdvertisementInterval_When_ConstructedWithoutOverride(t *testing.T) {
	f := newGossipFixture(t)
	g, err := NewClaimGossiper(f.self, f.registry, f.index, nil, 0)
	if err != nil {
		t.Fatalf("NewClaimGossiper() error = %v", err)
	}
	if g.interval != DefaultHostAdvertisementInterval {
		t.Fatalf("interval = %v, want %v", g.interval, DefaultHostAdvertisementInterval)
	}
}

func TestNewClaimGossiper_should_ReturnError_When_HostIdentityNil(t *testing.T) {
	f := newGossipFixture(t)
	if _, err := NewClaimGossiper(HostIdentity{}, f.registry, f.index, nil, time.Minute); err == nil {
		t.Fatalf("NewClaimGossiper(zero identity) error = nil, want error")
	}
	if _, err := NewClaimGossiper(f.self, nil, f.index, nil, time.Minute); err == nil {
		t.Fatalf("NewClaimGossiper(nil registry) error = nil, want error")
	}
	if _, err := NewClaimGossiper(f.self, f.registry, nil, nil, time.Minute); err == nil {
		t.Fatalf("NewClaimGossiper(nil index) error = nil, want error")
	}
}
