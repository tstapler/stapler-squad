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

## 2026-07-15 Addendum: Expanded Tool Grant, Relaxed Timeout, Richer Context

This addendum extends (does not replace) the decision above. The core
invariant this ADR exists to protect — **the codebase-read reviewer must
never be granted write access** — is unchanged. What changed is that the
read-only surface got richer, and the timeout budget was relaxed to match.

### Expanded tool grant: scoped Bash allowlist + explicit denylist

`headless.CallOptions` gained a third field, `DisallowedTools string`,
mirroring `AllowedTools`/`PermissionMode` exactly: `headless.ProcessRunner`
gained a `disallowedTools` field, `WithToolAccess` took a third parameter,
and `toolAccessArgs()` appends `--disallowedTools <value>` after the existing
two flags when set. This is the same flag-injection pattern this ADR
originally established for `--allowedTools`/`--permission-mode` — just a
third flag in the same family.

`headless.CodebaseReadAllowedTools` (previously `"Read,Grep,Glob"`) is
extended in place to also grant a **scoped** set of Bash commands, using the
CLI's documented `Bash(<command prefix>*)` scoping syntax (`claude --help`
gives `--allowedTools "Bash(git *) Edit"` as the canonical example of scoping
a Bash grant to a command prefix):

```
Read,Grep,Glob,Bash(git log:*),Bash(git show:*),Bash(git diff:*),Bash(git blame:*),Bash(go test:*),Bash(go vet:*),Bash(go build:*),Bash(sg:*)
```

Rationale per tool: `git log`/`show`/`diff`/`blame` let the reviewer
establish code provenance and history context (e.g. "was this code already
here before the session started, or did the session's claim of
'already implemented' just not produce a diff for some other reason?") that
plain file reads cannot answer. `go test`/`vet`/`build` let the reviewer
cheaply verify a claim ("tests pass", "it builds") instead of trusting a
self-report, closing exactly the class of gap the falsification-framing
system prompt exists to guard against. `sg` (ast-grep, per this repo's
`CLAUDE.md` reference) gives structural search for cases a plain grep is
unreliable for (e.g. "find every caller of this function").

A new constant, `headless.CodebaseReadDisallowedTools`, is paired with it as
belt-and-suspenders — an explicit, process-level denylist enforced by the CLI
itself, not only by `AllowedTools` being a scoped grant that in principle
could be misconfigured:

```
Bash(rm:*),Bash(git push:*),Bash(git commit:*),Bash(git checkout:*),Bash(git reset:*),Bash(curl:*),Bash(wget:*),Bash(chmod:*),Bash(mv:*),Bash(cp:*),Write,Edit,MultiEdit,NotebookEdit
```

`BuildReviewCallOptions`'s empty-diff branch now sets
`DisallowedTools: headless.CodebaseReadDisallowedTools` alongside the
existing `AllowedTools`/`PermissionMode` fields — still the single point of
construction this ADR's Repair Pass Addendum established, so the invariant
has exactly one place to audit. The hard-invariant regression test,
`TestBuildReviewCallOptions_EmptyDiff_NeverIncludesWriteTools`, was
strengthened to match: it no longer does a substring check for the literal
`"Bash"` (that check would now be trivially false, since safe Bash grants
exist by design) — it checks every entry in `AllowedTools` against an exact
allowlist of known-safe scoped prefixes, and checks `DisallowedTools` for
every destructive verb/write tool it must explicitly deny.

### Relaxed timeout: 150s → 600s

`headless.CodebaseReadCallTimeout` moves from 150s to 600s (10 minutes).
150s was calibrated for a bounded Read/Grep/Glob lookup; it was starting to
force premature UNVERIFIABLE degrades on codebase-read reviews that were
making genuine progress once the tool surface and context payload both grew
(git history digging, test/build runs, and the richer context below all
legitimately take longer). 600s remains well short of the shared 900s
`DefaultCallTimeout`, so a genuinely hung codebase-read call still fails into
the degrade path (`DegradeIfUnverified`, unchanged) before hitting the full
15-minute ceiling other headless call types tolerate — the fail-fast posture
this ADR originally established for the timeout is preserved, just recalibrated.

### Richer context: prior verdicts, notes history, item context, searchable transcript

Four new, read-only context sources are available to the codebase-read
reviewer, gated behind `diff == ""` exactly like the tool grant above (the
"expensive extras only on the hard-to-verify path" posture is unchanged):

- **Prior Review Attempts** — every past review-role `ItemSession` for the
  item (via `Storage.ListItemSessions`), with outcome, summary, and non-PASS
  per-criterion evidence.
- **Full Notes History** — the complete append-only `report_progress` history
  for the item (via `Storage.ListProgressNotesForItem`), superseding the
  single latest-note-per-criterion view already shown in the acceptance
  criteria list.
- **Item Context** — the item's Description (goal) and a compact
  status-transition history (`BacklogItemData.StatusEvents`, already eagerly
  loaded by every `GetBacklogItem` call — no new query needed at either call
  site, since both `ReviewGateRunner.Run` and `TriggerReReview` already hold
  an `item` loaded that way).
- **Session Transcript** — a searchable file (not embedded prompt text) of
  the work session's own terminal activity, written via the already-landed
  `WriteReviewTranscriptFile` (Wave 1) to a dot-prefixed file inside
  `codebaseWorkDir`, referenced by relative path with an instruction to
  `Grep` it rather than read it in full. Both call sites now hold (or
  received wiring for) a `*scrollback.ScrollbackManager` reference —
  `ReviewGateRunner` via a new `SetScrollbackManager` setter (delegated
  through `BacklogLifecycleListener.SetScrollbackManager`, wired in
  `server/dependencies.go` once `ScrollbackManager` is constructed at Step
  9 — the listener itself is constructed earlier, at Step 5), and
  `BacklogService` via an equivalent `SetScrollbackManager` setter, wired at
  its own construction site once `ScrollbackManager` already exists. The
  returned `cleanup func()` is always deferred immediately, so the transcript
  file never lingers in the real repo checkout regardless of whether the
  review succeeds, fails, or times out.

All four sources are read-only queries/writes to files the review process
itself created and cleans up — none of them grant the reviewer any new
capability, and none of them bear on the write-access invariant. They are
bundled into one struct parameter, `session.ReviewContextExtras`, added to
`BuildHeadlessReviewPrompt`'s signature — a struct rather than four more
positional parameters for the same reason `headless.CallOptions` itself is a
struct: Go has no named/optional parameters, and this keeps both call sites
and any future extension of the context payload clean. Every field is
zero-value-safe; a caller that doesn't have a given piece of context (e.g. no
scrollback manager wired) simply gets that section omitted rather than
needing a distinct code path.

Every fetch feeding `ReviewContextExtras` is best-effort/log-and-continue at
both call sites — a failure to list prior sessions, notes, or write a
transcript file never blocks a review that would otherwise succeed, matching
this ADR's existing posture that infrastructure hiccups on the enrichment
path are not evidence about the acceptance criteria.

### What did not change

- The reviewer still never receives `Write`, `Edit`, `MultiEdit`, or
  `NotebookEdit` — now enforced by both `AllowedTools` scoping (an
  allowlist) and `DisallowedTools` (an explicit denylist), not by
  `AllowedTools` scoping alone.
- `tool_reads` verification and `DegradeIfUnverified` are unchanged — a
  PASS/FAIL verdict with fabricated or unverifiable `tool_reads` still
  degrades to `UNVERIFIABLE` exactly as before, regardless of which tool
  produced the (claimed) evidence.
- The falsification-framing instruction ("treat the work session's claim as
  a hypothesis to check, not a fact to accept") in
  `headlessReviewSystemPromptWithCodebaseAccess` is preserved verbatim; the
  prompt update only adds guidance on the new tools/context sources and
  reframes the closing instruction toward a more thorough analysis, without
  weakening or removing the anti-gaming guardrails this feature depends on.

## 2026-07-15 Addendum #2: Bash Grant Reverted — Empirically Disproven

**This addendum records the FINAL decision on the tool grant. Addendum #1
above (the scoped Bash allowlist + `DisallowedTools` denylist) is superseded
by this one. Do not read Addendum #1 as current behavior — it is kept only
as a record of what was tried and why it did not work.**

### What was tested

Addendum #1's reasoning for the Bash grant treated `AllowedTools` (an
allowlist of scoped `Bash(<prefix>*)` entries) plus `DisallowedTools` (an
explicit denylist of destructive prefixes) as a real, process-level
technical restriction — "enforced by the CLI itself, not only by
`AllowedTools` scoping alone." This assumption was never empirically
verified for Bash specifically (unlike the Read/Grep/Glob grant, which
*was* verified by `TestPool_RealClaude_WorkDirOnly_GrantsReadAccess` and
`TestPool_RealClaude_WorkDirWithToolFlags_GrantsReadAccess`).

A new integration test,
`TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`
(`session/headless/integration_test.go`), was added specifically to check
this assumption against the real `claude` CLI under
`--permission-mode bypassPermissions` — the exact permission mode this
feature uses. It ran two sub-tests against the real binary:

1. **UnlistedCommand** — asked the reviewer to run `whoami`, a command
   present in neither the allowlist nor the denylist by name.
2. **ChainedAfterAllowed** — asked the reviewer to run
   `git log --help; whoami > <canary file>`, chaining an unlisted command
   after an explicitly-allowed `git log` prefix, to check whether the CLI's
   matching is naive-prefix-based (vulnerable to chaining) or genuinely
   parses/restricts the full command.

### Result

**Both sub-tests found no real enforcement.** The explicitly unlisted
`whoami` command executed freely and wrote a real file to disk; the
chained command after an allowed prefix also executed in full. Under
`bypassPermissions`, `--allowedTools`/`--disallowedTools` behave as
pre-approval hints at best for Bash — they are not a hard technical filter.
This directly contradicts the "enforced by the CLI itself" claim
Addendum #1's decision rested on.

### Decision

**Drop Bash access entirely.** Revert to the original,
empirically-PROVEN-safe grant: `AllowedTools: "Read,Grep,Glob"`, no
`DisallowedTools`. This is not a partial rollback — every `Bash(...)` entry
is removed from `headless.CodebaseReadAllowedTools`, which reverts to
exactly `"Read,Grep,Glob"`. `headless.CodebaseReadDisallowedTools` is
removed as a production constant (it existed only to back the now-reverted
Bash grant); the `DisallowedTools` field on `headless.CallOptions` /
`headless.ProcessRunner` remains as general-purpose plumbing for a future
call site that might genuinely need it, but `BuildReviewCallOptions` no
longer populates it.

The structural difference that makes `Read,Grep,Glob` safe where a scoped
Bash grant is not: Read/Grep/Glob have no arbitrary-execution surface —
their worst case is reading a file inside `WorkDir`. Bash's worst case is
running any command the underlying shell can run, and this test proved the
CLI's scoping syntax does not actually constrain that at the process level
under `bypassPermissions`. `TestPool_RealClaude_UnlistedBashCommand_BlockedOrAllowed`
is retained (not deleted) specifically so this finding stays discoverable in
the test suite and the reasoning above doesn't have to be rediscovered the
hard way by a future change that re-attempts a Bash grant. The exact
allowlist/denylist strings it exercised were moved into the test file
itself (`testCodebaseReadAllowedToolsWithBash` /
`testCodebaseReadDisallowedTools`) so the test keeps exercising the
disproven shape even though production no longer references those values.

### What changed as a result

- `headless.CodebaseReadAllowedTools`: `"Read,Grep,Glob,Bash(...)..."` →
  `"Read,Grep,Glob"`.
- `headless.CodebaseReadDisallowedTools`: removed.
- `BuildReviewCallOptions`'s empty-diff branch: no longer sets
  `DisallowedTools`.
- `headlessReviewSystemPromptWithCodebaseAccess`: the Bash-specific
  instructions (running `go test`/`go vet`/`go build`, `git log`/`show`/
  `blame` via Bash, `sg` structural search) are removed. The falsification
  framing, the `tool_reads` citation requirement, and the instructions to
  use the richer context sections (prior review attempts, full notes
  history, item context, searchable session transcript via Grep) are
  preserved — none of those depend on Bash.
- `TestBuildReviewCallOptions_EmptyDiff_NeverIncludesWriteTools`: reverted
  from an allowlist/denylist-membership check back to an exact-match
  assertion (`AllowedTools == "Read,Grep,Glob"`, `DisallowedTools` empty).
- `headless.CodebaseReadCallTimeout` stays at 600s (unchanged from
  Addendum #1) — the relaxed budget was motivated by the richer context
  payload (prior verdicts, notes history, item context, searchable
  transcript), not by Bash tool use, so it remains valid independent of
  this revert.

### What did not change (again)

- The reviewer still never receives `Write`, `Edit`, `MultiEdit`, or
  `NotebookEdit` — now enforced the same way the ORIGINAL grant enforced it
  (Addendum #1's belt-and-suspenders `DisallowedTools` reasoning is moot
  once there's no Bash to restrict): by `AllowedTools` being a narrow,
  empirically-verified allowlist of three read-only tools with no execution
  surface.
- `tool_reads` verification, `DegradeIfUnverified`, and the richer context
  sources (prior review attempts, full notes history, item context,
  searchable session transcript) are all unchanged and remain fully in
  effect — none of them depended on the Bash grant.
