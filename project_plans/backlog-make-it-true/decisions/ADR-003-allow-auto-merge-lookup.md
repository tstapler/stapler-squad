# ADR-003: Repo `allow_auto_merge` lookup lives in package `github`, tri-state with reason, in-memory cache

**Status**: Accepted (planning; one UNVERIFIED item pending Task 3.1.1c)
**Date**: 2026-10-17
**Project**: backlog-make-it-true, workstream C

## Context

`allow_auto_merge` (`backlog.proto:2144`, `optional bool`) is rendered by `StuckItemDetail.tsx:54-59` but never set (`backlog_service_stuck.go:140-143`). `NewConditionalRequest*` are github.com-only (`github/http_client.go:343,368`); the host-aware `newGHRequestForHostWithToken` is unexported (`:276`). `ETagCache` is keyed `host/owner/repo/prNumber` and stores `*PRInfo` (`github/etag_cache.go:33-58`). Repo-level `allow_auto_merge=true` is necessary, not sufficient (branch protection, squash enabled). A code comment (`session/backlog_lifecycle.go:1925-1935`) says this very repo has the setting disabled, so a real lookup will say "not allowed" for every stapler-squad item.

## Decision

- **Fetch** in new `github/repo_settings.go`: `RepoSettings(ctx, ref RepoRef, opts)` using `newGHRequestForHostWithToken` against `GET /repos/{owner}/{repo}` (host and account explicit, `getGHTokenForAccount`). Package-internal so no raw `http.NewRequest` and no export of the unexported builder. Parse `allow_auto_merge` and `allow_squash_merge`. Tag the call origin deliberately (`interactive` for a user-triggered refresh, background otherwise); a drop under admission control maps to `rate_limited`, never `false`.
- **Cache**: own repo-settings cache (the PR-keyed ETag cache cannot hold it), keyed `(host, owner, repo)`, holding the parsed value plus ETag so a 304 returns the cached value (never the zero value). In-memory TTL (short, default 10 min, INFERRED; a conditional 304 is free so a short TTL is cheap), `singleflight` per key, **negative results from transient 403/429/5xx are never cached as `false`** (cached as `backoff` for a short back-off only (an internal cache label, never shown to users)). Not persisted on the item: no ent schema change.
- **Tri-state with reason**: add proto enum `AutoMergeLookupStatus { UNSPECIFIED(not applicable/not asked), OK, RATE_LIMITED, NO_PERMISSION, NOT_FOUND, ERROR }` plus `google.protobuf.Timestamp auto_merge_checked_at` on the stuck item; keep `optional bool allow_auto_merge` meaningful only when status is OK: the Go `RepoSettings` keeps the bool unexported behind `AutoMerge() (allowed, ok bool)` (`ok` only when `Status==OK`), and the service layer sets the proto field only for `OK`, with a test asserting it is unset for every other status (a `oneof` is the stronger form if proto churn is acceptable). A bare optional bool cannot distinguish "never asked" from "lookup failed" (that is today's bug).
- **Populate in the service layer** at the caller of `stuckBacklogItemToProto` in `backlog_service_stuck.go`, never inside the pure mapper. Items with no GitHub PR repo (local-only, Jules) get status UNSPECIFIED and the UI omits the line.
- **Authority stays with `EnablePRAutoMerge`**: its failure path (`backlog_lifecycle_pr.go:718`) remains the effective-status authority. The field is labeled "repo setting" in UI.
- **Pre-PR warning is read-only on the cache**, adds no synchronous GitHub call to `pushAndCreatePR`, never blocks PR creation, and is deduplicated with the existing "Auto-merge not enabled" notification (one message per PR).
- **Noise policy** (this repo returns `false`): the not-allowed state is neutral informational text with an action, not alarm styling; the pre-PR notification is emitted at most once per repo per 24h (dedupe key `automerge-off:<host>/<owner>/<repo>`), not per item. Whether to add an operator "expected off" repo list is an Unresolved Question in the plan.
- **Latency isolation (pre-mortem #7)**: every lookup carries a 2s context timeout and a per-host circuit breaker (opens after 3 consecutive failures, half-opens after 60s); an open breaker returns `ERROR` immediately; `GetBacklogStuckReport` serves the cached value or `UNSPECIFIED` and refreshes in the background rather than blocking the RPC per repo.
- **Counter** `github_repo_settings_lookup_total{outcome=success|failure|cached|not_modified, reason}`.

## Alternatives rejected

- `google/go-github` (bypasses ETag/rate-limit/lint intent for one boolean), `gh api` subprocess (no ETag, no in-process accounting), persisting on the item (staleness, schema change), exporting `newGHRequestForHostWithToken` (widens a deliberately unexported surface).

## Consequences

- Needs `make proto-gen` and `make registry-generate`.
- A `httptest` GHE override test (`SetEnterpriseBaseURLOverride`, `github/hosts.go:44-58`) pins that a GHE repo never queries github.com.
- UNVERIFIED: whether a read-only token returns the field absent (mapped to `NO_PERMISSION`); verified by a token-without-push test case in Task 3.1.1c.
