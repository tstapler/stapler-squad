# Roll out session multi-PR tracking

Deploy the per-session PR set, then switch its readers on one at a time. The model is described in [Session pull requests](../reference/session-pull-requests.md); the decisions are in [ADR-001](../../project_plans/session-multi-pr-tracking/decisions/ADR-001-session-pr-entity-and-identity.md) to [ADR-004](../../project_plans/session-multi-pr-tracking/decisions/ADR-004-artifacts-separate-from-sessionpr-and-search-index.md).

Every flag and scope defaults to off. The state refresher and the legacy-field mirror are not behind the flag: they run from the first deploy.

## Before you start

- You can change feature flags in the UI flag panel or through the feature-flag RPC. Never use an environment variable.
- Metrics are in the `session-pr-tracking` Grafana dashboard (`docs/observability/grafana/dashboards/session-pr-tracking.json`).

## Step -1: back up the database

The boot rehearsal test (`session/session_pr_rehearsal_test.go`, opt-in through `SSQ_REHEARSAL_DB`) does not boot the service, so the real wiring is first exercised by your deploy. Take a consistent copy first:

```bash
mkdir -p ~/.stapler-squad/manual-builds/rehearsal
sqlite3 <live sessions.db> ".backup ~/.stapler-squad/manual-builds/rehearsal/sessions.db"
```

- Use `.backup`, never `cp`: the database runs in WAL mode.
- Keep the copy on disk, not in `/tmp` (a RAM-backed tmpfs on this host).
- Never boot a second instance on the copy. Its rows point at the live worktree paths, so the sweeper and cleanup tickers would act on them.

## Step 0: deploy with every scope off

1. Run `make install-service`. Leave `session_multi_pr` and all `reader:*` scopes off.
2. Watch for 10 minutes. The state refresher is already running, so the poll metrics move even with the flag off.

| Signal | Expect |
|---|---|
| Log line `session pr startup maintenance failed; retrying in the background` | Absent. If present, the 5-minute retry ticker is running; check for `session pr maintenance retry succeeded; stopping the retry ticker`. |
| `ssq_pr_poll_total` | Rising. Total PR requests stay at or under 1,500 per hour (`increase(ssq_pr_poll_total[1h])`). |
| `ssq_pr_maintenance_failed_total` | Not exported as an OpenTelemetry instrument at this commit. `Storage.PRMaintenanceFailures()` counts it in-process, so read the maintenance log lines above instead. |
| `ssq_pr_unverified_terminal_rows` | Falls as terminal rows are re-verified. |

3. Compare the `[Migration] session_pr backfill:` log line with your rehearsal record. It lists `created`, `imported_unrepresented`, `skipped`, `skipped_no_uuid`, `skipped_no_repo`, `skipped_invalid`, `skipped_item_bad_ref`, `skipped_item_invalid`, `skipped_tombstoned` and `deferred`, plus 10 titles of skipped rows.
4. Expect the cold-poll drain to take `ceil(N / 20)` poller ticks for N rows with state `unknown`. For 300 rows that is 15 ticks.

If the request rate exceeds the budget, set the `refresher_off` scope (see [Roll back](#roll-back)).

## Enable the readers

Enable the scopes of `session_multi_pr` in this order. A reader is effective only when every earlier reader is, and the server checks that when it reads the flag. An out-of-order enable is stored but resolves to off, so verify each stage before moving on.

| Stage | Scope | What turns on | Check before the next stage |
|---|---|---|---|
| 1 | `reader:poller` | Poller and prwatch use one entry per open PR | `ssq_pr_poll_total` per hour still within budget; `ssq_pr_poll_failures_total{reason}` quiet |
| 2 | `reader:backlog` | Backlog done rule over the item's PR set, PR holds, stuck-report PR lines | `ssq_cleanup_gate_blocked_items{path}` explained; `ssq_pr_live_work_hold_total{result}` as expected |
| 3 | `reader:actions` | Nothing at this commit (no server reader uses it); keep it in the order | `ssq_session_pr_mirror_drift_total` is 0 |
| 4 | `reader:ui` | PR chips and set UI in the web app | Spot-check a session with two PRs |
| 5 | `artifact_scan_v2` (global only) | Artifact rescan with typed records | See below |

Read `ssq_pr_unverified_terminal_rows` and the skipped-backfill counts before each stage.

### Artifact scan v2

Run a dry run on a database copy first. Watch `ssq_artifact_blob_bytes` (blobs are capped at 128 KB) before you enable `artifact_scan_v2` globally. Turning it off stops further rescans; stored records are kept.

## Operational notes

- A PR that is closed and then reopened on GitHub is noticed only if the `pull_request.reopened` webhook is enabled ([expose the webhook](expose-github-webhook-endpoint.md)). Otherwise its row stays terminal and cleanup can run under the reopened PR.
- Every restart zeroes verification counters. With frequent restarts an unfetchable historical row may never reach 5 counted failures in 30 minutes; its worktree is retained, which is the safe direction.
- Private-repo, SSO, expired-token and secondary-rate-limit failures do not count toward exhaustion, so those rows are held until you detach them.
- Skipped backfill rows keep their worktrees until you set a valid `pr_url` or attach the PR by URL.
- An abandoned open PR on a superseded round retains the shared worktree path. It is visible only through `ssq_cleanup_gate_blocked_items` and the stuck report. Detach it to release the worktree.
- After the maximum hold (1 hour after the first merge) a live session keeps its pane and worktree. End it with **End session**.
- The orphan garbage collector removes only owner-less links older than 24 hours.

## Roll back

Rollback is a flag change; no database restore is needed.

1. Turn the `reader:*` scopes off, or turn `session_multi_pr` off. Readers return to the legacy single-PR fields at once.
2. If poll cost is the problem, set the `refresher_off` scope to true. This stops only the state refresher (default: it runs). The global flag and the reader scopes never change it.
3. Turn `artifact_scan_v2` off to stop rescans.

Turning the flags off does not remove the legacy mirror. The `github_pr_*` fields stay a projection of the active PR, written from one place whatever the flags say, so older code keeps seeing a correct single PR. The new tables (`PullRequestState`, `SessionPR`, `SessionPRTombstone`) stay in place.

To go back to a previous binary and forward again, redeploy: the boot backfill imports any legacy number that no link represents, before sessions load, so the projection never overwrites it. Check the `[Migration]` counts again after the second deploy.
