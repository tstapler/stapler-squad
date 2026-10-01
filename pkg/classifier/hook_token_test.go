package classifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadOrCreateHookToken_CreatesOnce_With0600(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	first, err := LoadOrCreateHookToken(dir)
	require.NoError(t, err)
	assert.Len(t, first, hookTokenBytes)

	fi, err := os.Stat(filepath.Join(dir, HookTokenFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())

	second, err := LoadOrCreateHookToken(dir)
	require.NoError(t, err)
	assert.Equal(t, first, second, "restart must keep the same token")
}

func TestReadHookToken_Rejects(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadHookToken(dir)
	require.Error(t, err, "missing file")

	path := filepath.Join(dir, HookTokenFile)
	require.NoError(t, os.WriteFile(path, []byte("not-hex"), 0o600))
	_, err = ReadHookToken(dir)
	require.Error(t, err, "malformed")

	tok, err := LoadOrCreateHookToken(t.TempDir())
	require.NoError(t, err)
	d2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(d2, HookTokenFile), []byte(hexOf(tok)), 0o644)) //nolint:gosec // deliberately too open
	require.NoError(t, os.Chmod(filepath.Join(d2, HookTokenFile), 0o644))
	_, err = ReadHookToken(d2)
	require.Error(t, err, "group/other-readable token authenticates nothing")
}

func hexOf(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0xf])
	}
	return string(out)
}

func TestRequestSignature_BindsBodyAndToken(t *testing.T) {
	tok := []byte("0123456789abcdef0123456789abcdef")
	body := []byte(`{"a":1}`)
	sig := SignRequestBody(tok, body)
	assert.True(t, VerifyRequestBody(tok, body, sig))
	assert.False(t, VerifyRequestBody(tok, []byte(`{"a":2}`), sig), "different body")
	assert.False(t, VerifyRequestBody([]byte("ffffffffffffffffffffffffffffffff"), body, sig), "different token")
	assert.False(t, VerifyRequestBody(tok, body, ""), "missing signature")
}

func TestResponseProof_BindsNonceAndDecision(t *testing.T) {
	tok := []byte("0123456789abcdef0123456789abcdef")
	resp := RemoteClassifyResponse{Version: 1, ConfigDir: "/c", Result: ClassificationResult{Decision: AutoDeny, RuleID: "r"}}
	resp.Proof = ResponseProof(tok, "nonce-1", resp)
	assert.True(t, VerifyResponseProof(tok, "nonce-1", resp))
	assert.False(t, VerifyResponseProof(tok, "nonce-2", resp), "replay under another nonce")

	flipped := resp
	flipped.Result.Decision = AutoAllow
	assert.False(t, VerifyResponseProof(tok, "nonce-1", flipped), "decision tampered")

	forged := RemoteClassifyResponse{Version: 1, ConfigDir: "/c"} // zero Decision == AutoAllow, no proof
	assert.False(t, VerifyResponseProof(tok, "nonce-1", forged))
}
