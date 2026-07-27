# ADR-001: Startup Dialog Answer Latch (content-hash, bounded-retry)

Backlog item: `04089969-0f19-499c-be34-2e8bcfc4f13e`

## Status

Accepted

## Context

`session/session_driver.go`'s `runSessionDriverWithPrompt` polls `inst.Preview()`
every `driverPollInterval` (2s) and calls `inst.SendKeys("1\n")` whenever
`isStartupDialog(output)` matches (lines ~148-165), with no de-duplication,
backoff, or "already answered" memory. `Preview()` (`instance_terminal.go:105-125`)
reads from `ClaudeController.GetRecentOutput` (`claude_controller.go:628-646`),
an in-memory PTY buffer that can keep returning the exact same stale dialog
text for many poll ticks while the underlying tmux/control-mode connection is
mid-flap ("session not started or paused"). Phase 0
(`research/phase0-findings.md`) proved with a live-executing test
(`session/phase0_repro_test.go`) that this produces 3 repeated
`SendKeys("1\n")` calls over ~6.5s against a stalled buffer — the confirmed
mechanism behind the ticket's repeated phantom "1" keystroke.

A fix must guarantee "answer at most once" (or a small bounded number of
attempts before giving up) without breaking the legitimate case: a dialog
that genuinely has not yet been answered must still get answered once the
first time it's seen, and a *different* dialog that appears later (e.g. a
directory-access prompt after the trust dialog) must not be silently
swallowed by a latch left over from the first dialog.

## Decision

Use a **content-hash latch with bounded retry-on-failure** local to each
`runSessionDriverWithPrompt` invocation. No new `Instance` field or
synchronization is needed: `StartSessionDriver`'s `driverRunning.CompareAndSwap`
already guarantees a single sequential goroutine owns this state for the
life of one driver run, so the latch is a plain local variable (mirroring
`sentInitial`/`initialPromptSentAt`, already local vars in this exact
function).

State machine (`dialogLatchStatus`): `Unanswered → AwaitingDismissal → GaveUp`,
keyed by `dialogContentHash` (FNV-64a, reusing the existing `hashString`
helper at `claude_controller.go:621-625`, of a **normalized** form of the
dialog text — see "Content normalization before hashing" below):

- On each tick where `isStartupDialog(output)` is true, compute `hash :=
  hashString(normalize(output))`. If `hash` differs from the latch's stored
  hash, this is either a brand-new dialog or the same dialog having been
  dismissed and a new one having appeared — reset the latch to `Unanswered`
  for this hash.
- `Unanswered`: attempt `SendKeys("1\n")`. On success, transition to
  `AwaitingDismissal` (do not resend while the hash is unchanged — this is
  what bounds the ticket's stuck-buffer case to exactly one send). On
  failure, increment an attempt counter and retry next tick, up to
  `maxDialogAnswerAttempts` (3); after that, transition to `GaveUp` and log a
  warning so the stuck state is at least visible in server logs.
- `AwaitingDismissal` / `GaveUp`: no-op until the hash changes.

`answerDialogOnce` returns the resulting `dialogLatchStatus` so the call site
can decide whether to keep short-circuiting the rest of the driver's poll
tick (see "Control flow: latch status must not starve the rest of the tick"
below) — this return value is new relative to the original one-line design
sketch and is required to close adversarial-review.md's Blocker 1.

The same latch shape is applied to the second `SendKeys("1\n")` call site
(the `NeedsApproval` / `shouldApprovePrompt` branch, lines ~206-219), since it
is the identical defect class (unconditional resend on unchanged content) —
using a second, independently-keyed latch instance, not shared state with the
startup-dialog latch. Unlike the startup-dialog branch, this call site has no
`continue` today and needs no control-flow change (see below).

### Control flow: latch status must not starve the rest of the tick

The pre-fix startup-dialog branch is `if isStartupDialog(output) {
SendKeys(...); continue }` — the `continue` unconditionally skips the rest
of the tick (Ready-detection, the `driverInactivityTimeout` →
`handleDriverFailure` → `ReviewQueue` escalation, and the `NeedsApproval`
branch) whenever `isStartupDialog(output)` is true, regardless of whether a
send actually happened. Preserving that shape verbatim around
`answerDialogOnce` would mean a session whose latch reaches `dialogGaveUp`
(after 3 failed attempts, ~6s) permanently stops entering the rest of the
loop body for the remainder of the driver's life, since `isStartupDialog`
keeps matching the still-unchanged stale buffer forever. The only remaining
exit would be the bare `if time.Now().After(totalDeadline) { return }` at
~25 minutes, which calls no failure handler — trading "phantom keystrokes
forever" for "the whole driver silently wedges for up to 25 minutes with one
stale Warn log line, then exits with zero operator notification," which is
worse for genuinely (not just cosmetically) stuck sessions.

The fix: the startup-dialog call site branches on `answerDialogOnce`'s
returned status. Only `dialogUnanswered` (this tick just attempted a send —
success or a failure still under the retry cap) keeps the existing
single-tick `continue` semantics. `dialogAwaitingDismissal` and
`dialogGaveUp` both let the tick fall through to the rest of the loop body
exactly as if `isStartupDialog(output)` had been false — restoring
Ready-detection, the inactivity-timeout escalation, and the `NeedsApproval`
check for ticks after the dialog has been answered or abandoned. This means
a `dialogGaveUp` session now reaches the pre-existing
`handleDriverFailure`/`ReviewQueue` machinery the same way any other stuck
session does, which is why plan.md's Unresolved Question #1 ("should
`GaveUp` escalate to `ReviewQueue`?") resolves for free rather than needing
new wiring.

### Content normalization before hashing

A straight FNV-64a hash of the raw `Preview()` string is fragile to
incidental, non-semantic differences between ticks: terminal-width-driven
line-wrap shifts, trailing whitespace, and cursor-positioning artifacts can
all change the raw byte string tick-to-tick without the dialog itself having
changed. This is not a hypothetical concern for this fix specifically:
requirements.md and this plan already cross-reference the adjacent "infinite
resize loop" issue as sharing a reconnect-instability root with this ticket,
meaning resize-driven churn during the exact flapping window this fix
targets is a real, adjacent phenomenon in this codebase. An unnormalized
hash would reset the latch on such jitter and reproduce the original
unbounded-resend bug through a different path than the one Phase 0 proved,
silently defeating the fix.

Decision: normalize `output` before hashing via
`strings.Join(strings.Fields(output), " ")` (collapses all whitespace runs,
including newlines introduced by re-wrapping, to single spaces) — cheap,
allocates once per tick, no new dependency. This mirrors an existing
precedent one call away in the same package: `GetCurrentStatus`
(`claude_controller.go:500-553`) already hashes a tailed, filtered slice of
terminal content rather than the raw buffer for the identical reason
(avoiding false-different comparisons against incidental buffer noise),
though via tail-slicing and tmux-metadata filtering rather than whitespace
normalization — the "normalize before hash" idiom is the reusable part, not
the specific filter. `filterTmuxMetadata` itself doesn't apply here (it
strips tmux status-bar lines, which are not the noise source for this
dialog text), so a dedicated whitespace-collapse is used instead.

## Alternatives Considered

1. **Bounded attempt counter with exponential backoff, no content hash.** Cap
   total `SendKeys` attempts per driver run (e.g. 3) regardless of dialog
   content, with growing delay between attempts. *Rejected*: it cannot tell
   "the same stale dialog is still on screen" apart from "a genuinely new,
   different dialog just appeared" (e.g. trust-folder dialog followed later
   by a real directory-access prompt) — once the counter is exhausted, a
   legitimately new second dialog would be silently ignored, a functional
   regression. It also adds timing/backoff-scheduling complexity that doesn't
   address the actual signal (unchanging content) directly.
2. **Answer-and-verify-then-latch via re-preview after a delay.** After
   sending, schedule an explicit re-check some seconds later to confirm the
   dialog cleared before fully latching; retry once more if not cleared
   before giving up. *Rejected*: adds a second timing/scheduling axis on top
   of the existing 2s poll tick for no material benefit over comparing
   content hashes on the poll tick that already happens — more state-machine
   surface for the same outcome, and it delays the "already answered, stop
   resending" decision by design, which is the opposite of what's needed to
   bound the resend count quickly.

## Consequences

- A dialog whose content is byte-identical across a real "SendKeys silently
  failing forever" scenario will latch to `GaveUp` after 3 attempts rather
  than retrying indefinitely — this trades "never gives up" for "bounded,
  logged, and no longer phantom-replaying keystrokes," consistent with
  `research/ux.md`'s finding that a visible/loud failure is preferred over a
  silent-but-repeating one for this codebase's trust model. Unlike the
  original design sketch, `GaveUp` is **not** a dead end: per "Control flow:
  latch status must not starve the rest of the tick" above, once the latch
  reaches `GaveUp` the tick falls through to the rest of the loop body, so
  the session reaches the existing `driverInactivityTimeout` →
  `handleDriverFailure` → `ReviewQueue` escalation the same way any other
  stuck session does — no bespoke `GaveUp`-specific wiring was added; the
  pre-existing failure-handling path now simply gets a chance to run instead
  of being permanently skipped.
- FNV-64a hash collisions between two genuinely different dialog texts are
  theoretically possible but not a practical risk at this text volume/domain;
  not mitigated further.
- The zero value of `dialogAnswerState` (`hash: 0`, `status: dialogUnanswered`
  as `iota` 0, `attempts: 0`) doubles as the latch's "no dialog seen yet"
  sentinel. This is safe in practice — FNV-64a essentially never produces
  exactly `0` for any real input, including the empty string (which hashes to
  the FNV offset basis, not `0`) — but it is an implicit invariant rather
  than an explicit one; not worth a dedicated wrapper type for an
  astronomically unlikely collision, but noted here so it doesn't need
  re-deriving later.
- **`maxDialogAnswerAttempts = 3` (~6 seconds of polling) is an accepted,
  tunable constant, not a proven-correct value.** Phase 0's evidence only
  ran 3 poll ticks (~6.5s) against a stalled buffer; nothing in this
  project's research bounds how long a real "session not started or paused"
  flap can last in production. If real-world observation later shows flaps
  commonly outlasting ~6s, a genuine (not just visually-stale) `SendKeys`
  failure could hit `dialogGaveUp` before the underlying condition clears,
  converting "eventually succeeds, if noisily" into "gives up, then falls
  through to the inactivity-timeout/`handleDriverFailure` path" (per the
  control-flow fix above — no longer a silent wedge, but still not a
  successful auto-recovery). This constant should be revisited if that
  pattern is observed; it is not being changed now for lack of production
  duration data.
- No new `Instance` fields, no new locking primitives, no new dependencies.
