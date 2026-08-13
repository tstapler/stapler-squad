# Requirements: History Page Revamp

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-12

## Problem Statement

The history page serves a solo developer who needs to locate past conversations (some still in progress) and resume or fork them into a new worktree, an existing worktree, or a specific directory. Two compounding problems make this flow painful today:

1. **Identity problem** — Session cards lack enough metadata (repo, branch, timestamps, status, message previews) to distinguish one conversation from another without opening it.
2. **Performance problem** — The page is slow to load and navigate, even for moderate session counts. Search exists but is incomplete.

The net result: a workflow that should take seconds consistently takes too long and requires too many guesses.

## Success Criteria

- A developer can open the history page, visually identify the target session, and launch it in a new or existing worktree in under 10 seconds from page open.
- Each session card displays enough metadata at a glance (repo, branch, timestamps, status) to identify the session without opening it.
- The last few messages of any session are accessible via expand — either shown inline or via a quick-expand gesture — so context is readable without launching the session.
- The history list renders its initial view within 200ms even for large datasets (hundreds of sessions).
- Fork/resume workflow is a first-class, clearly surfaced action: one interaction to choose target (new worktree / existing worktree / specific directory) and launch.

## Scope

### Must Have (MoSCoW)

- **Rich session cards**: Show repo name, branch, last-active timestamp, creation timestamp, current status (running/paused/stopped), and file diff count per card.
- **Last N messages preview**: Collapsed by default; expandable inline to read the last 3–5 messages without leaving the history page.
- **Fast load / virtual scrolling**: History list renders initial view within 200ms; large lists (200+ sessions) scroll without jank via virtualization.
- **Fork / resume workflow**: Explicit action on each card to (a) create a new worktree copy, (b) attach to an existing worktree, or (c) open in a specific directory — carrying the full conversation context.
- **Session rename on resume**: When resuming or forking a session, the user must be able to rename it before launching. Current behavior auto-names as "Resumed: <original>" with no edit opportunity.
- **Fix broken resume**: The current "Resume" action from history is broken. Root cause must be diagnosed and fixed as part of this revamp.
- **Search + filter**: Filter by repo, branch, date range; full-text search across session titles, tags, and message content. Builds on the partial search already in place.

### Out of Scope

- Cross-device sync (history is local only)
- Sharing or exporting conversations
- Authentication or multi-user support

### Explicitly Not Changing

- The running-session view and terminal streaming behavior must remain regression-free.
- New session creation flow (the flow for starting a brand-new session, not forking an old one).

## Constraints

- **Tech stack**: Go backend + React frontend, ConnectRPC for API. No new languages or frameworks.
- **Performance budget**: Initial history list render ≤ 200ms; no regression to existing session views.
- **Backward compatibility**: Existing session state, worktree management, and tmux integration must keep working.
- **Incremental delivery**: Changes must ship in stages — no big-bang rewrite. Each increment must be independently releasable.

## Context

### Existing Work

- A history page exists with basic session listing.
- Search is partially implemented but incomplete or inconsistent.
- No pagination or virtual scrolling is in place — all sessions load eagerly.
- Fork/resume is either missing or buried in the UI.
- The performance root cause is likely eager full-load of all sessions on page open (no streaming, pagination, or lazy loading).

### Stakeholders

- Solo developer (Tyler) — primary and only user of this feature.

## Research Dimensions Needed

- [ ] Stack — evaluate virtualization libraries, pagination strategies, and message-preview rendering options compatible with the existing React stack
- [ ] Features — survey comparable tools (Linear, GitHub history, Cursor conversation history) for patterns in session identification and fork/resume UX
- [ ] Architecture — design patterns for lazy-loading session data, message preview expansion, and the fork/resume action model (ConnectRPC API shape, state management)
- [ ] Pitfalls — known failure modes: stale history state, worktree conflicts on fork, ANSI rendering in preview snippets, scroll virtualization edge cases
