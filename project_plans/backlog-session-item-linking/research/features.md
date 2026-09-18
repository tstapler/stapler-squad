# Research: Feature Landscape — backlog-session-item-linking

## 1. What already exists in this codebase

### `AttachSessionToItem` (RPC, not yet an MCP tool)
`server/services/backlog_service_sync.go:29-133`, proto at
`proto/session/v1/backlog.proto:421-428` (`AttachSessionToItemRequest{item_id, session_uuid}`
→ `AttachSessionToItemResponse{item_session}`). Currently called from exactly one place:
`SpawnSessionFromItem` at session-creation time. Its steps, in order:

1. Validate `item_id`/`session_uuid` non-empty (`InvalidArgument` otherwise).
2. Load item; `NotFound` if missing.
3. **Status gate**: item must be `idea`, `ready`, or `in_progress` — `FailedPrecondition`
   otherwise. This is the closest existing precedent for "reject relink to done/archived
   item," but it was written for the spawn path, not a relink path, so it doesn't
   distinguish "item already has a different live session" from "item is simply not
   attachable yet."
4. Snapshot current AC (`ac_snapshot`) onto the new `ItemSession` row.
5. Load prior sessions for the item (for the context file), *before* creating this
   session's row, specifically so the "prior sessions" list never transiently includes
   the session being attached — a documented ordering constraint the new tool must
   preserve if it reuses this code path.
6. `CreateItemSession` — **always inserts a new row**, never updates/replaces an
   existing one. There is no upsert or "detach old link" step.
7. If the session UUID is found in `LoadInstances()`, synchronously (under
   `s.worktreeMu`) call `session.WriteSlashCommands` + `session.WriteBacklogContextFile`
   into that instance's worktree, capture pre-work HEAD SHA, persist the instance. This
   is exactly the "regenerate slash commands after relink" behavior Goal 4 asks for —
   it already exists, just needs to be reachable from a relink tool, not only from a
   fresh attach.
8. Transition item to `in_progress` if the state machine (`CanTransitionBacklog`)
   permits — best-effort, logged+notified on failure, not a hard error.

**Key finding — no uniqueness constraint on the link.** `ItemSession` (schema:
`session/ent/schema/item_session.go`) has no unique index on `session_uuid` or on
`(session_uuid, backlog_item)`. `session_uuid` is documented as a "loose FK to Session;
not an ent edge." A session can accumulate multiple `ItemSession` rows across multiple
items over its lifetime (e.g. triage → work → review roles are separate rows per
`session_role`). Two lookup functions with different semantics already exist:

- `GetItemSessionBySessionAndItem(sessionUUID, itemID)` — used by every
  `PERMISSION_DENIED` gate in `server/mcp/tools_backlog.go` (lines 304, 376, 505, 665,
  758) to verify "is this session linked to *this specific* item."
- `GetItemSessionBySessionUUID(sessionUUID)` — no item filter, orders by
  `created_at DESC`, returns the single **most recent** link for a session. This is the
  natural primitive for "which item does my branch/session currently belong to"
  (Goal 3), and for a read-only introspection tool (Goal 5).

Because there's no uniqueness constraint, "session already linked to a DIFFERENT item"
is not something the schema prevents — it's purely a design decision for the new tool
to make (see Edge Cases below).

### `SpawnSessionFromItem` (`server/services/backlog_service_triage.go:360+`)
The sibling creation-time path. Notable precedents worth reusing rather than
reinventing:
- **Atomic check-and-set** on the item (line ~384) so only one spawn call per item can
  proceed concurrently — the template for "concurrent relink calls" (see Edge Cases).
- **WIP cap / live-session claim check** via `countLiveBacklogWorkSessions`
  (`server/services/backlog_service_triage.go:896-921`): counts an item as "live" if
  status is `in_progress`, or if status is `review` *and* `hasActiveWorkSession(sessions)`
  is true — i.e. "claimed by a live session" is determined by scanning `ItemSessions`
  for an active work-role session, not by a single boolean flag on the item. A relink
  tool that wants to detect "item already claimed by another live session" should reuse
  this helper or its `hasActiveWorkSession` primitive rather than re-deriving liveness.
- Planning-gate defense-in-depth comment (line 541) explicitly calls out "claimed at
  all" as a class of bug this project has hardened against before (PR #199, F2/F3) —
  i.e. double-claim races on `in_progress` items are a known, previously-fixed failure
  mode in this exact subsystem. Any new relink path should assume reviewers will apply
  the same scrutiny.

### MCP tool patterns (`server/mcp/tools_backlog.go`)
- Error codes are a flat `const` block: `ErrPermissionDenied`, `ErrItemNotFound`,
  `ErrFeatureDisabled` (lines ~55-59). `report_progress`/`request_review`/
  `submit_triage_result`/`report_pr_created` all use `ErrInvalidArgument`/
  `ErrInternalError` too (referenced but not in the excerpt read — grep confirms they
  exist as the same style of string constant elsewhere in the file). No
  `ErrConflict`/`ErrAlreadyLinked`/`ErrFailedPrecondition` constant currently exists in
  this file — one will likely be needed for "relink to a claimed/done item" and
  "relink already-linked-to-same-item" responses, matching the "actionable
  PERMISSION_DENIED" ask in Goal 2 (Requirements: "name cause + fix-it tool").
- `errResult(code, message, hint)` — three-argument shape: machine code, human message,
  and an optional actionable hint string (e.g. `"Set STAPLER_SESSION_UUID in your
  environment before calling this tool."`). This hint field is exactly the mechanism
  Goal 2 wants for "fix-it tool" guidance (e.g. hint could say `"Call
  link_session_to_item with item_id=... to relink."`).
- `sessionUUIDFromContext(ctx)` (line 30) — every tool resolves the caller's session
  UUID from context (backed by `STAPLER_SESSION_UUID` env var per the hint text), not
  from a request argument. The new tool should follow this convention rather than
  accepting `session_uuid` as an argument the caller could spoof/typo, *unless* the
  explicit design intent is to allow linking a *different* session than the caller
  (unlikely given "agent self-service recovery" framing in requirements).
- `submit_triage_result`/`submit_review_verdict`/`report_pr_created` all layer a
  **role check** after the link check (e.g. line 510: `"session role is %q — only
  'review' role may submit verdicts"`). A relink tool sits one level below this: it's
  what *establishes* the row those checks query, so it likely does not need its own
  role gate, but should decide what `session_role` value to assign on relink (the
  existing `AttachSessionToItem` always uses `session.SessionRoleWork` hardcoded —
  worth flagging as a decision point since a relink from a triage or review context
  might want a different role).
- `get_backlog_item` (line 114+) is the closest existing precedent for a **read-only,
  role-aware introspection response**: it looks up
  `GetItemSessionBySessionAndItem(callerUUID, itemID)` to find the caller's `role`, then
  branches its returned guidance text by role (triage/work/review/unlinked). A new
  read-only linkage-introspection tool (Goal 5) could either extend this tool's output
  or be a small new tool that returns `{linked: bool, item_id, role, status,
  session_role}` structured for programmatic use rather than the prose envelope
  `get_backlog_item` returns.
- Tool registration pattern: `registerBacklogTools` at the bottom of the file, each
  tool declared via `mcpgo.NewTool(name, WithDescription(...), WithString(arg,
  Description(...), Required())...)` then `s.AddTool(tool, h.handlerMethod)`. A new
  tool follows this exact shape.

### Slash-command regeneration (`session/backlog_commands.go`)
`WriteSlashCommands(engine PipelineEngine, item *BacklogItemData, worktreePath string)`
bakes `item.ID` into generated files at call time (Requirements Gap 2). The doc comment
explicitly warns: **both real callers (`SpawnSessionFromItem` and
`AttachSessionToItem`) must pass the same shared `s.pipelineEngine` instance** — "passing
two different engines would reintroduce the '2 independent callers can drift'
regression this seam closes." A new relink tool that calls into `AttachSessionToItem`
(or a refactored variant of it) for its regeneration step inherits this constraint for
free; a tool that reimplements regeneration separately would need to thread the same
engine instance through explicitly or risk reintroducing that exact regression.

## 2. Industry patterns — session/task binding in agent-orchestration tools

- **Sourcegraph Cody / GitHub Copilot Workspace** and similar "agent session ↔ ticket"
  integrations generally model the link as a single mutable pointer per session (a
  session works one ticket at a time) rather than an append-only history, and treat
  "relink" as an explicit user/agent action that **replaces** the pointer — but they
  keep an audit trail of prior links separately from the live pointer. This maps onto
  this codebase's actual data model reasonably well: `ItemSession` rows are already an
  append-only audit log (multiple rows per session over time), and "current link" is
  already computed as "most recent row" via `GetItemSessionBySessionUUID`'s
  `ORDER BY created_at DESC` — so the codebase has effectively already chosen the
  "history table + latest-wins" pattern industry tools converge on; the new tool
  doesn't need to invent a new relationship model, just decide the policy for *when*
  a new row is allowed to be appended (i.e. what "supersedes" the old link for
  purposes of the `PERMISSION_DENIED` gates elsewhere, since those gates use
  `GetItemSessionBySessionAndItem`, which is agnostic to "most recent" — see Edge
  Cases below for the resulting ambiguity).
- **Task-runner "claim" semantics** (e.g. Temporal workflow task queues, Sidekiq
  unique jobs, k8s Job leases) generally separate two concerns this feature conflates:
  (a) *can this worker claim this task* (single-owner enforcement, usually via
  optimistic CAS on the task's owner field) vs. (b) *what is this worker currently
  associated with* (a worker-local pointer, freely reassignable without touching the
  task's claim state). This codebase's item **status** (`in_progress`) plus the WIP-cap
  live-session count is the closest analog to (a); the `ItemSession` row is (b). The
  requirements' Gap 1 is squarely about (b) — giving an agent self-service control over
  its own pointer — while explicitly listing "reconciliation redesign" as a Non-Goal,
  i.e. (a)'s claim/ownership semantics are out of scope and should be treated as an
  existing invariant to respect (don't let relink silently steal another session's
  active claim), not something this feature re-implements.

## 3. Edge cases and failure modes to design for

| Case | Existing precedent | Open design question |
|---|---|---|
| Relink session already linked to a **different** item | None — `AttachSessionToItem` has no check for the caller's existing links at all; it just inserts a new row unconditionally | Should the old `ItemSession` row be left as historical (append-only, matches the industry "history + latest wins" pattern) or should linking to item B require a caller to already show as unlinked from A? Given the requirements frame this as "agent self-service recovery" for a *single* working session, silently appending is probably right, but the **response must tell the caller it was already linked to a different item** (see Unstated Needs) so it isn't a silent, surprising switch. |
| Relink to item currently `done`/`archived` | `AttachSessionToItem`'s existing status gate (`idea`/`ready`/`in_progress` only) already rejects this with `FailedPrecondition` | Reuse this gate as-is for the new tool. The `PERMISSION_DENIED` error taxonomy in `tools_backlog.go` doesn't have a `FailedPrecondition`-equivalent MCP error code yet — needs one (or reuse an existing code) so the agent gets an actionable message distinct from "you're just not linked." |
| Relink to item currently claimed by **another live session** | `countLiveBacklogWorkSessions` / `hasActiveWorkSession` in `backlog_service_triage.go` is the only existing liveness check, and it's item-count-level (WIP cap), not "is *this specific* item claimed by session X." `AttachSessionToItem`'s status gate allows attaching to an `in_progress` item with no check for whether a *different, still-live* session already holds it — this looks like a real existing gap, not just one this new tool inherits | Decide: does relinking to an `in_progress` item that already has a live work session silently create a second concurrent claimant (bad — two agents editing the same item), or should the tool detect this via `ListItemSessions` + liveness and reject/warn? This is the single highest-risk edge case since the underlying `AttachSessionToItem` RPC doesn't guard against it today. |
| Relink to item id that doesn't exist | `AttachSessionToItem` already returns `NotFound`/`connect.CodeNotFound` (mapped from `ent.IsNotFound`/`session.ErrNotFound`) | MCP layer needs its own `ErrItemNotFound`-style mapping (the constant already exists: `ErrItemNotFound`) — just wire it. |
| Session UUID not found in session store at all | Every existing `PERMISSION_DENIED` gate handles "no `ItemSession` row for this session+item," but that's different from "this session UUID doesn't exist as a live tmux session in `LoadInstances()` at all." `AttachSessionToItem`'s slash-command regeneration step (step 7 above) already tolerates this: it silently skips the worktree write if the UUID isn't found in `LoadInstances()`, no error | For an MCP tool called *from inside* a session, the caller's own UUID is always "real" by construction (it's the session making the call), but a badly configured `STAPLER_SESSION_UUID` env var could still not match any live instance. Decide whether "not found in `LoadInstances()`" should be a hard error (since slash-command regen — Goal 4 — silently becomes a no-op otherwise) or a soft warning in the response. Given Goal 4 is an explicit deliverable, silently skipping it defeats the point — should surface as at least a warning in the tool's response text. |
| Concurrent relink calls (same session, racing) | `SpawnSessionFromItem`'s atomic check-and-set (line ~384) and `DequeueNextQueuedItems`'s `dequeueMu` are the two existing concurrency-control precedents in this file pair, both added after real races (PR #199 F2). `AttachSessionToItem` itself has no equivalent guard | Two concurrent relink calls for the same session could both pass the status/liveness checks and both insert `ItemSession` rows (to the same or different items) — not obviously harmful given the append-only model and "latest wins" read pattern, but worth stress-testing; at minimum the slash-command write already has a mutex (`s.worktreeMu`) so file-write corruption isn't a new risk, just possible "last write wins" ordering ambiguity on which item's commands end up on disk. |
| Relink to the **same** item it's already linked to (no-op-ish) | No precedent — not handled by `AttachSessionToItem` today (would just insert a duplicate row and rewrite the same slash commands) | This is explicitly called out as an unstated need below — the caller needs to know "already linked" vs. "newly linked" to decide whether to skip other setup work. |

## 4. Unstated needs beyond the explicit requirements

1. **Idempotency signal in the response.** The tool must tell the caller whether the
   call was a no-op (already linked to this exact item) vs. a fresh link vs. a switch
   from a different item — none of `AttachSessionToItem`'s current response fields
   (`ItemSession` only) carry this distinction. An agent deciding "do I need to redo my
   context-loading steps" needs this, not just a bare success.
2. **Post-link item state in the response**, not just the `ItemSession` row — status,
   current AC list/snapshot, and (per `get_backlog_item`'s existing pattern) whether a
   review verdict is already pending — so the calling agent can decide its next action
   in one round-trip instead of immediately having to call `get_backlog_item` again.
   This mirrors what `get_backlog_item` already assembles; the new tool's response
   should either embed a similar payload or explicitly instruct the agent to call
   `get_backlog_item` next (cheaper to implement, but costs a round trip every time).
3. **Explicit confirmation that slash commands were actually regenerated**, not just
   that the link was created — since (per the edge-case table) the regeneration step
   can silently no-op if the calling session isn't found in `LoadInstances()`. Silent
   success-but-not-really is exactly the kind of gap that motivated this whole feature
   (Gap 2 in requirements starts from "this very session's generated commands reference
   a stale item id" — i.e. a prior regeneration silently didn't happen or wasn't
   triggered). The tool's response text should say "slash commands regenerated" or
   explicitly warn "could not regenerate — session not found in instance store" rather
   than being silent either way.
4. **A named, reusable "fix-it" tool reference in error hints** — Goal 2 asks for
   "actionable `PERMISSION_DENIED` errors (name cause + fix-it tool)." The `errResult`
   hint field already exists as the mechanism; the five existing
   `PERMISSION_DENIED: not linked` sites in `report_progress`/`request_review`/
   `submit_review_verdict`/`report_pr_created`/`submit_triage_result` should all be
   updated to reference the new relink tool by name in their hint text once it exists,
   otherwise Gap 1 is only half-closed (the tool exists but nothing points agents to
   it when they hit the original failure).
5. **Read-only introspection needs to answer "which item, if any, am I linked to"
   without requiring the caller to already know an `item_id`** — Goal 3 ("resolve
   which item my branch belongs to") — which only `GetItemSessionBySessionUUID`
   (no-item-arg lookup) supports; `GetItemSessionBySessionAndItem` requires already
   knowing the item id, so it can't serve this goal alone. The introspection tool's
   `item_id` argument should likely be optional: omitted → resolve via
   `GetItemSessionBySessionUUID`; provided → use
   `GetItemSessionBySessionAndItem` for a targeted check.

## Key file references

- `server/services/backlog_service_sync.go:29-133` — `AttachSessionToItem` (RPC to reuse/wrap)
- `server/services/backlog_service_triage.go:360-450, 896-921` — `SpawnSessionFromItem`, `countLiveBacklogWorkSessions`, `hasActiveWorkSession` (liveness/claim precedents)
- `server/mcp/tools_backlog.go:30, 55-59, 114-236, 280-330, 917-1050` — `sessionUUIDFromContext`, error code consts, `get_backlog_item` (role-aware read pattern), `report_progress` (link-check pattern), tool registration
- `session/ent/schema/item_session.go:19-93` — `ItemSession` schema (no uniqueness constraint on session_uuid/item pair)
- `session/storage_backlog.go:185-220` — `GetItemSessionBySessionUUID` vs `GetItemSessionBySessionAndItem`
- `session/backlog_commands.go:17-79` — `WriteSlashCommands`, shared-engine constraint doc comment
- `proto/session/v1/backlog.proto:421-428, 751-752` — `AttachSessionToItemRequest/Response`, RPC declaration
- `session/backlog.go:11-25, 191-192` — `BacklogStatus` constants, `CanTransitionBacklog`
