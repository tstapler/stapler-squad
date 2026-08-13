# Requirements: Session Notes

## Source

Migrated from backlog item `f105f307-3743-4f06-81fa-49a0b9f359db`, originally
[TylerStaplerAtFanatics/stapler-squad#172](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/172).

## Problem

Sessions have no place to attach free-form contextual notes ("left this waiting
on X", "this branch is a spike for Y"). Backlog items already have a `notes`
field (`session/ent/schema/backlog_item.go:72`), but that's task-planning
notes tied to a backlog item, not a running/parked session. When juggling many
simultaneous agent sessions, there's no way to leave yourself a reminder
attached to the session itself.

## Goals

- Attach one free-form markdown note per session, editable from the session
  detail view.
- Render the note as formatted markdown when not editing.
- Surface a lightweight visual indicator on `SessionCard` when a session has
  a note, so it's discoverable without opening the session.
- Persist across server restarts (i.e., stored in the existing ent/SQLite
  store, not in-memory).

## Non-Goals (per backlog item's own scope note)

- Multiple/titled notes per session (herdr-web's `PaneNote` model) — start
  with a single note field on the session, matching the "simple start"
  guidance in the source issue.
- Optimistic-concurrency revisioning (herdr-web's `expected_revision` guard)
  — single-user model, last-write-wins overwrite is acceptable.
- Note attachments/files.
- Notes on anything other than a session (e.g., per-pane notes within a
  multi-pane session) — out of scope; stapler-squad sessions are 1:1 with a
  tmux session today.

## Acceptance Criteria

1. A user can attach a markdown note to any session from the session detail
   view.
2. The note renders as formatted markdown in read mode.
3. `SessionCard` shows a visual indicator when its session has a non-empty
   note.
4. Notes persist across server restarts.

## Constraints / Conventions to Follow

- ent schema change → regenerate with
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  (`.claude/rules/ent-schema-generation.md`).
- New/changed RPC → update `docs/registry/features/backend/*.json`; new UI
  feature → `docs/registry/features/frontend/*.json`; run
  `make registry-generate` (`.claude/rules/feature-registry.md`).
- New UI feature needs at least one Playwright e2e test in `tests/e2e/`
  following `.claude/rules/e2e-test-conventions.md` (feature annotation, no
  `waitForTimeout`, `data-testid`/ARIA locators only).
- `react-markdown` is already a dependency (per source issue) — reuse it
  rather than adding a new markdown renderer.
- CSS for any new component must use vanilla-extract (`.claude/rules/css-architecture.md`),
  not new `.module.css` files.
- Consider both desktop and mobile layouts for the new panel (`feedback_mobile_desktop_ux.md`
  instinct: touch targets, responsive layout, mobile keyboard behavior).
