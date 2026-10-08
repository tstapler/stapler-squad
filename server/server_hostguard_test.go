package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
)

// registeredPatterns lists every pattern registered on mux by walking the
// stdlib routing tree. It reads unexported net/http fields (routingNode:
// pattern, children, multiChild, emptyChild), so the caller asserts a sanity
// floor and the test fails loudly if a Go upgrade reshapes the tree.
func registeredPatterns(mux *http.ServeMux) []string {
	seen := map[string]struct{}{}
	var walk func(n reflect.Value)
	walk = func(n reflect.Value) {
		if n.Kind() == reflect.Ptr {
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
			r := httptest.NewRequest(method, path, strings.NewReader("{}"))
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
