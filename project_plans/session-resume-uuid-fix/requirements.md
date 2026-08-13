# Requirements: Session Resume UUID Fix

## Problem Statement

When a Stapler Squad session is paused and other sessions subsequently run in the same
working directory, resuming the paused session picks up the wrong Claude conversation
history. Instead of resuming the specific conversation that was active when the session
was paused, it resumes the most recent conversation for that directory.

This is especially painful for non-worktree sessions (e.g., a personal wiki or any
directory-mode session), where multiple Claude sessions can run in the same directory
over time.

## Root Cause (Identified)

The bug lives in `session/history_linker.go:correlateSession()`.

When any JSONL history file changes (fsnotify fires) or on startup scan, `ScanAll()`
calls `correlateSession(inst, force=true)` for ALL sessions, including paused ones.

For a paused session that is already linked (has a stored conversation UUID):
1. `GetPanePID()` fails — the session is paused, no live tmux process.
2. The code falls through to the path-based fallback: `DetectByPath(effectivePath)`.
3. `DetectByPath` returns the **most recently modified** JSONL file in the project dir.
4. If another session ran in the same directory after the pause, that session's JSONL
   is newer and gets returned.
5. `SetHistoryInfo(wrongUUID, wrongPath)` overwrites the correct stored UUID.
6. On resume, `buildLaunchCommand(wrongUUID)` generates `claude --resume wrong-uuid`.

Both trigger paths for this bug:
- **During a session's lifetime**: fsnotify fires when sessions B/C/D create JSONL files
  in the same directory → paused session A's UUID gets overwritten.
- **On server restart**: `HistoryLinker.Start()` runs `ScanAll()` → same overwrite.

## Requirements

### R1: UUID Stability for Non-Running Sessions (Must Have)

A paused or hibernated session's stored conversation UUID MUST NOT be overwritten by
the path-based `DetectByPath` fallback. The stored UUID is the authoritative source of
truth for sessions that are not actively running.

**Acceptance criteria:**
- Pause session A in directory `/foo/` with UUID `uuid-A`.
- Run sessions B and C in `/foo/`, creating newer JSONL files.
- Resume session A. It MUST launch with `--resume uuid-A`, not `uuid-B` or `uuid-C`.

### R2: Live Session UUID Updates Still Work (Must Have)

The existing behavior of updating a running session's UUID when the user runs `/clear`
in Claude MUST continue to work. When Claude creates a new conversation (new JSONL
file), the HistoryLinker should detect it via PID-based inspection and update the UUID.

**Acceptance criteria:**
- Start session A, Claude opens `uuid-A.jsonl`.
- User runs `/clear` in Claude. Claude opens `uuid-new.jsonl`.
- HistoryLinker updates session A's UUID to `uuid-new` via PID-based detection.

### R3: Cold Restore via Path Fallback Still Works for Unlinked Sessions (Must Have)

Sessions that have NO stored UUID (newly created, or UUID was cleared) MUST still use
the `DetectByPath` fallback to find their history file. This handles fresh sessions
and cold restores after the UUID was intentionally cleared.

**Acceptance criteria:**
- New session in `/foo/` with no UUID.
- JSONL file exists for that path.
- HistoryLinker links the session via path fallback.

### R4: Server Restart Preserves Correct UUID for Paused Sessions (Must Have)

After a server restart, a paused session with a stored UUID from sessions.json MUST
keep that UUID even if newer JSONL files exist in the same directory.

**Acceptance criteria:**
- Pause session A with UUID `uuid-A`.
- Restart the server.
- Sessions B and C have run in the same directory (newer JSONL files).
- Session A still has UUID `uuid-A` after the startup scan.

## Scope

- All session types: directory-mode and worktree sessions.
- Both trigger paths: fsnotify-driven `ScanAll()` and polling.

## Out of Scope

- Handling the case where a paused session has NO stored UUID and multiple sessions
  have run in the same directory (this is a pre-existing limitation, best effort only).
- Changing how `tryExtractConversationUUID` works in `instance_claude.go` (already
  has the correct guard: skips if UUID is already set).

## Fix Design

In `session/history_linker.go`, `correlateSession()`: gate the path-based fallback
so it only runs when either (a) the session is not yet linked, OR (b) the session
has a live process (PID lookup succeeded). Skip the path fallback for already-linked
sessions with no live process.

Condition change:
```
// Before (buggy):
if info == nil {
    ... DetectByPath() ...
}

// After (correct):
if info == nil && (!alreadyLinked || pidErr == nil) {
    ... DetectByPath() ...
}
```

A corresponding unit test in `history_linker_test.go` should cover:
- Already-linked paused session is not overwritten by path fallback when PID fails.
- Already-linked active session IS updated when PID detection finds a new UUID.
- Unlinked session IS linked via path fallback regardless of PID state.
