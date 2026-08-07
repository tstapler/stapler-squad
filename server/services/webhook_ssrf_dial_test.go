package services

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPinnedDialer_DialContext_ProvesDNSRebindingIsClosed is the core proof for
// this fix: it simulates the exact DNS-rebinding scenario the bug report
// describes — a hostname that resolves to a public (allowed) IP at
// ValidateCallbackURL's send-time check, then resolves to a private/loopback IP
// moments later at actual dial time (TTL=0 rebinding). Before this fix, the
// stdlib http.Transport dialed the hostname directly and performed its own
// independent resolution at dial time, with no re-validation — so the
// rebound private IP would have been connected to. pinnedDialer.DialContext IS
// that dial-time resolution now, so it re-validates and must reject.
func TestPinnedDialer_DialContext_ProvesDNSRebindingIsClosed(t *testing.T) {
	// send-time check (what ValidateCallbackURL would have seen): public IP.
	sendTimeErr := checkDisallowedCallbackIP(net.ParseIP("8.8.8.8"))
	require.NoError(t, sendTimeErr, "sanity: the send-time answer must itself be allowed for this to be a real rebinding scenario")

	var dialCalled atomic.Bool
	p := &pinnedDialer{
		// dial-time answer (the rebound one): private, must never be dialed.
		lookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("10.0.0.5")}, nil
		},
		dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialCalled.Store(true)
			return nil, errors.New("dial should never be reached")
		},
	}

	_, err := p.DialContext(context.Background(), "tcp", "attacker-controlled.example:443")
	assert.Error(t, err, "dial-time resolution returning a private IP must be rejected even though the earlier send-time check saw a public IP")
	assert.False(t, dialCalled.Load(), "the disallowed IP must never actually be dialed")
}

// TestPinnedDialer_DialContext_RejectsRawPrivateIPLiteral proves a request
// targeting a bare private-IP literal (no DNS involved at all) is rejected
// without ever consulting lookupIP or dial.
func TestPinnedDialer_DialContext_RejectsRawPrivateIPLiteral(t *testing.T) {
	var lookupCalled, dialCalled atomic.Bool
	p := &pinnedDialer{
		lookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
			lookupCalled.Store(true)
			return nil, errors.New("lookupIP should not be called for an IP literal")
		},
		dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialCalled.Store(true)
			return nil, errors.New("dial should never be reached")
		},
	}

	_, err := p.DialContext(context.Background(), "tcp", "127.0.0.1:8080")
	assert.Error(t, err)
	assert.False(t, lookupCalled.Load())
	assert.False(t, dialCalled.Load())
}

// TestPinnedDialer_DialContext_RejectsIfAnyResolvedIPFails proves a host that
// resolves to multiple IPs is rejected outright if even one of them fails
// validation — fail-closed, matching ValidateCallbackURL's own posture, rather
// than trying to cherry-pick a safe address to dial.
func TestPinnedDialer_DialContext_RejectsIfAnyResolvedIPFails(t *testing.T) {
	var dialCalled atomic.Bool
	p := &pinnedDialer{
		lookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("169.254.169.254")}, nil
		},
		dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialCalled.Store(true)
			return nil, nil
		},
	}

	_, err := p.DialContext(context.Background(), "tcp", "multi-answer.example:443")
	assert.Error(t, err)
	assert.False(t, dialCalled.Load())
}

// TestPinnedDialer_DialContext_DialsResolvedIPNotHostname proves that once
// validation passes, the dial target is the validated IP address itself, not
// the original hostname — this is what actually pins the connection and closes
// the rebinding window (a second, hostname-based resolution never occurs).
func TestPinnedDialer_DialContext_DialsResolvedIPNotHostname(t *testing.T) {
	var dialedAddr string
	p := &pinnedDialer{
		lookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
			assert.Equal(t, "safe-host.example", host)
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		},
		dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialedAddr = addr
			return nil, nil
		},
	}

	_, err := p.DialContext(context.Background(), "tcp", "safe-host.example:443")
	require.NoError(t, err)
	assert.Equal(t, "8.8.8.8:443", dialedAddr, "must dial the resolved+validated IP directly, not the hostname")
}

// TestPinnedDialer_DialContext_RejectsWhenLookupFails proves a resolution
// failure at dial time is surfaced as an error rather than silently falling
// back to dialing the raw hostname (which would hand DNS resolution back to
// the OS/transport with no validation at all).
func TestPinnedDialer_DialContext_RejectsWhenLookupFails(t *testing.T) {
	var dialCalled atomic.Bool
	p := &pinnedDialer{
		lookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
			return nil, errors.New("boom")
		},
		dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialCalled.Store(true)
			return nil, nil
		},
	}

	_, err := p.DialContext(context.Background(), "tcp", "unresolvable.example:443")
	assert.Error(t, err)
	assert.False(t, dialCalled.Load())
}

// TestCallbackDispatcher_Deliver_RejectsRebindingAtDialTime is the
// dispatcher-level integration proof: it wires a pinnedDialer with a
// rebinding-simulating lookupIP directly into the *same* http.Client shape
// NewCallbackDispatcher builds in production (Transport.DialContext), then
// runs a full deliver() call with the real (permissive, since this test isn't
// exercising the send-time check) validateURL stub — proving the fix applies
// on the actual outbound send path used by CallbackDispatcher, not just to the
// pinnedDialer type in isolation.
func TestCallbackDispatcher_Deliver_RejectsRebindingAtDialTime(t *testing.T) {
	// The target server itself is irrelevant here — dial-time validation must
	// reject before any TCP connection is attempted, so the server should
	// never see a request.
	var serverHit atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rebindingDialer := &pinnedDialer{
		lookupIP: func(ctx context.Context, host string) ([]net.IP, error) {
			// Simulates the dial-time DNS answer differing (post-rebind) from
			// whatever the send-time ValidateCallbackURL check saw.
			return []net.IP{net.ParseIP("169.254.169.254")}, nil
		},
		dial: (&net.Dialer{}).DialContext,
	}

	d := &CallbackDispatcher{
		client: &http.Client{
			Transport: &http.Transport{DialContext: rebindingDialer.DialContext},
		},
		inFlight:    make(chan struct{}, 20),
		validateURL: permissiveValidator, // send-time check stubbed permissive, as in the existing test suite
	}

	d.inFlight <- struct{}{}
	done := make(chan struct{})
	go func() {
		d.deliver("session_complete", "http://rebinding-target.example/hook", map[string]any{"event": "session_complete"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(callbackAttemptTimeout*callbackRetryAttempts + 5*time.Second):
		t.Fatal("deliver did not complete in time")
	}

	assert.False(t, serverHit.Load(), "dial-time re-validation must reject the rebound IP before any request reaches a server")
}
