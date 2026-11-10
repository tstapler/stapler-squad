# Architecture Review: bun-build-7a-baseline-regressions

**Reviewer**: Architecture review subagent  
**Date**: 2026-10-22  
**Plan file**: `implementation/plan.md`  
**Verdict**: CONCERNS FOUND — 0 blockers, 2 concerns, 1 nitpick

---

## Lens 1 — Structural Integrity

### R1 / R2: `test.skip(isBunBuild(), ...)` at describe level

Guard placement — first line inside the `test.describe` callback, before `test.beforeEach` — matches the convention used by the existing Bun-only blocks. Playwright evaluates it once for the whole block. No structural issue.

### R4: SCROLLER fallback path

The proposed replacement preserves the existing DOM scan as an unconditional fallback. The testid-preference short-circuit exits early only when the element exists AND has a non-trivial scrollable range (`r > 0`). The fallback still initialises `range` from `document.scrollingElement` and iterates all elements, identical to the current implementation. No structural issue.

---

## Lens 2 — Type-level Design

### CONCERN 1 — Story 3.1.1: `not-found.tsx` is missing `"use client"` (Story 3.1.1, CONCERN)

**Problem**: The plan proposes:

```tsx
export { NotFound as default } from "./NotFound";
```

`NotFound.tsx` calls `usePathname()` and `useRouter()` — React hooks. Without a `"use client"` directive, Next.js App Router treats `not-found.tsx` as a Server Component and will error at build or render time when it encounters the hook calls in the component tree.

Evidence: every other page in `src/app/` that uses hooks carries `"use client"` (checked: `page.tsx`, `errors/page.tsx`, `history/page.tsx`, etc.). `layout.tsx` is a Server Component and does not use hooks. `NotFound.tsx` has no `"use client"` because it is currently only consumed by the Bun/react-router build, not the Next App Router tree. Adding it as the Next `not-found.tsx` entry point introduces it to the server-render path for the first time.

Story 3.1.1, Task 2 ("Run `make build` to confirm no compilation errors") will catch this — but the plan's code snippet is wrong as written.

**Remediation**: Change the proposed `not-found.tsx` content to:

```tsx
"use client";
export { NotFound as default } from "./NotFound";
```

This declares the client boundary at the entry-point file. `NotFound.tsx` itself does not need to change.

---

## Lens 3 — Pattern Selection

### `not-found.tsx` and `route-meta.ts` — confirmed non-issue (NITPICK)

The task brief asked whether the plan should add `not-found.tsx` to `route-meta.ts`. It should not, and correctly does not.

`route-meta.test.ts`'s `listPageRoutes()` scans only for files named `page.tsx` (line 19: `else if (entry.name === "page.tsx")`). `not-found.tsx` is a Next.js App Router convention file, not a `page.tsx`, and is excluded from that scan. The `NOT_FOUND_META` export is already present in `route-meta.ts` (line 22-24) and its title is pinned by the test at line 125 (`'exports the not-found title as data'`). No `ROUTE_META` row is required, and adding one would break the drift test (which asserts `Object.keys(ROUTE_META).sort()` equals the page routes list).

No action needed.

### R3: fixture strategy — convergence, not divergence

After R3, the `not found (Next baseline)` describe block has no `isBunBuild()` skip guard. This is intentional: creating `not-found.tsx` makes both builds show the same not-found page at `/nonexistent`. The updated fixture (`{ finalPath: "/nonexistent", title: "Page not found - Stapler Squad" }`) will match both builds. The Bun-only describe block tests more granular behavior (heading text, path display, home link, nav aria-current) and remains gated by `test.skip(!isBunBuild(), ...)`. The dual-block structure remains coherent.

---

## Lens 4 — Tech Debt Trajectory

### CONCERN 2 — `measureNotFound` 5-second latency spike after R3 (Story 3.1.2, CONCERN)

`measureNotFound` in `BaselineProbes.ts` (line 18) calls:

```ts
await page.waitForURL((url) => url.pathname !== missing, { timeout: 5000 }).catch(() => undefined);
```

This was designed to detect a client-side redirect away from `/nonexistent` (the current Next behavior: redirect to `/`). After R3, the Next build will stay at `/nonexistent` (showing the not-found page), so the predicate never fires, the 5-second timeout always elapses, and the `.catch` silently swallows it. The `not found (Next baseline)` describe block calls `measureNotFound` twice (tests at lines 20 and 28), adding ~10 seconds to every Next-build e2e run of `not-found.spec.ts`.

This is not a correctness problem (tests pass), and the Bun build already has this latency today. The regression is that Next now joins Bun in timing out on every run.

**Remediation** (can be a follow-on, not a blocker for this item): Remove or shorten the `waitForURL` in `measureNotFound` after confirming neither build redirects. The simplest fix is to remove the call entirely since its only purpose was to detect a redirect that no longer exists on either build:

```ts
// After R3: neither build redirects; remove the wait
// await page.waitForURL((url) => url.pathname !== missing, { timeout: 5000 }).catch(() => undefined);
```

If the wait needs to stay for future flexibility, reduce the timeout to something like 500 ms.

The plan should add a task to Story 3.1.2 or Epic 5.1 to address this.

### Tech debt disposition: accurate

The plan's disposition table (R4 cleanup, R5 out-of-scope, R3 one-line fix) is honest and proportionate. No artificially clean slate is claimed.

---

## Summary Table

| Story | Classification | Finding | Remediation |
|---|---|---|---|
| 3.1.1 | **CONCERN** | `not-found.tsx` missing `"use client"` — Next will fail to render `NotFound` as a Server Component because it calls React hooks | Add `"use client";` as first line of the proposed `not-found.tsx` content |
| 3.1.2 | **CONCERN** | `measureNotFound`'s 5-second `waitForURL` always times out after R3 on both builds; adds ~10s to every Next e2e run | Add task to remove/reduce the `waitForURL` call; can be a follow-on |
| 3.1.1 | **NITPICK** | `not-found.tsx` does not need a `ROUTE_META` row — confirmed by reading `route-meta.test.ts` line 19 (`page.tsx` files only) | None; plan is correct as written |

**Verdict**: 0 blockers, 2 concerns. The concerns do not require redesigning any phase; both are isolated file-content corrections. The plan's overall approach (guard injection for R1/R2, convergent not-found behavior for R3, stable testid for R4) is architecturally sound.
