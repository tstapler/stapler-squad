package artifacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeJSONLLine writes a minimal JSONL user/tool_result line containing the given text.
func writeJSONLLine(f *os.File, text string) error {
	entry := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role": "user",
			"content": []map[string]interface{}{
				{
					"type":    "tool_result",
					"content": text,
				},
			},
		},
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

func TestScanFile_IncrementalOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	// Write 2 lines, one with a PR URL.
	if err := writeJSONLLine(f, "Created: https://github.com/owner/repo/pull/1"); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONLLine(f, "No artifacts here"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Track store calls.
	var stored []string
	ae := NewArtifactExtractor(
		func(title, blob string) error {
			stored = append(stored, blob)
			return nil
		},
		func(title string) (string, error) {
			if len(stored) > 0 {
				return stored[len(stored)-1], nil
			}
			return "", nil
		},
		func(filePath string) (string, bool) {
			return "test-session", true
		},
	)

	// First scan.
	ae.scanFile(path)

	if len(stored) == 0 {
		t.Fatal("expected storeFn to be called after first scan")
	}
	var blob1 SessionArtifactsBlob
	if err := json.Unmarshal([]byte(stored[0]), &blob1); err != nil {
		t.Fatal(err)
	}
	if len(blob1.PRURLs) != 1 || blob1.PRURLs[0] != "https://github.com/owner/repo/pull/1" {
		t.Fatalf("expected 1 PR URL after first scan, got %v", blob1.PRURLs)
	}
	offset1 := blob1.ScanOffsetBytes

	// Append 2 more lines to the file.
	f2, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSONLLine(f2, "New PR: https://github.com/owner/repo/pull/2"); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONLLine(f2, "Also see https://example.com/docs"); err != nil {
		t.Fatal(err)
	}
	if err := f2.Close(); err != nil {
		t.Fatal(err)
	}

	// Second scan — should read only new bytes (from offset1 onward).
	prevCount := len(stored)
	ae.scanFile(path)

	if len(stored) <= prevCount {
		t.Fatal("expected storeFn to be called after second scan")
	}
	var blob2 SessionArtifactsBlob
	if err := json.Unmarshal([]byte(stored[len(stored)-1]), &blob2); err != nil {
		t.Fatal(err)
	}
	// After merge: should have both PR URLs
	if len(blob2.PRURLs) != 2 {
		t.Fatalf("expected 2 PR URLs after merge, got %v", blob2.PRURLs)
	}
	// Offset must have advanced past the first scan's offset
	if blob2.ScanOffsetBytes <= offset1 {
		t.Fatalf("expected offset to advance: got %d, was %d", blob2.ScanOffsetBytes, offset1)
	}
}

// C-2: TestScanFile_SkipsAgentFiles checks the queue channel directly rather than
// relying on a worker running. An agent- file must never be enqueued.
func TestScanFile_SkipsAgentFiles(t *testing.T) {
	ae := NewArtifactExtractor(
		func(title, blob string) error { t.Error("storeFn must not be called for agent- files"); return nil },
		func(title string) (string, error) { return "", nil },
		func(filePath string) (string, bool) { return "test", true },
	)
	ae.OnHistoryFileChanged("/home/user/.claude/projects/foo/agent-abc123.jsonl")
	select {
	case path := <-ae.queue:
		t.Fatalf("agent- file must not be enqueued, got: %q", path)
	default:
		// correct: queue is empty
	}
}

// C-2 (second test): non-agent .jsonl files must be enqueued.
func TestOnHistoryFileChanged_EnqueuesNonAgentJSONL(t *testing.T) {
	ae := NewArtifactExtractor(
		func(title, blob string) error { return nil },
		func(title string) (string, error) { return "", nil },
		func(filePath string) (string, bool) { return "test", true },
	)
	const path = "/home/user/.claude/projects/foo/session.jsonl"
	ae.OnHistoryFileChanged(path)
	select {
	case got := <-ae.queue:
		if got != path {
			t.Fatalf("unexpected path enqueued: %q", got)
		}
	default:
		t.Fatal("expected non-agent .jsonl to be enqueued")
	}
}

// C-3: TestScanFile_CallsOnScanComplete verifies the OnScanComplete callback fires.
func TestScanFile_CallsOnScanComplete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSONLLine(f, "PR: https://github.com/owner/repo/pull/99"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	var completedTitle string
	var completedBlob *SessionArtifactsBlob
	ae := NewArtifactExtractor(
		func(title, blob string) error { return nil },
		func(title string) (string, error) { return "", nil },
		func(filePath string) (string, bool) { return "my-session", true },
	)
	ae.OnScanComplete = func(title string, blob *SessionArtifactsBlob) {
		completedTitle = title
		completedBlob = blob
	}
	ae.scanFile(path)

	if completedTitle != "my-session" {
		t.Fatalf("OnScanComplete title = %q, want %q", completedTitle, "my-session")
	}
	if completedBlob == nil || len(completedBlob.PRURLs) != 1 {
		t.Fatalf("OnScanComplete blob = %+v, want 1 PR URL", completedBlob)
	}
	if completedBlob.PRURLs[0] != "https://github.com/owner/repo/pull/99" {
		t.Fatalf("unexpected PR URL: %s", completedBlob.PRURLs[0])
	}
}

// C-4: TestSeedOffsets_RestoresOffsetFromStoredBlob verifies startup hydration.
func TestSeedOffsets_RestoresOffsetFromStoredBlob(t *testing.T) {
	storedBlob := SessionArtifactsBlob{
		PRURLs:          []string{"https://github.com/o/r/pull/1"},
		ScanOffsetBytes: 1234,
	}
	raw, _ := json.Marshal(storedBlob)

	ae := NewArtifactExtractor(
		func(title, blob string) error { return nil },
		func(title string) (string, error) { return string(raw), nil },
		func(filePath string) (string, bool) { return "s1", true },
	)
	instances := []InstanceInfo{{Title: "s1", HistoryFilePath: "/path/to/s1.jsonl"}}
	ae.SeedOffsets(instances)

	ae.offsetsMu.Lock()
	got := ae.offsets["/path/to/s1.jsonl"]
	ae.offsetsMu.Unlock()

	if got != 1234 {
		t.Fatalf("SeedOffsets: offset = %d, want 1234", got)
	}
}

// C-4 (second test): SeedOffsets silently skips sessions with no stored blob.
func TestSeedOffsets_IgnoresMissingBlob(t *testing.T) {
	ae := NewArtifactExtractor(
		func(title, blob string) error { return nil },
		func(title string) (string, error) { return "", nil },
		func(filePath string) (string, bool) { return "s", true },
	)
	ae.SeedOffsets([]InstanceInfo{{Title: "s1", HistoryFilePath: "/path/to/s1.jsonl"}})
	ae.offsetsMu.Lock()
	got := ae.offsets["/path/to/s1.jsonl"]
	ae.offsetsMu.Unlock()
	if got != 0 {
		t.Fatalf("SeedOffsets: expected 0 offset for missing blob, got %d", got)
	}
}

// M-9: TestMergeAndPersist_ExternalURLCapAt50 verifies the 50-URL cap is enforced
// at merge time (not in ExtractFromToolResult).
func TestMergeAndPersist_ExternalURLCapAt50(t *testing.T) {
	// Seed 30 existing URLs in the stored blob.
	existing := make([]string, 30)
	for i := range existing {
		existing[i] = fmt.Sprintf("https://existing.com/%d", i)
	}
	storedBlob := SessionArtifactsBlob{ExternalURLs: existing}
	raw, _ := json.Marshal(storedBlob)

	ae := NewArtifactExtractor(
		func(title, blob string) error { return nil },
		func(title string) (string, error) { return string(raw), nil },
		func(filePath string) (string, bool) { return "s", true },
	)

	// 30 new unique URLs → total would be 60, should be capped at 50.
	newURLs := make([]string, 30)
	for i := range newURLs {
		newURLs[i] = fmt.Sprintf("https://new.com/%d", i)
	}
	blob := ae.mergeAndPersist("s", 999, nil, nil, newURLs, nil)
	if len(blob.ExternalURLs) != maxExternalURLs {
		t.Fatalf("expected %d external URLs after cap, got %d", maxExternalURLs, len(blob.ExternalURLs))
	}
}

// M-11: TestScanFile_AdvancesOffsetWhenNoArtifactsFound verifies offset advances even
// when no artifacts are found, so the same bytes are not re-scanned.
func TestScanFile_AdvancesOffsetWhenNoArtifactsFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	f, _ := os.Create(path)
	_ = writeJSONLLine(f, "No artifacts in this line")
	_ = f.Close()

	storeCalled := false
	ae := NewArtifactExtractor(
		func(title, blob string) error { storeCalled = true; return nil },
		func(title string) (string, error) { return "", nil },
		func(filePath string) (string, bool) { return "test-session", true },
	)
	ae.scanFile(path)

	// No artifacts found → storeFn should NOT be called (nothing to persist).
	if storeCalled {
		t.Error("storeFn should not be called when no artifacts found")
	}

	// But the offset must be advanced past the processed line so we don't re-scan.
	ae.offsetsMu.Lock()
	offset := ae.offsets[path]
	ae.offsetsMu.Unlock()
	if offset == 0 {
		t.Fatal("expected offset to advance past processed line, got 0")
	}
}

// M-12: TestScanFile_DoesNotAdvanceOffsetOnStoreFnError verifies that offset is NOT
// committed when storeFn fails, so the next scan can retry (M-2 fix).
func TestScanFile_DoesNotAdvanceOffsetOnStoreFnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	f, _ := os.Create(path)
	_ = writeJSONLLine(f, "PR: https://github.com/owner/repo/pull/1")
	_ = f.Close()

	ae := NewArtifactExtractor(
		func(title, blob string) error { return fmt.Errorf("db error") },
		func(title string) (string, error) { return "", nil },
		func(filePath string) (string, bool) { return "test-session", true },
	)
	ae.scanFile(path)

	// After storeFn failure, offset should NOT advance (so data can be retried).
	ae.offsetsMu.Lock()
	offset := ae.offsets[path]
	ae.offsetsMu.Unlock()
	if offset != 0 {
		t.Fatalf("expected offset=0 after storeFn error (so data can be retried), got %d", offset)
	}
}

// M-17: TestEnqueue_DeduplicatesInflightPaths verifies inflight dedup prevents
// the same path from being enqueued twice.
func TestEnqueue_DeduplicatesInflightPaths(t *testing.T) {
	ae := NewArtifactExtractor(
		func(title, blob string) error { return nil },
		func(title string) (string, error) { return "", nil },
		func(filePath string) (string, bool) { return "s", true },
	)
	const path = "/some/session.jsonl"
	ae.enqueue(path)
	ae.enqueue(path) // second enqueue of the same path should be dropped

	// Drain the queue — should have only 1 item.
	count := 0
	for {
		select {
		case <-ae.queue:
			count++
		default:
			goto done
		}
	}
done:
	if count != 1 {
		t.Fatalf("expected 1 enqueued item (dedup), got %d", count)
	}
}
