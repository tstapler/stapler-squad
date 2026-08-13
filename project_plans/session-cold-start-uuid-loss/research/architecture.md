# Architecture Research: Session Cold-Start UUID Loss

## 1. Data flow today

### 1.1 Where `ConversationUUID` gets set at runtime

Four distinct writers, with very different persistence behavior:

| Writer | File:line | Persists via callback? |
|---|---|---|
| `SetClaudeConversationUUID` | `session/instance_claude.go:429-444` | **Yes** — fires `claudeSessionIDSavedCallback` |
| `SetHistoryInfo` | `session/instance_claude.go:464-499` | **Yes** — fires the same callback, explicitly for this reason (see its doc comment, lines 458-463) |
| `tryExtractConversationUUID` | `session/instance_claude.go:308-363`, writes at line 360 | **No** — mutates `i.claudeSession.ConversationUUID` directly, no callback |
| `ClearConversationState` | `session/instance_claude.go:278-296` | N/A (clears to `""`), also direct-writes, no callback |

`tryExtractConversationUUID` is the one the cold-restore path actually calls (`session/instance.go:921` and `:1127`, right after a fresh `pm().Start()`). It wraps two detection strategies, both already built and reusable:

- **Live-PID fast path**: `HistoryFileDetector.Detect(pid)` (`session/history_detector.go:59-111`) — inspects the new tmux pane's open file descriptors for an open `.jsonl` under `~/.claude/projects/`.
- **Path fallback (the "recovery function" the task asks about — it already exists)**: `HistoryFileDetector.DetectByPath(projectPath)` (`session/history_detector.go:137-199`) — scans `~/.claude/projects/<ClaudeProjectDirName(projectPath)>/` (via `os.ReadDir`, no recursion, one directory), picks the most-recently-modified `*.jsonl` that isn't an `agent-*` file and has a valid UUID basename, returns `{ConversationUUID, HistoryFilePath}`. Returns `nil, nil` (not an error) if the directory doesn't exist yet — the "no such directory / first run" case is already handled cleanly.

Separately, `HistoryLinker` (`session/history_linker.go`) is a background service (5s poll + fsnotify on `~/.claude/projects/`) that also calls detection and, on a hit, calls `inst.SetHistoryInfo(...)` (`session/history_linker.go:316`) — which *does* persist. This is the "eventually consistent" path; it is not synchronous with session start.

### 1.2 Where `Instance` state is persisted to disk

`ClaudeSessionData` (`session/storage.go:162-169`), including `ConversationUUID` (`json:"session_id,omitempty"`, line 163), is part of `InstanceData`, the fully-serializable struct. Round-trip is confirmed:
- Save: `Instance.ToInstanceData()` → `Storage.SaveInstances()` → `saveInstancesToRepo` (`session/storage.go:263-282`) → `s.repo.Update`/`Create` (ent-backed, DB not flat JSON despite the "storage.go" name).
- Load: `Storage.LoadInstances()` (`session/storage.go:290-...`) → `fromInstanceData(data, true)` → `instance.claudeSession = &claudeSessionCopy` at `session/instance_serialization.go:332-335`, only if `data.ClaudeSession.ConversationUUID != ""`.

So durable storage already fully supports the UUID — **the gap is not the schema, it's the trigger**: `SaveInstances` only runs when something calls it. The one production wiring is `SessionService.wireClaudeSessionIDCallback` (`server/services/session_service.go:4031-4040`), which registers `inst.SetClaudeSessionIDSavedCallback(func() { s.storage.SaveInstances(...) })`. That callback fires from `SetClaudeConversationUUID` and `SetHistoryInfo` — **not** from `tryExtractConversationUUID`'s direct field write. So the UUID captured synchronously right after a cold restore (`session/instance.go:921`, `:1127`) sits in memory only, until either (a) `HistoryLinker`'s async sweep independently re-detects it and calls `SetHistoryInfo` (persists, but on a 5s-poll/fsnotify timescale, not synchronous with restart), or (b) some unrelated full `SaveInstances` sweep (hibernation, health check) happens to run first. A second restart before either of those completes finds `i.claudeSession.ConversationUUID` populated in memory but never durably saved — if the *process* also restarts in that window (the "service restart" leg of the reporter's repro), the in-memory value is lost entirely and load-from-storage returns whatever was last durably saved (possibly empty).

This confirms the requirements' root-cause hypothesis directly, with an exact mechanism: **`tryExtractConversationUUID` is the one UUID-setter in the whole codebase that bypasses the persistence callback**, and it's specifically the one invoked on the cold-restore hot path.

### 1.3 A second, more severe bug that compounds this: the `--resume` flag can be frozen stale even when the UUID *is* correct

This wasn't in the requirements doc's hypothesis but is directly relevant to "why does this look racy" and constrains the fix:

- `initTmuxSession()` (`session/instance_tmux.go:249-279`) is called unconditionally at the top of both `startLocked` (`session/instance.go:858`) and `start` (`session/instance.go:1046`), **before** the `HasClaudeSession()` branch.
- It early-returns at line 250-253 if `i.pm().HasSession()` is true, i.e. if the `*tmux.TmuxSession` Go object already exists in memory — **this is a pointer-non-nil check (`tmux_process_manager.go:50-52`, `tm.session.Load() != nil`), not a liveness check.** Liveness is `IsAlive()` (`tmux_process_manager.go:81-87`, `s.DoesSessionExist()`), a separate method. Nothing in `Kill()`/`Destroy()`/`Close()`/`KillSession()` ever nils `tm.session` back out (traced through `instance.go:1249` `Kill()` → `Destroy()` → `KillSession()` (`instance_tmux.go:282-289`) → `pm().Close()` → `tmux_process_manager.go:102-111`, none of which call `tm.session.Store(nil)`).
- Consequence: **once a `TmuxSession` object has been constructed for an `Instance` (first start in that process's lifetime), every subsequent same-process restart — including the `!i.pm().IsAlive()` cold-restore branch this bug is about — reuses the exact program string (`enrichedProgram`, with whatever `--resume` flag or lack thereof) that was baked in the *first* time `initTmuxSession` ran.** The current `i.claudeSession.ConversationUUID` at restart time has **no effect on the actual launch command** in this scenario; it only affects the `log.Info`/`log.Warn` line and the *next* restart's in-memory state, not the process about to be spawned by `i.pm().Start(startPath)` (`tmux/tmux.go:917-919`, which uses the immutable `t.program` field set once at construction — no `SetCommand`/`SetProgram` method exists on `TmuxSession`).
- This exactly explains the reporter's "same session cold-started fresh once, then correctly resumed via `--resume` at a later restart" observation: the resume that worked was very likely after a **full service restart** (which rebuilds `Instance`/`TmuxProcessManager` fresh from persisted storage via `fromInstanceData`, so `tm.session` is `nil` and `initTmuxSession` rebuilds the command using the freshly-loaded UUID), while the fresh-started one was an **in-process** restart (inactivity-timeout-driven, via `session/session_driver.go`) where the stale in-memory command object was reused verbatim.

**Design implication:** a fix that only recovers the UUID and reruns the `HasClaudeSession()` check/log is necessary but not sufficient. For the recovered UUID to actually produce `--resume <uuid>` on an in-process restart, either (a) `initTmuxSession`'s early-return needs to stop treating "the Go object exists" as "the command is still correct" for the dead-pane case (e.g. skip the early return, or add a way to rebuild/replace `enrichedProgram` on the existing `TmuxSession`, when `!i.pm().IsAlive()`), or (b) recovery must run and update `i.claudeSession` **before** `initTmuxSession()` is called at all (`instance.go:858`/`:1046`), and `initTmuxSession` must be made to rebuild the command whenever the pane is dead regardless of `HasSession()`. Recommend surfacing this as an explicit, separate finding to the plan phase — it may already be a live bug independent of the UUID-capture race, and the acceptance criteria ("resumes with `--resume`") cannot be satisfied by fixing only the UUID-capture side unless this is also addressed for the in-process-restart path.

## 2. Integration points for the fix

### 2.1 UUID-recovery function — already exists, needs relocation + persistence

No new detection algorithm is needed. `HistoryFileDetector.DetectByPath(effectivePath)` (`session/history_detector.go:137`) is exactly "given the session's working directory, find the newest matching JSONL under `~/.claude/projects/<encoded-path>/` and extract a UUID." It is:
- Already scoped to one project directory (not a global `~/.claude/projects/` walk) — bounded cost.
- Already handles "directory doesn't exist" (first-ever start) as a clean `nil, nil`, not an error.
- Already synchronous and fast (single `os.ReadDir` + `os.FileInfo` stats, no subprocess).

What's missing is a caller that (a) runs it **before** the `HasClaudeSession()` decision (today it's only reached *after* start, inside `tryExtractConversationUUID`, as a fallback when the live-PID path also has nothing — and per §1.3, only after the command was already built), and (b) persists what it finds via `SetHistoryInfo` (which already fires the save callback) instead of the direct-field-write `tryExtractConversationUUID` uses internally.

### 2.2 Where the persistence write needs to move

Two changes, not one:
1. **Stop bypassing the callback.** Either change `tryExtractConversationUUID`'s two write sites (`instance_claude.go:360-361`) to go through `SetHistoryInfo`/`SetClaudeConversationUUID` instead of direct field mutation, or add an explicit recovery step ahead of it that does. `SetHistoryInfo` already exists for exactly this "persist immediately, don't wait for a sweep" purpose (see its own doc comment, `instance_claude.go:458-463`) — reuse it rather than inventing a new setter.
2. **Run recovery earlier in the revive sequence** — ideally before `initTmuxSession()` (see §1.3) rather than after `pm().Start()`, so a correctly-recovered UUID can actually influence the launch command, not just the log line and the next restart.

### 2.3 Shared helper extraction — yes, with direct repo precedent

`startLocked` (`instance.go:845`, actor path, called from `Start()`) and `start` (`instance.go:1023`, `startMu`-locked legacy path, called from `StartWithCleanup()`) are confirmed near-verbatim duplicates at the cited line ranges — same VNC/CDP allocation calls, same `pm().Start`/`RestoreWithWorkDir`/`GetPTY` sequence, same clear-and-redetect comment block word-for-word. The repo already extracts logic shared between exactly these two call sites into plain `(i *Instance)` methods with no actor/lock parameter, called identically from both:
- `i.resolveStartPath(basePath)` — `session/instance_worktree.go:122`, called from `instance.go:880` and `:950` (startLocked) and `:1071`/`:1164` (start).
- `i.setupFirstTimeWorktree()` — `session/instance_worktree.go:36`, called from `instance.go:864` and `:1050`.

This is the precedent to follow: a new method like `(i *Instance) reviveConversationForColdRestart(startPath string)` (naming TBD in the plan phase) that encapsulates "check `HasClaudeSession()`, if false attempt `DetectByPath` recovery + persist via `SetHistoryInfo`, log the outcome, return whether a resume is possible" — called identically from both sites, exactly like `resolveStartPath`. No actor-awareness is needed in the helper itself since both call sites already run under serialization (`startLocked` inside the actor's mailbox; `start` inside `i.startMu.Lock()`), matching how `resolveStartPath`/`setupFirstTimeWorktree` are written today (no internal locking, callers already serialize).

### 2.4 Interface-pollution / go-git / double-checked-locking constraints

- **No new interface.** `.claude/rules/interface-pollution-checklist.md` — there is exactly one recovery implementation (`HistoryFileDetector.DetectByPath`) and one production caller shape. `HistoryFileDetector` is already injectable via `i.historyDetector` (`instance_claude.go:314-317`, defaulting to `NewHistoryFileDetectorWithRealInspector()`) for tests — reuse that seam; do not wrap it in a new "UUIDRecoverer" interface for a single implementation.
- **go-git**: not applicable here — recovery is filesystem scanning (`os.ReadDir` on `~/.claude/projects/...`), not a git operation. `.claude/rules/prefer-go-git-over-subshells.md` doesn't constrain this change.
- **Double-checked locking**: `SetClaudeConversationUUID` (`instance_claude.go:429-444`) already follows the "return the locally-computed value" discipline correctly — it writes under `claudeSessionMu.Lock()` and doesn't re-read the slot afterward for its return value (it returns nothing; callers that need the value call `GetClaudeConversationUUID()` separately, which is fine since there's no "compute vs. slot" ambiguity — there's only one writer per call, not a race between two computations of the same value). If the new recovery helper reads `HasClaudeSession()`, then recovers, then re-checks under a lock before writing, it must return/act on the **locally-recovered UUID**, not re-read `i.claudeSession.ConversationUUID` afterward, in case a concurrent `HistoryLinker` poll (§1.1) won a race and wrote a *different* (but also valid) UUID in between — otherwise the caller could act on one UUID while a different one ends up persisted, contradicting this repo's own rule.

## 3. Consistency requirements

- **Must be synchronous and fast**: yes for the per-project scan (`DetectByPath` is already just one `os.ReadDir` call on one directory, no recursion into the whole `~/.claude/projects/` tree — already bounded). No additional timeout/cap is needed for the directory-scan itself; the existing implementation doesn't walk unrelated projects.
- **First-run / no-such-directory case**: already handled — `os.ReadDir` on a missing directory returns an error that `DetectByPath` treats as `nil, nil` (`history_detector.go:146-150`), not a failure. The fix's call site just needs to treat a `nil` result as "no resumable conversation, existing fresh-start behavior stands" (AC #3), which is already the shape `tryExtractConversationUUID` uses today (`if info == nil { ...; return }`).
- **Concurrency**: the recovery+persist step will run inside the already-serialized `startLocked`/`start` bodies (actor mailbox / `startMu`), so no new locking primitive is needed for "don't run recovery twice concurrently for the same instance." The remaining concern is the `claudeSessionMu` vs. `i.mu` nesting order already documented at length in `ClearConversationState` and `SetHistoryInfo`'s comments (`instance_claude.go:281-296`, `:477-489`) — any new write path must follow that same nesting (`claudeSessionMu` outer, `i.mu` inner) rather than introducing a third lock-order variant.
- **Not addressed here, flagged for the plan phase**: `SwitchWorkspace` (`instance_workspace.go:75-81`) already calls `tryExtractConversationUUID()` *outside* any lock and outside the actor, racing `claudeSessionMu`-protected readers — a pre-existing issue, not introduced by this fix, but worth the plan phase deciding whether the new recovery helper's write path should be hardened against it now or ticketed separately (its own comment concedes "False negatives are safe — switchWorkspaceLocked re-checks under the actor," suggesting it was a deliberate, accepted tradeoff, not an oversight).

## 4. Event-Command-Policy table — skipped

This is not a multi-actor business domain. It's a single `Instance`'s revive sequence with one collaborator (`HistoryFileDetector`) and one persistence sink (`Storage.SaveInstances` via a callback). There's no saga, no cross-aggregate policy, and no need to reason about competing commands from independent actors — `startLocked`/`start` are already mutually exclusive via the actor mailbox / `startMu`. An ECP table would manufacture structure this problem doesn't have; a plain before/after description of the two call sites (§2.3) is the right level of formality, consistent with how `resolveStartPath`/`setupFirstTimeWorktree` are documented (plain doc comments, no event-sourcing framing).

## 5. Existing test coverage (for the validate phase)

`session/instance_cold_restore_test.go` already has `TestColdRestore_WithUUID` (line 44) and `TestColdRestore_WithoutUUID` (line 102), but both construct a **brand-new** `Instance` via `NewInstanceWithCleanup` and call `StartWithCleanup(false)` as the *first* start — meaning `i.pm().HasSession()` is `false` going in, so `initTmuxSession()` builds the command fresh in both cases. **Neither test exercises the actual bug scenario**: a second cold-restore of an instance that has *already been started once in the same process* (where §1.3's frozen-command issue applies). New tests for AC #7 should include that second-restart-in-same-process shape, not just first-start-after-construction, or they will pass without proving the fix works for the reported failure mode.
