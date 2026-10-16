# Architecture Research: CI Job Graph, Duplication, and Failure Modes

Research Agent 3 (Architecture) — ci-speed. All claims below are VERIFIED by reading the
cited file/line or running the cited command, except where marked INFERRED.

## 1. What generates what, and where it's shared vs. re-run from scratch

Two build-time artifacts are consumed across most workflows:

- **Generated code**: `gen/proto/go/**`, `web-app/src/gen/**` (via `buf generate`), and
  `session/ent/*.go` (via `go run entgo.io/ent/cmd/ent generate --feature sql/upsert
  ./session/ent/schema`). Both are gitignored (`.gitignore:31,36-41`) — confirmed zero
  tracked files under `gen/proto/` or `session/ent/*.go` (excluding `schema/` and
  `generate.go`) via `git ls-files`. Every workflow that touches Go code importing these
  packages must regenerate them fresh; nothing can rely on a checked-in copy.
- **`server/web/dist/`**: the Next.js static export, required only because
  `server/web/embed.go:8` does `//go:embed all:dist` — the Go binary won't compile without
  *some* non-empty `dist/` directory present, but **compiling does not require a real
  Next.js build**. `lint.yml:109-112` proves this: it writes a one-line stub
  (`mkdir -p server/web/dist && printf '<!DOCTYPE html>...' > index.html`) and that's
  sufficient for `go build ./...`'s import-cycle check.
- **Pinned tmux 3.4** (`bin/tmux`, `Makefile:249` `BIN_TMUX`): built by
  `scripts/build-tmux.sh` from the `third_party/tmux` submodule (autoconf/automake +
  `make -j$(nproc)`, `scripts/build-tmux.sh:137`). Only needed by test/benchmark jobs that
  actually spawn tmux sessions.

### The one already-correct build-once/fan-out pattern in this repo

`build.yml`'s `prepare` job (`build.yml:75-107`) runs `buf generate` + `ent generate` +
the full `pnpm run build` **once**, then `actions/upload-artifact@v4` uploads
`gen/`, `session/ent/`, `server/web/dist/` as `generated-files` (1-day retention). Six
downstream jobs in the *same workflow* download it instead of regenerating:
`test`, `pty-race-regression`, `integration-coverage`, `build` (6-way GOOS/GOARCH matrix),
`install-check`, `benchmark-gate`. This is the pattern the requirements ask to generalize —
it already exists, just scoped to one workflow file.

### Everywhere else, the same generation is re-run from scratch, independently

| Workflow / job | Regenerates proto? | Regenerates ent? | Builds real Next.js UI? | Builds tmux? |
|---|---|---|---|---|
| `build.yml` → `prepare` | Yes (once) | Yes (once) | Yes (once) | No |
| `build.yml` → `web-build-smoke` | Yes (own copy, via `make web-build`) | n/a | Yes (own copy) | No |
| `build.yml` → `test`/`pty-race-regression`/`integration-coverage`/`benchmark-gate` | downloads artifact | downloads artifact | downloads artifact | **Yes, each job independently** (see §1a) |
| `lint.yml` (single job) | Yes (own copy, `buf generate proto`) | Yes (own copy) | **No — stub only** | No |
| `mcp-integration.yml` | Yes (own copy, via `./.github/actions/prepare`) | Yes (own copy) | **Yes, full build — but only a stub is actually needed** (see §2) | No |
| `e2e-video.yml` (2-way shard matrix) | Yes — **twice**, once per shard | Yes — twice | Yes — **twice** (full build, once per shard) | No |
| `demo-publish.yml` | Yes (own copy) | Yes (own copy) | Yes (own copy) | No |
| `goreleaser-check.yml` → `build-smoke-test` | Yes (own copy) | Yes (own copy) | Yes (full build — only needs the binary to compile+run) | No |
| `ux-analysis.yml` | Yes (own copy) | Yes (own copy) | Yes (full build) | No |
| `registry-validation.yml` | Yes (own copy) | Yes (own copy) | Yes (full build) | No |
| `benchmark.yml` → `go-tier1` | **No — never generates anything** | **No** | n/a | No |
| `benchmark.yml` → `go-tier2`/`frontend-*`/`e2e-latency` | `frontend-bundle`/`throughput`/`lighthouse`/`e2e-latency` each run `buf generate proto` (proto only); `go-tier2` generates nothing | `e2e-latency` only | `frontend-bundle`/`throughput`/`lighthouse`/`e2e-latency` each do a full `pnpm run build` independently | No |
| `generated-proto-guard.yml`, `backlog-scaffolding-guard.yml` | No (git-diff-only checks, no build) | No | No | No |

**Every workflow above the guard rows independently pays for `pnpm install` + `buf
generate` + (usually) a full `next build`.** None of them share `build.yml`'s
`generated-files` artifact — GitHub Actions artifacts are scoped to the run that produced
them, so cross-*workflow* sharing would need `actions/download-artifact`'s
`run-id`/`github-token` cross-run form (not used anywhere today) or a different mechanism
entirely (remote cache, `actions/cache`, or restructuring so these jobs live in one
workflow). `ux-analysis.yml:61-65` even has a dead `actions/download-artifact` step with
`continue-on-error: true` that can never succeed (this workflow has no `prepare` job
producing that artifact in the same run) — the real generation already happened two steps
earlier via `./.github/actions/prepare`; the download step is inert dead weight.

### §1a — tmux build: shared by cache key, not by artifact, and stampede-prone

`test`, `pty-race-regression`, `integration-coverage`, and `benchmark-gate` in `build.yml`
each independently do:
```yaml
- uses: actions/cache@v4
  id: tmux-cache
  with: { path: bin/tmux, key: tmux-3.4-${{ runner.os }}-v1 }
- if: steps.tmux-cache.outputs.cache-hit != 'true'
  run: ./scripts/build-tmux.sh
```
All four use the **identical cache key** (`tmux-3.4-Linux-v1`) and all four run in parallel
(all `needs: prepare` only, no dependency on each other). On a warm cache this is fine —
each gets an independent fast hit. On a **cold cache** (first run ever, or a submodule bump
that changes `third_party/tmux`), all four jobs miss simultaneously and independently run
the full autoconf/automake/`make -j$(nproc)` build; only one of the four subsequent
`actions/cache` saves wins (GitHub Actions cache keys are immutable/first-write-wins, the
other three silently no-op) — so a cold cache costs ~4x the tmux compile time in parallel
(not serial, since jobs run concurrently) but the *build minutes billed* still count all
four. This is the second half of the "known Bazel-remote-cache-analogue" opportunity in §2.

## 2. Architectural patterns for CI speed, mapped to this repo's job graph

### Pattern A — build-once/fan-out-many (artifacts)
**Where it already works**: `build.yml`'s `prepare` → 6 downstream jobs (§1).
**Where it's missing and would pay off most**: `e2e-video.yml`'s 2-way shard matrix builds
the identical `stapler-squad` binary + web UI twice per PR run — sharding should happen
*after* one shared build job, with the binary+dist passed via `upload-artifact`/
`download-artifact` to both shard jobs (mirrors `build.yml`'s existing pattern exactly).
**Cross-workflow variant (harder)**: `mcp-integration.yml`, `registry-validation.yml`,
`ux-analysis.yml`, `goreleaser-check.yml`'s `build-smoke-test`, `demo-publish.yml` all
independently rebuild gen code + (mostly) the full web UI on every PR. GitHub Actions has
no native cross-workflow artifact reuse without extra API plumbing
(`actions/download-artifact`'s cross-run mode, needing `run-id` + a token with
`actions:read`, and a way to discover which `build.yml` run corresponds to the same commit
SHA — nontrivial and adds a hard dependency between otherwise-independent workflow files).
The **cheaper win available today, no cross-workflow plumbing needed**: several of these
jobs don't need a *real* Next.js build at all — they only need `go build`/`go test` to
compile against a non-empty `dist/`, which `lint.yml`'s stub (`lint.yml:109-112`) already
proves works. `mcp-integration.yml` (tests `server/mcp`/`server/services` only, no UI
assertions) and `goreleaser-check.yml`'s `build-smoke-test` (smoke-tests the sqlite driver
via CLI, not the UI) are the clearest candidates — each currently pays a full `pnpm install`
+ `next build` (the single most expensive step in `.github/actions/prepare/action.yml`,
mitigated only by a Next.js webpack cache keyed on `web-app/src/**`) purely to satisfy an
embed directive that a one-line stub would satisfy just as well.

### Pattern B — incremental/affected-only builds (path filtering)
Already used extensively and unevenly:
- Native `paths:` trigger filters: `lint.yml`, `goreleaser-check.yml`,
  `registry-validation.yml`, `benchmark.yml`.
- Job-level `if:` gates fed by a `detect-changes` job that diffs against
  `origin/${{ base_ref }}` (because a `paths:`-filtered `pull_request` trigger never posts
  *any* status for a required check, permanently blocking merge — see `build.yml:21-34`'s
  extensive comment, and the PR #618/#619 incident it cites): `build.yml`,
  `mcp-integration.yml`, `e2e-video.yml` (via `tools/ci/detect-feature-changes.sh`).
- **Not path-filtered at all**: `ux-analysis.yml` *is* path-filtered (`web-app/src/**`
  only, at the trigger level — safe, since it's PR-only, no push-to-main required-check
  concern). `generated-proto-guard.yml` and `backlog-scaffolding-guard.yml` intentionally
  run unconditionally on every PR — correct, since they're cheap git-diff-only checks with
  no build step, so there's nothing to save by filtering them.

This is Bazel's core value prop (affected-target analysis) approximated today via
hand-maintained regex path lists in 3 different places (`build.yml:65`,
`mcp-integration.yml:49`, `tools/ci/detect-feature-changes.sh`) that must be kept in sync
by hand — a real "two sources of truth" risk (see §4c).

### Pattern C — self-hosted/remote build-cache backends (Bazel remote cache via GHA cache, or lighter alternatives)
Bazel/bzlmod is explicitly flagged in requirements.md as a rabbit hole and unevaluated;
architecturally, the **narrower, lower-risk version** of the same idea already exists in
this repo in miniature: `actions/cache` keyed on content hashes (Next.js cache in
`.github/actions/prepare/action.yml:59-66`, tmux binary cache in `build.yml`, buf binary
cache). A full Bazel remote-cache migration would replace Go's own build cache (already
warmed via `setup-go`'s `cache: true`, which caches `~/.cache/go-build` +
`~/go/pkg/mod` keyed on `go.sum`) and pnpm's/Next's caches with one unified content-addressed
cache — but Go and Next.js/pnpm already have reasonably effective incremental caching
without Bazel; the marginal win of a Bazel remote cache here is narrower than in a
polyglot monorepo with no existing per-toolchain caching, which is consistent with the
requirements' framing of full Bazel adoption as a multi-week rabbit hole for uncertain
payoff. The lighter alternative — tightening what's already cached (tmux binary artifact
reuse instead of 4x parallel cache-miss rebuilds, §1a; sharing `generated-files` more
broadly) — captures much of the achievable win at a fraction of the migration risk.

## 3. Consistency requirements

### "Required" branch-protection status checks — VERIFIED: none currently exist
Checked directly against the live repo, not inferred from workflow comments:
```
$ gh api repos/tstapler/stapler-squad/branches/main/protection
{"message":"Branch not protected","documentation_url":"...","status":"404"}
$ gh api repos/tstapler/stapler-squad/rules/branches/main
[]
$ gh api graphql -f query='{ repository(owner:"tstapler",name:"stapler-squad") { branchProtectionRules(first:10) { nodes { pattern } } } }'
{"data":{"repository":{"branchProtectionRules":{"nodes":[]}}}}
```
(auth token has full `repo` scope, so this isn't a permissions gap). This matches a prior
finding already on record in this repo: `docs/tasks/backlog-feature-improvement.md:361-363`
states "`main` has no branch protection" (verified there via `gh api
repos/tstapler/stapler-squad`). **However**, extensive comments across `build.yml` (lines
21-34, 79-89, 122-132) and `mcp-integration.yml` (lines 13-25, 59-64) are written as if a
required-check gate exists and is sensitive to a job silently never posting a status
(citing a real incident, PR #618/#619) — i.e., the workflows are engineered defensively for
required-checks semantics that aren't currently turned on at the GitHub level. Two readings:
either (a) required checks were configured at some point and later removed/never
re-added, or (b) the team is deliberately keeping the workflows required-check-safe in
anticipation of turning protection back on. Either way, **the job names `detect-changes`
feeds into (`Test`, `MCP Integration Tests`, etc.) are not currently enforced by GitHub**,
so a rename today cannot break a live required-check gate — but restructuring should still
preserve these job names/semantics, both because turning protection on is one settings
change away and because the defensive comments' entire rationale (a skipped-vs-never-ran
distinction) only matters if/when protection exists.

### stapler-squad's own automation does not hardcode job names either — VERIFIED
Searched every CI-status consumer in the Go codebase:
- `session/git/worktree_git.go`'s `parsePRStatusPayload` (`worktree_git.go:684-760`) reads
  `gh pr view --json statusCheckRollup` and sets `status.CIFailing = true` on **any** check
  with a FAILURE/TIMED_OUT/CANCELLED conclusion or FAILURE/ERROR state — no job-name
  matching at all.
- `session/backlog_plugin_github_prs.go`'s `fetchCILabel` (lines 165-197) calls the GitHub
  check-runs API and emits a generic `pr:ci-failing` label if **any** check-run's
  `conclusion` is `failure`/`timed_out` — again no per-job-name logic.

So a restructuring that renames/splits/merges jobs is safe from the perspective of this
repo's own tooling; the only consumers of job identity are (a) any future GitHub branch
protection configuration (not currently present) and (b) human reviewers' muscle memory
reading the PR checks list.

### `make ci` / `make ready` as the "local approximation of every required PR check"
`Makefile:889` (`ci: build $(BIN_TMUX) test test-race vet lint lint-css-tokens
test-integration fmt-check registry-generate actor-field-guard ptmx-field-guard
otel-auto-isolation-guard`) and `Makefile:898` (`ready: ci ready-complexity-gate
ready-duplication-gate-web`, plus `next lint`/`lint:css`/scanner tests inline) are
documented (`Makefile:891-897`) as covering everything CI checks **except** things with no
local equivalent: the external `go-test-coverage` GitHub Action
(`build.yml:294-300`), the E2E-coverage PR-comment step, and anything needing live
PR/GitHub context. Any CI restructuring (splitting `test` into more jobs, moving steps
between workflows) must keep this mapping intact — i.e., every check `make ready` runs
locally must still correspond to *some* CI job, or `make ready` silently stops being a
faithful approximation and developers lose the ability to pre-validate before pushing.

### Generated code must never be stale in what tests actually exercise
Every workflow that runs `go test`/`go build` against packages depending on `gen/proto` or
`session/ent` must generate both fresh in **that job**, since nothing is committed and
GHA artifacts don't cross workflow-run boundaries by default (§1). `build.yml`'s
fan-out jobs satisfy this via the downloaded artifact. Everywhere else it's satisfied only
because each job happens to regenerate from scratch — **except one job, which doesn't**,
detailed next.

## 4. Cross-cutting failure modes for this migration

### (a) Stale/mismatched shared artifact across jobs — a live example already in production, not hypothetical
`benchmark.yml`'s `go-tier1` job (`benchmark.yml:53-84`) runs
`go test -bench='...' ... ./server/events ./server/terminal ./session/scrollback ./session
./session/detection/ratelimit ./session/tmux ./session/unfinished ./session/queue
./session/tokens` **without ever running `buf generate` or `ent generate`** — unlike every
other Go-benchmark/test job in this repo. `./session`, `./server/events`,
`./session/unfinished`, and `./session/queue` all import generated ent/proto packages
(confirmed: `session/ent_repository.go`, `session/storage.go`,
`session/session_summary_snapshot.go`, etc. import `gen/proto/go/session/v1` and 20+
`session/ent/*` subpackages — `grep` count: 29 files in `session/` alone import one or the
other).

**Verified via the actual run logs** (`gh api
repos/tstapler/stapler-squad/actions/jobs/98769887397/logs`, run `33146922227`,
2026-08-28): 4 of the 9 target packages fail with `FAIL	.../server/events [setup failed]`,
`FAIL	.../session [setup failed]`, `FAIL	.../session/unfinished [setup failed]`,
`FAIL	.../session/queue [setup failed]` — all `no required module provides package
.../gen/proto/go/session/v1` / `.../session/ent/<x>` errors. **Yet the job's reported
conclusion is `success`.** Root cause: the step is
```yaml
run: |
  go test -bench='...' ... ./... | tee tier1-bench.txt
```
with no `set -o pipefail`. Bash's default pipeline exit status is the *last* command's
(`tee`, which always exits 0), so `go test`'s non-zero exit from the 4 failed packages is
silently swallowed. This means `BenchmarkEventBus` (server/events),
`BenchmarkReviewQueue`/`BenchmarkDetectCommandsInText`/etc. (wherever they live under
`./session` or `./session/queue`), and anything in `./session/unfinished` have likely never
actually run in this "Tier 1" gate, while the job has reported green on every run. This is
exactly the class of failure requirement 4(a) asks about — a downstream job silently
consuming an artifact (here, the *absence* of generated code) that doesn't match what it
needs — and it demonstrates concretely why any restructuring toward more shared
artifacts/build-once patterns must (1) fail loud (`set -o pipefail`, or check `go test`'s
own exit code) rather than let a convenience wrapper (`tee`, an artifact-download step)
mask a build failure, and (2) this specific job should gain `ent-gen`/`proto-gen`
generation (or download `build.yml`'s artifact, if made cross-workflow) as part of any
"share the generated code once" restructuring — it's the one place in the entire workflow
inventory currently missing it. **This is a pre-existing bug independent of the ci-speed
project**, but it's the single clearest evidence in this codebase of the exact
silent-staleness risk requirement 4(a) asks to be evaluated against, so it belongs in the
plan as a concrete before/after test case: "does this job now correctly fail if generated
code is missing/stale," not just "is it faster."

### (b) Partial Bazel/build-cache migration leaving two build systems
Not yet applicable (no Bazel adoption has started — the prior tmux-only Bazel path was
fully removed in `b51b60eb1`, per requirements.md). The concrete risk if a phased
adoption were chosen: `make ci`/`make ready` (Makefile-driven) and any Bazel-driven
CI path would both need to independently produce correct, non-stale generated
code/binaries — the "two sources of truth" risk is the same shape as detect-changes'
duplicated path-filter regexes (§2, Pattern B) but higher-stakes, since a
Makefile-target and a Bazel-target silently drifting on what counts as "the ent schema
changed" would reintroduce exactly the staleness class in (a). Any migration plan should
sequence workflow-by-workflow cutover (e.g., migrate `lint.yml`'s cheap stub-based checks
first, since they have the least generated-code surface) rather than a flag-day switch,
and should not remove the Makefile path until every workflow that references it
(`make ci`/`make ready`/CLAUDE.md's own documented developer workflow) is migrated too —
otherwise local dev (`make ready`) and CI diverge on what "passing" means.

### (c) False-negative path-filter skip (a correctness risk, not just speed)
Three independent regex lists currently decide "is this PR relevant": `build.yml:65`
(`\.go$|^go\.(mod|sum)$|^Makefile$|^install\.sh$|^proto/|^web-app/src/|...`),
`mcp-integration.yml:49` (`^server/mcp/|^server/services/|^main\.go$|^go\.(mod|sum)$`), and
`tools/ci/detect-feature-changes.sh` (feature-marker-based, for `e2e-video.yml`). Each is
hand-maintained and can drift from what the underlying job graph actually needs — e.g., if
a future PR adds a new top-level Go directory whose packages are imported by `server/mcp`
but the regex isn't updated, `mcp-integration.yml`'s `detect-changes` job would report
`relevant=false`, the `mcp-integration` job would be *skipped* (which, per the documented
GitHub Actions behavior these comments rely on, counts as a **passing** status for a
required check), and a real breakage in that dependency would merge silently. This is the
sharp edge of the "skipped counts as passing" trick `build.yml`/`mcp-integration.yml`
deliberately rely on to avoid the PR #618/#619 "permanently pending" failure mode — it
trades one failure mode (blocked-forever) for a narrower one (false-negative skip). Any
consolidation of these three regex lists into one shared, more-affected-target-aware
mechanism (a lighter Bazel-style dependency graph, or even just a single shared path-list
constant) should be a design goal, not just a speed one — today, widening or narrowing any
one of the three lists is a manual, unverified judgment call with no test coverage checking
"does this filter actually match what the job imports."

### (d) Concurrency/cancel-in-progress cancelling a run branch protection depends on
Currently low-risk in practice given (3)'s finding that no branch protection exists, but
still worth getting right for when/if it's turned on. Audited every `concurrency:` block:
- `e2e-video.yml:21-23` and `ux-analysis.yml:23-25`: `cancel-in-progress: true`, scoped to
  `${{ github.event.pull_request.number }}` — cancels a superseded run's *entire* job
  (including its PR-comment mutation), which both files' comments justify as
  intentional/safe since a superseded PR run's own output (videos, UX findings) is stale
  anyway. This is fine as long as these checks are never made *required* — a required
  check that never reports a final status because it was cancelled mid-run would revert to
  the PR #618/#619 problem (permanently pending, not skipped) unless it's cancelled cleanly
  enough for GitHub to record a `cancelled` conclusion (which does satisfy "resolved," not
  "pending," for a required check — cancelled is a terminal status, not indefinite).
- `benchmark.yml:36-40` and `build.yml`'s `benchmark-gate` job (`concurrency: group:
  baseline-push-main`, `cancel-in-progress: false`): explicitly `cancel-in-progress: false`
  on `main` pushes, with an explicit comment that baseline-save steps must be allowed to
  complete — already correctly avoids this failure mode.
- No other workflow declares a `concurrency:` block, so a rapid sequence of pushes to the
  same PR runs every workflow to completion redundantly today (a speed cost, addressed by
  Pattern B/path-filtering, not a correctness risk).

## 5. Who/what depends on this

| Dependent | What it needs from CI | Current mechanism | Risk if job identity/semantics change |
|---|---|---|---|
| Branch protection (GitHub-native) | N/A today — **verified not configured** (§3) | — | None today; becomes real the moment protection is (re-)added, at which point job **names** (not just conclusions) matter |
| Backlog automation (`session/backlog_lifecycle.go` auto-merge, `EnablePRAutoMerge`) | Any-check-failing signal, not per-job | `parsePRStatusPayload`/`fetchCILabel` — generic, name-agnostic (§3) | None — safe to rename/restructure jobs freely from this consumer's perspective |
| `session/backlog_plugin_github_prs.go`'s PR-list plugin | Same generic check-runs signal, for labeling (`pr:ci-failing`) | Same as above | None |
| Human reviewers | Recognizable check names in the PR "Checks" tab to judge mergeability | GitHub UI convention only | Low — a rename is a one-time re-orientation, not a functional break, given no automation keys off names |
| `make ready`/`make ci` (local dev) | 1:1 coverage of what CI checks, so a green local run predicts a green PR | Manually maintained parity (Makefile comments cite this explicitly) | Medium — any workflow restructuring that adds/removes a check with no Makefile equivalent silently breaks this promise (§3) |
| This project's own success metric ("~50% cut in per-PR wall-clock to green, across all required checks") | An accurate current inventory of what's required | Currently: nothing is GitHub-required; "required" in practice means "the checks a human/backlog-automation waits on before merging" | The metric's baseline (requirements.md's per-workflow avg/max table) should be read as "every workflow that runs on a PR," not "every branch-protection-required check," since the latter set is currently empty |
