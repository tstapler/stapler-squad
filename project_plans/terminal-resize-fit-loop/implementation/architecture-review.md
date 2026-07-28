# Architecture Review: terminal-resize-fit-loop
**Date**: 2026-07-27
**Verdict**: CLEAN

## Constitution Check
`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository. No
constitution constraints apply — N/A, no violations to report.

---

## Blockers

None. The prior BLOCKER (webglAddonRef race across effect re-runs) is resolved.

Re-verified against `plan.md` Task 2.1.1b ("Set/null the ref in the IIFE, `onContextLoss`, and
cleanup — with a liveness guard against StrictMode/scrollback-remount races"):

- **Checked before `terminal.loadAddon()`, not just before the ref write**: the plan text places
  `if (cancelled) return;` immediately after `await import('@xterm/addon-webgl')` and explicitly
  states this is "before constructing or loading the addon at all, so a cancelled mount never
  calls `terminal.loadAddon()` on a disposed terminal in the first place." This closes the
  reviewer's stronger ask ("ideally before `terminal.loadAddon(webglAddon)` too") rather than only
  the ref-write.
- **`cancelled = true` set as the first line of cleanup**: the plan text says "In the effect
  cleanup block (lines 511-525), add `cancelled = true;` as the **first** line of the cleanup
  function (before any other cleanup runs)" — satisfies the ordering requirement; no other cleanup
  step can yield/await before the flag is flipped.
- **`onContextLoss`'s existing null-out behavior intact**: the plan updates the `onContextLoss`
  handler to keep its existing `console.warn` + `webglAddon.dispose()` and *adds*
  `webglAddonRef.current = null;` after them — it extends rather than removes any existing
  behavior, and remains the second (post-oscillation-detector) path that nulls the ref, consistent
  with the Story 2.1.1 acceptance criteria ("`null` after disposal via either the existing
  `onContextLoss` handler or the new oscillation path").
- The `cancelled` flag is declared as a `let` local to each effect invocation (`oscillationHistory`
  pattern reused), not a persistent ref, so it cannot leak state across mounts the way the
  original `webglAddonRef`-only design did.

Effect: a stale mount's IIFE now returns before ever calling `terminal.loadAddon()` on the
disposed terminal, and cannot resolve into `webglAddonRef.current` after the live mount has
already set it. The race described in the original BLOCKER is closed.

## Concerns

All 4 prior CONCERNS are resolved.

1. **`shouldFit`/`shouldSendResize` primitive obsession (transposition hazard)** — RESOLVED.
   `resizeConvergence.ts` (Task 1.1.1a/1.1.1b) now defines `TerminalSize { cols, rows }` and both
   functions take `TerminalSize`-shaped params (`shouldFit(proposed: Partial<TerminalSize> |
   undefined, current: TerminalSize)`, `shouldSendResize(next: TerminalSize, lastSent:
   TerminalSize | null)`). Verified call sites use object literals, not positional numbers: Task
   2.2.1a's `shouldFit(proposed, { cols: term.cols, rows: term.rows })` and Task 3.1.1's
   `shouldSendResize({ cols, rows }, lastSentSizeRef.current)`. `ResizeEvent` also now extends
   `TerminalSize`. The positional-argument transposition hazard the original concern flagged no
   longer exists in any call site checked.

2. **`webglAddonRef.current === null` conflating "never loaded" vs "already fell back"** —
   RESOLVED. Task 2.1.1a adds `webglFallbackTrippedRef = useRef(false)`, set `true` only in the
   oscillation-dispose branch (Task 2.3.1b) and never reset for the life of the mount (explicitly
   stated, and *not* reset in cleanup, which is correct since a new mount gets a fresh ref via
   `useRef`'s per-render-tree identity). The dispose branch in Task 2.3.1b now has three cases: (a)
   `webglAddonRef.current` truthy → dispose + set both refs; (b) falsy and
   `!webglFallbackTrippedRef.current` → the original `console.error` backstop, now correctly scoped
   to "genuinely never loaded"; (c) falsy and `webglFallbackTrippedRef.current` true → a
   `console.log('...persists after WebGL fallback')` instead of a repeated misleading error. This
   is a concrete, correct fix, not just an acknowledgment.

3. **Test gap for repeated oscillation burst** — RESOLVED. Story 4.3.1 / Task 4.3.1a's third
   acceptance-criteria case (plan.md lines 724-730) explicitly covers "WebGL already fell back
   once... a second independent oscillation sequence... occurs" and asserts `dispose` is not
   called again, `console.error` is not called again, and only the new `console.log` message
   fires — directly exercising the case #2's fix and preventing regression to the old
   always-console.error behavior.

4. **ADR deferred to implementation phase** — left as a deliberate, accepted deviation per the
   original review's own assessment (low risk, tracked mandatory task, not a TODO). Per this
   task's instructions, this was addressed by expanding the Task 5.1.1a ADR content outline rather
   than pulling the ADR-writing task forward, which is consistent with what was actually
   requested: Task 5.1.1a's "Consequences" section now explicitly requires the false-positive
   discriminator (window/pane-drag alternating-resize false trip), the imperative `fit()` path
   scope note, and other adversarial-review points to be included when the ADR is written during
   implementation. This closes the separate adversarial-reviewer points that had been bundled into
   this concern; the underlying "write now vs. write later" recommendation remains a stated,
   accepted trade-off rather than a defect, exactly as before.

## Nitpicks

Carried forward unchanged from the prior review — not in scope for this repair pass:

- Task 2.3.1a declares `let oscillationHistory: ResizeEvent[] = [];` "alongside `lastContainerSize`/
  `resizeTimeout`" at ~line 451-452, which is textually *after* `resizeDisposable`'s declaration at
  line 433 that Task 2.3.1b modifies to reference `oscillationHistory`. This works correctly (JS
  closures capture the block-scoped binding, and `resizeDisposable`'s callback body doesn't
  execute until a later tick, by which point the whole effect body — including the
  `oscillationHistory` declaration — has already run), but it's a forward reference that reads
  oddly. Consider declaring `oscillationHistory` before `resizeDisposable` for readability.
- The plan's new `XtermTerminalResize.test.tsx` correctly avoids the missing-`ResizeObserver`-
  polyfill blind spot that pitfalls.md flagged in the existing `XtermTerminal.test.tsx` smoke
  tests (Task 4.2.1a explicitly stubs `global.ResizeObserver`). The plan does not, however, touch
  the pre-existing "renders without error" tests that silently no-op today — those remain a latent
  false-confidence risk after this PR lands, even though it's explicitly out of scope per
  requirements.md. Worth a one-line follow-up note/ticket, not a blocker for this PR.
