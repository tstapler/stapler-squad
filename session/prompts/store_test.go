package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEntryIDStability(t *testing.T) {
	text := "build the auth service"
	id1 := entryID(text)
	id2 := entryID(text)
	if id1 != id2 {
		t.Fatalf("entryID not stable: %q != %q", id1, id2)
	}
	if len(id1) != 64 {
		t.Fatalf("expected 64-char SHA-256 hex, got %d chars: %q", len(id1), id1)
	}
}

func TestEntryIDDifferentTexts(t *testing.T) {
	if entryID("a") == entryID("b") {
		t.Fatal("different texts must produce different IDs")
	}
}

func TestRingBufferEviction(t *testing.T) {
	dir := t.TempDir()
	store := NewPromptStore(filepath.Join(dir, "prompts.json"))

	// Fill to exactly maxEntries.
	for i := range maxEntries {
		_, err := store.RecordUsage(fmt.Sprintf("prompt-%d", i))
		if err != nil {
			t.Fatalf("RecordUsage(%d): %v", i, err)
		}
	}

	entries, err := store.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != maxEntries {
		t.Fatalf("expected %d entries, got %d", maxEntries, len(entries))
	}

	// The oldest entry: prompt-0 (added first, never re-used).
	// Sleep a tiny bit to ensure the 501st entry has a strictly newer LastUsed.
	time.Sleep(2 * time.Millisecond)
	_, err = store.RecordUsage("prompt-overflow")
	if err != nil {
		t.Fatalf("RecordUsage(overflow): %v", err)
	}

	after, err := store.List(0)
	if err != nil {
		t.Fatalf("List after overflow: %v", err)
	}
	if len(after) != maxEntries {
		t.Fatalf("expected %d entries after eviction, got %d", maxEntries, len(after))
	}

	// "prompt-overflow" must be present; prompt-0 must have been evicted.
	hasOverflow := false
	hasPrompt0 := false
	for _, e := range after {
		if e.Text == "prompt-overflow" {
			hasOverflow = true
		}
		if e.Text == "prompt-0" {
			hasPrompt0 = true
		}
	}
	if !hasOverflow {
		t.Error("prompt-overflow was evicted instead of the oldest entry")
	}
	if hasPrompt0 {
		t.Error("prompt-0 should have been evicted as the oldest entry")
	}
}

func TestRecordUsageUpsert(t *testing.T) {
	dir := t.TempDir()
	store := NewPromptStore(filepath.Join(dir, "prompts.json"))

	e1, err := store.RecordUsage("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if e1.UsedCount != 1 {
		t.Fatalf("expected UsedCount=1, got %d", e1.UsedCount)
	}

	e2, err := store.RecordUsage("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if e2.UsedCount != 2 {
		t.Fatalf("expected UsedCount=2, got %d", e2.UsedCount)
	}
	if e1.ID != e2.ID {
		t.Fatalf("same text must produce same ID: %q vs %q", e1.ID, e2.ID)
	}

	entries, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 unique entry, got %d", len(entries))
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	store := NewPromptStore(path)

	_, _ = store.RecordUsage("alpha")
	_, _ = store.RecordUsage("beta")

	// New store instance reading from same file.
	store2 := NewPromptStore(path)
	entries, err := store2.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after reload, got %d", len(entries))
	}
}

func TestLoadMissingFile(t *testing.T) {
	store := NewPromptStore(filepath.Join(t.TempDir(), "nonexistent.json"))
	entries, err := store.Load()
	if err != nil {
		t.Fatalf("Load on missing file should not error, got: %v", err)
	}
	if entries == nil {
		t.Fatal("Load on missing file must return non-nil slice")
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompts.json")
	store := NewPromptStore(path)

	_, err := store.RecordUsage("atomic test")
	if err != nil {
		t.Fatal(err)
	}

	// No .tmp files should remain after a successful write.
	matches, _ := filepath.Glob(filepath.Join(dir, ".prompts-*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("temp files leaked: %v", matches)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("final file not created: %v", err)
	}
}
