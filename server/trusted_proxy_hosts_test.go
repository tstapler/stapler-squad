package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/middleware"
)

const proxyHost = "8543--main--ws--me.apps.example.com"

func TestParseTrustedProxyHosts(t *testing.T) {
	t.Parallel()
	valid, rejected := ParseTrustedProxyHosts(" " + proxyHost + " , APPS.Example.com ,,https://a.example.com,b.example.com:8543,*.example.com,127.0.0.1,localhost,::1,-bad.example.com,a_b.example.com,evil.com/path")
	assert.Equal(t, []string{proxyHost, "apps.example.com"}, valid)
	assert.Equal(t, []string{"https://a.example.com", "b.example.com:8543", "*.example.com", "127.0.0.1", "localhost", "::1", "-bad.example.com", "a_b.example.com", "evil.com/path"}, rejected)
}

func TestHostGuard_should_AllowOnlyTrustedProxyHostAndItsOrigin(t *testing.T) {
	t.Parallel()
	s := &Server{}
	s.SetTrustedProxyHosts([]string{proxyHost})
	guarded := middleware.HostGuard(s.hostGuardConfig())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	tests := []struct {
		name         string
		host, origin string
		want         int
	}{
		{"trusted host, no origin", proxyHost, "", http.StatusOK},
		{"trusted host, its own https origin", proxyHost, "https://" + proxyHost, http.StatusOK},
		{"trusted host, foreign origin", proxyHost, "https://evil.example.com", http.StatusForbidden},
		{"trusted host, http origin is not trusted", proxyHost, "http://" + proxyHost, http.StatusForbidden},
		{"unlisted host", "evil.example.com", "", http.StatusForbidden},
		{"suffix of trusted host", "x." + proxyHost, "", http.StatusForbidden},
		{"loopback unchanged", "localhost:8543", "", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			guarded.ServeHTTP(rec, req)
			assert.Equal(t, tc.want, rec.Code)
		})
	}
}

func TestHostGuard_should_RejectProxyHostByDefault(t *testing.T) {
	t.Parallel()
	guarded := middleware.HostGuard((&Server{}).hostGuardConfig())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = proxyHost
	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestGuardSets_should_IncludeTrustedProxyHostWithoutTouchingDetectedHostnames(t *testing.T) {
	t.Parallel()
	s := &Server{}
	s.SetHostnames([]string{"box.lan"})
	s.SetTrustedProxyHosts([]string{proxyHost})

	assert.Equal(t, []string{"box.lan"}, s.GetHostnames(), "GetHostnames also feeds TLS SANs and QR codes")
	assert.Equal(t, []string{"box.lan", proxyHost}, s.guardHosts())
	assert.Contains(t, s.guardVerifiedHosts(), proxyHost)
	assert.Contains(t, s.guardOrigins(), "https://"+proxyHost)
	assert.Contains(t, s.LocalWriteVerdictConfig().VerifiedHosts(), proxyHost)
	assert.Contains(t, s.probeGuardConfig().AllowedHosts(), proxyHost)
}
