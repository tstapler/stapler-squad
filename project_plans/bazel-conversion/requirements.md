# Requirements: bazel-conversion

**Date**: 2026-11-01
**Type**: migration
**Complexity**: 4 — high-stakes / cross-cutting (build-system migration touching every package, CI, release, and agent worktree workflow)

## Problem Statement
The build is a hand-wired chain of `make` targets (`proto-gen` → `ent-gen` → `server/web/dist` → `go build`, plus optional tmux/tymuxd embed and otelc variants) with no target graph. Consequences: (a) CI approximates "what changed" with three hand-maintained selection mechanisms (a `grep -qE` regex in `build.yml`, native `paths:` filters in `mcp-integration.yml`, and `tools/ci/detect-feature-changes.sh`; the plan verified they are three different mechanisms, not three regex lists) and re-runs buf/ent/web builds in many jobs; (b) builds are not hermetic — gitignored generated code (`gen/`, `web-app/src/gen/`, `session/ent/*`) and local tool versions (buf, ent, bun, tmux) mean worktrees and CI can drift; (c) many concurrent agent worktrees each re-pay codegen, `go build`, and `bun install` cost and disk.

## Baseline
- Build of record: `Makefile` + `go build`/`go test` (via gotestsum), Bun for `web-app/`, `.github/workflows/*` (18 files), `scripts/install-service.sh` for deploy.
- No `MODULE.bazel`, `WORKSPACE`, `.bazelrc` or BUILD files exist today. A narrow `rules_foreign_cc` tmux path was removed in `b51b60eb1` (2026-07-15) because WORKSPACE mode is broken under Bazel 9. `bazel`/`bazelisk` 9.3.1 is installed locally.
- `project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md` (2026-08-27) rejected Bazel for the CI-speed initiative on appetite and free-runner grounds, and explicitly allows revisiting "with dedicated appetite and a stronger driver." This project is that revisit; its reasons are inputs to research, not settled facts (some, e.g. the 3–6 week appetite cap, no longer apply).
- Scale (counted 2026-11-01): ~3,743 non-web `.go` files outside worktrees; 1 root module plus separate `go.mod` in `tuitest/`, `tools/scanner/`, `tools/lint/`; `replace go-git => tstapler/go-git` fork; proto in `proto/session/v1/*.proto` via buf; ent schema with required `--feature sql/upsert`; `third_party/tmux` git submodule.

## Users / Consumers
The owner; Claude Code agent sessions working in many parallel git worktrees; GitHub Actions CI (free-tier hosted runners); the release/deploy path (`make install-service`, release-please).

## Success Metrics
All three drivers are co-equal (owner decision):
1. **CI speed**: affected-target selection replaces the three selection mechanisms; measured CI time-to-green versus the ci-speed baseline, and remote cache hit ratio (floor set in P10; not a standalone improvement target).
2. **Hermeticity**: one pinned toolchain (Go, protoc/buf plugins, ent, bun/node, tmux) so a clean checkout, a worktree and CI produce the same artifacts; `bazel build //...` works on a fresh clone with no `make proto-gen ent-gen` precondition.
3. **Worktree cost**: cold-build time and disk per new worktree measured before/after, using a shared disk cache (and remote cache if available).
4. **Expensive tests re-run only when their inputs changed (owner, 2026-11-02)**: a PR runs only the e2e specs whose dependencies it touches, with no missed failures against a full run and a recorded escape count of zero over the soak; measured as e2e runner-minutes per partial-change PR against the full-suite baseline (target T8). Evidence: e2e is 1974 s of a 2601 s median critical path (RA2 = 0.987, `baselines.md`).
5. **Packages stay small and scoped (owner request, 2026-11-02)**: behaviour-preserving refactors make the largest, most widely imported packages smaller and more scoped, so that a change invalidates fewer builds, tests and e2e slices. Success is measured by target T9 (mean affected test-LOC and library-LOC share and the share of commits touching a package assumed to be in every e2e slice, replayed over the last 500 `main` commits; size of the two largest packages; test-only layering inversions; the Bazel-target-model T1 as a guard), set at Task 13.1f from baseline and owner-confirmed. Evidence and the honest expected payoff are in `research/package-scoping.md` and ADR-010 (Tier 0 test moves: affected test-LOC share 55.4% to 44.9%; the criteria-selected Tier A/B splits: `server/services` and `session` each about 27% smaller, affected library-LOC share 40.5% to 38.5%). Not a gate on the CI cutover.
Baselines are measured in implementation Story 0.1 (Task 0.1g computes the targets T1-T8 from them; the owner confirms them before Story 0.14). They do not exist at planning time.

**End state (owner decision)**: Bazel is the build of record. `bazel build //...` and `bazel test //...` cover Go, proto, ent, tmux and the web-app; `Makefile` targets become thin wrappers over Bazel; CI runs Bazel.

**Improvement targets (triad repair, 2026-11-01)**: beyond non-regression, the plan adds required improvement targets T1-T8 (work-avoided ratio, billed minutes and time-to-green on partial-change PRs, second-worktree cold build and disk, edit-build-test latency, first-error legibility, regen steps per agent session, e2e work avoided), plus T9 (package scoping, ADR-010, owner request 2026-11-02, reported by `tools/bzl/parity/scoping.sh` and enforced at Story 13.7, not at the CI flip). Each is a formula over measured baselines, set at Story 0.1 (Task 0.1g) and owner-confirmed; see "Improvement targets" in `implementation/plan.md`. Priority rationale (RICE, inputs UNVERIFIED) is in ADR-001.

## Size
XL (> 15M CU) — informational estimate, bands INFERRED (see ESTIMATION.md). Full scope is the default; not a gate.

**Wall-clock blockers**: remote cache provider/credentials decision (owner); soak window running Bazel in CI alongside make before cutover (owner sign-off on parity); macOS signing/TCC and launchd/systemd deploy parity needs device verification (owner); any `tstapler/go-git` fork or third_party submodule decisions.

## Constraints
- Bzlmod only (`MODULE.bazel`); no WORKSPACE (broken under Bazel 9).
- Forking upstream Bazel rule repos (rules_go, gazelle, aspect_rules_js, rules_foreign_cc, etc.) is explicitly allowed (owner, 2026-11-01) when needed to add missing support such as `bun.lock`, ent/connect-go codegen, or Bazel 9 fixes; prefer upstreaming and pin forks via `git_override`/`archive_override`, as with the existing `tstapler/go-git` fork.
- Use Gazelle wherever it applies (Go BUILD files, `go_deps` from `go.mod`, proto rules); hand-write only what Gazelle cannot generate, and write custom Gazelle extensions/directives where a pattern repeats (plan sequencing: directives plus one `map_kind` macro first; an extension is the pre-decided fallback if spike S11 fails, ADR-008 rule 5).
- Free-tier GitHub runners; no remote execution assumed. **Remote cache (owner decision 2026-11-01): self-hosted `bazel-remote` on `onyx`** plus shared local `--disk_cache`/`--repository_cache` across worktrees. Phase 3 must solve reachability from GitHub-hosted runners (tunnel/Tailscale/public endpoint with auth) and fall back gracefully when the cache is unreachable.
- Must keep Linux (primary) and macOS (arm64/amd64, signed) builds working. **Windows (owner decision 2026-11-01): cross-compile only** (windows_amd64 binaries via platform transition; no Windows tests under Bazel).
- Repo rules still apply: ent generated code and proto output stay gitignored/uncommitted; Bun (not npm/pnpm) for JS; Conventional Commits; live service at `:8543` must not be disturbed by manual testing.
- Never force push.

## Non-functional Requirements
- **Performance SLO**: warm no-op `bazel build //:stapler-squad` should be seconds; target for cold CI build set in Phase 3 from the measured baseline.
- **Scalability**: must scale to many concurrent worktrees sharing one cache without corrupting it.
- **Security classification**: internal. Remote-cache credentials must be handled via 1Password/GitHub secrets, never committed.
- **Data residency**: no special requirements.

## Scope
### In Scope
- `MODULE.bazel`, `.bazelversion`, `.bazelrc` (+ per-OS and CI configs), Gazelle setup (`gazelle` target, `# gazelle:` directives, `go_deps` extension, `go.mod` replace handling).
- Go: all packages and tests, build tags (`embed_tmux`, `embed_tymux`, `sinkguard`, `gitoracle`, `integration`, race), cgo for `mattn/go-sqlite3` (or evaluate its modernc alternative already in go.mod), `go:embed` of CHANGELOG.md, `server/web/dist`, JSON/tmux/tymuxd embeds, version stamping.
- Codegen as Bazel actions: proto → Go + ConnectRPC + TS (buf plugins), ent generate with `--feature sql/upsert`, feature-registry/other generated inputs if applicable.
- tmux build via a bzlmod-compatible rule (`rules_foreign_cc` under bzlmod, or genrule) from a pinned release tarball (`http_archive`) with targets at the tracked path `tools/bzl/tmux/`, and tymuxd fetch/embed. Amended in planning: the `third_party/tmux` submodule is a gitlink, so no tracked BUILD file can live under it; it remains only a version pin compared against `tools/bzl/tmux/PIN` (plan Story 0.7, ADR-007).
- web-app via `aspect_rules_js` / `rules_ts` / Bun-compatible install, producing `server/web/dist`; jest tests.
- Separate modules (`tuitest`, `tools/scanner`, `tools/lint`) and custom lint analyzers; `make lint`/`ready` gates (golangci-lint, dupl, jscpd, custom analyzers) mapped to Bazel or kept as wrapped tools.
- Makefile wrappers, `scripts/install-service.sh`, GitHub Actions workflows (Bazel setup, caching, affected-target selection), release-please/release artifacts, Formula/.
- Playwright e2e global-setup use of the built binary, and (owner directive 2026-11-02, ADR-009) e2e participation in the Bazel dependency graph: per-spec-group test targets whose declared inputs are the measured slice of Go packages, web files and protos the spec exercises, selected by reverse dependencies on every PR, with a full run on `main` and nightly as the safety net. Other expensive suites (Jest, Go integration, axe/Lighthouse) are selected the same way.
- Package scoping refactors (owner request 2026-11-02, ADR-010, Epic 13): behaviour-preserving moves of files, test files and relocated declarations that make `session`, `server/services` and other widely imported packages smaller and more scoped, chosen by a measured, reproducible payoff model; test-only layering inversions removed; a package-size ratchet and boundary rules so the packages do not regrow.
- Docs: how-to/reference/explanation under `docs/`, ADR superseding ci-speed ADR-001, updates to `CLAUDE.md`.

### Out of Scope
- Remote execution (RBE).
- Rewriting application code beyond what hermeticity requires (e.g. removing cwd/`go:embed` path assumptions). **Amended 2026-11-02 (owner request, ADR-010)**: behaviour-preserving package scoping moves are in scope (Epic 13); public API redesign, signature changes, new abstractions for the cut, and splitting `Instance`/`EntRepository`/`Storage` (Tier C, interface extraction) remain out of scope.
- Changing the frontend framework or the Next-to-Bun migration itself (tracked in `web-app-bun-build`).
- Committing generated code.

## Rabbit Holes
- **ent codegen**: gitignored output, needs `sql/upsert`, and Gazelle must know the generated package exists; likely a custom Gazelle extension or `# gazelle:resolve`/`gazelle:exclude` plus a `go_library` over genrule output.
- **proto/buf**: buf plugin set (Go, connect-go, TS) versus rules_proto/`rules_buf`/`protoc` toolchains; generated TS lands in `web-app/src/gen/` and feeds the web build.
- **cgo + sqlite** across Linux/macOS toolchains (hermetic C toolchain via `toolchains_llvm` vs host), and the `darwin && cgo` files; the `godebug netdns=go` setting.
- **tmux/tymuxd embedding**: `rules_foreign_cc` removed once; autotools build of a submodule with generated `.o` files committed/stale in the tree.
- **`go:embed all:dist` coupling** between web build output and the Go server package (dependency edge across languages).
- **Separate go.mod modules and the go-git replace fork** under `go_deps`.
- **macOS codesigning/TCC** requirement: Bazel output binaries must be signed identically for grant persistence.
- **Build tags and test matrix** (race, integration, sinkguard) mapped to Bazel configs/`gotags`.
- **Tests that assume repo-root cwd, real tmux, real git, network, or `$HOME`** will break in Bazel's sandbox; flake handling per repo policy (fix, don't defer).
- **bun + rules_js** lockfile translation (`bun.lock`) and node_modules sharing used by worktrees today.
- **Worktrees**: Bazel output bases are per-workspace-path, so N worktrees means N output bases unless a shared disk cache/repository cache is configured.
- Gazelle churn: keeping BUILD files in sync in a repo largely written by agents (needs a CI freshness gate and a lint/pre-commit hook).

## Alternatives Considered
- Status quo plus ci-speed fixes (build-once/fan-out artifacts) — already delivered; does not address hermeticity or worktree cost.
- Go-only Bazel with web-app left native — rejected by owner choice of full end state, but may be a sequencing stage.
- Nix / mise / Turborepo / Nx — to be re-evaluated briefly in research against the three drivers.
- Buy: BuildBuddy / EngFlow / GitHub Actions cache as Bazel remote cache.

## Feasibility Risks
- Free-runner constraint limits remote cache wins; cache provider decision is owner-gated.
- Bazel 9 ecosystem maturity for rules_go/gazelle/rules_js/rules_foreign_cc under bzlmod; version pinning needed (`bazel-version-upgrades` skill).
- Agent-authored repo: agents must be able to reason in Bazel (guardrail docs/skills required).
- Test suite sandbox incompatibilities may be widespread (3,700+ Go files).

## Observability Requirements
Build metrics: Bazel BEP/profile upload in CI, cache hit rate, CI time-to-green, per-worktree cold build time and disk; a Gazelle-freshness check; trend script comparable to ci-speed's. Existing service metrics/dashboards must be unaffected.

## Risk Control
- Strangler approach: Bazel runs alongside make/CI as a non-blocking check until parity (identical binary behaviour, test pass set) is shown, then cutover; make wrappers cut over target by target.
- Rollback: keep pre-cutover Makefile recipes available behind a `BUILD_SYSTEM=make` switch for one release cycle; revert is a single PR.
- Live service stays on the current build until the Bazel-built binary passes the e2e suite and a manual isolated-instance check.

## Open Questions
- ~~Remote cache provider~~ RESOLVED: self-hosted bazel-remote on onyx (reachability from CI is a Phase 3 design item).
- ~~Windows~~ RESOLVED: cross-compile only.
- ~~cgo sqlite~~ RESOLVED by research: prod code already uses modernc; mattn is imported only by 2 test files (move them to modernc). Real cgo = 3 darwin files.
- ~~go-git replace~~ RESOLVED by spike (resolves via go_deps.from_file). Separate-module `tools/lint` handling still to be spiked.
- ~~Which lint gates become Bazel targets versus stay as wrapped tools?~~ ANSWERED by the plan: golangci-lint and the diff/history gates stay wrapped, custom analyzers may move to `nogo` if spike S12 passes (ADR-005, Stories 8.3 and 8.4).
- ~~Can `aspect_rules_js` consume `bun.lock`, or does it require a pnpm/npm lockfile?~~ ANSWERED: no (pnpm lockfile only). Default is a repo-owned Bun repository rule, with `rules_bun` and an `aspect_rules_js` fork as costed fallbacks (ADR-003, ADR-008, spike S4).
- `tools/lint` separate-module handling: PENDING spike S-gomod (Task 0.2c); not yet verified.
