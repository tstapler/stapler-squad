# Stack Research: Session Cold-Start UUID Loss

## 1. How `ConversationUUID` is captured today

`Instance.claudeSession` (`*ClaudeSessionData`, guarded by `claudeSessionMu`) holds
`ConversationUUID` — [`session/instance.go:299`](/home/tstapler/Programming/stapler-squad/session/instance.go#L299),
struct defined at [`session/storage.go:161-169`](/home/tstapler/Programming/stapler-squad/session/storage.go#L161-L169)
(`json:"session_id,omitempty"`).

Three independent writers populate it:

1. **`Instance.tryExtractConversationUUID()`** —
   [`session/instance_claude.go:308-363`](/home/tstapler/Programming/stapler-squad/session/instance_claude.go#L308-L363).
   Called synchronously from inside `startLocked`/`start` (see below) and from
   `instance_workspace.go:79`. Doc comment: "caller must already hold
   stateMutex" — it **sets `i.claudeSession.ConversationUUID` and
   `i.HistoryFilePath` directly** (line 360-361), bypassing `SetHistoryInfo`/
   `SetClaudeConversationUUID`. This means **no persistence callback fires**
   when this path recovers a UUID — a durable-persistence gap directly
   relevant to Acceptance Criterion #2.
2. **`Instance.SetHistoryInfo(uuid, path)`** —
   [`session/instance_claude.go:464-495`](/home/tstapler/Programming/stapler-squad/session/instance_claude.go#L464-L495).
   Thread-safe setter, no-ops if unchanged, and **does** fire
   `claudeSessionIDSavedCallback` when the UUID actually changes — comment at
   line 461-463 explicitly states the intent: "a HistoryLinker-detected UUID
   is persisted to durable storage immediately... a tmux pane killed before
   that sweep runs would otherwise resume with no conversation UUID." This is
   used exclusively by `HistoryLinker.correlateSession` (see §3).
3. **`Instance.SetClaudeConversationUUID(uuid)`** —
   [`session/instance_claude.go:429-444`](/home/tstapler/Programming/stapler-squad/session/instance_claude.go#L429-L444).
   Same callback-firing behavior; used by `agy_adapter.go`, `RunWithResume`.

The callback itself is wired at the service layer —
`SessionService.wireClaudeSessionIDCallback`
([`server/services/session_service.go:4031-4040`](/home/tstapler/Programming/stapler-squad/server/services/session_service.go#L4031-L4040)):
```go
inst.SetClaudeSessionIDSavedCallback(func() {
    _ = s.storage.SaveInstances([]*session.Instance{inst})
})
```
This is the **existing persist-on-capture pattern** — AC #2 ("persisted... as
soon as it is captured") is already satisfied for `SetHistoryInfo`/
`SetClaudeConversationUUID` callers, but **not** for `tryExtractConversationUUID`,
which is the one actually invoked inline during cold restore.

## 2. Root cause — confirmed, more precise than the requirements hypothesis

Both call sites (`startLocked` line 845, `start` line 1023) call
`i.initTmuxSession()` **before** the `!i.pm().IsAlive()` cold-restart branch:

- `startLocked`: `initTmuxSession()` at
  [`session/instance.go:858`](/home/tstapler/Programming/stapler-squad/session/instance.go#L858),
  cold-restart branch starts at
  [`session/instance.go:878`](/home/tstapler/Programming/stapler-squad/session/instance.go#L878).
- `start`: same ordering, `initTmuxSession()` runs above line 1023's window,
  cold-restart branch at
  [`session/instance.go:1068`](/home/tstapler/Programming/stapler-squad/session/instance.go#L1068).

`initTmuxSession()` ([`session/instance_tmux.go:249-260`](/home/tstapler/Programming/stapler-squad/session/instance_tmux.go#L249-L260))
reads `i.claudeSession.ConversationUUID` **once**, at that moment, into
`claudeSessionID`, and bakes it into the tmux launch command via
`buildLaunchCommand` → `ClaudeCommandBuilder` — the `--resume <uuid>` flag is
only appended if `claudeSessionID != ""`
([`session/claude_command_builder.go:47-52`](/home/tstapler/Programming/stapler-squad/session/claude_command_builder.go#L47-L52)).

So: **whatever `HasClaudeSession()` would report is captured into the launch
command before the code ever checks it.** The `if i.HasClaudeSession() {...} else {...}`
block at lines 881-885 / 1072-1080 only decides *which log line to print* — it
has no effect on whether `--resume` is actually used, because the command
string is already frozen. If a restart races a concurrent
clear/re-detect window (e.g. a prior `ClearConversationState()` call, or the
instance was just reloaded from storage and `HistoryLinker` hasn't yet run
its next poll tick / fsnotify callback to relink it), `claudeSession.ConversationUUID`
is empty at `initTmuxSession()` time, Claude starts fresh, and the `tryExtractConversationUUID()`
call made afterward (lines 921 / 1127, *after* `Start()`) only detects
**whatever new conversation Claude just created** (fast path: live pane's
open files) — it cannot recover the old, now-orphaned conversation, because
by that point Claude has already decided (no `--resume`) to start a new one.

This matches the reporter's observation of inconsistent behavior across
back-to-back restarts: it is a genuine ordering race between (a) when
`initTmuxSession()` snapshots the in-memory UUID and (b) when the UUID is
actually populated in memory (via `HistoryLinker`'s async poll/fsnotify loop,
which is the primary populator for a session that was ever restarted).

**Implication for the fix:** recovery (from on-disk JSONL or durable storage)
must happen *before* `initTmuxSession()` is called in both `startLocked` and
`start` — not just before the `HasClaudeSession()` log-branch, and not only
after `Start()` returns.

## 3. On-disk JSONL format / encoding / "newest transcript" lookup — already implemented, reusable

`session/history_detector.go` (`HistoryFileDetector`) is the existing,
already-tested mechanism — **no new JSONL scraper should be written.**

- **Path encoding**: `ClaudeProjectDirName(projectPath string)` —
  [`session/history_detector.go:118-129`](/home/tstapler/Programming/stapler-squad/session/history_detector.go#L118-L129).
  Replaces every non-alphanumeric byte with `-` (covers `/`, `.`, `_`, etc.).
  Tested exhaustively in
  [`session/history_detector_test.go:155-170`](/home/tstapler/Programming/stapler-squad/session/history_detector_test.go#L155-L170)
  including a real worktree path with both `.` and `_`.
- **Newest-JSONL-for-a-directory lookup**: `HistoryFileDetector.DetectByPath(projectPath string)` —
  [`session/history_detector.go:137-199`](/home/tstapler/Programming/stapler-squad/session/history_detector.go#L137-L199).
  Lists `~/.claude/projects/<encoded-path>/*.jsonl`, skips `agent-*.jsonl` and
  non-UUID basenames (`isValidUUID`,
  [`session/claude_command_builder.go:91-94`](/home/tstapler/Programming/stapler-squad/session/claude_command_builder.go#L91-L94)),
  sorts by `ModTime` descending, returns the newest as `HistoryFileInfo{ConversationUUID, HistoryFilePath, ProjectDir}`.
  **Works with no live process** (doc comment: "does NOT require a live
  process, making it suitable for sessions whose tmux session is dead") —
  exactly the revive scenario in scope here.
- **Live-process fast path** (for comparison, not usable when tmux is dead):
  `HistoryFileDetector.Detect(pid)` —
  [`session/history_detector.go:59-111`](/home/tstapler/Programming/stapler-squad/session/history_detector.go#L59-L111)
  — inspects open FDs via `procinfo.ProcessFileInspector`.

`Instance.tryExtractConversationUUID()` already calls `DetectByPath` as its
fallback (lines 336-349) when the live-pane fast path fails — this is the
exact "recover from newest on-disk JSONL" logic AC #1 asks for. **The gap is
purely about *when* it runs relative to `initTmuxSession()`, and that it
doesn't persist through the normal callback path (§1).**

## 4. Existing background reuse candidate: `HistoryLinker`

[`session/history_linker.go`](/home/tstapler/Programming/stapler-squad/session/history_linker.go)
is a second, independent consumer of the same `HistoryFileDetector`, running
as a background service (5s poll + fsnotify watcher on
`~/.claude/projects/`). Its `correlateSession` (lines 231-317) does the same
Detect→DetectByPath two-step and calls `inst.SetHistoryInfo(...)`, which
**does** trigger the persistence callback. `wireCallbacks` in
`session_service.go` (lines 984-1005) registers every instance with it — a
comment there (lines 995-1001) documents an almost-identical prior bug fixed
2026-08-02 (sessions never registered with `HistoryLinker` at all, so
`HasClaudeSession()` stayed permanently false). That fix closed the
"never captured" case; this project's bug is the narrower "captured
async, but the synchronous cold-restore path races ahead of it" case.

`HistoryLinker.correlateSession`'s `pathFallbackAllowed` guard (lines 268-275)
is also worth noting for the fix design: it intentionally **skips** the
path-based (most-recently-modified) fallback for already-linked
Paused/Hibernated/Stopped sessions, because that heuristic can steal a
different session's newer JSONL in a shared directory. Any new
recovery-before-`initTmuxSession()` logic should apply the same caution if it
reuses `DetectByPath` for a session that already believes it's linked but
the in-memory value is merely stale/cleared.

## 5. Persistence / serialization patterns to follow

- **Domain struct**: `ClaudeSessionData` in
  [`session/storage.go:161-169`](/home/tstapler/Programming/stapler-squad/session/storage.go#L161-L169) —
  `ConversationUUID string \`json:"session_id,omitempty"\``. Note the
  intentional JSON-tag/Go-field name mismatch (kept for on-disk backward
  compatibility) and the precedent for compat shims: `UnmarshalJSON` at
  [`session/storage.go:175-`](/home/tstapler/Programming/stapler-squad/session/storage.go#L175)
  reads a legacy `conversation_id` key as a fallback for `squad_session_id` —
  the same pattern would apply if a fix ever needs a new persisted field.
- **Instance → durable form**: `Instance.ToInstanceData()` —
  [`session/instance_serialization.go:38-`](/home/tstapler/Programming/stapler-squad/session/instance_serialization.go#L38),
  copies `data.ClaudeSession = *i.claudeSession` (line 147 per earlier grep) —
  any new in-memory field on `ClaudeSessionData` is automatically carried
  through as long as it's set on that struct; no separate wiring needed there.
- **Write path**: `Storage.SaveInstances` → `saveInstancesToRepo` →
  `EntRepository` (`session/ent_repository.go:299`, `:561-634`) —
  `SetClaudeSessionID(data.ClaudeSession.ConversationUUID)` persists into the
  `claude_session` ent table (separate table from `session`, joined by edge).
- **Callback-triggered save**: the established pattern for "persist the
  instant a mutation happens, not on the next full sweep" is
  `claudeSessionIDSavedCallback` (§1) — this is the idiom a fix should extend
  (e.g., make `tryExtractConversationUUID` route through `SetHistoryInfo`
  instead of setting fields directly) rather than inventing a new save
  trigger.
- **Read path for "was a UUID ever durably persisted"**: two existing options,
  both already wired:
  - `Storage.FindInstanceDataByID(id string) (*InstanceData, error)` —
    [`session/storage.go:407`](/home/tstapler/Programming/stapler-squad/session/storage.go#L407) (full instance reload).
  - `Storage.GetClaudeConversationUUIDBySessionUUID(ctx, sessionUUID string) (string, error)` —
    [`session/storage.go:1072-1080`](/home/tstapler/Programming/stapler-squad/session/storage.go#L1072-L1080) →
    `EntRepository.GetClaudeConversationUUIDBySessionUUID` —
    [`session/ent_repository.go:706-724`](/home/tstapler/Programming/stapler-squad/session/ent_repository.go#L706-L724),
    a narrow, already-tested query keyed on session title. Already wired for
    a different purpose (search: tmux-UUID → Claude-UUID resolution) via
    `SessionService.SetResolveConversationUUID` →
    [`server/dependencies.go:1129`](/home/tstapler/Programming/stapler-squad/server/dependencies.go#L1129).
  - **Architectural note**: `Instance` (package `session`) holds **no
    reference to `*Storage`** — persistence is one-directional today
    (Instance → callback → service layer → Storage). A "reload from durable
    storage before `HasClaudeSession()` runs" fix (AC #2's second half)
    therefore has two shapes to choose between in planning: (a) keep
    `Instance` storage-agnostic and only recover from the on-disk JSONL via
    `tryExtractConversationUUID`/`DetectByPath` (self-contained, no new
    dependency), or (b) add a second callback hook (mirroring
    `SetClaudeSessionIDSavedCallback`'s shape) that the service layer wires
    to a storage lookup, invoked lazily when `ConversationUUID` is empty.
    Option (a) covers the disk-transcript recovery AC #1 explicitly asks for
    without new coupling; option (b) is needed only if in-memory-empty but
    JSONL-also-missing-yet-still-durably-known cases exist in practice
    (e.g., the JSONL was pruned but the UUID row survives) — worth confirming
    in the architecture research whether that case is real or purely
    theoretical.

## 6. Existing tests / fixtures

- **`session/instance_cold_restore_test.go`** — `TestColdRestore_WithUUID`,
  `TestColdRestore_WithoutUUID`, `TestHotRestore_ExistingSession` (real tmux
  integration tests, `testing.Short()`-skipped). These exercise
  `StartWithCleanup(false)` end-to-end but only assert lifecycle status
  (`Running`, tmux alive) — **they do not assert which UUID was actually
  used**, per their own comment ("--resume flag injection is verified at the
  unit level in claude_command_builder_test.go"). New tests for AC #7(a)/(b)
  will need a new integration test (or a lower-level unit test around
  `initTmuxSession`/`buildLaunchCommand`) that actually inspects the built
  command string or a fake JSONL fixture directory, since these existing
  tests don't cover the "in-memory empty, on-disk present" case at all.
- **`instance_cold_restore_test.go:219-257`** — `TestIsStaleResumeExit` /
  `isStaleResumeExit` (`session/instance_claude.go:19-29`) is a **related,
  independent precedent**: when `--resume <uuid>` fails because Claude's
  backend no longer has that conversation, `instance_controller.go:71-73`
  detects the PTY-exit error text and calls `recoverFromStaleResume()`
  ([`session/instance_claude.go:78-96`](/home/tstapler/Programming/stapler-squad/session/instance_claude.go#L78-L96)),
  which clears state and restarts fresh — silently, with only `log.Info`/
  `log.Error`, no UI signal. This is the same "silent fresh start" shape
  named in AC #4 and is worth fixing consistently in the same change if the
  plan touches that surfacing mechanism, though it's a distinct trigger
  (stale ID *rejected by Claude* vs. ID *never resumed at all*).
- **`session/history_linker_test.go`** — covers `correlateSession`'s
  Detect/DetectByPath/backoff/park behavior directly; a good template for
  unit-testing any new recovery call added to `startLocked`/`start`.
- **`session/history_detector_test.go`** — covers `DetectByPath` and
  `ClaudeProjectDirName` directly (fixture-based, uses
  `NewHistoryFileDetectorWithHomeDir` to avoid touching the real
  `~/.claude/projects/`) — the right pattern to reuse for any new fixture
  needed for AC #7.
- **`session/session_restart_test.go`** — `TestSessionRestartWithConversationContinuity`
  subtests (`RestartWithValidClaudeSession`, `RestartWithoutClaudeSession`,
  `RestartWithInvalidUUID`, `LazyRecoveryRestart`) — closest existing
  precedent for restart-cycle UUID-continuity testing; `LazyRecoveryRestart`
  in particular should be read before writing new tests to avoid duplicating
  coverage.
- No existing test exercises the specific race (`HasClaudeSession()` true implied at
  build-launch-command time but stale from a *prior* restart's clear) or the
  "persist immediately when `tryExtractConversationUUID` recovers a UUID
  during cold restore" gap identified in §1 — both are net-new coverage
  needed per requirements AC #7.
