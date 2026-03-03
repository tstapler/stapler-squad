package search

import (
	"sync"
	"testing"
)

func TestNewInvertedIndex(t *testing.T) {
	idx := NewInvertedIndex()
	if idx == nil {
		t.Fatal("NewInvertedIndex returned nil")
	}
	if idx.Index == nil {
		t.Fatal("Index map is nil")
	}
	if idx.DocFrequency == nil {
		t.Fatal("DocFrequency map is nil")
	}
	if idx.DocLengths == nil {
		t.Fatal("DocLengths map is nil")
	}
	if idx.TotalDocs != 0 {
		t.Errorf("TotalDocs = %d, want 0", idx.TotalDocs)
	}
	if idx.AvgDocLength != 0 {
		t.Errorf("AvgDocLength = %f, want 0", idx.AvgDocLength)
	}
}

func TestAddDocument_SingleDocument(t *testing.T) {
	idx := NewInvertedIndex()

	tokens := []string{"hello", "world"}
	positions := map[string][]int32{
		"hello": {0},
		"world": {1},
	}

	idx.AddDocument(1, tokens, positions)

	// Check document count
	if idx.TotalDocs != 1 {
		t.Errorf("TotalDocs = %d, want 1", idx.TotalDocs)
	}

	// Check document length
	if idx.DocLengths[1] != 2 {
		t.Errorf("DocLengths[1] = %d, want 2", idx.DocLengths[1])
	}

	// Check average doc length
	if idx.AvgDocLength != 2.0 {
		t.Errorf("AvgDocLength = %f, want 2.0", idx.AvgDocLength)
	}

	// Check posting list for "hello"
	helloPostings := idx.Search("hello")
	if helloPostings == nil {
		t.Fatal("Search('hello') returned nil")
	}
	if len(helloPostings.DocIDs) != 1 || helloPostings.DocIDs[0] != 1 {
		t.Errorf("DocIDs = %v, want [1]", helloPostings.DocIDs)
	}
	if len(helloPostings.Frequency) != 1 || helloPostings.Frequency[0] != 1 {
		t.Errorf("Frequency = %v, want [1]", helloPostings.Frequency)
	}

	// Check document frequency
	if idx.DocFrequency["hello"] != 1 {
		t.Errorf("DocFrequency['hello'] = %d, want 1", idx.DocFrequency["hello"])
	}
}

func TestAddDocument_MultipleDocuments(t *testing.T) {
	idx := NewInvertedIndex()

	// Add first document
	idx.AddDocumentSimple(1, []string{"hello", "world"})

	// Add second document with overlapping term
	idx.AddDocumentSimple(2, []string{"world", "peace"})

	// Check document count
	if idx.TotalDocs != 2 {
		t.Errorf("TotalDocs = %d, want 2", idx.TotalDocs)
	}

	// Check "world" appears in both documents
	worldPostings := idx.Search("world")
	if worldPostings == nil {
		t.Fatal("Search('world') returned nil")
	}
	if len(worldPostings.DocIDs) != 2 {
		t.Errorf("len(DocIDs) = %d, want 2", len(worldPostings.DocIDs))
	}

	// Check document frequencies
	if idx.DocFrequency["world"] != 2 {
		t.Errorf("DocFrequency['world'] = %d, want 2", idx.DocFrequency["world"])
	}
	if idx.DocFrequency["hello"] != 1 {
		t.Errorf("DocFrequency['hello'] = %d, want 1", idx.DocFrequency["hello"])
	}
	if idx.DocFrequency["peace"] != 1 {
		t.Errorf("DocFrequency['peace'] = %d, want 1", idx.DocFrequency["peace"])
	}
}

func TestAddDocument_RepeatedTerms(t *testing.T) {
	idx := NewInvertedIndex()

	// Document with repeated term
	tokens := []string{"test", "test", "test"}
	idx.AddDocumentSimple(1, tokens)

	// Check term frequency
	postings := idx.Search("test")
	if postings == nil {
		t.Fatal("Search('test') returned nil")
	}
	if len(postings.Frequency) != 1 || postings.Frequency[0] != 3 {
		t.Errorf("Frequency = %v, want [3]", postings.Frequency)
	}

	// Document frequency should still be 1 (one document)
	if idx.DocFrequency["test"] != 1 {
		t.Errorf("DocFrequency['test'] = %d, want 1", idx.DocFrequency["test"])
	}
}

func TestSearch_NonExistentTerm(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello", "world"})

	postings := idx.Search("nonexistent")
	if postings != nil {
		t.Errorf("Search('nonexistent') = %v, want nil", postings)
	}
}

func TestSearchMultiple(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello", "world"})
	idx.AddDocumentSimple(2, []string{"world", "peace"})

	results := idx.SearchMultiple([]string{"hello", "world", "nonexistent"})

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}

	if results["hello"] == nil {
		t.Error("results['hello'] is nil")
	}
	if results["world"] == nil {
		t.Error("results['world'] is nil")
	}
	if results["nonexistent"] != nil {
		t.Error("results['nonexistent'] should be nil")
	}
}

func TestGetters(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello", "world", "test"})
	idx.AddDocumentSimple(2, []string{"world"})

	// Test GetTotalDocs
	if got := idx.GetTotalDocs(); got != 2 {
		t.Errorf("GetTotalDocs() = %d, want 2", got)
	}

	// Test GetDocLength
	if got := idx.GetDocLength(1); got != 3 {
		t.Errorf("GetDocLength(1) = %d, want 3", got)
	}
	if got := idx.GetDocLength(2); got != 1 {
		t.Errorf("GetDocLength(2) = %d, want 1", got)
	}

	// Test GetAvgDocLength
	expected := 2.0 // (3 + 1) / 2
	if got := idx.GetAvgDocLength(); got != expected {
		t.Errorf("GetAvgDocLength() = %f, want %f", got, expected)
	}

	// Test GetDocumentFrequency
	if got := idx.GetDocumentFrequency("world"); got != 2 {
		t.Errorf("GetDocumentFrequency('world') = %d, want 2", got)
	}
	if got := idx.GetDocumentFrequency("hello"); got != 1 {
		t.Errorf("GetDocumentFrequency('hello') = %d, want 1", got)
	}

	// Test GetTermCount
	if got := idx.GetTermCount(); got != 3 {
		t.Errorf("GetTermCount() = %d, want 3", got)
	}
}

func TestHasDocument(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello"})

	if !idx.HasDocument(1) {
		t.Error("HasDocument(1) = false, want true")
	}
	if idx.HasDocument(2) {
		t.Error("HasDocument(2) = true, want false")
	}
}

func TestRemoveDocument(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello", "world"})
	idx.AddDocumentSimple(2, []string{"world", "peace"})

	// Remove document 1
	idx.RemoveDocument(1)

	// Check document count
	if idx.TotalDocs != 1 {
		t.Errorf("TotalDocs = %d, want 1", idx.TotalDocs)
	}

	// Check "hello" is removed (no longer in any document)
	if idx.Search("hello") != nil {
		t.Error("Search('hello') should return nil after removal")
	}

	// Check "world" still exists in document 2
	worldPostings := idx.Search("world")
	if worldPostings == nil {
		t.Fatal("Search('world') returned nil")
	}
	if len(worldPostings.DocIDs) != 1 || worldPostings.DocIDs[0] != 2 {
		t.Errorf("DocIDs = %v, want [2]", worldPostings.DocIDs)
	}

	// Check document frequency updated
	if idx.DocFrequency["world"] != 1 {
		t.Errorf("DocFrequency['world'] = %d, want 1", idx.DocFrequency["world"])
	}
	if idx.DocFrequency["hello"] != 0 {
		t.Errorf("DocFrequency['hello'] = %d, want 0", idx.DocFrequency["hello"])
	}

	// Check HasDocument
	if idx.HasDocument(1) {
		t.Error("HasDocument(1) = true after removal, want false")
	}
	if !idx.HasDocument(2) {
		t.Error("HasDocument(2) = false, want true")
	}
}

func TestRemoveDocument_NonExistent(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello"})

	// Should not panic or error
	idx.RemoveDocument(999)

	// Document count unchanged
	if idx.TotalDocs != 1 {
		t.Errorf("TotalDocs = %d, want 1", idx.TotalDocs)
	}
}

func TestClear(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello", "world"})
	idx.AddDocumentSimple(2, []string{"world", "peace"})

	idx.Clear()

	if idx.TotalDocs != 0 {
		t.Errorf("TotalDocs = %d, want 0", idx.TotalDocs)
	}
	if idx.AvgDocLength != 0 {
		t.Errorf("AvgDocLength = %f, want 0", idx.AvgDocLength)
	}
	if len(idx.Index) != 0 {
		t.Errorf("len(Index) = %d, want 0", len(idx.Index))
	}
	if len(idx.DocFrequency) != 0 {
		t.Errorf("len(DocFrequency) = %d, want 0", len(idx.DocFrequency))
	}
	if len(idx.DocLengths) != 0 {
		t.Errorf("len(DocLengths) = %d, want 0", len(idx.DocLengths))
	}
}

func TestGetAllTerms(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello", "world"})
	idx.AddDocumentSimple(2, []string{"world", "peace"})

	terms := idx.GetAllTerms()
	if len(terms) != 3 {
		t.Errorf("len(terms) = %d, want 3", len(terms))
	}

	// Check all expected terms are present
	termSet := make(map[string]bool)
	for _, term := range terms {
		termSet[term] = true
	}
	for _, expected := range []string{"hello", "world", "peace"} {
		if !termSet[expected] {
			t.Errorf("term %q not found in GetAllTerms()", expected)
		}
	}
}

func TestGetStats(t *testing.T) {
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello", "world"})
	idx.AddDocumentSimple(2, []string{"world", "peace"})

	stats := idx.GetStats()

	if stats.TotalDocs != 2 {
		t.Errorf("TotalDocs = %d, want 2", stats.TotalDocs)
	}
	if stats.TotalTerms != 3 {
		t.Errorf("TotalTerms = %d, want 3", stats.TotalTerms)
	}
	// Total postings: hello(1) + world(2) + peace(1) = 4
	if stats.TotalPostings != 4 {
		t.Errorf("TotalPostings = %d, want 4", stats.TotalPostings)
	}
	if stats.AvgDocLength != 2.0 {
		t.Errorf("AvgDocLength = %f, want 2.0", stats.AvgDocLength)
	}
	// AvgTermsPerDoc = 4 / 2 = 2.0
	if stats.AvgTermsPerDoc != 2.0 {
		t.Errorf("AvgTermsPerDoc = %f, want 2.0", stats.AvgTermsPerDoc)
	}
}

func TestConcurrentAccess(t *testing.T) {
	idx := NewInvertedIndex()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(docID int32) {
			defer wg.Done()
			idx.AddDocumentSimple(docID, []string{"test", "concurrent"})
		}(int32(i))
	}
	wg.Wait()

	// Verify results
	if idx.TotalDocs != numGoroutines {
		t.Errorf("TotalDocs = %d, want %d", idx.TotalDocs, numGoroutines)
	}

	testPostings := idx.Search("test")
	if testPostings == nil {
		t.Fatal("Search('test') returned nil")
	}
	if len(testPostings.DocIDs) != numGoroutines {
		t.Errorf("len(DocIDs) = %d, want %d", len(testPostings.DocIDs), numGoroutines)
	}

	// Concurrent reads while searching
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx.Search("test")
			idx.GetTotalDocs()
			idx.GetAvgDocLength()
		}()
	}
	wg.Wait()
}

func TestPositionTracking(t *testing.T) {
	idx := NewInvertedIndex()

	tokens := []string{"the", "quick", "brown", "fox", "quick"}
	positions := map[string][]int32{
		"the":   {0},
		"quick": {1, 4}, // "quick" appears at positions 1 and 4
		"brown": {2},
		"fox":   {3},
	}

	idx.AddDocument(1, tokens, positions)

	// Check positions are stored correctly
	quickPostings := idx.Search("quick")
	if quickPostings == nil {
		t.Fatal("Search('quick') returned nil")
	}
	if len(quickPostings.Positions) != 1 {
		t.Fatalf("len(Positions) = %d, want 1", len(quickPostings.Positions))
	}
	if len(quickPostings.Positions[0]) != 2 {
		t.Errorf("len(Positions[0]) = %d, want 2", len(quickPostings.Positions[0]))
	}
	if quickPostings.Positions[0][0] != 1 || quickPostings.Positions[0][1] != 4 {
		t.Errorf("Positions[0] = %v, want [1, 4]", quickPostings.Positions[0])
	}
}

// Benchmark tests
func BenchmarkAddDocument(b *testing.B) {
	idx := NewInvertedIndex()
	tokens := []string{"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.AddDocumentSimple(int32(i), tokens)
	}
}

func BenchmarkSearch(b *testing.B) {
	idx := NewInvertedIndex()
	// Add 10,000 documents
	for i := 0; i < 10000; i++ {
		idx.AddDocumentSimple(int32(i), []string{"hello", "world", "test", "document"})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search("test")
	}
}

func BenchmarkSearchMultiple(b *testing.B) {
	idx := NewInvertedIndex()
	// Add 10,000 documents
	for i := 0; i < 10000; i++ {
		idx.AddDocumentSimple(int32(i), []string{"hello", "world", "test", "document"})
	}

	terms := []string{"hello", "world", "test"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.SearchMultiple(terms)
	}
}
