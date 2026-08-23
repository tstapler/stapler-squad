package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// findFreePort asks the OS for an ephemeral port and immediately releases it, so
// tests exercising the "explicit, non-zero port" code path (as opposed to ":0")
// never hardcode a real, recognizable port number -- in particular, never the
// production stapler-squad port, which risks confusion with (or, if this test
// pattern is ever copied into code that actually binds, collision with) a real
// running instance on the developer's machine.
func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// installFakeClaudeBinary puts a fake `claude` executable at the front of PATH
// for the duration of the test (t.Setenv restores the original PATH on cleanup).
//
// classifyProgram (session/instance_tmux.go) matches on the literal "claude"
// basename to decide whether to build the --mcp-config flag these tests assert
// on, so Program must stay "claude" -- but CI runners don't have the real
// claude CLI installed. Without a stand-in, the tmux-spawned shell exits
// instantly ("claude: command not found"), and depending on a startup race for
// tmux's remain-on-exit default, that can kill the session before Start()'s
// own readiness check observes it -- CreateSession's async goroutine then
// returns early on that error, before ever calling InjectHookConfig, so
// waitForPermissionRequestHookCommand times out no matter how long its budget
// is (observed: failed at both 30s and 60s). The fake binary only needs to
// stay alive long enough for the session to be detected as live and for
// InjectHookConfig to run -- it does nothing else and is never invoked with
// real Claude Code behavior in mind.
func installFakeClaudeBinary(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nsleep 60\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatalf("installFakeClaudeBinary: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// buildRemoteTLSTestFixture generates a self-signed CA plus a leaf certificate
// (via this file's own generateCA/generateServerCert helpers in tls.go) for
// hostnames, and returns a server-side *tls.Config alongside the client-side
// *x509.CertPool that trusts it. Shared by the Task 1.1.1a tests below.
func buildRemoteTLSTestFixture(t *testing.T, hostnames []string) (*tls.Config, *x509.CertPool) {
	t.Helper()

	caKey, caCert, _, err := generateCA()
	require.NoError(t, err)

	certPEM, keyPEM, err := generateServerCert(caKey, caCert, hostnames)
	require.NoError(t, err)

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	return &tls.Config{Certificates: []tls.Certificate{cert}}, pool
}

// TestServer_should_NegotiateALPNHTTP2_When_StartRemoteServesOverRealTLS is a
// verification-only test (Task 1.1.1a, Story 1.1.1) confirming PRE-EXISTING
// stdlib behavior, not newly-built code: Go's net/http package automatically
// negotiates HTTP/2 via TLS ALPN for any TLS listener whose *http.Server sets
// neither Protocols nor TLSNextProto (this has been true since Go 1.6 -- see
// `go doc net/http`'s "Server ... automatically enable[s] HTTP/2 support when
// using HTTPS"). StartRemote (server.go:1378-1424) sets neither field, so this
// test exercises that automatic negotiation end-to-end through a real TLS
// listener rather than merely asserting it from the docs -- it is the
// evidence behind ADR-001's "zero server code change" claim for Epic 1.1.
func TestServer_should_NegotiateALPNHTTP2_When_StartRemoteServesOverRealTLS(t *testing.T) {
	srv, _ := newServerBase("localhost:0")
	// StartRemote reuses srv's shared mux (via srv.ServeHTTP) but does not
	// itself register /health -- that registration normally happens in
	// Start(), which this test does not call, so register it directly.
	srv.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	tlsCfg, caPool := buildRemoteTLSTestFixture(t, []string{"127.0.0.1", "localhost"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := findFreePort(t)
	remoteAddr := fmt.Sprintf("127.0.0.1:%d", port)
	require.NoError(t, srv.StartRemote(ctx, remoteAddr, tlsCfg, nil))

	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    caPool,
			ServerName: "127.0.0.1",
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}

	url := fmt.Sprintf("https://%s/health", remoteAddr)
	var resp *http.Response
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = client.Get(url) //nolint:noctx
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err, "expected StartRemote's TLS listener to become reachable")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 2, resp.ProtoMajor,
		"expected real ALPN-negotiated HTTP/2 (pre-existing stdlib behavior), got ProtoMajor=%d", resp.ProtoMajor)
}

// TestServer_should_RejectHTTP2PriorKnowledge_When_StartServesOverPlainHTTP is
// the companion error-path test (Task 1.1.1a, Story 1.1.1): the plain
// :8543-style listener (Server.Start, no TLS) must never negotiate cleartext
// HTTP/2 (h2c). This codebase deliberately never wires
// Protocols.SetUnencryptedHTTP2 anywhere (see the subtest below) -- no
// shipping browser implements h2c, so adding it would be dead complexity, not
// a capability. An h2c "prior knowledge" dial against the plain listener must
// therefore either fail outright or silently fall back to HTTP/1.1.
func TestServer_should_RejectHTTP2PriorKnowledge_When_StartServesOverPlainHTTP(t *testing.T) {
	t.Run("server.go_never_calls_SetUnencryptedHTTP2", func(t *testing.T) {
		data, err := os.ReadFile("server.go")
		require.NoError(t, err)
		assert.Equal(t, 0, strings.Count(string(data), "SetUnencryptedHTTP2"),
			"server.go must never wire cleartext HTTP/2 (h2c) -- no shipping browser implements it, "+
				"so this plan rejects h2c as a mechanism rather than merely deferring it")
	})

	srv, _ := newServerBase("localhost:0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()
	defer func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Errorf("server did not shut down within 5s")
		}
	}()

	var addr string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		addr = srv.GetAddr()
		if addr != "" && addr != "localhost:0" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotEmpty(t, addr, "expected Start() to resolve a real bound address")

	// h2c "prior knowledge" dial: send the HTTP/2 client preface directly over
	// a plain (non-TLS) connection, exactly as a client that assumed h2c
	// support would. AllowHTTP + a DialTLSContext override that returns a
	// plain net.Conn is the documented way to do this with http2.Transport.
	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	resp, err := client.Get("http://" + addr + "/health") //nolint:noctx
	if err != nil {
		// The connection failed outright -- expected, since this codebase
		// wires no h2c support anywhere.
		return
	}
	defer resp.Body.Close()
	assert.Equal(t, 1, resp.ProtoMajor,
		"expected HTTP/1.1 fallback since h2c is not wired on the plain listener, got ProtoMajor=%d", resp.ProtoMajor)
}

// Server_should_ServeHealthCheck_When_StartedWithPortZero (REQ-1 test #4).
//
// Real net.Listen("tcp","localhost:0"), real Serve(ln), real
// http.Get(addr+"/health") -> 200, asserting the resolved port is non-zero.
// This is the end-to-end regression test for Task 1.1.1a's listener change:
// binding an OS-assigned port must both update GetAddr() to the real bound
// address and actually serve requests on it.
func TestServer_should_ServeHealthCheck_When_StartedWithPortZero(t *testing.T) {
	srv, _ := newServerBase("localhost:0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Errorf("server did not shut down within 5s")
		}
	}()

	// Wait for the real bound address to be resolved and reachable.
	deadline := time.Now().Add(5 * time.Second)
	var addr string
	var resp *http.Response
	var lastErr error
	for time.Now().Before(deadline) {
		addr = srv.GetAddr()
		if addr != "" && addr != "localhost:0" {
			resp, lastErr = http.Get("http://" + addr + "/health")
			if lastErr == nil {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	if lastErr != nil || resp == nil {
		t.Fatalf("expected /health to become reachable on the resolved address %q, last error: %v", addr, lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /health to return 200, got %d", resp.StatusCode)
	}

	if strings.HasSuffix(addr, ":0") || addr == "" {
		t.Fatalf("expected a real, non-zero OS-assigned port to be resolved, got %q", addr)
	}
}

// Server_should_AllowCrossOriginRequest_When_ExtraOriginConfiguredViaEnvVar
// (REQ-4 test #4).
//
// This mirrors the caller-side wiring main.go performs when
// STAPLER_SQUAD_INSTANCE is set and STAPLER_SQUAD_EXTRA_ORIGINS names a
// well-formed localhost origin: the extra origin is appended to
// srv.GetOrigins() before Start(). It then issues a real HTTP request with
// that Origin header set and asserts the server reflects it back via
// Access-Control-Allow-Origin (proving the request is not rejected by CORS),
// exercising the real middleware.CORSWithOrigins chain wired in Start().
func TestServer_should_AllowCrossOriginRequest_When_ExtraOriginConfiguredViaEnvVar(t *testing.T) {
	const extraOrigin = "http://localhost:54212"
	t.Setenv("STAPLER_SQUAD_EXTRA_ORIGINS", extraOrigin)

	srv, _ := newServerBase("localhost:0")

	// Mirror main.go's append-to-existing-origins pattern: a single, already
	// well-formed entry from the env var, appended via GetOrigins()/SetOrigins().
	envOrigins := strings.Split(os.Getenv("STAPLER_SQUAD_EXTRA_ORIGINS"), ",")
	for i := range envOrigins {
		envOrigins[i] = strings.TrimSpace(envOrigins[i])
	}
	srv.SetOrigins(append(srv.GetOrigins(), envOrigins...))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Errorf("server did not shut down within 5s")
		}
	}()

	// Wait for the real bound address to be resolved and reachable.
	deadline := time.Now().Add(5 * time.Second)
	var addr string
	var resp *http.Response
	var lastErr error
	for time.Now().Before(deadline) {
		addr = srv.GetAddr()
		if addr != "" && addr != "localhost:0" {
			req, reqErr := http.NewRequest(http.MethodGet, "http://"+addr+"/health", nil)
			if reqErr != nil {
				lastErr = reqErr
				break
			}
			req.Header.Set("Origin", extraOrigin)
			resp, lastErr = http.DefaultClient.Do(req)
			if lastErr == nil {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	if lastErr != nil || resp == nil {
		t.Fatalf("expected /health to become reachable on the resolved address %q, last error: %v", addr, lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the cross-origin request to succeed (not be rejected), got status %d", resp.StatusCode)
	}

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != extraOrigin {
		t.Fatalf("expected Access-Control-Allow-Origin to reflect the trusted extra origin %q, got %q", extraOrigin, got)
	}
}

// Server_should_KeepSingleOriginAllowlist_When_ExtraOriginsEnvVarUnset
// (REQ-4 test #5 — regression).
//
// With STAPLER_SQUAD_EXTRA_ORIGINS unset, srv.GetOrigins() must remain exactly
// the single origin set by the caller (mirroring main.go's
// srv.SetOrigins([]string{localOrigin}) call) — no widening occurs, and a
// request from an origin outside that single-entry allowlist gets no
// Access-Control-Allow-Origin header, proving CORS behavior is unchanged from
// today's single-origin allowlist.
func TestServer_should_KeepSingleOriginAllowlist_When_ExtraOriginsEnvVarUnset(t *testing.T) {
	os.Unsetenv("STAPLER_SQUAD_EXTRA_ORIGINS") //nolint:errcheck // defensive: ensure no stray export leaks into this test
	if _, present := os.LookupEnv("STAPLER_SQUAD_EXTRA_ORIGINS"); present {
		t.Fatal("expected STAPLER_SQUAD_EXTRA_ORIGINS to be unset for this regression test")
	}

	const localOrigin = "http://localhost:54211"
	srv, _ := newServerBase("localhost:0")
	srv.SetOrigins([]string{localOrigin})

	if got := srv.GetOrigins(); len(got) != 1 || got[0] != localOrigin {
		t.Fatalf("expected GetOrigins() to remain the single default origin [%q], got %v", localOrigin, got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Errorf("server did not shut down within 5s")
		}
	}()

	// Wait for the real bound address to be resolved and reachable.
	deadline := time.Now().Add(5 * time.Second)
	var addr string
	var resp *http.Response
	var lastErr error
	for time.Now().Before(deadline) {
		addr = srv.GetAddr()
		if addr != "" && addr != "localhost:0" {
			req, reqErr := http.NewRequest(http.MethodGet, "http://"+addr+"/health", nil)
			if reqErr != nil {
				lastErr = reqErr
				break
			}
			req.Header.Set("Origin", "http://localhost:54212") // not in the allowlist
			resp, lastErr = http.DefaultClient.Do(req)
			if lastErr == nil {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	if lastErr != nil || resp == nil {
		t.Fatalf("expected /health to become reachable on the resolved address %q, last error: %v", addr, lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected the request to still succeed (CORS only omits headers, it doesn't reject), got status %d", resp.StatusCode)
	}

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin header for an untrusted origin, got %q", got)
	}
}

// TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated
// (REQ-3 test #5 -- the literal Story 1.3.1 acceptance criterion, and the regression test
// for the early-binding bug found in architecture-review.md).
//
// Starts the real server with PORT=0 (an OS-assigned port), creates a real Claude Code
// session against it, and asserts that BOTH the MCP server URL (Epic 1.1, Task 1.1.1c,
// session.Instance.MCPServerURL) and the PermissionRequest hook URL written into that
// session's .claude/settings.local.json (Epic 1.3, Story 1.3.1) contain the real OS-assigned
// port -- never the literal ":0" that a construction-time-baked string would have produced.
// This exercises the real wireDepsIntoServer wiring end to end: the shared baseURLFn closure
// threaded into NewApprovalHandler and SetHookBaseURLFn, each re-evaluated lazily against
// srv.GetAddr() at hook-injection time (never snapshotted before Start() resolves the port).
//
// Flake repro: to reproduce Race B (marginal-timeout-under-load, see
// project_plans/flaky-hook-url-tests/requirements.md and
// project_plans/ci-hookurl-race-flake/requirements.md) locally with
// artificial CPU contention approximating CI's 4-vCPU runner under full
// `-race` load:
//
//	yes > /dev/null & yes > /dev/null & yes > /dev/null &
//	TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 \
//	  -run 'TestServer_should_Write.*(HookURL|MCPURL)' ./server/...
//	kill %1 %2 %3
func TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated(t *testing.T) {
	installFakeClaudeBinary(t)
	// See STAPLER_SQUAD_TEST_DIR comment on TestSessionService_CreateThenImmediateDelete_NoDataRace
	// below: without a per-test SQLite file, this test contends on the shared PID-scoped store
	// with every other concurrently-running test in this package during a full-suite run, which
	// can push the 60s hook-write poll in waitForPermissionRequestHookCommand past its budget.
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}

	srv := NewServerWithDeps("localhost:0", deps)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()
	defer func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Errorf("server did not shut down within 5s")
		}
	}()

	// Wait for the real bound address to be resolved (mirrors the PORT=0 tests above).
	addr := waitForResolvedAddr(t, srv, 10*time.Second)

	// The session title must be unique per invocation, not just per test: config
	// test-mode state (config.GetConfigDir()) is keyed by os.Getpid(), and
	// DeleteSession's cleanup tears the tmux session down via an unawaited
	// goroutine, so every test in this binary (across `-count` repeats, or
	// simply within one CI run of the full suite) shares process-scoped state.
	// A hardcoded title lets a later run collide with a still-tearing-down
	// session from an earlier one (CodeAlreadyExists, or a stall behind stale
	// state that can exceed this test's 15s wait) -- observed as intermittent
	// "timed out waiting for tmux session" CI failures.
	title := fmt.Sprintf("hook-url-port-zero-test-%d", time.Now().UnixNano())
	resp, err := deps.SessionService.CreateSession(ctx, connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   title,
		Path:    t.TempDir(),
		Program: "claude",
	}))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessionID := resp.Msg.Session.Id
	var inst *session.Instance
	t.Cleanup(func() {
		_, _ = deps.SessionService.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: sessionID}))
		waitForTmuxTeardown(t, inst, 5*time.Second)
	})

	// CreateSession starts the instance and injects hook config asynchronously; wait for both.
	inst = waitForLiveInstance(t, deps, sessionID, 30*time.Second)
	settingsPath := filepath.Join(inst.GetEffectiveRootDir(), ".claude", "settings.local.json")
	hookCmd := waitForPermissionRequestHookCommand(t, settingsPath, 60*time.Second)

	if strings.Contains(hookCmd, "localhost:0/") {
		t.Fatalf("expected the PermissionRequest hook command to never contain the unresolved :0 port, got %q", hookCmd)
	}
	if !strings.Contains(hookCmd, addr) {
		t.Fatalf("expected the PermissionRequest hook command to contain the real bound address %q, got %q", addr, hookCmd)
	}

	if inst.MCPServerURL == "" {
		t.Fatal("expected inst.MCPServerURL (mcpServerURLFn's resolved value) to be populated")
	}
	if strings.Contains(inst.MCPServerURL, "localhost:0/") {
		t.Fatalf("expected MCPServerURL to never contain the unresolved :0 port, got %q", inst.MCPServerURL)
	}
	if !strings.Contains(inst.MCPServerURL, addr) {
		t.Fatalf("expected MCPServerURL to contain the real bound address %q, got %q", addr, inst.MCPServerURL)
	}
}

// TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort
// (REQ-3 test #6 -- regression: explicit port path, no PORT=0).
//
// Wires a server addressed at an explicit, non-zero port obtained from the OS via
// findFreePort (without binding the real OS port on the *server* object itself --
// srv.GetAddr() returns the configured addr regardless of whether Start() has run,
// and an explicit non-zero port is never rewritten by Start()'s listener bind, so
// this exercises the same lazy-read path). Using a freshly-discovered ephemeral
// port here -- rather than hardcoding the real production stapler-squad port --
// means this test can never be mistaken for (or accidentally start actually
// colliding with, if this pattern is ever copied into code that does call Start())
// the developer's own already-running instance. Asserts the PermissionRequest hook
// URL injected for a new session is still exactly
// http://localhost:<port>/api/hooks/permission-request -- proving Epic 1.3's lazy
// baseURLFn switch didn't regress the default/production instance.
//
// Flake repro: to reproduce Race B (marginal-timeout-under-load, see
// project_plans/flaky-hook-url-tests/requirements.md and
// project_plans/ci-hookurl-race-flake/requirements.md) locally with
// artificial CPU contention approximating CI's 4-vCPU runner under full
// `-race` load:
//
//	yes > /dev/null & yes > /dev/null & yes > /dev/null &
//	TMUX_BIN="$(pwd)/bin/tmux" go test -race -count=10 \
//	  -run 'TestServer_should_Write.*(HookURL|MCPURL)' ./server/...
//	kill %1 %2 %3
func TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort(t *testing.T) {
	installFakeClaudeBinary(t)
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}

	port := findFreePort(t)
	addr := fmt.Sprintf("localhost:%d", port)
	srv := NewServerWithDeps(addr, deps)
	if got := srv.GetAddr(); got != addr {
		t.Fatalf("expected GetAddr() to report the configured explicit address before Start(), got %q, want %q", got, addr)
	}

	// Unique per invocation -- see the comment on the equivalent line in
	// TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated
	// above for why a hardcoded title causes intermittent CI flakiness.
	title := fmt.Sprintf("hook-url-explicit-port-test-%d", time.Now().UnixNano())
	resp, err := deps.SessionService.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   title,
		Path:    t.TempDir(),
		Program: "claude",
	}))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	sessionID := resp.Msg.Session.Id
	var inst *session.Instance
	t.Cleanup(func() {
		_, _ = deps.SessionService.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: sessionID}))
		waitForTmuxTeardown(t, inst, 5*time.Second)
	})

	inst = waitForLiveInstance(t, deps, sessionID, 30*time.Second)
	settingsPath := filepath.Join(inst.GetEffectiveRootDir(), ".claude", "settings.local.json")
	hookCmd := waitForPermissionRequestHookCommand(t, settingsPath, 60*time.Second)

	wantURL := fmt.Sprintf("http://localhost:%d/api/hooks/permission-request", port)
	if !strings.Contains(hookCmd, wantURL) {
		t.Fatalf("expected the PermissionRequest hook command to contain the unchanged explicit-port URL %q, got %q", wantURL, hookCmd)
	}

	wantHost := fmt.Sprintf("localhost:%d", port)
	if inst.MCPServerURL != "" && !strings.Contains(inst.MCPServerURL, wantHost) {
		t.Fatalf("expected MCPServerURL to remain on the unchanged explicit port when set, got %q", inst.MCPServerURL)
	}
}

// TestSessionService_CreateThenImmediateDelete_NoDataRace deliberately does NOT call
// waitForLiveInstance before deleting: CreateSession's async controller-start goroutine
// (which calls GetPTY()) is very likely still in flight when DeleteSession's cleanup
// goroutine runs closePTYAndAttachCmd(). This is the exact interleave from the original
// -race report. This test is a realistic, best-effort repro under real scheduler timing
// (relying on -count=N outer repetition, not in-test concurrency, to raise the odds of
// landing in the window across runs -- an earlier version raced several concurrent
// create/delete pairs within one run, but that tripped the session store's SQLite
// busy_timeout under -race's slowdown, an unrelated flake this test must not introduce).
// The deterministic proof that the fix is correct is
// TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized in session/tmux/tmux_test.go,
// which forces the interleave directly instead of relying on timing.
//
// STAPLER_SQUAD_TEST_DIR gives this test its own SQLite file instead of the shared
// per-process one config.IsTestMode() falls back to (keyed only by PID, not by test):
// without it, every -count=N outer repetition in the "PTY-triple race regression" CI
// step re-triggers the exact same busy_timeout flake the paragraph above says was
// already fixed once -- N independently-constructed BuildDependencies() clients (one
// per repetition, none of them closed) contend for one shared database file, which
// under -race's slowdown can exceed the 5s busy_timeout ("database is locked").
func TestSessionService_CreateThenImmediateDelete_NoDataRace(t *testing.T) {
	installFakeClaudeBinary(t)
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	deps, err := BuildDependencies()
	if err != nil {
		t.Fatalf("BuildDependencies: %v", err)
	}

	title := fmt.Sprintf("ptmx-race-repro-%d", time.Now().UnixNano())
	resp, err := deps.SessionService.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   title,
		Path:    t.TempDir(),
		Program: "claude",
	}))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Captured before DeleteSession purely so we can wait for teardown to finish
	// below -- this does not delay the delete itself, so it doesn't touch the
	// Create/Delete interleave this test exists to race.
	inst := deps.SessionService.FindLiveInstance(resp.Msg.Session.Id)

	if _, err := deps.SessionService.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: resp.Msg.Session.Id})); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	// DeleteSession tears down tmux/git resources in an unawaited goroutine (see
	// waitForTmuxTeardown's doc comment) -- without waiting here, that goroutine
	// can still be writing into this test's t.TempDir() (e.g. the tmux-exec-gate
	// lock directory) when TempDir's own cleanup runs RemoveAll on it, producing
	// an intermittent "directory not empty" failure. This wait happens after the
	// race has already occurred, so it doesn't defeat the repro above.
	waitForTmuxTeardown(t, inst, 5*time.Second)
}

// waitForResolvedAddr polls srv.GetAddr() until it reports a real, non-zero bound address
// (i.e. no longer the pre-bind "localhost:0" placeholder), failing the test on timeout.
func waitForResolvedAddr(t *testing.T, srv *Server, timeout time.Duration) string {
	t.Helper()
	var addr string
	ok := assert.Eventually(t, func() bool {
		addr = srv.GetAddr()
		return addr != "" && addr != "localhost:0" && !strings.HasSuffix(addr, ":0")
	}, timeout, 10*time.Millisecond)
	if !ok {
		t.Fatalf("expected a real, non-zero OS-assigned port to be resolved within %s, got %q", timeout, addr)
	}
	return addr
}

// waitForLiveInstance polls SessionService.FindLiveInstance until the session created above
// is registered in the live poller, failing the test on timeout.
func waitForLiveInstance(t *testing.T, deps *ServerDependencies, sessionID string, timeout time.Duration) *session.Instance {
	t.Helper()
	var inst *session.Instance
	require.Eventually(t, func() bool {
		if got := deps.SessionService.FindLiveInstance(sessionID); got != nil {
			inst = got
			return true
		}
		return false
	}, timeout, 20*time.Millisecond, "expected session %q to appear in the live poller", sessionID)
	return inst
}

// waitForPermissionRequestHookCommand polls settingsPath until CreateSession's asynchronous
// InjectHookConfig call (server/services/approval_handler.go) has written a PermissionRequest
// command-type hook entry, then returns its command string. Fails the test on timeout.
//
// Callers pass 60s, not the ~instant time InjectHookConfig itself takes: the write only
// happens after instance.Start(true) (real tmux session + process spawn) returns, and on a
// contended CI runner running the full `-race` suite in parallel that can take much longer
// than the file write itself. Observed CI flakiness at 30s (this test intermittently timed
// out waiting on scheduling, not on a real hang) motivated the wider budget. See the
// repro-command comments above the two call sites for a local stress-repro of the
// load-sensitive failure mode this budget accounts for.
func waitForPermissionRequestHookCommand(t *testing.T, settingsPath string, timeout time.Duration) string {
	t.Helper()
	var hookCmd string
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			return false
		}
		var top map[string]json.RawMessage
		if jsonErr := json.Unmarshal(data, &top); jsonErr != nil {
			return false
		}
		hooksRaw, ok := top["hooks"]
		if !ok {
			return false
		}
		var hooks map[string]json.RawMessage
		if jsonErr := json.Unmarshal(hooksRaw, &hooks); jsonErr != nil {
			return false
		}
		prRaw, ok := hooks["PermissionRequest"]
		if !ok {
			return false
		}
		type hookEntry struct {
			Type    string `json:"type"`
			Command string `json:"command,omitempty"`
		}
		type hookMatcherGroup struct {
			Hooks []hookEntry `json:"hooks"`
		}
		var groups []hookMatcherGroup
		if jsonErr := json.Unmarshal(prRaw, &groups); jsonErr != nil {
			return false
		}
		for _, g := range groups {
			for _, h := range g.Hooks {
				if h.Type == "command" && h.Command != "" {
					hookCmd = h.Command
					return true
				}
			}
		}
		return false
	}, timeout, 50*time.Millisecond, "expected %s to contain a PermissionRequest command hook", settingsPath)
	return hookCmd
}

// waitForTmuxTeardown polls inst.TmuxSessionExists() until the underlying tmux
// session is confirmed gone, or timeout elapses.
//
// Root cause this addresses: DeleteSession (server/services/session_service.go)
// intentionally destroys tmux/git resources in an unawaited goroutine so the RPC
// returns immediately -- correct for production UX, but it means a test's
// t.Cleanup(DeleteSession) can return before the real teardown finishes. Every
// integration test in this package shares one process-scoped, PID-keyed tmux
// server socket (session/tmux.testSocketOnce, gated by config.IsTestMode()) --
// by design, so isolated test tmux calls within one `go test` binary land on the
// same server. Without waiting here, a still-tearing-down session from an earlier
// test can pile up on that shared socket, and CI observed exactly this: repeated,
// intermittent "timed out waiting for tmux session" failures that got WORSE
// (monotonically increasing latency) the more CreateSession/DeleteSession cycles
// ran in the same process (reproduced locally with `go test -race -count=10`,
// and confirmed via bisection this predates PR #144's actual changes -- it's a
// pre-existing gap between DeleteSession's fire-and-forget teardown contract and
// these tests' assumption that cleanup is synchronous). If
// TestServer_should_Write*HookURL flakes again after this fix, check whether it's
// this shared-tmux-socket contention before re-tuning timeouts or CI topology --
// file a follow-up ticket for that root cause instead of widening a budget further.
//
// Deliberately does not fail the test on timeout (t.Logf, not t.Fatalf/t.Errorf):
// this runs inside t.Cleanup, where the test's own PASS/FAIL verdict is already
// decided -- a slow-but-eventually-successful teardown shouldn't retroactively
// fail a test that otherwise passed. It still meaningfully reduces cross-test
// accumulation by not returning from Cleanup while teardown is still in flight.
// Uses testutil/wait.WaitForCondition rather than require.Eventually because the
// latter calls t.FailNow()/t.Errorf() internally on timeout, which would break this
// helper's deliberate non-fatal contract.
func waitForTmuxTeardown(t *testing.T, inst *session.Instance, timeout time.Duration) {
	t.Helper()
	if inst == nil {
		return
	}
	err := wait.WaitForCondition(func() bool {
		return !inst.TmuxSessionExists()
	}, wait.WaitConfig{
		Timeout:      timeout,
		PollInterval: 20 * time.Millisecond,
		Description:  fmt.Sprintf("tmux teardown for %q", inst.Title),
	})
	if err != nil {
		t.Logf("tmux session for %q still reported alive %s after DeleteSession; teardown may still be in flight: %v", inst.Title, timeout, err)
	}
}
