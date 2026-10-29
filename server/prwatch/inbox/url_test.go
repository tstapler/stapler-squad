package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient_should_AcceptURL_When_HostIsLoopback(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8789", "http://[::1]:8789", "http://localhost:8789"} {
		u, err := ParseLoopbackURL(raw)
		require.NoError(t, err, raw)
		assert.NotEmpty(t, u.Host)
	}
}

func TestNewClient_should_RefuseURL_When_HostIsNotLoopback(t *testing.T) {
	for _, raw := range []string{
		"http://example.com", "http://10.0.0.5:8789", "http://0.0.0.0:8789",
		"http://127.0.0.1.evil.com", "http://localhost.evil.com", "http://127.0.0.1@evil.com",
		"http://evil.com#@127.0.0.1", "http://[::ffff:10.0.0.1]", "ftp://127.0.0.1",
		"file:///x", "127.0.0.1:8789", "",
	} {
		_, err := ParseLoopbackURL(raw)
		assert.ErrorIs(t, err, ErrNonLoopbackURL, raw)
	}
}

// Refusal is pure parsing: a listener that counts accepts sees nothing.
func TestNewClient_should_MakeZeroNetworkCalls_When_URLRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			c.Close()
			accepted <- struct{}{}
		}
	}()

	_, err = ParseLoopbackURL("http://127.0.0.1.evil.com:" + portOf(t, ln))
	require.ErrorIs(t, err, ErrNonLoopbackURL)
	ln.Close()
	assert.Empty(t, accepted)
}

type fakeResolver map[string][]net.IP

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	out := make([]net.IPAddr, 0, len(f[host]))
	for _, ip := range f[host] {
		out = append(out, net.IPAddr{IP: ip})
	}
	return out, nil
}

func TestNewClient_should_RefuseDial_When_LocalhostResolvesToNonLoopback(t *testing.T) {
	dial := LoopbackDialer(fakeResolver{
		"localhost": {net.ParseIP("93.184.216.34")},
		"mixed":     {net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.5")},
	}, nil)

	for _, addr := range []string{"localhost:8789", "mixed:8789", "10.0.0.5:8789", "[2001:db8::1]:8789"} {
		_, err := dial(context.Background(), "tcp", addr)
		assert.True(t, errors.Is(err, ErrNonLoopbackURL), "%s: %v", addr, err)
	}
}

func TestNewClient_should_Dial_When_LocalhostResolvesToLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	dial := LoopbackDialer(fakeResolver{"localhost": {net.ParseIP("127.0.0.1")}}, nil)
	conn, err := dial(context.Background(), "tcp", "localhost:"+portOf(t, ln))
	require.NoError(t, err)
	conn.Close()
}

func TestTypes_should_DecodeWatchResult_When_CoverageUnavailable(t *testing.T) {
	body := `{"coverage":"unavailable","reason":"no admin access","subscription":{"id":"sub_1","consumer_id":"squad:h","repository":"a/b","pull_request":7,"created_at":"2026-09-04T19:00:00.000Z","active":true,"last_activity":null}}`
	var w WatchResult
	require.NoError(t, json.Unmarshal([]byte(body), &w))
	assert.Equal(t, CoverageUnavailable, w.Coverage)
	assert.False(t, w.Coverage.Covered())
	assert.Equal(t, "no admin access", w.Reason)
	assert.Equal(t, 7, w.Subscription.PullRequest)
	assert.Nil(t, w.Subscription.LastActivity)
}

func TestTypes_should_DecodeItem_When_EventHasNullHeadSHA(t *testing.T) {
	body := `{"id":"inbox_1","subscription_id":"sub_1","created_at":"2026-09-04T19:00:01.000Z","event":{"version":1,"id":"evt_1","repository":"a/b","pull_request":7,"kind":"comment","action":"created","actor":"r","occurred_at":"2026-09-04T19:00:00Z","head_sha":null,"summary":"s","trust":"untrusted","details":{"comment_id":456}}}`
	var it Item
	require.NoError(t, json.Unmarshal([]byte(body), &it))
	assert.Equal(t, "inbox_1", it.ID)
	assert.Nil(t, it.Event.HeadSHA)
	assert.Equal(t, "comment", it.Event.Kind)
	assert.JSONEq(t, `{"comment_id":456}`, string(it.Event.Details))
	assert.True(t, CoverageRegistered.Covered() && CoverageExisting.Covered())
}

func portOf(t *testing.T, ln net.Listener) string {
	t.Helper()
	_, p, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	return p
}
