---
globs: ["**/BUILD.bazel","**/*.bzl","MODULE.bazel",".bazelrc"]
---

# Gazelle Owns BUILD Files; Do Not Hand-Edit Them

Go `BUILD.bazel` rules are generated. After changing imports or adding or removing a `.go` file, run `make gazelle` and commit the diff instead of editing `srcs`/`deps` by hand.

- **HandBuildFile exception.** Only the paths in `tools/bzl/hand-build-files.txt` are hand-written (generated-code, tmux, windows, web seams). Each starts with a header comment saying so and carries `# gazelle:ignore`. A new hand-written BUILD file needs a row in that list and the header.
- **Directives.** `# gazelle:` lines live in the root `BUILD.bazel`; changing one needs owner review (`/BUILD.bazel` is in CODEOWNERS).
- **`.bazelrc`.** Do not change `--jobs`, `--local_resources` or memory flags without reading the `bazel-perf` skill's memory math; `bash tools/bzl/check-bazelrc.sh` validates flag names.
- **No Bazel-only source changes** (new `//go:build bazel` files, source edits that only the Bazel build needs) until Epic 10 of `project_plans/bazel-conversion`.
- **Drift remedy** (also the line for the backlog dispatcher prompt): `make gazelle && bazel mod tidy && make sync`. `tools/ci/bazel-gates` applies it in CI, and `bazel-main-repair`/`bazel-autofix` push it; repair PRs carry the label `generated-file-race`.
- `MODULE.bazel` edits: use `bazel mod tidy`, never hand-edit `MODULE.bazel.lock`.

## Why

Hand-edited Gazelle output is silently overwritten by the next run, and a stale `deps` list fails as `missing strict dependencies` far from the cause. The skill `bazel-stapler-squad` has the commands and the error table.
