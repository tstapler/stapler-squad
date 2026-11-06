# Architecture Research: mcp-program-param

Agent 3 (Architecture), SDD research phase. Scope: keep `create_session` /
`create_session_for_pr`'s `program` param in sync, and understand the
integration points around `ListProgramsConfig`.

## 1. No shared tool-option helper file exists yet

`server/mcp/` has no `mcp_helpers.go`/`tools_common.go` (confirmed via
directory listing — 33 non-test `.go` files, all named `tools_<domain>.go` or
`types.go`/`server.go`; `types.go` holds only result/summary structs, no
tool-building helpers). The two `mcpgo.Enum("claude", "aider")` literals
(`server/mcp/tools_lifecycle.go:59`, `server/mcp/tools_github.go:101-103`)
are hand-duplicated with no shared source today.

There is, however, a directly-applicable precedent for *how* to avoid this
duplication: `notificationTypeNames` (`server/mcp/tools_notifications.go:27-34`)
is a package-level var built by a small helper,
`enumNamesWithoutPrefix(names map[int32]string, prefix, excludeFull string) []string`
(`tools_notifications.go:39-49`), and consumed both by the tool registration
(`mcpgo.Enum(notificationTypeNames...)`, line 169) and by the handler's own
parsing (`parseNotificationTypeFilter`, line 55). Its doc comment states the
rationale directly: derive the enum "rather than hand-maintained, so it can't
silently drift out of sync." That is precisely this backlog item's problem,
one file over. A second, weaker precedent is `tools_backlog.go:598-608`,
where a block of `const eventType...` values is commented as "the single
source of truth referenced by ... the mcpgo.Enum(...) tool registration" —
same intent (avoid drift), different mechanism (shared consts, not a
generated list).

Important distinction: `notificationTypeNames`'s source
(`sessionv1.NotificationType_name`) is a **generated proto enum map** — fixed
at compile time. `ListProgramsConfig`'s source (built-ins + `cfg.SessionDefaults.Programs`,
see below) is **live runtime config**, read fresh from disk on every call.
The existing precedent's "derive once at package-var-init time" shape doesn't
transfer cleanly — see §3.

## 2. Integration points: where `svc` and tool registration meet

Both `create_session` and `create_session_for_pr` are registered from the
same single place, `NewCore` (`server/mcp/server.go:34-90`):

```go
registerLifecycleTools(s, &lifecycleHandlers{store: store, svc: svc})   // server.go:53
...
registerGitHubTools(s, &githubHandlers{cache: prCache, store: store, svc: svc})  // server.go:87
```

`svc` is `*services.SessionService`, and **both handler structs already carry
it** — `lifecycleHandlers.svc` and `githubHandlers.svc` are the same field
from the same construction call. `SessionService.ListProgramsConfig`
(`server/services/session_service.go:5321-5323`) just delegates to
`DefaultsService.ListProgramsConfig` (`server/services/defaults_service.go:685-702`),
which returns `BuiltInPrograms()` (7 entries as of this branch — `claude`,
`pi`, `aider`, `opencode`, `gemini`, `agy`, `bash`; `defaults_service.go:645-654`,
**not just `claude`/`aider`** — the current hardcoded enum is already stale
against the built-in list alone, before considering custom programs) plus
`cfg.SessionDefaults.Programs` (custom, user-added via `UpsertProgramConfig`).

`NewCore`/`NewHTTPHandler` is called exactly **once**, at server startup
(`server/server.go:951`, inside the HTTP server setup that runs once per
process). There is no code path that re-invokes `NewCore`, re-registers
tools, or otherwise refreshes `s.tools` later in the process lifetime —
confirmed by grepping every call site of `NewCore`/`NewHTTPHandler`
(exactly one, `server.go:951`) and by reading mcp-go's tool-call dispatch
(`handleToolCall`, `mcp-go@v0.48.0/server/server.go:1556`), which looks a
tool up in the `s.tools` map populated at `AddTool` time — nothing rebuilds
that map on a timer or config-change hook.

**So `svc` is available at registration time** (server startup) to build a
one-time enum snapshot, and **separately available inside each handler**
(`lh.createSession`, `gh.createSessionForPR`) to do per-call validation or
error enrichment if desired — these are two different integration points
with different staleness properties (see §3).

## 3. Staleness: a startup-time enum can't see programs added later

If the fix fetches `ListProgramsConfig` once during `registerLifecycleTools`/
`registerGithubTools` (i.e., inside `NewCore`, at process boot) and bakes the
result into `mcpgo.Enum(...)`, that snapshot goes stale the instant a user
adds a custom program via `UpsertProgramConfig` (the "Program Configurations"
UI) while the server keeps running — which is the exact scenario in the
backlog item. Per §2, nothing re-registers MCP tools mid-process, so this
staleness cannot self-heal short of a full server restart.

This is a real gap, but bounded: it only affects *newly added custom*
programs between server starts, and the enum was never going to be a hard
gate anyway (see §4) — worst case with a stale enum is the same "no
guidance for this one specific value" experience the free-form option has
for *all* values, not a functional regression.

## 4. The enum is advisory only — not server-side enforced

Traced `mcpgo.Enum` (`mcp-go@v0.48.0/mcp/tools.go:1025-1028`): it only sets
the `"enum"` key in the tool's declared JSON input schema. Traced the
request path (`handleToolCall`, `mcp-go@v0.48.0/server/server.go:1556` on),
and the Go handler itself (`args["program"].(string)` — `server/mcp/tools_lifecycle.go`
handler body, ~line 132): **no code in mcp-go or in stapler-squad validates
an incoming tool call's arguments against the declared `enum`.** A
well-behaved MCP client (e.g. Claude Code) treats the enum as authoritative
when *constructing* a call and won't offer other values — that's the
"rejection" the backlog item describes — but nothing stops a client from
sending an arbitrary string, and the Go handler would accept it unchanged.
This reframes the fix as a **client-facing discoverability/schema-honesty
problem**, not a server-side access-control gap — relevant because it means
a startup-time-stale enum degrades gracefully (an agent can still type an
unlisted value; it just won't be offered/autocompleted).

## 5. Backend validation on an unknown program is not adequate for self-correction

Traced program resolution: `config.ResolveProgramConfig` (`config/defaults.go:211-227`)
calls `FindProgramConfig`, which only searches `cfg.SessionDefaults.Programs`
(**custom programs only — it does not consult `BuiltInPrograms()` at all**).
If no match is found (true for a typo, a hallucinated program name, *or
coincidentally for every valid built-in*, since built-ins aren't in that
list), it falls through to:

```go
return ResolvedProgram{Command: program, CLIFlags: "", EnvVars: nil, IsCustom: false}
```

i.e. it silently treats the raw string as a literal executable name. There
is **no "program not found" error at `CreateSession` time** — an
unregistered/misspelled program doesn't fail fast with a structured MCP
error the agent can read and correct from; it proceeds to spawn a tmux
session that then fails asynchronously at process-exec time with a
shell-level error (e.g. `bash: foo: command not found`), surfaced later and
less legibly than a same-call MCP error. `IsBuiltInProgram`
(`server/services/defaults_service.go:657-665`) exists but is never consulted
by `ResolveProgramConfig`, so it isn't part of today's create-session
validation path either.

**Conclusion for the "consistency requirements" question: no, existing
backend validation does not already produce an error path adequate for
self-correction.** A purely free-form/no-enum fix (Option 1 in
requirements.md) would satisfy AC1-3 but leave AC4 weak (relies on tool
*description* text alone) and would inherit this pre-existing "unknown
program silently exec'd as a literal command" gap — worth flagging to the
plan phase, though tightening `ResolveProgramConfig` itself is out of scope
per requirements.md's "Out of Scope" section (backend validation change).

## 6. No existing MCP-level program-discovery tool

Checked `server/mcp/tools_discovery.go` and `tools_workflow.go`: neither
exposes `ListProgramsConfig` today. An agent using only MCP tools currently
has zero way to discover valid program values short of the enum text itself
or the tool description — reinforcing AC4's requirement that *something*
(enum or description) carry that information.

## Recommendation

**Introduce a small shared helper, not "extend as-is."** Duplicating the
"fetch programs via `svc`, build `mcpgo.Enum(...)`" logic independently in
`tools_lifecycle.go` and `tools_github.go` would recreate the exact
two-file-drift bug this item exists to fix — the project's own precedent
(`enumNamesWithoutPrefix` + `notificationTypeNames`, §1) already establishes
that enums derived from a canonical source, not hand-typed literals, are the
house style here. Concretely: a helper (e.g. `programEnumOption(svc
*services.SessionService) mcpgo.PropertyOption`, callable from both
`registerLifecycleTools` and `registerGitHubTools`, both of which already
receive `svc` at their single shared construction site in `NewCore`) that
calls `svc.ListProgramsConfig` once at registration time and returns
`mcpgo.Enum(ids...)`. This is a same-file-sized addition (one function, a
few lines), not a new module or abstraction layer — fits comfortably inside
"small, contained fix" framing, no hotspot/architecture-review escalation
warranted for `server/mcp/` itself (registration code, no elevated
complexity or churn observed in the files touched).
