package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git"
)

// setupTestGitRepo creates a temporary git repository with an initial commit and a
// configured test identity, mirroring session/git/worktree_creation_test.go's
// setupTestRepo helper (duplicated here rather than imported since that one lives in
// package git and is unexported). Uses go-git directly rather than shelling out — see
// the `prefer-go-git-over-subshells` skill.
func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{
		InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree failed: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if _, err := wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	return dir
}

// setupLinkedWorktree builds a real linked worktree (git worktree add topology)
// on top of an existing main repo — the topology where --git-dir and
// --git-common-dir resolve to different directories, which a plain setupTestGitRepo
// fixture cannot exercise.
func setupLinkedWorktree(t *testing.T, repoPath, sessionName string) string {
	t.Helper()
	worktree, _, err := git.NewGitWorktree(repoPath, sessionName)
	if err != nil {
		t.Fatalf("git.NewGitWorktree failed: %v", err)
	}
	if err := worktree.Setup(); err != nil {
		t.Fatalf("worktree.Setup failed: %v", err)
	}
	t.Cleanup(func() {
		if err := worktree.Cleanup(); err != nil {
			t.Logf("worktree.Cleanup failed (non-fatal): %v", err)
		}
	})
	return worktree.GetWorktreePath()
}

// gitLsFiles returns the list of paths currently tracked in the git index at dir.
func gitLsFiles(t *testing.T, dir string) []string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", "ls-files")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed: %v", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// containsPath reports whether paths contains target.
func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}

// makeTestBacklogItemWithID creates a *BacklogItemData with a specific string ID.
func makeTestBacklogItemWithID(id, title, acJSON string) *BacklogItemData {
	return &BacklogItemData{
		ID:                 id,
		Title:              title,
		Description:        "Test description",
		AcceptanceCriteria: AcCriteriaJSON(acJSON),
		Status:             "ready",
		Priority:           1,
	}
}

// TestWriteSlashCommands_CreatesCorrectFileCount verifies that 2 AC criteria produce
// status.md + done-0.md + fail-0.md + done-1.md + fail-1.md + review.md + ship.md +
// help.md + block.md + duplicate.md = 10 files.
func TestWriteSlashCommands_CreatesCorrectFileCount(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	ac := `[{"index":0,"text":"First criterion","status":"pending"},{"index":1,"text":"Second criterion","status":"pending"}]`
	item := makeTestBacklogItemWithID("test-item-id-1", "My Feature", ac)

	if err := WriteSlashCommands(nil, item, worktree); err != nil {
		t.Fatalf("WriteSlashCommands returned error: %v", err)
	}

	cmdDir := filepath.Join(worktree, backlogCommandsDir)
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		t.Fatalf("failed to read command dir: %v", err)
	}

	wantFiles := []string{
		"status.md",
		"done-0.md",
		"fail-0.md",
		"done-1.md",
		"fail-1.md",
		"review.md",
		"ship.md",
		"help.md",
		"block.md",
		"duplicate.md",
	}
	if len(entries) != len(wantFiles) {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected %d files, got %d: %v", len(wantFiles), len(entries), names)
	}

	for _, want := range wantFiles {
		path := filepath.Join(cmdDir, want)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", want)
		}
	}
}

// TestWriteSlashCommands_DoneFileContainsItemUUID verifies done-0.md contains the item UUID.
func TestWriteSlashCommands_DoneFileContainsItemUUID(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	itemID := "550e8400-e29b-41d4-a716-446655440000"
	ac := `[{"index":0,"text":"Do something","status":"pending"},{"index":1,"text":"Do more","status":"pending"}]`
	item := makeTestBacklogItemWithID(itemID, "Feature", ac)

	if err := WriteSlashCommands(nil, item, worktree); err != nil {
		t.Fatalf("WriteSlashCommands returned error: %v", err)
	}

	// done-0.md should reference the item UUID
	donePath := filepath.Join(worktree, backlogCommandsDir, "done-0.md")
	data, err := os.ReadFile(donePath)
	if err != nil {
		t.Fatalf("failed to read done-0.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, itemID) {
		t.Errorf("done-0.md does not contain item UUID %s\nContent:\n%s", itemID, content)
	}

	// done-2.md should NOT exist (only 2 criteria: index 0 and 1)
	done2Path := filepath.Join(worktree, backlogCommandsDir, "done-2.md")
	if _, err := os.Stat(done2Path); !os.IsNotExist(err) {
		t.Errorf("done-2.md should not exist for a 2-criteria item")
	}
}

// TestWriteSlashCommands_BlockAndDuplicateFilesContainItemIDAndGatingLanguage
// verifies block.md and duplicate.md (written via buildBlockAndDuplicateCommands)
// substitute the item's UUID and each explain the work-role-session gating and
// the SkipReviewGate exception, so an agent reading either file understands why
// a call might be rejected before it tries.
func TestWriteSlashCommands_BlockAndDuplicateFilesContainItemIDAndGatingLanguage(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	itemID := "550e8400-e29b-41d4-a716-446655440002"
	ac := `[{"index":0,"text":"Do something","status":"pending"}]`
	item := makeTestBacklogItemWithID(itemID, "Feature", ac)

	if err := WriteSlashCommands(nil, item, worktree); err != nil {
		t.Fatalf("WriteSlashCommands returned error: %v", err)
	}

	blockPath := filepath.Join(worktree, backlogCommandsDir, "block.md")
	blockData, err := os.ReadFile(blockPath)
	if err != nil {
		t.Fatalf("failed to read block.md: %v", err)
	}
	blockContent := string(blockData)
	blockMustContain := []string{itemID, "report_blocked", "work-role session"}
	for _, want := range blockMustContain {
		if !strings.Contains(blockContent, want) {
			t.Errorf("expected block.md to contain %q\nContent:\n%s", want, blockContent)
		}
	}

	duplicatePath := filepath.Join(worktree, backlogCommandsDir, "duplicate.md")
	duplicateData, err := os.ReadFile(duplicatePath)
	if err != nil {
		t.Fatalf("failed to read duplicate.md: %v", err)
	}
	duplicateContent := string(duplicateData)
	duplicateMustContain := []string{itemID, "report_duplicate", "work-role session", "skip the review gate"}
	for _, want := range duplicateMustContain {
		if !strings.Contains(duplicateContent, want) {
			t.Errorf("expected duplicate.md to contain %q\nContent:\n%s", want, duplicateContent)
		}
	}
}

// TestPruneStaleSlashCommandFiles_should_RemoveExtraFiles_When_NewItemHasFewerCriteria
// verifies pruneStaleSlashCommandFiles deletes done-N.md/fail-N.md files whose index is
// not present in the newly generated file set, while leaving unrelated always-present
// files (status.md) untouched.
func TestPruneStaleSlashCommandFiles_should_RemoveExtraFiles_When_NewItemHasFewerCriteria(t *testing.T) {
	cmdDir := t.TempDir()
	for i := 0; i < 8; i++ {
		for _, prefix := range []string{"done", "fail"} {
			name := fmt.Sprintf("%s-%d.md", prefix, i)
			if err := os.WriteFile(filepath.Join(cmdDir, name), []byte("old content"), 0o644); err != nil {
				t.Fatalf("seed %s: %v", name, err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "status.md"), []byte("status"), 0o644); err != nil {
		t.Fatalf("seed status.md: %v", err)
	}

	newFiles := map[string]string{
		"status.md": "status", "done-0.md": "new", "fail-0.md": "new",
		"done-1.md": "new", "fail-1.md": "new", "done-2.md": "new", "fail-2.md": "new",
	}
	pruneStaleSlashCommandFiles(cmdDir, newFiles)

	for i := 3; i < 8; i++ {
		for _, prefix := range []string{"done", "fail"} {
			path := filepath.Join(cmdDir, fmt.Sprintf("%s-%d.md", prefix, i))
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("expected %s to be pruned, but it still exists", filepath.Base(path))
			}
		}
	}
	for i := 0; i < 3; i++ {
		for _, prefix := range []string{"done", "fail"} {
			path := filepath.Join(cmdDir, fmt.Sprintf("%s-%d.md", prefix, i))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("expected %s to be preserved, but it was removed", filepath.Base(path))
			}
		}
	}
	if _, err := os.Stat(filepath.Join(cmdDir, "status.md")); os.IsNotExist(err) {
		t.Error("status.md should never be pruned by index-based logic")
	}
}

// TestPruneStaleSlashCommandFiles_should_PreserveAllFiles_When_NewItemHasMoreOrEqualCriteria
// verifies pruneStaleSlashCommandFiles never deletes a file that's about to be rewritten.
func TestPruneStaleSlashCommandFiles_should_PreserveAllFiles_When_NewItemHasMoreOrEqualCriteria(t *testing.T) {
	cmdDir := t.TempDir()
	for i := 0; i < 3; i++ {
		for _, prefix := range []string{"done", "fail"} {
			name := fmt.Sprintf("%s-%d.md", prefix, i)
			if err := os.WriteFile(filepath.Join(cmdDir, name), []byte("old"), 0o644); err != nil {
				t.Fatalf("seed %s: %v", name, err)
			}
		}
	}

	newFiles := map[string]string{}
	for i := 0; i < 10; i++ {
		newFiles[fmt.Sprintf("done-%d.md", i)] = "new"
		newFiles[fmt.Sprintf("fail-%d.md", i)] = "new"
	}
	pruneStaleSlashCommandFiles(cmdDir, newFiles)

	for i := 0; i < 3; i++ {
		for _, prefix := range []string{"done", "fail"} {
			path := filepath.Join(cmdDir, fmt.Sprintf("%s-%d.md", prefix, i))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("expected %s to survive when the new set is a superset, but it was removed", filepath.Base(path))
			}
		}
	}
}

// TestWriteSlashCommands_should_MatchExactFileSet_When_CalledTwiceWithDifferentCriteriaCounts
// exercises pruneStaleSlashCommandFiles through the real WriteSlashCommands entry point:
// an 8-AC item's files followed by a 3-AC item's must leave exactly the 3-AC file set on
// disk, with done-0.md content carrying the second item's id.
func TestWriteSlashCommands_should_MatchExactFileSet_When_CalledTwiceWithDifferentCriteriaCounts(t *testing.T) {
	worktree := t.TempDir()

	buildAC := func(n int) string {
		var sb strings.Builder
		sb.WriteString("[")
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `{"index":%d,"text":"Criterion %d","status":"pending"}`, i, i)
		}
		sb.WriteString("]")
		return sb.String()
	}

	oldItemID := "old-item-id"
	oldItem := makeTestBacklogItemWithID(oldItemID, "Old item", buildAC(8))
	if err := WriteSlashCommands(nil, oldItem, worktree); err != nil {
		t.Fatalf("WriteSlashCommands (first call, 8 AC) failed: %v", err)
	}

	newItemID := "new-item-id"
	newItem := makeTestBacklogItemWithID(newItemID, "New item", buildAC(3))
	if err := WriteSlashCommands(nil, newItem, worktree); err != nil {
		t.Fatalf("WriteSlashCommands (second call, 3 AC) failed: %v", err)
	}

	cmdDir := filepath.Join(worktree, backlogCommandsDir)
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		t.Fatalf("read command dir: %v", err)
	}
	wantFiles := map[string]bool{
		"status.md": true, "review.md": true, "ship.md": true, "help.md": true,
		"block.md": true, "duplicate.md": true,
		"done-0.md": true, "fail-0.md": true, "done-1.md": true, "fail-1.md": true,
		"done-2.md": true, "fail-2.md": true,
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name()] = true
	}
	if len(got) != len(wantFiles) {
		t.Errorf("expected exactly %d files for the 3-AC item, got %d: %v", len(wantFiles), len(got), got)
	}
	for name := range wantFiles {
		if !got[name] {
			t.Errorf("expected %s to exist after relink to the 3-AC item", name)
		}
	}
	for name := range got {
		if !wantFiles[name] {
			t.Errorf("stale file %s from the 8-AC item leaked into the 3-AC item's file set", name)
		}
	}

	data, err := os.ReadFile(filepath.Join(cmdDir, "done-0.md"))
	if err != nil {
		t.Fatalf("read done-0.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, newItemID) {
		t.Errorf("done-0.md should contain the second item's id %q, got:\n%s", newItemID, content)
	}
	if strings.Contains(content, oldItemID) {
		t.Errorf("done-0.md should not contain the stale first item's id %q, got:\n%s", oldItemID, content)
	}
}

// TestWriteSlashCommands_ReviewFileInstructsShipOnPassAndAfterAttemptCap verifies
// review.md tells the agent to run /backlog/ship both immediately on a PASS
// verdict and as a bounded escape hatch once the server-tracked attempt count
// (MaxSameSessionReviewAttempts) reports the cap is hit — closing the gap where
// review.md previously said "PASS → you're done" and "Keep looping until PASS"
// with no mention of /backlog/ship at all (see
// de6d7878-9d6e-4081-acfa-02ff545c87b4, 2026-07-20).
func TestWriteSlashCommands_ReviewFileInstructsShipOnPassAndAfterAttemptCap(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	itemID := "550e8400-e29b-41d4-a716-446655440001"
	ac := `[{"index":0,"text":"Do something","status":"pending"}]`
	item := makeTestBacklogItemWithID(itemID, "Feature", ac)

	if err := WriteSlashCommands(nil, item, worktree); err != nil {
		t.Fatalf("WriteSlashCommands returned error: %v", err)
	}

	reviewPath := filepath.Join(worktree, backlogCommandsDir, "review.md")
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("failed to read review.md: %v", err)
	}
	content := string(data)

	mustContain := []string{
		"/backlog/ship",
		fmt.Sprintf("%d allowed in this session", MaxSameSessionReviewAttempts),
	}
	for _, want := range mustContain {
		if !strings.Contains(content, want) {
			t.Errorf("expected review.md to contain %q\nContent:\n%s", want, content)
		}
	}

	mustNotContain := "you're done"
	if strings.Contains(content, mustNotContain) {
		t.Errorf("did not expect review.md to say a PASS verdict ends the task without shipping\nContent:\n%s", content)
	}
}

// TestWriteBacklogContextFile_WritesFileWithExpectedContent verifies the context file
// contains BuildSessionInitialPrompt output and the fallback instructions block.
func TestWriteBacklogContextFile_WritesFileWithExpectedContent(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	ac := `[{"index":0,"text":"Implement handler","status":"pending"}]`
	item := &BacklogItemData{
		ID:                 "test-item-id-2",
		Title:              "My Backlog Item",
		Description:        "A test description",
		AcceptanceCriteria: AcCriteriaJSON(ac),
		Status:             "ready",
		Priority:           2,
	}

	if err := WriteBacklogContextFile(item, nil, worktree); err != nil {
		t.Fatalf("WriteBacklogContextFile returned error: %v", err)
	}

	contextPath := filepath.Join(worktree, ".backlog-context.md")
	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("failed to read .backlog-context.md: %v", err)
	}
	content := string(data)

	// Must contain the prompt from BuildSessionInitialPrompt.
	expected := BuildSessionInitialPrompt(item, nil)
	if !strings.Contains(content, expected[:100]) {
		t.Errorf("file content does not match BuildSessionInitialPrompt output\nContent:\n%s", content[:200])
	}

	// Must contain the fallback instructions block.
	if !strings.Contains(content, "Fallback Instructions") {
		t.Errorf("expected 'Fallback Instructions' block in context file\nContent:\n%s", content)
	}
	if !strings.Contains(content, "MCP tools are unavailable") {
		t.Errorf("expected fallback text in context file\nContent:\n%s", content)
	}
}

// TestWriteBacklogContextFile_ConcurrentCallsToSameWorktree_NeverLeaveFileMissingOrTruncated
// is a regression test for the fixed-tmp-filename write race WriteBacklogContextFile used
// to have (destPath+".tmp") — it's called on every spawn AND re-attach for the same
// worktreePath, so two concurrent callers must never interleave writes and rename a
// torn/truncated .backlog-context.md into place. Mirrors config_test.go's
// TestSaveConfig_ConcurrentWritesToSamePath.
func TestWriteBacklogContextFile_ConcurrentCallsToSameWorktree_NeverLeaveFileMissingOrTruncated(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	item := &BacklogItemData{ID: "concurrent-test-item", Title: "Concurrent Test", Status: "ready"}

	const n = 20
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- WriteBacklogContextFile(item, nil, worktree)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Errorf("WriteBacklogContextFile must not error under concurrent callers targeting the same worktree: %v", err)
		}
	}

	contextPath := filepath.Join(worktree, ".backlog-context.md")
	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf(".backlog-context.md must exist after concurrent WriteBacklogContextFile calls: %v", err)
	}
	if !strings.Contains(string(data), "Fallback Instructions") {
		t.Errorf(".backlog-context.md must contain the full fallback instructions block, not a torn/truncated write:\n%s", data)
	}
}

// TestWriteBacklogContextFile_IncludesPlanArtifactsPath verifies that when the item
// has an approved plan, the on-disk fallback file the agent re-reads after context
// compaction includes the same "Your plan is at .../plan.md" reminder the live CLI
// prompt gets — this is the exact consistency this PR's fix moved into
// BuildSessionInitialPrompt so both channels render identically by construction.
func TestWriteBacklogContextFile_IncludesPlanArtifactsPath(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	ac := `[{"index":0,"text":"Implement handler","status":"pending"}]`
	item := &BacklogItemData{
		ID:                 "test-item-id-3",
		Title:              "My Backlog Item",
		Description:        "A test description",
		AcceptanceCriteria: AcCriteriaJSON(ac),
		Status:             "ready",
		Priority:           2,
		PlanArtifactsPath:  "/tmp/plans/my-item",
	}

	if err := WriteBacklogContextFile(item, nil, worktree); err != nil {
		t.Fatalf("WriteBacklogContextFile returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(worktree, ".backlog-context.md"))
	if err != nil {
		t.Fatalf("failed to read .backlog-context.md: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "/tmp/plans/my-item/plan.md") {
		t.Errorf("expected plan-artifacts reminder in context file\nContent:\n%s", content)
	}
}

// TestWriteBacklogContextFile_IncludesPriorSessions verifies a real, non-nil
// priorSessions slice is actually threaded through to the rendered content — the
// prior signature-only update (always passing nil) never exercised this contract.
func TestWriteBacklogContextFile_IncludesPriorSessions(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	ac := `[{"index":0,"text":"Implement handler","status":"pending"}]`
	item := &BacklogItemData{
		ID:                 "test-item-id-4",
		Title:              "My Backlog Item",
		Description:        "A test description",
		AcceptanceCriteria: AcCriteriaJSON(ac),
		Status:             "ready",
		Priority:           2,
	}
	ended := time.Now().Add(-time.Hour)
	priorSessions := []ItemSessionSummary{
		{
			Role:                  "work",
			EndedAt:               &ended,
			CommitCountSinceSpawn: 3,
			LastCommitMessage:     "fix the thing",
		},
	}

	if err := WriteBacklogContextFile(item, priorSessions, worktree); err != nil {
		t.Fatalf("WriteBacklogContextFile returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(worktree, ".backlog-context.md"))
	if err != nil {
		t.Fatalf("failed to read .backlog-context.md: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "Prior Attempts") {
		t.Errorf("expected 'Prior Attempts' section when priorSessions is non-nil\nContent:\n%s", content)
	}
	if !strings.Contains(content, "fix the thing") {
		t.Errorf("expected prior session's last commit message in context file\nContent:\n%s", content)
	}
}

// TestCleanupSlashCommands_NoErrorWhenAbsent verifies cleanup doesn't error on missing dir.
func TestCleanupSlashCommands_NoErrorWhenAbsent(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	if err := CleanupSlashCommands(worktree); err != nil {
		t.Errorf("CleanupSlashCommands should not error when dir absent, got: %v", err)
	}
}

// TestCleanupBacklogContextFile_NoErrorWhenAbsent verifies cleanup doesn't error on missing file.
func TestCleanupBacklogContextFile_NoErrorWhenAbsent(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()
	if err := CleanupBacklogContextFile(worktree); err != nil {
		t.Errorf("CleanupBacklogContextFile should not error when file absent, got: %v", err)
	}
}

// TestWriteBacklogContextFile_UntracksPreviouslyCommittedContextFile is the core
// regression test for the chronic bug this PR fixes: a branch that at some point got
// .backlog-context.md committed (however that happened — see git.ScaffoldingExcludePatterns'
// doc comment) must self-heal the next time a session is spawned/reattached on it,
// rather than requiring a manual "chore(backlog): untrack ..." commit. Confirmed via
// `git log --all --oneline -- "*.backlog-context.md"` in this repo's own history:
// dozens of such manual-untrack commits exist because addWorktreeExcludes' git-exclude
// approach only prevents NEW files from being staged — it does nothing once a file is
// already tracked.
func TestWriteBacklogContextFile_UntracksPreviouslyCommittedContextFile(t *testing.T) {
	t.Parallel()
	repo := setupTestGitRepo(t)

	// Simulate the historical pollution: .backlog-context.md gets committed to the branch.
	stalePath := filepath.Join(repo, ".backlog-context.md")
	if err := os.WriteFile(stalePath, []byte("stale content from a prior, buggy version"), 0o644); err != nil {
		t.Fatalf("failed to write stale context file: %v", err)
	}
	addCmd := safeexec.CommandContext(context.Background(), "git", "add", ".backlog-context.md")
	addCmd.Dir = repo
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s: %v", out, err)
	}
	commitCmd := safeexec.CommandContext(context.Background(), "git", "commit", "-m", "accidentally track backlog context")
	commitCmd.Dir = repo
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %s: %v", out, err)
	}

	// Sanity check: the file really is tracked before the fix runs.
	if !containsPath(gitLsFiles(t, repo), ".backlog-context.md") {
		t.Fatalf("test setup failed: .backlog-context.md is not tracked in the index")
	}

	// Now simulate the next session spawn on this branch.
	ac := `[{"index":0,"text":"Do something","status":"pending"}]`
	item := makeTestBacklogItemWithID("test-item-untrack", "Feature", ac)
	if err := WriteBacklogContextFile(item, nil, repo); err != nil {
		t.Fatalf("WriteBacklogContextFile returned error: %v", err)
	}

	// The file must no longer be tracked in the index...
	if containsPath(gitLsFiles(t, repo), ".backlog-context.md") {
		t.Errorf(".backlog-context.md is still tracked in the git index after WriteBacklogContextFile — self-heal did not run")
	}

	// ...but the working-tree file must still exist with fresh content (git rm --cached
	// semantics: untrack, don't delete).
	data, err := os.ReadFile(stalePath)
	if err != nil {
		t.Fatalf("expected .backlog-context.md to still exist on disk after untracking: %v", err)
	}
	if strings.Contains(string(data), "stale content from a prior") {
		t.Errorf("expected fresh content after WriteBacklogContextFile, found stale content: %s", data)
	}
	if !strings.Contains(string(data), "Fallback Instructions") {
		t.Errorf("expected fresh WriteBacklogContextFile content, got: %s", data)
	}
}

// TestWriteSlashCommands_UntracksPreviouslyCommittedSlashCommandFiles verifies the same
// self-heal for .claude/commands/backlog/ — the other half of the "chore(backlog):
// untrack backlog context/command files" incident history.
func TestWriteSlashCommands_UntracksPreviouslyCommittedSlashCommandFiles(t *testing.T) {
	t.Parallel()
	repo := setupTestGitRepo(t)

	cmdDir := filepath.Join(repo, backlogCommandsDir)
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("failed to create command dir: %v", err)
	}
	stalePath := filepath.Join(cmdDir, "status.md")
	if err := os.WriteFile(stalePath, []byte("stale status.md"), 0o644); err != nil {
		t.Fatalf("failed to write stale status.md: %v", err)
	}
	addCmd := safeexec.CommandContext(context.Background(), "git", "add", ".claude/commands/backlog/status.md")
	addCmd.Dir = repo
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %s: %v", out, err)
	}
	commitCmd := safeexec.CommandContext(context.Background(), "git", "commit", "-m", "accidentally track slash commands")
	commitCmd.Dir = repo
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %s: %v", out, err)
	}

	if !containsPath(gitLsFiles(t, repo), ".claude/commands/backlog/status.md") {
		t.Fatalf("test setup failed: status.md is not tracked in the index")
	}

	ac := `[{"index":0,"text":"Do something","status":"pending"}]`
	item := makeTestBacklogItemWithID("test-item-untrack-2", "Feature", ac)
	if err := WriteSlashCommands(nil, item, repo); err != nil {
		t.Fatalf("WriteSlashCommands returned error: %v", err)
	}

	tracked := gitLsFiles(t, repo)
	for _, p := range tracked {
		if strings.HasPrefix(p, ".claude/commands/backlog/") {
			t.Errorf("expected no .claude/commands/backlog/ files to remain tracked, found: %s", p)
		}
	}

	data, err := os.ReadFile(stalePath)
	if err != nil {
		t.Fatalf("expected status.md to still exist on disk after untracking: %v", err)
	}
	if string(data) == "stale status.md" {
		t.Errorf("expected fresh status.md content, found stale content")
	}
}

// TestWriteBacklogContextFile_NoStaleContentLeaksAcrossRespawn verifies that respawning
// (or reattaching) on the same worktree path always replaces prior content in full —
// addressing the "scoped by session/backlog item so it's not being reused poorly"
// half of the request: even though the file lives at a fixed relative path in the
// worktree, every write is a full atomic overwrite (WriteBacklogContextFile's
// tmp-file-then-rename), so a later spawn can never see content mixed from an earlier
// item or session.
func TestWriteBacklogContextFile_NoStaleContentLeaksAcrossRespawn(t *testing.T) {
	t.Parallel()
	worktree := t.TempDir()

	firstAC := `[{"index":0,"text":"First item criterion","status":"pending"}]`
	firstItem := makeTestBacklogItemWithID("item-alpha", "Alpha Feature — unique-marker-alpha", firstAC)
	if err := WriteBacklogContextFile(firstItem, nil, worktree); err != nil {
		t.Fatalf("first WriteBacklogContextFile returned error: %v", err)
	}

	contextPath := filepath.Join(worktree, ".backlog-context.md")
	firstData, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("failed to read context file after first write: %v", err)
	}
	if !strings.Contains(string(firstData), "unique-marker-alpha") {
		t.Fatalf("expected first write to contain unique-marker-alpha, got: %s", firstData)
	}

	secondAC := `[{"index":0,"text":"Second item criterion","status":"pending"}]`
	secondItem := makeTestBacklogItemWithID("item-beta", "Beta Feature — unique-marker-beta", secondAC)
	if err := WriteBacklogContextFile(secondItem, nil, worktree); err != nil {
		t.Fatalf("second WriteBacklogContextFile returned error: %v", err)
	}

	secondData, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("failed to read context file after second write: %v", err)
	}
	if strings.Contains(string(secondData), "unique-marker-alpha") {
		t.Errorf("second write leaked stale content from the first item's spawn: %s", secondData)
	}
	if !strings.Contains(string(secondData), "unique-marker-beta") {
		t.Errorf("expected second write to contain unique-marker-beta, got: %s", secondData)
	}
}

// TestUntrackTrackedScaffolding_NoErrorOnNonGitDirectory verifies the self-heal helper
// degrades gracefully (no error, nothing untracked) for directory-mode sessions that
// have no git backing at all — mirroring addWorktreeExcludes' existing best-effort
// handling of the same case.
func TestUntrackTrackedScaffolding_NoErrorOnNonGitDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	removed, err := git.UntrackScaffolding(dir, git.ScaffoldingExcludePatterns)
	if err != nil {
		t.Errorf("expected no error for a non-git directory, got: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected nothing removed for a non-git directory, got: %v", removed)
	}
}

// TestUntrackTrackedScaffolding_LeavesUnrelatedTrackedFilesAlone verifies the self-heal
// only touches paths matching git.ScaffoldingExcludePatterns and leaves everything else in
// the index untouched.
func TestUntrackTrackedScaffolding_LeavesUnrelatedTrackedFilesAlone(t *testing.T) {
	t.Parallel()
	repo := setupTestGitRepo(t)

	before := gitLsFiles(t, repo)
	if !containsPath(before, "README.md") {
		t.Fatalf("test setup failed: README.md is not tracked")
	}

	removed, err := git.UntrackScaffolding(repo, git.ScaffoldingExcludePatterns)
	if err != nil {
		t.Fatalf("UntrackScaffolding returned error: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected nothing to be untracked (no scaffolding files present), got: %v", removed)
	}

	after := gitLsFiles(t, repo)
	if !containsPath(after, "README.md") {
		t.Errorf("README.md was unexpectedly untracked")
	}
}

// TestAddWorktreeExcludes_LinkedWorktree_ExcludesScaffoldingFiles is the regression test
// for the bug this item fixes: addWorktreeExcludes must write to the git-COMMON info/exclude
// (shared by every worktree), not the worktree-private admin dir's info/exclude (which
// `git status` never reads). A plain setupTestGitRepo fixture can't detect this bug class —
// --git-dir and --git-common-dir resolve identically there. This test only fails against the
// pre-fix code (--git-dir); see the accompanying commit message for the RED/GREEN record.
func TestAddWorktreeExcludes_LinkedWorktree_ExcludesScaffoldingFiles(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)
	worktreePath := setupLinkedWorktree(t, repoPath, "excludes-regression")

	if err := os.WriteFile(filepath.Join(worktreePath, ".backlog-context.md"), []byte("scaffolding"), 0o644); err != nil {
		t.Fatalf("failed to write .backlog-context.md: %v", err)
	}

	addWorktreeExcludes(worktreePath)

	dirty, err := IsWorktreeDirty(context.Background(), worktreePath)
	if err != nil {
		t.Fatalf("IsWorktreeDirty returned error: %v", err)
	}
	if dirty {
		t.Errorf("expected worktree to be clean after addWorktreeExcludes, but IsWorktreeDirty returned true — " +
			"the exclude pattern likely landed in the worktree-private admin dir instead of the shared git-common-dir")
	}
}

// TestAddWorktreeExcludes_PlainRepo_GitCommonDirEqualsGitDir pins the mechanism by which
// the --git-common-dir fix is a no-op for plain (non-worktree) repos: both flags must
// resolve to the same directory there.
func TestAddWorktreeExcludes_PlainRepo_GitCommonDirEqualsGitDir(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)

	// Both sides must go through git's own --path-format=absolute resolution.
	// t.TempDir() on macOS returns a path through the /var -> /private/var
	// symlink; git's absolute-path formatting resolves it but a manual
	// filepath.Join of the unresolved repoPath onto a relative --git-dir does
	// not, producing a false mismatch unrelated to --git-dir vs
	// --git-common-dir semantics.
	gitDir := runGitRevParse(t, repoPath, "--path-format=absolute", "--git-dir")
	commonDir := runGitRevParse(t, repoPath, "--path-format=absolute", "--git-common-dir")

	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoPath, gitDir)
	}

	// Resolve symlinks on both sides: t.TempDir() on macOS returns a path under
	// /var/folders/..., which is itself a symlink to /private/var/.... git's
	// --path-format=absolute resolves through that symlink, but joining the
	// (unresolved) repoPath onto the relative --git-dir output does not, so the
	// two would otherwise mismatch on symlink resolution alone rather than on
	// any real --git-common-dir behavior difference.
	resolvedGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for git-dir %s: %v", gitDir, err)
	}
	resolvedCommonDir, err := filepath.EvalSymlinks(commonDir)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for git-common-dir %s: %v", commonDir, err)
	}
	if resolvedGitDir != resolvedCommonDir {
		t.Errorf("expected --git-dir (%s) and --git-common-dir (%s) to resolve identically for a plain repo", resolvedGitDir, resolvedCommonDir)
	}
}

func runGitRevParse(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", append([]string{"rev-parse"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// TestGetWorktreeDirtyPaths_EmptyForScaffoldingOnlyLinkedWorktree proves Epic 1.2's
// exclude-path fix and GetWorktreeDirtyPaths compose correctly end-to-end: a linked
// worktree containing only scaffolding files must never be reported dirty.
func TestGetWorktreeDirtyPaths_EmptyForScaffoldingOnlyLinkedWorktree(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)
	worktreePath := setupLinkedWorktree(t, repoPath, "scaffolding-only")

	addWorktreeExcludes(worktreePath)

	if err := os.WriteFile(filepath.Join(worktreePath, ".backlog-context.md"), []byte("scaffolding"), 0o644); err != nil {
		t.Fatalf("failed to write .backlog-context.md: %v", err)
	}
	cmdDir := filepath.Join(worktreePath, ".claude", "commands", "backlog")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("failed to create .claude/commands/backlog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "status.md"), []byte("status"), 0o644); err != nil {
		t.Fatalf("failed to write status.md: %v", err)
	}

	paths, err := GetWorktreeDirtyPaths(worktreePath)
	if err != nil {
		t.Fatalf("GetWorktreeDirtyPaths returned error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected no dirty paths for a scaffolding-only worktree, got %v", paths)
	}
}
