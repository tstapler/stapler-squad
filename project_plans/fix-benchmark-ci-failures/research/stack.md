# Stack Research: fix-benchmark-ci-failures

## Tool Versions

| Tool | Version | Source |
|---|---|---|
| buf | v1.41.0 | `.github/workflows/benchmark.yml` (all buf install steps) |
| Playwright | ^1.57.0 | `web-app/package.json` line 125 |
| ent ORM | v0.14.5 | `go.mod` line 19 (`entgo.io/ent v0.14.5`) |
| Node.js | 20 | benchmark.yml `setup-node` steps |

## GitHub Actions Runner Labels

All five benchmark jobs (`go-tier1`, `go-tier2`, `frontend-bundle`, `frontend-throughput`,
`e2e-latency`, `e2e-perf`) currently use `runs-on: ubuntu-latest`.

GitHub migrated `ubuntu-latest` to Ubuntu 26 on October 19, 2026. The pinned fallback
available as of this writing is `ubuntu-24.04` (Ubuntu 24 LTS). `ubuntu-22.04` is also
still available but further behind.

## Playwright and Ubuntu 26

Playwright 1.52 added Ubuntu 24 LTS support. Ubuntu 26 support status for Playwright
1.57 is not yet confirmed in the changelog available in this codebase. The safe, proven
fix is to pin Playwright-dependent jobs to `ubuntu-24.04` rather than waiting on a
Playwright release that explicitly claims Ubuntu 26 support.

Jobs that install Playwright (and therefore need the pinned runner):
- `frontend-throughput` — installs chromium via `npx playwright install --with-deps chromium`
- `e2e-latency` — same
- `e2e-perf` — same (runs from `tests/e2e`)

Jobs that do NOT install Playwright (can remain on `ubuntu-latest` or also be pinned for
consistency):
- `go-tier1` — Go benchmarks only, no Playwright
- `go-tier2` — Go benchmarks only, no Playwright
- `frontend-bundle` — builds and runs size-limit, no Playwright

## go-tier2 Missing Code-Gen Steps

Confirmed by reading `.github/workflows/benchmark.yml` lines 141–178:
`go-tier2` has only `checkout`, `setup-go`, `run full benchmark suite`, `save baseline`, and
`persist baseline` steps. It is missing both code-gen steps that `go-tier1` has:

```yaml
# Missing from go-tier2 (present in go-tier1):
- name: Install buf
  run: |
    curl -sSL "https://github.com/bufbuild/buf/releases/download/v1.41.0/buf-Linux-x86_64" \
      -o /usr/local/bin/buf && chmod +x /usr/local/bin/buf

- name: Generate protobuf code
  run: buf generate proto --template buf.gen.go-only.yaml

- name: Generate ent ORM code
  run: go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

Without these, `go test ./...` hits packages that import the missing generated
code and fails (likely with a build error that obscures the benchmark output).

## Terminal-Throughput Spec Timing

`web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts`:
- Overall test timeout: `test.setTimeout(300_000)` (5 minutes)
- Per-run completion wait: `waitForTestCompletion(page, 30_000)` (30 seconds per run)
- 10 total runs (2 warmup + 8 measured) with per-run page navigation

On slower Ubuntu 26 runners the 30-second per-run timeout is the likely culprit.
Increasing `waitForTestCompletion` to 60_000 (60s) gives headroom without risking
the 5-minute overall budget (10 × 60s = 600s > 300s, so the overall timeout would
also need to increase, e.g. to 600_000ms).

## ent Generate Command

The correct command (from CLAUDE.md and from go-tier1):
```
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```
The `--feature sql/upsert` flag is required; omitting it breaks `UpsertRule` and similar methods.
