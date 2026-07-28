# Verification Report — terminal-resize-fit-loop

**Date**: 2026-07-27

## Technology Surface

| Technology | Files | Review approach |
|---|---|---|
| TypeScript/React | `XtermTerminal.tsx`, `TerminalOutput.tsx`, `useTerminalFlowControl.ts`, `useTerminalStream.ts`, `resizeConvergence.ts` + 4 test files | `ui-react-best-practices` skill |

(All other diff content is `project_plans/`/`docs/adr/` SDD documentation, not reviewed as code.)

## Layer 1 — Idioms

| Technology | Findings | MUST FIX | Action taken |
|---|---|---|---|
| React/TS | 4 (0 MUST FIX, 3 SUGGEST, 1 NITPICK) | 0 | 3 SUGGEST fixed inline (see below); 1 NITPICK (`force` param naming) noted as follow-up |

Specifically verified idiomatic-sound (not violations): the `cancelled` async-liveness-flag pattern (React's own documented race guard for effect-scoped async work) and the effect-scoped `oscillationHistory` local variable (correctly avoids a `useRef` that would leak stale history across a `[scrollback]`-triggered remount).

## Layer 2 — Architecture

| Finding | Severity | Action |
|---|---|---|
| Duplicated `shouldFit` gate-and-fit logic (2 call sites, already drifted) | NITPICK (architecture) / SUGGEST (idiom) / HIGH (refactor) | Fixed — extracted `attemptFit()` helper |
| Double-pruning of `oscillationHistory` (outer filter + `shouldAbandonWebgl`'s internal filter) | NITPICK | Noted — effectively free at this scale, not worth restructuring |
| `fontSize`/`fontFamily` effects call `fit()` ungated | NITPICK | Noted — correct as-is, a font change is definitionally a real dimension change |
| Duplicated `OSCILLATION_WINDOW_MS`/`THRESHOLD` magic numbers | SUGGEST / LOW | Fixed — exported from `resizeConvergence.ts`, single source of truth |
| `webglFallbackTrippedRef` not reset on effect cleanup | SUGGEST (idiom-review-only finding) | **Fixed — genuine latent bug**: without this, a scrollback-triggered remount whose new instance never loads WebGL would misfire the "persists after fallback" log instead of the correct "no addon to dispose" backstop. New regression test added and verified to fail without the fix. |
| `terminal.onResize` callback doing 4 things at once | MEDIUM (refactor) | Deferred as follow-up — more invasive restructuring of already-tested working code; lower value than the other 3 |
| New test file re-implements the xterm.js mock harness (2nd copy in the directory) | LOW follow-up | Deferred — explicitly recommended as follow-up by the refactor reviewer, `jest.mock` factory hoisting makes a shared factory fiddly |

**Constitution check**: `docs/adr/ADR-000-architecture-constitution.md` does not exist — N/A.
**Verdict**: 0 BLOCKER, 0 CONCERN across both architecture and adversarial-style review passes.

## Layer 3 — Correctness

| Story | Criterion | Status |
|---|---|---|
| 1.1.1/1.1.2 | Pure gate/dedup/oscillation functions | ✅ |
| 1.2.1–1.2.3 | Unit test coverage (boundary, alternating-history cases) | ✅ |
| 2.1.1 | `webglAddonRef` reachable, race-guarded, leak-fixed | ✅ |
| 2.2.1 | AC2 — `fit()` gated on live `terminal.cols`/`rows` | ✅ |
| 2.3.1 | AC4 — oscillation detector at `terminal.onResize` funnel, 3-branch backstop | ✅ |
| 2.4.1 | AC1/AC7 — imperative `fit()` handle gated | ✅ |
| 3.1.1 | AC3 — value-dedup independent of time-throttle | ✅ |
| 3.1.2 | 2 non-standard callers pass `force=true`, regression-tested | ✅ |
| 4.1.1–4.3.1 | Full AC1–AC7 test coverage, mutation-tested | ✅ |
| 5.1.1 | ADR-018 written, consistent with shipped code | ✅ |
| 5.2.1 | Manual verification (AC1/AC7 live-browser pass) | ⚠️ Deferred to human PR review — see `manual-verification-report.md` |

## Tests

**65 passed, 0 failed, 0 skipped** across 4 suites (`resizeConvergence.test.ts`, `useTerminalFlowControl.test.ts`, `TerminalOutputBug.test.tsx`, `XtermTerminalResize.test.tsx`).

## Security

✅ No issues. Scanned the production-code diff for auth/authorization surface, external HTTP calls, user-input-to-sensitive-sink paths, and hardcoded secrets/credentials — none present. This is client-side terminal-resize convergence logic; `cols`/`rows` values originate from trusted internal xterm.js APIs, not raw external input.

## Error Handling

Pre-existing `try/catch` around `resize()`'s `pushMessage` call is unchanged. WebGL `.dispose()` calls are guarded by a `terminalRef.current` liveness check before invocation (prevents dispose-after-teardown). No new external-call error paths introduced by this diff.

## Observability

Matches `plan.md`'s Observability Plan: new `console.log`/`console.warn`/`console.error` sites at every new decision point (`fit()` skip, resize skip, oscillation trip, backstop, repeat-burst), following the existing `[XtermTerminal]`/`[useTerminalFlowControl]`-prefixed convention. No metrics/alerts required — explicitly justified (no frontend telemetry pipeline exists in this repo; client-side only, no backend/on-call surface change).

## Layer 4 — UX & Behavioral

Skipped — no user-facing surface (confirmed in `research/ux.md` and `plan.md`; this is a background reliability fix with no new UI, no `design/ux.md` exists for this project).

## Fix Loop Summary

| Layer | Iterations used | Items resolved | Items remaining |
|---|---|---|---|
| L1+L2 | 1 / 5 (inline fix, not a formal repair loop — no BLOCKER/MUST FIX triggered it) | 3 | 0 (2 deferred as documented, lower-value follow-ups) |
| L3 | 0 / 5 | 0 (nothing failed) | 0 |
| L4 | N/A — skipped | — | — |

## Verdict

**✅ PASS** — all layers clean. Ready for `/sdd:7-ship`.

The one open item (Story 5.2.1's live-browser manual verification) is a documented, transparent deferral to human PR review — not a code defect. It is called out explicitly in the PR description per `manual-verification-report.md`.
