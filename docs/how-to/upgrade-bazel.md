# Upgrade Bazel, rules_go or Gazelle

The `bazel-version-upgrades` skill is the procedure; this page says where the repo's pieces are. Commands are
proposals until `tools/bzl/verified-commands.txt` lists them.

## What watches for you

`.github/workflows/bazel-upgrade-canary.yml` runs weekly and on manual dispatch. It tries the latest Bazel 9.x
through Bazelisk and the latest stable `rules_go` and `gazelle` from the Bazel Central Registry in a
discarded CI checkout, resolves `MODULE.bazel`, builds and tests the generated-independent subset, runs the pinned macOS job, and
opens an issue naming the first failing step. It also checks:

- `go.mod`'s `go` and `toolchain` lines are listed in `tools/bzl/supported-go.txt`
  (`bash tools/bzl/check-go-toolchain.sh`);
- every row of the [forks register](../reference/bazel-forks.md) is within its remove-by date and not yet fixed
  upstream (`bash tools/bzl/check-fork-age.sh`).

## Upgrading

1. Read the canary issue, or run the workflow by hand with `workflow_dispatch`.
2. Follow the `bazel-version-upgrades` skill: bump `.bazelversion` and the `bazel_dep` versions in `MODULE.bazel`,
   then resolve the lock.

   <!-- proposal -->
   ```bash
   bazel mod tidy
   ```
3. A Go toolchain bump also needs a line in `tools/bzl/supported-go.txt` (path-scoped rule
   `.claude/rules/bazel-go-toolchain.md`).
4. Run `make bazel-doctor` and a subset build before opening the PR. <!-- proposal -->
5. Name a fork only under [ADR-008](../../project_plans/bazel-conversion/decisions/ADR-008-rule-repo-fork-policy.md),
   with an upstream link and a removal condition.

The earlier narrow tmux Bazel path was removed in `b51b60eb1` because Bazel 9 broke the WORKSPACE mode it used; the
canary exists so a version change shows up as an issue before it breaks `main`.
