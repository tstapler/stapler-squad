# Pitfalls: single-PR to multi-PR session data model

Date: 2026-11-03. Confidence labels: VERIFIED (command run or file opened this session), INFERRED (reasoned from verified facts), GENERAL (standard migration lore, not repo-checked).

## A. Generic pitfalls in single-to-many migrations (GENERAL)

1. **Keep a "primary" shortcut and it will be read as the truth.** Callers keep reading the old scalar and silently act on the wrong row. Fix: make the legacy fields a derived projection of the relation (written from one function), and delete direct writers.
2. **Dual write drift.** Two stores (legacy fields and relation) written from different code paths diverge. Fix: one write function updates both inside one actor critical section; a startup reconciler asserts equality while the flag is rolling out.
3. **Backfill that invents data.** Legacy rows have number but not always URL or host; guessing host as github.com corrupts GHE rows (the existing backfill already refuses to guess, see section B6).
4. **Identity by number alone.** PR numbers repeat across repos and hosts. Key: (host, owner, repo, number). A session can span more than one repo only rarely, but fork sessions and Jules outputs can point at a different repo than `GitHubOwner`/`GitHubRepo`.
5. **"Current" is ambiguous.** Stacked PRs have two open at once. Derived "most recent open" flips when an agent opens another PR; explicit choice goes stale. Pick one rule, document it, and make every action RPC accept an explicit PR id with "current" only as a default.
6. **Terminal flag stays session-scoped.** A per-session "terminal" bool cannot represent "PR A merged, PR B open"; it must move to per-PR state or the poller stops polling the new PR (see B3).
7. **Completion rule written implicitly.** "All PRs merged or closed" vs "last PR merged" vs "AC-based" give different outcomes for an abandoned (closed-unmerged) PR; decide it as an ADR before coding.
8. **Rollback that cannot roll back.** If the old path stops being written, flipping the flag off loses PRs created while it was on. The requirements already say legacy fields are written in parallel; the plan must make that a tested invariant.
9. **Unbounded growth and N+1.** Loading PRs per session in a list RPC without a batched edge query.

## B. Repo-specific pitfalls

### B1. Blast radius is larger than the requirements imply
- VERIFIED: excluding generated code, `.claude/worktrees/`, and tests, `GitHubPRNumber|GitHubPRURL` appears on 107 lines in 27 Go files (grep run from repo root with `--exclude-dir=worktrees`). Without the worktree exclusion it reports ~9956 lines in 2409 files, because stale agent worktrees under `.claude/worktrees/` hold full repo copies. Any audit script must exclude them or it will drown in noise (and a rename sweep must not touch them).
- VERIFIED writers (non-test): `session/instance_actor_setters.go:276-309` (`SetGitHubPR`, `SetGitHubPRNumber`), `:948-949` (`setGitHubResolutionLocked`, only if `r.PRNumber > 0`), `session/pr_status_poller.go:420`, `session/session_driver.go:961-962` (raw `inst.mu.Lock()` write in `scanAndLinkPRURL`, bypassing the setters), `session/session.go:338-339`, `server/services/pr_creation_service.go:305`, `server/services/session_service_lifecycle.go:868`, `server/services/session_service_create.go:896-897`, `server/dependencies.go:1882`. Each overwrites; none appends. This answers the requirements' open question: the field is overwritten by at least five independent flows (create-from-PR URL, PR creation service, terminal-output scan, poller discovery, GitHub resolution).
- Missing a writer reintroduces the stale-PR bug. The plan needs a checklist of these nine sites, and ideally a custom analyzer or `actor-field-guard`-style grep gate so new direct writes to the legacy fields fail CI (INFERRED: `make actor-field-guard`, `Makefile:1183`, covers writes outside `session/instance*.go`; `session_driver.go` is outside that list by name but the guard evidently allows it, so check what it actually matches before relying on it).
- Reads outside session: `server/adapters/instance_adapter.go`, `server/pr_annotations.go`, `review_queue_manager.go`, `prwatch_sessions.go`, `session_retention_sweeper.go`, `unfinished_work_service.go`, `approval_*`, `debug_snapshot.go` (VERIFIED file list). TS: `SessionCard`, `SessionRow`, `SessionActionsOverflow`, `DiffViewer`, `SessionDetailView`, `ReviewQueuePanel`, `UnfinishedItemDetail`, `lib/vcs/adapters.ts` (VERIFIED by grep of `web-app/src`).

### B2. ent schema: generated code is not committed
- VERIFIED (CLAUDE.md): `.gitignore` excludes `session/ent/*.go` and `session/ent/*/`. A new `SessionPR` entity ships only as `session/ent/schema/session_pr.go`. Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`; without `--feature sql/upsert`, `UpsertRule`-style methods vanish and the build breaks. Fresh worktrees need `make build` first (memory: instinct_alias_session).
- Never `git add -f` generated ent output (documented past breakage).
- `session/ent/schema/session.go:136-145` already holds `github_pr_url`, `github_pr_number`, `github_pr_status_terminal`; `backlog_item.go:98-101` has `pr_url`, `pr_number` too. A new entity must decide whether it also replaces the backlog item's scalar pair (the pr_pending gate keys off `PrNumberGT(0)`, see B7).
- Auto-migration adds tables/columns but never backfills. Data migration belongs in `startupMigrations` (`session/ent_repository_migrations.go:76-86`), run on every start, so it must be idempotent and cheap (existing ones shell out to git per item, hence "first: cheap" ordering). Edges in ent SQLite: unique index on (session_id, host, owner, repo, number) is what makes the idempotent backfill safe; omit it and a crash mid-backfill duplicates rows.
- INFERRED: `tools/lint/nowrongdbentity`, `norawdb`, `entfullscan` exist; a new entity queried naively (per-session loop) may trip `entfullscan`. Read those analyzers before writing the repository methods.

### B3. Poller correctness
- VERIFIED: `PRStatusPoller` loop (`session/pr_status_poller.go:306-325`) skips a session entirely when `GitHubPRStatusTerminal` is true, and persists discovery via `inst.SetGitHubPRNumber`. With one terminal bool, a merged PR A makes the poller skip the session, so PR B is never polled (the exact stale-PR bug in reverse). Terminal must become per PR, and the session-level flag must be derived ("all tracked PRs terminal") for `SessionRetentionSweeper`'s safety check (the schema comment at `session.go:143` says that persisted flag exists so retention survives restarts).
- VERIFIED: no-PR backoff is keyed by `inst.Title` (`noPRPollAfter[inst.Title]`, line ~415). Discovery today means "find the PR for the session's branch"; a second PR on a different branch (stacked) is not discoverable by the branch lookup. Decide whether discovery stays branch-based (only finds the head PR) and additional PRs arrive only via explicit attach (`report_pr_created`, PR creation service, terminal scan).
- Actor race: the setters take `s.inst.mu` inside the instance actor and republish `buildSnapshot`. Poller code must not read-modify-write a PR list from a snapshot: two writers (poller and `report_pr_created`) each read the list, append, write back, and one PR vanishes. Append/update must be a single setter that merges inside the lock (idempotent upsert by key). Reads go through `Snapshot()` (`.claude/rules/instance-lock-free-reads.md`); a PR list in `InstanceSnapshot` must be a copied slice (snapshots are shared across goroutines, so mutating a slice header-shared with the live struct is a data race that `-race` only sometimes catches).
- `session_driver.go:961` writes with raw `inst.mu.Lock()` instead of a setter; if the new write goes only through the setter, this path silently keeps the legacy field only. Route it through the new upsert.
- Memory instinct_persistence_writer_caller_check: do not extend a narrow DB writer without a non-test caller grep. `UpdateGitHubPRNumber` (`storage.go:800`) and `UpdateGitHubPRStatusTerminal` (`:792`) are the live narrow writers today; INFERRED, re-grep callers before copying the pattern. Anything not saved by a narrow writer reaches the DB only on full save, so state the crash-loss window for PR state changes.

### B4. ETag and rate-limit cost
- VERIFIED: `github/etag_cache.go:31,52` keys the cache by (owner, repo, prNumber), so it is already per-PR and a PR list does not break caching. Cost scales with open PRs: N open PRs means N conditional GETs per tick, each free on 304 but each counting if the ETag was evicted (`etagCacheEntryTTL`). Requirement says merged/closed are not re-polled, which only holds if terminal is per PR (B3).
- Do not add a "list all PRs for the branch" call per tick; the branch-to-PR list already has its own ETag (`listEtags`). Any new GitHub request must use `NewConditionalRequest`/`NewConditionalRequestNoCache` (`.claude/rules/norawghrequest.md`; `tools/lint/norawghrequest` enforces).
- Two pollers (`PRStatusPoller`, `worktree_pr_poller.go`, plus `jules_session_poller.go` and the prwatch path) could poll the same PR; they share `ETagCache()` (`:132`). A new per-PR loop must use that shared cache or cost doubles.
- "Ask to fix", comments, diff and merge actions each fetch by PR number; making the number a required argument is cheap, but defaulting to "current PR" must not trigger an extra lookup call.

### B5. Persisted JSON and legacy compatibility
- VERIFIED: `session/storage.go:66-67,78-84` has `InstanceData` JSON tags `github_pr_number`, `github_pr_url`, `github_pr_state`, `..._status_terminal`, all `omitempty`; `instance_serialization.go:148-164` and `:344-355` map both directions. Older stored JSON and older binaries (rollback) read only these. Keep them populated, add the new list under a new tag with `omitempty`, and never repurpose a tag. Unknown JSON fields are ignored by `encoding/json`, so an older binary reading newer data loses only the list, but a newer binary writing back through an older reader (flag flip) would drop PRs 2..N unless the relation table, not the JSON blob, is the source of truth.
- `session_artifacts` (schema `session.go:158`, `artifacts/types.go:7` `PRURLs []string`) already records PR URLs seen in scans and is exposed as `PrUrls` in `instance_adapter.go:155`. VERIFIED. It is a second, unkeyed PR list; the plan must say whether it feeds the relation or stays separate, or the UI will show two disagreeing PR lists.
- `session/session.go:338-339` copies from a legacy `Git` struct; check whether the domain model there needs the list too (INFERRED).
- Backfill: `runGitHubPRURLBackfill` (`github_pr_url_backfill.go`) deliberately skips rows whose host cannot be recovered. The new migration must reuse that host recovery (`hostFromClonedPath`) and skip, not guess, otherwise GHE rows get a github.com key.

### B6. Proto and mobile compatibility
- VERIFIED: `proto/session/v1/types.proto:93,96` `github_pr_number = 25`, `github_pr_url = 26`; also state/draft/priority at 157-163 and other messages at 831 and 1704-1707. Additive `repeated PullRequestRef` with a fresh field number is non-breaking; reusing, renumbering or retyping existing tags fails `make proto-breaking` (`Makefile:650`, compares against origin/main, needs a reachable origin/main). Keep populating the scalar fields with the current/active PR so old mobile clients still render one PR. Mobile app is a separate client (memory: project_mobile_app_server); it will not regenerate until released, so scalar fields are the contract until then.
- `web-app/src/gen/session/v1/types_pb.ts` is generated; proto changes need `make proto-gen` and gen dirs are gitignored (fresh worktrees need a build).
- Server-side defaulting: proto3 `0`/`""` mean "no PR"; an unset explicit `pr_ref` in an action request must fall back to current, never to PR number 0.

### B7. Backlog completion and `pr_pending`
- VERIFIED: `session/backlog_lifecycle_pr.go` (e.g. lines 163-216) documents that `FindPRPendingItems` filters `PrNumberGT(0)`; an item in `pr_pending` with `pr_number=0` is invisible and permanently stuck (BUG-040 detector). With multi-PR, an item whose current PR is cleared or replaced can hit the same invisible state. The reconcilers (`reconcileOrphanedAgentPRs`, `:320` area) set the scalar pair when they recover a PR; they overwrite too.
- The completion gate is AC-based (`docs/reference/backlog-completion-gate-and-cleanup.md`); adding "all PRs terminal" changes a gate that already has a synchronous terminal-transition cleanup. Tests for this gate must not use wall-clock thresholds (memory: instinct_new_rpc_ci_gates).
- Item to session linking can be broken mid-review (memory: instinct_backlog_session_link_broken_mid_review); a per-item PR list must not depend on a live session link.
- `server/mcp/tools_backlog_pr.go` and `tools_github.go` (`report_pr_created`) are the agent-facing writers; the MCP tool surface needs server-side gating, `--allowedTools` is client-side only (memory: instinct_mcp_dispatched_agent_tool_surface). A new MCP tool "attach PR" must check caller identity so a dispatched agent cannot attach arbitrary PRs to other sessions or items.

### B8. Archived-session revival lint
- VERIFIED rule: `.claude/rules/noarchivedrevival.md`, analyzer `tools/lint/noarchivedrevival`, file set includes `session/session_driver.go`, `session/instance_claude.go`, `server/dependencies.go`. New PR-aware automation that calls `Start(false)`, `RecoverFromStopped()`, `restartForRetry` or `transitionToLocked(..., Active)` (for example "PR B opened, revive session to rework") in those files needs an archival guard first. Note `restartForRetry`-style paths in "ask to fix" on a merged PR A while session is archived are the likely trip. Merged-PR events on an archived session must be record-only.
- A PR merged event that moves a session to a terminal state must not un-archive it. Writing PR history for archived sessions is fine (no revival involved), but the poller currently skips suspended sessions (`:313`), so archived sessions never get updated PR state; decide whether that is acceptable.

### B9. New RPC and CI gates (memory-derived, VERIFIED text, not re-run)
- instinct_new_rpc_ci_gates: (1) `server/services/terminal_write_rpcs_test.go` needs each new RPC in `terminalWriteRPCs` or `notTerminalWriteRPCs`; builds only with `-tags sinkguard`; run `go test -tags sinkguard -short -run UIRpcSurface ./server/services`. (2) Set `tested`/`testIds` by hand in `docs/registry/features/backend/*.json` after `make registry-generate`; fix all new RPCs at once. (3) No "gate" in `server/services/*_test.go` filenames (BUG-087 deliverygate guard). (4) No wall-clock assertions. (5) A new proto service (not just RPC) bumps a hard-coded count in `terminal_write_callers_test.go`. Prefer extending the existing session service over adding a service.
- A PR action RPC that mutates GitHub (merge, close) is a "write" in this classification; listing RPCs (`ListSessionPullRequests`) is not. Misclassifying either fails the test.
- instinct_pr_ci_gates_lint_routes_config_surface: rollout flag must be registered in `feature_flag_service.go` AND have an entry in `server/services/config_surface_proof_test.go` (or `exempt: "client-only: ..."`); run `go test ./server/services -run TestConfigSurfaceProof` after every merge of main. Flag must be live-settable via feature-flag RPC, not env (memory: feedback_rollout_flags_live_settable_no_env_vars). Per-scope override should be supported.
- Web lint: `web-app/scripts/eslint-gate.sh` is zero-warning over changed files; merging main can convert warnings in your files into failures. Run the documented command from `web-app/` before pushing.
- A new page needs rows in `lib/nav/paths.ts`, `app/route-meta.ts`, `app/routes.tsx`. A PR-history view as a tray or panel inside the session detail avoids this entirely; prefer that.
- Feature registry: `make registry-generate` and commit changed `docs/registry/features/` files when adding RPCs or components, with `// +api:` and `// +feature:` markers.
- Merge main again before final push (memory: instinct_ci_tests_against_live_base_not_stale_merge); CI tests the live base.

### B10. Duplication gates
- Go `dupl` is new-code-only vs `origin/main` (`make ready-complexity-gate`, `Makefile:1088`; needs `dupl` from `make install-tools`). Risk: per-PR-action handlers (merge, close, comments, ask-to-fix) that each repeat "resolve PR ref, load session, validate" will be flagged. Write one `resolveTargetPR(session, req)` helper first.
- Same gocognit/gocyclo/funlen gate applies to modified existing functions, not only new ones (memory: instinct_go_git_native_plumbing_gotchas). Editing the long `PRStatusPoller` loop (`:306-420`) to add per-PR iteration will trip it; extract before editing.
- web-app `jscpd` is an absolute threshold (0.14%, `web-app/.jscpd.json`), not diff-scoped. Risks: copy-pasting PR badge/status markup between `SessionCard`, `SessionRow`, `SessionDetailView` (already three consumers); heavily-mocked new test files add irreducible `jest.mock` blocks (memory: plan-feedback margin was 0.10% vs older threshold; check current headroom with `bun run lint:duplicates` in `web-app/` before and after). Build one `PullRequestList`/`PullRequestChip` component and reuse.

### B11. Testing traps
- Tests must construct sessions with PR A merged and PR B open and assert action target (success metric 2). Avoid `time.Sleep`/real timers (skill deterministic-fast-tests). Pollers have existing injected-clock tests (`pr_status_poller_test.go`, `worktree_pr_poller_*_test.go`); extend those seams.
- BUG-087..090 are known flakes in CI (memory: instinct_ci_hermetic_testing_gotchas); a red unrelated test needs a main check before debugging (instinct_ci_failure_check_main_first).
- E2E: specs need `// @feature` header, no `waitForTimeout`, `data-testid`/ARIA locators only; new helpers in `tests/e2e/pages/`.
- Use a short non-git temp path for fixtures (memory: full /tmp + long GOTMPDIR); keep worker scratch on disk, not tmpfs.

### B12. Process pitfalls from memory
- Do not open ad-hoc PRs from the main checkout session; use worktree sessions (feedback_no_ad_hoc_prs_from_main_session). Cap concurrent backlog sessions at 2.
- Parallel workers in one worktree sweep each other's hunks (feedback_parallel_workers_one_worktree...): this change spans session/server/web/proto, so give each worker its own worktree created from the feature branch, and merge in an order that keeps `make build` green (proto first, then ent schema, then session, then server, then web).
- Review-repair loops overbuild (feedback_review_repair_loops_overbuilding): set a stop rule; repeated blockers in the poller or gate means simplify.
- Check live PR state before pushing follow-ups; a squash-merge mid-session strands later commits.

## C. Highest-risk items to design explicitly
1. Per-PR terminal state replacing the session-level terminal bool (B3), including retention sweeper semantics.
2. Single upsert setter inside the actor lock; all nine writers (B1) migrated, with a guard against new direct writes to the legacy fields.
3. Active-PR rule and "scalar fields = projection of active PR" contract for proto/JSON compatibility (B5, B6).
4. Backlog completion rule and keeping `FindPRPendingItems` from losing items (B7).
5. Idempotent, host-safe backfill with a unique key (B2, B5).

## D. Open items I could not verify
- Whether `actor-field-guard` covers `session_driver.go:961`.
- Whether `entfullscan`/`nowrongdbentity` analyzers constrain a new entity's queries.
- Whether `worktree_pr_poller.go` and the prwatch path write the legacy fields indirectly (not in the writer grep, but they call poller or server paths).
- Current jscpd headroom (not run).
- `scripts/` guard script names (a `ls scripts | grep` printed nothing).
