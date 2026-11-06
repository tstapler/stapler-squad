# Research Findings: pure-go-fsevents

## Codebase State

### Current file-watch infrastructure
- **`session/unfinished/scanner.go`**: uses `github.com/fsnotify/fsnotify` v1.10.1.
  `fsnotifyLoop()` watches each repo's `.git` dir. On macOS, fsnotify uses kqueue (one fd per
  path, no recursive watching, rename-over saves are missed because the old inode is watched).
  A 2-second debounce (`fsnotifyDebounce`) backstops missed events via a periodic tick.
- **`session/git_worktree_watcher.go`**: also uses `fsnotify` for `.git` dir change detection
  in the per-worktree change detector.
- **`go.mod`**: `github.com/ebitengine/purego v0.10.0 // indirect` — already present, no
  version bump required; promoting to direct costs nothing.
- **`Makefile`**: `CGO_ENABLED=1` is required for `go-sqlite3`. A pure-Go FSEvents binding
  does not conflict; it simply means the FSEvents path avoids adding a *second* cgo surface
  area for something that can be done with purego.
- **No existing `internal/fsevents`** package — this is a greenfield addition.

### Build tag convention
The repo uses `//go:build unix` in `session/unfinished/statdir_unix.go` and
`//go:build !unix` in `statdir_other.go` — the same pattern applies to a
`//go:build darwin` / `//go:build !darwin` pair for FSEvents vs. fsnotify.

## FSEvents API surface needed

The binding only needs a subset of FSEvents:

| Symbol | Purpose |
|--------|---------|
| `FSEventStreamCreate` | Create a stream for a set of path roots |
| `FSEventStreamStart` / `FSEventStreamStop` | Lifecycle |
| `FSEventStreamInvalidate` / `FSEventStreamRelease` | Memory cleanup |
| `FSEventStreamFlushSync` | Force pending events to be delivered synchronously |
| `FSEventStreamScheduleWithRunLoop` | Register with CFRunLoop |
| `CFRunLoopGetCurrent` / `CFRunLoopRun` / `CFRunLoopStop` | Run loop management |
| `FSEventsGetCurrentEventId` | Snapshot "last known" event ID for caching |
| `FSEventsCopyUUIDForDevice` | Volume UUID → invalidate saved event ID on remount |
| `kFSEventStreamEventFlagMustScanSubDirs` etc. | Rescan flag constants (compile-time) |

## purego approach: dlopen + RegisterLibFunc

`purego` v0.10.0 supports `purego.Dlopen` + `purego.RegisterLibFunc` for calling C
functions with Go-compatible signatures (pointers, uintptrs, floats), and
`purego.NewCallback` for passing a Go function as a C function pointer.

Constraints and known issues:
1. **Callback threading**: `purego.NewCallback` attaches the goroutine to the OS thread.
   CoreFoundation's dispatch queues run callbacks on their own threads; Go's runtime expects
   goroutines to be attached. `runtime.LockOSThread()` inside the callback stabilises this.
   The ebitengine/oto audio package uses this exact pattern (dlopen CoreAudio + purego
   callback) and is in production — good precedent.
2. **CFRunLoop ownership**: FSEvents requires a `CFRunLoop`-attached stream. The standard
   approach is a dedicated goroutine with `runtime.LockOSThread()` that calls `CFRunLoopRun()`.
   The Go runtime can schedule other goroutines on the same OS thread only until
   `LockOSThread` is called; after that, `CFRunLoopRun` blocks, so events arrive on that
   thread and are forwarded to a channel.
3. **Pointer stability**: Go GC can move objects; passing Go slice/string pointers directly
   to C is unsafe without `unsafe.Pointer` pinning or using `CFStringCreateWithCString`.
   CoreFoundation arrays (CFArrayRef of CFStringRef) must be built with CF APIs, then
   released via `CFRelease`.
4. **sinceWhen / event ID replay**: `FSEventStreamCreate` takes a `FSEventStreamEventId`
   (uint64). Using `kFSEventStreamEventIdSinceNow` (0xFFFFFFFFFFFFFFFF) starts fresh.
   Passing a saved ID replays all events since that ID. The `HistoryDone` flag signals the
   end of replay.

## Existing alternatives assessed

| Package | License | CGO | Verdict |
|---------|---------|-----|---------|
| `fsnotify/fsnotify` | BSD-3 | No | Kqueue on macOS — rename-over misses |
| `fsnotify/fsevents` | BSD-3 | **Yes** | Unmaintained, 4096-path limit, API unstable |
| `rjeczalik/notify` | MIT | **Yes** (inferred from `_fsevents.go`) | No event-ID replay |
| `ebitengine/purego` (direct) | Apache-2 | **No** | Greenfield; all control, no inherited debt |

## TCC / Codesigning risk

FSEvents on protected paths (Desktop, Documents, Downloads) requires FDA (Full Disk Access)
or at minimum the path being under the app's sandbox. The existing self-signed cert
(`make setup-codesign`) does not grant FDA. For worktree paths under `~/code/` this is not
an issue (no system protection on user-owned paths). Protected-folder access is the same
TCC constraint that already affects `.git` watcher paths — no regression vs current state.

## Event volume (to be measured in spike)

Watchman documentation claims `~/code` checkouts can produce 10,000–100,000 events during a
`git checkout`; however this is for a directory-recursive stream. A stream anchored to a
single repo root with `NoDefer` and `FileEvents` flags will produce one event per
file-operation. A `go build .` on this repo produces ~500 object files; a `git checkout`
on a medium repo may produce 2,000–5,000 events. The spike test should measure this
directly before integrating into the scanner.

## Integration touchpoints

1. **`session/unfinished/scanner.go`**: replace/augment `fsnotifyLoop` on darwin with an
   FSEvents stream; keep the existing kqueue path on other platforms.
2. **`session/git_worktree_watcher.go`**: similarly replace `gitWatchLoop` on darwin.
3. **Cache key**: add `{eventID, volumeUUID}` fields to the per-worktree scan cache so a
   clean result can be returned in O(1) when the stream confirms no new events.

## Open questions

1. Can `purego.NewCallback` safely receive FSEvents callbacks on a CoreFoundation-owned
   thread? → spike must verify; if not, a `dispatch_queue_create` + Go channel approach
   avoids the thread model issue.
2. Should the binding live in `internal/fsevents` (app-internal) or a separate module (for
   potential open-source reuse)? Given no existing pure-Go FSEvents package, there is
   community interest but the risk of stable-API pressure; start internal.
3. Does `FSEventStreamCreate` with `CreateRelativeToDevice` give better dedup than path-based
   streams for repos on the same device? Probably yes but adds volume UUID bookkeeping.
4. What happens to the stream when the watched directory is deleted (worktree cleanup)?
   `RootChanged` flag fires; caller must call `Stop()` and not attempt restart until the path
   re-appears.
