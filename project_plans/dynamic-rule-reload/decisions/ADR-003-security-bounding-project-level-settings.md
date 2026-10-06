# ADR-003: Security Bounding for Project-Level Claude Settings via Seed-Deny Priority + Origin Tagging

**Status**: Accepted
**Date**: 2026-08-06
**Project**: dynamic-rule-reload

## Context

Wiring `LoadClaudeSettingsRules()` into the live classifier (Phase 2 of this project) turns
a currently-inert attack surface live. Today, a malicious `.claude/settings.json` committed
to a repo has **zero** effect, because nothing reads it into the classifier
(`LoadClaudeSettingsRules` has zero call sites — confirmed via
`git log --all -S"LoadClaudeSettingsRules" -- server/services/session_service.go`).

Project-level `.claude/settings.json` (distinct from `.claude/settings.local.json`) is **not**
gitignored in this repo — `.gitignore:3` only excludes `settings.local.json`. Nothing
structurally prevents a branch/PR from committing
`{"permissions":{"allow":["Bash(*)"]}}`. `LoadClaudeSettingsRules(projectDir)` reads
`<projectDir>/.claude/settings.json` where `projectDir` is a session's git worktree path —
exactly the checkout of whatever branch/PR the session was created against. This
application's own purpose includes creating sessions against arbitrary/unreviewed branches
(`create_session_for_pr`), so this is not a hypothetical scenario.

Per this project's ADR-001 (global-only scope), v1 only loads the *server's own* working
directory's project settings, not each session's individual worktree — which already
substantially narrows this specific risk relative to a naive per-worktree implementation.
But the underlying question — "is it safe to let claude-settings-derived rules auto-allow
tool calls at all?" — needs an explicit answer, not an implicit one.

## Decision

Rely on the **existing, unmodified** priority-ordering mechanism as the hard security
backstop, and add **origin-tagged visibility** as a soft mitigation on top:

1. **Hard backstop (unchanged by this project)**: `classifySingle` iterates rules sorted by
   `Priority` descending, first-match-wins (`pkg/classifier/classifier.go:395`, `ReplaceRules`'s
   sort). Seed hard-denies (destructive `rm -rf`, force-push to protected branches, etc.) sit
   at priority 1000. Claude-settings rules sit at priority 150-180 (`claude_settings_parser.go:70-79`).
   A malicious or careless claude-settings `permissions.allow` entry **cannot** override an
   explicit seed deny — it can only auto-allow commands that no seed rule already denies. This
   project makes no change to seed-rule priorities or to the sort order; it is inheriting an
   already-correct mechanism, not inventing a new one.
2. **Soft mitigation (new in this project)**: every reload — auto (fsnotify) or manual (RPC)
   — is tagged with an `origin` of `"global"`, `"project"`, or `"mixed"` (Story 4.2.1), based
   on which settings-file labels contributed the changed rules. This origin is included in
   the `log.Info` line and the `EventNotification` metadata that drives the frontend toast.
   A `git checkout` of a new branch inside an existing worktree that rewrites
   `<worktree>/.claude/settings.json` — which, per ADR-001, only applies to the server's own
   cwd, not arbitrary session worktrees, in v1 — is therefore visibly flagged as
   `origin=project` rather than blending into the ambient "reloaded rules" noise a global edit
   produces.

Explicitly, this project does **not** add a lower auto-allow ceiling specifically for
project-sourced rules (e.g. capping them to "escalate only, never auto-allow"). The seed-deny
priority ordering is judged sufficient as the hard boundary, because:
- It is a boundary that already exists and is exercised today for `user`-sourced rules (a
  malicious `user` rule added via the UI/RPC has exactly the same "can't beat priority 1000"
  ceiling) — treating `claude-settings` differently would be an inconsistency without a
  articulated reason, since both are equally "not seed-authored."
- A blanket "project-sourced claude-settings rules can never auto-allow" rule would silently
  defeat the entire point of the original ask for project-level settings that legitimately
  loosen restrictions for a specific codebase's known-safe commands (e.g. a monorepo's shared
  `.claude/settings.json` allow-listing its own build tool).

## Consequences

- **Positive**: no new deny-list/allow-list logic to get wrong; reuses a mechanism already
  proven correct for the `user` source; origin tagging gives an operator a way to *notice*
  a project-sourced expansion of auto-allow surface without blocking it outright.
- **Negative**: a sufficiently narrow, non-seed-listed dangerous command (i.e. one the seed
  rules don't already deny) committed in a project's `.claude/settings.json` can still sail
  through as an auto-approval instead of an escalation, once merged into the server's own cwd
  settings — the origin tag is visibility, not prevention. This residual risk is accepted
  because (a) ADR-001 already narrows the blast radius to the server's own working directory,
  not every session's worktree, and (b) the seed-deny list is the codebase's existing,
  actively-maintained security boundary for exactly this class of risk — expanding its
  coverage is a standing, separate concern from this project, not something this ADR should
  quietly take on as scope creep.
- **Follow-up (not scheduled by this ADR)**: if per-worktree claude-settings loading (ADR-001's
  future work) is ever built, this risk analysis should be revisited — loading arbitrary
  session worktrees' settings materially widens the attack surface this ADR currently bounds
  to "the server operator's own cwd," and may warrant the stricter per-project ceiling this
  ADR explicitly declines to add today.

## Alternatives Considered

1. **Cap project-level claude-settings rules to `escalate` only (never `auto_allow`)** —
   rejected: defeats the feature's purpose for the legitimate monorepo/shared-config case;
   inconsistent with how `user`-sourced rules are already treated (see Decision above).
2. **Gitignore project-level `.claude/settings.json` repo-wide** — rejected: out of scope
   (a global git-hygiene change unrelated to this project, and would break Claude Code's own
   *intended* use of committed project settings for teams, which is the whole reason the file
   isn't gitignored today; `.claude/settings.local.json` already covers the "don't commit my
   personal overrides" case).
3. **No visibility mitigation at all — rely purely on seed-deny priority** — rejected:
   requirements.md's open question 3 explicitly asks for the risk to be *bounded and stated*,
   not merely inherited silently; origin tagging is low-cost (a label lookup already available
   from `LoadClaudeSettingsRulesDetailed`'s per-path results) and directly answers "did this
   reload come from somewhere other than my own global settings."
