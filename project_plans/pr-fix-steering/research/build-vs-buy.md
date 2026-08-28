# Build vs. Buy — Steering an Active Session for PR-Fix Reconciliation

**Research question**: Should "steer an already-active agent session with fix-context content
from the reconciliation loop, plus reason-signature dedup/cooldown" be built from scratch or
sourced from an existing solution?

## Codebase findings (context for all options below)

- The steering primitive itself already exists and is already wired end-to-end:
  `UpdateSession`'s `SteerMessage` handling
  (`server/services/session_service.go:2925-2971`) branches on
  `instance.AutonomousMode` — autonomous sessions go through
  `instance.GetController().SendCommandImmediate` (ADR-001 command-queue path), everything
  else goes through `instance.SendKeys` (bounded by a 5s timeout via a `select`/`errCh`
  pattern) using `session.BuildSubmittableInput`. This is not new code to write; it's an
  existing, tested internal API this feature becomes a *new caller* of.
- That primitive was built and hardened over several PRs, not written once and left alone:
  `df3151e3e` (steer_session via `--resume`), `9f86b21c2` (widen to non-autonomous via
  SendKeys), `512365cd8` (steer live sessions from item detail UI), `ffb2f518d` (bound
  SendKeys with timeout), `ae596cced` (correct error codes, cap input length),
  `2c654e05e`/`1893f1db9` (test coverage, per-session draft keys). Existing test coverage
  lives in `TestUpdateSession_SteerMessage_*` (`server/services/session_service_test.go:1500-1650`),
  covering the SendKeys-success, SendKeys-failure→`FailedPrecondition`,
  autonomous-still-uses-controller, and max-length-rejection cases — i.e. the "does
  send-into-a-busy-session even work reliably" question this project's Feasibility Risks
  flags is already answered empirically by tests, not something to newly validate.
- The dedup/cooldown adjacent problem (`session/nudge_dedup.go`) was itself built and
  iterated live in production: `dab799362`/`f406850e4` (suppress duplicate autonomous
  nudges within a cooldown), then `dbc8e7f9d`... `bdfd9479b` (re-steer on idle-settle
  cadence instead of a fixed timer, after the fixed-cooldown version was found to
  re-steer even when the agent had already stated it was waiting). This history is
  directly relevant to point 3 below: it shows the *pane-snapshot re-arm* logic exists
  specifically because a naive time-based cooldown was already tried and found wanting for
  that (different) problem.
- `SessionStopper` (`server/services/backlog_service.go:45-61`) is the exact DI precedent
  requirements.md names for the new `SessionSteerer` interface: consumer-defined interface
  in `server/services`, injected via a `SetXxx` call, nil-safe so `BacklogService` degrades
  gracefully when not wired.
- No webhook/external API is involved anywhere in this path — `AutoReopenForPRFix`
  (`server/services/backlog_service_triage.go:2014-2051`) and `findActiveWorkSession`
  (`backlog_service_triage.go:1253`) are pure in-process Go over local state
  (`ItemSessionSummary`, `Instance`).
- `go.mod` has no tmux-wrapper dependency (`gotmux`, `gomux`, `go-tmux`, etc.) and no
  generic debounce/rate-limit library — the repo's tmux integration is its own
  control-mode client (see `session/tmux`, referenced from `CLAUDE.md`'s
  `STAPLER_SQUAD_USE_CONTROL_MODE` note), not a third-party wrapper.

## 1. Existing OSS library or framework

Searched for: Go tmux control-mode libraries, and generic "inject text into a live
session with dedup/debounce" libraries.

| Option | What it does | Fit |
|---|---|---|
| `gotmux`, `gomux`, `go-tmux` (Go tmux wrapper libraries) | Type-safe wrappers over tmux session/window/pane CRUD and `send-keys` | None of these add dedup, cooldown, or reason-signature comparison — they're at best a thinner call to the same `send-keys` the repo's own `session` package already issues directly through its own control-mode client. Adopting one now would mean running two tmux integrations side by side (the existing hand-rolled control-mode client plus a new wrapper lib) for zero functional gain, and would contradict the project's own established pattern of a single internal tmux abstraction. |
| Generic Go debounce/rate-limit packages (e.g. `rodaine/executor`'s debounce, `storj.io/storj/private/server/debounce`) | Time-window debounce of repeated calls | Solve a *different* shape of problem (collapsing a burst of calls in a short window) than what's needed here (compare a *reason-signature tuple* against the last delivered one, independent of call frequency, and re-fire only when the tuple changes). Bending a debounce library to do tuple-equality-plus-cooldown would need as much glue code as just writing the comparison directly, per Interface Pollution rule 5 (unjustified generic). |
| Agent-framework "steer" primitives found in the wild (e.g. OpenClaw's `/steer` drain-into-active-run with 500ms debounce) | Steering a live agent run in a *hosted, single-process* agent framework | Not applicable — a different execution model (in-process agent run) from this repo's out-of-process PTY/tmux-backed sessions. Confirms steering-with-dedup is a recurring *pattern* across agent tooling, but every implementation is coupled tightly to its own runtime; nothing here is packaged as an importable library. |

**Verdict**: Not recommended. Confirmed rather than assumed — no general-purpose "inject
text into a live PTY/terminal session with dedup" library exists, and the closest
candidates (tmux wrapper libs) don't touch the dedup problem at all while duplicating
functionality the repo already owns. This is a thin, repo-specific composition of two
already-existing internal primitives (`SteerMessage`'s autonomous/interactive branch, and
a reason-signature comparison in the shape of `nudge_dedup.go`), not a problem an OSS
library solves.

## 2. SaaS / managed API

Not applicable. This is internal automation over local state: a reconciliation loop
running in-process against locally-tracked `Instance`/`ItemSessionSummary` data, writing
into a local tmux pane or an in-process `ClaudeController` via a Unix-domain/local IPC
mechanism. There is no external network call in the entire path (`GetPRStatus` already
uses `gh` CLI for GitHub data, which is unrelated to *this* feature's job of delivering the
already-computed `fixContext` into a session). No SaaS product operates on "a locally
running PTY session on this machine" — it's not something a hosted API could reach.

**Verdict**: Not applicable.

## 3. LLM-generated implementation vs. battle-tested library — the dedup/cooldown logic

The concrete choice is among three shapes:

- **(a) Directly reuse `nudge_dedup.go`'s functions as-is.** `isDuplicateNudge` compares
  normalized *exact nudge text* plus a pane-snapshot re-arm; it has no concept of a
  structured reason (`CIFailing`/`HasBlockingReviews`/`HasConflicts` tuple) at all — the
  new caller would have to serialize the reason tuple into fixed text and hope
  `nudge_dedup.go`'s whitespace/case normalization treats two different reasons as
  different (it might, by accident, if the rendered text differs) and two identical
  reasons as duplicates (it will, since the tuple would render to the same string). This
  works only by accident of string comparison, and the pane-snapshot re-arm is actively
  wrong for this use case: `isDuplicateNudge` re-arms (bypasses cooldown) whenever the
  pane has *any* new output since the last nudge — but a working agent session produces
  constant new pane output as a matter of course, so re-arm-on-new-output would make the
  cooldown nearly useless here (every tick, the pane has moved, so it would look "re-armed"
  and re-steer even though the failure-reason tuple is unchanged). That behavior is correct
  for its *actual* problem (an idle autonomous driver deciding whether to nudge a
  quiet session) and wrong for this one (comparing a structured signal against the same
  session's noisy, always-changing PTY output).
- **(b) Generalize `nudge_dedup.go` so both callers share one implementation.** Would
  require adding a generic "candidate" type parameter or an interface for
  "comparable-with-cooldown" content, plus making the pane-snapshot re-arm opt-in/opt-out
  per caller. That's exactly interface-pollution-checklist smell #5 (unjustified generic —
  a generic used at a second call site whose comparison semantics *and* re-arm semantics
  are both different is not "the same logic reused," it's two different policies wearing
  one interface) and smell #4 in spirit (a forwarding shim that has to special-case behavior
  per caller isn't adding shared behavior, it's threading a flag through). The two
  problems only look similar at the vocabulary level ("don't repeat yourself to a PTY too
  often") — their actual comparison keys (freeform text vs. a typed reason tuple) and
  their re-arm triggers (pane-output-changed vs. reason-tuple-changed) are different by
  design, not by oversight.
- **(c) Write a small, independent implementation.** A pure function comparable in shape
  to `nudge_dedup.go` — e.g. `reasonSignature{ciFailing, hasBlockingReviews, hasConflicts bool; detail string}`
  plus `lastSteer{signature reasonSignature; at time.Time}` and an
  `isDuplicateSteerReason(candidate reasonSignature, last lastSteer, now time.Time, cooldown time.Duration) bool`
  that returns true only when the signature tuple is unchanged and within cooldown — no
  pane-snapshot check at all, because "has the agent's own output already shown it noticed"
  is explicitly called out in requirements.md's Rabbit Holes as a *separate*, not-yet-decided
  question, not a settled requirement to import from `nudge_dedup.go`. This is ~20-30 lines,
  directly unit-testable with the same table-driven, pure-function style
  `nudge_dedup_test.go` already establishes as this repo's convention for this kind of
  logic.

**Recommendation: (c), write a small independent implementation** — grounded directly in
`.claude/rules/interface-pollution-checklist.md`:

- The two problems are "different enough that forcing a shared abstraction would be
  premature/wrong" per the checklist's framing: different comparison key (freeform string
  vs. typed tuple), different re-arm trigger (pane output vs. reason change), and
  different callers with different failure semantics (an idle-nudge miss is a UX
  annoyance; this feature's dedup gate is the sole safety valve preventing "a bug in the
  new path" from becoming spam, per requirements.md's Risk Control section — it deserves
  its own, obviously-correct-by-inspection implementation rather than inheriting an
  unrelated pane-snapshot side effect from a shared generic).
- Forcing (b) would produce exactly smell #5 (unjustified generic) and, by needing
  per-caller behavior flags to paper over the differing re-arm semantics, drift toward
  smell #4 (forwarding wrapper that doesn't actually share behavior). Two real
  implementations with two real call sites is the generalize trigger this checklist and
  `.claude/rules/primitive-obsession-checklist.md`'s sibling document both use ("generalize
  only once 2+ real call sites need the *identical* logic") — these two do not need
  identical logic, only similar-shaped logic.
- Reusing (a) as-is is the worst option: it would work by coincidence today and silently
  misbehave (near-constant re-arm) the moment PTY output volume differs from the
  idle-nudge case it was tuned for.
- This is squarely "simple enough that a small, well-tested Go function is clearly fine,"
  matching the sibling `backlog-pr-conflict-detection` project's build-vs-buy verdict on
  its own small pure-function piece (`interpretMergeability`) — the same reasoning applies
  here.

## 4. Fork or adapt

- Checked `project_plans/pr-review-followup/` (`research/stack.md`, `research/pitfalls.md`)
  and `project_plans/backlog-pr-conflict-detection/` (`research/build-vs-buy.md`,
  `research/features.md`, `implementation/plan.md`) for any "steer the active session"
  idea that was scoped out or deferred. Neither mentions steering, `SendKeys`, or
  `SendCommandImmediate` in that context: `pr-review-followup` is about a different
  mechanism (its `stack.md`/`pitfalls.md` cover `gh pr` call sites and rework-blocked
  stale-session notifications, not PTY injection), and `backlog-pr-conflict-detection`'s
  scope is entirely about *detecting* conflicts (`mergeable`/`mergeStateStatus`
  interpretation), not about what to do with an active session once a problem is found.
  Neither project proposes, and neither defers, this feature's idea — there is nothing to
  adapt from them.
- `git log --all --grep=steer` confirms the steering primitive's own build history (§
  Codebase findings above) but no dead branch or abandoned PR implements
  "steer on reconciliation-detected PR problem" — the closest prior art is the *general*
  steering feature (`8dc8e7f9d` "close the operator feedback loop") which this project
  correctly treats as the machinery to call into, per the Constraints section, not as
  something to re-fork.
- `notifyRespawnBlockedByActiveSession` (`backlog_service_triage.go:1363`) is the one
  piece of existing code directly in this feature's blast radius, and it's not being
  forked so much as extended in place per requirements.md's explicit scope ("extending the
  existing... audit trail rather than replacing it").

**Verdict**: Not recommended — nothing in repo history or the two adjacent
`project_plans/` implements or defers this idea; the only real prior art is the
steering/dedup primitives themselves, which are reused via composition (calling them),
not forked (copying/adapting their code).

## Summary Verdict

| Option | Verdict |
|---|---|
| 1. OSS library for PTY-inject-with-dedup | **Not recommended** — confirmed no such library exists; tmux wrapper libs don't touch dedup, generic debounce libs solve a different problem shape |
| 2. SaaS / managed API | **Not applicable** — purely local, in-process automation with no external surface |
| 3a. Reuse `nudge_dedup.go` as-is | **Not recommended** — works by accident, actively wrong re-arm semantics for constant-PTY-output sessions |
| 3b. Generalize `nudge_dedup.go` for both callers | **Not recommended** — unjustified generic / forwarding-shim smells per `interface-pollution-checklist.md`; the two problems' comparison keys and re-arm triggers genuinely differ |
| 3c. Small independent dedup implementation | **Recommended** — ~20-30 lines, pure-function, table-tested, matches this repo's own precedent for right-sizing this kind of logic |
| 4. Fork/adapt existing code | **Not recommended** — nothing in this repo's history or the two checked `project_plans/` implements or defers this idea; only the general steering/dedup primitives exist, and those are reused via composition, not forked |

Build from scratch: a new `SessionSteerer` interface (mirroring `SessionStopper`) that
calls the existing `SteerMessage` autonomous/interactive branch logic, plus a small,
independent reason-signature dedup/cooldown pure function sitting alongside — not inside —
`nudge_dedup.go`.
