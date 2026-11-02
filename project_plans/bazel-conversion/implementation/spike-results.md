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
9. **Disk cache GC**: `--experimental_disk_cache_gc_max_size=100G`, `--experimental_disk_cache_gc_max_age=30d`, `--experimental_disk_cache_gc_idle_delay=10s` (default 5m never fires when the server exits after 30 s idle). `--repository_cache=~/.cache/stapler-squad-bazel/repo` and `--repo_contents_cache=.../repo/contents` are set explicitly beside each other (plan Task 0.2b; the default is already per-user shared, the committed paths are an owner-chosen location).
10. **`--remote_local_fallback` is dropped** (no-op for `--remote_cache` without `--incompatible_remote_local_fallback_for_remote_cache`; the failure policy is timeout + retries + circuit breaker, identical in `ci-cache-read` and `ci-cache-write`, plus `--remote_cache_compression`). Not exercised against a real cache (Epic 7 / Task 0.12).
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
| --batch_cpu_scheduling | spike-verified | `bazel help startup_options --long` 9.3.1: `--[no]batch_cpu_scheduling (a boolean; default: "false")` |
| --enable_platform_specific_config | spike-verified | `bazel help build --long` 9.3.1: `--[no]enable_platform_specific_config (a boolean; default: "false")` |
| --experimental_remote_cache_eviction_retries | spike-verified | `bazel help build --long` 9.3.1: `--experimental_remote_cache_eviction_retries (an integer; default: "5")` (not committed; Task 7.1a) |
| --io_nice_level | spike-verified | `bazel help startup_options --long` 9.3.1: `--io_nice_level (an integer; default: "-1")` |
| --remote_cache_compression | spike-verified | `bazel help build --long` 9.3.1: `--[no]remote_cache_compression (a boolean; default: "false")` |
| --repo_contents_cache | spike-verified | `bazel help build --long` 9.3.1: `--repo_contents_cache (a path; default: see description)` |
| --repo_contents_cache_gc_max_age | spike-verified | `bazel help build --long` 9.3.1: `--repo_contents_cache_gc_max_age (An immutable length of time.; default: "14d")` |
| --repository_cache | spike-verified | `bazel help build --long` 9.3.1: `--repository_cache (a path; default: see description)` |
| --sandbox_default_allow_network | spike-verified | `bazel help build --long` 9.3.1: `--[no]sandbox_default_allow_network (a boolean; default: "true")` (spike S1 uses `=false`; not committed) |
| --shutdown_on_low_sys_mem | spike-verified | `bazel help startup_options --long` 9.3.1: `--[no]shutdown_on_low_sys_mem (a boolean; default: "false")` |
| --experimental_run_in_user_cgroup | spike-verified | `bazel help build --long` 9.3.1: `--[no]experimental_run_in_user_cgroup (a boolean; default: "false")` (not committed; evaluated in E7, Task 0.10g) |
| --local_ram_resources | absent | `grep -cE -- '--local_ram_resources' ` of `bazel help build --long` 9.3.1 prints 0 |
| --local_cpu_resources | absent | same grep prints 0 |
| --incompatible_sandbox_hermetic_tmp | absent | same grep prints 0 |
| --experimental_guard_against_concurrent_changes | absent | same grep prints 0 |

## Fresh clone

`git clone` of the branch into a short temp path, then: `bazel build --config=agent //tools/lint/... //tools/scanner/... //tuitest/...` exit 0 (239 actions, 156 served from the shared disk cache, so it is not a fully cold compile); `bazel test //tools/lint/...` 16 pass; `bazel mod tidy` left `git diff --exit-code MODULE.bazel MODULE.bazel.lock` clean; with a planted `.claude/worktrees/agent-x/BUILD.bazel`, `bazel query //...` shows 0 labels under `//.claude`; `grep -c node_modules` of the lint/scanner/tuitest query is 0.

## S1 proto Go half (Story 0.3, Tasks 0.3a, 0.3b, 0.3d, 0.3e, 0.3f)

Host onyx, Bazel 9.3.1, 2026-11-01. Targets: `//proto/session/v1:{sessionv1_go_proto,sessionv1connect_go_proto,gen_go,sessionv1_go,sessionv1connect_go}`, `//tools/bzl/proto:{connect_go,protoc_gen_connect_go}`, `buf.gen.bazel.yaml`, `tools/bzl/parity/compare-proto.sh`.

### Reference (the `make proto-gen` Go half)
`buf generate proto --template buf.gen.go-only.yaml -o <scratch>` (Homebrew buf 1.66.1, remote plugins `buf.build/protocolbuffers/go` unpinned, `buf.build/connectrpc/go:v1.20.0`): 37 files (20 `*.pb.go`, 17 `*.connect.go`), `diff -r` against the main checkout's `gen/proto/go` is empty, so the scratch reference equals what `make proto-gen` produced there. Header `protoc-gen-go v1.36.12` equals the `google.golang.org/protobuf` pin in `go.mod`, but nothing pins the remote plugin: a BSR release would change `make proto-gen` output silently.

### Task 0.3a/0.3b: go_proto_library + connect compiler (result: does not reproduce)
- `go_proto_library` (`@rules_go//proto:go_proto`) builds, with prebuilt protoc `external/protobuf++protoc+prebuilt_protoc.linux_x86_64/bin/protoc`. `compare-proto.sh gopb`: DIFFERENT, 20 of 20 `*.pb.go` differ. Deltas (none in the allow-list): (1) header `protoc (unknown)` vs `protoc v7.36.2`; (2) `package v1` vs `package sessionv1` (sources carry no `go_package`; two files carry a bare one); (3) all leading comments are missing (rules_go feeds descriptor sets without source info); (4) `rawDesc` lacks the options buf managed mode injects (`java_package=com.session.v1`, `java_outer_classname`, `java_multiple_files`, `go_package=...;sessionv1`, objc/csharp/php/ruby options, verified in the reference).
- The connect half does not build: `protoc-gen-connect-go: Go package ".../sessionv1connect" has inconsistent names v1 (session/v1/github_user.proto) and sessionv1connect (session/v1/guidance_request.proto)` (run 1), same failure on the repair run (`-k` rerun through the script). A separate `go_proto_library` for connect also cannot point at the pb package without per-file `M` options. 17 of 17 connect files missing.
- Repair run: none that avoids reimplementing managed mode, which is the plan's fork path (L). Not attempted beyond the rerun.

### Task 0.3d: `buf generate` as a Bazel action (result: identical)
`genrule //proto/session/v1:gen_go`: buf built from `go_deps` (`github.com/bufbuild/buf` v1.57.2 per `go.mod`), local `protoc-gen-go` (v1.36.12) and `protoc-gen-connect-go` (connect v1.20.0, `go.mod` line 17; `bazel mod show_repo @com_connectrpc_connect`), `buf.gen.bazel.yaml` with the same managed mode and `paths=source_relative`. Built with `--sandbox_default_allow_network=false` (flag exists, `[no]sandbox_default_allow_network`, default true). `bash tools/bzl/parity/compare-proto.sh buf` with the scratch reference: `compare-proto(buf): identical (37 files)` after gofmt, empty diff, no allow-list entries. Negative control: `gopb` mode of the same script reports DIFFERENT. Import paths: the generated connect files import `v1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"`; `go_library` wrappers `:sessionv1_go` and `:sessionv1connect_go` compile under `.../gen/proto/go/session/v1` and `.../sessionv1connect`.

### Task 0.3e: buf.yaml BSR dependency
`git grep googleapis -- proto` is empty and `deps: buf.build/googleapis/googleapis` was never fetched (`buf build proto` exits 0 offline with it present). Removed the `deps:` entry; `buf generate` output unchanged (`diff -r` empty), `buf lint proto` output byte-identical before and after (72 lines). False plan assumptions: (a) `buf lint` exits 100 today (pre-existing STANDARD findings, e.g. go_package consistency), not 0; (b) `make proto-gen` cannot run offline at all: `unshare -rn buf generate ...` fails with "the server hosted at that remote is unavailable" because the committed templates use remote BSR plugins. The Bazel path is the offline one.

### Task 0.3f: prebuilt protoc
`toolchains_protoc` is not needed on Bazel 9.3.1 + protobuf 36.2: the module already resolves a prebuilt protoc (`prebuilt_protoc.linux_x86_64`). Cold build (fresh output base `.bazel-out`, `--disk_cache=`, `--config=ci`, 837 actions, 62.8 s on a loaded host): `grep -cE 'Compiling .*upb|protobuf/src'` of the log prints 0, no `CppCompile` mnemonic in `aquery` of the proto targets, and the buf path does not use protoc at all. This is a local cold output base, not a GitHub runner. Two more false assumptions: `--nodisk_cache` is rejected on 9.3.1 (`Illegal use of 'no' prefix on non-boolean option`; use `--disk_cache=`), and `proto_library` must load from `@protobuf//bazel:proto_library.bzl` (`@rules_proto` is not a dependency). A `.bazel-out` output base contains read-only files; deleting it needs `chmod -R u+w` first (relevant to Task 11.4).

### Verdict
`S1`: **FallbackWorkaround** (path: `buf generate` as a genrule with local pinned plugins; cost M). The primary `go_proto_library` path fails the diff check; every PASS check holds for the workaround. Epic 2 inherits: `buf.gen.bazel.yaml`, `gen_go`, `compare-proto.sh`. The TypeScript half stays S1-ts (Task 0.6e).
