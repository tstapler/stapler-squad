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
| --macos_minimum_os | spike-verified | `bazel help build` 9.3.1: `--macos_minimum_os (a dotted version (for example '2.3' or '3.3alpha2.4'); default: see description)` (flag exists; its effect on macOS is not verified, no Mac) |
| --max_idle_secs | spike-verified | `bazel help` 9.3.1: `--max_idle_secs (an integer; default: "10800")` |
| --platforms | spike-verified | `bazel help build` 9.3.1: `--platforms (a build target label; default: "")` |
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
| --sandbox_default_allow_network | spike-verified | `bazel help build --long` 9.3.1: `--[no]sandbox_default_allow_network (a boolean; default: "true")` (committed as `test --sandbox_default_allow_network=false`, Task 4.1b; loopback still works, S8 census ran with it) |
| --test_env | spike-verified | `bazel help build --long` 9.3.1: `--test_env (a 'name[=value]' assignment with an optional value part or the special syntax '=name' to unset a variable; may be used multiple times)` (committed only for `STAPLER_SQUAD_INSTANCE=bazel-test`, Task 4.1b) |
| --cache_test_results | spike-verified | `bazel help test --long` 9.3.1: `--[no]cache_test_results [-t] (a tri-state (auto, yes, no); default: "auto")` (committed for `test:e2e`, Task 12.3a) |
| --test_tag_filters | spike-verified | `bazel help test --long` 9.3.1: `--test_tag_filters (comma-separated list of options; default: "")` (committed for `test:e2e`, Task 12.3a) |
| --run_under | spike-verified | `bazel help build --long` 9.3.1: `--run_under (a prefix in front of command; default: see description)`; applies to `bazel test` (Task 4.1b: `//tools/bzl:test_wrapper`; `env_test` passes with it and fails without it, with `--run_under=/usr/bin/env`) |
| --shutdown_on_low_sys_mem | spike-verified | `bazel help startup_options --long` 9.3.1: `--[no]shutdown_on_low_sys_mem (a boolean; default: "false")` |
| --experimental_run_in_user_cgroup | spike-verified | `bazel help build --long` 9.3.1: `--[no]experimental_run_in_user_cgroup (a boolean; default: "false")` (not committed; evaluated in E7, Task 0.10g) |
| --compilation_mode | spike-verified | `bazel help build --long` 9.3.1: `--compilation_mode [-c] (fastbuild, dbg or opt; default: "fastbuild")` (committed only as `build:release --compilation_mode=opt`, Task 3.4b) |
| --workspace_status_command | spike-verified | `bazel help build --long` 9.3.1: `--workspace_status_command (a path; default: "")` (committed only as `build:release --workspace_status_command=tools/bzl/workspace_status.sh`, Task 3.4a/b) |
| --stamp | spike-verified | `bazel help build --long` 9.3.1: `--[no]stamp (a boolean; default: "false")` (committed only as `build:release --stamp`, Task 3.4b) |
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

## S2 ent generation as a Bazel action (Stories 0.4 / Tasks 0.4a-0.4c)

Host onyx, Bazel 9.3.1 via `tools/bazel`, Go 1.26.6 (rules_go SDK from go.mod), `entgo.io/ent` v0.14.5, branch `sdd/bazel-epic0-s2-ent`. Code: `tools/bzl/ent.bzl`, `tools/bzl/ent/{BUILD.bazel,ent_flags.txt,packages.bzl}`, `tools/bzl/ent/entc/{main.go,schemas.go,BUILD.bazel}`, `session/ent/schema/BUILD.bazel` (Gazelle-generated plus a `schema_srcs` filegroup), root `BUILD.bazel` directives, `Makefile` `ent-gen` reads `ent_flags.txt`.

**Verdict: `FallbackWorkaround`, path = declared-output rule, cost M.** Not `Pass` because (a) the stock route needed a workaround and (b) the importer check below was run on a subset (decision for the owner: re-run it after S1 and Story 0.11 and downgrade to `Fail` if it breaks).

### Mechanism (what was built, in the order tried)

1. Reference: copied `go.mod`, `go.sum` and the non-test schema files to `/tmp/s2ref` and ran the required `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`. `go list ./session/ent/...` there = 53 packages: 52 generated + the hand-written `schema`. 333 generated `.go` files (379 incl. 46 schema files).
2. `entc` (a `go_binary`) links `//session/ent/schema` as an ordinary dependency, calls `load.MarshalSchema` per schema (46, list in `schemas.go`, drift-checked against the sources by AST) and runs `entc/gen` with the flags from `ent_flags.txt`. Output is byte-identical to the reference: `diff -r bazel-bin/tools/bzl/ent /tmp/s2ref/session/ent -x schema` empty.
3. `ent_gen` is a custom rule with `attr.output_list` (333 declared outputs from `packages.bzl`), not a tree artifact: rules_go needs file srcs, so no `GoSource` rule or fork was necessary. `ent_packages` instantiates one `go_library` per directory with explicit `importpath` and deps taken from the listing.
4. The generator verifies the generated tree against `packages.bzl` (`-expect`) and fails with `Run: bazel run //tools/bzl/ent:update_packages` (tested: a doctored listing produces exactly that message; `update_packages` is idempotent, `git status` clean after it).
5. goimports (run by ent's formatter) shells out to `go env`. Failure without it: `format file .../claudemetadata_delete.go: err: go command required, not found: exec: "go": executable file not found in $PATH`. The action therefore gets the rules_go SDK (`go.sdk.go`, libs, srcs, tools) as inputs plus `PATH`/`GOROOT`; `entc` makes them absolute (`cannot run executable found relative to current directory` otherwise) and sets a temp `GOCACHE`. No module cache, no network, no `-mod=mod` needed.

### Checks (PASS iff list)

| Check | Result | Evidence |
|---|---|---|
| Every package from `go list ./session/ent/...` is a `go_library` with its own importpath | true (52 generated + schema = 53) | `bazel query 'kind(go_library, //tools/bzl/ent:all)' \| wc -l` = 52 = `go list ./session/ent/... \| grep -vc /schema`; `//session/ent/schema:schema` is the 53rd |
| Built from a clean clone, action network disabled | true for actions; repository fetch used the local repository cache, so "no network at all" is not proven | `git clone` to `/tmp/s2c` (only `session/ent/schema` present), `bazel --output_base=/tmp/s2obC build --config=agent --disk_cache= --sandbox_default_allow_network=false -- //session/ent/... //tools/bzl/ent/... //session/scanstate:scanstate -//session/ent/schema:schema_test`: exit 0, 264 actions |
| `sql/upsert` effective | true | 46 `func (...) OnConflict` builders in `bazel-bin/tools/bzl/ent`; same generator with an empty flags file: 0 |
| Two clean builds hash-equal | true | output bases `/tmp/s2obA`, `/tmp/s2obB`, `--disk_cache=` empty (action really ran, 111 actions each), 333 files, `sha256sum` listings identical (`db075ee09c8c5197...`) |
| Importers resolve | **subset only** | see below |

Importer check, exactly what was run: Gazelle on `session/scanbuf session/scanstate pkg/events server/analytics/escapedb session/contexthistory server server/analytics server/handlers server/services server/workflows session tools/lint/norawdb` (the 12 directories that import `session/ent*`; BUILD files not committed). The 84 generated BUILD files reference 49 distinct `//tools/bzl/ent...` labels; `bazel query 'set(<49 labels>)'` returns 49 (all exist). The 50th distinct import path in source is my own `entc/schemas.go`. `bazel build //session/scanstate:scanstate` (importer of `ent` plus `transcriptscanstate`, with the synced ent tree present on disk) builds, also in the clean clone. NOT built: `//session/... //server/...`: every other importer's closure contains `gen/proto/go/session/v1` (S1) and needs the whole-repo Gazelle run (Story 0.11); only `scanstate` and `tools/lint/norawdb` are proto-free (`go list -e -deps` counts of `gen/` imports: scanstate 0, norawdb 0, the other ten 1-2).

### Task 0.4c / Gazelle (S11 interplay)

- Resolve lines needed: **2** (`gazelle:resolve go .../session/ent //tools/bzl/ent:ent` and one `resolve_regexp go ^.../session/ent/(.+)$ //tools/bzl/ent:$1`), no per-entity line, so below the S11 threshold (more than 8 lines or any per-entity line); `$1` substitution confirmed live. `schema` is an alias in `//tools/bzl/ent` so the one regexp covers it.
- The provisional exclusion `session/ent/[!s]*` is wrong when the synced tree exists: with generated files present Gazelle created BUILD files in `session/ent/` and seven `s*` entity directories (`session`, `sessiongoal`, `sessionsummary`, `shell`, `sourcesyncevent`, `stagelivenessdefinition`, `stagetransition`). Adding `# gazelle:exclude session/ent/s[!c]*` fixed it (re-run: only `session/ent/schema/BUILD.bazel` exists). Brittle by construction (schema is the only `sc` name); Task 0.11b should keep or replace it.

### Cost and fallback

Workaround measured as M (`ent.bzl` 83 lines, over the 40-line cap Task 2.3 names for the productionised file; `entc` 334 lines of Go in two files, plus a 52-entry generated listing and a 46-entry schema list). Fork (rules_go directory `srcs`) not needed, not costed further. Both lists need `bazel run //tools/bzl/ent:update_packages` / `schemas.go` edit on a schema change; the first is enforced by the action, the second by an AST check in the same action.

### False or unsafe plan assumptions

1. "`UpsertRule`-style symbols": no generated symbol is named `UpsertRule`; the real evidence of `sql/upsert` is `OnConflict*` builders (46 vs 0 without the flag).
2. "Checked-in `packages.txt` read by the macro": Starlark cannot read files at load time; the listing is `packages.bzl` (generated).
3. "53 `go_library`" and "`bazel build //session/ent/...`": 53 includes the hand-written schema; the 52 generated libraries live in `//tools/bzl/ent`, so the `//session/ent/...` pattern does not build them, and it also matches `//session/ent/schema:schema_test`, which fails analysis until `//session` has a BUILD file.
4. "ent needs only module data": the formatter needs a `go` binary on PATH (rules_go SDK works; no module cache or network).
5. Task 0.4a's "schema as a normal dependency" works, but adds an unplanned third list (schema type names, `schemas.go`) because `entc/load` discovers them by AST and `go run`, which this approach avoids.
6. `session/ent/generate.go` (CLAUDE.md): absent, as the plan already noted.

## S5 - tmux built hermetically by Bazel (Story 0.7)

**Verdict: `Pass`, path = rules_foreign_cc 0.17.0 `configure_make` (Task 0.7a), cost M.** Task 0.7b (host-cc genrule) was not needed and not executed, so there is no hermeticity downgrade and no row for `hermetic-downgrades.txt`.

### Mechanism

`MODULE.bazel` adds `rules_foreign_cc` 0.17.0, BCR `libevent` 2.1.12-stable.bcr.0 and `ncurses` 6.4.20221231.bcr.15, and an `http_archive` `tmux_src` (release tarball `tmux-3.8.tar.gz`, which ships `configure`; `third_party/tmux` is never an input, and is empty in this worktree). `tools/bzl/tmux/BUILD.bazel` has `configure_make(tmux_build)` (`--disable-utf8proc`, `LIBEVENT_CORE_*`/`LIBTINFOW_*` env vars bypass pkg-config) and a `tmux_bin` genrule that copies the binary to `bazel-bin/tools/bzl/tmux/tmux`.

### Checks (PASS iff list)

| Check | Result | Evidence |
|---|---|---|
| `//tools/bzl/tmux:tmux_bin` builds from the `http_archive` tarball with `third_party/tmux` empty | true | `bazel build //tools/bzl/tmux:tmux_bin --config=agent` exit 0; `ls third_party/tmux \| wc -l` = 0; first build 80 s, 5294 actions (cold libevent, ncurses, m4, make, pkgconf toolchains) |
| `tmux -V` prints exactly `tmux 3.8` | true | `bazel-bin/tools/bzl/tmux/tmux -V` -> `tmux 3.8` |
| `git ls-files third_party/tmux` shows only the gitlink | true | prints `third_party/tmux` (mode 160000, `git ls-files -s`) |
| `git check-ignore tools/bzl/tmux/BUILD.bazel` prints nothing | true | no output, exit 1 |
| Action ran sandboxed | true | second build in a fresh output base with `--disk_cache= --spawn_strategy=linux-sandbox --sandbox_default_allow_network=false`: exit 0, "5294 processes: 3448 internal, 1846 linux-sandbox" (0 local). Repository fetches used the repository cache, so "no network at all" is not proven; actions had none |

Extra: the binary is reproducible across two output bases (sha256 `d8fbb95a691e82d114bd66dc062b2b3523c6ac01afd50ba97823c4def329fb62` both). Functional test on a private socket (`tmux -L s5spike<pid> -f /dev/null`): `new-session -d`, `list-sessions`, `capture-pane` returned `hi`; `kill-server -L` only. Live tmux untouched.

### Repair run (one allowed)

First build exited 0 but the binary did not run: `error while loading shared libraries: /usr/lib/libncurses.so: file too short`. Cause: the foreign_cc dep dir holds both `.a` and `.so`, `-lncurses`/`-levent_core` picked `.so`, leaving `NEEDED libncurses.so` resolved against the host (Arch's `/usr/lib/libncurses.so` is a linker script). Fix: `-Wl,-Bstatic -l... -Wl,-Bdynamic` in the env vars. After: `readelf -d` NEEDED = libm, libresolv, libc only. A build that exits 0 is not evidence the binary is hermetic, so the Story 3.3 target needs this `readelf` check.

### Comparison with the Make-built binary (embed seam)

| | Make (`bin/tmux`, main checkout) | Bazel (`tmux_bin`) |
|---|---|---|
| size | 1608424 B | 2179824 B |
| `ldd` | libncursesw.so.6, libevent_core-2.1.so.7 (host), libm, libresolv, libc | libm, libresolv, libc only (libevent and ncurses static) |
| `-V` | `tmux 3.8` | `tmux 3.8` |

Embed seam: `session/tmux/binary_embedded.go` embeds `embed/tmux` (path `session/tmux/embed/tmux`, gitignored by `session/tmux/embed/.gitignore`). A staging step `cp bazel-bin/tools/bzl/tmux/tmux session/tmux/embed/tmux` needs no Go change. I did not stage it or compile `-tags embed_tmux` (no tracked or embed file touched), so "embed build works" is UNVERIFIED; the file contents are the only difference. Note the Bazel binary is more portable than the Make one (no host libevent/ncurses dependency).

### Pin table (Task 0.7c)

| Source | Value |
|---|---|
| `tools/bzl/tmux/PIN` tag | `3.8` |
| tarball sha256 (computed from download, equals `MODULE.bazel` `tmux_src`) | `e79c699c7e949dccd0a4a125e17b8d1261e311b16979dbc8eb34542f3966d82e` |
| `git ls-remote https://github.com/tmux/tmux.git '3.8*'` | `refs/tags/3.8` = `72c20dc2...` (tag object); `3.8^{}` = `7f2a35ad3321f9ba57a1062ca73b1f3ff26aca53` |
| submodule gitlink (`git ls-files -s third_party/tmux`) | `7f2a35ad3321f9ba57a1062ca73b1f3ff26aca53` (equals PIN `commit`; consistent) |
| `awk` non-empty `tag`, `sha256`, `commit` in `PIN` | true |
| `bash tools/ci/check-tymux-pin.sh` | exit 0: "daemon v1.12.0 and client v0.1.1-0.20261031092300-04a5e941d697 are commit 04a5e941d697". This script checks the tymux daemon/client pin, NOT the tmux submodule vs `PIN`; the tmux pin comparison (gitlink == PIN commit, `MODULE.bazel` sha == PIN sha) is not implemented yet (the plan's "new pin test", Task 3.3b); I compared by hand above |

tymuxd: `scripts/fetch-tymuxd.sh` + `scripts/tymuxd-checksums.txt` (v1.12.0 = Makefile `TYMUX_VERSION`) mapped to four `http_archive` repos in `MODULE.bazel` (`tymuxd_<target>`, each exports `tymuxd`). The release asset is a tarball containing `tymuxd` and `tymux`, so `http_archive` (extracting) is used instead of the plan's `http_file`. The linux-x86_64 sha256 recomputed from the download equals the checksums file; all four fetch and verify under Bazel (`--nobuild` for the other three). tymuxd was not executed.

### Notes and risks

- Compiler is the host `gcc` 15.2.1 via the auto-configured `rules_cc` toolchain, as for every other cc target; the `configure_make` toolchains (m4, make, pkgconf, ninja, meson) come from BCR. Host-cc drift is covered by the cc toolchain's own tracking, not by a tmux-specific mechanism; no D22 `no-cache` tag was added because no host-cc genrule exists.
- BCR `ncurses` pulls a `local_terminfo` module extension that reads the host terminfo into a data target; the tmux link used `libncurses.a` only and ran with host terminfo, but this host read is a hermeticity leak to review in Story 3.3.
- BCR `libevent` depends on `openssl` and `zlib` modules (only module-graph cost; not built for `event_core`).
- `MODULE.bazel.lock` changed (rules_foreign_cc and its deps).

### False or unsafe plan assumptions

1. "`bazel-bin/tools/bzl/tmux/tmux -V`" from `configure_make(out_binaries)`: `configure_make` output lands in `tmux_build/bin/tmux` inside a directory output group; a wrapper genrule (`tmux_bin`) is needed to publish `bazel-bin/tools/bzl/tmux/tmux`.
2. "static options per `docs/how-to/bundle-tmux.md`": that doc has no static options; the Make build is dynamically linked against host libevent/ncurses. Static linking had to be forced explicitly (see repair run).
3. "tymuxd `http_file`": the asset is a tarball, so `http_archive` is needed.
4. "`tools/ci/check-tymux-pin.sh` ... pass iff the gitlink commit equals the `PIN` commit": the existing script does not check the tmux submodule at all.
5. `tools/bazel` cannot be invoked directly (needs `BAZEL_REAL`); it runs through `bazel`/`bazelisk`.

## S7 Go build tags as Bazel configs (Story 0.8 / Tasks 0.8a, 0.8b)

**Verdict: `Pass`, path = direct, cost S.** Every check of the S7 row is true on Bazel 9.3.1 / rules_go 0.65.0, but two plan assumptions are false and Task 1.4a must use the corrected form (see the last section).

Spike files: `tools/bzl/flags/BUILD.bazel` (the tracked HandBuildFile: two `bool_flag`s and their `config_setting`s), `tools/bzl/spike-tags/` (throwaway package: one `t_<tag>.go` per tag, `go_library` + `go_binary tagdump` that prints the tags whose files were compiled, `spike.bazelrc`). Fake payloads (`embed/tmux`, `embed/tymuxd`) are `testonly` `write_file` targets, so nothing tracked under `session/` changed. Configs were passed with `bazel --bazelrc=tools/bzl/spike-tags/spike.bazelrc ...`; `.bazelrc` itself is untouched. Every Bazel call ran through `tools/bazel`, one at a time.

### Exact syntax that worked (rules_go 0.65.0)

The repository name is `@rules_go`, NOT `@io_bazel_rules_go` (`MODULE.bazel` has no `repo_name`). With the old name every command fails: `ERROR: @io_bazel_rules_go//go/config:pure :: Error loading option ...: No repository visible as '@io_bazel_rules_go' from main repository`.

```
build:linux       --@rules_go//go/config:pure                      # auto-applies via enable_platform_specific_config
build:race        --@rules_go//go/config:pure=false
build:race        --@rules_go//go/config:race
build:embed_tmux  --@rules_go//go/config:tags=embed_tmux
build:embed_tmux  --//tools/bzl/flags:embed_tmux=true
build:embed_tymux --@rules_go//go/config:tags=embed_tmux,embed_tymux   # ONE flag listing both
build:embed_tymux --//tools/bzl/flags:embed_tmux=true
build:embed_tymux --//tools/bzl/flags:embed_tymux=true
build:sinkguard   --@rules_go//go/config:tags=sinkguard            # likewise gitoracle, integration
build:release     --config=embed_tymux                             # release has no other definition in the plan; assumed = embed_tymux
```

`go/config:tags`, `race`, `pure` all accepted. `static` and `msan` were not exercised (not used by this repo).

### Checks (PASS iff list)

| Check | Result | Evidence |
|---|---|---|
| All 7 `TagConfig` build (spike package) | true | `bazel run --config=<c> //tools/bzl/spike-tags:tagdump` exit 0 for none/embed_tmux/embed_tymux/sinkguard/gitoracle/integration/race/release; output: `!race`, `!race embed_tmux:FAKE-TMUX`, `!race embed_tmux:FAKE-TMUX embed_tymux:FAKE-TYMUXD`, `!race sinkguard`, `!race gitoracle`, `!race integration`, `race`, release = embed_tymux output |
| All 7 `TagConfig` build (real targets, Story 1.4 loop) | true | `bazel build --config=<c> //tools/lint/...` for embed_tmux, embed_tymux, sinkguard, gitoracle, integration, race, release: all `Build completed successfully` (54 targets). Weak evidence for tags (tools/lint carries none of them), real evidence the configs parse and compose with the tree |
| `embed_tymux` implies `embed_tmux` | true by construction, enforced by a compile guard | `cquery ... --config=embed_tymux` on `GoInfo.mode`: `tags = ["embed_tmux", "embed_tymux"]`, `pure = True`, `race = False`. Bazel has no implication mechanism: `tags=embed_tymux` alone fails to compile `t_tymux_requires_tmux.go:6:9: undefined: embedTymuxRequiresEmbedTmux` (my guard file, `//go:build embed_tymux && !embed_tmux`) |
| `select()` on `//tools/bzl/flags:embed_tmux` wires the embed file | true | flag true + tag: `embed_tmux:FAKE-TMUX` compiled in; `cquery` `embedsrcs` = `[embed/tmux, embed/tymuxd]` under `embed_tymux`. Flag true, tag off: builds, no embed (`!race`). Tag on, flag false: `t_embed_tmux.go:7:12: could not embed embed/tmux: no matching files found` (the failure ADR-007 predicts; it comes from the compile action, not at analysis) |
| `--config=linux --config=race` builds | true | `bazel build --config=linux --config=race //tools/lint/...` exit 0, 166 actions. `bazel aquery --config=race 'mnemonic("GoCompilePkg", //tools/lint/...)' \| grep -c -- '-race'` = 52 (>= 1). `tagdump` under `--config=race` prints `race` (the `!race` file is excluded) |
| Race without `pure=false` fails (the INFERRED claim) | VERIFIED | `--@rules_go//go/config:race` with `build:linux pure` set: `Error in fail: race instrumentation can't be enabled when cgo is disabled. Check that pure is not set to "off" and a C/C++ toolchain is configured.` |
| Linux default has no C compile | true | `bazel aquery --config=linux 'mnemonic("CppCompile", //tools/lint/...)'` = 0 actions |

### Gazelle `# gazelle:build_tags` (Task 0.8a)

- Spike package with 11 tagged/untagged files, no directive: `srcs = [t_norace.go, t_race.go, tags.go]`, so the 8 other tag files are silently dropped. With `# gazelle:build_tags integration,gitoracle,sinkguard,harness,live_github,embed_tmux,embed_tymux`: all 11 files in `srcs`. `race`/`!race` files are always kept (`race` is treated as known).
- Real package `testutil/gitoracle` (`//go:build gitoracle` test): `gazelle update -mode=print testutil/gitoracle` emits only `go_library`; with `-build_tags=gitoracle` it also emits `go_test srcs = ["oracle_test.go"]`. So the directive is required, not optional, for the 15 gitoracle / 12 integration / 7 sinkguard / `harness` / `live_github` files.
- `//go:embed embed/tmux` with no file present: Gazelle warns `pattern embed/tmux: matched no files` and emits no `embedsrcs`, so the embed `select()` is hand-maintained (confirms ADR-007). In the real tree `session/tmux/embed` and `session/tymux/embed` are empty directories.

### Cost and fallback

Pass path costs S (3 rc lines per config plus the 4-target flags package). The fallback in the matrix (transition macro for tag lists, cost S) is not needed for the sealed single-tag set. It would be needed only for combinations of tag configs in one invocation (see below). Not built.

### False or unsafe plan assumptions

1. **Label spelling**: every `@io_bazel_rules_go//go/config:*` in the plan (Task 1.1c, Story 0.8, `research/architecture.md:149-150`, `stack.md`) is wrong for this repo; use `@rules_go//go/config:*`. Task 1.1c's verify would fail as written.
2. **"additive behaviour of `--config` chains" (Task 0.8a) and `architecture.md:150` (`build:embed_tymux --config=embed_tmux` then a second `tags=` line)**: the `tags` flag is last-wins. Measured: `--config=embed_tmux --config=sinkguard --config=gitoracle --config=integration` compiles only `integration`; chain-style `embed_tymux` (config=embed_tmux + `tags=embed_tymux`) compiles with `-tags embed_tymux` only and fails on the guard. Consequences: (a) `embed_tymux` must list `embed_tmux,embed_tymux` in one flag (Story 1.4's "lists both tags" is right, `architecture.md:150` is wrong); (b) a second tag config combined with an embed or tag config silently drops the first; the sealed set therefore cannot be composed pairwise. `make` today uses only single-tag runs plus `embed_tmux embed_tymux`, so no current recipe is lost; any future combination needs its own named config or the transition macro.
3. **Story 0.8 AC "`bazel cquery --output=starlark` inspects `gotags`"**: there is no `gotags` provider field; the value is `GoInfo.mode.tags` (provider key `@@rules_go+//go/private:providers.bzl%GoInfo`).
4. **"Tag alone is silent"** (`architecture.md:163`): wrong direction of detail, not of conclusion. Tag-on with the flag off fails in the compile action with `could not embed embed/tmux: no matching files found`, not at analysis; the flag-on/tag-off combination builds without embedding. Both knobs must be set by the same config.
5. **`build:linux --...:pure` is only auto-applied** because `.bazelrc` has `--enable_platform_specific_config`; the plan relies on it without saying so (fine, but a macOS dev must not expect it).

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

## S1-ts proto TypeScript generation (Story 0.6, Task 0.6e)

Run 2026-11-01. `//tools/bzl/proto:gen_ts` (rule `buf_gen_ts` in `tools/bzl/proto/ts.bzl`, template `buf.gen.bazel-ts.yaml`) runs the Bazel-built buf v1.57.2 with `protoc-gen-es` 2.11.0 from `@web_node_modules//:node_modules/.bin` and the pinned Node 24.13.1 on PATH, and publishes a tree artifact equal to `web-app/src/gen`. It is `buf generate`, not `run_binary` of `protoc` as the task text suggested, because managed mode (the `go_package_prefix` override) changes the descriptors embedded in `*_pb.ts`, so the template keeps the same `managed:` block as `buf.gen.yaml`. First build worked without repair. `make proto-gen` applies no formatter to the TS half (no gofmt equivalent), so none is applied here.

Checks (all VERIFIED by command):

1. Parity: `PROTOC_GEN_ES=<main checkout web-app/node_modules/.bin/protoc-gen-es> bash tools/bzl/parity/compare-proto.sh ts` prints `compare-proto(ts): identical (20 files, sha256)`, exit 0. The reference is generated in a scratch dir by plain `buf generate` with the same TS plugin config but a plugin from the make-side install (shared store) under host Node 26.8.1, so the Bazel path (Node 24.13.1) and the make path (Node 26) agree. Also `diff -r bazel-bin/tools/bzl/proto/gen_ts/session <main checkout>/web-app/src/gen/session` is empty. `allowed-deltas.txt` needed 0 entries; the script's allow-list branch exists but was not exercised.
2. No network: built with `--sandbox_default_allow_network=false`; buf cache and HOME are scratch dirs; `buf.yaml` has no deps.
3. No source-tree writes: `git status` after all builds shows only intended edits; `web-app/src/gen` never appeared.
4. Determinism: two `bazel clean` + `--disk_cache=` builds (51.4 s and 42.5 s, includes re-running the Bun install repository rule) give the same tree hash `0de074ff...4712a`.
5. `//web-app:dist` consumes it: `bun_dist` got a `gen` attribute copied (`cp -RL`; a plain `cp -R` copies the sandbox symlinks and Bun then resolves modules from bazel-out) into `$W/src/gen`, `dist_srcs` excludes `src/gen/**`, and `dist`, `dist_nosandbox`, `dist_trace` and `jest_subset` take `gen = //tools/bzl/proto:gen_ts`. Baseline with the on-disk `src/gen` copied in: tree hash `b722078d930c...aff7`. With the generated target and no `src/gen` in the worktree: same hash `b722078d930c...aff7`, 380 files. `bazel test //web-app:jest_subset` passes (it needs `gen` too: without it the 20 suites fail because they import `src/gen`).

Findings: (a) the plan's `diff -r bazel-bin/web-app/gen web-app/src/gen` path is wrong for this design; the output is `bazel-bin/tools/bzl/proto/gen_ts/` rooted at the gen dir. (b) `protoc-gen-es` runs straight from the Bun install: it is a `#!/usr/bin/env node` script, so the action only needs the pinned Node on PATH. (c) Pinned-tarball fallback of ADR-003 was not needed and not built (cost M estimated, unchanged).

### Verdict

`S1-ts`: **Pass**, path `buf-generate+protoc-gen-es-from-bun-install`, cost S. All five checks true; no explained deltas.


## S13: commit or ignore `MODULE.bazel.lock` (Tasks 0.10b and 0.10d)

### Driver text (Task 0.10b, fetched 2026-11-01)

Source: https://bazel.build/external/lockfile#automatic-resolution (rendered page drops code blocks; the exact text is from the page's Markdown source, https://raw.githubusercontent.com/bazelbuild/bazel/9.0.0/site/en/external/lockfile.md, lines 254-274). Verbatim:

```gitattributes
# A custom merge driver for the Bazel lockfile.
# https://bazel.build/external/lockfile#automatic-resolution
MODULE.bazel.lock merge=bazel-lockfile-merge
```

```bash
jq_script=$(curl https://raw.githubusercontent.com/bazelbuild/bazel/master/scripts/bazel-lockfile-merge.jq)
git config --global merge.bazel-lockfile-merge.name   "Merge driver for the Bazel lockfile (MODULE.bazel.lock)"
git config --global merge.bazel-lockfile-merge.driver "jq -s '${jq_script}' -- %O %A %B > %A.jq_tmp && mv %A.jq_tmp %A"
```

The driver name is `bazel-lockfile-merge` (not the plan's provisional `bazel-lock`). jq 1.8.2 is installed here (needs 1.5+). The jq program is vendored at `tools/bzl/bazel-lockfile-merge.jq` from tag 9.3.1 (identical to master on 2026-11-01); `scripts/git-merge-bazel-lock.sh` runs it with the docs' argument shape (`jq -s -f script -- %O %A %B`); `.gitattributes` has the line; `make setup-git` registers the repo-local `git config` (per clone, shared by worktrees of that clone, not by a fresh clone).

### Replay (Task 0.10d), scratch clone of this tree, Bazel 9.3.1, 20 pairs from a pool of 12 BCR modules, each pair adds two different `bazel_dep` lines at separated positions of `MODULE.bazel`

Harness: `run.sh`/`repair.sh` (not kept in the repo). Baseline `bazel mod deps --lockfile_mode=error` on the unmodified lock: exit 0, 3.1 s.

| # | pair | ARM1 no driver: text conflict | ARM1 driver: conflict / error-mode rc | ARM1 driver + `mod deps` regen: error-mode rc | ARM2 (no lock): conflict / error-mode rc |
|---|---|---|---|---|---|
| 1 | rules_pkg+rules_license | no | no / 0 | 0 | no / 37 |
| 2 | rules_pkg+abseil-cpp | yes | no / 2 | 0 | no / 37 |
| 3 | rules_pkg+jq.bzl | yes | no / 2 | 0 | no / 37 |
| 4 | rules_pkg+rules_perl | yes | no / 2 | 0 | no / 37 |
| 5 | rules_license+googletest | no | no / 0 | 0 | no / 37 |
| 6 | rules_license+bazel_features | yes | no / 2 | 0 | no / 37 |
| 7 | rules_license+rules_python | yes | no / 2 | 0 | no / 37 |
| 8 | rules_proto+googletest | no | no / 0 | 0 | no / 37 |
| 9 | rules_proto+bazel_features | yes | no / 2 | 0 | no / 37 |
| 10 | rules_proto+rules_python | yes | no / 2 | 0 | no / 37 |
| 11 | googletest+abseil-cpp | yes | no / 2 | 0 | no / 37 |
| 12 | googletest+jq.bzl | yes | no / 2 | 0 | no / 37 |
| 13 | googletest+rules_perl | yes | no / 2 | 0 | no / 37 |
| 14 | abseil-cpp+bazel_features | yes | no / 2 | 0 | no / 37 |
| 15 | abseil-cpp+rules_python | yes | no / 2 | 0 | no / 37 |
| 16 | zlib+bazel_features | yes | no / 0 | 0 | no / 37 |
| 17 | zlib+rules_python | yes | no / 0 | 0 | no / 37 |
| 18 | bazel_features+jq.bzl | yes | no / 0 | 0 | no / 37 |
| 19 | bazel_features+rules_perl | yes | no / 2 | 0 | no / 37 |
| 20 | jq.bzl+rules_python | yes | no / 0 | 0 | no / 37 |

Totals over 20 pairs:

| Arm | Textual conflicts | `--lockfile_mode=error` failures on merged tree |
|---|---|---|
| ARM1 control, lock committed, no driver | **17 of 20** (all in `MODULE.bazel.lock`; the 3 clean pairs had no lock change on one side) | not run |
| ARM1, lock committed + documented driver | 0 | **13 of 20** (exit 2: "MODULE.bazel.lock is no longer up-to-date ... usages of the extension ... have changed", e.g. `pybind11_bazel`) |
| ARM1 + driver + one `bazel mod deps` regen (repair run; lock rewritten in 20 of 20) | 0 | 0 (regen 3-4 s each on a warm server) |
| ARM2, lock not committed | 0 | 20 of 20 exit 37 (by design: `error` mode needs a lock); 0 failures if CI uses `update` (20 of 20 exit 0, 3.2-4.4 s, the cost of re-resolving each run) or `off` (not run) |

Finding: the documented driver removes the textual conflict but does NOT by itself produce a lock that passes `--lockfile_mode=error` in 13 of 20 merges (it merges per-extension entries but not the `usagesDigest` that depends on the merged module graph). The committed-lock option is only conflict-free AND error-clean with a regenerate step after the merge (the plan's custom `bazel mod deps` fallback, Task 7.8 autofix), so the zero-failure check holds for "driver + regen", not the driver alone. Caveat: the pool deps share one `MODULE.bazel` and BCR-resolved versions; real PRs also touch `go.mod`-driven extension state. Scratch repos were removed after the run.

### GitHub-side check (run 2026-11-04, private scratch repo `tstapler/bazel-lockfile-merge-experiment`)

Scratch repo (owner-authorised, private, not `tstapler/stapler-squad`): a minimal bzlmod module, Bazel 9.3.1 via bazelisk, `MODULE.bazel.lock` committed, `.gitattributes` with `MODULE.bazel.lock merge=bazel-lockfile-merge` and the vendored jq program in the tree. Two bases: a small one (`platforms`, `bazel_skylib`; 50 KB lock) for trials 1-12, and a richer one (plus `rules_go`, `gazelle`, `rules_cc`, `rules_shell`, `bazel_lib`; 62 KB lock) for trials 13-32. (`protobuf` was dropped from the base: it pulls `rules_rust`, whose `crate` extension fails non-hermetically here; one `main` push with it failed `verify`, expected, and was fixed.) Each trial: branch `tN` from `main`, PR A and PR B (`pr-N-a`, `pr-N-b`) each add one different `bazel_dep` at separated places of `MODULE.bazel` with a lock regenerated by `bazel mod deps --lockfile_mode=update`; merge A through the API; read B's mergeability; try B through the API and `update-branch`; let the sync workflow act; merge B; read the `verify` run (`bazelisk mod deps --lockfile_mode=error`) on the final `tN`.

**Q1, does GitHub's server-side merge honour the `merge=` driver? No (VERIFIED).** The attribute was in the tree for every trial; the driver exists only in local git config. For the 8 pairs whose locks conflict (below), GitHub reported B as `mergeable=CONFLICTING`, `mergeStateStatus=DIRTY`; `PUT /pulls/B/merge` returned `405 {"message":"Pull Request has merge conflicts"}`; `PUT /pulls/B/update-branch` returned `422 {"message":"merge conflict between base and head"}` (in the other cases `422 "expected head sha didn't match current head ref."` once the sync workflow had already pushed, and `422 "There are no new commits on the base branch."` after the push, both of which are race artefacts, not a different verdict). The same 8 pairs locally: `git merge-tree --write-tree pr-N-a <B commit>` exits 1 (conflict) for 8 of 8 without a driver, and a real `git merge` with the driver registered exits 0 for 8 of 8. So a driver-resolvable conflict stays a conflict on GitHub's merge button, merge API and `update-branch`.

**Conflict rate.** 8 of 32 pairs conflicted in `MODULE.bazel.lock` (pairs 8, 20, 27-32; pairs 27-32 were chosen from a local `git merge-file` survey of which module pairs conflict, so 8 of 32 over-states the natural rate; unselected pairs 1-26 gave 2 of 26). Two pairs (8, 32) were already resolved by the sync workflow before the harness read the state, so their GitHub reading is from the workflow log and PR history, not from the API poll. Trial 8 also showed a transient `405 has merge conflicts` right after A merged; a retry about 20 s later succeeded for non-conflicting pairs, so the harness retries once.

**Driver alone is not enough (re-measured on the 8 conflicting pairs, local, Bazel 9.3.1).** Driver merge exit 0 for 8 of 8; `--lockfile_mode=error` on the merged tree fails (exit 2) for 6 of 8 (pairs 8, 20, 27, 28, 29, 30), passes for 2 (31, 32); after one `bazel mod deps --lockfile_mode=update` it passes 8 of 8. This matches the 13-of-20 finding of the earlier replay.

**Q2, auto-regeneration in Actions (VERIFIED).** `.github/workflows/sync.yml` in the scratch repo (adapted from `bazel-autofix.yml`; kept in the scratch repo only) runs on a push to a base branch: for each open PR it tests `git merge --no-commit origin/<base>`; on a conflict confined to `MODULE.bazel.lock` it takes the PR side (`git checkout --ours`), commits the merge, runs `bazelisk mod deps --lockfile_mode=update`, commits the regenerated lock, checks `--lockfile_mode=error` passes, and pushes a normal merge commit to the PR branch with `GITHUB_TOKEN` (never forced). Result: 8 of 8 conflicting PRs got a bot push (merge commit plus `chore(build): regenerate MODULE.bazel.lock`) within about 30 s of A's merge; GitHub then reported `mergeable=MERGEABLE`, `mergeStateStatus=UNSTABLE` (not `CLEAN`, see the next paragraph); 8 of 8 merged through the API afterwards. Note for Story 7.8: the repo's `bazel-autofix.yml` as written runs on `pull_request` and regenerates on the PR branch only; it never merges the base in and `pull_request` does not fire when the base moves, so it cannot clear this conflict. The sync step above (a `push`-to-`main` job that merges main into each conflicting same-repo PR) is the missing piece; `bazel-main-repair.yml` opens a repair PR after the fact and would not help the stuck PR either.

**`GITHUB_TOKEN` pushes (VERIFIED, limitation).** The bot push does not run the PR's checks. GitHub created a `verify` run for the new head (event `pull_request`, actor `github-actions[bot]`) that finished `failure` in 7 s with zero jobs and the message "This run likely failed because of a workflow file issue"; `check-runs` on the new head: 0 (6 of 6 bot pushes the harness checked, pairs 20 and 27-31; pairs 8 and 32 show the same ghost run in the run list), commit status `pending`. That is why B shows `UNSTABLE` instead of `CLEAN`, and a required status check on the PR would never turn green. Remedies already in the plan: a repo-scoped token or app token (`BAZEL_REPAIR_TOKEN`, Task 7.8a), or `workflow_dispatch` of the gate on the PR branch, which is exempt from the rule.

**Q3, `--lockfile_mode=error` after merge-by-regeneration (VERIFIED).** 32 pairs, 32 merged, final `tN` tip `verify` (`--lockfile_mode=error`) green for 32 of 32 (24 pairs merged with no regeneration because GitHub saw no conflict, 8 through the sync workflow). The only failed `main` run was the bad-base push above. Not run here: a ten-pair arm without regeneration, because for non-conflicting pairs GitHub's own textual merge already passed `error` mode 24 of 24; the 6-of-8 failure of the driver alone (above) is the no-regeneration datapoint for conflicting pairs.

**Q4, merge queue and branch protection on a private personal repo.** Merge queue unavailable: `POST /repos/{r}/rulesets` with a `merge_queue` rule returned `422 Validation Failed: Invalid rule 'merge_queue'` (user-owned repo; merge queue is for organisation repositories). Branch protection with a required status check `verify` was accepted (`PUT /branches/main/protection` returned 200; removed again afterwards), so a required check is available on this account. Nothing was upgraded or paid for. Without a merge queue, the CI auto-regen sync job plus a required check is the available path.

**Actions cost.** 270 runs on `ubuntu-latest` (173 `verify`, 97 `sync`), about 6,300 s wall-clock in total, at most 276 billed minutes counting every run as at least one billed minute (job-level rounding up). No run was cancelled by hand; 9 runs ended `failure`: 8 ghost runs and the one bad-base `main` push.

Commands (typed by hand; no new repo helper): `gh repo create tstapler/bazel-lockfile-merge-experiment --private`; per pair `git checkout -B tN main && git push origin tN`, then for each of A and B `git checkout -B pr-N-x tN`, edit one `# SLOT-n` line of `MODULE.bazel` into a `bazel_dep(...)`, `tools/bazel mod deps --lockfile_mode=update`, commit, `git push origin pr-N-x`, `gh pr create -R <repo> --base tN --head pr-N-x`; then `gh api -X PUT repos/<repo>/pulls/<A>/merge -f merge_method=merge`, `gh pr view <B> --json mergeable,mergeStateStatus`, `gh api -X PUT repos/<repo>/pulls/<B>/merge`, `gh api -X PUT repos/<repo>/pulls/<B>/update-branch`, `gh run list --workflow verify --branch tN --event push`; ruleset and protection probes with `gh api -X POST repos/<repo>/rulesets` and `gh api -X PUT repos/<repo>/branches/main/protection`; local confirmation with `git merge-tree --write-tree` and `git merge` with `git config merge.bazel-lockfile-merge.driver "jq -s -f bazel-lockfile-merge.jq -- %O %A %B > %A.jq_tmp && mv %A.jq_tmp %A"`.

### Verdict

`S13`: **FallbackWorkaround**, path `commit lock + client merge driver + CI auto-regen workflow + required check`, cost S. GitHub-side merges ignore the repo merge driver (8 of 8 driver-resolvable conflicts stayed conflicts on GitHub), so the driver only helps local merges and rebases; and the driver alone leaves 6 of 8 conflicting merges red in `error` mode. Zero red tips over 32 merged pairs was reached only with the regeneration step, so it is a workaround, not a plain Pass. Not committing the lock stays the rejected alternative (loses `error`-mode reproducibility, needs owner approval; unchanged from the replay). ADR-001's consequence line (lock committed unless S13 decides otherwise) is unchanged. Follow-ups for Story 7.8 (not done here): extend `bazel-autofix.yml` or add a sibling job that merges the base into conflicting same-repo PRs on a push to `main`, using the token choice of Task 7.8a so the gate re-runs. The scratch repo remains (private) until the owner deletes it; the account's token lacks the `delete_repo` scope, so deletion needs `gh auth refresh -s delete_repo && gh repo delete tstapler/bazel-lockfile-merge-experiment --yes`.

## S11 whole-repo Gazelle (Story 0.11, Tasks 0.11a, 0.11b, 0.11c)

Host onyx (load 16-24), Bazel 9.3.1 via `tools/bazel`, Gazelle 0.54.0, branch `sdd/bazel-epic0-s11-gazelle`. Generated BUILD files were NOT committed (Task 1.1a commits them); this spike commits only directives, the `go_test` macro, `# gazelle:ignore` lines on 5 hand-written BUILD files and these results.

**Verdict: `Pass`, path = directives + `map_kind` macro, cost M** (no custom extension). All four PASS-iff checks are true as measured; the importer build below is an extra check, not part of the matrix row.

### Checks (PASS iff list)

| Check | Result | Evidence |
|---|---|---|
| Second whole-repo Gazelle run has zero diff | true | all 141 generated files removed, `bazel run //:gazelle` (exit 0), then `bazel run //:gazelle -- -mode=diff`: 0 output lines, exit 0. Repeated after the `build_tags` and `map_kind` directives were added: 0 lines, exit 0 |
| Zero diff with and without a synced GenSurface | true | GenSurface = gitignored copies of `//proto/session/v1:gen_go` output in `gen/proto/go/session/v1` (37 `.go`) and `//tools/bzl/ent:ent_gen` output in `session/ent/**` (333 `.go`, `git check-ignore` confirms ignored). SHA-256 over (`git status --porcelain '*.bazel'` + content hashes of all added/modified BUILD files) was `ec62e531509af7e1` with the surface and `ec62e531509af7e1` after moving it away and re-running (also identical in an earlier pass before `build_tags`/`map_kind`: `3efaac9a02150cbe`/`3efaac9a02150cbe`); `-mode=diff` was 0 lines in both states. No `BUILD*` appears under `gen/` or the generated `session/ent/*` |
| At most 8 `resolve`/`resolve_regexp` lines | true, 6 | root `BUILD.bazel`: 2 ent (1 `resolve` + 1 `resolve_regexp`, `$1` substitution works), 2 proto, 1 `server/web`; `tools/lint/BUILD.bazel`: 1 `resolve_regexp` (isolated `lint_x_tools`). No per-entity line |
| `session/ent/schema` still gets its generated BUILD file | true | deleted `session/ent/schema/BUILD.bazel`, ran Gazelle: regenerated identically except for the hand-added `schema_srcs` filegroup, which Gazelle does not own and which survives a normal merge run (restored from backup) |

### Numbers

- Go package directories (outside excluded trees): 227 (`find`); BUILD.bazel files after the run: 181 = 40 tracked at start + 141 new. Gazelle wrote the 141 new files and rewrote 28 tracked ones (the `go_test` load line, the root package rules, and the 5 hand files before they were ignored); those rewrites were reverted, not committed.
- `bazel query //...`: 566 labels; by kind: 222 `go_library`, 143 `go_test`, 15 `go_binary`, 155 `alias` (import_alias naming), plus hand-written rules. 0 query errors after the two fixes below.
- Gazelle wall time: 1.05 s from an empty state (all generated BUILD files deleted), 0.5-3 s for `-mode=diff` (warm Bazel server, measured with `date`, host load about 22). A first call after server start queues behind the `tools/bazel` slot wrapper (one measured wait of 97 s that is queueing, not Gazelle).
- Unresolved imports: 0. The only Gazelle warnings are `pattern embed/tmux: matched no files` and `pattern embed/tymuxd: matched no files` (empty directories, confirms ADR-007: the embed `select()` stays hand-maintained).
- Exit codes of the freshness form `bazel run //:gazelle -- -mode=diff`: 0 when nothing differs, 1 when a diff exists (seen with 9322, 221 and 16-file diffs).

### Task 0.11b: ent exclusion mechanism (decision)

The S2 two-line form (`session/ent/[!s]*`, `session/ent/s[!c]*`) is **brittle, confirmed**: a throwaway `session/ent/scfoo/x.go` made Gazelle emit `session/ent/scfoo/BUILD.bazel`. Replaced with seven prefix-negation globs that exclude every name under `session/ent` except exactly `schema` (tested: `scfoo`, `sc`, `schemb`, `schemaX` all excluded, `schema` still generated). Residual limit: a generated package literally named `schema` cannot exist (ent reserves it), and one- or two-letter names `s`/`sc` are excluded by the earlier lines. No per-entity list, no extension. The `check-build-paths.sh --full` mode in Task 1.1a remains the backstop.

### Task 0.11c: `map_kind` go_test macro

`tools/bzl/go_test.bzl` (17 lines): `go_test(name, **kwargs)` wrapping rules_go `go_test`, sets `size`/`shard_count` from a package table (spike table: `server/services` -> enormous x4, `session` -> large x2). Root `# gazelle:map_kind go_test go_test //tools/bzl:go_test.bzl`. Gazelle then emits `load("//tools/bzl:go_test.bzl", "go_test")` in every package (141 generated + rewritten tracked files). Second run still zero diff. `bazel query 'attr(size, enormous, //server/services:all)'` prints `//server/services:services_test`; `attr(shard_count, 2, //session:all)` prints `//session:session_test`. Extending it for Epic 4 (sandbox tags, timeouts) is a table edit.

### Importer build (S2 deferred check)

`bazel build --config=agent --keep_going //session/... //server/...` (after Gazelle, with `build_tags`, GenSurface absent for Bazel since `gen/` and the generated ent are Bazel outputs): **246 of 253 top-level targets built, 7 failed, 1 failing action** (`BunWebBuild //web-app:dist`: `Could not resolve "@/gen/session/v1/session_pb"`, the TypeScript proto output of Task 0.6e/S1-ts is not wired). The 7 are exactly `rdeps(//session/... + //server/..., //web-app:dist)`: `//server`, `//server:server`, `//server:server_test`, `//server/services:services_test`, `//server/web`, `//server/web:web_test`, `//server/web:dist_staged`. All ent importers (`session`, `server/services`, `server/handlers`, ...) and all proto importers except via that embed chain compiled. Cause is outside Gazelle's remit (web build input), so a finding, not an S11 failure. Not built: nothing was run (tests not executed). This closes the S2 caveat for ent importers: nothing in the ent/proto resolution breaks, so S2 need not be downgraded to `Fail`.

### Things Gazelle gets wrong or needs told (each with the directive)

| Area | Behavior | Fix |
|---|---|---|
| Hand-written rules claim importpaths | `# gazelle:ignore` does NOT remove a file's rules from the resolver index: `gen/proto/go/session/v1`, `.../sessionv1connect` matched both S1's abandoned `go_proto_library` and the genrule `go_library` ("matches multiple rules"), and `server/web` matched `server/web/fixture:web`; Gazelle then silently dropped the dep from importers | 3 `gazelle:resolve` lines in root BUILD (above). Cleaner long term: delete the S1 `go_proto_library` attempts and give the fixture another importpath, which frees 3 lines |
| Hand BUILD files get rewritten | without `# gazelle:ignore` Gazelle reformatted `web-app`, `server/web`, `proto/session/v1`, `spike-tags` and rewrote `proto/session/v1` rules | `# gazelle:ignore` first line added to those 5 (committed here) |
| Nested directory that a hand file addresses by path | Gazelle created `tools/bzl/spike-tags/cmd/BUILD.bazel`, making `cmd/main.go` in the parent's `srcs` an invalid label (`bazel query //...` errors) | `# gazelle:exclude cmd` in `tools/bzl/spike-tags/BUILD.bazel` |
| Build tags | tagged files are silently dropped without the directive (dozens of test files: gitoracle 16, integration 13, sinkguard 8, embed_tmux/embed_tymux 4 each, live_github 2, harness 2, otelcauto 2, e2e_ghsignal 1) | `# gazelle:build_tags integration,gitoracle,sinkguard,harness,live_github,embed_tmux,embed_tymux,otelcauto,e2e_ghsignal`; rules_go filters by `--@rules_go//go/config:tags` (S7). `otelcauto` and `e2e_ghsignal` are new relative to the S7 list |
| cgo | root package (`infoplist_darwin.go`) emitted with `cgo = True`, `embedsrcs = ["CHANGELOG.md"]`; correct, no directive | none; `session/procinfo` and `session/tmux` also carry `cgo = True` |
| `go:embed` of empty/ignored dirs | `embedsrcs` omitted (warnings above) | hand-maintained select (ADR-007) |
| `**/testdata`, `node_modules`, `.claude`, `third_party/tmux`, `project_plans`, `website`, `web-app`, `gen` | no BUILD file generated in any of them (list of new BUILD dirs by top-level: session 58, server 24, internal 12, pkg 10, testutil 7, cmd 6, tests 5, tools 3, config 3, rest 1-2 each) | existing excludes work |
| Vendor-like dirs | none exist in the tracked tree | none |
| Nested modules | `tools/lint`, `tools/scanner`, `tuitest` keep their own `# gazelle:prefix`; `tools/lint` needs its `resolve_regexp` for the isolated universe | already in place |
| `session/ent/schema` dep label | the ent `resolve_regexp` also rewrites `//session/ent/schema` imports to the alias `//tools/bzl/ent:schema` (entc's dep changed) | harmless; Epic 1 keeps the alias |

### Cost and fallback

Directives-only path, no extension: actual cost S for the run plus M for diagnosing the three resolver/hand-file interactions. Fallback (custom extension) not needed. Remaining resolve budget: 8 - 6 = 2 (3 more after the S1/fixture cleanup).

### False or unsafe plan assumptions

1. Task 1.1a says Gazelle output "is never rewritten wholesale" and hand files need only the header later; in fact `# gazelle:ignore` is needed before the first run, and ignore does not take a file's rules out of the resolve index, so abandoned hand rules with real importpaths force `gazelle:resolve` lines.
2. Plan labels `//proto/session/v1:session_go_proto` / `:session_connect_go` do not exist; the real genrule-based labels are `:sessionv1_go` and `:sessionv1connect_go` (Task 1.1a should use those).
3. "`session/ent/[!s]*` spares schema" (provisional form) was correct but incomplete for `s*` (S2) and still leaks `sc*`; seven globs are needed for an exact "all but schema".
4. Plan says a second Gazelle run "changes nothing"; the first run over a tree holding hand-written `go_library` rules actually prints resolution ambiguities and drops deps silently (exit 0). Idempotence alone does not catch it; Task 1.1a should also fail on any `which matches multiple rules` line in Gazelle output.
5. Plan Task 0.11a verify says `-mode=diff; echo $?` prints `0`; true only when nothing differs (exit 1 otherwise).
6. `bazel query` and `--config=agent`: `agent` is a `build:` config and is rejected by `bazel query`.

### Owner actions

- Decide whether Epic 1 deletes the S1 `go_proto_library` attempts (frees resolve lines) before running Task 1.1a.
- Wire the TypeScript proto output (Task 0.6e) before expecting `//server:server` and `//server/web:*` to build.

## Importer completion check (S2/S11)

Run 2026-11-01 in worktree `bazel-epic0-importers` (branch `sdd/bazel-epic0-importers`, based on `sdd/bazel-epic0` with S-gomod, S1, S1-ts, S2, S3, S4, S5, S7, S11 and S13 merged), Bazel 9.3.1 via `tools/bazel`. Generated BUILD files were regenerated and not committed (Task 1.1a commits them).

Previously UNEVALUATED: (a) S2's importer check covered only `//session/scanstate` (the other importers needed S1 and Story 0.11); (b) S11's importer build left 7 of 253 targets failing because the TypeScript proto output was not wired; (c) no importer test was ever executed.

Measured (VERIFIED by command):

1. `bazel run //:gazelle`: exit 0, 141 new and 23 modified BUILD.bazel files (same 141 as S11; the 23 modified are the `go_test` load-line rewrites). Only warnings: `pattern embed/tmux` and `pattern embed/tymuxd` matched no files (ADR-007). No `matches multiple rules` line. Second run `-mode=diff`: no diff, exit 0.
2. `bazel build --config=agent --keep_going //session/... //server/...`: exit 0, **253 of 253 targets analyzed and built, 0 failed** (2306 actions, 36 s with a warm disk cache). The 7 S11 failures (`//server`, `//server:server`, `//server:server_test`, `//server/services:services_test`, `//server/web`, `//server/web:web_test`, `//server/web:dist_staged`) are gone because `//web-app:dist` now takes `gen = //tools/bzl/proto:gen_ts` (S1-ts). No directive or hand-file change was needed after the merge.
3. `bazel test --config=agent` on `//pkg/events:events_test //session/ent/schema:schema_test //session/scanbuf:scanbuf_test //session/scanstate:scanstate_test //tools/lint/norawdb:norawdb_test`: 5 of 5 pass (4 executed, 1 cached). These import the generated ent tree. No sandbox failures seen in this subset; the tmux/git/port-binding tests were deliberately not run.

Result: the ent and proto importer closure of `//session/... //server/...` builds in full, so S2's importer check is now satisfied. S2's verdict row stays `FallbackWorkaround` (its path is a workaround), S11 stays `Pass`; no verdict changes. Not covered: tests of `//session` and `//server/...` (they spawn tmux, git servers and bind ports; Epic 4 sandbox classes).

## S12 (Story 0.13 / Task 0.13a): custom analyzer as nogo

Run 2026-11-01, Bazel 9.3.1, rules_go 0.65.0, host load average 11 to 29 during the timings (shared with other workers).

### Verdict

`S12`: **FallbackWorkaround**. Path: nogo with `norawgitcli` recompiled against the ROOT `x/tools` universe (a second `go_library` over the same `analyzer.go`, see "False assumption 1"); `make lint-custom` stays the whole-suite path of record. Cost: M (plus S per extra analyzer, see below). Story 8.4 is NOT skipped but its design changes (below). A plain `Pass` is not allowed: the plan's assumed path (register `//tools/lint/norawgitcli` directly as nogo dep) fails; the workaround is not pre-approved in `allowed-deltas.txt`.

### Checks (VERIFIED, run in this worktree)

1. Plan path (a) as written: `nogo(deps=["//tools/lint/norawgitcli"])` + `go_sdk.nogo(nogo=...)`, `bazel build //tools/bzl/spike-nogo:clean` -> **FAIL** at `GoLink nogo_actual`: `package conflict error: golang.org/x/tools/go/analysis: multiple copies of package passed to linker: @@gazelle++go_deps+org_golang_x_tools//go/analysis, @@gazelle++_go_deps+++lint_go_deps+org_golang_x_tools//go/analysis`.
2. Repair (the one allowed repair run): `tools/bzl/spike-nogo/BUILD.bazel` `norawgitcli_rootdeps` = `srcs = ["//tools/lint/norawgitcli:analyzer.go"]` (new `exports_files`) with `deps = @org_golang_x_tools//...` (root universe, v0.51). `bazel build //tools/bzl/spike-nogo:clean` exit 0 (clean code passes).
3. `bazel build //tools/bzl/spike-nogo:seed` (seeded `exec.Command("git","status")`) exit 1 with `nogo: errors found by nogo during build-time code analysis: tools/bzl/spike-nogo/seed/seed.go:7:37: exec.Command invokes the git CLI directly — route it through session/git/backend (...) (norawgitcli)`. `grep -c norawgitcli` on that output is at least 1.
4. Agreement with `make lint-custom`'s linter (`go -C tools/lint build ./cmd/linter`, run as `linter ./tools/bzl/spike-nogo/...`): same message at the same position `seed.go:7:37`. The linter additionally reports `norawexec` on both files (a different analyzer; not registered in this spike), and nothing from `norawgitcli` on `clean.go`. Agreement on the norawgitcli finding: yes.
5. Cross-module dependency: `//tools/lint/norawgitcli` (in `@lint_x_tools`) cannot be a nogo dep (check 1), `local_path_override` is not needed (the module is in-repo, same label space; the obstacle is the dual `x/tools`, not the module boundary). Consequence: every analyzer used as nogo needs a root-universe twin target (S per analyzer, same srcs, `x/tools` deps swapped), and the root `go.mod`'s `x/tools` (v0.51.0) must stay API-compatible with what analyzers use. All 17 share `internal/nolintcomment`, which needs a twin too.
6. `analysistest` made to execute under Bazel for `norawgitcli_test`: before, `--test_arg=-test.v` shows `analysistest.go:402: skipping because go tool not available` and `--- SKIP: TestAnalyzer`. After adding `data = glob(["testdata/**"]) + ["@rules_go//go"]` and `gotool.Setup(t)` (new `tools/lint/internal/gotool`: finds the SDK in runfiles, then re-execs the test binary once with `GOROOT`, `PATH`, `GOCACHE`, `GOFLAGS=-mod=mod`), it shows `--- PASS: TestAnalyzer (2.24s)`. Assertions bite: changing one `// want` in `testdata/src/a/a.go` to a wrong regexp fails the target with `analysistest.go:713: a/a.go:23: no diagnostic was reported matching \`NOT-THE-MESSAGE\``; fixture restored (git status clean for it). Why the re-exec: testenv requires `go env GOROOT == runtime.GOROOT()`, and in a Bazel-built binary `runtime.GOROOT()` is the literal `GOROOT` and ignores a `Setenv` made after start, so the env must be set before the process starts (a `go_test(env=...)` cannot name the SDK path). Cost to apply to the other 14 analyzer tests: S each (one `data` line in BUILD, one `gotool.Setup(t)` call, one import); a shared `TestMain` or a rules_go-provided wrapper would be cleaner (Epic 4). Only `norawgitcli` was converted here. `bazel test //tools/lint/...`: 16 tests pass after the change.
7. Timing, `bazel clean` + `--disk_cache=` (disabled) + `-k`, target set `//proto/session/v1/... //tools/scanner/... //tools/lint/... //session/ent/schema/... //tools/bzl/ent/... //tools/bzl/spike-nogo:clean` (130 top-level targets; `//session/...` cannot be built because the main module has no BUILD files before Epic 1, so this is a stand-in, norawgitcli ran on every non-external Go target in it): without nogo **86.9 s** wall, 1332 processes (load average 11 at start, 28 at end); with nogo **76.8 s** wall, 1747 processes (load 27 at start, 29 at end). Both runs: 128 of 130 targets built, the same 2 pre-existing analysis failures (`//session/ent/schema:schema_test` needs `//session`). Wall delta is inside the noise of a loaded host (nogo faster by 10 s is not a speedup); the reliable signal is +415 actions (+31%), including one-time nogo binary link. One measurement each; no repetition.

### False plan assumptions

1. "Register `//tools/lint/norawgitcli:norawgitcli` as nogo dep" (Task 0.13a): false; link-time duplicate `x/tools` (check 1).
2. Task 0.13a's verify path `//tools/bzl/spike/nogo_seed:all`: the spike lives at `tools/bzl/spike-nogo` (the `tools/bzl/spike/` dir is a Python/script dir); the seed target is tagged `manual` so `bazel build //...` stays green.
3. Plan fallback matrix "Keep `make lint-custom` wrapped (zero)": still true and still needed; nogo covers analyzers in the Bazel build path only, and 14 of 15 analyzer tests still skip.
4. `go_sdk.nogo` registers ONE nogo for the whole repo, so the spike left it commented out in `MODULE.bazel` (enabling it lints every Go target; scope with `exclude_files`/`only_files` in `nogo_config.json`).

### Files

`tools/bzl/spike-nogo/{BUILD.bazel,nogo_config.json,seed/seed.go,clean/clean.go}`, `tools/lint/internal/gotool/{gotool.go,BUILD.bazel}`, `tools/lint/norawgitcli/{BUILD.bazel,analyzer_test.go}` (exports_files, data, gotool), `MODULE.bazel` (commented registration line).

### Implication for Story 8.4

Feasible but not free: per analyzer a root-universe twin library, `internal/nolintcomment` twin, one `nogo` target listing the twins, `go_sdk.nogo` in `MODULE.bazel`, and a `nogo_config.json` scope. Analyzers that need `go/packages` at runtime were not examined. Decision for the owner: pay that (and keep two build paths) or leave `make lint-custom` as the only path; both are consistent with the evidence.

### Owner actions

- Decide 8.4: nogo with root-universe twins (M plus S per analyzer) versus keep `make lint-custom` only.
- Enabling nogo repo-wide is a `MODULE.bazel` one-line change; do not enable before Epic 1 provides BUILD files and the 57-site/ratchet baselines are known.

## S8/S9 sandbox census and unix-socket path (Story 0.9, Tasks 0.9a and 0.9b)

Run 2026-11-01/02 in worktree `bazel-epic0-s89-census` (branch `sdd/bazel-epic0-s89-census`, based on `sdd/bazel-epic0`), Bazel 9.3.1 through `tools/bazel`. Gazelle was run (`bazel run //:gazelle`, exit 0, it also rewrote 23 committed BUILD files, which were not committed). Verdicts: **S8 Pass, S9 Pass (with the caveat under S9)**.

### Isolation and harness (VERIFIED)

Every run: `--test_env=HOME=~/.cache/stapler-squad-census/home --test_env=TMUX_TMPDIR=.../tmux --test_env=STAPLER_SQUAD_INSTANCE=census --test_env=GO_TEST_WRAP_TESTV=1 --sandbox_writable_path=~/.cache/stapler-squad-census --sandbox_default_allow_network=false --test_timeout=300 --local_test_jobs=1 --jobs=4 --local_resources=memory=3000 --sandbox_debug --test_output=errors --keep_going --nocache_test_results`. Without `GO_TEST_WRAP_TESTV=1` rules_go writes an empty `<testsuites>` for passing tests, so no pass counts. The default tmux socket (`/tmp/tmux-1000/default`, inode 40, mtime 2026-11-01 17:32:06) was unchanged after every package: no HOST_STATE_ESCAPE. One unisolated rerun (see below) also left it unchanged. Scratch peak 12 MB under `~/.cache/stapler-squad-census` (deleted at the end).

### Task 0.9a result (census.json, `python3 tools/bzl/spike/census-check.py` exit 0)

`python3 -m unittest tools/bzl/spike/census-check_test.py`: 7 tests OK. `census-check.py` re-parses every `test.xml` (incl. `shard_*_of_*/test.xml`) and printed `{"failures": 484, "by_class": {"cwd": 211, "exec-host-binary": 62, "home": 21, "shard-timeout": 190}, "problems": 0}` (211+62+21+190 = 484).

| Package (target) | passed | failed | skipped | cwd | exec-host-binary | home | shard-timeout |
|---|---|---|---|---|---|---|---|
| pkg/events | 20 | 0 | 0 | | | | |
| config | 308 | 0 | 0 | | | | |
| github | 317 | 8 | 0 | 8 | | | |
| session/vc | 127 | 4 | 23 | | | 4 | |
| session/mux (repair run) | 36 | 0 | 2 | | | | |
| session/tymux | 363 | 1 | 3 | 1 | | | |
| session/tmux (repair run) | 492 | 0 | 16 | | | | |
| tuitest/integration/claude_squad | 0 | 2 | 0 | | 2 | | |
| session | 4520 | 104 | 37 | 61 | 26 | 17 | |
| server/services | 4326 | 365 | 12 | 141 | 34 | | 190 |

`tuitest` has no `go_test` at its root; the only test target is `//tuitest/integration/claude_squad`. Failure counts include failing subtests and their parents (parents inherit the first classified subtest's class). The 190 `shard-timeout` rows are one cause: shard 2 of `server/services` aborted at `-test.timeout` (`panic: test timed out`, running test `TestStreamViaHub_should_SendHubStartFailedError_...`) and the 190 tests that never finished have `No pass/skip/fail event found`; the hung test itself is `exec-host-binary` (see below), the other 189 are collateral. Classes `runtime-caller`, `tmpdir-socket`, `network` and `LOAD_FLAKE`: 0 each. `other`/unclassified: 0 at the end (see how it got there below).

Evidence per class (regex or hand override, recorded per row in `census.json`): `cwd` = relative `testdata/`, `../../...` fixtures, `scripts/ssq-*` and source files (`open host_registry.go: no such file`) not present in the sandbox cwd, plus source-scanning guards that find zero files; `home` = `Author identity unknown`/`no git author identity` (the private HOME has no `~/.gitconfig`, which is a property of the isolation) and `TestTildeExpansionInNewInstance`/`TestMigrationOfCorruptedPaths` expecting `$HOME` where the code reads the passwd home; `exec-host-binary` = `exec: "tmux"`/`"go"`/`gh` not on the test PATH (Bazel's test PATH is `/usr/bin:/bin`; tmux here lives in `/home/linuxbrew/.linuxbrew/bin`) and `env: 'tmux'` on the remote-exec fake.

Noise control: the 6 timeout-looking `server/services` tests were rerun alone (`--test_filter`, load 3.4): all 6 failed again (not LOAD_FLAKE); with `--test_env=PATH=/usr/bin:/bin:/home/linuxbrew/.linuxbrew/bin` 5 of the 6 passed (including the hung test), the sixth (`TestGetSessionTurnTimeline_...`) opens `../../session/tokens/testdata/...` and is `cwd`. The `--test_timeout=900` rerun of the whole target hung at the same test for 15 min, so it is not a 300 s cap artefact.

go test sanity (not a gate; `gen/` and `session/ent/*` are absent in this worktree so only 2 packages compile): `config` go test 308 PASS / 0 FAIL equals Bazel 308/0; `session/vc` go test with the same private HOME 150 PASS / 4 FAIL (the same 4 git-identity tests that Bazel reports), so the `home` class there comes from the private HOME, not from Bazel. `pkg/events` go test: setup failed (needs `session/ent`, not generated here). Pass counts differ in definition between the two harnesses (go test counted PASS lines incl. subtests).

### Deviations and repairs (cap: one repair per task, exceeded once, stated)

1. First `session/mux` (3 fails) and `session/tmux` (4 fails: `TestPrependSocket`, `TestResolveSocket` + subtests) runs with `STAPLER_SQUAD_TMUX_SOCKET=ssq-census-...` failed because those tests assert the socket name `test-isolated-N`; with the variable omitted (repair run, `TMUX_TMPDIR` still private) both packages pass. Not a Bazel class; this contradicts Task 4.1b's `--test_env=STAPLER_SQUAD_TMUX_SOCKET=ssq-bazel-$(uniq)`. `session` and `server/services` were therefore run without the variable from the start.
2. `server/services` ran three times (initial 300 s, 900 s, then 300 s again so `bazel-testlogs` matches `census.json` after the filtered reruns overwrote it).
3. One filtered `server/services` run was started without the isolation flags by mistake (bare `bazel test //session:session_test` for diagnosing a missing `test.xml`); it ran in the sandbox with Bazel's default test env (HOME unset) and left the default tmux socket unchanged.
4. `census.py --parse-only` overwrote the per-package `load_start/load_end/status` in `census.json`; real-run loads were: pkg/events 11.4 to 14.3, config 14.3 to 14.0, github 14.0, session/vc 13.0, session/mux 13.0, tymux 13.3, tuitest 13.3, tmux 12.6 to 10.3 (10.1 to 9.0 repair), session 6.8 to 7.9, server/services 7.5 to 3.7 (later runs 3.9 to 3.3). Host load at start was about 30 (see plan), at run time 3 to 20; available memory stayed at or above 9 GB (never below the 8 GB gate).

### Task 0.9b / S9: socket path probe

`tools/bzl/spike/sock_test.go` binds `os.TempDir()+"/ssq-1234.sock"`; `sock_long_test.go` (tag `manual`) binds under `TEST_TMPDIR`.

| Case | socket path | `wc -c` (path+NUL) | result |
|---|---|---|---|
| `//tools/bzl/spike:sock_test`, default (and with `--test_env=TMPDIR=/tmp`) | `/tmp/ssq-1234.sock` | 19 | PASS |
| `//tools/bzl/spike:sock_long_test` (under `$TEST_TMPDIR`, `.../sandbox/linux-sandbox/19/execroot/_main/_tmp/<hash>/ssq-1234.sock`) | long | 169 | FAIL (`bind: invalid argument`, limit 108) |
| `--test_env=TMPDIR=<146-byte dir>` | still `/tmp/...` | 19 | PASS: the override is ignored |
| `--spawn_strategy=local` | `/tmp/...` (the host's shared RAM-backed `/tmp`) | 19 | PASS |

Mechanism found (VERIFIED from the `linux-sandbox` command line printed by `--sandbox_debug`: `-M .../sandbox/linux-sandbox/N/_hermetic_tmp -m /tmp`, and the test env `TMPDIR=/tmp`): Bazel 9.3.1 mounts a private per-action `/tmp` and forces `TMPDIR=/tmp` for tests with no flag at all, so tests that use `os.TempDir()`/`t.TempDir()` get short socket paths for free; `--test_env=TMPDIR=...` cannot change it. The failing arm is a test that binds under `TEST_TMPDIR`/the cwd (execroot is 120+ bytes under `~/.cache/bazel/_bazel_tstapler/<hash>/`). `session/mux` (36 pass) and `session/tymux` (363 pass, 1 `cwd` fail) socket tests pass in the census. Census `tmpdir-socket` count: 0. Caveat for the verdict: the "without it" arm could not be produced by switching the mechanism off (no flag does), so it is shown by the TEST_TMPDIR bind; `--incompatible_sandbox_hermetic_tmp` is absent: `bazel help build --long | grep -c incompatible_sandbox_hermetic_tmp` prints `0`. Flags confirmed in `bazel help build --long`: `--sandbox_base` (line 440), `--sandbox_tmpfs_path` (458), `--sandbox_add_mount_pair` (1511), `--sandbox_writable_path` (1527), `--test_tmpdir` (1994); `--experimental_use_hermetic_linux_sandbox` is not in the help output. Verify line: `bazel test --test_env=TMPDIR=/tmp //tools/bzl/spike:sock_test` exit 0.

Local-strategy caveat: a `no-sandbox`/`local` test (S4's repo-rule targets) sees the host `/tmp` (RAM-backed tmpfs), not a private one.

### False or unsafe plan assumptions

1. Task 4.1b: `STAPLER_SQUAD_TMUX_SOCKET=ssq-bazel-$(uniq)` as a global `--test_env` breaks tests that assert their own isolated socket name (3 in `session/mux`, 4 in `session/tmux` here). Set it per test only where a test needs it, or not at all with `TMUX_TMPDIR` private.
2. Story 0.9 says a short `TMPDIR` is what the spike must find; on 9.3.1 it is already the default, and `--test_env=TMPDIR` is overridden. Task 4.1b's `--test_env=TMPDIR=/tmp` is a no-op.
3. A private HOME (needed for isolation) makes every git-committing test fail with `Author identity unknown` (21 here): Epic 4's `home` sweep needs a test HOME fixture with a gitconfig or `GIT_AUTHOR_*`/`GIT_COMMITTER_*` env, not just a writable HOME.
4. Bazel's test PATH is `/usr/bin:/bin`, so Linuxbrew tools (tmux, gh) are invisible (62 `exec-host-binary` failures, plus 190 `shard-timeout` rows from the shard abort one of them caused): one hung test (a WebSocket read with no deadline) turned into 190 `No pass/skip/fail` results.
5. `testdata`, `../../web-app/...` fixtures and `scripts/` are not in the test runfiles because Gazelle `exclude **/testdata` and no `data`/`embedsrcs` are generated: 211 `cwd` failures are mostly one fix (declare data).
6. The 49-of-154 "sandbox-hostile packages" figure is not re-derived here (only 10 packages were run): 6 of 10 failed after the repair runs (github, session/vc, session/tymux, tuitest/integration/claude_squad, session, server/services); pkg/events, config, session/mux and session/tmux passed (the last two only after dropping the socket env var).

### Owner actions

None required. Epic 4 inputs: (a) decide a test HOME fixture (gitconfig), (b) put the tmux/gh directory in the Bazel test PATH or make tests skip, (c) generate `data` for `testdata/`, `scripts/` and the web fixtures, (d) drop the global `STAPLER_SQUAD_TMUX_SOCKET` and `TMPDIR` test env from Task 4.1b.


## Task 4.2b: sharding and filter semantics (2026-11-03, Bazel 9.3.1)

`bash tools/bzl/parity/shard-union.sh //server/services:services_test` exits 0 and prints `SHARD UNION EQUAL n=5225` (VERIFIED: the sorted passing-name sets of the sharded run, 4 shards via `--test_sharding_strategy=explicit`, and the unsharded run, `=disabled`, are identical; `shard_1_of_4` .. `shard_4_of_4` directories exist). Sharded wall time 47.0 s, unsharded 141.6 s. Both runs still report failing tests (the `cwd` class, fixed in Story 4.10), so the comparison is by passing names. With the Story 4.1b wrapper (tmux, gh and go on PATH) the S8 `shard-timeout` abort of shard 2 no longer occurs.

`bash tools/bzl/parity/shard-union.sh --filter-semantics` prints `filter: passthrough unanchored`: `--test_filter=TestLiveState` ran `TestLiveStateError` and its subtests (so it is not anchored), and `--test_filter='TestIsolatedHome|TestShortTempDir'` ran both tests (the value reaches Go as a regular expression, like `go test -run`). Task 8.2e can map `RUN=` to `--test_filter` unchanged.

## S15 otel-auto under Bazel (2026-11-03, Bazel 9.3.1, branch `sdd/bazel-otel-default`)

**Verdict: FallbackWorkaround** (row in `spike-results.yaml`). Question: can the owner decision "the one binary is always otelc-woven" hold under Bazel, so that the default `bazel build //:stapler-squad` yields the instrumented binary?

Approaches considered:

| Approach | Outcome |
|---|---|
| `go_binary` with the otelc toolexec through rules_go `gc_goopts`/`gotags` | Not attempted: otelc needs `otelc setup` first, which writes `otel.instrumentation.go` and `otelc.runtime.go` into the source tree and rewrites `go.mod`; a `go_binary` compiles declared sources only. |
| `genrule` | Superseded by the rule below: a genrule has no clean access to the Go SDK or stamp file. |
| Custom rule `otel_weave` (`tools/bzl/otel_weave.bzl` + `otel_weave.sh`): local, unsandboxed action that copies the checkout to `~/.cache/ssq-otel/src`, overlays Bazel's generated proto/ent code, web dist and embed staging, and runs `scripts/otel-auto-build.sh go build` with the Bazel Go SDK on PATH | **Chosen; works.** |

Evidence (VERIFIED, commands run in `stapler-squad.worktrees/bazel-otel-default`):

- `BAZEL_REAL=$(command -v bazelisk) tools/bazel build --config=agent --jobs=4 --local_resources=memory=3000 //:stapler-squad` exited 0: `Elapsed time: 67.448s ... 3 processes: 1536 action cache hit, 2 internal, 1 local` (cold otelc and Go cache, host load average 16 to 22).
- `strings -a bazel-bin/stapler-squad | grep -c 'go.opentelemetry.io/otelc'` printed 523; the live uninstrumented `./stapler-squad` of the main checkout printed 0.
- With `--stamp --workspace_status_command=tools/bzl/workspace_status.sh` the action passes the same `-X main.version=... buildinfo.*` ldflags `make` uses (the log line shows `2.8.0-274-g94b898cbb-dirty`, branch, commit, worktree).
- `bazel-bin/stapler-squad version` logs `OpenTelemetry initialized ... instrumentation_name: go.opentelemetry.io/otelc` at startup, so the weave is live.
- `git status --short go.mod go.sum` printed nothing after every Bazel build: the rewrite happens in the private copy.
- `bazel run //:gazelle -- -mode=diff` proposes no new `go_binary` for the main package after the rename of the plain target to `stapler-squad_plain`.

Non-hermetic (why this is not `Pass`): the action is `local`/`no-sandbox`/`no-remote`/`no-cache`/`requires-network`; `otelc` bumps and resolves modules from the host module cache or network; the source copy is the live checkout (Go changes still re-trigger it through the `:stapler-squad_lib` input); `otelc` itself is `go install`ed into `~/.cache/ssq-otel` at the pinned v1.0.1, not a Bazel repository. The private Go cache reached 4.6 GB across the stamped and unstamped variants; `otel_weave.sh` now drops it past 2 GB.

Not verified: `--config=release` / `embed_tmux` staging through the rule (needs the tmux and tymuxd builds; wiring mirrors ADR-007's selects), macOS (`CGO_LDFLAGS` Info.plist branch lives only in the Makefile), and Windows (cross builds use `//:stapler-squad_plain`, which is uninstrumented).

## S15b hermetic otel weave: findings (2026-11-03, Bazel 9.3.1, branch `sdd/bazel-otel-hermetic`, PROVISIONAL)

**Result: a sandboxed, remote-cacheable woven build works (`//:stapler-squad_otel_hermetic`, non-default).** `//:stapler-squad` is untouched. The approach is not one of the four listed alternatives: keep otelc's toolexec build exactly as is, but remove `otelc setup` from the build by committing its output as an overlay, and feed the build only declared inputs (call it option (e) below). No otelc fork was needed. Commits on the branch: `8a03cbc6b`, `eab12059a`, `200bbf7cb`, `c98efbd7b` (plus this doc).

### 1. How otelc v1.0.1 works (VERIFIED: source at `~/go/pkg/mod/go.opentelemetry.io/otelc@v1.0.1/tool`, plus runs)

| Phase | What it does | Evidence |
|---|---|---|
| `otelc setup` (run twice by `scripts/otel-auto-build.sh`) | Extracts the embedded rule bundle (`tool/data/otelc-bundle.tgz`, `//go:embed` in `data/export.go`) to `.otelc-build/{instrumentation,pkg}` (1.1 MB tar); writes `otel.instrumentation.go` and root `otelc.runtime.go`; **rewrites go.mod** (bumps `gofrs/flock` v0.12.1 to v0.13.0, `otlptracegrpc` v1.39.0 to v1.44.0, `otelc/pkg` to v0.0.0, adds 12 `otelc/instrumentation/*` requires, about two dozen indirect requires, a `tool` directive, `otelc v1.0.1` and 15 absolute-path `replace` lines into `.otelc-build`) and go.sum (+ lines); runs `go build -a -x -n` (build plan, `internal/setup/find.go:38`) and, when `otel.instrumentation.go` already exists (pass 2), `go mod tidy` (`internal/setup/pin.go:368`); writes `.otelc-build/matched.json`. | `diff` of go.mod before/after `otelc setup .` in a scratch copy; offline rerun with the overlay applied failed at `failed to run command "go" ... [mod tidy]`. |
| matched ruleset | 10 packages: std `log`, `log/slog`, `database/sql`, `net/http`, `runtime` (struct + raw rule), third-party `otel`, `otel/trace`, `otel/sdk/trace`, `grpc`, plus `main`. | `.otelc-build/matched.json` (python count). **It stores absolute paths**: GOROOT std files, module-cache files, the source dir, `.otelc-build`. |
| `-toolexec=otelc toolexec` | Intercepts each `compile` (`internal/instrument/toolexec.go:94`), rewrites the AST of matched packages into `$WORK`, swaps the file in the compile argv by **absolute-path equality** with `matched.json` (`apply_func.go:319-328`). | Run with a different GOMODCACHE than setup: `cannot replace ../../../../go/pkg/mod/.../context.go with $WORK/b221/context.go`. |
| vendor | When `vendor/modules.txt` exists otelc **forces `-mod=mod`** (`internal/setup/setup.go:344`, `vendor.go:forceModMod`). A vendor dir therefore cannot make the build offline; a module cache can. | source |

Trace (`strace -f -e trace=openat,execve,connect`, one cold-GOCACHE woven build of the unmodified flow, filtered to unique paths, files under `~/.cache/tmp/ssq-otel-hermetic/trace.{open,exec}`, deleted afterwards): 50,275 unique opens outside the build copy and GOCACHE, **zero `connect` calls** (warm host module cache, so the network need is latent, see below). Reads outside the tree: host module cache `~/go/pkg/mod` (40,674 opens, 895 under `cache/download`, of which 106 are sumdb tile/lookup files), the rules_go SDK repo (5,466), scratch, `/usr/lib/gcc`, `/usr/include`, `/usr/local/include` (cgo), `~/.config/go/env` and `~/.config/go/telemetry/*` (Go telemetry counters), `$PATH` probes of ~15 home directories for `bash` and `as`, host `gcc`, `ld`, `as`, `collect2`, `sha256sum`. The latent network need: with an empty module cache and `GOPROXY=off`, pass 1 fails with `go: downloading github.com/gofrs/flock v0.12.1 ... module lookup disabled by GOPROXY=off`.

### 2. Why each non-hermetic piece exists, and the removal (ranked by cost, lowest first)

| Piece in S15 | Why it exists | Removal (proved unless noted) |
|---|---|---|
| source copied from the live checkout (`git ls-files`) | the generated `otelc.runtime.go`/go.mod must live in a writable tree | **Done.** Sources come from the `GoArchive` closure of `:stapler-squad_lib` (+ the safeexec lib the weave alone imports); generated proto/ent/web/staging from their Bazel targets; 1,872 files in 175 dirs, written to a manifest. |
| `otelc setup` network/`go mod tidy`/go.mod mutation | otelc computes the rule set and module pins at build time | **Done.** Committed overlay `tools/bzl/otel_overlay/` (go.mod/go.sum with otelc pins, both generated Go files, rule bundle tar, `matched.json.tmpl` with `@SRC@ @GOROOT@ @MODCACHE@` placeholders). The weave runs only the toolexec build, `-mod=readonly`. go.mod/go.sum of the repo are never modified (`git diff` empty). |
| otelc binary `go install`ed into `~/.cache/ssq-otel` | external tool | **Done.** Built inside the action from the module cache (`go build go.opentelemetry.io/otelc/tool/cmd/otelc`), hash-pinned by go.sum. |
| host module cache and network | `-mod=mod` resolves and downloads | **Done (prototype form).** Action `OtelModcache` runs `go mod download` (`requires-network`, go.sum-verified, `-mod=readonly`) into a deterministic tarball (994 MB, 295 module zips). The weave extracts it to a private GOMODCACHE, `GOPROXY=off`. Better form (task 4): synthesize it from `go_deps` repos. |
| host Go cache | speed | **Done.** Private `GOCACHE` per execution. Price: every miss is cold (section 6). |
| `HOME`, `GOFLAGS`, `GOENV` leaks | default shell env | **Done.** `GOENV=off`, empty `HOME`, `GOFLAGS` set explicitly, `PATH=<sdk>:/usr/bin:/bin`. |
| `local`/`no-sandbox`/`no-remote`/`no-cache` tags | all of the above | **Done.** Only `no-remote-exec` remains (run locally, cache remotely). |
| host C toolchain (`gcc`, `ld`, `as`) | cgo packages; the weave bypasses rules_cc | **Not removed.** Still the host's `/usr/bin` seen read-only through the sandbox. Route: run cgo through the rules_cc toolchain (`CC=` from `cc_toolchain`), task 7. |
| nondeterministic otelc output | map iteration order in setup | **Not removed**: the overlay generator is not byte-stable (see break 8). |

### 3. What I ran in `linux-sandbox` and what broke, in order

Run: `BAZEL_REAL=$(command -v bazelisk) tools/bazel build //:stapler-squad_otel_hermetic --jobs=4 --local_resources=memory=3000 --strategy=OtelWeaveHermetic=linux-sandbox --strategy=OtelModcache=linux-sandbox`. The weave action has no `requires-network`, so linux-sandbox gives it an empty network namespace; `GOPROXY=off` is set as well. Result: `Build completed successfully`, binary `strings | grep -c go.opentelemetry.io/otelc` = 523 (plain build: 0, same as S15), `./binary version` logs `OpenTelemetry initialized ... go.opentelemetry.io/otelc`. Before Bazel, the same recipe ran in `unshare -rn` with an empty HOME and a read-only module cache: exit 0 in 40 s.

1. `otelc setup` cannot run offline against the pristine go.mod: it needs `flock v0.12.1` and `otlptracegrpc v1.39.0`, which the woven build does not (it needs v0.13.0/v1.44.0). Fix: do not run setup in the action (overlay).
2. With the overlay present, setup still runs `go mod tidy`, which needs every module including test-only ones (`go: downloading github.com/bufbuild/buf ...`). Fix: also commit `matched.json` as a template, and skip setup entirely.
3. `matched.json` absolute-path coupling (`cannot replace ... with $WORK/...`, above). Fix: placeholders substituted in the action from `go env GOROOT`, the extracted module cache and the work dir. Setup and build must agree on GOMODCACHE, which they do because both are the same action now.
4. A tree-artifact module cache is symlinked into the sandbox and `//go:embed` rejects symlinks: `cannot embed irregular file otelc-bundle.tgz`, `pattern autocomplete: cannot embed directory ... contains no embeddable files`. Fix: ship the cache as one tarball and extract inside the action (a tree artifact also meant 50,187 symlinks per run).
5. `cannot find module providing package .../instrumentation/otelc/safeexec ... -mod=readonly`: the custom rule package is not in the closure of `:stapler-squad_lib`. Fix: `extra_libs`.
6. `-trimpath` as a CLI flag: `link: fingerprint mismatch: go.opentelemetry.io/otelc/pkg/runtime has ..., import from .../database/sql expecting ...` (otelc's nested hook-package builds do not see CLI flags). Fix: `-trimpath` in `GOFLAGS`.
7. Output differed run to run by 59 bytes (Go and GNU build IDs): the toolexec string was a sandbox path, which cmd/go hashes into action IDs. Fix: `otelc` by name on `PATH`. **Still differs** after that fix (59 bytes, link action ID); cause not isolated (candidates: GOROOT path under the sandbox, cgo environment). Does not affect cache hits, only cross-machine bit-identity.
8. `gen.sh CHECK=1` reports drift on an unchanged tree: otelc setup output is order-nondeterministic (go.mod `replace` order, `otelc.runtime.go` stub numbering, `matched.json` order). go.mod is equal after sorting; the other two are not canonicalized. Not fixed.

### 4. Remote-cacheability (VERIFIED on the local disk cache, same action-cache protocol as a remote)

`bazel build` of the target: 89 s (cold weave, host load ~15). After `rm bazel-bin/stapler-squad_otel_hermetic` and an identical rebuild: `INFO: 2 processes: 3 action cache hit, 1 disk cache hit` in **1.3 s**. The S15 rule re-ran its local action in 47.7 s after the same deletion (`no-cache`).

### 5. Cross-compile (VERIFIED for Windows)

`bazel build //:stapler-squad_otel_hermetic --platforms=@rules_go//go/toolchain:windows_amd64 ...` succeeded in 53 s and produced `PE32+ executable for MS Windows 6.01 (console), x86-64` with 549 otelc strings. Mechanism: the source closure is resolved under the target platform (so `_windows.go` files come with it), and the script sets `GOOS/GOARCH/CGO_ENABLED=0` when `go.mode` differs from the SDK host. Minimal change needed to retire `//:stapler-squad_plain` for Windows: point the Windows release target at this rule and run the Windows parity tests (Epic 6). Darwin is **UNVERIFIED**: the Makefile build uses cgo plus the Info.plist `CGO_LDFLAGS` branch, so a CGO-off woven Darwin build needs its own check (and a macOS runner, or osxcross).

### 6. Cost (VERIFIED, bash `time`, load average 10 to 18 from another worker, so wall times are noisy; CPU-seconds are the stable column)

| Build | wall | user + sys CPU |
|---|---|---|
| plain `go build`, cold private GOCACHE | 40.7 s | 322 s |
| woven toolexec build, cold private GOCACHE, no setup | 41.9 s | 349 s (+8%) |
| plain, 1-line change in `main.go`, warm cache | 7.2 s | 14.5 s |
| woven, same | 5.8 s | 14.8 s |
| S15 rule (Bazel, local, cold otelc and cache) | 59 s | not measured |
| hermetic rule, cold (Bazel, includes sandbox setup, tar extraction of 994 MB) | 89 s | not measured |
| hermetic rule, action-cache hit | 1.3 s | 0 |

This does **not** reproduce `adoption-recommendation.md`'s 1.79x wall / 4.81x CPU: here the weave itself costs about +8% CPU; the earlier ratio likely included the two `otelc setup` passes and `go build -a -x -n` plan, which the overlay removes (UNVERIFIED explanation). The price of hermeticity is the cold GOCACHE: every action miss costs the full ~350 CPU-seconds, where S15's persistent cache made incremental rebuilds ~6 s. CI estimate (INFERRED): in the last 300 first-parent commits on `main`, 212 (71%) touch a weave input (`.go`, `.proto`, web sources, `go.mod/go.sum`, embed staging, `instrumentation/`) and 88 (29%) do not; each of the 29% skips about 350 CPU-seconds per platform job, and every further job in a PR run that needs the binary hits the entry the first one uploaded. The module-cache action is keyed only by go.mod/go.sum/overlay and is a hit almost always.

### 7. Options

| Option | What | Pros | Cons | Cost |
|---|---|---|---|---|
| (a) toolexec compile inside rules_go (`go_binary` with a wrapped `compile`) | Wrap the SDK `compile` with `otelc toolexec`, build everything with rules_go | per-package caching, incremental, best end state | otelc rewrites **std** (`runtime`, `net/http`, `log`, `database/sql`), but rules_go uses a precompiled stdlib (`go.sdk.libs`) and a rules_go `stdlib` rebuild would need forking; `matched.json` absolute paths do not survive per-action sandboxes; hook packages are built by otelc's nested `go build`, outside rules_go's importcfg | XL (fork rules_go stdlib + otelc) |
| (b) weave as source-to-source action, compile hermetic | Emit instrumented files plus a Go `-overlay` JSON, then plain compile | weave output small and cacheable | std rewrites need `-overlay` support, which rules_go does not have; otelc has no "emit only" mode (rewrite and compile are fused in toolexec); compile is still cmd/go in a sandbox, so no cache gain over (e) | L (needs otelc fork) |
| (c) fork otelc, add `--hermetic` | no go.mod mutation, no setup, path-independent matching, deterministic output | removes the overlay-template coupling and nondeterminism | owner-approved fork cost; still need modcache and cold GOCACHE | M |
| (d) keep S15, harden | pin otelc, restrict env | cheap | stays local, no-cache, no remote cache, unsandboxed | S |
| (e) **this prototype**: pre-computed overlay + sandboxed toolexec build | as above | proven today, no fork, remote-cacheable, Windows cross works, `go.mod` untouched | cold GOCACHE per miss, 994 MB cache tarball input, overlay must be regenerated when go.mod/otelc/custom rule change, host cgo toolchain | S to promote, M to harden |

### 8. Recommendation

Promote (e), then spend effort on (c) only if the overlay upkeep proves annoying. Rationale: (e) already satisfies the stated goal (sandboxed, declared inputs, no network in the build, remote-cacheable, cross-compile) at S cost; (a) and (b) are XL/L and need a rules_go stdlib fork. Do the task list below; the first four make (e) shippable, the rest harden it.

### 9. Next tasks (task format: id, cost class, Files, boolean Verify)

- **S15b-1 Promote** (S). Files: `BUILD.bazel`, `tools/bzl/otel_weave.bzl` (delete), `tools/bzl/otel_weave.sh` (delete). Verify: `bazel build //:stapler-squad` exits 0, `strings -a bazel-bin/stapler-squad | grep -c go.opentelemetry.io/otelc` is > 0, the action mnemonic is `OtelWeaveHermetic`, and a second build after deleting the output reports `disk cache hit`.
- **S15b-2 Stamping and embed staging parity** (S). Files: `tools/bzl/otel_weave_hermetic.sh`. Verify: `--stamp --workspace_status_command=tools/bzl/workspace_status.sh` binary prints the same `version` as S15's, and `--//tools/bzl/flags:embed_tmux=true --//tools/bzl/flags:embed_tymux=true` produces a binary larger than the unembedded one (the `go_tags` and `stage` selects were copied but not exercised in this spike).
- **S15b-3 Windows release target on the hermetic rule** (S). Files: root `BUILD.bazel` (Windows cross targets), `tools/bzl/windows/BUILD.bazel`. Verify: the Windows target builds with `--platforms=@rules_go//go/toolchain:windows_amd64` and `file` reports PE32+ with otelc strings, and Epic 6's Windows parity test passes.
- **S15b-4 Module cache from `go_deps`** (M). Files: `MODULE.bazel` (isolated `go_deps` with `from_file(tools/bzl/otel_overlay/go.mod.overlay)`, like `lint_go_deps`), `tools/bzl/otel_weave_hermetic.bzl`, `tools/bzl/otel_weave_hermetic.sh`. Verify: the `OtelModcache` action has no `requires-network` and `bazel build //:stapler-squad` succeeds with the network namespace empty for every action (`--sandbox_default_allow_network=false`); `MODULE.bazel.lock` pins every module hash.
- **S15b-5 Cut the cold-GOCACHE cost** (M). Files: `tools/bzl/otel_weave_hermetic.{bzl,sh}`. Verify: a boolean timing gate, e.g. a 1-line change to `main.go` rebuilds the weave in < 30 s wall (design: a separate action builds all non-`main` packages into a GOCACHE seed keyed only by go.mod, overlay, SDK and otelc, whose tarball seeds the final action).
- **S15b-6 Canonicalize the overlay generator, add the drift check** (S). Files: `tools/bzl/otel_overlay/gen.sh`, a CI step. Verify: `CHECK=1 tools/bzl/otel_overlay/gen.sh` exits 0 on a clean tree and non-zero after editing `go.mod` (needs sorted `replace`, sorted `matched.json`, canonical stub order or a build-equivalence check).
- **S15b-7 cgo through rules_cc** (M). Files: `tools/bzl/otel_weave_hermetic.{bzl,sh}`. Verify: the action's `PATH` no longer contains `/usr/bin` for `gcc`, and `bazel build` succeeds with the host `gcc` hidden (`--sandbox_block_path=/usr/bin/gcc`).
- **S15b-8 Byte-reproducibility** (S). Files: `tools/bzl/otel_weave_hermetic.sh`. Verify: two real executions (output deleted, `--modify_execution_info=OtelWeaveHermetic=+no-cache`) produce `cmp`-identical binaries.
- **S15b-9 ADR-008 starlark cap** (S). Files: `tools/bzl/*.bzl`, `tools/bzl/check-starlark-size.sh` limit. Verify: `tools/bzl/check-starlark-size.sh` exits 0. The cap was already exceeded before this spike (510 > 500 at `9ca6ef7ba`); this branch adds 103 lines (613), promotion removes 64 (549).

### 10. Uncertain / not done

Darwin woven cross-compile; stamped and embedded builds through the hermetic rule; byte-reproducibility; whether the 994 MB tarball (or its 50k-file tree form) is acceptable for a remote cache (the cache stores it once, each cold weave extracts it to disk: ~1.1 GB writes); overlay regeneration cadence (go.mod changes require `gen.sh`, which needs `bazel build` of the generated inputs first). `go_deps`-based modcache (S15b-4) and `go test` under toolexec were not tried. A permission classifier denial did not occur. /tmp was 100% full during the run (other sessions' data, not mine): tool output was lost twice mid-run, so evidence was captured to files under `~/.cache/tmp/ssq-otel-hermetic`.

### 11. Promotion status (2026-11-03, branch `sdd/bazel-otel-promote`, PROVISIONAL)

`//:stapler-squad` is now the hermetic rule; `otel_weave.{bzl,sh}` are deleted; the rule was reshaped so the Starlark carries no path logic (`tools/bzl/otel_weave_hermetic.sh` derives destinations from `@paramfile` arguments). Done: S15b-1 (promote), S15b-2 (stamped and embed builds, no script change needed), S15b-3 (Windows targets are `otel_cross_binary` over `//:stapler-squad`), S15b-6 (canonicalizing `gen.sh`, CI drift step), S15b-8 (root cause: the per-sandbox work path leaked into Go build IDs; fixed with a fixed path under Bazel's hermetic `/tmp`), S15b-9 (starlark surface 499 of 500, cap unchanged). Not done: S15b-4 (`go_deps` module cache), S15b-5 (GOCACHE seed action), S15b-7 (cgo through rules_cc). Each needs new rule logic and the starlark surface has 1 line of headroom, so they need either Starlark moved into the script or a deliberate cap decision (ADR-008). Darwin was not built (no macOS host).


## 8.4 nogo twins

Story 8.4 (owner decision `nogo_story_8_4`): all 17 `tools/lint` analyzers run inside nogo. Twin libraries in `tools/bzl/nogo` reference the real `analyzer.go` files (no copies); `go_sdk.nogo` is enabled in `MODULE.bazel`; `_base.exclude_files` skips `external/`, `bazel-out/`, `tools/lint/`. Results:

- `bazel build //...` exit 0 with nogo enabled, after fixing three real `norawexec` findings the Linux `make lint-custom` run cannot see (`server/services/exec_windows.go` is windows-only; `internal/testenv/testenv_test.go` and `tools/bzl/spike/env_test.go` are new test files): each got a `//nolint:norawexec <reason>`, no behaviour change.
- `bazel test //tools/lint/...`: 17 of 17 analysistest targets execute (each `test.log` shows `--- PASS: TestAnalyzer`, none `SKIP`); 16 were converted here (`data` line, `gotool` import, `gotool.Setup(t)`), `norawgitcli` was converted in S12.
- `make nogo-seed-check`: each of the 17 seeds (`//tools/bzl/nogo/testdata:seed_<analyzer>`, manual) fails the build with its own analyzer's finding.
- Action cost, `bazel aquery --output=summary //...` (657 top-level targets), one run each: **2479 actions without nogo, 4681 with nogo (+2202, +89%)**: `RunNogo` 1076 and `ValidateNogo` 1076 (one pair per Go compile), `GoGenNogo` 2, plus 38 more `GoCompilePkg` (the twins and their `x/tools` deps, 928 to 966). Wall time not compared (host load too noisy; a full `bazel build //...` with warm caches took 110 s with nogo).
- Gazelle: the nogo twins share importpaths with the real analyzers, so the root `BUILD.bazel` carries one `resolve_regexp` that prefers `//tools/lint/$1`. `nosessiondrivertest_test` needs a `# keep` on `deps` (Gazelle adds its embedded library as a dep, which rules_go rejects as a duplicate link).
- Not done: a dual-run of `make lint-custom` and nogo on the seeds. The path-scoped analyzers key on real package paths, which the seeds only stand in for, and both paths compile the same `analyzer.go`. `make lint-custom` was not re-run (needs generated `gen/` and `session/ent`, absent in this worktree); it stays the oracle until the Story 8.6 flip, and now also runs `nogotwins` (twin list, `tools/lint` directories and `cmd/linter` agree).

## S14: selective e2e through observed-dependency slices (Story 0.15) - Fail (2026-11-05)

Branch `sdd/bazel-s14`. Verdict row `S14: Fail` in `spike-results.yaml`; `e2e_selected_share_median` = **1.0** in `spike-measurements.json` (read by `tools/bzl/economic-gate.py`). Evidence is in `spike-data/s14/`; every number below was produced by the tools in `tools/bzl/e2e*` (Go, with tests). Raw per-spec captures (610 MB with line ranges) were scratch and are deleted; the file-level manifest is committed gzipped (`caps-file-level.json.gz`).

### PASS-iff evaluation (Story 0.15 acceptance criteria)

| Criterion | Result | Evidence |
|---|---|---|
| Observed manifest covers every spec | **Met in form, with caveats**: 193 of 193 specs captured (130 passing, 60 failing at this base after one rerun alone each, 3 excluded as `No tests found` for the chromium project: `dev-mode-hmr`, `mobile-webkit`, `website-demos`). 5 specs recorded no executed Go file (`notification-triage-walkthrough`, `notifications-responsive`, `session-auto-approve`, `triage-question-answer`, `triage-related-work`; cause not investigated) and are treated as always selected. | `spike-data/s14/progress.json`, `progress-first-pass.json`, `capture-summary.json` |
| Median selected share <= 0.50, p90 <= 0.90, `__ALL__` fallback <= 0.15 | **NOT MET at every granularity.** Median 1.00 and p90 1.00 for package, file, line and the optimistic line-strict variant, on both windows. `__ALL__` fallback 0.111 (100 commits) and 0.169 (500 commits): the 500-commit rate also exceeds 0.15. | `replay-100.json`, `replay-500.json` (table below) |
| Zero missed failures against a full run | **NOT ESTABLISHED.** 30 seeded mutations (16 Go packages, 6 RPC-handler files, 10 web files) were run as a *sampled* backtest (selected specs plus a seeded sample of the rest, 8 to 11 specs per mutation), not a full-suite run per mutation. 5 mutations failed at least one spec; the one apparent miss (m17, `multi-window.spec.ts`) was re-run 2x2 with both binaries and is a flake (each binary failed once and passed once). No confirmed miss, but the sample is too small to claim zero. | `mutation-summary.json`, `mutations.json` |
| Bazel semantics S-a..S-e | S-a, S-b, S-c, S-d **PASS**; **S-e FAIL** | `semantic.log` |

Verdict mapping: median/p90 above the limits would give `FallbackWorkaround` only with zero misses established; that is not established (sampled backtest), S-e fails, and the FallbackWorkaround path itself (diff-line coverage selection) was measured here and does not narrow selection either (line level median 1.00). Per the PASS-iff rule an unevaluable check is `Fail`, never `Pass`, so the row is **Fail** for the sound-slice design. Owner decision (ADR-009, not taken here): keep e2e as a full run and apply the directive to the Go, Jest and integration layers only, or accept an escape-budgeted diff-line selection whose measured saving is the mean share below. ADR-009's status is untouched.

### Replay (Task 0.15c), `main` tip `415f690c4`, `e2eslice replay-fine`

Relevant = commit touches non-test Go or web source outside `tools/`, `scripts/`, `.claude/`, `.github/`. Shares are per spec (193 specs; always-run specs count as selected).

| Window | Granularity | Relevant | Median | p90 | Mean | Commits with share <= 0.5 | `__ALL__` rate |
|---|---|---|---|---|---|---|---|
| 100 | package | 45 | 1.00 | 1.00 | 0.925 | 3 | 0.111 |
| 100 | file | 45 | 1.00 | 1.00 | 0.910 | 4 | 0.111 |
| 100 | line (sound: hunks outside statement blocks keep every spec that ran the file) | 45 | 1.00 | 1.00 | 0.867 | 6 | 0.111 |
| 100 | line-strict (UNSOUND upper bound on narrowness) | 45 | 1.00 | 1.00 | 0.764 | 11 | 0.111 |
| 500 | package | 325 | 1.00 | 1.00 | 0.924 | 21 | 0.169 |
| 500 | file | 325 | 1.00 | 1.00 | 0.856 | 45 | 0.169 |
| 500 | line | 325 | 1.00 | 1.00 | 0.843 | 50 | 0.169 |
| 500 | line-strict | 325 | 1.00 | 1.00 | 0.717 | 94 | 0.169 |

Why: of 1,240 executed Go files, 870 run in at least 90% of specs and 887 in at least half (server start-up, config, session actors); of 1,361 web files 524 run in at least half. A typical commit touches one such file, so even the executed-line set selects everything. Web capture (Chromium JS coverage resolved through source maps, `tests/e2e/helpers/web-coverage-preload.js`) is equally broad because module top-level code runs on load. Line-level data comes from the HEAD build and is matched to each historical commit's new-side hunk lines, so line results for old commits carry drift error (the file-level and package results do not). Group (Bazel target) shares equal the per-spec shares here (median 1.00).

### Capture (Task 0.15a)

`go build -cover -coverpkg=./...` (45 s, `-p 4`, `nice -n 19`) server; `tools/bzl/e2ebatch` ran one single-worker Playwright invocation per spec through `e2ecapture` (isolated server, ephemeral port, private `ssq-e2e-*` tmux socket, never :8543/:8444), gated on 1-minute load <= 24 and MemAvailable >= 12 GB. Load at spec start: min 2.86, median 10.13, max 23.9 (host 17-70 from other sessions); MemAvailable never below 28.9 GB. Wall time per attempt: median 41 s over all 278 attempts (30 s on the first pass); 3.4 h for the first pass, 6.3 h for both passes (`progress.json`). First pass: 85 of 193 reported Playwright failures; `e2ecapture` itself exits 0 whatever Playwright did, so `e2ebatch` initially misclassified them as passes and a second pass (`-redo-failed`) reran each once, alone: 22 turned green (INFERRED cause: the shared `~/.cache/ms-playwright` browser directory was replaced by another session during the first pass, `chromium_headless_shell-1194` gone at 01:20; the rerun used a private browser path), 60 failed again and are recorded `failed` with both outcomes, 3 `No tests found` (excluded). Failing specs still contribute the coverage they produced, so their slices are partial (an under-approximation, which only makes selection narrower than the truth).

A root-cause note on the tooling: `go tool covdata percent` prints 0.0% for a package with a few executed statements, so `go_pkgs` alone dropped small packages that ran (found when `config` was missing for a spec that executed `config/background_models.go`). `e2ecapture` now adds the package of every executed file and `e2eslice` derives packages from executed files; the package-level replay above uses the corrected data (medians unchanged).

### Mutation backtest (Task 0.15d), sampled

30 mutations in `mutations.json`: negate one single-line `if` on an executed line (`if c {` to `if !(c) {`), 20 Go (16 packages, 6 of them `server/services` handler files) and 10 web (rebuilt dist, rebuilt binary). Per mutation: the line-selected specs (capped at 4), up to 3 file-selected-only specs, 4 seeded random others; a failing spec is rerun once on the mutated binary and, if it fails again, once on the unmutated binary (a spec that also fails there is ignored). Failures observed: m02, m05, m17, m23, m29. Misses at line/file level: 0 confirmed (m17/`multi-window` = flake, re-run 2x2). Limits, stated plainly: only 5 of 30 mutations bit within 8 to 11 specs; the criterion asks for a full-suite run per mutation (about 30 x 3.4 h here); a flake rate of this order (multi-window fails about half its runs on the unmutated binary) would also need a quarantine step before a 0-miss claim means anything. Specs failing at baseline (60) were excluded from the run set.

### Semantic checks (Task 0.15d), throwaway targets `tools/bzl/spike/e2e_slice`

Run with `tools/bzl/e2esemantic` through `tools/bazel` (Bazel 9.3.1): `S-d: PASS` (rdeps over a go_library source label finds the test), `S-a: PASS` (rerun cached; editing `inside.txt` re-ran; editing `outside.txt` stayed cached), `S-b: PASS` (swapping the file behind a constant `--test_env` path stays cached), `S-c: PASS` (a failing target re-executes, not served from cache). **`S-e: FAIL`**: 5 short passing specs, one Playwright invocation 23.6 s versus 97.0 s as five separate invocations, so per-invocation start-up (server boot, seeding, browser launch) is 18.4 s against a limit of 3.5 s (15% of 23.6 s). Measured on direct Playwright invocations at load about 9 (load-affected, but the gap is 5x); the Bazel `e2e_test` wrapper was not run (no real `//tests/e2e` BUILD without the full manifest and Go labels), so the Bazel-side overhead is additional.

### NOT RUN

- A full-suite run per mutation (cost above); the backtest is sampled.
- The Bazel `e2e_test` path for S-e and `check-manifest`/`registry-coverage` on a committed `tests/e2e/slice-manifest.json` (not generated: the verdict is Fail and the manifest would cover 193 specs of which 63 are failing, excluded or empty).
- RPC/route observation as a selector (routes and RPC paths are recorded in the captures; not used for selection).
- Line-level replay with per-commit coverage (coverage is from HEAD only).

### Economic gate hook

`tools/bzl/economic-gate.py` EG5 evaluates the plan's post-selection RA2 projection with `s = e2e_selected_share_median`. It is now measured: `s = 1.0` (`spike-measurements.json`); outputs of the gate and of `check-spike-results.py` are in the branch report.

## S6 worktree cost and Story 2.7 mid-point T1/T3/T4 (2026-11-03) - PARTIAL, NO VERDICT RECORDED

No row for `S6` is written to `spike-results.yaml`: the PASS-iff line cannot be evaluated, so the plan's rule ("a check that cannot be evaluated is Fail, never Pass") applies and the row stays unrecorded (`check-spike-results.py` reports it `MISSING`, which blocks the same way a `Fail` would). Nothing below is a Pass.

### What blocked it (VERIFIED, `/proc/loadavg` samples)
Every timed Bazel measurement is gated on a 1-minute load of 2.0 or below, re-read before the run. The host never reached that for the whole polling window, because other sessions' builds, a Chrome renderer and the live service share it (an unrelated `go compile` at about 280% CPU and a spinning Chrome renderer were both seen in `top`). Samples, local time: 22:36 load 13.7 to 18.3 (right after my own `bazel build` of the three tools); 22:45 2.47; 22:48 4.33; 22:54 5.25; 22:55 to 22:58 4.4 to 6.6; 23:01 4.23; 23:07 5.67; 23:14 11.53. `s6measure cold` (first-worktree cold build, then 5 second-worktree builds) waited from 22:37 until 23:15 and was killed with no build started, so there is no `cold.json`. `economic-midpoint t4` (warm `bazel test //config:all` after an edit of `config/background_models.go`, median of 5) waited 23:17 to 23:57 (loads 3.0 to 10.7, best 2.53) and was stopped with no edit applied (`git status` clean). The wall times of a build started under load above 2.0 would be labelled NOT QUIET and unusable, so none was taken.

### Not run, so no evidence (every S6 clause)
Second-worktree cold build median of 5 versus the make median (56.04 s, `baselines.md`); the first Bazel cold build; 3 concurrent builds peak RSS at most 12 GB; 8 queued invocations plus a terminal-stream p99; the shared disk-cache concurrency test (Task 0.10c); E1 to E6 and E10 (hit ratio, third-worktree spawns, `GoCompile` command-line identity, digest lists, GC, offline repo cache, reflink); E7 and E11 (non-gating); Task 0.10h multiplex inventory (non-gating, skipped). The scripts for Tasks 0.10c to 0.10h (`disk-cache-concurrency.sh`, `cross-worktree-cache.sh`, `cache-layout-checks.sh`, `containment-checks.sh`, `worker-inventory.sh`, `bazel-worktree-cost.sh`) do not exist in the tree; they are Go tools under the owner's scripting rule and were not started.

### Harness that is ready (Bazel-built, `go_test` passes)
`tools/bzl/s6measure` (`cold` and `concurrent` subcommands: worktree per build at varying depth, the wrapper, a quiet-host gate that records load and free memory before and after, `/proc` RSS sampling of the Bazel server trees with a 20 GB host-protection watchdog, `clean --expunge` and `git worktree remove` after each build) and `tools/bzl/economic-midpoint` (`t1`, `t4`, `report`). Run `s6measure cold -repo <wt> -ref <sha> -dir ~/.cache/tmp/ssq-measure/s6 -disk-cache <empty dir> -runs 5` on a quiet host.

### Story 2.7 / Task 2.7a mid-point inputs (INFERRED where noted)
- T1 (measured, not load-sensitive): the 30 sampled partial PRs of `pr-sample.json` replayed through `tools/ci/affected-targets` against the real Bazel graph (`bazel query 'kind(".*_test", rdeps(//..., set(<seeds>)))'`, a path missing from the current tree is replayed as a deletion). Median affected-test ratio 0.0143 (selected tests over the 210 test targets in `//...`); 19 of 30 PRs select at least one test, 2 fall back to `//...` (ratio 1), 11 select none (docs, workflows, non-Bazel inputs map to CI jobs). Limit `min(0.30, 1.5 x 0.2587)` = 0.30, so T1 is PASS. Caveat: the graph is the Epic 1 subset (210 test targets, not the full test set), and RA1's 0.259 counts Go packages, not test targets, so the two ratios differ in kind; the low median is mostly the 11 zero-test PRs. Detail: `tools/bzl/economic-midpoint` `t1` output (per-PR table not committed).
- T3 and T4: NOT RUN (load), see above.

### Economic gate hook
`spike-measurements.json` now holds `make_cold_median_s` = 56.04 and `mid_t1_ratio` = 0.0143. `s6_first_cold_s`, `s6_second_cold_s`, `mid_first_cold_s`, `mid_second_cold_s` and `mid_edit_test_s` are absent, so `economic-gate.py` reports EG3 and T3/T4 UNMEASURED.

## S6 worktree cost and Story 2.7 T3/T4 under load (2026-11-06) - MEASURED UNDER LOAD, OWNER-AUTHORISED; NO S6 VERDICT

The owner authorised measuring on the busy host (`BZL_ALLOW_NOISY_HOST=1` / `-allow-noisy-host`; the default still refuses above a 1-minute load of 2.0). Every number below is NOT a quiet-host number: 1-minute load was 15 to 48 for all runs. Inputs are tagged under `noisy_inputs` in `spike-measurements.json`, and `economic-gate.py` and `check-spike-results.py` print NOISY for anything derived from them. Raw output: `s6measure cold` (`-runs 5`, empty disk cache, `//:stapler-squad`, wrapper with `BAZEL_WRAPPER_SLOTS=3`) and `economic-midpoint t4` (`-runs 5`).

| Run | wall s | load1 pre / post | free mem GB pre / post |
|---|---|---|---|
| first worktree, cold | 222.8 | 26.33 / 43.35 | 38.6 / 33.7 |
| second worktree 1 | 36.4 | 38.99 / 34.03 | 36.8 / 38.9 |
| second worktree 2 | 36.2 | 34.03 / 31.25 | 40.7 / 37.7 |
| second worktree 3 | 33.7 | 30.11 / 29.15 | 40.0 / 38.6 |
| second worktree 4 | 35.3 | 27.77 / 43.50 | 40.0 / 34.7 |
| second worktree 5 | 33.8 | 47.42 / 47.69 | 36.2 / 35.7 |

Second-worktree median 35.3 s. T4 (`bazel test //config:all` after a comment edit of `config/background_models.go`): 3.12, 1.13, 1.13, 1.19, 1.13 s, median 1.13 s at load 15.3 to 15.6; run 1 was disturbed by a `git checkout` of the edited file by the operator mid-run, the median does not depend on it. The file was restored (`git status` clean of it).

### Verdicts the data supports
- T3: 35.3 s <= 0.25 x 222.8 s = 55.7 s, PASS [NOISY]. T4: 1.13 s <= max(1.25 x 2.687, 2.687 + 5) = 7.7 s, PASS [NOISY]. `economic-gate.py --midpoint` exits 0.
- EG3: first cold 222.8 s > 3.0 x 56.04 s = 168 s, TRIP [NOISY]; second cold 35.3 s <= 2.0 x 0.6 x 56.04 s = 67 s. A quiet host would likely be faster, so the first-cold trip is not proof; it needs a quiet-host rerun.
- S6: NO ROW WRITTEN. The first clause (median second-worktree cold build <= 30% of the make median 56.04 s, i.e. 16.8 s) reads 35.3 s here, but a number taken at load 15 to 48 cannot show a Fail either. A Pass is impossible: 3 concurrent builds peak RSS, 8 queued invocations plus the terminal-stream p99, Task 0.10c and E1 to E6/E8 were not run (their tools are still not in the tree). `check-spike-results.py` keeps S6 as MISSING, which blocks like a Fail.
- Peak RSS per single build (sampled, Bazel server tree): 6.2 GB first cold, 2.3 to 3.5 GB second builds; output base after a second build 1.6 GB.
