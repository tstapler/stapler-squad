# Stack Research: session-worktree-reconciliation

Research question: what existing Go libraries/packages/patterns in this repo apply
directly, so a new reconciliation sweep reuses plumbing instead of reinventing it.

## 1. Git worktree validation — `session/git/` — house style is go-git + native admin-dir parsing, NOT shelling out

The `prefer-go-git-over-subshells` convention is already fully in force in `session/git/`.
Two complementary mechanisms exist, both directly reusable:

### a) Opening a repo / resolving a commit SHA — go-git (`github.com/go-git/go-git/v5`)

- `git.OpenRepo(path string) (*git.Repository, error)` — `session/git/util.go:43`.
  **The only sanctioned way to open a repo in this codebase** — enforced by the
  `tools/lint/norawgitopen` analyzer (comment at `util.go:39-44`). It wraps
  `git.PlainOpenWithOptions` with `defaultPlainOpenOptions` (`util.go:34-37`), which
  sets `EnableDotGitCommonDir: true` — required for linked worktrees, since without it
  go-git silently resolves objects/refs against the wrong gitdir (verified empirically,
  per that comment). **A reconciler must use `git.OpenRepo`, never a raw `git.PlainOpen`.**
- `git.IsGitRepo(path string) bool` — `util.go:202-215` — walks up from `path` calling
  `OpenRepo` until it succeeds or hits the filesystem root. Directly answers "is this a
  valid git worktree/repo directory."
- `git.CommitInfo(repoPath, sha string) (ShippedCommit, error)` — `ops.go:480-496` —
  opens the repo via `OpenRepo`, then `repo.CommitObject(plumbing.NewHash(sha))`; returns
  an error if the SHA doesn't resolve. **This is the direct answer to "does
  `base_commit_sha` still resolve in the repo"** — no shell-out needed.
- `git.IsUnbornRepo(repoPath string) bool` — `ops.go:266+` — checks for a repo with zero
  commits (distinguishes "no commits yet" from "can't resolve a name").
- `git.ResolveWorktreeBaseCommit` / `git.ResolveExplicitBranchSHA` (`ops.go:141`, `:247`) —
  show the established fallback-chain idiom (origin fetch → local branch → unborn-repo
  check) that a reconciler's own base_commit_sha repair logic should mirror rather than
  invent a new resolution order.

### b) Listing/validating worktrees registered against a repo — native pure-Go admin-dir parsing (Epic 2.3), not go-git and not shelling out to `git worktree list`

- `nativeListWorktrees(repoPath string) ([]NativeWorktreeEntry, error)` —
  `session/git/native_worktree_list.go:40-63` — reads `<repoPath>/.git/worktrees/`
  directly (no go-git, no subprocess) and returns one `NativeWorktreeEntry` per
  registered worktree, each with `WorktreePath`, `BranchRef`, `Locked`, and a computed
  `Prunable` flag (`WorktreePath` missing on disk && not locked — `native_worktree_list.go:14-32`).
  This is explicitly documented as "the pure-Go replacement for parsing `git worktree
  list --porcelain` output" (`native_worktree_list.go:12-13,34-35`) — confirming the
  house style rejected shelling out here in favor of native parsing, one level stronger
  than even go-git (go-git v5 has no worktree-admin-dir API at all).
  **This is the function to call to answer "is `repo_path` a real git repo with a
  worktree still registered whose `WorktreePath` matches the DB's `worktree_path`."**
- Supporting primitives: `fileExistsOnDisk`/`dirExistsOnDisk`
  (`native_worktree_list.go:140-166`) distinguish "doesn't exist" from a genuine stat
  error (EACCES) — the same distinction a reconciler needs so a permission error isn't
  silently treated as "worktree gone, safe to repair."

### What NOT to reuse: `GitWorktreeManager.HasWorktree()`

`session/git_worktree_manager.go:113`'s `HasWorktree()` (used pervasively in
`session/instance*.go`) is an **in-memory** "does this live `Instance` have a
`gitManager` configured" check — unrelated to whether the DB's `worktrees` row or the
on-disk directory is actually consistent. Do not confuse the two when reading existing
call sites.

## 2. Ent ORM — anti-join pattern already established, just not yet applied to Session/Worktree

Generated ent code (`session/ent/*.go` except `schema/`) is gitignored per this repo's
policy — not present in a fresh worktree checkout; inspect via `session/ent/schema/`
(hand-written) and existing call sites in `session/ent_repository.go` /
`session/storage_backlog.go` instead of trying to read generated files directly.

- **Schema** (`session/ent/schema/session.go:180-181`, `session/ent/schema/worktree.go`):
  `Session` → `Worktree` edge is `edge.To("worktree", Worktree.Type).Unique()` — **not
  `.Required()`** (some sessions legitimately have no worktree, confirmed both by
  requirements.md and the schema itself). `Worktree`'s back-edge to `Session` **is**
  `.Required()` (`worktree.go`'s `Edges()`), so every `worktrees` row must belong to some
  session — but nothing enforces the reverse.
- **The exact anti-join idiom already exists** for a structurally identical problem —
  `EntRepository.FindReviewItemsWithoutGate` (`session/storage_backlog.go:1002-1014`):
  ```go
  items, err := r.client.BacklogItem.Query().
      Where(
          backlogitem.Status(string(BacklogStatusReview)),
          backlogitem.SkipReviewGate(false),
          backlogitem.Not(backlogitem.HasItemSessionsWith(itemsession.SessionRole(SessionRoleReview))),
      ).
      WithItemSessions(func(q *ent.ItemSessionQuery) { ... }).
      All(ctx)
  ```
  A new "sessions with no worktree" query is the direct analog:
  `session.Not(session.HasWorktree())` (the generated `HasWorktree` edge predicate
  ent produces automatically for the `worktree` edge) combined with whatever session
  status filter scopes the sweep (e.g. exclude already-terminal/archived sessions).
  `session/storage_backlog.go:920,1042,1102` show three more `backlogitem.Not(...)`
  uses — this is a well-worn idiom in the codebase, not a one-off.
- **Eager-loading the worktree edge to inspect its fields** (for the "worktree row
  exists but its paths don't resolve" case) already has a named, reusable option:
  `LoadOptions.LoadWorktree` → `applyLoadOptions` calls `q.WithWorktree()`
  (`session/ent_repository.go:917-926`); `EntRepository.List`/`ListByStatus` already use
  this via `listLoadOptions` (`ent_repository.go:909-914`). `SessionRetentionSweeper`
  already calls `storage.ListInstanceDataWithWorktree()`
  (`server/services/session_retention_sweeper.go:81`) specifically because the default
  `LoadMinimal` path does **not** populate `Worktree.WorktreePath` — the same trap a new
  reconciler must avoid (its own comment at lines 77-80 explains a prior regression from
  exactly this).
- Point lookup precedent: `tx.Worktree.Query().Where(worktree.HasSessionWith(session.ID(sess.ID))).Only(ctx)`
  with `ent.IsNotFound(err)` handling — `session/ent_repository.go:637-639` — is the
  per-session "does a worktree row exist for this session" check, if a per-session (vs.
  bulk anti-join) query style is preferred for the repair path.

## 3. Background sweeper wiring — one consistent, well-documented integration point

Every sweeper in this codebase follows the identical shape; `SessionRetentionSweeper`
(`server/services/session_retention_sweeper.go`, 233 lines, read in full) is the cleanest
template:

1. **Type + constructor**: holds only the dependencies it needs (`storage *session.Storage`,
   `cfg *config.Config`, plus any service needed to act — e.g. `svc *SessionService`);
   constructor takes them as plain args (`NewSessionRetentionSweeper`, line 40).
2. **`Start(ctx context.Context)`**: `time.NewTicker(interval)` + `defer ticker.Stop()`,
   runs one sweep immediately before entering the loop, `select { case <-ctx.Done(): return;
   case <-ticker.C: s.sweep(ctx) }` (lines 45-65). Interval is a private package
   constant with a comment justifying its magnitude relative to sibling sweepers
   (`sessionRetentionSweepInterval = 1 * time.Hour`, lines 16-20).
3. **`sweep(ctx)` gates on a config-enabled check first** (`if
   !s.cfg.SessionRetention.EnabledOrDefault() { return }`, line 71) — every sweep tick
   re-checks the flag, so it can be disabled live without a restart.
4. **Construction/start site**: `server/server.go:1150-1213`, inside the same block as
   every sibling sweeper (hibernation, orphaned-tmux, retention, stale-session-notifier,
   stale-creation, superseded-session) — all gated by a config predicate, all started as
   `go xSweeper.Start(serverCtx)` right after `deps := BuildRuntimeDeps(...)`. E.g.:
   ```go
   if cfg.SessionRetention.EnabledOrDefault() {
       retentionSweeper := services.NewSessionRetentionSweeper(deps.Storage, cfg, deps.SessionService)
       go retentionSweeper.Start(serverCtx)
       log.Info("Session retention sweeper started", ...)
   }
   ```
   A new worktree-reconciliation sweeper's construction/start call belongs in this exact
   block, following the same `if <enabled-check> { construct; go .Start(serverCtx);
   log.Info(...) }` shape.
5. One-shot (non-ticker) startup reconciliation is a separate, simpler pattern used when
   a sweep only needs to run once at boot — `session.ReconcileSuspendedProcesses` called
   directly (not via goroutine) at `server/server.go:667`, and
   `session.ReconcileOrphanedTmuxSessions(instances, 0)` at
   `server/dependencies.go:984` inside `BuildRuntimeDeps` (its own periodic counterpart,
   `OrphanedTmuxSweeper`, is started separately at `server/server.go:1170-1172` with a
   mandatory nonzero `minAge` grace period to avoid a race with in-flight session
   creation — see `session/orphan_sweep.go:36-45`'s doc comment for why a periodic
   caller must never pass 0). A worktree reconciler is inherently periodic (drift can
   appear well after startup), so it should follow the ticker pattern, not the one-shot
   pattern — but the minAge-style "don't flag something mid-creation" grace period is
   worth carrying over given `CreateSession` writes the `sessions` row and the
   `worktrees` row in separate steps.

## 4. Notification / flagging pipe — durable stuck-marker + event-bus notification, two halves

The existing "flag for operator attention" pattern (`BacklogService.notifyIfActiveWorkSessionStale`,
`server/services/backlog_service_triage.go:1560-1601`, and its siblings
`notifyRepeatedFailure` at line 207, `notifyReworkCapHit` at line 169) has two
independent halves — reuse both:

1. **Durable, deduplicated marker** — `s.storage.MarkStuck(ctx, itemID,
   domain.StuckReasonX, currentStatus, humanReadableDetail) (applied bool, err error)`.
   `applied == false` means a status precondition no longer holds (item moved on between
   read and write) — treated as a no-op, not an error (see line 1588-1590). Optionally
   followed by `s.storage.MarkStuckNotified(ctx, itemID, reason)` to dedupe repeated
   notification firing across sweep ticks (`backlog_service_triage.go:214-216`).
   `StuckReason` is a validated string-enum in `session/domain/backlog.go:43+` — a new
   reason (e.g. `StuckReasonBrokenWorktree`) should be added there following the existing
   `StuckReasonStaleWork`/`StuckReasonPRPendingNoPR` naming and doc-comment style.
   **Caveat**: `MarkStuck` is backlog-**item**-scoped (keyed on `itemID` +
   `currentStatus` precondition). A session with broken worktree tracking that has no
   live backlog item association (e.g. a manually-created session, or a shell sibling)
   has no natural `itemID` to hang a `MarkStuck` row on — the plan phase needs to decide
   whether such sessions get a log-only flag, a different durable record, or are simply
   out of scope for the "MarkStuck" half and only get the event-bus notification below.
2. **Operator-facing notification** — `s.eventBus.Publish(events.NewNotificationEvent(
   sessionOrItemID, "", uuid.New().String(), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
   derivePriority(urgent, important), title, body, map[string]string{"item_id": itemID}))`
   (`backlog_service_triage.go:1593-1600`). This publishes onto the same event bus that
   `server/notifications/subscriber.go` consumes into the persisted
   `NotificationHistoryStore` (`server/notifications/store.go`) — i.e. publishing via
   `eventBus.Publish` is the one true entry point; there is no separate "write directly
   to the notification store" path to imitate. `NotificationHistoryStore` itself
   (770 lines) exposes `List`, `MarkRead`, `PruneOrphaned`, `GetUnreadCount` etc. for the
   UI side — a reconciler never touches it directly.

## 5. Config knobs — two live-settable patterns, pick per knob purpose

Per the "rollout flags: live-settable, no env vars" convention (MEMORY.md), this repo
already has **two** distinct live-settable mechanisms — a new sweep's config needs to
pick the right one per knob, and the existing precedent for sweep-specific knobs
(interval/retention-window-style numbers) is notably *not* wired to a live RPC today:

- **Dedicated per-feature config struct** (`config/types.go:141-169`,
  `SessionRetentionConfig`): `Enabled *bool` (pointer so unset ≠ explicit false) +
  `RetentionDays int`, with `EnabledOrDefault()`/`RetentionDaysOrDefault()` accessor
  methods. Registered on the root `Config` struct at `config/config.go:346-347`
  (`SessionRetention SessionRetentionConfig \`json:"session_retention,omitempty"\``).
  **Gap found**: grepping `proto/session/v1/*.proto` and `web-app/src` turns up **no**
  `GetSessionRetentionConfig`/`UpdateSessionRetentionConfig` RPC and no UI reference —
  unlike `UnfinishedWorkConfig` or `SlackConfig`, which do have dedicated
  `Get*Config`/`Update*Config` RPCs (`proto/session/v1/unfinished.proto:41,44`;
  `proto/session/v1/session.proto:316,322`). So `SessionRetentionConfig` today is only
  settable by hand-editing `config.json` on disk, not truly "live-settable" per the
  convention — worth flagging in planning rather than copying uncritically as "the"
  reference implementation for a sweep-interval knob.
- **Generic named feature-flag map** (`config.FeatureFlags map[string]bool`,
  `config/config.go:331-334`), with `GetFeatureFlagWithDefault(name string, defaultValue
  bool) bool` / `SetFeatureFlag(name string, value bool) error`
  (`config/config.go:1606-1644`) and a real RPC: `rpc UpdateFeatureFlag(...)`
  (`proto/session/v1/session.proto:481`, backed by
  `server/services/feature_flag_service.go`). This **is** genuinely live-settable
  end-to-end today (existing flags: `stream_hub`, `tymux`, `triage_guidance_halt` —
  `config/config.go:454,461,469`). **This is the pattern to use for the sweep's
  on/off toggle** if it's to be truly live-settable without a code change, matching the
  MEMORY.md convention exactly ("global+per-scope override via the feature-flag
  RPC/panel, never an env var").
- A numeric knob (sweep interval, if it needs to be configurable rather than a fixed
  constant like `sessionRetentionSweepInterval`) has no existing generic live-settable
  slot — every other sweeper hardcodes its interval as an unexported constant
  (`session_retention_sweeper.go:20`, similarly named constants for the other
  sweepers). Precedent therefore favors **a fixed constant, not a config knob**, for the
  interval itself — only the enabled/disabled toggle needs to be configurable, and that
  should go through `FeatureFlags`.
