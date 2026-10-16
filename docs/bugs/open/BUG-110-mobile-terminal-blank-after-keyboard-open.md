# BUG-110: Terminal intermittently renders blank after the Android soft keyboard opens [SEVERITY: High]

**Status**: 🐛 Open
**Discovered**: 2026-10-01, SDD project `scrolling`.

## Problem Description

After the soft keyboard opens (and sometimes closes) on Android Chrome the xterm canvas intermittently
stays blank until further interaction. See
[`project_plans/scrolling/requirements.md`](../../../project_plans/scrolling/requirements.md).

## Fix

Single-owner fit, repaint seam (`refresh`), `ViewportSettled` signal and resize bounce-hold bypass; see
[`project_plans/scrolling/implementation/plan.md`](../../../project_plans/scrolling/implementation/plan.md)
(Epic 2.1).

## Verification (how this closes)

Device checklist D7 (blank count over N keyboard open/close cycles, N = ceil(3 / baseline rate)) recorded
in plan.md. Move this file to `docs/bugs/fixed/` only after that record exists; if no device run happens,
the D7 claim stays unproven.
