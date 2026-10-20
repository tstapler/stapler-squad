package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

func TestProxyHeaders_should_ReadProofFromSessionFile_When_Present(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	keys, err := services.NewHookProofs(dir)
	require.NoError(t, err)
	proofs := services.NewMcpProofs(keys, dir)
	t.Setenv("STAPLER_SESSION_UUID", "S")
	require.NoError(t, proofs.EnsureFile("S"))

	headers := ProxyHeaders()

	assert.Equal(t, "S", headers["X-Stapler-Session-UUID"])
	assert.True(t, proofs.Verify("S", headers[session.MCPProofHeader]), "proof verifies for S")
	assert.False(t, proofs.Verify("T", headers[session.MCPProofHeader]), "and not for another session")

	require.NoError(t, os.Remove(proofs.ProofFilePath("S")))
	assert.NotContains(t, ProxyHeaders(), session.MCPProofHeader, "file removed, no proof header")
}

func TestProxyHeaders_should_OmitProof_When_FileMalformedOrNoSession(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, session.MCPProofsDirName), 0o700))
	require.NoError(t, os.WriteFile(session.MCPProofFilePath(dir, "S"), []byte("not-a-proof\n"), 0o600))

	t.Setenv("STAPLER_SESSION_UUID", "S")
	assert.NotContains(t, ProxyHeaders(), session.MCPProofHeader, "malformed content")

	t.Setenv("STAPLER_SESSION_UUID", "")
	assert.NotContains(t, ProxyHeaders(), session.MCPProofHeader, "no session uuid")

	t.Setenv("STAPLER_SESSION_UUID", "../S")
	assert.NotContains(t, ProxyHeaders(), session.MCPProofHeader, "path traversal")
}

func TestProxyHeaders_should_NotUseAnEnvironmentVariableForTheProof(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	t.Setenv("STAPLER_SESSION_UUID", "S")
	t.Setenv("STAPLER_SESSION_PROOF", session.MCPProofValuePrefix+"deadbeef")
	assert.NotContains(t, ProxyHeaders(), session.MCPProofHeader)
}
