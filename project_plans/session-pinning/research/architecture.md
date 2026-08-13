# Architecture Research: Session Pinning

This is a CRUD-shaped feature — one boolean flag, one toggle RPC, one list-rendering
change. No Event-Command-Policy table; this doc just traces the concrete files/lines
a `pinned` field needs to touch, mirroring `hidden`/`archived_at`/`auto_yes`.

## 1. Integration Points — the full path of an existing toggle

Traced `ArchiveSession`/`UnarchiveSession` (closest RPC-toggle precedent) and
`SetAutoYes` (closest actor-setter-shape precedent, since `Hidden` has no runtime
setter — see §2).

### RPC handler — `server/services/session_service.go:4285-4327`

```go
// +api: session:archive
func (s *SessionService) ArchiveSession(ctx, req) (*connect.Response[...], error) {
    inst := s.FindLiveInstance(req.Msg.SessionId)          // 1. resolve live Instance
    if inst == nil { return nil, connect.NewError(CodeNotFound, ...) }
    now := time.Now()
    inst.ArchiveWithStop(now)                               // 2. actor-routed mutation
    s.storage.SaveInstances([]*session.Instance{inst})      // 3. synchronous ent persist
    return connect.NewResponse(&sessionv1.ArchiveSessionResponse{}), nil
}
```

`UnarchiveSession` (session_service.go:4311-4327) is the exact mirror: resolve →
`inst.SetArchivedAt(nil)` → `SaveInstances` → empty response. This is the template
for `PinSession`/`UnpinSession` (a `PinSession(sessionId)` /
`UnpinSession(sessionId)` RPC pair, matching the archive/unarchive pair rather than
a single `SetSessionPinned(bool)` RPC — the codebase's own convention for a boolean
toggle is two verbs, not one setter + flag).

### Actor-owned state mutation — `session/instance_actor_setters.go`

Two shapes exist; pin should follow the **`AutoYes` shape**, not the `ArchivedAt`
shape:

- `ArchivedAt` (lines 202-282) is CAS/stop-aware (`SetArchivedAtIfNil`,
  `ArchiveWithStop`) because archiving also force-transitions `Status → Stopped` —
  that complexity is specific to archiving's semantics, not a general pattern.
- `AutoYes` (lines 317-334) is the plain case — exactly what a boolean flag with no
  side effects needs:

```go
func setAutoYesLocked(s *instanceState, v bool) {
    s.inst.mu.Lock()
    s.inst.AutoYes = v
    snap := buildSnapshot(s.inst)
    s.inst.mu.Unlock()
    s.inst.snapshot.Store(snap)
}

func (i *Instance) SetAutoYes(v bool) {
    _ = i.sendSyncErr(func(s *instanceState) error {
        setAutoYesLocked(s, v)
        return nil
    })
}
```

`SetSessionPinned(v bool)` (or `SetPinned`/`ClearPinned` pair to mirror
Archive/Unarchive's two-verb RPC naming) should follow this exact shape: lock →
mutate `s.inst.Pinned` → rebuild snapshot → unlock → atomic store, all inside
`sendSyncErr` so it's actor-routed (no data race with the instance's own
goroutine).

### ent persistence — `session/ent_repository.go`

Three touch points, all present for `Hidden`/`AutoYes`/`IsExpanded` already:

- `ent_repository.go:155-158` (Create) — `SetAutoYes(data.AutoYes)`,
  `SetIsExpanded(data.IsExpanded)` unconditionally; `Hidden` only conditionally
  (`if data.Hidden { sessionCreate.SetHidden(data.Hidden) }` at line 215-217,
  since it defaults false and ent's ` Default(false)` covers the false case). A new
  `Pinned` field should follow the `AutoYes`/`IsExpanded` unconditional pattern —
  no reason to special-case it like `Hidden` does.
- `ent_repository.go:444` (Update) — `sessionUpdate.SetHidden(data.Hidden)`,
  needs the equivalent `SetPinned(data.Pinned)`.
- `ent_repository.go:1059` (read-back, ent row → `InstanceData`) —
  `Hidden: sess.Hidden` needs `Pinned: sess.Pinned`.

### Back out via `ListSessions` / single-session reads

`Instance.ToInstanceData()` (referenced from `storage.go:269`) and
`fromInstanceData()` (storage.go:301) round-trip through `InstanceData`
(`session/storage.go:116-117` has `Hidden bool`) — a `Pinned bool` field belongs
next to it.

The **actual proto-conversion choke point is `server/adapters/instance_adapter.go`**,
not `session_service.go`. `InstanceToProto` (instance_adapter.go:15-60+) is the
single function every RPC response (`ListSessions`, single-session `GetSession`,
event-stream payloads) funnels through:

```go
protoSession := &sessionv1.Session{
    ...
    Category:    snap.Category,
    IsExpanded:  snap.IsExpanded,
    ...
}
...
protoSession.Hidden = snap.Hidden       // instance_adapter.go:176
...
if snap.ArchivedAt != nil {             // instance_adapter.go:183-184
    protoSession.ArchivedAt = timestamppb.New(*snap.ArchivedAt)
}
```

A `Pinned` field needs one line here: `protoSession.Pinned = snap.Pinned` (or in
the struct literal alongside `IsExpanded`, matching the plain-bool style, not the
nil-check style used for `ArchivedAt`).

**Correction to the requirements doc's file citation**: the requirements doc
implies the RPC handler code (`session_service.go`) contains the proto-field
mapping. It does not — `Hidden` and `IsExpanded` are read *from* `inst.Hidden` in
`session_service.go` only for **filtering** (`if inst.Hidden && !req.Msg.IncludeHidden`
at line 1140/1180, deciding whether to include the instance in the response slice
at all), never for building the outgoing proto struct. The actual proto assembly
is 100% in `instance_adapter.go`. Any plan/implementation phase should target that
file, not `session_service.go`, for the "return pin state in the proto" step.

## 2. Data Flow — source of truth

**Single source of truth: the in-memory `Instance` field, mirrored into its atomic
snapshot, persisted to ent on every `SaveInstances` call.** There is no separate
"read straight from ent on every request" path — `ListSessions` always reads from
the live in-memory `Instance` (via `FindLiveInstance`/the instance registry) and
calls `InstanceToProto`, which reads `inst.Snapshot()` (an `atomic.Pointer` load,
no lock, no DB round-trip). ent is the durability layer, not the read path.

Concretely, `hidden` (and every boolean flag in this family — `AutoYes`,
`IsExpanded`, `ArchivedAt`) has exactly one value that matters at request time:
`Instance.Hidden` → `buildSnapshot()` copies it into `InstanceSnapshot.Hidden`
(`session/instance_snapshot.go:115,196`) → `InstanceToProto` reads
`snap.Hidden`. ent's `hidden` column is written on every mutation
(`ent_repository.go`) and read back only at process-cold-start
(`Storage.LoadInstances` → `fromInstanceData`, `storage.go:290-301`), to
reconstruct `Instance.Hidden` when the server restarts.

So: **no separate in-memory-vs-ent duality to design around.** `pinned` should be:
1. A field on `Instance` (`session/instance.go`, alongside `Hidden` at line 220-223
   and `IsExpanded` at line 145-146).
2. Mirrored into `InstanceSnapshot` (`session/instance_snapshot.go`, alongside
   `Hidden`/`IsExpanded` at lines 93/115).
3. Set via the actor setter in `instance_actor_setters.go` (§1).
4. Persisted via `ent_repository.go`'s Create/Update/read-back (§1).
5. Serialized in `instance_serialization.go` (alongside `Hidden`/`IsExpanded` at
   lines 66/110/230/282) and `storage.go`'s `InstanceData` (line 116-117) — these
   are the JSON-snapshot/legacy-format paths parallel to the ent path; both
   existing fields appear in both, so `pinned` must too.
6. Read into the proto in `instance_adapter.go` (§1).

## 3. Consistency — does the RPC need to flush before returning?

**Already true today, no new work required.** `Storage.SaveInstances` (which
`ArchiveSession` calls before returning its response, `session_service.go:4303`)
is a misleading name — despite the "Save...Async"-sounding docstring pattern seen
elsewhere in the codebase, `saveInstancesToRepo` (`session/storage.go:263-282`) is
fully synchronous: it calls `s.repo.Update(ctx, data)` (falling back to `Create`)
directly, in-line, in the calling goroutine, with no queue/buffer/goroutine hop.
`SaveInstancesSync` (storage.go:285-287) is literally an alias —
`return s.saveInstancesToRepo(instances)` — confirming there is no separate
"eventually consistent" mode for this write path.

So `PinSession`/`UnpinSession` should call `s.storage.SaveInstances([]*session.Instance{inst})`
exactly like `ArchiveSession` does, and by the time the RPC handler returns, the
ent write has already completed (or the handler has returned a `CodeInternal`
error) — pin state is guaranteed durable before the client sees a success
response. No additional flush/fsync step needed beyond what `ArchiveSession`
already does.

## 4. Frontend Architecture — where the "Pinned" section goes

### Grouping/sorting pipeline — `web-app/src/components/sessions/SessionList.tsx`

The pipeline is a strict `useMemo` chain:

```
sessions (prop)
  → filteredSessions   (SessionList.tsx:530-585, search/status/category/tag/archived filters)
  → sortedSessions      (SessionList.tsx:598-630, sortField/sortDir)
  → groupedSessions      (SessionList.tsx:648-650, groupSessions(sortedSessions, groupingStrategy))
  → flatItems / cardGroupCounts (row-mode vs card-mode virtualization inputs)
```

`groupSessions()` (`web-app/src/lib/grouping/strategies.ts:69+`) returns
`GroupedSessions[]` — `{ groupKey, displayName, sessions }[]`. Every rendering
path (`GroupedVirtuoso` for card mode, the `flatItems` header/session interleave
for row mode) already consumes this shape generically — it doesn't know or care
what a "group" means semantically (Category vs Tag vs Status), so a synthetic
`{ groupKey: "pinned", displayName: "Pinned", sessions: [...] }` entry prepended
to the array **is the natural insertion point**, reusing 100% of existing
`SessionCard`/`SessionRow` rendering with zero duplication.

Concretely: compute a `pinnedSessions = sortedSessions.filter(s => s.pinned)` and
either (a) prepend a synthetic pinned group before calling `groupSessions()` on
the *remaining* (non-pinned) sessions, or (b) always call `groupSessions()` on
`sortedSessions` and prepend the pinned group unconditionally regardless of
`groupingStrategy` (since requirement #4 says pinned overrides grouping/sorting
entirely — pinned sessions should appear "regardless of their status/recency").
Option (b) avoids a special case for `GroupingStrategy.None` and matches "top
section above the normal grouped/sorted list" from the requirements literally.
This is a `SessionList.tsx` change only (~line 648-650) — no new component needed,
though a `PINNED_GROUP_KEY` sentinel constant would help `flatItems`/virtuoso
logic special-case its header styling (e.g. no collapse toggle, always expanded)
if desired.

### Toggle affordance — no existing archive/hide menu item to mirror

Requirement #6 says pin toggle should be "available from the session card context
menu... mirrors existing action-menu affordances such as archive/hide." That
precedent **does not exist yet in the frontend** — this is a gap in the
requirements doc's assumption, not a research dead-end:

- `useSessionService.ts` (`web-app/src/lib/hooks/useSessionService.ts:600-618,
  1108-1109`) already exports `archiveSession`/`unarchiveSession` hook functions
  that call the RPC (`ArchiveSessionRequestSchema`/`UnarchiveSessionRequestSchema`).
- But grepping the entire `web-app/src` tree for actual call sites of
  `archiveSession(`/`unarchiveSession(` outside `useSessionService.ts` itself and
  its tests returns **zero results** — archive/hide are backend-complete but have
  no wired-up UI trigger today (likely used only by backend automation/backlog
  sweeps, not manual user action).
- The actual overflow/kebab menu component is
  `web-app/src/components/sessions/SessionActionsOverflow.tsx` (imported by
  `SessionCard.tsx:13`) — it currently exposes Resume/Pause/Hibernate/Delete/
  Clone/Rename/ChangeProgram/OpenInNewPane/UpdateTags/NewWorkspace/
  ClearConversationState/Restart (see the `onX` prop list at lines 43-54 and the
  `hasGroupN` flags at lines 267-275), no Archive/Hide entries.

**Implication for planning**: `PinSession`/`UnpinSession` needs (a) hook functions
in `useSessionService.ts` following the exact `archiveSession`/`unarchiveSession`
shape (lines 600-618), and (b) a *new* menu item in `SessionActionsOverflow.tsx`
(there is no existing archive/hide menu item to copy structurally, but the
component's existing action-button pattern, e.g. `onDelete`/`onClone`, is the
template to follow) — plus, per requirement #6, optionally a second entry point
in whatever component renders the session-detail header (not yet located in this
research pass; a `grep -rn "SessionDetail" web-app/src/components` search should
be the first step of the planning phase if a detail-header toggle is in scope for
v1).

## Summary of files to touch (implementation-phase checklist, not exhaustive)

| Layer | File | Precedent field |
|---|---|---|
| Proto enum/field | `proto/session/v1/types.proto` | `hidden = 57`, `archived_at = 63` → next free number after 63 |
| ent schema | `session/ent/schema/session.go:102-104` | `field.Bool("hidden").Default(false)` |
| ent regen | `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` | — |
| Instance struct | `session/instance.go:220-223` | `Hidden bool` |
| Snapshot struct | `session/instance_snapshot.go:93,115,159,196` | `IsExpanded`/`Hidden` |
| Actor setter | `session/instance_actor_setters.go:317-334` | `SetAutoYes` (plain-bool shape) |
| ent repo Create/Update/read | `session/ent_repository.go:155-158,444,1059` | `SetAutoYes`/`SetIsExpanded`/`Hidden:` |
| InstanceData/serialization | `session/storage.go:116-117`, `session/instance_serialization.go:66,110,230,282` | `Hidden bool` |
| Proto conversion | `server/adapters/instance_adapter.go:46,176` | `IsExpanded: snap.IsExpanded` / `protoSession.Hidden = snap.Hidden` |
| RPC handlers | `server/services/session_service.go:4285-4327` | `ArchiveSession`/`UnarchiveSession` |
| Frontend hook | `web-app/src/lib/hooks/useSessionService.ts:600-618,1108-1109` | `archiveSession`/`unarchiveSession` |
| Frontend menu | `web-app/src/components/sessions/SessionActionsOverflow.tsx` | new entry, no direct precedent |
| Frontend list | `web-app/src/components/sessions/SessionList.tsx:648-650` | `groupedSessions` insertion point |
| Frontend types | `web-app/src/gen/session/v1/types_pb.ts` (regenerated by `make proto-gen`) | `pinned` field on `Session` |
