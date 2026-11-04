# Why package size matters under Bazel

A Go package is the unit of invalidation in the Bazel build: any change to one of its files rebuilds the library and every library and test that imports it, and the ADR-009 test slices are chosen at package granularity. `session/`, `server/services` and `config/` sit in nearly every slice, and `session` plus `server/services` are 92,000 and 91,700 non-test LOC, so a one-line change in either invalidates most of the repository. ADR-010 (`project_plans/bazel-conversion/decisions/ADR-010-package-scoping-refactors-for-bazel.md`) asked what moving code out of them buys.

What the measurements say (`tools/bzl/scoping`, `project_plans/bazel-conversion/implementation/scoping-ledger.md`):

- Test back-edges matter most. A package whose tests import a package that depends on it forces its test target to rebuild when the dependent changes. Moving the 12 inverted tests lowered the mean affected test-LOC share from 55.4% to 44.6% without moving any library code. `scoping inversions --allow` keeps that at zero.
- Moving a leaf cluster out of a hub package lowers the library-LOC share only when the cluster is large or churns. Most of the eleven small clusters measured move no number and are left alone; `skipped by criterion` rows in the ledger carry the figures.
- Count-based fan-out rises when upper packages are split, because the core now has more dependents. That is why the gate is the target model share, not the raw dependent count.
- A split whose tests share helpers with the core needs a test-support package first: a helper in a `_test.go` file is invisible to another package, and a support package that imports the core cannot be imported by the core's own tests.

The budget (`tools/bzl/scoping/budget.txt`) and boundary rules (`boundaries.txt`) keep the new packages from regrowing; [split-a-go-package](../how-to/split-a-go-package.md) is the procedure.
