# Implementation Plan: backlog-already-implemented

**Feature**: Let a backlog implementation agent credibly report "already implemented, nothing to change" and have the reviewer independently verify that claim against the live codebase instead of auto-marking every empty-diff criterion UNVERIFIABLE.
**Date**: 2026-07-14
**Status**: Ready for implementation
**ADRs**: ADR-001 (`project_plans/backlog-already-implemented/decisions/ADR-001-workdir-plus-defensive-tool-flags-for-headless-review.md`)

---

## Step 0.5 — Alternatives Considered (Creative Pass)

Three high-level approaches for the core problem (verifying an "already implemented" claim when the diff is empty):

1. **Trust specific claims without re-verification** (score the `note`/`verification_notes` text on specificity alone, no codebase check).
   - *Strength*: Zero new plumbing — a pure prompt-language change, cheapest to ship.
   - *Weakness*: Re-opens exactly the anti-gaming risk this project exists to close (requirements.md explicitly rejects this as an "Alternative Considered"); a fluent, specific-sounding lie is indistinguishable from a true claim to a text-only judge.

2. **Deterministic Go-side codebase inspection** — grep/read specific files server-side based on heuristics extracted from the AC text, inject the results as a labeled evidence section, no reviewer tool loop.
   - *Strength*: Fully deterministic, no dependency on `claude -p`'s undocumented tool-grant behavior, consistent with this codebase's existing self-heal patterns (`RecoverBaseCommitSHA`).
   - *Weakness*: Deciding *which* files/symbols are relevant to an arbitrary free-text acceptance criterion is a harder, more speculative problem than it looks — it needs the same reasoning an LLM already does naturally when it reads a tree, so building it deterministically trades a small amount of runtime uncertainty for a large amount of new, brittle heuristic code.

3. **Grant the reviewer bounded, scoped codebase read access via the existing `headless.CallOptions{WorkDir: ...}` mechanism** (extended with defensive `AllowedTools`/`PermissionMode` flags), and require the reviewer to independently locate and quote its own evidence before crediting any "already implemented" claim.
   - *Strength*: Reuses infrastructure already proven in this exact codebase (the triage headless call, `backlog_service_triage.go:671-676`) and the exact flag pair already proven for interactive sessions (`instance_tmux.go:117-124`) — small, targeted plumbing, no new subsystem.
   - *Weakness*: More expensive/slower per empty-diff review (a real tool-use round trip vs. a single-shot JSON call), and rests on an empirically-unverified-until-now assumption about headless tool-grant behavior — mitigated by an explicit smoke test as the first task (Epic 1.1) and defensive flags (ADR-001).

**Chosen: Approach 3**, per ADR-001. Approach 1 is rejected outright by the requirements' own "Alternatives Considered" section. Approach 2 is recorded as the rejected alternative in the Pattern Decisions table below with the full reasoning in ADR-001.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `AcCriterion` | A single acceptance criterion on a backlog item: `Index`, `Text`, `Status`, `Note`. `Note` is free-text evidence self-reported by the work session via `report_progress`. | Existing type, `session/domain/backlog.go:66-71`. No schema change. |
| `AcSnapshot` | The JSON-serialized `[]AcCriterion` frozen onto an `ItemSession` row at spawn time. Can go stale relative to the live item's `AcceptanceCriteria` once `report_progress` writes a `Note` mid-session. | `ItemSession.ac_snapshot`; the staleness is Finding A in architecture.md. |
| `MergeLiveCriterionNotes` | New helper (`session/backlog_review.go`) that overlays each criterion's live `Note`/`Status` (from `BacklogItem.AcceptanceCriteria`) onto a possibly-stale `AcSnapshot`, matched by `Index`, so a note written after session spawn still reaches the reviewer. | New in this feature. |
| `VerificationNotes` | Freeform evidence the work session attaches via `request_review`, persisted on `ItemSession.verification_notes`, rendered in the reviewer prompt's `## Verification Evidence` section. | Already implemented (commit `70e33b0d`); unchanged by this feature except for the new empty-diff instructions around it. |
| `ReviewOutcome` | Typed verdict: `PASS`, `FAIL`, `PARTIAL`, `UNVERIFIABLE`. | Existing, `session/domain/backlog.go:94-102`. |
| `CriterionVerdict` | One criterion's `ReviewOutcome` plus an `Evidence` string, as emitted by the reviewer. | Existing, `session/domain/backlog.go:122-127`. |
| `AggregateOutcome` | Computes the overall `ReviewOutcome` from `[]CriterionVerdict` (priority FAIL > PARTIAL > UNVERIFIABLE > PASS; empty list defaults to FAIL). | Existing, unchanged — the "default to FAIL, not PASS" posture this feature explicitly preserves. |
| Empty-diff / no-diff review | A review whose computed git diff is `""`, rendered today as `"(no diff available)"`. Can mean "already done before this session" or "nothing happened." | Not a new type — a `diff == ""` branch already present in both prompt builders. |
| Codebase-verification path | The new reviewer behavior for the empty-diff case: the reviewer is granted bounded, read-only access to the item's worktree/repo and must independently confirm or refute the work session's claim before verdicting, instead of defaulting straight to UNVERIFIABLE. | New in this feature; gated behind `diff == ""`. |
| `headless.CallOptions` | Per-call override struct (`WorkDir`, `Model`, `TimeoutSecs`, and new `AllowedTools`/`PermissionMode`) passed to `Pool.CallWithOptions`/`CallBlockingWithOptions` to configure one `claude -p` subprocess invocation. | Extended in this feature, `session/headless/caller.go:18-25`. |
| `AllowedTools` / `PermissionMode` (on `CallOptions`) | New fields mirroring `session.InstanceOptions.AllowedTools`/`PermissionMode`; passed to the `claude -p` subprocess as `--allowedTools`/`--permission-mode` to scope a `WorkDir`-bearing call to read-only tools. | New in this feature. See ADR-001. |
| `HeadlessReviewSystemPromptWithCodebaseAccess` | New system prompt (`session/headless/features.go`) used only for the empty-diff headless review call; instructs the model to independently verify each criterion against the live tree, cite its own evidence, AND populate a `tool_reads` list of files it actually opened. Distinct from the existing `headlessReviewSystemPrompt`, which stays exactly as-is for the normal (non-empty-diff) path. | New in this feature. Updated in this repair pass to require `tool_reads` (see below, Blocker 2). |
| `Pool.CallBlocking` (consolidated signature) | The four pre-existing near-duplicate blocking methods (`CallBlocking`, `CallBlockingWithCost`, `CallBlockingWithOptions`, plus the previously-proposed `CallBlockingWithCostAndOptions`) are collapsed into one: `func (p *Pool) CallBlocking(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions) (result string, costUSD float64, err error)`. Zero-value `opts` reproduces today's simplest-call behavior; cost is always returned (parsed from the JSON result at no extra cost when the caller ignores it via `_`). Replaces the plan's originally-proposed `CallBlockingWithCostAndOptions` entirely — see Pattern Decisions and Story 2.1.2 (rewritten in this repair pass, Architecture Review Concern B). | Rewritten in this feature, `session/headless/caller.go`. |
| `BuildReviewCallOptions` | New pure helper (`session/backlog_review.go`) — `func BuildReviewCallOptions(diff, codebaseWorkDir string) (systemPrompt string, opts headless.CallOptions, callTimeout time.Duration, path string)` — that is the single place the `diff == ""` branch (system prompt choice, `WorkDir`/`AllowedTools`/`PermissionMode`, and the stricter codebase-read timeout) is decided. Both `ReviewGateRunner.Run` and `TriggerReReview` call this instead of each independently constructing the same literals. Added in this repair pass to close Adversarial Review Concern A / Architecture Review's duplicated-branching-logic concern. | New in this feature, `session/backlog_review.go`. |
| `CodebaseReadCallTimeout` | New constant (`session/headless/features.go`) — `150 * time.Second` — used as the context timeout for the empty-diff codebase-read headless call specifically, in place of the shared `DefaultCallTimeout` (900s). Deliberately short so a hung/degraded tool-access call fails fast into the UNVERIFIABLE degrade path (Blocker 2 / Story 2.2.4) instead of blocking the review gate for up to 15 minutes. | New in this feature. See Story 2.2.4 for the 150s justification. |
| `tool_reads` / `ToolReads` | New JSON field in the codebase-access reviewer's response schema (`headlessVerdictJSON.ToolReads []string`) — the list of file paths the reviewer LLM asserts it actually opened via `Read`/`Grep`/`Glob` during this call. A non-empty `tool_reads` list is necessary but, as of this repair pass, no longer sufficient on its own: `verifyToolReadsExist` (Story 2.2.4, this repair pass) additionally `os.Stat`s every listed path against `codebaseWorkDir` — a confabulating model that fabricates a plausible-looking `tool_reads` list alongside fabricated `evidence` is now caught if even one claimed path doesn't actually exist in the worktree. See Pre-Mortem P1 #1. | New in this feature, `session/backlog_review.go`. See Story 2.2.4 (Blocker 2, and Pre-Mortem P1 #1). |
| Codebase-read degrade path | The behavior where, on the empty-diff path, a timeout against `CodebaseReadCallTimeout`, OR a PASS/FAIL verdict with an empty `tool_reads` list, OR a PASS/FAIL verdict where ANY `tool_reads` path fails `os.Stat` against `codebaseWorkDir` (this repair pass, Pre-Mortem P1 #1), is treated as "could not verify" and force-downgraded to `UNVERIFIABLE` (never `FAIL` — `FAIL` would wrongly signal "reviewed and found lacking" rather than "couldn't check at all"), preserving today's pre-feature UNVERIFIABLE baseline as the safe fallback instead of hanging or trusting a hallucinated verdict. | New in this feature, Story 2.2.4. See Blocker 2 of adversarial-review.md and Pre-Mortem P1 #1. |
| `verifyToolReadsExist` | New pure-ish helper (`session/backlog_review.go`) — `func verifyToolReadsExist(codebaseWorkDir string, toolReads []string) bool` — does a cheap, non-LLM `os.Stat` on every path in `toolReads`, resolved relative to `codebaseWorkDir` (or as an absolute path if already absolute), and returns `false` if ANY claimed path does not exist. Called from `DegradeIfUnverified` before trusting a non-empty `tool_reads` list. Closes Pre-Mortem P1 #1: a confabulating model can fabricate `evidence` text but cannot make `os.Stat` return success for a path it invented. | New in this feature, this repair pass. `session/backlog_review.go`. See Story 2.2.4. |
| `CodebaseReadCapabilitySelfCheck` | New `sync.Once`-guarded runtime self-check (`session/review_gate.go` or `session/headless/`) that lazily re-runs the shape of Task 1.1.1a/2.1.1e's smoke test (marker-file read via `WorkDir`+`AllowedTools`) exactly once per process lifetime, triggered by the first empty-diff (`diff == ""`) review attempted after boot. On failure, logs a distinguishable `log.WarningLog` line and causes subsequent empty-diff reviews in that process's lifetime to skip the codebase-read tool-access call entirely, falling back to the pre-feature plain-UNVERIFIABLE behavior, until the process restarts. Makes the Task 1.1.1b production-CLI-parity assumption self-verifying on every real deploy instead of a one-time manual attestation. See Story 2.2.6 (new in this repair pass, Pre-Mortem P1 #2). | New in this feature, this repair pass. |
| `codebaseWorkDir` | The resolved directory (worktree path, falling back to `item.RepoPath`) passed as `CallOptions.WorkDir` for a no-diff review call, and as the second argument to `BuildReviewCallOptions`. | New local variable in `review_gate.go`/`backlog_service_triage.go`. |
| `ReviewGateRunner` | Existing type (`session/review_gate.go`) encapsulating `spawnReviewGate` logic. Gains the empty-diff `WorkDir` branching (now via `BuildReviewCallOptions`), the snapshot-merge fix, and the codebase-read degrade path. | Existing, `session/review_gate.go:21-27`. |
| `resolveACSnapshot` | Existing helper (`server/services/backlog_service_triage.go:1078`) selecting which AC snapshot `TriggerReReview` uses. Gains the same live-note-merge fix as `ReviewGateRunner.Run`. | Existing, modified in this feature. |
| Falsification framing | The anti-gaming prompt stance requiring the reviewer to independently locate and quote its *own* evidence rather than accept the agent's claim at face value — applied specifically to the codebase-verification path. | Prompt-design principle from research/pitfalls.md, not a code symbol. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Empty-diff codebase access mechanism | WorkDir-only (primary) + defensive `AllowedTools`/`PermissionMode` flags on `headless.CallOptions` | ADR-001 (this project) | (b) Deterministic Go-side grep/read injected as a prompt section | Deciding which files/symbols are relevant to an arbitrary free-text AC criterion is itself a hard, speculative problem an LLM already solves naturally by reading the tree; hand-rolling that heuristic is more code and more brittle than granting bounded, scoped read access via infrastructure already proven in this codebase (triage headless call + `instance_tmux.go` flag pattern). |
| Empty-diff codebase access mechanism (secondary choice within the plumbing itself) | WorkDir + explicit flags (option c) | ADR-001 | (a) WorkDir-only, no defensive flags | Simplest, matches the triage precedent exactly, but rests the whole feature on an unstated/undocumented CLI default with no fallback if it differs for reads vs. writes or changes in a future CLI version; the explicit flags cost a handful of already-proven lines and remove that single point of failure. |
| AC-snapshot staleness fix | Small pure function (`MergeLiveCriterionNotes`) called from both existing call sites | This project | A schema/data-model change (e.g. writing `Note` directly into `AcSnapshot` at report-progress time) | No schema change is needed or wanted (requirements.md's appetite explicitly rules this in-scope work out); the staleness is a snapshot-*selection* bug, not a missing field — a merge function fixes it without touching how/when snapshots are captured. |
| Prompt structure for empty-diff instructions | Extend existing conditional (`if diff == ""` blocks already present in both builders) with a new labeled subsection | research/pitfalls.md item 4 | An unconditional prompt-language change (single shared instruction block for all reviews) | Any unconditional change to the shared prompt builders risks bleeding into the 99% non-empty-diff case (research/pitfalls.md's sharpest scope-creep risk); gating behind the existing `diff == ""` branch that already exists at `backlog_review.go:107`/`:170` costs nothing extra and structurally prevents leakage. |
| `report_progress` note rendering | Unconditional (every review, not diff-gated), with a `sanitizeField`-style length cap | requirements.md success metric #3 | Gating note rendering behind `diff == ""` too | The requirement explicitly wants the note surfaced in *what the reviewer sees* generally (it's collected today and dropped everywhere, not just on empty diffs) — a criterion note is useful context even alongside a diff (e.g. "see the refactor in commit abc123, not this diff"). Needs its own non-empty-diff test coverage precisely because it is unconditional. |
| `verification_notes` requirement on empty diff | Optional but strongly prompted (tool-description change only, no Go-level required-field validation) | Deferred to this plan per requirements.md open question | Hard-required (block `request_review` when the agent believes the diff will be empty) | (1) The tool has no way to know at call time whether the diff will end up empty — that's computed later in the review gate, so enforcing it would require new predictive plumbing for uncertain benefit. (2) Hard-requiring a field doesn't close the anti-gaming gap — a bad-faith agent can satisfy "non-empty" with filler text just as easily as a vague true claim; the actual guardrail is the reviewer's independent codebase read (Epic 2.2), not the presence of a note. Making it optional-but-strongly-prompted (Epic 2.4) gets the UX nudge without a false sense of enforcement. |
| New Go types/interfaces | None added beyond pure helper functions and struct field additions | Interface-pollution checklist (`.claude/rules/interface-pollution-checklist.md`) | A `ReviewEvidenceSource` interface, a `CodebaseVerifier` type, etc. | This is a targeted plumbing/prompt fix on existing types (`AcCriterion`, `CallOptions`, `Pool`) — no new abstraction has two real implementations or an imminent second one, so none is introduced. |
| Empty-diff `CallOptions`/system-prompt/timeout decision | Single shared pure helper `BuildReviewCallOptions(diff, codebaseWorkDir string)` in `session/backlog_review.go`, called by both `ReviewGateRunner.Run` and `TriggerReReview` | Adversarial Review Concern A / Architecture Review's "duplicated branching logic" concern | Each call site independently constructing its own `CallOptions{WorkDir:..., AllowedTools:..., PermissionMode:...}` literal and picking its own system prompt (the plan's original Story 2.2.2/2.2.3 shape) | ADR-001's "reviewer must never get write tools" invariant, plus the new degrade-path timeout (Blocker 2), are safety-critical and must not be able to silently drift between the automatic review-gate path and the manual re-review path. A single pure function is the one point of failure to audit instead of two, and is trivially unit-testable without either call site's storage/notifier machinery. |
| `Pool` blocking-call API surface | Consolidate `CallBlocking`/`CallBlockingWithCost`/`CallBlockingWithOptions` (and the originally-proposed `CallBlockingWithCostAndOptions`) into one method: `CallBlocking(ctx, key, systemPrompt, userPrompt string, opts CallOptions) (string, float64, error)` | Architecture Review Concern B, full-consolidation option chosen (not scoped down) | (1) Add a 4th method (`CallBlockingWithCostAndOptions`) as the plan originally proposed; (2) leave the M×N combinatorial pattern undecided/unaddressed | Verified via `grep` that call-site count is small and fully in-repo (12 call sites total: `CallBlocking`×4 in `session/headless/features.go`, `CallBlockingWithCost`×1 in `session/review_gate.go`, `CallBlockingWithOptions`×5 in `approval_handler.go`, `backlog_service_triage.go`×2, `autonomous_driver.go`, `session_service.go`, `CallWithOptions`×2). Each update is a one-line signature-shape change (pass `CallOptions{}` where nothing is needed today, ignore the cost return with `_`). Judged low-risk enough to fix now rather than deferred as debt, per the architecture review's own "cheap enough to consolidate now, before a 5th variant becomes tempting" framing — doing it now also means Story 2.1.2's new call site (Epic 2.2's empty-diff path) is just another ordinary `CallBlocking` call, not a bespoke 4th method that immediately becomes the *only* caller of a `WithCostAndOptions` combination. |
| Codebase-read "no tool access happened" detection | Checkable heuristic: reviewer JSON response must include `tool_reads []string`; an empty list on a PASS/FAIL verdict from the codebase-read path is treated as suspicious and force-downgraded to `UNVERIFIABLE` | Adversarial Review Blocker 2 | (a) Trust any PASS/FAIL verdict from the codebase-read path at face value (status quo the blocker rejects); (b) hard `FAIL` on suspected degraded tool access | (a) is exactly the anti-gaming failure this feature exists to prevent — a model that can't actually read files but doesn't know it may confidently hallucinate a PASS. (b) is rejected because `FAIL` incorrectly signals "reviewed and found lacking" when the true state is "could not verify at all," which would trigger `AutoReopenAfterFailedReview` and burn a rework cycle for a review that never actually happened — `UNVERIFIABLE` is the correct signal and matches the feature's pre-existing default-to-UNVERIFIABLE posture (`AggregateOutcome`'s priority order, unchanged). |
| Codebase-read call timeout | Dedicated `CodebaseReadCallTimeout = 150 * time.Second` constant, shorter than the shared `DefaultCallTimeout` (900s) | Adversarial Review Blocker 2 | Reuse `DefaultCallTimeout` (900s) / let the call run up to `MaxCallTimeout` (1800s) unchanged | A hung or degraded tool-access call on the empty-diff path should fail fast into the UNVERIFIABLE degrade path, not block the review gate — and by extension the item's forward progress and any queued re-review — for up to 15–30 minutes per attempt. 150s is generous relative to a handful of `Read`/`Grep`/`Glob` round trips on a single worktree (Epic 1.1's smoke test exercises the same call shape and is expected to complete in well under 30s) while still being short enough that a genuinely stuck call surfaces as a degraded UNVERIFIABLE within roughly one dev-attention-span rather than a quarter hour. **This repair pass adds an explicit pre-ship validation task (Story 2.5.2) so 150s is confirmed against observed call durations, not shipped as an untested estimate — see Pre-Mortem P2 #5.** |
| `tool_reads` corroboration (this repair pass — Pre-Mortem P1 #1) | Cheap, non-LLM Go-side `os.Stat` existence check (`verifyToolReadsExist`) on every path in a non-empty `tool_reads` list, run inside `DegradeIfUnverified` before trusting the list | Pre-Mortem P1 #1 (Failure #1) | (a) Trust any non-empty `tool_reads` list at face value, as originally specified in the first repair pass (status quo this finding rejects); (b) defer corroboration to a future `--output-format stream-json` `tool_use`-event cross-check (validation.md's noted but unimplemented ideal) | (a) is exactly the failure mode validation.md's own "Anti-Gaming Coverage Adequacy" section flagged as a genuine, undischarged residual risk: a confabulating model can fabricate `tool_reads` the same way it fabricates `evidence`, since both come from the same JSON blob the model authors. (b) is the more complete fix but requires plumbing the CLI's streaming tool-event log through the blocking-call path — a larger change than this "focused feature" appetite justifies right now. `os.Stat` is a few milliseconds of Go-side work, requires no new subsystem, and closes the specific, cheap-to-close half of the gap (fabricated *paths*) even though it cannot catch a model that reads a real file and then lies about what's in it (see Unresolved Questions). |
| Production-CLI-parity enforcement mechanism (this repair pass — Pre-Mortem P1 #2) | Lazy, `sync.Once`-guarded runtime self-check (`CodebaseReadCapabilitySelfCheck`) triggered by the first empty-diff review attempted after process boot, logging a WARNING and falling back to plain UNVERIFIABLE on failure | Pre-Mortem P1 #2 (Failure #2) | (a) Leave Task 1.1.1b as a purely manual, checklist-ticked pre-merge verification (status quo this finding rejects as insufficient — see Failure #2's first-symptom scenario); (b) a new boot-time `app.Phase(...)` step in `main.go`'s `rootCmd.RunE` startup sequence (confirmed via `main.go:246-338` to have no existing self-test/health-check phase pattern to extend) | (b) was considered and rejected in favor of (a lazy check): the codebase has no precedent for a startup self-test phase, and a boot-time check would add latency/failure-coupling to every server start (including deployments that never hit an empty-diff review) for a capability only the review-gate path needs. A lazy, once-per-process check that fires on first actual use ties the verification to the exact code path it protects, costs nothing on servers that never see an empty-diff item, and is a strictly smaller, more targeted change consistent with this codebase's existing preference for scoped fixes over new subsystems. Task 1.1.1b (manual, pre-merge) is kept as-is alongside this — the manual check is still useful pre-merge/pre-release; the automated check is the ongoing, post-deploy backstop, per the finding's explicit instruction not to replace one with the other. |
| Mixed-criteria ("already implemented" Note outside `diff == ""`) evidentiary weight (this repair pass — Pre-Mortem P1 #3) | Choice (b): explicitly downgrade an "already implemented" Note's evidentiary weight to informational-only whenever `diff != ""` — it can never by itself be sufficient for that criterion's PASS; the reviewer must still find diff-based or independently-verified support | Pre-Mortem P1 #3 (Failure #3) | Choice (a): extend codebase-read tool access to run per-criterion whenever ANY criterion's Note claims "already implemented," regardless of overall diff emptiness | Choice (a) would mean *every* review with at least one "already implemented" Note takes an agentic tool-use round trip (150s+ potential) instead of the fast single-shot JSON call the normal path uses today — a much larger blast-radius change than "focused feature" (complexity 2) justifies, since a mixed PR with several such Notes is a plausible common case, not an edge case, once Story 1.2.1 makes Note-rendering unconditional. Choice (b) is strictly cheaper (a fixed sentence added to the existing non-empty-diff system prompt, no new tool-access branch, no new timeout/degrade subsystem to extend) and is consistent with this plan's existing asymmetric-caution bias (default-to-FAIL `AggregateOutcome`, degrade-not-trust `DegradeIfUnverified`) — an unconfirmed claim losing evidentiary weight is the same posture as an unconfirmed tool-read losing trust. Per the finding's own steer, (b) is recorded here as the chosen, not merely default, option — see Story 2.2.5. |

---

## Observability Plan

- **Logs**: `session/review_gate.go`'s existing `log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate headless review complete for item %s (session %s, outcome %s)"...)` (line ~302) gains a `path=` field with **three** distinguishable values — `diff`, `codebase-read-verified` (empty-diff path, tool access confirmed via a non-empty `tool_reads` list where every path also passed `os.Stat`, this repair pass), and `codebase-read-degraded` (empty-diff path, timed out against `CodebaseReadCallTimeout`, or returned an empty `tool_reads` list, or claimed a `tool_reads` path that fails `os.Stat` — Pre-Mortem P1 #1 — and was force-downgraded to UNVERIFIABLE) — so operators can distinguish "graded normally," "graded via a working codebase check," and "silently couldn't verify" from the logs alone (Epic 2.5, extended in this repair pass per Blocker 2 and P1 #1). Mirrored in `TriggerReReview`'s equivalent log line in `server/services/backlog_service_triage.go`. **Also gains call duration (this repair pass, Pre-Mortem P2 #5)** — see Story 2.5.1's Task 2.5.1c.
- **Metrics**: None added — out of scope per requirements.md ("Observability Requirements: Not required (complexity 2)"). The cost field already tracked (`callCostUSD` / `ItemSessionData.EstimatedCostUsd`) continues to be populated on the new path via the consolidated `Pool.CallBlocking` (Story 2.1.2, rewritten in this repair pass — see Pattern Decisions), so per-review cost is already queryable without new instrumentation.
- **Alerts**: None added — consistent with existing rework-cap notification being the only operator-facing signal in this subsystem, and changing that is explicitly out of scope. **Exception (this repair pass, Pre-Mortem P1 #2)**: the new `CodebaseReadCapabilitySelfCheck` (Story 2.2.6) logs a `log.WarningLog` line (distinguishable text: `"[BacklogLifecycle] codebase-read capability self-check FAILED"`) on first-use failure — not a new alert channel, but a WARNING-level signal an operator's existing log-monitoring (if any) can already pick up, consistent with how `notifyReworkCapHit` is the only other WARNING-level signal in this subsystem today.

## Risk Control

- **Feature flag**: None — consistent with requirements.md ("Risk Control: Not required... no feature flag needed, rollback is a normal revert"). **Re-justified explicitly in this second repair pass, since the risk-mitigation footprint has grown substantially since that original one-line assertion**: this plan now includes hard-blocking integration gates (Epic 1.1/2.1.1e, Task 1.1.1b), a production-CLI-parity check (now doubled — a manual pre-merge check AND an automated runtime self-check, Story 2.2.6), an entirely new degrade/timeout subsystem (`DegradeIfUnverified`, `CodebaseReadCallTimeout`, Story 2.2.4, now also doing `os.Stat` path corroboration per P1 #1), and a 12-call-site `Pool.CallBlocking` consolidation (Story 2.1.2). Despite that expanded surface area, a feature flag remains unnecessary because **every new runtime behavior this feature adds is still reachable only through the single `diff == ""` branch** (`BuildReviewCallOptions`, the sole point of decision per the Pattern Decisions table) — the degrade subsystem, the self-check, and the codebase-access prompt/tool-grant are all dead code on every review where `diff != ""`, which remains the overwhelming majority case. The one piece of this footprint that is *not* diff-gated — the `Pool.CallBlocking` signature consolidation (Story 2.1.2) — is mechanically low-risk (each of its 12 call sites is a one-line signature-shape change verified by existing tests) and, per this repair pass's resolution of Pre-Mortem P2 #4, now ships as its own separate PR ahead of the review-gate changes (see Dependency Visualization), so it can be reverted independently of, and without reopening, the anti-gaming-critical portion of this feature. A revert of the review-gate PR is therefore still a plain `git revert` with no runtime toggle to manage; a regression traced to the `Pool.CallBlocking` consolidation itself is a separate, independently revertible PR by construction.
- **Rollback procedure**: Standard `git revert` of the merged PR(s) — now two PRs per Pre-Mortem P2 #4's sequencing split (Story 2.1.2 first, everything else second), each independently revertible. No data migration, no schema change, no in-flight state to reconcile (verified in architecture.md: "no ent/proto schema changes needed").
- **Staged rollout**: Not applicable — solo-operator project, no user cohorts. Epic 1.1's smoke test (integration-tagged, runs under `make test-integration`/`make ci`) is the closest analogue to a staged rollout: it fails fast in CI if the WorkDir-only assumption doesn't hold, before the feature reaches the review gate at all. **This repair pass makes that gate a hard blocking prerequisite, not just a de-risking measure** — see the Dependency Visualization and Epic 1.1 below, and ADR-001's "2026-07-14 repair pass" addendum. **This second repair pass adds a lazy, always-on runtime analogue of that same gate** (`CodebaseReadCapabilitySelfCheck`, Story 2.2.6) that re-verifies the same assumption on every real deploy, not just once at merge time.
- **Degradation path (added in this repair pass — Adversarial Review Blocker 2)**: previously unspecified. If the codebase-read call hangs, times out, or shows no evidence a tool was actually invoked, the review now force-downgrades to `UNVERIFIABLE` (never silently trusts an ungrounded PASS, never mis-signals `FAIL`) via Story 2.2.4, using a dedicated shorter timeout (`CodebaseReadCallTimeout`, 150s) so this happens fast rather than after the shared 900s `DefaultCallTimeout`. This directly narrows the residual-risk bullet below for the specific failure mode of "tool access silently didn't work" — it does **not** address a verdict that is wrong despite genuinely-grounded evidence (see below). **Second repair pass (Pre-Mortem P1 #1)**: the degrade path is further tightened so a *fabricated* `tool_reads` list (plausible-looking file paths the model never actually opened, submitted alongside fabricated `evidence`) is also caught — `verifyToolReadsExist` `os.Stat`s every claimed path against `codebaseWorkDir` and forces `UNVERIFIABLE` if any path doesn't exist, closing the specific gap Pre-Mortem Failure #1 identified: a confabulating model sailing past the original tool_reads-non-empty check with a false PASS and zero downstream signal.
- **Residual risk (explicitly out of scope to fix here, flagged per pitfalls.md item 5)**: A false PASS reached via this feature's codebase-verification path *with genuinely-grounded evidence* (i.e., the reviewer did read real files that exist, and still reached a wrong conclusion, or misrepresented what a real file contains) ships silently — there is no notification anywhere in the PASS path today (only `notifyReworkCapHit` fires, and only after 3 FAIL/PARTIAL/UNVERIFIABLE cycles). This makes the anti-gaming prompt design (falsification framing, independent citation requirement — Epic 2.2/2.3) the *only* backstop once a review reaches PASS via this path. Story 2.2.4's degrade path (both repair passes) closes the narrower "tool access silently failed" and "tool_reads path is outright fabricated/nonexistent" failure modes specifically, but does not, and cannot by testing alone, close the remaining "reviewer read a real file and still lied or erred about its contents" vector (see Unresolved Questions) — nor does it add a PASS-path notification, which remains out of scope, belonging to `project_plans/backlog-stuck-item-visibility/` per requirements.md's explicit scope boundary, and is recorded here so it isn't lost.

## Unresolved Questions

- [ ] Whether `claude -p`'s default tool-approval behavior for `Read`/`Grep`/`Glob` under `bypassPermissions` mode remains stable across future CLI versions — mitigated but not permanently resolved by the Epic 1.1 smoke test, which re-runs on every `make ci`/`make test-integration` and will fail fast if behavior regresses. Owner: whoever next touches `session/headless/` after a CLI upgrade — no action needed now.
- [ ] Whether a PASS-path notification should eventually be added so a false "already implemented" PASS doesn't ship with zero human-visible signal (residual risk noted above) — explicitly deferred to `project_plans/backlog-stuck-item-visibility/` per requirements.md scope boundary; owner: that project, not this one.
- [x] ~~Whether `CodebaseReadCallTimeout`'s proposed value (150s) needs tuning after real-world empty-diff reviews are observed~~ — **resolved into an explicit pre-ship task in this repair pass** (Story 2.5.2, Pre-Mortem P2 #5): call duration is now logged alongside `path=` from day one (Task 2.5.1c), and Epic 2.3/2.3.2–2.3.5's adversarial fixtures plus a handful of real empty-diff items must be run against representative worktree sizes and their durations reviewed *before* the feature is considered done — not deferred to "tune later if a problem is noticed." See Post-Implementation Checklist.
- [ ] **New in this repair pass (validation.md's "Anti-Gaming Coverage Adequacy" judgment #2, now tracked here per that document's own request)**: `tool_reads` self-attestation is still not *fully* independently verifiable even after Story 2.2.4's `os.Stat` corroboration (Pre-Mortem P1 #1) — `verifyToolReadsExist` closes the "cites a file that doesn't exist" vector but cannot catch a model that reads a *real* file and then fabricates or misremembers its contents in the `evidence` field. Closing that fully would require cross-checking against the CLI's own `--output-format stream-json` `tool_use` events (an independent signal outside the model-authored JSON blob), which is a larger plumbing change out of scope for this "focused feature" appetite. Owner: whoever next revisits this feature's anti-gaming posture, informed by real `path=codebase-read-verified` PASS outcomes once the feature has shipped and accumulated history.

---

## Dependency Visualization

**Repair pass note (resolves Adversarial Review Blocker 1)**: Epic 1.1 is now a
**hard blocking prerequisite** for Epic 2.2 — not a "de-risking, non-blocking"
note. Epic 2.2's stories (2.2.1, 2.2.2, 2.2.3, 2.2.4) must not *start* until
Task 1.1.1a is green, and 2.2.2/2.2.3 must not be considered *done* until the
former Task 1.1.1b (renamed/resequenced below to Task 2.1.1e, per the
architecture review's sequencing nitpick) and the new Task 1.1.1b (production-CLI-parity
check, new in this repair pass — reuses the freed "1.1.1b" slot) are also green.

**Second repair pass note — shipping sequence / PR boundaries (resolves Pre-Mortem P2 #4)**:
Story 2.1.2 (the `Pool.CallBlocking` consolidation touching 12 call sites, 8 of
which are entirely unrelated to backlog review — approvals, autonomous driver,
session service) **ships as its own separate PR (PR #1)**, merged and soaked
independently *before* the rest of this feature. Everything else — Epic 1.1,
Epic 1.2, Story 2.1.1 (and its Task 2.1.1e smoke test), Epic 2.2 (including the
new Stories 2.2.5/2.2.6 below), Epic 2.3 (including the new Stories
2.3.2–2.3.5), Epic 2.4, and Epic 2.5 (including the new Story 2.5.2) — ships
together as **PR #2**, built on top of PR #1's already-merged consolidated
`Pool.CallBlocking` signature. This decouples "a regression in an unrelated
call site" (e.g. approval routing, autonomous-driver error shape) from
"a regression in the safety-critical review-gate anti-gaming change" at triage
time, per Pre-Mortem Failure #4's exact scenario. It also resolves Task
2.1.1e's original hedge ("sequence this task after Story 2.1.2a too if Story
2.1.2 is implemented in the same pass") — that hedge is no longer conditional:
Story 2.1.2 (PR #1) is *always* merged before Task 2.1.1e (PR #2) is written,
so Task 2.1.1e can call the consolidated `pool.CallBlocking` signature
unconditionally.

```
Task 1.1.1a  WorkDir-only smoke test (no new fields needed — true Phase 1 gate)
   │  HARD BLOCKS: nothing below may start until this is green
   ▼
   ├──────────────────────────────────────────────────────────┐
   │                                                            │
Epic 1.2  Surface report_progress notes to the reviewer         │
   ├─ 1.2.1  Render AcCriterion.Note (both prompt builders)      │
   ├─ 1.2.2  Fix AC-snapshot staleness — review_gate.go           │
   └─ 1.2.3  Fix AC-snapshot staleness — TriggerReReview           │
        │                                                          │
        │        Epic 2.1  Extend headless Pool for bounded read access
        │        ├─ 2.1.1  CallOptions + ProcessRunner flags        │
        │        │    └─ 2.1.1e  WorkDir+flags smoke test (moved   │
        │        │         from "Task 1.1.1b" — runs immediately   │
        │        │         after 2.1.1a lands; still a HARD        │
        │        │         BLOCKER on Epic 2.2, just correctly     │
        │        │         sequenced in Phase 2 where its deps     │
        │        │         actually live)                          │
        │        └─ 2.1.2  Consolidate Pool.CallBlocking (Concern B)│
        │                    │                                     │
        └────────────────────┼─────────────────────────────────────┘
                              ▼
              Task 1.1.1b  Confirm 1.1.1a + 2.1.1e pass against the
              production systemd service's actual claude CLI/config
              (not just CI runner PATH) — HARD BLOCKS Epic 2.2.2/2.2.3
              being marked done (new in this repair pass, Blocker 1)
                              │
                              ▼
                    Epic 2.2  Empty-diff reviewer prompt & wiring
                    ├─ 2.2.1  New system prompt + BuildReviewCallOptions
                    │         helper + tool_reads schema (Concern A)
                    ├─ 2.2.2  Wire via BuildReviewCallOptions — review_gate.go
                    ├─ 2.2.3  Wire via BuildReviewCallOptions — TriggerReReview
                    ├─ 2.2.4  Degrade-to-UNVERIFIABLE on timeout / missing OR
                    │         nonexistent tool_reads evidence (Blocker 2 +
                    │         Pre-Mortem P1 #1's os.Stat check — NEW)
                    ├─ 2.2.5  Mixed-criteria evidentiary-weight guard — Note
                    │         claims outside diff=="" are informational only
                    │         (Pre-Mortem P1 #3 — NEW, independent of 2.2.1-4)
                    └─ 2.2.6  Runtime CodebaseReadCapabilitySelfCheck — lazy,
                              automated production-parity gate (Pre-Mortem
                              P1 #2 — NEW, independent of 2.2.1-4)
                              │
                    ┌─────────┴───────────────────────┐
                    ▼                                  ▼
        Epic 2.3  Adversarial regression tests    Epic 2.5  Observability
        ├─ 2.3.1  False claim, empty diff              ├─ 2.5.1  path= logging
        ├─ 2.3.2  TRUE claim, empty diff → PASS         │        (+ duration,
        │         (Blocker A — NEW)                     │        P2 #5)
        ├─ 2.3.3  Real-but-irrelevant file citation      └─ 2.5.2  Pre-ship
        │         (validation.md gap test — NEW)                  CodebaseReadCallTimeout
        ├─ 2.3.4  Mixed true/false claims, empty diff              validation against
        │         (validation.md gap test — NEW)                   real durations
        └─ 2.3.5  Partial diff + one falsely-claimed                (P2 #5 — NEW)
                  already-implemented criterion
                  (Pre-Mortem P1 #3 — NEW, validates 2.2.5)

Epic 2.4  Tool-description prompting for verification_notes — independent, no deps
```

**Reading this graph**: the critical path for shipping Epic 2.2 at all is now
`1.1.1a → (2.1.1a → 2.1.1e) → 1.1.1b (prod-parity) → 2.2.1 → 2.2.2/2.2.3 → 2.2.4`. Epic 1.2
and Epic 2.4 remain independent side branches with no bearing on whether the
core WorkDir mechanism works, so they may proceed in parallel with the Epic
1.1/2.1 gate — only Epic 2.2 (the actual review-gate wiring this whole feature
exists to ship) is blocked on it. **Stories 2.2.5 and 2.2.6 (this repair pass)
are additive side branches off Epic 2.2**: 2.2.5 is a pure prompt-text change
to the already-existing normal-path system prompt with no new tool-access
branch, and 2.2.6 is a lazy runtime check with no compile-time dependency on
2.2.1–2.2.4's types — neither blocks nor is blocked by 2.2.1–2.2.4, but both
are logically part of "Epic 2.2 done" for this feature's purposes and ship in
PR #2 alongside it. Epic 2.3's new Stories 2.3.2–2.3.5 depend on the same
Epic 1.1/2.1 gate as 2.3.1 (they are real-`claude` integration tests against
the codebase-read path) and, for 2.3.5 specifically, also depend on Story
2.2.5's prompt change existing to have something to validate.

---

## Phase 1: Evidence Plumbing

Fixes needed regardless of which empty-diff codebase-access mechanism is used — surfacing evidence that already exists but is dropped or stale today.

### Epic 1.1: Empirically Verify Headless WorkDir Tool Access (Pre-Implementation Gate — HARD BLOCKER on Epic 2.2)

**Goal**: Resolve the architecture.md/pitfalls.md factual disagreement about whether `headless.CallOptions{WorkDir: ...}` alone grants real file-read tool access, before any later epic's design depends on the answer. Per ADR-001.

**This epic is a hard blocking prerequisite for Epic 2.2, not a de-risking nice-to-have** (repair pass, resolves Adversarial Review Blocker 1). `session/review_gate.go:247` contains the comment `"Use JSON-output prompts because headless claude -p has no tool access"` — written for the exact code path Epic 2.2.2 modifies, and directly contradicted by this feature's core design assumption. Epic 2.2's stories (2.2.1–2.2.4) must not start until Task 1.1.1a is green. Epic 2.2.2/2.2.3 must not be marked **done** until Task 2.1.1e (the WorkDir+flags variant, resequenced into Epic 2.1 below) and Task 1.1.1b (production-CLI-parity confirmation, new below) are also green.

#### Story 1.1.1: Add an integration smoke test proving (or disproving) WorkDir-only read access, and confirm it against production
**As a** developer implementing Epic 2.1/2.2, **I want** an executable, CI-run fact about headless tool-grant behavior — confirmed not just in CI but against the actual environment this feature will run in — **so that** the empty-diff codebase-access design doesn't rest on an untested, or CI-only-tested, assumption.
**Acceptance Criteria**:
- A new integration test exists that writes a marker file into a temp directory and asserts a `WorkDir`-only headless call (no `AllowedTools`/`PermissionMode` set) can read its exact contents back.
  - *Given* a temp dir containing `marker.txt` with contents `STAPLER_SQUAD_MARKER_7f3a1`, *When* `pool.CallBlocking(ctx, FeatureKeyCustom, "", "Read the file marker.txt in your current working directory and output ONLY its exact contents, nothing else.", CallOptions{WorkDir: tempDir})` is called against a real `NewPool`, *Then* the returned string contains `STAPLER_SQUAD_MARKER_7f3a1`.
- This test passes both in CI (`make test-integration`/`make ci`) AND has been manually re-run, and confirmed passing, against the `claude` CLI binary/config actually used by the production `stapler-squad` systemd user service — not merely whatever version happens to be on the CI runner's `PATH`.
  - *Given* the production host where `systemctl --user status stapler-squad` reports the running service, *When* `go test -race -tags integration ./session/headless/... -run TestPool_RealClaude_WorkDirOnly_GrantsReadAccess` is run in a shell with the same `PATH`/`claude --version` the systemd unit resolves (verify via `systemctl --user show stapler-squad -p Environment` or by inspecting the unit file's `PATH`), *Then* the test passes there too, and this fact is recorded (e.g. as a checked-off item in this plan's Post-Implementation Checklist, or a one-line note in the PR description) before Epic 2.2.2/2.2.3 are considered done.
**Files**: `session/headless/integration_test.go`

##### Task 1.1.1a: Write `TestPool_RealClaude_WorkDirOnly_GrantsReadAccess` (~5 min)
- Add to `session/headless/integration_test.go` (already `//go:build integration`): create `t.TempDir()`, write `marker.txt` with a unique constant string, construct a real `NewPool(PoolConfig{...})`, call `result, _, err := pool.CallBlocking(ctx, FeatureKeyCustom, "", "<read-and-echo prompt>", CallOptions{WorkDir: tempDir})`, assert `require.NoError(t, err)` and `require.Contains(t, result, markerValue)`.
- Files: `session/headless/integration_test.go`
- **This task alone is the Phase 1 gate**: nothing in Epic 1.2, Epic 2.1, or Epic 2.2 depends on it code-wise, but per the hard-blocking rule above, Epic 2.2's stories must not *start* until this task's test is written and green.

##### Task 1.1.1b: Confirm Task 1.1.1a's (and Task 2.1.1e's) smoke tests pass against the production systemd service's actual `claude` CLI/config (~10 min, HARD BLOCKER on Epic 2.2.2/2.2.3 being done — new in this repair pass)
- After Task 2.1.1e (see Epic 2.1 below) lands, SSH/shell into (or otherwise obtain a shell matching) the environment `systemctl --user` resolves for the `stapler-squad` unit — see `.claude/rules/systemd-user-service.md` for how the user-service `PATH`/D-Bus environment can differ from an interactive terminal. Run `claude --version` there and compare against whatever CI's runner reports; note any difference.
- Re-run `go test -race -tags integration ./session/headless/... -run TestPool_RealClaude` in that matched environment. Both `TestPool_RealClaude_WorkDirOnly_GrantsReadAccess` (1.1.1a) and `TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess` (2.1.1e) must pass there, not just in CI.
- Record the result (CLI version used, pass/fail, any environment delta found) as a checklist item in this plan's Post-Implementation Checklist — this is the acceptance-criterion-level check the adversarial review asked for, not just prose reassurance.
- Files: none (verification task — no new test code beyond what 1.1.1a/2.1.1e already added); `project_plans/backlog-already-implemented/implementation/plan.md` (check off the Post-Implementation Checklist item).

---

### Epic 1.2: Surface `report_progress` Notes to the Reviewer

**Goal**: Close the two gaps identified in architecture.md — the note is collected but never rendered (a prompt bug), and even once rendered, the snapshot the reviewer sees is usually stale relative to where the note was written (a data-selection bug, Finding A). Both are required together; fixing only the render side is a silent no-op in the common case.

#### Story 1.2.1: Render `AcCriterion.Note` in both prompt builders (unconditional, all reviews)
**As a** reviewer (headless or legacy), **I want** to see any note the work session attached to a specific criterion, **so that** I have the same self-reported context the work session did, not just its index and text.
**Acceptance Criteria**:
- When a criterion's `Note` is non-empty, both `BuildReviewPrompt` and `BuildHeadlessReviewPrompt` render it directly under that criterion's line, regardless of whether the diff is empty.
  - *Given* `acSnapshot = []AcCriterion{{Index: 0, Text: "Add rate limiting to /api/sessions", Status: "done", Note: "Already present — see rateLimiter middleware in server/middleware/ratelimit.go:22"}}` and a non-empty `diff`, *When* `BuildHeadlessReviewPrompt(item, acSnapshot, diff, false, "")` is called, *Then* the returned string contains both `"0. Add rate limiting to /api/sessions"` and `"Note (self-reported by work session via report_progress): Already present — see rateLimiter middleware in server/middleware/ratelimit.go:22"`.
- When `Note` is empty, no note line is rendered (no behavior change for the common case).
  - *Given* `acSnapshot = []AcCriterion{{Index: 0, Text: "Add rate limiting", Status: "pending", Note: ""}}`, *When* `BuildHeadlessReviewPrompt` is called, *Then* the output contains `"0. Add rate limiting"` and does not contain the string `"Note ("`.
- A note longer than 500 characters is truncated using the existing `sanitizeField` convention, so a verbose or adversarial note cannot balloon every review prompt.
  - *Given* a criterion with `Note` = a 900-character string, *When* either builder renders it, *Then* the rendered note text is capped at 500 characters (matching the existing `c.Text` cap on the same line).
**Files**: `session/backlog_review.go`, `session/backlog_review_test.go`

##### Task 1.2.1a: Render `c.Note` in `BuildReviewPrompt`'s AC loop (~3 min)
- In the AC rendering loop (`session/backlog_review.go:83-86`), after the existing `fmt.Fprintf(&sb, "%d. %s\n", c.Index, sanitizeField(c.Text, 500))`, add: `if c.Note != "" { fmt.Fprintf(&sb, "   Note (self-reported by work session via report_progress): %s\n", sanitizeField(c.Note, 500)) }`.
- Files: `session/backlog_review.go`

##### Task 1.2.1b: Render `c.Note` in `BuildHeadlessReviewPrompt`'s AC loop (~3 min)
- Apply the identical change to the AC rendering loop at `session/backlog_review.go:152-155`.
- Files: `session/backlog_review.go`

##### Task 1.2.1c: Add note-rendering tests for both builders (~5 min)
- Add `TestBuildReviewPrompt_CriterionNote_IncludedWhenPresent`, `TestBuildReviewPrompt_CriterionNote_OmittedWhenEmpty`, `TestBuildReviewPrompt_CriterionNote_TruncatedBeyond500Chars` and the `Headless` equivalents (`TestBuildHeadlessReviewPrompt_CriterionNote_...`) to `session/backlog_review_test.go`, following the existing table style used by `TestBuildHeadlessReviewPrompt_VerificationNotes_IncludedInLabeledSection` (line 131). Use a non-empty diff in at least one case to prove this is unconditional, not diff-gated.
- Files: `session/backlog_review_test.go`

#### Story 1.2.2: Fix AC-snapshot staleness in `ReviewGateRunner.Run` (Finding A)
**As a** reviewer, **I want** the AC list I'm shown to reflect the *current* state of `report_progress` notes, **so that** Story 1.2.1's rendering fix isn't defeated by seeing a pre-note snapshot.
**Acceptance Criteria**:
- When `is.AcSnapshot` was captured before a `report_progress` call wrote a `Note` onto `item.AcceptanceCriteria`, the prompt built for that review includes the note anyway.
  - *Given* `is.AcSnapshot` = `[{"index":0,"text":"Add rate limiting","status":"pending","note":""}]` (frozen at session spawn) and `item.AcceptanceCriteria` = `[{"index":0,"text":"Add rate limiting","status":"done","note":"Already present — server/middleware/ratelimit.go:22"}]` (updated later via `report_progress`), *When* `ReviewGateRunner.Run` builds its prompt, *Then* the prompt contains the note `"Already present — server/middleware/ratelimit.go:22"` and the criterion is rendered with the live `status: done`, not the stale `pending`.
- When `is.AcSnapshot` is empty (legacy/edge case), the existing fallback to `item.AcceptanceCriteria` continues to work unchanged.
  - *Given* `is.AcSnapshot = ""`, *When* the same code runs, *Then* the resulting AC list equals `ParseAcCriteria(item.AcceptanceCriteria)` exactly, as before this change.
**Files**: `session/backlog_review.go`, `session/review_gate.go`, `session/backlog_review_test.go`, `session/review_gate_test.go`

##### Task 1.2.2a: Add `MergeLiveCriterionNotes` helper (~5 min)
- In `session/backlog_review.go`, add:
  ```go
  // MergeLiveCriterionNotes overlays each criterion's live Note and Status
  // (from a session's AcSnapshot's frozen source of truth, item.AcceptanceCriteria)
  // onto snapshot, matched by Index. Fixes the staleness where a report_progress
  // call writes a Note onto the live item after an ItemSession's AcSnapshot was
  // already captured at spawn time — see architecture.md Finding A.
  func MergeLiveCriterionNotes(snapshot, live []AcCriterion) []AcCriterion {
      if len(snapshot) == 0 {
          return live
      }
      liveByIdx := make(map[int]AcCriterion, len(live))
      for _, c := range live {
          liveByIdx[c.Index] = c
      }
      merged := make([]AcCriterion, len(snapshot))
      copy(merged, snapshot)
      for i, c := range merged {
          if lc, ok := liveByIdx[c.Index]; ok {
              if lc.Note != "" {
                  merged[i].Note = lc.Note
              }
              merged[i].Status = lc.Status
          }
      }
      return merged
  }
  ```
- Files: `session/backlog_review.go`

##### Task 1.2.2b: Wire the helper into `ReviewGateRunner.Run`'s snapshot selection (~3 min)
- In `session/review_gate.go:236-240`, replace:
  ```go
  acSnapshot, _ := ParseAcCriteria(is.AcSnapshot)
  if len(acSnapshot) == 0 {
      acSnapshot, _ = ParseAcCriteria(item.AcceptanceCriteria)
  }
  ```
  with:
  ```go
  acSnapshot, _ := ParseAcCriteria(is.AcSnapshot)
  liveAC, _ := ParseAcCriteria(item.AcceptanceCriteria)
  if len(acSnapshot) == 0 {
      acSnapshot = liveAC
  } else {
      acSnapshot = MergeLiveCriterionNotes(acSnapshot, liveAC)
  }
  ```
- Files: `session/review_gate.go`

##### Task 1.2.2c: Add tests for the helper and the wiring (~5 min)
- `session/backlog_review_test.go`: `TestMergeLiveCriterionNotes_OverlaysNoteAndStatusByIndex`, `TestMergeLiveCriterionNotes_SnapshotEmpty_ReturnsLive`, `TestMergeLiveCriterionNotes_LiveNoteEmpty_KeepsSnapshotNote`.
- `session/review_gate_test.go`: `TestReviewGateRunner_MergesLiveCriterionNoteIntoStalePromptWhenSnapshotPredatesIt`, following the existing `TestReviewGateRunner_ThreadsVerificationNotesIntoPrompt` (line 127) pattern — construct `is.AcSnapshot` without the note, `item.AcceptanceCriteria` with it, run, and assert the note text reaches the prompt sent to the fake pool.
- Files: `session/backlog_review_test.go`, `session/review_gate_test.go`

#### Story 1.2.3: Fix the same staleness in `TriggerReReview`'s `resolveACSnapshot`
**As a** reviewer running a manually-triggered re-review, **I want** the same live-note behavior Story 1.2.2 gives the automatic review gate, **so that** re-review doesn't silently regress to the stale-snapshot bug.
**Acceptance Criteria**:
- `resolveACSnapshot` merges live notes/status the same way `ReviewGateRunner.Run` does.
  - *Given* `mostRecentWorkSession.AcSnapshot` = `[{"index":0,"text":"Add rate limiting","status":"pending","note":""}]` and `itemAC` = `[{"index":0,"text":"Add rate limiting","status":"done","note":"already present — ratelimit.go:22"}]`, *When* `resolveACSnapshot(mostRecentWorkSession, itemAC)` is called, *Then* the returned slice's element at index 0 has `Note == "already present — ratelimit.go:22"` and `Status == AcStatusDone`.
**Files**: `server/services/backlog_service_triage.go`, `server/services/backlog_service_test.go`

##### Task 1.2.3a: Update `resolveACSnapshot` to merge live notes (~3 min)
- In `server/services/backlog_service_triage.go:1078-1086`, replace:
  ```go
  func resolveACSnapshot(workSession *session.ItemSessionSummary, itemAC session.AcCriteriaJSON) []session.AcCriterion {
      if workSession != nil && workSession.AcSnapshot != "" {
          if ac, _ := session.ParseAcCriteria(workSession.AcSnapshot); len(ac) > 0 {
              return ac
          }
      }
      ac, _ := session.ParseAcCriteria(itemAC)
      return ac
  }
  ```
  with:
  ```go
  func resolveACSnapshot(workSession *session.ItemSessionSummary, itemAC session.AcCriteriaJSON) []session.AcCriterion {
      live, _ := session.ParseAcCriteria(itemAC)
      if workSession != nil && workSession.AcSnapshot != "" {
          if ac, _ := session.ParseAcCriteria(workSession.AcSnapshot); len(ac) > 0 {
              return session.MergeLiveCriterionNotes(ac, live)
          }
      }
      return live
  }
  ```
- Files: `server/services/backlog_service_triage.go`

##### Task 1.2.3b: Add a unit test for the merged behavior (~4 min)
- Add `TestResolveACSnapshot_MergesLiveNoteIntoStaleWorkSessionSnapshot` and `TestResolveACSnapshot_NoWorkSession_ReturnsLiveAC` to `server/services/backlog_service_test.go` (pure function, no fixtures needed beyond constructing `session.ItemSessionSummary`/`session.AcCriteriaJSON` values directly).
- Files: `server/services/backlog_service_test.go`

---

## Phase 2: Grounded Empty-Diff Verification

### Epic 2.1: Extend the Headless Pool for Bounded Codebase Read Access

**Goal**: Add the `AllowedTools`/`PermissionMode` plumbing decided in ADR-001, and a cost-returning `WorkDir`-aware call variant, without touching any existing call site's behavior when the new fields are unset.

#### Story 2.1.1: Add `AllowedTools`/`PermissionMode` to `CallOptions` and `ProcessRunner`
**As a** review gate call, **I want** to pass explicit, scoped tool flags alongside `WorkDir`, **so that** the empty-diff codebase-read call is read-only by construction (ADR-001), not just by prompt instruction.
**Acceptance Criteria**:
- `CallOptions` has new `AllowedTools string` and `PermissionMode string` fields; existing call sites (triage, `TriggerReReview`'s current `CallOptions{}`) that don't set them are unaffected.
  - *Given* `CallOptions{WorkDir: "/repo"}` (fields unset), *When* `Pool.CallWithOptions` runs, *Then* the constructed subprocess args contain no `--allowedTools`/`--permission-mode` flags — identical to today's behavior.
- When `AllowedTools`/`PermissionMode` are set, the subprocess args include both flags with the given values.
  - *Given* `CallOptions{WorkDir: "/repo", AllowedTools: "Read,Grep,Glob", PermissionMode: "bypassPermissions"}`, *When* the `ProcessRunner`'s tool-access args are constructed, *Then* they equal `["--allowedTools", "Read,Grep,Glob", "--permission-mode", "bypassPermissions"]`, in that order.
**Files**: `session/headless/caller.go`, `session/headless/runner.go`

##### Task 2.1.1a: Add fields to `CallOptions` (~2 min)
- In `session/headless/caller.go:18-25`, add `AllowedTools string` and `PermissionMode string` fields to `CallOptions` with doc comments per ADR-001 (see ADR-001's "Concretely" section for exact wording).
- Files: `session/headless/caller.go`

##### Task 2.1.1b: Add `allowedTools`/`permissionMode` fields, `WithToolAccess`, and `toolAccessArgs` to `ProcessRunner` (~5 min)
- In `session/headless/runner.go`, add `allowedTools string` and `permissionMode string` fields to `ProcessRunner` (line 49-52); add:
  ```go
  // WithToolAccess returns a copy of this ProcessRunner with allowedTools/permissionMode
  // set, preserving any existing workDir. Used by CallWithOptions's WorkDir branch to
  // pass --allowedTools/--permission-mode to a WorkDir-scoped call (see ADR-001).
  func (r *ProcessRunner) WithToolAccess(allowedTools, permissionMode string) *ProcessRunner {
      return &ProcessRunner{claudeBin: r.claudeBin, workDir: r.workDir, allowedTools: allowedTools, permissionMode: permissionMode}
  }

  // toolAccessArgs returns the --allowedTools/--permission-mode flag pairs for r's
  // configured values, in that order. Extracted as a pure function so the flag
  // construction is unit-testable without executing a subprocess.
  func (r *ProcessRunner) toolAccessArgs() []string {
      var extra []string
      if r.allowedTools != "" {
          extra = append(extra, "--allowedTools", r.allowedTools)
      }
      if r.permissionMode != "" {
          extra = append(extra, "--permission-mode", r.permissionMode)
      }
      return extra
  }
  ```
  and in `Run` (line 89), prepend `args = append(args, r.toolAccessArgs()...)` before building `executor.ProcessOption`s. Also update `WithWorkDir` (line 56-58) to preserve `allowedTools`/`permissionMode` when copying, matching the existing pattern.
- Files: `session/headless/runner.go`

##### Task 2.1.1c: Wire `opts.AllowedTools`/`opts.PermissionMode` into `Pool.CallWithOptions`'s `WorkDir` branch (~3 min)
- In `session/headless/caller.go:391-410`, after `dirRunner := pr.WithWorkDir(opts.WorkDir)`, add:
  ```go
  if opts.AllowedTools != "" || opts.PermissionMode != "" {
      dirRunner = dirRunner.WithToolAccess(opts.AllowedTools, opts.PermissionMode)
  }
  ```
- Files: `session/headless/caller.go`

##### Task 2.1.1d: Add unit tests for `toolAccessArgs`/`WithToolAccess` (~5 min)
- In `session/headless/runner_test.go` (create if it doesn't exist — check `ls session/headless/*_test.go` first): `TestProcessRunner_ToolAccessArgs_Empty_WhenNeitherSet`, `TestProcessRunner_ToolAccessArgs_BothSet_ReturnsBothFlagsInOrder`, `TestProcessRunner_ToolAccessArgs_OnlyAllowedTools`, `TestProcessRunner_WithToolAccess_PreservesWorkDir`.
- Files: `session/headless/runner_test.go`

##### Task 2.1.1e: Write `TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess` (~5 min) — moved from "Task 1.1.1b", still a HARD BLOCKER on Epic 2.2
- **Sequencing note (resolves Architecture Review's nitpick and Adversarial Review Blocker 1's contradiction)**: this test was originally drafted as Task 1.1.1b under Epic 1.1 (nominally "Phase 1"), but it depends on this story's `CallOptions.AllowedTools`/`PermissionMode` fields (Task 2.1.1a, "Phase 2") existing — a self-contradiction the architecture review flagged. It is moved here so it runs immediately after Task 2.1.1a lands, while remaining exactly as load-bearing as before: Epic 2.2's stories still may not start until Task 1.1.1a is green, and Epic 2.2.2/2.2.3 are still not "done" until *this* task (2.1.1e) is also green, per the Dependency Visualization above.
- Same fixture as Task 1.1.1a, but pass `CallOptions{WorkDir: tempDir, AllowedTools: "Read,Grep,Glob", PermissionMode: session.PermissionModeBypassPermissions}` to `pool.CallBlocking(...)` (consolidated signature, Story 2.1.2 above). **Resolved in this repair pass (Pre-Mortem P2 #4)**: this is no longer a conditional hedge — Story 2.1.2 now always ships first as PR #1 (see Dependency Visualization), so by the time Task 2.1.1e is written in PR #2, the consolidated `pool.CallBlocking` signature is already merged and this call compiles against the final signature unconditionally.
- Files: `session/headless/integration_test.go`
- Run: `go test -race -tags integration ./session/headless/...` (requires `claude` in `PATH`) and confirm both Task 1.1.1a's and this task's tests pass before starting Epic 2.2. Then complete Task 1.1.1b (production-CLI-parity confirmation) before marking Epic 2.2.2/2.2.3 done.

---

#### Story 2.1.2: Consolidate `Pool`'s blocking-call methods into one `CallBlocking(ctx, key, systemPrompt, userPrompt, opts) (result, costUSD, err)`

**Ships as its own separate PR (PR #1), ahead of every other story in this plan (this repair pass, resolves Pre-Mortem P2 #4)**: this story's 12-call-site consolidation is bundled and merged/soaked independently from Epic 2.2's anti-gaming wiring, so a regression in one of the 8 unrelated call sites (approvals, autonomous driver, session service) cannot be conflated with the safety-critical review-gate change during triage. See the Dependency Visualization's "shipping sequence / PR boundaries" note.

**As a** maintainer of the `headless` package, **I want** exactly one blocking call method instead of a growing M×N matrix of `CallBlocking`/`CallBlockingWithCost`/`CallBlockingWithOptions`/(the originally-proposed `CallBlockingWithCostAndOptions`), **so that** cost tracking works uniformly on every call site — including the new empty-diff codebase-read path — without adding a 4th near-duplicate method (Architecture Review Concern B; see Pattern Decisions for why full consolidation was chosen over scoping this down to accepted debt).

**Verified call-site inventory (12 total, via `grep -rn "\.CallBlocking(\|\.CallBlockingWithCost(\|\.CallBlockingWithOptions(\|\.CallWithOptions(" --include="*.go" .`, excluding `_test.go`)**:
- `CallBlocking` (×4): `session/headless/features.go:116,136,158,171`
- `CallBlockingWithCost` (×1): `session/review_gate.go:252` — rewritten by Story 2.2.2 to call the consolidated method via `BuildReviewCallOptions`; no separate task needed here.
- `CallBlockingWithOptions` (×5): `server/services/approval_handler.go:330`, `server/services/backlog_service_triage.go:671` (triage) and `:871` (re-review — rewritten by Story 2.2.3, no separate task here), `server/services/session_service.go:3437`, `session/autonomous_driver.go:210`
- `CallWithOptions` (×2, streaming — NOT consolidated, out of scope): `server/services/headless_service.go:71`, and the internal call inside `CallBlockingWithOptions`'s own old implementation (becomes internal to the new consolidated `CallBlocking`).
- Three narrow consumer-defined interfaces (per this repo's interface-pollution convention — defined in the consuming package, not next to `Pool`) also mirror the old `CallBlockingWithOptions(ctx, key, systemPrompt, userPrompt, opts) (string, error)` signature and must be updated in lockstep: `session/autonomous_driver.go:20` (`HeadlessPoolClient`), `server/services/approval_handler.go:61` (`headlessPoolApprover`), `session/headless/client.go:7` (`PoolClient`).

**Acceptance Criteria**:
- `Pool.CallBlocking(ctx, key, systemPrompt, userPrompt, opts CallOptions) (string, float64, error)` exists as the sole blocking-call method; `CallBlockingWithCost` and `CallBlockingWithOptions` no longer exist as separate methods.
  - *Given* a `FakeRunner` configured to return a first-call JSON response `{"session_id":"s1","result":"ok","cost_usd":0.0042}`, *When* `pool.CallBlocking(ctx, FeatureKeyReview, "", "prompt", CallOptions{})` is called, *Then* it returns `("ok", 0.0042, nil)` — matching what `CallBlockingWithCost` returned today for the zero-`CallOptions` case.
  - *Given* the same `FakeRunner` and a non-zero `CallOptions{WorkDir: "/tmp/x"}`, *When* `pool.CallBlocking(ctx, FeatureKeyCustom, "", "prompt", CallOptions{WorkDir: "/tmp/x"})` is called, *Then* it returns `("ok", 0.0042, nil)` and the runner received `WorkDir` — matching what `CallBlockingWithOptions` did today, now also with cost.
- All 8 non-review-gate call sites listed above compile and behave identically after the signature change (pass `CallOptions{}` where nothing was passed before, ignore the new cost return with `_` where the caller doesn't use it today).
  - *Given* `session/headless/features.go:116`'s existing call `raw, err := pool.CallBlocking(ctx, FeatureKeySummarize, summarizeSystemPrompt, userPrompt)`, *When* updated to `raw, _, err := pool.CallBlocking(ctx, FeatureKeySummarize, summarizeSystemPrompt, userPrompt, CallOptions{})`, *Then* `TestSummarize*`-family tests (existing, `session/headless/features_test.go`) pass unchanged.
**Files**: `session/headless/caller.go`, `session/headless/client.go`, `session/autonomous_driver.go`, `server/services/approval_handler.go`, `session/headless/features.go`, `server/services/backlog_service_triage.go`, `server/services/session_service.go`, `session/headless/pool_test.go`, and the test files listed in Task 2.1.2d.

##### Task 2.1.2a: Rewrite `Pool.CallBlocking` to the consolidated signature; delete `CallBlockingWithCost` and `CallBlockingWithOptions` (~8 min)
- In `session/headless/caller.go`, replace the three existing methods with one:
  ```go
  // CallBlocking makes a single blocking headless call and returns the result text,
  // the cost in USD reported by claude, and any error. opts is the single place to
  // pass WorkDir/Model/AllowedTools/PermissionMode; the zero value reproduces the
  // simplest call shape (no WorkDir, no tool-access flags). Cost is always parsed
  // from the JSON result at no extra cost to callers that ignore it via `_`.
  //
  // This replaces the former CallBlocking (no opts, no cost), CallBlockingWithCost
  // (no opts, cost), and CallBlockingWithOptions (opts, no cost) — collapsed to one
  // method per the M×N-method-proliferation fix recorded in
  // project_plans/backlog-already-implemented/implementation/plan.md's Pattern
  // Decisions table (Architecture Review Concern B).
  func (p *Pool) CallBlocking(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions) (string, float64, error) {
      ch, err := p.CallWithOptions(ctx, key, systemPrompt, userPrompt, opts)
      if err != nil {
          return "", 0, err
      }
      return drainChannelWithCost(ch)
  }
  ```
- Files: `session/headless/caller.go`

##### Task 2.1.2b: Update the 3 narrow consumer interfaces to the new signature (~4 min)
- `session/autonomous_driver.go:20-22` (`HeadlessPoolClient`), `server/services/approval_handler.go:61-63` (`headlessPoolApprover`), `session/headless/client.go:7-9` (`PoolClient`): change each interface's single method from `CallBlockingWithOptions(ctx, key, systemPrompt, userPrompt string, opts CallOptions) (string, error)` to `CallBlocking(ctx, key, systemPrompt, userPrompt string, opts CallOptions) (string, float64, error)`. `var _ PoolClient = (*Pool)(nil)` in `client.go` continues to compile-time-verify the match.
- Files: `session/autonomous_driver.go`, `server/services/approval_handler.go`, `session/headless/client.go`

##### Task 2.1.2c: Update the 8 non-review-gate call sites (~10 min)
- `session/headless/features.go:116,136,158,171` — each `pool.CallBlocking(ctx, key, systemPrompt, userPrompt)` becomes `_, _, err := pool.CallBlocking(...)`-shaped with `CallOptions{}` appended and the cost return ignored via `_` (these 4 call sites don't need cost today).
- `server/services/approval_handler.go:330`, `server/services/backlog_service_triage.go:671` (triage call only — **not** line 871, which Story 2.2.3 rewrites), `server/services/session_service.go:3437`, `session/autonomous_driver.go:210` — each `CallBlockingWithOptions(...)` call becomes `CallBlocking(...)` with the same arguments (opts already passed positionally), adding `_` for the new cost return where unused.
- Files: `session/headless/features.go`, `server/services/approval_handler.go`, `server/services/backlog_service_triage.go`, `server/services/session_service.go`, `session/autonomous_driver.go`

##### Task 2.1.2d: Update test fakes/mocks and existing assertions referencing the old method names (~8 min)
- Update fake/mock implementations of `HeadlessPoolClient`/`headlessPoolApprover`/`PoolClient` and any direct `pool.CallBlockingWithCost(...)`/`pool.CallBlockingWithOptions(...)` call assertions in: `server/services/autonomous_orchestration_service_test.go`, `server/services/backlog_service_test.go`, `server/services/backlog_triage_harness_test.go`, `session/autonomous_driver_test.go`, `session/headless/integration_test.go`, `session/headless/pool_test.go` (found via `grep -rl "CallBlockingWithCost(\|CallBlockingWithOptions(\|\.CallBlocking(" --include="*_test.go" .`). Each fake's method signature must match the new `CallBlocking(ctx, key, systemPrompt, userPrompt, opts) (string, float64, error)`.
- Files: the six test files listed above

##### Task 2.1.2e: Add a test for the consolidated method's two behavior modes (~4 min)
- `TestPool_CallBlocking_ZeroValueOptions_MatchesLegacyCallBlockingBehavior` and `TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir` in `session/headless/pool_test.go`, mirroring the existing `TestPool_FirstCall_CostUSD_ForwardedOnDoneChunk` (line 471) pattern for cost assertions.
- Files: `session/headless/pool_test.go`

---

### Epic 2.2: Empty-Diff Reviewer Prompt & Wiring

**Goal**: Give the reviewer both the instructions and the actual tool access to independently verify criteria when `diff == ""`, on both the automatic review gate and manual re-review paths, without touching the non-empty-diff prompt — via one shared decision helper instead of duplicated branching (Adversarial Review Concern A), and with an explicit degrade-to-safe path when tool access silently fails (Adversarial Review Blocker 2).

#### Story 2.2.1: Add the codebase-verification system prompt, the shared `BuildReviewCallOptions` helper, and the `tool_reads` evidence schema
**As a** reviewer facing an empty diff, **I want** explicit falsification-framed instructions, my own read access, and a way to prove I actually used it, **so that** I verify the claim against the tree instead of judging its prose plausibility — and so the two call sites that grant this access can never silently diverge.
**Acceptance Criteria**:
- A new `headless.HeadlessReviewSystemPromptWithCodebaseAccess()` exists, is distinct from `HeadlessReviewSystemPrompt()`, instructs the model to independently verify each criterion and cite its own evidence rather than the agent's claim, and requires a `tool_reads` array listing every file path actually opened.
  - *Given* the new prompt string, *When* inspected, *Then* it contains the phrase `"your own"` (or equivalent citation-ownership language), contains `"tool_reads"`, and does not contain the diff-only phrase `"quote from diff"` used by the normal-path prompt.
- Both `BuildReviewPrompt` and `BuildHeadlessReviewPrompt`'s `diff == ""` branch render an additional `## No-Diff Verification` section; the non-empty-diff branch is completely unchanged.
  - *Given* `diff = ""`, *When* `BuildHeadlessReviewPrompt(item, ac, "", false, "")` is called, *Then* the output contains `"## No-Diff Verification"`.
  - *Given* `diff = "diff --git a/foo.go b/foo.go\n..."` (non-empty), *When* the same function is called, *Then* the output does NOT contain `"## No-Diff Verification"` — proving no leakage into the normal path (research/pitfalls.md item 4's regression guard).
- A single pure helper `BuildReviewCallOptions(diff, codebaseWorkDir string) (systemPrompt string, opts headless.CallOptions, callTimeout time.Duration, path string)` exists in `session/backlog_review.go` and is the only place the `diff == ""` branch's `CallOptions`/system-prompt/timeout decision is written.
  - *Given* `diff = ""` and `codebaseWorkDir = "/repo"`, *When* `BuildReviewCallOptions("", "/repo")` is called, *Then* it returns `headless.HeadlessReviewSystemPromptWithCodebaseAccess()`, `headless.CallOptions{WorkDir: "/repo", AllowedTools: "Read,Grep,Glob", PermissionMode: PermissionModeBypassPermissions}`, `headless.CodebaseReadCallTimeout`, and `"codebase-read"`.
  - *Given* a non-empty `diff`, *When* `BuildReviewCallOptions(diff, "/repo")` is called, *Then* it returns `headless.HeadlessReviewSystemPrompt()`, zero-value `headless.CallOptions{}`, `headless.DefaultCallTimeout`, and `"diff"`.
**Files**: `session/headless/features.go`, `session/backlog_review.go`, `session/headless/features_test.go`, `session/backlog_review_test.go`

##### Task 2.2.1a: Add `headlessReviewSystemPromptWithCodebaseAccess` constant and accessor, requiring `tool_reads` (~6 min)
- In `session/headless/features.go`, after `headlessReviewSystemPrompt` (line 82-87), add:
  ```go
  // headlessReviewSystemPromptWithCodebaseAccess is used for empty-diff headless review
  // calls that ARE granted read-only tool access (Read/Grep/Glob) via CallOptions.WorkDir.
  // Uses falsification framing: the model must independently locate and quote its OWN
  // evidence, treating the work session's claim as a hypothesis to check, not a fact to
  // accept — see research/pitfalls.md's anti-gaming guidance. Also requires a "tool_reads"
  // list of files actually opened, so the caller can detect a PASS/FAIL reached with no
  // real evidence of tool use and degrade it (see Story 2.2.4 / Blocker 2).
  const headlessReviewSystemPromptWithCodebaseAccess = `You are a code review agent reviewing a backlog item where no diff was found — either the acceptance criteria were already satisfied before this session started, or no work was done. You have read-only tool access (Read, Grep, Glob) scoped to the item's own repository checkout. Use it to independently verify each criterion:
  1. Search the current codebase yourself for code that satisfies each criterion. Treat the work session's note or verification evidence as a hypothesis to falsify, not a citation to trust — a criterion is not PASS merely because the agent claims it is already implemented.
  2. When you find satisfying code, your evidence field must quote YOUR OWN file path plus line/symbol and snippet from what you read — not a restatement of the agent's claim.
  3. If you cannot locate satisfying code — including because it would live in a vendored or dependency path you cannot fully inspect — mark that criterion UNVERIFIABLE, not PASS.
  4. Do not modify any files. Do not run destructive or write commands.
  5. List EVERY file path you actually opened with Read/Grep/Glob during this review in "tool_reads", even if it didn't contain relevant code. If you did not use any tool, leave "tool_reads" empty — do not fabricate entries; an empty list on a confident verdict will be treated as unverified.
  Output ONLY a single JSON object — no other text before or after it:
  {"overall":"PASS","summary":"concise assessment","tool_reads":["path/to/file.go","path/to/other.go"],"verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"file:line — quoted snippet you read yourself"}]}
  Valid outcome values: PASS, FAIL, PARTIAL, UNVERIFIABLE. Set overall to PASS only when every criterion passes.`

  // HeadlessReviewSystemPromptWithCodebaseAccess returns the system prompt used for
  // empty-diff headless review calls granted codebase read access. See ADR-001.
  func HeadlessReviewSystemPromptWithCodebaseAccess() string { return headlessReviewSystemPromptWithCodebaseAccess }

  // CodebaseReadCallTimeout is the context timeout used for the empty-diff codebase-read
  // headless call — deliberately shorter than DefaultCallTimeout (900s) so a hung or
  // degraded tool-access call fails fast into the UNVERIFIABLE degrade path instead of
  // blocking the review gate for up to 15 minutes. See Story 2.2.4 / plan.md Pattern
  // Decisions for the value's justification.
  const CodebaseReadCallTimeout = 150 * time.Second
  ```
- Files: `session/headless/features.go`

##### Task 2.2.1b: Add the `## No-Diff Verification` branch to both prompt builders (~5 min)
- In `session/backlog_review.go`, in `BuildReviewPrompt`'s diff block (line 107-108) and `BuildHeadlessReviewPrompt`'s diff block (line 170-171), change the `if diff == ""` branch from just `sb.WriteString("(no diff available)\n")` to:
  ```go
  if diff == "" {
      sb.WriteString("(no diff available — no committed code changes were found for this session)\n\n")
      sb.WriteString("## No-Diff Verification\n")
      sb.WriteString("This can mean the criteria were already satisfied before this session started, or that no work happened. Check each criterion against the CURRENT codebase yourself using your available tools before verdicting; do not rely on the work session's note or verification evidence alone.\n\n")
  } else {
  ```
  (Same text in both builders — `BuildReviewPrompt`'s tool-based legacy path already always has tool access, so this text applies there too even though it is currently unreachable in production per stack.md; kept symmetric and low-cost rather than special-cased.)
- Files: `session/backlog_review.go`

##### Task 2.2.1c: Add tests for the new prompt and the no-leakage guarantee (~5 min)
- `session/headless/features_test.go`: `TestHeadlessReviewSystemPromptWithCodebaseAccess_DistinctFromNormalPrompt`, `TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresOwnCitation`, `TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresToolReadsField`.
- `session/backlog_review_test.go`: `TestBuildHeadlessReviewPrompt_EmptyDiff_ContainsNoDiffVerificationSection`, `TestBuildHeadlessReviewPrompt_NonEmptyDiff_OmitsNoDiffVerificationSection` (the regression guard against leakage), and the `BuildReviewPrompt` equivalents.
- Files: `session/headless/features_test.go`, `session/backlog_review_test.go`

##### Task 2.2.1d: Add the shared `BuildReviewCallOptions` helper (~6 min) — resolves Adversarial Review Concern A
- In `session/backlog_review.go`, add:
  ```go
  // BuildReviewCallOptions decides the headless review call's system prompt, CallOptions,
  // and context timeout for a given diff state. This is the single point of decision for
  // the empty-diff codebase-access branch (WorkDir + AllowedTools + PermissionMode + system
  // prompt + stricter timeout) — both ReviewGateRunner.Run and TriggerReReview must call
  // this instead of independently constructing the same literals, so ADR-001's "reviewer
  // must never get write tools" invariant and the degrade-path timeout (Story 2.2.4) have
  // exactly one place to audit, not two. See Adversarial Review Concern A.
  func BuildReviewCallOptions(diff, codebaseWorkDir string) (systemPrompt string, opts headless.CallOptions, callTimeout time.Duration, path string) {
      if diff == "" {
          return headless.HeadlessReviewSystemPromptWithCodebaseAccess(),
              headless.CallOptions{
                  WorkDir:        codebaseWorkDir,
                  AllowedTools:   "Read,Grep,Glob",
                  PermissionMode: PermissionModeBypassPermissions,
              },
              headless.CodebaseReadCallTimeout,
              "codebase-read"
      }
      return headless.HeadlessReviewSystemPrompt(), headless.CallOptions{}, headless.DefaultCallTimeout, "diff"
  }
  ```
  (Add `"time"` to `session/backlog_review.go`'s imports if not already present.)
- Files: `session/backlog_review.go`

##### Task 2.2.1e: Add tests for `BuildReviewCallOptions` (~5 min)
- `session/backlog_review_test.go`: `TestBuildReviewCallOptions_EmptyDiff_ReturnsCodebaseAccessOptionsAndShortTimeout`, `TestBuildReviewCallOptions_NonEmptyDiff_ReturnsPlainOptionsAndDefaultTimeout`, `TestBuildReviewCallOptions_EmptyDiff_NeverIncludesWriteTools` (asserts `AllowedTools` never contains `"Bash"` or `"Write"` — a regression guard for ADR-001's hard invariant, cheap to add now that there's exactly one function to assert it against).
- Files: `session/backlog_review_test.go`

#### Story 2.2.2: Wire `ReviewGateRunner.Run`'s headless call through `BuildReviewCallOptions`
**As a** review gate call on an empty-diff item, **I want** the pool call to actually use the new codebase-access mechanism via the shared helper, **so that** Story 2.2.1's prompt has real tool access behind it and cannot drift from `TriggerReReview`'s wiring.
**Acceptance Criteria**:
- The headless call is built via `BuildReviewCallOptions(diff, codebaseWorkDir)` and uses the consolidated `pool.CallBlocking` (Story 2.1.2) with the returned `opts` and a context bounded by the returned `callTimeout`.
  - *Given* an item with `RepoPath = "/home/tstapler/repos/stapler-squad"`, a resolved worktree `wt.WorktreePath = "/home/tstapler/.stapler-squad/worktrees/item-42"`, and `diff == ""`, *When* `ReviewGateRunner.Run` reaches the headless call, *Then* it invokes `pool.CallBlocking` with `CallOptions{WorkDir: "/home/tstapler/.stapler-squad/worktrees/item-42", AllowedTools: "Read,Grep,Glob", PermissionMode: "bypassPermissions"}`, system prompt `headless.HeadlessReviewSystemPromptWithCodebaseAccess()`, and a context timeout of `headless.CodebaseReadCallTimeout` (150s), not `headless.DefaultCallTimeout`.
- When `diff != ""`, behavior is completely unchanged — same `CallOptions{}` (no `WorkDir`), same `HeadlessReviewSystemPrompt()`, same `headless.DefaultCallTimeout`.
  - *Given* a non-empty `diff`, *When* the same code runs, *Then* the pool call uses zero-value `CallOptions{}`, `headless.HeadlessReviewSystemPrompt()`, and `headless.DefaultCallTimeout`, exactly as before this change.
**Files**: `session/review_gate.go`, `session/review_gate_test.go`

##### Task 2.2.2a: Branch the headless call via `BuildReviewCallOptions` (~6 min)
- In `session/review_gate.go`, replace the call at line 248-252:
  ```go
  reviewCtx, reviewCancel := context.WithTimeout(ctx, headless.DefaultCallTimeout)
  defer reviewCancel()

  headlessPrompt := BuildHeadlessReviewPrompt(item, acSnapshot, diff, truncated, is.VerificationNotes)
  reviewResult, callCostUSD, callErr := pool.CallBlockingWithCost(reviewCtx, headless.FeatureKeyReview, headless.HeadlessReviewSystemPrompt(), headlessPrompt)
  ```
  with:
  ```go
  codebaseWorkDir := wt.WorktreePath
  if codebaseWorkDir == "" {
      codebaseWorkDir = item.RepoPath
  }
  systemPrompt, callOpts, callTimeout, reviewPath := BuildReviewCallOptions(diff, codebaseWorkDir)

  reviewCtx, reviewCancel := context.WithTimeout(ctx, callTimeout)
  defer reviewCancel()

  headlessPrompt := BuildHeadlessReviewPrompt(item, acSnapshot, diff, truncated, is.VerificationNotes)
  reviewResult, callCostUSD, callErr := pool.CallBlocking(reviewCtx, headless.FeatureKeyReview, systemPrompt, headlessPrompt, callOpts)
  ```
  (`reviewPath` starts as `"diff"` or `"codebase-read"` from the helper; Story 2.2.4 below refines it to `"codebase-read-verified"`/`"codebase-read-degraded"` after the call returns, and Epic 2.5 logs it.)
- Files: `session/review_gate.go`

##### Task 2.2.2b: Add tests for both branches, including the timeout (~6 min)
- `TestReviewGateRunner_EmptyDiff_UsesWorkDirAndCodebaseAccessPrompt`, `TestReviewGateRunner_NonEmptyDiff_UsesPlainCallOptions`, and `TestReviewGateRunner_EmptyDiff_UsesShorterCodebaseReadTimeout` in `session/review_gate_test.go`, following the `FakeRunner`-args-capture pattern already used elsewhere in that file (see `TestReviewGateRunner_HeadlessPassPath`, line 61) — inspect the args/system-prompt/context-deadline the fake pool/runner received.
- Files: `session/review_gate_test.go`

#### Story 2.2.3: Apply the same wiring to `TriggerReReview`
**As a** manually-triggered re-review on an empty-diff item, **I want** the same codebase-verification behavior the automatic review gate gets, via the same shared helper, **so that** re-review cannot silently diverge from Story 2.2.2's guardrails.
**Acceptance Criteria**:
- `TriggerReReview`'s headless call is built via the same `BuildReviewCallOptions(workSessionDiff, codebaseWorkDir)` call and the consolidated `pool.CallBlocking`.
  - *Given* `workSessionDiff = ""` and `mostRecentWorkSession` resolving to a worktree at `/home/tstapler/.stapler-squad/worktrees/item-42`, *When* `TriggerReReview` reaches its headless call, *Then* it passes `CallOptions{WorkDir: "/home/tstapler/.stapler-squad/worktrees/item-42", AllowedTools: "Read,Grep,Glob", PermissionMode: session.PermissionModeBypassPermissions}`, `headless.HeadlessReviewSystemPromptWithCodebaseAccess()`, and a context timeout of `headless.CodebaseReadCallTimeout`.
- When `workSessionDiff != ""`, the existing `CallOptions{}` / `HeadlessReviewSystemPrompt()` / `headless.DefaultCallTimeout` call is unchanged.
**Files**: `server/services/backlog_service_triage.go`, `server/services/backlog_service_test.go`

##### Task 2.2.3a: Branch `TriggerReReview`'s headless call via `BuildReviewCallOptions` (~6 min)
- In `server/services/backlog_service_triage.go:866-876`, replace the block calling `context.WithTimeout(ctx, headless.DefaultCallTimeout)` + `s.headlessPool.CallBlockingWithOptions(... headless.CallOptions{})` with: resolve `codebaseWorkDir` via `s.storage.GetWorktreeDataBySessionUUID(ctx, mostRecentWorkSession.SessionUUID)` falling back to `item.RepoPath`, call `systemPrompt, callOpts, callTimeout, reviewPath := session.BuildReviewCallOptions(workSessionDiff, codebaseWorkDir)`, wrap `ctx` with `callTimeout` (not the hardcoded `headless.DefaultCallTimeout`), and call `s.headlessPool.CallBlocking(reviewCtx, headless.FeatureKeyReview, systemPrompt, headlessPrompt, callOpts)` (the consolidated method from Story 2.1.2 — `TriggerReReview` didn't track cost before, so `_` the returned `costUSD` here, matching the "ignore what you don't need" pattern from Task 2.1.2c).
- Files: `server/services/backlog_service_triage.go`

##### Task 2.2.3b: Add a test (~5 min)
- `TestTriggerReReview_EmptyDiff_UsesWorkDirAndCodebaseAccessPrompt` and `TestTriggerReReview_EmptyDiff_UsesShorterCodebaseReadTimeout` in `server/services/backlog_service_test.go`, mirroring Task 2.2.2b's approach at the service-method level (existing tests in that file already construct `BacklogService` with a fake/injected headless pool — follow that pattern).
- Files: `server/services/backlog_service_test.go`

#### Story 2.2.4: Degrade to `UNVERIFIABLE` on Timeout or Missing Tool-Use Evidence (Codebase-Read Path Only) — NEW, resolves Adversarial Review Blocker 2
**As** the review gate, **I want** a codebase-read call that hangs, times out, or returns a confident verdict with no evidence it actually used a tool to be treated as "could not verify" rather than trusted or left to hang, **so that** a silent WorkDir-access failure cannot produce either (a) a 900s+ stall on every empty-diff review, or (b) a hallucinated PASS/FAIL citing file:line "evidence" the model never actually read — the exact anti-gaming failure this feature exists to prevent.
**Acceptance Criteria**:
- On the codebase-read path (`path == "codebase-read"` from `BuildReviewCallOptions`), if the call errors because `reviewCtx`'s `CodebaseReadCallTimeout` (150s) was exceeded, the review is recorded as `UNVERIFIABLE` (not `FAIL`), with a summary noting the timeout, and the log line uses `path=codebase-read-degraded`.
  - *Given* a `FakeRunner` that never responds within the test's simulated `CodebaseReadCallTimeout`, *When* `ReviewGateRunner.Run` processes an empty-diff item, *Then* the persisted `ReviewVerdictData.OverallOutcome` is `ReviewVerdictUnverifiable`, `Summary` mentions "timed out" or "codebase-read", and the completion log line contains `path=codebase-read-degraded`. (Contrast with the existing non-codebase-read error path, `Task 2.2.2a`'s unchanged `callErr != nil` branch for the `diff != ""` case, which still records `FAIL` exactly as today.)
- On the codebase-read path, if the call succeeds but parses to a `PASS` or `FAIL` overall outcome with an empty `tool_reads` list, the outcome (and every per-criterion verdict) is force-downgraded to `UNVERIFIABLE` before being persisted, with the original overall value preserved in the summary text, and the log line uses `path=codebase-read-degraded`.
  - *Given* a headless response `{"overall":"PASS","summary":"looks done","tool_reads":[],"verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"trust me"}]}` on the codebase-read path, *When* `ReviewGateRunner.Run` parses the result, *Then* the persisted `OverallOutcome` is `ReviewVerdictUnverifiable` (never `ReviewVerdictPass`), every entry in `PerCriterion` has `Outcome == ReviewOutcomeUnverifiable`, and `Summary` contains both the degrade explanation and the original claimed outcome (`"PASS"`) for debuggability.
- On the codebase-read path, a verdict with a non-empty `tool_reads` list where **every listed path exists** (verified via `os.Stat` against `codebaseWorkDir`) is trusted as normal (no forced downgrade) and logged as `path=codebase-read-verified`.
  - *Given* the same response shape but `"tool_reads":["auth/login.go"]` where `auth/login.go` actually exists under `codebaseWorkDir`, *When* the same code runs, *Then* the persisted `OverallOutcome` is `ReviewVerdictPass` (not downgraded) and the log line contains `path=codebase-read-verified`.
- **(NEW — Pre-Mortem P1 #1, closes the confabulation gap Failure #1 identified)** On the codebase-read path, a verdict with a non-empty, *plausible-looking* `tool_reads` list is still force-downgraded to `UNVERIFIABLE` if ANY listed path does not actually exist under `codebaseWorkDir` — regardless of how confident the claimed `overall`/`evidence` looks. This is a cheap, non-LLM `os.Stat` check (`verifyToolReadsExist`), not another agentic round trip, so it closes the gap where a confabulating model fabricates both `evidence` and a non-empty `tool_reads` list for a criterion that isn't actually implemented, sailing past the "is `tool_reads` empty?" check with a false PASS and zero downstream signal.
  - *Given* a headless response `{"overall":"PASS","summary":"looks done","tool_reads":["auth/does_not_exist.go"],"verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"auth/does_not_exist.go:12 — validateLogin() rejects empty passwords"}]}` on the codebase-read path, where `auth/does_not_exist.go` does not exist anywhere under `codebaseWorkDir`, *When* `ReviewGateRunner.Run` parses the result, *Then* the persisted `OverallOutcome` is `ReviewVerdictUnverifiable` (never `ReviewVerdictPass`), every entry in `PerCriterion` has `Outcome == ReviewOutcomeUnverifiable`, `Summary` names the nonexistent path, and the log line contains `path=codebase-read-degraded`.
  - *Given* a `tool_reads` list with two entries, one that exists and one that does not, *When* `DegradeIfUnverified` runs, *Then* the outcome is still force-downgraded (one bad path is enough — the check is not majority-vote).
- On the non-empty-diff (`path == "diff"`) path, none of this new logic applies — behavior, timeout, and log values are byte-for-byte unchanged from before this feature.
  - *Given* `diff != ""`, *When* any of the above scenarios' shape is fed through the diff path (e.g. an empty `tool_reads`-equivalent-shaped response, since the normal prompt has no `tool_reads` field at all), *Then* no downgrade logic runs and the existing `ParseHeadlessVerdictResult`/log-line behavior is exactly as it is today.
**Files**: `session/backlog_review.go`, `session/review_gate.go`, `server/services/backlog_service_triage.go`, `session/backlog_review_test.go`, `session/review_gate_test.go`, `server/services/backlog_service_test.go`

##### Task 2.2.4a: Add `ParseHeadlessToolReads` and the degrade decision helper (~6 min)
- In `session/backlog_review.go`, extend `headlessVerdictJSON` (line 194-198) with a `ToolReads []string `json:"tool_reads"`` field, and add:
  ```go
  // ParseHeadlessToolReads extracts the tool_reads list from a headless LLM JSON
  // response — the files the reviewer claims to have actually opened. Returns nil
  // if the field is absent or the JSON doesn't parse (same lenient-parsing posture
  // as ParseHeadlessVerdictResult). Kept as a sibling function rather than changing
  // ParseHeadlessVerdictResult's signature, to avoid touching that function's
  // existing call sites/tests for an empty-diff-only concern.
  func ParseHeadlessToolReads(text string) []string { /* mirrors ParseHeadlessVerdictResult's JSON-extraction logic */ }

  // verifyToolReadsExist does a cheap, non-LLM os.Stat on every path in toolReads,
  // resolved relative to codebaseWorkDir (absolute paths are stat'd as-is). Returns
  // false if ANY claimed path does not exist. This is the Pre-Mortem P1 #1 fix: a
  // confabulating model can fabricate plausible-looking evidence text, but it cannot
  // make os.Stat succeed for a path it invented — so this closes the gap where a
  // fabricated, non-empty tool_reads list would otherwise sail past the "is it empty?"
  // check with zero downstream signal.
  func verifyToolReadsExist(codebaseWorkDir string, toolReads []string) bool {
      for _, p := range toolReads {
          resolved := p
          if !filepath.IsAbs(p) {
              resolved = filepath.Join(codebaseWorkDir, p)
          }
          if _, err := os.Stat(resolved); err != nil {
              return false
          }
      }
      return true
  }

  // DegradeIfUnverified force-downgrades overall/verdicts to UNVERIFIABLE when path is
  // "codebase-read" and EITHER toolReads is empty OR any claimed tool_reads path does
  // not actually exist under codebaseWorkDir (verifyToolReadsExist, Pre-Mortem P1 #1) —
  // the checkable heuristics for "no evidence a tool was actually invoked" (Adversarial
  // Review Blocker 2) and "the claimed evidence is fabricated" (Pre-Mortem P1 #1).
  // Returns the possibly-downgraded outcome, verdicts, an annotated summary, and the
  // refined path label ("codebase-read-verified" or "codebase-read-degraded") for
  // logging. No-op (returns inputs unchanged, path unchanged) when path != "codebase-read".
  func DegradeIfUnverified(path string, overall ReviewOutcome, verdicts []CriterionVerdict, summary string, toolReads []string, codebaseWorkDir string) (ReviewOutcome, []CriterionVerdict, string, string) {
      if path != "codebase-read" {
          return overall, verdicts, summary, path
      }
      if len(toolReads) > 0 && verifyToolReadsExist(codebaseWorkDir, toolReads) {
          return overall, verdicts, summary, "codebase-read-verified"
      }
      if overall != ReviewOutcomeUnverifiable {
          downgraded := make([]CriterionVerdict, len(verdicts))
          for i, v := range verdicts {
              v.Outcome = ReviewOutcomeUnverifiable
              downgraded[i] = v
          }
          reason := "no tool_reads evidence"
          if len(toolReads) > 0 {
              reason = fmt.Sprintf("tool_reads claimed a path that does not exist under %s", codebaseWorkDir)
          }
          summary = fmt.Sprintf("Degraded to UNVERIFIABLE: codebase-read reviewer returned %s with %s — treated as unverified, not trusted. Original summary: %s", overall, reason, summary)
          return ReviewOutcomeUnverifiable, downgraded, summary, "codebase-read-degraded"
      }
      return overall, verdicts, summary, "codebase-read-degraded"
  }
  ```
  (Add `"os"` and `"path/filepath"` to `session/backlog_review.go`'s imports if not already present.)
- Files: `session/backlog_review.go`

##### Task 2.2.4b: Wire the degrade helper and timeout-specific handling into `ReviewGateRunner.Run` (~7 min)
- In `session/review_gate.go`, after the `callErr != nil` check (the block from Task 2.2.2a, previously lines 253-272), branch on timeout for the codebase-read path specifically: if `reviewPath == "codebase-read" && errors.Is(reviewCtx.Err(), context.DeadlineExceeded)`, create an `ItemSessionData`/`ReviewVerdictData` pair with `OverallOutcome: ReviewVerdictUnverifiable` and a summary noting the timeout (instead of the existing `ReviewVerdictFail` path, which remains exactly as-is for every other error case), and set `reviewPath = "codebase-read-degraded"` before the log line.
- After the existing `overall, perCriterion, summary := ParseHeadlessVerdictResult(reviewResult)` call (Task's prior line ~274), add `toolReads := ParseHeadlessToolReads(reviewResult)` and `overall, perCriterion, summary, reviewPath = DegradeIfUnverified(reviewPath, overall, perCriterion, summary, toolReads, codebaseWorkDir)` (note the added `codebaseWorkDir` argument, Pre-Mortem P1 #1 — the same variable resolved in Task 2.2.2a) before the verdict is persisted, so the degrade applies before `CreateItemSessionWithVerdict` and before `applyVerdictsToACs`.
- Files: `session/review_gate.go`

##### Task 2.2.4c: Wire the same degrade logic into `TriggerReReview` (~5 min)
- Apply the identical timeout-branch and `DegradeIfUnverified` wiring to `server/services/backlog_service_triage.go`'s headless call (from Task 2.2.3a), immediately after its `ParseHeadlessVerdictResult` call, before `CreateItemSessionWithVerdict`, passing the same `codebaseWorkDir` resolved in Task 2.2.3a as `DegradeIfUnverified`'s new final argument (Pre-Mortem P1 #1).
- Files: `server/services/backlog_service_triage.go`

##### Task 2.2.4d: Add tests for the degrade paths (~12 min, expanded in this repair pass)
- `session/backlog_review_test.go`: `TestParseHeadlessToolReads_ExtractsListFromValidJSON`, `TestParseHeadlessToolReads_ReturnsNilWhenAbsent`, `TestDegradeIfUnverified_DiffPath_NoOp`, `TestDegradeIfUnverified_CodebaseReadPath_NonEmptyToolReads_AllPathsExist_NoDowngrade`, `TestDegradeIfUnverified_CodebaseReadPath_EmptyToolReadsOnPass_DowngradesToUnverifiable`, `TestDegradeIfUnverified_CodebaseReadPath_EmptyToolReadsOnFail_DowngradesToUnverifiable`, `TestDegradeIfUnverified_CodebaseReadPath_AlreadyUnverifiable_LabeledDegradedNotDoubleWrapped`, and **(NEW — Pre-Mortem P1 #1)** `TestDegradeIfUnverified_ToolReadsPathDoesNotExist_ForcesUnverifiable` (a `tool_reads` entry pointing at a path that does not exist under a real `t.TempDir()`-based `codebaseWorkDir` forces `UNVERIFIABLE` despite a confident PASS/`evidence`), `TestDegradeIfUnverified_ToolReadsOnePathMissingAmongMultiple_ForcesUnverifiable` (one bad path among several good ones still downgrades), `TestVerifyToolReadsExist_AllPathsExist_ReturnsTrue`, `TestVerifyToolReadsExist_OnePathMissing_ReturnsFalse`, `TestVerifyToolReadsExist_ResolvesRelativeToCodebaseWorkDir`.
- `session/review_gate_test.go`: `TestReviewGateRunner_CodebaseReadTimeout_RecordsUnverifiableNotFail`, `TestReviewGateRunner_CodebaseReadEmptyToolReads_DowngradesPassToUnverifiable`, and **(NEW)** `TestReviewGateRunner_CodebaseReadFabricatedToolReadsPath_DowngradesPassToUnverifiable` (wiring-level: a `FakeRunner` response citing a nonexistent path is downgraded end-to-end through `ReviewGateRunner.Run`).
- `server/services/backlog_service_test.go`: `TestTriggerReReview_CodebaseReadTimeout_RecordsUnverifiableNotFail`, `TestTriggerReReview_CodebaseReadEmptyToolReads_DowngradesPassToUnverifiable`, and **(NEW)** `TestTriggerReReview_CodebaseReadFabricatedToolReadsPath_DowngradesPassToUnverifiable`.
- Files: `session/backlog_review_test.go`, `session/review_gate_test.go`, `server/services/backlog_service_test.go`

---

#### Story 2.2.5: Bound the Evidentiary Weight of an "Already Implemented" Note Outside the Empty-Diff Path — NEW, resolves Pre-Mortem P1 #3

**As** the review gate, **I want** a criterion's self-reported "already implemented" Note to never be, by itself, sufficient for PASS when the overall diff is non-empty, **so that** a mixed/partial PR cannot slip a falsely-claimed-already-implemented criterion past review just because the *rest* of the PR has a real diff — the exact gap Story 1.2.1's unconditional Note rendering (this Note is now shown on every review, not just empty-diff ones) newly creates if left unaddressed.

**Chosen resolution (per Pattern Decisions table, choice (b))**: the codebase-read tool-access mechanism stays diff-level (`diff == ""`), not criterion-level — extending it per-criterion was rejected as disproportionate to a "focused feature" appetite (see Pattern Decisions). Instead, the *evidentiary weight* of a Note is explicitly downgraded outside the `diff == ""` path: it is informational context the reviewer should consider, but can never by itself be the sole basis for a PASS verdict when a diff is present — the reviewer must still find diff-based or independently-verifiable support for that criterion.

**Acceptance Criteria**:
- `headlessReviewSystemPrompt` (the *normal*, non-empty-diff system prompt — distinct from `headlessReviewSystemPromptWithCodebaseAccess`) contains an explicit instruction that a criterion's self-reported Note claiming "already implemented" is informational only and is never sufficient by itself for that criterion's PASS when a diff is present.
  - *Given* the `headlessReviewSystemPrompt` string, *When* inspected, *Then* it contains language equivalent to: "A criterion's self-reported Note (e.g. claiming a criterion was already implemented before this session) is context only, not evidence — it must never by itself be treated as satisfying a criterion. If the diff does not support a criterion, mark it FAIL or UNVERIFIABLE regardless of what its Note claims."
- The legacy `BuildReviewPrompt`'s equivalent instruction text carries the same rule (kept symmetric with Task 2.2.1b's precedent of applying no-diff-verification text to both builders).
- **(Real-LLM adversarial fixture, see Epic 2.3 Story 2.3.5)** Given a non-empty diff that does not touch the criterion in question, and that criterion's Note falsely claims "already implemented — see `<real-but-unrelated-or-nonexistent path>`", the real headless reviewer call does not return PASS for that criterion.
**Files**: `session/headless/features.go`, `session/headless/features_test.go`

##### Task 2.2.5a: Append the evidentiary-weight instruction to `headlessReviewSystemPrompt` (~4 min)
- In `session/headless/features.go`, extend the existing `headlessReviewSystemPrompt` constant (the plain, non-codebase-access one used when `diff != ""`) with an additional sentence: `"A criterion's self-reported Note (e.g. 'already implemented, no diff needed') is informational context only, not evidence. It is never sufficient by itself for that criterion's PASS — you must still find the criterion's satisfying change reflected in the diff itself, or mark it FAIL/UNVERIFIABLE."` Do not change `headlessReviewSystemPromptWithCodebaseAccess` (Task 2.2.1a) — that prompt already requires independent verification unconditionally, so this instruction would be redundant there.
- Files: `session/headless/features.go`

##### Task 2.2.5b: Apply the same instruction text to the legacy `BuildReviewPrompt`'s reviewer instructions (~3 min)
- Apply the equivalent sentence to whatever instructional text precedes `BuildReviewPrompt`'s AC list (check the existing function for where general reviewer instructions live, following the same pattern Task 2.2.1b used to keep both builders symmetric).
- Files: `session/backlog_review.go`

##### Task 2.2.5c: Add prompt-content tests (~4 min)
- `session/headless/features_test.go`: `TestHeadlessReviewSystemPrompt_NoteOnNonEmptyDiffIsInformationalOnly` (string-containment assertion), `TestHeadlessReviewSystemPromptWithCodebaseAccess_UnaffectedByEvidentiaryWeightChange` (regression guard — the empty-diff prompt's own unconditional-verification language is unchanged by this story).
- Files: `session/headless/features_test.go`

---

#### Story 2.2.6: Runtime `CodebaseReadCapabilitySelfCheck` — Automated Production-Parity Gate — NEW, resolves Pre-Mortem P1 #2

**As** the running `stapler-squad` service, **I want** to verify, once per process lifetime and against my *own* actual `claude` CLI/config (not a human's memory of a CI run), that the WorkDir/tool-flag mechanism this feature depends on actually works, **so that** a production environment where that assumption silently doesn't hold degrades loudly (a WARNING + safe fallback) instead of silently reproducing the pre-feature 3x-rework failure mode under a new log label.

Task 1.1.1b (manual, pre-merge, confirmed against production once by hand) is **kept as-is, not replaced** — it remains a useful first gate before merge. This story adds a **second, automated, ongoing gate**: the same fact re-verified by the running binary itself, on every real deploy, not just once by a human.

**Acceptance Criteria**:
- On the first empty-diff (`diff == ""`) review attempted after process start (either in `ReviewGateRunner.Run` or `TriggerReReview` — whichever fires first), a `sync.Once`-guarded self-check runs the same marker-file-read smoke test shape as Task 1.1.1a/2.1.1e (write a marker file to a throwaway `os.MkdirTemp`, `pool.CallBlocking` with `WorkDir`+`AllowedTools`/`PermissionMode` set, assert the marker content comes back) before the *first* real codebase-read call is attempted.
  - *Given* a freshly-started process (self-check not yet run) and an empty-diff review, *When* `ReviewGateRunner.Run` reaches the codebase-read branch, *Then* `CodebaseReadCapabilitySelfCheck` runs exactly once (verified via a call counter in the test double) before the real review call is made, and does not re-run on a second empty-diff review in the same process.
- If the self-check succeeds, review proceeds normally via the codebase-read path with no observable behavior change, and an `log.InfoLog` line confirms the capability (e.g. `"[BacklogLifecycle] codebase-read capability self-check passed"`).
- If the self-check fails (the marker content is not returned, or the call errors), a `log.WarningLog` line is emitted containing distinguishable text (e.g. `"[BacklogLifecycle] codebase-read capability self-check FAILED"`), and the review that triggered the check — and every subsequent empty-diff review for the remaining lifetime of the process — falls back to the pre-feature behavior: `BuildReviewCallOptions` is *not* used for tool access; the review is recorded directly as `UNVERIFIABLE` with a summary noting the capability check failed, without attempting the codebase-read call at all.
  - *Given* a `FakeRunner` configured so the self-check's marker-read fails, *When* an empty-diff review runs, *Then* the persisted `OverallOutcome` is `ReviewVerdictUnverifiable`, the summary mentions the capability self-check failure, a `log.WarningLog` line containing `"self-check FAILED"` is emitted, and no `AllowedTools`/`PermissionMode`-bearing call is attempted for that review or any subsequent one in the same process.
- The self-check does not block or delay reviews with a non-empty diff — it only runs lazily on the diff-level branch this feature adds, consistent with the rest of this feature being self-gated behind `diff == ""`.
**Files**: `session/review_gate.go` (or a new `session/headless/capability_check.go` if the logic is shared between `ReviewGateRunner` and `TriggerReReview` — prefer the shared-package location to avoid duplicating the `sync.Once` state between the two call sites), `server/services/backlog_service_triage.go`, corresponding `_test.go` files.

##### Task 2.2.6a: Implement `CodebaseReadCapabilitySelfCheck` (~10 min)
- Add a small type (e.g. in `session/headless/`) holding a `sync.Once` and a `sync/atomic.Bool` "capability confirmed" flag, with a method `Ensure(ctx context.Context, pool *Pool) (ok bool, err error)` that runs the once-guarded marker-file smoke test on first call and returns the cached result on every subsequent call (including calls made concurrently with the first, blocked on the `sync.Once` until it resolves). Package-level singleton (mirroring how `Pool` itself is typically constructed once per process) so both `ReviewGateRunner` and `TriggerReReview` share the same check state.
- Files: `session/headless/capability_check.go` (new)

##### Task 2.2.6b: Wire the self-check into `ReviewGateRunner.Run`'s codebase-read branch (~6 min)
- Immediately before the codebase-read `pool.CallBlocking` call (Task 2.2.2a), call `Ensure(ctx, pool)`. On `ok == false`, skip the real codebase-read call entirely, record `ReviewVerdictUnverifiable` with a summary naming the capability-check failure, log the WARNING, and return — do not fall through to the normal codebase-read call path.
- Files: `session/review_gate.go`

##### Task 2.2.6c: Wire the same check into `TriggerReReview` (~5 min)
- Apply the identical guard to `server/services/backlog_service_triage.go`'s codebase-read branch (from Task 2.2.3a), sharing the same package-level `Ensure` state as Task 2.2.6b so a failure discovered via the automatic review gate also short-circuits a subsequent manual re-review in the same process, and vice versa.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.2.6d: Add tests (~8 min)
- `session/headless/capability_check_test.go` (new): `TestCodebaseReadCapabilitySelfCheck_RunsOnceAcrossConcurrentCallers`, `TestCodebaseReadCapabilitySelfCheck_Success_CachesOK`, `TestCodebaseReadCapabilitySelfCheck_Failure_CachesFailureAndDoesNotRetry`.
- `session/review_gate_test.go`: `TestReviewGateRunner_CapabilitySelfCheckFails_RecordsUnverifiableWithoutAttemptingCodebaseReadCall`, `TestReviewGateRunner_CapabilitySelfCheckSucceeds_ProceedsNormallyAndOnlyChecksOnce`.
- `server/services/backlog_service_test.go`: `TestTriggerReReview_CapabilitySelfCheckFails_RecordsUnverifiableWithoutAttemptingCodebaseReadCall`.
- Files: `session/headless/capability_check_test.go`, `session/review_gate_test.go`, `server/services/backlog_service_test.go`

---

### Epic 2.3: Anti-Gaming Regression Guard

**Goal**: Add the adversarial regression test research/pitfalls.md flags as missing today — a fabricated "already implemented" claim against a criterion that is genuinely not implemented in a fixture repo must not be credited as PASS. This is the load-bearing test for requirements.md's anti-gaming success metric. **Expanded in this repair pass** from one fixture (Story 2.3.1, a false claim on an empty diff) to five: a symmetric positive case proving a true claim reaches PASS (Story 2.3.2, resolves Consistency-Check Blocker A — without this, nothing proves the mechanism doesn't just reject everything), two gap fixtures validation.md identified but plan.md hadn't yet referenced (Stories 2.3.3, 2.3.4), and a fixture proving the mixed-criteria evidentiary-weight guard (Story 2.2.5) actually holds against a real `claude` call (Story 2.3.5, resolves Pre-Mortem P1 #3).

#### Story 2.3.1: Adversarial fixture — false "already implemented" claim is caught
**As** the project's anti-gaming guardrail, **I want** an executable test that would fail if the reviewer ever credits an unsubstantiated or false claim, **so that** this regression can't silently reappear.
**Acceptance Criteria**:
- Given a fixture git repo with no code satisfying a specific criterion, and a work session's `verification_notes`/`note` falsely claiming it's already implemented at a specific (non-existent) location, the real headless review call returns an overall outcome of `FAIL` or `UNVERIFIABLE` for that criterion — never `PASS`.
  - *Given* a temp git repo initialized with a single file `main.go` containing only `package main\nfunc main() {}`, an `AcCriterion{Index: 0, Text: "Add input validation to the /login handler rejecting empty passwords", Note: "Already implemented — see validateLogin() in auth/login.go:17"}` (where `auth/login.go` does not exist in the fixture repo), and `diff = ""`, *When* `ReviewGateRunner.Run` is invoked against a real `headless.Pool` with `WorkDir` set to the fixture repo, *Then* the persisted `ReviewVerdictData.OverallOutcome` for that criterion is `FAIL` or `UNVERIFIABLE`, never `PASS`.
**Files**: `session/review_gate_integration_test.go` (new file, `//go:build integration`)

##### Task 2.3.1a: Build the fixture repo helper (~5 min)
- In a new `session/review_gate_integration_test.go` (build-tagged `integration`, mirroring `session/mcp_integration_test.go`'s tag and `session/headless/integration_test.go`'s real-`claude` pattern), add a helper `newFixtureRepoWithoutClaimedCode(t *testing.T) string` that `git init`s a `t.TempDir()`, writes a minimal `main.go`, and commits it — deliberately omitting any `auth/login.go`.
- Files: `session/review_gate_integration_test.go`

##### Task 2.3.1b: Write the adversarial test (~5 min)
- `TestReviewGateRunner_RealClaude_FalseAlreadyImplementedClaim_IsCaughtNotPassed`: construct a real `headless.Pool` via `headless.NewPool`, a `BacklogItemData` pointing `RepoPath` at the fixture repo, an `ItemSessionSummary` with the false `Note`/`VerificationNotes` described above and `diff = ""` (no worktree — exercise the `item.RepoPath` fallback path from Task 2.2.2a), run `ReviewGateRunner.Run` with a `Storage` backed by the test harness already used elsewhere in `session/*_integration_test.go`, and assert the resulting verdict's `OverallOutcome` is `ReviewOutcomeFail` or `ReviewOutcomeUnverifiable`.
- Files: `session/review_gate_integration_test.go`
- Run: `go test -race -tags integration ./session/... -run TestReviewGateRunner_RealClaude` and confirm it passes (this test now runs under `make test-integration`/`make ci` going forward, giving durable regression coverage).

---

#### Story 2.3.2: Adversarial fixture — TRUE "already implemented" claim reaches PASS — NEW, resolves Consistency-Check Blocker A

**As** the project's anti-gaming guardrail, **I want** an executable test proving a genuinely-implemented criterion, with real supporting code in a fixture repo and a true citation, actually reaches PASS via the codebase-read path end-to-end with a real `claude` call, **so that** Epic 2.3 has a positive-case sibling to Story 2.3.1's negative case — today Epic 2.3 only proves a FALSE claim gets caught; nothing proves the mechanism doesn't just reject *everything*, which would be equally broken (a reviewer that never PASSes an empty-diff item reproduces the original 3x-rework failure mode under a different label).

**Acceptance Criteria**:
- Given a fixture git repo where the claimed code genuinely exists (a real `validateLogin()` function actually present at the cited file:line), and a work session's `verification_notes`/`note` truthfully citing that exact location, the real headless review call returns `OverallOutcome == PASS`, with a non-empty `tool_reads` list where every listed path passes `os.Stat` against the fixture repo (per Story 2.2.4/P1 #1's corroboration check — this test also exercises that the corroboration check does *not* falsely reject a true claim).
  - *Given* a temp git repo initialized with `auth/login.go` containing a real `func validateLogin(username, password string) error { if password == "" { return errors.New("password required") }; return nil }`, an `AcCriterion{Index: 0, Text: "Add input validation to the /login handler rejecting empty passwords", Note: "Already implemented — see validateLogin() in auth/login.go:1"}`, and `diff = ""`, *When* `ReviewGateRunner.Run` is invoked against a real `headless.Pool` with `WorkDir` set to the fixture repo, *Then* the persisted `ReviewVerdictData.OverallOutcome` is `ReviewOutcomePass`, `tool_reads` is non-empty, and every path in it exists under the fixture repo (i.e. `path=codebase-read-verified` in the completion log, not `codebase-read-degraded`).
**Files**: `session/review_gate_integration_test.go`

##### Task 2.3.2a: Build the fixture repo helper with genuinely-satisfying code (~5 min)
- Add `newFixtureRepoWithClaimedCodePresent(t *testing.T) string` alongside Task 2.3.1a's helper — `git init`s a `t.TempDir()`, writes `auth/login.go` with a real `validateLogin` function that actually rejects empty passwords, and commits it.
- Files: `session/review_gate_integration_test.go`

##### Task 2.3.2b: Write the positive-case adversarial test (~5 min)
- `TestReviewGateRunner_RealClaude_TrueAlreadyImplementedClaim_IsVerifiedAsPass`: mirrors Task 2.3.1b's structure (real `headless.Pool`, `BacklogItemData.RepoPath` pointing at the fixture repo, `diff = ""`), but with the true `Note`/`VerificationNotes` described above. Assert `OverallOutcome == ReviewOutcomePass`, `len(tool_reads) > 0`, and (via a small helper reused from the `DegradeIfUnverified` unit tests) that every `tool_reads` entry resolves to a real path under the fixture repo.
- Files: `session/review_gate_integration_test.go`
- Run: `go test -race -tags integration ./session/... -run TestReviewGateRunner_RealClaude_TrueAlreadyImplementedClaim` and confirm it passes.

---

#### Story 2.3.3: Adversarial fixture — cites a real but irrelevant file — NEW, folds in validation.md's proposed gap test

**As** the project's anti-gaming guardrail, **I want** an executable test proving the reviewer doesn't credit mere file-existence as satisfying a criterion, **so that** a citation to a real file/function that exists but doesn't actually implement the claimed behavior is still caught — closing the gap validation.md's "Anti-Gaming Coverage Adequacy" section flagged (`TestReviewGateRunner_RealClaude_CitesRealButIrrelevantFile_IsCaughtNotPassed`, proposed there but not previously referenced in plan.md).

**Acceptance Criteria**:
- Given a fixture repo containing `auth/login.go` that exists but is an empty stub (does not actually validate anything), and a Note citing that real path/line as satisfying the criterion, the real reviewer call does not return PASS for that criterion.
  - *Given* `auth/login.go` containing only `package auth\nfunc validateLogin(u, p string) error { return nil }` (accepts everything, including empty passwords) and a Note claiming "Already implemented — see validateLogin() in auth/login.go:2, rejects empty passwords", *When* the real reviewer call runs, *Then* `OverallOutcome` is `ReviewOutcomeFail` or `ReviewOutcomeUnverifiable` for that criterion, never `PASS`.
**Files**: `session/review_gate_integration_test.go`

##### Task 2.3.3a: Add `TestReviewGateRunner_RealClaude_CitesRealButIrrelevantFile_IsCaughtNotPassed` (~6 min)
- Build a fixture repo with the stub `auth/login.go` described above; construct the false-but-real-file-citing `Note`; run `ReviewGateRunner.Run` against a real `headless.Pool`; assert the outcome is never `PASS`.
- Files: `session/review_gate_integration_test.go`

---

#### Story 2.3.4: Adversarial fixture — mixed true and false claims within a codebase-read review — NEW, folds in validation.md's proposed gap test

**As** the project's anti-gaming guardrail, **I want** an executable test proving a review with two criteria — one truthfully claimed, one falsely claimed — downgrades only the false one, **so that** the reviewer cannot launder a false claim under a true sibling's credibility (validation.md's "arguably the most realistic gaming pattern" — a rework-fatigued agent claiming several criteria are done, some truthfully, one not — closing the gap via `TestReviewGateRunner_RealClaude_MixedTrueAndFalseClaims_OnlyFalseCriterionDowngraded`, proposed in validation.md but not previously referenced in plan.md).

**Acceptance Criteria**:
- Given a two-criterion review where criterion 0's Note is true (real code exists) and criterion 1's Note is false (cites nonexistent code), the per-criterion `CriterionVerdict` outcomes diverge correctly.
  - *Given* a fixture repo with real code satisfying criterion 0 and no code satisfying criterion 1, and Notes claiming both are already implemented (one truthfully, one not), *When* the real reviewer call runs, *Then* criterion 0's verdict is `PASS` and criterion 1's verdict is `FAIL` or `UNVERIFIABLE` — never both PASS, and never both downgraded together.
**Files**: `session/review_gate_integration_test.go`

##### Task 2.3.4a: Add `TestReviewGateRunner_RealClaude_MixedTrueAndFalseClaims_OnlyFalseCriterionDowngraded` (~6 min)
- Build a fixture repo satisfying criterion 0 only; construct a two-criterion `acSnapshot` with one true and one false Note; run `ReviewGateRunner.Run`; assert per-criterion divergence as described above.
- Files: `session/review_gate_integration_test.go`

---

#### Story 2.3.5: Adversarial fixture — partial diff + one falsely-claimed already-implemented criterion — NEW, resolves Pre-Mortem P1 #3

**As** the project's anti-gaming guardrail, **I want** an executable test proving Story 2.2.5's evidentiary-weight prompt change actually works end-to-end against a real `claude` call, **so that** the mixed-criteria gap Pre-Mortem Failure #3 identified (a Note claiming "already implemented, unrelated to this diff" on one criterion within an otherwise-normal, non-empty-diff review) doesn't silently regress — this is the highest-priority new adversarial fixture in this repair pass since, per Failure #3's framing, a partial/mixed PR is the *common* case, not the edge case.

**Acceptance Criteria**:
- Given a non-empty diff that genuinely satisfies criterion 0 but does not touch criterion 1 at all, and criterion 1's Note falsely claims "already implemented, unrelated to this diff — see `<nonexistent or irrelevant path>`", the real (non-codebase-access, normal-path) reviewer call does not credit criterion 1 as PASS on the strength of the Note alone.
  - *Given* a fixture repo/diff where criterion 0's change is genuinely present in the diff and criterion 1 has no diff-based support, with `AcCriterion{Index: 1, Note: "Already implemented — unrelated to this diff, see auth/ratelimit.go:9"}` where `auth/ratelimit.go` does not exist in the fixture repo, *When* the real headless reviewer call runs on the normal (non-empty-diff, no tool access) path, *Then* criterion 1's verdict is `FAIL` or `UNVERIFIABLE`, never `PASS` — proving the reviewer did not simply trust the Note's prose despite having no tool access to check it (Story 2.2.5's prompt change is what should produce this outcome; this test is what proves it).
**Files**: `session/review_gate_integration_test.go`, `project_plans/backlog-already-implemented/implementation/validation.md` (new fixture name — see validation.md's Requirement → Test Mapping table, updated separately)

##### Task 2.3.5a: Add the partial-diff-plus-false-Note fixture and test (~7 min)
- Build a fixture repo + non-empty diff genuinely satisfying one criterion; construct a second criterion with a false "already implemented, unrelated" Note; add `TestReviewGateRunner_RealClaude_PartialDiffWithFalselyClaimedUnrelatedCriterion_NotCreditedAsPass` to `session/review_gate_integration_test.go`; assert the false criterion's verdict is never `PASS`.
- Files: `session/review_gate_integration_test.go`
- Run: `go test -race -tags integration ./session/... -run TestReviewGateRunner_RealClaude_PartialDiff` and confirm it passes.

---

### Epic 2.4: Tool-Description Prompting for `verification_notes` on Empty-Diff Claims

**Goal**: Resolve the deferred "required vs. optional" question (see Pattern Decisions table) by strongly prompting the agent to cite specifics when claiming "already implemented," without adding brittle Go-level enforcement.

#### Story 2.4.1: Strengthen the `request_review` tool description for the already-implemented case
**As an** implementation agent that believes a criterion is already satisfied, **I want** the tool description to tell me exactly what makes that claim credible, **so that** I supply citeable evidence instead of a vague assertion.
**Acceptance Criteria**:
- The `request_review` MCP tool's `verification_notes` description includes explicit guidance for the "already implemented, no diff needed" scenario, naming the exact-citation requirement.
  - *Given* the updated tool description string, *When* inspected, *Then* it contains guidance equivalent to: cite the exact file path and function/symbol name for any criterion claimed as already implemented, because the reviewer will independently check that citation.
**Files**: `server/mcp/tools_backlog.go`

##### Task 2.4.1a: Update the `verification_notes` tool description (~3 min)
- In `server/mcp/tools_backlog.go`'s `registerBacklogTools` (around line 711-718), append to the existing `verification_notes` description:
  ```go
  " If you believe a criterion was ALREADY satisfied before this session started (no code change needed), you MUST still cite the exact file path and function/symbol name where it's already implemented — the reviewer will independently check that citation against the current codebase and will FAIL or mark UNVERIFIABLE any criterion it cannot confirm, so a vague \"already done\" claim with no specifics will not pass."
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 2.4.1b: Add a test asserting the description contains the new guidance (~3 min)
- In `server/mcp/tools_backlog_test.go`, add `TestRegisterBacklogTools_RequestReview_DescribesAlreadyImplementedCitationRequirement` (string-containment assertion against the registered tool's description, following whatever pattern existing tool-registration tests in that file use — check the file first for the exact assertion style).
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 2.5: Observability — Log Which Path Graded Each Review

**Goal**: Cheap, nice-to-have per requirements.md — make it possible to tell from logs alone whether a review was graded via the normal diff path, a working empty-diff codebase-read, or a degraded empty-diff codebase-read (Blocker 2's silent-failure signal — extended in this repair pass from a 2-way to a 3-way distinction).

#### Story 2.5.1: Add `path=diff|codebase-read-verified|codebase-read-degraded` to the existing completion log lines
**As an** operator debugging review behavior, **I want** the existing log line to say which path graded a review — and specifically whether a codebase-read review's tool access actually worked — **so that** I don't have to cross-reference diff size or dig through the response body to know, and so a silent tool-access failure (Blocker 2) is visible in logs rather than indistinguishable from a normal successful review.
**Acceptance Criteria**:
- The existing `[BacklogLifecycle] spawnReviewGate headless review complete...` log line includes the review path, using the three-way value produced by `DegradeIfUnverified` (Story 2.2.4) rather than the raw two-way value from `BuildReviewCallOptions` (Story 2.2.1).
  - *Given* `diff != ""`, *When* the completion log line is emitted, *Then* it contains `path=diff`.
  - *Given* `diff == ""` and the codebase-read call returned a non-empty `tool_reads` list, *Then* the same log line contains `path=codebase-read-verified`.
  - *Given* `diff == ""` and the codebase-read call either timed out or returned an empty `tool_reads` list on a PASS/FAIL, *Then* the same log line contains `path=codebase-read-degraded`.
**Files**: `session/review_gate.go`, `server/services/backlog_service_triage.go`

##### Task 2.5.1a: Extend the log line in `review_gate.go` (~2 min)
- Change line ~302's `log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate headless review complete for item %s (session %s, outcome %s)", item.ID, reviewIS.ID, overall)` to include `, path=%s` and pass the `reviewPath` variable — after Task 2.2.4b's `DegradeIfUnverified` call has updated it to its final three-way value (`diff` / `codebase-read-verified` / `codebase-read-degraded`), not the two-way value Task 2.2.2a initially sets.
- Files: `session/review_gate.go`

##### Task 2.5.1b: Extend the equivalent log line in `TriggerReReview` (~2 min)
- Change the `log.InfoLog.Printf("[TriggerReReview] headless re-review complete for item %s (outcome %s)", item.ID, overall)` line (around line 899) to include the same `path=` field, using the `reviewPath` variable set alongside Task 2.2.3a's branch and refined by Task 2.2.4c's `DegradeIfUnverified` call.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.5.1c: Log call duration alongside `path=` — NEW, resolves Pre-Mortem P2 #5 (~4 min)
- Record `callStart := time.Now()` immediately before the headless call in both `ReviewGateRunner.Run` (Task 2.2.2a's call site) and `TriggerReReview` (Task 2.2.3a's call site), and extend both completion log lines (Tasks 2.5.1a/2.5.1b) with a `duration_ms=%d` field computed as `time.Since(callStart).Milliseconds()`. This is the data `CodebaseReadCallTimeout` (150s)'s justification currently lacks — see Story 2.5.2, which consumes these numbers before the feature ships.
- Files: `session/review_gate.go`, `server/services/backlog_service_triage.go`

---

#### Story 2.5.2: Pre-Ship Validation of `CodebaseReadCallTimeout` Against Real Observed Data — NEW, resolves Pre-Mortem P2 #5

**As** the person shipping this feature, **I want** `CodebaseReadCallTimeout` (150s) confirmed against real call durations before considering the feature done, **so that** the value is validated data, not just a reasoned estimate — an unvalidated timeout that's too short would silently reproduce the original 3x-rework failure mode under the `codebase-read-degraded` label instead of fixing it, which is exactly the outcome this feature exists to prevent.

**Acceptance Criteria**:
- Before this feature is marked done (Post-Implementation Checklist), Epic 2.3's adversarial fixtures (Stories 2.3.1–2.3.5, all real-`claude` integration tests) have been run at least once with Task 2.5.1c's `duration_ms=` logging enabled, and their durations recorded.
  - *Given* Epic 2.3's five adversarial fixtures, *When* run via `go test -race -tags integration ./session/... -run TestReviewGateRunner_RealClaude`, *Then* each test's `duration_ms=` value (captured from the test log output, since these tests exercise the same logging path) is recorded — e.g. as a line in this plan's Post-Implementation Checklist or the PR description — and none exceeds `CodebaseReadCallTimeout` (150s) by more than a comfortable margin (if any does, that is itself the signal to raise the constant before shipping, not after).
- A small number (at least 2–3) of real empty-diff backlog items are run through the review gate against representative worktree sizes (not just the fixture repos' minimal single-file trees) before the feature is considered done, and their `duration_ms=` values are similarly recorded.
  - *Given* at least 2 real (non-fixture) empty-diff backlog items in the actual `stapler-squad` repo or another representative-sized worktree, *When* processed through the review gate post-deploy, *Then* their logged `duration_ms=` values are checked against `CodebaseReadCallTimeout`, and `path=codebase-read-degraded` frequency is confirmed to be rare (not the common case) before the feature is treated as fully validated.
**Files**: None (a validation/process task — consumes Task 2.5.1c's logging; no new production code).

##### Task 2.5.2a: Run Epic 2.3's fixtures and record durations (~10 min, pre-ship)
- Run `go test -race -tags integration ./session/... -run TestReviewGateRunner_RealClaude -v` and capture each test's `duration_ms=` log output. Record the results in the Post-Implementation Checklist.
- Files: none (process task)

##### Task 2.5.2b: Run a handful of real empty-diff items and record durations (~15 min, pre-ship, post-deploy)
- After deploying the feature (or in a manually-triggered dry run), find or construct 2–3 real empty-diff backlog items, trigger review, and record `duration_ms=` and `path=` from the logs. If `codebase-read-degraded` fires disproportionately due to timeout (not missing/fabricated `tool_reads`), raise `CodebaseReadCallTimeout` before considering the feature fully shipped rather than treating it as expected behavior.
- Files: none (process task)

---

## Post-Implementation Checklist

- [ ] Task 1.1.1a (`TestPool_RealClaude_WorkDirOnly_GrantsReadAccess`) is green in CI **before** any Epic 2.2 story is started (HARD BLOCKER — repair pass, resolves Adversarial Review Blocker 1).
- [ ] Task 2.1.1e (`TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess`) is green in CI **before** Epic 2.2.2/2.2.3 are marked done (HARD BLOCKER).
- [ ] Task 1.1.1b: both of the above tests have also been re-run and confirmed passing against the `claude` CLI/config actually resolved by the production `stapler-squad` systemd user service (not just the CI runner's `PATH`) **before** Epic 2.2.2/2.2.3 are marked done (HARD BLOCKER — new in this repair pass). Record: CLI version used, pass/fail, any environment delta found.
- [ ] `go build ./...` and `make quick-check` pass.
- [ ] `go test ./session/... ./server/services/... ./server/mcp/...` pass (unit tests, no `integration` tag) — includes Story 2.2.4's degrade-path unit tests (`DegradeIfUnverified`, timeout handling) which do not require the `integration` tag or a real `claude` binary.
- [ ] `go test -race -tags integration ./session/... ./session/headless/...` pass with `claude` in `PATH` (Epic 1.1's smoke test + Epic 2.1.1e's smoke test + Epic 2.3's adversarial fixture).
- [ ] Spot-check logs from a real empty-diff review (or the adversarial fixture's run) show the expected `path=` value — `codebase-read-verified` for a normal pass, and manually trigger a `codebase-read-degraded` case (e.g. by temporarily shortening `CodebaseReadCallTimeout` to 1s in a local build) to confirm the degrade path actually fires and never crashes the review gate.
- [ ] **(NEW — Pre-Mortem P1 #1)** `TestDegradeIfUnverified_ToolReadsPathDoesNotExist_ForcesUnverifiable` and its siblings (Task 2.2.4d) are green — confirms a fabricated, non-empty `tool_reads` list pointing at a nonexistent path is force-downgraded to `UNVERIFIABLE`, not credited.
- [ ] **(NEW — Pre-Mortem P1 #2)** `CodebaseReadCapabilitySelfCheck` (Story 2.2.6) has been observed to fire and log `"self-check passed"` on a real deploy's first empty-diff review — confirms the automated production-parity gate is actually wired up and running, not just unit-tested.
- [ ] **(NEW — Consistency-Check Blocker A)** `TestReviewGateRunner_RealClaude_TrueAlreadyImplementedClaim_IsVerifiedAsPass` (Story 2.3.2) is green — confirms the codebase-read path can reach PASS at all, not just reject everything.
- [ ] **(NEW — Pre-Mortem P1 #3)** `TestReviewGateRunner_RealClaude_PartialDiffWithFalselyClaimedUnrelatedCriterion_NotCreditedAsPass` (Story 2.3.5) is green — confirms Story 2.2.5's evidentiary-weight prompt change actually prevents a mixed-PR false claim from reaching PASS.
- [ ] **(NEW — Pre-Mortem P2 #4)** Story 2.1.2 (the `Pool.CallBlocking` consolidation) has been merged and soaked as its own PR (PR #1) *before* PR #2 (everything else in this plan) is opened — see Dependency Visualization's shipping-sequence note.
- [ ] **(NEW — Pre-Mortem P2 #5)** Story 2.5.2's pre-ship duration validation is complete: Epic 2.3's five adversarial fixtures' `duration_ms=` values are recorded and none is dangerously close to `CodebaseReadCallTimeout` (150s), and at least 2–3 real empty-diff items have been run against representative worktree sizes with recorded durations before the feature is treated as fully shipped.
- [ ] `gofmt -w .` before committing.
- [ ] No `docs/registry/` changes needed — this feature adds no new RPC or UI surface (backend-only prompt/plumbing change).
