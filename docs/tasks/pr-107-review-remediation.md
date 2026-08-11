# PR #107 Review Remediation

**Branch**: stapler-squad-perf
**Source**: Code review findings — Performance optimizations (go-git VCS, sync.Pool, lazy charts, a11y)

---

## Epic Overview

Resolve all confirmed code review findings from PR #107 before merge. 8 major findings (2 correctness bugs, 1 security/logic bug, 5 missing tests) and 4 nits. All findings are in 3 files.

**Success**: All MAJOR findings fixed and verified; all NITs addressed or explicitly deferred.

---

## Story 1: gogit VCS reader correctness and test coverage

**Files**: `session/unfinished/gogit_vcs_reader.go`, `session/unfinished/vcsreader_test.go`
**Objective**: Fix two silent-data-corruption bugs and add test coverage for three untested code paths.

---

### Task 1.1: Fix BFS/DFS bug in findMergeBase (1h)

**Scope**: Replace LIFO pop with FIFO dequeue in the second walk of `findMergeBase`.

**Files**:
- `session/unfinished/gogit_vcs_reader.go` (modify)
- `session/unfinished/vcsreader_test.go` (modify — add regression test)

**Location**: `gogit_vcs_reader.go` lines 546-548 (second walk, inside `// Walk from h2` block)

**Bug**: Both walks use `h := q[len(q)-1]; q = q[:len(q)-1]` (stack/DFS pop). The second walk must be BFS (queue/FIFO) to guarantee the *nearest* common ancestor is returned first. On merge-heavy histories, DFS returns a deeper ancestor, causing incorrect ahead/behind counts and false "no changes" results.

**Fix**:
```go
// BEFORE (lines 547-548):
h := q[len(q)-1]
q = q[:len(q)-1]

// AFTER:
h := q[0]
q = q[1:]
```

Note: The first walk (marking all ancestors of h1) does not require ordering guarantees — it only builds a set. Only the second walk needs FIFO to find the *nearest* ancestor.

**Success Criteria**:
- Both lines changed to FIFO dequeue
- New test `TestFindMergeBase_merge_heavy_history` creates a diamond merge graph (A -> B -> C -> merge commit, A -> D -> merge commit) and asserts `findMergeBase` returns C or D, not A
- `go test ./session/unfinished/... -run TestFindMergeBase` passes

**Testing**: New table-driven test covering linear history (baseline), diamond merge (regression for this bug), and octopus merge if feasible.

**Dependencies**: none

**Status**: Pending

---

### Task 1.2: Fix int32 truncation of file size (1h)

**Scope**: Replace the `uint32` cast on `info.Size()` at line 196 with an `int64` comparison.

**Files**:
- `session/unfinished/gogit_vcs_reader.go` (modify)
- `session/unfinished/vcsreader_test.go` (modify — add test note; actual >2GB file testing is impractical but document the fix)

**Location**: `gogit_vcs_reader.go` line 196

**Bug**: `uint32(info.Size())` silently wraps for files larger than 4,294,967,295 bytes (~4GB). The `entry.Size` field in go-git's index is `uint32`, so for files >4GB the on-disk size wraps to an identical value, producing a false cache hit ("no changes detected").

**Fix**:
```go
// BEFORE (line 196):
if uint32(info.Size()) != entry.Size ||

// AFTER:
if info.Size() != int64(entry.Size) ||
```

Check `entry.Size` type in go-git (`object.IndexEntry.Size` is `uint32`). The cast direction matters: `int64(entry.Size)` is safe (uint32 fits in int64); `uint32(info.Size())` is not.

**Success Criteria**:
- Line 196 uses `info.Size() != int64(entry.Size)` (or equivalent widening cast)
- `go build ./session/unfinished/...` passes with no type errors
- Existing `HasUncommitted` tests still pass

**Testing**: Add a comment in the test file explaining why a >4GB file test is omitted (impractical in CI) but referencing the fix.

**Dependencies**: none

**Status**: Pending

---

### Task 1.3: Add tests for staged-change, staged-deletion, and merge-conflict paths (2h)

**Scope**: Write three new test cases for `HasUncommitted` covering the three zero-coverage paths identified in M3, M4, M5.

**Files**:
- `session/unfinished/vcsreader_test.go` (modify)
- `session/unfinished/gogit_vcs_reader.go` (read-only reference)

**Existing test structure**: `TestHasUncommitted` in `vcsreader_test.go` uses `t.Run` sub-tests with a real `go-git` in-memory repo. Follow the same pattern.

**Three paths to cover**:

1. **Staged change (M3)** — index vs HEAD comparison path:
   - Create a commit, then `git add` a modified file without committing
   - Assert `HasUncommitted` returns `true`
   - Test name: `HasUncommitted_true_when_file_staged_but_not_committed`

2. **Staged deletion (M4)** — index scan for staged deletions:
   - Create a commit with file F, then `git rm --cached F` (remove from index, keep working tree)
   - Assert `HasUncommitted` returns `true`
   - Test name: `HasUncommitted_true_when_file_deleted_from_index`

3. **Merge conflict (M5)** — conflict markers in index:
   - Create a merge conflict state using go-git (two branches modify the same line, attempt merge)
   - Assert `HasUncommitted` returns `true`
   - Test name: `HasUncommitted_true_when_merge_conflict_in_index`
   - If go-git in-memory repos cannot represent conflict state, write the test using a real on-disk temp repo and document the limitation.

**Success Criteria**:
- All three `t.Run` blocks exist and pass
- `go test ./session/unfinished/... -run TestHasUncommitted` exits 0
- No test relies on `t.Skip` for the happy path — skip only if environment lacks git binary for the conflict test

**Testing**: Self-contained — these are the tests.

**Dependencies**: none (can run in parallel with 1.1 and 1.2)

**Status**: Pending

---

## Story 2: approval_handler.go security and logic fixes

**Files**: `server/services/approval_handler.go`, `server/services/approval_handler_integration_test.go`
**Objective**: Fix two logic/security bugs and add test coverage for the untested tmux-name matching branch.

---

### Task 2.1: Fix resolveSessionID swallowing storage error (1h)

**Scope**: Change the `ListInstanceData` error branch at line 437-438 to fall back to `headerVal` rather than returning empty string.

**Files**:
- `server/services/approval_handler.go` (modify)

**Location**: `approval_handler.go` lines 436-438

**Bug**:
```go
instances, err := h.storage.ListInstanceData()
if err != nil {
    return ""   // BUG: returns "" instead of original headerVal
}
```

When storage fails (DB locked, IO error), `resolveSessionID` returns `""`. The caller at line 145 uses this as the session identifier, so a transient storage error silently loses the session context for the approval request. The correct behavior is to fall back to `headerVal` — let the caller use the raw header value rather than nothing.

**Fix**:
```go
if err != nil {
    return headerVal, nil  // fall back to raw header on storage error
}
```

If `resolveSessionID` signature is `string` (not `(string, error)`), keep the single-return form:
```go
if err != nil {
    return headerVal
}
```

**Success Criteria**:
- On `ListInstanceData` error, function returns `headerVal` (the original raw header value), not `""`
- Existing integration tests pass
- New test `TestResolveSessionID_falls_back_to_header_on_storage_error` passes: inject a storage that returns an error, call `resolveSessionID("some-uuid", "")`, assert result is `"some-uuid"`

**Testing**: Add one test case to `approval_handler_integration_test.go` using the existing test harness pattern.

**Dependencies**: none

**Status**: Pending

---

### Task 2.2: Fix matchesIDData IDOR risk — require UUID when TmuxPrefix is empty (2h)

**Scope**: Tighten `matchesIDData` so the tmux-name match path requires a UUID component when `TmuxPrefix` is empty, aligning with `MatchesID` semantics.

**Files**:
- `server/services/approval_handler.go` (modify)
- `server/services/approval_handler_integration_test.go` (modify — add IDOR test)

**Location**: `approval_handler.go` lines 474-481

**Bug**: When `d.TmuxPrefix == ""`, the expression `d.TmuxPrefix + title == id` reduces to `title == id`, which matches on sanitized title alone with no UUID check. The canonical `MatchesID` method on `Instance` does not have this path — it always requires either a UUID or a full `prefix+title` match. A crafted tmux session name equal to another session's sanitized title would match the wrong session.

**Current code**:
```go
func matchesIDData(d session.InstanceData, id string) bool {
    if d.Title == id || stableIDForData(d) == id {
        return true
    }
    title := strings.Join(strings.Fields(d.Title), "")
    title = strings.NewReplacer(".", "_", ":", "_").Replace(title)
    return d.TmuxPrefix+title == id   // BUG: when TmuxPrefix=="", matches by title alone
}
```

**Fix**: Guard the tmux-name branch to only execute when `TmuxPrefix` is non-empty:
```go
func matchesIDData(d session.InstanceData, id string) bool {
    if d.Title == id || stableIDForData(d) == id {
        return true
    }
    if d.TmuxPrefix == "" {
        return false  // no prefix = no tmux-name match; avoids title-only collision
    }
    title := strings.Join(strings.Fields(d.Title), "")
    title = strings.NewReplacer(".", "_", ":", "_").Replace(title)
    return d.TmuxPrefix+title == id
}
```

Verify against `Instance.MatchesID` in `session/instance.go` to confirm semantic parity.

**Success Criteria**:
- `matchesIDData` with empty `TmuxPrefix` does not match on sanitized title alone
- New test `TestMatchesIDData_no_match_when_tmux_prefix_empty_and_only_title_matches` passes: construct `InstanceData{Title: "myproject", UUID: "uuid-abc", TmuxPrefix: ""}`, call `matchesIDData(d, "myproject")` only via the non-title path — confirm it does not return true when called with a crafted id equal to sanitized title
- New test `TestMatchesIDData_tmux_name_branch_with_prefix` passes: `InstanceData{TmuxPrefix: "sq_", Title: "my project"}` matches id `"sq_myproject"`
- Existing integration tests pass

**Testing**: Add both tests to `approval_handler_integration_test.go`. This also covers M8 (tmux-name branch test).

**Dependencies**: none

**Status**: Pending

---

## Story 3: sync.Pool nits in tmux.go

**Files**: `session/tmux/tmux.go`
**Objective**: Two low-risk clarity improvements — a named constant and an ordering comment.

These nits can be combined into a single commit.

---

### Task 3.1: Add named constant and ordering comment to sanitizerPool (1h)

**Scope**: Replace magic number `4096` with a named constant; add a comment explaining the `result.String()` / `defer sanitizerPool.Put(result)` ordering invariant.

**Files**:
- `session/tmux/tmux.go` (modify)

**Location**: Lines 1912-1925

**Change 1 — Named constant (N2)**:
```go
// BEFORE (line 1914):
b.Grow(4096)

// AFTER: add near top of file or adjacent to sanitizerPool:
// sanitizerInitialCap is the pre-grown buffer size for sanitizeUTF8String.
// Chosen to cover typical pane content (a few screenfuls) without reallocation.
const sanitizerInitialCap = 4096

// In New func:
b.Grow(sanitizerInitialCap)
```

**Change 2 — Ordering comment (N1)**:
```go
// BEFORE (line 1925):
defer sanitizerPool.Put(result)

// AFTER:
// result.String() is called before this defer fires; the string is copied out
// of the builder before the builder is returned to the pool and Reset()d.
defer sanitizerPool.Put(result)
```

Confirm `result.String()` is indeed called (implicitly or explicitly) before the function returns, so the comment is accurate. Scan the rest of `sanitizeUTF8String` for the return statement.

**Success Criteria**:
- `sanitizerInitialCap` constant defined and used in `b.Grow`
- Comment above `defer sanitizerPool.Put(result)` explains the ordering invariant
- `go build ./session/tmux/...` passes
- `go test ./session/tmux/...` passes

**Testing**: No new tests required; this is a clarity change. Build verification is sufficient.

**Dependencies**: none

**Status**: Pending

---

## Nit N3: AheadBehind test behind-value assertion (deferred)

**Finding**: Tests for `AheadBehind` assert the "ahead" return value but do not explicitly assert the "behind" value.
**File**: `session/unfinished/vcsreader_test.go`
**Recommendation**: When touching `vcsreader_test.go` for Task 1.1 or 1.3, add explicit `behind` assertions to existing `AheadBehind` test cases. Since Task 1.1 already adds a merge-base test, bundle this assertion improvement there — no separate task needed.

---

## Nit N4: stableIDForData / matchesIDData as methods on session.InstanceData (deferred)

**Finding**: These functions logically belong as methods on `session.InstanceData`; currently kept as package-level functions in the server layer due to import graph constraints.
**File**: `server/services/approval_handler.go` lines 463-481
**Recommendation**: Defer to a follow-up refactor when the import graph permits. No action in this PR. Add a `// TODO: move to session.InstanceData when import cycle resolved` comment above `stableIDForData`.

---

## Dependency Visualization

```
Story 1 (gogit)
  Task 1.1 BFS fix (1h)         -- parallel
  Task 1.2 int32 fix (1h)       -- parallel
  Task 1.3 missing tests (2h)   -- parallel

Story 2 (approval_handler)
  Task 2.1 storage error (1h)   -- parallel
  Task 2.2 IDOR + tests (2h)    -- parallel (covers M7 + M8)

Story 3 (tmux nits)
  Task 3.1 constant + comment (1h)  -- parallel, lowest priority
```

All tasks are independent. No sequential dependencies within or across stories. Suggested execution order by risk: 2.2 (security) -> 1.1 (correctness) -> 1.2 (correctness) -> 2.1 (reliability) -> 1.3 (coverage) -> 3.1 (nits).

---

## Progress

| Task | Finding(s) | Size | Status |
|------|-----------|------|--------|
| 1.1 BFS fix in findMergeBase | M1 | 1h | Pending |
| 1.2 int32 truncation fix | M2 | 1h | Pending |
| 1.3 Staged/conflict test coverage | M3, M4, M5 | 2h | Pending |
| 2.1 resolveSessionID storage error fallback | M6 | 1h | Pending |
| 2.2 matchesIDData IDOR fix + tmux branch test | M7, M8 | 2h | Pending |
| 3.1 sanitizerPool constant + comment | N1, N2 | 1h | Pending |
| N3 AheadBehind behind assertion | N3 | — | Bundle with 1.1/1.3 |
| N4 method refactor TODO comment | N4 | — | Add comment in 2.2 |

**Total estimated effort**: 8h (all major findings)
**All MAJOR findings**: 0/8 resolved
**All NIT findings**: 0/4 resolved
