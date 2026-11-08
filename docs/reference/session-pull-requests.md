# Session pull requests

How stapler-squad tracks more than one pull request (PR) per session. For rollout, see [Roll out session multi-PR tracking](../how-to/roll-out-session-multi-pr.md).

Decisions: [ADR-001](../../project_plans/session-multi-pr-tracking/decisions/ADR-001-session-pr-entity-and-identity.md) (entities and identity), [ADR-002](../../project_plans/session-multi-pr-tracking/decisions/ADR-002-active-pr-derivation-and-legacy-mirror.md) (active PR and legacy mirror), [ADR-003](../../project_plans/session-multi-pr-tracking/decisions/ADR-003-backlog-multi-pr-done-rule-and-additive-report.md) (backlog done rule), [ADR-004](../../project_plans/session-multi-pr-tracking/decisions/ADR-004-artifacts-separate-from-sessionpr-and-search-index.md) (artifacts).

## Entities

Hand-written ent schemas in `session/ent/schema/`. Generated ent code is not committed.

| Entity | File | Purpose |
|---|---|---|
| `PullRequestState` | `pull_request_state.go` | One row per PR: state, draft flag, check conclusion, review counts, timestamps, poll health. Unique on `(host, owner, repo, number)`. Only the PR status poller writes it. |
| `SessionPR` | `session_pr.go` | Link between a PR state and one owner: a session (`session_uuid`) or a backlog item (`backlog_item_id`). Holds `role`, `pinned`, `attach_source`, `attached_at`. |
| `SessionPRTombstone` | `session_pr_tombstone.go` | Records a detach for an owner and PR, written in the same transaction as the link delete. Stops backfill, discovery, scan and Jules from recreating the link. |

- **Identity.** A PR is `PRRef{host, owner, repo, number}`, never the number alone. `host`, `owner` and `repo` are stored lowercase; `url` keeps its original case.
- **State** is one of `open`, `closed`, `merged`, `unknown`. `unknown` is not terminal.
- **Role** is one of `primary`, `followup`, `stacked`, `unknown`.
- **Attach source** is one of `created_from_url`, `push_scan`, `branch_discovery`, `pr_creation_service`, `report_pr_created`, `jules`, `backfill`, `manual`.
- **Poll error** is one of `none`, `auth`, `rate_limited`, `not_found`, `repo_inaccessible`, `network`.
- **Unverified terminal.** A terminal row whose outcome was not confirmed is treated as non-terminal `unknown` until its verification is exhausted (5 counted failures over 30 minutes). The API reports it as `outcome_unverified`.
- **Item PR set** is derived at read time, never stored: links carrying the item's id, plus links of the item's current round, minus PRs tombstoned for the item.

## Active PR

`ActivePR` is a pure function. In order, it picks:

1. the pinned PR;
2. the open PR whose head branch is the session's current branch;
3. the newest `open` PR;
4. the newest non-terminal `unknown` PR;
5. the newest effectively terminal PR;
6. none.

Only a user RPC pins; automation never does.

## Legacy mirror

The legacy fields (`github_pr_*` columns and `Instance.GitHubPR*`) are always a projection of the active PR once a session has any link. They are written from one place (`Instance.AttachPR`, `session/instance_pr_set.go`) and do not depend on any flag or scope. A reconciler corrects drift through the actor and counts it in `ssq_session_pr_mirror_drift_total{kind}` (`corrected`, `unrepresented_skipped`).

## Flags and scopes

Live-settable through the feature-flag RPC. Defined in `config/session_multi_pr_flag.go`. Everything defaults to off.

| Flag | Scope | Default | Effect |
|---|---|---|---|
| `session_multi_pr` | `reader:poller` | off | Poller and prwatch read per-PR entries |
| `session_multi_pr` | `reader:backlog` | off | Backlog done rule, PR holds, stuck report |
| `session_multi_pr` | `reader:actions` | off | No server reader consumes it at this commit; it only sits in the enable order |
| `session_multi_pr` | `reader:ui` | off | Web UI set views |
| `session_multi_pr` | `refresher_off` | off | Kill switch for the state refresher. Off means the refresher runs. |
| `artifact_scan_v2` | (global only) | off | Artifact rescan with typed records |

A reader scope is effective only when its own value and every earlier reader's value (poller, backlog, actions, ui) are on. The order is applied at read time. The refresher, the mirror and the retention sweeper and cleanup gate (which fail closed on a non-terminal or `unknown` PR) are not gated by the flag.

## RPCs

In `proto/session/v1/session.proto`. The first group is UI-only and never reachable from MCP tools.

| RPC | Purpose |
|---|---|
| `PinSessionPullRequest` | Pin one attached PR as the active PR |
| `RefreshSessionPullRequests` | Poll a session's PRs now |
| `DetachSessionPullRequest` | Remove a link and write a tombstone |
| `AttachSessionPullRequest` | Attach a PR by URL |
| `AskSessionToFixPullRequest` | Type a server-built fix request into the session pane |
| `DetachItemPullRequest` | Release an item-direct PR hold |
| `ListSessionArtifacts` | Browse typed artifact records (the `/artifacts` page) |
| `SearchSessionArtifacts` | Find sessions by what they produced |
| `FindSessionsByPullRequest` | Sessions tied to one PR: tracked first, then seen in a transcript |
| `RescanSessionArtifacts` | Reset the scan position and queue a rescan |

## Proto fields

In `proto/session/v1/types.proto`, `Session` carries `repeated SessionPullRequest pull_requests = 102` and `repeated PrHold pr_holds = 101` (detail responses only). The `is_active` entry matches the legacy scalars. The backlog item carries `pull_requests = 47` and `pr_holds = 48` in `backlog.proto`.

## Metrics

| Metric | Source |
|---|---|
| `ssq_pr_poll_total{state}` | `session/pr_poll_metrics.go` |
| `ssq_pr_poll_failures_total{reason}` | same |
| `ssq_session_pr_count` | same |
| `ssq_pr_discovery_list_calls_total` | same |
| `ssq_pr_unverified_terminal_rows` | same |
| `ssq_session_pr_mirror_drift_total{kind}` | `session/session_pr_mirror_reconciler.go` |
| `ssq_cleanup_gate_blocked_total{path}`, `ssq_cleanup_gate_blocked_items{path}` | `session/pr_cleanup_gate_metrics.go` |
| `ssq_pr_live_work_hold_total{result}` | `session/pr_live_work_metrics.go` |
| `ssq_artifact_blob_bytes`, `ssq_artifact_browse_total`, `ssq_artifact_index_docs`, `ssq_artifact_search_queries_total`, `ssq_artifact_search_seconds` | `session/artifacts`, `session/search`, `server/services` |
| `ssq_pr_maintenance_failed_total` | In-process only (`Storage.PRMaintenanceFailures()`); not exported at this commit |

## Guards

- `make pr-field-guard`: no direct writes to the PR fields outside `session/instance*.go`.
- `make pr-state-write-guard`: no direct `PullRequestState` writes outside the repository and the garbage collector.
- Allow-list tests for readers of the legacy fields.
