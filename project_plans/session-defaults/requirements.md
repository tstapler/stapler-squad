# Requirements: Session Defaults

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-13

## Problem Statement

Creating new sessions in Stapler Squad requires re-entering the same settings repeatedly (program, tags, env vars). This causes inconsistency across sessions, onboarding friction when returning to a project, and general repetition overhead for the primary user (solo developer).

## Success Criteria

- A new session can be created with zero manual field entry when defaults are configured
- Defaults are visibly applied in the create-session dialog (user can see and override them)
- Named profiles exist and can be selected at session creation time
- Per-directory defaults are detected and pre-populated automatically
- The Settings page has a working Defaults section for managing all of the above

## Scope

### Must Have (MoSCoW)
- Global default program (claude, aider, etc.)
- Global default tags (pre-applied to every new session)
- Global default environment variables / CLI flags
- Named profiles (e.g., "Work", "Personal") selectable at session creation
- Per-directory/workspace defaults that override global defaults
- Settings page UI with a Defaults section (manage global defaults + profiles)
- "Save as default" shortcut inside the create-session dialog

### Should Have
- Per-directory defaults auto-detected from the working directory when creating a session
- Profile selector in the create-session dialog

### Won't Have (this iteration)
- Cloud/cross-machine sync of defaults
- REST/RPC API for programmatic defaults mutation from outside the UI

## Constraints

Tech stack: React + ConnectRPC (frontend); Go (backend); vanilla-extract for all new CSS
Dependencies: No new npm or Go packages; persist in existing config.json schema
Environment: Web UI at localhost:8543; state in `~/.stapler-squad/workspaces/<hash>/`

## Context

### Existing Work
- `config/` package manages JSON config; `~/.stapler-squad/config.json` is the persistence layer
- Session creation flow exists in `web-app/src/` (create-session dialog)
- No Settings page exists yet in the web UI; this feature would introduce it
- CSS architecture rule: all new component styles in `.css.ts` files using vanilla-extract

### Stakeholders
- Solo developer (primary user): removes repetitive session setup
- Future users: consistent onboarding experience

## Research Dimensions Needed

- [ ] Stack — how the current config.json schema is structured; what fields are available for extension
- [ ] Features — survey how comparable tools (tmux sessionizer, zellij layouts, Wezterm workspaces) handle session defaults
- [ ] Architecture — where to add the defaults data model, ConnectRPC endpoint, and React state; how per-directory detection works
- [ ] Pitfalls — migration of existing config.json without breaking changes; default precedence ordering (directory > profile > global)
