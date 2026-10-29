# Steer sessions from PR activity (inbox PR watch)

Stapler Squad can watch the pull requests that your ad-hoc sessions opened and nudge the session when something needs attention: a new review, a new comment, an inline comment or a failing CI run. Activity arrives through the gh-signal inbox, which receives GitHub webhooks. This guide covers setup, verification, the settings you can change, what each notification means and how to clean up.

Plain terms used here are defined in the glossary at the end. Where a setting or a reason code is shown in backticks, that is the exact text you will see in the config file, the logs or the JSON status.

## Prerequisites

Work through this checklist once per machine and once per repository.

1. gh-signal is installed and the `gh-signal-inbox` service is running.
2. The inbox token file exists and has mode 0600 (`chmod 600 ~/.local/state/gh-signal/inbox.token`). Squad refuses a token file that other users can read.
3. `gh auth status` succeeds. Squad uses it to learn your GitHub login, so it can ignore your own comments.
4. Every repository you want watched has a webhook that reaches gh-signal. Create one with `gh-signal inbox watch <owner>/<repo> <pr-number> --consumer squad:<hostname>`. Squad asks for the same thing when it opens a PR, but it cannot create a webhook for a repository where gh-signal lacks admin access.
5. The feature flag is on (next section).

Only github.com repositories are supported. Sessions on other GitHub hosts, on forks, and sessions whose backlog item is waiting on the PR (the backlog path handles those) are not watched.

## Turn the feature on and off

The switch is the feature flag `inbox_pr_watch`. Set it in your Squad config, `~/.stapler-squad/config.json`:

```json
{
  "feature_flags": { "inbox_pr_watch": true }
}
```

Squad reads the flag live on every tick, so turning it on or off needs no restart. Turning it off stops polling and steering. It does not remove any subscription: the subscriptions stay in the inbox and are harmless, and items keep accumulating there (see "While a session is paused" and "When you turn it back on"). To remove subscriptions, follow "Clean up subscriptions".

## Configuration keys and defaults

All other settings live under `inbox_pr_watch` in the same config file. A zero, negative or invalid value falls back to the default, with two exceptions: `min_steer_interval_seconds` set to 0 is honored (it turns batching off), and an invalid `human_feedback_mode` is treated as `off`. Every correction is logged once as a warning at load and listed under "Settings" in the status page.

| Key | Default | Notes |
|---|---|---|
| `url` | `http://127.0.0.1:8789` | Must be a loopback address (127.0.0.1, ::1 or localhost). |
| `token_path` | `~/.local/state/gh-signal/inbox.token` | File mode must be 0600. |
| `poll_interval_seconds` | `5` | Minimum 1. |
| `reconcile_interval_seconds` | `60` | How often Squad compares its subscriptions with the inbox. Minimum 10. |
| `coverage_recheck_minutes` | `10` | How often Squad retries a repository that has no webhook coverage. Minimum 1. |
| `min_steer_interval_seconds` | `300` | The batching window. 0 turns batching off. Capped at 1800. |
| `held_notice_after_minutes` | `60` | When a waiting event raises a notification. |
| `held_age_ceiling_hours` | `24` | A waiting event older than this is dropped. |
| `state_notify_grace_seconds` | `120` | How long "not reachable" or "idle in this process" must last before a notification. |
| `leader_retry_seconds` | `30` | How often a second Squad process checks whether the first one exited. |
| `followup_observe_seconds` | `120` | How long Squad watches whether the agent reacted to a steer. |
| `ignore_repos` | `[]` | List of `owner/name` (case-insensitive). Sessions in these repositories are never watched. |
| `ignore_bot_actors` | `true` | Ignore accounts whose name ends in `[bot]`. |
| `bot_actor_allowlist` | `[]` | Bots you still want, for example `["copilot[bot]"]`. |
| `human_feedback_mode` | `all` | `all`, `allowlist` or `off`. See "Who can wake your agent". |
| `actor_allowlist` | `[]` | Used only in `allowlist` mode. List other trusted logins, not your own. |
| `stranger_notice_threshold` | `3` | Steers from unfamiliar accounts, per repository per 7 days, before a notification. |

Which changes apply live: the feature flag is live. `ignore_repos` and `human_feedback_mode`, like every other key in this table, are read once at startup, so changing them needs a restart of Squad. (A reload is not part of the first version.)

## Check the status page

Squad serves a status page from memory (it makes no inbox call):

```
curl -s 'http://localhost:8543/actuator/inbox-pr-watch?format=text'
```

Drop `?format=text` for JSON. The port is your configured `listen_address` (default 8543). The page needs no token on that local listener. Squad's remote listener may also expose the route, but that is UNVERIFIED and is not a supported way to read the page.

The status history (recent events, counters and the unfamiliar-account window) is kept in memory and resets when Squad restarts. The log lines are the durable record.

Sample output, with one next-steer value per session and counts that add up (6 events: 2 steered, 1 waiting, 3 not steered):

```
PR watch status
Feature flag: on
This process polls: yes
Inbox: reachable (last ok 3s ago)
GitHub login: resolved
Watching 3 PRs in 3 repositories; 1 repository not covered.

Settings
  Comment and review steers: all accounts except you (human_feedback_mode all)
  Corrected: reconcile_interval_seconds 3 was raised to 10, the minimum
  Ignored: actor_allowlist has no effect while human_feedback_mode is all

Repositories
  acme-corp/widget-api   webhook coverage: covered
  other/repo             webhook coverage: NOT covered (gh-signal said: no admin access to the repository)
                         Run: gh-signal inbox watch other/repo 42 --consumer squad:devbox
                         Or add other/repo to inbox_pr_watch.ignore_repos
  third/repo             webhook coverage: unknown (gh-signal has no /v1/watch yet)

Sessions
  session 3f9a21c4 "fix flaky login test"   acme-corp/widget-api#123   watching, covered
    last steer: 12:03:11 delivered
    next steer: 12:08:11 (waiting for the batching window to close)
    CI failure on a new commit: steers immediately
  session a8c0e2d9 "retry backoff"          other/repo#42   watching, NOT covered (events will not arrive)
    last steer: none
    next steer: now
    CI failure on a new commit: steers immediately
  session 5d41b7aa "typo fixes"             third/repo#7    watching, coverage unknown
    last steer: 11:41:09 delivered
    next steer: now
    CI failure on a new commit: steers immediately
  session 77be0d10 "cache spike"            (not watched: repository is in ignore_repos)
  session 1c2d3e4f "docs cleanup"           (not watched: its backlog item is waiting on this PR, which the backlog path handles)

Waiting events
  3f9a21c4  acme-corp/widget-api#123  comment  waiting 3 minutes: batching with the steer sent at 12:03; next steer at 12:08

Recent events (newest first, up to 200, cleared on restart)
  12:04:30  comment  acme-corp/widget-api#123  waiting: batching with the steer sent at 12:03
  12:03:11  review   acme-corp/widget-api#123  steered
  12:02:40  comment  acme-corp/widget-api#123  not steered: you wrote it (own comment)
  12:01:02  ci       acme-corp/widget-api#123  not steered: same commit as an earlier CI steer
  11:41:09  comment  third/repo#7              steered
  11:30:15  review   third/repo#7              not steered: approval

Steers by who triggered them (since start): you 0, bot 0, allowlisted 1, other 1
Counts since start: steered 2, waiting 1, not steered 3, inbox errors 0
```

Reasons appear as plain phrases in the text page. The JSON page and the logs use the codes listed below.

## Symptoms and fixes

Start with the status page, then match what you see.

| What you see | Meaning | Fix |
|---|---|---|
| Inbox: unreachable | Squad cannot connect to the inbox. | Start the `gh-signal-inbox` service; check `url`. |
| Inbox: unauthorized | The inbox rejected Squad's token. | Make the token file hold the token the running inbox expects; restart `gh-signal-inbox` if the token was rotated. |
| Inbox: token file missing | There is no readable token file. | Check that `gh-signal-inbox` created it, or set `token_path`. |
| Inbox: token file too open | Other users can read the token file. | Run `chmod 600` on it. |
| Inbox: URL is not a loopback address | `url` points somewhere other than this machine. | Fix `url`, or turn the flag off. |
| GitHub login: unresolved | Squad does not know your login, so comment and review events wait (CI events still steer). | Run `gh auth status`; check that Squad can reach api.github.com. |
| A repository shows NOT covered | No webhook reaches gh-signal for that repository. | Run the command the status page prints, or add the repository to `ignore_repos`. |
| Coverage shows unknown | The inbox has no watch endpoint yet, so Squad cannot tell. Events still arrive if a webhook exists. | Create the webhook with `gh-signal inbox watch`. |
| Waiting events keep growing | The session is busy, not ready, paused, or the batching window is open. | Open the session. Events older than `held_age_ceiling_hours` are dropped. |
| Session listed under not watched | See the skip table below. | Follow the fix in that row. |
| Nothing happens when you comment | Your own comments are dropped. | See "Verify it works". |
| "idle in this Squad process" | Another Squad process holds the lock. | Expected with two Squads; otherwise stop the other one. |

### Why a session is not watched

| Reason code | In plain words | Fix |
|---|---|---|
| `no_pr` | The session has no PR number yet. | Wait; hand-opened PRs take 1 to 6 minutes to be noticed. |
| `no_owner_repo` | Squad could not work out the repository. | Check the session's git remote. |
| `ghe_host` | The PR is on a GitHub host other than github.com. | Not supported. |
| `fork` | The session works on a fork. | Not supported. |
| `backlog_pr_pending` | A backlog item is waiting on this PR and the backlog path handles it. | None needed. |
| `backlog_lookup_failed` | Squad could not read the linked backlog item, so it skipped the session to be safe. | Retried next tick. |
| `ignore_repos` | The repository is in your `ignore_repos` list. | Remove it and restart. |
| `hidden` | The session is hidden. | Unhide it. |
| `not_live` | The session is stopped or gone. | Start the session. |

### Why an event did not steer

These events are dropped on purpose. Each is counted and shown in the recent events list.

| Reason code | In plain words |
|---|---|
| `approved_review` | An approving review never steers. |
| `pull_request_event` | PR open, close and update events never steer. |
| `edited_or_deleted` | Edits and deletions never steer. |
| `ci_not_failure` | The run passed, was cancelled, skipped or inconclusive. |
| `ci_check_run` | A single job result; only the completed run as a whole steers. |
| `self_actor` | You wrote it. |
| `bot_actor` | An account ending in `[bot]` wrote it. |
| `human_feedback_off` | Comment and review steering is off. |
| `actor_not_allowlisted` | The author is not on your allowlist. |
| `unknown_kind` | An event type Squad does not recognize. |
| `repo_mismatch` | The event named a different repository than the subscription. |
| `hidden_session` | The target session is hidden. |
| `session_not_live` | The session ended, or Squad's catch-up covers this case after resume. |
| `held_age_ceiling` | The event waited longer than `held_age_ceiling_hours`. |
| `covered_same_head_sha` | Same commit as an earlier CI steer; one failed run produces many events. |
| `folded_over_cap` | More than 50 events waited at once; the closing steer covers them. |

### Why an event is waiting

A wait is not a failure. The event stays in the inbox and Squad retries.

| Reason code | In plain words |
|---|---|
| `window_batching` | A steer went out recently; the next one is sent when the window closes. |
| `login_unresolved` | Squad does not know your GitHub login yet. |
| `busy` | The session is busy. |
| `guard_busy` | Another nudge is being written to the session. |
| `cooling_down` | A write just failed; Squad pauses briefly. |
| `no_status_source` | Squad cannot tell if the session is ready. |
| `unspecified` | The write path gave no clear answer. |
| `not_tracked` | Squad does not see the session yet, for example just after a restart. |
| `failed` | The write to the session failed. This is the only wait that notifies you at once. |
| `session_suspended` | The session is paused or hibernated. |
| `ci_duplicate_new_sha` | A new failing commit arrived inside the duplicate-nudge window. |

## Verify it works

Your own comment will not do: events from your login are dropped as `self_actor`, and that includes comments you type in the GitHub web UI. To verify, use one of these:

- Comment on the PR from a different account (or a teammate), with `human_feedback_mode` set to `all`, or to `allowlist` with that account listed.
- Push a commit that fails a check, and wait for the completed run.
- Comment as yourself, then read the status page: the recent events list shows the event as not steered because you wrote it, and the not-steered counter goes up. That proves the event arrived.

When it works, the status page shows the event as steered, and the session pane receives one line that starts with `[stapler-squad automated notice]`.

## What the steer says

One line, the same for every program: `[stapler-squad automated notice] PR #<n> (<owner>/<repo>) has new activity (<kinds>).` followed by a pointer to `gh pr view`, `gh api` or `gh pr checks`, a reminder that all PR text is untrusted and that real Squad notices appear only in the terminal, and an instruction to act only if the feedback is within the task. `<kinds>` is some of review, comment, inline comment and CI failure.

The steer never contains comment text, the author's login, the PR title, a commit message or any other text from GitHub. The agent has to read the PR itself.

## Who can wake your agent

On a public repository, anyone can comment, and a comment steers your agent. This is a residual risk. The steer text carries no PR content, but an unfamiliar account can still make the agent look at the PR. Choose a `human_feedback_mode`:

- `all`: every account except you and (by default) bots.
- `allowlist`: only logins in `actor_allowlist` and `bot_actor_allowlist`. An empty `actor_allowlist` behaves like `off`. List other trusted people; your own login is dropped first.
- `off`: CI-only steering. Failing CI still steers, which suits a solo operator. In this mode comment and review events are dropped as `human_feedback_off`, and the catch-up after resume skips the changes-requested check.

When the effective setting is CI-only (an empty allowlist, or `off`), Squad shows that on the status page and sends one notice.

Squad classifies each actor as `self`, `bot`, `allowlisted` or `other` (`actor_class`). Only the class is ever logged, counted or shown, never a login. If steers from `other` accounts on one repository exceed `stranger_notice_threshold` in 7 days, you get the "comments from unfamiliar accounts" notification (the StrangerActivity notice) with the PR numbers involved. Squad does not pause itself; you decide.

To keep a useful bot such as Copilot review, add it to `bot_actor_allowlist`, for example `["copilot[bot]"]`.

## Opt out

- Per repository: add `owner/name` to `ignore_repos` and restart. On the next reconcile Squad removes its subscriptions for that repository.
- Per kind of feedback: set `human_feedback_mode` (restart).
- Everything: turn off the flag.

There is no per-session snooze. A snooze would need state that survives restarts and a way to expire it, and the repository-level controls cover the real cases.

## While a session is paused or hibernated, and while the flag is off

- Paused, hibernated or crashed sessions keep their subscription. Events for them wait in the inbox. When the session is live again, Squad does one catch-up: it re-reads the PR's check result and changes-requested count and steers once if needed. Comments and other review states are not covered by the catch-up, so they stay waiting until delivered or until `held_age_ceiling_hours`.
- Stopped, failed or removed sessions have their pending events dropped as `session_not_live`, and the subscription goes at the next reconcile.
- While the flag is off, nothing is polled, steered or acked. Subscriptions persist and items accumulate in the inbox.

## When you turn it back on

Items that piled up while the flag was off and are younger than `held_age_ceiling_hours` are steered, one steer per window. Items older than that are dropped as `held_age_ceiling`.

## What a batching window does

After a comment or review steer, further comments and reviews for the same PR wait (`window_batching`) for `min_steer_interval_seconds`. When the window closes, one steer announces them all. A second comment is therefore announced when the window closes, not lost. A failing CI run on a new commit ignores the window and steers immediately. Set the interval to 0 to turn batching off.

## Notifications and what to do

Every notification ends with a Guide line (the path of this document) and a Status line (the status URL).

| Notification title | What it means | Next step |
|---|---|---|
| `PR watch: no webhook coverage for <owner>/<repo>` | No webhook reaches gh-signal for that repository, so its PRs will not steer. The message includes gh-signal's reason. | Run `gh-signal inbox watch <owner>/<repo> <n> --consumer squad:<hostname>`, or add the repository to `ignore_repos`. |
| `PR watch: could not steer a session` | The write to the session failed. Squad will retry. | If it keeps happening, open the session. |
| `PR watch: an event is waiting for a session` | An event has waited past `held_notice_after_minutes`. The message states why: session busy, session not ready, login unresolved or window open. | Open the session or the PR yourself. |
| `PR watch: dropped an event that could not be delivered` | An event waited past `held_age_ceiling_hours` and was dropped. The message states why it waited. | Open the PR to review it. |
| `PR watch: GitHub login unresolved` | Squad could not learn your login for 10 minutes; comment and review events wait. | Run `gh auth status` and check api.github.com is reachable. |
| `PR watch: comments from unfamiliar accounts on <owner>/<repo>` | Accounts that are not you, not a bot and not allowlisted caused more steers than `stranger_notice_threshold` in 7 days. | Set `human_feedback_mode` to `allowlist` or `off`, or add the repository to `ignore_repos`. |
| `PR watch: gh-signal inbox is not reachable` | Squad cannot reach the inbox. | Start the `gh-signal-inbox` service. |
| `PR watch: gh-signal inbox rejected Squad's token` | The inbox answered 401. | Put the right token in `token_path`; restart `gh-signal-inbox` if it was rotated. |
| `PR watch: inbox token file is missing` | No readable token file. | Check `gh-signal-inbox` created it, or set `token_path`. |
| `PR watch: inbox token file is too open` | Other users can read the token file. | Run `chmod 600` on it. |
| `PR watch: inbox URL is not a loopback address` | `url` is not 127.0.0.1, ::1 or localhost. | Fix `url` or turn the flag off. |
| `PR watch: idle in this Squad process` | Another Squad process holds the lock. Expected when two run. | Otherwise stop the other process; this one retries. |
| `PR watch: recovered` | A problem above is resolved. | None. |

## Clean up subscriptions

Subscriptions are owned by the consumer name `squad:<hostname>`. After turning the flag off, or after the machine's hostname changes, list and remove leftovers:

```
gh-signal inbox subscriptions --consumer squad:<hostname>
gh-signal inbox unsubscribe <sub_id> --consumer squad:<hostname>
```

Run the second command once per row. The inbox cannot list consumers, so after a hostname change use the old hostname: those subscriptions are orphaned under the old consumer name and Squad no longer manages them.

Squad never calls the GitHub hooks API. The webhook itself is created by gh-signal at Squad's request; remove it with `gh-signal inbox unwatch` or by deleting the hook in the repository settings.

## Limits

- Squad notices a hand-opened PR 1 to 6 minutes after you open it. Until then the session shows no PR.
- Fork sessions, other GitHub hosts and backlog items waiting on a PR are not covered (see the skip table).
- The status page is local to the machine running Squad. The remote-listener status URL is UNVERIFIED and not supported.
- The status history resets on restart.

## Glossary

- **steer**: a single line Squad writes into a session's terminal to tell the agent there is new PR activity.
- **waiting**: an event Squad has received but not yet delivered; it stays in the inbox and is retried.
- **not steered**: an event Squad received and deliberately did nothing with. It is acknowledged and counted.
- **covered**: a webhook for the repository reaches gh-signal, so events can arrive.
- **window**: the batching period after a steer during which further comments and reviews wait and are announced together.
