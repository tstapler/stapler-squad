# Tune the filesystem watchers (`internal/dirwatch`)

Squad watches `~/.claude/projects`, every tracked repo's `.git` and linked-worktree admin dirs, and each repo's `objects/pack`. On macOS those watches go through `internal/dirwatch`, which uses FSEvents so a watch costs no file descriptor per watched file.

## Why not plain fsnotify

fsnotify's kqueue backend opens one fd per file inside every watched directory. On a long-lived instance (691 worktree admin dirs, 354 Claude project dirs) that held about 11,000 of 13,800 fds (`lsof -p <pid>`, 2026-11-05).

## Knobs and signals

| What | How |
|---|---|
| Roll back to kqueue | start with `STAPLER_SQUAD_DIRWATCH=fsnotify` |
| Watches per watcher | `fs.watches{watcher=...}` gauge; WARN above 4,000 per watcher |
| Live FSEvents streams | `fs.streams` gauge (all FSEvents streams in the process, including the working-tree watcher); each locks an OS thread and the kernel refuses new ones near 512 per process. Streams cost no fds: measured +0 fds across 280 `dirwatch` streams (`TestFDFreeStreamsHoldNoFDsEach`); `kFSEventStreamCreateFlagWatchRoot` used to add about 14 fds per stream (4,206 at 300), so `dirwatch` streams set `NoWatchRoot` |
| Kqueue fallback paths | `fs.fallback_watches` gauge; fallback paths stay on kqueue until restart (no re-promotion) |
| First-event lag | `fs.stream_first_event_lag` histogram per watcher; WARN when a stream takes over 5 s to deliver its first event |
| Stream budget | `dirwatch.DefaultMaxStreams` (300); past it, `Add` degrades to kqueue for that path and logs one WARN |
| Open fds | existing `process.open_fds` gauge |

`internal/fsevents` streams created elsewhere (working-tree watcher, worktree change detector) still hold about 14 fds each (WatchRoot on); they count toward `fs.streams` and the budget but are not capped by it.

Share one stream across many directories with `dirwatch.RootAt(root)` (history watcher) or `dirwatch.WithRootFunc` (scanner: all `.git/worktrees/*` of a repo). One stream per directory does not scale past a few hundred.

## Known limits

- FSEvents can delay or drop events with no error (observed: first delivery 13.8 s after `Add` in one run, 4 of 29 writes delivered in another, both at load average above 100). Safety nets: the scanner re-enqueues each repo once 15 s after `AddRepo`, the history watcher re-enqueues transcripts modified since start 15 s after `Start`, and the pack watcher refreshes indexes 0.2 s and 15 s after its watch starts. A repack at any later point still relies on the stream (or the registry TTL).
- A write in the first moments after a stream starts can be missed; `Add` replays from a pre-start event id and flushes, which cut misses from 4-9 of 30 to 1 of 30 at load average 75-110, with an average first event after about 0.6 s. The consumers' tick/poll backstops cover the rest.
- A stream on `.git` is recursive, so `objects/**` churn reaches the callback; events are filtered to the watched paths, and a dropped-events flag escalates to a rescan only when the dropped region can contain a watched path.
- Tests that need synchronous delivery force fsnotify through `dirwatch.ForceFSNotify(true)` in `TestMain`.

## Decision record

Options measured or projected against the 2026-11-05 live process:

| Option | fds | Risk |
|---|---|---|
| Full-dir kqueue (before) | about 11,000 | reliable, unaffordable |
| kqueue on specific files only (HEAD and index per admin dir) | about 1,400 for Tier A alone (2 x 691, INFERRED) | git replaces both by lock-file rename, so each watch dies with the old inode and needs a re-add; cold worktrees would still need a ticker |
| kqueue on live sessions only | a few hundred (INFERRED) | needs session-lifecycle wiring; cold repos go stale until the tick |
| FSEvents with shared roots, kqueue fallback (chosen) | about 0 per watch | async delivery; startup window and fseventsd saturation can delay or drop events |

FSEvents was chosen because the working-tree watcher and ADR-034 already depend on it, one stream covers many directories, and every consumer already has a tick or poll backstop.
