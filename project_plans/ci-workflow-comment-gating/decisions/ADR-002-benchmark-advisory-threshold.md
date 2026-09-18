# ADR-002: Advisory regression threshold for `benchmark.yml`'s PR-comment gate

**Status**: Proposed — amended 2026-08-12 to split the threshold by measurement type
(see Amendment in Context) after adversarial review Blocker 2; flagged in `plan.md`'s
Unresolved Questions #3 for explicit confirmation
**Date**: 2026-08-12

## Context

`benchmark.yml` has zero existing regression-detection logic today across all 3 of its PR
jobs (go-tier1, frontend-throughput, e2e-latency) — each posts raw benstat/pct-delta text
unconditionally, with no threshold check anywhere (stack.md, features.md §3). This item (AC #1)
requires gating each job's comment on whether the run shows a "real regression."

`build.yml`'s `benchmark-gate` job already implements a calibrated, **blocking** regression
check for main-branch pushes: `-delta-test=utest` for statistical significance, `count=5`
samples, and a documented 20% threshold (`build.yml:376-379`'s comment: "20% threshold is
deliberate — tmux-touching benchmarks have 15-25% natural variance on shared runners").

Pitfalls research (pitfalls.md §4) identified this as the single highest-risk part of the
entire item: a benstat plain-text output gate is far more fragile than `ux-analysis.yml`'s
clean numeric `score < 70` comparison, because benstat's output format/significance markers
aren't a fixed vocabulary this repo controls. Two specific failure modes were named:
- A keyword match (e.g. searching for the literal word "regression") would be a **100% false
  negative** — benstat's output never contains that word, so no run would ever be flagged,
  including real regressions.
- A gate keyed on the mere presence of a `%` sign would **false-positive** on statistically
  insignificant noise (benstat marks those with `~`, not a clean omission).

**Amendment (2026-08-12, adversarial review Blocker 2)**: the original version of this ADR
applied the same 20% number to all three jobs uniformly. That was wrong for two of the three.
`build.yml`'s 20% threshold is calibrated specifically for a **repeated-trial, statistically
tested** measurement: `-delta-test=utest` plus `count=5` (go-tier1 uses `count=8`), which
establishes significance before the percentage is even evaluated — the 15-25% natural-variance
figure it documents is the variance *of that significance-tested statistic*, not of a single
raw sample. `frontend-throughput` and `e2e-latency` are **single-sample** Playwright
measurements — one run, diffed against one cached baseline value, with no repeated trials and
no significance test anywhere in the file (confirmed:
`grep -in "varia\|noise\|flak" .github/workflows/benchmark.yml` returns nothing for these two
jobs). Nothing in this repo has ever measured the run-to-run noise floor of a single Playwright
sample on a shared GitHub runner, and browser-driven E2E timing is typically noisier than a
significance-tested Go micro-benchmark. Transplanting 20% onto these two jobs risked exactly
the two failure modes this ADR exists to avoid: (a) the gate fires on ordinary runner jitter,
reproducing the spam problem this item exists to fix, or (b) a real regression under 20% goes
completely unflagged with the raw dump now gated away and no fallback visibility.

Adding repeated trials to these two Playwright jobs so a significance test becomes possible
(mirroring go-tier1's approach) was considered and rejected for this item's scope: it requires
re-architecting how `frontend-throughput`/`e2e-latency` invoke Playwright (loop N times,
aggregate, then diff), which is a materially larger change than "add a threshold check" and
belongs to a follow-up, not this item's Low-Medium RICE effort rating.

## Decision

Two different thresholds, not one, split by whether the underlying measurement is
significance-tested:

- **go-tier1** (statistically tested — `count=8` + `-delta-test=utest`): keep the original
  **20% threshold**, reusing `build.yml`'s exact regex verbatim —
  `grep -E '\+([2-9][0-9]|[1-9][0-9]{2,})\.' benchstat-tier1.txt | grep -v '±'` — against
  `benchstat -delta-test=utest` output. Unchanged by this amendment.
- **frontend-throughput** and **e2e-latency** (single-sample, no significance test):
  downgrade to a much coarser **"obvious swing" threshold — a 2x change (double or half)** —
  rather than reusing 20%, and treat it explicitly as an unvalidated placeholder, not a
  calibrated number (per requirements.md's item-scope option "(c)" for gates that can't support
  statistical rigor):
  - **frontend-throughput**: `hasRegression = rows.some(r => r.pctNum <= -50)` (throughput
    dropped to half or worse of baseline; higher is better, so the regression direction is
    negative).
  - **e2e-latency**: `hasRegression = rows.some(r => r.pctNum >= 100)` (latency doubled or
    worse vs. baseline; lower is better, so the regression direction is positive).
  A 2x swing is chosen specifically because it is large enough that ordinary single-sample
  runner jitter is very unlikely to produce it by chance, even with zero noise-floor data to
  calibrate against — the goal is "only comment on something unmistakable," not "catch every
  real regression," for these two jobs specifically.

## Consequences

- go-tier1: unchanged from the original decision. A regression must be fairly large (≥20%) to
  generate an advisory comment; go-tier1's `count=8` gives it statistical power at least as
  good as `build.yml`'s blocking gate (`count=5`), so reusing 20% there is conservative, not
  under-powered.
- frontend-throughput / e2e-latency: these two jobs will now miss real regressions in the
  20%-99% range entirely — no comment, no step-summary flag beyond the unconditional raw
  numbers written per the Observability Plan. This is a deliberate, scope-bounded trade: it is
  strictly better than the pre-item baseline (an unfiltered dump nobody reads, per the Kano
  "basic expectation" framing) precisely because it will not fire on noise, but it is
  materially less sensitive than go-tier1's gate. This gap is the direct reason a
  repeated-trial follow-up (see rejected alternative above) is worth doing later if these two
  paths turn out to matter as regression-catchers rather than pure noise-avoidance advisories.
- If the team later adds repeated Playwright trials to these two jobs, this ADR's 2x threshold
  should be revisited and likely tightened toward go-tier1's 20% once real noise-floor data
  exists — tracked as a follow-up, not blocking this item.

## Alternatives Considered

- **Reuse the same 20% threshold for all three jobs** (the original version of this ADR).
  Rejected on amendment: no evidence exists that a single Playwright sample's noise floor is
  anywhere near go-tier1's significance-tested variance; 20% risks both false-positive noise
  and false-negative silent misses for the two single-sample jobs (see Amendment above).
- **A lower, advisory-specific threshold (e.g. 10%) for go-tier1**, to warn earlier since this
  is non-blocking. Rejected: `build.yml`'s own documented 15-25% natural variance for these
  benchmarks means a 10% threshold would fire on pure runner noise on a meaningful fraction of
  runs, directly undermining this item's goal.
- **Add repeated trials (e.g. `count=5`) to frontend-throughput/e2e-latency** so a real
  threshold could be calibrated like go-tier1. Rejected for this item's scope as a
  disproportionate rewrite of how those two jobs invoke Playwright — named as a follow-up in
  Consequences instead.
- **A keyword/text-presence match** against benstat output. Rejected outright per pitfalls.md
  §4 — benstat's output vocabulary doesn't contain the words such a gate would look for,
  making it a total, silent false negative.
