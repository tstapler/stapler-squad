# Research: Technology Stack — Session Pinning

## Summary

No new dependencies. This is a small, additive change on an already-in-place
Go/ent/ConnectRPC/React/vanilla-extract stack, following the exact shape of
the existing `hidden` boolean field end-to-end. Every layer already has a
direct precedent to copy.

## Versions in use (VERIFIED via `go.mod` / `web-app/package.json`)

| Component | Version |
|---|---|
| `entgo.io/ent` | v0.14.5 |
| `connectrpc.com/connect` (Go) | v1.19.0 |
| `google.golang.org/protobuf` | v1.36.11 |
| `@connectrpc/connect` / `connect-web` | ^2.1.1 |
| `@bufbuild/protobuf` | ^2.11.0 |
| `@bufbuild/protoc-gen-es` | ^2.11.0 |
| `@vanilla-extract/css` | ^1.20.1 |
| `@vanilla-extract/recipes` | ^0.5.7 |
| `@vanilla-extract/next-plugin` | ^2.5.1 |

## 1. ent schema field

`session/ent/schema/session.go` — `Session.Fields()` (line 18) already has five
plain `field.Bool(...).Default(false)` fields with no getters/setters beyond
generated ent code: `auto_yes` (46), `autonomous_mode` (48), `is_expanded`
(59), `one_shot` (87), `hidden` (102). `pinned` follows this exact shape:

```go
field.Bool("pinned").
    Default(false).
    Comment("When true, session is pinned and surfaces in the dedicated Pinned section regardless of status/recency."),
```

Regeneration command (per `.claude/rules/ent-schema-generation.md`, sourced
from `session/ent/generate.go`'s `//go:generate` directive):

```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

Do not add an `index.Fields("pinned")` unless a query filters by it directly
— none of the existing single-purpose booleans (`hidden`, `auto_yes`,
`is_expanded`, `one_shot`) are indexed; only fields used in `WHERE`/sort
predicates are (`status`, `category`, `archived_at`, etc., per
`Session.Indexes()` at line 168). Pinned sessions are filtered client-side
from the full `ListSessions` payload the same way `hidden`/`archivedAt` are
today, so no index is needed initially.

## 2. Proto — field number and RPC shape

`proto/session/v1/types.proto`, `message Session` (line 9): highest field
number in use is confirmed **63** (`google.protobuf.Timestamp archived_at =
63;`, line 209; `bool hidden = 57;` line 192; `bool autonomous_mode = 60;`
line 203; `string workflow_id = 62;` line 206). New field:

```protobuf
bool pinned = 64;
```

RPC pair precedent — `ArchiveSession`/`UnarchiveSession`
(`proto/session/v1/session.proto` lines 396-401, empty-response messages at
2551-2561):

```protobuf
rpc ArchiveSession(ArchiveSessionRequest) returns (ArchiveSessionResponse) {}
rpc UnarchiveSession(UnarchiveSessionRequest) returns (UnarchiveSessionResponse) {}
```
```protobuf
message ArchiveSessionRequest {
  string session_id = 1;
}
message ArchiveSessionResponse {}
```

`PinSession`/`UnpinSession` should mirror this exactly (request has only
`session_id`, empty response — the frontend re-derives state from the next
`ListSessions`/`WatchSessions` push, same as archive does).

Regen command: `make proto-gen`, which fans out to:
- `session/gen/session/v1/*.go` (Go bindings)
- `web-app/src/gen/session/v1/*_pb.ts` (TS bindings, e.g.
  `ArchiveSessionRequestSchema` used directly in
  `web-app/src/lib/hooks/useSessionService.ts:16`)

## 3. Go backend handler

`server/services/session_service.go:4283-4327` — `ArchiveSession` /
`UnarchiveSession` handlers are the template:

```go
func (s *SessionService) ArchiveSession(
    ctx context.Context,
    req *connect.Request[sessionv1.ArchiveSessionRequest],
) (*connect.Response[sessionv1.ArchiveSessionResponse], error) {
    if req.Msg.SessionId == "" {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
    }
    inst := s.FindLiveInstance(req.Msg.SessionId)
    if inst == nil {
        return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
    }
    // ... mutate via actor-routed setter ...
    if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
    }
    return connect.NewResponse(&sessionv1.ArchiveSessionResponse{}), nil
}
```

Both handlers carry a `// +api: session:archive` / `// +api: session:unarchive`
marker comment immediately above the func — required for
`make registry-generate` to auto-detect the RPC (see
`.claude/rules/feature-registry.md`). `PinSession`/`UnpinSession` need the
same `// +api: session:pin` / `// +api: session:unpin` markers.

## 4. Actor-routed setter (Instance mutation)

`session/instance_actor_setters.go:212-227` — `SetArchivedAt` /
`SetArchivedAtIfNil` is the precedent for how a boolean/timestamp field on
`Instance` is mutated safely through the actor's serialized-access channel
(`sendSyncErr`), not a direct field write + external mutex:

```go
func (i *Instance) SetArchivedAt(t *time.Time) {
    _ = i.sendSyncErr(func(s *instanceState) error {
        setArchivedAtLocked(s, t)
        return nil
    })
}
```

A `SetPinned(bool)` setter following this exact shape (route through
`sendSyncErr`, lock `s.inst.mu` inside the closure, set the field, return
nil) is the correct pattern — no CAS semantics needed here since pin/unpin
is a plain user toggle, not a race-guarded state transition like
`SetArchivedAtIfNil`.

## 5. Frontend: RPC client hook

`web-app/src/lib/hooks/useSessionService.ts` — `archiveSession`/
`unarchiveSession` (lines 600-622, exposed in the hook's return type at
110-111) are the direct template for `pinSession`/`unpinSession`:

```ts
const archiveSession = useCallback(
  (id: string) =>
    withResult(async () => {
      await clientRef.current.archiveSession(create(ArchiveSessionRequestSchema, { sessionId: id }));
    }),
  [...]
);
```

## 6. Frontend: session list — grouping/sorting/filtering conventions

`web-app/src/components/sessions/SessionList.tsx` (2000+ lines) is the
central component. Relevant existing patterns to reuse for the requirement
that pinned sessions render in a dedicated top section **above** the
grouped/sorted list:

- **Filter state precedent**: `showArchived` (line 332) is a `useState`
  persisted to `localStorage` via `STORAGE_KEYS` (`makeStorageKeys`, line
  238) and folded into the `filteredSessions` `useMemo` (line 530,
  dependency array at 585). Pin state itself is server-owned per
  requirement #2/#5 — no client persistence needed for the boolean itself,
  but if a "show/hide pinned section" UI toggle is ever added it would
  follow this same `useState` + `STORAGE_KEYS` pattern (out of scope here).
- **Grouping**: `groupSessions()` lives in `web-app/src/lib/grouping/strategies.ts`
  (pure function, `GroupingStrategy` enum, no new dependency). The pinned
  section is explicitly **separate** from this — requirements say pinned
  sessions render in a section above the grouped/sorted list "regardless of
  their status/recency," i.e. it is not itself a `GroupingStrategy` value;
  it's a pre-filter/partition step done before `groupSessions()` is called
  on the remainder (or before sorting, depending on whether pinned sessions
  should also appear again in their normal group — resolve in the plan
  phase, out of scope for this stack research).
- **Rendering point**: `groupedSessions` is computed via `useMemo` (line
  648: `groupSessions(sortedSessions, groupingStrategy)`) and consumed
  starting around line 1421 (`groupedSessions[groupIndex]`). A "Pinned"
  section would be a sibling render block inserted before this grouped
  list render, fed by `sortedSessions.filter(s => s.pinned)` (or filtered
  earlier, before `filteredSessions`/`sortedSessions`, depending on whether
  pinned sessions should be excluded from the normal list below — again a
  plan-phase decision, not a stack question).
- **Action menu precedent**: `web-app/src/components/sessions/SessionActionsOverflow.tsx`
  is the existing per-session action menu (Resume/Pause/Restart/Rename/
  Clone/Delete/etc., emoji-prefixed list items, lines 523-764). A "Pin" /
  "Unpin" menu item added here mirrors the Archive/Hide entries mentioned
  in the requirements (per requirement #6 "context menu and/or session
  detail header"). Note: no `onArchive`/`archiveSession` call was found
  wired directly in this file at present — archive appears to be invoked
  elsewhere (likely `SessionRow.tsx` or `SessionCard.tsx`, given `hidden`
  references also live there); confirm the exact current archive/hide
  call site in the plan phase before wiring pin identically.

## 7. CSS — vanilla-extract `.css.ts` for the new "Pinned" section

Per `.claude/rules/css-architecture.md`, new UI (the "Pinned" section
header/container) must be a `.css.ts` file, not a `.module.css` file, and
must import tokens from `@/styles/theme.css` (`vars`) rather than hardcoded
values or `var()` strings.

`web-app/src/components/sessions/SessionList.css.ts` is the directly
relevant existing file (colocated with `SessionList.tsx`) and shows the
exact idiom to extend:

```ts
import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const header = style({
  marginBottom: vars.space["6"],
});
```

A `pinnedSection` / `pinnedSectionHeader` export added to this same file
(or a new colocated `PinnedSessionsSection.css.ts` if the pinned section
becomes its own component) should follow this pattern: `style({...})` using
`vars.space`, `vars.color.textPrimary`, etc. — never raw hex or `var(--x)`
strings. If the section needs a sticky/elevated visual treatment, reuse
`vars.color.*` tokens already defined in `web-app/src/styles/theme.css.ts`
rather than inventing new ones; only add a new token there first if nothing
suitable exists (per the CSS architecture rule's "Theme Token Contract"
section).

## 8. Registry + e2e (process, not stack, but noted for completeness)

Per `.claude/rules/feature-registry.md`: new backend RPCs need
`docs/registry/features/backend/session-pin.json` /
`.../session-unpin.json` (or a single combined pair), new frontend UI needs
`docs/registry/features/frontend/<feature>.json`, then
`make registry-generate`. Per root `CLAUDE.md`'s E2E Tests section, a new
Playwright spec belongs in `tests/e2e/` with a `// @feature session:pin,
session:unpin` header annotation, `data-testid`/ARIA locators only, no
`waitForTimeout`. No new test framework or library — same Playwright +
Allure setup already in place (`tests/e2e/global-setup.ts` auto-manages the
isolated test server).

## Open questions for the plan phase (not stack questions)

- Whether pinned sessions still also appear in their normal group below the
  Pinned section, or are excluded from it (requirement doc's scope note
  defers "pin ordering within the pinned section" but doesn't explicitly
  say whether pinned sessions are duplicated or removed from the main list).
- Whether pinning an archived session is allowed (requirement doc explicitly
  defers this "decide in research" — but it's a product/UX decision, not a
  technology one; no stack constraint blocks either choice, since `pinned`
  and `archived_at` are independent boolean/nullable fields on the same
  entity).
- Exact current call site of `archiveSession`/`unarchiveSession` in the
  frontend (not confirmed to be `SessionActionsOverflow.tsx` — grep found
  the RPC hook definition but no direct call site among
  `web-app/src/components/sessions/*.tsx`; likely `SessionRow.tsx` or
  `SessionCard.tsx`, given both reference `hidden`). Confirm before wiring
  pin/unpin into the UI.
