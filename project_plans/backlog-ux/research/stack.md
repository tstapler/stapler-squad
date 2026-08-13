# Stack Research: backlog-ux

## 1. Vanilla-extract Badge / Count Indicator Patterns

### Token vocabulary (theme-contract.css.ts + theme.css.ts)

All new CSS must import from `@/styles/theme.css` which re-exports `vars` from
`theme-contract.css.ts`.

**Relevant token paths for badges:**

| Token | Typical use |
|---|---|
| `vars.color.primary` / `vars.color.primaryText` | Active/brand badge fill |
| `vars.color.accentBg` | Subtle primary tint (rgba 0.08) — used for selected rows |
| `vars.statusBadge.inputBg/Fg/Border` | Blue chip (maps to "ready" state) |
| `vars.statusBadge.uncommittedBg/Fg/Border` | Amber chip (maps to "in_progress") |
| `vars.statusBadge.approvalBg/Fg/Border` | Red/pink chip (maps to "review") |
| `vars.statusBadge.completeBg/Fg/Border` | Green chip (maps to "done") |
| `vars.statusBadge.processingBg/Fg/Border` | Indigo chip (maps to active/running) |
| `vars.color.surfaceMuted` / `vars.color.textMuted` | Neutral/idea chip |
| `vars.radii.full` | Pill shape (`border-radius: 9999px`) |
| `vars.radii.sm` | Square-ish badge (`4px`) |
| `vars.fontSize.xs` / `vars.fontWeight.semibold` | Standard badge text |
| `vars.space["1"]`–`vars.space["2"]` | Padding inside chips |

### Existing status badge pattern

`web-app/src/app/backlog/backlog.css.ts` already defines `statusBadge`,
`statusReady`, `statusInProgress`, `statusReview`, `statusDone`, etc. with the
exact token pattern to follow. For nav count badges (small numeric circle), the
same `vars.color.primary` + `vars.radii.full` pattern applies, rendered as a
`<span>` alongside the link text with `position: relative` or `display: inline-flex`.

**Nav badge skeleton (Navigation.css.ts):**
```ts
// add to Navigation.css.ts
export const navLinkWrapper = style({
  position: "relative",
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
});

export const countBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "18px",
  height: "18px",
  padding: `0 ${vars.space["1"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  background: vars.color.primary,
  color: vars.color.primaryText,
  lineHeight: 1,
});
```

No new theme-contract slots are needed for a simple count badge — `vars.color.primary`
and `vars.color.primaryText` are already defined. If a "warning" colour is required
(items needing review), `vars.statusBadge.approvalBg/Fg` can be used directly.

---

## 2. Real-time Update Patterns

### Session updates: WatchSessions (Server-Sent Events / WebSocket)

`web-app/src/lib/hooks/useSessionService.ts` has `watchSessions()` — a
server-streaming ConnectRPC call over `WatchReviewQueue`-style transport
(`createWatchTransport` from `@/lib/transport/watch-ws-transport`). The sessions
slice in Redux is populated from `SessionEvent` push events. All session-list
components read from the Redux store via selectors — they do not poll.

Pattern for real-time session data:
1. `SessionServiceContext` provides the transport + client.
2. `useSessionService` hook calls `client.watchSessions(req)` and dispatches
   `SessionEvent` payloads to `sessionsSlice`.
3. Components consume selectors (`selectSessions`, etc.) from the Redux store.

### Backlog updates: NO streaming RPC exists yet

`BacklogService` (`proto/session/v1/backlog.proto`) has **no `WatchBacklogItems`
RPC**. The existing `useBacklogService.ts` hook is purely request/response —
every call is a one-shot `await client.listBacklogItems(...)`. There is no stream
or push subscription for backlog data in the frontend today.

**Implication for US-3 (session card status badge) and US-4 (nav count badge):**

Two options:
- **Polling** (simplest, no proto change): `setInterval` inside `useBacklogItems`
  calling `listBacklogItems({statuses: ["ready","in_progress","review"]})` every
  N seconds. Acceptable for a count badge; less ideal for per-card status.
- **Add `WatchBacklogItems` RPC** (recommended for US-3/US-4): Add a server-streaming
  RPC to `BacklogService` that pushes `BacklogStatusEvent` payloads on item status
  changes. Would follow the same `WatchSessions` + Redux slice pattern. Requires
  proto change + `make generate-proto`.

For the nav badge (US-4) polling every 30s is likely acceptable. For session card
badges that must update as agents complete work (US-3), a streaming RPC or polling
at ~10s is needed.

### Review Queue pattern (reference implementation)

`web-app/src/lib/hooks/useReviewQueue.ts` is the canonical real-time hook pattern:
- Uses `createWatchTransport` (WebSocket-based ConnectRPC transport).
- Calls `client.watchReviewQueue(req)` as a streaming async iterator.
- Dispatches events to a Redux slice (`reviewQueueSlice`).
- Falls back to polling when WebSocket disconnects.
- `initial_snapshot: true` flag causes the server to emit a full current-state
  event on connection before going live — eliminates the initial-load race.

This is the pattern to follow when adding `WatchBacklogItems`.

---

## 3. Navigation Component Structure

**File:** `web-app/src/components/ui/Navigation.tsx`

Current structure:
- `navItems` array: `[{href, label}]` — conditionally includes Backlog when
  `useFeatureFlag("backlog")` is true.
- Renders `<ul className={menu}>` with `<li>` + `<AppLink>` per item.
- `active` CSS class applied via `pathname === item.href` check.
- `createButton` in a separate `<div className={actions}>` section.

**For US-4 (count badge):** The `navItems` array needs to accept an optional
`badge?: number` field. The JSX inside `navItems.map(...)` can render a `<span>`
badge conditionally when `item.badge && item.badge > 0`. The badge count needs
to come from a hook (e.g. `useBacklogCount()`) that queries active backlog items.

`Navigation.css.ts` currently has no badge styles — they need to be added to
that file following the token pattern above.

The `zIndex` of the nav container itself is `50` (hardcoded inline, not using
the named `zIndex` ladder from `theme-contract.css.ts`). This is a pre-existing
deviation; do not change it as part of this feature.

---

## 4. Omnibar Detector Interface

**File:** `web-app/src/lib/omnibar/detector.ts`

The `Detector` interface requires three fields:
```ts
interface Detector {
  name: string;       // used for registry deduplication & test IDs
  priority: number;   // lower = runs first; first match wins
  detect(input: string): DetectionResult | null;
}
```

`DetectionResult` must include `type`, `confidence`, `parsedValue`, and
`suggestedName`. The `type` field must be a value from the `InputType` enum
(in `./types`).

**For `BacklogCreateDetector` (US-5):**
- Pattern: `backlog: <description>` or `/backlog <description>`
- A new `InputType.BacklogCreate` value must be added to `InputType` enum in
  `web-app/src/lib/omnibar/types.ts`.
- Priority: ~25 (after GitHub URL detectors at 10–30, before NewSession at 35).
  Using 25 ensures `/backlog foo` is not mistaken for a GitHub repo shorthand.
- Register in `createDefaultRegistry()` in `detector.ts`.
- The `detect()` method should strip the `backlog:` or `/backlog ` prefix,
  trim the remainder as the description text, and return it as `parsedValue`.
- Additionally, the `OmnibarAction` union (`web-app/src/lib/omnibar/actions/types.ts`)
  needs a new `create_backlog_item` variant, and `dispatch.ts` needs a matching
  `case "create_backlog_item":` that calls `deps.createBacklogItem(...)`.
- The `createBacklogItem` dep must be threaded through `OmnibarContext.tsx` and
  ultimately call `useBacklogService().createBacklogItem(...)`.

---

## 5. Proto / RPC Patterns for Bulk / Backlog Operations

### Existing per-session deletion

`DeleteSession(DeleteSessionRequest)` → `DeleteSessionResponse` in
`proto/session/v1/session.proto`. Request has `id` (session title) and `force`
bool. No bulk variant exists.

### BacklogService — no bulk delete RPC

`proto/session/v1/backlog.proto` has no `DeleteBacklogItemSessions` or bulk
delete RPC. The requirements (US-2) ask for:
1. Per-item: delete all sessions linked to a single backlog item.
2. Bulk ("Clear completed"): delete sessions linked to all done/archived items.

**Recommended approach:**
- Add `DeleteBacklogItemSessions(DeleteBacklogItemSessionsRequest)` to
  `BacklogService` in `backlog.proto`. Request: `item_id string`. The server
  iterates `item.itemSessions`, calls `DeleteSession` for each (force=true for
  non-running ones), and returns a count.
- Add `ClearCompletedBacklogSessions(ClearCompletedBacklogSessionsRequest)` for
  the bulk path. Server filters items in `done`/`archived` status, deletes their
  sessions, and returns `items_processed int32` + `sessions_deleted int32`.
- Both RPCs must be followed by `make generate-proto`.

### Session ↔ BacklogItem link (ent schema)

The ent ORM schema has a many-to-many edge `session ←→ backlog_item`
(`session/ent/schema/session.go` line 138–139, `item_session.go` edge at line 63).
The `Session` proto message (`types.proto`) does **not** currently expose
`backlog_item_id` as a field — the link is ent-internal. For US-3 (session card
status badge), the session object returned by `ListSessions`/`WatchSessions`
needs either:
- A new `backlog_item_id string` field added to the `Session` proto message, or
- A separate lookup (`GetBacklogItem` by session UUID after the fact).
The first option is strongly preferred for performance and real-time consistency.

---

## Summary of Key Gaps / Decisions Needed

| Gap | Story | Recommended decision |
|---|---|---|
| No `WatchBacklogItems` streaming RPC | US-3, US-4 | Add to `BacklogService` proto (follow `WatchReviewQueue` pattern) OR use polling for US-4 |
| `Session` proto has no `backlog_item_id` field | US-3 | Add `string backlog_item_id = <next field num>` to `Session` message in `types.proto` |
| No bulk delete RPC in BacklogService | US-2 | Add `DeleteBacklogItemSessions` + `ClearCompletedBacklogSessions` RPCs |
| `InputType` enum has no `BacklogCreate` value | US-5 | Add to `types.ts` + register detector at priority 25 |
| Navigation CSS has no badge/count styles | US-4 | Add `countBadge` style to `Navigation.css.ts` using `vars.color.primary` + `vars.radii.full` |
