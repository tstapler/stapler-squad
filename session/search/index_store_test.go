package search

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/config"
)

func setupTestIndexStore(t *testing.T) (*IndexStore, string) {
	t.Helper()

	tmpDir := t.TempDir()
	indexDir := filepath.Join(tmpDir, "search_index")

	store, err := NewIndexStoreWithDir(indexDir)
	if err != nil {
		t.Fatalf("Failed to create index store: %v", err)
	}

	return store, indexDir
}

func createTestData() (*InvertedIndex, *DocumentStore) {
	idx := NewInvertedIndex()
	docStore := NewDocumentStore()

	// Add some test documents
	doc1 := &Document{
		SessionID:    "session-1",
		MessageIndex: 0,
		MessageRole:  "user",
		Content:      "hello world",
		WordCount:    2,
		Timestamp:    time.Now(),
	}
	docID1 := docStore.Add(doc1)
	idx.AddDocumentSimple(docID1, []string{"hello", "world"})

	doc2 := &Document{
		SessionID:    "session-1",
		MessageIndex: 1,
		MessageRole:  "assistant",
		Content:      "world peace",
		WordCount:    2,
		Timestamp:    time.Now(),
	}
	docID2 := docStore.Add(doc2)
	idx.AddDocumentSimple(docID2, []string{"world", "peace"})

	return idx, docStore
}

func TestNewIndexStore(t *testing.T) {
	t.Parallel()
	store, indexDir := setupTestIndexStore(t)

	if store == nil {
		t.Fatal("NewIndexStoreWithDir returned nil")
	}

	if store.GetIndexDir() != indexDir {
		t.Errorf("GetIndexDir() = %s, want %s", store.GetIndexDir(), indexDir)
	}

	// Verify directory was created
	if _, err := os.Stat(indexDir); os.IsNotExist(err) {
		t.Error("Index directory was not created")
	}
}

// TestNewIndexStore_TestMode_UsesIsolatedDir_NotRealHomeDir guards the CI
// timeout fix: NewIndexStore() (called by every NewSessionService in the
// server/services test suite) used to always persist to the real
// ~/.claude/search_index regardless of test mode. On an active dev machine
// that file accumulates to tens of thousands of documents over the
// developer's real usage; decoding it under -race on every NewSessionService
// call in a full package test run was slow enough to blow CI's 150s budget
// (unrelated to whatever the individual test was actually exercising). Under
// config.IsTestMode() (true for any go test binary), it must use
// config.GetConfigDir()'s isolated test directory instead.
func TestNewIndexStore_TestMode_UsesIsolatedDir_NotRealHomeDir(t *testing.T) {
	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

	store, err := NewIndexStore()
	if err != nil {
		t.Fatalf("NewIndexStore() error = %v", err)
	}

	want := filepath.Join(testDir, "search_index")
	if got := store.GetIndexDir(); got != want {
		t.Errorf("NewIndexStore().GetIndexDir() = %q, want %q (isolated STAPLER_SQUAD_TEST_DIR path)", got, want)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	realDir := filepath.Join(home, ".claude", "search_index")
	if store.GetIndexDir() == realDir {
		t.Error("NewIndexStore() used the real ~/.claude/search_index while config.IsTestMode() is true")
	}
}

func TestNewIndexStore_TestMode_True(t *testing.T) {
	t.Parallel()
	if !config.IsTestMode() {
		t.Fatal("config.IsTestMode() = false inside a go test binary; NewIndexStore's test-mode isolation would silently not apply")
	}
}

func TestIndexStore_SaveAndLoad(t *testing.T) {
	t.Parallel()
	store, _ := setupTestIndexStore(t)
	idx, docStore := createTestData()

	// Save
	err := store.Save(idx, docStore)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify files exist
	if !store.Exists() {
		t.Error("Exists() = false after Save()")
	}

	// Load
	loadedIdx, loadedDocStore, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded index
	if loadedIdx.TotalDocs != idx.TotalDocs {
		t.Errorf("Loaded TotalDocs = %d, want %d", loadedIdx.TotalDocs, idx.TotalDocs)
	}

	if loadedIdx.GetTermCount() != idx.GetTermCount() {
		t.Errorf("Loaded TermCount = %d, want %d", loadedIdx.GetTermCount(), idx.GetTermCount())
	}

	// Verify search still works
	postings := loadedIdx.Search("world")
	if postings == nil {
		t.Fatal("Search('world') returned nil after load")
	}
	if len(postings.DocIDs) != 2 {
		t.Errorf("len(DocIDs) = %d, want 2", len(postings.DocIDs))
	}

	// Verify loaded document store
	if loadedDocStore.Count() != docStore.Count() {
		t.Errorf("Loaded doc count = %d, want %d", loadedDocStore.Count(), docStore.Count())
	}

	doc := loadedDocStore.Get(0)
	if doc == nil {
		t.Fatal("Get(0) returned nil after load")
	}
	if doc.Content != "hello world" {
		t.Errorf("Loaded doc content = %q, want %q", doc.Content, "hello world")
	}
}

func TestIndexStore_Exists(t *testing.T) {
	t.Parallel()
	store, _ := setupTestIndexStore(t)

	// Initially doesn't exist
	if store.Exists() {
		t.Error("Exists() = true before Save()")
	}

	// Save data
	idx, docStore := createTestData()
	if err := store.Save(idx, docStore); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Now exists
	if !store.Exists() {
		t.Error("Exists() = false after Save()")
	}
}

func TestIndexStore_GetVersion(t *testing.T) {
	t.Parallel()
	store, _ := setupTestIndexStore(t)
	idx, docStore := createTestData()

	if err := store.Save(idx, docStore); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	version, err := store.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}

	if version.Version != CurrentIndexVersion {
		t.Errorf("Version = %d, want %d", version.Version, CurrentIndexVersion)
	}
	if version.DocumentCount != 2 {
		t.Errorf("DocumentCount = %d, want 2", version.DocumentCount)
	}
	if version.TermCount != 3 {
		t.Errorf("TermCount = %d, want 3", version.TermCount)
	}
}

func TestIndexStore_Delete(t *testing.T) {
	t.Parallel()
	store, _ := setupTestIndexStore(t)
	idx, docStore := createTestData()

	if err := store.Save(idx, docStore); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if !store.Exists() {
		t.Error("Exists() = false after Save()")
	}

	// Delete
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if store.Exists() {
		t.Error("Exists() = true after Delete()")
	}
}

func TestIndexStore_LoadNonExistent(t *testing.T) {
	t.Parallel()
	store, _ := setupTestIndexStore(t)

	_, _, err := store.Load()
	if err == nil {
		t.Error("Load() should fail when index doesn't exist")
	}
}

func TestIndexStore_AtomicWrites(t *testing.T) {
	t.Parallel()
	store, indexDir := setupTestIndexStore(t)
	idx, docStore := createTestData()

	if err := store.Save(idx, docStore); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify no temp files remain
	files, err := os.ReadDir(indexDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".tmp" {
			t.Errorf("Temp file remaining: %s", file.Name())
		}
	}
}

func TestIndexStore_MultipleSaves(t *testing.T) {
	t.Parallel()
	store, _ := setupTestIndexStore(t)
	idx, docStore := createTestData()

	// First save
	if err := store.Save(idx, docStore); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// Add more data
	doc3 := &Document{
		SessionID:    "session-2",
		MessageIndex: 0,
		MessageRole:  "user",
		Content:      "new document",
		WordCount:    2,
		Timestamp:    time.Now(),
	}
	docID3 := docStore.Add(doc3)
	idx.AddDocumentSimple(docID3, []string{"new", "document"})

	// Second save
	if err := store.Save(idx, docStore); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	// Load and verify
	loadedIdx, loadedDocStore, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loadedIdx.TotalDocs != 3 {
		t.Errorf("TotalDocs = %d, want 3", loadedIdx.TotalDocs)
	}
	if loadedDocStore.Count() != 3 {
		t.Errorf("Doc count = %d, want 3", loadedDocStore.Count())
	}
}

// Test document store separately
func TestDocumentStore_Basic(t *testing.T) {
	t.Parallel()
	ds := NewDocumentStore()

	doc := &Document{
		SessionID:    "session-1",
		MessageIndex: 0,
		MessageRole:  "user",
		Content:      "test content",
		WordCount:    2,
		Timestamp:    time.Now(),
	}

	docID := ds.Add(doc)
	if docID != 0 {
		t.Errorf("First docID = %d, want 0", docID)
	}

	// Get
	retrieved := ds.Get(docID)
	if retrieved == nil {
		t.Fatal("Get returned nil")
	}
	if retrieved.Content != "test content" {
		t.Errorf("Content = %q, want %q", retrieved.Content, "test content")
	}

	// Count
	if ds.Count() != 1 {
		t.Errorf("Count = %d, want 1", ds.Count())
	}
}

func TestDocumentStore_GetBySession(t *testing.T) {
	t.Parallel()
	ds := NewDocumentStore()

	// Add documents to session-1
	ds.Add(&Document{SessionID: "session-1", Content: "doc1"})
	ds.Add(&Document{SessionID: "session-1", Content: "doc2"})

	// Add document to session-2
	ds.Add(&Document{SessionID: "session-2", Content: "doc3"})

	// Get by session
	session1Docs := ds.GetBySession("session-1")
	if len(session1Docs) != 2 {
		t.Errorf("len(session1Docs) = %d, want 2", len(session1Docs))
	}

	session2Docs := ds.GetBySession("session-2")
	if len(session2Docs) != 1 {
		t.Errorf("len(session2Docs) = %d, want 1", len(session2Docs))
	}
}

func TestDocumentStore_RemoveBySession(t *testing.T) {
	t.Parallel()
	ds := NewDocumentStore()

	ds.Add(&Document{SessionID: "session-1", Content: "doc1"})
	ds.Add(&Document{SessionID: "session-1", Content: "doc2"})
	ds.Add(&Document{SessionID: "session-2", Content: "doc3"})

	ds.RemoveBySession("session-1")

	if ds.Count() != 1 {
		t.Errorf("Count = %d, want 1 after removal", ds.Count())
	}
	if ds.HasSession("session-1") {
		t.Error("HasSession('session-1') = true after removal")
	}
	if !ds.HasSession("session-2") {
		t.Error("HasSession('session-2') = false")
	}
}

func TestDocumentStore_Clear(t *testing.T) {
	t.Parallel()
	ds := NewDocumentStore()

	ds.Add(&Document{SessionID: "session-1", Content: "doc1"})
	ds.Add(&Document{SessionID: "session-2", Content: "doc2"})

	ds.Clear()

	if ds.Count() != 0 {
		t.Errorf("Count = %d, want 0 after Clear()", ds.Count())
	}
	if ds.SessionCount() != 0 {
		t.Errorf("SessionCount = %d, want 0 after Clear()", ds.SessionCount())
	}
}

func BenchmarkIndexStore_Save(b *testing.B) {
	tmpDir := b.TempDir()
	store, _ := NewIndexStoreWithDir(filepath.Join(tmpDir, "bench"))

	idx := NewInvertedIndex()
	docStore := NewDocumentStore()

	// Create test data
	for i := 0; i < 1000; i++ {
		doc := &Document{
			SessionID: "session-1",
			Content:   "test document content",
			WordCount: 3,
		}
		docID := docStore.Add(doc)
		idx.AddDocumentSimple(docID, []string{"test", "document", "content"})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Save(idx, docStore)
	}
}

func BenchmarkIndexStore_Load(b *testing.B) {
	tmpDir := b.TempDir()
	store, _ := NewIndexStoreWithDir(filepath.Join(tmpDir, "bench"))

	idx := NewInvertedIndex()
	docStore := NewDocumentStore()

	// Create and save test data
	for i := 0; i < 1000; i++ {
		doc := &Document{
			SessionID: "session-1",
			Content:   "test document content",
			WordCount: 3,
		}
		docID := docStore.Add(doc)
		idx.AddDocumentSimple(docID, []string{"test", "document", "content"})
	}
	store.Save(idx, docStore)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.Load()
	}
}

// TestIndexStore_Load_PostingsAllocs guards PerfFix-2: PostingsList.Positions
// moved from [][]int32 (one allocation per document per term) to a flat []int32
// plus PosOffsets (one allocation per term). Loading N docs sharing a small term
// vocabulary should allocate roughly O(terms), not O(terms x docs) — regressing
// back to a nested slice would show up here as allocs scaling with document count.
//
// This measures decoding just the inverted-index gob file (via the unexported
// loadGob), not the full IndexStore.Load(), which also decodes a
// map[int32]*Document with one *Document per document — an allocation that is
// inherently O(docs) and has nothing to do with PostingsList's shape. Folding
// that into the measurement made the threshold unsatisfiable regardless of how
// PostingsList is stored.
//
// This is a Test, not a Benchmark, deliberately: it calls testing.Benchmark()
// internally to get an AllocsPerOp() reading, and nesting that inside a function
// go test's own -bench harness invokes causes a hang. The outer harness
// recalibrates N by assuming per-call elapsed time scales with N, but this
// function's real cost (build docs, Save, run the nested benchmark) doesn't
// scale with the outer b.N at all — so measured duration stays flat between
// rounds and the harness keeps re-running with an exponentially larger target N
// (1, 10, 100, ... up to its 1e9 cap), each round repeating the ~1s nested
// testing.Benchmark() call. Calling testing.Benchmark() from a Test avoids this
// since Tests run exactly once, with no calibration loop.
func TestIndexStore_Load_PostingsAllocs(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewIndexStoreWithDir(filepath.Join(tmpDir, "bench-allocs"))

	idx := NewInvertedIndex()
	docStore := NewDocumentStore()

	const numDocs = 1000
	for i := 0; i < numDocs; i++ {
		doc := &Document{
			SessionID: "session-1",
			Content:   "test document content",
			WordCount: 3,
		}
		docID := docStore.Add(doc)
		idx.AddDocumentSimple(docID, []string{"test", "document", "content"})
	}
	if err := store.Save(idx, docStore); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	allocsPerOp := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var loaded InvertedIndex
			if err := store.loadGob(invertedIndexFile, &loaded); err != nil {
				b.Fatalf("loadGob(inverted index) failed: %v", err)
			}
		}
	}).AllocsPerOp()

	// InvertedIndex also carries DocLengths (map[int32]int, one entry per
	// document — legitimately O(docs), map bucket growth for 1000 entries
	// measured ~265 allocs/op here). The bound below is well below what an
	// O(terms x docs) PostingsList regression would produce (3 terms x 1000
	// docs = thousands of allocs) while leaving headroom for that map growth.
	const maxAllocsPerOp = 400
	if allocsPerOp > maxAllocsPerOp {
		t.Fatalf("loading inverted index allocated %d allocs/op, want <= %d (regressed to per-document allocation?)",
			allocsPerOp, maxAllocsPerOp)
	}
}
