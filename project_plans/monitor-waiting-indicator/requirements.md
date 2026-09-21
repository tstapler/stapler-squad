# Requirements: monitor-waiting-indicator

## Source

Backlog item `ae9ea980-c488-47dc-ba7e-a4b7ef2b7397`: "We need to support the claude waiting on shell/agent format"

## Problem

Claude Code CLI sessions render a footer status line when background work (a shell command or a `Monitor` tool call) is still in flight, e.g.:

```
still running (the deb build takes a while). I'll wait for the monitor's notification before touching the devstack again.

✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running
────────────────────────────────────────────────────────────────────
❯
────────────────────────────────────────────────────────────────────
  all tools: $3,162 MTD [! High Spend > $2,000] (as of 22h ago) · session: $33.58, 6h, 31 turns
  dashboard: go/aipi
  ⏵⏵ auto mode on · 1 shell, 1 monitor · ← for agents
```

stapler-squad manages Claude Code sessions in tmux panes and surfaces a derived session status (`Idle`/`Ready`/`Processing`/`NeedsApproval`/etc., via `SubStatus`) in the web UI — session cards, status badges, tag-organization-by-Status grouping.

**Verified scope correction (during planning, superseding the initial framing above):** stapler-squad already has a detector and UI chip for this status-line family — this is not greenfield work. The short "bottom status bar" form (`"⏵⏵ auto mode on · N shells, M monitors · ← for agents"`, aka the auto-mode footer) is already fully detected end-to-end and rendered as the `SubStatusChip`'s "⏳ Waiting for N Tasks" chip (`session/detection/detector.go`'s `autoModeFooterRegex`/`footerAgentCount`; `web-app/src/components/sessions/SubStatusChip.tsx`). The actual, narrower gap: the turn-completion long form shown in this item's description (`"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"`) — where a shell count and a monitor count are joined by a comma in one phrase — is undercounted. `shells_still_running`'s regex doesn't match when a comma-joined monitor phrase follows "N shell(s)", so detection falls through to the `monitors_still_running` pattern and silently drops the shell count (e.g. reports 1 instead of 2). This is a targeted bug fix to existing detection logic, not a new feature build. See `implementation/plan.md`'s "Scope Note" and "Verification of Current State" sections for the file:line evidence.

## Goal

Fix the comma-joined long-form undercounting so its combined shell+monitor count matches the precision the short footer form already has, so a user doesn't mistake "waiting on a background monitor" for "session is idle/needs input" regardless of which of the two status-line forms Claude Code is currently showing.

## Acceptance Criteria

1. Session output containing the `N shell, N monitor(s) still running` (and the shorter `N shell, N monitor` variant in the bottom status bar) is detected and parsed into a structured count of outstanding shells/monitors.
2. The web UI shows a distinct visual indicator (e.g. badge/icon) on a session when it has outstanding background shells/monitors, distinguishable from plain "idle" or "needs attention" states.
3. The indicator clears once the session output no longer reports outstanding shells/monitors (e.g. after the monitor's notification / completion line appears).
4. Detection works whether the session is in tmux control mode or legacy polling mode (`STAPLER_SQUAD_USE_CONTROL_MODE`).
5. New detection logic has unit test coverage (Go and/or TS depending on where it lands) using representative captured scrollback fixtures for both the "still running" and short status-bar forms.
6. No regression to existing status detection/tag-organization behavior for sessions without this footer format.

## Out of Scope

- Changing Claude Code CLI's own output format.
- General-purpose parsing of arbitrary CLI tool output beyond this specific status-line family.
