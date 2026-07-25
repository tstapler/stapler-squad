# Requirements: unified-vcs-widget

**Date**: 2026-07-18
**Type**: feature addition (cross-cutting UI unification + backend extension)
**Complexity**: 4 — cross-cutting change with Large appetite

## Problem Statement
stapler-squad has three independent, inconsistent VCS UI surfaces that each evolved separately and each have gaps the others don't:

- **`VcsPanel`** (session detail, VCS tab) — the richest live view: GitHub PR/CI badges (owner/repo, PR link, review approval/changes-requested counts, CI check conclusion), HEAD commit + message, categorized clickable file lists (conflicts/staged/unstaged/untracked) with per-file diff stats and click-to-navigate into the Files tab. Only works while a live session + worktree exist.
- **Backlog item detail "Version Control" section** (`BacklogItemDetail.tsx`, screenshot reference) — worktree path with copy/browse, shared `VcsStatusDisplay` (branch, clean/dirty, file-count badges, ahead/behind) while a live worktree exists, falling back to `ShipStatusDisplay` (shipped/not-shipped, PR link, branch, commit list, "View Diff" → `ReviewChangesModal`) once the worktree is cleaned up. No GitHub CI/PR review badges, no per-file diff stats, no inline diff.
- **`UnfinishedItemDetail`** (orphaned-worktree dashboard card) — a compact summary: +/- line stats, changed-file count, and a list of commit messages ahead of the default branch.

A user looking at a backlog item today gets a materially poorer VCS picture than a user looking at the same work from the session detail page, and neither view shows the richness the other has. There's no single "best" VCS widget — each surface is missing features the others have, and the underlying data (e.g. GitHub CI/PR review state, per-file diff stats) isn't durably persisted past worktree cleanup, so even a shared component can't show full richness for done items without backend changes.

## Baseline
Today, users switch between the Backlog item detail, a session's VCS tab, and the Unfinished dashboard to piece together full VCS context for a single piece of work, because no single surface shows branch/PR/CI/diff/commit-history state together. Once a work session's worktree is cleaned up (the normal end state for a "done" backlog item), CI/PR review data and per-file diff stats become permanently unavailable — `ShipStatusDisplay` only has shipped-flag, PR URL, branch name, and a commit list, none of which come from the same rich Session proto fields VcsPanel reads.

## Users / Consumers
Developers using the stapler-squad web UI (desktop and mobile, per screenshot) to monitor and review AI agent session work — specifically anyone inspecting a backlog item's, session's, or unfinished-worktree's git/GitHub state to decide whether work is ready, needs review, or needs cleanup.

## Success Metrics
- A single shared VCS widget component is used by Backlog item detail, Session detail (replacing `VcsPanel`'s bespoke rendering), and — where its list-row/detail split allows — Unfinished item detail, with no loss of any feature currently present in any of the three surfaces.
- A "done" backlog item whose worktree has been cleaned up still shows GitHub PR/CI badges, review counts, and per-file diff stats (currently only available live) — i.e. the previously-live-only fields are durably persisted and rendered from history.
- No regression: everything `VcsPanel`, `ShipStatusDisplay`/`VcsStatusDisplay`, and `UnfinishedItemDetail` render today must still be reachable somewhere in the unified widget (or its list-row-summary counterpart).

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut breadth — e.g. defer less-used historical fields — rather than slip the timeline.)*

## Constraints
- Must follow `.claude/rules/css-architecture.md` (vanilla-extract, no new CSS Modules, theme tokens only).
- Any new/changed RPC or proto field requires `make proto-gen` and updates to `docs/registry/features/` per `.claude/rules/feature-registry.md`.
- Any new session-creation-adjacent surface changes are out of scope for the 7-touchpoint session-creation registry (this feature does not add a new session creation mode).
- Both mobile and desktop form factors must be supported (per existing project convention — the reference screenshot is a mobile view of the current Backlog widget).

## Non-functional Requirements
- **Performance SLO**: not specified — should not add a distinguishable render delay to backlog/session detail views (reuse existing polling/refresh patterns, no new N+1 RPCs per visible row).
- **Scalability**: not applicable — same per-item data volume as today, just persisted longer and rendered more richly.
- **Security classification**: internal (existing GitHub PR/CI data already surfaced elsewhere in the app).
- **Data residency**: no special requirements.

## Scope
### In Scope
- Design and build one shared VCS widget component (name/location TBD in planning) that combines, at minimum:
  - GitHub PR/CI badges: repo link, PR link + state/draft flag, approval/changes-requested review counts, CI check conclusion (from `VcsPanel`)
  - Branch, clean/dirty, staged/unstaged/untracked/conflict counts, remote ahead/behind (from shared `VcsStatusDisplay`)
  - Categorized, clickable file lists with per-file diff stats (additions/deletions) and status glyphs (from `VcsPanel`)
  - Shipped/not-shipped status, PR URL, commit list with sha/summary/date, "View Diff" (from `ShipStatusDisplay`)
  - Compact ahead-of-main commit message list and aggregate +/- stats (from `UnfinishedItemDetail`)
  - Worktree path with copy-to-clipboard and "Browse files" (from current Backlog widget)
- Wire the shared widget into Backlog item detail and Session detail (VcsPanel's current call site); evaluate during planning whether Unfinished item detail's compact list-row card should adopt the same component in a "compact" display mode or remain a distinct summary card.
- Backend/proto extension so GitHub PR/CI state, review counts, and per-file diff stats are durably persisted against `BacklogItemShipStatus` (or equivalent) and survive worktree cleanup — not just available while a live session exists.
- Responsive/mobile layout for the unified widget, matching the existing mobile+desktop UX convention.
- Feature registry updates (`docs/registry/features/`) and at least one Playwright e2e test per `.claude/rules/feature-registry.md` and `.claude/rules/e2e-test-conventions.md`.
- Two additional scope items emerged during planning (`implementation/plan.md`) and were deliberately kept, called out explicitly rather than silently added — the same treatment `plan.md`'s Domain Glossary gives `MergeabilityState`: (a) a passive "N active sessions" visibility indicator — not full session-switching — surfaced on `VcsWidgetHeader` when a backlog item has multiple concurrently-active work sessions; and (b) an accessibility remediation pass (keyboard navigation, `aria-label`s, theme-token colors replacing hardcoded hex, `aria-live` regions) across the 3 legacy VCS surfaces being unified, justified by this repo's existing WCAG-AA Axe Core CI gate on `web-app/src/`.

### Out of Scope
- Redesigning `FileTree.tsx` or `BacklogFileBrowserModal` file-browsing UX itself — the unified widget links into the existing file browser/diff viewer rather than replacing them.
- Native mobile app changes — this is the responsive web UI only.
- Jujutsu-specific VCS feature parity beyond what's already implemented (VCS type detection stays as-is).
- Changing how sessions are created, session-type registries, or the omnibar — no new session creation mode is introduced.
- Real-time push updates for CI/PR status (existing refresh/polling behavior is preserved, not upgraded to websockets/webhooks).

## Rabbit Holes
- Persisting per-file diff stats and GitHub CI/PR review state durably may require a new background job or webhook-driven update path if the current "compute on demand from a live worktree" model doesn't have an equivalent for done items — this could balloon backend scope; plan phase should explicitly size this.
- Deciding whether `UnfinishedItemDetail`'s compact card becomes a "compact mode" of the same shared component vs. staying separate is a design decision with real complexity either way (component API design for two very different densities) — flag for architecture review in Phase 3.
- `ReviewChangesModal` vs. inline expandable diff: unifying diff-viewing UX (modal on Backlog vs. inline Files-tab navigation on Session detail) touches user flows beyond just the VCS section and should be scoped carefully, not assumed away.

## Alternatives Considered
- Enrich the Backlog widget in isolation, duplicating patterns from `VcsPanel`/`UnfinishedItemDetail` without extracting a shared component — rejected: user explicitly wants a single shared component reused across all three surfaces for consistency.
- Limit richness to live-session state only, no backend changes — rejected: user explicitly wants historical (post-cleanup) parity, since "done" is the terminal, most-viewed state for backlog items.

## Feasibility Risks
- Session-level proto fields (`githubCheckConclusion`, `githubApprovedCount`, `githubChangesReqCount`, `FileChange.additions/deletions`) may not currently flow into any durable store once a session/worktree is torn down — persisting them likely means capturing a snapshot at "ship" time (similar to how `ShipStatusDisplay`'s commit list is already captured), which needs Go backend design work, not just a proto field add.
- Three different current data-fetch patterns (`SessionVcsContext` live polling, `useBacklogItemShipStatus` historical RPC, whatever backs `UnfinishedWorktree`) will need to be reconciled behind one component's props/hook interface without forcing a live RPC call in contexts where only historical data exists.

## Observability Requirements
Standard request logging sufficient — no new oncall alert condition. If a new background persistence job is introduced for durable CI/PR/diff snapshotting (see Rabbit Holes), it should log failures the same way existing backlog reconciliation jobs do (see `.claude/rules` reconciler patterns) so silent data loss is visible in `~/.stapler-squad/logs/stapler-squad.log`.

## Risk Control
No feature flag needed — this is additive UI enrichment plus backend data persistence, not a behavior change to session/worktree lifecycle. Rollback is a standard revert; no staged rollout required. If the backend persistence design proves risky, backend work can ship behind the existing live-data fallback path (the shared component already needs to degrade gracefully when historical richness is absent).

## Open Questions
- Where exactly should durable GitHub PR/CI/diff-stat snapshots be captured — at PR-merge/ship time, on a periodic reconciler tick, or via GitHub webhook? (Phase 2 research)
- Does `UnfinishedItemDetail`'s card become a "compact" display mode of the shared component, or does it stay a distinct, simpler summary? (Phase 3 architecture review)
- Should the unified widget's file-list click-to-navigate behavior open the existing Files tab (session context) or an inline expandable diff (no navigation), given Backlog detail has no "Files tab" equivalent today? (Phase 3 planning)
