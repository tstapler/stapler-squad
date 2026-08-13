# Fix Session Rename: Duplication Bug

**Date**: 2026-04-13
**Type**: Bug fix
**Severity**: High -- data corruption (orphaned records accumulate in DB)
**Scope**: Backend storage layer, two RPC handlers

---

## Epic Overview

### Problem Statement

Renaming a session creates a duplicate record in the database instead of updating the
existing one. After a rename, both the old-title record and the new-title record exist
in the DB. The old record is orphaned (no in-memory Instance points to it) but persists
across restarts, appearing as a ghost session in the UI.

### Root Cause

The rename flow mutates `instance.Title` in memory **before** persisting, then calls
`SaveInstances` which delegates to `EntRepository.Update`. The `Update` method locates
the DB row by matching `data.Title` (the **new** title). That row does not exist yet,
so Update returns "not found". The `saveInstancesToRepo` fallback (line 184-188 of
`session/storage.go`) interprets any Update failure as "not found" and calls
`repo.Create` -- inserting a brand-new record with the new title. The old-title row
is never deleted.

**Affected code paths** (both exhibit the same bug):

1. `RenameSession` RPC (`server/services/session_service.go:1366`)
   - Calls `instance.Rename(newTitle)` which sets `instance.Title = newTitle`
   - Then calls `s.storage.SaveInstances(instances)` -- Update fails, Create succeeds

2. `UpdateSession` RPC (`server/services/session_service.go:609`)
   - Sets `instance.Title = *req.Msg.Title` directly (line 649)
   - Then calls `s.storage.SaveInstances(instances)` -- same failure cascade

**Why existing tests don't catch it**: The test `TestUpdateSession_HandlerOrdering_MetadataBeforeStatus`
(session_service_test.go:113) uses a paused session and reloads via `LoadInstances`, searching for
the new title. It finds the newly-created record and passes. It never checks that the old-title
record was removed, so the duplication is invisible.

### Solution

Add a `Rename(ctx, oldTitle, newTitle string) error` method to the Repository interface
that performs an atomic title update using the old title as the lookup key. Use this
method in both `RenameSession` and `UpdateSession` handlers instead of the
mutate-in-memory-then-save-all pattern.

### Success Criteria

- Renaming a session results in exactly one DB record with the new title
- The old-title record no longer exists after rename
- All other session data (path, branch, worktree, tags, status, timestamps) is unchanged
- Existing tests pass; new tests verify no duplication

---

## Architecture Decisions

### ADR: Atomic Rename via Repository Method

**Status**: Proposed

**Context**: The current Repository interface has no method that can update a record's
title (the unique key) because `Update` locates rows by `data.Title`. When the title
has already been changed in the InstanceData struct, Update cannot find the original row.

**Decision**: Add `Rename(ctx context.Context, oldTitle, newTitle string) error` to
the Repository interface. The EntRepository implementation executes:

```sql
UPDATE sessions SET title = ?, updated_at = ? WHERE title = ?
-- params: newTitle, now, oldTitle
```

This is a single atomic SQL statement. The RPC handlers call `repo.Rename` first,
then update the in-memory title, then call the normal `SaveInstances` path (which
will now find the record by the new title).

**Consequences**:
- Positive: Atomic, no window for duplication, no orphaned records
- Positive: Minimal change surface -- one new Repository method + two handler fixes
- Positive: Title uniqueness enforced by DB constraint (Unique on title field)
- Negative: Repository interface grows by one method (acceptable for a keyed-rename operation)

**Alternatives Considered**:

1. **Delete-then-Create in handler**: Call `repo.Delete(oldTitle)` then `repo.Create(newData)`.
   Rejected: non-atomic, loses child records (worktree, tags, diff_stats, claude_session
   edges) unless carefully re-created. Risk of partial failure leaving no record at all.

2. **Pass old title through InstanceData**: Add an `OriginalTitle` field to InstanceData
   so Update can use it for lookup. Rejected: pollutes the data model with persistence
   concerns, every caller must remember to set it.

3. **Fix saveInstancesToRepo to delete old record**: Track title changes in Instance.
   Rejected: requires diffing in-memory vs DB state for every save, complex and fragile.

---

## Story Breakdown

### Story 1: Add Repository.Rename Method

**As a** developer fixing the rename bug
**I want** the Repository interface to support atomic title updates
**So that** renaming doesn't create duplicate records

**Acceptance Criteria**:
- [ ] `Rename(ctx, oldTitle, newTitle string) error` added to Repository interface
- [ ] EntRepository implementation updates title + updated_at in a single transaction
- [ ] Returns error if oldTitle not found
- [ ] Returns error if newTitle already exists (unique constraint violation)
- [ ] Unit test: rename succeeds, old title gone, new title present
- [ ] Unit test: rename to existing title returns error
- [ ] Unit test: rename non-existent session returns error

### Story 2: Fix RenameSession RPC Handler

**As a** user renaming a session
**I want** the rename to update the existing record
**So that** I don't see duplicate sessions

**Acceptance Criteria**:
- [ ] RenameSession calls `repo.Rename(oldTitle, newTitle)` before mutating in-memory Instance
- [ ] In-memory Instance title updated only after DB rename succeeds
- [ ] SaveInstances call after rename finds the record by new title (no Create fallback)
- [ ] Event published with correct new title
- [ ] Integration test: rename then list shows exactly one session with new title

### Story 3: Fix UpdateSession Title-Change Path

**As a** user updating a session's title via UpdateSession
**I want** the title change to not create duplicates
**So that** my session list stays clean

**Acceptance Criteria**:
- [ ] When `req.Msg.Title` is set and differs from current title, use `repo.Rename`
- [ ] Existing UpdateSession behavior for non-title fields unchanged
- [ ] Integration test: UpdateSession with title change, verify no old record remains

---

## Task Breakdown

### Task 1.1: Add Rename to Repository Interface and EntRepository

**Files**: `session/repository.go`, `session/ent_repository.go`
**Estimated complexity**: Low

Add to Repository interface:
```go
// Rename atomically changes a session's title in storage.
// Returns error if oldTitle not found or newTitle already exists.
Rename(ctx context.Context, oldTitle, newTitle string) error
```

EntRepository implementation:
```go
func (r *EntRepository) Rename(ctx context.Context, oldTitle, newTitle string) error {
    tx, err := r.client.Tx(ctx)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }
    defer tx.Rollback()

    // Find session by old title
    sess, err := tx.Session.Query().Where(session.Title(oldTitle)).Only(ctx)
    if err != nil {
        return fmt.Errorf("session not found: %s: %w", oldTitle, err)
    }

    // Update title (unique constraint prevents conflicts)
    if err := tx.Session.UpdateOne(sess).
        SetTitle(newTitle).
        SetUpdatedAt(time.Now()).
        Exec(ctx); err != nil {
        return fmt.Errorf("failed to rename session from '%s' to '%s': %w", oldTitle, newTitle, err)
    }

    return tx.Commit()
}
```

**Boundary**: Only modifies repository interface and its implementation. No handler changes.

### Task 1.2: Add Repository Rename Tests

**Files**: `session/ent_repository_test.go`
**Estimated complexity**: Low

Test cases:
- `TestEntRepository_Rename_Success`: Create session "alpha", rename to "beta", verify
  "beta" exists with all original data, "alpha" does not exist
- `TestEntRepository_Rename_NotFound`: Rename non-existent title returns error
- `TestEntRepository_Rename_Conflict`: Create "alpha" and "beta", rename "alpha" to
  "beta" returns error, both records unchanged
- `TestEntRepository_Rename_PreservesChildData`: Create session with worktree + tags +
  claude_session, rename, verify all child records still linked to renamed session

**Boundary**: Only test files.

### Task 2.1: Fix RenameSession Handler

**Files**: `server/services/session_service.go`
**Estimated complexity**: Low

Change in `RenameSession` method (around line 1408-1420):

Before:
```go
oldTitle := instance.Title
if err := instance.Rename(req.Msg.NewTitle); err != nil {
    return nil, connect.NewError(connect.CodeInternal, ...)
}
instances[instanceIndex] = instance
if err := s.storage.SaveInstances(instances); err != nil {
    instance.Title = oldTitle  // rollback attempt
    return nil, ...
}
```

After:
```go
oldTitle := instance.Title

// Rename in DB first (atomic, uses old title as lookup key)
if err := s.storage.RenameInstance(oldTitle, req.Msg.NewTitle); err != nil {
    return nil, connect.NewError(connect.CodeInternal,
        fmt.Errorf("failed to rename session in storage: %w", err))
}

// Now update in-memory state (DB already has new title)
if err := instance.Rename(req.Msg.NewTitle); err != nil {
    // DB already renamed -- this should not fail for valid titles
    // (validation happened above). Log and continue.
    log.ErrorLog.Printf("in-memory rename failed after DB rename: %v", err)
}

// Save remaining instance state (SaveInstances will find by new title now)
instances[instanceIndex] = instance
if err := s.storage.SaveInstances(instances); err != nil {
    log.ErrorLog.Printf("failed to save instance after rename: %v", err)
    // Non-fatal: title is already correct in DB
}
```

**Boundary**: Single handler method in session_service.go.

### Task 2.2: Add Storage.RenameInstance Convenience Method

**Files**: `session/storage.go`
**Estimated complexity**: Trivial

```go
// RenameInstance atomically changes a session's title in the repository.
func (s *Storage) RenameInstance(oldTitle, newTitle string) error {
    return s.repo.Rename(context.Background(), oldTitle, newTitle)
}
```

Also add to InstanceStore interface if it needs to be mockable.

**Boundary**: Single method addition to storage.go.

### Task 2.3: Fix UpdateSession Title-Change Path

**Files**: `server/services/session_service.go`
**Estimated complexity**: Low

Change in `UpdateSession` method (around line 641-651):

Before:
```go
if req.Msg.Title != nil && *req.Msg.Title != "" && *req.Msg.Title != instance.Title {
    for _, inst := range instances {
        if inst.Title == *req.Msg.Title {
            return nil, connect.NewError(connect.CodeAlreadyExists, ...)
        }
    }
    instance.Title = *req.Msg.Title
    updatedFields = append(updatedFields, "title")
}
```

After:
```go
if req.Msg.Title != nil && *req.Msg.Title != "" && *req.Msg.Title != instance.Title {
    oldTitle := instance.Title
    if err := s.storage.RenameInstance(oldTitle, *req.Msg.Title); err != nil {
        return nil, connect.NewError(connect.CodeAlreadyExists,
            fmt.Errorf("failed to rename session: %w", err))
    }
    instance.Title = *req.Msg.Title
    updatedFields = append(updatedFields, "title")
}
```

The explicit in-memory duplicate check (loop over instances) can be removed since the
DB unique constraint now enforces this. However, keeping it provides a better error
message. Either approach is acceptable.

**Boundary**: Single handler method in session_service.go.

### Task 3.1: Integration Tests for Rename Deduplication

**Files**: `server/services/session_service_test.go`
**Estimated complexity**: Low

Test cases:
- `TestRenameSession_NoDuplicate`: Create "original", rename to "renamed", load all
  instances, assert exactly one instance exists and its title is "renamed"
- `TestRenameSession_PreservesData`: Create session with path/branch/tags, rename,
  verify all fields preserved
- `TestUpdateSession_TitleChange_NoDuplicate`: Same as above but via UpdateSession RPC
- `TestRenameSession_Conflict`: Create "a" and "b", rename "a" to "b", expect error,
  verify both sessions unchanged

Fix existing test `TestUpdateSession_HandlerOrdering_MetadataBeforeStatus` to also
assert the old title ("combo-session") no longer exists in storage.

**Boundary**: Only test files.

---

## Known Issues / Potential Bugs

### Bug 1: Orphaned Records From Past Renames [SEVERITY: Medium]

**Description**: Users who have already renamed sessions have orphaned old-title records
in their database. These appear as ghost sessions.

**Mitigation**: Consider a one-time migration or cleanup script that identifies sessions
with duplicate worktree paths (the worktree data is copied into both records) and removes
the record with the older `updated_at` timestamp. This is out of scope for this fix but
should be tracked as follow-up work.

**Files Affected**: None (data cleanup, not code)

### Bug 2: Race Condition Between Rename and Concurrent Save [SEVERITY: Low]

**Description**: If a background process (e.g., terminal timestamp updates, review queue
poller) calls `SaveInstances` or `UpdateInstanceTimestampsOnly` while a rename is in
progress, the old-title record could be re-created by the background save if it runs
between the DB rename and the in-memory title update.

**Mitigation**: The fix orders operations as DB-rename-first, then in-memory-update. The
window is small (microseconds). For a belt-and-suspenders approach, the
`UpdateInstanceTimestampsOnly` path uses `updateFieldInRepo` which does Get-by-title +
Update. If the title changed, Get will fail and the timestamp update is simply lost (not
harmful -- it will succeed on the next cycle). No additional locking needed.

**Prevention**: Ensure all storage write paths use the current in-memory title as the
lookup key, which is correct after the in-memory update completes.

### Bug 3: Search Index Stale After Rename [SEVERITY: Low]

**Description**: The search engine indexes sessions by title. After rename, the old title
may remain in the search index until the next sync cycle.

**Mitigation**: The `SessionUpdated` event (already published by both handlers) should
trigger a search index update. Verify that the event handler re-indexes the session with
the new title and removes the old entry. If not, the search index sync interval (default
5 minutes) will eventually correct it.

**Files Affected**: `server/services/search_service.go` (verify, likely no change needed)

---

## Dependency Visualization

```
Task 1.1: Repository.Rename ──┐
                               ├─→ Task 1.2: Repo Tests
                               │
Task 2.2: Storage.RenameInstance ◄─┘
    │
    ├─→ Task 2.1: Fix RenameSession handler
    │
    ├─→ Task 2.3: Fix UpdateSession handler
    │
    └─→ Task 3.1: Integration tests
```

Critical path: 1.1 -> 2.2 -> 2.1/2.3 -> 3.1

All tasks can be implemented in a single session. Total estimated scope: ~200 lines of
production code, ~150 lines of test code.

---

## Files Modified (Summary)

| File | Change |
|------|--------|
| `session/repository.go` | Add `Rename` to interface |
| `session/ent_repository.go` | Implement `Rename` method |
| `session/ent_repository_test.go` | Add rename test cases |
| `session/storage.go` | Add `RenameInstance` convenience method |
| `server/services/session_service.go` | Fix `RenameSession` and `UpdateSession` handlers |
| `server/services/session_service_test.go` | Add deduplication tests, fix existing test |
