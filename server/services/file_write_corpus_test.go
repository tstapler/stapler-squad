package services

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const corpusDir = "testdata/files_roundtrip"

// corpusRig copies the Story 2.1.4 fixtures into a fresh worktree. Fixture bytes are
// protected from line-ending conversion by testdata/files_roundtrip/.gitattributes.
func corpusRig(t *testing.T) *writeRig {
	t.Helper()
	r := newWriteRig(t)
	entries, err := os.ReadDir(corpusDir)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Name() == ".gitattributes" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		require.NoError(t, err)
		mode := os.FileMode(0o644)
		if e.Name() == "exec.sh" {
			mode = 0o755
		}
		dst := filepath.Join(r.dir, e.Name())
		require.NoError(t, os.WriteFile(dst, data, mode))
		require.NoError(t, os.Chmod(dst, mode)) // defeat the umask
	}
	return r
}

// normalizeToLf is the client's load-time normalization: fold CRLF to LF.
func normalizeToLf(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

func TestWriteFile_ShouldRoundTripByteIdentical_When_NoOpSaveOfCorpus(t *testing.T) {
	r := corpusRig(t)
	for _, name := range []string{"crlf.txt", "crlf_bom.txt", "bom.txt", "no_trailing_newline.txt", "exec.sh"} {
		t.Run(name, func(t *testing.T) {
			before := r.disk(name)
			infoBefore, err := os.Stat(filepath.Join(r.dir, name))
			require.NoError(t, err)
			read, err := r.ops.GetContent(t.Context(), writeSessionID, name)
			require.NoError(t, err)
			require.Equal(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE, read.ReadOnlyReason)

			resp, err := r.write(name, normalizeToLf(read.Content), read.VersionToken)

			require.NoError(t, err)
			require.NotNil(t, resp.GetSaved())
			after := r.disk(name)
			assert.True(t, bytes.Equal(before, after), "%s: bytes changed: %q -> %q", name, before, after)
			assert.NotContains(t, string(after), "\r\r\n")
			infoAfter, err := os.Stat(filepath.Join(r.dir, name))
			require.NoError(t, err)
			assert.Equal(t, infoBefore.Mode(), infoAfter.Mode(), "mode unchanged")
		})
	}
	assert.Empty(t, tempNames(t, r.dir))
}

func TestWriteFile_ShouldRejectMixedEolAndLatin1_And_NeverRewrite_When_CorpusReadOnlyFiles(t *testing.T) {
	r := corpusRig(t)
	cases := map[string]sessionv1.ReadOnlyReason{
		"mixed_eol.txt": sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL,
		"lone_cr.txt":   sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL,
		"latin1.txt":    sessionv1.ReadOnlyReason_READ_ONLY_REASON_NON_UTF8,
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			before := r.disk(name)
			read, err := r.ops.GetContent(t.Context(), writeSessionID, name)
			require.NoError(t, err)
			assert.Equal(t, want, read.ReadOnlyReason, "read side")

			_, err = r.write(name, "replacement\n", read.VersionToken)

			requireRefusal(t, err, connect.CodeFailedPrecondition, want)
			assert.Equal(t, before, r.disk(name), "never rewritten")
		})
	}
}

func TestWriteFile_ShouldRoundTripEmptyFile_And_WriteLfWithoutBom_When_TextSaved(t *testing.T) {
	r := corpusRig(t)
	read, err := r.ops.GetContent(t.Context(), writeSessionID, "empty.txt")
	require.NoError(t, err)
	require.Equal(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE, read.ReadOnlyReason, "a zero-byte file is editable")
	assert.Equal(t, sessionv1.LineEnding_LINE_ENDING_LF, read.LineEnding)
	assert.False(t, read.HasBom)
	tok, err := ParseFileVersionToken(read.VersionToken)
	require.NoError(t, err)
	assert.True(t, tok.Editable())
	assert.Equal(t, NewFileVersionToken(0, 0, nil).SHA256, tok.SHA256, "hash of zero bytes, not the dash form")

	infoBefore, err := os.Stat(filepath.Join(r.dir, "empty.txt"))
	require.NoError(t, err)
	resp, err := r.write("empty.txt", "", read.VersionToken)
	require.NoError(t, err)
	require.NotNil(t, resp.GetSaved())
	assert.Empty(t, r.disk("empty.txt"), "still 0 bytes")
	infoAfter, err := os.Stat(filepath.Join(r.dir, "empty.txt"))
	require.NoError(t, err)
	assert.Equal(t, infoBefore.Mode(), infoAfter.Mode())

	resp, err = r.write("empty.txt", "a\nb", resp.GetSaved().Token)
	require.NoError(t, err)
	require.NotNil(t, resp.GetSaved())
	assert.Equal(t, "a\nb", string(r.disk("empty.txt")), "LF, no BOM")

	// A CRLF file edited down to nothing saves as 0 bytes.
	resp, err = r.write("crlf.txt", "", r.token("crlf.txt"))
	require.NoError(t, err)
	require.NotNil(t, resp.GetSaved())
	assert.Empty(t, r.disk("crlf.txt"))
}

func TestWriteFile_ShouldRejectAndLeaveTargetUnchanged_When_SymlinkEscapeMatrix(t *testing.T) {
	r := newWriteRig(t) // directory-mode checkout: ActiveDir == ExistingDir == RepoRoot
	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("OUTSIDE"), 0o644))
	outsideDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "x"), []byte("OUTSIDE-DIR"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(r.dir, ".git", "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(r.dir, ".git", "config"), []byte("[core]\n"), 0o644))
	for link, target := range map[string]string{
		"leak": outsideFile, "dir_leak": outsideDir, "linkdir": ".git/hooks", "gitlink": ".git",
	} {
		require.NoError(t, os.Symlink(target, filepath.Join(r.dir, link)))
	}
	goodToken := "v1:1:1:" + strings.Repeat("0", sha256HexLen)
	gitBefore := treeDigest(t, filepath.Join(r.dir, ".git"))
	outFileBefore, outDirBefore := treeDigest(t, filepath.Dir(outsideFile)), treeDigest(t, outsideDir)

	for _, rel := range []string{"leak", "dir_leak/x", "linkdir/pre-commit", "gitlink/config"} {
		t.Run(rel, func(t *testing.T) {
			_, err := r.write(rel, "PWNED\n", goodToken)
			require.Error(t, err)
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		})
	}
	assert.Equal(t, gitBefore, treeDigest(t, filepath.Join(r.dir, ".git")), ".git unchanged")
	assert.Equal(t, outFileBefore, treeDigest(t, filepath.Dir(outsideFile)), "outside file unchanged")
	assert.Equal(t, outDirBefore, treeDigest(t, outsideDir), "outside dir unchanged")
}

func TestWriteFile_ShouldYieldOneSavedAnd19Conflicts_When_20ConcurrentSavesSameToken(t *testing.T) {
	const writers = 20
	r := corpusRig(t)
	tok := r.token("crlf.txt")
	var saved, conflicts, other int
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			resp, err := r.write("crlf.txt", fmt.Sprintf("writer %d\n", i), tok)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				other++
			case resp.GetSaved() != nil:
				saved++
			case resp.GetConflict() != nil:
				conflicts++
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, saved)
	assert.Equal(t, writers-1, conflicts)
	assert.Zero(t, other)
	got := string(r.disk("crlf.txt"))
	assert.Regexp(t, `^writer \d+\r\n$`, got, "one writer's content, CRLF re-applied, never interleaved")
	assert.Empty(t, tempNames(t, r.dir))
	assert.Zero(t, pathLocks.size(), "idle registry entries are released")
}
