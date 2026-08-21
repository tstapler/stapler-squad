# Research: Pitfalls

Dimension: Pitfalls for Omnibar Session Search + Fuzzy Matching + Directory Tree Cache
Date: 2026-04-14
Status: Complete

---

## P1: Critical (will break the feature if ignored)

### Detector mode conflict: session-search input vs path input

The existing `detector.ts` uses input shape to classify what the user is typing. A bare word like `squad` resolves to `InputType.Unknown` (no leading `/`, `~/`, `./`, no `://`). That means session search queries — which are natural-language strings like `my-feature` or `squad` — will produce `InputType.Unknown`, which today blocks submission entirely (`canSubmit` returns false when `detection.type === InputType.Unknown`, line 306 of `Omnibar.tsx`).

Risk: Adding session search without touching the detector means no session result is ever reachable by typing a bare word. Every search query except those starting with a path sigil falls through to Unknown.

Mitigation: Introduce a new `InputType.SessionSearch` that is returned when the input does not match any existing pattern. The detector's `LocalPathDetector` (priority 100) is the current catch-all fallback returning `null` for non-path strings; the default registry's final fallback returns `InputType.Unknown`. Session search should sit *after* `LocalPath` detection so path typing still activates path completion, but anything else activates session search mode.

---

### Enter key ambiguity: "navigate to session" vs "create session"

Currently `handleKeyDown` treats Enter in the dropdown as a path completion acceptance, and `Cmd+Enter` as form submit. When session results appear in the same omnibar, a plain Enter on a session result should *navigate* (not create). This conflicts with the current submission model where submitting requires `canSubmit` to be true and calls `onCreateSession`.

Risk: Without explicit routing of Enter by result type, the user presses Enter on a session result and either (a) nothing happens (session-search result doesn't satisfy `canSubmit`), or (b) a new session is accidentally created with the session name as path.

Mitigation: The omnibar needs two distinct Enter behaviors: if the focused item is a session result, Enter calls a navigation callback (e.g., `onNavigateToSession`). If the focused item is a path completion or the user is in creation mode, Enter selects the path completion or triggers `handleSubmit`. This must be explicit in the keyboard handler, not inferred.

---

### `PathCompletionService` is stateless and uncached — every keystroke hits the filesystem

`PathCompletionService` is explicitly documented "stateless; each call performs a fresh directory listing" (line 26, `path_completion_service.go`). The client-side `usePathCompletions` hook has a 30-second TTL LRU with 100 entries (`CACHE_MAX = 100`, `CACHE_TTL_MS = 30_000`), but this only helps for burst typing at the same prefix. Opening the omnibar a second time with the same prefix will hit the cache; however the directory tree cache requirement in the requirements doc is about server-side caching so that even the *first* call within a server session is fast.

Risk: Without a server-side cache, the cold-open latency target of <100ms (requirement #3) cannot be met, especially for home directory traversal (hundreds of entries). Currently each `ListPathCompletions` call does a full `os.ReadDir` on every invocation. A user typing `~/` five times across five omnibar opens will trigger five separate `os.ReadDir("~")` calls.

Mitigation: Add a server-side cache keyed by `(baseDir, maxResults, directoriesOnly)` with the directory's mtime as a cache-validity signal. The mtime approach has its own pitfall (see P3-C4).

---

### `usePathCompletions` creates a new transport and client on every render cycle

Lines 147–153 of `usePathCompletions.ts` create `createConnectTransport({ baseUrl })` and `createClient(SessionService, transport)` inside the debounce callback. This is inside a `useEffect` that re-runs on every change to `pathPrefix`, `baseUrl`, `enabled`, `directoriesOnly`, or `maxResults`. While the debounce/abort/generation pattern correctly discards stale results, a new HTTP transport object is created per keypress. At high typing speed this has memory and connection overhead. When session search adds a *second* async hook (for session results), the two hooks firing simultaneously could create connection contention.

Risk: Not an outright breakage, but it degrades performance and adds complexity when a second async hook is added alongside it.

Mitigation: Lift the transport/client creation to module level (similar to how the cache is module-level). The pattern is already used for the cache (`const completionCache = new Map(...)`) — extend it to the client instance.

---

## P2: High (significant UX or reliability impact)

### Ranking instability: BM25 tokenizer strips hyphens → session titles become unrecognizable

The `Tokenizer.Tokenize()` method splits on `!unicode.IsLetter(r) && !unicode.IsNumber(r)` (line 61–63, `tokenizer.go`). A session title like `my-feature-branch` becomes tokens `["my", "featur", "branch"]` (after Porter stemming). A query of `my-feature` would tokenize to `["my", "featur"]`, which matches, but BM25 scoring treats these as three separate tokens competing against all other documents. More importantly, hyphenated names like `stapler-squad` stem to `["stapl", "squad"]`, meaning "squad" matches dozens of unrelated sessions if they contain that word.

Risk: BM25 was built for long-form document search. Session titles are short (2–5 tokens). In a small corpus of 20–50 sessions, IDF will be near-zero for common tokens (every session might be in the `stapler-squad` repo), making scores nearly identical and result order unpredictable across queries.

Mitigation: Session search should not use the existing BM25 `SearchEngine` at all. It should use a dedicated fuzzy matcher operating on session metadata fields (title, branch, path, tags) with field-specific weights. The BM25 engine is for conversation history search, not session list navigation.

---

### Escape key: first press should dismiss session-search dropdown, not close omnibar

Current behavior (line 253–259 of `Omnibar.tsx`): when the path-completion dropdown is visible and Escape is pressed, `e.nativeEvent.stopImmediatePropagation()` is called and only the dropdown is dismissed. A second Escape closes the omnibar. This is correct behavior.

When session search results appear, the same two-press Escape contract must hold: first Escape dismisses the session result list, second Escape closes the omnibar. If session results and path completions share the same `dropdownDismissed` / `dropdownIndex` state, toggling between modes (typing a path, then backspacing to session-search) could leave `dropdownDismissed = true` and suppress session results.

Risk: Sessions results never appear after the user types a path, then deletes it, because `dropdownDismissed` is sticky and not reset when the input type changes.

Mitigation: Either add a separate `sessionResultsDismissed` state, or reset `dropdownDismissed` on every mode transition detected by the detector.

---

### Symlink loops in directory traversal for tree cache

`path_completion_service.go` follows symlinks for directory detection (lines 134–141) using `os.Stat` (follows symlinks) while walking with `os.ReadDir` (does not recurse). The current code only reads a single directory level, so symlink loops are not a problem now. However, if the server-side cache implementation adds recursive traversal (to pre-warm the cache with subdirectory contents), a symlink loop like `~/projects/link → ~/` would cause infinite recursion.

Risk: Not a current bug, but will become a critical bug if the cache is implemented with recursive pre-warming.

Mitigation: Any recursive cache-warming code must track visited real paths using `filepath.EvalSymlinks` and skip already-visited directories.

---

### Recent repos: localStorage path history not validated on load

`usePathHistory.ts` stores paths in `localStorage` keyed by `"omnibar:path-history"` (line 5). On load, `loadFromStorage()` returns the raw parsed array with no validation that paths still exist on disk. The `getMatching` function (line 68–77) returns entries sorted by score with no existence check.

Risk: The recents list accumulates deleted, renamed, or unmounted paths silently. When shown in the "Recent Repos" quick-pick for new-session creation, deleted paths lead to session creation failure (caught only at the server level with an error message).

Mitigation: On each omnibar open, validate recent repo entries against the path-existence endpoint (`pathExists` from `usePathCompletions`) before displaying them. Stale entries should be surfaced with a visual indicator or silently pruned. Cap the list to entries used within the last 30 days.

---

### Mixed result-type ranking: sessions and repos competing for the same slots

The requirements spec lists "unified result list mixing sessions and repo paths" as a Nice-to-Have but session search and recent repos as Must-Have. Even without explicit mixing, the UI must decide which results take priority when the query matches both a session title ("auth-service") and a recent repo path (`~/work/auth-service`).

Risk: If sessions and repos are mixed naively with no type-aware boosting, repo paths (which have longer strings with more matching characters) will typically outscore short session titles in fuzzy scoring algorithms, hiding the sessions the user actually wants to navigate to.

Mitigation: Use separate ranked sections (sessions first, then repos) rather than a single merged rank. Apply type-specific boosts: exact title match > prefix match > fuzzy match, with sessions ranked above repos when both match equally.

---

## P3: Medium (noticeable but workaround exists)

### mtime cache invalidation is unreliable for subdirectory changes

The requirements specify caching keyed by "root path + mtime." On macOS (HFS+/APFS), the mtime of a directory only updates when files are *directly* added or removed in that directory — not in subdirectories. So `~/projects` mtime does not change when `~/projects/myrepo/src/new-file.ts` is created.

Risk: Cache will serve stale results if a user clones a repo into an existing directory. The parent directory listing won't change (no new entry there), but the subdirectory listing at the leaf has changed. For shallow single-level listings (current behavior), this is mostly benign because a new subdirectory in `~/projects` *does* update `~/projects` mtime. The risk is higher for nested caching if ever implemented.

Mitigation: For single-level `os.ReadDir` caching, mtime-based invalidation is acceptable. Document the limitation. Set a short TTL (30–60 seconds) as a backstop. Do not rely solely on mtime if implementing multi-level caching.

---

### `usePathCompletions` generation counter only increments, never wraps — safe for React lifecycle

The `generationRef.current` counter in `usePathCompletions.ts` (line 109) increments on every effect run and is compared at line 155. JavaScript Numbers are IEEE 754 doubles (safe integers up to 2^53). In practice the user would need to trigger ~9 quadrillion debounce firings before overflow. Not a real risk, documented here for completeness.

---

### Concurrent session search requests arriving at `ListSessions` during session creation

`ListSessions` in `session_service.go` (line 364) reads from `reviewQueuePoller.GetInstances()`, which is the live in-memory instance list. If a session-search RPC fires while another request is adding a new session, the response may include the session before it is fully initialized (tmux session not yet started, worktree not yet created). A session search that navigates to such a session would navigate to a session with status "ready" that has no live terminal.

Risk: Navigating to a partially-created session. Not a crash, but confusing UX.

Mitigation: Session search results should filter for fully-started sessions (`instance.Started() == true`). The protobuf session `status` field should be used client-side to visually indicate sessions that are still initializing.

---

### Go filesystem walk performance: home directory traversal without recursion limit

`os.ReadDir` on `~` returns 50–200 entries on a typical developer machine. The current code caps at `pathCompletionHardMax = 500` entries and applies a 2-second timeout. This is adequate for single-directory reads.

Risk: If a user types `~/` and the home directory contains symlinked directories pointing to large trees (e.g., `~/node_modules` via a misconfigured symlink), the `os.Stat` call on each symlinked entry (line 136) multiplies syscall overhead. 200 entries × 1 `os.Stat` each = 200 extra stat calls per request.

Mitigation: The current broken-symlink skip (line 138–141) already handles most cases. Add a per-entry stat timeout using a context passed through, or pre-filter entries to skip obviously non-repo directories (`.DS_Store`, `Library`, `Applications`).

---

### `usePathHistory.getMatching` does prefix matching only — not fuzzy

The history matching in `Omnibar.tsx` (lines 107–118) calls `getMatching(completionPrefix)` which uses `e.path.startsWith(prefix)` (line 72 of `usePathHistory.ts`). This is purely a prefix filter, not fuzzy matching. If the user types `auth` hoping to find `~/projects/auth-service`, the history will not surface it because `~/projects/auth-service` does not start with `auth`.

Risk: The "recent repos quick-pick" will only surface repos when the user types the beginning of the full path. Typing a repo name fragment will show no history matches even if the user has used that repo dozens of times.

Mitigation: Replace the `startsWith` filter with a fuzzy match (same algorithm being added for session search). Score history entries by: fuzzy match quality × recency score × frequency count.

---

### Omnibar `dropdownDismissed` resets on any character change but not on mode transition

In `Omnibar.tsx` line 399–401 (`onChange` of the main input), `setDropdownDismissed(false)` is called on every character change. However, the detection mode change (from `LocalPath` to session-search mode) is debounced by 150ms (line 174). During the debounce window, if `dropdownDismissed` has been set true (user pressed Escape to dismiss path completions) and the user then backspaces to a non-path string, the dropdown will remain dismissed until the next character change because the mode transition happens asynchronously.

Risk: Minor: session results are suppressed for up to 150ms after Escape is pressed during mode switching.

Mitigation: Reset `dropdownDismissed` when the detected `InputType` changes.

---

## P4: Low (edge case, nice to handle)

### Path deduplication: same repo accessed via symlink and real path

On macOS, `~/Documents` may be a symlink, and `/Users/tyler/Documents` is the real path. A user who accesses the same repo as both `~/projects/myrepo` and `/Users/tyler/projects/myrepo` will see two separate history entries in the recents list, and two separate "recent repo" suggestions.

Risk: Minor duplicate clutter in recents.

Mitigation: Run `filepath.EvalSymlinks` on each recent-repo path before storing and before comparison. Store only canonical (resolved) paths.

---

### Porter stemmer treats branch names and technical terms incorrectly

The BM25 `Tokenizer.porterStem` (lines 168–195, `tokenizer.go`) was designed for English prose. Branch names and session titles like `fix-login-bug` stem to `["fix", "login", "bug"]`, `feature/add-oauth` stems to `["featur", "add", "oauth"]`. The stem `featur` will not match a query of `feat` because Porter does not stem abbreviations.

Risk: Fuzzy session search built on top of BM25 tokenization would produce counter-intuitive non-matches. For example, `feat` does not match `feature/add-oauth` through the tokenizer path because `feat` → `feat` (no stem applied) and `featur` ≠ `feat`.

Mitigation: Session search should bypass the BM25 tokenizer entirely. Use a character-level fuzzy matcher (e.g., fzf-style scoring) on the raw, lowercased session metadata fields, not on stemmed tokens.

---

### `GitHubShorthandDetector` priority 40 fires before session names that look like `org/repo`

The shorthand detector pattern `^([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_.-]+)` (line 118, `detector.ts`) will match a session title like `frontend/auth-overhaul` if the user types it. This means typing a session name that contains a slash would trigger GitHub shorthand detection instead of session search, adding confusing visual feedback (GitHub icon) and blocking session navigation.

Risk: Rare but confusing. Session titles with slashes are used in the category system (e.g., "Work/Frontend" becomes tags, but branch names like `feature/auth` appear in the input via `path@branch`).

Mitigation: The session-search mode should run before (or be explicitly excluded from) shorthand detection. If the user is clearly searching for an existing session (matching against the live session list), the omnibar should show session results even if the input pattern matches a GitHub shorthand.

---

### `localStorage` quota exceeded in private/restrictive environments

`usePathHistory.persistToStorage` (line 42–47, `usePathHistory.ts`) silently swallows `localStorage.setItem` errors with an empty catch. In Safari private browsing, `localStorage` throws a `QuotaExceededError` on every write. The history will appear to save but will not persist across opens.

Risk: Silent failure means the recent repos feature appears to work during a session but resets on every omnibar open in private-mode browsers or locked-down environments.

Mitigation: Already handled by the empty catch (graceful degradation). No change needed, but document the behavior as expected. Consider server-side recent-repos storage if this becomes a user complaint.

---

### `useWorktreeSuggestions` fires a git subprocess for every omnibar open where `sessionType === "existing_worktree"`

`PathCompletionService.ListWorktrees` (line 170–192, `path_completion_service.go`) runs `exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")` for every call. This is uncached. If the user opens the omnibar, switches to "existing_worktree" mode, and navigates away, a subprocess is spawned each time. Session search does not change this, but the omnibar refactor must not regress this behavior.

Risk: No regression risk unless omnibar refactoring causes `useWorktreeSuggestions` to fire more frequently.

Mitigation: Cache `git worktree list` output per repo path with a 10-second TTL, consistent with the proposed directory tree cache.

---

## Implementation Checklist

- [ ] Add `InputType.SessionSearch` to `detector.ts` types and registry — all non-path, non-URL inputs must route to session search mode, not `Unknown`
- [ ] Update `canSubmit` in `Omnibar.tsx` to exclude session-search mode from the submission gate (session results navigate, not create)
- [ ] Implement separate keyboard routing for Enter: session result item → navigate; path completion item → accept; neither → submit form
- [ ] Add server-side cache to `PathCompletionService` (keyed by baseDir + mtime + options) — required to hit <100ms repeated-open target
- [ ] Session search must NOT use BM25 tokenizer — use character-level fuzzy matching on raw session metadata fields
- [ ] Reset `dropdownDismissed` on `InputType` mode transition, not only on character change
- [ ] Replace `startsWith` prefix matching in `usePathHistory.getMatching` with the same fuzzy algorithm used for session search
- [ ] Validate recent-repo paths before display (skip paths that no longer exist on disk)
- [ ] Add symlink loop protection to any recursive directory traversal in cache-warming code
- [ ] Filter session-search results to fully-started sessions (`instance.Started() == true`) to avoid navigation to partially-initialized sessions
- [ ] Lift ConnectRPC transport/client creation out of the `useEffect` in `usePathCompletions.ts` to module-level singleton to reduce connection overhead
