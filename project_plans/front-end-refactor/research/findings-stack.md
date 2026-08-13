# Findings: Stack

**Date**: 2026-04-16
**Subtopic**: Component library selection, vanilla-extract createTheme migration, web + React Native code-sharing strategy
**Status**: Draft — pending web search verification (see Pending Web Searches)

---

## Summary

Three distinct decisions feed this subtopic. Each has a clear winner given the constraints.

1. **Component library**: **Radix UI primitives** (not shadcn/ui, not Headless UI, not Ark UI). Radix is headless, has no Tailwind dependency, is SSR-compatible with Next.js 15, and has zero runtime CSS-in-JS. shadcn/ui wraps Radix but bundles Tailwind — incompatible with the ADR-009 vanilla-extract mandate. Headless UI is Tailwind-first. Ark UI is younger and less battle-tested.

2. **vanilla-extract createTheme**: The current `theme.css.ts` is a hand-rolled wrapper of CSS custom property strings — it provides TypeScript autocompletion but does **not** use `createThemeContract` / `createTheme`. The migration is a well-defined two-step: define a contract with `createThemeContract`, implement two themes (light, dark) with `createTheme`, and replace the hand-rolled `vars` object. Component `.css.ts` files already reference `vars.color.xxx` so the surface syntax stays identical — only the import source changes.

3. **Web + RN code sharing**: **Turborepo monorepo with a `packages/core` package** is the 2025 consensus approach. The core package holds RTK slices, ConnectRPC client factory, and domain hooks. The web app (`apps/web`) and the future RN app (`apps/mobile`) both consume `packages/core`. Solito is an optional routing-abstraction layer on top; it is not required for business-logic sharing alone.

---

## Options Surveyed

### 1. Component Libraries

#### Radix UI Primitives (`@radix-ui/react-*`)
- Headless, unstyled primitive components: Dialog, Dropdown, Select, Tooltip, Tabs, Checkbox, Switch, etc.
- Style layer is 100% up to the consumer — vanilla-extract `.css.ts` files compose directly onto Radix's `asChild` prop pattern and `data-state` attributes.
- Accessibility: ARIA patterns built in, keyboard navigation, focus management. WCAG AA out of the box for supported primitives.
- SSR: fully server-render-safe; no browser API required at import time. [TRAINING_ONLY — verify with Next.js 15 App Router Server Components]
- React 19 compatible. [TRAINING_ONLY — verify]
- Bundle: tree-shakeable; each primitive is a separate package. Typical per-primitive gzip cost: 3–8 KB.
- RN compatibility: Web-only. Not shared with React Native (correct — primitives are for the web rendering layer only).
- Maturity: ~7 years old, widely adopted (Vercel, Linear, Loom use it in production). [TRAINING_ONLY — verify adoption claims]

#### shadcn/ui
- Not a library — it is a code generator that copies Radix UI components pre-styled with Tailwind CSS utility classes into your project.
- **Incompatible with this codebase**: ADR-009 mandates vanilla-extract. Tailwind's JIT runtime and vanilla-extract's build-time extraction have no conflict at a technical level, but ADR-009 explicitly rejects new CSS authoring outside vanilla-extract. Adding Tailwind would re-introduce the two-CSS-system problem the ADR was written to solve.
- If the team were to adopt Tailwind globally, shadcn/ui would be the strongest option. Under the current constraints it is a non-starter.

#### Headless UI (Tailwind Labs)
- Headless UI is architecturally similar to Radix but maintained by the Tailwind Labs team. Its documentation and examples are Tailwind-first.
- Smaller component surface than Radix (fewer primitives available).
- No Tailwind runtime dependency, but all documentation, community examples, and the design aesthetic assume Tailwind. Using it with vanilla-extract is possible but uncommon and poorly documented.
- Verdict: Inferior to Radix for this stack. Radix has broader coverage and a larger community writing vanilla-extract integrations.

#### Ark UI (Chakra-based headless)
- Ark UI is the headless primitive layer extracted from Chakra UI. Style-agnostic, accessibility-first.
- **React framework**: Uses Zag.js state machines under the hood (a separate dependency).
- Feature-complete: dialog, combobox, date picker, carousel, etc. — broader than Radix for complex widgets.
- SSR: compatible with Next.js. [TRAINING_ONLY — verify]
- Maturity: Released ~2023, younger than Radix. Smaller community, fewer real-world examples.
- Bundle: Zag.js adds ~15–20 KB gzip overhead across all primitives. [TRAINING_ONLY — verify]
- Verdict: Viable but riskier than Radix. The Zag.js dependency adds complexity; community resources for vanilla-extract + Ark are thin.

---

### 2. vanilla-extract createTheme Approaches

#### Current state: hand-rolled `vars` object

The current `web-app/src/styles/theme.css.ts` exports a plain TypeScript object with string values:

```ts
export const vars = {
  color: { primary: "var(--primary)", textPrimary: "var(--text-primary)", ... },
  font: { mono: "var(--font-mono)" },
} as const;
```

- Provides TypeScript autocompletion for token paths (`vars.color.primary` is typed as the literal string `"var(--primary)"`).
- Does **not** provide compile-time validation that the referenced CSS custom property exists in `globals.css`.
- Does **not** support theme switching at the vanilla-extract level — dark mode relies on `@media (prefers-color-scheme: dark)` blocks and CSS custom property overrides inside `globals.css`.
- Token additions require manual editing of both `globals.css` and `theme.css.ts` — two files to keep in sync with no tooling to enforce consistency.

#### Option A: `createThemeContract` + dual `createTheme` (recommended)

```ts
// theme.css.ts — with createThemeContract
import { createThemeContract, createTheme } from '@vanilla-extract/css';

export const vars = createThemeContract({
  color: {
    textPrimary: null,
    actionPrimary: null,
    background: null,
    cardBackground: null,
    // ... all tokens from globals.css
  },
  space: { 1: null, 2: null, 4: null, 8: null },
  radii: { sm: null, md: null, lg: null },
  fontSize: { sm: null, base: null, lg: null },
  font: { mono: null },
});

export const lightTheme = createTheme(vars, {
  color: {
    textPrimary: '#0a0a0a',
    actionPrimary: '#0070f3',
    background: '#ffffff',
    cardBackground: '#f9f9f9',
  },
  // ...
});

export const darkTheme = createTheme(vars, {
  color: {
    textPrimary: '#f9fafb',
    actionPrimary: '#3b82f6',
    background: '#0a0a0a',
    cardBackground: '#1a1a1a',
  },
  // ...
});
```

- `createThemeContract` generates a CSS custom property for every leaf node. `vars.color.textPrimary` resolves to the generated `var(--color-textPrimary-xxx)` string at build time — not the hand-rolled `var(--text-primary)`.
- **Compile-time safety**: a typo in a `.css.ts` consumer (`vars.color.textPrimery`) is a TypeScript error.
- **Theme switching**: apply `lightTheme` or `darkTheme` class name to `<html>`. No separate `@media (prefers-color-scheme)` blocks needed in `globals.css` for themed tokens.
- **Migration compatibility note**: The generated custom property names differ from `globals.css` names. During migration, `.module.css` files use `var(--text-primary)` while `.css.ts` files use the generated name. The two systems coexist but a component mixing both may show inconsistent theming. Migrate components atomically.

#### Option B: Keep hand-rolled wrapper, add sprinkles

- Continue with the current `vars` object but add `@vanilla-extract/sprinkles` for atomic utility generation.
- Does not fix the core contract problem (no compile-time enforcement that `var(--text-primary)` exists).
- `check-css-vars.mjs` CI script partially fills this gap but is not as tight as TypeScript.
- Verdict: A halfway measure. Appropriate only if the team decides not to complete the vanilla-extract migration. Not recommended.

#### Option C: Panda CSS

Already evaluated and rejected in ADR-009. Not re-evaluated here.

---

### 3. Web + React Native Code-Sharing Strategies

#### Turborepo monorepo with `packages/core`

```
stapler-squad/               <- git root (monorepo)
├── apps/
│   ├── web/                 <- current web-app/ (Next.js 15)
│   └── mobile/              <- future Expo app
├── packages/
│   ├── core/                <- RTK store, slices, ConnectRPC factory, domain hooks
│   ├── ui-web/              <- Radix + vanilla-extract component library
│   └── ui-native/           <- React Native component library (future)
└── turbo.json
```

- `packages/core` contains only framework-agnostic TypeScript: Redux slices, `createEntityAdapter` normalization, ConnectRPC client factory, and pure hooks. No DOM imports, no React Native APIs.
- `apps/web` imports from `packages/core` and `packages/ui-web`.
- `apps/mobile` (future) imports from `packages/core` and `packages/ui-native`.
- Turborepo handles incremental builds, caching, and task orchestration via `turbo.json`.
- **Migration path from current state**: `web-app/` becomes `apps/web/`. Shared logic extracted to `packages/core` over time. Not a big-bang migration — can be done incrementally.
- **Protobuf / ConnectRPC in core**: `@connectrpc/connect` is platform-agnostic (runs in browser, Node.js, React Native). `@bufbuild/protobuf` similarly. The transport adapter (`@connectrpc/connect-web` uses `fetch`) needs a platform-specific implementation for React Native. [TRAINING_ONLY — verify connect-react-native package status]

#### Nx

- Enterprise-grade monorepo tool with more opinionated structure and code generation.
- More powerful caching and affected-analysis than Turborepo.
- Higher learning curve, more configuration, larger tooling surface.
- Verdict: Overkill for a single-developer project. Turborepo is lighter and achieves the same output caching.

#### Bare Expo + Next.js side-by-side (no monorepo tooling)

- Two separate repos or a flat repo with manual workspace symlinks.
- No automated dependency graph; no incremental builds; no shared lint/test config.
- Verdict: Does not meet the code-sharing requirement at scale.

#### Solito (Next.js + Expo Navigation abstraction)

- Solito is a routing-abstraction layer: `useRouter`, `useSearchParams`, `Link` components that work in both Next.js and Expo Router.
- Does **not** help with state sharing, API client sharing, or non-navigation logic.
- Verdict: Complementary, not an alternative. Use if/when the RN app's screen routing needs to match web routes. Not required for the first phase (foundation only).

---

## Trade-off Matrix

### Component Library

| Axis | Radix UI | shadcn/ui | Headless UI | Ark UI |
|---|---|---|---|---|
| Bundle size impact | Low (3–8 KB/primitive) | Medium (Tailwind adds ~15 KB gzip) [TRAINING_ONLY] | Low | Medium (Zag.js ~15–20 KB) [TRAINING_ONLY] |
| Accessibility coverage | High (ARIA built-in) | High (inherits Radix) | High | High (state machine driven) |
| TypeScript ergonomics | Excellent | Excellent (generated) | Good | Good |
| React Native compatibility | None (web-only, correct) | None | None | None |
| Migration effort from current state | Low (drop-in for bespoke components) | Blocked by Tailwind/ADR-009 conflict | Medium | Medium |
| SSR compatibility (Next.js 15) | Full [TRAINING_ONLY — verify] | Full | Full | Full [TRAINING_ONLY — verify] |
| vanilla-extract compatibility | Excellent (data-state attrs + asChild) | Blocked | Partial | Good |
| Community / ecosystem | Large | Very large | Medium | Small |
| **Verdict** | **Recommended** | **Rejected** | Not recommended | Viable backup |

### vanilla-extract Token Approach

| Axis | createThemeContract | Hand-rolled wrapper |
|---|---|---|
| Compile-time token safety | Full (TypeScript) | Partial (path completion only) |
| Dark mode support | Theme switching via class name | CSS media query in globals.css |
| Migration effort | Medium (rewrite theme.css.ts; update .css.ts imports) | None (current state) |
| Coexistence with .module.css | Full (different namespaces) | Full |
| Token source of truth | Single (theme.css.ts) | Dual (globals.css + theme.css.ts) |
| **Recommended** | **Yes** | No |

### Code-Sharing Strategy

| Axis | Turborepo + packages/core | Nx | Side-by-side repos | Solito |
|---|---|---|---|---|
| Code sharing capability | High (explicit package boundaries) | High | Low | Routing only |
| Build tooling complexity | Medium | High | Low | Low (add-on) |
| Incremental migration | Yes (extract packages over time) | Yes | No | Yes |
| Single-developer fit | Good | Poor (too much config) | Poor (manual) | N/A |
| ConnectRPC in RN | Supported (platform transport swap) | Same | Same | N/A |
| **Recommended** | **Yes** | No | No | Optional later |

---

## Risk and Failure Modes

### Radix UI + vanilla-extract

**Risk 1 — Portal theming**: Some Radix primitives use portals (`Dialog`, `Tooltip`, `DropdownMenu`) which render into `document.body`. vanilla-extract class names still apply correctly — but CSS custom property inheritance may break if the theme class is applied only to `#__next` and not `body`.
- Mitigation: Apply theme class to `document.documentElement` (or `document.body`). Document in CSS architecture rules.
- Severity: Low; well-understood pattern.

**Risk 2 — Specificity with asChild**: Radix's `asChild` prop merges class names. If a Radix component internally adds utility classes, vanilla-extract class specificity may produce unexpected overrides.
- Mitigation: Use `recipe()` for all variant logic rather than relying on specificity ordering. Avoid global utility classes.
- Severity: Low.

**Risk 3 — Server Component boundaries**: [TRAINING_ONLY — verify] Some Radix primitives may require `"use client"` even for presentational use (they use `useId`, `useContext`, etc.).
- Mitigation: Wrap Radix primitives in thin `"use client"` wrappers in `packages/ui-web`. Keep the server-component boundary above the primitive layer.
- Severity: Medium; addressable with known pattern.

### vanilla-extract `createThemeContract` migration

**Risk 4 — Custom property name mismatch**: Generated custom property names from `createThemeContract` differ from `globals.css` names. During migration, `.module.css` files use `var(--text-primary)` while `.css.ts` files use the generated name. Inconsistent theming during the migration window.
- Mitigation: Migrate components atomically — once a component's CSS is in `.css.ts`, remove its `.module.css` file immediately. Track migration status in a checklist.
- Severity: Medium; manageable with discipline.

**Risk 5 — CI script lifetime**: `check-css-vars.mjs` must remain active until the last `.module.css` file is deleted. If removed prematurely, undefined variable references can re-enter the codebase silently.
- Mitigation: Do not remove `check-css-vars.mjs` until the last `.module.css` file is gone. Add a CI check that fails if both `check-css-vars.mjs` is absent and any `.module.css` files remain.
- Severity: Low.

**Risk 6 — Turbopack dev-mode instability**: [TRAINING_ONLY — verify] `@vanilla-extract/next-plugin` Turbopack support is newer. Development DX may be degraded (slower HMR, occasional full rebuilds) with `next dev --turbopack`.
- Mitigation: Keep fallback `next dev` (without `--turbopack`) available. Test with each vanilla-extract plugin upgrade.
- Severity: Medium for DX; does not affect production builds.

### Turborepo monorepo migration

**Risk 7 — Directory restructure blast radius**: Moving `web-app/` to `apps/web/` breaks all existing import paths, CI scripts, Makefile targets, and the Go server's static asset build step.
- Mitigation: Do the restructure as a single atomic commit. Update `Makefile`, CI workflows, and `server/web/` embed paths in one pass. Do not interleave with feature work.
- Severity: High (one-time); risk window is short.

**Risk 8 — Protobuf serialization in shared store**: `@bufbuild/protobuf` Message instances are non-serializable class objects. The Redux store's `serializableCheck: false` bypass (documented in ADR-008) must be replicated in the mobile app's store config or the factory must configure it automatically.
- Mitigation: Export a `createAppStore(config)` factory from `packages/core` that includes the correct middleware configuration. Consumers do not configure middleware directly.
- Severity: Medium; addressable at store factory design time.

**Risk 9 — ConnectRPC transport for React Native**: `@connectrpc/connect-web` uses the Fetch API. React Native's Fetch polyfill may lack HTTP/2 or streaming support on some Android versions.
- Mitigation: Abstract the transport factory in `packages/core`. Web uses `createConnectTransport` from `connect-web`; native uses a native-capable transport. RTK slice and hook code does not import the transport directly — it receives it via dependency injection.
- Severity: Medium; design the abstraction boundary early before any mobile work begins.

---

## Migration and Adoption Cost

### Radix UI

**Estimated effort**: Medium-low (3–4 weeks for core primitives).
- Install individual `@radix-ui/react-*` packages as needed. No global setup required.
- Write each primitive (e.g., `Modal`, `Button`, `Dropdown`, `Input`) as a vanilla-extract styled wrapper over the Radix primitive. Each is independently shippable.
- Replacement of bespoke components (e.g., `ResumeSessionModal`, `WorkspaceSwitchModal`, `TagEditor`, `AutocompleteInput`) happens as part of normal feature work, not a separate migration sprint.
- Rough estimate: ~1–2 days per primitive including vanilla-extract styles and accessibility testing. ~15 key primitives = 3–4 weeks.

### vanilla-extract `createThemeContract`

**Estimated effort**: Medium (1 week for foundation; ongoing per-component migration).
- Phase 1 (1–2 days): Audit all `var()` usages across 70 `.module.css` files to produce the canonical token list.
- Phase 2 (1–2 days): Write new `theme.css.ts` with `createThemeContract` covering all tokens. Add `lightTheme` and `darkTheme` implementations.
- Phase 3 (1 day): Update the two existing `.css.ts` files to import from the new contract.
- Phase 4 (ongoing, per-component): As each `.module.css` file is converted, use the new `vars` object. No bulk migration needed.
- Phase 5 (end state): When the last `.module.css` file is removed, `globals.css` shrinks to structural rules only. `check-css-vars.mjs` retired.

### Turborepo monorepo

**Estimated effort**: Medium-high (2–3 days for restructure; ongoing for package extraction).
- Phase 1 (1–2 days): Add `turbo.json`, root `package.json` with workspaces, restructure `web-app/` to `apps/web/`. Update Makefile, CI, Go embed paths.
- Phase 2 (1 day): Create `packages/core/` with RTK store factory + `sessionsSlice` as first extraction. Validate tree-shaking in `apps/web`.
- Phase 3 (future): Create `apps/mobile` (Expo) and `packages/ui-native` when mobile work begins.
- **Recommended**: Do this before writing new components so all new code lands in the correct location.

---

## Operational Concerns

**CI / Build pipeline**: Turborepo's `--filter=apps/web` scopes builds. The existing `make restart-web` target must be updated to run `turbo build --filter=apps/web` or equivalent. The `lint:css-vars` script path must be updated when `web-app/` moves to `apps/web/`.

**Type safety across package boundaries**: `packages/core` exports TypeScript source. Each consuming app gets full type checking with no pre-compilation step during development (use `ts-jest` or `tsx` for tests in core). Avoid circular imports between packages — add `eslint-plugin-import` with `no-cycle` from the start.

**Developer experience**: `turbo dev --filter=apps/web` gives the same Next.js dev experience. The Go `make restart-web` workflow continues to work after updating the embedded asset path.

**Remote caching**: Turborepo local caching is sufficient for a single developer. Remote caching (Vercel Remote Cache or self-hosted) can be added later if CI times become significant.

---

## Prior Art and Lessons Learned

**Radix UI + vanilla-extract**: The Seek design system (Braid) uses vanilla-extract as its CSS layer over accessible headless primitives. The canonical pattern for Radix state-driven styles in vanilla-extract uses `data-[state]` attribute selectors:
```ts
export const overlay = style({
  selectors: {
    '&[data-state="open"]': { opacity: 1 },
    '&[data-state="closed"]': { opacity: 0 },
  },
});
```
[TRAINING_ONLY — verify Braid/Seek as a public reference for this exact combination]

**vanilla-extract `createTheme` migration**: The vanilla-extract documentation explicitly covers migration from hand-rolled CSS custom property wrappers to `createThemeContract`. Component call sites (`vars.color.xxx`) do not change — only `theme.css.ts` internals change. This makes the migration low-risk. [TRAINING_ONLY — verify migration documentation exists]

**Turborepo + Next.js + Expo**: The Turborepo "with-expo" example starter demonstrates the `apps/web + apps/mobile + packages/core` pattern. The key community lesson is that `packages/core` must never import anything from `react-dom` or web-specific modules — enforce with peer-dependency constraints and CI checks. [TRAINING_ONLY — verify starter example exists and is up to date]

**shadcn/ui and Tailwind rejection**: The shadcn/ui copy-paste model introduces a maintenance burden (copied components diverge from upstream). The incompatibility with ADR-009's vanilla-extract mandate is a hard blocker independent of the maintenance concern.

---

## Open Questions

1. **Radix UI and Next.js 15 Server Components**: Which Radix primitives can be used as server components vs. which require `"use client"`? Needed before designing the component package boundary in `packages/ui-web`. [Pending web search]

2. **`@connectrpc/connect-react-native`**: Does a first-party package exist, or is the pattern to use `connect-web` with React Native's Fetch polyfill? What is the status as of early 2026? [Pending web search]

3. **vanilla-extract + Turbopack stability**: Is `@vanilla-extract/next-plugin` fully stable with `next dev --turbopack` in Next.js 15.3? Known issues? [Pending web search]

4. **Dark mode switching approach**: Should dark mode be driven by a vanilla-extract theme class on `<html>` (replacing the current `@media (prefers-color-scheme: dark)` in `globals.css`) or should both coexist during migration? The class-based approach enables a user-controlled toggle in addition to system default. (Non-blocking — the contract migration does not require resolving this first.)

5. **Token count audit**: The full set of CSS custom properties currently in `globals.css` must be inventoried before writing the `createThemeContract`. Some tokens are structural (`--header-height`, `--viewport-height`) rather than themeable — should these live in the contract or remain as raw CSS custom properties in `globals.css`?

6. **Turborepo remote caching**: Does the project warrant Vercel Remote Cache setup, or is local Turborepo caching sufficient? (Non-blocking — can default to local caching.)

---

## Recommendation

### Immediate actions (unblock all subsequent work)

1. **Turborepo restructure** — Move `web-app/` to `apps/web/`, add root `turbo.json` and `package.json` workspaces, create `packages/core/` skeleton. Update `Makefile`, CI, and Go embed paths. Do this before writing new components.

2. **`createThemeContract` migration** — Audit token inventory from 70 `.module.css` files, write new `theme.css.ts` with `createThemeContract` + `lightTheme` + `darkTheme`. Update the two existing `.css.ts` files. This unblocks all subsequent vanilla-extract component work with full type safety.

### Component library: Radix UI primitives

Install `@radix-ui/react-*` packages individually as primitives are needed. Write vanilla-extract styled wrappers in `packages/ui-web` (or `apps/web/src/components/primitives/` until the package boundary is established). Do not adopt shadcn/ui (ADR-009 conflict). Do not adopt Headless UI (inferior Radix alternative for this stack). Consider Ark UI only if Radix lacks a needed complex primitive (date picker, combobox with virtualization, etc.).

### Code-sharing foundation: Turborepo with packages/core

Use Turborepo (not Nx — too heavy). Do not start the mobile app without first extracting `packages/core`. The ConnectRPC transport must be injected as a dependency — not imported directly in shared hooks — to support platform-specific transports.

---

## Pending Web Searches

Run these exact queries to verify or refute `[TRAINING_ONLY]` claims before finalizing decisions:

1. `radix-ui react server components next.js 15 "use client" boundary` — Determine which Radix primitives require client-side rendering and the recommended App Router pattern.

2. `connectrpc connect react native transport 2025` — Determine if `@connectrpc/connect-react-native` exists as a first-party package or if `connect-web` + Fetch polyfill is the standard.

3. `vanilla-extract next-plugin turbopack stable 2025 next.js 15` — Confirm `@vanilla-extract/next-plugin` Turbopack compatibility status with Next.js 15.3.

4. `turborepo next.js expo monorepo starter 2025` — Find updated example starters and any reported breakages in the `apps/web + apps/mobile + packages/core` pattern.
