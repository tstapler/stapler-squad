# Working-tree watching for cheap, correct dirty detection

Scope: is "Tier B" (watch the working tree, answer HasUncommitted in O(changed)) viable for the go-git-only unfinished scanner, and what is the minimum correct design.
Labels: VERIFIED = source page opened this session and the statement is on it (the fetch tool returns model-summarised page text, so quotes are paraphrase); INFERRED = my reasoning or a search-result summary, not opened at primary source.

## 1. Watchman query model (design reference)

- Time is expressed as a clock id (`c:123:234`), a named cursor (`n:name`) or a unix timestamp. Clock ids advance "as changes are detected rather than as time moves", so a result's clock fed to the next query gives a race-free delta; timestamps are race-prone. VERIFIED: https://facebook.github.io/watchman/docs/clockspec
- A `since` query returns files changed after the clock. Named cursors need an exclusive view lock and are discouraged. VERIFIED (same page).
- `is_fresh_instance` is true when the clock came from a different watchman instance or a named cursor was never seen; the result is then a full listing of existing files (all `new:true`), not a delta, so the client must rebuild state. `empty_on_fresh_instance` suppresses the dump. VERIFIED: https://facebook.github.io/watchman/docs/cmd/query.html (the "must rebuild" part is the tool's inference from the described behaviour).
- A clock older than the last prune (deleted-file GC, `gc_age_seconds` default 12h) also yields a fresh instance, so a client can detect that it missed changes. VERIFIED via search snippet of https://facebook.github.io/watchman/docs/config (not opened in full).
- Takeaway: the contract is "give me a monotonic token; I will tell you what changed since, or tell you I cannot (fresh instance) and you must rescan". This is the contract to copy.

## 2. Git fsmonitor / untracked cache / split index (design only)

- `fsmonitor--daemon` watches one working directory, keeps recent changes, answers `git status` over IPC; Linux backend inotify (watch limit typically 8192), macOS and Windows built in since 2.37. VERIFIED: https://git-scm.com/docs/git-fsmonitor--daemon and https://github.blog/engineering/infrastructure/improve-git-monorepo-performance-with-a-file-system-monitor/
- Token is opaque, carries daemon PID and a filesystem sync ID. If either mismatches (restart or dropped events) the daemon "declares bankruptcy": answers "assume everything changed", issues a new token, and the client does one full scan. VERIFIED (GitHub blog).
- Index `FSMN` extension stores the token plus an EWAH bitmap of entries that are NOT known-valid; next run clears only entries the daemon reports. `UNTR` stores per-directory untracked lists plus dir stat/hash validity; with fsmonitor it skips the per-directory lstat. `link` (split index) only reduces index rewrite cost, irrelevant to read-side dirty checks. VERIFIED: https://git-scm.com/docs/gitformat-index
- Numbers (4-core Windows laptop, Chromium 393K files): status 17.6s to 0.83s; untracked cache alone about 2x, with fsmonitor about 10x. VERIFIED (GitHub blog).
- Git's fsmonitor refuses network filesystems by default and does not model submodules. VERIFIED (git-scm page).
- Takeaway: git's design is exactly "token + changed-path set + invalidate-only-those + bankruptcy fallback". Same as Watchman.

## 3. OS facilities and Go wrappers

macOS FSEvents (VERIFIED: Apple FSEvents Programming Guide, https://developer.apple.com/library/archive/documentation/Darwin/Conceptual/FSEvents_ProgGuide/UsingtheFSEventsFramework/UsingtheFSEventsFramework.html):
- Recursive by design, one stream covers a subtree; events are directory-granular unless the file-events flag is set (flag not on that page; INFERRED from API reference knowledge).
- 64-bit event IDs, monotonic per host or per device; persisted across reboots; pass `sinceWhen` to replay; replay is advisory (disks touched by other OSes discard the log; backup tools should still sweep periodically). Per-device streams are recommended for persisted IDs. IDs can wrap (`EventIdsWrapped`).
- Validate stored ID by volume UUID (`FSEventsCopyUUIDForDevice`); if UUID differs, or current ID is lower than stored, stored history is invalid.
- Coalescing: parent/child events may merge into `MustScanSubDirs`; kernel/user drop sets Kernel/UserDropped and always also MustScanSubDirs, so checking MustScanSubDirs suffices; requires a recursive rescan of that path. `kFSEventStreamEventFlagRootChanged` (WatchRoot flag) means rescan.
- Guide says start monitoring before scanning and rescan any dir modified during the scan. `FSEventStreamFlushSync` drains pending events (the flush gives read-your-writes before a query).
- Caveat: FSEvents was designed for backup software; may batch, reorder, miss events (watchexec's doc, page now marked "Withdrawn", seen via search snippet only: https://watchexec.github.io/docs/macos-fsevents.html). fsnotify issue #11 (Aug 2025 comment) relays watchexec preferring kqueue. VERIFIED that comment exists: https://github.com/fsnotify/fsnotify/issues/11
- Notable: git's own `lstat` on dirty files can trigger FSEvents, so background polling can feed back into watchers (search snippet of anthropics/claude-code#25750; INFERRED relevance). Our scanner must not generate events it then reacts to; go-git status only reads, so low risk, but ignore events under `.git` that are our own lock/index writes.

kqueue: needs an O_EVTONLY fd per watched file and directory, EMFILE risk, no recursion, inode-following (atomic save by rename goes silent). Source: search summary only (INFERRED-secondary). This is why recursive kqueue on big worktrees across many worktrees is not viable (fds about (files+dirs) x worktrees; macOS default soft limit 256 per process before raising, INFERRED).

Linux inotify: per-directory watch (not per file), `max_user_watches` limit (git doc: 8192 typical), `IN_Q_OVERFLOW` means rescan. fanotify with `FAN_MARK_FILESYSTEM`/FID reports (kernel 5.1+/5.9+/5.13+) can watch a whole mount but needs CAP_SYS_ADMIN for mount marks; INFERRED from man pages, not opened. Linux is secondary for us: keep existing TTL path or inotify per-dir.

Go libraries:
- fsnotify: no FSEvents backend; open issue #11 since 2014, maintainer objects to cgo, no work in flight. kqueue only on macOS. VERIFIED (issue page above).
- `github.com/fsnotify/fsevents`: BSD-3-Clause, wraps FSEvents; README says "not well maintained or well tested", API unstable, 63 commits. Has `Resume` + `EventID` (sinceWhen replay), per-Device streams, `Latency`, `MustScanSubDirs` flag; limit 4096 watched paths; one OS thread per stream; resolve symlinks first. cgo not stated on page (INFERRED yes: C callbacks, CFRunLoop). VERIFIED: https://github.com/fsnotify/fsevents
- `github.com/rjeczalik/notify`: MIT, has FSEvents + kqueue + inotify + ReadDirectoryChangesW backends, recursive watch; cgo for FSEvents inferred from file names (`watcher_fsevents_cgo.go`); maintenance recency not confirmed; overflow semantics undocumented on README. VERIFIED listing: https://github.com/rjeczalik/notify. It does not expose event-ID replay in its API (INFERRED).
- Implications of cgo (INFERRED): needs CGO_ENABLED=1 and Xcode CLT on build host (stapler-squad already built on macOS with `make install-service`, so fine); breaks Linux to macOS cross-compile; codesigning/TCC: FSEvents needs no TCC prompt for ordinary paths but paths under Documents/Desktop/Downloads and external volumes can be TCC-gated; repo already has `make setup-codesign` for stable TCC identity. Worktrees live under `~/.stapler-squad/`, not TCC-protected (INFERRED). No licensing blocker (BSD-3/MIT).
- Alternative without cgo: purego-style binding of CoreServices (suggested in fsnotify#11, no implementation committed). VERIFIED suggestion exists; feasibility INFERRED.

## 4. How other tools decide dirty

- VS Code: recursive watching via @parcel/watcher (FSEvents on macOS) in a UtilityProcess, requests deduped, excludes via `files.watcherExclude`. parcel coalesces events during large operations. VERIFIED via search summary of https://github.com/microsoft/vscode/wiki/File-Watcher-Internals (not opened in full). How the Git extension turns events into `git status` refresh was not found; it still shells out to git status on debounced events (INFERRED).
- jj: optional `fsmonitor.backend = "watchman"` so snapshots skip a full working-copy scan; default `none` scans with stat comparison. `.gitignore` changes force a subtree rescan under watchman; open issues about snapshot races and untrack inconsistencies. VERIFIED (search summary of https://docs.jj-vcs.dev/latest/config/).
- Mercurial fsmonitor + Sapling: watchman-backed dirstate walk; fresh instance means ignore saved state and walk fully. NOT verified (search returned nothing on hg; pointer: `hgext/fsmonitor/__init__.py` `overridewalk`). EdenFS: virtual FS keeps a journal so status needs no scan; Sapling docs fetch 404'd, so UNVERIFIED. Eden is not applicable to ordinary checkouts anyway.
- Pattern across all: watcher yields a candidate set; the VCS's own stat-compare decides; fresh/overflow means full scan.

## 5. Cheap non-watching alternatives

- Index stat cache + racy-git: a stat match is trusted only if entry mtime is strictly older than the index file mtime; otherwise compare content; git smudges size to 0 for racy entries when writing the index. VERIFIED: https://git-scm.com/docs/racy-git. For us: any "skip lstat" shortcut must preserve this rule. Relevant to Tier B too: a file edited in the same timestamp tick as our last clean verdict is racy; compare against the verdict time with a margin, or flush FSEvents before answering.
- Directory mtime pruning for the untracked walk (UNTR idea): cache per-dir mtime plus untracked-list; if dir mtime unchanged, skip re-reading it. Caveat from git's blog: build artifacts inside dirs bump mtimes and invalidate, and a dir mtime changes only on entry add/remove/rename, not on in-place edits of existing files, so it detects new untracked files but NOT modifications of tracked files. VERIFIED caveat (GitHub blog); mtime semantics INFERRED (POSIX). Also needs .gitignore/info/exclude hash validation, as UNTR stores. VERIFIED (gitformat-index).
- go-git status itself: slow with many untracked/ignored files (go-git#181), PR #2282 collapses untracked dirs (about 40% faster in its benchmark, still prewalks for ReadPatterns); nested-worktree descent bug #1896; ignore-prune regression #2369; no fsmonitor or untracked-cache issue exists upstream. VERIFIED via search summary: https://github.com/go-git/go-git/issues/181, /pull/2282, /issues/1896, /issues/2369. (Our fork is allowed to add these.)
- Sharing across worktrees: worktrees have separate working dirs and separate indexes (`.git/worktrees/<n>/index`), so no result is shareable for tracked-file stat checks. Shareable: object-level data (HEAD tree for a given commit, ignore patterns from `info/exclude` and global excludes), and "worktree whose HEAD==base and index==HEAD" fast paths (INFERRED).
- Short-circuit: HasUncommitted is boolean. Early exit on the first modified tracked file, and for untracked, on the first untracked non-ignored entry, avoids the full O(n) when dirty (INFERRED; likely already partly done, not checked in code since read-only research scope).

## 6. Correctness: overflow and missed events

| Tool | Signal | Response |
|---|---|---|
| Watchman | `is_fresh_instance`, pruned clock | client discards state, full rescan |
| git fsmonitor | token PID/SID mismatch | "assume all changed", one full scan, new token |
| FSEvents | MustScanSubDirs, Kernel/UserDropped, RootChanged, UUID mismatch or lower event ID, ID wrap | rescan that path recursively (or everything for UUID/ID problems) |
| inotify | IN_Q_OVERFLOW | full rescan of watched tree (INFERRED, man page not opened) |
| Apple guidance | replay log is advisory | periodic full sweep anyway |

## Recommendation

Verdict: Tier B as "recursive kqueue per worktree" is NOT viable on macOS. Tier B as "one FSEvents stream with persisted event ID, used only as an invalidation oracle for a cached verdict" IS viable on macOS, with real but bounded risk. Do not make it the sole source of truth.

Minimum design (INFERRED engineering synthesis, built from verified contracts above):
1. One FSEvents stream per volume (per-device, `FSEventStreamCreateRelativeToDevice`) rooted at the shared parent(s) that contain all worktrees (a handful of paths, under the 4096 limit) plus the main repos' `.git/worktrees/*` dirs (where each worktree's index and HEAD live). File-level events not required; directory granularity is enough to invalidate.
2. Per-worktree cache entry: `{verdict, verdictEventID, volumeUUID, dirtyDirs set}`. On event: map path to owning worktree (longest-prefix), add dir to its set, bump to "needs recheck". Ignore paths under `.git` except `index`/`HEAD` of the gitdir. No event since `verdictEventID` and no flags: serve cached verdict in O(1). Call `FlushSync` before answering to avoid the coalescing-latency race.
3. Recheck on events: re-run HasUncommitted scoped to `dirtyDirs` (stat the index entries under those dirs, walk only those dirs for untracked), then fall back to current whole-tree path if scoping isn't implemented.
4. Fallback to today's full scan (the existing TTL path) when: MustScanSubDirs, Kernel/UserDropped, RootChanged, UUID mismatch, stored ID greater than current ID, ID-wrap flag, stream restart without replay, worktree first seen, `.gitignore`/`info/exclude` event (ignore rules changed, same lesson as jj 0.38), or any error. Keep a max-age TTL safety net regardless (Apple: log is advisory).
5. Persist `{eventID, volumeUUID}` at a coarse interval so restarts replay via `sinceWhen` instead of rescanning everything; on replay HistoryDone flag distinguishes replay from live (flag from API reference knowledge, INFERRED; not on the guide page).
6. Linux: keep TTL path, or per-directory inotify with overflow = rescan; do not block macOS work on it.

go-git fork primitive needed (INFERRED): a scoped status API, e.g. `StatusOptions.Paths`/`DirPrefixes` (stat only index entries under given dirs; walk untracked only under given dirs) with an early-exit "first dirty" mode returning bool. Without it, events only tell you WHEN to recheck, not make the recheck cheaper, so wins are limited to clean worktrees (the common case: cached clean verdict never rechecked until an event arrives). Also useful: expose index mtime so racy-entry rule can be applied, and an untracked-dir cache keyed by dir mtime plus ignore-file hashes (UNTR analogue).

Main risks:
- Missed or reordered FSEvents (watchexec's claim, unverified at primary source): mitigated by safety TTL and fail-open to rescan; a missed event yields a stale "clean" verdict until TTL, which is the correctness cost to accept or reject explicitly.
- cgo dependency and unmaintained `fsnotify/fsevents` ("not well maintained"): likely need to vendor or own a thin cgo binding (or purego); breaks Linux-to-macOS cross-compiles.
- Event storms (`git checkout`, builds, `node_modules`): coalesce with latency and cap `dirtyDirs` size; overflow of the set means full rescan of that worktree.
- Feedback loops: scanner or other tooling reading files can generate FSEvents (git lstat on dirty files, per claude-code#25750 snippet); filter `.git` noise.
- Racy timestamps: verdict taken within the same mtime tick as a write; flush plus margin.
- Gain is concentrated on clean/idle worktrees; no benefit is proven until benchmarked against the TTL baseline. Recommend a spike measuring event volume across real worktrees before committing.

## Source gaps (not verified)
Primary sources not opened: Watchman config page, VS Code File Watcher wiki, jj config doc, Mercurial fsmonitor source, Sapling/EdenFS docs (404), inotify/fanotify man pages, FSEventStreamEventFlags reference (HistoryDone, file-events flag), rjeczalik/notify source (cgo, overflow), fsnotify kqueue fd analysis. The watchexec FSEvents page is marked Withdrawn.
