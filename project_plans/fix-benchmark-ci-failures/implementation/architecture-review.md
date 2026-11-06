# Architecture Review: fix-benchmark-ci-failures
**Date**: 2026-11-04
**Verdict**: CONCERNS

## Blockers
_(none)_

## Concerns

- [ ] **Story 1.1.1 (runner pinning) — dependency job left on `ubuntu-latest`.**
  `frontend-throughput`, `e2e-latency`, and `e2e-perf` are being pinned to `ubuntu-24.04`, but their shared `needs: frontend-bundle` dependency (line 185) stays on `ubuntu-latest`. When GitHub promotes `ubuntu-latest` to 26.04 (or later), `frontend-bundle` will build the web UI on a different OS than the three jobs that consume its artifacts and compare against baselines saved from pinned runners. The mismatch is benign today (artifact is a directory copy, not a runner-sensitive binary) but introduces a hidden inconsistency in the runner strategy. Recommendation: also pin `frontend-bundle` to `ubuntu-24.04` as part of Story 1.1.1 so the entire Playwright + frontend pipeline runs on the same OS class.

- [ ] **Stories 1.1.2 and existing tech debt — four-way duplication of the buf install curl command.**
  After this plan lands, four jobs (`go-tier1` line 68, `go-tier2` new, `e2e-latency` line 370, `e2e-perf` line 579) will all contain an identical hardcoded `curl … buf-Linux-x86_64 v1.41.0` step. A buf version bump will require touching four places with no mechanical check that all four were updated. The plan correctly extends an already-established pattern (not introducing new duplication) so this is not a blocker, but it sharpens an existing sync burden. Recommendation: extract a composite action at `.github/actions/go-codegen/action.yml` (Install buf → `buf generate proto --template buf.gen.go-only.yaml` → ent generate), or at minimum add a workflow-level `env.BUF_VERSION` to the top of `benchmark.yml` so a one-line change bumps all four jobs atomically.

## Nitpicks

- `set -o pipefail` on the go-tier2 `go test … | tee` command (Story 1.1.3) is semantically correct: `pipefail` returns the exit status of the last failing command in the pipeline, which is `go test` when it fails and `tee` succeeds. This matches the existing behavior of go-tier1 (line 85) and correctly makes test failures visible. No issue.
- The plan copies `buf generate proto --template buf.gen.go-only.yaml` (not the full `buf generate proto`) from go-tier1 to go-tier2. This is intentional and correct: go-tier2 is Go-only and does not install bun/node, so the full template's `protoc-gen-es` plugin would not be present. Copying from go-tier1 (not from e2e-latency/e2e-perf) avoids that subtle failure.
