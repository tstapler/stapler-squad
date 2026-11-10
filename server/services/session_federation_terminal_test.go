package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func terminalFixture(t *testing.T, capture func(string) (string, error)) (*FederatedSessions, session.HostID) {
	t.Helper()
	peerIdentity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	selfIdentity, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	peerReg, err := session.NewHostRegistry(t.TempDir(), time.Hour)
	require.NoError(t, err)
	_, ok, err := peerReg.Advertise(session.BuildAdvertisement(selfIdentity, []string{"198.51.100.1:8444"}, time.Now()))
	require.NoError(t, err)
	require.True(t, ok)

	srv := httptest.NewTLSServer(SessionFederationTerminalHandler(&fakePeers{reg: peerReg}, capture, 5*time.Millisecond))
	t.Cleanup(srv.Close)
	live := []session.RegistryEntry{{HostID: peerIdentity.ID, AdvertisedAddress: []string{strings.TrimPrefix(srv.URL, "https://")}}}
	allowLoopbackFederationDialsForTest(t)
	return NewFederatedSessions(selfIdentity, &fakePeers{live: live}), peerIdentity.ID
}

func TestFederatedTerminal_should_RelayOnlyChangedSnapshots_When_PeerPaneChanges(t *testing.T) {
	var n atomic.Int32
	f, host := terminalFixture(t, func(id string) (string, error) {
		assert.Equal(t, "work", id)
		switch n.Add(1) {
		case 1, 2:
			return "one", nil
		default:
			return "two", nil
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got []string
	err := f.StreamTerminal(ctx, session.FederatedSessionID{HostID: host.String(), SessionID: "work"}, func(c string) error {
		got = append(got, c)
		if len(got) == 2 {
			cancel()
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two"}, got)
}

func TestFederatedTerminal_should_ReportUnavailable_When_HostNotLive(t *testing.T) {
	f, _ := terminalFixture(t, func(string) (string, error) { return "", nil })
	other, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	err = f.StreamTerminal(context.Background(), session.FederatedSessionID{HostID: other.ID.String(), SessionID: "x"}, func(string) error { return nil })
	assert.ErrorIs(t, err, ErrFederatedHostUnavailable)
}

func TestFederatedTerminal_should_Fail_When_PeerSessionMissing(t *testing.T) {
	f, host := terminalFixture(t, func(string) (string, error) { return "", assert.AnError })
	err := f.StreamTerminal(context.Background(), session.FederatedSessionID{HostID: host.String(), SessionID: "gone"}, func(string) error { return nil })
	assert.EqualError(t, err, "session unavailable")
}

func TestSessionFederationTerminalHandler_should_Reject_When_RequestUnsigned(t *testing.T) {
	reg, err := session.NewHostRegistry(t.TempDir(), time.Hour)
	require.NoError(t, err)
	h := SessionFederationTerminalHandler(&fakePeers{reg: reg}, func(string) (string, error) {
		t.Fatal("capture must not run for an unauthenticated request")
		return "", nil
	}, time.Millisecond)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, session.SessionFederationTerminalPath+"?session=x", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
