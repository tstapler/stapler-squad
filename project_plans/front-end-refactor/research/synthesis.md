# Research Synthesis: Front-End Refactor

**Date**: 2026-04-16
**Input files**: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md
**Web searches run**: 8 (vanilla-extract+Turbopack, Radix+Next.js15, ConnectRPC+RN, xterm WebGL Android, viewport-segments Pixel Fold, Turborepo+Expo, connect-query streaming, xterm mobile touch)

---

## Decision Required

What architectural foundation — component system, CSS layer, state management pattern, and code-sharing structure — should the stapler-squad front end adopt to enable consistent mobile+desktop UX and eventual React Native sharing?

---

## Context

Stapler-squad is a Next.js 15 / React 19 / TypeScript session manager for AI coding agents. The current front end has:
- ~70 CSS Module files and only 1 vanilla-extract `.css.ts` file (ADR-009 was adopted but never executed)
- No shared component primitive library — every screen reinvents buttons, modals, inputs
- A 500-line god hook (`useSessionService`) mixing data fetching, CRUD, and terminal streaming
- App broken in meaningful ways on Pixel 9 Pro Fold — touch targets undersized, virtual keyboard obscures inputs, WebGL terminal crashes

Locked: Next.js 15, React 19, Redux Toolkit, ConnectRPC + protobuf.

---

## Options Considered

| Option | Summary | Key Trade-off |
|--------|---------|---------------|
| **Radix UI + vanilla-extract** | Headless primitives styled with VE recipes; token contract via `createThemeContract` | Requires `"use client"` wrapper for all interactive primitives; no Tailwind conflict |
| **shadcn/ui** | Radix + Tailwind code-generator | Hard conflict with ADR-009; re-introduces a second CSS system |
| **Headless UI** | Tailwind Labs' headless primitives | Tailwind-first docs/ecosystem; fewer primitives than Radix |
| **Ark UI** | Chakra-based headless with Zag.js state machines | Younger, less battle-tested; adds Zag.js dependency |
| **Turborepo `packages/core`** | RTK slices + ConnectRPC factory shared across Next.js and Expo | Best-supported 2025 monorepo pattern; Expo SDK 52 auto-detects |
| **Nx monorepo** | Heavier enterprise tooling | Excessive complexity for a 1-developer project |
| **RTK Query for server state** | Custom `baseQuery` wrapping ConnectRPC unary calls | Streaming endpoints must stay as manual dispatch hooks; protobuf serialization boundary needed |
| **connect-query (TanStack)** | Official ConnectRPC wrapper for TanStack Query | Streaming support is being reworked (Issue #524, March 2025); less stable for this use case |

---

## Dominant Trade-off

The fundamental tension is **incremental safety vs. big-bang consistency**. A full-migration approach (everything migrated before shipping) maximizes consistency but carries high risk for a 1-developer project. An incremental approach (new code uses new system, old code migrates opportunistically) reduces risk but risks permanent two-system coexistence.

The resolution: **phased migration with clear phase gates** — each phase produces a shippable increment, and old code is migrated in priority order (highest-traffic screens first). The foundation is built once; migration happens over 3–6 months.

---

## Recommendation

**Choose**: Radix UI primitives + `createThemeContract` vanilla-extract + RTK Query for unary server state + Turborepo `packages/core` for eventual RN sharing.

**Because**:

1. **Radix UI** is the only headless library compatible with vanilla-extract (no Tailwind dependency), React 19, Next.js 15 SSR, and WCAG AA accessibility out of the box. All interactive Radix components require `"use client"` — confirmed by web search — but this is expected and well-documented; the pattern is to create thin `"use client"` wrapper components in the primitive library.

2. **`createThemeContract` + `createTheme`** is what ADR-009 was always pointing toward. The current `theme.css.ts` is a stepping stone. The migration is low-cost (only `VcsStatusDisplay.css.ts` currently consumes `vars`; the surface syntax doesn't change — only the internals of `theme.css.ts`). **Critical**: vanilla-extract's Next.js plugin does not support Turbopack in Next.js 15 — it requires Webpack. The `next dev --turbopack` flag in `package.json` must be removed/replaced until upgrading to Next.js 16+. This is the single most immediate build-breaking risk.

3. **RTK Query with a custom ConnectRPC `baseQuery`** is the right Phase 3 server-state move. Protobuf class instances in Redux state must be serialized to plain objects at the cache boundary using `.toJson()` from `@bufbuild/protobuf`. Terminal streaming and real-time session updates stay as manual `dispatch` hooks — RTK Query has no first-class streaming story. connect-query (TanStack) is being reworked for streaming (Issue #524, March 2025) and is less stable for this use case.

4. **Turborepo with `packages/core`** is the 2025 consensus for Next.js + Expo sharing. Expo SDK 52 auto-detects monorepo structure. ConnectRPC unary transport can be shared with RN; **server-streaming cannot** — React Native's Fetch polyfill is XHR-based and cannot stream. This means every hook that does server-streaming must stay platform-specific (web only). ConnectRPC transport should be dependency-injected into service hooks rather than imported directly, enabling a future `packages/core` to work without a web-specific transport.

**Accept these costs**:
- Every Radix primitive needs a `"use client"` wrapper component — adds one file per primitive type
- RTK Query adds ~10 KB gzip to the bundle
- Turborepo restructure (moving `web-app/` to `apps/web/`) is a one-time file-system change that touches CI/CD and all local dev scripts
- Turbopack cannot be used for development until Next.js 16 upgrade (or vanilla-extract ships Turbopack support)

**Reject these alternatives**:
- **shadcn/ui**: Requires Tailwind; directly violates ADR-009. Rejected.
- **Headless UI**: Tailwind-first ecosystem, inferior primitive coverage. Rejected.
- **connect-query for server state**: Streaming support is in flux as of March 2025. Rejected in favor of direct RTK Query integration until connect-query streaming stabilizes.
- **Nx**: Overkill for a 1-developer project. Turborepo is sufficient. Rejected.

---

## Phased Implementation Plan

### Phase 1 — Foundation (1–2 weeks)
**Goal**: Every subsequent phase builds on this without rework.

1. **Remove `--turbopack` from `next dev`** — vanilla-extract is incompatible with Turbopack in Next.js 15; this is currently causing silent HMR failures.
2. **Migrate `theme.css.ts` to `createThemeContract` + `createTheme`** — define the full token contract; implement `lightTheme` and `darkTheme`. Keep CSS custom properties in `globals.css` as a bridge for legacy `.module.css` files (they can still read them via `var()`).
3. **Build 5 core primitives** in `components/ui/`: `Button`, `Badge`, `Modal` (Radix Dialog), `Input`, `Card`. Each gets a `.css.ts` file using `recipe()` and the new token contract.
4. **Add `"use client"` wrapper pattern** — document the pattern; each Radix wrapper lives in `components/ui/`.

### Phase 2 — Mobile Critical Path (2–3 weeks)
**Goal**: App is usable on Pixel 9 Pro Fold.

5. **Fix WebGL terminal crash** — add `WebGL2RenderingContext` capability check before loading `WebglAddon`; use `onContextLoss` callback to dispose and fall back to canvas renderer. This is a P1 crash confirmed by web search.
6. **Fix touch targets in ReviewQueuePanel** — replace icon-only approve/reject buttons with full-width text buttons (`min-height: 56px`). This is the highest-priority UX defect identified in the features research.
7. **Add `touch-action: pan-y; overscroll-behavior: contain`** to xterm container — prevents pinch-to-zoom layout scramble.
8. **Add foldable breakpoint** — introduce `--breakpoint-fold: 600px` between the outer screen (~390px) and inner screen (~900px). Use `@media (horizontal-viewport-segments: 2)` (available in Chrome 138+, which is on Pixel 9 Pro Fold) as a progressive enhancement for two-column layout around the hinge.
9. **Migrate navigation shell** — rebuild `Header.tsx` + `Navigation.tsx` using new primitives; implement responsive layout that collapses to bottom nav on mobile.

### Phase 3 — Server State Cleanup (2–3 weeks)
**Goal**: Consistent data-fetching pattern; god hook broken up.

10. **Split `useSessionService`** — extract pure polling hooks (`useApprovals`, `useApprovalRules`, `useApprovalAnalytics`) into RTK Query endpoints. Streaming hooks stay as manual dispatch.
11. **Protobuf serialization boundary** — wrap RTK Query `baseQuery` with `.toJson()` to store plain objects in cache; re-enable Redux `serializableCheck`.
12. **State colocation audit** — remove `ReviewQueueContext` and `ApprovalsContext` (now redundant; Redux is source of truth). Migrate filter state to URL search params.

### Phase 4 — Full CSS Migration (ongoing, 3–6 weeks)
**Goal**: Zero `.module.css` files remain.

13. **Codemod the 70 `.module.css` files** — use an AST-based codemod (or GritQL via the `gritql` skill) to transform the most mechanical conversions; handle edge cases (`:global()`, dynamic classes) manually.
14. **Migrate screens in priority order**: session list + cards → review queue → terminal view → diff viewer → history → settings.
15. **Replace `DiffViewer.tsx` with `@codemirror/merge`** — CodeMirror 6 merge addon is mobile-friendly, DOM-based (not canvas), and ~200 KB lighter than Monaco on mobile.

### Phase 5 — RN Foundation (post-6-months)
**Goal**: Code-sharing architecture in place; ready to start RN app.

16. **Turborepo restructure** — move `web-app/` → `apps/web/`; create `packages/core` with RTK slices, ConnectRPC factory (transport-injected), shared types.
17. **Transport injection** — refactor all service hooks to accept transport as a parameter; `apps/web` provides `createConnectTransport`; future `apps/mobile` provides `createGrpcWebTransport` (unary only).

---

## Critical Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Turbopack + vanilla-extract incompatible in Next.js 15 | **Critical** | Remove `--turbopack` from dev script immediately |
| xterm.js WebGL crash on Android (no capability check) | **Critical** | Add `WebGL2RenderingContext` guard before `loadAddon()` |
| `theme.css.ts` is not a real `createThemeContract` | **High** | Phase 1 migration — 2 existing consumers, low blast radius |
| xterm Android GBoard composition event corruption | **High** | Known upstream issue (xterm.js #3600); mitigate with input event filtering + VirtualKeyboard API |
| ConnectRPC streaming not shareable with React Native | **High** | Design transport injection boundary before starting `packages/core` |
| CSS fold API requires Chrome 138+ | **Medium** | Use as progressive enhancement only; breakpoint-only layout is the baseline |
| 70-file `.module.css` migration scope | **Medium** | Codemod handles mechanical cases; phase across 6 weeks |

---

## Open Questions Before Committing

- [ ] **xterm.js GBoard input corruption** — Is the composition event corruption (xterm.js #3600) fixed in xterm 6.x, or does it require an application-level workaround? Needs a manual test on Pixel 9 Pro Fold with GBoard before designing the keyboard input layer. Blocks: Phase 2 terminal work.
- [ ] **connect-query streaming stability** — When does the connect-query v2 streaming redesign (Issue #524) land? If it ships before Phase 3 starts, it may be preferable to RTK Query for unary hooks too. Blocks: Phase 3 server state decision.
- [ ] **Turbopack timeline** — vanilla-extract's `unstable_turbopack` option is planned for Next.js 16. What is the Next.js upgrade path from 15.3.2? Blocks: re-enabling `--turbopack` in dev.

If the xterm GBoard question is unresolved before Phase 2, stub a manual text input overlay (the `VirtualKeyboard.tsx` component already exists) as a fallback.

---

## Sources

### Findings Files
- `project_plans/front-end-refactor/research/findings-stack.md`
- `project_plans/front-end-refactor/research/findings-features.md`
- `project_plans/front-end-refactor/research/findings-architecture.md`
- `project_plans/front-end-refactor/research/findings-pitfalls.md`

### Web Search Results (verified 2026-04-16)
- [vanilla-extract Next.js integration](https://vanilla-extract.style/documentation/integrations/next/) — Turbopack support only in Next.js 16+
- [Turbopack + vanilla-extract Discussion #77348](https://github.com/vercel/next.js/discussions/77348) — confirms incompatibility in Next.js 15
- [Radix UI SSR Guide](https://www.radix-ui.com/primitives/docs/guides/server-side-rendering) — all components require "use client"
- [ConnectRPC supported browsers & frameworks](https://connectrpc.com/docs/web/supported-browsers-and-frameworks/) — streaming not supported in React Native
- [ConnectRPC streaming in React Native Issue #199](https://github.com/connectrpc/connect-es/issues/199)
- [xterm.js WebGL context loss pattern](https://github.com/xtermjs/xterm.js/issues/2033) — `onContextLoss` dispose pattern confirmed
- [Viewport Segments API shipped in Chrome 138](https://developer.chrome.com/blog/viewport-segments-api-shipped)
- [Turborepo React Native starter — Vercel](https://vercel.com/templates/next.js/turborepo-react-native)
- [connect-query streaming revisit Issue #524](https://github.com/connectrpc/connect-query-es/issues/524) — streaming being reworked March 2025
- [xterm.js mobile touch Issue #5377](https://github.com/xtermjs/xterm.js/issues/5377) — confirmed limited mobile support
- [xterm.js Android GBoard Issue #3600](https://github.com/xtermjs/xterm.js/issues/3600) — composition event corruption
- [vanilla-extract migration at Glean](https://www.glean.com/blog/optimizing-our-css-at-glean) — codemod approach for 300+ files
