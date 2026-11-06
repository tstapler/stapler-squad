# SSQ CLI Subcommands — Known Pitfalls and Risks

Research date: 2026-10-18

## 1. Server-not-running UX

When `localhost:8543` is unreachable, the Connect client returns a raw `connect.CodeUnavailable`
error wrapping a `dial tcp ... connection refused` string. Without a CLI-side intercept the user
sees a multi-line Go error with no actionable guidance.

**Recommendation**: Wrap the `connect.NewClient` call's error at the outermost command `RunE` —
detect `connect.CodeUnavailable` or a `net.Error` with `Timeout()`/`Temporary()` set and print a
single line: `"ssq: server not running — start stapler-squad or set --server/SSQ_SERVER"`.

## 2. Session ID resolution — titles accepted by the server

`GetSessionRequest.id` is documented as "Session identifier (uses session title as ID)".
Both `GetSession` and `GetSessionDiff` accept a human-readable title as the `id` field, not only a
UUID. The existing `get-session` command passes `args[0]` straight through, so title-based lookup
already works at the RPC layer. No extra name-to-ID resolution layer is needed in the CLI; the
server resolves it. `GetSessionDiff` has the same single `id string` field.

**Residual risk**: If two sessions share a title (which the server allows), the server resolves to
one of them non-deterministically. The CLI should document that `--title` values must be unique in
the running instance if used as an ID.

## 3. Steer works for non-autonomous sessions — but with different semantics

The proto comment on `steer_message` says "Only meaningful when autonomous_mode is true." This is
misleading. `steerAuthorized()` in `server/services/steer_authorization.go:172–219` has two
branches:

- **Autonomous session** (`snap.Autonomous.AutonomousMode == true`): message is injected into the
  ClaudeController command queue via `SendCommandImmediate` (async goroutine, 5-second timeout).
- **Non-autonomous session**: message is sent to the raw PTY via `session.SubmitContentWithEnter`
  — the same primitive the MCP `steer_session` tool uses.

Both paths succeed and return no error for an active visible session. The CLI `ssq steer` command
should therefore succeed on any active session, but callers must understand that on a non-autonomous
session the text is typed directly into the PTY as if the user pressed Enter — it is not queued by
the driver, and the session may be in the middle of something else.

**Pitfall for `hidden` sessions**: `decideSteerAccess` returns
`connect.CodeFailedPrecondition / "this background session is read-only"` if the session is hidden
(a backlog-driven session running under the delivery guard) and has no live `BacklogReviewLink`.
An `ssq steer` on a hidden session that is not in review state will fail. The CLI should surface
the connect error reason verbatim rather than swallowing it.

## 4. JSON output contract — `encoding/json` on protobuf structs

The existing `get_session.go` uses `json.MarshalIndent(resp.Msg.Session, "", "  ")` with
`encoding/json`. This has two documented hazards:

- **Enum values become integers**, not string names. `SessionStatus` field 6 prints as `6`, not
  `"SESSION_STATUS_CREATING"`. Scripts that pattern-match on `"active"` or `"SESSION_STATUS_ACTIVE"`
  will see an integer.
- **Proto `optional` fields** are represented as pointer types; `encoding/json` marshals them as
  their unwrapped value or `null`, which differs from the protojson canonical form (omitted when
  unset, or a JSON scalar when set).
- **`oneof` fields** are not rendered correctly; `encoding/json` has no awareness of proto3 oneof
  semantics and may omit the set variant.

**Recommendation**: Use `google.golang.org/protobuf/encoding/protojson` for the `--json` output
path. It emits stable camelCase field names, string-valued enums, and correct oneof handling.
`protojson.MarshalOptions{UseProtoNames: true}` can emit snake_case names for shell-script
compatibility. This is a hard requirement — `encoding/json` on a protobuf struct is an
interoperability bug.

## 5. Port configuration drift — hardcoded 8080 in existing command

`cmd/commands/get_session.go:25` hardcodes `"http://localhost:8080/api"` — the wrong port. The
documented default (CLAUDE.md, `cmd/ssq-hooks/main.go:842`) is `8543`. The comment in that file
explicitly acknowledges there is no dynamic port-discovery mechanism and that a user running on a
non-default port must reconfigure manually.

**Impact**: `ssq get-session` currently hits the wrong port and always fails unless the user
happens to be running on 8080. The new subcommands must not repeat this mistake.

**Recommendation**: Follow the `ssq-hooks` pattern — default the base URL to
`http://localhost:8543/api`, allow override via `--server` flag and `SSQ_SERVER` environment
variable. Document that `PORT` on the server and `--server`/`SSQ_SERVER` on the client must be
kept in sync when the default port is changed.

## 6. CI usage — auth, network, and SSRF

The server binds to `localhost` by default. In a standard single-machine CI environment (GitHub
Actions, local runner) this is safe — the CLI runs on the same host as the server and hits the
loopback interface. The server's `auth/handlers.go` short-circuits auth for loopback-origin
requests by default (no token required), which means `ssq` commands need no credential management
in CI as long as the server address remains `http://localhost:...`.

**SSRF note**: The `--server`/`SSQ_SERVER` design allows the CLI to target arbitrary URLs. If the
CLI is used in a CI system where an attacker can control environment variables, a malicious
`SSQ_SERVER` value could cause the CLI to forward authenticated requests to an internal service.
Mitigate by validating that the URL host resolves to loopback or is explicitly whitelisted, or by
documenting that `SSQ_SERVER` must only be set in trusted environments.

**Remote-access mode**: When the server is started with `--remote-access`, it binds beyond
loopback and `RequireLocalAuth` is active — loopback-origin auto-auth no longer applies. In that
configuration the CLI will receive `connect.CodeUnauthenticated` and must add an auth header or
session cookie. The new subcommands should document this limitation explicitly.

## 7. CreateSession async race — session is not ready on RPC return

`CreateSession` returns a `CreateSessionResponse` that contains a `Session` with
`status == SESSION_STATUS_CREATING` — the tmux session and git worktree setup happen in a
background goroutine ("async creation pipeline / Background Resolution Pipeline"). The session
transitions to `SESSION_STATUS_ACTIVE` once the pipeline completes. Fields like the actual
worktree path are populated by that goroutine and are not available in the initial response
(confirmed by proto comment on `CreateSessionRequest` field 842: "not read back by
CreateSession itself").

**Consequence**: A CI script that runs `ssq create-session` and then immediately `ssq steer`
will likely hit a session in `CREATING` state and fail — `steerUnderLease` tries to take a
`TryTerminalWriteLease`, which will fail because the tmux pane does not yet exist.

**Recommendation**: The `ssq create-session` command should either:
- Accept a `--wait` / `--timeout` flag that polls `GetSession` until
  `status == SESSION_STATUS_ACTIVE` (or a terminal error state), then exit.
- Or by default print the session title and immediately return with exit 0, and document
  explicitly that the session is not ready for steers until it becomes Active.

A non-blocking default (just print and exit) is the right choice for CI pipelines that only need
to queue work and check later. A `--wait` flag covers scripts that immediately steer or read the
diff.
