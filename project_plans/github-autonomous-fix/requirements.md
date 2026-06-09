# GitHub Autonomous Fix — Requirements

**Project**: github-autonomous-fix  
**Date**: 2026-06-09  
**Status**: Draft

---

## Problem Statement

Stapler Squad can already manage Claude sessions in worktrees and has partial infrastructure for autonomous operation (`AutonomousMode` flag, `ClaudeController` idle detection, `steer_session` MCP tool, `RunWithResume`). However, there is no end-to-end flow that lets a user say "fix GitHub issue #123" and have Stapler Squad handle it autonomously without further input. The missing pieces are: an LLM orchestrator loop, reliable text injection timed to session readiness, GitHub PR support in the backlog plugin, goal-completion detection, and omnibar UX for the autonomous mode.

---

## Current State Inventory

### What EXISTS (do not re-implement)

| Component | Location | What it does |
|---|---|---|
| `ClaudeController` | `session/claude_controller.go` | Monitors PTY, detects status (idle/thinking/waiting), can write to PTY |
| `steer_session` MCP tool | `server/mcp/tools_terminal.go:614` | Sends text via PTY send_keys OR `--resume` subprocess |
| `RunWithResume` | `session/instance_claude.go:373` | Headless `claude --resume <uuid> -p <msg>` subprocess |
| `RunOneShot` RPC | `server/services/session_service.go:2881` | Headless `claude -p <prompt>` in worktree |
| `SpawnSessionFromItem` | `server/services/backlog_service.go:843` | Creates Claude session from backlog item with goal prompt |
| GitHub Issues backlog plugin | `session/backlog_plugin_github.go` | Fetches open GitHub issues as backlog items |
| `AutonomousMode` field | `session/instance.go:146` | Bool flag stored on instance — **no controller logic wired** |
| `PermissionMode` | `session/instance.go:223` | Passed as `--permission-mode` to Claude CLI |
| Auto-approval classifier | `pkg/classifier/classifier.go` | Classifies tool-use requests as safe/risky |
| `WriteToSession` RPC | `server/services/session_service.go:3130` | PTY text injection |
| GitHub URL detectors | `web-app/src/lib/omnibar/detector.ts` | Detects GitHub PR, branch, repo URLs in omnibar |

### What's MISSING (the gaps)

1. **LLM orchestrator loop** — `AutonomousMode=true` does nothing today. There's no goroutine that monitors a session, waits for idle state, and injects the next message using an LLM decision.

2. **Reliable injection timing** — `steer_session` send_keys path doesn't wait for Claude to be in an idle/ready state. It injects into the PTY whenever called, which may corrupt ongoing output.

3. **GitHub PR backlog plugin** — `backlog_plugin_github.go` fetches issues only. PRs have different semantics: diff, CI status, review comments, requested changes.

4. **Goal-completion detection** — After spawning an autonomous session, there's no mechanism to: detect success/failure, extract artifacts (PR URL, commit hash), and transition the backlog item.

5. **"Fix autonomously" omnibar mode** — The GitHub URL detectors exist but there's no creation mode that wires a GitHub issue/PR URL to an autonomous session.

6. **LLM-assisted approval decisions** — The approval handler has a classifier but no path to ask the headless LLM "given this session's goal, should I approve this tool call?"

---

## User Stories

### US-1: Fix GitHub issue from omnibar (primary flow)
**As a** developer  
**I want to** paste a GitHub issue URL in the omnibar and click "Fix autonomously"  
**So that** Stapler Squad creates a worktree session, fetches the issue context, and attempts to fix it without my involvement

**Acceptance criteria:**
- Pasting a GitHub issue/PR URL in the omnibar shows a "Fix autonomously" option alongside existing modes
- Selecting it creates an autonomous OneShot session in the repo's directory
- The session prompt includes: issue title, body, relevant labels, acceptance criteria derived from the issue
- The session uses `--permission-mode auto` (safe tools auto-approved, risky ones escalated)
- The session's `AutonomousMode=true` flag enables the LLM controller loop
- Progress is visible in the UI as a normal session (terminal view works)

### US-2: Promote backlog item to autonomous run
**As a** developer  
**I want to** click "Run autonomously" on a backlog item that's already in Ready status  
**So that** Stapler Squad starts a session and runs it to completion without further input

**Acceptance criteria:**
- The Backlog page shows a "Run autonomously" button for items in `ready` status
- Clicking it calls `SpawnSessionFromItem` with `autonomous=true`
- The created session has `AutonomousMode=true` and `PermissionMode="auto"`
- The backlog item transitions `in_progress → done` (or `→ failed`) based on session outcome

### US-3: LLM orchestrator loop keeps session moving
**As the** system  
**I want to** monitor an autonomous session and inject the next message when the session becomes idle  
**So that** the session can make multi-turn progress without human steering

**Acceptance criteria:**
- When `AutonomousMode=true`, a background goroutine (`AutonomousDriver`) starts with the session
- The driver uses `ClaudeController`'s idle detection to wait for the session to be in idle/waiting state
- When idle, the driver calls the headless LLM pool with: current goal + session output tail → next message
- The next message is injected via `ClaudeController.WriteToPTY` (not raw send_keys)
- The driver detects completion by: session exiting (OneShot), or LLM evaluating "goal complete" 
- Maximum turn limit prevents infinite loops (default: 20 turns)
- Driver logs each turn to the session's log stream

### US-4: GitHub PR support in backlog
**As a** developer  
**I want to** sync open GitHub PRs into the backlog  
**So that** I can dispatch autonomous sessions to address PR review comments or fix failing CI

**Acceptance criteria:**
- New `github_prs` backlog plugin fetches open PRs from `GET /repos/{owner}/{repo}/pulls`
- Each PR becomes a backlog item with: title, body, diff URL, CI status summary, reviewer comments
- Items are tagged `pr:review-requested`, `pr:changes-requested`, or `pr:ci-failing` based on PR state
- PR items work with all existing backlog flows (triage, spawn session, autonomous run)

### US-5: LLM-assisted approval for risky tool calls
**As the** system  
**I want to** consult the LLM controller when the auto-classifier marks a tool call as risky  
**So that** the session can proceed autonomously on tool calls that are justified by the goal  

**Acceptance criteria:**
- When `AutonomousMode=true` AND a tool call is classified as risky, the approval handler sends a query to the headless LLM pool instead of immediately queuing for human review
- The query includes: session goal, the tool call (tool name + arguments), session output tail
- If the LLM approves (with reasoning), the tool call is auto-approved with the reasoning stored in the approval log
- If the LLM denies OR is unavailable, fall back to human review queue
- Human review queue must still work; this is an addition, not a replacement

### US-6: Goal completion and artifact extraction
**As the** system  
**I want to** detect when an autonomous session has completed its goal  
**So that** I can update the backlog item, surface the PR URL, and notify the user

**Acceptance criteria:**
- `AutonomousDriver` detects goal completion when: session exits (OneShot done) OR LLM evaluator returns `done=true`
- On completion, driver extracts PR URL from session output (using existing `parseClaudeSessionID`-style pattern matching)
- Backlog item is transitioned to `done` with PR URL stored as artifact
- If session exits with error OR LLM evaluator returns `done=false, reason=stuck`, item transitions to `failed`
- User sees a notification (via existing push notification system) when autonomous session completes

---

## Non-Goals (explicitly out of scope)

- Multi-session parallelism (autonomous sessions running concurrently on the same issue)
- GitHub webhook-triggered autonomous runs (manual trigger only for now)
- Autonomous PR merging (session can create PR but not merge it)
- Support for GitLab, Bitbucket, or other platforms
- Full `bypassPermissions` mode — all autonomous sessions use `auto` or explicit LLM approval

---

## Technical Constraints

- The `ClaudeController` idle detection must be used for injection timing (not raw sleep loops)
- The headless LLM pool (`headlessPool` in `SessionService`) is the only LLM access path — no direct API calls
- All autonomous decisions must be logged to the session's log stream for auditability
- Approval decisions made by the LLM controller must be stored in the `approval_store` alongside human approvals
- The existing 7-touchpoint session creation registry must be followed for the new `autonomous` session type

---

## Architecture Overview

```
GitHub Issue/PR URL
       │ (omnibar paste or backlog item)
       ▼
CreateSession(autonomous=true, permission_mode="auto")
       │
       ▼
AutonomousDriver goroutine starts
       │
       ├── Watches ClaudeController for idle state
       ├── On idle: calls headlessPool → "what should Claude do next?"
       ├── Injects response via ClaudeController.WriteToPTY
       ├── On risky tool call: calls headlessPool → "approve this?"
       │         ├── LLM approves → auto-approve in approval_store
       │         └── LLM denies → human review queue
       └── On completion: extracts artifacts → updates backlog item → sends notification
```

---

## Prioritization

| Priority | Story | Rationale |
|---|---|---|
| P0 | US-3: LLM orchestrator loop | Core primitive everything else depends on |
| P0 | US-2: Reliable injection timing | US-3 is unusable without this |
| P1 | US-1: Omnibar autonomous mode | Primary user-facing entry point |
| P1 | US-2: Backlog promote | Secondary entry point |
| P2 | US-6: Goal completion | Makes the loop useful end-to-end |
| P2 | US-5: LLM approval | Reduces human interruption |
| P3 | US-4: GitHub PR plugin | Extends scope beyond issues |
