# Maintain the tstapler/ghostty fork

Decision: we carry Ghostty patches in a fork (`github.com/tstapler/ghostty`, from `ghostty-org/ghostty`), test them in stapler-squad, then upstream them. Until `ssq/main` is pushed the Bazel build applies `third_party/ghostty-patches/` to an upstream archive; this document is the switch-over and the steady-state flow. Context: `docs/adr/ADR-039-own-ghostty-wasm-build.md`.

## Status (2026-11-08)

- The fork exists (`gh repo view tstapler/ghostty` reports `isFork: true`).
- `ssq/main` is prepared in a local clone at `~/code/github.com/tstapler/ghostty-ssq-staging`: upstream `5714ed07a1012573261b7b7e3ed2add9c1504496` plus four commits (the three patches in `third_party/ghostty-patches/`, then `README-SSQ.md`), tagged `ssq.1`, tip `38c7055f6ae361919d6f86c8521a0b9546783c25` (0002 was compile-fixed after the first draft). It is **not pushed**: the push was denied twice by the sandbox's permission classifier, so the owner must run the push below. Until then the fork's `ssq/main` does not exist and `MODULE.bazel` still points at upstream.

## Push the prepared branch (owner)

```bash
cd ~/code/github.com/tstapler/ghostty-ssq-staging
git push origin ssq/main ssq.1          # origin = https://github.com/tstapler/ghostty.git
```

The patch commits carry `Upstream-PR:` trailers; each topic is one commit.

## Switch stapler-squad to the fork

1. Get the archive hash: `curl -sSL https://github.com/tstapler/ghostty/archive/<ssq/main sha>.tar.gz | shasum -a 256`.
2. In `MODULE.bazel` set `GHOSTTY_ORG = "tstapler"`, `GHOSTTY_REV = "<sha>"`, `GHOSTTY_SHA256 = "<hash>"`, and `GHOSTTY_PATCHES = []`.
3. Delete `third_party/ghostty-patches/*.patch` and its `BUILD.bazel` (keep `README-SSQ.md` as a pointer; the patch-set id in the recorded key becomes `none`).
4. `make test-ghostty-wasm`, record the new sha256 line, run the differential harness.

No `third_party/ghostty` submodule: Bazel fetches the pinned fork commit as an archive and that directory is the Bazel package. To edit the fork in place, check it out anywhere, copy `third_party/ghostty/ghostty_src.BUILD` to `BUILD.bazel` in the checkout, and build with `--override_repository=ghostty_src=<checkout>`.

## Rules for `ssq/main`

- It is upstream at a chosen commit plus our patches as commits. One topic per commit, each with a Zig test and an `Upstream-PR: <url | none | not yet proposed>` trailer. Branch `ssq/main`, tags `ssq.N` (the go-git fork's convention; its `README-SSQ.md` could not be found when this was written, `gh api repos/tstapler/go-git/contents/README-SSQ.md` returned 404, so the fork's own `README-SSQ.md` is the index).
- The pin in stapler-squad is a commit SHA on `ssq/main`, never a branch name.
- Rollback: point `MODULE.bazel` back at upstream plus the patch files (restore them from git history), or delete the build output to fall back to stock ghostty-web.

## Sync with upstream

```bash
cd ~/code/github.com/tstapler/ghostty
git remote add upstream https://github.com/ghostty-org/ghostty.git   # once
git fetch upstream
git switch ssq/main && git rebase upstream/main     # or: git merge upstream/main to keep history
```

Drop any commit whose `Upstream-PR` has merged (the rebase will often do it for you). Then, in stapler-squad:

1. Read `minimum_zig_version` in the new `build.zig.zon`; update `ZIG_VERSION` and the four sha256 values in `MODULE.bazel`.
2. Push `ssq/main`, tag `ssq.N+1`, bump `GHOSTTY_REV`/`GHOSTTY_SHA256`.
3. `make test-ghostty-wasm`, run the differential harness, record the sha256.

Caution: Ghostty `main` and `v1.4.0` need Zig 0.17.0 and ship their own C API; coder's patch 0001 does not apply there (`patch --dry-run` against `v1.4.0` failed hunks in 5 of its 7 files, and `include/ghostty/vt/terminal.h` and `src/terminal/c/terminal.zig` already exist there). A sync past that point means porting `ghosttyWasm.ts` and the adapter to the native API, which is the Stage B wrapper work.

## Contribute upstream

1. `git switch -c upstream/<topic> upstream/main` (topic branch off upstream, not off `ssq/main`).
2. Cherry-pick the patch, port it to current `main`, run `zig build test -Dtest-filter=<test>`.
3. Open the PR against `ghostty-org/ghostty` from the fork; follow Ghostty's `CONTRIBUTING.md` (check its AI-assistance disclosure rules before submitting agent-written code).
4. Put the PR URL in the `Upstream-PR:` trailer on `ssq/main`. When it merges, drop our commit on the next sync.
