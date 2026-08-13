# Stack Research: Reduce PR/Issue Comment Noise, Prefer Check Runs

## 1. GitHub API client library

**No SDK is used.** `go.mod` has no `google/go-github`, `go-gh`, `shurcooL/githubv4`, or any
other GitHub client library dependency (verified: `grep -n github go.mod` and `grep go-github
go.sum` both return nothing beyond this repo's own module path). All GitHub API access is
hand-rolled `net/http` calls against the REST/GraphQL endpoints directly, centralized in the
`github/` package:

- `github/http_client.go` — `ghHTTPClient` (shared `*http.Client`, 30s timeout), `newGHRequest`
  (authenticated GET), `newGHPostRequest` (POST with JSON body, used for GraphQL — see
  [ADR-020](file:///home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-2375ca2e-2155-4165-a38a-214f1fd80e39_18cb1adb7dc0c73f/docs/adr/020-graphql-for-user-pr-list.md)).
- `github/client.go` — higher-level functions (`CheckGHAuth`, `MergePR`, etc.) built on top of
  the raw HTTP helpers.
- `session/backlog_plugin_github_prs.go:47-50,168-200` — already parses Check Runs API responses
  (`GET /repos/{owner}/{repo}/commits/{sha}/check-runs`) into a local `githubCheckRun{Conclusion
  string}` struct to derive a `pr:ci-failing` label. This is **read-only** — no code path calls
  `POST /repos/{owner}/{repo}/check-runs` (create) or `PATCH
  /repos/{owner}/{repo}/check-runs/{id}` (update) anywhere in the repo today. Confirmed via
  `grep -rn "Checks.Create\|check-runs" --include=*.go` — the only hits are the read path above.

**Implication for this feature**: there is no `CheckRunsService.CreateCheckRun`-style typed
client to call (as there would be with `google/go-github`). Creating/updating check runs means
either (a) adding hand-rolled POST/PATCH calls to `github/http_client.go` following the exact
pattern `newGHPostRequest` already established for GraphQL, or (b) introducing `google/go-github`
as a new dependency. Given [ADR-020](file:///home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-2375ca2e-2155-4165-a38a-214f1fd80e39_18cb1adb7dc0c73f/docs/adr/020-graphql-for-user-pr-list.md)'s explicit stance ("Features must use direct GitHub
HTTP API calls... not `gh` subprocess calls") and the repo's consistent avoidance of a full SDK
dependency, (a) matches existing convention. `POST .../check-runs` is a plain REST call (unlike
GraphQL), so it needs only a `newGHPostRequest`-shaped helper, not a new abstraction.

## 2. Auth mechanism

Token-based (PAT-style), **not** a GitHub App:

- `github/http_client.go:36-52` (`getGHToken`) — precedence: `GITHUB_TOKEN` env → `GH_TOKEN` env
  → OS keychain (`GetKeychainToken()`, cached 1 minute). No installation-token / JWT / GitHub App
  flow exists anywhere in the codebase (`grep -rn "installation.*token\|jwt\|x-access-token"
  github/*.go` — no hits).
- `github/keychain.go` stores per-host, per-account tokens (`GetKeychainTokenForAccount`,
  `SetKeychainTokenForAccount`) — this is a personal-access-token vault, not App credentials.
- **One exception**: `github/client.go:624-643` (`PostPRComment`, called by
  `session/pr_tracking.go:82` `Instance.PostComment`) still shells out to the `gh` CLI
  (`safeexec.CommandContext(ctx, "gh", "pr", "comment", ...)`) rather than using the direct-HTTP
  pattern the rest of the package (and ADR-020) mandates. This is the actual comment-posting path
  today, and it depends on whatever scopes the ambient `gh auth` session has, not the
  `GITHUB_TOKEN`/keychain token used by `getGHToken`.

**Scope/permission coverage for Checks API**: the Checks API (`checks:write`) requires either
a fine-grained PAT with the "Checks" repository permission, or a classic PAT (`repo` scope
covers checks on repos the token can access), or a GitHub App with the `checks: write`
permission. Nothing in this repo's docs, `bootstrap/roles/github/`, or `.github/workflows/*.yml`
declares or provisions a specific token scope for the app's own outbound calls — token
provisioning is manual (`gh auth login` / stored via `SetKeychainToken*`), so whether `checks:write`
is covered depends on how the user's personal token was scoped when they ran `gh auth login`,
not on anything this codebase controls. **This is a gap the plan must call out**: before
building check-run creation, confirm (or document) that the token source(s) in play
(`GITHUB_TOKEN`/`GH_TOKEN` env, or keychain PAT) actually carry `checks:write`/`repo` scope —
there's no existing scope-verification code path to reuse (`CheckGHAuth` only verifies the
token authenticates, via `GET user`; it does not verify granted scopes).

`.github/workflows/*.yml` `permissions:` blocks (`build.yml`, `release.yml`, etc.) are CI-job
`GITHUB_TOKEN` scoping for GitHub Actions runs, and are unrelated to the app's own runtime PAT —
those two token universes don't overlap.

## 3. Client library version / upgrade

Not applicable — there is no pinned GitHub client library to upgrade (see §1). If the team
decides to adopt `google/go-github` specifically for its `CheckRunsService` (which does support
`CreateCheckRun`/`UpdateCheckRun` with a clean typed API), that would be a **new** dependency
addition, not a version bump, and represents a bigger architectural shift than following the
existing hand-rolled-HTTP convention. Given ADR-020's stated preference and the small surface
area needed (one POST + occasional PATCH), extending `github/http_client.go` is lower-risk and
more consistent than introducing an SDK.

## 4. Existing patterns to reuse

- **Read pattern for check runs** (`session/backlog_plugin_github_prs.go:168-200`,
  `fetchCILabel`) — shows the request shape (`GET
  /repos/{owner}/{repo}/commits/{sha}/check-runs`, `Authorization: token <PAT>`, `Accept:
  application/vnd.github.v3+json`) and response struct (`githubCheckRun{Conclusion string}`).
  A create/update helper should mirror this: same header set, same `RestBaseURLForHost`-style
  host resolution, same best-effort/silent-failure posture is *not* appropriate for a
  user-facing status signal (unlike the label-only helper, a failed check-run write should
  probably surface, since it's meant to replace a comment as the authoritative status channel).
- **POST-with-body pattern**: `newGHPostRequest` in `github/http_client.go` (added for GraphQL
  per ADR-020) is the template for a new `POST /check-runs` helper — same auth/timeout/JSON-body
  conventions, different endpoint and non-GraphQL response shape (no top-level `errors` array to
  check).
- **Generic comment-posting primitives** (per requirements doc, already out-of-scope to remove):
  `server/services/github_service.go:137` `GitHubService.PostPRComment` (RPC handler) →
  `session/pr_tracking.go:71` `Instance.PostComment` → `github/client.go:624`
  `github.PostPRComment` (the `gh` CLI shellout). Any new "prefer check run" convention sits
  *alongside* this call chain, gating whether a session/skill invokes it at all — it does not
  require modifying the primitive itself.
- **No existing "structured PR state" abstraction** beyond the read-only `githubCheckRun`
  parsing and `github/priority.go`'s `DerivePRPriority` (pure derivation from PR/CI booleans, no
  API writes — see `docs/adr/ADR-026-mergeability-state-synthesis.md`). Nothing today creates or
  updates GitHub-side state; this feature would be the first check-run *write* path in the repo.
