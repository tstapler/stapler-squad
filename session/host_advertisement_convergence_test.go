package session_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/auth"
	"github.com/tstapler/stapler-squad/session"
)

// convergenceNode bundles one simulated instance's identity, registry, HTTP
// server, and HostAdvertiser for the three-node gossip convergence test
// below. It lives in an external (_test) package because it exercises the
// real advertisement endpoint (server/auth) end-to-end, not just
// session-internal state -- session already sits below server/auth in the
// dependency graph (server/auth imports session), so this test file
// importing server/auth back does not create a build cycle: the production
// session package never imports server/auth, only this test binary does.
type convergenceNode struct {
	identity   session.HostIdentity
	registry   *session.HostRegistry
	advertiser *session.HostAdvertiser
	url        string
}

func newConvergenceNode(t *testing.T) *convergenceNode {
	t.Helper()
	identity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateHostIdentity() error = %v, want nil", err)
	}
	registry, err := session.NewHostRegistry(t.TempDir(), session.DefaultHostRegistryTTL)
	if err != nil {
		t.Fatalf("NewHostRegistry() error = %v, want nil", err)
	}

	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	url := "http://" + server.Listener.Addr().String()

	advertiser := session.NewHostAdvertiser(identity, registry, []string{url}, time.Hour)
	auth.RegisterHostAdvertisementRoute(mux, identity, registry, advertiser, []string{url})
	server.Start()
	t.Cleanup(server.Close)

	return &convergenceNode{identity: identity, registry: registry, advertiser: advertiser, url: url}
}

// seedKnownPeer makes node aware of peer as if peer's address had been
// learned out-of-band (e.g. manual bootstrap or --list-known-hosts) --
// ADR-002 explicitly scopes discovery/NAT-traversal out, assuming hosts
// already know how to reach each other before gossip begins.
func seedKnownPeer(t *testing.T, node, peer *convergenceNode) {
	t.Helper()
	record := session.BuildAdvertisement(peer.identity, []string{peer.url}, time.Now())
	if _, _, err := node.registry.Advertise(record); err != nil {
		t.Fatalf("seeding known peer failed: %v", err)
	}
}

// TestAdvertisementEndpoint_should_ConvergeThreeNodeTopology_When_BoundedReGossipRuns
// wires up A -> B -> C (A knows only B, B knows only C, C knows nobody) and
// asserts that a single broadcast from A reaches C -- a peer A never
// directly advertised to -- via B's bounded one-hop re-gossip. It also
// exercises the endpoint's malformed-payload rejection directly against one
// of the three live nodes.
func TestAdvertisementEndpoint_should_ConvergeThreeNodeTopology_When_BoundedReGossipRuns(t *testing.T) {
	nodeA := newConvergenceNode(t)
	nodeB := newConvergenceNode(t)
	nodeC := newConvergenceNode(t)

	seedKnownPeer(t, nodeA, nodeB) // A --knows--> B
	seedKnownPeer(t, nodeB, nodeC) // B --knows--> C
	// C intentionally knows nobody, and A never learns of C directly.

	if _, ok := nodeA.registry.Lookup(nodeC.identity.ID); ok {
		t.Fatalf("precondition violated: A already knows about C before gossip runs")
	}
	if _, ok := nodeC.registry.Lookup(nodeA.identity.ID); ok {
		t.Fatalf("precondition violated: C already knows about A before gossip runs")
	}

	ctx := t.Context()
	nodeA.advertiser.BroadcastOnce(ctx)

	entry, ok := nodeC.registry.Lookup(nodeA.identity.ID)
	if !ok {
		t.Fatalf("C never learned about A -- bounded re-gossip through B did not converge")
	}
	if len(entry.AdvertisedAddress) == 0 || entry.AdvertisedAddress[0] != nodeA.url {
		t.Fatalf("C's entry for A has AdvertisedAddress = %v, want [%s]", entry.AdvertisedAddress, nodeA.url)
	}

	if _, ok := nodeB.registry.Lookup(nodeA.identity.ID); !ok {
		t.Fatalf("B (the direct recipient) never learned about A")
	}

	// Malformed payload against a live node's real HTTP server: rejected
	// cleanly (non-2xx), never a panic or a silently-accepted bad record.
	resp, err := http.Post(nodeB.url+session.AdvertisementEndpointPath, "application/json", strings.NewReader("{not valid json"))
	if err != nil {
		t.Fatalf("POST malformed payload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed payload status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
