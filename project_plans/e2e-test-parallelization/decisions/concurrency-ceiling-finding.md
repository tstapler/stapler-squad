# Finding: GitHub Actions concurrency ceiling (Story 2.1.1)

**Method:** `gh api /user --jq '.plan.name // "NOT_EXPOSED"'` → returned `NOT_EXPOSED`.
Confirmed this is because the `/user` response has no `plan` key at all for this
account (not a 403/error) — the `// "NOT_EXPOSED"` jq fallback is the catch path,
not an error path.

**Plan tier:** Undetermined via API. Needs a manual check at
https://github.com/settings/billing by Tyler — do not guess a tier.

**Known tier caps (for reference once the tier is confirmed):**
Free = 20, Pro = 40, Team = 60, Enterprise Cloud = 500 concurrent jobs.

**Current usage:** `build.yml` runs 20 shard jobs (`e2e-next-shard` × 10,
`e2e-bun-shard` × 10, via `strategy.matrix.shard`) plus ~15 other jobs
(`detect-changes`, `prepare`, `web-build-smoke-next`, `web-build-smoke-bun`,
`built-assets-guard`, `web-build-gate`, `storybook-build`, `web-supply-chain`,
`test-affected`, `test-tmux`, `test-delivery-guards`, `test-oracle`,
`test-rest`, `test`, `pty-race-regression`, `integration-coverage`, `build`,
`install-check`, `benchmark-gate`, plus the `e2e-next`/`e2e-bun` aggregators),
for roughly ~35 concurrent jobs on a single PR run.

**Verdict:** Undetermined pending Tyler's manual billing-page check. If the
account is on Free (cap 20), ~35 concurrent jobs already exceeds the cap today
and jobs are queuing/serializing; if Pro (cap 40) there is only slim headroom
before any further shard-count increase from this project's parallelization
work would exceed it; Team (60) or higher leaves comfortable headroom.
