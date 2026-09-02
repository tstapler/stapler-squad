# Dispatch Work to Google Jules

Google Jules is Google's cloud coding agent. stapler-squad can dispatch a
backlog item's already-pushed branch to Jules, poll its status, and let the
PR it opens converge through the normal backlog PR-review path — an
alternative to spawning a local tmux/Claude Code session for items you'd
rather run on Google's infrastructure.

## Prerequisites

1. **Connect the repo through Jules' GitHub App at
   [jules.google.com](https://jules.google.com).** This step is **not**
   automatable via the API — Jules only lists sources it already knows
   about (`ListSources`), it has no create endpoint. Do this once per repo,
   before the first dispatch.
2. **Push the branch first.** Jules clones from GitHub, not from your local
   worktree — dispatching a branch that only exists locally fails.
3. **Add your Jules API key** in Settings → Jules (`JulesSettings`). The key
   is stored in the OS keychain, never in `config.json`.
4. **Enable the feature** (same settings panel) and, the first time you
   dispatch against a given repo, **confirm cloud egress** — Jules runs on
   Google's infrastructure, so dispatching sends that repo's contents there.
   The confirmation is per-repo and persists once given.

## Dispatching an item

1. Open a `ready` backlog item's detail page. If Jules is disabled, or the
   item isn't eligible, the `Dispatch to Jules` button isn't shown at all
   (`[data-testid="dispatch-to-jules"]`); a disabled-with-reason state shows
   `[data-testid="dispatch-to-jules-reason"]` instead.
2. Click **Dispatch to Jules** to open the dialog.
3. Confirm/edit **Branch** and **Prompt** (prompt defaults to the item's
   title + acceptance criteria).
4. If this repo hasn't been acknowledged for egress yet, check the
   confirmation box — **Dispatch** stays disabled until you do.
5. Click **Dispatch**. The item moves to `in_progress`; a
   `JulesUsageCounter.session.dispatched` count and a
   `jules session created` log line record the event.

## Badge states

The Jules status badge on the item shows the cloud session's own reported
state — the only signal available for work running outside stapler-squad's
tmux/PTY layer:

| Badge | Meaning |
|---|---|
| Jules: Queued | Session accepted, not yet started. |
| Jules: Running | Jules is actively working. |
| Jules: Needs Review | Jules finished and opened a PR — review it like any other backlog PR. |
| Jules: Done | Jules finished but opened no PR (rare) — check the note on the item. |
| Jules: Failed | Jules reported a failure — the item returns to `ready` with the failure reason. |
| Jules: Reconnect required | The stored API key was rejected (401/403) — update it in Settings. |

## Escape hatch

If a session gets stuck, its badge goes stale, or Jules' own state seems
wrong, check the session directly at
[jules.google.com](https://jules.google.com) — it's the source of truth.
stapler-squad's poller fails soft: an unreachable or vanished session, or one
that exceeds its max runtime, is ended automatically and the item returns to
`ready` with a progress note explaining why, so you can just retry dispatch
rather than digging through logs. For log-level debugging, every step is
logged under the `jules` logger — see
[`docs/how-to/debug-with-logs.md`](debug-with-logs.md).
