# Changelog

## [1.50.0](https://github.com/tstapler/stapler-squad/compare/v1.49.0...v1.50.0) (2026-08-30)


### Features

* **backlog:** PR deep links and import dedup with progress ([#663](https://github.com/tstapler/stapler-squad/issues/663)) ([ccabdf0](https://github.com/tstapler/stapler-squad/commit/ccabdf05da0128ac99d37405272cb0dd5441e8dd))
* **session:** configurable retry backoff for crashed/stalled sessions ([#664](https://github.com/tstapler/stapler-squad/issues/664)) ([b7eab44](https://github.com/tstapler/stapler-squad/commit/b7eab4476ea9604dca730e0dc032c3ac9410badb))


### Bug Fixes

* **backlog:** trap keyboard focus in modal dialogs ([#665](https://github.com/tstapler/stapler-squad/issues/665)) ([876c786](https://github.com/tstapler/stapler-squad/commit/876c7866b9c15311c89dc8dbc913ae794a2e3142))
* **web-app:** stop flow-control-stress.test.ts full-suite timeout flake ([#660](https://github.com/tstapler/stapler-squad/issues/660)) ([6046f8c](https://github.com/tstapler/stapler-squad/commit/6046f8c02fcbfc11d3d80331b8c966a79d7640fa))

## [1.49.0](https://github.com/tstapler/stapler-squad/compare/v1.48.0...v1.49.0) (2026-08-29)


### Features

* **board:** board/kanban view toggle for sessions ([#644](https://github.com/tstapler/stapler-squad/issues/644)) ([9643938](https://github.com/tstapler/stapler-squad/commit/9643938945b8745458dd41d38c5fa3763e762a20))
* **detection:** use OSC title spinner/idle glyphs as high-priority status signals ([#654](https://github.com/tstapler/stapler-squad/issues/654)) ([abc2973](https://github.com/tstapler/stapler-squad/commit/abc29735ff864cd39ebb8c7b7c1911f0c05671f1))
* **install-service:** persist operator env vars across redeploys ([#618](https://github.com/tstapler/stapler-squad/issues/618)) ([284c62f](https://github.com/tstapler/stapler-squad/commit/284c62fa876afa66608c9ec3b2b2cdf349675d28))
* **lint:** add new-code-only Go duplication gate (dupl) + advisory web jscpd ([b61ea13](https://github.com/tstapler/stapler-squad/commit/b61ea1355f3fed3360a1aa4ce47be4d35a98e5ca))
* **logs:** parse JSON log lines and add a Patterns view ([#630](https://github.com/tstapler/stapler-squad/issues/630)) ([630a4e1](https://github.com/tstapler/stapler-squad/commit/630a4e1f5cdf3cdc58b0ac002bc42b41bebc2799))
* **notifications:** add MemoryPressureNotifier for cgroup memory ceiling warnings ([2e404ab](https://github.com/tstapler/stapler-squad/commit/2e404abc7c07937c7a14b87ba79d2f8596109335))
* **otel:** add opt-in compile-time auto-instrumentation build (otelc) ([#607](https://github.com/tstapler/stapler-squad/issues/607)) ([9389462](https://github.com/tstapler/stapler-squad/commit/9389462438cbb554c81aea1f1d11702c747bf193))
* restart with a compressed handoff summary ([#612](https://github.com/tstapler/stapler-squad/issues/612)) ([ab40c38](https://github.com/tstapler/stapler-squad/commit/ab40c38cf5431a620bb690411d3c9c44c27a6bbd))
* **review-queue:** add exclude filters, search, and responsive layout ([#598](https://github.com/tstapler/stapler-squad/issues/598)) ([6836ebf](https://github.com/tstapler/stapler-squad/commit/6836ebf14c55b1f03098a84d5a6eabbadcfbf97e))
* **services:** steer active sessions with PR-fix context instead of skipping respawn ([#645](https://github.com/tstapler/stapler-squad/issues/645)) ([bab20ea](https://github.com/tstapler/stapler-squad/commit/bab20ea530910908a96c10aa1a9b112d179f0776))
* **streamhub:** add a live global override toggle, no restart required ([b67f7f4](https://github.com/tstapler/stapler-squad/commit/b67f7f40fae3a3536f06cbad93eb6084ee7b3d99))
* **streamhub:** flip global default to on, matching control-mode convention ([4acdc6b](https://github.com/tstapler/stapler-squad/commit/4acdc6bf7d1265e2d35abf3781b76f25ee02d69a))
* **telemetry:** export cgroup memory metrics via OpenTelemetry ([874c6a5](https://github.com/tstapler/stapler-squad/commit/874c6a51a594308e0507c418b89a029db95f008f))
* **terminal:** default terminal:resync-exec-gate-fast-lane to on ([8ccec1a](https://github.com/tstapler/stapler-squad/commit/8ccec1a78b60e4e8731dadea805d2029b33895fe))
* **terminal:** watch the event bus for instance-started instead of polling ([dcfc9ed](https://github.com/tstapler/stapler-squad/commit/dcfc9ed9c419fa572d2d0c1520c81dc9ef6a876f))
* **tymux:** bundle, supervise, and make selectable the tymux backend ([#635](https://github.com/tstapler/stapler-squad/issues/635)) ([17b81a6](https://github.com/tstapler/stapler-squad/commit/17b81a64ec156d331eb195ef56c3f14972adb322))
* **vcs-widget:** redesign session VCS tab with commit history, checks, and reviews ([#649](https://github.com/tstapler/stapler-squad/issues/649)) ([7a95862](https://github.com/tstapler/stapler-squad/commit/7a9586221ac02f02f74a8e4b29b9d69ec767d4ab))
* **webhooks:** react to check_run/workflow_run/pull_request_review/issue_comment events ([#628](https://github.com/tstapler/stapler-squad/issues/628)) ([15702ae](https://github.com/tstapler/stapler-squad/commit/15702ae95d8383a2bb75fe172d420bb803456890))


### Bug Fixes

* **auth,executor:** dynamic WebAuthn RPID registration + Wait() race fix ([#626](https://github.com/tstapler/stapler-squad/issues/626)) ([03f31de](https://github.com/tstapler/stapler-squad/commit/03f31de50d4b8a04ea638ad0c17c3a499d01cc7f))
* **auth:** harden WebAuthn rpID trust model ([b18a689](https://github.com/tstapler/stapler-squad/commit/b18a6899559478b2502ce972c45e92a880196507))
* **auth:** harden WebAuthn rpID trust model ([e4c7cbc](https://github.com/tstapler/stapler-squad/commit/e4c7cbc582996a3778790d379cff5a489d455a27))
* **backlog:** restore focus to trigger element on modal close ([#641](https://github.com/tstapler/stapler-squad/issues/641)) ([63e283c](https://github.com/tstapler/stapler-squad/commit/63e283c65d9c4838be5b8ee06eff7f52feb45fb9))
* **ci:** stop required checks from permanently blocking path-irrelevant PRs ([#619](https://github.com/tstapler/stapler-squad/issues/619)) ([53655ed](https://github.com/tstapler/stapler-squad/commit/53655ed2552b410e0817144156a3ffa2616680ab))
* **ci:** stop the ESLint step failing on every direct push to main ([b5a200e](https://github.com/tstapler/stapler-squad/commit/b5a200ea6b9fa0a3acfad1e3c82a9c99b38c9e4b))
* **config:** prune orphaned test-&lt;pid&gt; dirs left by go test runs ([06140aa](https://github.com/tstapler/stapler-squad/commit/06140aa64422eaeb8f1edaf1ee7d4f71a73f47dd))
* **github:** derive omnibar enterprise-host detection from persisted keychain, not live cache ([4d2c16d](https://github.com/tstapler/stapler-squad/commit/4d2c16ddfd4435c470bdbcb5e1427b0a6c99be18))
* **log:** reduce log volume and add per-package log levels ([#627](https://github.com/tstapler/stapler-squad/issues/627)) ([495988f](https://github.com/tstapler/stapler-squad/commit/495988f29f25afd9716dd86380cf9d832ccbab2a))
* **log:** scope GetConfigDir to STAPLER_SQUAD_INSTANCE like config package ([#655](https://github.com/tstapler/stapler-squad/issues/655)) ([abb71b9](https://github.com/tstapler/stapler-squad/commit/abb71b99ddb80b87cd60056d326af92df29adea7))
* **perf:** stop background sweeps from pegging a CPU core and causing input lag ([#620](https://github.com/tstapler/stapler-squad/issues/620)) ([a0d5d5d](https://github.com/tstapler/stapler-squad/commit/a0d5d5dfb9c729d2734d1c22a621d6784bdaa11c))
* **perf:** stop gitignore-matcher rebuild on every commit ([#622](https://github.com/tstapler/stapler-squad/issues/622)) ([1c88409](https://github.com/tstapler/stapler-squad/commit/1c88409daf5f5ffd39fe3190c026fd03ebec606f))
* **registry:** consolidate proto enumeration in scanner tooling, wire headless.proto ([#642](https://github.com/tstapler/stapler-squad/issues/642)) ([b2dc926](https://github.com/tstapler/stapler-squad/commit/b2dc926185d31957c422d25f4fa23e7860255c02))
* **registry:** glob proto enumeration in scanner tooling, wire headless.proto ([#636](https://github.com/tstapler/stapler-squad/issues/636)) ([5ed55c3](https://github.com/tstapler/stapler-squad/commit/5ed55c383d4ab5964890a41b502f149758b4a995))
* **review-gate:** compute diff/base against item's own worktree, not shared repoPath ([#634](https://github.com/tstapler/stapler-squad/issues/634)) ([3a10d4d](https://github.com/tstapler/stapler-squad/commit/3a10d4d28d3f4e7e931788d9984f537e7fc0a866))
* **review-queue:** stack action buttons on mobile, add bulk skip ([#656](https://github.com/tstapler/stapler-squad/issues/656)) ([48403e8](https://github.com/tstapler/stapler-squad/commit/48403e8cd9d9d91b6bced1de9a778fd23a12ea8a))
* **server,session,config:** close fixed-tmp-filename write races surfaced by flaky-test triage ([#653](https://github.com/tstapler/stapler-squad/issues/653)) ([503116c](https://github.com/tstapler/stapler-squad/commit/503116c70fe7a521471bb6b79bfae1efa36a8fce))
* **server:** thread request ctx through handlers that discarded it for context.Background() ([e62d24b](https://github.com/tstapler/stapler-squad/commit/e62d24b19632494e27cef2bd233720c67bebae17))
* **service:** raise cgroup MemoryHigh/MemoryMax from 60%/80% to 80%/90% ([2af14ab](https://github.com/tstapler/stapler-squad/commit/2af14ab4f4e00122132fe59212b2765111f6e3e3))
* **session/git:** replace stderr matching with ground-truth re-query in worktree self-heal ([#615](https://github.com/tstapler/stapler-squad/issues/615)) ([ee17162](https://github.com/tstapler/stapler-squad/commit/ee17162ce82cf4fa53b69121895111b4d67ebaed))
* **session:** archive sessions not resident in the live poller ([#623](https://github.com/tstapler/stapler-squad/issues/623)) ([cd8ab5f](https://github.com/tstapler/stapler-squad/commit/cd8ab5f1bf62a02fba45035d276e3f77a078c093))
* **session:** close session-completion-summary e2e flake (grace period, exit race, UpdatedAt) ([#603](https://github.com/tstapler/stapler-squad/issues/603)) ([beee6d0](https://github.com/tstapler/stapler-squad/commit/beee6d0f2fb14086e137307f7726fc17aaf00b14))
* **session:** let report_duplicate claims reach a real reviewer instead of auto-FAIL ([#625](https://github.com/tstapler/stapler-squad/issues/625)) ([14c5abb](https://github.com/tstapler/stapler-squad/commit/14c5abb7a5e9a9ee5ccb3f72563acb82ae7e4400))
* **streamhub:** bound resize I/O with a caller-derived, cancelable deadline ([26cd3c4](https://github.com/tstapler/stapler-squad/commit/26cd3c407ed0d1593af9c0df13060565b73ef317))
* **streamhub:** bound withCursorSync's cursor lookup, fix a leak it exposed ([8c7b668](https://github.com/tstapler/stapler-squad/commit/8c7b668aa8ed150127a8f49ede1c8c8afa0c27de))
* **streamhub:** don't tear down the hub when a session is still cold-starting ([37161b5](https://github.com/tstapler/stapler-squad/commit/37161b575cb41b11ccde9bc64f7c4fa7451093f6))
* **streamhub:** restart the output pump when a torn-down hub reactivates ([9c604da](https://github.com/tstapler/stapler-squad/commit/9c604daccbe65dbf1bf165fa529002f16d443b76))
* **streamhub:** resubscribe to control-mode output after a mid-session crash/restart ([382c8e1](https://github.com/tstapler/stapler-squad/commit/382c8e1e46995fae39afc85ba8ea67a7601107ff))
* **streamhub:** stop broadcasting unprepared terminal snapshots on resize ([d2b06c3](https://github.com/tstapler/stapler-squad/commit/d2b06c31823c303f11e0644c252f1acb5b5d94cc))
* **telemetry:** resolve semconv schema-URL conflict in resource.Merge ([#651](https://github.com/tstapler/stapler-squad/issues/651)) ([c213548](https://github.com/tstapler/stapler-squad/commit/c2135485e3872f9651d711455f1976bd87f50672))
* **telemetry:** stop OTLP metrics export exceeding gRPC max message size ([#646](https://github.com/tstapler/stapler-squad/issues/646)) ([fc7b5fc](https://github.com/tstapler/stapler-squad/commit/fc7b5fcc84d97f137bd6da217dc269c26d62e5e8))
* **terminal:** bound resync fast-lane subprocess calls to under the client's watchdog ([eec06df](https://github.com/tstapler/stapler-squad/commit/eec06dff5f75f9a380ec02844dbacc9ca4593bae))
* **terminal:** send initial snapshot even when the pane is genuinely empty ([c70f641](https://github.com/tstapler/stapler-squad/commit/c70f641a7e48e516bc3c928eddfd8e3b1e6404dd))
* **terminal:** stop silently dropping mid-stream resync write errors ([d362a6c](https://github.com/tstapler/stapler-squad/commit/d362a6c476da0ec6add1b3ea944cafad67ca9273))
* **terminal:** wait for a concurrently-starting instance before attaching a hub ([aeae264](https://github.com/tstapler/stapler-squad/commit/aeae2648b8ba42f5872a552accc12f3a15a458b6))
* **tests:** resolve flaky test failures across services, session, and headless packages ([#595](https://github.com/tstapler/stapler-squad/issues/595)) ([8f96a44](https://github.com/tstapler/stapler-squad/commit/8f96a44abc07602aa6a3aa1bdbc81b69fde82d4b))
* **test:** use short temp dir for AF_UNIX socket paths on macOS ([#617](https://github.com/tstapler/stapler-squad/issues/617)) ([a09f002](https://github.com/tstapler/stapler-squad/commit/a09f0029d008fc792b14fc221de91465df9e55e6))
* **web-app:** memoize useGitHubIssuePicker's localRepos selector ([e46622a](https://github.com/tstapler/stapler-squad/commit/e46622a75cfb741d18858d78debf463d2cc953c3))
* **web-app:** send Connect-Timeout-Ms on every RPC through the shared transport ([62058e7](https://github.com/tstapler/stapler-squad/commit/62058e7114bb8b9aede5840adbb8e42ddd646d05))
* **webauthn:** don't fall back to a bare IP when all boot-time hostnames fail DNS re-verification ([22d24b1](https://github.com/tstapler/stapler-squad/commit/22d24b1e26161dd9c10369dbc1a920e2fc5575cb))

## [1.48.0](https://github.com/tstapler/stapler-squad/compare/v1.47.0...v1.48.0) (2026-08-23)


### Features

* **session/tymux:** add reconnect loop with backoff, resync, and daemon-restart detection (Epic 2.5) ([46ac1d7](https://github.com/tstapler/stapler-squad/commit/46ac1d71f82e1243234e6a42b8dce03509ee7af1))
* **session/tymux:** implement CellSGRRenderer for CapturePaneContent (Epic 2.6) ([eb82dfc](https://github.com/tstapler/stapler-squad/commit/eb82dfcf97394e5a70a07e7235f5be09f80d89ea))
* **session/tymux:** implement lifecycle, capture, and introspection methods (Epic 2.2) ([984609d](https://github.com/tstapler/stapler-squad/commit/984609d5d21066d28a28abb9e75ac5904509e3b5))
* **session/tymux:** open standing Attach stream with client-side fan-out (Epic 2.3) ([e68c4d4](https://github.com/tstapler/stapler-squad/commit/e68c4d4e922d3796cd622ca2c484dd8e29cca4a1))
* **session/tymux:** wire resize and fire-once exit-status callback (Epic 2.4) ([2ec2244](https://github.com/tstapler/stapler-squad/commit/2ec224445da802323b55b8f13918eca148e9322c))
* **session:** add TymuxBackend skeleton with per-session backend selection (Epic 2.1) ([acbbd57](https://github.com/tstapler/stapler-squad/commit/acbbd57273d19d8b5271372bf92666d6d160e01b))
* **web-app:** adopt ConnectRPC-native streaming for WatchSessions/WatchReviewQueue on TLS listener ([#605](https://github.com/tstapler/stapler-squad/issues/605)) ([9316ad9](https://github.com/tstapler/stapler-squad/commit/9316ad99893ff86856d2409bde499e8003ba0281))
* **web-app:** wire stream-hub rollout controls into the Feature Flags page ([19f92f5](https://github.com/tstapler/stapler-squad/commit/19f92f51f7d3dd73f22305ef812af83ccf47a5da))


### Bug Fixes

* **ci:** replace broken tymux sibling-checkout with plain git clone ([#604](https://github.com/tstapler/stapler-squad/issues/604)) ([deda08c](https://github.com/tstapler/stapler-squad/commit/deda08c594ae3cb34381c2964e532f3071bb220a))
* **ci:** use git clone instead of actions/checkout for the tymux sibling directory (Phase 6 fixes) ([b0a5387](https://github.com/tstapler/stapler-squad/commit/b0a53877a5269e6332f6ef92112ea111fc725d1e))
* **lint:** fix depguard files: glob bug hiding no_server_in_core/no_ent_in_services ([#608](https://github.com/tstapler/stapler-squad/issues/608)) ([a624930](https://github.com/tstapler/stapler-squad/commit/a624930879ba47730c1d6feccbbffc082e9645b1))
* **log:** stop defaulting to DEBUG on every boot, wire slog to the runtime level ([8414b42](https://github.com/tstapler/stapler-squad/commit/8414b4252de90dd9169bce0b77a3ad874239c252))
* **session/tymux:** close Phase 5 spec-compliance gaps ([f498c6b](https://github.com/tstapler/stapler-squad/commit/f498c6b21e80129f5d9ab84d27e596afb51acfa6))
* **session/tymux:** stop misclassifying clean exit as daemon restart, make reconnect-path calls interruptible (Phase 6 fixes) ([477eacd](https://github.com/tstapler/stapler-squad/commit/477eacd6af5a6f8f20b8ba00e41244732e0c84bc))
* **session/tymux:** thread context cancellation and preserve unreachable-vs-dead distinction in IsAlive (Phase 6 fixes) ([7cd632a](https://github.com/tstapler/stapler-squad/commit/7cd632a1ef6293b61ba52d8f3d1d0aeb3e02ce7a))
* **session:** bump UpdatedAt on status transitions so the frontend sees them ([#601](https://github.com/tstapler/stapler-squad/issues/601)) ([704f9c1](https://github.com/tstapler/stapler-squad/commit/704f9c1e879cc0b5ed69e6fa463fc77260405c20))
* **session:** durable signal + badge for lost-history cold restore ([#609](https://github.com/tstapler/stapler-squad/issues/609)) ([360d845](https://github.com/tstapler/stapler-squad/commit/360d845a0f813886aec2bdaf799186b0f6b1acb1))
* **session:** resolve worktree-path RepoPath to the main repo before anchoring ([1be9d69](https://github.com/tstapler/stapler-squad/commit/1be9d69104fe616e860ed8883d3ba358bc640699))
* **session:** stop backlog worktrees from silently branching off ambient HEAD ([43be584](https://github.com/tstapler/stapler-squad/commit/43be58416e3dcad5ed2b402e15cc54fcecef8930))
* **tmux:** eliminate goroutine leak from uncancellable capture-pane calls ([#597](https://github.com/tstapler/stapler-squad/issues/597)) ([47083ea](https://github.com/tstapler/stapler-squad/commit/47083ea7abe6df646afb72433be2261d033df98d))
* **web-app:** make /settings/features scroll on mobile ([aeeda53](https://github.com/tstapler/stapler-squad/commit/aeeda53aab47a364373349159d0c56cb9559b740))
* **web-app:** show linked backlog item on every session tab, scope workspace peers to literal path ([ab511c4](https://github.com/tstapler/stapler-squad/commit/ab511c42b07ef358f05dce33f7ad8cd8aa1be83f))

## [1.47.0](https://github.com/tstapler/stapler-squad/compare/v1.46.0...v1.47.0) (2026-08-22)


### Features

* **backlog:** add --list-known-hosts CLI command and registry observability logging ([d03cea4](https://github.com/tstapler/stapler-squad/commit/d03cea4a111d2b2dcc4459918f9936c9f45e0863))
* **backlog:** add gossip-based Workspace Host Registry (ADR-002) ([22299f7](https://github.com/tstapler/stapler-squad/commit/22299f702a0f136f5c0cd0100a8feeaf81f30ad8))
* **backlog:** add Linux ssq:// OS scheme registration and --open-url ([f762236](https://github.com/tstapler/stapler-squad/commit/f762236f3c1daa6a71f6ab0a2159c984c49bed0d))
* **backlog:** add ssq:// deep-link parsing and resolve UI surfaces ([d41af92](https://github.com/tstapler/stapler-squad/commit/d41af92c834fb1edc85fc0bb19c65e4887432473))
* **backlog:** add type-prefixed ULID BacklogItemID with additive migration ([3bb1ede](https://github.com/tstapler/stapler-squad/commit/3bb1ede74272dc9cf104566b4f97481e8e68ffa8))
* **headless:** require an explicit cost sink on every CallBlocking call ([#587](https://github.com/tstapler/stapler-squad/issues/587)) ([c14bf5e](https://github.com/tstapler/stapler-squad/commit/c14bf5e02a8708f3386bc873387f6383232b08b3))
* **mcp:** allow report_duplicate to archive unclaimed backlog items directly ([#589](https://github.com/tstapler/stapler-squad/issues/589)) ([bef2d40](https://github.com/tstapler/stapler-squad/commit/bef2d408df559176d57ce7ecf1b0d7b50506e0b5))
* **mcp:** link_session_to_item and get_linked_item backlog tools ([#586](https://github.com/tstapler/stapler-squad/issues/586)) ([06a9582](https://github.com/tstapler/stapler-squad/commit/06a95828f32afb3b0b91e9d805b90dbfcf7a2d71))
* one-click PR creation from session diff ([#580](https://github.com/tstapler/stapler-squad/issues/580)) ([59fa980](https://github.com/tstapler/stapler-squad/commit/59fa9806a39200225d47f921178519a621245ccd))
* **rules:** dynamic claude-settings rule reload without restart ([#538](https://github.com/tstapler/stapler-squad/issues/538)) ([4f40302](https://github.com/tstapler/stapler-squad/commit/4f40302eb50b188e22784248c52e9e243c2d8b82))
* **ssh-remote-workspaces:** SSH remote workspace support ([#571](https://github.com/tstapler/stapler-squad/issues/571)) ([ff4830c](https://github.com/tstapler/stapler-squad/commit/ff4830cbda0d82134a63bbf612eca3d6d02abd88))
* **streamhub:** add batch window and cross-subscriber sequencing ([c4fa026](https://github.com/tstapler/stapler-squad/commit/c4fa026b8d3c678825a2ecf8d2ae387c5383eb39))
* **streamhub:** add core types and Transport interface ([3818c35](https://github.com/tstapler/stapler-squad/commit/3818c35e6ef27b5ced5d9907a15ec37ab5ffc8be))
* **streamhub:** add in-memory transport and failure-mode test suite ([12664c0](https://github.com/tstapler/stapler-squad/commit/12664c0fd07fd1fe4a10a7d3ba9c164245450f4e))
* **streamhub:** add resize negotiation and owned capture pipeline ([40e20db](https://github.com/tstapler/stapler-squad/commit/40e20db8fa8d4d3bed56def7330f32859e1476e5))
* **streamhub:** add rollout mechanics and rollback rehearsal gate ([2c786ea](https://github.com/tstapler/stapler-squad/commit/2c786ea4150313dd999ac9607f1c681339c19126))
* **streamhub:** add sticky per-session StreamPath resolution ([bbb69a8](https://github.com/tstapler/stapler-squad/commit/bbb69a83b69f2e54dd92e05db5f5afd146916e00))
* **streamhub:** add structured observability and OverlapInvariant ([b8f1b3c](https://github.com/tstapler/stapler-squad/commit/b8f1b3c6a501c5c77f0d34f4baf62b7ca2275450))
* **streamhub:** add subscriber lifecycle, fan-out, and hub teardown ([dde5c5a](https://github.com/tstapler/stapler-squad/commit/dde5c5a1606fa095c0621824b945071956939568))
* **streaming:** add connection-count indicator UI ([00785f1](https://github.com/tstapler/stapler-squad/commit/00785f1b8eaa7a734b3d55994561b915f696d104))
* **streaming:** add ExternalTmuxStreamerTransport for the real ssq-mux path ([0ee2f58](https://github.com/tstapler/stapler-squad/commit/0ee2f58d0cc70ca9a79679668c074bfed34a324a))
* **streaming:** add ssq-mux output-only Transport adapter ([e8172fb](https://github.com/tstapler/stapler-squad/commit/e8172fbe08c90a69c6aacaf90a54d73e0291e6f0))
* **streaming:** add WebSocketTransport and hub-owned path (flagged off) ([a04075c](https://github.com/tstapler/stapler-squad/commit/a04075cee9e321fd39979345e8b7d754d4e8e7f7))
* **streaming:** attach ssq-mux MuxTransport to hub-owned sessions ([604a69b](https://github.com/tstapler/stapler-squad/commit/604a69b2b50bf4a3218807532c245aa2d3642795))


### Bug Fixes

* **a11y:** darken clean theme primary token to meet WCAG AA contrast ([#584](https://github.com/tstapler/stapler-squad/issues/584)) ([d41f3b0](https://github.com/tstapler/stapler-squad/commit/d41f3b06241e135234f831538b8402cb6587d436))
* **backlog:** add cold-retry heartbeat so parked remediation rows self-heal ([#572](https://github.com/tstapler/stapler-squad/issues/572)) ([70558c6](https://github.com/tstapler/stapler-squad/commit/70558c69b6cd07f4c96616bebaa4d96e6ae0d16c))
* **backlog:** close SSRF and query-injection gaps in deep-link handoff ([3aa6732](https://github.com/tstapler/stapler-squad/commit/3aa6732871075c96182be94ed87d6a44f426c0a5))
* **backlog:** dial peers over HTTPS with a scheme, fixing non-functional gossip/liveness ([7ae758a](https://github.com/tstapler/stapler-squad/commit/7ae758a069314c688d8300799d1e4ea62f9b60bc))
* **backlog:** expose public_id on BacklogItemSummary, fixing stale UUID display ([b1da171](https://github.com/tstapler/stapler-squad/commit/b1da171065abfb02f90746afd59f1c4a8e301bcf))
* **backlog:** populate allowedTransitions in ListBacklogItems DTO ([#585](https://github.com/tstapler/stapler-squad/issues/585)) ([f3945ed](https://github.com/tstapler/stapler-squad/commit/f3945ed4135b87eef9ca72f711ced44b1077ee1e))
* **backlog:** wire up registry-backed deep-link host resolver (Story 3.3) ([ec33700](https://github.com/tstapler/stapler-squad/commit/ec33700432634447883e3ddc7fffc20b94ea0c99))
* **fmt:** gofmt two files left unformatted by recent backlog changes ([#567](https://github.com/tstapler/stapler-squad/issues/567)) ([953d8c4](https://github.com/tstapler/stapler-squad/commit/953d8c4a3dae11d299752b5f60d1a4ca6e6efe65))
* **github:** distinguish no-token from rate-limit-exhausted errors ([#575](https://github.com/tstapler/stapler-squad/issues/575)) ([10c055b](https://github.com/tstapler/stapler-squad/commit/10c055b8f4b03e1d5e591d27c81adadc44337006))
* **gogitstore:** use CommandContextPG for fuzz-seed git subprocess ([#590](https://github.com/tstapler/stapler-squad/issues/590)) ([63cb9b3](https://github.com/tstapler/stapler-squad/commit/63cb9b39739ff5bd9292e0adf6806b07d4467ea2))
* **health:** avoid redundant IsAlive() call in checkSingleSession ([#592](https://github.com/tstapler/stapler-squad/issues/592)) ([be171b4](https://github.com/tstapler/stapler-squad/commit/be171b4957c98b97d96f407cd81a6c9ff572dd78))
* **log:** eliminate data race on global logger swap ([#576](https://github.com/tstapler/stapler-squad/issues/576)) ([320bd5f](https://github.com/tstapler/stapler-squad/commit/320bd5fde071e5769a83bbeee164265d927b332e))
* **mcp:** call log.InfoLog() instead of referencing it as a value ([4853ae0](https://github.com/tstapler/stapler-squad/commit/4853ae0c4a09f534b62fba9e9f165026f5e9c905))
* **mcp:** disambiguate ITEM_NOT_FOUND vs PERMISSION_DENIED for backlog link checks ([#581](https://github.com/tstapler/stapler-squad/issues/581)) ([f9bf813](https://github.com/tstapler/stapler-squad/commit/f9bf81359805a8122fec050a3d04dd7c1f2debc1))
* **mux:** close scheduling races in flaky mux tests ([#596](https://github.com/tstapler/stapler-squad/issues/596)) ([98be5bf](https://github.com/tstapler/stapler-squad/commit/98be5bf4c0f6ac3faf3eb48a10a89e0f68d11fbb))
* **server:** isolate history dir scanning from real ~/.claude/projects under test ([#555](https://github.com/tstapler/stapler-squad/issues/555)) ([10910c0](https://github.com/tstapler/stapler-squad/commit/10910c0acfe296d6f90f3c6bc6e0e0e760fe32c0))
* **services:** raise ListWorktrees timeout to absorb host-contention scheduling delay ([eb3c5ae](https://github.com/tstapler/stapler-squad/commit/eb3c5ae6ca78d29c6042356772fb26404826c6ea))
* **session:** sandbox HOME in dialog-gave-up test to stop real-disk-walk stall ([b9d1a2a](https://github.com/tstapler/stapler-squad/commit/b9d1a2a70669d57c96cfa16e3ac5e6e2179b6de4))
* **session:** skip tmux alive-probe for archived Stopped sessions ([c08513d](https://github.com/tstapler/stapler-squad/commit/c08513d479f28d552eed590a1000ea156b339b23))
* **streamhub:** close ownership-lock gap at StartControlMode, wire hub-start-failure banner ([4eec483](https://github.com/tstapler/stapler-squad/commit/4eec4830b1d1137102bff8eebe6ddc582aa140a2))
* **streamhub:** enforce mutual exclusion between hub and legacy paths ([2a62e58](https://github.com/tstapler/stapler-squad/commit/2a62e5847329b351ba9a1a27a81f7a93240b239d))
* **streamhub:** flush BatchWindow opportunistically from the production pump ([e41c9d5](https://github.com/tstapler/stapler-squad/commit/e41c9d57e2e9cd143bc3270193d9844cf2866838))
* **streamhub:** send CatchUpSnapshot from AttachSubscriber itself ([5437f7f](https://github.com/tstapler/stapler-squad/commit/5437f7fc9e03c3ddb7e1327872f46ae98d76945a))
* **streamhub:** serialize RequestResize negotiate+apply to stop concurrent applyNegotiatedSize runs ([f87941f](https://github.com/tstapler/stapler-squad/commit/f87941f48e7476be880f138d197437d09b88346e))
* **test:** eliminate flaky TestApprovalHandler global-mutation race ([e022630](https://github.com/tstapler/stapler-squad/commit/e022630f87388b1a4bc36f8c7eabe3a0c30ab920))
* **test:** eliminate flaky-test races (backlog hooks, tmux sockets, log redirection) ([#565](https://github.com/tstapler/stapler-squad/issues/565)) ([08d8eda](https://github.com/tstapler/stapler-squad/commit/08d8edaefd951ea76eaabca98369173bda095a36))
* **test:** eliminate remaining flaky-test races and a real timeout-classification bug ([#568](https://github.com/tstapler/stapler-squad/issues/568)) ([ffa8c94](https://github.com/tstapler/stapler-squad/commit/ffa8c9410c7d5a0b80c3a432c6d4cf4d8dbe0db0))
* **test:** scope tmux-forking package parallelism to fix flaky session tests under load ([#577](https://github.com/tstapler/stapler-squad/issues/577)) ([9a1813d](https://github.com/tstapler/stapler-squad/commit/9a1813d1566f44fb3a44950e91a166219b3d732d))
* **test:** widen e2e timeouts for connection-count-indicator flake ([63243e0](https://github.com/tstapler/stapler-squad/commit/63243e0faae7de6c29584e0d03a1dae4894d71b6))
* **tmux:** batch health-check pane status, close ADR-001/002 ([#588](https://github.com/tstapler/stapler-squad/issues/588)) ([ff7e7ae](https://github.com/tstapler/stapler-squad/commit/ff7e7ae75b9e66ef54baaaee6932aff93a43c368))
* **tmux:** eliminate AttachToExisting/RestoreWithWorkDir PTY-triple TOCTOU ([#583](https://github.com/tstapler/stapler-squad/issues/583)) ([192fead](https://github.com/tstapler/stapler-squad/commit/192fead0a298104686f692902c83dbe94cbf3935))
* **tmux:** give keystroke input its own exec-gate fast lane ([2ce4043](https://github.com/tstapler/stapler-squad/commit/2ce40432b07645b2f7fd5719cab3e64e189801f9))
* **web-app:** announce departure before unmounting ConnectionCountIndicator ([7f86a97](https://github.com/tstapler/stapler-squad/commit/7f86a974b9f34bef2d9643a7fefd985cae429b0c))


### Performance Improvements

* **scanbuf:** shrink pooled scan buffer default from 10MB to 64KB ([31ffc80](https://github.com/tstapler/stapler-squad/commit/31ffc8064151a00c5bf501b03f23edead54d165e))
* shared webpack cache + CSR-flattened search postings ([#562](https://github.com/tstapler/stapler-squad/issues/562)) ([c018b91](https://github.com/tstapler/stapler-squad/commit/c018b918b49ecee8caf269c72dd0f9e510a51eee))
* **tmux:** registry-gated fast path for DoesSessionExistNoCache ([#564](https://github.com/tstapler/stapler-squad/issues/564)) ([4bb0473](https://github.com/tstapler/stapler-squad/commit/4bb04738904119ee3bb962bf6a5f499208e6c173))

## [1.46.0](https://github.com/tstapler/stapler-squad/compare/v1.45.1...v1.46.0) (2026-08-20)


### Features

* **backlog:** add ungated activity log for backlog items ([#552](https://github.com/tstapler/stapler-squad/issues/552)) ([2297801](https://github.com/tstapler/stapler-squad/commit/2297801a8cb10b7bfe8f16cecde3ccbcfab65d8f))


### Bug Fixes

* **backlog:** improve headless review verdict parsing and no-diff schema example ([79239ec](https://github.com/tstapler/stapler-squad/commit/79239ecb58f023bbd30ea94b0f307842b651436d))
* **config,session:** close LoadConfig TOCTOU race and SessionDriver orphan-goroutine race ([#548](https://github.com/tstapler/stapler-squad/issues/548)) ([a32a01d](https://github.com/tstapler/stapler-squad/commit/a32a01d5d95652de0bde2442b38e24d231471420))
* **test:** dedupe in-memory-sqlite test helper, harden tmux subprocess flakes, fix wedged-git-subprocess timeout ([#559](https://github.com/tstapler/stapler-squad/issues/559)) ([6a1fccf](https://github.com/tstapler/stapler-squad/commit/6a1fccf2422ca8359d30050a6e61baa81f1536a0))
* **test:** resolve symlinks before comparing worktree path in TestWorkspace_UsesWorktreePath_WhenPresentOnDisk ([79ee345](https://github.com/tstapler/stapler-squad/commit/79ee3456e0d76e71556e3dc74b45e62b7ff2aa15))

## [1.45.1](https://github.com/tstapler/stapler-squad/compare/v1.45.0...v1.45.1) (2026-08-19)


### Bug Fixes

* **backlog:** recognize GitHub Enterprise URLs in report_pr_created/import_github_issue/report_duplicate ([5c3015e](https://github.com/tstapler/stapler-squad/commit/5c3015ef8919abfcd96266cbe60f829c61597106))
* **backlog:** sanitize LLM-controlled triage title against path traversal (security) ([#534](https://github.com/tstapler/stapler-squad/issues/534)) ([7b9aee4](https://github.com/tstapler/stapler-squad/commit/7b9aee4cd7b2bd463450b6ad79101cb2a008f987))
* **backlog:** strip ConnectRPC [code] prefix from error messages ([#549](https://github.com/tstapler/stapler-squad/issues/549)) ([b5acba7](https://github.com/tstapler/stapler-squad/commit/b5acba7a89e7487532997c3f228c1600c212521b))
* bound cmd.Wait() independently in gogitstore crash-subprocess harness ([#547](https://github.com/tstapler/stapler-squad/issues/547)) ([8f6cf22](https://github.com/tstapler/stapler-squad/commit/8f6cf22442473679d08d3ab05f99d7c1dd614cc8))
* **build:** add missing ent-gen/web-dist deps to lint, add make ready target ([7795ca7](https://github.com/tstapler/stapler-squad/commit/7795ca7a2e91532487ff632045bcbb4cefc85ee6))
* **concurrency:** serialize ent/atlas schema creation across independent clients ([702c86c](https://github.com/tstapler/stapler-squad/commit/702c86c017260e08201df822d6509e7847e0ef41))
* **deploy:** fix health-check timeout and rollback path bugs hit in a live incident ([88e819f](https://github.com/tstapler/stapler-squad/commit/88e819f140a0f8de6f30ba3988e0d0ecb83b0280))
* **github:** isolate session package's rate-limit test fixtures from the shared DefaultRateLimiter global ([#550](https://github.com/tstapler/stapler-squad/issues/550)) ([0b50326](https://github.com/tstapler/stapler-squad/commit/0b50326c5cd73b7a43d65717e595bb1c1c1b0905))
* **omnibar:** refetch GitHub Enterprise hosts when the omnibar opens ([7a52633](https://github.com/tstapler/stapler-squad/commit/7a5263301c0a013849b9a3f0cc6e56dbefba8548))
* **omnibar:** register GitHubEnterpriseURLDetector synchronously ([8c22835](https://github.com/tstapler/stapler-squad/commit/8c2283504d6e31155230f543b6da578d41ee8b69))
* **terminal:** prevent live-forward writes from racing resize/pane-capture snapshots ([77ebfc5](https://github.com/tstapler/stapler-squad/commit/77ebfc54362f72d5b16a52694ed709e90d5373c8))
* **terminal:** wait for real quiescence on control-mode reconnect ([d394352](https://github.com/tstapler/stapler-squad/commit/d3943524ba8dcfb962391591d00da18f855bb6e6))
* **test:** make goleak elision workaround tests deterministic ([#542](https://github.com/tstapler/stapler-squad/issues/542)) ([0297da8](https://github.com/tstapler/stapler-squad/commit/0297da8c0d1d6ae13dda2065fb28b88f2d919ece))
* **test:** root-cause and fix rotating cross-test flakes from t.Parallel() rollout ([30c6ce9](https://github.com/tstapler/stapler-squad/commit/30c6ce91478cb58e5ecbfc493db37b318621ae96))
* **triage:** classify subprocess-start failures instead of swallowing them into "other" ([#535](https://github.com/tstapler/stapler-squad/issues/535)) ([3d2a760](https://github.com/tstapler/stapler-squad/commit/3d2a7600bd3f1560fc37636d06194b5d667add6a))

## [1.45.0](https://github.com/tstapler/stapler-squad/compare/v1.44.1...v1.45.0) (2026-08-17)


### Features

* **insights:** surface cache read/write tokens across dashboard ([8ac1cca](https://github.com/tstapler/stapler-squad/commit/8ac1cca57fb41d8df75a7d5ae9397e794de95f95))
* **session:** add SetCommitStatus and PR head SHA tracking ([db4e208](https://github.com/tstapler/stapler-squad/commit/db4e20869e807e5930a2e1555235bce9c8b92c49))


### Bug Fixes

* **github:** fail fast on known rate limits instead of retrying blind ([a6e747e](https://github.com/tstapler/stapler-squad/commit/a6e747ef24c5dad9ed3cc055bc69caf8e552edc2))
* **server:** join fork-pressure/zombie-watcher/zombie-reaper goroutines in Shutdown() ([#531](https://github.com/tstapler/stapler-squad/issues/531)) ([658a7e9](https://github.com/tstapler/stapler-squad/commit/658a7e9cd3bfb4b54b59f92ec90799009a92d87a))
* **sqlite:** migrate to pure-Go modernc.org/sqlite driver for CGO_ENABLED=0 releases ([#526](https://github.com/tstapler/stapler-squad/issues/526)) ([0212d33](https://github.com/tstapler/stapler-squad/commit/0212d33002d41bd846d44010e9a52b1e108816f5))
* **web-app:** batch pending writes with Promise.all in flow-control stress test ([#539](https://github.com/tstapler/stapler-squad/issues/539)) ([ed5f596](https://github.com/tstapler/stapler-squad/commit/ed5f596b970de4e90c0dae1e94329bea11186c90))


### Performance Improvements

* **web-app:** preserve session entity refs on unchanged snapshot data ([a38f54f](https://github.com/tstapler/stapler-squad/commit/a38f54f219fceaa5e388a2578e1e1117347b41e3))

## [1.44.1](https://github.com/tstapler/stapler-squad/compare/v1.44.0...v1.44.1) (2026-08-17)


### Bug Fixes

* **insights:** fix broken virtualization and zero-value Tokens columns ([8a8917d](https://github.com/tstapler/stapler-squad/commit/8a8917d14e2ff08d5aff3ccf73b522a4f7bcf291))
* **insights:** restore missing sortOrderHint CSS export broken by 8a8917d14 ([47c75c5](https://github.com/tstapler/stapler-squad/commit/47c75c511e2f2343b5ce0df79b87110c277a0bdb))
* **server:** join fork-pressure/zombie-watcher/zombie-reaper goroutines in Shutdown() ([#533](https://github.com/tstapler/stapler-squad/issues/533)) ([d699554](https://github.com/tstapler/stapler-squad/commit/d6995542b8de58287a79b01b10f0e7a4d0e11155))
* **session:** use PID-keyed tmux exec-gate dir to avoid shared TempDir race ([4d7149f](https://github.com/tstapler/stapler-squad/commit/4d7149f6b72c9f6f3ea5b6c8f5b60c8c541d4b31))
* **session:** watch newly created subdirectories for history JSONL files ([bb71330](https://github.com/tstapler/stapler-squad/commit/bb71330e2832b2b23ebebcb539c7646cb1877c7c))
* **test:** resolve flaky SDD triage lifecycle test and ListWithOptions no-op ([#529](https://github.com/tstapler/stapler-squad/issues/529)) ([ccda7ae](https://github.com/tstapler/stapler-squad/commit/ccda7ae3e03b4aecf7f743871c183e84db938d30))

## [1.44.0](https://github.com/tstapler/stapler-squad/compare/v1.43.1...v1.44.0) (2026-08-16)


### Features

* **backlog:** add UnarchiveBacklogItem RPC and archive confirmation guard ([#499](https://github.com/tstapler/stapler-squad/issues/499)) ([8af1707](https://github.com/tstapler/stapler-squad/commit/8af1707bfd5d77e45854308e9705d34686cf5ceb))
* **backlog:** display current reworkCapOverride value to users ([#507](https://github.com/tstapler/stapler-squad/issues/507)) ([a89fe5d](https://github.com/tstapler/stapler-squad/commit/a89fe5d230b5abffabba4d4fbc89a0eae2b2bc2c))
* **backlog:** share filter/sort state between list and board views ([#506](https://github.com/tstapler/stapler-squad/issues/506)) ([b234af6](https://github.com/tstapler/stapler-squad/commit/b234af61743b35280da380e08be67225d303f18d))
* **backlog:** wire retry action into detail-page BlockerChip ([#510](https://github.com/tstapler/stapler-squad/issues/510)) ([54bd63b](https://github.com/tstapler/stapler-squad/commit/54bd63bcb5fd0c8faa65b25cfcf4845cfac2ee7c))
* **github:** add SetCommitStatus primitive for the Statuses API ([648bd24](https://github.com/tstapler/stapler-squad/commit/648bd24c1becb276087f7f463b5687ca47a8611b))
* **mcp:** expose backlog list/notification-history/history-search as MCP tools ([#502](https://github.com/tstapler/stapler-squad/issues/502)) ([24122b9](https://github.com/tstapler/stapler-squad/commit/24122b9b3862cc9ce9dcafd8ded93afe4ef0a7c5))
* **session:** clean up temp prompt files on destroy, add fast-lane pane capture ([3cb89cd](https://github.com/tstapler/stapler-squad/commit/3cb89cd0b95ddf4221eb728afe8e3d6184e1321f))
* **sessions:** add stale session detection and alerting ([#515](https://github.com/tstapler/stapler-squad/issues/515)) ([f5a78fb](https://github.com/tstapler/stapler-squad/commit/f5a78fb72338cecc075b6586ac08934a0cf41036))
* Slack webhook notifications for review queue ([#525](https://github.com/tstapler/stapler-squad/issues/525)) ([a3f57cf](https://github.com/tstapler/stapler-squad/commit/a3f57cf246970451eab5cc7b297a877a11b17995))
* **terminal-resync:** improve terminal resync reliability ([04ec32a](https://github.com/tstapler/stapler-squad/commit/04ec32a92af1a8ec3f7b66cd39e06f8a9a329258))


### Bug Fixes

* **autonomous-driver:** suppress duplicate nudges on fixed idle-settle cadence ([#511](https://github.com/tstapler/stapler-squad/issues/511)) ([bdfd947](https://github.com/tstapler/stapler-squad/commit/bdfd9479b7436c90814fd0fd00c39b982e70a842))
* **backlog:** derive board card primary action from real item state ([#500](https://github.com/tstapler/stapler-squad/issues/500)) ([bc05477](https://github.com/tstapler/stapler-squad/commit/bc054776cb7931b0429359b49706cc2bbd16141b))
* **backlog:** make sortable table headers keyboard-accessible ([#504](https://github.com/tstapler/stapler-squad/issues/504)) ([a64c2d5](https://github.com/tstapler/stapler-squad/commit/a64c2d52f7b325c2cb35f7aa593861141c2a0c85))
* **backlog:** restore focus to trigger element when modals close ([#516](https://github.com/tstapler/stapler-squad/issues/516)) ([eb01643](https://github.com/tstapler/stapler-squad/commit/eb016431e12e34ba7cd69c78a6cab34d4d092c90))
* **backlog:** trap keyboard focus in ReviewChangesModal and BacklogFileBrowserModal ([#508](https://github.com/tstapler/stapler-squad/issues/508)) ([d068cae](https://github.com/tstapler/stapler-squad/commit/d068caea8c361f8505942817d1df4636a53bed20))
* **ci:** widen Benchmark Gate timeout and unhide its output ([6f2dc5d](https://github.com/tstapler/stapler-squad/commit/6f2dc5d9bf52ce8a5663ea5e0ca203fce7434a82))
* **e2e:** stop review-queue.spec.ts's 10 failures at their real root causes ([#520](https://github.com/tstapler/stapler-squad/issues/520)) ([f8f3772](https://github.com/tstapler/stapler-squad/commit/f8f3772a24c6e5c2b8066a4ae07990d48c28267d))
* **git:** prevent SIGSEGV in DiffHashBetween on nil FilePatch.Files() ([28b06ac](https://github.com/tstapler/stapler-squad/commit/28b06ac25def918c700cc39370a30d0efeb6eef3))
* **gogitstore:** bound closeAll() to stop soak-test hang under sustained load ([#517](https://github.com/tstapler/stapler-squad/issues/517)) ([d7dce85](https://github.com/tstapler/stapler-squad/commit/d7dce85da6004c92853782e667862f485ce779b6))
* **gogitstore:** bound gitRunErr subprocesses with a real timeout ([#523](https://github.com/tstapler/stapler-squad/issues/523)) ([9de4fb9](https://github.com/tstapler/stapler-squad/commit/9de4fb940371665d53b9d99905953f2acac82187))
* **gogitstore:** bound test-fixture git subprocess calls with a timeout ([e952d2e](https://github.com/tstapler/stapler-squad/commit/e952d2eb71d1d3710cae46249294c81b24f820a0))
* **perf:** batch Associator lookups instead of per-result full scan ([623b770](https://github.com/tstapler/stapler-squad/commit/623b770d57172b826fa5130d0cb131e984cc2678))
* **perf:** prune stale ordinal counters in MangleCorrelator ([73c128c](https://github.com/tstapler/stapler-squad/commit/73c128c97782affa21c36dcb12d0cedbab8d9fbd))
* **safeexec:** escalate to SIGKILL after grace period in CommandContextPG ([7e92c29](https://github.com/tstapler/stapler-squad/commit/7e92c29cfedf1e0bd9054c64a47599c69ede3ff3))
* **safeexec:** instrument SIGKILL escalation with logs, metrics, and proc-state snapshot ([#514](https://github.com/tstapler/stapler-squad/issues/514)) ([3e4ddea](https://github.com/tstapler/stapler-squad/commit/3e4ddeac463d2420d148a4fea661b83d176df84a))
* **services:** bound DeleteSession cleanup goroutines and tmux kill-session timeout ([#503](https://github.com/tstapler/stapler-squad/issues/503)) ([e5252ab](https://github.com/tstapler/stapler-squad/commit/e5252abdb05388b695dbe2e28049af7caeb97996))
* **session-retention:** eager-load Worktree in retention sweep ([cce386f](https://github.com/tstapler/stapler-squad/commit/cce386fc15e635f7a4087bfba1879c0213d55c04))
* **session:** block PTYDiscovery.Stop() until monitorLoop exits ([c213aee](https://github.com/tstapler/stapler-squad/commit/c213aeec33123146cc0bac67a1f9f943a846fbb5))
* **session:** fix TestWaitForPaneSettle flake, join driver/hibernate goroutines before teardown ([#527](https://github.com/tstapler/stapler-squad/issues/527)) ([facfb86](https://github.com/tstapler/stapler-squad/commit/facfb869a3f337c4f23e6d3b202c5472bf2a5985))
* **session:** join PTYDiscovery.Stop() with monitorLoop to fix TempDir cleanup race ([#521](https://github.com/tstapler/stapler-squad/issues/521)) ([445e252](https://github.com/tstapler/stapler-squad/commit/445e2529a1a4e43a329add454ca73696f1bce830))
* **terminal-resync:** pick up live terminal:resync-batching flag toggle ([7e76f18](https://github.com/tstapler/stapler-squad/commit/7e76f18d1bdbae8e5cbefe03f921e1937e99637f))
* **terminal-resync:** route all CurrentPaneRequest replies through writeCurrentPaneResponse ([05bafa8](https://github.com/tstapler/stapler-squad/commit/05bafa83a8f9b9a28ad2ec2832f778eded574a7f))
* **terminal-resync:** use data-testid locators in feature flags e2e spec ([1aa7cfc](https://github.com/tstapler/stapler-squad/commit/1aa7cfc53edbeb5e73c3a308d527310bcd896af6))
* **test:** resolve symlinks before comparing worktree paths on macOS ([e6bc36c](https://github.com/tstapler/stapler-squad/commit/e6bc36cbfed439b6ffa085dcdc8712a6c05b610d))
* **tests:** update stale selective-loading assertions to match applyLoadOptions ([61b71a9](https://github.com/tstapler/stapler-squad/commit/61b71a95ddc2b64f024680f4eea4dd381cca8926))
* **tmux:** route Close()'s kill-session call through the injected executor ([f54fa69](https://github.com/tstapler/stapler-squad/commit/f54fa6991daa3ff25662831c38b3615694aba31a))
* **web-app:** prevent mobile overflow in Unfinished/backlog chip rows ([c91a831](https://github.com/tstapler/stapler-squad/commit/c91a8315339ecdf8e511baabf566f3e41b6da5be))
* **web-app:** wire triggerRef into remaining useFocusTrap call sites ([#518](https://github.com/tstapler/stapler-squad/issues/518)) ([4e5e318](https://github.com/tstapler/stapler-squad/commit/4e5e318b24957777ea3350f348ec1e69d803129d))
* **worktree:** resolve symlinks in worktree base directory ([c729c74](https://github.com/tstapler/stapler-squad/commit/c729c74e95892a07843138e622114f003fa28a3e))


### Performance Improvements

* **github:** coalesce concurrent keychain token reads with singleflight ([057d0ea](https://github.com/tstapler/stapler-squad/commit/057d0eae8f14e55b3e1015aa6eb870df6fbb2191))
* **github:** document why keychainMu stays a single global mutex ([d0392a1](https://github.com/tstapler/stapler-squad/commit/d0392a11ded29a27a2ec13d8839383a4ba2c373e))
* **git:** reuse ahead/behind cache on unchanged hashes, bound countCommitsTo ([6661fb4](https://github.com/tstapler/stapler-squad/commit/6661fb4065a412b45a05ac0d6f8b5988a51c825e))
* **gogitstore:** cache indexSnapshot to avoid per-lookup map copy ([3666e09](https://github.com/tstapler/stapler-squad/commit/3666e09c51a1d23ff74c27bf8b8f91b066a5b7d0))
* **gogitstore:** decouple gitignore matcher cache from cachedRepo TTL ([1c70e2c](https://github.com/tstapler/stapler-squad/commit/1c70e2c11a67a3405cd46cbb4fc106e2c9715faf))

## [1.43.1](https://github.com/tstapler/stapler-squad/compare/v1.43.0...v1.43.1) (2026-08-14)


### Bug Fixes

* **backlog:** stop Sessions list from disappearing on live updates ([#496](https://github.com/tstapler/stapler-squad/issues/496)) ([8b3020e](https://github.com/tstapler/stapler-squad/commit/8b3020e72161b528af7807bf7b58d4cbfcce8f82))
* **gogitstore:** add deterministic prober to eliminate mmap flake ([#471](https://github.com/tstapler/stapler-squad/issues/471)) ([23b8f08](https://github.com/tstapler/stapler-squad/commit/23b8f089905745903a87ea570e887bc0fe0e1c9a))
* **session:** make EntRepository.ListWithOptions respect LoadOptions ([#497](https://github.com/tstapler/stapler-squad/issues/497)) ([370fff4](https://github.com/tstapler/stapler-squad/commit/370fff454963c8ebbfcc6d8bcb248b3c9646ba88))

## [1.43.0](https://github.com/tstapler/stapler-squad/compare/v1.42.0...v1.43.0) (2026-08-14)


### Features

* **backlog:** add backlog item dependency ("blocked by") tracking ([#472](https://github.com/tstapler/stapler-squad/issues/472)) ([61e747e](https://github.com/tstapler/stapler-squad/commit/61e747eb674d35c01533873fece1b5467990ccec))
* **backlog:** add ClaimantHostID for cross-host claim provenance ([#489](https://github.com/tstapler/stapler-squad/issues/489)) ([370e1a5](https://github.com/tstapler/stapler-squad/commit/370e1a5455b6afa378332de5d21bb76253ce0038))
* **backlog:** chat-based backlog item creation and refinement ([#490](https://github.com/tstapler/stapler-squad/issues/490)) ([2676d14](https://github.com/tstapler/stapler-squad/commit/2676d1408dfef848e6e1a95333d240eb97083ed8))
* **backlog:** persist backlog view state across page reloads ([#465](https://github.com/tstapler/stapler-squad/issues/465)) ([add6387](https://github.com/tstapler/stapler-squad/commit/add6387ef640dd735210b70b4c362a657f8045b3))
* **backlog:** surface session commit/cost telemetry in UI ([#492](https://github.com/tstapler/stapler-squad/issues/492)) ([db732f9](https://github.com/tstapler/stapler-squad/commit/db732f970112eb3d65ea99f0babf81ee45ee61c3))
* **mcp:** add report_blocked tool for externally-blocked backlog items ([#470](https://github.com/tstapler/stapler-squad/issues/470)) ([578a5a8](https://github.com/tstapler/stapler-squad/commit/578a5a8981dd63c7a3a74b5e6feae3170d30f445))
* **session:** track and surface subagent count in WAITING_FOR_AGENT status ([#312](https://github.com/tstapler/stapler-squad/issues/312)) ([55d8cfa](https://github.com/tstapler/stapler-squad/commit/55d8cfaa59fe0b49f5353a7ca6a55f0057fdbaec))
* webhook/event-driven session creation and lifecycle callbacks ([#381](https://github.com/tstapler/stapler-squad/issues/381)) ([a68eda3](https://github.com/tstapler/stapler-squad/commit/a68eda32cb677c47d2456594a90bd44ead745acc))


### Bug Fixes

* **backlog:** explain why disabled card action buttons are disabled ([#495](https://github.com/tstapler/stapler-squad/issues/495)) ([9af1532](https://github.com/tstapler/stapler-squad/commit/9af1532b9c942d09794e98f3bbb9792ff4c27ad8))
* **ci:** extend tmux create-timeout override to remaining CI steps ([#474](https://github.com/tstapler/stapler-squad/issues/474)) ([790f6c8](https://github.com/tstapler/stapler-squad/commit/790f6c80c2dba7ccd5239cb3ec3a5415caeb8ba9))
* **github:** distinguish rate-limit 403s from real auth failures ([c437283](https://github.com/tstapler/stapler-squad/commit/c4372836a7ca088fd940d0ec98f5d8a024fc1a12))
* **github:** don't swallow rate limiting as unauthenticated in fetchLoginFromRequest ([bb549f8](https://github.com/tstapler/stapler-squad/commit/bb549f8cd4535d852cf2d010c504d702062f93ea))
* **git:** serialize worktree add/remove/prune across goroutines and processes ([de17465](https://github.com/tstapler/stapler-squad/commit/de174658fd14f652e8a9715ca6b89f0e124139af))
* **git:** stop DiffHashBetween from panicking on symlink/gitlink diffs ([6232a78](https://github.com/tstapler/stapler-squad/commit/6232a782ee5e8c55e41de59954eede51318cbbe2))
* **gogitstore:** bound crash-subprocess tests to prevent 600s test hang ([#473](https://github.com/tstapler/stapler-squad/issues/473)) ([a6700aa](https://github.com/tstapler/stapler-squad/commit/a6700aa38f04672c1756795c1b2d8a1c35cd743a))
* **gogitstore:** tie reader lifetime to real repack completion, not a 2s guess ([6834183](https://github.com/tstapler/stapler-squad/commit/68341839b481eb80810eb4c8783add26b50ba24a))
* **history:** resolve tmux session UUIDs to Claude conversation UUIDs in history lookups ([47a0f71](https://github.com/tstapler/stapler-squad/commit/47a0f71b300fb53872b9466f7362e8081c0ef1b5))
* **session-monitor:** stop polling a session's history once it 404s ([70ef739](https://github.com/tstapler/stapler-squad/commit/70ef739725e08f886a57c53089865f6e5c953639))
* **session:** defer initTmuxSession until after first-time worktree setup completes ([4989c1f](https://github.com/tstapler/stapler-squad/commit/4989c1f1abcf5b7c7f319aeaf76885dc1605f04b))
* **session:** fall back to repo root when worktree path is missing from disk ([9dbda9d](https://github.com/tstapler/stapler-squad/commit/9dbda9d2ebc7a963fcf140e9961b67224396b71f))
* **session:** stop phantom repeated keystroke replay on reconnect/flap ([#295](https://github.com/tstapler/stapler-squad/issues/295)) ([23437f0](https://github.com/tstapler/stapler-squad/commit/23437f05de5f3fe9716a0e929cddccc16586ea8e))
* **session:** suppress duplicate autonomous nudges within cooldown ([#493](https://github.com/tstapler/stapler-squad/issues/493)) ([dab7993](https://github.com/tstapler/stapler-squad/commit/dab799362d34170f5ddfc741e0586d5d58f87df5))
* **tests:** harden fake-claude fixture exec + confirm SIGBUS/SIGSEGV in gogitstore mmap test ([#469](https://github.com/tstapler/stapler-squad/issues/469)) ([d23ae33](https://github.com/tstapler/stapler-squad/commit/d23ae339eaf0cda58b44cbca8a471b972dd6b569))
* **vcs:** stop polling VCS status/diff for deleted sessions ([8a51e70](https://github.com/tstapler/stapler-squad/commit/8a51e70dc0da7fee43f645d6963cf2bd9fb75f90))
* **worktree:** close backlog worktree branch-collision race ([492b0d6](https://github.com/tstapler/stapler-squad/commit/492b0d6dfc03709ded9f1e39a4ca3d4334e34332))
* **worktree:** close CreateBacklogWorktree repair/setup race ([33cfab2](https://github.com/tstapler/stapler-squad/commit/33cfab20f9013f876419c1718085254b66a81a5a))
* **worktree:** stop misdiagnosing zero-commit repos as corrupted clones ([de73341](https://github.com/tstapler/stapler-squad/commit/de733418cc0a63c2bdf79812eb666ceed1d7305b))

## [1.42.0](https://github.com/tstapler/stapler-squad/compare/v1.41.0...v1.42.0) (2026-08-13)


### Features

* **agy:** full Antigravity CLI hook support in ssq-hooks and UI + SDD planning artifacts ([#382](https://github.com/tstapler/stapler-squad/issues/382)) ([715eec1](https://github.com/tstapler/stapler-squad/commit/715eec1bdec05af28c6263600610f3c21951c560))
* **analytics:** add all-sessions tab and per-session breakdown table ([aa3d6ce](https://github.com/tstapler/stapler-squad/commit/aa3d6ced076606f7d5937c12a5b266f8a1613e34))
* **analytics:** add GetEscapeAnalyticsGlobalSummary RPC for cross-session escape stats ([75783ce](https://github.com/tstapler/stapler-squad/commit/75783cedf0e380afc38a3ff90a1ccd02c255391c))
* **analytics:** add tab and breakdown table styles for global view ([c1b4665](https://github.com/tstapler/stapler-squad/commit/c1b4665f0f60d97d67362f59c777aa850ee4eae1))
* **analytics:** add useEscapeAnalyticsGlobalSummary hook ([510aac2](https://github.com/tstapler/stapler-squad/commit/510aac26969482e16b8c0630cb3e9121d8174814))
* **backlog:** add likely_flaky stuck-item detector (behavioral signal) ([#448](https://github.com/tstapler/stapler-squad/issues/448)) ([19221d9](https://github.com/tstapler/stapler-squad/commit/19221d9d5d3e362c4c8db2d8c8a5504513e2f7c9))
* **backlog:** close the operator feedback loop — triage Q&A, plan revisions, session steering ([#457](https://github.com/tstapler/stapler-squad/issues/457)) ([8dc8e7f](https://github.com/tstapler/stapler-squad/commit/8dc8e7f9d0796dafec5dfcdaf28911217809622d))
* **backlog:** durable escalation signals for bouncing/multi-reason stuck items ([#444](https://github.com/tstapler/stapler-squad/issues/444)) ([728f9a3](https://github.com/tstapler/stapler-squad/commit/728f9a39cb9348e8164c997ac82ab2176c3743da))
* **backlog:** pause/resume backlog automation on quota-headroom signal ([#412](https://github.com/tstapler/stapler-squad/issues/412)) ([70463fa](https://github.com/tstapler/stapler-squad/commit/70463fad7eceff10634b777de731f8749f4f1e73))
* **backlog:** persist full raw output when a headless triage/review call fails ([#328](https://github.com/tstapler/stapler-squad/issues/328)) ([4e3e365](https://github.com/tstapler/stapler-squad/commit/4e3e36540356c98f568882c09ce62d3856dea1ac))
* **ci:** wire web-app Jest suite into lint.yml CI gate ([#438](https://github.com/tstapler/stapler-squad/issues/438)) ([c6aa6a3](https://github.com/tstapler/stapler-squad/commit/c6aa6a36aaea87dd69469d5102ce8178adf6a3fe))
* **detection:** detect context-compaction state from Claude Code output ([#455](https://github.com/tstapler/stapler-squad/issues/455)) ([583852d](https://github.com/tstapler/stapler-squad/commit/583852dca19b7d4a336baa903ba60bb3b64cd943))
* **import-external-session:** import external tmux sessions into stapler-squad ([#433](https://github.com/tstapler/stapler-squad/issues/433)) ([af80447](https://github.com/tstapler/stapler-squad/commit/af80447ed009e072e254e7f3063d535495c335c2))
* **launcher-presets:** pre-configured agent launch commands from a JSON config file ([#451](https://github.com/tstapler/stapler-squad/issues/451)) ([e9fd0f9](https://github.com/tstapler/stapler-squad/commit/e9fd0f9f479fac741131fc6479ffa1bc964c902e))
* **mcp:** add wait_for_backlog_event tool to replace polling ([#447](https://github.com/tstapler/stapler-squad/issues/447)) ([678f1c8](https://github.com/tstapler/stapler-squad/commit/678f1c8161f3638508fd119fb58ac38b20e8ebb6))
* **search:** cross-session FTS search for triage context and prior art ([#324](https://github.com/tstapler/stapler-squad/issues/324)) ([307855e](https://github.com/tstapler/stapler-squad/commit/307855e3ed519216089b7476a2bc1c350f58e279))
* **session-card:** suppress secondary info rows that duplicate the title ([#454](https://github.com/tstapler/stapler-squad/issues/454)) ([d237637](https://github.com/tstapler/stapler-squad/commit/d2376370c763ab4a1062f9a9f39a1b85ea0cd586))
* **session:** add InhibitionEngine for secret redaction during history transfer ([#403](https://github.com/tstapler/stapler-squad/issues/403)) ([3fc253b](https://github.com/tstapler/stapler-squad/commit/3fc253bd13596bafc9488c927886f735caf2421e))
* **session:** surface yolo/auto-approve mode as a per-session setting ([#440](https://github.com/tstapler/stapler-squad/issues/440)) ([e21ad5d](https://github.com/tstapler/stapler-squad/commit/e21ad5d41211ac4deb22013b0dccffeb408a504c))
* severity levels on review queue items ([#411](https://github.com/tstapler/stapler-squad/issues/411)) ([80ab744](https://github.com/tstapler/stapler-squad/commit/80ab7441c3a20134bcaa72ee09023ec0ad49e081))
* **ssq-hooks:** replace open-code proxy wrapper with native plugin hook ([#431](https://github.com/tstapler/stapler-squad/issues/431)) ([0acbcce](https://github.com/tstapler/stapler-squad/commit/0acbcce49904d344362ff4f9d7c0073d7414ac36))
* **terminal:** add manual input-mode override, detect fine-pointer on mobile ([9739635](https://github.com/tstapler/stapler-squad/commit/97396354e36d28a38debb1811e6c53d1b135d257))
* **terminal:** foreground fast reconnect — shorter connect-timeout when selected ([#414](https://github.com/tstapler/stapler-squad/issues/414)) ([ecb6d67](https://github.com/tstapler/stapler-squad/commit/ecb6d677f5c0810998d3b067c869d6e8015aa1e2))
* **workflows:** fuzzy model autocomplete + server-resolved model families ([#420](https://github.com/tstapler/stapler-squad/issues/420)) ([27a1527](https://github.com/tstapler/stapler-squad/commit/27a152744895eee5251a872aabdced35aa70e03a))


### Bug Fixes

* **analytics:** backport cancellation guard and enabled flag into useEscapeAnalyticsSummary ([172f78a](https://github.com/tstapler/stapler-squad/commit/172f78acf86cfaf502748f012cdb4d0140e5614b))
* **analytics:** fix partial escape sequence truncation and total_count in escape analytics ([0e781f4](https://github.com/tstapler/stapler-squad/commit/0e781f4ddd120fae8d88d92886856890acf40d68))
* **auth:** tighten registration authorization check ([fd05297](https://github.com/tstapler/stapler-squad/commit/fd052974a12b400ffd8ac759c0f5d83f7544c366))
* **backlog,sessions:** recover 2 unshipped fixes from a stale stash ([#389](https://github.com/tstapler/stapler-squad/issues/389)) ([62638a6](https://github.com/tstapler/stapler-squad/commit/62638a689d1aa2b19af0d7a5d55308b35be1538b))
* **backlog:** allow report_pr_created to reassign PR from pr_pending ([#423](https://github.com/tstapler/stapler-squad/issues/423)) ([8a972b8](https://github.com/tstapler/stapler-squad/commit/8a972b8636e9b7898ac7e82f6a20a67a5ac3c1c6))
* **backlog:** allow report_pr_created to reassign PR from pr_pending ([#423](https://github.com/tstapler/stapler-squad/issues/423)) ([5c75ca5](https://github.com/tstapler/stapler-squad/commit/5c75ca58e7493db1ff351d712871a4a19be8a1cf))
* **backlog:** branch new work-session worktrees from origin/main's real tip ([cf0f939](https://github.com/tstapler/stapler-squad/commit/cf0f939fe5d3588aa07b5cbb5ee3e87d2dd4a5c1))
* **backlog:** close reconcileBouncingItems' direct in_progress-&gt;done gate bypass ([e48cc35](https://github.com/tstapler/stapler-squad/commit/e48cc35d28b37c887ed2a347e3b8a718c0038a75))
* **backlog:** don't trust a recycled worktree path's HEAD as shipped work ([fa5cf8e](https://github.com/tstapler/stapler-squad/commit/fa5cf8ea8cda796d11b7fb89f3df86501fa5d1a9))
* **backlog:** isolate SDD triage writes into a dedicated, reused worktree ([#397](https://github.com/tstapler/stapler-squad/issues/397)) ([47a599a](https://github.com/tstapler/stapler-squad/commit/47a599af1f6898f1a1f1751483bd2dc091900618))
* **backlog:** release triageInFlight before optional auto-spawn, not after ([#453](https://github.com/tstapler/stapler-squad/issues/453)) ([950ab86](https://github.com/tstapler/stapler-squad/commit/950ab86dfc4dfc0d370fd70529f75829fac7c745))
* **backlog:** resolve scaffolding excludes via --git-common-dir, name dirty paths in request_review ([#427](https://github.com/tstapler/stapler-squad/issues/427)) ([c57d4d3](https://github.com/tstapler/stapler-squad/commit/c57d4d3d73122839d756841b1af33e9b086561fe))
* **ci:** gate PR comments on actionable findings ([#462](https://github.com/tstapler/stapler-squad/issues/462)) ([562f26e](https://github.com/tstapler/stapler-squad/commit/562f26e327bedbe282068b412259bebb120005b8))
* **ci:** migrate from custom buf install to bufbuild/buf-action ([#459](https://github.com/tstapler/stapler-squad/issues/459)) ([05bc43d](https://github.com/tstapler/stapler-squad/commit/05bc43d06cd84165a8d53b30ad9792146c398b88))
* **ci:** pass github_token to buf-setup-action to avoid rate limits ([86700b1](https://github.com/tstapler/stapler-squad/commit/86700b173df48bf57ed87c3b259abb36e13ea050))
* **ci:** widen hook-URL wait budget to 60s and serialize race gate with -p 1 ([#300](https://github.com/tstapler/stapler-squad/issues/300)) ([aafb05d](https://github.com/tstapler/stapler-squad/commit/aafb05d9f835c6f1d2a6f965dfce02de44802b46))
* **detection:** loosen compile-budget test's wall-clock assertion under contention ([4d71ee1](https://github.com/tstapler/stapler-squad/commit/4d71ee1b1b0af344006d04f30004fad87770f6ea))
* **e2e:** create fixtures dir in global-setup before writing theme fixtures ([af80447](https://github.com/tstapler/stapler-squad/commit/af80447ed009e072e254e7f3063d535495c335c2))
* **e2e:** resolve ambiguous omnibar submit-button locator ([#415](https://github.com/tstapler/stapler-squad/issues/415)) ([8a73a05](https://github.com/tstapler/stapler-squad/commit/8a73a05fd6ca2d227ca32ec29aa5c3aefce7b8a5))
* **e2e:** scope escape-analytics-global-view locators to avoid strict-mode collisions ([3fab8e1](https://github.com/tstapler/stapler-squad/commit/3fab8e1e22a3d6db4c7bd878330680ca899dac34))
* **files-tab:** CodeMirror viewer, mobile pane fix, and review UX fixes ([#404](https://github.com/tstapler/stapler-squad/issues/404)) ([7533ee7](https://github.com/tstapler/stapler-squad/commit/7533ee75de443a6b0578bbaec4760b53a8b1234d))
* **git:** distinguish ErrReferenceNotFound from other repo.Reference() errors ([#442](https://github.com/tstapler/stapler-squad/issues/442)) ([a97829e](https://github.com/tstapler/stapler-squad/commit/a97829e922fb57c8e4546b1a96ec71eaa54f882d))
* **github:** migrate RepoRef struct literals to smart constructor ([e7c882d](https://github.com/tstapler/stapler-squad/commit/e7c882d71daf4bd2a0d4649136e8f4285b925c57))
* **github:** prefer github.com token in GetKeychainToken to fix 401s ([a9850ea](https://github.com/tstapler/stapler-squad/commit/a9850ea6fb2063296b60bece3e463c7e32c7c31f))
* **github:** route all GitHub HTTP calls through the rate-limit-aware client ([910592f](https://github.com/tstapler/stapler-squad/commit/910592f4b25d8d05823e0f5d1f739a0f92693711))
* **import-external-session:** commit missing generated proto bindings ([#445](https://github.com/tstapler/stapler-squad/issues/445)) ([ad72dec](https://github.com/tstapler/stapler-squad/commit/ad72decaed4e8c370ed06dd50f7013e21fef93d7))
* **logging:** stop streaming log/warn/debug console output by default ([a8870fa](https://github.com/tstapler/stapler-squad/commit/a8870faa819106a1829f89074909e9f8830521bd))
* **makefile:** match service-installed stapler-squad in restart pkill ([fa2926e](https://github.com/tstapler/stapler-squad/commit/fa2926e8d3f79384f55025d46072cbc4dd541495))
* **mobile:** reserve BottomNav space in detail pane full-screen overlay ([7e9e905](https://github.com/tstapler/stapler-squad/commit/7e9e905aed449148560437f55eb8a2d517903af3))
* **omnibar:** make footer reachable via scroll when Advanced Options expanded ([#437](https://github.com/tstapler/stapler-squad/issues/437)) ([b35add7](https://github.com/tstapler/stapler-squad/commit/b35add749a2f7192c2e83f625a8cb1829eba0d6e))
* **omnibar:** reset isSubmitting on success in SpawnShell and Alias branches ([#441](https://github.com/tstapler/stapler-squad/issues/441)) ([8680ee0](https://github.com/tstapler/stapler-squad/commit/8680ee0ad162f7ce117830079e10936ef79233f8))
* **review-gate:** block review on an empty committed diff instead of false-passing ([6fc5b74](https://github.com/tstapler/stapler-squad/commit/6fc5b7433e1454c3f5cfe3b9e1474dfe46271c73))
* **review-gate:** guard review/pr_pending-&gt;ready transition on PASS verdict ([#466](https://github.com/tstapler/stapler-squad/issues/466)) ([c1b6f62](https://github.com/tstapler/stapler-squad/commit/c1b6f626d511a672fdb0db9eb3e32249c868181e))
* **search:** isolate test-mode search index and skip disk persistence ([#391](https://github.com/tstapler/stapler-squad/issues/391)) ([3976520](https://github.com/tstapler/stapler-squad/commit/39765206cc70ce812da940ea642754105a358447))
* **server:** find UniFi-assigned LAN hostname via scoped-DNS reverse lookup ([8e3c91a](https://github.com/tstapler/stapler-squad/commit/8e3c91af6fd404a12158ab570c1c12aed5313884))
* **session,server:** close two unrelated CI flakes from PR [#385](https://github.com/tstapler/stapler-squad/issues/385) (torn read, Instance.Status race) ([#401](https://github.com/tstapler/stapler-squad/issues/401)) ([1b2818b](https://github.com/tstapler/stapler-squad/commit/1b2818b99cbe8000c482c0ecd41ae383b62129be))
* **session/git:** prevent path traversal in worktree/session names ([#393](https://github.com/tstapler/stapler-squad/issues/393)) ([d423af3](https://github.com/tstapler/stapler-squad/commit/d423af392053973fc522f0032dffe1f47568b3ff))
* **session:** delete shells before session to avoid FK constraint failure ([c35e68b](https://github.com/tstapler/stapler-squad/commit/c35e68bee55640fce76050163c3be70117547f00))
* **session:** fix root causes blocking one-off session STOPPED transition ([#432](https://github.com/tstapler/stapler-squad/issues/432)) ([03b2fab](https://github.com/tstapler/stapler-squad/commit/03b2fab881b7ffb5e1f35664a592c2c7747d1db0))
* **session:** gate workspace-peers nudge behind feature flag, fix format/sort bugs ([#413](https://github.com/tstapler/stapler-squad/issues/413)) ([e30c4c4](https://github.com/tstapler/stapler-squad/commit/e30c4c414af6ed83a865181fc4c7ab245967aa6d))
* **session:** populate GitHub PR URLs and fix macOS symlink test flake ([096ce79](https://github.com/tstapler/stapler-squad/commit/096ce7952694c3cd69cd923042fe3032047d8450))
* **session:** populate GitHubPRURL when creating a session from a PR URL ([1a41b15](https://github.com/tstapler/stapler-squad/commit/1a41b15ab17278158c9ce352637f22e5bf105356))
* **session:** recover conversation UUID from disk before cold-restore launch decision ([#439](https://github.com/tstapler/stapler-squad/issues/439)) ([e156a3f](https://github.com/tstapler/stapler-squad/commit/e156a3f9d7127e23eea9e71c50b60a6a6635c9d1))
* **session:** stop silently dropping history on unmatched program switches ([#419](https://github.com/tstapler/stapler-squad/issues/419)) ([4983bf3](https://github.com/tstapler/stapler-squad/commit/4983bf3884d87694776892da39f2a6a9c193e75d))
* **session:** use safeexec.CommandContext in diffhash test git helper ([d24c3f2](https://github.com/tstapler/stapler-squad/commit/d24c3f21d18e33b7849ca655d0ddcfd664f01fd5))
* **startup:** add instance lock and port-release wait to close service-restart race ([1c2774d](https://github.com/tstapler/stapler-squad/commit/1c2774d08b0aa2bfd0e0edff373ef59f4c476ef6))
* **terminal:** poll pane capture during resume instead of flashing stopped ([1ff3e12](https://github.com/tstapler/stapler-squad/commit/1ff3e12095d3d637f8f7d5e40d65e96ba6cd4fa3))
* **test:** serialize tmux-heavy integration test packages to fix flaky reconnect backoff test ([#436](https://github.com/tstapler/stapler-squad/issues/436)) ([97cbcea](https://github.com/tstapler/stapler-squad/commit/97cbcea8fb91e3bfdb5cea5a05d2e4279ef96ebf))
* **test:** use unique session title in StreamTerminal regression test ([852797d](https://github.com/tstapler/stapler-squad/commit/852797d8b5d75e6615edc90432562d86dbe7bfde))
* **test:** widen TestStreamTerminal_SendsRawOutput's stream wait budget for exec-gate contention ([#388](https://github.com/tstapler/stapler-squad/issues/388)) ([828d8b0](https://github.com/tstapler/stapler-squad/commit/828d8b057e2955b7dda213291e05a90ea27ee2b7))
* **tls:** enumerate all non-loopback interfaces for cert SANs and rpID ([9c0b81d](https://github.com/tstapler/stapler-squad/commit/9c0b81d757442cdb89120917c013d305075fefbc))
* **tls:** put literal IP hostnames in IPAddresses SAN, not DNSNames ([54c32c9](https://github.com/tstapler/stapler-squad/commit/54c32c9a83059473a3684e2a2187a7b77851695c))
* **tmux:** close slow control-mode subscribers instead of dropping bytes ([6e6a9f6](https://github.com/tstapler/stapler-squad/commit/6e6a9f676cc976a26fe477842d75c45abdad113e))
* **tmux:** give slow control-mode subscribers a grace period before closing ([b0416e2](https://github.com/tstapler/stapler-squad/commit/b0416e2243e108504ebc970f4bf20e248abd90f3))
* **tmux:** raise session-create poll loop's backoff cap to reduce self-inflicted subprocess contention ([#463](https://github.com/tstapler/stapler-squad/issues/463)) ([deedbe1](https://github.com/tstapler/stapler-squad/commit/deedbe19609f9a918db75220783be0584be6fbc5))
* **web-app:** mobile session panel tab bar overflow + pane chrome ([#418](https://github.com/tstapler/stapler-squad/issues/418)) ([c950dca](https://github.com/tstapler/stapler-squad/commit/c950dcaeee9bdf18bdf8a6db1a8807d2c5274be8))
* **web-app:** surface SpawnShell omnibar errors in discovery mode ([#443](https://github.com/tstapler/stapler-squad/issues/443)) ([e0d82f7](https://github.com/tstapler/stapler-squad/commit/e0d82f73de5869dd879f4690fb3aff65db5563ed))
* **worktree:** stop silently fabricating disconnected repos on missing paths ([9037ed5](https://github.com/tstapler/stapler-squad/commit/9037ed5e090f113e95e5dd7613cf0cf85564eacf))


### Performance Improvements

* **session-card:** reduce terminal snapshot overhead identified in browser profiling ([d90ae86](https://github.com/tstapler/stapler-squad/commit/d90ae8624d52b18dbd8c23a814f3876f93cc24a9))
* **session:** fix UpdateSession write amplification via narrow metadata UPDATE ([#421](https://github.com/tstapler/stapler-squad/issues/421)) ([541a9e8](https://github.com/tstapler/stapler-squad/commit/541a9e80f98c97aabce0bb680c5799f26b8ccd34))

## [1.41.0](https://github.com/tstapler/stapler-squad/compare/v1.40.1...v1.41.0) (2026-08-10)


### Features

* **backlog:** add category picker with per-category automation defaults ([#285](https://github.com/tstapler/stapler-squad/issues/285)) ([46b7e62](https://github.com/tstapler/stapler-squad/commit/46b7e6239e3c10d6214d254b6667fcb9798c44a5))
* **backlog:** auto-spawn ready items by priority by default ([f9aadda](https://github.com/tstapler/stapler-squad/commit/f9aadda7d9e68f293b63a4e388ccd9f25006e2d0))
* **backlog:** detect and address post-ship PR review feedback ([#309](https://github.com/tstapler/stapler-squad/issues/309)) ([e6160da](https://github.com/tstapler/stapler-squad/commit/e6160da84a322b69188f6a5e4be1e30da41ed43f))
* **backlog:** expand Description by default in item detail view ([#302](https://github.com/tstapler/stapler-squad/issues/302)) ([2a62181](https://github.com/tstapler/stapler-squad/commit/2a6218140ed345c22d12a5e16b7bc434c2df027d))
* **backlog:** manual escape hatch — associate PR / override status by hand ([#341](https://github.com/tstapler/stapler-squad/issues/341)) ([ee4c5cc](https://github.com/tstapler/stapler-squad/commit/ee4c5cc99091186f44243d0ed9f41f6c8cf30235))
* **backlog:** migrate GitHub source plugins to shared host-keyed keychain ([4584c1d](https://github.com/tstapler/stapler-squad/commit/4584c1d95c7bad34ab7e3c9397b36d082dbcccda))
* **backlog:** triage assigns priority and item category ([6393668](https://github.com/tstapler/stapler-squad/commit/63936680944377bed5f48f51b0af6c99f10d660f))
* **backlog:** two-way GitHub issue sync — provenance, forward/backward status sync, loop prevention ([#336](https://github.com/tstapler/stapler-squad/issues/336)) ([8a84747](https://github.com/tstapler/stapler-squad/commit/8a84747cae12a850c8ed3a9ee7cdfb68281892c4))
* CI/CD status in diff viewer ([#353](https://github.com/tstapler/stapler-squad/issues/353)) ([4443567](https://github.com/tstapler/stapler-squad/commit/4443567c1bf15e4daf1e41f644897e43007a8833))
* **detection:** user-extensible TOML agent-detector plugins ([#376](https://github.com/tstapler/stapler-squad/issues/376)) ([7dcd174](https://github.com/tstapler/stapler-squad/commit/7dcd174e2a869fbfb2c7dfe220d3392a1ffee13e))
* **github:** add personal access token auth as alternative to device flow ([c51ba77](https://github.com/tstapler/stapler-squad/commit/c51ba777b08595e957159365064db32c8dc84185))
* **github:** import accounts from gh CLI credentials ([789775f](https://github.com/tstapler/stapler-squad/commit/789775f4d673d9210d3f5820b873f40de45c8398))
* **github:** support GitHub Enterprise Server custom domains ([7daad3f](https://github.com/tstapler/stapler-squad/commit/7daad3f981f6dc81af542bfeb5ef73de14439a3d))
* **insights:** close remaining token-cost tracking gaps (AC-1..AC-7) ([#304](https://github.com/tstapler/stapler-squad/issues/304)) ([f214805](https://github.com/tstapler/stapler-squad/commit/f214805ca95afa4fbd7a8ef4fb2f43f3676c2e38))
* **mcp:** add create_backlog_item and import_github_issue tools ([8ddbafa](https://github.com/tstapler/stapler-squad/commit/8ddbafa583e4e0f605059a4c18d3b27453d10968))
* **mcp:** add workflow and approval-rule management tools ([7db255e](https://github.com/tstapler/stapler-squad/commit/7db255e6ef2c54fcbc789f7f42e6d14ce9824353))
* **mcp:** report_duplicate tool + request_review CAS generalization ([#308](https://github.com/tstapler/stapler-squad/issues/308)) ([a4793c1](https://github.com/tstapler/stapler-squad/commit/a4793c1d6858e8ae37c9f1a194bb7eed64c0e921))
* **review-queue:** escalation reasoning on review queue items ([#315](https://github.com/tstapler/stapler-squad/issues/315)) ([303d43f](https://github.com/tstapler/stapler-squad/commit/303d43fad345ec8a44980781235a3fe35656d239))
* **review-queue:** surface pending plan reviews and busy sessions ([1a75172](https://github.com/tstapler/stapler-squad/commit/1a751723b4ce38bb3b9f55700ba3bc896c1780c3))
* **session-summary:** session completion summary (proof-of-work document) ([#327](https://github.com/tstapler/stapler-squad/issues/327)) ([8bf21eb](https://github.com/tstapler/stapler-squad/commit/8bf21eb9e0ff64040e0ca83ff419c7091d322958))
* **session:** add markdown notes to sessions ([#383](https://github.com/tstapler/stapler-squad/issues/383)) ([fb29596](https://github.com/tstapler/stapler-squad/commit/fb2959628fed02828fb9dd24e7afa2975f9fdfe7))
* **session:** automatic retention cleanup sweep for archived sessions ([#303](https://github.com/tstapler/stapler-squad/issues/303)) ([bf2bb4e](https://github.com/tstapler/stapler-squad/commit/bf2bb4ea1ca35c959b869571b627f19260c9a9d6))
* **session:** preview destination checkout path for new_worktree and github_url modes ([0e724d7](https://github.com/tstapler/stapler-squad/commit/0e724d7a50abce450a1ec306a1bc66403e4d3751))
* **session:** resolve GitHub Enterprise URLs in omnibar session creation ([#343](https://github.com/tstapler/stapler-squad/issues/343)) ([aa29bcc](https://github.com/tstapler/stapler-squad/commit/aa29bccd251928c5a6421468211e701c94f57cdd))
* **session:** surface dead tmux panes as a distinct Crashed status ([#384](https://github.com/tstapler/stapler-squad/issues/384)) ([d46c099](https://github.com/tstapler/stapler-squad/commit/d46c0998d8ac547e500a04076a8ed5f43bd8667b))
* **web-app:** give repo path field parity with other path pickers ([#296](https://github.com/tstapler/stapler-squad/issues/296)) ([8813545](https://github.com/tstapler/stapler-squad/commit/8813545392a4f537a5c012ff592d36e2d90b0ee5))


### Bug Fixes

* **autonomous:** orchestrator response parser tolerates directive with no leading separator ([e66492c](https://github.com/tstapler/stapler-squad/commit/e66492c78c7b6b4b73eac8412728c38cbe8accd8))
* **backlog:** Approve Plan / triage-failed banner no longer dead-end for gated items with no plan ([#322](https://github.com/tstapler/stapler-squad/issues/322)) ([e41c670](https://github.com/tstapler/stapler-squad/commit/e41c6706ae8f1f2c1b9cc5280bda1133df18c685))
* **backlog:** attribute shutdown-killed triage sessions correctly (BUG-065) ([#370](https://github.com/tstapler/stapler-squad/issues/370)) ([d5ae229](https://github.com/tstapler/stapler-squad/commit/d5ae229429b1ba3a90586875640f51e3bc36996c))
* **backlog:** centralize headless-triage liveness check, raise staleness margin ([f7ab0c9](https://github.com/tstapler/stapler-squad/commit/f7ab0c9ad6e9480fb71b9a7f64b4b9b41c57b9ff))
* **backlog:** close lost-update race in SetBacklogItemPRAndTransition CAS ([#333](https://github.com/tstapler/stapler-squad/issues/333)) ([ba76043](https://github.com/tstapler/stapler-squad/commit/ba7604308092ef134a112b03621e16d0bc0e02da))
* **backlog:** closeIfSupersededByMain false-positives on empty BaseCommitSha (BUG-066) ([#371](https://github.com/tstapler/stapler-squad/issues/371)) ([0bbb46f](https://github.com/tstapler/stapler-squad/commit/0bbb46ff268a871e534be7439af3e5281dbb8bae))
* **backlog:** fail loudly instead of silently spawning sessions in the main checkout ([26e0d61](https://github.com/tstapler/stapler-squad/commit/26e0d610fe449a9b5fe53be8b6e0161ad4c24886))
* **backlog:** gate PR-fix respawns with backoff, notify on failed autonomous respawn ([#284](https://github.com/tstapler/stapler-squad/issues/284)) ([054f3fe](https://github.com/tstapler/stapler-squad/commit/054f3fe27297207aef3ad3b3ffd73c6b3939c81b))
* **backlog:** generalize orphaned-triage detector to cover queued items with no usable plan ([#321](https://github.com/tstapler/stapler-squad/issues/321)) ([d115a1c](https://github.com/tstapler/stapler-squad/commit/d115a1c7dbad73ffb9062ca6450a9ad34cd79681))
* **backlog:** kill tmux pane, not just archive, on terminal work/review sessions ([8be3f24](https://github.com/tstapler/stapler-squad/commit/8be3f242b386bf13e5a4b0e0308a6b88bcc056fc))
* **backlog:** record an audit trail when auto-respawn is skipped for an active session ([#319](https://github.com/tstapler/stapler-squad/issues/319)) ([36ae435](https://github.com/tstapler/stapler-squad/commit/36ae4352216282dae8484e395eaed7f26c0170f2))
* **backlog:** recover items wedged in review by an idle-but-alive reviewer (recovers [#342](https://github.com/tstapler/stapler-squad/issues/342)) ([#347](https://github.com/tstapler/stapler-squad/issues/347)) ([86b41f5](https://github.com/tstapler/stapler-squad/commit/86b41f53fef50f8594721e2ac4ecfee4873b0dae))
* **backlog:** recover sdd-pipeline triage stuck on premature-completion placeholder ([#290](https://github.com/tstapler/stapler-squad/issues/290)) ([1f8aa26](https://github.com/tstapler/stapler-squad/commit/1f8aa268a5066da5309f019a10bb9a79159e67c5))
* **backlog:** request_review permanently stuck once a zombie reviewer's FAIL verdict is auto-processed ([#320](https://github.com/tstapler/stapler-squad/issues/320)) ([bf2cdd8](https://github.com/tstapler/stapler-squad/commit/bf2cdd8cae0c2c1d296ee5305fc2fabdf8f191e0))
* **backlog:** stale-work remediation races onSessionExited, silently skips respawn (BUG-063) ([#365](https://github.com/tstapler/stapler-squad/issues/365)) ([661b986](https://github.com/tstapler/stapler-squad/commit/661b9869e18361e79bbd93608ec7c30e03cf9b20))
* **backlog:** stop auto-closing live PRs as "superseded" against the session's own base commit ([#346](https://github.com/tstapler/stapler-squad/issues/346)) ([04d485a](https://github.com/tstapler/stapler-squad/commit/04d485a4401539430f54ebc4d762808279266c51))
* **backlog:** stop orphaned-triage recovery from over-penalizing shutdown kills and duplicating live calls ([0ac6760](https://github.com/tstapler/stapler-squad/commit/0ac676001d50402c9b5ea840567a2a3887a2b992))
* **backlog:** stop plan-approval UI flicker on stuck items and item detail ([#386](https://github.com/tstapler/stapler-squad/issues/386)) ([ed0fda7](https://github.com/tstapler/stapler-squad/commit/ed0fda703f44e6424073703286e7c2e8d140f90f))
* **backlog:** stop zero-diff PASS items from landing in pr_pending/pr_number=0 or stalling forever ([#364](https://github.com/tstapler/stapler-squad/issues/364)) ([ed1a5b5](https://github.com/tstapler/stapler-squad/commit/ed1a5b52991d2013c16cc334c2bea45295789b89))
* **backlog:** surface classified EndReason in orphaned-triage stuck context ([#356](https://github.com/tstapler/stapler-squad/issues/356)) ([dcf4b0b](https://github.com/tstapler/stapler-squad/commit/dcf4b0bf29c918dbe25870fc84e03d5f08fee363))
* **backlog:** surface review verdict outcome/summary in bouncing stuck context ([#358](https://github.com/tstapler/stapler-squad/issues/358)) ([7178ead](https://github.com/tstapler/stapler-squad/commit/7178eadf1e0b3c2eb01ca7621a27078fc1d7ffae))
* **backlog:** surface staleness signal in blocked-spawn error ([#292](https://github.com/tstapler/stapler-squad/issues/292)) ([cf452de](https://github.com/tstapler/stapler-squad/commit/cf452dea732c9e02701f8c936124c42c58d997de))
* **backlog:** trigger auto-triage from MCP-created backlog items (BUG-061) ([#357](https://github.com/tstapler/stapler-squad/issues/357)) ([2b4ef51](https://github.com/tstapler/stapler-squad/commit/2b4ef51323cc94a7353bb507628ae01a1732bdee))
* **backlog:** validate repo_path is an absolute existing directory before triage ([#359](https://github.com/tstapler/stapler-squad/issues/359)) ([abbeed4](https://github.com/tstapler/stapler-squad/commit/abbeed45929bf5ed75ddac3cd3ad73cd2038c8c1))
* **detection:** recognize batched multi-tool-call summary lines as Active ([#385](https://github.com/tstapler/stapler-squad/issues/385)) ([dc75dad](https://github.com/tstapler/stapler-squad/commit/dc75dad148225ab0d9c56f55b41b0dbaeb8d67f9))
* **diff:** sanitize invalid UTF-8 in git diff output before proto marshal ([9f3845b](https://github.com/tstapler/stapler-squad/commit/9f3845bbfc3aea1094b3c7c1db8689b21e6cd10b))
* **e2e:** drop brittle exact-class fallback selector in SessionsPage ([#338](https://github.com/tstapler/stapler-squad/issues/338)) ([2621b7a](https://github.com/tstapler/stapler-squad/commit/2621b7abbaaa3fe043852c3130ec2c335bbf56cb))
* **escalation-reasoning:** sdd:6-verify findings — poller race, fragile mutation, shared category type ([#325](https://github.com/tstapler/stapler-squad/issues/325)) ([fc9198d](https://github.com/tstapler/stapler-squad/commit/fc9198dd7d687535e07b2d852004bd38d7b6f4c2))
* **github:** honor dynamically-added accounts in CreateSession host detection ([3536f4a](https://github.com/tstapler/stapler-squad/commit/3536f4a05df610584371ad7da4397abd4f56cec4))
* **github:** honor EnterpriseBaseURLOverride in graphQLURLForHost, isolate search index in tests ([#387](https://github.com/tstapler/stapler-squad/issues/387)) ([9c901ca](https://github.com/tstapler/stapler-squad/commit/9c901ca152133754b1a2c802b52ca3591bd8f5ad))
* **github:** include dynamically-added accounts in enterprise host detection ([a101f5e](https://github.com/tstapler/stapler-squad/commit/a101f5e22f8189122e41b2f927bb635d0d9c324b))
* **github:** serialize keychain access to close BUG-052 data race ([#287](https://github.com/tstapler/stapler-squad/issues/287)) ([db66eb3](https://github.com/tstapler/stapler-squad/commit/db66eb3ccb6ccf81e02679760d2a399d4582b037))
* **gogitstore:** canonicalize commondir path to fix cross-worktree store sharing ([77bd60a](https://github.com/tstapler/stapler-squad/commit/77bd60ab50e0d6cea534b2e67b6cc08577ac3c9b))
* **hooks:** stop URL-prefix collision from dropping/corrupting PostToolUse hook entries ([#329](https://github.com/tstapler/stapler-squad/issues/329)) ([65c0e01](https://github.com/tstapler/stapler-squad/commit/65c0e018f20aaff4b6865ccbe254278da5eaa5ca))
* **mcp:** allow report_pr_created to accept a fallback-branch PR with an audited override ([#351](https://github.com/tstapler/stapler-squad/issues/351)) ([645f211](https://github.com/tstapler/stapler-squad/commit/645f211f076bb0456c4aaa7695c38ea3ff53beea))
* **mcp:** close report_duplicate's CAS-race test flake via shared read seam ([#332](https://github.com/tstapler/stapler-squad/issues/332)) ([dcc939e](https://github.com/tstapler/stapler-squad/commit/dcc939e659f5f8172d0ca94fb32c5f735ea316f6))
* **notifications:** wrap header row and fix vertical scroll on narrow viewports ([#291](https://github.com/tstapler/stapler-squad/issues/291)) ([14e26b7](https://github.com/tstapler/stapler-squad/commit/14e26b7ba7c474118d57eebe84fbdbed5e32561c))
* **omnibar:** make suggested session names repo-first and owner-prefixed ([f0b67a3](https://github.com/tstapler/stapler-squad/commit/f0b67a3c4543557af282524e7d10d81ecc23ecc1))
* **repo:** drop LFS tracking and demo gifs to stop exceeding LFS budget ([#294](https://github.com/tstapler/stapler-squad/issues/294)) ([adebd19](https://github.com/tstapler/stapler-squad/commit/adebd196a9de158a0ff36717c1f8bc2a4544dcc3))
* **review-queue:** show Create Rule button for orphaned pre-deploy approvals ([#373](https://github.com/tstapler/stapler-squad/issues/373)) ([ee6d0bb](https://github.com/tstapler/stapler-squad/commit/ee6d0bb50ba7b07649d161261079ac6b796acdaa))
* **sdd:** remove unrelated context-compaction-detection file from prior commit ([3ef07de](https://github.com/tstapler/stapler-squad/commit/3ef07de64fc920aa1678430c1002c202dfb6c288))
* **services:** stop AnalyticsStore flush goroutine leak on SessionService shutdown ([#366](https://github.com/tstapler/stapler-squad/issues/366)) ([aeb5c1a](https://github.com/tstapler/stapler-squad/commit/aeb5c1a6ff958abfe9824b3562c0e4b6d526853e))
* **session:** exclude archived sessions from Review Queue polling ([#301](https://github.com/tstapler/stapler-squad/issues/301)) ([e6b1824](https://github.com/tstapler/stapler-squad/commit/e6b1824f641c46377834f8f17f7f613795bcecf7))
* **session:** register sessions with HistoryLinker and persist conversation UUID immediately ([d9816ec](https://github.com/tstapler/stapler-squad/commit/d9816ec779ad03d009873169a88b276c9d0b2322))
* **session:** stop nil GitWorktree panic in BenchmarkSessionRestorePerformance ([f3c08b8](https://github.com/tstapler/stapler-squad/commit/f3c08b8a6d9f955630849f86517b2fff26a438ad))
* **terminal:** cursor-sync post-resize snapshots so TUI repaints stop stacking ([#293](https://github.com/tstapler/stapler-squad/issues/293)) ([99c8d30](https://github.com/tstapler/stapler-squad/commit/99c8d30039787a4ca2bc3584ac8f34bea71d6a26))
* **tests:** fix flaky/failing tests across executor, tmux, gogitstore, and git packages ([dccee74](https://github.com/tstapler/stapler-squad/commit/dccee742a47e35f1001f2777da543607184ba1b3))
* **tests:** raise ManagedProcess_Wait timeout to 15s to reduce flakiness ([90ad5be](https://github.com/tstapler/stapler-squad/commit/90ad5be87d801fff03e531f2cfa1f995e07f906e))
* **tmux:** decouple pane-exit detection from reconnect backoff, close test-race ([#378](https://github.com/tstapler/stapler-squad/issues/378)) ([a2b8b8d](https://github.com/tstapler/stapler-squad/commit/a2b8b8dd4434909dcadc40abaa6f62724be75be2))
* **tmux:** guard PTY triple with ptmxMu to eliminate ptmx data race ([#377](https://github.com/tstapler/stapler-squad/issues/377)) ([5c1600c](https://github.com/tstapler/stapler-squad/commit/5c1600c1db6cc12eb003a389e17b5d25933b71bb))
* **tmux:** route production tmux exec calls through Binary()+ResolveSocket ([#363](https://github.com/tstapler/stapler-squad/issues/363)) ([2bab287](https://github.com/tstapler/stapler-squad/commit/2bab2878cd520fbfd582479c733087979846b7f4))
* **web-app:** add listGitHubCLIHosts mock to GitHubPRsSection.test.tsx ([#368](https://github.com/tstapler/stapler-squad/issues/368)) ([a10eb74](https://github.com/tstapler/stapler-squad/commit/a10eb74e1a7395d56dade4df39310b511786d347))
* **web-app:** move GitHubBadge to shared/ to fix boundaries/dependencies lint error ([84e107f](https://github.com/tstapler/stapler-squad/commit/84e107f8416be454a0b24f7704b1e18b9bf54306))
* **web-app:** un-quarantine BacklogEmptyState and SessionDetail.embedded suites ([#354](https://github.com/tstapler/stapler-squad/issues/354)) ([bf5b19f](https://github.com/tstapler/stapler-squad/commit/bf5b19f201bda366b9a290406cf5e32f2a60cbcd))


### Performance Improvements

* pool bufio.Scanner buffers to cut allocation churn ([5a4fb9d](https://github.com/tstapler/stapler-squad/commit/5a4fb9d6b93ed3102df1ac9576f4c0831a0cfa79))

## [1.40.1](https://github.com/tstapler/stapler-squad/compare/v1.40.0...v1.40.1) (2026-07-28)


### Bug Fixes

* **insights:** close pricing gaps and unaccounted costs on the Insights page ([#280](https://github.com/tstapler/stapler-squad/issues/280)) ([95ed72d](https://github.com/tstapler/stapler-squad/commit/95ed72d34fb224cced334016138b86a64896e67d))
* **notifications:** suppress dead-end notifications for headless review/triage sessions ([#227](https://github.com/tstapler/stapler-squad/issues/227)) ([5b4d9ea](https://github.com/tstapler/stapler-squad/commit/5b4d9ea307402a9bf9fce609f27c559d1f1bbb53))
* **session:** rebase pause/resume-500 fix onto the actor-based concurrency model ([#278](https://github.com/tstapler/stapler-squad/issues/278)) ([42b743d](https://github.com/tstapler/stapler-squad/commit/42b743d6b50620ebe0f55b5fb4c2192041f2d9cb))
* **web:** dead-band terminal resize loop, RPC dedup, and WebGL fallback ([#272](https://github.com/tstapler/stapler-squad/issues/272)) ([9651edd](https://github.com/tstapler/stapler-squad/commit/9651edd5ea2e485684545af235cc6b9155a8627e))

## [1.40.0](https://github.com/tstapler/stapler-squad/compare/v1.39.0...v1.40.0) (2026-07-27)


### Features

* **backlog:** add BacklogItemEvent proto schema and WatchBacklogItems RPC ([fd80654](https://github.com/tstapler/stapler-squad/commit/fd80654db228789043c59c14e2a1381939fce8ee))
* **backlog:** add backlogItemsSlice with normalized state and memoized selectors ([92fa823](https://github.com/tstapler/stapler-squad/commit/92fa8232567ad26abc0fb589049e1efcddd9678d))
* **backlog:** add column-transition fade to Kanban board on live status changes ([bf04a6a](https://github.com/tstapler/stapler-squad/commit/bf04a6a04caf375cf4f91b245924bebf5f9bc606))
* **backlog:** add flash-on-update visual treatment respecting prefers-reduced-motion ([fc370b9](https://github.com/tstapler/stapler-squad/commit/fc370b9d50306a82cd00bb73e8f91dfa0c364ee5))
* **backlog:** add ItemChangePublisher interface and event-bus adapter ([57ceff6](https://github.com/tstapler/stapler-squad/commit/57ceff66899a68466a93d1259069d895c27ad0d1))
* **backlog:** add useWatchBacklogItems hook with idle-staleness backstop ([d366c26](https://github.com/tstapler/stapler-squad/commit/d366c268d9f1eae19f7878fec6d560ba3012c7ee))
* **backlog:** configurable WIP cap + queued status for backlog work items ([#199](https://github.com/tstapler/stapler-squad/issues/199)) ([32405b1](https://github.com/tstapler/stapler-squad/commit/32405b161e4c02bb6f2ad033315bb194945d8e6f))
* **backlog:** expose item ID + deep link, restore board detail pane ([#216](https://github.com/tstapler/stapler-squad/issues/216)) ([b7bfb30](https://github.com/tstapler/stapler-squad/commit/b7bfb308e5380ccad74a35b4120b76dfafbc7cf7))
* **backlog:** fade out list rows that leave the active filter live (Epic 6.3) ([6da72a0](https://github.com/tstapler/stapler-squad/commit/6da72a00b9c7d41ff569674fd6afe5e9388a108f))
* **backlog:** implement WatchBacklogItems streaming RPC handler ([00c49a5](https://github.com/tstapler/stapler-squad/commit/00c49a5acd014cd9f41ed3beb914f1c08ebe05a2))
* **backlog:** mount ConnectionIndicator across all backlog live views ([36dabca](https://github.com/tstapler/stapler-squad/commit/36dabca490fa4fa5451b30577cc592a62fa93db5))
* **backlog:** PostToolUse git-drift steering hook for autonomous sessions ([1ac4bbd](https://github.com/tstapler/stapler-squad/commit/1ac4bbd483d14923b812a0e5234590f8fd5c376c))
* **backlog:** progressive disclosure + session diagnostics for item detail panel ([#208](https://github.com/tstapler/stapler-squad/issues/208)) ([8b37535](https://github.com/tstapler/stapler-squad/commit/8b37535575b47675866f2dec98d6b2e9ae0e2286))
* **backlog:** publish BacklogItemEvent from remaining mutation methods ([9e39106](https://github.com/tstapler/stapler-squad/commit/9e39106968ef4b0df7408fea0a8adf2186bff50e))
* **backlog:** publish BacklogItemEvent from TransitionBacklogItemStatus ([36286d6](https://github.com/tstapler/stapler-squad/commit/36286d634bd2077aa38f5a0d9782741b3db9a188))
* **backlog:** report_pr_created MCP tool + orphaned-PR reconciliation backstop ([547eeda](https://github.com/tstapler/stapler-squad/commit/547eeda578e7d4a4384082df074889dad8ee81d7))
* **backlog:** seed an sdd PipelineMode and an opt-in default-selection flag ([c2adbc7](https://github.com/tstapler/stapler-squad/commit/c2adbc7a6274e1d864a12de7f38595a01b43fb9b))
* **backlog:** wire /backlog list to live WatchBacklogItems stream ([6ab62d0](https://github.com/tstapler/stapler-squad/commit/6ab62d0f0fabb0268526fa942496e1571d131b09))
* **backlog:** wire /backlog/board to live WatchBacklogItems stream ([be7f46c](https://github.com/tstapler/stapler-squad/commit/be7f46c9becd6f8c17d4f2e408f832d27b7858ce))
* **backlog:** wire BacklogItemDetail to live WatchBacklogItems stream ([241bdb6](https://github.com/tstapler/stapler-squad/commit/241bdb6852179caf500ed2f2169e1e670c1364cf))
* **backlog:** wire BacklogItemPanel to live WatchBacklogItems stream ([31b774d](https://github.com/tstapler/stapler-squad/commit/31b774daeeefb8b13e50c9f40e97140db80b0cea))
* **events:** add BacklogItemChanged event type and payload ([ef2f9f7](https://github.com/tstapler/stapler-squad/commit/ef2f9f70b208bb501bb994504afb862cd9de94de))
* **git-history-cutover:** add 05-local-resync.sh for post-rewrite pull-down ([63581ed](https://github.com/tstapler/stapler-squad/commit/63581ed36fee3edd36dae65b8cd458c3dfc29f89))
* **session-list:** add collapsible category headers ([#211](https://github.com/tstapler/stapler-squad/issues/211)) ([bc31485](https://github.com/tstapler/stapler-squad/commit/bc314858d63ec2d448559eebc278b289c4982522))
* **session:** add workspace peer awareness across sessions ([#215](https://github.com/tstapler/stapler-squad/issues/215)) ([cce887b](https://github.com/tstapler/stapler-squad/commit/cce887bb74420c2b80c3e1f84d039a9b7948b903))
* **shell:** stream shell tabs through the control-mode pipeline ([716bd91](https://github.com/tstapler/stapler-squad/commit/716bd91271c194e556fade5e520ebac955c1dd67))
* **skills:** add git-history-cutover skill to drive the LFS purge runbook ([0a7026b](https://github.com/tstapler/stapler-squad/commit/0a7026b166e076b13c21787358adff6a82067f3e))
* **unfinished:** rebrand to "Up Next" with backlog queue + GitHub issue import ([#212](https://github.com/tstapler/stapler-squad/issues/212)) ([2d8560f](https://github.com/tstapler/stapler-squad/commit/2d8560f033c6e80b9257cb3c8ddd30e344febb96))
* **web:** always stream log/warn/error to server, gate only console.debug behind toggle ([30010bf](https://github.com/tstapler/stapler-squad/commit/30010bf207e128b8e85d2d7dca7a9eaae724c88e))
* **workflows:** add Simple/Advanced cron schedule widget ([#218](https://github.com/tstapler/stapler-squad/issues/218)) ([23ea31e](https://github.com/tstapler/stapler-squad/commit/23ea31ee1e53a6feed47d4e88309f4e0491bc05c))


### Bug Fixes

* **backlog:** add missing live-update flash to /backlog list rows ([ab13ec2](https://github.com/tstapler/stapler-squad/commit/ab13ec247d851a10b26dadb306c8ab5b448517bc))
* **backlog:** add missing readOnly prop to GateVerdictBox, backfill required mock fields ([175e698](https://github.com/tstapler/stapler-squad/commit/175e69882c08a6aaa6ea3400dabb270c881e3057))
* **backlog:** check for already-shipped work before reopening a closed PR ([365e98c](https://github.com/tstapler/stapler-squad/commit/365e98cf32d6941b319a81cf8c52ab02593a4c71))
* **backlog:** check RemediationBlocked(bouncing) before renotifying no-verdict review exits ([1b1fb20](https://github.com/tstapler/stapler-squad/commit/1b1fb209370326e3e55f73f488662ea50be54516))
* **backlog:** close remaining publish-hook bypasses on ItemSession mutations ([902a8de](https://github.com/tstapler/stapler-squad/commit/902a8de853a80582b52a63213e69b5c1e0ecdef1))
* **backlog:** close SkipReviewGate gap, surface fetch failures, link pipeline modes ([#273](https://github.com/tstapler/stapler-squad/issues/273)) ([ac0c9d0](https://github.com/tstapler/stapler-squad/commit/ac0c9d0facfec0aa85fdac450fb706137d1ee3fe))
* **backlog:** darken InlineNotice's accent text/icon color for WCAG AA contrast ([7013060](https://github.com/tstapler/stapler-squad/commit/70130601ec57749170c8f7771c97f893352b2975))
* **backlog:** debounce reconnecting-&gt;live flicker, announce status changes ([3bbc99a](https://github.com/tstapler/stapler-squad/commit/3bbc99af2958d1634a84802de503130b976f3c84))
* **backlog:** eager-load ItemSessions so live events don't blank verdict/triage data ([65c53b7](https://github.com/tstapler/stapler-squad/commit/65c53b74fa18b88bde4373c67273fbe029229f24))
* **backlog:** give stuck review-role autonomous_stuck rows a remediation path (BUG-048) ([36249ac](https://github.com/tstapler/stapler-squad/commit/36249ac1b5288490813a53dcd3e31d1b506f1db7))
* **backlog:** guard commit/push staging against already-tracked scaffolding files ([#206](https://github.com/tstapler/stapler-squad/issues/206)) ([92c1c71](https://github.com/tstapler/stapler-squad/commit/92c1c7113590b48bb2f51e5f4ee407eeda62c08f))
* **backlog:** harden TriggerReReview's diff/codebase-read path against a gone worktree ([fee6ad2](https://github.com/tstapler/stapler-squad/commit/fee6ad2ee2851408d7666cc65170b510c3a18725))
* **backlog:** implement GitHub issue import + reachable Approve Plan action ([#220](https://github.com/tstapler/stapler-squad/issues/220)) ([2f3fe9e](https://github.com/tstapler/stapler-squad/commit/2f3fe9e46c98e4b1623f2a7d525e75e91e4d5345))
* **backlog:** make main-sync a precondition of review, not a PR-fix side effect ([4aca93c](https://github.com/tstapler/stapler-squad/commit/4aca93cffa924c9ee154b518ccf2593acf711e9a))
* **backlog:** notify operator when reopen is blocked by a stale-but-alive work session ([86665d2](https://github.com/tstapler/stapler-squad/commit/86665d26dcd01acb69fa9cb7a8c373d6d2c6ca17))
* **backlog:** nudge work sessions toward isolated manual testing ([6c8e788](https://github.com/tstapler/stapler-squad/commit/6c8e788ffc1404c0bc57f1da3c935ec4eefbbc1f))
* **backlog:** pin edit-mode buffered-update banner outside scrollArea (Epic 6.4) ([cf37d10](https://github.com/tstapler/stapler-squad/commit/cf37d10052984c3dbb9da45291d5dfbdc1691251))
* **backlog:** publish live event when ReconcileStuckItems transitions status ([959715d](https://github.com/tstapler/stapler-squad/commit/959715dbac2aa082f13648166d775d07752d44c6))
* **backlog:** recalibrate stale-session thresholds + durable rework-block trace ([#219](https://github.com/tstapler/stapler-squad/issues/219)) ([b2a12ea](https://github.com/tstapler/stapler-squad/commit/b2a12eac2090109aacc808ec9b08d3714d182feb))
* **backlog:** record status-change audit events on all mutation paths ([#198](https://github.com/tstapler/stapler-squad/issues/198)) ([5d6b41e](https://github.com/tstapler/stapler-squad/commit/5d6b41eb860a7ff9f98c7c947709b645150a80c6))
* **backlog:** refuse codebase-read review fallback to shared main checkout (BUG-045) ([20eeaa8](https://github.com/tstapler/stapler-squad/commit/20eeaa8a22442114ac8ce7873dda9c2821e8583d))
* **backlog:** render queued/pr_pending/refining items on the kanban board ([23d173e](https://github.com/tstapler/stapler-squad/commit/23d173e3575515e1c2b62c2f96df9ae203cefba7))
* **backlog:** resolve any stuck reason on terminal status, not just autonomous_stuck/push_failed ([#204](https://github.com/tstapler/stapler-squad/issues/204)) ([a758b3a](https://github.com/tstapler/stapler-squad/commit/a758b3ae0d3b45ff21aca0c1b3111218490b8587))
* **backlog:** resolve orphaned push_failed rows on item completion ([#203](https://github.com/tstapler/stapler-squad/issues/203)) ([a5053cc](https://github.com/tstapler/stapler-squad/commit/a5053cc3de443ced34fb8eed26368f529617d2bd))
* **backlog:** restore status-change audit trail (events, triggeredBy, empty state) ([#214](https://github.com/tstapler/stapler-squad/issues/214)) ([c20be97](https://github.com/tstapler/stapler-squad/commit/c20be974f4eb64f152b8d0cf896fe43511c1958c))
* **backlog:** show gate verdict in BacklogItemPanel ([e71c06b](https://github.com/tstapler/stapler-squad/commit/e71c06b44dfcb8182c91a39c43ab9331704fa6be))
* **backlog:** show last review verdict read-only after item leaves review status ([897d3de](https://github.com/tstapler/stapler-squad/commit/897d3de803f903a01b8cc6ed9448c2b6bd5fb125))
* **backlog:** stop a fresh worktree's own base commit from reading as shipped ([31fb6d4](https://github.com/tstapler/stapler-squad/commit/31fb6d4a74c4434b5e8a9a83aa7f41fc78e67de0))
* **backlog:** stop abandoned_review from burning its budget on respawns the bouncing gate will discard ([d9aa2b5](https://github.com/tstapler/stapler-squad/commit/d9aa2b50cbe341e5e4086214161c0eb23610ace8))
* **backlog:** stop AutonomousDriver's orchestrator-inferred DONE from forcing premature review ([#222](https://github.com/tstapler/stapler-squad/issues/222)) ([03e6414](https://github.com/tstapler/stapler-squad/commit/03e64143d41d2f1e5638875a14d6e9c1791b2b44))
* **backlog:** stop flagging bouncing items whose PR already merged ([e310108](https://github.com/tstapler/stapler-squad/commit/e31010815e376c6c28c5e59d0e97622f97911329))
* **backlog:** stop pr_pending items from losing their PR reference forever ([b235f86](https://github.com/tstapler/stapler-squad/commit/b235f86aa02be83a7629f2baaf7995b5ca11e9fa))
* **backlog:** stop the stuck-review liveness checker from starving review-gate sessions ([b653b4f](https://github.com/tstapler/stapler-squad/commit/b653b4fa5ba3abe05bfca1dd284e113ee69ca60c))
* **backlog:** stop trusting a stale base-commit SHA as proof of shipped work ([eda0e0e](https://github.com/tstapler/stapler-squad/commit/eda0e0e1bcb90b3d0fb317f0c361d7f2a2cec080))
* **backlog:** stop unprocessed-review-verdict sweep from renotifying the same dead session every tick ([78f5bf7](https://github.com/tstapler/stapler-squad/commit/78f5bf7be3e1da8ca9130d87db5c7bdc4a4344d7))
* **backlog:** stop wasted rework cycles from two review-reconciliation gaps ([3203fbe](https://github.com/tstapler/stapler-squad/commit/3203fbe260683b714ba97ee5e0b0ee71f982b0d1))
* **backlog:** stop WatchBacklogItems hanging when nothing to report ([3df0d29](https://github.com/tstapler/stapler-squad/commit/3df0d290673fece745f81c5e80d5194d8c0d4f38))
* **backlog:** surface queued items permanently blocked by the planning gate ([68f1c9a](https://github.com/tstapler/stapler-squad/commit/68f1c9a7e874028b62af72314e94160ba5e59444))
* **backlog:** surface silent status-transition write failures instead of only logging ([#275](https://github.com/tstapler/stapler-squad/issues/275)) ([927b2c7](https://github.com/tstapler/stapler-squad/commit/927b2c70d67897bd7a57d1d1c46fa4a85ce5503d))
* **backlog:** surface swallowed AutoReopenAfterFailedReview spawn+rollback failures ([3b7f3ba](https://github.com/tstapler/stapler-squad/commit/3b7f3baef0b3f5683a365b191a74d20ab0b02d5a))
* **backlog:** unstick bouncing items whose code shipped without a PR ([fdf3522](https://github.com/tstapler/stapler-squad/commit/fdf352203484952072e018afd06ea81266729e54))
* **backlog:** update remaining TransitionBacklogItemStatus call sites for merged 5-arg signature ([3b6efd6](https://github.com/tstapler/stapler-squad/commit/3b6efd682d38736a023e3428ad1d95e7f0d3675c))
* **backlog:** wire orphaned_triage into the remediation backoff framework ([#274](https://github.com/tstapler/stapler-squad/issues/274)) ([f528f74](https://github.com/tstapler/stapler-squad/commit/f528f74eced4c4d59b6e5a4fb32e82bb2f9701ae))
* **backlog:** wire pr_pending_no_pr remediation action ([df9f558](https://github.com/tstapler/stapler-squad/commit/df9f5588e70144e8d9f0d0d30443cfb6fe8fdb8e))
* **ci:** skip LFS checkout in build/benchmark jobs that don't need it ([#276](https://github.com/tstapler/stapler-squad/issues/276)) ([5885c20](https://github.com/tstapler/stapler-squad/commit/5885c20314878b81d59f47a34b31272bf0d6f3a4))
* **gogitstore:** replace direct exec.Command with safeexec.CommandContext ([#223](https://github.com/tstapler/stapler-squad/issues/223)) ([42808a2](https://github.com/tstapler/stapler-squad/commit/42808a249e35cf6488615c5ea1cac57b9975ddd6))
* **headless:** surface swallowed ctx-timeout errors, revert bypassPermissions, add failure telemetry ([#221](https://github.com/tstapler/stapler-squad/issues/221)) ([2403d8f](https://github.com/tstapler/stapler-squad/commit/2403d8f4d20a78c87b0a5a1cecc8d3ee3bd5e985))
* **mcp:** resolve terminal tool mutations against the live instance, not a fresh deferred-start reload ([76c9076](https://github.com/tstapler/stapler-squad/commit/76c9076fb8ec26f82d0819532ce90587de944235))
* **omnibar:** bound worktree/session-create requests to fix indefinite hangs ([#207](https://github.com/tstapler/stapler-squad/issues/207)) ([3482017](https://github.com/tstapler/stapler-squad/commit/3482017504c9404036ca96609d7d6503e88cb0a3))
* **server:** bind both loopback addresses on dual-stack hosts ([9d3b455](https://github.com/tstapler/stapler-squad/commit/9d3b45528a2dbb2fc7f4d5566a9512fe64d6d4b6))
* **session:** avoid tmux command-too-long failure on large claude prompts ([219e4d9](https://github.com/tstapler/stapler-squad/commit/219e4d96209f7eecf4d775a74b2b3a9dd03ceb45))
* **session:** keep external tmux streamers in sync with shell lifecycle ([187f9c4](https://github.com/tstapler/stapler-squad/commit/187f9c40b75fb7a32280a5ffd4147431930df08f))
* **session:** notify backlog lifecycle listeners on operator-initiated session stop ([dd32916](https://github.com/tstapler/stapler-squad/commit/dd32916175cea39c86e98169dfc9434ec324257e))
* **session:** rate-limit backlog nudge retry on SendKeys failure (BUG-041) ([b8763c6](https://github.com/tstapler/stapler-squad/commit/b8763c63d1a2de3317a9d7c007d0d3786c2212f4))
* **session:** refuse to persist a worktree session's WorkingDir if it escaped the worktree ([e67f85e](https://github.com/tstapler/stapler-squad/commit/e67f85efaac069840213bc3fd6e07d621bf7bb09))
* **session:** split autonomous-driver turn injection into separate paste+submit writes ([96a2d22](https://github.com/tstapler/stapler-squad/commit/96a2d223684a75eb0c958dac337b6924dac941a9))
* **session:** use carriage return not newline to submit WriteToSession input ([4d2fab6](https://github.com/tstapler/stapler-squad/commit/4d2fab668a527795eb9db4d3010d5ecbf14fca5e))
* **shell:** fall back to capture-pane polling after early control-mode exit ([d146b96](https://github.com/tstapler/stapler-squad/commit/d146b96139a3d497ccc29e1a90b6ff5e607b4fb2))
* **terminal:** fix xterm theme flip and unpainted rows beyond 80x24 ([01cc299](https://github.com/tstapler/stapler-squad/commit/01cc299a779480e79863336ca920a9148b181efb))
* **tmux:** kill orphaned control-mode clients left over from a prior process instance ([b6e76be](https://github.com/tstapler/stapler-squad/commit/b6e76be7ddf8aba4cd51f6421309b160a4a8f1cf))
* **unfinished:** fix mobile sizing on Unfinished page and its modals ([#210](https://github.com/tstapler/stapler-squad/issues/210)) ([aa976a7](https://github.com/tstapler/stapler-squad/commit/aa976a7ac52f93d164d49580416f881843faae7a))
* **unfinished:** stop the scanner's watch list from growing forever ([8fd954d](https://github.com/tstapler/stapler-squad/commit/8fd954de61f9167365335a8befb1f2777d3b49ce))

## [1.39.0](https://github.com/tstapler/stapler-squad/compare/v1.38.0...v1.39.0) (2026-07-21)


### Features

* **backlog:** add durable ship-status widget for done items ([e5d72b2](https://github.com/tstapler/stapler-squad/commit/e5d72b24890beffa2371c866b0abf62b77a46fa0))
* **backlog:** auto-archive done items after 3 days, fix archived-item leak in default view ([#194](https://github.com/tstapler/stapler-squad/issues/194)) ([3dd9285](https://github.com/tstapler/stapler-squad/commit/3dd9285e58a02bbc2b2e5072762e7868769ad9a0))
* **backlog:** automated push_failed remediation with backoff gate ([#187](https://github.com/tstapler/stapler-squad/issues/187)) ([7c3508a](https://github.com/tstapler/stapler-squad/commit/7c3508a0dce2c1317f1d426b8eebdebd1e771af8))
* **backlog:** configurable rework cap + surface review verdicts to running sessions ([d0d2237](https://github.com/tstapler/stapler-squad/commit/d0d22371514f74267f834692ad7869972cfe2491))
* **backlog:** opt-in auto-create-PR policy for Review Queue ([#159](https://github.com/tstapler/stapler-squad/issues/159)) ([809b8a3](https://github.com/tstapler/stapler-squad/commit/809b8a344e92018e50e30177283b41f9c8e9f314))
* **backlog:** Phase A stuck-item auto-remediation with exponential backoff ([#185](https://github.com/tstapler/stapler-squad/issues/185)) ([b0f2678](https://github.com/tstapler/stapler-squad/commit/b0f2678567dd81e10848a4742df000cdb1607f48))
* **backlog:** richer ship-status widget — commit list + working diff view ([2072651](https://github.com/tstapler/stapler-squad/commit/2072651b4451693a7eabbec4ceb55459fba8e65a))
* **backlog:** route the automatic review gate through PipelineEngine and surface an empty-pipeline-modes hint ([#178](https://github.com/tstapler/stapler-squad/issues/178)) ([ad01a13](https://github.com/tstapler/stapler-squad/commit/ad01a1310d29d3e977381e1e83b02b4c87ba2511))
* **backlog:** self-service "Ship PR" action on the item detail page ([311228f](https://github.com/tstapler/stapler-squad/commit/311228f421753ba2e3eb9eb711e4fc04a27dbcdd))
* **backlog:** tell the agent to sync with main via the existing context file ([58ca3f1](https://github.com/tstapler/stapler-squad/commit/58ca3f1cd27147c330b91d42a30e8b35f332d2d0))
* **file-browser:** line-count badges, sort/filter, diff gutter markers, backlog status badges ([6272b9c](https://github.com/tstapler/stapler-squad/commit/6272b9ce555bfeb5d65ce586b7a23e200ac2c910))
* **scripts:** add sync-worktrees to merge main into every worktree ([f4395e6](https://github.com/tstapler/stapler-squad/commit/f4395e65c4a0b8738a07bbcb0af45edebaf22a1f))
* **server:** add actuator-style /actuator/health and /actuator/metrics ([d0baa1c](https://github.com/tstapler/stapler-squad/commit/d0baa1c48fa9860fd10ec57f51de2723d66e9334))
* **telemetry:** add an OTel MeterProvider alongside the existing tracer ([64eb073](https://github.com/tstapler/stapler-squad/commit/64eb073827be341608cee3815f90114c9bfe7945))
* **unfinished:** expose blobCache hit/miss stats via /debug/blob-cache ([367af95](https://github.com/tstapler/stapler-squad/commit/367af95e35578e5026e8d4e0c25adcaeb00d56ca))
* **vcs-widget:** 5 stateless sub-components (Phase 2 Epic 2.1) ([94f4e0b](https://github.com/tstapler/stapler-squad/commit/94f4e0bd932d21f78d164dc462fccbe25ae1e61d))
* **vcs-widget:** CaptureShipSnapshot + ReconcilePRPending wiring (Phase 3.3) ([2984f3d](https://github.com/tstapler/stapler-squad/commit/2984f3d9dd97149b5839c6085e14a852f21e8d66))
* **vcs-widget:** feature registry entries + Playwright e2e coverage (Phase 5) ([2a6df86](https://github.com/tstapler/stapler-squad/commit/2a6df86fc26204679f5afd3cfb864bbd15b04a3d))
* **vcs-widget:** FileStatsBetween go-git diff-stat helper (Phase 3.2) ([af072f2](https://github.com/tstapler/stapler-squad/commit/af072f2b43509136b13ba84e3a238c4ac4af7c87))
* **vcs-widget:** populate durable snapshot fields in GetBacklogItemShipStatus (Phase 3.4) ([8221b78](https://github.com/tstapler/stapler-squad/commit/8221b789064380af9238370015e7ffe0b37dca04))
* **vcs-widget:** proto + ent schema for durable ship-snapshot fields (Phase 3.1) ([191e34f](https://github.com/tstapler/stapler-squad/commit/191e34f47a8f9628d6be4be831a5e5a46cfaecc7))
* **vcs-widget:** render durable snapshot data + capture-failure copy (Phase 4) ([ac1a0d3](https://github.com/tstapler/stapler-squad/commit/ac1a0d3533c22876567114bf0ea41c08a0cffea1))
* **vcs-widget:** VcsWidget top-level composition (Phase 2 Story 2.2.1) ([cff0e16](https://github.com/tstapler/stapler-squad/commit/cff0e164fdffa78178bbee14748ae15e4198b0f9))
* **vcs-widget:** VcsWidgetData types, adapters, and mergeability derivation (Phase 1) ([df47728](https://github.com/tstapler/stapler-squad/commit/df477286e7971d2a7d474aa4486fc6ff58d3a22e))
* **vcs-widget:** wire VcsWidget compact mode into Unfinished item detail (Phase 2 Story 2.2.4) ([b17d72b](https://github.com/tstapler/stapler-squad/commit/b17d72b95bd867d93321ffb6521097b054b91cda))
* **vcs-widget:** wire VcsWidget into Backlog item detail (Phase 2 Story 2.2.3) ([f752bb6](https://github.com/tstapler/stapler-squad/commit/f752bb6c9d50419f0d5c43725a1331386ae4b1ab))
* **vcs-widget:** wire VcsWidget into Session detail VCS tab (Phase 2 Story 2.2.2) ([9d67a5a](https://github.com/tstapler/stapler-squad/commit/9d67a5a97fb7db9bba8c3bd2a3aacbbe37c1afbf))
* **vcs:** rich file status treatment and per-file diff stats in VcsPanel ([#161](https://github.com/tstapler/stapler-squad/issues/161)) ([752c1f7](https://github.com/tstapler/stapler-squad/commit/752c1f75be2b74ece1f004fd2e73edafd2aaafdd))


### Bug Fixes

* **backlog,settings:** dedup push-failure toast, fix empty diff modal, add config-tab timeout ([#179](https://github.com/tstapler/stapler-squad/issues/179)) ([7a06815](https://github.com/tstapler/stapler-squad/commit/7a06815fc0d42a875b15445bdf5489f0177ca28c))
* **backlog:** archive work sessions so they stop accumulating forever ([#191](https://github.com/tstapler/stapler-squad/issues/191)) ([19a0b68](https://github.com/tstapler/stapler-squad/commit/19a0b686ac7ec44b6817c51f93dd7262304b9627))
* **backlog:** auto-remediate stale work sessions instead of notify-only ([#196](https://github.com/tstapler/stapler-squad/issues/196)) ([410db67](https://github.com/tstapler/stapler-squad/commit/410db67b955bc5b6083be5ba8764a1d828daca9e))
* **backlog:** auto-resolve autonomous_stuck rows once the item finishes ([#200](https://github.com/tstapler/stapler-squad/issues/200)) ([721e6f9](https://github.com/tstapler/stapler-squad/commit/721e6f9217ef553a6990984a3a86ccb702b14f6d))
* **backlog:** auto-respawn review for items stuck abandoned in review ([#168](https://github.com/tstapler/stapler-squad/issues/168)) ([fc8c12a](https://github.com/tstapler/stapler-squad/commit/fc8c12ae2fc3ed5cf94666d3cafd9ae776f22213))
* **backlog:** close TOCTOU race that let a stale reopen clobber an already-shipped item ([#197](https://github.com/tstapler/stapler-squad/issues/197)) ([ce4783c](https://github.com/tstapler/stapler-squad/commit/ce4783c2fba379aa8ae102c99d346373aaf6e719))
* **backlog:** give agent sessions a bounded escape hatch out of the review loop ([#189](https://github.com/tstapler/stapler-squad/issues/189)) ([4f786f0](https://github.com/tstapler/stapler-squad/commit/4f786f0bb8675ad49638f30513841f039bedff7b))
* **backlog:** per-action + per-card pending state and toast feedback ([#167](https://github.com/tstapler/stapler-squad/issues/167)) ([68c339f](https://github.com/tstapler/stapler-squad/commit/68c339fff4aca4a42db2787747dae3106df3ebf2))
* **backlog:** recover PR-lifecycle drift when a real PR outlives its status ([#202](https://github.com/tstapler/stapler-squad/issues/202)) ([01db9b7](https://github.com/tstapler/stapler-squad/commit/01db9b7febad723a11362a42693012424ee32062))
* **backlog:** require code verified on main, not just PrURL set, before done ([95f23a2](https://github.com/tstapler/stapler-squad/commit/95f23a21ca13b31cd583c910441568dd8ff938ce))
* **backlog:** resolve autonomous_stuck rows when the pipeline later succeeds ([#177](https://github.com/tstapler/stapler-squad/issues/177)) ([a1c5cfe](https://github.com/tstapler/stapler-squad/commit/a1c5cfefc6f84206e47b63de9a67b8a9611309c8))
* **backlog:** reuse the same branch across rework/reopen spawns ([3675da9](https://github.com/tstapler/stapler-squad/commit/3675da970cda04ed30d17f992f0d38a810b61e17))
* **backlog:** self-heal already-tracked scaffolding files, add CI backstop ([#195](https://github.com/tstapler/stapler-squad/issues/195)) ([bd7933c](https://github.com/tstapler/stapler-squad/commit/bd7933c3c55de8f6c2ac02b88c2679fcc0480d17))
* **backlog:** serialize SpawnSessionFromItem per item to close a duplicate-work-session race ([#182](https://github.com/tstapler/stapler-squad/issues/182)) ([0f7d167](https://github.com/tstapler/stapler-squad/commit/0f7d167a2b1f79380e92fe035dace5113c56bbdb))
* **backlog:** set SkipTriage on tests that drive explicit status transitions ([#181](https://github.com/tstapler/stapler-squad/issues/181)) ([4574e0e](https://github.com/tstapler/stapler-squad/commit/4574e0eaf985d53c344a6c7400b41e9d5feaff9a))
* **backlog:** ship PASS-verdict PRs via a headless agent run before the mechanical fallback ([#193](https://github.com/tstapler/stapler-squad/issues/193)) ([1c310eb](https://github.com/tstapler/stapler-squad/commit/1c310eb5138d1c551441ef424fb56aee81a5dd5d))
* **backlog:** stop deleting a rework's worktree out from under itself ([653d04e](https://github.com/tstapler/stapler-squad/commit/653d04e13e457f712662257f07373a7421a2732f))
* **backlog:** stop the autonomous-driver bounce loop on turn-cap stops ([#180](https://github.com/tstapler/stapler-squad/issues/180)) ([dd3a287](https://github.com/tstapler/stapler-squad/commit/dd3a287f6a7645348cc9077c3ca1ee21995c6c89))
* **backlog:** sync PR branch with main before respawning a fix session ([#163](https://github.com/tstapler/stapler-squad/issues/163)) ([eb5c3bd](https://github.com/tstapler/stapler-squad/commit/eb5c3bd657b809f616385003048defed78d82681))
* **backlog:** tombstone confirmed-dead sessions in the stuck-item sweep ([a6e65dc](https://github.com/tstapler/stapler-squad/commit/a6e65dccd69d63f063a82e994b936e564d925f06))
* **backlog:** transition item to pr_pending when PR created via RunOneShot ([#160](https://github.com/tstapler/stapler-squad/issues/160)) ([0fd35e7](https://github.com/tstapler/stapler-squad/commit/0fd35e7edfd9b18c7fc38d7b128bafdeec0993cf))
* **backlog:** use earliest work session's base SHA for item diffs ([#188](https://github.com/tstapler/stapler-squad/issues/188)) ([ff684ee](https://github.com/tstapler/stapler-squad/commit/ff684ee907dc55809d9919d6b39cd4053d7b52ed))
* **git:** don't reuse a worktree left locked by an interrupted `worktree add` ([1375fee](https://github.com/tstapler/stapler-squad/commit/1375fee7b2984de4d9fbfe348d68e445b262b364))
* **mcp:** add missing backlogEnabled arg to NewCore call in integration test ([#192](https://github.com/tstapler/stapler-squad/issues/192)) ([468a39a](https://github.com/tstapler/stapler-squad/commit/468a39a94426a394458d80859950216a119d3798))
* **notifications:** thread backlog item ID as sessionID for coalescing ([#183](https://github.com/tstapler/stapler-squad/issues/183)) ([c8be796](https://github.com/tstapler/stapler-squad/commit/c8be796739f1c185c8922d54567f4414ce6f1843))
* **perf:** remove per-call logging from ClaudeCommandBuilder.Build ([d74c020](https://github.com/tstapler/stapler-squad/commit/d74c020074dc7d097f3bde69a941a8ffabdd7199))
* **session:** close ReviewState data race in HibernationSweeper and ReviewQueuePoller ([#186](https://github.com/tstapler/stapler-squad/issues/186)) ([d132bcb](https://github.com/tstapler/stapler-squad/commit/d132bcb03e1e024d4001c95af78bccf89be3ae1f))
* **session:** fail sessions loudly instead of silently landing in $HOME ([6fc7ce9](https://github.com/tstapler/stapler-squad/commit/6fc7ce960bdc9f7882c98dc0f6a6dd47bfef2994))
* **sessions:** named instances (e2e/preview/coverage) must not sweep the shared tmux socket ([9e0df51](https://github.com/tstapler/stapler-squad/commit/9e0df515db1343e98b3eb0ddbe2c6a73200cb62e))
* **test:** close a second git auto-maintenance trigger and harden fixture-rebuild cleanup in gogitstore ([#201](https://github.com/tstapler/stapler-squad/issues/201)) ([5d5ae4d](https://github.com/tstapler/stapler-squad/commit/5d5ae4d0b1fd43482ebaeb95aaf33b0afd6d7c74))
* **test:** disable gc.auto in gogitstore fixtures to stop concurrent-repack pack-count flakes ([#190](https://github.com/tstapler/stapler-squad/issues/190)) ([79594d8](https://github.com/tstapler/stapler-squad/commit/79594d873acca2f550aaa66d0a9f119311462353))
* **test:** share one tmux-socket reaper across session, session/mux, session/tmux ([b8b7077](https://github.com/tstapler/stapler-squad/commit/b8b7077d4051d7a9ea28ea84afb73bfb73bbf7b4))
* **test:** stop DefaultCapabilitySelfCheck singleton from poisoning backlog tests ([e164c2e](https://github.com/tstapler/stapler-squad/commit/e164c2efd4023a5d6efb1f3e4dd82cc3db8739b9))
* **test:** stop gogitstore heap-benchmark tests flaking under CI load ([#162](https://github.com/tstapler/stapler-squad/issues/162)) ([5908c76](https://github.com/tstapler/stapler-squad/commit/5908c762f983caa7d45ea6eb248834885bdaa95a))
* **test:** update FallbackBehaviorWhenWorktreePathMissing for the new fail-loud contract ([b70ef33](https://github.com/tstapler/stapler-squad/commit/b70ef3372097084f90356dc125b4d9b046776bd9))
* **vcs-widget:** close 3 parity gaps — browse-files wiring, deleted-branch copy, last-commit fallback ([ffb4f6c](https://github.com/tstapler/stapler-squad/commit/ffb4f6c5449e2cfa0eb0713220fa26ac470d4ce2))
* **vcs-widget:** fire neutral no-history copy on missing snapshot alone ([a254aba](https://github.com/tstapler/stapler-squad/commit/a254abaa3864543922f54eae117743dcb3d2983f))


### Performance Improvements

* **unfinished:** stop wiping blobCache on HEAD move, bound it with a fair-share LRU ([e5ddfe9](https://github.com/tstapler/stapler-squad/commit/e5ddfe92b56da39de2b382cdc7995211b829e9da))

## [1.38.0](https://github.com/tstapler/stapler-squad/compare/v1.37.0...v1.38.0) (2026-07-17)


### Features

* **backlog:** add pipeline-mode selector, management UI, and what-ran surface ([54a34cc](https://github.com/tstapler/stapler-squad/commit/54a34cc47e563039adf3f6bbc85d4086cbf614d9))
* **backlog:** add PipelineMode CRUD RPCs with structural validation ([edcf2f2](https://github.com/tstapler/stapler-squad/commit/edcf2f233e18945c8905bd70b7c25edfb41eeb57))
* **backlog:** add PipelineMode data model and PipelineEngine seam ([37daaed](https://github.com/tstapler/stapler-squad/commit/37daaed8dfdcee61cdffcc5fa33f719aa5b70b30))
* **backlog:** durable stuck-item visibility on /unfinished ([a0ebca8](https://github.com/tstapler/stapler-squad/commit/a0ebca880b0aeb58591272e9b56d304a72bc9c42))
* **backlog:** grant reviewer bounded codebase read access on empty-diff reviews ([#155](https://github.com/tstapler/stapler-squad/issues/155)) ([9697b86](https://github.com/tstapler/stapler-squad/commit/9697b869bce12b0f242ef697b249a255c863d9dd))
* **backlog:** opt-in auto-spawn-session toggle + fix a partial-update clobbering bug ([b28ace2](https://github.com/tstapler/stapler-squad/commit/b28ace2f5679a307a79b06ba494c2cee9c032757))
* **backlog:** standing detector for orphaned triage sessions ([60c8a2a](https://github.com/tstapler/stapler-squad/commit/60c8a2ab729237f3b19f3f92a81883e31c477243))
* **unfinished:** control gogitstore's mmap index via feature flags ([9da5dfa](https://github.com/tstapler/stapler-squad/commit/9da5dfacef9f23eee1f7204c21136b9c7bdc3dd9))
* **unfinished:** gogitstore production hardening — eviction, refcounting, mmap index ([97251a5](https://github.com/tstapler/stapler-squad/commit/97251a542b43d2bfd385a5d82c4fefe33ffcdc6f))


### Bug Fixes

* **actor:** route MCP server URL backfill through SetMCPServerURL ([c1c9e5e](https://github.com/tstapler/stapler-squad/commit/c1c9e5eb968fd70b5f810bb3558ee34cdc80aeb4))
* auto-regenerate ent ORM code on build (self-heal missing packages) ([81fa981](https://github.com/tstapler/stapler-squad/commit/81fa981a6a7e5d00b664f8cda3b92268824a84b4))
* **backlog:** auto-transition re-review PASS verdict to done; close bucket-2 manual-gates audit ([4d1501a](https://github.com/tstapler/stapler-squad/commit/4d1501a9ee489ea90ec363d2b31b9f9d45283836))
* **backlog:** AutoReopenForPRFix skips status churn when a fix is already active ([f8f788a](https://github.com/tstapler/stapler-squad/commit/f8f788ab46683e944f77acd1f20c73da88623170))
* **backlog:** distinguish real lookup failures from expected not-linked case ([d13755d](https://github.com/tstapler/stapler-squad/commit/d13755da8caf6ada2199abf5cf55693d41ed3377))
* **backlog:** notify on silent auto-merge fallback; dedup stuck-resolve idiom ([47bbe05](https://github.com/tstapler/stapler-squad/commit/47bbe05dfd42a7b79fff664f9e12009421ff98b4))
* **backlog:** notify operator when post-triage persistence fails ([11570a2](https://github.com/tstapler/stapler-squad/commit/11570a2c3b004ec50e58a4008ac4a9e5635bbb54))
* **backlog:** reconcile review-session-spawn refactor with pipeline-mode wiring ([7042dc8](https://github.com/tstapler/stapler-squad/commit/7042dc8c000df67ff810c0d660da1c8489c769f2))
* **backlog:** surface status-transition failure in autonomous-complete notification ([5a809a6](https://github.com/tstapler/stapler-squad/commit/5a809a6d0277794168fa2f5d313c40c28b1c7560))
* **backlog:** tombstone dead work sessions blocking respawn ([af426f2](https://github.com/tstapler/stapler-squad/commit/af426f27aea0152269179acf699e1bde4ffd0e4f))
* **backlog:** unrecognized session role still notifies the operator ([3295389](https://github.com/tstapler/stapler-squad/commit/329538912876a8d6ed6aa6ad6268e50839edd29c))
* **ci:** bump release build Go version to match go.mod ([7cde747](https://github.com/tstapler/stapler-squad/commit/7cde7473da66c24944ec3fab19be668d6f10ff65))
* **ci:** fetch the tmux submodule in every checkout that needs it ([8c30912](https://github.com/tstapler/stapler-squad/commit/8c309121f233a983bb911f8b0691b616427c0981))
* **ci:** make release builds structurally independent of tag-push recursion ([03ec5ab](https://github.com/tstapler/stapler-squad/commit/03ec5abfa791b6fd6b8ccc9ff2e41920fb5776c3))
* extract full tmux release tarball, not just configure ([c9a2528](https://github.com/tstapler/stapler-squad/commit/c9a2528cfc39e7968e2ba8f681638ccbb8071e22))
* **git:** stop deleting worktree branches on cleanup/reset ([a1e8efd](https://github.com/tstapler/stapler-squad/commit/a1e8efdf79eee2f22e34debea55df2f26914743a))
* **lint:** clear golangci-lint findings blocking CI ([3657cb0](https://github.com/tstapler/stapler-squad/commit/3657cb0ee8372d0cf938b1b1f0defef1be8a46dd))
* re-register third_party/tmux as a real git submodule gitlink ([7058d92](https://github.com/tstapler/stapler-squad/commit/7058d92fee95f47cc2e741c1d11afbd0872d17c1))
* remove broken Bazel/rules_foreign_cc tmux build path ([c4adbbf](https://github.com/tstapler/stapler-squad/commit/c4adbbfca5b35a3ca7578bf5c17c56e557d8a637))
* simplify build-tmux Makefile target and stamp prerequisite ([5e69576](https://github.com/tstapler/stapler-squad/commit/5e695769bda25e1e51ee1316fd247aa5e3bc8602))
* **test:** derive MCP tool count from registration, not a hardcoded literal ([e2302f0](https://github.com/tstapler/stapler-squad/commit/e2302f0a37ff1863383c36c88b7e88950e599bd7))
* **test:** eliminate two more tmux integration test flakes ([1f91b1b](https://github.com/tstapler/stapler-squad/commit/1f91b1b0a357300bb0f9a1c62c3cf999ad7513a3))
* **test:** retry transient git-gc failures in gogitstore test fixtures ([b78b6c8](https://github.com/tstapler/stapler-squad/commit/b78b6c8dbf8118871ac7a079f6694e60cf0c65c5))
* **test:** stimulate PTY output in TestStreamTerminal_SendsRawOutput to eliminate a real flake ([7ff0ac9](https://github.com/tstapler/stapler-squad/commit/7ff0ac99d5f337cd126c658cc9d9d8184b215dae))
* **test:** update MCP tool count assertion to 26 ([6a00974](https://github.com/tstapler/stapler-squad/commit/6a00974e59eb2a3d487f7e9aae1a55466e2f2f08))
* **tmux:** recover EnsureServerRunning from a transient check-race ([0d1272b](https://github.com/tstapler/stapler-squad/commit/0d1272b5f7b82e100aac4c8846b0a8ec481f980a))
* **unfinished:** harden gogitstore mmap index for production readiness ([cfa0121](https://github.com/tstapler/stapler-squad/commit/cfa01210ee88530af21c5f3a15bedc2dedba0f37))
* **unfinished:** memory-based cache eviction and event-driven scanning ([c998fa9](https://github.com/tstapler/stapler-squad/commit/c998fa900c5e90b268d0f59e6f151cdd141a6d80))
* **unfinished:** shrink and share go-git's per-repo object cache ([ed6d85a](https://github.com/tstapler/stapler-squad/commit/ed6d85ae9d66866dfed2588b6aa95ac38ff81f8c))

## [1.37.0](https://github.com/tstapler/stapler-squad/compare/v1.36.0...v1.37.0) (2026-07-14)


### Features

* **agy,opencode:** add comprehensive CLI parity for Antigravity and OpenCode ([#135](https://github.com/tstapler/stapler-squad/issues/135)) ([38d8a29](https://github.com/tstapler/stapler-squad/commit/38d8a297b67d1fdbb13811a12ce0d7ddb5273222))
* **alias:** add name_prefix field + fix session name oscillation ([9c730be](https://github.com/tstapler/stapler-squad/commit/9c730bec10bbb84e7a8e3ab4a48015b542057602))
* **analytics:** bulk rule creation + page density improvements ([7ae2910](https://github.com/tstapler/stapler-squad/commit/7ae2910e41afd07b0ec1fa3495426a3f8a62d055))
* **analytics:** program detail panel + copilot review fixes ([2774602](https://github.com/tstapler/stapler-squad/commit/2774602a7b4f4cd89baf5d3f8abcabfa21e6a052))
* **analytics:** program detail panel with subcommand drill-down ([29c7af8](https://github.com/tstapler/stapler-squad/commit/29c7af8c25f81b935b048e8a23e27eab06e11a48))
* **analytics:** unify activity tables into single filterable view ([36de4b8](https://github.com/tstapler/stapler-squad/commit/36de4b80f50681cf33e070348eca34f40a457dd5))
* **backlog:** add feedback field to TriggerTriage proto for refine loop ([15d7082](https://github.com/tstapler/stapler-squad/commit/15d70824c0edddf4471bd1f45b2e0a76752f9d57))
* **backlog:** add feedback textarea + Refine action to TriageReviewPanel ([897aa5b](https://github.com/tstapler/stapler-squad/commit/897aa5b3bba4a9f058d2bcfa56bd33d3f1c7e710))
* **backlog:** add hard delete for backlog items ([56483ca](https://github.com/tstapler/stapler-squad/commit/56483ca2ffa65f7b0b60c7a12d40f52df6481875))
* **backlog:** add pr_pending state with GitHub PR push + merge polling ([3714852](https://github.com/tstapler/stapler-squad/commit/37148527b6e6e4a11cbb09ea134195b123ab40a3))
* **backlog:** add review context and View Changes modal to review panel ([92e5d38](https://github.com/tstapler/stapler-squad/commit/92e5d38b1a0b3b8816796f3ab9daa90151cc9d28))
* **backlog:** add UNVERIFIABLE verdict + duplicate session guard + review polling ([67ea766](https://github.com/tstapler/stapler-squad/commit/67ea7660142cfdbfd98963b5bf745ed471ac5325))
* **backlog:** allow backward status transitions to any earlier state ([5b3c4e8](https://github.com/tstapler/stapler-squad/commit/5b3c4e82389e954fa7c4465971e7cebbdc6468eb))
* **backlog:** auto-reopen for rework after FAIL/PARTIAL review; add note to status events ([cf1742b](https://github.com/tstapler/stapler-squad/commit/cf1742b11ec01f41b62d2fafa3da3fa7f2e6966e))
* **backlog:** block review→done when worktree commits exist without a PR ([7111268](https://github.com/tstapler/stapler-squad/commit/7111268a8e691722c5098624286bde1d8eaf833e))
* **backlog:** cap concurrent in-progress items with a WIP limit ([#150](https://github.com/tstapler/stapler-squad/issues/150)) ([91063c4](https://github.com/tstapler/stapler-squad/commit/91063c437871e8d4a4392235f801445477297cc8))
* **backlog:** detect PR merge conflicts and auto-spawn fix sessions ([8413f09](https://github.com/tstapler/stapler-squad/commit/8413f09e1fe2974a6cf1945d80781e08315024a3))
* **backlog:** extend restart to review status + reset to in_progress ([3c9f5dc](https://github.com/tstapler/stapler-squad/commit/3c9f5dc9e045633a1e81ae5f9b99d22509322b8c))
* **backlog:** finish GitHub sync — TriggerSync/GetSyncHistory RPCs, tests, settings UI ([#138](https://github.com/tstapler/stapler-squad/issues/138)) ([9c5927b](https://github.com/tstapler/stapler-squad/commit/9c5927b2551c8fced04723f010aba5424cec97b2))
* **backlog:** fix Changes view + show review criteria for all verdicts + mobile Shift key ([20e70f7](https://github.com/tstapler/stapler-squad/commit/20e70f70a846650a219f2b4146d0e58a2d9e1971))
* **backlog:** fix re-review + add manual review submission ([d164e9e](https://github.com/tstapler/stapler-squad/commit/d164e9e5bc1ce5a2bff7736ffbb778e0b6e421f1))
* **backlog:** gate backlog behind feature flag on all layers ([66b1d83](https://github.com/tstapler/stapler-squad/commit/66b1d831c9d8bcc0df3d9ee654d2fd8822165283))
* **backlog:** gate backlog behind feature flag on all layers ([1186494](https://github.com/tstapler/stapler-squad/commit/1186494baa7d7f9429030f7703ad1e6e4b7e7132))
* **backlog:** git worktree per work session + restart button + review SHA fix ([cea3ae1](https://github.com/tstapler/stapler-squad/commit/cea3ae1d5af69c83d49767fac04261592ec5120a))
* **backlog:** GitHub issue picker — browse repos and issues to import ([4157558](https://github.com/tstapler/stapler-squad/commit/4157558b1309879e570cb5a360288ca24fcaffde))
* **backlog:** ignore injected backlog files in worktrees and repo ([0ee9e14](https://github.com/tstapler/stapler-squad/commit/0ee9e14ac2865261dcb5152fe11ed2331caea049))
* **backlog:** implement CancelTriage RPC and session delete button ([a690c36](https://github.com/tstapler/stapler-squad/commit/a690c366d6704651b913a60dab6b8774303a7f45))
* **backlog:** import backlog items from GitHub issues ([46cf6d4](https://github.com/tstapler/stapler-squad/commit/46cf6d431f8604be9c645b7614278cf73f24bfdf))
* **backlog:** introduce BacklogItemSummary for list-view queries (Epic 5.1) ([10fa01e](https://github.com/tstapler/stapler-squad/commit/10fa01e80094e196db65cbb1a45b10cb08cd90a9))
* **backlog:** PR iteration with CI + review checks, done items visible ([33e54b1](https://github.com/tstapler/stapler-squad/commit/33e54b1195cef1023ba6cfc3d574118d7a8cf068))
* **backlog:** reopen-for-revision spawns new work session with feedback ([0fbf275](https://github.com/tstapler/stapler-squad/commit/0fbf2754c898cbf76168648ff735ba98ab0fe802))
* **backlog:** revision sessions get numbered names and backlog:revision tag ([acef346](https://github.com/tstapler/stapler-squad/commit/acef346f4c59c9819dc7c7bbc5fd2a2ce728dd5b))
* **backlog:** show VCS state, worktree path, and add file/diff navigation ([540bc23](https://github.com/tstapler/stapler-squad/commit/540bc234726f5799e7170516ba497b9b5a189d3e))
* **backlog:** track estimated session cost via existing TokenStore ([658d876](https://github.com/tstapler/stapler-squad/commit/658d876cc17e51b130b71b7c849d1c30f93bdf77))
* **backlog:** wire feedback-driven refine into TriggerTriage ([749cf72](https://github.com/tstapler/stapler-squad/commit/749cf72831c5dd168e5106697dce1aa9d3e86664))
* **backlog:** worktree cleanup, ship skill, branch visibility ([b77665d](https://github.com/tstapler/stapler-squad/commit/b77665dfc73102b5d30df62e27bb1f4ccf0de53f))
* **files:** file browser UX overhaul — mobile, a11y, perf ([169879b](https://github.com/tstapler/stapler-squad/commit/169879b7e631bea2305e2db2e22c637cfc5653ed))
* **files:** wire up the premium LocalFileBrowser component to the files page ([3743b94](https://github.com/tstapler/stapler-squad/commit/3743b945cb6085d160ed5734361b99442a66d9dc))
* GitHub work continuity — persistence, annotation fallback, and type-safe RepoRef ([3be7e09](https://github.com/tstapler/stapler-squad/commit/3be7e0902f57295fb17953d9ff3a84cabcfd3b9f))
* GitHub work continuity — UserPRCache, GitHubUserService, and Unfinished Tab integration ([#141](https://github.com/tstapler/stapler-squad/issues/141)) ([e322a7e](https://github.com/tstapler/stapler-squad/commit/e322a7ebf798ba9e18543fd9d84e396ad065d501))
* **harness:** headless triage test harness + alias kebab-case fix ([7ad3340](https://github.com/tstapler/stapler-squad/commit/7ad33400c273dcc3dc1ec6a210e3cdc4509fa2cf))
* **insights:** link backlog sessions to Insights SessionsTable ([46b66f9](https://github.com/tstapler/stapler-squad/commit/46b66f9799b6c6517deaecdbe3901e214de67a40))
* **nav:** group navigation into 4 sections, restore mobile access ([a2f78a3](https://github.com/tstapler/stapler-squad/commit/a2f78a38698a4a8414d1640a6b0c16795c527418))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([670caef](https://github.com/tstapler/stapler-squad/commit/670caeffd64e6e89aaf13df26c692580aa4effe0))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([250e640](https://github.com/tstapler/stapler-squad/commit/250e64071b9ddb72d44ca85cd6f620988f6bfb33))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([bc794b7](https://github.com/tstapler/stapler-squad/commit/bc794b7fc4e88b39dbc86ece70a5c086e48d47b6))
* **onboarding:** offer to install Claude Code hooks during onboarding ([#138](https://github.com/tstapler/stapler-squad/issues/138)) ([41e0206](https://github.com/tstapler/stapler-squad/commit/41e0206f54acbf58203959fbc697e173cb7471fb))
* **perf:** add singleflight + hasUncommitted TTL cache to GoGitVCSReader ([6fc0fb2](https://github.com/tstapler/stapler-squad/commit/6fc0fb20f187116698196946f82d707e3dc21f7a))
* **perf:** invalidate IsDirty cache on session Pause and Resume ([4cee38d](https://github.com/tstapler/stapler-squad/commit/4cee38dc0ffd2f164ce19dad01e4fab7fb543955))
* **pr-status:** show PR badge in row mode and use go-git for branch detection ([664fbfb](https://github.com/tstapler/stapler-squad/commit/664fbfb118d036a7ef52e0519a05d9e8368f22ca))
* **reconnect:** jittered backoff reconnect for session-watch and terminal streams ([#136](https://github.com/tstapler/stapler-squad/issues/136)) ([837ba8c](https://github.com/tstapler/stapler-squad/commit/837ba8cc70eb6f637c2b3bab2a4b27838e1d1113))
* **rules:** auto-suggest rule name from criteria inputs ([#140](https://github.com/tstapler/stapler-squad/issues/140)) ([d072abc](https://github.com/tstapler/stapler-squad/commit/d072abc37fcb228dd052cd38cb8c324eb8c3bdcf))
* **services:** extract CheckpointSvc, TerminalSvc, FeatureFlagSvc; delegate ListBranches+workflow (CDD Epic 1) ([8936b3a](https://github.com/tstapler/stapler-squad/commit/8936b3afc3ad039de23ee952e80979efdd45ca5a))
* **session-list:** bulk selection for row mode with undo-on-delete ([#137](https://github.com/tstapler/stapler-squad/issues/137)) ([3571d03](https://github.com/tstapler/stapler-squad/commit/3571d037fa81e3f3b1a804da21dbb6adbf7376ba))
* **session:** actor goroutine + mailbox + sendSync + goleak verification (IAC Epic 3) ([8adc0c9](https://github.com/tstapler/stapler-squad/commit/8adc0c9be7c2699fc506b365ba4f0bdd45e18790))
* **session:** bidirectional session history transfer between Claude and Antigravity ([#130](https://github.com/tstapler/stapler-squad/issues/130)) ([08d3f87](https://github.com/tstapler/stapler-squad/commit/08d3f87b06a954acff5ab0a567defe97db407bed))
* **session:** confirm dialog + re-sync guard for the program-change picker ([f781d7c](https://github.com/tstapler/stapler-squad/commit/f781d7cd3ac3640a3984282be3092294cff77880))
* **session:** IAC Epic 4 — actor-path state-machine core migration ([7bcc479](https://github.com/tstapler/stapler-squad/commit/7bcc479642a682671acd26d80cb618779614e418))
* **session:** InstanceSnapshot + atomic.Pointer read path, snapshot publish in all mutators (IAC Epic 1) ([e8cfafc](https://github.com/tstapler/stapler-squad/commit/e8cfafc44a56c217b4bfdb03a69b418ea8124ecd))
* **session:** migrate all unguarded readers to snapshot.Load() (IAC Epic 2) ([f491483](https://github.com/tstapler/stapler-squad/commit/f491483a76b7b5ff1fef2d6dc2131ff882ebe5ba))
* **session:** pending-program-change badge on the session card ([442380e](https://github.com/tstapler/stapler-squad/commit/442380e9b904c3bf6b9e971666a5e6f51ac34cb8))
* **session:** Registry + LiveInstance type-split lifecycle layer (IAC Epic 2.5) ([4232806](https://github.com/tstapler/stapler-squad/commit/42328063146596b88000b022ce0022f9e6341653))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([b695764](https://github.com/tstapler/stapler-squad/commit/b695764e9a5d12827da7db0815d81db262288dd9))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([f5f7f36](https://github.com/tstapler/stapler-squad/commit/f5f7f3618271b62783ca906274a5d4cac8eae13c))
* support Antigravity CLI hooks.json format in ssq-hooks ([55df54d](https://github.com/tstapler/stapler-squad/commit/55df54d4f60be8cddf6082c8ed64621f60db26de))
* **tmux:** add cross-process concurrency gate for tmux subprocess execution ([00988ac](https://github.com/tstapler/stapler-squad/commit/00988acc82fd3b344dcb3a1570cc037c56b7f3f6))


### Bug Fixes

* address copilot review comments on analytics drill-down ([077f53f](https://github.com/tstapler/stapler-squad/commit/077f53f6e075ef467f61022a3be28321f56cc387))
* **alias:** default session type + name oscillation ([4d33c7d](https://github.com/tstapler/stapler-squad/commit/4d33c7d50c4cd66ed4cbbd585d9556329259f050))
* **analytics:** escape analytics session_id mismatch and dead mangle detection ([#149](https://github.com/tstapler/stapler-squad/issues/149)) ([a121de8](https://github.com/tstapler/stapler-squad/commit/a121de83717185a5a42183288ffde1f8e0a298cf))
* **approval:** inject PermissionRequest hook on CreateSession and RestartSession ([8238a64](https://github.com/tstapler/stapler-squad/commit/8238a64c57033833615cfa7dc83c26f2db57bd0b))
* **arch:** architecture review fixes — ctx ordering, lint, and recovered stash deletions ([d63388e](https://github.com/tstapler/stapler-squad/commit/d63388e410604a6866350436fefc45bfbbfc6d25))
* autonomous sessions rejected with "path is required" via omnibar ([#157](https://github.com/tstapler/stapler-squad/issues/157)) ([5ab6c4a](https://github.com/tstapler/stapler-squad/commit/5ab6c4a08a03dbb6b28e5ae98975a72b543878d6))
* **autonomous:** send raw \r instead of \r\n for turn injection ([b758451](https://github.com/tstapler/stapler-squad/commit/b758451e6fc790dfa3dd4c3e757085a09336c950))
* backlog/triage sessions die on launch (shell injection + flag-parsing crash) ([#150](https://github.com/tstapler/stapler-squad/issues/150)) ([8016921](https://github.com/tstapler/stapler-squad/commit/8016921fa302c01ed9c16c5f568e410b9081b2b7))
* **backlog:** add SessionVcsProvider to ReviewChangesModal and fall back to DB for GetSessionDiff ([3da4a42](https://github.com/tstapler/stapler-squad/commit/3da4a42c25d21ae16b27497ca3127f1c08e812aa))
* **backlog:** address architecture-review follow-ups from post-merge audit ([#142](https://github.com/tstapler/stapler-squad/issues/142)) ([7791548](https://github.com/tstapler/stapler-squad/commit/7791548c1f3b633033975cdc4b2d6b44f0a7fc17))
* **backlog:** backfill pr_number when set to 0 despite a real pr_url ([4e411f5](https://github.com/tstapler/stapler-squad/commit/4e411f5b29bd137542d39467fa0c7d3850a68e6e))
* **backlog:** capture base SHA at session spawn so review sees full diff ([b6ff13a](https://github.com/tstapler/stapler-squad/commit/b6ff13aadf703e51e18901f8deab2661ad6e41a3))
* **backlog:** capture base SHA in AttachSessionToItem + fix WorkingDir race ([30c2590](https://github.com/tstapler/stapler-squad/commit/30c2590990ab35d2bd974be11856e0d102a5ae52))
* **backlog:** categorize backlog-spawned sessions as "Backlog" ([#149](https://github.com/tstapler/stapler-squad/issues/149)) ([f65c4a1](https://github.com/tstapler/stapler-squad/commit/f65c4a1796d6a7481989d2554f66cf5b37d630a6))
* **backlog:** enable Reopen for Revision in pending state, add files tab UX improvements ([859b71a](https://github.com/tstapler/stapler-squad/commit/859b71aa58e1ef9d5ee1cdd91b41788e93395eb9))
* **backlog:** fix approve flow and synthetic session display ([de58992](https://github.com/tstapler/stapler-squad/commit/de58992337c9caa60ab12d73d8382efa14fa6e44))
* **backlog:** fix review gate receiving empty diffs + restore sessionCreator nil guard ([9436a9a](https://github.com/tstapler/stapler-squad/commit/9436a9a3aefcec50cd094a31a14ccd5f9ef4096d))
* **backlog:** fix triage sessions tombstoned immediately on GetBacklogItem ([dc131c2](https://github.com/tstapler/stapler-squad/commit/dc131c2d45f27228e7d0ee77a2e10f3fa74cb760))
* **backlog:** fix triage spinner, delete, nav, stderr capture, and artifact dir ([aebc872](https://github.com/tstapler/stapler-squad/commit/aebc87204568de026e62dd70644b3816d6a25efd))
* **backlog:** GitHub URL repo-path support, first-visit tour, and two related bugs ([#152](https://github.com/tstapler/stapler-squad/issues/152)) ([6ef8164](https://github.com/tstapler/stapler-squad/commit/6ef8164b215f0c08c4a497b00fd33da516e9a288))
* **backlog:** harden claude binary lookup against a stale Linux PATH ([#136](https://github.com/tstapler/stapler-squad/issues/136)) ([6839082](https://github.com/tstapler/stapler-squad/commit/68390826f0a22d380c04a1163c65017a113e8583))
* **backlog:** harden the review-&gt;PR-&gt;merge reconciler and autonomous session UX ([bb5fa80](https://github.com/tstapler/stapler-squad/commit/bb5fa80ebc1ea520708606bc498da19b5e96cb03))
* **backlog:** harden triage JSON parser against stray braces in preamble ([#134](https://github.com/tstapler/stapler-squad/issues/134)) ([fd56d6a](https://github.com/tstapler/stapler-squad/commit/fd56d6ae1e5fa73a86c42b2ef245c0ca7d10fba3))
* **backlog:** harden triage parser and add repoPath UI gate ([06f9857](https://github.com/tstapler/stapler-squad/commit/06f98573b53af59afab38dfb0684edcc59210531))
* **backlog:** include repo name in spawned session title ([880af9e](https://github.com/tstapler/stapler-squad/commit/880af9ef378633283ce29f30b76e1c12d35a90ac))
* **backlog:** inject MCP config when spawning backlog work sessions ([3ccfc42](https://github.com/tstapler/stapler-squad/commit/3ccfc42a57f7852ed06ce8ee7bc5527389932a2a))
* **backlog:** make autonomous mode properly integrated with backlog store ([a84e89b](https://github.com/tstapler/stapler-squad/commit/a84e89bb25053510c8d718289434f21d71cbcc26))
* **backlog:** review gate produces empty/wrong diffs for fast work sessions ([39f3f7b](https://github.com/tstapler/stapler-squad/commit/39f3f7b94f803e544af0ddfa2bdef273b76adcf1))
* **backlog:** run autonomously + lifecycle safety nets ([19f35ac](https://github.com/tstapler/stapler-squad/commit/19f35ac9e8864f09b884c1c43c3c7ec165b15fbe))
* **backlog:** skip gitManager.Setup() for ExistingWorktree sessions ([06a6b46](https://github.com/tstapler/stapler-squad/commit/06a6b4677aa14367f079e25998a0efd1b47af8de))
* **backlog:** store AC notes in separate field, use nudge grace period ([3705e48](https://github.com/tstapler/stapler-squad/commit/3705e4885a07ab407f3d71bdbbb409afe4a7f14d))
* **backlog:** surface work-session verification evidence to the review gate ([#152](https://github.com/tstapler/stapler-squad/issues/152)) ([70e33b0](https://github.com/tstapler/stapler-squad/commit/70e33b0d077506fef2e0942c9046edf258b9558a))
* **backlog:** triage now writes ACs; review includes plan; AC merge semantics ([d1103aa](https://github.com/tstapler/stapler-squad/commit/d1103aa1c20a44e27cd7d5b5a05edd2041e97de0))
* **backlog:** TriggerTriage's cleanupCtx expired before it was ever used ([#137](https://github.com/tstapler/stapler-squad/issues/137)) ([6552ec9](https://github.com/tstapler/stapler-squad/commit/6552ec9039b1a9f364e874e552ecb9cea7f04530))
* **backlog:** uncommitted-changes guard + TriggerTriage race fix ([15f1f9e](https://github.com/tstapler/stapler-squad/commit/15f1f9e36a581fcc8941129c7e840c58f59d0ecb))
* **backlog:** use auto permission mode, short session names, autonomous tag ([c770fcd](https://github.com/tstapler/stapler-squad/commit/c770fcd5bf914b105f232de4be723780944590cc))
* **backlog:** use earliest work session SHA in TriggerReReview ([447a056](https://github.com/tstapler/stapler-squad/commit/447a056b2282e1f758adf65821e65a33c1b4d904))
* **backlog:** use worktree path and base SHA for all review diffs ([82cf2ea](https://github.com/tstapler/stapler-squad/commit/82cf2ea0c7a02fbc05fdafc838745f1f4f250601))
* **backlog:** wire review sessions + recover stuck review items ([d7e05c7](https://github.com/tstapler/stapler-squad/commit/d7e05c7e012a6a60ae673e27d3a19a7542797d7d))
* **build:** decouple lint from binary build; fix proto duplicate + GitHub RPC stubs ([cb20700](https://github.com/tstapler/stapler-squad/commit/cb20700b95b8ffa28eb60b55e388e370e08e00ea))
* **ci:** resolve lint/build failures on main ([e656a02](https://github.com/tstapler/stapler-squad/commit/e656a02cfcc7df04db6b43800950a358daf7fccb))
* **ci:** serve the static export in Lighthouse CI instead of next start ([8470143](https://github.com/tstapler/stapler-squad/commit/847014300b28cfe2ebd978efac761d8a5f3601eb))
* **codesign:** correct otool byte-order in verify-codesign plist decode ([ca60c0a](https://github.com/tstapler/stapler-squad/commit/ca60c0ae893ba542a62cc46c5789af38fb57da4c))
* **css:** enable scroll on unfinished tab container ([fa5f37e](https://github.com/tstapler/stapler-squad/commit/fa5f37e25607ca094858655a57cbc0ca9202f368))
* **detection:** detect dynamic workflows + expand turn-marker to ✦ ([2fad2c0](https://github.com/tstapler/stapler-squad/commit/2fad2c0eacc453b5a729b05d185774007622095f))
* **files:** inject &lt;base&gt; tag into HTML served by /api/files/raw ([2bfbf17](https://github.com/tstapler/stapler-squad/commit/2bfbf1705731270c8b9141dc3ee3491b7c0031fc))
* **files:** rewrite relative HTML asset URLs instead of injecting &lt;base&gt; tag ([49e391d](https://github.com/tstapler/stapler-squad/commit/49e391d4954eaa7b1c8ab212d2ba8ce35c42e0ff))
* **github:** centralize rate-limit handling in transport; read all rate-limit headers ([7fd1d77](https://github.com/tstapler/stapler-squad/commit/7fd1d7703d174943e5eb474bdac97982b883ab63))
* **github:** prevent data race in UserPRCache.Start() via sync.Once ([9ec03fb](https://github.com/tstapler/stapler-squad/commit/9ec03fb5ce53102cc7243e8dfd946063f175e2bb))
* **github:** remove duplicate maxRetryAfterSleep declaration ([c9b21aa](https://github.com/tstapler/stapler-squad/commit/c9b21aa759cab7d56dbf0c5bc6708264131488c5))
* **github:** use conditional PR discovery in PRStatusPoller + fix 403 error messages in repos.go ([c65ffbc](https://github.com/tstapler/stapler-squad/commit/c65ffbcafcc3183d2ec4ac45c196a85266671487))
* **git:** shell out to git CLI for HEAD SHA instead of go-git ([#151](https://github.com/tstapler/stapler-squad/issues/151)) ([70fe264](https://github.com/tstapler/stapler-squad/commit/70fe264d73257626762d735d4b9d25b27baf8d83))
* **git:** use fmt.Fprintf instead of WriteString(fmt.Sprintf(...)) ([7909e2f](https://github.com/tstapler/stapler-squad/commit/7909e2fbef3960a1bf5f83f2814a6859d8825672))
* **headless:** use Setsid instead of Noctty for headless runner subprocess ([095e09e](https://github.com/tstapler/stapler-squad/commit/095e09e33e2d01b9164cc5e0da707ed9fa83f35f))
* **install:** skip FDA prompt for non-admin users with cert-signed binary ([c24d088](https://github.com/tstapler/stapler-squad/commit/c24d088cd1bdb72225f0ac81cb16846b7d637c2a))
* **lint:** add concurrency:1 to avoid golangci-lint v2.11.4 race on go1.26 deps ([3407f0c](https://github.com/tstapler/stapler-squad/commit/3407f0c161e3132362be9f175118be02a692480a))
* **lint:** add concurrency:1 to avoid golangci-lint v2.11.4 race on go1.26 deps ([43110eb](https://github.com/tstapler/stapler-squad/commit/43110ebdf64d99b3ebe3bd6f1fc5adefeee190b9))
* **lint:** analytics-exempt line order; chore(ci): make demo GIFs manual-only ([1255f16](https://github.com/tstapler/stapler-squad/commit/1255f168a5ecbebc7c1d3ad88c58fdd1e0401711))
* **lint:** move analytics-exempt to line 1 in backlog-sources page ([51ced91](https://github.com/tstapler/stapler-squad/commit/51ced915db251b07b68f575fe3e96bb71b042dca))
* **lint:** resolve 4 violations surfaced after golangci-lint go1.26 rebuild ([9c69d79](https://github.com/tstapler/stapler-squad/commit/9c69d797ab0a4fec0461a9e9223af32fdc34eaba))
* **lint:** resolve blocking ESLint errors surfaced by push-to-main full scan ([64cc150](https://github.com/tstapler/stapler-squad/commit/64cc150fd84908ee70d576adb40cd1e3ebbf85fe))
* **lint:** return empty map instead of nil in GetAllInstanceArtifacts ([c8b9780](https://github.com/tstapler/stapler-squad/commit/c8b97803aa46a02fa3b801ee073f1f45a2232a0e))
* **lint:** suppress norawexec on lookPathOnlyExecutor stub ([d054495](https://github.com/tstapler/stapler-squad/commit/d054495cb49d5a4151475df6fa0fe2e8cb99edc2))
* **lint:** use correct nolint directives for lookPathOnlyExecutor stub ([8987673](https://github.com/tstapler/stapler-squad/commit/898767374dad24c5c6ab8e30aecf4ec5c96c5c29))
* **lint:** widen forbidigo + depguard exclusions to cover P1 split files ([1f3a64e](https://github.com/tstapler/stapler-squad/commit/1f3a64e3590e552387924acac9f2813a5e899ff0))
* **mcp:** transition item to review status in request_review tool ([003e078](https://github.com/tstapler/stapler-squad/commit/003e07857736162b4461cdd8a83e27834e368138))
* **mcp:** use stateless session mode to survive server restarts ([9f462de](https://github.com/tstapler/stapler-squad/commit/9f462dea91dddb6dd95e12b3f2f9c152198036a0))
* **mcp:** write project MCP config to .mcp.json instead of settings.local.json ([7d47dbb](https://github.com/tstapler/stapler-squad/commit/7d47dbbdd1ad6d932ebbb1b6dd68db48722c26b6))
* **mobile:** prevent horizontal scroll on notifications and other pages ([759a100](https://github.com/tstapler/stapler-squad/commit/759a1006c2f4ad90b304793293dc89d46a693ac1))
* **nav:** add Feature Flags to navigation menu ([3fa665c](https://github.com/tstapler/stapler-squad/commit/3fa665cf20b250f51d09bb880fdcd9f1a7164c94))
* **omnibar,tmux:** kebab-case session names and fix tmux name mismatch ([#162](https://github.com/tstapler/stapler-squad/issues/162)) ([0c805fb](https://github.com/tstapler/stapler-squad/commit/0c805fb41c52f5d18cfc76fa829e823316959c52))
* **pane:** restore session peek modal integration in pane picker ([6aef840](https://github.com/tstapler/stapler-squad/commit/6aef84015c5dba709b17f812b8b1810abb601f5e))
* **perf:** downgrade snapshot-cache hot-path logging to Debug ([119782a](https://github.com/tstapler/stapler-squad/commit/119782a31758e4bbd9c5088d66539957f276f81b))
* **perf:** release entry.mu before OS stat walk in HasUncommitted; typed nil returns in Do bodies ([71f52b3](https://github.com/tstapler/stapler-squad/commit/71f52b39bc907b69b7c6103db641ed59c6e92db0))
* **perf:** rename misleading panic test, add scope comment, move InvalidateDirtyCache post-transition ([799a352](https://github.com/tstapler/stapler-squad/commit/799a3521a5a040772cf9aa23a071b4a84361eaf0))
* **poller:** add rate-limit backoff, no-PR backoff, and ETags to WorktreePRPoller ([3f4c5ca](https://github.com/tstapler/stapler-squad/commit/3f4c5caf11a25a6f717c0f15d30663b5ee92abea))
* **process:** reap tmux control-mode children on ungraceful parent death ([2f93900](https://github.com/tstapler/stapler-squad/commit/2f93900cc6cb001bd5a5ac0a45527e52350e2207))
* publish session update event on controller status change ([9241d3e](https://github.com/tstapler/stapler-squad/commit/9241d3e0bcb89c0f65971f64435f4afe4fab1064))
* **registry:** add alias RPCs to scanner methodToID map ([a1ac791](https://github.com/tstapler/stapler-squad/commit/a1ac791b11dfa3d38dfa81ee56106221286b9987))
* repair broken release pipeline and build-from-source path ([#147](https://github.com/tstapler/stapler-squad/issues/147)) ([1fc2ff6](https://github.com/tstapler/stapler-squad/commit/1fc2ff6c90fe88af32f14fa600bdfc03a63fcf54))
* repair main — duplicate actor declarations + orphaned config-file-rules stubs ([#140](https://github.com/tstapler/stapler-squad/issues/140)) ([9fa5059](https://github.com/tstapler/stapler-squad/commit/9fa5059d1da8090680499d35113a9cb912bcf049))
* review queue auto-advance respects preference after approve/deny ([c63886a](https://github.com/tstapler/stapler-squad/commit/c63886aa90a2354e41140a51280ad86eadfc06df))
* **review-gate:** include staged/unstaged changes in GetGitDiff ([c0e57ca](https://github.com/tstapler/stapler-squad/commit/c0e57ca1411f7e814126a896b61aa147031517f3))
* **review-gate:** two PASS/UNVERIFIABLE transition bugs ([98c8e8a](https://github.com/tstapler/stapler-squad/commit/98c8e8a9b4f43042d2e6b887c0425dd2247feebe))
* **review-queue:** resolve UUID→Title before Remove so approved/deleted sessions leave the queue ([ea94659](https://github.com/tstapler/stapler-squad/commit/ea94659ff329737bc7a15fab2bfb9c927a2ed943))
* **review-queue:** show INPUT_REQUIRED items + UX improvements ([757666a](https://github.com/tstapler/stapler-squad/commit/757666aff79b654bcb8a52468ce7a26778ebeb11))
* **review-queue:** suppress auto-advance on session status transitions ([c3fa5e3](https://github.com/tstapler/stapler-squad/commit/c3fa5e3f523d070ccbada952a157a36b67206335))
* **review-queue:** suppress auto-advance on session status transitions ([#135](https://github.com/tstapler/stapler-squad/issues/135)) ([8ec3e9d](https://github.com/tstapler/stapler-squad/commit/8ec3e9d4d170c86ce751858f4b62e6f6caafc2d9))
* **server:** bound shutdown-hook execution to prevent SIGKILL on restart ([9631fd1](https://github.com/tstapler/stapler-squad/commit/9631fd1b8b09267d893324120146bc0247411506))
* **service:** fall back to launchctl load when bootstrap fails on macOS ([46bd0ea](https://github.com/tstapler/stapler-squad/commit/46bd0ea9f5d92153f7bb279f56b2cbc0fcfdca56))
* **session-driver:** add live output check before prompt injection ([aa9834c](https://github.com/tstapler/stapler-squad/commit/aa9834cdc11b87460eb2d706946cc1c4e07eac2f))
* **session:** defer session Start() so the HTTP server binds immediately ([cc0903e](https://github.com/tstapler/stapler-squad/commit/cc0903e554cc14ceb4d5e79c1c765de6d5e70335))
* **session:** hot-attach to already-running tmux session on restore ([7d42363](https://github.com/tstapler/stapler-squad/commit/7d42363cac0e39c243b2bb3572b2f0b57c3cff64))
* **session:** program switching now saves correctly for all cases ([914138e](https://github.com/tstapler/stapler-squad/commit/914138ec162e494f9d90818182c93d3bb0a2ae53))
* **session:** release stateMutex before calling Start() in SwitchWorkspace ([f6f89d8](https://github.com/tstapler/stapler-squad/commit/f6f89d84868fc89e9242f6d3c68541548295dfbc))
* **session:** replace nil,nil return with ErrInstanceDataNotFound sentinel (nilnil lint) ([e2248ba](https://github.com/tstapler/stapler-squad/commit/e2248bac6bf0e8d8efc9a2ad7269dfad2a268525))
* **session:** respawn panes killed under remain-on-exit ([3d7c075](https://github.com/tstapler/stapler-squad/commit/3d7c075677be45b6f7f30faa84954a13817535fc))
* **session:** serialize SwitchProgram to prevent race on concurrent switches ([16bdf0f](https://github.com/tstapler/stapler-squad/commit/16bdf0f5a1447b1ed228a5297e130b80d95a14b0))
* **sessions:** prevent Claude process orphaning after server restart ([da222c2](https://github.com/tstapler/stapler-squad/commit/da222c259e1303a9b084489d550138fbb4539074))
* **session:** stop discarding the program-change save promise, surface save errors inline ([a478983](https://github.com/tstapler/stapler-squad/commit/a47898328421f5080ba0a8e79413269ee4d5576b))
* **session:** stop repeated "1" keystroke on directory-approval prompts ([#146](https://github.com/tstapler/stapler-squad/issues/146)) ([7948dae](https://github.com/tstapler/stapler-squad/commit/7948daebae68406b15d67bb9669e1ac0fc7eef6e))
* skip shell sourcing in test mode to prevent service test timeout ([49eb0ba](https://github.com/tstapler/stapler-squad/commit/49eb0ba5dab6438f9796f36fa6ff8da55b10875a))
* **terminal:** correctly scan OSC/DCS escape sequences to stop render artifacts ([#156](https://github.com/tstapler/stapler-squad/issues/156)) ([2151b4b](https://github.com/tstapler/stapler-squad/commit/2151b4b2186627c64c1be1617e4e096918d5b491))
* **terminal:** repair escape code pipeline for new Claude Code renderer ([#139](https://github.com/tstapler/stapler-squad/issues/139)) ([dc71b82](https://github.com/tstapler/stapler-squad/commit/dc71b828a6d6a61883cd12b7c666e836eda97c90))
* **test:** prevent tests from spawning a real, unbounded claude agent ([cd124e7](https://github.com/tstapler/stapler-squad/commit/cd124e7bfde6b863cd382b60a82155574c3c40f1))
* **tests:** recognize shared isolated tmux socket in leaked-server reaper ([f3b7ce4](https://github.com/tstapler/stapler-squad/commit/f3b7ce43018f825fb6f13763633de10114ba2201))
* **tmux:** always include session UUID in MCP HTTP config header ([63e21a7](https://github.com/tstapler/stapler-squad/commit/63e21a7743a7b9470942c370d17d872bdd5a907f))
* **tmux:** coalesce concurrent DoesSessionExistNoCache calls ([a9616bf](https://github.com/tstapler/stapler-squad/commit/a9616bfea47eea2268db1b27591e45a2fe9f2a0b))
* **tmux:** flush stale exists-cache in RestoreWithWorkDir and add --tmux-keep-server to plist ([12b992a](https://github.com/tstapler/stapler-squad/commit/12b992a32ca5e9997f6b042e719bc49ef99eec91))
* **triage:** silent storage error, hung session timeout, and Claude detection false positives ([cc82510](https://github.com/tstapler/stapler-squad/commit/cc8251090c0d6987e8f439fb1ba7b8cbce5d41e7))
* **ui:** prevent N×5 dispatch storm freezing home page on load ([6d0859d](https://github.com/tstapler/stapler-squad/commit/6d0859d26265c6f4c11cf12f9e1a3371f3635090))
* **unfinished:** stack GitHub auth banner vertically so Connect button is always visible ([750d252](https://github.com/tstapler/stapler-squad/commit/750d252092abf455d7c20ad8ad143486e28674d2))
* **ux:** address 8 UX review findings from 2026-06-30 ([4780b94](https://github.com/tstapler/stapler-squad/commit/4780b94df04df4fc3334bd0d238a269601a95574))
* **vcs:** periodically clear go-git repo cache to bound memory usage ([6a3e5dd](https://github.com/tstapler/stapler-squad/commit/6a3e5dd13811bd4416468ba67b0f656c385ac0f8))
* web-build target doesn't generate proto bindings on a clean clone ([#155](https://github.com/tstapler/stapler-squad/issues/155)) ([1bb135e](https://github.com/tstapler/stapler-squad/commit/1bb135e8d93456c70db12ceb77b9f335369925f9))


### Performance Improvements

* **mcp:** make stapler-squad --mcp a thin proxy of the running server ([6d6d1aa](https://github.com/tstapler/stapler-squad/commit/6d6d1aa8aa5747a9e6fd648fe121e0a3a1bdfff0))
* **tmux:** add semaphore to cap concurrent capture-pane subprocesses ([fa54896](https://github.com/tstapler/stapler-squad/commit/fa548964a70cc757ac46a5d49b810ca389b53f90))
* **unfinished:** cut go-git diff scanner allocations and bound worst-case cost ([#153](https://github.com/tstapler/stapler-squad/issues/153)) ([a45fa20](https://github.com/tstapler/stapler-squad/commit/a45fa20f6ed92afae1f928c381edeeaf71221c56))
* **vcs:** cache reachableSet results and batch-read blobs under single lock ([125f4ef](https://github.com/tstapler/stapler-squad/commit/125f4ef7e3f32f0a1f05c213cd394e4ab2e5e3bd))


### Reverts

* remove incorrect InjectMCPConfig from SpawnSessionFromItem ([0595394](https://github.com/tstapler/stapler-squad/commit/05953949b118b91c27ea892126b086283fb6e8fe))
* remove incorrect per-session hook injection in session_service ([cfdf4ff](https://github.com/tstapler/stapler-squad/commit/cfdf4ffb2d2ded1fb4604742da83f20ab44378c7))

## [1.36.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.35.0...v1.36.0) (2026-07-10)


### Features

* **analysis:** add gVisor checklocks + annotate critical mutex-protected fields ([bea8cde](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bea8cde8d627a3aa2bbb6fc2e08114eff52e3819))
* **tmux:** add Socket value type and tmuxsocketscope lint pass; fix remaining gaps ([5b61c3e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5b61c3e7fb9460ad1167bf3c89ebf0fc0fbb2cc3))


### Bug Fixes

* **build:** download tmux configure from release tarball, add ensure-tmux-configure target ([5612c89](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5612c892b91c528542fc842e86e45a3b94b3c196))
* **server:** stop integration tests from killing production tmux sessions ([bf9e78e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bf9e78e96bc584f45f0c098d6c681d709ba81a02))
* **session:** resolve tmux multi-socket reconcile bug and hibernation resurrection bug ([666d8ad](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/666d8ad39d1f3b13c474f5bb084595107f353cf4))
* **tmux:** close remaining gaps in per-process tmux socket isolation ([8885672](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/88856728d2d0f7c5f2b9f27a3e5b9440d90876ec))


### Performance Improvements

* **adapter:** compute GetStatus once per InstanceToProto instead of 3x ([4836f7a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4836f7abfb933f2df2a3babf6a4eebef63ada8a3))
* **analytics:** drop hex.EncodeToString per parsed escape sequence ([81fc691](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/81fc691c9d05fd4bdb67447bcb13f70c15565177))
* **analytics:** eliminate string allocs in charset and SGR description paths ([7d463f6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7d463f6423d3b3eba5037a2a5b7cfe0d8295ee9b))
* **analytics:** replace fmt.Sprintf string keys with zero-alloc struct keys ([b7f7807](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b7f7807e5c8fb7707d7b59afdf73103d9a1ee0f5))
* **analytics:** return ParsedEscapeCode by value to eliminate per-sequence heap alloc ([f350b4c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f350b4c5dea98065f79cdee544917bda77ea32d2))
* **analytics:** skip all extraction when escape writer is Noop (no DB configured) ([163dbe2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/163dbe21642c4d89f07995c526871be5cafd8fbd))
* **analytics:** skip escape extraction in Parse when captureLevel is off ([7ebce9f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7ebce9fa439447301d50108331626c565ba30510))
* **analytics:** skip mutex in EscapeCodeStore.Record when store is disabled ([e88d819](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e88d819ee3404d0a54cf97e1aeb90f3b27170bdd))
* batch tag updates, eliminate bytes.Buffer in decode, strings.Cut on hot path ([3b121f9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3b121f9114e4be89c8552c9a8e8d54f08a5e53a5))
* **buffer:** make TotalBytesWritten lock-free via atomic.Int64 ([a5e8a0d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a5e8a0d09d4941a8dd8e78a1a0a382a80144d304))
* cache capture-pane/PanePID, lazy-load SessionDetailView, memoize SessionRow ([8d816ea](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8d816ea806bb15d9eed2f8b700496158e2154abf))
* cache GetRemoteURL, GH keychain token; increase IsDirtyCacheTTL to 30s ([74e343b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/74e343b8c0feedf7ef84b4c0be664b153d0bc5ef))
* cacheMu Mutex→RWMutex in pollerContentProvider for concurrent cache reads ([bb3cb6b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bb3cb6b7b72b4e0459e011b26cd47e94ebc13196))
* **cache:** replace 4-map+RWMutex with xsync.MapOf in pollerContentProvider ([13f3230](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/13f32303f492d574a0ebab9fb6ebc5549f28c1a7))
* **cache:** replace Locked[cacheState] with atomic.Pointer in hot poll path ([0cbcab4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0cbcab45750b918294a6c9371571e7fd425e4371))
* **cache:** skip 4KB alloc on status cache hit via GetRecentHash ([79525c6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/79525c60a8cf0a717c6ad8307140a69a435ff1e7))
* **controller:** add started atomic.Bool + filterTmuxMetadata fast path ([7259e6b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7259e6ba987c06480959fd4fa6aa46849b6b6ca7))
* **controller:** fix GetRecentOutput(n) to read only n bytes, not full 10MB buffer ([f1c6580](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f1c6580a416b523defa3d304d26a2f50b5bc0f2e))
* **detect:** eliminate string(raw) alloc in hasScreenOverwrite ([5012711](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/50127111c88ef61619e219beba6c21b444d83f5f))
* **detection:** remove PatternSet mutex — struct is immutable after construction ([54dc79c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/54dc79c14a30b1395c7aecd242c2abbcc736a54b))
* **detection:** replace patternSet RWMutex with atomic.Pointer ([92e5151](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/92e51515ee1fb8f814408948dfb3ca4e81361159))
* **detect:** narrow lock scope in DetectStateFromContent/DetectState ([66633e0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/66633e0cd45089cca222107e1ff9833ac3b534cd))
* **detect:** scan lastNLines backward to avoid splitting discarded prefix ([e470e15](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e470e15f475ccb6eb1b778433189a85f4fe60ef0))
* **detect:** skip []byte round-trip in detectFromLines via string variant ([a4d24a9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a4d24a9bf58b3665e342606a89f1a5823cdadc66))
* **detect:** use IndexByte instead of ContainsRune for ASCII '\r' scan ([ea572e6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ea572e6ecacebc90a996883fe78e29458584e4fa))
* **dirty:** replace sync.RWMutex+2fields with atomic.Value on IsDirty fast path ([2ce0f7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2ce0f7e135ecf0df57fffde4db0587a94cf7c947))
* GetQueuedCommandsCount avoids slice alloc, drop Sprintf on hot status path ([c04db64](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c04db6414a23b7330d3e55f4c22722287472970d))
* **hash:** avoid []byte copy in ComputePromptSignature via unsafe.StringData ([c286c3e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c286c3e43c78155c4792ca2cc6ec2857d2f072db))
* **hash:** replace FNV+alloc with murmur3 for status cache keys ([0ea1f63](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0ea1f6305f7e67e9a4e507202884e70f3ee8104a))
* HasMeaningfulContent strings.Cut iteration, zero alloc vs strings.Split ([50c0256](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/50c0256641a94d9ef600d70c95d14a973c6b68ae))
* **idle:** replace GetLastActivity RLock with lock-free atomic read ([e319a89](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e319a89bf393f97adf17cf69cd67ad4d14486652))
* **idle:** skip mutex in RecordActivity via atomic debounce check ([9bc90b3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9bc90b3168ccf795961d9e2c1b12047fdaa07420))
* **lock:** replace deadlock.RWMutex with sync.RWMutex on hot paths ([f3e3300](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f3e3300990a2411637b900c14e67a5345b9f653a))
* **log:** demote hot-path log.Info to log.Debug in checkSession ([43182e8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/43182e8c39f45faa65e09960a728d65929704713))
* **pool:** eliminate 4KB alloc on status-cache miss via sync.Pool ([7c21eee](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7c21eee6250d4779fee01f90661581f6263ca10d))
* **pty:** remove outer RLock from buffer accessors on immutable field ([346cffe](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/346cffe4aa4ef96be17e68208b0a67f71c2f8f10))
* replace InfoLog.Printf with log.Debug in hot determiner path ([06b7279](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/06b7279d1307b7cade83185ae872e0cd88409adb))
* **server:** cache available programs at startup, not on every server-info request ([82b9cec](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/82b9cecf1cf5a8fd8f216cf1f4669622b189db5c))
* **session:** replace ControllerManager RWMutex with atomic.Pointer ([a1fe6f9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a1fe6f9208c7ba66aaabeadb75325d01a28ae63a))
* **session:** replace TmuxProcessManager session RWMutex with atomic.Pointer ([fbe7f3a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fbe7f3a7a67e7d72e5f6020f17062a7754da81f0))
* **snapshot:** skip buildSnapshot when terminal timestamps unchanged ([4c52474](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4c5247411a0b8197748ecf751a48b47eb15c6a3b))
* **status:** combine GetCurrentStatus+GetIdleState into one hash+cache read ([103e6a4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/103e6a4d169585825556d3cb502bdbc3e22df85a))
* **status:** replace InstanceStatusManager map+RWMutex with xsync.MapOf ([c7692fa](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c7692fa5dce141be4e16dc353e4d52f69b0176ca))
* **stream:** eliminate per-read mutex by reading exitTail from circular buffer ([97b91a1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/97b91a1742c081df78972bd41d00c4cecb7dac8f))
* **streamer:** replace byte-at-a-time ringBuffer loop with bulk copy ([2f5e906](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2f5e9068e080a9ee4fee49e027e27386ee5a1fdd))
* **stream:** skip ResponseChunk alloc in broadcast when no subscribers ([17669b9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/17669b9739fc11f82a3f669be948a03114d71ae1))
* **tmux:** atomic.Value+singleflight for DoesSessionExist; fix lock-held-across-I/O ([ef8d478](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ef8d4789806b9af267bb60ae0d7afec8de786f4d))
* **tmux:** eliminate scanner.Text() allocation on %output hot path ([e093c9b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e093c9bda5a5ce3090491675dcae0db70a8ae2b7))
* **tmux:** replace strconv.ParseUint with lookup table in decodeControlModeOutput ([88f9f6e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/88f9f6e7c397fe0193f096b7528ba2991a08cd39))
* **tmux:** use RLock in broadcastControlModeUpdate ([249dd63](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/249dd638c79b7e95fe1667d02390307e500f18f3))
* variable dirty-cache TTL, eliminate N+1 metadata query, O(1) queue empty check ([dd2f088](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dd2f0882ab6a7fd07df0ddee0126b173751549fb))
* **vc:** cache GetBranch result 30s to eliminate per-RPC fork lock contention ([d3441f2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d3441f20012f5af464608031fe5a590cb604e346))
* **vcs:** cache VCSStatus per workdir for 15s to eliminate repeated git subprocesses ([33d3c8c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/33d3c8c9afc5aad6e4e7b31fe73e90ff262d10a4))
* **ws:** pool coalesce buffer to eliminate 1 more alloc/frame ([aea5933](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/aea59338802c9b1d9be9f497fe67ae5c3ff25013))
* **ws:** pool envelope buffer + marshal direct to avoid 2 allocs/frame ([2327d57](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2327d57b849ca62ad7e07ca94b2f8e3920005bc3))
* **ws:** replace snapshotCache map+RWMutex with xsync.MapOf ([e9baedd](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e9baedde5c62955f8379917da69ab2b56f1e1b7c))

## [1.35.0](https://github.com/tstapler/stapler-squad/compare/v1.34.0...v1.35.0) (2026-07-04)


### Features

* **agy,opencode:** add comprehensive CLI parity for Antigravity and OpenCode ([#135](https://github.com/tstapler/stapler-squad/issues/135)) ([38d8a29](https://github.com/tstapler/stapler-squad/commit/38d8a297b67d1fdbb13811a12ce0d7ddb5273222))
* **alias:** add name_prefix field + fix session name oscillation ([9c730be](https://github.com/tstapler/stapler-squad/commit/9c730bec10bbb84e7a8e3ab4a48015b542057602))
* **analytics:** bulk rule creation + page density improvements ([7ae2910](https://github.com/tstapler/stapler-squad/commit/7ae2910e41afd07b0ec1fa3495426a3f8a62d055))
* **analytics:** program detail panel + copilot review fixes ([2774602](https://github.com/tstapler/stapler-squad/commit/2774602a7b4f4cd89baf5d3f8abcabfa21e6a052))
* **analytics:** program detail panel with subcommand drill-down ([29c7af8](https://github.com/tstapler/stapler-squad/commit/29c7af8c25f81b935b048e8a23e27eab06e11a48))
* **analytics:** unify activity tables into single filterable view ([36de4b8](https://github.com/tstapler/stapler-squad/commit/36de4b80f50681cf33e070348eca34f40a457dd5))
* **approvals:** dismiss notifications when user sends terminal input ([#101](https://github.com/tstapler/stapler-squad/issues/101)) ([8a15753](https://github.com/tstapler/stapler-squad/commit/8a15753be0790197a5ffdcd6298fad3b32b15613))
* **artifacts:** extract PR URLs, commits & external URLs from JSONL history ([#129](https://github.com/tstapler/stapler-squad/issues/129)) ([bfe06a5](https://github.com/tstapler/stapler-squad/commit/bfe06a543ce070f5e87b7a501ccd0dda1a2221c2))
* **autonomous:** P0 foundation — AutonomousDriver + StatusChangeListener fan-out ([#105](https://github.com/tstapler/stapler-squad/issues/105)) ([95b7c69](https://github.com/tstapler/stapler-squad/commit/95b7c696df5ca3c0eea6f0e81d6c56ac664ba978))
* **autonomous:** triage migration, review queue UX, backlog MCP tools, session detail polish ([633e88d](https://github.com/tstapler/stapler-squad/commit/633e88d7b56d9bf8d9077c3f2109aadaf46b383a))
* **backlog:** finish GitHub sync — TriggerSync/GetSyncHistory RPCs, tests, settings UI ([#138](https://github.com/tstapler/stapler-squad/issues/138)) ([9c5927b](https://github.com/tstapler/stapler-squad/commit/9c5927b2551c8fced04723f010aba5424cec97b2))
* **backlog:** gate backlog behind feature flag on all layers ([66b1d83](https://github.com/tstapler/stapler-squad/commit/66b1d831c9d8bcc0df3d9ee654d2fd8822165283))
* **backlog:** gate backlog behind feature flag on all layers ([1186494](https://github.com/tstapler/stapler-squad/commit/1186494baa7d7f9429030f7703ad1e6e4b7e7132))
* **detection:** add StatusWaitingForAgent for background agent state ([#115](https://github.com/tstapler/stapler-squad/issues/115)) ([dddb62d](https://github.com/tstapler/stapler-squad/commit/dddb62d555e2752abfca5d25ce79c946580bf6aa))
* **detection:** detect monitors-still-running as WaitingForAgent ([#121](https://github.com/tstapler/stapler-squad/issues/121)) ([995d065](https://github.com/tstapler/stapler-squad/commit/995d065c81d059fbc7de93304efe56ca407af351))
* **detection:** fix session status indicators + observability ring buffer ([#110](https://github.com/tstapler/stapler-squad/issues/110)) ([f9f3079](https://github.com/tstapler/stapler-squad/commit/f9f30798eb9bb44a0da3a46f3593234061d347b0))
* **detection:** surface WAITING_FOR_AGENT as distinct SubStatus chip ([#126](https://github.com/tstapler/stapler-squad/issues/126)) ([e5f80e8](https://github.com/tstapler/stapler-squad/commit/e5f80e8bf4b691212b9524cfcab216caf5e3a43a))
* **events:** fix event pipeline gaps for cross-surface state consistency ([#120](https://github.com/tstapler/stapler-squad/issues/120)) ([016b558](https://github.com/tstapler/stapler-squad/commit/016b5584b5005363c7160e95181f307224264421))
* **files:** local file browser with iframe/SVG rendering ([#113](https://github.com/tstapler/stapler-squad/issues/113)) ([e55ada1](https://github.com/tstapler/stapler-squad/commit/e55ada1df4ed8a157b21949b74c9852823cf7e05))
* **files:** wire up the premium LocalFileBrowser component to the files page ([3743b94](https://github.com/tstapler/stapler-squad/commit/3743b945cb6085d160ed5734361b99442a66d9dc))
* gap-finder workflow + SESSION_STATUS_RESTORING ProtoToStatus fix ([#123](https://github.com/tstapler/stapler-squad/issues/123)) ([45a5a91](https://github.com/tstapler/stapler-squad/commit/45a5a91d53480f500e00fa584e4db7b115edf309))
* **harness:** headless triage test harness + alias kebab-case fix ([7ad3340](https://github.com/tstapler/stapler-squad/commit/7ad33400c273dcc3dc1ec6a210e3cdc4509fa2cf))
* **history:** Stories 3–5 — rich cards, virtual scroll, fork modal ([b61cfac](https://github.com/tstapler/stapler-squad/commit/b61cfacd68b2d07308c5ed38ce0ee5edffdb1a98))
* **history:** Story 1 — proto fields, git enrichment, fork dispatch ([711d5fe](https://github.com/tstapler/stapler-squad/commit/711d5fe0a84045407d877d0d60b4c4ad8ca92399))
* **insights:** time range filter, session detail, cost projections, perf ([#98](https://github.com/tstapler/stapler-squad/issues/98)) ([25e0653](https://github.com/tstapler/stapler-squad/commit/25e06533f12a0bf5df1096456f08bc384172c4ca))
* local file browser + detection readline fixes ([#116](https://github.com/tstapler/stapler-squad/issues/116)) ([5ce6c1c](https://github.com/tstapler/stapler-squad/commit/5ce6c1c0914c1e774b851f2ac8025a21fb7208f1))
* **nav:** group navigation into 4 sections, restore mobile access ([a2f78a3](https://github.com/tstapler/stapler-squad/commit/a2f78a38698a4a8414d1640a6b0c16795c527418))
* **omnibar:** add [@alias](https://github.com/alias) session presets ([#122](https://github.com/tstapler/stapler-squad/issues/122)) ([a65f7d2](https://github.com/tstapler/stapler-squad/commit/a65f7d2ebfd49239c5436f01e2978984dfa4d3bb))
* **omnibar:** alias name_prefix field and trust-dialog double-fire fix ([#128](https://github.com/tstapler/stapler-squad/issues/128)) ([7dbfb10](https://github.com/tstapler/stapler-squad/commit/7dbfb10c27524319ad63e918a3f4ea704b309818))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([670caef](https://github.com/tstapler/stapler-squad/commit/670caeffd64e6e89aaf13df26c692580aa4effe0))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([250e640](https://github.com/tstapler/stapler-squad/commit/250e64071b9ddb72d44ca85cd6f620988f6bfb33))
* **omnibar:** replace Create shortcut hint with clickable Create Session button ([bc794b7](https://github.com/tstapler/stapler-squad/commit/bc794b7fc4e88b39dbc86ece70a5c086e48d47b6))
* **pane:** add session peek modal triggered from pane picker ([9bf226a](https://github.com/tstapler/stapler-squad/commit/9bf226a7564e54b36b433cc52a359c573887cd14))
* **reconnect:** jittered backoff reconnect for session-watch and terminal streams ([#136](https://github.com/tstapler/stapler-squad/issues/136)) ([837ba8c](https://github.com/tstapler/stapler-squad/commit/837ba8cc70eb6f637c2b3bab2a4b27838e1d1113))
* **registry:** typed feature catalog replacing comment-based annotations ([#112](https://github.com/tstapler/stapler-squad/issues/112)) ([6232826](https://github.com/tstapler/stapler-squad/commit/6232826397e21f803d4af4bc01af481b7ac39545))
* **rules:** structured rule builder, template library, analytics→rule workflow ([#103](https://github.com/tstapler/stapler-squad/issues/103)) ([48adec6](https://github.com/tstapler/stapler-squad/commit/48adec6b2e47301106ea1b9192828fea445e83b2))
* **rules:** YAML import/export and UX improvements for ApprovalRulesPanel ([#123](https://github.com/tstapler/stapler-squad/issues/123)) ([ea92de7](https://github.com/tstapler/stapler-squad/commit/ea92de7fc83f8b060eabb2d5425c9f13233bc1cd))
* **server:** startup fast restore, terminal snapshot fix, build warnings ([#128](https://github.com/tstapler/stapler-squad/issues/128)) ([e158b30](https://github.com/tstapler/stapler-squad/commit/e158b305f3bb7b19e9a81e772f082140043df82a))
* **services:** extract CheckpointSvc, TerminalSvc, FeatureFlagSvc; delegate ListBranches+workflow (CDD Epic 1) ([8936b3a](https://github.com/tstapler/stapler-squad/commit/8936b3afc3ad039de23ee952e80979efdd45ca5a))
* **session-list:** bulk selection for row mode with undo-on-delete ([#137](https://github.com/tstapler/stapler-squad/issues/137)) ([3571d03](https://github.com/tstapler/stapler-squad/commit/3571d037fa81e3f3b1a804da21dbb6adbf7376ba))
* **session:** actor goroutine + mailbox + sendSync + goleak verification (IAC Epic 3) ([8adc0c9](https://github.com/tstapler/stapler-squad/commit/8adc0c9be7c2699fc506b365ba4f0bdd45e18790))
* **session:** add 30-min background reaper for paused-session tmux processes ([66e33e2](https://github.com/tstapler/stapler-squad/commit/66e33e24b88bf91153f1d4b935ce9cfdd63ff224))
* **session:** bidirectional session history transfer between Claude and Antigravity ([#130](https://github.com/tstapler/stapler-squad/issues/130)) ([08d3f87](https://github.com/tstapler/stapler-squad/commit/08d3f87b06a954acff5ab0a567defe97db407bed))
* **session:** enable/disable autonomous mode on existing sessions ([4f2967c](https://github.com/tstapler/stapler-squad/commit/4f2967cfb604880a4ede96e7e7c2cf0db90a10c3))
* **session:** IAC Epic 4 — actor-path state-machine core migration ([7bcc479](https://github.com/tstapler/stapler-squad/commit/7bcc479642a682671acd26d80cb618779614e418))
* **session:** InstanceSnapshot + atomic.Pointer read path, snapshot publish in all mutators (IAC Epic 1) ([e8cfafc](https://github.com/tstapler/stapler-squad/commit/e8cfafc44a56c217b4bfdb03a69b418ea8124ecd))
* **session:** migrate all unguarded readers to snapshot.Load() (IAC Epic 2) ([f491483](https://github.com/tstapler/stapler-squad/commit/f491483a76b7b5ff1fef2d6dc2131ff882ebe5ba))
* **session:** Registry + LiveInstance type-split lifecycle layer (IAC Epic 2.5) ([4232806](https://github.com/tstapler/stapler-squad/commit/42328063146596b88000b022ce0022f9e6341653))
* **sessions:** fix InitialPrompt injection, add session goal tracking MCP + UI ([#99](https://github.com/tstapler/stapler-squad/issues/99)) ([5132c3f](https://github.com/tstapler/stapler-squad/commit/5132c3fb4f7c1ae08bcb1b32cb4b510a9c7b948e))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([b695764](https://github.com/tstapler/stapler-squad/commit/b695764e9a5d12827da7db0815d81db262288dd9))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([f5f7f36](https://github.com/tstapler/stapler-squad/commit/f5f7f3618271b62783ca906274a5d4cac8eae13c))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([#124](https://github.com/tstapler/stapler-squad/issues/124)) ([e54f41b](https://github.com/tstapler/stapler-squad/commit/e54f41b012370f4f3620e2fe64cad122230fc66e))
* **status:** unify session status vocabulary across UI ([c42514d](https://github.com/tstapler/stapler-squad/commit/c42514d4020c41c6ad16ec2f546846e257a81563))
* support Antigravity CLI hooks.json format in ssq-hooks ([55df54d](https://github.com/tstapler/stapler-squad/commit/55df54d4f60be8cddf6082c8ed64621f60db26de))
* workflow run history, session archiving, slash commands, and pause memory optimization ([9c23e61](https://github.com/tstapler/stapler-squad/commit/9c23e6126664bc7faf458babbefc1fc7069e5c99))
* **workflows:** Quick Workflows — [@slug](https://github.com/slug) omnibar, management UI, cron scheduling ([#106](https://github.com/tstapler/stapler-squad/issues/106)) ([883deca](https://github.com/tstapler/stapler-squad/commit/883deca379d7529f921d654b6671c559c7766ab9))
* **workflows:** session affordances, retention enforcer, bulk-session RPCs ([ac4efa1](https://github.com/tstapler/stapler-squad/commit/ac4efa121abdb34649470cc16133e0a04872e20e))


### Bug Fixes

* address copilot review comments on analytics drill-down ([077f53f](https://github.com/tstapler/stapler-squad/commit/077f53f6e075ef467f61022a3be28321f56cc387))
* **alias:** default session type + name oscillation ([4d33c7d](https://github.com/tstapler/stapler-squad/commit/4d33c7d50c4cd66ed4cbbd585d9556329259f050))
* **approval:** inject PermissionRequest hook on CreateSession and RestartSession ([8238a64](https://github.com/tstapler/stapler-squad/commit/8238a64c57033833615cfa7dc83c26f2db57bd0b))
* **backlog:** address architecture-review follow-ups from post-merge audit ([#142](https://github.com/tstapler/stapler-squad/issues/142)) ([7791548](https://github.com/tstapler/stapler-squad/commit/7791548c1f3b633033975cdc4b2d6b44f0a7fc17))
* **backlog:** harden claude binary lookup against a stale Linux PATH ([#136](https://github.com/tstapler/stapler-squad/issues/136)) ([6839082](https://github.com/tstapler/stapler-squad/commit/68390826f0a22d380c04a1163c65017a113e8583))
* **backlog:** harden triage JSON parser against stray braces in preamble ([#134](https://github.com/tstapler/stapler-squad/issues/134)) ([fd56d6a](https://github.com/tstapler/stapler-squad/commit/fd56d6ae1e5fa73a86c42b2ef245c0ca7d10fba3))
* **backlog:** harden triage parser and add repoPath UI gate ([06f9857](https://github.com/tstapler/stapler-squad/commit/06f98573b53af59afab38dfb0684edcc59210531))
* **backlog:** replace idle triage sessions with headless pool calls ([#127](https://github.com/tstapler/stapler-squad/issues/127)) ([2d7e116](https://github.com/tstapler/stapler-squad/commit/2d7e116ca3f624a891babb2e27aa447e904c50be))
* **backlog:** TriggerTriage's cleanupCtx expired before it was ever used ([#137](https://github.com/tstapler/stapler-squad/issues/137)) ([6552ec9](https://github.com/tstapler/stapler-squad/commit/6552ec9039b1a9f364e874e552ecb9cea7f04530))
* **bench+lint:** gofmt PR[#101](https://github.com/tstapler/stapler-squad/issues/101) files; replace rebase with reset+recommit for baselines ([bf39276](https://github.com/tstapler/stapler-squad/commit/bf3927633606ae12218811e6a9d1c745a76f9682))
* **bench:** benchmark gate under 10 minutes — fix 3 root causes ([d7157f5](https://github.com/tstapler/stapler-squad/commit/d7157f50601c0a50ed86ce40068eff4c92e88ce4))
* **bench:** force-add gitignored baseline file; add retry + rebase on push ([f940c0a](https://github.com/tstapler/stapler-squad/commit/f940c0a12d19a69ee01da32cdd9c1bdf905d77a8))
* **bench:** make benchmark flows robust — 10 fragility fixes ([e69c0bd](https://github.com/tstapler/stapler-squad/commit/e69c0bd52580f61126edb779855ece3f05fd88b6))
* **ci:** add push retry loop to demo GIFs workflow ([e6a6f54](https://github.com/tstapler/stapler-squad/commit/e6a6f54a87b7925b725aff0bf3d524ee263795a5))
* **ci:** force-add bench-baseline.txt to bypass .gitignore in benchmark gate ([ebd4278](https://github.com/tstapler/stapler-squad/commit/ebd4278098639893694f192f457fb214ed481257))
* **ci:** resolve Benchmarks race, Demo GIFs 403, and Release Please body parse ([d30a2db](https://github.com/tstapler/stapler-squad/commit/d30a2db48a8790248239711c3e59015214242c26))
* **ci:** resolve pre-existing lint-css-tokens and lint-custom failures ([b959669](https://github.com/tstapler/stapler-squad/commit/b95966947df405db4525c270b8edd94b2a4d009a))
* **codesign:** auto-run setup-codesign and fix OpenSSL 3 PKCS12 compatibility ([de6c7a6](https://github.com/tstapler/stapler-squad/commit/de6c7a62ca8ddb61ce728d2094e59808bac8a3d6))
* **codesign:** install binary to ~/.stapler-squad/bin/ before signing to prevent sealed-resource invalidation ([6c539c3](https://github.com/tstapler/stapler-squad/commit/6c539c31202e181c93b5905d929d5a49067c6591))
* **db:** make approval_rule JSON fields Optional to fix SQLite migration ([3f06c42](https://github.com/tstapler/stapler-squad/commit/3f06c420517312b4d7bfed3ae5c4201a9c3c9c90))
* **deps:** upgrade gopsutil v3 to v4 to drop crashing go-m1cpu cgo init ([#129](https://github.com/tstapler/stapler-squad/issues/129)) ([69bc09f](https://github.com/tstapler/stapler-squad/commit/69bc09fa3564ecd7d965fc51386311c5b49bf79d))
* **detection:** detect dynamic workflows + expand turn-marker to ✦ ([2fad2c0](https://github.com/tstapler/stapler-squad/commit/2fad2c0eacc453b5a729b05d185774007622095f))
* **detection:** detect indented spinners and CR-overwritten esc-to-interrupt ([#108](https://github.com/tstapler/stapler-squad/issues/108)) ([54a5f63](https://github.com/tstapler/stapler-squad/commit/54a5f637a3859dc901d521bb3f9861032616ab63))
* **detection:** detect WaitingForAgent when esc-to-interrupt appears below spinner ([44b455a](https://github.com/tstapler/stapler-squad/commit/44b455a751cc3907a9ce3fdb3b2c945929ba6a25))
* **detection:** detect workflow approval dialog and idle-while-typing states ([63c8ca9](https://github.com/tstapler/stapler-squad/commit/63c8ca9cff55e40879a08954dcfd1c1a528c5dc1))
* **detection:** fix ⌛ Thinking chip missing on most active sessions ([375f7b0](https://github.com/tstapler/stapler-squad/commit/375f7b0419a0d2011aedc9576aeaab27a90e4710))
* **detection:** use bottom-up scan in review queue for no-controller sessions ([5d2ba8f](https://github.com/tstapler/stapler-squad/commit/5d2ba8f85acaa209f96ae3d6ab70d9d3ef49688a))
* **github:** prevent data race in UserPRCache.Start() via sync.Once ([9ec03fb](https://github.com/tstapler/stapler-squad/commit/9ec03fb5ce53102cc7243e8dfd946063f175e2bb))
* **headless:** update argsCapturingRunner.Run to match ClaudeRunner interface ([4678065](https://github.com/tstapler/stapler-squad/commit/4678065a591a625a39f3239e071c12e53b9e4b0a))
* **headless:** use Setsid instead of Noctty for headless runner subprocess ([095e09e](https://github.com/tstapler/stapler-squad/commit/095e09e33e2d01b9164cc5e0da707ed9fa83f35f))
* **history-linker:** preserve paused/hibernated session UUID on rescan ([#118](https://github.com/tstapler/stapler-squad/issues/118)) ([fb0e49f](https://github.com/tstapler/stapler-squad/commit/fb0e49f723385a12a77f47fa67157c02cd3790e3))
* **history:** Story 2 — fix broken resume (path validation + session type) ([5626e19](https://github.com/tstapler/stapler-squad/commit/5626e19cd9741881be63f432135ce6b8adf7ad37))
* **history:** virtual scroll layout + mark epic complete ([23f7ba0](https://github.com/tstapler/stapler-squad/commit/23f7ba00ed7214dabaf8e22d3c80dd5d7ea5a607))
* **install:** fix macOS FDA flow and health-check timeout on first install ([38c98b5](https://github.com/tstapler/stapler-squad/commit/38c98b518a383d4aee9076420d87382e5701f843))
* **install:** increase health-check timeout to 120s for session-restore latency ([f64eea0](https://github.com/tstapler/stapler-squad/commit/f64eea0bd4d55fd8b6cfee1def6bb0b2db253710))
* **install:** require launchctl bootstrap; remove launchctl load fallback ([9d3c503](https://github.com/tstapler/stapler-squad/commit/9d3c503ec531b78a5f5808623bdd1aecec1f7c49))
* **lint,status:** fix ESLint errors on main and complete status descriptions ([5635c56](https://github.com/tstapler/stapler-squad/commit/5635c567e7c2b5ce9606e15b42b5fbe166cda2ba))
* **lint:** add concurrency:1 to avoid golangci-lint v2.11.4 race on go1.26 deps ([43110eb](https://github.com/tstapler/stapler-squad/commit/43110ebdf64d99b3ebe3bd6f1fc5adefeee190b9))
* **lint:** analytics-exempt line order; chore(ci): make demo GIFs manual-only ([1255f16](https://github.com/tstapler/stapler-squad/commit/1255f168a5ecbebc7c1d3ad88c58fdd1e0401711))
* **lint:** eslint-disable-next-line for forwardRef inside useMemo object ([5a956bb](https://github.com/tstapler/stapler-squad/commit/5a956bb5cea4eeaa84569e4b0e90adc184ef35c0))
* **lint:** gofmt rules_service_test.go ([cd3c4b1](https://github.com/tstapler/stapler-squad/commit/cd3c4b1628e9b8ffa6daf487f31695573b7bf8ec))
* **lint:** move useMemo above early returns in insights charts; add displayName ([902cab9](https://github.com/tstapler/stapler-squad/commit/902cab979f2b8ff9db584581fd36894d2d9e4452))
* **lint:** replace inline layout style with existing entryList class ([1a74ee5](https://github.com/tstapler/stapler-squad/commit/1a74ee534295c870bd73e8de670de205f952d6e7))
* **lint:** return empty map instead of nil in GetAllInstanceArtifacts ([c8b9780](https://github.com/tstapler/stapler-squad/commit/c8b97803aa46a02fa3b801ee073f1f45a2232a0e))
* **nav:** gate Backlog nav item behind feature flag ([3c2f739](https://github.com/tstapler/stapler-squad/commit/3c2f739a1e10790827c99d04d5080ae343a9b029))
* **notifications:** auto-resolve stale toasts and Chrome notifications ([#109](https://github.com/tstapler/stapler-squad/issues/109)) ([31fd160](https://github.com/tstapler/stapler-squad/commit/31fd160c667b018a817e60b2b8cb0853d831084d))
* **pane:** restore session peek modal integration in pane picker ([6aef840](https://github.com/tstapler/stapler-squad/commit/6aef84015c5dba709b17f812b8b1810abb601f5e))
* **registry:** add alias RPCs to scanner methodToID map ([a1ac791](https://github.com/tstapler/stapler-squad/commit/a1ac791b11dfa3d38dfa81ee56106221286b9987))
* **registry:** add workflow/detection RPCs to scanner methodToID map ([24388a0](https://github.com/tstapler/stapler-squad/commit/24388a079da33430e2d880e0b0647c8124b27388))
* repair main — duplicate actor declarations + orphaned config-file-rules stubs ([#140](https://github.com/tstapler/stapler-squad/issues/140)) ([9fa5059](https://github.com/tstapler/stapler-squad/commit/9fa5059d1da8090680499d35113a9cb912bcf049))
* **review-queue:** resolve UUID→Title before Remove so approved/deleted sessions leave the queue ([ea94659](https://github.com/tstapler/stapler-squad/commit/ea94659ff329737bc7a15fab2bfb9c927a2ed943))
* **review-queue:** show INPUT_REQUIRED items + UX improvements ([757666a](https://github.com/tstapler/stapler-squad/commit/757666aff79b654bcb8a52468ce7a26778ebeb11))
* **review-queue:** suppress auto-advance on session status transitions ([c3fa5e3](https://github.com/tstapler/stapler-squad/commit/c3fa5e3f523d070ccbada952a157a36b67206335))
* **review-queue:** suppress auto-advance on session status transitions ([#135](https://github.com/tstapler/stapler-squad/issues/135)) ([8ec3e9d](https://github.com/tstapler/stapler-squad/commit/8ec3e9d4d170c86ce751858f4b62e6f6caafc2d9))
* **rules:** correct coverage false-positives and add three-state UX ([#104](https://github.com/tstapler/stapler-squad/issues/104)) ([0d3d3af](https://github.com/tstapler/stapler-squad/commit/0d3d3af82156d45b4bec22d1321b7272c548275f))
* **server:** wire SessionHealthChecker into server startup ([bec3c4d](https://github.com/tstapler/stapler-squad/commit/bec3c4d65a13e9d4d7984dacc3b3a1264a86e2aa))
* **session:** hydrate SquadSessionID from legacy conversation_id key ([ac7e051](https://github.com/tstapler/stapler-squad/commit/ac7e051974558538de00e1f12a7dda8d1338196a))
* **session:** preserve initial prompt on restart, defer controller startup, add StartWithSize ([b4c78a1](https://github.com/tstapler/stapler-squad/commit/b4c78a15b056f1b22e6bf1ada07d32778f9ec488))
* **session:** program switching now saves correctly for all cases ([914138e](https://github.com/tstapler/stapler-squad/commit/914138ec162e494f9d90818182c93d3bb0a2ae53))
* **session:** release stateMutex before calling Start() in SwitchWorkspace ([f6f89d8](https://github.com/tstapler/stapler-squad/commit/f6f89d84868fc89e9242f6d3c68541548295dfbc))
* **session:** replace nil,nil return with ErrInstanceDataNotFound sentinel (nilnil lint) ([e2248ba](https://github.com/tstapler/stapler-squad/commit/e2248bac6bf0e8d8efc9a2ad7269dfad2a268525))
* **sessions:** prevent Claude process orphaning after server restart ([da222c2](https://github.com/tstapler/stapler-squad/commit/da222c259e1303a9b084489d550138fbb4539074))
* **session:** wire InitialPrompt to proto adapter and skip driver send when empty ([b8d9ed2](https://github.com/tstapler/stapler-squad/commit/b8d9ed297e9623b13a492dd23dafccd14555167f))
* **session:** wire status manager on new sessions before starting driver ([729968c](https://github.com/tstapler/stapler-squad/commit/729968c7b980f0132fa34c18db53e8459750ca72))
* **session:** wire status manager on new sessions before starting driver ([#111](https://github.com/tstapler/stapler-squad/issues/111)) ([715fcd5](https://github.com/tstapler/stapler-squad/commit/715fcd5ca4d6eda94f56aa016da8e45f88ee2e4a))
* **startup:** resume session drivers for sessions with undelivered InitialPrompt ([f62b772](https://github.com/tstapler/stapler-squad/commit/f62b77252ce8d3ff472ce95c4819a7fa6edfcc95))
* **status:** address second-pass UX review findings ([b03dc73](https://github.com/tstapler/stapler-squad/commit/b03dc7344430c8651c86cc34c6e07f9b23544ec4))
* **terminal:** prevent multi-tab disconnect from killing shared control-mode process ([#107](https://github.com/tstapler/stapler-squad/issues/107)) ([4314926](https://github.com/tstapler/stapler-squad/commit/43149267e596c515d481c1e7ac583eea37daf5fe))
* **terminal:** trailing-edge resize debounce + test isolation fixes ([#125](https://github.com/tstapler/stapler-squad/issues/125)) ([e5a42e7](https://github.com/tstapler/stapler-squad/commit/e5a42e730124cd557528e43a8f8b0c839377c162))
* **triage:** address code review findings from post-commit review ([6030616](https://github.com/tstapler/stapler-squad/commit/60306163562e4bf694580ad8b199c343db9fc0f1))
* **triage:** surface storage errors, add timeout, harden claude detection, link notifications to backlog ([59d41da](https://github.com/tstapler/stapler-squad/commit/59d41da13308eadabf3800e1279c4da11c011e28))
* **ui:** prevent N×5 dispatch storm freezing home page on load ([6d0859d](https://github.com/tstapler/stapler-squad/commit/6d0859d26265c6f4c11cf12f9e1a3371f3635090))
* **ux:** accessibility, status correctness, and detection bug fixes ([5dd16d5](https://github.com/tstapler/stapler-squad/commit/5dd16d57a98a6fd8bcf27ce5da0ee8ce96e178d2))
* **ux:** address 8 UX review findings from 2026-06-30 ([4780b94](https://github.com/tstapler/stapler-squad/commit/4780b94df04df4fc3334bd0d238a269601a95574))
* **ux:** session list accessibility and UX polish ([dee2af4](https://github.com/tstapler/stapler-squad/commit/dee2af4dd361d35bb1daa84b218e1b58da487513))
* **vcs:** periodically clear go-git repo cache to bound memory usage ([6a3e5dd](https://github.com/tstapler/stapler-squad/commit/6a3e5dd13811bd4416468ba67b0f656c385ac0f8))


### Performance Improvements

* batch HistoryLinker PIDs, guard hot debug logs, cache AheadBehind/CommitMessages, pool blob buffer ([#130](https://github.com/tstapler/stapler-squad/issues/130)) ([e385af2](https://github.com/tstapler/stapler-squad/commit/e385af280c449e5f940387214cc563c6412e2d33))
* **memory:** replace wt.Status() with index-based diff + customizable session columns ([1a147e0](https://github.com/tstapler/stapler-squad/commit/1a147e0d146b0c00064df0ab619a59bda37739e5))
* optimize CircularBuffer and eliminate full-buffer copies in status detection ([08a4ef7](https://github.com/tstapler/stapler-squad/commit/08a4ef75c2a7cf193823fc187c4573adc4189771))
* **session:** reduce lock contention across session concurrency primitives ([5dc5d8a](https://github.com/tstapler/stapler-squad/commit/5dc5d8ac524a6c1ad9fec28ad0f561101c6e924b))


### Reverts

* remove incorrect per-session hook injection in session_service ([cfdf4ff](https://github.com/tstapler/stapler-squad/commit/cfdf4ffb2d2ded1fb4604742da83f20ab44378c7))
* remove unnecessary commit-search-body flag ([35e0cd7](https://github.com/tstapler/stapler-squad/commit/35e0cd7894ae738e39492ac0e0955a98abd17f47))

## [1.34.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.33.1...v1.34.0) (2026-07-03)


### Features

* **backlog:** add hard delete for backlog items ([56483ca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/56483ca2ffa65f7b0b60c7a12d40f52df6481875))
* **backlog:** import backlog items from GitHub issues ([46cf6d4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46cf6d431f8604be9c645b7614278cf73f24bfdf))
* **perf:** add singleflight + hasUncommitted TTL cache to GoGitVCSReader ([6fc0fb2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6fc0fb20f187116698196946f82d707e3dc21f7a))
* **perf:** invalidate IsDirty cache on session Pause and Resume ([4cee38d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4cee38dc0ffd2f164ce19dad01e4fab7fb543955))


### Bug Fixes

* autonomous sessions rejected with "path is required" via omnibar ([#157](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/157)) ([5ab6c4a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5ab6c4a08a03dbb6b28e5ae98975a72b543878d6))
* **nav:** add Feature Flags to navigation menu ([3fa665c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3fa665cf20b250f51d09bb880fdcd9f1a7164c94))
* **perf:** release entry.mu before OS stat walk in HasUncommitted; typed nil returns in Do bodies ([71f52b3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/71f52b39bc907b69b7c6103db641ed59c6e92db0))
* **perf:** rename misleading panic test, add scope comment, move InvalidateDirtyCache post-transition ([799a352](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/799a3521a5a040772cf9aa23a071b4a84361eaf0))
* **service:** fall back to launchctl load when bootstrap fails on macOS ([46bd0ea](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46bd0ea9f5d92153f7bb279f56b2cbc0fcfdca56))
* **terminal:** correctly scan OSC/DCS escape sequences to stop render artifacts ([#156](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/156)) ([2151b4b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2151b4b2186627c64c1be1617e4e096918d5b491))

## [1.33.1](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.33.0...v1.33.1) (2026-07-02)


### Bug Fixes

* **analytics:** escape analytics session_id mismatch and dead mangle detection ([#149](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/149)) ([a121de8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a121de83717185a5a42183288ffde1f8e0a298cf))
* backlog/triage sessions die on launch (shell injection + flag-parsing crash) ([#150](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/150)) ([8016921](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8016921fa302c01ed9c16c5f568e410b9081b2b7))
* **backlog:** GitHub URL repo-path support, first-visit tour, and two related bugs ([#152](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/152)) ([6ef8164](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6ef8164b215f0c08c4a497b00fd33da516e9a288))
* web-build target doesn't generate proto bindings on a clean clone ([#155](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/155)) ([1bb135e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1bb135e8d93456c70db12ceb77b9f335369925f9))

## [1.33.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.32.0...v1.33.0) (2026-07-02)


### Features

* **pr-status:** show PR badge in row mode and use go-git for branch detection ([664fbfb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/664fbfb118d036a7ef52e0519a05d9e8368f22ca))


### Bug Fixes

* **codesign:** correct otool byte-order in verify-codesign plist decode ([ca60c0a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ca60c0ae893ba542a62cc46c5789af38fb57da4c))
* **css:** enable scroll on unfinished tab container ([fa5f37e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fa5f37e25607ca094858655a57cbc0ca9202f368))
* **lint:** suppress norawexec on lookPathOnlyExecutor stub ([d054495](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d054495cb49d5a4151475df6fa0fe2e8cb99edc2))
* **lint:** use correct nolint directives for lookPathOnlyExecutor stub ([8987673](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/898767374dad24c5c6ab8e30aecf4ec5c96c5c29))
* repair broken release pipeline and build-from-source path ([#147](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/147)) ([1fc2ff6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1fc2ff6c90fe88af32f14fa600bdfc03a63fcf54))
* review queue auto-advance respects preference after approve/deny ([c63886a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c63886aa90a2354e41140a51280ad86eadfc06df))
* **unfinished:** stack GitHub auth banner vertically so Connect button is always visible ([750d252](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/750d252092abf455d7c20ad8ad143486e28674d2))

## [1.32.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.31.1...v1.32.0) (2026-07-01)


### Features

* **artifacts:** extract PR URLs, commits & external URLs from JSONL history ([#129](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/129)) ([bfe06a5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bfe06a543ce070f5e87b7a501ccd0dda1a2221c2))
* **autonomous:** P0 foundation — AutonomousDriver + StatusChangeListener fan-out ([#105](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/105)) ([95b7c69](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/95b7c696df5ca3c0eea6f0e81d6c56ac664ba978))
* **autonomous:** triage migration, review queue UX, backlog MCP tools, session detail polish ([633e88d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/633e88d7b56d9bf8d9077c3f2109aadaf46b383a))
* **backlog:** implement CancelTriage RPC and session delete button ([a690c36](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a690c366d6704651b913a60dab6b8774303a7f45))
* **detection:** add StatusWaitingForAgent for background agent state ([#115](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/115)) ([dddb62d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dddb62d555e2752abfca5d25ce79c946580bf6aa))
* **detection:** detect monitors-still-running as WaitingForAgent ([#121](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/121)) ([995d065](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/995d065c81d059fbc7de93304efe56ca407af351))
* **detection:** fix session status indicators + observability ring buffer ([#110](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/110)) ([f9f3079](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f9f30798eb9bb44a0da3a46f3593234061d347b0))
* **detection:** surface WAITING_FOR_AGENT as distinct SubStatus chip ([#126](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/126)) ([e5f80e8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e5f80e8bf4b691212b9524cfcab216caf5e3a43a))
* **events:** fix event pipeline gaps for cross-surface state consistency ([#120](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/120)) ([016b558](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/016b5584b5005363c7160e95181f307224264421))
* **files:** local file browser with iframe/SVG rendering ([#113](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/113)) ([e55ada1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e55ada1df4ed8a157b21949b74c9852823cf7e05))
* gap-finder workflow + SESSION_STATUS_RESTORING ProtoToStatus fix ([#123](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/123)) ([45a5a91](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/45a5a91d53480f500e00fa584e4db7b115edf309))
* GitHub work continuity — persistence, annotation fallback, and type-safe RepoRef ([3be7e09](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3be7e0902f57295fb17953d9ff3a84cabcfd3b9f))
* GitHub work continuity — UserPRCache, GitHubUserService, and Unfinished Tab integration ([#141](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/141)) ([e322a7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e322a7ebf798ba9e18543fd9d84e396ad065d501))
* local file browser + detection readline fixes ([#116](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/116)) ([5ce6c1c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5ce6c1c0914c1e774b851f2ac8025a21fb7208f1))
* **omnibar:** add [@alias](https://github.com/alias) session presets ([#122](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/122)) ([a65f7d2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a65f7d2ebfd49239c5436f01e2978984dfa4d3bb))
* **omnibar:** alias name_prefix field and trust-dialog double-fire fix ([#128](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/128)) ([7dbfb10](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7dbfb10c27524319ad63e918a3f4ea704b309818))
* **onboarding:** offer to install Claude Code hooks during onboarding ([#138](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/138)) ([41e0206](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/41e0206f54acbf58203959fbc697e173cb7471fb))
* **pane:** add session peek modal triggered from pane picker ([9bf226a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9bf226a7564e54b36b433cc52a359c573887cd14))
* **reconnect:** jittered backoff reconnect for session-watch and terminal streams ([#136](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/136)) ([837ba8c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/837ba8cc70eb6f637c2b3bab2a4b27838e1d1113))
* **registry:** typed feature catalog replacing comment-based annotations ([#112](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/112)) ([6232826](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6232826397e21f803d4af4bc01af481b7ac39545))
* **rules:** auto-suggest rule name from criteria inputs ([#140](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/140)) ([d072abc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d072abc37fcb228dd052cd38cb8c324eb8c3bdcf))
* **rules:** structured rule builder, template library, analytics→rule workflow ([#103](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/103)) ([48adec6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/48adec6b2e47301106ea1b9192828fea445e83b2))
* **server:** startup fast restore, terminal snapshot fix, build warnings ([#128](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/128)) ([e158b30](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e158b305f3bb7b19e9a81e772f082140043df82a))
* **session-list:** bulk selection for row mode with undo-on-delete ([#137](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/137)) ([3571d03](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3571d037fa81e3f3b1a804da21dbb6adbf7376ba))
* **session:** add 30-min background reaper for paused-session tmux processes ([66e33e2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/66e33e24b88bf91153f1d4b935ce9cfdd63ff224))
* **session:** enable/disable autonomous mode on existing sessions ([4f2967c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4f2967cfb604880a4ede96e7e7c2cf0db90a10c3))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([#124](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/124)) ([e54f41b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e54f41b012370f4f3620e2fe64cad122230fc66e))
* **status:** unify session status vocabulary across UI ([c42514d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c42514d4020c41c6ad16ec2f546846e257a81563))
* workflow run history, session archiving, slash commands, and pause memory optimization ([9c23e61](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9c23e6126664bc7faf458babbefc1fc7069e5c99))
* **workflows:** Quick Workflows — [@slug](https://github.com/slug) omnibar, management UI, cron scheduling ([#106](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/106)) ([883deca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/883deca379d7529f921d654b6671c559c7766ab9))
* **workflows:** session affordances, retention enforcer, bulk-session RPCs ([ac4efa1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ac4efa121abdb34649470cc16133e0a04872e20e))


### Bug Fixes

* **backlog:** replace idle triage sessions with headless pool calls ([#127](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/127)) ([2d7e116](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2d7e116ca3f624a891babb2e27aa447e904c50be))
* **ci:** force-add bench-baseline.txt to bypass .gitignore in benchmark gate ([ebd4278](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ebd4278098639893694f192f457fb214ed481257))
* **codesign:** auto-run setup-codesign and fix OpenSSL 3 PKCS12 compatibility ([de6c7a6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/de6c7a62ca8ddb61ce728d2094e59808bac8a3d6))
* **codesign:** install binary to ~/.stapler-squad/bin/ before signing to prevent sealed-resource invalidation ([6c539c3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6c539c31202e181c93b5905d929d5a49067c6591))
* **db:** make approval_rule JSON fields Optional to fix SQLite migration ([3f06c42](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3f06c420517312b4d7bfed3ae5c4201a9c3c9c90))
* **deps:** upgrade gopsutil v3 to v4 to drop crashing go-m1cpu cgo init ([#129](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/129)) ([69bc09f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/69bc09fa3564ecd7d965fc51386311c5b49bf79d))
* **detection:** detect indented spinners and CR-overwritten esc-to-interrupt ([#108](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/108)) ([54a5f63](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/54a5f637a3859dc901d521bb3f9861032616ab63))
* **detection:** detect WaitingForAgent when esc-to-interrupt appears below spinner ([44b455a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/44b455a751cc3907a9ce3fdb3b2c945929ba6a25))
* **detection:** detect workflow approval dialog and idle-while-typing states ([63c8ca9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/63c8ca9cff55e40879a08954dcfd1c1a528c5dc1))
* **detection:** fix ⌛ Thinking chip missing on most active sessions ([375f7b0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/375f7b0419a0d2011aedc9576aeaab27a90e4710))
* **detection:** use bottom-up scan in review queue for no-controller sessions ([5d2ba8f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5d2ba8f85acaa209f96ae3d6ab70d9d3ef49688a))
* **history-linker:** preserve paused/hibernated session UUID on rescan ([#118](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/118)) ([fb0e49f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fb0e49f723385a12a77f47fa67157c02cd3790e3))
* **install:** fix macOS FDA flow and health-check timeout on first install ([38c98b5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/38c98b518a383d4aee9076420d87382e5701f843))
* **install:** increase health-check timeout to 120s for session-restore latency ([f64eea0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f64eea0bd4d55fd8b6cfee1def6bb0b2db253710))
* **install:** require launchctl bootstrap; remove launchctl load fallback ([9d3c503](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9d3c503ec531b78a5f5808623bdd1aecec1f7c49))
* **install:** skip FDA prompt for non-admin users with cert-signed binary ([c24d088](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c24d088cd1bdb72225f0ac81cb16846b7d637c2a))
* **lint,status:** fix ESLint errors on main and complete status descriptions ([5635c56](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5635c567e7c2b5ce9606e15b42b5fbe166cda2ba))
* **notifications:** auto-resolve stale toasts and Chrome notifications ([#109](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/109)) ([31fd160](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/31fd160c667b018a817e60b2b8cb0853d831084d))
* publish session update event on controller status change ([9241d3e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9241d3e0bcb89c0f65971f64435f4afe4fab1064))
* **registry:** add workflow/detection RPCs to scanner methodToID map ([24388a0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/24388a079da33430e2d880e0b0647c8124b27388))
* **review-queue:** suppress auto-advance on session status transitions ([#135](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/135)) ([8ec3e9d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8ec3e9d4d170c86ce751858f4b62e6f6caafc2d9))
* **rules:** correct coverage false-positives and add three-state UX ([#104](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/104)) ([0d3d3af](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0d3d3af82156d45b4bec22d1321b7272c548275f))
* **server:** wire SessionHealthChecker into server startup ([bec3c4d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bec3c4d65a13e9d4d7984dacc3b3a1264a86e2aa))
* **session-driver:** add live output check before prompt injection ([aa9834c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/aa9834cdc11b87460eb2d706946cc1c4e07eac2f))
* **session:** preserve initial prompt on restart, defer controller startup, add StartWithSize ([b4c78a1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b4c78a15b056f1b22e6bf1ada07d32778f9ec488))
* **session:** wire InitialPrompt to proto adapter and skip driver send when empty ([b8d9ed2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b8d9ed297e9623b13a492dd23dafccd14555167f))
* **session:** wire status manager on new sessions before starting driver ([729968c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/729968c7b980f0132fa34c18db53e8459750ca72))
* **session:** wire status manager on new sessions before starting driver ([#111](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/111)) ([715fcd5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/715fcd5ca4d6eda94f56aa016da8e45f88ee2e4a))
* skip shell sourcing in test mode to prevent service test timeout ([49eb0ba](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/49eb0ba5dab6438f9796f36fa6ff8da55b10875a))
* **startup:** resume session drivers for sessions with undelivered InitialPrompt ([f62b772](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f62b77252ce8d3ff472ce95c4819a7fa6edfcc95))
* **status:** address second-pass UX review findings ([b03dc73](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b03dc7344430c8651c86cc34c6e07f9b23544ec4))
* **terminal:** prevent multi-tab disconnect from killing shared control-mode process ([#107](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/107)) ([4314926](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/43149267e596c515d481c1e7ac583eea37daf5fe))
* **terminal:** repair escape code pipeline for new Claude Code renderer ([#139](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/139)) ([dc71b82](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dc71b828a6d6a61883cd12b7c666e836eda97c90))
* **terminal:** trailing-edge resize debounce + test isolation fixes ([#125](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/125)) ([e5a42e7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e5a42e730124cd557528e43a8f8b0c839377c162))
* **tmux:** flush stale exists-cache in RestoreWithWorkDir and add --tmux-keep-server to plist ([12b992a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/12b992a32ca5e9997f6b042e719bc49ef99eec91))
* **triage:** address code review findings from post-commit review ([6030616](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/60306163562e4bf694580ad8b199c343db9fc0f1))
* **triage:** silent storage error, hung session timeout, and Claude detection false positives ([cc82510](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cc8251090c0d6987e8f439fb1ba7b8cbce5d41e7))
* **triage:** surface storage errors, add timeout, harden claude detection, link notifications to backlog ([59d41da](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/59d41da13308eadabf3800e1279c4da11c011e28))
* **ux:** accessibility, status correctness, and detection bug fixes ([5dd16d5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5dd16d57a98a6fd8bcf27ce5da0ee8ce96e178d2))
* **ux:** session list accessibility and UX polish ([dee2af4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dee2af4dd361d35bb1daa84b218e1b58da487513))


### Performance Improvements

* batch HistoryLinker PIDs, guard hot debug logs, cache AheadBehind/CommitMessages, pool blob buffer ([#130](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/130)) ([e385af2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e385af280c449e5f940387214cc563c6412e2d33))
* **memory:** replace wt.Status() with index-based diff + customizable session columns ([1a147e0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1a147e0d146b0c00064df0ab619a59bda37739e5))
* optimize CircularBuffer and eliminate full-buffer copies in status detection ([08a4ef7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/08a4ef75c2a7cf193823fc187c4573adc4189771))
* **session:** reduce lock contention across session concurrency primitives ([5dc5d8a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5dc5d8ac524a6c1ad9fec28ad0f561101c6e924b))
* **tmux:** add semaphore to cap concurrent capture-pane subprocesses ([fa54896](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fa548964a70cc757ac46a5d49b810ca389b53f90))
* **vcs:** cache reachableSet results and batch-read blobs under single lock ([125f4ef](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/125f4ef7e3f32f0a1f05c213cd394e4ab2e5e3bd))

## [1.35.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.34.0...v1.35.0) (2026-07-07)


### Features

* **backlog:** enhance GitHub issue picker — PR support, timestamps, keyboard nav, history tier ([1e6e33e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1e6e33eea3b6db1cdc25feab74b3464a880b69c6))
* **backlog:** GitHub issue picker — browse repos and issues to import ([4157558](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4157558b1309879e570cb5a360288ca24fcaffde))
* build pinned tmux, wire TMUX_BIN into all test targets, add goroutine-dump TestMain and test-trace targets ([0fd79cd](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0fd79cdfb798567017d624ac222fa8526cb36bb6))
* wire tmux.Binary() into all call sites so TMUX_BIN env var is respected ([d9b188d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d9b188d1ecf382b8d37fda9c85e876a030853053))


### Bug Fixes

* **lint:** add concurrency:1 to avoid golangci-lint v2.11.4 race on go1.26 deps ([3407f0c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3407f0c161e3132362be9f175118be02a692480a))
* **lint:** analytics-exempt line order; chore(ci): make demo GIFs manual-only ([d275e54](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d275e540c4935e6c06eba8257578eac8f3063c51))


### Performance Improvements

* drain responseCh non-blocking in waitForCommandOrDrain to reduce 6.6M block events ([6dc4552](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6dc45527eeb7b0c3721397c02cb08547a08bafee))
* **frontend:** virtualize session list card mode with react-virtuoso to eliminate 1845ms scroll task ([bad2867](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bad2867a448e08754e1ceb534604af86858b523b))
* singleflight coalescing for IsDirtyWithHint, go-git for IsBranchCheckedOut/getHeadCommitSHA; remove NOT STALE debug log ([f157fa8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f157fa8b09b329ced0ff41054b78fef9c5d3ed0a))

## [1.34.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.33.1...v1.34.0) (2026-07-03)


### Features

* **backlog:** add hard delete for backlog items ([56483ca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/56483ca2ffa65f7b0b60c7a12d40f52df6481875))
* **backlog:** import backlog items from GitHub issues ([46cf6d4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46cf6d431f8604be9c645b7614278cf73f24bfdf))
* **perf:** add singleflight + hasUncommitted TTL cache to GoGitVCSReader ([6fc0fb2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6fc0fb20f187116698196946f82d707e3dc21f7a))
* **perf:** invalidate IsDirty cache on session Pause and Resume ([4cee38d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4cee38dc0ffd2f164ce19dad01e4fab7fb543955))


### Bug Fixes

* autonomous sessions rejected with "path is required" via omnibar ([#157](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/157)) ([5ab6c4a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5ab6c4a08a03dbb6b28e5ae98975a72b543878d6))
* **nav:** add Feature Flags to navigation menu ([3fa665c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3fa665cf20b250f51d09bb880fdcd9f1a7164c94))
* **perf:** release entry.mu before OS stat walk in HasUncommitted; typed nil returns in Do bodies ([71f52b3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/71f52b39bc907b69b7c6103db641ed59c6e92db0))
* **perf:** rename misleading panic test, add scope comment, move InvalidateDirtyCache post-transition ([799a352](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/799a3521a5a040772cf9aa23a071b4a84361eaf0))
* **service:** fall back to launchctl load when bootstrap fails on macOS ([46bd0ea](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/46bd0ea9f5d92153f7bb279f56b2cbc0fcfdca56))
* **terminal:** correctly scan OSC/DCS escape sequences to stop render artifacts ([#156](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/156)) ([2151b4b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2151b4b2186627c64c1be1617e4e096918d5b491))

## [1.33.1](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.33.0...v1.33.1) (2026-07-02)


### Bug Fixes

* **analytics:** escape analytics session_id mismatch and dead mangle detection ([#149](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/149)) ([a121de8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a121de83717185a5a42183288ffde1f8e0a298cf))
* backlog/triage sessions die on launch (shell injection + flag-parsing crash) ([#150](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/150)) ([8016921](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8016921fa302c01ed9c16c5f568e410b9081b2b7))
* **backlog:** GitHub URL repo-path support, first-visit tour, and two related bugs ([#152](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/152)) ([6ef8164](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6ef8164b215f0c08c4a497b00fd33da516e9a288))
* web-build target doesn't generate proto bindings on a clean clone ([#155](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/155)) ([1bb135e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1bb135e8d93456c70db12ceb77b9f335369925f9))

## [1.33.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.32.0...v1.33.0) (2026-07-02)


### Features

* **pr-status:** show PR badge in row mode and use go-git for branch detection ([664fbfb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/664fbfb118d036a7ef52e0519a05d9e8368f22ca))


### Bug Fixes

* **codesign:** correct otool byte-order in verify-codesign plist decode ([ca60c0a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ca60c0ae893ba542a62cc46c5789af38fb57da4c))
* **css:** enable scroll on unfinished tab container ([fa5f37e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fa5f37e25607ca094858655a57cbc0ca9202f368))
* **lint:** suppress norawexec on lookPathOnlyExecutor stub ([d054495](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d054495cb49d5a4151475df6fa0fe2e8cb99edc2))
* **lint:** use correct nolint directives for lookPathOnlyExecutor stub ([8987673](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/898767374dad24c5c6ab8e30aecf4ec5c96c5c29))
* repair broken release pipeline and build-from-source path ([#147](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/147)) ([1fc2ff6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1fc2ff6c90fe88af32f14fa600bdfc03a63fcf54))
* review queue auto-advance respects preference after approve/deny ([c63886a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c63886aa90a2354e41140a51280ad86eadfc06df))
* **unfinished:** stack GitHub auth banner vertically so Connect button is always visible ([750d252](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/750d252092abf455d7c20ad8ad143486e28674d2))

## [1.32.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.31.1...v1.32.0) (2026-07-01)


### Features

* **artifacts:** extract PR URLs, commits & external URLs from JSONL history ([#129](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/129)) ([bfe06a5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bfe06a543ce070f5e87b7a501ccd0dda1a2221c2))
* **autonomous:** P0 foundation — AutonomousDriver + StatusChangeListener fan-out ([#105](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/105)) ([95b7c69](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/95b7c696df5ca3c0eea6f0e81d6c56ac664ba978))
* **autonomous:** triage migration, review queue UX, backlog MCP tools, session detail polish ([633e88d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/633e88d7b56d9bf8d9077c3f2109aadaf46b383a))
* **backlog:** implement CancelTriage RPC and session delete button ([a690c36](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a690c366d6704651b913a60dab6b8774303a7f45))
* **detection:** add StatusWaitingForAgent for background agent state ([#115](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/115)) ([dddb62d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dddb62d555e2752abfca5d25ce79c946580bf6aa))
* **detection:** detect monitors-still-running as WaitingForAgent ([#121](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/121)) ([995d065](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/995d065c81d059fbc7de93304efe56ca407af351))
* **detection:** fix session status indicators + observability ring buffer ([#110](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/110)) ([f9f3079](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f9f30798eb9bb44a0da3a46f3593234061d347b0))
* **detection:** surface WAITING_FOR_AGENT as distinct SubStatus chip ([#126](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/126)) ([e5f80e8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e5f80e8bf4b691212b9524cfcab216caf5e3a43a))
* **events:** fix event pipeline gaps for cross-surface state consistency ([#120](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/120)) ([016b558](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/016b5584b5005363c7160e95181f307224264421))
* **files:** local file browser with iframe/SVG rendering ([#113](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/113)) ([e55ada1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e55ada1df4ed8a157b21949b74c9852823cf7e05))
* gap-finder workflow + SESSION_STATUS_RESTORING ProtoToStatus fix ([#123](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/123)) ([45a5a91](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/45a5a91d53480f500e00fa584e4db7b115edf309))
* GitHub work continuity — persistence, annotation fallback, and type-safe RepoRef ([3be7e09](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3be7e0902f57295fb17953d9ff3a84cabcfd3b9f))
* GitHub work continuity — UserPRCache, GitHubUserService, and Unfinished Tab integration ([#141](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/141)) ([e322a7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e322a7ebf798ba9e18543fd9d84e396ad065d501))
* local file browser + detection readline fixes ([#116](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/116)) ([5ce6c1c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5ce6c1c0914c1e774b851f2ac8025a21fb7208f1))
* **omnibar:** add [@alias](https://github.com/alias) session presets ([#122](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/122)) ([a65f7d2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a65f7d2ebfd49239c5436f01e2978984dfa4d3bb))
* **omnibar:** alias name_prefix field and trust-dialog double-fire fix ([#128](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/128)) ([7dbfb10](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7dbfb10c27524319ad63e918a3f4ea704b309818))
* **onboarding:** offer to install Claude Code hooks during onboarding ([#138](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/138)) ([41e0206](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/41e0206f54acbf58203959fbc697e173cb7471fb))
* **pane:** add session peek modal triggered from pane picker ([9bf226a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9bf226a7564e54b36b433cc52a359c573887cd14))
* **reconnect:** jittered backoff reconnect for session-watch and terminal streams ([#136](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/136)) ([837ba8c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/837ba8cc70eb6f637c2b3bab2a4b27838e1d1113))
* **registry:** typed feature catalog replacing comment-based annotations ([#112](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/112)) ([6232826](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6232826397e21f803d4af4bc01af481b7ac39545))
* **rules:** auto-suggest rule name from criteria inputs ([#140](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/140)) ([d072abc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d072abc37fcb228dd052cd38cb8c324eb8c3bdcf))
* **rules:** structured rule builder, template library, analytics→rule workflow ([#103](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/103)) ([48adec6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/48adec6b2e47301106ea1b9192828fea445e83b2))
* **server:** startup fast restore, terminal snapshot fix, build warnings ([#128](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/128)) ([e158b30](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e158b305f3bb7b19e9a81e772f082140043df82a))
* **session-list:** bulk selection for row mode with undo-on-delete ([#137](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/137)) ([3571d03](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3571d037fa81e3f3b1a804da21dbb6adbf7376ba))
* **session:** add 30-min background reaper for paused-session tmux processes ([66e33e2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/66e33e24b88bf91153f1d4b935ce9cfdd63ff224))
* **session:** enable/disable autonomous mode on existing sessions ([4f2967c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4f2967cfb604880a4ede96e7e7c2cf0db90a10c3))
* **settings:** add UpsertAlias and DeleteAlias RPCs with AliasesManager UI ([#124](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/124)) ([e54f41b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e54f41b012370f4f3620e2fe64cad122230fc66e))
* **status:** unify session status vocabulary across UI ([c42514d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c42514d4020c41c6ad16ec2f546846e257a81563))
* workflow run history, session archiving, slash commands, and pause memory optimization ([9c23e61](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9c23e6126664bc7faf458babbefc1fc7069e5c99))
* **workflows:** Quick Workflows — [@slug](https://github.com/slug) omnibar, management UI, cron scheduling ([#106](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/106)) ([883deca](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/883deca379d7529f921d654b6671c559c7766ab9))
* **workflows:** session affordances, retention enforcer, bulk-session RPCs ([ac4efa1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ac4efa121abdb34649470cc16133e0a04872e20e))


### Bug Fixes

* **backlog:** replace idle triage sessions with headless pool calls ([#127](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/127)) ([2d7e116](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2d7e116ca3f624a891babb2e27aa447e904c50be))
* **ci:** force-add bench-baseline.txt to bypass .gitignore in benchmark gate ([ebd4278](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ebd4278098639893694f192f457fb214ed481257))
* **codesign:** auto-run setup-codesign and fix OpenSSL 3 PKCS12 compatibility ([de6c7a6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/de6c7a62ca8ddb61ce728d2094e59808bac8a3d6))
* **codesign:** install binary to ~/.stapler-squad/bin/ before signing to prevent sealed-resource invalidation ([6c539c3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6c539c31202e181c93b5905d929d5a49067c6591))
* **db:** make approval_rule JSON fields Optional to fix SQLite migration ([3f06c42](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3f06c420517312b4d7bfed3ae5c4201a9c3c9c90))
* **deps:** upgrade gopsutil v3 to v4 to drop crashing go-m1cpu cgo init ([#129](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/129)) ([69bc09f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/69bc09fa3564ecd7d965fc51386311c5b49bf79d))
* **detection:** detect indented spinners and CR-overwritten esc-to-interrupt ([#108](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/108)) ([54a5f63](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/54a5f637a3859dc901d521bb3f9861032616ab63))
* **detection:** detect WaitingForAgent when esc-to-interrupt appears below spinner ([44b455a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/44b455a751cc3907a9ce3fdb3b2c945929ba6a25))
* **detection:** detect workflow approval dialog and idle-while-typing states ([63c8ca9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/63c8ca9cff55e40879a08954dcfd1c1a528c5dc1))
* **detection:** fix ⌛ Thinking chip missing on most active sessions ([375f7b0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/375f7b0419a0d2011aedc9576aeaab27a90e4710))
* **detection:** use bottom-up scan in review queue for no-controller sessions ([5d2ba8f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5d2ba8f85acaa209f96ae3d6ab70d9d3ef49688a))
* **history-linker:** preserve paused/hibernated session UUID on rescan ([#118](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/118)) ([fb0e49f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fb0e49f723385a12a77f47fa67157c02cd3790e3))
* **install:** fix macOS FDA flow and health-check timeout on first install ([38c98b5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/38c98b518a383d4aee9076420d87382e5701f843))
* **install:** increase health-check timeout to 120s for session-restore latency ([f64eea0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f64eea0bd4d55fd8b6cfee1def6bb0b2db253710))
* **install:** require launchctl bootstrap; remove launchctl load fallback ([9d3c503](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9d3c503ec531b78a5f5808623bdd1aecec1f7c49))
* **install:** skip FDA prompt for non-admin users with cert-signed binary ([c24d088](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c24d088cd1bdb72225f0ac81cb16846b7d637c2a))
* **lint,status:** fix ESLint errors on main and complete status descriptions ([5635c56](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5635c567e7c2b5ce9606e15b42b5fbe166cda2ba))
* **notifications:** auto-resolve stale toasts and Chrome notifications ([#109](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/109)) ([31fd160](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/31fd160c667b018a817e60b2b8cb0853d831084d))
* publish session update event on controller status change ([9241d3e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9241d3e0bcb89c0f65971f64435f4afe4fab1064))
* **registry:** add workflow/detection RPCs to scanner methodToID map ([24388a0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/24388a079da33430e2d880e0b0647c8124b27388))
* **review-queue:** suppress auto-advance on session status transitions ([#135](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/135)) ([8ec3e9d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8ec3e9d4d170c86ce751858f4b62e6f6caafc2d9))
* **rules:** correct coverage false-positives and add three-state UX ([#104](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/104)) ([0d3d3af](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0d3d3af82156d45b4bec22d1321b7272c548275f))
* **server:** wire SessionHealthChecker into server startup ([bec3c4d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bec3c4d65a13e9d4d7984dacc3b3a1264a86e2aa))
* **session-driver:** add live output check before prompt injection ([aa9834c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/aa9834cdc11b87460eb2d706946cc1c4e07eac2f))
* **session:** preserve initial prompt on restart, defer controller startup, add StartWithSize ([b4c78a1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b4c78a15b056f1b22e6bf1ada07d32778f9ec488))
* **session:** wire InitialPrompt to proto adapter and skip driver send when empty ([b8d9ed2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b8d9ed297e9623b13a492dd23dafccd14555167f))
* **session:** wire status manager on new sessions before starting driver ([729968c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/729968c7b980f0132fa34c18db53e8459750ca72))
* **session:** wire status manager on new sessions before starting driver ([#111](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/111)) ([715fcd5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/715fcd5ca4d6eda94f56aa016da8e45f88ee2e4a))
* skip shell sourcing in test mode to prevent service test timeout ([49eb0ba](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/49eb0ba5dab6438f9796f36fa6ff8da55b10875a))
* **startup:** resume session drivers for sessions with undelivered InitialPrompt ([f62b772](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f62b77252ce8d3ff472ce95c4819a7fa6edfcc95))
* **status:** address second-pass UX review findings ([b03dc73](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b03dc7344430c8651c86cc34c6e07f9b23544ec4))
* **terminal:** prevent multi-tab disconnect from killing shared control-mode process ([#107](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/107)) ([4314926](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/43149267e596c515d481c1e7ac583eea37daf5fe))
* **terminal:** repair escape code pipeline for new Claude Code renderer ([#139](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/139)) ([dc71b82](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dc71b828a6d6a61883cd12b7c666e836eda97c90))
* **terminal:** trailing-edge resize debounce + test isolation fixes ([#125](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/125)) ([e5a42e7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e5a42e730124cd557528e43a8f8b0c839377c162))
* **tmux:** flush stale exists-cache in RestoreWithWorkDir and add --tmux-keep-server to plist ([12b992a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/12b992a32ca5e9997f6b042e719bc49ef99eec91))
* **triage:** address code review findings from post-commit review ([6030616](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/60306163562e4bf694580ad8b199c343db9fc0f1))
* **triage:** silent storage error, hung session timeout, and Claude detection false positives ([cc82510](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cc8251090c0d6987e8f439fb1ba7b8cbce5d41e7))
* **triage:** surface storage errors, add timeout, harden claude detection, link notifications to backlog ([59d41da](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/59d41da13308eadabf3800e1279c4da11c011e28))
* **ux:** accessibility, status correctness, and detection bug fixes ([5dd16d5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5dd16d57a98a6fd8bcf27ce5da0ee8ce96e178d2))
* **ux:** session list accessibility and UX polish ([dee2af4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dee2af4dd361d35bb1daa84b218e1b58da487513))


### Performance Improvements

* batch HistoryLinker PIDs, guard hot debug logs, cache AheadBehind/CommitMessages, pool blob buffer ([#130](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/130)) ([e385af2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e385af280c449e5f940387214cc563c6412e2d33))
* **memory:** replace wt.Status() with index-based diff + customizable session columns ([1a147e0](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1a147e0d146b0c00064df0ab619a59bda37739e5))
* optimize CircularBuffer and eliminate full-buffer copies in status detection ([08a4ef7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/08a4ef75c2a7cf193823fc187c4573adc4189771))
* **session:** reduce lock contention across session concurrency primitives ([5dc5d8a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5dc5d8ac524a6c1ad9fec28ad0f561101c6e924b))
* **tmux:** add semaphore to cap concurrent capture-pane subprocesses ([fa54896](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fa548964a70cc757ac46a5d49b810ca389b53f90))
* **vcs:** cache reachableSet results and batch-read blobs under single lock ([125f4ef](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/125f4ef7e3f32f0a1f05c213cd394e4ab2e5e3bd))

## [1.31.1](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.31.0...v1.31.1) (2026-06-02)


### Bug Fixes

* **lint:** gofmt rules_service_test.go ([cd3c4b1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cd3c4b1628e9b8ffa6daf487f31695573b7bf8ec))

## [1.31.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.30.0...v1.31.0) (2026-06-02)


### Features

* add support for Antigravity (agy) CLI ([a981bcb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a981bcb16db6ee557c9a487b48703951c48d2cdd))
* **agy:** full Antigravity CLI support — hooks, install, UI, detection ([e66d058](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e66d05839789737cabad6d8b9401c3b431aff9e8))
* **analytics:** program detail panel with subcommand drill-down ([#85](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/85)) ([9d43582](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9d4358272d7293457c6f6474480d66f4bd4b2341))
* **backlog:** automated triage pipeline with review panel ([fcafaed](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fcafaedd9bdb2a01e8765d44d9d7324df0a80ec8))
* **backlog:** session monitor panel and backlog detail improvements ([80d3f3b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/80d3f3bf60fbb949c549265055a9855e43a631b6))
* **backlog:** triage re-trigger fix, detail UI polish, backlog in mobile nav ([6a174c3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6a174c3a0e1298bec2cc387b31de51f5c0eb7ecd))
* **backlog:** workflow engine and status event audit log ([7458bcb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7458bcb33d42d98fa2602c8caaab44cd402b5106))
* **headless:** session/headless pool — cache-optimized background LLM calls via claude -p ([fba787e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fba787e5c843848fceb76eb93654bf03ab8748a0))
* **headless:** session/headless pool — cache-optimized background LLM calls via claude -p ([0a4e10e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0a4e10e7714c495375468a46c5da7bc44d712b87))
* **mcp:** tag MCP-created sessions with source:mcp ([e3191bc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e3191bc271dbac302c944ee95e6c7f2ef5aaf36d))
* **memory:** surface memory pressure and fix hibernation sweeper ([f455f3b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f455f3bf655eea20690c3a5cdfdcb8ce5c132d5f))
* **memory:** surface memory pressure and fix hibernation sweeper ([5632d7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5632d7eacfcc47742b300b7b8ad097d7f0e0ed83))
* **memory:** surface memory pressure and fix hibernation sweeper ([2927de7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/2927de74739889a092d796e11b19190d7516ba2f))
* **rules:** modal dialog, scrollable table, analytics deep-link pre-fill ([6f654e3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6f654e30e9b721c8ab1f36dc6522a38093bc56dc))
* **rules:** UX improvements — toast z-index, mobile layout, form sections, table clarity ([17b2a6f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/17b2a6f20d4ef2e110c11d2356bc60d42c46eb14))
* **rules:** YAML import/export and UX improvements for ApprovalRulesPanel ([#123](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/123)) ([ea92de7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ea92de7fc83f8b060eabb2d5425c9f13233bc1cd))
* **session:** immortal migration — ProcessManager interface + TmuxBackend + NativeProcessManager ([#113](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/113)) ([66b4c65](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/66b4c65179ce444bcedb14f97af87d1cb90020fb))
* **session:** session steering — supervised driver for all automated sessions ([874c1e6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/874c1e63409953706378430326570fc2491112d4))
* **session:** WriteToSession RPC for sending raw input to a PTY ([5b528f1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5b528f1a015d202412b10084d0c41553f42d9c19))
* **shell-tabs:** complete UX discovery, error states, and full test coverage ([#93](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/93)) ([c380068](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c380068e34ff3e6772d9e1b9a4ee8bf5c092f9ca))
* **shells:** custom shell tabs — interactive PTY shells attached to sessions ([#109](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/109)) ([fb5a6d1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fb5a6d15d0e29b7e52cadf89412fac3f74ffc1c2))
* **upload:** multi-file + Android camera/gallery/file picker in terminal toolbar ([b4b43e4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b4b43e4d04d3f9bc2281ae95765304873338e2ba))
* **upload:** multi-file + Android camera/gallery/file picker in terminal toolbar ([417ac0a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/417ac0a2c8821c18aa09f11cd7dcde0d021b5922))
* **ux:** paused session clarity — overlay, visual distinction, pause reason ([#90](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/90)) ([d07584c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d07584cd95b6fb7a5f630a9db019de8d457b1ec3))


### Bug Fixes

* address code review findings from history-linker and workspace-service ([e80cb2a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e80cb2aea4bd9369b68ca52b83bd856b462002a9))
* **backlog:** repair triage pipeline — prompt injection, session exit, and oneshot flag ([cbc938b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cbc938bd4927ba6c6145148a85ad8cf948be08ab))
* **ci:** add ent ORM generation step before lint and build ([81e7ebf](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/81e7ebfb627186e5e3feaf89ec317594df2a351c))
* **ci:** allow ESLint warnings in full-scan path (push/workflow_dispatch) ([94b273c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/94b273ceb21252dd87ee451ebc747b4ac08fcf0e))
* **ci:** include session/ent/ in build artifact for test and cross-compile jobs ([3d823f1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3d823f1169182cb82204649505610d67c77c668f))
* **detection:** detect ✻ asterism prefix as Claude active state ([#114](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/114)) ([a339b29](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a339b29c64117e3ec647f7a901a727562b79646a))
* **headless:** resolve all review findings — atomicity, security, concurrency, test coverage ([6f2cb76](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6f2cb76fb287820f34f38b9a5113eb89326c523d))
* **lint:** remove embedded ReviewState selector in sweeper test (QF1008) ([3fbcac9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3fbcac9f25cee49d421a9be8e2332acc501f94a8))
* **lint:** remove redundant embedded field selector in sweeper test ([7b437d8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7b437d8037d873a8b2fe411cda950df9a50efc9f))
* **lint:** resolve prealloc violations and remove unused shellWgWait ([29c52c5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/29c52c5809efa20d0b830c3211850a61be95e161))
* **lint:** use LiveInstancesProvider and Active instead of deprecated LoadInstances/Running ([a99d967](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a99d967fdb023888e9d1fba4efb560a6fb82e597))
* **log:** prevent panic on service restart; fix ESLint violations ([38f666f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/38f666f0665ba7bc042deb6f9d9a66c97a02e735))
* **macos:** stable TCC permission grants via self-signed codesigning ([#112](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/112)) ([3cc037f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3cc037fb0eaa3f151d2acfdc6ca04b72e5d7e113))
* **memory:** address code review findings — cache I/O, RSS coverage, error UX ([8cc95ba](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8cc95ba0f6b5e93296af2549d0e84126619397f8))
* **memory:** fix cache population, pause guard, and error UX for hibernate/pause ([27b5584](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/27b5584bd421acdc03cbf0de811aa333f149eff6))
* **notifications:** condition-change gating for health alerts + native notification lifecycle ([#92](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/92)) ([c9b08c5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c9b08c5241bf6c48059df3f1fbd2beaa2db51228))
* **notifications:** fix type mapping, approval guard, and test enum values ([#110](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/110)) ([c4b2ee3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c4b2ee346b76c74cf38db3bb52ae273b74b0ee48))
* post-merge build repairs for sync/upstream-20260528 ([dea6a86](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dea6a869ca86668d147914fcd27b8c9a96899046))
* **session:** fix two data races and semaphore ordering from upstream merge ([5f3208a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/5f3208a82425aa0323794faae1c968b4e60039f4))
* **session:** re-detect conversation UUID after /clear ([3ec93c4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3ec93c43ae0a307099b1f99c7592c02965d41bde))
* **session:** replace inst.tmuxManager.GetTmuxSessionName() with inst.GetTmuxSessionName() ([3651044](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3651044a4783502d1c17e664e57b0264653df4e7))
* **session:** replace time.Sleep with os.Chtimes in HistoryLinker tests to fix mtime race ([b731b6a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b731b6a560bb74580d7d5841ea95eea7010277e1))
* **session:** replace undefined NeedsApproval with detection layer check ([df56305](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/df563054c15be35f1983a7fc2b1baf969019e170))
* **sessions:** cascade session deletion to review queue, notifications, and modal auto-advance ([ce14e75](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ce14e7586aa48a1e3789a42d3adfb194575f8e10))
* **test:** use Stopped status in seedInstance to avoid tmux spawn in LoadInstances ([6b2f9d9](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6b2f9d92ba2d9a9b7204ab54e17e6abae38a909b))
* **tokens:** use UTC in IsStale() to avoid timezone boundary false-positives ([b0cb29d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b0cb29d294271e7cbe1bceb7db300111a7d66b0c))
* **ui:** rename Gallery button to Images in terminal upload toolbar ([a23a124](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a23a124cbce4507c13001f7ffdf2e9df23b889d4))
* **ui:** show pane header tab strip on mobile (remove display:none at ≤768px) ([0eae9e6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0eae9e61b392cc6b96bb533e5cf6e5c2382bdb64))
* **upload:** extract FileList to array before input reset to fix Android upload ([c2ba4aa](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c2ba4aa156036a38143a0a029ff3c5845a635466))
* **upload:** fix stale upload tests and add disconnected-state UX fallback ([ebb8a1b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ebb8a1b3080babb41ef995c861d9386690f1ba02))


### Performance Improvements

* debounce omnibar Fuse searches and pre-sort sessions selector ([3231d75](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3231d75a5942fe77cc3bdf7d8cef99c90b125d15))
* eliminate hot-path allocs and reduce JS bundle size ([#107](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/107)) ([7e14c40](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/7e14c407435b01f312038fac1b7b3a933e595a84))
* eliminate LoadInstances per-request in WorkspaceService and stagger git-status bursts ([dc99f6d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/dc99f6d79823bdc2c7c7ea56f52bcdd92b4c38dc))
* hot-path alloc elimination, go-git low-alloc index scan, JS bundle reduction ([#111](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/111)) ([14cc562](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/14cc562140030c42635d2071af0c61e3fd011dcf))
* reduce goroutine wakeups, lazy-load xterm.js, memoize SessionCard, direct SQL updates ([#122](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/122)) ([4104b01](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4104b01dec64aeecc91d7a49e2b09437b32b3204))
* remaining enforcement tests + CI coverage ([e8c150f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e8c150f5dee4a3b1928a401dde48165998203a03))
* **session:** lock-free GetTimeSinceLastMeaningfulOutput via atomic shadow ([22d1de5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/22d1de574786e04f038a38b0f11c103a4070b9fe))
* **session:** lock-free shell registry + allocation hot-path fixes ([8b883f3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8b883f37052af83299ef4f3752f2e487813a2f59))
* TTL cache for DiffShortstat + zero-alloc enforcement tests ([98d503c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/98d503c756cb2b71f0e5e374fc2af869ef1d1df6))

## [1.30.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.29.0...v1.30.0) (2026-05-18)


### Features

* **insights:** token & spend monitoring dashboard with model-over-time chart ([#104](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/104)) ([6b682cb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6b682cbc02f25dcda630cce13724b38f5d94b01a))

## [1.29.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.28.0...v1.29.0) (2026-05-18)


### Features

* **backlog:** add full backlog management layer ([#74](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/74)) ([765b904](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/765b9049729d80d14181a228d23205b1c622bdd1))
* **files:** resizable tree panel, mobile layout, recent files, quick-open palette ([#81](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/81)) ([78ecb19](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/78ecb19c9e547de968c2ed107964574aba0bcc68))
* **mobile:** mobile UX fixes, Go concurrency fixes, reflect-and-fix enforcement ([790d10c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/790d10c01ead2a5a9a3affd9bada79a10203fbed))
* **mobile:** pane header mobile fixes and session row layout improvements ([b4e841e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b4e841e61814d6d1d195839031bf4cf9f0ff48d5))
* **ui:** dark slate theme, compact rows, onboarding modal, /help hub ([#79](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/79)) ([cf5b838](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/cf5b8380a4a75cfc416aa8102271ca890c64c5c8))
* **upload:** generalize file upload to accept any file type with chip UI ([#80](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/80)) ([fb70cf3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fb70cf315f4dd04156f2b0a5f6984c52ac7d923e))


### Bug Fixes

* **analytics:** serialize events as snake_case integers to fix null duration_ms ([0c2b35e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/0c2b35ee534fc4cdfbd3331882983c193962b3d3))
* **mobile:** keyboard visible on foldable widths; restore overflow menu in row view ([13c7950](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/13c7950b6ae38024c261062ae0f6bd0a75bd7b4a))
* **sessions:** pass full action set to SessionRow in row view mode ([58d8aa3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/58d8aa3f44ebba1be81bb4c29e57907955c73ef0))
* **tests:** use meaningful alt text on FileChipList thumbnail to fix role queries ([e67f63d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e67f63d7f6c323bd8d795d654e28f5460bf528b6))
* **theme:** add missing statusDot and transition tokens to contract ([9f045cf](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9f045cfe2d8ba6813b1ce93a32896324384623b9))


### Performance Improvements

* eliminate forkExec lock contention from hot subprocess paths ([#102](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/102)) ([79582d4](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/79582d49563347917065c73ee61d51dba5c9ac17))

## [1.28.0](https://github.com/TylerStaplerAtFanatics/stapler-squad/compare/v1.27.0...v1.28.0) (2026-05-15)


### Features

* **analytics:** pluggable analytics system with SQLite storage and ESLint enforcement ([#69](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/69)) ([74f6f28](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/74f6f2819e1df1b64a4e0b45950a60dc7a0bfee0))
* **create-session:** opt-in to create directory + git repo when path is missing ([#43](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/43)) ([6e0ebe7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/6e0ebe7fd0f56abfeb469c89055aaa2d5474d0c8))
* **events:** add sequence numbers and 1-hour catch-up replay to EventBus ([03bbfc7](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/03bbfc7e0ac6158ee778bed206d021253c300ce9))
* **files:** PDF/video inline viewer and ranger-style keyboard navigation ([#71](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/71)) ([f753a86](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f753a86da0624aae2e2dfe1020cf35da9e134c03))
* **mobile:** add "+" pane button to mobile tab strip for split creation ([178d893](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/178d8937b729e083d099b826fda2c445d441726d))
* **pane:** add action bar with V/H keyboard shortcuts to pane picker ([9153f7e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/9153f7ef74dd81c2758b184a08c3f7945eba36d8))
* **pane:** keyboard-driven pane picker and smart session routing ([f7b437e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f7b437eba59bbc2661bdc675d1fbaa34bd99aa02))
* **pane:** open session in new pane via Alt+click, overflow menu, and omnibar button ([37cb863](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/37cb86374007afdff3b0f55f57ae711aa6ca1a33))
* **queue:** event-driven review queue — eliminate 2s polling lag for controller sessions ([#68](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/68)) ([c66db93](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c66db9383eda0f5ae5f8acdcc436429b68c99fd8))
* **terminal:** robust resize quiescence, scrollback, and mobile gestures ([#67](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/67)) ([57ae1c1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/57ae1c16fa8a1c681e4db7b3a2c04b6529ee2176))
* **ui:** replace emoji and unicode icons with Lucide components ([f97ed72](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/f97ed72f5011ecebeed11953232c2f0103b7976e))
* **unfinished:** count untracked files in DiffShortstat; default to GoGitVCSReader ([d2fe92e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d2fe92e90a947b26677a96dc4feff3d94d1a8528))
* **unfinished:** VCSReader interface with CLI-git, go-git, and jj implementations ([e2c396c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e2c396ca7900747e9e75643219ab85304e84cac8))
* **ux:** cyberpunk theme system, cockpit layout & keyboard shortcuts ([#51](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/51)) ([e538e4b](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e538e4b9e1a5e4121d5d6a448a8a0bc11d7b3dc6))
* **ux:** notifications page, nav badge, and smart pane targeting ([49bae61](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/49bae61ba9e349cb99b3cceaf968a478065f4f31))
* **ux:** tmux-style tiling pane engine with resizable splits ([8b45f92](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/8b45f92dbdb1b45999fa4fa02d7b24285be016ce))
* **ux:** unified tmux-style tiling — any pane can show session list or detail ([c598ff1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c598ff16b193dfd59eedf27b6e1473455068c65e))
* **ux:** UX polish pass — session list split, toast, toolbar, card density ([123d24c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/123d24ce3f932b827bf4512cc7266b5a5b9e5a7c))


### Bug Fixes

* **a11y:** fix WCAG AA color-contrast violation in reset-layout button ([670fab2](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/670fab2e9164e165cfb34320385c10b35ca3aa51))
* **a11y:** suppress card fade animation under prefers-reduced-motion ([d0377bd](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/d0377bdd343f402e88d60266e7a848a99a05c91b))
* address is-it-ready review findings ([e038ef6](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e038ef6b4beaed0967644b06fc511c96f17c6353))
* **analytics:** replace analytics-exempt with real track() calls in page.tsx ([ee0eb16](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ee0eb1661da746fd4e1859ed7e2be7116a7749d1))
* **build:** move vanilla-extract style to .css.ts file ([38a6806](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/38a6806e06c4acd7d30b72635540a31cc74396c1))
* **ci:** resolve pre-existing benchmark path and analytics lint failures ([77e2d40](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/77e2d4086177143d60200dbe2ce62018862d65e3))
* correct bugs found in second is-it-ready pass ([c83e653](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c83e6531e0f8dfa7c09d46783f75e1a27e6ac1c4))
* **deps:** address PR review comments ([64eafd5](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/64eafd5e47246e5822c8af9a92d25500255f0e91))
* **deps:** fix InstanceReader docstring and remove duplicate StateStore init ([a9b9b75](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/a9b9b75366389b8d1952742fa05b9c425d1ac30b))
* **executor:** bound all blocking test reads/waits with 10s timeout helpers ([b309d96](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b309d96ef56d30218529e6f9101c0629a06e761b))
* **executor:** bound stderr test with context timeout to prevent hang ([45b7444](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/45b7444abd42e7c9da3002346f4c3277fbcdaee3))
* **frontend:** resolve TS type error in SessionDetailView + add planning docs ([43bbef3](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/43bbef364dba4e9c9666ff68e2ee0c7cdf68faa6))
* **lint:** resolve all pre-existing ESLint errors in full-scan mode ([b856ef8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b856ef8f3e3a974c9f22651350b466be03a366bd))
* **lint:** resolve all pre-existing ESLint warnings (react-hooks, a11y, next) ([b912a58](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b912a58456df500effcbb4a472fb08e1e9c07bd6))
* **lint:** update norawexec analyzer and add nolint directives to test helpers ([468a470](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/468a4709ebae20c454d0060b3338316c4d651453))
* **mobile:** comprehensive mobile UX improvements — pane switcher, toolbar, keyboard ([#61](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/61)) ([ec75da8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ec75da8e660751fbc436ea160f4bd0d1435f8ea6))
* **mobile:** restore scroll on sessions page by constraining leafContainer height ([b1d23e8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b1d23e8ec27e3dd4825678dcc4aa94def726d91b))
* **mobile:** session navigation, list scrolling, and bottom nav height ([#63](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/63)) ([c669e74](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c669e74482a28dc2daca31d755f74fe4dba0c04d))
* **mobile:** show session tab bar on mobile when embedded in pane ([20f92d1](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/20f92d15de6f1f2fb93b462fa0b512652f42e2e4))
* **nav:** show badge counts on icons when sidebar is collapsed ([#70](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/70)) ([e9582b8](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/e9582b81e87d91d84f07546019640007bd7648ca))
* **nav:** surface Notifications and Settings in desktop header bar ([4fc2a86](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/4fc2a8697915d54da7957eee4f0fefdbc6d72fde))
* **nav:** unify DrawerNav with NAV_PAGES — add Notifications, Settings, Unfinished ([21be016](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/21be0165b8b8d0d99e6495e3f8f7333be4b30044))
* **notifications:** publish EventApprovalResponse to event bus for cross-device sync ([133067f](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/133067f8db226d714e3b7e7f36c1051142c25055))
* **notifications:** resolve session title in approval and question notifications ([b6e2b57](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b6e2b5705a3fe210d20f4b4a42773d909d0f1989))
* **pane:** picker always shown with 2+ panes; sessions moveable between panes ([fd1e010](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/fd1e0105762694580207f07705864341a4515718))
* **pane:** prevent double picker on sessions stream update ([339bb07](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/339bb07c82802085681096bd8d7d8bb41af67646))
* resolve all non-blocking review findings ([27602fb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/27602fb5829f97d4fd49d617fa83ef2d149f991d))
* **review-queue:** evaluate controller sessions on every poll cycle ([#72](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/72)) ([910cd19](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/910cd193bef6fbb511eb4917347c39faa3fa5183))
* **startup:** enforce tmux-before-sessions ordering with proof token ([94420fc](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/94420fc92cc1b3ad8863691063957ae313cecc16))
* **startup:** wire notification store before subscriber + tiling pane fixes ([1523b0d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/1523b0d3e2aed0152d72ce485d22feb65d89e452))
* **terminal:** correct snapshot rendering after resize ([3851540](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/3851540dbe81cc839d260863df4bc79e04ac9b73))
* **terminal:** sync cursor position after snapshot replay and break resize oscillation ([347d991](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/347d991a581f6a059ffe9f584ae6dd2ae4ca7c19))
* **tests:** update type assertions, mock setup, and e2e fixtures ([c188efe](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/c188efeaec144ade267ed0566552af5c9b5d7330))
* **test:** update makeStaleInstance to access cache via contentProvider ([ce627fb](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/ce627fb0631485a7aa8ea63e6828062ecec01f83))
* **types:** make isValidTab a proper type predicate for SessionDetailTab ([bb3f99a](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/bb3f99aada248cab6a69d67386072b0ff58c4d7c))
* **unfinished:** add --no-optional-locks to all scanner git commands to prevent index.lock contention ([28c1241](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/28c12412f310dff24061b24e05b869af9e12890b))
* **ux:** stop review queue page from spawning a competing WebSocket stream ([b1fff3d](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b1fff3d65306c3c8ebd006e9274a72cfd7aaf717))


### Performance Improvements

* fix all hotpolllog violations from InfoLog extension (PerfFix-2/3/4) ([b81da4e](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/b81da4e07780b0686f81a58a0811cefd9eac4ec1))
* **log:** async writer eliminates log mutex contention in hot poll loop ([11f2d3c](https://github.com/TylerStaplerAtFanatics/stapler-squad/commit/11f2d3c8b9e524e71757edf6dc62c1cfe192deae))

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
