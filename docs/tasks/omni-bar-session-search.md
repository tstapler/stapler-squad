# Implementation Plan: Omni Bar Session Search

Status: Ready for Implementation
Created: 2026-04-14
Branch: claude-squad-better-search-fuzzy
MDD Spec: `project_plans/omni-bar-session-search/`

---

## Epic Overview

**User value:** Reach any existing session via the Omnibar in 3 keystrokes or fewer after Cmd+K; start a new session on any previously-used repo in 5 keystrokes or fewer.

**Success metrics:**
- Path completion results appear in <100ms on cache hit (repeated open); <500ms cold
- Session search results update on every keypress with no perceptible delay (<16ms)
- Zero new RPC endpoints for session search (client-side only)
- Zero proto changes

**Scope — Must Have:**
1. Session navigation — fuzzy search existing sessions from Omnibar; selecting navigates to that session
2. Recent repos quick-pick — frecent paths shown in empty state; selecting pre-fills path for new session creation
3. Fuzzy matching quality — Fuse.js multi-field weighted session search
4. Directory tree cache — stateful `DirCache` in `PathCompletionService` with mtime + TTL invalidation

**Out of scope:**
- Global OS-level hotkey (system-wide Cmd+K)
- AI/LLM suggestions
- Non-session navigation (Settings, Help, etc.)
- Mobile/responsive layout
- Remote path support (SSH, network mounts)
- Disk-backed cache persistence
- Worktree list caching (lower priority; separate follow-up)

---

## Dependency Diagram

```
Story 1: Go Directory Cache (backend)
    |
    | (independent — ship any time)
    |
Story 2: Detector + usePathHistory Fixes (frontend plumbing)
    |
    +---> Story 3: useSessionSearch hook
              |
              +---> Story 4: OmnibarResultList UI components
                        |
                        +---> Story 5: Omnibar Two-Phase Integration (wires everything)
```

Stories 1 and 2 are fully independent and can be worked in parallel. Story 3 depends on Story 2 (needs `InputType.SessionSearch`). Story 4 depends on Story 3 (needs the hook's return type to define component props). Story 5 depends on Stories 2, 3, and 4.

---

## Story 1: Go Directory Cache

**Value:** Eliminates redundant `os.ReadDir` calls on every Omnibar open. Meets the <100ms repeated-open requirement.

**Files:**
- `server/services/dir_cache.go` — new file, `DirCache` struct
- `server/services/path_completion_service.go` — add `cache *DirCache` field, wrap ReadDir call

### Task 1.1 — Implement DirCache

**File:** `server/services/dir_cache.go` (new)

**What to build:**

```go
package services

import (
    "os"
    "sync"
    "time"
)

type dirCacheEntry struct {
    entries  []os.DirEntry
    dirMtime time.Time
    cachedAt time.Time
}

type DirCache struct {
    mu      sync.RWMutex
    entries map[string]*dirCacheEntry
    maxSize int
    ttl     time.Duration
}

func NewDirCache(maxSize int, ttl time.Duration) *DirCache {
    return &DirCache{
        entries: make(map[string]*dirCacheEntry, maxSize),
        maxSize: maxSize,
        ttl:     ttl,
    }
}

// Get returns cached DirEntry slice for path if it is still valid.
// Validity: cachedAt within TTL AND directory mtime unchanged.
// Returns (nil, false) on miss, stale TTL, or changed mtime.
func (c *DirCache) Get(path string) ([]os.DirEntry, bool) { ... }

// Put stores entries for path. Evicts oldest entry when at maxSize.
func (c *DirCache) Put(path string, entries []os.DirEntry, dirMtime time.Time) { ... }

// evictOldest removes the entry with the oldest cachedAt. Caller holds write lock.
func (c *DirCache) evictOldest() { ... }
```

**Implementation notes:**
- `Get`: acquire read lock, check entry exists; if not, return miss. Check `time.Since(entry.cachedAt) > c.ttl` — return miss if expired. Call `os.Stat(path)` to get current mtime; if `info.ModTime().After(entry.dirMtime)` return miss (delete entry under write lock). Otherwise return entries.
- `Put`: acquire write lock, if `len(c.entries) >= c.maxSize` call `evictOldest()`. Store new entry.
- `evictOldest`: O(n) scan to find entry with smallest `cachedAt`. Acceptable at n=256.
- No goroutines; no background cleanup; all operations are on-demand.

**Tests to write** (`server/services/dir_cache_test.go`):
1. `TestDirCache_Hit` — Put then Get returns same entries
2. `TestDirCache_MissOnMtimeChange` — Put, then simulate mtime change via temp dir, Get returns miss
3. `TestDirCache_MissOnTTLExpiry` — Put with 1ms TTL, sleep, Get returns miss
4. `TestDirCache_Eviction` — Fill to maxSize, add one more, total entries stays at maxSize
5. `TestDirCache_ConcurrentReads` — 10 goroutines Get simultaneously after one Put, no data race (run with `-race`)

**INVEST:** Independent (no frontend dependency), Negotiable (eviction policy), Valuable (hits <100ms target), Estimable (1-2 hours), Small (single file + test file), Testable (all behaviors have unit tests).

---

### Task 1.2 — Wire DirCache into PathCompletionService

**File:** `server/services/path_completion_service.go`

**Changes:**

1. Change struct definition:
```go
// Before
type PathCompletionService struct{}

// After
type PathCompletionService struct {
    cache *DirCache
}
```

2. Change constructor:
```go
// Before
func NewPathCompletionService() *PathCompletionService {
    return &PathCompletionService{}
}

// After
func NewPathCompletionService() *PathCompletionService {
    return &PathCompletionService{
        cache: NewDirCache(256, 60*time.Second),
    }
}
```

3. In `ListPathCompletions`, after the `baseDirExists` check (line 76), capture mtime from the existing `baseDirInfo`:
```go
// baseDirInfo is already computed above (line 67-68)
dirMtime := baseDirInfo.ModTime()
```

4. Replace the goroutine+channel ReadDir block (lines 78-112) with a cache-aware wrapper:
```go
// Check cache first
var dirEntries []os.DirEntry
if cached, ok := p.cache.Get(baseDir); ok {
    dirEntries = cached
} else {
    // existing goroutine+channel ReadDir logic producing dirEntries
    // ... (unchanged from current implementation)
    if readErr == nil {
        p.cache.Put(baseDir, dirEntries, dirMtime)
    }
}
```

**No other changes.** Filtering, pagination, and response construction are unchanged.

**Tests to write** (`server/services/path_completion_service_test.go` — extend existing):
1. `TestListPathCompletions_CacheHit` — call twice for same path, second call returns same entries without new ReadDir
2. `TestListPathCompletions_CacheMissAfterMtimeChange` — call once, modify directory, call again, gets fresh entries

**INVEST:** Depends only on Task 1.1, small change to one function, testable via integration test.

---

## Story 2: Detector + usePathHistory Fixes

**Value:** Makes bare-text queries (e.g., "squad", "my-feature") route to session-search mode instead of `InputType.Unknown`. Adds `getAll()` to expose recent repos without prefix filtering. Fixes the `dropdownDismissed` stickiness bug between mode transitions.

**Files:**
- `web-app/src/lib/omnibar/types.ts` — add `InputType.SessionSearch`
- `web-app/src/lib/omnibar/detector.ts` — add `SessionSearchDetector`, register at priority 200
- `web-app/src/lib/hooks/usePathHistory.ts` — add `getAll(limit)` method, expose from hook return

### Task 2.1 — Add InputType.SessionSearch

**File:** `web-app/src/lib/omnibar/types.ts`

Add to `InputType` enum:
```typescript
SessionSearch = "session_search",
```

Add to `INPUT_TYPE_INFO`:
```typescript
[InputType.SessionSearch]: {
  label: "Search Sessions",
  icon: "🔍",
  description: "Search existing sessions",
},
```

**File:** `web-app/src/lib/omnibar/detector.ts`

Add new detector class after `LocalPathDetector`:
```typescript
/**
 * Session Search detector — catch-all for bare-text queries.
 * Fires for any non-empty input that no other detector claimed.
 * Priority 200 ensures it runs last (after LocalPath at priority 100).
 */
class SessionSearchDetector implements Detector {
  name = "SessionSearch";
  priority = 200;

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();
    if (!trimmed) return null;
    // All inputs that reach this point have already been rejected by
    // GitHub URL detectors (priorities 10-40), PathWithBranch (50),
    // and LocalPath (100). They are bare-text session search queries.
    return {
      type: InputType.SessionSearch,
      confidence: 0.5,
      parsedValue: trimmed,
      suggestedName: "",
    };
  }
}
```

Register in `createDefaultRegistry()`:
```typescript
registry.register(new SessionSearchDetector());
```

**Critical:** The empty-input case (`input === ""`) is handled by `Omnibar.tsx` which sets `detection = null` when input is empty (line 171-173). The `SessionSearchDetector` must return `null` for empty input to preserve this behavior.

**Pitfall addressed:** P1 — Detector mode conflict. Without this, bare-text queries resolve to `InputType.Unknown`, blocking session result display and flagging `canSubmit = false`.

**Tests:** Add to existing detector tests:
1. `"squad"` → `InputType.SessionSearch`
2. `"my-feature"` → `InputType.SessionSearch`
3. `""` → falls through to `InputType.Unknown` (the registry default)
4. `"~/projects"` → still `InputType.LocalPath` (not hijacked by SessionSearch)
5. `"org/repo"` → still `InputType.GitHubShorthand`

---

### Task 2.2 — Add getAll() to usePathHistory

**File:** `web-app/src/lib/hooks/usePathHistory.ts`

Add `getAll` alongside `getMatching`:
```typescript
/**
 * Return all stored paths sorted by score desc, limited to `limit` entries.
 * Used for "Recent Repos" empty-state display when no query is active.
 */
const getAll = useCallback(
  (limit: number = MAX_RESULTS): PathHistoryEntry[] => {
    return [...entries]
      .sort((a, b) => entryScore(b) - entryScore(a))
      .slice(0, limit);
  },
  [entries]
);
```

Update hook return value:
```typescript
// Before
return { getMatching, save };

// After
return { getMatching, getAll, save };
```

**No other changes.** The `entryScore` function and storage logic are unchanged.

**Pitfall addressed:** P3 — `getMatching` is prefix-only; `getAll` is needed for the empty-state recent repos list.

**Tests:** Add to `usePathHistory` tests:
1. `getAll(5)` returns top 5 by score when more than 5 entries exist
2. `getAll()` returns all entries sorted by score when fewer than limit
3. After `save(path)`, `getAll()` includes the newly saved path
4. Score ordering: entry used 2 hours ago > entry used 3 weeks ago

---

### Task 2.3 — Fix dropdownDismissed stickiness on mode transition

**File:** `web-app/src/components/sessions/Omnibar.tsx`

In the detection `useEffect` (lines 146-181), after `setDetection(result)`, add:
```typescript
// Reset dropdown dismissed state when input type changes modes.
// Prevents session results from being suppressed after user dismisses
// path completion dropdown then backspaces to bare text.
setDropdownDismissed(false);
```

This reset must happen on every detection type change, not just on character input (which already resets it). The detection runs 150ms debounced, so the reset happens at most 150ms after the mode switch — acceptable UX.

**Pitfall addressed:** P2 — `dropdownDismissed` stickiness. If user types a path, presses Escape to dismiss path completions (sets `dropdownDismissed = true`), then backspaces to bare text, session results would be suppressed without this fix.

**Tests:** Verify in OmnibarResultList integration test that session results appear after: type path → press Escape → backspace to bare text → type session query.

**INVEST:** All three tasks in Story 2 are small, independently testable changes to separate files. Together they form a coherent plumbing story required before Stories 3-5.

---

## Story 3: useSessionSearch Hook

**Value:** Client-side fuzzy session search using Fuse.js. Powers the sessions section of the result list.

**Prerequisite:** Story 2 (needs `InputType.SessionSearch` to know when to show results).

**Files:**
- `web-app/package.json` — add `fuse.js` dependency
- `web-app/src/lib/hooks/useSessionSearch.ts` — new file

### Task 3.1 — Install fuse.js

```bash
cd web-app && npm install fuse.js
```

Verify `"fuse.js"` appears in `web-app/package.json` dependencies. No other configuration changes.

**Version note:** Use fuse.js v7.x (latest stable). The API used here (`new Fuse(list, options)`, `.search(query)`) is stable since v6.

---

### Task 3.2 — Implement useSessionSearch hook

**File:** `web-app/src/lib/hooks/useSessionSearch.ts` (new)

```typescript
"use client";

import { useMemo } from "react";
import Fuse from "fuse.js";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";

export interface SessionSearchResult {
  session: Session;
  score: number; // 0.0 = perfect match, 1.0 = no match (Fuse.js convention)
  matchedFields: string[]; // which fields contributed to the match
}

// Fuse.js config: multi-field weighted search
// title:0.5 > branch:0.3 > path:0.15 > tags:0.05
// threshold:0.4 — allow moderate fuzziness; tighten if too many false positives
// minMatchCharLength:1 — single character queries return results
const FUSE_OPTIONS: Fuse.IFuseOptions<Session> = {
  keys: [
    { name: "title", weight: 0.5 },
    { name: "branch", weight: 0.3 },
    { name: "path", weight: 0.15 },
    { name: "tags", weight: 0.05 },
  ],
  includeScore: true,
  includeMatches: true,
  threshold: 0.4,
  minMatchCharLength: 1,
  ignoreLocation: true, // don't penalize matches far from string start
};

// Sessions in these statuses are excluded from results
const EXCLUDED_STATUSES = new Set([
  SessionStatus.STOPPED,
  SessionStatus.UNSPECIFIED,
]);

/**
 * Client-side fuzzy session search using Fuse.js.
 * Returns ranked results for display in the Omnibar session section.
 * Empty query returns empty array (empty state shows recents instead).
 */
export function useSessionSearch(query: string): SessionSearchResult[] {
  const sessions = useAppSelector(selectAllSessions);

  // Filter to active sessions only
  const activeSessions = useMemo(
    () => sessions.filter((s) => !EXCLUDED_STATUSES.has(s.status)),
    [sessions]
  );

  // Rebuild Fuse index when active session list changes
  const fuse = useMemo(
    () => new Fuse(activeSessions, FUSE_OPTIONS),
    [activeSessions]
  );

  return useMemo(() => {
    const trimmed = query.trim();
    if (!trimmed) return [];

    const results = fuse.search(trimmed, { limit: 8 });

    return results.map((r) => ({
      session: r.item,
      score: r.score ?? 1.0,
      matchedFields: r.matches?.map((m) => m.key ?? "") ?? [],
    }));
  }, [query, fuse]);
}
```

**Implementation notes:**
- `ignoreLocation: true` — prevents Fuse.js from penalizing matches that appear deep in the path string. Without this, "squad" matching the tail of "stapler-squad" would score lower than if "squad" appeared at position 0.
- `limit: 8` passed to `fuse.search()` — caps results before any filtering.
- The Fuse index (`new Fuse(...)`) is rebuilt only when `activeSessions` changes, not on every query. The `.search()` call runs on every query change; this is the fast path (<1ms for hundreds of sessions).
- `STOPPED` and `UNSPECIFIED` sessions excluded. Navigating to a stopped session is a confusing UX (the terminal is not running).

**Tests** (`web-app/src/lib/hooks/useSessionSearch.test.ts`):
1. Empty query returns `[]`
2. Query matching title scores higher than same query matching only path
3. STOPPED sessions are excluded from results
4. Results are capped at 8
5. "squad" matches session with title "stapler-squad" (fuzzy non-prefix match)
6. "myfeat" matches branch "my-feature-branch" (character-sequence fuzzy)
7. Title weight dominance: session A with "auth" in title outranks session B with "auth" only in path

**INVEST:** Independent of UI stories, fully testable in isolation, clear acceptance criteria.

---

## Story 4: OmnibarResultList UI Components

**Value:** Renders the two-section result list (sessions + repos) with keyboard navigation. Visually distinguishes session results from repo results.

**Prerequisite:** Story 3 (needs `SessionSearchResult` type). Also needs `PathHistoryEntry` from Story 2's `getAll()`.

**Files:**
- `web-app/src/components/sessions/OmnibarResultList.tsx` — new
- `web-app/src/components/sessions/OmnibarResultList.css.ts` — new (vanilla-extract)
- `web-app/src/components/sessions/OmnibarSessionResult.tsx` — new
- `web-app/src/components/sessions/OmnibarSessionResult.css.ts` — new
- `web-app/src/components/sessions/OmnibarRepoResult.tsx` — new
- `web-app/src/components/sessions/OmnibarRepoResult.css.ts` — new

### Task 4.1 — OmnibarSessionResult row component

**File:** `web-app/src/components/sessions/OmnibarSessionResult.tsx` (new)

Visual layout:
```
[status dot]  [session title (bold)]           [branch (muted, right)]
              [repo path (muted, smaller)]
```

Props interface:
```typescript
interface OmnibarSessionResultProps {
  session: Session;
  isHighlighted: boolean;
  onSelect: (session: Session) => void;
}
```

Status dot: colored `<span>` with `aria-label` set to status string.
- `SessionStatus.RUNNING` → green (#22c55e)
- `SessionStatus.PAUSED` → yellow (#eab308)
- `SessionStatus.READY` → blue (#3b82f6)
- Other → grey (#6b7280)

Path display: show only the last 2 segments of the path to avoid overflow (e.g., `/Users/tyler/projects/auth` → `projects/auth`).

**Accessibility:** The row is a `<li role="option">` inside the `OmnibarResultList`'s `<ul role="listbox">`. `aria-selected` reflects `isHighlighted`. `id` must be `omnibar-result-session-{session.id}` for `aria-activedescendant` on the input.

**CSS** (`OmnibarSessionResult.css.ts`): Use `vars` from `web-app/src/styles/theme.css.ts`. Two-line layout via flex-column. Status dot is a 8px `border-radius: 50%` span with inline color.

---

### Task 4.2 — OmnibarRepoResult row component

**File:** `web-app/src/components/sessions/OmnibarRepoResult.tsx` (new)

Visual layout:
```
[folder icon]  [parent/path (muted)] / [repo-name (bold)]    [relative time (muted, right)]
               [N sessions] (muted, small)
```

Props interface:
```typescript
interface OmnibarRepoResultProps {
  entry: PathHistoryEntry;
  sessionCount?: number;
  isHighlighted: boolean;
  onSelect: (path: string) => void;
}
```

Path splitting: `const parts = path.split('/').filter(Boolean); const repoName = parts[parts.length - 1]; const parent = parts.slice(-3, -1).join('/');`

Relative time: use a simple inline util (no library needed):
```typescript
function relativeTime(epochMs: number): string {
  const diff = Date.now() - epochMs;
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
  if (diff < 7 * 86_400_000) return `${Math.floor(diff / 86_400_000)}d ago`;
  return `${Math.floor(diff / (7 * 86_400_000))}w ago`;
}
```

Accessibility: `<li role="option">`, same `aria-selected` / `id` pattern as `OmnibarSessionResult`. Id format: `omnibar-result-repo-{encodeURIComponent(entry.path)}`.

---

### Task 4.3 — OmnibarResultList container component

**File:** `web-app/src/components/sessions/OmnibarResultList.tsx` (new)

This component owns the keyboard navigation state for the result list and renders both sections.

Props interface:
```typescript
interface OmnibarResultListProps {
  sessionResults: SessionSearchResult[];
  repoEntries: PathHistoryEntry[];
  sessionCounts?: Record<string, number>; // path → session count, for repo rows
  onSessionSelect: (session: Session) => void;
  onRepoSelect: (path: string) => void;
  highlightedIndex: number; // controlled from parent (Omnibar)
  // "Create new session" always-present item at bottom
  onCreateNew: () => void;
}
```

**Rendering logic:**
- Show "SESSIONS" section header + session rows if `sessionResults.length > 0`
- Show "REPOS" section header + repo rows if `repoEntries.length > 0`
- Show "+ New Session" item at bottom with a visual separator (always present)
- Hide entire component if both sections are empty and no create-new item needed

**Section headers:** `<div role="presentation" aria-hidden="true">` — they are visual only, not keyboard-navigable.

**Flat index management:** The component exposes a `totalItemCount` (sessions + repos + 1 for create-new) so the parent Omnibar can drive arrow-key navigation. The `highlightedIndex` prop maps: 0..sessionResults.length-1 = sessions, sessionResults.length..sessionResults.length+repoEntries.length-1 = repos, last = create-new.

**ARIA:** The list is `<ul role="listbox" id="omnibar-result-listbox">`. The parent input's `aria-controls` points to this id. `aria-activedescendant` on the input tracks the highlighted item's id.

**CSS** (`OmnibarResultList.css.ts`):
```typescript
import { style } from '@vanilla-extract/css';
import { vars } from '../../styles/theme.css';

export const resultList = style({
  listStyle: 'none',
  margin: 0,
  padding: `${vars.space[1]} 0`,
  maxHeight: '320px',
  overflowY: 'auto',
});

export const sectionHeader = style({
  padding: `${vars.space[1]} ${vars.space[3]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  letterSpacing: '0.08em',
  textTransform: 'uppercase',
  color: vars.color.textMuted,
  userSelect: 'none',
});

export const separator = style({
  height: '1px',
  background: vars.color.borderDefault,
  margin: `${vars.space[1]} ${vars.space[3]}`,
});
```

**Tests** (`web-app/src/components/sessions/OmnibarResultList.test.tsx`):
1. Renders session section only when sessionResults non-empty, repoEntries empty
2. Renders repo section only when repoEntries non-empty, sessionResults empty
3. Hides empty sections (no "SESSIONS" header when sessionResults is `[]`)
4. "+ New Session" item always renders
5. Arrow key down from last session item moves highlight to first repo item (skips section header)
6. `onSessionSelect` called with correct session on Enter when session highlighted
7. `onRepoSelect` called with correct path on Enter when repo highlighted

**INVEST:** Depends on Tasks 4.1 and 4.2 for row components, but those can be stubbed for testing. All navigation logic is testable with shallow render.

---

## Story 5: Omnibar Two-Phase Integration

**Value:** Wires session search, recent repos, and the result list into the existing Omnibar component. Implements the two-phase (discovery/creation) mode state. This is the story that delivers the full feature end-to-end.

**Prerequisite:** Stories 2, 3, and 4.

**Files:**
- `web-app/src/components/sessions/Omnibar.tsx` — significant additions, no deletion of existing form logic
- `web-app/src/components/sessions/Omnibar.module.css` — minor additions for mode-transition styles

### Task 5.1 — Add mode state and session navigation callback

**File:** `web-app/src/components/sessions/Omnibar.tsx`

**New prop:**
```typescript
interface OmnibarProps {
  isOpen: boolean;
  onClose: () => void;
  onCreateSession: (data: OmnibarSessionData) => Promise<void>;
  onNavigateToSession: (sessionId: string) => void; // NEW
}
```

**New state:**
```typescript
type OmnibarMode = "discovery" | "creation";
const [mode, setMode] = useState<OmnibarMode>("discovery");
const [resultHighlightIndex, setResultHighlightIndex] = useState(-1);
```

**Mode logic:**
- `mode` starts as `"discovery"` on open
- Transitions to `"creation"` when:
  - User selects a repo result (path pre-filled into input)
  - Input is detected as `InputType.LocalPath` or `InputType.PathWithBranch` (existing path detection)
- Transitions back to `"discovery"` when:
  - Input is detected as `InputType.SessionSearch`
  - Input is cleared (empty)

Add to the detection `useEffect`, after `setDetection(result)`:
```typescript
if (
  result.type === InputType.LocalPath ||
  result.type === InputType.PathWithBranch ||
  result.type === InputType.GitHubPR ||
  result.type === InputType.GitHubBranch ||
  result.type === InputType.GitHubRepo ||
  result.type === InputType.GitHubShorthand
) {
  setMode("creation");
} else if (result.type === InputType.SessionSearch) {
  setMode("discovery");
}
```

When input is cleared:
```typescript
// In the else branch (empty input, line 171-173):
setDetection(null);
setMode("discovery");
```

---

### Task 5.2 — Wire session search and recent repos hooks

**File:** `web-app/src/components/sessions/Omnibar.tsx`

Add hooks at the top of the component body:

```typescript
import Fuse from "fuse.js";

// Session search (only active in discovery mode)
const sessionSearchQuery =
  detection?.type === InputType.SessionSearch ? input : "";
const sessionResults = useSessionSearch(sessionSearchQuery);

// Recent repos for empty-state discovery
const { getAll: getAllHistory } = usePathHistory();
// useRepositorySuggestions for frecency-ranked paths from existing sessions
const { suggestions: repoSuggestions } = useRepositorySuggestions();

// What to show in the result list
const isDiscoveryMode = mode === "discovery" || !input.trim();

// Stable snapshot of recent repo entries — rebuilt only when history changes.
// Capped at 50 so Fuse index stays small.
const allRepoEntries = useMemo(() => getAllHistory(50), [getAllHistory]);

// Fuse instance over repo paths — same threshold/ignoreLocation as session search
// for consistent match quality. Rebuilt only when allRepoEntries changes.
const repoFuse = useMemo(
  () =>
    new Fuse(allRepoEntries, {
      keys: [{ name: "path", weight: 1.0 }],
      threshold: 0.4,
      ignoreLocation: true,
      minMatchCharLength: 1,
    }),
  [allRepoEntries]
);

// Empty state: show top-5 recent sessions + top-5 repo suggestions
// Active search: show session results + fuzzy-filtered repo suggestions
const displayedSessionResults = useMemo(() => {
  if (!input.trim()) {
    // Empty state — not yet implemented here; sessions come from Redux
    // selectAllSessions sorted by updatedAt, top 5
    return []; // populated in Task 5.3
  }
  return sessionResults;
}, [input, sessionResults]);

const displayedRepoEntries = useMemo(() => {
  if (!input.trim()) {
    // Empty state — top 5 by frecency score
    return allRepoEntries.slice(0, 5);
  }
  // Active search — fuzzy match against path (same quality as session search)
  return repoFuse.search(input).map((r) => r.item).slice(0, 8);
}, [input, allRepoEntries, repoFuse]);
```

**Why fuse.js here too:** The requirements specify fuzzy search for previously-used repos ("create a new session based on a repo I've already used before based on fuzzy search"). Substring `.includes()` would miss "squad" → "stapler-squad". Since fuse.js is already installed for session search (Story 3), there is no additional dependency cost. The same `threshold: 0.4` and `ignoreLocation: true` settings are used for consistency.

**Fuse instance stability:** `allRepoEntries` is memoized from `getAllHistory(50)`. `getAllHistory` is a `useCallback` inside `usePathHistory` — it only changes reference when the stored entries change (i.e. when `save()` is called). In practice the Fuse index rebuilds only when the user creates a new session or uses a new path, not on every keystroke.

---

### Task 5.3 — Empty state: recent sessions from Redux

**File:** `web-app/src/components/sessions/Omnibar.tsx`

For the empty-state session list, select top-5 sessions from Redux sorted by `updatedAt` desc:

```typescript
// At hook level (outside useMemo, to get selector):
const allSessions = useAppSelector(selectAllSessions);

// Inside displayedSessionResults useMemo, empty-state branch:
if (!input.trim()) {
  const active = allSessions
    .filter((s) => s.status !== SessionStatus.STOPPED && s.status !== SessionStatus.UNSPECIFIED)
    .sort((a, b) => {
      const aTime = a.updatedAt ? Number(a.updatedAt.seconds) : 0;
      const bTime = b.updatedAt ? Number(b.updatedAt.seconds) : 0;
      return bTime - aTime;
    })
    .slice(0, 5);
  return active.map((s) => ({ session: s, score: 0, matchedFields: [] }));
}
```

---

### Task 5.4 — Handle session result selection

**File:** `web-app/src/components/sessions/Omnibar.tsx`

Add handler:
```typescript
const handleSessionSelect = useCallback(
  (session: Session) => {
    onNavigateToSession(session.id);
    onClose();
  },
  [onNavigateToSession, onClose]
);
```

**Pitfall addressed (P1):** Enter key ambiguity. The `handleKeyDown` must route Enter by current context:
- If `isDiscoveryMode` and `resultHighlightIndex >= 0` (a result is highlighted):
  - If highlighted item is a session → call `handleSessionSelect`
  - If highlighted item is a repo → call `handleRepoSelect`
  - If highlighted item is create-new → call `handleCreateNew`
  - In all cases, `e.preventDefault()` and `return` before the Cmd+Enter check
- Otherwise, existing Cmd+Enter → `handleSubmit` behavior unchanged

```typescript
// Inside handleKeyDown, at the top of the function (before isDropdownVisible check):
if (isDiscoveryMode && resultHighlightIndex >= 0) {
  if (e.key === "ArrowDown") {
    e.preventDefault();
    setResultHighlightIndex((i) => Math.min(i + 1, totalResultCount - 1));
    return;
  }
  if (e.key === "ArrowUp") {
    e.preventDefault();
    setResultHighlightIndex((i) => Math.max(i - 1, -1));
    return;
  }
  if (e.key === "Enter" && !e.metaKey) {
    e.preventDefault();
    // dispatch based on which item is highlighted
    dispatchHighlightedResultAction(resultHighlightIndex);
    return;
  }
  if (e.key === "Escape") {
    e.nativeEvent.stopImmediatePropagation();
    setResultHighlightIndex(-1);
    return; // first Escape clears highlight; second Escape closes (falls through)
  }
}
```

---

### Task 5.5 — Handle repo result selection (pre-fill + mode transition)

**File:** `web-app/src/components/sessions/Omnibar.tsx`

```typescript
const handleRepoSelect = useCallback(
  (path: string) => {
    setInput(path + "/");
    setMode("creation");
    setResultHighlightIndex(-1);
    setDropdownDismissed(false);
    inputRef.current?.focus();
  },
  []
);
```

This pre-fills the input with the repo path, which will trigger path detection on the next debounce cycle (150ms), transitioning the omnibar into creation mode with the path completion dropdown active.

---

### Task 5.6 — Render OmnibarResultList conditionally

**File:** `web-app/src/components/sessions/Omnibar.tsx`

In the JSX, replace the path completion dropdown block and add discovery mode rendering:

```tsx
{/* Discovery mode: session results + recent repos */}
{isDiscoveryMode && (
  <OmnibarResultList
    sessionResults={displayedSessionResults}
    repoEntries={displayedRepoEntries}
    highlightedIndex={resultHighlightIndex}
    onSessionSelect={handleSessionSelect}
    onRepoSelect={handleRepoSelect}
    onCreateNew={handleCreateNew}
  />
)}

{/* Creation mode: existing path completion dropdown (unchanged) */}
{!isDiscoveryMode && isDropdownVisible && (
  <PathCompletionDropdown
    id="path-completion-listbox"
    entries={mergedEntries}
    historyCount={historyCount}
    selectedIndex={dropdownIndex}
    onSelect={handleCompletionSelect}
    isLoading={isCompletionLoading}
  />
)}
```

**Conditional form body:** Show `<div className={styles.body}>` only in `creation` mode:
```tsx
{mode === "creation" && (
  <div className={styles.body}>
    {/* ... existing form fields unchanged ... */}
  </div>
)}
```

**Update footer hints:**
```tsx
<div className={styles.shortcuts}>
  <span className={styles.shortcut}>
    <span className={styles.shortcutKey}>Esc</span> Close
  </span>
  {mode === "creation" && (
    <span className={styles.shortcut}>
      <span className={styles.shortcutKey}>⌘↵</span> Create
    </span>
  )}
  {isDiscoveryMode && (
    <>
      <span className={styles.shortcut}>
        <span className={styles.shortcutKey}>↑↓</span> Navigate
      </span>
      <span className={styles.shortcut}>
        <span className={styles.shortcutKey}>↵</span> Jump
      </span>
    </>
  )}
</div>
```

**Update placeholder text:**
```tsx
placeholder={
  mode === "creation"
    ? "Enter path, GitHub URL, or owner/repo..."
    : "Jump to session or search repos..."
}
```

**Update canSubmit:** Add `InputType.SessionSearch` to the block list:
```typescript
if (!detection || detection.type === InputType.Unknown || detection.type === InputType.SessionSearch) return false;
```

**INVEST:** All tasks in Story 5 modify a single file. Each task is independently reviewable. The story is complete when all six tasks pass their respective tests.

---

## Quality Gates

### Go cache unit tests

Location: `server/services/dir_cache_test.go`

| Test | Scenario |
|---|---|
| `TestDirCache_Hit` | Put entries, Get returns same slice |
| `TestDirCache_MissOnMtimeChange` | Create temp dir, Put, modify dir contents, Get returns miss |
| `TestDirCache_MissOnTTLExpiry` | Put with 1ms TTL, sleep 5ms, Get returns miss |
| `TestDirCache_NoEvictionBelowMax` | Fill to maxSize-1, verify no eviction |
| `TestDirCache_EvictsOldestAtMax` | Fill to maxSize, add one more, oldest entry is gone |
| `TestDirCache_ConcurrentReads` | 50 goroutines Get after one Put, run with `-race` |
| `TestListPathCompletions_CacheHit` | Call twice, second call does not invoke ReadDir |
| `TestListPathCompletions_CacheMissAfterMtimeChange` | Verify fresh entries after directory mutation |

### useSessionSearch unit tests

Location: `web-app/src/lib/hooks/useSessionSearch.test.ts`

| Test | Scenario |
|---|---|
| Empty query | Returns `[]` |
| Title match beats path match | "auth" in title outranks "auth" only in path |
| STOPPED sessions excluded | Sessions with `status === STOPPED` never in results |
| Result cap | At most 8 results even with 100 matching sessions |
| Fuzzy non-prefix match | "squad" matches "stapler-squad" |
| Consecutive char match | "myfeat" matches "my-feature-branch" |
| Multi-field | Session matching title AND branch scores higher than session matching only one field |

### OmnibarResultList keyboard navigation tests

Location: `web-app/src/components/sessions/OmnibarResultList.test.tsx`

| Test | Scenario |
|---|---|
| Empty sections hidden | `sessionResults=[]` → no SESSIONS header |
| Create-new always visible | Renders even with both sections empty |
| Arrow down skips headers | Down from last session item → first repo item (not header) |
| Enter dispatches session action | `onSessionSelect` called with correct session |
| Enter dispatches repo action | `onRepoSelect` called with correct path |
| ARIA activedescendant | Highlighted item id matches input's aria-activedescendant |

### Integration acceptance tests

| Scenario | Expected |
|---|---|
| Omnibar opens empty | Both sections visible with recents |
| User types "auth" | Session results update, repo results update, form body hidden |
| User types "~/" | Session results hidden, path completion dropdown appears |
| Enter on session result | Omnibar closes, `onNavigateToSession` called |
| Enter on repo result | Input pre-filled with path, creation form appears |
| Escape on highlighted result | Highlight cleared, results still visible |
| Second Escape | Omnibar closes |

---

## Known Issues

### Bug 1 — GitHubShorthand fires on branch-style session searches [SEVERITY: Medium]

**Description:** The `GitHubShorthandDetector` (priority 40) matches `^([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_.-]+)`. A user searching for a session with title `feature/auth-overhaul` will trigger GitHub detection instead of session search, showing a GitHub icon and blocking session results.

**Mitigation in this plan:** `SessionSearchDetector` has priority 200, so it only fires after all GitHub detectors. The user's query `feature/auth-overhaul` will be classified as `InputType.GitHubShorthand`, not `InputType.SessionSearch`. The session result for that session title will not appear.

**Workaround:** Search for `auth-overhaul` (the branch suffix) instead. The Fuse.js search will still find it via the `branch` field.

**Future fix:** Add an active-session-title check to `GitHubShorthandDetector`: if the input exactly matches an existing session title, prefer `SessionSearch` over `GitHubShorthand`. This requires passing the session list into the detector, which is a bigger architectural change. Deferred to a follow-up.

**Files likely affected:** `web-app/src/lib/omnibar/detector.ts`

---

### Bug 2 — Recent repos list shows deleted paths [SEVERITY: Medium]

**Description:** `usePathHistory` stores paths in `localStorage` without existence checks. A repo that was moved or deleted appears in the Recent Repos empty-state list. Selecting it pre-fills a non-existent path, which fails at session creation.

**Mitigation in this plan:** The path existence check (`✓`/`✗`) in the input indicator will show `✗` after the user selects a deleted path and the detection runs. The user sees immediate feedback before attempting to create the session.

**Future fix:** On each Omnibar open, call the `pathExists` check for each recent repo entry and filter out non-existent paths. This adds one RPC per recent repo but could be batched. Deferred because the `✓`/`✗` indicator provides sufficient feedback for now.

**Files likely affected:** `web-app/src/lib/hooks/usePathHistory.ts`, `web-app/src/components/sessions/OmnibarRepoResult.tsx`

---

### Bug 3 — ConnectRPC transport created per-render in usePathCompletions [SEVERITY: Low]

**Description:** `usePathCompletions.ts` creates a new `createConnectTransport` and `createClient` inside the debounce callback (inside a `useEffect`). This means a new HTTP transport object is created on every debounce firing, which has small but real memory overhead.

**Mitigation in this plan:** Not fixed in this plan. The issue predates this feature. Adding session search (a second async hook) makes the impact slightly larger but does not introduce the root cause.

**Future fix:** Lift transport/client creation to module level, matching the pattern used by `completionCache` in the same file.

**Files likely affected:** `web-app/src/lib/hooks/usePathCompletions.ts`

---

### Bug 4 — Fuse.js threshold may over-match on short queries [SEVERITY: Low]

**Description:** With `threshold: 0.4`, a 1-2 character query like "a" will match most sessions. The result list will be populated with low-relevance results ranked by score, but the user will see up to 8 results for a single character, all with mediocre scores.

**Mitigation:** `minMatchCharLength: 1` is set intentionally for the "3 keystroke" requirement (Cmd+K, type one char, arrow, Enter). The user expects results on a single character. If relevance drift becomes an issue, raise `minMatchCharLength` to 2 or increase `threshold` to 0.3 (stricter) in follow-up.

**Files likely affected:** `web-app/src/lib/hooks/useSessionSearch.ts`

---

### Bug 5 — DirCache mtime invalidation unreliable on network filesystems [SEVERITY: Low]

**Description:** On NFS or SMB mounts, mtime may not update synchronously when directory contents change. The 60s hard TTL is the backstop, but users on network filesystems could see stale path completions for up to 60 seconds after a directory change.

**Mitigation:** The 60s TTL is the fallback. For a solo-practitioner tool primarily used on local filesystems, this is acceptable. Document as a known limitation.

**Files likely affected:** `server/services/dir_cache.go`

---

### Bug 6 — useRepositorySuggestions makes a listSessions RPC on every Omnibar open [SEVERITY: Low]

**Description:** `useRepositorySuggestions` calls `client.listSessions({})` inside a `useEffect` that runs when `baseUrl` changes. Since the hook is instantiated inside `Omnibar.tsx` (which mounts/unmounts on each open/close), it fires an RPC on every Omnibar open.

**Mitigation in this plan:** The session data from `useRepositorySuggestions` is redundant with the Redux store (`selectAllSessions`). The empty-state recent session list (Task 5.3) uses Redux directly (zero RPCs). The `useRepositorySuggestions` hook provides frecency-ranked paths from session `path` fields, which is complementary to `usePathHistory` (user-typed paths).

**Future fix:** Replace `useRepositorySuggestions` with a `useMemo` over `selectAllSessions` for the repo paths section. The `rankPathsByFrecency` util can run client-side against the Redux store. This eliminates the redundant RPC.

**Files likely affected:** `web-app/src/lib/hooks/useRepositorySuggestions.ts`, `web-app/src/components/sessions/Omnibar.tsx`

---

## Implementation Sequence

1. **Task 1.1** — `dir_cache.go` (backend, independent)
2. **Task 1.2** — Wire cache into `path_completion_service.go`
3. **Task 2.1** — `InputType.SessionSearch` in `types.ts` + `SessionSearchDetector` in `detector.ts`
4. **Task 2.2** — `getAll()` in `usePathHistory.ts`
5. **Task 2.3** — `dropdownDismissed` reset in `Omnibar.tsx`
6. **Task 3.1** — Install fuse.js
7. **Task 3.2** — `useSessionSearch.ts` hook
8. **Task 4.1** — `OmnibarSessionResult.tsx`
9. **Task 4.2** — `OmnibarRepoResult.tsx`
10. **Task 4.3** — `OmnibarResultList.tsx`
11. **Task 5.1** — Mode state + `onNavigateToSession` prop in `Omnibar.tsx`
12. **Task 5.2** — Hook wiring in `Omnibar.tsx`
13. **Task 5.3** — Empty-state recent sessions from Redux
14. **Task 5.4** — Session result selection handler + Enter key routing
15. **Task 5.5** — Repo result selection + pre-fill
16. **Task 5.6** — Conditional render of result list vs form body

Tasks 1.1-1.2 and 2.1-2.3 can be started simultaneously in separate commits. Tasks 3-5 must follow in sequence.

---

## File Reference

| File | Action | Story |
|---|---|---|
| `server/services/dir_cache.go` | CREATE | 1 |
| `server/services/dir_cache_test.go` | CREATE | 1 |
| `server/services/path_completion_service.go` | MODIFY | 1 |
| `web-app/src/lib/omnibar/types.ts` | MODIFY | 2 |
| `web-app/src/lib/omnibar/detector.ts` | MODIFY | 2 |
| `web-app/src/lib/hooks/usePathHistory.ts` | MODIFY | 2 |
| `web-app/src/components/sessions/Omnibar.tsx` | MODIFY | 2, 5 |
| `web-app/package.json` | MODIFY | 3 |
| `web-app/src/lib/hooks/useSessionSearch.ts` | CREATE | 3 |
| `web-app/src/lib/hooks/useSessionSearch.test.ts` | CREATE | 3 |
| `web-app/src/components/sessions/OmnibarSessionResult.tsx` | CREATE | 4 |
| `web-app/src/components/sessions/OmnibarSessionResult.css.ts` | CREATE | 4 |
| `web-app/src/components/sessions/OmnibarRepoResult.tsx` | CREATE | 4 |
| `web-app/src/components/sessions/OmnibarRepoResult.css.ts` | CREATE | 4 |
| `web-app/src/components/sessions/OmnibarResultList.tsx` | CREATE | 4 |
| `web-app/src/components/sessions/OmnibarResultList.css.ts` | CREATE | 4 |
| `web-app/src/components/sessions/OmnibarResultList.test.tsx` | CREATE | 4 |

---

## ADR References

- `project_plans/omni-bar-session-search/decisions/ADR-001-client-side-session-search.md` — why no SearchSessions RPC
- `project_plans/omni-bar-session-search/decisions/ADR-002-fusejs-typescript-fuzzy.md` — why Fuse.js over fzf-for-js or match-sorter
- `project_plans/omni-bar-session-search/decisions/ADR-003-go-sync-map-dir-cache.md` — why DirCache over sync.Map
