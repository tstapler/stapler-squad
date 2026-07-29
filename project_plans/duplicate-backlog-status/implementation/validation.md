# Validation Plan: duplicate-backlog-status
**Date**: 2026-07-29

## Happy Path Scenario

Given three backlog items that independently describe the same `install-service.sh`
`.zshrc`-sourcing bug — `10128af0-e1eb-47bc-9016-3af8fde83b4d` (earliest-created, chosen
canonical), `1dc7ff10-326c-4276-a70f-eb8869713593`, and `67de6c7b*` — and a triage agent
session whose `STAPLER_SESSION_UUID` is linked via `ItemSession` to
`1dc7ff10-326c-4276-a70f-eb8869713593`, when the agent calls
`mark_duplicate(item_id="1dc7ff10-326c-4276-a70f-eb8869713593",
duplicate_of_id="10128af0-e1eb-47bc-9016-3af8fde83b4d", note="duplicate of the
install-service.sh .zshrc-sourcing bug")`, then `CanTransitionBacklog` +
`TransitionGuard` pass, `EntRepository.TransitionBacklogItemStatus` atomically writes
`status="duplicate"` and `duplicate_of_id="10128af0-..."` in one guarded update, the note
is appended to `notes`, the item disappears from the default `ListBacklogItems` result,
and opening its detail panel in the web UI shows a `DUPLICATE` badge (visually distinct
from `ARCHIVED`) plus a clickable "Duplicate of: install-service.sh sources .zshrc
unconditionally" link that navigates to the canonical item's own detail panel in one
click.

---

## Requirement → Test Mapping

Test-name convention used below matches this codebase's existing style, confirmed by
direct inspection: Go uses `TestFunctionOrScenario_Condition` (e.g.
`TestTransitionGuard_AnyToDuplicate_RejectsSelfReference`, already used in
`session/backlog_test.go`); Jest/RTL uses `describe`/`it` with
`ComponentOrFn_should_ExpectedBehavior_When_Condition` phrasing. Test names already
enumerated in `plan.md` (Tasks 1.1.2b–e, 2.2.5a–d, 3.1.3a–e, 4.1.1b, 5.4.1a–f) are reused
verbatim rather than renamed. Rows marked **[GAP]** are requirements whose literal
Given/When/Then already appears in `plan.md`'s Story-level acceptance criteria but has
**no corresponding task** in that story's task list — a new test is designed here to
close the gap; it is not yet in `plan.md` and should be added during implementation.

**Update (2026-07-29, post repair-pass 2)**: most `[GAP]` rows below are now stale —
a second plan-repair pass added the missing tasks directly to `plan.md`: AC1 →
Task 1.1.2f, AC2-RPC → Task 2.2.5g, AC5/frontend-mapping → Task 5.2.2d, AC8-authz-test →
Task 3.1.3f, AC9-explicit-filter → covered by the rewritten Task 4.1.1b, AC10-no-op-click
and AC11-archived-canonical and the keyboard-activation test → Tasks 5.4.1g/5.4.1h/5.4.1i.
AC4's round-trip-converter gap was judged genuinely redundant with Task 2.2.5a's
re-fetch assertion and intentionally left uncovered by a dedicated task, per this doc's
own suggested discretion. The `[GAP]` annotations are left in place below as a historical
record of what this validation pass originally found missing — do not re-add these tasks
to `plan.md`, they already exist there under the task numbers listed above.

Test-type taxonomy for this repo (confirmed by inspecting the actual harnesses):
- **Unit** = pure functions, no DB/network — `session/backlog_test.go`
  (`CanTransitionBacklog`, `TransitionGuard`) and Jest/RTL component tests with all I/O
  mocked.
- **Integration** = real temp-file-SQLite-backed `session.Storage`/`EntRepository` via
  `createTestStorage(t)`, exercised through the real `BacklogService` RPC handler or the
  real `backlogHandlers` MCP handler — `server/services/backlog_service_test.go`,
  `server/mcp/tools_backlog_test.go`.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1 — `BacklogStatusDuplicate` const | `session/backlog_test.go` | `TestBacklogStatusDuplicate_HasExpectedStringValue` **[GAP — no dedicated task; implicitly exercised by every row below but never asserted standalone]** | Unit | `string(session.BacklogStatusDuplicate) == "duplicate"` |
| AC1 (error path) | — | N/A | — | A `const` declaration has no invalid-input case; compile-time reference is the only "test." |
| AC2 — allowed `→duplicate` edges + `duplicate→idea` | `session/backlog_test.go` | `TestCanTransition_AllValidPaths` (extended, Task 1.1.2a) | Unit | Rows: `idea/refining/ready/in_progress/review → duplicate` = true, `duplicate → idea` = true |
| AC2 — `done→duplicate` / `archived→duplicate` rejected | `session/backlog_test.go` | `TestCanTransition_AllInvalidPaths` (extended, Task 1.1.2a) | Unit | Rows: `{done, duplicate, false}`, `{archived, duplicate, false}` |
| AC2 (integration) | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_DoneToDuplicate_RejectsWithFailedPrecondition` **[GAP — Story 2.2.2/2.2.4 have no RPC-layer test proving the rejected edge surfaces as `connect.CodeFailedPrecondition`, only the pure-function test above]** | Integration | Create item at `done`, call RPC `TransitionBacklogItemStatus(to="duplicate", duplicate_of_id=<valid>)`, assert `connect.CodeFailedPrecondition` and no row mutation |
| AC3 — `CanTransitionBacklog` correctness, all edges | `session/backlog_test.go` | `TestCanTransition_AllValidPaths` / `TestCanTransition_AllInvalidPaths` (same as AC2; pure function, no separate integration test needed) | Unit | Same table-driven rows as AC2 |
| AC4 — ent schema field + regen | `session/ent/schema/backlog_item.go` build gate | `go build ./...` after Task 1.2.1b (not a `_test.go` test — a compile gate) | Build gate | `ent.BacklogItem` exposes `DuplicateOfID string`, `SetDuplicateOfID`, `SetNillableDuplicateOfID` |
| AC4 — round-trip through `BacklogItemData` | `session/backlog_test.go` or `server/services/backlog_service_test.go` | `TestBacklogItemData_DuplicateOfIDRoundTripsThroughConverter` **[GAP — Task 1.2.1c threads the field through `backlogItemToData` with no dedicated test; covered only incidentally by AC7's integration test below]** | Unit/Integration | Set `DuplicateOfID` via ent builder, fetch via `backlogItemToData`, assert field survives |
| AC4 — only `schema/` committed | manual `git status` check (Task 1.2.1b) | — | Manual gate | Not a runnable test; verified once per PR |
| AC5 — proto fields on both messages | `go build ./...` + `cd web-app && npx tsc --noEmit` (Task 2.1.1c) | — | Build gate | Generated Go/TS bindings expose `DuplicateOfId`/`duplicateOfId` |
| AC5 — response carries the field (Go) | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_ToDuplicate_SetsStatusAndDuplicateOfIdAtomically` (Task 2.2.5a) | Integration | RPC response `Item.DuplicateOfId` equals the request's `duplicate_of_id` |
| AC5 — proto mapping (frontend) | `web-app/src/lib/hooks/__tests__/useBacklogService.test.ts` | `mapBacklogItem_should_IncludeDuplicateOfId_When_ProtoHasIt` **[GAP — Story 5.2.2 states this as an AC but no task in Epic 5.4's list (5.4.1a–f) or elsewhere tests `mapBacklogItem` directly; confirmed by inspection this test file has zero `mapBacklogItem`/`duplicateOfId` references today]** | Unit | Fake proto `{duplicateOfId: "10128af0-..."}` → `mapBacklogItem(proto).duplicateOfId === "10128af0-..."` |
| AC6 — guard rejects empty/self/nonexistent/chained | `session/backlog_test.go` | `TestTransitionGuard_AnyToDuplicate_RequiresDuplicateOfID` (1.1.2b), `TestTransitionGuard_AnyToDuplicate_RejectsSelfReference` (1.1.2c), `TestTransitionGuard_AnyToDuplicate_RejectsNonexistentTarget` + `..._RejectsChainedDuplicateTarget` (1.1.2d) | Unit | Each asserts the correct sentinel via `errors.Is` |
| AC6 — guard accepts valid target | `session/backlog_test.go` | `TestTransitionGuard_AnyToDuplicate_AcceptsValidTarget` (1.1.2e) | Unit | Valid non-self, existing, non-duplicate-status target → `nil` |
| AC6 (integration, via RPC + via MCP) | `server/services/backlog_service_test.go`, `server/mcp/tools_backlog_test.go` | `TestTransitionBacklogItemStatus_ToDuplicate_SetsStatusAndDuplicateOfIdAtomically` (2.2.5a); `TestMarkDuplicate_SelfReference_ReturnsInvalidArgument` / `..._ChainedDuplicateTarget_ReturnsInvalidArgument` (3.1.3d) | Integration | Guard is exercised identically from both entry points — this is the "single write path, no drift" property ADR-001 argues for |
| AC7 — atomic write, happy path | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_ToDuplicate_SetsStatusAndDuplicateOfIdAtomically` (2.2.5a) | Integration | One `Save(ctx)` sets both `status` and `duplicate_of_id`; re-`Get` confirms |
| AC7 — stale write / optimistic concurrency | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_ConcurrentStatusChange_ReturnsPreconditionFailed` (2.2.5b) | Integration | Second transition against a stale expected-status returns `session.ErrPreconditionFailed` / `connect.CodeAborted` |
| AC7 — reopen clears stale `duplicate_of_id` | `server/services/backlog_service_test.go` | `TestTransitionBacklogItemStatus_Reopen_ClearsDuplicateOfId` (2.2.5d) | Integration | `duplicate→idea` clears `duplicate_of_id` atomically with the status write |
| AC7 — existing call sites still compile/pass | `server/services/backlog_service_test.go` | Task 2.2.5c (build+test run, not a new named test) | Integration | `go build ./... && go test ./server/services/... ./session/...` exits 0 |
| AC8 — `mark_duplicate` happy path | `server/mcp/tools_backlog_test.go` | `TestMarkDuplicate_HappyPath_TransitionsAndAppendsNote` (3.1.3a) | Integration | Status→`duplicate`, `duplicate_of_id` set, note appended, success result |
| AC8 — `item_id` not found | `server/mcp/tools_backlog_test.go` | `TestMarkDuplicate_ItemIdNotFound_ReturnsItemNotFound` (3.1.3b) | Integration | `ErrItemNotFound` referencing `item_id` |
| AC8 — `duplicate_of_id` not found (disambiguation risk) | `server/mcp/tools_backlog_test.go` | `TestMarkDuplicate_DuplicateOfIdNotFound_ReturnsItemNotFound_NotInternalError` (3.1.3c) | Integration | `ErrItemNotFound` (not `ErrInternalError`) referencing `duplicate_of_id` specifically |
| AC8 — guard rejection (self-ref, chained) | `server/mcp/tools_backlog_test.go` | `TestMarkDuplicate_SelfReference_ReturnsInvalidArgument`, `TestMarkDuplicate_ChainedDuplicateTarget_ReturnsInvalidArgument` (3.1.3d) | Integration | `ErrInvalidArgument` for both |
| AC8 — best-effort note-append failure | `server/mcp/tools_backlog_test.go` | `TestMarkDuplicate_NoteAppendFailure_DoesNotFailTransition` (3.1.3e) | Integration | Uses `noteAppendFn` test seam; transition still reports success |
| AC8 — session-item authorization check | `server/mcp/tools_backlog_test.go` | `TestMarkDuplicate_SessionNotLinkedToItem_ReturnsPermissionDenied` **[GAP — Task 3.1.1a-bis implements this exact check and Story 3.1.1's own acceptance criteria describe this scenario verbatim, but Story 3.1.3's task list (3.1.3a–e) never enumerates a test for it]** | Integration | Caller session linked to an unrelated item (or unlinked) → `errResult(ErrPermissionDenied, ...)`, no transition occurs |
| AC9 — excludes `duplicate` from default view | `server/services/backlog_service_test.go` | `TestListBacklogItems_ExcludesDuplicateByDefault` (extends `TestListBacklogItems_DefaultFilterHidesTerminalStatuses`, corrects Task 4.1.1b's guessed file location — confirmed via `grep -rln "func TestListBacklogItems"` that the actual home is `server/services/backlog_service_test.go`, not `session/ent_repository_backlog_test.go`, which does not exist) | Integration | Items at `idea`/`done`/`duplicate`; `ExcludeTerminal: true` returns only `idea` |
| AC9 — explicit status filter overrides exclusion | `server/services/backlog_service_test.go` | `TestListBacklogItems_ExplicitDuplicateFilter_ReturnsDuplicateItems` **[GAP — Story 4.1.1's 2nd Given/When/Then is not covered by Task 4.1.1b's single described test]** | Integration | `Statuses: ["duplicate"]` returns the `duplicate` item despite `ExcludeTerminal` |
| AC10 — distinct badge, `BacklogItemBadge` | `web-app/src/components/backlog/BacklogItemBadge.test.tsx` | `BacklogItemBadge_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate` (Task 5.4.1a) | Unit | All 8 statuses parameterized; `duplicate`'s class ≠ `archived`'s class |
| AC10 — distinct badge, `BacklogItemDetail` | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate` (Task 5.4.1b) | Unit | Same assertion against this file's independent `STATUS_CLASS` map |
| AC10 — distinct badge, table row chip | `web-app/src/app/backlog/page.test.tsx` | `BacklogTable_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate` (Task 5.4.1c) | Unit | Same assertion against `page.tsx`'s `STATUS_CSS` map |
| AC10 — action-spec label, `BacklogItemCard` | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `getActionSpec_should_ReturnDuplicateLabel_When_StatusIsDuplicate` (Task 5.4.1f) | Unit | `getActionSpec({status:"duplicate"}).label === "Duplicate"` (not raw string) |
| AC10 — action button click is a no-op | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `BacklogItemCard_should_NotInvokeOnAction_When_DuplicateButtonClicked` **[GAP — Task 5.4.1f tests only the label, not the `isDone` no-op click behavior UX §1.6/§5 AC4 requires]** | Unit | Click handler fires with `isDone: true` → `onAction` not called |
| AC10 — missing-entry compile gate (all 3 maps) | `cd web-app && npx tsc --noEmit` | — (Tasks 5.3.1b/5.3.2b/5.3.3b's `Record<KnownBacklogStatus,string>` retyping) | Build gate | A future status added to `KnownBacklogStatus` without a matching map entry fails the TS build — stronger than a runtime test for the "missing entry" failure mode; the Jest tests above still catch a *wrong* entry, which `tsc` cannot |
| AC10 — contrast, all 6 themes | `web-app/scripts/check-theme-contrast.ts` (if extended, Task 5.1.2b option a) or manual computation (option b, values already recorded in Task 5.1.2a) | — | Script/manual | All 6 `duplicateBg`/`duplicateFg` pairs ≥ 4.5:1 |
| AC11 — loading state | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowLoadingText_When_DuplicateOfFetchInFlight` (Task 5.4.1e) | Unit | Renders "Duplicate of: Loading…" with `aria-live="polite"` before the mocked fetch resolves |
| AC11 — resolved state | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowClickableCanonicalLink_When_DuplicateOfResolves` (Task 5.4.1e) | Unit | Clickable `data-testid="duplicate-of-link"` calls `onNavigateToItem(canonicalId)` on click |
| AC11 — missing state | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowItemNotFoundText_When_DuplicateOfIsNull` (Task 5.4.1e) | Unit | `getBacklogItem` mocked to resolve `null` → plain non-interactive "Duplicate of: (item not found)" text |
| AC11 — resolved canonical that is itself archived | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ResolveArchivedCanonicalItem_When_DuplicateOfIsArchived` **[GAP — Task 5.4.1e's 3 states test loading/resolved/missing generically; UX §3.4's explicit "canonical archived" edge case has no test asserting it resolves as a normal clickable link, not MISSING]** | Unit | Mock `getBacklogItem` to resolve `{status:"archived", ...}` → still RESOLVED/clickable, not MISSING |
| AC12 — full suite passes | N/A (Tasks 6.1.1a–c) | — | Suite gate | `make build && make test`, `make lint`, `cd web-app && npx jest --no-coverage` all exit 0 |
| AC13 — triage guidance updated | manual code review of `server/mcp/tools_backlog.go`'s `case "triage":` block (Task 7.1.1a) | — | Manual | Guidance text mentions `mark_duplicate` instead of archive+note |
| AC13 — 3 motivating items backfilled | manual operational verification via `get_backlog_item` (Task 7.2.1c) | — | Manual/operational | Both non-canonical items show `status:"duplicate"` + correct `duplicate_of_id`; canonical item unchanged |

**FR → AC cross-reference** (no separate rows needed — each FR is fully covered by the AC
rows above): FR1→AC1–3, FR2→AC4–5, FR3→AC6, FR4→AC7, FR5→AC8, FR6→AC9, FR7→AC10–11,
FR8→AC12 (this document *is* FR8's deliverable), FR9→AC13.

---

## UX Acceptance Tests

Source: `design/ux.md` §5, 13 human-testable criteria.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Badge distinctness (S1, card) | `web-app/src/components/backlog/BacklogItemBadge.test.tsx` | `BacklogItemBadge_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate` (5.4.1a) | Jest + RTL | Render both statuses, assert distinct classes; supplement with a one-time manual visual check (side-by-side card screenshot) since automated tests can't verify actual rendered color, only class identity |
| 2. Badge distinctness (S2, detail panel) | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate` (5.4.1b) | Jest + RTL | Same pattern against the detail panel's own map |
| 3. Badge distinctness (S3, table) | `web-app/src/app/backlog/page.test.tsx` | `BacklogTable_should_RenderDistinctDuplicateClass_When_StatusIsDuplicate` (5.4.1c) | Jest + RTL | Same pattern against `page.tsx`'s map |
| 4. Action button label + no-op (S4) | `web-app/src/components/backlog/BacklogItemCard.test.tsx` | `getActionSpec_should_ReturnDuplicateLabel_When_StatusIsDuplicate` (5.4.1f) + `BacklogItemCard_should_NotInvokeOnAction_When_DuplicateButtonClicked` **[GAP, new]** | Jest + RTL | Assert label "Duplicate"; fire click; assert `onAction` not called |
| 5. Filter chips hide `duplicate` by default | `web-app/src/app/backlog/page.test.tsx` | Filter-chip default-visibility test (5.4.1d) | Jest + RTL | Assert default `displayStatuses` excludes both `archived` and `duplicate`; `duplicate` remains in `ALL_STATUSES` |
| 6. Duplicate-link happy path (≤2 clicks to canonical panel) | manual checklist (optional: `tests/e2e/backlog.spec.ts`, not in `plan.md` scope) | `e2e:backlog-duplicate-link-navigates-to-canonical` (optional, not required for sign-off) | Manual / optional Playwright | Open duplicate item's detail panel, click "Duplicate of:" link, confirm canonical item's own panel opens with matching title — Jest's 5.4.1e only asserts `onNavigateToItem` was called with the right id, not the full URL-param → remount → title-match round trip |
| 7. Duplicate-link loading state | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowLoadingText_When_DuplicateOfFetchInFlight` (5.4.1e) | Jest + RTL | Assert substring "Loading" (not bare `…`) present before the mocked promise resolves |
| 8. Duplicate-link missing state, panel stays usable | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ShowItemNotFoundText_When_DuplicateOfIsNull` (5.4.1e), extended to also assert the close button and title remain in the DOM | Jest + RTL | Mock `null` resolution; assert plain text, no `href`/`role=button`; assert sibling panel content still renders |
| 9. Duplicate-link to archived canonical | `web-app/src/components/backlog/BacklogItemDetail.test.tsx` | `BacklogItemDetail_should_ResolveArchivedCanonicalItem_When_DuplicateOfIsArchived` **[GAP, new — see mapping table]** | Jest + RTL | Mock resolution to an `archived`-status item; assert RESOLVED/clickable, not MISSING |
| 10. No dead ends (every state has an exit) | manual checklist | — | Manual | Walk all 3 S6 states + default filter view; confirm close `[x]`, other-item navigation, or clear-filters is always present and functional |
| 11. Contrast ≥4.5:1, all 6 themes | `web-app/scripts/check-theme-contrast.ts` (if extended) or manual WCAG computation | — | Script or manual | Verify all 6 `duplicateBg`/`duplicateFg` pairs; Task 5.1.2a already records computed ratios 5.92:1–14.23:1 |
| 12. Keyboard-only pass, 0 traps | manual checklist, supplemented by `BacklogItemDetail_should_ActivateLinkOnEnterKey_When_Focused` **[GAP, new — partial automation]** | — | Manual + Jest (`fireEvent.keyDown(link, {key:"Enter"})`) | Tab to action button (confirm no-op), open detail panel, Tab to "Duplicate of:" link, activate with Enter; confirm navigation and no trap |
| 13. Screen-reader announcement sanity | manual checklist with a real AT (VoiceOver/NVDA) | — | Manual (jsdom cannot simulate real AT announcement behavior) | With a screen reader running, open a `duplicate` item's detail panel and confirm the canonical title is announced once the `aria-live="polite"` region updates, not silence and not a bare "Duplicate of:" |

Note: UX criteria 6, 10, 12, and 13 are **not fully automatable** with this repo's Jest/RTL
stack (jsdom does not execute real navigation routing, does not run a real screen reader,
and does not verify actual pixel color) — each is paired with a manual checklist step,
which is the correct tool per the instructions ("manual checklist if no e2e coverage is
warranted"). `plan.md` does not include an e2e Playwright task for this feature (Phase 5
is Jest-only), so the manual checklist is the primary verification path for those four,
not a fallback from a missing automated test.

---

## Test Stack

- **Unit**: Go stdlib `testing` (table-driven), pure functions only (`session/backlog.go`'s
  `CanTransitionBacklog`/`TransitionGuard`) — no DB, no I/O. Frontend: Jest + React Testing
  Library, all backend calls (`getBacklogItem`, RPC clients) mocked.
- **Integration**: real temp-file-SQLite-backed `session.Storage`/`EntRepository`,
  constructed via `createTestStorage(t)` (defined in
  `server/services/session_service_test.go:17`, reused by `backlog_service_test.go` and
  implicitly by `tools_backlog_test.go`'s `backlogHandlers{storage: ...}` pattern) — each
  test gets a unique `t.TempDir()`-backed SQLite file, exercised through the real
  `BacklogService` RPC handler struct or the real `backlogHandlers` MCP struct. No mocked
  repository layer exists or is needed in this repo's convention.
- **E2E / UX**: Playwright (`tests/e2e/`) exists in this repo (`tests/e2e/backlog.spec.ts`
  already covers other backlog behaviors) but `plan.md` does not add a Playwright task for
  this feature — Phase 5 is Jest-only. Per the instructions, a **manual checklist** covers
  the 4 UX acceptance criteria that need a real browser/AT/rendering engine (criteria 6,
  10, 12, 13 above); an optional `tests/e2e/backlog.spec.ts` addition is noted but not
  required for sign-off.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

- All public service methods: happy path + error paths covered (see mapping table — every
  AC has at least one happy-path and, where an error condition exists, one error-path
  test).
- All external integrations: unit mocked (pure guard/transition functions) + at least one
  integration test through the real ent-backed storage (RPC handler and MCP handler both
  covered per AC6–AC9).
- UX acceptance criteria: all 13 in `design/ux.md` §5 have a corresponding automated test,
  script check, or explicit manual-checklist step (see UX table above) — none are left
  unaddressed.

## Migration Test

**N/A** — no down-migration tooling exists in this project; see `plan.md`'s Migration Plan
section. The only schema change is an additive, nullable `duplicate_of_id` string column
applied via ent's auto-migration (`Schema.Create`) on next server startup, identical in
kind to every prior optional-field addition in this schema (e.g. `plan_artifacts_path`).
There is no rollback script format in this repo to test the reversibility of, and the
column being nullable means no backfill/invalid-row risk exists to guard against. A
fabricated `migration_should_be_reversible` test would not exercise any code path that
actually exists in this repo's ent-based schema-sync approach.
