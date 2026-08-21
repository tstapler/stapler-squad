# Requirements: session-pr-creation

Source: backlog item `ceab8fa6-69a1-4cf1-ac85-6a10c6de2ba1` ("feat: one-click PR
creation from session diff"), migrated from
[TylerStaplerAtFanatics/stapler-squad#42](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/42).
Non-interactive triage pass — no ideation interview was run.

## Baseline (already shipped — verified in code, not to be re-built)

A "Create PR" action already exists for any session, not just backlog-driven
ones:

- `SessionActionsOverflow.tsx` renders a "Create PR" button
  (`onRunOneShot` prop), wired through `SessionCard` → `SessionList` →
  `web-app/src/app/page.tsx:294` (`handleRunOneShot`) and also
  `web-app/src/app/review-queue/page.tsx:343`.
- Click calls the `RunOneShot` RPC
  (`server/services/session_service.go:3616`) with a **fixed, unconfigurable
  prompt**: `"Create a pull request for the changes in this session."` — this
  spawns a full `claude -p` agent turn in the worktree that decides for
  itself how to commit/push/open the PR.
- On success, `RunOneShot` extracts the PR URL from agent output and persists
  it (`server/services/session_service.go:3690`) — session card already
  surfaces `githubPrUrl` (`proto/session/v1/types.proto:87`).
- A **separate, more direct mechanical path already exists** for
  backlog-driven sessions only: `pushAndCreatePR`
  (`session/backlog_lifecycle.go:3585`) calls
  `headless.DraftPRDescription` (`session/headless/features.go:280`, LLM
  writes only the PR body from the diff — no agentic commit/push behavior)
  and then `GitWorktree.CreatePR` (`session/git/worktree_git.go:329`, a
  literal `gh pr create --title ... --body ...` with existing-PR reuse via
  `findExistingPR`). This is faster, cheaper (one short LLM call for the
  body vs. a full agent turn), and deterministic compared to the
  one-shot-agent path the dashboard button uses today.
- `EnablePRAutoMerge` (opt-in, `AutoCreatePR` flag) and
  `RequestCopilotReview` already exist and are wired for the backlog path.

## Actual gap (what this item is scoped to close)

1. No pre-fill/edit modal. The user cannot review or adjust the PR title or
   body before it's created — today's flow is a single click that hands
   everything to an LLM agent with a static instruction string.
2. The dashboard's "Create PR" button uses the slow/non-deterministic
   agentic `RunOneShot` path instead of the already-built mechanical
   `DraftPRDescription` + `GitWorktree.CreatePR` path that backlog sessions
   use — for a session that already has a finished diff, spawning a full
   agent turn just to run `git push` + `gh pr create` is unnecessary latency
   and cost, and its unconstrained prompt gives no guarantee it won't also
   make unwanted edits before pushing.
3. No base-branch selection in the UI (mechanical path defaults to the
   repo's default branch via `gh pr create`'s own resolution).
4. No rule-driven "auto-create PR on session complete" for regular (non-backlog)
   sessions — `AutoCreatePR` today is a `BacklogItemData` field only
   (`session/repository.go:373`), unreachable for a plain session with no
   backlog item behind it.

## Acceptance Criteria

1. A "Create PR" action on a session card / diff viewer for a session with
   an active worktree and at least one commit ahead of its base branch opens
   a modal pre-filled with: title (session title), body (generated via the
   existing `headless.DraftPRDescription` call against the session's diff —
   not a new prompt template), and base branch (defaulting to the repo's
   main branch).
2. The user can edit title, body, and base branch in the modal before
   confirming.
3. Confirming calls a mechanical PR-creation path
   (`GitWorktree.CreatePR`/`findExistingPR`) directly — it must not route
   through a full agentic `RunOneShot` turn.
4. If a PR already exists for the session's branch, the modal (or the
   button state) reflects that instead of attempting to create a duplicate,
   consistent with `findExistingPR`'s existing reuse behavior.
5. On success, the PR URL is persisted on the session
   (`GitHubPrUrl`/equivalent field already used by `RunOneShot`) and shown
   on the session card, replacing/augmenting the existing "✅ PR Created"
   overflow-menu state.
6. On failure (`gh` not authenticated, push rejected, commit failure, etc.),
   the modal surfaces the specific error instead of a generic failure state.
   "No commits ahead of base" is handled pre-emptively, not as a modal error:
   the trigger button itself is disabled (with an explanatory tooltip) so the
   modal never opens for a session with nothing to ship — see
   `design/ux.md` Surface 1/2 State B. *(Clarified 2026-08-06 — `sdd:4-validate`'s
   cross-artifact check found the original wording contradicted the chosen
   UX/plan design, which gates on the trigger rather than surfacing an
   in-modal error for this specific case.)*
7. The existing one-shot-agent "Create PR" affordance is removed or
   demoted (not left as a second, confusing entry point) once the
   mechanical modal flow ships — avoid two "Create PR" buttons with
   different behavior on the same card.
8. New behavior is covered by a Go test on the mechanical PR-creation RPC
   path and a Playwright e2e spec per this repo's e2e conventions
   (`@feature` annotation, `data-testid`/ARIA locators, no
   `waitForTimeout`).
9. Feature registry entries updated (`docs/registry/features/backend/` and
   `frontend/`) per `.claude/rules/feature-registry.md`.

## Explicitly out of scope

- Rule-driven auto-PR-on-complete for non-backlog sessions (item 4 above) —
  flag as a follow-up suggestion, not a requirement, unless research shows
  it's trivial to reuse the existing `AutoCreatePR` plumbing.
- Auto-merge / Copilot review wiring for the new manual flow — those already
  exist for the backlog path; extending them here is a nice-to-have, not
  required for "one-click PR creation" as described.
- Any change to the backlog automation (`pushAndCreatePR`) path itself,
  except signature-only, behavior-preserving edits forced by sharing
  `CreatePR`/the `prCreator` interface with the new mechanical RPC (e.g.
  passing an added parameter as its zero value from `pushAndCreatePR`'s
  existing call site).

## Non-functional

- Must not increase RunOneShot/LLM turn cost for the common case (this is
  the whole point of switching to the mechanical path).
- Must follow `.claude/rules/session-creation-registry.md`-style discipline
  only if this introduces a new session *creation* mode — it does not; PR
  creation is a post-hoc action on an existing session, so that registry
  does not apply here.
