# ADR-013: Turborepo packages/core for Web + React Native Logic Sharing

## Status
Accepted — deferred to Phase 5 (post-6-months)

## Context

The longer-term goal stated in the requirements is to ship a React Native mobile app that shares business logic with the web app. The refactor should lay the foundation for this without delaying Phase 1–4 delivery.

The key question is: which parts of the current codebase can be shared with React Native, and which must remain web-only?

### Platform constraints

**What can be shared (platform-agnostic)**:
- Redux Toolkit slices (no DOM APIs)
- ConnectRPC unary transport calls (with transport injection — see below)
- TypeScript types generated from protobuf (`@bufbuild/protobuf`)
- Business logic hooks that do not reference `window`, `document`, or React DOM APIs
- Validation logic (zod schemas)

**What cannot be shared (web-only)**:
- ConnectRPC server-streaming calls: React Native's Fetch polyfill is XHR-based and cannot stream. This is a hard architectural boundary confirmed by the ConnectRPC team (Issue #199).
- xterm.js terminal rendering
- Next.js App Router components (`app/` directory)
- CSS / vanilla-extract (RN uses StyleSheet.create)
- Browser APIs (`localStorage`, `window`, `navigator`)

### Transport injection requirement

The current service hooks import `createConnectTransport` from `@connectrpc/connect-web` directly. This import fails in React Native (no browser Fetch API with streaming). For code to be shared, the transport must be provided by the platform consumer rather than imported by the hook:

```ts
// Current (web-only):
import { createConnectTransport } from '@connectrpc/connect-web';
const transport = createConnectTransport({ baseUrl });

// Required for sharing:
// packages/core/src/api/sessionService.ts
export function createSessionServiceHooks(transport: Transport) {
  // ... hooks that use the injected transport
}

// apps/web:
const transport = createConnectTransport({ baseUrl });
const { useGetApprovals } = createSessionServiceHooks(transport);

// future apps/mobile:
const transport = createGrpcWebTransport({ baseUrl }); // unary only
const { useGetApprovals } = createSessionServiceHooks(transport);
```

This refactor is established in Phase 3 (Task 3.1.1) as a precondition for Phase 5.

### Monorepo tooling evaluation

| Tool | Fit | Verdict |
|------|-----|---------|
| **Turborepo** | First-party Vercel support; works with npm/pnpm/yarn; Expo SDK 52 auto-detects monorepo structure; Vercel provides a Next.js + React Native starter template | Selected |
| Nx | Enterprise-grade; plugin ecosystem; code generators | Rejected — excessive complexity for a 1-developer project |
| Plain npm workspaces | Simple | Lacks build caching and pipeline dependency management that Turborepo provides |

### Expo SDK 52 compatibility

Expo SDK 52 auto-detects Turborepo workspace structure. A future `apps/mobile` would be an Expo app at `apps/mobile/` with `@stapler-squad/core` as a workspace dependency. This requires no special configuration beyond standard Turborepo setup.

## Decision

Restructure the repo as a **Turborepo monorepo** in Phase 5 (post-6-months), after Phase 3 establishes transport injection boundaries.

### Directory structure (Phase 5 target)

```
/ (repo root)
├── turbo.json
├── package.json          ← workspace root
├── apps/
│   └── web/              ← moved from web-app/
│       ├── package.json  ← name: "@stapler-squad/web"
│       └── src/
└── packages/
    └── core/
        ├── package.json  ← name: "@stapler-squad/core"
        └── src/
            ├── store/    ← Redux slices (sessions, approvals, filters)
            ├── api/      ← transport factory types + RTK Query endpoints
            └── types/    ← protobuf-generated types re-exported as plain TS
```

### turbo.json pipeline

```json
{
  "pipeline": {
    "build": {
      "dependsOn": ["^build"],
      "outputs": [".next/**", "dist/**"]
    },
    "dev": {
      "cache": false,
      "persistent": true
    },
    "lint": {},
    "test": {
      "outputs": ["coverage/**"]
    }
  }
}
```

The `^build` dependency ensures `packages/core` is built before `apps/web`.

### What lives in packages/core

- `src/store/sessionsSlice.ts` — session list state
- `src/store/approvalsSlice.ts` — approval state
- `src/store/filtersSlice.ts` — filter/search state
- `src/api/connectApi.ts` — RTK Query base (transport-injected)
- `src/api/approvalsApi.ts` — approval RTK Query endpoints
- `src/api/serialization.ts` — protobuf → plain object boundary
- `src/types/` — re-exports of protobuf-generated types

### What stays in apps/web (permanently web-only)

- `src/components/` — all React components (Next.js / DOM)
- `src/lib/hooks/useTerminalStream.ts` — server-streaming
- `src/lib/hooks/useWatchSessions.ts` — server-streaming
- `src/app/` — Next.js App Router pages
- `src/styles/` — vanilla-extract theme

### Makefile and CI implications

The Go backend build (`make build`, `make restart-web`) references `web-app/`. After restructure, these references change to `apps/web/`. The `make restart-web` target in the root Makefile must be updated.

CI (GitHub Actions) currently runs `cd web-app && npm run build`. After restructure: `turbo run build --filter=@stapler-squad/web`.

## Consequences

### Positive
- Future React Native app (`apps/mobile`) can import from `@stapler-squad/core` and get RTK slices, approval hooks, and type definitions without duplicating code
- Turborepo build cache means unchanged packages are not rebuilt — faster CI
- Expo SDK 52 auto-detection reduces RN monorepo setup friction

### Negative / Constraints
- One-time file-system restructure: all import paths in `apps/web` that previously pointed to `web-app/src/lib/` need updating (most use `@/lib/` alias — update alias root in `tsconfig.json` from `web-app/src` to `apps/web/src`)
- Go backend's `Makefile` must be updated to reference `apps/web/` instead of `web-app/`
- Until Phase 5, `web-app/` remains as-is — no premature restructuring
- Streaming hooks (`useTerminalStream`, `useWatchSessions`) are permanently web-only and must never be moved to `packages/core`. This is an architectural boundary enforced by code review.

## Deferred Decisions

The following are deferred until Phase 5 actually begins:

- Whether to use npm workspaces, pnpm workspaces, or Yarn workspaces as the package manager
- Whether `apps/mobile` is an Expo Router or bare Expo app
- Whether `@connectrpc/connect-react-native` (if it exists by then) supersedes the manual transport injection pattern

## References
- Turborepo Next.js + React Native starter: https://vercel.com/templates/next.js/turborepo-react-native
- Expo SDK 52 monorepo support: https://docs.expo.dev/guides/monorepos/
- ConnectRPC streaming RN limitation: https://github.com/connectrpc/connect-es/issues/199
- Research synthesis: `project_plans/front-end-refactor/research/synthesis.md`
- Implementation: Phase 5, Story 5.1 in `docs/tasks/front-end-refactor.md`
