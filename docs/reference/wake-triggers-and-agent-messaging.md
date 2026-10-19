# Wake triggers and agent messaging: MCP session credential

Reference for the per-session MCP credential that the wake and peer-help tools
(`schedule_wake`, `list_wakes`, `cancel_wake`, `request_help`, `answer_help`)
rely on. Decision record: ADR-007 rule 0 in
`project_plans/notification-actions-and-agent-messaging/decisions/`.

## What it is

A proof is `ssqmcp1.` plus `hex(HMAC-SHA256(secret, "ssq-mcp-v1" || session UUID))`.
The secret is the hook-proof secret (`<configDir>/hook-proof-secret`), so a
hook proof can never be used as an MCP proof, and `HookProofs.Rotate` revokes
every MCP proof (running sessions then need a relaunch).

The `/mcp` middleware (`server/mcp/identity_context.go`) always records the
claimed `X-Stapler-Session-UUID` as the unverified identity that pre-existing
tools use. It records a verified identity (`verifiedCallerSessionUUID`) only
when `X-Stapler-Session-Proof` verifies for that UUID and the session still
exists. The five tools above accept only the verified identity.

## Delivery: files only

| Path | Mechanism |
|---|---|
| Claude CLI | `--mcp-config <configDir>/mcp/<uuid>.json` (0600, directory 0700). The file carries the proof header. |
| Stdio proxy (`stapler-squad --mcp`) | Reads `<configDir>/mcp-proofs/<uuid>` using `STAPLER_SESSION_UUID`. |
| OpenCode | `OPENCODE_CONFIG=<configDir>/mcp/<uuid>.opencode.json`; the header value is `{file:<configDir>/mcp-proofs/<uuid>}` (spike S5). |

The proof is never in argv and never in the tmux environment: there is no
`STAPLER_SESSION_PROOF` variable. If a credential-bearing config file cannot
be written, the session launches without the proof (the inline fallback never
carries the proof or the local API token) and its five tools return
`identity_unverified`.

## Spawn-path matrix

| Spawn path | Outcome |
|---|---|
| Claude tmux: work, hidden, one-shot, resumed | verified |
| OpenCode (per-session config + proof file present) | verified |
| Headless Claude runner (no MCP servers) | not reachable |
| pi, Aider, agy, Gemini CLI (no per-session header path, spike S5) | `identity_unverified` |
| External or IDE sessions (ssq-mux) | `identity_unverified` |
| Boot window before the provider is wired | `identity_unverified` until relaunch |
| In-process `RunStdio` (no HTTP middleware) | `identity_unverified` |
| Session spawned before the upgrade | `identity_unverified` (relaunch the session) |

Tests: `server/mcp/spawn_path_matrix_test.go`, `session/instance_tmux_nonleak_test.go`.

## Lifetime

A proof is valid for the whole life of the session (until a global rotate).
The proof file, `mcp/<uuid>.json` and `mcp/<uuid>.opencode.json` are deleted
when the session is deleted (`EventSessionDeleted`), and a startup sweep
removes files whose session is not known. The identity middleware also
requires the session to exist, so a still-valid proof of a deleted session is
refused.

## Diagnosing "the tools are not working"

- Startup logs `mcp credential self-check ok` (or an error, in which case no
  session gets a proof).
- `get_session` returns `proof_present` (a boolean; the value is never
  returned).

## Residual risk (accepted)

The guarantee is that an agent cannot act as a session by knowing its UUID. It
is not a defence against a hostile or prompt-injected agent running as the same
OS user:

- Same-user file read: any process of the same user can read
  `<configDir>/mcp/<uuid>.json`, `mcp-proofs/<uuid>` and the secret file.
- `tmux show-environment` is why the environment is not used: it returns a
  session's environment to any process that can reach the tmux socket.
- An agent holding `run_command`, or `steer_session` plus `read_session_output`,
  can make another session print a proof file and read it back.

Mitigation, not a boundary: `read_session_output` and `wait_for_output` replace
proof values with `[redacted-proof]`. Scrollback on disk, Claude transcripts
(`search_claude_history`), `get_session` previews, notification snippets and
`get_session_diff` are not redacted. The `:8543` listener remains
unauthenticated.

## The five tools

`schedule_wake`, `list_wakes`, `cancel_wake`, `request_help` and `answer_help`
are registered through `withCallerIdentityGate`
(`server/mcp/caller_identity_gate.go`). The caller is always the verified
session; a `session_id` argument is ignored. A request without a verified
credential gets `identity_unverified` and nothing is written.

| Tool | Roles allowed |
|---|---|
| `schedule_wake`, `cancel_wake`, `answer_help` | work sessions |
| `list_wakes`, `request_help` | work, triage, review, diagnose |

Typed failures an agent can switch on: `identity_unverified`, `flag_off`
(`error.reason` is the flag scope), `cap_exceeded` (`error.limit`),
`unavailable`, `no_reconcile_predicate`, `rejected` (`error.reason`:
`self_target`, `peer_not_found`, `peer_archived`, `peer_one_shot`,
`peer_item_terminal`, `cross_workspace`) and `FORBIDDEN`.

Flags (`agent_wake_triggers`): a tool needs `tool:<name>` and a running
dispatcher; `schedule_wake` also needs `trigger:<type>`; `request_help` also
needs `trigger:guidance_answered`, because the asker is woken by it.

`pr_event` and `ci_event` come from the PR poller (`github.UserPRCache`), not
the event bus (Spike S4). They cover open PRs of the connected accounts; any
other PR is `unavailable`. Supported `(type, kind)` pairs are the table in
`server/wakes/kinds.go`; a kind without a persisted-state predicate is
`no_reconcile_predicate`.

## Operator surface

`WakeTriggerService` (`proto/session/v1/wake_trigger.proto`,
`server/services/wake_trigger_service.go`) is what the Background tab's
"Scheduled prompts" section and peer-help rows read and write. Agents never
call it.

| RPC | Does |
|---|---|
| `ListWakeTriggers` | Pending, claimed, `needs_revive` and (for 7 days) `missed` triggers of every owner, plus `enabled`, the kill-switch state. Reads storage, so it works while the flag is off. |
| `CancelWakeTrigger` | Cancels any owner's trigger. Outcome `ALREADY_FIRED` carries `delivered_at` when the trigger fired first. Goes through the dispatcher when it runs, straight to storage when it does not (Cancel works while paused). |
| `CreateOperatorReminder` | Schedules a `notify_operator` trigger about a session ("Remind me"). Outcomes: `CREATED`, `DUPLICATE` (same session and time), `PAST_TIME`, `CAP_EXCEEDED`, `UNAVAILABLE` (no dispatcher). |
| `ListPeerHelpRequests` | Open and 24-hour-old peer requests with a derived state: `waiting`, `answered`, `not_sent` (the answerer is deleted, archived or in another repository) or `cancelled`. |

`CANCEL_WAKE` and `CANCEL_PEER_HELP` (`InvokeNotificationAction`) share the
same code. Cancelling a peer request sets the row `cancelled` and fires the
asker's `guidance_answered` wake, whose text says the request was cancelled.

### Peer-help escalation

`wakes.Escalator` (`server/wakes/escalator.go`) holds one deadline per open
peer request at `created_at + 10 min` on the dispatcher's `Clock`, and escalates
at once when the answerer is stopped, archived, deleted or in another
repository. Escalation raises one `INPUT_REQUIRED` notification bound to the
asker, id `peer-help-stalled:<request id>`, with metadata `guidance_request_id`
and `peer_title`. The guidance row's `notified_at` is the durable marker, so a
restart or a second pass never raises a second one. The descriptor builder adds
`CANCEL_PEER_HELP` (with confirm) beside `ANSWER_GUIDANCE`, labelled
"Answer for {peer}".

An operator answer to a peer request flips the row's `answerer_kind` to `human`
(`answerer_session_uuid` keeps naming the peer); the asker's wake then reads
"answered by the operator on behalf of session X (from=operator on-behalf-of=X)".

### Prompts

With the `schedule_wake` tool row on (`tool:schedule_wake` and a running
dispatcher), the task protocol, the default review command and custom pipeline
mode review and initial prompts add one paragraph pointing at `schedule_wake`.
The existing ban on the client-side wake tool and `/loop` stays in the text.
With the row off the prompts are byte-identical to before
(`session/testdata/wake_prompt/`). `TestPollingToolReferencesInGo_*`
(`session/wake_polling_guard_test.go`) pins the lines per Go file that mention
the banned tool.

### Flag flips apply at runtime

A global or `trigger:<type>` flip of `agent_wake_triggers` starts or stops the
dispatcher at once (`wakeLifecycle` in `server/wakes_module.go`); the 60 s
reconcile tick also applies a change the flip missed and retries a start that
lost the leader guard. Stopping keeps every row. Starting runs the startup
reconcile, so claims stay fire-once. Each `trigger:<type>` scope is read at fire
time: a pending row of an off type is neither fired nor marked `missed`, except
that `expires_at` always wins (ADR-008). While the dispatcher is stopped nothing
marks rows `missed`; expired rows end at the next start.

## Trigger types, states and limits

Added so the page names the values the metrics and alerts use. Source:
`session/domain/wake_trigger.go` and `server/wakes/limits.go`.

Trigger types: `at`, `cron` (time based; `cron` recurs), `backlog_event`,
`pr_event`, `ci_event`, `session_state`, `guidance_answered`.

Row states: `pending`, `claimed`, `delivered`, `needs_revive`, `missed`,
`cancelled`, `skipped`. A claim moves `pending` to `claimed`; the delivery
write moves `claimed` to `delivered` only after a confirmed write; reconcile
re-queues a stale claim. A `claimed` row that reconcile cannot re-queue ends as
`missed`.

Limits. Defaults are config values; ceilings are compiled in and cannot be
raised by an agent or by config. A refusal at create is a typed `cap_exceeded`
result naming the limit; every hit counts in `wake_limit_hit_total{limit,phase}`
(`docs/reference/notification-actions-observability.md`).

| `limit` label | Default | Ceiling | Phase |
|---|---|---|---|
| `min_interval` (shortest cron gap) | 10 min, derived as `max(5, ceil(60 / per-owner limit))` | 5 min floor at any setting | create |
| `pending_per_owner` | 5 | 10 | create |
| `max_fires` | 1 (12 for `cron`) | 48 | create |
| `expires_horizon` | 24 h | 7 days | create |
| `generation` (a woken turn scheduling another wake) | 3 | 5 | create |
| `fires_per_owner_hour` | 6 | 12 | fire |
| `fires_global_hour` | 60 | 120 | fire |
| `peer_open_per_asker` | 3 | 5 | create |
| `payload_size` | 2,000 chars | 4,000 | create |
| `debounce` | 5 s | 60 s | none found |
| `busy_hold` | 30 min | 2 h | none found |
| `revive_lock` | 30 s | 60 s | none found |

"None found" means the limit exists in `ResolveLimits` but no call to
`wake_limit_hit_total` carries that label (`grep -rn LimitBusyHold server`), so the
series stays empty.

The two hourly delivery limits count `steer_text` deliveries only. A deferred
one-shot row becomes `skipped` (reason `rate_limited`) after 3 consecutive
deferrals; a `cron` row skips only that occurrence (`maxRateDeferrals`).
Hitting a limit raises one failure digest per owner per hour.

Expired rows: `ListWakeTriggers` first sweeps pending rows past `expires_at` to
`missed` (`wakes.ExpireOverdue`, `server/wake_trigger_wiring.go`), so a stopped
dispatcher still reports them. The next dispatcher start also marks them
missed in its reconcile. Each swept row raises one failure notification that
names the trigger id.
