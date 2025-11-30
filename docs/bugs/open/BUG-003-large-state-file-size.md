# BUG-003: Large State File Size (34MB JSON) [SEVERITY: Low]

**Status**: Open (Discovered 2025-01-17)
**Discovered**: During persistence architecture review
**Impact**: Large memory footprint, slow load/save operations, potential for file corruption

## Problem Description

The application state file (`~/.claude-squad/state.json`) has grown to **34MB** (34,476,797 bytes) without schema enforcement, indexing, or efficient querying. This indicates either:
1. Accumulation of many sessions over time (normal growth)
2. Inefficient serialization (e.g., embedded terminal content, large diffs)
3. Lack of archival/cleanup mechanism for old sessions

While not critical, this represents a **scalability concern** and **code smell** suggesting the JSON persistence approach is reaching its practical limits.

## Reproduction

```bash
# Check current state file size
ls -lh ~/.claude-squad/state.json

# Output (example from user's system)
-rw-r--r--@ 1 user  staff    33M Oct  3 09:19 /Users/user/.claude-squad/state.json
```

**Expected**: State file should be <5MB for typical usage (50-100 sessions)
**Actual**: 34MB file size suggests either many sessions or inefficient data storage

## Root Cause Analysis

### JSON Storage Characteristics

**Current persistence** (`config/state.go`, `session/storage.go`):
- Single monolithic JSON file containing all session data
- No compression, indexing, or query optimization
- Entire file loaded into memory on startup
- Entire file rewritten on each save operation

**Growth factors**:
1. **Session count**: Each session ~5-10KB serialized → 34MB ≈ 3,400-6,800 sessions (unlikely)
2. **Embedded data**: ClaudeSession, DiffStats, terminal content may be large
3. **No archival**: Paused/stopped sessions never removed
4. **Duplicate data**: Multiple instances may share repository/worktree data

### Performance Implications

**Load time** (estimated):
- 34MB JSON parse: ~200-500ms (depending on session count)
- Memory footprint: 34MB + parsed object overhead = ~50-70MB RAM
- Impact: Noticeable startup delay, high memory usage

**Save time** (estimated):
- 34MB JSON marshal + write: ~150-300ms
- With file locking and atomic write: ~300-500ms
- Impact: UI stuttering during saves (even with async worker)

**Query performance**:
- Load all sessions, filter in memory: ~50-100ms
- No indexes: Every query scans all sessions
- Impact: Slow filtering, search, bulk operations

## Files Affected (3+ files - Context boundary: Analysis only)

1. **~/.claude-squad/state.json** (34MB) - Data file
2. **config/state.go** (~550 lines) - Persistence implementation
3. **session/storage.go** (328 lines) - Storage API
4. **session/instance.go** (multiple structs) - Serialization logic

## Investigation Required

Before determining fix approach, need to **analyze state file contents**:

### Investigation Steps

1. **Count sessions**:
   ```bash
   # Count top-level instances in JSON array
   jq '. | length' ~/.claude-squad/state.json
   ```

2. **Analyze average session size**:
   ```bash
   # Calculate average bytes per session
   jq '. | length as $count | (input_filename | sub(".*/";"") | .[:-5] | tonumber) / $count' ~/.claude-squad/state.json
   ```

3. **Identify large fields**:
   ```bash
   # Find largest fields in a sample session
   jq '.[0] | to_entries | map({key: .key, size: (.value | tostring | length)}) | sort_by(.size) | reverse' ~/.claude-squad/state.json
   ```

4. **Check for embedded content**:
   ```bash
   # Look for large ClaudeSession or DiffStats fields
   jq '[.[] | {title, claude_size: (.claude_session | tostring | length), diff_size: (.diff_stats | tostring | length)}] | sort_by(.claude_size) | reverse | .[0:10]' ~/.claude-squad/state.json
   ```

### Expected Findings

**Likely scenario 1**: Many sessions accumulated over time
- **Count**: 500-1000+ sessions in state file
- **Fix**: Implement archival/cleanup for old/stopped sessions
- **Priority**: Medium

**Likely scenario 2**: Inefficient serialization (embedded content)
- **Large fields**: ClaudeSession contains full message history, DiffStats contains full diffs
- **Fix**: Store only metadata in state.json, persist large data separately
- **Priority**: Medium-High

**Likely scenario 3**: Duplicate/redundant data
- **Pattern**: Multiple sessions share repository/worktree data stored redundantly
- **Fix**: Normalize data, store shared data once with references
- **Priority**: Low-Medium

## Fix Approaches (Deferred Until Investigation Complete)

### Option 1: Session Archival (If many old sessions)

**Implement cleanup policy**:
- Archive sessions stopped/paused for >30 days
- Move to `~/.claude-squad/archive/` directory
- Add "View Archive" feature for historical browsing

**Estimated effort**: 3-4 hours (2 files: storage.go, archive.go)
**Impact**: Reduces active state file to recent sessions only

### Option 2: Separate Large Data (If inefficient serialization)

**Split persistence**:
- `state.json`: Core session metadata only (~100-200KB)
- `sessions/{id}/claude.json`: Claude message history per session
- `sessions/{id}/diffs.json`: Diff data per session

**Estimated effort**: 6-8 hours (refactor storage layer)
**Impact**: Dramatically reduces main state file size, enables lazy loading

### Option 3: Compress State File (Quick win)

**Add gzip compression**:
- Compress on save, decompress on load
- Typical 70-80% size reduction (34MB → 7-10MB)
- Minimal code changes (~2 hours, 1 file)

**Estimated effort**: 2 hours (1 file: state.go)
**Impact**: Reduces disk usage and load time, doesn't address in-memory size

### Option 4: Migrate to SQLite (Long-term solution)

**Full migration to SQLite**:
- Structured schema with indexes
- Query optimization and transactions
- Incremental loading and lazy fetching
- Backup/restore and migration support

**Estimated effort**: 8-12 hours (see BUG-003 related task doc)
**Impact**: Solves all scalability concerns, enables advanced features

**Note**: See `docs/tasks/repository-pattern-sqlite-migration.md` for detailed SQLite migration plan. However, this is **deferred per existing analysis** until Web UI MVP complete.

## Recommended Action

**Immediate**: **Investigation phase** (1 hour, no code changes)
1. Run investigation steps above to understand state file composition
2. Determine which scenario applies (session count vs embedded data)
3. Document findings in this bug report
4. Decide on appropriate fix approach based on findings

**Short-term**: Apply **targeted fix** based on investigation findings (2-4 hours)
- If many sessions → Implement archival (Option 1)
- If large embedded data → Separate large data (Option 2) or compress (Option 3)
- Defer SQLite migration (Option 4) until Web UI MVP complete

**Long-term**: Consider SQLite migration when:
- Session count regularly exceeds 500+
- Query performance becomes bottleneck
- Advanced features require SQL capabilities

## Impact Assessment

**Severity**: **Low** (not Medium or High)
- **User-Facing**: Indirect - slower startup, higher memory usage
- **Data Loss**: No - current implementation works correctly
- **Workaround**: Manual cleanup of state.json, restart app periodically
- **Frequency**: Grows over time, not immediate issue
- **Scope**: Affects all users with long-term usage patterns

**Priority**: P3 - Future optimization after Web UI MVP and critical bugs (BUG-001, BUG-002) are resolved.

**Recommended Timeline**:
1. Investigation (1h) - After BUG-001/002 fixes complete
2. Targeted fix (2-4h) - Based on investigation findings
3. SQLite migration (8-12h) - Only if investigation reveals fundamental scalability limits

## Prevention Strategy

**Short-term**:
1. Add monitoring: Log state file size on startup/save
2. Add warnings: Warn users when state file exceeds 10MB
3. Add documentation: Guide users to clean up old sessions

**Long-term**:
1. Implement session lifecycle management (auto-archive old sessions)
2. Consider SQLite migration for structured querying and indexing
3. Add telemetry: Track average session count and state file growth patterns

## Related Issues

- **SQLite Migration Task**: See `docs/tasks/repository-pattern-sqlite-migration.md` for comprehensive migration plan (deferred to P3)
- **Schema Enforcement**: Current JSON format has no validation (related to BUG-001/002)
- **Performance**: Large file affects startup time and memory usage

## Additional Notes

- 34MB is unusually large for typical usage patterns (50-100 active sessions)
- Suggests either:
  1. User has accumulated hundreds/thousands of sessions over time (power user)
  2. Serialization includes large embedded data (ClaudeSession message history, full diffs)
  3. Both factors contributing to size
- Investigation required before committing to fix approach
- Not blocking any current work, but should be addressed before production release

## Investigation Results (Completed 2025-11-30)

**Date Investigated**: 2025-11-30
**Session Count**: 40 sessions
**Average Session Size**: 840,867 bytes (821.2 KB per session)
**File Size**: 34,476,797 bytes (33.6 MB total)
**Largest Fields**: `diff_stats.content` (stores full git diffs)
**Largest Single Diff**: 26,294,403 bytes (25.7 MB) in session "load-restrictions"

### Root Cause Identified

**PRIMARY CULPRIT**: The `diff_stats.content` field stores **complete git diff output** for every session.

**Analysis**:
- Total `diff_stats` size across 40 sessions: 33,634,676 bytes (32.8 MB)
- This accounts for **97.6%** of the total file size
- One session alone contains a 25.7 MB diff (likely a large file or binary data)
- Average diff size: 821 KB per session (far too large for metadata)

**Example from state.json**:
```json
"diff_stats": {
  "added": 224,
  "removed": 26,
  "content": "diff --git a/.claude/settings.local.json b/.claude/settings.local.json\n..."
}
```

The `content` field stores the **entire diff output** including:
- All changed file contents
- Binary file differences
- Large generated files (builds, lockfiles, etc.)

### Recommended Fix: Option 2 (Separate Large Data)

**Approach**: Stop storing `diff_stats.content` in state.json

**Implementation Strategy**:
1. **Remove `diff_stats.content` from serialization** (1 hour, 2 files)
   - Keep `added` and `removed` counts in state.json for metadata
   - Remove the `content` field from `InstanceData` struct
   - Generate diffs on-demand when needed (via `GetSessionDiff` RPC)

2. **Update diff generation to be on-demand** (1 hour, 2 files)
   - Compute diff when user requests it (Web UI, TUI preview)
   - Cache in memory if needed for performance
   - Never persist full diff content to disk

**Files to Modify**:
- `session/storage.go` - Remove `content` from `DiffStats` serialization
- `session/instance.go` - Update `ToInstanceData()` to exclude diff content
- `server/services/session_service.go` - Ensure `GetSessionDiff` computes on-demand
- `proto/session/v1/session.proto` - Update `DiffStats` message (if needed)

**Expected Impact**:
- State file size: 34 MB → ~800 KB (42x reduction!)
- Load time: ~500ms → <50ms
- Memory footprint: ~70 MB → ~5 MB
- Save time: ~500ms → <50ms

**Estimated Effort**: 2-3 hours (4 files, fits context boundary)
**Risk**: Low - diff content not needed for session restoration
**Backward Compatibility**: Old state files will load correctly (content field ignored if present)

---

**Bug Tracking ID**: BUG-003
**Related Feature**: Persistence Layer (session/storage.go, config/state.go)
**Fix Complexity**: Variable (depends on investigation findings)
**Fix Risk**: Low-Medium (depends on chosen approach)
**Blocked By**: Investigation required (1 hour, no code changes)
**Blocks**: None
**Related To**: SQLite migration task (P3, deferred)
