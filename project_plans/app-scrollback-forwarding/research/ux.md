# UX Research: app-scrollback-forwarding

Agent 5 (UX Research), SDD Phase 2. Scope: comparable UX patterns, user mental
models, accessibility, error/edge-case UX, and jobs-to-be-done for forwarding a
client scroll-up gesture into an app-managed scrollback (starting with Claude
Code).

## 0. Current baseline in this codebase

Before comparing to other products, it matters what "scroll-up" already does
here, because the new feature has to compose with it, not replace it.

- **tmux-native scrollback pagination already exists and is the reference UX
  the new feature must match or explicitly diverge from.**
  `web-app/src/components/sessions/TerminalOutput.tsx:1034-1063` attaches a DOM
  `scroll` listener to xterm.js's `.xterm-viewport` element (not
  `terminal.onScroll`, which only fires on buffer writes — see the comment
  citing xterm.js issues #3201/#3864). When `viewportY < 200` and more history
  is available (`hasMoreScrollbackRef`), it calls
  `requestScrollback(oldestSequenceReceivedRef.current, 500)`, which round-trips
  to the server and prepends older lines via
  `TerminalStreamManager.prependScrollbackBatch` (`web-app/src/lib/terminal/TerminalStreamManager.ts:317-353`,
  a serialize-clear-rewrite pattern: serialize current buffer → clear → write
  older history → write serialized content back → scroll to bottom).
- **There is no visible loading indicator for this in-flight paged fetch.**
  The only loading spinner in `TerminalOutput.tsx` is gated on
  `isLoadingInitialContent` (`TerminalOutput.tsx:1919-1921`, `styles.loadingSpinner`),
  which covers the very first content load, not `isFetchingScrollbackRef`
  (set at `TerminalOutput.tsx:1055`, never read to drive UI). This confirms
  the requirements doc's Baseline claim in code: scrolling to the top today
  gives "no loading indicator, no error, no explanation" even for the
  already-working tmux-native case — the new self-managed-scrollback case
  inherits this gap and should not make it worse.
- **No existing detection of "does the foreground app self-manage its own
  scrollback."** `session/detection/binaries/claude.go` and
  `session/detection/detector.go` detect Claude Code's *status* (idle/running/
  waiting), not a scrollback-capability flag. This is a gap the plan phase
  needs to close (likely a new field on the detector/adapter registry), not
  a UX-only concern, but it directly shapes UX: the client needs a signal to
  decide "attempt forwarding" vs. "use tmux-native path" vs. "give up
  gracefully."
- **Mobile scroll is a fully custom touch pipeline, not native browser
  scroll-then-event.** `XtermTerminal.tsx` disables xterm's built-in mouse
  wheel/touch handling on touch-primary devices (`isTouchPrimary`, detected via
  `window.matchMedia('(pointer: coarse)')` at `XtermTerminal.tsx:555`) and
  replaces it with: a custom left-side scrollbar track/thumb
  (`scrollTrackRef`/`scrollThumbRef`, `XtermTerminal.tsx:238-240`, drag logic
  `XtermTerminal.tsx:932-1000`), and a separate touch-drag *selection* handler
  (`pointToCell`/`rafThrottlePoint` from `touchDrag.ts`, wired at
  `XtermTerminal.tsx:814-886`) that a comment explicitly notes replaced two
  earlier conflicting hooks (`useTouchScroll` + `useMobileTerminalGestures`)
  because "having both register touchmove caused double-scroll and prevented
  selection" (`XtermTerminal.tsx:257-258`). This history is a strong signal:
  mobile scroll-forwarding must not add a third competing touch handler onto
  an already-fragile, previously-buggy pipeline.

## 1. Comparable UX patterns

### tmux copy-mode (the direct ancestor of this product's own scrollback)
tmux's native scrollback and its own internal application-scrollback problem
are the closest analog, and tmux's own docs (`man tmux`, `COPY MODE` section)
are instructive because tmux faces exactly this bifurcation:
- Entering copy-mode (`prefix []`) is an **explicit, discrete mode switch**
  with its own status-line indicator (`[0/1234]` position counter) — the user
  always knows they are in a different interaction mode, not just scrolled.
  There is no ambiguity about "whose scrollback am I looking at."
  tmux never tries to forward scroll into the inner app's own pager
  automatically; the user must either scroll tmux's buffer (which, like this
  product's tmux-native path, only has what was written to the pty) or exit
  copy-mode and let the inner app's own keys work.
- tmux's `mouse` option, when on, sends wheel events to the *foreground
  application* if that application has requested mouse reporting (the same
  mechanism `less`/`git log --paginate`/`vim` use) — this is effectively
  "forwarding" already, but it's binary and always-on per pane, not a
  targeted "forward on scroll-up-at-top" gesture. This product's
  `TerminalOutput.tsx:344-345` mouse-tracking-mode comment ("none" on mobile,
  "any" on desktop) shows this exact tmux mechanism already partially in use
  for vim/tmux mouse support — the new feature is a variant of the same idea,
  scoped to one specific gesture and one specific app family (agent CLIs)
  rather than blanket mouse-report forwarding.

### `less` / `git log` pager UX
- Pager exit/entry is unambiguous to the underlying terminal: the pager uses
  the alternate screen buffer, so the terminal emulator (and iTerm2, see below)
  can tell definitively "an app is now owning the whole screen." This product's
  self-managed-scrollback case is *not* guaranteed to use the alt-screen the
  same way — Claude Code's own transcript view is drawn in the *primary*
  screen buffer with its own internal cursor/redraw logic, which is exactly
  why `tmux capture-pane` can't recover it (per the requirements doc's Problem
  Statement). This is a meaningful UX difference from `less`: there's no
  terminal-level signal (alt-screen on/off) marking "you're now in the app's
  own scrollback," so the client can't reuse iTerm2/tmux's existing detection
  primitive and must invent its own (see Open Question below).

### iTerm2 scrollback vs. alt-screen handling
- iTerm2 explicitly **disables its native scrollback while the alternate
  screen buffer is active** (used by `vim`, `less`, full-screen TUIs) and
  restores it on exit — this is documented, user-visible behavior (iTerm2
  shows no scrollback history while an alt-screen app is foregrounded, and
  many users are confused the first time this happens, per longstanding
  iTerm2/tmux user reports on the difference). The lesson for this feature:
  *swapping scrollback semantics based on foreground-app state is a known
  source of user confusion even in mature terminal products*, which directly
  motivates the requirements doc's Open Question about whether the client UI
  should indicate "you're now viewing the app's own scrollback."

### VS Code integrated terminal
- VS Code's terminal has a "Scroll To Top/Bottom" command but, like tmux and
  iTerm2, does not attempt to forward scroll gestures into TUI apps' internal
  pagers — it treats the terminal purely as a pty renderer. There's no
  precedent there for automatic gesture-forwarding into an app; this is a
  genuinely novel interaction for this feature to introduce, which raises the
  bar on discoverability/affordance since there's no existing convention to
  lean on.

### AI-coding-session products (Cursor, Claude Code's own CLI transcript, Copilot Workspace)
- Claude Code's own terminal-based CLI *is* the app whose internal scrollback
  this feature is trying to reach; its own transcript scrolling (arrow
  keys / PageUp-PageDown inside its TUI) is the "app's own scroll input" the
  requirements doc refers to forwarding. No public terminal-multiplexer or
  IDE product was found that already solves "forward host scroll gesture into
  a hosted TUI's internal pager" as a general mechanism — this appears to be
  a product-specific integration problem, not one with an off-the-shelf UX
  pattern to copy. The nearest prior art is mouse-reporting passthrough
  (tmux/iTerm2, above), which differs in being continuous/always-on rather
  than an edge-triggered "hit top → forward one page" interaction.

## 2. User mental models and expectations

- **Users conflate "the terminal" with "the conversation."** For a plain
  shell, users have a well-worn model: scrollback = everything printed, finite,
  and once you're at the top you're at the top (of *that session*). For an
  agent CLI's TUI, the visible surface is a *chat transcript*, not raw shell
  output — the product framing throughout this codebase (backlog items,
  session cards, "conversation" language in `docs/reference/*`) reinforces
  that users think of these sessions as conversations they're reviewing, not
  streams of terminal bytes. Scrolling up in a chat-like surface sets an
  expectation closer to "load older messages" (Slack, WhatsApp, any chat UI
  with infinite-scroll-up) than "reveal previously-printed shell lines."
  Chat products near-universally show a loading spinner/skeleton at the top
  during that load — the *absence* of one here (per §0) will read as broken,
  not merely quiet, because the mental model is chat, not terminal.
- **Users do not know, and should not need to know, that tmux's screen buffer
  and the inner app's own transcript are different data sources.** The
  requirements doc's baseline ("scrolling... simply does nothing... no
  explanation") is worse than a plain shell's ceiling, because a plain shell
  at least *behaves consistently* with the "finite scrollback" model (you can
  tell you hit the top because the content stops changing but the UI was
  never claiming to do anything else). An agent CLI silently doing nothing on
  scroll-up, when the user's mental model says "there must be more, this is a
  long conversation," reads as a bug, not a boundary.
  This is the core problem the feature must fix, and it is squarely a
  perceived-affordance problem before it's a technical one: even a
  well-implemented forwarding mechanism will fail the user's mental model
  if it can't distinguish "there is genuinely no more history" from
  "forwarding isn't supported for this app" from "still loading" — three
  states a plain shell scrollback never has to represent, because it has
  only two (more history / at the top).
- **Expectation asymmetry between desktop and mobile.** Desktop users
  scrolling a terminal with a wheel/trackpad have fine-grained, continuous
  gesture feedback (each notch = a few lines) and are more likely to notice
  a partial or stalled response. Mobile users performing a touch-scroll swipe
  expect the same rubber-banding/momentum feedback native apps give them
  (iOS/Android system scroll), and a swipe that "does nothing" reads as an
  unresponsive touchscreen, a much harsher failure than a desktop user seeing
  a wheel notch not scroll further.

## 3. Accessibility

Two accessibility-relevant surfaces already exist in this codebase and both
apply directly:

- **`web-app/src/lib/terminal/README.md`** documents xterm.js as
  **canvas-rendered with WebGL acceleration** (`README.md:8-9`) — this is a
  hard, already-acknowledged constraint: canvas-rendered terminal content is
  opaque to screen readers by default (no DOM text nodes for xterm's own
  buffer content), which is why xterm.js ships an internal accessibility tree
  overlay (a hidden, synced DOM mirror) as a *separate* mechanism from the
  canvas paint. Anything this feature adds that changes *what content is in
  the buffer* (forwarded app-scrollback pages) will automatically flow
  through xterm's existing a11y-tree mirror for free — but anything that adds
  a *new UI affordance* (a loading indicator, a "you're in app scrollback"
  banner, an error toast) needs its own explicit ARIA treatment, because
  those live outside xterm's canvas/a11y-tree pairing entirely.
- **The codebase's established sr-only/`aria-live` pattern**
  (`web-app/src/styles/a11y.css.ts:1-20`, `visuallyHidden` style) is already
  used for `aria-live="polite"` announcer spans elsewhere (ReviewQueuePanel,
  TriggersPanel, TriggerFormModal, CallbackSettings per that file's own
  comment) and is precedented *within this exact component*:
  `XtermTerminal.tsx:1325` already has an `aria-live="polite"` region (for
  copy-scrollback feedback, judging by context around
  `handleCopyScrollbackPointerDown` at `XtermTerminal.tsx:301`). Any new
  status text this feature needs ("Loading more of Claude Code's history…",
  "No more history available", "Couldn't load more — showing latest known
  view") should reuse this exact pattern (a `visuallyHidden` + `aria-live`
  span) rather than inventing a new announcer, for consistency and because
  it's the only mechanism in this component already proven to reach screen
  reader users past the canvas boundary.
- **Keyboard navigation**: `XtermTerminal.tsx` currently drives scroll via
  wheel/touch/thumb-drag; there is no dedicated keyboard scroll-up/down
  binding visible in the grepped ranges (PageUp/PageDown are typically
  forwarded to the pty as raw input, not intercepted as a "scroll the
  viewport" command, in a terminal emulator). If this feature adds any new
  *keyboard-triggered* forwarding path (e.g. a toolbar button per §4's
  discoverability concern), it must be independently focusable/operable per
  WCAG 2.1 SC 2.1.1 (Keyboard) — don't assume mouse/touch-only coverage is
  sufficient, since the existing scroll-to-bottom button
  (`XtermTerminal.tsx:1573-1578`, `ariaLabel: 'Scroll to bottom'`) is already
  a precedent for a keyboard-operable scroll-control button in this exact
  toolbar and is the natural place to add a "Load older" affordance if
  passive scroll-detection proves insufficient or ambiguous (see §4a).
- **WCAG-relevant color/contrast**: any new banner/indicator ("app scrollback
  mode") should be checked against `web-app/src/styles` theme tokens for
  AA contrast, same as any other UI text — no special exemption because it's
  terminal-adjacent chrome rather than terminal content.

## 4. Error / edge-case UX

### (a) Forwarding attempted, app has no scroll command, nothing visibly changes
This is the single highest-risk UX failure mode named in the requirements
doc's own Feasibility Risks section, and it's indistinguishable, at the
byte level, from "forwarding worked but there happened to be no new content
to show" (e.g. the agent CLI redrew an identical frame). The client cannot
reliably detect this from the terminal output alone.
- **Recommendation**: treat capability as a per-adapter, *statically known*
  flag (populated once feasibility is confirmed per app, per the
  requirements doc's Appetite section — "sequence Claude Code first, extend
  to other adapters as feasibility is confirmed"), not something inferred
  live per-scroll from output diffing. If an app isn't on the known-capable
  list, don't attempt forwarding at all — fall back to the existing baseline
  (today's silent no-op, or better, a static "no more history available for
  this session type" message) rather than attempting and silently failing.
  This converts an ambiguous runtime failure into a deterministic, testable
  condition, and avoids the false-positive risk of a redraw that happens to
  look identical.
- If a capable app *does* attempt forwarding and the resulting redraw is
  byte-identical to the pre-forward frame (a detectable condition, unlike the
  no-command case), that's the honest signal for "you've reached the top of
  what this app will show you" — treat it the same as case (b).

### (b) Forwarding succeeds but hits the app's own hard limit
- This is functionally identical, from the user's perspective, to reaching
  the top of tmux-native scrollback — and per §0, that existing case
  *already has no explicit "you've reached the top" indicator* in this
  codebase (`hasMoreScrollbackRef` just silently stops triggering more
  fetches once `metadata.hasMore` is false). Recommend the plan phase
  close this gap for *both* paths at once (a shared "reached the top /
  no more history" affordance) rather than building a bespoke one only for
  the new forwarded path — the two cases should feel identical to the user,
  since the distinction (tmux buffer vs. app-internal limit) is an
  implementation detail, not something the user needs to reason about.

### (c) Forwarding mid-flight, new live output arrives concurrently
- Named explicitly as a Rabbit Hole in the requirements doc, and rightly so:
  this is the crux technical/UX risk. The existing tmux-native path already
  solved an adjacent version of this — `TerminalStreamManager`'s write-lock
  ("Write-lock: prevents live output from being written while
  initial/paged history is loading... queue live output to avoid
  interleaving history with live data," `TerminalStreamManager.ts:144-145,
  256-266`) queues live writes during a paged-history load and flushes them
  after, in order. The forwarded-scroll case is harder because "the app's own
  redraw" *is* the live output channel — there's no separate "history batch"
  to distinguish from "live update" the way there is for tmux capture-pane
  history. UX-wise, the safest default is: while a forward-and-capture is
  in flight, suppress/hold any interleaved live redraw from reaching the
  visible viewport (mirroring the existing write-lock's intent) and make the
  in-flight state visibly distinct (a subtle loading indicator near the top,
  per the `aria-live` pattern in §3) so a user who scrolls back down mid-fetch
  isn't confused by a viewport that changed out from under them for a reason
  unrelated to their own gesture.
- If the two truly can't be disentangled (per the Rabbit Hole's own framing),
  the fallback UX should bias toward *not* forwarding while output is
  actively streaming (agent still "running"/"thinking") and only offering
  forwarded-scroll once the session is idle — reduces the collision surface
  substantially and matches the existing `detection.StatusIdle` gate this
  codebase already uses elsewhere for "safe to act unattended" decisions
  (per this session's MEMORY.md: "gate unattended PTY writes on StatusIdle +
  StatusContext allowlist instead").

### (d) Mobile touch-scroll vs. desktop wheel/trackpad
Yes, this differs meaningfully, and the existing mobile touch pipeline
constrains the design:
- Mobile scroll is **not** a native browser scroll event bubbling up
  naturally — it's the product's own custom scrollbar-track/thumb-drag
  implementation (`XtermTerminal.tsx:894-1000`) plus a separate touch-drag
  *selection* handler that shares the same `touchstart`/`touchmove` surface
  (`XtermTerminal.tsx:814-886`). The comment at `XtermTerminal.tsx:257-258`
  is a direct warning against this feature's most tempting implementation
  path: adding a *third* touchmove-based gesture handler ("having both
  register touchmove caused double-scroll and prevented selection" was
  already an incident here). Any mobile scroll-forwarding trigger should hook
  into the *existing* scroll-position signal (the same `onScroll`/`viewportY`
  check `TerminalOutput.tsx:1047-1058` already uses, which is DOM-scroll-based
  and therefore already fires correctly for both wheel and the custom
  touch-drag thumb, since both ultimately move `.xterm-viewport`'s scroll
  position) rather than adding a new touch-event listener.
- Practically, this means desktop and mobile *can* share one trigger
  (scroll-position-based, not gesture-based), which is good news for scope —
  but the *reached-limit feedback* (§4a/b) needs distinct treatment: a
  desktop user gets a stalled wheel with no visual object to point to, while
  a mobile user's custom scrollbar thumb (`scrollThumbRef`) is a visible,
  known DOM element that could show a bounce/rubber-band-style "you've hit
  the end" micro-animation, consistent with native mobile scroll
  conventions — worth flagging to the design/plan phase as a
  mobile-specific affordance opportunity, not a requirement.

## 5. Jobs-to-be-done: "reviewing an agent's past output"

**Functional job**: Verify what the agent actually did and why, without
re-running it — confirm a specific file was touched, a specific command was
run, a specific decision point was reached, or trace back to *when* something
went wrong. This is fundamentally an audit/verification job, not a passive
reading job — users scroll up with a specific question in mind ("did it run
the migration before or after the schema change?"), which is why silent
failure (§0, §2) is especially costly here: an unanswerable audit question is
a trust failure, not just an annoyance.

**Emotional job**: Reduce anxiety about autonomous/unattended work. This
product explicitly runs agent sessions unattended (backlog automation,
`STAPLER_SQUAD_INSTANCE`-isolated sessions, the whole "software factory"
framing referenced in this session's `backlog-feature-improvement` skill
description). A user scrolling into an agent's past output after stepping
away is looking for reassurance — "did it stay on track while I wasn't
watching" — and a broken/silent scrollback at exactly that moment
undermines the core value proposition of delegating work unattended in the
first place. This elevates the feature's priority beyond "nice-to-have
pagination polish": it's load-bearing for the product's trust story.

**Social job**: Produce a reviewable record to show or explain to someone
else — a teammate asking "how did the agent land on this approach," or
justifying a shipped PR's provenance. This is a weaker signal in this
specific product (no direct evidence found of a "share this session
transcript" feature in the grepped surface), but is consistent with the
broader product's PR/backlog-item framing (sessions link to PRs, backlog
items, and review verdicts per `docs/reference/*`) — the transcript is part
of the audit trail for a change, not just a live debugging aid, so
reliably retrievable history has value beyond the original user's own
session.

## Sources

- Direct code reading (all citations above are `file:line` within this
  repository at the current worktree HEAD; no external URLs were fetched —
  this research was scoped to codebase-grounded findings per the assignment
  plus general terminal-emulator/pager product knowledge for §1, which is
  documented common knowledge (tmux copy-mode, `less`/pager alt-screen
  behavior, iTerm2's alt-screen scrollback suppression) rather than a single
  citable source; no vendor doc URL was fetched to keep this pass scoped to
  the codebase per the task's tool-use guidance).
