# Validation
| AC | Covered by |
|---|---|
| 1 | FilterBar test: count/disabled/click; page wiring |
| 2 | bulkActions unit tests (all ok / partial) |
| 3 | slice tests; code review of store key |
| 4 | BacklogBulkActions test 0/1/N |
| 5 | page handler uses non-archived filter; helper test |
| 6 | dialog test (count, confirm, cancel) |
| 7 | unmount dispatch in page |
| 8 | `git diff --stat -- proto` empty |
| 9 | existing BacklogBoard/BacklogItemDetail/session tests still pass |
| 10 | new Jest files |
Pre-mortem: biggest risk is page.tsx colSpan/column regressions in existing page tests → run the backlog page tests. Readiness gate: PASS.
