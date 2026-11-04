# Research: Architecture — Bazel target architecture for stapler-squad

**Date**: 2026-11-01 | **Phase**: 2 (Architecture dimension) | **Complexity**: 4

Confidence labels: VERIFIED = file opened or command run this session; INFERRED = reasoned from verified facts or from general Bazel/rules_go knowledge, not exercised against this repo; UNVERIFIED = needs a spike. Nothing here has been run under Bazel. Section 10 lists the spikes that turn INFERRED into VERIFIED.

## 0. Prior analysis built on (cited, not re-derived)

- `project_plans/ci-speed/research/architecture.md` §2 Pattern C (lines 132-147): a Bazel remote cache replaces Go's `GOCACHE` + pnpm/Next caches with one content-addressed cache; marginal win is narrower than in a polyglot monorepo because per-toolchain caches already work. VERIFIED (read). Still valid for the *CI* driver; it does not address hermeticity or worktree cost, which are two of this project's three co-equal drivers.
- Same file §4c (flagged at line 130): the three hand-maintained regex lists (`build.yml`, `mcp-integration.yml`, `tools/ci/detect-feature-changes.sh`) are a "two sources of truth" correctness risk. This project's affected-target selection (§6c) retires that risk by construction.
- `project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md`: rejected on (1) multi-week appetite, (2) free runners remove RBE, (3) no Gazelle equivalent for the JS half, (4) `b51b60eb1` WORKSPACE scar tissue, (5) cheaper alternatives. Disposition for this project: (1) and (5) are superseded by the owner's three co-equal drivers and XL appetite; (2) still holds, so the plan assumes **no RBE and a weak remote cache** (§7); (3) is the biggest open technical risk (§2.3, §10 spike S3); (4) is addressed by bzlmod-only plus version pinning. The superseding ADR must say so explicitly.
- Other overlapping plans worth reading before Phase 3: `project_plans/web-app-bun-build/research/architecture.md` (the web build whose output Bazel must wrap), `project_plans/tymux-bundled-integration/research/architecture.md` (tymuxd embed), `project_plans/go-auto-instrumentation/research/architecture.md` (otelc variant), `project_plans/go-git-fork-full-git-replacement/research/architecture.md` (go-git replace). Not re-read here; INFERRED relevant from titles and from `Makefile` target names.

## 1. Repo facts that drive the design (all VERIFIED this session unless noted)

| Fact | Evidence |
|---|---|
| 4 live `go.mod` files: root, `tuitest/`, `tools/scanner/`, `tools/lint/` (plus 4 spike modules under `project_plans/.../spikes/S4-S7-src/` that must be excluded) | `find . -name go.mod` |
| Root `go 1.26.6`, `godebug netdns=go`, one replace: `go-git/go-git/v5 => tstapler/go-git/v5 v5.19.3-ssq.1` | `go.mod:3,14,232` |
| `go:embed` sites: `changelog_embed.go` (`CHANGELOG.md` at repo root), `server/web/embed.go` (`all:dist`), `server/analytics/client_metrics.go` (`client_pages.json`), `session/tmux/binary_embedded.go` (`embed/tmux`, tag `embed_tmux`), `session/tymux/binary_embedded.go` (`embed/tymuxd`, tag `embed_tymux`) | `grep go:embed` |
| Build tags in the tree (counts of files): `gitoracle` 15, `integration` 12, `sinkguard` 7, `race`/`!race` 8, `otelcauto` 2, `live_github` 1, `harness` 1, `tools` 1, plus OS tags (`windows` 23, `darwin` 11+4 `darwin && cgo`, `linux` 16, `unix` 10) | `grep '^//go:build'` |
| Generated, gitignored: `gen/` (proto Go+connect), `web-app/src/gen/` (protoc-gen-es TS), `session/ent/*` except `schema/` and `generate.go`; `third_party/tmux` is a submodule with in-tree `.o` build products | `.gitignore:28-38`, `buf.gen.yaml`, `ls third_party/tmux`, `CLAUDE.md` ent section |
| proto: `buf generate proto` with `buf.build/protocolbuffers/go`, `buf.build/connectrpc/go:v1.20.0` (remote plugins, network at gen time) and local `web-app/node_modules/.bin/protoc-gen-es` | `buf.gen.yaml` |
| ent: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` | `Makefile:662-670` |
| Version stamping: `LDFLAGS` sets `main.version` and `$(BUILDINFO_PKG).Version` etc. from `git describe`; macOS embeds `macos/Info.plist` via `CGO_LDFLAGS=-sectcreate` and verifies with `otool` | `Makefile:22-40, 198-215` |
| tmux: `scripts/build-tmux.sh` → `$(BIN_TMUX)` → copied to `session/tmux/embed/tmux`; tymuxd fetched (`fetch-tymuxd`) into `session/tymux/embed/` | `Makefile:384-387, 430-449` |
| Embedded installable build = `build-embedded-tymux` (proto-gen, ent-gen, web dist, tmux embed, tymuxd embed) and is what `install-service` depends on | `Makefile:449, 533` |
| 56 test files use `os.Getwd` or `../../` relative paths | grep count (an upper bound on cwd-dependent tests; includes legitimate uses) |
| sqlite: both `mattn/go-sqlite3` and `modernc.org/sqlite` are in go.mod | grep count = 2 |
| web build is Bun-based (`web-app/build/dev.ts`, `bun.lock`, `bunfig.toml`); `WEB_BUILD=next|bun` | `ls web-app`, CLAUDE.md |

## 2. Target architecture

### 2.1 Package and target layout (Q1)

**Principle**: Gazelle owns every Go BUILD file; humans own only ~10 leaf files whose inputs Gazelle cannot see. Never hand-edit a generated `go_library`.

```
MODULE.bazel            # bzlmod: rules_go, gazelle, rules_proto/protobuf, aspect_rules_js/ts, toolchains
.bazelversion  .bazelrc  .bazelrc.ci  .bazelrc.macos  user.bazelrc (gitignored)
BUILD.bazel             # gazelle target, //:stapler-squad aliases, //:sync-gen
proto/session/v1/BUILD.bazel     # HAND: proto_library + two codegen outputs (§2.2)
session/ent/BUILD.bazel          # HAND: ent genrule + go_library over its output (§2.2)
server/web/BUILD.bazel           # HAND-patched: go_library with embedsrcs = ["//web-app:dist"]
session/tmux/embed/BUILD.bazel   # HAND: tmux build genrule/rule
session/tymux/embed/BUILD.bazel  # HAND: tymuxd http_file -> embed
web-app/BUILD.bazel              # HAND: aspect_rules_js/ts targets (§2.3)
third_party/tmux/BUILD.bazel     # HAND: configure_make or genrule (§2.4)
tools/bzl/ (or build/bazel/)   # HAND: .bzl macros, gazelle extension if needed, platforms, stamping
**/BUILD.bazel                   # GENERATED by gazelle for every other Go package
```

**Gazelle directives** (root `BUILD.bazel`; INFERRED, standard gazelle syntax):

```
# gazelle:prefix github.com/tstapler/stapler-squad
# gazelle:go_naming_convention import_alias
# gazelle:exclude worktrees
# gazelle:exclude .claude
# gazelle:exclude node_modules
# gazelle:exclude web-app/node_modules
# gazelle:exclude third_party/tmux
# gazelle:exclude project_plans
# gazelle:exclude website
# gazelle:exclude build            # build/windows_amd64 output dir; verify contents first
# gazelle:resolve go github.com/tstapler/stapler-squad/gen/proto/go/session/v1 //proto/session/v1:session_go_proto
# gazelle:resolve go github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect //proto/session/v1:session_connect_go
# gazelle:resolve go github.com/tstapler/stapler-squad/session/ent //session/ent:ent
```

Notes:
- `worktrees/` at repo root exists (VERIFIED in `ls`); `.claude/worktrees` is the agent worktree location. Excluding both is mandatory or Gazelle will walk N copies of the repo (and Bazel will treat nested `BUILD` files as packages). Because each worktree is *its own workspace root*, the excludes also keep a worktree's Bazel from seeing sibling worktrees. Add `.bazelignore` with the same paths: `gazelle:exclude` does not stop Bazel itself from globbing into directories. (INFERRED; standard Bazel behavior.)
- Nested modules: Gazelle's go_deps/bzlmod treats a directory with its own `go.mod` as a separate module boundary only for dependency resolution; `gazelle:exclude tuitest tools/scanner tools/lint` is **not** what we want (§2.5) — we want them as separate `go_deps` namespaces or folded into one (§2.5 decision).
- `gazelle:resolve` is the stop-gap for generated packages whose directory has no `.go` source in the tree. Gazelle resolves imports of in-repo packages by scanning source dirs; `gen/proto/go/...` and `session/ent/...` (generated) do not exist at gen time, so without `resolve` lines every importer's `deps` would be unresolved. This is the single most important Gazelle directive in the repo. The alternative (a Gazelle extension) is deferred until the resolve list grows beyond ~5 lines.
- `go_naming_convention import_alias` keeps the target name equal to the directory so `//session/tmux` resolves naturally and rules_go's `importpath` is set from the prefix. INFERRED.

**Visibility policy**: default `# gazelle:default_visibility //:__subpackages__` at root (Gazelle sets it in generated rules), `go_test` implicit. Internal `internal/` dirs rely on Go's own `internal` rule plus rules_go's check. Tighten *only* the leaf targets that carry a security or deploy meaning: `//session/ent` visible to `//session/...`, `//server/...`, `//tools/...` only; `//third_party/tmux:tmux_bin` visible only to `//session/tmux/embed`. Rationale: visibility is cheap to add later and expensive to retrofit across 3,700 files; start permissive-within-repo, restrict the few non-Go leaves. (Recommendation, not evidence-based.)

**Granularity**: one `go_library` + one `go_test` per Go package (Gazelle default). The root `package main` plus its ~25 root-level `.go` files become one `go_binary` `//:stapler-squad`. Root-level `*.test*` binaries (e.g. `adapters.test`, `services.test`) in `ls` are stray build artifacts and must be added to `.bazelignore`/ignored (they are executable files, not sources). VERIFIED they exist in the root dir listing.

### 2.2 Generated, gitignored code: representation (Q3)

Three candidate designs:

| Design | How | Pro | Con for agents in N worktrees |
|---|---|---|---|
| **A. Bazel-only generation** | `genrule`/macro emits `*.pb.go`, `ent/*.go` into `bazel-bin`; `go_library` consumes them. Nothing lands in the source tree | Hermetic; no stale output; matches "no commit generated code" | `go build`, `go vet`, gopls, golangci-lint (`make lint`) and `go run` see no `gen/` or `session/ent` → red squiggles, `go build ./...` fails on a fresh clone. The whole existing tooling (and every agent's muscle memory, and `CLAUDE.md`) assumes `go build` works |
| **B. Sync target** `bazel run //:sync-gen` | Bazel builds the generated files, then a `bazel run` script copies them into `gen/`, `web-app/src/gen/`, `session/ent/` (still gitignored) | `go build`/gopls/lint keep working during and after migration; replaces `make proto-gen ent-gen` with a hermetic producer; same paths the gitignore already covers | Copy step is a drift source: files can be stale vs. the schema until `sync-gen` is rerun; needs a guard |
| **C. Both, with a freshness guard** (recommended) | A is the build of record for `bazel build/test`; B exists for IDE/`go` tooling; a cheap Make/Bazel check compares sync'd tree to `bazel-bin` output (hash) and fails `make ready` if stale | Bazel results never depend on the synced copy (Bazel ignores `gen/` via `.bazelignore` + `gazelle:exclude`), so a stale copy cannot poison Bazel results; humans/gopls keep working | Two producers of the same bytes: one codegen definition (the Bazel action) feeds both, so no logic is duplicated |

**Recommendation: C.** Critical rule: **`gen/`, `web-app/src/gen/` and `session/ent/*.go` must be in `.bazelignore` and `gazelle:exclude`**, so Bazel never reads the synced copy. Otherwise Gazelle would see real packages at `gen/proto/go/...` in some worktrees (synced) and not others (fresh clone), producing non-reproducible BUILD files — the exact "worktrees drift" failure the project exists to remove. (INFERRED, but the reasoning is mechanical.)

Worktree trade-off (Q3): sync-gen writes into each worktree's source tree, so each worktree pays a copy (~MBs, cheap) but **not** the generation (the action is a cache hit from the shared disk cache). Current `make proto-gen` shells to `buf generate` with remote plugins (network, VERIFIED in `buf.gen.yaml`) and a stamp in each worktree, so the Bazel path is strictly cheaper for the Nth worktree. Make `make build`/`test`/`lint` depend on a `sync-gen` wrapper during the strangler phase so non-Bazel paths keep working (§8).

**proto codegen under Bazel**: two realistic options.
1. `rules_proto`/`protobuf` + `rules_go` `go_proto_library` with `compilers = ["@io_bazel_rules_go//proto:go_proto", connect plugin]` and a custom compiler for connect-go v1.20.0 (pinned in `buf.gen.yaml` for the reason in that file's comment), plus a TS action running `protoc-gen-es` from the aspect_rules_js tree. Hermetic, no network, Gazelle has first-class proto support (`gazelle:proto` modes).
2. Run `buf generate` inside a `genrule` with vendored plugin binaries. Simpler to keep the exact `managed` go_package override semantics (`go_package_prefix` in `buf.gen.yaml`), but needs `buf` + plugins as hermetic Bazel tools.

Recommend (1) for Go and a thin rule for TS; the `go_package_prefix` managed mode is the main compatibility risk: generated import paths must remain `.../gen/proto/go/session/v1` for source compatibility, which is why the `gazelle:resolve` lines in §2.1 map that path onto the Bazel target. UNVERIFIED — spike S1.

**ent under Bazel**: ent generation is `go run entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`. Under Bazel: a `go_binary` wrapping `entgo.io/ent/cmd/ent` (from `go_deps`), run by a `genrule`/`rules_go` `go_run`-style action with `session/ent/schema/*.go` as inputs and a declared list of outputs. The output list is not statically known (ent emits one file group per entity), so use a **tree artifact** (`ctx.actions.declare_directory`) in a small Starlark rule and feed it to `go_library(srcs=...)` via `go_library` accepting directory srcs? — rules_go does **not** accept directories in `srcs`. Options: (i) generate a fixed filename list by committing a manifest checked by a test (drift risk, and the manifest is yet another thing agents must update when adding an entity); (ii) a custom rule that globs the output tree and creates `GoSource` providers directly (more Starlark, no manifest) — recommended; (iii) have the generate action produce a zip then unpack to a declared fixed list. Treat as spike S2 (UNVERIFIED). Note `go run -mod=mod` in the Makefile means ent resolves modules at gen time; under Bazel it must come from `go_deps` with no network.

### 2.3 web-app → `server/web/dist` → go:embed (cross-language edge)

Dependency chain, in Bazel terms:

```
proto/session/v1:*.proto
   ├─ (go) //proto/session/v1:session_go_proto  ──► Go packages
   └─ (ts) //proto/session/v1:session_ts_proto  ──► //web-app:srcs  (replaces web-app/src/gen/)
//web-app:build  (aspect_rules_js js_run_binary: bun-built bundle; inputs: srcs, node_modules, gen TS)
   └─► //web-app:dist  (tree artifact)
//server/web:web  go_library(embedsrcs = ["//web-app:dist"], embedcfg handled by rules_go)
```

`server/web/embed.go` uses `//go:embed all:dist` (VERIFIED). rules_go resolves `go:embed` patterns from `embedsrcs` by *label path*; a tree artifact from another package does not appear at `server/web/dist/` in the sandbox. Two options: (a) a copy rule that stages the tree at `server/web/dist/...` inside `bazel-bin/server/web` (`copy_directory` from bazel-lib) and passes that as `embedsrcs` — **no Go code change**, recommended; (b) change the embed path. rules_go's embed handling for directory trees is the known sharp edge (it needs file-level `embedsrcs`, not directories), so expect to enumerate files via a rule that produces a `File` list; this is the same tree-artifact issue as ent. UNVERIFIED — spike S3.

Bun vs rules_js: `aspect_rules_js` consumes a **pnpm lockfile** (`pnpm-lock.yaml`) via `npm_translate_lock`; it does not read `bun.lock` (INFERRED from general knowledge; I did not verify against current docs — the requirements file lists this as an open question and it is a Phase 2 dimension for the stack researcher). The `bun.lock` → pnpm lockfile gap is the most likely forced decision: either (i) generate a pnpm lock as a derived artifact from `package.json` for Bazel only (a second lockfile, drift risk, and conflicts with the "always Bun" repo rule), or (ii) run `bun install` + `bun build` inside a `genrule`/`js_run_binary`-like repository rule with a pinned Bun binary and `--frozen-lockfile`, treating `node_modules` as a repository-rule output (hermetic enough, loses per-package caching granularity). Recommend (ii) for the first cut: the web build is already a single bundle step (`web-app/build/dev.ts`), so per-package granularity buys little, and it honors the Bun rule. Jest tests (`bunx jest`) run as a `sh_test`/`js_test` over the same installed tree. Defer finer-grained rules_ts to a follow-up. UNVERIFIED — spike S4.

### 2.4 tmux and tymuxd embeds

- `third_party/tmux` is a submodule with `.o` files and `autom4te.cache` present in-tree (VERIFIED `ls`). Any Bazel rule over it must (a) treat the submodule as a source tree via `new_local_repository`-style or plain package, (b) **ignore the checked-out `.o`/`config.status` products** (stale in-tree build state would leak into the sandbox — use `gazelle:exclude` + a `.bazelignore` entry and build from a clean copy), (c) build in an isolated `exec_root` copy so autotools writes don't land in the submodule. `rules_foreign_cc` `configure_make` under bzlmod is the natural fit but it is the thing removed in `b51b60eb1`; the failure there was WORKSPACE-mode, so the bzlmod version is a legitimate retry, but fall back to a `genrule` that runs `scripts/build-tmux.sh` equivalent with declared tool inputs if `rules_foreign_cc` + Bazel 9 is rough. Spike S5. Host dependencies (autoconf, libevent, ncurses) are the hermeticity problem: either vendor them via bzlmod (`libevent`, `ncurses` BCR modules, availability UNVERIFIED) or accept host tools in a `local` (non-hermetic) tmux action — flag to the owner as a deliberate hermeticity exception, restricted to one target.
- Output: `//third_party/tmux:tmux_bin` → copied by a staging rule to `session/tmux/embed/tmux` path in `bazel-bin` → `embedsrcs` of `//session/tmux:tmux` when `--config=embed_tmux` (a `select()` on a `config_setting`, §3).
- tymuxd: `http_file`/`http_archive` in `MODULE.bazel` with sha256 (replaces `fetch-tymuxd`), staged to `session/tymux/embed/tymuxd`. Verified fact that tymuxd is "fetched" per Makefile target name; exact URL/provenance not read (UNVERIFIED — check `scripts/fetch-tymuxd*`). A pinned sha256 is a strict hermeticity improvement.

### 2.5 Multiple go.mod modules and the go-git replace (Q4)

| Module | Role | Recommendation |
|---|---|---|
| root | everything | primary `go_deps` source: `go_deps.from_file(go_mod = "//:go.mod")` |
| `tools/lint` | custom analyzers (golang.org/x/tools) | VERIFIED separate module; INFERRED reason = keeps x/tools out of root deps. Add as a second `go_deps.from_file` using a **separate extension tag** (rules_go supports multiple go.mod files in one `go_deps` via `from_file` repeated; names must not collide). Gazelle then generates BUILD for its dir under its own prefix. |
| `tools/scanner` | scanner tool | same as above |
| `tuitest` | TUI test harness | same as above |

Alternative: merge into one module — rejected, out of scope (rewrites application structure). Alternative: `go.work` — rules_go/gazelle's `go_deps` does not read `go.work` as of my knowledge (INFERRED; spike). **Recommendation**: one `go_deps` extension, multiple `from_file(go_mod=...)` tags, and `# gazelle:prefix` overrides per module directory (`tools/lint/BUILD.bazel` sets its own prefix). Cross-module imports (does root import `tools/lint`? — root `go.mod` has no `require` for them per grep, VERIFIED empty) are therefore not an issue.

**go-git replace**: `go_deps` honors `replace` directives in `go.mod` and fetches the replacement via `go_repository`-equivalent (`gazelle` reads `replace` when generating `go_deps`), so `tstapler/go-git/v5 v5.19.3-ssq.1` should resolve from the GOPROXY with the `go.sum` hash. INFERRED; the repo's own `docs/how-to/roll-back-go-git-fork.md` (named in CLAUDE.md, not read) tells us the fork's rollback path is one `go.mod` line, which maps cleanly onto "edit `go.mod`, rerun `bazel mod tidy`". Spike S6 must confirm Gazelle writes the right `use_repo` and that `go_deps` can fetch a replace target on a *private/any* host without `.netrc`. Also confirm `go.sum` has the fork's hash (610 lines VERIFIED present).

### 2.6 sqlite/cgo
Both drivers exist in go.mod (VERIFIED count = 2). cgo with `mattn/go-sqlite3` forces a C toolchain into the build; on macOS that means Xcode CLT + `-sectcreate` Info.plist link flag (VERIFIED in Makefile). **Recommend deferring the choice to the stack dimension but designing for the pure-Go driver**: if the production code can select `modernc.org/sqlite` behind a build tag, `--@io_bazel_rules_go//go/config:pure` + `CGO_ENABLED=0` removes the C toolchain from ~everything except the macOS binary link. But the repo also has `darwin && cgo` files (VERIFIED 4) so macOS keeps cgo regardless. Disposition: **isolate via seam** — don't change driver in this migration; make cgo a per-config setting (`config=macos` enables cgo and `linkopts`), Linux/CI default to the host C toolchain first and revisit `toolchains_llvm` only if cross-machine drift appears.

## 3. Build tags → Bazel configs (Q5)

rules_go reads tags from `--@io_bazel_rules_go//go/config:tags=...` (list flag) and `pure`, `race`, `static`. So each tag combination is a `.bazelrc` `--config` that sets the flag; **tags are global per invocation**, so the "test matrix" is a set of invocations, not targets. (INFERRED from rules_go docs; verify flag names against the pinned rules_go in spike S7.)

```
# .bazelrc (sketch)
build --stamp --workspace_status_command=tools/bzl/stamp.sh
build:embed_tmux  --@io_bazel_rules_go//go/config:tags=embed_tmux  --//build:embed_tmux=true
build:embed_tymux --config=embed_tmux --@io_bazel_rules_go//go/config:tags=embed_tmux,embed_tymux
build:release     --config=embed_tymux --compilation_mode=opt
build:sinkguard   --@io_bazel_rules_go//go/config:tags=sinkguard
build:gitoracle   --@io_bazel_rules_go//go/config:tags=gitoracle
build:integration --@io_bazel_rules_go//go/config:tags=integration --test_tag_filters=-slow
build:race        --@io_bazel_rules_go//go/config:race
build:otel        --@io_bazel_rules_go//go/config:tags=otelcauto  # see below
build:macos       --macos_minimum_os=... --linkopt=-Wl,-sectcreate,__TEXT,__info_plist,macos/Info.plist
build:ci          --disk_cache=... --remote_cache=... --bes_results_url=... --keep_going
```

Caveats:
- **Tag flags are additive-not-composable**: `embed_tymux` must list `embed_tmux` too because `--tags` is one list; use `--config` composition carefully (later flag overrides earlier for non-repeatable flags). Verify in S7.
- **Tag with a non-Go dependency** (`embed_tmux` needs the tmux binary): a tag alone does not wire the embed file. Combine tag + a `select()` over a `config_setting` on a bool `build_setting` so `embedsrcs` includes the staged binary only in that config. Without it, `--config=embed_tmux` compiles then fails on the missing file — the same error the Go source warns about (VERIFIED comment in `binary_embedded.go`), only now at Bazel analysis instead of compile time.
- **otel variant** (**Superseded 2026-11-03 (owner decision, PROVISIONAL, branch `sdd/bazel-otel-default`):** no longer opt-in or left outside Bazel: the one binary is always otelc-woven and `//:stapler-squad` runs the weave as the S15 `otel_weave` action; text below is the original recommendation): `build-otel-auto` used `otelc` compile-time instrumentation (a toolchain wrapper that rewrites the Go build), tags `otelcauto` (2 files). This wraps `go build` itself and has no Bazel analogue; **recommend leaving the otel variant on the Make/`go build` path permanently** (opt-in, per CLAUDE.md "opt-in `stapler-squad-otel` build") and marking it as an explicit non-goal in the cutover criteria. Disposition: extend as-is.
- `live_github`, `harness`, `ignore`, `tools` tags: not part of default test runs (`tools` is the tools.go pattern); map `live_github` to `tags = ["manual"]` tests, never in `//...`.
- **Race**: use `--@io_bazel_rules_go//go/config:race` as `--config=race`. Race changes the cache key for the entire graph; so `race` hits only a race-specific cache, fine.
- **Windows**: 23 `windows`-tagged files and `build/windows_amd64`. Recommend **drop Windows from the Bazel build** (cross-compile via `--platforms` stays possible for `go_binary` since files are pure Go; cgo makes it hard). Owner decision; the gain is sidestepping a toolchain. Keep `GOOS=windows go vet` as a CI lint in the Make path until someone cares. Flag as open question carried to Phase 3.

## 4. Makefile as a thin wrapper (Q6)

Principle: **Make stays the human/agent entrypoint** (names agents already use: `make build`, `make test`, `make ready`, `make install-service`) but bodies become `bazel` calls. Targets that Bazel should not own stay as-is.

| Target class | Examples | Disposition |
|---|---|---|
| Delegates to Bazel | `build`, `stapler-squad`, `build-embedded`, `build-embedded-tymux`, `test`, `test-race`, `test-integration`, `test-oracle`, `test-delivery-guards`, `proto-gen`, `ent-gen` (→ `bazel run //:sync-gen`), `build-tmux`, `build-tmux-embed`, `web-build` | thin wrapper with `BUILD_SYSTEM=make` escape (Risk Control in requirements) |
| Stays outside Bazel | `install-service` orchestration (systemd/launchd, codesign, backup-binary, preflight), `lint-shell`, `proto-lint`/`proto-breaking` (needs git history), `registry-*`, docs, release-please | keep; `install-service` calls `bazel build --config=release` for the binary only |
| Lint gates | `lint` (golangci-lint), `lint-custom` (17 analyzers), dupl, jscpd | **keep as wrapped tools in phase 1** — golangci-lint wants a normal Go tree (`sync-gen` provides it); custom analyzers can later become `go_test`/aspect targets, since `tools/lint` is a Go module (§2.5). jscpd/dupl stay tools |

`BUILD_SYSTEM` switch: `ifeq ($(BUILD_SYSTEM),bazel)` inside each wrapper target, defaulting to `make` until cutover (§8), then flipped, with the old recipes retained one release cycle (requirements Risk Control). The existing stamp-file pattern (`.proto-gen.stamp`, `.ent-gen.stamp`) is retired for Bazel paths because Bazel's own action cache is the source of truth.

**Version stamping**: replace `LDFLAGS := -X main.version=$(VERSION) ...` with `x_defs` on `go_binary` populated from `{STABLE_GIT_VERSION}` via `--workspace_status_command=tools/bzl/stamp.sh` (which reuses the exact `git describe | tr -cd` sanitizer from `Makefile:22`). Use **stable** keys only for version; volatile keys (timestamps) would invalidate caching of every stamped target. Bazel quirk: `--stamp` makes stamped targets rebuild on each commit; keep stamping to the final `go_binary` link only (`x_defs` on `//:stamped_binary`, unstamped `go_library`s) so a new commit relinks only the binary, not recompiles packages. INFERRED but standard.

## 5. Dependency graph (summary)

```
go.mod/go.sum ──go_deps──► @com_github_* (incl. tstapler/go-git replace)
proto/*.proto ──► go_proto ──► //gen-equivalent Go libs ──┐
              └─► ts_proto ───────────────► //web-app:build ─► dist ─► //server/web (embedsrcs)
session/ent/schema ──ent genrule──► //session/ent ───────────────────┤
third_party/tmux ──► tmux_bin ──(config=embed_tmux)──► //session/tmux ─┤
tymuxd http_file ──(config=embed_tymux)──► //session/tymux ──────────┤
CHANGELOG.md, client_pages.json ──► embedsrcs (root pkg, server/analytics)  [see §6 disposition]
                                                                      ▼
                                                           //:stapler-squad (go_binary, x_defs stamp)
```

`go:embed` of `CHANGELOG.md` from root (`changelog_embed.go`, VERIFIED) and the existing indirection `pkg/changelog/embedded.go` that sets it from `package main` (VERIFIED) are already Bazel-friendly: `embedsrcs = ["CHANGELOG.md"]` on the root `go_binary`. No change.

## 6. CI design (Q6 cont.)

### 6a. Setup
`bazel-contrib/setup-bazel` with Bazelisk (reads `.bazelversion`), `disk-cache: true`, `repository-cache: true`, `external-cache: true`; key components: `.bazelversion`, `MODULE.bazel.lock`, runner OS/arch. (Action name and options INFERRED from general knowledge; the repo has `bazel-github-actions-ci` and `bazel-perf` skills available — Phase 3 should read them before writing the workflow.) Commit `MODULE.bazel.lock` (bzlmod lockfile) and run `--lockfile_mode=error` in CI so drift fails loudly. Free runners (2 vCPU) means `--jobs` and `--local_resources` should be capped to avoid OOM during cgo+TS builds.

### 6b. Cache layers on free runners (Q7 partial)
GitHub's actions/cache is slow-ish and size-capped (10 GB/repo, eviction by LRU); a disk cache restored/saved per run works but is coarse (whole-blob save, last-writer-wins on key). Remote-cache options are an owner decision per requirements: (1) BuildBuddy free tier, (2) a `bazel-remote` on the home server reachable over Tailscale (CI runners would need to join Tailscale — adds a secret and a failure mode), (3) GHA-cache shim. For the pilot use disk cache via setup-bazel (zero new infra) and treat remote cache as a measured upgrade, in line with the ci-speed §2 Pattern C conclusion.

### 6c. Affected-target selection (replaces three regex lists)
Two options:
- `bazel query 'rdeps(//..., set(<changed files via git diff --name-only>))'` — built in, no new dependency, needs file→label mapping (`bazel query` accepts file labels as `//pkg:file`; use `--output=label` plus `kind('.*_test', ...)`). Weakness: changed *non-source* files not covered by any target (workflow YAML, docs) and changes to `.bzl`/`MODULE.bazel`/`.bazelrc` (must force "everything").
- `bazel-diff` (Tinder): hashes target graph at base and head, outputs impacted targets; handles MODULE/bzl changes via hashing; adds a JAR/dependency and a two-checkout step.

Recommend starting with `rdeps` plus an explicit "global inputs" list (`MODULE.bazel`, `.bazelrc*`, `.bazelversion`, `tools/bzl/**`, `go.mod`, `go.sum`) that forces `//...`, and move to `bazel-diff` only if the rdeps approach shows misses in the parity soak. Note the Makefile already has `test-affected` (`Makefile:708`, "packages transitively affected … vs BASE") — a Go-package-level analogue, so the idea is not new to the repo (VERIFIED title only; mechanism not read). Non-code gates (docs-only, registry checks, website) can remain `paths-filter` on a tiny list; the point is that the *code* graph stops being hand-maintained. Pitfall: affected selection must treat **e2e (Playwright)** as a target with the binary and web dist as inputs; if e2e is a `sh_test` with `tags=["manual","external"]` it will not be selected by `rdeps` unless named explicitly.

### 6d. Pipeline shape
1. `gazelle-freshness`: `bazel run //:gazelle -- -mode=diff` fails on non-empty diff (BUILD drift gate); also `bazel mod tidy` diff and `sync-gen` freshness check.
2. `build+test` job: `bazel test --config=ci <affected>`; on `main`, `//...` so the cache is populated for PRs.
3. Upload BEP / `--profile` (requirements: observability) as artifacts; trend script akin to ci-speed's.
4. Make-based jobs remain as the **non-Bazel baseline** during strangler phase.

## 7. Cache sharing across N worktrees (Q7)

Facts: Bazel's output base is derived from the workspace path hash, so each worktree gets its own output base (analysis cache, execroot, `bazel-*` symlinks) — requirements Rabbit Holes confirm. What is shared across output bases:

| Mechanism | Shares | Config | Notes |
|---|---|---|---|
| `--disk_cache=~/.cache/bazel-disk` | action outputs (CAS + AC) across **all** workspaces/worktrees | `.bazelrc` `common --disk_cache=` or `user.bazelrc` | The main lever for cold-build cost. Disk-cache has no automatic GC in older Bazel; Bazel 9 supports `--experimental_disk_cache_gc_max_size`/age (INFERRED; verify flag name in spike). Safe for concurrent writers (atomic renames) |
| `--repository_cache=~/.cache/bazel-repo` | downloaded archives (http_archive, go modules by hash) | default is already per-user `~/.cache/bazel/_bazel_$USER/cache/repos`, shared across output bases if same `--output_user_root` | Default likely already shared; setting explicitly is documentation |
| `--output_user_root` | the whole `install` + `output_base` parent | leave default so one Bazel server install dir is shared; **do not** point worktrees at a shared `output_base` | Sharing one output base across worktrees is unsupported (Bazel holds a lock and one server per output base; N concurrent agents would serialize on the lock) |
| `--distdir` | pre-seeded tarballs | useful for offline CI/airgap; low value here | skip |
| `external` repos | third-party sources | `--experimental_repository_cache_hardlinks` can save disk | INFERRED |
| Bazel server memory | each worktree's server is a JVM | N worktrees = N JVMs; set `--max_idle_secs=300`, `startup --host_jvm_args=-Xmx...` | this is a **cost regression vs `go build`** that the worktree driver must measure: the Backlog WIP limit memory note (cap concurrent backlog sessions at 2 after an OOM) shows the host is memory-constrained |

Disk per worktree after cutover: execroot symlink forest plus output tree (bazel-out) are *per worktree* and large (Go binaries, web dist, tmux). Disk cache dedups action outputs but `bazel-out` still hardlinks/copies materialized outputs. Mitigations: `--noexperimental_check_output_files`, `--remote_download_minimal` (when a remote cache is on), periodic `bazel clean --expunge` for dead worktrees folded into the existing `worktree-disk-cleanup` skill (which has uncommitted modifications in git status — leave to its owner). Honest expectation: **disk per worktree might go up, not down, unless the shared disk cache + hardlinks dominate**; this is exactly why Phase 2 must measure baselines (requirements say so) and why the worktree driver can be negative for a worktree that only edits one file. Compare to today: web-app `node_modules` is symlinked (VERIFIED `node_modules@`) and tools/shared-node-modules exists (VERIFIED dir), so the status quo is already partly shared.

## 8. Migration sequencing and parity criteria (Q8)

Stages (strangler; each stage independently revertible):

0. **Foundations (no CI impact)**: `.bazelversion`, `MODULE.bazel`, `.bazelrc`, `.bazelignore`, Gazelle target; spikes S1-S7 (§10). Exit: `bazel build //tools/lint/...` (smallest self-contained module) green on Linux.
1. **Go-only, generated code via sync-gen**: Gazelle BUILD files for everything; proto+ent actions; `bazel build //...` and `bazel test //...` on Linux, in a **non-blocking** CI job, alongside the existing make jobs. web-app and tmux still produced by Make and fed in as pre-built inputs (this is the "Go-only sequencing stage" the requirements mention). Exit: criteria P1-P4 below on Linux.
2. **Embeds**: tmux, tymuxd, web dist as Bazel targets; `--config=release` produces the embedded binary. Exit: P5-P7.
3. **macOS** (arm64+amd64) Bazel build with Info.plist, codesign (§9.5). Exit: P8.
4. **Wrappers**: Make targets switch `BUILD_SYSTEM=bazel` target by target; CI jobs switch from non-blocking to blocking one at a time.
5. **Cutover**: Bazel is the build of record; remove make recipe bodies after one release cycle (requirements Risk Control). Live service moves only after P9.

**Parity criteria** (all must be shown with evidence, run on the same commit):
- P1 **Test-set parity**: the set of Go test names that pass under `go test ./...` (default tags) equals the set under `bazel test //...`, diffed by name; every delta is either fixed or listed in a tracked exclusions file with owner and reason (cwd/HOME/network/tmux sandbox incompatibilities per Rabbit Holes). Verification: `gotestsum --jsonfile` vs Bazel `test.xml` aggregation.
- P2 **Tag-matrix parity**: same for `race`, `integration`, `sinkguard`, `gitoracle` configs.
- P3 **Gazelle idempotence**: a second `gazelle` run produces no diff; fresh clone → `bazel build //...` with no precondition (requirements metric 2).
- P4 **Generated-code equivalence**: bytes of Bazel-generated `*.pb.go`, connect, TS, and ent output equal `make proto-gen ent-gen` output (normalize via `gofmt`); diffs explained (e.g. plugin version).
- P5 **Binary behavior**: `stapler-squad version`/buildinfo output equal; `go version -m` / `strings` check that ldflags (version, commit, branch) are populated; embedded tmux and tymuxd checksums equal the pinned ones.
- P6 **Web parity**: `server/web/dist` file list and content hashes equal (or are explained by a minifier version); Jest pass set equal.
- P7 **E2E**: the Playwright suite passes against the Bazel-built binary (`tests/e2e/helpers/test-server.ts` currently calls `make stapler-squad`, VERIFIED comment at `Makefile:200-202`, so it works unchanged via the wrapper).
- P8 **macOS**: binary signed with the self-signed identity retains TCC grants across rebuild (device check by owner, wall-clock blocker per requirements); `otool -s __TEXT __info_plist` check passes.
- P9 **Deploy**: manual isolated instance per CLAUDE.md "Manual/interactive testing" (ports 62871/62872, private tmux socket) running the Bazel binary; only then `install-service`.
- P10 **Perf**: warm no-op `bazel build //:stapler-squad` in seconds; CI time-to-green and cache hit rate measured vs ci-speed baseline; per-worktree cold-build time and disk vs the Phase 2 baseline. Targets set in Phase 3.
- Soak: N consecutive green days/PRs of the non-blocking job before blocking (N is an owner decision).

## 9. Failure modes specific to the migration (Complexity 4)

| # | Failure mode | Mechanism | Detection | Mitigation |
|---|---|---|---|---|
| 1 | **make and bazel disagree** (partial-state repo) | Make `go build` uses synced/stale `gen/`; Bazel regenerates; a schema change lands, one path green and one red. Or a tag-gated file compiles under one and not the other | P1/P2 diff job on every PR in stage 1-3; a CI job that runs both and compares test sets | Single source for codegen (the Bazel action feeds sync-gen); freshness check; during strangler, `make ready` runs the Bazel freshness gates too |
| 2 | **BUILD file drift** | Agents add/rename a package or import and forget `gazelle`; repo is mostly agent-authored (requirements). Also `gazelle:resolve` list stale after a new generated package | CI `gazelle -mode=diff` gate (6d.1); pre-commit hook; `make ready` runs it | Add a `bazel run //:gazelle` step to a git hook/skill and document in CLAUDE.md; a `bazel`-aware skill in `.claude/skills` (requirements: "guardrail docs/skills required"); make Gazelle failure message say the exact fix command |
| 3 | **Stale generated code in synced tree** | gopls/lint read `gen/` that is older than `proto/` | Freshness hash check in `make lint`/`ready` and an `init` message | `sync-gen` is cheap and cached; make `make build`/`lint` wrappers run it; Bazel never reads the synced tree (§2.2) so correctness of Bazel results is unaffected |
| 4 | **Cutover rollback** | After cutover, a Bazel regression (toolchain, rules bump) blocks releases | `BUILD_SYSTEM=make` escape + old recipes retained one release cycle (requirements) | Keep make recipes executable and *tested weekly by a scheduled workflow* until removed; otherwise the fallback silently rots (the classic failure of "keep the old path" plans); revert is a single PR only if no source change depends on Bazel-only layout, so **ban Bazel-only code changes** (e.g. moving embed files) until after the retention window |
| 5 | **Cache poisoning / non-hermetic actions** | `local` or `no-sandbox` actions (tmux autotools, ent with network, bun install) write nondeterministic output into a shared disk/remote cache; every worktree then consumes it. Free CI with write access from PR branches could poison main's cache | Run a determinism check: build twice in clean output bases, compare output hashes (`bazel build --execution_log_compact_file` diff or `--experimental_remote_cache_...`); reproducibility CI job | Mark non-hermetic actions `tags=["no-cache"]`/`no-remote-cache` until proven deterministic; **CI cache writes only from `main` pushes; PR jobs read-only** (`--remote_upload_local_results=false` on PRs); disk cache keyed to lock/OS/arch; sha256-pin all downloads; scoped credentials in GitHub secrets / 1Password (requirement) |
| 6 | **macOS TCC / codesign** | Info.plist embedding via `-sectcreate`, signature identity, and binary path/`cdhash`; Bazel's output path `bazel-bin/...` or relinking changes the binary identity so TCC grants (documented in `docs/how-to/macos-codesigning.md`, not read) reset, causing permission dialogs (a prior plan `macos-tcc-dialog-reduction` exists) | P8 device check; `otool` assertion as a `genrule` test | Sign as a post-build step *outside* Bazel (in `install-service.sh`/`make`) from the Bazel output copied to a stable path — Bazel outputs are read-only, may be rebuilt with different bytes; ensure deterministic link so cdhash is stable for unchanged source (stamping only at link); retest on both arm64 and amd64; keep darwin out of the first gate (stage 3) |
| 7 | **Live-service deploy via `scripts/install-service.sh`** | Script/Make builds `build-embedded-tymux` and restarts the unit; a Bazel binary with subtly different embeds (missing tymuxd, wrong version stamp) could ship to every in-flight session/backlog item; the existing preflight guards `--tmux-keep-server` (VERIFIED `install-service-preflight`) | P5/P9 gates; post-build `stapler-squad version` and embed checksum assertion inside `install-service` | Switch `install-service` last; wrapper calls `bazel build --config=release` then **copies** to the same artifact path the script already backs up (`backup-binary` target); never use install-service to try changes (CLAUDE.md rule) |
| 8 | **Sandbox-incompatible tests** | 56 test files reference cwd/relative repo paths (grep upper bound); real tmux, git, `$HOME`, network | Run `bazel test` with `--sandbox_default_allow_network=false` on a trial branch; triage list | Disposition below; mark as `local`/`requires-tmux`/`manual` tags rather than hide; per repo policy (fix flaky tests, do not defer) fix root cause where cheap |
| 9 | **Bazel/rules version churn** (ADR-001 scar `b51b60eb1`) | Bazel 9.x, rules_go, gazelle, aspect_rules_js move independently | Renovate/pinned `MODULE.bazel.lock`; scheduled "upgrade canary" job | Pin everything via `bazel_dep(version=...)`; `bazel-version-upgrades` skill exists; document upgrade runbook; record in the superseding ADR as accepted risk |
| 10 | **Agent ergonomics regression** | Agents keep running `go test ./pkg` (fast, familiar) vs `bazel test //pkg:pkg_test`; JVM memory per worktree | n/a | Keep `make test PKG=` wrappers and `go test` working after `sync-gen`; do not break `go` workflows (the argument for design C in §2.2) |
| 11 | **Test timeouts / `size`** | Bazel enforces per-test timeouts by `size`; repo uses `-timeout=20m` for `server/services` (VERIFIED in CLAUDE.md) | failures on first run | set `size = "enormous"`/`timeout` via a Gazelle directive or macro for known-slow packages (`gazelle:map_kind go_test go_test //build:go_test.bzl` to attach defaults repo-wide — this is the "custom Gazelle macro where a pattern repeats" requirement) |
| 12 | **`go vet`/`nilaway`/custom analyzers lost** | Make's `go vet` and NilAway have no Bazel equivalent by default (rules_go runs `nogo` if configured) | parity of lint findings | Phase 1 keep lint on Make path; add `nogo` later using `tools/lint` analyzers (spike) |

An Event-Command-Policy table is not warranted: no multi-actor domain emerged. The migration is a build-graph change; the "actors" (agents, CI, owner) interact only through the files above.

## 10. Tech-debt disposition (as requested) and spikes

| Debt | Disposition | Reason |
|---|---|---|
| `go:embed all:dist` coupling in `server/web/embed.go` | **Isolate via seam**: keep the Go file unchanged; stage `dist` at the expected relative path via a Bazel copy rule (§2.3) | Changing the embed path would be a Bazel-only code change, violating the rollback rule in failure mode 4; if `embedsrcs` of a tree artifact proves impossible, fall back to refactor-first: have `server/web` embed a file list generated by Bazel |
| `changelog_embed.go` at repo root with `pkg/changelog` setter | **Extend as-is** | already Bazel-compatible (§5) |
| `session/tmux/embed/tmux` and `session/tymux/embed/tymuxd` as untracked staging paths | **Isolate via seam**: same stage-at-path trick; keep the `embed_tmux`/`embed_tymux` tags as the switch | |
| cwd-dependent tests (56 candidates) | **Refactor-first, incrementally, only for tests that fail under Bazel**: replace `os.Getwd()+"../../"` with `runfiles`-aware helper behind one `testutil.RepoRoot()` (or use `$TEST_SRCDIR`/`rules_go` `bazel.Runfile`); the existing `envtest/` and `testutil/` packages (VERIFIED dirs) are the home. Do not blanket-refactor | per-test cost is low, and fixing them also helps the `-race`/parallel flake work the repo values (`deterministic-fast-tests`) |
| `stapler-squad.bak-*`, `*.test` binaries, `err.log`, `lint_errors_v*.log`, `server*.log` in repo root | **Isolate**: `.bazelignore`/gitignore; not our cleanup | VERIFIED present; harmless but pollute globs and Gazelle's root-package file set (root `BUILD` must not glob) |
| gitignored `gen/` and `session/ent` | per §2.2 design C | |
| stamp-file Makefile logic | **Replace** inside wrappers | |
| cgo/sqlite dual drivers | **Isolate via seam**, decide in stack research | §2.6 |

**Spikes before Phase 3 commits to the plan** (each should produce a throwaway branch and a recorded result; all UNVERIFIED today):
- S1 proto: connect-go v1.20.0 + `go_package_prefix` import path parity via `go_proto_library`.
- S2 ent: `--feature sql/upsert` generation as a Bazel action + `go_library` over a variable-file output (tree artifact vs custom rule).
- S3 embed: `server/web` `embedsrcs` from a staged `dist` directory tree.
- S4 web: Bun-based `js_run_binary`/genrule with `--frozen-lockfile` vs `aspect_rules_js` + a derived pnpm lock; Jest as a test target.
- S5 tmux: `rules_foreign_cc` under bzlmod + Bazel 9 vs `genrule`; hermetic libevent/ncurses.
- S6 go_deps: `from_file` for four modules + go-git replace; `MODULE.bazel.lock` stability.
- S7 flags: confirm rules_go flag names (`go/config:tags`, `race`, `pure`) on the pinned version and that `--config` composition works for tag lists.
- S8 baseline: measure make-path cold build, `go test ./...` wall time, CI time-to-green, per-worktree disk (requirements: Phase 2 must measure baselines). Not done here.

## 11. Assumptions and gaps

- Everything in §2-§9 describing Bazel behavior is INFERRED from general rules_go/Gazelle/Bazel knowledge as of my cutoff, not from running Bazel 9.3.1 against this repo; Bazel 9 changes (WORKSPACE removal, rules default changes) may invalidate individual flag names.
- Not read: `scripts/build-tmux.sh`, `scripts/fetch-tymuxd*`, `scripts/install-service.sh`, `docs/how-to/macos-codesigning.md`, `docs/how-to/roll-back-go-git-fork.md`, the three CI regex lists, the 18 workflow files, `tools/ci/detect-feature-changes.sh`. The CI design (§6) therefore does not enumerate which jobs map to which Bazel invocation; Phase 3 must.
- The Windows decision, remote cache provider, and sqlite driver are owner/stack-dimension decisions flagged, not made.
- The `56` cwd-test count is a grep upper bound (`os.Getwd` or `../../` in `_test.go`, excluding node_modules/worktrees/project_plans), not a count of tests that will break.
