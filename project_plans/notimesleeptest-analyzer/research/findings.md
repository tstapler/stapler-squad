# Research findings (all VERIFIED by command in this worktree)

- Analyzer pattern: `tools/lint/norawgitopen/analyzer.go` — `analysis.Analyzer` + `inspect`, type-resolves callee, uses `internal/nolintcomment.Contains(pass, pos, name)`; tests via `analysistest` with `testdata/src`. Registered in `tools/lint/cmd/linter/main.go` (multichecker).
- `make lint-custom` (Makefile ~L814) runs the linter on `go list ./...` minus root pkg; `lint` depends on it. Multichecker loads test files by default. NOTE: root package excluded -> root-package `_test.go` files are not linted; must check/handle.
- `.github/workflows/lint.yml` go-lint job runs golangci-lint only; no `make lint-custom` step -> add one (needs generated files artifact, already downloaded there).
- Corpus: `grep 'time\.Sleep('` over `*_test.go` (excl. web-app) = 231 matches in 104 files; 34 of those in `testutil/` (currently exempted by Makefile target; the issue says 237/108 — count differs slightly by grep scope/comments). Includes `testutil/*_test.go` and helper code in non-_test files (testutil/wait.go is not a _test file -> not covered by analyzer; fine).
- Existing wait helpers: `testutil/wait.go` (`WaitForCondition`), `testutil/wait/`, `testutil/expect.go`. Prefer these / `require.Eventually` / channels / injected clocks.
- `tests/` has no Go real-time dir yet (`tests/demo, e2e, snapshots`); `tests/realtime/` must be created.
- Existing precedent: `deterministic-fast-tests` skill, `novartestseam` analyzer.
- Aliased-import and dot-import: resolve via `pass.TypesInfo.Uses` -> `*types.Func` with Pkg().Path()=="time" && Name()=="Sleep". Also consider `time.Sleep` passed as a value (func ref, e.g. `sleep := time.Sleep`) — flag SelectorExpr uses, not only calls.
- Package-scoped exemption: match on pkg path containing `/tests/realtime`, or file path; for external test pkgs (`_test` suffix) normalize.

## Risks
- 231 sites is large: migration must be staged (analyzer lands non-blocking or with temporary baseline) to avoid a red main.
- Some sleeps are legit (testing rate limiters, real tmux timing, negative assertions "nothing happens"); route to `tests/realtime/` or nolint with reason.
- Moving tests across packages breaks access to unexported identifiers → prefer nolint for those.
- CI time: lint-custom builds the linter binary; acceptable.
