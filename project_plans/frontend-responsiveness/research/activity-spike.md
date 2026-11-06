# Spike 4.1.1: React `<Activity>` vs CSS hiding for keep-alive windows

**Decision: CSS hide (`display:none` via the `hidden` attribute) + `inert`. Do not use `<Activity>` in Epic 4.** Epic 4 is written against the `windowActive` prop, so no other story changes.

## Why

The plan's rule: use `<Activity>` only if both builds pass. The Next build fails, and the Bun build was only partly checked (see Findings).

## Findings (2026-10-26, throwaway worktree, discarded afterwards)

Baseline: `web-app/package.json` has `react ^19.0.0`, installed 19.1.1, Next 15.3.2. `typeof Activity` is `undefined` on 19.1.1 (VERIFIED: `node -e` against `node_modules/react`).

| Check | Result | Evidence |
|---|---|---|
| React 19.2.9 renderer semantics (jsdom, `react-dom/client`, throwaway script) | PASS | `Activity` is a symbol. Hidden keeps the DOM (`display: none` applied by React), preserves state (`n: 1`) and uncontrolled input value (`typed`). Layout and passive effect cleanups both run on hide; both re-mount on show. Log: `layout-mount \| passive-mount \| --hide \| layout-cleanup \| passive-cleanup \| --show \| layout-mount \| passive-mount` |
| Bun build (`bun run build:bun`) with `react`/`react-dom` pinned to 19.2.9 and a page importing `Activity` plus `@xterm/xterm` | Bundle step PASSED, not runtime-checked | Failed only `verify-output`: `budget: wall time 13049 ms exceeds maxWallMs 10000 ms` (machine was loaded by a concurrent build). Not a compile or resolve error. The built output was not opened in a browser. |
| Next build (`next build`, App Router) with the same page | FAIL | Webpack: `Attempted import error: 'Activity' is not exported from 'react'`. Next's App Router aliases `react` to its vendored copy, `next/dist/compiled/react` 19.2.0-canary-3fbfb9ba-20250409, which has no `Activity` or `unstable_Activity` export (VERIFIED: `node -e` against it). Upgrading the `react` package has no effect on that copy. I killed the run during "Checking validity of types" after 11 min of compile; runtime behavior of the undefined import was not exercised, but it cannot render. |

## Consequences of the semantics

Even where `<Activity>` works, it would not fit the "bounded K=3 windows that keep consuming streams" design without extra work. Effect cleanup on hide means xterm sockets and `TerminalPool` docks would be torn down and rebuilt, while the plan (Story 4.1.2) requires hidden terminals to **stay docked and keep consuming streams**. CSS hide with `inert` keeps effects live and matches `ux.md` S2.

## What to revisit

Reopen only after Next ships a vendored React that exports stable `Activity` (the repo is also moving off Next, so this may never matter), and only if the effect-cleanup behavior is wanted for windows beyond K.

## Gaps

- Next build was not run to completion, and neither build's output was run in a browser.
- The jsdom check covers renderer semantics only (no real xterm under `<Activity>` at runtime).
