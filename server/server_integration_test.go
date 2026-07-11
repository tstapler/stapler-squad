package server

import (
	"context"
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
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
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
func TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated(t *testing.T) {
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
func TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort(t *testing.T) {
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

// waitForResolvedAddr polls srv.GetAddr() until it reports a real, non-zero bound address
// (i.e. no longer the pre-bind "localhost:0" placeholder), failing the test on timeout.
func waitForResolvedAddr(t *testing.T, srv *Server, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var addr string
	for time.Now().Before(deadline) {
		addr = srv.GetAddr()
		if addr != "" && addr != "localhost:0" && !strings.HasSuffix(addr, ":0") {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected a real, non-zero OS-assigned port to be resolved within %s, got %q", timeout, addr)
	return ""
}

// waitForLiveInstance polls SessionService.FindLiveInstance until the session created above
// is registered in the live poller, failing the test on timeout.
func waitForLiveInstance(t *testing.T, deps *ServerDependencies, sessionID string, timeout time.Duration) *session.Instance {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if inst := deps.SessionService.FindLiveInstance(sessionID); inst != nil {
			return inst
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected session %q to appear in the live poller within %s", sessionID, timeout)
	return nil
}

// waitForPermissionRequestHookCommand polls settingsPath until CreateSession's asynchronous
// InjectHookConfig call (server/services/approval_handler.go) has written a PermissionRequest
// command-type hook entry, then returns its command string. Fails the test on timeout.
//
// Callers pass 60s, not the ~instant time InjectHookConfig itself takes: the write only
// happens after instance.Start(true) (real tmux session + process spawn) returns, and on a
// contended CI runner running the full `-race` suite in parallel that can take much longer
// than the file write itself. Observed CI flakiness at 30s (this test intermittently timed
// out waiting on scheduling, not on a real hang) motivated the wider budget.
func waitForPermissionRequestHookCommand(t *testing.T, settingsPath string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(settingsPath)
		if err == nil {
			var top map[string]json.RawMessage
			if jsonErr := json.Unmarshal(data, &top); jsonErr == nil {
				if hooksRaw, ok := top["hooks"]; ok {
					var hooks map[string]json.RawMessage
					if jsonErr := json.Unmarshal(hooksRaw, &hooks); jsonErr == nil {
						if prRaw, ok := hooks["PermissionRequest"]; ok {
							type hookEntry struct {
								Type    string `json:"type"`
								Command string `json:"command,omitempty"`
							}
							type hookMatcherGroup struct {
								Hooks []hookEntry `json:"hooks"`
							}
							var groups []hookMatcherGroup
							if jsonErr := json.Unmarshal(prRaw, &groups); jsonErr == nil {
								for _, g := range groups {
									for _, h := range g.Hooks {
										if h.Type == "command" && h.Command != "" {
											return h.Command
										}
									}
								}
							}
						}
					}
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected %s to contain a PermissionRequest command hook within %s", settingsPath, timeout)
	return ""
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
// these tests' assumption that cleanup is synchronous).
//
// Deliberately does not fail the test on timeout (t.Logf, not t.Fatalf/t.Errorf):
// this runs inside t.Cleanup, where the test's own PASS/FAIL verdict is already
// decided -- a slow-but-eventually-successful teardown shouldn't retroactively
// fail a test that otherwise passed. It still meaningfully reduces cross-test
// accumulation by not returning from Cleanup while teardown is still in flight.
func waitForTmuxTeardown(t *testing.T, inst *session.Instance, timeout time.Duration) {
	t.Helper()
	if inst == nil {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !inst.TmuxSessionExists() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Logf("tmux session for %q still reported alive %s after DeleteSession; teardown may still be in flight", inst.Title, timeout)
}
