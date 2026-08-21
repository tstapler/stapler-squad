# Requirements: Terminal Toolbar Re-order & Analytics

## Problem Statement

The terminal toolbar in stapler-squad is cluttered and poorly ordered. Dev/diagnostic tools (Debug, Log Stream, Record, Raw) share equal prominence with everyday user actions (Copy, Paste, Clear). There are no usage analytics to guide data-driven improvements. Users struggle to find the buttons they need because the order does not reflect usage frequency.

## Goals

1. **Instrument button analytics** — track every toolbar button click with the existing analytics infrastructure so real usage data accumulates immediately after this PR ships.
2. **Re-order buttons by usage frequency** — apply UX research and first-principles reasoning to place high-frequency actions first; low-frequency/destructive actions last.
3. **De-clutter dev-only tools** — Debug, Log Stream, Record, and the Raw streaming selector are diagnostic tools, not everyday user actions. Determine the best UX pattern (collapse/hide/separate) via research.
4. **Evaluate button necessity** — for each button, make a research-backed recommendation: keep as-is, keep but demote, or remove.

## Current State

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`

**Current order (left → right):**
```
[Kill] [Resize-icon] | [Debug] [Log Stream] [Record] [Raw▾] | [Paste] [Gallery] [Files] | [Mouse] [Copy] [Bottom] [Resize] [Clear]
```

**Problems observed:**
- Debug, Log Stream, Record, Raw occupy the most prominent positions (leftmost after kill/resize controls) despite being rarely used by end users
- Copy, Paste, Clear — the most commonly needed terminal actions — are buried to the right
- No visual grouping signals importance or frequency
- "Mouse ON" toggle is prominent but rarely needed
- "Resize" (manual fit) is near-redundant if auto-fit works
- 13 visible buttons on desktop is too many for quick scanning

## Scope

### In scope
- Re-order and group toolbar buttons in `TerminalOutput.tsx`
- Add `track()` analytics calls on every toolbar button click
- Decide dev-tool presentation (collapsed section vs. end-of-bar vs. hidden)
- Recommend button removals with rationale
- CSS changes in `TerminalOutput.css.ts` if grouping needs new styles

### Out of scope
- Building an analytics dashboard to visualize the new data (follow-up)
- Changing button functionality (only order, grouping, visibility)

## Mobile Thumb-Reachability Constraint

On mobile (portrait, single-handed), users hold the device in their right hand and reach the screen with their right thumb. The natural reach zone on a phone is the **bottom-right quadrant**. The current mobile overflow row flows left-to-right, placing the most-used buttons (Copy, Paste, Clear) on the far left — the hardest spot for right-thumb access.

**Requirement:** On mobile, the most-used actions (Copy, Paste, Clear, Bottom) must be placed at the **right end** of the overflow row (or the primary bar) so they are in the right-thumb comfort zone. Options:
- Reverse the mobile overflow row order (CSS `flex-direction: row-reverse`) so high-frequency buttons appear on the right
- Or use `justify-content: flex-end` on the overflow container so buttons start from the right

The implementation should apply this only to the mobile overflow row, not the desktop layout.

## Constraints

- Must not break any existing tests in `__tests__/TerminalOutput.*.test.tsx`
- Dev-only buttons must remain reachable (not removed, just demoted)
- Analytics calls must use the existing `useAnalytics()` + `track()` pattern already in the file
- CSS must follow `.css.ts` (vanilla-extract) conventions — no new `.module.css` files
- No new proto/RPC changes needed (analytics are frontend-only events)

## Acceptance Criteria

1. Every toolbar button click fires a `track()` event with a consistent naming scheme (e.g. `toolbar_button_click` + `{button: "copy"}`)
2. High-frequency actions (Copy, Paste, Clear, Bottom) appear before low-frequency ones
3. Dev/diagnostic tools (Debug, Log Stream, Record, Raw) are visually separated or collapsed — not in the primary button row
4. Total visible buttons on desktop reduced from 13 to ≤8 (with dev tools hidden/collapsed)
5. All existing tests pass; new tests cover the analytics calls
6. Grouping has a clear visual separator or label so users understand the hierarchy
7. On mobile overflow row, high-frequency buttons (Copy, Paste, Clear, Bottom) are positioned on the right side for right-thumb reach (flex-direction: row-reverse or equivalent)

## Open Questions for Research

- What toolbar UX pattern best fits dev-tool hiding? (Overflow menu, collapsible panel, "Dev" toggle button, right-aligned group)
- Is "Mouse" mode toggle used enough to stay in the primary bar, or should it be demoted?
- Should "Resize" be removed (auto-fit covers most cases) or kept but demoted?
- What event schema should analytics use for toolbar clicks to be useful downstream?
