# Requirements: dynamic-rule-reload

## Source

Backlog item `ff6148fa-01ef-437d-818b-9ebf13470575`, migrated from
[TylerStaplerAtFanatics/stapler-squad#44](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/44)
(filed 2026-04-09). Original title: "feat: dynamic rule updates without restart."

## Original ask (verbatim summary)

Allow auto-approval rules to be updated at runtime without restarting the
stapler-squad process. Proposed: fsnotify-based file watching of a rules
config file, atomic in-place reload, a manual "Reload rules" button + `POST
/api/rules/reload`, a toast on auto-reload, and an audit-log entry per
reload. Competitive research (122 tools surveyed) found only one competitor
(Sortie) supports this; the issue frames it as a differentiator.

## Pre-implementation finding: the primary ask is already implemented

Triage against the current codebase (2026-08-06) found that **user-defined
auto-approval rules already update at runtime with no restart**, and have
for some time — this is not greenfield work:

- Rules are persisted in SQLite via the `ent` ORM
  (`session/ent/schema/approvalrule.go`), not a static config file.
- `RulesService.UpsertApprovalRule` / `DeleteApprovalRule`
  (`server/services/rules_service.go:74-157`) write through `RulesStore`
  then call `rebuildClassifier()` (`rules_service.go:432-443`), which
  hot-swaps the new rule set into the live `classifier.RuleBasedClassifier`
  via `ReplaceRules()` (`pkg/classifier/classifier.go:392-400`).
- `ReplaceRules()` already satisfies the requested ordering guarantee: it
  builds the new sorted rule slice off-lock, then swaps the slice under a
  short `Lock()`. In-flight `Classify()` calls hold an `RLock` and finish
  against whichever slice they started with — atomic, no partial state, no
  restart.
- This is already reachable from three surfaces today: the web UI
  (`ApprovalRulesPanel.tsx` / `useApprovalRules.ts`), MCP tools
  (`list_approval_rules`, `upsert_approval_rule`, `delete_approval_rule` in
  `server/mcp/tools_rules.go`), and ConnectRPC directly.
- A prior file-watching implementation for this exact purpose was already
  built and then **intentionally retired**: `RulesStore.WatchAndReload()`
  (`server/services/rules_store.go:197-199`) is a deliberate no-op, with a
  comment stating reload-via-file-watch was replaced once rules moved to
  the shared DB — i.e., this was tried, and DB + RPC hot-swap was judged
  the better mechanism.

**Implication:** the issue's premise (rule changes require a restart) is
stale. Re-scoping to a fsnotify watcher over a config file, as literally
proposed, would rebuild a mechanism the codebase already replaced on
purpose. This project instead scopes to the one real remaining gap below.

## The real remaining gap

A second, distinct rule source exists: **Claude's own permission rules**
from `~/.claude/settings.json` / `<project>/.claude/settings.json`
("claude-settings" source, `server/services/claude_settings_parser.go`,
`LoadClaudeSettingsRules()`). Per an earlier archived investigation
(`docs/archive/tasks/completed/permissions-analysis-auto-approvals.md:326,519`),
this function was known to be load-once-at-startup with "no fsnotify
watcher... known gap," and that same doc (line 521) already recommended
"add fsnotify watchers for the 4 Claude settings paths, similar to
`RulesStore.WatchAndReload()`."

**Correction from research (`research/features.md`):** `LoadClaudeSettingsRules()`
was never wired into `NewSessionService()` in the first place — `git log
--all -S"LoadClaudeSettingsRules" -- server/services/session_service.go`
shows zero commits ever touching that call site. It is a planned capability
that was built (parser + `RulesService` source-filtering plumbing) but never
connected, not a regression. `RulesService`'s existing filters for
`Source == "claude-settings"` (`rules_service.go:421-426,434-442`) are
therefore dead code today, ready to activate once the call site is wired.

**Also from research (`research/pitfalls.md`):** the retired
`RulesStore.WatchAndReload()` (`rules_store.go:197-199`, no-op since the
DB migration in commit `f2cfe350d`) was not evidence that file-watching is
the wrong approach — it went away because for DB-backed rules the writer
and the watcher became the same in-process call path, making a watch
redundant. Claude-settings is edited by a human/tool *outside* the process,
so the condition that justifies file-watching still holds only there. This
project is executing a two-year-old, previously-correctly-deferred
recommendation, not repeating a mistake.

## In scope

1. Root-cause and fix why `LoadClaudeSettingsRules()` is no longer wired
   into classifier construction (regression vs. deliberate removal —
   research phase to confirm via git blame/history before deciding the fix
   shape).
2. Once claude-settings rules load at startup again, add an fsnotify watch
   on the resolved settings.json path(s) (global + project) so edits made
   *outside* the app (hand-editing `~/.claude/settings.json`) are picked up
   without a restart, reusing the existing `ReplaceRules()` atomic-swap
   path — do not reintroduce the retired `WatchAndReload` DB-polling
   design.
3. A manual reload affordance for this source specifically (button in the
   rules/config viewer + one RPC), since file edits have no upsert-driven
   trigger the way DB rules do.
4. A visible signal (toast + one log/audit line) when claude-settings rules
   are reloaded, auto or manual — parity with the original ask's audit
   requirement, scoped to the source that actually needs it.

## Out of scope

- Rebuilding file-watching for DB-backed user rules — already solved via
  RPC/MCP + `ReplaceRules()`.
- A generic `POST /api/rules/reload` for all rule sources — DB-backed rules
  already reload on every upsert; a blanket endpoint would be dead code for
  that source. If validation surfaces a real need for a manual "force
  resync everything" affordance, scope it then.
- Auditing/notification for DB-rule changes — `UpsertApprovalRule` /
  `DeleteApprovalRule` already log (`rules_service.go:133,152`); UI-level
  toasts for those are a separate, already-served UX path.
- The dormant `session/approval_policy.go` `PolicyEngine` — grep confirms
  it is referenced only within its own file and `approval_automation.go`,
  wired into nothing else in the live request path. Likely dead/legacy
  code superseded by `pkg/classifier.RuleBasedClassifier`. Flagged as a
  separate cleanup suggestion, not part of this project.

## Acceptance criteria (draft — refined in validate phase)

1. `LoadClaudeSettingsRules()` output is merged into the live classifier at
   startup (root cause of the current non-wiring identified and fixed).
2. Editing `~/.claude/settings.json` (or a project's `.claude/settings.json`)
   while the server is running is reflected in rule evaluation within a
   bounded time, with no process restart.
3. The reload is atomic: an approval decision in flight during a reload
   completes against a single consistent rule set (old or new, never a mix).
4. A manual reload trigger exists (UI button + RPC) for the claude-settings
   source and works with the server already running.
5. Reload events (auto via fsnotify, or manual) are visible: a log line at
   minimum; a UI toast if a config-viewer surface already exists to host it.
6. No functional or performance regression to the existing DB-backed
   rule hot-swap path (`UpsertApprovalRule`/`DeleteApprovalRule` still work
   exactly as before).
7. Malformed settings.json on reload does not crash the server or wipe
   working rules — the previous valid rule set stays active and the error
   is logged.

## Non-goals

- Changing where DB-backed rules are stored or how they're edited.
- Cross-machine or team rule sync (separate, already-tracked via
  `SaveRulesToConfigFile`/`GetConfigFileRules` YAML export stub).

## Open questions (from research, for plan.md to resolve)

1. **Global vs. per-worktree scope.** There is one shared
   `*classifier.RuleBasedClassifier` per server process, but
   `LoadClaudeSettingsRules(projectDir)` takes a single project directory
   and stapler-squad runs many concurrent sessions across different git
   worktrees. `ClassificationContext` already carries per-request
   `Cwd`/`RepoRoot` (research/architecture.md), which would be the hook for
   a true per-worktree overlay — but that's materially bigger than a
   global-only reload. Default recommendation: global-only for v1 (load
   `~/.claude/settings.json` plus the *server's own* working directory's
   project settings, matching today's parser signature) — explicitly flag
   per-worktree overlay as future work, not silently drop it.
2. **Reload race between two independent triggers.** `rebuildClassifier()`
   (`rules_service.go:432-443`) does an unsynchronized read-filter-replace
   across `classifier.Rules()`/`ReplaceRules()`. Adding a second
   independent reload trigger (fsnotify) alongside the existing
   upsert/delete triggers makes a lost-update race real, not hypothetical.
   Plan must add a `RulesService`-level mutex serializing all reload paths.
3. **Security: project-level `.claude/settings.json` is not gitignored.**
   A PR branch could ship a crafted project-level settings file that
   auto-allows commands (bounded by seed hard-denies still winning at
   higher priority, per `research/pitfalls.md`). Wiring + live-reloading
   this source means such a file takes effect the moment a worktree
   checks out that branch, with no restart as an implicit checkpoint.
   Plan must state explicitly how this risk is bounded (e.g., rely on
   existing seed-deny priority ordering) rather than leaving it implicit.
