# Why the Windows cross targets are `manual` (dropped from the default graph)

Decision (owner, 2026-11-11): stop compiling and nogo-checking the Windows closure in the default `//...` graph.

**Cause.** Each `//tools/bzl/windows:stapler-squad_windows_*` target is the otelc weave under a Windows platform transition. The weave itself needs only source files, but its `libs` dependency pulls every stapler-squad package into the Windows configuration, and rules_go's nogo `_validation` output group then compiles and analyses all of them per arch. `bazel build --norun_validations` on the amd64 target ran 1 action instead of 1760. In CI run 43109172531 that duplicate work was about 555 CPU-s of `GoCompilePkg` plus 66 CPU-s of nogo (about 20% of 3140 CPU-s of action time), on top of two extra weaves.

**Change.** All cross binaries, their aliases and `pe_header_test` carry `tags = ["manual"]`, so `bazel build/test //...` skips them (`//tools/bzl/windows:all` now selects only `windows_only_probe`). `bazel aquery 'mnemonic("OtelWeaveHermetic", //...)'` goes from 3 actions to 1. They still build on demand by label, and the `Cross-compile Windows` step in `.github/workflows/bazel.yml` builds them and runs `pe_header_test` on push to main and `workflow_dispatch`, not on pull requests. `check-platform-targets.sh` (the incompatible-target canary) runs on every event as its own step.

**Coverage lost on pull requests.**
- nogo analysis of `*_windows.go` files and any package whose build is Windows-only.
- A PR that breaks the Windows cross-compile is caught after merge, not before.
- `pe_header_test` no longer runs on PRs.

To restore the PR check, remove the `if:` on the Cross-compile step (cost: the extra compile/nogo work above).
