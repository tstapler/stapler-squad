# Changelog

## [1.27.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.26.0...v1.27.0) (2026-05-06)


### Features

* **classifier:** safely parse multiline python -c blocks with # comments ([#97](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/97)) ([be05345](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/be05345332c65b431a656002f0223917ee0f3c47))
* **executor:** safe subprocess framework with zombie prevention and process group management ([4d03970](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4d03970402070b9bd841962fe25a0ac24df0e155))
* **lint:** add norawexec lint analyzer and safeexec framework plans ([cf96301](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cf963014f6d95290cbf94eac9371e8aa48f7ee77))
* **omnibar:** auto-populate session name, first prompt injection, inline shorthand ([#95](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/95)) ([58d94a7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/58d94a746d377fde1fb395bdca482747a1fc6bb7))


### Bug Fixes

* **control-mode:** eliminate 3s timeout race and blank terminal on dead session ([006b45a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/006b45a245ac6d5609166b946739ab13f856cb02))
* **executor:** resolve golangci-lint violations in new executor test files ([d6f154f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d6f154f7789dbc89d2193d523161f3be79a34ace))
* **session:** add WaitDelay to all missing subprocess calls to prevent zombie accumulation ([3224302](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/32243022110659eda4dbcbeea94a0464392e8dfc))

## [1.26.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.25.0...v1.26.0) (2026-05-05)


### Features

* **adr:** add ADR-010 frontend modularity + enforce with eslint-plugin-boundaries ([685eaba](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/685eaba154332bacf854edb954d3ba3d6f7418e4))
* **classifier:** add approval rules, parser fixes, and CommandPattern linter ([#93](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/93)) ([a809681](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a809681d6a0fbe1672143cc942db780768a2fecf))
* **classifier:** auto-allow gh api PR review workflow commands ([#46](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/46)) ([e996dce](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e996dce6d66a28b245963e60dd22dbc9ed8d4cd7))
* **debug:** stream browser console logs to server via LogClientEvents RPC ([efac73a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/efac73abcf28c00b66f1b1dcc838a290528ba452))
* **engineering-excellence:** DI framework, error observability, CI hardening ([b2f692f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b2f692ff6384c834d2b81c1da8ec45bd4174b2bc))
* **engineering-excellence:** DI framework, error observability, CI hardening ([13ec9f3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/13ec9f3e5237fe26754ebab557a7d0d7edcb8a1e))
* **file-tree:** performance, themes, and file browser enhancements ([#86](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/86)) ([3f25b10](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3f25b10a2e2877bcc60d0cbfd522bc2d3be728bb))
* **lint:** add hotpolllog AST linter and remove stale debug logs ([e40e531](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e40e53165ee4eb0862436c006ade8572a78bd352))
* **log:** runtime log level control via REST API and debug menu ([34bcaf0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/34bcaf0d8c7298da708471828ce06a39a788d479))
* **mobile:** collapse secondary toolbar actions into ··· overflow menu ([ef342b6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ef342b6312fb09a34d891420612c0215fcad0bd1))
* **ratelimit:** detect Claude rate limits and auto-resume sessions ([#53](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/53)) ([ff5119f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ff5119f3d5bce43ac44779bf7daf6da4a54b609e))
* **ratelimit:** push-based output notification + repository improvements ([a0660b9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a0660b9636c973d748d220e20b750556f32491b7))
* **rules:** close coverage gaps found in approval analytics ([68602a4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/68602a4cab7b5f857d78e49fef5a3265f5adb03e))
* **session:** add ClearConversationState RPC + expand registry test coverage ([c7e9547](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c7e9547409c8d12e34bda6f9d4b6482bed5ad969))
* **session:** add New Project creation mode with git init ([#52](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/52)) ([e2e8964](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e2e89640ae5fef699ce4486fd4bdeb73315a30d6))
* **session:** persist review queue interaction state through restarts ([e22a4ca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e22a4ca4b821d7ca51847312d3a38ae8cc922c68))
* **ssq-hooks:** add claude install target and proper hook output format ([#45](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/45)) ([01d77ac](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/01d77ac93fa6481bab7c9117b6dba7e7ca0b39ed))
* **streaming:** WebSocket bridge for Watch* RPCs + global session context ([a374322](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a374322b9c7a9c618e8d9623dbc6ecf6f9268f27))
* **tmux:** priority CM sender for low-latency input forwarding ([93cfa92](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/93cfa924bced4a9442499463ed6112f4c73c3c11))
* **unfinished:** view diff modal + extract DiffRenderer as shared component ([47aa99f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/47aa99f8e94212aa2947934922e47b6fb7cb8ab7))


### Bug Fixes

* address PR review comments from Copilot ([d8aded5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d8aded55a3a98e4afd8dfdb6b27461b7e865e5ef))
* **adr-003:** eliminate all time.Sleep from test files; enforce with lint gate ([6c18a96](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6c18a965647c14796a74a5de10b8a96a48d79f50))
* **bench:** restore benchmark baselines from upstream-fanatics/main ([2aa57d7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2aa57d7d27398207519314c35c63e286ea70d296))
* **concurrency:** migrate mutexes to deadlock-detecting wrappers + zombie reaper refactor ([f8f012e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f8f012eb0f4f06a8bb722c7035c293b4b862bc31))
* **detection:** fix review queue misclassifying active sessions as idle ([#54](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/54)) ([d076dbe](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d076dbed01373a2488ecc914783e4caf09f69e24))
* **install:** build ssq-hooks to ~/.local/bin during make install ([3321fc5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3321fc5256673fa3805f2e36d81df2321638d881))
* **lint:** fix forbidigo violations in test watchdog helpers ([28645ac](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/28645aca7e0d6b18ef8ad01e14e9de933cd73932))
* **lint:** remove invalid _comment property from boundaries/dependencies rule ([3969fb4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3969fb4ac60e6556521a7d15ce9d593849acbb8f))
* **lint:** suppress gochecknoglobals on runtimeLevel atomic ([0058fad](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0058fad85a9498e230f64b76ecb9a63411afd3e5))
* **omnibar:** session name typing no longer resets one-off creation mode ([b1398cc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b1398cc0f0724a9d58efd4d52a45a10d47c76aff))
* **review:** address Copilot review comments ([6464477](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/64644771da87ea07f46e1afad16715654de0d38e))
* **review:** address remaining Copilot review comments ([9d25b4f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9d25b4fa93f96f8c74564246eb8006766e22e0e3))
* **server:** use poller cache for WatchSessions; block on PTY pause ([18cd90f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/18cd90f2bbae9d310b4b9493258b79288b81154c))
* **session:** reduce zombie accumulation via WaitDelay and HistoryLinker backoff ([46756ef](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46756ef2a412d024381f4ceb8bf4df26e14acb44))
* **streaming:** replace polling with push-based updates for VCS and notifications ([14960de](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/14960dec217262ae8378a3795662023e983eaa24))
* **test:** prevent tmux server socket leaks across test runs ([7485594](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7485594cd1466a96b590b687a53e6db90f0a55e7))
* tmux session creation reliability and review queue force-advance ([a3d8655](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a3d865505fb239d522896d94a40ae230ea6b850e))
* **tmux:** initialize priority channels in CM dispatch test helper ([f9da30f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f9da30f765cf9bc202b9d6a233cb41cb2f5268de))
* **tmux:** initialize priority channels in CM dispatch test helper ([a7ae705](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a7ae705be731801ec561bc769b4bca08541e894f))
* **tmux:** non-blocking resize and fire-and-forget input to cut CM round-trips ([f0716d2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f0716d2133a3fe0a95a5518b051bcc8bc238e19a))
* **ui:** auto-reload once on ChunkLoadError to recover stale build cache ([3fd8ba2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3fd8ba29aa9106b773f04e37244a4eb9f6d503af))
* **ui:** pre-populate worktree dropdown when repo is pre-selected ([784eac4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/784eac4caf254382caa6ce2f28334210e48cce91))
* **ui:** remove nav click handler that broke Next.js routing ([0cbeda8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0cbeda8f0b8b0a08e929ebd301ab2a83634bab09))
* **unfinished:** compare against remote tracking refs and add GetWorktreeDiff RPC ([dc605ad](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dc605adcb1d208db902fd4baeba82ac62ce84205))


### Performance Improvements

* eliminate mutex contention and allocation hotspots ([#94](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/94)) ([6f3b278](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6f3b2781f5420d425741cf390f97ad3e94d1ce65))

## [1.25.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.24.0...v1.25.0) (2026-05-01)


### Features

* **zombie:** set PR_SET_CHILD_SUBREAPER on Linux so tmux's zombies reparent to us ([d597fd6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d597fd653dbbc7e24625a7e86fd2615125a3e3ef))


### Bug Fixes

* **ui:** show GitHub info in VCS tab; fix action sheet z-index ([9b95d93](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9b95d93c8a185bcd2c058f54c7399d5dceb6efc8))
* **zombie:** filter to direct children only, add spawn registry for origin logging ([a7d8fe2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a7d8fe27d17d6caf1ca3c8e473fb4b106fbbb123))
