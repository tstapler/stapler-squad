# Verification — test-log-isolation

The slog seam (`log.SetSlogDefaultForTest`) landed in main via PR #672 (`a4cae21aa`). This records the AC2 check that an earlier review could not complete (its run was OOM-killed, exit 137).

Run at HEAD `cb0cdd514` (tree identical to `origin/main`), 2026-09-21:

| Check | Command | Result |
|---|---|---|
| AC2 race suite | `go test -race -count=2 -p 1 -parallel 4 -timeout=25m ./server/services/...` | `ok` in 653s, exit 0, 0 `DATA RACE` occurrences |
| AC2 lint | `make lint` | custom lint ok, shellcheck ok, 0 issues |

`-p 1 -parallel 4` bounds memory; the unbounded run is what OOM-killed the reviewer.
