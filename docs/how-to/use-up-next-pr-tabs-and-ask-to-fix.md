# Use the Up Next PR tab and "Ask to fix"

Up Next (`/unfinished`) has four tabs: PRs, Stuck, Worktrees, Queue. The last tab you picked is remembered in `localStorage`; `?tab=<name>` selects one for a link, and `?item=<id>` (stuck-item deep links) always opens Stuck.

## What a PR card shows

Each open PR shows failing checks, unresolved review threads, merge conflicts, and every local session linked to it. A count that GitHub did not return shows "?", never 0.

## Ask a session to fix a PR

The "Ask <session> to fix" button appears only when the PR has something to fix (failing checks, unresolved threads, or a merge conflict). It sends the linked session a prompt built on the server from fresh PR state: the PR link plus check names, thread paths and authors. Comment bodies are never included, and no slash command is appended.

- If no linked session is idle and ready (`steer_ready`), "Open session" is the primary button and the ask button is disabled with a reason.
- A request already sent for the same PR and reasons within a minute is reported as already requested, not sent twice.

## Degraded poll mode

Set `STAPLER_SQUAD_PR_POLL_DEGRADED=1` (or `true`) before starting to drop `reviewThreads` from the PR poll. Thread counts then show "?" and the PRs tab badge reads "3+" (it counts failing CI, merge conflicts and changes-requested only). The default full poll measured 4 GraphQL points per poll on github.com; there is no automatic switch. See `project_plans/up-next-tabs-pr-nudge/decisions/ADR-001-pr-detail-data-sourcing.md`.

## Read the measurement logs

Search `~/.stapler-squad/logs/staplersquad.log` (see `debug-with-logs.md`) for:

| Log message | Meaning |
|---|---|
| `nudge_outcome` | One line per ask: outcome, reasons, and `session_live` (the delivery-rate denominator) |
| `nudge_followup` | What happened to the PR after a delivered request |
| `up_next_funnel` | Per snapshot: `prs_needing_attention`, `with_linked_session`, `with_live_session` |

Ready-made queries are in `project_plans/up-next-tabs-pr-nudge/decisions/ADR-002-nudge-session-for-pr-rpc.md`.
