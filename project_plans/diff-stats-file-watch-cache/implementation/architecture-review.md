# Architecture Review: diff-stats-file-watch-cache
**Date**: 2026-09-10
**Verdict**: CONCERNS

## Constitution Violations
- None — no `docs/adr/ADR-000-architecture-constitution.md` found in the repo (`ls docs/adr/` has no `000`-prefixed file).

## Blockers (iteration 2 — re-review of iteration 1's 2 blocked items only)

- [x] **Task 1.1.2a (`GitWorktree.IsDirtyUncached`)** — **RESOLVED.** Plan now shows:
  ```go
  func (g *GitWorktree) IsDirtyUncached() (bool, error) {
  	return worktreeIsDirtyFast(g.GetWorktreePath(), &g.gitignoreFS, &g.headTreeCache)
  }
  ```
  (`project_plans/diff-stats-file-watch-cache/implementation/plan.md:182-184`). This passes the real `&g.gitignoreFS`/`&g.headTreeCache` fields — matching their declared types as used elsewhere in `worktreeIsDirtyFast`'s other call sites — instead of the discarded `nil, nil` from iteration 1. Task 1.1.2b adds a regression test (`TestIsDirtyUncached_ReusesGitWorktreeCaches`) explicitly aimed at catching a future regression back to `nil, nil`. No new problem introduced by this fix.

- [x] **Task 2.1.1a (`WorktreeChangeDetector.fire()`)** — **RESOLVED.** Plan now shows each callback wrapped in its own `recover()` (`plan.md:296-310`):
  ```go
  func (d *WorktreeChangeDetector) fire() {
  	d.mu.Lock()
  	callbacks := d.onChange
  	d.mu.Unlock()
  	for _, fn := range callbacks {
  		func() {
  			defer func() {
  				if rec := recover(); rec != nil {
  					log.Warn("worktree change detector: OnChange callback panicked (recovered)", "worktreePath", d.worktreePath, "panic", rec)
  				}
  			}()
  			fn()
  		}()
  	}
  }
  ```
  This logs the recovered panic (with `worktreePath` and the panic value) rather than silently swallowing it, matching this codebase's convention — verified against `session/ent_repository_backlog.go`'s `publishItemChangedSnapshot`, which likewise recovers and logs via `log.WarningLog().Printf("... panicked (recovered): %v", rec)`. No new problem introduced by this fix (each callback gets an independent `recover()`, so one panicking callback still doesn't block or skip the others).

Both iteration-1 blockers are resolved with no regressions introduced by the fixes. No new blockers identified — this iteration was scoped only to the 2 items above, not a full re-review.

## Concerns
*(carried over verbatim from iteration 1 — not re-reviewed this iteration)*

- [ ] **Task 2.3.2a/c (`watchedWorkdirs sync.Map`) / Tech Debt Disposition table** — The disposition's stated safety bound doesn't actually hold as specified. `watchedWorkdirs` keys its "already subscribed" idempotency check purely by `workDir` string, with no notion of *which* instance's detector currently backs the subscription. If two live `Instance`s ever share a workdir (already an accepted-rare, pre-existing possibility per this same table), and the first instance to reach the cache-miss path is later torn down (`Cleanup`/`Remove` → `stopChangeDetector()` → `Stop()`s its detector, after which its registered callback simply never fires again), `watchedWorkdirs.LoadOrStore` keeps reporting "already subscribed" for that workdir forever — so a second, still-live instance sharing the same path never gets its edits wired to `ws.vcsStatusCache.Delete(workDir)` for the rest of the process's lifetime, not bounded to 5 minutes as the disposition claims ("bounds it exactly as tightly as today's 15s TTL bounds it, just wider"). That claim is false in this specific, if rare, sequence.
  **Remediation**: key the subscription-idempotency check by which live instance/detector currently owns it (re-subscribe when a different resolved `instance` shows up for the same workdir), or — simpler, since the plan already establishes that invalidation is cheap and idempotent — drop the workdir-only dedup and instead dedup per resolved instance identity, so a still-live sharing instance always ends up with its own registration regardless of another instance's teardown.

- [ ] **Task 2.2.1a (`changeDetectionActive bool` alongside `changeDetector *WorktreeChangeDetector`)** — Redundant state: both fields are always set/cleared together today (`startChangeDetector`/`stopChangeDetector` write both under one critical section), but nothing prevents a future edit from updating one without the other, producing exactly the "detector active but flag says off" / "flag says on but detector is nil" illegal state the type-driven-design checklist flags. `GitWatchActive()` already demonstrates the safer pattern (`detector != nil && detector.GitWatchActive()`) one line away from `WorktreeChangeDetectionActive()`, which instead reads the separate bool.
  **Remediation**: drop `changeDetectionActive` and derive `WorktreeChangeDetectionActive()` from `gm.changeDetector != nil`.

- [ ] **Task 2.1.1a / Story 2.3.2 (`OnChange` doc contract vs. actual usage)** — `OnChange`'s doc comment states "Must be called before Start()." `GitWorktreeManager.startChangeDetector` honors this, but `WorkspaceService`'s lazy subscription (Task 2.3.2c) calls `instance.OnWorktreeChange(fn)` → `detector.OnChange(fn)` well after `Start()` already ran (on the first `GetVCSStatus` cache-miss, potentially minutes later) — directly contradicting the documented precondition. The implementation happens to be safe for post-`Start()` registration (both `OnChange`'s append and `fire()`'s read go through `d.mu`), so the precondition is simply inaccurate, not enforced — leaving a false constraint in the code next to a real caller that relies on violating it.
  **Remediation**: fix the doc comment to state registration is safe at any time (mutex-protected append/read), matching what `WorkspaceService`'s usage actually requires.

- [ ] **Task 2.2.1f (`GitManager` interface growth)** — The interface already has 30 methods pre-plan; this plan adds 3 more (`WorktreeChangeDetectionActive`, `GitWatchActive`, `OnChange`) with no stated ISP trade-off. Per this repo's own architecture guidance, "extend as-is" on an already-large interface should be stated explicitly, not silent.
  **Remediation**: add one sentence to the Pattern Decisions table naming this as a deliberate "extend as-is" (consistent with existing convention, splitting out of scope for this project).

## Nitpicks
*(carried over verbatim from iteration 1 — not re-reviewed this iteration)*

- `WorktreeChangeDetector.gitWatchLoop`'s deferred `watcher.Close()` never nils `d.gitWatcher`, so `GitWatchActive()` would misreport `true` after `Stop()` if called directly on the detector rather than through `GitWorktreeManager` (which always nils its own pointer first — currently unreachable in the planned call graph, but cheap to fix defensively).
- Observability Plan's "Warn emitted at most once per worktree lifecycle" is a simpler, stronger guarantee than requirements.md's literal ask to mirror the `severePressureWarned`-style rate-limited pattern — functionally fine, just note the mechanism diverges from the literal ask (not its intent).
- Per `kibitzer architecture export --scope github.com/tstapler/stapler-squad/session`, `GitWorktreeManager` currently has 31 tracked symbols; this plan's additions (2 fields + 5 methods) grow it to ~38. This is a modest, cohesive addition (all lifecycle passthrough for one feature, not a new unrelated concern) and doesn't by itself push `GitWorktreeManager` into God Object territory — worth tracking only if a future change adds a 6th unrelated responsibility on top.
- `.claude/inspect.json` has no `architecture.components`/`dependency_rules`/`content_rules`/`naming_rules` configured, so the mechanical layering/naming checks this review would otherwise run (`component-deps`, `content-rules`, `naming-rules`) were skipped; the session↔server/services layering check above was done by manual grep confirmation instead.
