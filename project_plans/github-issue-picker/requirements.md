# Requirements: github-issue-picker

**Date**: 2026-07-03
**Type**: feature addition
**Complexity**: 2 — focused feature

## Problem Statement

The "Import from GitHub Issue" modal in the backlog page requires users to know and paste the exact GitHub issue URL (e.g., `https://github.com/owner/repo/issues/123`) or the `owner/repo#N` shorthand. This creates friction: users must leave the app to find the issue URL, copy it, come back, and paste it. There is no way to search, browse, or filter issues from within the app.

## Baseline

User opens the "Import from GitHub Issue" modal, sees a plain text input labeled "GitHub Issue URL", must paste a full URL or `owner/repo#N` shorthand from memory or after looking it up externally, then clicks "Import Issue". No autocomplete, no repo browser, no issue search.

## Users / Consumers

Solo developer (Tyler) importing GitHub issues into the stapler-squad backlog to drive triage and planning sessions.

## Success Metrics

- User can import a GitHub issue without leaving the app to look up the URL
- Repo list renders instantly (< 100ms) from local worktree/session data on first open
- Issue search returns results within 2 seconds after typing stops (debounced)
- Repos from GitHub are cached; a second open of the picker shows cached results without a network call
- User can filter issues by open/closed state and labels

## Appetite

Medium (1–2 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints

- Must use the native Go HTTP client in the `github/` package (`github/http_client.go`, `github/client.go`) — not `safeexec.CommandContext` subprocess calls for GitHub data fetching
- Cannot introduce any new npm packages that require a license review (use what's already in `package.json`)
- All new styles must use vanilla-extract `.css.ts` files (CSS modules are not allowed for new components)
- The component must not break the existing raw-URL import flow until explicitly replaced

## Non-functional Requirements

- **Performance SLO**: Repo list from local data < 100ms; issue search debounced at 150ms (research decision, smoother than 300ms), results < 2s
- **Scalability**: Users may have 50–200 GitHub repos; issue search results capped at 30 per query
- **Security classification**: internal (personal dev tool, no multi-user data isolation needed)
- **Data residency**: no special requirements — all data is fetched via native Go HTTP client using the user's local GitHub token

## Scope

### In Scope

- New `GitHubIssuePicker` component replacing the URL text input in the GitHub import modal
- Repo selection UI: tiered — show repos from existing stapler-squad sessions/worktrees first (instant), then allow searching all GitHub repos via `gh repo list`/`gh search repos` (cached)
- Issue search within a selected repo via `gh issue list --json` with filters (state: open/closed, optional label filter)
- Client-side caching of repo list and issue search results (localStorage with TTL of several hours, since repos/issues change infrequently)
- Keyboard navigation: arrow keys to move through results, Enter to select, Escape to close
- Filter controls: open/closed state toggle, label filter (text input that filters already-fetched issues client-side)
- Two new backend RPCs: `SearchGitHubRepos` and `ListGitHubIssues`
- New vanilla-extract CSS for the picker component

### Out of Scope

- Pull requests (issues only for this iteration)
- Real-time issue updates / webhooks
- Pagination beyond the first N results per query (cap at 30)
- Integration into the omnibar (future — component designed to be reusable, but wiring up is deferred)
- Assignee filter (can add later; label + state covers the main use case)
- Creating GitHub issues from within the app

## Rabbit Holes

- **Reusable across omnibar**: The component should be designed cleanly, but wiring it into the omnibar is out of scope. Over-engineering the prop API for hypothetical future callers will inflate scope.
- **Rate limiting**: GitHub API rate limits (60 unauthenticated, 5000 authenticated). Using `gh` CLI uses the user's authenticated token, so 5000/hr is fine for a personal tool. No special rate-limit handling needed.
- **Label autocomplete from API**: Fetching the full label list per repo requires an extra API call. Scope this as a client-side filter on already-fetched issues (filter by substring match), not a server-side label search.
- **Fuzzy matching complexity**: A simple substring match on issue title is sufficient. Don't introduce a fuzzy-search library.

## Alternatives Considered

- **Enhance the URL input with validation only**: Adds URL format hints but still requires the user to know the URL. Doesn't solve the core problem.
- **GraphQL GitHub API directly from the frontend**: Requires managing a GitHub token in the browser. Rejected — using `gh` CLI via the backend is the established pattern and keeps auth out of the frontend.
- **localStorage only (no backend RPCs)**: Possible for caching, but the search itself must go through the backend `gh` CLI since the CLI has the auth token. Client-side caching of results is fine.

## Feasibility Risks

- `gh issue list` performance: listing 30 issues from a single repo takes ~0.3–1s. Acceptable for debounced search.
- `gh repo list` for users with many orgs and repos may take 2–3s. Mitigated by caching and by showing local repos first.
- The existing `gh` CLI safeexec pattern works for issue view; extending it to list/search commands is low risk.

## Open Questions

- Should the picker open inline within the existing modal, or expand it into a larger side panel / full modal? (For research: check if the modal CSS handles larger content gracefully)
- Should issue search be triggered on every keystroke (debounced) or only on explicit submit (Enter/button)? Debounced is smoother UX but requires more careful loading state management.
- Cache TTL: how long to cache repos (proposed: 4 hours) and issue search results (proposed: 5 minutes)?
