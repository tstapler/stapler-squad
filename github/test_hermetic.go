package github

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// allowTestNetworkEnv mirrors envtest.AllowNetworkEnv (envtest is a test-helper
// package that production code should not import).
const allowTestNetworkEnv = "STAPLER_SQUAD_TEST_ALLOW_NETWORK"

// errTestNetworkDenied is returned by loopbackOnlyTransport for any
// non-loopback request made from a `go test` binary.
var errTestNetworkDenied = errors.New("github: non-loopback request refused in test binary")

// hermeticInTests reports whether this process is a `go test` binary that has
// not opted in to real network access.
func hermeticInTests() bool {
	return testing.Testing() && os.Getenv(allowTestNetworkEnv) == ""
}

func init() {
	// Test binaries must never read the developer's real OS keychain: server
	// and session tests start pollers that would otherwise authenticate to the
	// real GitHub API with the stored token. Tests that need tokens call
	// keyring.MockInit() themselves, which replaces this empty store.
	if hermeticInTests() {
		keyring.MockInit()
	}
}

// defaultBaseTransport is the innermost transport of ghHTTPClient: the real
// http.DefaultTransport in production, loopback-only in test binaries so a
// poller wired by an integration test cannot reach a real host. Tests that
// need a fake GitHub swap it via SetGHHTTPBaseTransportForTest or point
// GhBaseURL at an httptest server (loopback, so allowed).
func defaultBaseTransport() http.RoundTripper {
	if hermeticInTests() {
		return loopbackOnlyTransport{next: http.DefaultTransport}
	}
	return http.DefaultTransport
}

type loopbackOnlyTransport struct{ next http.RoundTripper }

func (t loopbackOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	if !isLoopbackHost(host) {
		return nil, fmt.Errorf("%w: %s", errTestNetworkDenied, host)
	}
	return t.next.RoundTrip(req)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
