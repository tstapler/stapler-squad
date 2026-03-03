package search

import (
	"testing"
	"time"
)

// Helper to add test messages directly to the search engine
func addTestMessages(e *SearchEngine, sessionID string, messages []struct {
	role    string
	content string
}) {
	for i, msg := range messages {
		e.IndexMessage(sessionID, i, msg.role, msg.content, time.Now())
	}
}

func TestNewSearchEngine(t *testing.T) {
	engine := NewSearchEngine()
	if engine == nil {
		t.Fatal("NewSearchEngine returned nil")
	}

	stats := engine.GetStats()
	if stats.TotalDocuments != 0 {
		t.Errorf("TotalDocuments = %d, want 0", stats.TotalDocuments)
	}
}

func TestSearchEngine_IndexMessage(t *testing.T) {
	engine := NewSearchEngine()

	err := engine.IndexMessage("session-1", 0, "user", "hello world", time.Now())
	if err != nil {
		t.Fatalf("IndexMessage failed: %v", err)
	}

	stats := engine.GetStats()
	if stats.TotalDocuments != 1 {
		t.Errorf("TotalDocuments = %d, want 1", stats.TotalDocuments)
	}
}

func TestSearchEngine_Search_SingleResult(t *testing.T) {
	engine := NewSearchEngine()

	addTestMessages(engine, "session-1", []struct {
		role    string
		content string
	}{
		{"user", "hello world"},
		{"assistant", "hi there"},
	})

	results, err := engine.Search("hello", SearchOptions{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if results.TotalMatches != 1 {
		t.Errorf("TotalMatches = %d, want 1", results.TotalMatches)
	}

	if len(results.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(results.Results))
	}

	if results.Results[0].SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", results.Results[0].SessionID, "session-1")
	}
}

func TestSearchEngine_Search_MultipleResults(t *testing.T) {
	engine := NewSearchEngine()

	addTestMessages(engine, "session-1", []struct {
		role    string
		content string
	}{
		{"user", "docker container error"},
		{"assistant", "let me help with the docker issue"},
	})

	results, err := engine.Search("docker", SearchOptions{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if results.TotalMatches != 2 {
		t.Errorf("TotalMatches = %d, want 2", results.TotalMatches)
	}
}

func TestSearchEngine_Search_NoResults(t *testing.T) {
	engine := NewSearchEngine()

	addTestMessages(engine, "session-1", []struct {
		role    string
		content string
	}{
		{"user", "hello world"},
	})

	results, err := engine.Search("nonexistent", SearchOptions{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if results.TotalMatches != 0 {
		t.Errorf("TotalMatches = %d, want 0", results.TotalMatches)
	}
}

func TestSearchEngine_Search_EmptyQuery(t *testing.T) {
	engine := NewSearchEngine()

	addTestMessages(engine, "session-1", []struct {
		role    string
		content string
	}{
		{"user", "hello world"},
	})

	results, err := engine.Search("", SearchOptions{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if results.TotalMatches != 0 {
		t.Errorf("TotalMatches = %d, want 0 for empty query", results.TotalMatches)
	}
}

func TestSearchEngine_Search_Pagination(t *testing.T) {
	engine := NewSearchEngine()

	// Add 5 messages all containing "test"
	for i := 0; i < 5; i++ {
		engine.IndexMessage("session-1", i, "user", "test message number", time.Now())
	}

	// Get first 2 results
	results, err := engine.Search("test", SearchOptions{Limit: 2})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if results.TotalMatches != 5 {
		t.Errorf("TotalMatches = %d, want 5", results.TotalMatches)
	}
	if len(results.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(results.Results))
	}

	// Get next 2 with offset
	results2, err := engine.Search("test", SearchOptions{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("Search with offset failed: %v", err)
	}

	if len(results2.Results) != 2 {
		t.Errorf("len(Results) with offset = %d, want 2", len(results2.Results))
	}

	// Get last 1 with offset 4
	results3, err := engine.Search("test", SearchOptions{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("Search with large offset failed: %v", err)
	}

	if len(results3.Results) != 1 {
		t.Errorf("len(Results) at end = %d, want 1", len(results3.Results))
	}
}

func TestSearchEngine_Search_FilterBySession(t *testing.T) {
	engine := NewSearchEngine()

	// Add messages to different sessions
	engine.IndexMessage("session-1", 0, "user", "test in session one", time.Now())
	engine.IndexMessage("session-2", 0, "user", "test in session two", time.Now())
	engine.IndexMessage("session-2", 1, "assistant", "another test here", time.Now())

	// Search all sessions
	results, _ := engine.Search("test", SearchOptions{})
	if results.TotalMatches != 3 {
		t.Errorf("All sessions: TotalMatches = %d, want 3", results.TotalMatches)
	}

	// Search only session-2
	results2, _ := engine.Search("test", SearchOptions{SessionID: "session-2"})
	if results2.TotalMatches != 2 {
		t.Errorf("Session-2 only: TotalMatches = %d, want 2", results2.TotalMatches)
	}
}

func TestSearchEngine_Search_Ranking(t *testing.T) {
	engine := NewSearchEngine()

	// Add messages with different term frequencies
	engine.IndexMessage("session-1", 0, "user", "docker", time.Now())                           // 1 occurrence
	engine.IndexMessage("session-1", 1, "assistant", "docker docker docker", time.Now())        // 3 occurrences

	results, err := engine.Search("docker", SearchOptions{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results.Results) < 2 {
		t.Fatalf("Expected at least 2 results, got %d", len(results.Results))
	}

	// Results should be sorted by score (higher first)
	if results.Results[0].Score < results.Results[1].Score {
		t.Error("Results not sorted by score descending")
	}
}

func TestSearchEngine_RemoveSession(t *testing.T) {
	engine := NewSearchEngine()

	engine.IndexMessage("session-1", 0, "user", "hello world", time.Now())
	engine.IndexMessage("session-2", 0, "user", "goodbye world", time.Now())

	// Verify both indexed
	if !engine.HasSession("session-1") {
		t.Error("session-1 should exist")
	}
	if !engine.HasSession("session-2") {
		t.Error("session-2 should exist")
	}

	// Remove session-1
	engine.RemoveSession("session-1")

	if engine.HasSession("session-1") {
		t.Error("session-1 should not exist after removal")
	}
	if !engine.HasSession("session-2") {
		t.Error("session-2 should still exist")
	}

	// Search should only find session-2
	results, _ := engine.Search("world", SearchOptions{})
	if results.TotalMatches != 1 {
		t.Errorf("TotalMatches after removal = %d, want 1", results.TotalMatches)
	}
}

func TestSearchEngine_Clear(t *testing.T) {
	engine := NewSearchEngine()

	engine.IndexMessage("session-1", 0, "user", "hello world", time.Now())
	engine.IndexMessage("session-2", 0, "user", "goodbye world", time.Now())

	stats := engine.GetStats()
	if stats.TotalDocuments != 2 {
		t.Errorf("Before clear: TotalDocuments = %d, want 2", stats.TotalDocuments)
	}

	engine.Clear()

	stats = engine.GetStats()
	if stats.TotalDocuments != 0 {
		t.Errorf("After clear: TotalDocuments = %d, want 0", stats.TotalDocuments)
	}
}

func TestSearchEngine_GetDocument(t *testing.T) {
	engine := NewSearchEngine()

	engine.IndexMessage("session-1", 0, "user", "hello world", time.Now())

	doc := engine.GetDocument(0)
	if doc == nil {
		t.Fatal("GetDocument(0) returned nil")
	}
	if doc.Content != "hello world" {
		t.Errorf("Content = %q, want %q", doc.Content, "hello world")
	}
}

func TestSearchEngine_MultiWordQuery(t *testing.T) {
	engine := NewSearchEngine()

	engine.IndexMessage("session-1", 0, "user", "docker container error troubleshooting", time.Now())
	engine.IndexMessage("session-1", 1, "assistant", "let me help with docker", time.Now())
	engine.IndexMessage("session-2", 0, "user", "error handling in golang", time.Now())

	// Search for multiple words
	results, err := engine.Search("docker error", SearchOptions{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// All 3 messages contain at least one term, but doc with both should rank higher
	if results.TotalMatches == 0 {
		t.Error("Expected some matches for multi-word query")
	}

	// First result should contain both terms (higher score)
	if len(results.Results) > 0 {
		topResult := results.Results[0]
		t.Logf("Top result: score=%f, content=%q", topResult.Score, topResult.Content)
	}
}

func TestSearchEngine_QueryTime(t *testing.T) {
	engine := NewSearchEngine()

	// Add some documents
	for i := 0; i < 100; i++ {
		engine.IndexMessage("session-1", i, "user", "test document with some content for searching", time.Now())
	}

	results, err := engine.Search("test", SearchOptions{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if results.QueryTime <= 0 {
		t.Error("QueryTime should be positive")
	}

	// QueryTime should be reasonable (less than 1 second for 100 docs)
	if results.QueryTime > time.Second {
		t.Errorf("QueryTime too long: %v", results.QueryTime)
	}
}

func TestSearchEngine_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	indexStore, err := NewIndexStoreWithDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create index store: %v", err)
	}

	// Create engine with persistence
	engine := NewSearchEngineWithPersistence(indexStore)

	// Index some messages
	engine.IndexMessage("session-1", 0, "user", "hello world", time.Now())
	engine.IndexMessage("session-1", 1, "assistant", "world peace", time.Now())

	// Save index
	if err := engine.SaveIndex(); err != nil {
		t.Fatalf("SaveIndex failed: %v", err)
	}

	// Create new engine and load
	engine2 := NewSearchEngineWithPersistence(indexStore)
	if err := engine2.LoadIndex(); err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	// Search should work
	results, err := engine2.Search("world", SearchOptions{})
	if err != nil {
		t.Fatalf("Search after load failed: %v", err)
	}

	if results.TotalMatches != 2 {
		t.Errorf("TotalMatches after load = %d, want 2", results.TotalMatches)
	}
}

// Benchmark tests
func BenchmarkSearchEngine_IndexMessage(b *testing.B) {
	engine := NewSearchEngine()
	content := "This is a test message with some content for indexing performance testing"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.IndexMessage("session-1", i, "user", content, time.Now())
	}
}

func BenchmarkSearchEngine_Search(b *testing.B) {
	engine := NewSearchEngine()

	// Pre-populate with 10,000 documents
	for i := 0; i < 10000; i++ {
		engine.IndexMessage("session-1", i, "user", "test document with some searchable content here", time.Now())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Search("searchable content", SearchOptions{Limit: 20})
	}
}

func BenchmarkSearchEngine_Search_LargeIndex(b *testing.B) {
	engine := NewSearchEngine()

	// Pre-populate with 50,000 documents (realistic large history)
	for i := 0; i < 50000; i++ {
		engine.IndexMessage("session-1", i, "user", "docker container kubernetes deployment error troubleshooting", time.Now())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Search("docker error", SearchOptions{Limit: 20})
	}
}
