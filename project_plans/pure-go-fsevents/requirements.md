# Requirements: pure-go-fsevents

**Item ID:** ec370109-0d4f-4fc7-a74f-7a434645837c  
**Status:** Pre-implementation triage  
**Category:** Feature (infrastructure)

## Problem Statement

The `session/unfinished` scanner and `session/git_worktree_watcher.go` both use `fsnotify`
for change detection. On macOS, fsnotify falls back to kqueue: one file descriptor per
watched file/directory, plus the inode-following limitation that makes rename-over saves
(the write pattern used by most editors and git itself) go undetected. The result is silent
misses in the "changed files" trigger path, forcing reliance on the backstop periodic tick.

Every alternative Go FSEvents wrapper (fsnotify/fsevents, rjeczalik/notify) requires cgo.
Although the project currently builds with `CGO_ENABLED=1` (required for go-sqlite3), adding
a second cgo dependency tied to macOS CoreServices complicates cross-compilation from Linux
build hosts and increases the linker surface. `ebitengine/purego` v0.10.0 is already an
**indirect** dependency; promoting it to a direct dependency to call CoreServices via
`dlopen` + `purego.RegisterLibFunc`/`purego.NewCallback` is zero additional binary weight.

## Goal

Deliver a small `internal/fsevents` package that:

1. Wraps the macOS FSEvents API via `purego` (no cgo) with `CGO_ENABLED=0 GOOS=darwin` build.
2. Exposes a Watchman-style "changed paths since token" contract: start a stream, receive
   batches of `(path, flags, eventID)`, resume from a saved event ID after restart.
3. Surfaces the six rescan flags callers must treat as "full-scan required":
   `MustScanSubDirs`, `UserDropped`, `KernelDropped`, `EventIdsWrapped`, `RootChanged`,
   `HistoryDone`.
4. Provides flush (`FSEventStreamFlushSync`), current event ID (`FSEventsGetCurrentEventId`),
   and volume UUID (`FSEventsCopyUUIDForDevice`) for cache-key invalidation on volume remount.
5. Includes `Stop`/`Invalidate`/`Release` lifecycle with no goroutine or CF object leaks.
6. Integrates into the existing `session/unfinished/scanner.go` and
   `session/git_worktree_watcher.go` scan-trigger paths behind a build tag, with Linux/other
   continuing to use the current kqueue-backed fsnotify path unchanged.

## Constraints

- Build tag: `//go:build darwin` isolates all CoreServices calls; no new symbol references on
  other platforms.
- `ebitengine/purego` is already in `go.sum` as an indirect dep; promote to direct, no
  version change required.
- No new cgo dependency for the FSEvents binding itself (the existing cgo for go-sqlite3 is
  unrelated and may remain).
- Callback thread safety: `purego.NewCallback` issues require verification that CoreFoundation
  dispatch queue threads can safely call back into the Go runtime.
- Event volume measurement (spike) must precede scanner integration to bound CPU cost.

## Acceptance Criteria

1. `internal/fsevents` package compiles with `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64` and
   `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64` without errors.
2. Integration test on macOS: create, modify, rename-over, and delete a file in a watched
   directory; all four events are delivered to the callback within 500 ms.
3. Resume from saved event ID: stop stream, perform file operations, start new stream with the
   saved ID; missed events are replayed and `HistoryDone` flag fires.
4. Overflow/rescan flag: when a `UserDropped` or `KernelDropped` flag arrives, the caller's
   rescan callback is invoked (not swallowed).
5. Volume UUID check: `FSEventsCopyUUIDForDevice` returns a non-zero UUID for any local
   (non-NFS) volume; a cross-volume-mount scenario (volume remounted) produces a changed UUID.
6. Spike report: measured event count per second during a `go build .` run and a
   `git checkout` on a real repo, captured in `project_plans/pure-go-fsevents/research/spike-event-volume.md`.
7. Scanner integration (darwin only): after integration, the backstop tick period for macOS
   can be lengthened (e.g. 2× current) without increasing scan latency on change, verified by
   the existing `scanner_eventdriven_test.go`.
8. No goroutine leaks after `Stop()`: `goleak.VerifyTestMain` or equivalent passes in the
   integration test suite.
9. `golangci-lint` passes with no new suppressions in the package.
10. Linux CI remains green: the build tag prevents any CoreServices symbol from being linked
    on Linux.
