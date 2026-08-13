package search

import (
	"sync"
	"time"
)

// Document represents a single indexed document (message) with its metadata.
type Document struct {
	// SessionID is the ID of the conversation containing this message
	SessionID string
	// MessageIndex is the index of the message within the conversation
	MessageIndex int
	// MessageRole is the role of the message sender (user, assistant, system)
	MessageRole string
	// Content is the full text content of the message
	Content string
	// WordCount is the number of tokens in the message
	WordCount int
	// Timestamp is when the message was created
	Timestamp time.Time
}

// DocumentStore is a thread-safe store mapping document IDs to their metadata.
// It's used alongside the InvertedIndex to retrieve document details after search.
type DocumentStore struct {
	// Docs maps document IDs to document metadata
	Docs map[int32]*Document
	// SessionIndex maps session IDs to their document IDs for quick lookup
	SessionIndex map[string][]int32
	// NextDocID is the next available document ID
	NextDocID int32
	// mu protects concurrent access
	mu sync.RWMutex
}

// NewDocumentStore creates a new empty document store.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		Docs:         make(map[int32]*Document),
		SessionIndex: make(map[string][]int32),
		NextDocID:    0,
	}
}

// Add stores a new document and returns its assigned document ID.
func (ds *DocumentStore) Add(doc *Document) int32 {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	docID := ds.NextDocID
	ds.NextDocID++

	ds.Docs[docID] = doc
	ds.SessionIndex[doc.SessionID] = append(ds.SessionIndex[doc.SessionID], docID)

	return docID
}

// AddWithID stores a document with a specific document ID.
// Used when loading from persistence or when ID is known.
func (ds *DocumentStore) AddWithID(docID int32, doc *Document) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.Docs[docID] = doc
	ds.SessionIndex[doc.SessionID] = append(ds.SessionIndex[doc.SessionID], docID)

	// Update NextDocID if necessary
	if docID >= ds.NextDocID {
		ds.NextDocID = docID + 1
	}
}

// Get retrieves a document by its ID.
// Returns nil if not found.
func (ds *DocumentStore) Get(docID int32) *Document {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return ds.Docs[docID]
}

// GetBySession returns all documents belonging to a session.
func (ds *DocumentStore) GetBySession(sessionID string) []*Document {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	docIDs := ds.SessionIndex[sessionID]
	docs := make([]*Document, 0, len(docIDs))
	for _, docID := range docIDs {
		if doc := ds.Docs[docID]; doc != nil {
			docs = append(docs, doc)
		}
	}
	return docs
}

// GetDocIDsBySession returns all document IDs belonging to a session.
func (ds *DocumentStore) GetDocIDsBySession(sessionID string) []int32 {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return ds.SessionIndex[sessionID]
}

// Remove deletes a document from the store.
func (ds *DocumentStore) Remove(docID int32) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	doc := ds.Docs[docID]
	if doc == nil {
		return
	}

	// Remove from session index
	sessionDocs := ds.SessionIndex[doc.SessionID]
	for i, id := range sessionDocs {
		if id == docID {
			ds.SessionIndex[doc.SessionID] = append(sessionDocs[:i], sessionDocs[i+1:]...)
			break
		}
	}

	// Remove document
	delete(ds.Docs, docID)
}

// RemoveBySession removes all documents belonging to a session.
func (ds *DocumentStore) RemoveBySession(sessionID string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	docIDs := ds.SessionIndex[sessionID]
	for _, docID := range docIDs {
		delete(ds.Docs, docID)
	}
	delete(ds.SessionIndex, sessionID)
}

// Count returns the total number of documents.
func (ds *DocumentStore) Count() int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return len(ds.Docs)
}

// SessionCount returns the number of unique sessions.
func (ds *DocumentStore) SessionCount() int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return len(ds.SessionIndex)
}

// GetAllSessionIDs returns all session IDs in the store.
func (ds *DocumentStore) GetAllSessionIDs() []string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	sessionIDs := make([]string, 0, len(ds.SessionIndex))
	for sessionID := range ds.SessionIndex {
		sessionIDs = append(sessionIDs, sessionID)
	}
	return sessionIDs
}

// Clear removes all documents from the store.
func (ds *DocumentStore) Clear() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.Docs = make(map[int32]*Document)
	ds.SessionIndex = make(map[string][]int32)
	ds.NextDocID = 0
}

// HasSession returns true if the session exists in the store.
func (ds *DocumentStore) HasSession(sessionID string) bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	_, exists := ds.SessionIndex[sessionID]
	return exists
}

// GetStats returns statistics about the document store.
func (ds *DocumentStore) GetStats() DocumentStoreStats {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	totalWords := 0
	for _, doc := range ds.Docs {
		totalWords += doc.WordCount
	}

	return DocumentStoreStats{
		TotalDocuments: len(ds.Docs),
		TotalSessions:  len(ds.SessionIndex),
		TotalWords:     totalWords,
	}
}

// DocumentStoreStats contains statistics about the document store.
type DocumentStoreStats struct {
	TotalDocuments int
	TotalSessions  int
	TotalWords     int
}
