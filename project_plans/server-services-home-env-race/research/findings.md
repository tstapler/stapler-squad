# Research: server/services env-var race

## Verified (commands run in this session)
- Baseline `main` (clean worktree at HEAD `3c825969e`): no failure reproduced in 10 full-package runs
  (4 plain concurrent, 6 `-race` concurrent) or 45/45 runs of the three named suspect tests under `-race -count=15`.
- Shape 2 appears only as a non-fatal WARN, 76x per run, always at the same log lines: `failed to save default config ... config.json.N.tmp: no such file or directory`.
  Source: `config/config.go:1443` (`loadConfigWithDefaultFallback`). Emitting tests include unisolated
  `TestSearchGitHubRepos_DefaultLimit` and `TestImportGitHubIssue_*`: with neither env var set, `GetConfigDirForDir`
  (`config/config.go:158-166`) returns `~/.stapler-squad/test/test-<pid>` without creating it.
- Migration v1 (non-reentrant locks held until cleanup) deadlocked: `TestTestRemoteConnection_TwoServiceInstances_DoNotShareAPoolEntry`
  calls `newTestRemoteService` twice (`remote_service_test.go:271,274`), and that helper calls `withIsolatedStateDir`.
  20m timeout, goroutine dump shows `sync.Mutex.Lock` at `home_env_test_helper_test.go` in `withIsolatedStateDirPath`.

## Design limit
`homeEnvMu`/`stateEnvMu` serialize *mutators* only. A goroutine leaked from test A reads `HOME` /
`STAPLER_SQUAD_TEST_DIR` live (`config/config.go:124,132`) and can resolve into test B's directory regardless of
either lock. Fixing that class requires capturing the resolved path at spawn time or joining goroutines
before cleanup.

## Separate finding: production data race in `Instance.ToInstanceData` (not an env-var race)
Seen once in `go test -race -count=5 ./server/services/` on the migrated tree
(`TestCreateSession_should_ReachActiveViaPipeline/ModeIsNewWorktree`, "race detected during execution of test").
- Write: `session/git_worktree_manager.go:489` (`gm.diffStats = stats` in `SetDiffStats`, under `gm.mu.Lock`), goroutine 29755,
  from `Instance.Destroy` -> `destroyChain` -> `UpdateDiffStats` (created at `server/services/session_service_lifecycle.go:194`).
- Read: `session/instance_serialization.go:203` (bare `i.gitManager.diffStats`, no lock) in `ToInstanceData`, goroutine 29713,
  from `runBackgroundResolutionPipeline` -> `Storage.UpdateInstance` (`session_creation_pipeline.go:296`).
- Root cause: unguarded field read against a mutex-guarded write; fix is `GetDiffStats()` (same file, one site).
- Scope: filed here because AC 6 (`-race -count=5` clean) cannot pass while it exists; kept as its own commit so it can be split out.
- Baseline `main` showed 0 races in 6 `-race` runs, so "pre-existing" is INFERRED (the migration diff touches tests only), not measured.

## Open
Which goroutine actually leaks: see goleak experiment in `implementation/plan.md`.
