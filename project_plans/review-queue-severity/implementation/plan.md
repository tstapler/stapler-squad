# Implementation Plan: Review Queue Severity Levels

Source: `project_plans/review-queue-severity/requirements.md` + `research/{stack,features,architecture,pitfalls,ux,build-vs-buy}.md`.

## System type

Additive metadata-threading + small UI feature on an existing approval/review-queue
pipeline. **Not** a new system, not a new classification engine, not a new RPC. One value
(`classifier.RiskLevel`, already computed on every escalation) is threaded through five
existing structs/messages that currently drop it, plus a badge/sort/filter UI and an
analytics-breakdown extension on top of data that's already recorded. See
`research/architecture.md` for the confirmed 5-hop drop-point chain — no EventStorming table,
no new interfaces (per `.claude/rules/interface-pollution-checklist.md`).

## Step 0.5 — Alternatives considered

| # | Approach | Strength | Weakness |
|---|---|---|---|
| A | **Thread the existing `classifier.RiskLevel` string through the existing structs/proto/UI** (chosen) | Zero new classification logic; every producer already exists; matches 3 already-shipped precedents (`ApprovalRuleProto.risk_level`, `EscalationReason`/`Category` threading, `escalation_reason_counts` aggregation) | Still a genuine 5-hop + 2-frontend checklist — easy to complete only 80% of it and ship a partial rollout |
| B | Introduce a new `RiskLevelProto` enum + a dedicated `SeverityService`/`SeverityClassifier` abstraction layer, decoupled from `classifier.RiskLevel` | Would give a clean seam if severity computation ever needs to diverge from raw classifier risk (e.g. agent-self-reported severity later) | Pure speculative-interface smell (`.claude/rules/interface-pollution-checklist.md` #1/#4) for a single producer/two consumers today; adds a service layer Go doesn't need and duplicates the already-working `riskLevelString`/`parseRiskLevel` conversion |
| C | Remap to P0/P1/P2 (matching the original GitHub issue's literal ask) throughout the stack (Go, proto, storage, UI) | Matches the issue's literal wording; familiar to anyone from incident-response tooling | Inverts `RiskLevel`'s ascending-severity `iota` direction (P0=worst vs. `RiskCritical`=highest ordinal), forces a lossy 4→3 collapse dropping `RiskLow`, and creates a second vocabulary for the same value the rules UI already shows as `RiskLevel` words — see ADR-001 |

Chosen: **A**. B and C are recorded as rejected alternatives in the Pattern Decisions table
below with their specific reasons, not just here.

## Domain Glossary

| Term | Kind | One-sentence definition |
|---|---|---|
| `classifier.RiskLevel` | Existing Go type (`pkg/classifier/classifier.go:16-24`) | 4-value `iota` enum (`RiskLow`/`RiskMedium`/`RiskHigh`/`RiskCritical`) already produced by every classification path; **unchanged by this feature**. |
| `riskLevelString(classifier.RiskLevel) string` | Existing Go helper (`server/services/analytics_store.go:574`) | Canonical enum→string conversion (`"low"`/`"medium"`/`"high"`/`"critical"`); **reused**, not reimplemented. |
| `parseRiskLevel(string) classifier.RiskLevel` | Existing Go helper (`server/services/rules_store.go:351`) | Canonical string→enum conversion; **reused** wherever a string needs to become a typed `RiskLevel` (not needed by most of this plan's tasks, since the new fields stay string end-to-end — see ADR-001). |
| `PendingApproval.RiskLevel` | New Go struct field, type `string` (`server/services/approval_store.go`) | The classifier-assigned risk level captured once at approval-creation time via `riskLevelString(escalation.RiskLevel)`; `""` means "not recorded" (legacy/pre-feature). Never re-derived after creation (matches `EscalationReason`/`EscalationCategory`). |
| `PersistedApproval.RiskLevel` | New Go struct field, type `string`, JSON key `risk_level,omitempty` (`server/services/approval_store.go`) | Disk-serialized mirror of `PendingApproval.RiskLevel`; absent key on old JSON deserializes to `""`, which is the "not recorded" sentinel — not a fallback to `RiskLow`. |
| `session.ApprovalMetadata.RiskLevel` | New Go struct field, type `string` (`session/review_queue_poller.go`) | Copy of `PendingApproval.RiskLevel` surfaced to the poller/`ReviewQueuePanel` path (Path B); populated by `ApprovalStore.GetApprovalMetadataBySession`. |
| `ReviewItem.Metadata["risk_level"]` | New metadata map key, type `string` | The same value written into the `ReviewItem.Metadata` string map by `session/review_queue_poller.go`'s enrichment block, alongside the existing `escalation_reason`/`escalation_reason_category` keys. Absent key (not just empty string) is used by the frontend as the "not recorded" signal, matching the existing `escalation_reason` absent-key pattern. |
| `PendingApprovalProto.risk_level` | New proto field, `string risk_level = 10;` (`proto/session/v1/types.proto`) | Wire representation of `PendingApproval.RiskLevel`, following the `string` (not enum) convention already used by `ApprovalRuleProto.risk_level`/`SuggestedRuleProto.risk_level`. |
| `AnalyticsSummaryProto.risk_level_counts` | New proto field, `map<string, int32> risk_level_counts = 18;` (`proto/session/v1/types.proto`) | Category→count breakdown of `ClassificationAnalytics.RiskLevel` over the analytics window, following the `escalation_reason_counts = 17` field-shape precedent exactly. |
| `AnalyticsSummary.RiskLevelCounts` | New Go struct field, `map[string]int` (`server/services/analytics_store.go`) | In-process aggregation result, populated in `ComputeSummary`, copied into the proto map by `summaryToProto`. |
| `PlainApproval.riskLevel` | New TS field, type `string` (`web-app/src/lib/api/approvalsApi.ts`) | Explicit (hand-allow-listed, not structurally-inferred) field on the `PlainApproval` interface — must be added by hand; `toPlainObject` does not add it automatically for consumers typed against the interface. |
| `RiskLevel` (TS type) | New TS type alias, `web-app/src/lib/sessions/riskLevel.ts` | `"low" \| "medium" \| "high" \| "critical"`; mirrors the existing `EscalationCategory` type file (`web-app/src/lib/sessions/escalationCategory.ts`) convention exactly — a hand-maintained mirror of the Go string-constant set, no codegen bridge. |
| `SeverityBadge` | New React component, `web-app/src/components/sessions/SeverityBadge.tsx` + `.css.ts` | Renders a `RiskLevel` (or `""`/undefined "not recorded") value as an icon + text label + colour badge, in a `compact` and full variant; adapted from `StatusBadge.tsx`'s `getAttentionReasonInfo` pattern, **not** a prop added to `ReviewQueueBadge.tsx` (see Pattern Decisions). |
| `getRiskLevelInfo(riskLevel: string): SeverityInfo` | New TS function, co-located in `SeverityBadge.tsx` | Enum-string → `{ label, abbr, icon, variant }` lookup, mirrors `StatusBadge.tsx`'s `getAttentionReasonInfo`/`getDetectedStatusInfo` shape. |
| `vars.color.critical` / `criticalBg` / `criticalText` | New theme tokens (`web-app/src/styles/theme-contract.css.ts` + all 6 blocks in `theme.css.ts`) | Fourth severity colour tier, distinct from the `error`/`errorBg`/`errorText` trio already claimed by `RiskHigh`, so `RiskCritical` doesn't share a hue with High within the same badge's own tier set. |
| `severityFilter` | New component state, `Set<string>` (`ReviewQueuePanel.tsx`) | Multi-select severity filter, mirrors the existing `priorityFilter`/`reasonFilter` `Set`-based toggle pattern. |
| `"severity"` `SortField` | New value added to the existing `SortField` union (`ReviewQueuePanel.tsx:121`) | New default-selected sort dimension: severity descending (Critical→Low, `""`/unknown treated as High), existing `created_at`/age ordering as tiebreaker. |

## Pattern Decisions

Per `.claude/rules/interface-pollution-checklist.md`: this feature adds **zero** new
interfaces, services, or abstraction layers. `session.ApprovalMetadataProvider` already
exists and is satisfied by `*ApprovalStore` unchanged — it just gains one more field on the
struct it already returns. No new RPC, no new proto message type, no re-classification
service, no cache/invalidation. Confirmed against `research/architecture.md` §9.

| Decision | Choice | Alternative Rejected | Reason |
|---|---|---|---|
| Display vocabulary | Keep `RiskLevel` words (Low/Medium/High/Critical) end-to-end | Remap to P0/P1/P2 | Inverts `RiskLevel`'s ascending-severity `iota` direction vs. P0=worst; forces a lossy 4→3 collapse; creates a second vocabulary for the same value already shown (if wired) as `RiskLevel` in the rules UI. See ADR-001. |
| "Unknown severity" representation | `PendingApproval.RiskLevel` is a plain `string`, set once via `riskLevelString()`; `""` = not recorded | Keep `RiskLevel` typed as `classifier.RiskLevel` (enum) internally, string only at proto/metadata boundary | The enum's zero value (`RiskLow`, `iota=0`) is indistinguishable from a genuinely-computed Low without a companion bool/pointer every future call site must remember to check; storing the canonical string end-to-end removes the bug class instead of requiring vigilance. See ADR-001. |
| Wire field type | `string risk_level = 10;` on `PendingApprovalProto` | New `RiskLevelProto` enum | Would need its own Go↔proto conversion boilerplate and break from the two sibling fields (`ApprovalRuleProto.risk_level`, `SuggestedRuleProto.risk_level`) already using `string` in the same proto file — mixing conventions within one message family. |
| Analytics breakdown shape | `map<string, int32> risk_level_counts = 18;` on `AnalyticsSummaryProto`, following the `escalation_reason_counts = 17` precedent | `repeated RiskLevelStatProto risk_level_breakdown` (new message, modeled on `RuleStatProto`/`top_triggered_rules`) | `escalation_reason_counts` is the closer structural match — an unconditional category→count breakdown with no per-item extra fields (unlike `RuleStatProto`, which also carries `manual_allow`/`manual_deny`). Introducing a second message-shaped convention for an identical map-shaped need is unwarranted. |
| Badge component | New `SeverityBadge.tsx`, adapting `StatusBadge.tsx`'s enum→badge-info pattern | Extend `ReviewQueueBadge.tsx` with a `riskLevel` prop | `ReviewQueueBadge`'s props are strictly typed to the *session-attention* `Priority`/`AttentionReason` concept (`session/queue.go`) — requirements.md's own scope item #8 says these two priority notions "must not be confused or merged." Bolting an unrelated `RiskLevel` prop onto that component blurs exactly that boundary, even though its *visual pattern* (icon+abbr+colour+`aria-label`) is worth copying wholesale. |
| Colour tiers | Add a 4th `critical`/`criticalBg`/`criticalText` token trio | Reuse `error`/`errorBg`/`errorText` for both High and Critical, differentiated only by icon+label | The feature's whole job-to-be-done is "glance, not read" (`research/ux.md` §5) — color must carry differentiation at list-scan speed before text is read. Sharing one hue between the two most severe tiers undermines exactly the value proposition being built, for a mechanical cost (3 tokens × 6 existing theme blocks, following the exact existing success/warning/error trio shape). |
| Default sort — primary key vs. tiebreaker | Severity is the **default primary** sort key, `created_at`/age as tiebreaker; sort order frozen at snapshot-capture time in `ReviewQueuePanel` (not recomputed every render) | (a) Severity as an opt-in `SortField` only, not default. (b) Severity as a secondary tiebreaker under the existing default queue order. | (a) contradicts AC3's explicit "sorts the queue by severity by default." (b) contradicts AC3's "highest risk first" objective and every industry precedent researched (Dependabot, PagerDuty, GitHub alerts all default severity-first). The *reordering-during-interaction* risk `research/pitfalls.md` §4b flags is addressed by freezing sort order at the same point the existing `reviewingIdsSnapshot` pattern already freezes set membership — not by demoting severity's sort priority. |
| `RiskLevel` Go-side methods | None added | Add `String()`/`IsHigherThan()` methods to `classifier.RiskLevel` (mirroring `queue.Priority`) | No Go-side call site in this plan needs to sort or compare `RiskLevel` values — analytics aggregation is a map increment keyed by string, and all sorting happens client-side in TypeScript. Adding unused methods would be speculative surface area. Confirmed in ADR-001. |
| Iota-reorder guard | Add a one-line warning comment to `pkg/classifier/classifier.go`'s `RiskLevel` block (mirroring `session/queue/queue.go:181-183`'s `DetectedStatus` comment) | Leave undocumented | `research/pitfalls.md` §2 flags this as pre-existing collateral debt whose blast radius this feature increases (a second string-keyed persistence surface now depends on the enum's meaning staying stable); one-line fix, in scope per the repo's fix-collateral-debt norm. |

## Migration Plan

**No schema/DB migration in this feature.** Three persistence-adjacent surfaces are touched,
all additive and backward-compatible:

1. **`PersistedApproval` (disk JSON, `~/.stapler-squad/.../pending_approvals.json`)** — adding
   `RiskLevel string \`json:"risk_level,omitempty"\`` is a pure additive field. Existing JSON
   files written by a pre-feature binary have no `risk_level` key; `encoding/json` unmarshals
   the absent key as `""` (Go zero value for `string`), which — per ADR-001 — is the correct
   "not recorded" sentinel, not a data-loss risk. No backfill script, no version bump needed.
2. **`ClassificationAnalytics` (ent-backed SQLite)** — already stores `RiskLevel` per decision
   (`session/ent/schema/classificationanalytics.go:32`); this feature reads it, writes nothing
   new. No ent schema change, no `go generate`.
3. **Proto wire format** — new field numbers (`PendingApprovalProto.risk_level = 10`,
   `AnalyticsSummaryProto.risk_level_counts = 18`) are backward-compatible additions per
   protobuf semantics: an old client talking to a new server simply ignores the new field; a
   new client talking to an old server (mid-rolling-deploy, not applicable to this single-binary
   local-service deployment per project `CLAUDE.md`) sees an empty string / empty map, which
   again resolves to the "not recorded" sentinel.

## Observability Plan

- Extend the existing enrichment debug log line in `session/review_queue_poller.go`
  (`log.Debug("enriched approval item with hook metadata", "session", ..., "escalation_category", ...)`)
  to also log the resolved `risk_level` value — reuses the existing log call site, no new
  logging infrastructure.
- Add a regression test (Story 1.1, Task 1.1.3) asserting `server.go`'s wiring always calls
  `approvalHandler.SetClassifier(...)` before serving traffic, closing the loop on
  `research/features.md`'s finding that `h.classifier == nil` is the only genuinely
  zero-valued-`escalation` path (currently unreachable in production, but silent if a future
  refactor reintroduces it). This is a test-level guard, not a runtime metric — no product
  requirement asks for alerting on this path, and it's confirmed unreachable today.
- No new metrics/traces are warranted: this is synchronous request-scoped metadata threading
  and a read-only aggregation query, both already covered by existing request logging and the
  analytics window query's existing instrumentation (unchanged by this feature).

## Risk Control

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Zero-value `RiskLow` silently mislabels unclassified/legacy items as safe | Was High pre-mitigation | High (defeats the feature's purpose) | ADR-001: string-typed field, `""` sentinel, fail-safe "sorts as High" UI treatment (Epic 4/6) |
| Two independent frontend paths (Path A: `ApprovalCard`/`ApprovalDrawer`; Path B: `ReviewQueuePanel`) — shipping only one | Medium (easy to miss, not called out in requirements.md's own gap list) | High (partial rollout looks "done" but half the UI shows nothing) | Epic 2 Story 2.2 + Epic 6 explicitly cover Path B's `ReviewItem.Metadata["risk_level"]` enrichment separately from Epic 2 Story 2.1's proto field |
| Sort-order reordering rows mid-interaction | Medium | Medium (UX trust issue, not correctness) | Freeze `ReviewQueuePanel` sort order at `reviewingIdsSnapshot` capture time (Epic 6 Story 6.2), not recomputed every render |
| Proto field-number collision (no reserved range on `PendingApprovalProto`) | Low | High if it happens (wire corruption) | Verify field 10 is unclaimed immediately before the proto edit (Task 2.1.1); add a short reserved-range comment mirroring `ApprovalRuleProto`'s convention if a second severity-adjacent field seems likely later |
| Hand-rolled `PendingApproval{}`/`ApprovalMetadata{}` test literals silently compile with zero-value `RiskLevel` | Medium | Medium (new severity tests could pass against unset fixtures without noticing) | Task 1.1.2/1.3.1 explicitly touch and audit the 4 known fixture sites (`approval_service_test.go`, `session_service_test.go`, `session/approval_automation_test.go`, `session/review_queue_poller_test.go`'s `stubApprovalMetadataProvider`) |
| Regenerated proto bindings hand-edited or forgotten | Low | High (silent drift, no test catches it) | Task 2.1.3 runs `make proto-gen && make proto-lint && make proto-build` and commits `session/gen/...` + `web-app/src/gen/...` together with the `.proto` source change in the same task/commit |
| Feature-registry / e2e obligations skipped because this is "just a field on an existing RPC" | Medium | Low-Medium (process debt, not a runtime bug) | Epic 8 explicitly budgets registry + e2e tasks rather than leaving them implicit |

## Regression Safety (AC7)

**AC7**: "Existing approval flow (approve/deny, expiry, auto-approval, secret scan auto-deny)
is unaffected — this is additive metadata, not a behavior change to the classifier's Decision
output."

**Given** the existing secret-scan auto-deny path (`approval_handler.go:223-251`), which
returns early via `writeDecision("deny", ...)` for a request containing a plaintext AWS key
and **never reaches `createApproval`**
**When** this plan's changes are applied (every change in Epics 1-2 only touches code at or
after the `createApproval:` label, or fields never read by the secret-scan/`AutoAllow`/
`AutoDeny` early-return branches per the call-site table in `research/architecture.md` §1)
**Then** the secret-scan request is still auto-denied in the same call, with the same
`writeDecision("deny", ...)` response body — `RiskLevel` threading adds a field to a struct
that path never constructs, so its behavior is provably unchanged, not just untested.
Verified by: the existing `approval_handler_test.go` secret-scan/auto-allow/auto-deny test
cases (exact file/test names to be confirmed at implementation time) continue to pass
unmodified — no assertion in those tests should need to change, since none of them construct
or inspect a `PendingApproval`.

## Unresolved Questions

- **Not blocking for this plan — tracked as an explicit follow-up.** Agent-self-reported
  severity (the original GitHub issue's third severity source) has no existing hook/tool
  contract in this codebase (no `tool_input` convention, no separate MCP report path) and
  needs its own design (new tool_input field vs. a dedicated report call). Classifier-derived
  risk already covers the issue's own stated examples (`rm`, `git push --force`), so this is
  not a blocker for the acceptance criteria in this plan. Recommend filing as a follow-up
  backlog item once this plan ships, scoped as its own `sdd:quick`/`sdd:full` pass rather than
  folded into this one.

## Dependency Visualization

```
                        ┌────────────────────────────────────────┐
                        │ Epic 1: Backend — capture RiskLevel      │
                        │  1.1 createApproval sets RiskLevel       │
                        │  1.2 Persist across restart              │
                        │  1.3 ApprovalMetadata (Path B foundation)│
                        └───────────────┬──────────────────────────┘
                                        │
                 ┌──────────────────────┼───────────────────────┐
                 ▼                                              ▼
   ┌─────────────────────────────┐                ┌──────────────────────────────┐
   │ Epic 2: Wire format          │                │ Epic 3: Analytics breakdown   │
   │  2.1 PendingApprovalProto     │                │  3.1 Backend aggregation      │
   │      (needs 1.1)              │                │      (independent of 1.x —    │
   │  2.2 ReviewItem.Metadata      │                │       reads ClassificationAn- │
   │      (needs 1.3)              │                │       alytics, not Pending-   │
   └───────────────┬───────────────┘                │       Approval)               │
                   │                                 │  3.2 Frontend rendering       │
                   │                                 │      (needs 3.1 + proto regen)│
                   │                                 └───────────────┬────────────────┘
                   │                                                 │
                   ▼                                                 │
   ┌─────────────────────────────┐                                   │
   │ Epic 4: Frontend types +      │                                 │
   │ SeverityBadge component        │◄────────────────────────────────┘ (shared component)
   │  4.1 RiskLevel TS type +       │
   │      PlainApproval field       │
   │      (needs 2.1)               │
   │  4.2 Colour tokens             │
   │  4.3 SeverityBadge component   │
   │      (needs 4.2)               │
   └───────────────┬─────────────────┘
                   │
       ┌───────────┼─────────────────────┬───────────────────────┐
       ▼                                  ▼                       ▼
┌────────────────────┐        ┌────────────────────────┐  ┌──────────────────────┐
│ Epic 5: Path A UI    │        │ Epic 6: Path B UI        │  │ Epic 7: Rules panel    │
│ (ApprovalCard/Drawer) │        │ (ReviewQueuePanel)        │  │ badge (existing gap)   │
│  needs 4.1 + 4.3      │        │  needs 2.2 + 4.3          │  │  needs 4.3             │
└───────────┬────────────┘        └────────────┬─────────────┘  └───────────┬─────────────┘
            │                                    │                          │
            └──────────────────┬─────────────────┴──────────────────────────┘
                               ▼
                  ┌─────────────────────────────┐
                  │ Epic 8: Registry + E2E + docs │
                  │  (needs 5.x, 6.x, 7.x, 3.2)   │
                  └─────────────────────────────┘
```

---

## Phase 1: Backend Threading

### Epic 1 — Capture and persist `RiskLevel` on `PendingApproval`

#### Story 1.1 — `createApproval` sets `RiskLevel` at the moment it's already computed and discarded

**Given** a session has a pending tool-use request `git push --force origin main`
**When** `ApprovalHandler.ServeHTTP` runs it through `h.classifier.Classify()`, which matches
the seed rule for `push --force` (`RiskCritical`, `classifier.go` seed rule table), returns
`ClassificationDecision = Escalate`, and reaches the `createApproval:` label
(`approval_handler.go:384`)
**Then** the resulting `PendingApproval.RiskLevel == "critical"` (via
`riskLevelString(classifier.RiskCritical)`), captured in the same struct literal as the
existing `EscalationReason`/`EscalationCategory` fields (`approval_handler.go:427-440`).

- **Task 1.1.1** — Add `RiskLevel string` field to `PendingApproval`
  (`server/services/approval_store.go:21-46`), with a doc comment mirroring the existing
  `EscalationReason`/`EscalationCategory` comment ("set once at creation ... `""` for
  approvals created before this field existed / not computed"). Files: `approval_store.go`.
- **Task 1.1.2** — Set `RiskLevel: riskLevelString(escalation.RiskLevel)` in the
  `PendingApproval{}` literal at `approval_handler.go:427-440`. Audit and update the 3 known
  hand-rolled `PendingApproval{...}` test fixtures (`server/services/approval_service_test.go`,
  `server/services/session_service_test.go`, `session/approval_automation_test.go`) — any
  fixture a new severity-sensitive test reuses must explicitly set `RiskLevel`, not rely on
  the Go zero value. Files: `approval_handler.go` + the 3 test files above.
- **Task 1.1.3** — Add a regression test asserting production wiring always calls
  `approvalHandler.SetClassifier(...)` (per `server/server.go:485`) before the handler serves
  traffic, closing the loop on the one confirmed zero-value-`escalation` edge case
  (`h.classifier == nil`). Add a one-line comment at the `h.classifier == nil` guard
  (`approval_handler.go` ~line 307) documenting that this path yields `RiskLevel == ""` (not
  `"low"`) by construction once Task 1.1.2 lands. Files: `server/server_test.go` (or the
  existing wiring test file — verify exact location first), `approval_handler.go`.

#### Story 1.2 — Severity survives a server restart

**Given** a `PendingApproval` with `RiskLevel == "high"` (domain-age escalation, hardcoded
`RiskHigh` per `approval_handler.go:277-283`) is persisted to `pending_approvals.json` before
a server restart
**When** the server restarts and `ApprovalStore.loadFromDisk()` runs
**Then** the reloaded `PendingApproval.RiskLevel == "high"` (`Orphaned: true`), and a
subsequent `ListPendingApprovals` call still returns `risk_level: "high"` for that item.

**Given** a `pending_approvals.json` file written by a pre-feature binary (no `risk_level` key
in its JSON)
**When** the server loads it
**Then** the reloaded `PendingApproval.RiskLevel == ""` — not `"low"` — per ADR-001.

- **Task 1.2.1** — Add `RiskLevel string \`json:"risk_level,omitempty"\`` to
  `PersistedApproval` (`approval_store.go:49-62`); add `RiskLevel: a.RiskLevel` to the
  `PersistedApproval{}` literal in `persistToDiskLocked` (`approval_store.go:307-323`); add
  `RiskLevel: p.RiskLevel` to the reconstructed `PendingApproval{}` in `loadFromDisk`
  (`approval_store.go:385-399`). Files: `approval_store.go`.
- **Task 1.2.2** — Add/extend unit tests covering: (a) persist→reload round-trip preserves
  `RiskLevel`; (b) loading a JSON fixture with no `risk_level` key yields `RiskLevel == ""`,
  not `"low"`. Files: `server/services/approval_store_test.go` (verify exact existing test
  file name first).

#### Story 1.3 — Propagate `RiskLevel` into `ApprovalMetadata` (foundation for Path B)

**Given** the `PendingApproval` from Story 1.1 (`RiskLevel == "critical"`)
**When** `ApprovalStore.GetApprovalMetadataBySession(sessionID)` is called
**Then** the returned `session.ApprovalMetadata` includes `RiskLevel: "critical"`.

- **Task 1.3.1** — Add `RiskLevel string` to `session.ApprovalMetadata`
  (`session/review_queue_poller.go:55-68`); add `RiskLevel: a.RiskLevel` to the
  `session.ApprovalMetadata{}` literal in `GetApprovalMetadataBySession`
  (`server/services/approval_store.go:146-165`). Update `stubApprovalMetadataProvider` usages
  in `session/review_queue_poller_test.go` (the hand-rolled mock at line 927) so any new
  severity-sensitive poller test explicitly sets `RiskLevel` on its stub data rather than
  silently defaulting. Files: `review_queue_poller.go`, `approval_store.go`,
  `review_queue_poller_test.go`.
- **Task 1.3.2** — Add a short "do not reorder these constants — see `PendingApproval`/
  `PersistedApproval`/`ApprovalRule.risk_level` int-column usages" warning comment to
  `pkg/classifier/classifier.go`'s `RiskLevel` block (`classifier.go:16-24`), mirroring
  `session/queue/queue.go:181-183`'s existing `DetectedStatus` comment. Collateral-debt fix
  per `research/pitfalls.md` §2, in scope since this feature increases the blast radius.
  Files: `pkg/classifier/classifier.go`.

---

## Phase 2: Wire Format

### Epic 2 — Thread `RiskLevel` onto the wire, both frontend paths

#### Story 2.1 — `PendingApprovalProto.risk_level` (Path A: `ApprovalCard`/`ApprovalDrawer`)

**Given** `PendingApproval.RiskLevel == "critical"` (Story 1.1)
**When** a client calls `ListPendingApprovals`
**Then** the returned `PendingApprovalProto` for that item has `risk_level == "critical"`.

- **Task 2.1.1** — Before editing, `grep`/check `git log -p -- proto/session/v1/types.proto`
  for any other in-flight change claiming field 10 on `PendingApprovalProto` (no reserved
  range exists today, per `research/pitfalls.md` §1). Add
  `string risk_level = 10;` with a doc comment ("Classifier-assigned risk level
  (`\"low\"`/`\"medium\"`/`\"high\"`/`\"critical\"`), captured once at creation time. Empty for
  approvals that predate this field.") to `PendingApprovalProto`
  (`proto/session/v1/types.proto:1034-1061`). Files: `types.proto`.
- **Task 2.1.2** — Set `RiskLevel: a.RiskLevel` on the `PendingApprovalProto{}` literal in
  `ApprovalService.ListPendingApprovals` (`server/services/approval_service.go:174-185`).
  Files: `approval_service.go`.
- **Task 2.1.3** — Run `make proto-gen && make proto-lint && make proto-build`; commit the
  regenerated `session/gen/session/v1/*.pb.go` and `web-app/src/gen/session/v1/*_pb.ts`
  together with the `.proto` source edit and the Go handler change in the same commit (per
  `research/stack.md`'s "regenerated files must be explicitly `git add`ed" note —
  `web-app/src/gen/` is git-tracked despite matching `.gitignore`). Files: generated files
  only, no hand-edits.

#### Story 2.2 — `ReviewItem.Metadata["risk_level"]` (Path B: `ReviewQueuePanel`)

**Given** `session.ApprovalMetadata.RiskLevel == "critical"` for a session's approval (Story
1.3)
**When** `ReviewQueuePoller`'s enrichment step runs (`session/review_queue_poller.go:840-860`)
**Then** the corresponding `ReviewItem.Metadata["risk_level"] == "critical"`, alongside the
existing `escalation_reason`/`escalation_reason_category` keys already set there.

**Given** `session.ApprovalMetadata.RiskLevel == ""` (legacy/not recorded)
**When** the enrichment step runs
**Then** `ReviewItem.Metadata["risk_level"]` key is **not set at all** (mirroring the existing
`if a.EscalationReason != ""` guard pattern at `review_queue_poller.go:854`) — the frontend
distinguishes "key absent" from "key present but empty" as its "not recorded" signal.

- **Task 2.2.1** — Add `if a.RiskLevel != "" { item.Metadata["risk_level"] = a.RiskLevel }` to
  the enrichment block in `session/review_queue_poller.go` (~lines 840-860), directly beside
  the existing `EscalationReason`/`EscalationCategory` copies; extend the existing
  `log.Debug("enriched approval item with hook metadata", ...)` call to include the
  `risk_level` field (Observability Plan). Files: `review_queue_poller.go`.
- **Task 2.2.2** — Add/extend a poller test asserting `item.Metadata["risk_level"]` is set
  when the stub provider returns a non-empty `RiskLevel`, and absent when it returns `""`.
  Files: `session/review_queue_poller_test.go`.

---

## Phase 3: Analytics Breakdown

### Epic 3 — Approval-count breakdown by severity

#### Story 3.1 — Backend aggregation (no new storage — `ClassificationAnalytics.RiskLevel` already exists)

**Given** over the last 7 days, `ClassificationAnalytics` recorded 40 decisions with
`RiskLevel` values: 5 `"critical"`, 10 `"high"`, 15 `"medium"`, 10 `"low"`
**When** a client calls `GetApprovalAnalytics(window_days=7)`
**Then** `AnalyticsSummaryProto.risk_level_counts == {"critical":5,"high":10,"medium":15,"low":10}`.

- **Task 3.1.1** — Add `riskLevelCounts := make(map[string]int)` local next to
  `escalationReasonCounts` in `ComputeSummary` (`server/services/analytics_store.go:321-458`,
  ~line 345); increment `riskLevelCounts[e.RiskLevel]++` unconditionally inside the existing
  `for _, e := range entries` loop (every `AnalyticsEntry` already carries a non-empty
  `RiskLevel` string, unlike escalation reason which is conditional); assign
  `summary.RiskLevelCounts = riskLevelCounts` next to `summary.EscalationReasonCounts =
  escalationReasonCounts` (~line 455). Add `RiskLevelCounts map[string]int
  \`json:"risk_level_counts"\`` to the `AnalyticsSummary` struct
  (`analytics_store.go:88-118`, next to `EscalationReasonCounts`). Files: `analytics_store.go`.
- **Task 3.1.2** — Add `map<string, int32> risk_level_counts = 18;` (next free field number
  after `escalation_reason_counts = 17`) with a doc comment to `AnalyticsSummaryProto`
  (`proto/session/v1/types.proto:1111-1140`). Run `make proto-gen && make proto-lint &&
  make proto-build`, commit generated files. `GetApprovalAnalyticsResponse`
  (`session.proto:1429-1433`) needs **no change** — it already embeds `AnalyticsSummaryProto`
  as `Summary`. Files: `types.proto` + generated files.
- **Task 3.1.3** — Add the 3-line copy loop (mirroring `EscalationReasonCounts`'s copy at
  `rules_service.go:573-576`) for `RiskLevelCounts` in `summaryToProto`
  (`server/services/rules_service.go:518-584`). Files: `rules_service.go`.
- **Task 3.1.4** — Unit test `ComputeSummary`'s new aggregation with a fixture set of
  `AnalyticsEntry` records spanning all 4 risk levels; assert exact counts. Files:
  `server/services/analytics_store_test.go` (verify exact existing test file name first).

#### Story 3.2 — Render the breakdown in `ApprovalAnalyticsPanel`

**Given** `AnalyticsSummaryProto.risk_level_counts == {"critical":5,"high":10,"medium":15,"low":10}`
**When** `ApprovalAnalyticsPanel` renders with this summary
**Then** a "Risk Level Breakdown" table appears near the existing "Escalation Reasons"
section (`ApprovalAnalyticsPanel.tsx:329-360`), with 4 rows (Critical/High/Medium/Low),
counts, and an inline `Bar` scaled to the max count — reusing the existing `Bar` component
and `tableSection`/`table`/`row` CSS classes, not a new visualization paradigm (per
`research/build-vs-buy.md` §4's explicit "don't introduce recharts here" finding).

**Given** a window with zero escalations
**When** the panel renders
**Then** the breakdown section shows the same "No escalations in this window." empty-state
copy already used by the Escalation Reasons section (reuse, don't invent a second empty
state).

- **Task 3.2.1** — Add a `RISK_LEVEL_LABELS: Record<RiskLevel, string>` map (mirroring
  `ESCALATION_CATEGORY_LABELS` at `ApprovalAnalyticsPanel.tsx:98-105`) and a "Risk Level
  Breakdown" `tableSection` block, modeled directly on the existing "Escalation Reasons"
  block (`ApprovalAnalyticsPanel.tsx:329-360`), reading `summary.riskLevelCounts` (raw
  `AnalyticsSummaryProto`, camelCase-converted automatically by the generated bindings — no
  `PlainApproval`-style manual field list to update, since `useApprovalAnalytics.ts` returns
  the proto object directly). Files: `ApprovalAnalyticsPanel.tsx`.
- **Task 3.2.2** — Add tests for the new section: renders 4 rows with correct labels/counts;
  renders the shared empty state when all counts are zero. Files:
  `ApprovalAnalyticsPanel.test.tsx`.

---

## Phase 4: Frontend Foundation

### Epic 4 — Shared types, colour tokens, `SeverityBadge` component

#### Story 4.1 — `RiskLevel` TS type + `PlainApproval.riskLevel`

**Given** `ListPendingApprovals` now returns `risk_level: "critical"` on the wire (Story 2.1)
**When** `approvalsApi.ts`'s `getApprovals` query runs `toPlainObject` on the response
**Then** `PlainApproval.riskLevel === "critical"` is available to consumers — but only once
the interface explicitly declares the field, since `PlainApproval` is a hand-allow-listed
interface, not structurally inferred (confirmed by reading `approvalsApi.ts:13-22` — it lists
`id`/`sessionId`/`toolName`/etc. explicitly).

- **Task 4.1.1** — Create `web-app/src/lib/sessions/riskLevel.ts`:
  `export type RiskLevel = "low" | "medium" | "high" | "critical";`, mirroring
  `web-app/src/lib/sessions/escalationCategory.ts`'s exact structure and header comment
  convention (hand-maintained mirror of `pkg/classifier.RiskLevel`, no codegen bridge).
  Files: new `riskLevel.ts`.
- **Task 4.1.2** — Add `riskLevel: string;` to the `PlainApproval` interface
  (`web-app/src/lib/api/approvalsApi.ts:13-22`). Files: `approvalsApi.ts`.

#### Story 4.2 — Fourth severity colour tier

**Given** the existing theme contract only defines `success`/`warning`/`error` 3-tier status
colours
**When** a `RiskCritical` badge needs to render distinctly from a `RiskHigh` badge
**Then** `vars.color.critical`/`criticalBg`/`criticalText` resolve to a distinct, WCAG
AA-contrast-checked value in every one of the 6 existing theme blocks (light, dark, matrix,
cyberpunk77, wh40k, clean).

- **Task 4.2.1** — Add `critical: null, criticalBg: null, criticalText: null,` to the
  `color` block in `web-app/src/styles/theme-contract.css.ts` (next to the existing
  `error`/`errorBg`/`errorText`/`errorDark` group). Files: `theme-contract.css.ts`.
- **Task 4.2.2** — Add concrete `critical`/`criticalBg`/`criticalText` hex values to each of
  the 6 `createTheme(vars, {...})` blocks in `web-app/src/styles/theme.css.ts`
  (`lightTheme`, `darkTheme`, `matrixTheme`, `cyberpunk77Theme`, `wh40kTheme`, `cleanTheme`),
  following the same "verify WCAG AA contrast, note the ratio in a comment" convention already
  used for `successText`/`warningText`/`errorText` in those same blocks (e.g.
  `theme.css.ts:99`'s `/* success on successBg = 3.83:1, fails WCAG AA; #065f46 = 6.78:1 */`
  style comment). `errorDark`'s existing per-theme values are a reasonable starting point for
  `critical`/`criticalBg` since they're already the "more saturated red" in each theme, but
  need a matching `criticalText` chosen for contrast, not copy-pasted blind. Files:
  `theme.css.ts`.

#### Story 4.3 — `SeverityBadge` component

**Given** a `riskLevel` value of `"critical"`
**When** `<SeverityBadge riskLevel="critical" />` renders
**Then** it shows an icon (🔴 or ⛔), the text label "Critical", is styled with
`vars.color.criticalBg`/`criticalText`, and has `role="status"` +
`aria-label="Critical risk"`, matching `StatusBadge.tsx`'s existing icon+text+colour+`aria-label`
structure (WCAG 1.4.1 — colour is never the sole differentiator).

**Given** a `riskLevel` value of `""` or `undefined`
**When** `<SeverityBadge riskLevel="" />` renders
**Then** it shows a distinct neutral/grey "Severity not recorded" state — never a "Low" badge
— per ADR-001 and `research/ux.md` §4.

**Given** `compact={true}`
**When** the badge renders
**Then** it shows the icon + a short abbreviation (`CRIT`/`HIGH`/`MED`/`LOW`) instead of the
full word, mirroring `ReviewQueueBadge.tsx`'s existing `getPriorityAbbr` compact-mode pattern
(same *pattern*, new component per the Pattern Decisions table).

- **Task 4.3.1** — Create `web-app/src/components/sessions/SeverityBadge.tsx` +
  `SeverityBadge.css.ts`. `getRiskLevelInfo(riskLevel: string): { label: string; abbr: string;
  icon: string; variant: "low" | "medium" | "high" | "critical" | "unknown" }` co-located in
  the component file (mirroring `StatusBadge.tsx`'s `getAttentionReasonInfo`); a `vanilla-extract`
  `recipe`-based `.css.ts` with `low`/`medium`/`high`/`critical`/`unknown` variants using
  `vars.color.successBg/successText` (Low), `vars.color.warningBg/warningText` (Medium),
  `vars.color.errorBg/errorText` (High), `vars.color.criticalBg/criticalText` (Critical, Story
  4.2), and a new neutral grey (reuse `vars.color.surfaceMuted`/`textMuted` — already in the
  theme contract, no new tokens needed for the "unknown" state) for `""`/undefined. Files: new
  `SeverityBadge.tsx`, `SeverityBadge.css.ts`.
- **Task 4.3.2** — Unit tests: 4 known levels render correct label/icon/aria-label; `""`/
  `undefined` renders "Severity not recorded"; `compact` renders abbreviation. Files: new
  `SeverityBadge.test.tsx` (colocate under `web-app/src/components/sessions/__tests__/` per
  the existing `ApprovalCard.test.tsx`/`ApprovalDrawer.test.tsx` convention).

---

## Phase 5: Approval Queue UI — Path A

### Epic 5 — `ApprovalCard` / `ApprovalDrawer` (+ `ApprovalPanel` if independently sorting)

#### Story 5.1 — Badge on `ApprovalCard`

**Given** `PlainApproval.riskLevel === "critical"` for an approval shown in a card
**When** `ApprovalCard` renders
**Then** a `<SeverityBadge riskLevel={approval.riskLevel} />` appears in the card header,
alongside the existing tool-name/countdown elements.

- **Task 5.1.1** — Import and render `<SeverityBadge riskLevel={approval.riskLevel} />` in
  `ApprovalCard.tsx`'s header section (near `toolName`/`toolIcon`, `ApprovalCard.tsx:56`
  onward). Files: `ApprovalCard.tsx`, `ApprovalCard.css.ts` (layout tweak only if needed).
- **Task 5.1.2** — Update `web-app/src/components/sessions/__tests__/ApprovalCard.test.tsx`
  to assert the badge renders for a fixture with `riskLevel: "critical"` and the "not
  recorded" state for `riskLevel: ""`. Files: `ApprovalCard.test.tsx`.

#### Story 5.2 — Severity-aware default sort in `ApprovalDrawer`

**Given** three pending approvals: A (`riskLevel: "low"`, `secondsRemaining: 30`), B
(`riskLevel: "critical"`, `secondsRemaining: 200`), C (`riskLevel: "medium"`,
`secondsRemaining: 10`)
**When** `ApprovalDrawer` renders its sorted list
**Then** the order is B (Critical) → C (Medium) → A (Low) — severity primary,
`secondsRemaining` ascending as the tiebreaker within equal severity — replacing the current
`secondsRemaining`-only comparator (`ApprovalDrawer.tsx:63-65`) per the resolved "severity
primary, age/expiry tiebreaker" decision (Pattern Decisions table). `""`/unrecorded severity
sorts as if High (fail-safe), never last.

- **Task 5.2.1** — Replace the `[...approvals].sort((a, b) => a.secondsRemaining -
  b.secondsRemaining)` comparator (`ApprovalDrawer.tsx:63-65`) with a two-key comparator:
  `riskLevelRank(a.riskLevel) - riskLevelRank(b.riskLevel)` (descending severity; treat `""`
  as rank-equivalent to High) as primary, `a.secondsRemaining - b.secondsRemaining` as
  tiebreaker. Add a small `riskLevelRank(riskLevel: string): number` helper (co-located in
  `ApprovalDrawer.tsx` or imported from `riskLevel.ts`, Task 4.1.1 — prefer adding it to
  `riskLevel.ts` so it's shared with Epic 6's comparator, avoiding a second definition). Files:
  `ApprovalDrawer.tsx`, `riskLevel.ts`.
- **Task 5.2.2** — Update `ApprovalDrawer.test.tsx` for the new sort order with the exact B→C→A
  fixture above. Files: `ApprovalDrawer.test.tsx`.

#### Story 5.3 — Verify `ApprovalPanel.tsx`

**Given** `research/stack.md`/`research/features.md` flag `ApprovalPanel.tsx` as "a third
consumer... verify in planning whether it has independent list-rendering"
**When** this task inspects `ApprovalPanel.tsx`
**Then** either (a) it delegates to `ApprovalDrawer`/reuses `ApprovalCard` with no independent
sort — no change needed, plan.md states this explicitly rather than silently dropping the
file — or (b) it has its own sort/list logic, in which case apply the identical
Task 5.2.1 comparator there too.

- **Task 5.3.1** — Read `ApprovalPanel.tsx` in full; if it has independent sorting logic,
  apply the same severity-primary/expiry-tiebreaker comparator and badge rendering as
  Stories 5.1/5.2; if it delegates, add a one-line note to this plan's task tracker confirming
  no change needed. Files: `ApprovalPanel.tsx` (conditional).

---

## Phase 6: Review Queue UI — Path B

### Epic 6 — `ReviewQueuePanel`: badge, default severity sort, severity filter

#### Story 6.1 — Badge from `metadata["risk_level"]`

**Given** a `ReviewItem` with `metadata["pending_approval_id"]` set and
`metadata["risk_level"] == "critical"`
**When** `ReviewQueuePanel` renders that item
**Then** a `<SeverityBadge riskLevel={queueItem.metadata["risk_level"] ?? ""} compact />`
appears alongside the existing escalation-reason text (`ReviewQueuePanel.tsx:744-770`).

**Given** a `ReviewItem` with `metadata["pending_approval_id"]` set but no `risk_level` key
(predates this feature)
**When** the panel renders
**Then** the badge shows "Severity not recorded" (via `SeverityBadge`'s built-in unknown
state) rather than being omitted or defaulting to Low.

- **Task 6.1.1** — Render `<SeverityBadge riskLevel={queueItem.metadata["risk_level"] ?? ""}
  compact />` inside the existing `queueItem.metadata?.["pending_approval_id"]` conditional
  block (`ReviewQueuePanel.tsx:744-770`), next to the escalation-reason paragraph. Files:
  `ReviewQueuePanel.tsx`.

#### Story 6.2 — Default severity-first sort, frozen at snapshot-capture

**Given** the queue has 3 approval-pending items — `rm -rf /tmp/build` (risk_level=critical),
`npm install left-pad` (risk_level=medium), `git commit -am "wip"` (risk_level=low) — all
created within the same polling cycle
**When** a user opens `ReviewQueuePanel` with no manual sort selection
**Then** the initial `sortField` state is `"severity"` (not `"default"`), and the panel
renders `rm -rf...` first, `npm install...` second, `git commit...` last.

**Given** the user is mid-review of the above 3-item severity-sorted snapshot, and a 4th item
arrives via polling with a different severity that would, if live-recomputed, reorder the
first 3
**When** the new item arrives
**Then** the existing "N new items added" banner (`ReviewQueuePanel.tsx:993-1002`) surfaces
it, and the already-rendered 3-item order does **not** change — sort is computed once when
`reviewingIdsSnapshot` is captured/refreshed (`ReviewQueuePanel.tsx:279-296`), not on every
`allFilteredItems` recompute (`ReviewQueuePanel.tsx:372-391`).

- **Task 6.2.1** — Add `"severity"` to the `SortField` type (`ReviewQueuePanel.tsx:121`) and
  `SORT_FIELDS` array (`:136`); change the initial `sortField` state
  (`ReviewQueuePanel.tsx:242-244`) from `"default"` to `"severity"`. Add the severity
  comparator case to the existing sort `switch` (~line 375): descending by
  `riskLevelRank(item.metadata["risk_level"] ?? "")` (shared helper from Task 5.2.1), with the
  existing `created_at`/age ordering as the tiebreaker for equal ranks. Files:
  `ReviewQueuePanel.tsx`.
- **Task 6.2.2** — Move the sort computation so it runs once at `reviewingIdsSnapshot`
  capture/refresh time (the `useEffect` at `ReviewQueuePanel.tsx:283-296`) rather than inside
  the `allFilteredItems` `useMemo` (`:372-391`) that recomputes on every render — store the
  frozen order alongside the snapshot set. Add/extend a test asserting order stability across
  a simulated poll that changes underlying data but not snapshot membership. Files:
  `ReviewQueuePanel.tsx`, `ReviewQueuePanel.test.tsx`.

#### Story 6.3 — Severity filter

**Given** the same 3-item queue from Story 6.2
**When** the user clicks the "Critical" severity filter chip
**Then** only the `rm -rf /tmp/build` item is visible, the chip label reads "Critical (1)",
`hasActiveFilter` is true, and the existing "No items match the current filter... Clear
filter" empty-state copy/behavior (`ReviewQueuePanel.tsx:1221-1235`) applies unchanged when a
combination of filters yields zero results.

- **Task 6.3.1** — Add `severityFilter: Set<string>` state (mirroring `priorityFilter`/
  `reasonFilter` at `ReviewQueuePanel.tsx:232-233`), add `"severity"` to `FILTER_URL_KEYS`
  (`:124`), add the filter predicate to `allFilteredItems` (~line 341-345, alongside the
  existing `priorityFilter`/`reasonFilter` filters), and add filter-chip UI with per-level
  counts (mirroring the `priorityFilter` button block at `:1051-1062`) composed into the
  existing `hasActiveFilter`/`clearAllFilters` pipeline (`:654`, `:681-694`). Files:
  `ReviewQueuePanel.tsx`.
- **Task 6.3.2** — Tests: severity filter narrows the visible list correctly; combined with
  Story 6.2's severity sort; empty-state renders via the existing shared copy when the filter
  yields zero results. Files: `ReviewQueuePanel.test.tsx`.

---

## Phase 7: Close the Existing `ApprovalRulesPanel` Gap

### Epic 7 — Render `riskLevel` on the rules table (already wired, never rendered)

**Given** an `ApprovalRuleProto` with `risk_level == "critical"` (already threaded through
`upsertRule`, `ApprovalRulesPanel.tsx:253`, but never displayed)
**When** `ApprovalRulesPanel` renders its rules table
**Then** a new "Risk" column shows `<SeverityBadge riskLevel={rule.riskLevel} compact />` next
to the existing `decisionBadge` column (`ApprovalRulesPanel.tsx:536-549`).

- **Task 7.1.1** — Add a `<th>Risk</th>` header and corresponding `<td>` with
  `<SeverityBadge riskLevel={rule.riskLevel} compact />` to the rules table
  (`ApprovalRulesPanel.tsx:525-549`). Files: `ApprovalRulesPanel.tsx`.
- **Task 7.1.2** — Update `ApprovalRulesPanel.test.tsx` to assert the new column renders for a
  rule fixture with a set `riskLevel`. Files: `ApprovalRulesPanel.test.tsx`.

---

## Phase 8: Registry, E2E, Docs

### Epic 8 — Close out project conventions

#### Story 8.1 — Feature registry

- **Task 8.1.1** — Update `docs/registry/features/backend/approval/list-pending.json` and
  `get-analytics.json` (`lastModified`, `testIds`) to reflect the new `risk_level`/
  `risk_level_counts` fields on the existing RPCs (additive field on an existing RPC, not a
  new backend feature entry, per `.claude/rules/feature-registry.md`'s "Existing RPC method"
  path). Files: the two JSON files above.
- **Task 8.1.2** — Create new frontend registry entries: `severity-badge.json` (new
  `SeverityBadge` component), `review-queue-severity-sort-filter.json` (Epic 6),
  `approval-analytics-risk-breakdown.json` (Epic 3, mirroring the exact shape of the existing
  `approval-analytics-reason-breakdown.json`), `approval-rules-risk-column.json` (Epic 7).
  Files: new JSON files under `docs/registry/features/frontend/`.
- **Task 8.1.3** — Run `make registry-generate`; run `make registry-diff` first to confirm the
  expected delta; verify `docs/registry/coverage-gaps.json`'s count does not grow net (every
  new feature entry above should carry `tested: true` once Epics 5-7's tests land). Files:
  generated registry files.

#### Story 8.2 — E2E test

**Given** the review queue has items at multiple severities
**When** a Playwright test drives `ReviewQueuePanel`
**Then** it can assert (via `data-testid`/ARIA locators only, per
`.claude/rules/e2e-test-conventions.md`) that severity badges render, the default sort order
is severity-first, and the severity filter narrows the list.

- **Task 8.2.1** — Create `tests/e2e/review-queue-severity.spec.ts` with the
  `// @feature review-queue-severity-sort-filter, approval-analytics-risk-breakdown` header
  annotation, using `data-testid` locators (add `data-testid="severity-badge-{riskLevel}"` to
  `SeverityBadge` if not already present from Task 4.3.1), no `waitForTimeout`. Files: new
  `review-queue-severity.spec.ts`; possible `data-testid` addition to `SeverityBadge.tsx`.

#### Story 8.3 — ADR

- **Task 8.3.1** — `ADR-001-risk-level-vocabulary-and-fail-safe-representation.md` already
  written in `project_plans/review-queue-severity/decisions/` as part of this planning pass —
  no further action; referenced here for completeness of the task hierarchy.
