# Adversarial Review: ci-status-diff-viewer

**Date**: 2026-08-02
**Verdict**: CONCERNS

## Blockers

None.

## Verification of the fix pass

1. **Task 2.2.4b's storage-lookup/logging self-contradiction — resolved.** The rewritten task
   (plan.md:444-466) now gates only the final `CodeFailedPrecondition` *return* on
   `!req.Msg.OverrideCiBlock`; Task 2.2.2a's lookup and its nil-storage/lookup-error fail-open
   handling stay unconditional with respect to `OverrideCiBlock` (they still depend only on
   `req.Msg.Decision == "allow"`, the feature flag, and `as.storage` being non-nil — exactly as
   before). Concretely: `blocked := data.GitHubPRNumber > 0 && data.GitHubCheckConclusion ==
   ciConclusionFailure` is computed once lookup succeeds; `blocked && !req.Msg.OverrideCiBlock`
   gates the early return, and `blocked && req.Msg.OverrideCiBlock` gates the distinct log line
   at `data.GitHubCheckConclusion` — both conditions read the same, already-populated `data`,
   so there is no path where the log line's precondition can reference undefined `data`. In the
   nil-storage/lookup-error fail-open paths, `blocked` is never computed (those paths never
   reach the block/log logic at all, per Task 2.2.2a's original structure), so no override log
   line can fire there either — consistent with the "no-op flag" Given/When/Then. The
   contradiction is closed.
2. **Dangling `InstanceData.HasGitHubPR()` Domain Glossary row — resolved.** The row now
   documents the real, *existing* `Instance.HasGitHubPR()` (`session/instance.go:739-741`,
   operates on `*Instance` via `Snapshot()`) and states plainly it is "Not called by this plan's
   new code," explaining that Task 1.1.2a (plan.md:220) and Task 2.2.2a (plan.md:385) both work
   from `*InstanceData` (no equivalent method) and therefore intentionally use the inline
   literal `data.GitHubPRNumber > 0`. Grepped the full plan for `HasGitHubPR`/`HasAssociatedPR`:
   the glossary row is the only occurrence: no other task references a
   `InstanceData.HasGitHubPR()` method that doesn't exist. Task 1.1.2a's and Task 2.2.2a's own
   text is consistent with this — both spell out the inline comparison directly. Resolved.

## Concerns

- [ ] **Carried forward, unresolved: unconditional storage lookup on the hot path.** Task
  1.1.2a still adds `h.storage.FindInstanceDataByID(sessionID)` unconditionally inside
  `HandlePermissionRequest` — the live hook handler invoked for every Bash/Edit/Write
  permission request in every session — even when no loaded rule has `RequireCIPassing: true`.
  No short-circuit behind a cheap "does any loaded rule require CI" flag was added in this fix
  pass.
- [ ] **Carried forward, unresolved: AC6's "auto-approves a session" wording still isn't
  literally true, and the substitution still isn't explicitly flagged as a deviation.**
  ADR-001 documents that AC5 and AC6 are separate gates and that AC6 gates
  `matchesRule`/`classifySingle` (a single tool call), which does make the actual mechanism
  traceable. But neither ADR-001 nor `requirements.md` contains the one explicit sentence the
  prior review asked for ("AC6's literal 'auto-approve a session' language doesn't map onto
  anything in the live system; this plan satisfies the evident intent, not the literal
  wording"). A future reader still has to reverse-engineer that from ADR-001's mechanism
  description rather than reading it as a stated decision.
- [ ] **Carried forward, unresolved: AC2's `${prUrl}/checks` link limitation still only lives
  in the Pattern Decisions table, not in Unresolved Questions.** The plan's Unresolved
  Questions section still lists only 3 items (5th badge state, synchronous re-fetch,
  SHA-mismatch) and omits "the checks-tab link doesn't deep-link to the specific failing run
  when a PR has multiple concurrent workflow runs," despite the prior review asking for it to
  be recorded there for consistency with the other two scope cuts.
- [ ] **Carried forward, unresolved: Task 1.1.2b leaves the poll-interval source as an open
  "Implementer TODO" rather than a decision.** The task still explicitly defers whether
  `ApprovalHandler` threads in the live `PRStatusPollerConfig.PollInterval` at construction vs.
  references a package-level default constant, and says "either is acceptable." Since this
  value directly determines the staleness threshold for an irreversible auto-approve gate,
  leaving the source ambiguous risks two different implementers picking different answers, or
  silent drift from the real poller interval if it's ever tuned. Worth resolving to a single
  choice before implementation starts.
- [ ] **New: Task 2.2.4d's test doesn't actually assert the distinct log line, despite its own
  name.** `TestResolveApproval_OverrideCiBlock_SkipsGuard_AndLogsDistinctly` (plan.md:481-483)
  only asserts `ResolveApproval` succeeds (no `CodeFailedPrecondition`) — it never asserts that
  the distinct override log line fires or that it carries `data.GitHubCheckConclusion`/
  `override_ci_block=true`, despite "AndLogsDistinctly" being half the test's name and Story
  2.2.4's AC explicitly requiring that log content. This was the second half of the original
  Blocker's recommendation ("Add an assertion on the logged fields... so this can't regress
  silently") and wasn't picked up in this fix pass — the design contradiction is resolved, but
  nothing in the specified test suite would catch a future regression where the log line is
  silently dropped or missing its `ci_conclusion` field. — **Recommendation**: add an assertion
  (e.g. a test logger/hook capturing the `log.Info` call, or an equivalent observable) to Task
  2.2.4d that checks the override log line actually fires with the expected fields.

**Resolved since the last review** (no longer applicable, confirmed by reading the current
plan): Blocker 1 ("Task 2.2.4b's storage-lookup/logging self-contradiction") — see Verification
above. "Dangling `InstanceData.HasGitHubPR()` reference in the Domain Glossary" — see
Verification above. "Pitfalls.md §2's SHA-mismatch/force-push mitigation is silently dropped" —
recorded as Unresolved Questions item 3. "No explicit regression-safety task for the
`ClassificationContext`/`matchesRule` signature change" — now Task 1.1.1e, requiring `go test
./pkg/classifier/...` as a verification gate.

## Minors

- `config.LoadConfig()` is still called synchronously on every `ResolveApproval` to read one
  boolean flag (Task 2.2.2a) — unchanged from the prior review, fine at current approval
  volume.
- Observability Plan still adds no metric/counter for how often the AC5 block fires or how
  often `ci_passing` prevents an auto-approve — unchanged, still explicitly justified via
  YAGNI rather than silently dropped.
- Task 2.2.4b's illustrated one-liner (`if blocked := data.GitHubPRNumber > 0 && ...;
  blocked && !req.Msg.OverrideCiBlock { return ... }`) declares `blocked` in the `if` statement's
  init clause, which in literal Go scopes it to that `if` only — it would not be visible to the
  separately-described "log a distinct line when `blocked && req.Msg.OverrideCiBlock`" step if
  transcribed verbatim as two statements. The design intent is unambiguous (compute `blocked`
  once, branch twice), and any competent implementer will hoist `blocked := ...` to its own line
  before both checks; flagged only because the task's illustrative snippet, if copy-pasted
  as-is, wouldn't compile.
