# UX Research: "Max Autonomous Session Turns" Settings Field

## Pattern Baseline (from GlobalDefaultsForm.tsx)

The two closest existing fields are:

**Max Auto-Rework Iterations** (lines 311–330) — `min={1}`, no `max`, clamps with
`Math.max(1, Number(e.target.value) || 1)`. Has a `<p className={hint}>` explaining
consequences: what happens when the cap is reached (item left in review for manual
action).

**Max Concurrent Backlog Work Items** (lines 332–354) — `min={1}`, `max={10}`, clamps
with `Math.min(10, Math.max(1, Number(e.target.value) || 1))`. The hard cap is
reflected in both the `max` HTML attribute and the clamp expression, making the bound
visible to assistive technologies and browser-native constraint validation.

The new field should mirror the bounded pattern from Max Concurrent, not the unbounded
pattern from Max Auto-Rework, because `config.go` enforces a hard ceiling at 200
server-side and a soft default at 30 — the UI should reflect both.

## Recommended Field Implementation

```tsx
{/* Max Autonomous Session Turns */}
<div className={field}>
  <label className={labelClass} htmlFor="global-autonomous-max-turns">
    Max Autonomous Session Turns
  </label>
  <input
    id="global-autonomous-max-turns"
    type="number"
    min={1}
    max={200}
    className={input}
    value={autonomousMaxTurns}
    onChange={(e) =>
      setAutonomousMaxTurns(Math.min(200, Math.max(1, Number(e.target.value) || 1)))
    }
  />
  <p className={hint}>
    How many turns a single autonomous backlog session gets before being stopped
    automatically. Setting this too low (under ~10) risks cutting off a session
    mid-task; too high (over 50) risks runaway cost on a stuck agent. Default: 30.
    Hard ceiling: 200.
  </p>
</div>
```

## Hint Text Rationale

The hint must answer three operator questions in one breath:
1. What does this control? — "turns a single autonomous backlog session gets before
   being stopped automatically."
2. What's the risk of too low? — cuts off mid-task before the agent finishes.
3. What's the risk of too high? — runaway cost/time if the agent gets stuck.

Mentioning both the system default (30) and the hard ceiling (200) is warranted here
because:
- The field loads a server-resolved value (populated from `defaults.autonomousMaxTurns`
  in `loadDefaults`), so the user may see 30 without knowing whether that's their
  explicit choice or the fallback.
- The ceiling is enforced server-side in `AutonomousMaxTurnsOrDefault`; surfacing it
  prevents confusion when a value above 200 silently gets clamped on save.

## Error / Edge-Case Handling

| Scenario | Handling |
|---|---|
| User types 0 or clears the field | `Number(e.target.value) \|\| 1` coerces to 1; clamp keeps it at `min=1` |
| User types > 200 | `Math.min(200, ...)` clamps before the state update; browser also constrains via `max={200}` |
| User types a decimal (e.g. 5.7) | `Number(...)` returns 5.7; `Math.max`/`Math.min` pass it through — add `Math.round` or `parseInt` to keep it integer: `Math.min(200, Math.max(1, parseInt(e.target.value, 10) \|\| 1))` |
| Server returns 0 (unset/omitempty) | Initialize state to `autonomousMaxTurnsDefault` (30) via `defaults.autonomousMaxTurns \|\| 30` in `loadDefaults`, matching the existing `maxConcurrentBacklogWorkItems \|\| 2` pattern |

## Accessibility

- `htmlFor` / `id` pairing on label + input: already required by the existing pattern;
  the `id="global-autonomous-max-turns"` value keeps the naming convention consistent.
- `type="number"` with explicit `min` and `max` attributes: these are read by screen
  readers as range constraints ("minimum 1, maximum 200").
- No additional ARIA attributes are needed beyond what `type="number"` provides, unless
  an inline error message is added — in that case add `aria-describedby` pointing to the
  error element. The current form handles errors at the form level via `{error && ...}`,
  so field-level ARIA is not needed to match the existing pattern.
- The CSS `input` class is shared with all other numeric inputs in this form; no
  accessibility-specific style changes are needed.

## Mobile / Touch Targets

- `type="number"` triggers a numeric keyboard on iOS/Android, matching operator
  expectations for a bounded integer field.
- The existing `input` CSS class already controls minimum touch target size (shared with
  other fields); no per-field override is needed unless the design system's base size
  is below 44×44 pt (the WCAG 2.5.5 recommended minimum). This should be verified
  against `GlobalDefaultsForm.css` but is not expected to need a change since the other
  numeric fields pass the same constraint.
- No spinner arrows from `type="number"` are shown on mobile; the clamp logic in
  `onChange` covers the constraint without relying on them.

## Field Placement

Place the new field immediately after "Max Auto-Rework Iterations" (line 330) and
before "Max Concurrent Backlog Work Items" (line 332). These three fields all govern
autonomous backlog behavior and should read as a group. Grouping by domain (autonomous
caps → concurrency → staleness) is more scannable than insertion at the end of the form.
