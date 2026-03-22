package search

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// SearchEngine is the main interface for full-text search over Claude history.
// It combines tokenization, indexing, scoring, and snippet generation.
type SearchEngine struct {
	index        *InvertedIndex
	docStore     *DocumentStore
	tokenizer    *Tokenizer
	scorer       *BM25Scorer
	indexStore   *IndexStore
	syncMetadata *IndexSyncMetadata
	mu           sync.RWMutex
}

// SearchOptions configures search behavior.
type SearchOptions struct {
	// Limit is the maximum number of results to return (0 = no limit)
	Limit int
	// Offset is the number of results to skip for pagination
	Offset int
	// SessionID filters results to a specific session (empty = all sessions)
	SessionID string
}

// SearchResults contains the results of a search query.
type SearchResults struct {
	// Results is the list of search results
	Results []SearchResult
	// TotalMatches is the total number of matching documents (before pagination)
	TotalMatches int
	// QueryTime is the duration of the search operation
	QueryTime time.Duration
}

// SearchResult represents a single search result.
type SearchResult struct {
	// DocID is the internal document ID
	DocID int32
	// SessionID is the conversation ID containing this message
	SessionID string
	// MessageIndex is the index of the message within the conversation
	MessageIndex int
	// MessageRole is the role (user, assistant, system)
	MessageRole string
	// Score is the BM25 relevance score
	Score float64
	// Content is the full message content
	Content string
	// Timestamp is when the message was created
	Timestamp time.Time
}

// NewSearchEngine creates a new search engine instance.
func NewSearchEngine() *SearchEngine {
	index := NewInvertedIndex()
	docStore := NewDocumentStore()
	tokenizer := NewTokenizer()
	scorer := NewBM25Scorer(index)

	return &SearchEngine{
		index:     index,
		docStore:  docStore,
		tokenizer: tokenizer,
		scorer:    scorer,
	}
}

// NewSearchEngineWithPersistence creates a search engine with index persistence.
func NewSearchEngineWithPersistence(indexStore *IndexStore) *SearchEngine {
	engine := NewSearchEngine()
	engine.indexStore = indexStore
	return engine
}

// BuildIndex indexes all messages from the provided history.
// This replaces any existing index.
func (e *SearchEngine) BuildIndex(history *session.ClaudeSessionHistory) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Clear existing index
	e.index.Clear()
	e.docStore.Clear()

	// Get all history entries
	entries := history.GetAll()

	for _, entry := range entries {
		// Load messages for this conversation
		messages, err := history.GetMessagesFromConversationFile(entry.ID)
		if err != nil {
			// Log and continue - don't fail entire index build
			continue
		}

		// Index each message
		for msgIdx, msg := range messages {
			// Tokenize content
			tokens := e.tokenizer.Tokenize(msg.Content)
			if len(tokens) == 0 {
				continue
			}

			// Create document
			doc := &Document{
				SessionID:    entry.ID,
				MessageIndex: msgIdx,
				MessageRole:  msg.Role,
				Content:      msg.Content,
				WordCount:    len(tokens),
				Timestamp:    msg.Timestamp,
			}

			// Add to document store
			docID := e.docStore.Add(doc)

			// Get positions for index
			tokenPositions := e.tokenizer.TokenizeWithPositions(msg.Content)
			positions := make(map[string][]int32)
			for _, tp := range tokenPositions {
				positions[tp.Token] = append(positions[tp.Token], int32(tp.Start))
			}

			// Add to inverted index
			e.index.AddDocument(docID, tokens, positions)
		}
	}

	// Update scorer with new average document length
	e.scorer.UpdateAvgDocLength()

	// Persist index if store is configured
	if e.indexStore != nil {
		if err := e.indexStore.Save(e.index, e.docStore); err != nil {
			return err
		}
	}

	return nil
}

// IndexMessage adds a single message to the index.
// Use for incremental updates when new messages arrive.
func (e *SearchEngine) IndexMessage(sessionID string, msgIdx int, role, content string, timestamp time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Tokenize content
	tokens := e.tokenizer.Tokenize(content)
	if len(tokens) == 0 {
		return nil // Nothing to index
	}

	// Create document
	doc := &Document{
		SessionID:    sessionID,
		MessageIndex: msgIdx,
		MessageRole:  role,
		Content:      content,
		WordCount:    len(tokens),
		Timestamp:    timestamp,
	}

	// Add to document store
	docID := e.docStore.Add(doc)

	// Get positions for index
	tokenPositions := e.tokenizer.TokenizeWithPositions(content)
	positions := make(map[string][]int32)
	for _, tp := range tokenPositions {
		positions[tp.Token] = append(positions[tp.Token], int32(tp.Start))
	}

	// Add to inverted index
	e.index.AddDocument(docID, tokens, positions)

	// Update scorer
	e.scorer.UpdateAvgDocLength()

	return nil
}

// Search performs a full-text search on the indexed messages.
func (e *SearchEngine) Search(query string, opts SearchOptions) (*SearchResults, error) {
	startTime := time.Now()

	e.mu.RLock()
	defer e.mu.RUnlock()

	// Tokenize query
	queryTokens := e.tokenizer.Tokenize(query)
	if len(queryTokens) == 0 {
		return &SearchResults{
			Results:      []SearchResult{},
			TotalMatches: 0,
			QueryTime:    time.Since(startTime),
		}, nil
	}

	// Get scored documents
	scoredDocs := e.scorer.ScoreAll(queryTokens)

	// Filter by session if specified
	if opts.SessionID != "" {
		filtered := make([]ScoredDocument, 0)
		for _, sd := range scoredDocs {
			doc := e.docStore.Get(sd.DocID)
			if doc != nil && doc.SessionID == opts.SessionID {
				filtered = append(filtered, sd)
			}
		}
		scoredDocs = filtered
	}

	totalMatches := len(scoredDocs)

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(scoredDocs) {
		scoredDocs = scoredDocs[opts.Offset:]
	} else if opts.Offset >= len(scoredDocs) {
		scoredDocs = nil
	}

	if opts.Limit > 0 && opts.Limit < len(scoredDocs) {
		scoredDocs = scoredDocs[:opts.Limit]
	}

	// Build search results
	results := make([]SearchResult, 0, len(scoredDocs))
	for _, sd := range scoredDocs {
		doc := e.docStore.Get(sd.DocID)
		if doc == nil {
			continue
		}

		results = append(results, SearchResult{
			DocID:        sd.DocID,
			SessionID:    doc.SessionID,
			MessageIndex: doc.MessageIndex,
			MessageRole:  doc.MessageRole,
			Score:        sd.Score,
			Content:      doc.Content,
			Timestamp:    doc.Timestamp,
		})
	}

	return &SearchResults{
		Results:      results,
		TotalMatches: totalMatches,
		QueryTime:    time.Since(startTime),
	}, nil
}

// LoadIndex loads a previously persisted index from disk.
// Also loads sync metadata if available.
func (e *SearchEngine) LoadIndex() error {
	if e.indexStore == nil {
		return nil
	}

	if !e.indexStore.Exists() {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	loadedIndex, loadedDocStore, err := e.indexStore.Load()
	if err != nil {
		return err
	}

	e.index = loadedIndex
	e.docStore = loadedDocStore
	e.scorer = NewBM25Scorer(e.index)

	// Also load sync metadata
	meta, err := e.indexStore.LoadSyncMetadata()
	if err != nil {
		// Log but don't fail - we can rebuild sync metadata
		return nil
	}
	e.syncMetadata = meta

	return nil
}

// SaveIndex persists the current index to disk.
func (e *SearchEngine) SaveIndex() error {
	if e.indexStore == nil {
		return nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.indexStore.Save(e.index, e.docStore)
}

// GetStats returns statistics about the search engine.
func (e *SearchEngine) GetStats() SearchEngineStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	indexStats := e.index.GetStats()
	docStats := e.docStore.GetStats()

	return SearchEngineStats{
		TotalDocuments:  indexStats.TotalDocs,
		TotalTerms:      indexStats.TotalTerms,
		TotalPostings:   indexStats.TotalPostings,
		TotalSessions:   docStats.TotalSessions,
		AvgDocLength:    indexStats.AvgDocLength,
		AvgTermsPerDoc:  indexStats.AvgTermsPerDoc,
	}
}

// SearchEngineStats contains statistics about the search engine.
type SearchEngineStats struct {
	TotalDocuments int
	TotalTerms     int
	TotalPostings  int
	TotalSessions  int
	AvgDocLength   float64
	AvgTermsPerDoc float64
}

// Clear removes all indexed data.
func (e *SearchEngine) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.index.Clear()
	e.docStore.Clear()
	e.scorer = NewBM25Scorer(e.index)
}

// RemoveSession removes all documents from a session.
func (e *SearchEngine) RemoveSession(sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Get all doc IDs for this session
	docIDs := e.docStore.GetDocIDsBySession(sessionID)

	// Remove from index
	for _, docID := range docIDs {
		e.index.RemoveDocument(docID)
	}

	// Remove from document store
	e.docStore.RemoveBySession(sessionID)

	// Update scorer
	e.scorer.UpdateAvgDocLength()
}

// HasSession returns true if the session is indexed.
func (e *SearchEngine) HasSession(sessionID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.docStore.HasSession(sessionID)
}

// GetDocument returns a document by its ID.
func (e *SearchEngine) GetDocument(docID int32) *Document {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.docStore.Get(docID)
}

// GetIndex returns the underlying inverted index (for advanced usage).
func (e *SearchEngine) GetIndex() *InvertedIndex {
	return e.index
}

// GetDocStore returns the underlying document store (for advanced usage).
func (e *SearchEngine) GetDocStore() *DocumentStore {
	return e.docStore
}

// GetTokenizer returns the tokenizer (for query highlighting).
func (e *SearchEngine) GetTokenizer() *Tokenizer {
	return e.tokenizer
}

// IncrementalSync synchronizes the index with current history state.
// It only indexes new/modified sessions and removes deleted ones.
// On first run or when metadata is missing, falls back to full rebuild.
func (e *SearchEngine) IncrementalSync(history *session.ClaudeSessionHistory) (*SyncResult, error) {
	startTime := time.Now()
	result := &SyncResult{}

	e.mu.Lock()
	defer e.mu.Unlock()

	// If no sync metadata, need full rebuild
	if e.syncMetadata == nil || e.ShouldRebuildLocked() {
		if err := e.buildIndexLocked(history); err != nil {
			return nil, err
		}
		result.WasFullRebuild = true
		result.SessionsAdded = e.syncMetadata.TotalSessions
		result.DocumentsAdded = e.syncMetadata.TotalDocuments
		result.SyncDuration = time.Since(startTime)
		return result, nil
	}

	// Compute what changed
	added, updated, removed := e.computeChangesLocked(history)

	// Process removals first
	for _, sessionID := range removed {
		if meta, ok := e.syncMetadata.Sessions[sessionID]; ok {
			result.DocumentsRemoved += meta.DocCount
		}
		e.removeSessionLocked(sessionID)
		delete(e.syncMetadata.Sessions, sessionID)
		result.SessionsRemoved++
	}

	// Process additions
	for _, entry := range added {
		docCount, err := e.indexSessionLocked(history, entry)
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		e.syncMetadata.Sessions[entry.ID] = &SessionIndexMetadata{
			SessionID:     entry.ID,
			UpdatedAt:     entry.UpdatedAt,
			MessageCount:  entry.MessageCount,
			LastIndexedAt: time.Now(),
			DocCount:      docCount,
		}
		result.SessionsAdded++
		result.DocumentsAdded += docCount
	}

	// Process updates (re-index modified sessions)
	for _, entry := range updated {
		if meta, ok := e.syncMetadata.Sessions[entry.ID]; ok {
			result.DocumentsRemoved += meta.DocCount
		}
		e.removeSessionLocked(entry.ID)

		docCount, err := e.indexSessionLocked(history, entry)
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		e.syncMetadata.Sessions[entry.ID] = &SessionIndexMetadata{
			SessionID:     entry.ID,
			UpdatedAt:     entry.UpdatedAt,
			MessageCount:  entry.MessageCount,
			LastIndexedAt: time.Now(),
			DocCount:      docCount,
		}
		result.SessionsUpdated++
		result.DocumentsAdded += docCount
	}

	// Update aggregate counts
	e.syncMetadata.TotalSessions = len(e.syncMetadata.Sessions)
	totalDocs := 0
	for _, meta := range e.syncMetadata.Sessions {
		totalDocs += meta.DocCount
	}
	e.syncMetadata.TotalDocuments = totalDocs
	e.syncMetadata.LastIncrementalSync = time.Now()

	// Update scorer
	e.scorer.UpdateAvgDocLength()

	// Persist index and metadata
	if e.indexStore != nil {
		if err := e.indexStore.Save(e.index, e.docStore); err != nil {
			return result, err
		}
		if err := e.indexStore.SaveSyncMetadata(e.syncMetadata); err != nil {
			return result, err
		}
	}

	result.SyncDuration = time.Since(startTime)
	return result, nil
}

// computeChangesLocked analyzes history vs index state to determine what changed.
// Must be called with lock held.
func (e *SearchEngine) computeChangesLocked(history *session.ClaudeSessionHistory) (
	added []session.ClaudeHistoryEntry,
	updated []session.ClaudeHistoryEntry,
	removed []string,
) {
	entries := history.GetAll()

	// Build map of current history
	historyMap := make(map[string]session.ClaudeHistoryEntry, len(entries))
	for _, entry := range entries {
		historyMap[entry.ID] = entry
	}

	// Find added and updated sessions
	for _, entry := range entries {
		meta, exists := e.syncMetadata.Sessions[entry.ID]
		if !exists {
			// New session
			added = append(added, entry)
		} else if entry.UpdatedAt.After(meta.UpdatedAt) || entry.MessageCount != meta.MessageCount {
			// Modified session
			updated = append(updated, entry)
		}
	}

	// Find removed sessions (in index but not in history)
	for sessionID := range e.syncMetadata.Sessions {
		if _, exists := historyMap[sessionID]; !exists {
			removed = append(removed, sessionID)
		}
	}

	return added, updated, removed
}

// indexSessionLocked indexes all messages from a single session.
// Must be called with lock held.
func (e *SearchEngine) indexSessionLocked(history *session.ClaudeSessionHistory, entry session.ClaudeHistoryEntry) (int, error) {
	messages, err := history.GetMessagesFromConversationFile(entry.ID)
	if err != nil {
		return 0, err
	}

	docCount := 0
	for msgIdx, msg := range messages {
		tokens := e.tokenizer.Tokenize(msg.Content)
		if len(tokens) == 0 {
			continue
		}

		doc := &Document{
			SessionID:    entry.ID,
			MessageIndex: msgIdx,
			MessageRole:  msg.Role,
			Content:      msg.Content,
			WordCount:    len(tokens),
			Timestamp:    msg.Timestamp,
		}

		docID := e.docStore.Add(doc)

		tokenPositions := e.tokenizer.TokenizeWithPositions(msg.Content)
		positions := make(map[string][]int32)
		for _, tp := range tokenPositions {
			positions[tp.Token] = append(positions[tp.Token], int32(tp.Start))
		}

		e.index.AddDocument(docID, tokens, positions)
		docCount++
	}

	return docCount, nil
}

// removeSessionLocked removes all documents from a session.
// Must be called with lock held.
func (e *SearchEngine) removeSessionLocked(sessionID string) {
	docIDs := e.docStore.GetDocIDsBySession(sessionID)
	for _, docID := range docIDs {
		e.index.RemoveDocument(docID)
	}
	e.docStore.RemoveBySession(sessionID)
}

// ShouldRebuild returns true if a full rebuild is recommended.
// This happens when: no sync metadata, version mismatch, or corruption.
func (e *SearchEngine) ShouldRebuild() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ShouldRebuildLocked()
}

// ShouldRebuildLocked is the lock-free version of ShouldRebuild.
// Must be called with at least read lock held.
func (e *SearchEngine) ShouldRebuildLocked() bool {
	if e.syncMetadata == nil {
		return true
	}
	if e.syncMetadata.Version != CurrentSyncMetadataVersion {
		return true
	}
	// Additional corruption checks could go here
	return false
}

// buildIndexLocked performs a full index build and initializes sync metadata.
// Must be called with lock held.
func (e *SearchEngine) buildIndexLocked(history *session.ClaudeSessionHistory) error {
	// Clear existing index
	e.index.Clear()
	e.docStore.Clear()

	// Initialize fresh sync metadata
	e.syncMetadata = NewIndexSyncMetadata()
	e.syncMetadata.LastFullSync = time.Now()

	// Get all history entries
	entries := history.GetAll()

	for _, entry := range entries {
		docCount, err := e.indexSessionLocked(history, entry)
		if err != nil {
			// Log and continue - don't fail entire index build
			continue
		}

		e.syncMetadata.Sessions[entry.ID] = &SessionIndexMetadata{
			SessionID:     entry.ID,
			UpdatedAt:     entry.UpdatedAt,
			MessageCount:  entry.MessageCount,
			LastIndexedAt: time.Now(),
			DocCount:      docCount,
		}
	}

	// Update aggregate counts
	e.syncMetadata.TotalSessions = len(e.syncMetadata.Sessions)
	totalDocs := 0
	for _, meta := range e.syncMetadata.Sessions {
		totalDocs += meta.DocCount
	}
	e.syncMetadata.TotalDocuments = totalDocs

	// Update scorer with new average document length
	e.scorer.UpdateAvgDocLength()

	// Persist if store is configured
	if e.indexStore != nil {
		if err := e.indexStore.Save(e.index, e.docStore); err != nil {
			return err
		}
		if err := e.indexStore.SaveSyncMetadata(e.syncMetadata); err != nil {
			return err
		}
	}

	return nil
}

// LoadSyncMetadata loads persisted sync metadata from the index store.
func (e *SearchEngine) LoadSyncMetadata() error {
	if e.indexStore == nil {
		return nil
	}

	meta, err := e.indexStore.LoadSyncMetadata()
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.syncMetadata = meta
	return nil
}

// SaveSyncMetadata persists current sync metadata to disk.
func (e *SearchEngine) SaveSyncMetadata() error {
	if e.indexStore == nil || e.syncMetadata == nil {
		return nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.indexStore.SaveSyncMetadata(e.syncMetadata)
}

// GetSyncMetadata returns the current sync metadata (for inspection).
func (e *SearchEngine) GetSyncMetadata() *IndexSyncMetadata {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.syncMetadata
}
