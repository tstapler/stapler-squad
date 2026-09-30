# Build vs. Buy: backlog-session-item-linking

## 1. Existing OSS library / framework for session↔task binding

**Options considered:** LangGraph checkpointing (thread-ID-keyed state), AutoGen conversation
handoff, Temporal workflow-to-task binding.

**Pros of adopting one:** These frameworks solve a genuinely similar-sounding problem —
associating a long-running agent execution with a unit of work — and are battle-tested at scale.

**Cons:**
- All three are full orchestration frameworks (LangGraph = graph-based agent runtime, AutoGen =
  multi-agent conversation framework, Temporal = distributed workflow engine). Adopting any one
  to get "session-to-task binding" means adopting its entire execution model — this repo already
  has its own session lifecycle (`session/instance.go`, tmux + git worktrees), its own ORM (ent,
  `session/ent/`), its own RPC layer (ConnectRPC, `server/services/`), and its own MCP tool
  registration pattern (`server/mcp/tools_backlog.go`). None of these frameworks would replace
  that plumbing; they'd sit alongside it, duplicating concepts (LangGraph's checkpoint store vs.
  this repo's `item_sessions` ent table; Temporal's workflow ID vs. this repo's session UUID).
- The actual binding primitive — `BacklogService.AttachSessionToItem`
  (`server/services/backlog_service_sync.go:29`) — already exists, is unit-tested
  (`server/services/backlog_service_test.go:1659`,
  `server/services/backlog_service_triage_test.go:2022`), and already handles the two hard parts
  (creating the `ItemSession` DB record and regenerating slash commands via
  `session.WriteSlashCommands` at `backlog_service_sync.go:94`). What's missing is purely an MCP
  tool *handler* that calls this method — estimated 30-60 lines following the `report_progress`
  template (`server/mcp/tools_backlog.go:259-333`, 74 lines including its full arg-validation,
  linkage-check, and response-formatting boilerplate).
- Introducing a new framework's dependency graph (LangGraph/AutoGen are Python; Temporal requires
  a server component) into a Go monolith is a much larger lift than the feature it would replace.

**Verdict: Not recommended.** No external library reduces code here — the internal method call
is already smaller than the glue code needed to bridge to any of these frameworks. This is
correctly assessed as "tightly coupled to this repo's own ent schema / ConnectRPC / MCP server"
and should be bespoke, matching the pattern of every other tool in `tools_backlog.go`.

## 2. SaaS / managed API

Not applicable. This is an in-repo agent harness feature operating entirely on local state (a
SQLite-backed ent DB, local git worktrees, a local MCP stdio server). There is no external data
or third-party service to integrate — session↔item linkage is a first-party concept with no
external system of record. No further evaluation needed.

## 3. Branch-name parser: bespoke regex vs. an existing canonical construction function

**Key finding: the assumed branch-name format in the requirements does not match the actual
convention, which changes the recommended design.**

The requirements describe resolving "which item does my branch belong to" via a parser for
`backlog/triage-<session-uuid>-<item-id>-<slug>`-shaped branch names. Tracing the actual branch
construction path shows this format is wrong:

- Branches are created by `CreateBacklogWorktree` (`session/instance_worktree.go:103`), which
  builds the branch name as `"backlog/" + branchSuffix` (line 108).
- `branchSuffix` is `slugify(baseTitle)` (`server/services/backlog_service_triage.go:720`), and
  `baseTitle` is `repoName + "-" + triageShortTitle(...)` (line 707) — a **title-derived slug**,
  not an ID-derived one. Neither the session UUID nor the backlog item ID appears anywhere in the
  branch name. `slugify` (`backlog_service_triage.go:326`) only emits `[a-z0-9-]`, so a 36-char
  UUID would in principle survive slugification if it were included — but it isn't part of the
  input string at all.
- Consequence: a regex/string-split parser looking for embedded session-uuid/item-id tokens in
  the branch name **cannot work** against the real data, because those tokens were never written
  into the branch name to begin with.

**The actual mirror-image function this "parser" should target doesn't exist because it isn't
needed** — the repo already has a direct DB lookup that answers the same question without
touching the branch name at all: `Storage.GetItemSessionBySessionUUID(ctx, sessionUUID)`
(`session/storage.go:933`, backed by `EntRepository.GetItemSessionBySessionUUID`,
`session/storage_backlog.go:185`). Since the calling session's UUID is already available via
`callerSessionUUID(ctx)` (`server/mcp/tools_backlog.go:36`, the same helper every other tool in
this file uses), "which item does my branch belong to" reduces to "which item is my session UUID
linked to" — a single ent query already used elsewhere (`tools_backlog.go:301`, `:662`), not a
new parser.

**Pros of writing a bespoke parser anyway:** none identified — it would need to parse a format
that doesn't exist in the current branch-naming scheme, and would be redundant with an existing,
already-tested DB query answering the identical question more reliably (works even if the branch
was renamed or the item was relinked to a differently-named branch).

**Cons:** a parser is one more thing to keep in sync if branch naming ever changes (it already
has — `backlog_service_triage.go:710-714`'s comment notes the slug is deliberately kept stable
across reopens specifically so worktree/branch reuse works, i.e. this format is actively
maintained and could change again without warning to any external parser).

**Verdict: Not recommended as scoped.** Do not build a branch-name parser. Recommend the plan
phase re-scope Goal 3 to use `Storage.GetItemSessionBySessionUUID` (a session-UUID → item lookup,
already implemented and tested) instead of a branch-name-based lookup. This should be flagged
back through requirements/plan as a correction, not implemented as originally described — the
premise ("branch encodes session-uuid + item-id") does not hold for this codebase.

## 4. Fork or adapt an existing MCP tool as a template

`server/mcp/tools_backlog.go` has five existing tools with a uniform shape: feature-disabled
check → `callerSessionUUID(ctx)` → arg extraction/validation via `validateUUID` → a linkage check
via `h.storage.GetItemSessionBySessionAndItem` → delegate to the underlying service/storage call
→ `mcpgo.NewToolResultText(...)` or `errResult(code, message, remediation)`.

- **`report_progress`** (`tools_backlog.go:259-333`) is the closest template by shape: single
  `item_id` UUID arg plus one additional typed arg, an existing-linkage guard, then a
  storage-layer call and a plain text success response. This is the template called out in the
  existing `research/stack.md` (line 43) and remains the best fit for a `link_session_to_item`
  tool — the new tool's linkage check is inverted (allow *unlinked* callers through, since the
  point of the tool is to create the link) but otherwise the control flow matches exactly.
- **`report_pr_created`** (`tools_backlog.go:623-728`) is a reasonable second reference for how
  to handle a call that mutates state on the underlying service (rather than just updating a
  storage field) and returns richer response data, but has more incidental complexity (GitHub
  cross-verification, role gating) not relevant to this feature.

**Pros of copying `report_progress`'s structure wholesale:** consistent error codes/shapes across
every tool in the file (important given Goal 2 explicitly wants actionable `PERMISSION_DENIED`
errors — reusing the existing `errResult(code, message, remediation)` signature, adding the
remediation string that's currently empty at 5 call sites listed in `research/stack.md:54`, is
the direct mechanism for that goal); no new patterns for a reviewer to learn; the interface-scoping
approach (`SessionAttacher`, narrow one-method interface per `.claude/rules/interface-pollution-checklist.md`)
is also already demonstrated by `ReviewCompletionSignaler`/`ReviewTrigger` in this same file
(`tools_backlog.go:77`, `:85`).

**Cons:** none found — this is a same-file, same-package, same-conventions addition; there is no
tension between "adapt the template" and "do it well" here.

**Verdict: Recommended.** Adapt `report_progress`'s structure directly for the new
`link_session_to_item` tool, and use `report_pr_created`'s error-remediation strings as the
pattern for filling in the currently-empty remediation arg at the 5 existing bare
`PERMISSION_DENIED` call sites (Goal 2).

## Summary table

| Option | Verdict |
|---|---|
| External agent-orchestration library (LangGraph/AutoGen/Temporal-style) | Not recommended |
| SaaS / managed API | N/A |
| Bespoke branch-name parser | Not recommended — premise doesn't match actual branch-naming; use existing `GetItemSessionBySessionUUID` DB lookup instead |
| Fork/adapt `report_progress` as the MCP tool template | Recommended |
