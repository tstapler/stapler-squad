# Build vs. Buy — mcp-program-param

Scope reminder: this is a 1-2 line schema change to two already-adopted
`mcpgo.WithString(..., mcpgo.Enum(...))` call sites. The build-vs-buy lens is
checked for completeness, not because a "buy" option was expected to exist.

## 1. Existing OSS library/framework

Checked `mark3labs/mcp-go v0.48.0` (`go.mod:171`, vendored at
`~/go/pkg/mod/github.com/mark3labs/mcp-go@v0.48.0/mcp/tools.go`) for any
built-in "enum from a runtime source" helper that the codebase isn't using.

- `Enum(values ...string) PropertyOption` (`tools.go:1025`) just writes
  `schema["enum"] = values`. It's variadic, but a `[]string` can be spread
  into it (`mcpgo.Enum(list...)`) — this is a normal Go call-site pattern, not
  a distinct API. The codebase already does exactly this
  (`server/mcp/tools_notifications.go:169`,
  `mcpgo.Enum(notificationTypeNames...)`).
- No `EnumFunc`, no lazy/deferred-provider option, no per-request
  re-evaluation hook anywhere in `tools.go`. `WithStringEnumItems([]string)`
  (`tools.go:1363`) is the only other enum-shaped helper, and it's scoped to
  array *item* schemas (`WithArray("priority", WithStringEnumItems(...))`),
  not applicable to a top-level string property like `program`.
- Tool schemas are built once, at `mcpgo.NewTool(name, opts...)` call time,
  via functional options over a `map[string]any`. There is no re-entrant
  schema-building hook in the library — "populate dynamically" necessarily
  means "call `Enum(currentList...)` at whatever point this repo already
  calls `NewTool` for `create_session`/`create_session_for_pr`," not
  anything mcp-go provides for free.

**Verdict: Not recommended (no adoption target exists).** Nothing to buy;
confirmed rather than assumed.

## 2. SaaS/managed API

Not applicable — this is a local schema-validation concern inside a Go
binary embedding an MCP server over stdio (`server/mcp/server.go:1-2`).
There's no third-party service boundary to delegate "which program names are
valid" to. **Verdict: Not applicable.**

## 3. LLM-generated implementation vs. battle-tested library / existing pattern

Not applicable in the algorithm/data-structure sense (no computation to get
wrong). Reframed as asked: is hand-rolling a new "config → enum" helper
low-risk, or is there a shared helper already doing this that should be
reused instead?

- Precedent found: `enumNamesWithoutPrefix` + `notificationTypeNames`
  (`server/mcp/tools_notifications.go:27-49`) — a package-level `var`
  derived once from `sessionv1.NotificationType_name` (a **generated proto
  enum map**, known at compile time), spread into `mcpgo.Enum(...)` at
  registration. Its own doc comment states the rationale directly: "Derived
  from the generated ... map ... rather than hand-maintained, so it can't
  silently drift out of sync."
- This precedent doesn't transfer cleanly: `program`'s valid values
  (`BuiltInPrograms()` + `cfg.SessionDefaults.Programs`, from
  `ListProgramsConfig`, `server/services/defaults_service.go:684`) are
  **runtime config, not a compile-time proto enum** — they change when a user
  calls `UpsertProgramConfig`. A `var` initialized once at package-init can't
  see that. The closer-fitting shape is: build the enum value list inside
  `registerLifecycleTools`/`registerGithubTools` at the point they're called,
  since `server/mcp/server.go:34-53`'s `NewCore` (which calls both) already
  takes `svc *services.SessionService` and is re-invoked fresh per MCP
  server instance (stdio subprocess per session launch, per its own doc
  comment) — so a `config.LoadConfig()`-backed list picked up at that call
  reflects programs registered before that session was spawned, same
  freshness envelope `ListProgramsConfig` itself offers callers.
- Counter-precedent (why a *shared* helper, not two independent call sites,
  matters): `server/mcp/tools_backlog.go:600` has a comment flagging that an
  `eventTypeFilter` assignment and its sibling `mcpgo.Enum(...)` registration
  (line 3038) must be kept in sync by hand — an existing example of exactly
  the drift risk the requirements doc's AC6 asks this fix to avoid across
  `tools_lifecycle.go`/`tools_github.go`.

**Verdict: Recommended** to write one small shared helper (e.g.
`programEnumOption()` returning a `mcpgo.PropertyOption`, or a plain
`[]string` getter) called from both `tools_lifecycle.go:59` and
`tools_github.go:101-103`, following the `enumNamesWithoutPrefix` shape but
sourced from `ListProgramsConfig`'s config-loading path instead of a proto
map. Hand-rolling two independent copies is the one thing to avoid — the
`tools_backlog.go:600` comment is a live example of that drift already
existing elsewhere in this package.

## 4. Fork or adapt (prior art in this repo's history)

```
git log --oneline -- server/mcp/tools_lifecycle.go server/mcp/tools_github.go
```

20 commits total, most unrelated (session-path refactors, GHE-awareness,
race fixes). Nothing touches the `program` parameter's enum specifically.
Closest related commit: `94d4e4f92 docs(mcp): clarify create_session is for
user-interactable sessions only` — a description-text change, not a schema
change, and not a precedent for widening an enum.

No prior attempt at this exact fix exists to fork or follow the shape of;
the `notificationTypeNames` pattern (item 3 above) is the best available
in-repo template, adapted for a config-backed rather than proto-backed
source list. **Verdict: Not applicable** (nothing to fork); use the
notifications pattern as the template instead.

## Summary

| # | Option | Verdict |
|---|--------|---------|
| 1 | Adopt an mcp-go dynamic-enum feature | Not recommended — doesn't exist |
| 2 | SaaS/managed API | Not applicable |
| 3 | Hand-roll vs. reuse existing pattern | Recommended — one shared helper, `notificationTypeNames`-shaped but config-sourced, called from both tool files |
| 4 | Fork/adapt prior fix | Not applicable — no prior attempt; use #3's pattern as template |

Net: build, small, following the `enumNamesWithoutPrefix`/`notificationTypeNames`
precedent rather than inventing a new shape — confirms the requirements doc's
option 2 (dynamic enum from `ListProgramsConfig`) is buildable with the
adopted library as-is, with a shared helper (not two hand-rolled copies) to
prevent the same drift risk already visible at `tools_backlog.go:600`.
