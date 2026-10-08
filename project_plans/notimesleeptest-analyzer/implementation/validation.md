# Validation

| AC | Covered by |
|---|---|
| 1 direct/aliased fails | analysistest cases (task 1) |
| 2 non-test / unrelated Sleep ok | analysistest cases |
| 3 only realtime + nolint exempt | analysistest cases + `tests/realtime/` (task 3) |
| 4 analyzer tests | task 1 |
| 5 GH Actions enforces | task 9, verified on PR lint run |
| 6 corpus migrated | tasks 5-8, `make lint-custom` clean |

## Pre-mortem
- Build-tagged tests escape lint → run linter with tags or document gap.
- Root package excluded by Makefile filter → task 4.
- Mass migration introduces flakiness → each batch run with `-race -count=3` on touched packages.
- Reviewers add new sleeps during migration → keep grep target as interim guard until blocking.
