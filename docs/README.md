# Documentation Map

This repo's `docs/` tree mixes two kinds of content: **artifact-type folders**
(decision records, bug tracking, generated registry data, point-in-time
project plans) and, as of 2026-08, a **Diataxis-style documentation
hierarchy** for general project documentation. This file explains both so a
contributor knows where to look and where to add something new.

## Diataxis hierarchy (general project documentation)

Formerly `.claude/docs/*.md` — moved here in 2026-08 so the content isn't
tied to Claude Code's own config directory, and organized by the kind of
question it answers ([Diataxis](https://diataxis.fr)):

| Directory | Answers | Example |
|---|---|---|
| `docs/how-to/` | "How do I accomplish X?" — a goal-oriented recipe assuming some existing knowledge | `docs/how-to/profile-lockups.md` |
| `docs/reference/` | "What are the exact options/fields/touchpoints for X?" — information-oriented, describes the machinery completely | `docs/reference/session-creation-registry.md` |
| `docs/explanation/` | "Why does X work this way / why did this bug happen?" — understanding-oriented background and root-cause writeups | `docs/explanation/tmux-keep-server-on-restart.md` |

There is no `docs/tutorials/` yet — nothing in this repo's docs is currently
learning-oriented (step-by-step, assumes no prior knowledge). Add one if that
changes; don't force existing how-to/reference content into a tutorial shape
to fill the quadrant.

**Adding a new doc**: pick the directory by what question it answers, not by
subsystem. A doc can start as a how-to and grow reference material — if it
does, consider splitting the reference portion into its own file once it's
useful standalone (see `docs/how-to/bundle-tmux.md` and
`docs/reference/bundling-tymuxd.md` for a worked example: the how-to covers
the build steps, the reference covers the full rollout/supervision/security
model).

**Not for this hierarchy**: AI-authorship code-review checklists and
guardrails (the kind of thing that used to live in `.claude/rules/*.md`) are
project skills under `.claude/skills/<slug>/SKILL.md` instead — see
CLAUDE.md's "Documentation Placement" section for why.

## Everything else in `docs/` (pre-existing, unchanged by this hierarchy)

These are artifact-type folders, not Diataxis categories — each holds a
specific *kind of record*, not general documentation:

| Directory | What it holds |
|---|---|
| `docs/adr/` | Architecture Decision Records |
| `docs/architecture/decisions/` | Also ADRs — pre-existing overlap with `docs/adr/`, not reconciled as part of the 2026-08 migration; out of scope for that change |
| `docs/architecture/` | Architecture audits and design docs outside `decisions/` |
| `docs/bugs/open/`, `docs/bugs/fixed/` | Point-in-time bug tracking records |
| `docs/registry/` | Generated feature-registry JSON + its own schema/README |
| `docs/tasks/` | Point-in-time task/investigation docs |
| `docs/testing/` | Testing-strategy docs |
| `docs/api/` | API-surface docs |
| `docs/upstream/` | Upstream-fork-sync tracking |
| `docs/reviews/` | Point-in-time review writeups |
| `docs/project_plans/` (and top-level `project_plans/`) | SDD workflow artifacts (requirements/research/plan/validation docs per project) |
| `docs/archive/` | Retired docs kept for history |
| `docs/tui-test-project/` | A specific test project's own docs |
| Flat top-level `docs/*.md` files (e.g. `docs/gap-analysis.md`, `docs/PROFILING.md`) | Historical, point-in-time documents predating this hierarchy — not retroactively migrated |

None of these are touched by the Diataxis hierarchy above, and historical
documents anywhere in `docs/` (or `project_plans/`) that cite an old
`.claude/docs/` or `.claude/rules/` path are left as-is — they're citations
frozen at the time they were written, the same way an old commit's file
reference stays valid as history even after a later commit moves the file.
