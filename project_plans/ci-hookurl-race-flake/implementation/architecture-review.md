# Architecture Review: ci-hookurl-race-flake
**Date**: 2026-08-01
**Verdict**: CONCERNS

## Scope Note

This plan is an explicit adoption/consolidation of the already-reviewed
`project_plans/flaky-hook-url-tests/implementation/plan.md` (verdict CONCERNS, no
blockers, in its own `architecture-review.md`). Per the task brief, that prior plan is not
re-reviewed from scratch here. This review verified: (1) the adoption is faithful (diffed
against the source plan — no dropped Risk Control / Unresolved Questions / task content
found; the adopted plan actually *resolves* two of the source review's own concerns by
folding their remediations directly into task text — Task 1.2.1's closure-safety sentence,
Task 2.1.2's ≥3-run methodology), (2) the two factual/technical claims flagged for
independent verification, and (3) the one net-new task (2.1.2b) and the Provenance/
line-verification framing that this plan itself adds.

**`assert.Eventually` background-goroutine claim — independently verified against actual
source** (`github.com/stretchr/testify@v1.11.1/assert/assertions.go:1988-2021`, module cache
at `/home/tstapler/go/pkg/mod/...`): `Eventually` spawns `go checkCond()` and only spawns the
*next* `checkCond()` after receiving the previous tick's result off `ch` (`case v := <-ch:
... tickC = ticker.C`), so at most one `checkCond` invocation is ever in flight — there is no
overlapping/concurrent execution of the condition closure to race against. The write to
`hookCmd` inside the closure happens-before the corresponding `ch <-` send, which
happens-before that send's receive in the `Eventually` loop, which happens-before
`Eventually` returns control to the caller, which happens-before the caller's `return
hookCmd`. This is a valid Go memory-model chain — the plan's mitigation (assign only in the
success branch) is genuinely race-safe, not merely plausible-looking, and testify's
`-race` detector will not flag it. Also independently confirmed `testutil/wait/wait.go:53-75`
(`WaitForCondition`) runs its condition synchronously in the calling goroutine (no `go`
statement anywhere in the function) and imports only `context`/`fmt`/`time` — the plan's
"no import cycle, no background goroutine" claims for Task 1.2.4 both hold.

## Constitution Violations

None. `docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo (confirmed
via `ls docs/adr/` — no `ADR-000-*` file present; also independently confirmed by the prior
`flaky-hook-url-tests/implementation/architecture-review.md`, which found the same).

## Blockers

None.

## Concerns

- [ ] **Task 2.1.1's CI-comment cross-project reference** — The inline comment this task adds
  above the `-p 1` flag change (`.github/workflows/build.yml`) reads: `# ... see
  project_plans/flaky-hook-url-tests/decisions/ADR-001. Do not remove without re-reading that
  ADR...`. This is the *only* place a future engineer reading the shipped CI config is pointed
  to the actual decision rationale, and it points into the sibling, non-canonical project
  directory (`flaky-hook-url-tests/`) rather than the backlog-tracked project actually being
  shipped (`ci-hookurl-race-flake/`, whose own `decisions/` directory this plan deliberately
  leaves empty by design). If `flaky-hook-url-tests/` is ever archived, pruned, or squashed out
  of the repo history (nothing in this plan or in
  `.claude/rules/sdd-planning-artifacts-commit.md` obligates it to be kept indefinitely — that
  rule only requires committing planning artifacts, not preserving superseded parallel
  projects forever), the comment's reference silently dangles and the "do not remove without
  re-reading" guardrail becomes unfollowable.
  - **Remediation**: Either (a) add a short pointer file (e.g.
    `project_plans/ci-hookurl-race-flake/decisions/ADR-001-POINTER.md`, explicitly *not* a
    competing ADR, just "see flaky-hook-url-tests/decisions/ADR-001 for the full record") so
    the canonical project directory has its own durable breadcrumb independent of the sibling
    project's survival, or (b) add one sentence to this plan's Risk Control section stating
    that `flaky-hook-url-tests/` must not be deleted/archived while this CI comment references
    it, making the dependency an explicit, tracked constraint rather than an implicit one.

- [ ] **Task 2.1.2b's placement inside Story 2.1** — Task 2.1.2b ("Capture a one-off
  hook-injection pipeline latency measurement") is listed directly after Task 2.1.2, under
  `#### Story 2.1: Reduce cross-package -race CPU contention on the gating job` (no new story
  header precedes it). But its content measures a different thing than Story 2.1's stated
  theme: Task 2.1.2 measures the wall-clock *cost* of serializing package binaries via `-p 1`
  (a CI-contention-mitigation cost); Task 2.1.2b measures the hook-injection *pipeline's own
  latency* under contention (evidence for whether the 60s budget has headroom, and whether
  `InjectHookConfig` itself is a latent bottleneck) — thematically this is an observability/
  evidence-gathering task, matching Epic 3's "Regression check / observability artifact" theme
  far more closely than Epic 2's "CI-side contention mitigation" theme. Numbering it `2.1.2b`
  (a lettered-suffix variant of 2.1.2) implies a tighter relationship to Task 2.1.2 than
  actually exists, and a reader scanning story headers to find "where does acceptance
  criterion 4's measurement live" would reasonably look under Epic 3, not buried inside Story
  2.1. This is a cosmetic/organizational issue, not a functional one — the plan is transparent
  elsewhere that 2.1.2b is a net-new, project-specific addition (called out explicitly in the
  Dependency Visualization diagram's "Additional" section) — but the task-breakdown body text
  itself doesn't carry that same signal.
  - **Remediation**: Renumber as `Task 3.1.4` (or introduce a small `Story 3.2: Pipeline
    latency evidence` under Epic 3) so its placement in the document matches its actual
    thematic home, and so the "no code change, evidence-only, feeds validation.md" nature of
    both this task and Epic 3's other tasks reads consistently on a first pass through the
    headers.

## Nitpicks

- Requirements.md's acceptance criterion 5 says "No source behavior in
  `session/services/approval_handler.go`..." — the actual path (confirmed by this plan's own
  Domain Glossary and by direct inspection) is `server/services/approval_handler.go`. This
  typo is in `requirements.md`, not in the plan under review (which uses the correct path
  throughout), so it's out of scope to fix here, but worth a one-line correction next time
  `requirements.md` is touched.
- Structural/DDD lens is genuinely N/A here as the plan itself notes: no new types, no new
  interfaces, no aggregate boundaries — the entire change surface is one test file's poll
  helpers and one CI YAML flag. No SOLID or interface-pollution-checklist violations found
  (no new interface, no new wrapper type, no speculative abstraction introduced anywhere in
  the task breakdown).
- The Unresolved Questions section's handling of `research/build-vs-buy.md`'s divergent
  build-tag/`-short`-isolation recommendation is honest, not papered over: it states the
  recommendation, explains concretely why it's not adopted (changes *what* the gating run
  covers vs. `-p 1`'s *how concurrently*), and invites a future reviewer to argue against
  ADR-001's reasoning explicitly rather than silently substituting the alternative. No finding
  — flagged here only because the review brief specifically asked whether this was handled
  honestly, and it is.
