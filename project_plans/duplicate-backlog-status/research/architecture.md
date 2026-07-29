# Architecture Research — `duplicate` BacklogStatus

No EventStorming/hotspot table included — this is a CRUD-flavored addition (enum
value + nullable string field + guard clause + one MCP tool + list filter + a
badge), not a multi-actor business domain.

## 1. Call chain today for a status transition, and where the guard actually runs

Two independent call paths reach `EntRepository.TransitionBacklogItemStatus`
today, and only one of them runs `CanTransitionBacklog`/`TransitionGuard`:

```
RPC path (guarded):
  BacklogService.TransitionBacklogItemStatus (server/services/backlog_service.go:597-672)
    → s.storage.GetBacklogItem                       (read current item)
    → s.engine.CanTransition(from, to)                (session/workflow_engine.go:37 → CanTransitionBacklog)
    → s.storage.GetMostRecentReviewVerdictForItem     (resolve OverallOutcome for the guard)
    → s.engine.ValidateGates(guardInput, to)          (session/workflow_engine.go:46 → TransitionGuard)
    → s.storage.TransitionBacklogItemStatus(ctx, id, to, precondition)
        → Storage.TransitionBacklogItemStatus (session/storage.go:528, thin passthrough)
        → EntRepository.TransitionBacklogItemStatus (session/ent_repository_backlog.go:274)

MCP path (UNGUARDED today):
  submitReviewVerdict (server/mcp/tools_backlog.go:372-379)
    → h.storage.TransitionBacklogItemStatus(ctx, itemID, BacklogStatusDone, precondition)
        (same Storage/EntRepository path as above)
```

Key finding: `CanTransitionBacklog` and `TransitionGuard` are invoked **only**
inside `BacklogService.TransitionBacklogItemStatus` (the RPC handler), wrapped
behind the `session.WorkflowEngine` interface (`session/workflow_engine.go`).
Neither `session.Storage.TransitionBacklogItemStatus` nor
`EntRepository.TransitionBacklogItemStatus` call the guard — they are pure
write paths that trust the caller already validated the transition.

`submitReviewVerdict` in `tools_backlog.go` confirms this: it calls
`h.storage.TransitionBacklogItemStatus` **directly**, with no
`CanTransitionBacklog`/`TransitionGuard` call at all. It gets away with this
only because it hardcodes a single known-safe transition (`review → done`,
guarded by an `ExpectedStatus: review` precondition) and treats the outcome as
best-effort/non-fatal. It is not a template to copy for `mark_duplicate` —
`mark_duplicate` is a general-purpose transition from several source statuses
and AC8 explicitly requires the guard to run.

Also relevant: `backlogHandlers` (`server/mcp/tools_backlog.go:63-64`) only
holds a `*session.Storage` — it has **no `session.WorkflowEngine` field**.
`NewBacklogService` is the only place a `WorkflowEngine` gets constructed
(`server/services/backlog_service.go:71-73`, defaulting to
`session.NewDefaultWorkflowEngine()` when nil).

### Recommendation for `mark_duplicate`

Don't thread `session.WorkflowEngine` into `backlogHandlers` — that's an
interface built for the RPC layer's dependency injection/testing needs, and
adding it to the MCP handler struct for one tool is unnecessary indirection.
Instead, call the underlying **pure package-level functions directly** from
the `mark_duplicate` handler, exactly as `WorkflowEngine` itself does:

```go
// in server/mcp/tools_backlog.go, mark_duplicate handler
if !session.CanTransitionBacklog(from, session.BacklogStatusDuplicate) {
    return errResult(ErrInvalidArgument, fmt.Sprintf("cannot mark %s duplicate from status %q", itemID, from), ""), nil
}
guardInput := session.BacklogItemTransitionInput{
    Status:            from,
    AcCriteriaJSON:    item.AcceptanceCriteria,
    PlanApproved:      item.PlanApproved,
    SkipPlanning:      item.SkipPlanning,
    PlanArtifactsPath: item.PlanArtifactsPath,
    DuplicateOfID:     duplicateOfID,
    DuplicateOfExists: exists, // resolved via storage.GetBacklogItem lookup, see §2
}
if guardErr := session.TransitionGuard(guardInput, session.BacklogStatusDuplicate); guardErr != nil {
    return errResult(ErrInvalidArgument, guardErr.Error(), ""), nil
}
updated, err := h.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusDuplicate, precondition) // see §4 for duplicate_of_id threading
```

This satisfies AC8 ("mark_duplicate must perform CanTransitionBacklog +
TransitionGuard + the transition itself") using the exact same pure functions
the RPC layer's `WorkflowEngine` wraps — same validation, no new abstraction,
no behavior drift between the RPC and MCP entry points.

## 2. Where the duplicate_of_id existence check should live

`TransitionGuard` (`session/backlog.go:185`) is a pure function — it takes a
plain `BacklogItemTransitionInput` struct and returns an error, with **no DB
handle, no context.Context, no storage dependency**. Every existing guard rule
follows this shape: `PlanApproved`, `SkipPlanning`, `PlanArtifactsPath`,
`OverallOutcome` are all resolved by the *caller* (the RPC handler) via prior
DB lookups and passed in as plain fields (see
`server/services/backlog_service.go:630-639`). `OverallOutcome` in particular
is the closest precedent: it requires a DB query
(`GetMostRecentReviewVerdictForItem`) performed by the caller *before*
constructing `guardInput`, specifically so `TransitionGuard` stays pure and
testable without mocking a DB.

**Recommendation: keep `TransitionGuard` pure.** Do not give it a DB
dependency. Add two new fields to `BacklogItemTransitionInput`:

```go
type BacklogItemTransitionInput struct {
    // ... existing fields ...
    DuplicateOfID     string // target's duplicate_of_id value being set (may be empty)
    DuplicateOfExists bool   // resolved by the caller via a prior GetBacklogItem lookup
}
```

And a new guard case in `TransitionGuard`:

```go
case to == BacklogStatusDuplicate:
    if item.DuplicateOfID == "" {
        return ErrDuplicateOfRequired
    }
    if item.DuplicateOfID == item.Status... // self-reference check needs the item's own id, see note below
    if !item.DuplicateOfExists {
        return ErrDuplicateOfNotFound
    }
    return nil
```

One wrinkle: a self-reference check (`duplicate_of_id != this item's own id`)
needs the item's own ID, which `BacklogItemTransitionInput` doesn't currently
carry (it only has `Status`, not `ID`). Add an `ID string` field to the input
struct alongside `DuplicateOfID`/`DuplicateOfExists` — this is a one-field,
non-breaking addition (existing callers leave it as the zero value and lose
nothing, since no existing guard branch reads `ID`).

The caller (RPC handler and `mark_duplicate` MCP handler, both) resolves
`DuplicateOfExists` with a plain `storage.GetBacklogItem(ctx, duplicateOfID)`
call before invoking `TransitionGuard` — mirroring exactly how
`GetMostRecentReviewVerdictForItem` is already resolved before the guard call
today. Two new sentinel errors follow the existing pattern
(`ErrACRequired`, `ErrPlanRequired`, etc.):

```go
ErrDuplicateOfRequired = errors.New("duplicate_of_id is required when marking an item duplicate")
ErrDuplicateOfNotFound = errors.New("duplicate_of_id does not reference an existing backlog item")
```

## 3. Atomic write: closing the race in `EntRepository.TransitionBacklogItemStatus`

Current code (`session/ent_repository_backlog.go:274-322`) is read-then-blind-write:

```go
current, err := r.client.BacklogItem.Get(ctx, parsedID)       // read
// ... precondition check against `current` (in Go, not in SQL) ...
item, err := r.client.BacklogItem.UpdateOneID(parsedID).
    SetStatus(string(toStatus)).
    SetUserModifiedStatusAt(now).
    Save(ctx)                                                  // blind write — no predicate
```

Between the `Get` and the `Save`, another goroutine/request can change the
row's status; the precondition check only validates the value read at `Get`
time, not the value at write time. This is the FR4 race.

**Fix**: move the precondition into the `Save()` call itself via `.Where(...)`
on the update builder, and add the `duplicate_of_id` write to the same builder
so both changes commit atomically in the one `UPDATE`:

```go
builder := r.client.BacklogItem.UpdateOneID(parsedID).
    SetStatus(string(toStatus)).
    SetUserModifiedStatusAt(now).
    Where(backlogitem.StatusEQ(current.Status)) // conditional predicate, closes the race

if precondition != nil && precondition.ExpectedUpdatedAt != nil {
    builder = builder.Where(backlogitem.UpdatedAtEQ(*precondition.ExpectedUpdatedAt))
}
if duplicateOfID != "" {
    builder = builder.SetDuplicateOfID(duplicateOfID)
}

item, err := builder.Save(ctx)
```

Ent version in `go.mod` is **`entgo.io/ent v0.14.5`**. `.Where(ps
...predicate.T)` on generated `*UpdateOne` builders is confirmed present in
this version — it's part of the standard entc-generated builder template
(`entc/gen/template/builder/update.tmpl` in the module cache defines `Where`
on both the `Update` and `UpdateOne` builder types; this template has emitted
`Where()` on `UpdateOne` since ent v0.10, and 0.14.5 is unchanged in this
regard). Note: this repo's `session/ent/` generated package is **not checked
in** (only `session/ent/schema/` is versioned; generated code is produced by
`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert
./session/ent/schema` per `session/ent/generate.go`), so the actual
`backlogitem_update.go` couldn't be inspected directly in this worktree — but
the template source confirms the API shape generically, and it's exercised by
existing code the same way in other repos using this ent version.

**Zero-rows-affected detection**: ent does not expose a rows-affected count on
`UpdateOne.Save()`. Per the SQL dialect template
(`entc/gen/template/dialect/sql/update.tmpl`), when the underlying `UPDATE ...
WHERE id = ? AND <predicate>` affects zero rows, `sqlgraph` returns a
`*sqlgraph.NotFoundError`, which ent's generated `sqlSave` translates into
`*ent.NotFoundError{Label: "backlogitem"}` — the **same error type** already
returned when the row's `id` itself doesn't exist. `ent.IsNotFound(err)` can't
distinguish "row missing" from "row exists but predicate excluded it (lost
the race)" by type alone.

Because `TransitionBacklogItemStatus` already did a successful `Get(ctx,
parsedID)` immediately before building the update (confirming the row exists
at read time), any `ent.IsNotFound` returned from the subsequent `Save()` call
in this function can only mean the `Where(StatusEQ(current.Status))`
predicate excluded the row — i.e., a concurrent transition already changed
the status. The fix: in this function specifically, translate a
post-`Get`-success `NotFoundError` from `Save()` into `ErrPreconditionFailed`
(the same sentinel already used for the existing application-level
precondition checks), not the generic `ErrNotFound`:

```go
item, err := builder.Save(ctx)
if err != nil {
    if ent.IsNotFound(err) {
        // We already confirmed the row exists via Get() above; a NotFound
        // here means the StatusEQ predicate excluded it — lost the race.
        return nil, fmt.Errorf("%w: status changed concurrently for item %s", ErrPreconditionFailed, id)
    }
    return nil, fmt.Errorf("failed to transition backlog item %s status: %w", id, err)
}
```

This also subsumes the existing pre-check block (lines 288-295) — it becomes
redundant once the DB-level predicate enforces the same condition
atomically, though keeping the pre-check as a fast-fail (cheap, avoids a
round trip to discover a stale read) alongside the DB-level predicate is a
reasonable belt-and-suspenders choice, not a requirement.

## 4. Threading `duplicate_of_id` through 3 layers without breaking existing callers

Signature chain today:

```
BacklogService.TransitionBacklogItemStatus(req)                              // RPC handler
  → s.storage.TransitionBacklogItemStatus(ctx, id, toStatus, precondition)   // session/storage.go:528
    → r.repo.TransitionBacklogItemStatus(ctx, id, toStatus, precondition)    // EntRepository, session/ent_repository_backlog.go:274
```

Three existing callers of `Storage.TransitionBacklogItemStatus` pass no
duplicate info today: the RPC handler (line 661), `submitReviewVerdict`
(tools_backlog.go:375), and two other call sites in backlog_service.go
(lines 953, 1046, 1119, 1356 per the earlier grep — bulk/session-lifecycle
transitions). None of these should need a code change if the new parameter
is optional and additive.

**Minimal-diff approach**: extend `BacklogItemPrecondition` — or add a
sibling struct — rather than adding a new positional parameter, so existing
call sites compile unchanged. Two viable shapes:

- **Option A (extend precondition struct is wrong place semantically)** —
  `BacklogItemPrecondition` is about *read-time expectations*, not
  *write-time payload*, so don't overload it.
- **Option B (recommended): add a variadic-free optional parameter via a new
  small struct**, e.g. `TransitionOptions`:

```go
// session/repository.go (near BacklogItemPrecondition)
type TransitionOptions struct {
    DuplicateOfID string // set only when toStatus == BacklogStatusDuplicate
}
```

Thread it as a new trailing parameter through all 3 layers:

```go
// EntRepository (session/ent_repository_backlog.go)
func (r *EntRepository) TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, opts *TransitionOptions) (*BacklogItemData, error)

// Storage (session/storage.go:528)
func (s *Storage) TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, opts *TransitionOptions) (*BacklogItemData, error) {
    return s.repo.TransitionBacklogItemStatus(ctx, id, toStatus, precondition, opts)
}
```

This **is** a breaking signature change (new required parameter), which
contradicts "no breaking change to unrelated callers" unless every call site
is updated. Since Go has no optional parameters, the only ways to avoid
touching all ~5 existing call sites are:
1. Accept the small mechanical diff — add `nil` as the new trailing arg at
   each of the ~5 existing call sites (RPC handler, `submitReviewVerdict`,
   the 3 backlog_service.go internal call sites). This is a one-line change
   per call site, not a design change, and every call site is already in the
   same package/module being edited for this feature, so "breaking" here
   just means "grep and add `nil`," not an external API break.
2. Alternatively, add a **new** method
   `TransitionBacklogItemStatusWithDuplicate(...)` used only by
   `mark_duplicate`/the RPC path that supports it, leaving
   `TransitionBacklogItemStatus`'s signature untouched. This avoids touching
   existing call sites at all, at the cost of two near-duplicate methods in
   `EntRepository`/`Storage`.

**Recommendation: option 1** (add `opts *TransitionOptions` as a 5th
parameter, update the ~5 existing call sites to pass `nil`). It keeps a
single write path (satisfying the "same validation path" goal from §1) rather
than forking into a near-duplicate method, and the mechanical diff is small
and entirely internal to this codebase (no external/generated-client
breakage — `Storage`/`EntRepository` aren't exposed outside this Go module).

**RPC-layer passthrough**: add `optional string duplicate_of_id = 6;` to
`TransitionBacklogItemStatusRequest` in `proto/session/v1/backlog.proto`
(next free field number; existing fields go up to 5). `BacklogService.
TransitionBacklogItemStatus` builds `opts := &session.TransitionOptions{
DuplicateOfID: req.Msg.DuplicateOfId}` only when `req.Msg.DuplicateOfId != ""`,
so the RPC handler's own signature doesn't change, only its body. Run
`make generate-proto` after the `.proto` edit.

**BacklogItem proto message**: add `string duplicate_of_id = 21;` (next free
field number; existing fields go up to 20) so `duplicate_of_id` round-trips
to the frontend for the list-exclusion/badge/link UI work.

**Ent schema**: add to `session/ent/schema/backlog_item.go` `Fields()`:
```go
field.String("duplicate_of_id").
    Optional().
    Comment("references another BacklogItem.ID; not an ent edge, mirrors the existing plain-string FK-like fields (external_id, plan_artifacts_path) rather than a self-referential edge"),
```
This matches the existing schema convention exactly — every other
identifier-shaped field on this schema (`external_id`, `plan_artifacts_path`,
`repo_path`) is a plain optional string, not an ent edge/relation. No new
edge, no FK constraint, no schema migration complexity from
self-referential edges.

## 5. Audit trail: does `BacklogStatusEvent` need `duplicate_of_id`?

**Recommendation: no schema change.** `BacklogStatusEvent`
(`session/ent_repository_backlog.go:310-318`) already records `from_status`
and `to_status` for every transition — that's sufficient to show "this item
moved to duplicate at time T." The `duplicate_of_id` value itself is:
- Queryable directly on the `BacklogItem` row at any time (it's a live field,
  not something that changes after being set, in the current design).
- Not something that changes across multiple duplicate-marking events for
  the same item in ordinary usage — there's no versioned history requirement
  in the ACs.

Adding `duplicate_of_id` to `BacklogStatusEvent` would only add value if the
product wants to reconstruct "what was this item a duplicate of at each
point in time" (i.e., if `duplicate_of_id` could be reassigned repeatedly and
that history mattered). Nothing in the known ACs asks for that. Simplest
correct design: leave `BacklogStatusEvent` unchanged; `to_status ==
"duplicate"` plus the item's current `duplicate_of_id` field is enough to
answer "is this a duplicate, and of what."

## Summary of touch points

| Layer | File | Change |
|---|---|---|
| Proto enum-ish | N/A | `duplicate` is a `BacklogStatus` string value, not a proto enum (status is `string status` in the message) — no proto enum change needed, just documentation of the new allowed value |
| Proto messages | `proto/session/v1/backlog.proto` | `BacklogItem.duplicate_of_id` (field 21), `TransitionBacklogItemStatusRequest.duplicate_of_id` (field 6) |
| Ent schema | `session/ent/schema/backlog_item.go` | new optional `duplicate_of_id` string field |
| State machine | `session/backlog.go` | new `BacklogStatusDuplicate` const; new rows in `validTransitions` for each source status that can go to duplicate; new guard case in `TransitionGuard`; new `DuplicateOfID`/`DuplicateOfExists`/`ID` fields on `BacklogItemTransitionInput`; two new sentinel errors |
| Repository | `session/ent_repository_backlog.go` | `TransitionBacklogItemStatus` gains `.Where(backlogitem.StatusEQ(current.Status))` + conditional `SetDuplicateOfID`, and a 5th `opts *TransitionOptions` parameter; post-Get NotFound → ErrPreconditionFailed translation |
| Storage wrapper | `session/storage.go` | passthrough `opts *TransitionOptions` parameter |
| RPC handler | `server/services/backlog_service.go` | build `opts` from `req.Msg.DuplicateOfId`; existing ~4 internal call sites pass `nil` |
| MCP tool | `server/mcp/tools_backlog.go` | new `mark_duplicate` handler calling `session.CanTransitionBacklog` + `session.TransitionGuard` directly (no `WorkflowEngine` needed in `backlogHandlers`), then `storage.TransitionBacklogItemStatus(..., opts)` |
| List filtering | `session/ent_repository_backlog.go:140-143` | add `string(BacklogStatusDuplicate)` to the existing `StatusNotIn(done, archived)` call in `ListBacklogItems` — one-line change, reuses the existing `ExcludeTerminal` mechanism |
| Audit log | `session/ent_repository_backlog.go:310-318` | no change — `to_status` already captures the transition; `duplicate_of_id` lives on the item itself |
