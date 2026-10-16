# Triad Review: lower-rework-turn-caps

**Date**: 2026-10-07
**Iteration**: 1 (initial review + one repair pass)

## Triad Readiness

| Lens        | Round 1        | Round 2 (after patch) | Key Gaps (original)                                |
|-------------|----------------|-----------------------|----------------------------------------------------|
| Product     | 🟡 needs-work  | 🟢 ready              | No target user def; no outcome metric; no named assumption |
| UX Design   | 🟡 needs-work  | 🟢 ready              | `aria-describedby` missing; loading state undocumented; clamping UX tradeoff unacknowledged |
| Engineering | 🟢 ready       | 🟢 ready              | Line numbers may have shifted; `AutonomousMaxTurnsDefault` export unconfirmed |

## Overall: READY TO BUILD

No blockers on any leg. One repair pass resolved all three PM gaps and all three UX gaps.

### Gaps fixed in repair pass

**PM — patched in requirements.md:**
- Added **Target User** section: "system operator" defined as the developer/DevOps engineer who manages the deployment.
- Added **Success Metric**: operator can view and change the cap through Settings UI and have it honored immediately by subsequent autonomous sessions.
- Added **Named Assumptions** section: assumption that operators find `config.json` editing painful enough to warrant a UI field — stated explicitly as unvalidated.

**UX — patched in design/ux.md:**
- Section 5 (Accessibility) updated: hint `<p>` must carry `id="global-autonomous-max-turns-hint"`; input must carry `aria-describedby="global-autonomous-max-turns-hint"` — DOM proximity alone does not satisfy WCAG 3.3.2 / 1.3.1.
- Section 3 (Interaction Flow) updated: loading state documented — field renders `0 || 30 = 30` during in-flight GET (no blank flash), matching adjacent fields.
- Section 3 updated: clear-and-retype limitation acknowledged as a known, intentional tradeoff of the shared clamp pattern; `select-all-and-type` documented as the workaround.

### Engineering gaps (non-blocking, no patch required)

- Line numbers cited in plan.md (e.g. `session.proto:2213`) may have shifted; implementer must verify insertion points against current file.
- `config.AutonomousMaxTurnsDefault` export status unconfirmed; test code may need `int32(30)` literal instead of the constant name until it is exported.
- Frontend load fallback `defaults.autonomousMaxTurns || 30` hardcodes the server default as a client constant — same pattern as existing fields, acknowledged documentation drift.
- `sampleDefaults` mock in `GlobalDefaultsForm.test.tsx` currently uses `maxAutoReworkIterations: 3` (pre-existing fixture gap) — plan already includes updating this fixture.

## Recommended Next Step

```
/sdd:5-implement lower-rework-turn-caps
```

Open a fresh session before implementing (phases 1–4 ran in this thread).
