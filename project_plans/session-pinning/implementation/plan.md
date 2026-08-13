# Implementation Plan: session-pinning

**Feature**: Server-owned pin/unpin toggle on a session, surfaced in a dedicated "Pinned" section at the top of the session list, independent of status/sort/group.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: None — every structural choice below follows an existing in-repo precedent (`hidden` boolean field, `ArchiveSession`/`UnarchiveSession` RPC pair, `SetAutoYes` actor-setter shape) closely enough that none of the choices are hard-to-reverse or novel enough to warrant a standalone ADR. The three decisions requirements/research explicitly deferred are resolved below in **Resolved Product Decisions** instead.

---

## Step 0.5 — Creative Pass: Alternative Approaches

| # | Approach | Strength | Weakness |
|---|---|---|---|
| 1 | **Boolean field on `Session` entity, mirroring `hidden`/`auto_yes`/`is_expanded`** (CHOSEN) | Reuses five already-proven identical code paths (ent → proto → actor-setter → adapter → frontend) end-to-end with zero new abstractions; every layer already has a direct precedent to copy. | No room for a future multi-user/per-viewer pin dimension without a schema change — acceptable since stapler-squad is explicitly single-user/local (build-vs-buy research, requirements' "cross-workspace pin sync" exclusion). |
| 2 | Separate `pinned_sessions` join/junction table (session_id → pinned_by, pinned_at) | Would support future multi-user pin ownership or per-device pin state without touching the `Session` entity. | Pure speculative complexity today — no ownership/ per-viewer dimension exists anywhere else in this schema; violates `.claude/rules/interface-pollution-checklist.md`'s guidance against speculative abstraction for a single-user tool. |
| 3 | Reuse the existing tag system with a magic `"pinned"` tag (no schema/proto change, works via existing `UpdateSessionRequest.tags`) | Zero schema/proto changes — ships today by piggybacking on `tags []string` already round-tripped everywhere. | Overloads a free-text, user-editable, multi-value field as a boolean flag: a typo'd or differently-cased tag ("Pinned" vs "pinned") silently breaks the feature, and conflates two unrelated domain concepts (arbitrary categorization vs. a single boolean state). Also can't carry an archived-auto-unpin invariant cleanly (tag mutation has no single choke point like `setArchivedAtLocked`). |

**Chosen: #1.** It is also the approach requirements.md and every research pass independently converged on.

---

## System Type

Small, additive CRUD-shaped feature on an existing entity (`Session`) — a single boolean flag, a toggle RPC pair, and a list-rendering partition. No new bounded context, no Event-Command-Policy modeling needed.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `pinned` | ent schema field (`field.Bool("pinned").Default(false)`) on the `Session` entity | `session/ent/schema/session.go` |
| `Pinned` (Instance) | In-memory field on `session.Instance`, mirrors `pinned` | `session/instance.go` |
| `Pinned` (InstanceSnapshot) | Read-only copy of `Instance.Pinned` for lock-free reads | `session/instance_snapshot.go` |
| `Pinned` (InstanceData) | Serialization-layer field for JSON snapshot / ent round-trip | `session/storage.go` |
| `pinned` (proto) | `bool pinned = 72;` on the `Session` message | `proto/session/v1/types.proto` |
| `PinSessionRequest`/`PinSessionResponse` | RPC request/response messages, mirror `ArchiveSessionRequest`/`Response` | `proto/session/v1/session.proto` |
| `UnpinSessionRequest`/`UnpinSessionResponse` | RPC request/response messages, mirror `UnarchiveSessionRequest`/`Response` | `proto/session/v1/session.proto` |
| `PinSession`/`UnpinSession` | ConnectRPC handler methods on `SessionService` | `server/services/session_service.go` |
| `setPinnedLocked` | Actor-internal lock-mutate-snapshot helper, mirrors `setAutoYesLocked` | `session/instance_actor_setters.go` |
| `SetPinned(bool)` | Actor-routed public setter on `*Instance` | `session/instance_actor_setters.go` |
| auto-unpin | The invariant that archiving a session always clears `Pinned`, enforced inside `setArchivedAtLocked` | `session/instance_actor_setters.go` |
| `InstanceToProto` | Proto-conversion choke point; gains one line (`protoSession.Pinned = snap.Pinned`) | `server/adapters/instance_adapter.go` |
| `pinSession`/`unpinSession` (hook) | Frontend RPC-calling functions, mirror `archiveSession`/`unarchiveSession` | `web-app/src/lib/hooks/useSessionService.ts` |
| `onTogglePinned` | Prop name threaded through `CockpitActions` → `page.tsx` → `PaneSplitRenderer` → `SessionList` → `SessionCard`/`SessionRow` → `SessionActionsOverflow` | multiple files |
| `handleTogglePinned` | Page-level handler that calls `pinSession`/`unpinSession` based on the target boolean | `web-app/src/app/page.tsx` |
| `session-pin-toggle` | `data-testid` on the menu item, per e2e locator convention | `SessionActionsOverflow.tsx` |
| `pinnedSessions` | Derived array (`sortedSessions.filter(s => s.pinned)`) in `SessionList.tsx` | `SessionList.tsx` |
| `PINNED_GROUP_KEY` | Sentinel constant (`"__pinned__"`) identifying the synthetic Pinned group/section, distinct from any real `groupKey` a `GroupingStrategy` could produce | `SessionList.tsx` |
| Pinned section | The dedicated top-of-list UI region rendering all pinned, non-archived, non-hidden sessions | `SessionList.tsx` |
| `Pin`/`PinOff` | lucide-react icons used for the menu item's decorative glyph | `SessionActionsOverflow.tsx` |
| `menuitemcheckbox` | ARIA role used for the pin toggle menu item (matches the existing autonomous-mode toggle) | `SessionActionsOverflow.tsx` |
| hidden-wins | The (already-free) invariant that a hidden+pinned session never reaches the frontend, because `ListSessions` excludes `Hidden` sessions by default and the frontend never sets `includeHidden` | `server/services/session_service.go:1140`, `web-app/src/gen/session/v1/session_pb.ts` |
| archived-guard | `PinSession` handler's rejection (`CodeFailedPrecondition`) of a pin request against an already-archived session | `server/services/session_service.go` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Schema shape | Plain `field.Bool("pinned").Default(false)` on `Session` | `hidden`/`auto_yes`/`is_expanded` (`session/ent/schema/session.go`) | Join table; magic tag (see Step 0.5) | No ownership/multi-value dimension exists in this schema; matches 5 existing identical-shaped fields |
| RPC shape | Two-verb RPC pair `PinSession`/`UnpinSession` | `ArchiveSession`/`UnarchiveSession` (`session_service.go:4285-4327`) | Single `SetSessionPinned(bool)` RPC | Codebase's own convention for a boolean session flag is two verbs, not one setter+flag (confirmed: `ArchiveSession`/`UnarchiveSession`, not `SetArchived(bool)`) |
| Actor setter shape | Plain lock→mutate→snapshot→unlock→store, no CAS | `SetAutoYes` (`instance_actor_setters.go:317-334`) | `SetArchivedAtIfNil` CAS shape | Pin/unpin is a direct user toggle, not a race-guarded "first writer wins" transition; CAS would add complexity with no corresponding requirement |
| Archived+pinned interaction | Auto-unpin enforced once, inside `setArchivedAtLocked` (single choke point covering every archive path: `ArchiveSession`, `ArchiveSessionByUUID`, `ArchiveWithStop`, workflow bulk-archive, any future sweep) | New for this feature | Ad hoc `SetPinned(false)` call duplicated in each RPC handler that can archive | Root-cause enforcement at the one function every archive path already funnels through, rather than N call-site patches that can drift out of sync |
| Hidden+pinned interaction | No new code — rely on the existing `ListSessions` `IncludeHidden` gate (`session_service.go:1140`), which the frontend never sets | Existing behavior | New explicit filter in the Pinned section render | The invariant is already enforced at the RPC boundary; duplicating it client-side would be redundant and could silently diverge if the gate's semantics ever change |
| Frontend section placement | Prepend a synthetic `GroupedSessions` entry (`groupKey: PINNED_GROUP_KEY`) ahead of `groupSessions()`'s output, over the *non-pinned* remainder | `groupSessions()`'s existing `GroupedSessions[]` shape (`web-app/src/lib/grouping/strategies.ts:36-40`) | New dedicated `PinnedSessionsSection` component with its own virtualizer wiring | Reuses 100% of existing `SessionCard`/`SessionRow`/virtualizer rendering with zero new plumbing; the array-of-groups shape already supports "one section on top, independent of the active sort/group choice" |
| Pinned-section exclusion from normal group | Pinned sessions render **only** in the Pinned section, excluded from `groupSessions()`'s input | Recommended by architecture.md §4 | Duplicate pinned sessions in both the Pinned section and their normal group | Avoids duplicate rendering/virtualizer bookkeeping; matches every industry comparable in ux.md §1 (Slack, VS Code, browser tabs all reposition rather than duplicate) |
| Pinned-section ordering | Reuse the list's existing `sortField`/`sortDir` state for the pinned subset — no new field | build-vs-buy.md §3, features.md §4 | New `pinned_at` timestamp for "most-recently-pinned-first" | Requirements explicitly scope out pin ordering/reordering; adding a timestamp field for an unrequested ordering guarantee would be a speculative field |
| Optimistic UI | `pinSession`/`unpinSession` dispatch `upsertSession` optimistically before the RPC call, roll back to the prior session object on failure | ux.md §4 recommendation; existing `dispatch(upsertSession(...))`/`dispatch(setError(...))` calls already present in `useSessionService.ts` | Pessimistic (spinner, wait for RPC) | Every comparable product (Slack/VS Code/browser tabs/taskbar) treats pin/unpin as instant/local; this repo already dispatches `setError` on RPC failure in `archiveSession`, so adding a rollback dispatch alongside it is a small, consistent extension |
| Pin cap | No cap | features.md §4 | Hard cap (GitHub's 6-pin model) | Not requested; every other boolean flag in this schema (`hidden`, `auto_yes`, `is_expanded`) is uncapped; a cap would be new, unrequested behavior |
| Card-face persistent pin icon | Out of scope for v1 — menu-item toggle only | FR6's "and/or" is satisfied by the menu entry alone | Standalone `aria-pressed` icon button on the card face | The Pinned section's position already communicates pinned state; within that section every card is already known-pinned, so a redundant badge adds no information. Revisit only if user feedback asks for at-a-glance identification outside the Pinned section (moot today since decision above rules out duplicate rendering) |

---

## Resolved Product Decisions

These three items were explicitly deferred to this planning phase by requirements.md/research; each is resolved here so nothing ships half-decided:

1. **Archived + pinned**: **Auto-unpin on archive.** Enforced once, inside `setArchivedAtLocked` (see Pattern Decisions). Additionally, `PinSession` rejects a pin request against an already-archived session with `CodeFailedPrecondition` (the "archived-guard"), so a client can never end up believing a pin succeeded when the backend's own invariant would immediately reverse it.
2. **Hidden + pinned**: **Hidden wins**, at zero implementation cost — `ListSessions` already excludes `Hidden` sessions unless `IncludeHidden` is set, and the frontend never sets it (`grep` confirmed zero call sites setting `includeHidden`). A hidden+pinned session therefore never reaches the frontend's `sessions` prop, so it can never appear in the Pinned section without any new code.
3. **Duplicate rendering**: **Pinned sessions appear only in the Pinned section**, excluded from their normal status/category/etc. group below it. See Pattern Decisions row "Pinned-section exclusion."

Also resolved, smaller scope calls flagged by research (not full deferrals, but worth recording so they aren't silently dropped):
- **No pin count cap** for v1.
- **No bulk pin/unpin action** — not in FR1's stated scope ("pinned and unpinned by the user"), and `BulkActions.tsx` is untouched by this plan.
- **No keyboard shortcut** — no existing precedent for any session-level shortcut to mirror.
- **No omnibar pin affordance** — omnibar registries govern session creation/navigation, not mutating an existing session (confirmed by pitfalls.md §3).
- **No session-detail-header pin toggle** — FR6 says "and/or"; the action menu alone satisfies it, and no `SessionDetail` component with existing mutation affordances was found to extend safely within this plan's scope.

---

## Migration Plan

- **Migration file**: None. This repo has no manual `.sql` migration files — ent's `client.Schema.Create(ctx)` (`session/ent_repository.go:93`) auto-migrates on startup, exactly how `hidden` was added.
- **Reversibility**: Forward migration (`ALTER TABLE sessions ADD COLUMN pinned BOOL DEFAULT 0`, emitted by ent) is additive and non-destructive. There is no automated down-migration (ent doesn't generate one, and this repo doesn't hand-write them per pitfalls.md §6) — a rollback leaves the unused `pinned` column in place, which is harmless (older binary code simply never reads/writes it).
- **Zero-downtime strategy**: Additive nullable-with-default boolean column; SQLite backfills existing rows with the default at the DDL level. No lock, no backfill script, no read/write-path branching required during rollout.
- **Rollback procedure**: Redeploy the previous binary. Do not manually drop the `pinned` column (no migration tooling in this repo supports it safely) — leave it as a harmless unused column until a future forward migration decides to drop it.

## Observability Plan

- **Logs**: No new structured logging required. `PinSession`/`UnpinSession` follow `ArchiveSession`'s existing convention of only logging on the `SaveInstances` internal-error path (`connect.CodeInternal`), which the `connect.NewError` wrapping already surfaces to standard request logging.
- **Metrics**: No new metrics required — no other boolean session flag (`hidden`, `auto_yes`, `is_expanded`) has dedicated metrics.
- **Alerts**: No new alerts required.

## Risk Control

- **Feature flag**: Not gated — matches `hidden`/`archived_at`/`auto_yes`, none of which are flag-gated in this codebase.
- **Rollback procedure**: Revert the PR / redeploy the previous binary; see Migration Plan for the schema-level rollback note.
- **Staged rollout**: Full rollout on merge — stapler-squad is a single-user local tool with no staged-rollout infrastructure (confirmed in build-vs-buy.md §2).

## Unresolved Questions

None. All items research flagged as open ((a) archived+pinned, (b) hidden+pinned, (c) duplicate rendering, plus the smaller scope questions) are resolved above in **Resolved Product Decisions**.

## Dependency Visualization

```
Write path (user clicks "Pin"/"Unpin"):

  SessionActionsOverflow.tsx (menuitemcheckbox, data-testid="session-pin-toggle")
        │ onClick → onTogglePinned(session.id, !session.pinned)
        ▼
  SessionCard.tsx / SessionRow.tsx  (prop passthrough)
        ▼
  SessionList.tsx  (SessionRowHandlers / SessionRowWrapperProps passthrough)
        ▼
  PaneSplitRenderer.tsx  (actions.onTogglePinned)
        ▼
  page.tsx: handleTogglePinned(sessionId, pinned)
        ▼
  useSessionService.ts: pinSession(id) / unpinSession(id)
        │  optimistic dispatch(upsertSession({...prev, pinned})) — rollback on catch
        ▼  ConnectRPC call
  server/services/session_service.go: PinSession / UnpinSession
        │  FindLiveInstance → archived-guard → inst.SetPinned(v)
        ▼
  session/instance_actor_setters.go: SetPinned → setPinnedLocked
        │  s.inst.mu.Lock() → s.inst.Pinned = v → buildSnapshot() → Unlock() → snapshot.Store()
        ▼
  server/services/session_service.go: s.storage.SaveInstances([]*Instance{inst})  (synchronous)
        ▼
  session/ent_repository.go: Update().SetPinned(data.Pinned) → ent write committed
        │
        ▼ (response returns; next ListSessions/WatchSessions push reflects new state)

Read path (ListSessions / WatchSessions):

  session/ent_repository.go (cold start only) → InstanceData.Pinned → Instance.Pinned
  Instance.Pinned → buildSnapshot() → InstanceSnapshot.Pinned  (in-memory source of truth for all live reads)
  InstanceSnapshot.Pinned → server/adapters/instance_adapter.go: InstanceToProto → protoSession.Pinned
  protoSession.Pinned → web-app/src/gen/session/v1/types_pb.ts: Session.pinned
  Session.pinned → SessionList.tsx: pinnedSessions = sortedSessions.filter(s => s.pinned)
                 → prepended PINNED_GROUP_KEY group, rendered above groupSessions(remainder)
```

---

## Phase 1: Backend — Data Model & Persistence

### Epic 1.1: ent Schema

**Goal**: Add the `pinned` column to the `Session` entity and regenerate the ent client.

#### Story 1.1.1: Add `pinned` field and regenerate
**As a** backend developer, **I want** a `pinned` boolean column on `Session`, **so that** pin state is server-persisted per FR2.
**Acceptance Criteria**:
- Pin state is stored server-side (ent-backed).
  - *Given* the ent schema has no `pinned` field, *When* `field.Bool("pinned").Default(false)` is added and the client regenerated, *Then* `session.Session.Pinned` and `SetPinned()`/`SetNillablePinned()` builder methods exist on the generated client.
**Files**: `session/ent/schema/session.go`

##### Task 1.1.1a: Add `pinned` field to the ent schema (~2 min)
- In `session/ent/schema/session.go`, add immediately after the `hidden` field (currently line ~102-104):
  ```go
  field.Bool("pinned").
      Default(false).
      Comment("When true, session is pinned and surfaces in the dedicated Pinned section regardless of status/recency. Cleared automatically when the session is archived."),
  ```
- Files: `session/ent/schema/session.go`

##### Task 1.1.1b: Regenerate the ent client (~3 min)
- Run exactly: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (per `.claude/rules/ent-schema-generation.md` — the `--feature sql/upsert` flag is required even for a plain bool add, or `UpsertRule`-style methods silently disappear).
- Run `go build ./session/ent/...` to confirm the generated package compiles.
- Files: `session/ent/*` (generated, do not hand-edit)

---

### Epic 1.2: Proto Contract

**Goal**: Expose `pinned` on the `Session` message and add the `PinSession`/`UnpinSession` RPC pair.

#### Story 1.2.1: Add `pinned` field to `Session` message
**Acceptance Criteria**:
- `ListSessions` (and single-session reads) return the pin state (FR3).
  - *Given* the `Session` proto message tops out at field 71 (`workspace_key`), *When* `bool pinned = 72;` is added, *Then* `make proto-gen` produces a `Session.pinned` field in both Go and TypeScript bindings with no field-number collision.
**Files**: `proto/session/v1/types.proto`

##### Task 1.2.1a: Add the proto field (~2 min)
- In `proto/session/v1/types.proto`, immediately after `string workspace_key = 71;` (line 239), add:
  ```protobuf
  bool pinned = 72;
  ```
- Files: `proto/session/v1/types.proto`

#### Story 1.2.2: Add `PinSession`/`UnpinSession` RPC pair
**Acceptance Criteria**:
- Pinning/unpinning is exposed via RPC (`PinSession`/`UnpinSession`).
  - *Given* only `ArchiveSession`/`UnarchiveSession` exist as toggle-RPC precedent, *When* `PinSession`/`UnpinSession` RPCs and their request/response messages are added mirroring that pair exactly, *Then* the ConnectRPC service definition compiles and generates client/server stubs for both methods.
**Files**: `proto/session/v1/session.proto`

##### Task 1.2.2a: Add RPC declarations and messages (~4 min)
- In `proto/session/v1/session.proto`, immediately after the `UnarchiveSession` RPC declaration (line 401), add:
  ```protobuf
  // PinSession pins a session so it surfaces in the dedicated Pinned section
  // regardless of status/sort/group. Rejects an already-archived session.
  rpc PinSession(PinSessionRequest) returns (PinSessionResponse) {}

  // UnpinSession clears the pinned flag, restoring the session to its normal
  // grouped/sorted position.
  rpc UnpinSession(UnpinSessionRequest) returns (UnpinSessionResponse) {}
  ```
- Immediately after `message UnarchiveSessionResponse {}` (line 2561), add:
  ```protobuf
  message PinSessionRequest {
    string session_id = 1;
  }
  message PinSessionResponse {}

  message UnpinSessionRequest {
    string session_id = 1;
  }
  message UnpinSessionResponse {}
  ```
- Files: `proto/session/v1/session.proto`

##### Task 1.2.2b: Regenerate bindings (~2 min)
- Run `make proto-gen`. Confirm `session/gen/session/v1/session_pb.go` gains `PinSessionRequest`/`PinSessionResponse`/`UnpinSessionRequest`/`UnpinSessionResponse`, and `web-app/src/gen/session/v1/session_pb.ts` gains matching `*Schema` exports and `SessionService.pinSession`/`unpinSession` client methods.
- Files: `session/gen/session/v1/*.go`, `web-app/src/gen/session/v1/*_pb.ts` (generated)

---

### Epic 1.3: In-Memory State Plumbing

**Goal**: Thread `Pinned` through `Instance`, `InstanceSnapshot`, serialization, and `InstanceData` — the source-of-truth chain architecture.md traced for `Hidden`.

#### Story 1.3.1: `Instance` struct + `InstanceSnapshot` + `buildSnapshot`
**Files**: `session/instance.go`, `session/instance_snapshot.go`

##### Task 1.3.1a: Add `Pinned bool` to `Instance` (~2 min)
- In `session/instance.go`, immediately after the `Hidden bool` field (line 223), add:
  ```go
  // Pinned surfaces the session in the dedicated Pinned section of the
  // session list regardless of status/sort/group. Cleared automatically
  // when the session is archived (see setArchivedAtLocked).
  Pinned bool
  ```
- Files: `session/instance.go`

##### Task 1.3.1b: Add `Pinned` to `InstanceSnapshot` + `buildSnapshot` (~3 min)
- In `session/instance_snapshot.go`, add `Pinned bool` to the `InstanceSnapshot` struct immediately after `Hidden bool` (line 115).
- In the same file's `buildSnapshot()` function, add `Pinned: i.Pinned,` immediately after `Hidden: i.Hidden,` (line 196).
- Files: `session/instance_snapshot.go`

#### Story 1.3.2: Serialization + `InstanceData`
**Files**: `session/instance_serialization.go`, `session/storage.go`

##### Task 1.3.2a: Add `Pinned` to `instance_serialization.go` (~3 min)
- Add `Pinned: snap.Pinned,` immediately after `Hidden: snap.Hidden,` in the snapshot→data direction (line 110-111 area).
- Add `Pinned: data.Pinned,` immediately after `Hidden: data.Hidden,` in the data→instance direction (line 281-282 area).
- Files: `session/instance_serialization.go`

##### Task 1.3.2b: Add `Pinned bool` to `InstanceData` (~2 min)
- In `session/storage.go`, immediately after `Hidden bool \`json:"hidden,omitempty"\`` (line 116-117), add:
  ```go
  // Pinned mirrors Instance.Pinned for JSON snapshot / ent round-trip.
  Pinned bool `json:"pinned,omitempty"`
  ```
- Files: `session/storage.go`

---

### Epic 1.4: Actor-Routed Mutation

**Goal**: Add the `SetPinned` actor setter and enforce the auto-unpin-on-archive invariant at its single choke point.

#### Story 1.4.1: `SetPinned` actor setter
**As a** backend developer, **I want** a `SetPinned(bool)` method on `Instance`, **so that** pin mutations are serialized through the actor like every other field.
**Acceptance Criteria**:
- *Given* `Instance.Pinned` is false, *When* `inst.SetPinned(true)` is called, *Then* `inst.Snapshot().Pinned` returns true and no data race is possible (mutation runs inside `sendSyncErr`).
**Files**: `session/instance_actor_setters.go`

##### Task 1.4.1a: Add `setPinnedLocked` + `SetPinned` (~3 min)
- In `session/instance_actor_setters.go`, add a new section (near `AutoYes`, e.g. immediately after its block, line ~334):
  ```go
  // ---- Pinned --------------------------------------------------------------------

  func setPinnedLocked(s *instanceState, v bool) {
      s.inst.mu.Lock()
      s.inst.Pinned = v
      snap := buildSnapshot(s.inst)
      s.inst.mu.Unlock()
      s.inst.snapshot.Store(snap)
  }

  // SetPinned sets the Pinned flag. Pinned sessions surface in the dedicated
  // Pinned section of the session list regardless of status/sort/group order.
  func (i *Instance) SetPinned(v bool) {
      _ = i.sendSyncErr(func(s *instanceState) error {
          setPinnedLocked(s, v)
          return nil
      })
  }
  ```
- Files: `session/instance_actor_setters.go`

#### Story 1.4.2: Auto-unpin on archive
**As a** user, **I want** an archived session to always be unpinned, **so that** the Pinned section never silently contains a session that's supposed to have disappeared (Resolved Product Decision #1).
**Acceptance Criteria**:
- *Given* a session is both `Pinned=true` and being archived, *When* `ArchiveSession` (or `ArchiveSessionByUUID`, or any future archive path) runs, *Then* `Pinned` becomes false in the same actor command as `ArchivedAt` is set — never as a separate, skippable step.
- *Given* a session is being **unarchived** (`SetArchivedAt(nil)`), *When* the call runs, *Then* `Pinned` is left untouched (unarchiving must not silently re-pin a session the user never asked to re-pin).
**Files**: `session/instance_actor_setters.go`

##### Task 1.4.2a: Clear `Pinned` inside `setArchivedAtLocked` when archiving (~3 min)
- In `session/instance_actor_setters.go`, modify `setArchivedAtLocked` (line 202-208):
  ```go
  func setArchivedAtLocked(s *instanceState, t *time.Time) {
      s.inst.mu.Lock()
      s.inst.ArchivedAt = t
      if t != nil {
          // Archiving auto-unpins: an archived session is never a candidate
          // for the Pinned section. This is the single choke point every
          // archive path (ArchiveSession, ArchiveSessionByUUID,
          // ArchiveWithStop, workflow bulk-archive) funnels through, so pin
          // state can't drift out of sync with archive state.
          s.inst.Pinned = false
      }
      snap := buildSnapshot(s.inst)
      s.inst.mu.Unlock()
      s.inst.snapshot.Store(snap)
  }
  ```
- Files: `session/instance_actor_setters.go`

---

### Epic 1.5: ent Persistence Wiring

**Goal**: Wire `Pinned` through the hand-written ent `Create`/`Update`/read-back helpers — ent's generated builders don't auto-populate from the Go struct (pitfalls.md §6).

#### Story 1.5.1: Create/Update/read-back
**Acceptance Criteria**:
- *Given* a new `Instance` with `Pinned=true` is saved, *When* `ent_repository.go`'s `Create` runs, *Then* the persisted row has `pinned=1`.
- *Given* an existing pinned session, *When* `Update` runs after any other field change, *Then* `pinned` is not silently reset to the SQL default.
**Files**: `session/ent_repository.go`

##### Task 1.5.1a: Wire `Create` (~2 min)
- In `session/ent_repository.go`, add `SetPinned(data.Pinned)` to the unconditional builder chain (line 155-158, alongside `SetAutoYes`/`SetIsExpanded` — unconditional like those two, not the `if data.Hidden {...}` conditional pattern, since `pinned` has no reason to special-case the false/default case):
  ```go
  sessionCreate := tx.Session.Create().
      SetTitle(data.Title).
      SetNillableUUID(nilIfEmpty(data.UUID)).
      SetPath(data.Path).
      SetStatus(int(data.Status)).
      SetCreatedAt(data.CreatedAt).
      SetUpdatedAt(data.UpdatedAt).
      SetAutoYes(data.AutoYes).
      SetAutonomousMode(data.AutonomousMode).
      SetProgram(data.Program).
      SetIsExpanded(data.IsExpanded).
      SetPinned(data.Pinned)
  ```
- Files: `session/ent_repository.go`

##### Task 1.5.1b: Wire `Update` (~2 min)
- In the same file's `Update` function, add `SetPinned(data.Pinned)` to the unconditional builder chain (line 367-370, mirroring the `Create` change above).
- Files: `session/ent_repository.go`

##### Task 1.5.1c: Wire read-back (~2 min)
- Add `Pinned: sess.Pinned,` immediately after `Hidden: sess.Hidden,` (line 1059) in the ent row → `InstanceData` mapping.
- Files: `session/ent_repository.go`

---

### Epic 1.6: Proto Response Wiring

**Goal**: Wire `Pinned` through the single proto-conversion choke point.

#### Story 1.6.1: `InstanceToProto`
**Acceptance Criteria**:
- `ListSessions` (and single-session reads) return the pin state (FR3).
  - *Given* `snap.Pinned` is true, *When* `InstanceToProto` builds the outgoing `Session` proto, *Then* `protoSession.Pinned` is true — for every response path (`ListSessions`, `GetSession`, event-stream/`WatchSessions` payloads), since they all funnel through this one function.
**Files**: `server/adapters/instance_adapter.go`

##### Task 1.6.1a: Add `Pinned` to `InstanceToProto` (~2 min)
- In `server/adapters/instance_adapter.go`, add `protoSession.Pinned = snap.Pinned` immediately after `protoSession.Hidden = snap.Hidden` (line 176).
- Files: `server/adapters/instance_adapter.go`

---

### Epic 1.7: RPC Handlers

**Goal**: Implement `PinSession`/`UnpinSession`, mirroring `ArchiveSession`/`UnarchiveSession` exactly, including the synchronous `SaveInstances` call and the `// +api:` marker for the feature registry scanner.

#### Story 1.7.1: `PinSession`/`UnpinSession` handlers
**As a** user, **I want** to pin/unpin a session via RPC, **so that** the toggle persists server-side before the UI reports success (FR1, FR2, FR5).
**Acceptance Criteria**:
- Sessions can be pinned/unpinned (FR1).
  - *Given* a live, non-archived session `S`, *When* `PinSession({session_id: S})` is called, *Then* the response is `PinSessionResponse{}` with no error, and by the time it returns, `s.storage.SaveInstances` has already committed `pinned=true` to ent (matching `ArchiveSession`'s synchronous-durability guarantee).
- Pin state is server-owned (FR2, FR5).
  - *Given* `PinSession` succeeded, *When* the process restarts and reloads from ent, *Then* `Instance.Pinned` is true (cold-start read-back via `fromInstanceData`).
- Archived-guard (Resolved Decision #1).
  - *Given* session `S` has `ArchivedAt != nil`, *When* `PinSession({session_id: S})` is called, *Then* the RPC returns `connect.CodeFailedPrecondition` and no `SaveInstances` call is made.
- Not-found handling.
  - *Given* `session_id` does not resolve to a live instance, *When* `PinSession`/`UnpinSession` is called, *Then* the RPC returns `connect.CodeNotFound`.
**Files**: `server/services/session_service.go`

##### Task 1.7.1a: Implement `PinSession` (~4 min)
- In `server/services/session_service.go`, immediately after the `UnarchiveSession` handler (after line 4327), add:
  ```go
  // +api: session:pin
  // PinSession pins a session so it surfaces in the dedicated Pinned section
  // of the session list regardless of status/sort/group order. Rejects an
  // already-archived session — archiving always auto-unpins (see
  // setArchivedAtLocked), so pinning one here would settle into a state the
  // backend immediately reverses.
  func (s *SessionService) PinSession(
      ctx context.Context,
      req *connect.Request[sessionv1.PinSessionRequest],
  ) (*connect.Response[sessionv1.PinSessionResponse], error) {
      if req.Msg.SessionId == "" {
          return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
      }
      inst := s.FindLiveInstance(req.Msg.SessionId)
      if inst == nil {
          return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
      }
      if inst.ArchivedAt != nil {
          return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot pin an archived session: %s", req.Msg.SessionId))
      }
      inst.SetPinned(true)
      if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
          return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
      }
      return connect.NewResponse(&sessionv1.PinSessionResponse{}), nil
  }
  ```
- Files: `server/services/session_service.go`

##### Task 1.7.1b: Implement `UnpinSession` (~3 min)
- Immediately after `PinSession`, add:
  ```go
  // +api: session:unpin
  // UnpinSession clears the pinned flag, restoring the session to its normal
  // grouped/sorted position.
  func (s *SessionService) UnpinSession(
      ctx context.Context,
      req *connect.Request[sessionv1.UnpinSessionRequest],
  ) (*connect.Response[sessionv1.UnpinSessionResponse], error) {
      if req.Msg.SessionId == "" {
          return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
      }
      inst := s.FindLiveInstance(req.Msg.SessionId)
      if inst == nil {
          return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
      }
      inst.SetPinned(false)
      if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
          return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
      }
      return connect.NewResponse(&sessionv1.UnpinSessionResponse{}), nil
  }
  ```
- Files: `server/services/session_service.go`

##### Task 1.7.1c: Build sanity check (~2 min)
- Run `go build ./...` to confirm the new handlers, proto types, and all Epic 1.1–1.6 plumbing compile together.
- Files: none (verification only)

---

## Phase 2: Frontend — Hook, Wiring, UI

### Epic 2.1: RPC Hook Layer

**Goal**: Add `pinSession`/`unpinSession` to `useSessionService.ts`, including the optimistic-update + rollback pattern.

#### Story 2.1.1: `pinSession`/`unpinSession` with optimistic update
**Acceptance Criteria**:
- *Given* the user clicks "Pin", *When* `pinSession(id)` is called, *Then* the Redux store's session for `id` shows `pinned: true` immediately (before the RPC resolves), and reverts to `pinned: false` if the RPC throws.
**Files**: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.1.1a: Add `pinSession`/`unpinSession` functions (~5 min)
- In `web-app/src/lib/hooks/useSessionService.ts`, immediately after `unarchiveSession` (after line ~627), add:
  ```ts
  const pinSession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      const previous = sessions.find((s) => s.id === id);
      if (previous) dispatch(upsertSession({ ...previous, pinned: true }));
      try {
        await clientRef.current.pinSession(create(PinSessionRequestSchema, { sessionId: id }));
        return true;
      } catch (err) {
        if (previous) dispatch(upsertSession(previous));
        dispatch(setError(err instanceof Error ? err.message : "Failed to pin session"));
        return false;
      }
    },
    [dispatch, sessions]
  );

  const unpinSession = useCallback(
    async (id: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      const previous = sessions.find((s) => s.id === id);
      if (previous) dispatch(upsertSession({ ...previous, pinned: false }));
      try {
        await clientRef.current.unpinSession(create(UnpinSessionRequestSchema, { sessionId: id }));
        return true;
      } catch (err) {
        if (previous) dispatch(upsertSession(previous));
        dispatch(setError(err instanceof Error ? err.message : "Failed to unpin session"));
        return false;
      }
    },
    [dispatch, sessions]
  );
  ```
- Add `PinSessionRequestSchema, UnpinSessionRequestSchema,` to the existing import block from `@/gen/session/v1/session_pb` (alongside `ArchiveSessionRequestSchema`, line 15-16).
- Files: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.1.1b: Expose in return type + return object (~2 min)
- Add `pinSession: (id: string) => Promise<boolean>;` and `unpinSession: (id: string) => Promise<boolean>;` to the hook's return-type interface, immediately after `unarchiveSession` (line 111).
- Add `pinSession, unpinSession,` to the returned object, immediately after `archiveSession, unarchiveSession,` (line 1108-1109).
- Files: `web-app/src/lib/hooks/useSessionService.ts`

---

### Epic 2.2: Cockpit Actions Plumbing

**Goal**: Thread a `pinned` toggle handler from the hook down to `PaneSplitRenderer`, matching the existing `onSetRateLimitEnabled`/`onToggleAutonomousMode` wiring shape.

#### Story 2.2.1: Context + page handler
**Files**: `web-app/src/lib/contexts/CockpitActionsContext.ts`, `web-app/src/app/page.tsx`

##### Task 2.2.1a: Add `onTogglePinned` to `CockpitActions` (~2 min)
- In `web-app/src/lib/contexts/CockpitActionsContext.ts`, add to the `CockpitActions` interface, immediately after `onToggleAutonomousMode` (line ~22):
  ```ts
  onTogglePinned: (sessionId: string, pinned: boolean) => void;
  ```
- Files: `web-app/src/lib/contexts/CockpitActionsContext.ts`

##### Task 2.2.1b: Add `handleTogglePinned` in `page.tsx` (~3 min)
- In `web-app/src/app/page.tsx`, immediately after `handleToggleAutonomousMode` (line ~287), add:
  ```ts
  const handleTogglePinned = useCallback(async (sessionId: string, pinned: boolean): Promise<void> => {
    track({ name: "session_pinned_updated", category: "user_action" });
    if (pinned) {
      await pinSession(sessionId);
    } else {
      await unpinSession(sessionId);
    }
  }, [pinSession, unpinSession, track]);
  ```
- Add `onTogglePinned: handleTogglePinned,` to the `cockpitActions` `useMemo` object (line ~432, alongside `onToggleAutonomousMode`) and `handleTogglePinned` to its dependency array (line ~441).
- Ensure `pinSession, unpinSession` are destructured from the `useSessionService()` call at the top of `page.tsx` (wherever `archiveSession`/other hook functions are currently destructured, if at all — otherwise destructure alongside `updateSession`).
- Files: `web-app/src/app/page.tsx`

#### Story 2.2.2: `PaneSplitRenderer` passthrough
**Acceptance Criteria**:
- *Given* `onTogglePinned` is provided by `CockpitActionsContext`, *When* `SessionListPaneBody` renders `<SessionList>`, *Then* `onTogglePinned` reaches `SessionList` as a prop, matching how `onSetRateLimitEnabled` already does.
**Files**: `web-app/src/components/pane/PaneSplitRenderer.tsx`

##### Task 2.2.2a: Pass `onTogglePinned` to `<SessionList>` (~2 min)
- In `web-app/src/components/pane/PaneSplitRenderer.tsx`, add `onTogglePinned={actions.onTogglePinned}` immediately after `onToggleAutonomousMode={actions.onToggleAutonomousMode}` (line 186).
- Files: `web-app/src/components/pane/PaneSplitRenderer.tsx`

---

### Epic 2.3: Action Menu Toggle

**Goal**: Add the pin/unpin menu item to `SessionActionsOverflow.tsx` and thread `onTogglePinned` through `SessionCard`, `SessionRow`, and `SessionList`'s prop layers — there is no existing archive/hide menu item to copy structurally (architecture.md §4 gap), so this menu item is new but follows the established `menuitemcheckbox` pattern from the autonomous-mode toggle exactly.

#### Story 2.3.1: `SessionActionsOverflow` menu item
**As a** user, **I want** a "Pin"/"Unpin" entry in the session's ··· menu, **so that** I can toggle pin state (FR1, FR6).
**Acceptance Criteria**:
- Toggling pin is available from the session card context menu (FR6).
  - *Given* the ··· menu is open for an unpinned session, *When* the "Pin" item is clicked, *Then* `onTogglePinned(session.id, true)` is called, the menu closes, and the item shows `role="menuitemcheckbox" aria-checked="true"` with label "Unpin {title}" on next render.
**Files**: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 2.3.1a: Add `onTogglePinned` prop + menu item (~5 min)
- Add `import { MoreHorizontal, Pin, PinOff } from "lucide-react";` (extend the existing `lucide-react` import, line 5).
- Add `onTogglePinned?: (sessionId: string, pinned: boolean) => void;` to `SessionActionsOverflowProps` (alongside `onToggleAutonomousMode`, line ~54) and destructure it in the component body.
- In the "Group 4: Mode toggles" region (immediately after the `onToggleAutonomousMode` block, line ~718), add:
  ```tsx
  {onTogglePinned && (
    <button
      role="menuitemcheckbox"
      aria-checked={session.pinned}
      className={overflowMenuItem}
      data-testid="session-pin-toggle"
      aria-label={session.pinned ? `Unpin ${session.title}` : `Pin ${session.title}`}
      onClick={(e) => {
        e.stopPropagation();
        close();
        onTogglePinned(session.id, !session.pinned);
      }}
    >
      {session.pinned ? <PinOff aria-hidden="true" size={16} /> : <Pin aria-hidden="true" size={16} />}{" "}
      {session.pinned ? "Unpin" : "Pin"}
    </button>
  )}
  ```
- Add the frontend registry marker for this feature to the file's existing marker line (first 10 lines, line 2): change
  `// +feature: session-change-program`
  to
  `// +feature: session-change-program session-pin-toggle`
  (the scanner only reads the *first* `// +feature:` marker per file but supports multiple space-separated IDs on that one line — confirmed in `tools/scanner/frontend/src/component-scanner.ts`).
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

#### Story 2.3.2: `SessionCard` wiring
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.3.2a: Add `onTogglePinned` prop and pass through (~3 min)
- Add `onTogglePinned?: (sessionId: string, pinned: boolean) => void;` to `SessionCardProps` (alongside `onSteerAutonomousSession`, line ~114) and destructure it in `SessionCardInner`.
- Add `onTogglePinned={onTogglePinned}` to the `<SessionActionsOverflow>` render (line ~860-887, alongside `onSteerAutonomousSession`).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

#### Story 2.3.3: `SessionRow` wiring
**Files**: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 2.3.3a: Add `onTogglePinned` prop and pass through (~3 min)
- Add `onTogglePinned?: (sessionId: string, pinned: boolean) => void;` to `SessionRowProps` (alongside `onToggleAutonomousMode`, line ~51) and destructure it (line ~148).
- Add `onTogglePinned={onTogglePinned}` to the `<SessionActionsOverflow>` render (line ~384-401, alongside `onToggleAutonomousMode`).
- Files: `web-app/src/components/sessions/SessionRow.tsx`

#### Story 2.3.4: `SessionList.tsx` prop interface threading
**Acceptance Criteria**:
- *Given* `SessionList` receives `onTogglePinned` as a prop, *When* it renders either a card (card mode) or a row (row mode), *Then* `onTogglePinned` reaches `SessionCard`/`SessionRow` unchanged in both rendering paths.
**Files**: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.3.4a: Thread `onTogglePinned` through `SessionListProps` (~2 min)
- Add `onTogglePinned?: (sessionId: string, pinned: boolean) => void;` to `SessionListProps` immediately after `onToggleAutonomousMode` (line ~76) and destructure it where the other handlers are destructured.
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.3.4b: Thread through `SessionRowHandlers`/`SessionRowWrapperProps` (~3 min)
- Add `onTogglePinned?: (id: string, pinned: boolean) => void;` to the `SessionRowHandlers` interface (line ~152, alongside `onToggleAutonomousMode`).
- Destructure it in `SessionRowWrapper`'s prop list (line ~148) and pass `onTogglePinned={onTogglePinned}` to the `<SessionRow>` it renders (line ~189, alongside `onToggleAutonomousMode`).
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.3.4c: Pass `onTogglePinned` at both card-mode and row-mode render call sites (~3 min)
- Add `onTogglePinned={onTogglePinned}` to the card-mode `<SessionCard>` render (line ~1396-1397, alongside `onToggleAutonomousMode`) and the row-mode `<SessionRowWrapper>`/equivalent render (line ~1579-1580).
- Files: `web-app/src/components/sessions/SessionList.tsx`

---

### Epic 2.4: Pinned Section Rendering

**Goal**: Render a dedicated "Pinned" section above the normal grouped/sorted list (FR4), excluding pinned sessions from their normal group (Resolved Decision #3).

#### Story 2.4.1: Derivation + exclusion
**As a** user, **I want** pinned sessions to always appear at the top, **so that** I don't lose track of them as the list reorders (FR4).
**Acceptance Criteria**:
- Pinned sessions appear in a dedicated section, at the top (FR4).
  - *Given* sessions A (pinned), B, C (unpinned) with `groupingStrategy = Status` and A/B/C are in different status groups, *When* the list renders, *Then* A appears only in a "Pinned" section above every status group, and does not additionally appear inside its own status group.
  - *Given* no sessions are pinned, *When* the list renders, *Then* no "Pinned" section header or container is rendered at all (no empty-state placeholder, per ux.md §4).
**Files**: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.4.1a: Compute `pinnedSessions` and exclude from `groupSessions()` input (~4 min)
- In `web-app/src/components/sessions/SessionList.tsx`, immediately before the `groupedSessions` `useMemo` (line 648), add:
  ```ts
  // Pinned sessions render in a dedicated top-of-list section regardless of
  // status/group/sort — excluded from their normal group below to avoid
  // duplicate rendering (see plan.md Pattern Decisions).
  const pinnedSessions = useMemo(
    () => sortedSessions.filter((s) => s.pinned),
    [sortedSessions]
  );
  const PINNED_GROUP_KEY = "__pinned__";
  ```
- Modify the `groupedSessions` `useMemo` (line 648-650) to run `groupSessions()` only over the non-pinned remainder:
  ```ts
  const groupedSessions = useMemo(() => {
    const rest = sortedSessions.filter((s) => !s.pinned);
    return groupSessions(rest, groupingStrategy);
  }, [sortedSessions, groupingStrategy]);
  ```
- Files: `web-app/src/components/sessions/SessionList.tsx`

#### Story 2.4.2: Pinned section CSS
**Files**: `web-app/src/components/sessions/SessionList.css.ts`

##### Task 2.4.2a: Add pinned-section styles (~4 min)
- In `web-app/src/components/sessions/SessionList.css.ts`, add, following the existing `categoryTitle`/`categoryGroup`/`categoryContent` pattern (line 232-295) and using only `vars` tokens (per `.claude/rules/css-architecture.md` — no hardcoded colors, no `var()` strings):
  ```ts
  export const pinnedSection = style({
    display: "flex",
    flexDirection: "column",
    gap: vars.space["3"],
    marginBottom: vars.space["6"],
  });

  export const pinnedSectionTitle = style({
    margin: 0,
    padding: `${vars.space["2"]} 12px`,
    fontSize: "1rem",
    fontWeight: 600,
    color: vars.color.textPrimary,
    display: "flex",
    alignItems: "center",
    gap: vars.space["2"],
    borderLeft: `4px solid ${vars.color.primary}`,
    borderRadius: vars.radii.sm,
    background: vars.color.surfaceSubtle,
  });

  export const pinnedSectionContent = style({
    display: "flex",
    flexDirection: "column",
    gap: vars.space["3"],
  });
  ```
- Files: `web-app/src/components/sessions/SessionList.css.ts`

#### Story 2.4.3: Render block
**Acceptance Criteria**:
- *Given* `pinnedSessions.length > 0`, *When* the list renders, *Then* a `role="region" aria-label="Pinned sessions"` container appears above the grouped list, reusing `SessionCard`/`SessionRow` for each pinned session with the same `onTogglePinned` wiring as the normal list.
**Files**: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.4.3a: Render the Pinned section (~5 min)
- Add the `// +feature: session-pinned-section` marker to `SessionList.tsx`'s first 10 lines (line 2, currently blank).
- Immediately before the existing grouped-list render block (~line 1421 for card mode / the row-mode equivalent), add a conditional render:
  ```tsx
  {pinnedSessions.length > 0 && (
    <div className={pinnedSection} role="region" aria-label="Pinned sessions">
      <h2 className={pinnedSectionTitle}>Pinned</h2>
      <div className={pinnedSectionContent}>
        {pinnedSessions.map((session) =>
          viewMode === "card" ? (
            <SessionCard key={session.id} session={session} /* ...same handler props as the grouped-list SessionCard render... */ onTogglePinned={onTogglePinned} />
          ) : (
            <SessionRowWrapper key={session.id} session={session} /* ...same handler props... */ onTogglePinned={onTogglePinned} />
          )
        )}
      </div>
    </div>
  )}
  ```
  (Wire every other handler prop — `onSessionClick`, `onDeleteSession`, etc. — identically to how the grouped-list render passes them, so pinned-section cards/rows have full functionality, not just the pin toggle.)
- Import `pinnedSection, pinnedSectionTitle, pinnedSectionContent` from `./SessionList.css`.
- Files: `web-app/src/components/sessions/SessionList.tsx`

---

## Phase 3: Registry, Tests, Ship-Readiness

### Epic 3.1: Feature Registry

**Goal**: Register the new backend RPCs and frontend feature per `.claude/rules/feature-registry.md`; both are scanner-generated from markers already added in Phase 1/2, not hand-written.

#### Story 3.1.1: Generate and commit registry entries
**Acceptance Criteria**:
- `docs/registry/features/` updated (per requirements' AC).
  - *Given* `// +api: session:pin` / `// +api: session:unpin` markers exist in `session_service.go` and RPC declarations exist in `session.proto`, *When* `make registry-generate` runs, *Then* `docs/registry/features/backend/session/pin.json` and `.../unpin.json` are created/updated with `markerFound: true`.
  - *Given* the `// +feature: session-change-program session-pin-toggle` marker exists in `SessionActionsOverflow.tsx` and `// +feature: session-pinned-section` exists in `SessionList.tsx`, *When* `make registry-generate` runs, *Then* corresponding frontend feature JSON files are created/updated.
**Files**: `docs/registry/features/backend/session/pin.json`, `docs/registry/features/backend/session/unpin.json`, `docs/registry/features/frontend/session-pinned-section.json` (all generated)

##### Task 3.1.1a: Run `make registry-generate` and commit (~2 min)
- Run `make registry-generate` after all Phase 1/2 tasks land.
- Verify `git status` shows new/changed files under `docs/registry/features/backend/session/` and `docs/registry/features/frontend/`; stage and note for commit (per `.claude/rules/sdd-planning-artifacts-commit.md` this is generated output tied to the feature, committed alongside the implementation, not a separate planning-artifact commit).
- Files: `docs/registry/features/backend/session/pin.json`, `docs/registry/features/backend/session/unpin.json`, `docs/registry/features/frontend/session-pinned-section.json`

---

### Epic 3.2: Backend Tests

**Goal**: Cover the RPC handlers and the auto-unpin invariant with Go tests, mirroring `session_archive_stop_test.go`'s structure.

#### Story 3.2.1: `session_pin_test.go`
**Acceptance Criteria**:
- Pinning/unpinning ... covered by a backend test (requirements' AC).
  - *Given* a live session, *When* `TestPinSession_SetsPinnedTrue` calls `PinSession`, *Then* `inst.Snapshot().Pinned` is true.
  - *Given* a live session, *When* `TestUnpinSession_SetsPinnedFalse` calls `UnpinSession`, *Then* `inst.Snapshot().Pinned` is false.
  - *Given* an archived session, *When* `TestPinSession_RejectsArchivedSession` calls `PinSession`, *Then* the RPC returns `connect.CodeFailedPrecondition` and `Pinned` remains false.
  - *Given* a pinned session, *When* `TestArchiveSession_ClearsPinned` calls `ArchiveSession`, *Then* `inst.Snapshot().Pinned` is false (regression test for the auto-unpin invariant).
**Files**: `server/services/session_pin_test.go` (new)

##### Task 3.2.1a: Write the test file (~5 min)
- Create `server/services/session_pin_test.go`, following `server/services/session_archive_stop_test.go`'s exact fixture pattern (`setupForkTestFixture`, `addPausedSession`, `connect.NewRequest`):
  ```go
  package services

  import (
      "context"
      "testing"

      "connectrpc.com/connect"
      "github.com/stretchr/testify/assert"
      "github.com/stretchr/testify/require"
      sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
  )

  func TestPinSession_SetsPinnedTrue(t *testing.T) {
      fix := setupForkTestFixture(t)
      defer fix.cleanup()
      addPausedSession(t, fix, "pin-me")

      resp, err := fix.svc.PinSession(context.Background(), connect.NewRequest(&sessionv1.PinSessionRequest{SessionId: "pin-me"}))
      require.NoError(t, err)
      require.NotNil(t, resp)

      inst := fix.poller.FindInstance("pin-me")
      require.NotNil(t, inst)
      assert.True(t, inst.Snapshot().Pinned)
  }

  func TestUnpinSession_SetsPinnedFalse(t *testing.T) {
      fix := setupForkTestFixture(t)
      defer fix.cleanup()
      addPausedSession(t, fix, "unpin-me")

      _, err := fix.svc.PinSession(context.Background(), connect.NewRequest(&sessionv1.PinSessionRequest{SessionId: "unpin-me"}))
      require.NoError(t, err)

      resp, err := fix.svc.UnpinSession(context.Background(), connect.NewRequest(&sessionv1.UnpinSessionRequest{SessionId: "unpin-me"}))
      require.NoError(t, err)
      require.NotNil(t, resp)

      inst := fix.poller.FindInstance("unpin-me")
      require.NotNil(t, inst)
      assert.False(t, inst.Snapshot().Pinned)
  }

  func TestPinSession_RejectsArchivedSession(t *testing.T) {
      fix := setupForkTestFixture(t)
      defer fix.cleanup()
      addPausedSession(t, fix, "archived-target")

      _, err := fix.svc.ArchiveSession(context.Background(), connect.NewRequest(&sessionv1.ArchiveSessionRequest{SessionId: "archived-target"}))
      require.NoError(t, err)

      _, err = fix.svc.PinSession(context.Background(), connect.NewRequest(&sessionv1.PinSessionRequest{SessionId: "archived-target"}))
      require.Error(t, err)
      assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

      inst := fix.poller.FindInstance("archived-target")
      require.NotNil(t, inst)
      assert.False(t, inst.Snapshot().Pinned)
  }

  func TestArchiveSession_ClearsPinned(t *testing.T) {
      fix := setupForkTestFixture(t)
      defer fix.cleanup()
      addPausedSession(t, fix, "pinned-then-archived")

      _, err := fix.svc.PinSession(context.Background(), connect.NewRequest(&sessionv1.PinSessionRequest{SessionId: "pinned-then-archived"}))
      require.NoError(t, err)

      _, err = fix.svc.ArchiveSession(context.Background(), connect.NewRequest(&sessionv1.ArchiveSessionRequest{SessionId: "pinned-then-archived"}))
      require.NoError(t, err)

      inst := fix.poller.FindInstance("pinned-then-archived")
      require.NotNil(t, inst)
      snap := inst.Snapshot()
      assert.False(t, snap.Pinned, "expected archiving to auto-unpin")
      assert.NotNil(t, snap.ArchivedAt)
  }
  ```
- Run `go test ./server/services -run TestPinSession -run TestUnpinSession -run TestArchiveSession_ClearsPinned -v` (adjust invocation per Go's single `-run` flag if needed — run each individually or with a combined regex) to confirm all four pass.
- Files: `server/services/session_pin_test.go`

##### Task 3.2.1b: Mark backend registry entries tested (~2 min)
- Edit `docs/registry/features/backend/session/pin.json` and `.../unpin.json` (generated in Task 3.1.1a): set `"tested": true` and `"testIds"` to `["TestPinSession_SetsPinnedTrue", "TestPinSession_RejectsArchivedSession"]` / `["TestUnpinSession_SetsPinnedFalse"]` respectively.
- Re-run `make registry-generate` to confirm these manual edits are preserved (the scanner reads existing `tested`/`testIds` before overwriting — confirmed in `tools/scanner/backend/cmd/main.go`).
- Files: `docs/registry/features/backend/session/pin.json`, `docs/registry/features/backend/session/unpin.json`

---

### Epic 3.3: Frontend Tests

**Goal**: Cover the menu toggle and the pinned-section rendering with Jest/RTL, per requirements' AC.

#### Story 3.3.1: `SessionActionsOverflow` pin toggle test
**Acceptance Criteria**:
- Frontend pin toggle ... covered by a Jest/RTL test (requirements' AC).
  - *Given* an unpinned session, *When* the menu is opened and "Pin" is clicked, *Then* `onTogglePinned` is called with `(session.id, true)`.
  - *Given* a pinned session, *When* the menu is opened, *Then* the menu item reads "Unpin" with `aria-checked="true"`.
**Files**: `web-app/src/components/sessions/__tests__/SessionActionsOverflow.test.tsx`

##### Task 3.3.1a: Add pin-toggle test cases (~4 min)
- In `web-app/src/components/sessions/__tests__/SessionActionsOverflow.test.tsx`, add a new `describe("pin toggle", ...)` block following the file's existing `renderOverflow`/`openMenu` helper pattern:
  ```tsx
  describe("pin toggle", () => {
    it("calls onTogglePinned(id, true) when Pin is clicked on an unpinned session", () => {
      const onTogglePinned = jest.fn();
      renderOverflow({ session: makeSession({ pinned: false }), onTogglePinned });
      openMenu();
      fireEvent.click(screen.getByRole("menuitemcheckbox", { name: /pin session-1|pin test session/i }));
      expect(onTogglePinned).toHaveBeenCalledWith("session-1", true);
    });

    it("shows Unpin with aria-checked=true for a pinned session", () => {
      renderOverflow({ session: makeSession({ pinned: true }), onTogglePinned: jest.fn() });
      openMenu();
      const item = screen.getByTestId("session-pin-toggle");
      expect(item).toHaveAttribute("aria-checked", "true");
      expect(item).toHaveTextContent(/unpin/i);
    });
  });
  ```
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="SessionActionsOverflow.test"` to verify.
- Files: `web-app/src/components/sessions/__tests__/SessionActionsOverflow.test.tsx`

#### Story 3.3.2: `SessionList` pinned-section rendering test
**Acceptance Criteria**:
- Pinned-section rendering ... covered by a Jest/RTL test.
  - *Given* sessions A (pinned), B, C (unpinned, different groups), *When* `SessionList` renders, *Then* a `role="region" aria-label="Pinned sessions"` container exists containing only A, and A does not also appear in its normal group.
  - *Given* no sessions are pinned, *When* `SessionList` renders, *Then* no element with `aria-label="Pinned sessions"` exists.
**Files**: `web-app/src/components/sessions/__tests__/SessionList.pinned.test.tsx` (new)

##### Task 3.3.2a: Write the pinned-section test file (~5 min)
- Create `web-app/src/components/sessions/__tests__/SessionList.pinned.test.tsx`, mirroring `SessionList.archived.test.tsx`'s mock/setup pattern (heavy dependency mocks, `render(<SessionList sessions={...} />)`):
  ```tsx
  import React from "react";
  import { render, screen } from "@testing-library/react";
  import { SessionList } from "../SessionList";
  // ...same heavy-dependency mocks as SessionList.archived.test.tsx...

  function makeSession(overrides: Partial<Record<string, unknown>> = {}) {
    return { id: "s1", title: "S", tags: [], pinned: false, ...overrides };
  }

  describe("SessionList — Pinned section", () => {
    it("renders a Pinned section containing only pinned sessions, excluded from their normal group", () => {
      const sessions = [
        makeSession({ id: "a", title: "Pinned A", pinned: true, category: "cat-1" }),
        makeSession({ id: "b", title: "B", pinned: false, category: "cat-2" }),
      ];
      render(<SessionList sessions={sessions as never} />);
      const pinnedRegion = screen.getByRole("region", { name: /pinned sessions/i });
      expect(pinnedRegion).toHaveTextContent("Pinned A");
      // The cat-1 group (if rendered at all) must not also contain "Pinned A".
      expect(screen.getAllByText("Pinned A")).toHaveLength(1);
    });

    it("hides the Pinned section entirely when no sessions are pinned", () => {
      const sessions = [makeSession({ id: "b", title: "B", pinned: false })];
      render(<SessionList sessions={sessions as never} />);
      expect(screen.queryByRole("region", { name: /pinned sessions/i })).not.toBeInTheDocument();
    });
  });
  ```
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="SessionList.pinned.test"` to verify.
- Files: `web-app/src/components/sessions/__tests__/SessionList.pinned.test.tsx`

##### Task 3.3.2b: Mark frontend registry entry tested (~2 min)
- Edit the generated `docs/registry/features/frontend/session-pinned-section.json` (and the pin-toggle entry if the scanner splits `session-change-program`/`session-pin-toggle` into separate files): set `"tested": true`, `"testIds"` listing the new describe/test names from Tasks 3.3.1a and 3.3.2a.
- Files: `docs/registry/features/frontend/session-pinned-section.json` (and/or the `session-pin-toggle` equivalent)

---

### Epic 3.4: E2E Test

**Goal**: Cover the end-to-end pin/unpin flow per `.claude/rules/e2e-test-conventions.md`.

#### Story 3.4.1: `tests/e2e/session-pin.spec.ts`
**Acceptance Criteria**:
- `docs/registry/features/` updated ... with an e2e test (requirements' AC).
  - *Given* a running session in the test server, *When* the user opens its ··· menu and clicks "Pin", *Then* the session appears inside the `Pinned sessions` region, and clicking "Unpin" removes it from that region.
**Files**: `tests/e2e/session-pin.spec.ts` (new)

##### Task 3.4.1a: Write the e2e spec (~5 min)
- Create `tests/e2e/session-pin.spec.ts`:
  ```ts
  // @feature session:pin, session:unpin
  import { test, expect } from "@playwright/test";
  // ...import whatever session-creation page helper existing specs use from tests/e2e/pages/...

  test.describe("session pinning", () => {
    test("pins a session and shows it in the Pinned section", async ({ page }) => {
      // ...create a session via existing test helper...
      await page.getByRole("button", { name: /more session actions/i }).first().click();
      await page.getByRole("menuitemcheckbox", { name: /^pin /i }).click();
      await expect(page.getByRole("region", { name: /pinned sessions/i })).toBeVisible();
      await expect(page.getByTestId("session-pin-toggle")).toHaveAttribute("aria-checked", "true");
    });

    test("unpins a session and removes it from the Pinned section", async ({ page }) => {
      // ...pin a session first (reuse steps from the test above or a beforeEach)...
      await page.getByRole("button", { name: /more session actions/i }).first().click();
      await page.getByRole("menuitemcheckbox", { name: /^unpin /i }).click();
      await expect(page.getByTestId("session-pin-toggle")).toHaveAttribute("aria-checked", "false");
    });
  });
  ```
  (No `waitForTimeout` anywhere; locators are `data-testid`/ARIA role only, per convention. Fill in the actual session-creation helper from `tests/e2e/pages/` used by neighboring specs — not fully specified here since the exact helper wasn't traced in this planning pass; the implementer should mirror whatever the nearest existing spec, e.g. one covering the autonomous-mode toggle, uses for session setup.)
- Files: `tests/e2e/session-pin.spec.ts`

##### Task 3.4.1b: Run the e2e spec (~3 min)
- Run `cd tests/e2e && npx playwright test session-pin.spec.ts` and confirm both tests pass against the auto-managed isolated test server.
- Files: none (verification only)

---

## Final Verification

- `make build && make test` — full backend build + test suite, including `session_pin_test.go`.
- `make quick-check` — build + test + lint.
- `cd web-app && npx jest --no-coverage` — full frontend test suite, including the two new files.
- `cd tests/e2e && npx playwright test session-pin.spec.ts` — e2e.
- `make registry-generate && git diff --exit-code docs/registry/features/` — confirms no registry drift (matches `build.yml`'s CI gate, pitfalls.md §4).
