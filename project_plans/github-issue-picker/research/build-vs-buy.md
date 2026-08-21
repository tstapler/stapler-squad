# Build vs. Buy: GitHubIssuePicker

Date: 2026-07-03
Feature: GitHubIssuePicker (combobox + list, SearchGitHubRepos + ListGitHubIssues RPCs, localStorage TTL cache)

---

## 1. Combobox / ARIA Component

### Headless UI library availability

No headless combobox library is in `web-app/package.json`:

| Library | In package.json? |
|---|---|
| @headlessui/react | No |
| downshift | No |
| cmdk | No |
| @radix-ui/react-combobox | No — only dialog, slot, tabs, tooltip |
| @radix-ui/react-popover | No |

The only Radix packages present are `@radix-ui/react-dialog`, `@radix-ui/react-slot`, `@radix-ui/react-tabs`, and `@radix-ui/react-tooltip`. None provide a combobox primitive.

### Existing bespoke patterns

The project has multiple hand-built autocomplete/combobox-like components:

- `web-app/src/components/ui/AutocompleteInput.tsx` — keyboard nav (ArrowUp, ArrowDown, Enter, Escape, Tab), click-outside close, `role="listbox"` + `role="option"` + `aria-selected`. Missing: `role="combobox"` on input, `aria-expanded`, `aria-controls`, `aria-activedescendant`.
- `web-app/src/components/shared/MultiSelect.tsx` — similar pattern; `aria-haspopup="listbox"` + `aria-expanded` on trigger button; no combobox role.
- `web-app/src/components/ui/PathCompletionDropdown.tsx`, `SlashCommandDropdown.tsx`, `AtCommandDropdown.tsx`, `AliasPalette.tsx`, `QuickOpenPalette.tsx` — multiple precedents for dropdown/palette patterns.
- `web-app/src/components/sessions/OmnibarResultList.tsx` — uses `role="listbox"` + `role="option"`.

### Assessment

**Verdict: Build**, extending the existing `AutocompleteInput` pattern.

Rationale:
- The constraint forbids new npm packages requiring license review. While @headlessui/react is MIT (low risk), adding any new package still violates the letter of the constraint.
- The project has 6+ bespoke dropdown/combobox components demonstrating a clear house style: vanilla React, `useState` for open/highlighted state, keyboard handlers inline, vanilla-extract .css.ts files for styles.
- The ARIA gap (missing `role="combobox"`, `aria-activedescendant`) is real but tractable. W3C ARIA combobox pattern requires: `role="combobox"` on input, `aria-expanded`, `aria-controls={listboxId}`, `aria-activedescendant={activeOptionId}`, `role="listbox"` on list, `role="option"` + `id` on each option. This is ~20 lines of incremental work on top of the existing `AutocompleteInput` pattern.
- `fuse.js` (already in project, `^7.3.0`) can handle client-side fuzzy filtering of cached results.
- `@tanstack/react-virtual` (already in project, `^3.13.25`) is available if the issue list grows large enough to need virtualization.

---

## 2. GitHub API Integration (Backend RPCs)

### Current gh CLI usage

The project already calls the `gh` CLI via `safeexec.CommandContext` in multiple places:

- `server/services/backlog_service.go:1724` — `gh issue view <number> --repo owner/repo --json number,title,body,labels,url,state`
- `server/services/path_completion_service.go:197` — `git worktree list`
- `server/services/unfinished_work_service.go` — uses safeexec

The `gh` CLI pattern is the established project standard for GitHub API access.

### Octokit / REST / GraphQL

No Octokit, GitHub REST client, or GraphQL client is in the codebase. All existing GitHub access goes through `gh` CLI subprocesses. This avoids token management in the frontend entirely — gh handles authentication via its own keyring.

### Proposed gh commands for new RPCs

**SearchGitHubRepos** — use `gh repo list <org> --json nameWithOwner,description,url --limit N` or `gh api /search/repositories?q=<query>` to search across orgs. The `gh api` route allows pagination via `--paginate` and returns raw JSON.

**ListGitHubIssues** — use `gh issue list --repo owner/repo --json number,title,state,labels,url --limit N --search <query>`. This mirrors the existing `gh issue view` call in backlog_service.go.

**Verdict: Use existing gh CLI pattern** via `safeexec.CommandContext`. Do not add Octokit or any new HTTP client. Subprocess overhead (~50–200ms per call) is acceptable given localStorage TTL caching will absorb repeated fetches.

---

## 3. localStorage Caching

### Existing patterns

Two established localStorage cache implementations exist:

- `web-app/src/lib/terminal/TerminalDimensionCache.ts` — simple get/set wrapper with error handling; no TTL.
- `web-app/src/lib/utils/notificationStorage.ts` — full TTL pattern: `NOTIFICATION_TTL_MS`, expiry checked on read, cleanup on write, `Map<string, {notifiedAt, acknowledgedAt}>`.

No SWR or react-query is in the project. Neither is in `package.json`.

### Verdict: Build a custom localStorage TTL cache

Follow the `notificationStorage.ts` pattern:
- Key: `github-cache-<repo>-issues` / `github-cache-repos-<query>`
- Value: `{ data: T[], fetchedAt: number }` (Unix ms timestamp)
- TTL: configurable per cache type (repos: 10min, issues: 2min suggested)
- Cleanup: purge expired keys on write to avoid quota creep
- SSR guard: `if (typeof window === 'undefined') return null`

This requires zero new dependencies and fits the established house style.

---

## 4. Summary Recommendation

| Concern | Decision | Rationale |
|---|---|---|
| Combobox UI | Build (extend AutocompleteInput) | No headless lib available; 6 existing precedents; ARIA gap is ~20 lines of incremental work |
| GitHub API (backend) | Use existing gh CLI via safeexec | Established pattern in backlog_service.go; avoids token management; subprocess latency absorbed by cache |
| localStorage TTL cache | Build (follow notificationStorage pattern) | No SWR/react-query in project; existing TTL pattern is directly reusable |
| Fuzzy filtering | Use existing fuse.js | Already in package.json; no new dep needed |
| Virtualized list | Use @tanstack/react-virtual if needed | Already in package.json; add only if issue list exceeds ~100 items |

**Net new npm packages required: 0.**

---

## 5. Risk Notes

- **ARIA correctness**: The biggest risk is shipping an ARIA combobox that fails screen reader testing. The key attributes to get right are `aria-activedescendant` (must point to the `id` of the focused option element, not just track an index) and `aria-controls` (must match the `id` of the `role="listbox"` element). Storybook with `@storybook/addon-a11y` (already in devDependencies) can catch these.
- **gh CLI subprocess overhead**: Each RPC call adds ~50–200ms latency for gh startup. The TTL cache is essential to make the UI feel snappy. Cache miss on first open is acceptable; subsequent interactions should be instant.
- **gh auth in test environment**: The `safeexec.CommandContext("gh", ...)` calls will fail in CI if `gh auth` is not configured. Existing tests in backlog_service probably mock safeexec or skip live gh calls — verify before writing tests for the new RPCs.
