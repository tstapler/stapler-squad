# ADR-001: PII Scanning Defaults to Enabled + Escalate (Not Opt-In, Not Deny)

**Status**: Accepted
**Date**: 2026-08-06
**Context project**: `project_plans/pii-scanning/`

## Context

The PII scanner (`server/services/pii_scanner.go`, wired via `PIIScanningConfig` in
`config/types.go`) is a new, regex-based, no-ML detection layer added to the approval hook.
Unlike the existing secret scanner it's modeled on, it will produce a meaningfully higher
false-positive rate against realistic test-fixture data (`research/pitfalls.md` §1) — a bare
16-digit number, an `@example.com` address, or a sequential test SSN are all common,
legitimate content in an agent session working on this or any other repo.

Two independent defaults needed to be picked, both with real consequences either way:

1. **Is the feature on by default (`pii_scanning.enabled` defaults to `true`), or opt-in
   (`false`)?**
2. **On a match, does it escalate to human review (`on_detection: "escalate"`) by default, or
   auto-deny (`"deny"`)?**

## Decision

- `PIIScanningConfig.EnabledOrDefault()` returns `true` when `Enabled` is unset (nil) — **the
  feature is on by default**, not opt-in.
- `PIIScanningConfig.OnDetectionOrDefault()` returns `"escalate"` for any value other than the
  literal `"deny"` — **escalate is the default and the fail-safe fallback**, not auto-deny.
- A built-in `skip_path_patterns` default list (`testdata/`, `/fixtures/`, `/mocks/`, `_test.go`,
  `.test.ts`, `.test.tsx`, `.spec.ts`) ships active by default, to blunt the false-positive rate
  this decision otherwise accepts.

## Alternatives Considered

1. **Opt-in (`enabled` defaults to `false`)** — the feature does nothing until an operator
   explicitly turns it on.
   - *Pro*: zero risk of surprising a user with new review-queue noise on upgrade; matches how a
     brand-new, higher-noise detector might reasonably be introduced.
   - *Con*: defeats the point of a "built-in, first-class check" (requirements.md's own framing)
     — a security control nobody turns on protects nobody. Every other built-in approval-hook
     check in this codebase (secret-scan, domain-age) is unconditionally active; making PII-scan
     the one opt-in exception would be inconsistent and easy to forget to enable.
2. **Deny by default** — mirror secret-scan's auto-deny exactly.
   - *Pro*: maximally protective; no PII-bearing action ever proceeds without explicit
     reconfiguration.
   - *Con*: directly contradicts the issue's own stated intent and requirements.md's
     recommendation (ESCALATE, not DENY) — test-fixture PII is common and expected, and
     auto-denying it would hard-block routine agent work on any fixture-heavy repo, likely
     causing teams to disable the feature entirely (the "cry wolf" failure mode `features.md`
     §4.1 names explicitly) rather than tune it.
3. **Enabled by default + escalate by default, with a built-in fixture-path skip list**
   *(chosen)*.
   - *Pro*: the feature does its job (built-in, active, catches real PII) without secret-scan's
     hard-block failure mode; escalate puts a human in the loop exactly where the false-positive
     risk is highest (fixture-vs-real judgment calls); the skip list pre-emptively absorbs the
     single largest named false-positive source before a human ever sees it.
   - *Con*: still produces some review-queue noise on repos with fixture patterns the default
     skip list doesn't cover, until an operator tunes `skip_path_patterns` for their repo.

## Consequences

- A repo upgrading to this version of stapler-squad will see new `pii-scan` escalations in its
  review queue immediately, with no action taken — this is intentional, not a regression, but
  should be called out in the PR description and/or release notes so it isn't mistaken for a bug.
- The `pii_scanning.enabled: false` config key is the documented, supported way to fully opt out
  (see plan.md's Risk Control section) — this is the safety valve that makes "on by default" an
  acceptable default rather than a one-way door.
- Because escalate is the default and denies nothing outright, this decision can never make an
  agent session *more* blocked than it was before this feature shipped — the downside is bounded
  to reviewer-queue noise, not lost work, which is why "on by default" was judged acceptable
  where "deny by default" was not.

## References

- `project_plans/pii-scanning/requirements.md` (Open Questions 1–2)
- `project_plans/pii-scanning/research/architecture.md` §2
- `project_plans/pii-scanning/research/pitfalls.md` §1
- `project_plans/pii-scanning/research/features.md` §3.1, §4.1
- `project_plans/pii-scanning/implementation/plan.md` (Resolutions to Open Questions, Risk Control)
