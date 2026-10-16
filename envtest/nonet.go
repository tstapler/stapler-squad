package envtest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"
)

// AllowNetworkEnv, when set to a non-empty value, disables DenyNonLoopbackNetwork
// so a developer can deliberately run a suite against real hosts.
const AllowNetworkEnv = "STAPLER_SQUAD_TEST_ALLOW_NETWORK"

// NetViolation is one blocked non-loopback dial.
type NetViolation struct {
	Network string
	Addr    string
	Stack   string
}

// ErrNonLoopbackDial is returned by the guarded dialer. Callers see it as an
// ordinary connection error, so production error paths run as they would
// offline.
var ErrNonLoopbackDial = errors.New("envtest: non-loopback dial blocked in test binary")

// NetGuard records and blocks non-loopback dials made through
// http.DefaultTransport. Transports built with their own net.Dialer (or
// subprocesses such as `gh`) are not covered; keep those out of tests by not
// giving them credentials (see github's test-mode keychain).
type NetGuard struct {
	mu         sync.Mutex
	violations []NetViolation
}

// DenyNonLoopbackNetwork installs the guard on http.DefaultTransport and
// returns it. Call it first thing in TestMain, before any request is made, and
// fail the run with Report if it returns a non-empty error.
func DenyNonLoopbackNetwork() *NetGuard {
	g := &NetGuard{}
	if os.Getenv(AllowNetworkEnv) != "" {
		return g
	}
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return g
	}
	inner := &net.Dialer{}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if isLoopbackAddr(addr) {
			return inner.DialContext(ctx, network, addr)
		}
		g.record(NetViolation{Network: network, Addr: addr, Stack: string(debug.Stack())})
		return nil, fmt.Errorf("%w: %s %s", ErrNonLoopbackDial, network, addr)
	}
	tr.DialTLSContext = nil
	return g
}

func (g *NetGuard) record(v NetViolation) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.violations = append(g.violations, v)
}

// Violations returns a copy of every blocked dial so far.
func (g *NetGuard) Violations() []NetViolation {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]NetViolation(nil), g.violations...)
}

// Report returns a multi-line description of blocked dials, or "" if none.
func (g *NetGuard) Report() string {
	vs := g.Violations()
	if len(vs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "test binary attempted %d non-loopback dial(s); tests must be hermetic:\n", len(vs))
	for _, v := range vs {
		fmt.Fprintf(&b, "\n  %s %s\n%s\n", v.Network, v.Addr, v.Stack)
	}
	return b.String()
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
