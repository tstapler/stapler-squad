package svctest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

type FakePeerLister struct{ Entries []hostclaim.RegistryEntry }

func (f FakePeerLister) LiveSnapshot() []hostclaim.RegistryEntry { return f.Entries }

func NewClaimTestIdentity(t *testing.T) hostclaim.HostIdentity {
	t.Helper()
	identity, err := hostclaim.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	return identity
}

// newClaimTestIndex builds an index whose registry has enrolled the given hosts:
// RecordClaim only accepts claims from enrolled hosts.
func NewClaimTestIndex(t *testing.T, enrolled ...hostclaim.HostIdentity) *hostclaim.ClaimIndex {
	t.Helper()
	registry, err := hostclaim.NewHostRegistry(t.TempDir(), hostclaim.DefaultHostRegistryTTL)
	require.NoError(t, err)
	for _, identity := range enrolled {
		_, accepted, err := registry.Advertise(hostclaim.BuildAdvertisement(identity, []string{"peer.example:8444"}, time.Now()))
		require.NoError(t, err)
		require.True(t, accepted)
	}
	index, err := hostclaim.NewClaimIndex(t.TempDir(), registry)
	require.NoError(t, err)
	return index
}

// claimPeer starts a TLS test server standing in for a peer's /internal/claim-lookup
// endpoint and returns its registry entry plus a request counter.
func ClaimPeer(t *testing.T, identity hostclaim.HostIdentity, handler http.HandlerFunc) (hostclaim.RegistryEntry, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return hostclaim.RegistryEntry{HostID: identity.ID, AdvertisedAddress: []string{srv.Listener.Addr().String()}}, &hits
}

func ClaimingHandler(record hostclaim.ClaimRecord) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(record)
	}
}
