package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// newResolverFixture builds a worktree with the symlink classes found in the
// owner's live worktrees (ADR-031 "Spike outcomes", Task 0.2.1d).
func newResolverFixture(t *testing.T) (dir, outside string) {
	t.Helper()
	outside = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("TOPSECRET"), 0o644))
	dir = t.TempDir()
	mustMkdir(t, filepath.Join(dir, "src"))
	mustMkdir(t, filepath.Join(dir, "docs"))
	mustMkdir(t, filepath.Join(dir, "pkg", "shared"))
	mustMkdir(t, filepath.Join(dir, ".git", "hooks"))
	writeFixtureFile(t, filepath.Join(dir, "src", "a.go"), []byte("package a\n"))
	writeFixtureFile(t, filepath.Join(dir, "CLAUDE.md"), []byte("claude\n"))
	writeFixtureFile(t, filepath.Join(dir, "README.md"), []byte("readme\n"))
	writeFixtureFile(t, filepath.Join(dir, "pkg", "shared", "x.txt"), []byte("shared\n"))
	writeFixtureFile(t, filepath.Join(dir, ".git", "config"), []byte("[core]\n"))
	links := map[string]string{
		"AGENTS.md":       "CLAUDE.md",                      // in-tree file link (257/335 in the live inventory)
		"docs/alias.md":   "../README.md",                   // in-tree file link via ..
		"shared":          "pkg/shared",                     // in-tree directory link
		"chain1":          "chain2",                         // chained in-tree links
		"chain2":          "CLAUDE.md",                      //
		"leak":            filepath.Join(outside, "secret"), // absolute target outside (AGENTS.md -> ~/.claude/CLAUDE.md class)
		"leak_rel":        "../" + filepath.Base(outside) + "/secret",
		"dir_leak":        outside,                  // directory link out of the root
		"dangling":        "../shared/slidev-theme", // dangling relative link above the root
		"dangling_inside": "nope.txt",               // dangling link that stays inside
		"gitlink":         ".git",
		"linkdir":         ".git/hooks",
		"loop1":           "loop2",
		"loop2":           "loop1",
	}
	for name, target := range links {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, name)))
	}
	return dir, outside
}

func openFixtureRoot(t *testing.T, dir string) *WorktreeRoot {
	t.Helper()
	root, err := OpenWorktreeRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestResolveInWorktree_ShouldReturnResolvedPath_When_PlainRelativePath(t *testing.T) {
	dir, _ := newResolverFixture(t)
	root := openFixtureRoot(t, dir)
	got, err := resolveInWorktree(root, "src/a.go", AccessRead)
	require.NoError(t, err)
	assert.Equal(t, ResolvedPath("src/a.go"), got)

	got, err = resolveInWorktree(root, "", AccessRead)
	require.NoError(t, err)
	assert.Equal(t, ResolvedPath("."), got, "empty path names the root")

	got, err = resolveInWorktree(root, "src/new.go", AccessWrite)
	require.NoError(t, err)
	assert.Equal(t, ResolvedPath("src/new.go"), got, "a missing leaf is allowed so a write can create it")
}

func TestResolveInWorktree_ShouldReject_When_DotGitDotDotAbsoluteNulOrSymlinkToGit(t *testing.T) {
	dir, _ := newResolverFixture(t)
	root := openFixtureRoot(t, dir)
	cases := []struct {
		name string
		path string
		want connect.Code
	}{
		{"dot git hook", ".git/hooks/pre-commit", connect.CodePermissionDenied},
		{"dot git itself", ".git", connect.CodePermissionDenied},
		{"nested dot git", "src/.git/config", connect.CodePermissionDenied},
		{"dot git, mixed case", ".GIT/config", connect.CodePermissionDenied},
		{"dot dot escape", "a/../../x", connect.CodeInvalidArgument},
		{"bare dot dot", "..", connect.CodeInvalidArgument},
		{"absolute", "/etc/passwd", connect.CodeInvalidArgument},
		{"nul", "a\x00b", connect.CodeInvalidArgument},
		{"link into dot git dir", "linkdir/pre-commit", connect.CodePermissionDenied},
		{"link to dot git", "gitlink", connect.CodePermissionDenied},
		{"link to dot git, child", "gitlink/config", connect.CodePermissionDenied},
		{"escaping absolute link", "leak", connect.CodePermissionDenied},
		{"escaping relative link", "leak_rel", connect.CodePermissionDenied},
		{"escaping dir link child", "dir_leak/secret", connect.CodePermissionDenied},
		{"dangling link above root", "dangling", connect.CodePermissionDenied},
		{"link loop", "loop1", connect.CodeInvalidArgument},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := resolveInWorktree(root, c.path, AccessRead)
			require.Error(t, err)
			assert.Equal(t, c.want, connect.CodeOf(err))
		})
	}
}

func TestResolveInWorktree_ShouldCarryLinkTargetAndNoBytes_When_SymlinkDenied(t *testing.T) {
	dir, outside := newResolverFixture(t)
	root := openFixtureRoot(t, dir)
	_, err := resolveInWorktree(root, "leak", AccessRead)
	var denied *SymlinkDeniedError
	require.True(t, errors.As(err, &denied))
	assert.Equal(t, "leak", denied.Link)
	assert.Equal(t, filepath.Join(outside, "secret"), denied.Target)
}

func TestResolveInWorktree_ShouldFollowInTreeLinks_When_ReadMode(t *testing.T) {
	dir, _ := newResolverFixture(t)
	root := openFixtureRoot(t, dir)
	cases := map[string]ResolvedPath{
		"AGENTS.md":       "CLAUDE.md",
		"docs/alias.md":   "README.md",
		"shared":          "pkg/shared",
		"shared/x.txt":    "pkg/shared/x.txt",
		"chain1":          "CLAUDE.md",
		"dangling_inside": "nope.txt",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			got, viaLink, err := resolveInWorktreeLinks(root, in, AccessRead)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			assert.True(t, viaLink)
		})
	}
	_, viaLink, err := resolveInWorktreeLinks(root, "src/a.go", AccessRead)
	require.NoError(t, err)
	assert.False(t, viaLink)
}

func TestResolveInWorktree_ShouldRefuseAnySymlink_When_WriteMode(t *testing.T) {
	dir, _ := newResolverFixture(t)
	root := openFixtureRoot(t, dir)
	for _, p := range []string{"AGENTS.md", "shared/x.txt", "shared", "linkdir/pre-commit", "leak"} {
		t.Run(p, func(t *testing.T) {
			_, err := resolveInWorktree(root, p, AccessWrite)
			require.Error(t, err)
			assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
		})
	}
}

type stubWorkspaceProvider struct{ ws session.Workspace }

func (s stubWorkspaceProvider) GetWorkspace(string) (session.Workspace, error) { return s.ws, nil }

func TestResolveFileRequest_ShouldUseExistingDirForRead_And_RequireActiveEqualsExistingForWrite(t *testing.T) {
	dir, _ := newResolverFixture(t)
	ctx := context.Background()
	ref := FileRef{SessionID: "s1", Path: "src/a.go"}

	read, err := resolveFileRequest(ctx, stubWorkspaceProvider{session.Workspace{ExistingDir: dir}}, ref, AccessRead)
	require.NoError(t, err)
	t.Cleanup(func() { _ = read.Root.Close() })
	assert.Equal(t, ResolvedPath("src/a.go"), read.Resolved)

	// Directory mode: no WorktreeDir, ActiveDir == ExistingDir == RepoRoot is editable.
	write, err := resolveFileRequest(ctx, stubWorkspaceProvider{session.Workspace{RepoRoot: dir, ActiveDir: dir, ExistingDir: dir}}, ref, AccessWrite)
	require.NoError(t, err)
	_ = write.Root.Close()

	// Paused worktree: ExistingDir fell back to the repo root.
	_, err = resolveFileRequest(ctx, stubWorkspaceProvider{session.Workspace{RepoRoot: dir, ActiveDir: "/gone/worktree", ExistingDir: dir}}, ref, AccessWrite)
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "WorktreeMissing")

	_, err = resolveFileRequest(ctx, stubWorkspaceProvider{session.Workspace{ExistingDir: dir}}, FileRef{}, AccessRead)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = resolveFileRequest(ctx, stubWorkspaceProvider{}, ref, AccessRead)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}
