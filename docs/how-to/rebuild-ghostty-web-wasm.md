# Rebuild the ghostty-web WASM with native DECSTR

**Status: patch written, not yet compiled or tested.** Building needs a downloaded Zig toolchain, and the agent session that wrote this was not permitted to run it. Until it is built and passes the differential harness, the adapter-level shim (`web-app/src/lib/terminal/ghosttySoftReset.ts`) stays the fix.

## Why

ghostty-web 0.4.0 embeds `ghostty-vt.wasm`, built from Ghostty commit `5714ed07a1012573261b7b7e3ed2add9c1504496` (2025-12-01) plus `patches/ghostty-wasm-api.patch` from [coder/ghostty-web](https://github.com/coder/ghostty-web). That commit's stream parser logs `ignoring unimplemented CSI p with intermediates: !` for DECSTR (`CSI ! p`). Upstream Ghostty `main` dispatches a `soft_reset` action to `Terminal.softReset` (`src/terminal/stream.zig`, `src/terminal/Terminal.zig`), and 0.4.0 is still npm `latest` and `next` (checked 2026-11-08 with `npm view ghostty-web dist-tags`).

`web-app/vendor/ghostty-vt/ghostty-vt-decstr.patch` backports that to the pinned commit: a `soft_reset` stream action, the `CSI ! p` dispatch, `Terminal.softReset` (with a Zig test), and the handler arms in `stream_readonly.zig` and `c/terminal.zig`. It applies after ghostty-web's own patch.

## Consuming it without forking the npm package

`Terminal` accepts a `ghostty?: Ghostty` option and `Ghostty.load(wasmPath)` takes a path, so squad can load a rebuilt WASM from its own static assets and pass it in. No fork of ghostty-web's JavaScript is needed, and stock 0.4.0 stays the fallback.

## Build (UNVERIFIED; run on a machine where fetching Zig is acceptable)

1. Zig 0.15.2 (the pinned Ghostty's `minimum_zig_version`). Official tarball: `https://ziglang.org/download/0.15.2/zig-aarch64-macos-0.15.2.tar.xz`, sha256 `3cc2bab367e185cdfb27501c4b30b1b0653c28d9f73df8dc91488e66ece5fa6b` (matches `https://ziglang.org/download/index.json`; recheck before use).
2. Fetch sources into a scratch directory outside the repo:
   - Ghostty at the pinned commit: `gh api repos/ghostty-org/ghostty/tarball/5714ed07a1012573261b7b7e3ed2add9c1504496 > g.tar.gz`
   - ghostty-web's `patches/ghostty-wasm-api.patch` from tag `v0.4.0` (or the `640db461` commit).
3. In the extracted Ghostty tree: `patch -p1 < ghostty-wasm-api.patch && patch -p1 < <repo>/web-app/vendor/ghostty-vt/ghostty-vt-decstr.patch`.
4. `zig build test -Dtest-filter=softReset` (the new test), then `zig build lib-vt -Dtarget=wasm32-freestanding -Doptimize=ReleaseSmall`; the output is `zig-out/bin/ghostty-vt.wasm`.
5. Check it: load it in `web-app/src/lib/terminal/__tests__/differential/harness.ts` via `Ghostty.load(<path>)` with the adapter's rewriter disabled (`softReset: false`) and confirm the DECSTR curated streams still match xterm.js and the log has no `unimplemented CSI p`.
6. Vendor the result with its sha256 (and the Zig and Ghostty versions) next to the patch, and have `GhosttyTerminal.tsx` pass `ghostty: await Ghostty.load("/ghostty-vt.wasm")`.

Compile errors are likely in the Zig test (field types such as `style_id`, `scrolling_region`, the `setAttribute` payload were written from reading the source, not compiled); fix those first.

## Rollback

Remove the `ghostty:` option so ghostty-web falls back to its embedded stock WASM, then delete the vendored binary. The adapter shim keeps DECSTR working on stock 0.4.0, so no behaviour is lost.

## Bumping

When a ghostty-web release ships a newer Ghostty, re-run the differential harness against it first: if it includes `softReset` the vendored WASM and `ghosttySoftReset.ts` can both go; the `it.failing` tests in `ghosttyVsXterm.test.ts` and `ghosttyFreedMemory.test.ts` turn red when an upstream fix lands.
