# Why and how we build our own ghostty-vt.wasm

Decision record: `docs/adr/ADR-039-own-ghostty-wasm-build.md`. Steps: `docs/how-to/build-own-ghostty-wasm.md`.

## Why

`ghostty-web` ships one prebuilt `ghostty-vt.wasm`. Its Ghostty is pinned by its maintainers, so a parser gap (DECSTR) or an allocator behavior (freed pages recycled without zeroing, so a new terminal shows an old one's text) can only be fixed by waiting for a release or by shimming around it in the adapter. Owning the wasm build lets us carry fixes, test them, upstream them, and drop them when upstream catches up, on our own timeline.

## Layout

| Path | Role |
|---|---|
| `MODULE.bazel`, `.bazelversion`, `.bazelrc`, `.bazelignore` | Bazel (bzlmod) config; pins: Ghostty commit archive + sha256, patch list, four Zig tarballs + sha256 |
| `third_party/ghostty/` | Bazel package: `ghostty_wasm.bzl` rule, `BUILD.bazel` targets (`:ghostty_vt_wasm`, `:softreset_test`), `ghostty_src.BUILD` for the fetched source |
| `third_party/ghostty-patches/NNNN-*.patch` | Ordered series applied with `patch -p1` (status table in `README-SSQ.md` there) |
| `scripts/build-ghostty-wasm.sh` | Drives `bazel build`, installs into `web-app/public`, compares to the record |
| `scripts/ghostty-wasm.sha256` | `<rev12>-<patchset12>-zig<ver> <wasm sha256>` reproducibility record |
| `make build-ghostty-wasm` / `test-ghostty-wasm` / `verify-ghostty-wasm` | Entry points (opt-in; not in `make build`) |
| `.github/workflows/ghostty-wasm.yml` | One job, workflow-level `paths:` filter |
| `web-app/src/lib/terminal/ghosttyWasm.ts` | Runtime loader with fallback to the stock wasm |

## Build flow

1. Bazel fetches the Ghostty commit archive and the host's Zig tarball, **verifying each sha256**, and applies the patches (`patch_tool = "patch"`; Bazel's built-in patcher rejects 0002). A patch that no longer applies fails the fetch.
2. The `ghostty_zig` rule copies the source tree out of the sandbox and runs Ghostty's own `zig build lib-vt -Dtarget=wasm32-freestanding -Doptimize=ReleaseSmall` (or `test-lib-vt -Dtest-filter=softReset` for `:softreset_test`) with private cache dirs. The action declares `requires-network` because `zig build` fetches Ghostty's hash-pinned Zig dependencies; every other input is pinned, so Bazel's caches (local, disk, remote) are sound.
3. The script copies the output to `web-app/public/ghostty-vt.<sha12>.wasm` and writes `ghostty-vt.json`.
4. It compares the sha256 with `scripts/ghostty-wasm.sha256`. A differing sha for a recorded key means the build is not reproducible, and fails. An absent key prints the line to add.

Not Bazel-native on purpose: Bazel wraps `zig build` instead of reimplementing it with rules_zig, so the build definition stays Ghostty's own and an upstream sync does not mean rewriting BUILD files.

## Serving and cache-busting

`web-app/public/` is copied to the output root unhashed (`web-app/build/stages/public.ts`), the Go static server resolves `.wasm` to `application/wasm` (Go's built-in `mime` table; checked with `mime.TypeByExtension(".wasm")`), and both file names are on the public allowlist (`server/middleware/testdata/public-allowlist.json`). The wasm name contains its own hash, so it can be cached forever; `ghostty-vt.json` is fetched with `cache: "no-cache"` and names the current file, so a new build takes effect on the next load without a bundler define. If the manifest is missing (the SPA fallback returns HTML, which fails `json()`), invalid, or the wasm fails to instantiate, the terminal uses the wasm embedded in ghostty-web.

## Policy: build, do not commit

The wasm is not committed. CI builds it on PRs that touch the pin, patches or script; release builds should run `make build-ghostty-wasm` before `make web-build` (wiring that into the release workflow is a follow-up, since no recorded sha exists yet). Auditability comes from the pinned inputs and the recorded sha256, not from a binary in git.

## Patch series (against `5714ed07`)

| # | Patch | Purpose | Upstream status |
|---|---|---|---|
| 0001 | `0001-wasm-api.patch` | coder/ghostty-web's `ghostty-wasm-api.patch` at tag `v0.4.0`, unchanged (the API ghostty-web's JS calls) | Not upstreamable as is; Ghostty main now has its own C API |
| 0002 | `0002-decstr-soft-reset.patch` | DECSTR: `soft_reset` stream action, `CSI ! p` dispatch, `Terminal.softReset` + Zig test | Upstream main has `Terminal.softReset`; this is a backport |
| 0003 | `0003-scrub-freed-pages-on-wasm.patch` | Zero page memory on free (wasm only) so recycled memory cannot ghost into a new terminal | Candidate to upstream; not yet proposed |

## Verification

Results and their evidence are in ADR-039 (Verification status). In short: the Zig `softReset` test passes and the wasm builds reproducibly in a Linux container, and the differential harness passes against it with native DECSTR and no stale-cell leak. Zig 0.15.2 cannot build on macOS 26, so use CI or a Linux container locally. Still unconfirmed: the Bazel rule on x86_64 CI producing the same sha256 as the aarch64 container, and whether 0003's two zeroing sites cover every page-recycling path (the leak test passes, which is evidence, not proof).

## Stage B (own wrapper) and the newer-Ghostty path

Stage A keeps ghostty-web's JavaScript. Its known problems (a render loop that never pauses, scroll-to-bottom on every write while scrolled up, a minified UMD bundle) need a wrapper of our own, a separate and larger change: canvas renderer, input handler, selection, fit, and the xterm-compatible surface that `ghosttyAdapter.ts` consumes. It should target Ghostty's native C API (`src/terminal/c/` on `v1.4.0`/`main`, Zig 0.17.0) rather than coder's patch, so the order is: Stage A lands, the fork exists, then the wrapper and the pin bump move together. Until then, #1487's pause and the adapter shims stay.
