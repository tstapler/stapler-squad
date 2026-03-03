package search

import (
	"math"
	"sort"
)

// BM25 scoring parameters
const (
	// K1 controls term frequency saturation. Higher values increase the impact
	// of term frequency. Standard value is 1.2-2.0.
	K1 = 1.5

	// B controls document length normalization. B=1.0 means full normalization,
	// B=0 means no normalization. Standard value is 0.75.
	B = 0.75
)

// BM25Scorer implements the Okapi BM25 relevance scoring algorithm.
// BM25 is a probabilistic ranking function that improves upon TF-IDF
// by adding term frequency saturation and document length normalization.
type BM25Scorer struct {
	index     *InvertedIndex
	k1        float64
	b         float64
	avgDocLen float64
}

// NewBM25Scorer creates a new BM25 scorer for the given inverted index.
func NewBM25Scorer(index *InvertedIndex) *BM25Scorer {
	return &BM25Scorer{
		index:     index,
		k1:        K1,
		b:         B,
		avgDocLen: index.GetAvgDocLength(),
	}
}

// NewBM25ScorerWithParams creates a BM25 scorer with custom parameters.
func NewBM25ScorerWithParams(index *InvertedIndex, k1, b float64) *BM25Scorer {
	return &BM25Scorer{
		index:     index,
		k1:        k1,
		b:         b,
		avgDocLen: index.GetAvgDocLength(),
	}
}

// Score calculates the BM25 relevance score for a document given a query.
// The query is provided as a list of terms (already tokenized).
//
// BM25 formula:
// score(D, Q) = Σ IDF(qi) * (f(qi, D) * (k1 + 1)) / (f(qi, D) + k1 * (1 - b + b * |D| / avgdl))
//
// Where:
// - D = document
// - Q = query
// - qi = query term i
// - f(qi, D) = frequency of term qi in document D
// - |D| = document length (number of tokens)
// - avgdl = average document length
// - k1 = term frequency saturation parameter
// - b = length normalization parameter
func (s *BM25Scorer) Score(queryTerms []string, docID int32) float64 {
	if len(queryTerms) == 0 {
		return 0.0
	}

	docLen := float64(s.index.GetDocLength(docID))
	if docLen == 0 {
		return 0.0
	}

	// Handle edge case where avgDocLen is 0
	avgDocLen := s.avgDocLen
	if avgDocLen == 0 {
		avgDocLen = docLen // Treat as normalized (no penalty/boost)
	}

	score := 0.0
	for _, term := range queryTerms {
		// Get term frequency in this document
		tf := s.getTermFrequency(term, docID)
		if tf == 0 {
			continue
		}

		// Calculate IDF
		idf := s.calculateIDF(term)

		// Calculate BM25 score component for this term
		numerator := tf * (s.k1 + 1)
		denominator := tf + s.k1*(1-s.b+s.b*(docLen/avgDocLen))

		score += idf * (numerator / denominator)
	}

	return score
}

// ScoreAll calculates BM25 scores for all documents containing any query term.
// Returns a slice of ScoredDocument sorted by score (descending).
func (s *BM25Scorer) ScoreAll(queryTerms []string) []ScoredDocument {
	if len(queryTerms) == 0 {
		return nil
	}

	// Find all candidate documents (those containing at least one query term)
	candidates := make(map[int32]bool)
	for _, term := range queryTerms {
		postings := s.index.Search(term)
		if postings != nil {
			for _, docID := range postings.DocIDs {
				candidates[docID] = true
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Score each candidate
	results := make([]ScoredDocument, 0, len(candidates))
	for docID := range candidates {
		score := s.Score(queryTerms, docID)
		if score > 0 {
			results = append(results, ScoredDocument{
				DocID: docID,
				Score: score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// ScoreAllWithLimit is like ScoreAll but returns at most limit results.
func (s *BM25Scorer) ScoreAllWithLimit(queryTerms []string, limit int) []ScoredDocument {
	results := s.ScoreAll(queryTerms)
	if limit > 0 && len(results) > limit {
		return results[:limit]
	}
	return results
}

// ScoredDocument represents a document with its BM25 relevance score.
type ScoredDocument struct {
	DocID int32
	Score float64
}

// calculateIDF calculates the Inverse Document Frequency for a term.
// Uses the BM25 IDF+ variant which adds 1 to ensure non-negative scores:
// IDF(qi) = log(1 + (N - df(qi) + 0.5) / (df(qi) + 0.5))
// Where N is total documents and df is document frequency.
func (s *BM25Scorer) calculateIDF(term string) float64 {
	N := float64(s.index.GetTotalDocs())
	df := float64(s.index.GetDocumentFrequency(term))

	if df == 0 || N == 0 {
		return 0.0
	}

	// BM25+ IDF formula: adds 1 inside log to ensure non-negative IDF
	// This prevents common terms from having zero or negative IDF
	return math.Log(1 + (N-df+0.5)/(df+0.5))
}

// getTermFrequency returns the frequency of a term in a specific document.
func (s *BM25Scorer) getTermFrequency(term string, docID int32) float64 {
	postings := s.index.Search(term)
	if postings == nil {
		return 0.0
	}

	// Find the document in the postings list
	for i, id := range postings.DocIDs {
		if id == docID {
			return float64(postings.Frequency[i])
		}
	}

	return 0.0
}

// UpdateAvgDocLength updates the average document length.
// Call this after adding/removing documents if using cached scorer.
func (s *BM25Scorer) UpdateAvgDocLength() {
	s.avgDocLen = s.index.GetAvgDocLength()
}

// GetParams returns the current BM25 parameters (k1, b).
func (s *BM25Scorer) GetParams() (k1, b float64) {
	return s.k1, s.b
}

// SetParams updates the BM25 parameters.
func (s *BM25Scorer) SetParams(k1, b float64) {
	s.k1 = k1
	s.b = b
}

// CalculateTermScore calculates the BM25 score contribution from a single term.
// Useful for debugging or understanding score breakdown.
func (s *BM25Scorer) CalculateTermScore(term string, docID int32) TermScore {
	docLen := float64(s.index.GetDocLength(docID))
	avgDocLen := s.avgDocLen
	if avgDocLen == 0 {
		avgDocLen = docLen
	}

	tf := s.getTermFrequency(term, docID)
	idf := s.calculateIDF(term)
	df := s.index.GetDocumentFrequency(term)

	var score float64
	if tf > 0 && docLen > 0 {
		numerator := tf * (s.k1 + 1)
		denominator := tf + s.k1*(1-s.b+s.b*(docLen/avgDocLen))
		score = idf * (numerator / denominator)
	}

	return TermScore{
		Term:              term,
		TermFrequency:     tf,
		DocumentFrequency: df,
		IDF:               idf,
		Score:             score,
		DocumentLength:    int(docLen),
		AvgDocLength:      avgDocLen,
	}
}

// TermScore provides detailed scoring breakdown for a single term.
type TermScore struct {
	Term              string
	TermFrequency     float64
	DocumentFrequency int
	IDF               float64
	Score             float64
	DocumentLength    int
	AvgDocLength      float64
}

// ExplainScore provides a detailed breakdown of the BM25 score for debugging.
func (s *BM25Scorer) ExplainScore(queryTerms []string, docID int32) ScoreExplanation {
	termScores := make([]TermScore, 0, len(queryTerms))
	totalScore := 0.0

	for _, term := range queryTerms {
		ts := s.CalculateTermScore(term, docID)
		termScores = append(termScores, ts)
		totalScore += ts.Score
	}

	return ScoreExplanation{
		DocID:      docID,
		TotalScore: totalScore,
		TermScores: termScores,
		K1:         s.k1,
		B:          s.b,
	}
}

// ScoreExplanation provides detailed information about how a score was calculated.
type ScoreExplanation struct {
	DocID      int32
	TotalScore float64
	TermScores []TermScore
	K1         float64
	B          float64
}
