package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

type fakePeers struct {
	reg  *session.HostRegistry
	live []session.RegistryEntry
}

func (p *fakePeers) LiveSnapshot() []session.RegistryEntry { return p.live }
func (p *fakePeers) AuthorizePeerRequest(h http.Header, m, path string, now time.Time) (session.HostID, error) {
	return p.reg.AuthorizePeerRequest(h, m, path, now)
}

// federationFixture runs a peer's handler on a TLS server and returns a
// FederatedSessions on a second identity that has the peer as its only live host.
func federationFixture(t *testing.T) (*FederatedSessions, *httptest.Server, session.HostID) {
	t.Helper()
	peerIdentity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	selfIdentity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)

	// The peer's registry must hold the caller's key, as gossip would enrol it.
	peerReg, err := session.NewHostRegistry(t.TempDir(), time.Hour)
	require.NoError(t, err)
	_, ok, err := peerReg.Advertise(session.BuildAdvertisement(selfIdentity, []string{"198.51.100.1:8444"}, time.Now()))
	require.NoError(t, err)
	require.True(t, ok)

	list := func(context.Context) ([]*sessionv1.Session, error) {
		return []*sessionv1.Session{{Id: "work", Title: "work"}}, nil
	}
	srv := httptest.NewTLSServer(SessionFederationHandler(&fakePeers{reg: peerReg}, list))
	t.Cleanup(srv.Close)

	addr := strings.TrimPrefix(srv.URL, "https://")
	live := []session.RegistryEntry{{HostID: peerIdentity.ID, AdvertisedAddress: []string{addr}}}
	allowLoopbackFederationDialsForTest(t)
	return NewFederatedSessions(selfIdentity, &fakePeers{live: live}), srv, peerIdentity.ID
}

func TestFederatedSessions_should_TagRemoteSessions_When_PeerReachable(t *testing.T) {
	f, srv, hostID := federationFixture(t)
	f.Refresh(context.Background())

	got := f.Sessions()
	require.Len(t, got, 1)
	assert.Equal(t, hostID.String()+"/work", got[0].Id)
	assert.Equal(t, hostID.String(), got[0].HostId)
	assert.Equal(t, strings.TrimPrefix(srv.URL, "https://"), got[0].HostDisplayName)
	assert.False(t, got[0].HostStale)
}

func TestFederatedSessions_should_KeepStaleSnapshot_When_PeerGoesDown(t *testing.T) {
	f, srv, _ := federationFixture(t)
	f.Refresh(context.Background())
	srv.Close()
	f.Refresh(context.Background())

	got := f.Sessions()
	require.Len(t, got, 1)
	assert.True(t, got[0].HostStale)
}

func TestFederatedSessions_should_DropPeer_When_NoLongerLive(t *testing.T) {
	f, _, _ := federationFixture(t)
	f.Refresh(context.Background())
	f.peers.(*fakePeers).live = nil
	f.Refresh(context.Background())
	assert.Empty(t, f.Sessions())
}

func TestSessionFederationHandler_should_Reject_When_RequestUnsigned(t *testing.T) {
	reg, err := session.NewHostRegistry(t.TempDir(), time.Hour)
	require.NoError(t, err)
	h := SessionFederationHandler(&fakePeers{reg: reg}, func(context.Context) ([]*sessionv1.Session, error) {
		t.Fatal("list must not run for an unauthenticated request")
		return nil, nil
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, session.SessionFederationEndpointPath, nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestSessionService_GetSession_should_ServeMirroredSession_When_RemoteID(t *testing.T) {
	f, _, hostID := federationFixture(t)
	f.Refresh(context.Background())
	svc := &SessionService{federation: f}

	resp, err := svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: hostID.String() + "/work"}))
	require.NoError(t, err)
	assert.Equal(t, hostID.String(), resp.Msg.Session.HostId)

	_, err = svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{Id: hostID.String() + "/missing"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
