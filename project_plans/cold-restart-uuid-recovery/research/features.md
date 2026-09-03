# Research: Feature/Edge-Case Landscape — Cold-Restart UUID Recovery

Agent 2 (Features), SDD research phase for `project_plans/cold-restart-uuid-recovery/`.

## 1. Interaction with prior work

### session-resume-uuid-fix (`session/history_linker.go` `correlateSession()`)

Read in full: `project_plans/session-resume-uuid-fix/requirements.md`. Its fix is **already
implemented** in `correlateSession()` (`session/history_linker.go:221-296`):

```go
alreadyLinked := inst.HasClaudeSession()
...
pathFallbackAllowed := !alreadyLinked ||
    (inst.Status != Paused && inst.Status != Hibernated && inst.Status != Stopped)
if info == nil && pathFallbackAllowed {
    ... info, err = hl.detector.DetectByPath(effectivePath) ...
}
```

This gates the "newest JSONL in the directory" heuristic **off** for already-linked
Paused/Hibernated/Stopped sessions, specifically to stop a newer sibling session's JSONL
from clobbering a paused session's correct stored UUID.

**No conflict — the two fixes target disjoint cases, and the new fix should reuse the same
detector, not add competing logic:**

- The earlier protection's guard is keyed on `alreadyLinked` = `inst.HasClaudeSession()`
  (i.e. `ConversationUUID != ""`). It only suppresses the fallback when the session
  **already has a stored UUID**.
- The new fix's trigger condition (per requirements.md) is precisely the opposite: the
  in-memory `ConversationUUID` is **empty** at `Start()` time. That is the `!alreadyLinked`
  branch, where `pathFallbackAllowed` is unconditionally `true` regardless of session
  status — the exact same branch `HistoryLinker` already treats as "safe to path-scan."
- So the new pre-launch recovery attempt is not a new heuristic to design — it's the
  existing `DetectByPath` fallback (identical to what `correlateSession()` and
  `tryExtractConversationUUID()` already call), just invoked **synchronously, earlier**
  in `Start()`/`startLocked()`/`start()`, before `initTmuxSession()` bakes the (empty)
  UUID into the launch command.
- **Recommendation for the plan phase**: don't reimplement the scan. Call
  `i.tryExtractConversationUUID()` (or extract its fallback path into a helper) *before*
  `initTmuxSession()` when `!i.HasClaudeSession()`, rather than only in the post-launch
  spot it's called today. `tryExtractConversationUUID()` already has the correct
  "skip if UUID already set" guard (`instance_claude.go:310-312`), so calling it earlier
  is safe to do unconditionally — it will simply no-op for sessions that already have a
  UUID (the case the earlier fix protects).

### session-resumption-hardening

Read in full: `project_plans/session-resumption-hardening/requirements.md`. Directly
relevant items:
- `session/history_detector.go` (`HistoryFileDetector` / `DetectByPath`) and
  `session/history_linker.go` (background `HistoryLinker`) are the same infra this fix
  reuses — confirmed "✅ Built" in that plan's status table, and confirmed still current
  by reading the code.
- That plan's Phase-1 gap-closure item "Populate `history_file_path` and
  `claude_conversation_uuid` in `server/adapters/instance_adapter.go`" is **partially
  done**: `protoSession.HistoryFilePath = snap.HistoryFilePath` is set
  (`server/adapters/instance_adapter.go:117`), but the flat `claudeConversationUuid`
  proto field is never assigned anywhere in the adapter — only the nested
  `ClaudeSession.SessionId` is (`instance_adapter.go:109-113`). See §4 below; this is a
  pre-existing, unrelated gap, not something the new fix needs to touch, but worth flagging
  since it affects observability (AC3).
- `session/instance_cold_restore_test.go` (named in that plan as a Must-Have deliverable)
  exists and is exactly the file this fix's regression test (AC4) should extend — see §2,
  it exercises `start()` via `StartWithCleanup`, not `startLocked()` via `Start()`.

## 2. The "second call site" at ~line 1061-1116

It is **not** `SwitchWorkspace` or a distinct lifecycle entry point. `session/instance.go`
has two structurally near-identical, independently-implemented bodies for starting an
instance:

- `startLocked(actorState *instanceState, firstTimeSetup bool)` (line 834) — the
  actor-safe body of `(i *Instance) Start(firstTimeSetup bool)` (line 817). Doc comment at
  823-833 explicitly calls this "the actor-safe body of `Start()`."
- `(i *Instance) start(firstTimeSetup bool, setupCleanup bool, cleanup *tmux.CleanupFunc)`
  (line 1012) — the internal implementation shared by `Start`... no, actually shared by
  `StartWithCleanup` only (comment at line 1011: "start is the internal implementation for
  Start and StartWithCleanup" is stale/inaccurate — `Start()` calls `startLocked` via
  `sendSyncErr`, not `start()`). `StartWithCleanup` (line 1000) calls `i.start(...)`
  directly at line 1004.

**Production reachability — checked by grepping all non-test callers of `.Start(` and
`StartWithCleanup(` across `session/` and `server/`:**
- Every production call site (`server/dependencies.go`, `server/services/session_service.go`,
  `server/mcp/tools_lifecycle.go`, `session/instance_serialization.go`, `session/health.go`,
  `session/instance_hibernate.go`, `session/instance_claude.go`, `session/session_driver.go`)
  calls `instance.Start(bool)` → `startLocked()`.
- `StartWithCleanup()` → `start()` has **zero non-test callers**. It is used exclusively
  by the test suite: `session/instance_cold_restore_test.go`,
  `session/comprehensive_session_creation_test.go`, `session/integration_test.go`,
  `session/session_creation_test.go`, `session/session_restart_test.go`,
  `session/mcp_integration_test.go` (30+ call sites total).

**Why this matters for the fix plan**: `start()` is not dead code (it compiles, is
exported indirectly via `StartWithCleanup`, and is exercised by dozens of tests), but it
has no production caller today. However, `session/instance_cold_restore_test.go` — the
existing regression-test file most relevant to this bug, and the natural home for the new
AC4 test — uses `StartWithCleanup()` exclusively (lines 82, 129, 174, 205), never `Start()`.
**If the fix is only applied to `startLocked()`, a regression test written against
`StartWithCleanup`/`start()` (following that file's existing convention) will not exercise
the fix at all, and will falsely pass or falsely fail depending on how it's written.** The
plan must either (a) apply the identical fix to both `startLocked()` and `start()` (true
duplication removal would be a larger refactor, likely out of scope — just mirror the
targeted change), or (b) write the AC4 test using `Start()` instead of `StartWithCleanup()`
and separately note `start()`/`StartWithCleanup()` as not-fixed/legacy. Given `start()` is
still what most existing cold-restore-adjacent tests use, mirroring the fix into both is
the safer choice and matches how the two functions already mirror each other's cold-restore
comments almost verbatim (compare `instance.go:899-910` to `instance.go:1107-1116`).

## 3. Edge cases the fix must handle

All confirmed by reading `session/history_detector.go` (`DetectByPath`,
`ClaudeProjectDirName`) and `session/instance_claude.go` (`tryExtractConversationUUID`):

- **Multiple JSONL files in the project dir** — already handled. `DetectByPath`
  (`history_detector.go:137-199`) collects all `*.jsonl` files (excluding `agent-*.jsonl`
  and non-UUID basenames via `isValidUUID`), sorts by `ModTime` descending, and returns the
  newest. No change needed; this is the same "best effort, newest wins" heuristic the
  session-resume-uuid-fix's "Out of Scope" section already accepts as a known limitation
  for *unlinked* sessions sharing a directory with other sessions.
- **Project dir doesn't exist yet** — already handled: `os.ReadDir(dir)` error is treated
  as "not an error," returns `nil, nil` (`history_detector.go:146-150`). The caller
  (`tryExtractConversationUUID`) treats `info == nil` as "nothing found," fine to fall
  through to fresh start.
- **JSONL empty/corrupt** — `DetectByPath` never opens/parses file *contents*, only reads
  directory entries (`entry.Info()` for mtime) and validates the *filename* is a UUID. An
  empty or truncated JSONL is still picked up as a candidate as long as the filename is a
  valid UUID and the file exists — `DetectByPath` does no corruption detection. Whether
  `claude --resume <uuid>` handles an empty/corrupt JSONL gracefully is a Claude CLI
  concern, out of scope for this Go-side fix; not a new edge case introduced by moving the
  call earlier (it's the same risk `tryExtractConversationUUID`'s existing post-launch call
  already carries).
- **Worktree session: `GetEffectiveRootDir()` vs `Path`** — already correct.
  `tryExtractConversationUUID` calls `i.GetEffectiveRootDir()` (`instance_claude.go:337`),
  and `correlateSession()` does the same (`history_linker.go` comment: "Use the effective
  root dir (worktree path for worktree sessions) so we look in the right
  `~/.claude/projects/` subdirectory"). `GetEffectiveRootDir()` (`instance_worktree.go:166`)
  resolves to the worktree path when `gitManager.HasWorktree()`, else `Path`. Moving the
  `DetectByPath` call earlier in `startLocked`/`start` does not change which path is passed
  — it should call the same `GetEffectiveRootDir()` accessor already used in the
  `!i.pm().IsAlive()` branch just a few lines below (`i.resolveStartPath(i.GetEffectiveRootDir())`
  at `instance.go:869`/`1060`).
- **Cost/latency of `DetectByPath` — should it block `Start()`'s critical path?** It is
  cheap and already effectively synchronous today (called inline, un-goroutined, from
  `tryExtractConversationUUID` right after `pm().Start()`). Per-call cost is one
  `os.ReadDir` on `~/.claude/projects/<encoded-path>/` plus one `entry.Info()` (stat) per
  `.jsonl` file in that single directory — not a recursive scan, not global. Directory
  sizes in practice are small (one JSONL per Claude conversation ever started against that
  exact project path — typically single digits, rarely more than a few dozen even for a
  long-lived directory). This is the same cost the fix is proposing to pay, just moved a
  few lines earlier in the same synchronous call path — no new blocking behavior, no
  argument for backgrounding it.

## 4. User-facing signal for resume vs fresh-start

Searched `web-app/src` for any UI element reflecting conversation-resume status:

- `claudeConversationUuid` (flat proto field, `web-app/src/gen/session/v1/types_pb.ts:336`)
  exists in the generated TS types but has **zero references** anywhere in
  `web-app/src/components` or hooks — it's never read by any component. It's also never
  populated server-side (see §1's adapter gap).
- The only UUID actually surfaced to the UI today is the *nested* field
  `session.claudeSession.sessionId` (== `ConversationUUID`), shown as a raw copyable string
  in `web-app/src/components/sessions/SessionDetailView.tsx:1061-1066` under "Claude Session
  ID" — this is populated via `protoSession.ClaudeSession.SessionId = cs.ConversationUUID`
  (`server/adapters/instance_adapter.go:110`).
- There is **no** UI element (badge, icon, tooltip, toast) anywhere in `web-app/src` that
  distinguishes "resumed an existing conversation" from "started fresh" from "recovered via
  path-fallback." The only current signal for any of this, as the requirements doc states,
  is the single `log.Warn`/`log.Info` line pairs in `instance.go` (`"cold restoring with
  --resume"` vs `"cold start: tmux dead, no conversation UUID, starting fresh"`), which are
  server logs only — invisible in the web UI. AC3 ("Log/observability must distinguish
  'never had a conversation' from 'had one but couldn't recover'") has no existing UI
  counterpart to extend; if the plan wants this surfaced to the user rather than just
  logged, it's new UI work, not a wiring fix — worth calling out explicitly as a scope
  decision, since it's not in the abbreviated acceptance criteria as a UI requirement, only
  a "log/observability" one.
