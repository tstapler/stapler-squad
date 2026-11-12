# Replacing slow, duplicated CI with Bazel targets and shared Go actions (2026-11-11)

Read-only research by a planning agent, extending (not contradicting) `ci-cutover-plan.md`. Data came from `gh run list/view` on `tstapler/stapler-squad` and a read-only clone of `tstapler/gha-actions`; run minutes are `updatedAt - startedAt` and may include re-attempts. Everything marked UNVERIFIED was not checked.

## Where the time goes (medians, minutes)

| Workflow | PR run median / p90 | Push run median / p90 | What the time is |
|---|---|---|---|
| goreleaser-check | 30.7 / 36.8 | 38.8 / 63.9 | job 22.7: 19.7 is the `goreleaser release --snapshot` formula dry run, 2.2 the single-target smoke build |
| bazel.yml | 32.6 / 66.9 | 23.2 / 45.5 | full-graph build 24.0, tests 8.5, checkout 2.4; `bazel-guards` 10.0 (8.2 of it checkout) |
| build.yml | 31.1 / 34.4 | 42.2 / 70.8 | e2e shards 9-20 each, Go guards 10.9, e2e binary 4.4 |
| lint.yml | 17.5 / 18.8 | 18.1 | golangci-lint 7.8, new-code gate 2.6, custom analyzers 1.8, Jest 3.9 |
| main-ci | n/a | 47.5 / 60.2 | test-rest 15.6, test-tmux 8.5, five platform builds 16.8 together |
| mcp-integration | 11.2 / 18.0 | 10.4 | -race 7.0, services 3.3 |
| bazel-e2e | 12.8 / 17.3 | 15.3 / 26.5 | `bin` job 8.5, `plan` 3.0 |

Repeated setup: `setup-go-cached` in 13 workflows, `_prepare.yml`/`actions/prepare` in 11, `setup-bun` in 11, `build-tmux` in 4, `fetch-depth: 0` in 13. The same `stapler-squad` binary is compiled in the bazel job, bazel-e2e `bin` (5.5 + 1.5), build.yml e2e binary (2.8), the goreleaser smoke (2.2) and the Go-guards test compile (2.5): about 14 job-minutes outside the Bazel job. The three release binaries on five targets are rebuilt in the 19.7 minute dry run, again on every main push (build matrix 16.8) and again in the release job.

## Bazel replacement map

| Job or step | Verdict |
|---|---|
| Windows cross-compile | already in the `bazel` job (`tools/bzl/windows`) |
| main-ci five-platform build matrix (16.8 job-min) | small change: add linux/darwin cross targets for `//:stapler-squad`, build them in the `bazel` job, delete the matrix. Darwin cross needs the host `cc` (ADR-006): UNVERIFIED |
| custom lint (1.8) | covered by the nogo twins in `bazel build //...`; drop `make lint-custom` after a parity check |
| golangci-lint (10.4) | stay outside Bazel (large rule set; native tooling is the supported path) |
| race, sinkguard, gitoracle, MCP integration, release smoke | already moving into the `bazel` job (cutover stages S4a/S4b) |
| `goreleaser check` | stay outside Bazel (schema validation) |
| formula dry run (19.7) | small change: make `tools/bzl/checkformula` a Bazel `go_test` against a committed fixture `dist/` |
| release archives, Homebrew publish | stay outside Bazel (secrets, tag context, tap push) |
| coverage gate | UNVERIFIED: `bazel coverage` parity with the `-coverprofile` merge gate was not inspected |

GoReleaser: the PR-time job proves the schema, goreleaser's own pipeline (the otelc weave shim, `-X main.version`, 3 binaries x 3 OS x 2 arch, archive naming, brew formula rendering) and, through `checkformula`, that formula URLs and sha256 values equal the snapshot assets. A Bazel build would prove only compilation. ADR-005 recorded `keep-go-build`; `builder: prebuilt` is GoReleaser Pro. Recommended: keep GoReleaser as the release builder, let Bazel guard every PR, and move the 19.7 minute snapshot run to the release job, a weekly schedule and pushes that touch `.goreleaser.yaml` or `Formula/**`. A spike (not a plan step) is the OSS `tool:` shim calling `bazel build`; it would have to reproduce the `-X main.version` stamping via `--stamp` and map `ssq-mux` and `ssq-hooks`. Pro `prebuilt` is an owner decision.

## Shared actions in `tstapler/gha-actions`

**Correction (2026-11-11, owner):** the first pass inspected only the pushed `tstapler/gha-actions` and wrongly reported four actions as missing. They exist in the owner's local clone (`~/code/github.com/tstapler/gha-actions`, branch `feat/rust-bazel-publish`) as **seven commits on no remote**: `setup-bazel`, `setup-git-cliff`, `bazel-rust-package`, `generate-homebrew-formula`, their unit and real-runner selftests, a reusable `rust-bazel-release` workflow, and docs. Pushed state today: branches `main`, `feat/publish-homebrew-formula`, `feat/setup-buf-and-protoc`; tags `v0` and `v0.1.0`-`v0.1.2`; ten composites (`bazel-profile`, `go-diagnostics-upload`, `gradle-ci-memory`, `jvm-diagnostics-upload`, `mem-sampler`, `otel-trace`, `publish-homebrew-formula`, `runner-monitor`, `setup-buf`, `setup-protoc`) and no reusable workflows; stapler-squad uses only `mem-sampler` and `go-diagnostics-upload`. Until the branch is pushed and tagged, stapler-squad cannot pin these actions.

`setup-bazel` (unpushed) installs a pinned bazelisk as `bazel` (checksum-verified, no GitHub API calls), reads `.bazelversion`, optionally caches the disk and repository caches (`cache-key`, `cache-save` to restore-only on PRs) and can point Bazel at a remote cache. It overlaps what `bazel-contrib/setup-bazel` does in `bazel.yml` today.

**Owner decision: the Bazel CI composite goes in `gha-actions`.** Sketch of `bazel-ci`: wraps `setup-bazel` plus `bazel-profile` (bazelrc fragment, then upload on the way out), takes `config` (default `ci`), `cache-key`, `cache-save` (false on PRs once a main-written cache exists), `remote-cache`/`remote-upload`, `working-directory`, and an optional `command`; outputs the profile and BEP paths. It must not carry repo-specific flags: the CI resource caps and the `--lockfile_mode` live in the repo's `.bazelrc` `build:ci`. First consumer: `bazel.yml`, then `bazel-e2e.yml` and `bazel-parity.yml`, replacing `bazel-contrib/setup-bazel`; compare disk-cache hit rates before and after.

Candidates to move up from stapler-squad's local `.github/actions`: `cache-main-writes`, `setup-go-cached`, `build-tmux`; new ones: `gen-go-code` (buf + ent with a `generated-hash` output), `bazel-ci` (setup-bazel with disk cache and `--config=ci`, wrapping `bazel-profile`), `goreleaser-check` (pinned version, not `latest`), `gotestsum-run`. Use `setup-buf` in `prepare`/`prepare-stub` (fewer rate-limit failures, no minutes). Keep `prepare`, `prepare-stub`, `fetch-tymuxd`, `setup-fidelity-oracle` and `tools/ci` gates in this repo. Honest saving: moving setup removes duplicated YAML, not minutes; the minutes come from deleting jobs. Test each new action with a `selftest.yml` job in `gha-actions`, then a stapler-squad PR that pins the unmerged branch SHA, then tag and bump the SHA.

## Sequence and savings (job-minutes)

| # | Step | Removes | PR saving | Main saving | Risk |
|---|---|---|---|---|---|
| 1 | after S3: cross targets in the `bazel` job, delete the main-ci build matrix | 5 jobs on main | 0 | 16.8 | darwin cross UNVERIFIED |
| 2 | after S4b: `checkformula` as a Bazel `go_test`; snapshot run only on main/weekly/release | 19.7 dry run + 2.2 smoke from PRs touching `go.mod`/`go.sum`/release config | up to 21.9 per affected PR | about 19.7 on pushes touching Go files | a broken formula template is found at release or weekly time |
| 3 | `bazel-e2e` reuses the `bazel` job's outputs instead of its own 5.5 + 1.5 build | `bin` job | 7.0 | 7.0 | PR cache hits are not guaranteed (disk cache written on main only; remote cache waived) |
| 4 | `gha-actions` v0.2.0 with the shared composites; migrate one workflow at a time | none | 0 | 0 | low (refactor) |
| 5 | `setup-buf` in `prepare`/`prepare-stub` | none | about 0 | about 0 | low |
| 6 | after S9: drop `make lint-custom` once the nogo twins are proven | 1.8 | 1.8 | 1.8 | twins miss a case |
| 7 | spike only: `tool:` shim calling `bazel build` | none until proven | n/a | n/a | high |

Per main push: steps 1 + 2 + 3 is about 43.5 job-minutes (job-minutes, not wall clock; the critical path is test-rest at 15.6). A typical Go PR after S9: about 8.8; a PR touching `go.mod`: up to about 30.7.

## Owner questions

1. Is a weekly plus release-time formula dry run acceptable in place of one on every `go.mod` PR (the same trade S6 already accepted for Go files)?
2. Is GoReleaser Pro (`builder: prebuilt`) worth buying; if not, should the `tool:` shim spike run, or should the release stay on `go build`?
3. Resolved: the four actions are local and unpushed; the `bazel-ci` composite goes in `gha-actions` (owner, 2026-11-11). Remaining: push `feat/rust-bazel-publish` (review the seven commits first), tag it, and decide whether `bazel-ci` lives beside `setup-bazel` or replaces it.
4. Is the waived self-hosted remote cache (ADR-004, S10) coming back? Step 3 depends on cache sharing between jobs.
5. Conflict: ADR-009 and plan.md say S14 failed and Epic 12 is not released, so e2e stays a full run; the cutover plan (S8a/S9) makes Bazel-selected e2e one check with the full 5-shard run on main and nightly. Which governs?
