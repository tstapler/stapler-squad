package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
// status.md + done-0.md + fail-0.md + done-1.md + fail-1.md + review.md + ship.md + help.md = 8 files.
func TestWriteSlashCommands_CreatesCorrectFileCount(t *testing.T) {
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

// TestWriteBacklogContextFile_WritesFileWithExpectedContent verifies the context file
// contains BuildSessionInitialPrompt output and the fallback instructions block.
func TestWriteBacklogContextFile_WritesFileWithExpectedContent(t *testing.T) {
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

// TestWriteBacklogContextFile_IncludesPlanArtifactsPath verifies that when the item
// has an approved plan, the on-disk fallback file the agent re-reads after context
// compaction includes the same "Your plan is at .../plan.md" reminder the live CLI
// prompt gets — this is the exact consistency this PR's fix moved into
// BuildSessionInitialPrompt so both channels render identically by construction.
func TestWriteBacklogContextFile_IncludesPlanArtifactsPath(t *testing.T) {
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
	worktree := t.TempDir()
	if err := CleanupSlashCommands(worktree); err != nil {
		t.Errorf("CleanupSlashCommands should not error when dir absent, got: %v", err)
	}
}

// TestCleanupBacklogContextFile_NoErrorWhenAbsent verifies cleanup doesn't error on missing file.
func TestCleanupBacklogContextFile_NoErrorWhenAbsent(t *testing.T) {
	worktree := t.TempDir()
	if err := CleanupBacklogContextFile(worktree); err != nil {
		t.Errorf("CleanupBacklogContextFile should not error when file absent, got: %v", err)
	}
}
