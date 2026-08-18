package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// makeDir creates a directory at the given path, failing the test on error.
func makeDir(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0755))
}

// makeFile creates an empty file at the given path, failing the test on error.
func makeFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

// newReq wraps a proto request message in a ConnectRPC Request.
func newReq[T any](msg *T) *connect.Request[T] {
	return connect.NewRequest(msg)
}

func TestListPathCompletions_BasicListing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "alpha"))
	makeDir(t, filepath.Join(dir, "beta"))
	makeFile(t, filepath.Join(dir, "file.txt"))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.True(t, resp.Msg.BaseDirExists)
	assert.False(t, resp.Msg.Truncated)

	names := make([]string, len(resp.Msg.Entries))
	for i, e := range resp.Msg.Entries {
		names[i] = e.Name
	}
	assert.Contains(t, names, "alpha")
	assert.Contains(t, names, "beta")
	assert.Contains(t, names, "file.txt")
}

func TestListPathCompletions_FilterByPrefix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "alpha"))
	makeDir(t, filepath.Join(dir, "apple"))
	makeDir(t, filepath.Join(dir, "beta"))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: filepath.Join(dir, "al"),
	}))
	require.NoError(t, err)

	names := make([]string, len(resp.Msg.Entries))
	for i, e := range resp.Msg.Entries {
		names[i] = e.Name
	}
	assert.Equal(t, []string{"alpha"}, names)
}

func TestListPathCompletions_DirectoriesOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "subdir"))
	makeFile(t, filepath.Join(dir, "afile.txt"))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix:      dir + "/",
		DirectoriesOnly: true,
	}))
	require.NoError(t, err)

	for _, e := range resp.Msg.Entries {
		assert.True(t, e.IsDirectory, "expected only directories, got file: %s", e.Name)
	}
	assert.Len(t, resp.Msg.Entries, 1)
	assert.Equal(t, "subdir", resp.Msg.Entries[0].Name)
}

func TestListPathCompletions_HiddenFilesHidden(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, ".hidden"))
	makeDir(t, filepath.Join(dir, "visible"))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)

	for _, e := range resp.Msg.Entries {
		assert.False(t, e.Name[0] == '.', "hidden entry %q should not appear", e.Name)
	}
}

func TestListPathCompletions_HiddenFilesShownWhenPrefixStartsWithDot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, ".hidden"))
	makeDir(t, filepath.Join(dir, "visible"))

	svc := NewPathCompletionService()
	// Use dir + "/." (not filepath.Join which would clean away the dot)
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/.",
	}))
	require.NoError(t, err)

	names := make([]string, len(resp.Msg.Entries))
	for i, e := range resp.Msg.Entries {
		names[i] = e.Name
	}
	assert.Contains(t, names, ".hidden")
	assert.NotContains(t, names, "visible") // "visible" doesn't start with "."
}

func TestListPathCompletions_PathExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existingSubDir := filepath.Join(dir, "existing")
	makeDir(t, existingSubDir)

	svc := NewPathCompletionService()

	// Exact path that exists.
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: existingSubDir,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.PathExists, "pathExists should be true for existing dir")

	// Partial path that does NOT exist as a directory.
	resp, err = svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: filepath.Join(dir, "exis"),
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.PathExists, "pathExists should be false for partial path")
}

func TestListPathCompletions_NonexistentBaseDir(t *testing.T) {
	t.Parallel()
	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: "/nonexistent/path/that/does/not/exist/prefix",
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.BaseDirExists)
	assert.Empty(t, resp.Msg.Entries)
}

func TestListPathCompletions_Truncation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		makeDir(t, filepath.Join(dir, string(rune('a'+i))+"dir"))
	}

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
		MaxResults: 5,
	}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Entries, 5)
	assert.True(t, resp.Msg.Truncated)
}

func TestListPathCompletions_NoTruncationUnderLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "adir"))
	makeDir(t, filepath.Join(dir, "bdir"))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
		MaxResults: 10,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Truncated)
}

func TestListPathCompletions_Symlink_ToDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "realdir")
	link := filepath.Join(dir, "linkdir")
	makeDir(t, target)
	require.NoError(t, os.Symlink(target, link))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)

	var linkEntry *sessionv1.PathEntry
	for _, e := range resp.Msg.Entries {
		if e.Name == "linkdir" {
			linkEntry = e
			break
		}
	}
	require.NotNil(t, linkEntry, "symlink entry should appear in results")
	assert.True(t, linkEntry.IsDirectory, "symlink to directory should report IsDirectory=true")
}

func TestListPathCompletions_BrokenSymlink_Skipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	link := filepath.Join(dir, "brokenlink")
	require.NoError(t, os.Symlink("/nonexistent/target", link))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)

	for _, e := range resp.Msg.Entries {
		assert.NotEqual(t, "brokenlink", e.Name, "broken symlink should be skipped")
	}
}

func TestListPathCompletions_TildeExpansion(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: "~/",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.BaseDirExists)
	assert.Equal(t, home, resp.Msg.BaseDir)
}

func TestListPathCompletions_TildeAlone(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: "~",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.BaseDirExists)
	assert.Equal(t, home, resp.Msg.BaseDir)
}

func TestListPathCompletions_MaxResultsHardCap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		makeDir(t, filepath.Join(dir, string(rune('a'+i))+"dir"))
	}

	svc := NewPathCompletionService()
	// Request more than the hard cap – should be silently clamped to 500.
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
		MaxResults: 99999,
	}))
	require.NoError(t, err)
	// Only 10 dirs exist so truncated=false; the cap just doesn't cause an error.
	assert.False(t, resp.Msg.Truncated)
	assert.Len(t, resp.Msg.Entries, 10)
}

func TestListPathCompletions_DefaultMaxResults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Create fewer than the default maximum (100) to verify no truncation.
	for i := 0; i < 5; i++ {
		makeDir(t, filepath.Join(dir, string(rune('a'+i))+"dir"))
	}

	svc := NewPathCompletionService()
	// MaxResults=0 should default to 100.
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
		MaxResults: 0,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Truncated)
	assert.Len(t, resp.Msg.Entries, 5)
}

func TestListPathCompletions_Symlink_ToFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "realfile.txt")
	link := filepath.Join(dir, "linkfile")
	makeFile(t, target)
	require.NoError(t, os.Symlink(target, link))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)

	var linkEntry *sessionv1.PathEntry
	for _, e := range resp.Msg.Entries {
		if e.Name == "linkfile" {
			linkEntry = e
			break
		}
	}
	require.NotNil(t, linkEntry, "symlink to file should appear in results")
	assert.False(t, linkEntry.IsDirectory, "symlink to file should report IsDirectory=false")
}

func TestListPathCompletions_Symlink_ToFile_ExcludedByDirOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "realfile.txt")
	link := filepath.Join(dir, "linkfile")
	makeFile(t, target)
	require.NoError(t, os.Symlink(target, link))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix:      dir + "/",
		DirectoriesOnly: true,
	}))
	require.NoError(t, err)

	for _, e := range resp.Msg.Entries {
		assert.NotEqual(t, "linkfile", e.Name, "file symlink should be excluded when directories_only=true")
	}
}

func TestListPathCompletions_PathExists_FileIsNotDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fileInDir := filepath.Join(dir, "somefile.txt")
	makeFile(t, fileInDir)

	svc := NewPathCompletionService()
	// A file at the exact path is NOT a directory → pathExists should be false.
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: fileInDir,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.PathExists, "pathExists should be false for a file, not a directory")
}

func TestListPathCompletions_EntryPathCorrectness(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "mydir"))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Entries, 1)

	entry := resp.Msg.Entries[0]
	assert.Equal(t, "mydir", entry.Name)
	// Path must be absolute and join baseDir with the entry name.
	assert.Equal(t, filepath.Join(dir, "mydir"), entry.Path)
}

func TestListPathCompletions_BaseDirInResponse(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "sub"))

	svc := NewPathCompletionService()
	resp, err := svc.ListPathCompletions(context.Background(), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)
	assert.Equal(t, dir, resp.Msg.BaseDir)
	assert.True(t, resp.Msg.BaseDirExists)
}

func TestListPathCompletions_ContextCancellation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "sub"))

	// Cancel the context before calling.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled.

	svc := NewPathCompletionService()
	_, err := svc.ListPathCompletions(ctx, newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	// With a pre-cancelled context the goroutine races with the done channel.
	// The result may be either an error (CodeDeadlineExceeded) or a valid response
	// depending on scheduling, but it must not panic.
	_ = err
}

// ---------------------------------------------------------------------------
// ListWorktrees (AC4)
// ---------------------------------------------------------------------------

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
}

func TestListWorktrees_BasicListing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "commit", "--allow-empty", "-q", "-m", "init")

	svc := NewPathCompletionService()
	resp, err := svc.ListWorktrees(context.Background(), newReq(&sessionv1.ListWorktreesRequest{
		RepoPath: dir,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Worktrees, 1)
	assert.True(t, resp.Msg.Worktrees[0].IsMain)
}

func TestListWorktrees_NotAGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	svc := NewPathCompletionService()
	resp, err := svc.ListWorktrees(context.Background(), newReq(&sessionv1.ListWorktreesRequest{
		RepoPath: dir,
	}))
	// Not a git repo: gracefully returns an empty list, not an error.
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Worktrees)
}

// TestListWorktrees_TimesOutOnHungGitCommand (AC4): a hung/blocking `git worktree
// list` subprocess must not block the RPC forever — it must return a bounded
// DeadlineExceeded error, matching ListPathCompletions' pathCompletionTimeout guard.
func TestListWorktrees_TimesOutOnHungGitCommand(t *testing.T) {
	// Install a fake "git" that hangs well past pathCompletionTimeout, ahead of
	// the real git on PATH. Prepend (not replace) PATH so the script's own
	// "sleep" invocation can still resolve.
	binDir := t.TempDir()
	fakeGit := filepath.Join(binDir, "git")
	require.NoError(t, os.WriteFile(fakeGit, []byte("#!/bin/sh\nsleep 30\n"), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	svc := NewPathCompletionService()

	start := time.Now()
	_, err := svc.ListWorktrees(context.Background(), newReq(&sessionv1.ListWorktreesRequest{
		RepoPath: t.TempDir(),
	}))
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Equal(t, connect.CodeDeadlineExceeded, connect.CodeOf(err))
	assert.Less(t, elapsed, 10*time.Second, "ListWorktrees should time out near pathCompletionTimeout, not hang")
}

// Unit tests for helper functions.

func TestExpandTilde(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	cases := []struct {
		input    string
		expected string
	}{
		{"~", home + "/"},
		{"~/", home + "/"},
		{"~/projects", home + "/projects"},
		{"~/a/b/c/", home + "/a/b/c/"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tc := range cases {
		got, err := expandTilde(tc.input)
		require.NoError(t, err, "input: %q", tc.input)
		assert.Equal(t, tc.expected, got, "input: %q", tc.input)
	}
}

func TestSplitPathPrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input      string
		wantBase   string
		wantFilter string
	}{
		{"/home/user/proj", "/home/user", "proj"},
		{"/home/user/", "/home/user", ""},
		{"/", "/", ""},
		{"/foo", "/", "foo"},
		{"proj", ".", "proj"},
		{"", ".", ""},
	}

	for _, tc := range cases {
		base, filter := splitPathPrefix(tc.input)
		assert.Equal(t, tc.wantBase, base, "input: %q base", tc.input)
		assert.Equal(t, tc.wantFilter, filter, "input: %q filter", tc.input)
	}
}
