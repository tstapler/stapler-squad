# ADR-039: Build our own ghostty-vt.wasm with Bazel from a pinned Ghostty and our patch series

- Status: Accepted (Stage A implemented; analysis verified, build not yet executed; see Verification status)
- Design: `docs/explanation/own-ghostty-wasm-build.md`
- Runbooks: `docs/how-to/build-own-ghostty-wasm.md`, `docs/how-to/ghostty-fork-workflow.md`

## Context

The `ghostty_renderer` terminal runs on `ghostty-web` 0.4.0 (npm, `coder/ghostty-web`), whose bundled `ghostty-vt.wasm` is Ghostty `5714ed07` (2025-12-01) plus coder's `ghostty-wasm-api.patch`. We carry adapter-level workarounds for gaps in it: no DECSTR (`CSI ! p`), freed WASM memory not zeroed (ghost text in fresh terminals), PR #1489/#1490. Fixing these natively needs a different wasm, and waiting for a ghostty-web release gives us no control over timing (0.4.0 was still npm `latest` on 2026-11-08 per the #1490 runbook).

## Decision

1. **Build the wasm ourselves** from a pinned Ghostty commit plus an ordered patch series, with a checksum-pinned Zig toolchain. `ghostty-web`'s JavaScript stays for now and loads our wasm through its `Ghostty.load(path)` / `Terminal({ ghostty })` hooks (Stage A). Replacing the JS with our own thin wrapper is Stage B, a separate PR series.
2. **Build system: Bazel (bzlmod).** The owner's direction is that the repo is moving to Bazel, so this artifact is built with it from the start. The change is additive: only `//third_party/ghostty:ghostty_vt_wasm` and `:softreset_test` exist; the Go and web builds are untouched, `make build-ghostty-wasm` wraps `bazel build`, and Bazel is pinned by `.bazelversion` (bazelisk).
3. **Pins** live in `MODULE.bazel`: the Ghostty commit archive (`http_archive`, sha256), the patch series, and four Zig 0.15.2 tarballs (sha256 from `ziglang.org/download/index.json`). Bazel verifies every digest before use, so the toolchain and source are hermetic by content.
4. **Bazel wraps `zig build`; it does not replace it with rules_zig.** Ghostty's `build.zig` is the real build graph (wasm API exports, `lib-vt` step, dependency pins in `build.zig.zon`). Re-describing it in rules_zig would fork the build definition we want to track upstream.
5. **Fork:** `github.com/tstapler/ghostty` exists. `ssq/main` (upstream `5714ed07` + the three patches as commits + `README-SSQ.md`) is prepared locally but not yet pushed (the push was denied by the sandbox's permission classifier; command in `docs/how-to/ghostty-fork-workflow.md`). After the push, `MODULE.bazel` switches `GHOSTTY_ORG`/`GHOSTTY_REV`/`GHOSTTY_SHA256` to the fork commit and `GHOSTTY_PATCHES` to `[]`. A `third_party/ghostty` git submodule is not used: Bazel fetches the fork commit as an archive (nothing else reads the tree), and the directory holds the Bazel package. For hacking on the fork in place use `--override_repository=ghostty_src=<checkout>` with a `BUILD.bazel` copied from `third_party/ghostty/ghostty_src.BUILD`.
6. **Do not commit the wasm.** It is built by `make build-ghostty-wasm` and in CI, served from `web-app/public/` under a content-addressed name, and its sha256 is recorded in `scripts/ghostty-wasm.sha256`. The web app falls back to ghostty-web's embedded stock wasm whenever the build output is absent.
7. **Rollback** is deleting the build output (or the loader call): behavior reverts to stock ghostty-web 0.4.0 plus the adapter shims. Removing Bazel from the repo again is deleting `MODULE.bazel*`, `.bazel*`, `third_party/ghostty/`.

## Prior Bazel in this repo

Not on `origin/main` (85a168b72): `git ls-files | grep -iE '(^|/)(BUILD|BUILD.bazel|WORKSPACE|MODULE.bazel)$|\.bzl$'` printed nothing. History: PR #3 (closed) proposed a Bazel build with Gazelle, and commit `760fde1a0` ("remove broken Bazel/rules_foreign_cc tmux build path") shows a Bazel tmux path existed on `main` before it was removed, so the owner's recollection that tmux was Bazel-built matches history, not today's tree. Today tmux is a submodule built by `scripts/build-tmux.sh`; tymuxd is a prebuilt fetched against `scripts/tymuxd-checksums.txt`. No open PR adds Bazel (`gh pr list --search bazel` returned only merged/closed PRs, none adding it).

## Consequences

- Upgrades are a pin bump plus a rebase in the fork, on our timeline, gated by the Zig test and the differential harness (#1489).
- Bazel (via bazelisk) is needed to build; the target is opt-in, not part of `make build`.
- The build action is not network-free: `zig build` fetches Ghostty's own Zig dependencies (hash-pinned in `build.zig.zon`), so the action declares `requires-network`. Mirroring those as Bazel repos would make it fully hermetic and is a follow-up; the action is still cacheable (local, disk and remote caches) because every declared input is pinned.
- Bazel's built-in patcher fails on patch 0002 (`CONTENT_DOES_NOT_MATCH_TARGET`, reproduced), so `MODULE.bazel` sets `patch_tool = "patch"`. This needs system `patch`, present on macOS and the GitHub Ubuntu runners.
- Patch 0001 (coder's wasm API) only applies to commits near `5714ed07`; Ghostty `main` and `v1.4.0` ship their own C API (`src/terminal/c/`) and require Zig 0.17.0 (`build.zig.zon`). A newer pin means porting the wrapper to that API, which is Stage B.

## Alternatives rejected

- **Script + Make + CI without Bazel** (the tmux/tymuxd pattern; built first, then replaced): lower friction today, but the owner is standardizing on Bazel and a second build system for the same artifact would have to be migrated later.
- **rules_zig instead of wrapping `zig build`:** see decision 4.
- **Fork the ghostty-web npm package:** keeps its minified bundle and render-loop/scroll bugs in our tree and does not address the wasm.
- **Commit the wasm** (as #1490 proposed): a ~0.4 MB binary per bump in git history that a reviewer cannot read; pinned inputs plus a recorded sha256 give the same audit trail.

## Verification status

- VERIFIED: `bazel build --nobuild //third_party/ghostty:ghostty_vt_wasm //third_party/ghostty:softreset_test` succeeds with Bazel 9.3.2: it fetched and sha256-verified the Ghostty archive and the host Zig tarball, applied all three patches, and analyzed both targets (0 actions executed). Patches also apply cleanly in order with `patch -p1` to a pristine `5714ed07` export. `jest ghosttyWasm` passes 13/13; `shellcheck` and `actionlint` pass.
- NOT RUN: any Zig execution. The sandbox's permission classifier denied running the downloaded Zig binary directly, and running it through `bazel build` would be the same action, so I did not. The wasm has not been built: its size and sha256 are unknown; the `softReset` Zig test and patch 0003 are uncompiled; the differential harness has not run against it. `scripts/ghostty-wasm.sha256` has no entry until someone runs the build.
