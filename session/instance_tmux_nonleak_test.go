package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session/launchcmd"
)

const (
	testMCPProof = MCPProofValuePrefix + "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	testMCPURL   = "http://localhost:8543/mcp"
	testToken    = "s3cret-token"
)

// installMCPProofProvider wires a provider that mints testMCPProof, optionally
// writing the proof file the way server/services.McpProofs does, and counts calls.
func installMCPProofProvider(t *testing.T, writeFile bool) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	SetMCPProofProvider(func(sessionUUID string) (string, bool) {
		calls.Add(1)
		if writeFile {
			dir, err := config.GetConfigDir()
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, MCPProofsDirName), 0o700))
			require.NoError(t, os.WriteFile(MCPProofFilePath(dir, sessionUUID), []byte(testMCPProof), 0o600))
		}
		return testMCPProof, true
	})
	t.Cleanup(func() { SetMCPProofProvider(nil) })
	return &calls
}

func readMCPConfigHeaders(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg struct {
		MCPServers map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &cfg))
	return cfg.MCPServers["stapler-squad"].Headers
}

func requirePerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, info.Mode().Perm(), path)
}

// mcpArgs and settingsArgs flatten the typed flag/value pairs back to the
// strings the assertions below were written against.
func mcpArgs(f launchcmd.Flag, v launchcmd.Literal) (string, string) { return f.Name(), string(v) }

func settingsArgs(f launchcmd.Flag, v launchcmd.Literal, ok bool) (string, string) {
	if !ok {
		return "", ""
	}
	return f.Name(), string(v)
}

func TestClaudeMCPConfigArgs_should_UseFileRoute_When_ProofPresentWithoutToken(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	inst := &Instance{Program: "claude", UUID: "uuid-proof"}

	flag, val := mcpArgs(inst.claudeMCPConfigArgsWithCredentials(testMCPURL, dir, mcpCredentials{Proof: testMCPProof}))

	require.Equal(t, "--mcp-config", flag)
	assert.NotContains(t, val, testMCPProof, "proof must not reach argv")
	path := strings.Trim(val, "'")
	assert.Equal(t, filepath.Join(dir, "mcp", "uuid-proof.json"), path)
	requirePerm(t, path, 0o600)
	requirePerm(t, filepath.Dir(path), 0o700)
	headers := readMCPConfigHeaders(t, path)
	assert.Equal(t, testMCPProof, headers[MCPProofHeader])
	assert.Equal(t, "uuid-proof", headers["X-Stapler-Session-UUID"])
	assert.NotContains(t, headers, "Authorization")
}

func TestClaudeMCPConfigArgs_should_FailClosed_When_ConfigFileCannotBeWritten(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	// A regular file where the mcp directory must be makes every write fail.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mcp"), []byte("not a dir"), 0o600))
	inst := &Instance{Program: "claude", UUID: "uuid-fail"}

	_, val := mcpArgs(inst.claudeMCPConfigArgsWithCredentials(testMCPURL, dir, mcpCredentials{Token: testToken, Proof: testMCPProof}))

	assert.NotContains(t, val, testMCPProof)
	assert.NotContains(t, val, testToken)
	assert.NotContains(t, val, "Authorization")
	assert.NotContains(t, val, MCPProofHeader)
	assert.Contains(t, val, "uuid-fail", "the unauthenticated session UUID header still travels inline")
}

func TestClaudeMCPConfigArgs_should_OmitProofHeader_When_NoProviderWired(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	inst := &Instance{Program: "claude", UUID: "uuid-noprov"}

	_, val := mcpArgs(inst.claudeMCPConfigArgsWithToken(testMCPURL, dir, ""))
	assert.NotContains(t, val, MCPProofHeader)

	_, val = mcpArgs(inst.claudeMCPConfigArgsWithToken(testMCPURL, dir, testToken))
	assert.NotContains(t, readMCPConfigHeaders(t, strings.Trim(val, "'")), MCPProofHeader)
}

func TestBuildLaunchCommand_should_KeepProofOutOfArgvAndEnv_When_ClaudeSessionLaunches(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	installMCPProofProvider(t, true)
	inst := &Instance{Program: "claude", UUID: "uuid-nonleak", EnvVars: map[string]string{"K": "v"}}
	inst.SetMCPServerURLProvider(func() string { return testMCPURL })

	for _, resume := range []string{"", "resumed-conversation"} {
		cmd := inst.buildLaunchCommand(resume)
		assert.NotContains(t, cmd, testMCPProof, "launch command resume=%q", resume)
		assert.Contains(t, cmd, "--mcp-config")
		dir, err := config.GetConfigDir()
		require.NoError(t, err)
		assert.Contains(t, cmd, filepath.Join(dir, "mcp", "uuid-nonleak.json"))
	}
	assert.NotContains(t, strings.Join(inst.buildExtraEnv(), "\n"), testMCPProof)
	flag, val := settingsArgs(inst.claudeSettingsEnvOverrideArgs())
	assert.NotContains(t, flag+val, testMCPProof)
}

func TestBuildLaunchCommand_should_DeliverOpenCodeProofByFile_When_ProofFilePresent(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	installMCPProofProvider(t, true)
	inst := &Instance{Program: "opencode", UUID: "uuid-oc"}
	inst.SetMCPServerURLProvider(func() string { return testMCPURL })

	cmd := inst.buildLaunchCommand("")

	cfgPath := filepath.Join(dir, "mcp", "uuid-oc.opencode.json")
	assert.True(t, strings.HasPrefix(cmd, "OPENCODE_CONFIG="+shellQuote(cfgPath)+" opencode"), cmd)
	assert.NotContains(t, cmd, testMCPProof)
	assert.NotContains(t, strings.Join(inst.buildExtraEnv(), "\n"), testMCPProof)
	requirePerm(t, cfgPath, 0o600)
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), testMCPProof, "the config references the proof file, never the value")
	assert.Contains(t, string(raw), "{file:"+MCPProofFilePath(dir, "uuid-oc")+"}")
	assert.Contains(t, string(raw), testMCPURL)
}

func TestBuildLaunchCommand_should_FailClosedForOpenCode_When_NoProofFileOrProvider(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	inst := &Instance{Program: "opencode", UUID: "uuid-oc2"}
	inst.SetMCPServerURLProvider(func() string { return testMCPURL })
	assert.Equal(t, "opencode", inst.buildLaunchCommand(""), "no provider")

	installMCPProofProvider(t, false) // provider mints a value but the proof file is missing
	assert.Equal(t, "opencode", inst.buildLaunchCommand(""), "no proof file")
}

// Harnesses Spike S5 did not pass never reach the provider, so no proof or
// proof-bearing config exists for them.
func TestBuildLaunchCommand_should_MintNothing_When_HarnessIsNotProofCapable(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	calls := installMCPProofProvider(t, true)

	for _, program := range []string{"pi", "gemini", "agy", "aider", "sleep 300"} {
		inst := &Instance{Program: program, UUID: "uuid-" + strings.Fields(program)[0]}
		inst.SetMCPServerURLProvider(func() string { return testMCPURL })
		cmd := inst.buildLaunchCommand("")
		assert.NotContains(t, cmd, "--mcp-config", program)
		assert.NotContains(t, cmd, opencodeConfigEnv, program)
		assert.NotContains(t, cmd, testMCPProof, program)
	}
	assert.Zero(t, calls.Load(), "provider must not be consulted for non-proof harnesses")
	entries, err := os.ReadDir(filepath.Join(dir, MCPProofsDirName))
	assert.True(t, os.IsNotExist(err) || len(entries) == 0, "no proof files for non-proof harnesses")
}

func TestRedactMCPProofs_should_ReplaceOnlyProofValues(t *testing.T) {
	t.Parallel()
	in := "header X-Stapler-Session-Proof: " + testMCPProof + " tail; sha256 " + strings.Repeat("ab", 32)
	got := RedactMCPProofs(in)
	assert.NotContains(t, got, testMCPProof)
	assert.Contains(t, got, "X-Stapler-Session-Proof: [redacted-proof] tail")
	assert.Contains(t, got, strings.Repeat("ab", 32), "plain 64-hex digests are untouched")
}

func TestValidMCPSessionUUID_should_RejectPathTraversal(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "a/b", `a\b`, "..", "x..y"} {
		assert.False(t, ValidMCPSessionUUID(bad), bad)
	}
	assert.True(t, ValidMCPSessionUUID("7c1e0a62-1f4d-4a5b-8f10-0123456789ab"))
}
