# Architecture Research: pr-fix-steering

Scope per requirements.md: steer an **already-active** work session with fresh
PR-fix context, instead of only logging + notifying when
`AutoReopenForPRFix` finds one. This doc does not re-derive PR-problem
detection (`GetPRStatus`/`HasConflicts`/`HasReviewFeedback`) or new-session
spawning (`AutoReopenForPRFix`'s cap/transition logic) — both are already
fully researched in the two sibling docs and cited by file:line below where
this project builds on them directly.

## 0. What the two sibling docs already establish (not re-derived here)

- `backlog-pr-conflict-detection/research/architecture.md` §1-§4: `PRStatus`
  (`CIFailing`/`HasBlockingReviews`/`HasConflicts`) and the fact that
  `FeedbackText` is a single pre-rendered string that flows unchanged through
  `ReconcilePRPending` → `fixCtx` → `AutoReopenForPRFix`'s `fixContext`
  parameter → `item.Notes` → the spawned session's prompt. This project reuses
  that exact same `fixContext` string as the raw material for the steer
  message — no new signal-carrying plumbing needed on the detection side.
- `pr-review-followup/research/architecture.md` §0: confirms the current tree
  has moved past both docs' original line citations — `ReconcilePRPending` is
  now `session/backlog_lifecycle.go:3850-4113`, `PRStatus` now also carries
  `HasReviewFeedback`/`LatestFeedbackAt`, and `remediatePRFixWithBackoffGate`
  (`backlog_lifecycle.go:3777-3845`) wraps every `AutoReopenForPRFix` dispatch
  from `ReconcilePRPending` in a durable, per-`(item, "pr_needs_fix")`
  exponential backoff gate (`Storage.RemediationDue`,
  `session/backlog_remediation.go:168-193`).
- `pr-review-followup/research/architecture.md` §1c: establishes the
  content-based-dedup-needs-a-watermark-not-an-ID-set pattern for a
  structurally similar problem (repeat GitHub feedback that doesn't
  self-clear). §1 below explains why this project's dedup problem is a
  materially *smaller* version of that one and doesn't need the DB-column
  answer §1c reached — see the durability analysis.
- `pr-review-followup/research/architecture.md` §5: both sibling docs
  concluded no Event-Command-Policy/EventStorming table is warranted for this
  family of change ("a straightforward polling-loop extension, not a
  multi-actor domain"). §5 below reaches the same conclusion for this
  project, for a stronger reason.

**Important scoping note**: this project's integration point
(`backlog_service_triage.go:2048-2051`, confirmed live below) is reached by
`AutoReopenForPRFix` regardless of whether `ReconcilePRPending` is calling it
directly or via `remediatePRFixWithBackoffGate`. The backoff gate governs
*whether AutoReopenForPRFix gets called this tick at all* (time-based); this
project's dedup governs *what happens once it's called and finds an active
session* (content-based). The two are complementary layers, not competing
mechanisms — a call that's due per the backoff schedule can still be a no-op
for steering purposes if the reason signature hasn't changed since the last
steer.

## 1. The exact integration point

Confirmed live at `server/services/backlog_service_triage.go:2015-2064`
(`AutoReopenForPRFix`):

```go
2047      s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)
2048      if active := findActiveWorkSession(sessions); active != nil {
2049          s.notifyRespawnBlockedByActiveSession(ctx, "AutoReopenForPRFix", itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID)
2050          return nil
2051      }
2052      s.resolveRespawnBlockedActiveLogged(ctx, "AutoReopenForPRFix", itemID)
```

`findActiveWorkSession` (`backlog_service_triage.go:1253`) takes
`[]session.ItemSessionSummary` — a storage-level view with `SessionUUID`,
`Role`, `EndedAt`, etc. **It does not carry `Program`** —
`ItemSessionData`/`ItemSessionSummary` (`session/storage_backlog.go:194-212`)
has no such field. `Program` lives only on the live, in-memory
`*session.Instance` (`session/instance.go:161`), which `BacklogService` has
no direct reference to today — it only ever touches sessions via
`ItemSessionSummary` and via the `SessionStopper` interface's UUID-keyed
methods. This is the reason the new interface (§3) must expose a
Program-query method, not just a "send this text" method: the gating
decision requires data `BacklogService` cannot see any other way.

### New call to insert

Replace the unconditional skip-and-notify with:

```go
if active := findActiveWorkSession(sessions); active != nil {
    s.steerActiveSessionForPRFix(ctx, itemID, item.Title, session.BacklogStatus(item.Status), active.SessionUUID, fixContext)
    return nil
}
```

where `steerActiveSessionForPRFix` (new, same file, mirrors the existing
`notify*`/`resolve*` helper-per-concern shape already used throughout this
file) does, in order:

1. **Dedup check** (§2): compute the reason signature from `fixContext` (or,
   better, from the same structured booleans `ReconcilePRPending` already has
   — see the note in §2 on why hashing `fixContext` text directly is the
   fallback, not the first choice) and compare against the last-steered
   signature recorded for `itemID`. If unchanged and within cooldown, call
   the **existing** `notifyRespawnBlockedByActiveSession` unchanged (see §1a
   below for why it stays) and return — this is the "no spam" success
   metric.
2. **Nil-safety check**: if `s.sessionSteerer == nil` (not wired — same
   nil-safe degrade pattern every other optional dependency on
   `BacklogService` uses, e.g. `sessionStopper`/`repoWatchRemover`), fall back
   to `notifyRespawnBlockedByActiveSession` unchanged — steering an
   `SessionSteerer`-less deployment must degrade to today's behavior exactly,
   not silently no-op with zero signal.
3. **Program-gated message construction**: `program, ok :=
   s.sessionSteerer.SessionProgram(active.SessionUUID)`. If `!ok` (session
   not tracked live — it may have exited between `findActiveWorkSession`'s
   storage read and now), treat like case 2: fall back to
   `notifyRespawnBlockedByActiveSession`. Otherwise build the steer message:
   `fixContext` plus, only `if program == "claude"` (the literal Program
   string Claude Code sessions use — confirmed the codebase compares this
   exact literal everywhere, e.g. `session/instance_program_test.go:19,76`,
   with no existing `IsClaudeCode`-style helper to reuse), an appended
   `\n\nRun /github:pr-ship to address this.` instruction. Any other
   `program` value gets the plain-English `fixContext` only, per the
   requirements.md constraint.
4. **Steer + record**: call `s.sessionSteerer.SteerActiveSession(ctx,
   active.SessionUUID, message)`. On success: update the dedup state (§2) and
   publish a **new** notification (§4) distinct from
   `notifyRespawnBlockedByActiveSession`, since the session is no longer
   "blocked" — it was just told what to do. On error: keep the old dedup
   state unchanged (so the next tick retries rather than silently accepting a
   failed send as "delivered") and publish a failure-flavored notification
   (§4) — satisfies the "every steer attempt, success or failure, is visible"
   requirement.

### What happens to `notifyRespawnBlockedByActiveSession`

**It stays, unrenamed, and keeps its exact current meaning** — "an automated
action was skipped because a session is already active and nothing was
communicated to it." It now fires in exactly three narrower cases instead of
unconditionally:

- dedup suppressed a repeat of the same reason (case 1 above) — this is
  still, accurately, "respawn/action blocked by an active session," just for
  the *steer*, not a respawn;
- `SessionSteerer` isn't wired (case 2);
- the session isn't live per `SessionProgram`'s `ok` (case 3).

This is the smallest correct change to that helper's contract: its callers
(`AutoRespawnAutonomousWork`, `AutoRespawnReview` — both explicitly out of
scope per requirements.md) are untouched and keep seeing it fire exactly as
today, since neither gets a `SessionSteerer` call added. Only
`AutoReopenForPRFix`'s call site changes what precedes the (now conditional)
call to it. `resolveRespawnBlockedActiveLogged` (line 2052,
`backlog_service_triage.go:1405`) is unaffected — it already runs
unconditionally once the active-session branch is skipped entirely (i.e. no
active session found at all), which this project doesn't touch.

A **new**, separate notify helper is added alongside it (§4) for the
successful/attempted-steer case — not a rename of the existing one, because
conflating "we skipped you" and "we told you what's wrong" under one
notification title would make the item's activity trail harder to read, not
easier, which cuts against the Observability requirement's intent.

## 2. Where the reason-signature dedup state should live

### Recommendation: in-memory, per-item map on `BacklogService` — option (a)

Mirrors `spawnInFlight sync.Map` (`server/services/backlog_service.go:139-165`)
and `triageInFlight sync.Map` (`:182-187`), both already documented on this
exact struct as intentional in-memory-only coordination state, and the same
shape as `session/nudge_dedup.go`'s `lastNudge{text, at, pane}` /
`isDuplicateNudge`/`nextLastNudge` pure-function trio the requirements.md
Scope section explicitly calls out as the testing pattern to follow.

```go
// lastSteerReason records the most recent PR-fix reason signature steered
// into an item's active session, keyed by item ID. In-memory only — see
// doc comment for why DB durability isn't worth it here.
type lastSteerReason struct {
    signature string
    at        time.Time
}
// on BacklogService:
steerDedup sync.Map // itemID (string) -> lastSteerReason
```

Pure helper functions (unit-testable without a live session, matching
`nudge_dedup.go`'s pattern exactly):

```go
func isDuplicateSteerReason(candidate string, last lastSteerReason, now time.Time) bool
func nextLastSteerReason(prev lastSteerReason, candidate string, delivered bool) lastSteerReason
```

**Signature construction**: hash/concatenate the three (four, counting
`HasReviewFeedback` once `pr-review-followup` ships) boolean signals plus a
coarse detail string, e.g.
`fmt.Sprintf("ci=%v|reviews=%v|conflicts=%v|detail=%s", ci, reviews,
conflicts, coarseDetail)` — **not** a raw hash of the full `fixContext`
string. `fixContext`/`FeedbackText` can include volatile substrings (a CI log
timestamp, a differently-worded but semantically identical failed-check
name) that would change on every tick without the underlying *reason*
changing, defeating the dedup. This requires `AutoReopenForPRFix` to receive
the structured signal, not just the pre-rendered string — see the Open
Question this raises in §6.

### Why not (b): a new column on `session.Storage`/`backlog_item`

`pr-review-followup`'s watermark (`pr_feedback_addressed_at`, a DB column) is
the right call **for that project** because losing it on restart has a real
cost: a re-dispatched *new fix session* for feedback GitHub will never clear
on its own, burning one of the 5 `RemediationDue` attempts and confusing an
operator watching `/unfinished`. This project's dedup failure mode is
strictly cheaper: losing the in-memory map on restart causes, at worst, **one
redundant steer message typed into an already-active pane** — the exact
"blast radius" the requirements.md Risk Control section already accepts as
the ceiling for a bug in this path ("at worst, send one redundant steer
message per reason change per item — not an unbounded spam loop"). A process
restart is already a low-frequency event, and the pane itself (tmux-backed,
independent of the Go process — confirmed by `SessionStopper`'s own
`IsSessionLive`/`FindLiveInstance` design treating "live in poller" and "tmux
session exists" as related but distinguishable facts) is what actually
receives the duplicate; the agent inside it can trivially recognize "I was
already told this" from its own transcript. Per the task's own framing: a
restart already discards the running session's *own* memory of being
steered in the sense that matters here (whether the operator sees a spam
burst), so matching durability with an in-memory map is consistent, not a
gap. Adding a schema column, migration, and `BacklogItemData`/
`BacklogItemUpdate` threading (the concrete cost §1c of the sibling doc
spells out in full) buys durability the failure mode doesn't need.

### Why not (c): reuse `BacklogStuckState`'s content field

Same rejection reasoning `pr-review-followup`'s architecture.md gives for
its own near-identical option: `StuckReasonRespawnBlockedActive`'s stored
content (`backlog_service_triage.go:1368-1369`,
`fmt.Sprintf("%s skipped auto-respawn — session %s already active (%s)", ...)`)
is a human-readable "why," documented as such, shared across all
`notifyRespawnBlockedByActiveSession` callers, and re-published on every call
regardless of a `MarkStuck` "already open" state (its own doc comment,
`backlog_service_triage.go:1358-1362`, says it deliberately does *not*
dedup). Repurposing it as a machine-parsed dedup key would require parsing
structured state back out of prose, and — worse — the row wouldn't even
exist in the new common case (steer succeeded, no "blocked" condition to
record), so there'd be no row to compare against on the very next tick,
inverting the dedup instead of implementing it.

## 3. `SessionSteerer`: new interface, not an addition to `SessionStopper`

**New interface**, following the `SessionStopper` precedent exactly
(`server/services/backlog_service.go:45-78`, wired via `SetSessionStopper`,
`server/dependencies.go:1191`), for the interface-segregation reason the task
brief already names: `SessionStopper`'s whole contract is about *ending*
sessions (`StopSessionByUUID`, `KillTmuxSessionByTitle`, `KillTmuxPaneOnly`,
`ArchiveSessionByUUID`) plus read-only liveness/staleness probes it needs for
its own kill decisions (`IsSessionLive`, `TimeSinceLastMeaningfulOutput`).
Injecting a message into a running session is a disjoint capability with a
disjoint failure mode (a bad send doesn't need `IsSessionLive`'s "already
gone" semantics — it needs "is this the right kind of session to receive a
slash command"). Bolting `SteerActiveSession` onto `SessionStopper` would
force every future consumer that only needs to kill sessions to also
implement steering, and vice versa — exactly what ISP exists to prevent, and
exactly the shape the file's own existing doc comments (e.g.
`RepoWatchRemover`'s single-method interface at
`backlog_service.go:80-91`) already demonstrate this codebase does per
distinct capability rather than growing one god-interface.

```go
// SessionSteerer allows BacklogService to inject a message into a live,
// already-active session and to query enough about it to decide what that
// message should contain. Used by AutoReopenForPRFix to redirect a session
// already working on an item when its PR develops a new problem, instead of
// only skipping + notifying. Nil-safe: BacklogService degrades to the
// pre-existing skip+notify behavior when not wired.
type SessionSteerer interface {
    // SessionProgram returns the Program of the live instance backing
    // sessionUUID (e.g. "claude", "aider ..."), and false if the session
    // isn't currently tracked live. Callers use this to decide whether a
    // Claude-Code-specific slash-command instruction belongs in the message
    // — a literal "/github:pr-ship" typed into a non-Claude-Code session's
    // PTY would just be garbage input.
    SessionProgram(sessionUUID string) (program string, ok bool)
    // SteerActiveSession injects message into the live session identified by
    // sessionUUID, reusing the exact autonomous-vs-interactive branching
    // UpdateSession's SteerMessage handling already implements
    // (session_service.go:2929-2972): ClaudeController.SendCommandImmediate
    // for autonomous sessions, Instance.SendKeys (bounded by a timeout) for
    // interactive ones. Returns an error if the session isn't live, isn't
    // reachable, or the send failed/timed out.
    SteerActiveSession(ctx context.Context, sessionUUID, message string) error
}
```

Implemented by `*SessionService`, wired the same way as `SessionStopper`:

```go
// server/dependencies.go, alongside line 1191:
backlogSvc.SetSessionSteerer(sessionService)
```

**Reuse, not reimplementation, of the branch logic**: the constraint ("must
reuse ... rather than reimplement SendKeys/ClaudeController handling a second
time") is satisfied by extracting `UpdateSession`'s existing inline block
(`session_service.go:2934-2971`) into a private method, e.g.
`(s *SessionService) steerInstance(ctx context.Context, instance
*session.Instance, message string) error`, that both `UpdateSession` (RPC
handler, keeps its own error-to-`connect.Error` mapping and
`session.MaxSteerMessageLength` length check at the call site since those are
RPC-request concerns, not steering concerns) and the new
`SteerActiveSession` (looks up the instance via the same
`s.FindLiveInstance(sessionUUID)` every other `SessionStopper` method already
uses — `session_service.go:891,933,946,970` — then calls `steerInstance`)
delegate to. This is a pure Extract Method refactor of existing, already-
tested logic (`TestUpdateSession_SteerMessage_*` per requirements.md's
Feasibility Risks) — no behavior change to the RPC path, and the new
`SteerActiveSession`'s own success/failure notification (`notifySteerSent`,
`session_service.go:3035-3045`) fires automatically as a session-scoped
"Steering input sent" event, in addition to (not instead of) the new
item-scoped notification §4 adds — the two are differently scoped (session
detail view vs. backlog item activity trail) and both already coexist for
other flows (e.g. `notifyReworkCapHit` is item-scoped while
`ArchiveSessionByUUID`'s callers get no notification at all — scoping is
already inconsistent per-helper in this codebase, so adding one more
item-scoped notification here is additive, not a new pattern to justify).

### Feasibility check: steering a busy/mid-turn session

Confirmed by inspection, not just assumed: `UpdateSession`'s existing
`SteerMessage` branch is reachable today from `steer_session` (MCP) and the
UI's steer box on any session, autonomous or not, with **no precondition on
the session being idle** — `SendCommandImmediate` and `SendKeys` both queue
into the PTY/tmux input stream unconditionally. This project's new call path
adds no new risk here: it becomes a second, automated *caller* of the exact
same unconditional-send code, matching the requirements.md Rabbit Holes
section's own expectation ("likely already a solved problem"). Confirming
this in research (rather than assuming) rules out one hypothesis: this
project does not need to add any own idle-detection/turn-boundary check
before calling `SteerActiveSession` — that would be new scope beyond what
the RPC path already does for human-triggered steers.

## 4. New notification for a delivered/attempted steer

Add `notifyActiveSessionSteered(ctx, itemID, itemTitle, activeSessionUUID
string, message string, deliverErr error)` next to
`notifyRespawnBlockedByActiveSession`, mirroring its `eventBus.Publish`
shape but **not** routing through `MarkStuck`/`StuckReason` — a successful
steer is not a stuck condition (nothing needs "resolving" the way
`rework_cap`/`bouncing`/`respawn_blocked_active` do), it is a completed
remediation action, closer in kind to a log-worthy event than a durable
stuck-state row. On `deliverErr == nil`: `NOTIFICATION_TYPE_INFO`, title
"Active session steered with PR fix context". On `deliverErr != nil`:
`NOTIFICATION_TYPE_WARNING`, title "Failed to steer active session", body
includes `deliverErr`. Both keyed with `map[string]string{"item_id":
itemID}` and `itemID` as the notification's `sessionID` slot, matching the
established coalescing-key comment at `backlog_service_triage.go:184-186`.
This satisfies the Observability requirement ("every steer attempt, success
or failure, is visible ... not just a log line") without inventing a new
`StuckReason` or new persistence — `AllStuckReasons`
(`session/domain/backlog.go:202-221`) and its parallel `IsValid` switch
(`:224-234`) are both left untouched, avoiding the two-list-must-stay-in-sync
maintenance burden a new reason constant would add for a case that isn't
actually a "stuck" state.

## 5. Event-Command-Policy / EventStorming table: not warranted

Skip it, for a stronger version of the reason both sibling docs already gave
("straightforward polling-loop extension, not a multi-actor domain"). This
project doesn't even add a new *branch* to the detect→gate→spawn flow those
docs describe — it changes what happens on one existing branch
(`active != nil`) that already unconditionally returns `nil` today. The
"actors" the task brief names (reconciler, GitHub API state, live agent
session) are not new to this project: the reconciler and GitHub API state
already exist and are already fully modeled by the sibling docs' work; the
"live agent session" as a receiver of information is a new *data sink*, not
a new actor with its own decision logic — it doesn't branch, retry, or
negotiate, it just receives a string. A table would produce a single row
("PRStillBrokenWhileSessionActive" event → "SteerActiveSession" command →
policy: "if reason signature changed and SessionSteerer wired and session
live") that restates the `if` chain in §1 with no added clarity, the same
verdict the sibling docs already reached for one-line gate extensions.

## 6. Open question raised by this research (for planning phase)

`AutoReopenForPRFix`'s signature is `(ctx, itemID, fixContext string) error`
— it receives only the pre-rendered string, not the structured
`CIFailing`/`HasBlockingReviews`/`HasConflicts`/`HasReviewFeedback` booleans
`ReconcilePRPending` has before rendering `FeedbackText`. §2's signature
recommendation needs those booleans, not the rendered text, to avoid
false-positive "reason changed" triggers from volatile text. Two options for
planning to choose between:
1. Widen `AutoReopenForPRFix`'s signature (and the `session.PRFixSpawner`
   interface it implements, `session/backlog_lifecycle.go:36-38` per the
   sibling doc) to also take a small structured reason value alongside
   `fixContext` — a real signature change to an existing, tested interface.
2. Keep `AutoReopenForPRFix`'s signature untouched and compute a coarser
   signature by parsing `fixContext`'s `## <Section>` headers (the sibling
   docs both confirm `render()` produces stable, named section headers per
   signal — `## Merge conflict`, `## Review: changes requested`, `## Reviewer
   comments` — even though the body text under each is volatile) rather than
   hashing the full string. This avoids the interface change at the cost of
   a small amount of string-parsing coupling to `render()`'s section-header
   naming.
Recommend option 2 for the smaller blast radius (no interface signature
change, no ripple into `session.PRFixSpawner`'s other implementers/callers),
but flag this explicitly for planning rather than deciding unilaterally here
since it's a real design trade-off, not a research-answerable fact.
