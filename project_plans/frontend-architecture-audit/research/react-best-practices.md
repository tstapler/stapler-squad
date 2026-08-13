# React Best Practices Research — Stack-Specific Findings

> Stack: React 18 + Next.js App Router · TypeScript strict · ConnectRPC (protobuf streaming) · RTK Query + Redux · vanilla-extract · Custom React Context (9 global providers)
> Research date: 2026-05-08

---

## 1. ConnectRPC + React Patterns

### What the ConnectRPC team recommends

The canonical ConnectRPC React pattern is a **single transport instance created at app initialisation**, exposed via a `TransportProvider`. The official `@connectrpc/connect-query-es` package (the ConnectRPC-endorsed TanStack Query wrapper) ships a `TransportProvider` component and a `useTransport()` hook precisely for this purpose. React components and hooks call `useTransport()` to obtain the singleton — they never call `createConnectTransport()` themselves.

Source: [connectrpc.com/docs/web/using-clients](https://connectrpc.com/docs/web/using-clients/) and [connectrpc/connect-query-es](https://github.com/connectrpc/connect-query-es)

### The current problem (48 ad-hoc transports)

Each call to `createConnectTransport()` in a hook allocates a new transport object with its own header configuration. This means:
- Auth interceptors must be duplicated or are simply missing from some hooks
- Timeout/retry policies diverge silently
- In React Strict Mode (double-mount), each hook creates two transports that are both active briefly — GC risk for streaming connections

### Recommended pattern for this codebase

```ts
// lib/transport/transport.ts  — created ONCE, module-level singleton
import { createConnectTransport } from "@connectrpc/connect-web";
import { authInterceptor } from "./authInterceptor";

export const transport = createConnectTransport({
  baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL!,
  interceptors: [authInterceptor],
});
```

```tsx
// app/providers.tsx  — wrap the App Router root layout
import { TransportProvider } from "@connectrpc/connect-query";
import { transport } from "../lib/transport/transport";

export function Providers({ children }: { children: React.ReactNode }) {
  return <TransportProvider transport={transport}>{children}</TransportProvider>;
}
```

```ts
// Any hook that previously called createConnectTransport()
import { useTransport } from "@connectrpc/connect-query";
import { createPromiseClient } from "@connectrpc/connect";
import { SessionService } from "../../gen/session/v1/session_connect";

export function useSessionClient() {
  const transport = useTransport();
  return useMemo(() => createPromiseClient(SessionService, transport), [transport]);
}
```

### Auth interceptor pattern

ConnectRPC interceptors are closures — they can read from module-level token storage (e.g., a cookie getter or a Zustand atom's `getState()`) without requiring a hook:

```ts
// lib/transport/authInterceptor.ts
import type { Interceptor } from "@connectrpc/connect";

export const authInterceptor: Interceptor = (next) => async (req) => {
  const token = getSessionToken(); // reads from cookie or module-level store
  req.header.set("Authorization", `Bearer ${token}`);
  return next(req);
};
```

For token refresh on `Code.Unauthenticated`, the interceptor catches the error, refreshes the token, and retries — all within the single shared interceptor. Source: [connectrpc/connect-es Discussion #323](https://github.com/connectrpc/connect-es/discussions/323)

### Streaming hooks

Streaming calls (`ServerStreaming`, `BidiStreaming`) hold a reference to the transport for the connection's lifetime. A singleton transport is required for streaming stability — per-hook transports would create a new WebSocket connection per component mount.

### Migration cost for this codebase

- Create `lib/transport/` with `transport.ts` and `authInterceptor.ts`
- Add `<TransportProvider>` to `app/providers.tsx` (or equivalent layout)
- Replace `createConnectTransport()` call in each of 48 hook files with `useTransport()`
- Test DI: wrap test renders with `<TransportProvider transport={createRouterTransport(...)}>` (no module mocking needed)

The `createRouterTransport` from `@connectrpc/connect` is the official in-memory mock transport for unit tests — it replaces MSW for ConnectRPC-specific tests. Source: [connectrpc.com/docs/web/testing](https://connectrpc.com/docs/web/testing/)

---

## 2. Context Performance at Scale

### The problem with 9 nested providers

When any value in a Context changes, every component that calls `useContext(ThatContext)` re-renders — regardless of whether the changed field is the one the component uses. With 9 providers and a large-component layout (cockpit, tabs, terminal), a session state change can trigger dozens of unnecessary renders.

### Pattern 1: Context splitting (stable vs. volatile)

Split each context into two: one for values that never change after mount (dispatch functions, refs, callbacks), and one for the volatile state slice. Components that only need stable values never re-render on state changes.

```ts
// Before: one context with everything
const CockpitContext = createContext<{ sessions: Session[]; dispatch: Dispatch; ... }>();

// After: stable + volatile split
const CockpitDispatchContext = createContext<Dispatch>(noop);   // stable — never changes
const CockpitStateContext = createContext<CockpitState>(initial); // volatile — updates frequently
```

This is the React team's own recommendation for high-frequency contexts. The `CockpitActionsContext` (20-slot callback bag) is an ideal candidate: actions never change after mount, so they belong in a stable context that causes zero re-renders when cockpit state updates.

### Pattern 2: `useSyncExternalStore` for selector-based subscriptions

For contexts that still cause too many re-renders after splitting, replace the context value with an external store:

```ts
// The context provides only {subscribe, getSnapshot} — never changes
const StoreContext = createContext<Store<SessionState>>(defaultStore);

// Consumers select only the slice they need
function useSessionName(id: string) {
  const store = useContext(StoreContext);
  return useSyncExternalStore(
    store.subscribe,
    () => store.getSnapshot().sessions.find(s => s.id === id)?.name ?? ""
  );
}
```

`useSyncExternalStore` bypasses React's context propagation model — only components whose selected slice actually changed re-render. This is the mechanism Redux (and therefore RTK Query) uses internally via `react-redux`. Source: [azguards.com — useSyncExternalStore performance](https://azguards.com/performance-optimization/the-propagation-penalty-bypassing-react-context-re-renders-via-usesyncexternalstore/)

### Pattern 3: Zustand or Jotai for cross-cutting state

For state that is accessed by many components at different tree depths (e.g., active session ID, global navigation state), Zustand is the pragmatic choice: it is a `useSyncExternalStore` wrapper with selector memoisation built in, requires no provider for global atoms, and coexists cleanly with Redux. However, given Redux is already in use for RTK Query, adding Zustand introduces a third state primitive — evaluate this only if context splitting alone does not resolve the re-render issues.

### Specific recommendation for `CockpitActionsContext`

The 20-slot callback bag causes re-renders whenever any single callback reference changes. Fix with `useReducer` + dispatch context split:
1. Replace the callback bag with a `dispatch` function (stable identity)
2. Move all action implementations into the reducer
3. Components call `dispatch({ type: "ACTION_NAME", payload })` instead of calling the callback directly

This eliminates all re-renders caused by `CockpitActionsContext` entirely, because `dispatch` is a stable reference.

---

## 3. Large Component Decomposition

### Context for this codebase

7 components exceed 1,100 lines in one directory. This is a mix of data fetching, business logic, event wiring, and JSX. No single decomposition pattern solves all cases — the choice depends on what is coupled.

### Pattern A: Custom hooks extraction (highest priority, lowest risk)

Extract all data fetching and derived state into custom hooks. The component file becomes JSX only. This is the modern React replacement for the container/presenter class pattern.

```tsx
// Before: SessionDetail.tsx — 1,400 lines with useEffect, useState, fetch calls mixed with JSX
// After:
function SessionDetail({ sessionId }: Props) {
  const session = useSessionData(sessionId);         // fetching + derived state
  const terminal = useTerminalState(sessionId);      // terminal-specific state
  const actions = useSessionActions(sessionId);      // callbacks
  return <SessionDetailView session={session} terminal={terminal} actions={actions} />;
}
```

The presenter (`SessionDetailView`) becomes purely a function of props — fully testable without mocking ConnectRPC. Source: [frontendmastery.com — advanced composition](https://frontendmastery.com/posts/advanced-react-component-composition-guide/)

### Pattern B: Compound components for the terminal + tabs layout

For UI with implicit shared state between subcomponents (tabs, panels, panes), compound components with a context bridge are cleaner than prop threading:

```tsx
<CockpitLayout>
  <CockpitLayout.TabStrip />
  <CockpitLayout.PaneArea>
    <CockpitLayout.Pane paneId="main">
      <SessionTerminal />
    </CockpitLayout.Pane>
  </CockpitLayout.PaneArea>
  <CockpitLayout.ActionBar />
</CockpitLayout>
```

The parent `CockpitLayout` owns layout state (active tab, split state); subcomponents read it via a layout-internal context that does not leak to the broader app context graph. Source: [patterns.dev — Compound Pattern](https://www.patterns.dev/react/compound-pattern/)

### Pattern C: Render props — avoid

Render props were the pre-hooks answer to logic sharing. With hooks available, render props add nesting and are harder to type in TypeScript strict mode. Use custom hooks instead.

### Decomposition priority order for this codebase

1. **Hook extraction first** — reduces line count immediately, no architectural risk
2. **Compound components for cockpit layout** — addresses the dual session-creation UI and tab layout
3. **Context splitting** (as described in §2) — after hook extraction reveals which state is truly cross-cutting vs. local

---

## 4. Vanilla-extract at Scale

### Theme contract enforcement

The `createThemeContract` / `createTheme` pattern creates a type-safe token contract that TypeScript enforces at compile time — referencing `vars.color.undefined` is a compile error. The contract lives in one file; themes implement it.

```ts
// styles/theme.css.ts
import { createThemeContract, createTheme } from "@vanilla-extract/css";

export const vars = createThemeContract({
  color: {
    actionPrimary: null,
    statusDanger: null,
    chartSeries: { "0": null, "1": null, "2": null, "3": null },
  },
  space: { "1": null, "2": null, "4": null },
});

export const lightTheme = createTheme(vars, {
  color: {
    actionPrimary: "#3b82f6",
    statusDanger: "#ef4444",
    chartSeries: { "0": "#6366f1", "1": "#f59e0b", "2": "#10b981", "3": "#f43f5e" },
  },
  space: { "1": "4px", "2": "8px", "4": "16px" },
});
```

### ESLint enforcement

`@antebudimir/eslint-plugin-vanilla-extract` is the production ESLint plugin for vanilla-extract. Its `prefer-theme-tokens` rule parses `.css.ts` files and flags hardcoded hex values, px sizes, and other literals that match a token category, suggesting the correct `vars.xxx` reference. Source: [npmjs.com/@antebudimir/eslint-plugin-vanilla-extract](https://www.npmjs.com/package/@antebudimir/eslint-plugin-vanilla-extract)

Configuration:
```js
// eslint.config.js (flat config)
import vanillaExtract from "@antebudimir/eslint-plugin-vanilla-extract";

export default [
  {
    plugins: { "vanilla-extract": vanillaExtract },
    rules: {
      "vanilla-extract/prefer-theme-tokens": "error",
      "vanilla-extract/no-duplicate-selectors": "warn",
    },
  },
];
```

### Data visualisation color tokens

Chart/dataviz colors require a separate token namespace — they are not semantic UI colors and should not be placed in the same contract section as `actionPrimary` etc. Recommended structure:

```ts
chart: {
  series: { "0": null, "1": null, "2": null, "3": null, "4": null },
  axis: null,
  grid: null,
  tooltip: { background: null, text: null },
}
```

Each chart theme (light, dark, print) implements the full `chart` contract. This way switching themes automatically recolors charts without component changes.

### Tree-shaking at scale

Vanilla-extract generates one CSS file per `.css.ts` file by default. In large design systems, use `createThemeContract` in one file and `createTheme` implementations in separate files, one per theme — this enables Next.js to tree-shake unused themes and only include the active theme's CSS. Source: [vanilla-extract Discussion #156 — design system performance tuning](https://github.com/vanilla-extract-css/vanilla-extract/discussions/156)

### What the vanilla-extract team does not provide

There is no official vanilla-extract CLI linter or token coverage report. The ESLint plugin above is community-maintained. For coverage reporting (which tokens are unused), a custom script using `ts-morph` to parse `.css.ts` files and cross-reference against `theme.css.ts` is the current best approach.

---

## 5. RTK Query vs. TanStack Query

### Current state

This codebase uses RTK Query for approvals/periodic polling and custom Redux hooks for ConnectRPC streaming. This is already a dual pattern. The migration question is whether consolidating on TanStack Query (via `@connectrpc/connect-query-es`) simplifies the stack.

### What `connect-query-es` provides

`@connectrpc/connect-query-es` is the ConnectRPC team's official TanStack Query wrapper. It provides:
- `useQuery` / `useInfiniteQuery` wrappers pre-configured with protobuf-derived `queryKey` and `queryFn`
- `TransportProvider` + `useTransport()` (the singleton transport pattern from §1)
- Streaming: active development as of March 2025 to support `streamedQuery` natively in TanStack Query (see [Issue #524](https://github.com/connectrpc/connect-query-es/issues/524))

Source: [connectrpc.com/docs/web/query/getting-started](https://connectrpc.com/docs/web/query/getting-started/)

### Trade-off matrix

| Dimension | RTK Query (keep) | TanStack Query v5 + connect-query-es (migrate) |
|-----------|-----------------|------------------------------------------------|
| ConnectRPC integration | Manual (custom hooks) | First-class (protobuf queryKey, official wrapper) |
| Streaming support | Manual Redux dispatch | In-progress native `streamedQuery` (2025) |
| Redux dependency | Required | Not required — can drop Redux for data fetching |
| Auth interceptor | Per-hook or Redux middleware | Single transport interceptor (§1) |
| Bundle size | Redux + RTK overhead | Smaller — TanStack Query is standalone |
| Cache invalidation | Tag-based (powerful) | Key-based + manual invalidation |
| Migration cost | N/A | High — all RTK Query endpoints must be rewritten |
| Existing team knowledge | Assumed present | Learning curve for connect-query-es patterns |

### Dominant trade-off

RTK Query's tag-based cache invalidation is more powerful for mutation-driven refetch patterns (e.g., approve an item → invalidate the approval list). TanStack Query's key-based model requires explicit `invalidateQueries` calls. For this codebase's approvals use case, this is a meaningful downgrade unless mutation hooks are carefully designed.

### Recommendation

**Do not migrate RTK Query to TanStack Query as a standalone refactor.** Instead:

1. **Phase 1 (immediate)**: Introduce `@connectrpc/connect-query-es` alongside RTK Query for *new* ConnectRPC hooks only — this eliminates new ad-hoc transports without touching existing code
2. **Phase 2 (conditional)**: If streaming support in `connect-query-es` stabilises (watch Issue #524), migrate streaming hooks from custom Redux to `streamedQuery`
3. **Phase 3 (optional)**: Only if Redux is removed for other reasons, migrate RTK Query approvals endpoints to TanStack Query mutations

Coexistence of RTK Query and TanStack Query is explicitly supported — both can share the same `QueryClient` boundary. Source: [TanStack comparison docs](https://tanstack.dev/query/v5/docs/framework/react/comparison)

---

## 6. React 18/19 Features

### React 18 features available now

**`startTransition` / `useTransition`**: Marks a state update as non-urgent, allowing React to yield to higher-priority updates (user input). Applicable to session list filtering and search — wrapping the filter dispatch in `startTransition` keeps the input responsive while the list re-renders.

**Streaming SSR with Suspense**: Relevant only if Next.js App Router Server Components are in use. ConnectRPC streaming is client-initiated, so server-side streaming SSR does not apply to ConnectRPC subscriptions.

**`useDeferredValue`**: Useful for heavy derived computations (e.g., filtering a large session list). Cheaper than `startTransition` for read-only derived state.

### React 19 `use()` hook

The `use(promise)` hook suspends a component until the promise resolves and integrates with `<Suspense>`. It is a cleaner replacement for `useEffect` + `useState` for one-shot data fetches:

```tsx
function SessionMeta({ sessionId }: Props) {
  const meta = use(fetchSessionMeta(sessionId)); // suspends until resolved
  return <MetaView meta={meta} />;
}
```

**Compatibility with ConnectRPC streaming**: The `use()` hook works with Promises, not async iterables. ConnectRPC streaming calls return `AsyncIterable<T>` — they are **not** directly wrappable with `use()`. Streaming hooks must continue to use `useEffect` + `useState` (or TanStack Query's `streamedQuery` when available). The `use()` hook is applicable to unary ConnectRPC calls via `createPromiseClient`.

**Error boundaries requirement**: React 19 Suspense requires `<ErrorBoundary>` wrappers around every `<Suspense>` — a rejected promise bubbles to the nearest error boundary, not to the component. This means adopting `use()` requires a systematic error boundary audit. Source: [dev.to — React 19 Suspense Deep Dive](https://dev.to/a1guy/react-19-suspense-deep-dive-data-fetching-streaming-and-error-handling-like-a-pro-3k74)

### What React 19 does NOT simplify for this stack

- Server Components: not applicable to ConnectRPC WebSocket streaming (client-side only)
- Actions: useful for form mutations but not for RPC streaming patterns
- The `use(context)` form: syntactic sugar over `useContext()` — no performance benefit

### Recommended adoption order

1. `startTransition` for session list filtering — low risk, immediate benefit
2. `useDeferredValue` for heavy list renders — drop-in addition
3. `use()` hook for unary RPC calls after error boundary audit — medium effort
4. `streamedQuery` (TanStack/ConnectRPC) when Issue #524 lands — replaces streaming `useEffect` hooks

---

## 7. Testing Patterns for RPC Hooks

### ConnectRPC's official testing primitive: `createRouterTransport`

`createRouterTransport` creates an in-memory server from your own RPC handler implementations. It is the ConnectRPC team's recommended approach for unit-testing React components and hooks:

```ts
import { createRouterTransport } from "@connectrpc/connect";
import { SessionService } from "../../gen/session/v1/session_connect";

const mockTransport = createRouterTransport(({ service }) => {
  service(SessionService, {
    listSessions: () => ({ sessions: [{ id: "1", name: "test" }] }),
    streamEvents: async function* () {
      yield { event: "created", sessionId: "1" };
    },
  });
});
```

The mock transport is then injected via `<TransportProvider transport={mockTransport}>` in tests — no `jest.mock()` module patching, no MSW handler setup. Source: [connectrpc.com/docs/web/testing](https://connectrpc.com/docs/web/testing/)

### MSW and ConnectRPC

MSW (Mock Service Worker) intercepts at the network layer (HTTP). ConnectRPC over WebSocket bypasses MSW's HTTP interception. There is an open issue ([#825](https://github.com/connectrpc/connect-es/issues/825)) for MSW support, unresolved as of 2025. **Do not use MSW for ConnectRPC hook tests** — use `createRouterTransport` instead.

MSW remains appropriate for testing non-ConnectRPC HTTP calls (e.g., Next.js API routes, third-party REST APIs).

### Three-tier testing strategy for this codebase

| Tier | Tool | What it tests | Speed |
|------|------|---------------|-------|
| Unit | `createRouterTransport` + RTL | Hook logic, error handling, loading states | Fast (~50ms) |
| Integration | Real server (test instance) | Full RPC round-trip, streaming | Slow (~2s) |
| E2E | Playwright | User flows end-to-end | Slowest (~10s) |

### Hook unit test pattern

```tsx
import { renderHook, waitFor } from "@testing-library/react";
import { TransportProvider } from "@connectrpc/connect-query";

function wrapper({ children }: { children: React.ReactNode }) {
  return <TransportProvider transport={mockTransport}>{children}</TransportProvider>;
}

it("returns sessions on success", async () => {
  const { result } = renderHook(() => useSessionList(), { wrapper });
  await waitFor(() => expect(result.current.sessions).toHaveLength(1));
});
```

### Normalising the 3 current error-handling patterns

The three patterns (Redux dispatch string, RTK Query error shape, silent swallow) should converge on a single type:

```ts
// lib/types/rpc-error.ts
export type RpcError =
  | { kind: "connect"; code: ConnectErrorCode; message: string }
  | { kind: "network"; message: string }
  | { kind: "unknown"; raw: unknown };

export function toRpcError(err: unknown): RpcError {
  if (err instanceof ConnectError) {
    return { kind: "connect", code: err.code, message: err.message };
  }
  if (err instanceof TypeError) {
    return { kind: "network", message: err.message };
  }
  return { kind: "unknown", raw: err };
}
```

Every hook that catches errors uses `toRpcError()`. RTK Query error shapes are mapped at the RTK Query `baseQuery` level. This gives tests a single error shape to assert against.

---

## Recommended Reading

**ConnectRPC + React**
- `@connectrpc/connect-query-es` — [github.com/connectrpc/connect-query-es](https://github.com/connectrpc/connect-query-es)
- ConnectRPC web testing docs — [connectrpc.com/docs/web/testing](https://connectrpc.com/docs/web/testing/)
- ConnectRPC interceptors — [connectrpc.com/docs/web/interceptors](https://connectrpc.com/docs/web/interceptors/)
- Auth interceptor discussion — [github.com/connectrpc/connect-es/discussions/323](https://github.com/connectrpc/connect-es/discussions/323)

**Context Performance**
- useSyncExternalStore performance pattern — [azguards.com — propagation penalty](https://azguards.com/performance-optimization/the-propagation-penalty-bypassing-react-context-re-renders-via-usesyncexternalstore/)
- Thoughtspile — context dangers — [thoughtspile.github.io/react-context-dangers](https://thoughtspile.github.io/2021/10/04/react-context-dangers/)

**Component Decomposition**
- Frontend Mastery — advanced React composition — [frontendmastery.com/posts/advanced-react-component-composition-guide](https://frontendmastery.com/posts/advanced-react-component-composition-guide/)
- patterns.dev — compound pattern — [patterns.dev/react/compound-pattern](https://www.patterns.dev/react/compound-pattern/)

**Vanilla-extract**
- `@antebudimir/eslint-plugin-vanilla-extract` — [npmjs.com/package/@antebudimir/eslint-plugin-vanilla-extract](https://www.npmjs.com/package/@antebudimir/eslint-plugin-vanilla-extract)
- vanilla-extract design system performance — [github.com/vanilla-extract-css/vanilla-extract/discussions/156](https://github.com/vanilla-extract-css/vanilla-extract/discussions/156)

**RTK Query / TanStack Query**
- connect-query-es streaming issue #524 — [github.com/connectrpc/connect-query-es/issues/524](https://github.com/connectrpc/connect-query-es/issues/524)
- TanStack Query comparison — [tanstack.dev/query/v5/docs/framework/react/comparison](https://tanstack.dev/query/v5/docs/framework/react/comparison)

**React 18/19**
- React 19 Suspense deep dive — [dev.to/a1guy/react-19-suspense-deep-dive](https://dev.to/a1guy/react-19-suspense-deep-dive-data-fetching-streaming-and-error-handling-like-a-pro-3k74)
- freecodecamp — modern React data fetching handbook — [freecodecamp.org/news/the-modern-react-data-fetching-handbook](https://www.freecodecamp.org/news/the-modern-react-data-fetching-handbook-suspense-use-and-errorboundary-explained/)
