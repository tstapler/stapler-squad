# Research: Pitfalls & Risks — CI Workflow Comment Gating

Research Agent 4 (Pitfalls), SDD Phase 2. All line numbers VERIFIED against the worktree
at the time of writing; re-check before implementation if the target files have since moved.

## 1. Over-gating / false negatives — is the comment the ONLY signal?

**Verified: for the specific failure modes at risk, yes, the comment is the only signal.**
None of the four workflows fail the underlying job on the advisory condition itself:

| Workflow / job | What "fails" the job today | What does NOT fail the job |
|---|---|---|
| `benchmark.yml` (all 3: go-tier1, frontend-throughput, e2e-latency) | Missing output file (`benchmark-results.json` / `e2e-latency-results.json` not produced — `benchmark.yml:293-297`, `509-511`) — an infra/test-execution failure, not a perf regression | An actual benchstat regression. There is no threshold check anywhere in the job; `build.yml:266-268`'s comment explicitly documents these as "advisory... never blocked." |
| `ux-analysis.yml` | Axe violations (`continue-on-error: false`, line 93) — independently red regardless of the comment gate | Lighthouse score < 70 (`continue-on-error: true`, line 109) — advisory only |
| `build.yml` | "Check new RPCs have tests" (lines 249-262) — a *different*, unrelated check (new RPC lacking `tested: true`) | Feature-coverage % dropping relative to prior state — no check anywhere enforces this |
| `e2e-video.yml` | Nothing — grepped the whole file for `exit 1`/`ERROR`/assertions on video count: none exist | Zero video artifacts produced — detected *only* inside the comment-building script (`videoArtifacts.length === 0`, line ~277) |

**`$GITHUB_STEP_SUMMARY` fallback check (VERIFIED via grep, all 5 files):** only
`benchmark.yml`'s go-tier1 job writes to `$GITHUB_STEP_SUMMARY` (lines 100-101), and only
for the PR-comparison case. frontend-throughput, e2e-latency, ux-analysis.yml, build.yml,
and e2e-video.yml write **nothing** to the step summary. So today, for three of the four
target workflows, the sticky PR comment is genuinely the *only* place any of this
information is surfaced — there is no redundant signal to fall back on.

**Consequence:** a bug in the new gate logic that silently suppresses a comment (e.g. the
Lighthouse NaN case below, or a benstat text-match that fails to recognize a real
regression — see §4) means the regression is **not just uncommented, it is completely
invisible** for frontend-throughput, e2e-latency, build.yml, and e2e-video.yml. This raises
the stakes on gate correctness well above "just avoid noise" — a false negative here is a
silent data loss, not a UX inconvenience. **Recommendation for the plan phase:** route the
raw comparison output to `$GITHUB_STEP_SUMMARY` unconditionally (always executes,
independent of the comment gate) in all four workflows, mirroring what go-tier1 already
does for its PR-comparison branch — this was already flagged as an Open Question in
requirements.md; this research confirms it should be treated as effectively required, not
optional, given the absence of any other signal.

**Concrete NaN pitfall (ux-analysis.yml, VERIFIED):** `ux-analysis.yml:96-108`'s Lighthouse
score extraction is wrapped in try/catch and defaults to the *string* `'unknown'` on any
parse failure (missing manifest, malformed JSON, `lhr.categories.performance` undefined,
etc.) — i.e., a Lighthouse *execution/parse failure* is indistinguishable from a clean run
by the time it reaches the comment step. Current code (`ux-analysis.yml:162-167`) does
`parseInt(lighthouseScore, 10)` → `NaN`, and correctly treats `isNaN(score)` as a warning
case today (comment always posts regardless). **A naive gate written as `if (score < 70)
{ shouldPost = true } else { return }` breaks this**: `NaN < 70` evaluates to `false` in
JS, so the parse-failure case would take the "no regression" branch and the comment (the
only visible sign that Lighthouse didn't run) would be silently suppressed. The gate must
explicitly branch on `isNaN(score)` as its own actionable case (post an "unable to measure"
comment or write to `$GITHUB_STEP_SUMMARY`), not rely on the numeric comparison alone.

## 2. Stale sticky comments

**Verified: no precedent exists in this repo for deleting a stale comment.** Grepped
`.github/workflows/*.yml` for `deleteComment` — zero hits across all 12 workflow files,
including `registry-validation.yml`. This directly answers the Open Question in
requirements.md ("does any workflow in this repo already delete a stale sticky comment?")
— no, there is no prior art to follow; this would be new code, not a copy.

**Important structural mismatch with the stated "copy the registry-validation.yml gate"
approach:** `registry-validation.yml`'s own comment step (lines 72-116) does **not** use
the sticky marker/`listComments`/`updateComment` pattern at all — it has no `marker`
variable, no `listComments` call, and unconditionally `createComment`s a brand-new comment
every time the gate passes. This means `registry-validation.yml` itself has no
stale-comment problem to solve (nothing to leave behind — worst case is duplicate comments
across pushes, which is out of scope per requirements.md). The four target workflows are
structurally different: they *do* have the sticky find-by-marker → update/create pattern
(`benchmark.yml:117-137,343-360,557-575`; `ux-analysis.yml:183-203`; `build.yml:226-246`;
`e2e-video.yml:300-317`). **Copying only the two-line early-return gate
(`registry-validation.yml:90-95`) into these steps — placed before the `listComments` call,
as a literal copy would do — means a cleared regression never reaches the
`updateComment`/`deleteComment` logic at all.** The prior "regression detected" comment
body is left exactly as it was, sitting on the PR indefinitely, because the early `return`
exits before the code that would have updated or removed it ever runs.

**Concrete risk:** a reviewer opens a PR days after the regression was fixed by a later
push, sees a sticky comment still reading "⚠️ Regression detected" from the earlier commit,
and either (a) blocks/requests changes for an issue that's already resolved, wasting
author/reviewer time, or (b) starts distrusting these bots' comments generally and ignores
future *real* regression comments too (the "trust erosion" failure mode explicitly named in
`.claude/rules/fix-flaky-tests-dont-defer.md`'s CI-signal-trust framing, same dynamic
applied to comments instead of test failures).

**Required fix shape (for the plan phase, not resolved here per requirements.md's Open
Questions):** the gate must be restructured as an if/else, not a bare early return:
`listComments`/find-`existing` must still run unconditionally; when the current run is
clean, either (a) call `deleteComment` on `existing` if found, or (b) call `updateComment`
with a "cleared" body. A bare `return` before the lookup is only safe for
`registry-validation.yml`-shaped steps (no marker, no existing-comment lookup) — it is
**not** a safe pattern to copy verbatim into any of the four target workflows, despite
requirements.md's characterization of them as sharing "this shared shape" (they share
`continue-on-error`+marker+sticky-update, `registry-validation.yml` shares only the sticky
marker's *absence* of a stale-comment problem).

## 3. `continue-on-error` and `if: always()` semantics

**Correction to a requirements.md claim, VERIFIED by grep across all 5 workflow files':**
requirements.md states "All five comment steps use `continue-on-error: true` on the
comment-posting step itself." This is **only true for `registry-validation.yml`
(line 74) and `build.yml` (line 247)**. It is **false** for:
- `benchmark.yml`'s three comment steps (`benchmark.yml:106-137`, `330-360`, `546-575`) —
  none have `continue-on-error` at all.
- `ux-analysis.yml`'s comment step (`ux-analysis.yml:149-203`) — no `continue-on-error`.
- `e2e-video.yml`'s comment step (`e2e-video.yml:245-317`) — no `continue-on-error`.

(The `continue-on-error: true` lines found by grep in `benchmark.yml` at 422/426 and in
`ux-analysis.yml` at 54/93/109/137 belong to *other* steps — e.g. Lighthouse execution,
artifact uploads — not the comment-posting step.)

**Consequence for this change:** adding a gate means adding more JS logic (regex/text
parsing per §4, conditional branches) to steps in 3 of the 4 target workflows that have **no
safety net** — if the new gate logic throws an unhandled exception (e.g. `TypeError` on an
unexpected `undefined`, a `github.rest.issues.listComments` API error, a malformed env var),
the step fails and, since there's no `continue-on-error`, the whole job goes red. Today
these steps can only fail on the underlying GitHub API calls themselves (already narrow
surface); adding parsing/branching logic increases that surface. **Recommendation:** add
`continue-on-error: true` to these three steps as part of this change (consistent with the
stated intent that these are advisory-only and should "never fail the job if posting is
denied" per the comment style already used in `registry-validation.yml:74` and
`benchmark.yml:422`), not just the gate logic itself.

**`return` inside `actions/github-script` semantics (confirmed against `actions/github-script`
v7 behavior, which all 5 workflows pin):** a `return` statement inside the `script:` block
simply ends the async function normally — the step is reported as **successful** (`success`,
exit-code-equivalent 0) exactly like `registry-validation.yml`'s existing early return
already does today in production. This is orthogonal to `continue-on-error`: an early
`return` was already step-success before `continue-on-error` entered the picture (it only
matters for *thrown* errors, not early returns), so `continue-on-error` is moot for the
`return`-based gate itself, but still relevant for the exception-surface concern above.

**`if: always()` usage (VERIFIED, mixed across workflows — not "several cases" uniformly):**
`ux-analysis.yml:150` and `e2e-video.yml`'s notify job (`e2e-video.yml:243`) use
`if: always() &&...`; `benchmark.yml`'s three comment steps (`if: github.event_name ==
'pull_request'`, no `always()`) and `build.yml:217` do not. `always()` here governs whether
the comment step *runs at all* when an earlier step in the same job failed — it has no
interaction with the new gate logic (the gate only affects whether the script posts once it
runs). No downstream `needs.<job>.result` checks were found anywhere in the 12 workflow
files that reference these comment-job results (grepped; the only `needs.*.outputs.*`
references found are for genuinely different outputs like
`needs.detect-feature-changes.outputs.record_features`), so adding a `return` does not
change any cross-workflow gating — confirms Acceptance Criterion #7's premise holds.

## 4. Threshold/parsing fragility (benchmark.yml specifically)

`ux-analysis.yml`'s existing gate is a clean numeric comparison (`score < 70`,
already-parsed integer from a JSON manifest). `registry-validation.yml`'s gate is a
substring check against **known, controlled tool output** (`validate-registry.sh`'s own
fixed vocabulary: "Added"/"Removed") plus an exit code — low fragility because the
producing script's output format is owned by this repo.

`benchmark.yml`'s three jobs, by contrast, pipe raw `benchstat` output straight into the
comment body (`benchstat-tier1.txt`/`throughput-comparison.txt`/`latency-comparison.txt` —
e.g. `benchmark.yml:98-104`) with **no existing threshold check at all** — confirmed above
in §1, there is no regression detection logic today, just "did the file get produced."
A naive text-match gate (e.g. checking the benchstat output for `"regression"`, a `+`
percentage sign, or `"~"` for no-change) is meaningfully more failure-prone than
`ux-analysis.yml`'s numeric threshold because:

- **benchstat's actual output format is not a fixed vocabulary the repo controls** — it's
  produced by the external `benchstat` tool. Column layout, significance markers (e.g.
  `p=0.023 n=10+10`), and the presence/absence of a delta column depend on the specific
  `benchstat` version and flags used, none of which are pinned/asserted in these jobs
  (checked: no `benchstat` version pin found in `benchmark.yml`).
- benchstat marks statistically **insignificant** changes with `~` and omits a percentage
  entirely for those rows — so a text-match gate keying on "presence of a `%` sign" would
  false-positive (treat noise as a regression) rather than false-negative, while a gate
  keying on "presence of the word `regression`" would **never fire** (benchstat's output
  never contains that literal word — it reports deltas, not verdicts), which is a **total,
  silent false negative** for every single run, not just edge cases: this reference case in
  requirements.md's own §4 framing ("checking for the word 'regression'") describes a
  gate that would gate out 100% of runs including real regressions, since that string does
  not occur in benchstat output.
- Correctly parsing benchstat's structured comparison (delta % per benchmark row, `p`-value
  significance) into "is there a real regression" requires either benchstat's `-format csv`
  flag (if supported by the pinned version) or a small parser for its plain-text table
  output — meaningfully more implementation effort than `ux-analysis.yml`'s one-line
  numeric compare, but the effort is justified given the false-negative risk above: a
  wrong text-match gate is worse than no gate (silently hides real regressions in the one
  place — see §1 — where nothing else would catch them).

**Recommendation for the plan phase:** treat benchmark.yml's gate as the highest-risk of the
four changes and budget real design time for it — either a benchstat-output parser (delta %
+ statistical significance) or, as a lower-effort interim step, keep posting unconditionally
but move to `$GITHUB_STEP_SUMMARY`-first with the comment reserved for a manually-defined,
conservative signal (e.g. any row showing a negative delta beyond N%, parsed from a known
benchstat column position) rather than a bare keyword match.

## 5. General GitHub Actions gotchas for this class of change

- **YAML/multi-line JS risk:** all `script:` blocks use the `|` block-scalar style, which is
  whitespace-sensitive but tolerant of embedded `#!`/`{`/backtick characters as long as
  indentation is consistent. The main risk when *editing* these (not rewriting from
  scratch) is an indentation slip when inserting a new `if (...) { return; }` block or an
  `else` branch — YAML block scalars don't error on inconsistent indentation the way you'd
  hope; they just include the wrong whitespace in the string, which `actions/github-script`
  then evaluates as JS (usually a `SyntaxError` at runtime, not a YAML parse error) —
  because 3 of the 4 target steps currently lack `continue-on-error` (§3), such a syntax
  error would fail the job, not just the step. **Recommend testing gate edits via `act` or
  a throwaway branch/PR before merging**, since GitHub Actions has no offline YAML+embedded-JS
  linter that catches this class of error pre-push.
- **Env var propagation:** all 5 workflows already use the `steps.<id>.outputs.<name>` →
  `env:` → `process.env.X` pattern correctly (e.g. `ux-analysis.yml:153`,
  `registry-validation.yml:76-81`) rather than reading `$GITHUB_OUTPUT` directly inside the
  script block — this is the correct, already-established pattern in this repo; no new
  propagation mechanism is needed for the gate logic, just new fields if a job needs to
  compare "current" vs "prior" state (e.g. AC #4's build.yml coverage-changed check, which
  needs some notion of "prior state" not currently captured in any step output — worth
  flagging as a genuine open design question, since there's no cache/baseline artifact for
  feature-coverage the way `benchmark.yml` has for benchmark baselines, `benchmark.yml`'s
  own top-of-file comment block explains its cache-based baseline approach at lines 21-30 as
  the pattern to potentially reuse).
- **`deleteComment`/`updateComment` permission scope:** all 5 workflows already successfully
  call `github.rest.issues.createComment`/`updateComment` on PR issue threads using only
  `pull-requests: write` (no `issues: write` anywhere in any of the 12 workflow files —
  grepped, zero hits). Per GitHub's documented permission model, PR comments are served by
  the Issues Comments REST API (`/repos/{owner}/{repo}/issues/{number}/comments`,
  `/repos/{owner}/{repo}/issues/comments/{comment_id}`) regardless of whether the target
  `issue_number` is a PR or a plain issue, and `create`/`update`/`delete` on that endpoint
  family are gated by the same permission tier. Since `createComment`/`updateComment`
  already work today with only `pull-requests: write` (confirmed working in production per
  these very workflows), `deleteComment` should need no additional permission — **this is
  INFERRED from the shared API family and GitHub's documented permissions table
  (https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication#permissions-for-the-github_token),
  not verified by actually calling `deleteComment` in this repo's CI**, since nothing here
  does so yet (§2). Recommend a real dry-run (e.g. a scratch workflow_dispatch step against
  a throwaway comment on a test PR) before relying on this in the shipped gate, rather than
  discovering a `403` only when the first real stale comment needs deleting.
