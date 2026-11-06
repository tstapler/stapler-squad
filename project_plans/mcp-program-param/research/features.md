# Feature Landscape Research: mcp-program-param

## 1. How `program` flows through today

`server/mcp/tools_lifecycle.go:132` and `server/mcp/tools_github.go:172` both do:

```go
program, _ := args["program"].(string)
...
if program == "" {
    program = "claude"
}
```

then pass it straight into `sessionv1.CreateSessionRequest.Program` (tools_lifecycle.go:194, tools_github.go:243) with **no other validation** — no trim, no case normalization, no allowlist check against the enum they just declared.

That request reaches `SessionService.CreateSession` (`server/services/session_service.go:2452`), which resolves defaults, then at line 2521-2529:

```go
if program != "" {
    resolvedProg := config.ResolveProgramConfig(cfg, program)
    if resolvedProg.IsCustom {
        // merge resolvedProg.EnvVars into instanceEnvVars
    }
}
```

`config.ResolveProgramConfig` (`config/defaults.go:211`) calls `FindProgramConfig` (case-insensitive via `strings.EqualFold`, `config/defaults.go:195`) against `cfg.SessionDefaults.Programs` (custom programs only — **not** the built-in list). If found, it returns the custom program's `Command`/`CLIFlags`/`Env`. **If not found — including for "claude"/"aider"/"pi"/etc. — it silently falls back to treating the raw string as a literal shell command** (`config/defaults.go:221-226`, `Command: program`). There is no allowlist rejection anywhere in this path.

**Key finding: the backend accepts literally any string as `program`, not just registered ones.** An unregistered/typo'd value (e.g. `"clude"`) is not rejected at request time — it's carried through as the literal executable name and only fails later, asynchronously, when tmux tries to exec it (`session/instance_tmux.go`'s `buildLaunchCommand`, used at `session/instance.go:2249`/`2446`). The MCP call itself will report success (`CreateSessionResult.Success: true`, possibly `StillCreating`), and the bad-program failure surfaces only inside the tmux pane or via later `get_session`/log inspection — **not as an actionable synchronous MCP error**. This is a pre-existing gap the requirements' Out-of-Scope section correctly excludes (backend validation), but it's worth naming explicitly since it means a dynamic-enum design (option 2) has real user-facing value beyond convenience: it's the only layer that can give the agent an *actionable* rejection before session creation is attempted.

## 2. Case sensitivity — confirmed consistent

`config/defaults.go:189`'s doc comment ("case-insensitive") is accurate, and `ResolveProgramConfig` shares it — it calls `FindProgramConfig` directly (`config/defaults.go:212`), which uses `strings.EqualFold`. So `Program: "Claude"` or `"OPENCODE"` would resolve correctly if `opencode` were a custom entry. No case-sensitivity mismatch between an MCP-level dynamic enum (populated verbatim from `ListProgramsConfig` IDs) and backend resolution — they'd agree.

Caveat: **no trimming** happens anywhere in this path — not in the MCP handlers, not in `ResolveProgramConfig`. `program: " claude"` (leading space) will not match `FindProgramConfig`'s `EqualFold` check and falls through to being used as a literal (broken) command. This is pre-existing behavior for today's `claude`/`aider` enum too (a client could already send a value with stray whitespace), so it isn't a regression introduced by loosening the MCP schema — but it's a real edge case worth a test either way.

## 3. Built-in programs — the enum is missing 5 of 7 today, independent of custom programs

`server/services/defaults_service.go:645-654` (`BuiltInPrograms()`):

```go
{ID: "claude", ...}
{ID: "pi", ...}
{ID: "aider", ...}
{ID: "opencode", ...}
{ID: "gemini", ...}
{ID: "agy", ...}     // Antigravity
{ID: "bash", ...}    // interactive shell session
```

The MCP tool schemas hardcode only `claude`/`aider` (`tools_lifecycle.go:59`, `tools_github.go:101-103`). So **this bug isn't just about custom programs** — five built-in, backend-registered programs (`pi`, `opencode`, `gemini`, `agy`, `bash`) are already unreachable through the constrained enum today, before any user ever registers a custom one. Any fix should be judged against this full built-in list, not just "claude/aider + custom."

## 4. Built-in/custom name collision — already prevented upstream

`DefaultsService.UpsertProgramConfig` (`server/services/defaults_service.go:735-737`) explicitly rejects registering a custom program whose ID collides with a built-in (`IsBuiltInProgram(id)` check) with `CodeInvalidArgument: cannot override built-in program %q`. So "a custom program ID that collides with a built-in name" is a non-issue for this feature — it's structurally impossible to create such a collision, whichever enum strategy is chosen.

## 5. `mcpgo.Enum(...)` is not server-side enforced — an important design fact

`mcp-go`'s `WithString(..., Enum(values...))` (`mark3labs/mcp-go@v0.48.0/mcp/tools.go:1023-1027`) only writes an `"enum"` key into the JSON Schema exposed via `tools/list`. Grepping `mark3labs/mcp-go@v0.48.0/server/*.go` for `enum` turns up nothing outside test files — **the Go MCP server library itself never validates a tool call's arguments against the declared enum**. Rejection of an out-of-enum `program` value happens only if/because the *calling* MCP client (e.g., the LLM agent itself, respecting the schema during tool-call generation) declines to emit it. Practically: the stapler-squad Go handler code has always accepted whatever string arrived in `args["program"]`, enum or not — confirmed by tools_lifecycle.go:132 doing a bare type-assertion with no validation. This means:

- Dropping the `Enum(...)` call (option 1) is a pure schema-metadata change with **zero risk to existing request-handling behavior** — the handler code doesn't change at all.
- A stale dynamic enum (option 2, if a custom program is later deleted) is **not a functional hazard** — nothing enforces it — only a documentation/UX staleness (an agent might not know a value it's about to try no longer exists, but nothing crashes or gets server-side-rejected differently than an equally-stale hardcoded enum would).

## 6. Tool registration happens once per process/connection — dynamic enum would be stale by construction

`server/mcp/server.go`'s `NewCore()` (called once from `RunServer` for the stdio path, line 153, and once from `NewHTTPHandler` at server-mount time, line 112) registers every tool's schema a single time. `registerLifecycleTools`/`registerGitHubTools` run at that point only — there is no re-registration per tool call, per session, or on config change, and no use of MCP's `notifications/tools/list_changed` capability anywhere in this codebase (`mcpserver.WithToolCapabilities(false)` at server.go:49 explicitly disables the tools capability's `listChanged` flag).

Consequence for option 2 (dynamic enum from `ListProgramsConfig`): a custom program registered *after* the MCP server/session started would not appear in the enum until the stdio process restarts (i.e., until the Claude session that owns this MCP server is itself relaunched) or, for the HTTP transport, until the server process restarts. Combined with finding #5 (enum isn't enforced), this staleness is cosmetic rather than a hard blocker — but it does undercut option 2's main advantage (fewer round-trips) in the case that matters most: a custom program just added in the same session.

## 7. No `list_programs` MCP tool exists

Enumerated every `mcpgo.NewTool(...)` registration across `server/mcp/*.go` (35 tools) — there is no `list_programs`/`list_program_configs` tool. The only way to discover configured programs today is the ConnectRPC `ListProgramsConfig` (`server/services/session_service.go:5321`, delegating to `server/services/defaults_service.go:685`) — which is a Web UI RPC, not exposed to MCP callers at all. **This is the clearest concrete gap for AC4** ("agent still has some way to discover valid program values") — regardless of whether the enum is dropped or kept dynamic, an MCP-native discovery tool is the only mechanism that stays correct in real time (reads config live, on every call) rather than being frozen at tool-registration time like any enum-based approach.

## 8. `session_type` is a different, closed-set pattern — correctly out of scope

`session_type`'s enum (`tools_lifecycle.go:60-61`: `directory`, `new_worktree`, `existing_worktree`; `tools_workflow.go:51/92` adds `one_off`, `new_project` for workflow creation) maps onto `sessionv1.SessionType`, a genuine closed **proto enum** (see `mcpSessionTypeToProto`, tools_lifecycle.go:266+) — the backend has no equivalent to `config.SessionDefaults.Programs` for session types; there's no user-extensible registry of session types the way there is for programs. So `session_type`'s enum is not "masking a more flexible backend" the way `program`'s is — it's an accurate, exhaustive representation of a fixed backend enum. This confirms the requirements doc's Out-of-Scope call is correct: `session_type` should not be touched by this fix, and isn't a precedent that generalizes here. No other MCP tool parameter in `server/mcp/` was found with the same "static enum hiding a config-driven/open-ended backend value" shape as `program` (checked every `mcpgo.Enum(...)` call site: notification types, session statuses, approval-rule categories/decisions/risk levels, guidance-request types, goal/task statuses, backlog verdict/event/item types, terminal control keys — all are genuinely fixed, backend-side closed enums, not registries).

## 9. Existing test coverage — confirmed gap (AC5)

Grepped `server/mcp/*_test.go` for `program`/`Program:` — the only hit is an incidental fixture value (`tools_github_test.go:239`, `Program: "claude"` on a pre-existing session, unrelated to the `create_session_for_pr` request path). **There is no test today that exercises the `program` argument through `createSessionWithAwaitTimeout`/`createSessionForPRWithAwaitTimeout` at all** — not even for the current `claude`/`aider` values. AC5's "new tests cover non-claude/aider program via MCP tool path" will need net-new test scaffolding, not an extension of an existing program-focused test.

## Summary of unstated needs / design implications for the planning phase

- The fix should account for **all 7 built-ins**, not just `claude`/`aider` — the bug is broader than "custom programs are rejected."
- Given finding #5 (enum not server-enforced) and #6 (registration-time staleness), **option 1 (drop the enum, document as free-form text) is lower-risk and avoids a staleness problem that option 2 cannot fully solve without also adding `notifications/tools/list_changed` support** (not present anywhere in this codebase today — would be new infrastructure).
- Regardless of which option is chosen for the enum itself, **AC4 (discovery) is best satisfied by adding a `list_programs` MCP tool** backed by the existing `ListProgramsConfig` RPC (`server/services/defaults_service.go:685`) — it's the only mechanism that reflects live config on every call, and no equivalent exists today.
- If option 1 is chosen, the tool description text should still name the built-in IDs (`claude`, `pi`, `aider`, `opencode`, `gemini`, `agy`, `bash`) as examples, since dropping the enum removes the client's only current visibility into valid values short of a new discovery tool.
- New tests (AC5) need to cover: a custom program end-to-end through `create_session`/`create_session_for_pr`, at least one previously-unreachable built-in (e.g. `opencode` or `bash`), and should decide whether to add trim/whitespace handling given none exists today at any layer.
