# Architecture Research: files-viewer-revamp

**Date**: 2026-10-16 (SDD Phase 2, architecture dimension)
**Builds on**: `project_plans/better-file-tree/research/architecture.md` (cited as BFT), `project_plans/diff-stats-file-watch-cache/research/architecture.md` (DSC), `project_plans/mobile-ux-improvements/research/findings-architecture.md` (MUX).
Labels: VERIFIED = opened/ran; INFERRED = reasoned, not run.

## 1. Existing integration surface (VERIFIED)

| Concern | Where | Notes |
|---|---|---|
| File RPCs | `proto/session/v1/session.proto:300,305,310`; messages `:2286-2365` | `ListFiles`, `GetFileContent`, `SearchFiles` (name-substring, unary, cap 500). No write RPC, no content search. |
| Handlers | `server/services/file_service.go` (889 lines): `ListFiles :118`, `GetFileContent :282`, `SearchFiles :444`; delegated from `server/services/session_service_delegates.go:648,842` | `FileService` is a separate struct with a `WorkspaceProvider`, `DirCache`, `GitignoreCache`. New RPCs should be added here and delegated the same way. |
| Worktree resolution | `fs.workspace.GetWorkspace(sessionId)` then `ws.ExistingDir` (`file_service.go:~130,~296`); path concepts defined at `session/types.go:323-353` | `.claude/rules/instance-lock-free-reads.md`: reading or writing files uses `Workspace().ExistingDir`. Note `ExistingDir` falls back to the repo root when a worktree is paused/deleted, so **a write could land in the main repo checkout**; the write RPC must require a real `WorktreeDir` (or `ActiveDir == ExistingDir`) and refuse otherwise. |
| Path confinement | `resolveAndValidatePath` `file_service.go:106-116` | **Lexical only** (`filepath.Clean` + prefix). No `EvalSymlinks`. `GetFileContent` does `os.Lstat` then `os.Open` (follows symlinks), so a symlink inside the worktree pointing outside can be read today (read-side latent gap, INFERRED from reading; not exploited). For a write this is unacceptable. |
| Gitignore | `go-git/.../format/gitignore` matcher + `GitignoreCache` (`file_service.go:21,91,597-700`) | Already go-git based; search walk `searchFilesInWorktree :497` uses `filepath.WalkDir` + matcher. |
| Feature flags | `server/services/feature_flag_service.go`: `knownFeatureFlags` registry (name, description, defaultValue; ~`:300-331`), `featureFlagDefault`, `SetFeatureController`, RPCs `GetFeatureFlags`/`UpdateFeatureFlag` (`session.proto:505,508`); live-read example `guards_flag.go:15-54`, `ProgramCLIFlagProbeEnabled` (`feature_flag_service.go:45`) reads `config.LoadConfig().GetFeatureFlagWithDefault(...)` per request | Live-settable and global+scoped overrides already exist; a flag is: const name, registry entry, per-request read. |
| Frontend | `web-app/src/components/sessions/FilesTab.tsx` (296), `FileTree.tsx` (970), `FileContentViewer.tsx` (693), `useFileService.ts`, `RecentFilesSection.tsx`; `useSessionVcsContext()` consumed at `FilesTab.tsx:60`, provider mounted in `SessionDetail.tsx:7` | FilesTab holds single `selectedPath` (`:38`), `recentPaths` (`:46`), `mobilePane` (`:45`), `isQuickOpenOpen` (`:47`). CodeMirror 6 packages present in `web-app/package.json:45-92` (no vim package currently). |
| Feature registry | `docs/registry/features/<id>.json` (e.g. `file-chip-list.json`); `make registry-generate` | Add `// +api:` markers in Go handlers and `// +feature:` in new React files; commit generated per-feature JSON. |
| Watcher / cache | `session/git_worktree_watcher.go` (`WorktreeChangeDetector`: `.git`-dir fsnotify + 15 s fingerprint, `:19,59-122`); per-session caches in `GitWorktreeManager` and `WorkspaceService` (`vcsStatusCache`) per DSC §1 | Plain working-tree edits do NOT produce fsnotify events (the detector watches `.git` only); working-tree changes are caught by the 15 s fingerprint poll. |

## 2. Patterns by decision

### 2.1 Buffer/tab state model (frontend)

Recommendation: a `useOpenBuffers` reducer-backed hook (plain `useReducer` + a context-free hook owned by FilesTab; no new global store), BFT §2's "single owner, no context" reasoning still applies (BFT:68). State per buffer:

```ts
interface Buffer {
  path: string;
  loaded: { content: string; baseToken: FileVersionToken; isTruncated: boolean; isBinary: boolean } | null;
  draft: string | null;          // null = clean
  viewState: { scrollTop: number; selection: {anchor:number; head:number} };  // restore on tab switch
  lastAccess: number;
}
state = { buffers: Buffer[]; activeIndex: number; history: string[]; historyPos: number }  // back/forward
```

- Keep one CodeMirror `EditorView` and swap `EditorState`s (CM6's documented pattern: `view.setState(buffer.state)`; scroll and selection live inside the state/`scrollSnapshot`), rather than one view per tab. This is the lever for the mobile memory requirement (one view, N inert states). INFERRED from CM6 API knowledge; Phase 3 should spike-confirm.
- This **replaces** `selectedPath` + `recentPaths` + `RecentFileEntry` (BFT:70-103) as the source of truth; `selectedPath` becomes derived (`buffers[activeIndex].path`), so existing `onSelectedPathChange` / `initialSelectedPath` cross-links keep working. `RecentFilesSection` stays, fed from MRU of `lastAccess`.
- Persistence (open question): localStorage keyed by session id, **paths + active index + viewState only, never draft text** (drafts may contain secrets; also stale vs disk). Server-side persistence adds an RPC and a cross-device story the owner has not asked for; defer. Wrap storage in try/catch.
- Dirty drafts: guard tab close/navigation with the existing unsaved-state pattern; cap open buffers (suggest 10 on mobile, LRU-evict only clean buffers).

### 2.2 Search service (backend)

Backend choice, in repo-convention order (`prefer-go-git-over-subshells` skill: no `git`/`rg` subprocess where a library works):

1. **In-process Go walker + `regexp`/`strings`** reusing `searchFilesInWorktree`'s walk and the cached go-git gitignore matcher. Zero external dependency, same ignore semantics as ListFiles/SearchFiles (so tree and search agree), runs on every host. Cost: slower than ripgrep on ~50k files; mitigate with a bounded worker pool, binary sniff (first 8 KB NUL check), per-file size cap (reuse 1 MB), skip `hardSkipDirs`, context cancellation, and early termination at a result cap.
2. **ripgrep as optional accelerator** behind an interface (`ContentSearcher`), `exec.LookPath("rg")` at construction, fall back to (1) when absent. Subprocess-spawning is the pattern the repo discourages, so this must be a measured follow-up, not v1. Ignore semantics differ (rg also honors `.ignore`, global gitignore, `.git/info/exclude`); results would diverge from the tree. Decision for Phase 3: ship (1); gate (2) on a benchmark against the 2 s / 50k-file target (requirements `:35`). I did not run a benchmark; the 2 s target is UNVERIFIED for approach (1).
3. `git grep` via go-git: go-git's `Worktree.Grep` exists but only searches tracked files at HEAD/worktree and is slow; rejected as the primary path because it misses untracked files that agents create (INFERRED; not tested).

Interface sketch (domain-owned, keeps FileService free of engine detail):

```go
type ContentSearcher interface {
  Search(ctx context.Context, root string, q SearchQuery, emit func(Match) bool) (SearchStats, error)  // emit returns false to stop
}
```

### 2.3 Streaming vs unary search results

The repo already uses server-streaming RPCs (`WatchSessions :45`, `WatchReviewQueue :70`, `WatchWorkflows :578`), so the transport is proven. Trade-off:

| | Unary (`SearchContent`) | Server-streaming (`SearchContentStream`) |
|---|---|---|
| First-result latency (target < 2 s) | Waits for full walk or cap | Batches stream as found |
| Cancel on keystroke | Request abort only; server walk continues unless ctx cancelled (it is, via `ctx`) | Cancel stream, same ctx path; cleaner UI progress |
| Mobile / proxies | Simple, retry-friendly | Long-lived streams over LAN/Tailscale/PWA are already used (WatchSessions), but unary is more robust to intermittent connectivity |
| Complexity | Low; matches existing `SearchFiles` shape (`truncated`, `total_matches`) | Frontend must accumulate, dedupe, handle partial-on-abort |

Recommendation: **define the RPC as server-streaming from day one** (`rpc SearchContent(SearchContentRequest) returns (stream SearchContentResponse)`), with each message a batch of file matches plus a final message carrying `truncated`, `files_scanned`, `duration_ms`. Rationale: a proto stream can be consumed as "collect all" by a trivial client, but a unary RPC cannot later become streaming without a new RPC and breaking change; the < 2 s first-results SLO is only meetable on big repos with streaming. Ship the UI as incremental results with a debounce (~250 ms) and abort-on-new-query. Fallback if streaming proves flaky on mobile: the same handler body can back a unary wrapper (INFERRED). Request fields: `session_id`, `query`, `is_regex`, `case_sensitive`, `whole_word`, `include_glob`, `exclude_glob`, `include_ignored`, `max_results`, `context_lines` (0 default). Minimum query length guard like SearchFiles (`:2`), regex compile errors returned as `InvalidArgument`, and a regex-complexity bound (Go's RE2 is linear-time, so ReDoS is not a concern; still cap match line length when returning snippets).

### 2.4 Write path with optimistic concurrency

New RPC `WriteFile(session_id, path, content (bytes), expected_token, encoding/eol metadata?)` returning the new token. Extend `GetFileContent` (additive field) with `version_token`; do not change existing semantics.

**Token choice (open question)**: a content hash alone cannot detect a concurrent change-and-revert (harmless) but needs a full read; `mtime+size` is cheap but coarse (mtime granularity, `touch`, git checkout restoring old mtime is rare). Recommended: **opaque token = `size:mtime_ns:sha256(content)`** computed by the server on read; on write, server `Lstat`s, compares `size+mtime_ns` first (fast path, equal means unchanged) and, if they differ, hashes the on-disk content and accepts if the hash still equals the token hash (mtime bumped, content unchanged), else returns `FailedPrecondition`/conflict with the current content and token so the client can show a diff without a second round-trip. Git blob id rejected: requires hashing identically to git (CRLF filters) and gives nothing extra over sha256. Truncated files (>1 MB, `is_truncated`) and binary files must be **non-editable** (write would destroy the tail); the server must also reject `WriteFile` for them via a token that encodes truncation or a separate guard.

**Atomicity**: write to a temp file in the same directory (`os.CreateTemp` + `fsync` + `os.Rename`), preserving mode bits; this protects against partial writes seen by a concurrent agent. Trade-off: rename replaces the inode (breaks hard links, loses inotify watch on that inode for tools like `tail -f`; acceptable). A compare-then-rename sequence still has a tiny TOCTOU window versus an agent's own write; acceptable for a single-user tool, mitigated by re-`Lstat` immediately before rename. Do not claim stronger. Per-path `sync.Mutex` map (keyed by cleaned absolute path) serializes concurrent browser saves; this does not and cannot serialize against agent processes.

**Confinement (security requirement `:36`)**: the write path needs stricter validation than `resolveAndValidatePath`: `filepath.EvalSymlinks` on the parent directory, verify the resolved parent is within `EvalSymlinks(baseDir)`, `Lstat` the target and **reject if it is a symlink** (do not follow), reject directories, reject paths under `.git/` (agents' and users' hooks live there; writing `.git/hooks/*` is code execution), and require existence (no create in v1). Phase 3 should also retrofit the same resolver into `GetFileContent` (collateral debt, own commit per the user's standing rule). Also: require a real worktree (`WorktreeDir != ""`), size cap on content (reuse 10 MB, practically lower), reject NUL bytes unless the original is the same encoding, preserve original EOL style and trailing newline (round-trip: client reports detected EOL; server never normalizes). Encoding: v1 supports UTF-8 only; non-UTF-8 files are read-only (GetFileContent currently returns `encoding` always "utf-8" per proto comment `:2322`).

**Guards/ownership**: register `WriteFile` with the hidden-session write guard (`hidden_session_readonly_guards` flag, `guards_flag.go`) so background sessions refuse UI writes, consistent with other write RPCs. INFERRED that guard registration is per-RPC; Phase 3 to confirm in `server/services/guards*`.

### 2.5 Feature flags

Add `files_editing` (default **off**) and optionally `files_viewer_v2` (default on or off per rollout) to `knownFeatureFlags`, per requirements `:78`. Server reads the flag per request in `WriteFile` (return `FailedPrecondition`/`PermissionDenied` "editing disabled" when off); frontend reads via the existing `GetFeatureFlags` plumbing to hide editor mode. Flag names in this registry use `area:name` or snake_case both (e.g. `terminal:resync-correlation-id`, `hidden_session_readonly_guards`); pick `files:editing` / `files:viewer-v2` to match the majority newer style (INFERRED from the sample I saw, not a count). Per-scope override already supported by `UpdateFeatureFlag`. Remember `docs` + `knownFeatureFlags` test (`feature_flag_service_test.go`) may assert registry contents.

### 2.6 go-git convention

`prefer-go-git-over-subshells`: the search engine, gitignore matching, and any tracked/untracked classification stay in-process (go-git `gitignore` already imported in `file_service.go:21`). The only subprocess candidate is `rg` (§2.2), which is why it is deferred behind an interface. No `git` CLI calls are needed for save: git status/diff derive from the working tree afterward.

## 3. Data flow and consistency

```
Browser editor --WriteFile(path, content, token)--> FileService
  -> workspace.GetWorkspace -> ExistingDir/WorktreeDir check
  -> confine (EvalSymlinks, not symlink, not .git)
  -> per-path mutex -> Lstat/compare token -> temp+fsync+rename
  -> invalidate caches (below) -> return new token
```

- **Agents writing the same worktree**: a conflict is detected only at the compare point; between compare and rename an agent write can be lost (small window, see §2.4). The reverse also holds: an agent can overwrite the user's saved edit at any time since agents are unaware. The UI should surface "changed on disk" proactively: re-check token on tab focus/visibility and on window refocus (cheap `GetFileContent`-with-token or a new `StatFile`), not only at save. A push channel is not warranted yet.
- **Git status/diff caches** (DSC): the VCS status cache is invalidated by `WorktreeChangeDetector` which watches only `.git` and polls a fingerprint every 15 s (`git_worktree_watcher.go:19`), so a pure working-tree save would show stale badges for up to ~15 s (INFERRED from reading the detector's header; I did not run it). Recommendation: after a successful write, call an explicit invalidation (`Instance`-level, the same callback DSC §1 designed: `OnWorktreeChange` consumers / `vcsStatusCache.Delete(workDir)` plus clearing `diffStatsAt`) and have the frontend call `refreshStatus()` from `useSessionVcsContext()` (`FilesTab.tsx:60`) on save success. Whether `OnWorktreeChange`/a public "invalidate now" method exists on `Instance` at this commit must be checked in Phase 3 (DSC proposed it; I did not verify it landed).
- `DirCache` (512 entries, 30 s TTL, `file_service.go:98`): saving an existing file does not change directory listings, except size/mtime if exposed; no invalidation needed for v1. Create/rename/delete (out of scope) would require invalidation.
- Search must see saved content immediately: no content index in v1 (walk-per-query), so no staleness class. A future index would reintroduce this; note as a reason not to add one now.
- Observability (requirements `:73`): one slog line per write (`session`, `path`, `bytes`, `outcome`: `ok|conflict|denied|disabled|error`) and per search (`query_len`, `duration_ms`, `results`, `truncated`); never content or query text at info.

## 4. Hotspot / boundary disposition (for Phase 3 to formalize)

| Area | Evidence | Disposition |
|---|---|---|
| `FilesTab.tsx` (296) | Single-file `selectedPath` + MRU + palette + mobile pane state all inline (`:38-49`); will need buffers, search panel, editing mode | **Isolate via seam**: extract a `useOpenBuffers` hook and a `FilesWorkspaceContext`-free prop contract; keep FilesTab as the thin composition root. Small enough to modify directly once state is behind the hook. |
| `FileContentViewer.tsx` (693) | Mixes breadcrumb, language detection, CodeMirror setup (`CodeMirrorViewer :194` ~300 lines), image/PDF/video branches (`:496+`), data fetching | **Refactor-first (narrow)**: split `CodeMirrorViewer` into a reusable `CodeEditor` (view + state swapping, read-only vs editable via a `Compartment`) and leave media viewers alone. Editing built into the current 300-line effect-driven component would entrench it. Do this as an own commit before the editing epic. |
| `FileTree.tsx` (970) | Tree, search filter, recent section, imperative handle (BFT:203), git badges; this feature does not need to change tree internals | **Extend as-is**: only add a "reveal in tree" call via the existing `FileTreeHandle.revealPath`. Do not refactor 970 lines for this feature; file it as separate debt. |
| `server/services/file_service.go` (889) | All file RPCs, walking, gitignore, raw serving in one file | **Isolate via seam**: put search in `content_search.go` and write in `file_write.go` (same package, `FileService` methods) behind `ContentSearcher`; extract a shared `resolveInWorktree` (symlink-safe) used by all RPCs. No move of existing code needed. |
| Mobile layout (`mobilePane`, MUX) | MUX §2 recommends the `visualViewport` CSS-variable hook as the keyboard-safe layout primitive (MUX:62-116, recommendation `:296`) | **Extend**: reuse that hook for the editor + touch toolbar; do not reimplement. |

## 5. Risks and gaps to carry forward

- Symlink escape in `GetFileContent`/`ListFiles` today (read side) is real but unverified by exploit; the write path must not inherit it. Cite `file_service.go:106-116`.
- `ExistingDir` fallback to repo root can aim a write at the main checkout; guard explicitly.
- 2 s / 50k-file search SLO is unmeasured; Phase 3/4 needs a benchmark task (benchmarks must run with `&` per CLAUDE.md).
- Vim mode: `@replit/codemirror-vim` is not in `package.json` (VERIFIED by grep of `codemirror` entries); handled in stack research, but it interacts with the buffer model (vim registers/marks per view, not per state).
- Existing e2e `tests/e2e/local-file-browser.spec.ts` and `web-app` unit tests must keep passing; selectors depend on tree/viewer markup, another reason the `CodeEditor` extraction must preserve test ids.
