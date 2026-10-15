package middleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testRebindingPath = "/api/session.v1.SessionService/PruneHiddenSessionNotifications"

var testLocalIPs = []net.IP{net.ParseIP("192.168.1.50"), net.ParseIP("fd00::50")}

// verdictCfg builds a VerdictConfig with fixed inputs; listen is the bound address.
func verdictCfg(listen string, verified, origins []string) VerdictConfig {
	return VerdictConfig{
		VerifiedHosts:  func() []string { return verified },
		ListenAddr:     func() string { return listen },
		AllowedOrigins: func() []string { return origins },
		LocalIPs:       func() []net.IP { return testLocalIPs },
	}
}

func TestNormalizeHost_should_LowercaseStripPortBracketsAndOneTrailingDot(t *testing.T) {
	cases := map[string]string{
		"Name.Example.:8543": "name.example",
		"name.example":       "name.example",
		"[::1]:8543":         "::1",
		"[::1]":              "::1",
		"LOCALHOST:80":       "localhost",
		"a.b..":              "a.b.",
		"":                   "",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeHost(in), in)
	}
}

// T-RP-56 (middleware half): the rebinding gate has no LoopbackBound condition.
func TestRebindingVerdict_should_AllowVerifiedLanHostnameOnWildcardBindAndRejectTheRest(t *testing.T) {
	verified := []string{"onyx.lan"}
	origins := []string{"https://ui.example"}
	cases := []struct {
		name   string
		listen string
		facts  CallerFacts
		want   string
	}{
		{"loopback name on wildcard bind", "0.0.0.0:8543", CallerFacts{Host: "localhost:8543"}, ""},
		{"loopback v6 literal", "0.0.0.0:8543", CallerFacts{Host: "[::1]:8543"}, ""},
		{"verified LAN hostname on wildcard bind", "0.0.0.0:8543", CallerFacts{Host: "onyx.lan:8543"}, ""},
		{"verified hostname on empty-host bind", ":8543", CallerFacts{Host: "ONYX.LAN.:8543"}, ""},
		{"verified hostname with same-origin Origin", "0.0.0.0:8543", CallerFacts{Host: "onyx.lan:8543", Origin: "http://onyx.lan:8543"}, ""},
		{"verified hostname with configured Origin", "0.0.0.0:8543", CallerFacts{Host: "onyx.lan:8543", Origin: "https://ui.example"}, ""},
		{"verified hostname with loopback Origin", "0.0.0.0:8543", CallerFacts{Host: "onyx.lan:8543", Origin: "http://localhost:3000"}, ""},
		{"local interface IP literal", "0.0.0.0:8543", CallerFacts{Host: "192.168.1.50:8543"}, ""},
		{"local interface IPv6 literal", "0.0.0.0:8543", CallerFacts{Host: "[fd00::50]:8543"}, ""},
		{"non-wildcard listen host name", "box.lan:8543", CallerFacts{Host: "box.lan:8543"}, ""},
		{"non-wildcard listen IP literal", "10.9.8.7:8543", CallerFacts{Host: "10.9.8.7:8543"}, ""},
		{"rebinding name resolving to 127.0.0.1", "localhost:8543", CallerFacts{Host: "evil.example:8543"}, "host_not_allowed"},
		{"rebinding name on wildcard bind", "0.0.0.0:8543", CallerFacts{Host: "evil.example:8543"}, "host_not_allowed"},
		{"foreign Origin on verified Host", "0.0.0.0:8543", CallerFacts{Host: "onyx.lan:8543", Origin: "https://evil.example"}, "origin_not_allowed"},
		{"null Origin", "0.0.0.0:8543", CallerFacts{Host: "localhost:8543", Origin: "null"}, "origin_not_allowed"},
		{"loopback Origin cannot rescue a rebinding Host", "localhost:8543", CallerFacts{Host: "evil.example", Origin: "http://localhost:8543"}, "host_not_allowed"},
		{"IP literal the machine does not own", "0.0.0.0:8543", CallerFacts{Host: "203.0.113.9:8543"}, "host_not_allowed"},
		{"wildcard literal is not a listen host", "0.0.0.0:8543", CallerFacts{Host: "0.0.0.0:8543"}, "host_not_allowed"},
		{"empty Host", "localhost:8543", CallerFacts{}, "host_missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RebindingVerdict(tc.facts, verdictCfg(tc.listen, verified, origins)))
		})
	}
}

// An unverified name is not allowed even when another source lists it; only
// the verified set counts.
func TestRebindingVerdict_should_RejectNameMissingFromVerifiedSet(t *testing.T) {
	cfg := verdictCfg("0.0.0.0:8543", nil, nil)
	assert.Equal(t, "host_not_allowed", RebindingVerdict(CallerFacts{Host: "attacker.example:8543"}, cfg))
	cfg.VerifiedHosts = nil
	assert.Equal(t, "host_not_allowed", RebindingVerdict(CallerFacts{Host: "onyx.lan"}, cfg))
}

// T-RO-34 / T-RP-56: on the authenticated chain the Host test is skipped and the
// same-origin Origin test stays.
func TestRebindingVerdict_should_SkipHostTestButKeepOriginTest_WhenAuthRequired(t *testing.T) {
	cfg := verdictCfg("localhost:8543", nil, nil)
	assert.Equal(t, "", RebindingVerdict(CallerFacts{Host: "onyx.staplerhome.internal:8444", AuthRequired: true}, cfg))
	assert.Equal(t, "", RebindingVerdict(CallerFacts{Host: "100.64.0.9:8444", AuthRequired: true}, cfg), "IP-literal Host on :8444")
	assert.Equal(t, "", RebindingVerdict(CallerFacts{Host: "onyx.staplerhome.internal:8444", Origin: "https://onyx.staplerhome.internal:8444", AuthRequired: true}, cfg))
	assert.Equal(t, "origin_not_allowed", RebindingVerdict(CallerFacts{Host: "onyx.staplerhome.internal:8444", Origin: "https://evil.example", AuthRequired: true}, cfg))
}

func TestLocalCallerVerdict_should_RefuseRemotePeerOrProxiedLoopbackUnlessAuthRequired(t *testing.T) {
	assert.Equal(t, "", LocalCallerVerdict(CallerFacts{PeerAddr: "127.0.0.1:50000"}))
	assert.Equal(t, "", LocalCallerVerdict(CallerFacts{PeerAddr: "[::1]:50000"}))
	assert.Equal(t, "", LocalCallerVerdict(CallerFacts{PeerAddr: "[::ffff:127.0.0.1]:50000"}))
	assert.Equal(t, "peer_not_loopback", LocalCallerVerdict(CallerFacts{PeerAddr: "192.168.1.20:50000"}))
	assert.Equal(t, "peer_not_loopback", LocalCallerVerdict(CallerFacts{PeerAddr: ""}))
	assert.Equal(t, "proxy_header", LocalCallerVerdict(CallerFacts{PeerAddr: "127.0.0.1:50000", Proxied: true}))
	assert.Equal(t, "", LocalCallerVerdict(CallerFacts{PeerAddr: "192.168.1.20:50000", Proxied: true, AuthRequired: true}))
}

func TestProxyHeaderHelpers_should_DetectHeadersAndFamilies(t *testing.T) {
	h := http.Header{}
	assert.False(t, HasProxyHeader(h))
	h.Set("X-Forwarded-For", "10.0.0.1")
	assert.True(t, HasProxyHeader(h))
	assert.Equal(t, "X-Forwarded-For", ProxyHeaderName(h))

	ts := http.Header{}
	ts.Set("Tailscale-User-Login", "a@b.c")
	assert.True(t, HasProxyHeader(ts))

	empty := http.Header{"X-Real-Ip": nil}
	assert.False(t, HasProxyHeader(empty), "a header with no value is not present")
}

type rebindingHarness struct {
	cfg     VerdictConfig
	reached int
}

func (h *rebindingHarness) do(method, host, origin string) int {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h.reached++
		w.WriteHeader(http.StatusOK)
	})
	guard := LocalWriteGuard(map[string]GuardProfile{testRebindingPath: ProfileRebinding}, LocalWriteGuardConfig{Rebinding: h.cfg})(next)
	r := httptest.NewRequest(method, testRebindingPath, strings.NewReader("{}"))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, r)
	return w.Code
}

// The procedure-level guard with the rebinding profile: verified LAN host works
// on a wildcard bind (where ProfileProbe refuses), rebinding is 403.
func TestLocalWriteGuard_should_ApplyRebindingProfileWithoutLoopbackBoundCondition(t *testing.T) {
	h := &rebindingHarness{cfg: verdictCfg("0.0.0.0:8543", []string{"onyx.lan"}, nil)}
	assert.Equal(t, http.StatusOK, h.do(http.MethodPost, "onyx.lan:8543", "http://onyx.lan:8543"))
	assert.Equal(t, http.StatusOK, h.do(http.MethodPost, "localhost:8543", ""))
	assert.Equal(t, 2, h.reached)

	assert.Equal(t, http.StatusForbidden, h.do(http.MethodPost, "evil.example:8543", ""))
	assert.Equal(t, http.StatusForbidden, h.do(http.MethodPost, "onyx.lan:8543", "https://evil.example"))
	assert.Equal(t, http.StatusMethodNotAllowed, h.do(http.MethodGet, "onyx.lan:8543", ""))
	assert.Equal(t, 2, h.reached)
}

// The same wildcard bind is refused by the probe profile, unchanged.
func TestLocalWriteGuard_should_KeepProbeProfileRefusingNonLoopbackBound(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	guard := LocalWriteGuard(map[string]GuardProfile{testProbePath: ProfileProbe}, LocalWriteGuardConfig{
		Probe: ProbeGuardConfig{LoopbackBound: func() bool { return false }},
	})(next)
	r := httptest.NewRequest(http.MethodPost, testProbePath, strings.NewReader("{}"))
	r.Host = "localhost:8543"
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
