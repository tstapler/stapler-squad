# ADR-039: Build our own ghostty-vt.wasm with Bazel from our pinned Ghostty fork

- Status: Accepted (Stage A implemented; see Verification status)
- Design: `docs/explanation/own-ghostty-wasm-build.md`
- Runbooks: `docs/how-to/build-own-ghostty-wasm.md`, `docs/how-to/ghostty-fork-workflow.md`

## Context

The `ghostty_renderer` terminal runs on `ghostty-web` 0.4.0 (npm, `coder/ghostty-web`), whose bundled `ghostty-vt.wasm` is Ghostty `5714ed07` (2025-12-01) plus coder's `ghostty-wasm-api.patch`. We carry adapter-level workarounds for gaps in it: no DECSTR (`CSI ! p`), freed WASM memory not zeroed (ghost text in fresh terminals), PR #1489/#1490. Fixing these natively needs a different wasm, and waiting for a ghostty-web release gives us no control over timing (0.4.0 was still npm `latest` on 2026-11-08 per the #1490 runbook).

## Decision

1. **Build the wasm ourselves** from a pinned commit of the `tstapler/ghostty` fork (upstream plus our patches as commits), with a checksum-pinned Zig toolchain. `ghostty-web`'s JavaScript stays for now and ships our wasm in place of its embedded one through the Bun build's ghostty plugin (Stage A). Replacing the JS with our own thin wrapper is Stage B, a separate PR series.
2. **Build system: Bazel (bzlmod).** The owner's direction is that the repo is moving to Bazel, so this artifact is built with it from the start. The change is additive: only `//third_party/ghostty:ghostty_vt_wasm` and `:softreset_test` exist; the Go and web builds are untouched, `make build-ghostty-wasm` wraps `bazel build`, and Bazel is pinned by `.bazelversion` (bazelisk).
3. **Pins** live in `MODULE.bazel`: the Ghostty fork commit archive (`http_archive`, sha256) and four Zig 0.15.2 tarballs (sha256 from `ziglang.org/download/index.json`). Bazel verifies every digest before use, so the toolchain and source are hermetic by content.
4. **Bazel wraps `zig build`; it does not replace it with rules_zig.** Ghostty's `build.zig` is the real build graph (wasm API exports, `lib-vt` step, dependency pins in `build.zig.zon`). Re-describing it in rules_zig would fork the build definition we want to track upstream.
5. **Fork:** `github.com/tstapler/ghostty`, `ssq/main` at tag `ssq.1` (`38c7055`): upstream `5714ed07` plus the three patches as commits. `MODULE.bazel` pins that commit and its archive sha256. A `third_party/ghostty` submodule is not used: Bazel fetches the fork commit as an archive. For hacking on the fork in place use `--override_repository=ghostty_src=<checkout>` with a `BUILD.bazel` copied from `third_party/ghostty/ghostty_src.BUILD`.
6. **Do not commit the wasm.** It is built by `make build-ghostty-wasm` and in CI, externalized as a hashed asset by the Bun build's ghostty plugin (the #1511 path), and its sha256 is recorded in `scripts/ghostty-wasm.sha256`. The build falls back to ghostty-web's stock wasm whenever the build output is absent.
7. **Rollback** is deleting the build output and rebuilding the web app: behavior reverts to stock ghostty-web 0.4.0 plus the adapter shims. Removing Bazel from the repo again is deleting `MODULE.bazel*`, `.bazel*`, `third_party/ghostty/`.

## Prior Bazel in this repo

Not on `origin/main` (85a168b72): `git ls-files | grep -iE '(^|/)(BUILD|BUILD.bazel|WORKSPACE|MODULE.bazel)$|\.bzl$'` printed nothing. History: PR #3 (closed) proposed a Bazel build with Gazelle, and commit `760fde1a0` ("remove broken Bazel/rules_foreign_cc tmux build path") shows a Bazel tmux path existed on `main` before it was removed, so the owner's recollection that tmux was Bazel-built matches history, not today's tree. Today tmux is a submodule built by `scripts/build-tmux.sh`; tymuxd is a prebuilt fetched against `scripts/tymuxd-checksums.txt`. No open PR adds Bazel (`gh pr list --search bazel` returned only merged/closed PRs, none adding it).

## Consequences

- Upgrades are a pin bump plus a rebase in the fork, on our timeline, gated by the Zig test and the differential harness (#1489).
- Bazel (via bazelisk) is needed to build; the target is opt-in, not part of `make build`.
- The build action is not network-free: `zig build` fetches Ghostty's own Zig dependencies (hash-pinned in `build.zig.zon`), so the action declares `requires-network`. Mirroring those as Bazel repos would make it fully hermetic and is a follow-up; the action is still cacheable (local, disk and remote caches) because every declared input is pinned.
- The fork's wasm API commit (coder's) only applies to commits near `5714ed07`; Ghostty `main` and `v1.4.0` ship their own C API (`src/terminal/c/`) and require Zig 0.17.0 (`build.zig.zon`). A newer pin means porting the wrapper to that API, which is Stage B.

## Alternatives rejected

- **Script + Make + CI without Bazel** (the tmux/tymuxd pattern; built first, then replaced): lower friction today, but the owner is standardizing on Bazel and a second build system for the same artifact would have to be migrated later.
- **rules_zig instead of wrapping `zig build`:** see decision 4.
- **Fork the ghostty-web npm package:** keeps its minified bundle and render-loop/scroll bugs in our tree and does not address the wasm.
- **Commit the wasm** (as #1490 proposed): a ~0.4 MB binary per bump in git history that a reviewer cannot read; pinned inputs plus a recorded sha256 give the same audit trail.

## Verification status

Zig 0.15.2 does not link its build runner on macOS 26 (undefined libSystem symbols), so builds run on Linux: CI, or a `linux/arm64` container with the sha256-pinned Zig tarball.

- VERIFIED (linux/arm64 container, Zig 0.15.2, patches applied to a pristine `5714ed07`): `zig build test-lib-vt -Dtest-filter=softReset` passes (the filter adds tests over a no-match baseline, 41 vs 37 tests passed); `zig build lib-vt -Dtarget=wasm32-freestanding -Doptimize=ReleaseSmall` produces `ghostty-vt.wasm`, 423919 bytes (stock ghostty-web 0.4.0: 423045), sha256 `18bd51b696cd8a993f467ba32fa7add51586ccbb065d8d8db0c5ab95720efafb`. Two clean builds from different source paths gave the same sha256. The first draft of patch 0002 did not compile (`default_modes` is not a `Terminal` field; CI reported it); it is fixed.
- VERIFIED (jest, PR #1489's differential harness loading the built file, DECSTR rewriter off): 199/199 xterm.js differential tests pass, including all 9 DECSTR streams, which fail on the stock wasm without the rewriter. The stale-cell leak test marked `it.failing` now fails as expected-to-fail-no-more, i.e. patch 0003 removes the leak (stock: 210/210 pass with the leak present).
- VERIFIED: `bazel build --nobuild` analysis of both targets, patch application via Bazel, `jest ghosttyWasm` 13/13, `shellcheck`, `actionlint`.
- INFERRED until CI reports it: that the Bazel rule itself runs the same commands successfully in the Bazel sandbox on x86_64 Linux and produces the same sha256 as the aarch64 container build. An earlier CI run was green while the build had failed (`| tee` hid the exit status; fixed with `pipefail`).
