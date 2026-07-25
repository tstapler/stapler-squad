# UX Research: Terminal visibility/focus resync

Research for backlog item `7728f6df-268a-4578-9066-c300ff69269b` (issue #154). See
`../requirements.md` for the technical scope. This doc answers five UX questions
about the resync-on-focus path: comparable patterns, whether to show any
transient affordance during the ~4s watchdog window, accessibility/ARIA-live
reuse, error-state messaging if the watchdog fires, and the underlying
job-to-be-done.

## 1. Comparable UX patterns in other terminal/remote products

Surveyed: VS Code Remote-SSH/Codespaces, xterm.js-based web terminals
(ttyd/GoTTY), tmux+mosh over SSH, Jupyter/JupyterLab kernel websockets, and
general reconnecting-websocket UX guidance. Consistent findings:

- **Silent resync-while-still-connected is not really a named, documented
  pattern anywhere** — because most of these products don't have this
  failure mode in the first place. tmux/mosh redraw the full screen from
  server-side state on every SSH/mosh resume by design (mosh explicitly
  resends the terminal init sequence on client resume so a corrupted
  terminal is put back in a "reasonable state"); there's no separate
  client-side buffer to desync because the terminal emulator *is* the
  source of truth on the far end. Stapler Squad's bug is specific to having
  a **client-side xterm.js buffer that must stay in sync with a
  server-side tmux pane over a control-mode delta stream that coalesces
  while backgrounded** — closer to a CRDT/sync-engine problem than a
  classic terminal-emulator problem.
- **What *is* universal: distinguish "still alive, just refreshing" from
  "actually disconnected," and only surface UI for the latter.** VS Code
  Remote-SSH's own bug history is instructive: users are frustrated
  precisely when the tool conflates "reconnect" (fast, invisible, resumes
  in place) with "reload window" (slow, visible, loses state) — see
  microsoft/vscode-remote-release#5875 and #10122. The complaint pattern is
  "why did this show me a disruptive prompt for something that should have
  self-healed silently." That is the strongest argument in this research
  for keeping the success path silent and reserving visible UI for the
  genuine-disconnect case.
- **ttyd/GoTTY** (closest architectural cousin — also xterm.js + WS +
  server-side PTY) keep reconnection fully automatic and mostly silent;
  ttyd's only visible affordance is a `--reconnect <seconds>` retry timer,
  and full-buffer replay from the server on reconnect is the norm, not a
  spinner. No toast/flash is standard for the successful case.
- **Jupyter/JupyterLab**: minimizing a tab pauses ping/pong; on refocus the
  websocket is torn down and re-established. Community complaints
  (jupyterhub discourse, jupyterlab#8432) are about the UI going stale
  *silently* with no indication a reconnect happened and no forced kernel
  state refresh — the opposite failure mode from a false-positive spinner.
  This supports pairing silence with an a11y-only announcement (see §3)
  rather than pure silence with zero signal anywhere.
- **General WebSocket reconnection guidance** converges on: show a
  connection-status affordance keyed to *actual* disconnect state (banner,
  toast, or persistent indicator), never fabricate one for an operation the
  user didn't initiate and that is expected to succeed within a short
  window.
- **Nielsen's Heuristic #1 (Visibility of System Status)** is the
  standard counter-argument for going fully silent: "when users cannot
  perceive system state, they experience anxiety... silent failures leave
  users unaware problems occurred." This is the crux of question 2 below —
  it argues for *some* signal if the operation can silently stall, but does
  not argue for a signal on the fast/successful path.

**Synthesis**: the strong industry consensus is silent-and-fast for the
success path, with visible UI reserved for genuine failure/stall — not
"a transient affordance on every refocus." The instruction in the
requirements ("no loading spinner, no visible flash ideally, just a clean
repaint") matches this consensus, not a novel/risky choice.

## 2. Should the ~4000ms watchdog window show any indicator?

Recommendation: **stay silent for the first ~1.5–2s, matching the existing
`showReconnectBanner` 2s-after-disconnect precedent already in this codebase
(`TerminalOutput.tsx:735-761`), and reuse that exact banner mechanism if the
resync is still unresolved past that point — do not invent a new indicator.**

Reasoning, weighing both sides of the tradeoff called out in the question:

- **Cost of full silence for ~4s**: if a user refocuses the tab specifically
  *because* they want to act on a visible TUI prompt (the job-to-be-done in
  §5), and the resync silently stalls, the terminal looks frozen at exactly
  the moment they're trying to type. This is the scenario Nielsen's
  heuristic warns about, and it's a real risk given the watchdog exists
  *because* stalls are anticipated (why else have `RESYNC_STALL_TIMEOUT_MS`
  at all).
- **Cost of showing something immediately**: the success case (which should
  be the overwhelming majority — this bug is about *repainting*, not about
  *reconnecting*, and the resync RPC round-trip is normally well under
  100ms per the existing resize-triggered resync precedent already in
  production) would now flash a banner/spinner on effectively every
  tab-refocus, which is precisely the flicker the requirements doc says to
  avoid ("no loading spinner... ideally no visible flash").
- **Resolution**: this repo already encodes the right compromise for the
  *disconnected* case — `showReconnectBanner` fires only after a 2s delay,
  not immediately on disconnect, specifically so brief blips don't flash
  UI. Apply the identical debounce-before-reveal logic to the *resync*
  case: if `isResyncingRef`/`waitingForPaneResponseRef` (already tracked in
  `useTerminalFlowControl.ts:41-42`) is still true ~1.5-2s after
  `requestFullResync(true)` was called, surface the existing
  `reconnectingBanner` UI (`TerminalOutput.tsx:1600-1603`, styled as
  "Reconnecting terminal…") rather than a new resync-specific string. Two
  reasons to reuse rather than add new copy:
  1. It is genuinely true-ish — by the time 2s have passed with no pane
     response, the client doesn't actually know if the connection is still
     healthy; from the user's perspective "reconnecting" is an accurate
     description of "we're re-establishing a known-good terminal state,"
     whether that's a resync-in-flight or an actual socket reconnect.
  2. Introducing a second, differently-worded transient banner
     ("Refreshing terminal…" vs "Reconnecting terminal…") for what is
     externally indistinguishable to the user doubles the UI surface and
     the a11y announcement surface for no user-facing benefit — see §4 for
     why *not* differentiating is also the right call for messaging.
- If the watchdog *does* fire at 4000ms and forces `disconnect() →
  connect()`, that's now a real disconnect and the existing disconnect
  machinery (banner already showing since ~2s, `showReconnectButton`,
  status dot flipping to "Disconnected") takes over with zero new code
  needed — this is the elegant part of reusing the existing primitive: the
  visible state naturally upgrades from "maybe fine, watch and wait" to
  "actually reconnecting" without a special-cased transition.

Net: **partially silent** — 0–2s fully silent (covers the success case
invisibly), 2–4s shows the *existing* reconnecting banner as a "just in
case" signal (covers the stall-anxiety case), 4s+ is a real disconnect using
100%-existing UI. No new visible component is justified by this research.

## 3. Accessibility / ARIA live-region considerations

The repo already has a clear, consistent convention:
`role="status" aria-live="polite" aria-atomic="true"`, usually on a visually
hidden element via a shared pattern — there is even a dedicated reusable
component, `web-app/src/components/ui/LiveRegion.tsx`, plus one-off
`srOnly`-styled live regions in `SessionList.tsx` (`#bulk-feedback-live`),
`SessionCard.tsx` (`#creation-status-${session.id}`), and `NotificationToast.tsx`
(`aria-live="polite"`/`"assertive"` depending on urgency). `polite` is used
almost everywhere except genuinely urgent cases (approval-needed toast,
`InlineError`'s `role="alert"` for permanent errors) — `assertive` is
reserved for things that need to interrupt.

Gap found: the terminal's own `reconnectingBanner`
(`TerminalOutput.tsx:1600-1603`, "Reconnecting terminal…") and the
`hardFailedBanner` ("Connection lost — Retry") currently have **no
`aria-live` attribute at all** — they're plain `<div>`s. A screen-reader
user gets no announcement today when a real disconnect happens, only when
it later resolves via `handleManualReconnect`'s call path (which doesn't
announce either). This is a pre-existing gap, not something this feature
should silently inherit.

Recommendations for the new resync path:

- **Do not add a new visible UI element or a new ARIA live region.** Per
  §2, the resync path should reuse the existing `showReconnectBanner` /
  `reconnectingBanner` div rather than introduce new UI.
- **While in the repo, wire `aria-live="polite" aria-atomic="true"
  role="status"`** onto the existing `reconnectingBanner` div (and
  `role="alert" aria-live="assertive"` onto `hardFailedBanner`, matching
  `InlineError`'s convention for the same severity). This is in scope for
  this task specifically *because* the resync feature increases how often
  that banner fires (now also on refocus-stall, not just disconnect), so a
  screen-reader user hitting the resync-stall path deserves the same
  polite announcement a mouse/eye user gets visually. This is a small, safe
  addition to a pre-existing pattern, not new UI surface — confirm with
  whoever plans the implementation whether it's in-scope or a fast-follow,
  since it's not explicitly named in the requirements' "In scope" list.
- **The successful/silent resync path needs no announcement.** A screen
  reader user does not need to be told "terminal refreshed" every time they
  tab back in and the repaint succeeds invisibly in the common case — that
  would be a *worse* experience than sighted users get (over-announcement
  is a well-documented ARIA anti-pattern; live regions should announce
  state changes that matter, not routine internal housekeeping). Do not add
  a "terminal refreshed" toast/announcement for the happy path — this
  matches the "invisible in the success case" requirement across both
  visual and auditory modalities.
- **Never steal focus** (explicit requirement) is compatible with all of
  the above: `aria-live` regions announce without moving
  `document.activeElement` or requiring `tabindex`, unlike `role="alert"`
  used carelessly with `.focus()` calls elsewhere in some apps. Confirm the
  implementation doesn't call `.focus()` anywhere in the resync/banner
  code path (a quick grep of the eventual diff for `.focus()` calls near
  the new code is a cheap verification step).

## 4. Error-state messaging when the watchdog fires

Recommendation: **indistinguishable from the existing plain-disconnect UX —
do not add "we forced this" messaging.**

Reasoning:

- The user cannot tell the difference between "the socket actually died
  while backgrounded" and "the socket looked alive but the resync
  round-trip never completed" — both present identically from their
  vantage point (they refocused a tab; something looks off or slow). A
  message like "Refresh failed, reconnecting…" implies a *cause* the user
  has no way to verify or act on differently than a plain "Reconnecting
  terminal…" — it adds words without adding actionability, which cuts
  against this codebase's existing terse banner copy ("Reconnecting
  terminal…", "Connection lost — Retry").
- The forced path already reuses `disconnect().then(() => connect())` per
  the requirements (§ In scope, item 1) — the *mechanism* is identical to
  a normal reconnect, so the state machine driving `showReconnectBanner`,
  `showReconnectButton`, `connectionAttempts`, and the status dot all
  naturally produce the exact same visible sequence a plain disconnect
  produces. Special-casing the copy would require plumbing a "this
  disconnect was watchdog-forced" flag through that entire state machine
  for a distinction only engineers would find interesting.
- VS Code Remote-SSH's user complaints (§1) are a cautionary tale in the
  *opposite* direction: users get frustrated when tools surface
  *internal* distinctions ("reconnect" vs "reload window") as if they were
  meaningfully different user choices, when from the user's side both just
  mean "wait, then it works again." Keep the watchdog-forced path boring
  and identical to a plain disconnect; the interesting engineering
  distinction (resync-stall-triggered vs. socket-error-triggered) belongs
  in logs/telemetry, not UI copy.
- One nuance worth flagging to implementers: because the watchdog path
  starts from "still connected, just resyncing," the banner will appear to
  go connected → (silent 0-2s) → reconnecting-banner (2-4s) → disconnected
  → reconnecting, i.e. a *smoother* visible transition than a real abrupt
  socket-drop disconnect (which jumps straight from Connected to
  Disconnected with no warning). That's a strict UX improvement over
  today's abrupt-disconnect banner timing and doesn't need special
  messaging to convey — it falls out of reusing the existing 2s-delay
  banner logic in §2.

## 5. Job-to-be-done when refocusing a tab with an active TUI prompt

**The job**: "I backgrounded this to do something else; now I'm back to
make a decision/answer a prompt (e.g. Claude's option picker) and I need to
trust that what's on screen right now reflects reality before I press a
key." The user is not thinking about the terminal's connection machinery at
all — they're context-switching back into an interactive decision point.
Two failure modes matter here, and they cut in opposite directions on the
flicker-vs-correctness tradeoff:

- **Corrupted-but-static render (the bug being fixed)**: the screen shows
  stale/overlapping content and *nothing else happens* — no spinner, no
  indication it's wrong. This is the worst outcome for the job-to-be-done:
  the user can act on what looks like a valid prompt (e.g. press "2" for an
  option that isn't actually option 2 anymore, or press Enter mid-corrupted
  state) with real consequences (wrong branch checked out, wrong file
  edited, an agent given a wrong instruction). Correctness has to win here
  — this is exactly why the fix's core requirement (never move keyboard
  focus, but *do* force a clean repaint) is correct: the user must be
  looking at ground truth before they act, full stop.
- **A few hundred ms of visible flicker/redraw**: mildly annoying, costs
  a beat of visual continuity, but does **not** put the user at risk of
  acting on wrong information — a flash-then-correct-content sequence is
  self-evidently "something just refreshed," which if anything *increases*
  correctness-confidence (the user can visually confirm the redraw
  happened before reading the prompt). This is a strictly lower cost than
  silent corruption.

**Conclusion**: correctness dominates. If the implementation has to choose
between a repaint that's occasionally visually abrupt (e.g. a legitimate
`clearAndHome` flash when the timing genuinely lands mid-keystroke-window)
versus any risk of leaving stale content on screen when the user starts
interacting, choosing the former is correct and matches the requirements'
own framing ("no visible flash **ideally**" — a preference, not a hard
constraint, subordinate to AC0's "shows a clean terminal with no manual
action"). The debounce (~300ms) and the "resync exactly once per window"
requirement (AC1) already minimize flicker for the common case (single
focus event) without compromising this priority — they exist to avoid
*redundant* full resyncs, not to trade away correctness for smoothness. Do
not let a future optimization pass (e.g. trying to diff-and-patch instead
of clear-and-redraw to reduce flicker further) regress the guarantee that
by the time the user's next keystroke lands, the pane content is real.

## Sources

- [Workaround for reload Window · Issue #2045 · microsoft/vscode-remote-release](https://github.com/microsoft/vscode-remote-release/issues/2045)
- [Reconnect to Remote SSH at any time without reloading the window · Issue #10122 · microsoft/vscode-remote-release](https://github.com/microsoft/vscode-remote-release/issues/10122)
- [Automatically reload window after SSH disconnection · Issue #5875 · microsoft/vscode-remote-release](https://github.com/microsoft/vscode-remote-release/issues/5875)
- [ttyd - Share your terminal over the web](https://tsl0922.github.io/ttyd/)
- [GitHub - sorenisanerd/gotty: Share your terminal as a web application](https://github.com/sorenisanerd/gotty)
- [mosh-devel: client suspend](https://mailman.mit.edu/pipermail/mosh-devel/2012-April/000038.html)
- [UI does not update after kernel reconnection - JupyterHub - Jupyter Community Forum](https://discourse.jupyter.org/t/ui-does-not-update-after-kernel-reconnection/10559)
- [Reconnect a websocket when a kernel is restarted. by jasongrout · Pull Request #8432 · jupyterlab/jupyterlab](https://github.com/jupyterlab/jupyterlab/pull/8432)
- [Visibility of System Status (Usability Heuristic #1) — NN/g](https://www.nngroup.com/articles/visibility-system-status/)
- [ARIA live regions - MDN Web Docs](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Guides/Live_regions)
- [Replit — More Reliable Connections to Your Repls](https://blog.replit.com/eval)

## Codebase references used

- `web-app/src/components/sessions/TerminalOutput.tsx:735-761` — existing
  2s-delayed `showReconnectBanner` logic (the pattern to reuse for the
  resync-stall indicator).
- `web-app/src/components/sessions/TerminalOutput.tsx:1600-1609` —
  `reconnectingBanner` / `hardFailedBanner` JSX (no `aria-live` today).
- `web-app/src/lib/hooks/useTerminalFlowControl.ts:41-42,72-100` —
  `isResyncingRef`, `waitingForPaneResponseRef`, `requestFullResync`.
- `web-app/src/lib/hooks/useTerminalStream.ts:420-446` — existing
  `NEXT_PUBLIC_RECONNECT_V2`-gated visibility/online reconnect-when-disconnected
  listener (left as-is per requirements; the resync path is additive).
- `web-app/src/components/ui/LiveRegion.tsx` — reusable
  `role="status" aria-live aria-atomic` component, the established
  convention for a11y announcements in this codebase.
- `web-app/src/components/ui/NotificationToast.tsx:166` —
  `aria-live="polite"`/`"assertive"` toggle based on urgency, the
  precedent for reserving `assertive` for interrupt-worthy states.
- `web-app/src/components/backlog/InlineError.tsx:56,106` —
  `role="alert" aria-live="assertive"` convention for
  transient/timeout/permanent error severities.
