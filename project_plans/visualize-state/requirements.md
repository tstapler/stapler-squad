# Requirements: Session State Visibility & Triage UX

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-14

## Problem Statement

Users managing multiple AI agent sessions (Claude Code, Aider, etc.) cannot efficiently triage which sessions need attention. Three compounding problems make this worse:

1. **Inaccurate status** - "Running" and "Ready" statuses in the Sessions page do not reflect actual session state. Users cannot trust the UI and must manually check each session.
2. **Unstable review queue** - The review queue continually jumps, changes terminals, and reloads, making it annoying and disorienting to use.
3. **Disruptive hook approval UX** - When pending hook approvals appear, they switch the active terminal view, interrupt the current work context, and require navigating away to approve. Users cannot approve without losing what they were doing.

The product is open source with a broad user base, so these UX problems affect all users running multiple sessions concurrently.

## Success Criteria

- **Triage speed**: User can scan all sessions and identify which need attention in under 30 seconds
- **No context interruptions**: Hook approvals and status changes do not pull the user away from their current session view
- **Trusted status**: Running/Ready/Waiting/Blocked statuses are accurate enough that users rely on them without manual verification
- **Stable review queue**: Queue does not jump or reload during active review

## Scope

### Must Have (MoSCoW)
- Accurate session status (Running / Ready / Waiting for input / Blocked on approval)
- Refined status labels - surface rich state already detected in backend (e.g., "Claude is waiting for your input") in the web UI
- Terminal preview / snapshot - show last N lines of terminal output on the Sessions page without navigating to that session
- Non-interrupting hook approval UX - approve pending hooks from a sidebar, overlay, or badge without losing the current session view
- Stable review queue - eliminate jumping/reloading behavior during active review

### Out of Scope
- New session creation flows
- Search and filter improvements (existing implementation is sufficient)
- Typing/sending input to sessions from the sessions list view
- Mobile/responsive layout optimization

## Constraints

**Tech stack**: React web UI (`web-app/src/`), Go backend, ConnectRPC for streaming
**Status detection**: Backend already detects refined session state by watching terminal output patterns (regex/pattern matching on scrollback buffer) — needs to be wired to frontend
**Terminal streaming**: Infrastructure already exists via ConnectRPC — terminal preview can leverage this
**Branch**: `claude-squad-visualize-state` (work is already scoped here)
**Timeline**: Not fixed
**Dependencies**: Session status model in Go, terminal scrollback buffer system

## Context

### Existing Work
- Terminal streaming via ConnectRPC is functional
- Backend pattern-matches terminal output to detect rich session states (e.g., "waiting for input")
- Sessions page exists with status display — status is stale/inaccurate
- Hook approval mechanism exists but interrupts active work

### Stakeholders
- Open source users running multiple concurrent AI agent sessions
- Primary persona: solo practitioners or small teams managing 3–20+ simultaneous sessions

## Research Dimensions Needed

- [ ] Stack — evaluate options for terminal snapshot rendering (ANSI rendering in React, snapshot format, polling vs. push)
- [ ] Features — survey how other multi-session tools (tmux web UIs, terminal multiplexers, CI dashboards) handle session status triage and approvals
- [ ] Architecture — design patterns for non-interrupting notification/approval flows; how to wire existing backend status detection to frontend
- [ ] Pitfalls — known failure modes for ANSI rendering in browser, race conditions in status updates, WebSocket/streaming reconnect edge cases
