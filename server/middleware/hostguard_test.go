package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func hostGuardStatus(host, origin, path string) int {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := HostGuard(HostGuardConfig{ExemptPaths: []string{"/health"}})(next)
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestHostGuard_should_Reject_NonLoopbackHosts(t *testing.T) {
	for _, host := range []string{
		"evil.example", "evil.example:8543", "127.0.0.1.evil.example:8543", "localhost.evil.example",
		"8543--main--ws--user.coder.prod.netflix.net", "evil.com@localhost", "192.168.1.5:8543", "",
	} {
		assert.Equal(t, http.StatusForbidden, hostGuardStatus(host, "", "/api/x"), "host %q", host)
	}
}

func TestHostGuard_should_Pass_LoopbackHosts(t *testing.T) {
	for _, host := range []string{"localhost", "localhost:8543", "127.0.0.1:8543", "[::1]:8543", "localhost.:8543"} {
		assert.Equal(t, http.StatusOK, hostGuardStatus(host, "", "/api/x"), "host %q", host)
	}
}

func TestHostGuard_should_Reject_ForeignOrigin_OnLoopbackHost(t *testing.T) {
	for _, origin := range []string{"https://evil.example", "http://localhost.evil.example", "null"} {
		assert.Equal(t, http.StatusForbidden, hostGuardStatus("localhost:8543", origin, "/api/x"), "origin %q", origin)
	}
	assert.Equal(t, http.StatusOK, hostGuardStatus("localhost:8543", "http://localhost:8543", "/api/x"))
}

func TestHostGuard_should_ExemptOnlyExactPaths(t *testing.T) {
	assert.Equal(t, http.StatusOK, hostGuardStatus("evil.example", "", "/health"))
	assert.Equal(t, http.StatusForbidden, hostGuardStatus("evil.example", "", "/health/x"))
	assert.Equal(t, http.StatusForbidden, hostGuardStatus("evil.example", "", "//health"))
}

func TestHostGuard_should_PinLoopbackOriginToRequestPort(t *testing.T) {
	assert.Equal(t, http.StatusOK, hostGuardStatus("localhost:8543", "http://127.0.0.1:8543", "/api/x"))
	assert.Equal(t, http.StatusForbidden, hostGuardStatus("localhost:8543", "http://localhost:9999", "/api/x"), "other localhost port is a different origin")
	assert.Equal(t, http.StatusForbidden, hostGuardStatus("localhost:8543", "http://localhost", "/api/x"))
}

func TestHostGuard_should_AllowExactlyListedOrigin_OnAnyPort(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := HostGuard(HostGuardConfig{AllowedOrigins: func() []string { return []string{"http://localhost:3000"} }})(next)
	do := func(origin string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
		r.Host = "localhost:8543"
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	assert.Equal(t, http.StatusOK, do("http://localhost:3000"))
	assert.Equal(t, http.StatusForbidden, do("http://localhost:3001"))
}
