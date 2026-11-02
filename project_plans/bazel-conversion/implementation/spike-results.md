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
