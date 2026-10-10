# Build vs. Buy: session-list-density

Research date: 2026-09-23. Repo state: `web-app/package.json`, `web-app/src/components/sessions/SessionRow.tsx`, `SessionCard.tsx`, `BoardCard.tsx`, `SessionList.tsx` as of branch `stapler-squad-wasted-space`.

## 1. Middle/smart text truncation for paths

**Existing code:** `SessionRow.tsx:168` already has `abbreviatePath(p)` — a one-line regex that swaps `/home/<user>/` or `/Users/<user>/` for `~/`. It does *not* do middle-truncation or opaque-segment collapsing; that's net-new for this feature. `truncateGoal` (`@/lib/utils/string`) is a separate generic end-truncation helper used for note tooltips, not paths.

**Candidate packages found:**
- `react-middle-truncate` — v1.0.3, last published ~6 years ago, no recent PR/issue activity. Effectively unmaintained.
- `react-middle-ellipsis` — v1.3.0, last published ~1 year ago, "Inactive" per Snyk (no release in 12 months), small (~7–19k weekly downloads depending on tracker). Does generic middle-ellipsis based on rendered pixel width via a wrapper component (recomputes on resize) — not path-segment-aware.
- `@re-dev/react-truncate` (`MiddleTruncate` component) — v0.6.0, published ~3 months ago, actively maintained fork/successor to `react-truncate`. Best-maintained of the bunch, but it's still generic middle-truncation (numeric/regex `start`/`end` anchors), not opaque-segment detection.
- `truncate-middle` (non-React string utility) — v2.0.1, ~1 year old. Plain string slicing, no path semantics.

**None of these packages solve the actual requirement.** The spec calls for something more specific than generic middle-ellipsis: collapse *opaque* segments (workspace hashes, worktree UUIDs) while preserving *meaningful* segments (repo/branch name) — a path-semantics-aware algorithm, not a pixel-width-based generic middle-truncate. Every package above operates on either raw character-count anchors or rendered-width, with no concept of a path segment being a hash/UUID vs. a human name. Adopting one would still require writing the opaque-segment detector yourself on top, and now you're maintaining an unmaintained dependency and the custom logic both.

**Build:** a ~20-30 line `truncateMiddlePath(path, maxChars)` utility that:
1. Splits on `/`.
2. Classifies each segment (regex: hex hash like `/^[0-9a-f]{7,40}$/i`, UUID like `/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i`).
3. Collapses runs of opaque segments to `…`, keeps first N and last M segments.
4. Falls back to plain middle-ellipsis if no opaque segments are found and the path still exceeds `maxChars`.

**Regex correctness risk:** low for this specific case. UUID v4 regex is a well-known, widely-copied pattern (same one used in countless validators) — copying a canonical UUID regex from a trusted reference (e.g. MDN, `uuid` package's own validation regex) rather than having the LLM invent one from scratch removes most of the risk. The hex-hash case (git SHAs, workspace hash IDs) is even simpler: `^[0-9a-f]{7,40}$` bounded by length is hard to get subtly wrong. The main correctness risk isn't the regex syntax, it's *false positives* — a legitimately-named branch or directory that happens to be all-hex (e.g. a branch named `dead beef` or a numeric-looking dir) getting collapsed when it shouldn't. That risk exists regardless of build-vs-buy since no package encodes this domain-specific rule either. Mitigate with unit tests covering: git SHA (7 and 40 char), UUID v4, workspace-hash-style IDs actually seen in this repo (check `session/instance_snapshot.go`/`config/` for the real ID format used, e.g. the `workspaces/<hash>/` directory naming from `docs/reference/state-isolation.md`), plus adversarial cases (short all-hex branch names) to confirm they're *not* collapsed.

**Verdict: Build.** The requirement's specificity (opaque-segment detection tied to this repo's actual path shapes — workspace hashes, worktree UUIDs) is exactly the kind of narrow, domain-coupled logic that doesn't benefit from a generic library, and no candidate package does opaque-segment detection anyway. A small custom utility with good unit-test coverage of the regex edge cases is lower total risk than pulling in an unmaintained (or barely-maintained) dependency you'd still have to wrap custom logic around. Shared via `web-app/src/lib/utils/` (alongside existing `truncateGoal`) between `SessionRow.tsx` and `SessionCard.tsx`/`BoardCard.tsx` per the requirements doc.

## 2. CSS Container Queries

**Package versions in `web-app/package.json`:** `@vanilla-extract/css@^1.20.1`, `@vanilla-extract/recipes@^0.5.7`, `@vanilla-extract/next-plugin@^2.5.1`.

**Finding:** Container queries are native CSS support that vanilla-extract has had since a 2022 PR (`vanilla-extract-css/vanilla-extract#807`, merged into the `@container` key of `style()`). No plugin or sprinkles extension is needed — it's built into `@vanilla-extract/css` core, and `^1.20.1` is far newer than the version that introduced it. Usage is the same shape as existing `@media` conditions in this codebase's `.css.ts` files:

```ts
style({
  '@container': {
    '(min-width: 400px)': { flexDirection: 'row' },
  },
})
```

For named-container targeting (needed if the narrow-layout query should target a specific ancestor rather than "nearest container"), vanilla-extract also exports `createContainer()` to scope a `container-name`.

**Caveat carried over from vanilla-extract's own docs:** vanilla-extract emits the `@container` at-rule as-is; it does **not** polyfill container queries for unsupported browsers. This is a browser-support question, not a tooling one — worth a quick check of this project's actual browser support matrix (not investigated here), but container queries have been supported in all major browsers (Chrome/Edge 105+, Firefox 110+, Safari 16+) since early 2023, so by 2026 this is very unlikely to be a real constraint for a locally-run dev tool like stapler-squad.

**Verdict: Native CSS, no library/plugin needed.** Write `@container` blocks directly in the existing `.css.ts` files (`SessionRow.css.ts`, `SessionCard.css.ts`, `BoardCard.css.ts`) using the same `style()` API already in use. If a named container is needed rather than nearest-ancestor matching, add `createContainer()` — still zero new dependencies.

## 3. List virtualization

**Finding: this is not a build-vs-buy decision — it's already adopted and already wired for variable-height rows.**

- `@tanstack/react-virtual@^3.13.25` is already a dependency (`web-app/package.json`).
- `SessionList.tsx:5` imports `useVirtualizer` from it.
- `SessionList.tsx:643-649` configures the virtualizer with `estimateSize` (40px header / 50px row — used only as the *initial* estimate) **and** `measureElement: (el) => el.getBoundingClientRect().height`.
- `SessionList.tsx:1182-1197`: each rendered row attaches `ref={rowVirtualizer.measureElement}` and `data-index={virtualItem.index}`, and is positioned via `transform: translateY(${virtualItem.start}px)`.

This is exactly the standard TanStack Virtual "dynamic size" pattern (estimate → render → measure via ResizeObserver → reflow) documented in TanStack's own dynamic-sizing example. It already tolerates variable-height content today — the `estimateSize` values (40/50) are just *initial guesses* used before the real DOM measurement lands, not a fixed-height constraint. Wrapping text in `SessionRow.tsx` making rows taller will trigger `measureElement`'s `ResizeObserver`-driven remeasurement automatically; no code change to the virtualizer itself is needed.

**What *does* need attention (not a new-library decision, just care in the existing wiring):**
- `estimateSize`'s current 50px average should probably be bumped up once rows can wrap to 2-3 lines, purely to reduce the initial layout jump/jank TanStack's docs warn about ("estimate the largest possible size, within comfort") — a config tweak, not an architecture change.
- The known TanStack Virtual gotcha (`TanStack/virtual#425`) — calling `.measure()` manually can go stale for dynamic rows — is only relevant if this feature adds a manual `.measure()` call (e.g. to force-remeasure after a bulk state change); the current code doesn't call `.measure()` directly, so this isn't yet a live risk, just something to watch if added.

**Verdict: N/A / already built — not in scope for a build-vs-buy decision.** The requirements doc's "Feasibility Risks" section flags "possible virtualization conflict with variable row height (unconfirmed if virtualized)" — that's now confirmed resolved: `SessionList.tsx` is virtualized via `@tanstack/react-virtual` with dynamic measurement already in place, so there's no additional virtualization work, and no case to defer or adopt anything new. The remaining work is just tuning `estimateSize` and validating row transitions visually once wrapping ships — normal implementation follow-through, not a scope decision.

## Summary Table

| Sub-problem | Decision | Verdict |
|---|---|---|
| Middle-truncation path utility | Custom ~20-30 line function, shared under `web-app/src/lib/utils/` | **Build** |
| Container-query narrow layout | Native CSS via vanilla-extract's built-in `@container` key (no plugin) | **Native, no dependency** |
| List virtualization for variable-height rows | Already implemented (`@tanstack/react-virtual`, `measureElement` wired) | **Already built — no action** |
