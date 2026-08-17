# Implementation Plan: backlog-link-error-consistency

**Feature**: Make `get_backlog_item` and the 5 mutating backlog MCP tools return the same error code (ITEM_NOT_FOUND vs PERMISSION_DENIED) for the same underlying condition on a given item id, with a self-diagnosable PERMISSION_DENIED message.
**Date**: 2026-08-16
**Status**: Ready for implementation
**ADRs**: ADR-001-handler-layer-link-existence-disambiguation.md

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `resolveItemLink` | New helper method on `backlogHandlers` (`server/mcp/tools_backlog.go`) that performs the session↔item link lookup and, only on the link-not-found path, disambiguates "item doesn't exist" from "item exists but no link" by falling back to `storage.GetBacklogItem`. Returns `(session.ItemSessionSummary, *mcpgo.CallToolResult)` — a non-nil result means "return this to the caller immediately." | The single new symbol this fix introduces. |
| Startup race window | The interval between `SpawnSessionFromItem` starting the tmux session (`server/services/backlog_service_triage.go` ~line 742, `CreateWorktreeSession`/`CreateDirectorySession` call) and it committing the `ItemSession` link row (~line 798, `s.storage.CreateItemSession`). A session can legitimately call a mutating tool during this window before its own link row exists. | Cited by AC2's message-wording requirement so PERMISSION_DENIED doesn't imply "stale/abandoned" for a healthy just-started session. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Where the item-vs-link disambiguation logic lives | Handler-layer helper `resolveItemLink` on `backlogHandlers` in `server/mcp/tools_backlog.go`, called by each of the 5 mutating handlers right where their existing link-check block sits today | research/architecture.md (explicit recommendation) | (A) Storage-layer fix: make `GetItemSessionBySessionAndItem` (`session/storage_backlog.go:201`) itself return a distinguishable sentinel/error type | (A)'s strength — every future caller of `GetItemSessionBySessionAndItem` gets the disambiguation for free — is outweighed by its weakness: it pushes an MCP-error-code concern down into a widely-shared storage method that has other, unrelated callers (e.g. `get_backlog_item`'s role-lookup at `tools_backlog.go:193`, which deliberately wants a single not-found-means-"no role" branch and would need to be touched to avoid behavior change). Per `.claude/rules/interface-pollution-checklist.md` item 1 (speculative interface/generalization), there is exactly one real consumer pattern today (5 handlers, same shape) — a shared *handler-layer* helper serves that consumer without widening the storage layer's contract. |
| Whether to add a new lightweight "item exists" storage primitive | Reuse the existing `storage.GetBacklogItem` (already imported and called in this file: `get_backlog_item` at line 127, `request_review` at line 404, `report_pr_created` at line 673) as the disambiguation fallback call inside `resolveItemLink` | research/stack.md, research/architecture.md | (C) Add a new lean storage method wrapping ent's `BacklogItemQuery.Exist(ctx)` (`session/ent/backlogitem_query.go:367`) — cheaper (`SELECT id LIMIT 1`) than a full `Get` | (C) is a genuinely cheaper query, but it is a new storage-layer surface with a single caller — checklist item 1 (speculative interface) and item 5 (unjustified new primitive for one call site) both apply. The extra cost of a full `GetBacklogItem` is paid only on the rare not-found-link path (never on the happy path), so the performance delta is immaterial; reusing the existing, already-imported method keeps the diff to "one more `errors.Is` branch," matching the flat sentinel + `errors.Is` convention already established in this file (research/stack.md). |
| Repository / Service Layer restructuring | N/A — explicitly out of scope | requirements.md non-goals | Redesigning `Storage`/`EntRepository` layering | This fix reuses `storage.GetItemSessionBySessionAndItem` and `storage.GetBacklogItem` exactly as they exist today; no interface or layering change. |
| TOCTOU between the link-check query and the existence-check fallback query | Accept as a documented, non-transactional race — no `ent.Tx` wrapping | research/architecture.md, research/pitfalls.md | Wrap both queries in an `ent.Tx` | `session/ent_repository.go`'s `SetMaxOpenConns(1)` already serializes all SQLite-backed calls process-wide onto one connection; a transaction would hold that single connection across two queries for a purely-read race whose failure mode is graceful (worst case: a request lands between "link created" and read-back, and returns PERMISSION_DENIED with the retry-guidance message from AC2, rather than corrupting data). This matches the existing unguarded two-query sequencing already present in `request_review` (`h.storage.GetItemSessionBySessionAndItem` at line 373, then `h.storage.GetBacklogItem` at line 404). |

---

## Migration Plan
*(Omitted — no schema or data changes.)*

## Observability Plan
- **Logs**: `resolveItemLink` logs one `log.InfoLog.Printf` line on the disambiguated PERMISSION_DENIED path only (item exists, no link found) — session UUID + item ID — matching the existing convention in this file (e.g. `request_review`'s dirty-worktree rejection log at `tools_backlog.go:386`). The far more common "item genuinely doesn't exist" (ITEM_NOT_FOUND) and "happy path" cases are not logged, consistent with existing handlers not logging their own not-found/success paths.
- **Metrics**: None added — this repo's MCP handlers have no existing metrics instrumentation to extend; out of scope to introduce one for this fix.
- **Alerts**: None — no new failure mode is being introduced; this fix makes an existing failure mode more legible, it does not create a new alertable condition.

## Risk Control
- **Feature flag**: None needed. All 5 mutating tools already gate on `featureDisabledResult(h.enabledCheck)` (FEATURE_DISABLED) before reaching the link check; this fix changes only what happens *after* that gate, on an already-error path. A flag would add complexity without a rollback benefit a plain revert doesn't already give.
- **Rollback procedure**: Standard `git revert` of the fix commit(s) — no data migration, no schema change, no in-flight state to reconcile. The old (inconsistent) behavior returns immediately.
- **Staged rollout**: N/A — ships as a normal `fix:` commit per this repo's Conventional Commits convention (patch version bump via release-please), no gradual rollout mechanism exists or is warranted for a backend MCP tool error-code fix.

## Unresolved Questions
- [ ] Should a follow-up backlog item be filed for `DeleteBacklogItem` (`session/ent_repository_backlog.go:783-840`) having no guard against deleting an item with an active session? — Not blocking any story in this plan (explicitly out of scope per requirements.md non-goals) — owner: human operator, to decide when triaging this item's suggestions.
- [ ] Should a follow-up backlog item be filed to add a real `report_blocked` MCP tool (research confirmed none exists anywhere in this codebase — no tool, command, or skill)? — Not blocking any story in this plan — owner: human operator.
- [ ] Should the list_sessions/get_session/get_session_goal timeout investigation (AC4, documented below in "Follow-up Findings") be filed as its own backlog item for root-cause confirmation against `SetMaxOpenConns(1)` contention? — owner: human operator.

## Dependency Visualization
```
Epic 1.1 (resolveItemLink helper — tools_backlog.go)
        │
        ▼
Epic 1.2 — five independent call-site stories, same file, done sequentially
  ├─ 1.2.1 report_progress        (tools_backlog.go:300-307)
  ├─ 1.2.2 request_review         (tools_backlog.go:372-379)
  ├─ 1.2.3 submit_review_verdict  (tools_backlog.go:501-508, preserve role check 509-511)
  ├─ 1.2.4 report_pr_created      (tools_backlog.go:661-668, preserve role check 669-671)
  └─ 1.2.5 submit_triage_result   (tools_backlog.go:754-761, preserve role check 762-764)
        │
        ▼
Epic 2.1 (table-driven regression test — tools_backlog_test.go)
        │
        ├────────────────────────────┐
        ▼                            ▼
Epic 3.1 (AC4 write-up —      Phase 4 (make build && make test
already drafted in this            && make lint — final gate)
plan's "Follow-up Findings"
section; implementer just
confirms it ships with the PR)
        └────────────────────────────┘
                       │
                       ▼
                     ship
```

---

## Follow-up Findings (Documented Per AC4, Not Fixed Here)

**Symptom**: `list_sessions`, `get_session`, and `get_session_goal` intermittently time out. This was reported alongside the ITEM_NOT_FOUND/PERMISSION_DENIED inconsistency but research/architecture.md confirmed it is a **structurally distinct** bug at the query/table level — none of these three RPCs join against `ItemSession` or `BacklogItem` at all, so they cannot be hitting the same join-predicate ambiguity this fix addresses.

**Plausible (unconfirmed) shared root cause**: `session/ent_repository.go:84`'s `SetMaxOpenConns(1)` forces every ent-backed call in the process onto a single SQLite connection. An unrelated slow or stuck query anywhere in the system could serialize behind that single connection and starve these three calls, producing timeouts with no direct code relationship to the `ItemSession` join bug. This is **INFERRED**, not confirmed against incident logs (none were available to research/architecture.md at investigation time).

**Recommendation**: File this as a separate follow-up backlog item scoped to root-causing `SetMaxOpenConns(1)` contention (e.g. connection-wait-time instrumentation, or evaluating whether SQLite's single-writer constraint still requires exactly one *read* connection too). Do not fix as part of this project — it shares no code path with the ITEM_NOT_FOUND/PERMISSION_DENIED fix below, and the requirements.md non-goals explicitly exclude it.

---

## Phase 1: Fix the Error-Code Inconsistency

### Epic 1.1: Add the `resolveItemLink` disambiguation helper
**Goal**: Introduce one shared helper that all 5 mutating handlers call in place of their current raw `h.storage.GetItemSessionBySessionAndItem` + single `errors.Is(linkErr, session.ErrNotFound)` branch, so the item-exists-vs-item-missing disambiguation is written exactly once.

#### Story 1.1.1: Implement `resolveItemLink` in `server/mcp/tools_backlog.go`
**As a** backlog MCP tool handler, **I want** a single method that resolves the caller's item link and classifies any not-found result correctly, **so that** all 5 mutating tools get identical, correct error-code behavior without duplicating the disambiguation logic five times.

**Acceptance Criteria**:
- The helper returns the real `session.ItemSessionSummary` and a `nil` result on success (link found).
  - *Given* session UUID `11111111-1111-1111-1111-111111111111` has an `ItemSession` row linking it to backlog item `22222222-2222-2222-2222-222222222222` with role `"work"`, *When* `h.resolveItemLink(ctx, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222")` is called, *Then* it returns `(itemSession, nil)` where `itemSession.Role == "work"`.
- The helper returns `ErrItemNotFound` when the item itself does not exist.
  - *Given* backlog item id `99999999-9999-9999-9999-999999999999` was never created (no row in `backlog_items`), *When* `h.resolveItemLink(ctx, "11111111-1111-1111-1111-111111111111", "99999999-9999-9999-9999-999999999999")` is called, *Then* it returns a non-nil `*mcpgo.CallToolResult` whose parsed JSON has `error.code == "ITEM_NOT_FOUND"`.
- The helper returns `ErrPermissionDenied` with a detailed, non-stale-implying message when the item exists but no link row matches this session.
  - *Given* backlog item `33333333-3333-3333-3333-333333333333` exists (created via `storage.CreateBacklogItem`) but no `ItemSession` row links session UUID `44444444-4444-4444-4444-444444444444` to it, *When* `h.resolveItemLink(ctx, "44444444-4444-4444-4444-444444444444", "33333333-3333-3333-3333-333333333333")` is called, *Then* it returns a result with `error.code == "PERMISSION_DENIED"` and `error.message` containing both `"44444444-4444-4444-4444-444444444444"` and `"33333333-3333-3333-3333-333333333333"`, and `error.remediation` mentioning that a just-spawned session's link may not have committed yet (not asserting the link is stale/abandoned).

**Files**: `server/mcp/tools_backlog.go`

##### Task 1.1.1a: Write the `resolveItemLink` method (~5 min)
- Insert immediately after `latestReviewVerdict` (ends at line 255) and before the `// --- report_progress ---` section marker (line 257) in `server/mcp/tools_backlog.go`.
- Implementation:
  ```go
  // resolveItemLink verifies that callerUUID is linked to itemID, returning the
  // ItemSession on success. On failure it returns a ready-to-return
  // *mcpgo.CallToolResult that distinguishes ITEM_NOT_FOUND (the item itself
  // doesn't exist) from PERMISSION_DENIED (the item exists but this session has
  // no link to it) — GetItemSessionBySessionAndItem's ent join predicate
  // (itemsession.HasBacklogItemWith) returns ErrNotFound for both cases and
  // cannot tell them apart on its own. See
  // project_plans/backlog-link-error-consistency/research/stack.md.
  func (h *backlogHandlers) resolveItemLink(ctx context.Context, callerUUID, itemID string) (session.ItemSessionSummary, *mcpgo.CallToolResult) {
      itemSession, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
      if linkErr == nil {
          return itemSession, nil
      }
      if !errors.Is(linkErr, session.ErrNotFound) {
          return session.ItemSessionSummary{}, errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), "")
      }

      // Disambiguate: does the item itself exist?
      if _, itemErr := h.storage.GetBacklogItem(ctx, itemID); itemErr != nil {
          if errors.Is(itemErr, session.ErrNotFound) {
              return session.ItemSessionSummary{}, errResult(ErrItemNotFound, fmt.Sprintf("backlog item %q not found", itemID), "")
          }
          return session.ItemSessionSummary{}, errResult(ErrInternalError, fmt.Sprintf("get backlog item: %v", itemErr), "")
      }

      log.InfoLog.Printf("[mcp:resolveItemLink] session=%s not linked to existing item=%s", callerUUID, itemID)
      return session.ItemSessionSummary{}, errResult(ErrPermissionDenied,
          fmt.Sprintf("session %s is not linked to backlog item %s", callerUUID, itemID),
          "This item exists, but no session-item link was found for this session. If this session was just spawned, "+
              "the link may not have committed yet — wait a few seconds and retry. Otherwise report this session UUID "+
              "and item ID to an operator; the link may have been lost.")
  }
  ```
- Files: `server/mcp/tools_backlog.go`

---

### Epic 1.2: Route all 5 mutating handlers through `resolveItemLink`
**Goal**: Replace each handler's existing raw link-check block with a call to `resolveItemLink`, touching only the `errors.Is(linkErr, session.ErrNotFound)` branch — never the separate, already-correct role-mismatch branches flagged by research/pitfalls.md.

#### Story 1.2.1: `report_progress`
**As a** work-role session, **I want** `report_progress` to return ITEM_NOT_FOUND (not PERMISSION_DENIED) when the item itself was deleted, **so that** I can tell "the item is gone" apart from "I'm not linked to it."
**Acceptance Criteria**:
- AC1 (requirements.md): item-missing → ITEM_NOT_FOUND; item-exists-no-link → PERMISSION_DENIED.
  - *Given* item id `55555555-5555-5555-5555-555555555555` does not exist, *When* `report_progress` is called with `item_id="55555555-5555-5555-5555-555555555555"`, `criteria_index=0`, `status="pass"` under any session UUID, *Then* the result's `error.code == "ITEM_NOT_FOUND"` (currently it would be `"PERMISSION_DENIED"`).
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.2.1a: Replace the link-check block at lines 300-307 (~3 min)
- Replace:
  ```go
  // Verify session is linked to item.
  _, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
  if linkErr != nil {
      if errors.Is(linkErr, session.ErrNotFound) {
          return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", "Only sessions assigned to the item may report progress."), nil
      }
      return errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), ""), nil
  }
  ```
  with:
  ```go
  // Verify session is linked to item (disambiguates ITEM_NOT_FOUND vs PERMISSION_DENIED).
  if _, errRes := h.resolveItemLink(ctx, callerUUID, itemID); errRes != nil {
      return errRes, nil
  }
  ```
- `report_progress` never used the returned `itemSession` value (it was already discarded via `_`), so no downstream reference needs updating.
- Files: `server/mcp/tools_backlog.go`

#### Story 1.2.2: `request_review`
**As a** work-role session, **I want** `request_review` to distinguish item-missing from not-linked, **so that** the same self-diagnosis is possible at the point implementation is declared complete.
**Acceptance Criteria**:
- *Given* item id `66666666-6666-6666-6666-666666666666` does not exist, *When* `request_review` is called with that `item_id` and a valid `message`, *Then* `error.code == "ITEM_NOT_FOUND"`.
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.2.2a: Replace the link-check block at lines 372-379 (~3 min)
- Replace:
  ```go
  // Verify session is linked to item.
  itemSession, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
  if linkErr != nil {
      if errors.Is(linkErr, session.ErrNotFound) {
          return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", ""), nil
      }
      return errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), ""), nil
  }
  ```
  with:
  ```go
  // Verify session is linked to item (disambiguates ITEM_NOT_FOUND vs PERMISSION_DENIED).
  itemSession, errRes := h.resolveItemLink(ctx, callerUUID, itemID)
  if errRes != nil {
      return errRes, nil
  }
  ```
- `itemSession` is still used later in this function (`itemSession.ID` at line 424 for `UpdateItemSessionVerificationNotes`) — keep that reference unchanged; `resolveItemLink` returns the same `session.ItemSessionSummary` shape.
- Files: `server/mcp/tools_backlog.go`

#### Story 1.2.3: `submit_review_verdict`
**As a** review-role session, **I want** `submit_review_verdict` to distinguish item-missing from not-linked while leaving the separate role-mismatch check untouched, **so that** the existing "wrong role" regression tests keep passing.
**Acceptance Criteria**:
- *Given* item id `77777777-7777-7777-7777-777777777777` does not exist, *When* `submit_review_verdict` is called with that `item_id`, a `summary`, and a valid `verdicts` array, *Then* `error.code == "ITEM_NOT_FOUND"`.
- *Given* item exists, session is linked, but `itemSession.Role != "review"` (e.g. role `"work"`), *When* `submit_review_verdict` is called, *Then* `error.code == "PERMISSION_DENIED"` with message containing `"only 'review' role may submit verdicts"` — unchanged from today (`TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork`-style existing tests for this handler must still pass).
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.2.3a: Replace the link-check block at lines 501-508 only (~3 min)
- Replace:
  ```go
  // Verify session is linked to item with role=review.
  itemSession, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
  if linkErr != nil {
      if errors.Is(linkErr, session.ErrNotFound) {
          return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", ""), nil
      }
      return errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), ""), nil
  }
  if itemSession.Role != "review" {
      return errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only 'review' role may submit verdicts", itemSession.Role), ""), nil
  }
  ```
  with:
  ```go
  // Verify session is linked to item (disambiguates ITEM_NOT_FOUND vs PERMISSION_DENIED).
  itemSession, errRes := h.resolveItemLink(ctx, callerUUID, itemID)
  if errRes != nil {
      return errRes, nil
  }
  if itemSession.Role != "review" {
      return errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only 'review' role may submit verdicts", itemSession.Role), ""), nil
  }
  ```
- **Do not touch** the `if itemSession.Role != "review"` block — it stays byte-for-byte identical (the role-mismatch condition is a separate, already-correct case per research/pitfalls.md).
- Files: `server/mcp/tools_backlog.go`

#### Story 1.2.4: `report_pr_created`
**As a** work-role session, **I want** `report_pr_created` to check the link before falling into its existing (already-correct) `GetBacklogItem` call, **so that** its dead-code ITEM_NOT_FOUND branch (currently unreachable because the link check runs first and always wins) becomes live.
**Acceptance Criteria**:
- *Given* item id `88888888-8888-8888-8888-888888888888` does not exist, *When* `report_pr_created` is called with that `item_id`, a `pr_url`, `pr_number`, and `summary`, *Then* `error.code == "ITEM_NOT_FOUND"` (today this handler already has a `GetBacklogItem` call at line 673 whose `ITEM_NOT_FOUND` branch is dead code, because the link check at line 662 always fires first and returns `PERMISSION_DENIED` instead).
- *Given* item exists, session is linked, but `itemSession.Role != session.SessionRoleWork`, *When* `report_pr_created` is called, *Then* `error.code == "PERMISSION_DENIED"` with the existing role-mismatch message — unchanged (must not regress `TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork`).
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.2.4a: Replace the link-check block at lines 661-668, leave the later `GetBacklogItem` call untouched (~4 min)
- Replace:
  ```go
  // Verify session is linked to item with role=work.
  itemSession, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
  if linkErr != nil {
      if errors.Is(linkErr, session.ErrNotFound) {
          return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", ""), nil
      }
      return errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), ""), nil
  }
  if itemSession.Role != session.SessionRoleWork {
      return errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only 'work' role may report a created PR", itemSession.Role), ""), nil
  }
  ```
  with:
  ```go
  // Verify session is linked to item (disambiguates ITEM_NOT_FOUND vs PERMISSION_DENIED).
  itemSession, errRes := h.resolveItemLink(ctx, callerUUID, itemID)
  if errRes != nil {
      return errRes, nil
  }
  if itemSession.Role != session.SessionRoleWork {
      return errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only 'work' role may report a created PR", itemSession.Role), ""), nil
  }
  ```
- The subsequent `item, getErr := h.storage.GetBacklogItem(ctx, itemID)` block at line 673 (used for the idempotency check at line 682, `item.Status`/`item.PrNumber`) stays exactly as-is — it now runs redundantly with `resolveItemLink`'s own fallback lookup only on the never-taken not-found path; on the happy path (the overwhelmingly common case) it is unchanged from today, so there is no new redundant query on the hot path.
- Files: `server/mcp/tools_backlog.go`

#### Story 1.2.5: `submit_triage_result`
**As a** triage-role session, **I want** `submit_triage_result` to distinguish item-missing from not-linked while leaving the role-mismatch check untouched, **so that** triage sessions get the same self-diagnosis capability.
**Acceptance Criteria**:
- *Given* item id `12121212-1212-1212-1212-121212121212` does not exist, *When* `submit_triage_result` is called with that `item_id` and a `summary`, *Then* `error.code == "ITEM_NOT_FOUND"`.
- *Given* item exists, session is linked, but `itemSession.Role != "triage"`, *When* `submit_triage_result` is called, *Then* `error.code == "PERMISSION_DENIED"` with the existing role-mismatch message — unchanged.
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.2.5a: Replace the link-check block at lines 754-761 only (~3 min)
- Replace:
  ```go
  // Verify session is linked to item with role=triage.
  itemSession, linkErr := h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
  if linkErr != nil {
      if errors.Is(linkErr, session.ErrNotFound) {
          return errResult(ErrPermissionDenied, "this session is not linked to the specified backlog item", ""), nil
      }
      return errResult(ErrInternalError, fmt.Sprintf("link check failed: %v", linkErr), ""), nil
  }
  if itemSession.Role != "triage" {
      return errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only 'triage' role may submit triage results", itemSession.Role), ""), nil
  }
  ```
  with:
  ```go
  // Verify session is linked to item (disambiguates ITEM_NOT_FOUND vs PERMISSION_DENIED).
  itemSession, errRes := h.resolveItemLink(ctx, callerUUID, itemID)
  if errRes != nil {
      return errRes, nil
  }
  if itemSession.Role != "triage" {
      return errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only 'triage' role may submit triage results", itemSession.Role), ""), nil
  }
  ```
- **Do not touch** the `if itemSession.Role != "triage"` block.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.2.6a: Confirm `get_backlog_item` needs no change (~2 min)
- Re-read `getBacklogItem` (lines 114-237): it already calls `h.storage.GetBacklogItem` directly (line 127) and maps `ErrNotFound` to `ITEM_NOT_FOUND` — it never goes through the ambiguous `GetItemSessionBySessionAndItem` join for its primary result (the role-lookup at line 193 uses that join, but on failure it just falls through to the generic `default` role-guidance block — it does not surface an error code at all, so there is nothing to fix there).
- No code change; record this confirmation in the PR description so a reviewer doesn't wonder why `get_backlog_item` was left untouched.
- Files: none (verification only)

---

## Phase 2: Regression Test Coverage

### Epic 2.1: Table-driven test across all 5 mutating tools + `get_backlog_item`
**Goal**: One table-driven test (not 5 copy-pasted tests) proving AC1–AC3, without regressing the existing role-mismatch PERMISSION_DENIED tests.

#### Story 2.1.1: `TestBacklogTools_LinkErrorConsistency` in `server/mcp/tools_backlog_test.go`
**As a** maintainer, **I want** one test that enumerates all 5 mutating tools plus `get_backlog_item` for both the item-missing and item-exists-no-link cases, **so that** a 6th future mutating tool that reintroduces this bug is easy to catch by extending one table, not writing a new test file.
**Acceptance Criteria** (AC3 from requirements.md):
- (a) item exists, no link → PERMISSION_DENIED for all 5 mutating tools.
  - *Given* a backlog item created via `storage.CreateBacklogItem` (e.g. title `"Link consistency test item"`, status `session.BacklogStatusInProgress`) and a session UUID `uuid.New().String()` with **no** `ItemSession` row created for it, *When* each of `reportProgress`, `requestReview`, `submitReviewVerdict`, `reportPRCreated`, `submitTriageResult` is invoked with that item's ID and minimally-valid other args (e.g. `criteria_index=0, status="pass"` for `reportProgress`; `verdicts=[{criterion_index:0, outcome:"PASS", evidence:"e"}], summary:"s"` for `submitReviewVerdict`), *Then* every one returns `error.code == "PERMISSION_DENIED"`.
- (b) item does not exist → ITEM_NOT_FOUND from `get_backlog_item` AND every one of the 5 mutating tools.
  - *Given* item id `uuid.New().String()` with no corresponding row ever created, *When* `getBacklogItem` and each of the 5 mutating handlers are invoked with that `item_id` (any session UUID, since the item-not-found branch is checked before role), *Then* every one returns `error.code == "ITEM_NOT_FOUND"`.
- (c) existing "item exists, link exists" happy path is unaffected.
  - *Given* a backlog item plus a properly-linked, correctly-roled `ItemSession` for each tool (role `"work"` for `report_progress`/`request_review`/`report_pr_created`, `"review"` for `submit_review_verdict`, `"triage"` for `submit_triage_result`), *When* each handler is called with valid arguments, *Then* each returns `success: true` (no error object) — asserted by running the existing happy-path tests (`TestReportProgress_SuccessfullyUpdatesAcStatus`, `TestRequestReview_TransitionsItemToReview`, `TestReportPRCreated_should_TransitionToPRPending_When_ValidPR`, etc.) unmodified, plus this new test's own minimal happy-path row per tool.
- Existing role-mismatch tests are confirmed unaffected: `TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork` and the equivalent role-mismatch assertions embedded in `submit_review_verdict`/`submit_triage_result` tests still return `PERMISSION_DENIED` for a wrong-role, correctly-linked session — this condition is orthogonal to the fix and must not change.

**Files**: `server/mcp/tools_backlog_test.go`

##### Task 2.1.1a: Write the table-driven test skeleton and the two negative-path scenarios (~5 min)
- Add near the end of `server/mcp/tools_backlog_test.go` (after the last existing test, `TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement` at line 1060).
- Table type:
  ```go
  type linkConsistencyCase struct {
      name     string
      role     string // role the ItemSession is created with, when a link is created
      invoke   func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error)
  }

  var linkConsistencyMutatingTools = []linkConsistencyCase{
      {"report_progress", "work", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
          return h.reportProgress(ctx, makeToolReq(map[string]interface{}{"item_id": itemID, "criteria_index": float64(0), "status": "pass"}))
      }},
      {"request_review", "work", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
          return h.requestReview(ctx, makeToolReq(map[string]interface{}{"item_id": itemID, "message": "done"}))
      }},
      {"submit_review_verdict", "review", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
          return h.submitReviewVerdict(ctx, makeToolReq(map[string]interface{}{
              "item_id": itemID, "summary": "s",
              "verdicts": []interface{}{map[string]interface{}{"criterion_index": float64(0), "outcome": "PASS", "evidence": "e"}},
          }))
      }},
      {"report_pr_created", session.SessionRoleWork, func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
          return h.reportPRCreated(ctx, makeToolReq(map[string]interface{}{
              "item_id": itemID, "pr_url": "https://github.com/tstapler/stapler-squad/pull/1", "pr_number": float64(1), "summary": "s",
          }))
      }},
      {"submit_triage_result", "triage", func(h *backlogHandlers, ctx context.Context, itemID string) (*mcpgo.CallToolResult, error) {
          return h.submitTriageResult(ctx, makeToolReq(map[string]interface{}{"item_id": itemID, "summary": "s"}))
      }},
  }
  ```
- Test `TestBacklogTools_LinkErrorConsistency_should_ReturnPermissionDenied_When_ItemExistsButNoLink`: create one item, one un-linked session UUID, loop `linkConsistencyMutatingTools`, assert `ErrPermissionDenied` for each via `t.Run(tc.name, ...)`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 2.1.1b: Write the item-not-found sub-test, including `get_backlog_item` (~5 min)
- Test `TestBacklogTools_LinkErrorConsistency_should_ReturnItemNotFound_When_ItemDoesNotExist`: generate `itemID := uuid.New().String()` with no `CreateBacklogItem` call, use any `WithSessionUUID` context (link state is irrelevant — item-not-found must win first), loop `linkConsistencyMutatingTools` plus a manual `h.getBacklogItem(ctx, ...)` call, assert `ErrItemNotFound` for all 6.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 2.1.1c: Run the new tests and the full existing backlog test file (~3 min)
- `go test ./server/mcp/... -run 'TestBacklogTools_LinkErrorConsistency|TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork|TestReportProgress|TestRequestReview|TestSubmitTriageResult|TestGetBacklogItem' -v`
- Confirm zero regressions in the pre-existing role-mismatch and happy-path tests.
- Files: none (verification only)

---

## Phase 3: Documentation (AC4)

### Epic 3.1: Confirm the timeout-investigation write-up ships with the PR
**Goal**: Satisfy AC4 — the root cause of the list_sessions/get_session/get_session_goal timeouts is documented, not fixed.

#### Story 3.1.1: Ship the "Follow-up Findings" section above with the implementation PR
**As an** operator reading the PR, **I want** the timeout investigation's finding and recommendation written down where I'll see it, **so that** I know it was looked at and consciously deferred, not missed.
**Acceptance Criteria** (AC4 from requirements.md):
- *Given* this plan.md's "Follow-up Findings (Documented Per AC4, Not Fixed Here)" section (above), *When* the implementation PR is opened, *Then* the PR description links to or quotes that section, and no code change in the PR touches `session/ent_repository.go`'s connection pool config or `list_sessions`/`get_session`/`get_session_goal` handlers.
**Files**: `project_plans/backlog-link-error-consistency/implementation/plan.md` (this file — already contains the write-up; no further edit needed)

##### Task 3.1.1a: No-op confirmation task (~2 min)
- At implementation time, re-read the "Follow-up Findings" section above and paste a short excerpt (2-3 sentences) into the PR description under a "Not fixed — follow-up" heading.
- Do **not** open any file to fix the timeout — this task is documentation-only, per requirements.md's explicit instruction not to attempt that fix here.
- Files: none (PR description only, not a repo file)

---

## Phase 4: Final Verification

### Epic 4.1: Full CI-equivalent gate before commit
**Goal**: Satisfy AC5 (no happy-path behavior change) and this repo's CLAUDE.md-mandated pre-commit checks.

#### Story 4.1.1: Run build, test, and lint
**As a** contributor, **I want** `make build && make test` and `make lint` to pass cleanly, **so that** AC5 is verified by the existing suite and no new lint violations are introduced.
**Acceptance Criteria** (AC5 from requirements.md):
- *Given* all Phase 1 and Phase 2 changes are complete, *When* `make build && make test` is run from the repo root, *Then* it exits 0 and the output includes `ok` for `github.com/tstapler/stapler-squad/server/mcp` with no failed tests (in particular, none of the pre-existing tests listed in Task 2.1.1c regress).
- *When* `make lint` is run, *Then* it exits 0 with no new findings in `server/mcp/tools_backlog.go` or `server/mcp/tools_backlog_test.go`.
**Files**: none (verification only)

##### Task 4.1.1a: Run the full gate (~5 min)
- `make build && make test`
- `make lint`
- If either fails, return to the relevant Phase 1/2 story — do not suppress or skip a failure.
- Files: none
