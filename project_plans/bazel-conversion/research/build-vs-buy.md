# Build vs. Buy: bazel-conversion (SDD Phase 2)

**Date**: 2026-11-01. **Inputs**: `../requirements.md`, `project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md`, `project_plans/ci-speed/research/build-vs-buy.md`.
**Method and limits**: web searches only (no repo experiments). Labels: VERIFIED = a fetched/search-surfaced source says it; INFERRED = my reasoning. Search-result summaries were not all opened at the primary source; items marked "check" need a Phase 3 spike before they are relied on. No baseline numbers were measured here.

## 0. Frame: what ADR-001 got right and what no longer applies

- Still true: ent output is gitignored and buf output needs custom wiring; the free-runner constraint removes remote *execution*; agents must learn Bazel (ADR-001 Evidence).
- No longer applies: the 3-6 week appetite cap (requirements.md Size: XL, full scope default), and the "CI speed only" driver. Hermeticity and worktree cost are co-equal drivers, and these are exactly where Bazel's disk cache and pinned toolchains win without any remote service.
- ADR-001 cited pnpm for the web-app; the repo is now Bun (`bun.lock`), which makes the rules_js lockfile question harder (section 1.3).

## 1. Existing OSS

### 1.1 Go: rules_go + Gazelle (stock) vs custom extensions
- **Stock rules_go + Gazelle** - Recommended as the Go backbone. `go_deps` reads `go.mod` and honors `replace` directives (Gazelle 0.30.0 release, "support go.mod replace directives": https://www.github.com/bazelbuild/bazel-gazelle/releases/tag/v0.30.0). `go_deps` can also read `go.work` for multiple modules, but every workspace module's direct deps must be listed with `use_repo` (https://github.com/bazel-contrib/rules_go/blob/master/docs/go/core/bzlmod.md).
  - Pros: battle-tested, generates ~3.7k files' worth of BUILD files, `# gazelle:` directives cover build tags, excludes and resolves.
  - Cons: the `tstapler/go-git` replace points at a fork; whether `go_deps` resolves a replace to a different module path cleanly is UNVERIFIED (search found no Bazel-9-specific evidence) and is a Phase 3 spike item. The separate modules (`tuitest`, `tools/scanner`, `tools/lint`) imply either `go.work` or a separate `go_deps.from_file` per module; the doc above says `go.work` is supported, so prefer that.
  - Bazel 9 compatibility: rules_go 0.58.3 was reported still incompatible with Bazel 9 and a downstream project pinned rules_go 0.60.0 (Riegeli commit via https://pigweed.googlesource.com/third_party/github/google/riegeli/+/86ff59bfec52a05efda32d3b0820781b6aa6146d). Pin rules_go >= 0.60 and use the `bazel-version-upgrades` skill.
- **Custom Gazelle extension for ent** - Viable but only if the `# gazelle:` directive route fails. Prior art is thin: `cloneable/rules_ent` is self-described "WIP; still very hacky" (https://www.github.com/cloneable/rules_ent), and a bazel-go-discuss thread reports that ent's subdirectory output works if each subpackage is its own `go_library` (https://groups.google.com/g/bazel-go-discuss/c/Xo9Tu10Bgaw). INFERRED simplest design: one hand-written `genrule`/`run_binary` running `ent generate --feature sql/upsert` that outputs a fixed set of files, wrapped in `go_library` targets named for the ent packages, plus `# gazelle:exclude session/ent` and `# gazelle:resolve go github.com/.../session/ent //session/ent:ent` directives. No custom Go extension needed unless generated package count churns. Verdict: directives + a small macro, **Recommended**; custom extension **Not recommended**.

### 1.2 Proto: rules_buf / rules_proto_grpc / native protobuf rules vs `buf generate` in a genrule
- **rules_proto_grpc** - Viable. v5.0.0 is bzlmod-only; Go rules include `go_proto_compile`/`go_grpc_compile`/`go_proto_library`; its Buf rules are lint/breaking *tests* only, not a generator (https://www.github.com/rules-proto-grpc/rules_proto_grpc, https://pkg.go.dev/github.com/rules-proto-grpc/rules_proto_grpc). The Go module is described as alpha in one issue (https://www.github.com/rules-proto-grpc/rules_proto_grpc/issues/311). connect-go and the TS plugins (protoc-gen-es / connect-es) are not first-class; they would need custom plugin rules. Bazel 9 status: not found.
- **rules_go's own `go_proto_library` (via Gazelle's proto extension)** - Recommended for the Go side. Gazelle generates `proto_library` + `go_proto_library` from `.proto` files; connect-go needs a `go_proto_library` `compilers` entry with the connect plugin (INFERRED; check `protoc-gen-connect-go` can be built from `go_deps` as a Bazel binary, which it can since it is a Go program).
- **rules_buf** - Not recommended as the generator. It targets lint/breaking checks; I did not find a documented `buf generate` rule (UNVERIFIED; a search for rules_buf specifically returned nothing). Keep as optional `buf lint` / `buf breaking` test targets.
- **Wrap `buf generate` in a genrule** - Viable fallback. Pros: matches the existing `buf.gen.yaml` exactly, one action produces both Go and the TS in `web-app/src/gen/`. Cons: buf plugins must be provided as hermetic binaries (remote plugins use the network, which breaks sandboxing and hermeticity); use local plugins built by Bazel and `buf` itself pinned via a download rule. Proto-dependency fetch via BSR must be vendored or avoided.
- **Verdict**: hybrid - `go_proto_library` + Gazelle for Go (gets proper per-package targets and affected-target precision), and one `buf generate`-style `run_binary` action (local pinned plugins) only for the TS output. Decide in Phase 3 after a spike on connect-go via `go_proto_library`.

### 1.3 Web: aspect_rules_js / rules_ts vs wrapping `bun run build`
- **Critical finding**: rules_js's `npm_translate_lock` imports a **pnpm** lockfile (https://pro.aspect.build/rulesets/aspect_rules_js/docs/npm_translate_lock); two searches found no `bun.lock` support, and no rules_js issue requesting it surfaced. Treat as NO native bun.lock support (VERIFIED absence in search, not a confirmed negative; open an issue or read the rules_js repo in Phase 3). Resolves requirements Open Question 6 as "not directly".
  - Workarounds (all costly): generate a `pnpm-lock.yaml` from `package.json` in CI (a second lockfile to keep in sync, and CLAUDE.md says Bun everywhere), or return to pnpm for web-app only.
- rules_js 3.0 is current with "enhanced compatibility with Bazel 9" and stable public APIs (https://www.plushcap.com/content/aspect-build/blog/aspect-build-rules_js-30-out-with-the-old-and-default-to-the-new); Gazelle JS is shipped as `aspect_gazelle_js` 1.1.x on the BCR (https://registry-preview.bazel.build/modules/aspect_gazelle_js/1.1.1) and in Aspect's prebuilt Gazelle (https://github.com/aspect-build). Maturity: not rated by any source I found; BenchSci's `rules_nodejs_gazelle` is an alternative (https://github.com/benchsci/rules_nodejs_gazelle). Next.js/`output: export` + rules_js remains the hardest part, per ADR-001.
- **Option A: full rules_js/rules_ts** - Not recommended in the first cutover (lockfile mismatch, Next integration, 1,281 TS files of BUILD churn for modest gain, because the web build is a single leaf producing `server/web/dist`).
- **Option B: hybrid, Bazel owns Go, web build is one opaque action** - **Recommended for v1**. `web-app` becomes a single `genrule`/`sh_binary` that runs `bun install --frozen-lockfile && bun run build` with declared inputs (`glob(web-app/src/**)`, `package.json`, `bun.lock`, generated TS proto output), output tree artifact `dist/` consumed by the server's `go:embed` via a `filegroup`. Cons: network at build time (bun install) means the action needs `no-sandbox`/`requires-network` tags and is not hermetic; mitigate by fetching Bun and the dependency set in a repository rule (`bun install` into a repository_rule-produced `node_modules` tree keyed on `bun.lock` hash) so the action itself is offline. Cache key = declared inputs, so web tests re-run only when web inputs change. jest can be a `sh_test` the same way.
- **Option C (later)**: migrate to rules_js once pnpm-lock or bun support is settled; only worth it if web test/build time dominates.

### 1.4 tmux: rules_foreign_cc vs genrule
- rules_foreign_cc latest seen is 0.15.1 with bzlmod instructions (https://github.com/bazel-contrib/rules_foreign_cc/releases); older versions broke on Bazel 9 over removed `--incompatible_use_toolchain_transition` (Riegeli commit above). No source states Bazel-9 `configure_make` works end to end.
- **rules_foreign_cc `configure_make`** - Viable. It is the idiomatic route and manages out-of-tree builds, but this repo already had it removed once (`b51b60eb1`), and tmux needs libevent + ncurses (+ utf8proc optionally) as deps, each of which is another `configure_make` or a BCR module (libevent/ncurses availability on BCR: unchecked).
- **genrule / `run_binary` calling a pinned build script** - Viable and simpler. Copy the submodule to a scratch dir, `./configure && make`, output the binary. Pros: no rules_foreign_cc version coupling; the stale committed `.o` files in the tree (Rabbit Holes) are sidestepped by building in a copied out-of-tree dir. Cons: relies on host compiler/libs unless a hermetic C toolchain is configured (section 1.8).
- **Verdict**: genrule first (**Recommended**), `rules_foreign_cc` only if hermetic libevent/ncurses are required (**Viable**). tymuxd fetch/embed is an `http_file` + checksum (Recommended).

### 1.5 Affected targets: bazel-diff / target-determinator
- **bazel-diff** (Tinder): computes affected targets between two git revisions, has explain mode and a cache-by-SHA service mode (https://github.com/tinder/bazel-diff). **Recommended** for replacing the three regex lists.
- **target-determinator**: same pattern, simpler CLI plus a Go API and a `driver` that tests the targets (https://github.com/jonmeow/target-determinator). **Viable**; supports Bazel up to latest LTS (Bazel 9 not confirmed here).
- Caveat (INFERRED): with a warm remote/disk cache, `bazel test //...` may already be fast enough that affected selection is an optimisation for cold GHA runners, which have no persistent disk. Measure before adding a tool; keep deployment-trigger regexes (path filters for docs/website-only changes) in workflows since Bazel's graph does not cover non-targeted files.

### 1.6 Aspect Workflows / bazel-lib
- **bazel-lib** (rules_js 3.0 depends on `bazel_lib` 3.0) helpers such as `write_source_files`, `copy_to_directory`, `expand_template`, stamping helpers: **Recommended** (small, widely used, replaces hand-written Starlark).
- **Aspect Workflows** (commercial CI runners with warm disks): **Not recommended** now; pricing not published in my results and requires non-GHA-free runners.
- **aspect-cli / AXL** task runner: **Viable** later for repo tasks; not needed for v1.

### 1.7 Gazelle freshness gate (OSS, no search needed)
- `bazel run //:gazelle -- -mode=diff` or `bazel test` with `gazelle_test` / `update_target_files`-style rule in CI. **Recommended**; closes the "agent-written repo drifts BUILD files" rabbit hole. Also a pre-commit hook.

### 1.8 Hermetic C toolchain / cgo (toolchains_llvm and alternatives)
- Candidates: `toolchains_llvm` (BCR module `llvm`, v0.8.x seen at https://registry-preview.bazel.build/modules/llvm/0.8.7), `hermetic-llvm` (https://github.com/helly25/hermetic-llvm; says vanilla Go needs patches for cgo with its zero-sysroot toolchain, so Go >= 1.27 or a source-built Go), `toolchains_llvm_bootstrapped` (https://github.com/cerisier/toolchains_llvm_bootstrapped), zig-based `hermetic_cc_toolchain` (a dfinity PR describes migrating off it because of darwin SDK blockers: https://github.com/dfinity/ic/pull/10991). The same PR keeps native macOS targets on system Xcode because hermetic macOS support was "not yet mature enough".
- Implication (INFERRED): hermetic cgo on macOS is the least mature corner of this stack. **Strongest simplification: switch SQLite from `mattn/go-sqlite3` (cgo) to `modernc.org/sqlite`** (already in go.mod per requirements). This removes the cgo requirement for the main binary on every platform, making `pure = "on"`, trivial cross-compilation and no C toolchain needed for Go. Cost: modernc performance and behaviour differences; needs an owner decision and a benchmark (the repo has `profile-db-usage` tooling). If kept: use the host toolchain on macOS (Xcode) and `toolchains_llvm` or host gcc on Linux for v1, and defer hermetic C.
- tmux still needs a C compiler; if it stays a genrule it uses the host cc. That is a documented hermeticity exception unless `toolchains_llvm` is registered.

## 2. SaaS and remote cache

Free-tier GitHub runners have no persistent disk; every run starts cold unless it uses a remote cache or GHA `actions/cache` for the disk cache. Remote execution is out of scope.

| Option | Cost / limits | Auth | Fit |
|---|---|---|---|
| **BuildBuddy Cloud free** | Free plan: 100 GB cache transfer, up to 80 RBE cores; no hard caps, vendor contacts on sustained overage (https://www.buildbuddy.io/pricing). Older notes said a 3-engineer cap (https://blog.buildbuddy.io/blog/buildbuddy-v1-2-1-release-notes); verify current terms. | API key header via `--remote_header=x-buildbuddy-api-key=...`; GitHub secret on CI, `op read`/1Password into a gitignored `user.bazelrc` locally | Best fit: also gives BES/invocation UI for the observability requirement. Works from free runners and local worktrees. |
| **EngFlow** | Contact-for-quote (https://docs.engflow.com/overview.html) | mTLS/credentials per contract | Not recommended: enterprise priced, RBE-oriented. |
| **Depot Cache** | On all Depot plans; 14-day default retention; third-party sources list $20/$200 tiers, UNVERIFIED (https://depot.dev/docs/cache/overview) | Depot token | Viable; adds a vendor with little advantage over BuildBuddy for this repo. |
| **Buildless** | GitHub Marketplace lists a "Public Beta" plan, no prices (https://github.com/marketplace/buildless) | token | Not recommended: beta, unclear longevity. |
| **GHA cache via bazel-remote shim** | Free; bounded by GitHub's cache quota (10 GB/repo, per GitHub docs - recall, not re-verified here) | n/a | Viable-fragile: a shim has to run in the job, adds startup time, and entries are scoped per branch. Use `actions/cache` for `--disk_cache` instead, which needs no shim. |
| **Self-hosted bazel-remote on `onyx`** | Free software (https://bazel.build/remote/caching.md, README: htpasswd basic auth, TLS flags `--tls_cert_file/--tls_key_file`); owner already has the host and Tailscale | htpasswd file + TLS; credentials in 1Password | Best for local worktrees (LAN, low latency). GitHub-hosted runners cannot reach a home LAN without exposing a port or tailnet (e.g. Tailscale GH Action - INFERRED). A cache poisoning risk exists if CI writes from untrusted PRs: make CI read-only for fork PRs. |

Recommendation: **two-tier** - (1) shared local `--disk_cache` + `--repository_cache` for all worktrees (zero cost, solves the worktree-cost driver locally), (2) BuildBuddy free tier for CI with read-write on `main` pushes and read-only for PRs, plus optional `onyx` bazel-remote as a LAN tier later. The owner's decision on provider/credentials is a flagged wall-clock blocker.

Worktree cost (VERIFIED from third-party docs, https://fuchsia.dev/fuchsia-src/development/build/bazel_disk_cache and https://blog.engflow.com/2024/05/13/the-many-caches-of-bazel/): disk and repository caches are shared across output bases; each checkout still gets its own output base keyed on workspace path and not deleted when the worktree is removed, leaking GBs. Mitigation: `startup --output_base=<worktree>/.local/bazel-out` (as suggested in the search summary; confirm the exact source) so deleting the worktree deletes the base; keep the shared caches. Pair with `worktree-disk-cleanup` skill. Also set `--disk_cache` GC (Bazel 9 supports max size/age flags; check exact flag names in Phase 3).

## 3. LLM-generated vs battle-tested: bounding the custom surface

Rule of thumb (INFERRED, to be encoded in the Phase 3 plan):
- **Justified hand-written**: thin macros (<=30 lines each) that compose existing rules: `ent_library`, `web_dist`, `embed_go_library`; genrules whose inputs/outputs are explicit; `.bazelrc` configs; `gazelle` directives; stamping scripts. These are small, locally testable with `bazel build`, and failures are loud.
- **Reckless**: a custom Gazelle Go extension (needs the Gazelle plugin API, resolver semantics, language ordering; failures are silent mis-resolution), custom toolchain rules, or custom repository rules re-implementing `go_deps`/`npm_translate_lock`. Only write one after a macro + directives demonstrably fail and with a Go unit test for it.
- **Bounding mechanisms**: (a) a rule that every custom `.bzl` lives under `//build/` with a single owner doc; (b) cap: <=~500 lines of repo-owned Starlark total at cutover, tracked in CI; (c) every macro has a `bazel test` target that exercises it (analysis test via `bazel_skylib`); (d) prefer BCR modules pinned by version and `bazel_dep` over `http_archive` of unreleased forks; (e) a `bazel` skill/doc for agents with canonical commands; (f) Gazelle freshness gate; (g) `buildifier` lint in CI; (h) strangler period (non-blocking CI job) before cutover.

## 4. Fork/adapt: worked examples

I could not retrieve primary repo files in this pass (the search tool returned no direct hits for these). The following are candidates to open in Phase 3; none is verified as matching this stack:
- `buildbuddy-io/buildbuddy` - Go server + TypeScript app + cgo-free, Bazel since inception (INFERRED from repo knowledge; my search did not return its MODULE.bazel; open `https://github.com/buildbuddy-io/buildbuddy/blob/master/MODULE.bazel`).
- `bazel-contrib/rules_go` `examples/` and `tests/` for `go_deps`/`go.work` (https://github.com/bazel-contrib/rules_go/blob/master/docs/go/core/bzlmod.md).
- `aspect-build/bazel-examples` for rules_js + protobuf-ts + Go server (INFERRED, not fetched).
- `cloneable/rules_ent` for ent shape (https://www.github.com/cloneable/rules_ent) - read, do not depend on (WIP).
- `dfinity/ic` for a very large Bazel-9 era monorepo with Go/Rust, hermetic-llvm, macOS toolchain trade-offs (https://github.com/dfinity/ic/pull/10991).
- `constellation` `.bazelrc` for disk-cache/remote-cache conventions (https://git.hackliberty.org/Git-Mirrors/constellation/src/commit/c6819b8d311be071e1f598aab5b935999e2b6fbb/.bazelrc).
No single repo found with Go + proto/connect + ent + Bun web + cgo + submodule tmux; expect to assemble from parts. Gap flagged.

## 5. Alternatives to Bazel, scored vs the three drivers (1 = poor, 5 = strong; INFERRED judgments)

| Option | CI speed | Hermeticity | Worktree cost | Notes |
|---|---|---|---|---|
| **Bazel (decided)** | 4 (with remote cache; 3 without) | 5 for Go/proto/ent, 3 for tmux/web (opaque actions, host cc) | 4 with shared disk cache; output base per worktree is a leak risk | Highest ceiling, highest adoption cost and agent learning cost. |
| Nix flakes (+ Go builders) | 3 | 5 | 3 (store shared, but `go build` is not incremental per-package; `buildGoModule` rebuilds the whole module) | Strong toolchain pinning; no affected-target graph; macOS+Linux works; poor incremental. |
| mise/just + existing caches (status quo+) | 3 (already done via ci-speed) | 2 | 2 | Cheap, no graph, does not address drivers 2 and 3. |
| Pants | 3 | 3 | 3 | Go support exists but smaller ecosystem than rules_go; no ent/buf precedent. Not recommended. |
| Buck2 | 4 | 4 | 4 | Small OSS Go ecosystem; heavier from-scratch rules; Not recommended. |
| Turborepo / Nx | 3 for JS, 1 for Go | 1 | 2 | JS-centric; ADR-001 already deferred. |

Statement: Bazel remains the best single-graph fit for all three drivers *given the owner's decision*; the only real competitor for hermeticity alone is Nix, which loses on incremental Go builds and affected targets. The honest risks are the Bazel-9-era ecosystem (rules_go pin, rules_foreign_cc, rules_js lockfile) and the macOS cgo/codesign corner, both of which are mitigated by pinning versions, the strangler period, and the modernc option. Drivers 2 and 3 are served even with a hybrid web action and no remote cache; driver 1 depends on the cache decision.

## 6. Recommended build-vs-buy bill of materials

| Area | Choice | Build/Buy | Verdict |
|---|---|---|---|
| Build tool | Bazel 9.x via bazelisk, `.bazelversion` pinned, bzlmod only | Buy (OSS) | Recommended |
| Go | rules_go (>= 0.60, Bazel-9 compatible) + Gazelle, `go_deps` from `go.work` incl. `replace` for go-git fork | Buy | Recommended (spike replace + multi-module) |
| Gazelle custom code | `# gazelle:` directives only; no custom Go extension | Build (config) | Recommended |
| ent | `run_binary`/genrule `ent generate --feature sql/upsert` + `go_library` per generated pkg via small macro | Build (<=~40 lines) | Recommended |
| Proto Go | `go_proto_library` via Gazelle; connect-go as compiler plugin | Buy + small config | Recommended (spike) |
| Proto TS | one pinned-plugin `buf generate`-style action into web gen dir | Build (small) | Viable |
| Buf lint/breaking | rules_buf or wrapped `buf` as test | Buy | Viable |
| Web | opaque action: repository-rule-installed Bun deps + `bun run build` -> `dist/` tree artifact; jest as `sh_test` | Build (small) | Recommended v1; rules_js deferred (no bun.lock support) |
| tmux/tymuxd | genrule on submodule copy; tymuxd via `http_file` | Build (small) | Recommended; rules_foreign_cc Viable |
| SQLite/cgo | Owner decision: modernc (pure Go) preferred; else host cc | Decision | Recommend modernc |
| C toolchain | host toolchain v1; `toolchains_llvm` later | Buy | Deferred |
| Affected targets | bazel-diff (or target-determinator) + kept path filters for non-code | Buy | Recommended (measure first) |
| Helpers | bazel_lib, bazel_skylib, buildifier | Buy | Recommended |
| Local caches | shared `--disk_cache` + `--repository_cache`, per-worktree `--output_base` inside worktree | Config | Recommended |
| Remote cache | BuildBuddy free tier (CI RW on main, RO on PRs); optional bazel-remote on `onyx` | Buy | Recommended; owner decision |
| Secrets | GitHub secret for CI; 1Password `op read` into gitignored `user.bazelrc` locally | Config | Recommended |
| Freshness gates | gazelle diff check, buildifier, custom-Starlark size cap | Build (small) | Recommended |
| Not adopted | Aspect Workflows, EngFlow, Buildless, Nix, Pants, Buck2, Turborepo/Nx, custom Gazelle extension, rules_ent | - | Not recommended |

## 7. Open items for Phase 3 spikes (ordered by risk)
1. go-git `replace` and 4-module `go.work` under `go_deps` on Bazel 9 + rules_go >= 0.60.
2. connect-go via `go_proto_library` plus the TS plugin action; confirm `buf.gen.yaml` parity.
3. Decision: modernc vs mattn (benchmark with the DB profile tooling).
4. `go:embed all:dist` edge from the web action; `embed_tmux`/`embed_tymux` tags as `gotags` configs.
5. Bun deps via repository rule; confirm no bun.lock path in rules_js by reading its repo/issues.
6. Test sandbox failures census (`bazel test //...` with `--spawn_strategy=local` vs sandboxed).
7. Measure baselines (cold build time, disk per worktree, CI time-to-green) - none measured in this pass.
