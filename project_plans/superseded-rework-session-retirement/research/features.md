# Research 2 — Data model / linkage: Instance → backlog item → round → role

All paths relative to the repo root. Line numbers are as of branch
`worktree-agent-ac7e8809d2c28ec21` (base `dc2d775c4`). Every claim below is
VERIFIED by reading the cited file unless explicitly labelled INFERRED.

---

## 1. Instance → backlog item linkage

**Canonical mechanism: the `item_sessions` link table (ent entity `ItemSession`).**

`session/ent/schema/item_session.go:19-101`

```go
field.String("session_uuid").
    Comment("Loose FK to Session; not an ent edge"),
field.String("session_role").
    Comment("One of: work, triage, review"),
...
edge.From("backlog_item", BacklogItem.Type).
    Ref("item_sessions").
    Unique().
    Required(),
```

- The backlog-item side is a **real ent edge** (`backlog_item`, `Unique().Required()`) —
  `session/ent/schema/backlog_item.go:196` (`edge.To("item_sessions", ItemSession.Type)`).
- The session side is a **loose string FK**: `session_uuid` matched against
  `Session.uuid` / `Instance.UUID`. There is deliberately **no ent edge** to `Session`
  (schema comment, line 24). This matters: you cannot join from an `Instance` to a
  backlog item in one ent query; you look up `item_sessions.session_uuid`.
- Indexes (`session/ent/schema/item_session.go:104-111`): `session_uuid`, and
  composite `created_at` + `backlog_item` edge. **There is no index on
  `session_role`**, and none on `(backlog_item, session_role)`.

**There is NO `backlog_item_id` column on the `Session`/`Instance` entity.**
`session/ent/schema/session.go` has no backlog field; `Instance`/`InstanceSnapshot`
carry no `BacklogItemID` (grep over `session/instance.go`,
`session/instance_snapshot.go`: no hits).

**Legacy / apparently-dead second mechanism:** a `BacklogItem ↔ Session` many-to-many
edge exists in the schema — `session/ent/schema/backlog_item.go:197`
(`edge.To("sessions", Session.Type)`) and its back-ref
`session/ent/schema/session.go:192-194` (`edge.From("backlog_items", …).Ref("sessions")`).
A grep for application-level use of that edge (`AddSessionIDs`, `AddSessions(`,
`WithSessions()`, `QuerySessions()`, `HasBacklogItems`) outside generated `session/ent/`
returns **zero non-test hits**. Treat it as vestigial; do not build on it.

**Naming convention is NOT the linkage.** Session titles do encode item identity
(`<repo-slug>-<short-title>[-rN]` for work, `review:<item-id-prefix>` for review),
but that is display-only. Authoritative Instance→item resolution is:

- `session/storage_backlog.go:351` `GetItemSessionBySessionUUID(ctx, sessionUUID)` —
  "session_uuid is not unique across records … order by created_at desc, take first",
  loads the `BacklogItem` edge so `BacklogItemID` is populated.
- `session/storage_backlog.go:367` `GetItemSessionBySessionAndItem(ctx, uuid, itemID)`.
- `session/ent_repository_backlog.go:2786` `GetAllItemSessionsWithBacklogInfo` — full
  join, newest-first, used by the Insights dashboard.

Note the "session_uuid is not unique" caveat: one tmux session UUID can legitimately
have more than one `item_sessions` row (e.g. attach after spawn), so any
Instance→(item, role) resolution must pick deterministically (the repo's convention:
newest `created_at` wins — see ADR-029 note at `ent_repository_backlog.go:2780-2785`).

---

## 2. Round number ("r3", "r10")

**Not a field. It is a title suffix, computed at spawn time from a COUNT of prior
work-role `ItemSession` rows, and never persisted as a number anywhere.**

`server/services/backlog_service_triage.go:1578-1591`:

```go
// buildRevisionTitle returns the session title for a backlog work session. On reopen
// (isReopen=true) it appends "-rN" where N is one past the existing work-session count.
func buildRevisionTitle(baseTitle string, isReopen bool, priorSessions []session.ItemSessionSummary) string {
	if !isReopen {
		return baseTitle
	}
	workCount := 0
	for _, s := range priorSessions {
		if s.Role == string(session.SessionRoleWork) {
			workCount++
		}
	}
	return fmt.Sprintf("%s-r%d", baseTitle, workCount+1)
}
```

Call site — `backlog_service_triage.go:944-949`:

```go
shortTitle := triageShortTitle(priorSessions, item.Title)
baseTitle := slugify(filepath.Base(item.RepoPath)) + "-" + shortTitle
title := buildRevisionTitle(baseTitle, isReopen, priorSessions)
```

Consequences for our predicate:

- The `-rN` suffix equals the session's **1-based ordinal among work-role rows for the
  item, ordered by `created_at`** (`ListItemSessions` orders `Asc(created_at)` —
  `session/storage_backlog.go:303`). The first spawn from `ready` status has
  `isReopen=false` and therefore **no suffix** (ordinal 1). A first spawn that happens
  to be a reopen (`item.Status == in_progress`) gets `-r1`.
- `isReopen` is derived purely from item status:
  `backlog_service_triage.go:574` — `isReopen := item.Status == string(session.BacklogStatusInProgress)`.
- Rows created by `AttachSessionToItem` are also role `work`
  (`server/services/backlog_service_sync.go:106`) and therefore count toward N.
- **Review sessions carry no round marker at all**: every review round is titled
  `"review:"+item.ID[:8]` (`server/services/session_service.go:1579`), and
  `TriggerReReview` titles its session `"re-review:"+slugify(item.Title)`
  (`backlog_service_triage.go:2967-2968`). So round identity for `review` role can
  only come from `created_at` ordering, never the title.

**Recommendation (INFERRED, design opinion):** derive the round from row ordinal in
`ListItemSessions` (which is ordered and role-filterable) rather than parsing `-rN`
out of the tmux title. Title parsing is lossy (no suffix on round 1, no suffix at all
on review rounds) and drifts if a row is ever hard-deleted.

---

## 3. Session roles

Definition — `session/backlog.go:48-54`:

```go
// Session role constants.
const (
	SessionRoleWork      = "work"
	SessionRoleTriage    = "triage"
	SessionRoleReview    = "review"
	SessionRoleJulesWork = "jules_work"
)
```

These are **untyped string constants**, not a named type (`string(session.SessionRoleWork)`
conversions appear in places, but they are no-ops). The ent column is a plain
`field.String("session_role")` whose comment (`item_session.go:26`) is stale — it still
says "One of: work, triage, review" and omits `jules_work`.

| Role | What it is | Can several legitimately coexist for one item? |
|---|---|---|
| `work` | A tmux-backed Claude Code agent doing the implementation for one rework round. | **No, not concurrently** — actively guarded: `spawnSessionAfterGates` step 8b (`findActiveWorkSession`, line 917) and step 8b2 (`findConfirmedLiveWorkSession`, line 929) both return `CodeAlreadyExists`. Many *ended* work rows accumulate (one per round) — that pile-up is the thing this project targets. |
| `review` | A hidden, tmux-backed review-gate session that writes a `ReviewVerdict`. Spawned per review round; also synthesized as non-tmux audit rows (headless re-review, manual verdict). | **No concurrently** — `HasActiveReviewSession`/`findActiveReviewSession` (`backlog_service_triage.go:1560-1576`) guard `AutoRespawnReview`. But a `review` row can coexist with a live `work` row (that is the normal reopen flow: `AutoReopenAfterFailedReview` explicitly leaves the work session alive). Multiple ended review rows accumulate too. |
| `triage` | A bounded, **headless one-shot** subprocess call (UUID prefix `headlessTriageUUIDPrefix`), not a tmux session — nothing to kill. | **No concurrently** — `tombstoneOrphanTriageSessions` returns `CodeAlreadyExists` if one is genuinely live (`backlog_service_triage.go:3110-3153`). |
| `jules_work` | Work running on Google Jules' infrastructure; the row starts life as a *reservation* with UUID prefix `julesPendingUUIDPrefix` (`server/services/jules_dispatch_service.go:323-328`). Not tmux-backed. | **No concurrently** — `findActiveJulesSession` blocks both local spawn (step 8b-Jules, line 936) and `AutoRespawnAutonomousWork` (line 1938). Mutually exclusive with a live local `work` session. |

`session.IsTmuxBackedSessionRole(role)` (`session/backlog.go:76-78`) is the single source
of truth for "has a real pane to kill": **`work` and `review` only**. This is the
predicate any retirement sweep should reuse for the kill side.

---

## 4. The spawn paths — is spawning purely additive?

**Claim REFUTED.** Spawning already performs several mutations on prior-round sessions.
All four named functions funnel into the same place, so they inherit identical behavior:

```
SpawnSessionFromItem (RPC, backlog_service_triage.go:509)  ─┐
DequeueNextQueuedItems                                      ├─> spawnSessionAfterGates (:855)
AutoReopenAfterFailedReview (:1658)   ─> SpawnSessionFromItem ┘
AutoRespawnAutonomousWork   (:1909)   ─> SpawnSessionFromItem
AutoReopenForPRFix          (:2102)   ─> autoReopenForPRFix (:2115) ─> SpawnSessionFromItem
```

### What `spawnSessionAfterGates` does to prior rounds

| Step | Line | Effect on previous rounds |
|---|---|---|
| 8a | `:905` `tombstoneOrphanWorkSessions` | Sets `EndedAt` on any open work row that `IsSessionLive` says is dead, **and prunes its worktree** (`:3082-3084` → `cleanupItemWorktrees`). Mutates the in-memory slice too. |
| 8a2 | `:914` `killEndedWorkSessionPanes` | `KillTmuxPaneOnly` on every **already-ended** work row (`:3094-3106`). Pane only — worktree is shared across `-rN` rounds. |
| 8b / 8b2 / 8b-Jules | `:917`, `:929`, `:936` | Refuse the spawn if a work/live-work/Jules session is still active. |
| 12c | `:1082` `cleanupItemWorktreesExcept(prior, worktreePath)` | **Only when `isReopen`.** Deletes prior work sessions' worktrees except the reused one. |
| 12c | `:1089` `archiveItemWorkSessions(prior)` | **Only when `isReopen`.** For every prior row whose role is tmux-backed (`work` or `review`): `ArchiveSessionByUUID` + `KillTmuxPaneOnly` (`server/services/backlog_service.go:1187-1202`). The doc comment at `:1180-1185` explicitly frames this as "rework respawns, where only the sessions loaded *before* the new spawn … are passed in". |

So the existing retirement mechanism is **`archiveItemWorkSessions` on the reopen path**.
It is (a) gated on `isReopen`, (b) driven by the snapshot of `priorSessions` loaded at
`:893`, and (c) best-effort/log-only. Any new "retire superseded rounds" predicate should
be checked against this function first — it is the incumbent, and the likely place to
either fix or generalize rather than duplicate.

### Per-function notes

- **`spawnSessionAfterGates`** (`:855-1117`) — creates role `work`
  (`:1054-1062`, `SessionRole: session.SessionRoleWork`); title from
  `buildRevisionTitle` (`:949`); tags `backlog:work` (+ `backlog:revision` if reopen,
  + `autonomous` if autonomous) at `:986-992`. Acts on prior rounds as tabulated above.
- **`AutoReopenAfterFailedReview`** (`:1658-1897`) — transitions `review → in_progress`,
  calls `tombstoneOrphanWorkSessions` itself (`:1686`), then **either** reuses the live
  work session (`:1827-1848`, no spawn at all) **or** calls `SpawnSessionFromItem` with
  `Autonomous: true` (`:1860`). Rework cap counted as `workCount >= effectiveReworkCap`
  (`:1792-1802`). Does not itself archive/kill prior rounds — inherits step 12c.
- **`AutoRespawnAutonomousWork`** (`:1909-1965`) — no status transition (item already
  `in_progress`); `tombstoneOrphanWorkSessions` (`:1933`); bails if a work or Jules
  session is active; same rework cap; `SpawnSessionFromItem` `Autonomous: true` (`:1956`).
- **`AutoReopenForPRFix`** / `AutoReopenForPRFixWithKnownSession` → `autoReopenForPRFix`
  (`:2115-2242`) — requires `pr_pending`; `tombstoneOrphanWorkSessions` (`:2144`); if a
  work session is active it **steers** it instead of spawning
  (`steerActiveSessionForPRFix`, `:2153`); otherwise transitions to `in_progress`,
  temporarily prepends PR-fix context to item notes, spawns, restores notes, rolls back
  on failure.

### Other spawn / ItemSession-creation entry points these four miss

Grep of `CreateItemSession(` / `CreateItemSessionWithVerdict(` (non-test, non-repository):

| Site | Role | Notes |
|---|---|---|
| `server/services/backlog_service_sync.go:103` `AttachSessionToItem` | `work` | Attaches a **pre-existing** user session. Does **not** call `tombstoneOrphanWorkSessions`, does **not** archive prior rounds, and does **not** go through `buildRevisionTitle` — the attached session keeps its own title, so it has no `-rN` at all yet still increments the work count for the next round's suffix. **Biggest blind spot for a title-parsing approach.** |
| `server/services/backlog_service_triage.go:2989` `TriggerReReview` (spawned path) | `review` | Title `re-review:<slug>`. Kills any stale tmux session with the same title first (`:2974-2976`). |
| `server/services/backlog_service_triage.go:2860`, `:2896` `TriggerReReview` (headless path) | `review` | Synthetic UUIDs (`headlessReReviewUUIDPrefix`), immediately ended; audit/verdict rows, no tmux pane. |
| `server/services/backlog_service_trigger_triage.go:321` | `triage` | Headless triage call. |
| `server/services/backlog_service_lifecycle.go:1212` (manual verdict RPC) | `review` | Synthetic UUID `manual-review-<item8>-<nanos>`; verdict-only row. |
| `session/review_gate.go:444` `spawnReviewGate` | `review` | The main review-gate spawn (`SpawnReviewSession`, title `review:<item8>`). |
| `server/services/jules_dispatch_service.go:324` | `jules_work` | Reservation row, UUID prefix `julesPendingUUIDPrefix`. |
| `server/services/backlog_debug_seed_handler.go:385,447,605,718` | various | Debug/seed handler only. |

Also relevant, a **fifth mutation path that is not a spawn**:
`RemediateStaleWorkSession` (`backlog_service_triage.go:2010-2077`) explicitly ends the
stale work `ItemSession` **before** killing its pane (BUG-064 ordering, documented at
`:1991-2009`) and then delegates to `AutoRespawnAutonomousWork`. And
`forceResetItem` (`:1122-1144`) stops + ends every open `work`/`review` row when
`SpawnSessionFromItem` is called with `Force=true`.

Terminal-item cleanup (not spawn-triggered): `CleanupTerminalItem`
(`server/services/backlog_service.go:1159-1167`) → `cleanupItemWorktrees` +
`archiveItemWorkSessions` over **all** sessions; and the 60s safety-net sweep
`reconcileTerminalItemSessions` in `session/backlog_lifecycle_archive.go`
(both keyed off `session.IsTmuxBackedSessionRole`).

---

## 5. Existing "current session for this item + role" helper

There is **no** single `GetCurrentSession(item, role)` helper. What exists, all in
`server/services/backlog_service_triage.go` and all operating on an already-loaded
`[]session.ItemSessionSummary` slice:

| Helper | Line | Predicate |
|---|---|---|
| `findActiveWorkSession` | `:1193` | first row with `Role == work && EndedAt == nil` |
| `hasActiveWorkSession` | `:1184` | `findActiveWorkSession != nil` |
| `findConfirmedLiveWorkSession(stopper, …)` | `:1210` | as above **plus** `stopper.IsSessionLive(uuid)` — OS/tmux truth, not the DB column |
| `findActiveReviewSession` / `HasActiveReviewSession` | `:1560` / `:1574` | `Role == review && EndedAt == nil` |
| `findActiveJulesSession` | `:1231` | `Role == jules_work && EndedAt == nil` |
| `session.HasActiveJulesSession` | `session/backlog.go:84` | same, exported |
| `BacklogService.HasActiveWorkSession(ctx, itemID)` | `:2084` | the only ctx-taking, storage-querying wrapper — `ListItemSessions` then `findActiveWorkSession` |
| `latestTriageSession(sessions)` | `session/backlog_lifecycle_triage.go:73` | **the only "latest by CreatedAt regardless of EndedAt" helper in the codebase** — triage-role only |

**Recommendation for the fix's predicate (INFERRED):** the shape to reuse/generalize is
`latestTriageSession` — "max `CreatedAt` among rows of role R" — parameterized by role,
rather than `findActive*`, which answers a different question ("is one still open?").
A superseded round is then exactly: `Role == R && CreatedAt < latestForRole(R).CreatedAt`.
Note the two notions diverge in the normal case: after `AutoReopenAfterFailedReview`
reuses a live work session, the latest work row is also the active one; but a dead-but-
not-tombstoned row from round N-1 is neither latest nor `EndedAt != nil`.

Combine with `session.IsTmuxBackedSessionRole` before doing anything that kills a pane,
and prefer `KillTmuxPaneOnly` over `StopSessionByUUID` — the latter runs
`CleanupWorktree`, and rework rounds **share one worktree/branch** across all `-rN`
rounds (`backlog_service.go:1109-1119`, `backlog_service_triage.go:951-959`,
`backlog_service_triage.go:3087-3093`).

---

## 6. Query capability: (item id, role) → its instances

**Yes, efficiently for the item; role filtering is done in Go, not SQL, on the main path.**

| Method | File:line | Shape |
|---|---|---|
| `ListItemSessions(ctx, itemID)` | `session/storage_backlog.go:294-315` | `Where(itemsession.HasBacklogItemWith(backlogitem.ID(id))).WithReviewVerdict().Order(Asc(created_at))`. **Backed by the composite index** `index.Fields("created_at").Edges("backlog_item")`. Returns all roles; callers filter by `Role` in a loop. This is the workhorse — every spawn path calls it. |
| `GetItemSession(ctx, id)` | `:274` | by `item_sessions.id` |
| `GetItemSessionBySessionUUID(ctx, uuid)` | `:351` | by `session_uuid` (indexed), newest-first, `WithBacklogItem()` |
| `GetItemSessionBySessionAndItem(ctx, uuid, itemID)` | `:367` | both, newest-first |
| `GetAllItemSessionsWithBacklogInfo(ctx)` | `session/ent_repository_backlog.go:2786` | **full table scan** (carries `//nolint:entfullscan`), newest-first, `WithBacklogItem()` |
| `ListOpenJulesItemSessions(ctx)` | `session/storage_backlog_jules.go:43-68` | **the one existing role-filtered SQL query**: `Where(itemsession.SessionRole(SessionRoleJulesWork), itemsession.EndedAtIsNil())` — proves `itemsession.SessionRole(...)` predicates are available and usable |
| `CountJulesItemSessionsSince(ctx, since)` | `session/storage_backlog_jules.go:~78` | role + `created_at >= since` + UUID-prefix / end-reason exclusions |
| `GetBaseCommitSHAsForSessions(ctx, uuids)` | `session/storage_backlog.go:327` | `SessionUUIDIn(...)` |

**So: no full scan is needed.** Given an item id, `ListItemSessions` is one indexed
query returning every round of every role, already ordered by `created_at` ascending —
enough to compute both "which round is this" (ordinal among role-filtered rows) and
"is it the current round" (is it the last role-filtered row). If a dedicated
`(itemID, role)` query is wanted, `ListOpenJulesItemSessions` is the template; note
there is **no index on `session_role`**, so such a query would be index-assisted on the
edge and filtered on role — adding `index.Fields("session_role").Edges("backlog_item")`
would be the schema change if profiling justifies it (it almost certainly does not:
rounds per item are in the single/low-double digits).

Every method above is exposed through the `Storage` facade (`session/storage.go`)
with the same names, which is what `BacklogService.storage` holds.

---

## Summary of gotchas for the fix

1. `Instance` has no backlog FK — resolution is always via `item_sessions.session_uuid`,
   and that column is **not unique** (attach-after-spawn); pick newest `created_at`.
2. Round number exists only as a title suffix and only for `work` role; review rounds
   are indistinguishable by title. Derive round from row ordinal, not the string.
3. `AttachSessionToItem` creates a `work` row with an arbitrary user-chosen title —
   any title-parsing heuristic breaks on it.
4. Spawning is **not** purely additive: `tombstoneOrphanWorkSessions`,
   `killEndedWorkSessionPanes`, `cleanupItemWorktreesExcept` and
   `archiveItemWorkSessions` already run. The gap is that the archive step is gated on
   `isReopen` and is best-effort, not that nothing exists.
5. All `-rN` rounds **share one worktree and one branch** — never use
   `StopSessionByUUID` (it runs `CleanupWorktree`) to retire a round; use
   `KillTmuxPaneOnly`, as the existing code does.
6. `triage` and `jules_work` rows have synthetic UUIDs with known prefixes
   (`headlessTriageUUIDPrefix`, `headlessReReviewUUIDPrefix`, `julesPendingUUIDPrefix`,
   `manual-review-`) and no live Instance — `IsTmuxBackedSessionRole` is the gate.
