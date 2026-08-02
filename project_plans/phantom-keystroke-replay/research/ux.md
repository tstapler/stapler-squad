# Research: UX — drop-and-signal badge for dropped input during reconnect (Agent 5)

Project: `phantom-keystroke-replay` · Backlog item `04089969-0f19-499c-be34-2e8bcfc4f13e`

Scope: AC3/AC4 require that input typed while the terminal connection is
disconnected/superseded is **dropped, not queued**, and that the drop is
"visibly and audibly signaled" via a badge + assertive announcement. This
document covers the UX shape of that signal only — not the underlying
queue/epoch fix (that's Agent-1/2 territory in `session/git`/`web-app/src/lib`).

## 0. Codebase inventory (confirms the gap and the reusable pieces)

- **No existing component.** `web-app/src/components/sessions/` has no
  `InputDropBadge` or equivalent — confirmed by directory listing (79 files,
  none matching `*Drop*`/`*Discard*`).
- **`LiveRegion` (`web-app/src/components/ui/LiveRegion.tsx`)** is the
  existing accessible-announcement primitive: a visually-hidden (`srOnly`,
  `LiveRegion.css.ts`) `<div role="status" aria-live={politeness} aria-atomic="true">`
  plus a `useLiveRegion()` hook (`announce(msg)` sets state, auto-clears after
  1000ms via `setTimeout`). This is reusable as-is for the assertive
  announcement — pass `politeness="assertive"`.
- **`ConnectionIndicator` (`web-app/src/components/layout/ConnectionIndicator.tsx`)**
  is the closest prior art in this codebase for "connection state → visual +
  announced signal": it renders its own inline `aria-live="polite"` region
  (not reusing `LiveRegion`, a minor inconsistency worth normalizing on) that
  only fires `STATE_ANNOUNCE` text on a state **transition** (`prevStateRef`
  guard), not on every render — this coalescing pattern is exactly what's
  needed for a flapping episode (see §4).
- **`MemoryPressureCallout` and `NotificationToast`** show the two competing
  visual shapes already in use in this app: an inline dismissible `role="alert"`
  callout banner anchored in the session list, vs. a corner toast stack with
  `role="alert"` + conditional `aria-live` and auto-close/auto-minimize timers
  driven by `lib/notification-policy.ts`. Both are heavier than what's needed
  here (they carry action buttons, undo, dismiss-per-item state).
- **Input path**: `XtermTerminal.tsx`'s `terminal.onData` callback
  (line ~689) and its `onData` prop is the keystroke entry point;
  `useTerminalStream.ts` owns `connect()`/reconnect epoch state and
  `MessageQueue.ts` owns the buffer that AC3 says must drop rather than
  flush. The badge naturally lives in the terminal chrome that wraps
  `XtermTerminal` (`TerminalOutput.tsx`), not inside the xterm canvas itself.
- **CSS**: per `.claude/rules/css-architecture.md`, the new component must be
  `InputDropBadge.css.ts` (vanilla-extract), tokens from
  `web-app/src/styles/theme.css.ts`/`theme-contract.css.ts`, and any stacking
  must reference a **named** `zIndex` slot (`theme-contract.css.ts` lines
  195–214) rather than a magic number. Existing slots: `raised: 10`,
  `toast: 1080`. A badge anchored inside the terminal panel (not a page-level
  overlay) should use `zIndex.raised`; if it's promoted to a floating/portal
  toast instead, reuse `zIndex.toast` rather than inventing a new value.
  No existing slot is named for this purpose — do **not** hardcode a number.

## 1. Comparable UX patterns for "your input didn't go through"

Surveyed by product category (SSH/terminal multiplexers, chat, collaborative
editors) for the field's established idiom, since "input silently vanished
during a network blip" is a decades-old problem:

| Product class | Pattern on drop/loss | Placement | Persistence |
|---|---|---|---|
| **mosc/tmux/SSH terminals** (mosh, iTerm2, Windows Terminal) | Dim/gray overlay + status text ("Connecting...", "Reconnected") while disconnected; buffered local echo (mosh) shows keystrokes speculatively, then reconciles — but mosh **never silently drops**, it holds until delivery confirmed. No terminal product silently discards without any signal. | Full-pane overlay or corner status chip | Persists for duration of the outage, clears on reconnect |
| **Chat apps** (Slack, iMessage, Signal) | Per-message "failed to send" (red exclamation / "Not Delivered") badge **attached to the specific message**, tap to retry | Inline, adjacent to the affected content | Persists until user acts (retry/delete) — not auto-dismissed |
| **Collaborative editors** (Google Docs, Figma) | Small persistent connection-status chip ("Trying to reconnect…" / offline cloud icon) in the header; **edits are queued locally and replayed**, not dropped — different tradeoff than this project's explicit "drop, don't queue" requirement | Header/toolbar, low-key | Persists through outage |
| **This codebase's own `ConnectionIndicator`** | Small pill in the layout header, spinner + "Reconnecting…" text, ARIA live region announces only on transition | Header (always-visible chrome) | Persists while reconnecting |

Synthesis: the near-universal minimal treatment for "this specific thing you
did wasn't delivered" is a **small, transient, inline badge attached to the
affected surface** (not a full-screen modal, not a corner toast competing
with unrelated notifications) — chat apps' per-message failure badge is the
closest analog to "the input you just typed into the terminal didn't send."
Full-pane overlays (mosh/iTerm2-style) are the right pattern for *general*
connection state (which `ConnectionIndicator` already owns) but are the wrong
scale for a discrete drop event.

**Recommendation**: `InputDropBadge` should be a small, terminal-anchored
badge/pill — not a toast, not a modal — that appears at the moment of drop
and auto-fades after a few seconds (visually), while the screen-reader
announcement fires once per coalesced drop episode (see §4). This is
distinct from and complementary to `ConnectionIndicator`, which already
covers the ambient "are we connected" state; `InputDropBadge` covers the
much narrower, more urgent "something you typed just now was lost" event.

## 2. User mental model

A user typing into what visually looks like a live, responsive terminal
during a reconnect blip has exactly two mental models available, and the
product must resolve to one explicitly:

1. **"It's buffering — my keystrokes will show up once it's back."** This is
   the mental model every polished terminal/chat product trains (mosh,
   iMessage's optimistic send-then-reconcile, Google Docs). It's *not* the
   model this project is implementing (AC3 requires **drop, not queue**), so
   staying silent here actively confirms a false expectation.
2. **"It didn't go through — I need to retype it."** This is the correct
   model for the drop-and-signal design, but it is **not the default
   assumption** for a terminal — terminals have no history of "your keypress
   vanished," unlike form fields (which have long trained users to expect
   "Session expired, please log back in" banners) or chat (delivery
   receipts).

Because model (1) is what users bring by default and model (2) is what's
actually true, **silence is the single worst outcome**: the user believes
their `git commit -m "wip"` was buffered and will land once reconnected, sees
nothing to correct that belief, and only discovers the loss later when the
terminal doesn't reflect what they typed — at which point they've often lost
track of *what* they typed, compounding the trust damage. This is exactly
the requirements doc's framing (`## Remaining confirmed gap`) and is why AC3
demands an explicit signal rather than treating "just don't lose data
silently" as sufficient — the signal has to actively correct the wrong
mental model at the moment of the drop, not just log it somewhere.

## 3. Accessibility requirements

- **`aria-live="assertive"` is correct here**, not `polite`. WCAG 2.2 SC 4.1.3
  (Status Messages, AA) and the WAI-ARIA Authoring Practices distinguish:
  `polite` queues behind whatever the screen reader is currently doing;
  `assertive` interrupts immediately. A dropped keystroke is time-sensitive
  and actionable (the user needs to know *now*, before they type more into a
  void) — this matches the existing precedent in this codebase:
  `NotificationToast.tsx` already uses `aria-live={notificationType ===
  "approval_needed" ? "assertive" : "polite"}`, i.e. this codebase already
  reserves `assertive` for "requires the user's attention/action now,"
  which a dropped-input event qualifies for.
- **`role="alert"`** on the announcing element is the ARIA-APA-recommended
  pairing with `aria-live="assertive"` (an `alert` role has implicit
  `aria-live="assertive"` and `aria-atomic="true"` even without the
  attributes spelled out, but this codebase's existing components — both
  `NotificationToast` and `MemoryPressureCallout` — spell out `role="alert"
  aria-live=... aria-atomic="true"` explicitly rather than relying on the
  implicit mapping, for defensiveness across browser/AT combinations. Match
  that convention.
- **A visual-only badge is not sufficient** — this directly restates the
  acceptance criterion ("visibly *and audibly* signaled") and is also a WCAG
  1.1.1/4.1.3 requirement independent of the AC: any information conveyed
  only by a transient visual element that disappears is inaccessible to
  screen-reader users and typically also to anyone not looking at the
  terminal pane at that instant (e.g., glancing at a second monitor). The
  live-region announcement is not a nice-to-have alongside the badge; for
  screen-reader users it **is** the signal.
- **`aria-atomic="true"`** (used consistently by `LiveRegion`,
  `ConnectionIndicator`, `NotificationToast`, `MemoryPressureCallout` in this
  codebase already) ensures the whole message is re-announced rather than a
  diff, which matters once coalescing produces a count (`"3 keystrokes
  dropped"` — see §4) — without `aria-atomic`, some AT/browser combinations
  only announce the changed substring.
- **Do not rely on color alone** for the visual badge (WCAG 1.4.1) — pair a
  color cue with an icon/glyph and text label ("Not sent" / a dropped-input
  glyph), consistent with how `MemoryPressureCallout` pairs a `⚠` glyph with
  text rather than color-coding alone.
- **Focus behavior**: unlike `NotificationToast`'s undo toast (which moves
  focus to the Undo button per WCAG 2.4.3, since it demands an action), the
  drop badge should **not** steal focus — the user is actively typing in the
  terminal and focus must stay there. This is a case where the live-region
  announcement (heard, not focused) is preferable to a focus-stealing modal
  or toast — reinforcing the "small inline badge, not a takeover" call in §1.

## 4. Coalescing/debounce for flapping episodes

The ticket's own reproduction (`session_driver.go` polling every 2s through a
flapping connect → "stopped" → reconnect cycle) is precisely the scenario
that would otherwise spam repeated assertive announcements — and rapid
repeated `assertive` interruptions are themselves a known screen-reader UX
failure mode (each one cuts off the previous, and users lose the ability to
follow the terminal output entirely if it fires on every dropped byte/chunk).

Design requirement: **coalesce, don't spam.**

- **Visual badge**: one badge instance, not one per dropped chunk. If a new
  drop occurs while the badge is already showing/fading, reset its dwell
  timer and (if it carries a count) increment a counter rather than stacking
  duplicate badges. This mirrors `NotificationToast`'s existing
  auto-minimize/auto-close timer pattern (`effectiveAutoClose`,
  `effectiveAutoMinimize`, centralized in `lib/notification-policy.ts`) —
  that same debounce-timer idiom (reset-on-repeat rather than stack-on-repeat)
  should back the badge's dwell/fade timing, not a bespoke timer.
- **Announcement**: debounce/batch drops within a short window (a few hundred
  ms, tunable — same order of magnitude as `useLiveRegion`'s existing 1000ms
  auto-clear) into a single assertive announcement. If multiple drops land
  inside that window, announce once with a count: *"3 keystrokes not
  sent — connection interrupted"* rather than firing the live region three
  times. `ConnectionIndicator`'s transition-only announcement
  (`prevStateRef` guard, only announcing on `connectionState` *change*, not
  on every poll) is the direct precedent in this codebase for "don't
  re-announce steady-state noise" — the drop-badge's coalescing should follow
  the same shape: accumulate a count while in a "recently dropped" window,
  flush one announcement when the window closes (either on a timer or on the
  next successful reconnect), rather than one `announce()` call per dropped
  send.
- **Recovery signal**: once the connection stabilizes, the badge should clear
  and — arguably — a brief, low-key confirmation ("Reconnected" /
  `ConnectionIndicator` already renders "Connection restored" on the
  `connected` transition) is enough; no separate announcement is needed for
  "drops stopped happening," since `ConnectionIndicator` already announces
  the reconnect. Avoid stacking a second "all clear" announcement from the
  new component — reuse the existing reconnect signal, don't duplicate it.

## 5. Job-to-be-done

The functional job is narrow ("tell me my keystroke didn't send"), but the
job the user is actually hiring the terminal for is **trust that the tool
faithfully reflects and transmits what they type, especially under bad
network conditions** — this is the same trust category as the original bug
report itself (a phantom `1` appearing that the user *didn't* type is the
mirror-image failure of input the user *did* type vanishing). Both failure
modes — phantom input and silently dropped input — break the same
underlying promise: *what I type is what gets sent, once, faithfully.*

Silence-on-drop is the worst outcome specifically because it's
indistinguishable, from the user's seat, from the tool just being slow — the
user has no way to tell "buffering, will arrive" from "gone, retype it" without
an explicit signal, and will typically only find out days later that a
command they thought they ran never executed (worse for destructive/
one-shot commands than for a chat message, since there's often no visible
absence in the terminal to notice — the prompt just looks normal, waiting for
the *next* command). The badge + assertive announcement isn't decorative
polish on top of the queue/drop fix; it's the component that actually
delivers the "faithful and trustworthy" job, since the underlying drop
behavior (correct per AC3) is invisible without it.

## Recommendations summary for Agent 3/4 (design) and implementation

1. New component `InputDropBadge.tsx` + `InputDropBadge.css.ts` in
   `web-app/src/components/sessions/`, following the vanilla-extract +
   `theme.css.ts` token convention (no CSS Modules, no hardcoded z-index —
   add a named slot, e.g. `zIndex.inputDropBadge` or reuse `zIndex.raised`,
   to `theme-contract.css.ts` if the badge is chrome-anchored rather than a
   floating overlay).
2. Reuse `LiveRegion`/`useLiveRegion` (`web-app/src/components/ui/LiveRegion.tsx`)
   for the announcement rather than hand-rolling a second live-region
   pattern — pass `politeness="assertive"`, and add `role="alert"` +
   `aria-atomic="true"` explicitly per this codebase's existing convention
   (already default in `LiveRegion`, confirm `role="alert"` isn't needed
   there too — currently it's `role="status"`, which is `polite`-oriented by
   ARIA convention; for the assertive case here, prefer `role="alert"`
   matching `NotificationToast`/`MemoryPressureCallout`, so either extend
   `LiveRegion` to accept a `role` prop or compose a thin wrapper).
3. Visual placement: small inline badge/pill anchored to the terminal panel
   chrome (`TerminalOutput.tsx`), not a corner toast or modal — matches the
   "attached to the affected surface" pattern from chat apps' failed-message
   badges, and keeps it distinct from `ConnectionIndicator`'s header-level
   ambient status.
4. Debounce/coalesce: batch drops within a short window into one badge
   instance (reset-timer-on-repeat, like `NotificationToast`'s
   auto-minimize/auto-close policy) and one assertive announcement carrying
   a count, mirroring `ConnectionIndicator`'s transition-only announcement
   discipline. Do not fire one announcement per dropped chunk.
5. No focus stealing — the user is actively typing; the live region is heard,
   not focused, unlike `NotificationToast`'s undo-toast focus move.
6. On reconnect, clear the badge; rely on `ConnectionIndicator`'s existing
   "Connection restored" announcement rather than adding a second one.
