package search

import (
	"sync"
	"testing"
)

func TestNewInvertedIndex(t *testing.T) {
	t.Parallel()
	idx := NewInvertedIndex()
	if idx == nil {
		t.Fatal("NewInvertedIndex returned nil")
	}
	if idx.Index == nil {
		t.Fatal("Index map is nil")
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
	t.Parallel()
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
	if idx.GetDocumentFrequency("hello") != 1 {
		t.Errorf("DocFrequency['hello'] = %d, want 1", idx.GetDocumentFrequency("hello"))
	}
}

func TestAddDocument_MultipleDocuments(t *testing.T) {
	t.Parallel()
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
	if idx.GetDocumentFrequency("world") != 2 {
		t.Errorf("DocFrequency['world'] = %d, want 2", idx.GetDocumentFrequency("world"))
	}
	if idx.GetDocumentFrequency("hello") != 1 {
		t.Errorf("DocFrequency['hello'] = %d, want 1", idx.GetDocumentFrequency("hello"))
	}
	if idx.GetDocumentFrequency("peace") != 1 {
		t.Errorf("DocFrequency['peace'] = %d, want 1", idx.GetDocumentFrequency("peace"))
	}
}

func TestAddDocument_RepeatedTerms(t *testing.T) {
	t.Parallel()
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
	if idx.GetDocumentFrequency("test") != 1 {
		t.Errorf("DocFrequency['test'] = %d, want 1", idx.GetDocumentFrequency("test"))
	}
}

func TestSearch_NonExistentTerm(t *testing.T) {
	t.Parallel()
	idx := NewInvertedIndex()
	idx.AddDocumentSimple(1, []string{"hello", "world"})

	postings := idx.Search("nonexistent")
	if postings != nil {
		t.Errorf("Search('nonexistent') = %v, want nil", postings)
	}
}

func TestSearchMultiple(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	if idx.GetDocumentFrequency("world") != 1 {
		t.Errorf("DocFrequency['world'] = %d, want 1", idx.GetDocumentFrequency("world"))
	}
	if idx.GetDocumentFrequency("hello") != 0 {
		t.Errorf("DocFrequency['hello'] = %d, want 0", idx.GetDocumentFrequency("hello"))
	}

	// Check HasDocument
	if idx.HasDocument(1) {
		t.Error("HasDocument(1) = true after removal, want false")
	}
	if !idx.HasDocument(2) {
		t.Error("HasDocument(2) = false, want true")
	}
}

// TestRemoveDocument_MiddleDocument_RebasesCSROffsets is a regression test for
// the CSR splice in RemoveDocument (inverted_index.go ~lines 222-230): removing
// docIndex==0 (the first posting) needs no rebasing of trailing offsets and no
// shift of a buffer around a middle element, so it can't catch an off-by-one in
// either the Positions splice or the "subtract span from every later offset"
// loop. This removes the middle of three postings, each with 2+ distinct
// positions, and checks that the flat Positions buffer and PosOffsets for the
// surviving postings are exactly right, not just non-crashing.
func TestRemoveDocument_MiddleDocument_RebasesCSROffsets(t *testing.T) {
	t.Parallel()
	idx := NewInvertedIndex()

	idx.AddDocument(1, []string{"shared", "a"}, map[string][]int32{"shared": {0, 5}})
	idx.AddDocument(2, []string{"shared", "b"}, map[string][]int32{"shared": {1, 2, 3}})
	idx.AddDocument(3, []string{"shared", "c"}, map[string][]int32{"shared": {10, 20}})

	postings := idx.Search("shared")
	if postings == nil {
		t.Fatal("Search('shared') returned nil before removal")
	}
	// Sanity-check the pre-removal CSR layout matches what we expect to splice.
	wantPosOffsetsBefore := []int32{0, 2, 5, 7}
	if !equalInt32Slices(postings.PosOffsets, wantPosOffsetsBefore) {
		t.Fatalf("PosOffsets before removal = %v, want %v", postings.PosOffsets, wantPosOffsetsBefore)
	}

	// Remove the middle document (docID 2, docIndex 1).
	idx.RemoveDocument(2)

	postings = idx.Search("shared")
	if postings == nil {
		t.Fatal("Search('shared') returned nil after removal")
	}

	wantDocIDs := []int32{1, 3}
	if !equalInt32Slices(postings.DocIDs, wantDocIDs) {
		t.Fatalf("DocIDs = %v, want %v", postings.DocIDs, wantDocIDs)
	}
	wantFrequency := []int32{1, 1}
	if !equalInt32Slices(postings.Frequency, wantFrequency) {
		t.Fatalf("Frequency = %v, want %v", postings.Frequency, wantFrequency)
	}
	// DocIDs/Frequency must stay aligned with PosOffsets: len(PosOffsets) == len(DocIDs)+1.
	if len(postings.PosOffsets) != len(postings.DocIDs)+1 {
		t.Fatalf("len(PosOffsets) = %d, want len(DocIDs)+1 = %d", len(postings.PosOffsets), len(postings.DocIDs)+1)
	}
	wantPosOffsetsAfter := []int32{0, 2, 4}
	if !equalInt32Slices(postings.PosOffsets, wantPosOffsetsAfter) {
		t.Fatalf("PosOffsets after removal = %v, want %v", postings.PosOffsets, wantPosOffsetsAfter)
	}

	// The critical assertion: positions for the posting that was AFTER the
	// removed one (doc 3, now at index 1) must read back correctly out of the
	// shifted flat buffer, not some off-by-one slice of it.
	doc1Positions := postings.PositionsAt(0)
	if !equalInt32Slices(doc1Positions, []int32{0, 5}) {
		t.Errorf("PositionsAt(0) (doc 1) = %v, want [0 5]", doc1Positions)
	}
	doc3Positions := postings.PositionsAt(1)
	if !equalInt32Slices(doc3Positions, []int32{10, 20}) {
		t.Errorf("PositionsAt(1) (doc 3, was after removed doc) = %v, want [10 20]", doc3Positions)
	}
}

// TestRemoveDocument_LastDocument_RebasesCSROffsets covers removing the last
// posting: docIndex == len(DocIDs)-1, so the "subtract span from every later
// offset" loop runs exactly once (on the final offset) and must still land on
// len(Positions) exactly.
func TestRemoveDocument_LastDocument_RebasesCSROffsets(t *testing.T) {
	t.Parallel()
	idx := NewInvertedIndex()

	idx.AddDocument(1, []string{"shared"}, map[string][]int32{"shared": {0, 5}})
	idx.AddDocument(2, []string{"shared"}, map[string][]int32{"shared": {1, 2, 3}})
	idx.AddDocument(3, []string{"shared"}, map[string][]int32{"shared": {10, 20}})

	// Remove the last document (docID 3, docIndex 2).
	idx.RemoveDocument(3)

	postings := idx.Search("shared")
	if postings == nil {
		t.Fatal("Search('shared') returned nil after removal")
	}

	wantDocIDs := []int32{1, 2}
	if !equalInt32Slices(postings.DocIDs, wantDocIDs) {
		t.Fatalf("DocIDs = %v, want %v", postings.DocIDs, wantDocIDs)
	}
	if len(postings.PosOffsets) != len(postings.DocIDs)+1 {
		t.Fatalf("len(PosOffsets) = %d, want len(DocIDs)+1 = %d", len(postings.PosOffsets), len(postings.DocIDs)+1)
	}
	wantPosOffsetsAfter := []int32{0, 2, 5}
	if !equalInt32Slices(postings.PosOffsets, wantPosOffsetsAfter) {
		t.Fatalf("PosOffsets after removal = %v, want %v", postings.PosOffsets, wantPosOffsetsAfter)
	}
	if last := postings.PosOffsets[len(postings.PosOffsets)-1]; int(last) != len(postings.Positions) {
		t.Errorf("final PosOffsets entry = %d, want len(Positions) = %d", last, len(postings.Positions))
	}

	doc1Positions := postings.PositionsAt(0)
	if !equalInt32Slices(doc1Positions, []int32{0, 5}) {
		t.Errorf("PositionsAt(0) (doc 1) = %v, want [0 5]", doc1Positions)
	}
	doc2Positions := postings.PositionsAt(1)
	if !equalInt32Slices(doc2Positions, []int32{1, 2, 3}) {
		t.Errorf("PositionsAt(1) (doc 2) = %v, want [1 2 3]", doc2Positions)
	}
}

func equalInt32Slices(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRemoveDocument_NonExistent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	if len(idx.DocLengths) != 0 {
		t.Errorf("len(DocLengths) = %d, want 0", len(idx.DocLengths))
	}
}

func TestGetAllTerms(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	if len(quickPostings.DocIDs) != 1 {
		t.Fatalf("len(DocIDs) = %d, want 1", len(quickPostings.DocIDs))
	}
	quickPositions := quickPostings.PositionsAt(0)
	if len(quickPositions) != 2 {
		t.Errorf("len(PositionsAt(0)) = %d, want 2", len(quickPositions))
	}
	if quickPositions[0] != 1 || quickPositions[1] != 4 {
		t.Errorf("PositionsAt(0) = %v, want [1, 4]", quickPositions)
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
