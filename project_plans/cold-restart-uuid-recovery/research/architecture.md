# Architecture Research: Cold-Restart UUID Recovery Before Fresh-Start Decision

## EventStorming table: skipped

This is a bug-fix reordering a single sequential code path inside one actor command
(`startLocked`), not a multi-actor business domain with commands/events/policies to
discover. Skipping the Event-Command-Policy table per requirements.md item 5.

## 1. Three responsibilities currently split across two call sites

Confirmed by reading `startLocked` in full at both mirrored locations
(`session/instance.go:834-996` the actor-safe body, and `session/instance.go:1012-1200+`
the legacy `start()` — see §6 below for why two copies exist) plus
`session/instance_tmux.go:249-279` (`initTmuxSession`) and `:105-119` (`buildLaunchCommand`):

| Responsibility | Owner | When it runs (today) |
|---|---|---|
| Decide launch command (`--resume <uuid>` or not) | `initTmuxSession()` → `buildLaunchCommand(i.claudeSession.ConversationUUID)` | `instance.go:847` (mirrored `:1029`) — **first thing** `startLocked` does, unconditionally, before the `!firstTimeSetup`/`IsAlive()` branch even runs |
| Decide log message ("cold restoring with --resume" vs "cold start... starting fresh") | inline `if i.HasClaudeSession() {...} else {...}` | `instance.go:870-874` (mirrored `:1061-1069`) — inside the `!firstTimeSetup && !IsAlive()` branch, **after** the launch command was already built and logged at `:260` |
| Post-hoc UUID re-detection | `i.tryExtractConversationUUID()` | `instance.go:910` (mirrored `:1116`) — **after** `i.pm().Start(startPath)` has already launched the process with whatever command `initTmuxSession()` built |

This confirms the root cause exactly as stated in requirements.md: `initTmuxSession()`
reads `i.claudeSession.ConversationUUID` at line 847/1029, roughly 20-30 lines *before*
the code that would tell it "wait, check disk for a UUID first" even runs. The
`HasClaudeSession()` check at 870/1061 is purely cosmetic — it only selects which log
line to print; it has already missed its chance to influence `buildLaunchCommand`.

Critically, `initTmuxSession()` has an early-return guard that matters for sequencing:

```go
// session/instance_tmux.go:249-253
func (i *Instance) initTmuxSession() {
	if i.pm().HasSession() {
		log.Info("reusing existing tmux session", "session", i.Title)
		return
	}
	...
```

`i.pm().HasSession()` checks whether a `*tmux.TmuxSession` Go object is already
registered on the process manager (via `TmuxProcessManager.SetSession`, called at the
bottom of `initTmuxSession`). This object is **process-local, in-memory only** — it does
not survive a `stapler-squad` restart. So for the cold-restart scenario in the bug
report (stapler-squad process restarted, or the instance freshly loaded from storage),
`HasSession()` is false and `initTmuxSession()` proceeds to rebuild the command from
`i.claudeSession.ConversationUUID` as loaded from persisted state.

## 2. `DetectByPath` / `tryExtractConversationUUID` — can it run standalone, pre-launch?

Read `session/history_detector.go` `DetectByPath` (lines 131-199) and
`session/instance_claude.go` `tryExtractConversationUUID` (lines 298-363) in full.

`DetectByPath(projectPath string)` has **no dependency on a live process**. It:
1. Resolves home dir via `d.resolveHomeDir()` (real `os.UserHomeDir()`, or the
   `d.homeDir` test override set via `NewHistoryFileDetectorWithHomeDir`).
2. Computes `ClaudeProjectDirName(projectPath)` and reads
   `~/.claude/projects/<encoded>/`.
3. Returns the most-recently-modified valid `*.jsonl` (excluding `agent-*.jsonl`,
   validating UUID format).

`tryExtractConversationUUID()` already wraps this correctly for pre-launch use, and
critically **already has the right guard structure to be called before a tmux session
exists**:

```go
// session/instance_claude.go:308-349 (abridged)
func (i *Instance) tryExtractConversationUUID() {
	if i.claudeSession != nil && i.claudeSession.ConversationUUID != "" {
		return // already linked — untouched
	}
	detector := i.historyDetector
	if detector == nil {
		detector = NewHistoryFileDetectorWithRealInspector()
	}
	var info *HistoryFileInfo
	if i.pm().IsAlive() {                    // fast path: live PID open-files scan
		pid, err := i.pm().GetPanePID()
		...
		info, err = detector.Detect(pid)
	}
	if info == nil {                          // fallback: path-based scan, no PID needed
		effectivePath := i.GetEffectiveRootDir()
		...
		info, err = detector.DetectByPath(effectivePath)
	}
	...
	// sets i.claudeSession.ConversationUUID / i.HistoryFilePath directly
}
```

Two things fall out of this that matter for the fix:

- **`i.pm().IsAlive()` is false before `initTmuxSession()` ever runs**, for exactly the
  same reason `HasSession()` is false: `TmuxProcessManager.IsAlive()`
  (`session/tmux_process_manager.go:81-87`) reads `tm.session.Load()`, which is `nil`
  until `initTmuxSession()` calls `SetSession()`. So calling `i.pm().IsAlive()` (and by
  extension `tryExtractConversationUUID`'s internal `IsAlive()` check) *before*
  `initTmuxSession()` returns the **identical** `false` it would return *after* — moving
  the check earlier does not change its meaning for the cold-restart case. This means
  `tryExtractConversationUUID()` called pre-launch will automatically skip the (n/a)
  fast path and go straight to `DetectByPath`, with zero new logic required.
- The function's doc comment ("The tmux session must be alive for this to work...") is
  **stale** — it predates the `DetectByPath` fallback and only describes the fast path.
  Worth a one-line comment fix when this is implemented, since a future reader could
  otherwise conclude the function is unsafe to call pre-launch.

`GetEffectiveRootDir()` (`session/instance_worktree.go:166-`) — returns the worktree
path if `i.gitManager.HasWorktree()`, else presumably `i.Path` (guard already exercised
identically by both the pre-existing post-launch call and `HistoryLinker.correlateSession`,
so no new dependency is introduced by calling it earlier).

**Conclusion: no new detection code is needed.** The existing `tryExtractConversationUUID()`
method, called *before* `initTmuxSession()` instead of only *after* `i.pm().Start()`,
already does exactly what's required — same signature, same test-injectable
`i.historyDetector` field (`session/instance.go:268`, unexported, set directly by
in-package tests such as `instance_cold_restore_test.go`).

## 3. Does the paused-session safeguard in `correlateSession` need duplicating?

Read `session/history_linker.go` `correlateSession()` (lines 212-307) in full.

The safeguard:

```go
// session/history_linker.go:264-265
pathFallbackAllowed := !alreadyLinked ||
	(inst.Status != Paused && inst.Status != Hibernated && inst.Status != Stopped)
```

This exists to protect **already-linked** sessions (`alreadyLinked == inst.HasClaudeSession()
== true`) from having their correct, previously-detected UUID clobbered by a *newer*
JSONL file that belongs to a different session sharing the same project directory —
`DetectByPath`'s "most recently modified wins" heuristic is only trustworthy for
sessions that have never been linked, or are still actively running (where a newer file
in the same dir really is this session's own `/clear`-triggered new conversation).

**The new pre-launch call does not need this safeguard duplicated, and is naturally
safe** — for the same reason stated in requirements.md item 3, now confirmed by reading
the code: `tryExtractConversationUUID()`'s very first statement
(`instance_claude.go:310-312`) is an unconditional early-return when
`i.claudeSession.ConversationUUID != ""`. The proposed call site is gated on exactly
that condition being false (see §4). So:

- If the session already has a stored UUID (the paused-session-with-correct-UUID case
  the safeguard protects), the new pre-launch call is a no-op — it never reaches
  `DetectByPath` at all. Nothing to overwrite, nothing to protect.
- The new call can therefore only ever act in the `alreadyLinked == false` case, which
  is exactly the case `correlateSession`'s own safeguard already treats as
  "path fallback always allowed" (`!alreadyLinked ||`). The two call sites agree.
- One residual, **pre-existing** risk (not introduced by this fix): a *never-linked*
  session whose directory is shared with another session that *has* written a newer
  JSONL could still pick up the wrong UUID via "most recent wins." This risk already
  exists today for `correlateSession`'s background polling of unlinked sessions
  (`!alreadyLinked` unconditionally allows the path fallback) and for the existing
  post-launch `tryExtractConversationUUID()` call. The new pre-launch call does not
  change this risk's shape or magnitude — worth a one-line note in the plan, not a
  blocker.

## 4. Proposed insertion point(s)

**Primary proposal — minimal diff, reuses existing method, no new locking, no
actor-model violation:**

Insert a guarded call to the *existing* `i.tryExtractConversationUUID()` immediately
before `i.initTmuxSession()`, gated on the same conditions the later branch already
uses (`!firstTimeSetup && !i.pm().IsAlive()`), narrowed further to only fire when there
is no in-memory UUID yet (the method already no-ops in that case, but gating avoids an
unnecessary directory scan / log line on every hot-restore and first-time-setup call):

```go
// startLocked, session/instance.go — insert between the Title guard (~845) and
// the existing i.initTmuxSession() call (~847):

if !firstTimeSetup && !i.pm().IsAlive() && !i.HasClaudeSession() {
	// Recover a persisted conversation UUID from disk BEFORE initTmuxSession()
	// reads i.claudeSession.ConversationUUID to decide whether to embed --resume.
	// i.pm().IsAlive() is guaranteed false here (see architecture research,
	// tmux_process_manager.go:81-87: tm.session is nil until initTmuxSession()
	// registers it below), so tryExtractConversationUUID's internal fast-path
	// PID check is skipped automatically and it falls straight to DetectByPath —
	// no new detection logic needed, just calling it earlier.
	i.tryExtractConversationUUID()
}

i.initTmuxSession()
```

This is a **pure reordering** — no new lock acquisition (the actor already guarantees
single-goroutine access to `i` for the duration of `startLocked`; `tryExtractConversationUUID`'s
doc comment "assumes stateMutex is already held by the caller" is satisfied the same way
it already is for its existing post-launch call site: by running inside the actor, not
by an explicit lock), and no call back into `Start()`/the actor from within itself —
consistent with the constraint in `project_plans/instance-actor-concurrency/research/architecture.md`
(§"command closures must call internal, lock-free twins... never the public sendSync-wrapped
API"). `tryExtractConversationUUID` is not `sendSync`-wrapped at all — it's a plain
field-mutating method already called directly from inside `startLocked`, so calling it a
second time (earlier) from the same function carries identical safety properties to the
existing call.

**This exact same reordering must be applied at both mirrored call sites**
(`instance.go` `startLocked` ~834-996, and legacy `start()` ~1012-1200+), since both
currently have the identical bug (confirmed identical structure and even near-identical
comments at 899-910 vs 1101-1116). Whether the fix should also finally collapse `start()`
into `startLocked` (per the actor-migration doc's stated direction) or just be
duplicated in both is a planning-phase decision, not this research's call — flagging it
since the requirements only mention `startLocked` explicitly but the bug is symmetric.

**Secondary consideration — does NOT clearing `i.claudeSession.ConversationUUID` after
`Start()` still make sense?**

Both call sites currently do this immediately after `i.pm().Start(startPath)` succeeds
in the cold-restore branch:

```go
if i.claudeSession != nil {
	i.claudeSession.ConversationUUID = ""
	i.HistoryFilePath = ""
}
i.tryExtractConversationUUID()
```

This clear-then-redetect exists so that if `--resume <uuid>` was stale (Claude creates a
*new* conversation because the old one is gone) the code doesn't cling to the wrong
UUID. With the new pre-launch recovery in place, this second call becomes a no-op in the
common case (fast path via live PID re-confirms the *same* UUID DetectByPath already
found), but it must be **kept**, not removed: it is the only mechanism that would notice
Claude minted a *different* UUID than the one passed to `--resume`. Do not shortcut this
post-launch step just because the pre-launch call already ran — they answer different
questions ("what UUID should we launch with" vs. "what UUID did Claude actually end up
using").

## 5. Test injection points for the acceptance-criteria regression test

`session/instance_cold_restore_test.go` already contains `TestColdRestore_WithUUID`
(UUID pre-set in memory) and `TestColdRestore_WithoutUUID` (no UUID, no JSONL) using
real tmux via `NewInstanceWithCleanup` + `StartWithCleanup(false)`. The missing case —
"dead tmux + empty UUID + JSONL present on disk" — fits the same pattern:

- Do **not** call `inst.SetClaudeSession(...)` (leave `i.claudeSession` nil, matching
  the bug's precondition).
- Inject a fake `historyDetector` via the unexported field (test file is
  `package session`, same package, so direct field access works — see
  `instance.go:268`, doc comment confirms "Set in tests to inject a fake home dir"):
  `inst.historyDetector = NewHistoryFileDetectorWithHomeDir(procinfo.NewProcessInspector(), fakeHome)`.
- Pre-write a valid `~/.claude/projects/<ClaudeProjectDirName(path)>/<uuid>.jsonl` file
  under `fakeHome` before calling `Start(false)`, where `path` matches
  `inst.GetEffectiveRootDir()` (== `inst.Path` for a non-worktree `SessionTypeDirectory`
  instance, matching the existing tests' `SessionType: SessionTypeDirectory`).
- Assert (after `Start(false)`) both: `inst.GetConversationUUID() == uuid` and — the
  behavior this whole fix is about — `strings.Contains(inst.LaunchCommand, "--resume")`
  and `strings.Contains(inst.LaunchCommand, uuid)`, proving recovery happened *before*
  the launch command was built, not just that `tryExtractConversationUUID` eventually
  ran. `i.LaunchCommand` is set at `instance_tmux.go:259` inside `initTmuxSession()`
  and is a plain exported-ish field readable from the test (same package).
- A companion "must NOT overwrite a paused session's UUID" test can reuse
  `history_linker_test.go`'s existing safeguard-test pattern (multiple JSONLs in one
  project dir, one older/correct + one newer/foreign) but assert
  `tryExtractConversationUUID` — called with `i.claudeSession.ConversationUUID` already
  set — is a no-op, confirming requirement 5 without needing to touch `correlateSession`
  at all.

## Summary of concrete findings for the planning phase

1. Root cause confirmed: `initTmuxSession()` (line ~847/1029) reads the in-memory UUID
   ~20 lines before the code that would recover it from disk even runs, and ~60 lines
   before that recovery (`tryExtractConversationUUID`) actually executes today (only
   after `pm().Start()`).
2. No new detection code needed — `tryExtractConversationUUID()` already has the right
   internal guards (`IsAlive()` fast-path / `DetectByPath` fallback) to be safely called
   before a tmux session exists; `IsAlive()` is provably `false` at that point regardless
   of call order, since it depends on `initTmuxSession()`'s own `SetSession()` call.
3. Fix is a pure reorder: hoist a guarded call to `i.tryExtractConversationUUID()` above
   `i.initTmuxSession()` in the `!firstTimeSetup && !IsAlive()` case, at **both** mirrored
   locations (`startLocked` and legacy `start()`).
4. No duplication of `correlateSession`'s paused-session safeguard is needed — the new
   call only ever fires when there's no stored UUID to protect, which is exactly the
   `!alreadyLinked` case that safeguard already treats as safe.
5. Keep the existing post-launch clear+redetect logic; it protects against a *different*
   failure mode (stale `--resume` UUID causing Claude to mint a new conversation) that
   pre-launch recovery cannot detect.
6. `instance_cold_restore_test.go` is the natural home for the new regression test —
   same fixtures, same real-tmux integration style as the two existing
   `TestColdRestore_*` tests, plus direct access to the unexported `historyDetector`
   test-injection field.
