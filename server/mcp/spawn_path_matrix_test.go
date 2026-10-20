package mcp

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// Spawn-path matrix of Story 4.9 (ADR-007 rule 0): for each way a client can
// reach /mcp, what the identity middleware concludes. A row is "verified" only
// when the client really presents a proof that verifies for its own UUID.

const matrixMCPURL = "http://localhost:8543/mcp"

type matrixEnv struct {
	dir    string
	proofs *services.McpProofs
	mw     http.Handler
	probe  *identityProbe
}

// newMatrixEnv installs a real McpProofs provider (which writes proof files)
// and returns the middleware every row is sent through. Process-global state
// (provider, env, state dir) makes these tests serial.
func newMatrixEnv(t *testing.T) *matrixEnv {
	t.Helper()
	dir := envtest.NewIsolatedStateDir(t)
	keys, err := services.NewHookProofs(dir)
	require.NoError(t, err)
	proofs := services.NewMcpProofs(keys, dir)
	session.SetMCPProofProvider(proofs.Provider())
	t.Cleanup(func() { session.SetMCPProofProvider(nil) })
	probe := &identityProbe{}
	return &matrixEnv{
		dir: dir, proofs: proofs, probe: probe,
		mw: NewIdentityMiddleware(probe, proofs.Verify, func(string) bool { return true }),
	}
}

// send sends one request carrying headers and reports the verified identity.
func (e *matrixEnv) send(t *testing.T, headers map[string]string) (verified string) {
	t.Helper()
	*e.probe = identityProbe{}
	probeRequest(t, e.mw, headers)
	return e.probe.verified
}

func (e *matrixEnv) launch(t *testing.T, program, uuid, resume string, mcpURL string) string {
	t.Helper()
	inst := &session.Instance{Program: program, UUID: uuid}
	inst.SetMCPServerURLProvider(func() string { return mcpURL })
	return inst.BuildLaunchCommandForTest(resume)
}

var mcpConfigFlag = regexp.MustCompile(`--mcp-config ('(?:[^']|'\\'')*')`)

// claudeConfigHeaders returns the headers of the stapler-squad entry a Claude
// session would load from its launch command's --mcp-config (a file path, or
// inline JSON for the fail-closed fallback).
func claudeConfigHeaders(t *testing.T, cmd string) map[string]string {
	t.Helper()
	m := mcpConfigFlag.FindStringSubmatch(cmd)
	require.NotNil(t, m, "no --mcp-config in %q", cmd)
	val := strings.Trim(m[1], "'")
	raw := []byte(val)
	if !strings.HasPrefix(val, "{") {
		var err error
		raw, err = os.ReadFile(val)
		require.NoError(t, err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(raw, &cfg))
	return cfg.MCPServers["stapler-squad"].Headers
}

// openCodeConfigHeaders mimics OpenCode loading OPENCODE_CONFIG, including its
// {file:path} substitution.
func openCodeConfigHeaders(t *testing.T, cmd string) map[string]string {
	t.Helper()
	m := regexp.MustCompile(`^OPENCODE_CONFIG='([^']+)' `).FindStringSubmatch(cmd)
	require.NotNil(t, m, "no OPENCODE_CONFIG in %q", cmd)
	raw, err := os.ReadFile(m[1])
	require.NoError(t, err)
	var cfg struct {
		MCP map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcp"`
	}
	require.NoError(t, json.Unmarshal(raw, &cfg))
	headers := cfg.MCP["stapler-squad"].Headers
	for k, v := range headers {
		if path, ok := strings.CutSuffix(strings.TrimPrefix(v, "{file:"), "}"); ok && strings.HasPrefix(v, "{file:") {
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			headers[k] = string(content)
		}
	}
	return headers
}

func TestSpawnPathMatrix_ClaudeTmuxSessions_should_Verify(t *testing.T) {
	e := newMatrixEnv(t)
	for name, resume := range map[string]string{"work or hidden or one-shot": "", "resumed": "conversation-123"} {
		uuid := "claude-" + strings.ReplaceAll(name, " ", "-")
		cmd := e.launch(t, "claude", uuid, resume, matrixMCPURL)

		headers := claudeConfigHeaders(t, cmd)
		assert.Equal(t, uuid, e.send(t, headers), name)
		assert.NotContains(t, cmd, headers[session.MCPProofHeader], "proof never in argv: %s", name)
	}
}

func TestSpawnPathMatrix_ClaudeStdioProxy_should_VerifyWhenProofFilePresent(t *testing.T) {
	e := newMatrixEnv(t)
	e.launch(t, "claude", "claude-proxy", "", matrixMCPURL) // the provider writes the proof file at spawn
	t.Setenv("STAPLER_SESSION_UUID", "claude-proxy")

	headers := ProxyHeaders()
	assert.Equal(t, "claude-proxy", e.send(t, headers))

	require.NoError(t, os.Remove(session.MCPProofFilePath(e.dir, "claude-proxy")))
	headers = ProxyHeaders()
	assert.NotContains(t, headers, session.MCPProofHeader, "no proof file, no header")
	assert.Equal(t, "", e.send(t, headers))
	assert.Equal(t, "claude-proxy", e.probe.claimed, "pre-existing tools still see the claimed uuid")
}

func TestSpawnPathMatrix_OpenCode_should_VerifyThroughPerSessionConfigFile(t *testing.T) {
	e := newMatrixEnv(t)
	cmd := e.launch(t, "opencode", "oc-1", "", matrixMCPURL)
	require.NoError(t, e.proofs.EnsureFile("oc-1"))

	headers := openCodeConfigHeaders(t, cmd)
	assert.Equal(t, "oc-1", e.send(t, headers))
	assert.NotContains(t, cmd, headers[session.MCPProofHeader], "proof never in argv")
	assert.Equal(t, "", e.send(t, map[string]string{"X-Stapler-Session-UUID": "oc-1"}), "forged uuid without the proof")
}

func TestSpawnPathMatrix_HarnessesWithoutProofSupport_should_StayUnverifiedWithNoProofFile(t *testing.T) {
	e := newMatrixEnv(t)
	for _, program := range []string{"pi", "gemini", "agy", "aider"} {
		uuid := "other-" + program
		cmd := e.launch(t, program, uuid, "", matrixMCPURL)
		assert.NotContains(t, cmd, "--mcp-config", program)
		assert.NotContains(t, cmd, "OPENCODE_CONFIG", program)
		assert.False(t, session.MCPProofFilePresent(e.dir, uuid), "%s must have no proof file", program)

		// Whatever it tries over the stdio proxy carries no proof.
		t.Setenv("STAPLER_SESSION_UUID", uuid)
		assert.Equal(t, "", e.send(t, ProxyHeaders()), program)
	}
}

func TestSpawnPathMatrix_SsqMuxSession_should_StayUnverified(t *testing.T) {
	e := newMatrixEnv(t)
	// External/IDE sessions are never launched by buildLaunchCommand, so no
	// provider call and no proof file exist for them.
	t.Setenv("STAPLER_SESSION_UUID", "mux-1")
	assert.False(t, session.MCPProofFilePresent(e.dir, "mux-1"))
	assert.Equal(t, "", e.send(t, ProxyHeaders()))
}

func TestSpawnPathMatrix_FailClosedPaths_should_StayUnverified(t *testing.T) {
	e := newMatrixEnv(t)

	// Forced write failure: a regular file where the mcp directory must be.
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "mcp"), []byte("x"), 0o600))
	cmd := e.launch(t, "claude", "claude-wf", "", matrixMCPURL)
	headers := claudeConfigHeaders(t, cmd)
	assert.NotContains(t, headers, session.MCPProofHeader)
	assert.Equal(t, "", e.send(t, headers))

	// Nil provider (boot window before wiring): the session keeps no proof.
	require.NoError(t, os.Remove(filepath.Join(e.dir, "mcp")))
	session.SetMCPProofProvider(nil)
	cmd = e.launch(t, "claude", "claude-nil", "", matrixMCPURL)
	headers = claudeConfigHeaders(t, cmd)
	assert.NotContains(t, headers, session.MCPProofHeader)
	assert.Equal(t, "", e.send(t, headers))
}

func TestSpawnPathMatrix_PreUpgradeSession_should_StayUnverifiedButKeepClaimedIdentity(t *testing.T) {
	e := newMatrixEnv(t)
	// A config file from before the upgrade: UUID header only.
	old := map[string]string{"X-Stapler-Session-UUID": "pre-upgrade"}
	assert.Equal(t, "", e.send(t, old))
	assert.Equal(t, "pre-upgrade", e.probe.claimed)
	assert.False(t, e.probe.verifiedOK)
}

func TestSpawnPathMatrix_ProofOfDeletedSession_should_NotVerify(t *testing.T) {
	e := newMatrixEnv(t)
	cmd := e.launch(t, "claude", "claude-del", "", matrixMCPURL)
	headers := claudeConfigHeaders(t, cmd)
	require.Equal(t, "claude-del", e.send(t, headers))

	live := map[string]bool{}
	mw := NewIdentityMiddleware(e.probe, e.proofs.Verify, func(id string) bool { return live[id] })
	*e.probe = identityProbe{}
	probeRequest(t, mw, headers)
	assert.False(t, e.probe.verifiedOK, "the still-HMAC-valid proof of a deleted session is refused")

	e.proofs.RemoveFiles("claude-del")
	assert.NoFileExists(t, session.MCPProofFilePath(e.dir, "claude-del"))
	assert.NoFileExists(t, filepath.Join(e.dir, "mcp", "claude-del.json"))
}

func TestSpawnPathMatrix_RunStdioInProcess_should_HaveNoVerifiedIdentity(t *testing.T) {
	t.Parallel()
	// In-process RunStdio has no HTTP middleware: only the unverified claim is set.
	_, ok := verifiedCallerSessionUUID(WithSessionUUID(t.Context(), "stdio-1"))
	assert.False(t, ok)
}
