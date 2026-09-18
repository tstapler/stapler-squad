# Implementation Plan: backlog-session-item-linking

**Feature**: Expose `BacklogService.AttachSessionToItem` as two new agent-callable MCP tools
(`link_session_to_item`, `get_linked_item`), make the 5 existing `PERMISSION_DENIED: not
linked` errors actionable, and stop `WriteSlashCommands` from leaving stale per-criterion
command files behind after a relink.
**Date**: 2026-08-16
**Status**: Ready for implementation
**ADRs**: ADR-001-nil-tolerant-session-attacher-across-mcp-transports.md

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `BacklogItem` | Existing persisted backlog item (id, title, status, acceptance criteria). No changes in this plan. | `session.BacklogItemData` |
| `ItemSession` | Existing DB row linking a session UUID to a backlog item with a role and lifecycle timestamps. | DTO: `session.ItemSessionSummary` (`session/repository.go:285`) |
| `AttachSessionToItem` | Existing `BacklogService` RPC method that creates an `ItemSession` row and (if the session is a live `Instance`) regenerates its worktree scaffolding. Unchanged by this plan except for the new consumer. | `server/services/backlog_service_sync.go:29` |
| `SessionAttacher` | New narrow interface in `server/mcp` scoping `backlogHandlers`' access to `AttachSessionToItem` to exactly the one method it needs — satisfied structurally by `*services.BacklogService`. | New, `server/mcp/tools_backlog.go` |
| `backlogHandlers` | Existing MCP handler struct; gains one new optional field, `attacher SessionAttacher`. | `server/mcp/tools_backlog.go:91` |
| `link_session_to_item` | New agent-facing MCP tool name. Thin-wraps `AttachSessionToItem` with an idempotency short-circuit and an exclusivity precheck. | New tool |
| `get_linked_item` | New agent-facing MCP tool name. Read-only: resolves which item(s) the calling session is linked to. | New tool |
| `activeWorkSessionOwner` | New helper in `server/mcp`: given `[]session.ItemSessionSummary`, the caller's UUID, and a liveness checker, returns the session UUID of a *different* row (`Role == session.SessionRoleWork`) on the item that is either still `EndedAt == nil` **and** liveness-checker-confirmed-alive (or no checker wired), if any. Rows with `EndedAt == nil` but a liveness checker that reports them dead are treated as stale, not a conflict — see pre-mortem.md #2 (P1). | New, mirrors `hasActiveWorkSession` (`server/services/backlog_service_triage.go:926`) without importing across the services→mcp boundary |
| `liveCheck` | New optional `func(sessionUUID string) bool` field on `backlogHandlers`, same nil-tolerant-wiring pattern as `attacher`. Wraps the same liveness primitive the zombie-session reconciler already uses (`newSessionLivenessChecker`, `server/session_liveness_checker.go`) so `activeWorkSessionOwner` doesn't trust a stale `EndedAt == nil` row for a crashed session. Nil-safe: `activeWorkSessionOwner` treats "no checker wired" as "treat `EndedAt == nil` as live" (today's behavior), never a panic. | New, `server/mcp/tools_backlog.go` — addresses pre-mortem.md P1 #2 |
| `actionablePermissionDenied` | New shared helper replacing the five bare `errResult(ErrPermissionDenied, "this session is not linked...", "")` call sites. Looks up the caller's current link via `GetItemSessionBySessionUUID` and names `link_session_to_item` in the remediation hint. | New, `server/mcp/tools_backlog.go` |
| `ErrConflict` | New MCP error-code constant (`"CONFLICT"`) for "item already claimed by a different live work session." | New, alongside `ErrPermissionDenied` etc. (`tools_backlog.go:56-60`) |
| `ErrUnavailable` | New MCP error-code constant (`"UNAVAILABLE"`) for "`link_session_to_item` is not wired on this transport" (stdio fallback with no `attacher`). | New |
| `ErrFailedPrecondition` | New MCP error-code constant (`"FAILED_PRECONDITION"`) translating `AttachSessionToItem`'s item-status guard (`connect.CodeFailedPrecondition`). | New |
| `LinkSessionToItemResult` | New JSON response struct for `link_session_to_item`'s success payload. | New, `server/mcp/types.go` |
| `GetLinkedItemResult` | New JSON response struct for `get_linked_item`'s success payload. | New, `server/mcp/types.go` |
| `pruneStaleSlashCommandFiles` | New function in `session/backlog_commands.go`. Deletes `done-N.md`/`fail-N.md` files present in the command directory whose index is no longer present in the newly generated file set (e.g. after relinking to an item with fewer acceptance criteria). | New |
| `previously_linked_item_id` | Response field (JSON, omitempty) on `LinkSessionToItemResult` surfacing the caller's most-recent prior link (if different from the item just linked), for operator/agent visibility. | New field name |
| `already_linked` | Response field (JSON bool) on `LinkSessionToItemResult` — `true` on the idempotent no-op path (same session, same item, row already exists). | New field name |
| `slash_commands_regenerated` | Response field (JSON bool) on `LinkSessionToItemResult` — `true` only if the caller's session UUID was found in the live instance registry (`h.store`) before the attach call, which is the same condition `AttachSessionToItem` step 6 uses internally to decide whether to write files at all. | New field name |

**Added post-triad-review (UX/PM gap, see Epic 1.5):** `BuildSessionInitialPrompt`
(`session/backlog_context.go:124`) — the prompt builder used for normal (non-triage)
implementation-mode session spawns (`session/backlog_commands.go:181`) — is confirmed by reading
the source to never embed `item.ID` anywhere in its output, unlike the separate triage-mode
prompt builder (`session/backlog_triage.go:46,108`), which already does via
`fmt.Fprintf(&sb, "item_id: %s\n\n", item.ID)`. This is the literal mechanism behind pre-mortem.md
failure #5: a normal session whose `item_sessions` row is later lost has no prompt-native,
MCP-native, or branch-name-derived way to recover its own item id — `get_linked_item` requires a
row that doesn't exist, and the UX reviewer independently confirmed the git branch does not
contain it either (research/build-vs-buy.md).

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| MCP tool ↔ `AttachSessionToItem` reuse | Thin MCP tool wraps the existing RPC via a new narrow `SessionAttacher` interface (PoEAA: Service Layer facade over an existing Service Layer method, GoF: none needed — no new creational/structural problem here) | `.claude/rules/interface-pollution-checklist.md`; `reviewStopper`/`reviewTrigger` precedent (`tools_backlog.go:77-97`) | (B) Duplicate the attach logic directly inside the MCP handler | Violates Goal 1 ("reusing... rather than duplicating") and reintroduces the exact "2 independent callers can drift" regression `WriteSlashCommands`' own doc comment (`session/backlog_commands.go:27-30`) warns about — a second hand-rolled caller of `CreateItemSession`/`WriteSlashCommands`/`WriteBacklogContextFile` could silently diverge from `AttachSessionToItem`'s sequencing again. |
| MCP tool ↔ `AttachSessionToItem` reuse | (same as above) | (same as above) | (C) Add the exclusivity/idempotency checks directly inside `AttachSessionToItem` itself | `AttachSessionToItem` is also called by `SpawnSessionFromItem`, an internal, already-serialized call site (atomic check-and-set at `backlog_service_triage.go:384`) that has no "hijack a live item" exposure — pushing agent-facing safety logic into the shared method would run unnecessary checks on a call site that doesn't need them, and couples an internal caller's behavior to an external-facing caller's safety requirements. |
| Exclusivity check (pitfalls.md §1) | New precheck **in the MCP handler**, before calling `h.attacher.AttachSessionToItem` — reject with `ErrConflict` if a *different* session already holds a live work-role `ItemSession` on the item | — | Silently allowing the attach (status quo) | pitfalls.md §1: any live session, given only an `item_id` (not secret), could otherwise attach itself to an item another session is actively working, causing silent duplicate/conflicting work. Scoped to `Role == SessionRoleWork` only — `AttachSessionToItem` hardcodes `SessionRoleWork` for every attach today, so a same-role collision is the only case this tool can actually create. |
| Exclusivity check race window (pre-mortem.md P2 #1) | Accepted, documented gap for this iteration — the `ListItemSessions` precheck → `AttachSessionToItem` call is check-then-act with no transaction/lock, matching `AttachSessionToItem`'s own pre-existing lack of a transactional guard (pitfalls.md §2). Not closed by a DB constraint or `ent.Tx` in this plan. | pre-mortem.md failure #1 | Wrap the precheck+attach sequence in a single `ent.Tx`, or add a partial unique index on `(item_id)` where `role=work AND ended_at IS NULL` | Deferred rather than fixed: doing this correctly requires either extending `AttachSessionToItem` itself (which Pattern Decision row 2 above already rejected, to avoid coupling an internal, already-serialized caller to an external-facing caller's safety requirements) or a schema change (out of this plan's stated Migration Plan of "N/A"). The race is narrow (two concurrent first-time links to the same item, a rare operational pattern for solo-user-per-item backlog work) and P2 (recoverable — a human or the reconciler can clean up duplicate rows), unlike P1 #2 above which reproduces the exact incident this feature exists to fix. Tracked as an explicit follow-up, not silently dropped. |
| Exclusivity check liveness (pre-mortem.md P1 #2) | Thread the same `func(sessionUUID string) bool` liveness checker the zombie-session reconciler already uses (`newSessionLivenessChecker`, `server/session_liveness_checker.go`) into `backlogHandlers` as an optional `liveCheck` field; `activeWorkSessionOwner` only reports a conflict for an `EndedAt == nil` row if `liveCheck` is nil (checker unavailable — fall back to today's behavior) or reports the owning session alive | pre-mortem.md failure #2 | (a) No change — accept that a session resuming crashed/interrupted work gets `CONFLICT` with no override, exactly reproducing the incident Gap 1 exists to fix; (b) add a "force relink" override flag | (a) directly reproduces requirements.md's motivating incident and was rejected. (b) was already explicitly rejected in the original Non-Goals ("no force relink override") because an unconditional override reintroduces the hijack risk the exclusivity check exists to prevent — the liveness-aware check gets the same practical outcome (crashed sessions don't block resumption) without an escape hatch a live, healthy session could also invoke. |
| Idempotency (pitfalls.md §4) | Precheck via `GetItemSessionBySessionAndItem` before calling `AttachSessionToItem`; short-circuit to a no-op success response if a row already exists for `(session_uuid, item_id)` | — | Making `AttachSessionToItem` itself upsert/dedupe | Would change `SpawnSessionFromItem`'s "always insert a fresh row" semantics, which its own ordering comment (`backlog_service_sync.go:65-67`) explicitly relies on — the "prior sessions" list must never transiently include the row being created. |
| `SessionAttacher` interface location | Defined in `server/mcp` (the consumer package), scoped to the one method `linkSessionToItem` needs | `.claude/rules/interface-pollution-checklist.md` smell #2; `golang-structs-interfaces` skill | Importing `*services.BacklogService` concrete type directly into `backlogHandlers` | Matches the established `reviewStopper ReviewCompletionSignaler` / `reviewTrigger ReviewTrigger` pattern already in this exact struct — consistency, and avoids a wide concrete dependency for one method. |
| Cross-transport wiring (stdio vs. HTTP) | Thread a new `backlogSvc *services.BacklogService` param through `NewCore`/`NewHTTPHandler`/`RunServer`; convert to the narrow `SessionAttacher` interface **inside** `NewCore` (nil-tolerant) | `liveFinder` guard precedent, `server/mcp/server.go:42-45` | Extending `server.BuildCoreDeps`/`main.go`'s `buildMCPDeps` to construct a full `*services.BacklogService` for the stdio fallback | architecture.md: would require pulling in `cfg`/`workflowEngine`/`pipelineEngine`/`pipelineModeRepo` for a rarely-hit degraded-mode path (daemon not up yet) that the codebase already tolerates gracefully elsewhere (nil `reviewStopper`/`reviewTrigger`). See ADR-001. |
| Read-only introspection (Goals 3/5) | New `get_linked_item` tool, Transaction Script shape matching `get_backlog_item`'s precedent (`tools_backlog.go:114`) | build-vs-buy.md | Adding an "am I linked" field onto `get_backlog_item`'s existing response | `get_backlog_item` requires already knowing `item_id` — Goal 3 is specifically "which item, if any, am I linked to" *without* already knowing it. |
| Stale slash-command files (pitfalls.md §3, §6) | Add `pruneStaleSlashCommandFiles`, called from inside `WriteSlashCommands` after content generation, before/alongside the write loop — deletes `done-N.md`/`fail-N.md` files not present in the new set | — | Full atomic-write-set rewrite of `WriteSlashCommands` (temp-dir + swap) | Goal 4 only promises correct *content* after a relink, not crash-atomicity of the write itself — the non-atomicity is pre-existing (affects every spawn today too) and orthogonal to this feature's scope. Explicitly deferred as a follow-up (see Non-Goals cross-reference below). |
| 5 `PERMISSION_DENIED` sites | Shared `actionablePermissionDenied(ctx, callerUUID, wantItemID)` helper, called from all 5 sites | architecture.md (b) | Inlining the hint text at each of the 5 call sites | The 5 sites are byte-identical today (`errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", "")`) — a shared helper prevents the wording/hint from drifting across sites on the next edit, and centralizes the one extra `GetItemSessionBySessionUUID` lookup. |

**Non-Goals cross-reference**: per requirements.md's Non-Goals, this plan does not implement branch-name parsing (superseded by `GetItemSessionBySessionUUID`, already resolved), does not add a "force relink" override for the exclusivity check, and does not fix `WriteSlashCommands`' per-file non-atomicity (temp+rename) — only its stale-file leak. Sessions that never call `link_session_to_item` are not retroactively fixed (pitfalls.md §6) — this is an explicit, accepted gap, not a defect of this plan.

**Follow-up items from triad review round 2** (non-blocking — zero blockers reported either
round — tracked for implementation-time awareness, not required to gate starting Phase 5):
whether a session recreated after a crash/tmux-server-restart (`.claude/rules/tmux-keep-server-on-restart.md`)
re-invokes `BuildSessionInitialPrompt` or resumes without it is unverified — if the latter, Epic
1.5 doesn't reach that specific cold-start path. Confirm during Epic 1.5 implementation by tracing
the tmux-restart reconciliation code path (`session/backlog_lifecycle.go`'s stuck/zombie handling)
rather than assuming. The exclusivity race window (pre-mortem #1, P2, accepted-deferred) means two
sessions racing to link a brand-new item can both silently succeed with no error to either side —
acceptable for this iteration per the Pattern Decisions row above, but worth a follow-up item if
concurrent first-link collisions are observed in practice.

---

## Migration Plan

N/A — no ent schema changes. `proto/session/v1/backlog.proto`'s `AttachSessionToItemRequest`/
`Response` messages and RPC already exist (`backlog.proto:421-428,752`); no `make proto-gen`
step is required. `ItemSession` (`session/ent/schema/item_session.go`) is unchanged — the
deliberate lack of a unique constraint on `(item_id, session_uuid)` is preserved; the
exclusivity check is enforced in application code (the MCP handler), not the schema, per the
Pattern Decisions above.

## Observability Plan
- **Logs**: Follow the existing `[mcp:<tool_name>]` prefix convention (e.g.
  `log.InfoLog.Printf("[mcp:link_session_to_item] session=%s item=%s already_linked=%v ...")`,
  matching `report_pr_created`'s existing log line at `tools_backlog.go:721`). Log the
  exclusivity-rejection path at `InfoLog` (expected, agent-recoverable), and any
  `AttachSessionToItem` `CodeInternal` translation at `WarningLog`.
- **Metrics**: None new — this codebase's MCP layer has no existing metrics emission to extend
  (verified: no metrics/prometheus import in `server/mcp/*.go`); out of scope to introduce one
  for two tool calls.
- **Alerts**: None new — no existing alerting hooks into MCP tool call outcomes; a repeated
  `ErrConflict` rate would be a UX/product signal (agents frequently colliding), not an
  operational alert.

## Risk Control
- **Feature flag**: No new flag — both tools register only inside the existing
  `if storage != nil && (backlogEnabled == nil || backlogEnabled())` gate
  (`server/mcp/server.go:53`) that already governs every other backlog tool, so the existing
  `backlog` feature flag covers this addition for free.
- **Rollback procedure**: Plain code revert — no schema/data changes, no feature-flagged
  partial state to unwind. The two new tools simply stop being registered; the 5
  `PERMISSION_DENIED` sites and `WriteSlashCommands` revert to prior behavior.
- **Staged rollout**: None — single-deployment-target internal tool (per CLAUDE.md's
  architecture section, one systemd user service); no canary/staged-rollout infra exists to
  hook into.

## Unresolved Questions

None. Every item research/pitfalls flagged as blocking (ownership/exclusivity, idempotency,
the stdio/HTTP transport gap, and the stale-file scope call) has an explicit decision recorded
in Pattern Decisions above. The Phase 4 pre-mortem (see `implementation/pre-mortem.md`) surfaced
one P1 gap post-hoc — the exclusivity check could reject the exact "resume crashed work" scenario
Gap 1 exists to fix — resolved via the liveness-aware `activeWorkSessionOwner` decision above and
folded into Epic 1.1 (Task 1.1.1c) and Epic 1.2 (Task 1.2.1a, Story 1.2.1 AC) below.

## Dependency Visualization

```
Phase 1: MCP tool exposure (Gap 1 + Gap 2 wiring)
                                                                                        
  Epic 1.1: Wire SessionAttacher through constructors
   Story 1.1.1 (interface + field) ──▶ Story 1.1.2 (thread through NewCore/HTTP/stdio + tests)
                    │
                    ▼
  Epic 1.2: link_session_to_item tool ──┐
   Story 1.2.1 (handler + prechecks)    │
                    │                   │
                    ▼                   │
   Story 1.2.2 (tests)                  │
                                         ├──▶ (independent of Epic 1.3, Epic 1.4)
  Epic 1.3: get_linked_item tool ───────┤
   Story 1.3.1 (handler) ──▶ 1.3.2 (tests)
                                         │
  Epic 1.4: actionable PERMISSION_DENIED hints (independent — can ship standalone)
   Story 1.4.1 (helper + 5 call sites) ──▶ Story 1.4.2 (tests)

  Epic 1.5: item_id in normal-mode initial prompt (fully independent — different file,
            session/backlog_context.go — no dependency on Epics 1.1-1.4)
   Story 1.5.1 (one-line addition + test)

Phase 2: Slash-command regeneration correctness (depends on nothing in Phase 1;
         Epic 1.2 exercises it end-to-end but does not require code changes here)
  Epic 2.1: prune stale done-N/fail-N files
   Story 2.1.1 (pruneStaleSlashCommandFiles + wiring) ──▶ Story 2.1.2 (tests incl. AC5 file-content read-back)
```

Suggested execution order: Epic 1.1 → Epic 2.1 (small, unblocks nothing but is independent and
quick) → Epic 1.2 → Epic 1.3 → Epic 1.4 (can run any time after Epic 1.1, even in parallel with
1.2/1.3 since it touches only the 5 existing call sites) → Epic 1.5 (fully independent, can run
in parallel with anything — different file, no shared state).

---

## Phase 1: MCP tool exposure for session↔item linking

### Epic 1.1: Wire a nil-tolerant `SessionAttacher` through the MCP server constructors
**Goal**: Give `backlogHandlers` a safe, structurally-typed path to call
`BacklogService.AttachSessionToItem` from both the HTTP transport (always has it) and the
stdio fallback transport (never has it, degrades gracefully) — see ADR-001.

#### Story 1.1.1: Define `SessionAttacher` and add it to `backlogHandlers`
**As an** MCP tool handler, **I want** a narrow interface for attaching a session to an item,
**so that** I don't depend on the full `*services.BacklogService` concrete type.
**Acceptance Criteria**:
- `backlogHandlers` compiles with a new optional `attacher SessionAttacher` field that
  defaults to `nil` in every existing test literal (`&backlogHandlers{storage: storage}`).
  - *Given* the existing test file `server/mcp/tools_backlog_test.go` with 20+
    `&backlogHandlers{storage: storage}` literals, *When* `SessionAttacher` and the `attacher`
    field are added to the struct, *Then* `go build ./server/mcp/...` succeeds with zero
    changes to those literals (the field is optional, zero-value `nil`).
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.1.1a: Add the `SessionAttacher` interface and 3 new error-code constants (~3 min)
- In `server/mcp/tools_backlog.go`, near the existing `ReviewCompletionSignaler`/`ReviewTrigger`
  interfaces (`tools_backlog.go:72-87`), add:
  ```go
  // SessionAttacher allows the MCP handler to (re)link a session to a backlog
  // item without depending on the full *services.BacklogService concrete type.
  // Satisfied structurally by *services.BacklogService.
  type SessionAttacher interface {
      AttachSessionToItem(ctx context.Context, req *connect.Request[sessionv1.AttachSessionToItemRequest]) (*connect.Response[sessionv1.AttachSessionToItemResponse], error)
  }
  ```
  Add `"connectrpc.com/connect"` to the import block (not currently imported in this file).
- Near the existing `ErrPermissionDenied`/`ErrItemNotFound`/`ErrFeatureDisabled` block
  (`tools_backlog.go:56-60`), add:
  ```go
  ErrConflict           = "CONFLICT"
  ErrUnavailable        = "UNAVAILABLE"
  ErrFailedPrecondition = "FAILED_PRECONDITION"
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.1b: Add the `attacher` field to `backlogHandlers` (~2 min)
- In the `backlogHandlers` struct (`tools_backlog.go:91-110`), add:
  ```go
  attacher SessionAttacher // optional; nil means link_session_to_item is unavailable on this transport
  ```
  directly below `reviewTrigger ReviewTrigger`.
- Run `go build ./server/mcp/...` to confirm no existing literal needs updating.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.1c: Add the `liveCheck` field to `backlogHandlers` and thread it alongside `attacher` (~4 min)
- Addresses pre-mortem.md P1 #2. In `backlogHandlers` (same location as Task 1.1.1b), add:
  ```go
  liveCheck func(sessionUUID string) bool // optional; nil means treat every EndedAt==nil ItemSession row as live (today's behavior)
  ```
- Thread it through the same constructors touched in Story 1.1.2 (`NewCore`/`NewHTTPHandler`/
  `RunServer`), reusing the existing `newSessionLivenessChecker` construction already available
  wherever those constructors are called (`server/session_liveness_checker.go`) — do not build a
  second liveness primitive. On the stdio fallback path (`main.go`), pass `nil` explicitly, same
  as `attacher`, with a comment noting the fallback: `activeWorkSessionOwner` degrades to
  "EndedAt==nil is live," matching pre-feature behavior exactly, not a new failure mode.
- Files: `server/mcp/tools_backlog.go`, `server/mcp/server.go`, `server/server.go`, `main.go`

#### Story 1.1.2: Thread `backlogSvc *services.BacklogService` through `NewCore`/`NewHTTPHandler`/`RunServer` and their call sites
**As a** server operator, **I want** the HTTP-transport MCP server to wire the real
`BacklogService` and the stdio fallback to safely wire nothing, **so that** `link_session_to_item`
works in the common case and degrades clearly, not silently or with a nil-pointer panic, in the
fallback case.
**Acceptance Criteria**:
- `NewCore` converts a possibly-nil `*services.BacklogService` into a possibly-nil
  `SessionAttacher` using the same nil-interface guard already used for `liveFinder`
  (`server/mcp/server.go:42-45`), never producing a non-nil interface wrapping a nil pointer.
  - *Given* `backlogSvc` passed as a literal Go `nil` `*services.BacklogService`, *When*
    `NewCore` constructs `backlogHandlers{attacher: ...}`, *Then* `h.attacher == nil` evaluates
    `true` (verified by a new unit test asserting `linkSessionToItem` returns `ErrUnavailable`,
    not a panic, when called against a handler built this way).
- `server/server.go`'s HTTP path passes `deps.BacklogService` (real, non-nil in the deployed
  service) through to `NewHTTPHandler`.
  - *Given* `deps.BacklogService` is a non-nil `*services.BacklogService` (the normal deployed
    state, `server/dependencies.go:1200`), *When* the HTTP `/mcp` handler is constructed via
    `servermcp.NewHTTPHandler(...)`, *Then* the resulting `backlogHandlers.attacher` is non-nil
    and a `link_session_to_item` call over that transport succeeds against a valid item.
- `main.go`'s stdio fallback path (`buildMCPDeps` + `mcpserver.RunServer` at `main.go:89-97`)
  passes a literal `nil` for `backlogSvc` — no new dependency construction added to
  `buildMCPDeps`.
  - *Given* the stdio fallback path is taken (HTTP daemon unreachable), *When* an agent calls
    `link_session_to_item` over that stdio connection, *Then* the tool returns
    `{"success":false,"error":{"code":"UNAVAILABLE","message":"...","remediation":"..."}}`
    rather than panicking or hanging.
**Files**: `server/mcp/server.go`, `server/server.go`, `main.go`, `server/mcp/server_integration_test.go`, `server/mcp/feature_flag_test.go`

##### Task 1.1.2a: Add `backlogSvc *services.BacklogService` param to `NewCore` (~4 min)
- In `server/mcp/server.go`, change `NewCore`'s signature to add
  `backlogSvc *services.BacklogService` as a new parameter (after `storage`, before
  `eventBus`, matching the order `backlogHandlers` is constructed at line 54).
- Inside `NewCore`, before the existing `registerBacklogTools` call, add the nil-tolerant
  conversion (mirroring `liveFinder` at lines 42-45):
  ```go
  var attacher SessionAttacher
  if backlogSvc != nil {
      attacher = backlogSvc
  }
  ```
- Update the `registerBacklogTools(s, &backlogHandlers{...})` literal at line 54 to include
  `attacher: attacher`.
- Update the doc comment above `NewCore` to document the new parameter (nil-tolerant, mirrors
  `svc`/`liveFinder`'s existing doc pattern).
- Files: `server/mcp/server.go`

##### Task 1.1.2b: Thread the new param through `NewHTTPHandler` and `RunServer` (~3 min)
- In `server/mcp/server.go`, add `backlogSvc *services.BacklogService` to both
  `NewHTTPHandler` and `RunServer`'s signatures, passing it straight through to their internal
  `NewCore(...)` calls. Update both functions' doc comments (one sentence each, matching the
  existing "storage is optional" style).
- Files: `server/mcp/server.go`

##### Task 1.1.2b.1: Log whether `attacher` ended up nil at HTTP-handler construction time (~2 min, pre-mortem.md P2 #3)
- In `NewHTTPHandler` (`server/mcp/server.go`, right after the `NewCore` call added in Task
  1.1.2b), add one `log.InfoLog.Printf("[mcp] link_session_to_item attacher wired: %v", attacher != nil)`
  line so a misconfigured HTTP-transport wiring bug (attacher unexpectedly nil on the transport
  that should always have it) is visible in startup logs immediately, rather than looking
  identical to the intentional stdio-fallback `UNAVAILABLE` degradation until an agent hits it.
- Files: `server/mcp/server.go`

##### Task 1.1.2c: Update the HTTP call site in `server/server.go` (~2 min)
- At `server/server.go:502`, change:
  ```go
  mcpHTTPHandler := servermcp.NewHTTPHandler(deps.Storage, deps.SessionService, deps.ScrollbackManager, deps.Storage, deps.EventBus, deps.UserPRCache, deps.BacklogEnabledCheck)
  ```
  to insert `deps.BacklogService` as the new parameter, in the position matched to
  `NewHTTPHandler`'s updated signature from Task 1.1.2b.
- Files: `server/server.go`

##### Task 1.1.2d: Update the stdio call site in `main.go` (~2 min)
- At `main.go:97`, change the `mcpserver.RunServer(ctx, store, svc, sbMgr, storage, nil, nil, backlogEnabled)`
  call to pass a literal `nil` for the new `backlogSvc` parameter in the correct position (per
  Task 1.1.2b's signature). Add a one-line comment: `// no BacklogService in the stdio
  fallback path — link_session_to_item degrades to UNAVAILABLE, see ADR-001`.
- Files: `main.go`

##### Task 1.1.2e: Update existing test call sites for the new `NewCore` param (~4 min)
- `server/mcp/server_integration_test.go:30`: update `NewCore(&stubStore{}, svc, nil, storage, nil, nil, nil)` to insert `nil` for `backlogSvc` at the correct position.
- `server/mcp/feature_flag_test.go:143,152,161`: same update for each of the 3 `NewCore(...)` calls.
- Run `go build ./server/mcp/...` and `go vet ./server/mcp/...` to confirm no other call sites were missed.
- Files: `server/mcp/server_integration_test.go`, `server/mcp/feature_flag_test.go`

---

### Epic 1.2: Implement `link_session_to_item`
**Goal**: Give an agent session a callable, safe, idempotent way to (re)link itself to a
backlog item, satisfying Goals 1, 2, 4 and AC1-AC5, AC7.

#### Story 1.2.1: Handler with idempotency short-circuit and exclusivity precheck
**As an** agent session, **I want** to call `link_session_to_item` with an `item_id`, **so
that** I become linked to that item (or learn clearly why I can't) without needing SQLite
access.
**Acceptance Criteria**:
- Calling the tool against a valid item id the session isn't yet linked to succeeds, creates
  an `ItemSession` row (role=work), and a subsequent `report_progress` call against that item
  id no longer returns `PERMISSION_DENIED`.
  - *Given* backlog item `id=b608ab1e-b86e-4130-8879-7328cd363063` in status `in_progress` with
    no `ItemSession` row for session UUID `3224bc15-6025-495b-9dff-219d7d0892b5`, *When* that
    session calls `link_session_to_item(item_id="b608ab1e-...")`, *Then* the response is
    `{"success":true,"already_linked":false,"item_id":"b608ab1e-...","previously_linked_item_id":null,...}`
    and a subsequent `report_progress(item_id="b608ab1e-...", criteria_index=0, status="pass")`
    call from the same session UUID succeeds instead of returning `PERMISSION_DENIED`.
- Calling the tool twice with the same `(session_uuid, item_id)` is a no-op on the second call
  — no duplicate `ItemSession` row, no re-triggered `in_progress` transition notification.
  - *Given* session UUID `S1` already has an `ItemSession` row for item `I1` (from a prior
    `link_session_to_item(item_id="I1")` call), *When* `S1` calls
    `link_session_to_item(item_id="I1")` again, *Then* the response is
    `{"success":true,"already_linked":true,...}` and `AttachSessionToItem` is not called a
    second time (verified via the fake `SessionAttacher`'s call count in the unit test).
- Calling the tool for an item another *live work-role* session already holds is rejected with
  `ErrConflict`, naming the other session.
  - *Given* item `I2` has an existing `ItemSession` row with `session_uuid=S_other`,
    `role="work"`, `ended_at=nil` (still live), *When* session `S_me` (≠ `S_other`) calls
    `link_session_to_item(item_id="I2")`, *Then* the response is
    `{"success":false,"error":{"code":"CONFLICT","message":"item I2 already has a live work session (S_other)",...}}`
    and no new `ItemSession` row is created for `S_me`.
- Calling the tool for an item whose existing live-looking row (`EndedAt == nil`) belongs to a
  session `liveCheck` reports as dead succeeds instead of returning `ErrConflict` (pre-mortem.md
  P1 #2 — resuming crashed/interrupted work must not be blocked by a stale row).
  - *Given* item `I4` has an `ItemSession` row `(session_uuid=S_dead, role=work, ended_at=nil)`
    and `h.liveCheck(S_dead)` returns `false`, *When* session `S_me` (≠ `S_dead`) calls
    `link_session_to_item(item_id="I4")`, *Then* the response is
    `{"success":true,"already_linked":false,...}` — `S_dead`'s stale row is not treated as a
    conflict.
- Calling the tool for an item id that doesn't exist returns `ErrItemNotFound`.
  - *Given* `item_id="00000000-0000-0000-0000-000000000000"` does not exist in the backlog,
    *When* any session calls `link_session_to_item(item_id="00000000-...")`, *Then* the
    response is `{"success":false,"error":{"code":"ITEM_NOT_FOUND",...}}`.
- Calling the tool for an item in a terminal status (`done`, `archived`, `pr_pending`, `review`)
  returns `ErrFailedPrecondition` with the item's actual status named.
  - *Given* item `I3` has `status="done"`, *When* any session calls
    `link_session_to_item(item_id="I3")`, *Then* the response is
    `{"success":false,"error":{"code":"FAILED_PRECONDITION","message":"item must be in \"idea\", \"ready\", or \"in_progress\" status..., got \"done\"",...}}`.
- When `h.attacher == nil` (stdio fallback, no daemon), the tool returns `ErrUnavailable`
  instead of a nil-pointer panic.
  - *Given* a `backlogHandlers` built with `attacher: nil`, *When* `link_session_to_item` is
    called with any valid `item_id`, *Then* the response is
    `{"success":false,"error":{"code":"UNAVAILABLE","message":"link_session_to_item is not available over this transport",...}}`.
**Files**: `server/mcp/tools_backlog.go`, `server/mcp/types.go`

##### Task 1.2.1a: Add `activeWorkSessionOwner` helper (~5 min, was ~3 min — expanded for pre-mortem.md P1 #2)
- In `server/mcp/tools_backlog.go`, add:
  ```go
  // activeWorkSessionOwner returns the session UUID of a different work-role ItemSession on the
  // given item that is still genuinely live, if one exists. A row with EndedAt == nil is only
  // treated as a conflict if liveCheck is nil (no liveness primitive wired — preserves
  // pre-feature behavior) or liveCheck reports that session alive; a row whose owning session
  // liveCheck reports dead is treated as stale, not a conflict, so a session resuming
  // crashed/interrupted work is not blocked by a zombie row (pre-mortem.md #2, P1).
  // Mirrors hasActiveWorkSession's predicate (server/services/backlog_service_triage.go:926)
  // without crossing the services -> mcp package boundary.
  func activeWorkSessionOwner(sessions []session.ItemSessionSummary, callerUUID string, liveCheck func(sessionUUID string) bool) (string, bool) {
      for _, s := range sessions {
          if s.Role != session.SessionRoleWork || s.EndedAt != nil || s.SessionUUID == callerUUID {
              continue
          }
          if liveCheck != nil && !liveCheck(s.SessionUUID) {
              continue // EndedAt not yet updated (crash/kill), but liveness check confirms it's dead — not a conflict
          }
          return s.SessionUUID, true
      }
      return "", false
  }
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.2.1b: Add `LinkSessionToItemResult` response struct (~2 min)
- In `server/mcp/types.go`, add:
  ```go
  // LinkSessionToItemResult is returned by link_session_to_item.
  type LinkSessionToItemResult struct {
      ItemID                   string `json:"item_id"`
      SessionUUID              string `json:"session_uuid"`
      ItemSessionID             string `json:"item_session_id"`
      AlreadyLinked             bool   `json:"already_linked"`
      PreviouslyLinkedItemID    string `json:"previously_linked_item_id,omitempty"`
      SlashCommandsRegenerated  bool   `json:"slash_commands_regenerated"`
      ItemStatus                string `json:"item_status"`
  }
  ```
- Files: `server/mcp/types.go`

##### Task 1.2.1c: Implement `linkSessionToItem` skeleton — arg parsing, feature flag, unavailable, caller UUID (~4 min)
- In `server/mcp/tools_backlog.go`, add a new handler following `reportProgress`'s shape
  (`tools_backlog.go:259-266`):
  ```go
  func (h *backlogHandlers) linkSessionToItem(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
      if r := featureDisabledResult(h.enabledCheck); r != nil {
          return r, nil
      }
      callerUUID, err := callerSessionUUID(ctx)
      if err != nil {
          return errResult(ErrPermissionDenied, err.Error(), "Set STAPLER_SESSION_UUID in your environment."), nil
      }
      if h.attacher == nil {
          return errResult(ErrUnavailable, "link_session_to_item is not available over this transport", "Retry once the Stapler Squad HTTP daemon is reachable — this tool requires the HTTP-connected MCP server."), nil
      }
      args := req.GetArguments()
      itemID, ok := args["item_id"].(string)
      if !ok || itemID == "" {
          return errResult(ErrInvalidArgument, "item_id is required", ""), nil
      }
      if err := validateUUID(itemID); err != nil {
          return errResult(ErrInvalidArgument, err.Error(), ""), nil
      }
      // ... continued in Task 1.2.1d/e/f
  }
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.2.1d: Add idempotency short-circuit + prior-link lookup (~4 min)
- Continue `linkSessionToItem`: before calling the attacher, check for an existing same-item
  link and capture the caller's most-recent prior link (for the response field):
  ```go
  if existing, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID); linkErr == nil {
      // Populate ItemStatus on the no-op path too (not just the fresh-link path) — an agent
      // hitting already_linked=true needs the same "is this item still workable" signal a
      // fresh link gets, without a second round trip (triad UX review finding).
      status := ""
      if item, itemErr := h.storage.GetBacklogItem(ctx, itemID); itemErr == nil {
          status = item.Status
      }
      return okResult(LinkSessionToItemResult{
          ItemID: itemID, SessionUUID: callerUUID, ItemSessionID: existing.ID,
          AlreadyLinked: true, SlashCommandsRegenerated: false, ItemStatus: status,
      }), nil
  }
  var previousItemID string
  if prior, priorErr := h.storage.GetItemSessionBySessionUUID(ctx, callerUUID); priorErr == nil && prior.BacklogItemID != itemID {
      previousItemID = prior.BacklogItemID
  }
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.2.1e: Add exclusivity precheck (~3 min)
- Continue `linkSessionToItem`, after the idempotency check: load all sessions for the item
  and reject on a live work-role collision:
  ```go
  itemSessions, listErr := h.storage.ListItemSessions(ctx, itemID)
  if listErr != nil && !errors.Is(listErr, session.ErrNotFound) {
      return errResult(ErrInternalError, fmt.Sprintf("list item sessions: %v", listErr), ""), nil
  }
  if owner, conflict := activeWorkSessionOwner(itemSessions, callerUUID, h.liveCheck); conflict {
      return errResult(ErrConflict,
          fmt.Sprintf("item %s already has a live work session (%s)", itemID, owner),
          "get_linked_item only reports your own session's linkage, not other sessions' — it cannot resolve this. If you believe this is stale (the other session crashed or was force-restarted), wait for the backlog reconciler to clear it, or escalate to a human rather than retrying — this tool has no force-relink override by design."), nil
  }
  ```
  (Second-round triad UX finding: the original remediation text pointed the agent at
  `get_linked_item`, which is scoped to the caller's own linkage only and cannot report on
  `owner`'s session — a dead end if followed literally. The corrected text above states that
  limitation explicitly and gives an honest exit path instead of a false lead.)
- Files: `server/mcp/tools_backlog.go`

##### Task 1.2.1f: Call the attacher, translate connect errors, build success response (~5 min)
- Continue `linkSessionToItem`: determine `slash_commands_regenerated` by checking whether the
  caller's UUID is a known live instance (reuses `findSessionTitleByUUID`,
  `tools_backlog.go:572`), then call the attacher and translate its `connect` error codes:
  ```go
  _, knownInstance := findSessionTitleByUUID(h.store, callerUUID)
  resp, attachErr := h.attacher.AttachSessionToItem(ctx, connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
      ItemId: itemID, SessionUuid: callerUUID,
  }))
  if attachErr != nil {
      switch connect.CodeOf(attachErr) {
      case connect.CodeNotFound:
          return errResult(ErrItemNotFound, attachErr.Error(), ""), nil
      case connect.CodeFailedPrecondition:
          return errResult(ErrFailedPrecondition, attachErr.Error(), "The item's status doesn't currently allow attaching a session."), nil
      case connect.CodeInvalidArgument:
          return errResult(ErrInvalidArgument, attachErr.Error(), ""), nil
      default:
          return errResult(ErrInternalError, fmt.Sprintf("attach session to item: %v", attachErr), ""), nil
      }
  }
  is := resp.Msg.ItemSession
  log.InfoLog.Printf("[mcp:link_session_to_item] session=%s item=%s already_linked=false slash_commands_regenerated=%v", callerUUID, itemID, knownInstance == nil)
  return okResult(LinkSessionToItemResult{
      ItemID: itemID, SessionUUID: callerUUID, ItemSessionID: is.GetId(),
      AlreadyLinked: false, PreviouslyLinkedItemID: previousItemID,
      SlashCommandsRegenerated: knownInstance == nil, ItemStatus: is.GetItem().GetStatus(),
  }), nil
  ```
  (Verify `sessionv1.ItemSession`'s exact field accessors against the generated proto —
  `is.GetId()`/`is.GetItem()` names must match `session/gen/proto/go/session/v1` output; adjust
  to the real generated getters during implementation if they differ.)
- Files: `server/mcp/tools_backlog.go`

##### Task 1.2.1g: Register `link_session_to_item` in `registerBacklogTools` (~3 min)
- In `server/mcp/tools_backlog.go`'s `registerBacklogTools` (near `report_progress`'s
  registration, `tools_backlog.go:932-954`), add:
  ```go
  s.AddTool(
      mcpgo.NewTool("link_session_to_item",
          mcpgo.WithDescription("Link (or relink) this session to a backlog item as a work session. Call this if a report_progress/request_review/submit_triage_result call fails with PERMISSION_DENIED. Get the item_id from the task/item description you were given at session start, or from get_linked_item if you have a prior link — do NOT infer it from your git branch name, which does not embed the item id in this repo. Rejects with CONFLICT if another live session already holds the item, and with FAILED_PRECONDITION if the item's status doesn't allow attaching (must be idea, ready, or in_progress)."),
          mcpgo.WithString("item_id",
              mcpgo.Description("UUID of the backlog item to link this session to"),
              mcpgo.Required(),
          ),
      ),
      h.linkSessionToItem,
  )
  ```
- Files: `server/mcp/tools_backlog.go`

#### Story 1.2.2: Tests for `link_session_to_item`
**As a** maintainer, **I want** unit test coverage for every branch of `linkSessionToItem`,
**so that** the exclusivity/idempotency/error-translation logic doesn't regress.
**Acceptance Criteria**:
- All 6 Given-When-Then scenarios in Story 1.2.1 have a corresponding passing test.
  - *Given* the test file `server/mcp/tools_backlog_test.go` and a fake `SessionAttacher`
    implementation, *When* `go test ./server/mcp/... -run TestLinkSessionToItem` is run,
    *Then* all subtests pass (new success, idempotent, conflict, item-not-found,
    failed-precondition, unavailable).
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 1.2.2a: Add `fakeSessionAttacher` test double (~3 min)
- In `server/mcp/tools_backlog_test.go`, add a small struct implementing `SessionAttacher`
  that records call count/args and returns a scripted response or `connect.NewError(...)`,
  following the existing test-double style in this file (e.g. how `resolveSessionBranch` is
  overridden in `reportPRCreated` tests).
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.2.2b: Test new-link success (AC2 from requirements.md) (~4 min)
- Seed a real `*session.Storage` (test helper already used elsewhere in this file) with one
  `idea`/`ready`/`in_progress` backlog item and no `ItemSession` row for the caller UUID. Call
  `linkSessionToItem`, assert `already_linked=false`, then call `reportProgress` for the same
  item/session and assert it no longer returns `PERMISSION_DENIED`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.2.2c: Test idempotent same-item relink (~3 min)
- Seed an existing `ItemSession` row for `(callerUUID, itemID)`. Call `linkSessionToItem`
  twice; assert the second call's response has `already_linked=true` and the fake attacher's
  call count is 0 (never reached, short-circuited before attach).
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.2.2d: Test relink-to-item-claimed-by-another-live-session rejection (~4 min)
- Seed item `I2` with an `ItemSession` row `(S_other, I2, role=work, ended_at=nil)`. Call
  `linkSessionToItem(item_id=I2)` as `S_me`; assert `CONFLICT` and that no `ItemSession` row
  was created for `S_me` (query storage directly to confirm).
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.2.2d.1: Test relink succeeds when the conflicting row's owner is liveness-dead (~3 min, pre-mortem.md P1 #2)
- Seed item `I4` with an `ItemSession` row `(S_dead, I4, role=work, ended_at=nil)`. Construct
  `backlogHandlers` with `liveCheck: func(uuid string) bool { return uuid != "S_dead" }`. Call
  `linkSessionToItem(item_id=I4)` as `S_me`; assert success (`already_linked=false`), not
  `CONFLICT`. Also assert the `liveCheck == nil` case (Task 1.2.2d's existing test) still
  returns `CONFLICT` — confirms the nil-checker fallback preserves pre-feature behavior.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.2.2e: Test relink-to-nonexistent-item and failed-precondition (~4 min)
- Two subtests: (1) `item_id` with no matching row → fake attacher returns
  `connect.NewError(connect.CodeNotFound, ...)` → assert `ITEM_NOT_FOUND`. (2) item with
  `status="done"` → fake attacher returns `connect.NewError(connect.CodeFailedPrecondition, ...)`
  → assert `FAILED_PRECONDITION`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.2.2f: Test `attacher == nil` unavailable path (~2 min)
- Construct `&backlogHandlers{storage: storage, attacher: nil}`, call `linkSessionToItem` with
  a valid `item_id`, assert `UNAVAILABLE` and no panic.
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 1.3: Implement `get_linked_item`
**Goal**: Give an agent session a read-only way to discover which item(s) it's linked to,
satisfying Goals 3 and 5.

#### Story 1.3.1: Handler and response shape
**As an** agent session, **I want** to call `get_linked_item` with an optional `item_id`,
**so that** I can find out what I'm linked to without already knowing the item id and without
reading SQLite directly.
**Acceptance Criteria**:
- Calling with no `item_id` returns the caller's most-recent link (if any).
  - *Given* session UUID `S1` has two `ItemSession` rows, the most recent for item `I5`
    (`created_at` later than the other), *When* `S1` calls `get_linked_item()` with no
    arguments, *Then* the response is
    `{"success":true,"linked":true,"item_id":"I5","role":"work",...}`.
- Calling with `item_id` set checks that specific item.
  - *Given* session `S1` is linked to item `I5` but not `I6`, *When* `S1` calls
    `get_linked_item(item_id="I6")`, *Then* the response is
    `{"success":true,"linked":false,"item_id":"I6"}`.
- Calling when the session has no link at all returns `linked=false`, not an error.
  - *Given* session `S2` has never called `link_session_to_item` and has no `ItemSession` row,
    *When* `S2` calls `get_linked_item()`, *Then* the response is
    `{"success":true,"linked":false}` (HTTP-level success, not a `PERMISSION_DENIED` error —
    "not linked" is a valid, expected read result for this tool, unlike for the write tools).
**Files**: `server/mcp/tools_backlog.go`, `server/mcp/types.go`

##### Task 1.3.1a: Add `GetLinkedItemResult` response struct (~2 min)
- In `server/mcp/types.go`, add:
  ```go
  // GetLinkedItemResult is returned by get_linked_item.
  type GetLinkedItemResult struct {
      Linked     bool   `json:"linked"`
      ItemID     string `json:"item_id,omitempty"`
      ItemTitle  string `json:"item_title,omitempty"`
      ItemStatus string `json:"item_status,omitempty"`
      Role       string `json:"role,omitempty"`
      StartedAt  *time.Time `json:"started_at,omitempty"`
  }
  ```
- Files: `server/mcp/types.go`

##### Task 1.3.1b: Implement `getLinkedItem` handler (~5 min)
- In `server/mcp/tools_backlog.go`, add:
  ```go
  func (h *backlogHandlers) getLinkedItem(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
      if r := featureDisabledResult(h.enabledCheck); r != nil {
          return r, nil
      }
      callerUUID, err := callerSessionUUID(ctx)
      if err != nil {
          return errResult(ErrPermissionDenied, err.Error(), "Set STAPLER_SESSION_UUID in your environment."), nil
      }
      args := req.GetArguments()
      itemID, _ := args["item_id"].(string)
      if itemID != "" {
          if err := validateUUID(itemID); err != nil {
              return errResult(ErrInvalidArgument, err.Error(), ""), nil
          }
      }
      var is session.ItemSessionSummary
      var lookupErr error
      if itemID != "" {
          is, lookupErr = h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
      } else {
          is, lookupErr = h.storage.GetItemSessionBySessionUUID(ctx, callerUUID)
      }
      if lookupErr != nil {
          if errors.Is(lookupErr, session.ErrNotFound) {
              return okResult(GetLinkedItemResult{Linked: false, ItemID: itemID}), nil
          }
          return errResult(ErrInternalError, fmt.Sprintf("lookup item session: %v", lookupErr), ""), nil
      }
      item, itemErr := h.storage.GetBacklogItem(ctx, is.BacklogItemID)
      title, status := "", ""
      if itemErr == nil {
          title, status = item.Title, item.Status
      }
      return okResult(GetLinkedItemResult{
          Linked: true, ItemID: is.BacklogItemID, ItemTitle: title, ItemStatus: status,
          Role: is.Role, StartedAt: is.StartedAt,
      }), nil
  }
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.3.1c: Register `get_linked_item` in `registerBacklogTools` (~2 min)
- Add tool registration next to `link_session_to_item`'s:
  ```go
  s.AddTool(
      mcpgo.NewTool("get_linked_item",
          mcpgo.WithDescription("Check which backlog item this session is currently linked to. Omit item_id to get the most recent link; pass item_id to check linkage to that specific item. Read-only — use this before link_session_to_item to confirm you're not already correctly linked, or to discover what item you're working on without SQLite access."),
          mcpgo.WithString("item_id",
              mcpgo.Description("Optional UUID of a specific backlog item to check linkage against. Omit to get the most recent link for this session."),
          ),
      ),
      h.getLinkedItem,
  )
  ```
- Files: `server/mcp/tools_backlog.go`

#### Story 1.3.2: Tests for `get_linked_item`
**As a** maintainer, **I want** test coverage for all three lookup modes, **so that** the
optional-arg branching doesn't regress.
**Acceptance Criteria**:
- All 3 Given-When-Then scenarios in Story 1.3.1 have a corresponding passing test.
  - *Given* `server/mcp/tools_backlog_test.go`, *When*
    `go test ./server/mcp/... -run TestGetLinkedItem` is run, *Then* all subtests pass
    (reverse-lookup found, specific-item found, specific-item not-linked, no-link-at-all).
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 1.3.2a: Test reverse lookup (no `item_id`) and specific-item lookup (~4 min)
- Seed two `ItemSession` rows for one session UUID at different `created_at`; call
  `getLinkedItem()` with no args, assert it returns the more recent one. Call again with
  `item_id` set to the older one's item, assert it returns that one instead.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.3.2b: Test not-linked (both with and without `item_id`) (~3 min)
- Call `getLinkedItem()` for a session with zero `ItemSession` rows → assert
  `{"linked":false}`. Call `getLinkedItem(item_id=<valid other item>)` for a session linked
  only to a *different* item → assert `{"linked":false,"item_id":"<that item>"}`.
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 1.4: Make the 5 `PERMISSION_DENIED: not linked` errors actionable
**Goal**: Satisfy AC4 — name `link_session_to_item` in every "not linked" error's remediation
hint. Independent of Epics 1.2/1.3's tool implementations; can ship even if those are blocked
(the helper's remediation text references the tool by name as a string, with no compile-time
dependency on the tool being registered).

#### Story 1.4.1: `actionablePermissionDenied` helper + 5 call sites
**As an** agent session hitting `PERMISSION_DENIED`, **I want** the error to tell me which
item I'm actually linked to (if any) and which tool fixes it, **so that** I can self-correct
without human intervention.
**Acceptance Criteria**:
- A session linked to a *different* item than the one it called against gets a message naming
  both items.
  - *Given* session `S1` is linked to item `I1` (via a prior `ItemSession` row) and calls
    `report_progress(item_id="I2", ...)`, *When* the link check fails, *Then* the response
    message is `"this session is not linked to item I2 — it is currently linked to item I1"`
    and the remediation names `link_session_to_item` with `item_id=I2` as the fix.
- A session with no link at all gets a simpler message.
  - *Given* session `S2` has zero `ItemSession` rows and calls
    `request_review(item_id="I3", ...)`, *When* the link check fails, *Then* the response
    message is `"this session is not linked to any backlog item"` and the remediation names
    `link_session_to_item` with `item_id=I3`.
- All 5 existing call sites (`report_progress`, `request_review`, `submit_review_verdict`,
  `report_pr_created`, `submit_triage_result`) use the shared helper instead of an inline
  `errResult` call.
  - *Given* the 5 call sites at `tools_backlog.go:304,376,505,665,758` (pre-change line
    numbers), *When* `grep -c 'this session is not linked to the specified backlog item'
    server/mcp/tools_backlog.go` is run post-change, *Then* it returns `0` (the literal string
    only exists inside the new shared helper, not duplicated 5 times).
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.4.1a: Add `actionablePermissionDenied` helper (~4 min)
- In `server/mcp/tools_backlog.go`, near `errResult`/`featureDisabledResult`, add:
  ```go
  // actionablePermissionDenied builds a PERMISSION_DENIED result for a "not linked to this
  // item" failure, naming the caller's actual current link (if any) and link_session_to_item
  // as the fix — see requirements.md AC4 / architecture.md (b).
  func (h *backlogHandlers) actionablePermissionDenied(ctx context.Context, callerUUID, wantItemID string) *mcpgo.CallToolResult {
      linked, err := h.storage.GetItemSessionBySessionUUID(ctx, callerUUID)
      if err == nil && linked.BacklogItemID != "" {
          return errResult(ErrPermissionDenied,
              fmt.Sprintf("this session is not linked to item %s — it is currently linked to item %s", wantItemID, linked.BacklogItemID),
              fmt.Sprintf("Call link_session_to_item with item_id=%s to relink, or use item_id=%s if that's what you meant.", wantItemID, linked.BacklogItemID))
      }
      return errResult(ErrPermissionDenied,
          "this session is not linked to any backlog item",
          fmt.Sprintf("Call link_session_to_item with item_id=%s to link this session before retrying.", wantItemID))
  }
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.4.1b: Replace the `report_progress` and `request_review` call sites (~3 min)
- At `tools_backlog.go:304` (`reportProgress`) and `:376` (`requestReview`), replace
  `return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", "..."), nil`
  with `return h.actionablePermissionDenied(ctx, callerUUID, itemID), nil`.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.4.1c: Replace the `submit_review_verdict` and `report_pr_created` call sites (~3 min)
- At `tools_backlog.go:505` (`submitReviewVerdict`) and `:665` (`reportPRCreated`), same
  replacement as Task 1.4.1b.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.4.1d: Replace the `submit_triage_result` call site (~2 min)
- At `tools_backlog.go:758` (`submitTriageResult`), same replacement.
- Files: `server/mcp/tools_backlog.go`

#### Story 1.4.2: Tests for `actionablePermissionDenied`
**As a** maintainer, **I want** both branches of the helper covered, **so that** the hint
wording doesn't silently regress to the old bare message.
**Acceptance Criteria**:
- Both Given-When-Then scenarios in Story 1.4.1 have a corresponding passing test.
  - *Given* `server/mcp/tools_backlog_test.go`, *When*
    `go test ./server/mcp/... -run TestActionablePermissionDenied` is run, *Then* both
    subtests pass (linked-to-different-item, not-linked-at-all).
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 1.4.2a: Test both branches directly, plus one existing-call-site regression check (~4 min)
- Unit test `actionablePermissionDenied` directly with both storage states. Additionally,
  update one existing `report_progress` "not linked" test (already present in
  `tools_backlog_test.go` per the current 5-site behavior) to assert the new remediation text
  contains `"link_session_to_item"` instead of the old empty/generic string.
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 1.5: Embed `item_id` in the normal-mode session initial prompt
**Goal**: Close pre-mortem.md failure #5 and the triad UX review's matching finding for real,
not just by documentation — a freshly-spawned implementation-mode session must have at least one
reliable, MCP-independent channel to learn its own `item_id`, so that if its `item_sessions` row
is ever lost it can still call `link_session_to_item` correctly instead of guessing from a stale
slash-command file or a branch name that (per research/build-vs-buy.md) never contains it.

#### Story 1.5.1: Add `item_id` to `BuildSessionInitialPrompt`'s output
**As an** agent session spawned for backlog work, **I want** my own item id printed in my
initial prompt, **so that** I can call `link_session_to_item`/`get_linked_item` correctly even
if my `item_sessions` row is later lost or was never created.
**Acceptance Criteria**:
- The initial prompt for a normal (non-triage) backlog session contains its `item_id`, matching
  the pattern the triage-mode prompt builder already uses.
  - *Given* a backlog item with `ID="I7"`, *When* `BuildSessionInitialPrompt(item, nil)` is
    called, *Then* the returned string contains the substring `item_id: I7` (or equivalent
    clearly-labeled form), matching `session/backlog_triage.go:46`'s existing
    `fmt.Fprintf(&sb, "item_id: %s\n\n", item.ID)` pattern for consistency across both prompt
    builders.
- `BuildTokenBudgetedPrompt`'s truncated-item fallback path (`backlog_context.go:229`) still
  includes the id (the id is a fixed ~36-byte UUID, negligible against the token budget this
  function manages).
**Files**: `session/backlog_context.go`

##### Task 1.5.1a: Add the `item_id` line to `BuildSessionInitialPrompt` (~2 min)
- In `session/backlog_context.go`, at the top of `BuildSessionInitialPrompt` (line ~127, right
  after the `"--- BACKLOG ITEM DATA ..."` header line), add:
  ```go
  fmt.Fprintf(&sb, "item_id: %s\n\n", item.ID)
  ```
- Files: `session/backlog_context.go`

##### Task 1.5.1b: Test (~3 min)
- In `session/backlog_context_test.go`, add
  `TestBuildSessionInitialPrompt_should_IncludeItemID_When_Called` asserting the output contains
  `item_id: <the test item's ID>`.
- Files: `session/backlog_context_test.go`

---

## Phase 2: Slash-command regeneration correctness

### Epic 2.1: Prune stale `done-N.md`/`fail-N.md` files on regeneration
**Goal**: Satisfy AC5/AC7 fully — after a relink to an item with a *different* acceptance
criteria count than what was previously on disk, the file set on disk exactly matches the new
item, not a superset.

#### Story 2.1.1: `pruneStaleSlashCommandFiles` implementation
**As an** agent session that just relinked to a different item, **I want** the old item's
leftover `done-N.md`/`fail-N.md` files removed, **so that** `/backlog:help` and my own
exploration of `.claude/commands/backlog/` don't show commands for an item I'm no longer
working on.
**Acceptance Criteria**:
- Regenerating with fewer acceptance criteria than before removes the extra files.
  - *Given* a worktree whose `.claude/commands/backlog/` directory already contains
    `done-0.md` through `done-7.md` and `fail-0.md` through `fail-7.md` (from a prior item
    with 8 AC), *When* `WriteSlashCommands` is called again for a new item with only 3 AC
    (indices 0-2), *Then* `done-3.md` through `done-7.md` and `fail-3.md` through `fail-7.md`
    no longer exist on disk, while `done-0.md`-`done-2.md`/`fail-0.md`-`fail-2.md`/
    `status.md`/`review.md`/`ship.md`/`help.md` do exist with the new item's content.
- Regenerating with the same or more criteria never deletes a file it's about to (re)write.
  - *Given* the same setup, *When* `WriteSlashCommands` is called for a new item with 10 AC,
    *Then* all 10 `done-N.md`/`fail-N.md` pairs exist and none of the original 8 pairs' content
    is stale (all rewritten with the new item's `item_id`).
**Files**: `session/backlog_commands.go`

##### Task 2.1.1a: Implement `pruneStaleSlashCommandFiles` (~4 min)
- In `session/backlog_commands.go`, add:
  ```go
  // pruneStaleSlashCommandFiles removes done-N.md/fail-N.md files in cmdDir that are not
  // present in the newly generated file set — e.g. left over from a previous item with more
  // acceptance criteria than the one just linked. Only touches done-*.md/fail-*.md; status.md,
  // review.md, ship.md, help.md are always present in newFiles for every item and never stale
  // by count. Best-effort: logs and continues past a single file's removal failure rather than
  // failing the whole write (matches WriteSlashCommands' own not-atomic contract).
  var staleCommandFileRe = regexp.MustCompile(`^(done|fail)-\d+\.md$`)

  func pruneStaleSlashCommandFiles(cmdDir string, newFiles map[string]string) {
      entries, err := os.ReadDir(cmdDir)
      if err != nil {
          return // directory just created / unreadable — nothing to prune
      }
      for _, e := range entries {
          name := e.Name()
          if !staleCommandFileRe.MatchString(name) {
              continue
          }
          if _, keep := newFiles[name]; keep {
              continue
          }
          if rmErr := os.Remove(filepath.Join(cmdDir, name)); rmErr != nil {
              log.WarningLog.Printf("[pruneStaleSlashCommandFiles] failed to remove stale %s: %v", name, rmErr)
          }
      }
  }
  ```
  Add `"regexp"` to the import block.
- Files: `session/backlog_commands.go`

##### Task 2.1.1b: Wire `pruneStaleSlashCommandFiles` into `WriteSlashCommands` (~2 min)
- In `WriteSlashCommands` (`session/backlog_commands.go:31-68`), after `files, err := ...`
  content generation succeeds and before the write loop, add:
  ```go
  pruneStaleSlashCommandFiles(cmdDir, files)
  ```
- Files: `session/backlog_commands.go`

#### Story 2.1.2: Tests, including AC5's file-content read-back requirement
**As a** maintainer, **I want** both a unit test for the pruning logic and an integration test
that reads real file contents post-relink, **so that** AC5's explicit requirement ("verified
by reading the file contents post-call in a test") is met, not just inferred.
**Acceptance Criteria**:
- Both Given-When-Then scenarios in Story 2.1.1 have a corresponding passing test.
  - *Given* `session/backlog_commands_test.go`, *When*
    `go test ./session/... -run TestWriteSlashCommands_PrunesStale` is run, *Then* it passes.
- A `AttachSessionToItem`-level (or `link_session_to_item`-level) test reads actual file
  contents after a relink and asserts the `item_id` embedded in `done-0.md` matches the newly
  linked item, not the previously linked one.
  - *Given* a live `Instance` whose worktree already has slash commands generated for item
    `I_old` (`item_id=I_old` embedded in `done-0.md`'s text), *When* that session calls
    `AttachSessionToItem`/`link_session_to_item` with `item_id=I_new`, *Then* reading
    `<worktree>/.claude/commands/backlog/done-0.md` from disk shows `item_id=I_new`, not
    `I_old`.
**Files**: `session/backlog_commands_test.go`, `server/services/backlog_service_sync_test.go`

##### Task 2.1.2a: Unit test `pruneStaleSlashCommandFiles` directly (~4 min)
- In `session/backlog_commands_test.go`, create a temp dir with 8 `done-N.md`/`fail-N.md`
  pairs plus `status.md`, call `pruneStaleSlashCommandFiles` with a `newFiles` map containing
  only 3 pairs, assert via `os.Stat` that files 3-7 are gone and 0-2 remain untouched.
- Files: `session/backlog_commands_test.go`

##### Task 2.1.2b: Test `WriteSlashCommands` end-to-end with fewer AC on second call (~4 min)
- Call `WriteSlashCommands` once with an 8-AC item, then again with a 3-AC item pointed at the
  same `worktreePath`. Assert the resulting directory listing matches exactly the 3-AC item's
  expected file set (via `os.ReadDir`), and that `done-0.md`'s content contains the *second*
  item's `item_id`, not the first's.
- Files: `session/backlog_commands_test.go`

##### Task 2.1.2c: Integration test reading files post-relink through `AttachSessionToItem` (AC5) (~5 min)
- In `server/services/backlog_service_sync_test.go` (alongside the existing
  `AttachSessionToItem` tests referenced in requirements.md — `backlog_service_test.go:1709`,
  `backlog_service_triage_test.go:2022`), add a test that: creates two backlog items with
  different AC counts, registers a fake/real `Instance` with a temp worktree path, calls
  `AttachSessionToItem` for item 1, asserts files on disk via `os.ReadFile`, then calls it
  again for item 2, and asserts (a) the file set now matches item 2 exactly (no stale item-1
  `done-N.md` beyond item 2's count) and (b) `done-0.md`'s content string-contains item 2's ID,
  not item 1's — satisfying AC5's literal "verified by reading the file contents post-call in
  a test" requirement at the RPC layer, not just the lower-level `WriteSlashCommands` unit
  layer.
- Files: `server/services/backlog_service_sync_test.go`

---

## Post-implementation verification checklist (not a task — run once all tasks above are done)

- `make build && make test` — full build + test suite (regenerates protos, though none changed here).
- `make quick-check` — build + test + lint.
- `go test ./server/mcp/... ./session/... ./server/services/... -run 'LinkSessionToItem|GetLinkedItem|ActionablePermissionDenied|PruneStale|WriteSlashCommands|AttachSessionToItem'` — targeted rerun of everything this plan touched.
- Manually verify AC6 (introspection without SQLite access) by calling `get_linked_item` against a real running instance per CLAUDE.md's "Manual/interactive testing" section (`PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`) — do **not** use `make install-service` for this, per `.claude/rules/tmux-keep-server-on-restart.md`.
