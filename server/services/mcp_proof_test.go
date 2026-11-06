package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func newTestMcpProofs(t *testing.T) (*McpProofs, string) {
	t.Helper()
	dir := t.TempDir()
	keys, err := NewHookProofs(dir)
	require.NoError(t, err)
	return NewMcpProofs(keys, dir), dir
}

func TestMcpProofs_Verify_should_AcceptOnlyOwnSessionProof(t *testing.T) {
	t.Parallel()
	m, _ := newTestMcpProofs(t)
	a, b := "session-a", "session-b"

	assert.True(t, m.Verify(a, m.HeaderValue(a)))
	assert.False(t, m.Verify(b, m.HeaderValue(a)), "session A's proof with B's uuid")
	assert.False(t, m.Verify(a, ""))
	assert.False(t, m.Verify(a, "garbage"))
	assert.False(t, m.Verify("", m.HeaderValue("")))
	assert.False(t, m.Verify("../etc", m.HeaderValue("../etc")))
	assert.True(t, strings.HasPrefix(m.HeaderValue(a), session.MCPProofValuePrefix))
}

func TestMcpProofs_HeaderValue_should_NotBeAHookProof(t *testing.T) {
	t.Parallel()
	m, _ := newTestMcpProofs(t)
	hook := m.keys.HeaderValue("s1") // "<uuid>.<hook proof>"
	assert.False(t, m.Verify("s1", hook))
	assert.NotContains(t, m.HeaderValue("s1"), m.keys.mac("s1"), "domain separation: different MAC")
}

func TestMcpProofs_Verify_should_RejectOldProofs_When_SecretRotated(t *testing.T) {
	t.Parallel()
	m, dir := newTestMcpProofs(t)
	old := m.HeaderValue("s1")
	require.NoError(t, m.keys.Rotate(dir))
	assert.False(t, m.Verify("s1", old))
	assert.True(t, m.Verify("s1", m.HeaderValue("s1")))
}

func TestMcpProofs_EnsureFile_should_WritePrivateFileWithBareValue(t *testing.T) {
	t.Parallel()
	m, dir := newTestMcpProofs(t)
	require.NoError(t, m.EnsureFile("s1"))
	require.NoError(t, m.EnsureFile("s1"), "idempotent")

	path := filepath.Join(dir, "mcp-proofs", "s1")
	assert.Equal(t, path, m.ProofFilePath("s1"))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, m.HeaderValue("s1"), string(got), "no trailing newline: OpenCode substitutes the file verbatim")

	assert.Error(t, m.EnsureFile("../escape"))
}

func TestMcpProofs_Provider_should_MintAndWriteFile(t *testing.T) {
	t.Parallel()
	m, _ := newTestMcpProofs(t)
	provide := m.Provider()

	value, ok := provide("s1")
	require.True(t, ok)
	assert.True(t, m.Verify("s1", value))
	onDisk, err := os.ReadFile(m.ProofFilePath("s1"))
	require.NoError(t, err)
	assert.Equal(t, value, string(onDisk))

	_, ok = provide("../bad")
	assert.False(t, ok)
}

func TestMcpProofs_RemoveFiles_should_DeleteEveryFileOfTheSession(t *testing.T) {
	t.Parallel()
	m, dir := newTestMcpProofs(t)
	require.NoError(t, m.EnsureFile("s1"))
	require.NoError(t, m.EnsureFile("s2"))
	for _, name := range []string{"s1.json", "s1.opencode.json", "s2.json"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "mcp"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "mcp", name), []byte("{}"), 0o600))
	}

	m.RemoveFiles("s1")
	m.RemoveFiles("s1") // idempotent
	m.RemoveFiles("../s2")

	for _, gone := range []string{"mcp-proofs/s1", "mcp/s1.json", "mcp/s1.opencode.json"} {
		assert.NoFileExists(t, filepath.Join(dir, gone))
	}
	assert.FileExists(t, filepath.Join(dir, "mcp-proofs", "s2"))
	assert.FileExists(t, filepath.Join(dir, "mcp", "s2.json"))
}

func TestMcpProofs_SweepStale_should_RemoveFilesOfUnknownSessions(t *testing.T) {
	t.Parallel()
	m, dir := newTestMcpProofs(t)
	require.NoError(t, m.EnsureFile("live"))
	require.NoError(t, m.EnsureFile("dead"))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "mcp"), 0o700))
	for _, name := range []string{"live.json", "dead.json", "dead.opencode.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "mcp", name), []byte("{}"), 0o600))
	}

	removed := m.SweepStale(func(uuid string) bool { return uuid == "live" })

	assert.Equal(t, 3, removed)
	assert.NoFileExists(t, filepath.Join(dir, "mcp-proofs", "dead"))
	assert.NoFileExists(t, filepath.Join(dir, "mcp", "dead.json"))
	assert.FileExists(t, filepath.Join(dir, "mcp-proofs", "live"))
	assert.FileExists(t, filepath.Join(dir, "mcp", "live.json"))
}

func TestMcpProofs_SelfCheck_should_PassAndLeaveNoFiles_When_DirWritable(t *testing.T) {
	t.Parallel()
	m, dir := newTestMcpProofs(t)
	require.NoError(t, m.SelfCheck())
	entries, err := os.ReadDir(filepath.Join(dir, "mcp-proofs"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestMcpProofs_SelfCheck_should_Fail_When_ProofDirBlocked(t *testing.T) {
	t.Parallel()
	m, dir := newTestMcpProofs(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mcp-proofs"), []byte("x"), 0o600))
	assert.Error(t, m.SelfCheck())
}
