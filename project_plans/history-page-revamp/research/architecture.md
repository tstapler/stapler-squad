# History Page Revamp — Architecture Research

**Date**: 2026-04-12
**Branch**: stapler-squad-history-resumption

---

## 1. Proto Changes: Enriching `ClaudeHistoryEntry`

### What to add

```protobuf
message ClaudeHistoryEntry {
  // ... existing fields 1-7 unchanged ...

  // Git branch the conversation was run on (extracted from JSONL cwd field
  // or from the stapler-squad session record if the session still exists).
  string branch = 8;

  // Short status summary, e.g. "2 modified, 1 untracked".
  // Only populated when a corresponding stapler-squad worktree exists.
  // Empty string means "no worktree / unknown".
  string git_status_summary = 9;

  // Session lifecycle status from stapler-squad's session.Status enum.
  // Maps to: RUNNING | READY | PAUSED | STOPPED | UNKNOWN.
  // Populated by cross-referencing ClaudeHistoryEntry.id against
  // the in-memory session store.
  SessionStatus session_status = 10;

  // First line of the HEAD commit message for the associated worktree.
  // Only populated when a worktree exists.
  string last_commit_message = 11;

  // Number of files changed between HEAD and working tree.
  // Populated lazily; -1 = not available.
  int32 diff_file_count = 12;
}
```

### Where to source each field

| Field | Source | Cost |
|---|---|---|
| `branch` | JSONL first-line `cwd` field → run `git rev-parse --abbrev-ref HEAD` in that dir, cached per path | One `exec.Command` per unique project path per cache cycle |
| `git_status_summary` | Only when a live stapler-squad worktree exists: `go-git` `Worktree().Status()` | Zero if session not in store |
| `session_status` | `session.Storage` in-memory lookup by `resume_id` → `session.Status` | O(1) map lookup |
| `last_commit_message` | `go-git` `Repository.Head()` + `CommitObject()` on the worktree repo | Only when worktree exists |
| `diff_file_count` | Count of non-clean entries from `go-git` `Status()` | Same call as git_status_summary |

### Key design decision

Do **not** eagerly populate git fields on every `ListClaudeHistory` call. The history cache loads by scanning all JSONL files; adding git I/O inside `parseConversationFile` would multiply disk latency. Instead:

- `session_status` is cheap (in-memory) — populate always.
- `branch` can be cached in a secondary `map[string]string` (project path → branch) with a separate 60-second TTL; populated lazily on first miss.
- `git_status_summary`, `last_commit_message`, `diff_file_count` are only populated when a stapler-squad `session.Instance` with `ResumeId == entry.ID` exists in the session store. This keeps history load fast for the 99% case (sessions with no live worktree).

---

## 2. Pagination API Shape for `ListClaudeHistory`

### Recommended: opaque cursor string (AIP-158 style)

```protobuf
message ListClaudeHistoryRequest {
  optional string project      = 1;
  optional string search_query = 2;
  int32           page_size    = 3;  // replaces `limit`; default 50, max 200
  string          page_token   = 4;  // opaque; empty = first page
}

message ListClaudeHistoryResponse {
  repeated ClaudeHistoryEntry entries        = 1;
  int32                       total_count    = 2;
  string                      next_page_token = 3;  // empty = no more pages
}
```

### Cursor encoding

The history list is already sorted by `UpdatedAt` descending. A safe cursor is:

```
base64url( updatedAt_unix_ns + ":" + entry_id )
```

Example server-side:

```go
type pageCursor struct {
    UpdatedAt int64  `json:"u"`
    ID        string `json:"i"`
}

func encodeCursor(c pageCursor) string {
    b, _ := json.Marshal(c)
    return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (pageCursor, error) {
    b, err := base64.RawURLEncoding.DecodeString(s)
    ...
}
```

Clients receive an opaque string; the server uses the decoded `(updatedAt, id)` pair to resume a slice-scan of the in-memory sorted list. This is O(log N) with a binary search on `updatedAt`, falling back to linear scan only for ties.

### Why not offset?

The current `limit` approach is essentially offset=0 pagination. Pure offset (`LIMIT x OFFSET y`) is fragile when new entries arrive between pages (duplicates/skips). Cursor-based pagination on `updatedAt+id` is stable, cacheable per cursor value, and gives the frontend a reliable "load more" primitive.

### Page size recommendation

Default 50, max 200. The current 500-entry eager load is a UX bottleneck (no virtual scrolling yet). With cursor pagination + virtual scrolling, 50 per page is enough to keep the viewport full while keeping initial load under 100ms.

### Backward compatibility

Keep the existing `limit` field (field 3) as a deprecated alias — it's used by the current frontend. The new `page_size` (also field 3, same wire number) replaces it. Simply rename the field in the proto and regenerate; existing callers using the old name will keep working because the wire format is identical.

---

## 3. Fork Action API

### Recommendation: extend `CreateSession`, do not add a new RPC

`CreateSession` already carries `resume_id`, `path`, `branch`, `session_type`, and `existing_worktree`. The fork action needs only two additions:

```protobuf
message CreateSessionRequest {
  // ... existing fields unchanged ...

  // Optional: ID of a ClaudeHistoryEntry to fork from.
  // When set, the backend calls ForkClaudeConversation(src, fork_at_message, dst)
  // and sets the resulting UUID as the effective resume_id.
  // If resume_id is also set, fork_source_id takes precedence.
  string fork_source_id = 12;

  // Optional: Truncate the forked conversation at this message index (1-based).
  // 0 or absent = copy all messages.
  uint64 fork_at_message = 13;
}
```

`target_type` does not need a new enum — it is already covered by `session_type` (`SessionType`: `SESSION_TYPE_DIRECTORY`, `SESSION_TYPE_NEW_WORKTREE`, `SESSION_TYPE_EXISTING_WORKTREE`). `target_path` is already `path` + `working_dir`.

### Backend implementation path

In `server/services/session_service.go` `CreateSession` handler:

1. If `fork_source_id != ""`, locate the source JSONL file via `ClaudeSessionHistory.GetByID`.
2. Call `session.ForkClaudeConversation(srcPath, forkAtMessage, dstDir)` → returns `newUUID`.
3. Override `req.Msg.ResumeId = newUUID` and continue normal `CreateSession` flow.

`ForkClaudeConversation` already exists in `session/history_fork.go` with the correct signature. No new package-level code is needed.

### Tradeoff vs. new RPC

A dedicated `ForkHistorySession` RPC would be cleaner for API consumers that only want to fork without starting a session. However, the fork action in the UI always immediately starts a session, so merging the two saves a round trip. If a "fork-only" use case emerges later, a thin wrapper RPC can be added without changing the internal logic.

---

## 4. Frontend Component Architecture

### (a) Virtual scrolling for the entry list

**Library**: `@tanstack/react-virtual` (`useVirtualizer`) combined with `@tanstack/react-query` (`useInfiniteQuery`).

The existing `HistoryGroupView` renders a flat `flatEntries` array with no windowing. The minimal refactor:

1. Add `@tanstack/react-virtual` and `@tanstack/react-query` (or add just `react-virtual` if full Query migration is too large).
2. Wrap the ConnectRPC `listClaudeHistory` call in `useInfiniteQuery`:

```ts
const { data, fetchNextPage, hasNextPage, isFetchingNextPage } =
  useInfiniteQuery({
    queryKey: ['history', filters],
    queryFn: ({ pageParam = '' }) =>
      client.listClaudeHistory({ pageSize: 50, pageToken: pageParam, ...filters }),
    getNextPageParam: (lastPage) => lastPage.nextPageToken || undefined,
  });
```

3. Flatten pages into a single `allEntries` array for the virtualizer.
4. Replace the `<div>` loop in `HistoryGroupView` with a `useVirtualizer` measured-height list. Key settings:

```ts
const virtualizer = useVirtualizer({
  count: allEntries.length + (hasNextPage ? 1 : 0),  // +1 for sentinel loader row
  getScrollElement: () => entryListRef.current,
  estimateSize: () => 72,   // px; measured height of a collapsed card
  overscan: 5,
});
```

5. Add an intersection-observer sentinel at the bottom to call `fetchNextPage`.

**Grouping compatibility**: `useHistoryGrouping` currently takes `filteredEntries[]` and returns `groupedEntries`. With infinite query, pass `allEntries` (the flattened pages array) to the same hook — no hook changes needed.

**State preservation**: `selectedEntry` index logic moves from array index to `entry.id` key to survive page-boundary refetches.

### (b) Inline expandable message preview on cards

**Pattern**: Each history card gets an expand toggle. The card renders a `<details>` element (native HTML, zero JS, keyboard-accessible by default) or a controlled `div` with `aria-expanded` / `aria-controls`.

Recommended structure:

```tsx
<article
  role="option"
  aria-selected={isSelected}
  aria-expanded={isExpanded}
  onClick={handleSelect}
>
  <HistoryCardSummary entry={entry} />
  {isExpanded && (
    <HistoryCardPreview
      entryId={entry.id}
      // Reuse existing getClaudeHistoryMessages({ id, limit: 3 })
    />
  )}
</article>
```

- Use `aria-expanded` on the card element itself (not the button) so screen readers announce "expanded/collapsed" when navigating the list.
- A separate "expand" icon button with `aria-label="Expand preview"` and `aria-controls={previewId}` provides a click target without hijacking card-level selection.
- Keyboard: `Space` on the expand button toggles; `Enter` on the card (already used) selects for detail panel; these do not conflict.
- Lazy-load preview content via the existing `getClaudeHistoryMessages({ id, limit: 3 })` call — only fire when the card first expands. Cache with a simple `Map<id, messages>` in a `useRef` or via `useQuery` with `enabled: isExpanded`.

**Minimal change surface**: Add `isExpanded` state to `HistoryGroupView` (a `Set<string>` of expanded IDs), pass a toggle callback down to each card. `HistoryDetailPanel` is unchanged — it continues to show full detail on selection.

### (c) Fork action modal

**Placement**: A "Fork" button on `HistoryDetailPanel` (next to "Resume") opens a `<dialog>` modal.

**Modal state** (local to `HistoryDetailPanel` or lifted to page):

```ts
type ForkModalState = {
  isOpen: boolean;
  entry: ClaudeHistoryEntry | null;
  forkAtMessage: number;       // slider 1..entry.messageCount
  targetPath: string;          // pre-filled from entry.project
  targetBranch: string;        // auto-suggested: `fork/${entry.id.slice(0,8)}`
  sessionType: SessionType;
};
```

**Form fields**:
1. "Fork at message" — range slider `1..messageCount` with label showing "Keep first N messages".
2. "Target path" — text input, pre-filled with `entry.project`.
3. "Branch name" — text input, auto-suggested.
4. "Session type" radio — Directory / New worktree / Existing worktree.

**Submission** calls `client.createSession({ forkSourceId: entry.id, forkAtMessage, path, branch, sessionType })` — the two new proto fields from section 3.

**Accessibility**: Use `<dialog>` with `role="dialog"` and `aria-labelledby`. Trap focus with `inert` attribute on the rest of the page or a focus-trap library. Return focus to the "Fork" button on close.

**Minimal change surface**: New `ForkModal.tsx` + `ForkModal.css.ts` (vanilla-extract per ADR-009). No changes to existing components except adding the "Fork" button to `HistoryDetailPanel`.

---

## Summary of Minimal Change Surface

| Area | Files changed | Net new files |
|---|---|---|
| Proto | `proto/session/v1/session.proto` (add 5 fields to `ClaudeHistoryEntry`, add `page_token`/`next_page_token` to List messages, add `fork_source_id`/`fork_at_message` to `CreateSessionRequest`) | 0 |
| Backend | `session/history.go` (`ClaudeHistoryEntry` struct), `server/services/search_service.go` (cursor logic), `server/services/session_service.go` (fork dispatch) | 0 |
| Frontend | `web-app/src/app/history/page.tsx`, `web-app/src/components/history/HistoryGroupView.tsx` | `ForkModal.tsx`, `ForkModal.css.ts`, `HistoryCardPreview.tsx` |

---

## Key Tradeoffs

**Cursor vs offset pagination**: Cursor is stable under concurrent writes to the history JSONL but requires the server to maintain sort order. Since history is already sorted by `UpdatedAt` in `Reload()`, this is free. Offset pagination is simpler to implement but produces duplicate/skipped entries when new conversations are written between page fetches.

**go-git vs exec.Command for git enrichment**: The codebase already mixes both (go-git for branch resolution, `exec.Command` for `worktree list`). For `branch` and `last_commit_message`, go-git is preferred (no subprocess, no shell injection risk, already in `go.mod`). For `git_status_summary`, `Worktree().Status()` in go-git is known to be slow on large repos — fall back to `exec.Command("git", "status", "--short")` with a 2s timeout if go-git status takes >200ms.

**TanStack Query adoption**: Introducing `@tanstack/react-query` is a meaningful dependency addition. If the team wants to keep the dependency surface minimal, the infinite-scroll pagination can be implemented with plain `useReducer` + a custom `usePaginatedHistory` hook that manages `pageToken` state directly, without the full Query library. The virtualizer (`@tanstack/react-virtual`) is standalone and does not require Query.

**Fork modal complexity**: Exposing the `fork_at_message` slider requires `entry.messageCount` to be accurate. The current `parseConversationFile` caps scanning at 1000 lines (`maxLinesToScan = 1000`), which means `MessageCount` underestimates long conversations. The fork modal should note "up to N messages" or fetch the real count via `getClaudeHistoryMessages({ id, limit: 0 }).totalCount` before opening.
