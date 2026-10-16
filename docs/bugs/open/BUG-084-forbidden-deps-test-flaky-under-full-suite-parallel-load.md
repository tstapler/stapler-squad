# BUG-084: `TestNoForbiddenDependencies` Flakes Under `make test`'s Full-Suite Parallel Load [SEVERITY: Low]

**Status**: 🔓 Open
**Discovered**: 2026-08-20, while validating the `session`/`session/mux`/`session/tmux` `-p 1` scoping fix for the flaky-tmux-tests backlog item (see `docs/bugs/fixed/BUG-051-session-tmux-package-flaky-under-parallel-quick-check.md`).
**Impact**: `make test`'s second (unscoped) `go test` invocation intermittently fails the root package's `TestNoForbiddenDependencies` under full-suite parallel load. Does not affect runtime correctness — only test-suite reliability.

## Problem Description

A full `make test` run (`/tmp/make-test-5.log`, 2026-08-20) failed the root package (66.215s) with:

```
--- FAIL: TestNoForbiddenDependencies (60.03s)
    deps_guard_test.go:33: go list -deps ./... failed: signal: killed
FAIL
FAIL	github.com/tstapler/stapler-squad	66.215s
```

`deps_guard_test.go:28` wraps the `go list -deps ./...` subprocess in a fixed `context.WithTimeout(context.Background(), 60*time.Second)`. Under full-suite parallel load (many other packages' tests forking `go`/`git`/`tmux` subprocesses concurrently), that subprocess didn't finish within 60s and was killed on the context deadline.

Passes reliably in isolation:
```
go test -short . -run TestNoForbiddenDependencies -count=5 -v
Go test: 5 passed in 1 packages
```

Same failure shape as BUG-051 and BUG-083 (fixed wall-clock budget getting blown under `t.Parallel()`/full-suite scheduler contention), but in the root package, not covered by BUG-051's `-p 1` scoping of `session`/`session/mux`/`session/tmux`.

Confirmed unrelated to the diff in flight when this was found (Makefile/test-scoping changes for the flaky-tmux-tests backlog item — `git diff --stat main` touches no file in the dependency-scan path).

## Suggested Investigation

- Widen `deps_guard_test.go`'s 60s timeout, or exempt `go list -deps ./...` from full-suite parallel contention (e.g. mark the test to skip under `-short`, or move it into an isolated/serialized invocation like BUG-051's `-p 1` group).
- `go list -deps ./...` itself may simply be slow under CPU contention (this repo has grown substantially — 362-file diff vs. `main` includes a large generated `session/ent/` package) rather than genuinely hung; a longer timeout may be sufficient without further structural change.

## Related

- Filed per `.claude/rules/fix-flaky-tests-dont-defer.md` — found during BUG-051 remediation validation but out of scope to fix in that change (different package, would expand that change's blast radius).
- Same failure class as `docs/bugs/fixed/BUG-051-session-tmux-package-flaky-under-parallel-quick-check.md` and `docs/bugs/open/BUG-083-server-services-flaky-under-full-suite-parallel-load.md`.
