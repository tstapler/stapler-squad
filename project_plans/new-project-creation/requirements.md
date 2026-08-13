# New Project Creation — Requirements

## Summary

Add a first-class "New Project" session creation mode and improve Directory mode's handling of non-existent paths. Both features reduce friction when starting work on a brand-new codebase.

## Background

The application already auto-creates directories and runs `git init` when using "New Worktree" with a non-existent path (`session/git/util.go:78-101`), but this is an undocumented side-effect. There is no dedicated UI path for "I want to create a brand-new project." Additionally, Directory mode silently fails or produces confusing errors when given a non-existent path.

## Requirements

### R1 — New Project Creation Mode

A new "New Project" option appears in the session creation panel alongside the existing types (New Worktree, Directory, Use Worktree, One-off).

#### R1.1 Form Fields

- **Parent directory** — A configurable base directory (defaults to a user-configurable value, e.g. `~/Projects/`). The existing `config.json` pattern should be extended to store this preference.
- **Project name** — The folder name for the new repo (required). Free-text, validated against filesystem naming rules.
- **Resolved path preview** — A read-only display showing the computed full path (`{parentDir}/{projectName}`), updated in real time as the user types.
- **Session program** — Which AI tool to launch (from existing Advanced Options; claude / aider / etc.).
- **Session type** — User selects whether to open as Directory or New Worktree after creation. New Worktree implies a branch name field appears below.

#### R1.2 Creation Behavior (backend)

On submit, in order:
1. `mkdir -p {parentDir}/{projectName}` — Create the directory (fail gracefully if it already exists and is already a git repo; error if it exists and is a file).
2. `git init {path}` — Initialize an empty git repository.
3. `git commit --allow-empty -m "Initial commit"` — Create an initial commit (required for worktrees to work).
4. Start a session using the selected session type (Directory or New Worktree).

#### R1.3 Error Handling

- If `{parentDir}` doesn't exist: offer to create it or error with a clear message.
- If `{path}` already exists as a non-empty directory without `.git`: warn the user; offer to `git init` in place or abort.
- If `{path}` already exists and is a git repo: skip init and proceed to session creation.
- If `git init` fails: surface the error in the UI; no session is started.

### R2 — Directory Mode: Confirmation on Non-Existent Path

When the user submits a "Directory" session with a path that doesn't exist:

- Show a confirmation dialog before proceeding: *"'{path}' doesn't exist. Create directory and initialize as a git repo?"*
- **Confirm** → `mkdir -p {path}` + `git init` + `git commit --allow-empty -m "Initial commit"` → proceed with session creation.
- **Cancel** → close dialog; user can correct the path.
- **Do not** silently create directories or fail with an opaque error.

### R3 — Parent Directory Configuration

- Add a `new_project_base_dir` field to the application config (`config.json`).
- Default value: `~/Projects` (created on first use if it doesn't exist).
- Exposed in the app settings UI so the user can change it without editing JSON.
- The New Project form reads this value as the default for the Parent directory field; it can be overridden per-creation.

## Out of Scope

- Copying template files / `.gitignore` templates (future enhancement)
- Remote origin setup (GitHub/GitLab repo creation)
- Monorepo / multi-package scaffolding
- Changes to the One-off or Use Worktree creation modes

## Acceptance Criteria

- AC1: A "New Project" radio option appears in the creation panel.
- AC2: Submitting a valid project name + parent dir creates the directory, runs git init, creates an initial commit, and opens a session.
- AC3: Attempting to create a project when the path already exists as a git repo skips init and opens a session.
- AC4: Attempting to create a project when the path exists as a non-git directory shows a warning.
- AC5: Directory mode shows a confirmation dialog when the given path doesn't exist, and proceeds only on confirmation.
- AC6: The `new_project_base_dir` config field is readable and writable via the settings UI.
- AC7: All new code paths have Go unit tests; the new creation mode has at least one Playwright e2e test.
