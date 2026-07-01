# Backlog Feature — Implementation Inventory

Audit date: 2026-07-01. Scope: read-only inventory of what is actually built vs. planned,
gated behind `config.GetFeatureFlag("backlog")` (default off). No code changed.

---

## Capability Matrix

| Capability | Status | Where it lives | Exposed via |
|---|---|---|---|
| Create backlog item | Fully implemented | `server/services/backlog_service.go:396` `CreateBacklogItem`; `web-app/src/components/backlog/BacklogItemForm.tsx`, `BacklogEmptyState.tsx` | UI + RPC |
| List / filter / board view | Fully implemented | `BacklogService.ListBacklogItems`; `BacklogBoard.tsx`, `app/backlog/page.tsx`, `app/backlog/board/page.tsx` | UI + RPC |
| Update / edit item, AC list | Fully implemented | `UpdateBacklogItem`; `BacklogItemDetail.tsx`, `AcCriteriaList.tsx` | UI + RPC |
| Status state machine (idea→refining→ready→in_progress→review→done/archived) | Fully implemented | `session/backlog.go` (`CanTransitionBacklog`, `TransitionGuard`); `TransitionBacklogItemStatus` handler | UI + RPC, unit-tested (`backlog_test.go`) |
| Archive item | Fully implemented | `ArchiveBacklogItem` handler; archive action wired in `BacklogItemDetail.tsx` | UI + RPC |
| AI triage / planning (research + plan.md + validation.md) | Fully implemented (headless, rewritten from the original tmux/AutonomousDriver design) | `TriggerTriage`/`CancelTriage` handlers (`backlog_service.go:1101-1305`); `session/backlog_triage.go` (`BuildHeadlessTriagePrompt`, `ParseHeadlessTriageResult`, brace-scan per ADR-001); UI: `TriageLoadingIndicator.tsx`, `TriageErrorBanner.tsx`, `BacklogItemDetail.tsx` | UI + RPC + MCP (`submit_triage_result` still registered but the headless prompt explicitly forbids calling it — dead code path for the current triage flow) |
| Plan approval gate | Fully implemented | `ApprovePlan` handler; `TransitionGuard` enforces `plan_approved \|\| skip_planning` before `ready→in_progress`; UI approve button in `BacklogItemDetail.tsx` | UI + RPC |
| Spawn work session from item | Fully implemented | `SpawnSessionFromItem` (`backlog_service.go:883`); writes slash commands + `.backlog-context.md` (`session/backlog_commands.go`) | UI + RPC |
| Attach existing session to item (retroactive linking) | **Partial — backend/RPC only, no UI caller** | `AttachSessionToItem` handler (`backlog_service.go:1007`) | RPC-only (grep of `web-app/src/` finds zero callers outside the generated `backlog_pb.ts` bindings) |
| Autonomous execution (no human in the loop) | Fully implemented as a flag, not a separate mode | `SpawnSessionFromItemRequest.autonomous` bool → `AutonomousDriverStarter.StartAutonomousDriverForInstance` (`backlog_service.go:965`). Feature registry lists `backlog-spawn-session-autonomous` as if it were a distinct RPC (`server/features/backlog.go:116`) but it is the same `SpawnSessionFromItem` RPC — registry entry is a naming/documentation mismatch, not a real second endpoint | UI (autonomous toggle) + RPC |
| Review gate (post-session verdict) | Fully implemented (headless, matches ADR-013 intent but executor changed from "spawn tmux session" to "direct headless LLM call") | `session/backlog_lifecycle.go` (`onSessionExited` → `spawnReviewGate`); headless path via `headless.Pool.CallBlocking` + `ParseHeadlessVerdictResult`; legacy tmux-spawn path (`ReviewGateSpawner`) retained for backward compat but headless is primary; pre-gate secret scan (`RunPreGateSecurityCheck`) blocks and records a FAIL verdict | UI (`GateVerdictBox.tsx`, `TriageReviewPanel.tsx`) + MCP (`submit_review_verdict` for the legacy tmux path) + automatic (lifecycle hook) |
| Verdict override (mark done / reopen despite verdict) | Fully implemented | `OverrideVerdict` handler (`backlog_service.go:1336`) | UI + RPC |
| Re-review (manual) | Fully implemented, degrades gracefully | `TriggerReReview` (`backlog_service.go:1422`); falls back to a placeholder response if `sessionCreator` is nil rather than erroring | UI + RPC |
| GitHub Issues sync (external source ingestion) | Fully implemented as a background poller, but **operationally unreachable from the UI** | `session/backlog_sync.go` (`SyncLoop`), `session/backlog_plugin_github.go`; wired via `BacklogController.Enable()` (`session/feature_controller.go`) which starts `SyncLoop.Start` automatically once the feature flag is on and at least one `ItemSource` exists | RPC-only for source CRUD (`CreateItemSource`/`ListItemSources`/`UpdateItemSource`/`DeleteItemSource` all implemented); **zero frontend UI** — no source-settings page exists (`grep` for `ItemSource`/`CreateItemSource` in `web-app/src/` returns 0 matches outside generated bindings) |
| GitHub PRs plugin | Fully implemented, registered, never surfaced | `session/backlog_plugin_github_prs.go`, registered in `NewDefaultRegistry()` (`session/backlog_plugin.go:55`) | Backend-only; no UI, same reachability gap as Issues sync |
| Manual "trigger sync now" | **Stub** | `TriggerSync` handler returns `connect.CodeUnimplemented` (`backlog_service.go:1589-1594`) | RPC endpoint exists in proto/registry but always errors |
| Sync history view | **Stub** | `GetSyncHistory` handler returns `connect.CodeUnimplemented` (`backlog_service.go:1596-1603`) | Same as above |
| Session-linkage git activity badge | Fully implemented | `BacklogItemBadge.tsx`, `BacklogItemPanel.tsx`, `SessionMonitor.tsx` | UI |
| Feature flag gating (default off, defense-in-depth) | Fully implemented, both layers shipped | Frontend: `web-app/src/app/backlog/layout.tsx` (client-side redirect guard, handles `isLoading` race correctly); Backend: `server/interceptors/feature_flag_interceptor.go` wired only to `BacklogServiceHandler` in `server/server.go:374` | UI + RPC interceptor |

---

## Previously-Flagged Unresolved Risks (mined from planning docs)

From **backlog-triage-autonomous** adversarial-review / architecture-review / pre-mortem (2026-06-22), later addressed by commits `2d7e116c`, `b1f63bab`, `069f80ef`, `7ad33400`:

- **"Pool not wired into BacklogService — silent no-op on deploy"** and **"goroutine context is the HTTP request context, not a server lifecycle context"** (architecture review) — **Resolved.** Current code has `headlessPool headless.PoolClient`, `shutdownCtx`/`shutdownCancel`, and `SetHeadlessPool` on `BacklogService` (`backlog_service.go:64-117`); the triage goroutine derives its 30-min timeout from `s.shutdownCtx`, not `context.Background()` (`backlog_service.go:1219`).
- **"Auto-triage from CreateBacklogItem bypasses the new headless path"** (gates on `sessionCreator != nil` instead of the headless pool) — **Resolved.** `CreateBacklogItem` now gates on `s.headlessPool != nil` (`backlog_service.go:435`).
- **"Concurrent re-trigger TOCTOU window between orphan-check and goroutine start"** — **Resolved.** `ItemSession` is created synchronously before the goroutine is spawned (`backlog_service.go:1183-1192`), and an orphan-aware guard tombstones dead headless sessions (`backlog_service.go:1138-1153`).
- **"Type duplication creates a schema-drift risk"** (three copies of triage suggestion/task types) — **Resolved.** Canonical `TriageSuggestion`/`TriageTask`/`HeadlessTriageResult` types now live in `session/backlog_triage.go` and are imported elsewhere.
- **"WorkDir + FakeRunner incompatibility breaks the integration test"** — Addressed via a `HeadlessPoolClient`/`headless.PoolClient` interface (referenced in commit `7ad33400 feat(harness): headless triage test harness`); not independently re-verified line-by-line in this audit but the interface exists as recommended.
- **"FeatureKeyTriage must not be added to AllowedFeatureKeys"** — appears followed (triage is an internal-only call path via the goroutine, not exposed as an externally-invocable headless feature key in the RPC surface reviewed).

From **backlog-triage-e2e-hardening** adversarial-review / architecture-review (2026-06-23), addressed by commit `06f98573`:

- **"Brace-scan breaks on JSON embedded in research content"** — **Still an accepted risk, not a bug.** The shipped parser (`session/backlog_triage.go:95-96`) uses `strings.Index`/`strings.LastIndex`, the exact pattern the reviewers flagged as fragile for multi-step triage output that echoes research file content containing `{`/`}`. No secondary fallback was added. This is a **live, unaddressed risk** — the triage prompt (`BuildHeadlessTriagePrompt`) does instruct the model to write 4 research files with prose (which can contain braces) and then emit JSON, so the failure mode described in the review remains structurally possible.
- **"E2e test creates item via wrong path, making the guard unreachable"** and **"Test cleanup uses DeleteBacklogItem RPC which does not exist"** — both were BLOCKER-severity in the adversarial review; the plan's own fix commit `06f98573 fix(backlog): harden triage parser and add repoPath UI gate` suggests the repoPath UI guard shipped. Not independently re-verified against current `tests/e2e/backlog.spec.ts` content in this pass — worth a follow-up check before trusting e2e coverage claims for the triage gate.
- **"Happy-path e2e test (Success Metric #3) is entirely unaddressed"** — no evidence found in this audit that a full "trigger triage → verify ready" Playwright test exists; only the disabled-button negative-path test was in scope for the hardening plan.

From **backlog-management** ADR-012 (context injection) — **Resolved / superseded in-repo.** Current code matches the superseding decision: `SpawnSessionFromItem` builds a token-budgeted prompt (`session.BuildTokenBudgetedPrompt`) rather than mutating CLAUDE.md, and writes slash commands + `.backlog-context.md` via `session/backlog_commands.go`.

From **backlog-management** implementation/validation.md — this was the original MVP plan (five ent schemas, `SuggestNextItem`, GitHub-sync-with-UI, drift detection via inotify). The **as-built** system diverges from it in several ways not previously flagged as risks but worth surfacing:
- `SuggestNextItem` RPC exists (`backlog_service.go:1306`) but no evidence of an inotify-based file-watch drift detector — git-log/commit-polling activity signals appear to be the only mechanism, not inotify as originally specified in the MVP plan's "inotify File Descriptor Budget" section.
- The originally-planned GitHub sync UI (`SourceSettings` component, `/backlog/settings` route) was never built — confirmed by this audit (see capability matrix). This was not previously flagged as a *risk* in any adversarial review — it appears to have been silently descoped between the original MVP plan and current implementation, with no ADR recording that decision.

---

## Fragility Signals (recent commit history)

`git log --oneline --all --grep="backlog\|triage\|autonomous" -i`, most recent first:

1. `1186494b`/`66b1d831` (2026-06-29) `feat(backlog): gate backlog behind feature flag on all layers` — the change that prompted this audit; confirms the flag was *just* added, meaning most of the feature shipped and was live-by-default before this.
2. `a690c366` (2026-06-23) `feat(backlog): implement CancelTriage RPC and session delete button`
3. `06f98573` (2026-06-23) `fix(backlog): harden triage parser and add repoPath UI gate` — direct fix from the e2e-hardening plan above.
4. `2d7e116c` / `b1f63bab` (2026-06-22) `fix(backlog): replace idle triage sessions with headless pool calls` — this is the big rewrite: the original tmux+AutonomousDriver triage path (from the `backlog-management` MVP plan) was broken (idle-detection timing, 5-min timeout too short for 15-min triage) and was replaced wholesale with the headless-pool architecture documented in `backlog-triage-autonomous`.
5. `069f80ef` (2026-06-22) `fix(backlog): address Phase 6 review findings`
6. `7ad33400` `feat(harness): headless triage test harness + alias kebab-case fix`
7. `60306163` `fix(triage): address code review findings from post-commit review`
8. `59d41da1` `fix(triage): surface storage errors, add timeout, harden claude detection, link notifications to backlog`
9. `633e88d7`/`3527fc64` `feat(autonomous): triage migration, review queue UX, backlog MCP tools, session detail polish`
10. `ad43bbbd` `fix(autonomous): address all triad-review blockers`

**Reading of the pattern**: the triage/autonomous execution path has been rewritten at least once end-to-end (tmux+AutonomousDriver → headless pool) due to production fragility (idle-detection timing, timeouts), and has accumulated a steady stream of "fix" commits through late June 2026 right up to the feature-flag-gating commit. This is the most fragile subsystem in the feature by commit-frequency. The review gate underwent the same tmux→headless migration pattern (`spawnReviewGate` now has both legacy and headless code paths coexisting in `backlog_lifecycle.go`). The feature-flagging itself (default-off) landed only days after the last triage fix, consistent with the user's stated distrust of the autonomous flows — the flag appears to be a direct response to this fragility history rather than a pre-planned rollout gate.

---

## Stubs / TODOs Found in Code

- `server/services/backlog_service.go:1589-1594` — `TriggerSync` RPC: `return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("TriggerSync not yet implemented"))`. No manual "sync now" capability; users must wait for the `SyncLoop` ticker.
- `server/services/backlog_service.go:1596-1603` — `GetSyncHistory` RPC: same `CodeUnimplemented` stub. `SourceSyncEvent`-equivalent history is not retrievable via RPC even though the sync loop presumably records some run state internally (not verified in this pass whether any history is persisted at all, or just dropped).
- No `TODO`/`FIXME`/`HACK`/`XXX` comments found anywhere in `session/backlog*.go`, `server/services/backlog_service.go`, `server/mcp/tools_backlog.go`, or `web-app/src/components/backlog/*` — the two `CodeUnimplemented` stubs above are the only explicitly-unfinished code paths.
- **Dead/vestigial code, not a marked stub but worth flagging**: `submit_triage_result` MCP tool (`server/mcp/tools_backlog.go:423`) is still registered, but the current headless triage prompt (`session/backlog_triage.go:84`) explicitly instructs the model "Do NOT call submit_triage_result" — this tool is now only reachable by a hypothetical non-headless/tmux triage session, a code path that no longer exists in `TriggerTriage`. Same pattern for `ReviewGateSpawner`/`SpawnReviewSession` in `backlog_lifecycle.go` — retained "for backward compatibility with existing tests and callers" per its doc comment, but the headless path is what actually runs when `headlessPool` is set (which it is whenever `claude` binary is available).
- **Feature registry inconsistency, not a code stub**: `server/features/backlog.go` declares `BacklogSpawnSessionAutonomous` as if it were a distinct RPC (`RPCIDs: []string{"backlog:spawn-session-autonomous"}`), but there is no such RPC — it's the `autonomous` bool field on `SpawnSessionFromItemRequest`. This could cause the feature-registry tooling (`make registry-generate`/`registry-diff`) to report a phantom RPC or fail to detect the real touchpoint.

---

## Key File Reference

- Backend core: `session/backlog.go`, `session/backlog_lifecycle.go`, `session/backlog_triage.go`, `session/backlog_review.go`, `session/backlog_context.go`, `session/backlog_commands.go`, `session/backlog_sync.go`, `session/backlog_plugin*.go`, `session/backlog_crypto.go`, `session/feature_controller.go`
- Service/RPC layer: `server/services/backlog_service.go` (1603 lines, 22 handlers), `server/mcp/tools_backlog.go` (681 lines, 5 tools registered)
- Feature flag plumbing: `web-app/src/app/backlog/layout.tsx`, `server/interceptors/feature_flag_interceptor.go`, `server/server.go:374`
- Proto: `proto/session/v1/backlog.proto` (389 lines, 21 RPCs)
- Frontend: `web-app/src/components/backlog/` (39 files — `BacklogItemDetail.tsx` at 886 lines is the largest and most central), `web-app/src/lib/hooks/useBacklogService.ts` (556 lines)
- Notably absent: any `SourceSettings`/`/backlog/settings` frontend route for GitHub source configuration.
