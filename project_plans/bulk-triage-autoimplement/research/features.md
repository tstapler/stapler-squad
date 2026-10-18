# Bulk Triage & Bulk Auto-Implement: Feature Research

## Q1 — Eligibility conditions in `getAvailableActions` (`itemActions.ts`)

`getAvailableActions` is the canonical gate (single source of truth per file header). It lives at
`web-app/src/lib/backlog/itemActions.ts`.

**`trigger_triage`** is added for:
- status `idea` (unconditionally)
- status `ready` (unconditionally, alongside `spawn_session` and `spawn_session_autonomous`)

**`spawn_session_autonomous`** is added only for status `ready` (same slot as `spawn_session`), but
the spawn gate in the server (`backlog_service_triage.go:600`) additionally enforces
`!isGatedOnPlanApproval` — i.e. `item.skipPlanning || item.planApproved` — before actually
starting a session. The UI-side `getPrimaryCardAction` encodes the same condition:
`planStatus === "approved" || planStatus === "skipped"`.

`triageStatus` is a derived front-end field (computed from `linkedSessions` in
`useBacklogService.ts:640–694`), not a raw proto field. The bulk-triage eligibility filter
`triageStatus !== "running"` must therefore be evaluated client-side from that derived value, not
from proto state.

**Practical eligibility map for bulk operations:**

| Action | Eligible statuses | Additional guard |
|---|---|---|
| Bulk Triage | `idea`, `ready` | `triageStatus !== "running"` |
| Bulk Auto-Implement | `ready` | `item.skipPlanning \|\| item.planApproved` (plan gate) |

## Q2 — Server behavior when `TriggerTriage` is called on an already-running item

`tombstoneOrphanTriageSessions` (`backlog_service_triage.go:3546–3592`) runs at the top of every
`TriggerTriage` call. If a triage session is genuinely live (`s.IsTriageLive(itemID)` returns true
and the session is not stale), the function returns:

```go
connect.NewError(connect.CodeAlreadyExists,
    fmt.Errorf("triage session already running for item %s", itemID))
```

This surfaces as a ConnectRPC `CodeAlreadyExists` error thrown by `triggerTriage` in
`useBacklogService.ts`. The existing `runBulk` pattern (`Promise.allSettled`) already tolerates
per-item rejections — so the `CodeAlreadyExists` response will mark that item as `failed` in the
`BulkResult` without stopping other items. The bulk feedback counter will therefore include
already-running items in the failure total.

**Recommendation:** Filter out `triageStatus === "running"` client-side before calling (as
specified) so already-running items never appear in the feedback failure count.

## Q3 — `spawnSessionFromItem` when it hits the WIP concurrency cap

At `backlog_service_triage.go:604–620`: when `liveCount >= wipCap` (default cap = 2, settable via
`MaxConcurrentBacklogWorkItems`), the server does **not** return an error. Instead it:

1. Transitions the item to `queued` status via `queueBacklogItem`.
2. Returns `SpawnSessionFromItemResponse{Queued: true}`.

In the client (`useBacklogService.ts:1151`): `spawnSessionFromItem` returns
`{ sessionUuid: resp.sessionUuid, queued: resp.queued }` — `queued: true` with an empty
`sessionUuid` when capped.

The `runBulk` utility only distinguishes `fulfilled` vs `rejected` — a `queued: true` response is
a fulfilled promise. Bulk auto-implement feedback should therefore distinguish three outcome
buckets in its result:

- **spawned** (fulfilled, `queued === false`)
- **queued** (fulfilled, `queued === true`) — item will run once a slot frees
- **failed** (rejected — e.g. plan gate error, item not found)

The existing `formatBulkFeedback` only renders succeeded/failed, so a custom formatter or an
extended `BulkResult` interface (adding a `queued` list) will be needed.

## Q4 — Rate-limit / concurrency concerns for parallel `triggerTriage` calls

There is no explicit rate-limit or per-user throttle on `TriggerTriage` at the RPC layer. Each
call runs a triage session as a backgrounded headless process (`triageInFlight` map,
`backlog_service_triage.go`). In practice, concurrency is bounded by process resources and the
`maxTriageSessionAge` backstop (stale sessions are tombstoned on the next re-trigger). The WIP cap
(`MaxConcurrentBacklogWorkItems`) applies only to `SpawnSessionFromItem`, NOT to `TriggerTriage`
(explicitly noted at `backlog_service_triage.go:2501`: "Deliberately NOT gated by
maxConcurrentBacklogWorkItems").

**Risk:** firing many triage calls in parallel (e.g. 20+ items) will start that many headless LLM
processes simultaneously. The `runBulk` pattern uses `Promise.allSettled` which fires all calls
concurrently. For large selections, consider a sequential or small-batch dispatch (e.g., pLimit 3)
to avoid overwhelming the host machine. This is not enforced by the server today.

## Q5 — Precedent from session-list `BulkActions.tsx`

`web-app/src/components/sessions/BulkActions.tsx` handles: Pause Selected, Resume Selected, Add
Tag, Group As (project), Delete Selected. It uses a flat action-button layout in a `role="toolbar"`
toolbar, with a `feedback` prop rendered as a `role="status"` div and a short auto-clearing timer
(not visible in BulkActions.tsx itself — timeout managed by the page).

`BacklogBulkActions.tsx` follows the same pattern (same CSS imports from `BulkActions.css`, same
`role="toolbar"`, same `busy` prop disabling all buttons, same `feedback` prop). The existing
delete action shows a confirmation dialog with an overlay (`styles.overlay` + `styles.dialog`) —
this is the precedent to follow for the Bulk Auto-Implement confirmation dialog.

The session bulk actions fire operations in parallel via `runBulk` in
`web-app/src/lib/backlog/bulkActions.ts`, which uses `Promise.allSettled` to collect
fulfilled/rejected outcomes, then calls `formatBulkFeedback(verb, result)` for the toast.

## Key edge cases to handle at implementation time

1. **Eligibility filter is client-side only.** The server will return `CodeAlreadyExists` for
   already-running triage — filter these out before dispatching.
2. **Bulk Auto-Implement needs a 3-bucket result.** The existing `BulkResult` (succeeded/failed) is
   insufficient; add a `queued` bucket or extend `formatBulkFeedback` to accept it.
3. **No server-side fan-out throttle for triage.** Consider limiting parallel triage dispatches to
   avoid resource exhaustion on large selections (e.g. `p-limit` with concurrency 3–5).
4. **Plan gate error for auto-implement.** If an item's plan gate is not passed (e.g. plan
   unapproved), `SpawnSessionFromItem` returns `CodeFailedPrecondition` — that item lands in
   `failed`. The client-side filter (`skipPlanning || planApproved`) must be applied before
   dispatching to keep failures out of the feedback count.
5. **Confirmation dialog for Bulk Auto-Implement.** Follow the existing `BacklogBulkActions`
   delete-confirmation dialog pattern (`styles.overlay` + `styles.dialog` with `role="dialog"` and
   `aria-modal="true"`).
