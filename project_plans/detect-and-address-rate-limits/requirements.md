# Requirements: Detect and Address Rate Limits

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-05

## Problem Statement

When LLM programs (Claude Code, Aider, etc.) hit API rate limits, they display a dialog requiring user interaction to continue once the rate limit reset time has elapsed. This requires manual intervention to press "continue" or "keep trying" - interrupting workflows and requiring users to monitor sessions.

## Success Criteria

Sessions automatically detect rate limits, wait for the reset time to elapse, and resume without user intervention. The "self-healing" behavior eliminates manual intervention for rate limit recovery.

Measurable outcomes:
- Rate limit dialogs are automatically detected within X seconds of appearing
- Sessions automatically resume after rate limit reset without user input
- Zero manual intervention required for rate limit recovery

## Scope

### Must Have (MoSCoW)
- Detect rate limit dialogs from terminal output for common LLM providers (Anthropic, OpenAI, Google, etc.)
- Parse rate limit reset timestamp from dialog messages
- Automatically send input to tmux session to continue/keep trying when reset time elapses
- Support multiple LLM programs (Claude Code, Aider, etc.)

### Out of Scope
- Rate limit prevention strategies (throttling, request scheduling)
- Modifying LLM client programs themselves
- Analytics or reporting dashboards

## Constraints

- **Tech stack**: Go, built on existing tmux session infrastructure
- **Timeline**: Not specified
- **Dependencies**: Existing terminal output streaming, existing tmux session management

## Context

### Existing Work
- Terminal output streaming is already implemented (captures output for web UI)
- tmux session management is already implemented (create, attach, send input to sessions)
- No prior work on rate limit detection or auto-resolution

### Stakeholders
- Users of Stapler Squad running LLM sessions who experience rate limits

## Research Dimensions Needed

- [ ] Stack — evaluate technology options for terminal output parsing and automation
- [ ] Features — survey rate limit dialog formats from different LLM providers
- [ ] Architecture — design patterns for detecting patterns in streaming output and triggering actions
- [ ] Pitfalls — known failure modes (false positives, missed detections, timing issues)
