# Cutover gate results

Shared log for Epic 8 (Soak and cutover) stories. Each story appends its own dated section rather than overwriting another's; do not clobber existing entries.

## Story 8.4: Soak runbook and exit criteria

PR 8 has not merged yet, so no soak window has started and none of the dated fields below are filled in. This entry exists to carry the mechanism and defaults into place now, per the plan's AC, and will be appended to (not replaced) once PR 8 merges.

**Defaults (plan, Story 8.4; owner may lengthen, never shorten — record any change below with a reason):**

- First Soak window (PR 8 -> PR 8b gate): >= 14 calendar days and >= 3 distinct dev sessions after PR 8 merges, with zero reverts and no open P1 `web-build` issue.
- Second Soak window (PR 8b -> PR 9 gate): >= 7 further days after PR 8b merges.
- T3 (ADR-007 Vite 8 fallback trigger) is counted from CI across the whole Soak window: a Bun bundler crash or miscompile in >= 2 CI runs fires it, regardless of which PR's CI run it appears in.

**PR 9 deadline reminders (pre-mortem failure mode 9):** once PR 8 merges on date D, two dated reminders are created — D+21 (earliest PR 9) and D+45 (stop-loss decision point) — via `mcp__stapler-squad__schedule_wake` or a scheduled routine (see `docs/how-to/build-web-app.md`'s Soak section for the mechanism). Their links will be recorded here as a new dated entry once PR 8 actually merges; no date is fabricated in advance of that merge.

**Stop-loss principle:** the dual-build tax (two CI cells, aliases, conditionals in tests) must not become permanent. If PR 9 has not merged by D+45, the owner decides in writing — recorded as a new entry in this file — between finishing PR 9 and reverting to Next outright. Epics 1, 2c, 2d, and 7a remain valuable under Next either way, so a stop-loss revert at D+45 does not waste that work.

<!-- Next entry: owner's lengthen/shorten decisions, actual Soak dates, T3 occurrences, and the dated-reminder links, appended here once PR 8 merges. -->
