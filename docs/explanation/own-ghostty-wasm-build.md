# Why and how we build our own ghostty-vt.wasm

Decision record: `docs/adr/ADR-039-own-ghostty-wasm-build.md`. Steps: `docs/how-to/build-own-ghostty-wasm.md`.

## Why

`ghostty-web` ships one prebuilt `ghostty-vt.wasm`. Its Ghostty is pinned by its maintainers, so a parser gap (DECSTR) or an allocator behavior (freed pages recycled without zeroing, so a new terminal shows an old one's text) can only be fixed by waiting for a release or by shimming around it in the adapter. Owning the wasm build lets us carry fixes, test them, upstream them, and drop them when upstream catches up, on our own timeline.

## Layout

| Path | Role |
|---|---|
| `MODULE.bazel`, `.bazelversion`, `.bazelrc`, `.bazelignore` | Bazel (bzlmod) config; pins: Ghostty fork commit archive + sha256, four Zig tarballs + sha256 |
| `third_party/ghostty/` | Bazel package: `ghostty_wasm.bzl` rule, `BUILD.bazel` targets (`:ghostty_vt_wasm`, `:softreset_test`), `ghostty_src.BUILD` for the fetched source |
| `scripts/build-ghostty-wasm.sh` | Drives `bazel build`, installs into `web-app/build/ghostty-own/`, compares to the record |
| `scripts/ghostty-wasm.sha256` | `<rev12>-zig<ver> <wasm sha256>` reproducibility record |
| `make build-ghostty-wasm` / `test-ghostty-wasm` / `verify-ghostty-wasm` | Entry points (opt-in; not in `make build`) |
| `.github/workflows/ghostty-wasm.yml` | One job, workflow-level `paths:` filter |
| `web-app/build/plugins/ghostty-wasm.ts` | Bun plugin: externalizes ghostty-web's wasm as a hashed asset (#1511), using our build when present |

## Build flow

1. Bazel fetches the Ghostty commit archive and the host's Zig tarball, **verifying each sha256**. The Ghostty source is a commit of the `tstapler/ghostty` fork (`ssq/main`, tag `ssq.1`): upstream plus our patches as commits, so there is no patch step.
2. The `ghostty_zig` rule copies the source tree out of the sandbox and runs Ghostty's own `zig build lib-vt -Dtarget=wasm32-freestanding -Doptimize=ReleaseSmall` (or `test-lib-vt -Dtest-filter=softReset` for `:softreset_test`) with private cache dirs. The action declares `requires-network` because `zig build` fetches Ghostty's hash-pinned Zig dependencies; every other input is pinned, so Bazel's caches (local, disk, remote) are sound.
3. The script copies the output to `web-app/build/ghostty-own/ghostty-vt.wasm` (gitignored).
4. It compares the sha256 with `scripts/ghostty-wasm.sha256`. A differing sha for a recorded key means the build is not reproducible, and fails. An absent key prints the line to add.

Not Bazel-native on purpose: Bazel wraps `zig build` instead of reimplementing it with rules_zig, so the build definition stays Ghostty's own and an upstream sync does not mean rewriting BUILD files.

## Serving and cache-busting

No runtime loader. `web-app/build/plugins/ghostty-wasm.ts` rewrites ghostty-web's inlined base64 wasm into an import of one file and Bun emits it as a content-hashed asset under `_next/static/media/` (PR #1511). When `web-app/build/ghostty-own/ghostty-vt.wasm` exists the plugin imports that file instead of ghostty-web's stock `dist/ghostty-vt.wasm`, so our wasm takes exactly the same path, cache-busts by content hash, and needs no public-allowlist entry. Without the build output the stock wasm ships. The Makefile's `web-app/out` target tracks `web-app/build/**`, so a new wasm triggers a rebuild.

## Policy: build, do not commit

The wasm is not committed. CI builds it on PRs that touch the pin or script; release and CI web builds get it through `.github/actions/prepare` (fetch the published, sha256-verified asset, or Bazel-build it), and a Mac downloads the same asset (`docs/how-to/build-own-ghostty-wasm.md`). Auditability comes from the pinned inputs and the recorded sha256, not from a binary in git.

## Patch series

The patches are commits on the fork's `ssq/main` (index: its `README-SSQ.md`; workflow: `docs/how-to/ghostty-fork-workflow.md`):

| Commit | Purpose | Upstream status |
|---|---|---|
| `17f5c98` | coder/ghostty-web's wasm C API at `v0.4.0`, unchanged (the API ghostty-web's JS calls) | Not upstreamable as is; Ghostty main now has its own C API |
| `ffd0420` | DECSTR: `soft_reset` stream action, `CSI ! p` dispatch, `Terminal.softReset` + Zig test | Upstream main has `Terminal.softReset`; this is a backport |
| `9c7f2f1` | Zero page memory on free (wasm only) so recycled memory cannot ghost into a new terminal | Candidate to upstream; not yet proposed |

## Verification

Results and their evidence are in ADR-039 (Verification status). In short: the Zig `softReset` test passes and the wasm builds reproducibly in a Linux container, and the differential harness passes against it with native DECSTR and no stale-cell leak. Zig 0.15.2 cannot build on macOS 26, so use CI or a Linux container locally. Still unconfirmed: the Bazel rule on x86_64 CI producing the same sha256 as the aarch64 container, and whether 0003's two zeroing sites cover every page-recycling path (the leak test passes, which is evidence, not proof).

## Stage B (own wrapper) and the newer-Ghostty path

Stage A keeps ghostty-web's JavaScript. Its known problems (a render loop that never pauses, scroll-to-bottom on every write while scrolled up, a minified UMD bundle) need a wrapper of our own, a separate and larger change: canvas renderer, input handler, selection, fit, and the xterm-compatible surface that `ghosttyAdapter.ts` consumes. It should target Ghostty's native C API (`src/terminal/c/` on `v1.4.0`/`main`, Zig 0.17.0) rather than coder's patch, so the order is: Stage A lands, the fork exists, then the wrapper and the pin bump move together. Until then, #1487's pause and the adapter shims stay.
