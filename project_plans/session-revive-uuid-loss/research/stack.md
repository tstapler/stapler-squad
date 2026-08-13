# Research: Stack & Existing Patterns — session-revive-uuid-loss

## TL;DR

This is a pure Go bugfix in existing files. No new dependency is needed — every
building block (JSONL scanning, UUID detection, actor-locking pattern, test
doubles) already exists and should be reused, not reinvented. The one real gap
is the **user-visible signal** (Goal 3 / AC3): there is no existing "session
event/notice" primitive in `session/` to hang that off of — that will need a
small new field/mechanism, designed in the plan phase, not research.

## Go module / version

- `go.mod:3` — `go 1.26.3`. Modern Go; generics, `slices`/`maps` stdlib, etc.
  all available if useful, though this fix doesn't need them.
- Relevant existing deps (no new ones required):
  - `github.com/google/uuid v1.6.0` — already used for UUID validation
    (`isValidUUID` in `session/history_detector.go`).
  - `github.com/fsnotify/fsnotify v1.9.0` — used by `session/history_watcher.go`
    for **live** JSONL watching (continuous, event-driven). Not applicable here:
    this bug is about a **point-in-time decision** made once at cold-restore
    time, not continuous watching. Do not reach for fsnotify.
  - `github.com/stretchr/testify v1.11.1` — existing test assertion library,
    used throughout `session/*_test.go`.

## Existing UUID-recovery machinery (reuse, don't reinvent)

- `session/history_detector.go`:
  - `HistoryFileDetector.Detect(pid int32)` (line 59) — fast path, inspects a
    **live** tmux pane process's open FDs via `ProcessFileInspector`.
  - `HistoryFileDetector.DetectByPath(projectPath string)` (line 137) — the
    fallback this bug is about. Scans `~/.claude/projects/<encoded-path>/` for
    `*.jsonl` files, filters out `agent-*` and non-UUID basenames, and returns
    the most-recently-modified candidate as `*HistoryFileInfo{ConversationUUID,
    HistoryFilePath, ProjectDir}`. Returns `(nil, nil)` — not an error — when
    the directory doesn't exist or has no valid candidates (AC2's "no jsonl ⇒
    behave exactly as today" falls out of this for free, since `DetectByPath`
    already treats "nothing found" as a clean no-op).
  - `ClaudeProjectDirName(projectPath string)` (line 118) — the path-encoding
    helper (non-alphanumeric → `-`), already correctly used with
    `i.GetEffectiveRootDir()` (worktree-aware) in `tryExtractConversationUUID`.
- `session/instance_claude.go:308` — `tryExtractConversationUUID()` already
  implements the exact call sequence needed: live-PID fast path, then
  `DetectByPath` fallback, then sets `i.claudeSession.ConversationUUID` /
  `i.HistoryFilePath` directly. **This is the function to reuse/reorder, not a
  new detector.** Its doc comment already documents the actor/locking contract:
  "assumes stateMutex is already held by the caller... must NOT be called
  without the lock."

## The actual bug mechanics (confirmed by reading, relevant to fix design)

The requirements describe the bug as `HasClaudeSession()` gating the
resume/fresh decision before `tryExtractConversationUUID()` runs. Reading the
code shows the bug is slightly sharper than "just a log line":

- `startLocked` calls `i.initTmuxSession()` at `session/instance.go:858` —
  **before** the `if !i.pm().IsAlive()` cold-restore branch even starts. Per
  `session/instance_tmux.go:249`, `initTmuxSession()` is what builds the
  `ClaudeCommandBuilder` command line, and it decides whether to embed
  `--resume <uuid>` based on `i.claudeSession.ConversationUUID` **as it stands
  at that moment** — i.e. before any fallback recovery has run.
- The `if i.HasClaudeSession() { ... } else { ... }` block at
  `instance.go:881-885` (and its mirror at `instance.go:1072-1080`) currently
  only selects which **log line** to print — the actual `--resume` decision was
  already baked into the tmux command by `initTmuxSession()` earlier.
- `tryExtractConversationUUID()` is called at `instance.go:921` /
  `instance.go:1127`, **after** `i.pm().Start(startPath)` has already launched
  the process without `--resume`. Its results are only used for whatever comes
  next (HistoryLinker correlation, subsequent restarts), not to change what
  was already spawned.
- **Implication for the plan phase:** satisfying AC1 ("revive resumes using
  the UUID recovered from JSONL... instead of starting a fresh Claude
  process") requires running the `DetectByPath` recovery attempt **before**
  `initTmuxSession()` builds the command — not merely before the
  `log.Info`/`log.Warn` branch where the requirements' line numbers point.
  Moving/duplicating `tryExtractConversationUUID()`'s fallback logic earlier
  in `startLocked` (and its restart mirror) is the shape of the fix; both call
  sites need the same ordering per AC4 ("no duplicated divergent logic").

## Related prior work — already landed, don't re-fix

- `d9816ec77` (already merged) — fixed HistoryLinker *registration* gaps
  (sessions created post-boot never got linked) and made `SetHistoryInfo`
  persist immediately via a save callback
  (`session/instance_claude.go:464`, fires on UUID change). Orthogonal to this
  bug: that fix is about *linker registration timing*; this bug is about
  *decision-time UUID-recovery ordering* at cold-restore. No overlap requiring
  rework.
- `project_plans/session-resume-uuid-fix/` (older, distinct plan) — **already
  implemented**, confirmed live in `session/history_linker.go:274`
  (`pathFallbackAllowed := !alreadyLinked || (inst.Status != Paused &&
  inst.Status != Hibernated)`), landed in commit `b55101143` ("fix(history-linker):
  preserve paused/hibernated session UUID on rescan (#118)"). That fix stops
  `DetectByPath` from clobbering a paused/hibernated session's *correct*
  stored UUID with a newer JSONL from a different session sharing the same
  directory. This is unrelated to (and already resolved, no re-derivation
  needed for) the cold-restore ordering bug this project targets.

## Test patterns already in place (reuse the harness, don't build a new one)

- `session/instance_cold_restore_test.go` — **directly on point**. Existing
  tests:
  - `TestColdRestore_WithUUID` (line 44) — dead tmux + UUID present → resume path.
  - `TestColdRestore_WithoutUUID` (line 102) — dead tmux + no UUID → fresh path.
  - `TestHotRestore_ExistingSession` (line 146), `TestIsStaleResumeExit` (line 220).
  - The new AC1/AC2 tests ("recovery finds a UUID on disk that was never in
    memory" / "recovery finds nothing, behaves as today") belong in this file
    as siblings of `TestColdRestore_WithUUID`/`WithoutUUID`, following the same
    setup pattern (this file already stands up a dead-tmux instance and
    inspects `pm().Start` args / `claudeSession` state after `startLocked`).
- `session/history_detector_test.go` — `mockProcessInspector` (line 15) is the
  existing `ProcessFileInspector` test double; for `DetectByPath` tests no
  mock is needed since it hits the real filesystem — existing tests there
  already show the pattern of writing real JSONL files under a temp
  `$HOME/.claude/projects/<dir>/` and pointing `NewHistoryFileDetectorWithHomeDir`
  at the temp dir.
- `session/instance_claude_test.go` — `TestInstance_SetHistoryInfo_*` (lines
  15/30/44) show the pattern for asserting on `ConversationUUID`/callback
  firing; useful if the fix also needs to assert the persistence callback
  still fires correctly after recovery.
- Actor/locking convention: per `tryExtractConversationUUID`'s own doc
  comment, any new code path that touches `i.claudeSession` directly must run
  under `stateMutex` (already held inside `startLocked`) — this matches the
  project's actor-mailbox pattern referenced elsewhere in `session/`
  (`instance_actor_setters.go`, `session_driver.go`). No new locking primitive
  needed; just preserve the existing single-writer-under-lock discipline.

## Gap: no existing "user-visible signal" primitive (Goal 3 / AC3)

Searched `session/*.go` for an existing session-level event/notice/banner
mechanism to reuse for "started fresh after failed recovery, previously had
history":
- No generic `SessionEvent`/`Notice` field on `Instance` exists. The closest
  analogues are narrower-purpose: `backlog_lifecycle.go`'s `Notifier`
  interface (backlog-item notifications, not session-level), and
  `session_summary_snapshot.go`'s `NotificationDecisionLister` (unrelated
  domain).
- The `SherClockHolmes/webpush-go` dependency exists in `go.mod` but is used
  for push notifications, not in-app session state — likely too heavy/wrong
  layer for this.
- **Conclusion for the plan phase:** this needs a small new mechanism —
  most likely a new field on the session status/state (e.g. a
  `ResumeOutcome`/`lastColdStartResult` enum-ish string field persisted
  alongside existing session state, or a lightweight append-only session
  event log) plus a proto field so the frontend can render it, per this
  repo's existing convention of "self-heal/fallback actions must be visible,
  not silent" (`.claude/rules` /
  `feedback_document_ai_decisions_in_edge_cases` memory). This is a design
  decision, not a reuse question — flag it explicitly for `sdd:3-plan`.

## Idioms to follow (per repo conventions, not new to this fix)

- Prefer extending the existing `tryExtractConversationUUID` (or factoring its
  fallback body into a small helper callable from both `initTmuxSession`-time
  and its current call sites) over introducing a new detector type — avoids
  interface pollution per `.claude/rules/interface-pollution-checklist.md`
  (no new `Resolver`/`Recovery` interface needed; `HistoryFileDetector` already
  has exactly the two methods required).
- Keep both call sites (`instance.go` `startLocked` and its restart mirror)
  sharing one code path rather than two copies, per AC4 and per this repo's
  general "don't duplicate divergent logic" bias.
- No subshells/CLI calls needed — this is pure in-process file I/O via
  `os.ReadDir`, already how `DetectByPath` works; nothing here touches git, so
  `prefer-go-git-over-subshells` doesn't apply.
