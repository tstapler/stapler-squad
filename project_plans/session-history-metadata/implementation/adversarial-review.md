# Adversarial Review: session-history-metadata (Pass 2 — Post-Patch)

**Date**: 2026-06-22
**Verdict**: CONDITIONAL PASS — 0 blockers, 3 concerns, 5 minors

---

## Original Blockers — Status

### Blocker 1: `storage.GetInstances()` does not exist
**Status: RESOLVED**

Task 3.1.1b now explicitly states: "NOTE: uses `instances` slice already in scope in `BuildDependencies`, NOT `storage.GetInstances()` which does not exist." The `lookupTitle` closure closes over the `instances []*session.Instance` slice already present in scope. All seven prior references to `storage.GetInstances()` are gone. The fix is architecturally sound.

### Blocker 2: `inst.stateMutex` unexported / inaccessible from `server` package
**Status: RESOLVED**

Task 3.1.1a now adds `Instance.HistoryFilePath() string` (acquires `stateMutex.RLock` internally) and `Instance.HasGitHubPR() bool` (Task 3.1.1c). The `dependencies.go` closure never touches `stateMutex` directly. The plan also adds `FindInstanceByHistoryPath` as a package-level helper in `session/artifact_lookup.go`. The accessor pattern matches the existing `Instance` convention.

### Blocker 3: `mergeAndPersist` overwrites existing data on restart
**Status: RESOLVED**

`mergeAndPersist` now takes a `readFn func(title string) (string, error)` injected at construction and loads the existing blob before merging. `seedOffsetsFromDB(instances []instanceSnapshot)` is called from `Start()` before workers launch, restoring byte offsets from previously stored blobs so restarts do not re-scan from byte 0. Both the O(new-bytes) property and data durability are preserved.

### Blocker 4: JSONL type coupling
**Status: RESOLVED**

Task 2.1.0a creates `session/artifacts/jsonl.go` with private copies of `artifactEntry`, `artifactMessage`, `artifactContentBlock`. The plan no longer imports `session/tokens` types for struct definitions. The rationale (avoiding coupling to the `tokens` package's 237 KB allocation optimization) is documented in the ADR decision comment.

---

## Previously Flagged Concerns — Status

| Concern | Status | Notes |
|---|---|---|
| O(n²) dedup with `strings.Contains` | **RESOLVED** | `ExtractFromToolResult` now uses `map[string]struct{}` for all three dedup sets; exact-key comparison, not substring |
| Scanner buffer 1 MB (should be 10 MB) | **RESOLVED** | `maxScannerTokenSize = 10 * 1024 * 1024`; constant declared in `store.go`; `scanner.Buffer` call references it |
| `scanner.Err()` never checked | **RESOLVED** | After the scan loop: `if err := scanner.Err(); err != nil { log.Warn(...); return }` — aborts without advancing offset |
| Byte-offset accounting (`len(line)+1`) desyncs on `\r\n` | **RESOLVED** | Offset now determined by `f.Seek(0, io.SeekCurrent)` after the loop; manual line-length arithmetic removed |
| Event push wired through opaque closure | **RESOLVED** | `OnScanComplete func(title string, blob *SessionArtifactsBlob)` is now an injected hook with a no-op default; event bus logic is assigned in `dependencies.go` after construction |
| `lookupTitle` O(n) scan on every callback | **PARTIALLY RESOLVED — NEW CONCERN (see below)** | |
| PR poller race guard reads `GitHubPRNumber` without lock | **RESOLVED** | `HasGitHubPR() bool` method added to `session/instance.go`; acquires `stateMutex.RLock` internally |

---

## New Concerns Introduced by the Patches

### Concern 1: `lookupTitle` still O(n) per callback; no cache invalidation path
**Severity: Concern (not blocker)**

The `lookupTitle` closure closes over `instances []*session.Instance`, the slice that was in scope at the time `BuildDependencies` ran. This slice is **captured by value** (a slice header copy). New sessions created after startup — which are appended to the live instance list via `storage` or `session.Manager` — will not appear in this snapshot. The `OnScanComplete` closure has the same problem: it iterates the same captured `instances` slice.

This means:
- A session created after server boot will never have its artifacts persisted (lookup returns `("", false)`)
- A session created after boot that scans a JSONL file will silently drop all extracted data

The original concern about O(n) was flagged. The patch resolves the lock-race issue but does not resolve the staleness issue. The previous review recommended building a `map[string]string` updated via `historyLinker.SetInstances`, or injecting a live snapshot closure. The patch instead closes over a snapshot.

**Recommendation**: Replace the captured `instances` slice with a live-snapshot closure: `func() []*session.Instance { return sessionManager.Instances() }` (or equivalent), called inside `lookupTitle` and `OnScanComplete` on every invocation. Verify whether `historyLinker` or `sessionManager` already exposes such a method and use it.

---

### Concern 2: `SeedOffsets` is called as a public method but `seedOffsetsFromDB` is defined as a private method
**Severity: Concern (compilation blocker if not caught)**

Task 2.1.3b defines the method as:
```go
func (ae *ArtifactExtractor) seedOffsetsFromDB(instances []instanceSnapshot) {
```
(lowercase, unexported)

But Task 3.1.1b calls it as:
```go
artifactExtractor.SeedOffsets(instances) // uppercase
```

`SeedOffsets` is not defined anywhere in the plan. This will fail to compile. Additionally, `instances` in `dependencies.go` is `[]*session.Instance`, but `seedOffsetsFromDB` takes `[]instanceSnapshot` (the plan's private helper type). There is no conversion shown between the two slice types.

**Recommendation**: Either (a) export `seedOffsetsFromDB` as `SeedOffsets(instances []*session.Instance)` and convert internally, or (b) make `Start()` accept the instances slice and call the private method internally. The plan must be consistent on method name and signature; right now it is not.

---

### Concern 3: `lastGoodPos` accumulation is stale/vestigial after the `f.Seek` fix
**Severity: Concern (logic confusion, potential future regression)**

In `scan.go` (Task 2.1.3a), the scan loop has:
```go
var lastGoodPos int64 = startPos
for scanner.Scan() {
    ...
    curPos, _ := f.Seek(0, io.SeekCurrent)
    _ = curPos // used below via f.Seek after loop
    lastGoodPos += int64(len(line)) + 1 // approximate; replaced by Seek below
```

The `lastGoodPos` variable is still updated by `len(line) + 1` inside the loop even though the comment says it is "replaced by Seek below." After the loop, the code uses:
```go
newOffset, _ := f.Seek(0, io.SeekCurrent)
```
and never references `lastGoodPos` again. So `lastGoodPos` is assigned but never read — a dead write. Worse, `curPos` is also assigned inside the loop with `_ = curPos`, which means it is discarded. This is contradictory: the code appears to intend tracking good position via both `lastGoodPos` and `curPos` but uses neither. This is confusing dead code that will cause linter warnings (`curPos` declared and not used in some Go versions, depending on the `_ =` suppression).

The `f.Seek(0, io.SeekCurrent)` call *inside* the loop (discarded as `curPos`) does work as a position query because `bufio.Scanner` reads in chunks — the file position returned by `f.Seek(0, io.SeekCurrent)` reflects scanner's internal buffer, not the logical scan position. Calling `f.Seek` inside the loop while `bufio.Scanner` is buffering is unreliable; the `Scanner` reads ahead in 4096-byte chunks. The only reliable `f.Seek(0, io.SeekCurrent)` call is the one *after* `scanner.Scan()` returns false — at that point the scanner has stopped reading and the file position reflects actual consumed bytes (though still buffered).

**Recommendation**: Remove `lastGoodPos`, `curPos`, and their dead assignments. Keep only the post-loop `newOffset, _ := f.Seek(0, io.SeekCurrent)`. Add a comment: "bufio.Scanner reads ahead; file position is only reliable after Scan() returns false." This prevents future maintainers from re-introducing the in-loop tracking.

---

## Minors — Status

| Minor | Status | Notes |
|---|---|---|
| `dedup` body is a stub comment | **STILL PRESENT** | `func dedup(ss []string) []string { /* set dedup, preserve order */ }` — still a stub in Task 2.1.3b. Implementer must write the actual body. |
| No `IsLoading()` equivalent for startup walk | **STILL PRESENT** | `walkAndEnqueue` has no loading signal. Empty state in ArtifactsTab mitigates UX impact. |
| e2e test clicks first session with no guard for empty list | **STILL PRESENT** | `page.getByRole("listitem").first()` will fail on a fresh test instance. Add a fixture or skip guard. |
| `ArtifactsTab.css.ts` import path from theme | **PARTIALLY RESOLVED** | Import path corrected to `"../../styles/theme-contract.css"` (NOT `theme.css`). Token name corrected to `vars.color.primary`. However, the plan includes a "NOT vars.color.actionPrimary — verify token exists" comment, meaning the implementer is still required to read `theme-contract.css.ts` to validate. If `vars.color.primary` also does not exist, the build will fail. This is a mild risk, not a blocker. |
| `ArtifactsTab` null-check vs. empty-array guard commentary | **RESOLVED** | Plan now has three distinct sub-states (undefined → "Extraction pending", all-empty → "No artifacts found", content → rendered). The null check and empty-array guard are both present and correct. |
| `ParseGitHubURL` import not listed in `scan.go` | **STILL PRESENT** | `scan.go` uses `session.ParseGitHubURL` (Task 3.1.1c calls it from `dependencies.go`, not `scan.go` — this minor was slightly misattributed). The import is implicitly needed in `dependencies.go`; verify it is listed in the import block added in Task 3.1.1b. |
| Frontend empty-state text mismatch | **NEW MINOR** | The acceptance criteria in Story 5.1.2 says: `"ArtifactsTab_should_showEmptyState_When_artifactsIsNull"` and the test asserts `screen.getByText(/No artifacts found yet/)`. But the component renders `"Extraction pending — will populate automatically..."` when `artifacts === undefined`. The regex `/No artifacts found yet/` will not match; the test will fail immediately. The two empty states need aligned text between the component and the test. |

---

## Summary

**All 4 original blockers are resolved.** The patches address the core structural problems cleanly.

**3 new concerns introduced:**
1. `lookupTitle`/`OnScanComplete` close over a stale snapshot — sessions created after startup are invisible (correctness hole)
2. `SeedOffsets` vs. `seedOffsetsFromDB` name/signature mismatch — will not compile as written
3. `lastGoodPos`/`curPos` dead-write confusion in scan loop — confusing but not a runtime bug

**5 minors remain or are newly introduced** (including one new test assertion mismatch that will cause `ArtifactsTab.test.tsx` to fail).

**Recommendation**: Address Concerns 1 and 2 before implementation — Concern 1 is a correctness bug affecting post-startup sessions, Concern 2 is a compilation failure. Concern 3 and the minors can be fixed during implementation with minimal risk.
