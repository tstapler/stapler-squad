# ADR-003: Frontend Scanner Approach

Status: Accepted
Date: 2026-04-17
Deciders: Solo developer (owner)

---

## Context

The frontend feature scanner must discover React components that represent user-facing features in the Stapler Squad web application and produce entries in `docs/registry/frontend-features.json`.

The frontend stack:
- React 19 with TypeScript
- Next.js-style routing under `web-app/src/app/`
- Component files colocated with their route directories
- Generated TypeScript from protobuf at `gen/session/v1/*_pb.ts` and `web-app/src/gen/`
- Existing CSS uses vanilla-extract (`.css.ts` files) — not relevant to scanner but context for exclusion rules

The scanner must distinguish "features" (page-level components and named user flows directly invocable by users) from internal helpers, utility functions, and generated code. The key challenge is that TypeScript/React codebases have high rates of false positives from static AST analysis alone — HOCs, re-exports, barrel files, and dynamically imported components all confuse naive AST walkers.

---

## Options Considered

### Option A: TypeScript Compiler API (full AST walk, no markers)

Use `ts.createProgram` to walk the entire `web-app/src/` directory, identify all React component exports (functions returning JSX), and catalog them.

Pros:
- No developer action required; all components discovered automatically
- Full TypeScript type information available (prop types, return types)

Cons:
- High false positive rate: utility components, primitive wrappers, icon components, generated types that look like components
- HOCs (`export default withRouter(MyComponent)`) ambiguous — which is the "feature"?
- Barrel exports (`export * from './components'`) cause double-counting
- Generated `*_pb.ts` files contain type definitions resembling components
- Research estimates 20-30% false positive rate without markers
- Maintenance burden: any new React pattern (Server Components, `use client` directive, Suspense boundaries) requires scanner rule updates
- Rejected as primary approach for solo practitioner due to alarm fatigue risk

### Option B: Runtime Component Registry via Playwright

Instrument React DevTools protocol during E2E test runs to capture the component tree for each route.

Pros:
- Only discovers components actually rendered in tests (no theoretical components)
- Captures dynamic imports and lazy-loaded components

Cons:
- Circular dependency: only discovers what tests exercise; tests need the registry to be written first
- Slow: full app must run for every scan
- Coverage bounded by test coverage (sparse at project start)
- Rejected unanimously in architecture research

### Option C: Storybook-based Registry

Extract component metadata from Storybook story files.

Pros:
- High-quality metadata (stories are written with intent)
- Visual preview available

Cons:
- Requires writing and maintaining story files for each component (significant upfront work)
- Adds 50-100MB build artifact
- Storybook server is a separate process
- No stories exist today; would require writing stories before the scanner has value
- Deferred: revisit after scanner proves value and marker adoption is established

### Option D: TypeScript Compiler API + `// +feature:` Marker Filtering — Chosen

Use TypeScript Compiler API to walk source files, but only include files that contain a `// +feature: {feature-id}` comment in the first 10 lines. The marker is the explicit signal from the developer that "this file defines a feature." All other files are ignored for feature discovery purposes.

This is a hybrid of full AST capability and marker-based precision.

Pros:
- Zero false positives from unmarked files (HOCs, utilities, generated code all excluded)
- TypeScript type information still available for marked files (component name, prop types)
- Marker adoption is incremental; scanner produces correct output at any adoption level
- Explicit developer intent encoded in marker; no scanner heuristic required
- Consistent with Go scanner philosophy (dual-scan with explicit markers)
- Research precedent: annotation-based discovery (Spring `@RestController`, Swagger `// @Router`) has lowest FP rate in comparable ecosystems

Cons:
- False negatives on unmarked features (by design; tracked as `markerFound: false` equivalent via coverage gap report)
- Requires developer to add markers when creating new feature components
- Marker discipline must be enforced via convention (CLAUDE.md / CONTRIBUTING.md)

Accuracy: ~100% precision; coverage proportional to marker adoption

---

## Decision

**TypeScript Compiler API + `// +feature:` marker filtering.**

The scanner walks all files in `web-app/src/`. For each file, it checks the first 10 lines for a `// +feature: {feature-id}` comment. If found, the file is scanned for its primary exported component name using the TypeScript Compiler API. All other files are skipped.

This approach trades recall for precision. For a solo practitioner where false positive alarm fatigue is the primary risk, precision is the correct optimization target.

---

## Implementation Details

**Marker format**:
```tsx
// +feature: ui:new-session-modal
import React from 'react';
// ...

export function NewSessionModal(props: NewSessionModalProps) {
  // ...
}
```

Marker must appear in the first 10 lines of the file. Feature ID format: `ui:{kebab-case-name}`.

**File exclusion rules** (applied before marker check):
- `*_pb.ts` — generated protobuf TypeScript
- `*.pb.ts` — alternative generated protobuf naming
- `*.test.tsx`, `*.test.ts` — test files
- `*.spec.tsx`, `*.spec.ts` — spec files
- `*.stories.tsx`, `*.stories.ts` — Storybook files
- `*.css.ts` — vanilla-extract style files
- Files under `__tests__/`, `__mocks__/` directories
- Files under `gen/` directory (generated TypeScript)

**Component name extraction**: For marked files, use `ts.createProgram` to find the primary exported declaration. Prefer `export default` if present; otherwise use the first exported function that starts with an uppercase letter (React component convention).

**Feature ID conventions**:
- Page-level routes: `ui:{route-name}` (e.g., `ui:sessions-page`, `ui:review-queue`)
- Modal/dialog features: `ui:{name}-modal` (e.g., `ui:new-session-modal`)
- Panel-level features with distinct user flows: `ui:{name}-panel`
- Utility components (tag editors, etc.): NOT features; do not mark

**Coverage gap report**: After both scans run, `tools/scanner/frontend/src/gap-reporter.ts` cross-references backend and frontend registries. The gap report is advisory; unmapped features are listed but not errors.

---

## Consequences

- Feature discovery starts at zero on day one (no markers exist yet)
- First implementation task after the scanner is built: add `// +feature:` markers to the top 10 most important components in `web-app/src/`
- Scanner produces correct output immediately after markers are added
- Adding a new feature component without a marker = feature is invisible to tooling (acceptable; coverage gap report will show frontend feature count is lower than expected)
- The scanner is stable against React pattern evolution (HOCs, Server Components, lazy imports) because it does not attempt to resolve imports — it only reads the file where the marker lives

---

## References

- `project_plans/qa-engineering-tooling/research/findings-architecture.md` — Options 3A through 3D
- `project_plans/qa-engineering-tooling/research/findings-stack.md` — TypeScript Compiler API analysis
- `project_plans/qa-engineering-tooling/research/findings-pitfalls.md` — False positive analysis for TypeScript scanners
- `web-app/src/` — Current React component structure
