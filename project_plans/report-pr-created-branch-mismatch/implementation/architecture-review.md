# Architecture Review: report-pr-created-branch-mismatch
**Date**: 2026-08-04
**Verdict**: CONCERNS

No `docs/adr/ADR-000-architecture-constitution.md` exists in this repository
(confirmed via `find` from repo root) — no constitution check applies.

## Blockers

(none)

## Concerns

- [ ] **Task 3.1 (Lens 1, items 1 & 4 — SRP / testability)** — The override
  policy (existence check → matched fast-path → override_reason presence →
  state gate → persist + audit log) is added as ~20-30 more lines of inline
  `if`/`else if`/`else` directly inside `reportPRCreated`
  (`server/mcp/tools_backlog.go:623-726`), which already does argument
  parsing, role/link auth, idempotency, URL parsing, branch resolution, and
  persistence in one function. Nothing here is wrong for a Transaction
  Script (Step 1's choice is right for this complexity), but the plan's own
  Step 3 rationale for keeping the policy *out of* `VerifyPRMatchesBranch`
  ("what does GitHub say" vs. "should we trust it") argues for a seam that
  the plan then doesn't create — the decision itself is still buried inside
  the giant handler, forcing Tasks 4.1-4.5's five new tests to exercise it
  only through the full handler (mock storage, item-session setup, etc.).
  **Remediation**: extract a small pure function in the same file, e.g.
  `decideOverridePolicy(v PRVerification, overrideReason string) (accept bool, code, msg string)`,
  called once from `reportPRCreated`. It costs nothing extra structurally
  (still Transaction Script, still one file, no new type), but lets most of
  Tasks 4.1-4.5 become table-driven unit tests against the pure function
  instead of five full-handler integration tests — cheaper to write and to
  keep green.

- [ ] **Task 2.1 (Lens 2, item 6 — illegal states in `PRVerification`)** —
  `PRVerification{Exists, Matched, ActualHeadBranch, State}` has four
  independent fields with an implicit invariant (`Matched ⇒ Exists`;
  `Exists ⇒ State != ""`) that nothing in the type enforces. In practice the
  only production producer (`VerifyPRMatchesBranch`, Task 2.1) always
  constructs it correctly, but Tasks 2.3 and 4.1-4.5 both hand-roll roughly
  ten `PRVerification{...}` struct literals directly in test seams (the
  existing `verifyPRMatchesBranch` function-field pattern), which is exactly
  where a copy-paste mistake (e.g. `PRVerification{Matched: true}` with the
  zero-value `Exists: false`) would silently compile and pass. **Remediation**:
  add a small smart constructor — `func NewPRVerification(exists, matched bool, actualHeadBranch, state string) PRVerification`
  — that the real producer and every test literal call through instead of
  building the struct directly; have it assert/log the `Matched ⇒ Exists`
  invariant. This is a few lines, not a sum-type rewrite, and keeps the
  "smallest diff" framing Approach C is built on while removing the one
  place ten independent literals could quietly diverge from the invariant.

- [ ] **Task 3.1 (Lens 2, item 5 — `PRVerification.State` as raw string)** —
  The new fallback-path gate does `verification.State not in {"open", "merged"}`
  as a literal string comparison. There is exactly one existing precedent for
  this pattern in the codebase (`session/backlog_lifecycle.go:2663`,
  `if info.State != "open"` — inside `reconcileOrphanedAgentPRs`, the very
  function Story 5 flags as sharing this bug's blind spot), so this isn't a
  new anti-pattern for the codebase, but it does mean two independent call
  sites now each hardcode the same magic strings with no shared source of
  truth. **Remediation**: add `const` values in the `github` package
  (`PRStateOpen = "open"`, `PRStateClosed = "closed"`, `PRStateMerged = "merged"`)
  that `GetPRByNumber`'s normalization (Task 1.1) and `reportPRCreated`'s gate
  (Task 3.1) both reference — a few lines, no new type, and a natural spot to
  point `backlog_lifecycle.go:2663` at later without this fix needing to
  touch that file itself.

## Nitpicks

- `override_reason` (Task 3.2) is correctly left as a raw `string` rather
  than a newtype — it's free-text audit content only ever length-checked and
  logged, matching the existing `summary` argument's treatment in the same
  handler. No action needed; noted only because Lens 2 item 5 explicitly
  asked the question.
- Lens 3 items 8-9 (Transaction Script fit; inline `if`/`else` vs.
  Strategy/Chain-of-Responsibility for the 3-4-branch fallback gate) are both
  correctly decided as-is — a GoF pattern would add indirection for a fixed,
  non-extensible 3-4-way check. No remediation needed.
- Lens 3 item 11 (consistency with `build-vs-buy.md`): the plan deviates from
  `build-vs-buy.md` Q4's literal suggestion to reuse `GetPRInfoCtx` for the
  number-keyed lookup, in favor of a new pure-REST `GetPRByNumber`. This is a
  reasoned, documented deviation (Task 1.1's alternatives table: avoids the
  untyped-stderr-string-sniffing anti-pattern `GetPRInfoCtx`'s `gh pr view`
  subprocess would force on a 404) — verified against `github/client.go:247-263`,
  where a nonexistent PR number does produce an untyped `fmt.Errorf("failed to get PR info: %s", stderr)`.
  No finding; confirmed consistent with the research's own stated goal (typed
  404 signal), just not its literal first suggestion.
