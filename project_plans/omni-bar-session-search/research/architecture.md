# Research: Architecture

## Existing Architecture Audit

### Frontend Session Data

The full session list is already available client-side with zero additional RPCs needed.

**How it works:**
- `useSessionService` (`web-app/src/lib/hooks/useSessionService.ts`) calls `listSessions` on mount and opens a `WatchSessions` stream for real-time updates.
- All session objects are stored in a Redux entity adapter (`sessionsSlice.ts`) keyed by session ID.
- The `sessions` array from `useAppSelector(selectAllSessions)` is passed as a prop to `SessionList`, which does client-side filtering today (see `filteredSessions` `useMemo` in `SessionList.tsx:175–195`).
- Session objects in Redux already carry every field needed for Omnibar search: `title`, `path`, `branch`, `category`, `tags`, `status`, `program`, `updatedAt`.

**Conclusion:** Client-side fuzzy filtering is fully viable with zero new RPCs. The data is already in the Redux store when the Omnibar opens.

### Existing Search Engine (session/search/)

The `session/search/` package is a full BM25 search engine over Claude **conversation history** (the JSONL files in `~/.claude/`). It is completely separate from the live session list.

**Components:**
- `engine.go` — `SearchEngine` struct with `InvertedIndex`, `DocumentStore`, `BM25Scorer`. Supports `IncrementalSync` against `ClaudeSessionHistory`, incremental add/remove, and disk persistence via `IndexStore`.
- `bm25.go` — Standard Okapi BM25 with K1=1.5, B=0.75.
- `tokenizer.go` — Text tokenizer (splits on whitespace/punctuation, normalizes).
- `inverted_index.go` — In-memory inverted index with `sync.RWMutex`.
- `snippet.go` — Snippet generation with highlight ranges for search results.
- `sync_types.go` — `SyncResult`, `IndexSyncMetadata` for incremental index maintenance.

**Critical distinction:** This engine indexes _message content_ from historical Claude conversations, not the live session list. `SearchEngine.Search()` returns `SearchResult` objects with `SessionID` (conversation UUID), `MessageIndex`, BM25 score, and snippet text. It does **not** search active session titles/paths/branches.

**Limitations for Omnibar use:**
- No fuzzy matching — requires exact tokenized term hits.
- No consecutive-character bonus, no start-of-word bonus.
- Not designed for short, incomplete queries like "myfeat" or "squad".
- Operates on Claude history, not the live session store.

**Extending this engine for session search:** Technically possible but architecturally wrong. BM25 is a bag-of-words model; it does not reward subsequence/fuzzy matches. "squad" would not match "stapler-squad" because BM25 needs the token "squad" to appear in the document, but "stapler-squad" tokenizes to "stapler" and "squad" — actually this would work for exact word matches. However "myfeat" would never match "my-feature-branch" because BM25 has no character-level fuzzy matching. A separate lightweight fuzzy algorithm is needed.

### Path Completion Service

`path_completion_service.go` is **completely stateless**:

```go
type PathCompletionService struct{}
```

Every call to `ListPathCompletions` does a fresh `os.ReadDir` with a 2-second timeout. There is no cache. The only latency guard is the 150ms debounce in `usePathCompletions.ts` (client-side) and a module-level LRU cache in `usePathCompletions.ts` with 100 entries, 30s TTL.

**Current client-side cache** (`usePathCompletions.ts:27–55`):
- `completionCache`: `Map<string, CacheEntry>` (module-level, survives component remounts)
- Key: `${pathPrefix}::${directoriesOnly}::${maxResults}`
- TTL: 30 seconds
- Max size: 100 entries, LRU eviction

This client-side cache is already working. The requirement for a server-side cache would address the case where multiple browser tabs or clients share the server, or where the cache survives a full page reload (the client cache does not survive page reload). A server-side cache would also reduce filesystem I/O across all concurrent calls.

### usePathHistory Hook

`usePathHistory.ts` is a **client-side localStorage hook** — entirely frontend, no backend involvement.

**What it stores:**
```typescript
interface PathHistoryEntry {
  path: string;   // full absolute path
  count: number;  // times used
  lastUsed: number; // epoch ms
}
```

**Storage key:** `"omnibar:path-history"` in `localStorage`
**Max entries:** 50, trimmed by score on each save
**Max results returned:** 10

**Scoring function:**
```typescript
function entryScore(e: PathHistoryEntry): number {
  return recencyScore(e.lastUsed) + Math.log1p(e.count);
}
// recencyScore: 1.0 (<1h), 0.8 (<1d), 0.6 (<1w), 0.4 (<1mo), 0.2 (older)
```

**`getMatching(prefix)`:** Returns entries where `e.path.startsWith(prefix)` (strict prefix filter, not fuzzy), sorted by score desc.

**`save(path)`:** Called in `Omnibar.tsx` after a successful session creation (`saveHistory(detection.localPath)`).

**What this means for "Recent Repos":** The data source for recent repos already exists in `usePathHistory`. It already tracks every path used for session creation with recency scoring. The hook only needs to be extended (or supplemented) to expose the full list (not just prefix-matching entries) when the Omnibar input is empty.

---

## Session Search Design Decision

### Option A: Client-side filtering

**Mechanism:** When the Omnibar input does not look like a path (no leading `~` or `/`), apply a fuzzy scoring function over the Redux session array already in memory. Render matching sessions as a separate result section.

**Pros:**
- Zero new RPCs — session data is already in Redux.
- Zero latency — no network round trip, runs synchronously in a `useMemo`.
- No new backend code.
- Real-time: results update instantly as WatchSessions stream pushes changes.
- Simple to implement: add a `useSessionSearch(query, sessions)` hook.

**Cons:**
- If session count is very large (1000+), filtering in the render loop may cause jank. In practice, most users have ≤100 sessions, making this a non-issue.
- Fuzzy scoring happens on every keystroke. Acceptable if the algorithm is O(n * |query|), as it would be with a simple fuzzy scorer.

**Feasibility:** High. The `filteredSessions` `useMemo` in `SessionList.tsx` already does client-side string matching today; this is an upgrade to that pattern.

### Option B: New SearchSessions RPC

**Mechanism:** Add `SearchSessions(SearchSessionsRequest)` RPC to the backend. Backend receives query string, runs fuzzy match over the in-memory session store, returns ranked results.

**Pros:**
- Search logic lives in one place (Go), reusable by TUI and web.
- Can leverage existing `session/search/` infrastructure (though that's overkill here).
- Future-proof if session count grows very large.

**Cons:**
- Network round trip for every keystroke (even with debounce, adds 20–50ms latency).
- Adds backend complexity: new RPC, new proto messages, new handler, new tests.
- The session store is already in Redux client-side — sending it to the server and back is redundant.
- Breaks incremental update model: a search result could be stale if a session is updated between query and response.

**Feasibility:** Medium. Technically straightforward but introduces unnecessary latency and complexity.

### Option C: Extend existing SearchSessions RPC in ListSessionsRequest

`ListSessionsRequest` already has a `search_query` field (proto field 4). The backend `session/storage.go` presumably uses this for filtering. However, examining the client, `listSessions` in `useSessionService.ts` does not pass `searchQuery` from the Omnibar — it would require a new call pattern and introduces the same network-round-trip problem as Option B.

### Recommendation: Option A — Client-side filtering

**Rationale:**
1. The data is already in memory (Redux store, kept fresh by WatchSessions).
2. Client-side filtering eliminates all network latency — results appear on every keypress.
3. Session counts are small (typically <100). A fuzzy scorer over 100 sessions takes <1ms.
4. The existing `filteredSessions` pattern in `SessionList.tsx` proves the approach works.
5. Adding a `useSessionSearch(query, sessions)` hook requires no proto changes and no backend work.

**Implementation sketch:**
```typescript
// web-app/src/lib/hooks/useSessionSearch.ts
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { useMemo } from "react";
import { fuzzyScore } from "@/lib/fuzzy"; // new module

export function useSessionSearch(query: string) {
  const sessions = useAppSelector(selectAllSessions);
  return useMemo(() => {
    if (!query.trim()) return [];
    return sessions
      .map(s => ({
        session: s,
        score: fuzzyScore(query, [s.title, s.branch, s.path, ...(s.tags ?? [])].join(" "))
      }))
      .filter(r => r.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, 10)
      .map(r => r.session);
  }, [query, sessions]);
}
```

---

## Recent Repos Design

### Where the data comes from

`usePathHistory` already captures every path used for session creation (called from `Omnibar.tsx:360`: `saveHistory(detection.localPath)`). It stores up to 50 entries with recency scoring.

**Current limitation:** `getMatching(prefix)` only returns entries that start with the query prefix. For the "Recent Repos" empty-input case, we need `getAll()` — all entries sorted by score, not filtered by prefix.

### Proposed extension to usePathHistory

Add a `getAll()` method to the existing hook:
```typescript
const getAll = useCallback((): PathHistoryEntry[] => {
  return [...entries].sort((a, b) => entryScore(b) - entryScore(a));
}, [entries]);
```

This returns all stored paths sorted by recency+frequency score — exactly what "Recent Repos" needs.

### Where the data lives

**Frontend (localStorage):** All history data lives in `localStorage["omnibar:path-history"]`. This persists across page reloads but not across different browsers/devices. Sufficient for a solo-practitioner use case.

**No backend needed:** Path history is already a client-side concern. Moving it to the backend would require a new RPC, new state persistence, and new proto messages for no functional gain.

**Displayed when:** Input is empty OR when query is non-empty but does not look like a path (so it's in "session search mode"). The recent repos section is shown as a secondary list below session results.

---

## Directory Tree Cache Design

The current `PathCompletionService` is stateless. Every `ListPathCompletions` call performs a fresh `os.ReadDir`. The client already has a 30s LRU cache in `usePathCompletions.ts`, but this is lost on page reload and not shared across browser sessions.

### Recommended Server-Side Cache Design

**Data structure:**

```go
type DirCacheEntry struct {
    Entries   []os.DirEntry
    CachedAt  time.Time
    DirMtime  time.Time // mtime of the directory at cache time
}

type DirCache struct {
    mu      sync.RWMutex
    entries map[string]*DirCacheEntry // key: absolute baseDir path
    maxSize int                       // max entries (LRU eviction)
    ttl     time.Duration             // hard TTL (even if mtime unchanged)
}
```

**Invalidation strategy — mtime-based with hard TTL:**

On each cache lookup, stat the directory:
```go
func (c *DirCache) Get(path string) ([]os.DirEntry, bool) {
    c.mu.RLock()
    entry, ok := c.entries[path]
    c.mu.RUnlock()

    if !ok {
        return nil, false
    }

    // Hard TTL check
    if time.Since(entry.CachedAt) > c.ttl {
        c.mu.Lock()
        delete(c.entries, path)
        c.mu.Unlock()
        return nil, false
    }

    // mtime check: is the directory newer than our cache?
    info, err := os.Stat(path)
    if err != nil || info.ModTime().After(entry.DirMtime) {
        c.mu.Lock()
        delete(c.entries, path)
        c.mu.Unlock()
        return nil, false
    }

    return entry.Entries, true
}
```

**Thread safety:** `sync.RWMutex` — multiple concurrent reads (multiple Omnibar users, though in practice this is a single user), exclusive writes on cache miss/invalidation.

**Scope:** Global singleton, shared across all `ListPathCompletions` requests. This means the second time any client opens the Omnibar and browses the same directory, the cache is warm.

**LRU eviction:** Keep at most 256 directory entries. When evicting, remove the entry with the oldest `CachedAt`. Simple O(n) scan is fine at this scale; a heap is unnecessary.

**TTL recommendation:** 5 seconds. This covers the typical Omnibar interaction (open → type path → navigate → close) while preventing stale data from persisting across filesystem mutations (git checkout, npm install, etc.).

**Why not background refresh:** Background goroutines that poll directories consume resources even when no user is active. mtime-check on demand is cheaper and simpler. A 5s TTL with mtime invalidation handles all real-world cases.

**Integration into PathCompletionService:**

```go
type PathCompletionService struct {
    cache *DirCache
}

func NewPathCompletionService() *PathCompletionService {
    return &PathCompletionService{
        cache: NewDirCache(256, 5*time.Second),
    }
}

// In ListPathCompletions, replace os.ReadDir with:
if cached, ok := p.cache.Get(baseDir); ok {
    dirEntries = cached
} else {
    // existing ReadDir with timeout logic
    dirEntries = ... // result of os.ReadDir
    p.cache.Put(baseDir, dirEntries)
}
```

**Client cache interaction:** The client's 30s LRU cache in `usePathCompletions.ts` still operates as the first layer. The server cache is only hit when the client cache misses (page reload, first open, or after 30s). This means the effective behavior is:
- Reopening Omnibar within 30s: served from client cache (0ms overhead)
- After 30s or page reload: hits server cache (1 stat call instead of ReadDir)
- After server cache expires (5s TTL) or directory changed: fresh ReadDir

---

## Unified Result Ranking

If session results and recent repo paths are shown in a single list, a unified ranking approach is needed.

### Sections vs. Interleaved

**Recommendation: Sections, not interleaved.** The mental model for the user is different: sessions are things to navigate to; repos are things to create new sessions in. Mixing them confuses the action. VS Code Command Palette and Linear both use sections with headers.

**Layout:**
```
[Sessions]                    ← section header (only shown when query active)
  my-feature-session   Running
  stapler-squad-bug    Paused

[Recent Repos]                ← section header
  ~/projects/stapler-squad    2h ago
  ~/work/api-server            3d ago
```

### Score normalization

**Session scoring** (fuzzy score 0.0–1.0):
- Base: character-sequence fuzzy match score (consecutive bonus, start-of-word bonus)
- Status boost: Running × 1.2, Paused × 1.0, Stopped × 0.7
- Recency boost: `Math.log1p(hoursSinceUpdate)` decay applied as a multiplier

**Repo scoring** (from `usePathHistory`):
- Uses `entryScore(e) = recencyScore(e.lastUsed) + Math.log1p(e.count)` already implemented in the hook
- When query is active, also apply fuzzy match score against the path

**Score normalization:** Since sessions and repos are in separate sections, there is no need to normalize scores against each other. Each section is ranked independently.

### Implementation

```typescript
type OmnibarResult =
  | { type: "session"; session: Session; score: number }
  | { type: "repo";    entry: PathHistoryEntry; score: number };

function rankSessions(query: string, sessions: Session[]): OmnibarResult[] { ... }
function rankRepos(query: string, history: PathHistoryEntry[]): OmnibarResult[] { ... }
```

---

## Required Proto/RPC Changes

Session search via client-side filtering requires **no new proto messages or RPC methods**. All session data is already available through existing `ListSessions` / `WatchSessions` RPCs.

However, the following changes may be needed as nice-to-haves:

### Strictly Required (Must Have)

None. The architecture decision (client-side search) requires no proto changes.

### Potentially Useful

**1. `ListSessionsRequest.search_query` already exists (field 4)** but the backend implementation is currently a substring filter. If the recommendation is later revised to use the server for ranking (e.g., for TUI consistency), the backend implementation of this field would need upgrading to fuzzy. No proto changes needed.

**2. Server-side directory cache does not require proto changes.** It is an internal implementation change to `PathCompletionService`.

### If unified result type is needed on a future server RPC

```protobuf
// OmnibarSearchRequest — future only, not needed for client-side approach
message OmnibarSearchRequest {
  string query = 1;
  int32 max_sessions = 2;     // default 8
  int32 max_repos = 3;        // default 5
}

// OmnibarResult — future only
message OmnibarResult {
  oneof item {
    Session session = 1;
    RecentRepo repo = 2;
  }
  float score = 3;
  string match_source = 4;    // "title", "branch", "path", "tags"
}

message RecentRepo {
  string path = 1;
  google.protobuf.Timestamp last_used = 2;
  int32 use_count = 3;
}

message OmnibarSearchResponse {
  repeated OmnibarResult results = 1;
  int64 query_time_ms = 2;
}
```

**Verdict: No proto changes needed for the recommended client-side architecture.** The `ListSessionsRequest.search_query` field exists as a future hook if server-side ranking is ever desired for TUI/multi-client scenarios. The server-side directory cache is a pure internal refactor with no API surface changes.
