# Research: Built-in Changelog

## Existing Infrastructure

### Version / Build Provenance (`pkg/buildinfo`)

`pkg/buildinfo/buildinfo.go` exposes four package-level vars set via ldflags:
- `Version` — semver string, set by Makefile (`-X .../buildinfo.Version=$(VERSION)`) and copied from `main.version` at startup for GoReleaser builds.
- `Branch`, `Commit` — only set by the Makefile (`make install-service`), never by GoReleaser; non-empty only for locally compiled binaries.
- `Worktree` — `"true"` when the checkout was dirty at build time.

`main.go`'s `resolveDevVersion()` additionally reads `debug.ReadBuildInfo()` (Go's automatic VCS stamping) to replace `"dev"` with a real commit-derived version for bare `go build`/`go install` runs. The net result: the running binary always knows its provenance, even without ldflags.

### `/api/server-info`

`server/server.go:registerServerInfoHandler()` already returns:
```json
{
  "version": "2.5.0",
  "branch": "",
  "commit": "",
  "worktree": false,
  "binary_path": "/home/user/.local/bin/stapler-squad",
  ...
}
```
`branch`/`commit`/`worktree` are `omitempty` — absent for tagged releases, present for local builds.

### CHANGELOG.md

- File: repo root, 2280 lines, maintained by release-please.
- Format: standard keep-a-changelog / release-please Markdown: `## [VERSION](compare-url) (DATE)` headers, `### Features` / `### Bug Fixes` / `### Performance Improvements` / `### Reverts` subsections, `* entry` bullet items with optional `(#PR) (commit)` suffixes.
- Sections are hidden per `release-please-config.json` for `chore`, `docs`, `style`, `refactor`, `test`, `build`.
- The file is checked into the repo and Git-tracked. It is not currently embedded.

### GitHub Releases

- Releases are at `https://api.github.com/repos/tstapler/stapler-squad/releases/latest` (GitHub REST) or via GraphQL.
- The existing `github/etag_cache.go` provides `NewConditionalRequest` / `GetPRInfoConditional` for ETag-aware GET requests to GitHub REST. That pattern should be reused for the release check to avoid rate-limit waste.
- There is no existing "check for app updates" code anywhere in the codebase.

### Feature Flags

`server/services/` hosts feature flag infrastructure. Flags are toggled via `UpdateFeatureFlag` RPC and read via `useFeatureFlags()` in the React layer. Adding a new `"changelog"` flag follows the same pattern as `"backlog"`, `"pi"`, etc.

### Settings / UI

- Settings page: `/app/settings/page.tsx` — tabbed layout (General, Config Files, Appearance, Keyboard Shortcuts).
- The Config Files tab (`ConfigPageContent.tsx`) already calls `/api/server-info` and displays version info.
- No existing "About" section, "What's New" route, or version badge in the nav.

## Design Patterns Found

### Embedded files
The codebase already uses `//go:embed` for static assets (web UI bundle, etc.). The CHANGELOG.md embed follows the same pattern.

### Markdown parsing server-side
The changelog parsing can be done purely in Go with a small state-machine parser (no external library needed) that reads the `CHANGELOG.md` content and groups it by release version. The format is stable (release-please generates it consistently).

### Polling background services
Several background pollers exist (PR poller, health checker). An optional update check can be added as a similar background goroutine that fires once at startup (after a short delay to not block) and re-checks on a configurable interval (default 6h), writing its result into an `atomic.Pointer`.

### ETag / conditional requests
`github/etag_cache.go`'s `NewConditionalRequest` sends `If-None-Match` and handles `304 Not Modified`. This is the required pattern per `norawghrequest` rule.

## Alternatives Considered

### Fetch CHANGELOG.md from GitHub at runtime
Rejected: requires network access, fails offline, adds latency. The embedded approach is zero-dependency at runtime.

### Parse markdown in the React layer
Possible (react-markdown or similar), but would require shipping a markdown parser to the browser. Server-side parsing to structured JSON is cleaner and already follows how the rest of the API works.

### Dedicated `/changelog` route vs Settings tab
Both viable. A dedicated route (`/changelog`) is better UX (linkable, deep-linkable from a "What's New" banner), and a compact version badge in the Settings sidebar can link to it. The Settings page already has a pattern of linking out to dedicated pages (`Pipeline Modes ↗`, etc.).

### Rate limit risk for update check
Using `NewConditionalRequest` with ETag caching + a 6-hour TTL means the actual GitHub API costs are: 1 full request at startup (or first check), then `304 Not Modified` responses thereafter until the release actually changes. This is negligible even with many instances.

## Open Questions

1. **Update check opt-in vs opt-out?** The check makes an outbound network call to `api.github.com`. For privacy-conscious users this may be undesirable. Recommend: default ON but surfaced in settings as a toggle; or default OFF and opt-in.
2. **What to show for `dev` version?** When version is `"dev"` and `buildinfo.Commit` is also empty (very unusual), we can't point to a changelog section. Show the full changelog from latest release down.
3. **Notification badging strategy?** Should the nav show a numeric badge or a dot? Keep it minimal — a dot badge on the settings icon (or a dismissable banner) is sufficient; avoid notification-tray overhead.
