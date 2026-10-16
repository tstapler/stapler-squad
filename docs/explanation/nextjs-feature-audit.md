# Next.js Guide Adoption Audit

`web-app/next.config.ts` sets `output: "export"`: there is no Next.js/Node server
at runtime, ever. The build produces static files in `web-app/out/`, copied into
`server/web/dist/`, and served by the Go binary (`server/server.go`) on
`localhost:8543`. This audit walks every page in the Next.js docs
[Guides section](https://nextjs.org/docs/app/guides) against that constraint and
the actual state of this repo, to find what's worth adopting.

Two efforts run in parallel elsewhere and are only noted in passing here: a
separate PWA/offline/mobile-shell research task, and an in-progress
implementation of browser-side OpenTelemetry.

## Applicable & recommended

### High priority

**`interactive-apps`** ([guide](https://nextjs.org/docs/app/guides/interactive-apps)) —
`useOptimistic`/`useTransition`/`useActionState` and the `data-pending`
CSS-attribute pattern are plain React, independent of Server Actions, and work
fine under static export. A repo-wide grep found zero usage of any of these
hooks in `web-app/src`; session/backlog mutations (start/stop/pause, approvals)
use plain `useState` plus a manual refetch. Wrapping ConnectRPC mutation calls
in `useOptimistic`/`useTransition` (e.g. `web-app/src/components/sessions/SessionDetailView.tsx`,
backlog action buttons) would make these actions feel instant instead of
waiting on a round trip. (The guide's Suspense-streaming and `"use cache"`
sections are server-only and not applicable.) Tracked in
[#707](https://github.com/tstapler/stapler-squad/issues/707).

**`prefetching`** ([guide](https://nextjs.org/docs/app/guides/prefetching)) —
Base `<Link>` prefetching of static routes (JS chunk warm-up, no server round
trip) works under `output: "export"` since every route here is static, but the
repo has explicitly disabled it: `web-app/src/components/ui/AppLink.tsx`
defaults `prefetch = false` (used in 19 files) with a comment about preventing
CSS preload warnings, and `next.config.ts` separately carries an
`optimizePackageImports` setting and a webpack `splitChunks`/`styles`
cache-group consolidation for the same original problem. Since the CSS-chunk
consolidation may have already fixed the underlying warning, it's worth
re-testing with `prefetch={null}` (hover-triggered prefetch) instead of a
blanket `false` — this could restore navigation-warming benefits without the
eager-viewport cost. Tracked in
[#708](https://github.com/tstapler/stapler-squad/issues/708).

### Medium priority

**`local-development`** ([guide](https://nextjs.org/docs/app/guides/local-development)) —
Two low-risk wins for `web-app/next.config.ts`: Turbopack-for-dev (unused here)
for the `next dev --port ${PORT:-3001}` daily-iteration loop, and auditing
`optimizePackageImports` (currently set only for `@/components`/`@/lib`,
`next.config.ts:22`) against Next's list of auto-optimized third-party
barrel-import packages. Tracked in
[#711](https://github.com/tstapler/stapler-squad/issues/711).

**`package-bundling`** ([guide](https://nextjs.org/docs/app/guides/package-bundling)) —
`web-app/package.json` has `size-limit` as a regression *budget*, but nothing
that shows *what's* in a bundle or its import chain. Adding `@next/bundle-analyzer`
(or the Turbopack bundle analyzer) as a diagnostic — not a CI gate — would help
investigate `size-limit` failures when they happen. Tracked in
[#709](https://github.com/tstapler/stapler-squad/issues/709).

**`content-security-policy`** ([guide](https://nextjs.org/docs/app/guides/content-security-policy)) —
The guide's nonce-based CSP requires a live server and doesn't apply here, but
its static "without nonces" fixed-header approach does. The app currently
ships zero CSP for its own pages — `server/services/file_service.go:789` only
sets `Content-Security-Policy: sandbox` when serving untrusted user-uploaded
files (PDF/HTML previews), not for the app itself. Since `next.config.ts`'s
`headers()` doesn't run under static export either, this has to be added on
the Go side (`server/server.go`) when serving `server/web/dist/`. Tracked in
[#710](https://github.com/tstapler/stapler-squad/issues/710).

### Low priority

- **`debugging`** — no `.vscode/launch.json` exists for attaching a debugger to
  `next dev`; would help web-app contributors, but only touches
  local component-dev iteration, not the shipped app.
- **`upgrading`** — pinned at `next: 15.3.2`; no CVE or breaking-fix motivation
  surfaced. A version bump is a separate, deliberate decision, not something
  this audit should trigger.
- **`ai-agents`** — this guide is actually about Next's own AGENTS.md
  generation and dev-server MCP tooling for agents *working on* a Next.js
  codebase (not, as assumed going in, building agent/chat UIs). `web-app/`
  has no subdirectory `AGENTS.md`; low value since the root `CLAUDE.md` already
  documents web-app conventions comprehensively.
- **`environment-variables`** — no doc captures the guide's key gotcha for this
  setup: under `output: "export"`, every `NEXT_PUBLIC_*` value is frozen into
  the JS bundle at `next build` time, with no runtime env server to reread it
  later. A short `docs/how-to/` note would head off "why didn't my env var
  change take effect" confusion.
- **`view-transitions`** — React's `<ViewTransition>` works client-side under
  static export with zero server dependency; the repo has no current usage.
  Purely a visual nice-to-have (session list → detail, tab switches), and
  needs to be confirmed against the pinned React 19 build first.

## Applicable but already handled

- **`static-exports`** — `web-app/next.config.ts` already matches the guide's
  recommended config; no dynamic route segments, no Route Handlers, no
  rewrites/redirects/headers.
- **`redirecting`** — client-side `router.replace()` already used correctly
  (`web-app/src/app/sessions/new/page.tsx`, `FeatureFlagsContext.tsx`).
- **`client-side-data-fetching`** — Redux (`sessionsSlice`) + WebSocket push
  transport (`useSessionService.ts`, `SessionServiceContext.tsx`) already
  deliver push-based freshness beyond what SWR/TanStack Query would add.
- **`ci-build-caching`** — `.github/workflows/build.yml` already caches
  `web-app/.next/cache` keyed on lockfile + src hash.
- **`testing`** — Jest + Playwright already fully wired (see root `CLAUDE.md`).
- **`analytics`** — `WebVitalsReporter.tsx` already calls `useReportWebVitals`
  into a homegrown pipeline, the guide's own recommended "build your own" path.
- **`css-in-js`** — vanilla-extract (an explicitly guide-supported,
  zero-runtime library) is already the adopted CSS architecture
  (`docs/reference/css-architecture.md`).
- **`lazy-loading`** — `next/dynamic` already used in 10 files, including the
  guide's recommended pattern for heavy client-only libraries
  (`web-app/src/app/insights/InsightsDashboard.tsx`'s Recharts components,
  `XtermTerminal.tsx`).
- **`preventing-flash-before-hydration`** — `web-app/src/app/layout.tsx`
  already has an inline FOUC-prevention script matching the guide's
  recommended theme pattern almost verbatim.
- **`single-page-applications`** — this app already follows the guide's own
  patterns (static export, `next/dynamic` for browser-only components, shallow
  routing via `history.replaceState` in `SessionDetailView.tsx`).
- **`migrating`** — already fully on the App Router; no `pages/` directory.

## Not applicable (requires a Next.js server)

- `adopting-partial-prefetching` — requires Cache Components + a live server.
- `caching-without-cache-components` — requires server-side `fetch`/`unstable_cache` revalidation.
- `cdn-caching` — requires a live server issuing per-request response headers.
- `draft-mode` — requires request-time `cookies()`-based cache bypass.
- `how-revalidation-works` — requires a running Next.js server process.
- `incremental-static-regeneration` — guide's own platform table excludes static export.
- `incremental-static-regeneration-cache-components` — requires background re-rendering on a live server.
- `migrating-to-cache-components` — Cache Components requires the Node.js runtime.
- `ppr-platform-guide` — written for CDN/adapter engineers fronting a live origin.
- `rendering-philosophy` — conceptual model premised on a live server blending static/dynamic per request.
- `streaming` — guide's own platform table excludes static export.
- `public-static-pages` — requires Cache Components + Suspense-based per-request dynamic content.
- `authentication` — requires Server Actions + `cookies()` at request time (repo's auth is Go-backend WebAuthn/passkey).
- `authentication-with-cache-components` — requires a live server.
- `backend-for-frontend` — requires Route Handlers/proxy at request time (the Go server already fills this role).
- `custom-server` — no Next.js server process exists to eject from.
- `instrumentation` — `register()` hook requires a server instance (Go-side OTel already covers this).
- `mcp` — requires a running `next dev` server to connect to.
- `multi-tenant` — requires live server dynamic per-tenant routing.
- `multi-zones` — requires a live server and multiple deployed Next.js zones.
- `server-actions` — cannot exist under static export.
- `deploying-to-platforms` — assumes deploying Next.js itself as a running service.
- `self-hosting` — assumes a `next start` server process.
- `instant-navigation` — requires Cache Components (Next 16) + live rendering runtime.
- `optimizing-prefetching` — requires Cache Components server-side prefetch resolution per link.
- `preserving-ui-state` — requires Cache Components (Next 16); repo is on 15.3.2.
- `forms` — the guide is Server Actions-specific (react-hook-form is already the applicable client-side substitute).
- `building` — the guide's prerender-blocking-error debugging is Cache Components machinery for a live server.

## Covered by parallel research

- `progressive-web-apps` — covered by separate PWA research.
- `offline-support` — covered by separate PWA research.
- `open-telemetry` — in progress, being implemented directly right now.

## Skip / not worth it

- `memory-usage` — build-time webpack heap tuning; reach for it only if a build starts OOMing, no evidence of that today.
- `production-checklist` — mostly already covered by App Router defaults; its novel items (CSP, tainting, Server Action auth) target dynamic surfaces this app doesn't have.
- `data-security` — its Data Access Layer/tainting advice targets Server Component/Action code paths that don't exist here; all data access already goes through Go-backend-enforced ConnectRPC.
- `server-and-client-boundary` — the RSC module-graph model's real payoff (keeping server secrets out of client bundles) doesn't map onto a codebase with no server secrets in the Next layer at all.
- `mdx` — no `.mdx` files or `@next/mdx` dependency; project docs are plain markdown rendered by GitHub, not authored long-form content.
- `sass` — no `sass` dependency or `.scss` files; would fragment the existing vanilla-extract architecture.
- `scripts` — no `next/script` usage or third-party scripts loaded today; nothing to retrofit yet.
- `tailwind-v3-css` — no Tailwind dependency; adopting it would mean replacing the deliberately-chosen CSS architecture.
- `third-party-libraries` — no GTM/GA4/Maps/YouTube embeds present; nothing to retrofit yet.
- `internationalization` — English-only personal/small-team tool, no localization need.
- `videos` — only plain `<video controls>` for local file preview (`LocalFileBrowser.tsx`, `FileContentViewer.tsx`); the guide's actual subject (external hosting, CDN delivery, adaptive bitrate) doesn't apply.
- `json-ld` — the app sits behind auth and isn't publicly indexed; structured data for search/AI crawlers has no audience here.
