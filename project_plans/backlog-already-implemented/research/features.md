# Research: Features — "already implemented" no-op review verification

## 1. Current codebase: does it distinguish "empty diff, nothing needed" from "empty diff, agent stalled"?

**No — confirmed gap, precisely located.** There is exactly one path that reads diff content
(`session/backlog_review.go:106-117`, duplicated in `BuildHeadlessReviewPrompt` at
`:169-180`), and it has a single branch for `diff == ""`: literally write
`"(no diff available)\n"` with **no distinguishing signal** about *why* the diff is empty.

- `session/backlog_lifecycle.go:307` (`spawnReviewGate`) auto-reopens for rework on
  `FAIL || PARTIAL || UNVERIFIABLE` — identical treatment for all three. There is no
  separate outcome value or code path for "verified as already-done, no diff needed."
- `server/services/backlog_service_triage.go:55` (`maxAutoReworkIterations = 3`) then caps
  the blind retry loop described in the problem statement; nothing upstream of this cap
  knows the diff was empty because the criteria were already satisfied vs. because the
  agent produced nothing.
- `session/domain/backlog.go:132-137` (`AggregateOutcome`) explicitly defaults empty verdict
  lists to `FAIL` ("prevent auto-approval of empty reviews") — a deliberate anti-laziness
  guard, but it means the aggregate function has no notion of a legitimate no-op either.

**Important correction to the requirements.md baseline** — one part of the "problem
statement" is already partially fixed on `main`. Commit `70e33b0d` "fix(backlog): surface
work-session verification evidence to the review gate (#152)" added
`writeVerificationEvidenceSection` (`session/backlog_review.go:51-63`) and reviewer-prompt
language distinguishing specific/checkable claims from vague ones
(`session/headless/features.go:79-80` and `:84-87`, the `reviewSystemPrompt` /
`headlessReviewSystemPrompt` constants). So `request_review`'s `verification_notes` DOES
reach the reviewer today, and the reviewer prompt already says: "consult the 'Verification
Evidence' section... specific, checkable claims... may resolve a criterion as PASS... Vague
or generic claims... are not evidence." This existing mechanism is the closest thing this
codebase has to "no-op self-attestation," but it is generic (applies to any hard-to-diff
claim, not scoped to the already-implemented case) and it is text-only, unverified — the
reviewer cannot check the claim against anything except its own text plausibility, because:

- **The headless review path has no tool access at all.** `BuildHeadlessReviewPrompt`'s doc
  comment (`session/backlog_review.go:134-136`) states this explicitly: "headless `claude -p`
  subprocesses do not have tool access." So when `spawnReviewGate` takes the headless branch
  (`session/review_gate.go:244-252`, gated on `r.getPool() != nil` — this is the primary path
  per commit `2d7e116c` "replace idle triage sessions with headless pool calls"), the
  reviewer literally cannot open a file to confirm a claim like "criterion 2 is already
  satisfied by foo.go:42." It can only judge the prose.
- **The legacy tmux review path (fallback when no headless pool is configured) does have real
  tool access.** `SpawnReviewSession` (`server/services/session_service.go:738-745`) calls
  `CreateDirectorySession` rooted at `item.RepoPath` with `PermissionModeAuto` — a full
  Claude Code session that could Read/Grep the current tree. But the prompt built for it
  (`BuildReviewPrompt`, same function, same diff-only framing) never instructs it to go
  inspect the codebase for empty-diff criteria; nothing in `reviewSystemPrompt` says "when
  the diff is empty, go read the current tree yourself." This is the structurally cheapest
  lever available: the capability already exists in one of the two review paths, it's just
  unused for this scenario, and the two paths are inconsistent with each other (a design
  smell worth flagging in the plan — headless-vs-tmux review parity).

**report_progress's per-criterion note — confirmed dropped, precise location.**
`server/mcp/tools_backlog.go:223,243` accepts `note` and persists it via
`storage.UpdateAcCriterionStatus(ctx, itemID, criteriaIndex, acStatus, note)` ->
`session/storage_backlog.go:629-669`, which writes it into `AcCriterion.Note`
(`session/domain/backlog.go:66-71`) and serializes it back onto
`BacklogItem.AcceptanceCriteria`. But the only place acceptance criteria are rendered into a
reviewer prompt — the loop at `session/backlog_review.go:83-86` (and the identical loop in
`BuildHeadlessReviewPrompt` at `:152-155`) — prints only `c.Index` and `c.Text`:

```go
for _, c := range acSnapshot {
    fmt.Fprintf(&sb, "%d. %s\n", c.Index, sanitizeField(c.Text, 500))
}
```

`c.Note` is never referenced. Grepped the whole tree for `.Note` reads outside serialization
— none exist. This exactly matches the requirements.md claim.

**Per-criterion model already supports mixed diff/no-diff verification structurally.**
`AcCriterion{Index, Text, Status, Note}` and `CriterionVerdict{CriterionIndex, Outcome,
Evidence}` are already per-criterion, not a single shared blob verdict. The reviewer is
already asked to return `verdicts: [{criterion_index, outcome, evidence}, ...]` for each
criterion (`session/backlog_review.go:126`), and `applyVerdictsToACs`
(`session/backlog_lifecycle.go:452-507`) already writes per-criterion `AcStatus` (`done` on
PASS, `in_progress` on PARTIAL, unchanged on FAIL/UNVERIFIABLE) back onto the item. So edge
case (a) — some criteria already-done, some needing a diff — is NOT a data-model gap; it's a
prompt/reasoning gap. The one loss: `applyVerdictsToACs` updates `Status` but never writes the
reviewer's `verdict.Evidence` into `AcCriterion.Note`, so the reasoning behind a PASS verdict
doesn't persist for the next review cycle to build on (relevant if this item gets reopened
later for unrelated reasons).

## 2. Industry patterns for "no changes required" vs "nothing was done"

Patterns observed across CI/PR-gate and AI-agent-review tooling (general knowledge, not
web-verified in this pass — flag for a stack/pitfalls research pass if precision matters):

- **Explicit typed outcome, not inferred from diff shape.** Systems that handle this well
  (e.g. Renovate/Dependabot "up to date, no PR needed" runs, Terraform's "No changes.
  Infrastructure is up-to-date." plan output) have a first-class "no-op verified" result
  distinct from both "success with changes" and "failed." This project's `ReviewOutcome` enum
  (PASS/FAIL/PARTIAL/UNVERIFIABLE) has no such value — worth deciding whether to add one (e.g.
  a PASS-with-no_op:true flag alongside the existing outcome, or a reason code) rather than
  overload PASS.
- **Evidence must cite a location, not just assert.** Static analysis / code-review bots
  (CodeQL suppressions, Sonar "won't fix" flows, human PR review "already handled in X") treat
  an unverified claim of "already done" as untrustworthy by default; the credible version
  always cites file+line or a command+output. The existing reviewSystemPrompt already gestures
  at this ("specific, checkable claims... an exact command and its result") but only for the
  free-text verification_notes field, and there is no mechanism to check that a cited
  file:line claim is true — nothing re-reads the file to confirm the citation matches reality.
  A trustworthy version needs either (a) tool access for the reviewer to open the cited
  location, or (b) the implementation agent's own tool calls captured and logged as an audit
  trail the reviewer can inspect without re-running them. This matters even more for AC.Note
  (the report_progress field this project is adding to the reviewer's view) since that field
  is written during implementation, before the agent necessarily has full context — a good
  place to require "why: cite file+line" structurally rather than freeform.
- **Negative-result explanations require the same rigor as positive ones.** "I checked X, Y, Z
  and confirmed already met" is a testable claim only if X/Y/Z are named. A prompt requiring
  the agent to enumerate what was checked (not just the conclusion) gives the reviewer
  something falsifiable — it can spot a criterion that was not actually checked even though
  the overall claim sounds confident.
- **A skeptical-by-default reviewer stance, with an explicit high bar to overturn it.** This
  project's reviewSystemPrompt already has some of this shape ("Vague or generic claims... are
  not evidence — do not let them upgrade a verdict") — the same posture should extend to
  "already implemented" claims: default to not trusting a bare assertion; require either
  tool-verified evidence or a structured, specific self-report before allowing PASS on an
  empty diff.
- **Distinguish "no diff, verified in codebase" from "no diff, git history shows it was done
  earlier."** Two different evidentiary bases (current tree vs. git log) that industry systems
  (e.g. git-blame-driven bots, "closes #123" auto-linking) usually keep separate because the
  second is weaker/staler evidence (a later revert wouldn't show up). This project's reviewer,
  even with tool access, has no explicit instruction to check git log for "was this criterion
  satisfied by an earlier merged commit" vs. "is it satisfied by the current tree" — worth
  calling out explicitly in the prompt design so the reviewer doesn't conflate the two.

## 3. Edge cases

**(a) Partially already-implemented (mixed diff/no-diff criteria).** Data model already
supports this (see #1) — the design work is entirely in the prompt: instruct the reviewer to
evaluate each criterion independently, using the diff for criteria the diff touches and
(codebase inspection / verification evidence / AC.Note) for criteria it doesn't, rather than
gating all criteria on "is there a diff at all." Current prompt structure treats the diff as
one global artifact ("(no diff available)" is a single sentence for the whole prompt, not
per-criterion), which biases toward exactly the bug this project is fixing — a criterion with
a real diff sitting next to a criterion that's a legitimate no-op both get the same "no diff"
framing if the overall diff happens to be empty (e.g. all changes were config/non-git, or the
diff computation itself failed — see `RecoverBaseCommitSHA`, `session/backlog_review.go:306-
332`, a related but distinct empty-diff cause: a corrupted base SHA, not a genuine no-op. The
design should make sure "diff computation failed" and "diff is genuinely empty because nothing
changed" don't get conflated into the same reviewer signal, since one is a system bug needing
self-heal/retry and the other is a legitimate verdict path).

**(b) Agent claims "already implemented" incorrectly (misunderstanding the criterion).** This
is exactly what AggregateOutcome's FAIL-default-on-empty and the existing "vague claims don't
count" prompt language are trying to guard against generally. The new mechanism must preserve
or strengthen this: a false-positive "already implemented" is worse than the current
UNVERIFIABLE-storm because it terminates the rework loop with a PASS instead of just wasting
cycles. This is the sharpest requirement in requirements.md ("a FALSE 'already implemented'
claim must still be caught and rejected") and argues strongly for requiring tool-verified
evidence (reviewer actually reads the cited file) rather than trusting agent self-report
alone, at least for the headless (no-tool) path — or routing empty-diff reviews preferentially
through the tool-having (tmux) path when available.

**(c) "Already implemented" code lives in a dependency/vendored path the reviewer can't easily
read.** Neither review path currently has any vendor/dependency awareness — the tmux path's
tool access is bounded by whatever Read/Grep can reach in item.RepoPath, and vendored code
inside the repo (e.g. generated session/ent/, session/gen/, node_modules) is readable but
noisy/misleading if the agent cites generated code as evidence. go.sum/vendor/ or third-party
module source is not readable at all in the tmux session either unless GOPATH/module cache is
reachable — treat this as an explicit UNVERIFIABLE trigger ("criterion depends on external
dependency behavior the reviewer cannot inspect") rather than silently failing to find
evidence.

**(d) Feature pre-dates the backlog item vs. was implemented in an earlier session/PR that
already merged.** Git history evidence (git log -S, git blame) is a different evidentiary
class from current-tree evidence and is stronger for "was this actually shipped and merged"
claims but weaker for "is it still true today" (a later commit could have reverted it). The
codebase already has git plumbing usable for this (session/git/util.go, GetGitHeadSHA,
GetGitDiffRef in session/backlog_review.go:257-304) but nothing wires it into the review
prompt as an evidence source. Worth deciding in planning whether "already implemented"
verification is scoped to current tree state only (simpler, avoids the staleness trap) or also
allows "verified via git history" as a distinct, separately-labeled evidence class.

## 4. What makes this trustworthy enough that Tyler doesn't have to double-check every verdict?

Given he's a solo operator relying on AutoReopenAfterFailedReview and the rework cap to run
unattended (per MEMORY.md's WIP-limit and stuck-review-investigation context), the bar is: a
false PASS must be rarer than the status quo's false-UNVERIFIABLE-loop, and when the system is
unsure, it should say so distinguishably rather than guess.

Concretely, from what's in the codebase already:
- Reuse the existing "specific, checkable claims only" reviewer stance
  (session/headless/features.go:80,87) but make it load-bearing for the no-op case
  specifically, not just a generic aside.
- Prefer routing empty-diff reviews through a tool-having path (the existing tmux
  SpawnReviewSession capability, or extend the headless pool to grant limited read-only file
  tools for this call) so "already implemented" claims get actually checked against the tree,
  not just judged for plausibility of prose — this is the single highest-leverage change given
  the codebase's existing FAIL-default-on-empty-verdicts guard already shows the team's bias
  toward not trusting silence.
- Keep the FAIL-default posture (AggregateOutcome, session/domain/backlog.go:132-137) as the
  fallback when evidence is insufficient — don't relax it; add a narrow, well-evidenced path to
  PASS instead of loosening the general default.
- Surface the reviewer's per-criterion evidence (not just the final outcome) somewhere Tyler
  can scan without opening the item — the notification/summary path already exists
  (r.getNotifier() calls in session/backlog_lifecycle.go) and could carry a short "why
  already-implemented was accepted" string per PASS-via-no-op verdict, giving him a spot-check
  surface without requiring him to read every diff-less review by default.

## Key file:line references

- session/backlog_review.go:66-132 — BuildReviewPrompt (tool-having/tmux path prompt)
- session/backlog_review.go:137-191 — BuildHeadlessReviewPrompt (no-tool-access path)
- session/backlog_review.go:51-63 — writeVerificationEvidenceSection (existing, #152)
- session/backlog_review.go:83-86, :152-155 — AC criteria render loop, drops .Note
- session/headless/features.go:79-87 — reviewer system prompts, existing evidence rules
- session/domain/backlog.go:66-71 — AcCriterion{Index,Text,Status,Note}
- session/domain/backlog.go:122-164 — CriterionVerdict, AggregateOutcome (FAIL default)
- session/review_gate.go:236-323 — spawnReviewGate, headless-vs-tmux branch, auto-reopen
  trigger at :307
- session/backlog_lifecycle.go:452-507 — applyVerdictsToACs (per-criterion status write)
- server/mcp/tools_backlog.go:187-253 — reportProgress handler, note param
- session/storage_backlog.go:629-669 — UpdateAcCriterionStatus persistence
- server/services/session_service.go:738-745 — SpawnReviewSession (tool-having path)
- server/services/backlog_service_triage.go:52-55,394-482 — maxAutoReworkIterations,
  AutoReopenAfterFailedReview
