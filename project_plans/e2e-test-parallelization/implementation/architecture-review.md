# Architecture Review: e2e-test-parallelization
**Date**: 2026-10-25
**Verdict**: CONCERNS

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo (verified: file absent). No constitution constraints apply — N/A, not a violation.

## Methodology

This plan is CI/test-infrastructure only (GitHub Actions YAML, a Playwright config flag, a new non-gating job, two design-only docs) — no domain model, no persistence layer, no business logic. SOLID/DDD/type-driven-design/GoF-pattern lenses mostly report N/A below, as the task brief anticipated; I did not force-fit findings where none existed.

Grounding performed (not just reading plan.md prose):
- Confirmed the local working tree is 27 commits behind `origin/main` and does **not** yet contain PR #1166's shard matrix; all verification below was done against `origin/main` via `git show origin/main:<path>`, matching the plan's own stated baseline commit (`0551de0b858b…`, 1 commit behind current `origin/main`).
- Verified byte-exact: `playwright.config.ts:53/59` (`fullyParallel: false`/`workers: 1`), `build.yml:392,559` (`npx playwright install chromium firefox webkit --with-deps`), `build.yml:670` (`web-build-gate`'s `needs:` list), `web-build-gate.mjs:6` (`DEFAULT_MEMBERS`), the two call sites `connection-count-indicator.spec.ts:39`/`launcher-presets.spec.ts:21`, and `e2e-video.yml:171`'s chromium-only precedent. All match the plan's citations exactly.
- Verified the cited CI run (`gh run view 40184788547`) is real and its 20 shard-job start/end timestamps reproduce the plan's exact shard-duration figures (`e2e-next-shard` #6 15m49s / #10 31m36s; `e2e-bun-shard` #6 15m48s / #8 34m4s) to the second.
- Verified `scripts/dev-stack/web-build-mode.ts`'s `parseWebBuildMode`: unset `WEB_BUILD` defaults to `"next"`, so `e2e-next-shard`'s explicit `WEB_BUILD: next` and the new job's planned unset value are behaviorally identical (not a bug, see Nitpicks).
- Ran `kibitzer run tests/e2e/helpers --trigger batch` and `kibitzer run web-app/scripts/ci --trigger batch`: no findings on any file this plan touches (`playwright.config.ts`, `web-build-gate.mjs`); pre-existing advisories on unrelated, untouched helpers (`session-client.ts`, `a11y.ts`, `test-server.ts`) are out of this plan's diff scope and not reported as findings against it.
- Live-tested Story 2.1.1's proposed command against the real account (see Concerns).

Overall: this plan is unusually well-grounded — every line-number/file citation I checked against `origin/main` was accurate. No BLOCKER-level structural issues found.

## Blockers

(none)

## Concerns

- [ ] **Story 2.1.1 / Task 2.1.1a** — The proposed command `gh api /user --jq '.plan.name'` does not work for this account: live-tested, the `/user` response for this personal account has no `plan` key at all (`gh api /user --jq 'keys'` lists no `plan` field), so the command silently prints nothing rather than erroring. The AC's fallback ("ask Tyler... if the API call 403s") only triggers on an HTTP error, not on a silent empty/null result — an implementing agent could misread the empty output as "plan tier queried, found empty" and write a wrong or blank finding into `concurrency-ceiling-finding.md` instead of falling back to the billing UI. **Remediation**: change Task 2.1.1a to explicitly check for an empty/null result (not just a non-2xx status) before falling back, e.g. `gh api /user --jq '.plan.name // "NOT_EXPOSED"'`, and fall back to `https://github.com/settings/billing` whenever the result is `NOT_EXPOSED`, not only on a 403.

- [ ] **Story 3.2.1 (Lens 1, testability)** — `web-build-gate.test.mjs`'s existing suite (verified in full against `origin/main`) tests the gate's own pass/fail logic and one structural invariant (every `strategy.matrix` block sets `fail-fast: false`), but has no test asserting that a new informational job (e.g. the planned `e2e-secondary-browsers`, or `storybook-build` before it) stays out of `web-build-gate`'s `needs:` array and out of `DEFAULT_MEMBERS`. Task 3.2.1c's check for this is a one-time manual `grep` run by the implementing agent, not a durable regression test — exactly the kind of workflow-membership mistake that caused this session's own #1165 chronic-red-gate incident (cited in this plan's own Pattern Decisions row). **Remediation**: add one `node:test` case to `web-build-gate.test.mjs` asserting `web-build-gate`'s `needs:` array (parsed from `build.yml`, same regex-extraction style the existing structural test already uses) does not contain `e2e-secondary-browsers` (or, more durably, contains exactly the 5 named members) — cheap to add now, in the same PR, while the job is being introduced.

## Nitpicks

- Pre-existing magic-number duplication (Lens 2): the shard count `10` is a literal duplicated in 4 places in `build.yml` — two job-name template strings (`"... shard ${{ matrix.shard }}/10"`) and two `matrix.shard` arrays (`[1, 2, ..., 10]`), with no single source of truth. Not touched by this plan (Phase 3 explicitly keeps shard count at 10), so not a blocking finding here, but worth a one-line note since Story 2.1.1's own finding could eventually motivate a shard-count change, which would then require four synchronized edits instead of one.
- Task 3.2.1b describes the new `e2e-secondary-browsers` job running "against the Next build (`WEB_BUILD` unset, same as `e2e-next-shard`)." This is functionally correct (`parseWebBuildMode` defaults unset to `"next"`) but stylistically inverted from the codebase's own convention: `test-server.ts`'s comment at the `WEB_BUILD` assignment site says it's set explicitly "so a typo fails here, not as a silent Next build." Recommend the new job explicitly set `WEB_BUILD: next` (mirroring `e2e-next-shard`'s own style) rather than relying on the implicit default, for consistency with that stated rationale — purely cosmetic, no functional risk either way.

## Lens-by-Lens Notes (for completeness)

- **Lens 1 (SOLID/layer coupling)**: N/A for SOLID (no OOP domain code). Layer coupling checked and clean: the new `e2e-secondary-browsers` job is verified absent from both `web-build-gate`'s `needs:` and `DEFAULT_MEMBERS`; `fullyParallel: true` leaves `workers: 1` unchanged per the plan's own AC, verified against `origin/main`.
- **Lens 2 (type-level design)**: N/A — YAML config and a single boolean flag; no illegal-state/primitive-obsession pattern applies beyond the pre-existing shard-count literal noted above.
- **Lens 3 (pattern selection)**: Consistent. `fullyParallel: true` over a third-party balancer matches `build-vs-buy.md` §1's verdict; the deferred seam's "adapt `TestServer` + worker-scoped fixture, not bespoke orchestration" matches `build-vs-buy.md` §3-4's verdict, and Story 5.1.1 cites the correct sections and exact call sites (verified).
- **Lens 4 (tech debt trajectory)**: The "Isolate via seam" disposition is backed by a concretely-named task (Story 5.1.1, `docs/explanation/e2e-per-worker-server-seam.md`) that correctly cites `architecture.md` §2-3 and `build-vs-buy.md` §3-4 and names the two real call sites (verified byte-exact). No implement-now story deepens the shared-server-constraint violation — `workers: 1` is explicitly preserved. ADR-002's staging ("fullyParallel before any third-party balancer") is trivially satisfied since no third-party-balancer story exists in this round at all.
