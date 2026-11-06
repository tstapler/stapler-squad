# Implementation Plan: fix-benchmark-ci-failures

**Feature**: Fix three root-cause failures in the benchmark CI workflow persisting since Oct 31, 2026
**Date**: 2026-11-04
**Status**: Ready for implementation
**ADRs**: None

---

## Domain Glossary

| Term | Definition |
|---|---|
| `ubuntu-24.04` | GitHub Actions runner label for Ubuntu 24 LTS; stable replacement for `ubuntu-latest` after its Oct 19, 2026 migration to Ubuntu 26 |
| `ubuntu-latest` | GitHub Actions runner label; now resolves to Ubuntu 26 as of Oct 19, 2026 — Playwright 1.57's `--with-deps` incompatible with its apt package set |
| `runs-on` | GitHub Actions YAML keyword declaring the runner image for a job |
| `buf generate` | Protobuf code generation command; `buf.gen.go-only.yaml` generates only Go/ConnectRPC output (no Node plugins) |
| `ent generate` | ORM schema code generation command; `--feature sql/upsert` flag is mandatory in this repo (per CLAUDE.md) |
| `go-tier2` | Benchmark workflow job running the full Go benchmark suite (`./...`); push-to-main only |
| `set -o pipefail` | Bash option that propagates a non-zero exit from any pipe segment through `tee`, preventing silent build failures |
| `waitForTestCompletion` | Playwright helper in `terminal-stress/helpers` that polls for benchmark run completion with a per-run timeout argument |
| `test.setTimeout` | Playwright method setting the total wall-clock timeout for the enclosing test block |
| Playwright-dependent jobs | The three workflow jobs requiring a Chromium browser: `frontend-throughput`, `e2e-latency`, `e2e-perf` |
| `alert-on-main-failure` | Workflow job that opens/comments on GitHub issue #1362 on any main-branch benchmark failure |

---

## Pattern Decisions

| Decision | Chosen | Alternative Rejected | Reason |
|---|---|---|---|
| Runner pinning strategy | Pin `ubuntu-24.04` explicitly on the 3 Playwright jobs | A: Docker custom image with pinned Ubuntu 24, Playwright, buf, ent pre-installed | Docker adds image maintenance burden and significant build infrastructure overhead — the fix is 3 one-line changes |
| Runner pinning strategy | Pin `ubuntu-24.04` explicitly on the 3 Playwright jobs | B: Upgrade Playwright to a version with confirmed Ubuntu 26 support | Ubuntu 26 Playwright support is unconfirmed at Playwright 1.57; speculative and defers the fix |
| `go-tier2` missing code-gen | Copy buf + ent steps verbatim from Tier 1 | Running `make build` target | `make build` would also run lint/proto/ent but its dep graph isn't suited for a benchmark-only job and adds unrelated step failure modes |
| Throughput timeout fix | Raise `waitForTestCompletion` to 60s, `test.setTimeout` to 600s | Reduce `TOTAL_RUNS` from 10 to 5 | Reducing runs degrades statistical validity of the benchmark; raising the timeout preserves measurement quality |

---

## Tech Debt Disposition

None identified.

---

## Migration Plan

N/A — no schema or data changes.

---

## Observability Plan

After the fix merges, the `alert-on-main-failure` job will stop commenting on issue #1362. That issue going quiet is the primary observable signal that all three fixes are effective. No new metrics, dashboards, or log lines are added.

---

## Risk Control

| Risk | Likelihood | Mitigation |
|---|---|---|
| `ubuntu-24.04` deprecated before next review | Low — GHA policy requires 60-day notice | No action needed; deprecation notice will surface in GHA announcements |
| buf install step adds wall-clock time to Tier 2 | Low — ~30s; Tier 2 is already a 50-min job | Go module cache (`cache: true`) covers `ent generate`; curl download of buf is ~5s |
| `test.setTimeout(600_000)` still too tight if rendering degrades further | Low — 60s × 10 runs = 600s = exact cap | The pitfalls research flagged this; 600_000 is set to match the new 60s × 10 ceiling exactly, but the per-run slack is now 2× the old limit — any single run up to 60s passes |

---

## Unresolved Questions

None — all three root causes are confirmed and fixes are specified.

---

## Dependency Visualization

```
Phase 1 (all tasks independent — no cross-task dependencies):

  Task 1.1.1  ──┐
  Task 1.1.2  ──┤── all edits to benchmark.yml (single file, serial within story)
  Task 1.1.3  ──┤
  Task 1.1.4  ──┘

  Task 1.2.1  ──── edit terminal-throughput.spec.ts (independent file)
```

---

## Phase 1: Repair benchmark.yml and terminal-throughput spec

### Epic 1.1: Fix .github/workflows/benchmark.yml

**Goal**: All three root causes in the workflow file are addressed in a single atomic commit.

**Acceptance Criteria mapping**:
- AC-1: `ubuntu-24.04` on `frontend-throughput`, `e2e-latency`, `e2e-perf`
- AC-2: buf install + `buf generate proto --template buf.gen.go-only.yaml` + `ent generate` added to `go-tier2` before the benchmark run step
- AC-5: `set -o pipefail` added to the `go-tier2` run step

---

#### Story 1.1.1 — Pin Playwright jobs to ubuntu-24.04

**File**: `.github/workflows/benchmark.yml`
**Size**: XS

Change `runs-on: ubuntu-latest` to `runs-on: ubuntu-24.04` on three jobs:
- Line 231: `frontend-throughput`
- Line 343: `e2e-latency`
- Line 547: `e2e-perf`

The two non-Playwright Go jobs (`go-tier1` at line 51, `go-tier2` at line 143) and `frontend-bundle` at line 187 may optionally be pinned too for reproducibility, but are not required by any AC.

**Given-When-Then (AC-1)**:
- Given: A push lands on main after this fix is merged
- When: The `frontend-throughput`, `e2e-latency`, and `e2e-perf` jobs start
- Then: Each job runs on an Ubuntu 24 runner where `npx playwright install --with-deps chromium` succeeds on the first attempt within the 8-minute timeout

**Tasks**:

| # | Description | File | Lines | Size |
|---|---|---|---|---|
| 1.1.1a | Change `frontend-throughput` `runs-on` from `ubuntu-latest` to `ubuntu-24.04` | `.github/workflows/benchmark.yml` | 231 | XS |
| 1.1.1b | Change `e2e-latency` `runs-on` from `ubuntu-latest` to `ubuntu-24.04` | `.github/workflows/benchmark.yml` | 343 | XS |
| 1.1.1c | Change `e2e-perf` `runs-on` from `ubuntu-latest` to `ubuntu-24.04` | `.github/workflows/benchmark.yml` | 547 | XS |

---

#### Story 1.1.2 — Add code-generation steps to go-tier2

**File**: `.github/workflows/benchmark.yml`
**Size**: S

Insert three new steps in `go-tier2` (currently at lines 141–178) between "Set up Go" (line 153) and "Run full benchmark suite" (line 158). The steps are identical to Tier 1's steps at lines 67–81.

**Given-When-Then (AC-2)**:
- Given: A push lands on main after this fix is merged
- When: The `go-tier2` job runs `go test -bench=. ... ./...`
- Then: The packages `session/ent/backlogstage`, `session/ent/backlogprogressnote`, and `session/ent/backlogitemdependency` are present (ent-generated) and the benchmark compiles and runs successfully instead of failing silently

**Tasks**:

| # | Description | File | Insert after line | Size |
|---|---|---|---|---|
| 1.1.2a | Add "Install buf" step (curl download of buf v1.41.0) after "Set up Go" in `go-tier2` | `.github/workflows/benchmark.yml` | 157 | XS |
| 1.1.2b | Add "Generate protobuf code" step (`buf generate proto --template buf.gen.go-only.yaml`) after Install buf | `.github/workflows/benchmark.yml` | after 1.1.2a | XS |
| 1.1.2c | Add "Generate ent ORM code" step (`go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`) after Generate protobuf | `.github/workflows/benchmark.yml` | after 1.1.2b | XS |

Exact YAML for the three steps (copy verbatim from Tier 1, lines 67–81):

```yaml
      - name: Install buf
        run: |
          curl -sSL "https://github.com/bufbuild/buf/releases/download/v1.41.0/buf-Linux-x86_64" \
            -o /usr/local/bin/buf && chmod +x /usr/local/bin/buf

      - name: Generate protobuf code
        # Go-only template: this job never installs bun/node (it doesn't need
        # a web UI build), so the default buf.gen.yaml's local protoc-gen-es
        # plugin (web-app/node_modules/.bin/protoc-gen-es) isn't present and
        # `buf generate proto` would fail outright. buf.gen.go-only.yaml
        # generates only the Go/ConnectRPC output these target packages need.
        run: buf generate proto --template buf.gen.go-only.yaml

      - name: Generate ent ORM code
        run: go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

---

#### Story 1.1.3 — Add set -o pipefail to go-tier2 run step

**File**: `.github/workflows/benchmark.yml`
**Size**: XS

The existing `go-tier2` "Run full benchmark suite" step (line 158–166) uses `go test ... | tee`. Without `set -o pipefail`, a non-zero exit from `go test` is masked by `tee`'s success. Add `set -o pipefail` as the first line of the run block, mirroring Tier 1 (line 85).

**Given-When-Then (AC-5)**:
- Given: A go package fails to build in `go-tier2` (e.g. a generated file is missing)
- When: `go test ... | tee tier2-bench.txt` runs
- Then: The step exits non-zero and the CI job fails visibly, instead of masking the error through `tee`

**Tasks**:

| # | Description | File | Lines | Size |
|---|---|---|---|---|
| 1.1.3a | Add `set -o pipefail` as first line of `go-tier2`'s "Run full benchmark suite" run block | `.github/workflows/benchmark.yml` | 159 | XS |

Current run block (lines 158–166) becomes:
```yaml
      - name: Run full benchmark suite
        run: |
          set -o pipefail
          go test \
            -bench=. \
            -run='^$' \
            -benchmem \
            -count=8 \
            -timeout=30m \
            ./... | tee tier2-bench.txt
```

---

### Epic 1.2: Fix terminal-throughput spec timeouts

**Goal**: The terminal-throughput benchmark does not time out on ubuntu-24.04 CI runners.

**Acceptance Criteria mapping**:
- AC-4: `waitForTestCompletion` timeout raised from 30s to 60s, and `test.setTimeout` raised from 300s to 600s, so a single run taking up to 60s does not exhaust the outer test limit

---

#### Story 1.2.1 — Raise waitForTestCompletion and test.setTimeout

**File**: `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts`
**Size**: XS

Two constant values need updating:
- Line 44: `test.setTimeout(300_000)` → `test.setTimeout(700_000)` (700s provides ~100s overhead slack above 60s × 10 runs; per pre-mortem FM-1, 600_000 had zero overhead slack)
- Line 64: `waitForTestCompletion(page, 30_000)` → `waitForTestCompletion(page, 60_000)`

**Given-When-Then (AC-4)**:
- Given: The `frontend-throughput` CI job runs on ubuntu-24.04 (after Story 1.1.1a)
- When: The terminal-throughput spec runs 10 benchmark iterations on a CI runner that takes up to 60s to render each 100KB payload
- Then: All 10 iterations complete within the 700s outer timeout (60s × 10 + ~100s overhead slack) without triggering a Playwright assertion timeout, and `benchmark-results.json` is produced

**Tasks**:

| # | Description | File | Line | Size |
|---|---|---|---|---|
| 1.2.1a | Change `test.setTimeout(300_000)` to `test.setTimeout(700_000)` | `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts` | 44 | XS |
| 1.2.1b | Change `waitForTestCompletion(page, 30_000)` to `waitForTestCompletion(page, 60_000)` | `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts` | 64 | XS |

**Note**: The comment on line 43 reads "each run takes ~5–15s depending on rendering speed". Update it to "each run takes ~5–60s depending on rendering speed (CI canvas renderer on ubuntu-24.04)" to document the new upper bound.

---

## Summary

| Phase | Epics | Stories | Tasks |
|---|---|---|---|
| Phase 1 | 2 | 4 | 8 |
| **Total** | **2** | **4** | **8** |

All 8 tasks target exactly 2 files:
1. `.github/workflows/benchmark.yml` — 6 tasks (runner pins ×3, code-gen steps ×3, pipefail ×1 = 7 edits grouped across 4 stories into 6 task IDs; the 3 code-gen steps are delivered as one logical insertion)
2. `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts` — 2 tasks

No new files. No schema changes. No dependency changes. The fix is a single conventional-commit PR: `fix(ci): pin Playwright jobs to ubuntu-24.04, add code-gen to tier2, raise throughput timeouts`.
