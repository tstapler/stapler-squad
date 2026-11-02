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
