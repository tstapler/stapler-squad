# ADR-002: Hibernation Checkpoint Storage Layout

**Date:** 2026-05-18  
**Status:** Accepted  
**Deciders:** Tyler Stapler  

---

## Context

When a session is hibernated (FR-4), two classes of data must survive the process kill so the session can be resumed (FR-6):

1. **Scrollback** — the terminal output buffer accumulated during the session's lifetime. This can be several megabytes for long-running sessions. `ScrollbackManager` already writes scrollback to disk under `~/.stapler-squad/` as part of its normal operation; the exact path is tracked by the session record.

2. **Checkpoint metadata** — small structured data needed to re-launch the session: the AI process command, working directory, environment variables, the reason for hibernation, timestamps, and a reference to the scrollback data.

Three storage questions must be answered:

### Question 1: Where do checkpoint files live?

Options:
- **A. Inline with session state** — store checkpoint data inside the existing session record / SQLite database
- **B. Separate checkpoint directory per session** — `~/.stapler-squad/checkpoints/<session-uuid>/`
- **C. Alongside the scrollback file** — add a `checkpoint.json` next to the existing `scrollback.txt` in whatever directory `ScrollbackManager` uses

### Question 2: How is scrollback handled in the checkpoint?

Options:
- **A. Copy on hibernate** — duplicate the scrollback file into the checkpoint directory
- **B. Reference only** — write the path of the existing `ScrollbackManager` scrollback file into `checkpoint.json`; do not copy

### Question 3: What is the checkpoint directory default and how is it configured?

---

## Decision

### 1. Checkpoint directory: `~/.stapler-squad/checkpoints/<session-uuid>/`

Each hibernated session gets its own subdirectory under a top-level `checkpoints/` directory inside the stapler-squad data root. The directory contains exactly two files:

```
~/.stapler-squad/checkpoints/
  <session-uuid>/
    checkpoint.json       ← metadata
    scrollback.txt        ← copy of scrollback at hibernate time (see §2)
```

The checkpoint directory is created atomically on hibernate and deleted when the session is deleted (FR-8).

### 2. Scrollback handling: copy at hibernate time

The scrollback file is **copied** into `<checkpoint_dir>/<session-uuid>/scrollback.txt` at the moment of hibernation, not referenced by path.

`checkpoint.json` does **not** store the original `ScrollbackManager` path; it always reads `scrollback.txt` from the same directory as itself.

### 3. Configuration

Default checkpoint root: `~/.stapler-squad/checkpoints/` (derived from the existing data root, same as `worktrees/` and `logs/`).

Config fields added to `config.json` under a `hibernation` key:

```json
{
  "hibernation": {
    "enabled": true,
    "checkpoint_dir": "",
    "idle_timeout_minutes": 120,
    "resource_pressure_threshold_pct": 85
  }
}
```

`checkpoint_dir` defaults to `<data_root>/checkpoints/` when empty.

### checkpoint.json schema

```json
{
  "schema_version": 1,
  "session_id": "<uuid>",
  "hibernated_at": "<RFC3339>",
  "hibernate_reason": "idle_timeout | resource_pressure | manual",
  "ai_command": ["claude", "--resume", "..."],
  "working_directory": "/home/user/project",
  "tmux_session_name": "stapler-<uuid>",
  "scrollback_lines": 4821,
  "scrollback_file": "scrollback.txt"
}
```

`scrollback_file` is always a relative path within the checkpoint directory; it exists so the resume path can locate the file without hardcoding the name.

---

## Rationale

### Why a separate checkpoint directory (Option B) over inline (A) or alongside scrollback (C)?

**Against inline/SQLite (A):** Scrollback can be megabytes. Storing it in SQLite rows would balloon the database, complicate backup, and make it impossible to inspect or manually recover a checkpoint without a database client. Structured binary blobs in SQLite are also awkward to stream.

**Against alongside scrollback (C):** `ScrollbackManager` may rotate, compress, or relocate scrollback files as part of its own lifecycle. Coupling checkpoint metadata to that path creates an implicit dependency between two subsystems that should be independent. A dedicated `checkpoints/` directory is a clean, self-contained unit.

**For separate directory (B):** Each `<session-uuid>/` subdirectory is a self-describing, portable unit. It can be archived, inspected, or manually restored without any database access. The layout mirrors the `worktrees/` convention already in use.

### Why copy scrollback rather than reference it?

The original `ScrollbackManager` path is owned by the scrollback subsystem, which may:
- Rotate or compress the file after a configurable size limit
- Delete it when the session is deleted (or cleaned up by a separate GC)
- Change its path if the data root is reconfigured

A reference-only approach makes the checkpoint fragile: any of the above events silently breaks resume. The scrollback file is the primary user-visible artifact of a hibernated session (the "show me where it was" experience on resume); it must be reliably present.

The copy is bounded in size. The scrollback file is already on disk; copying it is a sequential file operation that completes in milliseconds for typical session lengths. The cost is acceptable relative to the reliability gain.

### Why not store the full AI process state (CRIU)?

Full process checkpointing (CRIU) is listed as a Non-Goal in the requirements. The scrollback + metadata approach is the minimal viable checkpoint needed for seamless resume: the AI process is re-launched fresh, and the saved scrollback is loaded into the terminal buffer so the user sees context.

---

## Consequences

### Staleness risk: scrollback copied at hibernate time is a snapshot

The copied `scrollback.txt` reflects terminal output up to the moment of hibernation. If the session had any uncommitted terminal output that `ScrollbackManager` had not yet flushed to disk, it will be missing from the checkpoint. Mitigation: call `ScrollbackManager.Flush()` as the first step of the hibernate sequence before copying.

### Checkpoint orphan risk: checkpoint exists without a session record

If a session record is deleted from the database but the checkpoint directory is not cleaned up (e.g., crash between the two operations), the checkpoint directory becomes an orphan. Mitigation:

1. Deletion of a session must atomically remove the checkpoint directory as part of the same transaction (or as a post-commit hook that is safe to retry)
2. A startup sweep should log any `checkpoints/<uuid>/` directories whose `uuid` does not correspond to a known session, and optionally prune them after a configurable retention period (FR-8)

### Checkpoint becomes stale if the working directory is deleted

The `working_directory` stored in `checkpoint.json` may no longer exist if the user manually removes the worktree. Resume must validate that the working directory exists before relaunching the AI process, and surface a clear error (not a silent failure) if it does not.

### Resume reads scrollback from checkpoint, not from ScrollbackManager

On resume, the terminal buffer is populated from `<checkpoint_dir>/<session-uuid>/scrollback.txt`. `ScrollbackManager` is re-initialized for the new process run with a fresh scrollback file. The checkpoint scrollback is read-only and is not appended to by the resumed session. After successful resume, the checkpoint directory can be deleted (or retained for a configurable period for debugging).

### Config migration

Existing installations without a `hibernation` key in `config.json` must be handled gracefully. The config loader should populate defaults for all `hibernation` fields when the key is absent, following the pattern used by other optional config sections in `config/config.go`.
