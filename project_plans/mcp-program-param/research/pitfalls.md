# Pitfalls Research: mcp-program-param

## 1. Dynamic-enum option: stale enum after registration

The MCP tool schema is built exactly once per process lifetime, not per-request or per-connection:

- `server/mcp/server.go:34` `NewCore(...)` calls `registerLifecycleTools(s, ...)` (`tools_lifecycle.go:52`)
  and, when `prCache != nil`, `registerGithubTools(s, ...)` (`tools_github.go:68`) — both build the
  `mcpgo.NewTool(...)` schema (including any `Enum(...)`) a single time when `NewCore` runs.
- `NewCore` itself is invoked exactly once at process startup on both transports:
  - HTTP: `server/server.go:951`, `servermcp.NewHTTPHandler(...)`, called once during server setup
    (not per-request — `mcpserver.NewStreamableHTTPServer(core, ...)` wraps the single `core` built at
    that call site; requests reuse the same `MCPServer`/tool registry, only session state is stateless).
  - stdio: `server/mcp/server.go:153`, `RunServer` calls `NewCore` once before `stdio.Listen(...)`.

So a dynamic-enum fix that reads `ListProgramsConfig`/`cfg.SessionDefaults.Programs` **at
tool-registration time** bakes in whatever programs exist at process boot. A program added later via
`UpsertProgramConfig` (`server/services/defaults_service.go:709`, which calls `config.SaveConfig`) is
invisible to the already-built schema until the process restarts — a strictly milder version of the
exact bug this fix addresses (silently rejects a program that now exists on the backend, for the life
of the process rather than forever). Given `make install-service` is the only thing that restarts the
service, and per this repo's own `docs/explanation/tmux-keep-server-on-restart.md`, that's a
deliberately rare, disruptive operation — not something a user would do just to pick up a newly added
program.

**Implication for planning:** if the dynamic-enum option is chosen, this staleness window must be
called out explicitly (and probably accepted, or mitigated by re-registering tools on config change —
no such mechanism exists today; nothing in `server/mcp/` re-runs `registerLifecycleTools`). The
free-form-string option has no equivalent staleness risk since it does no enum lookup at all.

## 2. Free-form-string option: typo failure mode is opaque, not absent

Traced the full path a `program` string takes from the MCP tool through to actual execution:

- `server/services/session_service.go:2452` reads `program := req.Msg.Program` with **no validation
  against any known-programs list** anywhere in `CreateSession`.
- `server/services/session_service.go:2521-2531`: `config.ResolveProgramConfig(cfg, program)`
  (`config/defaults.go:211`) is called, but only to special-case *custom* IDs:
  ```go
  func ResolveProgramConfig(cfg *Config, program string) ResolvedProgram {
      if prog := FindProgramConfig(cfg, program); prog != nil {
          ... return ResolvedProgram{..., IsCustom: true}
      }
      return ResolvedProgram{Command: program, CLIFlags: "", EnvVars: nil, IsCustom: false}
  }
  ```
  `FindProgramConfig` only searches `cfg.SessionDefaults.Programs` (custom programs), not built-ins.
  For any string that isn't a registered custom ID — including a typo like `clade` — `IsCustom` is
  `false` and the **raw string is silently carried forward unchanged** as the eventual command.
- Nothing later in `CreateSession` rejects the request. The RPC returns success; a session and tmux
  pane are created.
- The typo only surfaces when `Instance.buildLaunchCommand` (`session/instance_tmux.go:262`) actually
  execs the program string in the tmux pane — the shell reports `clade: command not found` (or
  similar) as terminal *output*, not as an API error. An agent calling `create_session` would see
  `still_creating`/`Session` success back from the tool call and would have to separately inspect
  session output (`read_session_output` / `get_session`) to discover the failure.

**Implication for planning:** dropping the enum trades a hard client-side rejection (clear, but
overly strict) for a "successful" API response followed by a silent runtime failure discoverable only
by reading terminal output — this is a worse failure mode than the current one for a typo, and
directly implicates AC4 ("must not silently trade 'rejects valid input' for 'no guidance on valid
input'"). The requirements already anticipate mitigating this via tool description text and/or a
still-present (dynamically populated) `enum` for guidance; a free-form-only fix with no such guidance
would regress typo UX. Consider whether validating `program` against `ListProgramsConfig`'s result
inside `CreateSession` itself belongs in a follow-up — out of scope here per requirements
("No proto or backend validation behavior changes").

## 3. mcp-go's `Enum(...)` is documentation only — never server-enforced

Checked `github.com/mark3labs/mcp-go@v0.48.0` (the version in `go.mod`, resolved via `go env
GOMODCACHE`): `Enum(...)` (`mcp/tools.go:1025`) only sets `schema["enum"] = values` in the tool's
published JSON Schema. Neither `mcp/tools.go` nor `server/server.go` in that module ever reads
`schema["enum"]` back to validate an incoming `CallToolRequest`'s arguments — there is no
`ValidateArguments`/schema-validation step anywhere in the package (confirmed via grep for
`enum`/`Enum` in `server/server.go`: no hits).

This means the enum was **never a server-side security boundary** — it only shapes what a
schema-respecting MCP client (e.g., Claude itself, deciding what value to pass based on the declared
schema) will construct. A client that ignores the schema could already send an arbitrary `program`
string today; the bug this project fixes is a *usability* bug (a well-behaved client refuses to try
values outside the enum), not a bypassable-but-present validation gate. This confirms dropping/loosening
the enum introduces **no new attack surface** — see also #5.

## 4. Test-suite pitfalls: no literal-enum assertions found

Searched `server/mcp/*_test.go` for any test asserting on the JSON schema's enum values (e.g.
checking `["claude","aider"]` verbatim) or referencing `"aider"` at all:

```
grep -rn "aider" server/mcp/*_test.go   → no hits
grep -rln "mcpgo.Enum(" server/mcp/*_test.go → no hits
```

No existing MCP-layer test inspects `create_session`/`create_session_for_pr`'s tool schema directly,
so removing or changing the `Enum(...)` call is very unlikely to break an unrelated assertion. (By
contrast, `tests/e2e/pipeline-mode-stage-executors.spec.ts` and
`tests/e2e/session-program-change.spec.ts` exercise `program: "aider"` through UI/backend paths
unrelated to the MCP tool schema — those are not at risk from this change either way.) New tests per
AC5 (a non-`claude`/`aider` program value through the MCP tool path) have a clean slate to add to.

## 5. Concurrency / config-read pitfalls

- `config.LoadConfig()` (`config/config.go:1203`) does **not** read from a shared, long-lived
  in-memory `*Config` singleton — every call re-reads the config file fresh from disk via
  `LoadConfigFromPath`. `ListProgramsConfig` (`server/services/defaults_service.go:685`) and
  `UpsertProgramConfig` (`:709`) both call `config.LoadConfig()` themselves per-RPC, so there is no
  analogous "raw field vs. `Snapshot()`" data race here the way `.claude/rules/instance-lock-free-reads.md`
  describes for `*Instance` fields — reads are always fresh, and writes go through
  `saveConfigLockFor(configPath)` (`config/config.go:1249` comment), a per-path mutex serializing the
  write-tmp-then-rename sequence so concurrent writers can't tear the file, and renames are atomic so
  concurrent readers never observe a half-written file.
- The actual pitfall is **not a data race** but the staleness window from #1: a dynamic-enum fix that
  calls `ListProgramsConfig`-equivalent logic once at `NewCore`/tool-registration time captures a single
  point-in-time snapshot with no subscription to later changes. There is no existing convention in this
  codebase for invalidating/rebuilding an already-registered MCP tool's schema when config changes —
  this would be new territory, not a pattern to reuse.

## 6. Security: no injection risk from accepting arbitrary program strings

Confirmed the command-construction path already treats `Program` as untrusted, independent of the MCP
enum:

- `Instance.buildLaunchCommand` (`session/instance_tmux.go:262`) shell-quotes the program and each
  argument token individually (`shellQuote`) before assembling the tmux command line.
- `session/instance_tmux_test.go:660`
  `TestBuildLaunchCommand_should_PreventCommandInjection_When_ProgramContainsShellMetacharacters` is an
  existing regression test built for exactly this concern (a "pre-mortem P1" test, per its comment) —
  it constructs `Instance{Program: "true; touch /tmp/pwned"}`, runs the built command through a real
  shell, and asserts the injected `touch` never executes.
- A program ID is never used to look up a filesystem path directly (no path-traversal vector) — it's
  either matched against `cfg.SessionDefaults.Programs` (custom ID → its stored `Command`/`CLIFlags`) or
  passed through verbatim as the command token, quoted the same way a raw command string always has
  been (e.g., via `AliasConfig.Program`, which already accepted arbitrary text).

**Conclusion:** accepting arbitrary program strings through the MCP tool does not widen the injection
attack surface — that surface (arbitrary program/command text) already existed via aliases and the
proto's free-form `program` field, and is already defended at the `buildLaunchCommand` layer regardless
of what the MCP schema declares. Per #3, the MCP enum was never itself a security control.

## Summary of decision-relevant pitfalls

| Option | Key risk | Severity |
|---|---|---|
| Dynamic enum (`ListProgramsConfig` at registration) | Stale until process restart — programs added after boot are invisible to the schema | Real but bounded; matches requirements' own framing as a fix candidate to evaluate |
| Free-form string (drop `Enum`) | Typos succeed at the API layer and fail silently later in terminal output, not as a clear tool-call error | Real UX regression risk for AC4 unless description text specifically warns/guides |
| Either | mcp-go's `Enum` was never server-enforced; no test asserts on literal enum; no injection risk either way | Not blocking, but explains why "hardcode the enum" only ever affected well-behaved clients |
