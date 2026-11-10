# Split a Go package into a smaller one

Use this when a cluster of files in `session/` or `server/services/` should become its own package so that a change inside it, or a change in the rest of the package, stops invalidating the other side. The measured reasons are in [package-size-under-bazel](../explanation/package-size-under-bazel.md); the decision record is `project_plans/bazel-conversion/decisions/ADR-010-package-scoping-refactors-for-bazel.md`. Every command below is a subcommand of `tools/bzl/scoping` (Go, Bazel-built); run it with `go run ./tools/bzl/scoping <cmd>` or `bazel run //tools/bzl/scoping -- <cmd>`.

## 1. Decide whether the split is worth it (criteria SC1 to SC6)

A cluster is split only when all of these hold (ADR-010 Decision 2). Evaluate against the tree you will land on, not an old measurement.

| Criterion | Check |
|---|---|
| SC1 boundary budget | At most 10 distinct non-test declarations the staying core uses from the cluster, and no import cycle. `scoping move --dry-run` prints one `WARNING: X (staying) uses moved symbol Y` line per use. |
| SC2 size | At least 3 files and 400 LOC; at most 25% of the original package. |
| SC3 isolation | At most 5 importing packages outside the original. |
| SC4 invalidation relevance | Its files appear in at least 3 of the last 500 `main` commits and some touch only them, or it has no import edge to the rest of its package. |
| SC5 slice dilution | Needs the slice manifest (`tests/e2e/slice-manifest.json`); `UNMEASURED` until Epic 12 produces it. |
| SC6 materiality | Mean affected library-LOC share lower by 0.10 percentage points, or test-LOC share by 0.30, or 3% of replayed changes affect less library LOC, or universal-slice touch share lower by 1.0 point. |

Measure SC6 by applying the move on a throwaway commit and comparing `scoping replay` with the same command on the parent (`scoping t9` compares with the stored baseline `tools/bzl/scoping/baseline.json`). A cluster that fails SC6 is recorded in `project_plans/bazel-conversion/implementation/scoping-ledger.md` as `skipped by criterion` with its numbers, not moved.

## 2. Check for conflicts

`git log --since=30.days -- <cluster files>` and the open PRs on the same files (`gh pr list --search "path:<dir>"`). A move rewrites every importer, so land it inside one working session.

## 3. Relocate what the core uses (separate commit)

When `scoping move --dry-run` reports core uses of the cluster, move those declarations out first. They are usually an interface, a callback type or a constant:

```
scoping relocate --dir server/services --to backlog_ports.go --decls SessionStopper,redactErrorDetail [--with-methods]
scoping deps --dir server/services --decls helperA,helperB   # what a declaration drags with it
```

`relocate` keeps doc comments, carries imports, and refuses to cross the `_test.go` boundary unless `--any-kind`. Build and test after this commit; it changes no package.

## 4. Move the files

```
scoping move --from server/services --to server/services/devtools --files a.go,a_test.go,b.go
scoping move --lower ...   # for a test-support cluster that the staying files import
```

`move` does `git mv`, re-points every reference with go/types, capitalises the unexported symbols the moved files use, and refuses a capitalisation that would collide with an existing declaration. Then `bazel run //:gazelle`, `bazel build //...` (runs nogo), and the tests of the moved and importing packages. Compare `go test -list '.*'` before and after: no test may disappear or be renamed. Update path-keyed code (`Makefile` guard file lists, `.golangci.yml`, `.claude/rules/*.md` analyzer file lists, tests that name source paths).

## 5. Keep it scoped

Add the package to `tools/bzl/scoping/budget.txt` and the boundary rules to `tools/bzl/scoping/boundaries.txt`; `make ready-scoping-budget` and `make ready-scoping-boundaries` enforce them. Raising a cap needs a `scoping-budget-raise: <reason>` line in a commit message.

## 5b. Pick candidates from the graph

`scoping touch [--files a,b | --per-file]` prints how many replayed production changes touch only the given files among the universal packages (the T9b gain of moving them). `scoping leaves --dir D` lists files nothing else in D uses; `--bases` lists files that use nothing else in D (lower layers). A leaf cluster still depends on the core and rarely passes SC6; a lower cluster that the core imports usually does, but count its importers against SC3.

## 6. Record the result

Append the measured numbers (before and after `scoping t9`) to the ledger, one row per cluster, executed or skipped.
