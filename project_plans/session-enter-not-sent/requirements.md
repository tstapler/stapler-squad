# Requirements: session-enter-not-sent

**Date**: 2026-09-28
**Type**: bugfix (verification/closure — root cause already fixed in main)
**Complexity**: 1 — verification + targeted gap-closing, not new implementation

## Problem Statement (as filed)

Backlog item `f7201b49-8318-43fb-a22f-0ce1926f9980`: `run_command`, `write_to_session`,
and `steer_session` were observed (by direct terminal inspection, not just tool-output
inference) to write text into a session's PTY input without ever actually submitting
it — the trailing Enter/newline never reached the receiving program, so text sat queued
and unprocessed. The tools reported `{"success": true, ...}` regardless, making the
failure silent. The report ties this to backlog item #819 (empty output from
`run_command`/`read_session_output`), hypothesizing the same root cause: if Enter never
lands, the shell/program never runs anything, so of course there's nothing to read back.

## Investigation Finding (2026-09-28, this triage pass)

**This exact defect was already root-caused and fixed on `main`, and the fix is present
in this worktree's HEAD** (`1dced16a9`, 2026-09-28), landed a week earlier in PR #832
("fix(mcp): route steer_session/write_to_session/run_command through two-write submit",
merged 2026-09-21, `e2085dec4`). This is the *second* time this exact item has been
triaged: a prior triage pass on 2026-09-23 (preserved on the abandoned branch
`backlog/stapler-squad-item-f7201b49`, commit `12d2849de`, which never merged — likely
superseded by this re-triage) reached the identical conclusion independently. This pass
re-verifies that finding against current `main` rather than assuming it still holds.

Evidence:

- `docs/bugs/fixed/BUG-047-write-to-session-uses-newline-instead-of-carriage-return.md`
  (fixed 2026-07-24) first fixed the `\n`-vs-`\r` half of this bug for the same three
  tools, but PR #832's own commit message identifies that BUG-047's fix reintroduced a
  second, subtler defect (BUG-031's shape): `content + "\r"` built as one *string* and
  sent as a single `SendKeys` write still lands inside Claude Code's Ink-TUI
  paste-detection window for long-enough content, so the trailing Enter gets folded into
  the pasted block instead of registering as submit — bytes reach the PTY, tool reports
  `success: true`, nothing gets processed. This is precisely the symptom the backlog
  item describes.
- PR #832 routes all affected call sites — `server/mcp/tools_terminal.go`'s
  `writeToSession` (`server/mcp/tools_terminal.go:277-326`), `steerSession`
  (`server/mcp/tools_terminal.go:664-722`), `runCommand`
  (`server/mcp/tools_terminal.go:543-651`) — through `session.SubmitContentWithEnter`
  (`session/pane_submit.go:117-137`) → `session.SubmitDriverContent`
  (`session/pane_submit.go:77-109`), which sends content and the Enter keystroke as two
  *separate* `SendKeys` writes with a settle-wait (`waitForPaneSettle`) in between,
  closing the paste-detector window.
- It also implements the backlog item's own "Suggested fix direction" bullet — *"verify
  submission actually happened... before returning success"* — via
  `waitForPaneUpdate` (`session/pane_submit.go:168-181`): after sending Enter, it polls
  for an observed pane change and retries the Enter once before giving up and returning
  `ErrSubmitNotConfirmed` (`session/pane_submit.go:34`), surfaced to MCP callers as
  error code `SUBMIT_NOT_CONFIRMED` via `submitErrResult`
  (`server/mcp/tools_terminal.go:335-346`) — instead of a false `success: true`.
- Confirmed directly against this worktree's current code (not just reading the PR
  diff): `writeToSession`, `runCommand`, and `steerSession` all call
  `session.SubmitContentWithEnter`/`SendKeysWithTimeout` today, not a hand-rolled
  `+"\r"`/`+"\n"` concatenation. `git log e2085dec4..HEAD` for the affected files shows
  only one touching commit since PR #832 (`ccbc1dba6`, #837, 2026-09-22), a mechanical
  extraction of a shared `sessionNotReadyResult` helper with no behavioral change to the
  submit path.
- **The related item, #819's empty-output symptom, has also since shipped its own fix**:
  PR #837 (`ccbc1dba6`, merged 2026-09-22 — one day after PR #832, one day before the
  prior triage pass) titled "new_worktree sessions branch from dirty local HEAD instead
  of origin/<default>, and terminal output capture returns empty for trivial commands"
  touches `server/mcp/tools_terminal.go`, `server/services/session_service.go`,
  `session/git/ops.go`, and `session/instance_worktree.go`. Both halves of this backlog
  item's "related" cross-reference are therefore now addressed on `main`, independently
  of each other and independently of this item ever being explicitly closed.
- A structural AST regression guard (`session/sendkeysguard`) is wired into `session/`,
  `server/mcp/`, `server/services/`, and `session/tymux` to fail the build if any call
  site reintroduces the single-write `content + EnterKeySequence` pattern — specifically
  to stop this defect class from resurfacing a fourth time (it had already recurred at
  three independent call sites before PR #832).
- Test coverage for the fix is substantial: `session/pane_submit_test.go` (two-write
  shape, swallowed-submit retry/failure, context-cancellation edge cases, wedged-write
  timeout) plus MCP-layer tests in `server/mcp/tools_terminal_test.go`
  (`TestMCPPackage_NoDirectSendKeysPlusEnterConcatenation`, `TestSubmitErrResult_*`,
  `TestSteerSessionMCP_*`) and a dedicated `sendkeysguard_test.go`.

**One confirmed, deliberately out-of-scope gap remains**: `session/tymux`'s
`SendPromptWithEnter` (`session/tymux/session.go:542-548`) does **not** route through
`session.SubmitDriverContent` — a separate, unconfirmed implementation, because
`HasUpdated()` on that backend is permanently stubbed to always report no change
(`session/tymux/session.go:719-721`), so a swallowed submit there is undetectable and
never retried. This is mitigated by `docs/bugs/open/BUG-113-tymux-backend-never-passes-
target-program-launches-default-shell.md` (confirmed still **open** as of this pass):
tymux sessions currently launch `$SHELL` instead of the target program, so the specific
Ink-TUI paste-detector race this item is about is likely unreachable on that backend
today regardless. tymux is also not the default backend (opt-in via
`STAPLER_SQUAD_USE_TYMUX`).

**Conclusion**: the acceptance criteria below are written as a *verification and
closure* pass, not a reimplementation. The only real open work is (a) confirming the fix
holds via a live/manual smoke test (all evidence above is static-code-reading plus
existing unit tests, not a fresh live-session reproduction), and (b) closing this item
with a citation to PR #832 (and #837 for the related #819 symptom) rather than
dispatching a duplicate implementation session that would find nothing to change (see
`project_plans/backlog-already-implemented/` for why that outcome is itself a known,
costly failure mode if not headed off explicitly here).

## Baseline

- Before PR #832 (through 2026-09-20): confirmed broken — `content+"\r"` sent as one
  `SendKeys` write, silently swallowed by TUI paste detection on long/multi-line input,
  `success: true` returned regardless.
- After PR #832 (2026-09-21 onward) and PR #837 (2026-09-22 onward), current `main` /
  this worktree (`1dced16a9`, 2026-09-28): two-write submit + swallowed-submit
  detection/retry + `ErrSubmitNotConfirmed` error surfacing for the primary tmux
  backend, plus the shared `sessionNotReadyResult` refactor for #819's empty-output
  guard. Not yet established: whether a live smoke test against a real Claude Code TUI
  session confirms the fix in practice (all verification above is static analysis +
  existing automated tests).
- tymux backend (non-default): unconfirmed / gapped, gated behind BUG-113 (open).

## Users / Consumers

- MCP clients (this triage agent itself, and any other automation) calling
  `write_to_session` / `steer_session` / `run_command` against a live stapler-squad
  session.
- The web UI's session chat box (`WriteToSession` RPC), same underlying fix.
- Tyler (solo operator), who filed this from direct observation and needs confidence the
  fix actually holds, not just that a PR merged claiming to fix it.

## Success Metrics

- Zero silent-submit-swallow incidents attributable to the single-write pattern after
  this closure — enforced structurally by `sendkeysguard`'s AST guard, not just by this
  item's closure.
- A live smoke test (send a real multi-line/long-enough input via each of the three
  tools against a manually-run instance, per this repo's manual-instance testing
  convention) confirms Enter is actually registered end-to-end, not just that unit tests
  pass.
- This item is closed with a citation to PR #832 and PR #837, and the duplicate/stale
  planning artifacts on `backlog/stapler-squad-item-f7201b49` are not left as the only
  record of this conclusion.

## Appetite

Small (well under a day) — this is a verification pass over an already-merged fix, plus
closing paperwork, not new engineering.

## Constraints

None hard. No deadline, no compliance surface, solo-maintainer project.

## Scope

### In Scope
- Confirming PR #832's (and PR #837's) fixes are present and intact on current `main` /
  this worktree (done above).
- A live smoke-test task to exercise `write_to_session`, `steer_session`, and
  `run_command` end-to-end against a real running session and observe actual submission.
- Closing this backlog item with a citation to PR #832 and PR #837, rather than routing
  it through a full fresh implementation session.

### Out of Scope
- Any new code changes to the submit path — none are indicated by this investigation.
- `session/tymux`'s confirm-and-retry parity gap — explicitly deferred, gated on
  BUG-113 (open) landing first; that backend can't reach the paste-detector race today
  regardless since it launches `$SHELL` instead of the target program.
- The distinct `INPUT_REQUIRED`/numbered-selection prompt delivery failure mode PR #832's
  own commit message calls out as NOT covered by this fix (already filed separately as
  backlog item `e08ad709-6ade-48cf-81f9-f577b53c1b10`) — do not conflate with this item.
- Re-litigating `sendkeysguard`'s design or `ErrSubmitNotConfirmed`'s retry-can-
  double-submit tradeoff — both were already through adversarial review in PR #832.

## Rabbit Holes

- Treating "PR merged claiming to fix X" as equivalent to "verified fixed" without at
  least one live smoke test — static code reading confirms the *shape* of the fix is
  present, not that it behaves correctly against a real Ink-TUI target today.
- Re-implementing any part of the two-write submit mechanism from scratch, which would
  waste a full session finding nothing to change (the exact failure mode
  `project_plans/backlog-already-implemented/` exists to prevent).
- Backporting tymux confirm/retry parity as part of this item — it's a distinct,
  already-gapped concern blocked on a different open bug (BUG-113).

## Alternatives Considered

- **Dispatch straight to a normal implementation session** — rejected: the evidence
  strongly indicates there is no remaining code change to make; doing so risks an
  agent either inventing busywork or (per the "already implemented" failure mode)
  cycling through rework because the reviewer can't verify a near-empty diff.
- **Close silently as duplicate with no verification** — rejected: the item was filed
  from direct observation, and static analysis alone (no live PTY test) isn't full proof
  the fix holds in practice; a cheap smoke test is worth the small cost before closing.

## Feasibility Risks

- Low. The main risk is the live smoke test surfacing that the fix has a gap static
  analysis missed (e.g., a `sendkeysguard`-blind-spot shape, or the tymux backend
  behaving differently once BUG-113 lands). If so, this becomes a small, well-scoped
  follow-up bug rather than a large one, since the fix's architecture (single shared
  `SubmitDriverContent` choke point) is already in place.

## Open Questions

- Was this backlog item's most recent observation made before or after PR #832 landed
  (2026-09-21)? Backlog-item creation/comment timestamps are not repo-resident data;
  this triage pass cannot resolve it from static research and defers to the live
  backlog store or the smoke test's own result. *(unresolved after Phase 2 research —
  confirmed by all three research agents: this is backlog-store metadata outside repo
  research's reach.)*
- Does the live smoke test confirm the fix holds for genuinely long/multi-line input
  (the shape that specifically triggers the Ink-TUI paste-detector window per PR #832's
  root cause), not just short single-line input that may never have been affected?
  *(unresolved after Phase 2 research by design — this can only be answered by actually
  running the smoke test, which research/build-vs-buy.md confirms nothing in the
  existing test suite already does for long/multi-line input against a real Ink-TUI
  target: `session/pane_submit_test.go` is mock-only, and `tests/e2e/backlog-session-
  steer.spec.ts` — the closest real-tmux coverage — only exercises short single-line
  messages against a plain `bash` prompt. research/pitfalls.md adds a concrete trap for
  that smoke test: `maxInputBytes = 4096` will silently reject an oversized test
  payload before it ever reaches the submit path, so the "long" input used must stay
  under that cap while still being long enough to plausibly hit the paste-detector
  window.)*
- **New from Phase 2 research (pitfalls agent)**: how should this item actually be
  closed, mechanically? `report_duplicate` (`server/mcp/tools_backlog.go:2158`) is the
  real tool, and its behavior forks: a linked-work-session caller routes through the
  normal review gate (where BUG-032's still-open "Not Fixed" factors could risk
  mishandling this as needing rework); an *unclaimed* caller archives directly but
  explicitly refuses to act while any ItemSession — including this triage session
  itself — is still open on the item. `duplicate_ref` also takes exactly one full
  GitHub URL, not a "PR #832" shorthand, so citing both PR #832 and PR #837 requires
  putting the second in `reason`. This is a plan.md-level task detail, not something
  this triage pass resolves itself (this pass produces planning artifacts only, per its
  own task framing — it does not call `report_duplicate`).
