# Research: Pitfalls — backlog-configurable-pipeline

Grounded in this repo's actual failure history (`docs/tasks/backlog-feature-improvement.md`
buckets [1]/[2]) and direct reading of the code paths a `PipelineEngine` will touch:
`session/repository.go`, `server/services/backlog_service_lifecycle.go`,
`server/services/backlog_service_triage.go`, `session/backlog_commands.go`,
`session/backlog_review.go`, `session/workflow_engine.go`.

---

## 1. Reconciliation-bug classes a `PipelineEngine` must not reintroduce

The prior audit found 10 concrete bugs (bucket [1]) plus one more during cleanup, almost all in
the same handful of files a `PipelineEngine` will now also touch (`backlog_service_triage.go`,
`autonomous_orchestration_service.go`, `TransitionBacklogItemStatus` call sites). Each bug class
below recurs; the design must guard against it explicitly, not just avoid repeating the exact
line.

### 1a. Silent error-swallowing (bugs #2, #5, #6, #7, and the "recurring pattern" row)
Pattern: `_ = s.storage.Update...(...)` or an error checked but only logged, never surfaced to
an operator or reflected in item state. Four of the ten bugs were this exact shape.

**Guard for PipelineEngine**: any new persistence call the engine introduces (e.g. writing
"which mode ran" / "what the engine resolved to" onto the item) must follow the
`notifyTriagePersistFailure`/`notifyReworkCapHit` pattern already established — accumulate
failures and emit one consolidated operator notification — not a bare `_ =`. If
`PipelineEngine.Resolve(...)` is consulted inside `TriggerTriage`'s goroutine (line ~740) or
`WriteSlashCommands`'s callers, any error from the engine must propagate into the same
already-fixed `notifyTriagePersistFailure`-style path, not a new silent swallow.

### 1b. Notification/transition TOCTOU (bug #2)
Pattern: a push notification fired off `outcome.Done` decoupled from whether the actual DB
transition (`TransitionBacklogItemStatus`) succeeded — operator sees "complete" while the item
is actually stuck.

**Guard for PipelineEngine**: if pipeline-mode selection ever gates which notification fires
(e.g. "ran via `/sdd:full` — notify differently"), the notification must be built from the
*result* of the transition call, not fired in parallel with it. Model any new
engine-driven notification after the *fixed* version of `autonomous_orchestration_service.go`
(commit `5a809a6d`), not the pre-fix pattern.

### 1c. Swallowed lookup errors indistinguishable from "not found" (bug #3)
Pattern: `GetItemSessionBySessionUUID`/`GetBacklogItem` errors and nil results collapsed into
one silent no-op, making a real failure undiagnosable.

**Guard for PipelineEngine**: if the engine needs to look up per-item pipeline-mode state (e.g.
via `GetBacklogItem`), distinguish `session.ErrNotFound` (expected — item has no explicit mode,
fall back to default) from a real storage failure (log at Warn, do not silently fall back to
`DefaultPipelineEngine` in a way indistinguishable from the "no mode set" case).

### 1d. Unrecognized-value no-op with zero signal (bug #4)
Pattern: a `switch` on role/status silently no-op'd on an unrecognized value — a future new
value added elsewhere would silently stop advancing items.

**Guard for PipelineEngine**: this is the single most directly relevant bug class, because a
pipeline-mode field is itself a new enum-like `switch` dimension. `PipelineEngine.Resolve(mode
string)` (or equivalent) must **fail closed and loud** on an unrecognized mode string — return
an error / fall back to `DefaultPipelineEngine` *with an explicit Warn log and/or operator
notification* — never a silent no-op that leaves `WriteSlashCommands` or the review-gate runner
doing nothing. This is also the direct enforcement point for the Security NFR (§3 below): an
unrecognized mode must be rejected, not passed through.

### 1e. Orphan/standing-detector gap (bug #8)
Pattern: a guard only ran on manual re-trigger, with no periodic detector for the
stuck-after-success case, until `reconcileOrphanedTriageItems` was added.

**Guard for PipelineEngine**: N/A directly (in scope is Phase-1 "seam" only, no new
reconciliation loop) — but flag for the plan phase: if a future pipeline mode introduces a new
intermediate wait-state (e.g. "waiting on an external CI/plan-approval step for this mode"), it
needs a `reconcileX`-shaped detector from day one, not bolted on after a stuck item is found in
production, per this bug's lesson.

### 1f. Missing audit trail on direct-ent status mutation (bug #9)
Pattern: paths that mutate status via ent directly (bypassing `TransitionBacklogItemStatus`)
skip `BacklogStatusEvent` audit-row creation.

**Guard for PipelineEngine**: any status transition the engine triggers or gates must go through
`TransitionBacklogItemStatus` (or `session.BacklogItemPrecondition`-guarded equivalents) — never
a direct ent update — so the pipeline-mode's effect on the state machine is auditable in
`BacklogStatusEvent` history, consistent with the "what ran" UI surface required in scope.

### 1g. Unconditional-retry / no in-flight check before transition (bug #10, live incident)
Pattern: `ReconcilePRPending`'s 60s tick called `AutoReopenForPRFix` unconditionally on every
pr_pending item with failing CI, with no check for "is a fix already in flight" — churned status
every tick until `hasActiveWorkSession` was checked **before** any transition (fix
`f8f788ab`). This is explicitly called out in requirements.md as a class to avoid.

**Guard for PipelineEngine**: if `PipelineEngine` consultation ever happens inside a periodic
reconciliation tick (e.g. deciding "should this item's mode trigger stage N now"), it must apply
the same pattern as the fixed `AutoReopenForPRFix`: check "is this stage/action already in
flight" **before** any transition, using the same `hasActiveWorkSession`-style liveness check —
not just a dead-session check (the first, insufficient fix `af426f27` proved a liveness-only
check is not enough; the *ordering* — guard before transition — is what actually closed the
bug). Concretely: any new reconciliation logic gated by pipeline mode must read as
`if activeAlready { return early, zero churn }` before touching
`TransitionBacklogItemStatus`, mirroring `backlog_service_triage.go:549-552`.

### 1h. Destructive path with no warning (bonus finding, `stop_session`/`CleanupWorktrees`)
Not directly relevant to `PipelineEngine`'s data path, but the general lesson — undocumented
destructive side effects — applies to scope: `WriteSlashCommands` **deletes and recreates**
`.claude/commands/backlog/` files on every call (retried `os.MkdirAll`, then per-item
`writeFile`). If pipeline-mode selection is mutable and re-triggers `WriteSlashCommands` with a
different mode, confirm stale command files from the *previous* mode are actually
removed/overwritten, not left alongside new ones (currently only known files
`status.md`/`done-N.md`/`fail-N.md`/`review.md` etc. are written; a mode switch that changes
which command set applies must not leave orphaned old-mode command files an agent could still
invoke).

---

## 2. Avoiding the proto3-bool-clobbering bug for the new pipeline-mode field

### Root cause confirmed in code (not just requirements.md's description)
`server/services/backlog_service_lifecycle.go:231-236` (`UpdateBacklogItem` handler):

```go
skipRG := req.Msg.SkipReviewGate
update.SkipReviewGate = &skipRG
skipP := req.Msg.SkipPlanning
update.SkipPlanning = &skipP
autoSpawn := req.Msg.AutoSpawnSession
update.AutoSpawnSession = &autoSpawn
```

This **unconditionally** wraps whatever came off the wire into a pointer and applies it. Because
`proto/session/v1/backlog.proto`'s `UpdateBacklogItemRequest` still declares these as plain
`bool skip_review_gate = 6;` / `bool skip_planning = 7;` / `bool auto_spawn_session = 12;` (no
`optional` keyword, no wrapper type), proto3 gives these fields **no wire presence** — an
omitted field and an explicit `false` are indistinguishable on the wire. So even though the *Go*
domain layer (`session.BacklogItemUpdate`) already uses the "pointer means unset" convention
correctly (`SkipReviewGate *bool`), the *handler* defeats it by always taking the address of
whatever `req.Msg.X` deserialized to (`false` when omitted). The actual fix shipped was a
**frontend-only** mitigation (`currentFlags()` helper spread into all 4 partial-update call
sites in `BacklogItemDetail.tsx`) — it papers over the wire-format gap rather than closing it,
and depends on every future call site remembering to spread current flags. This is exactly the
kind of "fragile, easy to forget for a NEW field" risk requirements.md flags.

### Recommendation for the new field

**Use `optional string pipeline_mode = N;` in the proto (proto3 `optional`, which synthesizes a
oneof and *does* give wire presence), not a plain `bool`/`string`.** Concretely:

1. **Proto**: `optional string pipeline_mode = 25;` on `BacklogItem`, and `optional string
   pipeline_mode = 13;` (next free field number) on `UpdateBacklogItemRequest`. `optional`
   generates a `HasPipelineMode()` / `req.Msg.PipelineMode != nil` presence check in the
   generated Go code — this is the mechanism the existing plain-bool fields lack.
2. **Go handler** (`backlog_service_lifecycle.go`'s `UpdateBacklogItem`): mirror the *existing
   correct* pattern already used for `Title`/`Description`/`RepoPath` (`if req.Msg.Title != ""
   { ... }`) — but gated on **presence**, not truthiness, since `""` is a legitimate explicit
   "reset to default mode" value that must not be conflated with "field omitted":
   ```go
   if req.Msg.PipelineMode != nil {
       mode := req.Msg.PipelineMode.GetValue() // or *req.Msg.PipelineMode with proto3 optional
       update.PipelineMode = &mode
   }
   ```
   This closes the bug **at the layer where it was actually introduced** (the handler), instead
   of relying on every frontend call site to remember a `currentFlags()`-style spread — which is
   the real lesson from the `SkipReviewGate`/`SkipPlanning`/`AutoSpawnSession` incident: the fix
   that shipped treated the symptom (frontend omission) rather than the cause (backend
   unconditional-write over a presence-less wire type). Retrofit note: consider filing a
   follow-up to convert the three existing plain-bool fields to `optional bool` for the same
   reason, but that's out of this feature's scope — call it out in the plan as a related but
   separate cleanup.
3. **`session.BacklogItemUpdate`**: add `PipelineMode *string` — already matches the existing
   convention (`AutoSpawnSession *bool` etc.), no change needed to that layer's shape.
4. **Empty string semantics**: treat `""` (or an explicit sentinel like `"default"`) as "use
   `DefaultPipelineEngine`" — this must be a recognized value in the mode registry (§3), not
   merely "falsy," so an explicit reset-to-default request is distinguishable from "field not
   sent" at every layer, closing the ambiguity class that caused the original bug.

An enum (proto `enum PipelineMode`) was also considered — it gives compile-time-checked values
on the wire but proto3 enums default to `0`/first-value with **no unset variant either** unless
also wrapped in `optional`, so it doesn't avoid the presence problem by itself and adds
codegen/registry-sync overhead for what's explicitly scoped as "1-2 real alternative modes."
`optional string` validated against a server-side registry (§3) is the simpler, safer choice
here — the registry does the real validation work, not the wire type.

---

## 3. Prompt-injection risk: reusable pattern from `SanitizeDiff`

`session/backlog_review.go:234-236`:
```go
func SanitizeDiff(diff string) string {
    return strings.ReplaceAll(diff, "```", "` `` ")
}
```
Used exactly once, at `server/services/backlog_service_triage.go:980`, where a git diff is
interpolated via `fmt.Sprintf` directly into a multi-section review prompt string
(`reReviewPrompt`) that is later handed to an LLM. The technique: git diff content is
attacker-adjacent (it's arbitrary code the work session produced, including possibly adversarial
content in comments/strings), so before splicing it into a markdown-fenced prompt section, any
triple-backtick sequence that could **prematurely close the fence and inject new instructions
into the prompt** is neutralized by inserting a space (`` ``` `` → `` ` `` ``).

**This pattern does not directly generalize to the pipeline-mode field**, and that's the
important finding: `SanitizeDiff` is a *content-escaping* strategy appropriate for large,
inherently-untrusted free text that must still be included verbatim. A pipeline-mode selector is
categorically different — it's a small, closed-vocabulary control value, not prose that needs to
be preserved. Per the Security NFR in requirements.md, the correct pattern is **allow-list
validation, not escaping**:

- Validate `pipeline_mode` against a fixed, server-side registry of known mode identifiers
  (`map[string]PipelineEngine` or a `switch` in `PipelineEngine.Resolve`) **before** it is used
  anywhere — reject unknown values outright (connect `CodeInvalidArgument`), the same fail-closed
  posture recommended in §1d.
- **Never** interpolate the raw `pipeline_mode` string directly into a prompt or into
  `WriteSlashCommands`'s file-writing logic (`session/backlog_commands.go`) the way `item.ID` is
  today (`fmt.Sprintf("...item_id=%s...", itemID)` at lines 39/50/54 — safe today only because
  `item.ID` is a server-generated UUID, not user input). If pipeline mode ever needs to appear in
  generated command-file content, only registry-approved constant strings the engine emits (e.g.
  a fixed template chosen *because* mode == `"sdd-full"`) should be written — never the raw field
  value itself, even after validation, since defense-in-depth here costs nothing (the mode's
  entire value space is a handful of known strings).
- If a "reason for this mode" or similar free-text note is ever added alongside the mode
  selector, *that* field (analogous to `diff`) is the one that would need `SanitizeDiff`-style
  fence-escaping if it's ever embedded in an LLM prompt — but that's explicitly out of the
  current scope (mode selection only, no free-text stage config per requirements.md).

---

## 4. Concurrency: is read-once-at-goroutine-start sufficient for a mutable pipeline mode?

Confirmed pattern in `TriggerTriage` (`server/services/backlog_service_triage.go:626-761`):
`item` is loaded once via `s.storage.GetBacklogItem` at line 635 (top of the RPC, still on the
request goroutine), and the async work is dispatched with `itemID := item.ID`, `itemRepoPath :=
item.RepoPath` captured by value at lines 736-738 before `go func() { ... }()`. The `item`
struct itself (built from that single read) is what's passed into `session.BuildHeadlessTriagePrompt(item,
...)` / `WriteSlashCommands`-adjacent flows — there is **no re-read of the item from storage
inside the goroutine**. So whatever fields were on `item` at RPC-entry time are what the
in-flight triage call uses for its entire lifetime, including if triage takes the full 30-minute
timeout budget (`triageCtx, cancel := context.WithTimeout(s.shutdownCtx, 30*time.Minute)`).

**This pattern does provide adequate protection for a pipeline-mode field, if the design follows
it**: if `PipelineEngine` consultation happens once, at the point the item is loaded (RPC entry),
and the *resolved engine/mode* is captured by value into the goroutine closure (exactly like
`itemID`/`itemRepoPath` today), a concurrent `UpdateBacklogItem` that changes `pipeline_mode`
mid-flight cannot retroactively alter an already-dispatched triage/review call — consistent with
existing behavior for every other item field consulted at RPC-entry (title, description, AC
list, etc., none of which are re-read mid-goroutine either).

**Where this breaks down — the actual risk to design around**: the requirements state pipeline
mode should be consulted by **three separate call sites** (`WriteSlashCommands`, the triage
prompt builder, the review-gate runner), which fire at *different points in the item's
lifecycle*, not all from one goroutine's initial read. Concretely:
- `WriteSlashCommands` is called from both `backlog_service_sync.go:93` (sync-time) and
  `backlog_service_triage.go:436` (triage-time) — two *independent* read-then-write points.
- The review-gate runner (`session/backlog_lifecycle.go:533`,
  `l.runner.Run(l.shutdownCtx, item, is, l.pushAndCreatePR)`) fires later still, potentially
  hours after triage, from a different code path entirely (`BacklogLifecycleListener`, not
  `BacklogService`).

If mode is mutated between when `WriteSlashCommands` wrote mode-A command files and when the
review-gate runner later reads mode to decide its post-review stage, **each site does its own
fresh `GetBacklogItem` read** (as they do today for other fields) rather than trusting a value
threaded through from hours earlier — so the "one read at RPC/goroutine start, captured by
value" protection is real *within* a single call's lifetime but does **not** by itself guarantee
all three consultation points agree on the same mode for one item's overall pipeline run. This
is not a data race (no unsynchronized concurrent access — each read goes through
`s.storage.GetBacklogItem`, which is presumably safe), it's a **consistency** risk: an item could
plausibly run triage under mode A and review-gate under mode B if the mode is user-editable at
any time and the user (or an automated reconciliation loop) changes it mid-pipeline.

**Recommendation for the plan phase**: either (a) treat `pipeline_mode` as capturable-once at
triage time and **persist the resolved mode onto the `ItemSession` or a pipeline-run record**
(not just the mutable `BacklogItemData`) so later stages (review-gate) read "what mode this run
started under," not "what mode the item currently has" — the `AcSnapshot` field already does
exactly this for acceptance criteria (`ItemSessionData.AcSnapshot`, captured at triage-session
creation, immune to later item edits) and is the established precedent to extend; or (b)
explicitly scope pipeline-mode as immutable-after-first-triage-session in this phase (simplest,
lowest risk, matches "requirements.md: open question") and defer true mid-pipeline mode changes
to the out-of-scope ADR-013 Phase 2 work. Given scope explicitly excludes
`ConfiguredWorkflowEngine`/custom states, **(b) is the lower-risk choice for this phase** — but
the plan must state this explicitly rather than leaving it implicit, since the `AcSnapshot`
precedent shows the codebase already has the "snapshot at session-start" pattern on hand if a
future phase needs (a).

---

## 5. Rollout/blast-radius: what could make `DefaultPipelineEngine` silently NOT identical to today

The requirements state `DefaultPipelineEngine` (zero-regression) must behave exactly like the
current hardcoded path. Concrete ways this silently breaks, found by tracing every call site:

1. **Missed call site — `WriteSlashCommands` has 2 independent callers**
   (`server/services/backlog_service_sync.go:93` and
   `server/services/backlog_service_triage.go:436`). Both must be updated to consult
   `PipelineEngine`; updating only the triage-time call site (the more visible one) would leave
   the sync-time path (likely triggered by external-source item sync, per file name) running the
   old hardcoded command set for items synced from external plugins — a silent behavioral fork
   between item-creation paths.
2. **Review-gate runner lives in a different package/type than the triage/spawn logic.**
   `l.runner.Run(l.shutdownCtx, item, is, l.pushAndCreatePR)` at `session/backlog_lifecycle.go:533`
   is inside `BacklogLifecycleListener`, a different type from `BacklogService` (which owns
   `TriggerTriage`/`WriteSlashCommands` calls in `server/services/`). A `PipelineEngine` field
   added to `BacklogService` but not threaded into `BacklogLifecycleListener` (or vice versa)
   would leave one of the three required consultation points (`WriteSlashCommands`, triage
   prompt builder, review-gate runner) on the old hardcoded path while the others use the new
   seam — exactly the "1-2 real alternative modes work for triage but review-gate silently
   ignores the item's chosen mode" bug. Confirm both types get the same `PipelineEngine`
   instance (constructor-injected, per the anti-interface-pollution convention this repo already
   uses for `WorkflowEngine`) rather than two independently-constructed engines that could drift.
3. **`autonomous_driver.go` is explicitly out of scope** (per requirements.md Scope-Out) but
   `autonomous_orchestration_service.go`'s role→status switch (bucket [1] bug #4's fix location)
   is adjacent — items spawned with `Autonomous: true` (via `AutoSpawnSession`) go through
   orchestration code that does **not** currently consult any pipeline-mode-aware seam. If a
   pipeline-mode item is auto-spawned autonomously, verify the orchestration path either (a)
   also consults `PipelineEngine` for its stage decisions, or (b) is documented as a known gap
   for this phase (matching the explicit out-of-scope note) — silently defaulting autonomous
   items to `DefaultPipelineEngine` behavior regardless of their chosen mode is a real
   "not identical" risk if a user expects mode selection to affect autonomous runs too.
4. **Test coverage gap noted in the audit itself**: `docs/tasks/backlog-feature-improvement.md`'s
   "Known Coverage Gap" section flags that `SpawnSessionFromItem`/`TriggerTriage` are "the two
   highest-complexity functions in the whole subsystem" and did not get deep `code:review`
   coverage in the prior pass. Any `PipelineEngine` seam threaded through these two functions
   should get characterization tests asserting `DefaultPipelineEngine`'s output is byte-identical
   to today's hardcoded `WriteSlashCommands`/prompt output *before* any new mode is added — a
   snapshot/golden-file test would catch silent drift here more reliably than manual review,
   given these functions' existing complexity.
5. **`session/workflow_engine.go`'s `DefaultWorkflowEngine` is the template to clone** — note it
   is deep-copy-on-construct and stateless per the audit's "positive pattern to reuse" callout.
   If `PipelineEngine`'s `Default` implementation instead reads live config/DB state on each
   call (rather than being constructed once with the current hardcoded values baked in, mirroring
   `NewDefaultWorkflowEngine()`), that's a structural deviation from the proven-safe template and
   a plausible source of subtle behavioral drift under concurrent access — stick to the
   construct-once, stateless-thereafter shape already validated in this codebase.
