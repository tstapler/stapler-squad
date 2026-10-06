# Plan: notimesleeptest

## Architecture
New package `tools/lint/notimesleeptest` (mirrors norawgitopen). Run: for each file where `pass.Fset.File(pos).Name()` ends `_test.go`, inspect `*ast.SelectorExpr` (catches calls and func values); resolve `pass.TypesInfo.Uses[sel.Sel]` to `*types.Func` pkg `time` name `Sleep`; handle dot-imports via `*ast.Ident` too. Skip if package path contains `/tests/realtime` or nolint present (`nolintcomment.Contains(pass,pos,"notimesleeptest")`; require a non-empty reason if helper permits). Doc/ADR-003 update points to analyzer; remove or repoint `lint-no-sleep-tests`.

## Rollout (avoid red main)
1. Land analyzer + tests, registered in linter but **not yet** run on corpus in CI (or with a flag/baseline).
2. Migrate corpus in batches by package (PRs), each replacing sleeps with `testutil.WaitForCondition`/`require.Eventually`/channels/fake clock; genuine real-time → `tests/realtime/` or nolint+reason.
3. Final PR: enable blocking (`make lint-custom` step in GH lint.yml) once `make lint-custom` is clean.

## Tasks
1. Create analyzer + analysistest testdata (direct, aliased, dot-import, func value, non-test file, unrelated Sleep method, nolint with/without reason, realtime dir). [backend 3h]
2. Register in cmd/linter, update docs lists in main.go + Makefile description. [backend 0.5h]
3. Create `tests/realtime/` with README/doc.go stating the policy. [test 0.5h]
4. Verify root package coverage gap in lint-custom (excludes root pkg); include root `_test.go` or confirm none. [infra 1h]
5. Migrate `testutil/` (34 sites) first, since other packages depend on it. [test 4h]
6. Migrate `session/` tests. [test 8h]
7. Migrate `server/` + remaining packages (batch). [test 8h]
8. Triage genuinely real-time tests → `tests/realtime/` or nolint with reason. [test 3h]
9. Add `make lint-custom` step to `.github/workflows/lint.yml` go-lint job. [infra 1h]
10. Retire/repoint `lint-no-sleep-tests`; update ADR-003, CLAUDE.md/skill `deterministic-fast-tests`. [docs 1h]
11. Run `make lint-custom`, `make ci`; confirm GH lint workflow green. [test 1h]

## Adversarial review
- Blocking before migration would break all PRs → rollout ordering above is mandatory (tasks 9 last).
- Grep audit count differs from the issue (231/104 vs 237/108): the analyzer's own count is authoritative; record baseline after task 1.
- A nolint without reason would defeat the policy → enforce non-empty reason.
- Func-value `time.Sleep` and `time.After`-style busy waits evade a Sleep-only rule; out of scope, note as follow-up.
- Test files in external `_test` packages and build-tagged tests (integration) are only loaded if tags set; CI must run linter with relevant tags or they escape.
