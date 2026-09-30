# Research 3 — Evidence / Architecture: superseded rework-session resurrection

Ground-truth evidence gathered **read-only** from the live deployed instance on 2026-09-14
(~15:54 UTC / 08:54 PDT). Nothing was written, restarted, stopped or mutated.

Confidence labels: **VERIFIED** = command run and output shown below; **INFERRED** = reasoned
from verified data.

---

## 1. Where the state lives

**VERIFIED.** The live service does **not** use `~/.stapler-squad/sessions.db` (that file is
stale — mtime 2026-09-04). It runs in *workspace* mode. The live state directory is:

```
/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/
├── sessions.db        (1.4 GB, +  -wal / -shm, mtime = now)
├── analytics.db       (4.3 GB)
├── config.json
└── logs/
    ├── staplersquad.log                              (8.4 MB, live)
    └── staplersquad-2026-09-14T{00-42,02-25,04-08,05-52,07-34}*.log.gz
```

Confirmed by mtimes advancing during the investigation and by log content matching the DB.
Read access used `sqlite3 'file:<path>?mode=ro'` throughout — a read-only connection, WAL
included, so the running server was never disturbed.

### Relevant schema (`sessions.db`)

```sql
CREATE TABLE "sessions" (
  `id` integer PRIMARY KEY AUTOINCREMENT,
  `title` text NOT NULL,        -- UNIQUE INDEX sessions_title_key
  `uuid` text NULL DEFAULT (''),
  `path` text NOT NULL, `working_dir` text, `branch` text,
  `status` integer NOT NULL,    -- session.Status enum, see below
  `created_at` datetime NOT NULL, `updated_at` datetime NOT NULL,
  `archived_at` datetime NULL,  -- soft-archive marker
  `session_type` text, `hidden` bool NOT NULL DEFAULT (false),
  `exit_reason` text, `failure_reason` text DEFAULT (''),
  `workflow_id` text, `github_pr_url` text, `github_pr_number` integer,
  ... 50 columns total ...
);
CREATE INDEX session_status ON sessions(status);
CREATE INDEX session_archived_at ON sessions(archived_at);

-- (backlog item, role) association — this is the grouping key for the fix
CREATE TABLE "item_sessions" (
  `id` uuid PRIMARY KEY,
  `session_uuid` text NOT NULL,          -- FK-by-value to sessions.uuid
  `session_role` text NOT NULL,          -- 'work' | 'review' | 'triage' | ...
  `started_at`, `ended_at` datetime, `end_reason` text,
  `backlog_item_item_sessions` uuid NOT NULL,  -- → backlog_items.id
  ...
);
CREATE INDEX itemsession_session_uuid ON item_sessions(session_uuid);
CREATE INDEX itemsession_created_at_backlog_item_item_sessions
  ON item_sessions(created_at, backlog_item_item_sessions);
```

There is also a thinner `backlog_item_sessions(backlog_item_id, session_id)` join table with no
role column. **`item_sessions` is the table a supersession filter must use** — it is the only
place `session_role` lives.

### Status enum (`session/instance.go:39-75`)

| value | name | terminal? |
|---|---|---|
| 0 | `Creating` | no |
| 1 | `Active` | no (live AI process) |
| 2 | `Paused` | no |
| 3 | `Stopped` | yes |
| 4 | `Hibernated` | no |
| 5 | `Restoring` | transient, never persisted |
| 6 | `Crashed` | yes |
| 7 | `PermanentlyFailed` | yes |
| 8 | `Failed` | no (pre-Active creation failure) |

Whole-DB status distribution (144 sessions): `1`→32, `2`→7, `3`→97, `7`→8.

---

## 2. Incident reconstruction — the three backlog items

**VERIFIED.** Query joining `sessions` → `item_sessions`. All 26 rows for the three items:

| id | title | status | role | item (8) | created_at (UTC) | archived_at (UTC) | uuid |
|---|---|---|---|---|---|---|---|
| 1341 | add-durable-guidance-request | 3 Stopped | work | 1c08da73 | 09-12 15:13:50 | 09-12 16:51:51 | b3a054a1-e8c5-442e-88a0-37b741c1d7d8 |
| 1351 | …-r3 | **1 Active** | work | 1c08da73 | 09-12 17:30:20 | **09-12 18:18:11** | 9001a2e3-3542-4d70-91e8-151dd7529960 |
| 1355 | …-r4 | **1 Active** | work | 1c08da73 | 09-12 18:18:11 | **09-12 18:55:15** | e2978dc7-9959-44d6-b718-e753a4920422 |
| 1359 | …-r5 | 3 Stopped | work | 1c08da73 | 09-12 18:55:15 | 09-12 20:55:47 | c2d8c8ea-6a7d-47f8-ab17-9b5ef02f5afa |
| 1372 | …-r6 | **1 Active** | work | 1c08da73 | 09-12 20:55:47 | **09-12 20:55:50** | faee4b8a-7379-4d7d-96eb-ae904710d187 |
| 1373 | …-r7 | **1 Active** | work | 1c08da73 | 09-12 20:55:52 | **09-13 04:56:42** | 9bcb0183-2fe0-48f7-968c-105385055077 |
| 1381 | …-r8 | **1 Active** | work | 1c08da73 | 09-13 04:56:42 | **09-13 04:56:45** | 7be2889c-a511-4c9c-999b-52f251a20583 |
| 1382 | …-r9 | **1 Active** | work | 1c08da73 | 09-13 04:56:46 | **09-14 05:03:41** | 11d36b9a-c3b6-4bbe-8a93-74faf74a67b2 |
| 1390 | …-r10 | 3 Stopped | work | 1c08da73 | 09-14 05:03:41 | 09-14 07:04:57 | b7eb8acd-440a-45f1-8461-63a504245118 |
| 1394 | …-r11 | 3 Stopped | work | 1c08da73 | 09-14 07:04:56 | 09-14 07:29:01 | 38d0b546-e17f-495e-9b24-8389168b88e0 |
| 1395 | …-r12 | 1 Active | work | 1c08da73 | 09-14 07:29:02 | *(none)* | d3313a57-52ac-4d2d-85e0-addc3f93c01a | ← **legitimate current round** |
| 1357 | fix-fork-pressure-flap-and-status-banner-r2 | 3 Stopped | work | cfda07b7 | 09-12 18:31:51 | 09-12 19:42:43 | 035f2c46-15f8-40e3-bc14-2c747cab109f |
| 1364 | …-r3 | **1 Active** | work | cfda07b7 | 09-12 19:42:43 | **09-12 19:57:09** | eba7ec52-5fe1-4fb9-b13e-d023ebebe5f9 |
| 1369 | …-r4 | **1 Active** | work | cfda07b7 | 09-12 20:13:43 | **09-12 20:13:47** | 299385a9-62d4-43f7-8f0e-c343720beb19 |
| 1370 | …-r5 | **1 Active** | work | cfda07b7 | 09-12 20:13:48 | **09-12 20:25:09** | 9275aecb-fb37-4cae-b3fd-f6de45550650 |
| 1376 | …-r6 | **1 Active** | work | cfda07b7 | 09-12 22:14:41 | **09-12 22:14:44** | d74ccf13-1835-42b4-89f1-8733e665d7d0 |
| 1377 | …-r7 | **1 Active** | work | cfda07b7 | 09-12 22:14:45 | **09-14 03:31:52** | 11453fb6-5473-4691-9ae7-53095632d215 |
| 1389 | …-r8 | 3 Stopped | work | cfda07b7 | 09-14 03:31:52 | 09-14 05:33:39 | 68e72c62-0fb4-42f9-aa72-aa78ab370c79 |
| 1391 | …-r9 | 3 Stopped | work | cfda07b7 | 09-14 05:33:40 | 09-14 06:54:06 | 97fdca65-d3ab-454f-9180-d86c05d4d174 |
| 1393 | …-r10 | 3 Stopped | work | cfda07b7 | 09-14 06:54:06 | 09-14 08:55:04 | e28752ab-6b44-4e6c-b54f-2bd671f0091e |
| 1396 | …-r11 | 1 Active | work | cfda07b7 | 09-14 08:55:05 | *(none)* | 11e08157-5d37-4cf6-84f7-471ab2fab125 | ← **legitimate current round** |
| 1358 | gate-request-review-on-ac-completion-r3 | 3 Stopped | work | 4e38d55f | 09-12 18:52:15 | 09-12 19:20:02 | 9d58625b-604b-4cf1-a036-288904773ae4 |
| 1362 | …-r4 | 3 Stopped | work | 4e38d55f | 09-12 19:20:02 | 09-12 19:53:52 | e973e7eb-ab08-47e7-8104-9ab5766e6130 |
| 1367 | …-r5 | **1 Active** | work | 4e38d55f | 09-12 19:53:53 | **09-12 21:54:48** | 2b5c78b8-4713-4fde-b0ea-1d6383621c2d |
| 1374 | …-r6 | **1 Active** | work | 4e38d55f | 09-12 21:54:48 | **09-12 21:54:51** | d20c3abe-41ba-4e39-9a9b-2b2682efa53e |
| 1375 | …-r7 | **1 Active** | work | 4e38d55f | 09-12 21:54:52 | **09-13 03:36:42** | dc491422-9c60-476c-b4e2-96633c909d1c |

Full backlog item ids: `1c08da73-d569-47df-ab9b-676301748159`,
`cfda07b7-73fb-42e1-a21b-7fdf8a052a14`, `4e38d55f-7487-4a95-a520-39741295a5e6`.

**Every one of the three items has exactly one role: `work`.** No review/triage rows are
involved.

### Answer: are the superseded rounds still Active right now?

**CONFIRMED — yes.** 14 sessions are simultaneously `archived_at IS NOT NULL` **and**
`status = 1 (Active)`, which is a contradiction the archive path is supposed to make
impossible (`ArchiveWithStop` sets `ArchivedAt` *and* transitions to `Stopped` "in a single
actor command", per `session/instance_actor_setters.go:266-276`).

```
sqlite> SELECT COUNT(*) FROM sessions WHERE status=1 AND archived_at IS NOT NULL;
14
sqlite> SELECT CASE WHEN archived_at IS NULL THEN 'not-archived' ELSE 'ARCHIVED' END k,
        COUNT(*) FROM sessions WHERE status=1 GROUP BY k;
ARCHIVED      14
not-archived  18
```

All 14 belong to the three incident items. There are **no other** archived-but-Active sessions
in the whole database.

The tmux sessions are live right now too — from the most recent
`DoesSessionExistNoCache` record in `logs/staplersquad.log`, the tmux server currently holds:

```
staplersquad_stapler-squad-add-durable-guidance-request-{r3,r4,r6,r7,r8,r9,r12}
staplersquad_stapler-squad-fix-fork-pressure-flap-and-status-banner-{r3,r4,r5,r6,r7,r11}
staplersquad_stapler-squad-gate-request-review-on-ac-completion-{r5,r6,r7}
```

= exactly the 14 zombies + the 2 legitimate current rounds (r12, r11). One-to-one with the DB.

The `item_sessions` rows for these *have* been ended (`ended_at` set, e.g. the three
gate-request rows all got `ended_at = 2026-09-14 15:53:21`, ~1 minute before this query). So
the **orchestration layer believes these rounds are finished while the session layer keeps
them Active** — the two layers are out of sync, and only the session layer holds the tmux
process.

---

## 3. Blast radius across the whole dataset

**VERIFIED.** Dataset totals: 144 sessions, 1910 `item_sessions` rows, **611 distinct
(backlog_item, role) groups**.

Groups with **more than one** session currently in `status = 1 (Active)`:

| backlog item id | role | live count | sessions |
|---|---|---|---|
| `1c08da73-d569-47df-ab9b-676301748159` | work | **7** | r3, r4, r6, r7, r8, r9, **r12** |
| `cfda07b7-73fb-42e1-a21b-7fdf8a052a14` | work | **6** | r3, r4, r5, r6, r7, **r11** |
| `4e38d55f-7487-4a95-a520-39741295a5e6` | work | **3** | r5, r6, r7 *(no legitimate current round — all three are stale)* |

**3 offending groups out of 611 (0.5%).** 16 sessions in those groups, of which **14 are
stale** and 2 are the genuinely-current rounds (r12, r11). Bold = the one round that should be
Active.

**A one-time backfill is feasible and small: 14 rows.** It is also the only way to clear them
— a code-only fix stops future resurrections but leaves these 14 tmux sessions and DB rows in
place, and the gate-request-review item has *no* current round at all, so it is fully wedged.

### Latent, not yet fired

7 further sessions are `archived_at IS NOT NULL AND status = 7 (PermanentlyFailed)` — armed to
resurrect on the next restart if their tmux session happens to be alive:

| id | title |
|---|---|
| 1366/1371/1379/1384/1385 | `stapler-squad-fix-tmux-stale-session-resume-bypass-{r4,r5,r7,r8,r9}` |
| 1340 | `stelekit-desktop-quick-capture-r2` |
| 1259 | `triage-d7656c58-…-mirror-workspace-log-dir-priority-r3` |

That is a **fourth backlog item** (`fix-tmux-stale-session-resume-bypass`, 5 rounds) one
restart away from the same failure. This is not a one-off.

---

## 4. The resurrection event in the logs

**VERIFIED.** Exactly one server startup exists in the retained log window:

```json
{"time":"2026-09-13T23:52:53.079196106-07:00","level":"INFO","msg":"Building core dependencies (phase 1/3)..."}
```

`2026-09-13 23:52:53 PDT` = **`2026-09-14 06:52:53 UTC`** — this is the `make install-service`
redeploy. It matches the `updated_at` timestamps stamped on the stale rows
(`2026-09-14 06:53:13` for the add-durable/gate cluster).

19 seconds later, the bulk restore fires. **The restore path emits two records per instance**,
from `server/dependencies.go:883` and `:888`:

```json
{"time":"2026-09-13T23:53:12.158515338-07:00","level":"INFO","msg":"Reconcile: session is terminal in DB but tmux is alive — restoring","session":"stapler-squad-fix-fork-pressure-flap-and-status-banner-r3","status":7}
{"time":"2026-09-13T23:53:12.160561027-07:00","level":"INFO","msg":"Reconcile: restored session (was terminal, now Running)","session":"stapler-squad-fix-fork-pressure-flap-and-status-banner-r3"}
```

`"status":7` = `PermanentlyFailed`. These are the exact two lines a fix should suppress, and
the exact two lines to assert on when verifying it.

**13 instances were restored in that burst**, all within 27 ms
(`23:53:12.158` → `23:53:12.185`):

| # | session | status at restore | archived? |
|---|---|---|---|
| 1 | `stapler-squad-fix-fork-pressure-flap-and-status-banner-r3` | 7 | yes |
| 2 | `stapler-squad-gate-request-review-on-ac-completion-r5` | 7 | yes |
| 3 | `stapler-squad-fix-fork-pressure-flap-and-status-banner-r4` | 7 | yes |
| 4 | `stapler-squad-fix-fork-pressure-flap-and-status-banner-r5` | 7 | yes |
| 5 | `stapler-squad-add-durable-guidance-request-r6` | 7 | yes |
| 6 | `stapler-squad-add-durable-guidance-request-r7` | 7 | yes |
| 7 | `stapler-squad-gate-request-review-on-ac-completion-r6` | 7 | yes |
| 8 | `stapler-squad-gate-request-review-on-ac-completion-r7` | 7 | yes |
| 9 | `stapler-squad-fix-fork-pressure-flap-and-status-banner-r6` | 7 | yes |
| 10 | `stapler-squad-fix-fork-pressure-flap-and-status-banner-r7` | 7 | yes |
| 11 | `stapler-squad-add-durable-guidance-request-r9` | 7 | yes |
| 12 | `Knowledge Maintenance — 2026-09-12 23:00` | 7 | **no** |
| 13 | `Knowledge Maintenance — 2026-09-13 23:00` | 7 | **no** |

`add-durable-guidance-request-{r3,r4,r8}` are *not* in this list yet are archived-but-Active
today — **INFERRED**: they were resurrected by an earlier restart whose log has already
rotated out of the 5-file retention window. Their presence in the 06:52 startup's live tmux
list (quoted in `DoesSessionExistNoCache` records from 23:21 PDT, *before* the restart)
confirms they were already Active going in, so Step 6b correctly skipped them as non-terminal.

### Unrelated-but-adjacent log noise

- `reconcileSessions: stopped session found alive, reviving to Active` — **1383 occurrences**
  in the 2026-09-14 logs, from `session/review_queue_poller.go:548`. This is the *steady-state
  poller* doing the same revival repeatedly, paired with 1381 of
  `reconcileSessions: managed session not found in live sessions, transitioning to Stopped`
  (`:505`). That near-equal pairing is a **flap**: the poller stops a session, then revives it,
  ~1380 times in a day. Worth flagging to research agents 1/2 — it may be the same root cause
  reached by a second path, and any fix should be applied to both `dependencies.go:882` and
  `review_queue_poller.go:548`.
- `cold restoring with --resume` — 1762 occurrences. Separate concern.

---

## 5. Architecture note — what a supersession filter would have done

### Where the current code decides

`server/dependencies.go`, background init goroutine, **Step 6b**:

```go
for _, inst := range instances {
    if inst.IsHotRestoreRecoverable() && inst.TmuxSessionExists() {
        log.Info("Reconcile: session is terminal in DB but tmux is alive — restoring",
            "session", inst.Title, "status", inst.GetLifecycleStatus())
        inst.RecoverFromStopped()
        if err := inst.Start(false); err != nil { ... } else {
            log.Info("Reconcile: restored session (was terminal, now Running)", "session", inst.Title)
            ...
        }
    }
}
```

`IsHotRestoreRecoverable()` (`session/instance_state.go:485`) is a pure status switch:

```go
switch i.GetLifecycleStatus() {
case Stopped, PermanentlyFailed, Failed:
    return true
default:
    return false
}
```

**The predicate has no notion of archival and no notion of supersession.** Every terminal
session with a surviving tmux process is unconditionally revived.

### Why the tmux process survives in the first place (the precondition)

`ArchiveSession` / `ArchiveSessionByUUID` (`server/services/session_service.go:6038` and
`:1103`) call `ArchiveWithStop` / `SetArchivedAtIfNilAndStop`, which set `ArchivedAt` and
transition the DB status to `Stopped` — **but never call `inst.Kill()` or `Destroy()`**. The
service has a `StopSessionByUUID` that *does* kill tmux (`session_service.go:1112-1123`), but
the archive path does not use it. So archiving a superseded rework round leaves a fully live
tmux session on disk with a terminal DB row — precisely the shape Step 6b is designed to
"repair". The restore is doing what it was told; the archive is what lied.

This makes it a two-sided bug:
- **Precondition (upstream):** archive leaves the tmux session alive. Same class as the
  2026-07-29 OOM session-leak incident ("archive-on-terminal never killed the tmux pane").
- **Amplifier (downstream):** Step 6b restores anything terminal-with-live-tmux, with no
  archival or supersession guard.

### Counterfactual: `archived_at IS NOT NULL → skip restore`

`ArchivedAt` is already a first-class field on `InstanceSnapshot`
(`session/instance_snapshot.go:139`), so the guard is a lock-free read — no new plumbing.

Applied to the 06:53 UTC restart:

- **Skipped: 11 of 13** — every rework round in the burst (fork-pressure r3/r4/r5/r6/r7,
  add-durable r6/r7/r9, gate-request r5/r6/r7). All 11 were archived.
- **Kept: 2 of 2 legitimate** — both `Knowledge Maintenance` sessions have
  `archived_at IS NULL` and are genuine scheduled sessions that *should* hot-restore. The
  guard does not touch them.

**This filter alone is sufficient for the observed incident: 11 suppressed, 0 false positives.**

### Counterfactual: `skip if a newer instance exists for the same (item, role)`

Evaluated against the state *as of 06:52:53 UTC*, using `item_sessions.created_at` ordering
within each `(backlog_item_item_sessions, session_role)` group:

| item | newest round at restart time | would skip | would keep |
|---|---|---|---|
| `1c08da73` add-durable / work | **r10** (created 09-14 05:03) | r3, r4, r6, r7, r8, r9 | r10 |
| `cfda07b7` fork-pressure / work | **r9** (created 09-14 05:33) | r3, r4, r5, r6, r7 | r9 |
| `4e38d55f` gate-request / work | **r7** (created 09-12 21:54) | r5, r6 | **r7** |

- add-durable: r10 was terminal-with-live-tmux at restart and was *not* in the restore burst
  (its tmux wasn't alive), so the "keep" is a no-op. All 6 zombies suppressed. ✅
- fork-pressure: same — r9's tmux wasn't alive. All 5 zombies suppressed. ✅
- gate-request: **r7 is itself a zombie** (archived 09-13 03:36, no successor ever spawned).
  The newest-wins rule would have kept restoring it forever.

**Verdict: newest-wins is necessary but NOT sufficient.** It reduces 11 restores to 1
(`gate-request-review-on-ac-completion-r7`) but leaves exactly one immortal zombie per item
that has no successor — which is the worst case, because that item is precisely the one whose
pipeline is wedged. It also costs a join against `item_sessions` at startup that the archived
check does not.

### Recommended shape

1. **Primary guard (necessary and sufficient here):** `ArchivedAt != nil` → skip hot-restore,
   and kill the orphaned tmux session instead of adopting it. Cheap, snapshot-local, zero
   false positives on this dataset (11/11 zombies caught, 2/2 legitimate sessions preserved).
2. **Fix the precondition:** make the archive path kill tmux (route through the existing
   `StopSessionByUUID` / `inst.Kill()`), so the zombie never exists. Without this, Step 6d's
   "kill orphaned tmux sessions" still won't help — these rows are *not* orphans, they have DB
   records.
3. **Supersession as defense-in-depth, not the primary rule:** "a newer instance exists for
   the same (item, role)" catches rounds that were superseded without being archived, but must
   not be relied on alone — the `gate-request-review-on-ac-completion-r7` case proves it leaves
   a permanent zombie per item.
4. **Apply to both revival sites.** `review_queue_poller.go:548` runs the same unconditional
   revival 1383 times/day in steady state. A startup-only fix leaves that path open.
5. **One-time backfill (14 rows + 14 tmux sessions).** Required — a code fix cannot retire the
   already-Active rows, and `gate-request-review-on-ac-completion` currently has no live round
   at all.

### Verification handles for the eventual fix

- **Log assertion:** after the fix, a restart with archived-and-tmux-alive sessions must emit
  **zero** `"Reconcile: session is terminal in DB but tmux is alive — restoring"` records for
  archived sessions, while still emitting them for non-archived ones (the `Knowledge
  Maintenance` case).
- **DB invariant:** `SELECT COUNT(*) FROM sessions WHERE status=1 AND archived_at IS NOT NULL`
  must be `0`. It is `14` today. This is a good candidate for a startup consistency check or a
  test-suite invariant.
- **Group invariant:** no `(backlog_item_item_sessions, session_role)` group may have more than
  one `status=1` session. 3 groups violate this today out of 611.
