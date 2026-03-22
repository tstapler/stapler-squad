# Feature Plan: Full-Text Search for Claude History Browser

**Status**: Planning  
**Priority**: High  
**Epic**: History Browser Enhancement  
**Created**: 2025-12-05  
**Estimated Effort**: 3-4 weeks (1 developer)

---

## Table of Contents

1. [Epic Overview](#epic-overview)
2. [Requirements Analysis](#requirements-analysis)
3. [Architecture & Design](#architecture--design)
4. [Architecture Decision Records (ADRs)](#architecture-decision-records-adrs)
5. [Story Breakdown](#story-breakdown)
6. [Atomic Task Decomposition](#atomic-task-decomposition)
7. [Known Issues & Mitigation](#known-issues--mitigation)
8. [Testing Strategy](#testing-strategy)
9. [Context Preparation Guides](#context-preparation-guides)
10. [Dependencies & Sequencing](#dependencies--sequencing)

---

## Epic Overview

### User Value Proposition

**Problem Statement**: Users of Stapler Squad cannot efficiently find past conversations or specific exchanges buried in their Claude history. The current search only matches session names and project paths, forcing users to manually browse through potentially hundreds of conversations to locate relevant content.

**User Impact**:
- **Wasted Time**: Users spend 5-10 minutes per day manually browsing history
- **Lost Context**: Cannot quickly reference past solutions or discussions
- **Poor Knowledge Reuse**: Difficulty finding similar problems solved in the past
- **Frustration**: "I know Claude answered this before, but I can't find it"

**Solution**: Implement full-text search across all conversation content with context-aware snippet highlighting, enabling users to instantly find relevant discussions by searching message content, not just metadata.

**Value Delivered**:
- **Time Savings**: Reduce search time from 5-10 minutes to 5-10 seconds (>90% improvement)
- **Knowledge Discovery**: Surface relevant past conversations users forgot existed
- **Better Decisions**: Quick access to past reasoning and solutions
- **Improved Workflow**: Seamless history exploration without breaking concentration

### Success Metrics

**Quantitative KPIs**:
- **Search Response Time**: < 500ms for queries on 10,000+ messages
- **Search Precision**: > 90% of top 10 results are relevant to query
- **Snippet Quality**: Snippets contain query terms in 95%+ of results
- **User Adoption**: 60%+ of active users perform at least one content search per week

**Qualitative Indicators**:
- Users report "finding things faster" in feedback
- Reduction in support requests about finding old conversations
- Increased engagement with history browser feature

### Technical Requirements

**Performance Targets**:
- Index build time: < 30 seconds for 50,000 messages
- Index update latency: < 100ms per new message
- Memory footprint: < 50MB for index metadata
- Search query latency: < 500ms p95, < 200ms p50

**Scalability Targets**:
- Support 100,000+ messages per user
- Handle 50+ concurrent search queries (web UI multiple users)
- Index size < 20% of original message content size

**Quality Attributes**:
- **Availability**: 99.9% uptime (index corruption recovery)
- **Reliability**: Zero data loss during indexing
- **Maintainability**: Clear separation of indexing and search logic
- **Usability**: Intuitive snippet highlighting and ranking

---

## Requirements Analysis

### Functional Requirements

#### FR-1: Full-Text Message Search
**Priority**: MUST HAVE  
**User Story**: As a user, I want to search message content (not just session names) so I can find specific conversations.

**Acceptance Criteria**:
- [x] Search query matches against message content (user and assistant messages)
- [x] Search supports multi-word queries with implicit AND logic
- [x] Search returns ranked results based on relevance
- [x] Search handles partial word matches (prefix matching)
- [x] Search is case-insensitive

**EARS Notation**:
```
WHEN the user enters a search query in the history browser,
THE system SHALL search all message content across all conversations
AND return results ranked by relevance within 500ms.
```

#### FR-2: Context-Aware Snippets
**Priority**: MUST HAVE  
**User Story**: As a user, I want to see WHERE my search term appears in the conversation so I can quickly assess relevance.

**Acceptance Criteria**:
- [x] Each search result includes 1-3 snippets showing query term context
- [x] Snippets highlight the search term with visual emphasis (e.g., bold, background color)
- [x] Snippets include 20-30 words of surrounding context (before and after match)
- [x] Snippets show role (user vs assistant) and message timestamp
- [x] Long messages show multiple snippets if query appears multiple times

**EARS Notation**:
```
WHEN displaying search results,
THE system SHALL include text snippets showing the search term in context
WITH the search term highlighted
AND 20-30 words of surrounding text.
```

#### FR-3: Hybrid Search (Metadata + Content)
**Priority**: MUST HAVE  
**User Story**: As a user, I want search to consider both message content and session metadata for comprehensive results.

**Acceptance Criteria**:
- [x] Search considers session name, project path, model, AND message content
- [x] Results from session name matches rank higher than content matches (metadata boost)
- [x] Search results clearly indicate match source (name, project, or message content)
- [x] User can filter results by match source (e.g., "content only")

#### FR-4: Search Result Navigation
**Priority**: SHOULD HAVE  
**User Story**: As a user, I want to quickly navigate to the exact message where my search term appears.

**Acceptance Criteria**:
- [x] Clicking a snippet opens the conversation modal
- [x] Modal auto-scrolls to the message containing the search term
- [x] Matched term is highlighted in the full message view
- [x] User can navigate between multiple matches within same conversation (next/previous)

#### FR-5: Search Filtering
**Priority**: SHOULD HAVE  
**User Story**: As a user, I want to filter search results by date, model, or project to narrow down results.

**Acceptance Criteria**:
- [x] Search results can be filtered by date range (same as existing filters)
- [x] Search results can be filtered by model (e.g., only sonnet-4 conversations)
- [x] Search results can be filtered by project path
- [x] Filters apply instantly without re-executing search

#### FR-6: Search Performance Optimization
**Priority**: MUST HAVE  
**User Story**: As a power user with 10,000+ messages, I want search to be fast so I don't have to wait.

**Acceptance Criteria**:
- [x] Initial search response < 500ms for 10,000 messages
- [x] Subsequent searches < 200ms (cached results)
- [x] Index builds incrementally (doesn't block app startup)
- [x] Index updates within 100ms when new messages arrive

### Non-Functional Requirements (NFRs)

#### NFR-1: Performance (ISO/IEC 25010 - Performance Efficiency)

**Response Time**:
- Search queries SHALL complete in < 500ms (p95) and < 200ms (p50)
- Index updates SHALL complete in < 100ms per message
- Initial index build SHALL complete in < 30 seconds for 50,000 messages

**Throughput**:
- System SHALL support 50 concurrent search queries (web UI)
- System SHALL index 1,000 messages per second during initial build

**Resource Utilization**:
- Index memory footprint SHALL NOT exceed 50MB for metadata
- Index disk footprint SHALL NOT exceed 20% of message content size
- Search operations SHALL NOT block UI rendering

#### NFR-2: Scalability (ISO/IEC 25010)

**Data Volume Scalability**:
- System SHALL support 100,000+ messages per user
- System SHALL handle conversations with 10,000+ messages each
- System SHALL scale index size linearly with message count (O(n))

**User Scalability**:
- System SHALL support 10+ concurrent users searching (web UI multi-user mode)
- System SHALL maintain < 500ms search latency with 50 concurrent queries

#### NFR-3: Reliability (ISO/IEC 25010)

**Fault Tolerance**:
- Index corruption SHALL be detected automatically
- System SHALL rebuild index automatically if corruption detected
- Search SHALL gracefully degrade to metadata-only search if index unavailable

**Data Integrity**:
- Index SHALL guarantee zero message loss during indexing
- Index updates SHALL be atomic (all or nothing)
- System SHALL recover from crashes during index build without data loss

#### NFR-4: Maintainability (ISO/IEC 25010)

**Modularity**:
- Search indexing logic SHALL be isolated from history loading logic
- Search query logic SHALL be independent of UI rendering
- Index persistence SHALL use pluggable storage abstraction

**Testability**:
- All search functions SHALL have unit test coverage > 90%
- Integration tests SHALL verify end-to-end search workflow
- Performance tests SHALL validate < 500ms search latency

**Observability**:
- System SHALL log index build progress and timing
- System SHALL expose metrics: query latency, index size, cache hit rate
- System SHALL provide debug endpoint for index inspection

#### NFR-5: Security (ISO/IEC 25010)

**Access Control**:
- Search SHALL only return results for the authenticated user's conversations
- Index SHALL NOT expose conversation content across user boundaries
- Search queries SHALL be sanitized to prevent injection attacks

**Data Protection**:
- Index SHALL be stored with same permissions as conversation files
- Index SHALL NOT leak sensitive data in error messages
- Index SHALL be encrypted if conversation files are encrypted

#### NFR-6: Usability (ISO/IEC 25010 - User Experience)

**Learnability**:
- Search interface SHALL be consistent with existing search patterns (same input)
- Snippet highlighting SHALL be visually intuitive (no learning curve)
- Search results SHALL display without modal confusion (inline in existing list)

**Efficiency**:
- User SHALL complete search workflow in < 10 seconds (query + review + open conversation)
- Keyboard shortcuts SHALL support search without mouse (/, Enter, Escape)
- Search SHALL provide instant feedback (loading state, result count)

**Error Prevention**:
- System SHALL provide query suggestions for zero-result searches
- System SHALL show "No results" with helpful guidance (e.g., try different terms)
- System SHALL handle malformed queries gracefully (no crashes)

---

## Architecture & Design

### System Context (C4 Model - Level 1)

```
┌─────────────────────────────────────────────────────────────┐
│                     Stapler Squad User                        │
└──────────────────────┬──────────────────────────────────────┘
                       │ Searches conversation history
                       │ Views snippets and opens messages
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                   Stapler Squad Web UI                        │
│  (History Browser with Full-Text Search)                     │
└──────────────────────┬──────────────────────────────────────┘
                       │ gRPC/ConnectRPC API
                       ▼
┌─────────────────────────────────────────────────────────────┐
│              Stapler Squad Backend Services                   │
│  - SessionService (history + search RPCs)                    │
│  - SearchEngine (indexing + querying)                        │
└──────────────────────┬──────────────────────────────────────┘
                       │ Reads conversation files
                       ▼
┌─────────────────────────────────────────────────────────────┐
│            ~/.claude/projects/ (File System)                 │
│  - Conversation .jsonl files (source of truth)               │
│  - Search index files (inverted index + metadata)            │
└─────────────────────────────────────────────────────────────┘
```

### Container Architecture (C4 Model - Level 2)

```
┌───────────────────────────────────────────────────────────────┐
│                     Web UI (Next.js/React)                     │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ History Page (history/page.tsx)                         │  │
│  │  - Search input component                               │  │
│  │  - Results list with snippets                           │  │
│  │  - Message modal with highlight                         │  │
│  └─────────────────────────────────────────────────────────┘  │
└──────────────────────┬────────────────────────────────────────┘
                       │ SearchClaudeHistory RPC
                       │ GetClaudeHistoryMessages RPC
                       ▼
┌───────────────────────────────────────────────────────────────┐
│              Backend Go Services (server/)                     │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ SessionService (services/session_service.go)            │  │
│  │  - SearchClaudeHistory() - NEW RPC                      │  │
│  │  - ListClaudeHistory() - existing                       │  │
│  └───────────────────┬─────────────────────────────────────┘  │
│                      │                                         │
│  ┌───────────────────▼─────────────────────────────────────┐  │
│  │ SearchEngine (session/search_engine.go) - NEW           │  │
│  │  - BuildIndex()  - parses .jsonl, builds inverted index │  │
│  │  - Search()      - queries index, returns ranked results│  │
│  │  - GenerateSnippets() - extracts highlighted snippets   │  │
│  └───────────────────┬─────────────────────────────────────┘  │
│                      │                                         │
│  ┌───────────────────▼─────────────────────────────────────┐  │
│  │ IndexStore (session/search_index.go) - NEW              │  │
│  │  - Save/Load index to disk                               │  │
│  │  - Inverted index: term -> [docID, positions]           │  │
│  │  - Document store: docID -> (sessionID, messageIdx)     │  │
│  └───────────────────┬─────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
                       │
                       ▼
┌───────────────────────────────────────────────────────────────┐
│            File System (~/.claude/)                            │
│  - projects/{project}/*.jsonl  (conversation files)           │
│  - search_index/                                               │
│     - inverted_index.gob       (term -> postings list)        │
│     - doc_metadata.gob         (doc -> session/message map)   │
│     - index_version.json       (index version + timestamp)    │
└───────────────────────────────────────────────────────────────┘
```

### Component Design (C4 Model - Level 3)

#### SearchEngine Component

```go
// session/search_engine.go

type SearchEngine struct {
    index        *InvertedIndex
    docStore     *DocumentStore
    snippetGen   *SnippetGenerator
    tokenizer    *Tokenizer
    mu           sync.RWMutex
}

// Core Operations
func (e *SearchEngine) BuildIndex(historyEntries []ClaudeHistoryEntry) error
func (e *SearchEngine) Search(query string, opts SearchOptions) (*SearchResults, error)
func (e *SearchEngine) UpdateIndex(newMessages []ClaudeConversationMessage) error

// SearchResults structure
type SearchResults struct {
    Results      []SearchResult
    TotalMatches int
    QueryTime    time.Duration
}

type SearchResult struct {
    SessionID    string
    SessionName  string
    MessageIndex int
    Score        float64
    Snippets     []Snippet
    Metadata     ResultMetadata
}

type Snippet struct {
    Text           string
    HighlightStart int
    HighlightEnd   int
    MessageRole    string
    MessageTime    time.Time
}
```

#### InvertedIndex Structure

```go
// session/search_index.go

type InvertedIndex struct {
    // term -> list of documents containing term
    Index map[string]*PostingsList
    // Document frequency for TF-IDF scoring
    DocFrequency map[string]int
    TotalDocs    int
}

type PostingsList struct {
    DocIDs    []int32          // Document IDs containing this term
    Positions [][]int32        // Position within each document
    Frequency []int32          // Term frequency per document
}

type DocumentStore struct {
    // docID -> document metadata
    Docs map[int32]*Document
}

type Document struct {
    SessionID    string
    MessageIndex int
    MessageRole  string
    Content      string
    WordCount    int
    Timestamp    time.Time
}
```

### Data Flow Diagrams

#### Search Query Flow

```
User enters query → Web UI → Backend RPC → SearchEngine
                                               ↓
                                         Tokenize query
                                               ↓
                                    Look up terms in index
                                               ↓
                                    Compute relevance scores
                                               ↓
                                    Rank and paginate results
                                               ↓
                                    Generate snippets
                                               ↓
                                    Return to Web UI
                                               ↓
                                    Display results with highlights
```

#### Index Build Flow

```
App Startup → Load history.jsonl → Parse conversations
                                          ↓
                                    For each message:
                                     - Tokenize content
                                     - Build postings list
                                     - Track document metadata
                                          ↓
                                    Save index to disk
                                          ↓
                                    Index ready for queries
```

#### Index Update Flow (Incremental)

```
New message added → Detect change (polling/watch)
                                ↓
                         Parse new message
                                ↓
                         Update index in-memory
                                ↓
                         Persist delta to disk
                                ↓
                         Index ready for queries
```

---

## Architecture Decision Records (ADRs)

### ADR-1: Use In-Memory Inverted Index with Disk Persistence

**Status**: Accepted  
**Date**: 2025-12-05  
**Deciders**: Engineering Team

**Context**:
We need to decide on a search indexing strategy for Stapler Squad history. Options include:
1. SQLite FTS5 (Full-Text Search extension)
2. External search engine (Elasticsearch, MeiliSearch)
3. In-memory inverted index with disk persistence
4. Linear scan with caching

**Decision**:
We will implement an in-memory inverted index with Gob-encoded disk persistence.

**Rationale**:

**Pros**:
- **Performance**: In-memory index provides < 50ms query latency for 10,000+ messages
- **Simplicity**: No external dependencies, pure Go implementation
- **Control**: Full control over ranking algorithm, snippet generation, and scoring
- **Portability**: Works anywhere Go runs, no external services required
- **Atomicity**: Gob encoding provides atomic write guarantees
- **Startup Speed**: Index loads from disk in < 500ms (lazy loading possible)

**Cons**:
- **Memory Footprint**: ~10-20MB for 50,000 messages (acceptable for desktop app)
- **Implementation Effort**: Higher than using existing FTS library
- **Index Maintenance**: Must implement incremental updates ourselves

**Alternatives Considered**:

**1. SQLite FTS5**:
- Pros: Battle-tested, efficient, SQL integration
- Cons: Requires SQL migration, less control over ranking, snippet generation complexity
- Rejected because: We already have file-based history, adding SQL adds complexity

**2. Elasticsearch/MeiliSearch**:
- Pros: Best-in-class search, scalable
- Cons: Requires external service, deployment complexity, overkill for desktop app
- Rejected because: Not suitable for single-user desktop application

**3. Linear Scan**:
- Pros: Simplest implementation
- Cons: O(n) complexity, > 2 seconds for 10,000 messages
- Rejected because: Unacceptable performance for power users

**Consequences**:
- We control ranking and scoring algorithms completely
- Index must be rebuilt if format changes (version migrations)
- Memory usage scales linearly with message count (needs monitoring)
- Faster query performance than SQL or external services for our scale

---

### ADR-2: Use TF-IDF Scoring with BM25 Variant

**Status**: Accepted  
**Date**: 2025-12-05  
**Deciders**: Engineering Team

**Context**:
We need a relevance scoring algorithm for ranking search results. Options:
1. Simple term frequency (TF)
2. TF-IDF (Term Frequency-Inverse Document Frequency)
3. BM25 (Best Match 25 - improved TF-IDF)
4. Vector embeddings + cosine similarity

**Decision**:
We will implement BM25 scoring, a probabilistic ranking function that improves upon TF-IDF.

**Rationale**:

**BM25 Formula**:
```
score(D, Q) = Σ IDF(qi) * (f(qi, D) * (k1 + 1)) / (f(qi, D) + k1 * (1 - b + b * |D| / avgdl))

Where:
- D = document
- Q = query
- qi = query term i
- f(qi, D) = term frequency of qi in D
- |D| = document length
- avgdl = average document length across corpus
- k1 = term frequency saturation parameter (default: 1.5)
- b = length normalization parameter (default: 0.75)
- IDF(qi) = log((N - df(qi) + 0.5) / (df(qi) + 0.5))
```

**Pros**:
- **Non-linear TF**: Handles repeated terms better than linear TF-IDF
- **Length Normalization**: Prevents long documents from dominating results
- **Tunable Parameters**: k1 and b can be adjusted for domain-specific tuning
- **Industry Standard**: Used by Elasticsearch, Lucene, and major search engines
- **Proven Performance**: Consistently outperforms TF-IDF in IR benchmarks

**Cons**:
- Slightly more complex than TF-IDF
- Requires computing average document length

**Alternatives Considered**:

**1. Simple TF-IDF**:
- Pros: Simpler math, easier to understand
- Cons: Poor handling of term repetition, length bias
- Rejected because: BM25 is only marginally more complex with significant quality gains

**2. Vector Embeddings**:
- Pros: Semantic search, understands context
- Cons: Requires ML model, high latency (>100ms), complex infrastructure
- Rejected because: Overkill for exact term matching, adds complexity

**Consequences**:
- We must compute and store average document length
- Index must track term frequency per document
- Scoring parameters (k1, b) may need tuning based on user feedback

---

### ADR-3: Store Index in Gob Format at ~/.claude/search_index/

**Status**: Accepted  
**Date**: 2025-12-05  
**Deciders**: Engineering Team

**Context**:
We need to persist the search index to disk for fast startup. Options:
1. JSON encoding
2. Gob (Go binary) encoding
3. Protocol Buffers
4. SQLite database
5. Custom binary format

**Decision**:
We will use Go's `encoding/gob` package and store index files at `~/.claude/search_index/`.

**Rationale**:

**Pros**:
- **Performance**: Gob encoding is 5-10x faster than JSON for Go structs
- **Simplicity**: Native Go support, no external dependencies
- **Type Safety**: Preserves Go type information
- **Atomic Writes**: Can write to temp file and rename for atomicity
- **Compactness**: 30-50% smaller than JSON

**Cons**:
- **Go-Only**: Not human-readable, Go-specific format
- **Version Sensitivity**: Requires careful handling of schema changes

**Storage Layout**:
```
~/.claude/
  └── search_index/
      ├── inverted_index.gob       # Term -> postings list
      ├── doc_metadata.gob         # Document store
      ├── index_version.json       # Version + timestamp
      └── index.lock               # Flock for concurrent access
```

**Alternatives Considered**:

**1. JSON**:
- Pros: Human-readable, language-agnostic
- Cons: 5-10x slower, 2x larger files
- Rejected because: Performance is critical for large indexes

**2. Protocol Buffers**:
- Pros: Cross-language, compact, schema evolution
- Cons: Requires schema definition, code generation
- Rejected because: Overkill for Go-only internal format

**3. SQLite**:
- Pros: ACID properties, query capabilities
- Cons: Adds SQL complexity, slower than in-memory
- Rejected because: We already have in-memory index, just need persistence

**Consequences**:
- Index files are not human-readable (use debug tools)
- Must implement versioning for schema migrations
- Atomic writes require temp file + rename pattern

---

### ADR-4: Hybrid Search (Metadata Boost + Content Search)

**Status**: Accepted  
**Date**: 2025-12-05  
**Deciders**: Engineering Team

**Context**:
Users search for both session metadata (names, projects) and message content. We need to decide how to combine these search modes.

**Decision**:
We will implement hybrid search with metadata boosting: search results combine metadata matches (session name, project) and content matches, with metadata matches receiving a 2x score boost.

**Rationale**:

**Scoring Formula**:
```
final_score = (metadata_match ? 2.0 : 1.0) * bm25_score + metadata_exact_boost
```

**Metadata Exact Boost**:
- Session name exact match: +10.0
- Project path exact match: +5.0
- Model exact match: +2.0

**Pros**:
- **User Intent**: Session names often more relevant than random content matches
- **Backwards Compatible**: Existing name/project search still works
- **Comprehensive**: Single search covers all dimensions
- **Tunable**: Boost factors can be adjusted based on feedback

**Cons**:
- More complex scoring logic
- Requires metadata indexing in addition to content

**Alternatives Considered**:

**1. Content-Only Search**:
- Pros: Simpler implementation
- Cons: Users lose ability to find sessions by name
- Rejected because: Name search is critical workflow

**2. Separate Search Modes**:
- Pros: Clear separation of concerns
- Cons: Confusing UX (which search to use?), fragmented results
- Rejected because: Users want "Google-style" search that finds everything

**Consequences**:
- Metadata must be indexed alongside content
- Search results must indicate match source (name vs content)
- UI should show metadata matches first (pre-sorted)

---

### ADR-5: Generate Snippets on Query (Not Pre-Indexed)

**Status**: Accepted  
**Date**: 2025-12-05  
**Deciders**: Engineering Team

**Context**:
Search results need context snippets showing where query terms appear. We need to decide when to generate snippets.

**Decision**:
We will generate snippets dynamically at query time by re-scanning matched messages, not pre-generating or storing snippets in the index.

**Rationale**:

**Pros**:
- **Flexibility**: Snippets adapt to any query without re-indexing
- **Compactness**: Index size stays small (no snippet storage)
- **Freshness**: Snippets always reflect current message content
- **Performance**: Snippet generation is fast (< 10ms per message)

**Cons**:
- Query latency increases by 10-50ms for snippet generation
- Must re-read message content from disk (mitigated by OS page cache)

**Implementation**:
```go
func GenerateSnippet(message string, query string, contextWords int) Snippet {
    // 1. Find query term positions in message
    // 2. Extract 20-30 words before and after first match
    // 3. Highlight query terms with start/end positions
    // 4. Truncate snippet to max length (200 chars)
}
```

**Alternatives Considered**:

**1. Pre-Generate All Possible Snippets**:
- Pros: Fastest query time
- Cons: Impossible (infinite possible queries), huge storage
- Rejected because: Not feasible

**2. Store Fixed Snippets (e.g., first 100 words)**:
- Pros: Simple, fast
- Cons: May not contain query term, poor user experience
- Rejected because: Defeats purpose of contextual snippets

**Consequences**:
- Search latency includes snippet generation time (budget: 50ms)
- Snippet generation must be optimized for performance
- Cache snippet generation for repeat queries on same results

---

## Story Breakdown

### Epic: Full-Text Search for Claude History Browser

**Total Estimated Effort**: 12-15 developer-days (3-4 weeks for 1 developer)

---

### Story 1: Backend Search Engine Foundation

**Priority**: MUST HAVE  
**Effort**: 4-5 days  
**Dependencies**: None  
**Value**: Enables all search functionality

**User Story**:
As a backend engineer, I want a robust search engine with inverted index so that I can efficiently query message content.

**Acceptance Criteria**:
- [x] `SearchEngine` struct implemented in `session/search_engine.go`
- [x] `InvertedIndex` with term -> postings list mapping
- [x] `DocumentStore` with docID -> message metadata
- [x] `Tokenizer` with lowercasing, stemming, stop word removal
- [x] `BuildIndex()` method parses history and creates index
- [x] `Search()` method queries index with BM25 scoring
- [x] Index persists to disk using Gob encoding
- [x] Index loads from disk on startup (< 500ms for 50,000 messages)
- [x] Unit tests for tokenization, indexing, and search (>90% coverage)
- [x] Benchmark tests for index build (< 30s for 50,000 messages)

**Technical Notes**:
- Use Porter Stemmer algorithm for word stemming
- Stop words: "the", "a", "an", "and", "or", "but", "is", "are", "was", "were"
- Tokenization: split on whitespace and punctuation, lowercase, stem
- BM25 parameters: k1=1.5, b=0.75 (standard values)

**Definition of Done**:
- Code reviewed and merged
- Unit tests pass with >90% coverage
- Benchmarks meet performance targets
- Documentation updated with API examples

---

### Story 2: Snippet Generation and Highlighting

**Priority**: MUST HAVE  
**Effort**: 2-3 days  
**Dependencies**: Story 1 (Search Engine Foundation)  
**Value**: Provides contextual search results

**User Story**:
As a user, I want to see highlighted snippets showing where my search term appears so that I can quickly assess relevance.

**Acceptance Criteria**:
- [x] `SnippetGenerator` struct with configurable context window
- [x] `GenerateSnippet()` extracts 20-30 words around first match
- [x] Snippet includes highlight start/end positions
- [x] Long messages with multiple matches show 2-3 snippets
- [x] Snippet respects word boundaries (no mid-word cuts)
- [x] Snippet includes message role and timestamp
- [x] Unit tests for snippet extraction with various edge cases
- [x] Performance: < 10ms per snippet generation

**Technical Notes**:
- Context window: 30 words before and after match
- Max snippet length: 200 characters
- Multiple matches: show first 3 occurrences
- Handle edge cases: match at start/end of message

**Definition of Done**:
- Code reviewed and merged
- Unit tests cover edge cases (empty message, match at boundary, long message)
- Performance benchmark confirms < 10ms per snippet
- Documentation includes snippet examples

---

### Story 3: Backend gRPC API Integration

**Priority**: MUST HAVE  
**Effort**: 2 days  
**Dependencies**: Story 1, Story 2  
**Value**: Exposes search to web UI

**User Story**:
As a frontend developer, I want a gRPC API for full-text search so that I can integrate search into the web UI.

**Acceptance Criteria**:
- [x] `SearchClaudeHistory` RPC defined in `proto/session/v1/session.proto`
- [x] `SearchClaudeHistoryRequest` message with query, filters, pagination
- [x] `SearchClaudeHistoryResponse` message with results, snippets, metadata
- [x] `SessionService.SearchClaudeHistory()` implementation in `server/services/session_service.go`
- [x] Search integrates with existing history cache
- [x] Search supports pagination (limit, offset)
- [x] Search respects existing filters (date, model, project)
- [x] Error handling for malformed queries
- [x] Integration tests for search RPC

**Protobuf Schema**:
```protobuf
message SearchClaudeHistoryRequest {
  string query = 1;                      // Search query
  optional string project = 2;           // Filter by project
  optional string model = 3;             // Filter by model
  google.protobuf.Timestamp start_time = 4; // Date range start
  google.protobuf.Timestamp end_time = 5;   // Date range end
  int32 limit = 6;                       // Pagination limit
  int32 offset = 7;                      // Pagination offset
}

message SearchClaudeHistoryResponse {
  repeated SearchResult results = 1;
  int32 total_matches = 2;
  int64 query_time_ms = 3;
}

message SearchResult {
  string session_id = 1;
  string session_name = 2;
  string project = 3;
  int32 message_index = 4;
  float score = 5;
  repeated Snippet snippets = 6;
  ResultMetadata metadata = 7;
}

message Snippet {
  string text = 1;
  int32 highlight_start = 2;
  int32 highlight_end = 3;
  string message_role = 4;
  google.protobuf.Timestamp message_time = 5;
}

message ResultMetadata {
  bool is_metadata_match = 1;  // True if match is in session name/project
  string match_source = 2;     // "session_name", "project", "message_content"
}
```

**Definition of Done**:
- Protobuf schema merged and code generated
- RPC implementation tested with integration tests
- API documentation updated
- ConnectRPC client code generated for web UI

---

### Story 4: Frontend Search UI Integration

**Priority**: MUST HAVE  
**Effort**: 3-4 days  
**Dependencies**: Story 3 (Backend API)  
**Value**: Delivers user-facing search capability

**User Story**:
As a user, I want to search message content from the history browser so that I can find specific conversations.

**Acceptance Criteria**:
- [x] Search input component in History page (`web-app/src/app/history/page.tsx`)
- [x] Search triggers API call to `SearchClaudeHistory` RPC
- [x] Search results display in existing entry list (replace or augment)
- [x] Each result shows snippets with highlighted search terms
- [x] Clicking a result opens message modal and scrolls to matched message
- [x] Search query persists in localStorage (same as existing filters)
- [x] Loading state shown during search
- [x] Empty state for "No results found" with helpful message
- [x] Keyboard shortcuts: `/` focuses search, `Escape` clears search

**UI Mockup** (Pseudo-React):
```tsx
<div className={styles.searchContainer}>
  <input
    type="text"
    placeholder="Search conversations... (Press /)"
    value={searchQuery}
    onChange={(e) => setSearchQuery(e.target.value)}
    onKeyDown={(e) => e.key === "Enter" && handleSearch()}
  />
  <button onClick={handleSearch} disabled={searching}>
    {searching ? "Searching..." : "Search"}
  </button>
</div>

{searchResults.length > 0 && (
  <div className={styles.searchResults}>
    <h3>Search Results ({searchResults.length})</h3>
    {searchResults.map((result) => (
      <SearchResultCard
        key={result.sessionId + result.messageIndex}
        result={result}
        onClick={() => openMessageModal(result)}
      />
    ))}
  </div>
)}
```

**Definition of Done**:
- UI implemented and styled
- Search integrates with existing history page layout
- User testing confirms intuitive workflow
- Keyboard shortcuts work as expected

---

### Story 5: Snippet Highlighting in Message Modal

**Priority**: SHOULD HAVE  
**Effort**: 2 days  
**Dependencies**: Story 4 (Frontend Search UI)  
**Value**: Improves user navigation to exact match

**User Story**:
As a user, I want the search term highlighted in the message modal so that I can see the exact match in context.

**Acceptance Criteria**:
- [x] Message modal auto-scrolls to matched message when opened from search
- [x] Matched search term highlighted with background color in full message
- [x] Multiple matches in same message have "Next/Previous" navigation buttons
- [x] Highlight persists until modal closed or new search performed
- [x] Highlight styling consistent with snippet highlight (same color)

**Technical Notes**:
- Use `scrollIntoView()` to auto-scroll to matched message
- Highlight search term with `<mark>` tag or custom span with background
- Track current match index for next/previous navigation

**Definition of Done**:
- Auto-scroll works reliably
- Highlight visible and intuitive
- Next/previous navigation functional
- User testing confirms improved workflow

---

### Story 6: Incremental Index Updates

**Priority**: SHOULD HAVE  
**Effort**: 2-3 days  
**Dependencies**: Story 1 (Search Engine Foundation)  
**Value**: Keeps search index up-to-date without full rebuilds

**User Story**:
As a power user, I want new conversations to be searchable immediately so that I don't have to wait for index rebuilds.

**Acceptance Criteria**:
- [x] `UpdateIndex()` method adds new messages to existing index
- [x] Index update triggered when new conversation detected
- [x] Index update completes in < 100ms per message
- [x] Index persisted after each update (atomic writes)
- [x] No search downtime during index updates (read-write lock)
- [x] Background polling checks for new conversations every 30 seconds
- [x] Manual "Refresh Index" button in UI

**Technical Notes**:
- Use `sync.RWMutex` for concurrent read/write access
- Persist index delta (not full rebuild) for performance
- Detect new conversations by comparing file modification times

**Definition of Done**:
- Incremental updates work without blocking searches
- Index stays consistent across updates
- Performance benchmarks meet < 100ms target
- Background polling doesn't impact UI performance

---

### Story 7: Search Performance Optimization

**Priority**: SHOULD HAVE  
**Effort**: 2 days  
**Dependencies**: Story 1, Story 6  
**Value**: Ensures fast search for large history

**User Story**:
As a power user with 50,000+ messages, I want search to be fast so that I don't have to wait.

**Acceptance Criteria**:
- [x] Search query latency < 500ms (p95) for 50,000 messages
- [x] Index build time < 30 seconds for 50,000 messages
- [x] Index memory footprint < 50MB
- [x] Query result caching for repeated searches (LRU cache, 100 entries)
- [x] Index compression (if footprint > 50MB)
- [x] Profiling and benchmarks validate performance targets

**Optimizations**:
- Use `sync.Map` for concurrent index access
- Cache BM25 scores for common terms
- Compress postings lists with delta encoding
- Limit snippet generation to top 20 results

**Definition of Done**:
- Benchmark tests confirm < 500ms search latency
- Profiling shows no performance bottlenecks
- Memory usage stays under 50MB for 50,000 messages
- Documentation includes performance characteristics

---

## Atomic Task Decomposition

### Story 1: Backend Search Engine Foundation

#### Task 1.1: Implement Tokenizer
**Estimated Time**: 3 hours  
**Files**: `session/tokenizer.go`, `session/tokenizer_test.go`  
**Completion Criteria**: Unit tests pass with >95% coverage

**Implementation Approach**:
```go
package session

import (
    "strings"
    "unicode"
)

type Tokenizer struct {
    stopWords map[string]bool
    stemmer   *PorterStemmer
}

func NewTokenizer() *Tokenizer {
    stopWords := map[string]bool{
        "the": true, "a": true, "an": true, "and": true,
        "or": true, "but": true, "is": true, "are": true,
        "was": true, "were": true, "be": true, "been": true,
    }
    return &Tokenizer{
        stopWords: stopWords,
        stemmer:   NewPorterStemmer(),
    }
}

func (t *Tokenizer) Tokenize(text string) []string {
    // 1. Lowercase
    text = strings.ToLower(text)
    
    // 2. Split on non-alphanumeric
    words := strings.FieldsFunc(text, func(r rune) bool {
        return !unicode.IsLetter(r) && !unicode.IsNumber(r)
    })
    
    // 3. Remove stop words and stem
    tokens := make([]string, 0, len(words))
    for _, word := range words {
        if len(word) < 2 || t.stopWords[word] {
            continue
        }
        tokens = append(tokens, t.stemmer.Stem(word))
    }
    
    return tokens
}
```

**Test Cases**:
- Empty string → empty slice
- Single word → single token
- Multiple words → multiple tokens
- Stop words removed → "the cat" → ["cat"]
- Stemming applied → "running" → ["run"]
- Punctuation removed → "hello, world!" → ["hello", "world"]

---

#### Task 1.2: Implement InvertedIndex
**Estimated Time**: 4 hours  
**Files**: `session/inverted_index.go`, `session/inverted_index_test.go`  
**Completion Criteria**: Index can store/retrieve postings lists

**Implementation Approach**:
```go
package session

type InvertedIndex struct {
    Index        map[string]*PostingsList
    DocFrequency map[string]int
    TotalDocs    int
    mu           sync.RWMutex
}

type PostingsList struct {
    DocIDs    []int32
    Positions [][]int32
    Frequency []int32
}

func NewInvertedIndex() *InvertedIndex {
    return &InvertedIndex{
        Index:        make(map[string]*PostingsList),
        DocFrequency: make(map[string]int),
    }
}

func (idx *InvertedIndex) AddDocument(docID int32, tokens []string) {
    idx.mu.Lock()
    defer idx.mu.Unlock()
    
    // Track term positions
    termPositions := make(map[string][]int32)
    for pos, token := range tokens {
        termPositions[token] = append(termPositions[token], int32(pos))
    }
    
    // Update postings lists
    for term, positions := range termPositions {
        if idx.Index[term] == nil {
            idx.Index[term] = &PostingsList{}
            idx.DocFrequency[term] = 0
        }
        
        postings := idx.Index[term]
        postings.DocIDs = append(postings.DocIDs, docID)
        postings.Positions = append(postings.Positions, positions)
        postings.Frequency = append(postings.Frequency, int32(len(positions)))
        idx.DocFrequency[term]++
    }
    
    idx.TotalDocs++
}

func (idx *InvertedIndex) Search(term string) *PostingsList {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    return idx.Index[term]
}
```

**Test Cases**:
- Add single document → index contains doc
- Add multiple documents → postings list grows
- Search non-existent term → nil
- Concurrent reads → no race conditions

---

#### Task 1.3: Implement BM25 Scoring
**Estimated Time**: 3 hours  
**Files**: `session/bm25.go`, `session/bm25_test.go`  
**Completion Criteria**: Scoring matches BM25 formula

**Implementation Approach**:
```go
package session

import "math"

const (
    K1 = 1.5
    B  = 0.75
)

type BM25Scorer struct {
    index      *InvertedIndex
    docStore   *DocumentStore
    avgDocLen  float64
}

func (s *BM25Scorer) Score(query []string, docID int32) float64 {
    doc := s.docStore.Get(docID)
    if doc == nil {
        return 0.0
    }
    
    score := 0.0
    docLen := float64(doc.WordCount)
    
    for _, term := range query {
        postings := s.index.Search(term)
        if postings == nil {
            continue
        }
        
        // Find term frequency in this document
        tf := 0.0
        for i, did := range postings.DocIDs {
            if did == docID {
                tf = float64(postings.Frequency[i])
                break
            }
        }
        
        if tf == 0 {
            continue
        }
        
        // IDF calculation
        N := float64(s.index.TotalDocs)
        df := float64(s.index.DocFrequency[term])
        idf := math.Log((N - df + 0.5) / (df + 0.5))
        
        // BM25 formula
        numerator := tf * (K1 + 1)
        denominator := tf + K1*(1-B+B*(docLen/s.avgDocLen))
        score += idf * (numerator / denominator)
    }
    
    return score
}
```

**Test Cases**:
- Single term match → positive score
- Multiple term match → higher score than single
- Repeated term → non-linear score increase (BM25 property)
- Long document vs short → length normalization works

---

#### Task 1.4: Implement SearchEngine.BuildIndex()
**Estimated Time**: 4 hours  
**Files**: `session/search_engine.go`, `session/search_engine_test.go`  
**Completion Criteria**: Index builds from ClaudeHistoryEntry list

**Implementation Approach**:
```go
package session

type SearchEngine struct {
    index     *InvertedIndex
    docStore  *DocumentStore
    tokenizer *Tokenizer
    scorer    *BM25Scorer
    mu        sync.RWMutex
}

func NewSearchEngine() *SearchEngine {
    idx := NewInvertedIndex()
    docStore := NewDocumentStore()
    tokenizer := NewTokenizer()
    
    return &SearchEngine{
        index:     idx,
        docStore:  docStore,
        tokenizer: tokenizer,
    }
}

func (e *SearchEngine) BuildIndex(history *ClaudeSessionHistory) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    
    docID := int32(0)
    totalWordCount := 0
    
    // Index all conversations
    entries := history.GetAll()
    for _, entry := range entries {
        // Load messages for this conversation
        messages, err := history.GetMessagesFromConversationFile(entry.ID)
        if err != nil {
            continue
        }
        
        // Index each message
        for msgIdx, msg := range messages {
            tokens := e.tokenizer.Tokenize(msg.Content)
            if len(tokens) == 0 {
                continue
            }
            
            // Add to index
            e.index.AddDocument(docID, tokens)
            
            // Store document metadata
            e.docStore.Add(docID, &Document{
                SessionID:    entry.ID,
                MessageIndex: msgIdx,
                MessageRole:  msg.Role,
                Content:      msg.Content,
                WordCount:    len(tokens),
                Timestamp:    msg.Timestamp,
            })
            
            totalWordCount += len(tokens)
            docID++
        }
    }
    
    // Initialize scorer with average document length
    avgDocLen := float64(totalWordCount) / float64(docID)
    e.scorer = &BM25Scorer{
        index:     e.index,
        docStore:  e.docStore,
        avgDocLen: avgDocLen,
    }
    
    return nil
}
```

**Test Cases**:
- Empty history → empty index
- Single conversation → index contains messages
- Multiple conversations → all indexed
- Invalid conversation file → skipped gracefully

---

#### Task 1.5: Implement SearchEngine.Search()
**Estimated Time**: 3 hours  
**Files**: Update `session/search_engine.go`, add tests  
**Completion Criteria**: Search returns ranked results

**Implementation Approach**:
```go
type SearchOptions struct {
    Limit  int
    Offset int
}

type SearchResults struct {
    Results      []SearchResult
    TotalMatches int
    QueryTime    time.Duration
}

type SearchResult struct {
    SessionID    string
    SessionName  string
    MessageIndex int
    Score        float64
    Document     *Document
}

func (e *SearchEngine) Search(query string, opts SearchOptions) (*SearchResults, error) {
    startTime := time.Now()
    
    e.mu.RLock()
    defer e.mu.RUnlock()
    
    // Tokenize query
    tokens := e.tokenizer.Tokenize(query)
    if len(tokens) == 0 {
        return &SearchResults{}, nil
    }
    
    // Find all documents containing any query term
    candidateDocs := make(map[int32]bool)
    for _, term := range tokens {
        postings := e.index.Search(term)
        if postings != nil {
            for _, docID := range postings.DocIDs {
                candidateDocs[docID] = true
            }
        }
    }
    
    // Score each candidate
    results := make([]SearchResult, 0, len(candidateDocs))
    for docID := range candidateDocs {
        score := e.scorer.Score(tokens, docID)
        if score > 0 {
            doc := e.docStore.Get(docID)
            results = append(results, SearchResult{
                SessionID:    doc.SessionID,
                MessageIndex: doc.MessageIndex,
                Score:        score,
                Document:     doc,
            })
        }
    }
    
    // Sort by score descending
    sort.Slice(results, func(i, j int) bool {
        return results[i].Score > results[j].Score
    })
    
    // Apply pagination
    totalMatches := len(results)
    if opts.Offset > 0 && opts.Offset < len(results) {
        results = results[opts.Offset:]
    }
    if opts.Limit > 0 && opts.Limit < len(results) {
        results = results[:opts.Limit]
    }
    
    return &SearchResults{
        Results:      results,
        TotalMatches: totalMatches,
        QueryTime:    time.Since(startTime),
    }, nil
}
```

**Test Cases**:
- Single term query → returns matches
- Multi-term query → returns documents with all terms ranked higher
- No matches → empty results
- Pagination works correctly
- Query time < 500ms for 10,000 messages

---

#### Task 1.6: Implement Index Persistence (Gob)
**Estimated Time**: 3 hours  
**Files**: `session/index_store.go`, `session/index_store_test.go`  
**Completion Criteria**: Index saves/loads from disk

**Implementation Approach**:
```go
package session

import (
    "encoding/gob"
    "os"
    "path/filepath"
)

type IndexStore struct {
    indexDir string
}

func NewIndexStore() (*IndexStore, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return nil, err
    }
    
    indexDir := filepath.Join(home, ".claude", "search_index")
    if err := os.MkdirAll(indexDir, 0755); err != nil {
        return nil, err
    }
    
    return &IndexStore{indexDir: indexDir}, nil
}

func (s *IndexStore) Save(index *InvertedIndex, docStore *DocumentStore) error {
    // Save to temp files first (atomic writes)
    indexPath := filepath.Join(s.indexDir, "inverted_index.gob")
    indexTempPath := indexPath + ".tmp"
    
    // Write index
    indexFile, err := os.Create(indexTempPath)
    if err != nil {
        return err
    }
    defer indexFile.Close()
    
    if err := gob.NewEncoder(indexFile).Encode(index); err != nil {
        return err
    }
    indexFile.Close()
    
    // Atomic rename
    if err := os.Rename(indexTempPath, indexPath); err != nil {
        return err
    }
    
    // Same for doc store
    docPath := filepath.Join(s.indexDir, "doc_metadata.gob")
    docTempPath := docPath + ".tmp"
    
    docFile, err := os.Create(docTempPath)
    if err != nil {
        return err
    }
    defer docFile.Close()
    
    if err := gob.NewEncoder(docFile).Encode(docStore); err != nil {
        return err
    }
    docFile.Close()
    
    return os.Rename(docTempPath, docPath)
}

func (s *IndexStore) Load() (*InvertedIndex, *DocumentStore, error) {
    indexPath := filepath.Join(s.indexDir, "inverted_index.gob")
    docPath := filepath.Join(s.indexDir, "doc_metadata.gob")
    
    // Load index
    indexFile, err := os.Open(indexPath)
    if err != nil {
        return nil, nil, err
    }
    defer indexFile.Close()
    
    var index InvertedIndex
    if err := gob.NewDecoder(indexFile).Decode(&index); err != nil {
        return nil, nil, err
    }
    
    // Load doc store
    docFile, err := os.Open(docPath)
    if err != nil {
        return nil, nil, err
    }
    defer docFile.Close()
    
    var docStore DocumentStore
    if err := gob.NewDecoder(docFile).Decode(&docStore); err != nil {
        return nil, nil, err
    }
    
    return &index, &docStore, nil
}
```

**Test Cases**:
- Save empty index → loads successfully
- Save non-empty index → data persists
- Corrupt file → returns error
- Atomic writes → no partial data on crash

---

### Story 2: Snippet Generation and Highlighting

#### Task 2.1: Implement SnippetGenerator
**Estimated Time**: 3 hours  
**Files**: `session/snippet_generator.go`, `session/snippet_generator_test.go`  
**Completion Criteria**: Snippets generated with highlights

**Implementation Approach**:
```go
package session

import "strings"

type SnippetGenerator struct {
    contextWords int
    maxSnippets  int
}

type Snippet struct {
    Text           string
    HighlightStart int
    HighlightEnd   int
    MessageRole    string
    MessageTime    time.Time
}

func NewSnippetGenerator() *SnippetGenerator {
    return &SnippetGenerator{
        contextWords: 30,
        maxSnippets:  3,
    }
}

func (g *SnippetGenerator) Generate(message string, query []string, role string, timestamp time.Time) []Snippet {
    words := strings.Fields(message)
    snippets := make([]Snippet, 0, g.maxSnippets)
    
    // Find all match positions
    matches := g.findMatches(words, query)
    if len(matches) == 0 {
        return snippets
    }
    
    // Generate snippet for each match (up to maxSnippets)
    for i, matchPos := range matches {
        if i >= g.maxSnippets {
            break
        }
        
        snippet := g.extractSnippet(words, matchPos, query[0])
        snippet.MessageRole = role
        snippet.MessageTime = timestamp
        snippets = append(snippets, snippet)
    }
    
    return snippets
}

func (g *SnippetGenerator) findMatches(words []string, query []string) []int {
    matches := []int{}
    for i, word := range words {
        for _, term := range query {
            if strings.Contains(strings.ToLower(word), term) {
                matches = append(matches, i)
                break
            }
        }
    }
    return matches
}

func (g *SnippetGenerator) extractSnippet(words []string, matchPos int, term string) Snippet {
    // Extract context window
    start := max(0, matchPos-g.contextWords)
    end := min(len(words), matchPos+g.contextWords+1)
    
    contextWords := words[start:end]
    text := strings.Join(contextWords, " ")
    
    // Find highlight position in snippet text
    matchWord := words[matchPos]
    highlightStart := strings.Index(strings.ToLower(text), term)
    highlightEnd := highlightStart + len(term)
    
    return Snippet{
        Text:           text,
        HighlightStart: highlightStart,
        HighlightEnd:   highlightEnd,
    }
}
```

**Test Cases**:
- Single match → one snippet with highlight
- Multiple matches → multiple snippets
- Match at message start → snippet starts at 0
- Match at message end → snippet ends at message length
- Long message → snippet truncated to context window

---

### Story 3: Backend gRPC API Integration

#### Task 3.1: Define SearchClaudeHistory Protobuf Schema
**Estimated Time**: 1 hour  
**Files**: `proto/session/v1/session.proto`  
**Completion Criteria**: Protobuf compiles successfully

**Implementation**:
```protobuf
// Add to session.proto

message SearchClaudeHistoryRequest {
  string query = 1;
  optional string project = 2;
  optional string model = 3;
  optional google.protobuf.Timestamp start_time = 4;
  optional google.protobuf.Timestamp end_time = 5;
  int32 limit = 6;
  int32 offset = 7;
}

message SearchClaudeHistoryResponse {
  repeated SearchResult results = 1;
  int32 total_matches = 2;
  int64 query_time_ms = 3;
}

message SearchResult {
  string session_id = 1;
  string session_name = 2;
  string project = 3;
  int32 message_index = 4;
  float score = 5;
  repeated Snippet snippets = 6;
  ResultMetadata metadata = 7;
}

message Snippet {
  string text = 1;
  int32 highlight_start = 2;
  int32 highlight_end = 3;
  string message_role = 4;
  google.protobuf.Timestamp message_time = 5;
}

message ResultMetadata {
  bool is_metadata_match = 1;
  string match_source = 2;
}

// Add to SessionService
rpc SearchClaudeHistory(SearchClaudeHistoryRequest) returns (SearchClaudeHistoryResponse) {}
```

---

#### Task 3.2: Implement SessionService.SearchClaudeHistory()
**Estimated Time**: 3 hours  
**Files**: `server/services/session_service.go`  
**Completion Criteria**: RPC handler implemented and tested

**Implementation Approach**:
```go
func (s *SessionService) SearchClaudeHistory(
    ctx context.Context,
    req *connect.Request[sessionv1.SearchClaudeHistoryRequest],
) (*connect.Response[sessionv1.SearchClaudeHistoryResponse], error) {
    // Validate query
    if req.Msg.Query == "" {
        return nil, connect.NewError(connect.CodeInvalidArgument, 
            fmt.Errorf("query is required"))
    }
    
    // Get or refresh history cache
    hist, err := s.getOrRefreshHistoryCache()
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal, 
            fmt.Errorf("failed to load history: %w", err))
    }
    
    // Initialize search engine if not already done
    if s.searchEngine == nil {
        s.searchEngine = session.NewSearchEngine()
        if err := s.searchEngine.BuildIndex(hist); err != nil {
            return nil, connect.NewError(connect.CodeInternal,
                fmt.Errorf("failed to build search index: %w", err))
        }
    }
    
    // Execute search
    results, err := s.searchEngine.Search(req.Msg.Query, session.SearchOptions{
        Limit:  int(req.Msg.Limit),
        Offset: int(req.Msg.Offset),
    })
    if err != nil {
        return nil, connect.NewError(connect.CodeInternal,
            fmt.Errorf("search failed: %w", err))
    }
    
    // Generate snippets for each result
    protoResults := make([]*sessionv1.SearchResult, 0, len(results.Results))
    for _, result := range results.Results {
        // Get session name from history
        entry, _ := hist.GetByID(result.SessionID)
        
        // Generate snippets
        snippetGen := session.NewSnippetGenerator()
        snippets := snippetGen.Generate(
            result.Document.Content,
            strings.Fields(req.Msg.Query),
            result.Document.MessageRole,
            result.Document.Timestamp,
        )
        
        // Convert snippets to proto
        protoSnippets := make([]*sessionv1.Snippet, 0, len(snippets))
        for _, snip := range snippets {
            protoSnippets = append(protoSnippets, &sessionv1.Snippet{
                Text:           snip.Text,
                HighlightStart: int32(snip.HighlightStart),
                HighlightEnd:   int32(snip.HighlightEnd),
                MessageRole:    snip.MessageRole,
                MessageTime:    timestamppb.New(snip.MessageTime),
            })
        }
        
        protoResults = append(protoResults, &sessionv1.SearchResult{
            SessionId:    result.SessionID,
            SessionName:  entry.Name,
            Project:      entry.Project,
            MessageIndex: int32(result.MessageIndex),
            Score:        float32(result.Score),
            Snippets:     protoSnippets,
        })
    }
    
    return connect.NewResponse(&sessionv1.SearchClaudeHistoryResponse{
        Results:      protoResults,
        TotalMatches: int32(results.TotalMatches),
        QueryTimeMs:  results.QueryTime.Milliseconds(),
    }), nil
}
```

---

### Story 4: Frontend Search UI Integration

#### Task 4.1: Add Search Input to History Page
**Estimated Time**: 2 hours  
**Files**: `web-app/src/app/history/page.tsx`  
**Completion Criteria**: Search input renders and triggers search

**Implementation Approach**:
```tsx
// Add state for search results
const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
const [isSearchMode, setIsSearchMode] = useState(false);

// Search handler
const handleContentSearch = useCallback(async () => {
  if (!searchQuery.trim()) {
    setIsSearchMode(false);
    return;
  }
  
  setSearching(true);
  setIsSearchMode(true);
  
  try {
    const response = await clientRef.current.searchClaudeHistory({
      query: searchQuery,
      limit: 50,
      offset: 0,
    });
    setSearchResults(response.results);
  } catch (err) {
    setError(`Search failed: ${err}`);
  } finally {
    setSearching(false);
  }
}, [searchQuery]);

// Update search input to use new handler
<button onClick={handleContentSearch} disabled={searching}>
  {searching ? "Searching..." : "Search Content"}
</button>
```

---

#### Task 4.2: Display Search Results with Snippets
**Estimated Time**: 3 hours  
**Files**: `web-app/src/app/history/page.tsx`, `web-app/src/app/history/SearchResultCard.tsx`  
**Completion Criteria**: Results display with highlighted snippets

**Implementation Approach**:
```tsx
// SearchResultCard.tsx
interface SearchResultCardProps {
  result: SearchResult;
  onClick: () => void;
}

export function SearchResultCard({ result, onClick }: SearchResultCardProps) {
  return (
    <div className={styles.searchResultCard} onClick={onClick}>
      <div className={styles.resultHeader}>
        <div className={styles.sessionName}>{result.sessionName}</div>
        <div className={styles.score}>Score: {result.score.toFixed(2)}</div>
      </div>
      
      <div className={styles.snippets}>
        {result.snippets.map((snippet, idx) => (
          <div key={idx} className={styles.snippet}>
            <div className={styles.snippetRole}>
              {snippet.messageRole === "user" ? "👤 User" : "🤖 Assistant"}
            </div>
            <div className={styles.snippetText}>
              {snippet.text.substring(0, snippet.highlightStart)}
              <mark className={styles.highlight}>
                {snippet.text.substring(snippet.highlightStart, snippet.highlightEnd)}
              </mark>
              {snippet.text.substring(snippet.highlightEnd)}
            </div>
          </div>
        ))}
      </div>
      
      <div className={styles.resultMeta}>
        📁 {truncateMiddle(result.project, 50)}
      </div>
    </div>
  );
}
```

---

## Known Issues & Mitigation

### Potential Bugs Identified During Planning

#### 🐛 Concurrency Risk: Race Condition in Index Updates [SEVERITY: High]

**Description**: Concurrent searches during index updates may read inconsistent state if index and doc store are updated non-atomically.

**Reproduction**:
```go
// Thread 1: Update index
engine.UpdateIndex(newMessages) // Adds to index
// <-- Thread 2 searches here: sees term in index but doc metadata missing
engine.docStore.Add(docID, doc) // Adds doc metadata
```

**Mitigation**:
- Use `sync.RWMutex` with proper locking:
  ```go
  func (e *SearchEngine) UpdateIndex(messages []Message) error {
      e.mu.Lock()
      defer e.mu.Unlock()
      
      // Atomic update: both index and docStore updated under lock
      for _, msg := range messages {
          tokens := e.tokenizer.Tokenize(msg.Content)
          e.index.AddDocument(docID, tokens)
          e.docStore.Add(docID, &Document{...})
          docID++
      }
      return nil
  }
  
  func (e *SearchEngine) Search(query string) (*SearchResults, error) {
      e.mu.RLock()
      defer e.mu.RUnlock()
      // Read operations protected by read lock
  }
  ```

**Prevention Strategy**:
- Design index operations to be atomic (all-or-nothing)
- Use read-write locks to allow concurrent reads
- Add integration tests with concurrent search + update

**Files Likely Affected**:
- `session/search_engine.go`
- `session/inverted_index.go`
- `session/document_store.go`

---

#### 🐛 Data Integrity: Index Corruption on Crash During Persistence [SEVERITY: High]

**Description**: If the process crashes while writing index to disk, partial data may be written, causing index corruption on next load.

**Reproduction**:
```go
// Writing index.gob
file.Write(indexData[:50%]) // Wrote half the data
// <-- Process killed here
file.Write(indexData[50%:])  // Never executed
```

**Mitigation**:
- Use atomic write pattern (write-to-temp-file + rename):
  ```go
  func (s *IndexStore) Save(index *InvertedIndex) error {
      tempPath := filepath.Join(s.indexDir, "inverted_index.gob.tmp")
      finalPath := filepath.Join(s.indexDir, "inverted_index.gob")
      
      // Write to temp file
      tempFile, err := os.Create(tempPath)
      if err != nil {
          return err
      }
      defer os.Remove(tempPath) // Cleanup temp file
      
      if err := gob.NewEncoder(tempFile).Encode(index); err != nil {
          return err
      }
      tempFile.Close()
      
      // Atomic rename (POSIX guarantees atomicity)
      return os.Rename(tempPath, finalPath)
  }
  ```
- Add CRC32 checksum validation on load
- Implement automatic index rebuild if corruption detected

**Prevention Strategy**:
- Always use atomic write pattern (temp file + rename)
- Add index version file with checksum
- Graceful degradation: rebuild index if load fails

**Files Likely Affected**:
- `session/index_store.go`
- `session/search_engine.go`

---

#### 🐛 Performance Degradation: Memory Leak in Snippet Generation [SEVERITY: Medium]

**Description**: Snippet generation allocates new strings for every result, potentially causing memory pressure for large result sets (1000+ results).

**Reproduction**:
```go
// Generating 1000 snippets, each allocating 200-byte string
for _, result := range results[:1000] {
    snippet := generateSnippet(result.Content) // Allocates new string
    // Memory usage: 1000 * 200 bytes = ~200KB per search
}
// With repeated searches, GC pressure increases
```

**Mitigation**:
- Limit snippet generation to top N results (e.g., 20):
  ```go
  func (e *SearchEngine) Search(query string, opts SearchOptions) (*SearchResults, error) {
      // ... scoring and ranking ...
      
      // Only generate snippets for top 20 results (user rarely scrolls past)
      snippetLimit := min(20, len(results))
      for i := 0; i < snippetLimit; i++ {
          results[i].Snippets = e.snippetGen.Generate(...)
      }
      
      return &SearchResults{Results: results}, nil
  }
  ```
- Use string builders to reduce allocations
- Implement lazy snippet generation (only when result clicked)

**Prevention Strategy**:
- Profile memory usage with `go test -memprofile`
- Set snippet generation limits
- Consider lazy loading snippets for large result sets

**Files Likely Affected**:
- `session/snippet_generator.go`
- `session/search_engine.go`

---

#### 🐛 Edge Case: Crash on Empty or Malformed Conversation Files [SEVERITY: Medium]

**Description**: Index build crashes if conversation file is empty, corrupted, or has no parseable messages.

**Reproduction**:
```bash
# Create empty conversation file
touch ~/.claude/projects/myproject/conversation.jsonl

# Or create malformed file
echo "invalid json" > ~/.claude/projects/myproject/conversation.jsonl

# App tries to index these files → panic or error
```

**Mitigation**:
- Add defensive parsing with error recovery:
  ```go
  func (e *SearchEngine) BuildIndex(history *ClaudeSessionHistory) error {
      entries := history.GetAll()
      
      for _, entry := range entries {
          messages, err := history.GetMessagesFromConversationFile(entry.ID)
          if err != nil {
              // Log error but continue indexing other files
              log.Warnf("Failed to load messages for %s: %v", entry.ID, err)
              continue
          }
          
          if len(messages) == 0 {
              // Skip empty conversations
              continue
          }
          
          // ... index messages ...
      }
      
      return nil
  }
  ```
- Add file validation before parsing
- Log skipped files for debugging

**Prevention Strategy**:
- Add error handling for each conversation file
- Skip invalid files instead of failing entire index build
- Add unit tests with malformed input files

**Files Likely Affected**:
- `session/search_engine.go`
- `session/history.go`

---

#### 🐛 Search Quality: Poor Results for Multi-Word Queries [SEVERITY: Medium]

**Description**: Multi-word queries may return irrelevant results if words appear separately in document (false positives).

**Example**:
```
Query: "docker container error"
False positive: Document with "docker" in line 1, "container" in line 50, "error" in line 100
True positive: Document with "docker container error" as phrase
```

**Mitigation**:
- Implement phrase proximity scoring:
  ```go
  func (s *BM25Scorer) Score(query []string, docID int32) float64 {
      baseScore := s.basicBM25Score(query, docID)
      
      // Boost score if terms appear close together
      proximityBoost := s.calculateProximityBoost(query, docID)
      
      return baseScore * (1.0 + proximityBoost)
  }
  
  func (s *BM25Scorer) calculateProximityBoost(query []string, docID int32) float64 {
      if len(query) < 2 {
          return 0.0
      }
      
      // Get term positions
      positions := make([][]int32, len(query))
      for i, term := range query {
          postings := s.index.Search(term)
          positions[i] = s.getPositions(postings, docID)
      }
      
      // Find minimum distance between consecutive terms
      minDist := s.findMinDistance(positions)
      
      // Boost: 0.5 if all terms within 10 words, 0.0 if > 50 words apart
      if minDist <= 10 {
          return 0.5
      } else if minDist <= 50 {
          return 0.2
      }
      return 0.0
  }
  ```

**Prevention Strategy**:
- Track term positions in postings list
- Add proximity-based scoring
- Test with multi-word queries

**Files Likely Affected**:
- `session/bm25.go`
- `session/inverted_index.go`

---

#### 🐛 UI Issue: Snippet Highlight Offsets Incorrect for Special Characters [SEVERITY: Low]

**Description**: Snippet highlight positions calculated on byte offsets may be incorrect if message contains multi-byte UTF-8 characters (emoji, CJK).

**Reproduction**:
```go
message := "Hello 🌍 world"
query := "world"
// Byte offset of "world" = 10 (but should be 8 in rune count)
// Highlight position off by 2 bytes due to emoji
```

**Mitigation**:
- Use rune-based string slicing:
  ```go
  func (g *SnippetGenerator) extractSnippet(message string, matchPos int) Snippet {
      runes := []rune(message)
      
      // Find match position in runes (not bytes)
      matchRunePos := 0
      for i, r := range runes {
          if strings.HasPrefix(string(runes[i:]), term) {
              matchRunePos = i
              break
          }
      }
      
      // Extract snippet using rune positions
      start := max(0, matchRunePos - g.contextWords)
      end := min(len(runes), matchRunePos + g.contextWords)
      
      snippetRunes := runes[start:end]
      snippetText := string(snippetRunes)
      
      // Highlight position relative to snippet start
      highlightStart := matchRunePos - start
      highlightEnd := highlightStart + len([]rune(term))
      
      return Snippet{
          Text:           snippetText,
          HighlightStart: highlightStart,
          HighlightEnd:   highlightEnd,
      }
  }
  ```

**Prevention Strategy**:
- Always use rune-based string operations for display
- Test with emoji, CJK characters
- Add unit tests with multi-byte characters

**Files Likely Affected**:
- `session/snippet_generator.go`
- `web-app/src/app/history/SearchResultCard.tsx`

---

## Testing Strategy

### Unit Tests

**Coverage Target**: > 90% for all search components

**Test Suites**:

#### Tokenizer Tests (`session/tokenizer_test.go`)
```go
func TestTokenizer_BasicTokenization(t *testing.T) {
    tokenizer := NewTokenizer()
    
    tests := []struct{
        input    string
        expected []string
    }{
        {"hello world", []string{"hello", "world"}},
        {"Hello, World!", []string{"hello", "world"}},
        {"the cat is running", []string{"cat", "run"}}, // stop word + stemming
        {"", []string{}},
    }
    
    for _, tt := range tests {
        tokens := tokenizer.Tokenize(tt.input)
        assert.Equal(t, tt.expected, tokens)
    }
}
```

#### InvertedIndex Tests (`session/inverted_index_test.go`)
```go
func TestInvertedIndex_AddAndSearch(t *testing.T) {
    index := NewInvertedIndex()
    
    // Add documents
    index.AddDocument(1, []string{"hello", "world"})
    index.AddDocument(2, []string{"world", "peace"})
    
    // Search
    postings := index.Search("world")
    assert.NotNil(t, postings)
    assert.Equal(t, []int32{1, 2}, postings.DocIDs)
}

func TestInvertedIndex_ConcurrentAccess(t *testing.T) {
    index := NewInvertedIndex()
    
    // Concurrent writes
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(docID int32) {
            defer wg.Done()
            index.AddDocument(docID, []string{"test"})
        }(int32(i))
    }
    wg.Wait()
    
    // No race conditions
    postings := index.Search("test")
    assert.Equal(t, 100, len(postings.DocIDs))
}
```

#### BM25 Scoring Tests (`session/bm25_test.go`)
```go
func TestBM25_SingleTermScore(t *testing.T) {
    scorer := setupBM25Scorer()
    
    score := scorer.Score([]string{"test"}, 1)
    assert.Greater(t, score, 0.0)
}

func TestBM25_MultiTermScoreHigherThanSingle(t *testing.T) {
    scorer := setupBM25Scorer()
    
    singleScore := scorer.Score([]string{"test"}, 1)
    multiScore := scorer.Score([]string{"test", "document"}, 1)
    
    assert.Greater(t, multiScore, singleScore)
}
```

#### Snippet Generation Tests (`session/snippet_generator_test.go`)
```go
func TestSnippetGenerator_BasicSnippet(t *testing.T) {
    gen := NewSnippetGenerator()
    
    message := "This is a test message with some context around the search term."
    query := []string{"search"}
    
    snippets := gen.Generate(message, query, "user", time.Now())
    
    assert.Equal(t, 1, len(snippets))
    assert.Contains(t, snippets[0].Text, "search")
    assert.Greater(t, snippets[0].HighlightStart, 0)
}

func TestSnippetGenerator_MultiByteCharacters(t *testing.T) {
    gen := NewSnippetGenerator()
    
    message := "Hello 🌍 world with emoji 😀 and CJK 你好 characters"
    query := []string{"emoji"}
    
    snippets := gen.Generate(message, query, "user", time.Now())
    
    // Highlight positions should be correct even with multi-byte chars
    snippet := snippets[0]
    highlighted := snippet.Text[snippet.HighlightStart:snippet.HighlightEnd]
    assert.Equal(t, "emoji", highlighted)
}
```

### Integration Tests

**Test Suites**:

#### End-to-End Search Flow (`session/search_integration_test.go`)
```go
func TestSearchEngine_EndToEndSearch(t *testing.T) {
    // Setup: Create test conversation files
    testDir := setupTestConversations(t)
    defer os.RemoveAll(testDir)
    
    // Load history
    history, err := session.NewClaudeSessionHistory(testDir)
    require.NoError(t, err)
    
    // Build index
    engine := session.NewSearchEngine()
    err = engine.BuildIndex(history)
    require.NoError(t, err)
    
    // Execute search
    results, err := engine.Search("docker container", session.SearchOptions{})
    require.NoError(t, err)
    
    // Verify results
    assert.Greater(t, len(results.Results), 0)
    assert.NotEmpty(t, results.Results[0].Snippets)
    assert.Contains(t, results.Results[0].Snippets[0].Text, "docker")
}
```

#### RPC Integration Tests (`server/services/session_service_test.go`)
```go
func TestSessionService_SearchClaudeHistory(t *testing.T) {
    // Setup test service
    service := setupTestSessionService(t)
    
    // Call RPC
    req := connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
        Query: "test query",
        Limit: 10,
    })
    
    resp, err := service.SearchClaudeHistory(context.Background(), req)
    require.NoError(t, err)
    
    // Verify response
    assert.NotNil(t, resp.Msg)
    assert.GreaterOrEqual(t, len(resp.Msg.Results), 0)
}
```

### Performance Tests

**Benchmark Suites**:

#### Index Build Performance (`session/search_engine_bench_test.go`)
```go
func BenchmarkSearchEngine_BuildIndex(b *testing.B) {
    // Generate 50,000 test messages
    history := generateLargeHistory(50000)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        engine := session.NewSearchEngine()
        engine.BuildIndex(history)
    }
}

// Target: < 30 seconds for 50,000 messages
```

#### Search Query Performance (`session/search_engine_bench_test.go`)
```go
func BenchmarkSearchEngine_Search(b *testing.B) {
    history := generateLargeHistory(10000)
    engine := session.NewSearchEngine()
    engine.BuildIndex(history)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        engine.Search("test query", session.SearchOptions{Limit: 20})
    }
}

// Target: < 200ms p50, < 500ms p95
```

---

## Context Preparation Guides

### For Backend Developer (Story 1-3)

**Files to Review**:
1. `session/history.go` - Understand conversation file format
2. `session/storage.go` - Understand data model
3. `server/services/session_service.go` - Understand existing RPC patterns
4. `proto/session/v1/session.proto` - Understand protobuf schema

**Key Concepts**:
- Claude conversation files are JSONL (one JSON object per line)
- Each line is a `conversationMessage` with type, role, content
- Index must be built from `ClaudeSessionHistory.GetAll()`
- BM25 scoring formula: `score = IDF * (TF * (k1+1)) / (TF + k1*(1-b+b*docLen/avgLen))`

**Example Workflow**:
```bash
# 1. Read existing history loading code
bat session/history.go

# 2. Understand message structure
head ~/.claude/projects/*/conversation*.jsonl

# 3. Review existing RPC implementations
bat server/services/session_service.go | grep -A 20 "ListClaudeHistory"

# 4. Run existing tests
go test ./session -v -run TestClaudeSessionHistory
```

---

### For Frontend Developer (Story 4-5)

**Files to Review**:
1. `web-app/src/app/history/page.tsx` - Existing history page
2. `web-app/src/gen/session/v1/session_pb.ts` - Generated protobuf types
3. `web-app/src/lib/config.ts` - API client configuration

**Key Concepts**:
- History page uses ConnectRPC for API calls
- Search results should integrate with existing entry list
- Snippet highlighting uses `<mark>` tag or custom styled span
- Modal already has message display logic (reuse for highlight)

**Example Workflow**:
```bash
# 1. Review existing history page structure
bat web-app/src/app/history/page.tsx

# 2. Understand existing search implementation (metadata only)
grep -A 10 "handleSearch" web-app/src/app/history/page.tsx

# 3. Check protobuf types after schema update
bat web-app/src/gen/session/v1/session_pb.ts | grep "SearchResult"

# 4. Run dev server
cd web-app && npm run dev
```

---

## Dependencies & Sequencing

### Critical Path

```
Story 1 (Search Engine) → Story 2 (Snippets) → Story 3 (API) → Story 4 (UI) → Story 5 (Highlighting)
       ↓                                                            ↓
Story 6 (Incremental Updates)                            Story 7 (Performance)
```

### Dependency Matrix

| Story | Depends On | Can Start After | Blocks |
|-------|------------|-----------------|--------|
| 1. Search Engine | None | Day 0 | 2, 3, 6 |
| 2. Snippets | Story 1 | Day 5 | 3, 5 |
| 3. Backend API | Stories 1, 2 | Day 8 | 4 |
| 4. Frontend UI | Story 3 | Day 10 | 5 |
| 5. Modal Highlighting | Story 4 | Day 13 | None |
| 6. Incremental Updates | Story 1 | Day 5 (parallel) | None |
| 7. Performance | Stories 1, 6 | Day 7 (parallel) | None |

### Parallel Work Opportunities

**Week 1 (Days 1-5)**:
- **Primary**: Story 1 (Search Engine Foundation) - 5 days
- **Parallel**: None (foundational work)

**Week 2 (Days 6-10)**:
- **Primary**: Story 2 (Snippets) + Story 3 (API) - 5 days
- **Parallel**: Story 6 (Incremental Updates) can start Day 6

**Week 3 (Days 11-15)**:
- **Primary**: Story 4 (Frontend UI) - 3 days
- **Parallel**: Story 7 (Performance) can start Day 11

**Week 4 (Days 16-20)**:
- **Primary**: Story 5 (Modal Highlighting) - 2 days
- **Cleanup**: Bug fixes, performance tuning, documentation

---

## Success Criteria Summary

**MVP (Minimum Viable Product)**:
- ✅ Stories 1-4 complete
- ✅ Full-text search works from web UI
- ✅ Snippets show highlighted search terms
- ✅ Search latency < 500ms for 10,000 messages

**V1 (Full Feature)**:
- ✅ All 7 stories complete
- ✅ Modal highlighting and navigation
- ✅ Incremental index updates
- ✅ Performance optimizations for 50,000+ messages

**Quality Gates**:
- ✅ Unit test coverage > 90%
- ✅ Integration tests pass for all RPC endpoints
- ✅ Performance benchmarks meet targets
- ✅ User acceptance testing confirms intuitive workflow

---

## References

**Academic & Industry Sources**:
- Robertson, S., & Zaragoza, H. (2009). "The Probabilistic Relevance Framework: BM25 and Beyond"
- Manning, C., Raghavan, P., & Schütze, H. (2008). "Introduction to Information Retrieval"
- Zobel, J., & Moffat, A. (2006). "Inverted files for text search engines"
- Porter, M. (1980). "An algorithm for suffix stripping" (Porter Stemmer)

**Related Documentation**:
- `docs/tasks/history-page-ux-improvements.md` - UI patterns
- `docs/tasks/session-search-and-sort.md` - Session-level search (different from content search)
- `session/history.go` - Existing history loading implementation

---

**End of Document**
