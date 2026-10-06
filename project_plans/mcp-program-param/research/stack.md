# Stack Research: mcp-program-param

## `mark3labs/mcp-go`

- Pinned at `github.com/mark3labs/mcp-go v0.48.0` (`go.mod:171`).
- Import aliases: `mcpgo "github.com/mark3labs/mcp-go/mcp"` (tool/schema builders) and
  `mcpserver "github.com/mark3labs/mcp-go/server"` (the `*MCPServer` type), both aliased
  identically across every `server/mcp/*.go` file.

### Dynamic enum construction — already supported, no new dependency needed

`mcpgo.Enum` is declared as:

```go
// mcp-go@v0.48.0/mcp/tools.go:1025
func Enum(values ...string) PropertyOption
```

A variadic `...string`, so it accepts a spread slice (`mcpgo.Enum(names...)`) built at
tool-registration time from any Go slice — no static-literal restriction. `WithString`
(`mcp/tools.go:1165`) composes any number of `PropertyOption`s, so `Enum` built from a
computed slice combines with `Description`, `DefaultString`, etc. exactly like the
hardcoded two-value case. **No new dependency is required** — this is achievable with
the existing `mcpgo` import plus a call to the already-injected `*services.SessionService`.

### Updating a tool's schema after registration — supported, but gated

`*mcpserver.MCPServer` exposes (`mcp-go@v0.48.0/server/server.go`):

- `AddTool(tool, handler)` → `AddTools(ServerTool{...})` (`:741`, `:790`)
- `SetTools(tools ...ServerTool)` — wipes and replaces the whole tool map (`:836`)
- `DeleteTools(names ...string)` (`:868`)

`AddTools` unconditionally overwrites `s.tools[name] = entry` (`:801`) — re-registering a
tool with the same name replaces its schema in place, so **runtime updates to the enum
are technically possible** (e.g. re-running `registerLifecycleTools` after a
`UpsertProgramConfig` call). Whether a *connected* client sees the change without
reconnecting depends on the `tools.listChanged` capability: `AddTools` only fires
`mcp.MethodNotificationToolsListChanged` to already-initialized sessions when
`s.capabilities.tools.listChanged` is `true` (`:805-808`). This repo's `NewCore`
(`server/mcp/server.go:47`) constructs the server with
`mcpserver.WithToolCapabilities(false)` — list-changed notifications are explicitly
disabled, and `implicitlyRegisterToolCapabilities` (`:749-756`) is written to *not*
override an explicit `false`. So today, even if the schema is updated in-process, no
notification goes out. This is orthogonal to the requirements' acceptance criteria (which
only require *build-time* enum accuracy, not live propagation to open MCP client
sessions) — flagging it for the plan phase in case "programs added while the server is
already running" needs to be handled, but it's not blocking: **tool definitions are
built once, at server startup, from whatever `ListProgramsConfig` returns at that
moment** — matching how every other registration function in this file already works
(no tool is currently rebuilt after `NewCore` returns).

## Tool registration path (`server/mcp/server.go`, `NewCore`)

`NewCore(store, svc *services.SessionService, sbMgr, storage, eventBus, prCache, ...)`
(`server/mcp/server.go:34-88`) builds one `*mcpserver.MCPServer` and calls each
`register*Tools` function once, synchronously, before returning it. Relevant call sites:

```go
registerLifecycleTools(s, &lifecycleHandlers{store: store, svc: svc})       // :53
...
if prCache != nil {
    registerGitHubTools(s, &githubHandlers{cache: prCache, store: store, svc: svc}) // :87
}
```

Both `lifecycleHandlers` (`server/mcp/tools_lifecycle.go:35-38`) and `githubHandlers`
(`server/mcp/tools_github.go:22-30`) already carry `svc *services.SessionService` —
the same field used for other RPCs (e.g. `githubHandlers.svc` is already used to route
`create_session_for_pr` through the async creation pipeline). `svc` may be `nil` in some
test/fallback-transport constructions (see `githubHandlers.svc`'s doc comment and
`tools_github_test.go:241`), so any `ListProgramsConfig` call at registration time needs
a nil-guard, consistent with the existing `if svc != nil { ... }` gating pattern already
used around lines 68-73 of `server.go` for other `svc`-dependent tool registrations.

`svc.ListProgramsConfig` is a thin passthrough:

```go
// server/services/session_service.go:5322-5324
func (s *SessionService) ListProgramsConfig(ctx context.Context, req *connect.Request[sessionv1.ListProgramsConfigRequest]) (*connect.Response[sessionv1.ListProgramsConfigResponse], error) {
    return s.defaultsSvc.ListProgramsConfig(ctx, req)
}
```

which delegates to `DefaultsService.ListProgramsConfig` (`server/services/defaults_service.go:684-703`):

```go
func (d *DefaultsService) ListProgramsConfig(ctx context.Context, req *connect.Request[...]) (*connect.Response[...], error) {
    cfg := config.LoadConfig()
    builtIns := BuiltInPrograms()
    result := make([]*sessionv1.ProgramConfigProto, 0, len(builtIns)+len(cfg.SessionDefaults.Programs))
    for _, p := range builtIns { result = append(result, programConfigToProto(p, true)) }
    for _, p := range cfg.SessionDefaults.Programs { result = append(result, programConfigToProto(p, false)) }
    return connect.NewResponse(&sessionv1.ListProgramsConfigResponse{Programs: result}), nil
}
```

This is a cheap, synchronous, side-effect-free call (`config.LoadConfig()` + two slice
builds) — safe to call once at `NewCore`/tool-registration time with `context.Background()`
and an empty `&sessionv1.ListProgramsConfigRequest{}`. Each returned
`ProgramConfigProto` has `.Id` (the enum value to surface) and `.Label`/`.Description`
(usable for the tool's description text, satisfying AC 4's "guidance" requirement
alongside or instead of a literal enum).

## Precedent: existing dynamic/config-driven enum population

`registerNotificationTools` (`server/mcp/tools_notifications.go:152-181`) already does
exactly this shape of thing, just from a proto enum instead of a config service:

```go
// :27-33
// notificationTypeNames is the enum surface get_notification_history's
// type_filter argument accepts, without the NOTIFICATION_TYPE_ prefix.
// Derived from the generated NotificationType_name map (skipping the
// UNSPECIFIED sentinel) rather than hand-maintained, so it can't silently
// drift out of sync when the proto enum gains a new value.
var notificationTypeNames = enumNamesWithoutPrefix(
    sessionv1.NotificationType_name, notificationTypePrefix,
    sessionv1.NotificationType_NOTIFICATION_TYPE_UNSPECIFIED.String())
...
// :169
mcpgo.WithString("type_filter",
    mcpgo.Description("Filter to a single notification type, e.g. TASK_COMPLETE or NOTIFICATION_TYPE_TASK_COMPLETE"),
    mcpgo.Enum(notificationTypeNames...),
),
```

Two differences from the `program` case worth noting for the plan phase:
1. `notificationTypeNames` is a **package-level `var`** computed once at `init`/package-load
   time from a static proto enum map (no service call, no request context needed) — a
   slightly simpler case than `program`, which needs a live `*services.SessionService`
   call (config-file-backed, can change via `UpsertProgramConfig`) rather than a compile-time
   proto enum.
2. It's registered unconditionally in `registerNotificationTools`, which itself is only
   called `if svc != nil` (`server.go:74`) — the same nil-guard shape needed for
   `registerLifecycleTools`/`registerGitHubTools` if they gain a `svc`-derived enum
   (though today those two are registered unconditionally regardless of `svc` nilness,
   since `lifecycleHandlers`/`githubHandlers` tolerate a nil `svc` elsewhere — the new
   code will need its own nil check around the `ListProgramsConfig` call specifically,
   likely falling back to a static `claude`/`aider` enum or no enum at all when `svc`
   is nil, e.g. in tests).

No other `server/mcp/*.go` tool builds an enum from a live service/config call — every
other `mcpgo.Enum(...)` use (`server/mcp/server.go:163` status_filter, `tools_rules.go:37,40,41`
tool_category/decision/risk_level, `tools_workflow.go:51` session_type) is a hardcoded
literal list. `notificationTypeNames` is the only precedent for "build the `[]string`
programmatically," and a shared helper (e.g. a `programNames(ctx, svc) []string`
function) is a reasonable pattern to introduce and share between
`tools_lifecycle.go` and `tools_github.go`, per the requirements' note about "any shared
helper introduced to keep the two in sync."

## Summary of what needs to change (stack-only view, not a design)

- No new Go dependency. `mcpgo.Enum(...string)` already takes a variadic slice.
- `registerLifecycleTools`/`registerGitHubTools` already receive `svc *services.SessionService`
  in their handler structs; add a call to `svc.ListProgramsConfig(context.Background(), &connect.Request[sessionv1.ListProgramsConfigRequest]{})`
  (or a small wrapper) at registration time inside `NewCore`, guarded for `svc == nil`,
  to produce the `[]string` of program IDs passed into each tool's `program` field's
  `mcpgo.Enum(...)`.
- The two proto/schema options from the requirements doc (drop `Enum` entirely vs. populate
  it dynamically) are both mechanically supported by the pinned mcp-go version; nothing in
  the library forces one over the other.
