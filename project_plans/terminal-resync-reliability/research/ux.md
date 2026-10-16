# UX Research: terminal-resync-reliability

**Agent 5 — Phase 2 research.** Scope: user experience implications of scoping/staggering/
correlating resync traffic across multiple concurrently-mounted terminals.

## 0. Bottom line up front

This is almost entirely an invisible reliability fix, not a UI feature. The existing codebase
already has the one piece of user-facing surface this project can touch — the "reconnecting"
banner shipped in Story 2.1.8/3.2.1 (`web-app/src/components/sessions/TerminalOutput.tsx:1719-1727`,
wired up from `web-app/src/components/sessions/useVisibilityResync.ts`) — and the correct UX target
for every in-scope fix (1, 2, 4, 5 below) is to make that banner fire **less** than it does today,
not to add a new indicator. Item 3 (server-side capacity) has zero UI surface at all. I did not
find a genuine need for new UI; see §6 for the one gap worth flagging (accessibility on the
existing banner) and §7 for why I'm not inventing anything beyond that.

## 1. Comparable UX patterns in similar products

I looked at how other multi-pane/multiplexer terminal tools handle reconnect/resync feedback for
panes that are not currently focused.

- **tmux itself** — the closest architectural analogue (this product is a tmux front-end). tmux's
  own behavior is instructive precedent: background/non-attached windows within a session are
  *not* proactively redrawn when content changes — redraw is deferred until the pane becomes the
  attached/current one (or a manual `prefix+r` force-redraw / terminal resize occurs). This is
  itself a `tmux` GitHub-documented behavior/limitation ([tmux/tmux#2237](https://github.com/tmux/tmux/issues/2237)), and no visible "resyncing" indicator is shown for the deferred windows — the
  content is simply presented correctly once you switch to it. This is direct precedent for
  **silent background resync being an acceptable, expected mental model** in the exact product
  category stapler-squad sits in: users of tmux do not expect or want a spinner on windows they
  aren't looking at.
- **VS Code integrated terminal** — persists sessions across window reload/restart
  (`terminal.integrated.enablePersistentSessions`) and reconnects/restores scrollback silently on
  reload; the only visible signal is a small icon on the terminal's tab (not the pane itself),
  and only for semantically-relevant task state ("running"/"bell"), which extensions can suppress
  entirely for expected-long-running/background processes (`isBackground: true` in `tasks.json`).
  There's an open, unresolved feature request
  ([microsoft/vscode#121659](https://github.com/microsoft/vscode/issues/121659)) asking VS Code to
  better distinguish "still running" from "done but tab not closed" — i.e. even in a mature
  product, background-pane status signaling is an acknowledged UX gap, not a solved problem with
  an obvious answer to copy. Takeaway: tab-level (not pane-level) status icons are the norm, and
  no product in this space shows a modal/banner interrupt for a background pane's internal
  reconnect plumbing.
- **ttyd / GoTTY** (the closest browser-based analogues to this product's xterm.js-over-websocket
  architecture) — ttyd exposes a configurable client-side `--reconnect` retry timer but neither
  tool's documentation describes any special-cased background-tab behavior; reconnect is a
  connection-level concern handled uniformly regardless of visibility. Browser background-tab
  timer/network throttling is a known general problem for any websocket app (not specific to
  terminals), reinforcing that "reconnect churn is worse for backgrounded content" is a platform-
  level reality every one of these tools has to cope with, not something specific to this
  project's architecture.
- **Zellij / Warp** — no authoritative documentation found describing pane-level resync/reconnect
  indicators distinct from a whole-session connectivity state; I'm not asserting a specific pattern
  for these two beyond noting the absence of evidence for per-pane background indicators (see the
  confidence labels in §7 — this is a gap, not a finding).

**Precedent conclusion**: There is real precedent (tmux, VS Code) for *silent* background
resync with no visible indicator, and *no* precedent found for a "resyncing…" status badge shown
specifically for a non-focused pane. This supports Scope item 1 (only fire `requestFullResync` for
visible terminals) as the UX-correct default — background terminals should just work when you
switch to them, matching the tmux mental model this product's users already have.

## 2. User mental models

- **What the user expects when switching back to a backgrounded tab**: given tmux-trained user
  expectations (this product's entire audience is developers who use tmux — the tool's own
  premise), the expected experience on switching to a background terminal is "it just shows the
  current, correct state" — not a visible transition. A **brief, sub-second flicker/redraw** on
  tab-switch is within the tolerance band tmux itself has trained users to accept (see §1). A
  **visible disconnect+reconnect banner** ("Reconnecting terminal…", `TerminalOutput.tsx:1721`) on
  the other hand reads as "something went wrong" — it is literally the same UI the codebase already
  uses for real connection loss, so a resync-driven false-positive teaches the user to distrust a
  signal that's supposed to mean "the connection actually dropped." This is the direct UX
  articulation of the bug this whole project fixes: the banner is currently miscalibrated because
  it can't distinguish "resync taking >2s because it's contending for a shared exec-gate slot"
  from "we actually lost the connection."
- **Stagger-delay risk — does staggering make a background terminal look "frozen" or "behind"?**
  Yes, this is a genuine risk worth calling out for the design phase. If terminal B's resync is
  deliberately staggered behind terminal A's, and the user switches to B in that window, B is
  *by definition* newly-visible — at that moment it should immediately become the highest-priority
  resync in the queue (jump the queue), not simply wait its turn. If the design instead keeps B
  waiting on its original stagger slot after it becomes visible, the user will see genuinely stale
  content for the gap and reasonably read it as "broken," which is exactly the corruption
  perception the original visibility-resync feature (PR #184) was built to eliminate. Concretely:
  **the stagger/priority scheme must be visibility-driven, not just entry-order driven** — a
  terminal that becomes visible must always preempt a terminal that is merely queued while still
  backgrounded. This is a plan-phase requirement, not just a research note, so it's worth carrying
  into `research/technical.md` / `plan.md` if it isn't already covered by scope item 4
  ("stagger/prioritize resync bursts across multiple simultaneously-visible terminals" — note the
  scope text already says "simultaneously-visible," which is consistent with this constraint, but
  the *newly*-visible-preempts-still-backgrounded case should be made explicit).
- **Emotional read of the fix succeeding**: if this project fully succeeds, the user's experience
  of switching tabs among several live sessions should be indistinguishable from switching tabs in
  a single-session, no-multiplexer product — no perceptible cost to having many sessions open. The
  measurable proxy for "did we succeed emotionally" is exactly Success Metric 1 in requirements.md
  (stall-watchdog fires → near-zero for backgrounded terminals): every fire is a moment the tool
  broke that mental model.

## 3. Accessibility requirements (WCAG / ARIA / keyboard)

Scope items 1, 2, 4, 5 add no new UI, so there's no new accessibility surface to design for them.
Two things are worth recording:

- **Existing gap on the banner this project must not regress further**: I read the banner markup
  directly (`web-app/src/components/sessions/TerminalOutput.tsx:1719-1727`) —
  ```tsx
  {showReconnectBanner && !isHardFailed && (
    <div className={styles.reconnectingBanner}>Reconnecting terminal…</div>
  )}
  {showReconnectBanner && isHardFailed && (
    <div className={styles.hardFailedBanner}>Connection lost — <button onClick={handleHookReconnect}>Retry</button></div>
  )}
  ```
  Neither `<div>` carries `role="status"`/`role="alert"` or `aria-live`. `XtermTerminal.tsx:1262`
  does use `aria-live="polite"` elsewhere (on the terminal's live-region announcer for content
  changes — not on this banner). WCAG 4.1.3 (Status Messages, AA) requires that a status update
  like this be programmatically determinable without moving focus, i.e. it needs
  `role="status" aria-live="polite"` (or `role="alert"` for the hard-failure variant, since that
  one demands more urgent attention and has an actionable `Retry` button). **This gap already
  exists today and is orthogonal to this project's scope** — flagging it is not a request to fix
  it as part of this bug-fix project, but if scope item 1/2 changes *when* this banner fires (and
  it should fire less often after this project), it's a natural, very-low-cost time to add the
  missing `role`/`aria-live` attribute in the same diff, since the JSX is already being touched by
  necessity (the banner's trigger conditions are precisely what's changing). I'd recommend it as a
  small drive-by fix, not as new scope.
- **No new indicator, no new keyboard interaction**: since the correct design direction is "banner
  fires less, not a new/different banner," there's no new focus order, keyboard trap, or
  operable-interface requirement to design. The `Retry` button in the hard-failed banner is
  existing, unaffected functionality (Constraint: "must not regress the three pre-existing
  full-resync/refit triggers" — this banner path is downstream of the watchdog-fires-in-3, not
  those three, so it's in scope for regression testing but not for accessibility redesign).
- If the plan phase does end up wanting a queued/staggered-state affordance (see §4), it must
  follow the same `role="status" aria-live="polite"` pattern for consistency, and must not steal
  focus — the user may be actively typing in the visible terminal while a background one is queued.

## 4. Error states and edge cases

Working through "what should the user see" for each state a resync can be in, given the existing
`useVisibilityResync.ts` state machine (`RESYNC_DEBOUNCE_MS=300`, `RESYNC_BANNER_DELAY_MS=2000`,
`RESYNC_STALL_TIMEOUT_MS=4000`):

| State | Recommended user-visible behavior |
|---|---|
| Resync scoped-out because terminal isn't visible (scope item 1) | **Nothing.** No banner, no indicator. This is the success case — the whole point is these terminals no longer participate in the burst at all. |
| Resync staggered/queued behind another visible terminal's resync (scope item 4) | **Nothing, provided it resolves before the existing 2s banner threshold.** If a stagger scheme pushes a *visible* terminal's resync past ~2s, that terminal should fall into the existing banner path (it already exists for exactly this "still not done" signal) rather than inventing a separate "queued" indicator — see the "don't add new UI" framing in §0. If in practice staggering makes hitting 2s common for the 2nd/3rd visible terminal, that's a signal the stagger delay is too aggressive relative to `RESYNC_BANNER_DELAY_MS`, and the two should be tuned together, not treated as independent knobs. |
| Resync completes normally, correlation ID confirms completion (scope item 2) | **Nothing** — same as today's happy path. Precision here is a purely internal reliability win (no more heuristic-driven false stalls); it has no distinct visible state of its own. |
| Resync genuinely stalls past 4s (watchdog fires) | Same as today: existing "Reconnecting terminal…" banner, then the `--- reconnected ---` scrollback separator (`TerminalOutput.tsx:790-797`) once the forced disconnect+reconnect completes. This is correct, existing behavior for a real failure — the goal of this whole project is to make it fire less often for backgrounded/spurious cases, not to change what it looks like when it's genuinely warranted. |
| Feature flag off (fallback to today's behavior) | Unchanged — today's disconnect+reconnect-on-stall banner path, unmodified. This is the safe, tested default the flag rolls back to; no new UX to validate for the "off" state beyond confirming it's bit-for-bit identical to pre-project behavior (a good candidate for a regression test assertion, not just a doc note). |
| Unrecoverable error (e.g. exec-gate exhausted even on the fast lane, dimension resolution repeatedly fails) | Falls through to the existing `isHardFailed` banner (`Connection lost — Retry`, `TerminalOutput.tsx:1724-1727`) — reuse, don't reinvent. The user already has a manual escape hatch (the Retry button) for exactly this case. |

The unifying design principle across every row: **every state this project introduces maps onto
an existing visible state (nothing, the 2s banner, or the hard-failed banner) — none of them need
a new visual language.** That's a direct consequence of item 0's finding and worth protecting
during planning: a design that invents a fourth visible state (e.g., a "queued" spinner) adds
surface area, accessibility burden (§3), and test burden (E2E conventions require
`data-testid`/ARIA-role locators for anything new, `.claude/rules/e2e-test-conventions.md`) for a
case that the existing 2-state banner model already covers adequately once the timing is
right-sized.

## 5. Job-to-be-done lens

Framing: "trust that my backgrounded terminal sessions are still alive and correct without me
having to babysit them."

- **Functional job**: switch between N concurrent agent sessions and have each one's content be
  accurate the instant it becomes visible, with no manual action (no Retry click, no waiting for a
  visible spinner to resolve) required for the common case. Success metric 1 in requirements.md
  (stall-watchdog fires → near-zero for backgrounded terminals) is the direct functional proxy.
- **Emotional job**: the disconnect+reconnect banner today is a *trust-eroding* signal precisely
  because it fires on cases that are not real disconnects (per the Baseline section — "it's usually
  not the one I'm typing in that runs into trouble"). Every spurious banner the user sees on a
  terminal they weren't even touching teaches them the terminal state is unreliable, which
  generalizes badly: a user who's seen background terminals "randomly" reconnect starts manually
  re-checking terminals they didn't need to check, which is the opposite of "not babysitting."
  Fixing this restores the tool's credibility as a place to safely park multiple long-running
  agent sessions unattended — arguably the core value proposition of a session-management tool
  like stapler-squad in the first place.
- **Social/cognitive-load job**: for a user running several simultaneous Claude Code/Aider agent
  sessions (the stated primary use case per requirements.md's Users/Consumers section), each
  spurious background reconnect is a context-switch tax — a flashing banner or unexpected
  scrollback separator on a tab the user glances at is a "wait, did something break?" interruption
  that competes with the actual cognitive work of monitoring multiple agents. Reducing visible
  reconnect churn is directly in service of the "low cognitive load" half of the trust job: the
  fewer ambiguous signals competing for attention, the more of the user's attention budget goes to
  the agents' actual output rather than to policing the tool's own plumbing.

## 6. The one flagged gap (not new scope)

The reconnecting/hard-failed banners lack `role="status"`/`role="alert"` + `aria-live` (§3). This
predates this project and is not required by any of the five in-scope items, but since scope items
1–2 necessarily touch the code paths that decide *when* this banner renders, it's a low-cost
drive-by accessibility fix worth doing in the same PR rather than a separate one. Flagging for the
plan phase to make an explicit in/out-of-scope call rather than silently doing or silently skipping
it.

## 7. Confidence labels

- **VERIFIED** (read directly from this repo): existing banner markup/timing/state machine in
  `web-app/src/components/sessions/TerminalOutput.tsx` and
  `web-app/src/components/sessions/useVisibilityResync.ts`; `isVisible` prop already exists and is
  wired per-terminal from `web-app/src/components/sessions/SessionDetailView.tsx:741,760,868`
  (`isVisible={poolPath === session.externalMetadata?.muxSocketPath}`,
  `isVisible={poolId === session.id}`, `isVisible={activeTabId === shellKey}`) — meaning the
  primitive scope item 1 needs (a per-terminal "is this the one the user is looking at" signal) is
  already present in the codebase today, just not yet plumbed into the resync-trigger decision in
  `useVisibilityResync.ts` (which currently checks only `document.visibilityState`, not
  per-terminal `isVisible`); lack of `aria-live`/`role` on the banner divs (verified by reading the
  JSX, `XtermTerminal.tsx:1262` has `aria-live="polite"` elsewhere but not on this banner).
- **VERIFIED via web search** (see inline links in §1): tmux background-window redraw deferral
  ([tmux/tmux#2237](https://github.com/tmux/tmux/issues/2237)), VS Code persistent-session
  reconnect and tab-icon-only status signaling
  ([code.visualstudio.com/docs/terminal/advanced](https://code.visualstudio.com/docs/terminal/advanced),
  [microsoft/vscode#121659](https://github.com/microsoft/vscode/issues/121659)), ttyd's
  `--reconnect` flag and lack of background-tab-specific handling
  ([tsl0922/ttyd](https://github.com/tsl0922/ttyd)).
- **INFERRED**: the "newly-visible-must-preempt-still-queued" stagger constraint in §2 is my
  synthesis from the requirements' own stagger scope item plus the corruption-fix constraint — it
  isn't stated verbatim in requirements.md and should be confirmed with the plan-phase design
  rather than treated as settled.
- **UNVERIFIED / explicit gap**: I found no authoritative documentation describing Zellij's or
  Warp's specific background-pane resync/status UX; I'm surfacing the absence of evidence rather
  than guessing a pattern for either product.
