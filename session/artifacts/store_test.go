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

func TestScanFile_SkipsAgentFiles(t *testing.T) {
	called := false
	ae := NewArtifactExtractor(
		func(title, blob string) error { called = true; return nil },
		func(title string) (string, error) { return "", nil },
		func(filePath string) (string, bool) { return "test", true },
	)
	ae.OnHistoryFileChanged("/home/user/.claude/projects/foo/agent-abc123.jsonl")
	// Process any queued items synchronously — queue should be empty.
	select {
	case <-ae.queue:
		t.Fatal("agent- file should not have been enqueued")
	default:
	}
	if called {
		t.Fatal("storeFn should not be called for agent- files")
	}
}
