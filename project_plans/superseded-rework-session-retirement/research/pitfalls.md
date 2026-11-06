# Research 4 — Pitfalls / Risk: retiring superseded rework-round sessions

Scope: what can go wrong with (a) archive-on-supersede and (b) a startup skip-restore
filter, grounded in this repo's code and incident history. All file references are to
this worktree (`.claude/worktrees/agent-ac7e8809d2c28ec21`), same paths as `main`.

Confidence labels: **VERIFIED** = source opened / command run and cited.
**INFERRED** = reasoned from code read, not executed.

---

## 0. Finding that reframes the whole fix (read first)

**Archive-on-supersede already exists.** VERIFIED —
`server/services/backlog_service_triage.go:1081-1090` (`spawnSessionAfterGates` step 12c)
already calls `s.archiveItemWorkSessions(ctx, priorSessions)` on every `isReopen` spawn,
and `archiveItemWorkSessions` (`server/services/backlog_service.go:1187-1202`) already
does exactly the proposed thing: `ArchiveSessionByUUID` + `KillTmuxPaneOnly` for every
work- **and** review-role prior session. Step 8a2
(`backlog_service_triage.go:907-914`, `killEndedWorkSessionPanes` at `:3094`) separately
kills the pane of every already-ended round.

`isReopen` is `item.Status == in_progress` (`backlog_service_triage.go:574`), and every
`-rN` round is by construction a reopen (`buildRevisionTitle` only appends `-rN` when
`isReopen` is true, `:1580-1591`), and `DequeueNextQueuedItems` hardcodes `true`
(`:817`). So the archive path *should* already be firing on every round.

**Implication for the plan:** writing a second archive-on-supersede is likely to be
(i) dead-on-arrival duplication that the `dupl` new-code gate will flag, and (ii) a fix
without a root cause, which this repo's own engineering rules forbid. The real question
to answer before writing any code is **why the existing archive didn't stick** for the
~17 restored sessions. Candidate hypotheses, all UNVERIFIED, each cheaply falsifiable
against `~/.stapler-squad/logs/staplersquad.log`:

1. `archiveItemWorkSessions` is a silent no-op when `s.sessionStopper == nil`
   (`backlog_service.go:1188-1190`) — returns with **no log line at all**.
2. Both its calls are best-effort: failures are logged at WARN
   (`[archiveItemWorkSessions] failed to archive session=…`) and swallowed. Grep the log
   for that exact string first.
3. `ArchiveSessionByUUID`'s storage-only fallback has a documented re-save race
   (`server/services/session_service.go:1078-1098`) where a live instance resuming
   concurrently can end up re-saved; if the live instance's snapshot still has
   `ArchivedAt == nil`, the periodic `SaveInstances` sweep would clobber the archive.
4. The zombie sessions may not be backlog work sessions at all (review sessions, or
   sessions whose `ItemSession` rows were never created/were orphaned) — in which case
   no amount of archive-on-supersede helps.

Answer (1)-(4) before choosing a design. If the answer is (1) or (2), the fix is
observability + a retry, not a new archival path.

---

## 1. Prior incidents in this repo — the guards we must not regress

| Incident | What happened | Guard added | Where |
|---|---|---|---|
| **2026-09-12 duplicate-session-into-shared-worktree** | `AutoReopenAfterFailedReview`'s reuse check and `spawnSessionAfterGates` step 8b both trusted `FindLiveInstance(...) != nil` (in-memory poller map). A still-running work session transiently dropped out of that map; both concluded it was dead and spawned a **duplicate session into the same worktree the original was still writing to**. Operator had to kill tmux panes by hand. | `findConfirmedLiveInstance` (map fast path → shadow-Instance OS/tmux truth check); `OtherLiveSessionInsideWorktree`; `findConfirmedLiveWorkSession` (8b2); `noliveinstanceraw` analyzer | `server/services/session_service.go:1126-1230`, `backlog_service_triage.go:920-932`, `tools/lint/noliveinstanceraw/analyzer.go`, commit `da956b477` (PR #804) |
| **#791 / #799 stale-liveness (tmux layer)** | `HasSession()` pointer alone was treated as "alive"; `RestoreWithWorkDir` relaunched over a still-running process | `IsAlive()` required alongside `HasSession()`; "confirm prior process is dead before relaunch" | commits `08c134f5b`, `533118e71`, `c747f18ea` |
| **BUG-027 `stop_session` skips `ItemSession.EndedAt`** | An operator stop tore down tmux + worktree but left `EndedAt = null` forever; `onSessionExited` never ran, so no review-verdict processing, no auto-ship, no stale-work remediation | New `EventStopped` lifecycle event fired unconditionally from `Destroy()`; `instanceBacklogListener` handles `EventExited`/`EventStopped` identically | `docs/bugs/fixed/BUG-027-stop-session-skips-item-session-ended-at.md` |
| **BUG-064 stale-work remediation races `onSessionExited`** | Killing the pane before ending the `ItemSession` let `onSessionExited`'s goroutine transition the item to `review` first, so the respawn no-op'd and the item went back to review with the same already-PARTIAL diff | **Ordering rule**: `UpdateItemSessionEnded` MUST run *before* `KillTmuxPaneOnly` | `backlog_service_triage.go:1991-2009` |
| **Reopen deleted the worktree out from under the new session** | `cleanupItemWorktrees` ran unconditionally on reopen; because rounds share one deterministic worktree path, it deleted the directory the brand-new session had just started using — item left with no worktree, diffs and codebase-read review both empty | `cleanupItemWorktreesExcept(…, worktreePath)` exempts the live path | `backlog_service.go:1109-1148` |
| **BUG-045 review read the shared main checkout** | With the item's worktree gone, `resolveCodebaseWorkDir` fell back to `repoPath` — the shared main checkout — and handed the reviewer *someone else's uncommitted work*, producing a confident, completely wrong FAIL | Fallback must resolve `exists=false` rather than silently degrade to `repoPath` | `docs/bugs/fixed/BUG-045-…md` |
| **BUG-057 worktree-creation failure landed the session in the live checkout** | A git-managed repo whose worktree creation failed fell back to `repoPath`; the agent then edited/committed against the real working tree of `main` | `resolveSessionPath` only falls back when the repo isn't git-managed at all | `backlog_service_triage.go:1593-1612` |
| **2026-07-29 OOM: session leak** | Archive-on-terminal set `ArchivedAt` but never killed the tmux pane — dozens of superseded work *and review* sessions kept their `claude` process + MCP subprocess fleet alive | `archiveItemWorkSessions` now archives **and** `KillTmuxPaneOnly`s; `IsTmuxBackedSessionRole` | `backlog_service.go:1169-1202` |
| **BUG-034 unfinished-scanner never removes completed repos** | Deleted worktrees kept being rescanned forever | `repoWatchRemover.RemoveRepo` after a *successful* cleanup only | `backlog_service.go:1140-1147` |
| **Service restart destroys every tmux session** | Linux systemd unit lacked `--tmux-keep-server`; every restart rebuilt all sessions from scratch, losing all scrollback and in-flight state | `--tmux-keep-server` in `install_linux()` | `docs/explanation/tmux-keep-server-on-restart.md` |
| **macOS restart orphans** | `launchctl bootout` didn't block; up to 4 `stapler-squad` binaries ran concurrently against the same tmux server and `sessions.json`, cancelling sessions one-by-one | Documented pre-restart check procedure (no code guard) | `docs/explanation/service-restart-orphan-process.md` |
| **Policy: never force-stop a slow-but-alive agent** | Explicit repo policy, stated at `backlog_service_triage.go:1421-1426`: "a live session is never stopped, killed, or bypassed here… killing the session ourselves would just trade one bug for a worse one" | Detection + notification only (`StuckReasonReworkBlockedStale`) | `backlog_service_triage.go:1405-1448` |

**The pattern across all of these:** every single one is "we concluded a session was dead
/ superseded and acted destructively on that conclusion, and the conclusion was wrong."
That is exactly the shape of the fix being proposed. The bug we are fixing (17 zombie
processes after a restart) costs RAM; the failure mode of this fix costs *work*.

---

## 2. PR #804's guards — what they defend, and how our fix must compose

VERIFIED from `git show da956b477` and the sources.

### `findConfirmedLiveInstance` (`server/services/session_service.go:1151-1171`)

The canonical liveness-truth check. Map fast path (`FindLiveInstance`); on a **miss**, it
does *not* conclude dead — it rebuilds a read-only shadow `Instance` from persisted data
(`session.FromInstanceDataDeferred`: no `Start()`, no PTY, no goroutines) bound to the
same tmux identity and asks `IsBackendProcessAlive()`. Only dead-on-both-signals returns
nil. Backs `IsSessionLive`, `KillTmuxPaneOnly`, `StopSessionByUUID`.

- **Guards against:** a transient live-poller-map miss being read as "session is dead,"
  which is what let a duplicate session spawn into a shared worktree on 2026-09-12.
- **Documented caveat we must respect:** `FindInstanceDataByID` loads with `LoadMinimal`,
  so **a shadow instance's worktree metadata is empty** — it is explicitly *not* a source
  of truth for worktree deletion (`session_service.go:1146-1150`). Any new code that
  decides to delete/clean a worktree must not route the path through a shadow instance.
- **Our fix MUST:** ask liveness only via `IsSessionLive`/`findConfirmedLiveInstance`,
  never via a raw `FindLiveInstance(...) != nil`. If we add a new
  liveness/kill-decision method on `SessionService`, add its name to
  `monitoredFuncNames` in `tools/lint/noliveinstanceraw/analyzer.go:67-71` — the
  analyzer is a *watchlist*, not a blanket ban, so a new method is unguarded by default.

### `OtherLiveSessionInsideWorktree` (`session_service.go:1193-1230`) + `refuseIfWorktreeSharedWithOtherLiveSession` (`server/mcp/tools_lifecycle.go:350-362`)

Walks every live poller instance and asks `GetCurrentWorkingDirectory()` — a **live
pane/process introspection, not the persisted `Path` field** (which can report the repo
root rather than the worktree the session is really in) — and blocks if any other live
session's real cwd resolves inside the target worktree. Wired into `pauseSession` and
`stopSession`, both of which delete the target's worktree.

- **Guards against:** destroying the worktree a sibling rework round is still working in.
- **Our fix MUST:** if it ever calls `StopSessionByUUID` / `Instance.Kill()` /
  `Destroy()` on a superseded round, it must first go through
  `OtherLiveSessionInsideWorktree`. Better: **don't**. Use `KillTmuxPaneOnly`
  (`session_service.go:1244-1260`) like `archiveItemWorkSessions` already does —
  `Instance.KillSession()` closes the pane and leaves the worktree alone
  (`session/instance_tmux.go:586-594`), whereas `Instance.Kill()` runs `CleanupWorktree`.

### `findConfirmedLiveWorkSession` (`backlog_service_triage.go:1210-1224`)

Step 8b2's concurrent-liveness cap. Independent of both the rework cap and the 8b
`EndedAt`-column guard: re-asks OS truth via `IsSessionLive` for every open work-role
`ItemSession`. Nil stopper ⇒ returns nil (never blocks) — the file's conservative-nil
convention.

- **Our fix MUST:** not undo this. If archive-on-supersede runs *before* the 8b/8b2
  guards, it could mark a genuinely-live prior round as ended/archived and thereby
  *disarm* 8b2, reintroducing the 2026-09-12 duplicate-spawn. Ordering matters:
  the existing code archives at **step 12c, after** the guards and after the new session
  is persisted. Keep it there.

---

## 3. Failure modes of archive-on-supersede

### 3.1 Racing/concurrent rounds — can we archive the *newer* one?

- `spawnSessionAfterGates` reads `priorSessions` once at step 8
  (`:893`) and archives that snapshot at step 12c (`:1089`). Between those two points it
  does worktree resolution, file writes, session spawn, instance persist, and
  `CreateItemSession` — a long window with I/O and subprocess calls. A second spawn that
  starts during that window loads its own `priorSessions` that may already contain the
  first spawn's new session row. **INFERRED risk:** a naive "archive everything except
  the newest" rule computed from a *stale* list can archive the newest.
- The existing code sidesteps this by construction: it archives only
  `priorSessions` (the list loaded **before** the new session existed), never the new
  one — see `archiveItemWorkSessions`'s doc comment, `backlog_service.go:1180-1185`.
  **Any redesign must preserve this "pass the pre-spawn snapshot, never re-query"
  property.** Re-querying `ListItemSessions` at archive time is the bug.
- `s.worktreeMu` (`:972-980`) only covers `resolveSessionPath` + `writeSessionFilesLocked`.
  It does **not** cover the read-at-8 / archive-at-12c window. Two concurrent
  `SpawnSessionFromItem` calls for the same item are otherwise gated only by the 8b/8b2
  `AlreadyExists` guards, which are themselves TOCTOU-prone (`transitionWithGuard`
  handles the item-status side, not the session side).

### 3.2 The "previous round" is actually still doing useful work

- What survives: the worktree and everything on disk. `KillTmuxPaneOnly` →
  `Instance.KillSession()` → `pm().Close()` only closes the tmux session
  (`session/instance_tmux.go:586-594`). Uncommitted changes in the shared worktree stay
  on disk, and rounds share one worktree, so the *next* round inherits them.
- What is lost: the agent's in-flight turn (an LLM response mid-generation, an uncommitted
  multi-file edit the agent had planned to finish, its conversation context beyond what
  the JSONL holds), the tmux scrollback of that pane, and — because rounds share the
  worktree — a half-applied edit becomes indistinguishable from the next round's starting
  state. That is the BUG-045-adjacent hazard: the next round reads a working tree it
  didn't create.
- **Repo policy says don't do this unilaterally** (`backlog_service_triage.go:1421-1426`).
  The guard: only archive a round that `IsSessionLive` reports **dead**, or that
  `TimeSinceLastMeaningfulOutput` reports stale past `maxReworkBlockStaleness` (15min),
  and never one whose worktree `IsDirty()`. There is prior art for a dirty-check gate in
  `SessionRetentionSweeper.baseSafeToDelete` (`session_retention_sweeper.go:136-157`).
- **Delayed-destruction hazard (HIGH, easy to miss):** archiving starts a **14-day
  retention clock**. `SessionRetentionSweeper` (`server/services/session_retention_sweeper.go`,
  hourly tick, `defaultSessionRetentionDays = 14` at `config/types.go:121`) deletes
  archived sessions past the window via `svc.DeleteSession`, whose `Destroy()` removes the
  worktree. Its `sessionSafeToDelete` has a shared-worktree convergence rule
  (`:162-208`): a sibling only blocks while it is *not itself* independently eligible.
  **So if we over-archive — archive the live round by mistake — every sibling becomes
  eligible together and the shared worktree is deleted 14 days later, far from the code
  that caused it.** The dirty-worktree and open-PR checks are the only things standing
  between an over-eager archive and permanent loss.

### 3.3 Round-number parsing / `r10` vs `r9`

- **Nothing in the codebase parses `-rN` back out of a title.** VERIFIED: grepped
  `server/`, `session/`, `web-app/src` for `-r[0-9]`, round regexes, and int parses —
  the only hits are prose in doc comments (`backlog_service_triage.go:1415`, `:1976`;
  `session/backlog_lifecycle_stale.go:23`). `buildRevisionTitle`
  (`backlog_service_triage.go:1580-1591`) *writes* `-r%d` from
  `workCount+1` and nothing ever reads it back.
- **Therefore: do not introduce a round-number parse.** Ordering by `-rN` from a title
  is a brand-new lexicographic-vs-numeric footgun (`"…-r10" < "…-r9"` as strings) with
  no existing need. Order by `ItemSessionSummary.CreatedAt` (a real `time.Time`, always
  set at `CreateItemSession`) or by the `ItemSession.ID` the storage layer already
  returns in insertion order.
- Secondary: the title is not even a stable key across rounds. `triageShortTitle`
  (`:469-486`) re-derives the short title from the *most recent completed triage
  session*, and falls back to a slug of `item.Title` — which can change between rounds
  (`cleanupItemWorktreesExcept`'s doc comment at `backlog_service.go:1118-1119` calls out
  exactly this: "the item's title changed between rework rounds"). So `baseTitle` itself
  can differ between `…-r3` and `…-r4`. Title-derived grouping is unreliable.

### 3.4 Sessions with no backlog-item link — the mass-archive footgun

**This is the single most dangerous design mistake available here.**

- If the grouping key is `ItemSession.BacklogItemID`, you are safe by construction:
  `ListItemSessions(ctx, itemID)` is already scoped per item, and an `ItemSession` row
  cannot exist without an item.
- If the grouping key is derived from the **`Instance`** side, it is catastrophic:
  VERIFIED, `session.Instance` / `session.InstanceData` have **no backlog-item field at
  all** (grepped `BacklogItemID`/`backlog_item_id` across `session/instance*.go` and
  `session/storage.go` — zero hits). The link lives only in the `ItemSession` table,
  reachable via `storage.GetItemSessionBySessionUUID(uuid)`, which returns
  `(zero-value, nil)` — **no error** — when the session isn't backlog-linked. That is the
  exact silent-empty-string shape that caused BUG-045.
  A `map[itemID][]*Instance` built that way buckets **every non-backlog session on the
  machine** (manual sessions, workflow sessions, `create_session` MCP sessions, the
  operator's own live Claude Code session) under key `""`, and "keep only the newest per
  key" then archives and pane-kills all but one of them.
- **Mandatory guard:** explicitly `continue` on an empty `BacklogItemID` (and on an empty
  `SessionUUID` — `archiveItemWorkSessions` and `cleanupItemWorktreesExcept` both already
  do this, `backlog_service.go:1122`, `:1192`). Add a test that a session with no item
  link is never touched. The retention sweeper models the correct handling:
  `if err != nil || is.BacklogItemID == "" { return true, "" } // not backlog-linked`
  (`session_retention_sweeper.go:205-208`).

### 3.5 Role conflation — retiring a review or Jules session

- `archiveItemWorkSessions` deliberately covers **both** work and review roles
  (`IsTmuxBackedSessionRole` = `work || review`, `session/backlog.go:76-78`), because the
  caller contract is "every prior round *including* the review session whose FAIL verdict
  triggered this reopen." That contract holds only because it is called from two specific
  places (terminal transition, and reopen-after-verdict).
- **Risk:** if the new fix widens the trigger — e.g. archive-on-every-spawn, or a generic
  "supersede" sweep — it will kill **in-flight review sessions**. A review session that
  hasn't yet written its verdict has real state to lose, and `HasActiveReviewSession`
  (`backlog_service_triage.go:1574`) exists precisely so `AutoRespawnReview` doesn't
  double-spawn one. Any new retirement rule must filter to `SessionRoleWork` only, or
  re-establish the "verdict already written" precondition the current doc comment relies
  on (`backlog_service.go:1177-1180`).
- `SessionRoleJulesWork` is **not** tmux-backed, so `IsTmuxBackedSessionRole` skips it.
  But `findActiveJulesSession` (`:1231-1238`) gates spawns on it. A retirement sweep that
  ends Jules `ItemSession` rows would disarm that guard and let a local session compete
  with a Jules run on Google's infra. Filter Jules rows out explicitly.
- Triage sessions have their own tombstone path (`tombstoneOrphanTriageSessions`,
  `:3110`) with headless-specific liveness (`IsTriageLive`, `headlessTriageUUIDPrefix`).
  A generic sweep that doesn't know about headless UUIDs will mis-classify every headless
  triage session as dead.

### 3.6 Ordering constraint inherited from BUG-064

If the fix ends `ItemSession` rows as part of retirement: `UpdateItemSessionEnded` must
run **before** `KillTmuxPaneOnly`. Killing the pane fires `EventStopped` →
`BacklogLifecycleListener.onSessionExited` in its **own goroutine**, which unconditionally
transitions an `in_progress` item to `review` unless it observes `EndedAt` already set
(`backlog_service_triage.go:1991-2009`). Reversing the order reintroduces BUG-064: the
round is killed, the item silently lands back in review with the same diff, and no fresh
work session is ever spawned. Also note the `silenttransition` analyzer flags
`UpdateItemSessionEnded` calls whose error is only logged — see §6.

---

## 4. Failure modes of the startup skip-restore filter

### 4.1 Is "skip restore" recoverable, or does the session vanish?

**It depends entirely on which state the skipped instance is left in — and the naive
implementation leaves it unrecoverable.** VERIFIED:

- Boot loads *everything*, archived included: `Storage.LoadInstances`
  (`session/storage.go:335+`) applies no archived/status filter.
- The cold-restore loop is `server/dependencies.go:853-867` — for each instance,
  `if !inst.Started() { inst.Start(false) }`, staggered 200ms.
- The **existing** mechanism that suppresses restore is `ArchivedAt != nil`:
  `fromInstanceData` sets `instance.started.Store(true)` for an archived, Stopped
  instance and skips the tmux liveness probe entirely
  (`session/instance_serialization.go:497-511`). Step 6 then skips it because
  `Started()` is true. Archived sessions are also skipped by the review-queue poller
  (`session/review_queue_poller.go:755-771`).
- Archived is **recoverable**: `ListSessions` hides them only when
  `!req.Msg.IncludeArchived` (`server/services/session_service.go:1979`), and
  `UnarchiveSession` (`:6049-6064`) clears `ArchivedAt`. So route (a) — archive —
  degrades gracefully.
- A **new, ad hoc** "skip restore because a newer sibling exists" filter that merely
  skips `Start()` without setting `ArchivedAt`/`Stopped` leaves the instance
  `Active`-but-`!Started()`. That state has **no recovery affordance**:
  `Instance.Resume()` returns `"can only resume paused instances"` unless
  `Status == Paused` (`session/instance.go:1996-2003`), and Step 6b's hot-restore only
  covers `IsHotRestoreRecoverable()` statuses (Stopped/Failed/PermanentlyFailed,
  `dependencies.go:881-892`). The session shows in the list, looks live, and cannot be
  started or resumed. **Recommendation: do not add a bespoke skip; reuse `ArchivedAt`,
  which the whole codebase already understands.**

### 4.2 "Newest wins" with no liveness check

- Nothing in a startup filter as described asks whether the newest instance is actually
  functional. A newest round that crashed at spawn (worktree creation failed, tmux
  start failed, `claude` binary missing) would suppress restoration of the older round
  that was the real live one — and Step 6b's hot-restore would then not fire for the
  older one either, since we suppressed it.
- The repo's own answer to this exact question already exists and must be reused:
  `findConfirmedLiveInstance` for liveness, `Instance.TmuxSessionExists()` +
  `IsHotRestoreRecoverable()` for the boot-time variant (`dependencies.go:882`).
  "Newest wins" without one of those is the 2026-09-12 bug with the sign flipped.
- Ordering hazard at boot: the filter would run **before** Step 6, but Step 6b's
  hot-restore runs after and would happily revive a session the filter meant to suppress
  — unless the filter sets `ArchivedAt` (which `fromInstanceData` and Step 6b both
  respect) rather than a transient in-memory flag.

### 4.3 Clock skew / equal timestamps / NULL `created_at`

- `ItemSessionSummary.CreatedAt` is a non-pointer `time.Time`
  (`session/backlog.go`, struct printed in research) — a missing/NULL column
  deserializes to the **zero time**, which sorts *oldest*, so a row with a lost timestamp
  is treated as the superseded one and archived. That is fail-dangerous, not fail-safe.
- `StartedAt` and `EndedAt` *are* `*time.Time` and can legitimately be nil.
- Two rounds created inside the same clock tick (or after an NTP step backwards) tie.
  With a strict `>` comparison a tie means *neither* is newest → nothing is archived
  (fail-safe); with `>=` or a sort-and-take-last, the choice is arbitrary
  (fail-dangerous). **Pick `>` / explicit "strictly newer" semantics, and tiebreak on the
  monotonically-increasing `ItemSession.ID`, not on the timestamp.**
- Cross-host skew is real here: `ItemSessionData.ClaimantHostID` exists precisely because
  more than one stapler-squad process/host can claim sessions
  (`session/backlog.go`, `ItemSession.claimant_host_id` schema comment), and
  `docs/explanation/service-restart-orphan-process.md` documents four concurrent
  processes writing the same state. A timestamp written by a different process is not
  comparable to one written by ours.
- Guard to add: never archive anything whose `CreatedAt.IsZero()`.

---

## 5. Testing constraints our new tests must satisfy

From `.claude/skills/deterministic-fast-tests/SKILL.md`,
`.claude/skills/fix-flaky-tests-dont-defer/SKILL.md`, and
`docs/explanation/test-io-storage-isolation.md`.

**Must do**

- **In-memory DB:** `session.NewTestEntRepository(t)` (`session/testing.go`) — named
  shared-cache in-memory SQLite, unique per call, parallel-safe. Never a
  `t.TempDir()`-backed file.
- **State-dir isolation:** `envtest.NewIsolatedStateDir(t)` (`envtest/envtest.go`)
  instead of hand-writing `t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())`. Call it
  **before** `t.Parallel()` on the same `t`, or don't combine them.
  `envtest.ClearAmbientStaplerSquadStateEnv()` is already wired into
  `server/services`'s `TestMain`.
- **`t.Parallel()` by default** on new tests unless they mutate shared state.
- **Inject time, don't sleep on it.** Pass a `time.Time`/duration parameter the way
  `shouldSkipWorkTombstoneForRestartGrace(is.CreatedAt, serverStartTime, time.Now())`
  (`backlog_service_triage.go:3060`) and `shouldAttributeTombstoneToShutdown`
  (`:3156+`) already do — both are pure and side-effect-free **specifically so the
  decision table is directly testable**. Our supersede-decision predicate should follow
  the same shape: a free function taking `(sessions, now, bootTime)` and returning which
  rows to retire, with the I/O left to the caller.
- **Fake the executor, not tmux.** PR #804's own tests (`server/services/liveness_consolidation_test.go`)
  answer liveness and pane-cwd through an `executor.Executor` double so every case is
  deterministic with no tmux server — copy that. Only the one shadow-alive case there
  needs a real tmux session on a per-process isolated socket
  (BUG-075 / `tmuxsocketscope`).
- **`SessionStopper` is an interface** — `server/services/liveness_consolidation_test.go`
  already has stopper doubles (nil stopper, ended-but-reported-live, non-work role,
  open+confirmed-live). Reuse them.
- **Prove it fails pre-fix.** PR #804 and BUG-027 both did this explicitly (revert just
  the guard, watch the test go red). Do the same, and say so in the PR.
- **Fix, don't defer, any flake you hit** (`fix-flaky-tests-dont-defer`). No
  "known pre-existing flake, unrelated."

**Must not do**

- No `time.Sleep` / real timeouts to make an assertion settle.
- No new `t.Setenv`-based fixture where a parameter would do.
- No large-N `Test` to show scale — that's a `*_bench_test.go` `Benchmark`.
- No test that writes into the real `~/.stapler-squad` (the doc records 27,578 real files
  accumulated from exactly that mistake).

---

## 6. Repo gates that will block the PR

`make ready` = `make ci` + `ready-complexity-gate` + `ready-duplication-gate-web` +
web-app lint/CSS/scanner/ci-gates (`Makefile:951-965`).

| Gate | Threshold / scope | Relevance here |
|---|---|---|
| **`dupl`** | 150 tokens, **new-code-only** via `--new-from-rev=origin/main` (`.golangci.yml:52-63`, `Makefile:967-975`) | **Highest risk.** A second archive/retire loop will look near-identical to `archiveItemWorkSessions`, `killEndedWorkSessionPanes`, `tombstoneOrphanWorkSessions`, and `cleanupItemWorktreesExcept` — four existing same-shaped loops over `[]ItemSessionSummary`. Extract, don't copy. Requires `dupl` from `make install-tools`; requires a fresh `origin/main` (`git fetch origin main`). |
| **`gocyclo`** | `min-complexity: 25` | `spawnSessionAfterGates` is already a ~260-line, heavily-branched function. Adding branches in place will trip this. |
| **`gocognit`** | `min-complexity: 40` | Same. Applies to *modified existing* functions too, not just new ones. |
| **`funlen`** | `lines: 150`, `statements: 100`, `ignore-comments: true` | `spawnSessionAfterGates` (`:855-1117`) is right at the edge. Add a helper, don't inline. |
| **`revive` file-length-limit** | `max: 1000` lines, skipping comments/blanks | `backlog_service_triage.go` is >3000 lines; `backlog_service.go` is large too. Both are presumably grandfathered by the new-code-only scoping, but **a new file is cleaner and safer than growing either.** Precedent: commit `864a710d4` "split top complexity×churn hotspots into dedicated files," and `601b4550e` "extract TriggerTriage/CancelTriage into their own file." |
| **`make lint-custom`** | `entfullscan`, `hotpolllog`, `nocommandpattern`, `noliveinstanceraw`, `norawexec`, `norawghrequest`, `norawgitopen`, `silenttransition`, `tmuxsocketscope` (`Makefile:797`) | Four of these bite here: **`noliveinstanceraw`** (no raw `FindLiveInstance(...) != nil` in a liveness/kill decision; add any new such method to `monitoredFuncNames`); **`silenttransition`** (a `TransitionBacklogItemStatus`/`UpdateItemSessionEnded` error that is only logged needs `//nolint:silenttransition` with a reason — see the existing one at `backlog_service_triage.go:3070`); **`entfullscan`** (any new ent `.All(ctx)` with no `.Where`); **`tmuxsocketscope`** (any new tmux command must build args via `tmux.ResolveSocket(...).Args(...)`). |
| **`actor-field-guard`** | `Makefile:998-1010` — greps for `inst.Field = …` in `session_service.go`, `review_queue_poller.go`, `pr_status_poller.go`, `autonomous_driver.go`, `daemon.go` | Route every `*Instance` mutation through an actor setter; read via `Snapshot()` (`.claude/rules/instance-lock-free-reads.md`). |
| **`make registry-generate` / `registry-diff`** | `make ci` runs `registry-generate`; `quick-check` runs `registry-diff` | Only if we add/rename a proto RPC or move a `// +api:`/`// +feature:` marker. The proposed fix should need **no new RPC** — if a design calls for one, that is a signal it's too big. Note the git status already shows 5 uncommitted `docs/registry/features/backend/*.json` from other work; don't sweep them in (`git add -A` is banned in this repo). |
| **`make test-race`** | part of `make ci` | The archive path touches live `*Instance` state from a spawn goroutine while pollers read it. Expect `-race` to be the gate that finds a bug here. |
| **`make nil-safety` / NilAway** | `make analyze` | `findConfirmedLiveInstance` returns `nil`; `GetItemSessionBySessionUUID` returns a zero value with a nil error. |
| **jscpd (web-app)** | absolute 0.12% ratchet | Only if the fix touches `web-app/`. It shouldn't. |

**Commonly trips people up**

- `--new-from-rev=origin/main` needs a reachable, current `origin/main` — `git fetch
  origin main` first or the gate reports nonsense.
- `dupl` must be installed (`make install-tools`); without it the local gate silently
  differs from CI.
- Do **not** commit generated `session/ent/*.go` or `gen/` output; `git add -f` on ent
  output has broken `main` before (CLAUDE.md).
- `make install-service` must **not** be used to try the change by hand — it restarts the
  live `:8543` instance and kills every tmux session. Use the manual port block
  (`PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test …/manual-builds/manual-1/stapler-squad --tmux-keep-server`).
  This matters more than usual here: reproducing "zombie sessions after restart" is
  *literally* the thing that destroys live work.

---

## 7. Recommended risk posture (summary)

1. **Root-cause why the existing `archiveItemWorkSessions` didn't retire these rounds
   before writing a second one.** Grep the log for `[archiveItemWorkSessions] failed to`
   and for its absence entirely (nil `sessionStopper`).
2. Prefer **route (a) only**, at the existing step-12c call site, reusing `ArchivedAt` —
   it is the one signal `fromInstanceData`, Step 6/6b, the review-queue poller, and
   `ListSessions` all already honor, and it is reversible via `UnarchiveSession`.
3. If a boot-time filter is still wanted, make it **set `ArchivedAt`**, not a bespoke
   skip flag, and gate it on `TmuxSessionExists()` / `findConfirmedLiveInstance` rather
   than on recency alone.
4. Never retire anything that is `IsSessionLive`, has a dirty worktree, has an empty
   `BacklogItemID`, has a zero `CreatedAt`, or is not `SessionRoleWork`.
5. Never widen from `KillTmuxPaneOnly` to `StopSessionByUUID`/`Kill`/`Destroy`.
6. Keep the decision logic a pure function of `(sessions, now, bootTime)` so it is
   table-testable with no tmux, no disk, and no sleep.
