# ADR-001: Client router is React Router 7.18.4 (pinned) behind the `src/lib/nav` facade

Status: Accepted — owner chose React Router 7 on 2026-10-19 (flagged choice F1). Gate S4 still has to pass before Epic 4; if it fails, re-open. Date: 2026-10-19
Evidence: `research/build-vs-buy.md` §2, `research/stack.md` §4, `research/architecture.md` §3-4, `research/pitfalls.md` §5.

## Context

Next's router surface used by the app is narrow: `useRouter` (27 importers), `useSearchParams` (23), `usePathname` (6), `redirect` (2 files), `Link`. The 16 "nested layouts" are really 12 title pass-throughs, 3 feature-flag guards and the root shell. URL state is real (about 25 query keys), and four sites bypass the router with raw `history.replaceState/pushState`.

## Decision

1. All code outside `src/lib/nav/**` and tests imports navigation from `@/lib/nav`. A lint rule (`no-restricted-imports`) bans `next/*`, `react-router`, `react-router-dom` elsewhere, with no exclusion beyond `src/lib/nav/**`, `src/lib/lazy/**` and tests. The router *entry* (`createBrowserRouter`, `RouterProvider`, `Outlet`) is not an exception: it lives in `src/lib/nav/entry.tsx` (inside the exempt directory) and exposes `createAppRouter(spec)` and `AppRouterProvider`; `src/main.tsx` and `routes.tsx` import only those and facade types. `entry.tsx` is not part of the 8-name facade (`index.ts` does not re-export it, so the Next build never bundles it) and a second restricted-import pattern allows `@/lib/nav/entry` only from `src/main.tsx` and tests.
2. The Bun build selects `impl.router.tsx` (React Router **7.18.4, exact pin, no caret**, data router via `createBrowserRouter`). The Next build selects `impl.next.tsx` until Next is removed.
3. Typed search-param helpers are written in the facade (small parsers), not obtained from the router.
4. React Router 8.x is out of scope: it needs `react >= 19.2.7` and Node >= 22.22; the repo is on React 19.1.1.
5. Spike gate S4 (Epic 0) must pass before Epic 4 starts. Dependency gate: the pinned tarball has no `optionalDependencies`, no `github:` source and no install scripts; a repo check rejects banned versions.

## Alternatives rejected

- **TanStack Router** (viable only at an audited exact pin >= 1.170.0, never 1.169.5 / 1.169.8): typed search params are its real advantage, but the May 2026 compromise (provenance was valid on the malicious versions), a file-route codegen step with no Bun integration, and a re-expression of routes rather than a facade swap outweigh it. The facade helper gives the typed-search benefit. If the owner picks it anyway: `impl.router.tsx` is rewritten, Epic 4 is re-planned, S4 re-run against it, and the read-the-hardening-post item in `research/build-vs-buy.md` must be closed first.
- **wouter, hand-rolled router**: nested layouts, scroll restoration, blockers and code splitting would be ours. A tiny fake is acceptable only as the jest test double.
- **React Router 8.x**: forces a React bump in a migration that should not carry one.

## Consequences

- The 63 production files and 43 test files move once, under Next, onto the facade; the router swap itself becomes a one-file change.
- RR does not observe raw `history.replaceState`; the four raw sites move to facade helpers (Story 2.6).
- RR has no route announcer and no focus move; the facade owns both (Story 4.5).
- Revisit trigger: S4 fails, or an advisory lands against 7.18.x.

## Amendments (Phase 3 repair, 2026-10-19)

- Facade options are router-neutral: `{ resetScroll: false }` replaces Next's `{ scroll: false }` (mapped per impl). Impl files are `.tsx` and own `Link` and `Redirect`; one in-memory impl, `impl.memory.tsx`, serves Storybook and Jest (plan Stories 2.1, 2.9).
- `RoutePath` is enforced only as the key type of the route table and `RouteMeta`; `push`/`replace`/`Link href` accept `string` because call sites add query strings and ids.
- Item 3 (typed search-param helpers) is scheduled as plan Task 2.1d (pure parsers in `lib/nav/search-params.ts`, adopted by two call sites, not mass-migrated).
- Iteration 3 (Phase 4 repair, consistency BLOCKER B2): chose "strict ban plus an entry module inside `src/lib/nav/`" over "widen the exclusions to `src/main.tsx`/`routes.tsx`". Reason: the ban's value is that router types and calls exist in exactly one directory; widening it by two app files would let `routes.tsx` grow router-specific code that the Next build and `impl.memory.tsx` cannot run. Cost: one extra file (Task 4.2d) and the entry-only restricted-import pattern.
- Titles and guards have one source for the router build: `RouteMeta`; `RouteDef` carries neither (plan Story 4.3). The root `errorElement` sits on a pathless child inside a pathless shell layout route so a failed lazy import keeps the shell (Story 4.4).
