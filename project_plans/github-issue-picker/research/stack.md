# Technology Stack Research: GitHub Issue Picker

Date: 2026-07-03
Feature: Import from GitHub Issue - in-app picker replacing raw URL input

---

## 1. safeexec.CommandContext Pattern

**Package**: `executor/safeexec/safeexec.go`

`safeexec.CommandContext` is a thin wrapper around `exec.CommandContext` that pre-sets `WaitDelay = 2s` on every command. This prevents zombie processes when a context expires and grandchildren (e.g. git credential helpers) hold pipes open.

**Usage pattern in existing backlog_service.go (lines 1720-1730):**
```go
cmd := safeexec.CommandContext(ghCtx, "gh", "issue", "view", number,
    "--repo", owner+"/"+repo,
    "--json", "number,title,body,labels,url,state")
out, err := cmd.Output()
```

Pattern for new RPCs:
- Create a child context with timeout: `ghCtx, ghCancel := context.WithTimeout(ctx, 30*time.Second)`
- Call: `cmd := safeexec.CommandContext(ghCtx, "gh", <subcommand>, flags...)`
- Capture output: `out, err := cmd.Output()`
- Parse JSON: `json.Unmarshal(out, &target)`
- The `norawexec` lint rule enforces safeexec usage; do NOT use `exec.CommandContext` directly

---

## 2. gh CLI Commands Needed

### gh issue list (for listing issues within a repo)
```
gh issue list --repo OWNER/REPO --state open|closed|all --limit 30 \
    --json number,title,labels,state,url,createdAt,assignees
```
- `--repo` (or `-R`): required for non-current-dir repos
- `--state`: `open` (default), `closed`, `all`
- `--limit` (`-L`): default 30, cap at 30 per spec
- `--json`: comma-separated field list
- `--search`: supports GitHub search query syntax (title, body, comments match)
- Useful JSON fields: `number, title, labels, state, url, createdAt, assignees, body`

### gh repo list (for listing repos by owner)
```
gh repo list [OWNER] --limit 30 --json name,nameWithOwner,description,isPrivate,pushedAt
```
- Owner argument is optional; defaults to authenticated user
- `--limit` (`-L`): default 30
- `--json`: useful fields: `name, nameWithOwner, description, isPrivate, isArchived, pushedAt, defaultBranchRef`
- `--no-archived`: omit archived repos
- `--visibility`: filter `public|private|internal`

### gh search issues (cross-repo issue search)
```
gh search issues QUERY --repo OWNER/REPO --state open --limit 30 \
    --json number,title,labels,url,repository
```
- Command exists: `gh search issues`
- Useful for finding issues when the user types a search term in the picker
- `--repo`: scope to specific repo (but also works without for broad search)
- `--match`: restrict to `title|body|comments`
- JSON fields: `number, title, labels, state, url, repository, createdAt`

**Recommended backend approach for SearchGitHubRepos RPC:**
- Primary: `gh repo list [owner] --limit 30 --json name,nameWithOwner,isPrivate,pushedAt`
- For issue search within repo: `gh issue list --repo OWNER/REPO --search QUERY --state all --limit 30 --json number,title,labels,state,url`

---

## 3. Existing ImportGitHubIssue RPC - ghIssueJSON struct

`server/services/backlog_service.go` already defines:
```go
type ghIssueJSON struct {
    Number int    `json:"number"`
    Title  string `json:"title"`
    Body   string `json:"body"`
    URL    string `json:"url"`
    State  string `json:"state"`
    Labels []struct {
        Name string `json:"name"`
    } `json:"labels"`
}
```
The new `ListGitHubIssues` RPC can reuse this struct (or a similar lightweight struct) for the list response.

---

## 4. ConnectRPC Handler Registration Pattern

**Proto file**: `proto/session/v1/backlog.proto`
**Service**: `BacklogService` — all new RPCs go here (not a new service)

Proto message pattern (add to backlog.proto, append field numbers):
```protobuf
message SearchGitHubReposRequest {
    string query = 1;
    int32 limit = 2;
}
message SearchGitHubReposResponse {
    repeated GitHubRepo repos = 1;
}
message GitHubRepo {
    string name_with_owner = 1;
    string description = 2;
    bool is_private = 3;
}

message ListGitHubIssuesRequest {
    string repo = 1;       // "owner/repo"
    string state = 2;      // "open", "closed", "all"
    string search = 3;     // optional query string
    int32 limit = 4;
}
message ListGitHubIssuesResponse {
    repeated GitHubIssue issues = 1;
}
message GitHubIssue {
    int32 number = 1;
    string title = 2;
    string state = 3;
    string url = 4;
    repeated string labels = 5;
}
```

**Handler registration** (server/server.go, lines 367-381): BacklogService is already registered via `sessionv1connect.NewBacklogServiceHandler(deps.BacklogService, ...)`. New RPCs added to BacklogService in the proto automatically get their generated handler methods; implement them on `*BacklogService` in `server/services/backlog_service.go`.

Pattern for a new handler:
```go
// +api: backlog:search-github-repos
func (s *BacklogService) SearchGitHubRepos(
    ctx context.Context,
    req *connect.Request[sessionv1.SearchGitHubReposRequest],
) (*connect.Response[sessionv1.SearchGitHubReposResponse], error) {
    ghCtx, ghCancel := context.WithTimeout(ctx, 15*time.Second)
    defer ghCancel()
    cmd := safeexec.CommandContext(ghCtx, "gh", "repo", "list", "--limit", "30", "--json", "name,nameWithOwner,isPrivate")
    out, err := cmd.Output()
    // ...
}
```

---

## 5. Vanilla-extract CSS Patterns

**Theme contract**: `web-app/src/styles/theme-contract.css.ts` — all design tokens via `createThemeContract`. Access via `import { vars } from "@/styles/theme.css"`.

**Key available tokens:**
- Colors: `vars.color.textPrimary`, `vars.color.textSecondary`, `vars.color.cardBackground`, `vars.color.borderColor`, `vars.color.hoverBackground`, `vars.color.inputBackground`, `vars.color.primary`, `vars.color.error`
- Spacing: `vars.space["2"]`, `vars.space["4"]`, etc.
- Font sizes: `vars.fontSize.sm`, `vars.fontSize.base`, `vars.fontSize.xs`

**Component CSS file pattern** (from `BacklogItemPanel.css.ts`):
```ts
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
    display: "flex",
    flexDirection: "column",
    borderLeft: `1px solid ${vars.color.borderColor}`,
    background: vars.color.cardBackground,
    selectors: {
        "&:hover": {
            background: vars.color.hoverBackground,
        },
    },
});
```

New file placement: `web-app/src/components/backlog/GitHubIssuePicker.css.ts` (colocated with component).

---

## 6. Frontend Debouncing Pattern

The project uses **manual debouncing with `useRef<NodeJS.Timeout>`** — there is no shared `useDebounce` hook.

Pattern from Omnibar.tsx (lines 158-163):
```ts
const debounceFuseRef = useRef<NodeJS.Timeout | null>(null);
// In an effect or handler:
if (debounceFuseRef.current) clearTimeout(debounceFuseRef.current);
debounceFuseRef.current = setTimeout(() => {
    setDebouncedInput(input);
}, 150);
return () => { if (debounceFuseRef.current) clearTimeout(debounceFuseRef.current); };
```

For the GitHubIssuePicker, use the same pattern with a 500ms delay (spec requires "2 seconds after typing stops" but 500ms is more usable; use 500ms since the spec says debounced):
```ts
const searchDebounceRef = useRef<NodeJS.Timeout | null>(null);
// On input change:
if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
searchDebounceRef.current = setTimeout(() => fetchIssues(query), 500);
```

---

## 7. localStorage Caching Pattern

**Existing pattern** (`usePathHistory.ts`, `usePaneLayout.ts`, `ThemeContext.tsx`):
```ts
const STORAGE_KEY = "github-issue-picker:repo-cache";
const CACHE_TTL_MS = 5 * 60 * 1000; // 5 minutes

interface CacheEntry<T> {
    data: T;
    fetchedAt: number; // epoch ms
}

function readCache<T>(key: string, ttlMs: number): T | null {
    try {
        const raw = localStorage.getItem(key);
        if (!raw) return null;
        const entry = JSON.parse(raw) as CacheEntry<T>;
        if (Date.now() - entry.fetchedAt > ttlMs) return null;
        return entry.data;
    } catch {
        return null;
    }
}

function writeCache<T>(key: string, data: T): void {
    try {
        localStorage.setItem(key, JSON.stringify({ data, fetchedAt: Date.now() }));
    } catch {
        // ignore quota/private-browsing errors
    }
}
```

Always guard reads/writes with try/catch (private browsing, quota exceeded).

---

## 8. Tiered Repo List (Instant + Cached)

The "instant" repo source from existing sessions is already built: `useRepositorySuggestions` (`web-app/src/lib/hooks/useRepositorySuggestions.ts`) calls `client.listSessions({})` and extracts `session.path` ranked by frecency. This gives immediate local paths.

For the picker's tier 1 (instant): extract unique `owner/repo` strings from session paths and worktrees already known to the client.

For tier 2 (GitHub-fetched): call the new `SearchGitHubRepos` RPC, cache results in localStorage with TTL.

---

## 9. Existing Import Modal Location

The current GitHub import form is **inline in the backlog page** (`web-app/src/app/backlog/page.tsx`, around lines 284-573), not in a separate modal component. The `handleImportGitHubIssue` callback calls `importGitHubIssue(githubIssueUrl.trim())` from `useBacklogService`.

The `importGitHubIssue` hook method is in `web-app/src/lib/hooks/useBacklogService.ts` (line 547-564), which calls `clientRef.current.importGitHubIssue({ issueUrl: ..., ... })`.

The new `GitHubIssuePicker` component should replace the `<input>` and submit logic in this inline form section without changing the final `importGitHubIssue()` call — the picker just produces the issue URL/shorthand that feeds into the existing RPC.

---

## 10. Proto Field Numbering

The last field number used in `backlog.proto` is `CancelTriageResponse.cancelled = 1`. The `ImportGitHubIssueRequest` ends at field 3, `ImportGitHubIssueResponse` ends at field 2. New request/response messages start their own numbering from 1. The `BacklogService` already has 18 RPCs; add new ones after `GetSyncHistory`.

---

## Key File Paths

| File | Purpose |
|------|---------|
| `executor/safeexec/safeexec.go` | safeexec wrapper — use this for all gh CLI calls |
| `server/services/backlog_service.go` | All backlog RPCs; add SearchGitHubRepos + ListGitHubIssues here |
| `proto/session/v1/backlog.proto` | Add new messages and RPC declarations |
| `server/server.go` (lines 367-381) | BacklogService registration — no changes needed for new RPCs |
| `web-app/src/app/backlog/page.tsx` | Contains current GitHub import form (replace input with picker) |
| `web-app/src/lib/hooks/useBacklogService.ts` | `importGitHubIssue` hook — add `searchGitHubRepos` + `listGitHubIssues` |
| `web-app/src/lib/hooks/useRepositorySuggestions.ts` | Pattern for fetching sessions as repo sources |
| `web-app/src/lib/hooks/usePathHistory.ts` | localStorage pattern with TTL-free entry scoring |
| `web-app/src/styles/theme-contract.css.ts` | Token definitions (`vars.*`) |
| `web-app/src/components/backlog/BacklogItemPanel.css.ts` | Example vanilla-extract component CSS |
