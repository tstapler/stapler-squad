# UX Research: go-git-worktree-and-merge

This is backend infrastructure (worktree lifecycle + three-way merge on top of
go-git) with **no new end-user feature UI**. Two indirect surfaces exist,
checked concretely below.

## 1. Does conflict content/`ConflictedFiles` reach a human in the web UI?

No, and the reason is structural, not incidental:

- `MergeMainIntoWorktree`'s `MergeMainResult.ConflictedFiles`
  ([`session/git/ops.go:801-803`](../../../session/git/ops.go)) is populated
  only transiently — the function **always runs `git merge --abort` before
  returning** on conflict (`ops.go:851-858`; doc comment at `ops.go:797-799`:
  "the merge is always aborted before returning, so the worktree is left
  clean either way"). Its only consumers are backlog automation log lines and
  a notification/prompt string built from the *file name list*
  (`session/backlog_lifecycle_pr.go:983-991`,
  `server/services/backlog_service_triage.go:2421-2424`) — never a diff of
  conflict-marker content, and never plumbed to any RPC or web-app type.
  Grepping `web-app/src` for `ConflictedFiles`/`MergeMain` returns nothing.
- The requirements doc's own Rabbit Holes section already names the actual
  consumer of real conflict-marker byte content: "backlog automation's
  fix-agent prompts currently show real conflict markers to an LLM" — an
  agent working the resulting fix session (via terminal/tmux, re-running the
  merge itself), not a web-app component.
- There **is** a generic, already-existing, human-facing conflict UI
  surface — `FileStatus.CONFLICT` → `"U"` glyph
  ([`web-app/src/lib/utils/gitStatus.ts:19`](../../../web-app/src/lib/utils/gitStatus.ts)),
  a `"conflict"` file-change section with an `AlertTriangle` icon and
  "Conflict — resolve before merging" label
  ([`VcsWidgetFileList.tsx:32-39`](../../../web-app/src/components/shared/vcs-widget/VcsWidgetFileList.tsx)),
  and a conflict count in `VcsStatusDisplay.tsx:66-68`. But this is fed by a
  completely separate, unrelated pipeline: live `git status --porcelain`
  parsing in `session/vc/git_provider.go` → `workspace_service.go` →
  `adapters.ts`, which shows unmerged (`U`) files for *any* real in-progress
  conflicted merge/rebase in a worktree, regardless of who started it. It has
  nothing to do with `MergeMainIntoWorktree` today because that function
  never leaves a conflict unresolved.

**Implication for planning, not a scope addition**: as long as the new
three-way merge implementation preserves the existing "always abort on
conflict, never leave the worktree dirty" contract (nothing in the
requirements proposes changing this), no new frontend work is needed and none
of the existing conflict-display UI activates for this feature. If planning
ever considers *not* aborting (e.g., to let a human resolve conflicts by hand
in a worktree instead of only reporting file names), that would land in the
existing generic VCS-status UI for free — worth a one-line confirmation in
the plan doc that abort-on-conflict is being kept, so this isn't rediscovered
mid-implementation.

## 2. Does the StreamHubRolloutPanel pattern imply a settings panel for this project's flags?

Yes — explicitly, not by inference. Scope already says so: "A feature-flag-gated
rollout (global + per-session override, live-settable — matching this repo's
existing stream-hub/tymux rollout pattern from earlier this session)", and
Risk Control spells out the pattern's shape: "`config.FeatureFlags`,
`EffectiveXEnabled`, per-session override map, RPC + settings panel."

Verified the precedent concretely:

- `server/services/stream_hub_rollout_service.go` — a dedicated `*Service`
  (`StreamHubRolloutService`) exposing `GetStreamHubRolloutStatus`,
  `SetStreamHubGlobalOverride`, `SetStreamHubSessionOverride`, and
  `CompleteStreamHubRollbackRehearsal` RPCs, delegated to from
  `SessionService` (same shape as `SlackConfigService`/`CallbackConfigService`
  — see that file's header comment).
- `web-app/src/components/settings/StreamHubRolloutPanel.tsx` (253 lines) — a
  real settings-page component: loads rollout status, renders a global
  override toggle, a rollback-rehearsal completion action, and a per-session
  override list with add/remove, all live (no restart).
- Both `StreamHubRolloutPanel` and its sibling `TymuxRolloutPanel` mount on
  one shared page, `web-app/src/app/settings/features/page.tsx` ("Features"
  settings tab) — confirmed via grep; there is no separate page per flag.

**Implication for planning**: this project's two flags (worktree management,
merge — Open Questions leans toward two independent flags) need the matching
RPC pair(s) and a panel following this exact shape, most likely added onto
the existing `settings/features` page rather than a new page — mirroring
`StreamHubRolloutPanel`/`TymuxRolloutPanel`'s placement. This is UI work, but
it is rollout/ops tooling (a kill switch for developers/operators), not a
new end-user-facing feature; scope it as such in the plan (roughly
StreamHubRolloutPanel-sized effort, times two flags or one panel handling
both).

## Bottom line

- No user-facing feature surface: the worktree/merge subsystem itself is
  invisible to end users: no new page, no new session-facing UI.
- The one UI deliverable this project does need, per its own explicit Scope
  and Risk Control sections, is an admin/rollout settings panel (global +
  per-session override) for the two feature flags, matching
  `StreamHubRolloutPanel`'s existing pattern and mount point.
- Conflict *content* stays a backend/LLM-agent concern (terminal-visible
  conflict markers for the fix-agent), not a new web-app surface — confirmed
  by the current always-abort contract and by grepping web-app for any
  existing wiring, which found none.
