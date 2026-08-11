# Requirements: Prompt Library for Reusable Session Templates

Source: backlog item `9b214f90-1a7b-4e02-9ef4-23e97dc7c360`, migrated from
[TylerStaplerAtFanatics/stapler-squad#52](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/52).
Generated non-interactively (no user present to interview) per this item's SDD pipeline mode —
derived directly from the issue title/description/labels below rather than a live ideation session.

## Problem

Users re-type the same session-kickoff prompts (dependency audits, code review passes, test
generation) from memory or keep them in an external notes app. There is no in-tool way to save,
discover, and reuse a prompt when creating a new session.

## Summary

Add a prompt template library: markdown files with YAML frontmatter, stored globally
(`~/.stapler-squad/prompts/`) and per-workspace (`.stapler-squad/prompts/`, committed to the
repo so templates are shareable via the repo itself). Templates are selectable from the session
creation UI, support variable interpolation from session-creation context, and are creatable
both by hand (drop a `.md` file) and via a "Save as template" action from an existing session's
initial prompt.

## In Scope

1. **Storage & format** — markdown + YAML frontmatter (`name`, `description`, `tags`), body is
   the prompt text. Global dir `~/.stapler-squad/prompts/`; workspace-local dir
   `.stapler-squad/prompts/` under the repo root of the working directory a session is created
   against. Workspace templates are additive to (not a replacement for) global ones.
2. **Variable interpolation** — `{{repo}}`, `{{branch}}`, `{{issue_title}}` substituted from
   session-creation context at apply time. Undefined variables (e.g. `{{issue_title}}` with no
   linked issue) render as empty string rather than erroring — see open question in
   `plan.md`/`validation.md` for the alternative (leave-as-literal) considered.
3. **Session creation UI** — a "From template" option in the New Session flow (Omnibar /
   `OmnibarCreationPanel.tsx`), with a searchable, tag-filterable picker. Selecting a template
   populates the initial prompt field (interpolated), which remains user-editable before submit.
4. **Save as template** — a button on an existing session's initial prompt that writes a new
   template file (prompts the user for name/description/tags/scope-global-or-workspace).
5. **CLI** — a new subcommand on the existing `stapler-squad` cobra binary (see `main.go`, e.g.
   `rootCmd.AddCommand(...)`), not a separate `ssq` tool (no such binary exists in this repo
   today — the issue's `ssq new --template ...` examples are aspirational shorthand and should
   map onto whatever the actual session-creation CLI surface is, or be dropped if this repo's
   CLI does not currently support session creation at all — see research.md).
6. **Backend** — template listing/read is local filesystem access (no proto/RPC strictly
   required if the frontend can read via an existing static/file-serving mechanism); but given
   this app's architecture (Go backend + React SPA over ConnectRPC, no client-side filesystem
   access), a small ConnectRPC service (list templates, get template, save template) is the
   correct approach. Follows the "New API Endpoints" convention in `CLAUDE.md`.

## Out of Scope (initial version)

- Template versioning / edit history beyond what git already gives workspace templates.
- Template sharing across users/orgs beyond git-repo commit (no central registry).
- Rich template editor UI (initial version: plain textarea + frontmatter fields, not a WYSIWYG).
- Non-session-creation uses of templates (e.g. mid-session prompt injection) — only initial
  session prompt is in scope per the issue.
- Full user-defined variable system beyond the three named in the issue
  (`{{repo}}`, `{{branch}}`, `{{issue_title}}`) — arbitrary custom variables are a future
  extension, not required here.

## Acceptance Criteria

1. A markdown file with YAML frontmatter dropped into `~/.stapler-squad/prompts/` appears as a
   selectable template in the New Session "From template" picker, showing its `name` and
   `description`.
2. A markdown file dropped into `<workspace>/.stapler-squad/prompts/` appears as a selectable
   template only when creating a session scoped to that workspace, alongside global templates.
3. Selecting a template in the picker populates the session's initial prompt field with the
   template body, with `{{repo}}`, `{{branch}}`, and `{{issue_title}}` replaced by
   session-creation context where available, and left blank where not.
4. The template picker supports free-text search (matches `name`/`description`) and filtering by
   one or more `tags`.
5. Clicking "Save as template" on an existing session's initial prompt opens a form (name,
   description, tags, global-vs-workspace scope) and writes a new well-formed template file to
   the correct directory on submit.
6. A malformed template file (missing frontmatter, invalid YAML) is skipped with a logged
   warning rather than crashing the picker or the listing endpoint.
7. `make registry-generate` reflects the new RPC(s)/UI feature per this repo's feature-registry
   rule, and per-feature JSON files exist under `docs/registry/features/`.
8. New backend logic has Go test coverage (template parsing, variable interpolation, malformed
   file handling); new frontend logic has Jest coverage (picker search/filter, save-as-template
   form); at least one Playwright e2e test exists per `.claude/rules/e2e-test-conventions.md`.
9. Session-creation-mode registry (`.claude/rules/session-creation-registry.md`) is NOT triggered
   by this feature — templates alter the *content* of the initial prompt for existing creation
   modes, they do not add a new `SessionType`. (Documented explicitly so implementers don't
   conflate the two.)

## Non-Functional / Constraints

- Must not block or slow down session creation when no templates exist (0 templates = empty,
  fast-loading picker, not an error state).
- Workspace-local templates must work correctly across this app's git-worktree-based session
  model — "workspace" here means the repo root of the directory the session is being created
  against, not the specific worktree, so templates committed on `main` are visible regardless of
  which branch/worktree the new session targets.
- Must follow `.claude/rules/css-architecture.md` (vanilla-extract for any new component styles).
- Must follow `.claude/rules/feature-registry.md` and `.claude/rules/feature-testing-registry.md`
  for any new RPC/UI/omnibar touchpoints.

## Priority Signal (from issue)

Label: `p2`. Net-new feature, not a regression or blocking issue — competitive/quality-of-life
improvement referenced against two competitor tools (CodexMonitor, clideck).
