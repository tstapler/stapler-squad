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

## Get the wasm without building it (macOS, `make install-service`)

Zig cannot link on macOS 26, so a Mac downloads the wasm CI built for the pin instead. `make web-build` / `make install-service` run `scripts/fetch-ghostty-wasm.sh` automatically (a stamp re-runs it when `MODULE.bazel` or `scripts/ghostty-wasm.sha256` changes); run it by hand with `make fetch-ghostty-wasm`.

- Source: release asset `ghostty-wasm-<rev12>-zig<version>/ghostty-vt.wasm`, published by `.github/workflows/ghostty-wasm-publish.yml` after a successful push build of the Ghostty WASM workflow on main. The publisher refuses bytes whose sha256 differs from `scripts/ghostty-wasm.sha256`.
- Fail-closed: the download is installed only if its sha256 equals the recorded one. On any failure (not published yet, offline, mismatch) the script removes any installed wasm, prints a `!!!` warning, and the web build falls back to ghostty-web's stock wasm with a second warning. The stock wasm lacks the DECSTR and page-zeroing fixes, so treat the warning as a bug to fix, not noise.
- The Bun plugin re-checks the sha256 of whatever is in `web-app/build/ghostty-own/` against the record at bundle time, so a stale or hand-placed wasm is never shipped as ours. `GHOSTTY_WASM_ALLOW_UNPINNED=1` lets you test a new pin's local build before its sha256 is recorded.
- A fresh pin has no asset until its PR merges and the publish workflow runs; until then a Mac ships the stock wasm (warned). Linux can `make build-ghostty-wasm` instead.

## Release and CI builds

`.github/actions/prepare` (used by the release workflows, e2e, and every other Linux job that builds the web UI) has `own-ghostty-wasm: true` by default. It fetches and verifies the published asset; if the pin has none yet (a pin-bump PR) it builds with Bazel and `--verify`. It then builds the web UI with `GHOSTTY_WASM_REQUIRE_OWN=1`, so a missing or mismatching wasm fails the build instead of shipping the stock one. Pass `own-ghostty-wasm: 'false'` to opt a job out.

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
