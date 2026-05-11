package scrollback

import (
	"fmt"
	"testing"
)

// newTestManager creates a ScrollbackManager backed by a temp directory
// so tests do not touch real persistent state.
func newTestManager(t *testing.T) *ScrollbackManager {
	t.Helper()
	cfg := DefaultScrollbackConfig()
	cfg.StoragePath = t.TempDir()
	// Use a very long flush interval so background flushes don't interfere.
	cfg.FlushInterval = 999999999
	m := NewScrollbackManager(cfg)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// appendN appends n entries with data "entry-<i>" to sessionID.
func appendN(t *testing.T, m *ScrollbackManager, sessionID string, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if err := m.AppendOutput(sessionID, []byte(fmt.Sprintf("entry-%d", i))); err != nil {
			t.Fatalf("AppendOutput(%d): %v", i, err)
		}
	}
}

// ---- TestGetScrollbackBefore ----

func TestGetScrollbackBefore_BeforeSeqZero_ReturnsEmpty(t *testing.T) {
	m := newTestManager(t)
	appendN(t, m, "sess1", 5)

	entries, err := m.GetScrollbackBefore("sess1", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for beforeSeq=0, got %d", len(entries))
	}
}

func TestGetScrollbackBefore_BeforeSeq1_ReturnsEmpty(t *testing.T) {
	// Only entry 1 exists; nothing is strictly before seq 1.
	m := newTestManager(t)
	appendN(t, m, "sess1", 1)

	entries, err := m.GetScrollbackBefore("sess1", 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for beforeSeq=1 with only one entry, got %d", len(entries))
	}
}

func TestGetScrollbackBefore_BeforeSeq5_Returns4Entries(t *testing.T) {
	// 10 entries (seq 1–10); beforeSeq=5 → entries 1–4.
	m := newTestManager(t)
	appendN(t, m, "sess1", 10)

	entries, err := m.GetScrollbackBefore("sess1", 5, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}
	// Entries should be in chronological order (seq 1 to 4).
	for i, e := range entries {
		expectedSeq := uint64(i + 1)
		if e.Sequence != expectedSeq {
			t.Errorf("entry[%d].Sequence: want %d, got %d", i, expectedSeq, e.Sequence)
		}
	}
	// No entry should be >= beforeSeq (5).
	for _, e := range entries {
		if e.Sequence >= 5 {
			t.Errorf("entry with seq %d should not appear (>= beforeSeq 5)", e.Sequence)
		}
	}
}

func TestGetScrollbackBefore_LimitIsRespected(t *testing.T) {
	// 10 entries (seq 1–10); beforeSeq=10, limit=3 → last 3 before seq 10 = entries 7,8,9.
	m := newTestManager(t)
	appendN(t, m, "sess1", 10)

	entries, err := m.GetScrollbackBefore("sess1", 10, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (limit), got %d", len(entries))
	}
	expectedSeqs := []uint64{7, 8, 9}
	for i, e := range entries {
		if e.Sequence != expectedSeqs[i] {
			t.Errorf("entry[%d].Sequence: want %d, got %d", i, expectedSeqs[i], e.Sequence)
		}
	}
}

func TestGetScrollbackBefore_BeforeSeqExceedsAll_ReturnsAllUpToLimit(t *testing.T) {
	// 5 entries; beforeSeq=100 (beyond all) with limit=10 → all 5 entries.
	m := newTestManager(t)
	appendN(t, m, "sess1", 5)

	entries, err := m.GetScrollbackBefore("sess1", 100, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
	// Chronological order: seq 1 → 5.
	for i, e := range entries {
		expectedSeq := uint64(i + 1)
		if e.Sequence != expectedSeq {
			t.Errorf("entry[%d].Sequence: want %d, got %d", i, expectedSeq, e.Sequence)
		}
	}
}

func TestGetScrollbackBefore_NoEntries_ReturnsEmpty(t *testing.T) {
	// Session has no scrollback at all.
	m := newTestManager(t)

	entries, err := m.GetScrollbackBefore("empty-session", 5, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty session, got %d", len(entries))
	}
}

func TestGetScrollbackBefore_LimitLargerThanAvailable(t *testing.T) {
	// 3 entries; beforeSeq=10, limit=50 → all 3 entries.
	m := newTestManager(t)
	appendN(t, m, "sess1", 3)

	entries, err := m.GetScrollbackBefore("sess1", 10, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (all), got %d", len(entries))
	}
}
