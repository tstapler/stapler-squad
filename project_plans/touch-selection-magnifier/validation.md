# Validation: touch-selection-magnifier

**Date**: 2026-10-17
**Reviewed by**: adversarial review pass

## Adversarial review findings

### Finding 1 — `innerHTML` assignment introduces XSS risk

**Concern**: Task 5b uses `mag.innerHTML = ...` with terminal content. If a running
program writes a string that looks like `<img src=x onerror=alert(1)>` into the terminal
buffer, `translateToString` returns that raw string, and `innerHTML` would parse and
execute it.

**Severity**: Medium — the terminal already displays arbitrary program output, but
injecting executable HTML into the DOM is a different threat model.

**Mitigation in plan**: The plan already includes an `escapeHtml` function that escapes
`<`, `>`, and `&` before insertion. Verified: the only dynamic content set via `innerHTML`
is `escapeHtml(before) + <span>escapeHtml(cursor)</span> + escapeHtml(after)` — all
user-controlled strings are escaped. ✓

**Residual risk**: None, given the escaping. Could alternatively use `textContent` +
DOM node manipulation to avoid innerHTML entirely, which would be safer by construction
but requires more code. Recommend using `textContent` + `createElement('span')` instead
of innerHTML in the implementation to be belt-and-suspenders safe.

**Plan adjustment**: Update Task 5b to use DOM node manipulation instead of innerHTML.

---

### Finding 2 — Approximate magnifier dimensions may clip on small phones

**Concern**: Task 5b uses hardcoded `MAGNIFIER_HEIGHT = 36` and `MAGNIFIER_WIDTH = 200`
for positioning, but the actual rendered size depends on font metrics and the browser's
pixel ratio. On a 320-px phone with a large system font setting, the magnifier bubble
could be wider than `200px`, causing the clamp to position it incorrectly.

**Severity**: Low — the magnifier is a display aid, not interactive; misplacement is
cosmetic.

**Mitigation**: The plan should note that after the first frame the element has a real
`offsetWidth` / `offsetHeight` that can be used for positioning subsequent frames. For
the first frame, approximate values are acceptable. The `MAGNIFIER_WIDTH` style is also
set explicitly (`mag.style.width = ...`) in the plan, which fixes the width — only height
remains approximate.

**Plan adjustment**: The position calculation in Task 5b should use `MAGNIFIER_WIDTH`
consistently (it already sets `mag.style.width`, so the approximation is self-fulfilling).
For height, either fix a min-height in CSS or accept the one-frame positioning error.
Add a note in the implementation code comment.

---

### Finding 3 — `magnifierRef` passed across the `attachHandleDrag` closure boundary

**Concern**: `magnifierRef` is a React ref created in the component body. `attachHandleDrag`
is called inside a `useEffect` with the `[scrollback]` dependency. If React ever re-runs
the effect (e.g. scrollback changes), `attachHandleDrag` is called again with the same
`magnifierRef` — no stale closure issue because `magnifierRef.current` is read at call
time, not at effect setup time. ✓

**Mitigation**: React refs are stable objects; `.current` is always live. No action needed.

---

### Finding 4 — Magnifier not hidden if `touchcancel` fires without prior `touchstart`

**Concern**: If `touchcancel` fires without `touchstart` (edge case on some Android
versions under extreme memory pressure), `magnifierRef.current.style.display` is never
set to `'block'` so `'none'` assignment in `onTouchEnd` is a no-op. No visible bug.

**Severity**: Negligible — state is already `display: none` by default.

---

### Finding 5 — No unit test for the `escapeHtml` function itself

**Concern**: `escapeHtml` is used to prevent XSS; it should be tested with inputs
containing `<`, `>`, `&`, and combinations.

**Plan adjustment**: Add `escapeHtml` tests in Task 2 (extend the test file for
`touchDrag.ts` helpers, or add a separate `escapeHtml.test.ts`). Test cases:
`'a<b>c&d'` → `'a&lt;b&gt;c&amp;d'`, empty string, string with no special chars.

However, given Finding 1's recommendation to avoid `innerHTML` entirely, if the
implementation uses `textContent` + DOM nodes, `escapeHtml` is not needed and
Finding 5 is moot.

---

### Finding 6 — `window.innerWidth` read inside the RAF callback

**Concern**: `window.innerWidth` in Task 5b is read inside `applyHandleDrag`, which runs
on every animation frame. On mobile, `window.innerWidth` can trigger a layout flush if
the browser hasn't finalized it.

**Severity**: Low — `window.innerWidth` is a cached layout value on all modern browsers
(does not force a full reflow). The concern would be `getBoundingClientRect()` or
`offsetWidth`/`offsetHeight` on a non-fixed-position element; `innerWidth` is safe.

**Mitigation**: No action needed. Could capture `window.innerWidth` once at `touchstart`
alongside `geometry`; this is a micro-optimization that can be done opportunistically.

---

## Validation summary

The plan is sound. Four adjustments are recommended before implementation:

1. **Replace `innerHTML` with DOM node manipulation** to avoid XSS by construction
   (Finding 1 + Finding 5 → both resolved if innerHTML is avoided).
2. **Add `escapeHtml` unit tests** if `innerHTML` is kept (else skip per Finding 5).
3. **Document the height approximation** in a code comment (Finding 2).
4. **Optionally cache `window.innerWidth`** at `touchstart` (Finding 6 — micro-opt,
   not required).

All findings are low-severity cosmetic or defensive hardening issues. No blockers.
The core approach (text content from buffer API, fixed-position portal element,
RAF-throttled updates) is validated.
