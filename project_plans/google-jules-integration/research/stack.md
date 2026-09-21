# Research: Technology Stack — Google Jules API Integration

Agent 1 (Stack), SDD Phase 2, `google-jules-integration`.

## 1. Jules REST API shape

Sources: [Jules API overview](https://developers.google.com/jules/api),
[Sources reference](https://jules.google/docs/api/reference/sources/),
[Sessions reference](https://developers.google.com/jules/api/reference/rest/v1alpha/sessions),
[Activities reference](https://jules.google/docs/api/reference/activities/).
All VERIFIED via WebFetch against the live docs pages on 2026-09-01.

### Base URL, versioning, auth
- Base URL: `https://jules.googleapis.com/v1alpha` — API is explicitly **v1alpha** (alpha stability; Google's own docs give no compatibility guarantee).
- Auth: API-key header, **not** `Authorization: Bearer`. Header is `x-goog-api-key: <API_KEY>`. Key is obtained per-user from `jules.google.com/settings` (matches requirements.md's constraint).
- Example: `curl -H "x-goog-api-key: $JULES_API_KEY" https://jules.googleapis.com/v1alpha/sessions`

### Sources — answers Open Question #1
- `GET /v1alpha/sources` (list), `GET /v1alpha/sources/{sourceId}` (get). **No create endpoint** — sources are read-only via the API.
- A Source represents a **GitHub repository connected through the Jules web UI / GitHub App** — "Currently, Jules supports GitHub repositories" (docs' own words). Name format: `sources/github-{owner}-{repo}`.
- **Conclusion: Jules cannot target an arbitrary local git ref/worktree.** It only works against a GitHub-hosted repo that has already been connected via Jules' GitHub App, and only branches that exist on that remote (`sourceContext.githubRepoContext.startingBranch`). This rules out any MVP flow that hands Jules a local uncommitted worktree — the branch must already be pushed to GitHub and the repo must be pre-connected through jules.google.com (one-time, human, out-of-band setup, not automatable via this API).

### Sessions
| Method | Endpoint | Purpose |
|---|---|---|
| POST | `/v1alpha/sessions` | Create a session (starts Jules work) |
| GET | `/v1alpha/sessions` | List sessions |
| GET | `/v1alpha/sessions/{session}` | Get session |
| POST | `/v1alpha/sessions/{session}:approvePlan` | Approve a generated plan |
| POST | `/v1alpha/sessions/{session}:sendMessage` | Send a message into a running session |

Create-session request body:
```json
{
  "prompt": "string, required",
  "sourceContext": {
    "source": "sources/{source}",
    "githubRepoContext": { "startingBranch": "string" }
  },
  "title": "string, optional (server-generated if omitted)",
  "requirePlanApproval": false,
  "automationMode": "AUTO_CREATE_PR"
}
```
Response includes `name`, `id`, `state` (`QUEUED`/`PLANNING`/`IN_PROGRESS`/`COMPLETED`/`FAILED`, ...), `outputs[]` (PR objects with `url`/`title`/`description`), `createTime`/`updateTime` (RFC 3339), and a `url` back to the session in the Jules web app.

`automationMode: AUTO_CREATE_PR` plus polling `state`/`outputs[]` is the natural fit for the requirements' "fire-and-forget create + poll" MVP scope — no need to touch `sendMessage`/`approvePlan` for v1.

### Activities — answers Open Question re: MVP relevance
- `GET /v1alpha/sessions/{sessionId}/activities` (list, paginated: `pageSize` 1–100 default 50, `pageToken`) and `GET /v1alpha/sessions/{sessionId}/activities/{activityId}` (get). Polling only — no documented streaming/webhook/SSE endpoint.
- Activity types: plan generated, plan approved, user messaged, agent messaged, progress updated, session completed, session failed. Can carry artifacts (git patches, command output, base64 media).
- **For MVP, the Activities API is not required.** `Session.state` + `Session.outputs[]` (from the Sessions GET) already gives enough signal to drive backlog status transitions and to hand the PR off to `WorktreePRPoller`. Activities would only matter for a richer in-app progress feed or for mid-session steering (`sendMessage`) — both explicitly out of scope per requirements.md ("Out of Scope: real-time interactive steering").

### Rate limits
- Docs mention a `429` response for "too many requests" but do not publish concrete thresholds, headers, or backoff guidance. Treat as unknown/opaque — any Jules client needs its own conservative client-side throttling/backoff (see `github/http_client.go`'s `rateLimitTransport` for the existing in-repo pattern to mirror) rather than relying on published limits.

## 2. Existing Go HTTP client patterns in this repo

Two live patterns for calling external REST APIs with token/key auth exist; a Jules client should follow the **second** one (`github/http_client.go` / `session/backlog_plugin_github.go`), not the first:

1. **`gh` CLI subprocess wrapper** — most of [`github/client.go`](github/client.go) (936 lines) shells out to the `gh` CLI (`safeexec.CommandContext(ctx, "gh", "pr", "view", ...)`) for PR/issue operations. Not applicable — Jules has no CLI to shell out to.
2. **Direct `net/http` + API-key/token header** — the actual pattern to mirror:
   - [`github/http_client.go`](github/http_client.go) (205 lines): package-level shared `*http.Client` (`ghHTTPClient`, 30s `Timeout`), a custom `http.RoundTripper` (`rateLimitTransport`) wrapping the client's `Transport` to detect/handle rate-limit responses, a `newGHRequest(ctx, path)` helper that builds the request and sets the auth header, and `classifyGHResponse(resp, notFoundMsg, sentinels)` to turn HTTP status codes into typed Go errors.
   - [`session/backlog_plugin_github.go`](session/backlog_plugin_github.go): per-call `http.NewRequestWithContext(ctx, method, url, body)`, `req.Header.Set("Authorization", "token "+cfg.Token)` (GitHub's older token scheme — a Jules client would instead do `req.Header.Set("x-goog-api-key", cfg.APIKey)` per the auth section above), `json.Marshal` for request bodies, `json.NewDecoder(resp.Body).Decode(&target)` for responses. Config (including the credential) is decoded from an opaque `PluginConfig{Raw string}` JSON blob at call time rather than held as client state.

Grep evidence for the header pattern actually in use today (not `gh` CLI):
```
github/user_pr_cache.go:614:      req.Header.Set("Authorization", "Bearer "+token)
github/http_client.go:137:        req.Header.Set("Authorization", "Bearer "+token)
server/services/github_webhook_pr_fix.go:392: req.Header.Set("Authorization", "Bearer "+token)
session/backlog_plugin_github.go:184,327,393: req.Header.Set("Authorization", "token "+cfg.Token)
session/backlog_plugin_github_prs.go:93,172:  req.Header.Set("Authorization", "token "+cfg.Token)
```

Also relevant: [`session/backlog_plugin.go`](session/backlog_plugin.go) defines the `ItemSourcePlugin` interface (`PluginID()`, `Fetch(ctx, config, cursor) ([]ExternalItem, string, error)`, `MapToBacklogItem(item, sourceID) BacklogItemData`) that alternative (b) from requirements.md ("thin: PR-import only") would reuse as-is — no new HTTP client needed for that path, since `GitHubPRsPlugin`/`WorktreePRPoller` already ingest PRs by polling GitHub directly regardless of who opened them.

## 3. Package/module layout convention

Confirmed via `find . -name 'client.go'` → only two hits: `./github/client.go` and `./session/headless/client.go`. Top-level package listing (`find . -maxdepth 1 -type d`) shows external-integration packages live as **their own top-level directory at repo root**, sibling to `session/`, `server/`, not nested under a `pkg/` or `internal/` umbrella:
```
./github/     — GitHub REST + gh CLI wrapper (936-line client.go + http_client.go, user_pr_cache.go, etc.)
./pkg/        — exists but holds unrelated internal libs: analytics/, ansi/, classifier/, events/, warren/
```
`pkg/` is not the convention for an external API client — `github/` (a peer of `session/`, `server/`) is. **A new Jules client belongs in a new top-level `jules/` package** (e.g. `jules/client.go`), mirroring `github/`'s placement, isolating the alpha-API churn risk into one importable package exactly as requirements.md's constraint calls for ("Any adapter must be isolated (single package)").

Module: `github.com/tstapler/stapler-squad`, Go **1.26.4** (`go.mod` line 3).

## 4. Go idioms / dependencies for a thin REST client

- `go.mod` has **no** third-party HTTP client library (no resty, sling, retryablehttp, go-retryablehttp, oauth2 package beyond otel instrumentation). Only HTTP-adjacent deps are `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` (tracing middleware) and the transitive `github.com/felixge/httpsnoop`.
- **Convention is stdlib `net/http` directly**, wrapped in a small package-local client with a custom `http.RoundTripper` for cross-cutting concerns (rate-limit detection/retry) — exactly `github/http_client.go`'s `rateLimitTransport` pattern. A Jules client should follow suit: package-level `*http.Client` with sane `Timeout`, `http.NewRequestWithContext` per call, `encoding/json` for (de)serialization, typed error classification from HTTP status (mirroring `classifyGHResponse`), and no new HTTP library dependency.
- If OTel tracing of outbound calls is desired for consistency with the rest of the codebase, wrap the client's `Transport` with `otelhttp.NewTransport(...)` — same mechanism already available via the existing `go.mod` dependency, no new import needed.

## Recommendation feeding into architecture (Agent 2/3 territory, noted here for continuity)
- New package: `jules/` at repo root, `package jules`, single-package isolation boundary satisfying the "alpha API churn shouldn't ripple into session/" constraint.
- Client: stdlib `net/http` + `x-goog-api-key` header, mirroring `github/http_client.go`'s shared-client + custom-transport shape, not `session/backlog_plugin_github.go`'s per-call token-header shape (that one's fine too, but the transport-level rate-limit handling in `http_client.go` is worth the reuse given Jules' 429 behavior is unpublished/unknown).
- Because Sources are GitHub-repo-only and read-only via the API, and `startingBranch` must already exist on GitHub, an MVP Jules-backed backlog item requires the item's branch to be pushed to GitHub before the Jules session is created — this is a hard external constraint, not a design choice, and needs to be reflected in Agent 3/4's data-model and flow research.
