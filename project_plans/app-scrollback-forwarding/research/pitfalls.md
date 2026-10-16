# Research: Pitfalls & Risks — app-scrollback-forwarding

**Agent**: Phase 2, Agent 4 (Pitfalls Research)
**Date**: 2026-09-14

## Summary

This feature proposes sending synthetic keystrokes into a live PTY to make an
agent CLI (Claude Code first) scroll its own internal transcript, then
capturing whatever redraw results and streaming it back as a "page" of
scrollback. Every pitfall below traces back to one structural fact: **this
repo has already been burned, three separate times (BUG-013, BUG-025,
BUG-031), by writing into or reading from a live PTY shared with an agent
CLI's undocumented rendering/input behavior**, and this feature is a new,
higher-stakes instance of exactly that pattern — this time *injecting input*
into a modal TUI rather than just *parsing output* from one. None of the
prior bugs involved sending synthetic input for a purpose the user didn't
initiate; this feature is the first to do so.

## 1. Concurrency / state corruption risk (synthetic scroll keystroke racing live input/output)

### What could go wrong

Claude Code's Ink-based TUI is a single input stream with no side-channel for
"this keystroke means scroll, not command." Whatever key or key-combo the
adapter would send to scroll (e.g. a hypothetical `PageUp`/`Ctrl+O`/similar)
is indistinguishable, at the PTY byte level, from the user typing that exact
sequence themselves. Two consequences already have precedent in this repo:

- **BUG-031** (`docs/bugs/fixed/BUG-031-autonomous-driver-large-prompt-paste-not-submitted.md`)
  found that `AutonomousDriver.run`'s raw `SendKeys(nextMsg + "\r")` could be
  absorbed into Claude Code's TUI paste-detection buffer instead of being
  interpreted as submitted input, because the write raced the TUI's own
  input-handling state. The fix required splitting the write into two
  separate `SendKeys` calls with a `waitForPaneSettle` poll in between
  (`session/autonomous_driver.go`) — i.e., "send a keystroke and hope it's
  interpreted correctly" was proven unsafe here once, for a *much* simpler
  payload (literal text) than a scroll command competing with live model
  output.
- The same BUG-031 postmortem explicitly names a **recurring bug shape**
  across BUG-029/030/031: *"an action is taken but its actual effect is never
  verified."* A scroll-forwarding feature that fires a keystroke and assumes
  the next redraw is "the scroll result" repeats this exact shape a fourth
  time unless it positively verifies the redraw corresponds to the scroll
  request (see §2 — false positive risk — for why that verification is hard).

### Specific failure modes to design against

1. **Scroll keystroke lands mid-agent-turn.** If Claude Code is actively
   streaming a response when the scroll keystroke arrives, the TUI's own
   redraw (`\x1b[2J\x1b[3J` per BUG-013) can interleave with whatever partial
   redraw the scroll command triggers. The client has no way to distinguish
   "this frame is the scroll result" from "this frame is normal streaming
   output" — the exact race called out in the requirements' Rabbit Holes
   section.
2. **Scroll keystroke is interpreted as a real command** if Claude Code is
   momentarily in a different input mode than assumed (e.g. a permission
   approval prompt, `/model` picker, or any modal sub-UI). The
   `session/detection` package's `DetectedStatus` enum
   (`session/detection/detector.go:88-104`) has no explicit "modal picker"
   or "scroll-safe" status — the closest states are `StatusNeedsApproval`,
   `StatusInputRequired`, `StatusExecuting`, `StatusIdle`. None of these were
   designed to answer "is it currently safe to send a non-content keystroke
   without it being misinterpreted" — that's a new invariant this feature
   would need to establish and verify, not one it can borrow.
3. **Ordering with the existing input path.** `Instance.SendKeys`
   (`session/instance_tmux.go:1234` → `TmuxSession.SendKeys`,
   `session/tmux/tmux.go:2011`) is also the same call path used for real user
   input and for `AutonomousDriver`'s turn injection. A scroll-forward
   keystroke queued behind (or ahead of) a real pending write on the same PTY
   has no defined interleaving guarantee today — there is no per-instance
   write queue/lock visible around `SendKeys` beyond `AutonomousDriver`'s own
   `restartTriggerMu`-scoped usage, which doesn't cover this new call site.

### Design implication

Treat a forwarded scroll keystroke with at least the same caution BUG-031's
fix applied to turn injection: never fire-and-forget. At minimum, gate
sending on a detected-safe state (extend `DetectedStatus` or add a
narrower check), and verify the resulting redraw is attributable to the
scroll request (e.g. via a pane-settle poll bounded by a deadline, mirroring
`waitForPaneSettle` in `session/autonomous_driver.go`) rather than assuming
the next output chunk is the answer.

## 2. False positive / false negative detection risk

### The core problem: `Program` is a launch-time label, not a live UI-mode signal

`Instance.GetProgram()` (`session/instance_terminal.go:47`) returns
`i.Snapshot().Program` — the program the session was **configured/launched**
with (e.g. `"claude"`), set via `SwitchProgram`
(`session/instance_terminal.go` / `session/instance_program.go:59`). It is
**not** a live detection of what's currently running in the foreground of
the pane. Concretely:

- A user can drop to a plain shell inside a Claude Code session (backgrounding
  it, or exiting to `$SHELL`) without the session's `Program` field changing —
  `SwitchProgram` is only invoked by an explicit RPC
  (`UpdateSession`/capacity-monitor fallback), not by anything that observes
  actual foreground-process state.
- If the scroll-forwarding feature keys off `GetProgram() == "claude"` (the
  obvious naive implementation) to decide whether to send a scroll keystroke
  instead of falling back to tmux-native scrollback, it will send that
  keystroke into a **plain shell** whenever the user has shelled out — with
  no keybinding meaning on the shell side. The keystroke either echoes as
  literal garbage characters into the shell buffer, or — worse — is a shell
  keybinding with an unrelated meaning (e.g. some scroll-candidate
  keystrokes collide with shell history search or job control chords).
- This is the exact class of bug already documented as having happened once
  in an adjacent feature: `session/instance_program.go:12-20`'s
  `isClaudeAntigravityFamily` comment describes "the exact drift that caused
  'gemini' to be treated as portable by `AgyAdapter.CanHandle` but not by"
  the family check — i.e., two pieces of code that are supposed to agree
  on "is this program X" drifted apart and produced a real, shipped bug. A
  scroll-forwarding gate that duplicates (rather than reuses) the
  adapter-selection logic in `session/history_adapter.go:22-25`
  (`resolveHistoryAdapter`) risks the same drift.

### Version-specific keybinding risk compounds this

Even when the foreground program genuinely is Claude Code, the requirements'
own Feasibility Risks section notes it's **unconfirmed** whether any
adapter-covered CLI exposes a terminal-forwardable scroll command at all. If
a future/older Claude Code version doesn't bind the chosen key the way this
feature assumes (see §3), the same "keystroke sent to an app that doesn't
have that binding" failure mode applies even with correct program detection —
inserting literal characters or triggering an unrelated command bound to that
key in that version.

### Design implication

- Do not gate on `GetProgram()` alone. If a stronger live signal exists (the
  detection package's `DetectedStatus`, or something more direct — e.g. tmux
  alt-screen-mode state), require it, and treat `GetProgram()` as necessary
  but not sufficient.
- Route the "which programs support scroll-forwarding" check through the
  *same* single source of truth pattern used for `isClaudeAntigravityFamily`
  (derive from one adapter-capability registry, not a second independent
  string match) so a future new adapter can't silently create the same kind
  of drift bug again.
- Because both false-positive risks (wrong program, right program/wrong
  version) converge on "keystroke sent to something that doesn't expect it,"
  the mitigation needs a runtime confirmation step, not just a static
  gate — see §3's canary suggestion, which serves double duty here.

## 3. Version drift risk (undocumented external CLI behavior)

### Why this is structurally worse than the prior escape-sequence bugs

BUG-013 and BUG-025 were both cases of this repo depending on Claude Code's
*output* behavior (what escape sequences its renderer emits) — already shown
to change across versions (BUG-025's investigation explicitly notes "since
Claude Code switched to a new (Ink-based) renderer" as the trigger for a full
audit, and BUG-013's fix cites a specific upstream issue,
`anthropics/claude-code#36582`, as the only reason the exact repaint pattern
was known at all). This feature depends on Claude Code's *input handling* —
which keybinding, if any, scrolls its internal transcript — which is:

- Less likely to be documented publicly than rendering output (a keybinding
  for scrolling an internal transcript view is a UX/product decision, not a
  protocol Anthropic has committed to).
- Not discoverable by static inspection the way escape sequences are (you can
  capture a PTY's raw output bytes and pattern-match; you cannot statically
  determine which keystroke a closed-source TUI's event loop will treat as
  "scroll" without either source access or empirical testing against a
  running binary).
- Entirely silent on breakage: an escape-sequence mismatch corrupts visible
  rendering (loud, user-visible, as BUG-013 was). A scroll keybinding that
  stops working after a CLI version bump degrades to **no-op** — indistinguishable
  from the feature simply not having anything left to scroll to. Nothing
  fails loudly; the feature quietly regresses to the pre-feature baseline
  (scrolling does nothing), which is easy to miss in manual QA and has no
  natural test signal.

### Detection strategy

- **Version pin + explicit compatibility table**, not silent trust. Record
  the Claude Code version(s) the scroll keybinding was verified against
  (mirroring how `session/detection/detector.go`'s pattern-matching approach
  already implicitly assumes specific CLI output shapes, but doing so
  *explicitly* this time rather than accreting undocumented assumptions).
- **A canary check**, analogous to the two canaries this repo already has for
  the same class of problem — `compactingCanary` and
  `shellMonitorWordingCanary` (`session/detection/detector.go:361-413`) —
  which exist specifically to detect when a *textual* pattern this repo
  depends on has silently stopped matching real CLI output. A scroll-forward
  canary would need a live-state equivalent: after sending the scroll
  keystroke, verify the pane actually changed in a way consistent with
  "scrolled" (not just "any redraw happened," which live output would also
  produce) within a bounded window, and log/flag when it doesn't — surfacing
  drift instead of silently no-op'ing forever.
- **Feature-flag gate is the operationally cheap first line of defense** —
  requirements' Risk Control section already calls for this. Given this
  repo's live-settable, per-scope flag infrastructure
  (`server/services/feature_flag_service.go`), a version bump that breaks the
  keybinding can be responded to by flipping the flag off globally without a
  deploy, buying time to re-verify the keybinding against the new version —
  but only if the flag is actually wired to gate the *send* path per-adapter
  as the requirements ask, not just a top-level "is scroll-forwarding on"
  toggle that can't be scoped to a specific adapter/version once one breaks.

## 4. Multi-client risk (concurrent connections to the same session)

### Existing precedent this repo already accepted — and why it doesn't transfer

`docs/tasks/scrollback-client-delivery.md`'s "Known Issues" section
(`Concurrent connects to same session duplicate history`, line ~454) treats
concurrent connects as a non-issue for the *existing*, already-shipped,
non-forwarding scrollback feature:

> "If two clients connect to the same session simultaneously, both receive
> the history bytes independently. This is correct behavior... There is no
> shared state issue."

That reasoning holds specifically because tmux-native scrollback capture
(`CapturePaneContentWithOptions`) is a **read-only, idempotent** operation
against tmux's own history buffer — capturing it twice, for two clients,
changes nothing about the session's live state. **Scroll-forwarding breaks
that precondition**: sending a scroll keystroke is a **write** to the shared
PTY, and the resulting redraw is not scoped to the requesting client — it is
the *actual current rendered state of the pane*, which every connected client
receives via the same live-streaming path
(`streamViaControlMode`/`streamViaTmuxCapturePane`,
`server/services/connectrpc_websocket.go`).

### Concrete failure

If client A scrolls up (forwarded to the app), the resulting redraw is not a
private "page" delivered only to A — it changes what the *actual foreground
pane* shows, which is exactly what gets streamed live to client B too. Client
B, who did nothing, sees their live view jump to whatever transcript position
A scrolled to. This is a strictly worse UX regression than "scrolling does
nothing" (the status quo the requirements describe as the bug being fixed):
today, a second client's live view is simply unaffected by a first client's
inert scroll attempt; after this feature, a first client's scroll actively
corrupts a second client's live view of current agent activity.

### Design implication

This is very likely the single highest-severity architectural gap in the
current requirements scope. The requirements document doesn't mention
multi-client behavior at all. Given the mechanism (forwarding literally
becomes a shared, observable mutation of the one real PTY, not a per-client
read), there is no way to make a forwarded scroll "private" to the requesting
client without either (a) restricting the feature to single-client sessions
only, (b) some form of leader/follower client arbitration (only the
"focused" client's scroll gets forwarded, others are locked out or shown a
different indicator), or (c) accepting and explicitly documenting that a
forwarded scroll is a shared, disruptive action — the opposite of the
existing tmux-native scrollback's per-client independence. This needs an
explicit decision in the plan phase, not a "known issue, acceptable" footnote
like the precedent it can't actually reuse.

## 5. Accessibility / mobile risk (touch-scroll interaction)

### Existing touch-scroll surface

`web-app/src/components/sessions/XtermTerminal.tsx` already layers multiple
custom touch handlers onto xterm.js's native viewport:

- A custom left-side scrollbar track/thumb (`scrollTrackRef`/`scrollThumbRef`,
  `updateScrollbar` around line 420) kept in sync with
  `terminal.buffer.active.viewportY` via a dedicated `onScroll` xterm
  listener (line 791).
- Touch-drag text selection, gated behind `isTouchPrimary` (a
  `pointer: coarse` media-query check, line 555), registering
  `touchstart`/`touchmove`/`touchend` at the document level per selection
  handle (lines 814-886).
- A separate touch handler for the scrollbar thumb itself
  (`onScrollThumbTouchStart`/`onScrollThumbTouchMove`, lines 972-1008).
- A code comment (line 257-258) already documents one prior touch-handling
  bug class: "Replaces the conflicting `useTouchScroll` +
  `useMobileTerminalGestures` hooks: having both register `touchmove` caused
  double-scroll and prevented selection" — i.e., this exact file has already
  had a real double-handler touch conflict bug, in the same interaction
  surface this feature would need to extend.

### Where the trigger actually lives, and why it likely won't fire for app-managed scrollback

The existing scroll-triggers-pagination logic lives in
`web-app/src/components/sessions/TerminalOutput.tsx` (~line 1046-1060), not
`XtermTerminal.tsx`: it listens on the `.xterm-viewport` DOM element's native
`scroll` event and fires `requestScrollback` when
`terminal.buffer.active.viewportY < 200`. This depends on the *browser*
successfully generating scroll events against xterm's own DOM scroll
container — which requires there to be scrollable content in xterm's buffer
in the first place. For a foreground app that manages its own scrollback
internally (Claude Code, per the requirements' Problem Statement), xterm's
own buffer for that pane is likely to already be near-empty/non-scrollable
(everything is redrawn in place, not scrolled through xterm's history), so
the *existing* trigger mechanism may never fire a native `scroll` event at
all on touch devices — there's nothing to scroll natively, which on touch
means no scroll gesture is even recognized as having happened, let alone
compared against the `viewportY < 200` threshold.

### Concrete risks to design against

1. **Gesture detection needs a new source on touch.** If forwarding depends
   on the current `.xterm-viewport` `scroll` event, it inherits this gap on
   mobile specifically — touch scroll-up over a non-scrollable/near-empty
   xterm buffer may not produce the DOM scroll events this code currently
   relies on. A touch-specific gesture listener (independent of native
   scroll) is likely required, which means writing a *fourth* touch handler
   into a file that has already had one documented double-handler conflict
   bug (the line 257-258 comment above) — the same class of risk exists
   again unless the new handler is designed to compose with, not duplicate,
   the existing selection-drag and scrollbar-thumb touch handlers.
2. **Gesture ambiguity between selection-drag and scroll-forward.** Touch-drag
   selection (lines 814-886) is already triggered by a touch-and-hold-then-move
   pattern on the terminal surface. A forwarded-scroll gesture (likely a
   vertical swipe) needs a clear disambiguation rule against both
   text-selection-drag and the custom scrollbar-thumb drag — three
   touch-driven interpretations of "finger moves vertically on the terminal"
   competing for the same input surface.
3. **No mobile-native visual affordance for "this scroll is being forwarded to
   the app, not handled locally."** Unlike native xterm scrollback (instant,
   client-side, no round-trip), a forwarded scroll requires a PTY round-trip
   and redraw wait — on mobile in particular (higher latency, harder to
   perceive "did my gesture register"), this needs a loading/pending
   indicator distinct from normal scroll, or users will repeat the gesture
   assuming it didn't register, compounding the concurrency risk in §1 by
   sending multiple overlapping scroll keystrokes.

## Cross-Cutting Recommendations

1. **Treat this as PTY-input-injection, not PTY-output-parsing** — the
   engineering bar should match `AutonomousDriver`'s (verify effect, don't
   fire-and-forget), not the detection package's (best-effort pattern
   match on output is acceptable there because failure just means a status
   badge is momentarily wrong; failure here means characters land in a live
   agent session or a shell).
2. **Single source of truth for "is this program scroll-forward-capable"** —
   do not let a new adapter-capability check drift independently of
   `resolveHistoryAdapter`/`CanHandle`, repeating the documented
   `isClaudeAntigravityFamily` drift bug.
3. **Multi-client semantics need an explicit decision before implementation**
   — the existing "duplicate history is fine" precedent in
   `docs/tasks/scrollback-client-delivery.md` does not transfer, because
   forwarding is a write to shared state, not an independent read.
4. **Canary the keybinding assumption, not just the feature flag** — a
   silent no-op regression on CLI version bump has no natural test signal;
   without an active canary, this will only be discovered by a user noticing
   scrolling "stopped working" again, with no error anywhere.
5. **Design the touch-gesture trigger before assuming the existing
   `viewportY`-based `scroll` listener in `TerminalOutput.tsx` covers
   mobile** — it may not fire at all for an app-managed-scrollback pane on
   touch, and any new touch handler must be designed against
   `XtermTerminal.tsx`'s already-documented history of touch-handler
   conflicts.

## Key File References

- `session/autonomous_driver.go` — `waitForPaneSettle` pattern (verify PTY
  write effect before proceeding); reusable building block explicitly
  flagged in BUG-031 for future PTY-write call sites.
- `session/instance_tmux.go:1234` (`Instance.SendKeys`) /
  `session/tmux/tmux.go:2011` (`TmuxSession.SendKeys`) — the shared raw
  PTY-write call path this feature would reuse, with no existing
  write-ordering guarantee beyond `AutonomousDriver`'s own lock scope.
- `session/instance_terminal.go:47` (`GetProgram`) — launch-time program
  label, not a live foreground-mode signal; do not gate solely on this.
- `session/instance_program.go:12-20` — documented precedent of adapter
  capability-check drift (`isClaudeAntigravityFamily` vs `AgyAdapter.CanHandle`
  disagreeing on "gemini").
- `session/history_adapter.go:22-25` (`resolveHistoryAdapter`) — the single
  source of truth pattern worth reusing for a scroll-capability registry.
- `session/detection/detector.go:88-104` (`DetectedStatus`) — existing
  status enum with no "safe to send non-content keystroke" state; the
  closest analogues (`StatusIdle`, `StatusExecuting`, `StatusNeedsApproval`)
  were not designed for this purpose.
- `session/detection/detector.go:361-413` (`compactingCanary`,
  `shellMonitorWordingCanary`) — existing canary pattern for detecting
  silent drift in CLI text output; a live-state analogue is needed for
  scroll-command drift.
- `server/services/feature_flag_service.go` — live-settable, per-scope
  feature-flag infrastructure the Risk Control section should be built on.
- `server/services/connectrpc_websocket.go` (`streamViaControlMode`,
  `streamViaTmuxCapturePane`, `handleScrollbackRequest` ~line 3116) — the
  live-streaming paths every connected client shares; the mechanism by which
  a forwarded scroll's redraw becomes visible to all clients, not just the
  requester.
- `docs/tasks/scrollback-client-delivery.md` — precedent "duplicate history
  on concurrent connect" note (does not transfer to a write-based mechanism)
  and the ANSI-safety analysis this feature should model its own safety
  writeup on.
- `web-app/src/components/sessions/TerminalOutput.tsx:1046-1060` — the
  existing `viewportY < 200` native-scroll-driven trigger for
  `ScrollbackRequest`; likely insufficient for app-managed-scrollback panes,
  especially on touch.
- `web-app/src/components/sessions/XtermTerminal.tsx` — touch-scroll-thumb,
  touch-drag-selection, and custom-scrollbar code, including a documented
  prior double-touch-handler bug (line 257-258 comment).
- `docs/bugs/fixed/BUG-013-xterm-viewport-corruption-from-ed3-sequence.md`,
  `BUG-025-csi-terminator-range-too-narrow.md`,
  `BUG-031-autonomous-driver-large-prompt-paste-not-submitted.md` — the
  repo's prior PTY-corruption/undocumented-CLI-behavior bug class this
  feature is a new instance of.
