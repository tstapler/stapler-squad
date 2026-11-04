package server

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func originReq(t *testing.T, host, origin string, hdr map[string]string) *http.Request {
	r, err := http.NewRequest(http.MethodGet, "https://"+host+"/api/ws/unary", nil)
	require.NoError(t, err)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}

func TestWsUnaryRemoteOrigin_should_FollowStrictAllowList(t *testing.T) {
	origins := []string{"https://box.tail.ts.net:8444", "https://localhost:8444", "http://localhost:8543", "https://Mixed.Example:443"}
	hostnames := []string{"box.tail.ts.net", "lan.example"}

	cases := []struct {
		name   string
		host   string
		origin string
		hdr    map[string]string
		want   bool
	}{
		{"configured origin", "box.tail.ts.net:8444", "https://box.tail.ts.net:8444", nil, true},
		{"configured localhost", "localhost:8444", "https://localhost:8444", nil, true},
		{"case-insensitive", "box.tail.ts.net:8444", "HTTPS://BOX.tail.TS.net:8444", nil, true},
		{"default port normalized (origin omits :443)", "x:8444", "https://mixed.example", nil, true},
		{"default port normalized (configured omits)", "x:8444", "https://mixed.example:443", nil, true},
		{"trailing slash tolerated", "x:8444", "https://localhost:8444/", nil, true},
		{"same-origin with verified hostname", "lan.example:9000", "https://lan.example:9000", nil, true},
		{"same-origin default port", "lan.example", "https://lan.example", nil, true},
		{"unconfigured unicode origin", "x:8444", "https://münchen.example", nil, false},

		{"evil origin", "box.tail.ts.net:8444", "https://evil.example", nil, false},
		{"rebinding shape: origin=host, unverified name", "evil.example:8444", "https://evil.example:8444", nil, false},
		{"same site other port", "box.tail.ts.net:8444", "https://box.tail.ts.net:9999", nil, false},
		{"plaintext origin", "box.tail.ts.net:8444", "http://box.tail.ts.net:8444", nil, false},
		{"plaintext configured entry never matches", "localhost:8543", "http://localhost:8543", nil, false},
		{"null origin", "box.tail.ts.net:8444", "null", nil, false},
		{"userinfo", "box.tail.ts.net:8444", "https://evil@box.tail.ts.net:8444", nil, false},
		{"path smuggling", "box.tail.ts.net:8444", "https://box.tail.ts.net:8444/x", nil, false},
		{"subdomain suffix", "box.tail.ts.net:8444", "https://evil.box.tail.ts.net:8444", nil, false},
		{"prefix attack", "box.tail.ts.net:8444", "https://box.tail.ts.net.evil.example:8444", nil, false},
		{"wildcard attempt", "box.tail.ts.net:8444", "https://*.ts.net:8444", nil, false},
		{"garbage", "box.tail.ts.net:8444", "::not a url", nil, false},

		{"missing origin with cookie", "box.tail.ts.net:8444", "", map[string]string{"Cookie": "cs_auth=t"}, false},
		{"missing origin, no credentials", "box.tail.ts.net:8444", "", nil, false},
		{"missing origin with bearer", "box.tail.ts.net:8444", "", map[string]string{"Authorization": "Bearer tok"}, true},
		{"missing origin with bearer and cookie", "box.tail.ts.net:8444", "", map[string]string{"Authorization": "Bearer tok", "Cookie": "cs_auth=t"}, false},
		{"missing origin with basic auth", "box.tail.ts.net:8444", "", map[string]string{"Authorization": "Basic abc"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := remoteUnaryOriginAllowed(originReq(t, tc.host, tc.origin, tc.hdr), origins, hostnames)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWsUnaryRemoteOrigin_should_AcceptPunycodeForm_When_ConfiguredAsUnicode(t *testing.T) {
	origins := []string{"https://münchen.example:8444"}
	assert.True(t, remoteUnaryOriginAllowed(originReq(t, "x:8444", "https://xn--mnchen-3ya.example:8444", nil), origins, nil))
}

func TestWsUnaryRemoteOrigin_should_ReadLiveSets_When_ServerOriginsChange(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	policy := srv.wsUnaryRemoteOriginPolicy()

	r := func() *http.Request { return originReq(t, "lan.example:8444", "https://lan.example:8444", nil) }
	assert.False(t, policy(r()), "empty configuration allows nothing")

	srv.SetOrigins([]string{"https://lan.example:8444"})
	assert.True(t, policy(r()), "policy reads GetOrigins lazily")

	srv.SetOrigins(nil)
	srv.SetHostnames([]string{"lan.example"})
	assert.True(t, policy(r()), "same-origin clause uses GetHostnames")
}
