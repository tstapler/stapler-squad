# Research: Stack & Patterns for Backlog Status Transitions (duplicate/closed)

## 1. `BacklogStatus` type — `session/domain/backlog.go`

Plain string-backed enum, no `IsValid()` method exists yet (unlike `StuckReason`,
`AcStatus`, `BacklogCategory`, `ReviewOutcome`, which all have one at lines
153/175/203/267). Adding a new status value is idiomatic:

```go
// session/domain/backlog.go:16-24
type BacklogStatus string

const (
	BacklogStatusIdea       BacklogStatus = "idea"
	BacklogStatusRefining   BacklogStatus = "refining"
	BacklogStatusReady      BacklogStatus = "ready"
	BacklogStatusQueued     BacklogStatus = "queued"
	BacklogStatusInProgress BacklogStatus = "in_progress"
	BacklogStatusReview     BacklogStatus = "review"
	BacklogStatusPRPending  BacklogStatus = "pr_pending"
	BacklogStatusDone       BacklogStatus = "done"
	BacklogStatusArchived   BacklogStatus = "archived"
	// candidate addition: BacklogStatusClosed BacklogStatus = "closed"
)
```

There is **no existing `BacklogStatus.IsValid()`** — validity today is enforced
implicitly via the `validTransitions` map (`CanTransitionBacklog`, line 391) which
returns `false` for any status not present as a map key/value. Adding a new terminal
value requires:

1. A new `const` (as above).
2. A new entry in `validTransitions` (line 331) listing which source statuses may
   transition into it — per requirements AC2, at minimum `in_progress`, `review`,
   `pr_pending` need an added target edge to the new status. The terminal status
   itself should map to an **empty** transition set (like `BacklogStatusArchived` only
   allows going back to `idea` at line 385-387 — for a stricter "duplicate is truly
   terminal" semantics, the new status's map entry could have zero allowed exits, or
   allow only a manual reopen-to-idea escape hatch mirroring `archived`'s pattern).
3. If precondition-guarded transitions are added (see `TransitionGuard`, line 445),
   decide whether marking duplicate needs a guard (probably not — it's a
   self-certified terminal exit, not a quality gate like done/review).
4. Nothing in `domain/backlog.go` requires a formal `IsValid()` for this feature, but
   since none of the three source statuses currently transition into `archived` and
   the tool need only check `CanTransitionBacklog(from, newStatus)`, that function
   already generalizes correctly once the map is updated.

## 2. MCP tool registration pattern — `server/mcp/tools_backlog.go`

**Library**: `github.com/mark3labs/mcp-go v0.48.0` (go.mod:132), imported as:
```go
mcpgo "github.com/mark3labs/mcp-go/mcp"
mcpserver "github.com/mark3labs/mcp-go/server"
```

**Registration** happens in one central function, `registerBacklogTools(s
*mcpserver.MCPServer, h *backlogHandlers)` (line 920), which calls `s.AddTool(...)` once
per tool — declarative schema via `mcpgo.NewTool(name, mcpgo.WithDescription(...),
mcpgo.WithString(...)/WithNumber(...)/WithArray(...), ...)` followed by the bound
handler method. A new tool (e.g. `mark_duplicate`) is registered the same way:

```go
s.AddTool(
    mcpgo.NewTool("mark_duplicate",
        mcpgo.WithDescription("..."),
        mcpgo.WithString("item_id", mcpgo.Description("UUID of the backlog item"), mcpgo.Required()),
        mcpgo.WithString("reason", mcpgo.Description("..."), mcpgo.Required()),
        mcpgo.WithString("reference_url", mcpgo.Description("optional superseding PR/issue URL")),
    ),
    h.markDuplicate,
)
```

**Handler shape** — every handler is a method `func (h *backlogHandlers) xxx(ctx
context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`. The
canonical sequence, taken from `requestReview` (line 337) and `reportPRCreated` (line
623), that a new `mark_duplicate`-style tool should follow:

1. `featureDisabledResult(h.enabledCheck)` early-out.
2. `callerSessionUUID(ctx)` → `errResult(ErrPermissionDenied, ...)` if unset
   (`STAPLER_SESSION_UUID` env var check).
3. Parse/validate `args := req.GetArguments()` — required `item_id` (+ `validateUUID`),
   required free-text fields with a max-length guard (`message`/`reason` capped at
   2000 chars in `requestReview`).
4. **Link check**: `h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)`
   — `session.ErrNotFound` → `errResult(ErrPermissionDenied, "this session is not
   linked to the specified backlog item", "")`. This directly satisfies AC3's
   "unlinked session" rejection.
5. **Role check**: `reportPRCreated`'s pattern (line 623+) —
   `if itemSession.Role != session.SessionRoleWork { return errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only 'work' role may ...", itemSession.Role), "") }`.
   `SessionRoleWork = "work"` is defined in `session/backlog.go:50`. This directly
   satisfies AC3's "wrong role" rejection.
6. Load the item (`h.storage.GetBacklogItem`) if the target status depends on current
   state, or just build a `BacklogItemPrecondition{ExpectedStatus: string(fromStatus)}`
   — but note AC2 requires the new tool to work from **any** of three different source
   statuses. `requestReview`/`reportPRCreated` both hardcode a single expected
   `ExpectedStatus`, so a new tool needs one of:
   - Fetch the item first, use its *current* status as `ExpectedStatus` in the CAS
     precondition (optimistic-CAS-against-current-value pattern), or
   - Loop/try the transition without a precondition and rely on
     `CanTransitionBacklog(from, to)` as the sole guard (transition validity, not
     concurrency safety).
   The former is safer against races and consistent with the CAS pattern used
   everywhere else in this file (`TransitionBacklogItemStatus` always takes a
   precondition parameter).
7. Call `h.storage.TransitionBacklogItemStatus(ctx, itemID, targetStatus,
   precondition, session.TriggeredBySystem)` (signature at `session/storage.go:736`,
   thin wrapper over `EntRepository.TransitionBacklogItemStatus` at
   `session/ent_repository_backlog.go:869`). This internally calls
   `recordStatusEvent` (`ent_repository_backlog.go:45`) which appends the
   `BacklogStatusEvent` row — this is the existing audit mechanism AC1/AC3
   (requirements) call for; no new audit code path needed, just pass the
   reason/reference through as the event's `note` (the `Note` field already exists on
   `BacklogItemPrecondition`, e.g. `precondition := &BacklogItemPrecondition{ExpectedStatus: ..., Note: note}`
   used at `backlog_lifecycle.go:3333`).
8. Log via `log.InfoLog.Printf("[mcp:mark_duplicate] ...")` matching the
   `[mcp:request_review]` / other tool log-tag convention.
9. Return `mcpgo.NewToolResultText(...)` on success or `errResult(ErrInternalError,
   ...)` on transition failure.

Error helper constants (`ErrPermissionDenied`, `ErrInvalidArgument`,
`ErrInternalError`) and `errResult(...)` are already defined/used throughout this file
— reuse them, no new error taxonomy needed.

## 3. ent schema — no migration required

`session/ent/schema/backlog_item.go:35`:
```go
field.String("status").
```
`status` is a **plain `field.String`**, not an ent-level enum (`field.Enum(...)`)
would have needed the ent code generator to be rerun with `--feature sql/upsert`
per `.claude/rules/ent-schema-generation.md`). Because it's a bare string column,
**adding a new `BacklogStatus` value requires zero ent schema changes and zero
migration** — the new string value is just written/read like any other status string.
Indexes referencing `status` (lines 142-146: `index.Fields("status", "priority")`,
`index.Fields("status", "updated_at")`, `index.Fields("status", "queued_at")`,
`index.Fields("status")`) are structural (column-level) and already cover any new
string value with no changes.

If a separate `close_reason` enum field is added per the Notes-for-Downstream-Phases
suggestion (`duplicate`/`superseded`/`obsolete`/`wont_fix`), *that* would need a new
`field.String("close_reason").Optional()` (or `field.Enum` if DB-level constraint is
wanted) added to `backlog_item.go`, followed by the ent codegen command from
`session/ent/generate.go` (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature
sql/upsert ./session/ent/schema`) and a real migration, since this *is* a new column.
The status value itself, however, needs no such step — only a reason/reference field
would.

## 4. Versions / dependencies

- `github.com/mark3labs/mcp-go v0.48.0` (go.mod:132) — MCP tool/server library.
- `entgo.io/ent v0.14.5` (go.mod:8) — ORM; codegen command is pinned in
  `session/ent/generate.go`, always include `--feature sql/upsert` (breaks
  `UpsertRule`/upsert methods otherwise — see `.claude/rules/ent-schema-generation.md`).
- No new dependency is needed for this feature — new status value, new MCP tool, and
  audit trail all reuse existing library versions and existing data-access code paths.

## 5. Reconciliation loops that enumerate "active" statuses — `session/backlog_lifecycle.go`

Confirmed several places hardcode the "in-flight" status universe as
`in_progress`/`review`/`pr_pending`, all of which will need the new terminal status
excluded (per requirements item 4 / AC4):

- Line 1264: `Statuses: []string{string(BacklogStatusInProgress)}` (stale-work stuck
  detector).
- Line 2063: `Statuses: []string{string(BacklogStatusInProgress)}`.
- Line 2545: `Statuses: []string{string(BacklogStatusPRPending)}` (PR-pending-no-PR
  stuck detector).
- Line 2618: `Statuses: []string{string(BacklogStatusReview)}`.
- Line 2799: `Statuses: []string{string(BacklogStatusInProgress), string(BacklogStatusReview)}`.
- Line 3432: explicit terminal/near-terminal check —
  `item.Status == string(BacklogStatusPRPending) || item.Status == string(BacklogStatusDone) || item.Status == string(BacklogStatusArchived)`
  — this is the kind of exhaustive list the new status must be added to wherever it
  represents "already resolved, don't touch."
- Lines 2997-3011: a switch/if-chain computing `resolve` by comparing `row.ItemStatus`
  against `BacklogStatusPRPending`/`Review`/`InProgress` — needs audit to confirm the
  new terminal status is treated as "already resolved" (`resolve = true`) rather than
  falling through un-handled.
- `PRFixSpawner` (per requirements, `backlog_lifecycle.go:49`) and
  `recoverDriftedPRItem` (`backlog_lifecycle.go:3363`, confirmed:
  `l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusPRPending, precondition, TriggeredBySystem)`)
  are system-mediated reopen/CAS-transition examples — useful prior art for how a new
  work-session-initiated transition should be structured, but these specific call
  sites don't need to *touch* the new status themselves; they just illustrate the CAS
  pattern.

This file is large (4000+ lines) and the grep above is not exhaustive of every branch
— `sdd:3-plan`/implementation should re-grep
`BacklogStatusInProgress\|BacklogStatusReview\|BacklogStatusPRPending` across the full
file before finalizing the touch list, and cross-check `SessionRoleWork` role-narrowing
logic (lines ~868, 979, 1277, 2080, 2644, 2743) in case any of those also assume the
three known active statuses are the only ones a work-role session can be attached to.

## 6. Frontend status enumeration (AC5 touchpoint)

`grep -rl "pr_pending" web-app/src` surfaces the files with exhaustive/near-exhaustive
status handling that will need the new status added for it to render meaningfully
instead of falling through to an "unknown status" default:

- `web-app/src/app/backlog/page.tsx`
- `web-app/src/components/backlog/BacklogBoard.tsx` (+ its
  `BacklogBoard.hiddenStatuses.test.tsx` — likely already has a notion of
  "hidden"/inactive statuses similar to `archived`, worth checking as a template for
  hiding the new status from the default active-work board view)
- `web-app/src/components/backlog/BacklogItemDetail.tsx`
- `web-app/src/components/backlog/detail/ActionsSection.tsx`,
  `PullRequestSection.tsx`, `VersionControlSection.tsx` (+ its `.test.tsx`),
  `StageTracker.tsx` (+ its `.test.tsx`)

Not deeply explored per the "backend/library patterns" framing of this research pass —
flagging file locations only; a frontend-specific research/planning pass should read
these fully before implementation.

## Summary of Answers to the Specific Research Questions

1. **`BacklogStatus`**: string-backed enum, no `IsValid()` exists yet — validity is
   implicit via the `validTransitions` map. Add a new `const`, then add transition
   edges into/out of it in `validTransitions` (`session/domain/backlog.go:16-24,
   331-388`).
2. **MCP tool pattern**: `mark3labs/mcp-go v0.48.0`, tools registered via
   `s.AddTool(mcpgo.NewTool(...), h.handlerMethod)` in `registerBacklogTools`
   (`tools_backlog.go:920`). Handler signature:
   `func (h *backlogHandlers) name(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`.
   `requestReview` (line 337) and `reportPRCreated` (line 623) are the direct
   templates for session-UUID extraction, link check (`GetItemSessionBySessionAndItem`),
   role check (`itemSession.Role != session.SessionRoleWork`), and CAS transition via
   `TransitionBacklogItemStatus`.
3. **ent migration**: not needed. `status` is `field.String("status")`
   (`session/ent/schema/backlog_item.go:35`), a plain string column — a new enum value
   is just a new string literal, no schema/migration change. A migration would only be
   needed if a *new column* (e.g. `close_reason`) is added.
4. **Versions**: `mark3labs/mcp-go v0.48.0`, `entgo.io/ent v0.14.5`; no new dependency
   required for this feature.
