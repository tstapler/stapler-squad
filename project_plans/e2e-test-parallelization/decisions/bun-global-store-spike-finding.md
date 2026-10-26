# Finding: Bun 1.4 global-store linker timing spike (Story 2.2.1)

Measured in a scratch copy of `tests/e2e` (`/tmp/e2e-bun-spike-scratch`, deleted
after this spike) using Bun 1.4.3, `rm -rf node_modules` before each run.
`tests/e2e`'s devDependency tree is small: 7 top-level packages, 309-311
resolved packages.

**Baseline (no `bunfig.toml`, default linker):**
- Run 1: `993.00ms` installed / `0.996s` wall
- Run 2: `219.00ms` installed / `0.221s` wall

**With `[install]\nlinker = "isolated"\nglobalStore = true` added to the
scratch copy's `bunfig.toml`:**
- Run 1: `162.00ms` installed / `0.165s` wall
- Run 2: `5.00ms` installed ("Checked 310 installs across 311 packages (no
  changes)") / `0.007s` wall

**Delta:** ~57ms (run 1) to ~214ms (run 2, steady-state warm-cache reinstall
— the more realistic proxy for repeated CI frozen installs). Both the
baseline and the isolated-linker runs land well under 1 second on this
tree either way; Bun's package-level download cache (`~/.bun/install/cache`)
is shared across both linker modes and was already warm from the earlier
baseline runs by the time the isolated-linker runs executed, so run 1's gap
partly reflects that shared warm cache rather than the global store itself.

**Verdict: no-go** — cosmetic win (sub-second delta in both absolute and
relative terms), not worth a `bunfig.toml` change. Consistent with the
vendor's stated "7x" claim being aimed at large dependency trees; this
repo's `tests/e2e` devDependency tree (7 packages) is too small for the
isolated-linker/global-store strategy to matter. The real `tests/e2e/bunfig.toml`
was not touched — no such file existed before or after this spike; only the
scratch copy's `bunfig.toml` was created/edited.
