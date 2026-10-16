# Implementation Plan: Built-in Changelog

## Architecture

```
CHANGELOG.md (embedded)
    │
    ▼
pkg/changelog/parser.go          ← parse CHANGELOG.md into []Release structs
    │
    ▼
/api/changelog  (server/server.go or new handler file)
    │
    ▼
web-app: /changelog route         ← full changelog list
web-app: /settings → About tab   ← version badge + link + update status
```

```
github/releases.go               ← GetLatestRelease (ETag-aware, uses NewConditionalRequest)
    │
    ▼
server/services/update_check_service.go  ← background goroutine, 6h poll, atomic result
    │
    ▼
/api/update-check                ← serves cached result to UI
    │
    ▼
web-app: UpdateBadge component   ← dot badge / dismissable banner
```

---

## Epic 1 — CHANGELOG.md Embedding and Parsing

### Story 1.1 — Embed CHANGELOG.md

**File:** `server/changelog_embed.go`

```go
//go:embed CHANGELOG.md
var changelogContent []byte
```

Note: `//go:embed` directives must live in a `.go` file in the same directory as the target file. Since `CHANGELOG.md` is at the repo root, this file must also be at the repo root (same package `main`) or in a subpackage with a relative path. Use `//go:embed ../CHANGELOG.md` from a `pkg/changelog` subpackage OR place the embed in a root-level `server_changelog_embed.go`. Recommended: root-level `changelog_embed.go` in `package main`, exporting `changelogContent` via `pkg/changelog` init from a passed-in byte slice.

**Simpler approach:** `pkg/changelog/content.go`:
```go
package changelog

// Content is set by main() from the embedded CHANGELOG.md bytes.
// It is populated before any handler or background service reads it.
var Content []byte
```

And in `main.go`:
```go
//go:embed CHANGELOG.md
var changelogMD []byte
// then in init or before server start:
changelog.Content = changelogMD
```

This keeps the `//go:embed` in `package main` (where `CHANGELOG.md` lives) and avoids `../` relative paths.

### Story 1.2 — CHANGELOG.md Parser

**File:** `pkg/changelog/parser.go`

```go
type Entry struct {
    Scope   string   // e.g. "backlog", "web", "session" (from **scope:** prefix)
    Text    string   // human-readable description
    PRs     []int    // referenced PR numbers
    Commits []string // short commit SHAs
}

type Section struct {
    Type    string  // "feature" | "fix" | "perf" | "revert"
    Entries []Entry
}

type Release struct {
    Version  string    // "2.5.0"
    Date     time.Time
    CompareURL string
    Sections []Section
}

func ParseChangelog(content []byte) ([]Release, error)
```

Parser state machine:
1. Scan for `## [VERSION](url) (DATE)` lines → new Release
2. Within a release, scan for `### Features` / `### Bug Fixes` / `### Performance Improvements` / `### Reverts` → new Section
3. Within a section, `* **scope:** text (#PR) (commit)` or `* text (#PR) (commit)` → Entry

Unit tests: `pkg/changelog/parser_test.go` — test against a 3-release synthetic excerpt and against the real `CHANGELOG.md` (golden file or spot-check assertions).

### Story 1.3 — `/api/changelog` Endpoint

**File:** `server/server.go` (add to `Build()`) or new `server/changelog_handler.go`.

```
GET /api/changelog
  → { releases: Release[] }  (all releases, newest first)

GET /api/changelog?since=2.3.0
  → { releases: Release[] }  (only releases newer than given version)

GET /api/changelog/current
  → { release: Release | null }  (the release matching buildinfo.Version)
```

Response is computed once from the parsed content at startup (not per-request). Cache parsed releases in `Server` as `[]changelog.Release` parsed from `changelog.Content` during `Build()`.

---

## Epic 2 — Update Check Service

### Story 2.1 — GitHub Latest Release Fetcher

**File:** `github/releases.go`

```go
type LatestRelease struct {
    TagName     string    // e.g. "v2.6.0"
    Version     string    // "2.6.0" (tag without "v")
    HTMLURL     string    // GitHub release page URL
    PublishedAt time.Time
    Body        string    // release notes (first 1000 chars max)
}

// GetLatestRelease fetches the latest release from the GitHub REST API.
// Uses NewConditionalRequestNoCache (or NewConditionalRequest with a per-call
// ETag cache) to respect 304 Not Modified responses.
func GetLatestRelease(ctx context.Context, owner, repo string, etagCache *ETagCache) (*LatestRelease, bool /* fromCache */, error)
```

Endpoint: `GET /repos/{owner}/{repo}/releases/latest`.
Uses `newGHRequestForHostWithToken` / `NewConditionalRequest` per the `norawghrequest` rule.

### Story 2.2 — Update Check Background Service

**File:** `server/services/update_check_service.go`

```go
type UpdateCheckResult struct {
    CurrentVersion string
    LatestVersion  string    // empty if check hasn't run or failed
    IsOutdated     bool
    IsSourceBuild  bool      // true when buildinfo.Branch/Commit is non-empty
    LastCheckedAt  time.Time
    LatestURL      string
    Error          string    // non-empty if last check failed (informational)
}

type UpdateCheckService struct {
    result atomic.Pointer[UpdateCheckResult]
    // ...
}

func (s *UpdateCheckService) Start(ctx context.Context)
func (s *UpdateCheckService) Current() *UpdateCheckResult
```

- Fires first check 30s after startup (let the server settle).
- Re-checks every 6 hours.
- Never checks if `buildinfo.Version == "dev"` or `buildinfo.Branch != ""` (source builds) — sets `IsSourceBuild = true` and skips network call.
- Errors are logged at Info level and stored in `result.Error`; they never propagate to the UI as errors.
- Feature flag `"changelog_update_check"` (child of `"changelog"`): when disabled, the service starts but always returns `IsOutdated: false` without making network calls.

### Story 2.3 — `/api/update-check` Endpoint

```
GET /api/update-check
  → UpdateCheckResult (JSON)
```

Registered similarly to `/api/server-info`. Returns the cached `UpdateCheckService.Current()` result.

---

## Epic 3 — Web UI

### Story 3.1 — `/changelog` Page

**File:** `web-app/src/app/changelog/page.tsx`

- Fetches `GET /api/changelog` on mount.
- Renders a list of releases, each collapsible, with version/date headers.
- The release matching the current running version (from `/api/server-info`'s `version` field) is expanded by default and visually highlighted (e.g. a "You are here" chip).
- Source build banner: if `branch` or `commit` is non-empty in the server-info response, show a `Source build · branch: X · commit: Y` banner above the changelog list.
- Feature flag `"changelog"` gates this page (redirect to `/` if disabled).
- Accessible: headings use proper hierarchy, collapsible sections use `aria-expanded`.
- Mobile-friendly: stacked card layout, no horizontal overflow.

### Story 3.2 — About Section in Settings

**File:** `web-app/src/app/settings/page.tsx` (add new tab or section)

Add an "About" section (or tab) to Settings showing:
- Version badge: `v2.5.0` (tagged release) or `dev · branch: main · commit: abc123f` (source build).
- "What's New" link → `/changelog`.
- Update status: if `UpdateCheckResult.IsOutdated`, show "v2.6.0 available — view release notes" with link; if up to date, show "Up to date"; if IsSourceBuild, show nothing.

**Alternative (lower effort):** Add to the existing Settings General tab's "Help" subsection — no new tab needed. A link "What's New ↗" and the version string. This is the minimum viable option.

### Story 3.3 — Update Badge (Optional Enhancement)

**File:** `web-app/src/components/UpdateBadge.tsx`

A small dot badge on the Settings nav icon when `IsOutdated === true`. Dismissable (stored in localStorage). Uses the existing notification/badge patterns in the codebase.

**Decision:** Implement this in the same PR as 3.1/3.2 only if the badge pattern doesn't require significant new infrastructure. Otherwise defer to a follow-up.

---

## Epic 4 — Feature Flag and Registry

### Story 4.1 — Feature Flag

Add `"changelog"` flag with default `true` to the seed flags in `server/services/`. Pattern follows existing flags like `"backlog"`, `"pi"`.

### Story 4.2 — Feature Registry

Run `make registry-generate` after adding the `/api/changelog` RPC/handler and the new React page/component. Add `// +feature: changelog` markers to the new files.

---

## Implementation Order

1. **Story 1.2** (parser) — most isolated, easiest to test independently. Write tests first.
2. **Story 1.1** (embed) — wire `CHANGELOG.md` bytes into `pkg/changelog`.
3. **Story 1.3** (`/api/changelog`) — needs 1.1 + 1.2.
4. **Story 4.1** (feature flag) — parallel with 1.x.
5. **Story 3.1** (`/changelog` page) — needs 1.3.
6. **Story 3.2** (About section in Settings) — can be done alongside 3.1.
7. **Story 2.1** (GitHub release fetcher) — parallel with 3.x, needs `github` package knowledge.
8. **Story 2.2** (update check service) — needs 2.1.
9. **Story 2.3** (`/api/update-check`) — needs 2.2.
10. **Story 3.3** (update badge) — needs 2.3; optional.
11. **Story 4.2** (registry) — final step before PR.

---

## Risk Register

| Risk | Likelihood | Mitigation |
|---|---|---|
| CHANGELOG.md `//go:embed` path issue (file must be in same dir as `.go` file using it) | Medium | Put the embed in `package main` (`changelog_embed.go` at repo root); pass bytes to `pkg/changelog` |
| GitHub release API rate limit on update check | Low | ETag + 6h TTL; 1 full request then 304s |
| `norawghrequest` analyzer flags new GitHub call | Medium | Use `newGHRequestForHostWithToken` pattern; ensure no raw `http.NewRequest` |
| CHANGELOG.md parser breaks on format changes | Low | Parser is tested against current format; release-please format is stable |
| Update check leaks user IP to GitHub | Medium | Default ON but document; add opt-out toggle in settings |
| jscpd threshold exceeded by new test files | Low | Keep test files focused; the 0.14% threshold has headroom |
