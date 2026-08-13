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
| `HasGitHubPR()` | Existing `Instance` method; `true` when the session has a linked GitHub PR (`i.Snapshot().GitHub.GitHubPRNumber > 0`). Not called by this plan's new code. | `session/instance.go:739-741`. Operates on `*Instance` via `Snapshot()`, not `*InstanceData` — the two new gate call sites (Task 1.1.2a, Task 2.2.2a) work from `*InstanceData` (returned by `FindInstanceDataByID`), which has no equivalent method, so both intentionally use the inline literal `data.GitHubPRNumber > 0` instead. Two independent inline copies of a one-line comparison is judged not worth a shared helper for a plan this size — flag during implementation if a third call site appears. (Corrected from an earlier, incorrect `HasAssociatedPR()` citation.) |
| `FindInstanceDataByID` | Existing `Storage` lookup used by both new gates to resolve a session's CI status from a session ID. Returns `(*InstanceData, error)`; errors/not-found and a nil `Storage` are both handled explicitly by each call site — see Task 1.1.2a and Task 2.2.2a. | `session/storage.go:407`. |
| `ciConclusionSuccess`, `ciConclusionFailure` | New unexported string constants (`"success"`, `"failure"`) for the two conclusion values this feature compares against, colocated with `Rule` in `pkg/classifier/classifier.go` and with the guard in `server/services/approval_service.go`. Replaces literal-string comparisons in Task 1.1.1d and Task 2.2.2a. | New. Does not introduce a wire-format newtype — `GitHubCheckConclusion`/`ClassificationContext.CIStatus` remain plain `string`, matching the existing 3-fetcher convention (see Pattern Decisions table, "CI-status normalization" row). |
| `overrideCiBlock` | New `bool` field on `ResolveApprovalRequest` (proto field 4); when `true`, a reviewer is explicitly re-submitting an already-blocked approval with acknowledgment that CI is red. Server skips the AC5 block check when set, and logs the resolution distinctly from a normal approval. | New — `proto/session/v1/session.proto:1244-1254`. See Story 2.2.4. |
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
| Diff-viewer CI badge | (same as above) | | (c) Render `VcsWidgetGithubRow.tsx` with `showPrLink={false}` instead of a new component | `VcsWidgetGithubRow.tsx:8-19` already has a `showPrLink?: boolean` prop built for exactly the "suppress duplicate PR chip, keep CI span" scenario this row cites, and `research/architecture.md`'s Gap Analysis names it as the preferred reuse target. Evaluated and rejected for this specific need: (1) it takes `VcsWidgetData` (`VcsWidgetGithubRow.tsx:8`), not the `Session` proto object `DiffViewer` already receives — reuse would require a new `fromSessionVcs()` adapter, itself new code; (2) its CI-conclusion rendering is plainer text (`CI: {conclusion}`, `VcsWidgetGithubRow.tsx:81-83`) with no icon and no staleness tooltip, while AC1's Given-When-Then (Story 3.1.2) requires specific text ("Failing"/"Passing"/"Pending"/"No checks"), an icon (❌/✅/⏳), and `GitHubBadge.css.ts`'s `prBadgeBlocking`/`prBadgeReady`/etc. variant classes plus a staleness-aware tooltip (Task 3.1.2a) — a real visual/behavioral difference, not a cosmetic one. Reusing `VcsWidgetGithubRow` would mean either changing its shared rendering (risking the 3 existing call sites: `VcsPanel.tsx`, `UnfinishedItemDetail.tsx`, `BacklogItemDetail`) or building the adapter anyway, for no less total new code than `CIStatusBadge.tsx`. Kept as a new sibling component; this row makes the comparison explicit and traceable per the architecture review, rather than a silently-dropped alternative. |
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
- `overrideCiBlock` adds one new proto field (`ResolveApprovalRequest.override_ci_block =
  4`, see Story 2.2.4). Proto3 default `false` means every existing/in-flight
  `ResolveApprovalRequest` (from clients built before this change) is unaffected — it
  behaves exactly as it does today, since the AC5 guard only skips when the field is
  explicitly set `true`. No data migration; `make proto-gen` regenerates bindings.

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

None blocking. Three deliberate scope decisions are recorded rather than left open:
1. Whether to add a 5th "CI status unavailable / fetch error" badge state — deferred; no
   backend signal exists to drive it yet (see table row "Badge state vocabulary").
2. Whether `ci_passing` should force a **synchronous** CI re-fetch at rule-evaluation time
   instead of trusting the poller cache — still deferred; `research/architecture.md` §3
   frames this as optional and not required by any AC. This plan does, however, no longer
   accept the cache *unbounded* — Task 1.1.2b adds a bounded-staleness guard (treat
   `CIStatus` as unknown, not "success," once `LastPRStatusCheck` is older than
   `2 * PollInterval`) specifically because `ci_passing` gates an irreversible auto-approve
   action (see Story 1.1.2, AC6). A synchronous re-fetch remains a possible future
   escalation if false-positive auto-approvals are still observed in practice with the
   staleness guard in place.
3. `research/pitfalls.md` §2's SHA-mismatch/force-push case — the diff viewer renders the
   *local* worktree while the CI badge reflects the *remote* PR head, and these can
   silently disagree (e.g. after a force-push the badge may show stale CI for a SHA no
   longer checked out). Explicitly deferred, not implemented: no local-HEAD-vs-badge-SHA
   comparison is added in this plan. Revisit if this proves confusing in practice.

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
  1.5 Unit tests                                              2.5 Override ("Approve anyway") — Story 2.2.4
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
- Add an unexported constant `ciConclusionSuccess = "success"` colocated with the `Rule`
  struct (avoids a bare literal string at the one comparison site in this package; mirrors
  `ciConclusionFailure` added alongside the guard in `approval_service.go`, Task 2.2.2a).
- After the existing `FilePattern` check (`:718-723`), add: `if rule.RequireCIPassing && ctx.CIStatus != ciConclusionSuccess { return false }`.
- Files: `pkg/classifier/classifier.go`

##### Task 1.1.1e: Run the existing classifier test suite as a regression gate (~2 min)
- After Tasks 1.1.1a–1.1.1d land, run `go test ./pkg/classifier/...` and confirm zero
  regressions in pre-existing tests before considering Story 1.1.1 done. This is the
  highest-blast-radius change in the plan — `matchesRule`/`classifySingle` are evaluated
  for every tool-call classification in the app — so this is an explicit verification
  gate, not an assumption.
- Files: none (verification only)

#### Story 1.1.2: Populate `ClassificationContext.CIStatus` from the requesting session
**As a** reviewer, **I want** `ci_passing` to reflect the actual session's branch, **so
that** the condition means something (not a global/unscoped flag).
**Acceptance Criteria**:
- *Given* `ApprovalHandler` already resolves `sessionID` (`server/services/approval_handler.go:200`) and holds `storage *session.Storage` (`:70`).
- *When* `HandlePermissionRequest` builds `classCtx := h.classifier.BuildContext(payload.Cwd)` (`:282`).
- *Then* it additionally looks up `h.storage.FindInstanceDataByID(sessionID)` and sets `classCtx.CIStatus = data.GitHubCheckConclusion` only when `data.GitHubPRNumber > 0`; otherwise `CIStatus` stays `""`.
- AC6 stale-CI mitigation (closes adversarial-review.md Blocker 3 — "AC6's stale-CI race
  has no mitigation, despite gating an irreversible auto-approve action").
  - *Given* `data.GitHubCheckConclusion == "success"` but `data.LastPRStatusCheck` is older
    than `2 * pollInterval` (the poller's configured interval — see Task 1.1.2b).
  - *When* `classCtx.CIStatus` is populated.
  - *Then* it is set to `""` (unknown), not `"success"` — a rule with `RequireCIPassing:
    true` falls through to Escalate rather than auto-allowing on a conclusion that may no
    longer reflect the branch's real CI state.
**Files**: `server/services/approval_handler.go`

##### Task 1.1.2a: Populate `CIStatus` before `Classify` is called (~5 min)
- In `HandlePermissionRequest`, immediately after `classCtx := h.classifier.BuildContext(payload.Cwd)` (`server/services/approval_handler.go:282`), add the `FindInstanceDataByID` lookup and conditional assignment described above. Handle lookup error/nil gracefully (leave `CIStatus` as `""`, matching this handler's existing best-effort error style elsewhere in the function).
- Files: `server/services/approval_handler.go`

##### Task 1.1.2b: Add a bounded-staleness guard before trusting `CIStatus` (~5 min)
- Immediately after Task 1.1.2a's assignment of `classCtx.CIStatus = data.GitHubCheckConclusion`,
  add: if `time.Since(data.LastPRStatusCheck) > 2 * h.pollInterval`, reset `classCtx.CIStatus
  = ""` (treat as unknown) regardless of what `GitHubCheckConclusion` says.
- **Decision (resolved — pre-mortem.md Failure #1 P1)**: thread the poller's live configured
  interval into `ApprovalHandler`, not a hardcoded literal. Add `pollInterval time.Duration`
  to `ApprovalHandler`'s struct fields, set at construction from the same
  `PRStatusPollerConfig.PollInterval` value the running `PRStatusPoller` is built with
  (`session/pr_status_poller.go:26`, defaulting to `60 * time.Second` per
  `session/pr_status_poller.go:41`) — mirroring how `storage` is already threaded in as a
  constructor field (Task 1.1.2a). A second, disconnected `60 * time.Second` literal in
  `approval_handler.go` is not acceptable: it would silently desync from the real poller
  interval if that's ever tuned, undermining the exact freshness guarantee this guard exists
  to provide for an irreversible auto-approve gate. Task 1.1.2c's test must assert the guard
  reads `h.pollInterval` (constructed from the real config), not a duplicated constant.
- Rationale: unlike the classifier's other, synchronous/local conditions (regex, file
  pattern), CI status is inherently async/network-sourced with no fresher local value to
  fall back to (`research/pitfalls.md` §4) — and `ci_passing` gates an irreversible
  auto-allow action (AC6's own worked example is `^npm publish`). Treating a
  too-old-to-trust conclusion as unknown fails closed (falls through to Escalate) rather
  than risking an auto-approve on data that may no longer reflect the branch's current
  head.
- Files: `server/services/approval_handler.go`

##### Task 1.1.2c: Add a unit test for the staleness guard (~3 min)
- The staleness guard lives in `approval_handler.go`, not in `pkg/classifier` —
  `matchesRule` only ever sees whatever `ClassificationContext.CIStatus` it's handed; it has
  no concept of "stale." Add `TestHandlePermissionRequest_StaleCIStatus_TreatedAsUnknown` to
  `server/services/approval_handler_test.go`: given `data.LastPRStatusCheck` older than
  `2 * pollInterval` and `data.GitHubCheckConclusion == "success"`, assert the `classCtx`
  passed to `Classify` has `CIStatus == ""`, not `"success"` — i.e. a stale-but-cached
  "success" cannot silently satisfy a `RequireCIPassing` rule. (Story 1.1.5 cross-references
  this test as the concrete coverage for the stale-CI mitigation named in its own AC list —
  see Task 1.1.5c.)
- Files: `server/services/approval_handler_test.go`

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

##### Task 1.1.3d: Confirm AC9 — no session-creation-registry touchpoint is introduced (~2 min, verification only)
- AC9: "session creation/session-type registry touchpoints (`.claude/rules/session-creation-registry.md`) are unaffected — confirm no new session type is introduced." This entire feature (Phase 1's `RequireCIPassing` classifier condition, Phase 2's approval-block/override, Phase 3's read-only diff-viewer badge) adds zero new `SessionType` proto enum values, zero new `CreateSessionRequest` fields, and touches none of the 7 registry touchpoints in `.claude/rules/session-creation-registry.md` — it only reads `Instance`/`InstanceData` fields (`GitHubPRNumber`, `GitHubCheckConclusion`, `LastPRStatusCheck`) that already exist on every session type. Verify by grepping the diff for `SESSION_TYPE_` and `sessionTypeMap` before considering this plan done — a hit in either would mean scope crept into session-creation and this AC has been violated.
- Files: none (verification only)

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
- AC6 stale-CI mitigation coverage: the staleness guard added in Story 1.1.2/Task 1.1.2b has
  a corresponding unit test (see Task 1.1.5c — the test itself lives outside
  `pkg/classifier` since the guard does).
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

##### Task 1.1.5c: Stale-CI-status test coverage (cross-reference, ~0 min — no new work here)
- `matchesRule`/`Classify` have no concept of staleness; they only ever see whatever
  `ClassificationContext.CIStatus` they're handed (verified by Task 1.1.5a's tests, which
  already cover "CIStatus == \"\" → falls through" via `..._NoPR_Escalate`). The actual
  staleness computation — treating an old `LastPRStatusCheck` as reason to reset `CIStatus`
  to `""` before it ever reaches the classifier — lives in `approval_handler.go` (Story
  1.1.2/Task 1.1.2b), so its test, `TestHandlePermissionRequest_StaleCIStatus_TreatedAsUnknown`,
  is specified and added under Task 1.1.2c in `server/services/approval_handler_test.go`,
  not here. This task exists only to make that cross-reference explicit, so Blocker 3's
  test-coverage requirement isn't satisfied by a misleading duplicate test asserting the
  same `CIStatus == ""` behavior `..._NoPR_Escalate` already covers.
- Files: none (see `server/services/approval_handler_test.go`, Task 1.1.2c)

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
- Add an unexported constant `ciConclusionFailure = "failure"` colocated with this guard (mirrors `ciConclusionSuccess` in `pkg/classifier/classifier.go`, Task 1.1.1d, per the Domain Glossary row); use it in place of the literal `"failure"` comparison above.
- **Error path (explicit, closes adversarial-review.md Blocker 2):**
  - **Nil `as.storage`**: guard it first — `if as.storage == nil { /* skip the block check entirely */ }` — and proceed to `approvalStore.Resolve` as if the flag were off. Fail open, not closed: this is a net-new, opt-in safety feature layered on top of the pre-existing approval flow, not a pre-existing invariant the flow already depended on, so a missing dependency should degrade to "feature quietly does nothing" rather than blocking (or panicking on) every approval in the workspace.
  - **Lookup error or not-found from `FindInstanceDataByID`** (e.g. the session was deleted between escalation and the human clicking Approve — a race this feature is newly capable of hitting, since `ApprovalService` never touched storage before this change): treat CI status as unknown and fail open — skip the block, let `approvalStore.Resolve` proceed normally. Do not return an error and do not treat "lookup failed" as equivalent to "CI is failing." One-sentence rationale: an approval flow should never hard-fail a human's explicit "Approve" click because of an infrastructure lookup miss that has nothing to do with the actual CI state.
- Files: `server/services/approval_service.go`

##### Task 2.2.2b: Surface the block reason inline in the Approve button UI (~5 min)
- In `resolveApproval` (`web-app/src/components/ui/NotificationPanel.tsx:162-176`), on a caught error, inspect the ConnectRPC error code; if it is `FailedPrecondition`, store the error's message keyed by `approvalId` (new state, e.g. `blockedApprovals`) instead of falling into the generic `"expired"` branch, and render it as inline text near the Approve/Deny buttons (`:461-481`) rather than silently disabling them (per AC5, "visibly explained... not a silent no-op").
- This inline text is not a dead end: Story 2.2.4/Task 2.2.4c adds an "Approve anyway" button next to it, so the block is always scoped-overridable rather than a pure stop sign.
- Files: `web-app/src/components/ui/NotificationPanel.tsx`

#### Story 2.2.3: Unit test coverage for the block
**Acceptance Criteria**:
- AC8 (partial — approval-block half): the block behavior has test coverage.
**Files**: `server/services/approval_service_test.go`

##### Task 2.2.3a: Add `ResolveApproval` block tests (~6 min)
- `TestResolveApproval_BlocksOnFailingCI_WhenFlagEnabled`, `TestResolveApproval_AllowsOnFailingCI_WhenFlagDisabled`, `TestResolveApproval_UnaffectedWhenNoPR`, `TestResolveApproval_FailsOpen_WhenStorageLookupErrors` (asserts a `FindInstanceDataByID` error/not-found does not block, does not panic, and `approvalStore.Resolve` still proceeds — see Task 2.2.2a's error-path spec) — following the existing test setup pattern in `server/services/approval_service_test.go` (`NewApprovalService(store)` calls at lines 24/54/79/105/120/351).
- Every new test above that exercises the CI-block path must explicitly call `SetStorage(...)` after `NewApprovalService(store)` (mirrors the existing `SetEventBus` call pattern) so it isn't silently exercising the nil-`storage` fail-open path by accident. `TestResolveApproval_FailsOpen_WhenStorageLookupErrors` is the one exception: it should call `SetStorage` with a storage double that deliberately returns an error/not-found from `FindInstanceDataByID`, not leave `storage` nil, so it tests the lookup-error branch specifically rather than the nil-storage branch. Confirm the 6 pre-existing `NewApprovalService(store)` call sites (lines 24/54/79/105/120/351) are unrelated to the CI-block feature and are therefore unaffected by Task 2.2.2a's nil-storage guard being fail-open — no changes needed to those sites.
- Files: `server/services/approval_service_test.go`

#### Story 2.2.4: Override the AC5 block with an audited "Approve anyway"
**As a** reviewer, **I want** to override the CI-red block for one specific session while
explicitly acknowledging red CI, **so that** I don't have to disable
`review:block-approval-on-ci-failure` workspace-wide just to approve the one session I've
reviewed and judged safe despite its failing CI (closes adversarial-review.md Blocker 1 —
"a hard block with no override is a foot-gun... re-introduces exactly the kind of hard gate
Goal 2 explicitly rejects").
**Acceptance Criteria**:
- *Given* the flag `review:block-approval-on-ci-failure` is `true`, and `ResolveApproval` has
  just returned `CodeFailedPrecondition` for a session with `GitHubPRNumber > 0` and
  `GitHubCheckConclusion == "failure"` (Story 2.2.2's block).
- *When* the reviewer clicks "Approve anyway" and the client re-submits `ResolveApproval`
  with `Decision: "allow"`, `OverrideCiBlock: true`.
- *Then* `ResolveApproval` skips the AC5 guard clause entirely for this request (does not
  re-evaluate `GitHubPRNumber > 0 && GitHubCheckConclusion == "failure"`), `approvalStore.Resolve`
  proceeds normally, and the resolution is logged with a line distinct from the existing
  `log.Info("[ApprovalService] resolved approval"...)` at `:97` — including
  `override_ci_block=true` and the CI conclusion at the time of override — so overrides are
  grep-able separately from ordinary approvals.
- *Given* `OverrideCiBlock: true` is sent on a request where the block would not have fired
  anyway (flag off, no PR, or CI passing).
- *When* `ResolveApproval` is called.
- *Then* behavior is unchanged from Story 2.2.2/2.2.3 (no distinct override log line is
  emitted, since nothing was actually overridden) — `OverrideCiBlock` is a no-op flag in
  this case, not a second code path with its own side effects.
**Files**: `proto/session/v1/session.proto`, `server/services/approval_service.go`,
`web-app/src/components/ui/NotificationPanel.tsx`

##### Task 2.2.4a: Add `override_ci_block = 4` to `ResolveApprovalRequest` (~2 min)
- Add `bool override_ci_block = 4;` to `ResolveApprovalRequest`
  (`proto/session/v1/session.proto:1244-1254`), with a doc comment: "When true, the caller
  explicitly acknowledges failing CI and re-submits an already-blocked approval; the server
  skips the AC5 CI-red guard for this request only."
- Run `make proto-gen` to regenerate `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/*_pb.ts`.
- Files: `proto/session/v1/session.proto`, generated files.

##### Task 2.2.4b: Skip only the block *decision* (not the lookup) when `OverrideCiBlock` is set (~4 min)
- Correction (adversarial review round 2): the lookup itself must always run — its result
  is what the distinct log line reports — only the *early-return* decision built on top of
  it is conditional on the override flag. Do not gate the whole guard clause (including the
  nil-storage/lookup-error fail-open paths from Task 2.2.2a) behind
  `if !req.Msg.OverrideCiBlock`; that would make the lookup itself conditional and leave
  nothing to log.
- In `ResolveApproval` (`server/services/approval_service.go`), keep Task 2.2.2a's lookup
  and its nil-storage/lookup-error fail-open handling unconditional. Change only the final
  decision: today it returns `CodeFailedPrecondition` when `data.GitHubPRNumber > 0 &&
  data.GitHubCheckConclusion == ciConclusionFailure`; change that branch to
  `if blocked := data.GitHubPRNumber > 0 && data.GitHubCheckConclusion ==
  ciConclusionFailure; blocked && !req.Msg.OverrideCiBlock { return
  connect.NewError(...) }` — i.e. `OverrideCiBlock` suppresses the *return*, not the lookup.
- When `blocked && req.Msg.OverrideCiBlock` (the override actually mattered — not a no-op
  flag sent on a passing/no-PR session), log a distinct line, e.g.
  `log.Info("[ApprovalService] approved despite failing CI (override)", "approval_id",
  req.Msg.ApprovalId, "session_id", sessionID, "ci_conclusion", data.GitHubCheckConclusion)`,
  separate from the existing `log.Info("[ApprovalService] resolved approval"...)` line at
  `:97`. When the lookup itself failed/was skipped (nil storage, not-found — Task 2.2.2a's
  fail-open paths), `blocked` is `false` by construction, so no override log line fires
  either — consistent with Story 2.2.4's second Given/When/Then ("no-op flag" case).
- Files: `server/services/approval_service.go`

##### Task 2.2.4c: Add "Approve anyway" affordance in `NotificationPanel.tsx` (~5 min)
- Extend `resolveApproval`'s signature (`:162`) with an optional 4th parameter,
  `overrideCiBlock?: boolean`, threaded into the `create(ResolveApprovalRequestSchema, {
  approvalId, decision, overrideCiBlock })` call at `:165` (existing callers omit it and get
  proto3's default `false`, so Task 2.2.2b's Approve/Deny buttons are unaffected).
- Next to the inline block-explanation text added in Task 2.2.2b, render a second button
  "Approve anyway" when a stored block error exists for that `approvalId`. On click, call
  `resolveApproval(approvalId, "allow", group.allIds, true)`, then clear the stored
  block-error state for that `approvalId` on success (same success path Approve already
  takes at `:166`).
- Files: `web-app/src/components/ui/NotificationPanel.tsx`

##### Task 2.2.4d: Unit test coverage for the override (~4 min)
- Add `TestResolveApproval_OverrideCiBlock_SkipsGuard_AndLogsDistinctly` to
  `server/services/approval_service_test.go`: given failing CI and the flag on, assert
  `ResolveApproval` succeeds (no `CodeFailedPrecondition`) when `OverrideCiBlock: true`.
  Add `TestResolveApproval_OverrideCiBlock_NoOp_WhenBlockWouldNotHaveFired`: given the flag
  off (or CI passing, or no PR), assert `OverrideCiBlock: true` has no observable behavioral
  difference from the equivalent Story 2.2.3 test.
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

---

## Implementation Deviations (discovered during Phase 1/2)

Two corrections to this plan's assumptions, found while implementing Tasks 1.1.2a/2.2.2a:

1. **`GitHubCheckConclusion`/`LastPRStatusCheck` are not persisted.** `Storage.UpdateInstancePRStatus`
   (`session/storage.go:539-543`) is a deliberate no-op with the comment "PR fields are not
   stored in the ent schema — they live in memory and are re-populated by PRStatusPoller on
   each poll cycle." `session/ent/schema/session.go` has no `github_check_conclusion` or
   `last_pr_status_check` column. This means `Storage.FindInstanceDataByID` (the lookup Tasks
   1.1.2a and 2.2.2a specified) can **never** see these fields — they only exist on the live
   in-memory `*session.Instance` objects the poller mutates directly. Fixed by reusing the
   existing `LiveInstanceFinder` interface (`server/services/workspace_service.go:39-40`,
   already satisfied by `*SessionService`) instead of `*session.Storage` for both gates:
   `ApprovalHandler.liveFinder`/`SetLiveInstanceFinder` (Task 1.1.2a) and
   `ApprovalService.liveFinder`/`SetLiveInstanceFinder` (Task 2.2.1a/2.2.2a). Both are wired
   to `deps.SessionService` in `server/server.go`. `ApprovalService` still needed no `storage`
   field at all — `LiveInstanceFinder` alone covers both `GitHubPRNumber` and
   `GitHubCheckConclusion`. Ent schema/migration was not touched — persisting these fields was
   out of scope and would be a much larger change than this plan's stated size.
2. Adding `RequireCIPassing` (Task 1.1.3a-c) also required a previously-undocumented 3rd
   persistence layer beyond `RuleSpec`/proto: rules are stored via `EntRepository.UpsertRule`/
   `AllRules` (`session/ent_repository.go`) backed by `session/ent/schema/approvalrule.go`, not
   only the JSON export path the plan's Migration Plan described. Added
   `require_ci_passing` to the ent schema (`Default(false)`, same pattern as
   `safe_python_imports_only`) and regenerated via `go run -mod=mod entgo.io/ent/cmd/ent
   generate --feature sql/upsert ./schema` per `.claude/rules/ent-schema-generation.md`.
