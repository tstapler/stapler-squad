# Implementation Plan: terminal-resize-fit-loop

**Feature**: Close the ResizeObserver → `fit()` → `terminal.onResize` → resize-RPC feedback loop
so it converges on real value changes and self-heals (via a WebGL→default-renderer fallback)
when a sub-pixel glyph-metric wobble prevents convergence.
**Date**: 2026-07-27
**Status**: Ready for implementation
**ADRs**: `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md`

---

## Step 0.5 — Creative Pass: candidate approaches for AC2+AC3+AC4

**Approach A — Layered pure-function gates** (chosen). Three independent pure predicate
functions (`shouldFit`, `shouldSendResize`, `shouldAbandonWebgl`) are inserted at the three
existing points in the control flow that already do the relevant measurement/dispatch work
(ResizeObserver's post-debounce callback, `useTerminalFlowControl.resize()`, and
`terminal.onResize`'s dedup branch). Each gate closes exactly one AC and is independently unit
testable with no DOM/React dependency.
- *Strength*: minimal blast radius — extends the exact code paths research already traced
  line-by-line, matching the codebase's existing small-pure-file convention
  (`cellDimensions.ts`, `mouseTracking.ts`).
- *Weakness*: three independently-invoked gates instead of one funnel means a future new
  `resize()` call site (a 4th caller) could bypass the AC3 gate if a contributor doesn't know
  the convention exists — mitigated by the regression test in Story 3.1.2 and by keeping all
  three functions colocated in one file with one set of docs.

**Approach B — Central resize state machine.** A single explicit state machine
(idle/pending/converged/oscillating) owned by one hook, that all resize triggers (mount,
ResizeObserver, font-size/font-family effects, imperative `ref.fit()`, `StateApplicator`) must
dispatch actions into, deciding fit/send/fallback centrally.
- *Strength*: single source of truth; eliminates the "three independently-tuned timers" problem
  noted in `research/pitfalls.md` §2 (150ms ResizeObserver debounce, 200ms RPC throttle, 300-400ms
  `isFittingRef` guard) by unifying them.
- *Weakness*: requires rewiring 5+ existing call sites that already work correctly per the
  R1.2/R1.3 debounce fix (`research/build-vs-buy.md` §4) — high regression risk against AC6, and
  directly contradicts the "adapt, don't fork wholesale" verdict in `build-vs-buy.md`.

**Approach C — Reactive stream pipeline (RxJS).** Model the resize event stream as an
`Observable`, use `distinctUntilChanged`/`bufferTime`/`scan` operators to implement dedup and
burst detection declaratively.
- *Strength*: dedup/burst-window logic composes from well-tested operators instead of hand-rolled
  timestamp-array bookkeeping.
- *Weakness*: introduces a new dependency and paradigm never used elsewhere in this codebase for
  a ~30-50 line problem — directly contradicted by `research/build-vs-buy.md`'s verdict ("Build,
  not buy... no npm package fits").

**Decision: Approach A.** See Pattern Decisions table below for the corresponding
alternative-rejected entries.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `TerminalSize` | `{ cols: number; rows: number }` — the shared domain type for a terminal grid size. | Added per architecture-review.md Concern #1: prevents the loose-positional-`number` transposition hazard (`cols`/`rows` or proposed/current swapped silently type-checking). Used by `shouldFit`, `shouldSendResize`, `ResizeEvent`, and `lastSentSizeRef`. |
| `shouldFit(proposed, current)` | Pure predicate: `proposed: Partial<TerminalSize> \| undefined`, `current: TerminalSize`. True iff `FitAddon.proposeDimensions()`'s integer output differs from the terminal's **live** `cols`/`rows`. Returns `false` if either proposed value is `undefined` (pre-layout). | AC2 gate. Baseline is `terminal.cols`/`terminal.rows`, never a fit()-only ref (see Pattern Decisions). |
| `shouldSendResize(next, lastSent)` | Pure predicate: `next: TerminalSize`, `lastSent: TerminalSize \| null`. True iff `next` differs from `lastSent` (or `lastSent` is `null`). | AC3 gate. Independent of, and applied in addition to, the existing 200ms `THROTTLE_MS` time-throttle. |
| `shouldAbandonWebgl(history, now, windowMs = 2000, threshold = 3)` | Pure predicate: true iff the most recent `{cols, rows}` entry in `history` recurs `>= threshold` times within `windowMs` of `now`. | AC4 oscillation/burst detector. Renamed from research's working name `shouldFallbackToCanvas` — see "canvas → default renderer" terminology correction below. |
| `ResizeEvent` | `TerminalSize & { at: number }` — one entry in the oscillation history. | Pushed only when `terminal.onResize` fires with a genuinely new value (i.e., post-xterm's-own-dedup), never on raw observer/onResize noise. |
| `oscillationHistory` | Effect-scoped `ResizeEvent[]` (a local variable inside the mount effect, **not** a component-level `useRef`). | Scoping choice is deliberate — see Pattern Decisions ("StrictMode / scrollback-remount leak" row). |
| `burst window` | The 2000ms rolling window `shouldAbandonWebgl` prunes `history` against. | Matches AC4's literal wording ("recurring ≥3 times within a rolling 2000ms window"). |
| `webglAddonRef` | New `useRef<WebglAddon | null>(null)` in `XtermTerminal.tsx`, set once the WebGL addon successfully loads inside the mount-time async IIFE (guarded by a `cancelled` liveness flag — see Pattern Decisions), nulled on dispose (either via existing `onContextLoss` or the new oscillation trip). | Makes the WebGL addon instance reachable outside its IIFE closure — today it is unreachable (research finding). |
| `webglFallbackTrippedRef` | New `useRef(false)` in `XtermTerminal.tsx`. Set `true` the first time the oscillation detector successfully disposes WebGL; never reset for the life of the mount. | Added per architecture-review.md Concern #2: distinguishes "WebGL never loaded" (AC4's `console.error` backstop case) from "already fell back this session" (a later burst is expected and should not re-log the backstop error). |
| default (DOM) renderer | What xterm.js actually falls back to when `webglAddon.dispose()` is called and no other accelerated renderer addon is loaded. | Corrects AC4's literal "canvas renderer" wording — `@xterm/addon-canvas` is deprecated/incompatible with the pinned `@xterm/xterm ^6.0.0`. See ADR-018. |
| `lastSentSizeRef` | New `useRef<TerminalSize \| null>(null)` inside `useTerminalFlowControl`, alongside the existing `lastResizeTimeRef`. Tracks the last `(cols, rows)` value actually pushed via a `TerminalResize` RPC. | Distinct from `TerminalOutput.tsx`'s `lastResizeRef` (client-observed size) and from the existing dead-code `dimensionSyncRef` — do not conflate any of the three. |
| `force` (resize bypass parameter) | New optional 3rd parameter on `resize(cols, rows, force?)`. When `true`, skips **both** the 200ms time-throttle and the AC3 value-dedup check, guaranteeing delivery, and unconditionally updates `lastResizeTimeRef`/`lastSentSizeRef.current` after sending. **CORRECTED post-implementation (Gate 2 code review, CRITICAL finding)**: the original design bypassed only the value-dedup, not the time-throttle — but both real callers need *guaranteed* delivery (reconnect-resync, manual force-resize), and a partial bypass let the send be silently dropped if it fired within 200ms of a prior send, exactly the failure class this fix exists to prevent. | Required so the 2 non-standard callers (reconnect-resync, manual force-resize) keep working under a naive value-dedup *and* aren't silently swallowed by the pre-existing time-throttle. |
| dedup | Value-based skip (same value → no-op), as opposed to throttle (time-based skip regardless of value). | AC3 requires both to independently apply. |

14 glossary terms.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| AC2/AC3/AC4 overall architecture | Layered pure guard functions (Strategy-shaped predicates), inserted into existing call sites | `research/architecture.md` §3, `research/build-vs-buy.md` §1d | Central resize state machine (Approach B) | Requires rewiring 5+ already-working `fit()` call sites; high AC6 regression risk; contradicts "adapt, don't fork wholesale" verdict |
| Burst/oscillation counting | Sliding-window log (timestamp array; prune-then-count) | `research/build-vs-buy.md` §3 | RxJS observable pipeline (Approach C) | New dependency/paradigm never used in this codebase for a ~30-line problem |
| Burst/oscillation counting | Sliding-window log | `research/build-vs-buy.md` §3 | Fixed-size ring-buffer counter (no timestamps) | Loses the "within 2000ms" recency requirement the AC explicitly specifies; would still need expiry logic, no simpler |
| AC2 comparison baseline | Live `terminal.cols`/`terminal.rows` | `research/pitfalls.md` §1 | `lastFitDimensionsRef` (fit()-only ref) | `StateApplicator.applyDimensionChange()` (`StateApplicator.ts:562`) mutates `terminal.cols/rows` independent of `fit()`; a fit()-only ref goes stale and can silently swallow a legitimate resize (AC6 violation risk) |
| AC4 renderer fallback | Reuse `webglAddon.dispose()` → xterm's default DOM renderer (no new addon load) | `research/stack.md` §3, `research/pitfalls.md` §4 | Add `@xterm/addon-canvas` as a new dependency | Deprecated upstream and incompatible with the pinned `@xterm/xterm ^6.0.0`; not a current dependency |
| Oscillation detector hook point | `terminal.onResize` (the common funnel all resize sources route through) | `research/pitfalls.md` §2 (explicit finding), corroborating `research/architecture.md` §4 | Hook only inside the `ResizeObserver` debounced callback | `fit()` is called from 5+ sites (mount, ResizeObserver, fontSize/fontFamily effects, imperative `ref.fit()`); hooking only the ResizeObserver path would miss oscillations from those, and window/pane-divider dragging risks false-triggering if raw (non-funneled) events were counted instead |
| Pure function location | New file `web-app/src/lib/terminal/resizeConvergence.ts` | `research/architecture.md` §3 | Inline in `XtermTerminal.tsx` / `useTerminalFlowControl.ts` | Not independently unit-testable without mounting the full component/hook; AC5 explicitly requires unit tests for the extracted pure functions |
| Oscillation history storage | Effect-scoped local array (mirrors existing `lastContainerSize`/`resizeTimeout` pattern) | `research/pitfalls.md` §6 | Component-level `useRef` | A top-level ref survives a genuine `[scrollback]`-triggered effect re-run (full terminal teardown/recreate) and would carry stale burst history into the new instance, risking a spurious immediate fallback trigger |
| `resize()` dedup bypass | `force?: boolean` 3rd parameter | `research/architecture.md` §1d (required design finding) | Naive unconditional value-dedup with no escape hatch | Breaks 2 of the 3 existing callers (reconnect-resync at `TerminalOutput.tsx:664`, manual force-resize at `:1160`) — a functional regression outside the 7 stated ACs but discoverable by tracing callers |
| `shouldFit`/`shouldSendResize` parameter shape | `TerminalSize` object type | `architecture-review.md` Concern #1 (type-driven-design lens) | Four/three loose positional `number` params | Positional `(proposedCols, proposedRows, currentCols, currentRows)` type-checks fine if the caller transposes proposed/current or cols/rows — a silent wrong-gating-decision bug, not a compile error; an object shape makes transposition a named-field mismatch instead |
| `webglAddonRef` write timing | `cancelled` liveness flag closed over by the async IIFE, checked before every ref/terminal mutation | `architecture-review.md` Blocker (async-load race) | Unguarded persistent `useRef` written directly from the IIFE | The WebGL addon load is asynchronous, unlike the other addons loaded synchronously in the same tick; on a StrictMode double-invoke or fast `[scrollback]` remount, a stale mount's IIFE can resolve *after* the live mount's (module-cache-driven near-instant resolution) and silently overwrite the ref with an addon bound to an already-disposed terminal |
| WebGL "already fell back" state | `webglFallbackTrippedRef` (separate from `webglAddonRef.current === null`) | `architecture-review.md` Concern #2 | Reusing `webglAddonRef.current === null` to mean both "never loaded" and "already disposed" | The two states need different backstop behavior — "never loaded" is the AC4-required `console.error`; "already disposed by an earlier oscillation" should not repeat that error on every subsequent burst |

---

## Migration Plan
(Omit — no schema/data changes)

## Observability Plan
- **Logs**: `console.log`/`console.warn`/`console.error`, following the existing
  `[XtermTerminal]`/`[useTerminalFlowControl]`-prefixed convention already used pervasively in
  both files (15 existing console sites in `XtermTerminal.tsx` per `research/ux.md`). New sites:
  "Skipping fit(): proposed dims match current cols/rows" (AC2), "Resize skipped: (cols x rows)
  matches last sent value" (AC3), "Resize oscillation detected... falling back to default
  renderer" (AC4, `console.warn`) and "...but no WebGL addon to dispose" (AC4 backstop,
  `console.error`, per the AC4 acceptance criterion's explicit wording).
- **Metrics**: None. No frontend telemetry/analytics pipeline exists in `web-app/src/lib/`
  (confirmed in `research/ux.md`) and adding one is out of scope for a targeted bug fix.
- **Alerts**: None — client-side only, no backend/on-call surface change (out of scope per
  requirements.md: "Server-side (Go) resize RPC handling changes").

## Risk Control
- **Feature flag**: None. These are precision gates that strictly narrow when `fit()`/RPC/WebGL-
  dispose already-existing code paths fire — they cannot introduce new user-visible behavior, only
  suppress redundant work. The AC6 no-regression test (Story 4.2.3) is the safety net instead of a
  runtime flag; do not merge without it green.
- **Rollback procedure**: `resizeConvergence.ts` is purely additive; all integration changes are
  precise, small edits at documented file:line locations. A single `git revert` of the PR fully
  restores prior behavior.
- **Staged rollout**: N/A — client-side web app shipped via `make install-service`; no gradual/
  canary rollout mechanism exists in this repo. Land after `make ci` (build + test + lint) passes.

## Unresolved Questions
None. The two candidate ambiguities surfaced during research — (1) whether the WebGL fallback
should be sticky/persisted across sessions, and (2) AC4's literal "canvas renderer" wording — are
both resolved in this plan and in ADR-018: (1) session-scoped only, by construction (WebGL only
loads once per mount; no persistence mechanism is added — see `research/architecture.md` §4), and
(2) "canvas" is corrected to "xterm.js's default (non-WebGL) DOM renderer" throughout the plan,
code comments, and ADR.

## Dependency Visualization
```
resizeConvergence.ts (Phase 1 — pure fns + unit tests)
        |
        |-----------------------------------------------------------.
        v                                                            v
XtermTerminal.tsx: webglAddonRef +                        useTerminalFlowControl.ts:
webglFallbackTrippedRef plumbing,                          lastSentSizeRef + force param (3.1)
cancelled-guard (2.1)                                                |
        |                                                            |
        v                                                            |
XtermTerminal.tsx: AC2 gate in ResizeObserver->fit() (2.2)           |
        |                                                            |
        |-----------------------------.                              |
        v                              v                             |
XtermTerminal.tsx: AC4 detector    XtermTerminal.tsx: AC1/AC7 gate   |
at terminal.onResize (2.3)         on imperative fit() (2.4)         |
        |                              |                             v
        |                              |          TerminalOutput.tsx: 2 callers pass
        |                              |          force=true + regression test (3.2, incl. 3.1.2c)
        |                              |                             |
        '------------------.       .--'       .--------------------'
                             v       v         v
                        Test coverage (Phase 4): resizeConvergence.test.ts,
                        useTerminalFlowControl.test.ts additions,
                        new XtermTerminalResize.test.tsx (AC1/AC4/AC5/AC6/AC7)
                                       |
                                       v
                        ADR-018 + docs/adr stub (Phase 5.1)
                                       |
                                       v
                        Manual verification hard gate AC1/AC7 (Phase 5.2)
```

---

## Phase 1: Foundation — Resize Convergence Domain

### Epic 1.1: `resizeConvergence.ts` pure decision functions
**Goal**: Provide the three pure predicates (`shouldFit`, `shouldSendResize`,
`shouldAbandonWebgl`) that every downstream gate calls, fully unit-testable in isolation.

#### Story 1.1.1: Implement `shouldFit` and `shouldSendResize`
**As a** maintainer, **I want** integer-dimension and value-dedup logic extracted into pure
functions, **so that** AC2 and AC3 can be unit tested without mounting React components or
mocking xterm.js internals.
**Acceptance Criteria**:
- `shouldFit` returns `false` when `proposed.cols`/`proposed.rows` equal the live `current.cols`/
  `current.rows`, and `false` (not `true`) when either proposed value is `undefined`.
  - *Given* `terminal.cols=84`, `terminal.rows=60`, and `FitAddon.proposeDimensions()` returns
    `{cols: 84, rows: 60}` after a 0.3px container wobble (the WebGL glyph-metric mismatch case
    from `research/pitfalls.md` — `Actual pixels per column: 8.45px` vs `Expected: 8.33px`),
    *When* `shouldFit({cols:84,rows:60}, {cols:84,rows:60})` is evaluated, *Then* it returns
    `false`.
- `shouldSendResize` returns `true` when `lastSent` is `null` or differs from `next`.
  - *Given* `lastSent = {cols: 100, rows: 30}`, *When*
    `shouldSendResize({cols:100,rows:30}, lastSent)` is evaluated, *Then* it returns `false`; and
    *When* `shouldSendResize({cols:110,rows:35}, lastSent)` is evaluated, *Then* it returns `true`.
**Files**: `web-app/src/lib/terminal/resizeConvergence.ts`

##### Task 1.1.1a: Create `resizeConvergence.ts` with `TerminalSize` + `shouldFit` (~4 min)
- Create `web-app/src/lib/terminal/resizeConvergence.ts`.
- Export:
  ```ts
  export interface TerminalSize {
    cols: number;
    rows: number;
  }

  export function shouldFit(
    proposed: Partial<TerminalSize> | undefined,
    current: TerminalSize,
  ): boolean {
    if (proposed?.cols === undefined || proposed?.rows === undefined) return false;
    return proposed.cols !== current.cols || proposed.rows !== current.rows;
  }
  ```
  (`Partial<TerminalSize>` matches `FitAddon.proposeDimensions()`'s actual return shape — callers
  can pass the addon's return value directly with no field-by-field unpacking, which is also what
  closes architecture-review.md Concern #1's transposition hazard: there's no longer a
  `(proposedCols, proposedRows, currentCols, currentRows)` positional order to get wrong.)
- Files: `web-app/src/lib/terminal/resizeConvergence.ts`

##### Task 1.1.1b: Add `shouldSendResize` (~3 min)
- In the same file, add:
  ```ts
  export function shouldSendResize(
    next: TerminalSize,
    lastSent: TerminalSize | null,
  ): boolean {
    return lastSent === null || lastSent.cols !== next.cols || lastSent.rows !== next.rows;
  }
  ```
- Files: `web-app/src/lib/terminal/resizeConvergence.ts`

#### Story 1.1.2: Implement `ResizeEvent` + `shouldAbandonWebgl`
**As a** maintainer, **I want** a rolling-window burst detector extracted as a pure function,
**so that** AC4's oscillation/renderer-fallback trigger condition is independently verifiable and
free of `Date.now()`/DOM coupling in its core logic.
**Acceptance Criteria**:
- `shouldAbandonWebgl` returns `true` iff the most recent entry's `(cols, rows)` recurs `>=
  threshold` (default 3) times within `windowMs` (default 2000) of `now`.
  - *Given* `history = [{cols:84,rows:60,at:1000}, {cols:85,rows:60,at:1400}, {cols:84,rows:60,at:1900}]`
    and a new resize `{cols:84,rows:60,at:2300}` is appended before the check, *When*
    `shouldAbandonWebgl(history, 2300)` is evaluated, *Then* it returns `true` (three `{84,60}`
    entries — at 1000, 1900, 2300 — all within the 2000ms window ending at 2300; `85` at 1400 does
    not match the most recent value and is excluded).
  - *Given* the same three-entry history but the new entry arrives at `at:3200` (2200ms after the
    oldest `{84,60}` entry at 1000), *When* `shouldAbandonWebgl(history, 3200)` is evaluated,
    *Then* it returns `false` (the 1000ms entry has aged out of the 2000ms window, leaving only 2
    matching entries).
**Files**: `web-app/src/lib/terminal/resizeConvergence.ts`

##### Task 1.1.2a: Add `ResizeEvent` type + `shouldAbandonWebgl` (~5 min)
- In the same file, add:
  ```ts
  export interface ResizeEvent extends TerminalSize {
    at: number;
  }

  export function shouldAbandonWebgl(
    history: ResizeEvent[],
    now: number,
    windowMs = 2000,
    threshold = 3,
  ): boolean {
    const recent = history.filter((e) => now - e.at <= windowMs);
    if (recent.length === 0) return false;
    const last = recent[recent.length - 1];
    const matches = recent.filter((e) => e.cols === last.cols && e.rows === last.rows);
    return matches.length >= threshold;
  }
  ```
- Document in a file-header comment that "canvas" is not literally what this triggers — dispose
  falls back to xterm.js's default DOM renderer (link to ADR-018).
- Files: `web-app/src/lib/terminal/resizeConvergence.ts`

### Epic 1.2: Unit tests for `resizeConvergence.ts`
**Goal**: AC5's "unit tests for the extracted pure decision functions" requirement.

#### Story 1.2.1: `shouldFit` unit tests
**Acceptance Criteria**:
- Covers: proposed equals current (false), proposed differs on cols only (true), proposed differs
  on rows only (true), proposed is `undefined` on either axis (false).
**Files**: `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts`

##### Task 1.2.1a: Write `shouldFit` test cases (~4 min)
- Create `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts`; add
  `describe('shouldFit', ...)` with 4 cases per the acceptance criteria above, using the exact
  `{84,60}` values from Story 1.1.1's Given-When-Then.
- Files: `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts`

#### Story 1.2.2: `shouldSendResize` unit tests
**Acceptance Criteria**:
- Covers: `lastSent === null` (true), `lastSent` equals `(cols,rows)` (false), `lastSent` differs
  on cols only (true), differs on rows only (true).
**Files**: `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts`

##### Task 1.2.2a: Write `shouldSendResize` test cases (~3 min)
- Add `describe('shouldSendResize', ...)` with 4 cases per the acceptance criteria.
- Files: `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts`

#### Story 1.2.3: `shouldAbandonWebgl` unit tests
**Acceptance Criteria**:
- Covers: exactly 3 recurrences within window (trips), 3 recurrences spanning >2000ms where the
  oldest ages out (does not trip), a boundary case at exactly `now - e.at === windowMs` (document
  the inclusive `<=` convention explicitly and assert it), and an alternating A/B/A/B/A sequence
  where only the most-recent value's count matters (per `research/build-vs-buy.md` §3's required
  boundary tests).
**Files**: `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts`

##### Task 1.2.3a: Write `shouldAbandonWebgl` test cases (~5 min)
- Add `describe('shouldAbandonWebgl', ...)` with the 4 cases above, reusing the exact
  `{84,60}`/`{85,60}` values and timestamps from Story 1.1.2's Given-When-Then.
- Files: `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts`

---

## Phase 2: `XtermTerminal.tsx` Integration (AC2, AC4)

### Epic 2.1: Make the WebGL addon reachable outside its mount-time closure
**Goal**: Today `webglAddon` is a `const` local to the async IIFE at lines 264-281 — nothing else
in the component can reach it. AC4's oscillation detector (Epic 2.3) needs to call `.dispose()`
on it from the `terminal.onResize` handler at lines 433-440, a different closure in the same
effect.

#### Story 2.1.1: Add `webglAddonRef`
**As a** maintainer, **I want** the loaded `WebglAddon` instance exposed via a ref parallel to
`fitAddonRef`, **so that** the oscillation detector (and any future consumer) can dispose it and
so double-dispose races with the existing `onContextLoss` handler are guarded.
**Acceptance Criteria**:
- `webglAddonRef.current` is non-null after a successful WebGL load and `null` after disposal via
  either the existing `onContextLoss` handler or the new oscillation path.
**Files**: `web-app/src/components/sessions/XtermTerminal.tsx`

##### Task 2.1.1a: Declare `webglAddonRef` + `webglFallbackTrippedRef` (~2 min)
- Add `import type { WebglAddon } from "@xterm/addon-webgl";` near the top imports (type-only —
  the runtime addon is still loaded via the existing dynamic `import('@xterm/addon-webgl')` at
  line 267, unchanged).
- After `const lastSizeRef = useRef<{ cols: number; rows: number } | null>(null);` (line 105), add
  `const webglAddonRef = useRef<WebglAddon | null>(null);` and
  `const webglFallbackTrippedRef = useRef(false);` (distinguishes "WebGL never loaded" from
  "already fell back via oscillation this session" — see Domain Glossary and Epic 2.3).
- Files: `web-app/src/components/sessions/XtermTerminal.tsx`

##### Task 2.1.1b: Set/null the ref in the IIFE, `onContextLoss`, and cleanup — with a liveness guard against StrictMode/scrollback-remount races (~6 min)
- **BLOCKER fix (architecture-review.md Blocker)**: the async `import('@xterm/addon-webgl')` IIFE
  is not synchronous with effect mount/cleanup, unlike the other addons (loaded synchronously in
  the same tick). On a StrictMode dev double-invoke, or a fast `[scrollback]`-triggered remount
  while the addon chunk is still downloading, a stale (now-disposed) mount's IIFE can resolve
  *after* the live mount's — because the second `import()` call typically resolves near-instantly
  from the module cache — and silently overwrite `webglAddonRef.current` with an addon loaded onto
  the already-disposed terminal. Guard with a `cancelled` flag scoped to this effect run:
  - Immediately before the async IIFE (near line 264), add `let cancelled = false;`.
  - Inside the IIFE, immediately after
    `const { WebglAddon } = await import('@xterm/addon-webgl');`, add `if (cancelled) return;` —
    before constructing or loading the addon at all, so a cancelled mount never calls
    `terminal.loadAddon()` on a disposed terminal in the first place.
  - After `terminal.loadAddon(webglAddon);` (line 273), add `webglAddonRef.current = webglAddon;`
    (this line only runs when the guard above didn't already return).
  - Update the `onContextLoss` handler (lines 269-272) to also null the ref after disposing:
    ```ts
    webglAddon.onContextLoss(() => {
      console.warn('[XtermTerminal] WebGL context lost, falling back to default renderer');
      webglAddon.dispose();
      webglAddonRef.current = null;
    });
    ```
    (Note the message wording change: "canvas renderer" → "default renderer" per ADR-018.)
  - Also fix the sibling `console.log("[XtermTerminal] WebGL2 unavailable (Android?), using canvas
    renderer")` at line 279 to say "default renderer" — same terminology correction, same file,
    trivial to miss otherwise (flagged in adversarial-review.md Minors).
  - In the effect cleanup block (lines 511-525), add `cancelled = true;` as the **first** line of
    the cleanup function (before any other cleanup runs), plus `webglAddonRef.current = null;`
    alongside the other addon-ref nulling (`fitAddonRef.current = null;` etc.). Do **not** reset
    `webglFallbackTrippedRef.current` on cleanup — it is fine (and irrelevant) if it's stale on a
    disposed instance since a new mount gets a fresh ref via `useRef`'s per-render-tree identity.
  - Add a StrictMode regression test (Task 4.2.1b, new — see Epic 4.2) asserting `webglAddonRef`
    ends up pointing at the *second* (real) mount's addon instance, not the first (throwaway) one,
    after both IIFEs resolve.
- Files: `web-app/src/components/sessions/XtermTerminal.tsx`

### Epic 2.2: AC2 — gate `fit()` on live `terminal.cols`/`terminal.rows`
**Goal**: Replace the unconditional `fitAddonRef.current?.fit()` call inside the ResizeObserver's
debounced double-rAF (line 490) with a `shouldFit`-gated call, comparing against the terminal's
**live** current dimensions (not a fit()-only ref, per the Pattern Decisions row above).

#### Story 2.2.1: Gate the debounced `fit()` call
**As a** user with multiple terminal tabs open, **I want** `fit()` to be skipped when the proposed
integer grid hasn't actually changed, **so that** a sub-cell WebGL glyph-metric wobble cannot keep
re-triggering the debounce→rAF→fit() cycle.
**Acceptance Criteria**:
- `fit()` is only called when `proposeDimensions()`'s integer output differs from
  `terminal.cols`/`terminal.rows` at the moment the debounce fires.
  - *Given* `terminal.cols=84`, `terminal.rows=60` and `FitAddon.proposeDimensions()` returns
    `{cols:84,rows:60}` after a 0.3px container wobble, *When* the ResizeObserver's 150ms debounce
    elapses and the double-rAF fires, *Then* `shouldFit({cols:84,rows:60}, {cols:84,rows:60})`
    returns `false` and `fitAddonRef.current.fit()` is never called for that cycle (only a log
    line is emitted).
**Files**: `web-app/src/components/sessions/XtermTerminal.tsx`,
`web-app/src/lib/terminal/resizeConvergence.ts` (import only)

##### Task 2.2.1a: Replace the unconditional `fit()` call with a `shouldFit`-gated call (~5 min)
- Add `import { shouldFit } from "@/lib/terminal/resizeConvergence";` near the top imports.
- Replace line 490 (`fitAddonRef.current?.fit();`) inside the nested double-`requestAnimationFrame`
  callback (lines 488-489) with:
  ```ts
  const term = terminalRef.current;
  const addon = fitAddonRef.current;
  if (term && addon) {
    const proposed = addon.proposeDimensions();
    if (shouldFit(proposed, { cols: term.cols, rows: term.rows })) {
      addon.fit();
    } else {
      console.log('[XtermTerminal] Skipping fit(): proposed dims match current cols/rows');
    }
  }
  ```
  (`proposed` is passed straight through from `addon.proposeDimensions()` — no field unpacking, no
  positional-argument transposition risk.)
- Leave the existing post-fit `lastContainerSize` re-sync (lines 494-497) and the
  `AFTER fit` console.log (line 498) as-is — they read `terminalRef.current?.cols/rows` and remain
  correct whether or not `fit()` actually ran this cycle.
- Files: `web-app/src/components/sessions/XtermTerminal.tsx`

### Epic 2.3: AC4 — oscillation/burst detector hooked at the `terminal.onResize` funnel
**Goal**: Per the explicit funnel finding, hook the detector at `terminal.onResize`
(lines 433-440) — the point every resize source (ResizeObserver, font-size/font-family effects,
imperative `ref.fit()`, `StateApplicator.applyDimensionChange()`) already routes through, and
which already only fires on a genuine value change (the existing `lastSizeRef` guard). This means
the history buffer is naturally fed only "post-dedup" applications, avoiding the window/pane-drag
false-positive risk documented in `research/pitfalls.md` §3.

#### Story 2.3.1: Record history and trip the WebGL fallback
**As a** user hitting the WebGL sub-pixel glyph-metric wobble, **I want** the terminal to
automatically abandon WebGL after 3 identical/alternating resizes in 2 seconds, **so that** the
oscillation cannot peg CPU indefinitely even if the underlying pixel-vs-cell mismatch is never
root-caused.
**Acceptance Criteria**:
- History is scoped per-effect-run (a local variable, not a component-level `useRef` — see Pattern
  Decisions), pushed only inside the existing `lastSize` value-changed branch.
  - *Given* `oscillationHistory` accumulates `[{cols:84,rows:60,at:t}, {cols:85,rows:60,at:t+400},
    {cols:84,rows:60,at:t+900}]` from three real `terminal.onResize` firings, and a fourth
    genuinely-new-value firing `{cols:84,rows:60}` arrives at `t+1300`, *When* the history is
    pushed and `shouldAbandonWebgl(history, t+1300)` is evaluated, *Then* it returns `true`
    (three `{84,60}` entries within the 2000ms window), `webglAddonRef.current?.dispose()` is
    called, a `console.warn('[XtermTerminal] Resize oscillation detected (cols/rows repeated 3x
    in <2000ms), falling back to default renderer')` is logged, `webglAddonRef.current` is set to
    `null`, `webglFallbackTrippedRef.current` is set to `true`, and `oscillationHistory` is cleared
    so the fallback does not re-trigger on the next matching resize.
  - *Given* `webglAddonRef.current` is already `null` **and** `webglFallbackTrippedRef.current` is
    `false` (WebGL genuinely never loaded — e.g. no `WebGL2RenderingContext`, or the dynamic
    `import()` failed) when `shouldAbandonWebgl` trips, *When* the dispose branch runs, *Then*
    `console.error('[XtermTerminal] Resize oscillation detected but no WebGL addon to dispose')`
    is logged instead of throwing (the AC4-required backstop).
  - *Given* a **second** oscillation burst occurs later in the same session, after
    `webglFallbackTrippedRef.current` was already set `true` by an earlier fallback (i.e. WebGL
    was disposed, but the underlying pixel/glyph mismatch this fix does not root-cause keeps
    producing bursts even on the default renderer), *When* `shouldAbandonWebgl` trips again, *Then*
    the backstop branch does **not** re-log `console.error` (the addon is legitimately gone, not
    missing-by-surprise) — instead a single `console.log('[XtermTerminal] Resize oscillation
    persists after WebGL fallback')` is emitted, so a still-oscillating post-fallback session
    doesn't spam the console with a misleading repeated error (architecture-review.md Concern #2 /
    adversarial-review.md Minor).
  - *Given* `terminalRef.current` is `null` (component torn down) at the moment the oscillation
    check would run, *When* the `resizeDisposable` callback executes, *Then* the dispose branch is
    never reached at all — the callback's existing outer guard (mirroring the pattern at the top of
    the `ResizeObserver` callback: `if (!fitAddonRef.current || !terminalRef.current) return;`)
    prevents calling `.dispose()` against a torn-down instance (closes
    adversarial-review.md's xterm.js#5181 double-dispose/teardown-race concern from
    `research/pitfalls.md` §4).
**Files**: `web-app/src/components/sessions/XtermTerminal.tsx`

##### Task 2.3.1a: Declare effect-scoped oscillation history + constants (~2 min)
- Import `shouldAbandonWebgl` and `ResizeEvent` from `@/lib/terminal/resizeConvergence` (extend
  the import added in Task 2.2.1a).
- Inside the mount effect, alongside `let lastContainerSize = ...` / `let resizeTimeout = ...`
  (lines 451-452), add:
  ```ts
  let oscillationHistory: ResizeEvent[] = [];
  const OSCILLATION_WINDOW_MS = 2000;
  const OSCILLATION_THRESHOLD = 3;
  ```
- Files: `web-app/src/components/sessions/XtermTerminal.tsx`

##### Task 2.3.1b: Push history + trip fallback inside `resizeDisposable` (~6 min)
- Replace the `resizeDisposable` callback body (lines 433-440) with:
  ```ts
  const resizeDisposable = terminal.onResize(({ cols, rows }) => {
    if (!terminalRef.current) return; // torn down — never dispose against a dead instance

    const lastSize = lastSizeRef.current;
    if (!lastSize || lastSize.cols !== cols || lastSize.rows !== rows) {
      lastSizeRef.current = { cols, rows };

      const now = Date.now();
      oscillationHistory.push({ cols, rows, at: now });
      oscillationHistory = oscillationHistory.filter((e) => now - e.at <= OSCILLATION_WINDOW_MS);

      if (shouldAbandonWebgl(oscillationHistory, now, OSCILLATION_WINDOW_MS, OSCILLATION_THRESHOLD)) {
        if (webglAddonRef.current) {
          console.warn(
            `[XtermTerminal] Resize oscillation detected (cols/rows repeated ${OSCILLATION_THRESHOLD}x in <${OSCILLATION_WINDOW_MS}ms), falling back to default renderer`
          );
          webglAddonRef.current.dispose();
          webglAddonRef.current = null;
          webglFallbackTrippedRef.current = true;
        } else if (!webglFallbackTrippedRef.current) {
          console.error('[XtermTerminal] Resize oscillation detected but no WebGL addon to dispose');
        } else {
          console.log('[XtermTerminal] Resize oscillation persists after WebGL fallback');
        }
        oscillationHistory = [];
      }

      onResizeRef.current?.(cols, rows);
    }
  });
  ```
- Files: `web-app/src/components/sessions/XtermTerminal.tsx`

### Epic 2.4: AC1/AC7 — gate the imperative `fit()` entry point too
**Goal**: **Added per pre-mortem.md P1 #1.** The original plan left the imperative `fit()` method
exposed via `useImperativeHandle` (`XtermTerminal.tsx:614-616`, called by `TerminalOutput.tsx`'s
tab-visibility handler and its `visualViewport.resize` handler) ungated, reasoning it was outside
the 7 ACs' literal "ResizeObserver handler" wording. Pre-mortem correctly escalated this: AC1 and
AC7's own repro condition is "tab background/resume **or** window resize" — and tab
background/resume routes through this imperative path, not the `ResizeObserver`. Leaving it
ungated risks the fix eliminating CPU pegging only for the window-resize half of the repro, not
the backgrounded-tab half. Gating it is a ~5-line reuse of the exact same `shouldFit` predicate
already built in Phase 1 — cheap, and removes the residual risk outright rather than merely
documenting it.

#### Story 2.4.1: Gate the imperative `fit()` handle
**As a** user whose tab is backgrounded and resumed, **I want** the imperative `fit()` call
(triggered by visibility/viewport-change handlers, not the `ResizeObserver`) to skip redundant
work the same way the debounced path does, **so that** AC1/AC7's tab-background/resume trigger is
covered by the same convergence guarantee as the window-resize trigger.
**Acceptance Criteria**:
- The `fit` method exposed via `useImperativeHandle` only calls `fitAddonRef.current.fit()` when
  `shouldFit` returns `true` against the terminal's live `cols`/`rows` — identical gating logic to
  Task 2.2.1a, applied at this second call site.
  - *Given* `terminal.cols=84`, `terminal.rows=60` and `FitAddon.proposeDimensions()` returns
    `{cols:84,rows:60}`, *When* `TerminalOutput.tsx`'s tab-visibility handler calls `ref.fit()`
    (e.g. on tab resume) with no actual layout change, *Then* `shouldFit({cols:84,rows:60},
    {cols:84,rows:60})` returns `false` and `fitAddonRef.current.fit()` is not called.
  - *Given* the same setup but the container genuinely changed size while backgrounded (e.g.
    `visualViewport` reports a new size on resume) and `proposeDimensions()` now returns
    `{cols:90,rows:62}`, *When* `ref.fit()` is called, *Then* `shouldFit` returns `true` and
    `fitAddonRef.current.fit()` **is** called — no regression to the existing legitimate use of
    this path (mobile viewport changes, visibility restoration after a real size change).
**Files**: `web-app/src/components/sessions/XtermTerminal.tsx`

##### Task 2.4.1a: Gate the `useImperativeHandle` `fit` method (~3 min)
- In the `useImperativeHandle(ref, () => ({ ... }), [])` block (lines 595-629 in the original
  file), replace the `fit` method (currently `fit: () => { fitAddonRef.current?.fit(); },`) with:
  ```ts
  fit: () => {
    const term = terminalRef.current;
    const addon = fitAddonRef.current;
    if (term && addon) {
      const proposed = addon.proposeDimensions();
      if (shouldFit(proposed, { cols: term.cols, rows: term.rows })) {
        addon.fit();
      }
    }
  },
  ```
  (`shouldFit` is already imported at the top of the file from Task 2.2.1a — no new import
  needed.) No new console.log here — this path is called frequently enough during normal
  visibility/viewport handling that a log line per skip would be noisier than useful; the
  `ResizeObserver` path's existing log line is sufficient for debugging convergence.
- Files: `web-app/src/components/sessions/XtermTerminal.tsx`

---

## Phase 3: `useTerminalFlowControl.ts` + `TerminalOutput.tsx` Integration (AC3)

### Epic 3.1: Value-dedup with a bypassable `force` parameter

#### Story 3.1.1: Add `lastSentSizeRef` and gate `resize()`
**As a** maintainer, **I want** `resize()` to skip the RPC (and its follow-up
`currentPaneRequest`) when `(cols, rows)` matches the last value actually sent, independent of the
200ms time throttle, **so that** a value that recurs every 150-200ms (the exact cadence the
ResizeObserver debounce produces) doesn't keep re-sending after the throttle window reopens.
**Acceptance Criteria**:
- A repeated `(cols, rows)` call after the 200ms throttle window has elapsed does not send an RPC.
  - *Given* `lastSentSizeRef.current = {cols: 100, rows: 30}` (from a prior send) and 250ms have
    elapsed since that send (past the 200ms `THROTTLE_MS`), *When* `resize(100, 30)` is called
    again with no `force` argument, *Then*
    `shouldSendResize({cols:100,rows:30}, {cols:100,rows:30})` returns `false`, `pushMessage` is
    not called, and the 100ms-delayed `currentPaneRequest` follow-up is not scheduled.
- A genuinely new value is never deduped (regression guard for AC6's RPC-level requirement).
  - *Given* the same `lastSentSizeRef.current = {cols:100,rows:30}`, *When* `resize(150, 45)` is
    called, *Then* `shouldSendResize({cols:150,rows:45}, {...})` returns `true`, `pushMessage` is
    called exactly once, and `lastSentSizeRef.current` updates to `{cols:150,rows:45}`.
- `force=true` (the 2 non-standard callers) bypasses **both** the value-dedup and the time-throttle
  — **CORRECTED per Gate 2 code review (CRITICAL finding)**: the original design bypassed only
  the value-dedup, leaving `force` calls that land within 200ms of a prior send silently dropped
  by the pre-existing throttle — exactly defeating the guarantee both real callers need.
  - *Given* the client reconnects (`isConnected` transitions `false→true`) with
    `TerminalOutput.tsx`'s `lastResizeRef.current = {cols:100,rows:30}` — matching what
    `lastSentSizeRef.current` in the hook already holds from before the disconnect — *When* the
    reconnect effect calls `resize(100, 30, true)`, *Then* `pushMessage` **is** called despite the
    value matching `lastSentSizeRef.current`, because `force=true` skips the AC3 check; afterward
    `lastSentSizeRef.current` is (re-)set to `{cols:100,rows:30}`, which is what makes this
    "reset on reconnect" — the forced send always re-establishes the ref to the authoritative
    post-reconnect value rather than requiring a separate clear-to-null step.
  - *Given* `lastResizeTimeRef.current` was set 50ms ago (inside the 200ms `THROTTLE_MS` window —
    the exact scenario a reconnect or a fast double-click on the manual force-resize button can
    produce), *When* `resize(100, 30, true)` is called, *Then* `pushMessage` **is still called** —
    `force=true` bypasses the time-throttle check too, not just the value-dedup check. This is the
    regression this correction specifically closes: the original implementation would have hit the
    throttle's `return` before ever reaching the `force` check and silently dropped the send.
**Files**: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 3.1.1a: Add `lastSentSizeRef` + import `shouldSendResize` (~2 min)
- Add `import { shouldSendResize } from "@/lib/terminal/resizeConvergence";` near the top imports.
- After `const lastResizeTimeRef = useRef<number>(0);` (line 70), add:
  `const lastSentSizeRef = useRef<{ cols: number; rows: number } | null>(null);`.
  (Do not touch or repurpose `dimensionSyncRef` at line 71 — it is separate dead-code write-only
  state per `research/pitfalls.md` §1; leave it exactly as-is.)
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 3.1.1b: Update `resize()` signature + dedup check + ref update (~5 min)
- **CORRECTED per Gate 2 code review (CRITICAL finding, empirically verified)**: `force` must
  bypass BOTH the time-throttle and the value-dedup check, not just the latter. The original plan
  text below left the time-throttle check unconditional, which silently drops a forced send if it
  lands within 200ms of a prior send — exactly the failure mode the 2 real `force=true` callers
  (reconnect-resync, manual force-resize) need guaranteed immunity from.
- Change the signature at line 403 from `const resize = useCallback((cols: number, rows: number) => {`
  to `const resize = useCallback((cols: number, rows: number, force = false) => {`.
- Change the existing time-throttle check (lines 409-416) to also respect `force`:
  ```ts
  if (!force && timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
    console.log(`[useTerminalFlowControl] Resize throttled (${timeSinceLastResize}ms since last, need ${THROTTLE_MS}ms)`);
    return;
  }
  ```
  (i.e. add `!force &&` to the existing condition — this is the actual fix; everything else in
  this task is unchanged from the original plan.)
- After that (now `force`-aware) time-throttle check and before the `try` block (line 418),
  insert:
  ```ts
  if (!force && !shouldSendResize({ cols, rows }, lastSentSizeRef.current)) {
    console.log(`[useTerminalFlowControl] Resize skipped: (${cols}x${rows}) matches last sent value`);
    return;
  }
  ```
- Immediately after `lastResizeTimeRef.current = now;` (line 420), add
  `lastSentSizeRef.current = { cols, rows };`.
- Add a doc comment above the `resize` function explaining `force`'s guarantee (per
  architecture-review's specific request): "When true, bypasses both the send-throttle and the
  value-dedup check, guaranteeing the resize reaches the server even if it repeats the last-sent
  value or arrives within the throttle window. Use only for resync-critical call sites — silently
  dropping those leaves the server with stale dimensions with no user-visible signal."
- Add `lastSentSizeRef` to the `useCallback` dependency array... it is a ref (stable identity), so
  no dependency-array change is required — confirm the existing deps list at line 455 is unchanged.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

##### Task 3.1.1c: Update `UseTerminalFlowControlResult` interface (~1 min)
- Change `resize: (cols: number, rows: number) => void;` at line 25 to
  `resize: (cols: number, rows: number, force?: boolean) => void;`.
- Files: `web-app/src/lib/hooks/useTerminalFlowControl.ts`

#### Story 3.1.2: Update the 2 non-standard callers to pass `force=true`
**As a** maintainer, **I want** the reconnect-resync and manual force-resize call sites to keep
sending unconditionally, **so that** AC3's dedup doesn't regress reconnect behavior or the debug
force-resize action (neither of which is deduped by `TerminalOutput`'s own `sizeChanged` check
today).
**Acceptance Criteria**:
- Both call sites compile and pass `true` as the 3rd argument; the normal path (call site 1 at
  `TerminalOutput.tsx:640`, inside `handleTerminalResize`) is left unchanged (it stays
  caller-side-deduped by `sizeChanged` plus now also hook-side-deduped by AC3 — both apply, no
  conflict).
- Regression-tested: both call sites are asserted to pass `force=true` literally, not just
  "compiles" (per `validation.md`'s cross-check finding — `force` is an optional 3rd parameter, so
  a future edit that silently drops it would compile fine and only surface as AC3's dedup
  unexpectedly swallowing a legitimate reconnect/force-resize call, a hard-to-diagnose runtime bug
  rather than a build failure).
**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`,
`web-app/src/components/sessions/__tests__/TerminalOutputBug.test.tsx`

##### Task 3.1.2a: Reconnect-resync call site → `force=true` (~2 min)
- Change line 664 from `resize(currentSize.cols, currentSize.rows);` to
  `resize(currentSize.cols, currentSize.rows, true);`.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 3.1.2b: Manual force-resize call site → `force=true` (~2 min)
- Change line 1160 from `resize(cols, rows);` to `resize(cols, rows, true);`.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 3.1.2c: Regression test — both call sites pass `force=true` (~4 min)
- In `TerminalOutputBug.test.tsx`, using the existing `resize: jest.fn()` mock already wired at
  line 134 for `useTerminalFlowControl`, add two assertions: (1) triggering the reconnect-resync
  path (simulate `isConnected` transitioning `false→true`) asserts `resize` was called with
  `(currentSize.cols, currentSize.rows, true)` — 3rd arg literally `true`; (2) triggering the
  manual force-resize debug action asserts `resize` was called with `(cols, rows, true)`.
- Files: `web-app/src/components/sessions/__tests__/TerminalOutputBug.test.tsx`

---

## Phase 4: Test Coverage (AC3, AC5, AC6, AC4)

### Epic 4.1: `useTerminalFlowControl.test.ts` additions (AC3, AC6 RPC-level)

#### Story 4.1.1: Value-dedup and force-bypass tests
**Acceptance Criteria**: per Story 3.1.1's Given-When-Then (all three bullets).
**Files**: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 4.1.1a: Same-value dedup skip test (~4 min)
- Inside the existing `describe('resize', ...)` block (line 131), add a test that: advances fake
  timers past 200ms after an initial `resize(100, 30)` call, calls `resize(100, 30)` again, and
  asserts `pushMessage`'s mock call count did not increase on the second call (only the initial
  call's send + its 100ms-delayed `currentPaneRequest` are present).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 4.1.1b: Genuinely-new-value is not deduped test (AC6 RPC-level) (~3 min)
- In the same `describe` block, add a test that calls `resize(100, 30)`, advances past 200ms,
  calls `resize(150, 45)`, and asserts `pushMessage` was called twice total (once per distinct
  value) — the mirror-image of Task 4.1.1a, closing the "no regression" half of AC6 at the
  RPC-dedup layer.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 4.1.1c: `force=true` bypass + reconnect scenario test (~5 min)
- Add a test that: calls `resize(100, 30)` (send #1), advances fake timers past 200ms, then calls
  `resize(100, 30, true)` (simulating the reconnect-resync call site) and asserts `pushMessage`
  fires again despite the identical value (send #2, bypassing AC3's dedup); **advances fake timers
  past 200ms again** (isolating the AC3 value-dedup assertion below from the pre-existing
  time-throttle — without this the 3rd call's skip would be ambiguous between "deduped by value"
  and "throttled by time", per adversarial-review.md Minor #2); then calls `resize(100, 30)` (no
  force) and asserts it IS deduped (no 3rd send), proving the forced call correctly re-set
  `lastSentSizeRef`.
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

##### Task 4.1.1d: `force=true` within the throttle window still sends (~4 min) — **added per Gate 2
CRITICAL finding**
- The existing Task 4.1.1c test always advances fake timers past 200ms *before* the forced call —
  it structurally cannot catch a `force` that only bypasses value-dedup, not the time-throttle
  (exactly the bug the architecture reviewer found empirically). Add a distinct test: call
  `resize(100, 30)` (send #1), advance fake timers by only **50ms** (still inside the 200ms
  `THROTTLE_MS` window), then call `resize(100, 30, true)` and assert `pushMessage` **is** called
  a second time — proving `force=true` bypasses the time-throttle, not just the value-dedup.
  Without the Task 3.1.1b fix, this test fails (the throttle's early `return` fires before the
  `force` check is ever reached).
- Files: `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts`

### Epic 4.2: `XtermTerminal` component test harness + AC5/AC6 coverage

#### Story 4.2.1: New test file with a configurable resize harness
**As a** maintainer, **I want** a harness where `MockFitAddon.proposeDimensions()` is settable
per-test and `MockTerminal` gains a `resize()` method that mutates `cols`/`rows` and only fires
`onResize` when the value actually changes (mirroring real xterm.js and real `FitAddon.fit()`
semantics), **so that** AC2's "propose == current → skip" / "propose != current → fit" interaction
can be exercised — the existing `XtermTerminalBug.test.tsx` harness (hardcoded
`proposeDimensions()` returning `{cols:200,rows:50}`, no `MockTerminal.resize()`) cannot express
this.
**Acceptance Criteria**:
- New file exists, follows the existing `XtermTerminalBug.test.tsx` mock/harness shape (own
  `jest.mock` calls per file — no shared module, matching this codebase's established convention
  per `research/stack.md` §5), and explicitly stubs `global.ResizeObserver` (required — there is
  no shared polyfill in `jest.setup.js`).
**Files**: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx` (new)

##### Task 4.2.1a: Create `XtermTerminalResize.test.tsx` with configurable harness (~5 min)
- Create `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`, copying the
  `jest.mock('@xterm/xterm', ...)`, `jest.mock('@xterm/addon-search', ...)`,
  `jest.mock('@xterm/addon-web-links', ...)`, and the non-mobile/gesture mocks from
  `XtermTerminalBug.test.tsx` (lines 34-118) as a starting point, with these changes:
  - `MockTerminal` gains `resize(cols: number, rows: number)`: if `cols !== this.cols || rows !== this.rows`,
    set `this.cols = cols; this.rows = rows;` and invoke the captured `onResizeCb` (mirrors real
    `Terminal.resize()`'s no-op-if-unchanged behavior).
  - `harness` gains `proposedDimensions: {cols,rows} | undefined` (mutable, default
    `{cols:80,rows:24}`) and a `setProposedDimensions(cols, rows)` helper.
  - `MockFitAddon.proposeDimensions()` returns `harness.proposedDimensions`.
  - `MockFitAddon.fit()` calls `harness.terminal.resize(harness.proposedDimensions.cols,
    harness.proposedDimensions.rows)` if a `harness.terminal` back-reference is captured at
    `MockTerminal` construction — increments `harness.fitCalledCount` unconditionally (fit() being
    *called* is still tracked even though it's now gated upstream by `shouldFit` in
    `XtermTerminal.tsx`, so `fitCalledCount` measures "how many times XtermTerminal decided to
    call `fit()`", which is exactly what AC5/AC6 need to assert on).
  - `jest.mock('@xterm/addon-webgl', ...)` gains an observable `dispose: jest.fn()` and a way to
    retrieve the constructed instance (harness field), for Epic 4.3.
  - Add the `Object.defineProperty(global, 'ResizeObserver', ...)` + `fireResizeObserver(width,
    height)` helper from `XtermTerminalBug.test.tsx` lines 247-274, verbatim.
- Files: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

##### Task 4.2.1b: StrictMode double-mount `webglAddonRef` regression test (~5 min)
- Closes architecture-review.md's Blocker remediation requirement. Add
  `describe('webglAddonRef survives StrictMode double-mount', ...)`: render `<XtermTerminal />`
  wrapped in `<React.StrictMode>`, make the mocked `import('@xterm/addon-webgl')` resolve with a
  distinguishable addon instance per call (e.g. tag each mock instance with an incrementing `id`),
  flush all pending microtasks/promises (`await act(async () => { await Promise.resolve(); })`,
  possibly twice — once per StrictMode invocation), then assert the component's live
  `webglAddonRef`-equivalent state (exposed for the test via the same mechanism Epic 4.3 uses to
  observe `dispose()` calls) points at the addon instance loaded by the **second** (real) mount's
  IIFE, not the first (throwaway, cancelled) one.
- Files: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

#### Story 4.2.2: AC5 — sub-cell wobble simulation against the real `ResizeObserver` wiring
**Acceptance Criteria**:
- *Given* `XtermTerminal` is rendered and mounted (initial fit settles at `cols:120,rows:40`, with
  `harness.proposedDimensions` set to `{cols:120,rows:40}` matching the mock terminal's post-mount
  state), *When* `fireResizeObserver(801,600)`, then `fireResizeObserver(802,601)`, then
  `fireResizeObserver(801,600)` are each dispatched with `jest.advanceTimersByTime(150)` +
  `jest.runAllTimers()` between them (simulating sub-cell container wobble that never changes the
  proposed integer grid), *Then* `harness.fitCalledCount` does not increase beyond its post-mount
  value, and the `onResize` prop callback is not called again after mount.
**Files**: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

##### Task 4.2.2a: Write the AC5 sub-cell-wobble test (~5 min)
- Add `describe('AC5: sub-cell resize wobble is a no-op', ...)` with the scenario from the
  Acceptance Criteria above, using `jest.useFakeTimers()` + `jest.runAllTimers()` per the existing
  Bug-3-test timer-control convention (`research/pitfalls.md` §5 — do not mix in the manual
  `captureRaf()` spy style within this `describe` block).
- Files: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

#### Story 4.2.3: AC6 — real cols/rows-crossing resize still converges in one shot
**Acceptance Criteria**:
- *Given* `terminal.cols=80, terminal.rows=24` at mount, *When* `harness.setProposedDimensions(150,
  45)` is set and `fireResizeObserver(1200, 900)` is dispatched (a genuine cell-boundary crossing)
  with the debounce+rAF chain flushed, *Then* `fitAddonRef`'s underlying `fit()` is invoked exactly
  once for this cycle (`harness.fitCalledCount` increases by exactly 1 from its pre-resize
  baseline), the mock terminal's `cols`/`rows` become `150`/`45`, and the `onResize` prop callback
  fires exactly once with `(150, 45)` — proving the AC2 gate does not silently swallow a real
  resize.
**Files**: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

##### Task 4.2.3a: Write the AC6 no-regression test (~5 min)
- Add `describe('AC6: genuine cols/rows change still converges exactly once', ...)` with the
  scenario above.
- Files: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

#### Story 4.2.4: AC1/AC7 — imperative `fit()` handle is gated too
**Added per engineering-lens triad review (2026-07-27 repair pass)**: Epic 2.4's fix (gating the
`useImperativeHandle`-exposed `fit` method, used by `TerminalOutput.tsx`'s tab-visibility and
`visualViewport.resize` handlers for the tab-background/resume trigger) previously had zero
automated coverage — only the one-time manual QA checklist (Story 5.2.1) exercised it, meaning a
future regression there would pass CI silently. This closes that gap with the same harness already
built for Story 4.2.2/4.2.3, exercising the imperative handle directly instead of the
`ResizeObserver`.
**Acceptance Criteria**:
- *Given* `XtermTerminal` is rendered with a ref, mounted with `terminal.cols=84, terminal.rows=60`
  and `harness.proposedDimensions` set to the same `{cols:84,rows:60}` (no actual layout change),
  *When* the test calls `ref.current.fit()` directly (simulating `TerminalOutput.tsx`'s
  visibility-restore/`visualViewport.resize` handlers calling the imperative handle), *Then*
  `harness.fitCalledCount` does not increase — mirrors Story 4.2.2's no-op assertion but via the
  imperative path instead of the `ResizeObserver`.
- *Given* the same setup but `harness.proposedDimensions` is changed to `{cols:90,rows:62}` before
  the call (a genuine change, as if the container resized while the tab was backgrounded), *When*
  `ref.current.fit()` is called, *Then* `harness.fitCalledCount` increases by exactly 1 and the mock
  terminal's `cols`/`rows` become `90`/`62` — proving the gate on this path doesn't regress the
  path's existing legitimate use (mobile viewport changes, visibility restoration after a real
  size change), mirroring Story 4.2.3's no-regression assertion.
**Files**: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

##### Task 4.2.4a: Write the imperative-`fit()` gate tests (~4 min)
- Add `describe('AC1/AC7: imperative fit() handle is gated the same as the ResizeObserver path',
  ...)` with both cases above, calling the component's exposed ref method directly (no
  `fireResizeObserver` involved) — reuses the same `harness.fitCalledCount` counter and
  `MockFitAddon.proposeDimensions()`/`MockTerminal.resize()` semantics from Task 4.2.1a.
- Files: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

### Epic 4.3: AC4 — oscillation burst trips the WebGL fallback

#### Story 4.3.1: Burst detection integration test + backstop
**Acceptance Criteria**:
- *Given* the mock `WebglAddon` successfully "loads" (per the mocked async IIFE resolving), *When*
  `harness.setProposedDimensions` is toggled `84,60 → 85,60 → 84,60` across three
  `fireResizeObserver` calls each separated by <2000ms of fake-timer advancement (with debounce
  flushed each time so each proposed value actually reaches `terminal.onResize`), *Then* the mock
  `WebglAddon.dispose` is called exactly once and `console.warn` logs the oscillation message
  containing "falling back to default renderer" (not "canvas renderer").
- *Given* the WebGL addon never loaded (mock the async IIFE's `WebGL2RenderingContext` guard to be
  `undefined`, taking the "default renderer" `console.log` branch at line 279), *When* the same
  oscillation sequence occurs, *Then* `console.error` logs "no WebGL addon to dispose" and nothing
  throws.
- *Given* WebGL already fell back once (first oscillation burst already tripped `dispose()`, mock
  `WebglAddon.dispose` call count is 1), *When* a **second** independent oscillation sequence
  (`84,60 → 85,60 → 84,60` again, separated in time from the first by more than the 2000ms window
  so it's a genuinely new burst, not a continuation) occurs, *Then* `WebglAddon.dispose` is **not**
  called a second time (nothing left to dispose), and `console.error` is **not** called again —
  only a single `console.log` containing "persists after WebGL fallback" is emitted (closes
  architecture-review.md Concern #2 / adversarial-review.md's unbounded-backstop-spam Minor).
**Files**: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

##### Task 4.3.1a: Write burst-trips-dispose + backstop + repeated-burst tests (~7 min)
- Add `describe('AC4: oscillation burst falls back off WebGL', ...)` with all three cases from the
  Acceptance Criteria; since the WebGL IIFE is async (`await import('@xterm/addon-webgl')`),
  `await act(async () => {})` (or flush a resolved promise via `Promise.resolve()`) after mount
  before asserting `webglAddonRef`-equivalent state is populated, following the existing async-IIFE
  handling pattern already used for WebGL-related assertions in this test suite if present, or the
  standard `await Promise.resolve()` microtask-flush idiom otherwise.
- Files: `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx`

---

## Phase 5: ADR + Manual Verification

### Epic 5.1: ADR-018 (AC4)
**Goal**: Document the WebGL→default-renderer fallback decision, correcting AC4's literal "canvas
renderer" wording, per requirements.md's explicit ADR mandate.

#### Story 5.1.1: Write the real ADR and the SDD-local pointer stub
**Acceptance Criteria**:
- `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md` exists, follows the format of
  `docs/adr/009-vanilla-extract-type-safe-css.md`, and explicitly states the "canvas" → "default
  DOM renderer" correction with the `@xterm/addon-canvas`/`@xterm/xterm ^6.0.0` incompatibility
  evidence.
- `project_plans/terminal-resize-fit-loop/decisions/ADR-001-webgl-oscillation-fallback-to-default-renderer.md`
  exists as a short pointer stub (project-local numbering, matching the
  `project_plans/terminal-jank/decisions/ADR-00N-*.md` convention), linking to the real ADR.
**Files**: `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md`,
`project_plans/terminal-resize-fit-loop/decisions/ADR-001-webgl-oscillation-fallback-to-default-renderer.md`

##### Task 5.1.1a: Write `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md` (~8 min)
- Content: Context (sub-cell glyph metric mismatch, `Actual pixels per column: 8.45px` vs
  `Expected: 8.33px`), the "canvas renderer" terminology correction with evidence
  (`@xterm/addon-canvas` deprecated/incompatible with pinned `@xterm/xterm ^6.0.0`; cockpit-project
  issue #22509), the Decision (reuse `webglAddon.dispose()`, no new dependency, session-scoped by
  construction, ≥3 recurrences/2000ms threshold and rationale, hooked at `terminal.onResize` not
  the raw `ResizeObserver` callback). Consequences must additionally cover (per
  architecture-review.md and adversarial-review.md, do not drop these):
  - Silent/console-only per `research/ux.md`; one-shot dispose per mount (via
    `webglFallbackTrippedRef`), not persisted across remounts/sessions.
  - Not a fix for the underlying pixel/glyph math, which remains explicitly out of scope — a
    session can still oscillate after falling back to the default renderer; that's treated as
    acceptable degraded behavior (logged once via `console.log`, not repeated as an error) rather
    than a complete cure.
  - **Residual false-positive risk** (`research/pitfalls.md` §3): a legitimate slow window/pane
    drag that lingers near a cell boundary can, in principle, produce an alternating A/B/A/B/A
    `onResize` sequence indistinguishable from the real WebGL-wobble bug, tripping the fallback for
    a session that didn't actually need it. Accepted as low-cost — a false trip just disables WebGL
    for that tab for the rest of the session (a rendering-performance regression, not a correctness
    break) — and structurally rare, since the counter only sees *post*-AC2/AC3-dedup applications
    (a value that survived `shouldFit`/became a real `terminal.onResize` firing), not raw
    observer/pointer-move noise.
  - **The imperative `fit()` path is also gated** (`XtermTerminal.tsx:615`, exposed via
    `useImperativeHandle`, called by `TerminalOutput.tsx`'s tab-visibility and
    `visualViewport.resize` handlers — see Epic 2.4, added per pre-mortem.md P1 #1). AC1/AC7's
    repro explicitly includes "tab background/resume," which routes through this path, not the
    `ResizeObserver`; leaving it ungated would have left that half of the repro unfixed. It now
    reuses the identical `shouldFit` predicate, so a repeated call with no actual layout change is
    a no-op there too, and the oscillation counter observes it via the same `terminal.onResize`
    funnel whenever it does produce a genuinely new value.
  - **Double-dispose safety**: the oscillation-triggered dispose path checks `terminalRef.current`
    is non-null before calling `.dispose()`, structurally preventing a dispose-after-teardown race
    (xterm.js#5181) given the `onResize` subscription is itself torn down during unmount cleanup.
- Files: `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md`

##### Task 5.1.1b: Write the SDD-local pointer stub (~2 min)
- Content: `**Status**: Accepted`, `**Real ADR**: docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md`,
  one paragraph summary, per the `project_plans/terminal-jank/decisions/ADR-00N-*.md` sibling
  format.
- Files: `project_plans/terminal-resize-fit-loop/decisions/ADR-001-webgl-oscillation-fallback-to-default-renderer.md`

### Epic 5.2: Manual verification (AC1, AC7) — HARD MERGE GATE
**Goal**: There is no automated way to peg CPU and observe convergence end-to-end across multiple
real browser tabs, and no automated test in this plan exercises real WebGL glyph-metric divergence
at all (Jest/JSDOM has no GPU rendering) — this is a documented manual QA checklist, not a code
task. **Per pre-mortem.md P1 #4, this is upgraded from a PR-description nice-to-have to a required
pre-merge gate**: the PR must not be merged until Story 5.2.1's checklist is completed and its
results recorded, specifically on hardware that can reproduce the original report's WebGL
glyph-metric mismatch (or the closest available approximation), not "any available dev machine."

#### Story 5.2.1: Manual repro checklist (must pass before merge)
**As a** developer shipping this fix, **I want** a documented manual verification pass reproducing
the original ticket scenario, **so that** AC1 and AC7 (which have no automated equivalent) are
explicitly checked off before merge, and so a future regression has a quantitative baseline to
compare against rather than a bare pass/fail.
**Acceptance Criteria** (the checklist itself — no code):

**CORRECTED (post-implementation, during PR review)**: the original checklist below used "3
concurrent browser tabs" as the primary repro topology, citing `research/features.md`'s claim
that no in-page tiled layout exists. That claim was wrong — see `requirements.md`'s corrected
Problem Statement and `research/features.md`'s ERRATA note. `PaneSplitRenderer.tsx` genuinely
tiles multiple independent `XtermTerminal` instances as sibling panes on one page, and that is
the *original* ticket's actual repro topology ("3 terminals open in a split/tiled layout... panes
resize in lockstep"). The checklist below now requires the same-page tiled scenario as the
**primary** check, with separate browser tabs as a secondary (still valid, since that topology is
also real) check.

- AC1 (window-resize trigger, PRIMARY — same-page tiled panes): *Given* 3 concurrent terminal
  sessions tiled as sibling panes on **one page** via the split-pane cockpit UI (open a session,
  split the pane 2-3 times to add sibling `session-detail` panes, each bound to a different
  session — this is the actual original topology, not 3 browser tabs), on a machine/browser with
  WebGL enabled (Chrome, per the original report), *When* the OS window is resized once (e.g.
  1200×800 → 1400×900) — which resizes *all 3 panes simultaneously* via their shared flex
  container, potentially cascading layout reflows between sibling panes — *Then* Chrome DevTools'
  Performance panel, recorded for 5s starting at the resize, shows the ResizeObserver→fit()→onResize
  chain settling in every pane (no repeating `[XtermTerminal] Container resized` log lines after
  the first 1-2 debounce cycles, in any pane, including cascading reflows triggered by sibling
  panes) and CPU usage returns to idle (<5%, verified via the DevTools Performance panel's CPU
  track) within 2 seconds of the resize ending.
- AC1 (window-resize trigger, SECONDARY — separate browser tabs): *Given* 3 concurrent browser
  tabs, each with its own `XtermTerminal`/session (a real, independently-valid topology, just not
  the original ticket's), *When* the OS window is resized once, *Then* the same convergence
  criteria as above hold in each tab.
- AC7 (tab background/resume trigger — tested as its own independent sub-case, not assumed covered
  by AC1's window-resize pass, per pre-mortem.md P1 #1): *Given* the same tiled-panes setup (and,
  secondarily, the 3-tab setup), *When* the tab/window is backgrounded (switch away for ≥5s, to let
  `visualViewport`/visibility handlers fire) and resumed (switch back), *Then* input in all panes
  (or all 3 tabs) remains responsive throughout (typing echoes without a perceptible stall) and no
  pane/tab shows a frozen/unresponsive terminal — the original repro condition from the backlog
  item, verified specifically via the tab-background/resume path (Epic 2.4's imperative-`fit()`
  gate), not just the window-resize path.
- **Quantitative baseline** (per pre-mortem.md P1 #4): record the console-logged "Actual pixels per
  column" vs "Expected pixels per column" values (`XtermTerminal.tsx`'s existing mount-time log,
  lines ~386-393) observed during this pass, along with the browser/OS/GPU used, in the PR
  description — even on a pass, these numbers establish a baseline; if the original 8.45px vs
  8.33px mismatch cannot be reproduced on available hardware, say so explicitly rather than
  reporting an unqualified "pass."
- **Corroborating signal** (per cross-artifact consistency check): watch the console during both
  sub-cases above for `shortcutRegistry.ts`'s "Duplicate shortcut id" churn (the requirements.md
  out-of-scope carve-out is conditioned on "if verification shows it still fires after the
  convergence fix lands" — this pass is that verification). Record whether it still fires; if it
  does, the shortcut-registry idempotency issue should be filed as a separate follow-up per
  requirements.md's carve-out, not silently ignored.
- Record the full outcome (pass/fail per sub-case + DevTools screenshot or CPU trace if fail +
  the quantitative baseline + the shortcut-id observation) directly in the PR description. **This
  is a merge gate, not an optional checklist item** — do not merge without it, and do not report
  AC1/AC7 as satisfied without these specifics recorded.
**Files**: None (manual QA — documented in the PR description, not committed to the repo).
