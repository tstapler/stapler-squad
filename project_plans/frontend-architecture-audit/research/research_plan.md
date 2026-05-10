# Research Plan — React Best Practices for ConnectRPC + vanilla-extract Stack

## Scope

7 subtopics, each targeting a specific pain point from the architecture audit.

## Subtopics and Search Strategies

### 1. ConnectRPC + React Patterns
- Search: "connectrpc react hooks singleton transport typescript"
- Search: "connectrpc interceptor auth react typescript"
- Search: "@connectrpc/connect-web transport factory react"
- Key axes: singleton transport, streaming hooks, auth interceptors, error normalization
- Cap: 5 searches

### 2. Context Performance at Scale
- Search: "react context performance optimization 9 providers re-render"
- Search: "useSyncExternalStore context selector pattern react 18"
- Search: "context splitting stable volatile react performance"
- Key axes: re-render prevention, external store migration, selector patterns
- Cap: 4 searches

### 3. Large Component Decomposition
- Search: "react 1000 line component decomposition patterns container presenter"
- Search: "compound components render props vs hooks react"
- Search: "react terminal component architecture split hooks"
- Key axes: data/logic/render separation, composability, testability
- Cap: 4 searches

### 4. Vanilla-extract at Scale
- Search: "vanilla-extract theme contract tokens best practices"
- Search: "vanilla-extract enforce token usage eslint"
- Search: "vanilla-extract data visualization color tokens"
- Key axes: token contract enforcement, tooling, scale patterns
- Cap: 4 searches

### 5. RTK Query vs TanStack Query Migration
- Search: "rtk query vs tanstack query migration 2024 2025"
- Search: "tanstack query v5 connectrpc streaming react"
- Search: "redux toolkit query replace custom hooks migration"
- Key axes: streaming support, migration cost, dual-pattern elimination
- Cap: 5 searches

### 6. React 18/19 Features for Complex Apps
- Search: "react 18 use hook suspense connectrpc streaming"
- Search: "react 19 use hook data fetching patterns"
- Search: "react transitions concurrent features real-time data"
- Key axes: streaming compatibility, progressive adoption, complexity reduction
- Cap: 4 searches

### 7. Testing Patterns for RPC Hooks
- Search: "testing connectrpc react hooks mock transport"
- Search: "msw connectrpc protobuf testing react"
- Search: "react testing library hooks rpc mock"
- Key axes: mock fidelity, maintenance overhead, CI speed
- Cap: 4 searches

## Output Files
- `findings-connectrpc-patterns.md`
- `findings-context-performance.md`
- `findings-component-decomposition.md`
- `findings-vanilla-extract.md`
- `findings-rtk-tanstack.md`
- `findings-react18-19.md`
- `findings-rpc-testing.md`
- `react-best-practices.md` (final synthesis)
