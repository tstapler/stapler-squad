# Built-in Changelog

## Problem Statement

Users of stapler-squad have no in-app way to discover what changed between releases or whether their running build is up to date. They must navigate to GitHub releases manually. For source builds (which are common — the README documents `go build` and `go install` as the primary install paths), the version provenance (branch, commit) is not surfaced anywhere in the UI, making it hard to know exactly what code is running.

## Goals

1. Surface the changelog inside the app so users can see what changed without leaving the UI.
2. Show the current build's provenance (version, commit, branch) prominently and in context alongside the changelog.
3. Optionally notify the user when a newer release is available so they can update.
4. Support source builds: when `buildinfo.Commit`/`buildinfo.Branch` is populated (i.e. a locally compiled binary), display that clearly so users know their build came from a checkout, not a tagged release.

## Out of Scope

- Auto-updating / one-click upgrade (would require OS-specific package management integration).
- Showing changelogs for components other than stapler-squad itself (e.g. tmux, Claude Code CLI).
- Backfilling changelog entries for releases older than what `CHANGELOG.md` covers.

## Acceptance Criteria

1. There is a `/changelog` route (or a prominent "What's New" tab in Settings) that renders the parsed contents of `CHANGELOG.md` embedded in the binary — no network call required to see the changelog.
2. The changelog view shows the running version highlighted (or scrolled-to) so users can see where they are.
3. Build provenance (version, commit SHA, branch, "source build" badge when `Branch`/`Commit` is non-empty) is displayed near the top of the changelog or About section.
4. The `/api/changelog` endpoint returns structured JSON: list of releases, each with version string, date, and categorised entries (features, bug fixes, etc.) — parsed from the embedded `CHANGELOG.md`.
5. For tagged releases (not source builds), the app optionally checks GitHub Releases API for a newer tag and shows a non-intrusive update badge/banner. The check uses ETags (via `NewConditionalRequest`) to avoid wasting GitHub API quota. The check can be disabled via a feature flag.
6. The update check result is cached with a TTL (≥ 1 hour) so the server never hammers GitHub; the last-checked time and latest-version are surfaced in the UI.
7. Source builds (where `buildinfo.Branch`/`buildinfo.Commit` is non-empty) show a distinct "Source build — branch: X, commit: Y" indicator rather than an update nudge (there is no "latest release" concept for a dev build).
8. The changelog page and version info are accessible at mobile screen widths (consistent with the project's mobile UX requirement).
9. The feature is guarded by a feature flag (`changelog`) defaulting to enabled; disabling it hides the route and the update badge.
10. Unit tests cover: CHANGELOG.md parser (sections, entries, version extraction), update-check cache TTL and ETag logic, and the React changelog component rendering with mocked API data.

## Non-Functional Requirements

- CHANGELOG.md is embedded at compile time (`//go:embed`); no filesystem access at runtime.
- The GitHub release check is fire-and-forget: a failed check (network error, rate limit) is silently swallowed and never breaks the UI.
- The update badge/banner must not appear for source builds or when running version is `"dev"`.

## Reference Points

- `CHANGELOG.md` — 2280 lines, release-please format, covers v0.x → v2.5.0
- `pkg/buildinfo` — `Version`, `Branch`, `Commit`, `Worktree` injected via ldflags
- `/api/server-info` — already returns `version`, `branch`, `commit`, `worktree`, `binary_path`
- `github/etag_cache.go` — `NewConditionalRequest` pattern for ETag-aware GitHub REST calls
- Settings `ConfigPageContent.tsx` — existing `/api/server-info` fetch pattern in the React layer
- `release-please-config.json` — confirms `tstapler/stapler-squad` on GitHub as the release repo
