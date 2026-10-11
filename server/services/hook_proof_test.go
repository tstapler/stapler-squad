package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

func newTestProofs(t *testing.T) (*HookProofs, string) {
	t.Helper()
	dir := t.TempDir()
	p, err := NewHookProofs(dir)
	require.NoError(t, err)
	return p, dir
}

func TestHookProof_ShouldVerifyOnlyItsOwnUUIDAndRejectTampering(t *testing.T) {
	p, _ := newTestProofs(t)
	v := p.HeaderValue("uuid-a")
	id, ok := p.Verify(v)
	require.True(t, ok)
	assert.Equal(t, "uuid-a", id)

	_, ok = p.Verify(strings.Replace(v, "uuid-a", "uuid-b", 1))
	assert.False(t, ok, "a proof for another session")
	for _, bad := range []string{"", "uuid-a", "uuid-a.", ".abc", "uuid-a.zz"} {
		_, ok := p.Verify(bad)
		assert.False(t, ok, bad)
	}
}

func TestHookProof_ShouldStayValidAcrossRestartAndRevokeOnRotation(t *testing.T) {
	p, dir := newTestProofs(t)
	v := p.HeaderValue("s1")
	again, err := NewHookProofs(dir)
	require.NoError(t, err)
	_, ok := again.Verify(v)
	assert.True(t, ok, "the secret persists")

	info, err := os.Stat(filepath.Join(dir, hookProofSecretFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, again.Rotate(dir))
	_, ok = again.Verify(v)
	assert.False(t, ok, "rotation revokes old proofs")
}

func TestHookProof_ShouldWriteProofFile0600In0700Dir_AndRefuseUnsafeIds(t *testing.T) {
	p, dir := newTestProofs(t)
	require.NoError(t, p.EnsureFile("sess-1"))
	require.NoError(t, p.EnsureFile("sess-1"), "idempotent")
	path := filepath.Join(dir, hookProofDirName, "sess-1")
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, hookProofHeader+": "+p.HeaderValue("sess-1")+"\n", string(b))
	fi, _ := os.Stat(path)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	di, _ := os.Stat(filepath.Join(dir, hookProofDirName))
	assert.Equal(t, os.FileMode(0o700), di.Mode().Perm())
	for _, bad := range []string{"", "../x", "a/b"} {
		assert.Error(t, p.EnsureFile(bad), bad)
	}
	p.RemoveFile("sess-1")
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

// stubCurl installs a `curl` that prints its arguments, one per line, and
// returns a runner for a hook command under a controlled environment.
func stubCurl(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "curl"), []byte(script), 0o755))
	return bin
}

func runHookCommand(t *testing.T, command, binDir string, env ...string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append([]string{"PATH=" + binDir + ":/usr/bin:/bin"}, env...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

// T-RP-73: the proof is read from a file through STAPLER_SESSION_UUID, never in
// argv, and a missing file still POSTs.
func TestBuildLocalHookCommand_ShouldAddProofHeaderFromFileAndNeverPutItInArgv(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", cfgDir)
	dir, err := config.GetConfigDir()
	require.NoError(t, err)
	setCurlHeaderFileSupport(t, true)

	p, err := NewHookProofs(dir)
	require.NoError(t, err)
	cmd := buildLocalHookCommand("http://127.0.0.1:8543/api/hooks/permission-request", "it's a title")
	assert.NotContains(t, cmd, p.HeaderValue("sess-1"), "the proof must not be in the command text")
	bin := stubCurl(t)

	t.Run("file present", func(t *testing.T) {
		require.NoError(t, p.EnsureFile("sess-1"))
		args := runHookCommand(t, cmd, bin, "STAPLER_SESSION_UUID=sess-1")
		assert.Contains(t, args, "@"+p.ProofFilePath("sess-1"))
		assert.Contains(t, args, "X-CS-Session-ID: it's a title", "title is escaped, not truncated")
		assert.Equal(t, "-d", args[len(args)-2])
		for _, a := range args {
			assert.NotContains(t, a, p.HeaderValue("sess-1"))
		}
	})
	t.Run("file missing still posts", func(t *testing.T) {
		args := runHookCommand(t, cmd, bin, "STAPLER_SESSION_UUID=no-such-session")
		assert.Equal(t, 2, countArg(args, "-H"), "only Content-Type and X-CS-Session-ID")
		assert.Contains(t, args, "-X")
		assert.Contains(t, args, "http://127.0.0.1:8543/api/hooks/permission-request")
	})
	t.Run("variable unset", func(t *testing.T) {
		args := runHookCommand(t, cmd, bin)
		assert.Equal(t, 2, countArg(args, "-H"))
		assert.Contains(t, args, "-X")
	})
	t.Run("path with a space and a quote", func(t *testing.T) {
		weird := filepath.Join(cfgDir, "we ird'dir")
		require.NoError(t, os.MkdirAll(filepath.Join(weird, hookProofDirName), 0o700))
		t.Setenv("STAPLER_SQUAD_TEST_DIR", weird)
		d2, err := config.GetConfigDir()
		require.NoError(t, err)
		p2, err := NewHookProofs(d2)
		require.NoError(t, err)
		require.NoError(t, p2.EnsureFile("sess-2"))
		args := runHookCommand(t, buildLocalHookCommand("http://x/y", "t"), bin, "STAPLER_SESSION_UUID=sess-2")
		assert.Contains(t, args, "@"+p2.ProofFilePath("sess-2"))
	})
}

// A curl older than 7.55 gets the command without the proof branch.
func TestBuildLocalHookCommand_ShouldOmitProofBranch_WhenCurlCannotReadHeaderFiles(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setCurlHeaderFileSupport(t, false)
	cmd := buildLocalHookCommand("http://x/y", "t")
	assert.NotContains(t, cmd, "STAPLER_SESSION_UUID")
	assert.True(t, strings.HasPrefix(cmd, "curl -s "))
}

// T-RP-73: an entry from before the proof branch is rewritten in place (one
// entry, no duplicate), and the rewritten entry is a fixed point.
func TestInjectHooks_ShouldReplaceAStaleEntryInPlaceAndKeepACurrentOne(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setCurlHeaderFileSupport(t, true)
	root := t.TempDir()
	url := hookApprovalURL()
	stale := "curl -s --max-time 300 -X POST '" + url + "' -H 'Content-Type: application/json' -H 'X-CS-Session-ID: s' -d @-"
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".claude"), 0o755))
	body := `{"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":` + jsonString(stale) + `,"timeout":300}]}]}}`
	require.NoError(t, os.WriteFile(filepath.Join(root, ".claude", "settings.local.json"), []byte(body), 0o644))

	require.NoError(t, InjectHookConfig(root, "s", ""))
	first, err := os.ReadFile(filepath.Join(root, ".claude", "settings.local.json"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(first), "/api/hooks/permission-request"), "replaced in place, not duplicated")
	assert.Contains(t, string(first), "STAPLER_SESSION_UUID")

	require.NoError(t, InjectHookConfig(root, "s", ""))
	require.NoError(t, InjectHooksConfig(root, "s", nil))
	second, err := os.ReadFile(filepath.Join(root, ".claude", "settings.local.json"))
	require.NoError(t, err)
	assert.JSONEq(t, string(first), string(second), "the new command is a fixed point of both producers")
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func setCurlHeaderFileSupport(t *testing.T, supported bool) {
	t.Helper()
	orig := curlHeaderFileSupported
	curlHeaderFileSupported = func() bool { return supported }
	t.Cleanup(func() { curlHeaderFileSupported = orig })
}

func countArg(args []string, want string) int {
	n := 0
	for _, a := range args {
		if a == want {
			n++
		}
	}
	return n
}
