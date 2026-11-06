# BUG-109: Android Chrome touch-drag scrolling is one-directional and does not reach the TUI scrollback [SEVERITY: High]

**Status**: 🐛 Open
**Discovered**: 2026-10-01, SDD project `scrolling`.

## Problem Description

On Android Chrome, dragging the terminal scrolls in one direction only (drag-down is dead), does not
scroll Claude Code's own scrollback, and has no momentum. See
[`project_plans/scrolling/requirements.md`](../../../project_plans/scrolling/requirements.md).

## Fix

Mode-routed scroll modules (`web-app/src/lib/terminal/`) plus gesture-hook integration; see
[`project_plans/scrolling/implementation/plan.md`](../../../project_plans/scrolling/implementation/plan.md)
(Epics 1.1, 1.2).

## Verification (how this closes)

Device checklist D1/D2 in plan.md Story 3.1.3 passes on a physical Android phone. Move this file to
`docs/bugs/fixed/` only after that record exists.
