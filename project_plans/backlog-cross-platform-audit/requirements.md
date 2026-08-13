# Backlog Feature — Cross-Platform Reliability Audit

## Origin

Resumed session (previous session exited unexpectedly). The backlog feature (autonomous
triage + review pipeline) has been built across three prior SDD efforts
(`backlog-management`, `backlog-triage-autonomous`, `backlog-triage-e2e-hardening`) plus a
later feature-flag gating pass, but the user has only ever seen it *partially* work on a
work Mac laptop, and has **never** seen it work on this machine (Linux). There is no
confidence the autonomous AI flows behave consistently across machines.

## Ask

Not a new feature. This is a diagnostic/audit pass:

1. Understand what has actually been implemented (vs. planned) for the backlog feature.
2. Identify gaps, risks, or problems controlling sessions that block reliable operation —
   especially anything platform-dependent (Mac vs. Linux).
3. Step through and document the user journey end-to-end, marking each step as
   implemented/tested, implemented/untested, or not implemented.
4. Produce a report the user can act on to decide what to fix next.

## Known lead

`config.GetFeatureFlag("backlog")` defaults to `false` (config/feature_flags_test.go). The
backlog nav item and RPCs are gated behind this flag (commits `66b1d831`/`1186494b`
"gate backlog behind feature flag on all layers", `3c2f739a` "gate Backlog nav item behind
feature flag"). This alone would explain "never seen it work on this computer" if the flag
was never enabled locally — but is not assumed to be the only issue; autonomous session
control (tmux control mode, headless pool, safeexec) has its own history of platform-specific
fixes (`fix(session): fix ControllerManager data race`, `fix(terminal): prevent multi-tab
disconnect`, `fix(detection): detect indented spinners`, etc.).

## Out of scope

- Writing new features or fixing bugs in this pass (that's a follow-on decision).
- Re-deriving what's already documented in prior `adversarial-review.md` / `pre-mortem.md`
  / `architecture-review.md` files — mine them, don't ignore them.

## Deliverables

- `research/implementation-inventory.md`
- `research/cross-platform-risks.md`
- `research/test-coverage.md`
- `user-journey.md` (top-level, the primary requested artifact)
- `gaps-and-risks.md` (top-level synthesis)
