# Requirements: Squad UX Polish

**Status**: Draft | **Phase**: 1 — Ideation complete
**Created**: 2026-04-17
**Target users**: Any Claude Code user running stapler-squad

---

## Problem Statement

Stapler-squad works, but the workflow for managing multiple AI agent sessions — especially parallel sessions on the same repo — is still "a little labor intensive" for real usage (per firsthand tutorial experience). The friction points cluster around three moments: **starting sessions** (too many clicks/fields per session), **feeding prompts** (no first-class prompt input at creation time), and **landing work** (no integrated merge/review path when sessions complete).

Additionally, a natural grouping concept has emerged in practice — users think in terms of "projects" (a repo + a set of related tasks), not just flat session lists. The current model doesn't surface this.

---

## Success Criteria

- A user can spin up N parallel sessions for the same repo from a single interaction (no N×form-filling)
- A prompt can be loaded at session creation time — typed, pasted, or pulled from a recent-prompt library
- Completed sessions have a first-class path to create a PR and surface to a review queue
- Sessions can be organized under a named "project" so related work is visually grouped
- The IntelliJ/IDE open integration is available for any session with a worktree
- The overall session creation flow feels faster than the current omnibar → form → submit path

---

## Scope

### Must Have (MoSCoW)

1. **Batch / multi-session creation** — from a task list (paste N tasks → N sessions created), fork/clone of an existing session, or a named template
2. **Prompt input at creation time** — text input, clipboard paste, or file load on the new-session form; no need to navigate to the session and type
3. **Prompt library / recents** — persist and surface recently-used prompts; autocomplete or pick-from-list
4. **Review queue / merge flow** — a dedicated view of completed sessions; one-click "create PR" action; later optionally integrates AI review before merge
5. **Project concept** — named group of related sessions, possibly tied to a repo; sessions belong to a project; project-level actions (e.g., show all sessions for a repo)
6. **IDE open integration** — "open in IDE" button on session card (IntelliJ first, VS Code stretch); opens worktree directory

### Should Have

- Template-based session creation (pre-fill title, prompt, tags, category from a named template)
- Session fork/clone (duplicate a running session with a new branch, keeping same prompt/config)
- Batch-approve merge for multiple completed sessions in the review queue

### Out of Scope

- AI-assisted code review inside the merge flow (may follow, but not in this iteration)
- Remote or cloud-hosted session management
- Multi-user / team session sharing
- Full Gastown-style agent fleet orchestration

---

## Constraints

- **Tech stack**: Go backend (ConnectRPC / protobuf), React + vanilla-extract frontend, tmux + git worktrees for session isolation — all changes must fit this stack
- **CSS**: New components use vanilla-extract `.css.ts`; edits to existing files use only tokens defined in `globals.css` (see `css-architecture.md`)
- **Proto changes**: Any new API endpoints require protobuf schema update + `make generate-proto`
- **No breaking changes** to existing session persistence format without migration
- **Parallel workstream**: MDD plugin (fbg-guidelines-aware) is in progress separately; this work should not depend on it

---

## Context

### Existing Work (Already Done / In Progress)

- **Omnibar improvement**: Updated to make re-selecting an existing repo faster (already shipped on this branch)
- **"Create another session in same workspace" shortcut**: Faster sibling-session creation, rough UX but functional
- **"Open in IntelliJ" button**: Rich Drinkwater's PR incoming; opens session worktree in IJ
- **Tag-based organization**: Full tag system with grouping strategies already implemented
- **Session defaults**: Previous MDD plan (`project_plans/session-defaults/`) explored config-level defaults — relevant to template/prompt-at-creation work

### Observed Friction (from Rich's MDD tutorial run)

- "For my trivial demo app it was a little labor intensive"
- Had to navigate to each session to feed the prompt after creation
- No concept of a "project" to see all sessions for one task/repo together
- Merge path was manual (no UI affordance to create PR from completed session)

### Review Flow Today

- Tyler's current approach: PR checks pass → `/is-it-ready` skimm → merge if clean
- No UI support for this in stapler-squad; happens outside in GitHub / terminal

### Stakeholders

- **Tyler** — primary maintainer, daily driver of the tool
- **Rich** — active contributor and tutorial author; surfaced most of the friction
- **FBG engineering** — target audience for the MDD plugin + tutorial

---

## Research Dimensions Needed

- [ ] **Stack** — evaluate any new libraries needed (e.g., prompt storage, template engine, virtual lists for review queue)
- [ ] **Features** — survey comparable tools: Cursor, claude-flow, Gastown Refinery, tmuxinator; what do they do for batch/prompt/merge UX?
- [ ] **Architecture** — where does "project" fit in the data model? How does the review queue integrate with existing session state machine? Prompt library storage location?
- [ ] **Pitfalls** — session creation race conditions with batch create; prompt delivery timing (session must be attached before prompt is sent); PR creation auth/token scope
