# Research: Pitfalls — backlog-stuck-item-visibility

Scope: what commonly goes wrong with (a) migrating in-memory notify-once maps to
durable storage, (b) periodic reconciliation/polling systems, (c) "why is X stuck"
dashboards, (d) notification deduplication — grounded in this repo's actual code
(`session/backlog_lifecycle.go`, `server/dependencies.go`, `session/pr_status_poller.go`,
`session/worktree_pr_poller.go`, `session/ent_repository_backlog.go`,
`web-app/src/lib/hooks/useUnfinishedWork.ts`).

---

## 1. Race conditions: reconcile ticker vs. legitimate status transitions

**Current state is already TOCTOU, not atomic.** `EntRepository.TransitionBacklogItemStatus`
(`session/ent_repository_backlog.go:516`) is check-then-act, not a single conditional
SQL statement:

```go
current, err := r.client.BacklogItem.Get(ctx, parsedID)   // 1. read
if precondition != nil && current.Status != precondition.ExpectedStatus {
    return nil, ErrPreconditionFailed                       // 2. check (in Go, not SQL)
}
item, err := r.client.BacklogItem.UpdateOneID(parsedID).
    SetStatus(string(toStatus)).Save(ctx)                   // 3. write (unconditional)
```

Between steps 1 and 3 another goroutine can write. The precondition catches *some*
races (a concurrent write that also happened to check the same precondition), but
two concurrent transitions that don't share a precondition can still interleave —
there is no `UPDATE ... WHERE status = ?` single-statement guard. This existing gap
is the backdrop any new "mark as stuck" write lands on.

**The two existing notify-once reconcilers (`reconcileStaleWorkSessions`,
`reconcileStuckReviewItems`, `backlog_lifecycle.go:602-696`) do not re-verify current
status before firing.** Pattern:

```go
l.stuckReviewNotifiedMu.Lock()
alreadyNotified := l.stuckReviewNotified[item.ID.String()]
if !alreadyNotified { l.stuckReviewNotified[item.ID.String()] = true }
l.stuckReviewNotifiedMu.Unlock()
if alreadyNotified { continue }
// ... notify ...
```

The `item` was fetched by a query (`FindStuckReviewItems`) at the top of the 60s
tick. Between that query and the notify call there is no re-check against the DB.
If a durable "stuck record" write is added here (e.g. `INSERT stuck_state`), the
same gap applies to the write itself: a legitimate transition (e.g. a human
re-triggers review, or `AutoReopenForPRFix` moves the item back to `in_progress`)
could complete in the window between "item queried as stuck" and "stuck record
persisted," leaving a stale stuck-state row pointing at a status the item no
longer has.

**Design against this:**
- Any durable stuck-state write must include the *same* status/updated_at
  precondition idiom already used by `TransitionBacklogItemStatus` — write the
  stuck record only `WHERE item.status = <status observed at detection time>`,
  and no-op (not error) if the precondition fails, since "it already moved on" is
  the correct outcome, not a bug.
  - This means, ideally, the fix goes further than the two-step
  read-then-conditionally-write pattern shown above and uses a single UPDATE ...
  WHERE clause (or ent's equivalent) so the check and the write are atomic at the
  SQL layer — worth flagging to the architecture-review reader even though it's
  the existing house style, since a new entity is a chance to not propagate the gap.
- Clearing stuck-state must be wired into every code path that moves an item off
  the stuck status — not just the reconcile tick. `onSessionExited`,
  `pushAndCreatePR`, `AutoReopenForPRFix`, and manual `TriggerReReview`/status-change
  RPCs all mutate backlog status outside the reconcile tick. If stuck-state
  clearing only happens on the *next* 60s tick's "not stuck anymore" branch, there's
  up to a 60s window where the UI shows an item as stuck when it just started
  making progress — acceptable, but should be a stated tradeoff, not an oversight.
- Prefer keying the stuck-state row by `(item_id, status_at_detection)` or storing
  the observed `updated_at`/status-event-id at detection time, so a stale write
  can be detected and self-corrected on the next tick rather than requiring
  perfect coordination.

---

## 2. ent schema migration risk: forgetting `--feature sql/upsert`

`session/ent/generate.go` pins the required command:
```go
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
```
per `.claude/rules/ent-schema-generation.md`, omitting `--feature sql/upsert`
**compiles fine** and silently produces generated code without upsert methods —
there is no compiler error, just missing functionality (or a runtime error only if
code already calls `.OnConflictColumns(...)` against stale-generated code).

This feature needs upsert semantics ("mark stuck, or update the existing stuck
record's reason/last-seen timestamp") — the canonical in-repo precedent is the
`DiffStats` upsert in `session/ent_repository.go:494-511`:
```go
upsertDiff := diffCreate.
    OnConflictColumns(diffstats.SessionColumn).
    UpdateAdded().
    UpdateRemoved()
```
and `UpsertRule` (`session/ent_repository.go:1240`, `approvalrule.FieldRuleID`).
Both require the `sql/upsert` ent extension to exist as generated methods
(`OnConflictColumns`, `Update...`). If the new stuck-state entity is added without
regenerating with the correct flag:
- `go build` still succeeds today (no new upsert call yet) but any code written
  against the assumption that `.OnConflictColumns` exists **will not compile**,
  which is actually the good case — worse is if a partial/prior generation with
  the flag already ran and a *second* regeneration without the flag silently
  strips the upsert capability from an unrelated entity someone else added
  meanwhile (this flag is global per generate run, not per-schema-file).
- Always run `go generate ./session/ent/...` (which reads the correct pinned
  command) rather than typing `ent generate` by hand, and `go build ./...`
  immediately after to catch drift.
- Since the new entity presumably needs a unique constraint (e.g. one stuck-state
  row per `item_id`, or per `(item_id, reason)`) for `OnConflictColumns` to target,
  get the unique index defined in the schema *before* first generation — adding a
  unique constraint after rows already exist requires a data-cleanup migration
  step too (see §migration data risk below).

---

## 3. Notification storm on first reconcile pass after migration

Direct restatement of the problem statement: **6+ items are already parked in
`review`, one PR (#148) has been mergeable-but-unmerged for 3 days.** If durable
state replaces the in-memory maps by simply "on the first tick after the DB
migration ships, treat every currently-stuck item as newly-detected," the very
first reconcile tick after deploy fires a notification for every one of them
simultaneously — a burst that looks like a fresh mass failure rather than a
backlog of pre-existing issues, and is exactly the kind of notification fatigue
this feature is trying to cure.

**Design against this:**
- On first-ever detection of durable stuck-state (i.e., the migration adds a new
  table with no rows), backfill existing stuck items into the table *without*
  triggering notification — treat "row already exists at deploy time" and "row
  created by this tick" as different code paths, or seed the table in the
  migration itself with a `notified_at` already set (or an explicit
  `backfilled=true` flag suppressing the first notify).
- More generally: notify-once semantics should key off "did the stuck **reason**
  change" not just "does a stuck record exist," otherwise flipping between two
  stuck classifications (e.g. stale-work → stuck-review) either double-notifies
  or never re-notifies depending on which side of the key you chose. Pick one
  and document it.
- The existing dedup keys are `item.ID` only (`staleWorkNotified`,
  `stuckReviewNotified` — both `map[string]bool` in `backlog_lifecycle.go:125,133`).
  A durable version needs to decide whether re-entering the *same* stuck reason
  after having recovered should re-notify (arguably yes — that's a second distinct
  incident) — plain "have we ever notified this item ID" loses that distinction
  the moment it's made durable and permanent instead of reset-by-restart.

---

## 4. GitHub API rate limiting for "positively confirm PR is mergeable"

Good news: this is nearly free to add. `ReconcilePRPending` (`backlog_lifecycle.go:807-890`)
already calls `g.GetPRStatus(item.PrNumber)` once per pr_pending item per tick
(after `IsPRMerged`), and the result already carries `CIFailing`,
`HasBlockingReviews`, `HasConflicts`. The silent-failure root cause is purely a
missing `else` branch:

```go
if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts {
    continue // PR is open and healthy — wait for merge.  ← dead end, no signal
}
```

**Do not add a second API call/poller for this.** The data needed ("green and
mergeable") is already fetched by the existing `GetPRStatus` call inside the
existing 60s tick — implement the new notification as an `else` at that same
call site, with its own notify-once key (e.g. `prReadyNotified[item.ID]`) so it
doesn't fire every tick once true. Adding a parallel/duplicate poller (a
`prMergeReadyPoller` alongside `PRStatusPoller`/`WorktreePRPoller`) would
double the `gh`/GitHub API call volume for the exact same data and risks hitting
`github.DefaultRateLimiter` — both existing pollers already gate on
`github.DefaultRateLimiter.IsLimited()` (`pr_status_poller.go:190`,
`worktree_pr_poller.go:187`) and use per-key backoff (`noPRPollAfter`,
`NoPRBackoff = 5*time.Minute` in both `PRStatusPollerConfig` and
`WorktreePRPollerConfig`) precisely because GitHub secondary rate limits bite
under concurrent `gh` CLI calls. Project memory also flags buf-setup-action
rate-limiting in CI as a related precedent for how easy it is to trip GitHub API
limits without noticing until it's a recurring failure — same risk class here,
avoidable entirely by not adding new API surface.

---

## 5. Frontend streaming RPC pitfalls (if extending `WatchUnfinishedWork` pattern)

Read `web-app/src/lib/hooks/useUnfinishedWork.ts` in full as the template a new
`WatchStuckItems`-style hook would likely copy. Concrete issues in the existing
hook worth NOT repeating:

- **Fixed 3s reconnect timer, no backoff** (`reconnectTimerRef.current = setTimeout(() => startWatch(), 3000)`,
  line ~93-97). If the server is down for an extended period, this reconnects
  every 3s indefinitely — fine for one client, but if this pattern is reused for
  a second stream (stuck items) every open browser tab now independently retries
  every 3s against a downed server. Add capped exponential backoff for any new
  hook, or better, share one reconnect/backoff utility across both hooks instead
  of copy-pasting the raw 3000ms literal.
- **`transport` and `client` are constructed unconditionally on every render**
  (`createConnectTransport(...)` and `createClient(...)` are called at the top of
  the hook body, not inside `useMemo`). They're not gc'd instantly but are wasted
  work on every re-render; `startWatch`'s `useCallback` has an empty dep array so
  it captures whichever `client` existed on mount and never picks up a new one —
  functionally harmless today only because `baseUrl`/interceptors never change
  after mount, but it's a stale-closure trap if a future edit makes `baseUrl`
  dynamic. Wrap `transport`/`client` in `useMemo` for the new hook rather than
  inheriting this.
- **Unbounded client-side `Map` keyed by an identity string** (`worktreeKey =
  repoPath|branch`) that only shrinks on an explicit `worktreeRemoved` event.
  For a stuck-items feed, the server must reliably emit a "removed" event the
  moment an item stops being stuck (transitions off the stuck state) — if that
  removal event is ever dropped (server restart mid-stream, event ordering bug,
  reconnect losing in-flight events), the client map leaks a phantom "stuck"
  entry forever, since there is no TTL/expiry sweep on the client. Consider
  either (a) having the client always trust a full-snapshot-on-reconnect (some
  "initial state" message that replaces the whole map on (re)connect, not just
  deltas) or (b) a periodic reconciliation fetch (a plain unary `ListStuckItems`
  RPC polled every N minutes) that authoritatively replaces the map, so a missed
  delta self-heals instead of persisting indefinitely.
- **No visible loading/error/disconnected state** surfaced to callers — the hook
  only exposes `worktrees`/`lastScanTime`/`isScanning`. A stuck-items dashboard
  whose entire value proposition is "trustworthy signal of what's wrong" should
  not silently show stale data while disconnected; expose a `connectionState`
  (`connected`/`reconnecting`/`error`) so the UI can visibly indicate "this list
  may be out of date" rather than implying false confidence.

---

## 6. Interface pollution risk: "StuckItemDetector"

Per `.claude/rules/interface-pollution-checklist.md`, watch for exactly this shape
in the implementation phase: a `StuckItemDetector` interface with a single
production implementation (e.g. `DefaultStuckItemDetector`) that just wraps the
4 detection queries (`FindStuckReviewItems`, stale-work-session query,
rework-cap-hit query, PR-ready-but-unmerged query) with no second implementation
on the horizon. This is smell #1 (speculative interface) and #4 (forwarding-only
wrapper) simultaneously if each "detect" method just calls straight through to an
`EntRepository` query method.

The existing code does **not** do this — `ReconcileStuck` calls concrete methods
directly (`er.FindStuckReviewItems`, `l.reconcileStaleWorkSessions`,
`er.FindPRPendingItems`) on concrete types, no interface in between. The four new
"stuck reason" classes should follow the same shape: either four concrete
functions/methods on `BacklogLifecycleListener`/`EntRepository` (matching the
existing `reconcileStaleWorkSessions`/`reconcileStuckReviewItems` naming and
placement), or — only if the reason-classification logic itself is genuinely
reused across >1 call site (e.g. both the reconcile tick AND a new `GetStuckReason`
RPC handler need the exact same "why is this stuck" branching) — a plain function
returning a `StuckReason` enum/struct, not an interface. An interface only earns
its place here if a second real implementation (e.g. a test fake beyond the
existing `fakePRPendingChecker`-style pattern) is imminent; even then, per the
checklist, define it in the consumer package (e.g. the ConnectRPC service handler
package), not next to the ent repository implementation.

Also watch for no-op getter/setter proliferation: the existing
`BacklogLifecycleListener` already has ~6 `Set*/get*` pairs each guarded by their
own mutex (`SetNotifier`/`getNotifier`, `SetPRFixSpawner`/`getPRFixSpawner`,
etc. — `backlog_lifecycle.go:100-238`). Adding a 7th for something like
"SetStuckStateStore" should be scrutinized: does it need runtime override
independent of construction (tests need it, per the existing pattern), or can it
be a constructor-only field? The existing pattern exists because tests override
factories on a per-test-run listener instance — that's a legitimate reason, not
speculative — but don't add the Nth one reflexively just because the file already
has six.

---

## Summary of concrete design constraints to carry into planning/ADRs

1. Any new "mark stuck" write must carry the same status-precondition idiom as
   `TransitionBacklogItemStatus`, and must no-op silently (not error, not retry)
   when the item has already moved on — never let a stuck-tick "win" a race
   against a legitimate transition.
2. Regenerate ent via `go generate ./session/ent/...` (not hand-typed `ent
   generate`), and design the stuck-state entity's unique constraint (e.g. one
   row per `item_id`) *before* first generation so `OnConflictColumns` has a
   target from day one.
3. The migration must explicitly backfill-without-notifying for
   already-stuck items at deploy time — otherwise the first tick after ship
   fires a simultaneous notification storm for the existing 6+ items, exactly
   the fatigue this feature exists to prevent.
4. Implement "PR green and mergeable" detection as a new branch on the
   already-fetched `GetPRStatus` result inside `ReconcilePRPending` — do not
   add a second poller or a second GitHub API call for this.
5. If extending the `WatchUnfinishedWork` streaming pattern: memoize
   transport/client, add capped backoff (not a fixed 3s retry), and give the
   client either a full-snapshot-on-reconnect or a periodic authoritative
   `List` fallback so a dropped "removed" event can't leak a phantom stuck
   entry forever.
6. Keep the four stuck-reason detectors as concrete functions/methods
   matching the existing `reconcileStaleWorkSessions`/`reconcileStuckReviewItems`
   shape — do not introduce a `StuckItemDetector` interface unless a second
   real implementation is imminent, and define it in the consumer package if it
   is.
