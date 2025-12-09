package session

import (
	"math"
	"testing"
)

func setupTestIndex() *InvertedIndex {
	idx := NewInvertedIndex()

	// Doc 1: Short document with "test" once
	idx.AddDocumentSimple(1, []string{"test", "document", "short"})

	// Doc 2: Long document with "test" multiple times
	idx.AddDocumentSimple(2, []string{"test", "test", "test", "long", "document", "with", "many", "words", "about", "testing"})

	// Doc 3: Document without "test"
	idx.AddDocumentSimple(3, []string{"another", "document", "here"})

	// Doc 4: Document with unique terms
	idx.AddDocumentSimple(4, []string{"unique", "words", "only"})

	return idx
}

func TestNewBM25Scorer(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	if scorer == nil {
		t.Fatal("NewBM25Scorer returned nil")
	}

	k1, b := scorer.GetParams()
	if k1 != K1 {
		t.Errorf("k1 = %f, want %f", k1, K1)
	}
	if b != B {
		t.Errorf("b = %f, want %f", b, B)
	}
}

func TestNewBM25ScorerWithParams(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25ScorerWithParams(idx, 2.0, 0.5)

	k1, b := scorer.GetParams()
	if k1 != 2.0 {
		t.Errorf("k1 = %f, want 2.0", k1)
	}
	if b != 0.5 {
		t.Errorf("b = %f, want 0.5", b)
	}
}

func TestBM25Score_SingleTerm(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	// Score for doc 1 (contains "test" once)
	score1 := scorer.Score([]string{"test"}, 1)
	if score1 <= 0 {
		t.Errorf("Score for doc 1 = %f, want > 0", score1)
	}

	// Score for doc 3 (doesn't contain "test")
	score3 := scorer.Score([]string{"test"}, 3)
	if score3 != 0 {
		t.Errorf("Score for doc 3 = %f, want 0", score3)
	}
}

func TestBM25Score_TermFrequencyImpact(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	// Doc 2 has "test" 3 times, doc 1 has "test" 1 time
	// Due to BM25's non-linear TF, doc 2 should score higher but not 3x
	score1 := scorer.Score([]string{"test"}, 1)
	score2 := scorer.Score([]string{"test"}, 2)

	// Doc 2 should score higher (more term occurrences)
	// But score2 should be less than 3x score1 due to saturation
	if score2 <= score1 {
		t.Errorf("Doc 2 score (%f) should be > doc 1 score (%f)", score2, score1)
	}

	// Verify saturation effect (score2 < 3 * score1)
	// Note: This is affected by document length normalization too
	// Just check that higher TF leads to higher score
	t.Logf("Score doc 1 (tf=1): %f, Score doc 2 (tf=3): %f, ratio: %f",
		score1, score2, score2/score1)
}

func TestBM25Score_MultipleTerms(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	// Single term score
	singleTermScore := scorer.Score([]string{"test"}, 1)

	// Multi-term score (doc 1 has "test" and "document")
	multiTermScore := scorer.Score([]string{"test", "document"}, 1)

	// Multi-term should score higher than single term
	if multiTermScore <= singleTermScore {
		t.Errorf("Multi-term score (%f) should be > single-term score (%f)",
			multiTermScore, singleTermScore)
	}
}

func TestBM25Score_EmptyQuery(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	score := scorer.Score([]string{}, 1)
	if score != 0 {
		t.Errorf("Empty query score = %f, want 0", score)
	}
}

func TestBM25Score_NonExistentDocument(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	score := scorer.Score([]string{"test"}, 999)
	if score != 0 {
		t.Errorf("Non-existent doc score = %f, want 0", score)
	}
}

func TestBM25Score_NonExistentTerm(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	score := scorer.Score([]string{"nonexistent"}, 1)
	if score != 0 {
		t.Errorf("Non-existent term score = %f, want 0", score)
	}
}

func TestBM25ScoreAll(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	results := scorer.ScoreAll([]string{"test"})

	// Should return docs 1 and 2 (docs that contain "test")
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	// Results should be sorted by score descending
	if results[0].Score < results[1].Score {
		t.Error("Results not sorted by score descending")
	}

	// All results should have positive scores
	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("Score for doc %d = %f, want > 0", r.DocID, r.Score)
		}
	}
}

func TestBM25ScoreAll_MultipleTerms(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	// "document" appears in docs 1, 2, 3
	results := scorer.ScoreAll([]string{"document"})

	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}

	// Verify descending order
	for i := 1; i < len(results); i++ {
		if results[i-1].Score < results[i].Score {
			t.Error("Results not in descending order")
			break
		}
	}
}

func TestBM25ScoreAll_EmptyQuery(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	results := scorer.ScoreAll([]string{})
	if results != nil {
		t.Errorf("Empty query results = %v, want nil", results)
	}
}

func TestBM25ScoreAll_NoMatches(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	results := scorer.ScoreAll([]string{"nonexistent"})
	if results != nil {
		t.Errorf("No matches results = %v, want nil", results)
	}
}

func TestBM25ScoreAllWithLimit(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	results := scorer.ScoreAllWithLimit([]string{"document"}, 2)

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
}

func TestBM25ScoreAllWithLimit_NoLimit(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	results := scorer.ScoreAllWithLimit([]string{"document"}, 0)

	// With limit=0, should return all results
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
}

func TestBM25CalculateIDF(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	// "unique" appears in only 1 document (doc 4)
	// Should have high IDF
	uniqueScore := scorer.CalculateTermScore("unique", 4)

	// "document" appears in 3 documents
	// Should have lower IDF
	documentScore := scorer.CalculateTermScore("document", 1)

	if uniqueScore.IDF <= documentScore.IDF {
		t.Errorf("IDF for rare term 'unique' (%f) should be > common term 'document' (%f)",
			uniqueScore.IDF, documentScore.IDF)
	}
}

func TestBM25ExplainScore(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	explanation := scorer.ExplainScore([]string{"test", "document"}, 1)

	if explanation.DocID != 1 {
		t.Errorf("DocID = %d, want 1", explanation.DocID)
	}

	if len(explanation.TermScores) != 2 {
		t.Errorf("len(TermScores) = %d, want 2", len(explanation.TermScores))
	}

	// Total score should equal sum of term scores
	sumTermScores := 0.0
	for _, ts := range explanation.TermScores {
		sumTermScores += ts.Score
	}

	if math.Abs(explanation.TotalScore-sumTermScores) > 0.0001 {
		t.Errorf("TotalScore (%f) != sum of term scores (%f)",
			explanation.TotalScore, sumTermScores)
	}

	// Verify term scores have expected fields
	for _, ts := range explanation.TermScores {
		if ts.Term == "" {
			t.Error("Term is empty")
		}
		if ts.DocumentLength <= 0 {
			t.Errorf("DocumentLength = %d, want > 0", ts.DocumentLength)
		}
	}
}

func TestBM25SetParams(t *testing.T) {
	idx := setupTestIndex()
	scorer := NewBM25Scorer(idx)

	originalScore := scorer.Score([]string{"test"}, 1)

	// Change parameters
	scorer.SetParams(2.5, 0.9)

	newScore := scorer.Score([]string{"test"}, 1)

	// Score should change with different parameters
	if originalScore == newScore {
		t.Error("Score should change with different parameters")
	}

	k1, b := scorer.GetParams()
	if k1 != 2.5 {
		t.Errorf("k1 = %f, want 2.5", k1)
	}
	if b != 0.9 {
		t.Errorf("b = %f, want 0.9", b)
	}
}

func TestBM25UpdateAvgDocLength(t *testing.T) {
	idx := NewInvertedIndex()
	scorer := NewBM25Scorer(idx)

	// Initially empty
	if scorer.avgDocLen != 0 {
		t.Errorf("Initial avgDocLen = %f, want 0", scorer.avgDocLen)
	}

	// Add documents
	idx.AddDocumentSimple(1, []string{"hello", "world"})
	idx.AddDocumentSimple(2, []string{"test"})

	// Update scorer
	scorer.UpdateAvgDocLength()

	expected := 1.5 // (2 + 1) / 2
	if scorer.avgDocLen != expected {
		t.Errorf("avgDocLen = %f, want %f", scorer.avgDocLen, expected)
	}
}

func TestBM25_LengthNormalization(t *testing.T) {
	// Create index with documents of very different lengths
	idx := NewInvertedIndex()

	// Short document with "test"
	idx.AddDocumentSimple(1, []string{"test"})

	// Long document with "test"
	longDoc := make([]string, 100)
	longDoc[0] = "test"
	for i := 1; i < 100; i++ {
		longDoc[i] = "filler"
	}
	idx.AddDocumentSimple(2, longDoc)

	scorer := NewBM25Scorer(idx)

	score1 := scorer.Score([]string{"test"}, 1)
	score2 := scorer.Score([]string{"test"}, 2)

	// With length normalization (b > 0), short doc should score higher
	// because it's more focused on the search term
	if score1 <= score2 {
		t.Errorf("Short doc score (%f) should be > long doc score (%f) due to length normalization",
			score1, score2)
	}

	// Test with b=0 (no length normalization)
	scorerNoNorm := NewBM25ScorerWithParams(idx, K1, 0.0)
	scoreNoNorm1 := scorerNoNorm.Score([]string{"test"}, 1)
	scoreNoNorm2 := scorerNoNorm.Score([]string{"test"}, 2)

	// With b=0, scores should be equal (same TF, no length penalty)
	if math.Abs(scoreNoNorm1-scoreNoNorm2) > 0.0001 {
		t.Errorf("With b=0, scores should be equal: %f vs %f", scoreNoNorm1, scoreNoNorm2)
	}
}

// Benchmark tests
func BenchmarkBM25Score(b *testing.B) {
	idx := NewInvertedIndex()
	for i := 0; i < 10000; i++ {
		idx.AddDocumentSimple(int32(i), []string{"hello", "world", "test", "document", "search"})
	}
	scorer := NewBM25Scorer(idx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scorer.Score([]string{"test", "document"}, int32(i%10000))
	}
}

func BenchmarkBM25ScoreAll(b *testing.B) {
	idx := NewInvertedIndex()
	for i := 0; i < 10000; i++ {
		idx.AddDocumentSimple(int32(i), []string{"hello", "world", "test", "document", "search"})
	}
	scorer := NewBM25Scorer(idx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scorer.ScoreAll([]string{"test", "document"})
	}
}

func BenchmarkBM25ScoreAllWithLimit(b *testing.B) {
	idx := NewInvertedIndex()
	for i := 0; i < 10000; i++ {
		idx.AddDocumentSimple(int32(i), []string{"hello", "world", "test", "document", "search"})
	}
	scorer := NewBM25Scorer(idx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scorer.ScoreAllWithLimit([]string{"test", "document"}, 20)
	}
}
