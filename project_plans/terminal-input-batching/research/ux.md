# UX Research: Terminal Input Batching

## 1. Settings-UI-gap — confirmed

No component under `web-app/src/components/settings/` renders `TerminalConfig` fields.
`grep -rn "loadTerminalConfig\|useTerminalConfig\|TerminalConfig"` across
`web-app/src` returns exactly four hits: `XtermTerminal.tsx` (the one production
consumer), two test files, and `terminalConfig.ts` itself. `useTerminalConfig()` (the
reactive hook meant for a settings panel) has zero callers anywhere.

`XtermTerminal.tsx:210` does `const config = useConfig ? loadTerminalConfig() : null;`
— a one-time synchronous read at mount, not the `useTerminalConfig()` hook. So even the
one place that *reads* `TerminalConfig` won't pick up a live change from a settings
panel without a remount; only a page reload (or the existing `terminal-config-changed`
custom event, which `XtermTerminal.tsx` does not subscribe to) would apply it today.

`web-app/src/app/settings/page.tsx` — the settings page — has four tabs: **General**,
**Config Files**, **Appearance**, **Keyboard Shortcuts**. `Appearance` currently holds
`ThemePicker` and `PushNotificationSettings` (`page.tsx:120-129`). There is no
"Terminal" tab or section anywhere. `Appearance` is the natural home for a new
`TerminalSettings` component (fontSize/cursorBlink/scrollback/mouseTracking/batching
all belong together, and none of it is theme-specific, but it's the closest existing
bucket) — or a new fifth tab if the set of terminal options grows enough to want its
own space. Either way, **building this settings surface is a separate, larger piece of
work than the single `inputBatchDelayMs` field** — it would be the first UI ever built
for the `TerminalConfig` object, not an incremental addition to an existing one.

## 2. Comparable UX patterns / friendliness of a raw ms dropdown

iTerm2, VS Code's integrated terminal, and Windows Terminal all bury WS/IPC-level
transport tuning (VS Code's terminal has no user-facing "batch renderer updates"
control at all — it's an internal constant; iTerm2's equivalent tuning, e.g.
`Unlimited scrollback`/GPU renderer toggles, sits under an "Advanced" tab, never in
the primary settings surface). None of the three exposes a raw millisecond dropdown
for something this deep in the transport path — the norm for this class of setting is
either (a) no user control at all (silently tuned by the app), or (b) a single
on/off toggle under an "Advanced"/"Experimental" section, not a 5-value technical
dropdown in a primary tab.

Given this repo's YAGNI stance (ponytail plugin instructions: cut unused abstractions,
prefer the smallest surface that satisfies the requirement) and that **the requirements
doc itself says this item has no attached user complaint** — "No user-reported
performance complaint is attached to this item — it originates from studying another
project's implementation, not from an observed stapler-squad problem" — a full
0/32/64/128/256ms dropdown is more surface than the evidence justifies. A simpler
on/off toggle ("Reduce network chatter for fast typing") with a single hardcoded
non-zero delay (e.g. 32ms, herdr-web's early-flush threshold) behind it would satisfy
the same functional need with one control instead of five options to explain and test.
**Recommendation: keep the 5-value enum in the underlying `TerminalConfig` type (cheap,
matches the acceptance criteria's explicit ask), but if/when a UI is built, expose only
a toggle in the UI layer** — map "on" to a single sane default (32ms) and "off" to 0.
This keeps the acceptance-criteria-mandated flexibility in the data model without
forcing an under-motivated 5-way choice onto a user who has no way to judge the
tradeoff. Flag this as a scope decision for whoever picks up the settings-UI followup,
not something to resolve unilaterally in the batching change itself.

## 3. User mental model / label wording

"Input batch delay" (or `inputBatchDelayMs` verbatim) will not mean anything to a
non-technical user — "batch" and "delay" both read as *slower*, which is the opposite
of the intended framing (fewer network messages, not slower input). If a UI control is
ever built:

- **Avoid**: "Input Batch Delay (ms)"
- **Prefer toggle label**: "Reduce typing network traffic" or "Optimize fast typing for
  low-bandwidth connections"
- **One-line help text**: "Batches rapid keystrokes into fewer network messages.
  Recommended if you're on a slow or metered connection; off by default because most
  users won't notice a difference."

If the 5-value dropdown ships instead of a toggle (e.g. because the settings-UI
follow-up decides the flexibility is worth it), label the field "Typing responsiveness"
with values framed as "Instant (default)" / "Balanced" / "Low bandwidth" rather than
raw millisecond counts — most users have no basis for choosing between 64ms and 128ms
as numbers, but can reason about a responsiveness-vs-bandwidth tradeoff in plain terms.

## 4. Accessibility

Risk is low for this control shape (a `<select>` or a radio group over 5 fixed
values), but the existing patterns in this codebase set the bar to match:

- `BacklogSourcesSettings.tsx:462` uses a native `<select className={styles.select}
  value={pluginId} onChange={...}>` for a similarly small fixed-option field — **prefer
  native `<select>` over a custom dropdown widget**, consistent with that file and with
  this repo's established settings patterns (native controls get keyboard nav,
  screen-reader labeling, and mobile picker UI for free).
- `PushNotificationSettings.tsx:54-63` shows the label-association pattern already used
  in this settings surface: `<input id="x" .../>` paired with `<label htmlFor="x">`.
  Any new control (toggle or select) must follow the same explicit `id`/`htmlFor` pairing
  — no bare `<input>`/`<select>` without an associated `<label>`.
- If a toggle is chosen instead of a select (per the recommendation in §2), use the
  same `type="checkbox"` + `<label htmlFor>` pattern PushNotificationSettings already
  uses, not a custom switch component, for consistency and to avoid re-solving ARIA
  state (`aria-checked`, focus ring, space/enter activation) that native `<input
  type="checkbox">` gets for free.
- If a 5-value radio group is chosen instead (matching `ThemePicker.tsx:63`'s
  `role="radiogroup"` pattern for its theme swatches), each option needs `role="radio"`
  + `aria-checked` + a visible focus state, and the group needs `aria-label` — the
  `ThemePicker` implementation is the closest in-repo precedent to copy from
  structurally, though a native `<select>` remains simpler for a flat value list with no
  visual preview.

## 5. Error/edge-case UX — local echo framing (important correction to requirements)

Checked whether xterm.js in this app does client-side local echo (which would make
batching invisible to the user regardless of delay) or waits for the server to echo
back over the WebSocket.

**Finding: this is NOT a local-echo terminal.** Traced both directions in
`XtermTerminal.tsx` and `TerminalOutput.tsx`:

- **Outgoing (keystroke → server)**: `XtermTerminal.tsx:689-690` — `terminal.onData()`
  fires `onDataRef.current?.(data)`, which is the sole handler for typed input. It does
  **not** call `terminal.write()` or otherwise render the character locally.
- **Incoming (server → screen)**: The *only* path that renders characters to the visible
  terminal is `TerminalOutput.tsx:443`'s `manager.write(output)`, fed by WS messages
  arriving from the server (plus `XtermTerminalHandle.write()` at
  `XtermTerminal.tsx:1194-1196`, used the same way).

In other words: every character a user sees appear on screen got there by round-tripping
through the server's PTY (tmux, in this repo — a cooked-mode PTY that echoes input by
default) and back over the WebSocket. There is no optimistic/predictive client-side
render.

**This means the requirements doc's framing needs a correction.** §"Goal" says batching
must "never introduc[e] perceptible input lag at the default setting" — true only
because the default is `0` (off). But for any *non-zero* `inputBatchDelayMs`, the visual
echo of a keystroke is delayed by up to the full batch window (plus round-trip time),
because the character can't be echoed by the remote PTY until the batched send actually
reaches it. This is the opposite of what a local-echo terminal (e.g. a browser text
input, or SSH clients with predictive local echo like Mosh) would give you — there,
batching the *transport* wouldn't delay the *render*. Here it does.

Practical implication: 32ms is very likely imperceptible (below typical human reaction/
perception thresholds for discrete events, ~50-100ms), but 128-256ms on the high end of
the proposed option set is close to or above the range where users report perceived
lag in interactive terminals/editors. This isn't a blocker — it's exactly why the
setting should default to off and be opt-in — but the plan/pre-mortem phase should
treat "may introduce perceptible lag at higher settings" as an accepted, disclosed
tradeoff rather than something the current requirements text's blanket "never
perceptible" claim covers. Recommend either qualifying that sentence in requirements.md
("...at the default setting of 0ms") or adding a caveat in any help text shown at
128/256ms ("higher values may add a small, noticeable delay before typed characters
appear").

## 6. Job-to-be-done — infra vs. user-facing framing

The functional job this serves — "fewer WebSocket messages for a burst of keystrokes"
— is a server/network load-reduction outcome. The people who feel that job directly are
the ops/infra side (fewer messages to marshal/route/tmux-write per burst) and, at the
margin, users on constrained connections (mobile data, high-latency links) where every
extra WS frame has real round-trip cost. For the overwhelming majority of users on a
normal desktop broadband connection, typing at normal speed is — per the requirements
doc's own admission — "nowhere near WebSocket message overhead limits," so there is no
felt emotional or functional job being solved for them; the benefit is invisible.

**This is a mismatch worth flagging explicitly**: the feature is framed and filed as a
`perf` item with no attached user complaint, originating from reading another project's
code rather than an observed stapler-squad problem (requirements.md, "Constraints /
Context"). That's an honest description of an **infra/perf item**, not a user-facing UX
feature. The social/emotional job a *user* might get from a visible "reduce network
traffic" toggle is thin — mostly power-user reassurance ("the app is being efficient"),
not a felt problem solved. Recommend treating this as infra work with the batching
default-off and no UI in the first cut (satisfying the acceptance criteria's
mechanism-level requirements 1-5, 7-8), and either deferring the settings-UI exposure
(criterion 6) to a follow-up scoped explicitly as "build the first TerminalConfig
settings panel" (a meaningfully bigger task per §1), or shipping it as a minimal toggle
per §2 rather than investing UX polish proportional to a user-facing feature this thin.
