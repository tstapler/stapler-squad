# UX Design: Max Autonomous Session Turns

**Feature:** Add "Max Autonomous Session Turns" numeric input to the Global Defaults settings form.
**Date:** 2026-10-07
**Status:** Pre-implementation artifact

---

## 1. User-Facing Surfaces

**One surface:** Settings → Global Defaults form (`web-app/src/components/settings/GlobalDefaultsForm.tsx`).

A single numeric input field inserted between "Max Auto-Rework Iterations" and "Max Concurrent Backlog Work Items". No new page, modal, or navigation entry is required.

---

## 2. Wireframe

The Global Defaults form renders as a vertical stack of labeled fields. The new field slots in at position shown below. Surrounding fields are shown for placement context only.

```
┌─────────────────────────────────────────────────────────────────┐
│  Global Defaults                                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Max Auto-Rework Iterations                                     │
│  ┌──────────┐                                                   │
│  │    3     │  ← existing field (unchanged)                    │
│  └──────────┘                                                   │
│  How many times a backlog item can be auto-reopened...         │
│                                                                 │
│ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ NEW FIELD ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─  │
│                                                                 │
│  Max Autonomous Session Turns                                   │
│  ┌──────────┐                                                   │
│  │   30     │  ← type="number" min=1 max=200                   │
│  └──────────┘                                                   │
│  How many turns a single autonomous backlog session gets        │
│  before being stopped automatically. Setting this too low       │
│  (under ~10) risks cutting off a session mid-task; too high     │
│  (over 50) risks runaway cost on a stuck agent.                 │
│  Default: 30. Hard ceiling: 200.                                │
│                                                                 │
│ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─  │
│                                                                 │
│  Max Concurrent Backlog Work Items                              │
│  ┌──────────┐                                                   │
│  │    2     │  ← existing field (unchanged)                    │
│  └──────────┘                                                   │
│  How many backlog items can have a live work session at once.  │
│                                                                 │
│  ...                                                            │
│                                                                 │
│  ┌──────────┐                                                   │
│  │   Save   │                                                   │
│  └──────────┘                                                   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Interaction Flow

### Happy path

```
User opens Settings → Global Defaults
        │
        ▼
Form loads; GET /getSessionDefaults
        │
        ├─ server returns defaults.autonomousMaxTurns = N > 0
        │         → field displays N
        │
        └─ server returns defaults.autonomousMaxTurns = 0 (omitempty / unset)
                  → field displays 30  (client-side fallback)
        │
        ▼
User focuses "Max Autonomous Session Turns" input
(keyboard Tab or mouse click)
        │
        ▼
User types a value (e.g. "50") or uses arrow keys / mobile stepper
        │
        ├─ onChange fires: parseInt(e.target.value, 10) || 1
        │         → clamps to Math.min(200, Math.max(1, parsed))
        │         → updates React state immediately
        │
        ▼
User clicks "Save"
        │
        ├─ saving=true → button shows "Saving..." (disabled)
        │
        ├─ PUT /updateGlobalDefaults { autonomousMaxTurns: <value>, … }
        │
        ├─ success → banner "Global defaults saved." auto-dismisses after 3 s
        │
        └─ error   → banner "Failed to save defaults: <reason>"
                      Save button re-enabled; user can retry
```

### Loading state

When the Settings → Global Defaults component mounts, the `autonomousMaxTurns` React state is initialised to `0` (matching the existing pattern for adjacent fields). The `useEffect` that calls `getSessionDefaults` sets the value to the server-resolved result (or 30 if the response returns 0 via `|| 30`). During the in-flight GET, the field briefly renders with the initial state value — `0 || 30 = 30` — so the user sees "30" from the first render. There is no blank flash. This matches the behaviour of `maxAutoReworkIterations` and `maxConcurrentBacklogWorkItems`.

### Out-of-range entry (browser native vs. clamping)

The `onChange` handler clamps immediately; the field never displays a value outside `1–200`. Typing "999" produces "200" on blur equivalent; typing "0" or "-5" produces "1". There is no separate validation step and no submit-time error for this field — clamping at input time prevents out-of-range values from reaching the server.

**Known UX limitation — clear-and-retype:** Because the clamp runs on every `onChange` event, clearing the field (producing an empty string) immediately snaps the value to 1 (`parseInt("") = NaN; NaN || 1 = 1`). The user cannot clear-and-retype a multi-digit value using blank as an intermediate step. The intended workaround is **select-all-and-type** (`Ctrl-A` or triple-click to select the current value, then type the replacement). This is the same behaviour as the `maxConcurrentBacklogWorkItems` field — it is a known, intentional tradeoff of the shared clamp pattern, not a bug to fix in this story. Document in the hint text that values outside the range `1–200` are automatically adjusted to stay within bounds.

### Empty / NaN entry

`parseInt(e.target.value, 10) || 1` treats blank, non-numeric, or NaN input as 1. The field will show 1 rather than remain blank, preventing a zero from reaching the server.

---

## 4. Error and Edge-Case Handling

| Scenario | UI behaviour | Exit path |
|---|---|---|
| Server returns `autonomousMaxTurns = 0` (omitempty) | Field initialises to 30 (client fallback: `defaults.autonomousMaxTurns \|\| 30`) | User sees the default; no action required |
| User types a value > 200 | `onChange` clamps to 200 immediately; visual feedback is the field value snapping | User sees 200 in the field |
| User types a value < 1 or 0 or negative | `onChange` clamps to 1 immediately | User sees 1 in the field |
| User types non-numeric text | `parseInt(...) \|\| 1` resolves to 1; field shows 1 | User sees 1 |
| User clears the field completely | Same as non-numeric: resolves to 1 on next keystroke | Field shows 1 |
| Save fails (network error / server error) | Error banner: "Failed to save defaults: <reason>"; Save button re-enabled | User can retry or correct underlying issue |
| Page load fails (GET error) | Existing error banner: "Failed to load defaults: <reason>" | User can refresh; load is retried on component remount |
| Mobile / touch device | `type="number"` triggers numeric keyboard; touch target size handled by shared `input` CSS class; no spinner arrows needed (onChange clamping handles bounds) | — |

---

## 5. Accessibility

### Semantic structure

- `<label htmlFor="global-autonomous-max-turns">` paired with `<input id="global-autonomous-max-turns">` — explicit `htmlFor`/`id` association.
- `type="number"` with `min={1}` and `max={200}` — browsers expose these attributes to screen readers as range constraints.
- Hint text rendered as `<p id="global-autonomous-max-turns-hint" className={hint}>` immediately after the input. The input must carry `aria-describedby="global-autonomous-max-turns-hint"` — DOM proximity alone is not sufficient for WCAG 3.3.2 / 1.3.1; VoiceOver, NVDA, and JAWS do not reliably announce an unassociated hint `<p>` on focus.

### Keyboard navigation

- Tab order: field is reached by Tab in document flow; no custom focus management required.
- Arrow-key increment/decrement: native `<input type="number">` behaviour; clamping in `onChange` prevents overshoot past bounds.
- Enter on Save button: button is `type="button"` and responds to Enter/Space as expected for a `<button>`.

### Color contrast

- Inherits the shared `input` and `hint` CSS classes (`GlobalDefaultsForm.css`) already used by adjacent fields. No new color values are introduced. Existing palette must meet WCAG AA minimum 4.5:1 for normal text — verified by the existing CI Axe Core run (`make e2e-lighthouse`).

### Screen reader label

Announced as: "Max Autonomous Session Turns, spin button, minimum 1, maximum 200" (browser + screen reader default for `<input type="number" min max>`).

---

## 6. UX Acceptance Criteria

All criteria are testable by a human in a browser without instrumentation.

### Loading and initialisation

1. After opening Settings → Global Defaults, the "Max Autonomous Session Turns" field appears between "Max Auto-Rework Iterations" and "Max Concurrent Backlog Work Items". (1 navigation step)
2. If the server has never saved a value for this field (i.e., it returns 0), the field displays **30** as its initial value.
3. If the server returns a previously saved value (e.g., 50), the field displays **50**.

### Editing and clamping

4. Typing **201** into the field results in the field showing **200** (upper bound clamped); no separate error message is shown.
5. Typing **0** or **-1** into the field results in the field showing **1** (lower bound clamped); no separate error message is shown.
6. Clearing the field and tabbing away results in the field showing **1**; the Save button remains enabled.
7. Typing **25** and saving completes in ≤ 3 clicks (focus field → type value → click Save).

### Save and feedback

8. Clicking Save while the field contains a valid value (1–200) shows a "Saving..." disabled button state while the request is in flight, then shows "Global defaults saved." on success. The banner disappears automatically within 4 seconds.
9. After a successful save, reloading the page shows the previously saved value — confirming round-trip persistence.

### Error recovery

10. If the save request fails (simulate with network offline), the error banner reads "Failed to save defaults: `<reason>`" and the Save button becomes enabled again. The user can click Save again without refreshing the page. (No dead end.)
11. If the initial page load fails, the error banner reads "Failed to load defaults: `<reason>`". Refreshing the page retries the load. (No dead end.)

### Accessibility

12. With keyboard-only navigation, the user can Tab to the "Max Autonomous Session Turns" input, type a value, Tab to the Save button, and activate it with Enter — all without using a mouse.
13. The field's label "Max Autonomous Session Turns" is announced by a screen reader (VoiceOver or NVDA) when the input receives focus, followed by the current numeric value.
14. The hint text describing the field's behaviour is reachable by a screen reader in natural reading order (no skip-link required).
15. Text contrast for the label and hint text meets WCAG AA (≥ 4.5:1) — confirmed by Axe Core in the CI Lighthouse run touching `web-app/src/`.

### Mobile

16. On a mobile device (iOS Safari / Android Chrome), tapping the "Max Autonomous Session Turns" field raises a numeric keypad (not a full QWERTY keyboard).
17. The touch target for the field is large enough to tap without accidental mis-taps — visually comparable to adjacent numeric fields.

---

## 7. Implementation Notes (for handoff)

These notes capture constraints discovered during UX analysis; they are not design decisions but hard constraints the implementation must satisfy.

- **Field ID:** `global-autonomous-max-turns` — used by `htmlFor`/`id` pairing and by any future E2E `data-testid` (add `data-testid="autonomous-max-turns-input"` to match the `stale-session-threshold-input` precedent in the same file).
- **Hint ID and `aria-describedby`:** The hint `<p>` must carry `id="global-autonomous-max-turns-hint"` and the `<input>` must carry `aria-describedby="global-autonomous-max-turns-hint"` — required for WCAG 3.3.2 / 1.3.1 (screen readers announce the hint on focus only when the programmatic association is explicit).
- **State initialisation:** `defaults.autonomousMaxTurns || 30` — the `|| 30` guard handles the proto `omitempty` zero value so the field never starts at 0.
- **onChange handler:** `Math.min(200, Math.max(1, parseInt(e.target.value, 10) || 1))` — mirrors the `maxConcurrentBacklogWorkItems` handler's clamp pattern at `GlobalDefaultsForm.tsx:346`.
- **Save payload:** add `autonomousMaxTurns` to the `updateGlobalDefaults` call at `GlobalDefaultsForm.tsx:97`; the field must round-trip through the existing `handleSave` function without special-casing.
- **No new CSS:** the field uses the same `field`, `labelClass`, `input`, and `hint` class names already imported at lines 11–28 of `GlobalDefaultsForm.tsx`.
