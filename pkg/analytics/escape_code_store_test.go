package analytics

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestNewEscapeCodeStore(t *testing.T) {
	store := NewEscapeCodeStore()
	if store == nil {
		t.Fatal("Expected new store, got nil")
	}
	if store.IsEnabled() {
		t.Error("Store should be disabled by default")
	}
	if len(store.GetAll()) != 0 {
		t.Error("New store should be empty")
	}
	if store.maxEntries != 10000 {
		t.Errorf("Expected maxEntries 10000, got %d", store.maxEntries)
	}
}

func TestStoreEnableDisable(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	if !store.IsEnabled() {
		t.Error("Store should be enabled")
	}
	store.SetEnabled(false)
	if store.IsEnabled() {
		t.Error("Store should be disabled")
	}
}

func TestRecordBasic(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)

	sessionID := "sess-1"
	rawBytes := []byte{0x1b, '[', 'A'}
	category := CategoryCursor
	desc := "Cursor Up"

	store.Record(sessionID, rawBytes, category, desc)

	entries := store.GetAll()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.HumanReadable != desc {
		t.Errorf("Expected desc %s, got %s", desc, entry.HumanReadable)
	}
	if entry.Category != category {
		t.Errorf("Expected category %s, got %s", category, entry.Category)
	}
	if entry.Count != 1 {
		t.Errorf("Expected count 1, got %d", entry.Count)
	}
	if len(entry.SessionIDs) != 1 || entry.SessionIDs[0] != sessionID {
		t.Errorf("Expected session ID %s, got %v", sessionID, entry.SessionIDs)
	}
}

func TestRecordDisabled(t *testing.T) {
	store := NewEscapeCodeStore()
	// not enabled

	store.Record("sess-1", []byte{0x1b, '[', 'A'}, CategoryCursor, "Cursor Up")
	if len(store.GetAll()) != 0 {
		t.Error("Record should do nothing when disabled")
	}
}

func TestRecordUpdateExisting(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)

	rawBytes := []byte{0x1b, '[', 'A'}
	store.Record("sess-1", rawBytes, CategoryCursor, "Cursor Up")
	store.Record("sess-2", rawBytes, CategoryCursor, "Cursor Up")
	store.Record("sess-1", rawBytes, CategoryCursor, "Cursor Up") // duplicate session

	entries := store.GetAll()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Count != 3 {
		t.Errorf("Expected count 3, got %d", entry.Count)
	}
	if len(entry.SessionIDs) != 2 {
		t.Errorf("Expected 2 unique sessions, got %d", len(entry.SessionIDs))
	}
}

func TestSessionIDLimit(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	rawBytes := []byte{0x1b, '[', 'A'}

	for i := 0; i < 150; i++ {
		// generate 150 unique session IDs
		var sessID [3]byte
		sessID[0] = byte(i)
		store.Record(string(sessID[:]), rawBytes, CategoryCursor, "Cursor Up")
	}

	entries := store.GetAll()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if len(entries[0].SessionIDs) != 100 {
		t.Errorf("Expected max 100 sessions, got %d", len(entries[0].SessionIDs))
	}
}

func TestEviction(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	store.maxEntries = 10 // small limit for testing

	// Add 10 entries
	for i := 0; i < 10; i++ {
		store.Record("sess-1", []byte{byte(i)}, CategorySimple, "Test")
	}

	if len(store.GetAll()) != 10 {
		t.Fatalf("Expected 10 entries, got %d", len(store.GetAll()))
	}

	// Add 1 more entry to trigger eviction
	store.Record("sess-1", []byte{10}, CategorySimple, "Test")

	// Store removes 10% of entries (which is 1 out of 10 in this case)
	// Plus the 1 newly added entry, total should be 10 again.
	entries := store.GetAll()
	if len(entries) != 10 {
		t.Fatalf("Expected 10 entries after eviction, got %d", len(entries))
	}
}

func TestGetters(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)

	store.Record("sess-1", []byte{1}, CategoryCursor, "C1")
	store.Record("sess-1", []byte{2}, CategoryCursor, "C2")
	store.Record("sess-1", []byte{2}, CategoryCursor, "C2")
	store.Record("sess-2", []byte{3}, CategorySGR, "S1")

	// Test GetBySession
	sess1Entries := store.GetBySession("sess-1")
	if len(sess1Entries) != 2 {
		t.Errorf("Expected 2 entries for sess-1, got %d", len(sess1Entries))
	}

	sess2Entries := store.GetBySession("sess-2")
	if len(sess2Entries) != 1 {
		t.Errorf("Expected 1 entry for sess-2, got %d", len(sess2Entries))
	}

	// Test GetByCategory
	cursorEntries := store.GetByCategory(CategoryCursor)
	if len(cursorEntries) != 2 {
		t.Errorf("Expected 2 entries for CategoryCursor, got %d", len(cursorEntries))
	}

	sgrEntries := store.GetByCategory(CategorySGR)
	if len(sgrEntries) != 1 {
		t.Errorf("Expected 1 entry for CategorySGR, got %d", len(sgrEntries))
	}

	// Test sorting (GetByCategory returns sorted by count desc)
	if cursorEntries[0].Count != 2 {
		t.Errorf("Expected highest count first, got %d", cursorEntries[0].Count)
	}
}

func TestGetStats(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)

	store.Record("sess-1", []byte{1}, CategoryCursor, "C1")
	store.Record("sess-1", []byte{2}, CategoryCursor, "C2")
	store.Record("sess-1", []byte{2}, CategoryCursor, "C2")
	store.Record("sess-2", []byte{3}, CategorySGR, "S1")

	stats := store.GetStats()

	if !stats.Enabled {
		t.Error("Expected stats to show enabled")
	}
	if stats.TotalCodes != 4 {
		t.Errorf("Expected 4 total codes, got %d", stats.TotalCodes)
	}
	if stats.UniqueCodes != 3 {
		t.Errorf("Expected 3 unique codes, got %d", stats.UniqueCodes)
	}
	if stats.CategoryCounts[CategoryCursor] != 3 {
		t.Errorf("Expected 3 cursor counts, got %d", stats.CategoryCounts[CategoryCursor])
	}
	if stats.CategoryCounts[CategorySGR] != 1 {
		t.Errorf("Expected 1 sgr count, got %d", stats.CategoryCounts[CategorySGR])
	}
	if len(stats.TopCodes) != 3 {
		t.Errorf("Expected 3 top codes, got %d", len(stats.TopCodes))
	}
	if len(stats.RecentCodes) != 3 {
		t.Errorf("Expected 3 recent codes, got %d", len(stats.RecentCodes))
	}
}

func TestClear(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	store.Record("sess-1", []byte{1}, CategoryCursor, "C1")

	if len(store.GetAll()) != 1 {
		t.Fatal("Expected 1 entry")
	}

	store.Clear()

	if len(store.GetAll()) != 0 {
		t.Error("Expected 0 entries after Clear")
	}
	if store.totalCount != 0 {
		t.Error("Expected total count 0 after Clear")
	}
}

func TestExport(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)
	store.Record("sess-1", []byte{1}, CategoryCursor, "C1")

	data, err := store.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse exported JSON: %v", err)
	}

	if parsed["enabled"] != true {
		t.Error("Expected enabled true in export")
	}
	if parsed["totalCount"].(float64) != 1 {
		t.Error("Expected totalCount 1 in export")
	}

	entries := parsed["entries"].([]interface{})
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry in export, got %d", len(entries))
	}
}

func TestGetGlobalStore(t *testing.T) {
	store1 := GetGlobalStore()
	store2 := GetGlobalStore()

	if store1 == nil {
		t.Fatal("Expected non-nil global store")
	}
	if store1 != store2 {
		t.Error("Expected GetGlobalStore to return the same instance")
	}
}

func TestThreadSafety(t *testing.T) {
	store := NewEscapeCodeStore()
	store.SetEnabled(true)

	var wg sync.WaitGroup
	numGoroutines := 10
	recordsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				// Record a mix of same and different codes
				store.Record("sess-1", []byte{byte(id), byte(j)}, CategoryCursor, "Test")
				store.Record("sess-1", []byte{1, 2, 3}, CategoryCursor, "Test") // Shared
			}
		}(i)
	}

	// Also have some goroutines reading
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = store.GetAll()
				_ = store.GetStats()
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// 10 goroutines * 100 unique + 1 shared = 1001 unique codes
	if store.totalCount != int64(numGoroutines*recordsPerGoroutine*2) {
		t.Errorf("Expected total count %d, got %d", numGoroutines*recordsPerGoroutine*2, store.totalCount)
	}
}
