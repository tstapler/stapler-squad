package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestIndexStore_SaveAndLoad(t *testing.T) {
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
	store, _ := setupTestIndexStore(t)

	_, _, err := store.Load()
	if err == nil {
		t.Error("Load() should fail when index doesn't exist")
	}
}

func TestIndexStore_AtomicWrites(t *testing.T) {
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
	for i := 0; i < b.N; i++ {
		store.Load()
	}
}
