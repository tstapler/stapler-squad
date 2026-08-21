# Research: Stack

## Fuzzy Matching - Go Libraries

### `github.com/sahilm/fuzzy`

**Algorithm:** Ports the Sublime Text fuzzy match algorithm (forrestthewoods' reverse-engineering write-up). Scores results based on four weighted bonuses:
- **First-character match** — pattern starts at position 0 of the target
- **Consecutive adjacency bonus** — matched characters appear back-to-back in the target
- **Word-boundary bonus** — matched character follows a separator (space, `/`, `-`, `_`)
- **CamelCase bonus** — matched character is an uppercase letter preceded by lowercase

**API:**
```go
// Simple slice of strings
matches := fuzzy.Find("myfeat", []string{"my-feature-branch", "main", "fix-bug"})
// Custom types via Source interface
matches := fuzzy.FindFrom("squad", mySource)
// Match.Str, Match.Index, Match.MatchedIndexes []int (for highlighting)
```

**Performance:** Benchmarked on ~16K files in 12.9ms, ~60K files in 30.9ms. For the session-search use case (tens to hundreds of sessions) this is negligible — sub-millisecond. Zero external dependencies (pure stdlib).

**Assessment for this project:** This is the right choice. The consecutive-char bonus and word-boundary bonus are exactly what makes "myfeat" → "my-feature-branch" and "squad" → "stapler-squad" feel natural. Highlighted match positions (`.MatchedIndexes`) are a bonus for UI rendering.

---

### `github.com/lithammer/fuzzysearch`

**Algorithm:** Fuzzy character-subsequence matching (all pattern chars must appear in order, not necessarily adjacent), scored by Levenshtein distance. Higher score = better match, -1 = no match.

**API:**
```go
fuzzy.Match("myfeat", "my-feature-branch")  // bool
fuzzy.RankMatch("myfeat", "my-feature-branch")  // int score, -1 if no match
fuzzy.RankFind("query", []string{...})  // []Rank, sortable
```

**Scoring quality:** Levenshtein distance is a reasonable proxy for similarity but does not give bonuses for word boundaries or consecutive chars. "squad" matching "stapler-squad" relies purely on edit distance, so the ranking relative to other matches may be less intuitive than fzf-style.

**Performance:** Described as "tiny and fast." No published benchmarks, but Levenshtein on short strings is O(mn) where m and n are string lengths — this is fast enough for a few hundred sessions.

**Assessment:** Adequate for basic matching but produces noticeably worse ranking than sahilm/fuzzy for the "jump to existing session" use case. The lack of word-boundary and consecutive-char bonuses makes results feel random when multiple sessions share common substrings.

---

### `github.com/junegunn/fzf` (the CLI tool's internals)

The fzf repository contains `src/algo` with the authoritative Smith-Waterman-like scoring used by the fzf CLI. However, this package is **not designed as a library** — it's an internal package of the CLI tool. Extracting it would require forking. `github.com/ktr0731/go-fuzzyfinder` wraps fzf-style TUI selection but does not expose public scoring functions.

**Assessment:** Not directly usable as a library. Use `sahilm/fuzzy` instead, which is a clean reimplementation of the same scoring philosophy (Sublime Text algorithm, which is similar in spirit to fzf's Smith-Waterman approach).

---

### Recommendation (Go)

**Use `github.com/sahilm/fuzzy`.** It is the only Go library surveyed that implements all three quality signals the requirements explicitly call out: consecutive-char bonus, word-boundary bonus, and match-position highlighting. It is zero-dependency, actively maintained, and performant far beyond what the session-count scale requires.

Note: `github.com/agext/levenshtein` is already in `go.mod` as an indirect dependency, but it is a Levenshtein distance utility, not a multi-field fuzzy search API. It does not meet the scoring quality requirements.

---

## Fuzzy Matching - TypeScript Libraries

### `fuse.js`

**Algorithm:** Bitap algorithm (Shift-or/Shift-and), a classic approximate string matching algorithm. Returns scores from 0.0 (exact) to 1.0 (no match). Scoring is threshold-based — results with score above a configurable `threshold` are excluded.

**Bundle size:** ~8 kB gzipped (full), ~6.5 kB gzipped (basic build). Zero dependencies.

**Multi-field support:** First-class. The `keys` config accepts an array of field names with optional weights:
```ts
const fuse = new Fuse(sessions, {
  keys: [
    { name: "title", weight: 0.5 },
    { name: "branch", weight: 0.3 },
    { name: "path", weight: 0.1 },
    { name: "tags", weight: 0.1 },
  ],
  includeScore: true,
  threshold: 0.4,
});
```

**TypeScript:** Full TypeScript types included. `fuse.search(query)` returns `Array<{ item, score, refIndex }>`.

**Assessment:** The most capable option for multi-field weighted search. Bitap scoring is solid, though it does not give explicit consecutive-char or word-start bonuses the way fzf does. The threshold-based filter can cut legitimate matches if tuned poorly. Good fit for session search where multiple fields matter.

---

### `match-sorter`

**Algorithm:** Seven-tier ranking (case-sensitive equals → case-insensitive equals → starts-with → word-starts-with → contains → acronym → simple match). Not a fuzzy-distance algorithm — it is a ranked filter.

**Bundle size:** Not published in official docs, but the package is minimal (no runtime dependencies beyond `@babel/runtime`). Estimated ~3–4 kB gzipped.

**Multi-field support:** Yes, via `keys` array with nested dot notation, array wildcards, and custom getter callbacks. Items matching across multiple keys get the best (lowest) rank from any of their fields.

**Assessment:** Excellent for UX quality — the tier system ensures that "starts-with" results always appear above "contains" results. However, it is not true fuzzy matching: "myfeat" will not match "my-feature-branch" because none of the tiers handle non-contiguous character patterns. This makes it unsuitable as the sole matching strategy for the "≤3 keystrokes" requirement. It could be used as a post-filter or ranking layer on top of another algorithm.

---

### `fzf` npm package (`fzf-for-js`)

**Package:** `fzf` on npm (https://github.com/ajitid/fzf-for-js), version 0.5.2 as of research date.

**Algorithm:** A port of fzf's main algorithm to browser/Node context. Implements the same Smith-Waterman-like scoring with consecutive-char bonus and word-boundary bonus as the fzf CLI. This is the closest TypeScript equivalent to `sahilm/fuzzy` in Go.

**Bundle size:** Not prominently documented. As a port of the fzf algorithm in pure TypeScript it should be small (estimated <10 kB), but no authoritative gzipped size found.

**Multi-field support:** Not first-class. The fzf algorithm operates on a single string. Multi-field search requires building a composite string (e.g., `${title} ${branch} ${path} ${tags.join(' ')}`) or running multiple searches and merging.

**Assessment:** Best single-field fuzzy quality (closest to fzf CLI feel). Not ideal for the multi-field weighted scoring needed for session search. Composite-string workaround is functional but loses per-field weight control.

---

### Recommendation (TypeScript)

**Use `fuse.js`** for session search in the Omnibar. The multi-field weighted `keys` configuration is exactly what's needed to rank title matches above path matches above tag matches. The Bitap algorithm produces good enough fuzzy results for a session list. Bundle size (~8 kB gzip) is acceptable given the existing bundle is already 5 MB limit.

For path completion filtering (which is single-field, operating on directory entry names), **`fzf-for-js`** would produce better feel (consecutive/word-boundary bonuses) if the current `strings.HasPrefix` filter is replaced. This is a separate decision from session search.

Neither library is currently in `web-app/package.json` — both would be new additions.

---

## Client-side vs Server-side Session Search

**Recommendation: Client-side only.**

Sessions are already loaded into Redux state (`sessionsSlice`) via `useSessionService`. The `selectAllSessions` selector gives instant O(1) access to the full session list. A typical deployment has tens to low-hundreds of sessions — well within the range where JavaScript fuzzy matching on pre-loaded data is indistinguishable from instant.

Factors that confirm client-side:
1. **Data already in memory.** The Redux store holds the complete `Session[]`. There is no I/O cost.
2. **Session count is bounded.** Even at 500 sessions, `fuse.js` Bitap runs in <1ms.
3. **Latency budget.** The requirement is "reach any session in ≤3 keystrokes." A round-trip RPC (even on localhost) adds 5–20ms latency per keystroke. Client-side filtering is synchronous.
4. **No new proto/service needed.** Adding a `SearchSessions` RPC endpoint would require proto changes, code generation, server implementation, and transport — all for a problem that JavaScript solves in 10 lines.
5. **Offline/reconnect resilience.** Client-side filtering works during brief WebSocket reconnects; server search would degrade.

**The existing BM25 `SearchEngine` in `session/search/` is irrelevant for this use case.** It indexes Claude conversation message history (full-text search within chat), not session metadata. Session search (by title, branch, path, tags) is a different problem entirely and should remain client-side.

---

## Directory Tree Caching Patterns

Four patterns are viable for caching `os.ReadDir` results in Go:

### Pattern 1: In-memory map with mtime check (recommended)

```go
type dirCacheEntry struct {
    entries []os.DirEntry
    mtime   time.Time
    cachedAt time.Time
}
var cache sync.Map // key: string (baseDir), value: *dirCacheEntry
```

On each call: `os.Stat(baseDir)` to get mtime. If mtime matches cached entry, return cache hit. If mtime changed or entry missing, call `os.ReadDir` and store new entry.

**Pros:** Correct invalidation (mtime changes when directory contents change). No background goroutine needed. Low memory overhead (only entries requested by users).
**Cons:** `os.Stat` on every call, but `os.Stat` is a single syscall (~1µs) — acceptable overhead for a 100ms cold / <100ms warm target. Race between stat and readdir is negligible for the UX use case.

### Pattern 2: TTL-based expiry (simpler, slightly less correct)

Cache entries expire after a fixed TTL (e.g., 30s). No mtime check.

**Pros:** Simpler code. Works for the UX case where "eventual consistency" within 30s is fine.
**Cons:** Stale results between directory change and TTL expiry. For the "recent repos" use case this is mostly fine, but a user who just created a new directory would see it missing until TTL expires.

This is exactly what `usePathCompletions.ts` already does on the client side (30s TTL LRU). Adding the same pattern server-side doubles the protection.

### Pattern 3: `sync.RWMutex` map vs `sync.Map`

`sync.Map` is optimized for read-heavy workloads with infrequent writes — which is the directory cache profile exactly (many reads, rare writes). Use `sync.Map` over a custom `RWMutex` map unless profiling shows contention.

### Pattern 4: Disk-backed JSON

Persist the cache to `~/.stapler-squad/dir_cache.json` on writes, load on startup. Survives server restarts.

**Assessment:** Complexity is high for marginal gain. The cold-start cost (first open after restart) is already budgeted at <500ms in the requirements. A disk-backed cache would only matter if the <500ms cold target is being violated in practice.

### Recommendation

**Pattern 1 (mtime-keyed in-memory sync.Map) as the primary cache, with Pattern 2 (TTL) as a safety fallback.** Specifically:

- Key: `baseDir` (absolute path after tilde expansion)
- Invalidation: compare `os.Stat(baseDir).ModTime()` against cached mtime
- Fallback TTL: 60s (to handle cases where mtime isn't updated, e.g., network filesystems)
- Max cache size: 200 entries (covers typical home directory tree traversal depth)
- No disk persistence initially — implement only if cold-start timing fails the <500ms requirement

---

## Existing `path_completion_service.go` Analysis

### Current approach

`PathCompletionService` is **stateless** by design (the struct comment says so explicitly). Every RPC call:

1. Expands `~` in the path prefix (`expandTilde`)
2. Calls `os.Stat(expanded)` to check if the full path exists as a directory
3. Calls `splitPathPrefix(expanded)` to separate `baseDir` and `filterPrefix`
4. Calls `os.Stat(baseDir)` to check if the base directory exists
5. Spawns a goroutine to call `os.ReadDir(baseDir)` with a 2s timeout via channel
6. Filters entries: skip hidden unless filter starts with `.`, skip non-matching prefixes, skip non-directories if `directoriesOnly=true`, follow symlinks
7. Returns up to `maxResults` (default 100, hard max 500) entries

The service also has a `ListWorktrees` method that runs `git worktree list --porcelain` via `exec.CommandContext` — this is also uncached.

### Where a cache slots in

The cache should be inserted at step 5 — wrapping the `os.ReadDir(baseDir)` call:

```
Before step 5: check cache[baseDir]
  Cache hit (mtime unchanged): skip goroutine + ReadDir, use cached entries
  Cache miss / mtime changed: proceed with ReadDir goroutine, store result in cache

After ReadDir returns: store entries + mtime in cache
```

The struct must become stateful to hold the cache:

```go
type PathCompletionService struct {
    cache sync.Map  // map[string]*dirCacheEntry
}
```

Steps 6 (filtering) and 7 (pagination) still run on every request — they are cheap CPU operations on the cached entries, and the filter depends on the user's input which changes per-call.

The `ListWorktrees` method (`git worktree list --porcelain`) is a separate caching candidate. It runs a subprocess, which is expensive (~50ms). A cache keyed on `repoPath + worktree list mtime` (or simply a short TTL cache on `repoPath`) would eliminate redundant subprocess invocations. This is lower priority than the directory listing cache since worktree listing is called once per repo path selection, not on every keystroke.

**Key implementation note:** The `NewPathCompletionService()` constructor currently takes no arguments. After adding the cache field, the constructor signature stays the same (`sync.Map` zero value is ready to use). No change needed to callers.

---

## Recommendations Summary

| Decision | Recommendation | Rationale |
|---|---|---|
| Go fuzzy library | `github.com/sahilm/fuzzy` | Only Go library with consecutive-char + word-boundary bonuses; zero deps; sub-ms for hundreds of items |
| TypeScript fuzzy library | `fuse.js` | Best multi-field weighted scoring; 8kB gzip; first-class TypeScript types |
| Session search location | Client-side only (no RPC) | Sessions already in Redux state; 0 round-trip; synchronous; no proto changes needed |
| Directory cache strategy | In-memory `sync.Map`, mtime-keyed, 60s TTL fallback | Correct invalidation, low complexity, no background goroutines |
| Cache insertion point | Wrap `os.ReadDir(baseDir)` in `ListPathCompletions` | `PathCompletionService` becomes stateful; constructor unchanged; filtering still per-request |
| `ListWorktrees` cache | Short TTL (5–10s) keyed on `repoPath` | Subprocess is expensive; worktree lists change rarely |
