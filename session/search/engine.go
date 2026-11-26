package search

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/session/claudehistory"
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
	// syncMu serialises sync passes, which release mu between sessions.
	syncMu sync.Mutex
	// dirty records index changes not yet saved, so idle passes skip rewriting the index.
	dirty atomic.Bool
	// lastSave throttles full index saves (guarded by syncMu).
	lastSave time.Time

	// lazyLoad defers the persisted-index decode (hundreds of MB of gob) from
	// startup to the first method that touches the index. Every method that
	// reads or writes index state calls ensureLoaded first, so a SaveIndex can
	// never overwrite the persisted index with an unloaded, empty one.
	lazyLoad     bool
	lazyLoadOnce sync.Once
	onLazyLoad   func(meta *IndexSyncMetadata, err error)
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

// EnableLazyLoad makes the engine load its persisted index on first use instead
// of at startup. onLoad, if non-nil, receives the load error and the loaded
// sync metadata (both nil when there is no persisted index); it runs inside the
// load, so it must not call back into the engine. Must be called before the
// engine is shared.
func (e *SearchEngine) EnableLazyLoad(onLoad func(meta *IndexSyncMetadata, err error)) {
	e.lazyLoad = true
	e.onLazyLoad = onLoad
}

// ensureLoaded performs the deferred load at most once. Call it before taking
// e.mu: the load acquires the write lock itself.
func (e *SearchEngine) ensureLoaded() {
	if !e.lazyLoad {
		return
	}
	e.lazyLoadOnce.Do(func() {
		err := e.loadIndexFromDisk()
		if e.onLazyLoad != nil {
			e.mu.RLock()
			meta := e.syncMetadata
			e.mu.RUnlock()
			e.onLazyLoad(meta, err)
		}
	})
}

// markLoaded records that the persisted index no longer needs loading (the
// caller is about to replace the whole index).
func (e *SearchEngine) markLoaded() {
	e.lazyLoadOnce.Do(func() {})
}

// BuildIndex indexes all messages from the provided history.
// This replaces any existing index.
func (e *SearchEngine) BuildIndex(history *claudehistory.ClaudeSessionHistory) error {
	e.ensureLoaded()
	transcripts, err := claudehistory.ScanTranscripts(context.Background(), "")
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	// Clear existing index
	e.index.Clear()
	e.docStore.Clear()
	e.dirty.Store(true)

	for _, entry := range history.GetAll() {
		// A session that can't be read is skipped rather than failing the build.
		_, _ = e.indexSessionLocked(transcripts, entry)
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
	e.ensureLoaded()
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
		// #nosec G115 -- tp.Start is a byte offset into a single chat/scrollback message string; reaching 2^31 would require a single message over 2GB, far beyond any realistic message size
		positions[tp.Token] = append(positions[tp.Token], int32(tp.Start))
	}

	// Add to inverted index
	e.index.AddDocument(docID, tokens, positions)
	e.dirty.Store(true)

	// Update scorer
	e.scorer.UpdateAvgDocLength()

	return nil
}

// Search performs a full-text search on the indexed messages.
func (e *SearchEngine) Search(query string, opts SearchOptions) (*SearchResults, error) {
	e.ensureLoaded()
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
	e.markLoaded() // an explicit load supersedes any pending lazy load
	return e.loadIndexFromDisk()
}

func (e *SearchEngine) loadIndexFromDisk() error {
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
	e.ensureLoaded()
	if e.indexStore == nil {
		return nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.indexStore.Save(e.index, e.docStore)
}

// GetStats returns statistics about the search engine.
func (e *SearchEngine) GetStats() SearchEngineStats {
	e.ensureLoaded()
	e.mu.RLock()
	defer e.mu.RUnlock()

	indexStats := e.index.GetStats()
	docStats := e.docStore.GetStats()

	return SearchEngineStats{
		TotalDocuments: indexStats.TotalDocs,
		TotalTerms:     indexStats.TotalTerms,
		TotalPostings:  indexStats.TotalPostings,
		TotalSessions:  docStats.TotalSessions,
		AvgDocLength:   indexStats.AvgDocLength,
		AvgTermsPerDoc: indexStats.AvgTermsPerDoc,
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
	e.markLoaded()
	e.mu.Lock()
	defer e.mu.Unlock()

	e.index.Clear()
	e.docStore.Clear()
	e.dirty.Store(true)
	e.scorer = NewBM25Scorer(e.index)
}

// RemoveSession removes all documents from a session.
func (e *SearchEngine) RemoveSession(sessionID string) {
	e.ensureLoaded()
	e.mu.Lock()
	defer e.mu.Unlock()

	e.removeSessionsLocked(sessionID)

	// Update scorer
	e.scorer.UpdateAvgDocLength()
}

// HasSession returns true if the session is indexed.
func (e *SearchEngine) HasSession(sessionID string) bool {
	e.ensureLoaded()
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.docStore.HasSession(sessionID)
}

// GetDocument returns a document by its ID.
func (e *SearchEngine) GetDocument(docID int32) *Document {
	e.ensureLoaded()
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.docStore.Get(docID)
}

// GetIndex returns the underlying inverted index (for advanced usage).
func (e *SearchEngine) GetIndex() *InvertedIndex {
	e.ensureLoaded()
	return e.index
}

// GetDocStore returns the underlying document store (for advanced usage).
func (e *SearchEngine) GetDocStore() *DocumentStore {
	e.ensureLoaded()
	return e.docStore
}

// GetTokenizer returns the tokenizer (for query highlighting).
func (e *SearchEngine) GetTokenizer() *Tokenizer {
	return e.tokenizer
}

// IncrementalSync synchronizes the index with current history state.
// It only indexes new/modified sessions and removes deleted ones.
// On first run or when metadata is missing, falls back to full rebuild.
func (e *SearchEngine) IncrementalSync(history *claudehistory.ClaudeSessionHistory) (*SyncResult, error) {
	return e.IncrementalSyncContext(context.Background(), history)
}

// IncrementalSyncContext is IncrementalSync that checks ctx between sessions
// (not within one) and returns ctx.Err() once cancelled, without persisting.
// A cancelled full build clears the sync metadata so the next sync rebuilds; a
// cancelled incremental pass refreshes the totals and the next sync picks up
// the remainder.
func (e *SearchEngine) IncrementalSyncContext(ctx context.Context, history *claudehistory.ClaudeSessionHistory) (*SyncResult, error) {
	e.ensureLoaded()
	// One sync at a time: e.mu is released between sessions so searches keep running.
	e.syncMu.Lock()
	defer e.syncMu.Unlock()

	startTime := time.Now()
	result := &SyncResult{}

	// Scanned once per pass, before taking any lock. Not cancellable: the checks below own
	// what a cancelled pass must still record, and the scan takes well under a second.
	transcripts, err := claudehistory.ScanTranscripts(context.WithoutCancel(ctx), "")
	if err != nil {
		return nil, err
	}

	changed, rebuilt, err := e.beginSync(ctx, history, transcripts, result)
	if err != nil {
		return nil, err
	}
	if rebuilt {
		result.SyncDuration = time.Since(startTime)
		return result, nil
	}

	for i, entry := range changed.entries {
		if err := ctx.Err(); err != nil {
			e.mu.Lock()
			e.finalizeCancelledIncrementalLocked()
			e.mu.Unlock()
			return nil, err
		}
		e.syncSession(transcripts, entry, i < changed.added, result)
	}

	e.mu.Lock()
	e.refreshAggregatesLocked()
	e.syncMetadata.LastIncrementalSync = time.Now()
	e.mu.Unlock()

	if err := e.persist(); err != nil {
		return result, err
	}
	result.SyncDuration = time.Since(startTime)
	return result, nil
}

// pendingSessions lists the sessions to (re)index; the first `added` of entries are new.
type pendingSessions struct {
	entries []claudehistory.ClaudeHistoryEntry
	added   int
}

// beginSync does the work that needs the write lock up front: a full rebuild when
// required, otherwise diffing history against the index and dropping removed sessions.
func (e *SearchEngine) beginSync(ctx context.Context, history *claudehistory.ClaudeSessionHistory, transcripts *claudehistory.Transcripts, result *SyncResult) (pendingSessions, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// If no sync metadata, need full rebuild
	if e.syncMetadata == nil || e.ShouldRebuildLocked() {
		if err := e.buildIndexLocked(ctx, history, transcripts); err != nil {
			return pendingSessions{}, false, err
		}
		result.WasFullRebuild = true
		result.SessionsAdded = e.syncMetadata.TotalSessions
		result.DocumentsAdded = e.syncMetadata.TotalDocuments
		return pendingSessions{}, true, nil
	}

	// After the rebuild branch: a cancelled rebuild must clear the metadata (see
	// buildIndexLocked), so only the incremental path may return early.
	if err := ctx.Err(); err != nil {
		return pendingSessions{}, false, err
	}

	added, updated, removed := e.computeChangesLocked(history)

	var doomed []string
	for _, sessionID := range removed {
		if meta, ok := e.syncMetadata.Sessions[sessionID]; ok {
			result.DocumentsRemoved += meta.DocCount
		}
		doomed = append(doomed, sessionID)
		delete(e.syncMetadata.Sessions, sessionID)
		result.SessionsRemoved++
	}
	e.removeSessionsLocked(doomed...)

	return pendingSessions{entries: append(added, updated...), added: len(added)}, false, nil
}

// syncSession (re)indexes one session. Reading and tokenizing happen without the
// lock; only the swap of old documents for new ones blocks searches.
func (e *SearchEngine) syncSession(transcripts *claudehistory.Transcripts, entry claudehistory.ClaudeHistoryEntry, isNew bool, result *SyncResult) {
	docs, err := e.prepareSession(transcripts, entry)
	if err != nil {
		// The previously indexed copy (if any) stays searchable; the next pass retries.
		result.Errors = append(result.Errors, err)
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if meta, ok := e.syncMetadata.Sessions[entry.ID]; ok {
		result.DocumentsRemoved += meta.DocCount
	}
	e.removeSessionsLocked(entry.ID)
	e.addPreparedLocked(docs)
	e.syncMetadata.Sessions[entry.ID] = &SessionIndexMetadata{
		SessionID:     entry.ID,
		UpdatedAt:     entry.UpdatedAt,
		MessageCount:  entry.MessageCount,
		LastIndexedAt: time.Now(),
		DocCount:      len(docs),
	}
	if isNew {
		result.SessionsAdded++
	} else {
		result.SessionsUpdated++
	}
	result.DocumentsAdded += len(docs)
}

// minIndexSaveInterval bounds disk writes: the index is hundreds of MB and an active
// session changes it on every pass.
const minIndexSaveInterval = 10 * time.Minute

// persist writes the index and its metadata. Both are skipped while a changed index is
// throttled, so the files on disk stay consistent and a restart just re-indexes the gap.
// A read lock is enough, so searches continue while it encodes.
func (e *SearchEngine) persist() error {
	if e.indexStore == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.dirty.Load() {
		if time.Since(e.lastSave) < minIndexSaveInterval {
			return nil
		}
		if err := e.indexStore.Save(e.index, e.docStore); err != nil {
			return err
		}
		e.dirty.Store(false)
		e.lastSave = time.Now()
	}
	return e.indexStore.SaveSyncMetadata(e.syncMetadata)
}

// finalizeCancelledIncrementalLocked keeps the totals true to the partial index
// and stamps LastIncrementalSync to record that a pass ran, even if cancelled
// before its first change.
// Must be called with lock held.
func (e *SearchEngine) finalizeCancelledIncrementalLocked() {
	e.refreshAggregatesLocked()
	e.syncMetadata.LastIncrementalSync = time.Now()
}

// refreshAggregatesLocked recomputes the session/document totals and the
// scorer's average document length from the sync metadata.
// Must be called with lock held.
func (e *SearchEngine) refreshAggregatesLocked() {
	e.syncMetadata.TotalSessions = len(e.syncMetadata.Sessions)
	totalDocs := 0
	for _, meta := range e.syncMetadata.Sessions {
		totalDocs += meta.DocCount
	}
	e.syncMetadata.TotalDocuments = totalDocs
	e.scorer.UpdateAvgDocLength()
}

// computeChangesLocked analyzes history vs index state to determine what changed.
// Must be called with lock held.
func (e *SearchEngine) computeChangesLocked(history *claudehistory.ClaudeSessionHistory) (
	added []claudehistory.ClaudeHistoryEntry,
	updated []claudehistory.ClaudeHistoryEntry,
	removed []string,
) {
	entries := history.GetAll()

	// Build map of current history
	historyMap := make(map[string]claudehistory.ClaudeHistoryEntry, len(entries))
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

// preparedDoc is a tokenized message waiting to be added to the index.
type preparedDoc struct {
	doc       *Document
	tokens    []string
	positions map[string][]int32
}

// prepareSession reads and tokenizes a session's messages. Needs no lock.
func (e *SearchEngine) prepareSession(transcripts *claudehistory.Transcripts, entry claudehistory.ClaudeHistoryEntry) ([]preparedDoc, error) {
	messages, err := transcripts.Messages(entry.ID)
	if err != nil {
		return nil, err
	}

	docs := make([]preparedDoc, 0, len(messages))
	for msgIdx, msg := range messages {
		tokens := e.tokenizer.Tokenize(msg.Content)
		if len(tokens) == 0 {
			continue
		}

		positions := make(map[string][]int32)
		for _, tp := range e.tokenizer.TokenizeWithPositions(msg.Content) {
			// #nosec G115 -- tp.Start is a byte offset into a single chat/scrollback message string; reaching 2^31 would require a single message over 2GB, far beyond any realistic message size
			positions[tp.Token] = append(positions[tp.Token], int32(tp.Start))
		}

		docs = append(docs, preparedDoc{
			doc: &Document{
				SessionID:    entry.ID,
				MessageIndex: msgIdx,
				MessageRole:  msg.Role,
				Content:      msg.Content,
				WordCount:    len(tokens),
				Timestamp:    msg.Timestamp,
			},
			tokens:    tokens,
			positions: positions,
		})
	}
	return docs, nil
}

// addPreparedLocked adds tokenized documents. Must be called with lock held.
func (e *SearchEngine) addPreparedLocked(docs []preparedDoc) {
	for _, d := range docs {
		e.index.AddDocument(e.docStore.Add(d.doc), d.tokens, d.positions)
	}
	if len(docs) > 0 {
		e.dirty.Store(true)
	}
}

// indexSessionLocked indexes all messages from a single session.
// Must be called with lock held.
func (e *SearchEngine) indexSessionLocked(transcripts *claudehistory.Transcripts, entry claudehistory.ClaudeHistoryEntry) (int, error) {
	docs, err := e.prepareSession(transcripts, entry)
	if err != nil {
		return 0, err
	}
	e.addPreparedLocked(docs)
	return len(docs), nil
}

// removeSessionsLocked removes all documents of the given sessions in one index pass.
// Must be called with lock held.
func (e *SearchEngine) removeSessionsLocked(sessionIDs ...string) {
	docIDs := make([]int32, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		docIDs = append(docIDs, e.docStore.GetDocIDsBySession(sessionID)...)
	}
	if len(docIDs) > 0 {
		e.index.RemoveDocuments(docIDs)
		e.dirty.Store(true)
	}
	for _, sessionID := range sessionIDs {
		e.docStore.RemoveBySession(sessionID)
	}
}

// ShouldRebuild returns true if a full rebuild is recommended.
// This happens when: no sync metadata, version mismatch, or corruption.
func (e *SearchEngine) ShouldRebuild() bool {
	e.ensureLoaded()
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
func (e *SearchEngine) buildIndexLocked(ctx context.Context, history *claudehistory.ClaudeSessionHistory, transcripts *claudehistory.Transcripts) error {
	// Clear existing index
	e.index.Clear()
	e.docStore.Clear()

	// Initialize fresh sync metadata
	e.syncMetadata = NewIndexSyncMetadata()
	e.syncMetadata.LastFullSync = time.Now()

	// Get all history entries
	entries := history.GetAll()

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			e.syncMetadata = nil // partial index: force a full rebuild next time
			return err
		}
		docCount, err := e.indexSessionLocked(transcripts, entry)
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

	e.refreshAggregatesLocked()

	// Persist if store is configured
	if e.indexStore != nil {
		if err := e.indexStore.Save(e.index, e.docStore); err != nil {
			return err
		}
		e.dirty.Store(false)
		if err := e.indexStore.SaveSyncMetadata(e.syncMetadata); err != nil {
			return err
		}
	}

	return nil
}

// LoadSyncMetadata loads persisted sync metadata from the index store.
func (e *SearchEngine) LoadSyncMetadata() error {
	e.ensureLoaded()
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
	e.ensureLoaded()
	if e.indexStore == nil || e.syncMetadata == nil {
		return nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.indexStore.SaveSyncMetadata(e.syncMetadata)
}

// GetSyncMetadata returns the current sync metadata (for inspection).
func (e *SearchEngine) GetSyncMetadata() *IndexSyncMetadata {
	e.ensureLoaded()
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.syncMetadata
}
