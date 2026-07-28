# ADR-001: Add `@xterm/addon-canvas` as an explicit new dependency for the WebGL fallback

**Status**: Accepted
**Date**: 2026-07-24
**Project**: terminal-resize-fit-loop

## Context

Requirements AC5 (verbatim): "The WebGL actual-vs-expected pixels-per-column discrepancy is
corrected or mitigated so `fit()` converges under WebGL rendering: a sustained mismatch beyond
a defined tolerance triggers a one-directional fallback to the **canvas renderer**..."

The requirements' Constraints section also states "No new dependencies expected" for this
change, and this is flagged explicitly in requirements.md as a tension needing a plan-level
decision.

Investigation (Phase 2 build-vs-buy + stack research, confirmed directly against
`web-app/package.json` and `web-app/package-lock.json` in this repo):

- `@xterm/addon-webgl@^0.18.0` is installed and used in
  `web-app/src/components/sessions/XtermTerminal.tsx` (lines 7, 150-155).
- `@xterm/addon-canvas` is **not** installed anywhere in this repo (`node_modules`,
  `package.json` dependencies, and `package-lock.json` all confirmed absent).
- xterm.js core has no automatic WebGL→canvas fallback. When a `WebglAddon` is disposed
  (e.g., via `onContextLoss` or manual `dispose()`), xterm.js core falls back to its
  built-in **DOM** renderer, not canvas. A real canvas-based fallback requires loading
  `@xterm/addon-canvas`'s `CanvasAddon` explicitly — it is a distinct, separately-installed
  addon, not a built-in mode.
- `@xterm/addon-canvas@0.7.0`'s peer dependency is `@xterm/xterm: ^5.0.0`. The installed
  `@xterm/xterm@^5.5.0` satisfies this range — confirmed compatible, no version conflict.
- A one-time test-harness precedent exists at
  `web-app/src/app/test/terminal-stress/page.tsx` (lines ~106-125) that disposes
  `WebglAddon` on context loss and labels the resulting state `'canvas'` in its UI —
  but that page does **not** load `CanvasAddon`; it mislabels the DOM-renderer fallback as
  "canvas." This is existing, pre-fix terminology drift in test-only code, not evidence that
  a canvas renderer is already wired up anywhere.

## Decision

**Add `@xterm/addon-canvas@^0.7.0` as a new production dependency** in `web-app/package.json`,
and load a real `CanvasAddon` as the fallback target when the WebGL mismatch-tracker or
`onContextLoss` handler trips.

## Rationale

1. **Literal requirement compliance.** AC5's text says "canvas renderer," not "DOM renderer"
   or "non-WebGL renderer." Treating "canvas" as loose terminology for the DOM fallback would
   silently under-deliver against a criterion that was written with a specific rendering tier
   in mind (the existing docstring in `XtermTerminal.tsx` lines 71-73 already documents a
   3-tier mental model: "Canvas-based rendering (10-100x faster than DOM)" / "WebGL
   acceleration (2x faster than canvas)" — implying DOM < Canvas < WebGL was always the
   intended hierarchy, and "falling back" from WebGL was always meant to land on Canvas, not
   skip past it to DOM).
2. **Performance is an incidental benefit of satisfying #1, not an independent goal of this
   fix.** requirements.md's Out-of-Scope list explicitly excludes "general terminal performance
   work beyond what's needed to stop the specific feedback loop" — this ADR is not proposing
   Canvas *for* its speed; it is required regardless because AC5's literal wording names
   "canvas renderer." That the chosen (mandatory) fallback target also happens to preserve more
   throughput than the DOM-only alternative is a welcome side effect of literal-compliance,
   worth noting so a reader understands the fallback isn't a cosmetic downgrade — but it does
   not independently justify the dependency addition; only Rationale #1 does that.
3. **Peer-dependency compatibility is confirmed, low-risk.** `^5.0.0` peer range covers the
   installed `@xterm/xterm@^5.5.0` with no version pin conflicts against the other installed
   `@xterm/addon-*` packages (fit `^0.10.0`, search `^0.15.0`, web-links `^0.11.0`, webgl
   `^0.18.0` — all published for the `@xterm/xterm@5.x` line).
4. **One-line, low-blast-radius addition.** This is a single new leaf dependency with no
   transitive footprint of its own beyond the xterm.js addon family already in the tree — not
   a new class of tooling, build step, or architectural surface. The "no new dependencies
   expected" constraint reads as a *default expectation* set before this specific tension was
   discovered during Phase 2 research, not as an absolute prohibition; the constraint's intent
   (avoid unnecessary dependency sprawl for a bug fix) is honored by picking the smallest
   possible addition that satisfies the literal, deliberately-worded acceptance criterion.

## Alternatives Considered

- **Redefine "canvas" as loose terminology for the DOM fallback (no new dependency).**
  Rejected: avoids the dependency but does not literally satisfy AC5's wording, permanently
  bakes the `terminal-stress/page.tsx` mislabeling into production code/comments, and throws
  away a real, cheap performance tier for exactly the workload (multiple concurrent
  high-throughput terminals) this bug report is about.
- **Skip a rendering-tier fallback entirely; just stop calling `fit()` when mismatch is
  detected.** Rejected: does not address AC5's literal requirement, and leaves the terminal
  running under a renderer known to be measuring itself inconsistently — the actual defect,
  not just its downstream symptom (the resize loop), would remain unaddressed.

## Consequences

- `web-app/package.json` and `web-app/package-lock.json` gain one new dependency:
  `@xterm/addon-canvas@^0.7.0`.
- `XtermTerminal.tsx` gains an import of `CanvasAddon` and a code path that loads it only when
  the WebGL fallback trips (not loaded eagerly on every session, keeping the common-case
  bundle/runtime cost limited to the WebGL happy path already in place).
- Future maintainers reading "canvas renderer" in code comments/logs get a renderer that is
  actually Canvas, not a mislabeled DOM fallback — resolves the terminology drift from
  `terminal-stress/page.tsx` rather than propagating it.

## Addendum (2026-07-27): `@xterm/xterm` bumped to 6.0.0 in an unrelated PR

After this ADR was written, `main` independently upgraded `@xterm/xterm` from `^5.5.0` to
`^6.0.0` (unrelated PR, merged before this branch's rebase). This reopened the peer-dependency
question: `@xterm/addon-canvas@0.7.0`'s peer range is `@xterm/xterm: ^5.0.0` and was never
updated, because upstream removed the canvas renderer from the xterm.js monorepo entirely as
of 6.0.0 (xtermjs/xterm.js#5105, an explicitly-flagged breaking change: "Remove the canvas
renderer — this addon no longer exists and we recommend using either the DOM renderer or
WebGL"). `pnpm install` only warns on the unmet peer range (no `strict-peer-dependencies`
config in this repo) — CI stays green regardless of whether the addon actually still works.

**Verified, not assumed**: `CanvasAddon.activate()` reaches into `terminal._core`'s private
service surface (`coreService`, `optionsService`, `screenElement`, `linkifier`, `onWillOpen`,
`_bufferService`, `_renderService`, `_characterJoinerService`, `_charSizeService`,
`_coreBrowserService`, `_decorationService`, `_logService`, `_themeService`, plus
`renderService.setRenderer()`/`handleResize()`). All of these still exist, unrenamed, in the
compiled `@xterm/xterm@6.0.0` bundle (confirmed via `grep` against
`node_modules/.pnpm/@xterm+xterm@6.0.0/.../xterm.js`), and a real (unmocked)
`CanvasAddon` from the installed `@xterm/addon-canvas@0.7.0` activates, resizes, writes, and
disposes cleanly against a real (unmocked) `@xterm/xterm@6.0.0` `Terminal` in a jsdom
environment with a minimal fake 2D canvas context standing in for the browser's real one (jsdom
has no real 2D context at all — the same limitation this codebase already works around for
`@xterm/addon-serialize`). See
`web-app/src/components/sessions/__tests__/XtermTerminal.canvasAddonXterm6Compat.test.ts` for
the runnable proof, which will fail loudly if a *future* `@xterm/xterm` bump breaks this wiring
(addon-canvas itself will never be updated again, since it no longer exists upstream to update).

**Conclusion**: the peer-dependency warning is stale metadata, not a signal of actual
incompatibility — `@xterm/xterm@^6.0.0` stays in `web-app/package.json`, `@xterm/addon-canvas`
stays pinned at `^0.7.0` (the only version that will ever exist), and no code change to the
fallback path in `XtermTerminal.tsx` is required. This is explicitly a point-in-time finding,
not a standing guarantee — any future `@xterm/xterm` major bump must re-run (or extend) this
same verification before assuming the Canvas fallback tier still works, since upstream is not
maintaining this compatibility on our behalf.
