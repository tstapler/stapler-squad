# Spike results

Host: onyx, Bazel 9.3.1 via bazelisk 1.28.1, run 2026-11-01. Bazel invoked with `--host_jvm_args=-Xmx2g --max_idle_secs=30`, one invocation at a time.

## S-gomod (Stories 0.2 / Tasks 0.2a-0.2d)

### Commands and output (VERIFIED, run in this worktree)

- `bazel version` -> `Build label: 9.3.1`.
- `bazel mod tidy` run twice; second run left `MODULE.bazel` and `MODULE.bazel.lock` byte-identical (`cmp` against a copy taken after the first run).
- `bazel run @rules_go//go -- version` -> `go version go1.26.6 linux/amd64` (SDK from `go_sdk.from_file(go_mod = "//:go.mod")`; no fallback needed, `go.mod` untouched).
- `bazel build //tools/lint/... //tools/scanner/... //tuitest/...` -> exit 0 (see exclusions below).
- `bazel test --config=agent //tools/lint/...` -> exit 0, 16 tests pass, 1 excluded (`tools/bzl/test-exclusions.yaml`).
- `bazel mod show_repo`: `@lint_x_tools` is `golang.org/x/tools v0.42.0`, `@org_golang_x_tools` is `v0.51.0`: resolved separately.
- Fresh-clone build: see "Fresh clone" below.

### Verdict

`S-gomod`: **Pass** on its stated criteria (tidy stable, `//tools/lint/...` green, Go 1.26.6 resolved), with the deviations and findings below. Path: direct (no workaround needed). Cost: M.

### Deviations from the plan / assumptions that were false

1. **`--enable_bzlmod` is not a documented flag on 9.3.1** (absent from `bazel help build`; accepted silently). Dropped from `.bazelrc`; bzlmod is the only mode. `check-bazelrc.sh` rejects `--enable_workspace` and `--enable_bzlmod=false` instead.
2. **Isolated extension usage needs `common --experimental_isolated_extension_usages`** (exists on 9.3.1). Also, an isolated `go_deps` usage needs its own `use_repo`, and the repo name must differ from the root's (`use_repo(lint_go_deps, lint_x_tools = "org_golang_x_tools")`), else `use_repo` fails with a duplicate-name error.
3. **Gazelle does not follow the isolated universe by itself.** `tools/lint/BUILD.bazel` carries `# gazelle:resolve_regexp go ^golang\.org/x/tools/(.*)$ @lint_x_tools//$1`; Gazelle 0.54.0 does substitute `$1` (relevant to S11's resolve_regexp counting rule).
4. **`go_deps.from_file` is allowed once per module** (`Multiple "go_deps.from_file" tags` error), so `tuitest/go.mod` cannot be a second tag. `tuitest` imports only modules already in the root `go.mod`, so it joins the root universe without a tag. A `go.work` was not introduced (it would change `go` behaviour for everyone).
5. **Gazelle generates BUILD files inside `testdata/` dirs** unless excluded; root `BUILD.bazel` adds `# gazelle:exclude **/testdata` (not in the plan's directive list).
6. **A systemd scope rejects `OOMScoreAdjust`**: `systemd-run --user --scope -p OOMScoreAdjust=500` -> `Unknown assignment: OOMScoreAdjust=500`. `tools/bazel` sets `/proc/self/oom_score_adj` to 500 inside the scope instead (wrapper_test asserts 500 and `MemoryMax=15032385536`).
7. **`startup:<config>` is ignored by Bazel**, so `--config=agent` cannot set `--max_idle_secs`; `.bazelrc` has `startup --max_idle_secs=300` and `tools/bazel` passes `--max_idle_secs=30` (non-tty or `BAZEL_AGENT`) and `--host_jvm_args=-Xmx2g` on the command line, because `~/.bazelrc` sets 8g (confirmed: the server command line carries `-Xmx8g` from the home rc and the final `-Xmx2g` wins). `check-bazelrc.sh` rejects `startup:<non-platform-config>` lines.
8. **Flags that do not exist on 9.3.1** and are not used: `--local_ram_resources`, `--local_cpu_resources`, `--incompatible_sandbox_hermetic_tmp`, `--experimental_guard_against_concurrent_changes` (grep of `bazel help build --long` returns 0). `--local_resources=cpu=...` and `memory=...` are used.
9. **Disk cache GC**: `--experimental_disk_cache_gc_max_size=100G`, `--experimental_disk_cache_gc_max_age=30d`, `--experimental_disk_cache_gc_idle_delay=10s` (default 5m never fires when the server exits after 30 s idle). No `--repository_cache` flag: the repository cache is already shared per user by default.
10. **`--remote_local_fallback` needs `--incompatible_remote_local_fallback_for_remote_cache`** for `--remote_cache`; both are in `build:ci-cache-read` along with the circuit breaker (`--experimental_circuit_breaker_strategy=failure`). Not exercised against a real cache (Epic 7 / Task 0.12).
11. **`analysistest` tests SKIP when `go` is not on PATH** (`analysistest.go:402: skipping because go tool not available`). Under the sandbox all 15 analyzer tests currently pass vacuously. The `//tools/lint/...` pass is therefore weaker than it looks; making them run needs the Go SDK in `data` and PATH (to be handled in Epic 4 / Story 0.13). Recorded as a finding, not fixed here.
12. **Two pre-existing failures surfaced**: `//tools/lint/silenttransition:silenttransition_test` reads `../../../session/transition_callers.go` (outside runfiles; class `cwd`); `//tuitest/integration/claude_squad:claude_squad_test` imports packages `tuitest/go.mod` does not require and already fails `go vet` (and needs `//testutil/wait`). Both are `tags = ["manual"]  # keep` and listed in `tools/bzl/test-exclusions.yaml`. Also `//tools/scanner/backend:backend_test` fails under Bazel (reads `tools/scanner/list-backend-protos.sh` from cwd; class `cwd`); it is outside the `//tools/lint/...` gate and is not yet excluded.
13. `bazel mod tidy` reports the root `go.mod` is behind Bazel's resolved versions (`x/net` 0.58.0->0.59.0, `x/sys`, `x/crypto`, `x/mod`, `x/sync`, `x/text`, `x/tools` 0.48.0->0.51.0): transitive requirements raise them. `go.mod` was not changed (owner approval needed); expect `go mod tidy` drift to be reconciled in Epic 1.
14. `tools/bzl/check-no-force-push.sh` needs an allow-list (`tools/bzl/no-force-push-allow.txt`) for the existing owner-run `scripts/git-history-cutover/04-push-and-notify.sh`, which contains `git push --mirror --force`.
15. `scripts/hooks/pre-push` (dispatcher) was added; git only runs hooks by exact name, so `pre-push-no-force.sh` alone would never run under `core.hooksPath`.
16. **Task 0.2i premise**: the live unit already has `OOMScoreAdjust=-500` in the unit file but `stapler-squad.service.d/oom.conf` sets `1000` on purpose (Claude sessions sacrificial), and the user manager clamps a requested -900 to its own 100 (measured). See `docs/how-to/protect-live-service-from-bazel.md`.

### Flag verification (Task 0.2b)

| Flag | Status | Evidence |
|---|---|---|
| --color | spike-verified | `bazel help` 9.3.1: `--color (yes, no or auto; default: "auto")` |
| --curses | spike-verified | `bazel help` 9.3.1: `--curses (yes, no or auto; default: "auto")` |
| --disk_cache | spike-verified | `bazel help` 9.3.1: `--disk_cache (a path; default: see description)` |
| --experimental_circuit_breaker_strategy | spike-verified | `bazel help` 9.3.1: `--experimental_circuit_breaker_strategy (failure; default: see description)` |
| --experimental_disk_cache_gc_idle_delay | spike-verified | `bazel help` 9.3.1: `--experimental_disk_cache_gc_idle_delay (An immutable length of time.; default: "5m")` |
| --experimental_disk_cache_gc_max_age | spike-verified | `bazel help` 9.3.1: `--experimental_disk_cache_gc_max_age (An immutable length of time.; default: "0")` |
| --experimental_disk_cache_gc_max_size | spike-verified | `bazel help` 9.3.1: `--experimental_disk_cache_gc_max_size (a size in bytes, optionally followed by a K, M, G or T multiplier; default: "0")` |
| --experimental_isolated_extension_usages | spike-verified | `bazel help` 9.3.1: `--[no]experimental_isolated_extension_usages (a boolean; default: "false")` |
| --experimental_remote_failure_rate_threshold | spike-verified | `bazel help` 9.3.1: `--experimental_remote_failure_rate_threshold (an integer in 0-100 range; default: "10")` |
| --experimental_remote_failure_window_interval | spike-verified | `bazel help` 9.3.1: `--experimental_remote_failure_window_interval (An immutable length of time.; default: "60s")` |
| --host_jvm_args | spike-verified | `bazel help` 9.3.1: `--host_jvm_args (a string; may be used multiple times)` |
| --incompatible_enable_proto_toolchain_resolution | spike-verified | `bazel help` 9.3.1: `--[no]incompatible_enable_proto_toolchain_resolution (a boolean; default: "true")` (not committed) |
| --incompatible_remote_local_fallback_for_remote_cache | spike-verified | `bazel help` 9.3.1: `--[no]incompatible_remote_local_fallback_for_remote_cache (a boolean; default: "false")` |
| --local_resources | spike-verified | `bazel help` 9.3.1: `--local_resources (a named double, 'name=value', where value is an integer, or a keyword ("HOST_CPUS", "HOST_RAM"), optionally followed by an operation ([-\|*]<float>) eg. "HOST_CPUS", "HOST_CPUS*.5"; may be used multiple times)` |
| --lockfile_mode | spike-verified | `bazel help` 9.3.1: `--lockfile_mode (off, update, refresh or error; default: "update")` |
| --max_idle_secs | spike-verified | `bazel help` 9.3.1: `--max_idle_secs (an integer; default: "10800")` |
| --remote_download_minimal | spike-verified | `bazel help` 9.3.1: `--remote_download_minimal` (not committed) |
| --remote_download_toplevel | spike-verified | `bazel help` 9.3.1: `--remote_download_toplevel` |
| --remote_local_fallback | spike-verified | `bazel help` 9.3.1: `--[no]remote_local_fallback (a boolean; default: "false")` |
| --remote_retries | spike-verified | `bazel help` 9.3.1: `--remote_retries (an integer; default: "5")` |
| --remote_timeout | spike-verified | `bazel help` 9.3.1: `--remote_timeout (An immutable length of time.; default: "60s")` |
| --show_loading_progress | spike-verified | `bazel help` 9.3.1: `--[no]show_loading_progress (a boolean; default: "true")` |
| --show_progress | spike-verified | `bazel help` 9.3.1: `--[no]show_progress (a boolean; default: "true")` |
| --test_output | spike-verified | `bazel help` 9.3.1: `--test_output (summary, errors, all or streamed; default: "summary")` |
| --test_summary | spike-verified | `bazel help` 9.3.1: `--test_summary (short, short_uncached, terse, detailed, detailed_uncached, none or testcase; default: "short")` |
| --verbose_failures | spike-verified | `bazel help` 9.3.1: `--[no]verbose_failures (a boolean; default: "false")` |

## Fresh clone

`git clone` of the branch into a short temp path, then: `bazel build --config=agent //tools/lint/... //tools/scanner/... //tuitest/...` exit 0 (239 actions, 156 served from the shared disk cache, so it is not a fully cold compile); `bazel test //tools/lint/...` 16 pass; `bazel mod tidy` left `git diff --exit-code MODULE.bazel MODULE.bazel.lock` clean; with a planted `.claude/worktrees/agent-x/BUILD.bazel`, `bazel query //...` shows 0 labels under `//.claude`; `grep -c node_modules` of the lint/scanner/tuitest query is 0.

## S3 embed from a tree artifact (Story 0.5, Tasks 0.5a / 0.5b)

Run 2026-11-01 in worktree `bazel-epic0-s4-bun`, Bazel 9.3.1 via `tools/bazel`, rules_go 0.65.0, bazel_lib 3.7.0 (added as `bazel_dep`).

### Commands and output (VERIFIED)

- `server/web/BUILD.bazel`: `copy_directory(name = "dist_staged", src = "//web-app:dist", out = "dist")` then `go_library(name = "web", srcs = ["embed.go"], embedsrcs = [":dist_staged"])`. `server/web/embed.go` is unchanged (`//go:embed all:dist`).
- `bazel build //server/web:web` -> exit 0 (rules_go 0.65.0 accepts the staged tree artifact as `embedsrcs` directly, no expansion rule).
- `bazel test --test_arg=-test.v //server/web/...` -> 2 of 2 pass. Real tree: `S3: index.html 1472 bytes sha256=9549cbe7...fcd7, 380 files embedded` (380 equals the S4 `dist` file count). Fixture tree from an action (`dist/index.html`, `dist/assets/app.abc123.js`, package `server/web/fixture`, which compiles a byte-identical copy of `embed.go`): `index.html 20 bytes, 2 files embedded`.
- Staging happens in the consuming package (`bazel-bin/server/web/dist`); `sha256` of the file list of the staged tree equals the `//web-app:dist` tree (`b722078d...aff7`).

### Verdict

`S3`: **Pass**, path `copy_directory+embedsrcs`, cost S. Both PASS checks hold (compiles; a test reads `index.html` from the embedded FS). Task 0.5b (cost both fallbacks) is **not applicable**: Task 0.5a did not fail, so the custom tree-expansion rule and the rules_go patch were not prototyped; their ADR-008 classes (M; M+L) stay INFERRED.

Findings:

1. A genrule cannot declare a directory output (`output ... is a directory but was not declared as such`); the acceptance-criterion fixture needs a tiny rule with `declare_directory` (`tree_fixture` in `tools/bzl/bun/bun.bzl`).
2. Two `copy_directory` targets in one package that both stage to `dist` collide on the output path (`conflicting actions`): the fixture lives in its own package (`server/web/fixture`). ADR-007's embed sites (tmux, tymuxd, changelog) each need a distinct staging path per package.
3. Any web change relinks `server/web` (all:dist embeds 380 files, 43 MB) as ADR-007 already notes.
4. `embed_spike_test.go` and the two BUILD files are spike-only; nothing in `server/web/*.go` besides that test changed.

## S4 hermetic Bun web build (Story 0.6, Tasks 0.6a-0.6d; 0.6e skipped, 0.6f not needed)

Run 2026-11-01, same worktree and toolchain. Pinned toolchains come from `http_archive`: Bun 1.4.3 linux-x64 (sha256 `3e730528...1e18`, byte-identical to the asdf `bun` 1.4.3 binary) and Node 24.13.1 linux-x64 (`.tool-versions`; sha256 `30215f90...1089`). `@web_node_modules` is a repository rule (`tools/bzl/bun/bun.bzl: bun_install`) that copies `package.json`, `bun.lock`, `bunfig.toml`, the two `file:` plugin dirs and `patches/` and runs `bun install --frozen-lockfile` (1449 packages, 0.7-1.5 s with a warm Bun download cache; the cache under `~/.bun/install/cache` is read through `HOME`, so a cold first fetch downloads from the registry). `//web-app:dist` is a rule (`bun_dist`) whose action stages the sources and a hardlink copy of `node_modules` into `_work/`, runs `bun build/build.ts` and publishes the tree.

### Task 0.6a / parity (VERIFIED: `bash tools/bzl/parity/compare-web.sh bazel-bin/web-app/dist server/web/dist` prints `identical: 380 files`, exit 0)

- File list and sha256 of all 380 files equal the reference `make web-build`; `allowed-deltas.txt` is empty (created; the unminified default build, `WEB_MINIFY=0`, has no banner). Three clean sandboxed builds and a no-sandbox build produced the same tree hash (`b722078d...aff7`); two of the sandboxed runs used `--disk_cache=` after `bazel clean`.
- **The reference is `make web-build` under a pinned environment, and this matters.** Against the default developer `make web-build` the trees differ in 174 of 380 files (chunk names change). Root causes found, each reproduced in a scratch directory:
  1. Bun 1.4.x's default isolated linker uses a global virtual store: `node_modules/.bun/<pkg>` symlinks to `~/.bun/install/cache/links/<pkg>-<hash>/`. Bun embeds paths relative to the realpath of `node_modules` in CSS source comments, source-map `sources` and `debugId`, so the output depends on the store location and hash. Fixed for the Bazel install with `BUN_INSTALL_GLOBAL_STORE=0` (or `globalStore = false` under `[install]` in `bunfig.toml`).
  2. The same embedded relative paths mean `node_modules` must be a real directory under the work dir, not a symlink (Bun resolves the symlink). The action therefore hardlink-copies the tree (`cp -al`), which is the dominant cost below. The shared-store layout of `tools/shared-node-modules` has the same property: dist bytes depend on where the shared store lives.
  3. The Node version changes bytes: with the host Node (26.8.1) the vanilla-extract CSS carries `/* ve-css:src/styles/theme.css.ts.vanilla.css?source=#<base64> */`; with Node 24.13.1 (and 25.8.1) it is `/* ve-css: */`. Host `bun` here was 1.4.4 while `.tool-versions` pins 1.4.3.
  The reference was therefore produced with PATH starting at asdf Node 24.13.1 and Bun 1.4.3, `STAPLER_SQUAD_SHARED_NODE_MODULES=0 BUN_INSTALL_GLOBAL_STORE=0 make -B web-app/out server/web/dist`. The default `make web-build` needs the same pins to be a stable parity reference (owner/Story 3.1 input: add `globalStore = false` and pin Node in the Makefile path, or compare against a pinned run).
- Determinism and path independence: 3 sandboxed runs in different `sandbox/linux-sandbox/N` directories are byte-identical. Not tested: a second worktree path or a second `--output_base` (E3/E4 territory, Task 0.10e).

### Task 0.6b hidden coupling (VERIFIED: `bash tools/bzl/spike/count-escapes.sh` prints `0 0`)

`strace -f -y` over the bun process inside the sandbox (`//web-app:dist_trace`, log `bazel-bin/web-app/dist_trace.strace`, 74,280 lines): 2,114 distinct successful reads (2,058 inside the work tree, 56 system loader/`/proc`/`/dev` or scratch `HOME`), 507 distinct writes (390 inside the work tree, 117 scratch `HOME`), 0 `socket`/`connect` syscalls, 0 failed probes outside the tree. Reads outside `web-app/` = 0, writes into the source tree = 0 (`git status` clean for tracked files after all runs). The action also builds with `--sandbox_default_allow_network=false`. Caveats, all real hidden coupling in the make path:

1. `web-app/build/stages/verify.ts:9` reads `server/middleware/testdata/public-allowlist.json` (`../../../server/middleware/testdata/...` from `web-app/build/stages`), outside `web-app/`. The action passes `--allowlist` to a staged copy (`//web-app:public_allowlist`, a `cp` genrule from `//server/middleware/testdata:public-allowlist.json`), so the measured action-level count is 0; at source level the count is 1 declared cross-package edge. Without the flag the sandboxed build fails. The Go middleware tests share this file.
2. `web-app/jest.config.js` lists `roots` outside `web-app/` for projects that are not run (`../scripts/dev-stack`, `../tests/e2e`); jest validates every project's roots even with `--selectProjects web-app` and fails with `Directory ... in the roots[0] option was not found`. The Jest test creates empty `scripts/dev-stack` and `tests/e2e` directories beside the work dir.
3. `src/app/spike-lazydoc/virtualDoc.ts` imports `@/lib/__tests__/virtualFile`, so a source glob that excludes `__tests__` breaks the production build.
4. `web-app/src/gen` is gitignored and listed in `.bazelignore`, so the spike removed it from `.bazelignore` and globs the on-disk files (from `make proto-gen`). In the real plan this comes from the Bazel proto TS action (Task 0.6e, skipped here because the proto spike runs in another worktree). The spike also added `web-app/node_modules` and `web-app/out` to `.bazelignore`.

### Task 0.6c Jest (VERIFIED: `bazel test //web-app:jest_subset` passes; `python3 tools/bzl/spike/compare-jest.py` exit 0)

20 test suites (231 test cases) from `src/lib/{backlog,board,cron,constants}` (list in `web-app/BUILD.bazel`). `bunx jest --selectProjects web-app` outside Bazel (pinned toolchain, local install): 20 suites, 231 tests passed. Under Bazel (`bun x jest` with the pinned Node on PATH): 20 suites, 231 passed; the set of `(file, test name, status)` is identical (only-a 0, only-b 0). Note the plan's "20 passing tests" is read as 20 suites.

### Wall time and sandbox cost (MEASURED, host load about 8, so noisy)

| Run | Wall |
|---|---|
| `make -B web-app/out server/web/dist`, warm install, 3 runs | 3.56 s, 4.72 s, 4.81 s (median 4.72 s; `build.ts` itself 2.2-3.1 s) |
| `//web-app:dist` sandboxed action, 3 clean runs (`--disk_cache=`) | 12.94 s, 12.35 s, 11.62 s (median 12.35 s, **2.6x**) |
| `//web-app:dist_nosandbox`, 2 clean runs | critical path 9.71 s, 6.99 s (**2.1x**, **1.5x**) |

Inside the sandboxed action: stage (tar of sources plus `cp -al` of `node_modules`) 5.3-6.4 s, `build.ts` 1.9-2.3 s, publish (copy of out/ plus delete of the hardlink tree) 1.7-3.1 s; action wall minus inner time is about 0.2 s, so **sandbox setup itself is small and the cost is the `node_modules` relocation forced by Bun's realpath-relative output**. The no-sandbox variant halves the stage step but cannot remove it. The ratio exceeds 1.5x, so per the S4 rule the verdict stays `Pass` with `path = repo-rule+no-sandbox`; the honest cost is that even no-sandbox is 1.5-2.1x make. A cheaper design (not tried, cap): stage `node_modules` once as a cached tree action rather than per consuming action, or make the make-path layout match the Bazel one.

### Task 0.6d fork versus workaround costing for `bun.lock` (prototype VERIFIED; numbers below are measured unless marked INFERRED)

- Prototype: `python3 tools/bzl/spike/bunlock_to_pnpm.py web-app/bun.lock 10` (64 lines) converts `bun.lock` into the pnpm-v9 `importers`/`packages`/`snapshots` shape that `aspect_rules_js` 3.5.2 `npm/private/pnpm.bzl` (390 lines) consumes. 10 direct deps -> 30 packages, 89 edges, 0 unresolved; the whole lock -> 125 direct, 1,586 packages, 3,658 edges, 0 unresolved (this repo's lock pins exact versions: 3,221 dependency edges, 35 non-exact).
- Not covered by the prototype and needed in a real reader: peer-dependency instance keys (pnpm's `(react@19.1.1)` snapshot suffix; bun resolves peers per install), `patchedDependencies` (`eval@0.1.8`), lifecycle/`trustedDependencies` mapping, `file:` deps, `npm:` aliases, registry tarball URLs, JSONC parsing in Starlark (trailing commas; `json.decode` rejects them).
- Fork row (`FallbackFork`): INFERRED about 300-400 lines in `rules_js` (a `bun_lock.bzl` reader of about 200 Starlark lines, wiring in `npm_translate_lock*.bzl`, tests), above the ADR-008 fork cap of about 200 lines; cost class L+L+M+S as in ADR-008; per-bump maintenance S to M: `pnpm.bzl` had 4 commits and `npm_translate_lock_helpers.bzl` 19 commits since 2026-01-01 (`gh api repos/aspect-build/rules_js/commits?path=...`). Upstream: only issue #1258 "Support for other runtimes like Bun" (open); no `bun.lock` reader issue found by `gh search issues`.
- Workaround row, the chosen repository-rule path (`Pass`): repo-owned Starlark is `tools/bzl/bun/bun.bzl`, 173 lines including spike probes (strace, timing, fixture) and the Jest rule; production-trimmed about 110. Within the 500-line cap. Cost M+M+S+S per ADR-008, measured effort matched. Per-bump maintenance: Bun pin sha256 per OS/arch (XS) plus re-checking the Node-version and global-store byte sensitivity above. Limits: no per-package granularity (one install, one opaque `node_modules` input; any dependency change rebuilds every web action), and Linux x64 only was pinned here (aarch64/darwin shas are in Bun's `SHASUMS256.txt`, not wired).
- Task 0.6f (`rules_bun` via `git_override`) was **not run**: the repository rule met the parity, escape-count and determinism checks, so the fallback is not reached. `rules_bun`'s status and bus factor remain INFERRED from ADR-003.

### Verdict

`S4`: **Pass**, path `repo-rule+no-sandbox`, cost M+M+S+S. Checks: (1) parity 380 of 380 files identical, 0 allow-listed deltas needed, **but against a pinned-environment `make web-build`; against the default developer environment the output differs in 174 files (explained above, not allow-listed), so a strict reading of the rule would make this `FallbackWorkaround`**: the orchestrator or owner should confirm the pinned reference; (2) Jest 231 of 231 equal; (3) escapes `0 0` (with the `public-allowlist.json` staging caveat); (4) wall time 2.6x sandboxed, 1.5-2.1x no-sandbox, above 1.5x so the `no-sandbox` path applies. Task 0.6e (S1-ts) skipped by instruction: no `S1-ts` row written here.

Owner/follow-up inputs from S4: pin Node and set `globalStore = false` (or `BUN_INSTALL_GLOBAL_STORE=0`) in the make path before treating make output as a parity oracle; consider moving `public-allowlist.json` into `web-app/` or giving `build.ts` an explicit input; do not rely on the host `bun`/`node` (host Bun 1.4.4 and Node 26.8.1 here differ from the pins).
