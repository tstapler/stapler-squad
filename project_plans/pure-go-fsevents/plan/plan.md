# Implementation Plan: pure-go-fsevents

## Architecture

```
internal/fsevents/               ← new package, //go:build darwin
  fsevents.go                    ← Stream type, public API
  fsevents_types.go              ← Flag constants, EventID type alias
  cfrunloop.go                   ← Dedicated CFRunLoop goroutine helper
  fsevents_test.go               ← Integration tests (darwin only)
  fsevents_stub.go               (//go:build !darwin) ← no-op stub for compile check

session/unfinished/
  scanner_fsevents_darwin.go     ← darwin scanner wiring (replaces fsnotify loop)
  scanner_fsevents_other.go      ← no-op stub (other platforms)

session/
  git_worktree_watcher_fsevents_darwin.go  ← darwin worktree watcher integration
  git_worktree_watcher_fsevents_other.go   ← no-op stub
```

## Phase 1: Spike — purego callback thread safety (½ day)

Before any production code:
- Write a minimal `spike/fsevents_spike_test.go` that:
  1. `dlopen`s `/System/Library/Frameworks/CoreServices.framework/CoreServices`
  2. Creates a single FSEvents stream on `$TMPDIR`
  3. Calls `purego.NewCallback` with a Go closure that sends to a channel
  4. Verifies delivery within 1 second of a `os.WriteFile` in the watched dir
- Run with `-race` and `goleak` on macOS
- Record thread-attach behavior and any panics
- If `purego.NewCallback` on a CFRunLoop thread is unsafe, pivot to a
  `dispatch_queue_create` C shim with a Go-side channel receive

Output: `project_plans/pure-go-fsevents/research/spike-event-volume.md`

## Phase 2: `internal/fsevents` package (1 day)

### Public API

```go
// Stream watches one or more directory trees via FSEvents.
// Must be created with NewStream; safe for concurrent use after Start.
type Stream struct { ... }

// EventID is a monotonically increasing FSEvents cursor.
type EventID = uint64

const SinceNow EventID = 0xFFFFFFFFFFFFFFFF

// Flags carried on each event; callers must inspect RescanFlags.
type EventFlags uint32

const (
    MustScanSubDirs EventFlags = 0x00000001
    UserDropped     EventFlags = 0x00000002
    KernelDropped   EventFlags = 0x00000004
    EventIdsWrapped EventFlags = 0x00000008
    HistoryDone     EventFlags = 0x00000010
    RootChanged     EventFlags = 0x00000020
    // FileEvents-mode flags
    ItemCreated     EventFlags = 0x00000100
    ItemRemoved     EventFlags = 0x00000200
    ItemModified    EventFlags = 0x00001000
    ItemRenamed     EventFlags = 0x00000800
)

// RescanFlags are the flags that require a full rescan.
const RescanFlags = MustScanSubDirs | UserDropped | KernelDropped |
    EventIdsWrapped | RootChanged

// Event is delivered to the EventFunc for each FSEvents batch entry.
type Event struct {
    Path    string
    Flags   EventFlags
    EventID EventID
}

// EventFunc is called for each batch; may be called from a non-Go thread.
// Implementations must not block; send to a buffered channel.
type EventFunc func(events []Event)

// NewStream creates a stream watching paths (directory roots).
// sinceWhen: SinceNow or a previously saved EventID for replay.
// latency: coalesce window (0 = NoDefer).
func NewStream(paths []string, sinceWhen EventID, latency time.Duration, fn EventFunc) (*Stream, error)

func (s *Stream) Start() error
func (s *Stream) Stop()  // idempotent; blocks until run loop exits

// FlushSync forces delivery of any pending events synchronously.
func (s *Stream) FlushSync()

// CurrentEventID returns the most recent event ID for this stream.
func CurrentEventID() EventID  // wraps FSEventsGetCurrentEventId

// VolumeUUID returns the UUID for the volume containing path.
// Returns "" if path is on a network volume or UUID is unavailable.
func VolumeUUID(path string) string  // wraps FSEventsCopyUUIDForDevice
```

### Implementation notes

- `NewStream` allocates a `CFArrayRef` of `CFStringRef` from `paths`, calls
  `FSEventStreamCreate` with `kFSEventStreamCreateFlagFileEvents |
  kFSEventStreamCreateFlagNoDefer | kFSEventStreamCreateFlagWatchRoot`.
- A dedicated OS thread (`runtime.LockOSThread()` + `CFRunLoopRun()`) owns the run loop.
  `Stop()` calls `FSEventStreamStop` + `CFRunLoopStop` then waits on a done channel.
- `purego.NewCallback` wraps the C-signature callback:
  `(streamRef uintptr, info uintptr, numEvents uintptr, paths uintptr, flags uintptr, ids uintptr)`.
  The callback converts raw C arrays to `[]Event` and calls the user's `EventFunc`.
- All `CFRelease` calls are deferred in `Stop()` to avoid leaks.
- Build tags: `//go:build darwin` on all files in `internal/fsevents/` except the stub.

## Phase 3: Integration — unfinished scanner (½ day)

`session/unfinished/scanner_fsevents_darwin.go`:
- Implements an `fseventsWatcher` that wraps `internal/fsevents.Stream`.
- On darwin, `Start()` creates a stream per unique repo root (not one per `.git` dir — the
  stream's `FileEvents` flag covers the whole tree, so we filter `.git/` paths in the
  callback).
- Delivers coalesced repo keys to the existing `fsDebounce` channel path.
- Stores `{lastEventID, volumeUUID}` per repo; on restart, resumes from saved ID.
- On any `RescanFlags` event: enqueue a full-scan for that repo.
- `scanner_fsevents_other.go`: `//go:build !darwin` stub with the same function signatures,
  all returning `(nil, nil)` or `false`.

## Phase 4: Integration — git_worktree_watcher (½ day)

Same pattern as scanner integration; the per-worktree `WorktreeChangeDetector` gains a
darwin-specific `startFSEventsWatch()` that replaces `gitWatchLoop`. The `RootChanged` flag
triggers the existing worktree-gone handling path.

## Phase 5: Tests and spike report (½ day)

- `internal/fsevents/fsevents_test.go`:
  - Create/modify/rename/delete in `t.TempDir()`; assert events within 500 ms.
  - Stop and resume: operate on dir; restart with saved ID; assert replay + `HistoryDone`.
  - Overflow simulation: not easily reproducible in a test; assert the flag is forwarded.
  - `goleak.VerifyTestMain` for leak detection.
- Spike report: measure event count during `go build .` and `git checkout` on this repo.
- CI: tests are `//go:build darwin` — skipped on Linux CI automatically.

## Risks and mitigations

| Risk | Likelihood | Mitigation |
|------|-----------|-----------|
| `purego.NewCallback` unsafe on CF thread | Medium | Spike in Phase 1; fallback = dispatch shim |
| Event storm during large checkout floods channel | Low | Buffered channel (1024 events); drop with counter; backstop tick unchanged |
| TCC blocking FSEvents on protected paths | Low (not a regression) | Document; same behaviour as existing kqueue watcher |
| purego API breakage in future version | Low | purego v0.10.0 already locked in go.sum |
| Stub compile on Linux misses symbol | Low | CI runs `CGO_ENABLED=0 GOOS=linux go build ./...` in lint step |

## Out of scope

- Windows support (no FSEvents equivalent; inotify on Linux is already fine)
- `FSEventStreamCreateRelativeToDevice` (device-relative streams) — deferred to Tier B
- Exposing the FSEvents binding as a standalone module / open-source release
- go-git `DirPrefixes`/`Paths` scoped-status primitive (separate item, mentioned in backlog
  description as "also needs" but distinct)
