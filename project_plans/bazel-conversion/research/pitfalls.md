# Research: Pitfalls — Bazel 9 + bzlmod + rules_go/Gazelle in this repo

**Date**: 2026-11-01. Builds on `project_plans/ci-speed/research/pitfalls.md` §3 (generic Bazel/bzlmod pitfalls; not repeated here) and `project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md`. Labels: VERIFIED = command run this session; INFERRED = reasoned, needs the spike; EXTERNAL = from a web source.

## 0. Prior-art facts

- `b51b60eb1` (2026-07-15) removed the old tmux Bazel path because `rules_foreign_cc` was WORKSPACE-mode and WORKSPACE is gone in Bazel 9. It was never exercised in CI (`build.yml` called `scripts/build-tmux.sh`). The same commit fixed an unrelated bug: a hard Make prerequisite on `third_party/tmux/configure.ac` broke fresh worktrees because `git worktree add` doesn't init submodules. VERIFIED (`git show b51b60eb1 --stat`). The submodule-empty-in-worktree problem carries straight over to Bazel (see R7).
- ADR-001's "3-6 week appetite" and "free-runner removes the lever" reasons are not repeated here; its still-live technical claims (ent/proto need custom Gazelle handling, tmux rewire, JS lockfile sharp edges) are re-tested below.
- Searches for Bazel-9-specific rules_go/Gazelle/rules_js bun issues returned only general docs. EXTERNAL: [rules_go bzlmod docs](https://github.com/bazel-contrib/rules_go/blob/master/docs/go/core/bzlmod.md) (go_deps reads `go.mod` incl. `replace`, has `go_deps.gazelle_override`; cgo needs a registered C toolchain; cross-compiling cgo needs a `_cgo` platform suffix). aspect_rules_js is pnpm-layout based (npm/yarn lockfiles via `npm_translate_lock`); no first-party `bun.lock` support was found in the search, only a community [rules_bun](https://github.com/levonk/rules_bun) using the old `bun.lockb` (searched 2026-11-01; UNVERIFIED against rules_js release notes, which the spike must read).

## 1. Repo facts gathered (all VERIFIED 2026-11-01)

| Fact | Evidence |
|---|---|
| Generated code gitignored: `gen/`, `web-app/src/gen/`, `web/src/gen/`, `session/ent/*.go`, `session/ent/*/` | `.gitignore` lines 35-46 |
| go-git replace: `github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.1` | `go.mod:232` |
| Both `mattn/go-sqlite3 v1.14.40` (cgo) and `modernc.org/sqlite v1.56.0` are in go.mod | `go.mod:38,68` |
| Real cgo files: only 3, all darwin (`infoplist_darwin.go`, `session/tmux/socket_owner_darwin.go`, `session/procinfo/openfiles_darwin.go`), plus go-sqlite3 transitively | `grep 'import "C"'` |
| Go modules: root, `tuitest/`, `tools/scanner/`, `tools/lint/` (+ spike go.mod files under `project_plans/` that Gazelle will walk) | `find -name go.mod` |
| `go:embed` sites: `CHANGELOG.md` (repo root), `server/web` `all:dist`, `server/analytics/client_pages.json`, `session/tmux/embed/tmux`, `session/tymux/embed/tymuxd` | `grep go:embed` |
| Build tags in use (counts): `!windows` 41, `windows` 23, `linux` 16, `gitoracle` 15, `integration` 12, `darwin` 11, `sinkguard` 7, `!race` 7, `darwin && cgo` 4, `otelcauto` 2, `embed_tmux`/`embed_tymux` (Makefile) | grep of `go:build` |
| Test-env coupling: 38 test files use `UserHomeDir`/`HOME`/`Getwd`; 106 reference `"tmux"`/`"git"`; 104 use `net.Listen`/`httptest`; 20 use `runtime.Caller`; 67 use `Getwd` or `../../` paths | greps |
| tmux submodule: 142 `*.o` present in working tree, 0 git-tracked (`*.o` is in the submodule's own `.gitignore`); gitlink pinned `7f2a35ad3321`. Stale in-tree objects are a local artifact, not committed | `git ls-files`, `ls third_party/tmux/*.o` |
| This machine already has `~/.cache/bazel` = 14 GB (8.4 GB `cache`, one output base 2.9 GB), 107 git worktrees registered, `/home` 67% of 1.9 TB, 61 GB RAM (34 used) | `du`, `git worktree list`, `free` |
| Version stamp: `git describe --tags --always --dirty` into ldflags `-X $(BUILDINFO_PKG).Version`; also GoReleaser, release-please config | `Makefile:22,40`, `.goreleaser.yaml` |
| `web-app/bun.lock` exists (1,910 lines); no root lockfile | `ls` |
| `tools/lint` has 16 analyzers + own `go.mod`; `make lint` runs `golangci-lint` and `lint-custom` | `ls tools/lint`, `Makefile:932` |

## 2. Ranked pitfalls (likelihood x impact; 5 = worst)

| # | Risk | L | I | Score | Live here? |
|---|---|---|---|---|---|
| R1 | Test sandbox failures at scale (cwd, `$HOME`, real tmux/git, ports, absolute paths, `runtime.Caller`) | 5 | 4 | 20 | Yes, quantified above |
| R2 | Unix-socket path length under Bazel execroot/TEST_TMPDIR | 4 | 3 | 12 | Yes, computed |
| R3 | Gazelle vs gitignored generated code (ent, proto, TS gen) | 5 | 4 | 20 | Yes |
| R4 | Web: bun.lock vs rules_js; `go:embed all:dist` edge | 4 | 5 | 20 | Yes |
| R5 | Worktree disk/inode blowup (output base per path) | 4 | 4 | 16 | Yes, already incident-prone |
| R6 | cgo sqlite + hermetic C toolchain (macOS/Linux) | 4 | 3 | 12 | Yes, but narrow |
| R7 | tmux autotools build + empty submodule in worktrees | 3 | 3 | 9 | Yes |
| R8 | Lint/custom analyzers are not hermetic (go/packages) | 4 | 2 | 8 | Yes |
| R9 | Gazelle/BUILD and MODULE.bazel.lock merge conflicts across parallel agent PRs | 5 | 2 | 10 | Yes (agent-authored repo) |
| R10 | macOS signing/TCC persistence of Bazel-built binary | 3 | 4 | 12 | Yes |
| R11 | Version stamping / release-please / GoReleaser parity | 3 | 2 | 6 | Yes |
| R12 | Bazel server memory and CPU vs the repo's OOM history | 3 | 4 | 12 | Yes |
| R13 | go_deps `replace` + separate modules | 2 | 3 | 6 | Mostly fine |
| R14 | Race detector, build tags test matrix | 2 | 2 | 4 | Manageable |
| R15 | Contributor/agent ergonomics | 4 | 3 | 12 | Yes |

### R1. Test sandbox failures (score 20)
Go tests under Bazel run with cwd = the package's runfiles dir, read-only inputs, a private `TEST_TMPDIR`, and `HOME` unset or unreliable. Of the ~3,700 Go files, many tests assume repo-root-relative paths (67 test files with `Getwd`/`../../`), a real `HOME` (38), a real `tmux`/`git` on PATH (106), and loopback listeners (104; fine, loopback works in the Linux sandbox, but fixed ports collide when tests run in parallel). 20 use `runtime.Caller` to find the source tree, which resolves to a path that doesn't exist in runfiles. INFERRED impact; counts VERIFIED.
Mitigation / spike (<=1 day): run Gazelle on 10 representative packages (`session`, `session/tmux`, `server/services`, `session/vc`, `config`, `github`, `session/mux`, `session/tymux`, `tuitest`, one pure-logic package), run `bazel test --sandbox_debug`, and tabulate failure classes. Then fix once at the source with test helpers (repo-root resolver using `TEST_SRCDIR`/`RUNFILES_DIR` via `rules_go`'s `runfiles` package, `t.Setenv("HOME", t.TempDir())`), rather than per test. Tag genuinely environment-bound tests (`requires-network`, `local`, `no-sandbox`) as a stopgap, with the repo's fix-don't-defer rule requiring a tracking item.

### R2. Unix-socket path limit (score 12)
`sun_path` is 108 bytes on Linux, 104 on macOS. Matches the repo's own scar (`instinct_full_disk_tmp_gotmpdir_socket_path`: long `GOTMPDIR` broke `session/mux`, `session/tymux` socket tests). A placeholder path of the shape `~/.cache/bazel/_bazel_tstapler/<32hex>/sandbox/linux-sandbox/123/execroot/_main/_tmp/<32hex>` is 155 chars (+ `/ssq-1234.sock` = 169); the macOS shape `/private/var/tmp/_bazel_tyler/<hash>/sandbox/darwin-sandbox/123/execroot/_main/_tmp/<hash>/x.sock` is 149. All exceed the limit. VERIFIED arithmetic with `wc -c`; real layout INFERRED (Bazel sets `TEST_TMPDIR`/`TMPDIR` inside the sandbox/test dir).
Mitigation: in `.bazelrc` `test --test_env=TMPDIR=/tmp` (or a short dir) plus a short-private-`/tmp` mechanism (`--incompatible_sandbox_hermetic_tmp` does NOT exist on 9.3.1, VERIFIED by run 2026-11-01; see plan Task 0.9b); also `--output_user_root=/var/tmp/b` shortens the base. Spike (2 h): one `go_test` that binds a socket in `os.TempDir()`, run under linux-sandbox and, if a Mac is available, darwin-sandbox.

### R3. Gazelle vs gitignored generated code (score 20)
Gazelle scans the source tree; `session/ent/*.go`, `gen/**`, and `web-app/src/gen/**` don't exist on a fresh clone, so any importing package fails Gazelle's import resolution (it would try to resolve `stapler-squad/session/ent` to an external dep or leave it unresolved). The `ent` generator needs `--feature sql/upsert` (CLAUDE.md). Also proto output imports (`gen/session/v1`) need to resolve to `go_proto_library`-style targets.
Mitigation: commit a hand-written `BUILD.bazel` in `session/ent/` and proto dirs with `# gazelle:exclude` for generated files plus `# gazelle:resolve go <importpath> //session/ent:ent` directives; ent as a `genrule`/custom rule running the pinned `ent` binary (`go_binary` from go.mod via go_deps) against `session/ent/schema`. Or use rules_go's `go_proto_library` for proto with a connect-go plugin (`protoc-gen-connect-go` as a `go_binary`). Spike (1 day): get `//session/ent:ent` and `//gen/session/v1:..` building from a clean clone with Gazelle producing zero diffs; this is the gating spike for the whole project.

### R4. Web build and embed edge (score 20)
(a) `aspect_rules_js` is pnpm-layout based; `web-app/bun.lock` has no confirmed first-party path (see §0), so expect either generating a `pnpm-lock.yaml` in CI from bun resolution (two lockfiles, drift risk) or running `bun install` as a repository rule/`genrule` (non-hermetic, defeats caching of node_modules). (b) `//go:embed all:dist` in `server/web/embed.go` requires `server/web/dist` to be in the package's `embedsrcs`; a tree artifact (directory output of a web build) cannot be listed as `embedsrcs` file-by-file because rules_go needs file labels at analysis time. Known workaround is to emit a declared set of files or a custom rule that expands the tree; either way `dist` filenames (hashed chunks) are unknown at analysis time. INFERRED from rules_go's embedsrcs semantics; needs the spike to confirm.
Mitigation: spike (1 day) with two tracks: (i) `rules_js` + a pnpm lock produced from bun; (ii) fallback "web as a single opaque action" (`genrule` running `bun install --frozen-lockfile && bun run build` with `local` tag, outputs a tar/zip) and feed the Go embed through a small custom rule that unpacks to a fixed directory and passes a declared file list (`all:dist` can be replaced by embedding a single `dist.tar` plus runtime extract/`fs.Sub`, a small app change that avoids the tree-artifact problem entirely). Pick (ii) if (i) isn't clean; decision belongs in Phase 3.

### R5. Worktree disk/inode blowup (score 16)
Bazel keys the output base on the workspace path, so 107 worktrees imply up to 107 output bases plus 107 `bazel-*` symlink farms and 107 Bazel server JVMs if each is used. The machine already holds 14 GB under `~/.cache/bazel` from past experimentation (VERIFIED); `.claude/skills/worktree-disk-cleanup/SKILL.md:17` records a 2026-09-11 sweep of 148 worktrees eating 178 GB (VERIFIED). Inodes: `/tmp` tmpfs has 8.1 M inodes with 20 k used (VERIFIED); `/home` is a filesystem that reports no inode counts (btrfs-style), so inode exhaustion is not the live risk, disk bytes are. Sandbox symlink trees add inode churn on ext4/xfs CI runners (INFERRED).
Mitigation: user-level `~/.bazelrc` with a shared `--disk_cache=~/.cache/bazel-disk` and `--repository_cache`, shared `--output_user_root` and `--experimental_disk_cache_gc_max_size` (verify flag name on 9.x), `--noexperimental_check_output_files`; extend `worktree-disk-cleanup` to run `bazel clean --expunge` / delete output bases for removed worktrees (`bazel info output_base` per path). Alternative: point all worktrees at one output base is unsupported (one server per workspace). Spike (0.5 day): create 5 worktrees, build the same target, record disk per worktree with a shared disk cache.

### R6. cgo sqlite and C toolchains (score 12)
Only go-sqlite3 needs a C compiler on Linux; the three `import "C"` files are darwin-only. A host C toolchain (default `rules_cc` autodetect) is non-hermetic: the produced binary links the host libc, so a worktree on Manjaro and CI on Ubuntu get different glibc baselines. Hermetic options (`toolchains_llvm`, zig-cc) are viable on Linux but darwin cgo needs the macOS SDK, which Bazel can't fetch hermetically. EXTERNAL: rules_go disables cgo when cross-compiling unless a `_cgo` platform and C toolchain are registered. `modernc.org/sqlite` is already in go.mod, so switching removes most cgo from the Linux path (not verified whether code uses both; check imports before deciding).
Mitigation: decision spike (0.5 day): `grep` which drivers are imported, build with `CGO_ENABLED=0` on modernc, compare test pass set and a benchmark. Keep host toolchain on macOS (`darwin` is arm64/amd64 native build; cross-compiling darwin from Linux is out of scope).

### R7. tmux autotools via rules_foreign_cc (score 9)
`rules_foreign_cc` is bzlmod-capable now (versions must be pinned and checked against Bazel 9; UNVERIFIED), but the submodule's tree contains 142 stale `.o` files that are untracked/ignored in the submodule yet will be globbed into any `filegroup`/source copy, poisoning an in-tree `./configure && make` (stale objects from a different host). Worktrees start with an empty `third_party/tmux` (b51b60eb1). Use out-of-tree build: a `genrule` or `configure_make` with `out_of_tree`/copy of `*.c|*.h|configure|*.am|compat/**` only, or fetch tmux as an `http_archive` pinned to the same tag instead of the submodule (also fixes worktree empties; gitlink `7f2a35ad3321` vs tag 3.8 must be reconciled). Requires `libevent`, `ncurses` (hermetic via `http_archive` or host-provided; host-provided breaks hermeticity). Spike (1 day): `http_archive` tmux + `configure_make` (or genrule) producing `tmux` and the `embed_tmux` go:embed file, on Linux first.

### R8. Lint and custom analyzers (score 8)
golangci-lint and the 16 analyzers in `tools/lint` use `go/packages`, which shells out to `go list` and needs a populated module cache and network-reachable GOPATH; under Bazel sandbox that breaks unless run via `rules_go`'s `nogo` (analyzers as Go libraries linked into the compile action, runs per-package, cached) or as non-hermetic `local` wrapped tools. `nogo` runs only `analysis.Analyzer`s; the repo's analyzers are in a separate module (`tools/lint/go.mod`), which needs go_deps to see it as a dependency or a `local_path_override`. golangci-lint plugins (dupl, gocyclo, revive) stay wrapped (INFERRED). Recommendation: keep `make lint` as a wrapper outside the sandbox initially; port the in-repo analyzers to `nogo` later as a separate story.

### R9. Merge conflicts: BUILD files and `MODULE.bazel.lock` (score 10)
Each parallel agent PR adding a Go import or file touches `BUILD.bazel` in that package (conflicts only when two PRs touch the same package) and any dependency change touches `MODULE.bazel.lock` (a single file; high conflict rate; the lock is line-based JSON that merges badly). INFERRED. Mitigation: CI gate "gazelle produces no diff" plus a `bazel mod deps --lockfile_mode=update` regeneration instruction in the conflict-resolution doc; consider `--lockfile_mode=off` or not committing the lock (trade: reproducibility), or a merge driver that regenerates. Spike (2 h): simulate two branches adding different deps and measure conflict.

### R10. macOS signing / TCC (score 12)
CLAUDE.md requires a stable self-signed identity so TCC grants persist (`make setup-codesign`, `_codesign-binary`; `scripts/install-service.sh` verifies designated requirements). Bazel outputs are read-only, may be rebuilt without re-signing, and macOS invalidates the signature if the binary is copied with xattrs stripped incorrectly. Keep signing as a post-build step in `install-service` (copy out of `bazel-bin`, then `codesign`), not a Bazel action. Verify with `codesign --verify` and a TCC prompt check on the Mac (owner-gated device run).

### R11. Version stamping and releases (score 6)
`git describe --dirty` is used for ldflags; Bazel needs `--workspace_status_command` and `x_defs` with `{STABLE_...}` stamping and `--stamp` (stamped builds defeat caching of dependents, so stamp only the final binary). GoReleaser keeps building with `go build` unless replaced; release-please only edits version/CHANGELOG files, which are `go:embed` inputs (`CHANGELOG.md`), so a BUILD file edit is not needed. Low risk.

### R12. Bazel server memory (score 12)
Repo memory index records an OOM incident (2026-07-29 session leak; 2026-07-12 backlog OOM → WIP cap of 2). A Bazel server is a JVM per output base (typically 1-3 GB heap on a repo this size, INFERRED) idle-timing out after 3 h; parallel agent worktrees multiply this. Machine has 61 GB with 34 GB used (VERIFIED). Mitigation: `startup --max_idle_secs=300`, `startup --host_jvm_args=-Xmx2g`, `build --jobs` cap, and a documented rule that agents run at most N concurrent bazel invocations; consider `--noshutdown_on_low_sys_mem`. Spike: measure RSS of server + local actions for 3 concurrent builds.

### R13. go_deps, `replace`, separate modules (score 6)
EXTERNAL: go_deps honors `replace` in the root `go.mod` and offers `gazelle_override`. The go-git fork replace is a plain module-path replace to a public repo, the supported case. Separate modules (`tuitest`, `tools/scanner`, `tools/lint`) need either a `go.work` (supported per rules_go docs) or separate `go_deps.from_file` tags; each needs `use_repo` listing, and the duplicated third-party versions across modules need MVS alignment. Gazelle will also descend into `project_plans/**/spikes/*/go.mod` (4 stray modules, VERIFIED) and `.claude/worktrees`, `third_party/tmux`; add `# gazelle:exclude` and `.bazelignore` (also for `web-app/node_modules`, `worktrees`).

### R14. Race and tags (score 4)
`go_test` supports `race = "on"`/`--@io_bazel_rules_go//go/config:race`; `gotags` via `--@io_bazel_rules_go//go/config:tags=...` is global, not per-target, so `integration`/`sinkguard`/`gitoracle` need separate `config_setting`s/transitions and a `.bazelrc` config each (cache for tagged variants is separate). `!race`-tagged files (7) flip with the flag. Gazelle evaluates tags with `select`, so no regeneration per tag. Build tag `ignore` (3 files) and `tools` must be excluded from `//...`.

### R15. Agent / contributor ergonomics (score 12)
Agents default to `go build`, `go test ./...`, and `go vet`; after Bazel is the build of record these will silently work on stale-or-missing generated code or skip BUILD updates. Mitigation: a CLAUDE.md section plus a project skill (`bazel-*` skills already exist globally: `bazel-perf`, `bazel-version-upgrades`, `bazel-github-actions-ci`), `make` wrappers that call Bazel, a pre-commit gazelle hook, and keeping `go build ./...` working (via a `go.work`-free generate step) during transition so IDEs/gopls keep functioning. gopls needs `GOPACKAGESDRIVER` or the generated code present, which is a real DX regression unless `make gen` still materialises the gitignored files.

## 3. Early spikes, ordered (each <= 1 day of agent work)

1. **S1 Gazelle + generated code (R3)**: clean clone, `bazel build //session/... //gen/...` with zero Gazelle diff. Gate for everything.
2. **S2 Sandbox test census (R1, R2)**: 10 packages, failure-class table, socket-path test.
3. **S3 Web (R4)**: rules_js with lockfile conversion vs opaque genrule + tar embed; decide.
4. **S4 cgo/driver decision (R6)**: modernc vs mattn on Linux; darwin host toolchain.
5. **S5 tmux (R7)**: `http_archive` + `configure_make` vs genrule; reconcile pin.
6. **S6 Worktree cost (R5, R12)**: 5 worktrees, shared disk cache, RSS and disk per worktree.
7. **S7 Lock conflict (R9)**: two-branch merge simulation.

## 4. Sources
- ci-speed pitfalls §3 and ADR-001 (paths above); `git show b51b60eb1`.
- [rules_go bzlmod docs](https://github.com/bazel-contrib/rules_go/blob/master/docs/go/core/bzlmod.md), searched 2026-11-01.
- [levonk/rules_bun](https://github.com/levonk/rules_bun), [aspect-build/rules_js overview](https://docsearch.algolia.com/mcp/docs/repo/aspect-build/rules_js), searched 2026-11-01.
- Repo evidence: `.gitignore`, `go.mod`, `.gitmodules`, `Makefile`, `.claude/skills/worktree-disk-cleanup/SKILL.md`, memory notes `instinct_full_disk_tmp_gotmpdir_socket_path.md`.
- Gaps: no Bazel-9-specific bug reports were located; Bazel flag names marked "verify" were not run against 9.3.1.
