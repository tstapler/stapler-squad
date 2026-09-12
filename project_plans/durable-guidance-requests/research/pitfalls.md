# Pitfalls Research: Durable Guidance Requests

Grounded in the current state of the `pr-782-work` worktree
(`/home/tstapler/Programming/stapler-squad/.claude/worktrees/agent-a1b0139bdd7548d55`),
HEAD `78db27678`.

## 1. Concurrency / race pitfalls

`.claude/rules/instance-lock-free-reads.md` documents a real, CI-caught data race:
`session/instance_worktree.go`'s `GetEffectiveRootDir`/`GetWorkingDirectory`/`Workspace`
read `i.Path` directly, racing with `session/instance_actor_setters.go`'s
`setGitHubResolutionLocked` writing it under `i.mu.Lock()` from a background goroutine.
The fix pattern: every mutable `*Instance` field is written under the actor lock and
republished via `i.snapshot.Store(...)` (an `atomic.Pointer[InstanceSnapshot]`), and
`InstanceSnapshot` (`session/instance_snapshot.go:1-9`) is documented as "the single
authoritative place that knows every mutable field."

**Applicability to GuidanceRequest**: if the feature adds any field to `*Instance`
itself — e.g. `HasOutstandingGuidanceRequest bool` or a cached `PendingGuidanceRequestID
string` so the UI/session-view can render a badge without a separate query — that field
MUST be added to `InstanceSnapshot` and written only through an actor setter in
`session/instance_actor_setters.go`, exactly like the other ~40 fields already listed
there. A quick `grep -c` of `InstanceSnapshot`'s struct fields (`session/instance_snapshot.go`)
confirms it is large and actively maintained; a GuidanceRequest field added directly to
`Instance` without a matching snapshot field, then read anywhere outside
`i.mu.RLock()`, reproduces the exact bug class `TestCreateSession_GitHubURLResolution_NotBoundByRequestContext`
caught.

**Better design to avoid the whole class**: keep GuidanceRequest state entirely in
`session/storage`/ent (like `BacklogItemData`, `NotificationRecord`), and give `*Instance`
at most a *derived, read-only* accessor that queries the store by session/item ID rather
than caching request state as a mutable field on the live object. Session state
(`session/instance*.go`) already carries a lot of actor-guarded mutable surface area —
every additional field is one more thing that must be kept in the `InstanceSnapshot`
allowlist correctly, forever. If the UI needs a live "has pending guidance request"
badge without a round-trip, prefer publishing it through the existing WebSocket/notification
fan-out (see §2) rather than a new `Instance` field.

## 2. "Looks right, silently wrong" notification-bypass pitfall

`.claude/rules/norawghrequest.md` documents the precedent failure shape: a raw
`http.NewRequest`/`http.NewRequestWithContext` against GitHub compiles, passes tests
(a `200` is still a valid response), and silently costs full rate-limit quota forever
because it skips the `If-None-Match` ETag header `newGHRequest` sets up. The lesson
generalizes: a hand-rolled alternate path that produces a plausible *response* but skips
a load-bearing side effect is invisible to both manual testing and naive unit tests.

**Direct analogue for GuidanceRequest's "durably notify the originating session"
requirement**: there are two existing, independent notification mechanisms in this
codebase, and a new implementation could plausibly reinvent either one badly:

- `server/notifications/store.go`'s `NotificationHistoryStore` — a durable,
  disk-persisted (`saveToDisk`/`loadFromDisk`, `server/notifications/store.go:622-670`),
  deduplicated (`findDuplicate(sessionID, notifType)`, `:257`), retention-bounded
  (`enforceRetention`, `MaxNotifications = 500`, `MaxNotificationAge = 7*24h`,
  `:585` / `:18-20`) notification log. This is what survives a process restart —
  notifications live in a JSON file (`loadFromDisk`/`saveToDisk`), not just in memory.
  `NOTIFICATION_TYPE_INPUT_REQUIRED = 2` (`proto/session/v1/types.proto:945`) is already
  wired end-to-end as the "question" notification type (`server/notifications/store.go:36`,
  used by `server/review_queue_manager.go:954`, `server/mcp/tools_backlog.go:2862`,
  `server/services/approval_handler.go:705`). A GuidanceRequest answered by a human should
  almost certainly emit through this exact store/type rather than a parallel ad hoc
  "resolved" event, or it silently loses durability-across-restart, dedup, and retention
  for free.
- `WatchBacklogItems`'s streaming fan-out (`server/services/backlog_service_events.go`,
  `server/services/backlog_service_query.go`, `pkg/events/types.go`,
  `session/ent_repository_backlog.go`, `server/services/backlog_service.go`,
  `server/services/watch_stream_metrics.go`) — the live-update channel the backlog UI
  subscribes to. A guidance-request answer that updates storage but never publishes a
  `BacklogItemEvent` (or equivalent) through this fan-out will look correct in the
  database and in a fresh page load, but a *currently open* triage panel or backlog
  detail view will not refresh until the viewer manually reloads — exactly the "no error,
  quietly stale" failure shape called out under Evidence and Claims ("Read a mutation
  back before claiming it happened").

**Concrete risk to flag in the plan**: a `SubmitGuidanceAnswer` RPC that (a) writes the
answer to storage and (b) tries to write directly into a live `*Instance`'s in-memory
state to "notify" the originating session bypasses both durable mechanisms above. It
will pass a manual test performed while the asking session is still live and attached,
then silently fail to notify once that session has been paused, its process restarted,
or `Instance` re-hydrated from disk — which is exactly the scenario the requirement
calls out ("must survive the asking session being paused/restarted/no-longer-live").
The fix is the same shape as `norawghrequest`'s: go through the one already-durable path
(`NotificationHistoryStore.Append` + `WatchBacklogItems`/notification WebSocket), never
a new bespoke in-memory notify.

## 3. Stale / orphaned state pitfalls

Three existing reaping/cleanup precedents to model a GuidanceRequest reaper after:

1. **`testutil/tmuxreap/tmuxreap.go`** (referenced by the recent commit
   `0cb0f3315 fix(testutil): reap orphaned test tmux servers by generic prefix + process scan`):
   reaps by a *generic prefix match* (`testSocketPrefixes`, `tmuxreap.go:44-64`) plus a
   liveness check on the owning PID (`isProcessAlive`), bounded by an overall time budget
   (`reapOverallBudget = 20s`) and concurrency cap (`reapMaxConcurrent = 32`) so a large
   backlog of leaked state doesn't itself hang the caller. The doc comment explains *why*
   prefix-matching had to be broadened repeatedly (BUG-105: 882 leaked sockets over 8+
   days) — narrow, exact-match cleanup logic reliably misses new producers of the same
   kind of state.
2. **`session/backlog_lifecycle_archive.go`**'s `archiveStaleDoneItems` (age-based:
   `maxDoneAge = 3 * 24h`, `FindDoneItemsOlderThan`) and `reconcileTerminalItemSessions`
   (a periodic *safety-net* sweep over already-terminal items whose sessions never got
   cleaned up by the normal transition hook, `:80-128`) — both idempotent-by-construction
   (re-running is always safe because the query naturally excludes already-handled rows).
3. **`server/notifications/store.go`**'s `PruneOrphaned(existingSessionIDs)` (`:423-459`)
   — removes session-scoped notification records whose `SessionID` no longer exists,
   batch-fetching existing IDs once per pass and treating a `nil` map as "not ready to
   judge" rather than "nothing exists" (a real footgun this code explicitly guards
   against, `:434-437`).

**Applicability**: a GuidanceRequest can go orphaned in at least three ways — (a) its
backlog item is deleted/archived, (b) the asking session's UUID stops resolving (session
deleted, or predates a rename), (c) the request is never answered and accumulates
forever. The plan should include an analogous periodic reconciler (most naturally a new
detector in `session/backlog_lifecycle*.go`, following `archiveStaleDoneItems`'s pattern)
that: expires/archives guidance requests whose parent item reached a terminal state,
nulls or flags requests whose originating session no longer exists (mirroring
`pruneOrphanedRecords`'s `nil`-map-is-not-empty-map care), and enforces an age-based
retention cap so an abandoned request doesn't linger indefinitely. Without this, the
new entity/table follows the exact trajectory `test-io-storage-isolation.md` describes
for `triage-artifacts/` (27,578 accumulated files) and `headless-failures/` (661 files):
harmless individually, silently unbounded over the life of the repo.

## 4. Testing pitfalls

From `docs/explanation/test-io-storage-isolation.md` and the CI hermetic-testing memory
note:

- **Don't hardcode a state directory.** Any new `*DirOrDefault()`-style helper for
  guidance-request artifacts (if the design stores anything on disk rather than purely
  in the ent DB) must resolve through `config.GetConfigDir()`, not a hand-rolled
  `os.UserHomeDir()` + literal join — that exact mistake was made independently five
  times in this repo and each one leaked real files into the developer's actual
  `~/.stapler-squad/` during tests before being caught.
- **Prefer the in-memory ent repository for storage tests**: `session.NewTestEntRepository(t)`
  (`session/testing.go`) backs tests with an in-memory, uniquely-named SQLite DB — this is
  the model for any new `GuidanceRequest` ent schema's tests, not a `t.TempDir()`-backed
  file DB.
- **Isolate `STAPLER_SQUAD_TEST_DIR` explicitly** via `envtest.NewIsolatedStateDir(t)`
  rather than a manual `t.Setenv(...)` one-liner, and remember it must be called before
  `t.Parallel()` on the same `t` (or not combined with parallel tests at all).
- **`server.BuildDependencies()` makes real machine-wide calls** — notably
  `session.ReconcileOrphanedTmuxSessions(instances, 0)` at `server/dependencies.go:920`,
  which (per its own doc comment, `:906-919`) targets the *shared default tmux socket*
  with no isolation and has previously killed real production sessions when an
  integration test called `BuildDependencies()` on the same machine. Any test exercising
  guidance-request wiring end-to-end should build the narrower `ServerDependencies`/
  `RuntimeDeps` surface directly (or set `config.IsIsolatedInstance()`-detected state, e.g.
  `STAPLER_SQUAD_TEST_DIR`) rather than calling `BuildDependencies()` unguarded, per the
  CLAUDE.md memory note and this file's own `IsIsolatedInstance` gate.
- **No real sleeps/timeouts for the async notify path.** Because "answered → originating
  session notified" is inherently asynchronous (possibly across a process restart), the
  naive test-writing failure mode is a `time.Sleep` + poll or a `require.Eventually` on
  wall-clock time. Per the `deterministic-fast-tests` skill and this repo's own
  `test-io-storage-isolation.md` callout (BUG-103 item 2: a `require.Eventually` against
  a real shared directory was misdiagnosed as scheduler flakiness when it was really I/O
  contention), any test of the notify path should inject a fake clock/synchronous
  dispatch or observe a channel/callback rather than polling real time.
- **`e2e-test-conventions`**: if any E2E coverage is added for the guidance-request UI
  (backlog detail, triage panel, session view), it must start with a
  `// @feature <ids>` annotation, avoid `waitForTimeout` in favor of
  `expect(locator).toHaveValue(...)`/`waitForSelector`, and use `data-testid`/ARIA-role
  locators only — the CI-enforced rules in `tests/e2e/`.

## 5. Scope / abuse / rate-limit risk

The requirement text is explicit that "any subscribed LLM/session can create" a
guidance request, including from *automated* triage (`session/backlog_triage.go`). This
is a real spam vector: a malfunctioning or looping triage run (or a buggy automated
consumer) could create hundreds of guidance requests per item, each rendered as a
form across three UI surfaces.

Existing dedup/rate-limit precedent to mirror, in increasing sophistication:

1. **`session/nudge_dedup.go`** — the simplest applicable pattern: an exact-repeat
   suppression window (`nudgeCooldown = 3 * time.Minute`) keyed on normalized content,
   re-armed only when the underlying context (pane output) actually changes
   (`isDuplicateNudge`, `:42-60`). A guidance-request creation path could apply the same
   idea: suppress an exact-duplicate question from the same source within a cooldown
   unless something material changed.
2. **`server/services/backlog_service_pr_fix_steer.go`**'s "Epic 2.2: Cooldown-based
   dedup" (`steerDedup` keyed by `itemID`, `:82-119`) — a per-item dedup map that only
   advances on successful delivery, directly analogous to "one outstanding guidance
   request per (item, question-topic)."
3. **`server/notifications/store.go`**'s `findDuplicate(sessionID, notifType)`
   (`:257`) — collapses repeated notifications of the same type for the same session
   into one record (with special-cased ID-swap-on-collapse for `APPROVAL_NEEDED`,
   `:24-28`) rather than accumulating N cards.
4. **`github/rate_limit.go`**'s `AdmitOrigin(origin CallOrigin, resource string)`
   (`:282`, from the recent `b38c9ca46 feat(github): rate-limit-aware, cache-efficient
   GitHub call path`) — a priority-tiered admission control that reserves headroom
   (`backgroundHeadroomPercent = 10`) for interactive traffic over background/automated
   callers. The closest structural analogue for guidance requests: automated-triage-
   originated requests (`session/backlog_triage.go`) are the "background" caller class
   and should be capped/throttled more aggressively than a human-initiated ask, e.g. a
   hard cap on outstanding *unanswered* automated guidance requests per item or per
   process, mirroring `MaxConcurrentBacklogWorkItems`-style caps already in the codebase.

**Recommendation for the plan**: require (a) a per-item or per-source cap on
outstanding unanswered guidance requests (reject/collapse creation past the cap, the
way `steerDedup` and `findDuplicate` collapse rather than accumulate), and (b) treat
`backlog_triage.go`'s automated call path as a distinct, more tightly throttled
`CallOrigin`-style category from human-initiated requests, rather than trusting good
behavior from the caller.

## 6. Proto/codegen pitfalls specific to this repo's tooling

Two concrete "will silently break if skipped" hazards from CLAUDE.md, both directly
relevant since this feature "likely" needs new proto messages/RPCs and new storage:

- **`gen/` (proto output) and `session/ent/*.go`/`session/ent/*/` are gitignored but
  required-generated.** `.gitignore` deliberately excludes everything in `session/ent/`
  except `schema/` and `generate.go` (hand-written). A new `GuidanceRequest` ent schema
  file goes in `session/ent/schema/`; do not `git add -f` the generated output even to
  "unblock" a build — CLAUDE.md notes this has caused real breakage before (a
  missing/incomplete package left `main` broken until someone ran `make ent-gen` and
  noticed). Every relevant Make target (`build`, `test`, `lint`) already depends on
  `ent-gen`, which regenerates from a stamp file, so committing only the schema change
  is both necessary and sufficient.
- **The ent generate command must include `--feature sql/upsert`.** The correct
  invocation lives in `session/ent/generate.go`:
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`.
  Running the plain `entgo.io/ent/cmd/ent generate ./session/ent/schema` (without the
  flag) compiles and looks identical in a diff of the schema file itself, but silently
  breaks `UpsertRule` and similar generated methods — a classic "looks right, silently
  wrong" failure that would only surface later when a guidance-request upsert path is
  exercised. Same failure shape as `norawghrequest`: the omission is invisible at the
  call site and only shows up as a runtime/behavioral gap.
- New RPCs added to `proto/session/v1/session.proto` require `make proto-gen` before
  `gen/` reflects them; forgetting this step leaves handlers referencing stale/missing
  generated types, again invisible until build/test time rather than at proto-edit time.
- Per CLAUDE.md's "Adding New Features" checklist, a new guidance-request RPC/endpoint
  also needs a `docs/registry/features/` entry (`make registry-generate`) — this repo's
  git status already shows several backend registry JSON files pending for the current
  rollout-flags feature, confirming this step is easy to forget and is checked for
  separately from the build/lint gates.
