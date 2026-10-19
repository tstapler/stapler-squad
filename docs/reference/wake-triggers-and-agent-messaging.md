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
