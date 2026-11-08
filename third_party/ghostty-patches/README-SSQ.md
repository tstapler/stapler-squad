# Ghostty patch series (applied to the pin in MODULE.bazel)

Applied in the order listed in `MODULE.bazel` (`GHOSTTY_PATCHES`) with system `patch -p1`. Changing any file changes the `patchset12` part of the key in `scripts/ghostty-wasm.sha256`, so record a new sha256.

| Patch | Topic | Upstream status |
|---|---|---|
| 0001-wasm-api.patch | coder/ghostty-web `patches/ghostty-wasm-api.patch` at `v0.4.0`, unchanged | none |
| 0002-decstr-soft-reset.patch | DECSTR (`CSI ! p`) via `Terminal.softReset`, with Zig test | backport of upstream `softReset` |
| 0003-scrub-freed-pages-on-wasm.patch | zero freed page memory on wasm | not yet proposed |

These same patches are the commits on `tstapler/ghostty` `ssq/main` once pushed; then this directory is deleted. See `docs/how-to/ghostty-fork-workflow.md` and `docs/explanation/own-ghostty-wasm-build.md`.
