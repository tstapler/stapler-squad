# Build vs Buy: session multi-PR tracking

Scope: how to record and track several PRs per session. Evidence is from a targeted repo scan on 2026-11-03. Items marked INFERRED were not run or opened in full.

## 1. Reuse or extend existing in-repo mechanisms

| Mechanism | Evidence | Pros | Cons | Verdict |
|---|---|---|---|---|
| `session/artifacts` `SessionArtifactsBlob.PRURLs` (`session/artifacts/types.go:7`, merge/dedup in `store.go:278`, exposed as `PrUrls` in `server/adapters/instance_adapter.go:155`) | Already stores a deduped list of PR URLs per session, scanned from output | Multi-PR history list already exists in the DB (`session.go:159`); the UI adapter already surfaces it; zero schema change | URLs only: no per-PR state, role or timestamps. Scraped from terminal output, so it can include mentioned PRs that the session did not open. Not queryable across sessions (JSON blob). | Viable as a seed or backfill source for the history list; not sufficient as the model |
| Legacy `GitHubPRNumber`/`GitHubPRURL` on `Instance` (ent `github_pr_url`, `session.go:136`) | The single-PR field the task replaces | Everything already reads it, so it is the compatibility anchor | One slot, overwritten. | Keep as a dual-written "current PR" mirror during rollout (matches the requirements' risk-control section) |
| Backlog item `pr_url` / `pr_number` (`session/ent/schema/backlog_item.go:98-102`) | Also single-valued | Existing completion gate and `pr_pending` read it | Same single-PR assumption; extending it with more scalar columns does not scale | Not recommended as the home for multi-PR; becomes a derived "primary PR" mirror |
| `WorktreePRPoller` + `PRStatusPoller` (`session/worktree_pr_poller.go`, `pr_status_poller.go`) | Poll keyed by (repoPath, branch) via `GetPRForBranch`, with ETag cache and no-PR backoff | Reuse the transport, ETag and backoff code unchanged; polling is already per branch, not per session | `GetPRForBranch` returns one PRInfo per branch; no knowledge of several PRs per session | Recommended to reuse as the polling engine, extended to iterate a session's open PR rows |
| `pr_provenance.go` (PR comment stamp holding the backlog item deep link and HostID) | Round-trips PR to item from the GitHub side | Free durable PR-to-item link that survives a lost DB | Written only when the deep link has a hostname; covers item-to-PR, not session-to-PR history | Viable as a recovery/backfill signal only |
| `github_pr_url_backfill.go` | Startup backfill of a missing URL | Pattern for the legacy-field migration | Single-field logic | Viable as the template for the migration step |
| New ent entity `SessionPR` (host, owner, repo, number; state, role, timestamps; edges to session and backlog item) | Proposed in requirements "Alternatives" | Queryable, per-PR state, supports "merged/closed stop polling" and the item completion gate | Most work: schema, migration in `ent_repository_migrations.go`, proto, UI | Recommended as the system of record |

## 2. Derive PR history from GitHub without persisting

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| REST `GET /repos/{o}/{r}/pulls?head=owner:branch&state=all` (already used in `github/client.go:493,939`) | Already wired with ETag and host awareness; lists every PR ever opened from a branch, including merged/closed | One call per branch, and a head-branch query misses follow-up PRs on a different branch (stacked or rework branches); branch deletion or rename loses linkage; cost grows with every list render if not cached | Viable as a discovery/reconcile source feeding persisted rows; Not recommended as the only source |
| GraphQL `closingIssuesReferences` / issue `timelineItems` (cross-referenced, connected events) | Can find PRs tied to an issue or backlog item, which covers PRs on other branches | INFERRED from GitHub's GraphQL schema, not tested here. Needs a GraphQL path through the required request constructors; depends on issue linkage the sessions may not create; higher query cost | Viable only as an optional backfill; Not recommended as the main path |
| `gh pr list --head` subprocess | Zero code | Repo convention prefers native HTTP over `gh` subprocesses (`github/client.go:489` comment on forkExec lock contention) and `prefer-go-git-over-subshells`; bypasses ETag | Not recommended |
| Pure on-demand, no persistence | No schema change | Spends rate limit per view; loses PRs when branches are deleted; cannot hold per-PR role or the active-PR choice; fails the "session list without N+1" goal | Not recommended |

## 3. SaaS options

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| Hosted PR/issue trackers (Linear, Jira, GitHub Projects) to track PR-to-work links | Native multi-PR-per-issue support | New external dependency and credentials, which the requirements rule out ("no new credentials"); the data needed here is local session state, not project-management state; no offline use | Not recommended |
| GitHub-hosted stacked-PR tooling (e.g. Graphite) | Understands stacks | Stacked-PR tooling is explicitly out of scope; external service | Not recommended |

## 4. LLM-written bespoke code vs reuse

Reuse the HTTP, ETag, poller, backfill and ent-migration infrastructure. Hand-write only the thin new layer: the `SessionPR` entity, the active-PR rule, per-PR poll iteration, the item completion rule and the UI list. Every piece of that layer is specific to this repo's data model, so no OSS library applies; no Go library gives per-session PR history. The risk of bespoke code is a missed consumer of the legacy field, so the plan needs an audit task and a lint or grep ratchet on `GitHubPRNumber` reads.

Verdict: Recommended (build the thin layer, reuse everything below it).

## Recommendation

1. Build a `SessionPR` ent entity as the system of record. Keep the legacy fields dual-written as the "current PR" mirror behind the live-settable flag.
2. Reuse `WorktreePRPoller`/`PRStatusPoller`/ETag code; poll only open `SessionPR` rows.
3. Seed history from `SessionArtifactsBlob.PRURLs` and the existing `pulls?head=` lookup at migration/reconcile time; do not rely on GitHub derivation at read time.

## Gaps

- The consumer count of `GitHubPRNumber`/`GitHubPRURL` was not recounted (the grep for it failed on a shell glob error); do that in the audit phase.
- GraphQL timeline behavior was not tested.
- Whether `PRURLs` entries are filtered to PRs the session actually opened was not checked (`session/artifacts/store.go` scan logic).
