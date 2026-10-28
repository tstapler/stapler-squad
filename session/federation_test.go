package session

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederatedSessionID_should_RoundTrip_When_Remote(t *testing.T) {
	hid, err := NewHostID()
	require.NoError(t, err)
	remote := FederatedSessionID{HostID: hid.String(), SessionID: "my/session"}
	parsed := ParseFederatedSessionID(remote.String())
	assert.Equal(t, remote, parsed)
	assert.False(t, parsed.IsLocal())
}

func TestFederatedSessionID_should_BeLocal_When_BareOrSlashedTitle(t *testing.T) {
	for _, id := range []string{"plain", "feat/x", "not_a_host/abc", "host_/x", ""} {
		parsed := ParseFederatedSessionID(id)
		assert.True(t, parsed.IsLocal(), id)
		assert.Equal(t, id, parsed.String())
	}
}

func newPeerFixture(t *testing.T, now time.Time) (*HostRegistry, HostIdentity) {
	t.Helper()
	identity, err := LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	reg, err := NewHostRegistryWithClock(t.TempDir(), time.Hour, fixedClock{now})
	require.NoError(t, err)
	rec := BuildAdvertisement(identity, []string{"203.0.113.5:8444"}, now)
	_, accepted, err := reg.Advertise(rec)
	require.NoError(t, err)
	require.True(t, accepted)
	return reg, identity
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestAuthorizePeerRequest(t *testing.T) {
	now := time.Now()
	reg, identity := newPeerFixture(t, now)
	path := SessionFederationEndpointPath

	signed := func(at time.Time) http.Header {
		h := http.Header{}
		SignPeerRequest(h, identity, http.MethodGet, path, at)
		return h
	}

	got, err := reg.AuthorizePeerRequest(signed(now), http.MethodGet, path, now)
	require.NoError(t, err)
	assert.Equal(t, identity.ID, got)

	_, err = reg.AuthorizePeerRequest(http.Header{}, http.MethodGet, path, now)
	assert.Error(t, err, "unsigned")

	_, err = reg.AuthorizePeerRequest(signed(now.Add(-time.Minute)), http.MethodGet, path, now)
	assert.Error(t, err, "replayed outside window")

	_, err = reg.AuthorizePeerRequest(signed(now), http.MethodPost, path, now)
	assert.Error(t, err, "signature binds the method")

	_, err = reg.AuthorizePeerRequest(signed(now), http.MethodGet, "/internal/other", now)
	assert.Error(t, err, "signature binds the path")

	stranger, err := LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	h := http.Header{}
	SignPeerRequest(h, stranger, http.MethodGet, path, now)
	_, err = reg.AuthorizePeerRequest(h, http.MethodGet, path, now)
	assert.Error(t, err, "not enrolled")

	// Enrolled ID with a different key must fail verification.
	forged := stranger
	forged.ID = identity.ID
	h = http.Header{}
	SignPeerRequest(h, forged, http.MethodGet, path, now)
	_, err = reg.AuthorizePeerRequest(h, http.MethodGet, path, now)
	assert.Error(t, err, "pinned key mismatch")
}
