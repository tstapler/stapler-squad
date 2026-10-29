# Post-Phase-3 Checkpoint Finding (Story 3.3.1 / Task 3.3.1a)

**Date**: 2026-10-26
**Verdict: (b) insufficient — escalate.** `percent_improvement = -43.3%` (a regression, not an improvement). Backlog item `0892086f-79c1-4b8d-a0bf-15867facdf4d` ("Schedule shared-server rearchitecture (e2e-test-parallelization Phase 5 follow-up)") opened per the plan's `< 10%` branch.

## Methodology caveat: the retroactive delta computation

Stories 3.1.2 and 3.2.2 (the "validate on real CI, record a delta in the PR body" stories) were never executed as their own explicit steps in this session — both PRs (#1196, #1197) merged with their "Real CI run on this PR" checklist box unchecked and no delta number in either PR body. This checkpoint recomputed both deltas retroactively from the GitHub Actions API rather than reading them off an already-recorded number.

**Worse than just "not recorded": PR #1196's own CI run never actually exercised the change.** `build.yml`'s "Detect Relevant Changes" job classifies relevance via a hardcoded grep regex (`\.go$|^go\.(mod|sum)$|^Makefile$|^install\.sh$|^proto/|^web-app/src/|...`) that has **no pattern matching `tests/e2e/`**. PR #1196 touched only `tests/e2e/playwright.config.ts`; its only CI run (`40297913960`) logged `Changed files: tests/e2e/playwright.config.ts` → `relevant=false`, and every `e2e-next-shard`/`e2e-bun-shard` job shows `conclusion: "skipped"` with the literal unexpanded matrix name. `web-build-gate` still passed (skipped jobs aren't failures), so the PR merged clean, but **Story 3.1.2's acceptance criteria — comparator fail/flaky-count, the 54-file shard-split review, before/after shard-duration spread — were never actually run against real CI data before merge.** Filed as backlog item `2fd3a692-9a6c-4fe7-9248-46fc60fa7533` (fix the regex) since this is a real gap that would silently repeat for any future test-content-only PR.

Because of that gap, `final_delta` below is taken from PR #1197's own validating run instead of PR #1196's — PR #1197's branch (`e2e-browser-install-isolation`) was created after #1196 merged to `main`, so its head commit (`72e80b7d9082745f64fbe4cdfb894e2be922f6a2`) already carries `fullyParallel: true` (confirmed: `git show 72e80b7d9...:tests/e2e/playwright.config.ts` shows `fullyParallel: true` at line 59). Run `40380966243` is therefore already a **combined** measurement of both Phase 3 levers, not browser-isolation alone — which incidentally satisfies most of this checkpoint's own "validate both levers together" requirement (AC #3) one run earlier than the plan anticipated.

**Interpretation of "slowest-shard-start"**: in every run inspected (baseline and both PR runs), all `e2e-*-shard` jobs in a given run start within 1-2 seconds of each other (GitHub Actions dispatches a `strategy.matrix` as one batch). So "slowest-shard-start" and "earliest-shard-start" are numerically indistinguishable here — the ambiguity the task flagged does not affect the computed delta. The delta measured below is **wall-clock from shard-matrix dispatch to `web-build-gate` job completion** — the gate wall-clock a PR author/the post-merge `main` state actually waits through, matching architecture.md §7's "wall-clock-to-verdict" framing.

## The three numbers

| Run | Role | Earliest shard `started_at` | `web-build-gate` `completed_at` | Delta (min) |
|---|---|---|---|---|
| [`40184788547`](https://github.com/tstapler/stapler-squad/actions/runs/40184788547) | baseline (pre-Phase-3, cited by architecture.md §7) | 2026-10-25T11:02:42Z | 2026-10-25T11:38:26Z | **35.73** (35m44s) |
| [`40297913960`](https://github.com/tstapler/stapler-squad/actions/runs/40297913960) | PR #1196's own run | n/a — e2e jobs skipped, see caveat above | — | not usable |
| [`40380966243`](https://github.com/tstapler/stapler-squad/actions/runs/40380966243) | PR #1197's final run (head already has both levers) | 2026-10-26T19:09:11Z | 2026-10-26T20:00:24Z | **51.22** (51m13s) |

```
percent_improvement = (baseline_delta - final_delta) / baseline_delta
                    = (35.73 - 51.22) / 35.73
                    = -43.3%
```

Verified via `gh api repos/tstapler/stapler-squad/actions/runs/<id>/jobs --jq '.jobs[] | select(.name | test("e2e.*shard|web-build-gate")) | {name, started_at, completed_at, conclusion}'` against all three runs.

### Shard-duration spread (sanity check against architecture.md §6/§7)

| Run | `e2e-next-shard` fastest/slowest | ratio | `e2e-bun-shard` fastest/slowest | ratio |
|---|---|---|---|---|
| `40184788547` (baseline) | 15m49s / 31m36s | 2.0x | 15m48s / 34m4s | 2.16x |
| `40380966243` (final, both levers) | 17m52s / 42m2s | 2.35x | 20m5s / 49m46s | 2.48x |

The baseline numbers reproduce architecture.md §6/§7's cited figures exactly (confirms this measurement method is consistent with the plan's own prior analysis). **The spread got worse, not better, after `fullyParallel: true` + chromium-only installs** — both the ratio and the absolute slowest-shard duration increased. `fullyParallel: true` only changes how `--shard` partitions tests across jobs; it does not change `workers: 1`'s in-job serial execution, and the chromium-only install change added two new concurrent `e2e-secondary-browsers-*` jobs competing for the same runner pool. A plausible (unverified — would need multiple repeated runs to separate from one-off GH-hosted-runner noise) explanation is increased queueing/contention from the extra job pair plus normal run-to-run variance, which architecture.md §6 already documented as large (2.0x-2.2x) even on an unchanged 10-way split. This checkpoint does not have enough repeated-run data to attribute the regression definitively to either lever, the added concurrent job pair, or ambient noise — stated as a gap, not papered over.

## Threshold applied

Per Story 3.3.1's AC: `percent_improvement < 10%` → verdict **(b) insufficient — escalate**. `-43.3%` is far below the floor. Backlog item `0892086f-79c1-4b8d-a0bf-15867facdf4d` opened, linking `docs/explanation/e2e-per-worker-server-seam.md` and this note.

## Combined-interaction check (pre-mortem P2 item #3)

First post-both-merges `main` CI run: [`40392784895`](https://github.com/tstapler/stapler-squad/actions/runs/40392784895) (triggered by #1197's merge commit `28b1685359d16f9902e7ac736bfe2bb4f39b8150`), completed 2026-10-26T20:54:39Z. Workflow-level `conclusion: "failure"` — but caused entirely by `e2e (Bun build)` (the comparator/merge job, same job that also failed on PR #1197's own run) and the two `continue-on-error` secondary-browser jobs, none of which are in `web-build-gate`'s `needs:`. `web-build-gate` itself: **success**.

Comparison against run `40380966243` (PR #1197's own validating run — the more recent of the two stories' individually-validated numbers, since it already carries both levers):

| Metric | `40380966243` (PR #1197) | `40392784895` (first post-merge `main`) | Verdict |
|---|---|---|---|
| Wall-clock shard-start → `web-build-gate` completion | 51.22 min | 44.9 min | improved |
| `e2e-next-shard` fastest/slowest spread | 17m52s/42m2s = 2.35x | 15m49s/42m26s = 2.68x | slightly worse, but within the same order of magnitude as architecture.md §6's own 2.0-2.2x *unchanged-split* baseline variance — not treated as a new regression |
| `e2e-bun-shard` fastest/slowest spread | 20m5s/49m46s = 2.48x | 21m42s/43m37s = 2.01x | improved, back near baseline's 2.16x |
| Comparator `fail`-severity count | 33 | 27 | improved — matches baseline run's count (27) exactly |
| Comparator `report`-severity count | 184 | 181 | improved — matches baseline's count (181) exactly |
| "flaky → fail" escalation count | 5 | 5 | unchanged |

**Spec-level diff of the 27 `fail`-severity findings in the `main` run against the 33 in PR #1197's run**: 3 findings appear in `main` that weren't `fail`-severity in #1197's run — `backlog-copy-link.spec.ts`'s `copyLink_should_AnnounceViaAriaLivePolite_When_ActivatedViaKeyboard` and `..._RemainFocusedAfterActivation_When_TriggeredViaKeyboard`, and `backlog.spec.ts`'s `No items match filters shows empty message`. All three were `report`-severity "Shared flake: fails on both e2e-next and e2e-bun" in PR #1197's run and flipped to `fail`-severity "New Bun-only failure" in the `main` run (same test, failed on Bun only this time instead of on both builds) — i.e. the identical already-known "New Bun-only failure"/"Shared flake" flakiness class recurring against a different random subset of specs, not a new failure signature. Per this checkpoint's instruction to only flag a genuinely new signature, **no new regression was found**, and no revert was triggered.

**Verdict for this check: clean.** The combined run shows no regression beyond what architecture.md already documents as normal run-to-run shard-timing noise, and the comparator's fail/report/flaky counts hold steady or improve relative to PR #1197's own isolated validation.
