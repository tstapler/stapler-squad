# Research: Stack / Libraries / APIs

Scope: what GitHub client, auth, and existing mutation/idempotency patterns
apply to two-way backlog↔issue sync (forward: backlog status/labels → GitHub
issue; backward: GitHub issue state/labels → backlog item).

## 1. GitHub API client — no go-github SDK; two parallel raw-HTTP stacks exist

`go.mod` has **no** `github.com/google/go-github` (or any GitHub SDK)
dependency — confirmed via `grep -n "go-github" go.mod` (no match). All GitHub
API access in this repo is either a raw `net/http` REST client or the `gh` CLI
shelled out via `safeexec`. Two independent implementations exist and a new
sync feature should pick (or reconcile) between them:

**A. `session/backlog_plugin_github.go` / `backlog_plugin_github_prs.go`** (the
code the new sync directly extends)
- Builds requests by hand: `http.NewRequestWithContext` + `req.Header.Set("Authorization", "token "+cfg.Token)` (old-style `token` scheme, not `Bearer`).
- Per-source token comes from **plugin config JSON** (`cfg.Token`, decrypted from `ItemSource.config`), not from env/keychain — this is a source-scoped PAT, separate from the shared `github` package's token resolution (see §2).
- Shared helper `githubAPIURL(host, pathAndQuery)` (`session/backlog_plugin_github.go:28`) builds the URL and already supports GHE hosts via `github.RestBaseURLForHost`.
- Client is a bare `&http.Client{Timeout: 30 * time.Second}` — no shared rate-limit transport, no retry.
- `GitHubIssuesPlugin.Fetch` (`session/backlog_plugin_github.go:74`) currently queries `issues?state=open&per_page=50&since=<cursor>` — hardcoded `state=open`. **`state=all` is a valid, supported GitHub REST query param** (`GET /repos/{owner}/{repo}/issues?state=all`) and is exactly what's needed to observe closed/reopened issues per the requirements doc; this is a one-line change to the query string (`state=open` → `state=all`), not a client capability gap.
- `githubIssue` struct (`session/backlog_plugin_github.go:46`) does not currently decode `state` or `closed_at` — both are plain top-level fields on the GitHub issue JSON and can be added to the struct trivially.
- GitHub's REST API supports both mutations the backward/forward sync needs on the same `/repos/{owner}/{repo}/issues/{number}` resource:
  - **Closing/reopening**: `PATCH` with body `{"state":"closed"}` / `{"state":"open"}` (also accepts `state_reason`).
  - **Labels**: `PUT /repos/{owner}/{repo}/issues/{number}/labels` (replace all) or `POST .../labels` (add). None of this exists yet in the codebase for issues (only for PRs, via `gh pr close`/`gh pr merge`, see §3) — it's new code, but it's the same REST resource this plugin already reads from, so wiring a `PATCH`/`PUT` call into the same file is a natural, small extension.

**B. `github/*.go` package** (used by the PR-lifecycle/backlog machinery, not the plugin)
- `github/http_client.go` has a **shared, cached token resolver**: `getGHToken` — precedence `GITHUB_TOKEN` env → `GH_TOKEN` env → OS keychain (`GetKeychainToken()`, cached 1 min via `atomic.Value`/`atomic.Int64`). Uses modern `Authorization: Bearer <token>` + `X-GitHub-Api-Version: 2022-11-28` headers (`newGHRequestForHostWithToken`, `github/http_client.go:60`).
- `github/rate_limit.go`'s `DefaultRateLimiter`/`RateLimiter.Update` parses `X-RateLimit-Remaining`/`-Reset` and `Retry-After` to track primary/secondary GitHub rate limits and exposes `IsLimited()`/`WaitIfLimited(ctx)` for pollers to check before dispatching work. This is **not currently wired into the backlog plugin's bare `http.Client`** — the plugin only special-cases a 429/403-with-zero-remaining response into an error string (`backlog_plugin_github.go:110`), it doesn't consult or update `DefaultRateLimiter`.
- `github/hosts.go` (`NormalizeHost`, `IsGitHubCom`, `RestBaseURLForHost`) is shared by both stacks and already GHE-aware.
- `github/etag_cache.go` + `GetPRForBranchConditional` (`github/client.go:716`) show the codebase's existing conditional-request (If-None-Match/ETag) pattern for cheap re-polling — a candidate technique for backward-sync polling of issue state without burning rate-limit budget on unchanged issues, though nothing in the issues plugin uses it today.

**Recommendation for the plan phase**: extend `backlog_plugin_github.go` in place (same file, same per-source-token model) rather than pulling in `github/*.go`'s shared-token model or a new SDK — the existing plugin's per-source PAT-in-config design is intentional (each ItemSource can point at a different repo/host/token) and mismatches the shared package's single-process-token assumption. Do consider borrowing `RateLimiter`'s header-parsing logic (or the type itself) since the plugin's current rate-limit handling is a bare error string with no backoff.

## 2. Auth mechanism in the existing Fetch call

- `GitHubIssuesPlugin.Fetch` / `GitHubPRsPlugin.Fetch` read `cfg.Token` from the **per-`ItemSource` decrypted config JSON** (`ItemSource.config` field, ent schema `session/ent/schema/item_source.go:26`, commented "JSON with encrypted PAT"). No token = plugin returns an empty fetch, not an error (`if cfg.Token == "" { return nil, cursor, nil }`) — i.e. silently disabled, by design.
- Auth header is the legacy `Authorization: token <PAT>` scheme (still accepted by GitHub, just not the newer `Bearer` form the shared `github` package uses).
- Rate-limit handling: manual check for `429` or (`403` + `X-RateLimit-Remaining: 0`) → returns a hard error string; no `Retry-After`/reset-time backoff, no shared rate limiter consulted. Contrast with `github/rate_limit.go`'s much richer primary/secondary detection.
- Forward-sync mutations (closing/labeling an issue) will need to reuse this **same per-source token** — the mutation must be attributed to the same authenticated identity/token the source was configured with, not the shared env/keychain token, since a given backlog source may point at a repo the ambient `GITHUB_TOKEN` has no access to.

## 3. Existing issue/PR mutation code to mirror

No existing code mutates **GitHub issues** (close/reopen/label) anywhere in the repo — this is genuinely new. However, `github/client.go` already has the equivalent **PR** mutation pattern to mirror structurally:
- `ClosePR` (`github/client.go:575`) — shells out `gh pr close <num> --repo <owner/repo>` via `safeexec.CommandContext`.
- `MergePR` (`github/client.go:543`) — `gh pr merge` with `--squash`/`--rebase`/`--merge`.
- `PostPRComment` (`github/client.go:520`) — `gh pr comment --body <text>`.
- All three follow the same shape: `CheckGHAuth()` guard → build `owner/repo` + number refs → `safeexec.CommandContext(ctx, "gh", args...)` → typed error unwrap via `*exec.ExitError`.

Per `.claude/rules/prefer-go-git-over-subshells.md`, the project's general policy is to prefer native Go/API calls over subshells where a library already does the job — and per §1, the plugin's existing `net/http` REST calls already read issues natively. **The natural-fit approach for issue mutation is a native REST call in `backlog_plugin_github.go` (PATCH state, PUT labels) using the same per-source token and `githubAPIURL` helper**, not a new `gh issue close`/`gh issue edit` subshell — that would introduce a third auth/access pattern (relies on `gh` CLI being authed for that specific repo/host) alongside the two already in the codebase.

## 4. Loop-prevention / idempotency patterns already in the codebase

There is **no existing "synced by us" marker, content hash, or explicit
loop-prevention mechanism** for the backlog↔GitHub direction — confirmed by
grepping for `SyncedBy|LastSyncHash|ContentHash|selfWrite` etc. across
`session/*.go` (no hits outside worktree copies). The closest analogous
pattern already in the codebase, reusable as a model, is the **per-item
watermark** used for PR-feedback dedup:

- `BacklogItem.pr_feedback_addressed_at` (`session/ent/schema/backlog_item.go:102`, `time.Time`, optional/nillable) — "the newest substantive PR review-feedback timestamp a fix session has already been dispatched to address." Set in `session/backlog_lifecycle.go:4183-4198` (`ReconcilePRPending`): after successfully acting on feedback, the item is updated with `PrFeedbackAddressedAt: &watermark` where `watermark := prStatus.LatestFeedbackAt`. On the next poll, `hasNewFeedback := ... && (item.PrFeedbackAddressedAt == nil || prStatus.LatestFeedbackAt.After(*item.PrFeedbackAddressedAt))` — i.e. only feedback strictly newer than the stored watermark triggers action again.
- This is a **high-water-mark, not a hash/signature** — it works because GitHub timestamps are monotonic per-resource and the field only needs "have we already reacted to everything up to time T." The same shape applies directly to backward sync: a `github_synced_at` (or similar) timestamp on `BacklogItem`, compared against the issue's `updated_at`, would let backward sync skip issues it has already reconciled, and forward sync could set that same watermark right after pushing a status/label change so the very next backward-sync poll doesn't see its own write as an "external change" and bounce it back.
- `ItemSource.last_synced_at` / `ItemSource.sync_cursor` (`session/ent/schema/item_source.go:31-35`) are the existing **per-source** (not per-item) forward-fetch cursor/watermark — same idiom, coarser granularity. The new per-item watermark would live alongside these at the `BacklogItem` level, following the existing `Optional().Nillable()` + explicit `Clear*` update-field convention already used for `pr_feedback_addressed_at` / `ClearPrFeedbackAddressedAt` in `BacklogItemUpdate` (`session/repository.go:546-554`, applied in `session/ent_repository_backlog.go:627-629`).

## 5. Schema gaps confirmed

- `BacklogItem` ent schema (`session/ent/schema/backlog_item.go`) has `external_id` (string, optional) but **no `external_url` and no `labels`/`external_labels` field** — both need to be added, consistent with the requirements doc. `ExternalItem` (the plugin's fetch-result struct, `session/backlog_plugin.go:21`) already carries `Labels []string` and `URL string`, but `BacklogItemData.MapToBacklogItem` (both `backlog_plugin_github.go:166` and `_prs.go:196`) currently drops them on the floor when mapping into `BacklogItemData` — the plumbing from plugin → domain type is already there, just not persisted end-to-end.
- Regenerating ent after adding fields **must** use `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (`session/ent/generate.go`, also documented in `.claude/rules/ent-schema-generation.md`) — omitting `--feature sql/upsert` silently breaks `UpsertRule`-style methods.

## 6. Settings UI location (for cross-reference, not deep-dived here)

Per-source toggles belong in `web-app/src/components/settings/BacklogSourcesSettings.tsx` (+ `backlogSourceSchemas.ts` for the schema-driven form, + `useBacklogSourcesService.ts` hook) — confirmed present at `web-app/src/app/settings/backlog-sources/`. Not explored in depth here; a UI-focused research pass should look at how `enabled` (the one existing per-source bool) is rendered/wired as the template for new sync-direction toggles.

## Key files (absolute paths)

- `/home/tstapler/Programming/stapler-squad/session/backlog_plugin_github.go` — Issues plugin Fetch/MapToBacklogItem, `githubAPIURL`, `githubAPIBaseURL`
- `/home/tstapler/Programming/stapler-squad/session/backlog_plugin_github_prs.go` — PRs plugin Fetch, `computeLabels`/`fetchCILabel` (label-derivation pattern)
- `/home/tstapler/Programming/stapler-squad/session/backlog_plugin.go` — `ExternalItem`, `PluginRegistry`, `ItemSourcePlugin` interface
- `/home/tstapler/Programming/stapler-squad/github/client.go` — `ClosePR`/`MergePR`/`PostPRComment` (mutation pattern to mirror), `GetPRForBranchConditional` (ETag pattern)
- `/home/tstapler/Programming/stapler-squad/github/http_client.go` — `getGHToken` (env→keychain precedence), `newGHRequestForHostWithToken`
- `/home/tstapler/Programming/stapler-squad/github/rate_limit.go` — `RateLimiter`/`DefaultRateLimiter` (header-parsing logic worth reusing)
- `/home/tstapler/Programming/stapler-squad/github/hosts.go` — `RestBaseURLForHost`, GHE host handling (shared by both stacks)
- `/home/tstapler/Programming/stapler-squad/session/ent/schema/backlog_item.go` — `BacklogItem` fields (no `external_url`/`labels` yet); `pr_feedback_addressed_at` watermark pattern to mirror for loop-prevention
- `/home/tstapler/Programming/stapler-squad/session/ent/schema/item_source.go` — `ItemSource` fields (`config`, `enabled`, `sync_cursor`, `last_synced_at`)
- `/home/tstapler/Programming/stapler-squad/session/backlog_lifecycle.go:4090-4201` — `ReconcilePRPending`, the concrete watermark-based dedup implementation to model loop-prevention on
- `/home/tstapler/Programming/stapler-squad/session/repository.go:346-560` — `BacklogItemData`, `BacklogItemUpdate` (partial-update + explicit `Clear*` convention)
- `/home/tstapler/Programming/stapler-squad/session/ent/generate.go` — canonical `ent generate` command (`--feature sql/upsert` required)
- `/home/tstapler/Programming/stapler-squad/web-app/src/components/settings/BacklogSourcesSettings.tsx` — Settings > Backlog Sources UI (per-source `enabled` toggle template)
