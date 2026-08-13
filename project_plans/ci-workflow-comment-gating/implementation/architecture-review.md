# Architecture Review: ci-workflow-comment-gating
**Date**: 2026-08-12
**Verdict**: CONCERNS → both concerns patched into plan.md 2026-08-12 during Phase 4 validation
(see checkboxes below)

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository (confirmed:
`find . -iname "*constitution*"` returns no results, and `docs/adr/` contains no `ADR-000` file
of any name). No constitution to check the plan against — skipping this section.

## Scope note

A separate `implementation/adversarial-review.md` (dated the same day, verdict BLOCKED) already
covers correctness/pre-mortem concerns — a `concurrency:`-block race between `deleteComment` and
an out-of-order run, and the 20% threshold being transplanted from a statistically-sampled path
onto two single-sample Playwright paths with no significance testing. Those are real and this
review does not re-litigate them. This pass applies only the three architecture lenses
(structural integrity, type-level design, pattern selection) requested for this review; findings
below are additional to, not a replacement for, the adversarial review's blockers.

**Lenses that don't meaningfully apply, confirmed by reading the plan against the actual
workflow files:** DDD aggregate boundaries (no data model — each gate is a single-request
conditional over CI step outputs), GoF creational/structural/behavioral patterns, and PoEAA
layering (Transaction Script/Repository/Unit of Work — no persistence layer exists). The plan's
own Pattern Decisions table reaches the same conclusion with the same reasoning (avoiding
`.claude/rules/interface-pollution-checklist.md`-style speculative abstraction) — agreed, not
re-argued here. Build-vs-buy consistency (Lens 3 #11) is also confirmed: the plan matches
`research/build-vs-buy.md` Option 1 (in-repo copy of the gate pattern) exactly, with Options 2-4
correctly deferred to Unresolved Questions rather than folded in.

## Blockers

(none from this review's three lenses — see adversarial-review.md for the two blockers already
identified from the correctness/pre-mortem pass)

## Concerns

- [x] **RESOLVED** (patched 2026-08-12 during Phase 4 validation, pre-mortem Failure #1) —
  **Phase 1 (`benchmark.yml`, Tasks 1.1.1b/1.2.1a/1.3.1a) and Phase 3 (`build.yml`, Task
  3.1.1b) — the actual gate/parsing logic is designed to be testable only via a live PR push, not
  in isolation.** Tasks 1.1.1e, 2.1.1f, and 3.1.1e extract each workflow's gate decision logic
  into standalone `tools/ci-gates/*.js` modules with `node:test` unit tests against literal
  fixtures, exactly as recommended below. Lens 1 criterion #4 (testability): can each component be tested standalone, or
  does the plan force integration-only testing? As designed, the answer is integration-only for
  every gate site — `plan.md`'s own Risk Control section states verification happens by "push to
  a scratch branch/test PR... observe the Actions run," and `research/pitfalls.md` §5 confirms
  "GitHub Actions has no offline linter for embedded `github-script` JS syntax." This is the
  precise class of bug `research/pitfalls.md` §4 calls "the single highest-risk failure mode
  identified in this research" — a regex/threshold gate that silently matches nothing produces a
  100% false negative with no error, and §1 confirms the sticky comment is the *only* surfacing
  channel for 3 of the 4 target workflows. The regex reused in Task 1.1.1b (`build.yml`'s
  `benchmark-gate` job's `-delta-test=utest` + grep combo) is at least a proven pattern, not novel
  code, which lowers but doesn't eliminate this — the frontend-throughput/e2e-latency numeric
  `pctNum` computation (Tasks 1.2.1a/1.3.1a) and `build.yml`'s `previousCoveragePct` extraction
  (Task 3.1.1b) are net-new logic with zero prior validation anywhere.
  **Remediation**: extract the pure gate-computation functions — `hasRegression` threshold logic,
  `previousCoveragePct` extraction, `isActionable` composition — into small standalone
  `.js`/`.ts` modules (e.g. `tools/ci-gates/benchmark-regression.js`,
  `tools/ci-gates/coverage-delta.js`) invoked from the `github-script`/`node -e` blocks via
  `require()`, each with a handful of `node:test`/Jest cases against literal captured
  benchstat/comment-body fixtures. This is not the CRUD-extraction the plan already correctly
  declined (build-vs-buy.md Option 4, orthogonal, rejected for good reason) — it's extracting only
  the ~5-10 lines of decision logic that are the actual subject of this item, turning "testable by
  pushing a real PR" into "testable in seconds locally," and it directly closes the failure mode
  the plan's own research names as highest-risk. Proportionate to a Low-Medium item: a few small
  files, not a new framework.

- [x] **RESOLVED** (patched 2026-08-12 during Phase 4 validation, pre-mortem Failure #1) —
  **`build.yml` Task 3.1.1b — `previousCoveragePct` is recovered by regex-scraping the
  rendered Markdown prose of a prior comment, using the comment thread as an unstructured
  persistence layer for numeric state.** Task 3.1.1e switches the marker to
  `<!-- feature-coverage:pct=NN.N -->` and parses against that anchored format exclusively,
  exactly as recommended below. Lens 2 criterion #7 (parse-at-boundary): raw input should
  be parsed into a proven domain value at a clear boundary, not re-derived by scanning free text.
  `existing.body.match(/\((\d+(?:\.\d+)?)%\)/)` depends on the *exact wording* of a comment body
  written by an earlier run of this same script (confirmed against `tools/coverage/feature-coverage.ts:61`'s
  current output format, `Feature E2E coverage: X/Y tested (NN%)` — only one parenthesized
  percentage exists in the body today, but nothing enforces that going forward). A future edit to
  the comment template — rewording the summary sentence, adding a second percentage elsewhere in
  the body (e.g. a per-category breakdown), localizing text — silently breaks the comparison: the
  regex would either match the wrong number (producing a wrong `isActionable` result with no error
  signal) or fail to match (falling into the `previousCoveragePct === null` branch, which the plan
  already treats as "no prior state" and posts unconditionally — a safe failure mode, but only by
  accident of how the fallback happens to be written, not by design).
  **Remediation**: encode `previousCoveragePct` in the HTML marker itself, in a fixed format the
  parser owns exclusively, e.g. `<!-- feature-coverage:pct=48.3 -->` instead of the bare
  `<!-- feature-coverage -->`, and parse it with an anchored regex against the marker
  (`/feature-coverage:pct=([\d.]+)/`) rather than scanning the free-form summary sentence. One-line
  change to the marker constant and the parse regex; removes an entire class of future breakage
  tied to comment-copy changes, and makes the "boundary" where prior state is parsed explicit and
  self-documenting instead of coincidental.

## Nitpicks

- Task 2.1.1a inlines the `GITHUB_OUTPUT` write directly into `tools/ux-analysis/analyze.ts`'s
  `main()` (`if (process.env.GITHUB_OUTPUT) { fs.appendFileSync(...) }`), coupling a general CLI
  analysis tool's core function to a CI-specific side channel. Low-stakes given the file's
  existing style (already does direct `console.log` in `main()`), but if this tool is ever run
  outside CI (a local dev invocation), consider a small `writeGithubOutput(name, value)` helper
  called from `main()` rather than growing `main()` with CI-reporting concerns. Not urgent — a
  two-line diff either way.
- Task 4.1.1a's "$GITHUB_STEP_SUMMARY write of the shard/artifact list" is ambiguous about whether
  it carries the same demo-preview GIF links (`gifArtifacts`) the old normal-case comment provided,
  or just raw artifact names. Once the comment is suppressed on the normal path, the step summary
  is the only remaining in-PR surface for those links — worth confirming during implementation
  that the convenience links aren't quietly dropped, not just the artifact count.
