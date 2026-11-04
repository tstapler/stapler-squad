# Bazel rules forks register

Forks of Bazel rules repositories, kept only when ADR-008 rule 3 applies. Each row needs an upstream PR or issue, a
removal condition and a remove-by date; `tools/bzl/check-fork-age.sh` (run by the weekly upgrade canary) fails when a
row is past its remove-by date or its `Fixed upstream in` cell is filled in (the removal condition is met).
Policy: [ADR-008](../../project_plans/bazel-conversion/decisions/ADR-008-rule-repo-fork-policy.md).

| Fork | Upstream link | Removal condition | Pinned commit | Added | Remove by | Fixed upstream in |
|---|---|---|---|---|---|---|

No forks are registered. Epic 0 chose workarounds for every spike (see `spike-results.md`).
