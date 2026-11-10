# Research: Features -- what "Bazel as build of record" must do beyond compiling

Date: 2026-11-01. All counts below were derived by command from `git ls-files` (tracked files only, so worktrees/third_party are excluded unless noted). Confidence labels: VERIFIED = command run or file opened; INFERRED = reasoned, not run.

## 1. Measured scale and sandbox-hostility (VERIFIED by command)

| Metric | Value | Command basis |
|---|---|---|
| Tracked `.go` files (excl. `third_party/`) | 3,373 | `git ls-files '*.go'` |
| `_test.go` files | 1,679 | `git ls-files '*_test.go'` |
| Packages with tests | 154 | unique `dirname` of test files |
| Tracked `go.mod` | 3 real + root (`tools/lint`, `tools/scanner`, `tuitest`) plus 3 spike modules under `project_plans/go-git-fork-full-git-replacement/implementation/spikes/S{4,5,6,7}-src` (exclude via `# gazelle:exclude`) | `git ls-files '*go.mod'` |
| `testdata/` tracked files | 530 | `git ls-files \| grep -c testdata/` |
| Test pkgs with at least one sandbox-hostile pattern (union) | **49 of 154 (32%)** | grep union below |
| Test pkgs mentioning tmux at all | 30 | `grep -liE tmux` |

Per-pattern, in test files (files / packages):
- `os.Getwd(`: 13 / 10 (cwd assumptions; Bazel runs tests in `$TEST_SRCDIR/<ws>/<pkg>` runfiles, not repo root)
- `"../` relative path literals: 59 / 23 (reach for repo-root files; need `data = [...]` or `rlocation`)
- `runtime.Caller`: 12 / 7 (source-path discovery; broken under sandbox/runfiles)
- `exec.Command("git"...)`: 10 / 8; `exec.Command("tmux"...)`: 3 / 2 (direct host binaries; need hermetic git/tmux in `data`/toolchain or `local` tag)
- `net.Listen|httptest.NewServer`: 104 / 19 (loopback is fine in Linux sandbox; ports are the risk only for fixed-port tests; `requires-network` not needed for loopback)
- `os.UserHomeDir|"HOME"`: 60 / 22 ($HOME is not set in the sandbox / differs; tests touching `~/.stapler-squad` need `env`/TEST_TMPDIR)
- `t.Chdir|os.Chdir`: 5 files

Top offender packages by combined hit count (Getwd, `"../`, git/tmux exec, net.Listen, UserHomeDir, Caller, `tmux.` references in test files; ranking is a proxy, not a failure prediction):
1. `server/services` 207
2. `session` 148
3. `session/tmux` 53
4. `config` 45
5. `session/sshremote` 32
6. `server` 32
7. `session/git` 29
8. `server/deliverygate` 18, `cmd/ssq-hooks` 18
10. `session/mux` 12, `session/git/backend/gogit` 10; then `session/gitwiring`, `session/git/native`, `server/mcp`, `log` (8 each)

Implication: ~1/3 of test packages need per-target treatment (`data`, `env`, `tags = ["local"]`/`no-sandbox`, or code fixes). The big three (`server/services`, `session`, `session/tmux`) are also the slowest packages (Makefile cites `server/services` race at ~20-22m timeouts, `Makefile:1164,1197`), so they need `shard_count`/`size = "enormous"` and `timeout` plumbing. A single Go test target is one package; rules_go does NOT shard inside a package except via `-test.run` per shard (`shard_count` is supported by rules_go via `TEST_SHARD_INDEX`; INFERRED, verify in Phase 3 spike).

## 2. Inventory of capabilities, Bazel mapping, failure modes

Legend for mapping: NATIVE = Bazel rule; WRAP = genrule/sh_test/run target around existing tool; OUT = stays outside Bazel (workflow/script, maybe invoking `bazel run`).

### 2.1 Build and codegen (Makefile)
| Capability | Evidence | Mapping | Edge cases / failure modes |
|---|---|---|---|
| Main binary build, LDFLAGS version stamp (`-X main.version=...` from `git describe`) | `Makefile:22,39,204-213` | NATIVE `go_binary` + `x_defs` + `--workspace_status_command` stamping | Stamping needs `--stamp`; `git describe --dirty` makes stamped actions uncacheable on dirty trees; keep stamp out of non-release configs |
| `CGO_ENABLED=1` global export; sqlite via mattn (cgo) | `Makefile:14`; `go.mod:38` (mattn), `go.mod:68` (modernc) | NATIVE with C toolchain | Divergence: goreleaser pins `CGO_ENABLED=0` (`.goreleaser.yaml:14,28,40`) and `goreleaser-check.yml:80-143` exists specifically to guard a cgo-only-driver regression. Release binaries and dev binaries are already different builds; Bazel must pick one policy (this is the strongest argument for modernc/pure-Go and a single hermetic config) |
| macOS Info.plist embed via `CGO_LDFLAGS=-sectcreate` + `otool` verify, then codesign | `Makefile:204-210,555 (_codesign-binary)`, `scripts/install-service.sh:415-449`, `scripts/setup-codesign.sh`, `check-codesign.sh` | WRAP: `go_binary` `cgo_linkopts` select on darwin + post-link signing as a `genrule`/`bazel run` step outside the cache | Signing needs keychain identity (non-hermetic, local-only); sign must occur on the final file, outside sandbox. TCC grants persist only if identity stable. Needs device verification (owner blocker in requirements) |
| proto -> Go + connect-go + TS via `buf generate proto` into gitignored `gen/proto/go` and `web-app/src/gen` | `Makefile:645-660` | NATIVE (rules_proto/`go_proto_library` + connect plugin) or WRAP (`buf` as genrule tool) | Stamp-file logic in Makefile replaced by action graph. TS plugin `protoc-gen-es` comes from web-app `node_modules/.bin` (`Makefile:650`), i.e. proto-gen depends on a JS install: cross-language edge. Gazelle proto mode vs `# gazelle:proto disable` decision |
| ent generate `--feature sql/upsert`, output gitignored `session/ent/*` | `Makefile:662-670` | WRAP: genrule running `ent generate` (needs `go run -mod=mod`, which reaches network: not hermetic) | `-mod=mod` mutates go.mod/go.sum and downloads; must be replaced by a vendored/`go_deps` ent binary. Gazelle can't see generated package: needs `# gazelle:resolve` or an extension. Output is a package whose files are not in the tree |
| web build -> `server/web/dist` consumed by `//go:embed all:dist` | `Makefile:294`, `server/web/embed.go:8` | NATIVE (`aspect_rules_js` + bun/esbuild) feeding `go_embed_data`/`embedsrcs` | `make stapler-squad` depends on `server/web/dist` (`Makefile:204`) -- cross-language dependency edge. Web build mode toggle `WEB_BUILD=next/bun` (`FORCE-web-build-mode`, `Makefile:251`) is an extra axis; pick Bun only |
| `bun install` with shared node_modules across worktrees | `Makefile:237-249`, `tools/shared-node-modules/` | NATIVE (rules_js lockfile translation) replaces this | This tool exists *because* of the worktree-cost problem; Bazel repository cache + disk cache supersedes it. `bun.lock` support in rules_js is an open question (requirements) |
| Embedded tmux (`embed_tmux`), tymuxd (`embed_tymux`) | `Makefile:384-456`, `session/tmux/binary_embedded.go:22`, `session/tymux/binary_embedded.go:23`, `scripts/build-tmux.sh`, `scripts/fetch-tymuxd.sh`, `scripts/tymuxd-checksums.txt` | tmux: WRAP genrule/`rules_foreign_cc` (removed once, `b51b60eb1`); tymuxd: `http_file` with sha256 from `tymuxd-checksums.txt` (native fit) | `go:embed embed/tmux` needs the file inside the package dir: need `embedsrcs` or copy genrule. `fetch-tymuxd.sh` network fetch becomes `http_file` (hermetic win). tmux needs libevent/ncurses: pin or static. Release binaries need `--//:embed_tmux` flag and `gotags` via config_setting |
| otelc variant (`stapler-squad-otel`; **Superseded 2026-11-03 (owner decision, PROVISIONAL, branch `sdd/bazel-otel-default`):** now the one binary, S15 `otel_weave` action, not an exception) | `Makefile:467-475`, `scripts/otel-auto-build.sh`, `//go:build otelcauto` (2 files) | OUT / WRAP: otelc rewrites the Go toolchain build (`go build -toolexec`); Bazel go rules do not support it | Likely stays a Make/script target running native `go build` after Bazel-built codegen; document as a permanent exception. `otel-auto-isolation-guard.sh` checks the main build is unaffected, which Bazel trivially satisfies |
| Auxiliary binaries (`ssq-hooks`, `claude-mux`, `tools/scanner/backend/cmd/scanner`, `cmd/*`) | `Makefile:148,341,352` | NATIVE `go_binary` | scanner is in a separate module (`tools/scanner/go.mod`): see 3.x |
| Version/CHANGELOG embed | `changelog_embed.go:9` (`//go:embed CHANGELOG.md`), `pkg/changelog/embedded.go` | NATIVE `embedsrcs` | release-please rewrites CHANGELOG.md; fine, it is a source file |
| `server/analytics/client_metrics.go:76` embeds `client_pages.json` | file | NATIVE `embedsrcs` | Generated by `docs-gen`/registry? (INFERRED) check freshness gate |

### 2.2 Test matrix
| Capability | Evidence | Mapping | Failure modes |
|---|---|---|---|
| `make test` = `go test -short -v ./...` (gotestsum) | `Makefile:687-706` | NATIVE `go_test`; `-short` via `args`/`--test_arg=-test.short` in a `--config=short`; gotestsum output replaced by BEP/`--test_output=errors` | JUnit/pretty output expectations of CI; `--test_output` differences |
| `test-race`: `-race` (all) | `Makefile:788`, CI | NATIVE `--@io_bazel_rules_go//go/config:race` config | race needs cgo toolchain; doubles compile time and cache keys |
| `test-integration`: `-race -tags integration -p 1` for `./session ./session/tmux`, 20m, then rest | `Makefile:793-806` | NATIVE `--@io_bazel_rules_go//go/config:tags=integration` config + `exclusive`/`tags=["exclusive"]` or `local_jobs` for the serialised pair | `-p 1` equals Bazel `exclusive` tag; real tmux required; session/tmux needs a private tmux socket (see `STAPLER_SQUAD_TMUX_SOCKET`, CLAUDE.md) or tests collide across parallel Bazel shards |
| `gitoracle` tag tests (15 files), `-race`, 10m | `Makefile:812-816` | config `--gotags=gitoracle` | needs real `git` CLI as oracle: hermetic git or `local` tag; byte-diff oracle compares against host git version, so pin it |
| `sinkguard` tag (7 files), iterated per dir with `-run` filters | `Makefile:819-830`, `build.yml:1158,1201` | config `--gotags=sinkguard` + per-target `args=-test.run=...` | Makefile loops dirs and builds `-run` regex from names: that logic must be reproduced in BUILD (`go_test` per tag) |
| `harness` tag triage tests, `RealClaude` | `Makefile:831-847` | OUT (manual, needs real Claude) | skip in Bazel; tag `manual` |
| `live_github` tag (1 file), `ignore`, `tools` tags | go:build census | `manual`; `tools` tag file needs `go_deps` handling | gazelle must be told `-build_tags` or tag files get excluded: `# gazelle:build_tags integration,gitoracle,sinkguard,harness,live_github`. rules_go compiles only the tag set of the *config*, so each matrix is a transition/config, not extra targets |
| `test-with-pinned-tmux` `TMUX_BIN=...`, `test-tymux-pin` | `Makefile:859-860,1114`, `tools/ci/check-tymux-pin.sh` | WRAP: `env = {"TMUX_BIN": "$(rootpath //third_party/tmux:tmux)"}` | tmux built from submodule becomes a real data dep: improves hermeticity |
| Coverage: `-coverprofile`, `coverage-func/gaps/pkg/refactor`, `tools/coverage`, CI `coverage-delta.js` | `Makefile:730-786`, `tools/coverage/`, `tools/ci-gates/coverage-delta.js` | NATIVE `bazel coverage` + `--combined_report=lcov`; convert lcov for existing delta gate or keep via adapter | rules_go coverage emits lcov; Go-native cover profile tooling (`go tool cover`) needs conversion; `-covermode=atomic` |
| Benchmarks (`go test -bench`, soak, baseline-compare) | `Makefile:877-890,1288-1316`, `.github/workflows/benchmark.yml` (766 lines), `tools/ci-gates/benchmark-regression.js` | WRAP: `bazel run`/`--test_arg=-test.bench` with `tags=["manual"]`; or OUT | Benchmarks need noisy-neighbour-free, uncached runs (`--nocache_test_results`, `--runs_per_test`). Bazel's sandbox/caching must not memoise results |
| pprof/trace targets | `Makefile:862-875,1242-1335` | OUT | `-cpuprofile` flags with absolute `/tmp` outputs: keep as `go test` direct |
| `test-affected` (`scripts/test-affected.py`) and `test-affected-web` | `Makefile:708-722`, `scripts/test-affected.py` | NATIVE replacement: `bazel-diff` or `rdeps(//..., set(changed))` via `bazel query` | This is the headline CI-speed deliverable; needs git diff base and a stable query; submodule/`go.mod` changes invalidate everything |
| Shell tests (`test-shell`, `*.test.sh`) | `Makefile:973`, `scripts/*.test.sh` | WRAP `sh_test` | Many assume repo-root cwd and `git`; `shellcheck` lint via `lint-shell` (`find`-based list, `Makefile:957-971`) |
| Jest web tests | `CLAUDE.md` `cd web-app && bunx jest`; `Makefile:724 test-web-build` | NATIVE `jest_test` (rules_js) | needs node_modules sandbox; snapshot files; slow cold install |
| Playwright e2e | `tests/e2e/helpers/test-server.ts:57-94` runs `make stapler-squad` (line 74) unless `TEST_SERVER_BINARY` set; `build.yml:355,562` prebuild binary jobs since Playwright image has no Go | WRAP: `TEST_SERVER_BINARY=$(bazel cquery --output=files //:stapler-squad)`; change test-server.ts to `bazel build` or require env | The "make stapler-squad as dependency chain keeps e2e from serving stale frontend" comment (`test-server.ts:65`) becomes automatic under Bazel. Do not make Playwright a `sh_test` inside the sandbox (browsers, ports, long); keep as `bazel build` + `bun run test` outside, tagged manual/external |
| e2e port/tmux isolation | CLAUDE.md E2E section (`--test-mode --test-dir`, private tmux socket) | unchanged | Already hermetic by design; works if binary is provided |

### 2.3 Lint and gates
| Gate | Evidence | Mapping | Notes |
|---|---|---|---|
| golangci-lint (534-line config), `CGO_ENABLED=0 golangci-lint run` | `Makefile:932-940`, `.golangci.yml` | OUT (wrapped via `bazel run //tools:lint`) or `nogo` for a subset | golangci needs full type info via `go list`; works on a normal `go` checkout, so requires generated code to exist on disk (see 3 gopls/go build). Prefer: `bazel run` wrapper that first materialises generated code, or rules_go `nogo` for vet-class checks |
| custom analyzers (`make lint-custom`): 17+ analyzers in `tools/lint` module: entfullscan, hotpolllog, noarchivedrevival, nocommandpattern, nolegacylog, noliveinstanceraw, norawdb, norawexec, norawghrequest, norawgitcli, norawgitopen, nosessiondrivertest, nowrongdbentity, notimesleeptest, novartestseam, silenttransition, tmuxsocketscope | `Makefile:948-955`, `tools/lint/` (own `go.mod`) | NATIVE candidate: `nogo` (rules_go) with these `analysis.Analyzer`s -- runs per-package on every build, cached. Requires analyzers be importable from the main module (separate module blocks it) | `.claude/rules/*.md` reference these as the structural enforcement. Moving to nogo changes failure from "make lint" to "bazel build". Test fixtures use `testdata/` via analysistest (cwd-sensitive). `go -C tools/lint test ./norawgitcli/...` runs separately |
| `dupl` new-code-only (`--new-from-rev=origin/main`), `ready-complexity-gate` (gocyclo/gocognit/funlen/revive) | `Makefile:1117-1137`, `lint.yml` | OUT: diff-based gates are not a build graph concern | keep wrapped; needs git history (origin/main) which the sandbox lacks |
| jscpd threshold gate (0.14%) | `Makefile:1144`, `web-app/.jscpd.json`, `scripts/check-jscpd-threshold-doc.sh` | OUT / `sh_test` | non-hermetic scan of working tree |
| actor-field-guard, ptmx-field-guard (grep-based) | `Makefile:1212,1227` | WRAP `sh_test` with `data = glob(...)` | Source-grep tests need whole-tree data deps: coarse, invalidated by any change |
| vet-architecture, vet-rpc-markers, registry-generate/diff (`docs/registry/features`) | `Makefile:909-931,146-182` | WRAP genrule + diff test | registry-generate writes into tracked tree: model as `bazel run` updater + `diff_test` freshness check (the standard "write_source_files" pattern) |
| CSS tokens lint, ESLint zero-warning gate, stylelint | `Makefile:984`, `ready` (1108-1112) | NATIVE rules_js `eslint_test`, or OUT | |
| nil-safety (nilaway), staticcheck, gosec, deadcode, checklocks | `Makefile:1020-1066` | OUT, or nogo plugins (nilaway can run as nogo analyzer) | |
| `tools/scanner` tests, `tools/ci-gates` JS tests (own `bun.lock`), `tools/ci/*_test.sh` | `Makefile:1110-1112` | NATIVE (needs separate go.mod handling) / `sh_test` | tools/ci-gates has own bun lockfile: a second rules_js npm root |
| workflow lint (`lint-workflows`, actionlint) | `Makefile:1138` | OUT | |
| shellcheck over all `.sh` | `Makefile:957-971` | OUT or `sh_test`/aspect | |

### 2.4 CI (18 workflows, 4,958 lines total; `build.yml` 1,253)
- Change detection regex lists: `build.yml:87-95` (one very long `grep -qE`), `mcp-integration.yml`, `tools/ci/detect-feature-changes.sh`. Replace with `bazel-diff`/`rdeps` over changed files from the GitHub compare API (they deliberately avoid clone: `.git` is ~16GB, `build.yml:78-84`). Failure mode: Bazel needs a full checkout (no sparse/shallow) to build, so a cheap pre-job without checkout must stay; use API file list -> `bazel query` in a job that does checkout only when `relevant`.
- Jobs: `detect-changes`, `prepare` (proto + web once, artifacts, `build.yml:107`), `web-checks`, `e2e-binary-{next,bun}` + shards (`build.yml:355-760`), `test-affected` (`:855`), `go-guards` (`:1169`), plus lint.yml, mcp-integration.yml, nightly.yml, benchmark.yml, ux-analysis.yml, css-differential.yml, website-ci.yml, deploy-website.yml, demo-publish.yml, e2e-video.yml, goreleaser-check.yml, release-please.yml, release.yml, ci-duration-trend.yml, backlog-scaffolding-guard.yml, `_prepare.yml` (reusable). Several (website, demo, deploy-website, release-please, backlog guard) are not Go builds and stay OUT.
- Under Bazel, the `prepare` artifact fan-out job largely disappears *only if* a remote or restored disk cache is shared; on free runners, `actions/cache` of the disk cache is the stand-in (cache size/eviction limits 10 GB per repo is INFERRED, verify). `.claude/skills/bazel-github-actions-ci` and `bazel-perf` exist as local reference.
- `tools/ci/duration-trend.sh`, `count-pr-checks.py`, `runner-utilization.py`: CI-observability scripts to extend with BEP/profile metrics (requirements' observability item).
- Required status checks: branch protection names jobs; renaming/merging jobs under Bazel changes required-check names (owner action, INFERRED).

### 2.5 Release and distribution
- `.goreleaser.yaml`: builds for linux/darwin/windows with `CGO_ENABLED=0` (`:6-40`), windows/arm exclusion (`:47`), generates `Formula/stapler-squad.rb` ("generated by GoReleaser, DO NOT EDIT"). `goreleaser-check.yml` validates a CGO=0 binary.
- Options: (a) keep goreleaser, feed it Bazel-built binaries via `builds: - builder: prebuilt` (goreleaser Pro feature? INFERRED, verify) or run goreleaser's own `go build` (leaves two build systems for release, contradicting "build of record"); (b) `rules_pkg` tarballs + a script to compute sha256 and render the Formula (own the Formula generator). Cross-compiling Go with cgo for darwin from Linux under Bazel needs a hermetic C toolchain; with modernc (pure Go) it is trivial. This ties the sqlite decision to release design.
- release-please (`release-please.yml`, `.release-please-manifest.json` = 2.8.0): independent of build; stays OUT. Note Formula shows version 2.2.0 vs manifest 2.8.0 (Formula appears stale; not Bazel's problem but flags that the Formula pipeline is not currently exercised).
- Windows: 23 `//go:build windows` files + goreleaser windows target. Decision needed; dropping windows from Bazel `//...` (tag `manual`/`target_compatible_with`) is cheap, but goreleaser still ships it unless release moves.

### 2.6 Deploy
- `scripts/install-service.sh` (1,011 lines): locates binary by `command -v` or `$(pwd)/stapler-squad` (`:184-189`), branch/commit annotation (`:206-247`), systemd user unit / launchd plist generation, port-free handling, signing verify, ssq:// handler registration. `Makefile:512-565` backup-binary, rollback, `_codesign-binary`, `sync-worktrees`.
- Mapping: OUT, but its input changes from `./stapler-squad` to `bazel-bin/stapler-squad` (a symlink into the output base; must `cp` out to a stable path since launchd/systemd `ExecStart` must not point into a volatile output tree, and `bazel clean` would break the service). `make install-service` becomes `bazel build //:stapler-squad_release && cp && scripts/install-service.sh`. `install-service-preflight.test.sh` stays `sh_test`/OUT.
- CLAUDE.md rule: manual testing uses `~/.stapler-squad/manual-builds/manual-N/`; a `bazel run`-based or `cp` flow must keep that and the `STAPLER_SQUAD_TMUX_SOCKET` isolation. Do NOT `bazel run //:stapler-squad` from agent sessions against the live port.

## 3. Unstated needs

1. **`go build` / `go test` must keep working** for contributors, agents, and tools (golangci-lint, gopls, nilaway, otelc, goreleaser-check, e2e Playwright image). The repo's generated code is gitignored (`gen/`, `session/ent/*`, `web-app/src/gen`), so plain `go build ./...` currently needs `make proto-gen ent-gen`. Under Bazel the generated outputs live in `bazel-bin`, invisible to `go`. Need a decision: a `bazel run //:sync_generated` (or `write_source_files`) that materialises generated code into the gitignored paths for IDE/go tooling, keeping `go build` functional on the same layout. Without it, the dual-tool story ("`go build` still works") fails. INFERRED but low risk.
2. **IDE/gopls**: rules_go provides `@rules_go//go/tools/gopackagesdriver` (set `GOPACKAGESDRIVER` via a wrapper script; VS Code/`gopls` settings). It is slow on a repo of this size (INFERRED) and requires generated sources be in the Bazel graph. Alternative is the materialise-to-disk option above (better for agents; kibitzer/gopls MCP likely use plain go list). Recommend materialise as default and gopackagesdriver as optional.
3. **Agent-friendliness**: agents run in many parallel worktrees (memory notes: WIP cap of 2 after OOM incident; worktree deletion incidents). Needs: a `bazel`-oriented skill + `CLAUDE.md` section (existing local skills `bazel-perf`, `bazel-github-actions-ci`, `bazel-version-upgrades` cover generic Bazel but not this repo); cheat-sheet mapping "make X" -> "bazel Y"; `.bazelrc` `--config=agent` (low `--jobs`, `--local_resources`, `--output_base` discipline, `--noshow_progress`, terse output); BUILD-file failure triage guidance ("missing dep" errors from Gazelle drift). The Makefile wrappers (thin) are the primary agent interface, so wrapper target names/semantic parity matter more than raw bazel usage.
4. **Gazelle freshness gate**: CI job `bazel run //:gazelle` then `git diff --exit-code`; pre-commit hook; plus a lint hook mirroring `.claude/rules` guardrails. Agent-written repo makes drift the most likely daily failure (new file or import not in BUILD). Consider `gazelle_test`/`update_repos` for go.mod (`go_deps` extension handles `go.mod` directly, `go mod tidy` + `use_repo` sync is another drift point: `bazel mod tidy`).
5. **Worktree disk and cache sharing**: Bazel output base is keyed by workspace path hash, so each worktree gets its own output base (large). Mitigations: `--disk_cache=~/.cache/bazel-disk`, `--repository_cache=~/.cache/bazel-repo`, `--output_user_root` shared, `--experimental_...` none needed; consider `bazel shutdown` + `bazel clean --expunge` in the existing `worktree-disk-cleanup` skill (it currently doesn't know about Bazel output bases under `~/.cache/bazel/_bazel_$USER/<hash>`). Disk cache growth needs GC (`--experimental_disk_cache_gc_max_size`, available Bazel 7.4+/9; INFERRED, verify). Concurrent worktrees each start a Bazel server JVM (memory ~ GBs each): conflicts with OOM history; mitigate with `--max_idle_secs`, `--host_jvm_args=-Xmx`, shared server impossible across workspaces. Worktree creation inside `~/.stapler-squad/workspaces/*/worktrees` and `.claude/worktrees/agent-*` (nested inside the main repo checkout!) means Bazel in the main repo would glob into nested worktree dirs: need `.bazelignore` listing `.claude/worktrees`, `node_modules`, `third_party/tmux` build dirs, and `project_plans/**/spikes/*` modules.
6. **Quick iteration loops**: `make dev`/`web-dev`/`restart-web` (`Makefile:303-334`, bun dev server on 62875) rely on bun dev + `go run`. Keep these native for HMR; offer `ibazel run` optionally. `go:embed all:dist` coupling means web changes rebuild the Go binary under Bazel (dist is an input): dev loop should serve dist from disk (`WEB_DEV_HMR`) rather than via the embed edge.
7. **Submodules**: `third_party/tmux` is a git submodule (1 tracked entry). Bazel sees it as a directory only if initialised; fresh worktrees do not auto-init submodules (`init-submodules`, `Makefile:381`). Failure mode: `bazel build //...` on a fresh worktree fails unless submodule is present (or tmux comes via `http_archive`/`git_repository` instead of the submodule). Requirements claim "works on a fresh clone": `git_repository`/`http_archive` pinned to the same commit avoids this and also lets `.bazelignore` exclude the submodule path. Also `check-tymux-pin.sh` pins tymux version: keep it in sync with MODULE.bazel pin.
8. **go-git replace fork** (`tstapler/go-git`): `go_deps` honours `replace` via `go_deps.module_override`/reading go.mod (INFERRED; verify in spike). Separate modules (`tools/lint`, `tools/scanner`, `tuitest`) need `use_repo`/`go_work` or `local_path_override`; simplest is to fold analyzers into one module or use `go.work`-free multi-module via `go_deps.from_file` per module.
9. **Hermetic `$HOME`/network in tests**: set `--test_env=HOME=$TEST_TMPDIR`-style defaults in `.bazelrc` `test` section; verify tests that read real `~/.stapler-squad` (22 packages by grep) are the ones that fail first. Some tests (BUG-087..090 known flakes per memory) would become visible; repo policy is fix, not defer.
10. **macOS CI**: the macOS signing/Info.plist check cannot run on Linux runners; a darwin job (slower, paid minutes on free tier: macOS runners cost 10x, INFERRED) is needed for parity before cutover.
11. **Cross-platform `//...` hygiene**: use `target_compatible_with` for windows/darwin/linux files, or `bazel build //... ` fails on Linux for windows-only srcs; 41 `!windows`, 23 `windows`, 11 `darwin` build-tagged files (go:build census above).
12. **Fallback switch**: requirements call for `BUILD_SYSTEM=make` for one release cycle; wrappers must be written so each Makefile target keeps its legacy recipe under `ifeq`.

## 4. Recommended permanent exceptions (stay outside Bazel)
release-please, deploy of website/demo workflows, golangci-lint/dupl/jscpd diff gates (wrapped via `bazel run`), benchmark/profiling runs, harness (RealClaude) tests, install-service.sh (with Bazel-built input), codesigning, `tools/ci` observability scripts.

## 5. Open items for later phases (evidence needed)
- Spike: `go_deps` with `replace` fork + multiple modules (Q in requirements).
- Spike: ent genrule without `-mod=mod`.
- Spike: rules_js with `bun.lock` (not run; not verified here).
- Measure baselines (cold build, CI time-to-green, per-worktree disk) -- not done in this research task.
- Decide sqlite (modernc vs mattn): drives release cross-compile design and goreleaser CGO=0 divergence.
