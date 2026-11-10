# Build vs Buy — Bun Build 7a Baseline Regressions

## R1: Route Announcer

**Verdict: Keep custom `RouteAnnouncer.tsx` (build).**

No third-party package is worth pulling in here. react-aria's `VisuallyHidden` and live-region utilities exist but are general-purpose primitives, not a ready-made route announcer. The custom `RouteAnnouncer.tsx` is already written, small (~44 lines), and intentionally uses `role="status"` / `aria-live="polite"` rather than `assertive` — the ADR-001 comment in the file records that Next.js's own announcer is assertive, and this is the deliberate departure. A third-party package would either replicate this behaviour or force an assertive policy that was already rejected. **No change needed; wire it into the Bun layout root the same way Next's built-in announcer was wired.**

## R5: Scroll Restoration (inner-element, remount-safe)

**Verdict: Build (keep and fix `ScrollPolicy.tsx`); react-router's `<ScrollRestoration>` is insufficient.**

react-router v7's built-in `<ScrollRestoration>` only manages `window.scrollY` — it has no API for restoring scroll on inner elements (the comment on line 5 of `ScrollPolicy.tsx` already records this, citing spike-results.md gate S4). There is no maintained third-party package that fills the gap for inner-element + remount-safe restoration; `scroll-restoration` npm packages either wrap the same window-only browser Session History API or are unmaintained.

The regression is the sibling-index path descriptor (`tag:N>tag:N>...`) breaking after Suspense remounts: a remounted subtree rebuilds DOM in a new position, so the index path resolves to a different element. The fix is in `describe()` and `find()` in `ScrollPolicy.tsx`: prefer `id` (already done for the id case), then fall back to a stable data attribute (`data-scroll-key="<logical-name>"`), and only use sibling-index as a last resort (or drop it). **This is a targeted fix to the existing file, not a new dependency.**

## R3: Not-Found Handling (Next App Router)

**Verdict: Create `app/not-found.tsx` (buy the convention — it's zero-code).**

The Next.js App Router convention is `app/not-found.tsx` — a default export that Next automatically renders for `notFound()` throws and unmatched routes. The repo currently has `src/app/NotFound.tsx` (a named export rendered as a route component) but no `not-found.tsx`, which is why the baseline test fails to see a proper 404 page. The fix is a one-line file that re-exports the existing component as a default export:

```tsx
// app/not-found.tsx
export { NotFound as default } from "./NotFound";
```

No new dependency, no behaviour change — the existing `NotFound` component already renders correctly. The wildcard-route alternative (`routes.tsx` catch-all) works for react-router but does not satisfy Next's `not-found.tsx` baseline check, so the file is the right fix.
