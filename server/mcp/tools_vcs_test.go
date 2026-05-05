package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session"
)

// initGitRepo creates a temporary directory with an initialised git repository
// that has at least one commit so that git commands succeed.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	writeFile(t, dir, "README.md", "# Test\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}

// newVCSInstance builds a minimal session.Instance pointing at repoDir.
func newVCSInstance(title, repoDir string) *session.Instance {
	inst := &session.Instance{}
	inst.Title = title
	inst.Path = repoDir
	inst.Branch = "main"
	return inst
}

// TestGetSessionDiff verifies that get_session_diff returns the expected
// top-level response shape (success, truncated, stats fields) without error.
func TestGetSessionDiff(t *testing.T) {
	repoDir := initGitRepo(t)

	// Stage an untracked file as "intent to add" so the worktree has something
	// to diff (if the git version supports it). Any error here is non-fatal
	// because the test only checks response structure, not diff content.
	var added strings.Builder
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&added, "line %d\n", i)
	}
	writeFile(t, repoDir, "newfile.go", added.String())
	_ = safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "add", "-N", ".").Run()

	inst := newVCSInstance("test-session", repoDir)
	store := &stubStore{instances: []*session.Instance{inst}}
	vh := &vcsHandlers{store: store}

	req := makeToolReq(map[string]interface{}{"session_id": "test-session"})
	result, err := vh.getSessionDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed := parseResult(t, result)
	if _, ok := parsed["truncated"]; !ok {
		t.Error("response missing 'truncated' field")
	}
	if _, ok := parsed["stats"]; !ok {
		t.Error("response missing 'stats' field")
	}
	if success, _ := parsed["success"].(bool); !success {
		t.Errorf("expected success=true, got %v", parsed["success"])
	}
}

// TestGetSessionDiffTruncation verifies that the diff is capped at max_bytes.
func TestGetSessionDiffTruncation(t *testing.T) {
	repoDir := initGitRepo(t)

	// Create a large staged file so there is content to diff.
	var large strings.Builder
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&large, "this is a long line number %d with extra content to exceed byte cap\n", i)
	}
	writeFile(t, repoDir, "large.go", large.String())
	// Stage and commit so we have a base; then add more content to diff against it.
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "add large file")
	// Append more lines so there is a diff relative to HEAD~1.
	large.WriteString("extra line to create diff\n")
	writeFile(t, repoDir, "large.go", large.String())

	inst := newVCSInstance("test-session", repoDir)
	store := &stubStore{instances: []*session.Instance{inst}}
	vh := &vcsHandlers{store: store}

	req := makeToolReq(map[string]interface{}{
		"session_id": "test-session",
		"max_bytes":  float64(100),
	})
	result, _ := vh.getSessionDiff(context.Background(), req)

	parsed := parseResult(t, result)
	if _, ok := parsed["truncated"]; !ok {
		t.Error("response missing 'truncated' field")
	}
	// If a diff was returned, it must not exceed max_bytes.
	if diff, ok := parsed["diff"].(string); ok && len(diff) > 100 {
		t.Errorf("diff length %d exceeds max_bytes=100", len(diff))
	}
}

// TestGetSessionDiffSessionNotFound verifies the error response when the
// session_id does not match any known session.
func TestGetSessionDiffSessionNotFound(t *testing.T) {
	store := &stubStore{}
	vh := &vcsHandlers{store: store}

	req := makeToolReq(map[string]interface{}{"session_id": "ghost"})
	result, err := vh.getSessionDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	parsed := parseResult(t, result)
	if parsed["success"] != false {
		t.Error("expected success=false for missing session")
	}
	errField, ok := parsed["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'error' field in response")
	}
	if errField["code"] != ErrSessionNotFound {
		t.Errorf("expected code %q, got %v", ErrSessionNotFound, errField["code"])
	}
}

// TestListSessionBranchesNotFound verifies that list_session_branches returns
// success=false when the requested session does not exist (U-5.3 partial).
func TestListSessionBranchesNotFound(t *testing.T) {
	store := &stubStore{}
	vh := &vcsHandlers{store: store}

	req := makeToolReq(map[string]interface{}{"session_id": "ghost"})
	result, _ := vh.listSessionBranches(context.Background(), req)

	parsed := parseResult(t, result)
	if parsed["success"] != false {
		t.Error("expected success=false for missing session")
	}
}

// TestGetSessionDiffMissingSessionID verifies the error when session_id is omitted.
func TestGetSessionDiffMissingSessionID(t *testing.T) {
	store := &stubStore{}
	vh := &vcsHandlers{store: store}

	req := makeToolReq(map[string]interface{}{})
	result, err := vh.getSessionDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	parsed := parseResult(t, result)
	if parsed["success"] != false {
		t.Error("expected success=false when session_id is missing")
	}
	errField, ok := parsed["error"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'error' field in response")
	}
	if errField["code"] != ErrInvalidArgument {
		t.Errorf("expected code %q, got %v", ErrInvalidArgument, errField["code"])
	}
}

// TestCountDiffFiles verifies the countDiffFiles helper counts "diff --git" headers.
func TestCountDiffFiles(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\nindex abc..def 100644\n--- a/foo.go\n+++ b/foo.go\ndiff --git a/bar.go b/bar.go\nindex 123..456 100644\n"
	if n := countDiffFiles(diff); n != 2 {
		t.Errorf("countDiffFiles=%d, want 2", n)
	}
	if n := countDiffFiles(""); n != 0 {
		t.Errorf("countDiffFiles(empty)=%d, want 0", n)
	}
	single := "diff --git a/only.go b/only.go\n+some line\n"
	if n := countDiffFiles(single); n != 1 {
		t.Errorf("countDiffFiles(single)=%d, want 1", n)
	}
}

// TestCountDiffFilesNoTrailingNewline verifies countDiffFiles works on content
// without a trailing newline.
func TestCountDiffFilesNoTrailingNewline(t *testing.T) {
	diff := "diff --git a/a.go b/a.go\n+line"
	if n := countDiffFiles(diff); n != 1 {
		t.Errorf("countDiffFiles=%d, want 1", n)
	}
}
