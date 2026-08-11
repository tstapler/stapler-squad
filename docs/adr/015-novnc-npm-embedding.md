# ADR-015: noVNC Embedding Strategy — npm Package via Next.js Bundler

## Status
Accepted

## Context

FR-5 requires the web UI to embed the noVNC JavaScript RFB client to render the browser passthrough canvas. Three embedding strategies were evaluated.

### noVNC Package Facts

**`@novnc/novnc` v1.7.0** (npm, MPL-2.0). Tarball: ~155 KB (66 files). Entry point: `./core/rfb.js` (ES module). The package ships no HTML wrapper — the consuming application writes its own container. Key dependencies (`vendor/pako/` for zlib) are bundled inside the package; no CDN is required.

The RFB client communicates over binary WebSocket frames (`binaryType = "arraybuffer"`). The Go proxy relays these frames transparently to the x11vnc TCP socket — no VNC protocol awareness is required at the proxy layer.

### Options Considered

**Option A: `@novnc/novnc` npm package, imported via Next.js bundler (Recommended)**

Add `@novnc/novnc` to `web-app/package.json`. Import `RFB` in a React component:

```typescript
import RFB from '@novnc/novnc/core/rfb.js';
```

Because noVNC accesses browser APIs (`WebSocket`, `HTMLCanvasElement`) at module load time, the component must be rendered client-side only. This is accomplished with `next/dynamic` and `{ ssr: false }`:

```typescript
const BrowserView = dynamic(() => import('./BrowserView'), { ssr: false });
```

This is identical to how `xterm.js` is embedded for the terminal tab: `TerminalView` is already wrapped in a `next/dynamic` SSR-disabled import. The pattern is established, understood by the team, and consistent with the rest of the SPA.

The `vendor/pako/` zlib files are included automatically via the import graph — no manual inclusion needed. Webpack tree-shakes unused decoders; the final bundle adds approximately 150 KB gzipped.

**Option B: iframe to a self-hosted noVNC HTML page**

noVNC ships an `app/vnc.html` reference page in its source repository (not in the npm package). This could be extracted and served as a static page by the Go binary, with the React SPA embedding it via `<iframe src="/static/novnc/vnc.html?host=...">`.

Advantages: complete isolation between noVNC's DOM and the React SPA; simpler React component (just an `<iframe>`).

Disadvantages:
- Cross-frame communication for quality controls (FR-10: Low/Medium/High toggle) requires `postMessage`, adding complexity.
- The Browser tab's visual integration with the rest of the session view (tab state, placeholder messaging, status indicators) is harder to implement across an iframe boundary.
- The noVNC HTML page would need to be vendored or fetched from the noVNC source repository — it is not in the npm package. This creates a split between the npm-managed version and the vendored HTML, requiring manual sync.
- The iframe's VNC WebSocket URL must be constructed from the parent page's origin, which complicates WSS handling in different deployment configurations.

**Option C: Vendored static files served via Go `embed`**

Extract noVNC's `core/` and `vendor/` directories from the npm package into `server/static/novnc/` and embed them with `//go:embed`. The React component loads noVNC via a `<script>` tag pointing to the embedded path, or the files are copied into `web-app/public/` and served by Next.js.

Advantages: the feature is available even if the npm build pipeline is unavailable; the Go binary is fully self-contained.

Disadvantages:
- Manual update process: updating noVNC requires copying files manually rather than running `npm update`.
- The `server/static/novnc/` directory adds ~155 KB of vendored JavaScript to the Go repo — a pattern not used elsewhere in the codebase.
- Build-time imports via `import RFB from '/static/novnc/core/rfb.js'` bypass webpack bundling and TypeScript checking.
- The existing pattern for all other frontend assets is npm + Next.js bundling. Adding a vendored static path creates a second, parallel asset pipeline with different update and caching semantics.

### Comparison Summary

| Concern | Option A (npm) | Option B (iframe) | Option C (vendored) |
|---|---|---|---|
| Consistency with existing pattern | Yes — matches xterm.js | No — new iframe pattern | No — new static pipeline |
| Update mechanism | `npm update` | Manual sync | Manual copy |
| Cross-frame communication overhead | None | `postMessage` required | None |
| SSR compatibility | `next/dynamic ssr:false` | Full (iframe) | Partial (script tag) |
| Bundle size impact | ~150 KB gzipped | Served separately | Served separately |
| TypeScript types | Via `@types/novnc` or inline | None | None |

### RFB Integration

The `RFB` object is constructed inside the `BrowserView` React component's `useEffect`:

```typescript
useEffect(() => {
  if (!containerRef.current || !vncReady) return;
  const rfb = new RFB(containerRef.current, wsUrl);
  rfb.scaleViewport = true;
  rfb.resizeSession = false;
  rfb.clipViewport = false;
  return () => rfb.disconnect();
}, [wsUrl, vncReady]);
```

The `containerRef` points to a `<div>` element — RFB creates its own `<canvas>` inside it. `wsUrl` is derived from the session ID and the current page origin (`ws(s)://${location.host}/api/sessions/${id}/vnc`), so TLS is handled automatically (pitfalls.md §4.3).

Clipboard sync is disabled by not passing `clipboardPasteFrom` events and by launching x11vnc with `-noclipboard` (pitfalls.md §4.2).

## Decision

Embed noVNC via the **`@novnc/novnc` npm package**, imported in a `next/dynamic`-wrapped React component with `{ ssr: false }`.

- Add `@novnc/novnc` to `web-app/package.json` as a direct dependency, pinned to a specific minor version.
- The `BrowserView` component is co-located with other session view components in `web-app/src/components/sessions/`.
- The parent `SessionView` imports `BrowserView` via `next/dynamic` with `ssr: false`, matching the xterm.js terminal component pattern.
- Clipboard sync is disabled at both the noVNC and x11vnc layers.
- noVNC version is pinned in `package.json`; updates are subject to the same review process as other frontend dependencies.

## Consequences

### Positive

- Consistent with the xterm.js embedding pattern already in use for the terminal tab — no new patterns to learn.
- Dependency updates follow the standard `npm update` + review workflow.
- webpack includes noVNC in the normal bundle, enabling code splitting, tree shaking, and content-addressed caching.
- TypeScript import of `rfb.js` works with module resolution; type stubs can be added if needed.

### Negative / Constraints

- Adds approximately 150 KB gzipped to the webpack bundle. This is the same order of magnitude as xterm.js (~130 KB gzipped) and is acceptable for a feature-gated tab.
- The `@novnc/novnc` package exports a `.js` extension in its import path (`./core/rfb.js`). TypeScript's `moduleResolution: "bundler"` (used by Next.js 15) handles this correctly, but older `node` resolution may require a path alias in `tsconfig.json`.
- noVNC's `vendor/pako/` (bundled zlib) will be included in the webpack output even though Next.js already has access to a zlib implementation. This is a known noVNC packaging characteristic and has no functional impact.
- `next/dynamic` with `ssr: false` means the Browser tab canvas is not rendered on the server. This is correct — it requires live WebSocket connectivity and a browser DOM.

## References

- Requirements: `project_plans/browser-passthrough/requirements.md` (FR-5)
- Stack research: `project_plans/browser-passthrough/research/stack.md` §3
- Pitfalls: `project_plans/browser-passthrough/research/pitfalls.md` §4.2, §4.3, §4.4
- noVNC npm package: https://www.npmjs.com/package/@novnc/novnc
- noVNC RFB API: https://github.com/novnc/noVNC/blob/master/docs/API.md
- xterm.js embedding (existing pattern): `web-app/src/components/sessions/TerminalView.tsx`
