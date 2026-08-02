# UX Design: Drop-and-Signal Badge for Dropped Input During Reconnect

Project: `phantom-keystroke-replay` · Backlog item `04089969-0f19-499c-be34-2e8bcfc4f13e`

Scope: the wireframes, interaction flows, and testable UX acceptance criteria
for the one user-facing surface this project ships — the `InputDropBadge` +
assertive announcement pair described in `research/ux.md` and scoped by
`implementation/plan.md` Phase 4 (Epic 4.1/4.2, tasks 4.1.1.1–4.2.3.2). This
document does not re-derive the UX research (see `research/ux.md` for the
comparable-products survey, mental-model analysis, and a11y rationale) — it
turns that research into wireframes, flows, and acceptance criteria a human
reviewer can check off against the shipped component.

Component inventory this design covers, matching the plan's realized names:

- `InputDropBadge.tsx` / `InputDropBadge.css.ts` — visible pill, mounted in
  `TerminalOutput.tsx` alongside the existing `reconnectingBanner`/
  `hardFailedBanner` overlays.
- `LiveRegion` (extended with a `role` prop) — the assertive announcement,
  driven by `droppedInputEvent` from `useTerminalStream`.
- `ConnectionIndicator.tsx` — **not modified**, referenced only as the
  existing "connection restored" signal this design deliberately reuses
  rather than duplicates (Pattern Decisions table, `implementation/plan.md`).

---

## Step 1 — User-facing surfaces

Four surfaces fully describe the feature's visible/audible behavior:

| # | Surface | Trigger | Owning state |
|---|---|---|---|
| A | Badge appears | `droppedInputEvent` transitions `null` → `{count, at}` | `useTerminalStream.droppedInputEvent` |
| B | Badge auto-dismisses | Dwell timer (4000ms) elapses with no new drop | Local timer inside `InputDropBadge` |
| C | Coalesced re-announcement during flapping | A second (or Nth) drop lands while the badge is still showing | Same `droppedInputEvent`, new `at`; badge's running-count ref |
| D | Default "connection restored, nothing lost" state | Reconnect succeeds with an empty queue (no drop occurred) | Absence of `droppedInputEvent` change — `ConnectionIndicator`'s existing "Connection restored" owns this, `InputDropBadge` stays unmounted/`null` |

Surface D is a **non-event** by design — its acceptance criteria are about
what must *not* happen (no badge, no duplicate announcement) as much as what
must.

---

## Step 2 — Wireframes and interaction flows

### Surface A — Badge appears (first drop in an episode)

**Trigger:** `useTerminalStream`'s `connect()` closes a superseded
`MessageQueue` with `droppedCount > 0` (Task 3.1.1.2/4.1.1.1), or
`useTerminalFlowControl.sendInput`'s early-return fires `onDrop()` (Task
4.1.1.2) — either path sets `droppedInputEvent = { count: N, at: timestamp }`.

```
┌─────────────────────────────────────────────────────────┐
│  TerminalOutput  (styles.terminal)                       │
│                                                            │
│  ┌──────────────────────────────────────────────────┐    │
│  │  ⊘  1 keystroke not sent — connection interrupted │◄───┼── InputDropBadge
│  │     (small pill, top edge of terminal chrome,      │    │   (visible)
│  │      warning-severity color + icon, not color-only)│    │
│  └──────────────────────────────────────────────────┘    │
│                                                            │
│  $ git commit -m "wip▊                                    │
│  ~                                                        │
│  ~                                                        │
│  ~                                                        │
└─────────────────────────────────────────────────────────┘
  ^ terminal keeps input focus throughout — badge never steals it

[off-screen, srOnly]
<div role="alert" aria-live="assertive" aria-atomic="true">
  1 keystroke not sent — connection interrupted
</div>
```

**Flow:**
1. Connection is superseded (reconnect) or a keystroke is typed while
   already-known-disconnected.
2. `droppedInputEvent` updates in the hook; `TerminalOutput` re-renders.
3. `InputDropBadge` mounts (was `null`), renders the pill inside
   `styles.terminal`, `aria-hidden` icon + visible text, no color-only cue.
4. `InputDropBadge` itself — not `TerminalOutput` and not a hook-level
   effect — owns the call to `announce(...)` on the `LiveRegion`
   (`politeness="assertive"`, `role="alert"`) as part of its own coalescing
   logic (Task 4.2.1.1/4.2.3.2): the component's internal effect keyed on
   `droppedInputEvent.at` fires the announcement once per distinct `at`
   while it's also updating the running-count ref and (re)starting the
   dwell timer. `TerminalOutput.tsx` carries no `useLiveRegion()`/`announce()`
   responsibility of its own — that was deliberately removed from it during
   plan repair to eliminate a per-event announcement-spam bug; see the
   Cross-reference table below.
5. Focus remains wherever it was (the terminal); badge has no `autoFocus`,
   no `tabIndex`.
6. Badge starts its 4000ms dwell timer (Surface B).

**What the user sees/hears:** a small pill appears near the terminal with an
icon + short text; a screen reader interrupts whatever it was announcing to
read the same message once.

**Dismissal:** none available or needed at this stage — it resolves via
Surface B (timeout) or Surface C (superseded by a new drop, timer resets).

---

### Surface B — Badge auto-dismisses

**Trigger:** 4000ms elapse since the badge's dwell timer was last (re)started
with no intervening drop.

```
t=0ms      badge appears, dwell timer starts (4000ms)
t=4000ms   ┌──────────────────────────────┐        (nothing — badge gone)
           │  (no drop in the interim)     │   →    $ git commit -m "wip▊
           └──────────────────────────────┘
```

**Flow:**
1. `InputDropBadge` starts a `setTimeout` on mount / on each new
   `droppedInputEvent.at`.
2. If the timer fires without being cleared/reset by a newer drop, the
   component clears its local "currently showing" state and renders `null`.
3. No announcement accompanies the dismissal — silence is correct here (the
   *drop* was the newsworthy event; its fade is not, mirroring
   `ConnectionIndicator`'s "announce on state transition, not on every
   tick" discipline).
4. No focus change — nothing was focused to begin with (Surface A never
   stole focus).

**Edge case:** if the component unmounts (e.g. the terminal itself closes)
before the timer fires, the timer must be cleared in a cleanup function —
otherwise a `setState` call on an unmounted component (React warning /
potential leak). This is an implementation-detail acceptance criterion, not
just a nice-to-have (see AC-VIS-4 below).

---

### Surface C — Coalesced announcement during a flapping episode

**Trigger:** the connection flaps repeatedly (the ticket's own repro:
connect → "stopped" → reconnect on a ~2s poll), producing multiple drops in
quick succession — the exact scenario `research/ux.md` §4 identifies as the
"spam risk."

```
Flapping timeline (connect → stopped → reconnect, repeating):

t=0ms     drop #1 (count=1)   → badge: "1 keystroke not sent — connection interrupted"
                                 announced (assertive, once)
                                 dwell timer starts → would expire t=4000ms

t=800ms   drop #2 (count=+2)  → badge updates IN PLACE: "3 keystrokes not sent — connection interrupted"
                                 (running total, not a second stacked badge)
                                 announced again (content changed → new announcement,
                                 not suppressed by AT as a duplicate)
                                 dwell timer RESETS → now expires t=4800ms

t=1600ms  drop #3 (count=+1)  → badge updates: "4 keystrokes not sent — connection interrupted"
                                 announced again
                                 dwell timer RESETS → now expires t=5600ms

t=5600ms  (no further drops)  → badge fades, no announcement
                                 ConnectionIndicator later announces
                                 "Connection restored" independently — no
                                 duplicate "all clear" from InputDropBadge
```

**Visual — single badge instance, count increments, never stacks:**
```
┌──────────────────────────────────────────────────┐
│  ⊘  4 keystrokes not sent — connection interrupted │   ← ONE pill, count updates
└──────────────────────────────────────────────────┘
```
Never:
```
┌───────────────────┐
│ ⊘ 1 keystroke...   │   ← WRONG: stacked badges per drop
├───────────────────┤
│ ⊘ 2 keystrokes...  │
├───────────────────┤
│ ⊘ 1 keystroke...   │
└───────────────────┘
```

**Flow:**
1. First drop in an episode: badge mounts, count = N₁, dwell timer starts,
   one announcement fires.
2. Each subsequent drop *while the badge is still visible* (i.e. before its
   dwell timer has expired): running total increments (`count = count +
   Nᵢ`), the pill's text updates in place, the dwell timer resets to a fresh
   4000ms, and a **new** announcement fires with the updated count string.
   Because the announced string's content changes (`"1 keystroke..."` →
   `"3 keystrokes..."` → `"4 keystrokes..."`), it is not suppressed as a
   duplicate by assistive tech that dedupes identical consecutive
   announcements — this is the `aria-atomic="true"` + changing-content
   pattern research/ux.md §3 calls out.
3. If a drop lands *after* the badge has already fully faded (a gap longer
   than the dwell window since the last drop), it starts a **new** episode:
   count resets to that drop's own count, not cumulative with the prior
   (already-resolved) episode.
4. No more than one assertive announcement fires per distinct
   `droppedInputEvent.at` — the coalescing window is "however long the badge
   is continuously visible," not a separate fixed debounce timer, which
   keeps the implementation simple (one timer, not two) while still
   satisfying "don't spam" (an assertive interruption at most once per
   ~800ms–4s of active flapping, not once per dropped byte).

**What the user sees/hears:** one pill whose count climbs while the flap is
ongoing; a screen reader hears a handful of updated interruptions (one per
drop event, not one per dropped character/chunk), each with the current
total, never a wall of identical repeated announcements.

**Residual risk — noisy badge under frequent benign reconnect blips:** a
session experiencing frequent short, benign reconnects (e.g. flaky wifi that
auto-recovers in well under a second) could show `InputDropBadge` repeatedly.
Each firing is a real (true-positive) drop — `count` is legitimately small,
often `1`, because only a single in-flight keystroke happened to be buffered
at the exact instant the connection was superseded — but a user seeing the
badge fire on every one of many quick, self-healing blips may experience it
as noisy or alarmist relative to how inconsequential each individual drop
felt (a single character, instantly retypeable). This is an **accepted
tradeoff**, not a defect: correctness over silence — a false-negative silent
loss (the pre-fix behavior) is worse than an occasional true-positive badge,
and Surface D's "silent by default" guarantee only holds when nothing was
actually dropped, so the badge cannot be suppressed here without
reintroducing silent loss. This is flagged as something to watch in real
production usage, not something this plan needs to solve now — e.g. a "don't
show for N seconds after the last dismiss" suppression window would trade
correctness for quiet and would be scope creep beyond this ticket's bug-fix
mandate (`requirements.md`'s Non-Goals already excludes general
reconnect/re-render stability work beyond stopping input replay/loss). If
this turns out to be a real annoyance in practice, it's a follow-up UX
ticket, not a blocker for shipping this one.

---

### Surface D — Connection restored, nothing lost (default / no-badge state)

**Trigger:** the common case — a reconnect completes with an empty
`MessageQueue` (nothing was buffered when the old connection was
superseded), or no disconnect happens at all during a session.

```
┌─────────────────────────────────────────────────────────┐
│  TerminalOutput  (styles.terminal)                       │
│                                                            │
│                                                            │
│  $ git commit -m "wip"                                    │
│  [main abc1234] wip                                        │
│  $ ▊                                                       │
│  ~                                                        │
│  ~                                                        │
└─────────────────────────────────────────────────────────┘
   ^ no InputDropBadge rendered — component returns null, not hidden via CSS

Header (unrelated component, already exists):
┌───────────────────────────────┐
│  ● Connected                  │  ← ConnectionIndicator, unchanged by this work
└───────────────────────────────┘
```

**Flow:**
1. Reconnect succeeds; `droppedInputEvent` is never set (stays `null`) because
   `close()` returned `0` and `onDrop()` never fired.
2. `InputDropBadge` renders `null` — not present in the DOM at all (not a
   CSS-hidden element sitting inert), consistent with `GitHubBadge.tsx`'s
   `return null` idiom this component's plan explicitly follows.
3. `ConnectionIndicator` (pre-existing, out of this project's scope) handles
   its own "Connection restored" polite announcement on the
   disconnected→connected transition — `InputDropBadge`/`LiveRegion` from
   this project fire nothing.
4. The user's mental model is correctly reinforced: no signal = nothing was
   lost. This is the silent case, and it's silent *only* because it's true —
   the whole point of Surfaces A–C is that silence is reserved exclusively
   for this state.

**Edge case:** a reconnect that *does* drop input must never coincidentally
land in this silent path — this is why Task 3.1.1.2's close-before-install
and Task 4.1.1.2's flow-control `onDrop` wiring both funnel into the same
`droppedInputEvent` state rather than only covering one of the two silent-
drop code paths identified in `research/ux.md`/Unresolved Question 1.

---

## Step 3 — UX acceptance criteria

Each item below is phrased so a human can check it off by looking at the
running app (or, where noted, reading a specific test assertion) — no code
familiarity required beyond what's written here.

### Visual

- **AC-VIS-1 (placement):** The badge renders inside the terminal panel's own
  chrome (anchored near the top of `TerminalOutput`'s terminal area,
  alongside the existing reconnect/hard-failure banners) — never as a
  page-level corner toast, never as a modal overlay that covers terminal
  text.
- **AC-VIS-2 (contrast/no color-only signal):** The badge pairs a
  non-decorative icon (`aria-hidden="true"`, e.g. a circle-slash/no-entry
  glyph) with visible text stating the count and reason (e.g. "1 keystroke
  not sent — connection interrupted"). Removing color entirely (e.g.
  grayscale simulation) must not make the badge's meaning ambiguous — verify
  by toggling OS-level grayscale/high-contrast mode and confirming the icon
  + text are still legible and distinguishable from surrounding terminal
  chrome.
- **AC-VIS-3 (auto-dismiss timing):** With no further drops, the badge
  disappears automatically within 4 seconds (± normal timer jitter) of the
  most recent drop — never requires a manual dismiss action, and never
  persists indefinitely after the flap has ended.
- **AC-VIS-4 (no stacking):** During a flapping episode with 3+ drops in
  quick succession, exactly one badge instance is ever visible at a time —
  its count updates in place; duplicate/stacked pills never appear
  simultaneously.
- **AC-VIS-5 (silent default):** With no dropped input during a session
  (including a session that never disconnects, and one that reconnects
  cleanly with nothing queued), the badge is never present in the DOM.

### Screen reader

- **AC-SR-1 (assertive, not polite):** The announcing element uses
  `aria-live="assertive"` and `role="alert"` — verify via DOM inspection
  (browser devtools accessibility tree) that both attributes are present
  together on the same element whenever a drop has occurred, and that
  `aria-atomic="true"` is also present so the full message (not a diff) is
  re-announced.
- **AC-SR-2 (coalesced wording):** A single drop announces exact count and
  cause, e.g. `"1 keystroke not sent — connection interrupted"` (singular
  wording for count = 1); multiple coalesced drops announce the **running
  total**, e.g. `"4 keystrokes not sent — connection interrupted"` (plural),
  never a raw list of per-event announcements and never a stale count that
  fails to reflect the latest drop in the same episode.
- **AC-SR-3 (no spam):** During a flapping episode producing many
  drops in rapid succession (sub-second apart), the number of distinct
  assertive announcements fired is bounded by the number of times the badge's
  content actually changed (one per new `droppedInputEvent.at` while the
  badge is visible) — not one per dropped byte/chunk, and not a continuous
  stream that would make the terminal's other output unlistenable via screen
  reader.
- **AC-SR-4 (no duplicate "all clear"):** Reconnection success announces
  exactly once, via the pre-existing `ConnectionIndicator` "Connection
  restored" transition announcement — `InputDropBadge`/its `LiveRegion` never
  fires a second, redundant "all clear" announcement of its own.

### No dead ends

- **AC-RESOLVE-1 (always resolves):** Every badge that appears is guaranteed
  to reach one of exactly two end states — auto-dismiss (Surface B, no
  further drops) or superseded-in-place by a newer drop within the same
  episode (Surface C, count updates and timer resets) — and from a
  superseded state, still eventually reaches auto-dismiss once drops stop.
  There is no third state where the badge remains visible indefinitely with
  no active timer.
- **AC-RESOLVE-2 (unmount safety):** If the terminal/component unmounts while
  a badge's dwell timer is pending (e.g. the user navigates away from the
  session mid-flap), the timer is cleared in cleanup — no state update fires
  against an unmounted component, and no orphaned badge reappears if the
  component later remounts for the same session.
- **AC-RESOLVE-3 (survives page-level reconnect noise):** A badge shown for
  one flapping episode does not persist across a full page reload / new
  `TerminalOutput` mount — component state (including any running count) is
  local to the mounted instance and starts clean on remount, matching
  Surface D's default.

### Keyboard / focus

- **AC-KBD-1 (no focus theft on appear):** When the badge appears
  (Surface A) while the user is actively typing in the terminal (terminal
  element has focus, e.g. mid-keystroke on the next command), focus remains
  on the terminal's input element after the badge renders — verify by
  checking `document.activeElement` immediately before and after a
  drop-triggered render; it must be unchanged.
- **AC-KBD-2 (not in tab order):** The badge is not a focusable element by
  default — no `tabIndex="0"` or implicit focusability — a user tabbing
  through the page does not land on the badge as an interactive stop, since
  it carries no action (unlike `NotificationToast`'s undo button, which
  intentionally is focusable).
- **AC-KBD-3 (no focus theft on update or dismiss):** Neither an in-place
  count update (Surface C) nor an auto-dismiss (Surface B) moves focus,
  scrolls the viewport, or otherwise disrupts the user's active typing
  location in the terminal.

---

## Cross-reference to implementation plan

| Design surface | Plan task(s) |
|---|---|
| A — Badge appears | 4.1.1.1, 4.1.1.2, 4.2.1.1, 4.2.2.2 |
| B — Auto-dismiss | 4.2.1.1 (dwell timer, 4000ms per Unresolved Question 4) |
| C — Coalesced announcement | 4.2.1.1 (running-total ref, dwell-timer reset, and the `announce()` call — all owned by `InputDropBadge` itself), 4.2.3.2 (tests) — **not** 4.2.2.2: that task's post-repair scope removed all `useLiveRegion()`/`announce()` responsibility from `TerminalOutput.tsx`; `InputDropBadge` owns 100% of coalescing and announcement |
| D — Silent default | 4.2.1.1 (`return null` idiom), Pattern Decisions (`ConnectionIndicator` left untouched) |
| AC-SR-1/AC-SR-2 | 4.2.2.1 (`LiveRegion` `role` prop), 4.2.3.2 |
| AC-KBD-1..3 | 4.2.1.1 ("No focus-stealing" bullet), 4.2.3.1 (`does not set focus/tabIndex on mount` test) |
| AC-RESOLVE-2 | 4.2.1.1 (`InputDropBadge` implements timer cleanup in its `useEffect` return function), 4.2.3.1 (test) — both tasks explicitly cite "ux.md AC-RESOLVE-2" |
