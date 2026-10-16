# Pitfalls Research — Durable Guidance Request

Agent 4 (Pitfalls), SDD Phase 2. Repo: `tstapler/stapler-squad`, DB: SQLite via
`mattn/go-sqlite3` (`session/ent_repository.go:175`, confirmed by
`entsql.OpenDB(dialect.SQLite, db)`).

## Primary case study: ADR-001 (backlog-stuck-item-visibility)

`project_plans/backlog-stuck-item-visibility/decisions/ADR-001-durable-stuck-state-storage-model.md`
is the closest prior art in this repo for "durable, notify-once, restart-surviving
state," and Guidance Request should copy its shape almost verbatim, not merely
"be inspired by" it.

**The bug it fixed:** `BacklogLifecycleListener` (`session/backlog_lifecycle.go:119-133`)
tracked notify-once dedup in in-memory `map[string]bool` fields
(`staleWorkNotified`, `stuckReviewNotified`). On every process restart (15+/day in
this dev instance) the maps reset, so "already notified" and "since when" context
was lost — causing repeated or lost notifications. This is structurally the same
failure mode a naive Guidance Request "already notified originating session"
flag would have if it lived in a Go struct field instead of a row.

**The fix, and what to copy directly:**
- One ent entity, one row per logical key, **resolve-in-place** (reopen an
  existing row, never insert a duplicate for the same key) — not an append-only
  event log. The ADR explicitly rejected an append-only design because **SQLite
  treats `NULL` as distinct in unique indexes**, so a key like
  `(item_id, reason, resolved_at NULL)` permits unlimited concurrent "open" rows
  and can't be an `OnConflictColumns` target. A Guidance Request analog: if
  answered/cancelled requests are ever kept as history rather than resolved
  in-place, don't put a nullable "resolved_at"-style column in the same unique
  index that governs "only one pending request of key X."
- A **plain, fully-non-null 2-column unique index** doubling as both the
  `OnConflictColumns` upsert target and the correctness guarantee (`index.Fields("item_id", "reason").Unique()` in `session/ent/schema/backlog_stuck_state.go:90`).
- **Atomic upsert, not check-then-act**, verified in
  `session/ent_repository_backlog.go:1901-1965` (`MarkStuck`): a single
  `tx.BacklogStuckState.Create().OnConflictColumns(...).Update(...).Exec(ctx)`
  inside a transaction, followed by a separate conditional `UPDATE ... WHERE
  resolved_at NOT NULL` reopen step that only ever touches an already-resolved
  row (so it can't race the upsert). No SELECT-then-INSERT anywhere in the path.
- **Best-effort precondition, not atomicity, for cross-entity guards.** `MarkStuck`
  reads the parent `BacklogItem.Status` and pre-filters on it (line 1917:
  `if current.Status != string(expectedStatus) { return false, nil }`), but the
  ADR is explicit that "the plan does **not** claim a tick can never win a race" —
  a **self-heal sweep** on every reconcile tick is the actual correctness backstop,
  not the precondition check. Guidance Request's ownership/status checks (item not
  archived, session still owns the scope) should adopt the same posture: check
  as a best-effort pre-filter at write time, then add a sweep/reconciler that
  corrects state the checks missed, rather than trying to make the whole
  operation cross-table-atomic in SQLite.
- **Idempotent resolve.** `ResolveStuck` (line 1972) is `UPDATE ... SET
  resolved_at=now WHERE resolved_at IS NULL` and returns whether a row was
  *actually* resolved — answering a Guidance Request should follow the same
  shape (`UPDATE ... SET answer=..., answered_at=now WHERE answered_at IS NULL`),
  so a double-answer race is a no-op on the loser, not a second write.
- **Notify-once as its own nullable timestamp field**, separately settable from
  "resolved," via its own idempotent no-op-safe setter (`MarkStuckNotified`,
  line 1996) — not folded into the same write as the state transition. This
  matters because "answered" and "originating session notified" are genuinely
  different facts that can each independently fail/retry.

## 1. Create-if-not-exists / dedup race conditions (Go + ent + SQLite)

- **Naive failure mode:** `SELECT ... WHERE item_id=? AND status='pending'` then
  `INSERT` if nothing found. Two concurrent MCP tool calls (e.g. automated
  triage halting twice in a race, or a retried RPC) both pass the SELECT before
  either INSERT commits, producing two "pending" rows for the same scope —
  exactly the bug class this feature is supposed to prevent for guidance
  requests, and the same shape ADR-001 fixed for stuck-state.
- **Correct pattern (as above):** `Create().OnConflictColumns(<key fields>).Update(...)`
  in one statement, with the uniqueness enforced by a real unique index, not
  application-level "check first." ent requires the `sql/upsert` feature flag
  (already enabled repo-wide per this repo's `session/ent/generate.go` comment
  and reused by `MarkStuck`) — confirm the generate command
  (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`)
  stays in the regenerate step; the CLAUDE.md for this repo calls out that
  omitting `--feature sql/upsert` "breaks UpsertRule and similar methods."
- **SQLite-specific gotcha:** don't build the uniqueness key from any nullable
  column (see ADR-001's NULL-distinctness rejection of Option "episode
  history"). If Guidance Request needs "one open request per (scope_type,
  scope_id, question_kind)," all three columns must be NOT NULL, or the unique
  index silently stops deduplicating for the NULL case.
- **Second race to check for, not covered by ADR-001 (no precedent in this
  repo):** the *answer* path itself. If "answer" is a separate mutation from
  "create," two concurrent answers to the same request (two Slack clicks, or a
  UI submit racing an MCP `submit_triage_result`-style call) must be guarded by
  the same `UPDATE ... WHERE answered_at IS NULL` idiom `ResolveStuck` uses —
  a plain `UPDATE ... SET answer=?` with no status guard silently allows the
  second answer to overwrite the first with no error and no signal to either
  caller that they lost the race.

## 2. Notification delivery to a possibly-not-running process

- This repo's existing notification layer (`server/notifications/store.go`) is
  a JSON-file-backed store (not ent-backed) keyed by `SessionID`, with dedup by
  `(sessionID, notificationType)` and a documented special case for
  `APPROVAL_NEEDED` where "collapsing into an existing unread record updates the
  record ID to the incoming UUID" (`store.go:24-29`) specifically so outcome-
  stamping can still find the right record — a useful precedent for wiring
  "guidance answered → notify" without creating a second unread card per
  question.
- **Known, tracked gap in this exact class of problem:** per repo memory
  (`project_backlog_wakeup_polling.md`), sessions currently learn about backlog
  changes via `ScheduleWakeup` **polling**, not a subscription to
  `WatchBacklogItems` — tracked as `ssq#428`. A "durably notify the originating
  session when answered" design should not assume push delivery exists; it must
  work under polling (session wakes up periodically and checks "is my pending
  guidance request answered yet") as the baseline, with any push-style
  notification purely a redundant fast path — otherwise "the session was
  suspended when the notification hook fired" reproduces the exact
  restart-loses-state failure ADR-001 fixed, just for delivery instead of dedup.
- **Notifying a session that no longer exists.** A session can be archived,
  stopped, or its worktree deleted between "guidance request created" and
  "answered." The notify path needs the same terminal-state check
  `IsTmuxBackedSessionRole` added for the 2026-07-29 OOM fix
  (`project_2026_07_29_oom_session_leak_fix.md` — archive-on-terminal previously
  didn't kill the tmux pane): don't fire-and-forget a wakeup/write to a session
  actor that's gone, and don't treat "failed to deliver because session is
  archived" as a reason to keep retrying forever — resolve the notification as
  undeliverable and surface that on the backlog item instead (see
  `feedback_document_ai_decisions_in_edge_cases.md`: self-heal/auto-close style
  actions must post a visible comment, not act silently).
- **Double notification on replay/self-heal.** If a self-heal sweep (as
  ADR-001 uses for stuck-state) is added to catch missed notify-once writes, it
  must re-check the durable `notified_at`-equivalent field before re-firing —
  otherwise every sweep tick re-notifies every answered-but-not-yet-cleared
  request, recreating the exact bug ADR-001 exists to kill, just moved into the
  new sweep instead of the old map.

## 3. Feature-flag pitfalls for automated-pipeline control flow

- This repo's flag plumbing (`server/interceptors/feature_flag_interceptor.go`)
  is a live-read-per-request gate at the RPC boundary (`isEnabled()` called on
  every request, "flag changes are reflected immediately without a server
  restart" — line 17), and repo memory confirms rollout flags are
  live-settable, never env vars or precondition-baked-in
  (`feedback_rollout_flags_live_settable_no_env_vars.md`). That solves "flag
  read is stale" for RPC handlers, but **triage is a background pipeline, not a
  request/response RPC** — the flag must be re-read at the specific instant the
  halt-vs-continue decision is made inside `server/services/backlog_service_triage.go`,
  not cached once at the start of a (possibly long-running) triage pass. Caching
  it risks: flag flipped ON mid-run after triage already decided to guess, or
  flipped OFF after a request was created but before triage checks whether to
  wait for it — leaving a created-but-orphaned request with a flag-disabled
  pipeline that will never look at it again.
- **A flag that's "on" but the halted item has no resume path.** The dangerous
  failure mode isn't the flag being wrong; it's the flag being correctly ON, a
  request correctly created, and then nothing in the reconcile/triage loop ever
  re-checking "is my guidance request answered" for that item — the same class
  of gap as `ssq#428`'s wakeup-polling reliance. Treat "does every item halted
  behind a guidance request eventually get re-examined" as its own testable
  invariant, independent of the flag's on/off state, because turning the flag
  back off doesn't retroactively unstick items already halted while it was on.
- **Per-scope override precedence.** No existing per-scope (per-item/per-
  session) flag override mechanism was found in `server/features/flags.go` or
  `config/types.go` (flags there are global, e.g. `terminal:resync-visibility-scope`
  gates *what it affects*, not *who it applies to*) — if the plan introduces a
  per-scope override (e.g. "triage halting off for item X specifically"), there
  is no existing precedent in this repo for override-precedence resolution
  (global vs. per-item), so that logic needs its own explicit precedence rule
  and test, not an assumption borrowed from elsewhere in the codebase.

## 4. Cap / rate-limit pitfalls

- **TOCTOU on the cap check itself.** `effectiveReworkCap`
  (referenced from `session/backlog_context.go:93`) is this repo's existing
  precedent for a per-scope numeric cap, but it's read-then-compare against a
  count, not atomically enforced at the DB layer as far as the grep surfaced —
  worth confirming during planning whether `ReworkCapOverride`
  (`session/ent_repository_backlog.go:1001`) enforcement has ever had a
  concurrent-creation race, since if it does, Guidance Request's "per-scope cap
  on pending requests" must not copy that same read-then-insert shape. The
  correct pattern is the same one used for uniqueness: compute a `COUNT(*)
  WHERE scope=? AND answered_at IS NULL` inside the same transaction as the
  insert, or better, a `CHECK`-style guard enforced by retrying the insert and
  re-validating the count post-commit (SQLite doesn't support a partial-count
  constraint natively) — two concurrent creates that both read count=N-1 before
  either commits must not both succeed and produce N+1.
- **Off-by-one at the boundary.** Decide explicitly whether the cap is "reject
  the request that would make count > cap" or "count >= cap" — a one-line
  inversion silently allows one more than intended or blocks the last
  legitimate slot; write the boundary case as its own test per
  `sdd:4-validate`'s coverage-mapping step.
- **Cap silently blocking legitimate escalation.** If automated triage hits the
  per-scope pending cap and simply drops/refuses a new question instead of
  surfacing that refusal, triage effectively resumes guessing — defeating the
  entire point of "halt instead of guess." The cap-exceeded path must itself be
  a visible, notified condition (again, `feedback_document_ai_decisions_in_edge_cases.md`),
  not a silent no-op.

## 5. UI pitfalls — one shared component in 3 embedding contexts

- **Stale state after the question is answered elsewhere.** Because the same
  logical request renders in backlog-item detail, triage panel, and session
  view simultaneously, whichever view didn't submit the answer needs a
  read-after-write path that actually invalidates its cached "pending" state —
  a polling-refetch-on-focus or a subscription, not an assumption that only one
  view is ever open. Given the confirmed `ssq#428` gap (polling instead of
  subscribing to backlog updates), the realistic baseline is: each view must
  independently re-poll/re-fetch the request's current status rather than
  relying on a push event, or a user who resolves a question from the triage
  panel will see the backlog-item-detail view still showing an open form,
  inviting a duplicate/conflicting answer that then hits the double-answer
  race in §1.
- **Dynamically-typed form accessibility.** One component branching between
  yes/no, multiple-choice, and short-answer needs to swap not just the input
  control but the accessible name/role/description together — a `<fieldset>`/
  `<legend>` (or ARIA `role="radiogroup"` + `aria-labelledby`) per question,
  not a bare set of buttons relabeled by JS, or screen readers announce stale
  "button" semantics when the question type changes without a full re-render.
  Keyboard focus must land on the *new* control type's first focusable element
  after a type switch, not persist on a now-removed node (a focus-loss bug the
  `ui-web-design-guidelines`/`ux:review` skills would flag directly — invoke
  those during Phase 3/6 review of this component).
- **Mobile + desktop parity** (per `feedback_mobile_desktop_ux.md`, a standing
  repo instinct): the triage panel and session view are likely tighter/denser
  contexts than backlog-item detail — verify multiple-choice touch targets and
  short-answer keyboard behavior are checked in the narrowest of the three
  embedding contexts, not just the roomiest.

## 6. Ownership-check pitfalls

- **No existing ownership-check precedent for "does session X own scope Y" was
  found in this repo's ent schemas** — `ApprovalRule` (`session/ent/schema/approvalrule.go`)
  has no owner/session FK at all (it's a global rule table), so Guidance
  Request's ownership model (session can only create requests scoped to
  items/sessions it owns) has no local pattern to copy structurally; it needs
  its own explicit FK + check, most naturally mirroring `BacklogStuckState`'s
  `item_id` edge with `Ref(...).Field("item_id").Unique().Required()`
  (`session/ent/schema/backlog_stuck_state.go:76-81`) plus `OnDelete(Cascade)`
  (used on `BacklogItem`, `BacklogStage`, `StageTransition`, `TransitionGate`
  schemas) so a deleted parent cascades rather than orphaning rows — do the
  same for Guidance Request's item_id/session_id FK(s) rather than leaving
  orphaned requests to be caught later by a cleanup job.
- **TOCTOU between ownership check and write.** If "check the calling session
  owns this item" is a separate SELECT before the INSERT (the natural way to
  implement an MCP-tool-level authorization check), the owning session could be
  reassigned/archived between the check and the write. Given ADR-001's
  established house style, the safer shape is: perform the ownership check as
  a best-effort pre-filter (matching `MarkStuck`'s `expectedStatus` pattern,
  §1 above) and treat a stale-ownership row (created just before the owner
  changed) as something a **self-heal sweep** cleans up — e.g. a periodic pass
  that resolves/cancels a pending guidance request whose scope was archived
  after creation — rather than trying to make check+write atomic across the
  session/backlog-item and guidance-request tables in SQLite.
- **Archived scope leaving an orphaned pending request with no answerer.** A
  backlog item or session that's archived/deleted while a guidance request is
  still pending needs an explicit terminal state (e.g. `cancelled_at`) set by
  the same cascade/cleanup path that archives the parent, not left `pending`
  forever — an unanswerable pending request is indistinguishable from a live
  one to any UI or reconciler that only checks `answered_at IS NULL`.

## Sources consulted

- `project_plans/backlog-stuck-item-visibility/decisions/ADR-001-durable-stuck-state-storage-model.md` (full read)
- `session/ent/schema/backlog_stuck_state.go`
- `session/ent_repository_backlog.go:1871-2160` (`MarkStuck`, `ResolveStuck`, `MarkStuckNotified`, `SnoozeStuckState`, `RecordRemediationAttempt`, `RecordRemediationRestartGrace`, `ResetStuckRemediation`, `BulkResetStuckRemediation`)
- `session/ent_repository.go:175` (SQLite dialect confirmation)
- `server/interceptors/feature_flag_interceptor.go`
- `server/features/flags.go`, `server/services/feature_flag_service.go`, `config/types.go` (no per-scope override precedent found)
- `server/notifications/store.go:1-80`
- `session/ent/schema/approvalrule.go` (no ownership-FK precedent found)
- `session/backlog_context.go:93`, `session/ent_repository_backlog.go:1001,1148` (`ReworkCapOverride` cap precedent)
- Repo memory: `project_backlog_wakeup_polling.md`, `feedback_document_ai_decisions_in_edge_cases.md`, `feedback_mobile_desktop_ux.md`, `feedback_rollout_flags_live_settable_no_env_vars.md`, `project_2026_07_29_oom_session_leak_fix.md`
