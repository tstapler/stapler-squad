# Build and use our own ghostty-vt.wasm

Why: `docs/explanation/own-ghostty-wasm-build.md`. Supersedes the manual steps in `docs/how-to/rebuild-ghostty-web-wasm.md` (PR #1490) once both are merged.

## Build

```bash
make build-ghostty-wasm      # bazel build //third_party/ghostty:ghostty_vt_wasm, then install into web-app/public
make test-ghostty-wasm       # same, but first builds //third_party/ghostty:softreset_test (Zig test; fails the build on failure)
make verify-ghostty-wasm     # CI mode: fails unless the wasm sha256 equals scripts/ghostty-wasm.sha256
```

Needs `bazel` (bazelisk; `.bazelversion` pins 9.3.2), system `patch`, and network (Bazel downloads the sha256-pinned Zig and Ghostty archives; `zig build` fetches Ghostty's own Zig dependencies on first build). Bazel fetches and verifies Zig itself, so there is no manual download. Outputs are `web-app/public/ghostty-vt.<sha12>.wasm` and `web-app/public/ghostty-vt.json` (gitignored). A remote or disk cache is opt-in: `BAZEL_FLAGS="--remote_cache=grpcs://<host>" make build-ghostty-wasm`, or put it in the gitignored `.bazelrc.user`.

Analysis-only check (downloads and verifies the inputs, runs no Zig): `bazel build --nobuild //third_party/ghostty:ghostty_vt_wasm //third_party/ghostty:softreset_test`.

## First run: record the checksum

The first build prints `! No recorded sha256 for key ...` and the exact line. Build twice (`bazel clean --expunge` between, so the second is not a cache hit), confirm both shas are identical, then add the line to `scripts/ghostty-wasm.sha256`. From then on CI passes `--verify`. Expect to fix compile errors in the unrun Zig first (the `softReset` test and patch 0003 have never been compiled).

## Use it in the web app

Nothing to configure: `GhosttyTerminal.tsx` fetches `/ghostty-vt.json` and, if present, loads the named wasm through `Ghostty.load(...)`. For a local check run `make web-build` after the wasm build (`public/` is copied into the output), or the Bun dev server (`web-app/build/dev.ts` serves `public/`). The browser network tab should show `ghostty-vt.<sha12>.wasm` with `Content-Type: application/wasm`. Without the build output the terminal uses the wasm embedded in ghostty-web.

## Check it (differential harness)

With PR #1489's harness (`web-app/src/lib/terminal/__tests__/differential/`), load the built file with `Ghostty.load("build/ghostty-wasm/ghostty-vt.wasm")`, disable the adapter's DECSTR rewriter, and confirm the DECSTR streams match xterm.js and the log has no `unimplemented CSI p`; `ghosttyFreedMemory.test.ts` covers patch 0003. The `it.failing` cases turn red when the behavior is fixed, which is the signal to flip them.

## Roll back

Delete `web-app/public/ghostty-vt.json` (or run `scripts/build-ghostty-wasm.sh --clean`) and rebuild the web app. The loader returns null and ghostty-web's embedded stock wasm is used. To also stop building it, remove the Make targets and the workflow file.

## Bump the pin or a patch

1. In `MODULE.bazel` change `GHOSTTY_REV` and `GHOSTTY_SHA256` (`curl -sSL https://github.com/<org>/ghostty/archive/<sha>.tar.gz | shasum -a 256`), or edit/add a patch in `third_party/ghostty-patches/`, or move to the fork: `docs/how-to/ghostty-fork-workflow.md`. Bazel refuses a source archive whose hash differs, and the error prints the real one.
2. Check `minimum_zig_version` in that commit's `build.zig.zon`; update `ZIG_VERSION` and all four `ZIG_SHA256_*` values from `https://ziglang.org/download/index.json`.
3. `make test-ghostty-wasm`, run the differential harness, then record the new sha256 line.
