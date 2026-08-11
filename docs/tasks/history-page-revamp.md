# History Page Revamp — Implementation Plan

**Status**: Ready for Implementation
**Created**: 2026-04-12
**Branch**: `stapler-squad-history-resumption`
**Project plans**: `project_plans/history-page-revamp/`

---

## Epic Overview

### User Value

A solo developer (Tyler) maintains dozens of Claude Code sessions across multiple repos and branches. Today, the history page forces them to either remember session names (they're often auto-generated), click into each session to identify it, or accept that "resuming" a session is broken and silently fails. Each failed lookup costs 10–30 seconds of context-switching.

After this revamp: open the history page, visually identify the right session from rich metadata on the card, expand it to read the last few messages in context, and launch it in the right worktree — all in under 10 seconds.

### Success Metrics

| Metric | Current | Target |
|---|---|---|
| Time to identify + launch a session | ~30s (multiple clicks) | <10s (single interaction) |
| Initial list render time (200 sessions) | ~400ms+ | ≤200ms |
| Resume action: broken → fixed | Broken | Works reliably |
| Metadata visible per card without clicking | title only | repo, branch, timestamps, status |
| Fork workflow discoverability | Hidden / absent | First-class split-button on card |

### Scope

**Included:**
- Rich session cards (repo, branch, timestamps, status, diff count)
- Inline message preview (expand to show last 3–5 messages)
- Virtual scrolling with cursor-based pagination
- Fork/resume workflow with worktree picker and session rename
- Fix broken resume (diagnose root cause, fix)
- Unified search + filter (repo, branch, date range, full-text)

**Excluded:**
- Cross-device sync, sharing, authentication
- New session creation flow
- Running-session terminal streaming behavior

### Constraints

- Go backend + React frontend, ConnectRPC — no new frameworks
- Initial history list render ≤ 200ms performance budget
- Incremental delivery — each story independently releasable
- Existing session views must remain regression-free

---

## Architecture Decisions

| ADR | File | Decision |
|---|---|---|
| ADR-001 | `project_plans/history-page-revamp/decisions/ADR-001-tanstack-virtual-virtualization.md` | TanStack Virtual v3 (`useVirtualizer`) for list virtualization — headless, RSC-safe, handles dynamic heights |
| ADR-002 | `project_plans/history-page-revamp/decisions/ADR-002-cursor-pagination-for-history.md` | Cursor-based pagination (opaque `page_token`) replaces `limit: 500` eager load; default 50 items/page |
| ADR-003 | `project_plans/history-page-revamp/decisions/ADR-003-extend-createsession-for-fork.md` | Extend `CreateSession` with `fork_source_id` + `fork_at_message` rather than adding a new RPC |
| ADR-004 | `project_plans/history-page-revamp/decisions/ADR-004-lazy-git-enrichment-for-history-entries.md` | Lazy git enrichment: `session_status` always populated, `branch` via 60s cache, `git_status_summary`/`diff_file_count` only when a live worktree exists |

---

## Story Breakdown

### Story 1: Proto + Backend Foundation [1 week]

**User value**: All the metadata fields needed for rich cards exist in the API; pagination works; `ListClaudeHistory` returns a first page in <100ms.

**Acceptance criteria:**
- `ClaudeHistoryEntry` proto has `branch`, `git_status_summary`, `session_status`, `last_commit_message`, `diff_file_count` (fields 8–12)
- `ListClaudeHistoryRequest` has `page_size` (field 3, renamed from `limit`) and `page_token` (field 4); existing callers continue to work unchanged
- `ListClaudeHistoryResponse` has `next_page_token` (field 3)
- Server returns first page (50 items) in ≤100ms on a 500-entry dataset
- `session_status` is correctly populated via in-memory session store lookup
- `branch` is populated via a 60-second cached `git rev-parse` call per unique project path
- `CreateSessionRequest` has `fork_source_id` (field 12) and `fork_at_message` (field 13)
- `CreateSession` handler dispatches to `ForkClaudeConversation` when `fork_source_id` is set

---

#### Task 1.1: Enrich ClaudeHistoryEntry proto and regenerate [2h]

**Objective**: Add 5 new fields to `ClaudeHistoryEntry` and pagination fields to `ListClaudeHistory*` messages; update `CreateSessionRequest` with fork fields; regenerate proto bindings.

**Context boundary:**
- Primary: `proto/session/v1/session.proto`
- Supporting: `Makefile` (for `make generate-proto`)
- ~200 lines changed

**Prerequisites:**
- Proto field number conventions in this project (fields 8+ are free in `ClaudeHistoryEntry`)
- `SessionStatus` enum already defined in `proto/session/v1/types.proto`

**Implementation approach:**
1. Add fields 8–12 to `ClaudeHistoryEntry` as specified in ADR-004
2. Rename `limit` → `page_size` in `ListClaudeHistoryRequest` (same field number 3, backward compat); add `page_token` as field 4
3. Add `next_page_token` to `ListClaudeHistoryResponse` as field 3
4. Add `fork_source_id` (field 12) and `fork_at_message` (field 13) to `CreateSessionRequest`
5. Run `make generate-proto` to regenerate Go and TypeScript bindings
6. Verify `make build` passes (compilation check)

**Validation strategy:**
- Unit: `go build ./...` compiles without errors
- Unit: `make generate-proto` completes without errors
- Integration: existing test suite passes (`make test`)

**INVEST check:**
- Independent: proto change is self-contained; no business logic changes
- Negotiable: field names/numbers open for review
- Valuable: unblocks all subsequent tasks
- Estimable: 2h with high confidence
- Small: proto only, no logic
- Testable: compilation + proto generation are pass/fail checks

---

#### Task 1.2: Implement cursor pagination in ListClaudeHistory handler [3h]

**Objective**: Replace the `limit`-based slice with cursor-based pagination in `server/services/search_service.go`.

**Context boundary:**
- Primary: `server/services/search_service.go`
- Supporting: `session/history.go` (ClaudeHistoryEntry struct, Reload/GetAll)
- ~150 lines changed + ~50 lines new

**Prerequisites:**
- Completion of Task 1.1 (proto fields available in generated Go code)
- Understanding of the current `Reload()` sort order (UpdatedAt desc) in `session/history.go`

**Implementation approach:**
1. Add `pageCursor` struct and `encodeCursor`/`decodeCursor` helpers to `search_service.go`:
   ```go
   type pageCursor struct {
       UpdatedAt int64  `json:"u"`
       ID        string `json:"i"`
   }
   ```
2. In `ListClaudeHistory` handler:
   - Default `pageSize` to 50 if 0; clamp max to 200
   - If `pageToken` is non-empty, decode cursor to `(updatedAt, id)` and binary-search the sorted slice to find the resume position
   - Return at most `pageSize` entries starting from resume position
   - If more entries remain, encode the last-returned entry as `next_page_token`
3. Keep the `limit` fallback: if `pageSize == 0` and old `limit > 0`, use old behavior (no cursor)
4. Set `total_count` in response

**Validation strategy:**
- Unit tests in `server/services/search_service_test.go`:
  - First page returns 50 items and a non-empty `next_page_token`
  - Second page starts where first left off (no duplicates, no gaps)
  - Empty `page_token` returns first page
  - Invalid cursor returns first page (graceful degradation)
  - Last page returns empty `next_page_token`
- Benchmark: `go test -bench=BenchmarkListClaudeHistory -benchmem ./server/services` — first page <100ms on 500 entries

**INVEST check:**
- Independent: only touches search_service.go; no frontend changes
- Negotiable: cursor encoding format flexible
- Valuable: enables the 200ms render budget
- Estimable: 3h with high confidence
- Small: single handler function
- Testable: unit tests verify pagination invariants

---

#### Task 1.3: Populate git enrichment fields in history entries [3h]

**Objective**: Implement the lazy git enrichment strategy from ADR-004 — `session_status` always, `branch` via cache, git diff fields only for live worktrees.

**Context boundary:**
- Primary: `server/services/search_service.go`
- Supporting: `session/storage.go` (in-memory session store), `session/history.go`
- ~100 lines new code

**Prerequisites:**
- Completion of Task 1.1 (new fields on ClaudeHistoryEntry)
- Understanding of `session.Storage.GetAll()` API and `session.Instance.Status` enum

**Implementation approach:**
1. Add `branchCache` struct (map + mutex + TTL) to `search_service.go` — initialized once in the service constructor
2. For each entry in the `ListClaudeHistory` response:
   a. Look up `entry.id` in `session.Storage` by `ResumeId` field → set `session_status`
   b. Look up `entry.project` in `branchCache` → if miss, run `git rev-parse --abbrev-ref HEAD` in project dir, cache result for 60s
   c. If a live session with matching `ResumeId` exists: populate `git_status_summary`, `last_commit_message`, `diff_file_count` via `go-git`
3. Populate only the enrichment fields that are in the response page (not the full 500-entry corpus)

**Validation strategy:**
- Unit tests:
  - `session_status` is `RUNNING` for entries with a matching live session
  - `session_status` is `UNKNOWN` for purely historical entries
  - `branch` is populated after cache warm-up
  - `branch` returns cached value on second call within TTL
  - `git_status_summary` is empty for entries with no live worktree
- Manual: start a session, load history page, verify status dot shows "Running"

**INVEST check:**
- Independent: enrichment is additive; existing fields unchanged
- Negotiable: cache TTL, fields populated are flexible
- Valuable: enables rich card status display
- Estimable: 3h with high confidence
- Small: ~100 lines of new enrichment logic
- Testable: status and branch fields verifiable in unit tests

---

#### Task 1.4: Implement fork dispatch in CreateSession handler [2h]

**Objective**: Add `fork_source_id` / `fork_at_message` dispatch to the `CreateSession` handler, using the existing `ForkClaudeConversation` function.

**Context boundary:**
- Primary: `server/services/session_service.go`
- Supporting: `session/history_fork.go` (ForkClaudeConversation), `session/history.go`
- ~40 lines new code

**Prerequisites:**
- Completion of Task 1.1 (fork fields on proto)
- Familiarity with `ForkClaudeConversation(srcPath, forkAtMessage, dstDir)` signature in `session/history_fork.go`

**Implementation approach:**
1. At the start of the `CreateSession` handler, check `req.Msg.ForkSourceId != ""`
2. If set: look up the source JSONL path via `ClaudeSessionHistory.GetByID(forkSourceId)`
3. Call `session.ForkClaudeConversation(srcPath, req.Msg.ForkAtMessage, dstDir)` → returns `newUUID`
4. Set `req.Msg.ResumeId = newUUID` and continue the normal session-start flow
5. Handle errors: source not found → return `codes.NotFound`; fork fails → return `codes.Internal` with context

**Validation strategy:**
- Unit tests in `server/services/session_service_test.go`:
  - Fork with valid `fork_source_id` sets `ResumeId` and starts session
  - Fork with invalid `fork_source_id` returns `NotFound` error
  - Fork with `fork_at_message = 0` copies all messages
  - Fork with `fork_at_message = N` truncates at N
- Integration: `make build && make test` passes

**INVEST check:**
- Independent: only adds dispatch logic; existing CreateSession path unchanged for callers without fork_source_id
- Negotiable: error codes, dstDir convention flexible
- Valuable: enables the fork modal flow
- Estimable: 2h with high confidence
- Small: ~40 lines
- Testable: unit tests cover all fork code paths

---

### Story 2: Diagnose and Fix Broken Resume [3 days]

**User value**: "Resume" on the history page reliably relaunches the correct conversation in Claude — the most important unblock for the whole revamp.

**Acceptance criteria:**
- Root cause of the broken resume is documented with evidence
- Resuming a stopped session from the history page launches `claude --resume <id>` in the correct working directory
- Resuming a session whose project directory no longer exists shows a clear error, not a silent failure
- Session is named via the rename modal before launch (not auto-named "Resumed: X")

---

#### Task 2.1: Diagnose broken resume root cause [2h]

**Objective**: Trace the `handleResumeSession` → `createSession` RPC → tmux session start flow and identify the exact failure point.

**Context boundary:**
- Primary: `web-app/src/app/history/page.tsx` (handleResumeSession)
- Supporting: `server/services/session_service.go` (CreateSession handler), `session/instance.go` (session start)
- Read-only investigation; no code changes

**Prerequisites:**
- Access to browser dev tools and server logs at `~/.stapler-squad/logs/stapler-squad.log`
- A history entry that can be used as a test case

**Implementation approach:**
1. Open browser DevTools network tab; trigger a resume; capture the full RPC request/response
2. Check server logs for the corresponding `CreateSession` call: does it reach the handler? Does it fail?
3. In `session/instance.go`, trace `Start()` → `startClaude()` → tmux command execution
4. Check: does `claude --resume <id>` with a non-existent `--resume` UUID silently succeed or fail?
5. Check: does the `path` field in `CreateSessionRequest` point to a valid directory?
6. Document root cause in a comment block at the top of the fix task (Task 2.2)

**Validation strategy:**
- Root cause documented: a specific file + line number + behavior description
- Hypothesis: the resume silently succeeds at the RPC layer but the `claude --resume <id>` flag either points to a wrong UUID or the session start completes without Claude loading the conversation context

**INVEST check:**
- Independent: investigation only; no other tasks blocked on outcome
- Negotiable: investigation approach flexible
- Valuable: without this, Task 2.2 is guesswork
- Estimable: 2h with high confidence (bounded by log inspection)
- Small: no code changes
- Testable: root cause statement is falsifiable

---

#### Task 2.2: Fix resume and add session rename modal [3h]

**Objective**: Fix the resume flow based on Task 2.1 findings; add a rename modal so users can set a session title before launch.

**Context boundary:**
- Primary: `web-app/src/app/history/page.tsx`
- Supporting: `web-app/src/components/history/HistoryDetailPanel.tsx`, new `ResumeModal.tsx`
- ~80 lines changed + ~120 lines new (modal)

**Prerequisites:**
- Task 2.1 root cause identified
- Understanding of `CreateSessionRequest` fields: `title`, `path`, `resume_id`, `session_type`

**Implementation approach:**
1. Apply the root cause fix from Task 2.1
2. Replace the inline `handleResumeSession` logic with a "Resume" button that opens a `<ResumeModal>`
3. `ResumeModal` fields:
   - Session title input (pre-filled with `entry.name`, editable)
   - Target path (pre-filled with `entry.project`, read-only for simple resume)
   - "Launch" button calls `createSession({ title, path, resumeId: entry.id })`
4. On success: `router.push("/")` to return to session list
5. On error: show error inside modal (not global error banner)

**Validation strategy:**
- Manual: resume a known-good history entry; verify Claude loads with `--resume` context
- Manual: rename the session in the modal; verify new title appears on session list
- Manual: resume with a non-existent project path; verify error shows in modal
- Unit: modal renders with pre-filled title from entry

**INVEST check:**
- Independent: depends only on Task 2.1 root cause, not on Story 1
- Negotiable: modal fields, styling flexible
- Valuable: core requirement — resume must work
- Estimable: 3h assuming root cause is identified
- Small: modal + fix is bounded scope
- Testable: manual verification is straightforward

---

### Story 3: Rich Session Cards + Inline Preview [1 week]

**User value**: Each session card shows enough metadata to identify the session without opening it; message preview is one click away.

**Acceptance criteria:**
- Cards display: status indicator (color + pill), repo name, branch, last-active (relative), creation timestamp, message count badge
- Cards show a 1-line snippet of the last message (no interaction required)
- Clicking a chevron on the card expands an inline accordion with last 3–5 message pairs
- Expanded preview lazy-loads messages only when first expanded
- ANSI escape codes are stripped from message previews (no garbage characters)
- Cards use vanilla-extract CSS per ADR-009 (`.css.ts` files)

---

#### Task 3.1: Update HistoryEntryCard with rich metadata display [3h]

**Objective**: Redesign `HistoryEntryCard` to display the full metadata set from the enriched `ClaudeHistoryEntry` proto (branch, status, timestamps, message count).

**Context boundary:**
- Primary: `web-app/src/components/history/HistoryEntryCard.tsx`
- Supporting: `web-app/src/components/history/HistoryEntryCard.module.css` → replace with `HistoryEntryCard.css.ts`
- ~200 lines changed

**Prerequisites:**
- Task 1.1 complete (new proto fields available in TypeScript generated code)
- Task 1.3 complete (backend populates branch and session_status)
- ADR-009: vanilla-extract CSS (`.css.ts` colocated with component)

**Implementation approach:**
1. Create `HistoryEntryCard.css.ts` using vanilla-extract `recipe()`:
   - Status accent bar variants: `running` (green + animated pulse), `paused` (yellow), `stopped` (grey)
   - Card layout: left accent bar, title row, metadata row, snippet row, expand row
2. Update `HistoryEntryCard.tsx`:
   - Left edge: `<div className={statusBar({ status: entry.sessionStatus })}>`
   - Title row: session name (truncated to 1 line)
   - Metadata row: repo icon + name, branch icon + name, last-active relative timestamp, message count badge
   - Status pill (right-aligned): "Running" / "Paused" / "Done"
3. Add `strip-ansi` import for the last-message snippet (see Task 3.2)
4. Keep `HistoryEntryCard.module.css` as an empty stub until all consumers migrate

**Validation strategy:**
- Visual: card matches the wireframe from features.md §4
- Visual: status variants render correct colors; "Running" shows animated pulse
- Unit: snapshot test for card with `session_status: RUNNING`
- Accessibility: status pill has `aria-label="Session status: Running"`

**INVEST check:**
- Independent: UI change; no backend dependency for the card layout itself
- Negotiable: exact color values, font sizes, layout spacing
- Valuable: directly addresses the identity problem
- Estimable: 3h with high confidence
- Small: single component + its CSS
- Testable: visual snapshot + accessibility attribute check

---

#### Task 3.2: Add inline message preview with ANSI stripping [3h]

**Objective**: Add expand/collapse inline message preview to `HistoryEntryCard`; implement safe ANSI stripping for preview text.

**Context boundary:**
- Primary: `web-app/src/components/history/HistoryEntryCard.tsx`
- New file: `web-app/src/components/history/HistoryCardPreview.tsx`
- New file: `web-app/src/components/history/HistoryCardPreview.css.ts`
- Supporting: existing `getClaudeHistoryMessages` RPC hook
- ~150 lines new

**Prerequisites:**
- Task 3.1 complete (card layout established)
- `strip-ansi` v7 installed: `npm install strip-ansi@7.2.0` (pin exact version per pitfalls.md §3)

**Implementation approach:**
1. Install `strip-ansi`: `cd web-app && npm install strip-ansi@7.2.0`
2. In `HistoryEntryCard.tsx`, add `isExpanded: boolean` prop + toggle button:
   ```tsx
   <button
     aria-label={isExpanded ? "Collapse preview" : "Show messages"}
     aria-expanded={isExpanded}
     aria-controls={`preview-${entry.id}`}
     onClick={(e) => { e.stopPropagation(); onToggleExpand(entry.id); }}
   >
     {isExpanded ? "▲ Collapse" : `▼ Show ${entry.messageCount} messages`}
   </button>
   ```
3. Create `HistoryCardPreview.tsx`:
   - Receives `entryId: string`, `isVisible: boolean`
   - On first render when `isVisible=true`: call `getClaudeHistoryMessages({ id: entryId, limit: 5 })`
   - Cache fetched messages in `useRef<Map<string, ClaudeMessage[]>>` to avoid re-fetching
   - Render last 3–5 message pairs in accordion; most-recent pre-expanded
   - Strip ANSI from each message content: `stripAnsi(msg.content).slice(0, 300)`
   - Render as `<span>{strippedContent}</span>` — never `dangerouslySetInnerHTML`
4. In `HistoryGroupView.tsx`, maintain `expandedIds: Set<string>` state; pass `isExpanded` and `onToggleExpand` down

**Validation strategy:**
- Unit: preview is not fetched until card is expanded
- Unit: ANSI sequences are stripped (test with `\x1b[31mred text\x1b[0m`)
- Unit: OSC sequences (`\x1b]8;;https://...\x07`) are stripped
- Visual: expanded card shows last messages without garbage characters
- Accessibility: `aria-expanded` reflects state correctly

**INVEST check:**
- Independent: depends on Task 3.1 for card layout; preview logic is self-contained
- Negotiable: number of messages shown, expand animation
- Valuable: core "context without launching" requirement
- Estimable: 3h with high confidence
- Small: one new component + toggle state in group view
- Testable: ANSI stripping and lazy fetch are unit-testable

---

### Story 4: Virtual Scrolling + Infinite Load [1 week]

**User value**: The history page renders in ≤200ms and scrolls 200+ sessions without jank.

**Acceptance criteria:**
- Initial render of 200+ sessions completes in ≤200ms (measured via browser performance timeline)
- List uses `useVirtualizer` from `@tanstack/react-virtual`
- Load-more trigger fires when user scrolls to 80% of list height; fetches next page via cursor
- Expand/collapse of a card does not cause visible scroll position jump for other cards
- Keyboard navigation (↑↓ / j/k) continues to work on the virtualized list

---

#### Task 4.1: Install dependencies and create virtualized list scaffold [2h]

**Objective**: Install `@tanstack/react-virtual`; create the `VirtualHistoryList` component scaffold that wraps `useVirtualizer` with the correct configuration.

**Context boundary:**
- Primary: new `web-app/src/components/history/VirtualHistoryList.tsx`
- New file: `web-app/src/components/history/VirtualHistoryList.css.ts`
- Supporting: `web-app/package.json`
- ~120 lines new

**Prerequisites:**
- `node_modules` available; npm install access
- Familiarity with `useVirtualizer` API: `count`, `getScrollElement`, `estimateSize`, `overscan`, `measureElement`

**Implementation approach:**
1. Install: `cd web-app && npm install @tanstack/react-virtual`
2. Create `VirtualHistoryList.tsx`:
   ```tsx
   const virtualizer = useVirtualizer({
     count: items.length + (hasNextPage ? 1 : 0),
     getScrollElement: () => scrollContainerRef.current,
     estimateSize: () => 72,        // collapsed card height
     overscan: 10,                  // pre-render 10 off-screen items
     measureElement: (el) => el.getBoundingClientRect().height,
     shouldAdjustScrollPositionOnItemSizeChange: () => adjustRef.current,
   });
   ```
3. Implement `adjustRef` expand-animation guard (from pitfalls.md §1):
   ```tsx
   const adjustRef = useRef(true);
   function handleToggleExpand(id: string) {
     adjustRef.current = false;
     setExpandedIds(prev => toggle(prev, id));
     requestAnimationFrame(() => { adjustRef.current = true; });
   }
   ```
4. Render: `<div style={{ height: virtualizer.getTotalSize() }}>` with translated item blocks
5. Add intersection-observer sentinel row at bottom: triggers `onLoadMore` when 80% depth reached
6. Export component; wire up a placeholder `items` prop (connected in Task 4.2)

**Validation strategy:**
- Visual: renders N rows where N = viewport height / 72px, not all 500
- Visual: no empty white space or DOM overflow
- Unit: virtualizer renders only visible items (item count < total entries)

**INVEST check:**
- Independent: scaffold has no real data dependency; uses placeholder items
- Negotiable: estimateSize, overscan count, sentinel threshold
- Valuable: establishes the virtualization structure for Task 4.2
- Estimable: 2h with high confidence
- Small: component scaffold only, no data wiring
- Testable: DOM item count < total verifies virtualization works

---

#### Task 4.2: Wire infinite query to VirtualHistoryList [3h]

**Objective**: Replace the `loadHistory(limit: 500)` call in `page.tsx` with `useInfiniteQuery` + cursor pagination; connect paginated data to `VirtualHistoryList`.

**Context boundary:**
- Primary: `web-app/src/app/history/page.tsx`
- Supporting: `web-app/src/components/history/VirtualHistoryList.tsx` (from Task 4.1)
- `web-app/src/components/history/HistoryGroupView.tsx`
- ~150 lines changed

**Prerequisites:**
- Task 1.2 complete (backend returns `next_page_token`)
- Task 4.1 complete (`VirtualHistoryList` scaffold ready)
- Decision: use plain `useReducer` + `usePaginatedHistory` hook (no `@tanstack/react-query`) to minimize dependency surface (per architecture.md §4 tradeoff note)

**Implementation approach:**
1. Create `usePaginatedHistory` hook in `web-app/src/lib/hooks/usePaginatedHistory.ts`:
   - State: `{ pages: ClaudeHistoryEntry[][], nextPageToken: string, loading: boolean, error: string | null }`
   - `fetchFirstPage()`: call `listClaudeHistory({ pageSize: 50 })`, replace pages
   - `fetchNextPage()`: call `listClaudeHistory({ pageSize: 50, pageToken: nextPageToken })`, append page
   - `allEntries`: computed from `pages.flat()`
2. In `page.tsx`:
   - Replace `useState<ClaudeHistoryEntry[]>([])` + `loadHistory` with `usePaginatedHistory`
   - Pass `allEntries` to `useHistoryFilters`, `useHistoryGrouping` (unchanged hooks)
   - Pass `fetchNextPage` and `hasNextPage` to `VirtualHistoryList` as `onLoadMore`
3. Replace `<HistoryGroupView>` with `<VirtualHistoryList>` passing virtualized `items`
4. Update `selectedEntry` tracking to use `entry.id` key instead of array index

**Validation strategy:**
- Performance: open browser DevTools Performance; record page load; verify LCP ≤200ms
- Manual: scroll to bottom of list; verify next page loads automatically
- Manual: filter by search query; verify first page is refetched with filter params
- Manual: select entry with ↑↓ keys; verify selection works across page boundaries
- No regressions: existing filter/search/grouping hooks work with `allEntries`

**INVEST check:**
- Independent: depends on Tasks 1.2 and 4.1; otherwise self-contained
- Negotiable: hook structure, state shape
- Valuable: meets 200ms performance requirement
- Estimable: 3h with moderate confidence (data wiring can surface edge cases)
- Small: one hook + wiring in page.tsx
- Testable: LCP measurement is objective

---

#### Task 4.3: Keyboard navigation on virtualized list [2h]

**Objective**: Restore ↑↓/j/k keyboard navigation on the virtualized list with correct focus management and `aria-activedescendant`.

**Context boundary:**
- Primary: `web-app/src/app/history/page.tsx` (keydown handler)
- Supporting: `web-app/src/components/history/VirtualHistoryList.tsx`
- ~60 lines changed

**Prerequisites:**
- Task 4.2 complete (`VirtualHistoryList` wired with real data)
- `focusedIndex` concept exists; needs to be connected to virtualizer's `scrollToIndex`

**Implementation approach:**
1. Add `focusedIndex` state to `page.tsx` (replaces `selectedIndex`)
2. In the `onKeyDown` handler:
   - `ArrowDown/j`: `setFocusedIndex(i => Math.min(i + 1, allEntries.length - 1))`
   - `ArrowUp/k`: `setFocusedIndex(i => Math.max(i - 1, 0))`
   - After state update: `virtualizer.scrollToIndex(focusedIndex, { align: 'auto' })`
3. Pass `focusedIndex` to `VirtualHistoryList`; apply `aria-selected` and `aria-activedescendant` on the focused row
4. Add `useEffect(() => { rowRef.current?.focus() }, [focusedIndex])` — restores DOM focus after virtualizer reconciles
5. On `PageDown`/`PageUp`: jump by `Math.floor(viewportHeight / 72)` items

**Validation strategy:**
- Manual: navigate 200-item list with ↓/↑; verify smooth scrolling and focus tracking
- Manual: navigate past page 1 boundary; verify next page loads and focus continues
- Accessibility: `aria-activedescendant` on scroll container points to focused row ID
- No regression: `Enter` still opens message modal for focused entry

**INVEST check:**
- Independent: depends on Task 4.2; keyboard logic is otherwise self-contained
- Negotiable: scroll alignment, page-jump size
- Valuable: keyboard power users (including the primary user) need this
- Estimable: 2h with high confidence
- Small: ~60 lines in keydown handler + virtualizer scroll call
- Testable: aria attributes + focus management are testable

---

### Story 5: Fork/Resume Modal and Unified Search [1 week]

**User value**: Fork to a new worktree or directory in one interaction; search across repo, branch, date, and message content in a single filter bar.

**Acceptance criteria:**
- Split-button on each card: primary "Resume" opens rename modal; chevron dropdown reveals "New worktree", "Open in directory", "Clone to..."
- Fork modal has: session title input, target path, branch name (auto-suggested), session type radio, "Fork at message" range slider
- Fork calls `createSession({ forkSourceId, forkAtMessage, ... })` (from Task 1.4)
- Repo and branch filters work in the filter bar; date range filter works
- Full-text search and metadata search are unified (no separate "modes")

---

#### Task 5.1: Build ForkModal component [3h]

**Objective**: Create `ForkModal.tsx` — the dialog for configuring and launching a forked session.

**Context boundary:**
- New file: `web-app/src/components/history/ForkModal.tsx`
- New file: `web-app/src/components/history/ForkModal.css.ts`
- Supporting: `web-app/src/app/history/page.tsx` (caller)
- ~200 lines new

**Prerequisites:**
- Task 1.4 complete (backend fork dispatch working)
- Task 3.1 complete (HistoryEntryCard has "Fork" action button)
- Understanding of `<dialog>` element and focus trap pattern

**Implementation approach:**
1. Create `ForkModal.tsx` as a controlled `<dialog>` component:
   ```tsx
   type ForkModalProps = {
     entry: ClaudeHistoryEntry | null;
     onClose: () => void;
     onSubmit: (params: ForkParams) => void;
   };
   ```
2. Form fields:
   - Session title: `<input>` pre-filled with `entry.name`
   - Target path: `<input>` pre-filled with `entry.project`
   - Branch name: `<input>` auto-suggested as `resume/${entry.id.slice(0, 8)}-${Date.now()}`
   - Session type: radio buttons (Directory / New worktree / Existing worktree)
   - Fork at message: `<input type="range">` 1..`entry.messageCount`, label shows "Keep first N of M messages"
3. On submit: call `createSession({ title, path, branch, sessionType, forkSourceId: entry.id, forkAtMessage })`
4. CSS via vanilla-extract `ForkModal.css.ts` (per ADR-009); use `vars` tokens, no hardcoded colors
5. Focus trap: `useEffect` to set `inert` on `document.body` children outside dialog while open; return focus to trigger button on close
6. Keyboard: `Escape` closes; `Enter` on last field submits

**Validation strategy:**
- Manual: open fork modal; verify all fields are pre-filled correctly
- Manual: fork a session to a new worktree; verify it appears on session list
- Manual: press Escape; verify focus returns to "Fork" button
- Unit: pre-filled branch name matches `resume/<id-prefix>-<timestamp>` pattern
- Accessibility: `<dialog>` has `aria-labelledby` pointing to title heading

**INVEST check:**
- Independent: isolated modal component; only needs entry data from parent
- Negotiable: field set, range slider behavior
- Valuable: fork is a first-class user requirement
- Estimable: 3h with high confidence
- Small: single modal component
- Testable: field pre-fill + submit are testable

---

#### Task 5.2: Add split-button (Resume / Fork) to session cards [2h]

**Objective**: Add the split-button pattern (primary "Resume" + chevron dropdown with fork options) to `HistoryDetailPanel` and `HistoryEntryCard`.

**Context boundary:**
- Primary: `web-app/src/components/history/HistoryDetailPanel.tsx`
- Supporting: `web-app/src/components/history/HistoryEntryCard.tsx`
- ~80 lines changed

**Prerequisites:**
- Task 2.2 complete (resume works + rename modal exists)
- Task 5.1 complete (ForkModal exists)
- Familiarity with CSS `position: relative` dropdown pattern

**Implementation approach:**
1. In `HistoryDetailPanel.tsx`, replace the single "Resume" button with a split-button:
   - Left button: "Resume" → opens `ResumeModal` (from Task 2.2)
   - Right chevron button: toggles dropdown with options:
     - "New worktree" → opens `ForkModal` with `sessionType: NEW_WORKTREE` pre-selected
     - "Open in directory..." → opens `ForkModal` with `sessionType: DIRECTORY` pre-selected
     - "Copy path" → `navigator.clipboard.writeText(entry.project)`
2. Dropdown: positioned `absolute` below split-button; closed on click-outside or Escape
3. In `HistoryEntryCard.tsx`, add hover-visible "Resume" quick action button that triggers the same flow

**Validation strategy:**
- Manual: click "Resume" → rename modal opens
- Manual: click chevron → dropdown shows 3 options
- Manual: click "New worktree" → fork modal opens with correct session type pre-selected
- Accessibility: dropdown items have `role="menuitem"`; chevron button has `aria-haspopup="menu"`

**INVEST check:**
- Independent: composes Tasks 2.2 and 5.1; split-button itself is self-contained
- Negotiable: dropdown vs slide-out panel, exact options
- Valuable: discoverability of fork workflow depends on this button
- Estimable: 2h with high confidence
- Small: ~80 lines of composition
- Testable: interaction flow testable manually

---

#### Task 5.3: Unify search and add repo/branch/date filters [2h]

**Objective**: Remove the "metadata search" vs "full-text search" mode split; add repo and date-range filters to `HistoryFilterBar`.

**Context boundary:**
- Primary: `web-app/src/lib/hooks/useHistoryFilters.ts`
- Supporting: `web-app/src/components/history/HistoryFilterBar.tsx`
- ~100 lines changed

**Prerequisites:**
- Task 1.3 complete (backend populates `branch` field)
- No backend changes needed for this task (client-side filtering against `allEntries`)

**Implementation approach:**
1. In `useHistoryFilters.ts`:
   - Add `branchFilter: string` and `dateRangeFilter: { start: Date | null, end: Date | null }` to filter state
   - Apply branch filter: `entry.branch.toLowerCase().includes(branchFilter.toLowerCase())`
   - Apply date range: `entry.updatedAt >= start && entry.updatedAt <= end`
   - Remove `searchMode` state — always search both metadata and full-text simultaneously
2. In `HistoryFilterBar.tsx`:
   - Add branch text input (debounced 200ms)
   - Add date range: two `<input type="date">` fields
   - Remove "mode" toggle (metadata vs fulltext); show full-text results inline in the same list
3. Update `page.tsx` to remove `searchMode`-conditional rendering; always render `HistoryGroupView` with `filteredEntries` + full-text results merged

**Validation strategy:**
- Manual: type a branch name in branch filter; verify list filters correctly
- Manual: set date range; verify only sessions in range appear
- Manual: type search query; verify both metadata matches and full-text matches appear in list
- Unit: `useHistoryFilters` applies branch filter correctly to test fixture

**INVEST check:**
- Independent: client-side filter changes only; no backend dependency
- Negotiable: filter UI design, merge strategy for metadata + full-text
- Valuable: completes the search experience requirement
- Estimable: 2h with high confidence
- Small: hook + filter bar changes are bounded
- Testable: filter logic is pure function, unit-testable

---

## Known Issues / Proactive Bug Tracking

### BUG-HPR-001: Virtual scroll scroll-anchor stutter on upward scroll [SEVERITY: Medium]

**Description**: TanStack Virtual v3 re-measures item heights during upward scroll, producing a correction loop that causes stutter on macOS Chrome and iOS Safari (GitHub issues #659, #832).

**Mitigation:**
- Implement `adjustRef` guard: `shouldAdjustScrollPositionOnItemSizeChange: () => adjustRef.current`
- Set `adjustRef.current = false` before expand animation; reset after `requestAnimationFrame`
- Overestimate `estimateSize` (return max expanded height ~300px) to prefer under-correction

**Files likely affected:**
- `web-app/src/components/history/VirtualHistoryList.tsx` — virtualizer config

**Prevention strategy:**
- Include upward scroll test in QA checklist
- Test on macOS Chrome and Safari before merging Story 4

**Related tasks**: Task 4.1, Task 4.2

---

### BUG-HPR-002: Git worktree "branch already checked out" on fork [SEVERITY: High]

**Description**: `git worktree add <path> <branch>` fails if the branch is already checked out in another worktree. Stale worktrees from deleted directories produce the same error.

**Mitigation:**
- Always generate unique branch name: `resume/<source-branch>-<unix_timestamp>`
- Run `git worktree prune` before fork operation
- Pass explicit commit SHA as starting point, not branch name
- Surface clear error message ("Branch X is already in use — try a different name")

**Files likely affected:**
- `session/instance.go` — worktree creation
- `server/services/session_service.go` — fork error handling

**Prevention strategy:**
- Add unit test: fork with a branch already checked out returns descriptive error
- Add `git worktree prune` call to application startup

**Related tasks**: Task 1.4, Task 5.1

---

### BUG-HPR-003: OSC hyperlink sequences survive naive ANSI stripping [SEVERITY: Medium]

**Description**: Claude CLI output contains OSC 8 hyperlinks (`\x1b]8;;https://...\x07`). SGR-only strippers miss these, rendering as garbage in preview text. `strip-ansi` v7 handles them, but only if pinned to ≥7.1.0.

**Mitigation:**
- Pin `strip-ansi` to exact version `7.2.0` in `package.json`
- Add test fixture with OSC sequence and verify stripped output is clean text
- Never use `dangerouslySetInnerHTML` for preview content

**Files likely affected:**
- `web-app/src/components/history/HistoryCardPreview.tsx`
- `web-app/package.json`

**Prevention strategy:**
- Include OSC sequence in the `strip-ansi` unit test
- Lint rule: flag any `dangerouslySetInnerHTML` in history components

**Related tasks**: Task 3.2

---

### BUG-HPR-004: ConnectRPC transport recreated on every render [SEVERITY: High]

**Description**: The current `page.tsx` creates `createConnectTransport` inside a `useEffect`. Moving pagination to a custom hook risks creating new transport instances on re-renders, causing `useEffect` infinite loops (pitfalls.md §5).

**Mitigation:**
- Create transport as a module-level singleton outside any component
- Use `useRef` for the client instance
- The `usePaginatedHistory` hook must accept an externally-created client reference, not create its own

**Files likely affected:**
- `web-app/src/app/history/page.tsx`
- `web-app/src/lib/hooks/usePaginatedHistory.ts`

**Prevention strategy:**
- ESLint rule: warn on `createConnectTransport` inside component body
- Code review checklist: verify transport is module-level singleton

**Related tasks**: Task 4.2

---

### BUG-HPR-005: messageCount underestimate for long conversations [SEVERITY: Low]

**Description**: `parseConversationFile` caps scanning at `maxLinesToScan = 1000`, so `message_count` underestimates long conversations. The fork modal's "fork at message" slider will show a lower max than the actual conversation length.

**Mitigation:**
- Display "up to N messages" in slider label
- Fetch actual count via `getClaudeHistoryMessages({ id, limit: 0 }).totalCount` before opening fork modal
- Show loading state while count is fetching

**Files likely affected:**
- `web-app/src/components/history/ForkModal.tsx`
- `session/history.go` — maxLinesToScan constant

**Prevention strategy:**
- Add note in ForkModal: "Message count is estimated; actual conversation may be longer"
- Future: remove 1000-line scan cap or add a separate metadata-only parse pass

**Related tasks**: Task 5.1

---

## Dependency Visualization

```
Story 1: Backend Foundation
  Task 1.1 (Proto changes)
    ├── Task 1.2 (Cursor pagination)
    ├── Task 1.3 (Git enrichment)
    └── Task 1.4 (Fork dispatch)

Story 2: Fix Resume (PARALLEL with Story 1)
  Task 2.1 (Diagnose)
    └── Task 2.2 (Fix + rename modal)

Story 3: Rich Cards (requires Story 1 done)
  Task 3.1 (Card metadata display)
    └── Task 3.2 (Inline preview + ANSI strip)

Story 4: Virtual Scrolling (requires Story 1 done)
  Task 4.1 (VirtualHistoryList scaffold) ─────────────────┐
  Task 1.2 (Cursor pagination) ──────────────────────────┤
                                                          └── Task 4.2 (Wire infinite query)
                                                               └── Task 4.3 (Keyboard nav)

Story 5: Fork Modal + Unified Search (requires Stories 2, 3, 4 done)
  Task 2.2 (Resume modal) ──────────────────────────────┐
  Task 1.4 (Fork dispatch) ──────────────────────────────┤
                                                          └── Task 5.1 (ForkModal)
  Task 5.1 (ForkModal) ─────────────────────────────────┐
  Task 2.2 (Resume modal) ──────────────────────────────┤
                                                          └── Task 5.2 (Split-button)

  Task 1.3 (Branch enrichment) ─────────────────────────── Task 5.3 (Unified search) [independent]

Parallel tracks:
  [Story 1] ══════════════════════╗
  [Story 2] ══════════════════════╣ → [Story 3] → [Story 5]
                                   ╚ → [Story 4] → [Story 5]
```

---

## Integration Checkpoints

**After Story 1 (Backend Foundation):**
- `ListClaudeHistory` returns paginated results with `next_page_token`
- `ClaudeHistoryEntry` includes `branch`, `session_status` fields populated
- `CreateSession` with `fork_source_id` creates a forked conversation and starts a session
- All existing tests pass (`make test`)

**After Story 2 (Fix Resume):**
- Clicking "Resume" on a history entry launches a Claude session with `--resume <id>` and the conversation context is restored
- User can rename the session before launch

**After Story 3 (Rich Cards):**
- History cards show repo, branch, status, timestamps, message count without any clicks
- Expanding a card shows the last 3–5 messages with no ANSI garbage

**After Story 4 (Virtual Scrolling):**
- Initial render of 200+ sessions ≤ 200ms (measured via Lighthouse / browser DevTools)
- Scrolling a 500-entry list is smooth (no DOM of 500 mounted components)
- Keyboard navigation works end-to-end on the virtualized list

**After Story 5 (Fork Modal + Search) — Feature Complete:**
- Full fork/resume workflow works: resume (with rename), fork to new worktree, fork to directory
- Branch and date range filters work in the filter bar
- Full-text and metadata search are unified — no mode toggle
- All success metrics from the Epic Overview are verifiable

---

## Context Preparation Guide

### Task 1.1 (Proto changes)
- Files to load: `proto/session/v1/session.proto`, `proto/session/v1/types.proto` (SessionStatus enum)
- Concepts: proto3 field numbering, backward-compatible field rename (same number = same wire type)

### Task 1.2 (Cursor pagination)
- Files to load: `server/services/search_service.go`, `session/history.go`
- Concepts: in-memory sorted slice, base64url encoding, AIP-158 cursor pattern

### Task 1.3 (Git enrichment)
- Files to load: `server/services/search_service.go`, `session/storage.go`, `session/instance.go`
- Concepts: session store lookup by ResumeId, go-git Worktree.Status(), exec.Command timeout

### Task 1.4 (Fork dispatch)
- Files to load: `server/services/session_service.go`, `session/history_fork.go`
- Concepts: ForkClaudeConversation signature, ConnectRPC error codes

### Task 2.1 (Diagnose resume)
- Files to load: `web-app/src/app/history/page.tsx` (handleResumeSession), `server/services/session_service.go`, `session/instance.go`
- Concepts: `claude --resume <uuid>` flag behavior, tmux session start sequence

### Task 2.2 (Fix + rename modal)
- Files to load: `web-app/src/app/history/page.tsx`, `web-app/src/components/history/HistoryDetailPanel.tsx`
- Concepts: `<dialog>` element, ConnectRPC createSession call

### Task 3.1 (Rich card layout)
- Files to load: `web-app/src/components/history/HistoryEntryCard.tsx`, `web-app/src/styles/theme.css.ts`, `docs/adr/009-vanilla-extract-type-safe-css.md`
- Concepts: vanilla-extract `recipe()`, status enum mapping to CSS variants

### Task 3.2 (Inline preview + ANSI)
- Files to load: `web-app/src/components/history/HistoryEntryCard.tsx`, `web-app/src/components/history/HistoryGroupView.tsx`
- Concepts: `strip-ansi` API, lazy fetch with `useRef` cache, `aria-expanded`/`aria-controls`

### Task 4.1 (VirtualHistoryList scaffold)
- Files to load: `web-app/src/components/history/HistoryGroupView.tsx`
- Concepts: `useVirtualizer` API, `measureElement`, `shouldAdjustScrollPositionOnItemSizeChange`

### Task 4.2 (Wire infinite query)
- Files to load: `web-app/src/app/history/page.tsx`, `web-app/src/lib/hooks/useHistoryFilters.ts`, `web-app/src/lib/hooks/useHistoryGrouping.ts`
- Concepts: cursor pagination, `usePaginatedHistory` hook design, ConnectRPC client as module singleton

### Task 4.3 (Keyboard nav)
- Files to load: `web-app/src/app/history/page.tsx` (keydown handler), `web-app/src/components/history/VirtualHistoryList.tsx`
- Concepts: `virtualizer.scrollToIndex`, `aria-activedescendant`, focus restoration after reconcile

### Task 5.1 (ForkModal)
- Files to load: `web-app/src/components/history/HistoryDetailPanel.tsx`, `session/history_fork.go`
- Concepts: `<dialog>` focus trap, `inert` attribute, fork_source_id / fork_at_message proto fields

### Task 5.2 (Split-button)
- Files to load: `web-app/src/components/history/HistoryDetailPanel.tsx`, `web-app/src/components/history/HistoryEntryCard.tsx`
- Concepts: split-button pattern, `role="menu"` / `role="menuitem"` ARIA

### Task 5.3 (Unified search)
- Files to load: `web-app/src/lib/hooks/useHistoryFilters.ts`, `web-app/src/components/history/HistoryFilterBar.tsx`
- Concepts: client-side filter composition, debounced input, date range comparison

---

## Success Criteria

- All 13 atomic tasks completed and validated
- All story acceptance criteria met
- Initial history list render ≤ 200ms for 200+ sessions (measured)
- Resume action reliably launches Claude with correct conversation context
- Fork to new worktree / directory works end-to-end
- ANSI codes stripped from all message preview content
- Keyboard navigation (↑↓/j/k + Enter + Escape) works on virtualized list
- No regressions to existing session views, terminal streaming, or search behavior
- `make test` passes; `make lint` passes
- All 5 known issues (BUG-HPR-001 through BUG-HPR-005) addressed or explicitly accepted
