# Stack Research: live smoke test for write_to_session/steer_session/run_command

Scope: the technology needed for the one remaining piece of work on this item — a
live smoke test confirming PR #832's two-write submit fix actually registers Enter
against a real running Claude Code TUI session. Not researching the fix itself
(already shipped and cited in requirements.md).

## Versions / dependencies

- Go: `go 1.26.6` (`go.mod:3`).
- MCP server library: `github.com/mark3labs/mcp-go v0.48.0` (`go.mod:171`).
- tmux: pinned/bundled version is **3.4** — `make build-tmux` compiles it
  (`docs/how-to/bundle-tmux.md:7`, "Compile pinned tmux 3.4 (~30s)"). No separate
  minimum-version check found in `session/` beyond this build pin; the manual
  instance will use whatever `tmux` resolves on `$PATH` unless built against the
  bundled binary.

## How the MCP endpoint is exposed (two transports, pick one for the smoke test)

1. **Streamable HTTP** (the one to use for an ad hoc external client): mounted
   directly on the running web server. `server/server.go:970` builds the handler via
   `servermcp.NewHTTPHandler(...)`, and `server/server.go:978` mounts it:
   `srv.mux.Handle("/mcp", mcpWithUUID)`. `server/mcp/server.go:97-122`:
   `NewHTTPHandler` returns `mcpserver.NewStreamableHTTPServer(core, mcpserver.WithStateLess(true))`
   — MCP protocol version 2025-03-26, stateless. So a manual instance started on
   `PORT=62871` (per the port block below) exposes MCP at
   `http://localhost:62871/mcp` with no extra flags — this is the simplest path for
   the smoke test.
2. **stdio** (`--mcp` flag): `main.go:137-165`. This is what real Claude Code
   sessions launched *by* stapler-squad use (injected via `--mcp-config`), and it has
   a thin-client proxy fast path (`main.go:130-148`, `mcpProxyURL` at
   `main.go:1763-1772`): it calls `config.LoadConfig()` and proxies stdio to
   `http://<cfg.ListenAddress>/mcp` of whatever instance that config resolves to.
   **Caveat for the smoke test**: `cfg.ListenAddress` comes from the resolved
   instance's persisted `config.json` (default `"localhost:8543"`,
   `config/config.go:697`), not from a `PORT` env var read at this point in
   `main.go` — the `PORT` env override only applies later, in the actual
   HTTP-server-start branch (`main.go:313-321`), which the `--mcp` flag path never
   reaches. So invoking `stapler-squad --mcp` naively will proxy to the *default*
   `localhost:8543` (the live deployed instance) unless the manual instance's own
   `STAPLER_SQUAD_INSTANCE`-scoped config was previously started once with the
   custom `PORT` so it got persisted, or `--listen`/config is set explicitly. This
   is a real footgun for accidentally hitting the live instance during a "manual"
   test — the HTTP Streamable transport (option 1) sidesteps it entirely by naming
   the port directly.

## Connecting a real MCP client for the smoke test

No existing docs page for this specific "point `claude mcp add` at a non-default
stapler-squad instance" workflow — root `CLAUDE.md`'s "Manual/interactive testing"
section covers running the second instance but stops short of MCP client wiring.
Two viable approaches, inferred from the code above and this machine's existing
registration pattern:

- **Direct HTTP registration (recommended)**: `claude mcp add --transport http
  <name> http://localhost:62871/mcp` (Streamable HTTP, matches
  `NewStreamableHTTPServer`'s transport). Requires no env-var care and can't
  accidentally hit the live `:8543` instance.
- **stdio, mirroring the real usage pattern**: this machine's `~/.claude.json`
  already has a project-scoped example for the *default* checkout at
  `projects."/home/tstapler/Programming/stapler-squad".mcpServers.stapler-squad`:
  `{"type": "stdio", "command": ".../stapler-squad", "args": ["--mcp"], "env": {}}`.
  Reproducing this against the manual build would need
  `env: {"STAPLER_SQUAD_INSTANCE": "claude-manual-test"}` (or equivalent) plus
  confirming that instance's config.json's `ListenAddress` is actually
  `localhost:62871` before trusting the proxy — see the caveat above. This path
  is a closer analog to how a real Claude Code TUI session talks to stapler-squad
  (since `buildLaunchCommand` injects `--mcp-config` with the stdio form,
  `docs/tasks/llm-omnibar.md:43`), but the port-resolution caveat makes it the
  riskier of the two for a quick manual test.

## Manual second-instance mechanics (already documented, root CLAUDE.md)

- Build to `~/.stapler-squad/manual-builds/manual-<N>/stapler-squad` (never
  `./stapler-squad` or a bare `/tmp` path).
- Run with `PORT=<port> STAPLER_SQUAD_INSTANCE=<name> <binary> --tmux-keep-server &`.
- **Manual dev port block** (`make ports` recomputes/displays; base `62871`):

  | Port | Use |
  |---|---|
  | 62871 | Manual instance #1 — `PORT` |
  | 62872 | Manual instance #1 — `--remote-port` |
  | 62873 | Manual instance #2 — `PORT` |
  | 62874 | Manual instance #2 — `--remote-port` |
  | 62875–62880 | Spare |

- `STAPLER_SQUAD_INSTANCE=<name>` isolates state under
  `~/.stapler-squad/instances/<name>/` (`docs/reference/state-isolation.md`) —
  separate sessions/config/worktrees from the live `:8543` instance, so the smoke
  test's session-creation calls (`create_session`, `write_to_session`, etc.) can't
  touch real backlog/session state.
- **Never use `make install-service`** for this — it restarts the live systemd
  unit and kills every live tmux session (`docs/explanation/tmux-keep-server-on-restart.md`).

## Existing scripted/automated coverage (why it doesn't already close this gap)

No existing harness runs this exact smoke test (write_to_session/steer_session/
run_command against a live Claude Code Ink-TUI target verifying real Enter
submission). What does exist, and why each falls short of it:

- `server/mcp/server_integration_test.go` (`//go:build integration`,
  `TestMCPHandshakeSubprocess`) — verifies the MCP handshake over the subprocess
  transport, not tool behavior against a live TUI.
- `session/mcp_integration_test.go` (`//go:build integration`,
  `TestSessionStartInWorktreeWithMCP`, `TestRestartFromPausedUsesWorktreeDir`) —
  covers session/worktree startup with MCP wired in, not the submit path.
- `session/pane_submit_test.go` and `server/mcp/tools_terminal_test.go`
  (unit-level, already cited in requirements.md) exercise the two-write
  shape/retry logic and MCP-layer validation, but per requirements.md these run
  against test doubles/synthetic panes, not a real Claude Code Ink-TUI process —
  which is exactly the paste-detector race PR #832 fixed and this item wants
  re-confirmed live.
- Both integration test files are gated behind the `integration` build tag, run
  via `make test-integration`, not `make test`/`make ci` by default.

**Conclusion**: no shortcut exists; the smoke test has to be done by hand (or a
short one-off script) per the "Manual/interactive testing" convention — there is
no pre-built harness to invoke instead.
