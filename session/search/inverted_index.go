package search

import (
	"sync"
)

// PostingsList represents the posting list for a term in the inverted index.
// It contains document IDs, positions within each document, and term frequencies.
//
// Positions are stored CSR-style (one flat slice plus per-document offsets)
// instead of [][]int32. A separate slice per document meant AddDocument
// allocated O(terms × docs) tiny slices; profiling a real index load showed
// this dominating resident heap. Flattening cuts that to O(terms) growable
// slices — read a document's positions back out via PositionsAt.
type PostingsList struct {
	// DocIDs contains the list of document IDs containing this term
	DocIDs []int32
	// Positions is the flat, concatenated positions for every document in
	// DocIDs. Use PositionsAt(i) rather than indexing this directly.
	Positions []int32
	// PosOffsets has len(DocIDs)+1 entries; PositionsAt(i) is
	// Positions[PosOffsets[i]:PosOffsets[i+1]].
	PosOffsets []int32
	// Frequency contains the term frequency in each document
	// Frequency[i] corresponds to DocIDs[i]
	Frequency []int32
}

// PositionsAt returns the positions recorded for DocIDs[i].
func (p *PostingsList) PositionsAt(i int) []int32 {
	return p.Positions[p.PosOffsets[i]:p.PosOffsets[i+1]]
}

// InvertedIndex is a thread-safe inverted index for full-text search.
// It maps terms to posting lists containing document IDs, positions, and frequencies.
type InvertedIndex struct {
	// Index maps terms to their posting lists
	Index map[string]*PostingsList
	// TotalDocs is the total number of documents indexed
	TotalDocs int
	// DocLengths maps document IDs to their lengths (number of tokens) for BM25
	DocLengths map[int32]int
	// AvgDocLength is the average document length for BM25 scoring
	AvgDocLength float64
	// mu protects concurrent access to the index
	mu sync.RWMutex
}

// NewInvertedIndex creates a new empty inverted index.
func NewInvertedIndex() *InvertedIndex {
	return &InvertedIndex{
		Index:        make(map[string]*PostingsList),
		DocLengths:   make(map[int32]int),
		TotalDocs:    0,
		AvgDocLength: 0,
	}
}

// AddDocument adds a document to the inverted index.
// docID is a unique identifier for the document.
// tokens is the list of tokens (already processed by the tokenizer).
// positions maps each token to its positions in the original document.
func (idx *InvertedIndex) AddDocument(docID int32, tokens []string, positions map[string][]int32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Track term frequencies within this document
	termFreq := make(map[string]int)
	for _, token := range tokens {
		termFreq[token]++
	}

	// Update document count and length
	idx.TotalDocs++
	idx.DocLengths[docID] = len(tokens)

	// Recalculate average document length
	totalLength := 0
	for _, length := range idx.DocLengths {
		totalLength += length
	}
	idx.AvgDocLength = float64(totalLength) / float64(idx.TotalDocs)

	// Add each unique term to the index
	for term, freq := range termFreq {
		// Get or create posting list for this term
		postingList, exists := idx.Index[term]
		if !exists {
			postingList = &PostingsList{
				DocIDs:     make([]int32, 0),
				Positions:  make([]int32, 0),
				PosOffsets: []int32{0},
				Frequency:  make([]int32, 0),
			}
			idx.Index[term] = postingList
		}

		// Add this document to the posting list
		postingList.DocIDs = append(postingList.DocIDs, docID)
		// #nosec G115 -- freq is a per-term occurrence count within a single document (tokens); no realistic document has anywhere near 2^31 tokens
		postingList.Frequency = append(postingList.Frequency, int32(freq))

		// Append positions (if any) to the flat slice and record the new offset
		if pos, ok := positions[term]; ok {
			postingList.Positions = append(postingList.Positions, pos...)
		}
		// #nosec G115 -- Positions is an in-memory, per-process search index over local session/scrollback content; reaching 2^31 accumulated positions for one term would require terabytes of indexed text, far beyond this process's practical memory footprint
		postingList.PosOffsets = append(postingList.PosOffsets, int32(len(postingList.Positions)))
	}
}

// AddDocumentSimple is a simplified version of AddDocument that takes just tokens
// without position information. Useful when positions aren't needed.
func (idx *InvertedIndex) AddDocumentSimple(docID int32, tokens []string) {
	// Build positions map from tokens
	positions := make(map[string][]int32)
	for i, token := range tokens {
		positions[token] = append(positions[token], int32(i))
	}
	idx.AddDocument(docID, tokens, positions)
}

// Search returns the posting list for a term.
// Returns nil if the term is not in the index.
func (idx *InvertedIndex) Search(term string) *PostingsList {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.Index[term]
}

// SearchMultiple returns posting lists for multiple terms.
// Useful for multi-term queries.
func (idx *InvertedIndex) SearchMultiple(terms []string) map[string]*PostingsList {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	results := make(map[string]*PostingsList)
	for _, term := range terms {
		if postingList, exists := idx.Index[term]; exists {
			results[term] = postingList
		}
	}
	return results
}

// GetDocumentFrequency returns the number of documents containing the term.
func (idx *InvertedIndex) GetDocumentFrequency(term string) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	postingList, exists := idx.Index[term]
	if !exists {
		return 0
	}
	return len(postingList.DocIDs)
}

// GetTotalDocs returns the total number of indexed documents.
func (idx *InvertedIndex) GetTotalDocs() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.TotalDocs
}

// GetDocLength returns the length of a specific document.
func (idx *InvertedIndex) GetDocLength(docID int32) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.DocLengths[docID]
}

// GetAvgDocLength returns the average document length.
func (idx *InvertedIndex) GetAvgDocLength() float64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.AvgDocLength
}

// GetTermCount returns the total number of unique terms in the index.
func (idx *InvertedIndex) GetTermCount() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return len(idx.Index)
}

// HasDocument checks if a document ID exists in the index.
func (idx *InvertedIndex) HasDocument(docID int32) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	_, exists := idx.DocLengths[docID]
	return exists
}

// RemoveDocument removes a document from the index.
// This is an expensive operation as it requires updating all posting lists.
func (idx *InvertedIndex) RemoveDocument(docID int32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Check if document exists
	if _, exists := idx.DocLengths[docID]; !exists {
		return
	}

	// Remove from all posting lists
	termsToDelete := make([]string, 0)
	for term, postingList := range idx.Index {
		// Find the index of this docID in the posting list
		docIndex := -1
		for i, id := range postingList.DocIDs {
			if id == docID {
				docIndex = i
				break
			}
		}

		if docIndex >= 0 {
			// Remove docIndex's positions from the flat slice, then shift every
			// later offset left by the removed span's length.
			start, end := postingList.PosOffsets[docIndex], postingList.PosOffsets[docIndex+1]
			span := end - start
			postingList.Positions = append(postingList.Positions[:start], postingList.Positions[end:]...)
			postingList.PosOffsets = append(postingList.PosOffsets[:docIndex], postingList.PosOffsets[docIndex+1:]...)
			for i := docIndex; i < len(postingList.PosOffsets); i++ {
				postingList.PosOffsets[i] -= span
			}

			// Remove from posting list
			postingList.DocIDs = append(postingList.DocIDs[:docIndex], postingList.DocIDs[docIndex+1:]...)
			postingList.Frequency = append(postingList.Frequency[:docIndex], postingList.Frequency[docIndex+1:]...)

			// Mark term for deletion if no documents contain it
			if len(postingList.DocIDs) == 0 {
				termsToDelete = append(termsToDelete, term)
			}
		}
	}

	// Delete empty terms
	for _, term := range termsToDelete {
		delete(idx.Index, term)
	}

	// Update document count and length tracking
	delete(idx.DocLengths, docID)
	idx.TotalDocs--

	// Recalculate average document length
	if idx.TotalDocs > 0 {
		totalLength := 0
		for _, length := range idx.DocLengths {
			totalLength += length
		}
		idx.AvgDocLength = float64(totalLength) / float64(idx.TotalDocs)
	} else {
		idx.AvgDocLength = 0
	}
}

// Clear removes all documents and terms from the index.
func (idx *InvertedIndex) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.Index = make(map[string]*PostingsList)
	idx.DocLengths = make(map[int32]int)
	idx.TotalDocs = 0
	idx.AvgDocLength = 0
}

// GetAllTerms returns all terms in the index.
func (idx *InvertedIndex) GetAllTerms() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	terms := make([]string, 0, len(idx.Index))
	for term := range idx.Index {
		terms = append(terms, term)
	}
	return terms
}

// GetStats returns statistics about the index.
func (idx *InvertedIndex) GetStats() InvertedIndexStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	totalPostings := 0
	for _, postingList := range idx.Index {
		totalPostings += len(postingList.DocIDs)
	}

	return InvertedIndexStats{
		TotalDocs:      idx.TotalDocs,
		TotalTerms:     len(idx.Index),
		TotalPostings:  totalPostings,
		AvgDocLength:   idx.AvgDocLength,
		AvgTermsPerDoc: float64(totalPostings) / float64(max(idx.TotalDocs, 1)),
	}
}

// InvertedIndexStats contains statistics about the inverted index.
type InvertedIndexStats struct {
	TotalDocs      int
	TotalTerms     int
	TotalPostings  int
	AvgDocLength   float64
	AvgTermsPerDoc float64
}
