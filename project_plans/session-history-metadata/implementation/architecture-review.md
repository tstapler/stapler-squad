# Architecture Review: session-history-metadata Implementation Plan

**Reviewer**: Claude Code (architecture review)
**Date**: 2026-06-22
**Plan file**: `project_plans/session-history-metadata/implementation/plan.md`

---

## Summary Verdict

The plan is structurally sound and closely mirrors existing patterns. Four issues need resolution before implementation begins — one is a compile blocker, two are correctness issues, and one is a design smell.

---

## Finding 1: JSONL Types Are Unexported — Export Them (Compile Blocker)

**Confirmed bug.** Reading `session/tokens/jsonl_types.go` directly: all three types are unexported:

```go
type jsonlEntry struct { ... }
type jsonlUserMessage struct { ... }
type jsonlUserContent struct { ... }
```

The plan's `scan.go` references `tokens.JSONLEntry` and `tokens.JSONLUserMessage` (lines 421, 432). These will not compile. The plan correctly identifies the two options (export or copy), but leaves the choice open. **Recommended resolution: export them.** The plan correctly shows the rename pattern (`jsonlEntry` → `JSONLEntry`, etc.). Copying locally creates a maintenance hazard — if the JSONL format evolves, there will be two structs to update. The `jsonlUserContent` type has a performance-tuned design comment (avoids the `Input json.RawMessage` allocation from assistant messages) that is equally valid in the artifacts scanner. Export all three and delete the local copies if any were drafted.

---

## Finding 2: `mergeAndPersist` Overwrites Existing Data — Merge Logic Is Broken

**Correctness issue.** The `mergeAndPersist` implementation in Task 2.1.3b constructs the persisted blob from only the current scan batch's `newPRURLs / newCommitSHAs / newExternalURLs`:

```go
blob := SessionArtifactsBlob{
    PRURLs:       dedup(newPRURLs),      // ← only THIS scan's findings
    CommitSHAs:   dedup(newCommitSHAs),
    ExternalURLs: cap50(dedup(newExternalURLs)),
    ...
}
```

For an incremental scanner, this means artifacts found in scan #1 are deleted when scan #2 runs, because scan #2 only sees the new bytes. The in-memory comment ("accumulate in-memory per filePath between process restarts") does not survive a restart, and the offsetsMu-guarded `offsets` map is also lost on restart. The result is that after a restart, all offsets reset to 0, the full file is re-scanned, and all artifacts are rediscovered — but between a restart and the next file-change event, the DB blob may be stale.

**Correct approach**: `mergeAndPersist` must first read the existing blob from the DB (via an injected `readFn func(title string) (string, error)`) and merge new findings into the existing set. The `ScanOffsetBytes` in the blob also serves as the authoritative offset after restart — the in-memory `offsets` map should be seeded from the blob's `ScanOffsetBytes` on first access.

The plan's Pitfall Reference correctly lists "O(n²) re-read" but the proposed mitigation (`ScanOffsetBytes` in blob) is only effective if `ScanOffsetBytes` is loaded on startup and merging reads the existing blob. The current implementation sketch does neither.

---

## Finding 3: `storage.GetInstances()` Does Not Exist — Use `storage.LoadInstances()`

**Compile blocker.** The `lookupTitle` closure in Task 3.1.1a calls:

```go
for _, inst := range storage.GetInstances() {
```

There is no `GetInstances()` method on `*session.Storage`. Reading `session/storage.go`, the methods available are:

- `LoadInstances() ([]*Instance, error)` — loads from DB, constructs Instance objects
- `ListInstanceData() ([]InstanceData, error)` — returns raw data without constructing Instance objects
- `ListSessionRecords() []tokens.SessionRecord`

`LoadInstances()` is also wrong here because it performs DB I/O on every file-change callback and returns an error. The correct approach is to use the already-loaded `instances []*session.Instance` slice from `BuildDependencies` — it is already in scope at the wiring site and kept live by the server. The closure should capture that slice directly (or a function that reads from the StatusManager or ReviewQueue which already hold live Instance pointers). Accessing `inst.stateMutex` and `inst.HistoryFilePath` is valid because both are in the `session` package and `dependencies.go` is in the `server` package which imports `session` — `stateMutex` is unexported, so this will be a compile error from the `server` package. The closure must call an exported helper or be placed in the `session` package.

**Recommended fix**: Add an exported method on `*Storage` or `*session.Instance` such as:
```go
// FindInstanceByHistoryPath returns the title of the instance whose HistoryFilePath
// matches filePath, or "" and false if not found.
func FindInstanceByHistoryPath(instances []*Instance, filePath string) (string, bool)
```
Pass `instances` captured from the local `BuildDependencies` scope and call this exported function from the closure.

---

## Finding 4: JSON Blob Field Type — `field.String` Matches the Established Pattern

**Pattern consistent.** The existing `session_goal` schema uses:

```go
field.String("tasks").Optional().Comment("JSON []TaskNode"),
```

The plan proposes the same pattern for `session_artifacts`:

```go
field.String("session_artifacts").Optional().Default("")
```

This is correct. The `field.JSON` type in ent maps to a SQLite BLOB column and uses Go's built-in JSON marshaler internally — it does not provide any query/filter capability on JSON subfields (SQLite's JSON functions are not used by ent's JSON field type). Since this project does not need to query inside the artifacts blob (only read and write the whole value), `field.String` is the right choice and matches the established `tasks` field pattern in `session_goal.go`. The `Default("")` is also correct — it ensures the column has a non-null empty value rather than NULL, which avoids nil-check boilerplate when reading.

**One minor note**: the approvalrule schema uses `field.JSON("programs", []string{})` for typed list fields that ent can serialize automatically. Since `SessionArtifactsBlob` is a complex struct (not a primitive slice), `field.String` with manual `json.Marshal`/`json.Unmarshal` is the correct choice — consistent with `session_goal.tasks`.

---

## Finding 5: Event Push Location — Injected Callback Preferred Over `storeFn` Closure

**Design smell, not a blocker.** The plan puts event bus push inside the `storeFn` closure in `dependencies.go` (Task 3.1.2a, last bullet). This means `ArtifactExtractor` calls `storeFn` and that closure also does event publishing. This is workable but creates an opaque dependency: the `ArtifactExtractor` struct itself has no event semantics, but its behavior changes based on what `storeFn` happens to do.

The existing `PRStatusPoller` pattern in `dependencies.go` (line ~471) is cleaner: the struct does its work, and an `onUpdated` callback is injected separately at wiring time:

```go
svc.PRStatusPoller.SetOnUpdated(func(inst *session.Instance) {
    eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_priority", "github_pr_state"}))
})
```

**Recommended approach**: Add `onUpdated func(title string)` to `ArtifactExtractor`. The extractor calls `onUpdated(title)` after `storeFn` succeeds. The wiring in `dependencies.go` sets it to the event bus publish. This keeps `ArtifactExtractor` testable without an event bus, and matches the separation of concerns in `PRStatusPoller`. The plan notes this in Task 3.1.2a ("or a helper method on ArtifactExtractor") but the `onUpdated` pattern should be the primary recommendation, not the secondary one.

---

## Summary Table

| # | Finding | Severity | Resolution |
|---|---|---|---|
| 1 | `jsonlEntry`/`jsonlUserMessage`/`jsonlUserContent` are unexported | **Compile blocker** | Export all three types in `session/tokens/jsonl_types.go` |
| 2 | `mergeAndPersist` overwrites existing DB data instead of merging | **Correctness** | Inject `readFn`; seed in-memory offsets from blob's `ScanOffsetBytes` on first scan |
| 3 | `storage.GetInstances()` does not exist; `inst.stateMutex` inaccessible from `server` package | **Compile blocker** | Capture `instances []*session.Instance` from wiring scope; add exported `FindInstanceByHistoryPath` helper |
| 4 | `field.String("session_artifacts")` field type | **Correct** | Matches `session_goal.tasks` established pattern; no change needed |
| 5 | Event push in `storeFn` closure vs. injected `onUpdated` callback | **Design smell** | Prefer `onUpdated` injection (mirrors `PRStatusPoller.SetOnUpdated` pattern) |
