# ADR-001: Grant the headless reviewer bounded codebase access via WorkDir, with defensive AllowedTools/PermissionMode flags

**Status**: Accepted
**Date**: 2026-07-14
**Project**: backlog-already-implemented

## Context

When a backlog item's review gate sees an empty git diff, the reviewer LLM call
today has no way to check the implementation agent's "already implemented"
claim against the actual codebase — it can only judge the plausibility of
prose (`session/backlog_review.go:107-108,170-171`, `session/headless/features.go:79-87`).
Two viable mechanisms surfaced in Phase 2 research for giving the reviewer
real codebase access on this path:

- **(a) WorkDir-only.** `headless.CallOptions{WorkDir: ...}` already exists
  and is already used in production by backlog triage
  (`server/services/backlog_service_triage.go:671-676`), which writes files
  into that directory with **no** `--allowedTools`/`--permission-mode` flags
  set anywhere in the call. That demonstrably works today — proving the
  `claude -p` subprocess gets *some* form of working tool access purely from
  `WorkDir` being set, contradicting the `backlog_review.go:135-137` doc
  comment that headless calls "do not have tool access." (research/architecture.md,
  research/build-vs-buy.md)
- **(b) Deterministic Go-side inspection.** Do the codebase check in Go code
  (targeted grep/read of specific files, injected as a labeled prompt
  section) instead of giving the reviewer any agentic tool loop at all —
  consistent with this codebase's existing preference for deterministic
  Go-side plumbing (`RecoverBaseCommitSHA`, the `GetGitDiffRef` fallback
  chain) and sidesteps the entire question of whether an unapproved tool
  call in a non-interactive `claude -p` process hangs or silently
  auto-denies. (research/pitfalls.md)
- **(c) WorkDir plus explicit `AllowedTools`/`PermissionMode` flags** —
  same mechanism as (a), but also passing `--allowedTools "Read,Grep,Glob"`
  and `--permission-mode bypassPermissions` (mirroring the pattern already
  proven for interactive `Instance`s in `session/instance_tmux.go:117-124`)
  so the read-only tool grant is explicit and scoped rather than resting on
  an unstated default.

research/pitfalls.md's objection to (a) — that an unapproved tool call in a
headless, non-interactive context might hang or silently degrade — is
conditioned entirely on the `backlog_review.go:135-137` "no tool access"
comment being an accurate description of current behavior. research/architecture.md's
triage finding is direct evidence that comment is stale for at least the
write case. The two research threads read the same code and reached opposite
conclusions because neither had an empirical answer for whether that finding
generalizes to reads across a full worktree (versus writes into a scratch
artifacts directory).

## Decision

Use **(c): WorkDir-only as the load-bearing mechanism, with `AllowedTools`/
`PermissionMode` added as a defensive, low-cost companion**, gated by an
empirical smoke test added *before* any prompt/wiring work depends on the
assumption (Task 1.1.1a in `plan.md`, plus Task 2.1.1e — the former Task
1.1.1b, resequenced into Phase 2 where its `CallOptions` dependency actually
lives — see the 2026-07-14 Repair Pass Addendum below).

**This smoke test is a hard blocking prerequisite for Epic 2.2, not a
de-risking measure the rest of the plan can proceed without.** An adversarial
review of the initial plan correctly identified that `session/review_gate.go:247`
carries the comment `"Use JSON-output prompts because headless claude -p has
no tool access"` — written for the exact code path this ADR's decision
modifies, and directly contradicted by the WorkDir-only assumption above. The
plan's Dependency Visualization and task sequencing now reflect this as a
hard gate, not a footnote.

Concretely:
- Extend `headless.CallOptions` (`session/headless/caller.go`) with
  `AllowedTools string` and `PermissionMode string`, mirroring
  `session.InstanceOptions`.
- Extend `headless.ProcessRunner` with the same two fields plus a
  `WithToolAccess(allowedTools, permissionMode string) *ProcessRunner`
  constructor (mirroring the existing `WithWorkDir`), and append
  `--allowedTools <value>` / `--permission-mode <value>` to the subprocess
  args when set — this is the exact flag pair `instance_tmux.go:117-124`
  already uses for the tmux-launched interactive path, just added to the
  second, headless-subprocess call site.
- The review gate's empty-diff call sets `WorkDir` to the item's worktree
  (or repo path fallback) **and** `AllowedTools: "Read,Grep,Glob"` /
  `PermissionMode: PermissionModeBypassPermissions`.
- Add `TestPool_RealClaude_WorkDirOnly_GrantsReadAccess` to
  `session/headless/integration_test.go` (build-tag `integration`, already
  run in CI via `make test-integration`) that writes a marker file into a
  temp dir and asserts a `WorkDir`-only call (no `AllowedTools`) can read it
  — empirically confirming or refuting the triage-precedent assumption
  *before* it becomes load-bearing.

If the smoke test confirms WorkDir-only access (matching the triage
precedent), the `AllowedTools`/`PermissionMode` flags are redundant
belt-and-suspenders that make the read-only intent explicit in the
subprocess invocation and protect against future CLI default changes. If the
smoke test instead shows WorkDir-only is insufficient for reads (unlike
writes), the flags become load-bearing and the feature still works with zero
further design changes — the plumbing for both is added in the same task.
Either outcome unblocks Epic 2.2 without a second design cycle.

## Alternatives Considered

| Alternative | Reason Rejected |
|---|---|
| (a) WorkDir-only, no defensive flags | Simplest and matches the triage precedent exactly, but the entire empty-diff verification feature would then rest on an unstated, undocumented `claude -p` default-tool-grant behavior with no fallback if that behavior differs for reads vs. writes, differs across worktrees vs. scratch dirs, or changes in a future CLI version. The marginal cost of adding explicit flags is a handful of lines (already-proven pattern); the marginal risk of not adding them is a silent, hard-to-diagnose regression in exactly the path this feature exists to make trustworthy. |
| (b) Deterministic Go-side codebase inspection (grep/read specific files, inject as a labeled prompt section) | Consistent with the codebase's existing deterministic-plumbing preference (`RecoverBaseCommitSHA`, diff fallback chains) and completely sidesteps tool-access uncertainty. Rejected because it requires solving a harder, more speculative problem first: deterministically deciding *which* files/symbols are relevant to an arbitrary free-text acceptance criterion is not a targeted grep — it needs the same reasoning an LLM does naturally by reading the tree, and hand-rolling that heuristic (or an LLM call to *generate* grep targets, then a second call to judge the results) is more code, more indirection, and more failure surface than granting bounded read access directly, for a "focused feature" appetite (complexity 2). |

## Consequences

- Two new fields on `headless.CallOptions`, one new `ProcessRunner` method,
  ~10 lines of flag-injection logic (Story 2.1.1). No new dependency, no new
  subsystem.
- The empty-diff review call now takes a real `claude -p` codebase-read
  round trip (turn-bounded and timeout-bounded by the CLI's own tool loop
  and the existing `DefaultCallTimeout`/`MaxCallTimeout` ceiling) instead of
  a single-shot JSON call — slower and more expensive per empty-diff review,
  which is the accepted, explicitly-flagged tradeoff in requirements.md
  against 3x full rework sessions.
- A CI-run integration test (`TestPool_RealClaude_WorkDirOnly_GrantsReadAccess`)
  now encodes this assumption as an executable fact rather than a comment,
  so a future CLI upgrade that silently changes default tool-grant behavior
  fails a test instead of silently degrading every empty-diff review back to
  UNVERIFIABLE.
- `AllowedTools`/`PermissionMode` are scoped to `Read,Grep,Glob` and
  `bypassPermissions` specifically for the review call — never `Bash` or any
  write-capable tool, keeping the reviewer's "must not modify files"
  invariant enforceable at the process level in addition to the existing
  prompt-level instruction.

## 2026-07-14 Repair Pass Addendum

An adversarial review of the initial plan raised two BLOCKER findings and two
CONCERNS directly bearing on this ADR. Resolved as follows (full detail in
`plan.md`'s Dependency Visualization, Pattern Decisions table, Epic 1.1, and
Epic 2.2):

- **Hard-blocking sequencing (was: "de-risks but does not hard-block").**
  Task 1.1.1a (WorkDir-only smoke test) now hard-blocks the *start* of Epic
  2.2 (the prompt/wiring work this ADR's decision enables), and the former
  Task 1.1.1b (WorkDir+flags smoke test) — resequenced to Task 2.1.1e,
  immediately after Story 2.1.1's `CallOptions` fields land, resolving a
  self-contradictory dependency the architecture review also flagged — hard-
  blocks Epic 2.2.2/2.2.3 being marked *done*. A new Task 1.1.1b requires both
  smoke tests to also be re-run and confirmed passing against the `claude`
  CLI/config actually resolved by the production systemd service (see
  `.claude/rules/systemd-user-service.md`), not just whatever's on a CI
  runner's `PATH` — closing the gap between "CI says this works" and "this
  actually works where it will run."
- **Degradation path for silent WorkDir-access failure (previously
  unspecified).** The empty-diff review call now uses a dedicated, shorter
  `headless.CodebaseReadCallTimeout` (150s, vs. the shared 900s
  `DefaultCallTimeout`) so a hung/degraded call fails fast. The reviewer's
  JSON response schema gains a `tool_reads []string` field listing files it
  actually opened; a PASS/FAIL verdict from the codebase-read path with an
  empty `tool_reads` list — or a timeout against `CodebaseReadCallTimeout` —
  is force-downgraded to `UNVERIFIABLE` (never silently trusted, never
  mis-labeled `FAIL`) via a new `DegradeIfUnverified` helper (Story 2.2.4).
  This preserves the feature's pre-existing UNVERIFIABLE baseline as the safe
  fallback for exactly the failure mode this ADR's WorkDir-access mechanism
  could otherwise fail into silently. Logging gains a third `path=` value
  (`codebase-read-degraded`, alongside `diff` and `codebase-read-verified`)
  so this is visible in production logs, not silent.
- **Shared decision helper (Adversarial Review Concern A).** The `CallOptions`
  construction this ADR specifies (`WorkDir` + `AllowedTools: "Read,Grep,Glob"`
  + `PermissionMode: bypassPermissions`) is now written in exactly one place —
  `BuildReviewCallOptions(diff, codebaseWorkDir string)` in
  `session/backlog_review.go` — called by both `ReviewGateRunner.Run` and
  `TriggerReReview`, instead of each call site independently constructing the
  same literals. This directly protects the invariant in this ADR's last
  Consequences bullet ("never `Bash` or any write-capable tool") by giving it
  one point of failure to audit instead of two.
- **`Pool` blocking-method consolidation (Architecture Review Concern B) —
  affects this ADR's implementation surface, not its decision.** The plan
  originally proposed a 4th blocking method, `CallBlockingWithCostAndOptions`,
  alongside this ADR's `CallOptions` extension. That method is no longer
  added; instead, `Pool.CallBlocking`, `CallBlockingWithCost`, and
  `CallBlockingWithOptions` are consolidated into one
  `CallBlocking(ctx, key, systemPrompt, userPrompt string, opts CallOptions) (string, float64, error)`
  (Story 2.1.2, rewritten in the repair pass). This is a Pool-API-surface
  decision independent of the WorkDir-access mechanism this ADR decides, but
  is recorded here because the empty-diff review call site (this ADR's
  primary consumer) is what originally motivated the 4th-method proposal —
  the consolidated method is what `BuildReviewCallOptions`'s output is passed
  to.
