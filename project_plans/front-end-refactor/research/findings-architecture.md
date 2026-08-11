# Findings: Architecture

## Summary

The stapler-squad front-end has a coherent but incomplete architecture. Core locked decisions (RTK, vanilla-extract, ConnectRPC) are sound. The primary gaps are: (1) the token contract is a hand-rolled wrapper over CSS custom properties rather than a true `createThemeContract` with light/dark variants, (2) there is no primitive component layer — every feature component reimplements buttons, modals, inputs, and status badges inline, (3) ConnectRPC is managed entirely in a single monolithic service hook (`useSessionService`) with ~500 lines that doubles as both data-access layer and streaming coordinator, and (4) state colocation rules are informal — the current split between Redux slices, React Context, and URL state follows historical accident rather than documented policy.

The recommended path is: establish the token contract first (theme.css.ts → `createThemeContract`), build a primitive component layer in `web-app/src/components/ui/`, centralize ConnectRPC transport creation and define a server-state boundary, then document state colocation rules that leverage React 19 patterns. React Native sharing is a future concern that can be planned for without a monorepo restructure in the near term.

---

## Options Surveyed

### 1. Design System Layer Architecture

#### Option A: Flat Token File + Component Library (status quo + expansion)
Continue with the current `styles/theme.css.ts` as a typed wrapper over CSS custom properties in `globals.css`. Expand `components/ui/` into a full primitive library (Button, Badge, Modal, Input, etc.). No change to CSS authoring.

**Current state**: `theme.css.ts` wraps ~12 color tokens and 1 font token as string literals (`"var(--primary)"`). There is no `createThemeContract` — light/dark theming is handled by re-defining the same custom properties in a `@media (prefers-color-scheme: dark)` block in `globals.css`. `VcsStatusDisplay.css.ts` is the only `.css.ts` file consuming `vars`; all other components still use `.module.css`.

**Problems**: Cannot add a second theme (e.g., high-contrast) without duplicating `globals.css` sections. Token values are runtime strings, not build-time constants — no dark-mode safety at compile time.

#### Option B: vanilla-extract `createThemeContract` + primitive layer
Replace `theme.css.ts` with a proper `createThemeContract` that defines the token shape, then implement the light and dark themes as separate `createTheme` calls. Primitives use `recipe()` for variant systems.

**Strengths**: Full compile-time token safety for both light and dark themes. `recipe()` replaces manual `clsx()` variant assemblies across `SessionCard`, `ApprovalCard`, `FilterPill`. Consistent with ADR-009 intent. Atlassian Atlaskit and Seek's Braid design systems use this exact pattern at production scale [TRAINING_ONLY — verify].

**Weaknesses**: Requires updating `globals.css` to remove hardcoded CSS variables (or keeping them as a bridge during migration). One-time migration cost for `theme.css.ts`. Turbopack + vanilla-extract interop is still maturing as of early 2026 [TRAINING_ONLY — verify current status].

#### Option C: Panda CSS
Replace vanilla-extract + CSS modules with Panda CSS's utility-first approach. ADR-009 already rejected this for the current preference of class-based authoring. Not re-evaluated here per locked decision.

**Verdict**: Option B is the correct path. The current `theme.css.ts` is a stepping stone, not the destination. ADR-009 explicitly calls out `createTheme` / `createThemeContract` as the target API.

---

### 2. Server State with ConnectRPC

#### Option A: Hooks-dispatch-to-Redux (status quo, ADR-008 Phase 1)
All ConnectRPC logic lives in service hooks (`useSessionService`, `useApprovals`, `useReviewQueue`). Hooks dispatch to Redux slices. Each hook creates its own transport and client.

**Problems identified in ADR-008**:
- 9–10 separate transport instantiations in the original pre-RTK state (now partially consolidated)
- `useSessionService` is 500+ lines handling listing, CRUD, streaming, checkpoints, and fork — a service layer that has grown into a god hook
- Protobuf class instances in Redux state disable `serializableCheck`, which silences all serialization warnings including legitimate ones

#### Option B: RTK Query with custom ConnectRPC baseQuery
Wrap ConnectRPC unary calls with a custom `baseQuery` for RTK Query. Streaming endpoints remain as standalone hooks. ADR-008 rejected this for Phase 1 due to protobuf serialization boundary and bidirectional terminal stream. The ADR explicitly marks this as viable for Phase 3 for pure polling hooks.

**Current viability**: The `@connectrpc/connect` v2 client is just an async iterable for streaming and an async function for unary calls — both are compatible with RTK Query's `baseQuery` interface. The serialization boundary decision (serialize protobuf to plain objects at the cache layer via `.toJson()` from `@bufbuild/protobuf`) is the only real blocker. [TRAINING_ONLY — verify if community-maintained RTK Query + ConnectRPC adapters exist as of 2026]

**Incremental path**: Start with RTK Query for the three pure polling hooks (`useApprovals`, `useApprovalRules`, `useApprovalAnalytics`) by introducing a serialization boundary. Leave streaming hooks (`watchSessions`, terminal) as manual Redux dispatch. This gives RTK Query's cache invalidation, deduplication, and `isLoading`/`isError` states for the polling domain without touching the streaming architecture.

#### Option C: TanStack Query (React Query) v5 with custom ConnectRPC fetcher
Use TanStack Query instead of RTK Query for server state. Coexist with Redux for client-only state.

**Strengths**: TanStack Query v5 has improved streaming support. Better DevTools UI than RTK Query. Simpler mental model — queries identified by keys, not cache tags. The ConnectRPC team maintains `@connectrpc/connect-query` as an official TanStack Query adapter [TRAINING_ONLY — verify current package status].

**Weaknesses**: Adds a second state management dependency alongside Redux. Two separate DevTools panels. Team familiarity cost. Redux Toolkit's `createEntityAdapter` already provides normalized entity storage for sessions, which overlaps heavily with what TanStack Query would provide.

#### Option D: React Server Components + direct RPC calls (Next.js 15 App Router)
In Next.js 15 App Router, server components can call ConnectRPC directly (Node.js transport) without a client-side fetch layer.

**Viability**: The app uses `"use client"` on most components and depends on real-time streaming (terminal output, session watch stream). RSC for static/initial data (session list on page load) is viable, but the interactive streaming use cases require client-side ConnectRPC. The existing architecture is client-heavy by necessity.

**Verdict for server state**: The right near-term path is Option B (RTK Query for polling hooks only). The serialization boundary — call `.toJson()` at the RTK Query adapter boundary and store plain JSON — is the key design decision to make explicitly. Option C is worth monitoring but adds dependency complexity for a solo-developer project.

---

### 3. Monorepo Structure for Web + React Native Sharing

#### Option A: No monorepo — extract shared logic to a `packages/` directory within the repo
Create a `packages/core/` directory adjacent to `web-app/`. This package contains Redux slices, hook business logic (minus React.createElement), ConnectRPC clients, protobuf types, and utility functions. `web-app/` and a future `mobile-app/` both import from `packages/core/` via TypeScript path aliases or local package references.

**Strengths**: No Turborepo/Nx overhead for a 1-developer project. Works with `npm workspaces` or `pnpm workspaces`. Keeps the repo as a single git repository. [TRAINING_ONLY — verify workspace linking behavior with Next.js 15 and pnpm]

**What can realistically be shared**:
- Redux slices (`sessionsSlice.ts`, `approvalsSlice.ts`, `reviewQueueSlice.ts`) — these have no React dependencies, only RTK
- Selector functions — pure functions, fully shareable
- ConnectRPC transport factory and client creation logic
- Protobuf-generated types and service definitions (`gen/session/v1/`) — already platform-agnostic TypeScript
- Hook business logic extracted from React hooks into plain functions — requires decomposing current hooks into (1) pure data-fetching functions and (2) React integration wrappers

**What must be platform-specific**:
- `@connectrpc/connect-web` transport (uses browser Fetch API) — React Native needs a different transport
- Terminal streaming (`useTerminalStream`, xterm.js) — entirely web-specific
- Next.js routing (`next/navigation`, `next/link`) — React Native uses React Navigation
- CSS / vanilla-extract styles — React Native uses StyleSheet

#### Option B: Turborepo monorepo
Full Turborepo setup with `apps/web`, `apps/mobile`, `packages/core`, `packages/ui-web`, `packages/ui-native`.

**Weaknesses**: High overhead for a solo developer. Turborepo's value is primarily in parallel builds for large teams. The incremental build problem is more easily solved with TypeScript project references. [TRAINING_ONLY — verify Turborepo's current recommendation for small teams]

#### Option C: Separate repositories
Keep web and mobile as separate repos, share only through published npm packages. Highest friction for a solo developer. Not recommended.

**Verdict**: Option A (packages/ within the monorepo, npm workspaces) is appropriate for the current team size. The critical enabler is decoupling Redux slice logic and hook business logic from React-specific APIs — this is an architecture decision that should happen regardless of whether React Native is ever pursued.

---

### 4. State Colocation with React 19 + RTK

#### Current State
The app uses three state systems in parallel without documented colocation rules:
1. **Redux slices**: sessions, approvals, reviewQueue — server-originated shared data
2. **React Context**: NotificationContext, AuthContext, OmnibarContext, ReviewQueueContext, ApprovalsContext, SessionVcsContext — mixed: some are thin wrappers for Redux state, some hold ephemeral UI state
3. **Local `useState`**: per-component ephemeral state (rename modal open/closed, checkpoint labels, fork titles — `SessionCard.tsx` lines 49-60 has 8+ `useState` calls managing inline edit UX)

**Problems**:
- `ReviewQueueContext` and `ApprovalsContext` exist only as thin wrappers that create hooks and pass state down — should be eliminated now that Redux is the source of truth
- URL state is unused — filters and selected session are managed in Redux or local state, but URL-encoding them would enable shareable links and back-button support
- `SessionCard.tsx` has become a multi-responsibility component: it renders a card AND manages the state for 5+ different inline editor modals (rename, tag edit, checkpoint, fork, restart confirm)

#### React 19 Relevance

**`useOptimistic`**: Relevant for session operations (pause, resume, delete) where the UI should respond immediately while the RPC call is in-flight. The current architecture dispatches to Redux synchronously on RPC success, causing a visible delay on state change. `useOptimistic` could drive the Redux dispatch pattern: dispatch optimistic action → fire RPC → confirm or rollback. [TRAINING_ONLY — verify `useOptimistic` interaction with Redux dispatch]

**`use()` (Promise unwrapping)**: Useful for suspending on async data in server components. Less relevant for this client-heavy app's interactive flows.

**Server Actions**: Not applicable — the app communicates exclusively via ConnectRPC, not Next.js form actions.

#### Recommended Colocation Rules

| State Category | Where it Lives | Rationale |
|---|---|---|
| Server-originated shared data (sessions, approvals) | Redux slice | Accessed by multiple components, serializable identity, DevTools visibility |
| Server-originated session-scoped data (VCS status, diffs) | React Context (scoped to SessionDetail) | Single subtree owner, no cross-feature sharing needed |
| Ephemeral UI modal state (is-open booleans) | Local `useState` in modal trigger | Owned by one component, transient, reset on unmount |
| Multi-step form state | Local `useState` or `useReducer` | Component-scoped, throw away on cancel |
| Filter and selection state | URL search params (`useSearchParams`) | Shareable, survives refresh, integrates with browser history |
| Auth state | React Context (`AuthContext`) | App-wide but not Redux because it pre-gates all RPC calls |
| Omnibar open/close | React Context (`OmnibarContext`) | UI-only, single-subtree, fine as Context |

---

## Trade-off Matrix

| Axis | Option A (Flat tokens + ad-hoc) | Option B (createThemeContract + primitives + RTK Query boundary) | Option C (Full Turborepo + TanStack Query) |
|---|---|---|---|
| Separation of concerns | Low — CSS tokens, component logic, and API calls mixed together | High — token contract, primitive layer, server state layer each explicit | High, but creates boundary overhead |
| Code reuse web/RN | None — all code is web-specific | Medium — slices/selectors extractable to packages/core | High — but requires full monorepo migration |
| Tooling complexity | Low — current setup unchanged | Medium — adds `createThemeContract`, RTK Query for polling, TypeScript project refs | High — Turborepo, pnpm workspaces, separate build pipelines |
| Incremental adoptability | N/A (status quo) | High — each layer independently adoptable; ADR-008/009 already paved the way | Low — big-bang restructuring required |
| 1-developer team fit | Good (minimal overhead) | Good (adds structure where pain is felt) | Poor (overhead exceeds benefit at this team size) |

**Recommended option**: B (createThemeContract + primitive layer + RTK Query boundary for polling hooks).

---

## Risk and Failure Modes

### R1: vanilla-extract `createThemeContract` and Turbopack interop
ADR-009 notes that Turbopack + `@vanilla-extract/next-plugin` is newer than the webpack integration and "may have issues." As of early 2026, the project runs `next dev --turbopack`. If `createThemeContract` triggers a Turbopack-specific bug, the development server breaks.

**Mitigation**: Test the `createThemeContract` migration with a production webpack build (`next build`) first. Keep `next dev --turbopack` as the daily driver and fall back to `next dev` if issues arise.

### R2: Protobuf class instances in Redux breaking with RTK Query
RTK Query normalizes cache entries by serializing them. Protobuf `Message` instances from `@bufbuild/protobuf` are class instances with non-enumerable internal fields. If passed directly to RTK Query's cache without a serialization boundary, DevTools will show corrupt state.

**Mitigation**: Introduce an explicit serialization boundary at the RTK Query `baseQuery` adapter: call `response.toJson()` (protobuf's built-in JSON serializer) before returning from the query function. The Redux slice selectors would need updating to work with plain objects instead of class instances. [TRAINING_ONLY — verify that `@bufbuild/protobuf` v2 `toJson()` output is fully RTK Query compatible]

### R3: `SessionCard` complexity explosion
`SessionCard.tsx` currently manages 8+ local state items for inline modals. Adding more features will make this component unmanageable.

**Mitigation**: Extract each modal into its own component (`RenameModal`, `CheckpointModal`, `ForkModal`). `SessionCard` becomes a thin orchestrator.

### R4: `packages/core/` import resolution in Next.js
When `web-app/` imports from a local `packages/core/` package, Next.js needs to be configured to transpile that package (via `transpilePackages` in `next.config.ts`). Without this, TypeScript decorators, JSX in shared hooks, or ESM-only dependencies may fail to compile.

**Mitigation**: Use `transpilePackages: ['@stapler-squad/core']` in `next.config.ts`. Keep the shared package pure TypeScript with no JSX and no framework-specific imports. [TRAINING_ONLY — verify `transpilePackages` behavior with Next.js 15]

### R5: URL state + Redux state divergence
If filters move to URL search params while selected session ID stays in Redux, there is a risk of inconsistency on back-navigation.

**Mitigation**: Treat URL as the source of truth for filters. Derive filter state entirely from URL via `useSearchParams()` and remove filter state from Redux.

---

## Migration and Adoption Cost

### Phase 1: Token Contract (1–2 days)
1. Convert `styles/theme.css.ts` from a hand-rolled `vars` wrapper to a proper `createThemeContract` + `createTheme` for light and dark themes.
2. Update `globals.css` to remove redundant CSS custom property definitions (or keep them as a bridge for `.module.css` files still in migration).
3. Update `VcsStatusDisplay.css.ts` (already uses `vars`) to verify the migration works.

**Cost**: Low. Only 2 `.css.ts` files consume `vars` today.

### Phase 2: Primitive Component Library (3–5 days)
1. Audit `components/ui/` for patterns that recur across feature components: button variants, badge/pill, modal skeleton, input field, skeleton loader.
2. Implement each as a `recipe()` in a new `.css.ts` file and a `.tsx` file under `components/ui/`.
3. Replace inline implementations in `SessionCard`, `ApprovalCard`, `FilterPill`, `TagEditor`.

**Cost**: Medium. Each primitive requires defining variants, writing the recipe, and updating consumers. Can be done incrementally — one primitive per PR.

### Phase 3: ConnectRPC / Server State Boundary (2–3 days)
1. Create a shared transport factory (`lib/transport.ts`) that returns a singleton transport.
2. Introduce a serialization utility (`lib/proto-utils.ts`) with `sessionToJson()` and `jsonToSession()` helpers.
3. Migrate `useApprovals`, `useApprovalRules`, and `useApprovalAnalytics` to RTK Query endpoints using the serialization boundary. Keep `useSessionService` and streaming hooks as-is.

**Cost**: Medium. The serialization boundary adds boilerplate but the RTK Query migration for pure polling hooks is straightforward.

### Phase 4: State Colocation Cleanup (1–2 days)
1. Remove `ReviewQueueContext` and `ApprovalsContext` thin wrappers.
2. Move filter state to URL search params.
3. Extract `SessionCard` modal state into dedicated modal components.

**Cost**: Low to medium.

### Phase 5: packages/core extraction (2–3 days, only if React Native is planned)
1. Set up `npm workspaces` with `packages/core/`.
2. Configure TypeScript project references.
3. Update `web-app/` to import from `@stapler-squad/core`.

**Total estimated cost for Phases 1–4**: 7–12 developer-days, fully incremental, no big-bang.

---

## Operational Concerns

### Build Time
vanilla-extract compiles `.css.ts` files at build time. Adding more `.css.ts` files for the primitive library will increase build time, but vanilla-extract compilation is fast. The existing CI pipeline should absorb the change without noticeable regression. [TRAINING_ONLY — verify build time benchmarks with large vanilla-extract component libraries]

### Bundle Size
RTK Query adds approximately 9–11 KB gzipped to the JavaScript bundle [TRAINING_ONLY — verify current RTK Query bundle size]. Given the existing `@reduxjs/toolkit` dependency, the incremental cost is only the RTK Query runtime itself (RTK Query is part of the same package). The project has a 5 MB total JS size-limit budget — RTK Query fits comfortably.

### Developer Experience
- `createThemeContract` gives autocomplete and compile-time errors for all `vars.xxx` references — a significant DX improvement over the current string-literal approach
- RTK Query's `isLoading`, `isError`, `refetch` API surface is more ergonomic than manual `dispatch(setLoading(true))` / `dispatch(setError(...))` patterns
- URL-encoded filters enable direct linking to filtered session views

### Dark Mode
The current theming approach (CSS custom properties re-defined in `@media prefers-color-scheme`) works but has no compile-time safety for dark-mode token values. `createThemeContract` with two `createTheme` calls (light and dark) gives compile-time verification that all tokens are defined in both themes.

---

## Prior Art and Lessons Learned

### Shopify Polaris (vanilla-extract token system)
Shopify Polaris v12+ migrated from a flat CSS custom property system to vanilla-extract with a typed token contract. Key lesson: the token contract should have a semantic layer (`color.text.primary`) over a primitive layer (`palette.blue.600`). Components reference only the semantic layer. [TRAINING_ONLY — verify Polaris v12 migration specifics]

**Applicable pattern**: stapler-squad's `globals.css` already uses semantic names (`--text-primary`, `--primary`). The missing step is the `createThemeContract` that makes this TypeScript-typed.

### GitHub Primer (design system layer hierarchy)
GitHub Primer uses a three-tier hierarchy: primitives (raw values), functional tokens (semantic meaning), and component tokens (component-specific). For a small project like stapler-squad, collapsing primitive and functional into a single semantic tier is appropriate.

**Applicable pattern**: One semantic tier with `vars.color.xxx`, `vars.space.xxx`, `vars.font.xxx` is sufficient. Do not over-engineer the token hierarchy.

### Atlassian Atlaskit (vanilla-extract at scale)
Atlaskit uses `createThemeContract` with multiple theme implementations. Key lesson: keep the contract shape flat — deeply nested token objects cause TypeScript type inference to slow down on large files. [TRAINING_ONLY — verify Atlaskit's current design token architecture]

### RTK Query + ConnectRPC community patterns
The ConnectRPC/connect-es ecosystem has a `@connectrpc/connect-query` package for TanStack Query integration but not an official RTK Query adapter as of late 2024 [TRAINING_ONLY — verify current 2026 status]. Custom `baseQuery` implementations wrapping ConnectRPC clients follow the same pattern as custom REST adapters and are documented in community blog posts.

### React 19 `useOptimistic` lessons
`useOptimistic` is designed for form/action flows. For ConnectRPC CRUD operations through Redux dispatch, the pattern is: dispatch an optimistic action (e.g., set status to PAUSED before the RPC completes), await the RPC, then dispatch the confirmed result or roll back. This is achievable with the existing RTK slice pattern — the `updateSessionStatus` action can be dispatched optimistically and corrected by the streaming `watchSessions` event. [TRAINING_ONLY — verify best practice for `useOptimistic` + Redux combined state]

---

## Open Questions

1. **Serialization boundary for RTK Query**: Should protobuf messages be converted to plain JSON at the RTK Query adapter boundary, or should the Redux store continue to hold class instances (with `serializableCheck: false`)? The former enables DevTools time-travel but requires maintaining serialization/deserialization helpers.

2. **Theme toggle vs. media query**: Should dark mode be media-query-driven (current approach) or class-driven (vanilla-extract `assignVars` + JavaScript toggle)? Class-driven enables a user preference toggle in the UI.

3. **URL state scope**: Which state should live in the URL? Candidates: `sessionId`, `filter.status`, `filter.tags`, `filter.category`, `groupBy`. This is a product decision as much as an architecture decision.

4. **`@connectrpc/connect-query` adoption**: If this official package has evolved to support streaming and has an RTK Query variant, it may be a better-maintained option than a custom adapter. [TRAINING_ONLY — needs web search]

5. **vanilla-extract `recipe()` vs. `clsx` migration scope**: Should the migration to `recipe()` be done component-by-component or in a single pass for all `components/ui/` primitives?

6. **React Native transport**: ConnectRPC's web transport uses the Fetch API. React Native's Hermes runtime `fetch` differs from browser Fetch. Is `@connectrpc/connect-web` directly usable in React Native? [TRAINING_ONLY — needs web search]

---

## Recommendation

Execute in four phases, each independently shippable:

**Phase 1 (Token Contract)**: Upgrade `styles/theme.css.ts` to use `createThemeContract` + two `createTheme` implementations (light, dark). This is the highest-leverage step: eliminates the class of silent CSS bug that motivated ADR-009 and enables type-safe dark mode. **Start here.**

**Phase 2 (Primitive Library)**: Build a primitive component set in `components/ui/` covering: `Button` (intent/size variants via `recipe()`), `Badge`/`Pill`, `Modal`, `Input`, `Select`. Replace the most common inline reimplementations. This directly addresses the "every screen reimplements buttons, modals, inputs" pain point.

**Phase 3 (Server State Boundary)**: Create a single shared transport factory. Migrate the three pure polling hooks to RTK Query with a protobuf serialization boundary. Document the rule: "streaming hooks stay as manual Redux dispatch; unary polling hooks use RTK Query."

**Phase 4 (State Colocation Cleanup)**: Remove redundant Context providers. Move filter state to URL search params. Extract `SessionCard` modal state into dedicated modal components.

**Hold**: React Native sharing (`packages/core/` extraction) until there is a concrete mobile app plan. The architecture choices in Phases 1–4 make the extraction easier when needed — Redux slices will have no React dependencies, and the protobuf serialization boundary will already exist.

---

## Pending Web Searches

The following queries should be executed to verify training-knowledge claims marked `[TRAINING_ONLY]`:

1. `@connectrpc/connect-query tanstack query streaming support 2025 2026`
   Verify whether the official ConnectRPC TanStack Query adapter supports streaming RPCs and whether an RTK Query variant exists.

2. `vanilla-extract createThemeContract turbopack next.js 15 compatibility 2026`
   Verify current status of vanilla-extract + Turbopack interop with Next.js 15.

3. `@bufbuild/protobuf toJson RTK Query redux serializable plain object 2025`
   Verify that `Message.toJson()` from `@bufbuild/protobuf` v2 produces a plain-object structure compatible with Redux's serializability requirements.

4. `shopify polaris vanilla-extract design token migration createThemeContract`
   Verify Polaris's vanilla-extract adoption pattern and token contract structure.

5. `connectrpc connect-web react native fetch transport hermes 2025`
   Verify whether `@connectrpc/connect-web` works in React Native (Hermes runtime) or requires a custom transport.

6. `@reduxjs/toolkit rtk query bundle size gzip kb 2025`
   Verify current RTK Query bundle size impact when added to an existing RTK project.

7. `next.js 15 transpilePackages npm workspaces typescript project references local package`
   Verify the correct setup for importing from a local `packages/core/` workspace package in Next.js 15 with TypeScript project references.

8. `react 19 useOptimistic redux dispatch pattern server state`
   Find examples of `useOptimistic` used alongside Redux dispatch (rather than Server Actions) to verify the optimistic update pattern for ConnectRPC mutations.

9. `atlassian atlaskit design token vanilla-extract createThemeContract architecture 2025`
   Verify Atlaskit's current token contract architecture and lessons from large-scale vanilla-extract adoption.
