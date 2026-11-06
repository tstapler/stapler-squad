package inttest

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/auth"
	"github.com/tstapler/stapler-squad/session/hostclaim"
	"github.com/tstapler/stapler-squad/session/internal/peeraddr"
)

// claimNode is one simulated host serving the real claim endpoint. Like
// convergenceNode it lives in the external test package so it can use
// server/auth without a build cycle.
type claimNode struct {
	identity hostclaim.HostIdentity
	registry *hostclaim.HostRegistry
	index    *hostclaim.ClaimIndex
	gossiper *hostclaim.ClaimGossiper
	addr     string
	srv      *httptest.Server
}

func newClaimNode(t *testing.T) *claimNode {
	t.Helper()
	peeraddr.AllowImplausible = true
	t.Cleanup(func() { peeraddr.AllowImplausible = false })

	identity, err := hostclaim.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity: %v", err)
	}
	registry, err := hostclaim.NewHostRegistry(t.TempDir(), hostclaim.DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry: %v", err)
	}
	index, err := hostclaim.NewClaimIndex(t.TempDir(), registry)
	if err != nil {
		t.Fatalf("NewClaimIndex: %v", err)
	}
	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	gossiper, err := hostclaim.NewClaimGossiper(identity, registry, index, []string{u.Host}, time.Minute)
	if err != nil {
		t.Fatalf("NewClaimGossiper: %v", err)
	}
	auth.RegisterClaimAdvertisementRoute(mux, index, gossiper)
	return &claimNode{identity: identity, registry: registry, index: index, gossiper: gossiper, addr: u.Host, srv: srv}
}

// closeServer takes the node's endpoints offline, simulating a down host.
func (n *claimNode) closeServer() { n.srv.Close() }

func (n *claimNode) learn(t *testing.T, peer *claimNode) {
	t.Helper()
	ad := hostclaim.BuildAdvertisement(peer.identity, []string{peer.addr}, time.Now())
	if _, accepted, err := n.registry.Advertise(ad); err != nil || !accepted {
		t.Fatalf("Advertise(peer) accepted=%v err=%v", accepted, err)
	}
}

func TestClaimGossip_should_ReachHostBAndReGossipToHostC_When_HostABroadcastsOnce(t *testing.T) {
	a, b, c := newClaimNode(t), newClaimNode(t), newClaimNode(t)
	a.learn(t, b)
	b.learn(t, a)
	b.learn(t, c)
	c.learn(t, b)
	c.learn(t, a) // C must know A to accept A's claim re-gossiped by B

	const issue = "https://github.com/o/r/issues/1"
	record := hostclaim.NewSignedClaimRecord(a.identity, issue, "ssq://hostA/backlog/v1/bl_01J", time.Now())
	if err := a.gossiper.BroadcastOnce(t.Context(), record); err != nil {
		t.Fatalf("BroadcastOnce() error = %v, want nil", err)
	}
	b.gossiper.Wait() // B re-gossips to C asynchronously

	for name, n := range map[string]*claimNode{"B (direct)": b, "C (one re-gossip hop)": c} {
		got, ok := n.index.CheckClaim(issue)
		if !ok || got.ClaimingHostID.String() != a.identity.ID.String() {
			t.Errorf("host %s CheckClaim() = (%+v, %v), want host A's record", name, got, ok)
		}
	}
}
