# Findings: Pitfalls

> Research date: 2026-04-16
> Researcher: Claude Sonnet 4.6 (training knowledge + codebase analysis)
> Uncertain claims are marked `[TRAINING_ONLY — verify]`

---

## Summary

This document catalogs failure modes and migration pitfalls for the stapler-squad front-end refactor across four risk domains:

1. **xterm.js 6.0 on Android Chrome (Pixel 9 Pro Fold)** — WebGL context limits, touch input gaps, and canvas memory pressure on mobile Chrome.
2. **vanilla-extract migration at scale** — SSR/build-time execution model, Turbopack immaturity, `:global()` equivalent traps, dynamic-style limits.
3. **React Native / Next.js code-sharing limits** — Hard boundaries at DOM APIs, `window`, `fetch`-based transports, and anything that imports `@connectrpc/connect-web`.
4. **Pixel 9 Pro Fold CSS/viewport quirks** — Fold-state CSS environment variables, inner/outer display inconsistency, virtual keyboard interactions with `visualViewport`.

The codebase is already partially prepared: `viewport-fit=cover` is set in `layout.tsx`, `ViewportProvider` wires `--keyboard-height` and `--viewport-height` at runtime, a `VirtualKeyboard` component exists, and `@vanilla-extract/next-plugin` is installed with one `.css.ts` file in production use. However, `XtermTerminal.tsx` imports `WebglAddon` unconditionally at module-load time with no capability guard, and `theme.css.ts` uses string-based `var()` wrappers rather than `createTheme`/`createThemeContract`, meaning token typos remain runtime-silent.

**Blocking risks**: WebGL unconditional load on Android (Critical, High likelihood), and `theme.css.ts` not using `createThemeContract` (High, certain — confirmed in code).

---

## Options Surveyed

### xterm.js Rendering Paths

| Renderer | Android Chrome Support | Notes |
|---|---|---|
| WebGL (`@xterm/addon-webgl`) | Partial — WebGL2 required | Context loss under memory pressure is common on Android |
| Canvas (DOM 2D) | Good | Fallback path; slower but reliable |
| DOM renderer (legacy) | Deprecated in xterm 5+ | Not available in xterm 6 |

### vanilla-extract Integration Approaches

| Approach | Turbopack | Webpack | Risk |
|---|---|---|---|
| `@vanilla-extract/next-plugin` | Partial (v2.5.x) | Stable | Turbopack dev-mode glitches known |
| Manual Babel plugin | No Turbopack | Stable | More config, same output |
| Vite + `@vanilla-extract/vite-plugin` | N/A | N/A | Would require ejecting from Next.js |

### RN/Next.js Code-Sharing Strategies

| Strategy | Shareability | Complexity |
|---|---|---|
| Pure logic packages (no DOM) | High — Redux slices, zod schemas, pure utils | Low |
| Platform-abstracted hooks | Medium — requires adapter pattern | Medium |
| Shared UI components | Low — JSX is portable but styling is not | High |
| ConnectRPC transport layer | Not shareable — `connect-web` uses Fetch API | N/A: replace with platform-injected transport |

### Fold/Multi-Display CSS Strategies

| Strategy | Browser Support | Complexity |
|---|---|---|
| CSS `env(viewport-segment-*)` (Device Posture API Level 2) | Chrome 108+ [TRAINING_ONLY — verify exact version] | Low — pure CSS |
| `window.getWindowSegments()` (deprecated Level 1) | Chrome 84-109 (removed) | None — do not use |
| JavaScript + ResizeObserver + visualViewport | Universal | Medium |
| Media queries (`@media (spanning: single-fold-vertical)`) | Chrome 88-108 experimental flag [TRAINING_ONLY] | Low but API changed |

---

## Trade-off Matrix

| # | Pitfall | Severity | Likelihood | Mitigation Available | Blocks Refactor |
|---|---|---|---|---|---|
| P1 | WebGL unconditional import crashes on Android (no capability check) | Critical | High | Add `try/catch` + `WebGL2RenderingContext` guard before `new WebglAddon()` | Yes |
| P2 | `theme.css.ts` uses `var()` strings not `createTheme` — token typos remain silent | High | Certain (confirmed in code) | Migrate to `createTheme`/`createThemeContract` early | Yes (defeats ADR-009 goal) |
| P3 | Turbopack + `@vanilla-extract/next-plugin` dev-mode breakage | High | Medium | Fall back to `next dev` (without `--turbopack`) for CSS work | No (workaround exists) |
| P4 | `:global()` CSS Modules pattern has no direct vanilla-extract equivalent | High | High (XtermTerminal.module.css has 15+ `:global()` rules) | Use `globalStyle()` with correct scoped selectors | No |
| P5 | `connect-web` transport imports browser `fetch` — not shareable with RN | High | Certain | Never put `createConnectTransport` in shared packages | No (affects RN scope only) |
| P6 | Redux slices are shareable but `store.ts` may pull web-only deps into RN | Medium | High | Export only reducers/actions/selectors from shared packages; `configureStore` stays per-platform | No |
| P7 | Monaco Editor WebWorker path may break under `output: "export"` | Medium | Medium | Verify worker config; may need `publicPath` workaround [TRAINING_ONLY — verify] | No |
| P8 | FitAddon measures wrong container size during Pixel 9 Pro Fold state transition | High | Medium | Debounce ResizeObserver callback ~150ms before calling `fitAddon.fit()` | No |
| P9 | CSS `env(viewport-segment-*)` has no polyfill — fold layout only on Chrome 108+ | Medium | Low | Use `@supports` feature detection + JS fallback | No |
| P10 | vanilla-extract `.css.ts` files executed at Node build-time — browser globals throw | Medium | Medium | Never import anything referencing `window`/`document` from `.css.ts` files | No |
| P11 | WebGL context count limit in Chrome (~16 max per page; lower on Android RAM-constrained) | Medium | Low (single terminal per view) | Dispose `webglAddon` on terminal unmount | No |
| P12 | xterm 6 does not synthesize touch → keyboard input on mobile | Medium | High | VirtualKeyboard component exists; add auto-show on touch-device terminal focus | No |
| P13 | Radix UI SSR hydration mismatch with `useId()` under React 19 strict mode | Low | Low | Use `suppressHydrationWarning` per Radix docs | No |
| P14 | RTK Query must not be used for streaming endpoints (incompatible architecture) | Medium | Medium | Scope RTK Query to unary RPC only; keep streaming in custom hooks | No |
| P15 | Missing `next/dynamic` with `ssr: false` on xterm/Monaco/CodeMirror causes SSR crash | High | High | Audit all heavy editor components; wrap with `dynamic(..., { ssr: false })` | No |

---

## Risk and Failure Modes

### 1. xterm.js on Android Chrome / Pixel 9 Pro Fold

**P1 (Critical): WebGL unconditional load**

Current code in `XtermTerminal.tsx` lines 150-155:
```ts
const webglAddon = new WebglAddon();
webglAddon.onContextLoss(() => { webglAddon.dispose(); });
terminal.loadAddon(webglAddon);
```

There is no capability check before `new WebglAddon()`. On Android Chrome, WebGL2 may be unavailable when:
- The browser has too many active WebGL contexts (Chrome hard limit ~16 contexts, lower on Android RAM-constrained devices [TRAINING_ONLY — verify exact Android limit])
- The page is backgrounded and GPU memory is reclaimed
- The device is in low-power mode

On construction, `WebglAddon` calls `canvas.getContext('webgl2')` internally. If this returns `null`, xterm.js 6 throws an uncaught error and the terminal fails to render entirely [TRAINING_ONLY — verify exact error behavior in xterm 6.0].

The stress-test page (`/test/terminal-stress/page.tsx`) already implements the correct lazy-load + try/catch pattern. That pattern must be back-ported to production `XtermTerminal.tsx`.

**Mitigation**:
```ts
const supportsWebGL2 = typeof WebGL2RenderingContext !== 'undefined' &&
  (() => {
    try {
      const c = document.createElement('canvas');
      return !!c.getContext('webgl2');
    } catch { return false; }
  })();

if (supportsWebGL2) {
  try {
    const webglAddon = new WebglAddon();
    webglAddon.onContextLoss(() => { webglAddon.dispose(); });
    terminal.loadAddon(webglAddon);
  } catch (e) {
    console.warn('[XtermTerminal] WebGL unavailable, using canvas renderer:', e);
  }
}
```

**P8 (High): FitAddon during fold state transition**

When the Pixel 9 Pro Fold transitions between folded and unfolded states, the browser fires `resize` events. FitAddon measures `offsetWidth`/`offsetHeight`. If the container is mid-animation, FitAddon can compute incorrect col/row counts, causing the terminal to render misaligned or with incorrect line wrapping.

`XtermTerminal.module.css` already has `min-height: 0` and `box-sizing: content-box` comments noting FitAddon requirements, but there is no debounce on resize.

**Mitigation**: Debounce the ResizeObserver callback in XtermTerminal by 100-150ms. During the debounce window, defer `fitAddon.fit()` until dimensions stabilize.

**P12 (Medium): Touch input gap**

xterm.js does not map `touchstart`/`touchend` to terminal input. The `VirtualKeyboard` component exists but requires manual invocation. On touch devices, the app must detect "no hardware keyboard" (via `navigator.userAgentData` or `matchMedia('(pointer: coarse)')`) and auto-show the VirtualKeyboard when the terminal is focused.

### 2. vanilla-extract Migration Pitfalls

**P2 (High/Certain): `theme.css.ts` pattern defeats ADR-009 type-safety goal**

Current `web-app/src/styles/theme.css.ts` exports a `vars` object of raw `var()` strings:
```ts
export const vars = {
  color: {
    primary: "var(--primary)",
    textPrimary: "var(--text-primary)",
  },
} as const;
```

This is typed `as const` — TypeScript catches typos in `vars.color.textPrimary` — but the underlying `var(--primary)` string is not validated against actual CSS custom property definitions at build time. If `--text-primary` is removed from `globals.css`, the reference silently renders as nothing.

The ADR-009 recommendation is `createTheme()` or `createThemeContract()`. These generate hashed CSS variable names at build time and guarantee the CSS variable and TypeScript reference stay in sync. The current pattern is an improvement over raw strings but does not achieve the ADR's stated type-safety goal.

**P3 (High): Turbopack + vanilla-extract dev-mode**

`next.config.ts` uses `withVanillaExtract(nextConfig)` and `package.json` dev script is `next dev --turbopack`. The `@vanilla-extract/next-plugin` v2.5.x claims Turbopack support but the webpack integration is more mature. Known issues [TRAINING_ONLY — verify current status]:
- HMR for `.css.ts` files can fail to propagate changes in Turbopack mode
- The plugin registers a webpack custom loader that Turbopack may not respect, causing `.css.ts` to produce no CSS output in dev mode
- ADR-009 already notes: "If dev-mode issues arise, fall back to `next dev` (without `--turbopack`)"

**P4 (High): `:global()` CSS Modules → `globalStyle()` migration complexity**

`XtermTerminal.module.css` contains 15+ `:global()` selectors targeting xterm.js internal classes (`.xterm`, `.xterm-screen`, `.xterm-rows`, `.xterm-viewport`, `.xterm-selection`, `.xterm-viewport::-webkit-scrollbar`, etc.). CSS Modules exposes `:global()` for this purpose. vanilla-extract exposes `globalStyle(selector, styles)`.

The scoping difference is critical:
- CSS Modules: `.terminal :global(.xterm)` → selects `.xterm` descendants of `.terminal`
- vanilla-extract: `globalStyle(\`${terminal} .xterm\`, { ... })` → equivalent but requires the local class reference

Getting the selector composition wrong produces rules that apply globally (affecting all terminals on the page) rather than component-scoped. This file should be migrated last, in an isolated PR, with visual regression testing.

**P10 (Medium): Build-time `.css.ts` execution environment**

vanilla-extract runs `.css.ts` files in a Node.js child process at build time via esbuild. Any import that touches `window`, `document`, `localStorage`, or browser globals throws a ReferenceError and crashes the build. Common traps in this codebase:
- `getApiBaseUrl()` from `@/lib/config.ts` branches on `typeof window !== 'undefined'` — do not import this from a `.css.ts` file
- Any React context or hook import will likely transitively reference browser APIs

**Rule**: `.css.ts` files must only import from `@vanilla-extract/css` and pure constant modules.

### 3. React Native / Next.js Code-Sharing Limits

**P5 (High/Certain): ConnectRPC transport is web-only**

All ConnectRPC client hooks (`useApprovalAnalytics`, `useApprovals`, `useApprovalRules`, `useAuditLog`, etc.) call `createConnectTransport` from `@connectrpc/connect-web`. This package uses the browser Fetch API and browser-specific headers. React Native's `fetch` polyfill differs (no `ReadableStream` in older RN versions, different `Headers` behavior) [TRAINING_ONLY — verify current RN fetch support].

**Hard boundary**: Nothing in `web-app/src/lib/hooks/` that calls `createConnectTransport` can be shared with React Native. The isolation pattern: extract service call logic into a function that accepts a transport as an argument, then inject the appropriate transport per platform.

**P6 (Medium): Redux slices are shareable; store initialization is per-platform**

`sessionsSlice.ts`, `approvalsSlice.ts`, and `reviewQueueSlice.ts` import only from `@reduxjs/toolkit` — these are React Native compatible. `store.ts` calls `configureStore()` with `react-redux` middleware, which is also RN-compatible. However, if the store setup pulls in DevTools or web-only middleware, it becomes platform-specific.

**Recommendation**: In a shared package, export only slice reducers, actions, and selectors. `configureStore()` lives in each platform's app entrypoint.

**Hooks not shareable due to DOM/browser APIs**:
- `useFocusTrap.ts` — uses DOM `focus()` methods
- `useSwipe.ts` — uses `TouchEvent`
- `useKeyboard.ts` — uses `KeyboardEvent`
- `ViewportProvider.tsx` — manipulates `document.documentElement.style`
- `getApiBaseUrl()` — branches on `typeof window !== 'undefined'`

All require platform abstraction layers if shared packages are desired.

**P14 (Medium): RTK Query + ConnectRPC streaming conflict**

RTK Query does not support streaming endpoints natively. If added for unary RPC calls, developers may be tempted to misuse `useQuery` for streaming endpoints. The risk is stale state: RTK Query's cached result for a session list will not reflect real-time streaming updates arriving via ConnectRPC `streamSessions`.

**Rule**: RTK Query covers unary RPC only (create, update, delete, get). Streaming stays in custom hooks.

### 4. Pixel 9 Pro Fold Browser Quirks

**P9 (Medium): Fold-state CSS environment variables**

The CSS Device Posture API (Level 2) exposes `env(viewport-segment-width, 0 0)` and related variables for fold-aware layouts. Chrome 108+ supports this [TRAINING_ONLY — verify exact version]. The older Level 1 API (`env(fold-top)`, `env(fold-left)`, etc.) was removed in Chrome 109.

The codebase does not currently use fold-state CSS. If fold-aware layout is desired:
- `viewport-fit=cover` is already set in `layout.tsx` (correct prerequisite)
- Wrap fold-specific CSS in `@supports (env(viewport-segment-width, 0 0))` to ensure graceful degradation
- Do not use the Level 1 API

**P8 follow-up: visualViewport false keyboard detection during display transitions**

The `ViewportProvider` listens to `visualViewport.resize` and computes `--keyboard-height` as `window.innerHeight - vv.height - vv.offsetTop`. During a Pixel 9 Pro Fold display-mode transition, the browser fires multiple resize events as the inner display activates. This can briefly compute `--keyboard-height` as a large positive value (misidentifying the display expansion as a keyboard appearance), causing layout jumps.

**Mitigation**: Add a guard — if `--keyboard-height` changes by more than 30% of `window.innerHeight` in a single event, clamp it to 0 and treat as a display mode change rather than a keyboard event.

---

## Migration and Adoption Cost

### CSS Modules → vanilla-extract (70 files)

**Effort estimate**: 70 files, average 40 rules each = ~2800 rule migrations.

High-complexity files (require manual review, not mechanical migration):
- `XtermTerminal.module.css` — 15+ `:global()` selectors; requires `globalStyle()` with correct scoping; migrate last
- Any file using `composes:` (CSS Modules composition) — vanilla-extract has no equivalent; use `style()` object spread
- `SessionDetail.module.css`, `SessionList.module.css` — likely use complex responsive layouts

Low-complexity files (mechanical token replacement):
- Simple layout files with 5-10 rules using only defined tokens from `globals.css`
- Can be batch-migrated with an AST-based codemod targeting the `var(--token-name)` → `vars.color.tokenName` pattern

**Theme migration prerequisite**: Migrate `theme.css.ts` from `var()` string wrappers to `createTheme()` before migrating any component file. All component migrations depend on the correct `vars` object type.

**Build time impact**: vanilla-extract adds a build-time CSS extraction step. For 70 files, expect a 5-15% increase in webpack build time [TRAINING_ONLY — verify]. Turbopack may not have this overhead if it handles `.css.ts` natively.

### Radix UI Primitives Integration

Radix UI components render into a React portal by default. Styling portaled content from a vanilla-extract `.css.ts` file requires:
- `globalStyle('[data-radix-dialog-content]', { ... })` — works but fragile (relies on Radix's internal `data-` attribute names)
- Using Radix's `className` props with vanilla-extract-generated class names — the recommended approach

Radix ships no default styles; all styling is consumer-provided. This is compatible with vanilla-extract but requires discipline: do not use Radix's `asChild` prop with components that carry CSS Module class names during the migration period (class name conflicts).

### RTK Query Addition

Current state: ConnectRPC client hooks use `useRef`-held client instances with manual loading/error state. RTK Query's `createApi` would replace this boilerplate for unary endpoints. Estimated effort: 2-3 days for unary endpoints. Do not parallelize with vanilla-extract migration — both touch many files and will produce high merge conflict surface area.

---

## Operational Concerns

### CI Pipeline

The existing `lint:css-vars` script validates `var(--xxx)` references in `.module.css` files against `globals.css`. This does not cover `.css.ts` files. Once vanilla-extract migration is underway, add `tsc --noEmit` as a CI gate — it catches `vars.color.nonExistentToken` references in `.css.ts` files at PR time.

### Dev Experience During Migration

During the migration period, `next dev --turbopack` may produce inconsistent results for `.css.ts` files. A documentation note should be added to CLAUDE.md: if styles from a `.css.ts` file are not appearing in dev mode, run `make restart-web` (full webpack build) rather than debugging Turbopack HMR.

### Bundle Size

Monaco Editor is the largest dependency (~4MB minified [TRAINING_ONLY — verify]). It is used only in `web-app/src/app/config/page.tsx`. Verify it is code-split via `next/dynamic` — the current `size-limit` cap of 5MB total JS bundle leaves little headroom with Monaco. If the config page is not dynamically imported, Monaco will land in the initial bundle.

### Android Chrome Memory

xterm.js with WebGL on Android allocates significant GPU memory per context (~50MB [TRAINING_ONLY — verify]). If multiple sessions are open across tabs, the browser may reclaim GPU contexts. The current `onContextLoss` handler in `XtermTerminal.tsx` disposes the addon but does not re-initialize with a canvas fallback — the terminal goes blank.

**Mitigation**: In `onContextLoss`, after disposing `WebglAddon`, attempt to load `CanvasAddon` as a fallback, or re-render the terminal component with WebGL disabled.

---

## Prior Art and Lessons Learned

### vanilla-extract at Scale

**Atlaskit (Atlassian)** migrated their design system to vanilla-extract [TRAINING_ONLY — verify current status]. Key lessons:
- Define `createThemeContract` (token shape) before `createTheme` (token values) — this enables multiple themes without coupling shape to values
- Component-level `recipe()` significantly reduces `clsx()` call sites
- Migration can be done file-by-file with no coordination required between teams

**braid-design-system (Seek)** — Origin of vanilla-extract. Key lesson: `sprinkles` (atomic utilities) is powerful but adds learning curve; defer until after basic `style()` migration is complete.

### xterm.js on Mobile

The canonical solution adopted by web terminal apps (ttyd, wetty, GoTTY) is:
1. Detect touch device via `navigator.maxTouchPoints > 0` or `matchMedia('(pointer: coarse)')`
2. Disable WebGL on mobile — use canvas renderer instead
3. Add a custom soft keyboard trigger

This codebase's stress-test page already implements WebGL lazy-loading with fallback. That pattern needs to reach the production `XtermTerminal.tsx`.

### Foldable Devices and Web APIs

Chrome's Device Posture API on Pixel foldables has gone through multiple iterations:
- Chrome 84-88: `getWindowSegments()` origin trial (removed)
- Chrome 88-108: `@media (spanning: ...)` behind flag (removed)
- Chrome 108+: `env(viewport-segment-*)` standardized [TRAINING_ONLY — verify]
- Android 14 introduced OS-level fold state events

The safest approach: design for single-pane first, enhance with fold-aware CSS only behind `@supports`.

---

## Open Questions

1. **Does `@xterm/addon-webgl` v0.19 throw on `new WebglAddon()` when WebGL2 is unavailable, or only on `loadAddon()`?** Determines whether the capability check must precede construction or loading.

2. **What is the exact Chrome version on the Pixel 9 Pro Fold's stock browser?** Pixel 9 Pro Fold ships with Android 14/15 and Chrome 120+. Does Chrome 120+ on Android support `env(viewport-segment-*)` without flags?

3. **Does `@connectrpc/connect-react-native` exist?** If not, what is the ConnectRPC team's recommended transport adapter for React Native?

4. **Is `@vanilla-extract/next-plugin` v2.5.x fully compatible with Next.js 15 Turbopack as of April 2026?** The ADR noted this as an outstanding risk.

5. **Does Monaco Editor require special `next.config.ts` changes for `output: "export"` (static export)?** The `@monaco-editor/react` dynamic worker loading may conflict with static asset paths.

6. **What is the WebGL context count limit on Android Chrome?** Desktop Chrome limits to ~16 contexts; Android may be lower.

7. **Does the Pixel 9 Pro Fold fire `visualViewport.resize` when switching between inner and outer displays?** If yes, `ViewportProvider` needs a guard to avoid miscomputing `--keyboard-height` during display transitions.

---

## Recommendation

### Priority Order for Unblocking the Refactor

**Do first (blocks other work or causes production regressions on Android)**:

1. **Fix P1**: Add WebGL capability check to `XtermTerminal.tsx`. Copy the try/catch + capability guard pattern from `/test/terminal-stress/page.tsx`. Estimated effort: 15-30 minutes. This prevents blank terminals on Android.

2. **Fix P2**: Migrate `theme.css.ts` from `var()` string wrappers to `createTheme()`/`createThemeContract()`. This is the foundation that makes all subsequent `.css.ts` migrations actually type-safe. Estimated effort: 2 hours.

**Do second (high-leverage, establishes patterns for team)**:

3. Define the full `createThemeContract` shape covering all tokens in `globals.css`. Separate the contract (shape) from the `createTheme` implementation (light + dark values). This is the single highest-leverage step for the migration.

4. Document the `globalStyle()` migration pattern for `:global()` rules with a worked example using `XtermTerminal.module.css`. This pattern is needed for every editor/terminal component migration.

5. Add `tsc --noEmit` as a CI gate alongside `lint:css-vars`.

**Do before any React Native work starts**:

6. Audit all files in `web-app/src/lib/` for DOM/browser-API usage and tag as "platform-specific" vs "shareable". Budget 1-2 days. Only untagged files go into a shared package.

7. Decide on ConnectRPC transport strategy for RN before writing shared hook logic. This decision determines the entire shared package boundary.

**Defer (low severity or safely reversible)**:

8. Fold-state CSS layout (`env(viewport-segment-*)`): implement only after confirming target device Chrome version supports it. Default layout should be responsive single-pane; fold-aware is an enhancement.

9. RTK Query: add only after vanilla-extract migration is complete. Do not parallelize — both changes have high file-count overlap and will produce merge conflicts.

---

## Pending Web Searches

Execute these exact queries to fill training-knowledge gaps:

1. `xterm.js addon-webgl WebGL2 context null Android Chrome mobile failure` — Confirm whether `new WebglAddon()` throws on construction or on `loadAddon()` when WebGL2 is unavailable.

2. `"@vanilla-extract/next-plugin" Turbopack support 2025 2026 HMR Next.js 15` — Verify current Turbopack compatibility status.

3. `connectrpc connect-web React Native transport fetch streaming` — Find official or community ConnectRPC transport for React Native.

4. `CSS "Device Posture API" "viewport-segment" Chrome 108 Android Pixel fold support` — Confirm exact Chrome version and Android version for `env(viewport-segment-*)`.

5. `xterm.js 6.0 mobile Chrome touch keyboard onData Android 2024 2025` — Confirm known issues and recommended patterns for mobile keyboard input.

6. `"@monaco-editor/react" "next.js" "output: export" static export worker 2025` — Confirm whether Monaco needs special config under `output: "export"`.

7. `vanilla-extract createThemeContract migration "CSS Modules" codemod large scale` — Find community migration guides or codemods.

8. `"Pixel 9 Pro Fold" Chrome browser version Android 15 inner display viewport CSS bug` — Confirm Chrome version shipped and known web platform bugs.

9. `RTK Query streaming SSE ReadableStream limitations official position` — Confirm RTK Query's stance on streaming endpoints.

10. `WebGL context limit Android Chrome mobile tab count 2024 2025` — Find Android-specific WebGL context count limit for Chrome mobile.
