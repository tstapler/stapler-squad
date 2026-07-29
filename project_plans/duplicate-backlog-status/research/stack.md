# Stack Research: Duplicate Backlog Status

## 1. ent ORM — version, conditional-update support

- **Version**: `entgo.io/ent v0.14.5` (go.mod:8), Go module `1.25.0`.
- Generated ent code is **gitignored** (`.gitignore:22-27`: `session/ent/*.go` excluded, only `session/ent/generate.go` and `session/ent/schema/` are committed). It's produced by `go generate ./session/ent/`, which runs:
  ```
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
  ```
  (`session/ent/generate.go:3`) — matches the `--feature sql/upsert` flag CLAUDE.md calls out as required; using plain `ent generate` breaks `UpsertRule`.
- **`.Where()` on `UpdateOneID` builders IS supported** in this ent version. Confirmed directly in the vendored template source (`~/go/pkg/mod/entgo.io/ent@v0.14.5/entc/gen/template/builder/update.tmpl`):
  - Line 109: `func (u *XUpdateOne) Where(ps ...predicate.X) *XUpdateOne { u.mutation.Where(ps...); return u }` — the `UpdateOne` builder generates a `Where` method identical in shape to the plain `Update` builder's (line 30).
  - The SQL dialect template (`dialect/sql/update.tmpl:174-175`) converts a zero-affected-rows result from a mismatched predicate into `*sqlgraph.NotFoundError` → wrapped as ent's `*NotFoundError`. So a failed optimistic-concurrency predicate surfaces as `ent.IsNotFound(err) == true`, same error path already handled in the repo (see below).
  - **Practical pattern for FR4**:
    ```go
    item, err := r.client.BacklogItem.UpdateOneID(parsedID).
        Where(backlogitem.StatusEQ(string(currentExpectedStatus))).
        SetStatus(string(toStatus)).
        SetUserModifiedStatusAt(now).
        Save(ctx)
    if err != nil {
        if ent.IsNotFound(err) {
            // either the row doesn't exist OR the predicate didn't match (status changed concurrently)
            return nil, fmt.Errorf("%w: concurrent status change on backlog item %s", ErrPreconditionFailed, id)
        }
        ...
    }
    ```
    This distinguishes "row missing" from "row exists but predicate failed" only via a re-read if that distinction matters to callers — `ent.NotFoundError` doesn't differentiate the two cases itself.

- **Existing repo code does NOT yet use this pattern** — it's a gap the feature must fix, not follow. `session/ent_repository_backlog.go:274-321` (`TransitionBacklogItemStatus`) currently does:
  1. `r.client.BacklogItem.Get(ctx, parsedID)` (read)
  2. In-app comparison against `precondition.ExpectedStatus` / `ExpectedUpdatedAt` (app-level check)
  3. Unconditional `UpdateOneID(parsedID).SetStatus(...).Save(ctx)` (write, no `.Where()`)

  This is a classic **read-then-check-then-write race** (TOCTOU) — nothing stops two concurrent transitions between steps 1–3. FR4 ("persist status and duplicate_of_id atomically with optimistic-concurrency protection") should replace this with a single conditional `UpdateOneID(...).Where(backlogitem.StatusEQ(current.Status))...Save(ctx)` call so the concurrency check and the write happen as one SQL statement.
  - `backlogitem` package (generated) exposes `StatusEQ`, `StatusIn`, `StatusNotIn` predicate helpers already used elsewhere (e.g. `ListBacklogItems` at `session/ent_repository_backlog.go:138,140`), confirming the naming convention to use for the new predicate (`backlogitem.StatusEQ(...)`).
  - `duplicate_of_id` should be set in the same `UpdateOneID` chain (e.g. `.SetDuplicateOfID(...)`) so status + link update in one atomic statement.

- Filtering pattern to reuse for FR6 (exclude `duplicate` from default views): `ListBacklogItems` (`session/ent_repository_backlog.go:134-144`) already excludes terminal statuses via `backlogitem.StatusNotIn(string(BacklogStatusDone), string(BacklogStatusArchived))` when `filter.ExcludeTerminal` is set and no explicit `filter.Statuses` are given. Add `BacklogStatusDuplicate` to that `StatusNotIn(...)` list (and decide whether it belongs in "terminal" semantics generally, since duplicate is functionally terminal like archived/done).

## 2. buf / protoc-gen-go / protoc-gen-connect-go pipeline

- `buf.yaml` (v2): `modules: [proto]`, deps on `buf.build/googleapis/googleapis`, lint `STANDARD` + `enum_zero_value_suffix: _UNSPECIFIED`, breaking-change check on `FILE` scope (excluding `FIELD_SAME_NAME` since "first version, no breaking changes to check").
- `buf.gen.yaml` (v2), `managed.enabled: true`, `go_package_prefix: github.com/tstapler/stapler-squad/gen/proto/go`. Three plugins:
  1. `remote: buf.build/protocolbuffers/go` → `gen/proto/go` (protoc-gen-go, fetched from BSR, no pinned local version in repo — resolved by buf at generate time)
  2. `remote: buf.build/connectrpc/go` → `gen/proto/go` (protoc-gen-connect-go, likewise BSR-remote)
  3. `local: web-app/node_modules/.bin/protoc-gen-es` → `web-app/src/gen` (TS message types, `target=ts`, `ts_nocheck=false`, `keep_empty_files=true`)
- go.mod pins: `github.com/bufbuild/buf v1.57.2` (the buf CLI as a Go dependency), `connectrpc.com/connect v1.19.0`, `connectrpc.com/otelconnect v0.8.0`, `github.com/bufbuild/connect-go v1.10.0` (older/legacy connect-go, likely just transitively required — the actual RPC handlers use `connectrpc.com/connect`), plus BSR-generated indirect deps (`buf.build/gen/go/bufbuild/...`).
- **`make proto-gen`** target (Makefile, confirmed near the referenced line range): builds are gated through `ensure-tools` (asdf-managed `buf`/`node`/`go` per `.tool-versions`), with a stamp file `PROTO_STAMP := .proto-gen.stamp` and `PROTO_OUT_DIRS := gen/proto/go web-app/src/gen` (Makefile:20-21) used to skip regeneration when nothing changed. The main `stapler-squad` build target depends on `proto-gen` (`Makefile:109: stapler-squad: ensure-tools proto-gen server/web/dist lint $(GO_FILES)`), so any proto schema edit for `duplicate_of_id` / `SessionType`-style enum changes will be picked up automatically by `make build`. CLAUDE.md's documented workflow (`make generate-proto`) matches this pipeline.

## 3. mark3labs/mcp-go — version and conventions

- **Version**: `github.com/mark3labs/mcp-go v0.48.0` (go.mod:125).
- Tool registration convention, confirmed from `server/mcp/tools_backlog.go` (existing tools: `get_backlog_item`, `report_progress`, `request_review`, `submit_review_verdict`, `submit_triage_result`, registered in `registerBacklogTools(s *mcpserver.MCPServer, h *backlogHandlers)` at line 541):
  ```go
  s.AddTool(
      mcpgo.NewTool("request_review",
          mcpgo.WithDescription("..."),
          mcpgo.WithString("item_id",
              mcpgo.Description("UUID of the backlog item"),
              mcpgo.Required(),
          ),
          mcpgo.WithString("message",
              mcpgo.Description("..."),
              mcpgo.Required(),
          ),
      ),
      h.requestReview,
  )
  ```
  Arrays of structured objects use `mcpgo.WithArray(name, mcpgo.Description(...), mcpgo.Required(), mcpgo.Items(map[string]any{ "type": "object", "properties": {...}, "required": [...] }))` — see `submit_review_verdict`'s `verdicts` param (lines 599-611).
- Handler signature: `func (h *backlogHandlers) <name>(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`, args pulled via `req.GetArguments()` (a `map[string]any`), success responses built with `mcpgo.NewToolResultText(...)`.
- **`mark_duplicate` tool plan**: needs `item_id` (string, required, UUID) + `duplicate_of_id` (string, required, UUID) params, following the exact `WithString(...).Description(...).Required()` idiom above; handler should call the (to-be-added) atomic transition+link repo method and return a `NewToolResultText` confirmation (or an error result if the target ID doesn't exist / is self-referential — surface `ErrPreconditionFailed`/`ErrNotFound` distinctly, matching existing handler error-mapping conventions in the same file).

## 4. Frontend — vanilla-extract version & theme token contract

- **`@vanilla-extract/css`**: `^1.20.1`; also `@vanilla-extract/recipes ^0.5.7`, `@vanilla-extract/next-plugin ^2.5.1` (web-app/package.json:56,122-123).
- Theme contract **already exists**: `web-app/src/styles/theme-contract.css.ts` (created via `createThemeContract`, referenced as `vars`) and `web-app/src/styles/theme.css.ts` (uses `createTheme(vars, ...)`, re-exports `vars`, `breakpoints`, `zIndex` from the contract). This matches `.claude/rules/css-architecture.md`'s guidance — no need to create the contract from scratch.
- Status-badge-specific token slot already present in the contract: `vars.statusBadge` (`theme-contract.css.ts:93-113`) with per-status `{name}Bg / {name}Fg / {name}Border` triples for `approval`, `input`, `complete`, `uncommitted`, `idle` (+ `staleFg`, `processingBg/Fg/Border`). **No `duplicate` slot exists yet** — must be added here first (per the rule: "If you need a token that doesn't exist yet, add it to `globals.css` [here: theme-contract.css.ts] first, then reference it").
- Current status badge styling lives in `web-app/src/components/backlog/BacklogItemBadge.css.ts` — one `style()` export per status (`statusIdea`, `statusReady`, `statusInProgress`, `statusReview`, `statusDone`, `statusArchived`, `statusRefining`), each pulling colors from either generic `vars.color.*` tokens (surfaceMuted/textMuted/borderMuted for idea/archived, warningBg/warningText/warning for refining) or the dedicated `vars.statusBadge.*` triples (ready/inProgress/review/done). **New `statusDuplicate` export needed**, most naturally backed by a new `vars.statusBadge.duplicateBg/duplicateFg/duplicateBorder` triple (to get a visually distinct badge color, consistent with how `ready`/`inProgress`/`review`/`done` each get dedicated triples rather than reusing generic `color.*` tokens) — populated with actual light/dark values in `theme.css.ts`.

## Summary of key file:line references for planning

| Concern | File:Line |
|---|---|
| ent version / generate command | `go.mod:8`, `session/ent/generate.go:3` |
| ent gitignore rule | `.gitignore:22-27` |
| `UpdateOneID.Where()` template proof | `~/go/pkg/mod/entgo.io/ent@v0.14.5/entc/gen/template/builder/update.tmpl:109` |
| NotFoundError-on-predicate-miss proof | same module, `dialect/sql/update.tmpl:174-175` |
| Current (non-atomic) transition impl | `session/ent_repository_backlog.go:274-321` |
| Existing `StatusNotIn` filter pattern | `session/ent_repository_backlog.go:134-144` |
| buf config | `buf.yaml`, `buf.gen.yaml` |
| buf/connect versions | `go.mod:6,7,12,13` |
| proto-gen Make target wiring | `Makefile:20-21,109` |
| mcp-go version | `go.mod:125` |
| MCP tool registration examples | `server/mcp/tools_backlog.go:541-632` |
| vanilla-extract versions | `web-app/package.json:56,122-123` |
| Theme contract | `web-app/src/styles/theme-contract.css.ts` (statusBadge block: lines 93-113) |
| Theme values (light/dark) | `web-app/src/styles/theme.css.ts` |
| Existing status badge styles | `web-app/src/components/backlog/BacklogItemBadge.css.ts` |
