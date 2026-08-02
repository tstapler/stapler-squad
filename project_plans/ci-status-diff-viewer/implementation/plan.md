# Implementation Plan: ci-status-diff-viewer

**Feature**: Surface GitHub Actions CI status in the diff viewer, let a configurable rule
block manual Approve on red CI, and add a `ci_passing` condition to the auto-approve rule
engine — reusing the already-live `PRStatusPoller`/`WatchSessions` infrastructure end to
end rather than building new fetch/transport.
**Date**: 2026-08-02
**Status**: Ready for implementation
**ADRs**: [ADR-001: AC5 and AC6 are two independent gates on the same CI-status data](../decisions/ADR-001-separate-ci-gates.md)

---

## System type

This is an **integration/wiring feature layered onto existing infrastructure**, not a new
system. Every piece of hard infrastructure this feature needs — GitHub CI-status fetch,
caching, rate-limit handling, `Instance`/proto plumbing, and real-time delivery via
`WatchSessions` — already exists and runs in production (`session/pr_status_poller.go`,
confirmed live in `research/architecture.md` §2). The work below is: (a) reading that
already-flowing data from two new call sites (diff viewer UI, manual-approve RPC), (b)
adding one new boolean condition to an existing rule-matching function, and (c) two small,
targeted gap fixes (change-detection for CI-only updates, a checks-page link). No new
services, pollers, background goroutines, or transport are introduced anywhere in this plan.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `GitHubCheckConclusion` | Existing `Instance` field holding the CI rollup conclusion: `success`/`failure`/`pending`/`neutral`/`""`. | `session/instance.go:206-207`. Do not rename. |
| `LastPRStatusCheck` | Existing `Instance` timestamp of the last successful CI/PR status fetch, used to show badge staleness. | `session/instance.go:210-211`. |
| `HasAssociatedPR()` | Existing `Instance` method; `true` when the session has a linked GitHub PR. Used to gate AC7's "no PR → no badge / unaffected by blocking rule." | `session/instance.go:740`. |
| `FindInstanceDataByID` | Existing `Storage` lookup used by both new gates to resolve a session's CI status from a session ID. | `session/storage.go:407`. |
| `ClassificationContext.CIStatus` | New `string` field on the classifier's per-request context, mirroring `GitHubCheckConclusion`'s vocabulary; `""` means "no PR or unknown." | New — `pkg/classifier/classifier.go:61-72`. |
| `Rule.RequireCIPassing` | New `bool` field on `pkg/classifier`'s `Rule`; when `true`, the rule only matches if `ClassificationContext.CIStatus == "success"`. ANDed with existing fields exactly like `FilePattern`. | New — `pkg/classifier/classifier.go:343-369`. |
| `RuleSpec.RequireCIPassing` | New `bool` field mirroring `Rule.RequireCIPassing` in the persisted/proto rule shape. | New — `server/services/rules_store.go:22-48`. |
| `ApprovalRuleProto.require_ci_passing` | New proto field (number 29) on `ApprovalRuleProto` carrying `RuleSpec.RequireCIPassing` over the wire. | New — `proto/session/v1/types.proto:1076-1105`. |
| `review:block-approval-on-ci-failure` | New feature-flag name registered in the existing generic feature-flag system; gates AC5's manual-Approve block. Default off (unset flags read `false`). | New — `server/services/feature_flag_service.go:22-46`, backed by `Config.FeatureFlags` (`config/config.go:326`, `config/config.go:999-1013`). |
| `CIStatusBadge` | New React component rendering the 4-state CI badge (passing/failing/pending/no-checks) with a link to the PR's Checks tab. Renders nothing when the session has no PR (AC7). | New — `web-app/src/components/sessions/CIStatusBadge.tsx`. |
| `CheckConclusion` | Existing frontend type: `"success" \| "failure" \| "pending" \| ""`. | `web-app/src/lib/vcs/types.ts`. Not modified — `neutral` already collapses to `""` via `toCheckConclusion`. |
| `toCheckConclusion` | Existing adapter narrowing a raw proto string to `CheckConclusion`; already folds `neutral`/anything unrecognized into `""`. | `web-app/src/lib/vcs/adapters.ts:21-23`. Reused as-is. |
| `formatRelativeTime` / `timestampDate` | Existing utilities used to render `LastPRStatusCheck` as "checked Ns ago" in the badge tooltip. | `web-app/src/lib/utils/datetime.ts:62`, `@bufbuild/protobuf/wkt` (precedent: `web-app/src/app/logs/page.tsx:8,325`). |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| AC5 block-on-red gate | Inline guard clause in `ApprovalService.ResolveApproval`, config-driven | `research/architecture.md` §1d (explicit recommendation) | (a) Speculative `ApprovalGate` strategy interface | Single implementation, no near-term second one — the exact "speculative interface" smell `.claude/rules/interface-pollution-checklist.md` flags. See ADR-001. |
| AC5 block-on-red gate | (same as above) | | (b) Gate inside `Instance.Approve()` (`session/instance_state.go:292`) | Wrong layer: mixes a product-config toggle into the session state machine, and that layer has no natural access to `config.LoadConfig()` or a way to return a rich `connect.Code` to the RPC caller. |
| AC6 `ci_passing` condition | New `Rule.RequireCIPassing bool`, ANDed in `matchesRule` like existing fields | `research/architecture.md` §1c | (a) Pre-filter CI status in `HandlePermissionRequest` outside the classifier | Breaks the declarative rule model — AC6 explicitly requires composing with an existing regex condition on the *same* rule; a caller-side special case can't express "regex AND ci_passing" on one rule. |
| AC6 `ci_passing` condition | (same as above) | | (b) Generic `ConditionEvaluator` plugin interface for future condition types | Unjustified generic for a single current need — YAGNI/interface-pollution violation; `matchesRule`'s existing flat-AND model already accommodates one more bool field with no abstraction needed. |
| Diff-viewer CI badge | New sibling `CIStatusBadge.tsx`, importing `GitHubBadge.css.ts`'s existing variant classes (`prBadgeReady`/`prBadgeBlocking`/`prBadgePending`/`prBadgeUnknown`) | Synthesizes `research/ux.md` §3 option (b) + `research/build-vs-buy.md` §4 | (a) Extend `GitHubBadge.tsx`'s existing PR-badge render branch in place | `GitHubBadge` is used at 3 other call sites (`SessionCard`, `SessionRow`, `SessionDetailView`) rendering a *PR-number* badge; the diff viewer needs a CI-only badge (no duplicate PR-number chip, since one is already shown elsewhere in the same `SessionDetailView`). Editing the shared component's PR-badge branch to conditionally suppress the PR number risks regressing all 3 existing call sites for a need only the new 4th caller has. |
| Diff-viewer CI badge | (same as above) | | (b) Net-new CSS/visual idiom for the badge | Explicitly rejected by both `research/ux.md` ("do not invent a third badge idiom") and `research/build-vs-buy.md` — the new component reuses 100% of `GitHubBadge.css.ts`'s existing tokens/variants, so it is *not* a third idiom, just a second render target for the same one. |
| Global settings surface for AC5 | Reuse existing `Config.FeatureFlags` map + `knownFeatureFlags` registry (`server/services/feature_flag_service.go`) | Confirmed live, generic, zero-new-UI mechanism (verified by reading `web-app/src/app/settings/features/page.tsx`, which renders `knownFeatureFlags` generically) | New dedicated `Config.BlockApprovalOnCIFailure bool` field + bespoke settings UI | Duplicates a mechanism that already exists for exactly this shape of toggle (see `sddDefaultPipelineFlagName` precedent, `server/services/backlog_service_lifecycle.go:134,145`) — adds a new JSON key and new UI code for no behavioral gain. |
| CI-status normalization | Reuse `GitHubCheckConclusion`'s existing vocabulary (`success`/`failure`/`pending`/`neutral`/`""`) as-is; no new normalizer | `research/build-vs-buy.md` §1/§3 (already canonical, already the one on the wire) | Retroactively consolidate all 3 existing fetchers (`getCheckConclusion`, `normalizeCheckState`, `fetchCILabel`) into one shared function now | Out of scope: none of this feature's new code paths call the two weaker fetchers (`normalizeCheckState`, `fetchCILabel`) — they're used by unrelated features (`UserPRCache`, backlog labeling). Touching them adds risk with no acceptance-criterion benefit; flagged as a separate future cleanup, not silently dropped. |
| AC2 check-page link | Pragmatic `${prUrl}/checks` URL construction, no new stored field | `research/architecture.md` §2 gap analysis (explicit "likely minimal fix") | Capture a specific `details_url` per check run end-to-end (new `Instance` field, new poller plumbing, new proto field) | Disproportionate new work for AC2's literal text ("link out to the corresponding GitHub Actions run/check page" — GitHub's own Checks tab at `<prUrl>/checks` satisfies this for any conclusion state). |
| Badge state vocabulary | Exactly AC1's literal 4 states (passing/failing/pending/no-checks); no 5th "fetch-error" state | AC1's literal text is authoritative over `research/ux.md`'s elaborated 5-state ideal | Add a distinct "CI status unavailable / fetch error" badge state | No backend signal exists today distinguishing "last fetch failed" from "no checks configured" (`PRStatusPoller` retries/backs off silently, no per-`Instance` last-error field) — building one is unscoped net-new plumbing beyond AC1. Logged in Unresolved Questions as a deliberate, documented scope cut, not silently dropped. |

---

## Migration Plan

No schema/data migration needed.

- `review:block-approval-on-ci-failure` reuses the existing `Config.FeatureFlags
  map[string]bool` (`config/config.go:326`), persisted via the existing
  `SetFeatureFlag`/`SaveConfig` path (`config/config.go:999-1013`) — the same mechanism
  already used for `backlog:sdd-default-pipeline`
  (`server/services/feature_flag_service.go:18,42-45`). No new config field, no new
  top-level JSON key.
- `RequireCIPassing` adds one new proto field
  (`ApprovalRuleProto.require_ci_passing = 29`) and one new Go/JSON field on `RuleSpec`.
  Proto3 defaults + Go `omitempty` mean existing persisted `auto_approve_rules.json`
  entries and existing proto clients are unaffected — the field simply reads as `false`
  for every rule that predates this change. `make proto-gen` regenerates bindings; no
  data backfill required.

## Observability Plan

- **Logs**: `ApprovalService.ResolveApproval` logs the session ID and CI conclusion when
  the block fires (mirrors the existing `log.Info("[ApprovalService] resolved
  approval"...)` at `server/services/approval_service.go:97`). The classifier's
  `ci_passing`-caused non-match populates a distinguishable `ClassificationResult.Reason`
  string (per `research/features.md` §7) so `AnalyticsStore.RecordFromResult` — already
  logging every classification decision — captures *why* a rule didn't match, not just
  that it didn't.
- **Metrics**: none added. No existing metrics/counter pipeline for approval or
  classification decisions was found in research; consistent with this repo's YAGNI
  guidance, this plan does not introduce a first one for a two-gate feature.
- **Alerts**: none. No existing alerting infrastructure covers approval-flow features in
  this codebase.

## Risk Control

- **Feature flag**: `review:block-approval-on-ci-failure`, default off, toggled
  instantly via the existing Settings → Feature Flags UI (`UpdateFeatureFlag` RPC,
  "Changes take effect immediately — no restart needed" per
  `web-app/src/app/settings/features/page.tsx:37`).
- **Rollback procedure**: AC5 — toggle the flag back off; the gate is re-evaluated from
  `config.LoadConfig()` on every `ResolveApproval` call, so no code rollback or restart
  is needed. AC6 — `RequireCIPassing` is opt-in per rule (a user must explicitly check it
  when authoring/editing a rule); existing rules are structurally unaffected, so rollback
  is "uncheck the box on that rule" with zero blast radius to other rules.
- **Staged rollout**: not applicable — this is a single-instance, self-hosted tool with
  no multi-tenant rollout infrastructure in this codebase.

## Unresolved Questions

None blocking. Two deliberate scope decisions are recorded in the Pattern Decisions table
above rather than left open:
1. Whether to add a 5th "CI status unavailable / fetch error" badge state — deferred; no
   backend signal exists to drive it yet (see table row "Badge state vocabulary").
2. Whether `ci_passing` should force a synchronous CI re-fetch at rule-evaluation time
   instead of trusting the ~60s-stale poller cache — `research/architecture.md` §3
   explicitly frames this as optional and not required by any AC; this plan accepts the
   existing staleness bound, consistent with every other consumer of
   `GitHubCheckConclusion`. Revisit only if false-positive auto-approvals are observed in
   practice.

## Dependency Visualization

```
Phase 1: ci_passing rule condition (AC6, AC8, AC9)       Phase 2: block manual Approve on CI red (AC5, AC7, AC9)
  1.1 Rule + ClassificationContext plumbing                 2.1 Feature flag registration
        |                                                          |
  1.2 Populate CIStatus in ApprovalHandler                   2.2 Storage wired into ApprovalService
        |                                                          |
  1.3 Persist RequireCIPassing (RuleSpec/proto)              2.3 Gate in ResolveApproval + frontend surfacing
        |                                                          |
  1.4 Rule-builder UI checkbox                               2.4 Unit test
        |                                                          |
  1.5 Unit tests                                                   |
        |___________________________________________________________|
                                    |
                    Phase 3: diff-viewer badge + freshness fix (AC1, AC2, AC3, AC4, AC7)
                      3.1 DiffViewer wiring + CIStatusBadge component
                      3.2 PRStatusPoller change-detection fix (checkConclusion-only updates)
                                    |
                    Phase 4: e2e coverage + feature registry (AC8, AC10)
                      4.1 e2e spec for badge states
                      4.2 registry entries + make registry-generate
```

Phases 1 and 2 are independent and can be implemented in parallel; Phase 3 depends on
`GitHubCheckConclusion` semantics being final (it is, already, in production) but not on
Phases 1/2 landing first. Phase 4 depends on all of 1–3 being complete (tests reference
behavior from all three).

---

## Phase 1: Auto-Approve Rule Engine — `ci_passing` Condition

### Epic 1.1: Thread CI status through the classifier
**Goal**: Make `Instance.GitHubCheckConclusion` visible to `matchesRule` as a new,
composable, opt-in condition, without changing behavior for any existing rule.

#### Story 1.1.1: Add `RequireCIPassing` to `Rule` and thread `ClassificationContext` into matching
**As a** rule author, **I want** a rule to be able to require green CI, **so that** I can
combine it with existing conditions (e.g. a command-pattern regex) per AC6's example.
**Acceptance Criteria**:
- AC6: The auto-approve rule engine supports a `ci_passing` condition combinable with
  existing conditions.
  - *Given* a `Rule{ CommandPattern: regexp.MustCompile("^npm publish"), RequireCIPassing: true, Decision: AutoAllow }` loaded into `RuleBasedClassifier`, and `ClassificationContext{ CIStatus: "success" }`.
  - *When* `Classify(payload, ctx)` is called with `payload.ToolInput["command"] = "npm publish"`.
  - *Then* `matchesRule` returns `true` only because both the regex matched AND `ctx.CIStatus == "success"`; the same rule against `ctx.CIStatus == "failure"` (or `""`) returns `false` from `matchesRule`.
**Files**: `pkg/classifier/classifier.go`

##### Task 1.1.1a: Add `RequireCIPassing bool` field to `Rule` (~2 min)
- Add the field with a doc comment (mirrors `FilePattern`'s comment style) to the `Rule` struct at `pkg/classifier/classifier.go:343-369`.
- Files: `pkg/classifier/classifier.go`

##### Task 1.1.1b: Add `CIStatus string` field to `ClassificationContext` (~2 min)
- Add to the struct at `pkg/classifier/classifier.go:61-72`, doc comment stating the vocabulary (`success`/`failure`/`pending`/`neutral`/`""`, `""` meaning "no PR or unknown").
- Files: `pkg/classifier/classifier.go`

##### Task 1.1.1c: Thread `ctx ClassificationContext` through `classifySingle`/`matchesRule`/`classifyOneSubCmd` (~5 min)
- Change `classifySingle(payload)` → `classifySingle(payload, ctx)` (`:506`), update its one call site in `classifyInternal` (`:502`) and the one in `classifyOneSubCmd` (`:550`) to pass `ctx` through.
- Change `matchesRule(rule, payload)` → `matchesRule(rule, payload, ctx)` (`:679`), update its call site inside `classifySingle` (`:511`).
- Files: `pkg/classifier/classifier.go`

##### Task 1.1.1d: Implement the `RequireCIPassing` check in `matchesRule` (~3 min)
- After the existing `FilePattern` check (`:718-723`), add: `if rule.RequireCIPassing && ctx.CIStatus != "success" { return false }`.
- Files: `pkg/classifier/classifier.go`

#### Story 1.1.2: Populate `ClassificationContext.CIStatus` from the requesting session
**As a** reviewer, **I want** `ci_passing` to reflect the actual session's branch, **so
that** the condition means something (not a global/unscoped flag).
**Acceptance Criteria**:
- *Given* `ApprovalHandler` already resolves `sessionID` (`server/services/approval_handler.go:200`) and holds `storage *session.Storage` (`:70`).
- *When* `HandlePermissionRequest` builds `classCtx := h.classifier.BuildContext(payload.Cwd)` (`:282`).
- *Then* it additionally looks up `h.storage.FindInstanceDataByID(sessionID)` and sets `classCtx.CIStatus = data.GitHubCheckConclusion` only when `data.GitHubPRNumber > 0`; otherwise `CIStatus` stays `""`.
**Files**: `server/services/approval_handler.go`

##### Task 1.1.2a: Populate `CIStatus` before `Classify` is called (~5 min)
- In `HandlePermissionRequest`, immediately after `classCtx := h.classifier.BuildContext(payload.Cwd)` (`server/services/approval_handler.go:282`), add the `FindInstanceDataByID` lookup and conditional assignment described above. Handle lookup error/nil gracefully (leave `CIStatus` as `""`, matching this handler's existing best-effort error style elsewhere in the function).
- Files: `server/services/approval_handler.go`

#### Story 1.1.3: Persist `RequireCIPassing` through `RuleSpec` and the wire proto
**As a** user, **I want** a `ci_passing`-requiring rule I create to survive a restart and round-trip through the RPC API, **so that** it behaves like every other rule field.
**Acceptance Criteria**:
- *Given* a `RuleSpec{ RequireCIPassing: true, ... }` persisted to `auto_approve_rules.json` via `RulesStore.Upsert`.
- *When* the server restarts and `RulesStore.ToRules()` rebuilds the classifier.
- *Then* the resulting `classifier.Rule` has `RequireCIPassing == true`, and `UpsertApprovalRule`/`ListApprovalRules` round-trip the same value through `ApprovalRuleProto.require_ci_passing`.
**Files**: `server/services/rules_store.go`, `proto/session/v1/types.proto`, `server/services/rules_service.go`

##### Task 1.1.3a: Add `RequireCIPassing` to `RuleSpec` and its `classifier.Rule` conversion (~4 min)
- Add `RequireCIPassing bool \`json:"require_ci_passing,omitempty"\`` to `RuleSpec` (`server/services/rules_store.go:22-48`).
- Thread it through `specsToRules` (`:272` area, mirrors the existing `SafePythonImportsOnly` passthrough at `:145`) and `ToRules` (`:83`).
- Files: `server/services/rules_store.go`

##### Task 1.1.3b: Add `require_ci_passing = 29` to `ApprovalRuleProto` (~3 min)
- Add the field to `proto/session/v1/types.proto:1076-1105` (next available number after `tool_category = 28`).
- Run `make proto-gen` to regenerate `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`.
- Files: `proto/session/v1/types.proto`, generated files.

##### Task 1.1.3c: Wire `RequireCIPassing` through `rules_service.go`'s mapping functions (~5 min)
- Update all 3 existing `SafePythonImportsOnly`-adjacent mapping sites to also carry `RequireCIPassing`: the proto→`RuleSpec` construction in `UpsertApprovalRule` (`server/services/rules_service.go:95-119`), `specToProto` (`:446-476`), and `ruleToSpec` (`:478-513`).
- Files: `server/services/rules_service.go`

#### Story 1.1.4: Expose `RequireCIPassing` in the rule-builder UI
**As a** rule author, **I want** a checkbox to require CI passing when creating/editing a rule, **so that** AC6 is reachable from the app, not only via direct JSON editing.
**Acceptance Criteria**:
- *Given* the `RuleBuilderForm` in structured-criteria mode.
- *When* a user checks "Require CI passing" and saves.
- *Then* the submitted payload includes `requireCiPassing: true`, following the exact state/submit pattern already used for `safePythonImportsOnly` (`web-app/src/components/rules/RuleBuilderForm.tsx:108,161,226,269,315,458`).
**Files**: `web-app/src/components/rules/RuleBuilderForm.tsx`

##### Task 1.1.4a: Add the "Require CI passing" checkbox (~5 min)
- Mirror the `safePythonImportsOnly` state/checkbox/submit-payload pattern at the 6 cited line locations in `RuleBuilderForm.tsx` for a new `requireCiPassing` field.
- Files: `web-app/src/components/rules/RuleBuilderForm.tsx`

#### Story 1.1.5: Unit test coverage for `ci_passing`
**Acceptance Criteria**:
- AC8 (partial — classifier half): unit tests exist for the new condition.
**Files**: `pkg/classifier/classifier_test.go`

##### Task 1.1.5a: Add `TestClassify_RequireCIPassing_*` tests (~5 min)
- Following the existing `TestClassify_*` naming convention (`pkg/classifier/classifier_test.go:8` onward), add:
  `TestClassify_RequireCIPassing_Success_AutoAllow` (CIStatus "success" → matches),
  `TestClassify_RequireCIPassing_Failure_Escalate` (CIStatus "failure" → falls through to Escalate),
  `TestClassify_RequireCIPassing_NoPR_Escalate` (CIStatus "" → falls through).
- Files: `pkg/classifier/classifier_test.go`

##### Task 1.1.5b: Add an AND-composition test matching AC6's literal example (~4 min)
- `TestClassify_RequireCIPassing_CommandPatternAnd_BothMustMatch`: a rule with both `CommandPattern` and `RequireCIPassing: true`; assert it only matches when both conditions hold (regex matches AND CIStatus == "success"), and fails when either alone is true.
- Files: `pkg/classifier/classifier_test.go`

---

## Phase 2: Block Manual Approve on CI Red

### Epic 2.1: Feature flag registration
**Goal**: Give AC5's "configurable rule (default: off)" a real, discoverable, zero-new-UI settings surface by reusing the existing generic feature-flag system.

#### Story 2.1.1: Register `review:block-approval-on-ci-failure`
**Acceptance Criteria**:
- AC5 (partial — flag exists, defaults off): a feature flag exists and is visible in Settings → Feature Flags, defaulting to off.
  - *Given* the flag has never been set.
  - *When* `Config.GetFeatureFlag("review:block-approval-on-ci-failure")` is called.
  - *Then* it returns `false` (unset flags default false per `config/config.go:999-1004`).
**Files**: `server/services/feature_flag_service.go`, `web-app/src/app/settings/features/page.tsx`

##### Task 2.1.1a: Add the flag constant and registry entry (~3 min)
- Add `blockApprovalOnCIFailureFlagName = "review:block-approval-on-ci-failure"` (mirrors `sddDefaultPipelineFlagName`, `server/services/feature_flag_service.go:18`) and an entry in `knownFeatureFlags` (`:22-46`) with a description explaining the block behavior and its default-off/human-override nature.
- Files: `server/services/feature_flag_service.go`

##### Task 2.1.1b: Add a friendly label in the settings UI (~2 min)
- Add an entry to `FEATURE_META` (`web-app/src/app/settings/features/page.tsx:24-27`), e.g. `{ label: "Block Approve when CI is failing" }`. No other frontend change needed — the page renders `knownFeatureFlags` generically.
- Files: `web-app/src/app/settings/features/page.tsx`

### Epic 2.2: Gate `ResolveApproval` on CI-red
**Goal**: Implement the actual block, visibly explained, with sessions lacking a PR unaffected (AC7).

#### Story 2.2.1: Wire session/instance lookup into `ApprovalService`
**Acceptance Criteria**:
- *Given* `ApprovalService` currently holds only `approvalStore`/`notificationStore`/`eventBus` (`server/services/approval_service.go:17-22`).
- *When* `NewSessionService` constructs it.
- *Then* `ApprovalService` also holds `storage *session.Storage`, set via a new `SetStorage` method, wired at the same construction site as `SetEventBus` (`server/services/session_service.go:284-285`).
**Files**: `server/services/approval_service.go`, `server/services/session_service.go`

##### Task 2.2.1a: Add `storage` field + `SetStorage` setter, wire at construction (~4 min)
- Add `storage *session.Storage` field to `ApprovalService` struct (`server/services/approval_service.go:17-22`) and a `SetStorage(s *session.Storage)` method (mirrors `SetEventBus`, `:37-40`).
- Call `approvalSvc.SetStorage(concStorage)` immediately after `approvalSvc.SetEventBus(eventBus)` in `server/services/session_service.go:284-285`.
- Files: `server/services/approval_service.go`, `server/services/session_service.go`

#### Story 2.2.2: Implement the block with a visible inline explanation
**Acceptance Criteria**:
- AC5: manual Approve is blocked when CI is failing and the flag is on, with a visible explanation (not a silent no-op).
  - *Given* the flag `review:block-approval-on-ci-failure` is `true`, and the pending approval's session has `InstanceData.GitHubPRNumber = 42`, `GitHubCheckConclusion = "failure"`.
  - *When* the web UI calls `ApprovalService.ResolveApproval` with `Decision: "allow"` for that approval.
  - *Then* `ResolveApproval` returns `connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("CI is failing on this branch — review before approving"))` *before* calling `approvalStore.Resolve`, and `NotificationPanel.tsx`'s Approve button surfaces this message inline next to the button instead of collapsing the error into the generic "expired" state.
- AC7: sessions without a PR are unaffected.
  - *Given* `InstanceData.GitHubPRNumber == 0` for the approval's session.
  - *When* `ResolveApproval` is called with `Decision: "allow"` and the flag is on.
  - *Then* the block check is skipped entirely (short-circuits on `GitHubPRNumber == 0` before reading `GitHubCheckConclusion`), and `approvalStore.Resolve` proceeds normally.
**Files**: `server/services/approval_service.go`, `web-app/src/components/ui/NotificationPanel.tsx`

##### Task 2.2.2a: Add the CI-red guard clause in `ResolveApproval` (~5 min)
- Before `as.approvalStore.Resolve(...)` (`server/services/approval_service.go:73`), when `req.Msg.Decision == "allow"` and `config.LoadConfig().GetFeatureFlag(blockApprovalOnCIFailureFlagName)`: look up `as.storage.FindInstanceDataByID(sessionID)` (using the `sessionID` already resolved at `:68-69`); if `data.GitHubPRNumber > 0 && data.GitHubCheckConclusion == "failure"`, return the `CodeFailedPrecondition` error described above and log the block (mirrors the existing `log.Info` at `:97`).
- Files: `server/services/approval_service.go`

##### Task 2.2.2b: Surface the block reason inline in the Approve button UI (~5 min)
- In `resolveApproval` (`web-app/src/components/ui/NotificationPanel.tsx:162-176`), on a caught error, inspect the ConnectRPC error code; if it is `FailedPrecondition`, store the error's message keyed by `approvalId` (new state, e.g. `blockedApprovals`) instead of falling into the generic `"expired"` branch, and render it as inline text near the Approve/Deny buttons (`:461-481`) rather than silently disabling them (per AC5, "visibly explained... not a silent no-op").
- Files: `web-app/src/components/ui/NotificationPanel.tsx`

#### Story 2.2.3: Unit test coverage for the block
**Acceptance Criteria**:
- AC8 (partial — approval-block half): the block behavior has test coverage.
**Files**: `server/services/approval_service_test.go`

##### Task 2.2.3a: Add `ResolveApproval` block tests (~5 min)
- `TestResolveApproval_BlocksOnFailingCI_WhenFlagEnabled`, `TestResolveApproval_AllowsOnFailingCI_WhenFlagDisabled`, `TestResolveApproval_UnaffectedWhenNoPR` — following the existing test setup pattern in `server/services/approval_service_test.go` (`NewApprovalService(store)` calls at lines 24/54/79/105/120/351).
- Files: `server/services/approval_service_test.go`

---

## Phase 3: Diff-Viewer CI Badge + Real-Time Freshness Fix

### Epic 3.1: Wire the diff viewer to existing CI-status data
**Goal**: Satisfy AC1/AC2/AC3/AC7 by reading the already-flowing `Session.githubCheckConclusion` in a new UI location — no new fetch code.

#### Story 3.1.1: Pass `session` into `DiffViewer`
**Acceptance Criteria**:
- *Given* `DiffViewer`'s current `DiffViewerProps` is an empty interface (`web-app/src/components/sessions/DiffViewer.tsx:6-8`) and `SessionDetailView.tsx:792` renders `<DiffViewer />` with no props, even though `session` is already in scope there (used for the adjacent `GitHubBadge` at `:470-482` and the info panel at `:1157-1163`).
- *When* this story lands.
- *Then* `<DiffViewer session={session} />` is passed the same `Session` object `VcsPanel` already receives (`web-app/src/components/sessions/VcsPanel.tsx:14`, destructured at `:17`).
**Files**: `web-app/src/components/sessions/DiffViewer.tsx`, `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 3.1.1a: Extend `DiffViewerProps` to accept `session` (~3 min)
- Replace the empty `DiffViewerProps` interface with `{ session?: Session }` (import `Session` from `@/gen/session/v1/types_pb`, matching `VcsPanel.tsx:1-2`).
- Files: `web-app/src/components/sessions/DiffViewer.tsx`

##### Task 3.1.1b: Update the call site to pass `session` (~2 min)
- Change `<DiffViewer />` to `<DiffViewer session={session} />` at `web-app/src/components/sessions/SessionDetailView.tsx:792`.
- Files: `web-app/src/components/sessions/SessionDetailView.tsx`

#### Story 3.1.2: Render the CI status badge in the diff viewer header
**As a** reviewer, **I want** to see CI status without leaving the diff viewer, **so that** I don't have to separately tab over to GitHub Actions before approving.
**Acceptance Criteria**:
- AC1: badge shows passing/failing/pending/no-checks for the session's branch when it has an associated PR.
  - *Given* `session.githubPrNumber = 42`, `session.githubPrUrl = "https://github.com/acme/widgets/pull/42"`, `session.githubCheckConclusion = "failure"`.
  - *When* the diff tab renders.
  - *Then* `CIStatusBadge` renders with `role="status"`, text "Failing", an `❌` icon, CSS class `prBadgeBlocking` (imported from `GitHubBadge.css.ts`), and `aria-label="CI status: Failing"`.
- AC2: badge links to the Actions run/check page.
  - *Given* the same session.
  - *When* `CIStatusBadge` renders.
  - *Then* its anchor `href` is `"https://github.com/acme/widgets/pull/42/checks"`, opening in a new tab (`target="_blank" rel="noopener noreferrer"`, mirroring `GitHubBadge.tsx`'s `handleClick`).
- AC3: no new fetch code.
  - *Given* `CIStatusBadge` only reads props derived from the `session` prop.
  - *When* implemented.
  - *Then* no `fetch`/RPC call is added inside `CIStatusBadge` or `DiffViewer` — it is purely presentational over already-delivered data.
- AC7: no PR → no badge.
  - *Given* `session.githubPrNumber = 0`.
  - *When* `CIStatusBadge` renders.
  - *Then* it returns `null` (no chip, not an error state).
**Files**: `web-app/src/components/sessions/CIStatusBadge.tsx` (new), `web-app/src/components/sessions/DiffViewer.tsx`

##### Task 3.1.2a: Create `CIStatusBadge.tsx` (~5 min)
- New file. Props: `checkConclusion?: string`, `prUrl?: string`, `prNumber?: number`, `lastChecked?: Timestamp` (from `@bufbuild/protobuf/wkt`).
- Return `null` when `!prNumber`. Otherwise map `checkConclusion` → `{success: "Passing"/✅/prBadgeReady, failure: "Failing"/❌/prBadgeBlocking, pending: "Pending"/⏳/prBadgePending, default("" or "neutral"): "No checks"/prBadgeUnknown}` (import variant classes from `./GitHubBadge.css`).
- `href = prUrl ? \`${prUrl}/checks\` : undefined`. Tooltip/`title` includes staleness via `formatRelativeTime(timestampDate(lastChecked).getTime())` when `lastChecked` is set (precedent: `web-app/src/app/logs/page.tsx:8,325`).
- `role="status"`, `aria-label="CI status: <label>"` (precedent: `StatusBadge.tsx:94-104`).
- Files: `web-app/src/components/sessions/CIStatusBadge.tsx`

##### Task 3.1.2b: Render `CIStatusBadge` in `DiffViewer`'s header (~4 min)
- In `DiffViewer.tsx`, above/alongside `<DiffRenderer>`, render `<CIStatusBadge checkConclusion={session?.githubCheckConclusion} prUrl={session?.githubPrUrl} prNumber={session?.githubPrNumber} lastChecked={session?.lastPrStatusCheck} />`.
- Files: `web-app/src/components/sessions/DiffViewer.tsx`

#### Story 3.1.3: Unit tests for `CIStatusBadge`
**Acceptance Criteria**:
- AC8 (partial — badge unit-test half).
**Files**: `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx` (new)

##### Task 3.1.3a: Add `CIStatusBadge.test.tsx` (~5 min)
- Following `StatusBadge.test.tsx`'s RTL pattern: assert rendered text/class/`aria-label` for `success`/`failure`/`pending`/`""` conclusions, assert `href` construction, and assert `null` render when `prNumber` is `0`/undefined.
- Files: `web-app/src/components/sessions/__tests__/CIStatusBadge.test.tsx`

### Epic 3.2: Fix the "priority-only" event-publish gap
**Goal**: Close the AC4 gap identified in research — `onUpdated` today only fires when derived PR priority changes, so a CI conclusion flip that doesn't cross a priority boundary can leave the diff-viewer badge stale between ticks.

#### Story 3.2.1: Publish on CI-conclusion change, not just priority change
**Acceptance Criteria**:
- AC4: CI status updates reach the frontend via existing `WatchSessions`, promptly on change.
  - *Given* `PRStatusPoller.applyPRUpdate` currently gates `onUpdated(inst)` on `result.PriorityChanged` only.
  - *When* a poll tick observes `GitHubCheckConclusion` change from `"pending"` to `"failure"` with no `PRPriority` boundary crossed.
  - *Then* `onUpdated(inst)` still fires (new `checkConclusion`-changed branch, compared against the previously-stored value before the update), publishing `events.NewSessionUpdatedEvent(inst, []string{"github_pr_priority", "github_pr_state", "github_check_conclusion"})` (`server/dependencies.go:585-587`) — and when neither priority nor conclusion changed, no event is published (changed-only-publish, per `research/pitfalls.md` §5).
**Files**: `session/pr_status_poller.go`, `server/dependencies.go`

##### Task 3.2.1a: Add a checkConclusion-changed branch to `applyPRUpdate` (~5 min)
- In `session/pr_status_poller.go`'s `applyPRUpdate`, capture the pre-update `GitHubCheckConclusion` and compare to the newly-fetched value; OR it into the existing `onUpdated` trigger condition alongside `result.PriorityChanged`.
- Files: `session/pr_status_poller.go`

##### Task 3.2.1b: Add `"github_check_conclusion"` to the `updatedFields` hint list (~2 min)
- Update `server/dependencies.go:585-587`'s `NewSessionUpdatedEvent(inst, []string{"github_pr_priority", "github_pr_state"})` call to include `"github_check_conclusion"`.
- Files: `server/dependencies.go`

#### Story 3.2.2: Unit test for the change-detection fix
**Files**: `session/pr_status_poller_test.go`

##### Task 3.2.2a: Add a checkConclusion-only-change test (~5 min)
- `TestApplyPRUpdate_FiresOnUpdated_WhenCheckConclusionChangesWithoutPriorityChange` — assert `onUpdated` is called when only `GitHubCheckConclusion` changes, and NOT called when neither priority nor conclusion changed (changed-only-publish regression guard).
- Files: `session/pr_status_poller_test.go`

---

## Phase 4: E2E Coverage + Feature Registry

### Epic 4.1: E2E badge-rendering coverage
**Goal**: Satisfy AC8's e2e requirement per `.claude/rules/e2e-test-conventions.md`.

#### Story 4.1.1: Badge state e2e spec
**Acceptance Criteria**:
- AC8: e2e test covers CI badge rendering states.
  - *Given* a test-mode session fixture with `githubCheckConclusion` set to each of `success`/`failure`/`pending`/`""`, and a fixture with no PR.
  - *When* the diff tab is opened in the e2e harness.
  - *Then* `page.getByTestId("ci-status-badge")` is visible with the expected text for the first four, and absent (not just empty) for the no-PR fixture.
**Files**: `tests/e2e/pages/SessionDetailPage.ts`, `tests/e2e/ci-status-badge.spec.ts` (new)

##### Task 4.1.1a: Add `data-testid="ci-status-badge"` and a page-helper accessor (~3 min)
- Add the `data-testid` to `CIStatusBadge.tsx`'s root element (Task 3.1.2a).
- Add `getCIStatusBadge()` to `tests/e2e/pages/SessionDetailPage.ts` (mirrors the existing `getByTestId` accessor pattern at `:34,38,42,46`).
- Files: `web-app/src/components/sessions/CIStatusBadge.tsx`, `tests/e2e/pages/SessionDetailPage.ts`

##### Task 4.1.1b: Write `tests/e2e/ci-status-badge.spec.ts` (~5 min)
- Header: `// @feature session:ci-status-badge`. No `waitForTimeout` — use `expect(locator).toHaveText(...)`/`toBeVisible()`/`not.toBeVisible()`. Locators via `data-testid`/ARIA only, per `.claude/rules/e2e-test-conventions.md`.
- Files: `tests/e2e/ci-status-badge.spec.ts`

### Epic 4.2: Feature registry
**Goal**: Satisfy AC10 per `.claude/rules/feature-registry.md`.

#### Story 4.2.1: Register new/modified features
**Acceptance Criteria**:
- AC10: feature registry entries exist and `make registry-generate` shows no unexplained coverage-gap increase.
**Files**: `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/*.json`

##### Task 4.2.1a: Register the `ci_passing` rule condition (~2 min)
- Create `docs/registry/features/backend/ci-passing-rule-condition.json` per `docs/registry/schema.json`, `tested: true`, `testIds` populated from Task 1.1.5a/1.1.5b's test function names.
- Files: `docs/registry/features/backend/ci-passing-rule-condition.json`

##### Task 4.2.1b: Register the block-on-CI-failure gate (~2 min)
- Create `docs/registry/features/backend/block-approval-on-ci-failure.json`, `tested: true`, `testIds` from Task 2.2.3a.
- Files: `docs/registry/features/backend/block-approval-on-ci-failure.json`

##### Task 4.2.1c: Register the diff-viewer CI badge (~2 min)
- Create `docs/registry/features/frontend/ci-status-badge.json`, `filePath: "web-app/src/components/sessions/CIStatusBadge.tsx"`, `tested: true`, `testIds` from Tasks 3.1.3a and 4.1.1b.
- Files: `docs/registry/features/frontend/ci-status-badge.json`

##### Task 4.2.1d: Regenerate and verify (~3 min)
- Run `make registry-generate`; diff `docs/registry/coverage-gaps.json` before/after and confirm no unexplained increase; commit the regenerated aggregate files alongside the per-feature sources.
- Files: `docs/registry/coverage-gaps.json`, `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`
