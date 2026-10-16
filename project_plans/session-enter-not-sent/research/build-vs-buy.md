# Build vs. Buy: session-enter-not-sent

**Date**: 2026-09-28
**Researcher**: Agent 6 (Phase 2, SDD)
**Repo state cited**: `stapler-squad` @ `1dced16a9` (this worktree's HEAD)

## Framing

This project has no new feature to build (Complexity 1 — verification/closure
of an already-shipped fix, PR #832 / `e2085dec4`). The build-vs-buy question
here is retrospective and forward-looking, not a design choice to make now:
(1) was the shipped fix's own bespoke design justified against what the repo
already depends on, and (2) should the remaining live-smoke-test work reuse
existing test infrastructure rather than be hand-rolled from scratch.

---

## Option 1: Bespoke two-write-plus-confirm protocol (`session.SubmitDriverContent`) vs. an existing terminal-automation/expect library

**Verdict: Recommended (the bespoke choice was justified) — but note the repo
already has expect-style tooling, just scoped to tests, not production.**

`go.mod` shows two terminal-automation dependencies already in the module
graph:

- `github.com/Netflix/go-expect v0.0.0-20220104043353-73e0943537d2`
- `github.com/creack/pty v1.1.24`

Both are used exclusively by `testutil/expect.go` (`TUISession`, an
expect-style pattern-matching test harness) and its consumers
(`testutil/tui_*_test.go`) — i.e., they drive a locally-spawned PTY process
directly for *testing* stapler-squad's own TUI behavior. Neither is wired
into the production submit path at all.

The production path doesn't talk to a PTY directly — it talks to a *tmux
session* through two backends, neither of which wraps an off-the-shelf tmux
Go library:

- `session/tmux_process_manager.go`'s `TmuxProcessManager.SendKeys` delegates
  to the repo's own tmux session type (`session/instance_tmux.go`), which
  drives tmux control mode itself.
- `session/tymux/session.go`'s `tymuxGRPCSession.SendKeys`/`TapEnter` go
  over an internal gRPC `AttachRequest_Input` stream to a separate `tymuxd`
  daemon (bundled, not third-party).

No `github.com/.../gotmux` or similar tmux-control-mode client library
appears anywhere in `go.mod`. `go-expect` is architecturally the wrong tool
for the production submit path even if it had been considered: it expects to
own the PTY master/slave pair of a process it spawned directly (`pty.Open()`
+ `exec.Command`), which doesn't match "send keystrokes into an existing
remote tmux pane that the process doesn't own or spawn" — stapler-squad's
actual shape once tmux control mode or the tymux gRPC bridge sits in
between. Adapting go-expect to that shape would mean re-implementing the
send/observe loop around it anyway, at which point the wrapper adds no
value over `SubmitDriverContent`'s ~30 lines.

The bespoke pieces that mattered here were narrow and specific to
stapler-squad's own accidental complexity, not generic PTY automation:

- Two separate `SendKeys` writes with a settle-wait between them, to close
  Claude Code's Ink-TUI paste-detector window (`session/pane_submit.go:49-66`,
  BUG-031's root cause) — no generic library would know this app-specific
  timing quirk exists.
- Confirm-and-retry via `HasUpdated()` polling
  (`session/pane_submit.go:161-181`), reusing the repo's own pane-diff
  primitive that already existed for other purposes rather than adding a new
  dependency's own "wait for output matching X" pattern-match loop (which is
  what go-expect actually offers, and isn't the right primitive here — there's
  no known-in-advance pattern to match against, just "did the pane change at
  all").
- `session/sendkeysguard`, an AST-based structural regression guard specific
  to this repo's call-site history (three independent recurrences of the same
  bug before consolidation) — inherently un-buyable.

**Pros of the bespoke approach**: zero new dependency surface; reuses
`HasUpdated()`, a primitive the codebase already needed for other reasons;
encodes app-specific timing knowledge (paste-detector window) no generic
library would have; ~180 lines total (`pane_submit.go`), proportionate to the
problem.
**Cons**: the confirm/retry logic is stapler-squad's own code to maintain,
and (per Alternatives Considered below) the retry-can-double-submit tradeoff
is a real, self-owned risk.
**Verdict**: Recommended. No adjacent library fits the actual shape of the
problem (writing into an existing remote tmux pane via control mode / gRPC,
not spawning and owning a PTY), and the two dependencies already in
`go.mod` for this problem space are correctly scoped to test-only use.

---

## Option 2: Live smoke test — hand-rolled manual click-through vs. existing test harness/e2e/CI reuse

**Verdict: Viable to extend existing e2e coverage; a fully manual
click-through is Not recommended given what's already close.**

Checked three places for existing real-PTY/real-tmux coverage of the three
affected tools:

1. **`session/pane_submit_test.go`** — mock-only. `fakePaneSubmitter`,
   `retryConfirmFake`, and `blockingKeySender` (lines 19, 142, 271) are all
   hand-rolled fakes satisfying the `paneSubmitter`/`keySender` interfaces;
   no real tmux/PTY backend is instantiated anywhere in this file. Confirms
   requirements.md's own claim that this coverage is unit-level only.

2. **`server/mcp/tools_terminal_test.go`** — same. Its own comment at line
   481 is explicit: *"cannot be tested without tmux in unit tests"* —
   `TestSteerSessionMCP_passesValidationAndReachesSendKeys` (line 491)
   deliberately accepts either a validation-layer failure or success "if
   somehow it worked" (line 519), because it has no real backend to assert
   against. **This means the MCP-tool call sites themselves
   (`write_to_session`, `steer_session`, `run_command`) have zero real-PTY
   test coverage anywhere in the Go test suite** — the gap the live smoke
   test is meant to close is real, not already covered and just
   unacknowledged.

3. **`tests/e2e/backlog-session-steer.spec.ts`** — the closest existing
   thing to a live smoke test, but for a different call path (the
   `UpdateSession` ConnectRPC steering path, not the MCP tools) and a
   narrower shape than what's needed:
   - It creates a **real tmux-backed session** via `SessionClient.createSession`
     (`program: "bash"`) and drives it through the real `UpdateSession` RPC →
     real `Instance` → real `SendKeys`/`SubmitContentWithEnter` — see the
     spec's own header comment (lines 9-19) confirming this is intentional,
     not mocked, following `backlog-pipeline-mode.spec.ts`'s precedent.
   - It asserts only the UI-level `"Steering message sent."` toast (lines
     170, 218) — which *is* a meaningful proxy (that toast only renders on a
     non-error RPC response, and `SubmitContentWithEnter` returns
     `ErrSubmitNotConfirmed` on a swallowed submit, which would surface as an
     RPC error instead), but it never asserts on the pane's actual rendered
     content, and it targets a plain `bash` prompt, not a real Claude Code
     Ink-TUI target.
   - The test messages (`"please run the tests"`, `"run the linter"`) are
     short single-line strings — not the long/multi-line shape that
     specifically triggers the Ink-TUI paste-detector race PR #832 fixed
     (requirements.md's own Open Questions section flags this exact gap).

**Recommendation for the closure task**: don't hand-roll a fully manual
click-through from zero. Two cheaper paths, in order of preference:

- **Reuse the documented manual-instance convention** (per this repo's
  `CLAUDE.md` "Manual/interactive testing" section) to spin up a second
  instance, start a real Claude Code session in it, and call
  `write_to_session`/`steer_session`/`run_command` against it with a
  genuinely long/multi-line payload — this is the actual gap (a real Ink-TUI
  target + long input), and nothing existing covers it, so this step can't
  be skipped. This is what requirements.md's Success Metrics already call
  for; it's a ~15-minute manual check, not new engineering.
  Do **not** run `make install-service` for this (would kill the live
  deployed instance's tmux server per the WARNING in the project's own
  `CLAUDE.md`).
- Optionally, `backlog-session-steer.spec.ts` is a reasonable template to
  crib from if any of this ever needs to become an automated regression test
  (real-session bootstrap pattern, cleanup-in-`finally` pattern) — but
  extending it now would exceed this item's "Small, well under a day"
  appetite for what is explicitly a closure task, and out-of-scope per
  requirements.md ("no new code changes to the submit path" / no
  re-litigating already-reviewed design). Note it for a future PR if the
  live smoke test finds a real gap, not as part of this closure.

---

## Option 3: SaaS/managed alternative

**Verdict: Not applicable.** This is local PTY/tmux automation against a
process running on the same machine as the caller (or a daemon it directly
controls, for the tymux backend) — there is no managed/hosted service that
sends keystrokes into a local tmux pane. Noted per the research brief and
moved on.

---

## Option 4: Fork-or-adapt — does an existing internal helper already do
confirm-and-retry that the tymux gap (BUG-113-adjacent) could adopt?

**Verdict: Not recommended today — `SubmitDriverContent` itself is the
future target, but tymux can't use it yet; no other existing helper fills
the gap.**

- `session/tymux/session.go`'s `SendPromptWithEnter` (lines ~542-548) already
  independently adopted the *first* half of the pattern — two separate sends
  (`SendKeys` then `TapEnter`) with a settle delay
  (`tymuxPromptSettleDelay`) between them — its own doc comment cites BUG-031
  explicitly. So tymux is not naively concatenating; it already tracks
  `TmuxProcessManager`'s shape for the write-then-Enter split.
- What it cannot adopt is the *confirm* half. `HasUpdated()` on
  `tymuxGRPCSession` (lines ~719-721) is permanently stubbed:
  `return false, false, ""`. Plugging tymux's session type directly into
  `SubmitDriverContent` today wouldn't add confirm/retry — it would make
  every tymux submit look unconfirmed, trigger the blind Enter retry every
  single time, and then always return `ErrSubmitNotConfirmed` regardless of
  whether the first Enter actually landed. That's strictly worse than
  tymux's current silent-success behavior for callers who don't check the
  return value carefully.
- Searched `session/tymux/*.go` and `session/*.go` for any other
  confirm-and-retry helper that's simpler than `SubmitDriverContent` and
  could be adopted now — none exists. `RetryState`/`retry_state.go`'s retry
  machinery is a different concept (session-restart backoff policy, not
  keystroke-submission confirmation).
- **Conclusion**: no fork-or-adapt work is needed or possible right now.
  The correct sequencing (already implied by requirements.md's own framing,
  and reaffirmed here) is: BUG-113 lands → tymux launches the real target
  program instead of `$SHELL` → *then* a follow-up implements a real
  `HasUpdated()` for the tymux gRPC backend (reading actual pane-diff state
  from the daemon, analogous to what `TmuxProcessManager` already gets from
  tmux control mode) → *then* `tymuxGRPCSession` can satisfy
  `paneSubmitter` for real and `SendPromptWithEnter` can be replaced by a
  call through `SubmitDriverContent`, closing the parity gap with a net
  code *deletion* rather than new design work. This confirms this item's own
  scoping decision to defer the tymux gap rather than pull it in now.

---

## Summary Table

| Option | Verdict |
|---|---|
| 1. Bespoke `SubmitDriverContent` vs. adopting an expect/PTY library | Recommended — no library fits the "existing remote tmux pane via control mode/gRPC" shape; `go-expect`/`creack/pty` are correctly test-only |
| 2. Live smoke test: reuse vs. hand-roll | Viable — no automated test closes the exact gap (MCP-tool + real Ink-TUI + long input), but `backlog-session-steer.spec.ts` proves the real-tmux mechanics work end to end and is a template if this is ever automated; the actual closure step should be the documented manual-instance smoke test, not a new automated harness |
| 3. SaaS/managed alternative | Not applicable (local PTY automation) |
| 4. Fork-or-adapt for tymux's gap | Not recommended today — blocked on `HasUpdated()` being unimplemented for the tymux gRPC backend; `SubmitDriverContent` is the eventual target once that lands, no interim helper exists or is needed |
