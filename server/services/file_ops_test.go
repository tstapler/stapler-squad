package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// readCorpusMTime pins mtimes so version tokens are reproducible.
var readCorpusMTime = time.Unix(1_700_000_000, 123_456_789)

// buildReadCorpus lays out the Story 0.2.3 read corpus plus the symlink fixture.
func buildReadCorpus(t *testing.T) string {
	t.Helper()
	dir, _ := newResolverFixture(t)
	files := map[string][]byte{
		"plain.txt":    []byte("a\nb\n"),
		"docs/a.md":    crlfBomDoc(),
		"latin1.txt":   []byte("caf\xE9\n"),
		"mixed.txt":    []byte("a\r\nb\nc\n"),
		"lone_cr.txt":  []byte("a\nb\rc\n"),
		"bin.dat":      {0, 1, 2, 3},
		"big.txt":      []byte(strings.Repeat("世", (2<<20)/3)),
		"huge.txt":     []byte(strings.Repeat("a", maxFileSize+1)),
		"empty.txt":    {},
		"nul_late.dat": append([]byte("text then "), 0, 'x'),
	}
	for rel, data := range files {
		writeFixtureFile(t, filepath.Join(dir, rel), data)
	}
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return err
		}
		return os.Chtimes(p, readCorpusMTime, readCorpusMTime)
	}))
	return dir
}

var readCorpusPaths = []string{
	"plain.txt", "docs/a.md", "latin1.txt", "mixed.txt", "lone_cr.txt", "bin.dat", "big.txt",
	"huge.txt", "empty.txt", "nul_late.dat", "AGENTS.md", "docs/alias.md", "shared/x.txt",
	"missing.go", "docs", "../outside", "leak", "gitlink", ".git/config",
}

// readCorpusDigest summarises GetFileContent and StatFile for every corpus path as a
// digest of the deterministically marshaled response, or "code:<connect code>".
func readCorpusDigest(t *testing.T, svc *FileService) map[string]string {
	t.Helper()
	out := map[string]string{}
	digest := func(m proto.Message) string {
		b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
		require.NoError(t, err)
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:])
	}
	for _, rel := range readCorpusPaths {
		g, err := svc.GetFileContent(context.Background(), connect.NewRequest(
			&sessionv1.GetFileContentRequest{SessionId: "test-session", Path: rel}))
		if err != nil {
			out["get:"+rel] = "code:" + connect.CodeOf(err).String()
		} else {
			out["get:"+rel] = digest(g.Msg)
		}
		st, err := svc.StatFile(context.Background(), connect.NewRequest(
			&sessionv1.StatFileRequest{SessionId: "test-session", Path: rel}))
		if err != nil {
			out["stat:"+rel] = "code:" + connect.CodeOf(err).String()
		} else {
			out["stat:"+rel] = digest(st.Msg)
		}
	}
	return out
}

const readGoldenPath = "testdata/file_ops_read_golden.json"

// Pre-refactor digests of the read corpus (captured before FileOps existed) pin
// that moving the policy into FileOps changed no response byte.
func TestGetFileContent_ShouldBeByteIdenticalThroughFileOps_And_EmitSlogAndTelemetryOnce(t *testing.T) {
	dir := buildReadCorpus(t)
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	got := readCorpusDigest(t, realFileService(dir))

	if os.Getenv("WRITE_READ_GOLDEN") != "" {
		b, err := json.MarshalIndent(got, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(readGoldenPath, append(b, '\n'), 0o644))
	}
	raw, err := os.ReadFile(readGoldenPath)
	require.NoError(t, err)
	var want map[string]string
	require.NoError(t, json.Unmarshal(raw, &want))
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	require.Equal(t, len(want), len(got))
	for _, k := range keys {
		assert.Equal(t, want[k], got[k], k)
	}
	assert.Empty(t, withoutRefusalDebugLines(logs.String()), "reads emitted no slog lines before the refactor, so FileOps must emit none either")
}

// strictOver wraps a backend and panics on any Open/Lstat path the resolver
// could not have produced: absolute, unclean, escaping or inside .git.
type strictOver struct{ inner FileBackend }

func strictBackend() FileBackend { return strictOver{osRootBackend{}} }

type strictRoot struct{ FileRoot }

func (b strictOver) OpenRoot(dir string) (FileRoot, error) {
	r, err := b.inner.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return strictRoot{r}, nil
}

func mustBeResolverOutput(p ResolvedPath) {
	s := string(p)
	if s == "" || filepath.IsAbs(s) || s != filepath.ToSlash(filepath.Clean(s)) || s == ".." ||
		strings.HasPrefix(s, "../") || hasGitComponent(s) || strings.ContainsRune(s, 0) {
		panic("backend reached with unresolved path: " + s)
	}
}

func (r strictRoot) Open(p ResolvedPath) (FileHandle, error) {
	mustBeResolverOutput(p)
	return r.FileRoot.Open(p)
}

func (r strictRoot) Lstat(p ResolvedPath) (os.FileInfo, error) {
	mustBeResolverOutput(p)
	return r.FileRoot.Lstat(p)
}

func TestFileOps_ShouldNeverReachBackendWithUnresolvedPath_When_ConfinementCorpusRuns(t *testing.T) {
	dir := buildReadCorpus(t)
	ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}}, strictBackend())
	hostile := append([]string{
		"/etc/passwd", "a/../../x", "..", "./../x", "x\x00y", `\evil`, ".GIT/config", "docs/../.git/config",
		"linkdir/x", "dir_leak/secret", "loop1", "dangling",
	}, readCorpusPaths...)
	for _, rel := range hostile {
		assert.NotPanics(t, func() {
			_, _ = ops.GetContent(context.Background(), "test-session", rel)
			_, _ = ops.Stat(context.Background(), "test-session", rel)
		}, rel)
	}
}

// withoutRefusalDebugLines drops the one deliberate server-side line for a refused symlink (the target stays off the wire).
func withoutRefusalDebugLines(logs string) string {
	var kept []string
	for _, line := range strings.Split(logs, "\n") {
		if line != "" && !strings.Contains(line, "symlink read refused") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
