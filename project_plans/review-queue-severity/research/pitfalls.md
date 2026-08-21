# Research: Known Pitfalls & Risks — Review Queue Severity

Scope: risks for threading `classifier.RiskLevel` through `PendingApproval` → wire →
persistence → `ReviewQueuePanel`/`ApprovalCard` UI, per `requirements.md`.

## 1. Proto/wire compatibility

- **No reserved range exists on `PendingApprovalProto`.** Unlike `ApprovalRuleProto`
  (`proto/session/v1/types.proto:1092`, explicit "field numbers 15–19 reserved" comment),
  `PendingApprovalProto` (`types.proto:1034-1061`) has no such reservation. Fields run
  1–9 (`seconds_remaining` is 9); the next free number is **10** — low collision risk today,
  but nothing stops a second in-flight change from also claiming 10. Grep
  `git log -p -- proto/session/v1/types.proto` / open PRs touching this message before
  picking the number, and add a short reserved-range comment if more than one severity-
  related field is anticipated (e.g. a future numeric risk score), mirroring the
  `ApprovalRuleProto` convention.
- **Field type precedent is `string`, not a proto `enum`.** `ApprovalRuleProto.risk_level`
  (`types.proto:1084`) and `SuggestedRuleProto.risk_level` (`:1455`) are both `string`
  (`"low"|"medium"|"high"|"critical"`), converted via hand-written
  `riskLevelString()`/`parseRiskLevel()` (`server/services/analytics_store.go:574`,
  `server/services/rules_store.go:351`). A new `PendingApprovalProto.risk_level` should
  follow this precedent (`string`) rather than introducing a `RiskLevelProto` enum — mixing
  conventions within the same message family would be inconsistent and is unnecessary
  since the existing string round-trip already works.
- `make proto-gen` regenerates both `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/*_pb.ts` — must run after any `.proto` edit, per repo
  convention (`CLAUDE.md`).

## 2. Persistence / migration — zero-value risk is dangerous here specifically

- **The Go zero value of `classifier.RiskLevel` is `RiskLow` (iota = 0)**
  (`pkg/classifier/classifier.go:20-23`: `RiskLow RiskLevel = iota` first in the block).
  `riskLevelString()`'s switch has an **explicit case for `RiskLow` → `"low"`**
  (`analytics_store.go:576-577`), so an unset/zero `RiskLevel` field does **not** fall
  through to the `default: return "medium"` branch — it renders as `"low"`.
- Consequence for orphaned/pre-existing data: any `PersistedApproval` written to disk
  *before* this field exists (or any hand-rolled test fixture that omits it) will
  deserialize with `RiskLevel` == zero value == `RiskLow`, and the UI will show a
  **"Low" severity badge** for those items — even if the underlying request was a
  `rm -rf` or force-push that would have classified as `RiskCritical` had the field been
  populated at creation time. This is the *opposite* of fail-safe for a feature whose
  entire purpose is surfacing dangerous items first. Compare to the existing
  `EscalationReason`/`EscalationCategory` comment in `approval_store.go:32-35` ("Empty for
  approvals created before this field existed") — those are strings where "empty" is
  visually distinguishable from a real value; an `int`/iota-backed `RiskLevel` has no such
  natural "unknown" state at zero.
  - **Mitigation options to weigh in planning**: (a) reorder the iota so `RiskUnspecified`
    (or similar) is 0 and `RiskLow` is 1 — but this is itself the exact hazard called out
    in `session/queue/queue.go:181-183`'s `DetectedStatus` warning ("inserting values
    mid-iota will silently corrupt persisted queue entries" — reordering existing
    `RiskLow`/`RiskMedium`/`RiskHigh`/`RiskCritical` values would corrupt every already-persisted
    `ApprovalRule.risk_level` `field.Int` row, see §2b below); (b) keep the iota as-is but
    make `PendingApproval.RiskLevel` a **pointer** or add a companion `RiskLevelKnown bool`/
    use the string wire representation with an empty-string sentinel (mirroring
    `EscalationReason`'s pattern) so "never computed" is representable and distinct from
    "computed as Low"; (c) explicitly render "Unknown" (not "Low") in the UI when the
    approval predates the field, keyed off `Orphaned` + a marker. Do **not** silently let
    zero-value `RiskLow` stand in for "unknown" — surface it as its own state.
- **No iota-reordering warning exists on `classifier.RiskLevel` today** — unlike
  `session/queue/queue.go:181-183`'s explicit `DetectedStatus` comment, there is no
  equivalent comment on `pkg/classifier/classifier.go:16-24`'s `RiskLevel` block, even
  though it is *already* persisted as a raw int today via `ApprovalRule.risk_level`
  (`session/ent/schema/approvalrule.go:35`, `field.Int("risk_level")`, converted through
  `riskLevelToInt`/`riskLevelStringFromInt` in `rules_store.go:376-399`). This gap
  predates this feature but this feature increases the blast radius (a second int-backed
  persisted store) — worth adding the warning comment to `classifier.go` while touching
  this code, per the repo's `fix-flaky-tests-dont-defer.md`-style "fix collateral debt"
  norm (`.claude/rules` / user memory `feedback_fix_collateral_debt.md`).
- **2b. Inconsistent int-vs-string persistence convention already exists in this codebase**:
  `ClassificationAnalytics.risk_level` is `field.String` (`session/ent/schema/classificationanalytics.go:32`,
  stores `"low"`/`"medium"`/etc. directly) while `ApprovalRule.risk_level` is `field.Int`
  (`approvalrule.go:35`, stores the raw iota, requiring `riskLevelToInt`/`riskLevelStringFromInt`
  round-trip functions). `PersistedApproval` (JSON, not ent) is a third storage surface —
  pick the `field.String` / `EscalationReason`-style convention (self-describing on disk,
  immune to iota reordering, consistent with how `PersistedApproval` already stores
  `EscalationCategory` as a string) rather than adding a fourth int-backed representation.

## 3. Concurrency

- `ApprovalStore.mu` (a single `sync.RWMutex`, `approval_store.go:69`) already guards all
  reads/writes of `PendingApproval`, including `Create`. Since `RiskLevel` would be set
  once at creation time inside `approval_handler.go`'s `createApproval` label (~line 384,
  same struct literal as `EscalationReason`/`EscalationCategory`) and never mutated
  afterward, adding the field introduces **no new race** — it's covered by the same
  lock discipline the existing metadata fields already use. No `Update`/`Set` method
  exists on `ApprovalStore` for post-creation mutation of an approval; don't add one for
  `RiskLevel` unless a real requirement needs it (agent-self-report is explicitly out of
  scope for this pass).
- **Real risk is a missed touchpoint, not a race**: `ApprovalStore.GetApprovalMetadataBySession`
  (`approval_store.go:146-165`) builds `session.ApprovalMetadata` values under `s.mu.RLock()`
  but currently copies only `ApprovalID, ToolName, ToolInput, Cwd, Orphaned,
  EscalationReason, EscalationCategory` — **no `RiskLevel`**. This function is a distinct
  code path from the proto `ListPendingApprovals` RPC and is easy to forget (see §4).
- Approval-rule edits (`rules_store.go`, guarded by its own separate store/mutex) and
  `ClassificationAnalytics` writes (`AnalyticsStore.RecordFromResult`) are independent
  stores from `ApprovalStore` — no shared-lock or lock-ordering risk was found between
  them and `ApprovalStore.Create`.

## 4. Frontend pitfalls

### 4a. Two separate frontend consumers of `PendingApproval` — both need the field

- **`ReviewQueuePanel.tsx`** does *not* consume `PendingApprovalProto`/`ListPendingApprovals`
  directly. It renders `ReviewItem`s (a session-attention concept, `session/queue/queue.go`),
  and approval-specific data reaches it only via `ReviewItem.Metadata` (a
  `map<string,string>`), populated in **`session/review_queue_poller.go:840-860`** from
  `session.ApprovalMetadata` (`review_queue_poller.go:55-68`) — the same struct returned
  by `ApprovalStore.GetApprovalMetadataBySession` from §3. Today it copies
  `pending_approval_id`, `tool_name`, `tool_input_command`/`tool_input_file`, `cwd`,
  `orphaned`, `escalation_reason`, `escalation_reason_category` into `item.Metadata[...]`.
  **A `risk_level` key must be added to this same copy site**, or severity will be present
  on the `ListPendingApprovals` RPC but invisible in the primary triage UI
  (`ReviewQueuePanel.tsx`), which is the component the requirements doc explicitly names
  for the severity badge/sort/filter (req. #3–4). This is a 3rd wire-through touchpoint
  beyond `PendingApproval` (Go) and `PendingApprovalProto` (wire) called out in the
  requirements doc's own gap analysis — not mentioned there, and easy to miss.
- **`ApprovalCard.tsx`** (`web-app/src/components/sessions/ApprovalCard.tsx`) instead
  consumes `PlainApproval` from `web-app/src/lib/api/approvalsApi.ts`, which flattens
  the RTK-Query response from `getApprovals` (i.e. the `ListPendingApprovals` proto
  response) via `toPlainObject`. This path *would* pick up a new
  `PendingApprovalProto.risk_level` field automatically once regenerated types are
  updated (`toPlainObject` is presumably structural), but should be verified — grep
  `PlainApproval` interface definition (`approvalsApi.ts:13`) to confirm it isn't an
  explicit allow-listed field set that would need the new field added by hand.
- Net: **there are two independent frontends and three Go-side touchpoints
  (`PendingApproval` struct, `session.ApprovalMetadata`/poller copy, `PendingApprovalProto`)**
  to thread through for full coverage — missing any one produces a partial rollout where
  one surface shows severity and the other silently doesn't.

### 4b. Reordering-during-interaction — already mitigated by an existing pattern, but only for `ReviewQueuePanel`'s snapshot list

- `ReviewQueuePanel.tsx:275-302` already implements a **"snapshot-on-enter" pattern**
  specifically to prevent exactly the UX pitfall named in the research question (a user
  about to click Approve when the list re-sorts out from under them): `reviewingIdsSnapshot`
  captures session IDs present when the queue is first entered; new items arriving later
  surface in a **"N new items added" banner** (`newItemsBanner`, lines 993-1002) rather
  than being spliced into the live list, and the displayed `items` array
  (line 415-418) is filtered down to the snapshot, forward-only (items are only ever
  *removed* from the snapshot on resolution, never re-added — `useEffect` at
  lines 289-296).
- **However, this snapshot only stabilizes *set membership*, not *sort order within the
  snapshot***. The default `sortField` is `"default"` (queue order, effectively insertion/
  poll order — line 242-244), and manual sort-by-`priority`/`age`/`diffSize`/`name` is
  opt-in via a `<select>` (lines 1176-1187), re-sorting `allFilteredItems` on every render
  via `useMemo` (lines 372-391). Per requirements acceptance criterion #3 ("sorts the
  queue by severity **by default**"), making severity the *default* sort (rather than
  today's stable insertion-order default) reintroduces the very risk the snapshot pattern
  exists to prevent: **if a live-refreshing item's severity value changes** (e.g., re-poll
  picks up a corrected classification, or — more likely — the item is simply re-fetched
  with slightly different data) **its position within the already-stable snapshot list can
  still jump**, because sort is recomputed from live `allItems` data every render, not
  frozen at snapshot time. Whether this is an actual behavior change depends on how the
  plan implements "default sort by severity": if severity is stamped once at creation and
  truly immutable per approval (per §3, it is), then within a single snapshot the *value*
  won't change server-side — but the *sort itself* still runs unconditionally on every
  render pass instead of being snapshotted alongside `reviewingIdsSnapshot`, so a newly
  favorited/filtered re-render could still visually reorder rows the user is mid-click on
  if e.g. an item's `diffStats` or other tie-break field changes. **Recommend**: freeze the
  *sort order* (not just set membership) at snapshot time — sort once when
  `reviewingIdsSnapshot` is captured/refreshed, not on every `allFilteredItems` recompute —
  or scope severity-sort strictly to initial queue load, consistent with the existing
  mitigation's intent. This directly bears on the open question in requirements.md ("hard
  primary sort key, or secondary tiebreaker") — a hard primary sort key is more prone to
  this jump than a stable secondary tiebreaker under existing `created_at`/expiry ordering.
- `ApprovalCard.tsx` has no equivalent snapshot/stabilization — it renders whatever list
  its caller (likely a modal/list showing all live `PlainApproval`s) passes in; check the
  caller (not read in this pass) for whether it re-sorts on every poll before assuming the
  same protection applies there.

## 5. Testing pitfalls

- **Hand-rolled `PendingApproval{...}` struct literals exist in at least 3 test files**:
  `server/services/approval_service_test.go` (2 occurrences), `server/services/session_service_test.go`
  (1), `session/approval_automation_test.go` (2). Go struct literals with named fields
  compile fine when a new field is added and simply omitted — these fixtures will silently
  get zero-value `RiskLevel` (`RiskLow`, per §2) rather than failing to compile, so no
  build-time signal will catch a fixture that should represent a `RiskCritical` scenario
  but doesn't set the field. Any new test asserting severity-sort/badge behavior must
  explicitly set `RiskLevel` on these fixtures, and existing fixtures should be audited
  (not just left to default) if they're reused by a new severity-sort test.
- **`ApprovalMetadata` (§3/§4a) has the same silent-omission risk** — if `RiskLevel` is
  added to `session.ApprovalMetadata` and `GetApprovalMetadataBySession` but a test mocks
  `ApprovalMetadataProvider` directly (implements the interface by hand rather than using
  the real `ApprovalStore`), that mock could return metadata without `RiskLevel` and the
  poller test would still pass while shipping an incomplete `ReviewItem.Metadata` map in
  production. Grep for other implementers of `ApprovalMetadataProvider`
  (`session/review_queue_poller.go:72`) before assuming `ApprovalStore` is the only one.
- **Regenerated proto Go/TS bindings are not hand-editable** — `make proto-gen` must be
  re-run and its output committed; a diff that hand-edits `session/gen/...` or
  `web-app/src/gen/...` instead of running the generator is a correctness gap the tests
  won't catch (no test asserts generated-vs-source consistency), only visible on next
  regen or CI drift.
- **Flaky-test discipline applies if any new severity test is order-dependent**: if a new
  `ReviewQueuePanel` test asserts exact row order under severity-default-sort, and the
  underlying data/poll timing is not fully deterministic in test setup (e.g. relies on
  `Date.now()`/wall-clock `created_at` tie-breaking), that's the same class of flake this
  repo has previously had to root-cause rather than defer (`.claude/rules/fix-flaky-tests-dont-defer.md`).
  Prefer fixed/injected timestamps in the new tests over relying on real elapsed time
  between fixture creation calls.
- **Feature registry + e2e test obligations** (project convention, not classifier-specific):
  per `.claude/rules/feature-registry.md`, any new RPC-visible field or UI feature (severity
  badge, severity filter, severity sort) needs a per-feature JSON entry under
  `docs/registry/features/` and at least one Playwright e2e test — easy to forget since this
  is additive metadata on an *existing* RPC (`ListPendingApprovals`) rather than a new RPC,
  which can make it feel exempt from the registry requirement when it isn't.

## Summary of Go-side touchpoints (for planning's task breakdown)

1. `pkg/classifier/classifier.go` — no change needed to `RiskLevel` itself; optionally
   add the iota-reorder warning comment (§2).
2. `server/services/approval_store.go` — `PendingApproval.RiskLevel`,
   `PersistedApproval.RiskLevel` (+ `loadFromDisk`/`persistToDiskLocked`),
   `GetApprovalMetadataBySession` (§3/§4a).
3. `session/review_queue_poller.go` — `ApprovalMetadata.RiskLevel` field + the
   `item.Metadata["risk_level"] = ...` copy at the `escalation_reason` copy site (§4a).
4. `server/services/approval_handler.go` — set `RiskLevel` at the `createApproval` label
   from `escalation.RiskLevel` (already computed, per requirements.md gap #4).
5. `proto/session/v1/types.proto` — `PendingApprovalProto.risk_level` (string, field 10)
   + `make proto-gen` (§1).
6. `proto/session/v1/session.proto` — `GetApprovalAnalyticsResponse` risk breakdown
   (message currently has only `summary`/`daily_buckets`, next field number 3, no
   collision risk found).
7. Frontend: `ReviewQueuePanel.tsx` (badge/sort/filter, reading `item.Metadata["risk_level"]`),
   `ApprovalCard.tsx`/`approvalsApi.ts` `PlainApproval` (verify field passthrough), analytics
   UI component consuming `GetApprovalAnalyticsResponse` (not located in this pass — grep
   for `AnalyticsSummaryProto` consumers).
