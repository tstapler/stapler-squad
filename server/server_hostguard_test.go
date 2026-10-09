package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/auth"
	"github.com/tstapler/stapler-squad/server/middleware"
)

// registeredPatterns lists every pattern registered on mux by walking the
// stdlib routing tree. It reads unexported net/http fields (routingNode:
// pattern, children, multiChild, emptyChild), so the caller asserts a sanity
// floor and the test fails loudly if a Go upgrade reshapes the tree.
func registeredPatterns(mux *http.ServeMux) []string {
	seen := map[string]struct{}{}
	var walk func(n reflect.Value)
	walk = func(n reflect.Value) {
		if n.Kind() == reflect.Pointer {
			if n.IsNil() {
				return
			}
			n = n.Elem()
		}
		if p := n.FieldByName("pattern"); !p.IsNil() {
			seen[p.Elem().FieldByName("str").String()] = struct{}{}
		}
		walk(n.FieldByName("multiChild"))
		walk(n.FieldByName("emptyChild"))
		children := n.FieldByName("children")
		for i := 0; i < children.FieldByName("s").Len(); i++ {
			walk(children.FieldByName("s").Index(i).FieldByName("value"))
		}
		m := children.FieldByName("m")
		for _, k := range m.MapKeys() {
			walk(m.MapIndex(k))
		}
	}
	walk(reflect.ValueOf(mux).Elem().FieldByName("tree"))

	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	return out
}

var patternWildcard = regexp.MustCompile(`\{[^}]*\}`)

// requestFor turns a ServeMux pattern ("POST /a/{id}/b", "/c/") into a method and concrete path.
func requestFor(pattern string) (method, path string) {
	method = http.MethodGet
	if m, rest, ok := strings.Cut(pattern, " "); ok {
		method, pattern = m, strings.TrimLeft(rest, " ")
	}
	return method, patternWildcard.ReplaceAllString(pattern, "x")
}

// Every route on the unauthenticated :8543 listener must reject a DNS-rebinding
// Host and a same-box reverse-proxy Host. New routes inherit the guard because
// it wraps the whole mux; this test fails if one ever escapes it.
func TestLocalChain_should_Reject_RebindingAndProxyHosts_OnEveryRegisteredRoute(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	deps, err := BuildDependencies()
	require.NoError(t, err)
	srv := NewServerWithDeps("localhost:0", deps)
	t.Cleanup(func() { _ = srv.Shutdown() })

	patterns := registeredPatterns(srv.Mux())
	require.Greater(t, len(patterns), 40, "route enumeration found too few patterns; did net/http's routing tree change?")

	exempt := map[string]bool{}
	for _, p := range localExemptPaths {
		exempt[p] = true
	}
	badHosts := []string{
		"evil.example",
		"evil.example:8543",
		"8543--main--ws--user.coder.prod.netflix.net",
		"127.0.0.1.evil.example:8543",
	}
	chain := srv.localChain()
	for _, pattern := range patterns {
		method, path := requestFor(pattern)
		if exempt[path] {
			continue
		}
		for _, host := range badHosts {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			r := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader("{}"))
			defer cancel()
			r.Host = host
			w := httptest.NewRecorder()
			chain.ServeHTTP(w, r)
			assert.Equal(t, http.StatusForbidden, w.Code, "%s with Host %q was not rejected", pattern, host)
		}
	}
}

func TestLocalChain_should_Reject_ForeignOrigin_And_ExemptHealthOnly(t *testing.T) {
	srv, reached := newChainTestServer(t, "localhost:8543")
	srv.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	chain := srv.localChain()

	// Loopback Host with a foreign Origin (cross-site browser request).
	assert.Equal(t, http.StatusForbidden, postProbe(chain, "localhost:8543", "https://evil.example"))
	assert.Zero(t, *reached)

	// /health stays reachable under a foreign Host for load-balancer style probes.
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.Host = "evil.example"
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
}

// require_local_auth: a loopback request with a valid Host but no credential is
// rejected, the local API token and a login-code session cookie are accepted.
func TestLocalChain_should_RequireCredential_When_LocalAuthMiddlewareSet(t *testing.T) {
	srv, reached := newChainTestServer(t, "localhost:8543")
	sessions := auth.NewSessionManager("")
	t.Cleanup(sessions.Close)
	validator := auth.NewLocalValidator(sessions, "local-token")
	auth.RegisterLocalLoginRoutes(srv.Mux(), auth.NewLocalLogin(sessions, validator), true)
	srv.SetupAuth(middleware.Auth(validator))
	chain := srv.localChain()

	call := func(mutate func(*http.Request)) int {
		r := httptest.NewRequest(http.MethodPost, probeProcedurePath, strings.NewReader("{}"))
		r.Host = "localhost:8543"
		mutate(r)
		w := httptest.NewRecorder()
		chain.ServeHTTP(w, r)
		return w.Code
	}
	assert.Equal(t, http.StatusUnauthorized, call(func(*http.Request) {}))
	assert.Zero(t, *reached)
	assert.Equal(t, http.StatusUnauthorized, call(func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") }))
	assert.Equal(t, http.StatusOK, call(func(r *http.Request) { r.Header.Set("Authorization", "Bearer local-token") }))

	// Mint a code with the token, exchange it for a cookie, and use the cookie.
	mintReq := httptest.NewRequest(http.MethodPost, "/auth/local-login/code", nil)
	mintReq.Host = "localhost:8543"
	mintReq.Header.Set("Authorization", "Bearer local-token")
	mintRec := httptest.NewRecorder()
	chain.ServeHTTP(mintRec, mintReq)
	require.Equal(t, http.StatusOK, mintRec.Code)
	var body struct{ Code string }
	require.NoError(t, json.Unmarshal(mintRec.Body.Bytes(), &body))

	exReq := httptest.NewRequest(http.MethodGet, "/auth/local-login?code="+body.Code, nil)
	exReq.Host = "localhost:8543"
	exRec := httptest.NewRecorder()
	chain.ServeHTTP(exRec, exReq)
	require.Equal(t, http.StatusFound, exRec.Code)
	cookies := exRec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, http.StatusOK, call(func(r *http.Request) { r.AddCookie(cookies[0]) }))

	// A rebinding Host is still rejected even with the token.
	assert.Equal(t, http.StatusForbidden, call(func(r *http.Request) {
		r.Host = "evil.example"
		r.Header.Set("Authorization", "Bearer local-token")
	}))
}

// The exempt list is a security boundary: widening it silently opens routes.
func TestLocalExemptPaths_should_BeExactlyHealth(t *testing.T) {
	assert.Equal(t, []string{"/health"}, localExemptPaths)
}

// HostGuard's own Origin check, on a non-probe route so ProbeGuard can't mask it.
func TestLocalChain_should_Reject_ForeignOrigin_OnNonProbeRoute(t *testing.T) {
	srv, _ := newChainTestServer(t, "localhost:8543")
	srv.mux.HandleFunc("/api/anything", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	do := func(origin string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
		r.Host = "localhost:8543"
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		srv.localChain().ServeHTTP(w, r)
		return w.Code
	}
	assert.Equal(t, http.StatusForbidden, do("https://evil.example"))
	assert.Equal(t, http.StatusOK, do("http://localhost:8543"))
	assert.Equal(t, http.StatusOK, do(""))
}
