# ADR-003: Emit assets under `/_next/static/` through the dual-build period; rename later in one atomic PR

Status: Proposed — default pending owner confirmation (flagged choice F3). Date: 2026-10-19
Evidence: `server/middleware/auth.go:95-101` (`exemptPrefixes`), `server/middleware/static.go:76-78` (immutable cache), `research/pitfalls.md` R1, `research/architecture.md` §2 and F4.

## Context

Go exempts `/auth/`, `/login`, `/health`, `/_next/`, `/favicon` from login and applies `Cache-Control: public, max-age=31536000, immutable` only under `/_next/`. Assets anywhere else are 302'd to `/login` for unauthenticated users, so the login page cannot load its own JS and CSS (lockout), and they lose long-lived caching. No e2e spec references `_next/` (only `#__next-route-announcer__`).

## Decision (default)

1. `Bun.build` `naming` writes chunks, entry and assets under `_next/static/chunks/`, `_next/static/media/`, and so on. `publicPath` is `"/"`. No Go change is needed at cutover.
2. A Go test enumerates the asset URLs in the built `index.html` and asserts each is auth-exempt and gets the immutable header (Story 6.8). This is the F4 guard whatever prefix is used.
3. After the soak window, a follow-up PR renames the prefix (for example `/assets/`) as a single change: `auth.go` `exemptPrefixes`, `static.go` cache rule, the Go test, `build.ts` naming, `web-app/.size-limit.json` globs, docs. The binary embeds the UI, so there is no skew except stale browser tabs (handled by the 404 plus reload-once pair, Stories 1.6 and 2.7).

## Alternatives

- **Rename at cutover**: cleaner URLs sooner, but the cutover PR then mixes a bundler flip with a Go auth change; a lockout bug would be attributed to the wrong cause, and revert is no longer a one-line Makefile flip.
- **Never rename**: leaves a misleading `_next` path after Next is gone. Acceptable indefinitely; the follow-up is optional (Story 9.4).

## Consequences

Cutover stays a pure build flip. Cosmetic debt (`_next` name) is accepted and tracked.
