# Implementation Plan: dynamic-rule-reload

**Feature**: Wire `LoadClaudeSettingsRules()` into the live classifier at startup, add an fsnotify watch (+ debounce + last-known-good preservation) so edits to `~/.claude/settings.json` / `<projectDir>/.claude/settings.json` hot-reload without a restart, add a manual reload RPC/button, and serialize this new reload path against the existing DB-rule reload path with a mutex.
**Date**: 2026-08-06
**Status**: BLOCKED pending Required Plan Revisions below — do not start `/sdd:5-implement` until all four are folded into their target tasks (see `implementation/adversarial-review.md`, `implementation/architecture-review.md`, `implementation/pre-mortem.md`).
**ADRs**: ADR-001 (global-only claude-settings scope for v1), ADR-002 (RulesService-level mutex for reload serialization), ADR-003 (security bounding via seed-deny priority + origin-tagged visibility)

## Required Plan Revisions (post-review, pre-implementation)

Adversarial review, architecture review, and pre-mortem (2026-08-06) found 4 issues that must be folded into the tasks below before implementation starts — not addressed as drive-by fixes mid-implementation:

1. **Race on `ClaudeSettingsWatcher.lastGood` (adversarial-review Blocker 1, pre-mortem P1 #1).** Task 4.1.1b's `Reload()` reads/writes `w.lastGood` from two unsynchronized goroutines (fsnotify debounce callback + concurrent RPC calls via Task 5.1.1d) — a `fatal error: concurrent map read and map write` crash, not just a logic bug, that would kill the whole process (and every live tmux session with it). **Fix**: hold `w.mu` for `Reload()`'s entire body. Add `TestClaudeSettingsWatcher_ConcurrentReloadCalls_NoRace` (already scoped in `validation.md`) as a required gate before Phase 5 work starts.
2. **Project/global path collision on the real deployed config (adversarial-review Blocker 2, pre-mortem P1 #2).** The live systemd unit's `WorkingDirectory=$HOME` (`scripts/install-service.sh:181`) makes `cwd == home`, so the "project" and "global" settings paths resolve to the identical file — rules load doubled, and every benign self-edit gets tagged `origin=mixed`, defeating ADR-003's security-visibility signal on day one. **Fix**: dedupe resolved (post-`EvalSymlinks`) paths in `LoadClaudeSettingsRulesDetailed`/its path-listing helper before Phase 4 wires the watcher (Task 1.1.1b/1.2.1a). Add a `cwd == home` regression test (in `validation.md`) mirroring the real deployed config, not just a hypothetical distinct `<projectDir>`.
3. **No first-activation opt-in (pre-mortem P1 #3).** Task 2.1.1b silently merges a user's pre-existing interactive-CLI `permissions.allow` entries into unattended stapler-squad auto-approval rules via `AddRules` at construction — no confirmation, just a log line. **Fix**: Task 2.1.1b must surface a blocking first-run notification ("N claude-settings rules found — now active as stapler-squad auto-approval rules") through the same `EventNotification`/toast pipeline Phase 4 builds for later reloads, not a log-only signal — this was flagged as a Concern in adversarial review and promoted to a P1 by pre-mortem.
4. **Notification-ID collision (architecture-review Blocker).** Task 4.3.1a's `fmt.Sprintf("claude-settings-reload-%d", time.Now().Unix())` truncates to whole-second granularity; `NotificationContext.tsx` silently drops same-ID notifications, so an auto-reload followed immediately by a manual reload (the exact workflow Story 5.1.1 encourages) loses one toast. **Fix**: generate the ID via `uuid.New().String()` (per `notification_service.go:104`'s precedent, not `capacity_monitor.go:291`'s), and extract the callback's event-construction logic out of the `NewSessionService` closure into a named, unit-tested method.

Also apply, lower severity, opportunistically during implementation: architecture-review's `RuleSource` should be `type RuleSource string` (a defined type), not `= string` (an alias with zero compile-time protection); consistency-check found Epic 1.2's symlink resolution has no requirements.md-level anchor (fold into requirements.md's item 2 rather than leaving unanchored); ux.md §7.1/7.2 (RPC try/catch, double-click guard) should be folded into Task 6.1.1a/c's task text, not left implicit.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ClaudeSettingsWatcher` | New type (`server/services/claude_settings_watcher.go`) that owns the fsnotify watch on Claude settings paths, debounces bursts, tracks last-known-good rules per path, and triggers reload | Constructed once per server process inside `NewSessionService`; callback-driven (mirrors `HistoryFileWatcher`), not DI-injected via `warren.Set` |
| `rebuildMu` | `sync.Mutex` field added to `RulesService` (`server/services/rules_service.go`) guarding the full read-filter-replace sequence in both `rebuildClassifier()` and the new `rebuildClaudeSettingsRules()` | Lives on `RulesService`, not inside `classifier.RuleBasedClassifier` — see ADR-002 |
| `claude-settings` (rule source) | Existing `Rule.Source` string value (`pkg/classifier/classifier.go`) identifying rules derived from Claude's own `permissions.allow` settings, as opposed to `"user"` (DB-backed) or `"seed"` (hardcoded) | Not changed to a typed enum this project — see Pattern Decisions |
| `RuleSource` | New string-typed constants (`SourceSeed`, `SourceUser`, `SourceClaudeSettings`) added to `pkg/classifier/classifier.go`, used only by the new allow-list filter helper | Deliberately NOT a change to `Rule.Source`'s field type — scoped compromise, see Pattern Decisions |
| `filterRulesBySource` | New helper (`server/services/rules_service.go`) — `func filterRulesBySource(rules []classifier.Rule, allowed ...string) []classifier.Rule` — replaces the exclusion-based filtering both rebuild functions previously used inline | Symmetric allow-list, not exclusion-list — prevents a future 4th source being silently dropped |
| `ReloadClaudeSettingsRules` (RPC) | New ConnectRPC method on `SessionService`/`RulesService`, proto in `proto/session/v1/session.proto` next to `DeleteApprovalRule` | Manual trigger; empty request, returns `success`, `rule_count`, `message` |
| `LoadClaudeSettingsRulesDetailed` | New function (`server/services/claude_settings_parser.go`) — per-path variant of the existing `LoadClaudeSettingsRules`, returning `[]ClaudeSettingsPathResult` (path, priority, label, rules, err) instead of a flattened slice | `LoadClaudeSettingsRules` becomes a thin wrapper over this for backward compatibility |
| `ClaudeSettingsPathResult` | New struct (`server/services/claude_settings_parser.go`) carrying one settings-file's parse outcome, including its own error | Lets the watcher merge per-path instead of wholesale replace |
| `debounce window` | 250ms `time.Timer` reset-on-event pattern inside `ClaudeSettingsWatcher.Start`, modeled on `session/unfinished/gogitstore/mmapwatch.go`'s `packWatchLoop` | Coalesces the multiple fsnotify events one editor save produces |
| `last-known-good rules` | Per-settings-path cache (`ClaudeSettingsWatcher.lastGood map[string][]classifier.Rule`) — when a path fails to parse during reload, that path's previous successfully-parsed rules are reused instead of being dropped | Directly implements pitfalls.md §4 — prevents a transient parse error from wiping working rules |
| `parent-dir watch idiom` | fsnotify pattern of calling `watcher.Add(dir)` on a settings file's containing directory rather than the file itself, so atomic-rename saves (which change the inode) don't silently stop future events | Copied from `server/auth/setup.go`'s `SetupManager.WatchFile` and the retired `RulesStore.WatchAndReload` |
| `origin` (reload metadata) | New `"origin"` key in the `EventNotification`'s metadata map — `"global"`, `"project"`, or `"mixed"` — set by `ClaudeSettingsWatcher.Reload` based on which settings-path labels contributed changed rules | Implements requirements.md open question 3 (security visibility), see ADR-003 |
| `EventClaudeSettingsRulesReloaded` (log context) | Not a new `events.EventType` — reload notifications reuse the existing `events.EventNotification` type (see Pattern Decisions for why `EventBacklogItemChanged` was rejected) | `Metadata["type"] = "claude_settings_reload"` distinguishes it from other notifications |
| global-only scope (v1) | `NewSessionService` calls `LoadClaudeSettingsRulesDetailed(cwd)` where `cwd` is the **server process's own working directory**, never a per-session git worktree path | Resolves requirements.md open question 1 — see ADR-001 |

---

## Pattern Decisions

### Step 0.5 — Creative pass on watcher structure

| Option | Key Strength | Key Weakness |
|---|---|---|
| A. Standalone `ClaudeSettingsWatcher` type, callback-driven, constructed inside `NewSessionService`, started from `wireDepsIntoServer` | Matches 3 existing repo precedents (`HistoryLinker`, `SetupManager.WatchFile`, `HistoryFileWatcher`/`WatchDirWatcher`) for exactly this shape of problem; independently unit-testable (debounce, last-known-good merge) without dragging in `RulesService`'s DB/analytics/AI dependencies | One more small type + one more startup-wiring touchpoint to keep in sync |
| B. Fold the watch loop directly into `RulesService` (a `startClaudeSettingsWatcher` method with fsnotify state as fields on `RulesService`) | Fewer new files/types; reload logic and the thing it reloads live in one place | `RulesService` already owns rule CRUD, analytics, AI-suggestion generation — adding fsnotify lifecycle (watcher handles, debounce timers, goroutine shutdown) is a distinct I/O concern bolted onto a service already doing a lot (SRP violation); harder to unit-test the watcher/debounce logic without a full `RulesService` (rulesStore, analyticsStore, classifier) in every test |
| C. Generic reusable `FileGroupWatcher` (N files, parent-dir + debounce, generic callback) shared by future config-watch needs | Would deduplicate 4 near-identical hand-rolled watch+debounce loops already in the repo (`SetupManager.WatchFile`, `HistoryFileWatcher`, `WatchDirWatcher`, `packWatchLoop`) | Only one concrete call site today (this project); the 4 existing implementations differ enough in shape (single- vs multi-file, debounce or not, polling-fallback or not) that forcing them into one abstraction now is speculative — `.claude/rules/interface-pollution-checklist.md` smell #5 (unjustified generic at a single call site) |

**Chosen: Option A.** It is the only one that both matches established convention and keeps `RulesService` focused. Option C is recorded as a legitimate *future* refactor once a second concrete config-watch need materializes, not now.

### Pattern Decisions table

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| `ClaudeSettingsWatcher` | Standalone watcher type, callback-injected (not interface-injected) | Repo convention (`HistoryFileWatcher`) | Fold into `RulesService` (Option B) | SRP — I/O lifecycle vs. rule business logic; independent testability |
| `ClaudeSettingsWatcher` reuse across config types | — | build-vs-buy.md | Generic `FileGroupWatcher` (Option C) | Single call site today; premature generalization (interface-pollution-checklist smell #5) |
| `rebuildMu` | Lock/Guard (single `sync.Mutex` around the whole read-filter-replace) | This plan (informed by `.claude/rules/go-double-checked-locking.md`) | Full Unit-of-Work-style transactional rebuild abstraction | Only 2 call sites (DB reload, claude-settings reload); a raw mutex fully closes the race — Fowler's Unit of Work solves dirty-object-tracking-with-rollback, a problem that doesn't exist here |
| `rebuildMu` placement | On `RulesService` (composition layer) | architecture.md | Lock inside `classifier.RuleBasedClassifier` | `ReplaceRules` is already atomic for the slice-swap; the race is in the *caller's* read-then-filter-then-write composition across two independent callers, which the classifier has no visibility into |
| `ReloadClaudeSettingsRules` RPC | Service Layer / Transaction Script (PoEAA) | Sibling RPCs (`DeleteApprovalRule`) | Domain Model | No cross-aggregate business rules; a straight "re-read file, hot-swap" action — Domain Model would over-engineer a CRUD-shaped operation |
| `Rule.Source` filtering | Symmetric allow-list helper (`filterRulesBySource`) with typed `RuleSource` constants for the *new* call sites only | architecture.md's "generalize filters to be symmetric allow-lists" recommendation; type-driven-design (partial) | Full `RuleSource` sum-type migration of the `Rule.Source` field itself | Repo-wide field-type migration touches `SeedRules()`, `RulesStore.ToRules()`, and every existing `r.Source == "..."` comparison — materially bigger than this project's scope; the allow-list helper closes the actual bug (silent drop of a future 4th source) without that blast radius |
| Reload notification delivery | Reuse existing `events.EventNotification` (`pkg/events/types.go`), forwarded to the frontend today via `server/events/forward.go` → `SessionEvent` → `NotificationContext` | Verified by direct code read (see note below) | `events.EventBacklogItemChanged` (architecture.md's original suggestion) | **Correction from plan-phase verification**: grepped `web-app/src` for `backlog_item_changed` — zero matches. `EventBacklogItemChanged` is consumed only server-side (`backlog_service_events.go`, `backlog_github_forward_sync.go`) and has no frontend delivery path. `EventNotification` **does** — it is converted to a `SessionEvent` in `server/services/event_converter.go:51` and already drives the exact `NotificationContext`/`NotificationToast` pipeline ux.md describes. Using it means zero new delivery plumbing, correcting an assumption in architecture.md before it became a wrong task |
| Watcher → `RulesService` wiring | Setter injection (`RulesService.SetClaudeSettingsWatcher`), both objects built together inside `NewSessionService` | Existing `SetHistoryLinker`/`SetHeadlessPool` idiom in the same file | `warren.Set` DI (architecture.md's original suggestion) | `warren.Set` exists for objects wired in later, separate construction phases (`BuildDependencies`); here both objects are constructed in the same function in the same breath — a same-function setter call is simpler and avoids threading a new dependency through `RuntimeDeps`/`ServerDependencies` for something with only one consumer (`RulesService`) |

---

## Observability Plan

- **Logs**: `log.Info("[ClaudeSettingsWatcher] reloaded claude-settings rules", "rule_count", n, "changed_paths", ..., "origin", origin)` on every successful reload (auto or manual); `log.Warn("[ClaudeSettingsWatcher] path failed to parse, using last-known-good", "path", p, "err", err)` per failed path; `log.Warn("[ClaudeSettingsWatcher] fsnotify unavailable, watcher disabled", "err", err)` on graceful-degradation startup failure.
- **Metrics**: none — reload frequency is edit-driven (human-paced), not a hot path; no new metric warranted per requirements (no perf claims made).
- **Alerts**: no new alerts required — failures already fail safe (previous rules retained) and are logged; this is a developer-facing convenience feature, not an availability-critical path.

## Risk Control

- **Feature flag**: not gated — this activates dead code (`LoadClaudeSettingsRules` already exists, unit-tested at the parser level, zero blast radius until wired) plus a strictly additive fsnotify watch that degrades to a no-op on failure (`fsnotify.NewWatcher()` error → `log.Warn` + return, matching `WatchDirWatcher`'s idiom). Full rollout on merge.
- **Rollback procedure**: standard revert via PR close + revert commit. No schema/data changes to unwind (see Migration Plan, omitted).
- **Staged rollout**: full rollout on merge — this is a single-process, single-binary desktop-style server (`localhost:8543`), not a fleet; there is no partial-rollout mechanism in this codebase for backend Go changes.

## Unresolved Questions

None — all three open questions from requirements.md are resolved below (also see ADR-001, ADR-002, ADR-003):
1. **Global vs. per-worktree scope** — resolved: global-only for v1 (`~/.claude/settings.json` + `~/.claude/settings.local.json` + the *server process's own* `cwd`'s `.claude/settings*.json`, never a per-session worktree path). Per-worktree overlay is explicitly future work (see ADR-001's Future Work section) — it needs `ClassificationContext.RepoRoot`-scoped evaluation, a materially bigger change than this project's scope.
2. **Reload race between two independent triggers** — resolved: `RulesService.rebuildMu` serializes `rebuildClassifier()` and `rebuildClaudeSettingsRules()` — see Story 3.1.
3. **Security bounding for project-level settings** — resolved: seed hard-denies remain highest priority (unchanged, already true), plus reload events/logs are tagged with `origin: "project"` vs `"global"` for visibility — see Story 4.2 and ADR-003.

## Dependency Visualization

```
Phase 1: Parser refactor (no behavior change yet)
  1.1.1 ClaudeSettingsPathResult + LoadClaudeSettingsRulesDetailed ─┐
  1.1.2 Symlink resolution for settings paths                      │
                                                                     ▼
Phase 2: Startup wiring (fixes the dead-code bug, independent of fsnotify)
  2.1.1 Wire LoadClaudeSettingsRulesDetailed into NewSessionService ┐
                                                                     ▼
Phase 3: Race-condition fix (independent of watcher, must land before Phase 4 touches rebuild paths)
  3.1.1 Add rebuildMu + filterRulesBySource + RuleSource constants ─┐
  3.1.2 Guard rebuildClassifier() with rebuildMu                   │
                                                                     ▼
Phase 4: ClaudeSettingsWatcher (depends on 1, 2, 3)
  4.1.1 ClaudeSettingsWatcher type + debounce + last-known-good ───┐
  4.1.2 rebuildClaudeSettingsRules() (mutex-guarded)                │
  4.1.3 Wire into NewSessionService, GetClaudeSettingsWatcher()     │
  4.1.4 Start from wireDepsIntoServer (serverCtx lifecycle)         │
  4.2.1 Origin-tagging (project vs global) for security visibility  │
                                                                     ▼
Phase 5: Manual reload RPC (depends on 4)
  5.1.1 Proto RPC + messages + make proto-gen ─────────────────────┐
  5.1.2 RulesService.ReloadClaudeSettingsRules handler              │
  5.1.3 SessionService delegator                                    │
  5.1.4 MCP tool registration                                       │
                                                                     ▼
Phase 6: Frontend (depends on 5's generated TS bindings)
  6.1.1 useApprovalRules.ts reloadClaudeSettingsRules callback ─────┐
  6.1.2 ApprovalRulesPanel.tsx button + toast wiring                │
                                                                     ▼
Phase 7: Tests + verification (depends on everything above)
  7.1.1 Go unit tests (parser, watcher, rebuildMu race, RPC)
  7.1.2 Frontend jest tests
  7.1.3 make quick-check + web-app jest run
```

---

## Phase 1: Parser Refactor

### Epic 1.1: Per-path reload results with last-known-good support
**Goal**: Make `claude_settings_parser.go` return per-path outcomes so a later reload can preserve rules from files that transiently fail to parse, without changing existing callers' behavior.

#### Story 1.1.1: Detailed per-path load results
**As a** `ClaudeSettingsWatcher` (built in Phase 4), **I want** per-settings-path parse results instead of one flattened slice, **so that** a parse failure in one file doesn't wipe rules successfully loaded from another.
**Acceptance Criteria**:
- Requirement 7 (malformed settings.json does not crash or wipe working rules): *Given* `~/.claude/settings.json` contains valid JSON with `{"permissions":{"allow":["Bash(git *)"]}}` and `<projectDir>/.claude/settings.json` contains truncated/invalid JSON (mid-autosave), *When* `LoadClaudeSettingsRulesDetailed(projectDir)` is called, *Then* it returns two `ClaudeSettingsPathResult` entries: one for the global path with `Rules=[<Bash git * rule>]` and `Err=nil`, and one for the project path with `Rules=nil` and `Err` set to a JSON parse error — no panic, no aborted call.
**Files**: `server/services/claude_settings_parser.go`

##### Task 1.1.1a: Add `ClaudeSettingsPathResult` struct (~3 min)
- Add `type ClaudeSettingsPathResult struct { Path string; Priority int; Label string; Rules []classifier.Rule; Err error }` above `LoadClaudeSettingsRules` in `server/services/claude_settings_parser.go`.
- Files: `server/services/claude_settings_parser.go`

##### Task 1.1.1b: Add `LoadClaudeSettingsRulesDetailed` (~5 min)
- Extract the existing `for _, p := range paths { ... }` loop body from `LoadClaudeSettingsRules` (lines ~70-90) into a new `func LoadClaudeSettingsRulesDetailed(projectDir string) []ClaudeSettingsPathResult` that appends one `ClaudeSettingsPathResult{Path: p.path, Priority: p.priority, Label: p.label, Rules: rules, Err: err}` per path (keep the existing `log.Warn`/`log.Info` calls as-is inside this function).
- Files: `server/services/claude_settings_parser.go`

##### Task 1.1.1c: Rewrite `LoadClaudeSettingsRules` as a thin wrapper (~3 min)
- Replace `LoadClaudeSettingsRules`'s body with: call `LoadClaudeSettingsRulesDetailed(projectDir)`, flatten `.Rules` from each result (ignoring `.Err`) into one `[]classifier.Rule`, return it. Preserves exact current behavior/signature for existing callers.
- Files: `server/services/claude_settings_parser.go`

##### Task 1.1.1d: Unit tests for detailed load (~5 min)
- New file `server/services/claude_settings_parser_test.go` (none existed before this project — confirmed via `grep -rln LoadClaudeSettingsRules --include=*.go`). Tests: `TestLoadClaudeSettingsRulesDetailed_AllPathsValid_ReturnsPerPathRules`, `TestLoadClaudeSettingsRulesDetailed_OneMalformedPath_ReturnsErrForThatPathOnly`, `TestLoadClaudeSettingsRules_FlattensDetailedResults_IgnoringErrors` (regression guard for the wrapper).
- Files: `server/services/claude_settings_parser_test.go`

### Epic 1.2: Symlink safety for project-level paths
**Goal**: Prevent a symlinked project-level `.claude/settings.json` (monorepo case) from silently breaking fsnotify.

#### Story 1.2.1: Resolve symlinks before returning a watchable path
**As a** `ClaudeSettingsWatcher`, **I want** each settings path resolved through `filepath.EvalSymlinks` before I watch its parent directory, **so that** a symlinked project settings file doesn't cause fsnotify to silently stop firing.
**Acceptance Criteria**:
- *Given* `<projectDir>/.claude/settings.json` is a symlink to `<projectDir>/../shared-config/.claude/settings.json` (monorepo pattern), *When* `resolveSettingsPathOrOriginal("<projectDir>/.claude/settings.json")` is called, *Then* it returns the resolved real path so the watcher's `watcher.Add(dir)` targets the real parent directory, not the symlink's directory.
- *Given* a path resolution error (e.g. broken symlink), *When* `resolveSettingsPathOrOriginal` is called, *Then* it logs a `log.Warn` and returns the original path unchanged (graceful degradation, matches `config/defaults.go`'s `evalSymlinksOrOriginal` idiom — not importable directly since it's unexported in package `config`, so a local equivalent is added here).
**Files**: `server/services/claude_settings_parser.go`

##### Task 1.2.1a: Add `resolveSettingsPathOrOriginal` helper (~3 min)
- Add `func resolveSettingsPathOrOriginal(path string) string` to `server/services/claude_settings_parser.go`, mirroring `config/defaults.go:124-133`'s `evalSymlinksOrOriginal` (log.Warn + fallback to original on error) since that helper is unexported in package `config` and not importable from `server/services`.
- Files: `server/services/claude_settings_parser.go`

##### Task 1.2.1b: Use the resolved path in `LoadClaudeSettingsRulesDetailed`'s `Path` field (~3 min)
- In the `paths := []settingsPath{...}` construction, apply `resolveSettingsPathOrOriginal` to each `.path` value before it's used for `ParseClaudeSettings` and stored in the result's `Path` field — this is also what `ClaudeSettingsWatcher` will watch (Phase 4).
- Files: `server/services/claude_settings_parser.go`

##### Task 1.2.1c: Unit test for symlink resolution (~3 min)
- Add `TestResolveSettingsPathOrOriginal_Symlink_ReturnsRealPath` and `TestResolveSettingsPathOrOriginal_NoSymlink_ReturnsUnchanged` to `server/services/claude_settings_parser_test.go` (create a `t.TempDir()` symlink via `os.Symlink`).
- Files: `server/services/claude_settings_parser_test.go`

---

## Phase 2: Startup Wiring (fixes the dead-code bug)

### Epic 2.1: Load claude-settings rules into the classifier at construction time
**Goal**: Root-cause fix — `LoadClaudeSettingsRules()` has zero call sites (confirmed via `git log --all -S"LoadClaudeSettingsRules" -- server/services/session_service.go` = zero commits); wire it into the one place the classifier is actually built.

#### Story 2.1.1: `NewSessionService` merges claude-settings rules at startup
**As a** stapler-squad operator, **I want** my `~/.claude/settings.json` `permissions.allow` entries active as auto-approval rules the moment the server starts, **so that** I don't have to separately configure the same allow-list twice.
**Acceptance Criteria**:
- Requirement 1 (`LoadClaudeSettingsRules()` output merged into the live classifier at startup): *Given* `~/.claude/settings.json` contains `{"permissions":{"allow":["Bash(git log*)"]}}` and no prior wiring existed, *When* the server starts and `NewSessionService` runs, *Then* `classifierObj.Rules()` contains a rule with `Source == "claude-settings"` and a `CommandPattern` matching `git log*`, confirmed via `rs.allRuleSpecs()` (already filters for this source, previously dead code, now populated).
**Files**: `server/services/session_service.go`

##### Task 2.1.1a: Determine the server's own working directory (~2 min)
- In `NewSessionService` (`server/services/session_service.go:252`), immediately before the existing classifier construction block (~line 303-309), add `cwd, cwdErr := os.Getwd(); if cwdErr != nil { log.Warn("failed to determine working directory for claude-settings project scope", "err", cwdErr); cwd = "" }`. Empty `cwd` makes `LoadClaudeSettingsRulesDetailed` skip project-level paths gracefully (existing behavior when `projectDir == ""`).
- Files: `server/services/session_service.go`

##### Task 2.1.1b: Load and merge claude-settings rules (~4 min)
- Right after the existing `if userRules := rulesStore.ToRules(); len(userRules) > 0 { classifierObj.AddRules(userRules) }` block (`server/services/session_service.go` ~line 306-309), add: call `LoadClaudeSettingsRulesDetailed(cwd)`, flatten `.Rules` from each result into one slice, and `classifierObj.AddRules(flattened)` if non-empty (additive is correct here — this is the classifier's first-ever population of claude-settings rules, not a replace).
- Files: `server/services/session_service.go`

##### Task 2.1.1c: Unit test for startup wiring (~5 min)
- Add `TestNewSessionService_LoadsClaudeSettingsRulesAtStartup` to an existing session_service test file (`server/services/session_service_test.go` if present, else a focused new `_test.go`) — write a temp `~/.claude`-style settings dir (override via whatever env/config-dir seam `NewSessionService` already uses for testability, e.g. `HOME` env var override pattern used elsewhere in this package), call `NewSessionService`, assert `svc.GetClassifier().Rules()` contains a `Source == "claude-settings"` entry.
- Files: `server/services/session_service_test.go` (or new `server/services/session_service_claude_settings_test.go` if the existing file is large/unrelated)

---

## Phase 3: Race-Condition Fix

### Epic 3.1: Serialize the two independent reload triggers
**Goal**: Close the lost-update race pitfalls.md identifies (fsnotify reload racing `UpsertApprovalRule`'s DB reload) before a second reload trigger (the watcher) exists to make the race real.

#### Story 3.1.1: `RulesService.rebuildMu` guards both rebuild paths
**As a** developer relying on approval rules, **I want** a DB rule upsert and a claude-settings reload that happen at the same instant to never clobber each other, **so that** neither a just-added deny rule nor a claude-settings tightening is silently lost.
**Acceptance Criteria**:
- *Given* goroutine A is mid-`rebuildClassifier()` (has read `rs.classifier.Rules()` but not yet called `ReplaceRules`) after a user upserts a new `auto_deny` rule for `rm -rf`, *When* goroutine B concurrently calls the new `rebuildClaudeSettingsRules()` after a `~/.claude/settings.json` edit, *Then* `rebuildMu` ensures B's read-filter-replace only begins after A's completes (or vice versa) — the final classifier state reflects BOTH the new deny rule and the new claude-settings rules, never just one.
**Files**: `server/services/rules_service.go`, `pkg/classifier/classifier.go`

##### Task 3.1.1a: Add `RuleSource` constants (~2 min)
- Add to `pkg/classifier/classifier.go` near the `Rule` struct's `Source` field comment: `type RuleSource = string` (alias, not a distinct type, to avoid forcing conversions at every existing `Rule{Source: "user"}` literal) with `const (SourceSeed RuleSource = "seed"; SourceUser RuleSource = "user"; SourceClaudeSettings RuleSource = "claude-settings")`.
- Files: `pkg/classifier/classifier.go`

##### Task 3.1.1b: Add `filterRulesBySource` helper (~3 min)
- Add `func filterRulesBySource(rules []classifier.Rule, allowed ...string) []classifier.Rule` to `server/services/rules_service.go` (package-level, near `rebuildClassifier`) — returns only rules whose `Source` is in `allowed`.
- Files: `server/services/rules_service.go`

##### Task 3.1.1c: Add `rebuildMu` field and guard `rebuildClassifier()` (~4 min)
- Add `rebuildMu sync.Mutex` field to the `RulesService` struct (`server/services/rules_service.go` ~line 30-37). Wrap `rebuildClassifier()`'s entire body in `rs.rebuildMu.Lock(); defer rs.rebuildMu.Unlock()`. Replace its inline exclusion-loop (`for _, r := range existing { if r.Source != "user" { ... } }`) with `nonUser := filterRulesBySource(existing, classifier.SourceSeed, classifier.SourceClaudeSettings)`.
- Files: `server/services/rules_service.go`

##### Task 3.1.1d: Unit test proving the mutex closes the race (~5 min)
- Add `TestRebuildClassifier_ConcurrentWithClaudeSettingsRebuild_NeitherUpdateIsLost` to `server/services/rules_service_test.go`: spin up N goroutines alternating calls to `rebuildClassifier()` and a stubbed reload path, assert via `-race` (`go test -race`) that no data race is reported and both a user-sourced and claude-settings-sourced rule are present in the final `classifier.Rules()`.
- Files: `server/services/rules_service_test.go`

---

## Phase 4: ClaudeSettingsWatcher

### Epic 4.1: fsnotify watch with debounce and last-known-good preservation

#### Story 4.1.1: Watcher detects external edits and reloads atomically
**As a** stapler-squad operator, **I want** an edit to `~/.claude/settings.json` made in my editor while the server is running to take effect without a restart, **so that** I don't have to remember to restart stapler-squad after tuning Claude's permissions.
**Acceptance Criteria**:
- Requirement 2 (edits reflected within a bounded time, no restart): *Given* the server is running with `~/.claude/settings.json` containing `{"permissions":{"allow":["Bash(git log*)"]}}`, *When* the file is edited (via editor atomic-rename-save) to add `"Bash(npm test*)"` to the allow list, *Then* within ~250-500ms (debounce window + one reload cycle) `rs.allRuleSpecs()` includes a new claude-settings rule matching `npm test*`, with no process restart.
- Requirement 3 (atomicity — in-flight classification never sees a mixed rule set): *Given* a `Classify()` call is in progress holding `RuleBasedClassifier`'s `RLock` when a reload's `ReplaceRules()` call is waiting on `Lock()`, *When* the in-flight `Classify()` completes, *Then* `ReplaceRules()` proceeds and the classification result returned to the caller reflects exactly one consistent snapshot (old or new), never a partial slice — this is `ReplaceRules`'s existing guarantee (`pkg/classifier/classifier.go:392-400`), unchanged by this project; the new mutex only serializes the two *callers*, not the classifier's own locking.
**Files**: `server/services/claude_settings_watcher.go` (new)

##### Task 4.1.1a: `ClaudeSettingsWatcher` struct + constructor (~4 min)
- New file `server/services/claude_settings_watcher.go`. Define `type ClaudeSettingsWatcher struct { projectDir string; onReload func(rules []classifier.Rule, origin string); mu sync.Mutex; lastGood map[string][]classifier.Rule; watcher *fsnotify.Watcher; stopped chan struct{} }` and `func NewClaudeSettingsWatcher(projectDir string, onReload func(rules []classifier.Rule, origin string)) *ClaudeSettingsWatcher`.
- Files: `server/services/claude_settings_watcher.go`

##### Task 4.1.1b: `Reload` method with last-known-good merge (~5 min)
- Add `func (w *ClaudeSettingsWatcher) Reload(ctx context.Context) (ruleCount int, failedPaths []string)`. Calls `LoadClaudeSettingsRulesDetailed(w.projectDir)`; for each result: if `Err == nil`, update `w.lastGood[result.Path] = result.Rules` and use `result.Rules`; if `Err != nil`, append `result.Path` to `failedPaths` and reuse `w.lastGood[result.Path]` (may be empty if never successful) instead of dropping — implements pitfalls.md §4. Merge all paths' rules, compute `origin` (see Task 4.2.1a), call `w.onReload(merged, origin)`, return `len(merged), failedPaths`.
- Files: `server/services/claude_settings_watcher.go`

##### Task 4.1.1c: `Start` with parent-dir watch + 250ms debounce (~5 min)
- Add `func (w *ClaudeSettingsWatcher) Start(ctx context.Context)`. Resolve the 2-4 settings paths (reuse the same path-listing logic as `LoadClaudeSettingsRulesDetailed` — factor a small shared `settingsPaths(projectDir string) []settingsPath` if not already extractable). Create `fsnotify.NewWatcher()`; on error, `log.Warn` and return (graceful degradation, matches `WatchDirWatcher`'s idiom — never blocks startup). `watcher.Add(filepath.Dir(p))` for each resolved path's parent directory (parent-dir idiom, survives atomic-rename saves). Do an initial `w.Reload(ctx)` synchronously before entering the loop. Then loop: on `watcher.Events` matching one of the watched *filenames* (ignore unrelated files in the same dir) with `Write|Create|Rename`, reset a `time.Timer` (250ms, mirrors `mmapwatch.go`'s `packWatchDebounce` pattern); on timer fire, call `w.Reload(ctx)`. `select` on `ctx.Done()` to exit; `defer watcher.Close()`; close `w.stopped` on exit.
- Files: `server/services/claude_settings_watcher.go`

##### Task 4.1.1d: `Stopped()` accessor for shutdown-sync tests (~2 min)
- Add `func (w *ClaudeSettingsWatcher) Stopped() <-chan struct{} { return w.stopped }`, matching `HistoryFileWatcher.Stopped()`.
- Files: `server/services/claude_settings_watcher.go`

##### Task 4.1.1e: Unit tests — debounce, last-known-good, graceful degradation (~5 min, may split into 2 tasks if needed)
- New file `server/services/claude_settings_watcher_test.go`. Tests: `TestClaudeSettingsWatcher_Reload_MalformedPath_KeepsLastKnownGood` (write valid settings, call `Reload`, corrupt the file, call `Reload` again, assert `ruleCount` unchanged and `failedPaths` non-empty), `TestClaudeSettingsWatcher_Start_DebouncesRapidWrites` (write to the file 5x within 50ms, assert `onReload` called once, not 5 times — use a channel-based spy), `TestClaudeSettingsWatcher_Start_NoPanicWhenWatcherUnavailable` (can't easily force `fsnotify.NewWatcher()` to fail cross-platform — instead assert `Start` returns/doesn't hang when given an unwritable/nonexistent parent dir).
- Files: `server/services/claude_settings_watcher_test.go`

### Epic 4.2: Security visibility — tag reload origin

#### Story 4.2.1: Project-sourced rule changes are distinguishable from global-sourced ones
**As a** stapler-squad operator whose sessions run against arbitrary/unreviewed PR branches, **I want** a reload triggered by a *project-level* `.claude/settings.json` change (which could originate from someone else's branch) to be visibly distinct from my own global `~/.claude/settings.json` edit, **so that** I notice if a branch checkout silently expanded the auto-allow surface.
**Acceptance Criteria**:
- Requirement 3 (security bounding for project-level settings, resolving open question 3): *Given* a `git checkout` of a PR branch inside an existing worktree rewrites `<projectDir>/.claude/settings.json` to add `{"permissions":{"allow":["Bash(*)"]}}`, *When* fsnotify fires and `Reload` runs, *Then* the resulting log line and toast include `origin=project` (not `origin=global`), distinguishing it from an edit to `~/.claude/settings.json` (`origin=global`) — the seed hard-deny rules (priority 1000, e.g. `rm -rf` on protected paths) remain unaffected regardless, since they always sort above priority-150-180 claude-settings rules in `ReplaceRules`'s descending-priority sort (`pkg/classifier/classifier.go:395`) — this ordering is pre-existing and unchanged by this project; see ADR-003 for why it's the accepted backstop rather than a rewritten one.
**Files**: `server/services/claude_settings_watcher.go`

##### Task 4.2.1a: Compute `origin` from changed paths' labels (~4 min)
- In `Reload` (Task 4.1.1b), after merging: compute `origin` as `"global"` if every result whose `Err == nil` and whose `Rules` differ from the previous `lastGood` snapshot has `Label` in `{"global", "global-local"}`; `"project"` if any changed result has `Label` in `{"project", "project-local"}`; `"mixed"` if both. Pass this to `onReload`.
- Files: `server/services/claude_settings_watcher.go`

##### Task 4.2.1b: Unit test for origin tagging (~3 min)
- Add `TestClaudeSettingsWatcher_Reload_TagsOriginByChangedPathLabel` to `server/services/claude_settings_watcher_test.go`: reload with only the global path changed → assert `origin == "global"`; reload with the project path changed → assert `origin == "project"`.
- Files: `server/services/claude_settings_watcher_test.go`

### Epic 4.3: Wire the watcher into service construction and server lifecycle

#### Story 4.3.1: Watcher is constructed, reachable, and started/stopped with the server
**As a** stapler-squad operator, **I want** the watcher goroutine to start when the server starts and stop cleanly when the server shuts down, **so that** repeated e2e-test-server or manual-test-instance restarts on my dev machine don't leak inotify watch descriptors.
**Acceptance Criteria**:
- Requirement 5 (reload events visible — log line at minimum), and goroutine lifecycle: *Given* the server starts via `wireDepsIntoServer`, *When* `deps.ClaudeSettingsWatcher.Start(serverCtx)` runs in its own goroutine, *Then* a `log.Info("[ClaudeSettingsWatcher] watching claude settings paths", ...)` line appears, and *When* `Server.Shutdown()` cancels `serverCtx`, *Then* the watcher's goroutine exits within its `select`'s `ctx.Done()` branch and `fsnotify.Watcher.Close()` is called (verified via `Stopped()` closing in the shutdown test).
**Files**: `server/services/session_service.go`, `server/services/rules_service.go`, `server/dependencies.go`, `server/server.go`

##### Task 4.3.1a: Wire `onReload` callback + construct watcher in `NewSessionService` (~5 min)
- In `NewSessionService` (`server/services/session_service.go`), right after `rulesSvc := NewRulesService(...)` (the confirmed construction line, ~324), construct: `claudeSettingsWatcher := NewClaudeSettingsWatcher(cwd, func(rules []classifier.Rule, origin string) { rulesSvc.rebuildClaudeSettingsRules(rules); log.Info("[ClaudeSettingsWatcher] reloaded claude-settings rules", "rule_count", len(rules), "origin", origin); if eventBus != nil { eventBus.Publish(events.NewNotificationEvent("", "System", fmt.Sprintf("claude-settings-reload-%d", time.Now().Unix()), int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW), "Claude Settings Reloaded", fmt.Sprintf("%d claude-settings rule(s) reloaded (%s)", len(rules), origin), map[string]string{"type": "claude_settings_reload", "origin": origin})) } })`, then `rulesSvc.SetClaudeSettingsWatcher(claudeSettingsWatcher)`. Verified enum names via `grep -rn "sessionv1.NotificationType_\|sessionv1.NotificationPriority_" server/` (excluding worktrees) — actual constants are `NOTIFICATION_TYPE_INFO`/`NOTIFICATION_PRIORITY_LOW`, not the shorter form initially assumed.
- Files: `server/services/session_service.go`

##### Task 4.3.1b: `rebuildClaudeSettingsRules` on `RulesService` (~4 min)
- Add to `server/services/rules_service.go`: `func (rs *RulesService) rebuildClaudeSettingsRules(newClaudeRules []classifier.Rule) { rs.rebuildMu.Lock(); defer rs.rebuildMu.Unlock(); existing := rs.classifier.Rules(); kept := filterRulesBySource(existing, classifier.SourceSeed, classifier.SourceUser); rs.classifier.ReplaceRules(append(kept, newClaudeRules...)) }` plus `SetClaudeSettingsWatcher` and the `claudeSettingsWatcher *ClaudeSettingsWatcher` field.
- Files: `server/services/rules_service.go`

##### Task 4.3.1c: `GetClaudeSettingsWatcher` accessor on `SessionService` (~2 min)
- Add `func (s *SessionService) GetClaudeSettingsWatcher() *ClaudeSettingsWatcher { if s.rulesSvc == nil { return nil }; return s.rulesSvc.claudeSettingsWatcher }`, matching the existing `GetClassifier()`/`GetAnalyticsStore()` pattern (`server/services/session_service.go:663-676`).
- Files: `server/services/session_service.go`

##### Task 4.3.1d: Thread the watcher through `ServerDependencies` (~4 min)
- Add `ClaudeSettingsWatcher *services.ClaudeSettingsWatcher` field to the `ServerDependencies` struct (`server/dependencies.go`, near `HistoryLinker` ~line 50). In `BuildDependencies`/the construction flow near where `sessionService` is available (after `NewSessionService`/`NewSessionServiceFromConfig` returns, alongside the `historyLinker := session.NewHistoryLinkerFromRealInspector()` block ~line 807), add `claudeSettingsWatcher := sessionService.GetClaudeSettingsWatcher()`. Assign it in the final `ServerDependencies{...}` struct literal (~line 1270, alongside `HistoryLinker: historyLinker,`).
- Files: `server/dependencies.go`

##### Task 4.3.1e: Start/stop lifecycle in `wireDepsIntoServer` (~3 min)
- In `server/server.go`'s `wireDepsIntoServer` (~line 154-159, next to the `HistoryLinker`/`UnfinishedScanner` starts), add: `if deps.ClaudeSettingsWatcher != nil { go deps.ClaudeSettingsWatcher.Start(serverCtx); log.Info("ClaudeSettingsWatcher started") }`. No new shutdown hook needed — `serverCtx` cancellation (already wired to `Server.Shutdown()`) is what the watcher's `select` on `ctx.Done()` already handles (Task 4.1.1c).
- Files: `server/server.go`

##### Task 4.3.1f: Integration test — startup-to-shutdown lifecycle (~5 min)
- Add `TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable` to `server/services/session_service_test.go` (or the file from Task 2.1.1c): assert `svc.GetClaudeSettingsWatcher() != nil` after `NewSessionService`. Add a `server`-package test (or extend an existing `wireDepsIntoServer`-adjacent test if one exists) asserting `Start` + `serverCtx` cancellation causes `Stopped()` to close within a bounded `select`/timeout.
- Files: `server/services/session_service_test.go`, `server/server_test.go` (if one exists; else skip this half with a note — verify via `ls server/server_test.go` during implementation)

---

## Phase 5: Manual Reload RPC

### Epic 5.1: `ReloadClaudeSettingsRules` — proto through MCP

#### Story 5.1.1: RPC re-parses and hot-swaps claude-settings rules on demand
**As a** stapler-squad operator who just hand-edited `~/.claude/settings.json`, **I want** to trigger a reload immediately instead of waiting for the debounce window or wondering if fsnotify picked it up, **so that** I get an immediate, deterministic confirmation.
**Acceptance Criteria**:
- Requirement 4 (manual reload trigger exists, works with server already running): *Given* the server is running and `~/.claude/settings.json` was just edited to add `"Bash(npm test*)"`, *When* a client calls `ReloadClaudeSettingsRules({})` via ConnectRPC, *Then* the response has `success=true`, `rule_count` equal to the total merged claude-settings rule count (including the new `npm test*` rule), and `message="Reloaded N claude-settings rule(s)."` — verified by a subsequent `ListApprovalRules({source_filter:"claude-settings"})` call showing the new rule.
- Requirement 7 (malformed settings surfaces distinctly, doesn't silently report success with 0 changes): *Given* `<projectDir>/.claude/settings.json` is mid-autosave-corrupt when `ReloadClaudeSettingsRules({})` is called, *When* the RPC runs, *Then* the response has `success=false`, `rule_count` equal to the last-known-good count (unchanged), and `message` containing the failed path and "previous rules still active".
**Files**: `proto/session/v1/session.proto`, `server/services/rules_service.go`, `server/services/session_service.go`, `server/mcp/tools_rules.go`

##### Task 5.1.1a: Proto RPC declaration (~2 min)
- Add to `proto/session/v1/session.proto` right after `rpc DeleteApprovalRule(DeleteApprovalRuleRequest) returns (DeleteApprovalRuleResponse) {}` (line 152): `// ReloadClaudeSettingsRules re-parses ~/.claude/settings.json (and project-level equivalents) and hot-swaps the resulting claude-settings-sourced rules into the live classifier. Malformed files are skipped; previously loaded rules for that file are retained.` then `rpc ReloadClaudeSettingsRules(ReloadClaudeSettingsRulesRequest) returns (ReloadClaudeSettingsRulesResponse) {}`.
- Files: `proto/session/v1/session.proto`

##### Task 5.1.1b: Proto messages (~2 min)
- Add right after `DeleteApprovalRuleResponse` (line ~1422): `message ReloadClaudeSettingsRulesRequest {}` and `message ReloadClaudeSettingsRulesResponse { bool success = 1; int32 rule_count = 2; string message = 3; }`.
- Files: `proto/session/v1/session.proto`

##### Task 5.1.1c: `make proto-gen` (~2 min)
- Run `make proto-gen` from repo root; commit the regenerated `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`.
- Files: `session/gen/session/v1/*.go`, `web-app/src/gen/session/v1/*_pb.ts`

##### Task 5.1.1d: `RulesService.ReloadClaudeSettingsRules` handler (~5 min)
- Add to `server/services/rules_service.go`, modeled on `DeleteApprovalRule` (lines 141-157): `func (rs *RulesService) ReloadClaudeSettingsRules(ctx context.Context, req *connect.Request[sessionv1.ReloadClaudeSettingsRulesRequest]) (*connect.Response[sessionv1.ReloadClaudeSettingsRulesResponse], error)`. If `rs.claudeSettingsWatcher == nil`, return `connect.NewError(connect.CodeUnimplemented, fmt.Errorf("claude-settings reload not available"))`. Else call `ruleCount, failedPaths := rs.claudeSettingsWatcher.Reload(ctx)`; build `success := len(failedPaths) == 0`; `message` per the two Given-When-Then cases above; `log.Info`/`log.Warn` accordingly.
- Files: `server/services/rules_service.go`

##### Task 5.1.1e: `SessionService` delegator (~2 min)
- Add to `server/services/session_service.go` right after `DeleteApprovalRule`'s delegator (~line 3102): `func (s *SessionService) ReloadClaudeSettingsRules(ctx context.Context, req *connect.Request[sessionv1.ReloadClaudeSettingsRulesRequest]) (*connect.Response[sessionv1.ReloadClaudeSettingsRulesResponse], error) { return s.rulesSvc.ReloadClaudeSettingsRules(ctx, req) }`.
- Files: `server/services/session_service.go`

##### Task 5.1.1f: MCP tool registration (~4 min)
- In `server/mcp/tools_rules.go`'s `registerRulesTools` (~line 21-62), add after the `delete_approval_rule` registration: `s.AddTool(mcpgo.NewTool("reload_claude_settings_rules", mcpgo.WithDescription("Re-parse ~/.claude/settings.json (and project-level equivalents) and hot-swap the resulting rules into the live classifier.")), h.reloadClaudeSettingsRules)`. Add handler `func (h *rulesHandlers) reloadClaudeSettingsRules(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)` modeled on `deleteApprovalRule` (~line 212), calling `h.svc.ReloadClaudeSettingsRules(ctx, connect.NewRequest(&sessionv1.ReloadClaudeSettingsRulesRequest{}))`.
- Files: `server/mcp/tools_rules.go`

##### Task 5.1.1g: Backend RPC + MCP tests (~5 min)
- Add `TestReloadClaudeSettingsRules_ValidSettings_ReturnsSuccessAndRuleCount` and `TestReloadClaudeSettingsRules_MalformedPath_ReturnsFailureWithLastKnownGoodCount` to `server/services/rules_service_test.go`.
- Files: `server/services/rules_service_test.go`

---

## Phase 6: Frontend

### Epic 6.1: Manual reload button + toast (scoped small per ux.md)

#### Story 6.1.1: Reload button in the claude-settings tab, wired to existing toast infra
**As a** stapler-squad operator viewing the "Claude Settings" tab of the approval rules panel, **I want** a "Reload rules" button right there (not buried in the already-6-button header row), **so that** after hand-editing settings.json I can confirm the change landed without leaving the app.
**Acceptance Criteria**:
- Requirement 4 (UI button exists, works with server running): *Given* the `ApprovalRulesPanel` is open with `sourceFilter === "claude-settings"`, *When* the user clicks the "Reload rules" button next to the existing hint text, *Then* `useApprovalRules().reloadClaudeSettingsRules()` is called, `refresh()` re-fetches the rule list, and the table updates to show any newly-added claude-settings rules.
- Requirement 5 (visible signal — toast): *Given* the reload RPC returns `{success: true, ruleCount: 4, message: "Reloaded 4 claude-settings rule(s)."}`, *When* the button's click handler resolves, *Then* `showActionToast("Reloaded 4 claude-settings rule(s).", "success", "claude-settings-reload")` is called (5000ms auto-dismiss, per existing `NotificationContext` convention — no new toast component).
- Requirement 5 / 7 (error path distinct): *Given* the RPC returns `{success: false, message: "Failed to reload Claude settings rules — previous rules still active (1 path failed to parse)."}`, *When* the button's click handler resolves, *Then* `showActionToast(message, "error", "claude-settings-reload")` is called (10000ms auto-dismiss) — raw JSON parse error text is NOT included in the toast body (only in `console.error`/server log), matching `useApprovalRules.ts:72`'s existing convention.
**Files**: `web-app/src/lib/hooks/useApprovalRules.ts`, `web-app/src/components/sessions/ApprovalRulesPanel.tsx`, `web-app/src/components/sessions/ApprovalRulesPanel.css.ts`

##### Task 6.1.1a: `reloadClaudeSettingsRules` callback in `useApprovalRules.ts` (~4 min)
- Add near `deleteRule` (~line 159-168): `const reloadClaudeSettingsRules = useCallback(async () => { if (!clientRef.current) return { success: false, ruleCount: 0, message: "Client not ready" }; const req = create(ReloadClaudeSettingsRulesRequestSchema, {}); const resp = await clientRef.current.reloadClaudeSettingsRules(req); await refresh(); return resp; }, [refresh]);`. Add `ReloadClaudeSettingsRulesRequestSchema` to the existing `session_pb` import block. Add `reloadClaudeSettingsRules` to the returned object and `UseApprovalRulesReturn` interface.
- Files: `web-app/src/lib/hooks/useApprovalRules.ts`

##### Task 6.1.1b: Button in the claude-settings tab hint area (~4 min)
- In `ApprovalRulesPanel.tsx`, add a new conditional block modeled on the existing `{sourceFilter === "config" && (<div className={configFileHint}>...</div>)}` (~line 452-457): `{sourceFilter === "claude-settings" && (<div className={configFileHint}>Loaded from ~/.claude/settings.json{" "}<button className={retryButton} onClick={handleReloadClaudeSettings}>Reload rules</button></div>)}`. Reuses the existing `configFileHint`/`retryButton` CSS classes (vanilla-extract, no new styles needed per css-architecture.md's "existing tokens only" rule for a small addition).
- Files: `web-app/src/components/sessions/ApprovalRulesPanel.tsx`

##### Task 6.1.1c: Wire the click handler to `useNotifications()` (~4 min)
- Import `useNotifications` from `@/lib/contexts/NotificationContext` (first use in this file) and destructure `showActionToast`. Add `const handleReloadClaudeSettings = async () => { const resp = await reloadClaudeSettingsRules(); showActionToast(resp.message || (resp.success ? \`Reloaded ${resp.ruleCount} claude-settings rule(s).\` : "Failed to reload Claude settings rules — previous rules still active."), resp.success ? "success" : "error", "claude-settings-reload"); };` and pass `reloadClaudeSettingsRules` through from `useApprovalRules()`'s destructured return (~line 82).
- Files: `web-app/src/components/sessions/ApprovalRulesPanel.tsx`

##### Task 6.1.1d: Frontend tests (~5 min)
- Add to `web-app/src/components/sessions/ApprovalRulesPanel.test.tsx`: `test("clicking Reload rules in claude-settings tab calls reloadClaudeSettingsRules and shows a success toast")`, `test("shows an error toast when reload reports failure")`. Mock `useApprovalRules`'s `reloadClaudeSettingsRules` and assert `showActionToast` (via mocked `useNotifications`) is called with the right variant.
- Files: `web-app/src/components/sessions/ApprovalRulesPanel.test.tsx`

---

## Phase 7: Verification

### Epic 7.1: Prove it, don't just claim it

#### Story 7.1.1: Full test + lint gate
**As a** reviewer, **I want** every claim in this plan backed by a passing check, **so that** "done" means done per this repo's own engineering discipline (`.claude/CLAUDE.md`'s "no completion claim without proof").
**Acceptance Criteria**:
- *Given* all Phase 1-6 tasks are complete, *When* `make build && make test` is run, *Then* it exits 0, including the new `server/services/claude_settings_parser_test.go`, `server/services/claude_settings_watcher_test.go`, and the extended `server/services/rules_service_test.go`.
- *Given* all Phase 1-6 tasks are complete, *When* `make lint` is run, *Then* it exits 0 (no `gofmt`/`golangci-lint` violations in the new files).
- *Given* Phase 6 is complete, *When* `cd web-app && npx jest --no-coverage --testPathPatterns="ApprovalRulesPanel"` is run, *Then* it exits 0 including the two new tests from Task 6.1.1d.

##### Task 7.1.1a: Backend verification (~3 min)
- Run `make build && make test` from repo root; run `go test ./server/services/... -race -run "ClaudeSettings|RebuildClassifier|ReloadClaudeSettingsRules"` specifically to confirm the race-condition test (Task 3.1.1d) passes under `-race`.
- Files: none (verification only)

##### Task 7.1.1b: Lint (~2 min)
- Run `make lint`; fix any findings in the new files (`gofmt -w .` first if needed).
- Files: none (verification only)

##### Task 7.1.1c: Frontend verification (~2 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="ApprovalRulesPanel|useApprovalRules"`.
- Files: none (verification only)

##### Task 7.1.1d: `make quick-check` full gate (~3 min)
- Run `make quick-check` (build + test + lint) from repo root as the final pre-handoff gate referenced by this repo's CLAUDE.md.
- Files: none (verification only)

---

## Notes on Scope Discipline

- **Out of scope, confirmed not touched by any task above**: `session/approval_policy.go`'s dormant `PolicyEngine`; a generic `POST /api/rules/reload` for all sources; file-watching for DB-backed user rules; `permissions.deny` conversion in `claude_settings_parser.go` (pre-existing gap, `claudeAllowsToRules` only converts `Allow`, not `Deny` — noted in research/features.md, explicitly out of scope for this project, not silently fixed as a drive-by).
- **Collateral debt found but not silently fixed**: none identified beyond the above (which requirements.md already flagged as out of scope) — if implementation surfaces new pre-existing debt, per `.claude/rules/fix-flaky-tests-dont-defer.md`'s spirit (applied generally, not just to flaky tests) it should be fixed in the same session or filed as its own tracked item, not silently routed around.
