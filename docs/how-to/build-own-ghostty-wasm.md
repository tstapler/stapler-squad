# Build and use our own ghostty-vt.wasm

Why: `docs/explanation/own-ghostty-wasm-build.md`. Supersedes the manual steps in `docs/how-to/rebuild-ghostty-web-wasm.md` (PR #1490) once both are merged.

## Build

```bash
make build-ghostty-wasm      # bazel build //third_party/ghostty:ghostty_vt_wasm, then install into web-app/build/ghostty-own
make test-ghostty-wasm       # same, but first builds //third_party/ghostty:softreset_test (Zig test; fails the build on failure)
make verify-ghostty-wasm     # CI mode: fails unless the wasm sha256 equals scripts/ghostty-wasm.sha256
```

**macOS 26 caveat:** Zig 0.15.2 fails to link its build runner there; build on Linux (CI) or in a `linux/arm64` container (mount the sha256-pinned Zig linux tarball and run the `zig build` commands from `third_party/ghostty/ghostty_wasm.bzl`).

Needs `bazel` (bazelisk; `.bazelversion` pins 9.3.2), and network (Bazel downloads the sha256-pinned Zig and Ghostty archives; `zig build` fetches Ghostty's own Zig dependencies on first build). Bazel fetches and verifies Zig itself, so there is no manual download. The output is `web-app/build/ghostty-own/ghostty-vt.wasm` (gitignored). A remote or disk cache is opt-in: `BAZEL_FLAGS="--remote_cache=grpcs://<host>" make build-ghostty-wasm`, or put it in the gitignored `.bazelrc.user`.

Analysis-only check (downloads and verifies the inputs, runs no Zig): `bazel build --nobuild //third_party/ghostty:ghostty_vt_wasm //third_party/ghostty:softreset_test`.

## Record the checksum for a new pin

A build with an unrecorded key prints `! No recorded sha256 for key ...` and the exact line. Build twice (`bazel clean --expunge` between, so the second is not a cache hit), confirm both shas are identical, then add the line to `scripts/ghostty-wasm.sha256`. CI passes `--verify` once any record exists, and the script installs the wasm only after the record matches. A stale `web-app/build/ghostty-own/` from an older pin stays installed until you rebuild or run `--clean`, so rebuild after every pin bump.

## Use it in the web app

Nothing to configure: after the wasm build, `make web-build` (or the Bun dev server) makes `web-app/build/plugins/ghostty-wasm.ts` import `web-app/build/ghostty-own/ghostty-vt.wasm` instead of ghostty-web's stock wasm, and Bun emits it as a hashed asset under `_next/static/media/`, the same path as the stock one. Confirm with the network tab (a `ghostty-vt-<hash>.wasm` request, `Content-Type: application/wasm`) on a terminal with the `ghostty_renderer` flag on. Without the build output the stock wasm ships.

## Check it (differential harness)

With PR #1489's harness (`web-app/src/lib/terminal/__tests__/differential/`), make `loadGhostty()` read `GHOSTTY_WASM_PATH` (instantiate the file with `WebAssembly.instantiate` and `new Ghostty(instance)`, because `Ghostty.load(path)` falls back to `fetch` under jest) and turn the adapter's DECSTR rewriter off when it is set, then run `GHOSTTY_WASM_PATH=<built.wasm> bunx jest --testPathPatterns=differential` and confirm the DECSTR streams match xterm.js and the log has no `unimplemented CSI p`; `ghosttyFreedMemory.test.ts` covers patch 0003. The `it.failing` cases turn red when the behavior is fixed, which is the signal to flip them.

## Roll back

Run `scripts/build-ghostty-wasm.sh --clean` (removes `web-app/build/ghostty-own/`) and rebuild the web app: the plugin falls back to ghostty-web's stock wasm. To also stop building it, remove the Make targets and the workflow file.

## Bump the pin or a patch

1. In `MODULE.bazel` change `GHOSTTY_REV` and `GHOSTTY_SHA256` (`curl -sSL https://github.com/<org>/ghostty/archive/<sha>.tar.gz | shasum -a 256`), or add a commit to the fork: `docs/how-to/ghostty-fork-workflow.md`. Bazel refuses a source archive whose hash differs, and the error prints the real one.
2. Check `minimum_zig_version` in that commit's `build.zig.zon`; update `ZIG_VERSION` and all four `ZIG_SHA256_*` values from `https://ziglang.org/download/index.json`.
3. `make test-ghostty-wasm`, run the differential harness, then record the new sha256 line.
