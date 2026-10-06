# Pitfalls & Risks: dynamic-rule-reload

## 1. Why the prior fsnotify watcher was retired

`server/services/rules_store.go:197-199` — `WatchAndReload` is a deliberate no-op:

```go
// WatchAndReload is now a no-op as we use shared DB.
func (s *RulesStore) WatchAndReload(ctx context.Context) {
}
```

**Root cause, found via `git show f2cfe350d -- server/services/rules_store.go`**: commit
`f2cfe350d` (`docs: Update TODO.md and bug status based on analysis`) migrated `RulesStore`
from a single JSON file (`filePath string`, atomic tmp-then-rename writes) to SQLite-backed
storage (`storage *session.Storage`). The pre-migration implementation
(`git show f2cfe350d~1:server/services/rules_store.go`) watched the *parent directory* of
`auto_approve_rules.json` (to catch atomic renames) with a 100ms debounce timer, and reloaded
on any write to that exact path. Once rules lived in SQLite and were mutated exclusively via
`Upsert`/`Delete`/`BulkUpsert` (each of which already calls `exportRulesLocked()` then the
caller calls `rebuildClassifier()` synchronously), the file being watched no longer changed
independently of the in-process write path — **the writer and the watcher became the same
goroutine**, so file-watching added latency and complexity for zero benefit. This is not
"file watching is bad," it's "don't watch a file that only your own process writes."

**This is exactly why the current scope must stay narrow to claude-settings**: that file is
edited by a human (or another tool) *outside* the stapler-squad process, so unlike
`auto_approve_rules.json`, the writer and watcher are genuinely different actors — the
condition that made file-watching necessary in the first place still holds only for this
one file. Confirmed by `docs/archive/tasks/completed/permissions-analysis-auto-approvals.md:100`
(pre-migration design doc): *"Settings are read at startup only (no fsnotify watcher for
Claude settings files). If a user changes their Claude settings, they need to restart the
server... The rules store has its own fsnotify watcher for hot-reload of user rules."* — and
line 521: *"Recommendation: Add fsnotify watchers for the 4 Claude settings paths, similar to
`RulesStore.WatchAndReload()`."* This project is executing a two-year-old recommendation that
was correctly deferred until the DB migration made it the *only* remaining stale-read gap.

**Risk of repeating the mistake**: don't reuse `WatchAndReload`'s old debounce-on-parent-dir
pattern uncritically — copy the *idiom* (watch parent dir, debounce), not literally revive
dead code, and make sure the new watcher is scoped only to claude-settings paths, never
re-wired to `auto_approve_rules.json`/DB rules (explicitly out of scope per requirements.md).

## 2. fsnotify pitfalls specific to this file

- **Atomic-rename saves break inode-based watches.** Confirmed in code: the old
  `WatchAndReload` already worked around this by watching the *parent directory*
  (`filepath.Dir(s.filePath)`) rather than the file itself, specifically because "we catch
  atomic renames" (comment at old `rules_store.go:143`). Any editor or tool that writes
  claude-settings.json via temp-file+rename (many JSON-aware editors, and potentially Claude
  Code's own `/permissions` UI) will silently stop firing events if you `watcher.Add(path)`
  the file directly instead of its directory. **Copy this idiom**, don't `Add()` the file path.
- **Debounce required.** A single logical save can emit multiple rapid `Write`/`Create`/`Chmod`
  events. The old code used a 100ms debounce timer coalescing rapid events before reloading.
  `session/unfinished/watcher.go` doesn't need this because it enqueues idempotent rescans
  rather than reparsing content, but claude-settings reload should debounce the same way the
  old `RulesStore` did.
- **Symlinks confirmed NOT an issue for this user's global file, but still a real risk for
  project-level settings.** Checked directly: `~/.claude/settings.json` is a plain regular
  file (`file ~/.claude/settings.json` → `JSON text data`, not a symlink), and the dotfiles
  `.cfgcaddy.yml` only symlinks `.claude/{CLAUDE.md,agents,agents.md,commands,docs,skills,
  STAPLER.md,RTK.md,skills-index.md}` — `settings.json` is deliberately excluded (it's
  host-specific config, not something cfgcaddy manages). So for *this* user's global path,
  symlink-target watching is not needed. However, `LoadClaudeSettingsRules()` also watches
  `<projectDir>/.claude/settings.json` per session — `projectDir` is a git worktree path
  (`~/.stapler-squad/worktrees/...`), and nothing in this codebase prevents a project from
  symlinking its own `.claude/settings.json` (e.g. a monorepo sharing settings across
  packages). Since this can't be ruled out in general, the watcher should resolve
  `filepath.EvalSymlinks()` on each configured path and watch the resolved target's
  directory too, or fsnotify will silently stop firing for that specific worktree with no
  visible error.
- **Multiple server instances watch the same global file.** This repo runs several
  concurrent `stapler-squad` processes against the same machine (live `:8543` systemd
  service, e2e test server, ad-hoc manual-test instances per `CLAUDE.md`'s "Manual/interactive
  testing" section) — all sharing one `~/.claude/settings.json` even though each has its own
  `STAPLER_SQUAD_INSTANCE` state dir. Each process will independently fsnotify-watch and
  reload from the same global file. This is not a correctness bug (each process's reload is
  self-contained) but means N processes wake up on every edit — acceptable given low edit
  frequency, but worth noting so a future "why did the manual-test instance's rules change
  when I edited my global settings" isn't a surprise.

## 3. Concurrency: fsnotify reload racing UpsertApprovalRule/DeleteApprovalRule

**This is a real, currently-latent race that this feature will make far more likely to
trigger.** `RuleBasedClassifier.ReplaceRules()` (`pkg/classifier/classifier.go:392-400`) itself
is safe — it sorts a local copy and swaps `c.rules` under `c.mu.Lock()` in one atomic step, so
no reader ever sees a torn/partial rule list.

The unsafe part is one layer up, in `RulesService.rebuildClassifier()`
(`server/services/rules_service.go:431-443`):

```go
func (rs *RulesService) rebuildClassifier() {
	userRules := rs.rulesStore.ToRules()
	// Keep seed rules and claude-settings rules; replace user rules.
	existing := rs.classifier.Rules()          // ← read
	var nonUser []classifier.Rule
	for _, r := range existing {
		if r.Source != "user" {
			nonUser = append(nonUser, r)
		}
	}
	rs.classifier.ReplaceRules(append(nonUser, userRules...))  // ← write
}
```

This is a **read-modify-write spanning two separate lock acquisitions** (`Rules()` takes and
releases `RLock`; `ReplaceRules()` takes and releases `Lock()` separately) with no compare-
and-swap and no serialization at the `RulesService` level — `RulesService` itself
(`server/services/rules_service.go:29-36`) has no mutex of its own. Today this is called from
three RPC handlers (`UpsertApprovalRule`, `DeleteApprovalRule`, `BulkUpsertRules`), which
ConnectRPC can invoke concurrently from different HTTP requests.

Concrete lost-update scenario once this feature adds a 4th (fsnotify callback) and 5th
(manual-reload RPC) caller:

1. Goroutine A (fsnotify reload) reads `existing` = `[seed..., claude-settings-OLD, user1,
   user2]`, computes `nonUser = [seed..., claude-settings-OLD]`, and starts parsing the
   updated settings.json to build `claude-settings-NEW`.
2. Goroutine B (`UpsertApprovalRule` for an unrelated user rule) reads `existing` from the
   *same* pre-A-write snapshot, computes its own `nonUser`, and calls `ReplaceRules` first.
3. Goroutine A finishes parsing and calls `ReplaceRules([seed..., claude-settings-NEW,
   user1, user2])` — this **overwrites B's freshly-upserted user rule** because A's `nonUser`
   + `userRules` snapshot was captured before B's write.

Either ordering silently drops one side's update — most concerning when it drops a user's
just-added deny rule (a permission narrowing) in favor of a stale wider rule set, or drops a
claude-settings reload so a user who just tightened `~/.claude/settings.json` keeps operating
under the old, looser permissions until the next unrelated rule edit happens to trigger
another rebuild. **Fix must add a mutex around the whole read-modify-write in
`rebuildClassifier` (or equivalent), not just trust `ReplaceRules`'s internal atomicity** —
this is the same shape of bug the double-checked-locking rule in this repo already warns about
(`.claude/rules/go-double-checked-locking.md`), just with `rebuildClassifier` instead of a
cache slot.

## 4. Fail-safe behavior on malformed settings.json

`ParseClaudeSettings` (`server/services/claude_settings_parser.go:33-46`) and
`LoadClaudeSettingsRules` (lines 59-95) **already fail safe per-path**: a JSON parse error on
any one of the 4 candidate paths is caught, logged via `log.Warn("[ClaudeSettings] skipping
settings file", ...)`, and that path is `continue`d — it does not abort the whole load or
propagate an error to the caller. `os.IsNotExist` is handled identically (missing file is not
an error). Good starting point; two gaps to close for the *live reload* path specifically:

- **Naive "replace claude-settings rules with what LoadClaudeSettingsRules returns now" would
  transiently wipe rules on a mid-autosave corrupt read.** If a reload fires while the file is
  mid-write (many editors write in chunks even without atomic rename), `ParseClaudeSettings`
  may see truncated/invalid JSON, log a warning, and return `nil` for that path — meaning
  `LoadClaudeSettingsRules` returns an empty or partial rule set for that reload cycle. If the
  reload handler does `nonUser-minus-claude-settings + freshly-loaded-claude-settings` the way
  `rebuildClassifier` does for user rules today, a transient parse failure would **remove**
  previously-loaded claude-settings AutoAllow rules until the next successful reload — not a
  security problem (removing auto-allows just means more manual escalation, which is fail-safe
  in the security direction) but a functional regression that will look like "rules randomly
  disappeared" to the user. The reload handler should skip the ReplaceRules step entirely
  (keep last-known-good) when `LoadClaudeSettingsRules` returns nothing changed/empty due to a
  parse error, not silently swap in an empty set — debouncing (see #2) also mitigates this by
  giving the editor time to finish its write before the watcher fires.
- **No distinction today between "file has no permissions key" (valid, 0 rules) and "file
  failed to parse" (invalid, 0 rules)** — both currently produce the same silent `continue`.
  For a one-shot startup load this is fine; for a live reload with a user-visible toast (in
  scope per requirements.md item 4), the toast should surface parse failures distinctly ("could
  not reload settings.json: invalid JSON at line N") rather than reporting a
  reload success with 0 rules changed, so a user editing the file gets useful feedback instead
  of silent no-ops.

## 5. Resource/goroutine leak risk — shutdown idiom to copy

The codebase already has one fsnotify-based background watcher with a clean lifecycle to
copy: `session/unfinished/watcher.go`'s `WatchDirWatcher`. Key patterns worth reusing:

- **Falls back gracefully, never fails construction.** `NewWatchDirWatcher` tries
  `fsnotify.NewWatcher()`; on error it logs a warning and sets `w.watcher = nil`, then the
  caller checks `if w.watcher != nil` before starting the event-loop goroutine
  (`watcher.go:29-36`, `:49-51`). The rest of the type still functions (e.g. via the periodic
  re-walk fallback) even with no fsnotify support. Do the same for the claude-settings watcher
  rather than making fsnotify unavailability fatal to server startup — some sandboxed/CI
  environments (e.g. certain container filesystems, `fs.inotify.max_user_watches` exhaustion
  under a heavy multi-worktree fleet) can make `fsnotify.NewWatcher()` fail.
- **Context-scoped shutdown, not a package-level goroutine.** `fsnotifyLoop` and
  `periodicReWalk` both `select` on `ctx.Done()` and return; `defer w.watcher.Close()` inside
  `fsnotifyLoop` ties the OS watcher's lifetime to the same context the caller controls
  (`watcher.go:145-150`). `server/server.go:909` (`Shutdown()`) and the connection-scoped
  `context.WithCancel` at `server.go:68` are the existing hooks to thread a cancel into — the
  new watcher's goroutine must be started with a context derived from server shutdown, not
  `context.Background()`, or it leaks across every restart. This matters concretely here: the
  e2e suite (`tests/e2e/global-setup.ts`) and the "second manual instance" pattern in this
  repo's `CLAUDE.md` both spin up and tear down full `stapler-squad` processes routinely —
  every leaked watcher goroutine + un-`Close()`'d `fsnotify.Watcher` (which holds OS-level
  inotify watch descriptors) accumulates across those restarts within a single long-lived dev
  machine session in a way a one-off manual test wouldn't surface.
- **A periodic fallback re-check is cheap insurance against missed fsnotify events.**
  `WatchDirWatcher` re-walks every 60s (`periodicReWalk`, `watcher.go:190-205`) *in addition
  to* fsnotify — this is itself evidence that even this codebase's own maintainers don't fully
  trust fsnotify alone to never miss an event (e.g. watch descriptor churn, inotify queue
  overflow under `fs.inotify.max_queued_events`). Consider an analogous cheap periodic
  reconciliation (e.g. reload claude-settings every N minutes regardless of fsnotify) as a
  backstop, especially since claude-settings edits are rare and low-cost to re-parse.

## 6. Security: claude-settings as an attacker-influenced input

**Confirmed real, and this feature activates a currently-inert attack surface.**

- `LoadClaudeSettingsRules()` currently has **zero call sites** anywhere in the codebase
  (confirmed via `grep -rn "LoadClaudeSettingsRules"` across all `.go` files — the only match
  is the function definition itself). `classifierObj := classifier.NewRuleBasedClassifier()`
  in `server/services/session_service.go:304` seeds only `SeedRules()` + DB-backed user rules
  (`:306-308`); claude-settings-derived rules are never merged in. This is the "dead code"
  finding from requirements.md item 1 — **and it means today, a malicious `.claude/settings.json`
  committed to a repo has zero effect**, because nothing ever reads it into the live classifier.
  Wiring it in (in-scope item 1) is what turns this from a documented gap into a live trust
  boundary.
- **Project-level `.claude/settings.json` (without `.local`) is not gitignored in this repo**
  (`git ls-files` shows it untracked here only because this repo happens not to have one
  committed; `.gitignore:3` only excludes `settings.local.json`) — so nothing structurally
  prevents a branch/PR from committing a `.claude/settings.json` with
  `{"permissions": {"allow": ["Bash(*)"]}}`. `LoadClaudeSettingsRules(projectDir)` reads
  `<projectDir>/.claude/settings.json` where `projectDir` is a session's git worktree path —
  i.e. exactly the checkout of whatever branch/PR the session was created against. This app's
  own purpose (per `mcp__stapler-squad__create_session_for_pr` and PR-review workflows in
  `CLAUDE.md`) includes creating sessions against arbitrary branches, including ones a
  reviewer hasn't yet audited.
- **Mitigating factor, confirmed by priority ordering**: `RuleBasedClassifier.classifySingle`
  (`pkg/classifier/classifier.go:513-535`) iterates rules sorted by `Priority` descending and
  returns on **first match** (`:518-528`). `claudeAllowsToRules` assigns claude-settings rules
  priority 150 (global) / 160 (global-local) / 170 (project) / 180 (project-local)
  (`claude_settings_parser.go:71-77`), while `SeedRules()`'s hard-deny rules (destructive `rm
  -rf`, force-push to protected branches, etc.) sit at **priority 1000**
  (`pkg/classifier/classifier.go:768-885`). So a malicious project settings file cannot
  override an explicit seed deny — those are checked first regardless. **But it can still
  auto-allow anything not covered by an explicit seed-deny rule** — e.g. a broad `"Bash(*)"`
  or a narrowly-crafted allow for a specific dangerous-but-not-seed-listed command would sail
  straight through the default "no matching rule → Escalate" fallback
  (`classifier.go:530-534`) as an auto-approval instead of a manual review, silently, and (once
  this feature ships) **live, without even needing to recreate the session** — a `git checkout`
  of a new branch inside an existing worktree rewrites `.claude/settings.json` on disk, which
  fsnotify will pick up as a `Write`/`Create` event just like a manual edit would.
- **Recommendation for the plan phase** (not something to fix silently in this research doc):
  consider whether project-level (not global) claude-settings-derived rules should be capped
  to a lower ceiling than "AutoAllow, full stop" — e.g. require the analytics/toast surfacing
  in scope item 4 to explicitly flag *project-sourced* (vs. global-sourced) rule changes with
  higher visibility, since a global settings edit is always the operator's own action but a
  project-level one can originate from someone else's branch.
